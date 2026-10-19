package dto

// AI route and AI configuration DTOs, used by the Obsidian plugin's settings tab.
//
// AI 线路与 AI 配置的 DTO，供 Obsidian 插件设置页使用。

// AIRouteDTO one upstream route as reported to clients.
//
// APIKey is returned in full: these endpoints are admin-only and the client is the user's own
// Obsidian instance, which needs the current value to edit it in place. That is the same trust
// model the sync token already runs under.
//
// AIRouteDTO 返回给客户端的一条上游线路。
//
// APIKey 完整回传：这些接口仅管理员可用，且客户端就是用户自己的 Obsidian 实例，
// 需要当前值才能就地编辑。这与同步令牌所依赖的信任模型一致。
type AIRouteDTO struct {
	Name        string `json:"name"`
	BaseURL     string `json:"baseUrl"`
	APIKey      string `json:"apiKey"`
	Model       string `json:"model"`
	MinInterval string `json:"minInterval"`
	Timeout     string `json:"timeout"`
	Enabled     bool   `json:"enabled"`
}

// AIConfigDTO the whole AI section as reported to clients.
// AIConfigDTO 返回给客户端的完整 AI 配置段。
type AIConfigDTO struct {
	Enabled         bool         `json:"enabled"`
	TopK            int          `json:"topK"`
	MaxContextChars int          `json:"maxContextChars"`
	CacheTTL        string       `json:"cacheTtl"`
	MaxQuestionLen  int          `json:"maxQuestionChars"`
	SystemPrompt    string       `json:"systemPrompt"`
	Routes          []AIRouteDTO `json:"routes"`

	// HomeURL is the public blog address, so the settings tab can link straight to it.
	// HomeURL 是博客的公开地址，便于设置页直接跳转。
	HomeURL string `json:"homeUrl"`

	// AskURL is the streaming endpoint the blog front-end posts questions to.
	// AskURL 是博客前端提交问题的流式端点。
	AskURL string `json:"askUrl"`
}

// AIRouteRequest partial update of one route. Nil fields keep their current value.
// AIRouteRequest 单条线路的部分更新。为 nil 的字段保持原值。
type AIRouteRequest struct {
	Name        *string `json:"name"`
	BaseURL     *string `json:"baseUrl"`
	APIKey      *string `json:"apiKey"`
	Model       *string `json:"model"`
	MinInterval *string `json:"minInterval"`
	Timeout     *string `json:"timeout"`
	Enabled     *bool   `json:"enabled"`
}

// AIConfigRequest partial update of the AI section.
//
// Routes replaces the whole list when non-nil (the settings tab edits routes as a unit and
// sends them back together), so route ordering — which is the fallback order — stays exactly
// as the user arranged it.
//
// AIConfigRequest AI 配置段的部分更新。
//
// Routes 非 nil 时整表替换（设置页把线路当作一个整体编辑并一起回传），
// 因此线路顺序——也就是降级顺序——会严格保持用户排定的次序。
type AIConfigRequest struct {
	Enabled         *bool            `json:"enabled"`
	TopK            *int             `json:"topK"`
	MaxContextChars *int             `json:"maxContextChars"`
	CacheTTL        *string          `json:"cacheTtl"`
	MaxQuestionLen  *int             `json:"maxQuestionChars"`
	SystemPrompt    *string          `json:"systemPrompt"`
	Routes          *[]AIRouteRequest `json:"routes"`
}
