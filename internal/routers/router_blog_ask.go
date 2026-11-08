package routers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		c.Header("Cache-Control", "no-cache")
		c.String(http.StatusOK, blogAskScript)
	})

	// Code blocks: the frame, language label, gutter and copy button. Gated by the blog switch
	// only — it decorates markup that is already there and needs no upstream configuration.
	//
	// 代码块：外框、语言标签、行号槽与复制按钮。只受博客总开关约束——
	// 它装饰的是本来就存在的标记，不需要任何上游配置。
	r.GET("/blog/code.js", func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.String(http.StatusOK, blogCodeJS())
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
		c.Header("Cache-Control", "no-cache")
		c.String(http.StatusOK, blogLightboxScript)
	})

	// The subscribe dialog. Gated by the blog switch only: it needs no upstream configuration and
	// no indexable flag of its own — the entry point that uses it is already conditional on
	// blog.indexable in the template, so a page that hides the button never requests this file.
	//
	// 订阅弹层。只受博客总开关约束：它不需要任何上游配置，也不需要自己的收录开关——
	// 使用它的入口在模板里已经依赖 blog.indexable，因此隐藏了按钮的页面不会请求这个文件。
	r.GET("/blog/feed.js", func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.String(http.StatusOK, blogFeedScript)
	})

	// Home page view switching. Gated by the blog switch only, like the two above: it is pure
	// interaction, and standing it behind the Q&A flag would make the cover grid and the wheel
	// disappear whenever the model was off.
	//
	// 首页视图切换。与上面两个一样只受博客总开关约束：它是纯交互，
	// 把它挂在问答开关后面意味着模型一关，封面网格与同心轮就一起消失。
	r.GET("/blog/views.js", func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.String(http.StatusOK, blogViewsScript)
	})

	// Bottom-sheet dragging. Gated by the blog switch only, for the same reason as the
	// lightbox: it is pure interaction, and tying it to the Q&A flag would make every drawer
	// stop dragging whenever the model was off.
	//
	// 底部 sheet 拖拽。与灯箱同理只受博客总开关约束：它是纯交互，
	// 绑到问答开关上会导致模型一关、所有抽屉都不能拖。
	r.GET("/blog/sheet.js", func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.String(http.StatusOK, blogSheetScript)
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
		// The concurrency gate has two dimensions now. The per-address one says "wait for your
		// own answer"; the global one says "the blog is busy" and is the capacity limit that
		// replaced the old single slot. A hung request can no longer hold either forever: the
		// handler always returns, and runAskStream below bounds how long it can stay.
		//
		// 两道滥用闸，从便宜的开始，两者都以真实客户端地址为键，而不是以客户端能改写的头为键
		// （见 clientIdentityResolver）。
		//
		// 先频次、后并发：一个在猛刷端点的访客，即使服务空闲也该被告知放慢；
		// 而一个已有回答在途的访客应该被告知这一具体事实，而不是被当成洪水。
		//
		// 并发闸现在有两个维度。按地址的那条说的是「等自己的回答结束」；全站的这条说的是
		// 「博客正忙」，它是取代旧单槽的容量上限。挂起的请求再也占不住任何一条：处理器总会返回，
		// 而下面的 runAskStream 限定它能挂多久。
		clientKey := ident.identity(c)
		if !blogGuardInstance.allowAsk(clientKey) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code": 429, "message": blogAskMsgTooFrequent})
			return
		}
		releaseGen, slot := blogGuardInstance.tryLockGeneration(clientKey)
		switch slot {
		case askSlotBusySelf:
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code": 429, "message": blogAskMsgBusySelf})
			return
		case askSlotBusyGlobal:
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code": 429, "message": blogAskMsgBusyGlobal})
			return
		}
		// Released when the handler returns, which is exactly when the answer finishes
		// streaming — shareSvc.Ask below is synchronous and returns only after the last SSE
		// frame or after the deadline interrupts it. Holding it any longer would be impossible;
		// holding it shorter would let the same visitor open two paid generations at once.
		//
		// 在处理器返回时释放，而那正是回答流式输出结束的时刻——下面的 shareSvc.Ask 是同步的，
		// 直到最后一帧 SSE 或截止时间打断它才返回。持有更久做不到；持有更短则会让同一位访客
		// 同时开两个付费生成。
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
		// ResponseController reaches the underlying connection through gin's Unwrap, so each
		// SSE write carries its own deadline. Without it, a client that stops reading can block
		// Write while holding a global slot, and the context deadline below cannot unblock it.
		//
		// ResponseController 经 gin 的 Unwrap 抵达底层连接，因此每次 SSE 写都有自己的截止时间。
		// 没有它，停止读取的客户端会在占着全站名额时阻塞 Write，而下面的上下文截止无法解开它。
		rc := http.NewResponseController(w)

		writeEvent := func(ev service.AskEvent) error {
			_ = rc.SetWriteDeadline(time.Now().Add(blogAskWriteTimeout))
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

		// The answer runs under the handler's wall-clock budget. The route-level timeout still
		// governs a single upstream call; this deadline caps the whole exchange, including
		// retrieval, failover between routes, and any upstream that ignores its own timeout.
		//
		// 回答在处理器的墙钟预算下运行。线路级超时仍管单次上游调用；这个截止时间约束整场交互，
		// 包括检索、线路间故障转移，以及任何无视自身超时的上游。
		runAskStream(c.Request.Context(), blogGuardInstance.askBudget(),
			func(ctx context.Context) error {
				return shareSvc.Ask(ctx, opts, writeEvent)
			}, writeEvent)
	})
}

// runAskStream runs one answer under a wall-clock budget and renders the deadline as a final
// SSE frame.
//
// The status code is already committed by the time the first stage event is written, so a
// timeout cannot become a 504. What it can be is a last `error` event followed by the handler
// returning, which ends the response; the browser's reader then completes and its cleanup path
// runs exactly as it does after a normal `done`.
//
// A deadline is not an ordinary error: the pipeline may have already streamed half an answer,
// so the closing message must say the answer was cut short rather than print a transport error.
// The deadline is therefore checked before the returned error, because the service often turns
// a context cancellation into its own `error` frame and still returns nil; the timeout sentence
// must be the last thing the reader sees either way. A parent cancellation is different again —
// that means the reader closed the tab, and there is nobody left to read anything.
//
// The budget is passed in rather than read from the constant so tests can inject milliseconds
// and exercise the deadline path without sleeping for a minute.
//
// runAskStream 在墙钟预算下运行一次回答，并把截止时间渲染成最后一帧 SSE。
//
// 第一帧 stage 写出时状态码就已经提交，因此超时不可能变成 504。它能做的是发最后一帧
// `error` 然后让处理器返回，从而结束响应；浏览器的流读取器随后正常完成，清理路径与收到
// `done` 后完全一样。
//
// 截止不是普通错误：管线可能已经输出了一半答案，所以收尾文案必须说「回答被中断」，
// 而不是打印一个传输错误。父上下文被取消又是另一回事——那意味着读者关掉了标签页，
// 已经没有人会读任何东西。
//
// 预算以参数传入而不是直接读常量，使测试能注入毫秒级时长、走一遍截止路径，而不必真等一分钟。
func runAskStream(parent context.Context, timeout time.Duration, ask func(context.Context) error,
	writeEvent func(service.AskEvent) error) {

	if timeout <= 0 {
		timeout = blogAskTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	err := ask(ctx)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		secs := int(timeout.Round(time.Second) / time.Second)
		if secs < 1 {
			secs = 1
		}
		_ = writeEvent(service.AskEvent{
			Type:    "error",
			Message: fmt.Sprintf("这个问题想了超过 %d 秒，已自动中断，请稍后再问一次", secs),
		})
		return
	}
	if err == nil {
		return
	}
	if parent.Err() != nil {
		return
	}
	_ = writeEvent(service.AskEvent{Type: "error", Message: err.Error()})
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
			ThinkingLevel:  r.ThinkingLevel,
			Headers:        r.Headers,
			ExtraBody:      r.ExtraBody,
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
