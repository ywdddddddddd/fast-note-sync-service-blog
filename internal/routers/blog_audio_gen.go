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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
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
		case blogAudioQueued, blogAudioGenerating, blogAudioReadyStatus:
			return st.Status, st.Estimate, true
		}
	}
	select {
	case g.queue <- job:
		g.jobs[job.Slug] = &blogAudioJobState{Status: blogAudioQueued, Estimate: job.Estimate}
		return blogAudioQueued, job.Estimate, true
	default:
		return "", 0, false
	}
}

func (g *blogAudioGen) setState(slug, status string, estimate int64, errMsg string) {
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
}

func (g *blogAudioGen) worker() {
	for job := range g.queue {
		g.setState(job.Slug, blogAudioGenerating, job.Estimate, "")
		if err := g.generate(job); err != nil {
			g.setState(job.Slug, blogAudioFailed, job.Estimate, err.Error())
			continue
		}
		g.setState(job.Slug, blogAudioReadyStatus, 0, "")
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
// blogAudioExtract 与浏览器 buildArticleText 一致：h2/h3/h4/p/li 块、取自身文本（跳过子块）、
// 排除公式与代码块子树、折叠空白。
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
			tag := strings.ToLower(n.Data)
			if tag == "h2" || tag == "h3" || tag == "h4" || tag == "p" || tag == "li" {
				if blogAudioClassSkip(n) {
					return
				}
				text := strings.Join(strings.Fields(blogAudioOwnText(n)), " ")
				if text != "" {
					blocks = append(blocks, text)
				}
				return
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

func blogAudioSkipSubtree(n *html.Node) bool {
	switch strings.ToLower(n.Data) {
	case "pre", "code", "table", "figure":
		return true
	}
	for _, cls := range strings.Fields(blogAudioClass(n)) {
		switch cls {
		case "quiz", "fns-quiz", "fns-graph", "fns-audio-ignore":
			return true
		}
	}
	return false
}

func blogAudioClassSkip(n *html.Node) bool {
	for _, cls := range strings.Fields(blogAudioClass(n)) {
		switch cls {
		case "math", "katex", "fns-audio-ignore":
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
			if blogAudioClassSkip(c) {
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

// blogAudioBuildTimings is the Go counterpart of the Node mapper: place each edge-tts word
// boundary in the source text, then take each sentence's first/last word. Missing boundaries
// are interpolated monotonically rather than dropped, so the player never gets a sentence with
// no time window.
//
// blogAudioBuildTimings 是 Node 映射器的 Go 版本：把每个 edge-tts 词边界落回源文本，再取每句的
// 首/末词。缺失的边界按单调时间插值而不是丢弃，播放器因此不会得到没有时间窗的句子。
func blogAudioBuildTimings(full string, sentences []string, bounds []blogAudioBound) ([]blogAudioSentence, int64) {
	type word struct {
		start, end int
		o, d       int64
	}
	var words []word
	cursor := 0
	for _, b := range bounds {
		if b.T == "" {
			continue
		}
		needle := b.T
		idx := strings.Index(full[cursor:], needle)
		if idx < 0 {
			stripped := strings.Join(strings.Fields(b.T), "")
			if stripped != "" {
				if j := strings.Index(full[cursor:], stripped); j >= 0 {
					idx = j
					needle = stripped
				}
			}
		}
		if idx < 0 {
			continue
		}
		start := cursor + idx
		words = append(words, word{start: start, end: start + len(needle), o: b.O, d: b.D})
		cursor = start + len(needle)
	}

	type span struct {
		start, end int
		ok         bool
	}
	spans := make([]span, len(sentences))
	cursor = 0
	for i, s := range sentences {
		idx := strings.Index(full[cursor:], s)
		if idx < 0 {
			idx = strings.Index(full, s)
		}
		if idx < 0 {
			continue
		}
		start := cursor + idx
		spans[i] = span{start: start, end: start + len(s), ok: true}
		cursor = start + len(s)
	}

	out := make([]blogAudioSentence, len(sentences))
	for i, s := range sentences {
		out[i] = blogAudioSentence{T: s}
	}
	for i := range sentences {
		if !spans[i].ok {
			continue
		}
		var first, last int64 = -1, -1
		for _, w := range words {
			if w.end <= spans[i].start {
				continue
			}
			if w.start >= spans[i].end {
				break
			}
			if first < 0 {
				first = w.o
			}
			last = w.o + w.d
		}
		if first >= 0 {
			out[i].S = first
			out[i].E = last
		}
	}

	var prev int64
	for i := range out {
		if out[i].S == 0 && out[i].E == 0 {
			out[i].S = prev
			out[i].E = prev + 1200 + int64(utf8.RuneCountInString(out[i].T))*160
		}
		if out[i].S < prev {
			out[i].S = prev
		}
		if out[i].E <= out[i].S {
			out[i].E = out[i].S + 800
		}
		prev = out[i].E
	}
	return out, prev
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
