package service

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// markTestRenderer is the same renderer configuration the service builds, minus everything that
// needs a live repository. The point is to exercise the mark extension through the real goldmark
// pipeline rather than calling the parser directly.
//
// markTestRenderer 与服务实际构建的渲染器配置相同，只去掉了需要真实仓储的部分。
// 目的是让 mark 扩展走真实的 goldmark 管线，而不是直接调用解析器。
func markTestRenderer() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.CJK,
			extension.Typographer,
			Mark,
		),
		goldmark.WithRendererOptions(html.WithUnsafe(), html.WithHardWraps()),
	)
}

func renderMark(t *testing.T, src string) string {
	t.Helper()
	var b strings.Builder
	if err := markTestRenderer().Convert([]byte(src), &b); err != nil {
		t.Fatalf("convert: %v", err)
	}
	return b.String()
}

// TestMarkSingleEqualsIsNotADelimiter 单个等号不是高亮定界符。
//
// 这是本仓库上最重要的一条：77 篇已发布笔记含孤立 "="，0 篇含 "=="。
// 把单个 "=" 当定界符时，一行里出现两个等号就会把中间的一切包进 <mark> ——
// 「头部=颅部+面部」这类句子因此整段变成高亮，而作者从未写过任何高亮标记。
func TestMarkSingleEqualsIsNotADelimiter(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"等号连接两个词", "头部=颅部 + 面部。"},
		{"一行里两个等号", "颅部=颅脑+颅面，交界=颅底；前脑形成并发育成颅面大部分。"},
		{"边界情况", "上界=额鼻突+中鼻突；下界=2下颌突；两侧=2上颌突。"},
		{"等号加粗混排", "**总体**：出生时面部**宽度**最大；出生后增长量=**高度 > 深度 > 宽度**。"},
		{"公式", "2. 公式： 替牙间隙 =（Ⅲ+Ⅳ+Ⅴ）－（3+4+5）"},
		{"遗传加环境", "生物体亲代与子代间存在着一定差异，**遗传+环境因素=自然规律**。"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderMark(t, tc.src)
			if strings.Contains(got, "<mark") {
				t.Fatalf("a single '=' produced a highlight:\n  src=%q\n  html=%s", tc.src, got)
			}
		})
	}
}

// TestMarkDoubleEqualsStillHighlights "==" 仍然高亮。
//
// 修掉单个等号之后必须确认没有连功能一起关掉 —— 否则「修好了误报」与「功能消失」
// 在页面上是同一种表现，而这一条是唯一能区分两者的东西。
func TestMarkDoubleEqualsStillHighlights(t *testing.T) {
	got := renderMark(t, "这是 ==高亮== 文本。")
	if !strings.Contains(got, "<mark>高亮</mark>") {
		t.Fatalf("'==' did not highlight: %s", got)
	}
}

// TestMarkTripleEqualsIsRejected "===" 不被当作高亮。
//
// "===" 既是 setext 下划线的候选，也是常见误输入。允许它会把一个原本是分隔线的行
// 变成一段高亮。
func TestMarkTripleEqualsIsRejected(t *testing.T) {
	src := "上一行\n===\n下一行"
	got := renderMark(t, src)
	if strings.Contains(got, "<mark") {
		t.Fatalf("'===' produced a highlight: %s", got)
	}
}

// TestMarkUnclosedDoubleEquals 未闭合的 "==" 不吞掉后文。
//
// 一个只有开头没有结尾的 "==" 不应把余下整篇都包进 <mark> —— 那正是这次要修的那类症状，
// 只是触发字符从一个变成两个。
func TestMarkUnclosedDoubleEquals(t *testing.T) {
	got := renderMark(t, "开头 ==没有闭合的段落。\n\n第二段正常文本。")
	// 未闭合时不应产生 mark 节点。
	if strings.Contains(got, "<mark") {
		t.Fatalf("an unclosed '==' still opened a highlight: %s", got)
	}
}

// TestMarkHardWrapsDoesNotBreakBlocks hardWrap 不破坏块级结构。
//
// 同时启用 hard wrap 与 mark 之后要确认：列表项之间、表格行之间、段落之间
// 仍然是块级边界，而不是被折成 <br>。
func TestMarkHardWrapsDoesNotBreakBlocks(t *testing.T) {
	// 列表:两项之间是块级边界
	got := renderMark(t, "- 第一项\n- 第二项\n- 第三项")
	if n := strings.Count(got, "<li>"); n != 3 {
		t.Fatalf("expected 3 list items, got %d: %s", n, got)
	}
	if strings.Contains(got, "<li>第一项<br>") {
		t.Fatalf("list items were joined by a hard wrap: %s", got)
	}

	// 段落:空行分隔仍是两个 <p>
	got = renderMark(t, "第一段\n\n第二段")
	if n := strings.Count(got, "<p>"); n != 2 {
		t.Fatalf("expected 2 paragraphs, got %d: %s", n, got)
	}

	// 段落内的单换行应当成为 <br>
	got = renderMark(t, "第一行\n第二行")
	if !strings.Contains(got, "<br") {
		t.Fatalf("a soft break inside a paragraph did not become <br>: %s", got)
	}

	// 表格:行之间不应有 <br>
	got = renderMark(t, "| A | B |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |")
	if !strings.Contains(got, "<table") {
		t.Fatalf("table did not render: %s", got)
	}
	tbl := got[strings.Index(got, "<table"):]
	if strings.Contains(tbl, "<br") {
		t.Fatalf("table rows were joined by a hard wrap: %s", got)
	}
}
