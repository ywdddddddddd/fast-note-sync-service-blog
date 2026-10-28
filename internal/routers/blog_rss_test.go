package routers

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/service"
)

// The feed must be a well-formed XML document. This is the test that would have caught the
// CDATA terminator problem, an unescaped ampersand in a title, and a mistyped namespace prefix —
// all of which produce a document that parses fine in Go's head but fails in every real reader.
//
// 订阅源必须是格式良好的 XML 文档。这条测试能抓住 CDATA 终止符问题、标题里未转义的 & 号、
// 以及拼错命名空间前缀——这三者都会产出一份在 Go 里看着没问题、但在每个真实阅读器里都失败的文档。
func TestBuildBlogFeedIsWellFormed(t *testing.T) {
	entries := []*service.BlogFeedEntry{
		{
			Title:       "带 & 和 < 的标题",
			Link:        "https://yaowendao.asia/blog/1.1-abc?from=rss",
			GUID:        "https://yaowendao.asia/blog/1.1-abc",
			Published:   "2026-10-27",
			Summary:     "摘要第一段\n\n摘要第二段",
			ContentHTML: `<p>正文</p><img src="https://yaowendao.asia/api/share/file?id=1">`,
			Author:      "口腔",
		},
	}

	body, err := buildBlogFeed(entries, "问道", "https://yaowendao.asia")
	if err != nil {
		t.Fatalf("buildBlogFeed: %v", err)
	}

	var doc struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title   string `xml:"title"`
				Link    string `xml:"link"`
				GUID    string `xml:"guid"`
				Content string `xml:"encoded"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("生成的文档无法解析: %v\n%s", err, body)
	}

	if doc.Version != "2.0" {
		t.Errorf("version = %q, want 2.0", doc.Version)
	}
	if len(doc.Channel.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(doc.Channel.Items))
	}
	it := doc.Channel.Items[0]
	if it.Title != "带 & 和 < 的标题" {
		t.Errorf("标题往返后变了: %q", it.Title)
	}
	if it.Content == "" {
		t.Error("content:encoded 为空：全文没有进订阅源")
	}
	if !strings.Contains(it.Content, "<p>正文</p>") {
		t.Errorf("正文内容丢失: %q", it.Content)
	}
}

// The document must declare the content module's namespace, or the prefixed element is not
// namespace-well-formed and strict readers reject the whole feed.
//
// 文档必须声明内容模块的命名空间，否则带前缀的元素不满足命名空间良构性，严格的阅读器会拒收。
func TestBuildBlogFeedDeclaresContentNamespace(t *testing.T) {
	body, err := buildBlogFeed([]*service.BlogFeedEntry{{
		Title: "t", Link: "https://x/blog/1", GUID: "https://x/blog/1",
		ContentHTML: "<p>x</p>",
	}}, "问道", "https://x")
	if err != nil {
		t.Fatalf("buildBlogFeed: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, `xmlns:content="http://purl.org/rss/1.0/modules/content/"`) {
		t.Error("缺少 xmlns:content 声明")
	}
	if !strings.Contains(s, "<content:encoded>") {
		t.Error("缺少 content:encoded 元素")
	}
	if !strings.Contains(s, "<![CDATA[") {
		t.Error("正文没有用 CDATA 包裹")
	}
}

// A "]]>" inside an article would close the CDATA section early and leave the rest of the note as
// markup. cdataSafe splits the section around it so the document stays parseable.
//
// 正文里出现 "]]>" 会提前关闭 CDATA 段，把笔记剩余部分留成标记。
// cdataSafe 绕开它把段切开，让文档仍然可解析。
func TestBuildBlogFeedSurvivesCDATATerminator(t *testing.T) {
	body, err := buildBlogFeed([]*service.BlogFeedEntry{{
		Title: "t", Link: "https://x/blog/1", GUID: "https://x/blog/1",
		ContentHTML: "<p>a]]>b</p>",
	}}, "问道", "https://x")
	if err != nil {
		t.Fatalf("buildBlogFeed: %v", err)
	}

	var doc struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Items []struct {
				Content string `xml:"encoded"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("含 ]]> 的正文把文档弄坏了: %v\n%s", err, body)
	}
	if len(doc.Channel.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(doc.Channel.Items))
	}
	// The terminator is rewritten to its HTML entity rather than escaped around, because the
	// value is wrapped in CDATA by the encoder afterwards: "&gt;" renders as ">" everywhere,
	// so the article reads identically while the fatal byte sequence is gone.
	// 终止符被改写成 HTML 实体，而不是绕着它转义，因为值随后会被编码器包进 CDATA：
	// "&gt;" 在任何地方都渲染成 ">"，文章读起来一致，而致命的字节序列已经不存在。
	if got, want := doc.Channel.Items[0].Content, "<p>a]]&gt;b</p>"; got != want {
		t.Errorf("CDATA 终止符处理错误: got %q, want %q", got, want)
	}
}

// link and guid must differ, and they must differ in exactly one way: the click-through marker.
//
// This is the single most damaging mistake available in this change. A reader keys "already
// seen?" on the guid, so putting the marker in both would re-deliver the entire backlog to every
// existing subscriber the first time the feed was fetched.
//
// link 与 guid 必须不同，且只能差一处：点击来源标记。
//
// 这是本次改动里破坏力最大的一个错误。阅读器以 guid 判断「是否已经展示过」，
// 两处都带标记会让每位既有订阅者在第一次拉取时收到整份历史当作新内容。
func TestBuildBlogFeedKeepsGUIDStable(t *testing.T) {
	body, _ := buildBlogFeed([]*service.BlogFeedEntry{{
		Title: "t",
		Link:  "https://x/blog/1.2-abcd?from=rss",
		GUID:  "https://x/blog/1.2-abcd",
	}}, "问道", "https://x")

	s := string(body)
	if !strings.Contains(s, `<link>https://x/blog/1.2-abcd?from=rss</link>`) {
		t.Errorf("link 上没有点击来源标记\n%s", s)
	}
	if !strings.Contains(s, `<guid isPermaLink="true">https://x/blog/1.2-abcd</guid>`) {
		t.Errorf("guid 不稳定或带上了标记\n%s", s)
	}
	if strings.Contains(s, `<guid isPermaLink="true">https://x/blog/1.2-abcd?from=rss</guid>`) {
		t.Error("guid 里混进了 ?from=rss：这会让所有老订阅者收到整份历史重推")
	}
}

// lastBuildDate must describe when the document was generated, not the newest item's date.
//
// lastBuildDate 必须描述文档生成的时间，而不是最新条目的日期。
func TestBuildBlogFeedLastBuildDateIsNow(t *testing.T) {
	body, _ := buildBlogFeed([]*service.BlogFeedEntry{{
		Title: "t", Link: "https://x/blog/1", GUID: "https://x/blog/1",
		Published: "2020-01-01",
	}}, "问道", "https://x")

	s := string(body)
	// Extract the element rather than searching the whole document: the item's pubDate also
	// contains "Wed, 01 Jan 2020", so a bare Contains() reports success no matter what
	// lastBuildDate holds — which is exactly how this test passed while the bug was present.
	// 要取元素内容而不是全文搜索：条目的 pubDate 里同样含 "Wed, 01 Jan 2020"，因此裸用
	// Contains() 无论 lastBuildDate 是什么都会通过——这个 bug 存在时测试正是这样通过了的。
	const open, close = "<lastBuildDate>", "</lastBuildDate>"
	i := strings.Index(s, open)
	j := strings.Index(s, close)
	if i < 0 || j < 0 || j < i {
		t.Fatalf("缺少 lastBuildDate\n%s", s)
	}
	got := s[i+len(open) : j]
	if got == "Wed, 01 Jan 2020 00:00:00 +0000" {
		t.Error("lastBuildDate 仍在复用最新条目的日期")
	}
	if !strings.Contains(got, "+0000") {
		t.Errorf("lastBuildDate 不是 RFC1123Z 格式: %q", got)
	}
}

// The cap is what keeps a full-text feed from becoming a megabyte per poll.
// 上限是全文订阅源不会变成每次轮询一兆字节的原因。
func TestBuildBlogFeedCapsItems(t *testing.T) {
	entries := make([]*service.BlogFeedEntry, 0, blogFeedMaxItems+10)
	for i := 0; i < blogFeedMaxItems+10; i++ {
		entries = append(entries, &service.BlogFeedEntry{
			Title: "t", Link: "https://x/blog/1", GUID: "https://x/blog/1",
		})
	}
	body, err := buildBlogFeed(entries, "问道", "https://x")
	if err != nil {
		t.Fatalf("buildBlogFeed: %v", err)
	}
	if got := strings.Count(string(body), "<item>"); got != blogFeedMaxItems {
		t.Errorf("item 数 = %d, want %d", got, blogFeedMaxItems)
	}
}

// An entry with no rendered body must still publish, with its excerpt only. Dropping it would
// hide a note the site itself serves.
//
// 没有渲染出正文的条目仍然要发布，只是只有摘要。丢弃它等于隐藏一篇站点自己能打开的文章。
func TestBuildBlogFeedPublishesExcerptWhenBodyMissing(t *testing.T) {
	body, err := buildBlogFeed([]*service.BlogFeedEntry{{
		Title: "只有摘要", Link: "https://x/blog/1", GUID: "https://x/blog/1",
		Summary: "这是摘要",
	}}, "问道", "https://x")
	if err != nil {
		t.Fatalf("buildBlogFeed: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "<description>这是摘要</description>") {
		t.Errorf("摘要没有进文档\n%s", s)
	}
	if strings.Contains(s, "<content:encoded>") {
		t.Error("空正文不该产出一个空的 content:encoded 元素")
	}
}

// A cover has to reach the reader in both dialects, and the document must declare the Media RSS
// namespace or the prefixed element is not well-formed.
//
// 封面必须同时以两种方言送达阅读器，且文档必须声明 Media RSS 命名空间，否则带前缀的元素
// 就不是格式良好的。
func TestBuildBlogFeedCarriesCover(t *testing.T) {
	body, err := buildBlogFeed([]*service.BlogFeedEntry{{
		Title: "有封面", Link: "https://x/blog/1", GUID: "https://x/blog/1",
		CoverURL:  "https://x/api/share/file?id=7&share_token=t",
		CoverMIME: "image/png",
	}}, "问道", "https://x")
	if err != nil {
		t.Fatalf("buildBlogFeed: %v", err)
	}

	var doc struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Items []struct {
				Enclosure struct {
					URL    string `xml:"url,attr"`
					Type   string `xml:"type,attr"`
					Length string `xml:"length,attr"`
				} `xml:"enclosure"`
				Media struct {
					URL    string `xml:"url,attr"`
					Type   string `xml:"type,attr"`
					Medium string `xml:"medium,attr"`
				} `xml:"content"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("生成的文档无法解析: %v\n%s", err, body)
	}
	if len(doc.Channel.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(doc.Channel.Items))
	}

	it := doc.Channel.Items[0]
	const wantURL = "https://x/api/share/file?id=7&share_token=t"
	if it.Enclosure.URL != wantURL {
		t.Errorf("enclosure url = %q, want %q", it.Enclosure.URL, wantURL)
	}
	if it.Enclosure.Type != "image/png" {
		t.Errorf("enclosure type = %q, want image/png", it.Enclosure.Type)
	}
	// The schema requires length, and "0" is the conventional unknown.
	// 模式要求 length，而 "0" 是惯例的未知值。
	if it.Enclosure.Length != "0" {
		t.Errorf("enclosure length = %q, want 0", it.Enclosure.Length)
	}
	if it.Media.URL != wantURL {
		t.Errorf("media:content url = %q, want %q", it.Media.URL, wantURL)
	}
	if it.Media.Medium != "image" {
		t.Errorf("media:content medium = %q, want image", it.Media.Medium)
	}
}

// An article with no image at all must omit both cover elements rather than emit an empty url
// attribute, which some readers treat as a broken enclosure and drop the whole item.
//
// 完全没有图片的文章必须两个封面元素都不发，而不是发出一个空的 url 属性——
// 部分阅读器会把它当成损坏的附件，进而丢掉整条内容。
func TestBuildBlogFeedOmitsCoverWhenAbsent(t *testing.T) {
	body, err := buildBlogFeed([]*service.BlogFeedEntry{{
		Title: "无图", Link: "https://x/blog/1", GUID: "https://x/blog/1",
	}}, "问道", "https://x")
	if err != nil {
		t.Fatalf("buildBlogFeed: %v", err)
	}
	s := string(body)
	if strings.Contains(s, "<enclosure") {
		t.Errorf("无封面的条目不该产出 enclosure\n%s", s)
	}
	if strings.Contains(s, "<media:content") {
		t.Errorf("无封面的条目不该产出 media:content\n%s", s)
	}
}

// A video cover must be declared as video, or readers render it in an image layout and show a
// broken frame.
//
// 视频封面必须声明为视频，否则阅读器会用图片版式渲染它，显示成一个碎掉的画框。
func TestBuildBlogFeedMarksVideoCover(t *testing.T) {
	body, err := buildBlogFeed([]*service.BlogFeedEntry{{
		Title: "视频封面", Link: "https://x/blog/1", GUID: "https://x/blog/1",
		CoverURL:  "https://x/api/share/file?id=8&share_token=t",
		CoverMIME: "video/mp4",
	}}, "问道", "https://x")
	if err != nil {
		t.Fatalf("buildBlogFeed: %v", err)
	}
	if !strings.Contains(string(body), `medium="video"`) {
		t.Errorf("视频封面没有声明 medium=video\n%s", body)
	}
}
