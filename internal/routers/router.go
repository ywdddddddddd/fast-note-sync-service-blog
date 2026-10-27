package routers

import (
	"embed"
	"net/http"
	"time"

	"github.com/haierkeys/fast-note-sync-service/internal/app"
	"github.com/haierkeys/fast-note-sync-service/internal/middleware"
	pkgapp "github.com/haierkeys/fast-note-sync-service/pkg/app"
	"github.com/haierkeys/fast-note-sync-service/pkg/limiter"
	"github.com/haierkeys/fast-note-sync-service/pkg/util"

	"github.com/gin-gonic/gin"
	ut "github.com/go-playground/universal-translator"
	"github.com/lxzan/gws"
	"go.uber.org/zap"
)

var methodLimiters = limiter.NewMethodLimiter().AddBuckets(
	limiter.BucketRule{
		Key:          "/auth",
		FillInterval: time.Second,
		Capacity:     10,
		Quantum:      10,
	},
	limiter.BucketRule{
		Key:          "/api/user/login",
		FillInterval: time.Second,
		Capacity:     5,
		Quantum:      1,
	},
	limiter.BucketRule{
		Key:          "/api/share/verify",
		FillInterval: time.Second,
		Capacity:     10,
		Quantum:      1,
	},
	limiter.BucketRule{
		Key:          "/api/share/note",
		FillInterval: time.Second,
		Capacity:     10,
		Quantum:      1,
	},
)

// applyTrustedProxies tells gin which peers may supply the client address.
//
// gin ships with trustedProxies = ["0.0.0.0/0", "::/0"] (gin.go), i.e. every peer is trusted,
// and Context.ClientIP walks X-Forwarded-For from the right, skipping trusted hops. With
// everything trusted it lands on the left-most entry — the one the client wrote. So without
// this call every reader of c.ClientIP() (login, token BoundIP checks, the recovery log) is
// reading a value an attacker chooses.
//
// The default here mirrors middleware.Proxy and clientIdentityResolver: an empty list means
// loopback only. Keeping the three in agreement matters more than which rule is chosen — if
// Proxy rewrites RemoteAddr from a peer that this engine then refuses to trust, the address
// ends up in a state no single rule predicts.
//
// applyTrustedProxies 告诉 gin 哪些对端可以提供客户端地址。
//
// gin 的内置值是信任 0.0.0.0/0（即全部），Context.ClientIP 从右往左跳过可信跳，于是在全信
// 的情况下落到最左项——那是客户端自己写的。不调这一行，每个 c.ClientIP() 的调用方
//（登录、Token 的 BoundIP 校验、panic 日志）读到的都是攻击者可选的值。
//
// 这里的默认值与 middleware.Proxy、clientIdentityResolver 一致：空列表＝只信回环。
// 三者保持一致比选了哪条规则更重要——若 Proxy 采信了某个本引擎拒绝信任的对端改写的
// RemoteAddr，地址就会落进任何单一规则都预测不了的状态。
func applyTrustedProxies(r *gin.Engine, trusted []string, appContainer *app.App) {
	if len(trusted) == 0 {
		trusted = []string{"127.0.0.1", "::1"}
	}
	if err := r.SetTrustedProxies(trusted); err != nil {
		// Configuration that cannot be parsed is not a reason to serve traffic with a wrong
		// client identity: every limit and every BoundIP check downstream would be keyed on
		// something nobody intended. Reported loudly; SetTrustedProxies has already cleared
		// the CIDRs, so the engine falls back to refusing to trust anything.
		//
		// 配置解析失败不能成为「带着错误的客户端身份继续服务」的理由：下游每个限流与
		// BoundIP 校验都会键在没人预期的值上。这里大声记录；SetTrustedProxies 已经把
		// CIDR 清空，因此引擎退化为谁都不信。
		if appContainer != nil {
			appContainer.Logger().Error("SetTrustedProxies failed; client address will be the TCP peer only",
				zap.Error(err), zap.Strings("trusted", trusted))
		}
	}
}

func NewRouter(frontendFiles embed.FS, appContainer *app.App, uni *ut.UniversalTranslator) *gin.Engine {
	// Get configuration
	// 获取配置
	cfg := appContainer.Config()

	// Convert custom response headers to http.Header for WebSocket handshake
	// 将自定义响应头转换为 http.Header 以便 WebSocket 握手使用
	var responseHeader = make(http.Header)
	for k, v := range cfg.Server.CustomResponseHeaders {
		responseHeader.Set(k, v)
	}

	var wss = pkgapp.NewWebsocketServer(pkgapp.WSConfig{
		GWSOption: gws.ServerOption{
			ResponseHeader:   responseHeader,
			CheckUtf8Enabled: *cfg.App.WebSocketCheckUtf8Enabled,
			ParallelEnabled:  *cfg.App.WebSocketParallelEnabled, // Enable parallel message processing from config
			// 从配置开启并行消息处理
			Recovery: gws.Recovery, // Enable exception recovery
			// 开启异常恢复
			PermessageDeflate: gws.PermessageDeflate{
				Enabled:               *cfg.App.WebSocketCompressionEnabled,
				Level:                 cfg.App.WebSocketCompressionLevel,
				Threshold:             cfg.App.WebSocketCompressionThreshold,
				ServerContextTakeover: true,
				ClientContextTakeover: true,
			}, // Enable compression from config
			// 从配置开启压缩
			ParallelGolimit:    cfg.App.WebSocketParallelGolimit,
			ReadMaxPayloadSize: int(util.ParseSize(cfg.App.WebSocketReadMaxPayloadSize, 1024*1024*64)), // Load from config, default 64MB
			// 从配置读取，默认 64MB
			WriteMaxPayloadSize: int(util.ParseSize(cfg.App.WebSocketWriteMaxPayloadSize, 1024*1024*64)), // Load from config, default 64MB
			// 从配置读取，默认 64MB
		},
		// WriteTimeout application-layer write deadline for outbound messages, from config
		// (already resolved: nil-vs-explicit-0 distinguished by defaults.Set on the *int field)
		// WriteTimeout 应用层出站消息写超时，来自配置（已解析：*int 字段上 defaults.Set 已区分 nil 与显式 0）
		WriteTimeout: time.Duration(*cfg.App.WebSocketWriteTimeout) * time.Second,
	}, appContainer)
	appContainer.SetWSS(wss)

	// Initialize WebSocket routes
	// 初始化 WebSocket 路由
	initWebSocketRoutes(wss, appContainer)

	r := gin.New()
	applyTrustedProxies(r, cfg.Server.TrustedProxies, appContainer)
	r.Use(middleware.Proxy(cfg.Server.TrustedProxies))
	r.Use(middleware.Cors(cfg.Server.CORSAllowedOrigins, cfg.Server.ExtApiUrl))
	if len(cfg.Server.CustomResponseHeaders) > 0 {
		r.Use(middleware.CustomHeaders(cfg.Server.CustomResponseHeaders))
	}

	// Register Static routes
	// 注册静态资源路由
	// If independent ports are configured, the main port only provides necessary static files (for API/compatibility)
	// 如果配置了独立端口，则主端口仅提供必要的静态资源，不再提供 Web/Share 页面访问
	registerStaticFiles(r, frontendFiles, appContainer)
	if cfg.Server.WebGuiPort == "" {
		registerWebGuiRoutes(r, frontendFiles, appContainer)
	}
	registerOAuthAuthorizePageRoutes(r, frontendFiles, appContainer)
	if cfg.Server.SharePort == "" {
		registerShareRoutes(r, frontendFiles, appContainer)
	}
	registerOAuthMetadataRoutes(r, appContainer)

	// Register API routes
	// 注册 API 路由
	registerAPIRoutes(r, appContainer, wss, uni)

	// Register OpenAPI/Swagger routes only for non ReleaseMode
	// 注册 OpenAPI/Swagger 路由
	if gin.Mode() != gin.ReleaseMode {
		registerOpenAPIRoutes(r, frontendFiles)
	}

	r.NoRoute(middleware.NoFound())

	return r
}

func NewWebGuiRouter(frontendFiles embed.FS, appContainer *app.App) *gin.Engine {
	cfg := appContainer.Config()
	r := gin.New()
	applyTrustedProxies(r, cfg.Server.TrustedProxies, appContainer)
	r.Use(middleware.Proxy(cfg.Server.TrustedProxies))
	r.Use(middleware.Cors(cfg.Server.CORSAllowedOrigins, cfg.Server.ExtApiUrl))
	if len(cfg.Server.CustomResponseHeaders) > 0 {
		r.Use(middleware.CustomHeaders(cfg.Server.CustomResponseHeaders))
	}

	registerStaticFiles(r, frontendFiles, appContainer)
	registerWebGuiRoutes(r, frontendFiles, appContainer)
	registerOAuthAuthorizePageRoutes(r, frontendFiles, appContainer)

	r.NoRoute(middleware.NoFound())
	return r
}

func NewShareRouter(frontendFiles embed.FS, appContainer *app.App) *gin.Engine {
	cfg := appContainer.Config()
	r := gin.New()
	applyTrustedProxies(r, cfg.Server.TrustedProxies, appContainer)
	r.Use(middleware.Proxy(cfg.Server.TrustedProxies))
	r.Use(middleware.Cors(cfg.Server.CORSAllowedOrigins, cfg.Server.ExtApiUrl))
	if len(cfg.Server.CustomResponseHeaders) > 0 {
		r.Use(middleware.CustomHeaders(cfg.Server.CustomResponseHeaders))
	}

	registerStaticFiles(r, frontendFiles, appContainer)
	registerShareRoutes(r, frontendFiles, appContainer)

	r.NoRoute(middleware.NoFound())
	return r
}
