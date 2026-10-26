package api_router

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/config"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	pkgapp "github.com/haierkeys/fast-note-sync-service/pkg/app"
	"github.com/haierkeys/fast-note-sync-service/pkg/code"
)

// AIHandler exposes the blog Q&A configuration to the Obsidian plugin's settings tab.
//
// It lives beside BlogHandler and follows the same rules: the regular authenticated group
// (the plugin's ObsidianPlugin-scoped token is rejected by the WebGUI group), reads open to
// any authenticated user, writes restricted to the admin.
//
// AIHandler 向 Obsidian 插件的设置页暴露博客问答配置。
//
// 与 BlogHandler 并列并遵循同样的规则：挂在普通认证组（插件的 ObsidianPlugin 作用域令牌会被
// WebGUI 组拒绝），读取对任意已认证用户开放，写入仅限管理员。

// GetAIConfig returns the current AI configuration
// @Summary Get blog Q&A configuration
// @Description Returns the AI section used by the built-in blog's Q&A entry.
// @Tags Blog
// @Security UserAuthToken
// @Success 200 {object} object "Success"
// @Router /api/blog/ai-config [get]
func (h *BlogHandler) GetAIConfig(c *gin.Context) {
	response := pkgapp.NewResponse(c)
	response.ToResponse(code.Success.WithData(h.aiSnapshot(c)))
}

// UpdateAIConfig updates the AI configuration
// @Summary Update blog Q&A configuration
// @Description Updates the AI section and persists it to the config file.
// @Tags Blog
// @Security UserAuthToken
// @Param params body dto.AIConfigRequest true "AI settings"
// @Success 200 {object} object "Success"
// @Router /api/blog/ai-config [post]
func (h *BlogHandler) UpdateAIConfig(c *gin.Context) {
	response := pkgapp.NewResponse(c)
	params := &dto.AIConfigRequest{}
	if valid, errs := pkgapp.BindAndValid(c, params); !valid {
		response.ToResponse(code.ErrorInvalidParams.WithDetails(errs.ErrorsToString()).WithData(errs.MapsToString()))
		return
	}
	uid := pkgapp.GetUID(c)
	if !h.isAdmin(uid) {
		response.ToResponse(code.ErrorUserIsNotAdmin)
		return
	}

	cfg := &h.App.Config().AI

	if params.Enabled != nil {
		cfg.Enabled = params.Enabled
	}
	if params.TopK != nil && *params.TopK > 0 {
		cfg.TopK = *params.TopK
	}
	if params.MaxContextChars != nil && *params.MaxContextChars > 0 {
		cfg.MaxContextChars = *params.MaxContextChars
	}
	if params.CacheTTL != nil {
		cfg.CacheTTL = strings.TrimSpace(*params.CacheTTL)
	}
	if params.MaxQuestionLen != nil && *params.MaxQuestionLen > 0 {
		cfg.MaxQuestionChars = *params.MaxQuestionLen
	}
	if params.SystemPrompt != nil {
		cfg.SystemPrompt = *params.SystemPrompt
	}

	if params.Routes != nil {		cfg.Routes = mergeAIRoutes(cfg.Routes, *params.Routes)	}

	// Persist so the setting survives a restart.
	// 持久化，确保重启后设置仍然生效。
	if err := h.App.Config().Save(); err != nil {
		response.ToResponse(code.Failed.WithDetails(err.Error()))
		return
	}

	response.ToResponse(code.SuccessUpdate.WithData(h.aiSnapshot(c)))
}

func (h *BlogHandler) aiSnapshot(c *gin.Context) *dto.AIConfigDTO {
	cfg := &h.App.Config().AI

	topK := cfg.TopK
	if topK <= 0 {
		topK = 6
	}
	maxCtx := cfg.MaxContextChars
	if maxCtx <= 0 {
		maxCtx = 14000
	}
	maxQ := cfg.MaxQuestionChars
	if maxQ <= 0 {
		maxQ = 500
	}
	ttl := strings.TrimSpace(cfg.CacheTTL)
	if ttl == "" {
		ttl = "5m"
	}

	routes := make([]dto.AIRouteDTO, 0, len(cfg.Routes))
	for _, r := range cfg.Routes {
		routes = append(routes, dto.AIRouteDTO{
			Name:        r.Name,
			BaseURL:     r.BaseURL,
			APIKey:      r.APIKey,
			Model:       r.Model,
			MinInterval: r.MinInterval,
			Timeout:     r.Timeout,
			Enabled:     r.AIRouteEnabled(),
		})
	}

	return &dto.AIConfigDTO{
		Enabled:         cfg.AIEnabled(),
		TopK:            topK,
		MaxContextChars: maxCtx,
		CacheTTL:        ttl,
		MaxQuestionLen:  maxQ,
		SystemPrompt:    cfg.SystemPrompt,
		Routes:          routes,
		HomeURL:         h.baseURL(c) + "/blog",
		AskURL:          h.baseURL(c) + "/api/blog/ask",
	}
}

// itoa avoids pulling strconv in for one call site.
// itoa 避免为一个调用点引入 strconv。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// mergeAIRoutes applies a partial route list on top of the existing one.
//
// 每条线路都以**现有配置为底**，只覆盖请求里真的给出的字段。
//
// 为什么必须这样：设置页并不展示 headers / thinking-level / enable-thinking /
// max-tokens / temperature / retries，它回传的线路表里没有这些字段。
// 若从零构造，这些"界面上看不见"的设置就会在每一次保存后消失，
// 而紧接着的 Config().Save() 会把这份残缺配置写回磁盘 ——
// 用户看到的是"我什么都没改，线路就挂了"。
//
// 2026-10-26 实例：OpenCode 的 x-opencode-session 被清成 headers: {}，
// 网关回 400 MissingSessionID，全站问答不可用，而 api-key 与 base-url 都还在，
// 配置文件看上去像没被动过。
//
// 线路身份取 base-url：它决定用哪套上游协议（自定义头与 thinking 形态都绑在它上面）。
// 找不到同 base-url 的旧线路时退回同下标，再不行才从零开始。
//
// mergeAIRoutes applies a partial route list on top of the existing one so that fields the
// settings UI does not expose (headers, thinking level, sampling params, retries) survive a save.
func mergeAIRoutes(old []config.AIRouteSetting, in []dto.AIRouteRequest) []config.AIRouteSetting {
	byBase := make(map[string]config.AIRouteSetting, len(old))
	for _, r := range old {
		key := strings.TrimSpace(r.BaseURL)
		if key == "" {
			continue
		}
		if _, exists := byBase[key]; !exists {
			byBase[key] = r
		}
	}

	next := make([]config.AIRouteSetting, 0, len(in))
	for i, r := range in {
		var route config.AIRouteSetting
		if r.BaseURL != nil {
			if prev, ok := byBase[strings.TrimSpace(*r.BaseURL)]; ok {
				route = prev
			}
		} else if i < len(old) {
			route = old[i]
		}

		if r.Name != nil {
			route.Name = strings.TrimSpace(*r.Name)
		}
		if r.BaseURL != nil {
			route.BaseURL = strings.TrimSpace(*r.BaseURL)
		}
		if r.APIKey != nil {
			route.APIKey = strings.TrimSpace(*r.APIKey)
		}
		if r.Model != nil {
			route.Model = strings.TrimSpace(*r.Model)
		}
		if r.MinInterval != nil {
			route.MinInterval = strings.TrimSpace(*r.MinInterval)
		}
		if r.Timeout != nil {
			route.Timeout = strings.TrimSpace(*r.Timeout)
		}
		if r.Enabled != nil {
			route.Enabled = r.Enabled
		}

		// An entry with no address and no model carries no information; drop it rather than
		// leaving a row that can never succeed.
		// 既无地址也无模型的条目没有信息量，直接丢弃，避免留下一条永远不会成功的线路。
		if route.BaseURL == "" && route.Model == "" && route.APIKey == "" {
			continue
		}
		if route.Name == "" {
			route.Name = "route-" + strings.TrimSpace(itoa(i+1))
		}
		next = append(next, route)
	}
	return next
}
