package routers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
	"github.com/haierkeys/fast-note-sync-service/internal/config"
	"github.com/haierkeys/fast-note-sync-service/internal/service"
)

// Blog Q&A endpoint.
//
// Server-Sent Events rather than a WebSocket: the exchange is one question in, one streamed
// answer out, and SSE survives the plain HTTP proxies a self-hosted blog usually sits behind
// without needing an upgrade handshake.
//
// 博客问答端点。
//
// 使用 Server-Sent Events 而非 WebSocket：交互形态是「一问、一答、流式输出」，
// 且 SSE 能穿过自建博客常见的普通 HTTP 反代，无需升级握手。
func registerBlogAskRoutes(r *gin.Engine, appContainer *app.App) {
	shareSvc := appContainer.ShareService
	if shareSvc == nil {
		return
	}

	// Compiled once here rather than per request: the trusted-proxy list is startup
	// configuration, and parsing CIDRs on every question would be work done for no benefit.
	// 在这里编译一次而不是每个请求编译：可信代理列表是启动期配置，每个问题都去解析 CIDR 是无谓开销。
	ident := newClientIdentityResolver(appContainer.Config().Server.TrustedProxies)

	r.GET("/blog/ask.js", func(c *gin.Context) {
		cfg, ok := blogEnabled(c, appContainer)
		if !ok {
			return
		}
		if !blogAskEnabled(appContainer) {
			c.String(http.StatusNotFound, "// blog ask is disabled")
			return
		}
		_ = cfg
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "public, max-age=300")
		c.String(http.StatusOK, blogAskScript)
	})

	// The lightbox is gated by the blog switch only. It needs no upstream configuration, so
	// tying it to the Q&A flag would make the images stop opening whenever the model is off.
	// 灯箱只受博客总开关约束。它不需要任何上游配置，把它绑在问答开关上会导致
	// 模型一关、图片就打不开。
	r.GET("/blog/lightbox.js", func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		// Short cache instead of a versioned URL: the templates are plain string literals, and
		// splicing a version into them would mean cutting the literal apart at two places for a
		// benefit max-age already delivers. Five minutes matches ask.js and bounds how long a
		// stale viewer can survive a deploy.
		// 用短缓存而不是版本化 URL：模板是纯字符串字面量，为拼版本号得在两处把字面量割开，
		// 而 max-age 已经给到同样的效果。五分钟与 ask.js 一致，也界定了部署后旧查看器
		// 最长能存活多久。
		c.Header("Cache-Control", "public, max-age=300")
		c.String(http.StatusOK, blogLightboxScript)
	})

	r.POST("/api/blog/ask", func(c *gin.Context) {
		_, ok := blogEnabled(c, appContainer)
		if !ok {
			return
		}

		aiCfg := appContainer.Config().AI
		if !aiCfg.AIEnabled() {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "blog ask is disabled"})
			return
		}

		// Abuse gates, cheapest first, both keyed on the real client address rather than on a
		// header the client can rewrite (see clientIdentityResolver).
		//
		// Frequency before concurrency: a visitor hammering the endpoint should be told to slow
		// down even when the service is idle, and a visitor with one answer in flight should be
		// told that specifically rather than being counted as a flood.
		//
		// 两道滥用闸，从便宜的开始，两者都以真实客户端地址为键，而不是以客户端能改写的头为键
		// （见 clientIdentityResolver）。
		//
		// 先频次、后并发：一个在猛刷端点的访客，即使服务空闲也该被告知放慢；
		// 而一个已有回答在途的访客应该被告知这一具体事实，而不是被当成洪水。
		clientKey := ident.identity(c)
		if !blogGuardInstance.allowAsk(clientKey) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code": 429, "message": "提问太频繁，请稍等一会儿再问"})
			return
		}
		releaseGen := blogGuardInstance.tryLockGeneration(clientKey)
		if releaseGen == nil {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code": 429, "message": "您上一个问题还在回答，请等它结束"})
			return
		}
		// Released when the handler returns, which is exactly when the answer finishes
		// streaming — shareSvc.Ask below is synchronous and returns only after the last SSE
		// frame. Holding it any longer would be impossible; holding it shorter would let the
		// same visitor open two paid generations at once.
		//
		// 在处理器返回时释放，而那正是回答流式输出结束的时刻——下面的 shareSvc.Ask 是同步的，
		// 直到最后一帧 SSE 才返回。持有更久做不到；持有更短则会让同一位访客同时开两个付费生成。
		defer releaseGen()

		var req struct {
			Question string `json:"question"`
			History  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"history"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求体不是合法 JSON"})
			return
		}

		routes := buildAskRoutes(aiCfg)

		// The upstream call can outlive the browser's patience; the per-route timeout governs,
		// so the request context is used only to detect client disconnects.
		opts := service.AskOptions{
			UID:              blogUID(appContainer.Config().Blog.UID),
			SiteName:         blogSiteTitle(appContainer.Config().Blog.Title),
			Question:         req.Question,
			Routes:           routes,
			TopK:             aiCfg.TopK,
			MaxContextChars:  aiCfg.MaxContextChars,
			CacheTTL:         parseDurationOr(aiCfg.CacheTTL, 5*time.Minute),
			SystemPrompt:     aiCfg.SystemPrompt,
			MaxQuestionChars: aiCfg.MaxQuestionChars,
			Embedding:        buildAskEmbedding(aiCfg),
			Rerank:           buildAskRerank(aiCfg),
		}

		c.Header("Content-Type", "text/event-stream; charset=utf-8")
		c.Header("Cache-Control", "no-cache, no-transform")
		c.Header("Connection", "keep-alive")
		// Nginx buffers proxied responses by default, which would defeat streaming entirely.
		// 默认情况下 Nginx 会缓冲反代响应，那会让流式输出彻底失效。
		c.Header("X-Accel-Buffering", "no")

		flusher, canFlush := c.Writer.(http.Flusher)
		w := c.Writer

		writeEvent := func(ev service.AskEvent) error {
			payload, err := json.Marshal(ev)
			if err != nil {
				return err
			}
			if _, err := w.Write([]byte("data: ")); err != nil {
				return err
			}
			if _, err := w.Write(payload); err != nil {
				return err
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return err
			}
			if canFlush {
				flusher.Flush()
			}
			return nil
		}

		err := shareSvc.Ask(c.Request.Context(), opts, writeEvent)
		if err != nil && c.Request.Context().Err() == nil {
			_ = writeEvent(service.AskEvent{Type: "error", Message: err.Error()})
		}
	})
}

// blogAskEnabled reports whether the Q&A entry is switched on.
// blogAskEnabled 返回问答入口是否已开启。
func blogAskEnabled(appContainer *app.App) bool {
	return appContainer.Config().AI.AIEnabled()
}

// buildAskRoutes turns config into ordered service routes.
//
// Incomplete entries (no base URL, no key, no model) are dropped rather than failing at call
// time, so a half-filled config degrades to "route unavailable" instead of a broken feature.
//
// buildAskRoutes 把配置转换为有序的服务线路。
//
// 缺少 base-url、api-key 或 model 的条目会被直接丢弃，而不是等到调用时才失败，
// 因此填了一半的配置只会退化为「该线路不可用」，而不会让整个功能坏掉。
func buildAskRoutes(cfg config.AISettings) []service.AskRoute {
	out := make([]service.AskRoute, 0, len(cfg.Routes))
	for _, r := range cfg.Routes {
		if !r.AIRouteEnabled() {
			continue
		}
		base := strings.TrimSpace(r.BaseURL)
		model := strings.TrimSpace(r.Model)
		key := strings.TrimSpace(r.APIKey)
		if base == "" || model == "" || key == "" {
			continue
		}
		name := strings.TrimSpace(r.Name)
		if name == "" {
			name = model
		}
		out = append(out, service.AskRoute{
			Name:        name,
			BaseURL:     base,
			APIKey:      key,
			Model:       model,
			MinInterval: parseDurationOr(r.MinInterval, 0),
			Timeout:     parseDurationOr(r.Timeout, 300*time.Second),

			Temperature:      r.Temperature,
			TopP:             r.TopP,
			TopK:             r.TopK,
			PresencePenalty:  r.PresencePenalty,
			FrequencyPenalty: r.FrequencyPenalty,
			MinP:             r.MinP,

			MaxTokens:      r.AIRouteMaxTokens(),
			EnableThinking: r.AIRouteThinking(),
			Retries:        r.AIRouteRetries(),
		})
	}
	return out
}

func parseDurationOr(v string, fallback time.Duration) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return fallback
	}
	return d
}

// buildAskEmbedding returns the vector channel config, or nil when it is not usable.
//
// A nil result is not an error: the retrieval pipeline degrades to BM25, so an operator who
// has not configured an embedding endpoint still gets a working (if weaker) Q&A entry.
//
// buildAskEmbedding 返回向量通道配置，不可用时返回 nil。
//
// 返回 nil 不是错误：检索管线会退化到 BM25，因此没有配置嵌入端点的运维者仍然得到
// 一个可用（虽然更弱）的问答入口。
func buildAskEmbedding(cfg config.AISettings) *service.AskEmbeddingConfig {
	if !cfg.AIEmbeddingEnabled() {
		return nil
	}
	return &service.AskEmbeddingConfig{
		BaseURL:        strings.TrimSpace(cfg.Embedding.BaseURL),
		APIKey:         strings.TrimSpace(cfg.Embedding.APIKey),
		Model:          strings.TrimSpace(cfg.Embedding.Model),
		BatchSize:      cfg.Embedding.BatchSize,
		ChunkChars:     cfg.Embedding.ChunkChars,
		ChunkOverlap:   cfg.Embedding.ChunkOverlap,
		CandidateLimit: cfg.Embedding.CandidateLimit,
	}
}

// buildAskRerank returns the reranker config, or nil when it is not usable.
// buildAskRerank 返回重排器配置，不可用时返回 nil。
func buildAskRerank(cfg config.AISettings) *service.AskRerankConfig {
	if !cfg.AIRerankEnabled() {
		return nil
	}
	return &service.AskRerankConfig{
		BaseURL:  strings.TrimSpace(cfg.Rerank.BaseURL),
		APIKey:   strings.TrimSpace(cfg.Rerank.APIKey),
		Model:    strings.TrimSpace(cfg.Rerank.Model),
		TopN:     cfg.Rerank.TopN,
		DocChars: cfg.Rerank.DocChars,
	}
}
