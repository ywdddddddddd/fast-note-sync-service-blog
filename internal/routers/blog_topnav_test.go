package routers

import (
	"bytes"
	"html/template"
	"reflect"
	"strings"
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// The top bar's page-note entries had a definition and a full set of styles from the beginning,
// but no page ever included them: the shell existed and nothing rendered it. The data behind it
// was dropped a layer below, too — GetBlogHome replaced the whole Nav struct after filling Pages.
// Three separate links in one chain, each failing silently on its own.
//
// These assertions hold the chain together. They are cheap, and they fail the moment someone
// reorders the template concatenation, drops the field, or renames the anchor.
//
// 顶栏的页面笔记入口从一开始就有定义和完整样式，却没有任何页面 include 它：外壳存在，无人渲染。
// 它背后的数据在更下一层也被丢掉了——GetBlogHome 在填好 Pages 之后又把整个 Nav 结构体替换了。
// 一条链上三处环节，每一处都能独自安静地失效。
//
// 下面几条断言把这条链固定住。代价很小，而谁打乱模板拼接顺序、删掉字段或改掉锚点名，它就会红。
func TestTopNavIsWiredOnEveryPage(t *testing.T) {
	cases := []struct {
		page string
		tmpl *template.Template
		view any
	}{
		{"home", blogHomeTmpl, blogHomeView{}},
		{"post", blogPostTmpl, blogPostView{}},
		{"page", blogPageTmpl, blogPageView{}},
	}

	for _, tc := range cases {
		// The definition has to live in this page's own template set. A {{template "topnav"}}
		// call against a set that was built without blogTopNav parses fine and only fails when
		// the page is executed — which in production means an error page instead of the site.
		//
		// 定义必须在这个页面自己的模板集合里。一个没有拼进 blogTopNav 的集合里调用
		// {{template "topnav"}}，解析不会报错，只在执行时才失败——在线上这意味着错误页取代站点。
		if tc.tmpl.Lookup("topnav") == nil {
			t.Fatalf("%s: template set has no \"topnav\" definition", tc.page)
		}

		// And the page body has to actually call it.
		// 而且页面正文必须真的调用它。
		if body := tc.tmpl.Tree.Root.String(); !strings.Contains(body, `{{template "topnav" .}}`) {
			t.Fatalf("%s: page body does not include topnav", tc.page)
		}

		// Finally the view must carry the entries, or the include renders nothing at all. HasShelf
		// is checked the same way: topnav reads it in the same conditional, so a view missing it
		// fails at execute time rather than at compile time.
		//
		// 最后，view 必须带着入口数据，否则这个 include 什么也不渲染。HasShelf 同样要查：
		// topnav 在同一个条件里读它，所以少了这个字段的 view 会在执行时失败，而不是编译时。
		field, ok := reflect.TypeOf(tc.view).FieldByName("Pages")
		if !ok {
			t.Fatalf("%s: view has no Pages field, so topnav could never render", tc.page)
		}
		if want := reflect.TypeOf([]dto.BlogNavPageDTO{}); field.Type != want {
			t.Fatalf("%s: Pages is %s, want %s", tc.page, field.Type, want)
		}
		shelfField, ok := reflect.TypeOf(tc.view).FieldByName("HasShelf")
		if !ok {
			t.Fatalf("%s: view has no HasShelf field, so topnav would fail to render", tc.page)
		}
		if shelfField.Type.Kind() != reflect.Bool {
			t.Fatalf("%s: HasShelf is %s, want bool", tc.page, shelfField.Type)
		}
	}
}

// The warp transition's departing half binds to #fns-warp. That id used to live on the "about me"
// card in the right-hand column; the card is gone and the anchor moved into the top bar. If the
// anchor and the script ever disagree, the transition silently stops playing — nothing throws.
//
// 穿越过渡的「出发」那一半绑定 #fns-warp。这个 id 原本在右栏「关于我」那张卡片上；
// 卡片已撤，锚点迁进顶栏。锚点与脚本一旦对不上，过渡会安静地不再播放——没有任何报错。
func TestWarpAnchorLivesInTopNav(t *testing.T) {
	if !strings.Contains(blogTopNav, `id="fns-warp"`) {
		t.Fatalf("topnav does not carry id=\"fns-warp\"; the warp departure would never bind")
	}
	if !strings.Contains(blogWarpJS(), "getElementById('fns-warp')") {
		t.Fatalf("warp script no longer looks up fns-warp; the anchor above is dead")
	}
	// The anchor is attached to the page-note entry whose alias is "about", not to every entry.
	// 锚点挂在别名为 "about" 的那一条上，而不是每一条上。
	if !strings.Contains(blogTopNav, `eq .Alias "about"`) {
		t.Fatalf("topnav does not gate the warp anchor on the about alias")
	}
}

// A real render, not just an AST check. The page-note template is the one of the three that can
// be executed with an empty view — it has no stats pointer to dereference — so it supplies the
// end-to-end evidence that the include, the data and the escaping work together.
//
// 真实渲染，而不只是 AST 检查。页面笔记模板是三者中唯一能用空 view 执行的
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
		`<a href="/blog/about" id="fns-warp">关于我</a>`,
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
