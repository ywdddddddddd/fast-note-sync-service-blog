package routers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"github.com/haierkeys/fast-note-sync-service/internal/middleware"
	"github.com/haierkeys/fast-note-sync-service/internal/service"
	"go.uber.org/zap"
)

// Built-in blog pages.
//
// They publish the very same shares that /share/<resID>/<token> exposes, but server-rendered:
// the layout mirrors the native share page's design language (warm-grey palette, translucent
// borders, 1.75 line-height, frosted sticky bar) while every page stays readable and indexable
// without JavaScript.
//
// 内置博客页面。
//
// 发布与 /share/<resID>/<token> 完全相同的分享内容，但改为服务端渲染：布局沿用原生分享页的
// 设计语言（暖灰配色、半透明边框、1.75 行高、粘性毛玻璃顶栏），同时每个页面在无 JavaScript
// 的环境下依然可读、可被搜索引擎收录。
//
// Routes:
//
//	GET /blog        home: dashboard stats, tag cloud, knowledge graph, note list
//	GET /blog/<slug> article: three-column reading view with an in-page outline
//
// 路由：
//
//	GET /blog        首页：仪表盘统计、标签云、知识图谱、笔记列表
//	GET /blog/<slug> 文章页：三栏阅读视图，带本页目录
//
// Enable with:
//
//	blog:
//	  enabled: true
//	  title: "我的笔记"

// sortedDepths returns the depths present in the data, ascending.
//
// The slider's range comes from this rather than from a constant, so a graph that does not reach
// depth 3 does not offer a control position that renders nothing.
//
// sortedDepths 返回数据中实际存在的深度，升序。
//
// 滑块的范围取自它而不是常量，因此一个到不了深度 3 的图，不会提供一个渲染出空白的位置。
func sortedDepths(m map[int]template.JS) []int {
	out := make([]int, 0, len(m))
	for d := range m {
		out = append(out, d)
	}
	sort.Ints(out)
	return out
}

// Local graph depth range. These mirror the limits the management API enforces on the same
// setting; they are repeated here because that package is not importable from this one.
//
// 局部图深度范围。它们与管理接口对同一设置所施加的限制一致；
// 这里重复一份是因为那个包无法从本包导入。
const (
	blogLocalDepthMin = 1
	blogLocalDepthMax = 3
)

func registerBlogRoutes(r *gin.Engine, appContainer *app.App) {
	shareSvc := appContainer.ShareService
	if shareSvc == nil {
		return
	}

	// Same resolver the Q&A endpoints use, built from the same startup configuration.
	// 与问答端点同一套解析器，取自同一份启动期配置。
	ident := newClientIdentityResolver(appContainer.Config().Server.TrustedProxies)

	// Sites values are re-read per request, so toggling blog.enabled from the plugin's
	// settings tab takes effect immediately without restarting the service.
	// 站点配置按请求读取，因此从插件设置页切换 blog.enabled 会立即生效，无需重启服务。

	// The public pages are registered on the engine, not on the API group, so the visit log needs
	// its own middleware here. Group with an empty prefix keeps every route path unchanged while
	// attaching the recorder only to the blog's own handlers.
	//
	// 公开页面注册在 engine 上而不是 API 组上，因此访问日志需要在这里有自己的中间件。
	// 空前缀的组保持所有路由路径不变，同时把记录器只挂到博客自身的处理器上。
	blog := r.Group("", middleware.BlogVisitRecorder(func(method, path string, status int, ms int64, ip, client string) {
		shareSvc.RecordBlogAccess(method, path, status, ms, ip, client)
	}))

	// Reactions. Public by design: a reader who can read the page is allowed to react to it, and
	// requiring a token would mean the button only works for the author's own devices.
	//
	// The endpoint returns the note's new tally so the button can show the authoritative number
	// rather than the client's guess — two tabs reacting at once then agree instead of drifting.
	//
	// 反应接口。刻意公开：能读到页面的读者就可以对它作出反应，而要求 token 意味着这个按钮
	// 只在作者自己的设备上有效。
	//
	// 接口返回该笔记的新计数，使按钮显示权威数字而不是客户端的猜测——两个标签页同时反应时会一致，
	// 而不是各自漂移。
	blog.POST("/blog/react", func(c *gin.Context) {
		cfg, ok := blogEnabled(c, appContainer)
		if !ok {
			return
		}
		var in struct {
			Slug  string `json:"slug"`
			Kind  string `json:"kind"`
			Delta int64  `json:"delta"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		_ = cfg

		// Rate limiting only, applied after the body is parsed because the vote key needs the
		// slug and kind. The limits themselves are generous enough that a reader who misclicks
		// and corrects themselves never notices them; they exist to stop a loop, not a person.
		//
		// 只做限流，放在解析请求体之后，因为去重键需要 slug 与 kind。
		// 限制本身足够宽松，点错又改回来的读者不会察觉；它们是为了挡住循环，不是挡人。
		switch blogGuardInstance.allowReact(ident.identity(c), in.Slug, in.Kind, in.Delta) {
		case reactTooFrequent:
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "操作太频繁，请稍后再试"})
			return
		case reactAlreadyVoted:
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "今天已经对这篇笔记表过态了"})
			return
		}

		tally, err := shareSvc.ReactBlogNote(in.Slug, in.Kind, in.Delta)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, tally)
	})

	// RSS feed. Registered before any "/blog/:slug" style route so the literal path wins.
	// RSS 订阅源。在任何 "/blog/:slug" 形式的路由之前注册，确保字面路径优先。
	blog.GET("/blog/rss.xml", func(c *gin.Context) {
		serveBlogFeed(c, appContainer)
	})

	// Graph fragment, loaded on demand by the article page's graph controls.
	// Registered before "/blog/:slug" for the same reason as the feed.
	// 图谱片段，由文章页的图谱控件按需加载。
	// 与订阅源同理，注册在 "/blog/:slug" 之前。
	blog.GET("/blog/graph", func(c *gin.Context) {
		serveGraphFragment(c, appContainer)
	})

	blog.GET("/blog", func(c *gin.Context) {
		cfg, ok := blogEnabled(c, appContainer)
		if !ok {
			return
		}
		siteTitle := blogSiteTitle(cfg.Title)
		uid := blogUID(cfg.UID)
		base := blogBaseURL(c, appContainer)

		publishedOnly := !strings.EqualFold(strings.TrimSpace(cfg.GraphScope), "all")
		maxNodes := cfg.MaxGraphNodes
		if maxNodes <= 0 {
			maxNodes = 120
		}

		home, err := shareSvc.GetBlogHome(c.Request.Context(), uid, base, publishedOnly, maxNodes)
		if err != nil {
			blogFail(c, appContainer, err.Error())
			return
		}

		// One visit per page render. It counts refreshes and crawlers too, which is why the label
		// says 访问次数 rather than 阅读量 — see blogReactionLog.addVisit.
		//
		// 每次页面渲染计一次。它同样统计刷新与爬虫，这也是标签写「访问次数」而不是「阅读量」的原因——
		// 见 blogReactionLog.addVisit。
		shareSvc.AddBlogVisit()

		// Reaction tallies: the dashboard's seventh figure and the popular list both come from
		// this one read.
		//
		// 反应计数：仪表盘的第七个数字与热门列表都来自这同一次读取。
		reactions := shareSvc.GetBlogReactions()

		// Page entry points come from the home assembly itself. They used to be re-fetched from
		// GetBlogNav because Nav.Pages arrived empty — the cause was GetBlogHome replacing the
		// whole Nav struct after filling Pages, and that is now fixed at the source.
		//
		// 页面入口直接取首页自己的组装结果。它此前要从 GetBlogNav 重新取一次，因为 Nav.Pages
		// 到手时是空的——原因是 GetBlogHome 在填好 Pages 之后又把整个 Nav 结构体替换掉了，
		// 现在已在源头修好。

		// The particle count is the published-note count. home.Stats.Notes is exactly that number,
		// and it is already computed for the dashboard — so the field and the seventh figure can
		// never disagree about how many notes there are.
		//
		// 粒子数等于已发布笔记数。home.Stats.Notes 正是这个数字，而且它已经为仪表盘算好了 ——
		// 因此这片场与第七个数字永远不会在「有多少篇笔记」上互相矛盾。
		particlesCfg, _ := json.Marshal(map[string]any{
			"count": home.Stats.Notes,
		})

		svg, note := renderGraphSVG(home.Graph)

		// Render into memory first. Executing straight into the response writer means a
		// mid-template error leaves a half-written page and the error is invisible — which is
		// exactly how a missing struct field turned into a silently truncated page.
		// 先渲染到内存。直接写响应的话，模板中途出错会留下半截页面，而且错误看不到——
		// 之前一个缺失的结构体字段就是这样变成了静默截断的页面。
		homeURL := strings.TrimRight(base, "/") + "/blog"
		homeDesc := fmt.Sprintf("%s — 共 %d 篇公开笔记。", siteTitle, len(home.Entries))

		var buf bytes.Buffer
		if err := blogHomeTmpl.Execute(&buf, blogHomeView{
			Title:        siteTitle,
			SiteName:     siteTitle,
			Indexable:    blogIndexable(cfg.Indexable),
			Canonical:    homeURL,
			Description:  homeDesc,
			FeedURL:      homeURL + "/rss.xml",
			JSONLD:       blogHomeJSONLD(siteTitle, homeURL, homeDesc, len(home.Entries)),
			CSS:          template.CSS(blogCSS),
			Stats:        home.Stats,
			Tags:         home.Tags,
			Entries:      home.Entries,
			Nav:          home.Nav.Tree,
			Pages:        home.Nav.Pages,
			GraphSVG:     svg,
			GraphNote:    note,
			Truncated:    home.Graph != nil && home.Graph.Truncated,
			Activity:     home.Activity,
			ShowGraph:    cfg.ShowGraph,
			AskEnabled:   blogAskEnabled(appContainer),
			Visits:       reactions.Visits,
			Hot:          blogHotList(reactions, home.Entries, blogHotLimit),
			ReactionJS:   template.JS(blogReactionJS()),
			WarpJS:       template.JS(blogWarpJS()),
			ParticlesJS:  template.JS(blogParticlesJS()),
			ParticlesCfg: template.JS(particlesCfg),
		}); err != nil {
			blogFail(c, appContainer, err.Error())
			return
		}

		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(buf.Bytes())
	})

	// Page aliases. Registered before the :slug catch-all for readability; gin's tree prefers a
	// static segment over a parameter either way, so /blog/about can never be mistaken for a note
	// whose slug happens to be "about".
	//
	// 页面别名。注册在 :slug 通配之前是为了可读性；gin 的路由树无论如何都优先匹配静态段，
	// 因此 /blog/about 绝不会被误认成一篇 slug 恰好叫 about 的笔记。
	for _, alias := range blogPageRouteAliases() {
		alias := alias
		blog.GET("/blog/"+alias, func(c *gin.Context) {
			serveBlogPage(c, appContainer, shareSvc, alias)
		})
	}

	blog.GET("/blog/:slug", func(c *gin.Context) {
		cfg, ok := blogEnabled(c, appContainer)
		if !ok {
			return
		}
		siteTitle := blogSiteTitle(cfg.Title)
		uid := blogUID(cfg.UID)
		// baseURL is needed again, but for a different reason than before: canonical / og:url /
		// JSON-LD / the feed link must all be absolute. The article page still does not assemble
		// any home-page datasets.
		// 再次需要 baseURL，但理由与从前不同：canonical / og:url / JSON-LD / 订阅源链接都必须
		// 是绝对地址。文章页依然不组装任何首页数据集。
		base := blogBaseURL(c, appContainer)
		slug := c.Param("slug")

		post, err := shareSvc.RenderBlogBySlug(c.Request.Context(), slug)
		if err != nil {
			// The service distinguishes "never existed / not a note" (code.ErrorShareNotFound)
			// from "revoked or expired" (domain.ErrShareCancelled). That distinction is useful in
			// the server log and must not reach the visitor: telling a stranger whether a note
			// exists-but-is-unpublished or never existed turns the blog into an enumeration
			// oracle for the vault's note names.
			//
			// 服务端区分「从未存在 / 不是笔记」（code.ErrorShareNotFound）与「已撤销或已过期」
			// （domain.ErrShareCancelled）。这个区分对服务端日志有用，但绝不能传给访客：
			// 告诉陌生人某篇笔记是「存在但未公开」还是「从未存在」，
			// 会把博客变成探测仓库笔记名的枚举器。
			blogErrorPage(c, appContainer, http.StatusNotFound, "无法访问",
				"这个链接不可用",
				"这篇笔记不存在、已被取消分享，或链接已失效。",
				[]string{"确认地址完整：短链形如 /s/1.12-682acdc4e9，文章页形如 /blog/1.12-682acdc4e9。"},
			)
			return
		}

		// The visit is counted only after the note was found and rendered, so a 404 does not
		// inflate the figure. A reader who mistypes an address has not visited anything.
		//
		// 只有在笔记确实找到并渲染之后才计数，使 404 不会把数字抬高。
		// 打错地址的读者并没有访问到任何东西。
		shareSvc.AddBlogVisit()

		// Reactions for the interaction bar. Read once per render: the article view needs this
		// note's tally, and reading the whole file is the only shape the store offers.
		//
		// 互动条所需的反应计数。每次渲染读一次：文章页需要这一篇的计数，
		// 而整份读取是这个存储唯一提供的形状。
		reactions := shareSvc.GetBlogReactions()
		tally := reactions.Notes[slug]
		if tally == nil {
			tally = &dto.BlogReactionTallyDTO{}
		}

		// Serialised with encoding/json rather than concatenated in the template, because the
		// slug is a string that arrived in the URL: a stray quote in one would otherwise break
		// the script that reads this island.
		//
		// 用 encoding/json 序列化，而不是在模板里拼装，因为 slug 是一个从 URL 到达的字符串：
		// 其中一个游离的引号就会破坏读取这块 JSON 的脚本。
		reactionCfg, _ := json.Marshal(map[string]any{
			"slug":         slug,
			"count":        blogParticleCount(appContainer),
			"dislikeBurst": blogDislikeBurst(appContainer),
		})

		// 站外链接新标签打开；服务端无法提供的本地图片降级为提示；本页目录由标题提取而来。
		body := sanitizeImages(externalizeLinks(post.HTML))
		// The outline and the headings come out of one pass, so the anchor a link names and the
		// anchor a heading carries cannot disagree. See blog_outline.go for what that fixed.
		//
		// 大纲与标题由同一遍产出，因此链接所指的锚点与标题所带的锚点不可能不一致。
		// 这一改动修好了什么，见 blog_outline.go。
		toc, body := extractTOC(body)

		// The article view reuses the same navigation tree as the home page, but asks for the
		// tree only. It used to call GetBlogHome(…, publishedOnly=false, maxNodes=0) and read
		// Nav.Tree out of the result, which meant every article view re-ran the whole home
		// assembly — two N+1 passes, the force-directed layout, the heat map and the tag cloud —
		// and minted anonymous nodes for unpublished notes on a page that renders no graph.
		//
		// 文章页复用与首页相同的导航树，但只索取这棵树。此前它调用
		// GetBlogHome(…, publishedOnly=false, maxNodes=0) 再从结果里读出 Nav.Tree，
		// 于是每次打开文章都会重跑整套首页组装——两轮 N+1、力导向布局、热力图与标签云——
		// 并且在一个不渲染图谱的页面上为未发布笔记铸造匿名节点。
		nav := []dto.BlogNavNode{}
		pages := []dto.BlogNavPageDTO{}
		if navDTO, err := shareSvc.GetBlogNav(c.Request.Context(), uid); err == nil && navDTO != nil {
			nav = navDTO.Tree
			pages = navDTO.Pages
			// Flag the current page on the node itself. Recursive {{template}} calls reset $ to
			// the node being rendered, so the template cannot reach a root-level ActiveSlug —
			// carrying the flag in the data keeps the template trivial.
			// 在节点自身标记当前页。递归 {{template}} 调用会把 $ 重置为当前渲染的节点，
			// 模板无法再访问根级的 ActiveSlug；把标记放进数据里可以让模板保持极简。
			markActiveNav(nav, slug)
		}

		meta := []string{}
		if post.CreatedAt != "" {
			meta = append(meta, post.CreatedAt)
		}
		if post.Path != "" {
			meta = append(meta, post.Path)
		}

		// Same in-memory rendering as the home page, so a template error surfaces instead of
		// producing a silently truncated article.
		// 与首页相同的内存渲染：模板出错会显式暴露，而不是产出一篇被静默截断的文章。
		// Local graph. Gated by the same switch as the home graph, and it never fails the page:
		// if the sub-graph cannot be built, the card is simply omitted.
		//
		// 局部图。与首页图谱受同一开关控制；它绝不会让页面失败：
		// 若子图无法构建，则直接省略该卡片。
		var localSVG template.HTML
		var localNote string
		// One data island per depth, so the slider swaps between them without a request.
		//
		// The alternative was an endpoint the client calls on every slider move. Three islands
		// cost about 10 KB of markup on an article that already renders at ~120 KB, and they buy
		// three things a round-trip cannot: the interaction is instant, it works with the page
		// cached, and depth 2 and 3 are already present when the reader first drags the control.
		//
		// 每个深度一份数据岛，使滑块在它们之间切换而无需请求。
		//
		// 另一种做法是让客户端在每次拖动滑块时请求一个端点。三份数据岛在一篇本就约 120 KB 的
		// 文章上多出约 10 KB 标记，却换来往返做不到的三件事：交互是即时的、页面被缓存时依然可用、
		// 并且读者第一次拖动滑块时深度 2 与 3 的数据已经就位。
		localGraphData := map[int]template.JS{}
		localDepth := blogLocalGraphDepth(cfg)
		showLocal := cfg.ShowGraph
		if showLocal {
			for d := blogLocalDepthMin; d <= blogLocalDepthMax; d++ {
				graph, gerr := shareSvc.GetLocalGraph(c.Request.Context(), slug, d, cfg.MaxGraphNodes)
				if gerr != nil || graph == nil || len(graph.Nodes) <= 1 {
					// A graph of one node is the article itself — nothing to show.
					// 只有一个节点的图就是文章本身——没有可展示的内容。
					if d == localDepth {
						showLocal = false
					}
					continue
				}
				// The static SVG and the interactive canvas are two renderings of one value, so
				// they can never disagree about the graph.
				// 静态 SVG 与交互画布是同一个值的两种渲染，因此它们永远不会对这张图给出不同说法。
				if d == localDepth {
					localSVG, localNote = renderGraphSVG(graph)
				}
				localGraphData[d] = graphDataJSON(graph)
			}
			if len(localGraphData) == 0 {
				showLocal = false
			}
		}

		postURL := strings.TrimRight(base, "/") + "/blog/" + slug
		// The article description falls back to the title when the note has no summary text,
		// so the meta tag is never an empty string (some crawlers treat that as a missing tag).
		// 文章摘要为空时回退为标题，使 meta 标签永远不是空串
		// （部分爬虫会把空串当作标签缺失处理）。
		postDesc := post.Title
		if s := blogPlainTextFromHTML(body); s != "" {
			postDesc = s
		}

		var pbuf bytes.Buffer
		if perr := blogPostTmpl.Execute(&pbuf, blogPostView{
			Title:       post.Title + " · " + siteTitle,
			SiteName:    siteTitle,
			Indexable:   blogIndexable(cfg.Indexable),
			Canonical:   postURL,
			Description: postDesc,
			FeedURL:     strings.TrimRight(base, "/") + "/blog/rss.xml",
			JSONLD:      blogPostJSONLD(siteTitle, post.Title, postURL, postDesc, post.CreatedAt),
			CSS:         template.CSS(blogCSS),
			Nav:         nav,
			Pages:       pages,
			Like:        tally.Like,
			Dislike:     tally.Dislike,
			Visits:      reactions.Visits,
			ReactionJS:  template.JS(blogReactionJS()),
			ReactionCfg: template.JS(reactionCfg),
			WarpJS:      template.JS(blogWarpJS()),
			ActiveSlug:  slug,
			PostTitle:   post.Title,
			PostPath:    post.Path,
			PostMeta:    meta,
			Body:        template.HTML(body), //nolint:gosec // rendered by goldmark from the user's own notes
			TOC:         toc,
			AskEnabled:  blogAskEnabled(appContainer),

			HasMath:          service.HasMathMarkup(body),
			ShowLocalGraph:   showLocal,
			LocalGraphSVG:    localSVG,
			LocalGraphNote:   localNote,
			LocalGraphDepth:  localDepth,
			GraphEngineJS:    graphEngineJS(), //nolint:gosec // static constant with an asset fingerprint
			GraphDataByDepth: localGraphData,
			GraphDepths:      sortedDepths(localGraphData),
			PreviewJSON:      linkPreviewJSON(shareSvc, c, cfg, body),
			PreviewJS:        template.JS(blogLinkPreviewJS),
		}); perr != nil {
			blogFail(c, appContainer, perr.Error())
			return
		}

		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(pbuf.Bytes())
	})
}

// markActiveNav sets Active on the node whose slug matches the page being rendered.
// markActiveNav 把与当前页面 slug 匹配的节点标记为 Active。
func markActiveNav(nodes []dto.BlogNavNode, slug string) {
	for i := range nodes {
		n := &nodes[i]
		if n.Slug != "" && n.Slug == slug {
			n.Active = true
		}
		if len(n.Children) > 0 {
			markActiveNav(n.Children, slug)
		}
	}
}

// blogEnabled resolves the request-scoped blog configuration, writing the 404 itself when the
// blog is switched off.
// blogEnabled 读取本次请求的博客配置；当博客已关闭时直接写出 404。
// blogPageRouteAliases lists the page aliases this router registers a route for.
//
// It reads from the service package's whitelist rather than repeating the names, so a route can
// never exist for an alias the filter does not recognise — that combination is the one that would
// take a note out of the article list and give the reader nowhere to land.
//
// blogPageRouteAliases 列出本路由为其注册路由的页面别名。
//
// 它读取服务端包里的白名单，而不是重抄一遍名字，因此绝不会出现「有路由但过滤器不认」的别名——
// 那种组合会把一篇笔记从文章列表里拿走，却不给读者任何落脚点。
func blogPageRouteAliases() []string {
	return service.BlogPageAliases()
}

// serveBlogPage renders a page note at its alias route.
//
// A missing page is a 404 with the same wording as a missing article. Saying "this alias has no
// note" separately would announce which aliases exist, and the alias set is small enough that
// enumerating it is trivial.
//
// serveBlogPage 在别名路由上渲染页面笔记。
//
// 页面不存在时是一个 404，措辞与文章缺失时一致。单独说「这个别名没有对应笔记」会泄露哪些别名存在，
// 而别名集合小到可以被轻易枚举。
func serveBlogPage(c *gin.Context, appContainer *app.App, shareSvc service.ShareService, alias string) {
	cfg, ok := blogEnabled(c, appContainer)
	if !ok {
		return
	}
	siteTitle := blogSiteTitle(cfg.Title)
	uid := blogUID(cfg.UID)
	base := blogBaseURL(c, appContainer)

	page, err := shareSvc.RenderBlogPage(c.Request.Context(), uid, alias)
	if err != nil {
		blogErrorPage(c, appContainer, http.StatusNotFound, "无法访问",
			"这个页面不存在",
			"还没有笔记声明这个页面。",
			[]string{"在笔记的 frontmatter 里写 page: " + alias + " 就会把它当作这个页面发布。"},
		)
		return
	}

	// Same single pass as the article page, for the same reason: a page note has headings too.
	// 与文章页同一遍处理，理由相同：页面笔记也有标题。
	toc, pageHTML := extractTOC(page.HTML)
	pageTitle := page.Title
	if pageTitle == "" {
		// The alias label is a fallback, not the preferred title: an author who wrote a title in
		// frontmatter or as the first heading gets that, and one who did neither still gets a
		// page that is not headed "about".
		//
		// 别名标签是回退值，不是首选标题：在 frontmatter 或首个标题里写了标题的作者用自己写的，
		// 两者都没写的作者也仍然得到一个不是以「about」为标题的页面。
		pageTitle = page.Alias
	}
	pageURL := strings.TrimRight(base, "/") + "/blog/" + alias
	desc := fmt.Sprintf("%s — %s", siteTitle, pageTitle)

	// The page route shows the same top bar as the article route, so it needs the same page-note
	// entries. They come from GetBlogNav rather than RenderBlogPage because the latter knows only
	// about the single page it was asked to render. The alias being viewed right now is dropped:
	// a top bar whose only item points at the page you are already standing on is not a way out.
	//
	// 页面路由与文章路由显示同一个顶栏，因此需要同一份页面笔记入口。它们取自 GetBlogNav
	// 而不是 RenderBlogPage，因为后者只认识被索取渲染的那一个页面。当前正在看的别名会被剔除：
	// 一个只有「指向你脚下」的入口的顶栏，不是一条去路。
	pages := []dto.BlogNavPageDTO{}
	if navDTO, err := shareSvc.GetBlogNav(c.Request.Context(), uid); err == nil && navDTO != nil {
		for _, p := range navDTO.Pages {
			if p.Alias != alias {
				pages = append(pages, p)
			}
		}
	}

	var buf bytes.Buffer
	if err := blogPageTmpl.Execute(&buf, blogPageView{
		Title:       pageTitle,
		SiteName:    siteTitle,
		Indexable:   blogIndexable(cfg.Indexable),
		Canonical:   pageURL,
		Description: desc,
		FeedURL:     strings.TrimRight(base, "/") + "/blog/rss.xml",
		JSONLD:      blogPostJSONLD(siteTitle, pageTitle, pageURL, desc, page.UpdatedAt),
		CSS:         template.CSS(blogCSS),
		PageTitle:   pageTitle,
		PageAlias:   page.Alias,
		Pages:       pages,
		Avatar:      page.Avatar,
		UpdatedAt:   page.UpdatedAt,
		Path:        page.Path,
		Body:        template.HTML(pageHTML),
		TOC:         toc,
		HasMath:     strings.Contains(page.HTML, "math-inline") || strings.Contains(page.HTML, "math-block"),
		WarpJS:      template.JS(blogWarpJS()),
		AskEnabled:  blogAskEnabled(appContainer),
	}); err != nil {
		blogFail(c, appContainer, err.Error())
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(buf.Bytes())
}

// blogEnabled reports whether the blog is switched on, writing the refusal page when it is not.
// blogEnabled 报告博客是否开启，未开启时写出拒绝页。
func blogEnabled(c *gin.Context, appContainer *app.App) (*blogRuntimeConfig, bool) {
	raw := &appContainer.Config().Blog
	if raw.Enabled == nil || !*raw.Enabled {
		// The blog switch is global state, so naming it leaks nothing about any note.
		// 博客开关是全局状态，说明它不会泄露任何单篇笔记的信息。
		blogErrorPage(c, appContainer, http.StatusNotFound, "站点未开启",
			"站点当前未开放",
			"这个博客当前是关闭状态。",
			[]string{"如果你在 Obsidian 里关闭了博客，重新打开后此地址立即恢复。", "博客开关每次请求实时生效，无需重启服务。"},
		)
		return nil, false
	}
	return &blogRuntimeConfig{
		Title:           raw.Title,
		UID:             raw.UID,
		Indexable:       raw.Indexable,
		GraphScope:      raw.GraphScope,
		MaxGraphNodes:   raw.MaxGraphNodes,
		ShowGraph:       raw.ShowGraph != nil && *raw.ShowGraph,
		LocalGraphDepth: blogClampDepth(raw.LocalGraphDepth),
	}, true
}

// blogRuntimeConfig mirrors the fields the templates need, decoupled from the config struct.
// blogRuntimeConfig 汇总模板需要的配置字段，与配置结构解耦。
type blogRuntimeConfig struct {
	Title         string
	UID           int64
	Indexable     *bool
	GraphScope    string
	MaxGraphNodes int
	ShowGraph     bool
	// LocalGraphDepth is the starting depth of the in-article graph. Depth is a viewing
	// preference rather than a per-article fact, so it lives with the other switches.
	// LocalGraphDepth 是文章内图谱的起始深度。深度是「查看偏好」而不是每篇文章的事实，
	// 因此它与其它开关放在一起。
	LocalGraphDepth int
}

func blogSiteTitle(v string) string {
	if t := strings.TrimSpace(v); t != "" {
		return t
	}
	return "Notes"
}

func blogIndexable(v *bool) bool { return v == nil || *v }

// blogLocalGraphDepth reads the in-article graph depth from config, clamped to the range the
// slider exposes. Depth is a viewing preference rather than a per-article fact, so it lives with
// the other blog switches instead of on the post.
//
// blogLocalGraphDepth 从配置读取文章内图谱的深度，并夹到滑块暴露的区间内。
// 深度是「查看偏好」而不是每篇文章的事实，因此它与其它博客开关放在一起，而不挂在文章上。
func blogLocalGraphDepth(cfg *blogRuntimeConfig) int {
	if cfg == nil {
		return 1
	}
	return blogClampDepth(cfg.LocalGraphDepth)
}

// blogClampDepth keeps the depth inside the range the slider exposes (1–3). The config file is
// hand-editable, so a value outside the range must not reach the graph builder.
//
// blogClampDepth 把深度夹在滑块暴露的区间内（1–3）。配置文件可手工编辑，
// 因此越界值不得传到图构建器。
func blogClampDepth(d int) int {
	if d < 1 {
		return 1
	}
	if d > 3 {
		return 3
	}
	return d
}

func blogUID(v int64) int64 {
	if v <= 0 {
		return 1
	}
	return v
}

// blogBaseURL resolves the externally reachable base address, mirroring getShareBaseUrl.
// blogBaseURL 解析对外可访问的基础地址，与 getShareBaseUrl 保持一致。
func blogBaseURL(c *gin.Context, appContainer *app.App) string {
	if v := strings.TrimRight(appContainer.Config().Server.ExtApiUrl, "/"); v != "" {
		return v
	}
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

// blogErrorPage renders the shared error page.
//
// It re-reads the site title from config instead of taking one, because two of its three call
// sites sit in configuration helpers that never resolved a title.
//
// blogErrorPage 渲染公共错误页。
// 它自行从配置读取站点标题，而不是接收一个——因为三处调用点里有六处在配置辅助函数中，
// 那些位置从来没有解析过标题。
func blogErrorPage(c *gin.Context, appContainer *app.App, status int, crumb, heading, message string, hints []string) {
	cfg := &appContainer.Config().Blog
	siteTitle := blogSiteTitle(cfg.Title)

	var buf bytes.Buffer
	if err := blogErrorTmpl.Execute(&buf, blogErrorView{
		Title:    crumb + " · " + siteTitle,
		SiteName: siteTitle,
		CSS:      template.CSS(blogCSS),
		Code:     status,
		Heading:  heading,
		Message:  message,
		Hints:    hints,
	}); err != nil {
		// Fall back to plain text rather than serve a half-written page.
		// 退化为纯文本，而不是提供一个写到一半的页面。
		c.String(status, "%s", heading)
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Status(status)
	_, _ = c.Writer.Write(buf.Bytes())
}

// blogFail reports a render failure. The detailed message goes to the server log only: it is
// internal error text and must not be handed to a visitor.
//
// blogFail 上报渲染失败。详细信息只写服务端日志：它是内部错误文本，不应交给访客。
func blogFail(c *gin.Context, appContainer *app.App, msg string) {
	if lg := appContainer.Logger(); lg != nil {
		lg.Error("blog render failed",
			zap.String("path", c.Request.URL.Path),
			zap.String("error", msg))
	}
	blogErrorPage(c, appContainer, http.StatusInternalServerError, "页面出错",
		"页面生成时出错",
		"服务端在生成这个页面时遇到了问题。",
		[]string{"稍后刷新重试。", "如果持续出现，请查看服务端日志中的 blog render failed 条目。"},
	)
}
