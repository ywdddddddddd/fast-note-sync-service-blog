package routers

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
)

// Graph fragment endpoint.
//
// The full graph is ~113 nodes; inlining it into every article page would add roughly 45 KB per
// response for a component most readers never open. This endpoint hands it out on demand instead,
// which is also what makes the depth slider practical: each depth is a separate request rather
// than three pre-rendered copies in the document.
//
// It returns a fragment, not a page: the caller injects it into an existing document. Nothing
// here is a full HTML response, so the endpoint has no layout, no navigation and no error page —
// on failure it answers with a plain text status, because there is no page to render an error
// into.
//
// 图谱片段端点。
//
// 全站图谱约 113 个节点；把它内联进每个文章页，会为大多数读者从不打开的一个组件
// 在每次响应里多塞约 45 KB。本端点改为按需发放；这也是深度滑块可行的前提：
// 每个深度是一次独立请求，而不是在文档里预渲染三份副本。
//
// 它返回的是片段而不是页面：由调用方注入既有文档。这里没有任何完整 HTML 响应，
// 因此没有布局、没有导航、也没有错误页——失败时以纯文本状态作答，因为没有可渲染错误页的容器。

// graphFragmentQuery is the parsed, validated request.
// graphFragmentQuery 是解析并校验后的请求。
type graphFragmentQuery struct {
	Scope string
	Slug  string
	Depth int
}

// parseGraphFragmentQuery validates the request parameters.
//
// Unknown scopes are rejected rather than defaulted: silently answering a typo with the global
// graph would look like it worked while doing the wrong thing.
//
// parseGraphFragmentQuery 校验请求参数。
//
// 未知的 scope 会被拒绝而不是回落到默认值：把拼错的值静默当作全站图，会让它看起来正常工作，
// 实际做的却是另一件事。
func parseGraphFragmentQuery(c *gin.Context) (graphFragmentQuery, bool) {
	q := graphFragmentQuery{
		Scope: strings.ToLower(strings.TrimSpace(c.Query("scope"))),
		Slug:  strings.TrimSpace(c.Query("slug")),
		Depth: 1,
	}
	switch q.Scope {
	case "global":
	case "local":
		if q.Slug == "" {
			return q, false
		}
		if d := c.Query("depth"); d != "" {
			n := 0
			for _, r := range d {
				if r < '0' || r > '9' {
					return q, false
				}
				n = n*10 + int(r-'0')
				if n > 100 {
					return q, false
				}
			}
			if n > 0 {
				q.Depth = n
			}
		}
	default:
		return q, false
	}
	return q, true
}

// serveGraphFragment answers the graph fragment request.
//
// serveGraphFragment 响应图谱片段请求。
func serveGraphFragment(c *gin.Context, appContainer *app.App) {
	cfg, ok := blogEnabled(c, appContainer)
	if !ok {
		return
	}
	// The endpoint is only meaningful when the graph is enabled. When it is off, the page never
	// renders the buttons that would call it, so this is a consistency guard rather than a
	// user-visible state.
	//
	// 该端点只在图谱开启时有意义。关闭时页面根本不会渲染出调用它的按钮，
	// 因此这里是致性守卫，而不是用户可见的状态。
	if !cfg.ShowGraph {
		c.String(http.StatusNotFound, "graph is disabled")
		return
	}

	shareSvc := appContainer.ShareService
	if shareSvc == nil {
		c.String(http.StatusServiceUnavailable, "share service unavailable")
		return
	}

	q, valid := parseGraphFragmentQuery(c)
	if !valid {
		c.String(http.StatusBadRequest, "invalid graph query")
		return
	}

	maxNodes := cfg.MaxGraphNodes
	if maxNodes <= 0 {
		maxNodes = 120
	}

	// renderGraphSVG already returns trusted markup produced from server-side data.
	// renderGraphSVG 返回的是由服务端数据生成的可信标记。
	var svg template.HTML
	var note string
	var graphJSON template.JS

	switch q.Scope {
	case "global":
		base := blogBaseURL(c, appContainer)
		publishedOnly := !strings.EqualFold(strings.TrimSpace(cfg.GraphScope), "all")
		home, err := shareSvc.GetBlogHome(c.Request.Context(), blogUID(cfg.UID), base, publishedOnly, maxNodes)
		if err != nil {
			c.String(http.StatusInternalServerError, "graph unavailable")
			return
		}
		if home.Graph == nil || len(home.Graph.Nodes) == 0 {
			c.String(http.StatusNotFound, "graph is empty")
			return
		}
		svg, note = renderGraphSVG(home.Graph)
		graphJSON = graphDataJSON(home.Graph)

	case "local":
		graph, err := shareSvc.GetLocalGraph(c.Request.Context(), q.Slug, q.Depth, maxNodes)
		if err != nil {
			c.String(http.StatusNotFound, "graph unavailable")
			return
		}
		if graph == nil || len(graph.Nodes) == 0 {
			c.String(http.StatusNotFound, "graph is empty")
			return
		}
		svg, note = renderGraphSVG(graph)
		graphJSON = graphDataJSON(graph)
	}

	// format=json hands back the island the client simulation reads, instead of a rendered SVG.
	//
	// The fullscreen layer needs the full-site graph, and inlining that into every article would
	// cost every reader ~45 KB for a view most never open. Requesting it here keeps the page small
	// and the layer instant after its first open. The card's own three local islands stay inline
	// because they are small and the slider must feel immediate.
	//
	// format=json 返回客户端模拟读取的数据岛，而不是渲染好的 SVG。
	//
	// 全屏浮层需要全站图，而把它内联进每篇文章会让每个读者为大多数人不打开的视图付约 45 KB。
	// 在这里按需请求既保持页面小，又让浮层在首次打开后立刻可用。
	// 卡片自身的三份局部数据岛仍然内联，因为它们很小，而且滑块必须跟手。
	if strings.EqualFold(strings.TrimSpace(c.Query("format")), "json") {
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusOK)
		_, _ = c.Writer.WriteString(string(graphJSON))
		return
	}

	body := `<div class="graph-frag">` + string(svg) +
		`<p class="graph-note">` + template.HTMLEscapeString(note) + `</p></div>`

	// The fragment depends on live configuration and on which notes are currently shared, so it
	// must not be cached by an intermediary. The browser may keep it briefly for slider drags.
	//
	// 片段取决于实时配置与当前已分享的笔记集合，因此不得被中间层缓存。
	// 浏览器可为滑块拖动在短时间内保留它。
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	_, _ = c.Writer.WriteString(body)
}
