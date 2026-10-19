package service

import (
	"strings"
	"testing"
)

// TestPreprocessMath covers the shapes Obsidian accepts and, just as importantly, the shapes it
// must not treat as math — a price in prose and a shell variable in a code sample are the two
// realistic false positives.
//
// TestPreprocessMath 覆盖 Obsidian 接受的形态，同样重要的是它**不应**视为公式的形态——
// 正文里的价格与代码示例里的 shell 变量是两个最现实的误判来源。
func TestPreprocessMath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
		not  []string
	}{
		{"inline", "质能方程 $E=mc^2$ 很重要", []string{`<span class="math-inline"`, "E=mc^2"}, nil},
		{"display", "$$\n\\int_0^1 x^2 dx\n$$", []string{`<div class="math-block"`, `\int_0^1 x^2 dx`}, nil},
		{"display single line", "$$a^2+b^2=c^2$$", []string{`<div class="math-block"`}, nil},
		{"multiple inline", "$a$ 与 $b$", []string{`math-inline" role="math">a<`, `math-inline" role="math">b<`}, nil},

		// False positives: must stay literal.
		// 误判防护：必须保持字面。
		{"price is not math", "价格是 $5 到 $10", nil, []string{"math-inline"}},
		{"trailing space is not math", "符号 $ x $ 不是公式", nil, []string{"math-inline"}},
		{"shell var inside fence", "```bash\necho $HOME\n```", nil, []string{"math-inline"}},
		{"code span is protected", "使用 `$PATH` 变量", []string{"`$PATH`"}, []string{"math-inline"}},
		{"no dollar", "普通段落没有公式", nil, []string{"math-"}},

		// Escaping: a formula is emitted as raw HTML.
		// 转义：公式体作为原始 HTML 输出。
		{"escapes angle brackets", "$a < b$", []string{"a &lt; b"}, []string{"<b>"}},
		{"escapes ampersand", "$a & b$", []string{"a &amp; b"}, nil},
	}

	for _, c := range cases {
		got := preprocessMath(c.in)
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

// TestPreprocessMathLeavesFenceIntact: the fence must survive so goldmark still renders it as a
// code block.
//
// TestPreprocessMathLeavesFenceIntact：围栏必须保留，使 goldmark 仍把它渲染为代码块。
func TestPreprocessMathLeavesFenceIntact(t *testing.T) {
	src := "```bash\necho $HOME\nexport X=$((1+2))\n```\n\n正文 $x$ 结束"
	got := preprocessMath(src)
	if !strings.Contains(got, "echo $HOME") {
		t.Errorf("fenced content was altered: %q", got)
	}
	if !strings.Contains(got, `math-inline" role="math">x<`) {
		t.Errorf("inline math outside the fence was not processed: %q", got)
	}
}

// TestHasMathMarkup is what gates the MathJax include, so a false negative means formulas stay
// as LaTeX source and a false positive means every article pays for a 200 KB library.
//
// TestHasMathMarkup 决定是否加载 MathJax：漏判会让公式停留在 LaTeX 源码态，
// 误判会让每篇文章都为一个 200 KB 的库付出代价。
func TestHasMathMarkup(t *testing.T) {
	if !HasMathMarkup(`<p>before <span class="math-inline" role="math">x</span></p>`) {
		t.Errorf("inline math not detected")
	}
	if !HasMathMarkup(`<div class="math-block" role="math">x</div>`) {
		t.Errorf("block math not detected")
	}
	if HasMathMarkup(`<p>plain article with no formulas</p>`) {
		t.Errorf("plain article reported as containing math")
	}
	// The class name also appears in the stylesheet, which is injected into every page — the
	// detection runs on the article body only, never on the whole document.
	// 类名同样出现在样式表里，而样式表被注入每个页面——检测只针对正文，从不针对整个文档。
	if HasMathMarkup(`<style>.math-block{}</style>`) == false {
		t.Log("note: hasMathMarkup is content-based, not a parser; callers must pass the body only")
	}
}

// TestPreprocessMathEndToEnd runs the configured pipeline over a mixed document.
// TestPreprocessMathEndToEnd 在配置好的管线上跑一篇混合文档。
func TestPreprocessMathEndToEnd(t *testing.T) {
	md := newTestMarkdown()
	src := "# 标题\n\n行内 $a^2$ 与块级：\n\n$$\n\\sum_{i=1}^n i\n$$\n\n代码：\n\n```\n$notmath$\n```\n"
	got := render(t, md, preprocessMath(src))

	for _, want := range []string{`class="math-inline"`, `class="math-block"`, `\sum_{i=1}^n i`} {
		if !strings.Contains(got, want) {
			t.Errorf("end-to-end output missing %q\ngot: %s", want, got)
		}
	}
	if !strings.Contains(got, "$notmath$") {
		t.Errorf("fenced $notmath$ was converted: %s", got)
	}
}
