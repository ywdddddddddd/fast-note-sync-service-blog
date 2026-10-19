package routers

import (
	"encoding/xml"
	"net/http"
	"strings"
	"time"

	"encoding/json"
	"html/template"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// RSS 2.0 feed for the built-in blog.
//
// Why encoding/xml instead of a text/template: every field is escaped by the encoder, so a note
// title containing "&" or "<" cannot break the document. html/template would emit HTML entities
// (&amp;) inside an XML document, which some readers display literally.
//
// 内置博客的 RSS 2.0 订阅源。
//
// 为什么用 encoding/xml 而不是 text/template：编码器会转义每个字段，因此标题里含有 "&" 或
// "<" 的笔记不会破坏文档。html/template 会在 XML 文档里输出 HTML 实体（&amp;），
// 部分阅读器会把它们原样显示出来。

// blogFeedMaxItems caps the feed length. Readers poll repeatedly, and an unbounded feed turns
// every poll into a full-vault transfer.
//
// blogFeedMaxItems 限制订阅源条数。阅读器会反复轮询，不设上限会让每次轮询都变成整库传输。
const blogFeedMaxItems = 50

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	AtomLink      rssAtom   `xml:"atom:link"`
	Items         []rssItem `xml:"item"`
}

type rssAtom struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        rssGUID `xml:"guid"`
	PubDate     string  `xml:"pubDate,omitempty"`
	Description string  `xml:"description,omitempty"`
	Author      string  `xml:"author,omitempty"`
}

// encoding/xml has no "-" name for a chardata field: the field name must be empty so the
// struct contributes only its attribute plus the element text. Writing `xml:"-,chardata"`
// compiles but Marshal fails at runtime with "invalid tag in field Value".
// encoding/xml 没有 chardata 字段的 "-" 写法：字段名必须留空，结构体才会只贡献属性加元素文本。
// 写成 `xml:"-,chardata"` 能编译，但 Marshal 会在运行时报 "invalid tag in field Value"。
type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink string `xml:"isPermaLink,attr"`
}

// rfc1123z is the date format RSS readers expect.
// rfc1123z 是 RSS 阅读器期望的日期格式。
const rfc1123z = "Mon, 02 Jan 2006 15:04:05 -0700"

// buildBlogFeed renders the feed document for a set of blog entries.
//
// Entries whose CreatedAt failed to parse are still published, just without a pubDate: dropping
// a note from the feed because its timestamp was odd is worse than omitting one field.
//
// buildBlogFeed 为一组博客条目渲染订阅源文档。
//
// CreatedAt 解析失败的条目仍然发布，只是不带 pubDate：因为时间戳格式异常就把笔记从订阅源里
// 丢掉，比少一个字段更糟。
func buildBlogFeed(entries []*dto.BlogEntryDTO, siteTitle, baseURL string) ([]byte, error) {
	base := strings.TrimRight(baseURL, "/")
	home := base + "/blog"
	feedURL := home + "/rss.xml"

	limit := len(entries)
	if limit > blogFeedMaxItems {
		limit = blogFeedMaxItems
	}

	items := make([]rssItem, 0, limit)
	var latest time.Time
	for _, e := range entries[:limit] {
		if e == nil {
			continue
		}
		link := home + "/" + e.Slug
		item := rssItem{
			Title:       e.Title,
			Link:        link,
			GUID:        rssGUID{Value: link, IsPermaLink: "true"},
			Description: e.Summary,
			Author:      e.Vault,
		}
		if e.CreatedAt != "" {
			if t, err := time.Parse("2006-01-02", e.CreatedAt); err == nil {
				item.PubDate = t.Format(rfc1123z)
				if t.After(latest) {
					latest = t
				}
			}
		}
		items = append(items, item)
	}

	channel := rssChannel{
		Title:       siteTitle,
		Link:        home,
		Description: siteTitle + " — 笔记订阅",
		Language:    "zh-CN",
		AtomLink:    rssAtom{Href: feedURL, Rel: "self", Type: "application/rss+xml"},
		Items:       items,
	}
	if !latest.IsZero() {
		channel.LastBuildDate = latest.Format(rfc1123z)
	}

	body, err := xml.MarshalIndent(rssFeed{
		Version: "2.0",
		Atom:    "http://www.w3.org/2005/Atom",
		Channel: channel,
	}, "", "  ")
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, len(body)+48)
	out = append(out, `<?xml version="1.0" encoding="UTF-8"?>`...)
	out = append(out, '\n')
	out = append(out, body...)
	out = append(out, '\n')
	return out, nil
}

// serveBlogFeed writes the RSS document for the built-in blog.
//
// The feed follows the same visibility switch as the pages: when blog.indexable is off the feed
// is not served, because "do not index me" and "hand out a subscription document" are the same
// decision made from opposite sides.
//
// serveBlogFeed 写出内置博客的 RSS 文档。
//
// 订阅源与页面遵循同一个可见性开关：blog.indexable 关闭时不提供订阅源，因为「不要收录我」
// 与「对外发放订阅文档」是从两侧表述的同一个决定。
func serveBlogFeed(c *gin.Context, appContainer *app.App) {
	cfg, ok := blogEnabled(c, appContainer)
	if !ok {
		return
	}
	if !blogIndexable(cfg.Indexable) {
		blogErrorPage(c, appContainer, http.StatusNotFound, "订阅源未开放",
			"订阅源当前不可用",
			"这个站点没有开放对外订阅。",
			[]string{"站点可以随时重新开启对外收录，开启后此地址立即恢复。"},
		)
		return
	}

	shareSvc := appContainer.ShareService
	if shareSvc == nil {
		blogFail(c, appContainer, "share service unavailable")
		return
	}

	uid := blogUID(cfg.UID)
	siteTitle := blogSiteTitle(cfg.Title)
	base := blogBaseURL(c, appContainer)

	// GetBlogHome rather than ListBlogEntries: only the home assembly fills Summary (the index
	// list, which also serves wiki-link resolution, does not), and a feed of bare titles is not
	// worth subscribing to. maxNodes=0 keeps the graph layout out of this path entirely — the
	// guard added in stage 2 means nothing gets relaxed — and publishedOnly stays true so no
	// anonymous nodes are minted for unpublished notes.
	//
	// 这里用 GetBlogHome 而不是 ListBlogEntries：只有首页组装会填充 Summary
	// （同时承担 wiki 内链解析的索引列表不填），而一份只有标题的订阅源不值得订阅。
	// maxNodes=0 让图谱布局完全不进入这条路径——阶段 2 加的守护意味着不会有任何松弛计算——
	// 且 publishedOnly 保持 true，不会为未发布笔记铸造匿名节点。
	home, err := shareSvc.GetBlogHome(c.Request.Context(), uid, base, true, 0)
	if err != nil {
		blogFail(c, appContainer, err.Error())
		return
	}

	body, err := buildBlogFeed(home.Entries, siteTitle, base)
	if err != nil {
		// Render into memory first, then write: an encoding failure must not leave a truncated
		// document that readers cache as "the feed is broken".
		// 先渲染到内存再写出：编码失败不能留下一个被阅读器缓存为「订阅源坏了」的半截文档。
		blogFail(c, appContainer, err.Error())
		return
	}

	c.Header("Content-Type", "application/rss+xml; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(body)
}

// ── JSON-LD (schema.org) ────────────────────────────────────────────────
// Structured data is what lets a search engine render a result with the site name, the article
// date and the description instead of a bare URL. Built with encoding/json so every string is
// escaped by the marshaller; the result is handed to the template as template.JS.
//
// JSON-LD（schema.org）────────────────────────────────────────────────
// 结构化数据让搜索引擎能渲染出带站点名、文章日期与摘要的结果，而不是一串裸地址。
// 用 encoding/json 构建，因此每个字符串都由序列化器转义；结果以 template.JS 交给模板。

// blogJSONLD marshals a schema.org document. It returns an empty value on failure rather than
// a broken script block: a malformed JSON-LD document is worse than no document at all.
//
// blogJSONLD 序列化一个 schema.org 文档。失败时返回空值而不是一个坏掉的脚本块：
// 畸形的 JSON-LD 文档比没有文档更糟。
func blogJSONLD(doc map[string]any) template.JS {
	body, err := json.Marshal(doc)
	if err != nil {
		return template.JS("")
	}
	return template.JS(body)
}

// blogHomeJSONLD describes the blog index as a WebSite plus a Blog.
// blogHomeJSONLD 把博客首页描述为 WebSite 加 Blog。
func blogHomeJSONLD(siteTitle, homeURL, desc string, noteCount int) template.JS {
	return blogJSONLD(map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Blog",
		"name":        siteTitle,
		"url":         homeURL,
		"description": desc,
		"inLanguage":  "zh-CN",
		"blogPost":    []any{},
		"about": map[string]any{
			"@type":         "Thing",
			"description":   desc,
			"numberOfItems": noteCount,
		},
	})
}

// blogPostJSONLD describes one article as a BlogPosting.
// blogPostJSONLD 把单篇文章描述为 BlogPosting。
func blogPostJSONLD(siteTitle, postTitle, postURL, desc, published string) template.JS {
	doc := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "BlogPosting",
		"headline":    postTitle,
		"url":         postURL,
		"description": desc,
		"inLanguage":  "zh-CN",
		"isPartOf": map[string]any{
			"@type": "Blog",
			"name":  siteTitle,
		},
	}
	if published != "" {
		doc["datePublished"] = published
		doc["dateModified"] = published
	}
	return blogJSONLD(doc)
}

// blogPlainTextFromHTML extracts a short plain-text description from rendered article HTML.
//
// The article body has already been through goldmark, so its text is HTML; the description meta
// tag must not contain markup. Entities are left as-is because the template escapes again when
// it writes the attribute, and decoding here would double-unescape a literal "&amp;" in the note.
//
// blogPlainTextFromHTML 从已渲染的文章 HTML 中提取一段简短的纯文本描述。
//
// 正文已经过 goldmark，因此其文本是 HTML；description meta 标签不能包含标记。
// 实体保持原样：模板写属性时会再次转义，而在这里解码会把笔记里字面的 "&amp;" 双重反转。
func blogPlainTextFromHTML(html string) string {
	var b strings.Builder
	depth := 0
	for _, r := range html {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	s := strings.Join(strings.Fields(b.String()), " ")
	const limit = 140
	runes := []rune(s)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return s
}
