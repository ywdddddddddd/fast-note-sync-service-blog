package routers

import (
	"bytes"
	"html/template"
	"reflect"
	"strings"
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// The card page's classes are selected by stylesheet rules, and the two live in different files.
// Rename a class here and the stylesheet silently matches nothing — no error, no warning, just a
// page that looks wrong in a way the diff cannot show.
//
// These assertions are that joint: they hold the hooks the new card-page rules select on, and they
// hold the values that would otherwise come from data the view does not carry.
//
// 名片页的类名被样式表规则选中，而两者在不同的文件里。在这里改掉一个类名，样式表就继续匹配
// 一个不存在的东西——没有报错、没有警告，只有一张以 diff 看不出来的方式变错的页面。
//
// 下面几条断言就是那个接缝：它们钉住新的名片页规则所选择的钩子，
// 也钉住那些本应来自数据、而 view 并不携带的取值。
func TestCardPageCarriesItsHooks(t *testing.T) {
	var buf bytes.Buffer
	err := blogCardTmpl.Execute(&buf, blogCardView{
		blogChrome: blogChrome{
			Title:    "问道漫步",
			SiteName: "问道漫步",
			Sections: []*dto.BlogSectionDTO{{Name: "口腔笔记", Slug: "口腔笔记", Count: 40}},
		},
		Stats: &dto.BlogStatsDTO{
			Notes: 152, Words: 858121, WordsText: "85.8万",
			Tags: 0, Links: 370, DaysOnline: 53, Since: "2026-09-10", Updated: "2026-11-02",
		},
		Profile: &dto.BlogProfileDTO{
			Name: "爻问道 （wen）", Tagline: "口腔医学 · 本科在读",
			Location: "浙江 · 金华",
			Meta:     []dto.BlogProfileMetaDTO{{Label: "性别", Value: "男"}},
			Links:    []dto.BlogProfileLinkDTO{{Label: "GitHub", Href: "https://github.com/x", Kind: "link", Icon: "mark-github"}},
		},
		Pills:    []dto.BlogProfileLinkDTO{{Label: "浙江 · 金华", Kind: "text", Icon: "location"}},
		Activity: &dto.BlogActivityDTO{Weeks: 14, Days: []dto.BlogDayDTO{{Date: "2026-11-01", Lvl: 2}}},
		DockJS:   "void 0;",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	// The identity block and its pills.
	// 身份块与药丸。
	for _, want := range []string{
		`class="hero"`, `class="hero-avatar"`, `class="hero-name"`, `class="hero-tag"`,
		`class="pills"`, `class="pill"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("card page lacks %q", want)
		}
	}

	// The "关于" rows, the three statistic cards and the heat map are the only content blocks.
	// 「关于」键值行、三张统计卡与热力图是仅有的内容区块。
	for _, want := range []string{
		`class="kv"`, `class="kv-row"`, `class="stats-row"`, `class="stat-card"`,
		`act-card`, `id="activity"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("card page lacks %q", want)
		}
	}

	// The compact word count, not the raw six digits: 858121 does not fit a stat card.
	// 压缩字数而不是原始六位数：858121 塞不进一张统计卡。
	if !strings.Contains(out, "85.8万") {
		t.Fatalf("stat card did not use WordsText")
	}

	// The mobile capsule and its two sheets must exist even though the page has no rail.
	// 移动胶囊与它的两块面板必须存在，即便这一页没有右栏。
	for _, want := range []string{`id="fns-dock"`, `class="card-sheet sheet-nav"`, `id="fns-scrim"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("card page lacks %q", want)
		}
	}

	// And the article list must NOT be here: the card page is a card, not a feed.
	// 而文章列表必须不在这里：名片页是名片，不是信息流。
	if strings.Contains(out, `class="post-card"`) {
		t.Fatalf("card page rendered article cards; the list belongs to /blog/all")
	}
}

// Every page either renders the shared chrome or the legacy topnav, and the include has to be in
// its own template set. A {{template}} call against a set built without the definition parses fine
// and only fails when the page executes — which in production is an error page instead of a site.
//
// 每个页面要么渲染共用外壳，要么渲染旧顶栏，而 include 必须在该页自己的模板集合里。
// 一个没有拼进定义的集合里调用 {{template}}，解析不会报错，只在执行时才失败——
// 在线上这意味着错误页取代站点。
func TestChromeIsWiredOnEveryPage(t *testing.T) {
	cases := []struct {
		page    string
		tmpl    *template.Template
		view    any
		def     string
		include string
	}{
		{"card", blogCardTmpl, blogCardView{}, "blogtopbar", `{{template "blogtopbar" .}}`},
		{"list", blogListTmpl, blogListView{}, "blogtopbar", `{{template "blogtopbar" .}}`},
		{"archives", blogArchivesTmpl, blogArchiveView{}, "blogtopbar", `{{template "blogtopbar" .}}`},
		{"post", blogPostTmpl, blogPostView{}, "topnav", `{{template "topnav" .}}`},
		{"page", blogPageTmpl, blogPageView{}, "topnav", `{{template "topnav" .}}`},
	}

	for _, tc := range cases {
		if tc.tmpl.Lookup(tc.def) == nil {
			t.Fatalf("%s: template set has no %q definition", tc.page, tc.def)
		}
		if body := tc.tmpl.Tree.Root.String(); !strings.Contains(body, tc.include) {
			t.Fatalf("%s: page body does not include %s", tc.page, tc.def)
		}
		// The view must carry the entries, or the include renders nothing at all.
		// view 必须带着入口数据，否则这个 include 什么也不渲染。
		field, ok := reflect.TypeOf(tc.view).FieldByName("Pages")
		if !ok {
			t.Fatalf("%s: view has no Pages field, so the chrome could never render", tc.page)
		}
		if want := reflect.TypeOf([]dto.BlogNavPageDTO{}); field.Type != want {
			t.Fatalf("%s: Pages is %s, want %s", tc.page, field.Type, want)
		}
		shelfField, ok := reflect.TypeOf(tc.view).FieldByName("HasShelf")
		if !ok {
			t.Fatalf("%s: view has no HasShelf field", tc.page)
		}
		if shelfField.Type.Kind() != reflect.Bool {
			t.Fatalf("%s: HasShelf is %s, want bool", tc.page, shelfField.Type)
		}
	}
}

// A real render, not just an AST check. The page-note template is the one that can be executed
// with an empty view — it has no stats pointer to dereference — so it supplies the end-to-end
// evidence that the include, the data and the escaping work together.
//
// 真实渲染，而不只是 AST 检查。页面笔记模板是唯一能用空 view 执行的
// （它没有需要解引用的统计指针），因此由它提供端到端证据：include、数据与转义确实合得上。
func TestPageTopNavRendersAboutEntry(t *testing.T) {
	var buf bytes.Buffer
	err := blogPageTmpl.Execute(&buf, blogPageView{
		SiteName: "test",
		Pages: []dto.BlogNavPageDTO{
			{Alias: "about", Title: "关于我", URL: "/blog/about"},
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`<nav class="topnav">`,
		`<a href="/blog/about">关于我</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered page lacks %q", want)
		}
	}

	// No entries means no <nav> element at all. An empty one would still take part in the flex
	// layout and push the breadcrumb off its baseline — the reason the fragment is guarded.
	//
	// 没有入口时连 <nav> 元素都不输出。一个空的仍会参与弹性布局，把面包屑挤离基线——
	// 这正是该片段带条件的原因。
	var empty bytes.Buffer
	if err := blogPageTmpl.Execute(&empty, blogPageView{SiteName: "test"}); err != nil {
		t.Fatalf("render empty: %v", err)
	}
	if strings.Contains(empty.String(), `class="topnav"`) {
		t.Fatalf("empty Pages still rendered a topnav shell")
	}

	// The shelf entry appears on its own, without any page note being published.
	// 书架入口可以单独出现，不依赖任何已发布的页面笔记。
	var shelfOnly bytes.Buffer
	if err := blogPageTmpl.Execute(&shelfOnly, blogPageView{SiteName: "test", HasShelf: true}); err != nil {
		t.Fatalf("render shelf: %v", err)
	}
	if !strings.Contains(shelfOnly.String(), `<a href="/blog/shelf">资料</a>`) {
		t.Fatalf("HasShelf produced no 资料 entry")
	}
}
