package config

// AISettings configures the blog's retrieval-augmented Q&A entry.
//
// The blog is public, so this section is off by default: it needs at least one upstream route
// with an API key before the floating "ask" button does anything.
//
// AISettings 配置博客的检索增强问答入口。
//
// 博客是公开页面，因此该段默认关闭：在至少配置一条带 api-key 的上游线路之前，悬浮问答按钮不会做任何事。
type AISettings struct {
	// Enabled turns the whole feature on. Nil (unset) means off.
	// Enabled 总开关。nil（未配置）表示关闭。
	Enabled *bool `yaml:"enabled" default:"false"`

	// Routes are tried in order: the first is preferred, later ones act as fallbacks when the
	// preferred route errors or is rate limited. Each route may declare its own pacing so a
	// strict upstream does not take down the feature.
	// Routes 按顺序尝试：第一条优先，后续几条在前一条报错或被限流时顶替。
	// 每条线路可声明各自的节流间隔，以免某个严格限速的上游把整个功能拖垮。
	Routes []AIRouteSetting `yaml:"routes"`

	// TopK is how many note fragments are fed to the model as context.
	// TopK 作为上下文喂给模型的笔记片段数量。
	TopK int `yaml:"top-k" default:"6"`

	// MaxContextChars caps the assembled context budget (characters, not tokens).
	// MaxContextChars 限制拼装后的上下文预算（按字符数，不是 token）。
	MaxContextChars int `yaml:"max-context-chars" default:"14000"`

	// CacheTTL caches the published-note corpus between questions; empty or "0" disables caching.
	// CacheTTL 缓存公开笔记语料，避免每次提问都重新读盘；空值或 "0" 表示不缓存。
	CacheTTL string `yaml:"cache-ttl" default:"5m"`

	// SystemPrompt overrides the built-in instructions. Leave empty to use the default.
	// SystemPrompt 覆盖内置指令。留空则使用默认值。
	SystemPrompt string `yaml:"system-prompt"`

	// MaxQuestionChars rejects absurdly long questions before they reach an upstream.
	// MaxQuestionChars 在问题送达上游之前拦截过长的输入。
	MaxQuestionChars int `yaml:"max-question-chars" default:"500"`

	// Embedding adds a semantic retrieval channel beside BM25.
	// Embedding 在 BM25 之外增加一条语义检索通道。
	Embedding AIEmbeddingSettings `yaml:"embedding"`

	// Rerank reorders the merged candidate set before the answer is generated.
	// Rerank 在生成回答之前对合并后的候选集重新排序。
	Rerank AIRerankSettings `yaml:"rerank"`
}

// AIEmbeddingSettings configures the vector channel.
//
// ChunkChars is capped well below the model's context window on purpose: bge-m3 accepts
// 8192 tokens and rejects 12000 outright (measured: HTTP 400), and Chinese costs roughly one
// token per character. A 1800-character chunk therefore uses under a quarter of the window,
// which leaves room for the model's own instruction prefix and keeps a chunk narrow enough
// that its vector describes one topic rather than a whole chapter.
//
// AIEmbeddingSettings 配置向量通道。
//
// ChunkChars 刻意远低于模型的上下文窗口：bge-m3 接受 8192 token，而 12000 会直接报错
// （实测 HTTP 400），中文大致一字一 token。因此 1800 字符的块只占窗口不到四分之一，
// 既给模型自身的前缀留了余量，也让一个块足够窄，使它的向量描述一个主题而不是一整章。
type AIEmbeddingSettings struct {
	// Enabled turns the vector channel on. Nil (unset) means off, and BM25 alone still works.
	// Enabled 打开向量通道。nil（未配置）表示关闭，此时仅 BM25 仍然可用。
	Enabled *bool `yaml:"enabled" default:"false"`

	BaseURL string `yaml:"base-url"`
	APIKey  string `yaml:"api-key"`
	Model   string `yaml:"model" default:"BAAI/bge-m3"`

	// BatchSize is how many chunks one embedding request carries. Measured on the upstream:
	// 64 texts complete in ~0.6 s, so a whole vault indexes in a handful of round trips.
	// BatchSize 单次 embedding 请求携带的块数。实测上游 64 条文本约 0.6 秒完成，
	// 因此整个仓库只需少数几次往返即可建好索引。
	BatchSize int `yaml:"batch-size" default:"32"`

	ChunkChars   int `yaml:"chunk-chars" default:"1800"`
	ChunkOverlap int `yaml:"chunk-overlap" default:"200"`

	// CandidateLimit is how many chunks each channel contributes to the rerank pool.
	// CandidateLimit 每条通道向重排池贡献的块数。
	CandidateLimit int `yaml:"candidate-limit" default:"60"`
}

// AIRerankSettings configures the cross-encoder pass.
//
// DocChars truncates each candidate before it reaches the reranker. The reranker scores the
// question against every candidate in one request, so an untruncated set of long notes would
// blow the request budget; 1200 characters is enough to carry the passage that actually
// answers the question.
//
// AIRerankSettings 配置交叉编码器环节。
//
// DocChars 在候选进入重排器之前截断。重排器在一次请求里对每个候选与问题打分，
// 因此不截断的长笔记会撑爆请求预算；1200 字符足以承载真正回答该问题的那一段。
type AIRerankSettings struct {
	Enabled *bool  `yaml:"enabled" default:"false"`
	BaseURL string `yaml:"base-url"`
	APIKey  string `yaml:"api-key"`
	Model   string `yaml:"model" default:"BAAI/bge-reranker-v2-m3"`

	// TopN is how many reranked candidates survive into the prompt.
	// TopN 重排后进入提示词的候选数量。
	TopN int `yaml:"top-n" default:"8"`

	DocChars int `yaml:"doc-chars" default:"1200"`
}

// AIEmbeddingEnabled reports whether the vector channel is configured well enough to use.
// AIEmbeddingEnabled 返回向量通道是否已配置到可用状态。
func (s AISettings) AIEmbeddingEnabled() bool {
	return s.Embedding.Enabled != nil && *s.Embedding.Enabled &&
		s.Embedding.BaseURL != "" && s.Embedding.APIKey != "" && s.Embedding.Model != ""
}

// AIRerankEnabled reports whether the reranker is configured well enough to use.
// AIRerankEnabled 返回重排器是否已配置到可用状态。
func (s AISettings) AIRerankEnabled() bool {
	return s.Rerank.Enabled != nil && *s.Rerank.Enabled &&
		s.Rerank.BaseURL != "" && s.Rerank.APIKey != "" && s.Rerank.Model != ""
}

// AIRouteSetting one upstream OpenAI-compatible chat endpoint.
// AIRouteSetting 一条上游 OpenAI 兼容对话端点。
type AIRouteSetting struct {
	Name    string `yaml:"name"`
	BaseURL string `yaml:"base-url"`
	APIKey  string `yaml:"api-key"`
	Model   string `yaml:"model"`

	// Headers are extra HTTP headers sent with every request on this route. They exist because
	// some gateways refuse requests that lack a routing hint: OpenCode Go returns HTTP 400
	// "Request is missing x-opencode-session and cannot be routed efficiently" (enforced since
	// 2026-09-06) unless x-opencode-session carries a per-session id. The value supports the
	// placeholder ${uuid}, replaced with a fresh UUIDv4 on every request.
	//
	// Headers 是每条线路额外发送的 HTTP 头。它存在的原因是部分网关会拒绝缺少路由提示的请求：
	// OpenCode Go 自 2026-09-06 起，若没有 x-opencode-session，直接回 HTTP 400
	// 「Request is missing x-opencode-session and cannot be routed efficiently」。
	// 值支持占位符 ${uuid}，每次请求替换为新生成的 UUIDv4。
	Headers map[string]string `yaml:"headers"`

	// MinInterval is the minimum spacing between two calls on this route (e.g. "6s" for a
	// 10-requests-per-minute budget). Empty means no client-side pacing.
	// MinInterval 该线路上两次调用之间的最小间隔（例如每分钟 10 次的预算对应 "6s"）。
	// 留空表示不做客户端节流。
	MinInterval string `yaml:"min-interval"`

	// Enabled lets a route be parked without deleting its credentials.
	// Enabled 允许在不删除凭据的前提下临时停用某条线路。
	Enabled *bool `yaml:"enabled"`

	// Timeout caps a single upstream call. Thinking models need a generous value: they emit
	// hundreds of reasoning tokens before the first answer token.
	// Timeout 单次上游调用的超时。思考模型需要给足余量：它们在首个回答 token 之前会先吐出
	// 数百个推理 token。
	Timeout string `yaml:"timeout" default:"300s"`

	// ── Decoding parameters ────────────────────────────────────────────────
	//
	// These are pointers on purpose. A retrieval answer must be able to pin
	// presence-penalty and frequency-penalty at exactly 0 so that domain terms and entity
	// names may repeat naturally instead of being paraphrased away; a plain float would turn
	// that explicit 0 into "unset", and the upstream default would silently apply instead.
	//
	// ── 解码参数 ──────────────────────────────────────────────────────────
	//
	// 这些字段刻意使用指针。检索问答需要把 presence-penalty 与 frequency-penalty 精确钉在 0，
	// 让专业术语与实体名自然重复而不被改写；普通 float 会把显式的 0 变成「未设置」，
	// 于是上游默认值被静默套用。
	Temperature      *float64 `yaml:"temperature"`
	TopP             *float64 `yaml:"top-p"`
	TopK             *int     `yaml:"top-k"`
	PresencePenalty  *float64 `yaml:"presence-penalty"`
	FrequencyPenalty *float64 `yaml:"frequency-penalty"`
	MinP             *float64 `yaml:"min-p"`

	// MaxTokens bounds thinking tokens plus answer tokens together.
	// MaxTokens 同时限制思考 token 与回答 token。
	MaxTokens *int `yaml:"max-tokens" default:"3000"`

	// EnableThinking turns on the model's reasoning phase when the upstream supports it.
	// Reasoning arrives as delta.reasoning_content and is surfaced separately from the answer.
	// EnableThinking 在上游支持时开启模型的推理阶段。
	// 推理内容以 delta.reasoning_content 到达，并与正式回答分开呈现。
	EnableThinking *bool `yaml:"enable-thinking" default:"false"`

	// ThinkingLevel expresses reasoning depth for upstreams that take a thinking object rather
	// than a boolean. Empty means "derive it from enable-thinking" (enabled/disabled).
	// Accepted values match the DeepSeek-style vocabulary: disabled | enabled | low | high | max.
	//
	// Measured on OpenCode Go (deepseek-v4.1-flash): a one-line factual question spent 1133 ms
	// and 17 completion tokens with thinking disabled, 1512 ms / 66 tokens at max. The level
	// is a ceiling, not a quota — the model stops as soon as it is satisfied, so raising it
	// does not automatically cost more. It does raise the bound on MaxTokens though: a long
	// reasoning trace will consume the whole budget and leave content empty with
	// finish_reason=length. Keep MaxTokens generous whenever this is set above disabled.
	//
	// ThinkingLevel 为使用 thinking 对象（而不是布尔开关）的上游表达推理深度。
	// 留空表示由 enable-thinking 推导（enabled/disabled）。取值沿用 DeepSeek 风格的词汇：
	// disabled | enabled | low | high | max。
	//
	// 在 OpenCode Go（deepseek-v4.1-flash）实测：一个只需一行的常识问题，关闭思考耗时 1133 ms、
	// 17 个 completion token；max 档 1512 ms、66 个 token。该档位是上限而不是配额——模型满足即停，
	// 因此调高它不会自动变贵。但它抬高了 MaxTokens 的下界：一次长推理会吃光预算，
	// 让 content 为空且 finish_reason=length。把它设成 disabled 以上时，MaxTokens 要给足。
	ThinkingLevel string `yaml:"thinking-level"`

	// Retries is how many extra attempts this route gets on a transport-level failure
	// (TLS reset, connection dropped mid-stream). Long thinking streams are the most exposed
	// to those, so one retry is worth having.
	// Retries 是本线路在传输层失败（TLS 重置、流中途断连）时的额外尝试次数。
	// 长思考流最容易撞上这类问题，因此留一次重试是值得的。
	Retries *int `yaml:"retries" default:"1"`

	// ExtraBody is merged into the request JSON verbatim, after every built-in key.
	//
	// Each provider spells "how hard should you think" differently, and some put it in a nested
	// object (reasoning: {effort: high}) rather than a top-level field. Adding a dedicated field
	// per provider would turn the config model into a vendor inventory, so this is the escape
	// hatch: the config states the exact JSON fragment, the service only merges it.
	//
	// Top-level keys in ExtraBody WIN over built-in ones. That is deliberate — an explicit
	// setting in the config file should never be silently overridden by a default the code
	// happened to pick. Nested objects are replaced wholesale, not deep-merged: predicting
	// half-merged shapes across providers is how you get "it worked for one vendor".
	//
	// ExtraBody 会被原样合并进请求 JSON，位置在所有内置键之后。
	//
	// 各家对「推理强度」的叫法与位置都不同，有的还塞在嵌套对象里
	// （reasoning: {effort: high}）而不是顶层字段。为每家加一个专用字段，会让配置模型
	// 逐渐变成供应商清单，所以这里留一个逃生口：配置直接写它要的 JSON 片段，服务只负责合并。
	//
	// 顶层同名键 **ExtraBody 优先**。这是刻意的：配置文件里的显式设置，
	// 不该被代码里某个默认值悄悄盖掉。嵌套对象整体替换、不做深合并——
	// 猜「两边各留一半」的形状，正是"只对一家好用"的来源。
	ExtraBody map[string]any `yaml:"extra-body"`
}

// AIRouteMaxTokens returns the token budget, defaulting when unset.
// AIRouteMaxTokens 返回 token 预算，未设置时取默认值。
func (r AIRouteSetting) AIRouteMaxTokens() int {
	if r.MaxTokens == nil || *r.MaxTokens <= 0 {
		return 3000
	}
	return *r.MaxTokens
}

// AIRouteThinking reports whether the reasoning phase is requested.
// AIRouteThinking 返回是否请求推理阶段。
func (r AIRouteSetting) AIRouteThinking() bool {
	return r.EnableThinking != nil && *r.EnableThinking
}

// AIRouteRetries returns the extra-attempt count, defaulting to 1.
// AIRouteRetries 返回额外尝试次数，默认 1。
func (r AIRouteSetting) AIRouteRetries() int {
	if r.Retries == nil {
		return 1
	}
	if *r.Retries < 0 {
		return 0
	}
	return *r.Retries
}

// AIRouteEnabled reports whether the route should be used (default true when unset).
// AIRouteEnabled 返回该线路是否可用（未配置时默认可用）。
func (r AIRouteSetting) AIRouteEnabled() bool { return r.Enabled == nil || *r.Enabled }

// AIEnabled reports whether the feature is switched on.
// AIEnabled 返回该功能是否已开启。
func (a AISettings) AIEnabled() bool { return a.Enabled != nil && *a.Enabled }
