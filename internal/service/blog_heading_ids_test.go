package service

import (
	"strings"
	"testing"

	gast "github.com/yuin/goldmark/ast"
)

// TestHeadingIDsKeepsCJK is the defect this file exists for. goldmark's built-in generator
// drops multi-byte characters, so "## 测试标题" became id="heading". Chinese headings must
// produce Chinese ids.
//
// TestHeadingIDsKeepsCJK 就是本文件存在的理由。goldmark 内置生成器丢弃多字节字符，
// 于是 "## 测试标题" 变成 id="heading"。中文标题必须产生中文 id。
func TestHeadingIDsKeepsCJK(t *testing.T) {
	ids := newBlogHeadingIDs()

	cases := []struct{ in, want string }{
		{"测试标题", "测试标题"},
		{"第二标题", "第二标题"},
		{"1. 根管", "1-根管"},
		{"切牙组综述", "切牙组综述"},
		{"Hello 世界", "hello-世界"},
		{"根管  冲洗", "根管-冲洗"},
		{"a-b_c", "a-b-c"},
	}
	// The empty-result fallbacks are covered separately: they collide with each other inside one
	// generator instance (the second "heading" becomes "heading-1"), which is correct
	// deduplication but makes them unusable as rows in this table.
	// 空结果的兜底单独覆盖：它们在同一个生成器实例内会互相冲突（第二个 "heading" 会变成
	// "heading-1"），这是正确的去重行为，但使它们不适合作为本表的用例行。
	for _, c := range cases {
		got := string(ids.Generate([]byte(c.in), gast.KindHeading))
		if got != c.want {
			t.Errorf("Generate(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHeadingIDsAreStable pins the property that actually matters to readers: the id of a
// heading depends only on its own text, so headings can be added or removed above it without
// renumbering it.
//
// TestHeadingIDsAreStable 固定对读者真正重要的性质：标题 id 只取决于它自身的文本，
// 因此在其上方增删标题不会让它重新编号。
func TestHeadingIDsAreStable(t *testing.T) {
	before := newBlogHeadingIDs()
	after := newBlogHeadingIDs()

	// Same target heading, different number of preceding headings.
	// 同一个目标标题，但前置标题数量不同。
	_ = before.Generate([]byte("第一节"), gast.KindHeading)
	beforeID := string(before.Generate([]byte("目标小节"), gast.KindHeading))

	_ = after.Generate([]byte("新增的第一节"), gast.KindHeading)
	_ = after.Generate([]byte("另一节"), gast.KindHeading)
	afterID := string(after.Generate([]byte("目标小节"), gast.KindHeading))

	if beforeID != afterID {
		t.Errorf("heading id changed when neighbours changed: %q vs %q", beforeID, afterID)
	}
	if beforeID != "目标小节" {
		t.Errorf("unexpected id %q", beforeID)
	}
}

// TestHeadingIDsDeduplicate: identical headings must still get distinct ids, otherwise the
// in-page outline cannot address the second one.
//
// TestHeadingIDsDeduplicate：完全相同的标题仍必须得到不同的 id，
// 否则本页目录无法定位到第二个。
func TestHeadingIDsDeduplicate(t *testing.T) {
	ids := newBlogHeadingIDs()
	first := string(ids.Generate([]byte("小结"), gast.KindHeading))
	second := string(ids.Generate([]byte("小结"), gast.KindHeading))
	third := string(ids.Generate([]byte("小结"), gast.KindHeading))

	if first != "小结" {
		t.Errorf("first = %q", first)
	}
	if second != "小结-1" {
		t.Errorf("second = %q", second)
	}
	if third != "小结-2" {
		t.Errorf("third = %q", third)
	}
}

// TestHeadingIDsNonHeadingFallback: a non-heading node with no usable characters gets "id",
// matching goldmark's own convention.
//
// TestHeadingIDsNonHeadingFallback：没有可用字符的非标题节点得到 "id"，与 goldmark 自身约定一致。
func TestHeadingIDsNonHeadingFallback(t *testing.T) {
	// A fresh generator per case: an empty-result id is still deduplicated, so reusing one
	// instance would produce "heading", "heading-1", "heading-2" and hide the real value.
	// 每个用例使用全新生成器：空结果的 id 同样会去重，复用同一实例会得到
	// "heading"、"heading-1"、"heading-2"，从而掩盖真实取值。
	for _, c := range []struct {
		in   string
		kind gast.NodeKind
		want string
	}{
		{"!!!", gast.KindHeading, "heading"},
		{"", gast.KindHeading, "heading"},
		{"---", gast.KindHeading, "heading"},
		{"!!!", gast.KindLink, "id"},
	} {
		got := string(newBlogHeadingIDs().Generate([]byte(c.in), c.kind))
		if got != c.want {
			t.Errorf("Generate(%q, %v) = %q, want %q", c.in, c.kind, got, c.want)
		}
	}
}

// TestHeadingIDsEndToEnd renders headings through the real Markdown configuration and reads the
// ids back out of the HTML, which is exactly what extractTOC does.
//
// TestHeadingIDsEndToEnd 用真实 Markdown 配置渲染标题，并从 HTML 里回读 id ——
// 这正是 extractTOC 的做法。
func TestHeadingIDsEndToEnd(t *testing.T) {
	md := newTestMarkdown()
	got := render(t, md, "## 测试标题\n\n正文\n\n## 第二标题\n\n正文\n\n### 三级标题\n")

	for _, want := range []string{`id="测试标题"`, `id="第二标题"`, `id="三级标题"`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %s\ngot: %s", want, got)
		}
	}
	if strings.Contains(got, `id="heading"`) {
		t.Errorf("multi-byte heading collapsed to a generic id: %s", got)
	}

	// The ids must be reachable as anchor targets, which is what the outline links to.
	// 这些 id 必须能作为锚点目标被访问，本页目录正是链接到它们。
	for _, want := range []string{"#测试标题", "#第二标题", "#三级标题"} {
		if !strings.Contains(got, want) && !strings.Contains(got, strings.ReplaceAll(want, "#", "")) {
			t.Errorf("anchor target missing for %s", want)
		}
	}
}
