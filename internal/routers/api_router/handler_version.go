package api_router

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	pkgapp "github.com/haierkeys/fast-note-sync-service/pkg/app"
	"github.com/haierkeys/fast-note-sync-service/pkg/code"
	"github.com/haierkeys/fast-note-sync-service/pkg/fileurl"
)

// VersionHandler version info API router handler
// VersionHandler 版本信息 API 路由处理器
// Uses App Container to inject dependencies
// 使用 App Container 注入依赖
type VersionHandler struct {
	*Handler
}

// NewVersionHandler creates VersionHandler instance
// NewVersionHandler 创建 VersionHandler 实例
func NewVersionHandler(a *app.App) *VersionHandler {
	return &VersionHandler{
		Handler: NewHandler(a),
	}
}

// ServerVersion retrieves server version information
// @Summary Get server version info
// @Description Get current server software version, Git tag, and build time
// @Tags System
// @Produce json
// @Success 200 {object} pkgapp.Res{data=dto.VersionDTO} "Success"
// @Router /api/version [get]
func (h *VersionHandler) ServerVersion(c *gin.Context) {
	response := pkgapp.NewResponse(c)
	versionInfo := h.App.Version()
	checkInfo := h.App.CheckVersion("")
	response.ToResponse(code.Success.WithData(dto.VersionDTO{
		Version:                          versionInfo.Version,
		GitTag:                           versionInfo.GitTag,
		BuildTime:                        versionInfo.BuildTime,
		VersionIsNew:                     checkInfo.VersionIsNew,
		VersionNewName:                   checkInfo.VersionNewName,
		VersionNewLink:                   checkInfo.VersionNewLink,
		VersionNewChangelog:              checkInfo.VersionNewChangelog,
		VersionNewChangelogContent:       checkInfo.VersionNewChangelogContent,
		VersionHistory:                   checkInfo.VersionHistory,
		PluginVersionNewName:             checkInfo.PluginVersionNewName,
		PluginVersionNewLink:             checkInfo.PluginVersionNewLink,
		PluginVersionNewChangelog:        checkInfo.PluginVersionNewChangelog,
		PluginVersionNewChangelogContent: checkInfo.PluginVersionNewChangelogContent,
		PluginVersionHistory:             checkInfo.PluginVersionHistory,
	}))
}

// ProbeSources probes GitHub and CNB release endpoints in parallel and reports
// reachability + latency for each, plus the recommended source. Used by the
// webgui settings "test latency" panel.
// @Summary Probe version sources latency
// @Description Parallel-probe GitHub and CNB release endpoints, return reachability, latency, recommended source and current selected mode
// @Tags System
// @Produce json
// @Security UserAuthToken
// @Success 200 {object} pkgapp.Res{data=dto.SourceProbeDTO} "Success"
// @Router /api/version/probe [get]
func (h *VersionHandler) ProbeSources(c *gin.Context) {
	response := pkgapp.NewResponse(c)
	snap := h.App.SourceSelector().Probe(c.Request.Context())
	recommended := fileurl.SourceCNB
	if snap.UseGitHub {
		recommended = fileurl.SourceGitHub
	}
	response.ToResponse(code.Success.WithData(dto.SourceProbeDTO{
		GitHub: dto.SourceProbeItem{
			OK:        snap.GitHub.OK,
			LatencyMs: snap.GitHub.LatencyMs,
		},
		CNB: dto.SourceProbeItem{
			OK:        snap.CNB.OK,
			LatencyMs: snap.CNB.LatencyMs,
		},
		Recommended:  recommended,
		SelectedMode: h.App.SourceSelector().Mode(),
	}))
}

// Support retrieves support records by language with pagination and sorting
// Support 分页并排序获取指定语言的打赏记录
// @Summary Get support records
// @Description Get support records for the specified language with pagination and sorting
// @Tags System
// @Produce json
// @Param lang query string false "Language code (default: en)"
// @Param sortBy query string false "Sort by field (amount, time, name, item)"
// @Param sortOrder query string false "Sort order (asc, desc)"
// @Param page query int false "Page number"
// @Param pageSize query int false "Page size"
// @Success 200 {object} pkgapp.Res{data=pkgapp.ListRes} "Success"
// @Router /api/support [get]
func (h *VersionHandler) Support(c *gin.Context) {
	response := pkgapp.NewResponse(c)
	lang := strings.ToLower(c.Query("lang"))
	if lang == "" {
		lang = "en"
	}

	sortBy := c.Query("sortBy")
	sortOrder := c.Query("sortOrder")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	pager := pkgapp.NewPager(c)
	data, total := h.App.GetSupportRecordsPage(lang, sortBy, sortOrder, pager.Page, pager.PageSize)

	// Supporter names are removed before the list leaves the process.
	//
	// This endpoint is unauthenticated (router_api.go registers it outside the auth group)
	// and it publishes records belonging to *third parties* — people who chose to support the
	// upstream plugin's author, not this site's owner, and who agreed to nothing about being
	// listed on someone else's blog. The amount and the message stay; they are what the
	// patrons wrote for public display. The name is what turns a thank-you into a directory.
	//
	// Removed rather than masked: a masked name ("J*") still lets two donations be tied to
	// one person, which is the only thing the field was useful for anyway.
	//
	// 赞助者姓名在列表离开进程之前就被去掉。
	//
	// 这个端点免认证（router_api.go 把它注册在 auth 组之外），而它发布的是**第三方**的记录——
	// 那些人支持的是上游插件作者，不是本站主人，也从未同意被列在别人的博客上。
	// 金额与留言保留：那是赞助者写给公开看的内容。姓名才是把一个致谢变成一份名录的东西。
	//
	// 选择移除而不是打码：打码后的姓名（"J*"）仍让两笔赞助能被关联到同一个人，
	// 而关联性恰恰是这个字段唯一的用途。
	response.ToResponseList(code.Success, withoutSupporterNames(data), total)
}

// withoutSupporterNames returns the records with Name cleared.
//
// A copy, not an in-place edit: the slice comes from the app container's cache, and zeroing
// the field there would erase the names for every future caller too — turning a response
// filter into data loss the first time something else needs it.
//
// The field is cleared rather than the JSON tag changed to "-", because the tag is shared with
// any internal consumer that may still want the name; the redaction belongs at the boundary
// that is actually public.
//
// withoutSupporterNames 返回把 Name 清空后的记录。
//
// 复制而不是就地改：切片来自 app 容器的缓存，那里把字段清零会连将来每个调用方也拿不到姓名——
// 一旦有别的功能需要它，响应过滤就变成了数据丢失。
//
// 选择清空字段而不是把 JSON tag 改成 "-"，因为那个 tag 与可能仍需姓名的内部消费方共享；
// 脱敏应当发生在真正公开的那条边界上。
func withoutSupporterNames(records []pkgapp.SupportRecord) []pkgapp.SupportRecord {
	out := make([]pkgapp.SupportRecord, len(records))
	copy(out, records)
	for i := range out {
		out[i].Name = ""
	}
	return out
}
