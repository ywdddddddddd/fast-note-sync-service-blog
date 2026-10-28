package service

import (
	"strings"
	"testing"
)

// absolutizeBlogHTML is the difference between a readable feed and seventeen broken images.
//
// Measured on the live site before this existed: an article body carried
// `<img src="/api/share/file?id=1968&share_token=AAAA...">`. In a reader that resolves against
// the reader's own origin, so every image in every item was a 404.
//
// absolutizeBlogHTML 是「可读的订阅源」与「十七张裂图」之间的差别。
//
// 上线前实测：一篇正文里写着 `<img src="/api/share/file?id=1968&share_token=AAAA...">`。
// 在阅读器里它按阅读器自己的域名解析，于是每一条里的每一张图都是 404。
func TestAbsolutizeBlogHTML(t *testing.T) {
	const base = "https://yaowendao.asia"

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "root-relative image",
			in:   `<img src="/api/share/file?id=1&share_token=x">`,
			want: `<img src="https://yaowendao.asia/api/share/file?id=1&share_token=x">`,
		},
		{
			name: "root-relative link",
			in:   `<a href="/blog/1.144-40b3e389f1">下一步</a>`,
			want: `<a href="https://yaowendao.asia/blog/1.144-40b3e389f1">下一步</a>`,
		},
		{
			// A protocol-relative URL is already absolute; prefixing it would produce
			// "https://yaowendao.asia//cdn.example.com/x", which 404s.
			// 协议相对地址本身已是绝对的；给它加前缀会得到
			// "https://yaowendao.asia//cdn.example.com/x"，那是 404。
			name: "protocol-relative is left alone",
			in:   `<img src="//cdn.example.com/x.png">`,
			want: `<img src="//cdn.example.com/x.png">`,
		},
		{
			name: "already absolute is left alone",
			in:   `<a href="https://elsewhere.test/x">外链</a>`,
			want: `<a href="https://elsewhere.test/x">外链</a>`,
		},
		{
			// Bare root: "<a href="/">首页</a>" must become "<a href="https://host/">首页</a>",
			// not lose its closing quote.
			// 裸根路径：`<a href="/">首页</a>` 必须变成 `<a href="https://host/">首页</a>`，
			// 而不是丢掉收尾引号。
			name: "bare root keeps its quote",
			in:   `<a href="/">首页</a>`,
			want: `<a href="https://yaowendao.asia/">首页</a>`,
		},
		{
			name: "uppercase attribute",
			in:   `<IMG SRC="/a.png">`,
			want: `<IMG SRC="https://yaowendao.asia/a.png">`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := absolutizeBlogHTML(c.in, base); got != c.want {
				t.Errorf("absolutizeBlogHTML(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
		})
	}
}

// A base with a trailing slash must not produce a double slash.
// 带尾斜杠的 base 不得产生双斜杠。
func TestAbsolutizeBlogHTMLTrailingSlashBase(t *testing.T) {
	got := absolutizeBlogHTML(`<img src="/a.png">`, "https://yaowendao.asia/")
	want := `<img src="https://yaowendao.asia/a.png">`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// An empty base means "we do not know our own address", and returning the input unchanged is the
// only honest answer: inventing a prefix would build links to a host that may not exist.
//
// 空 base 意味着「不知道自己的地址」，原样返回是唯一诚实的选择：
// 凭空编一个前缀会造出指向可能不存在的主机的链接。
func TestAbsolutizeBlogHTMLEmptyBase(t *testing.T) {
	const in = `<img src="/a.png">`
	if got := absolutizeBlogHTML(in, ""); got != in {
		t.Errorf("got %q, want unchanged %q", got, in)
	}
}

func TestBlogPlainTextParagraphsKeepsBreaks(t *testing.T) {
	const md = "第一段的内容在这里，讲了一些东西。\n\n第二段讲另一些东西，也很重要。\n\n第三段是结尾。"

	got := blogPlainTextParagraphs(md, 180)

	if !strings.Contains(got, "\n\n") {
		t.Fatalf("段落分隔丢失，得到 %q", got)
	}
	if strings.Contains(got, "第一段的内容在这里，讲了一些东西。 第二段") {
		t.Fatalf("段落被合并成一行，得到 %q", got)
	}
	if !strings.HasPrefix(got, "第一段") {
		t.Errorf("开头不是第一段：%q", got)
	}
}

func TestBlogPlainTextParagraphsRespectsLimit(t *testing.T) {
	long := strings.Repeat("字", 500)
	got := blogPlainTextParagraphs(long, 180)
	r := []rune(got)
	// The ellipsis is one rune beyond the limit; anything more means the budget leaked.
	// 省略号是超出行数上限的一个字符；再多就说明预算漏水了。
	if len(r) > 181 {
		t.Errorf("长度 %d 超过上限 180（+1 省略号）：%q", len(r), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("被截断却少了省略号：%q", got)
	}
}

// The wiki-path leak: [[path/to/note|别名]] used to render as "path/to/note别名", which put the
// raw vault path into every excerpt that started with a link.
//
// wiki 路径泄漏：[[path/to/note|别名]] 过去渲染成「path/to/note别名」，
// 使每一个以链接开头的摘要都带着原始仓库路径。
func TestBlogWikiDisplayText(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`[[（1）口腔笔记/口组笔记/3.2牙本质（Dentin）|牙本质]]`, "牙本质"},
		{`[[（1）口腔笔记/口组笔记/3.2牙本质（Dentin）]]`, "3.2牙本质（Dentin）"},
		{`[[简单笔记]]`, "简单笔记"},
		{`[[目录/笔记#小节|章节]]`, "章节"},
		{`[[目录/笔记#小节]]`, "笔记"},
		// An alias that is only whitespace is no alias at all: the path's last segment wins.
		// 只有空白的别名不算别名：取路径最后一段。
		{`[[目录/笔记|   ]]`, "笔记"},
		// A malformed target — nothing before the pipe — is left untouched rather than
		// silently emptied, because rewriting it would hide the author's typo.
		// target 畸形（管道符前为空）时原样返回而不是悄悄清空，因为改写会掩盖作者的笔误。
		{`[[|空别名]]`, `[[|空别名]]`},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := blogWikiDisplayText(c.in); got != c.want {
				t.Errorf("blogWikiDisplayText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// The home page excerpt and the feed excerpt must agree about link text: they read the same notes
// and a reader who opens both would otherwise see two different renderings of the same sentence.
//
// 首页摘要与订阅摘要必须对链接文字达成一致：它们读的是同一批笔记，
// 同时打开两处的读者否则会看到同一个句子的两种渲染。
func TestBlogPlainTextUsesWikiFix(t *testing.T) {
	got := blogPlainText("[[（1）口腔笔记/口组笔记/3.2牙本质（Dentin）|牙本质]]的开头", 90)
	if strings.Contains(got, "口腔笔记/口组笔记") {
		t.Errorf("摘要里仍然漏出仓库路径：%q", got)
	}
	if !strings.HasPrefix(got, "牙本质") {
		t.Errorf("别名没有被采用：%q", got)
	}
}

// firstBlogImageSrc must return the first real image, skip inline data URIs, and stop at the tag
// boundary rather than scanning on to a later element's src.
//
// The tag-boundary case is the one that breaks with a lazy `.*?`: this note renders as a single
// long line, so the match would start at an <a> and end at whatever came next.
//
// firstBlogImageSrc 必须返回第一张真实图片、跳过内联 data URI，并在标签边界停下，
// 而不是继续扫到后面某个元素的 src。
//
// 标签边界这一条正是惰性 `.*?` 会出错的地方：这篇笔记渲染成一整行，
// 匹配会从一个 <a> 开始、止于后面任意一个元素。
func TestFirstBlogImageSrc(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "取第一张",
			in:   `<p>文</p><img src="/api/share/file?id=1"><img src="/api/share/file?id=2">`,
			want: "/api/share/file?id=1",
		},
		{
			name: "跳过 data URI",
			in:   `<img src="data:image/png;base64,AAA"><img src="/api/share/file?id=3">`,
			want: "/api/share/file?id=3",
		},
		{
			name: "不跨标签",
			in:   `<a href="/x"><img src="/a.png"></a>`,
			want: "/a.png",
		},
		{
			name: "没有图片",
			in:   `<p>只有文字</p>`,
			want: "",
		},
		{
			name: "属性顺序颠倒也认",
			in:   `<img alt="图" src="/b.png" width="10">`,
			want: "/b.png",
		},
		{
			name: "只有 data URI",
			in:   `<img src="data:image/gif;base64,BBB">`,
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstBlogImageSrc(c.in); got != c.want {
				t.Errorf("firstBlogImageSrc(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// absolutizeBlogURL prefixes root-relative addresses and leaves everything else alone.
//
// Protocol-relative is the case worth naming: "//host/x" already names a host, and prefixing it
// produces "https://blog//host/x", which resolves to the blog's own origin and 404s.
//
// absolutizeBlogURL 给根相对地址加前缀，其余一律原样返回。
//
// 协议相对是值得单独点出的那一例："//host/x" 本身已经指明了主机，加前缀会得到
// "https://blog//host/x"，它会被解析成博客自己的域名然后 404。
func TestAbsolutizeBlogURL(t *testing.T) {
	const base = "https://yaowendao.asia"
	cases := []struct{ in, want string }{
		{"/api/share/file?id=1", base + "/api/share/file?id=1"},
		{"//cdn.example.com/x.png", "//cdn.example.com/x.png"},
		{"https://other/x.png", "https://other/x.png"},
		{"", ""},
		{"/", base + "/"},
	}
	for _, c := range cases {
		if got := absolutizeBlogURL(c.in, base); got != c.want {
			t.Errorf("absolutizeBlogURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := absolutizeBlogURL("/x", ""); got != "/x" {
		t.Errorf("空 base 应原样返回，得到 %q", got)
	}
	if got := absolutizeBlogURL("/x", base+"/"); got != base+"/x" {
		t.Errorf("base 尾斜杠应被吸收，得到 %q", got)
	}
}

// The declared cover must win over the body's first image: that is the whole point of the
// frontmatter key, and the reason the editor exists.
//
// 显式声明的封面必须优先于正文第一张图：这正是 frontmatter 键存在的全部意义，
// 也是那个编辑器的存在理由。
func TestFeedCoverPrefersDeclaredOverBody(t *testing.T) {
	entry := &BlogFeedEntry{
		ContentHTML: `<img src="https://x/body.png">`,
	}
	feedCoverFor(entry, &blogCover{
		URL: "/api/share/file?id=9", Kind: "image", MIME: "image/webp",
	}, "https://x")

	if entry.CoverURL != "https://x/api/share/file?id=9" {
		t.Errorf("没有采用声明的封面: %q", entry.CoverURL)
	}
	if entry.CoverMIME != "image/webp" {
		t.Errorf("MIME 没有跟着声明走: %q", entry.CoverMIME)
	}
}

// With no declaration the body's first image is the cover, and it is already absolute because
// ContentHTML was rewritten before this runs.
//
// 没有声明时正文第一张图就是封面，而它已经是绝对地址，因为 ContentHTML 在此之前已改写。
func TestFeedCoverFallsBackToBodyImage(t *testing.T) {
	entry := &BlogFeedEntry{
		ContentHTML: `<p>x</p><img src="https://x/body.png">`,
	}
	feedCoverFor(entry, nil, "https://x")

	if entry.CoverURL != "https://x/body.png" {
		t.Errorf("没有退化到正文图片: %q", entry.CoverURL)
	}
	if entry.CoverMIME != blogFeedFallbackCoverMIME {
		t.Errorf("兜底 MIME = %q, want %q", entry.CoverMIME, blogFeedFallbackCoverMIME)
	}
}

// A cover whose file went missing resolves to an empty URL, so the entry must fall through to the
// body image rather than emitting an enclosure pointing at nothing.
//
// 封面文件丢失时解析出的 URL 为空，因此条目必须继续退化到正文图片，
// 而不是发出一个指向空无的附件。
func TestFeedCoverIgnoresUnresolvedDeclaration(t *testing.T) {
	entry := &BlogFeedEntry{
		ContentHTML: `<img src="https://x/body.png">`,
	}
	feedCoverFor(entry, &blogCover{URL: "", Kind: "image"}, "https://x")

	if entry.CoverURL != "https://x/body.png" {
		t.Errorf("未解析的封面挡住了正文图片: %q", entry.CoverURL)
	}
}
