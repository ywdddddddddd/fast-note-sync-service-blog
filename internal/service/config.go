// Package service implements the business logic layer
// Package service 实现业务逻辑层
package service

// ServiceConfig service layer configuration
// ServiceConfig 服务层配置
type ServiceConfig struct {
	User  UserServiceConfig  // User related config // 用户相关配置
	App   AppServiceConfig   // App related config // 应用相关配置
	Token TokenServiceConfig // Token related config // Token 相关配置

	// BlogAccessLogDir is where the per-day visit files live. An empty value disables the log
	// rather than erroring: a service that cannot write a visit log should still serve the blog.
	//
	// BlogAccessLogDir 是按日划分的访问文件所在目录。空值会停用该日志而不是报错：
	// 一个写不了访问日志的服务仍然应该能提供博客。
	BlogAccessLogDir string
}

// UserServiceConfig user service configuration
// UserServiceConfig 用户服务配置
type UserServiceConfig struct {
	RegisterIsEnable bool // Whether registration is enabled // 注册是否启用
	AdminUID         int  // Admin UID, 0 means no restriction // 管理员 UID，0 表示不限制
}

// TokenServiceConfig token service configuration for WebGUI auto-issued login tokens
// TokenServiceConfig WebGUI 自动签发登录 Token 的配置
type TokenServiceConfig struct {
	WebGUILoginTokenExpiry string // Expiry duration of WebGUI login tokens (e.g. 7d, 24h) // WebGUI 登录 Token 有效期（如 7d、24h）
	WebGUILoginTokenBindIP bool   // Whether to bind client IP on WebGUI login token issuance // WebGUI 登录 Token 是否绑定客户端 IP
}

// AppServiceConfig app service configuration
// AppServiceConfig 应用服务配置
type AppServiceConfig struct {
	SoftDeleteRetentionTime string                 // Soft delete retention time (e.g., 7d, 24h, 30m, 0/empty for no cleanup) // 软删除保留时间（支持格式：7d、24h、30m、0 或空表示不自动清理）
	HistoryKeepVersions     *int                   // History versions to keep; nil = default 100, explicit 0 = keep unlimited (no cleanup) // 历史记录保留版本数；nil=默认100，显式 0=无限保留不清理
	HistorySaveDelay        string                 // History save delay (e.g., 10s, 1m, default 10s) // 历史记录保存延迟时间（支持格式：10s、1m，默认 10s）
	ShareTokenExpiry        string                 // Share token expiry // 分享 Token 过期时间
	ShortLink               ShortLinkServiceConfig // Short link configuration // 短链配置
}

// ShortLinkServiceConfig short link service configuration
// ShortLinkServiceConfig 短链服务配置
type ShortLinkServiceConfig struct {
	// Mode selects the backend: "sink" (third-party) or "local" (built-in /s/<slug>)
	// Mode 选择后端："sink"（第三方）或 "local"（内置 /s/<slug>）
	Mode     string // Backend mode // 后端模式
	BaseURL  string // Base URL // 基础 URL
	APIKey   string // API Key // API 密钥
	Password string // Password // 密码
	Cloaking bool   // Cloaking // 遮盖
	// SigKey is the HMAC key used to sign built-in short link slugs. It is supplied by the
	// infrastructure layer from security.share-token-key, so no extra config is needed.
	// SigKey 是内置短链 slug 的 HMAC 签名密钥，由基础设施层从 security.share-token-key 注入，
	// 因此无需新增配置项。
	SigKey string // Local short link signing key // 内置短链签名密钥
}
