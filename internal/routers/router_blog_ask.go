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
