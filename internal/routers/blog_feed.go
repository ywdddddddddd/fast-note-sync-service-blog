package routers

import (
	"encoding/json"
	"encoding/xml"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
	"github.com/haierkeys/fast-note-sync-service/internal/service"
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

// blogFeedMaxItems caps the feed length.
//
// It came down from 50 when the feed went full-text: twelve of these notes average roughly
// 5,700 characters, so a full-body item costs about 25 KB and 50 of them made every poll a
// megabyte. Fifteen is also the honest number for this format — a subscriber drains the backlog
// on day one and sees one or two new items a day afterwards, so the extra 35 slots were paying
// bandwidth on every request to cover a window nobody scrolls back through.
//
// blogFeedMaxItems 限制订阅源条数。
//
// 全文订阅上线时它从 50 降到 15：本仓库这些笔记平均约 5700 字，因此一条全文项约 25 KB，
// 50 条会让每次轮询都变成一兆字节。15 对这个格式也是诚实的数字——订阅者第一天拉完历史，
// 之后每天只见一两篇新的，多出来的 35 个位置是在为一段没人回翻的窗口反复支付带宽。
const blogFeedMaxItems = 15

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Content string     `xml:"xmlns:content,attr"`
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
	Title       string      `xml:"title"`
	Link        string      `xml:"link"`
	GUID        rssGUID     `xml:"guid"`
	PubDate     string      `xml:"pubDate,omitempty"`
	Description string      `xml:"description,omitempty"`
	Author      string      `xml:"author,omitempty"`
	Content     *rssEncoded `xml:"content:encoded,omitempty"`
}

// rssEncoded carries full article HTML in the content module's element.
//
// The tag name is written with its prefix literally. encoding/xml does not resolve prefixes, so
// what makes this document valid is the xmlns:content declaration on <rss>, not anything the
// encoder knows about the content module.
//
// The value is CDATA rather than escaped text: article HTML is tag-dense, and escaping would turn
// every "<p>" into "&lt;p&gt;" — roughly a third more bytes on a document that is already the
// largest thing this site serves. The one sequence CDATA cannot contain is "]]>", and a "<p>"
// stream never produces it, so it is escaped defensively rather than assumed away.
//
// rssEncoded 在内容模块的元素里承载完整文章 HTML。
//
// 标签名连同前缀按字面书写。encoding/xml 不解析前缀，因此使这份文档合法的是 <rss> 上的
// xmlns:content 声明，而不是编码器对内容模块的任何了解。
//
// 值用 CDATA 而不是转义文本：文章 HTML 标签密集，转义会把每个 "<p>" 变成 "&lt;p&gt;"——
// 在一份本就是本站最大响应的文档上多出约三分之一字节。CDATA 唯一不能包含的序列是 "]]>"，
// 而 "<p>" 流永远不会产生它，所以这里只做防御性转义，而不是假定它不存在。
type rssEncoded struct {
	XMLName xml.Name `xml:"content:encoded"`
	Value   string   `xml:",cdata"`
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

// buildBlogFeed renders the feed document for a set of already-rendered entries.
//
// Entries whose Published value failed to parse are still published, just without a pubDate:
// dropping a note from the feed because its timestamp was odd is worse than omitting one field.
//
// lastBuildDate is the moment the document was built, not the newest item's date. The RSS
// specification defines it as when the content was last generated, and readers that conditionally
// refetch on it would stop noticing new items if it only ever repeated the latest publish date.
//
// buildBlogFeed 为一组已渲染完成的条目渲染订阅源文档。
//
// Published 解析失败的条目仍然发布，只是不带 pubDate：因为时间戳格式异常就把笔记从订阅源里
// 丢掉，比少一个字段更糟。
//
// lastBuildDate 是文档构建的时刻，不是最新条目的日期。RSS 规范把它定义为内容最后生成的
// 时间，而会据此做条件重取的阅读器，在它只是重复最新发布日的情况下将不再察觉新条目。
func buildBlogFeed(entries []*service.BlogFeedEntry, siteTitle, baseURL string) ([]byte, error) {
	base := strings.TrimRight(baseURL, "/")
	home := base + "/blog"
	feedURL := home + "/rss.xml"

	limit := len(entries)
	if limit > blogFeedMaxItems {
		limit = blogFeedMaxItems
	}

	items := make([]rssItem, 0, limit)
	for _, e := range entries[:limit] {
		if e == nil {
			continue
		}
		item := rssItem{
			Title:       e.Title,
			Link:        e.Link,
			GUID:        rssGUID{Value: e.GUID, IsPermaLink: "true"},
			Description: e.Summary,
			Author:      e.Author,
		}
		if e.Published != "" {
			if t, err := time.Parse("2006-01-02", e.Published); err == nil {
				item.PubDate = t.Format(rfc1123z)
			}
		}
		if e.ContentHTML != "" {
			item.Content = &rssEncoded{Value: cdataSafe(e.ContentHTML)}
		}
		items = append(items, item)
	}

	channel := rssChannel{
		Title:         siteTitle,
		Link:          home,
		Description:   siteTitle + " — 笔记订阅",
		Language:      "zh-CN",
		LastBuildDate: time.Now().UTC().Format(rfc1123z),
		AtomLink:      rssAtom{Href: feedURL, Rel: "self", Type: "application/rss+xml"},
		Items:         items,
	}

	body, err := xml.MarshalIndent(rssFeed{
		Version: "2.0",
		Atom:    "http://www.w3.org/2005/Atom",
		Content: "http://purl.org/rss/1.0/modules/content/",
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

// cdataSafe rewrites the one sequence a CDATA section cannot contain.
//
// A "]]>" inside a CDATA section would close it early and leave the rest of the article as
// markup, so it cannot be passed through. Splitting the section around it — the technique that
// works when writing XML by hand — does not work here, because the value still goes through
// encoding/xml, which wraps whatever it is given in another CDATA layer; measured, the split
// came back as the literal text "]]]]><![CDATA[>" instead of "]]>".
//
// So the sequence is replaced with its HTML entity rather than escaped around. "&gt;" renders as
// ">" in every reader, so the article reads identically, and the byte sequence that would break
// the document no longer exists.
//
// cdataSafe 改写 CDATA 段唯一无法包含的序列。
//
// CDATA 段里的 "]]>" 会提前关闭它，把文章剩余部分留成标记，因此不能原样通过。把它切开——
// 手工书写 XML 时可行的技巧——在这里无效，因为这个值仍要经过 encoding/xml，
// 而它会再包一层 CDATA；实测切开之后拿回来的是字面量 "]]]]><![CDATA[>" 而不是 "]]>"。
//
// 所以这里把该序列换成它的 HTML 实体，而不是绕着它转义。"&gt;" 在每个阅读器里都渲染成 ">"，
// 文章读起来完全一致，而那个会破坏文档的字节序列已经不存在了。
func cdataSafe(s string) string {
	if !strings.Contains(s, "]]>") {
		return s
	}
	return strings.ReplaceAll(s, "]]>", "]]&gt;")
}

// serveBlogFeed writes the RSS document for the built-in blog.
//
// The document is cached for blogFeedCacheTTL. Rendering fifteen full articles costs whole
// seconds — measured at 4.9 s cold, 263 ms warm on this host — because each item goes through the
// share pipeline: token generation, attachment resolution, transclusion expansion and a wiki-link
// pass over the note index. Readers poll far more often than notes change, so every one of those
// seconds would be spent rebuilding identical bytes.
//
// Ten minutes, not ninety seconds: at ninety seconds almost every poll is a cold one, because
// Feedly and friends poll on intervals of tens of minutes — the cache would never hit while the
// cost stayed. Ten minutes also costs nothing in freshness that RSS does not already cost: a
// subscriber's client decides when to fetch, so the feed is minutes-to-hours behind regardless.
// The only property this window can violate is "a note published now appears instantly", and ten
// minutes is well inside the delay a reader already expects from a feed.
//
// 文档会缓存 blogFeedCacheTTL 这么久。渲染十五篇全文的代价是以秒计的——本机实测冷 4.9 秒、
// 热 263 毫秒——因为每一条都要经过分享管线：生成令牌、解析附件、展开嵌入，并对笔记索引做
// 一遍 wiki 链接处理。阅读器轮询的频率远高于笔记变化的频率，因此这些秒数里每一次都花在
// 重建完全相同的字节上。
//
// 十分钟，而不是九十秒：九十秒的话几乎每次轮询都是冷的，因为 Feedly 之类的轮询间隔是几十分钟
// ——缓存永远命中不了，而代价照付。十分钟在新鲜度上也并不额外损失什么 RSS 本来就在损失的东西：
// 何时拉取由订阅者的客户端决定，所以订阅源本来就会晚几分钟到几小时。
// 这个窗口唯一可能破坏的性质是「此刻发布的笔记立刻出现」，而十分钟远在读者对订阅源
// 本就预期的延迟之内。
const blogFeedCacheTTL = 10 * time.Minute

var blogFeedCache struct {
	mu      sync.Mutex
	key     string
	body    []byte
	builtAt time.Time
}

// cachedFeedBody returns a previously built document when it is still fresh.
//
// A stale entry is kept rather than cleared: if rendering fails, serving a 30-second-old document
// is strictly better than serving an error page to every subscriber at once.
//
// cachedFeedBody 在文档仍然新鲜时返回上次构建的结果。
//
// 过期条目保留而不是清除：如果渲染失败，提供一份 30 秒前的文档严格优于同时向每位订阅者
// 提供一个错误页。
func cachedFeedBody(key string) ([]byte, bool) {
	blogFeedCache.mu.Lock()
	defer blogFeedCache.mu.Unlock()
	if blogFeedCache.body == nil || blogFeedCache.key != key {
		return nil, false
	}
	if time.Since(blogFeedCache.builtAt) > blogFeedCacheTTL {
		return nil, false
	}
	return blogFeedCache.body, true
}

func storeFeedBody(key string, body []byte) {
	blogFeedCache.mu.Lock()
	defer blogFeedCache.mu.Unlock()
	blogFeedCache.key = key
	blogFeedCache.body = body
	blogFeedCache.builtAt = time.Now()
}

// staleFeedBody returns whatever was cached, ignoring freshness.
// staleFeedBody 返回缓存中的内容，忽略新鲜度。
func staleFeedBody(key string) []byte {
	blogFeedCache.mu.Lock()
	defer blogFeedCache.mu.Unlock()
	if blogFeedCache.key != key {
		return nil
	}
	return blogFeedCache.body
}

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

	// The cache key covers everything the document depends on, so switching the site title or
	// the public address invalidates it instead of serving a document that describes a different
	// site.
	// 缓存键覆盖文档依赖的一切，因此改站点标题或对外地址会让它失效，
	// 而不是继续提供一份描述着另一个站点的文档。
	cacheKey := strconv.FormatInt(uid, 10) + "|" + base + "|" + siteTitle

	if body, hit := cachedFeedBody(cacheKey); hit {
		writeFeedDocument(c, body)
		return
	}

	// GetBlogFeed rather than GetBlogHome: the feed needs rendered bodies, and GetBlogHome's
	// entry struct carries only an excerpt. It shares the same source list underneath, so the
	// feed and the site still cannot disagree about what is published, while the graph layout,
	// the heat map and the tag cloud stay out of this path entirely.
	//
	// 用 GetBlogFeed 而不是 GetBlogHome：订阅源需要渲染好的正文，而 GetBlogHome 的条目结构里
	// 只有摘要。它在底层共用同一份来源清单，因此订阅源与站点依然不可能在「什么算已发布」上
	// 产生分歧，同时图谱布局、热力图与标签云完全不会进入这条路径。
	entries, err := shareSvc.GetBlogFeed(c.Request.Context(), uid, base, blogFeedMaxItems)
	if err != nil {
		if body := staleFeedBody(cacheKey); body != nil {
			writeFeedDocument(c, body)
			return
		}
		blogFail(c, appContainer, err.Error())
		return
	}

	body, err := buildBlogFeed(entries, siteTitle, base)
	if err != nil {
		// Render into memory first, then write: an encoding failure must not leave a truncated
		// document that readers cache as "the feed is broken".
		// 先渲染到内存再写出：编码失败不能留下一个被阅读器缓存为「订阅源坏了」的半截文档。
		if stale := staleFeedBody(cacheKey); stale != nil {
			writeFeedDocument(c, stale)
			return
		}
		blogFail(c, appContainer, err.Error())
		return
	}

	storeFeedBody(cacheKey, body)
	writeFeedDocument(c, body)
}

func writeFeedDocument(c *gin.Context, body []byte) {
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
