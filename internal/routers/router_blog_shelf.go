package routers

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
)

// The shelf routes. There is deliberately no route that returns a PDF: the only thing a visitor
// can ask for is one rendered page at a time, or the cover. That absence is the mechanism, not an
// oversight — do not add a "download original" convenience without deciding, explicitly, to give
// up the property this whole design exists for.
//
// 书架路由。这里刻意没有任何返回 PDF 的路由：访问者能索取到的只有一次一页的渲染结果，或者封面。
// 这个「没有」本身就是机制，不是疏漏——要加「下载原件」这种便利，就得明确决定放弃整套设计
// 存在的那个属性。
func registerBlogShelfRoutes(blog *gin.RouterGroup, appContainer *app.App) {
	blog.GET("/blog/reader.js", func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(blogReaderJS))
	})

	blog.GET("/blog/shelf", func(c *gin.Context) {
		cfg, ok := blogEnabled(c, appContainer)
		if !ok {
			return
		}
		siteTitle := blogSiteTitle(cfg.Title)
		base := strings.TrimRight(blogBaseURL(c, appContainer), "/")

		books := shelfBooks()
		cards := make([]shelfCard, 0, len(books))
		for _, b := range books {
			// A book whose file is missing or unreadable is left off the wall entirely. Listing it
			// would produce a card that opens onto nothing, which is worse than not offering it.
			//
			// 文件缺失或读不出来的书直接不上墙。列出来会得到一张点开什么都没有的卡片，
			// 比不给这个入口更糟。
			n := shelfPageCountMode(c.Request.Context(), b)
			if n <= 0 {
				continue
			}
			cards = append(cards, shelfCard{
				ID:       b.ID,
				Title:    b.Title,
				Author:   b.Author,
				Note:     b.Note,
				Tags:     b.Tags,
				Added:    b.Added,
				Pages:      n,
				CoverURL:   "/blog/shelfcover/" + b.ID + "?v=" + shelfCoverVersion(b),
				Collection: b.Collection,
			})
		}

		// 按栏目分组，保持书目里首次出现的顺序——分组顺序应当由 shelf.json 决定，
		// 而不是由 map 的迭代顺序决定。
		//
		// Group into rails, preserving first-appearance order: the section order should come
		// from shelf.json, not from map iteration order.
		cols := make([]shelfCollection, 0, 4)
		index := map[string]int{}
		for _, c := range cards {
			name := c.Collection
			if name == "" {
				name = "其它资料"
			}
			if i, ok := index[name]; ok {
				cols[i].Cards = append(cols[i].Cards, c)
				continue
			}
			index[name] = len(cols)
			cols = append(cols, shelfCollection{Name: name, Cards: []shelfCard{c}})
		}

		shelfURL := base + "/blog/shelf"
		desc := fmt.Sprintf("%s — %d 份文档，可在线阅读。", siteTitle, len(cards))

		var buf bytes.Buffer
		if err := blogShelfTmpl.Execute(&buf, shelfView{
			Title:       "资料 · " + siteTitle,
			SiteName:    siteTitle,
			Indexable:   blogIndexable(cfg.Indexable),
			CSS:         template.CSS(blogCSS),
			Cards:       cards,
			Collections: cols,
			Count:       len(cards),
			Canonical:   shelfURL,
			Description: desc,
			FeedURL:     base + "/blog/rss.xml",
			JSONLD:      blogPostJSONLD(siteTitle, "资料", shelfURL, desc, ""),
		}); err != nil {
			blogFail(c, appContainer, err.Error())
			return
		}
		writeBlogHTML(c, buf.Bytes())
	})

	blog.GET("/blog/shelf/:id", func(c *gin.Context) {
		cfg, ok := blogEnabled(c, appContainer)
		if !ok {
			return
		}
		siteTitle := blogSiteTitle(cfg.Title)
		base := strings.TrimRight(blogBaseURL(c, appContainer), "/")

		id := c.Param("id")
		book, found := shelfBookByID(id)
		if !found {
			blogErrorPage(c, appContainer, http.StatusNotFound, "无法访问",
				"没有这份文档", "书架上找不到这本书。",
				[]string{"回到 /blog/shelf 看看架上有什么。"})
			return
		}
		pages := shelfPageCountMode(c.Request.Context(), book)
		if pages <= 0 {
			blogErrorPage(c, appContainer, http.StatusNotFound, "无法访问",
				"这份文档打不开", "文件存在，但读不出页数。",
				[]string{"可能是 PDF 损坏，或者服务器上的 pdfinfo 读不了它。"})
			return
		}

		bookURL := base + "/blog/shelf/" + book.ID
		desc := book.Title
		if book.Author != "" {
			desc += " · " + book.Author
		}
		if book.Note != "" {
			desc += " — " + book.Note
		}

		var buf bytes.Buffer
		if err := blogReaderTmpl.Execute(&buf, readerView{
			Title:       book.Title + " · " + siteTitle,
			SiteName:    siteTitle,
			Indexable:   blogIndexable(cfg.Indexable),
			CSS:         template.CSS(blogCSS),
			ID:          book.ID,
			BookTitle:   book.Title,
			Author:      book.Author,
			Note:        book.Note,
			PageCount:   pages,
			Version:     shelfAssetVersion(book),
			FirstPage:   "/blog/shelfimg/" + book.ID + "/1?v=" + shelfAssetVersion(book),
			Canonical:   bookURL,
			Description: desc,
			FeedURL:     base + "/blog/rss.xml",
			JSONLD:      blogPostJSONLD(siteTitle, book.Title, bookURL, desc, book.Added),
		}); err != nil {
			blogFail(c, appContainer, err.Error())
			return
		}
		writeBlogHTML(c, buf.Bytes())
	})

	blog.GET("/blog/shelfimg/:id/:n", func(c *gin.Context) {
		if !blogOn(appContainer) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		book, found := shelfBookByID(c.Param("id"))
		if !found {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		// The URL carries no extension: a pages-mode book stores .avif, a pdf-mode book produces
		// .jpg, and the reader should not have to know which kind it is looking at.
		//
		// URL 不带扩展名：pages 模式存的是 .avif，pdf 模式产出 .jpg，
		// 阅读器不该需要知道自己在看哪一种。
		raw := c.Param("n")
		if i := strings.LastIndex(raw, "."); i > 0 {
			raw = raw[:i]
		}
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		total := shelfPageCountMode(c.Request.Context(), book)
		if total <= 0 || page > total {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		if shelfIsPages(book) {
			p := shelfPageFile(book, page)
			if p == "" {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			serveShelfImage(c, p)
			return
		}

		path, err := shelfRenderPage(c.Request.Context(), book, page)
		if err != nil {
			// Renders fail for reasons the visitor cannot act on (a corrupt source file, a
			// saturated rasteriser). 503 on the image keeps the reader usable — the surrounding
			// page stays, and it can retry by turning back to this page.
			//
			// 渲染失败的原因访问者无从处理（源文件损坏、光栅化器饱和）。图片这里回 503 能让
			// 阅读器继续可用——外层页面还在，翻回这一页就能重试。
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		serveShelfImage(c, path)
	})

	blog.GET("/blog/shelfcover/:id", func(c *gin.Context) {
		if !blogOn(appContainer) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		book, found := shelfBookByID(c.Param("id"))
		if !found {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		// A hand-made cover wins: a scan of the real jacket looks far better on a wall of covers
		// than page one of a textbook, which is usually a title page or a half-title.
		//
		// 手做封面优先：一面封面墙上，扫描的真实书封比教材第一页好看得多，
		// 而教材第一页通常是扉页或书名页。
		if book.Cover != "" {
			if p := filepath.Join(shelfRoot, "covers", book.Cover); shelfCoverNameOK(book.Cover) {
				if st, err := os.Stat(p); err == nil && st.Size() > 0 {
					serveShelfImage(c, p)
					return
				}
			}
		}
		if shelfIsPages(book) {
			if p := shelfPageFile(book, 1); p != "" {
				serveShelfImage(c, p)
				return
			}
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		path := shelfRenderCover(c.Request.Context(), book)
		if path == "" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		serveShelfImage(c, path)
	})
}

// serveShelfImage hands out one cacheable JPEG. The cache name carries the source PDF's stamp, so
// a rendered page is immutable for its URL and the response can be cached for a month without any
// revalidation machinery.
//
// serveShelfImage 交付一张可缓存的 JPEG。缓存文件名带着源 PDF 的时间戳，
// 所以对给定的 URL 来说页图是不变的，响应可以缓存一个月而不需要任何校验机制。
func serveShelfImage(c *gin.Context, path string) {
	c.Header("Cache-Control", "public, max-age=2592000")
	c.Header("X-Content-Type-Options", "nosniff")
	// image/avif is not in Go's built-in mime table on some toolchains; set it explicitly rather
	// than relying on the browser to sniff.
	//
	// image/avif 在某些工具链里不在 Go 内置 mime 表；显式设置，不靠浏览器嗅探。
	switch strings.ToLower(filepath.Ext(path)) {
	case ".avif":
		c.Header("Content-Type", "image/avif")
	case ".webp":
		c.Header("Content-Type", "image/webp")
	case ".png":
		c.Header("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		c.Header("Content-Type", "image/jpeg")
	}
	// c.File sets Content-Type from the extension and honours Range/If-Modified-Since.
	// c.File 按扩展名设置 Content-Type，并处理 Range 与 If-Modified-Since。
	c.File(path)
}

// blogOn is the cheap half of blogEnabled for routes that must not answer with an HTML error page
// — an image request handed a page of markup shows as a broken image, not as an explanation.
//
// blogOn 是 blogEnabled 的轻量版，供那些不能拿 HTML 错误页作答的路由使用——
// 一个图片请求收到一页标记，表现出来是裂图，而不是一句解释。
func blogOn(appContainer *app.App) bool {
	raw := &appContainer.Config().Blog
	return raw.Enabled != nil && *raw.Enabled
}

// writeBlogHTML is the shared response shape for the two shelf pages: rendered into memory first,
// no-cache, and the same headers the other blog pages use.
//
// writeBlogHTML 是书架两个页面共用的响应形态：先渲染到内存、no-cache，
// 以及与其它博客页面相同的响应头。
func writeBlogHTML(c *gin.Context, body []byte) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(body)
}
