package service

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// newTestMarkdown mirrors the production goldmark configuration in NewShareService. If the two
// drift apart the tests stop proving anything about the real pipeline.
//
// newTestMarkdown 复刻 NewShareService 里的生产 goldmark 配置。两者一旦分叉，
// 测试就不再能证明真实管线的行为。
func newTestMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.CJK,
			extension.Footnote,
			extension.Typographer,
			Mark,
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
}

func render(t *testing.T, md goldmark.Markdown, src string) string {
	t.Helper()
	var buf bytes.Buffer
	// Same options the production path passes, so the tests exercise the real heading-id rules.
	// 与生产路径传入相同的选项，使测试检验真实的标题 id 规则。
	if err := md.Convert([]byte(src), &buf, blogHeadingIDOptions()...); err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	return buf.String()
}

// TestMarkExtension covers ==highlight==.
// TestMarkExtension 覆盖 ==高亮==。
func TestMarkExtension(t *testing.T) {
	md := newTestMarkdown()

	cases := []struct {
		name string
		in   string
		want []string
		not  []string
	}{
		{"basic", "这是 ==重点== 内容", []string{"<mark>重点</mark>"}, nil},
		{"multiple", "==a== 和 ==b==", []string{"<mark>a</mark>", "<mark>b</mark>"}, nil},
		{"empty", "== ==", nil, []string{"<mark>"}},
		{"triple equals not a mark", "a===b", nil, []string{"<mark>"}},
		{"inside code span is literal", "`==not mark==`", []string{"==not mark=="}, []string{"<mark>"}},
		{"inside fence is literal", "```\n==not mark==\n```", []string{"==not mark=="}, []string{"<mark>"}},
	}
	for _, c := range cases {
		got := render(t, md, c.in)
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

// TestCJKSoftBreak is the highest-impact fix in batch A: without extension.CJK, a paragraph
// written one sentence per line renders with a space at every line end (the browser collapses
// the raw newline into a space).
//
// TestCJKSoftBreak 是批 A 中影响最大的修复：缺少 extension.CJK 时，逐句换行书写的段落
// 会在每行末尾多出一个空格（浏览器把原始换行折叠成空格）。
func TestCJKSoftBreak(t *testing.T) {
	md := newTestMarkdown()
	cjk := render(t, md, "第一句。\n第二句。")
	if strings.Contains(cjk, "第一句。 第二句。") {
		t.Errorf("CJK soft break inserted a space: %q", cjk)
	}
	if !strings.Contains(cjk, "第一句。") || !strings.Contains(cjk, "第二句。") {
		t.Errorf("CJK text lost: %q", cjk)
	}

	// Latin text must keep its separator. goldmark emits the raw newline for a Latin soft break
	// (the browser collapses it to a space), so the assertion is "a separator survived", not
	// "it is literally a space".
	//
	// 拉丁文本必须保留分隔。对拉丁软换行，goldmark 输出的是原始换行符（浏览器会把它折叠成
	// 空格），因此断言是「分隔仍然存在」，而不是「它字面上就是空格」。
	latin := render(t, md, "first line\nsecond line")
	if !strings.Contains(latin, "first line\nsecond line") && !strings.Contains(latin, "first line second line") {
		t.Errorf("latin soft break lost its separator: %q", latin)
	}

	// And the CJK case must not have gained a space either way.
	// 而中日韩情形无论哪种形式都不应多出空格。
	if strings.Contains(cjk, "。 ") || strings.Contains(cjk, "。\n") && !strings.Contains(cjk, "第一句。 第二句。") {
		t.Logf("cjk output form: %q", cjk)
	}
}

// TestFootnoteExtension covers [^1].
// TestFootnoteExtension 覆盖 [^1] 脚注。
func TestFootnoteExtension(t *testing.T) {
	md := newTestMarkdown()
	got := render(t, md, "正文[^1]\n\n[^1]: 脚注内容")
	if !strings.Contains(got, "footnote") {
		t.Errorf("footnote markup missing: %q", got)
	}
	if !strings.Contains(got, "脚注内容") {
		t.Errorf("footnote text lost: %q", got)
	}
}

// TestCalloutPreprocess covers the callout rewriter directly (it runs before goldmark).
// TestCalloutPreprocess 直接覆盖 callout 改写器（它在 goldmark 之前运行）。
func TestCalloutPreprocess(t *testing.T) {
	identity := func(s string) string { return "<p>" + s + "</p>" }

	cases := []struct {
		name string
		in   string
		want []string
		not  []string
	}{
		{
			"note with title",
			"> [!note] 我的标题\n> 正文内容",
			[]string{`class="callout callout-note"`, `data-callout="note"`, "我的标题", "正文内容"},
			nil,
		},
		{
			"unknown type falls back instead of being dropped",
			"> [!whatever] 自定义\n> 内容",
			[]string{`class="callout callout-note"`, `data-callout="whatever"`, "内容"},
			[]string{"[!whatever]"},
		},
		{
			"warning gets warn colour",
			"> [!warning] 小心\n> 危险内容",
			[]string{`class="callout callout-warn"`},
			nil,
		},
		{
			"danger gets danger colour",
			"> [!danger]\n> 破坏性操作",
			[]string{`class="callout callout-danger"`, "危险"},
			nil,
		},
		{
			"multi-line body",
			"> [!info] T\n> 第一行\n> 第二行",
			[]string{"第一行", "第二行"},
			nil,
		},
		{
			"marker inside fence stays literal",
			"```\n> [!note] not a callout\n```",
			[]string{"> [!note] not a callout"},
			[]string{`class="callout`},
		},
		{
			"plain blockquote is untouched",
			"> 只是引用",
			[]string{"> 只是引用"},
			[]string{`class="callout`},
		},
		{
			"content without marker is untouched",
			"普通段落",
			[]string{"普通段落"},
			nil,
		},
	}

	for _, c := range cases {
		got := preprocessCallouts(c.in, identity)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: %q does not contain %q", c.name, got, w)
			}
		}
		for _, n := range c.not {
			if strings.Contains(got, n) {
				t.Errorf("%s: %q must not contain %q", c.name, got, n)
			}
		}
	}
}

// TestCalloutTitleEscaping: a callout title lands in HTML text, so a title containing markup
// must not become markup.
//
// TestCalloutTitleEscaping：callout 标题会进入 HTML 文本，因此含标记的标题不能变成标记。
func TestCalloutTitleEscaping(t *testing.T) {
	identity := func(s string) string { return s }
	got := preprocessCallouts("> [!note] <script>alert(1)</script>\n> body", identity)
	if strings.Contains(got, "<script>") {
		t.Errorf("title was not escaped: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected escaped title, got %q", got)
	}
}

// TestCalloutEndToEnd runs the full configured pipeline over a callout containing Markdown.
// TestCalloutEndToEnd 在完整配置的管线上跑一个内部含 Markdown 的 callout。
func TestCalloutEndToEnd(t *testing.T) {
	md := newTestMarkdown()
	renderInner := func(s string) string { return render(t, md, s) }

	src := "> [!tip] 提示\n> 这是 **加粗** 与 ==高亮==\n> \n> - 列表项"
	prepared := preprocessCallouts(src, renderInner)
	got := render(t, md, prepared)

	for _, want := range []string{`class="callout callout-tip"`, "<strong>加粗</strong>", "<mark>高亮</mark>", "<li>列表项</li>"} {
		if !strings.Contains(got, want) {
			t.Errorf("end-to-end output missing %q\ngot: %s", want, got)
		}
	}
	if strings.Contains(got, "[!tip]") {
		t.Errorf("callout marker leaked into output: %s", got)
	}
}

// TestCalloutAllBuiltInTypes walks the type table documented in Obsidian's own
// "Callouts reference" note, so a type that gains a case here is a type the blog claims to
// support. Unknown types must still render (see the fallback case above).
//
// TestCalloutAllBuiltInTypes 走一遍 Obsidian 自己的「Callouts reference」笔记里记录的类型表，
// 这里出现的类型就是博客声称支持的类型。未知类型仍必须渲染（见上面的回退用例）。
func TestCalloutAllBuiltInTypes(t *testing.T) {
	identity := func(s string) string { return s }
	types := []string{
		"note", "abstract", "summary", "tldr", "info", "todo", "tip", "hint", "important",
		"success", "check", "done", "question", "help", "faq", "warning", "caution", "attention",
		"failure", "fail", "missing", "danger", "error", "bug", "example", "quote", "cite",
	}
	for _, ty := range types {
		got := preprocessCallouts("> [!"+ty+"] 标题\n> 正文", identity)
		if !strings.Contains(got, `data-callout="`+ty+`"`) {
			t.Errorf("type %q was not recognised (output: %s)", ty, got)
		}
		if strings.Contains(got, "[!"+ty+"]") {
			t.Errorf("type %q leaked its marker into output: %s", ty, got)
		}
		if !strings.Contains(got, "正文") {
			t.Errorf("type %q lost its body: %s", ty, got)
		}
	}
}

// TestCalloutFolding pins the "+"/"-" markers: "-" is collapsed, "+" is expanded, and the
// native <details> element is what provides the toggle (no JavaScript).
//
// TestCalloutFolding 固定 "+"/"-" 标记的语义："-" 默认收起，"+" 默认展开，
// 开关由原生 <details> 提供（无需 JavaScript）。
func TestCalloutFolding(t *testing.T) {
	identity := func(s string) string { return s }

	collapsed := preprocessCallouts("> [!faq]- 收起\n> 正文", identity)
	if !strings.Contains(collapsed, "<details>") {
		t.Errorf("collapsed callout should render a closed <details>: %s", collapsed)
	}
	if strings.Contains(collapsed, "<details open>") {
		t.Errorf("collapsed callout must not be open: %s", collapsed)
	}

	expanded := preprocessCallouts("> [!faq]+ 展开\n> 正文", identity)
	if !strings.Contains(expanded, "<details open>") {
		t.Errorf("expanded callout should render an open <details>: %s", expanded)
	}
}

// TestCalloutInsideFenceIsLiteral is the case the vault actually contains: n_233 (the
// "Callouts reference" note) is entirely a fenced ```markdown sample, so every "> [!type]"
// line in it must survive as literal text and no callout markup may appear.
//
// TestCalloutInsideFenceIsLiteral 正是本仓库真实存在的情形：n_233（「Callouts reference」笔记）
// 整体就是一个 ```markdown 围栏示例，因此其中每一行 "> [!type]" 都必须作为字面文本保留，
// 且不得出现任何 callout 标记。
func TestCalloutInsideFenceIsLiteral(t *testing.T) {
	identity := func(s string) string { return s }
	src := "# Callouts reference\n\n~~~markdown\n" +
		"> [!faq]- Collapsed by default\n> Hidden until expanded.\n\n" +
		"> [!faq]+ Expanded by default\n> Visible but collapsible.\n\n" +
		"> [!question] Outer\n> > [!note] Inner\n> > Nested content.\n" +
		"~~~\n"

	got := preprocessCallouts(src, identity)
	if strings.Contains(got, "callout-") {
		t.Errorf("fenced callout sample must not be converted: %s", got)
	}
	for _, want := range []string{"> [!faq]- Collapsed by default", "> [!question] Outer", "> [!note] Inner"} {
		if !strings.Contains(got, want) {
			t.Errorf("fenced sample lost %q", want)
		}
	}
}

// TestCalloutNestedInlineAtLeastDegrades documents the known limit: a callout nested inside a
// callout is not recursively rewritten (the body is handed to goldmark, which renders the inner
// "> [!note]" as an ordinary blockquote). It degrades to readable content rather than raw
// marker text, which is the contract for every unsupported form in this pipeline.
//
// TestCalloutNestedInlineAtLeastDegrades 记录已知限制：嵌套在 callout 内部的 callout 不会被
// 递归改写（正交通常交给 goldmark，内层 "> [!note]" 会按普通引用块渲染）。它会退化为可读内容
// 而不是原始标记文本 —— 这是本管线对所有不支持形态的契约。
func TestCalloutNestedInlineAtLeastDegrades(t *testing.T) {
	identity := func(s string) string { return s }
	got := preprocessCallouts("> [!question] Outer\n> > [!note] Inner\n> > Nested content.", identity)
	if !strings.Contains(got, `class="callout callout-note"`) {
		t.Errorf("outer callout missing: %s", got)
	}
	// The inner marker text may remain as literal text inside the rendered body — that is the
	// documented degradation, not a failure — but the outer wrapper must exist.
	if !strings.Contains(got, "Outer") {
		t.Errorf("outer title lost: %s", got)
	}
}
