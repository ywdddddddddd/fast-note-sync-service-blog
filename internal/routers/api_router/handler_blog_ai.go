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

	if params.Routes != nil {
		next := make([]config.AIRouteSetting, 0, len(*params.Routes))
		for i, r := range *params.Routes {
			route := config.AIRouteSetting{}
			// Routes are sent whole, so start from the incoming values.
			// 线路是整表回传的，因此直接以传入值构建。
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
		cfg.Routes = next
	}

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
