package routers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	ut "github.com/go-playground/universal-translator"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
	appconfig "github.com/haierkeys/fast-note-sync-service/internal/config"
	"github.com/haierkeys/fast-note-sync-service/internal/middleware"
	"github.com/haierkeys/fast-note-sync-service/internal/routers/api_router"
	pkgapp "github.com/haierkeys/fast-note-sync-service/pkg/app"
)

func registerAPIRoutes(r *gin.Engine, appContainer *app.App, wss *pkgapp.WebsocketServer, uni *ut.UniversalTranslator) {
	cfg := appContainer.Config()
	api := r.Group("/api")
	{
		// One resolver, shared with the blog endpoints: the process keeps a single definition
		// of "who is calling", built from the same trusted-proxy list middleware.Proxy uses.
		//
		// 与博客端点共用同一个 resolver：进程内只有一份「谁在调用」的定义，
		// 且取自 middleware.Proxy 使用的同一份可信代理列表。
		ident := newClientIdentityResolver(cfg.Server.TrustedProxies)

		api.Use(middleware.AppInfoWithConfig(app.Name, appContainer.Version().Version))
		api.Use(gin.Logger())
		api.Use(middleware.TraceMiddlewareWithConfig(*cfg.Tracer.Enabled, cfg.Tracer.Header)) // Trace ID middleware
		// Trace ID 中间件
		api.Use(middleware.RateLimiter(methodLimiters))

		// MCP routes
		registerMCPRoutes(api, appContainer, wss)

		api.Use(middleware.ContextTimeout(time.Duration(cfg.App.DefaultContextTimeout) * time.Second))
		api.Use(middleware.LangWithTranslator(uni))
		// The text log and the structured visit log are written from the same place, so the two can
		// never disagree about what happened.
		//
		// 文本日志与结构化访问日志从同一处写出，因此两者永远不会对「发生了什么」给出不同答案。
		api.Use(middleware.AccessLogWithRecorder(appContainer.Logger(),
			func(method, path string, status int, ms int64, ip, client string) {
				if appContainer.ShareService != nil {
					appContainer.ShareService.RecordBlogAccess(method, path, status, ms, ip, client)
				}
			}))
		api.Use(middleware.RecoveryWithLogger(appContainer.Logger()))

		// Create Handlers (injected App Container)
		// 创建 Handlers（注入 App Container）
		userHandler := api_router.NewUserHandler(appContainer)
		vaultHandler := api_router.NewVaultHandler(appContainer)
		noteHandler := api_router.NewNoteHandler(appContainer, wss)
		folderHandler := api_router.NewFolderHandler(appContainer)
		fileHandler := api_router.NewFileHandler(appContainer, wss)
		noteHistoryHandler := api_router.NewNoteHistoryHandler(appContainer, wss)
		versionHandler := api_router.NewVersionHandler(appContainer)
		adminControlHandler := api_router.NewAdminControlHandler(appContainer, wss)
		shareHandler := api_router.NewShareHandler(appContainer, wss)
		storageHandler := api_router.NewStorageHandler(appContainer)
		backupHandler := api_router.NewBackupHandler(appContainer)
		gitSyncHandler := api_router.NewGitSyncHandler(appContainer)
		settingHandler := api_router.NewSettingHandler(appContainer, wss)
		syncLogHandler := api_router.NewSyncLogHandler(appContainer)
		tokenHandler := api_router.NewTokenHandler(appContainer)
		stytchOAuthHandler := api_router.NewStytchOAuthHandler(appContainer)
		oidcHandler := api_router.NewOIDCHandler(appContainer)
		blogHandler := api_router.NewBlogHandler(appContainer)

		// No-auth WebGUI restricted routes
		// 免认证但仅限 WebGUI 访问的路由组
		noAuthWebgui := api.Group("")
		noAuthWebgui.Use(middleware.RequireWebGUI())
		{
			// Credential endpoints are the only two anonymous routes that verify a secret, so
			// they are the only two that need a per-caller limit. MethodLimiter already caps
			// the whole path at five per second, but its bucket key is the URI — one bucket
			// shared by everyone, which lets anyone lock the operator out and never bounds a
			// slow guessing run. See login_guard.go.
			//
			// 凭据端点是在所有匿名路由里唯二会校验秘密的，因此也是唯二需要「按调用方」限流的。
			// MethodLimiter 已经把那整条路径压到每秒五次，但它的桶键是 URI —— 一个全站共享的桶，
			// 于是任何人都能把运维锁在外面，同时永远约束不了一次慢速猜解。见 login_guard.go。
			noAuthWebgui.POST("/user/register", loginRateLimited(userHandler.Register, ident))
			noAuthWebgui.POST("/user/login", loginRateLimited(userHandler.Login, ident))
			noAuthWebgui.GET("/user/auth/oidc/config", oidcHandler.Config)
			noAuthWebgui.GET("/webgui/config", adminControlHandler.Config)
		}
		api.GET("/user/auth/oidc/start", oidcHandler.Start)
		api.GET("/user/auth/oidc/start/:providerID", oidcHandler.Start)
		for _, route := range oidcCallbackRoutes(cfg.OIDC) {
			api.GET(route, oidcHandler.Callback)
		}
		api.GET("/user/sync", wss.Run())

		// Add server version interface (no auth required)
		// 添加服务端版本号接口（无需认证）
		api.GET("/version", versionHandler.ServerVersion)
		api.GET("/support", versionHandler.Support)

		// Health check interface (no auth required)
		// 健康检查接口（无需认证）
		healthHandler := api_router.NewHealthHandler(appContainer)
		api.GET("/health", healthHandler.Check)

		// Share routing group (controlled read-only access)
		// 分享路由组 (受控的只读访问)
		share := api.Group("/share")
		share.Use(middleware.ShareAuthToken(appContainer.ShareService))
		{
			share.GET("/note", shareHandler.NoteGet) // Get shared note
			// 获取分享的笔记
			share.GET("/file", shareHandler.FileGet) // Get shared file content
			// 获取分享的文件内容
		}

		// Auth routing group (authentication required)
		// 需要认证的路由组
		auth := api.Group("/")
		auth.Use(middleware.UserAuthTokenWithConfig(cfg.Security.AuthTokenKey, appContainer.TokenService))
		{
			// Create share
			// 创建分享
			auth.POST("/auth/logout", userHandler.Logout)
			auth.POST("/share", shareHandler.Create)
			auth.POST("/share/password", shareHandler.UpdatePassword)
			auth.GET("/share", shareHandler.Query)
			auth.DELETE("/share", shareHandler.Cancel)
			auth.POST("/share/short_link", shareHandler.CreateShortLink)
			auth.GET("/shares", shareHandler.List)

			// Built-in blog settings (used by the Obsidian plugin's "Blog" tab)
			// 内置博客设置（供 Obsidian 插件的“博客”标签页使用）
			auth.GET("/blog/config", blogHandler.GetConfig)
			auth.POST("/blog/config", blogHandler.UpdateConfig)
			auth.GET("/blog/posts", blogHandler.ListPosts)
			auth.GET("/blog/access-log", blogHandler.GetAccessLog)

			// Blog Q&A configuration (upstream routes, retrieval budget)
			// 博客问答配置（上游线路、检索预算）
			auth.GET("/blog/ai-config", blogHandler.GetAIConfig)
			auth.POST("/blog/ai-config", blogHandler.UpdateAIConfig)

			// Admin config interface
			// 管理员配置接口
			auth.GET("/admin/upgrade", adminControlHandler.Upgrade)
			auth.GET("/admin/check", adminControlHandler.CheckAdmin)
			auth.GET("/admin/ws_clients", adminControlHandler.GetWSClients)
			auth.DELETE("/admin/ws_client/:traceId", adminControlHandler.KickWSClient)

			// Version source latency probe (auth required: triggers real outbound requests)
			// 版本源延迟探测（需认证：会触发真实的外部网络请求）
			auth.GET("/version/probe", versionHandler.ProbeSources)

			auth.GET("/user/info", userHandler.UserInfo)
			auth.POST("/oauth/stytch/authorize/start", stytchOAuthHandler.AuthorizeStart)
			auth.POST("/oauth/stytch/authorize/submit", stytchOAuthHandler.AuthorizeSubmit)

			auth.GET("/note", noteHandler.Get)
			auth.POST("/note", noteHandler.CreateOrUpdate)
			auth.DELETE("/note", noteHandler.Delete)
			auth.PUT("/note/restore", noteHandler.Restore)
			auth.POST("/note/rename", noteHandler.Rename)
			auth.GET("/notes", noteHandler.List)
			auth.DELETE("/note/recycle-clear", noteHandler.RecycleClear)
			auth.GET("/notes/share-paths", shareHandler.NoteSharePaths)

			auth.GET("/folder", folderHandler.Get)
			auth.POST("/folder", folderHandler.Create)
			auth.DELETE("/folder", folderHandler.Delete)
			auth.GET("/folders", folderHandler.List)
			auth.GET("/folder/notes", folderHandler.ListNotes)
			auth.GET("/folder/files", folderHandler.ListFiles)
			auth.GET("/folder/tree", folderHandler.Tree)

			// Note edit operations
			auth.PATCH("/note/frontmatter", noteHandler.PatchFrontmatter)
			auth.POST("/note/append", noteHandler.Append)
			auth.POST("/note/prepend", noteHandler.Prepend)
			auth.POST("/note/replace", noteHandler.Replace)

			// Note link operations
			auth.GET("/note/backlinks", noteHandler.GetBacklinks)
			auth.GET("/note/outlinks", noteHandler.GetOutlinks)

			auth.GET("/file", fileHandler.GetInfo)
			auth.POST("/file", fileHandler.Upload)
			auth.OPTIONS("/file", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			auth.GET("/file/info", fileHandler.Get)
			auth.OPTIONS("/file/info", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			auth.DELETE("/file", fileHandler.Delete)
			auth.PUT("/file/restore", fileHandler.Restore)
			auth.POST("/file/rename", fileHandler.Rename)
			auth.GET("/files", fileHandler.List)
			auth.DELETE("/file/recycle-clear", fileHandler.RecycleClear)
			auth.OPTIONS("/files", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			auth.GET("/note/history", noteHistoryHandler.Get)
			auth.GET("/note/histories", noteHistoryHandler.List)
			auth.PUT("/note/history/restore", noteHistoryHandler.Restore)

			auth.GET("/setting", settingHandler.Get)
			auth.POST("/setting", settingHandler.CreateOrUpdate)
			auth.DELETE("/setting", settingHandler.Delete)
			auth.POST("/setting/rename", settingHandler.Rename)
			auth.GET("/settings", settingHandler.List)

			// Vault discovery is required before REST clients can address note,
			// file, and folder APIs. Mutating vault operations remain WebGUI-only.
			auth.GET("/vault", vaultHandler.List)

			// WebGUI restricted routes
			// 仅限 WebGUI 访问的路由组
			webguiGroup := auth.Group("")
			webguiGroup.Use(middleware.RequireWebGUI())
			{
				// User management routes
				// 用户管理接口
				webguiGroup.POST("/user/change_password", userHandler.UserChangePassword)

				// Vault management routes
				// 笔记库管理接口
				webguiGroup.POST("/vault", vaultHandler.CreateOrUpdate)
				webguiGroup.DELETE("/vault", vaultHandler.Delete)
				webguiGroup.POST("/vault/rebuild-index", vaultHandler.RebuildIndex)
				webguiGroup.POST("/vault/force-delete-item", vaultHandler.ForceDeleteDataItem)

				// Admin config interface
				// 管理员配置接口
				webguiGroup.GET("/admin/config", adminControlHandler.GetConfig)
				webguiGroup.POST("/admin/config", adminControlHandler.UpdateConfig)
				webguiGroup.GET("/admin/config/user_database", adminControlHandler.GetUserDatabaseConfig)
				webguiGroup.POST("/admin/config/user_database", adminControlHandler.UpdateUserDatabaseConfig)
				webguiGroup.POST("/admin/config/user_database/test", adminControlHandler.ValidateUserDatabaseConfig)
				webguiGroup.GET("/admin/config/cloudflare", adminControlHandler.GetCloudflareConfig)
				webguiGroup.POST("/admin/config/cloudflare", adminControlHandler.UpdateCloudflareConfig)
				webguiGroup.GET("/admin/systeminfo", adminControlHandler.GetSystemInfo)
				webguiGroup.GET("/admin/restart", adminControlHandler.Restart)
				webguiGroup.GET("/admin/gc", adminControlHandler.GC)
				webguiGroup.GET("/admin/cloudflared_tunnel_download", adminControlHandler.CloudflaredTunnelDownload)

				// Admin user managment
				webguiGroup.GET("/admin/users/list", adminControlHandler.GetUsers)
				webguiGroup.POST("/admin/users/create", adminControlHandler.CreateUser)
				webguiGroup.POST("/admin/users/update", adminControlHandler.UpdateUser)

				// Storage management routes
				// 存储配置接口
				webguiGroup.GET("/storage", storageHandler.List)
				webguiGroup.POST("/storage", storageHandler.CreateOrUpdate)
				webguiGroup.GET("/storage/enabled_types", storageHandler.EnabledTypes)
				webguiGroup.POST("/storage/validate", storageHandler.Validate)
				webguiGroup.DELETE("/storage", storageHandler.Delete)

				// Backup routes
				// 本地备份接口
				webguiGroup.GET("/backup/configs", backupHandler.GetConfigs)
				webguiGroup.POST("/backup/config", backupHandler.UpdateConfig)
				webguiGroup.DELETE("/backup/config", backupHandler.DeleteConfig)
				webguiGroup.GET("/backup/historys", backupHandler.ListHistory)
				webguiGroup.POST("/backup/execute", backupHandler.Execute)

				// Git sync routes
				// Git 同步接口
				webguiGroup.GET("/git-sync/configs", gitSyncHandler.GetConfigs)
				webguiGroup.POST("/git-sync/config", gitSyncHandler.UpdateConfig)
				webguiGroup.DELETE("/git-sync/config", gitSyncHandler.DeleteConfig)
				webguiGroup.POST("/git-sync/validate", gitSyncHandler.Validate)
				webguiGroup.DELETE("/git-sync/config/clean", gitSyncHandler.CleanWorkspace)
				webguiGroup.POST("/git-sync/config/execute", gitSyncHandler.Execute)
				webguiGroup.GET("/git-sync/histories", gitSyncHandler.GetHistories)

				// Sync log routes
				// 同步日志路由
				webguiGroup.GET("/sync-logs", syncLogHandler.List)

				// Token management routes
				// 令牌管理路由
				webguiGroup.GET("/tokens", tokenHandler.List)
				webguiGroup.DELETE("/tokens/clean-expired", tokenHandler.CleanExpired)
				webguiGroup.POST("/token", tokenHandler.Create)
				webguiGroup.PUT("/token/:id", tokenHandler.Update)
				webguiGroup.DELETE("/token/:id", tokenHandler.Revoke)
				webguiGroup.POST("/token/:id/rotate", tokenHandler.Rotate)
				webguiGroup.GET("/token/:id/logs", tokenHandler.ListLogs)
			}
		}
	}
}

func oidcCallbackRoute(callbackPath string) string {
	callbackPath = strings.TrimSpace(callbackPath)
	if callbackPath == "" {
		return "/user/auth/oidc/callback"
	}
	if strings.HasPrefix(callbackPath, "/api/") {
		return strings.TrimPrefix(callbackPath, "/api")
	}
	if strings.HasPrefix(callbackPath, "/") {
		return callbackPath
	}
	return "/" + callbackPath
}

func oidcCallbackRoutes(cfg appconfig.OIDCConfig) []string {
	routes := []string{}
	seen := map[string]struct{}{}
	add := func(route string) {
		if _, ok := seen[route]; ok {
			return
		}
		seen[route] = struct{}{}
		routes = append(routes, route)
	}

	add(oidcCallbackRoute(cfg.CallbackPath))
	for _, provider := range cfg.Providers {
		route := oidcCallbackRoute(provider.CallbackPath)
		if route == oidcDefaultProviderCallbackRoute(provider.ID) || strings.Contains(route, ":") {
			continue
		}
		add(route)
	}
	add("/user/auth/oidc/callback/:providerID")
	return routes
}

func oidcDefaultProviderCallbackRoute(providerID string) string {
	if providerID == "" || providerID == "default" {
		return "/user/auth/oidc/callback"
	}
	return "/user/auth/oidc/callback/" + providerID
}
