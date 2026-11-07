package routers

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
	"github.com/haierkeys/fast-note-sync-service/internal/service"
)

// serveBlogList renders /blog/all, /blog/c/<section> and /blog/c/<section>/<sub>.
//
// The three pages are one handler because they differ only in which entries they slice and which
// card shape they use: same template, same rail assembly, same pager. Splitting them would
// duplicate the chrome, and the chrome is exactly what drifted between pages in the previous
// design.
//
// serveBlogList 渲染 /blog/all、/blog/c/<版块> 与 /blog/c/<版块>/<二级>。
//
// 三个页面共用一个处理函数，因为它们只差「切哪一批条目」与「用哪种卡片」：同一个模板、
// 同一套右栏组装、同一个分页器。把它们拆开会复制一遍外壳，而外壳正是上一版页面之间发生漂移的那部分。
func serveBlogList(c *gin.Context, appContainer *app.App, shareSvc service.ShareService, section, sub string, page int) {
	cfg, ok := blogEnabled(c, appContainer)
	if !ok {
		return
	}
	siteTitle := blogSiteTitle(cfg.Title)
	icp, mps, mpCode := blogFiling(cfg)
	uid := blogUID(cfg.UID)
	base := blogBaseURL(c, appContainer)
	publishedOnly := !strings.EqualFold(strings.TrimSpace(cfg.GraphScope), "all")

	list, err := shareSvc.GetBlogList(c.Request.Context(), uid, base, publishedOnly, section, sub, page)
	if err != nil {
		if errors.Is(err, service.ErrBlogSectionNotFound) {
			blogErrorPage(c, appContainer, http.StatusNotFound, "版块",
				"没有这个版块",
				"这个地址对应的版块不存在。它可能被改名了，或者是你手动输入的。",
				[]string{"从顶栏的「博客」下拉里挑一个现有版块。", "或者回到全部笔记。"},
			)
			return
		}
		blogFail(c, appContainer, err.Error())
		return
	}

	// Out-of-range pages answer 404 rather than silently rendering the last page: the reference
	// site does the same, and a page number that quietly means something else is worse than a
	// missing page — it teaches the reader their URL was fine.
	//
	// 越界页返回 404 而不是悄悄渲染最后一页：参照站也是这么做的，而一个悄悄表示别的东西的页码
	// 比一个不存在的页面更糟——它会教读者以为自己那条 URL 是对的。
	if page > list.TotalPages {
		blogErrorPage(c, appContainer, http.StatusNotFound, "分页",
			"没有这一页",
			fmt.Sprintf("「%s」一共 %d 页，你请求的是第 %d 页。", list.Kicker, list.TotalPages, page),
			[]string{"回到第一页。"},
		)
		return
	}

	// Recommendations rank folders, not articles, and they come from the whole published set —
	// the same list the card page's capsule shows. Which level they cover is decided by the
	// service: top level on /blog/all, one step down on a section page, none on a sub-section page.
	//
	// 推荐排的是文件夹而不是文章，且来自整个已发布集合——与名片页胶囊展示的是同一份。
	// 覆盖哪一层由 service 决定：/blog/all 上是顶层，版块页上下移一层，二级栏目页上没有。
	list.Hot = nil
	root := strings.TrimRight(base, "/")
	canonical := root + "/blog/all"
	switch {
	case sub != "":
		canonical = root + "/blog/c/" + section + "/" + sub
	case section != "":
		canonical = root + "/blog/c/" + section
	}
	if list.Page > 1 {
		canonical = canonical + "/" + strconv.Itoa(list.Page)
	}
	desc := fmt.Sprintf("%s — %d 篇笔记。", list.Kicker, list.Total)

	var buf bytes.Buffer
	if err := blogListTmpl.Execute(&buf, blogListView{
		blogChrome: blogChrome{
			Title:       list.Kicker + " · " + siteTitle,
			SiteName:    siteTitle,
			ICP:         icp,
			MPS:         mps,
			MPSCode:     mpCode,
			Indexable:   blogIndexable(cfg.Indexable),
			Canonical:   canonical,
			Description: desc,
			// 栏目页自动发现自己的源：栏目源收录的正是本页列出的文章，在这里订阅即订阅这个栏目。
			// A section page autodiscovers its own feed: the section feed carries exactly the
			// articles this page lists, so subscribing here subscribes to the section.
			FeedURL:     blogFeedURL(base, section),
			CSS:         template.CSS(blogCSS),
			Pages:       nil,
			HasShelf:    shelfHasBooks(),
			Sections:    list.Sections,
			Nav:         nil,
			TintJS:      template.JS(blogTintJS()),
			WarpJS:      "", // 已改由 head 的 inkjs 注入；字段留下只为不惊动五个 view 结构体。
		},
		Kicker:     list.Kicker,
		Section:    list.Section,
		Sub:        list.Sub,
		Entries:    list.Entries,
		Subs:       list.Subs,
		Total:      list.Total,
		Page:       list.Page,
		TotalPages: list.TotalPages,
		PagerLinks: list.Pages,
		PrevURL:    list.PrevURL,
		NextURL:    list.NextURL,
		Recs:       list.Recs,
		DockJS:     template.JS(blogNavDockJS()),
		AskEnabled: blogAskEnabled(appContainer),
	}); err != nil {
		blogFail(c, appContainer, err.Error())
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(buf.Bytes())
}

// blogPageParam parses the :page segment, answering 404 for anything that is not a page number.
//
// A non-numeric page is not a server error: /blog/all/abc is a URL that never described a page.
// Answering 500 would blame the server for a typo, and answering page 1 would hide the typo.
//
// blogPageParam 解析 :page 段，对任何不是页码的输入返回 404。
//
// 非数字页码不是服务端错误：/blog/all/abc 是一条从未描述过任何页面的 URL。
// 返回 500 等于为一次手滑责怪服务器，而返回第一页则把这次手滑藏了起来。
func blogPageParam(c *gin.Context, appContainer *app.App, kicker string) (int, bool) {
	n, err := strconv.Atoi(c.Param("page"))
	if err != nil || n < 1 {
		blogErrorPage(c, appContainer, http.StatusNotFound, kicker,
			"没有这一页",
			"页码必须是正整数。",
			[]string{"回到第一页。"},
		)
		return 0, false
	}
	return n, true
}

// serveBlogArchives renders /blog/archives.
// serveBlogArchives 渲染 /blog/archives。
func serveBlogArchives(c *gin.Context, appContainer *app.App, shareSvc service.ShareService) {
	cfg, ok := blogEnabled(c, appContainer)
	if !ok {
		return
	}
	siteTitle := blogSiteTitle(cfg.Title)
	icp, mps, mpCode := blogFiling(cfg)
	uid := blogUID(cfg.UID)
	base := blogBaseURL(c, appContainer)
	publishedOnly := !strings.EqualFold(strings.TrimSpace(cfg.GraphScope), "all")

	years, total, err := shareSvc.GetBlogArchives(c.Request.Context(), uid, base, publishedOnly)
	if err != nil {
		blogFail(c, appContainer, err.Error())
		return
	}

	// The section list is needed by the nav dropdown on this page too; it comes from a home
	// assembly, which is cheap without a graph and is the only place sections are derived.
	//
	// 这一页的顶栏下拉也需要版块清单；它来自一次不带图谱的首页组装，
	// 而那是版块被推导出来的唯一地方。
	home, err := shareSvc.GetBlogHome(c.Request.Context(), uid, base, publishedOnly, 0)
	if err != nil {
		blogFail(c, appContainer, err.Error())
		return
	}

	canonical := strings.TrimRight(base, "/") + "/blog/archives"
	desc := fmt.Sprintf("%s — 全部 %d 篇笔记，按年份归档。", siteTitle, total)

	var buf bytes.Buffer
	if err := blogArchivesTmpl.Execute(&buf, blogArchiveView{
		blogChrome: blogChrome{
			Title:       "归档 · " + siteTitle,
			SiteName:    siteTitle,
			ICP:         icp,
			MPS:         mps,
			MPSCode:     mpCode,
			Indexable:   blogIndexable(cfg.Indexable),
			Canonical:   canonical,
			Description: desc,
			FeedURL:     strings.TrimRight(base, "/") + "/blog/rss.xml",
			CSS:         template.CSS(blogCSS),
			Pages:       home.Nav.Pages,
			HasShelf:    shelfHasBooks(),
			Sections:    home.Sections,
			Nav:         home.Nav.Tree,
			WarpJS:      "", // 已改由 head 的 inkjs 注入；字段留下只为不惊动五个 view 结构体。
		},
		Years:      years,
		Total:      total,
		DockJS:     template.JS(blogNavDockJS()),
		AskEnabled: blogAskEnabled(appContainer),
	}); err != nil {
		blogFail(c, appContainer, err.Error())
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(buf.Bytes())
}

