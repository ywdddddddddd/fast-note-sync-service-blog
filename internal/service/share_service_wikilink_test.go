package service

import (
	"strings"
	"testing"
)

// testLabel mirrors the alias rule rewriteWikiLinks passes in.
// testLabel 复现 rewriteWikiLinks 传入的别名规则。
func testLabel(target, alias string) string {
	if strings.TrimSpace(alias) != "" {
		return strings.TrimSpace(alias)
	}
	return target
}

// TestRewriteWikiLinkSyntaxEmitsAnchor is the regression test for the defect this change fixes.
//
// A note pasted from WPS keeps its content inside a raw <table> block. CommonMark does not
// parse Markdown inside an HTML block, so emitting "[前磨牙](/blog/…)" left the reader looking
// at the markup. The substitution now emits an anchor, which renders in both contexts.
//
// TestRewriteWikiLinkSyntaxEmitsAnchor 是本次修复的回归测试。
//
// 从 WPS 粘贴的笔记把正文包在原始 <table> 块里。CommonMark 在 HTML 块内部不解析 Markdown，
// 因此输出「[前磨牙](/blog/…)」会让读者看到标记本身。替换逻辑现在输出锚点，
// 在两种上下文里都能渲染。
func TestRewriteWikiLinkSyntaxEmitsAnchor(t *testing.T) {
	byPath := map[string]string{
		"（1）口腔笔记/口解笔记/牙体解剖与口腔功能/3、前磨牙": "1.135-94e527cec3",
	}
	byName := map[string]string{
		"5、下颌第一磨牙": "1.137-99975c2826",
	}

	cases := []struct {
		name    string
		in      string
		want    string
		notWant string
	}{
		{
			name: "表格内全路径链接",
			in:   `<table><tr><td><p>磨牙位于[[（1）口腔笔记/口解笔记/牙体解剖与口腔功能/3、前磨牙|前磨牙]]的远中</p></td></tr></table>`,
			want: `<table><tr><td><p>磨牙位于<a href="/blog/1.135-94e527cec3">前磨牙</a>的远中</p></td></tr></table>`,
		},
		{
			name: "正文裸名链接",
			in:   `### 相关 [[5、下颌第一磨牙|5、下颌第一磨牙]] 对照`,
			want: `<a href="/blog/1.137-99975c2826">5、下颌第一磨牙</a>`,
		},
		{
			name: "无别名时用目标名作文本",
			in:   `[[5、下颌第一磨牙]]`,
			want: `<a href="/blog/1.137-99975c2826">5、下颌第一磨牙</a>`,
		},
		{
			name: "未分享目标降级为纯文本",
			in:   `[[不存在的笔记|别名]]`,
			want: `别名`,
		},
		{
			name: "锚点被剥离后再查表",
			in:   `[[5、下颌第一磨牙#牙根|看这里]]`,
			want: `<a href="/blog/1.137-99975c2826">看这里</a>`,
		},
		{
			name: ".md 后缀被剥离",
			in:   `[[5、下颌第一磨牙.md|对照]]`,
			want: `<a href="/blog/1.137-99975c2826">对照</a>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteWikiLinkSyntax(tc.in, byPath, byName, testLabel)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("输出未包含期望片段\n  输入: %s\n  实际: %s\n  期望含: %s", tc.in, got, tc.want)
			}
			if strings.Contains(got, "](/blog/") {
				t.Fatalf("仍有 Markdown 链接语法残留: %s", got)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Fatalf("输出不应包含 %q: %s", tc.notWant, got)
			}
		})
	}
}

// TestRewriteWikiLinkSyntaxEscapesLabel makes sure an alias cannot inject markup.
//
// The text lands inside an attribute-adjacent HTML context, so an unescaped "<" would let a
// note author break out of the anchor.
//
// TestRewriteWikiLinkSyntaxEscapesLabel 确保别名无法注入标记。
//
// 文本落在紧邻属性的 HTML 上下文中，未转义的「<」会让笔记作者从锚点里逃逸出来。
func TestRewriteWikiLinkSyntaxEscapesLabel(t *testing.T) {
	byName := map[string]string{"目标": "1.1-abc"}
	got := rewriteWikiLinkSyntax(`[[目标|<img src=x onerror=alert(1)>]]`, nil, byName, testLabel)
	if strings.Contains(got, "<img") {
		t.Fatalf("别名未被转义，可能注入 HTML: %s", got)
	}
	if !strings.Contains(got, "&lt;img") {
		t.Fatalf("别名应被转义为实体: %s", got)
	}
	if !strings.Contains(got, `href="/blog/1.1-abc"`) {
		t.Fatalf("锚点 href 丢失: %s", got)
	}
}

// TestRewriteWikiLinkSyntaxKeepsEmbedsAsText pins the residual-embed rule.
//
// By the time this runs, GetSharedNote has already turned resolvable "![[…]]" into <img>.
// Anything left is an attachment that is not shared, and showing its name is better than
// showing the syntax.
//
// TestRewriteWikiLinkSyntaxKeepsEmbedsAsText 钉住残留嵌入的规则。
//
// 执行到这里时，可解析的「![[…]]」已被 GetSharedNote 换成 <img>。剩下的都是未分享的附件，
// 显示名称总好过显示语法本身。
func TestRewriteWikiLinkSyntaxKeepsEmbedsAsText(t *testing.T) {
	got := rewriteWikiLinkSyntax(`见 ![[未同步的图.png|示意图]] 一节`, nil, nil, testLabel)
	if strings.Contains(got, "![[") {
		t.Fatalf("嵌入语法未被清除: %s", got)
	}
	if !strings.Contains(got, "示意图") {
		t.Fatalf("应保留可读名称: %s", got)
	}
}
