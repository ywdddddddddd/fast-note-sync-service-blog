package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"go.uber.org/zap"

	"github.com/google/uuid"

	"github.com/haierkeys/fast-note-sync-service/pkg/util"
)

// Blog retrieval-augmented Q&A.
//
// Pipeline: tokenize the question → score every published note against those tokens → pull in
// one hop of wiki-link neighbours for the best hits → cut the winning notes into excerpts →
// ask an upstream chat model to answer using only those excerpts, citing them by index.
//
// Everything runs against the *published* corpus only. The blog is a public surface, so the
// answer must never quote an unpublished note; that restriction is enforced at corpus build
// time rather than in the prompt, so a prompt injection cannot widen the scope.
//
// 博客检索增强问答。
//
// 流程：对问题分词 → 用这些词给每篇已发布笔记打分 → 把最佳命中笔记的一跳 wiki 链接邻居拉进来
// → 将入选笔记切成片段 → 交给上游对话模型，要求它仅依据这些片段作答并按下标引用。
//
// 全程只针对「已发布」语料。博客是公开界面，回答绝不能引用未发布的笔记；该限制在语料构建阶段
// 强制执行，而不是写进提示词，因此提示词注入无法扩大检索范围。

// ── Public types ─────────────────────────────────────────────────────────────

// AskRoute one upstream endpoint the service may call.
// AskRoute 服务可调用的一条上游线路。
type AskRoute struct {
	Name        string
	BaseURL     string
	APIKey      string
	Model       string
	MinInterval time.Duration
	Timeout     time.Duration

	// Decoding parameters; nil means "leave it to the upstream".
	// 解码参数；nil 表示交由上游默认。
	Temperature      *float64
	TopP             *float64
	TopK             *int
	PresencePenalty  *float64
	FrequencyPenalty *float64
	MinP             *float64

	MaxTokens      int
	EnableThinking bool
	Retries        int

	// ThinkingLevel overrides the on/off boolean for upstreams that take a thinking object.
	// Empty means "derive from EnableThinking". See config.AIRouteSetting.ThinkingLevel.
	//
	// ThinkingLevel 为使用 thinking 对象的上游覆盖布尔开关。
	// 留空表示由 EnableThinking 推导。详见 config.AIRouteSetting.ThinkingLevel。
	ThinkingLevel string

	// Headers are extra request headers. Values may contain ${uuid}, replaced per request.
	// Headers 是额外的请求头。值中的 ${uuid} 每次请求替换为新生成的 UUIDv4。
	Headers map[string]string

	// ExtraBody is merged into the request JSON after every built-in key.
	// See config.AIRouteSetting.ExtraBody for why this exists and who wins on conflicts.
	//
	// ExtraBody 在所有内置键之后合并进请求 JSON。
	// 存在理由与冲突规则见 config.AIRouteSetting.ExtraBody。
	ExtraBody map[string]any
}

// AskOptions everything a single question needs. Assembled by the router layer from config, so
// the service stays free of a config-package dependency.
// AskOptions 单个问题所需的全部输入。由路由层从配置组装，使服务层不依赖 config 包。
type AskOptions struct {
	UID              int64
	SiteName         string
	Question         string
	Routes           []AskRoute
	TopK             int
	MaxContextChars  int
	CacheTTL         time.Duration
	SystemPrompt     string
	MaxQuestionChars int

	// Embedding and Rerank are optional. When either is unset the pipeline degrades to the
	// channel that is available, ending at BM25 alone — a partially configured deployment
	// must still answer questions, just with weaker recall.
	// Embedding 与 Rerank 是可选的。任一未配置时管线退化为可用的那条通道，
	// 最差退到纯 BM25——配置不完整的部署仍须能回答问题，只是召回弱一些。
	Embedding *AskEmbeddingConfig
	Rerank    *AskRerankConfig
}

// AskEmbeddingConfig carries the vector channel's settings.
// AskEmbeddingConfig 承载向量通道的配置。
type AskEmbeddingConfig struct {
	BaseURL        string
	APIKey         string
	Model          string
	BatchSize      int
	ChunkChars     int
	ChunkOverlap   int
	CandidateLimit int
}

// AskRerankConfig carries the reranker's settings.
// AskRerankConfig 承载重排器的配置。
type AskRerankConfig struct {
	BaseURL  string
	APIKey   string
	Model    string
	TopN     int
	DocChars int
}

// AskEvent is one Server-Sent Event frame. Type drives how the browser renders it.
// AskEvent 一帧 Server-Sent Event，Type 决定浏览器如何渲染。
type AskEvent struct {
	Type      string         `json:"type"`                // retrieval | delta | citations | done | error
	Query     string         `json:"query,omitempty"`     // echoed question
	Terms     []string       `json:"terms,omitempty"`     // tokens actually used
	Stage     string         `json:"stage,omitempty"`     // human-readable progress line
	// Icon names the pictogram for this step. Derived here rather than in the browser:
	// matching Chinese keywords to icons breaks the moment a line is reworded, and it did
	// ("翻遍了 147 篇…" contains no 检索, so it silently fell back to the generic icon).
	// Icon 指定这一步的图标。由服务端给出而不是浏览器猜：用中文关键词匹配图标，
	// 文案一改就失灵，事实上已经失灵过——「翻遍了 147 篇…」不含「检索」，于是落到了通用图标。
	Icon      string         `json:"icon,omitempty"`
	Sources   []*AskSource   `json:"sources,omitempty"`   // retrieved notes
	Text      string         `json:"text,omitempty"`      // streamed answer fragment
	// ThoughtChars is the running count of reasoning characters seen so far; it lets the
	// browser show progress during a long thinking phase without the server forwarding every
	// reasoning token twice.
	// ThoughtChars 是累计到目前收到的推理字符数；它让浏览器在漫长思考阶段显示进度，
	// 而服务端无需把每个推理 token 再转一遍。
	ThoughtChars int            `json:"thoughtChars,omitempty"`
	Citations    []*AskCitation `json:"citations,omitempty"` // cited notes, in display order
	Model     string         `json:"model,omitempty"`     // upstream model that answered
	Route     string         `json:"route,omitempty"`     // upstream route name
	ElapsedMS int64          `json:"elapsedMs,omitempty"`
	Message   string         `json:"message,omitempty"` // error text
}

// AskSource one retrieved note, shown while the answer streams.
// AskSource 一篇被检索到的笔记，在回答流式输出期间展示。
type AskSource struct {
	ID      int64   `json:"id"`
	Title   string  `json:"title"`
	Slug    string  `json:"slug"`
	URL     string  `json:"url"`
	Score   float64 `json:"score"`
	Via     string  `json:"via"`     // why it was retrieved
	Excerpt string  `json:"excerpt"` // snippet sent to the model
}

// AskCitation one note referenced by the answer.
// AskCitation 被回答引用的一篇笔记。
type AskCitation struct {
	Index   int    `json:"index"` // 1-based, matches [n] in the answer text
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Slug    string `json:"slug"`
	URL     string `json:"url"`
	Excerpt string `json:"excerpt"`
}

// ── Corpus ───────────────────────────────────────────────────────────────────

// askNote is one published note in the retrieval corpus.
// askNote 检索语料中的一篇已发布笔记。
type askNote struct {
	ID      int64
	VaultID int64
	Path    string
	Hash    string
	Title   string
	Slug    string
	Plain   string // markdown-stripped body, used for matching and excerpting
	// ContentHash is the note's own content digest. It feeds the vector index fingerprint, so
	// an edit that changes the wording without changing the length still invalidates the
	// cached vectors. Length alone would not: a one-character fix and its typo have the same
	// length and would keep serving vectors of the old text.
	//
	// ContentHash 是笔记自身的内容摘要。它参与向量索引的指纹，使「改动了措辞但长度不变」的
	// 编辑同样能让缓存向量失效。只看长度做不到这一点：修正一个字与它的笔误长度相同，
	// 于是会继续使用旧文本的向量。
	ContentHash string
	Sort        float64
}

type askCorpus struct {
	notes  []*askNote
	byHash map[string]*askNote
	byID   map[int64]*askNote
	// activeIDs is the authoritative set of shared note ids this corpus was built from.
	// Revalidation compares it against a fresh read of the same query, so both sides always
	// use one definition of "shared". Deriving it from notes instead would disagree forever
	// with the live query whenever a share points at a note that no longer exists: the live
	// list includes such an id, the corpus drops it, and every question would rebuild.
	//
	// activeIDs 是本语料构建时的权威分享 id 集合。校验时与同一条查询的新鲜结果比对，
	// 因此两侧始终使用同一个「已分享」定义。若改从 notes 推导，只要存在指向已删除笔记的
	// 分享就会永远对不上：实时列表包含该 id、语料丢弃它，于是每次提问都重建。
	activeIDs []int64
	built     time.Time
}

// sameIDSet reports whether two id lists contain exactly the same ids, ignoring order.
// sameIDSet 判断两个 id 列表是否完全同集，忽略顺序。
func sameIDSet(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	seen := make(map[int64]int, len(a))
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		n := seen[x]
		if n == 0 {
			return false
		}
		seen[x] = n - 1
	}
	return true
}

// askRuntime holds per-service caches and pacing state.
// askRuntime 持有服务级的缓存与节流状态。
type askRuntime struct {
	mu       sync.Mutex
	corpus   map[int64]*askCorpus
	lastRun  map[string]time.Time // route key -> last call start
	cooldown map[string]time.Time // route key -> not before
}

// askRouteCooldown is how long a route is skipped after a failure.
//
// Without it, a route that is permanently broken (wrong key, exhausted quota, model not
// provisioned) is retried on every single question, and because the preferred route is tried
// first, every question pays its full round-trip before falling through. Measured against a
// 403-ing endpoint that was 1–6 s of dead time per question.
//
// askRouteCooldown 是线路失败后被跳过的时长。
//
// 没有它，一条永久故障的线路（密钥错误、额度耗尽、模型未开通）会在每次提问时被重试，
// 而首选线路又总是先试，于是每个问题都要先付掉它的整个往返。实测对一个持续返回 403 的
// 端点，这是每次提问 1–6 秒的纯浪费。
const askRouteCooldown = 120 * time.Second

func (s *shareService) askRT() *askRuntime {
	s.askMu.Lock()
	defer s.askMu.Unlock()
	if s.askRuntime == nil {
		s.askRuntime = &askRuntime{
			corpus:   make(map[int64]*askCorpus),
			lastRun:  make(map[string]time.Time),
			cooldown: make(map[string]time.Time),
		}
	}
	return s.askRuntime
}

// askRouteKey identifies a route for pacing and cooldown bookkeeping.
// askRouteKey 为节流与冷却记账标识一条线路。
func askRouteKey(r AskRoute) string {
	if r.Name != "" {
		return r.Name
	}
	return r.BaseURL + "|" + r.Model
}

func (s *shareService) askRouteCooling(key string) bool {
	rt := s.askRT()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	until, ok := rt.cooldown[key]
	return ok && time.Now().Before(until)
}

func (s *shareService) askCoolRoute(key string) {
	rt := s.askRT()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.cooldown[key] = time.Now().Add(askRouteCooldown)
}

func (s *shareService) askClearCooling(key string) {
	rt := s.askRT()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.cooldown, key)
}

// publishedCorpus builds (or reuses) the searchable set of published notes.
//
// Only shares that are still usable and point at a live note are included — the same rule the
// blog index applies, so the Q&A entry and the article list can never disagree about what is
// public.
//
// publishedCorpus 构建（或复用）可检索的已发布笔记集合。
//
// 只纳入仍然有效、且指向存活笔记的分享——与博客首页使用同一套规则，
// 因此问答入口与文章列表对「什么算公开」的判断永远一致。
func (s *shareService) publishedCorpus(ctx context.Context, uid int64, ttl time.Duration) (*askCorpus, error) {
	rt := s.askRT()

	// A cached corpus is only reusable while the set of shared notes is unchanged.
	//
	// The TTL alone is not safe here. Revoking a share does not touch updated_at — every
	// share update is a single-column `Update(field, value)`, which gorm's timestamp hook does
	// not fire on — so no "changed since" query can notice it. Serving the stale corpus for up
	// to five minutes would let Q&A quote a note the owner has just unpublished, which is a
	// privacy failure, not a caching annoyance.
	//
	// So the cache is revalidated against the authoritative id list on every question. That
	// query reads one table and returns ~145 ids; measured cost is a couple of milliseconds
	// against an end-to-end answer of about two seconds.
	//
	// 缓存的语料只有在「已分享笔记集合未变」时才可复用。
	//
	// 单靠 TTL 在这里是不够的。取消分享走的是单列 `Update(field, value)`，gorm 的时间戳钩子
	// 不会触发，因此没有任何「自某时刻起变更」的查询能发现它。若继续用最长五分钟的陈旧语料，
	// 问答就可能引用博主刚刚取消发布的笔记——那是隐私缺陷，不是缓存不便。
	//
	// 因此每次提问都用权威 id 列表校验缓存。该查询只读一张表、返回约 145 个 id；
	// 实测代价是几毫秒，而端到端回答约两秒。
	if ttl > 0 {
		rt.mu.Lock()
		cached, ok := rt.corpus[uid]
		fresh := ok && time.Since(cached.built) < ttl
		rt.mu.Unlock()

		if fresh {
			cur, err := s.repo.ListActiveNoteResIDs(ctx, uid)
			if err == nil && sameIDSet(cached.activeIDs, cur) {
				return cached, nil
			}
			// Either the set changed or the check failed; rebuilding is the safe branch in
			// both cases, because a stale corpus is the only outcome that leaks.
			// 集合变了或校验失败——两种情形都重建，因为「陈旧语料」是这里唯一会泄漏的结果。
		}
	}

	shares, err := s.repo.ListByUID(ctx, uid, "created_at", "desc", 0, 2000)
	if err != nil {
		return nil, err
	}

	c := &askCorpus{
		notes:  make([]*askNote, 0, len(shares)),
		byHash: make(map[string]*askNote),
		byID:   make(map[int64]*askNote),
		built:  time.Now(),
	}
	seen := make(map[string]struct{})

	for _, sh := range shares {
		if !s.isShareUsable(sh) || sh.ResType != "note" {
			continue
		}
		note, err := s.noteRepo.GetByID(ctx, sh.ResID, uid)
		if err != nil || note == nil || note.Action == "delete" {
			continue
		}
		if _, dup := seen[note.Path]; dup {
			continue
		}
		seen[note.Path] = struct{}{}

		n := &askNote{
			ID:          note.ID,
			VaultID:     note.VaultID,
			Path:        note.Path,
			Hash:        note.PathHash,
			Title:       strings.TrimSuffix(pathBase(note.Path), ".md"),
			Slug:        s.BuildLocalShortLink(uid, sh.ID),
			Plain:       blogPlainText(note.Content, 0),
			ContentHash: note.ContentHash,
			Sort:        float64(firstNonZero(note.UpdatedTimestamp, note.Mtime, note.Ctime)),
		}
		c.notes = append(c.notes, n)
		if n.Hash != "" {
			c.byHash[n.Hash] = n
		}
		// The link table stores whatever the author typed, and Obsidian's wiki syntax is
		// usually `[[note]]` or `[[folder/note]]` — neither carries the .md extension, and
		// neither equals note.Path. Indexing only n.Hash therefore made askExpandLinks a
		// no-op: every neighbour lookup missed. Measured on this vault, the whole-corpus
		// variant of the same defect resolved 13 of 4666 link rows.
		//
		// 链接表存的是作者原样敲下的文本，而 Obsidian 的 wiki 语法通常是 `[[笔记]]` 或
		// `[[文件夹/笔记]]`——两者都不带 .md，也不等于 note.Path。只索引 n.Hash 使
		// askExpandLinks 形同虚设：每次邻居查找都落空。实测同一缺陷在全站图谱上
		// 只让 4666 条链接中的 13 条命中。
		for _, variation := range util.GeneratePathVariations(n.Path) {
			h := util.EncodeHash32(variation)
			if h == "" {
				continue
			}
			if _, exists := c.byHash[h]; !exists {
				c.byHash[h] = n
			}
		}
		c.byID[n.ID] = n
	}

	// Record the authoritative shared set so the next question can revalidate cheaply. If the
	// read fails the corpus is still returned — a question must not fail over a cache-check
	// query — but it is not cached, because caching a corpus without its signature would make
	// every later revalidation fail and rebuild on each question.
	//
	// 记录权威分享集合，供下次提问低成本校验。该读取失败时仍返回语料——提问不该因为一次
	// 缓存校验查询而失败——但不写入缓存：没有签名的缓存会让后续每次校验都失败，
	// 于是每次提问都重建。
	sigOK := false
	if ids, err := s.repo.ListActiveNoteResIDs(ctx, uid); err == nil {
		c.activeIDs = ids
		sigOK = true
	}

	if ttl > 0 && sigOK {
		rt.mu.Lock()
		rt.corpus[uid] = c
		rt.mu.Unlock()
	}
	return c, nil
}

// pathBase is filepath.Base without importing filepath just for one call site.
// pathBase 等价于 filepath.Base，避免为一个调用点引入 filepath。
func pathBase(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// ── Tokenizing ───────────────────────────────────────────────────────────────

// askStopwords are dropped from the query. They are grammatical particles plus the words that
// appear in almost every Chinese question, and keeping them would make every note look relevant.
// askStopwords 会从查询中剔除。它们是虚词以及几乎出现在每个中文问句里的词，
// 保留它们会让所有笔记看起来都相关。
var askStopwords = map[string]struct{}{
	"的": {}, "了": {}, "在": {}, "是": {}, "我": {}, "有": {}, "和": {}, "就": {}, "不": {},
	"人": {}, "都": {}, "一": {}, "上": {}, "也": {}, "很": {}, "到": {}, "说": {}, "要": {},
	"去": {}, "你": {}, "会": {}, "着": {}, "看": {}, "好": {}, "这": {}, "那": {}, "与": {},
	"及": {}, "或": {}, "被": {}, "把": {}, "让": {}, "从": {}, "对": {}, "为": {}, "以": {},
	"什么": {}, "怎么": {}, "怎样": {}, "如何": {}, "为什么": {}, "为何": {}, "哪里": {}, "哪些": {},
	"哪个": {}, "多少": {}, "吗": {}, "呢": {}, "吧": {}, "啊": {}, "请": {}, "告诉": {}, "一下": {},
	"介绍": {}, "解释": {}, "说明": {}, "关于": {}, "以及": {}, "还有": {}, "可以": {}, "能否": {},
	"是否": {}, "区别": {}, "关系": {}, "意思": {}, "内容": {}, "笔记": {}, "the": {}, "a": {},
	"an": {}, "is": {}, "are": {}, "of": {}, "to": {}, "and": {}, "what": {}, "how": {},
	"why": {}, "where": {}, "which": {}, "please": {}, "tell": {}, "me": {}, "about": {},
}

// askTokenize splits a question into search tokens.
//
// Chinese has no spaces, so every CJK run of length >= 2 is expanded into overlapping
// bigrams ("牙体解剖" -> 牙体/体解/解剖). Bigrams are the sweet spot here: single characters
// match far too much, while whole runs demand an exact phrase and miss paraphrases.
//
// askTokenize 把问题切成检索词。
//
// 中文没有空格，因此长度 >= 2 的连续汉字串会展开成重叠二元组（"牙体解剖" → 牙体/体解/解剖）。
// 二元组是这里的平衡点：单字命中过泛，而整串要求精确短语、会漏掉同义改写。
func askTokenize(q string) []string {
	var tokens []string
	seen := make(map[string]struct{})
	add := func(t string) {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			return
		}
		if _, bad := askStopwords[t]; bad {
			return
		}
		if _, dup := seen[t]; dup {
			return
		}
		seen[t] = struct{}{}
		tokens = append(tokens, t)
	}

	var cjk []rune
	var latin []rune
	flushCJK := func() {
		if len(cjk) == 1 {
			add(string(cjk))
		} else if len(cjk) >= 2 {
			for i := 0; i+2 <= len(cjk); i++ {
				add(string(cjk[i : i+2]))
			}
			// Trigrams exist to separate near-identical compounds. 上颌 and 下颌 differ in a
			// single bigram and share every other one, so a bigram-only index cannot tell
			// 《4、上颌第一磨牙》 from 《5、下颌第一磨牙》 — measured: asking for the former
			// ranked the latter first. A trigram ("上颌第" vs "下颌第") carries the
			// distinction, and blog_ask_rank.go weights longer tokens higher.
			//
			// 三元组的用途是区分高度相似的词。上颌与下颌只差一个 bigram，其余完全相同，
			// 因此纯 bigram 索引无法区分《4、上颌第一磨牙》与《5、下颌第一磨牙》——
			// 实测问前者时后者排到了第一。三元组（「上颌第」对「下颌第」）承载了这个
			// 差异，blog_ask_rank.go 会给更长的检索词更高权重。
			if len(cjk) >= 3 {
				for i := 0; i+3 <= len(cjk); i++ {
					add(string(cjk[i : i+3]))
				}
			}
			// The full run carries more signal than any single bigram, so keep it too.
			if len(cjk) <= 8 {
				add(string(cjk))
			}
		}
		cjk = cjk[:0]
	}
	flushLatin := func() {
		if len(latin) >= 2 {
			add(string(latin))
		}
		latin = latin[:0]
	}

	for _, r := range q {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			latin = append(latin, unicode.ToLower(r))
		default:
			flushCJK()
			flushLatin()
		}
	}
	flushCJK()
	flushLatin()
	return tokens
}

// ── Retrieval ────────────────────────────────────────────────────────────────

type askHit struct {
	note    *askNote
	score   float64
	matched []string
	via     string
}

// askRetrieve now lives in blog_ask_rank.go, which scores with BM25 instead of raw substring
// counts. See that file for why the previous formula ranked an entire folder above the note
// that actually matched the question.
// askRetrieve 现在位于 blog_ask_rank.go，改用 BM25 打分而不是裸子串计数。
// 旧公式为何会把整个目录排到真正匹配的笔记之前，见该文件的说明。

// askExpandLinks pulls in one hop of wiki-link neighbours around the top hits.
//
// This is what makes a question land on a note that never repeats the question's wording:
// the neighbour is admitted because the note actually links to it, an editorial signal the
// text match alone cannot see. Neighbours enter at a fraction of the parent's score so they
// never outrank a direct hit.
//
// askExpandLinks 为最佳命中笔记拉入一跳 wiki 链接邻居。
//
// 这正是让问题能落到「从未复述问题措辞」的笔记上的原因：邻居之所以入选，是因为笔记确实链接到它，
// 这是纯文本匹配看不到的编辑信号。邻居以父节点得分的若干分之一进入，因此永远不会盖过直接命中。
func askExpandLinks(ctx context.Context, s *shareService, uid int64, c *askCorpus, hits []*askHit, limit int) []*askHit {
	if s.noteLinkRepo == nil || len(hits) == 0 {
		return hits
	}
	present := make(map[int64]int, len(hits))
	for i, h := range hits {
		present[h.note.ID] = i
	}
	var extra []*askHit

	for _, h := range hits {
		base := h.note

		if out, err := s.noteLinkRepo.GetOutlinks(ctx, base.ID, uid); err == nil {
			for _, l := range out {
				nb, ok := c.byHash[l.TargetPathHash]
				if !ok || nb.ID == base.ID {
					continue
				}
				if _, dup := present[nb.ID]; dup {
					continue
				}
				present[nb.ID] = -1
				extra = append(extra, &askHit{
					note:  nb,
					score: h.score * 0.45,
					via:   "出链自《" + base.Title + "》",
				})
			}
		}

		if base.Hash != "" {
			if in, err := s.noteLinkRepo.GetBacklinks(ctx, base.Hash, base.VaultID, uid); err == nil {
				for _, l := range in {
					nb, ok := c.byID[l.SourceNoteID]
					if !ok || nb.ID == base.ID {
						continue
					}
					if _, dup := present[nb.ID]; dup {
						continue
					}
					present[nb.ID] = -1
					extra = append(extra, &askHit{
						note:  nb,
						score: h.score * 0.4,
						via:   "入链指向《" + base.Title + "》",
					})
				}
			}
		}
	}

	sort.SliceStable(extra, func(i, j int) bool { return extra[i].score > extra[j].score })
	if len(extra) > limit {
		extra = extra[:limit]
	}
	return append(hits, extra...)
}

// askExcerpt cuts a readable window around the first token hit.
//
// The first 200 characters of a note are frequently frontmatter, a title repeat or a table of
// contents, so anchoring the window on the matched term is what makes the excerpt actually
// answer the question.
//
// askExcerpt 在检索词命中处切出若干可读窗口。
//
// 笔记开头 200 字往往是 frontmatter、标题重复或目录，因此把窗口锚定在命中词上，
// 摘录内容才真的与问题相关。
//
// 为什么是「若干窗口」而不是一个：旧实现只取**最靠前**的那个命中点
// （`if best < 0 || idx < best`），然后截 1200 字。一篇两万字的笔记里，
// 同一个词往往散落在多个章节，取第一个等于赌"答案在开头"。
//
// 2026-10-28 实测的翻车：问「拔牙的并发症有哪些」，《第4章：牙及牙槽外科》21153 字，
// 「并发症」出现在适应证章节（讲什么情况下不宜拔牙），而真正的并发症正文在
// 2321 / 6603 / 21001 字三处。「出血」在 2321，「干槽症」在 6603。
// 旧算法锚在最靠前的命中，窗口落在适应证那一段，模型只看到一句
// 「拔牙造成菌血症发生率 50%~80%」，于是如实回答"其余没有相关内容" ——
// 它没说错，是我们只给了它那一小段。
//
// 现在收集全部命中点，按"窗口内命中数"排序挑选，窗口之间不重叠，
// 直到用完预算。多个不连续的窗口之间用「…」连接，让模型知道这里有省略。
//
// askExcerpt returns several non-overlapping windows anchored on query terms, instead of one
// window at the earliest hit. Spreading the budget across a long note is what lets the model
// see that the same topic is discussed in more than one section.
func askExcerpt(plain string, tokens []string, budget int) string {
	if plain == "" {
		return ""
	}
	runes := []rune(plain)
	if budget <= 0 {
		budget = 1200
	}
	if len(runes) <= budget {
		return plain
	}

	lower := []rune(strings.ToLower(plain))

	// 每个检索词的所有命中位置。上限 24 个/词，避免极短词（如「的」被误当关键词时）
	// 在长文里产生成百上千个候选窗口，把选窗这一步拖慢。
	const perTokenCap = 24
	var hits []int
	for _, t := range tokens {
		tr := []rune(t)
		if len(tr) == 0 {
			continue
		}
		found := 0
		for i := 0; i+len(tr) <= len(lower) && found < perTokenCap; i++ {
			if lower[i] == tr[0] && runeIndex(lower[i:], tr) == 0 {
				hits = append(hits, i)
				found++
				i += len(tr) - 1
			}
		}
	}
	if len(hits) == 0 {
		return string(runes[:budget]) + "…"
	}

	// 候选窗口以每个命中点为中心。窗口宽度按「预算 / 期望覆盖的窗口数」推算，
	// 而不是固定比例 —— 固定比例会让 3 个窗口就吃满预算，第 4 个命中永远进不来。
	//
	// wantWins 是希望在预算内铺开的窗口数：3 个起步，命中多时最多 5 个。
	// 下限 300 字：再窄就切断了上下文，读起来是断句而不是段落。
	wantWins := 3
	if len(hits) >= 6 {
		wantWins = 5
	} else if len(hits) >= 4 {
		wantWins = 4
	}
	// 每个窗口之间留一点余量给省略标记与重叠损失。
	share := budget/wantWins - 20
	if share < 300 {
		share = 300
	}
	if share > budget {
		share = budget
	}

	type cand struct {
		start, end int
		score      int
	}
	cands := make([]cand, 0, len(hits))
	for _, h := range hits {
		// 命中点落在窗口偏前（1/5 处）而不是居中：一个孤立的命中（例如只出现一次的
		// "菌血症"）前面通常就是它的上下文定义句，后面则是一长段无关内容。
		// 居中对齐会让这种单点命中分到一半无用的尾巴。
		start := h - share/5
		if start < 0 {
			start = 0
		}
		end := start + share
		if end > len(runes) {
			end = len(runes)
			start = end - share
			if start < 0 {
				start = 0
			}
		}
		// 窗口内命中次数：密度高的窗口更可能是一整节相关内容，而不是顺带提一句。
		score := 0
		for _, h2 := range hits {
			if h2 >= start && h2 < end {
				score++
			}
		}
		cands = append(cands, cand{start: start, end: end, score: score})
	}
	// 选取：先把命中点按位置聚类，保证**每个区段都分到一个窗口**，
	// 余下的预算才用来看谁的命中更密集。
	//
	// 为什么必须这样做：一篇长笔记里，同一个词常常在两处含义完全不同
	// （「并发症」在适应证章节是"什么情况不宜拔牙"，在并发症章节才是答案）。
	// 纯按密度挑，会把预算全给到出现次数多的那一节，开头那句概述反而落选 ——
	// 而那句往往是最短、最直接的答案。
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].start < cands[j].start })

	// 相邻候选窗口的起点相距超过 share 就算另一个区段。
	var picked []cand
	used := 0
	pushWin := func(c cand) bool {
		w := c.end - c.start
		// 预留省略标记与分隔符的空间（"…" + "\n…\n"，约 8 字）。
		if w <= 0 || used+w+8 > budget {
			return false
		}
		for _, p := range picked {
			if c.start < p.end && p.start < c.end {
				return false
			}
		}
		picked = append(picked, c)
		used += w + 8
		return true
	}

	// 第一轮：沿文档推进，每个"新区段"先拿一个窗口。
	lastEnd := -1
	for _, c := range cands {
		if c.start < lastEnd {
			continue
		}
		if pushWin(c) {
			lastEnd = c.end
		}
	}
	// 第二轮：预算还有剩，按命中密度补窗口。
	for _, c := range cands {
		if used >= budget*9/10 {
			break
		}
		pushWin(c)
	}
	if len(picked) == 0 {
		return string(runes[:budget]) + "…"
	}
	if len(picked) == 0 {
		return string(runes[:budget]) + "…"
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].start < picked[j].start })

	var b strings.Builder
	for i, p := range picked {
		if i > 0 {
			b.WriteString("\n…\n")
		}
		if p.start > 0 {
			b.WriteString("…")
		}
		b.WriteString(string(runes[p.start:p.end]))
		if p.end < len(runes) {
			b.WriteString("…")
		}
	}
	return b.String()
}

func runeIndex(haystack, needle []rune) int {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return -1
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		ok := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
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

// ── Prompt assembly ──────────────────────────────────────────────────────────

type upstreamMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const defaultAskSystemPrompt = `你是这个博客的知识库助手。你只能依据「资料片段」中的内容作答。

规则：
1. 用简体中文回答，条理清晰；能用要点就用要点，不要寒暄、不要复述问题。
2. 每个来自资料的结论后面必须标注来源编号，例如 [1] 或 [2][3]，编号对应资料片段的编号。
3. 资料里没有提到的内容，直接回答「已发布的笔记里没有相关内容」，不要凭常识补全或编造。
4. 不要输出资料片段以外的链接或文件名。
5. 如果资料之间说法不一致，指出分歧并分别标注来源。`

// askBuildMessages assembles the system + user turns.
// askBuildMessages 组装 system 与 user 两条消息。
func askBuildMessages(opts AskOptions, hits []*askHit, excerpts map[int64]string) []upstreamMessage {
	sys := strings.TrimSpace(opts.SystemPrompt)
	if sys == "" {
		sys = defaultAskSystemPrompt
	}

	var b strings.Builder
	fmt.Fprintf(&b, "【资料片段】共 %d 条\n\n", len(hits))
	for i, h := range hits {
		fmt.Fprintf(&b, "[%d] 《%s》\n%s\n\n", i+1, h.note.Title, excerpts[h.note.ID])
	}
	fmt.Fprintf(&b, "【问题】\n%s\n\n请依据上述资料回答，并按规则标注来源编号。", opts.Question)

	return []upstreamMessage{
		{Role: "system", Content: sys},
		{Role: "user", Content: b.String()},
	}
}

// ── Upstream streaming ───────────────────────────────────────────────────────

type upstreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
			// Reasoning models stream their reasoning phase here, before any answer token.
			// 推理模型在首个回答 token 之前，把推理阶段的内容流在这里。
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// askCallRoute streams one completion, invoking onDelta for every answer fragment and
// onReasoning for every reasoning fragment.
//
// onDelta returns false to stop early (e.g. the client hung up). A route that fails before the
// first fragment is reported with started=false so the caller may fall back to the next route;
// once any text has been emitted the failure can no longer be papered over.
//
// askCallRoute 流式请求一次补全，每收到一个回答片段调用 onDelta，每收到一个推理片段调用
// onReasoning。
//
// onDelta 返回 false 可提前终止（例如客户端断开）。在收到首个片段之前失败的线路会以
// started=false 报回，调用方可以改用下一条线路；一旦已经输出过文本，这次失败就无法再掩盖。
// askRouteUsesThinkingObject reports whether an upstream switches reasoning through a thinking
// object instead of a boolean flag. Two families need the object today: SenseNova's DeepSeek V4
// and OpenCode. Deciding by URL brand rather than by a config flag is deliberate: sending the
// boolean to an object-only upstream fails silently (empty content, no error), and a silently
// empty answer is the worst outcome this pipeline can produce.
//
// askRouteUsesThinkingObject 判断某上游是用 thinking 对象而不是布尔开关来切换推理。
// 目前有两类需要对象：商汤的 DeepSeek V4 与 OpenCode。用 URL 品牌而不是配置开关来判断是刻意的：
// 向只认对象的上游发布尔字段会静默失败（content 为空、无报错），
// 而「静默地给出空答案」正是这条管线能造成的最坏结果。
func askRouteUsesThinkingObject(baseURL string) bool {
	u := strings.ToLower(baseURL)
	return strings.Contains(u, "sensenova") || strings.Contains(u, "opencode.ai")
}

// askRouteThinkingMode resolves the thinking object's type. ThinkingLevel wins when set, otherwise
// the boolean decides. Measured on OpenCode Go: enable_thinking is ignored outright — the model
// still emits reasoning_content and, under a small MaxTokens, returns empty content with
// finish_reason=length. That is why the object is mandatory there, not merely preferable.
//
// askRouteThinkingMode 决定 thinking 对象的 type。ThinkingLevel 有值时优先，否则由布尔值决定。
// 在 OpenCode Go 实测：enable_thinking 被完全忽略——模型照样输出 reasoning_content，
// 而 MaxTokens 不大时返回空 content 与 finish_reason=length。
// 这就是那里必须用对象、而不只是「最好用对象」的原因。
//
// 白名单是必要的：这个字段直接进请求体，上游对枚举的改动会让**整条线路**回 422，
// 而错误消息埋在几千 token 的请求尾部，从外面只能看到「所有模型线路都不可用」。
// 2026-10-26 就发生过一次：配置写着 max，上游已改为只接受 adaptive/enabled/disabled，
// 于是问答彻底不可用。这里对未知值降级并说明，而不是原样转发。
func askRouteThinkingMode(r AskRoute) string {
	if v := strings.TrimSpace(r.ThinkingLevel); v != "" {
		switch strings.ToLower(v) {
		case "adaptive", "enabled", "disabled":
			return strings.ToLower(v)
		default:
			// 未知取值：按 enable-thinking 降级，宁可少一层推理，也不要整条线路失败。
			if r.EnableThinking {
				return "enabled"
			}
			return "disabled"
		}
	}
	if r.EnableThinking {
		return "enabled"
	}
	return "disabled"
}

// askExpandHeaderValue substitutes ${uuid} with a fresh UUIDv4. Values without the placeholder come
// back unchanged, so a static header costs nothing extra.
//
// askExpandHeaderValue 把 ${uuid} 替换为新生成的 UUIDv4。不含占位符的值原样返回，
// 静态头因此没有额外开销。
func askExpandHeaderValue(v string) string {
	if !strings.Contains(v, "${uuid}") {
		return v
	}
	return strings.ReplaceAll(v, "${uuid}", uuid.NewString())
}

func (s *shareService) askCallRoute(ctx context.Context, r AskRoute, msgs []upstreamMessage,
	onDelta func(string) bool, onReasoning func(string) bool) (started bool, err error) {

	if r.Timeout <= 0 {
		r.Timeout = 300 * time.Second
	}
	maxTokens := r.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 3000
	}

	payload := map[string]any{
		"model":       r.Model,
		"messages":    msgs,
		"stream":      true,
		"max_tokens":  maxTokens,
		"temperature": 0.1,
	}
	// Only send what the route actually specifies. Sending every knob on every route is how a
	// wrong default silently overrides a provider's tuned baseline.
	//
	// 只发送线路真正指定的参数。把每个旋钮都发出去，正是错误的默认值静默覆盖供应商调优基线的方式。
	set := func(k string, v any) { payload[k] = v }
	if r.Temperature != nil {
		set("temperature", *r.Temperature)
	}
	if r.TopP != nil {
		set("top_p", *r.TopP)
	}
	if r.TopK != nil {
		set("top_k", *r.TopK)
	}
	if r.PresencePenalty != nil {
		set("presence_penalty", *r.PresencePenalty)
	}
	if r.FrequencyPenalty != nil {
		set("frequency_penalty", *r.FrequencyPenalty)
	}
	if r.MinP != nil {
		set("min_p", *r.MinP)
	}
	// enable_thinking is sent as an explicit true/false, never omitted.
	//
	// Omitting it looks harmless but is not: SiliconFlow's Qwen3.5 family ships with reasoning
	// ON by default, so "leave the field out" means "let the model think" — the opposite of what
	// a false setting is asking for. Measured with the field omitted, a 4096-token budget went
	// entirely into 5131 characters of reasoning and the answer came back empty every time.
	// Providers that do not know the field ignore it.
	//
	// enable_thinking 一律显式发送 true/false，绝不省略。
	//
	// 省略看似无害，其实不然：SiliconFlow 的 Qwen3.5 系列默认开启推理，
	// 因此「不带这个字段」等于「让模型去思考」——与 false 想要的正好相反。
	// 实测省略该字段时，4096 token 的预算被 5131 字推理全部吃掉，回答每次都是空的。
	// 不认识该字段的供应商会忽略它。
	// 商汤的 DeepSeek V4 用 thinking:{type:...} 开关思考，不认 enable_thinking；
	// 而认 enable_thinking 的那批供应商（SiliconFlow 等）不认 thinking 里的对象。
	// 所以按上游分开发，而不是两个都发——往一个只接受布尔值的同名字段塞对象，
	// 有的网关直接 400，有的默默忽略，两种都比明确分流难排查。
	//
	// SenseNova's DeepSeek V4 toggles reasoning through thinking:{type:...} and does not know
	// enable_thinking; providers that do know enable_thinking do not accept an object there.
	// Send one or the other based on the upstream rather than both.
	if askRouteUsesThinkingObject(r.BaseURL) {
		set("thinking", map[string]string{"type": askRouteThinkingMode(r)})
	} else {
		set("enable_thinking", r.EnableThinking)
	}

	// Route-specific body fields go last so an explicit config value wins over the defaults
	// above. Without this, a provider whose "thinking depth" lives in a field we never modelled
	// (or in a nested object) silently runs at whatever its own default is.
	//
	// 线路自定义字段放在最后合并，使配置值覆盖上面的默认值。
	// 否则，那些「思考深度」住在我们没建模的字段里（或嵌套对象里）的供应商，
	// 会静默地按它自己的默认值运行。
	for k, v := range r.ExtraBody {
		payload[k] = v
	}

	body, _ := json.Marshal(payload)
	url := strings.TrimRight(r.BaseURL, "/") + "/chat/completions"

	callCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	// Route-specific headers go last so a configured value wins over the defaults above.
	// 线路自定义头放在最后设置，使配置值可以覆盖上面的默认值。
	for name, value := range r.Headers {
		req.Header.Set(name, askExpandHeaderValue(value))
	}

	resp, err := askHTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return false, fmt.Errorf("upstream %s HTTP %d: %s", r.Name, resp.StatusCode,
			strings.TrimSpace(string(raw)))
	}

	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				if payload != "" && payload != "[DONE]" {
					var chunk upstreamChunk
					if json.Unmarshal([]byte(payload), &chunk) == nil {
						if chunk.Error != nil && chunk.Error.Message != "" {
							if !started {
								return false, fmt.Errorf("upstream %s: %s", r.Name, chunk.Error.Message)
							}
							return true, fmt.Errorf("upstream %s: %s", r.Name, chunk.Error.Message)
						}
						for _, ch := range chunk.Choices {
							if ch.Delta.ReasoningContent != "" && onReasoning != nil {
								if !onReasoning(ch.Delta.ReasoningContent) {
									return started, nil
								}
							}
							if ch.Delta.Content == "" {
								continue
							}
							started = true
							if !onDelta(ch.Delta.Content) {
								return true, nil
							}
						}
					}
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return started, nil
			}
			return started, readErr
		}
	}
}

// askHTTPClient is shared so keep-alive connections are reused across questions.
// askHTTPClient 共享使用，使连接在多次提问之间得以复用。
var askHTTPClient = &http.Client{
	Timeout: 0, // per-route context deadline governs
	Transport: &http.Transport{
		MaxIdleConns:        16,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	},
}

// askWaitPacing blocks until the route's minimum interval has elapsed.
// askWaitPacing 阻塞到该线路的最小间隔已过。
func (s *shareService) askWaitPacing(ctx context.Context, r AskRoute) error {
	if r.MinInterval <= 0 {
		return nil
	}
	rt := s.askRT()
	key := r.Name
	if key == "" {
		key = r.BaseURL + "|" + r.Model
	}

	rt.mu.Lock()
	last, ok := rt.lastRun[key]
	now := time.Now()
	var wait time.Duration
	if ok {
		if elapsed := now.Sub(last); elapsed < r.MinInterval {
			wait = r.MinInterval - elapsed
		}
	}
	rt.lastRun[key] = now.Add(wait)
	rt.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ── Orchestration ────────────────────────────────────────────────────────────

// Ask answers a question against the published corpus, emitting events as it goes.
//
// emit is called synchronously from this goroutine; returning an error from it aborts the run
// (the SSE writer uses that to stop when the browser disconnects).
//
// Ask 针对已发布语料回答问题，并在过程中发出事件。
//
// emit 在本 goroutine 中同步调用；它返回错误会中止整个流程（SSE 写入器借此在浏览器断开时停止）。
func (s *shareService) Ask(ctx context.Context, opts AskOptions, emit func(AskEvent) error) error {
	start := time.Now()

	question := strings.TrimSpace(opts.Question)
	if question == "" {
		return emit(AskEvent{Type: "error", Message: "问题不能为空"})
	}
	if opts.MaxQuestionChars > 0 && len([]rune(question)) > opts.MaxQuestionChars {
		return emit(AskEvent{Type: "error",
			Message: fmt.Sprintf("问题过长（上限 %d 字）", opts.MaxQuestionChars)})
	}
	if len(opts.Routes) == 0 {
		return emit(AskEvent{Type: "error", Message: "服务端未配置可用的模型线路"})
	}

	topK := opts.TopK
	if topK <= 0 {
		topK = 6
	}
	budget := opts.MaxContextChars
	if budget <= 0 {
		budget = 14000
	}

	if err := emit(AskEvent{Type: "stage", Icon: "question", Stage: "先看清楚你问的是哪件事…"}); err != nil {
		return err
	}
	tokens := askTokenize(question)
	if len(tokens) == 0 {
		return emit(AskEvent{Type: "error", Message: "没能在问题里识别出可用于检索的关键词"})
	}

	// The corpus size is not known until it is built, so the first line stays count-free rather
	// than printing a placeholder that would immediately be contradicted.
	// 语料规模要等到构建完成才知道，因此第一行不带数字，避免先印一个随后立刻被推翻的占位值。
	if err := emit(AskEvent{Type: "stage",
		Icon: "search", Stage: "翻箱倒柜，先把相关的那几页找出来…", Query: question, Terms: tokens}); err != nil {
		return err
	}

	corpus, err := s.publishedCorpus(ctx, opts.UID, opts.CacheTTL)
	if err != nil {
		return emit(AskEvent{Type: "error", Message: "读取已发布笔记失败：" + err.Error()})
	}
	if len(corpus.notes) == 0 {
		return emit(AskEvent{Type: "error", Message: "还没有已发布的笔记可供检索"})
	}

	// Retrieval now runs three stages inside askHybridRetrieve: BM25 recall, an optional
	// semantic channel, and an optional cross-encoder that decides the final order. The
	// returned label names the channels that actually contributed, so the progress line
	// describes what happened instead of always claiming a hybrid search.
	//
	// It runs behind the retrieval gate: at most askRetrievalConcurrency retrievals at a time,
	// because those seconds are the ones that cost money per question. The gate is released the
	// moment retrieval returns — the streaming that follows is long but nearly free, and holding
	// the gate across it would turn a 2-slot limit into "one visitor at a time, everybody else
	// gets 503 for 30 seconds".
	//
	// 检索现在由 askHybridRetrieve 内部走三步：BM25 召回、可选的语义通道、可选的交叉编码器
	// 决定最终顺序。返回的标签指明实际生效的通道，使进度行描述真实发生的事，
	// 而不是永远声称做了混合检索。
	//
	// 它在检索闸之后运行：同一时刻最多 askRetrievalConcurrency 次检索，因为这些秒数是每次提问
	// 真金白银的那部分。闸在检索返回的瞬间就归还——随后的流式输出很长但几乎免费，
	// 若跨过它继续持闸，一个 2 名额的限制就会退化成「一次只服务一个人，其余人 30 秒内都收 503」。
	hits, channels, gateErr := s.askRetrieveGuarded(func() ([]*askHit, string) {
		return s.askHybridRetrieve(ctx, opts, corpus, tokens, topK)
	})
	if gateErr != nil {
		// Capacity, not failure. Said in the answer area because the SSE headers were already
		// committed by the progress stages above, so an HTTP status is no longer available.
		//
		// 这是容量问题，不是故障。写在回答区里说，是因为上面的进度事件已经把 SSE 响应头提交了，
		// 已经无法再改 HTTP 状态码。
		return emit(AskEvent{Type: "error", Message: "检索繁忙，请稍候再试"})
	}
	if err := emit(AskEvent{Type: "stage",
		Icon: "funnel", Stage: fmt.Sprintf("翻遍了 %d 篇，按%s捞到 %d 篇最像的…", len(corpus.notes), channels, len(hits))}); err != nil {
		return err
	}
	if len(hits) == 0 {
		if err := emit(AskEvent{Type: "delta",
			Text: "已发布的笔记里没有检索到与该问题相关的内容。"}); err != nil {
			return err
		}
		return emit(AskEvent{Type: "done", ElapsedMS: time.Since(start).Milliseconds()})
	}

	// 排名靠前的笔记直接给**全文**，其余按预算切片段。
	//
	// 为什么：一篇两万字的笔记里，同一主题常分散在多个章节（适应证 / 并发症 / 各类牙拔除术）。
	// 只给片段就是赌"答案在被选中的那一段"，赌错时模型会如实说"没有相关内容"——
	// 它没说错，是我们只给了它一小块。
	//
	// 2026-10-28 实测：问「拔牙的并发症有哪些」，《第4章：牙及牙槽外科》21153 字，
	// 「出血」在 2321 字、「干槽症」在 6603 字、文末还有一份考点清单，
	// 而 1200 字的摘录窗口落在开头的适应证段，只捞到一句"菌血症 50%~80%"。
	//
	// 全文优先的取舍：上下文更贵，但答案对不对是第一位的；而且长上下文线路
	// （当前主线路 1M）本来就有余量。剩下的名额仍走片段，保证覆盖面。
	//
	// Top-ranked notes get their full text; the remainder are excerpted. A long note often
	// discusses one topic in several sections, and a single window bets on the wrong one.
	const fullTextRanks = 3
	const fullTextCap = 30000 // 单篇上限，防止一篇超长笔记吃掉整个预算

	per := budget / len(hits)
	if per < 400 {
		per = 400
	}
	if per > 2600 {
		per = 2600
	}

	excerpts := make(map[int64]string, len(hits))
	sources := make([]*AskSource, 0, len(hits))
	// fullUsed 记录前三名吃掉的字数，从剩余预算里扣掉，避免"前三给全文"把后面的名额挤成 0。
	fullUsed := 0
	for i, h := range hits {
		var ex string
		plain := h.note.Plain
		plainLen := len([]rune(plain))
		if i < fullTextRanks && plainLen > 0 {
			if plainLen <= fullTextCap {
				ex = plain
				fullUsed += plainLen
			} else {
				// 超长笔记：切到上限，但仍用多窗口而不是单窗口。
				ex = askExcerpt(plain, tokens, fullTextCap)
				fullUsed += len([]rune(ex))
			}
		} else {
			ex = askExcerpt(plain, tokens, per)
		}
		excerpts[h.note.ID] = ex
		title := h.note.Title
		if title == "" {
			title = pathBase(h.note.Path)
		}
		sources = append(sources, &AskSource{
			ID:      h.note.ID,
			Title:   title,
			Slug:    h.note.Slug,
			URL:     "/blog/" + h.note.Slug,
			Score:   math.Round(h.score*100) / 100,
			Via:     h.via,
			Excerpt: blogPlainText(ex, 160),
		})
	}
	_ = fullUsed

	if err := emit(AskEvent{Type: "retrieval", Sources: sources, Terms: tokens}); err != nil {
		return err
	}

	msgs := askBuildMessages(opts, hits, excerpts)

	// 模型接手前原本还有两幕（"被海量知识砸晕" / "被 GPT 小姐拖下工作台"），
	// 2026-10-26 按用户要求删除：它们是纯粹的垫场拟人，既不代表真实阶段，
	// 又让状态流转多出两拍，读起来像在等一个不存在的步骤。
	// 现在从「命中统计」直接到「由 X 接手」，中间不再有虚构动作。
	//
	// The two "interlude" beats before the model takes over were removed on request:
	// they were pure theatre — neither a real stage nor useful information.

	var lastErr error
	skipped := 0
	announced := false
	for i, route := range opts.Routes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		key := askRouteKey(route)
		if s.askRouteCooling(key) {
			skipped++
			continue
		}
		if err := s.askWaitPacing(ctx, route); err != nil {
			return err
		}
		if err := emit(AskEvent{Type: "route", Route: route.Name, Model: route.Model}); err != nil {
			return err
		}
		// 接手之后才说「开始写」。只报一次：重试换线路时再报一遍，
		// 读者会以为要写两遍。
		//
		// Say "writing" only after the hand-off, and only once — repeating it on each failover
		// reads as if the answer is about to be written twice.
		if !announced {
			announced = true
			if err := emit(AskEvent{Type: "stage", Icon: "pencil",
				Stage: route.Name + " 一秒就想清楚了！开始写…"}); err != nil {
				return err
			}
		}

		// Reasoning models emit their thinking phase before any answer token. It is streamed
		// out as its own event type so the browser can show the model actually working,
		// separately from the answer text.
		//
		// 推理模型在首个回答 token 之前先输出思考阶段。它以自己的事件类型流出，
		// 使浏览器能把「模型正在推理」与正式回答分开呈现。
		reasonLen := 0
		onReasoning := func(text string) bool {
			reasonLen += len([]rune(text))
			return emit(AskEvent{Type: "thinking", Text: text, ThoughtChars: reasonLen}) == nil
		}

		var started bool
		var callErr error
		attempts := 1 + route.Retries
		if attempts < 1 {
			attempts = 1
		}
		for attempt := 0; attempt < attempts; attempt++ {
			if attempt > 0 {
				// A transport-level failure (TLS reset, dropped stream) is worth one clean
				// retry on a fresh connection: the first attempt proves nothing about the second.
				if err := emit(AskEvent{Type: "stage",
					Icon: "alert", Stage: fmt.Sprintf("路断了一下，再走一次（第 %d 次）…", attempt)}); err != nil {
					return err
				}
				time.Sleep(500 * time.Millisecond)
			}
			started, callErr = s.askCallRoute(ctx, route, msgs, func(text string) bool {
				return emit(AskEvent{Type: "delta", Text: text}) == nil
			}, onReasoning)
			if callErr == nil || started {
				break
			}
			// Log every failed attempt, not just the final one. The route-level warning below
			// fires only when every attempt fails, so a retry that eventually succeeds used to
			// leave no trace at all — the "路断了一下" line in the browser was the sole evidence
			// that anything had gone wrong, with no server-side cause to match it against.
			//
			// 每次失败尝试都记，而不是只记最后一次。下面那条线路级 WARN 只在全部尝试失败时才打，
			// 因此「重试后成功」的情况原先不留任何痕迹——浏览器里那句「路断了一下」
			// 是唯一证据，而服务端没有可对照的原因。
			s.logger.Warn("blog ask attempt failed",
				zap.String("route", route.Name), zap.String("model", route.Model),
				zap.Int("attempt", attempt+1), zap.Int("attempts", attempts),
				zap.String("error", callErr.Error()))
			// 429 是分钟级冷却（实测 10s–2min，连续触发会更长），在同一条线路上
			// 等 500ms 再试必然还是 429。直接跳到下一条线路：换 key 有可能立刻可用。
			//
			// A 429 means a cooldown measured in minutes; retrying the same route after 500 ms
			// just spends another 429. Move to the next route instead — a different key may
			// have quota right now.
			if strings.Contains(callErr.Error(), "HTTP 429") {
				break
			}
		}

		if callErr == nil {
			s.askClearCooling(key)
			cites := askBuildCitations(hits, sources)
			if err := emit(AskEvent{Type: "citations", Citations: cites,
				Route: route.Name, Model: route.Model,
				ElapsedMS: time.Since(start).Milliseconds()}); err != nil {
				return err
			}
			return emit(AskEvent{Type: "done", Route: route.Name, Model: route.Model,
				ElapsedMS: time.Since(start).Milliseconds()})
		}

		lastErr = callErr
		s.logger.Warn("blog ask route failed",
			zap.String("route", route.Name), zap.String("model", route.Model),
			zap.String("error", callErr.Error()), zap.Int("routeIndex", i))

		if started {
			// Text already reached the browser; a silent switch would splice two answers.
			return emit(AskEvent{Type: "error",
				Message: "上游在输出中途中断：" + callErr.Error()})
		}

		// The caller left. Do not cool the route down and do not try the next one either —
		// there is nobody left to answer.
		// 调用方已离开：不冷却该线路，也不必再试下一条——已经没有人等着这个回答了。
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.askCoolRoute(key)

		if i < len(opts.Routes)-1 {
			if err := emit(AskEvent{Type: "stage",
				Icon: "fork", Stage: "这条道堵了，换条路走…"}); err != nil {
				return err
			}
		}
	}

	// Every route was either cooling down from a recent failure or failed just now. Say which,
	// because "all routes unavailable" reads like a config problem while "cooling down" is
	// simply "try again shortly".
	var msg string
	switch {
	case skipped == len(opts.Routes):
		msg = fmt.Sprintf("所有模型线路都在冷却中（最近均失败过），约 %d 秒后再试即可",
			int(askRouteCooldown.Seconds()))
	case lastErr != nil:
		msg = "所有模型线路都不可用：" + lastErr.Error()
	default:
		msg = "所有模型线路都不可用"
	}
	return emit(AskEvent{Type: "error", Message: msg})
}

// askBuildCitations maps the retrieved notes to 1-based indexes so the browser can resolve the
// [n] markers the model emitted.
// askBuildCitations 把检索到的笔记映射为 1 起始的下标，供浏览器解析模型输出的 [n] 标记。
func askBuildCitations(hits []*askHit, sources []*AskSource) []*AskCitation {
	out := make([]*AskCitation, 0, len(hits))
	for i, h := range hits {
		var src *AskSource
		if i < len(sources) {
			src = sources[i]
		}
		c := &AskCitation{Index: i + 1, ID: h.note.ID, Slug: h.note.Slug, URL: "/blog/" + h.note.Slug}
		c.Title = h.note.Title
		if src != nil {
			c.Title = src.Title
			c.Excerpt = src.Excerpt
		}
		out = append(out, c)
	}
	return out
}
