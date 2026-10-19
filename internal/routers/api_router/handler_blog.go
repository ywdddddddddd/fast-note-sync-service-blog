package api_router

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	pkgapp "github.com/haierkeys/fast-note-sync-service/pkg/app"
	"github.com/haierkeys/fast-note-sync-service/pkg/code"
)

// BlogHandler exposes the built-in blog settings to authenticated clients, i.e. the
// Obsidian plugin's "Blog" settings tab.
//
// These endpoints deliberately live in the regular authenticated group rather than the
// WebGUI group: the plugin authenticates with an ObsidianPlugin-scoped token, which the
// WebGUI group rejects. Write access is still limited to the admin user.
//
// BlogHandler 向已认证客户端（Obsidian 插件的“博客”设置页）暴露内置博客设置。
// 这些接口刻意挂在普通认证组而不是 WebGUI 组：插件使用的是 ObsidianPlugin 作用域令牌，
// WebGUI 组会拒绝它。写操作仍然仅限管理员。
type BlogHandler struct {
	App *app.App
}

// NewBlogHandler creates a BlogHandler
// NewBlogHandler 创建 BlogHandler
func NewBlogHandler(appContainer *app.App) *BlogHandler {
	return &BlogHandler{App: appContainer}
}

// isAdmin reports whether the user may change blog settings.
// isAdmin 判断该用户是否有权修改博客设置。
func (h *BlogHandler) isAdmin(uid int64) bool {
	adminUID := h.App.Config().User.AdminUID
	return adminUID == 0 || uid == int64(adminUID)
}

// baseURL resolves the externally reachable base address, mirroring getShareBaseUrl.
// baseURL 解析对外可访问的基础地址，与 getShareBaseUrl 保持一致。
func (h *BlogHandler) baseURL(c *gin.Context) string {
	if v := strings.TrimRight(h.App.Config().Server.ExtApiUrl, "/"); v != "" {
		return v
	}
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

func (h *BlogHandler) snapshot(c *gin.Context) *dto.BlogConfigDTO {
	cfg := &h.App.Config().Blog
	title := strings.TrimSpace(cfg.Title)
	if title == "" {
		title = "Notes"
	}
	uid := cfg.UID
	if uid <= 0 {
		uid = 1
	}
	return &dto.BlogConfigDTO{
		Enabled:   cfg.Enabled != nil && *cfg.Enabled,
		Title:     title,
		Indexable: cfg.Indexable == nil || *cfg.Indexable,
		ShowIndex: cfg.ShowIndex == nil || *cfg.ShowIndex,
		UID:       uid,
		HomeURL:   h.baseURL(c) + "/blog",

		GraphScope:      blogGraphScopeOrDefault(cfg.GraphScope),
		MaxGraphNodes:   cfg.MaxGraphNodes,
		ShowGraph:       cfg.ShowGraph != nil && *cfg.ShowGraph,
		LocalGraphDepth: clampBlogDepth(cfg.LocalGraphDepth),
	}
}

// Valid ranges for the graph settings.
//
// The floor on maxGraphNodes is 10 because a graph below that is not a graph; the ceiling is 400
// because the layout is O(n^2) and this vault's whole published set is 112 notes, so 400 is
// already four times more than any real site of this kind would need.
//
// 图谱设置的有效范围。
//
// maxGraphNodes 的下限是 10，因为比这更小的图不成其为图；上限是 400，因为布局是 O(n²)，
// 而本仓库的已发布全集是 112 篇，因此 400 已经是这类真实站点所需量的四倍。
const (
	blogMinGraphNodes = 10
	blogMaxGraphNodes = 400
	blogMinLocalDepth = 1
	blogMaxLocalDepth = 3
)

// blogGraphScopeOrDefault normalises the scope, defaulting to the private option.
//
// An unset or unrecognised value resolves to "shared" — the restrictive choice. A settings file
// edited by hand should never be able to widen what is published by accident.
//
// blogGraphScopeOrDefault 规范化 scope，默认取更私密的选项。
//
// 未设置或无法识别的值解析为 "shared"——更严格的那个选择。手工编辑过的设置文件不应能意外放宽
// 已发布的可见范围。
func blogGraphScopeOrDefault(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "all":
		return "all"
	default:
		return "shared"
	}
}

// clampBlogDepth keeps the article-graph depth inside the slider's range.
// clampBlogDepth 把文章图谱深度限制在滑块范围内。
func clampBlogDepth(d int) int {
	if d < blogMinLocalDepth {
		return blogMinLocalDepth
	}
	if d > blogMaxLocalDepth {
		return blogMaxLocalDepth
	}
	return d
}

// GetConfig returns the current blog configuration
// @Summary Get blog configuration
// @Description Returns the built-in blog settings. Available to any authenticated user.
// @Tags Blog
// @Security UserAuthToken
// @Success 200 {object} object "Success"
// @Router /api/blog/config [get]
func (h *BlogHandler) GetConfig(c *gin.Context) {
	response := pkgapp.NewResponse(c)
	response.ToResponse(code.Success.WithData(h.snapshot(c)))
}

// UpdateConfig updates blog configuration (admin only)
// @Summary Update blog configuration
// @Description Updates the built-in blog settings and persists them to the config file.
// @Tags Blog
// @Security UserAuthToken
// @Param params body dto.BlogConfigRequest true "Blog settings"
// @Success 200 {object} object "Success"
// @Router /api/blog/config [post]
func (h *BlogHandler) UpdateConfig(c *gin.Context) {
	response := pkgapp.NewResponse(c)
	params := &dto.BlogConfigRequest{}
	if valid, errs := pkgapp.BindAndValid(c, params); !valid {
		response.ToResponse(code.ErrorInvalidParams.WithDetails(errs.ErrorsToString()).WithData(errs.MapsToString()))
		return
	}
	uid := pkgapp.GetUID(c)
	if !h.isAdmin(uid) {
		response.ToResponse(code.ErrorUserIsNotAdmin)
		return
	}

	cfg := &h.App.Config().Blog
	if params.Enabled != nil {
		cfg.Enabled = params.Enabled
	}
	if params.Title != nil {
		cfg.Title = strings.TrimSpace(*params.Title)
	}
	if params.Indexable != nil {
		cfg.Indexable = params.Indexable
	}
	if params.ShowIndex != nil {
		cfg.ShowIndex = params.ShowIndex
	}

	// Graph fields are validated rather than trusted: this endpoint writes a value that is later
	// used as a node budget and a slider bound, and an out-of-range maxGraphNodes would either
	// disable the graph silently (0) or make every render pay for a full-vault layout.
	//
	// 图谱字段经校验而不是直接信任：本端点写入的值之后会被当作节点预算与滑块边界使用，
	// 而越界的 maxGraphNodes 要么静默停用图谱（0），要么让每次渲染都为全仓库布局付出代价。
	if params.GraphScope != nil {
		scope := strings.ToLower(strings.TrimSpace(*params.GraphScope))
		if scope != "shared" && scope != "all" {
			response.ToResponse(code.ErrorInvalidParams.WithDetails(`graphScope must be "shared" or "all"`))
			return
		}
		cfg.GraphScope = scope
	}
	if params.ShowGraph != nil {
		v := *params.ShowGraph
		cfg.ShowGraph = &v
	}
	if params.MaxGraphNodes != nil {
		n := *params.MaxGraphNodes
		if n < blogMinGraphNodes || n > blogMaxGraphNodes {
			response.ToResponse(code.ErrorInvalidParams.WithDetails(
				fmt.Sprintf("maxGraphNodes must be between %d and %d", blogMinGraphNodes, blogMaxGraphNodes)))
			return
		}
		cfg.MaxGraphNodes = n
	}
	if params.LocalGraphDepth != nil {
		d := *params.LocalGraphDepth
		if d < blogMinLocalDepth || d > blogMaxLocalDepth {
			response.ToResponse(code.ErrorInvalidParams.WithDetails(
				fmt.Sprintf("localGraphDepth must be between %d and %d", blogMinLocalDepth, blogMaxLocalDepth)))
			return
		}
		cfg.LocalGraphDepth = d
	}

	// Persist so the setting survives a restart.
	// 持久化，确保重启后设置仍然生效。
	if err := h.App.Config().Save(); err != nil {
		response.ToResponse(code.Failed.WithDetails(err.Error()))
		return
	}

	response.ToResponse(code.SuccessUpdate.WithData(h.snapshot(c)))
}

// GetAccessLog returns a page of blog visits plus an aggregate summary (admin only).
//
// Admin-only because this is the one endpoint that describes the site's readers. The summary
// exists so the panel rarely needs the raw page: totals and top paths are what an owner reads,
// and handing over fewer raw records is itself a privacy measure.
//
// GetAccessLog 返回一页博客访问记录以及聚合摘要（仅管理员）。
//
// 仅管理员，因为这是唯一一个描述站点读者的端点。摘要的存在使面板很少需要原始分页：
// 主人真正要看的是总计与热门路径，而少交出原始记录本身就是一种隐私措施。
//
// @Summary List blog access log
// @Description Returns recent blog visits and an aggregate summary. Admin only.
// @Tags Blog
// @Security UserAuthToken
// @Param days query int false "Days to look back (1..30, default 7)"
// @Param limit query int false "Max entries (1..500, default 100)"
// @Success 200 {object} object "Success"
// @Router /api/blog/access-log [get]
func (h *BlogHandler) GetAccessLog(c *gin.Context) {
	response := pkgapp.NewResponse(c)

	uid := pkgapp.GetUID(c)
	if !h.isAdmin(uid) {
		response.ToResponse(code.ErrorUserIsNotAdmin)
		return
	}
	if h.App.ShareService == nil {
		response.ToResponse(code.ErrorServerInternal)
		return
	}

	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	page, err := h.App.ShareService.GetBlogAccessLog(c.Request.Context(), days, limit)
	if err != nil {
		response.ToResponse(code.Failed.WithDetails(err.Error()))
		return
	}
	response.ToResponse(code.Success.WithData(page))
}

// ListPosts lists published blog posts with absolute URLs
// @Summary List published blog posts
// @Description Returns the notes currently published on the built-in blog, with absolute URLs.
// @Tags Blog
// @Security UserAuthToken
// @Success 200 {object} object "Success"
// @Router /api/blog/posts [get]
func (h *BlogHandler) ListPosts(c *gin.Context) {
	response := pkgapp.NewResponse(c)
	if h.App.ShareService == nil {
		response.ToResponse(code.Failed.WithDetails("share service unavailable"))
		return
	}

	cfg := &h.App.Config().Blog
	uid := cfg.UID
	if uid <= 0 {
		uid = 1
	}

	entries, err := h.App.ShareService.ListBlogEntries(c.Request.Context(), uid)
	if err != nil {
		response.ToResponse(code.Failed.WithDetails(err.Error()))
		return
	}

	base := h.baseURL(c)
	posts := make([]*dto.BlogPostItem, 0, len(entries))
	for _, e := range entries {
		posts = append(posts, &dto.BlogPostItem{
			Title:     e.Title,
			URL:       base + "/blog/" + e.Slug,
			Path:      e.Path,
			CreatedAt: e.CreatedAt,
		})
	}
	response.ToResponse(code.Success.WithData(map[string]any{
		"list":    posts,
		"total":   len(posts),
		"homeUrl": base + "/blog",
		"enabled": cfg.Enabled != nil && *cfg.Enabled,
		"title":   h.snapshot(c).Title,
	}))
}
