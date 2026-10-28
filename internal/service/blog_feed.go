package service

import (
	"context"
	"regexp"
	"strings"
)

// Full-text feed assembly.
//
// The feed used to publish a 90-character excerpt per note, which made the subscription list
// indistinguishable from a table of contents: a reader had to open the site to learn anything.
// Everything here exists to turn that into a document someone can actually read in their reader.
//
// 全文订阅源的组装。
//
// 订阅源过去每条只发 90 字摘要，使订阅列表和目录没有区别：读者想知道任何内容都得打开网站。
// 这里的全部代码都是为了把它变成一份读者能在阅读器里真正读完的文档。

// blogFeedSummaryChars is the excerpt length kept alongside the full text.
//
// The excerpt is not dead weight once the body is present: reader list views render only this
// field, so it is what a scanning reader sees before deciding to open an item.
//
// blogFeedSummaryChars 是随全文一起保留的摘要长度。
//
// 正文存在之后摘要也不是废料：阅读器的列表视图只渲染这个字段，因此它就是快速浏览的读者
// 在决定展开某条之前看到的内容。
const blogFeedSummaryChars = 180

// BlogFeedEntry is one finished feed item.
//
// ContentHTML is already absolute-ized: a relative /blog/... or /api/share/file?... in a reader
// resolves against the reader's own origin, not the blog's, so every in-body link and image would
// be dead without this rewrite.
//
// Link and GUID are deliberately different strings. Link carries "?from=rss" so the visit log can
// tell a subscriber's click from a search engine's landing; GUID must not, because a reader keys
// "have I already shown this?" on the GUID, and changing it would re-deliver every existing item
// to every existing subscriber as if it were new.
//
// BlogFeedEntry 是一条已经完成组装的订阅项。
//
// ContentHTML 里的地址已经绝对化：阅读器会把正文里的 /blog/... 或 /api/share/file?... 按
// 阅读器自己的域名解析，而不是博客域名，因此不做这一步改写，正文里所有链接与图片都是死的。
//
// Link 与 GUID 刻意是两个不同的字符串。Link 带 "?from=rss"，让访问日志能分辨订阅者的点击与
// 搜索引擎的落地；GUID 绝不能带，因为阅读器以 GUID 判断「这条我展示过没有」，
// 改动它会把每一条既有内容当成新内容重新推送给每一位既有订阅者。
type BlogFeedEntry struct {
	Title       string
	Link        string
	GUID        string
	Published   string
	Summary     string
	ContentHTML string
	Author      string
	// CoverURL is the image (or video poster) a reader should show beside this item.
	//
	// It is an absolute address for the same reason ContentHTML is rewritten: a reader resolves a
	// bare path against its own origin. Empty means the article carries no image at all, and the
	// feed simply omits both cover elements for it.
	//
	// CoverURL 是阅读器应在这条内容旁展示的图片（或视频封面）。
	//
	// 它是绝对地址，理由与改写 ContentHTML 相同：阅读器会把裸路径按自己的域名解析。
	// 为空表示这篇文章根本没有任何图片，订阅源对它就完全省略两个封面元素。
	CoverURL string
	// CoverMIME is the declared content type for CoverURL, e.g. "image/png".
	//
	// It comes from the author's cover reference when there is one. For an article whose cover is
	// merely the first image in its body the served address carries no extension — it is
	// `/api/share/file?id=…&share_token=…` — so the fallback type is declared as JPEG rather than
	// guessed. Readers sniff the bytes anyway; the field exists so validators see a well-formed
	// enclosure.
	//
	// CoverMIME 是 CoverURL 声明的类型，如 "image/png"。
	//
	// 作者显式声明封面时来自那条引用。若封面只是正文里的第一张图，对外地址不带扩展名
	// ——它是 `/api/share/file?id=…&share_token=…`——因此兜底类型直接写 JPEG 而不是去猜。
	// 阅读器本来就会嗅探字节；这个字段存在的意义是让校验器看到合法的附件声明。
	CoverMIME string
}

// blogFeedAbsURL matches the opening of a root-relative href/src plus its first path character.
//
// The first character is consumed by the pattern instead of being guarded with a negative
// lookahead: Go's regexp is RE2, which has no lookahead at all, and `(?!` panics at init rather
// than failing to match. Requiring a following non-slash character is what excludes
// protocol-relative addresses ("//cdn.example.com/x"), which are already absolute and would
// otherwise become "https://blog//cdn.example.com/x".
//
// blogFeedAbsURL 匹配根相对 href/src 的开头及其路径首字符。
//
// 首字符由模式吃掉，而不是用负向先行断言排除：Go 的 regexp 是 RE2，根本没有先行断言，
// 而 `(?!` 会在 init 阶段 panic，而不是匹配失败。要求后面跟一个非斜杠字符，正是排除
// 协议相对地址（"//cdn.example.com/x"）的办法——它们本就是绝对的，
// 否则会变成 "https://blog//cdn.example.com/x"。
var blogFeedAbsURL = regexp.MustCompile(`(?i)(href|src)="/([^/])`)

// absolutizeBlogHTML prefixes every root-relative link and image with the site's base address.
//
// ReplaceAllStringFunc rather than ReplaceAllString: the replacement is built from the matched
// text plus a caller-supplied base, and a base containing "$" would be read as a capture-group
// reference by the template form.
//
// absolutizeBlogHTML 给每个根相对的链接与图片加上站点基础地址。
//
// 用 ReplaceAllStringFunc 而不是 ReplaceAllString：替换内容是「匹配到的文本 + 调用方给的 base」，
// 若 base 含 "$"，模板形式会把它当成捕获组引用。
func absolutizeBlogHTML(html, base string) string {
	base = strings.TrimRight(base, "/")
	if base == "" || html == "" {
		return html
	}
	return blogFeedAbsURL.ReplaceAllStringFunc(html, func(m string) string {
		// m is `<attr>="/<c>`: keep the attribute and the opening slash, insert the base
		// between them, then append the path from the captured character onwards.
		// m 形如 `<attr>="/<c>`：保留属性名与开头斜杠，在两者之间插入 base，
		// 再接上从被捕获字符开始的路径。
		i := strings.Index(m, `="/`)
		if i < 0 {
			return m
		}
		return m[:i+2] + base + "/" + m[i+3:]
	})
}

// blogPlainTextParagraphs renders a note's opening as plain text that keeps its paragraph breaks.
//
// This is blogPlainText's sibling, not its replacement. The home page wants one line because it
// lays the excerpt out inside a fixed-height card; a feed item wants the paragraph structure,
// because collapsing it produces a single wall of text that reads worse than no excerpt at all.
//
// blogPlainTextParagraphs 把笔记开头渲染成保留段落分隔的纯文本。
//
// 它与 blogPlainText 并列而不是取代它。首页要单行，因为摘要在固定高度的卡片里排版；
// 订阅项要保留段落结构，因为压平之后得到的是一堵文字墙，比不给摘要更难读。
func blogPlainTextParagraphs(content string, limit int) string {
	s := fenceRegex.ReplaceAllString(content, " ")
	s = frontmatterRegex.ReplaceAllString(s, "")
	// Before the markdown rules for the same reason blogPlainText does it: an HTML block may
	// wrap markdown-looking text, and the heading/list rules would then edit inside a tag pair.
	// 与 blogPlainText 同理，放在 markdown 规则之前：HTML 块可能包着看起来像 markdown 的文字，
	// 先跑标题/列表规则会改到标签对内部。
	s = stripHTML(s)
	s = mdHRRegex.ReplaceAllString(s, "\n\n")
	s = mdEmbedRegex.ReplaceAllString(s, " ")
	s = mdImageRegex.ReplaceAllString(s, " ")
	s = mdWikiRegex.ReplaceAllStringFunc(s, blogWikiDisplayText)
	s = mdLinkRegex.ReplaceAllString(s, "$1")
	s = mdHeadingRegex.ReplaceAllString(s, "")
	s = mdQuoteRegex.ReplaceAllString(s, "")
	s = mdListRegex.ReplaceAllString(s, "")

	// Split on newlines and rebuild paragraphs, because strings.Fields — which blogPlainText uses
	// — destroys the only signal that says "this is a new paragraph".
	//
	// 按行切分再拼回段落：blogPlainText 用的 strings.Fields 会销毁「这里换段了」这唯一的信号。
	paras := make([]string, 0, 8)
	cur := make([]string, 0, 8)
	for _, line := range strings.Split(s, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if len(cur) > 0 {
				paras = append(paras, strings.Join(cur, " "))
				cur = cur[:0]
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		paras = append(paras, strings.Join(cur, " "))
	}

	if limit <= 0 {
		return strings.Join(paras, "\n\n")
	}

	var b strings.Builder
	used := 0
	for _, p := range paras {
		r := []rune(p)
		if used+len(r) > limit {
			// A tail shorter than 20 characters reads as a fragment ("白内…"), so it is
			// dropped rather than appended.
			// 短于 20 字的尾巴读起来像残片（"白内…"），因此丢弃而不是接上去。
			if remain := limit - used; remain > 20 {
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				b.WriteString(strings.TrimRight(string(r[:remain]), " ，,。.") + "…")
			}
			break
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(p)
		used += len(r)
	}
	return b.String()
}

// GetBlogFeed returns the newest published articles with their full rendered bodies.
//
// It shares loadBlogSources with the home page rather than issuing its own query, so the feed can
// never disagree with the site about what is published: the same usability check, the same
// de-duplication by path, and the same exclusion of page notes apply in one place.
//
// A note whose body fails to render is still published, with its excerpt only. Dropping the item
// would hide a note that the site itself serves, and failing the whole feed because one note is
// malformed would take every other note down with it.
//
// GetBlogFeed 返回最新的已发布文章及其渲染好的完整正文。
//
// 它与首页共用 loadBlogSources 而不是自己发查询，使订阅源永远不可能和站点在「什么算已发布」
// 上产生分歧：可用性检查、按路径去重、页面笔记的排除，都只有一处定义。
//
// 正文渲染失败的笔记仍然发布，只是只有摘要。丢弃该条目等于隐藏一篇站点本身能打开的文章，
// 而因为一篇畸形笔记让整个订阅源失败，会把其余每一篇都一起拖下水。
func (s *shareService) GetBlogFeed(ctx context.Context, uid int64, baseURL string, limit int) ([]*BlogFeedEntry, error) {
	all, _, err := s.loadBlogSources(ctx, uid)
	if err != nil {
		return nil, err
	}
	sources := articlesOnly(all)

	if limit <= 0 || limit > len(sources) {
		limit = len(sources)
	}

	base := strings.TrimRight(baseURL, "/")
	home := base + "/blog"

	out := make([]*BlogFeedEntry, 0, limit)
	for _, src := range sources {
		if len(out) >= limit {
			break
		}
		canonical := home + "/" + src.Slug
		entry := &BlogFeedEntry{
			Title:     src.Title,
			Link:      canonical + "?from=rss",
			GUID:      canonical,
			Published: src.CreatedAt,
			Summary:   blogPlainTextParagraphs(src.Content, blogFeedSummaryChars),
			Author:    src.Vault,
		}
		if body, rerr := s.renderBlogPostHTML(ctx, src.Slug); rerr == nil {
			entry.ContentHTML = absolutizeBlogHTML(body, base)
		}
		feedCoverFor(entry, src.Cover, base)
		out = append(out, entry)
	}
	return out, nil
}

// blogFeedFallbackCoverMIME is the declared type when the cover is an in-body image.
//
// The served address has no extension, so there is nothing to read a type out of. JPEG is the
// honest default: nearly every note image in this vault is a screenshot or a phone photo, and a
// reader that trusts the header rather than the bytes is rarer than one that ignores it.
//
// blogFeedFallbackCoverMIME 是封面取自正文图片时声明的类型。
//
// 对外地址没有扩展名，读不出类型。JPEG 是诚实的默认值：这个仓库里几乎每张笔记图片都是截图
// 或手机照片，而信任响应头而不是字节的阅读器，比忽略这个字段的阅读器更少见。
const blogFeedFallbackCoverMIME = "image/jpeg"

// blogFeedImgSrc matches the opening of an <img> tag up to and including its src value.
//
// [^>]* rather than .*?: the rendered body is one long line for a short note, and a lazy any-char
// match would scan across tag boundaries to find a "src=" that belongs to a later element.
//
// blogFeedImgSrc 匹配一个 <img> 标签的开头，直到并包含它的 src 取值。
//
// 用 [^>]* 而不是 .*?：短笔记渲染出的正文是一整行，惰性任意字符匹配会跨过标签边界，
// 命中后面某个元素上的 "src="。
var blogFeedImgSrc = regexp.MustCompile(`(?i)<img\b[^>]*\bsrc\s*=\s*"([^"]+)"`)

// firstBlogImageSrc returns the first usable image address in rendered article HTML.
//
// data: URIs are skipped rather than returned: a reader would be handed a multi-kilobyte string
// in an attribute, and any image embedded that way is already inside the body the reader renders.
//
// firstBlogImageSrc 返回已渲染正文里第一个可用的图片地址。
//
// data: URI 被跳过而不是返回：阅读器会在属性里拿到一个几 KB 的字符串，而任何这样嵌入的图片
// 本就已经在阅读器要渲染的正文里面了。
func firstBlogImageSrc(html string) string {
	for _, m := range blogFeedImgSrc.FindAllStringSubmatch(html, 8) {
		src := strings.TrimSpace(m[1])
		if src == "" || strings.HasPrefix(strings.ToLower(src), "data:") {
			continue
		}
		return src
	}
	return ""
}

// feedCoverFor fills in an entry's cover, preferring the author's declaration over the body.
//
// The precedence is the product decision, restated in code: a note that declares `cover:` gets
// that image even when its body opens with a different one, and a note that declares nothing
// falls back to the first image it happens to contain. The fallback is why covers worked at all
// before the frontmatter key existed, and it is why the grid is not empty for the 150 notes that
// have not been annotated yet.
//
// feedCoverFor 填充条目的封面，作者显式声明的优先于正文。
//
// 这个优先级就是产品决定，在这里用代码重述一遍：声明了 `cover:` 的笔记得到那张图，
// 即使正文开头是另一张；什么都没声明的笔记退化到它碰巧包含的第一张图。正是这个退化
// 让封面键存在之前封面也能工作，也正是它让那 150 篇还没标注的笔记不会把网格留空。
func feedCoverFor(entry *BlogFeedEntry, cover *blogCover, base string) {
	if entry == nil {
		return
	}
	if cover != nil && cover.URL != "" {
		entry.CoverURL = absolutizeBlogURL(cover.URL, base)
		entry.CoverMIME = cover.MIME
		if entry.CoverMIME == "" {
			entry.CoverMIME = blogFeedFallbackCoverMIME
		}
		return
	}
	// ContentHTML is already absolutized, so the extracted address needs no further rewrite.
	// ContentHTML 已是绝对地址，因此提取出的地址不需要再改写。
	if src := firstBlogImageSrc(entry.ContentHTML); src != "" {
		entry.CoverURL = absolutizeBlogURL(src, base)
		entry.CoverMIME = blogFeedFallbackCoverMIME
	}
}

// absolutizeBlogURL prefixes one root-relative address with the site's base.
//
// Protocol-relative addresses ("//host/x") are returned untouched: they are already absolute, and
// prefixing them would produce "https://blog//host/x".
//
// absolutizeBlogURL 给单个根相对地址加上站点基础地址。
//
// 协议相对地址（"//host/x"）原样返回：它们本就是绝对的，加前缀会得到 "https://blog//host/x"。
func absolutizeBlogURL(u, base string) string {
	u = strings.TrimSpace(u)
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if u == "" || base == "" {
		return u
	}
	if !strings.HasPrefix(u, "/") || strings.HasPrefix(u, "//") {
		return u
	}
	return base + u
}

// renderBlogPostHTML renders one article exactly as the site renders it.
//
// It goes through RenderBlogBySlug rather than calling renderNoteBody directly. That is not a
// convenience: renderNoteBody operates on raw note Markdown and knows nothing about attachments,
// so a note written as `![[图.png]]` comes out with no <img> at all. The share pipeline is what
// resolves those references into /api/share/file?id=...&share_token=... and hands the renderer
// something it can turn into an image tag. Measured before this fix: the feed's copy of
// 《（7）图片排版》 was 3,377 bytes with zero images, while the article page for the same note
// carried twelve.
//
// renderBlogPostHTML 按站点完全一致的方式渲染一篇文章。
//
// 它走 RenderBlogBySlug，而不是直接调 renderNoteBody。这不是图方便：renderNoteBody 处理的是
// 原始 Markdown，对附件一无所知，因此写成 `![[图.png]]` 的笔记渲染出来根本没有 <img>。
// 真正把这些引用解析成 /api/share/file?id=...&share_token=... 、再交给渲染器变成图片标签的，
// 是分享管线。修复前实测：订阅源里《（7）图片排版》那篇是 3377 字节、零张图片，
// 而同一篇笔记的文章页上有十二张。
func (s *shareService) renderBlogPostHTML(ctx context.Context, slug string) (string, error) {
	post, err := s.RenderBlogBySlug(ctx, slug)
	if err != nil {
		return "", err
	}
	if post == nil {
		return "", nil
	}
	return post.HTML, nil
}
