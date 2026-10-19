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
}

// AIRouteSetting one upstream OpenAI-compatible chat endpoint.
// AIRouteSetting 一条上游 OpenAI 兼容对话端点。
type AIRouteSetting struct {
	Name    string `yaml:"name"`
	BaseURL string `yaml:"base-url"`
	APIKey  string `yaml:"api-key"`
	Model   string `yaml:"model"`

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

	// Retries is how many extra attempts this route gets on a transport-level failure
	// (TLS reset, connection dropped mid-stream). Long thinking streams are the most exposed
	// to those, so one retry is worth having.
	// Retries 是本线路在传输层失败（TLS 重置、流中途断连）时的额外尝试次数。
	// 长思考流最容易撞上这类问题，因此留一次重试是值得的。
	Retries *int `yaml:"retries" default:"1"`
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
