package routers

// On-demand reading audio: extract the same speakable text the browser player sees, ask the
// local TTS sidecar for an MP3, map word boundaries onto sentences, and publish the files
// through the existing /blog/audio route.
//
// The sidecar (tts_server.py, bound to loopback) owns edge-tts and the Python runtime. The Go
// service never spawns a process: it POSTs JSON with the text and two output paths. That keeps
// the service binary free of an exec surface and lets the TTS runtime be restarted, upgraded or
// replaced (for example by an Azure voice) without touching the blog binary.
//
// This is deliberately a single-worker queue. TTS is network-bound and concurrent generation
// would only make every request slower; the queue cap turns load into a clear "busy" response.
//
// 按需朗读：提取与浏览器播放器一致的可朗读文本，向本地 TTS sidecar 要一份 MP3，把词边界映射到
// 句子，再通过既有 /blog/audio 路由发布。
//
// sidecar（tts_server.py，只听 loopback）拥有 edge-tts 与 Python 运行时；Go 服务不启动任何进程，
// 只 POST 一份带文本和两个输出路径的 JSON。二进制因此没有 exec 面，TTS 运行时也可以独立重启、
// 升级或替换（例如换成 Azure 音色）而不动博客二进制。
//
// 刻意单工作协程：TTS 网络密集，并发生成只会让每个请求都更慢；队列上限把过载变成明确的「忙」。

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/haierkeys/fast-note-sync-service/internal/app"
)

const blogAudioDefaultVoice = "zh-CN-XiaoxiaoNeural"

// Loopback only. The sidecar is a local capability, not a public endpoint.
// 仅 loopback。sidecar 是本地能力，不是公开端点。
const blogAudioTTSURL = "http://127.0.0.1:8765/synthesize"

var blogAudioSlugRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,120}$`)

const (
	blogAudioQueued      = "queued"
	blogAudioGenerating  = "generating"
	blogAudioReadyStatus = "ready"
	blogAudioFailed      = "failed"
)

type blogAudioBound struct {
	T string `json:"t"`
	O int64  `json:"o"`
	D int64  `json:"d"`
}

type blogAudioSentence struct {
	S int64  `json:"s"`
	E int64  `json:"e"`
	T string `json:"t"`
}

type blogAudioTimingsDoc struct {
	Version   int                 `json:"version"`
	TextHash  string              `json:"textHash"`
	Voice     string              `json:"voice"`
	Duration  int64               `json:"duration"`
	Sentences []blogAudioSentence `json:"sentences"`
}

type blogAudioJob struct {
	Slug      string
	TextHash  string
	Text      string
	Sentences []string
	Voice     string
	Estimate  int64
}

type blogAudioJobState struct {
	Status   string
	Estimate int64
	Error    string
	// TextHash identifies which version of the article this job produced. A "ready" state from
	// an older hash must not satisfy a prepare for the current text: that is how the player used
	// to accept the previous, shorter audio and skip the regeneration it had just requested.
	//
	// TextHash 标识这个任务产出的是哪一版正文。旧哈希的 ready 状态不能当作当前正文已就绪——
	// 播放器曾经因此接受上一版更短的音频，把它刚请求的重生成晾在一边。
	TextHash string
}

type blogAudioGen struct {
	app   *app.App
	mu    sync.Mutex
	jobs  map[string]*blogAudioJobState
	queue chan *blogAudioJob
}

func newBlogAudioGen(a *app.App) *blogAudioGen {
	g := &blogAudioGen{
		app:   a,
		jobs:  make(map[string]*blogAudioJobState),
		queue: make(chan *blogAudioJob, 4),
	}
	go g.worker()
	return g
}

func (g *blogAudioGen) status(slug string) *blogAudioJobState {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.jobs[slug]
	if st == nil {
		return nil
	}
	copied := *st
	return &copied
}

func (g *blogAudioGen) enqueue(job *blogAudioJob) (string, int64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if st := g.jobs[job.Slug]; st != nil {
		switch st.Status {
		case blogAudioQueued, blogAudioGenerating:
			// A request for the same text is already in flight; report it instead of adding a
			// duplicate. A different hash while one job runs is rare (the article changed during
			// generation); the next page load will ask again, so do not complicate the queue.
			//
			// 同一正文已在队列/生成中：直接报告状态，不重复入队。生成期间正文变了属于罕见情况，
			// 下一次打开页面会重新请求，不必为此复杂化队列。
			return st.Status, st.Estimate, true
		case blogAudioReadyStatus:
			// The state says ready, but that is only trustworthy if the manifest entry still
			// exists, its file is on disk, and it belongs to the same text. A deleted manifest
			// row or an edited article must fall through to a new job rather than handing the
			// caller an empty "ready".
			//
			// 状态说 ready，但只有清单条目仍在、文件在磁盘上、且属于同一正文时才可信。清单被删
			// 或正文已改时必须落到新任务，而不是把一个空的 "ready" 交给调用者。
			if st.TextHash == job.TextHash {
				if ready := blogAudioReady(g.app, job.Slug); ready != nil && ready.TextHash == job.TextHash {
					return blogAudioReadyStatus, 0, true
				}
			}
		}
	}
	select {
	case g.queue <- job:
		g.jobs[job.Slug] = &blogAudioJobState{Status: blogAudioQueued, Estimate: job.Estimate, TextHash: job.TextHash}
		return blogAudioQueued, job.Estimate, true
	default:
		return "", 0, false
	}
}

func (g *blogAudioGen) setState(slug, textHash, status string, estimate int64, errMsg string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.jobs[slug]
	if st == nil {
		st = &blogAudioJobState{}
		g.jobs[slug] = st
	}
	st.Status = status
	st.Estimate = estimate
	st.Error = errMsg
	st.TextHash = textHash
}

func (g *blogAudioGen) worker() {
	for job := range g.queue {
		g.setState(job.Slug, job.TextHash, blogAudioGenerating, job.Estimate, "")
		if err := g.generate(job); err != nil {
			g.setState(job.Slug, job.TextHash, blogAudioFailed, job.Estimate, err.Error())
			continue
		}
		g.setState(job.Slug, job.TextHash, blogAudioReadyStatus, 0, "")
	}
}

func blogAudioVoiceAllowed(voice string) bool {
	switch voice {
	case blogAudioDefaultVoice, "zh-CN-YunxiNeural", "zh-CN-YunyangNeural", "zh-CN-XiaoyiNeural":
		return true
	}
	return false
}

func (g *blogAudioGen) generate(job *blogAudioJob) error {
	if !blogAudioSlugRe.MatchString(job.Slug) {
		return fmt.Errorf("invalid slug")
	}
	if !blogAudioVoiceAllowed(job.Voice) {
		return fmt.Errorf("unsupported voice")
	}

	dir := blogAudioDir(g.app)
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create audio dir: %w", err)
	}

	payload, err := json.Marshal(map[string]string{
		"text":  job.Text,
		"voice": job.Voice,
	})
	if err != nil {
		return fmt.Errorf("encode job: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, blogAudioTTSURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build tts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("tts sidecar unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var result struct {
		OK     bool   `json:"ok"`
		Audio  string `json:"audio"`
		Bounds string `json:"bounds"`
		Error  string `json:"error"`
	}
	_ = json.Unmarshal(body, &result)
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(result.Error)
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		return fmt.Errorf("tts sidecar %s: %s", resp.Status, msg)
	}
	if !result.OK || !blogAudioFileRe.MatchString(result.Audio) || !blogAudioFileRe.MatchString(result.Bounds) {
		return fmt.Errorf("tts sidecar returned an invalid job result")
	}

	audioPath := filepath.Join(outDir, result.Audio)
	boundsPath := filepath.Join(outDir, result.Bounds)
	boundsBytes, err := os.ReadFile(boundsPath)
	if err != nil {
		return fmt.Errorf("read bounds: %w", err)
	}
	defer os.Remove(boundsPath)
	var bounds []blogAudioBound
	if err := json.Unmarshal(boundsBytes, &bounds); err != nil {
		return fmt.Errorf("parse bounds: %w", err)
	}

	timed, duration := blogAudioBuildTimings(job.Text, job.Sentences, bounds)
	doc := blogAudioTimingsDoc{
		Version:   1,
		TextHash:  job.TextHash,
		Voice:     job.Voice,
		Duration:  duration,
		Sentences: timed,
	}
	timingsBytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal timings: %w", err)
	}
	timingsPath := filepath.Join(dir, job.Slug+".timings.json")
	if err := blogAudioWriteAtomic(timingsPath, timingsBytes); err != nil {
		return fmt.Errorf("write timings: %w", err)
	}

	finalMP3 := filepath.Join(dir, job.Slug+".mp3")
	if err := os.Rename(audioPath, finalMP3); err != nil {
		return fmt.Errorf("move mp3: %w", err)
	}

	manifest := blogAudioManifestFresh(g.app)
	if manifest.Items == nil {
		manifest.Items = make(map[string]blogAudioManifestEntry)
	}
	manifest.Items[job.Slug] = blogAudioManifestEntry{
		File:     job.Slug + ".mp3",
		Timings:  job.Slug + ".timings.json",
		TextHash: job.TextHash,
		Duration: duration,
		Voice:    job.Voice,
	}
	if err := blogAudioManifestStore(g.app, manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func blogAudioManifestFresh(a *app.App) *blogAudioManifest {
	b, err := os.ReadFile(blogAudioManifestPath(a))
	if err != nil {
		return &blogAudioManifest{Version: 1, Items: make(map[string]blogAudioManifestEntry)}
	}
	var m blogAudioManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return &blogAudioManifest{Version: 1, Items: make(map[string]blogAudioManifestEntry)}
	}
	if m.Items == nil {
		m.Items = make(map[string]blogAudioManifestEntry)
	}
	return &m
}

func blogAudioManifestStore(a *app.App, m *blogAudioManifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return blogAudioWriteAtomic(blogAudioManifestPath(a), b)
}

func blogAudioWriteAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func blogAudioEstimate(text string) int64 {
	chars := int64(utf8.RuneCountInString(text))
	sec := 15 + chars/70
	if sec < 20 {
		sec = 20
	}
	return sec
}

// blogAudioExtract mirrors the browser's buildArticleText: h2/h3/h4/p/li blocks, own text
// (child blocks skipped), math and code-block subtrees excluded, whitespace collapsed.
//
// The walk must not stop at a block element. Obsidian renders a list item with a nested
// paragraph as <li><p>…</p></li>; the browser's querySelectorAll sees both the empty <li> and
// the <p>, while an early return here saw only the <li> and dropped the text. That made the
// transcript shorter than the page, changed textHash, and the player then refused to paint any
// highlight — the "highlight is completely broken" report. Descending after a block keeps the
// two extractors identical.
//
// blogAudioExtract 与浏览器 buildArticleText 一致：h2/h3/h4/p/li 块、取自身文本（跳过子块）、
// 排除公式与代码块子树、折叠空白。
//
// 遍历不能在块元素处停止。Obsidian 会把「列表项里的段落」渲染成 <li><p>…</p></li>；浏览器的
// querySelectorAll 同时看到空的 <li> 和 <p>，而这里提前 return 只看到 <li>，正文因此丢失。
// 后果是转写文本比页面短、textHash 不一致，播放器于是拒绝画高亮——正是「高亮完全失效」的
// 报告。块处理完继续下探，两边才会逐字一致。
func blogAudioExtract(body string) (string, []string) {
	nodes, err := html.ParseFragment(strings.NewReader(body), &html.Node{
		Type:     html.ElementNode,
		Data:     "div",
		DataAtom: atom.Div,
	})
	if err != nil {
		return "", nil
	}

	var blocks []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if blogAudioSkipSubtree(n) {
				return
			}
			switch strings.ToLower(n.Data) {
			case "h2", "h3", "h4", "p", "li":
				text := strings.Join(strings.Fields(blogAudioOwnText(n)), " ")
				if text != "" {
					blocks = append(blocks, text)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	if len(blocks) == 0 {
		return "", nil
	}
	return strings.Join(blocks, "\n"), blogAudioSplitSentences(blocks)
}

// blogAudioSkipSubtree is the Go counterpart of the browser's el.closest(...) guard. Math is
// excluded as a whole: reading the LaTeX source aloud ("backslash rightarrow") is never what
// the note means, and leaving it in the transcript would only make the two sides disagree once
// KaTeX has replaced the source with its rendering.
//
// blogAudioSkipSubtree 是浏览器 el.closest(...) 判断的 Go 版本。公式整棵排除：把 LaTeX 源码
// 读成「反斜杠 rightarrow」从来不是笔记的意思；KaTeX 用排版结果替换源码后，留在转写里也只会
// 让两边对不上。
func blogAudioSkipSubtree(n *html.Node) bool {
	switch strings.ToLower(n.Data) {
	case "pre", "code", "table", "figure":
		return true
	}
	for _, cls := range strings.Fields(blogAudioClass(n)) {
		switch cls {
		case "quiz", "fns-quiz", "fns-graph", "fns-audio-ignore",
			"math", "math-inline", "math-block", "katex":
			return true
		}
	}
	return false
}

func blogAudioInlineSkip(n *html.Node) bool {
	for _, cls := range strings.Fields(blogAudioClass(n)) {
		switch cls {
		case "math", "math-inline", "math-block", "katex", "fns-audio-ignore":
			return true
		}
	}
	return false
}

func blogAudioClass(n *html.Node) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, "class") {
			return a.Val
		}
	}
	return ""
}

func blogAudioOwnText(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			b.WriteString(c.Data)
		case html.ElementNode:
			tag := strings.ToLower(c.Data)
			switch tag {
			case "ul", "ol", "pre", "table", "figure", "blockquote", "p", "h1", "h2", "h3", "h4", "h5", "h6", "div", "hr":
				continue
			}
			if blogAudioInlineSkip(c) {
				continue
			}
			b.WriteString(blogAudioOwnText(c))
		}
	}
	return b.String()
}

func blogAudioSplitSentences(blocks []string) []string {
	var out []string
	for _, block := range blocks {
		for _, s := range blogAudioSplitBlock(block) {
			out = append(out, s)
		}
	}
	return out
}

func blogAudioSplitBlock(text string) []string {
	var raw []string
	var b strings.Builder
	flush := func() {
		s := strings.TrimSpace(b.String())
		if s != "" {
			raw = append(raw, s)
		}
		b.Reset()
	}
	for _, r := range text {
		if r == '\n' {
			flush()
			continue
		}
		b.WriteRune(r)
		switch r {
		case '。', '！', '？', '!', '?', '；', ';':
			flush()
		}
	}
	flush()

	var out []string
	for _, s := range raw {
		rs := []rune(s)
		for len(rs) > 120 {
			cut := -1
			for i := 80; i < len(rs) && i < 120; i++ {
				if rs[i] == '，' || rs[i] == ',' || rs[i] == '、' {
					cut = i
				}
			}
			if cut < 0 {
				cut = 110
			}
			out = append(out, strings.TrimSpace(string(rs[:cut+1])))
			rs = []rune(strings.TrimSpace(string(rs[cut+1:])))
		}
		if len(rs) > 0 {
			out = append(out, string(rs))
		}
	}
	return out
}

// blogAudioBuildTimings maps edge-tts word boundaries back onto the source sentences.
//
// Matching happens on a whitespace-free copy of both sides. Boundary text is not a clean slice
// of the input: edge-tts reports SSML-escaped fragments (">" arrives as "&gt;" and can be split
// into "&", "g", …), joins punctuation from both sides of a block break ("）\n缺"), and carries
// spaces the source may not have. An exact byte search therefore fails on a large part of a
// long article. Worse, a fragment like "的" would still match somewhere far ahead, moving the
// cursor past the real position; every remaining boundary then fails and the interpolator
// stretches the tail beyond the audio.
//
// So: unescape, remove whitespace, and only accept a match inside a small window after the
// previous one. A miss does not move the cursor. Sentences without any matched word are then
// filled from their neighbours, proportionally to their length, instead of being appended after
// the previous estimate. Duration is the last real word end — never an extrapolated one.
//
// blogAudioBuildTimings 把 edge-tts 词边界映射回源句子。
//
// 匹配在「去掉空白」的副本上进行。边界文本并不是输入的干净切片：edge-tts 会报告 SSML 转义后
// 的碎片（">" 以 "&gt;" 到达，还可能拆成 "&"、"g"……），跨块断行时把两侧标点连在一起
// （"）\n缺"），并带上原文没有的空格。因此在长文里逐字节搜索会大量失败；更糟的是 "的" 这种
// 碎片总能在很远的地方命中，游标被推过真实位置，其后所有边界都失败，插值把尾部拉得比音频还长。
//
// 因此：先反转义、去空白，并且只接受上一次命中之后一个小窗口内的匹配；未命中不移动游标。
// 没有任何词命中的句子由相邻已知句按长度比例补全，而不是接在上一句的估算后面。时长取最后一个
// 真实词边界的结束，绝不使用外推值。
func blogAudioBuildTimings(full string, sentences []string, bounds []blogAudioBound) ([]blogAudioSentence, int64) {
	nfull := blogAudioNormalizeTiming(full)

	type span struct {
		start, end int
		ok         bool
	}
	spans := make([]span, len(sentences))
	cursor := 0
	for i, s := range sentences {
		needle := blogAudioNormalizeTiming(s)
		if len(needle) == 0 {
			continue
		}
		limit := cursor + len(needle) + 32
		if limit > len(nfull) {
			limit = len(nfull)
		}
		idx := blogAudioIndexRunes(nfull, needle, cursor, limit)
		if idx < 0 {
			// A sentence is a substring of full by construction; the wider search is only a
			// guard against an unexpected normalization edge.
			//
			// 句子按构造就是 full 的子串；放宽搜索只是对意外归一化边界的兜底。
			idx = blogAudioIndexRunes(nfull, needle, 0, len(nfull))
		}
		if idx < 0 {
			continue
		}
		spans[i] = span{start: idx, end: idx + len(needle), ok: true}
		cursor = idx + len(needle)
	}

	type word struct {
		start, end int
		o, d       int64
	}
	words := make([]word, 0, len(bounds))
	cursor = 0
	for _, b := range bounds {
		if b.T == "" {
			continue
		}
		needle := blogAudioNormalizeTiming(stdhtml.UnescapeString(b.T))
		if len(needle) == 0 {
			continue
		}
		limit := cursor + len(needle) + 160
		if limit > len(nfull) {
			limit = len(nfull)
		}
		idx := blogAudioIndexRunes(nfull, needle, cursor, limit)
		if idx < 0 {
			// A fragment of an escaped entity or a normalization difference. Skipping it
			// without moving the cursor lets the next real word land in the right place.
			//
			// 转义实体的碎片或归一化差异。跳过且不移动游标，下一个真实词仍能落回正确位置。
			continue
		}
		words = append(words, word{start: idx, end: idx + len(needle), o: b.O, d: b.D})
		cursor = idx + len(needle)
	}

	out := make([]blogAudioSentence, len(sentences))
	for i, s := range sentences {
		out[i] = blogAudioSentence{T: s}
	}
	seen := make([]bool, len(sentences))
	si := 0
	for _, w := range words {
		for si < len(spans) && (!spans[si].ok || w.start >= spans[si].end) {
			si++
		}
		if si >= len(spans) || !spans[si].ok || w.start < spans[si].start {
			continue
		}
		if !seen[si] {
			out[si].S = w.o
			seen[si] = true
		}
		out[si].E = w.o + w.d
	}

	var duration int64
	for _, w := range words {
		if e := w.o + w.d; e > duration {
			duration = e
		}
	}

	lens := make([]int, len(sentences))
	var totalLen int64
	for i, s := range sentences {
		lens[i] = utf8.RuneCountInString(s)
		totalLen += int64(lens[i])
	}

	if len(words) == 0 {
		// No usable boundary at all: fall back to a length-based estimate. duration stays 0 so
		// the caller can still publish a document; the player will simply have coarse windows.
		//
		// 完全拿不到可用边界时退回按长度估算；duration 保持 0，播放器只会得到较粗的时间窗。
		var prev int64
		for i := range out {
			dur := 1200 + int64(lens[i])*160
			out[i].S = prev
			out[i].E = prev + dur
			prev = out[i].E
		}
		return out, prev
	}

	var known []int
	for i := range seen {
		if seen[i] {
			known = append(known, i)
		}
	}
	if len(known) == 0 {
		var prev int64
		for i := range out {
			dur := 1200 + int64(lens[i])*160
			out[i].S = prev
			out[i].E = prev + dur
			prev = out[i].E
		}
		return out, prev
	}

	// fill distributes [start,end) across sentences [from,to), proportionally to their length.
	// 按长度比例把 [start,end) 分配给 [from,to) 之间的句子。
	fill := func(from, to int, start, end int64) {
		if from >= to {
			return
		}
		if end <= start {
			end = start + int64(to-from)*300
		}
		window := end - start
		var part int64
		if total := func() int64 {
			var n int64
			for i := from; i < to; i++ {
				n += int64(lens[i])
			}
			return n
		}(); total > 0 {
			part = total
		} else {
			part = int64(to - from)
		}
		cur := start
		for i := from; i < to; i++ {
			var share int64
			if part > 0 {
				share = window * int64(lens[i]) / part
			}
			if share <= 0 {
				share = window / int64(to-from)
			}
			if share <= 0 {
				share = 1
			}
			out[i].S = cur
			out[i].E = cur + share
			cur += share
		}
		out[to-1].E = end
	}

	if known[0] > 0 {
		fill(0, known[0], 0, out[known[0]].S)
	}
	for k := 0; k+1 < len(known); k++ {
		fill(known[k]+1, known[k+1], out[known[k]].E, out[known[k+1]].S)
	}

	last := known[len(known)-1]
	if last+1 < len(out) {
		rate := float64(0)
		if spanDur := out[last].E - out[last].S; spanDur > 0 && lens[last] > 0 {
			rate = float64(spanDur) / float64(lens[last])
		} else if duration > 0 && totalLen > 0 {
			rate = float64(duration) / float64(totalLen)
		}
		if rate <= 0 {
			rate = 160
		}
		cur := out[last].E
		for i := last + 1; i < len(out); i++ {
			dur := int64(float64(lens[i]) * rate)
			if dur < 300 {
				dur = 300
			}
			out[i].S = cur
			out[i].E = cur + dur
			cur = out[i].E
		}
		// Compress the tail if the estimate runs past the real audio; a window that can never
		// be reached is worse than a short one.
		//
		// 估算越过真实音频时压缩尾部；永远到不了的时间窗比短一点更糟。
		if duration > 0 && cur > duration {
			start := out[last].E
			avail := duration - start
			if avail <= 0 {
				for i := last + 1; i < len(out); i++ {
					out[i].S = duration
					out[i].E = duration + 1
				}
			} else {
				scale := float64(avail) / float64(cur-start)
				cur = start
				for i := last + 1; i < len(out); i++ {
					dur := int64(float64(out[i].E-out[i].S) * scale)
					if dur < 1 {
						dur = 1
					}
					out[i].S = cur
					out[i].E = cur + dur
					cur = out[i].E
				}
			}
		}
	}

	var prev int64
	for i := range out {
		if out[i].S < prev {
			out[i].S = prev
		}
		if out[i].E <= out[i].S {
			out[i].E = out[i].S + 300
		}
		prev = out[i].E
	}
	if duration <= 0 {
		duration = prev
	}
	if len(out) > 0 && out[len(out)-1].E < duration {
		out[len(out)-1].E = duration
	}
	return out, duration
}

// blogAudioNormalizeTiming removes whitespace. Punctuation and case are kept: they are the
// characters that make one boundary distinguishable from another.
//
// blogAudioNormalizeTiming 去掉空白。保留标点与大小写：它们正是区分相邻边界的字符。
func blogAudioNormalizeTiming(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if !unicode.IsSpace(r) {
			out = append(out, r)
		}
	}
	return out
}

// blogAudioIndexRunes finds needle in hay[from:to]. The explicit window is what stops a common
// short token from matching far ahead of the spoken position.
//
// blogAudioIndexRunes 在 hay[from:to] 中查找 needle。显式窗口阻止常见短词在朗读位置之前很远
// 的地方命中。
func blogAudioIndexRunes(hay, needle []rune, from, to int) int {
	if from < 0 {
		from = 0
	}
	if to > len(hay) {
		to = len(hay)
	}
	if from >= to || len(needle) == 0 || len(needle) > to-from {
		return -1
	}
	first := needle[0]
	for i := from; i <= to-len(needle); i++ {
		if hay[i] != first {
			continue
		}
		ok := true
		for j := 1; j < len(needle); j++ {
			if hay[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func blogAudioTextHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func blogAudioHashPrefix(h string) string {
	if len(h) > 10 {
		return h[:10]
	}
	return h
}
