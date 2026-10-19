package service

import (
	"strings"
	"testing"
)

// TestPreprocessDynamicBlocks covers the two rules the file exists for: never present raw source
// as content, and never silently delete it.
//
// TestPreprocessDynamicBlocks 覆盖本文件存在的两条规则：绝不把原始源码当作内容展示，
// 也绝不静默删除。
func TestPreprocessDynamicBlocks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
		not  []string
	}{
		{
			"dataview becomes a card, not a code block",
			"```dataview\nTABLE file.mtime FROM #tag\n```",
			// The query text IS kept — behind the toggle, which is the point: the reader is not
			// shown a query as if it were content, but it is not destroyed either.
			// 查询文本确实被保留——在折叠里，这正是要点：读者不会看到查询被当作内容展示，
			// 但它也没有被销毁。
			[]string{`class="dynblock"`, `data-kind="dataview"`, "Dataview 查询", "查看原始定义", "TABLE file.mtime FROM"},
			nil,
		},
		{
			"dataviewjs",
			"```dataviewjs\ndv.list()\n```",
			[]string{`data-kind="dataviewjs"`, "DataviewJS 脚本"},
			nil,
		},
		{
			"base",
			"```base\nfilters:\n  and: []\n```",
			[]string{`data-kind="base"`, "Obsidian Base"},
			nil,
		},
		{
			"mermaid keeps its source behind a toggle",
			"```mermaid\ngraph TD\n  A-->B\n```",
			[]string{`data-kind="mermaid"`, "Mermaid 图表", "查看原始定义", "graph TD"},
			nil,
		},
		{
			"plantuml",
			"```plantuml\n@startuml\nA -> B\n@enduml\n```",
			[]string{`data-kind="plantuml"`, "PlantUML 图表"},
			nil,
		},
		{
			"canvas drops the source (not worth keeping)",
			"```canvas\n{\"nodes\":[]}\n```",
			[]string{`data-kind="canvas"`, "Canvas 画布"},
			[]string{"查看原始定义"},
		},
		{
			"language tag is case-insensitive",
			"```Mermaid\ngraph LR\n```",
			[]string{`data-kind="mermaid"`},
			nil,
		},
		{
			"tilde fence works",
			"~~~dataview\nLIST\n~~~",
			[]string{`data-kind="dataview"`},
			nil,
		},

		// Must be left alone.
		// 必须原样保留。
		{"a normal language is untouched", "```go\nfunc main() {}\n```", []string{"func main()"}, []string{"dynblock"}},
		{"no fence at all", "普通段落", []string{"普通段落"}, []string{"dynblock"}},
		{"inline mention is not a fence", "语法是 ```dataview 这样写", nil, []string{"dynblock"}},
		{"unterminated fence is left to the renderer", "```dataview\nLIST", []string{"\nLIST"}, []string{"dynblock"}},
	}

	for _, c := range cases {
		got := preprocessDynamicBlocks(c.in)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: output %q does not contain %q", c.name, got, w)
			}
		}
		for _, n := range c.not {
			if strings.Contains(got, n) {
				t.Errorf("%s: output %q must not contain %q", c.name, got, n)
			}
		}
	}
}

// TestDynamicBlockSourceIsEscaped: the source goes into an HTML text node, so it must not be able
// to close the <pre> or inject an element.
//
// TestDynamicBlockSourceIsEscaped：源码进入 HTML 文本节点，因此不能闭合 <pre> 或注入元素。
func TestDynamicBlockSourceIsEscaped(t *testing.T) {
	got := preprocessDynamicBlocks("```mermaid\ngraph TD\n  A[<script>alert(1)</script>]\n```")
	if strings.Contains(got, "<script>") {
		t.Errorf("source was not escaped: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected escaped source, got %q", got)
	}
}

// TestDynamicBlockDoesNotSwallowFollowingContent: the closing fence ends the block, so whatever
// follows must survive as ordinary document text.
//
// TestDynamicBlockDoesNotSwallowFollowingContent：结束围栏终止该块，因此其后的内容必须作为
// 普通文档文本保留下来。
func TestDynamicBlockDoesNotSwallowFollowingContent(t *testing.T) {
	src := "```dataview\nLIST\n```\n\n## 之后的标题\n\n之后的正文"
	got := preprocessDynamicBlocks(src)
	if !strings.Contains(got, "## 之后的标题") {
		t.Errorf("heading after the block was lost: %q", got)
	}
	if !strings.Contains(got, "之后的正文") {
		t.Errorf("paragraph after the block was lost: %q", got)
	}
	if strings.Contains(got, "LIST") && !strings.Contains(got, "查看原始定义") {
		t.Errorf("query text leaked without the toggle: %q", got)
	}
}

// TestDynamicBlockLongerClosingFence: CommonMark allows a closing fence longer than the opening
// one. Treating the exact string as the only terminator would leave the block unterminated.
//
// TestDynamicBlockLongerClosingFence：CommonMark 允许结束围栏比起始围栏更长。
// 只把完全相同的字符串当作终止符会让该块处于未闭合状态。
func TestDynamicBlockLongerClosingFence(t *testing.T) {
	got := preprocessDynamicBlocks("```dataview\nLIST\n`````\n\n之后")
	if !strings.Contains(got, `class="dynblock"`) {
		t.Errorf("longer closing fence was not recognised: %q", got)
	}
	if !strings.Contains(got, "之后") {
		t.Errorf("content after the block was lost: %q", got)
	}
}

// TestDynamicBlockEndToEnd runs the real pipeline: the card must survive goldmark and reach the
// reader as markup, not as escaped text.
//
// TestDynamicBlockEndToEnd 跑真实管线：卡片必须穿过 goldmark 并以标记形式到达读者，
// 而不是被转义成文本。
func TestDynamicBlockEndToEnd(t *testing.T) {
	md := newTestMarkdown()
	got := render(t, md, preprocessDynamicBlocks("## 标题\n\n```mermaid\ngraph TD\n  A-->B\n```\n\n之后"))

	for _, want := range []string{`class="dynblock"`, "Mermaid 图表", "<details", "graph TD", "<h2"} {
		if !strings.Contains(got, want) {
			t.Errorf("end-to-end output missing %q\ngot: %s", want, got)
		}
	}
	if strings.Contains(got, "&lt;div class=") {
		t.Errorf("card markup was escaped instead of rendered: %s", got)
	}
}

// TestDynamicBlockSourceIsNotRenderedAsTheContent is the rule stated as an assertion: the query
// text must appear only inside the collapsed <details>, never as the block's visible body.
//
// TestDynamicBlockSourceIsNotRenderedAsTheContent 把规则写成断言：查询文本只能出现在折叠的
// <details> 之内，绝不能成为该块可见的正文。
func TestDynamicBlockSourceIsNotRenderedAsTheContent(t *testing.T) {
	src := "```dataview\nTABLE file.mtime FROM #tag\n```"
	got := preprocessDynamicBlocks(src)

	idxDetails := strings.Index(got, "<details")
	idxQuery := strings.Index(got, "TABLE file.mtime")
	if idxDetails < 0 {
		t.Fatalf("source toggle missing: %q", got)
	}
	if idxQuery < idxDetails {
		t.Errorf("query text appears before the toggle, i.e. as visible content: %q", got)
	}
	// A plain fenced code block would render the query as body text; a card must not.
	// 普通围栏代码块会把查询渲染为正文；卡片绝不能这样。
	if strings.HasPrefix(strings.TrimSpace(got), "TABLE") {
		t.Errorf("block still begins with the query text: %q", got)
	}
}
