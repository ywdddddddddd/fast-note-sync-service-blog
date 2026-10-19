package service

import (
	"regexp"
	"strings"
)

// Math support (Obsidian's $inline$ and $$display$$).
//
// Why the server does not typeset the formulas: there is no production-grade TeX→HTML renderer
// in the Go ecosystem (KaTeX and MathJax are JavaScript). Embedding a browser would be a much
// larger dependency than the feature justifies.
//
// So the server does the part it can do reliably — recognise the math, keep its source intact,
// and mark it up so it can be laid out — and the article page loads MathJax only when that
// markup is present. Without JavaScript the LaTeX source is still on the page and readable;
// nothing is faked and nothing is dropped.
//
// 数学公式支持（Obsidian 的 $行内$ 与 $$独立$$）。
//
// 服务端为什么不排版公式：Go 生态没有生产级的 TeX→HTML 渲染器（KaTeX 与 MathJax 都是
// JavaScript）。为此内嵌一个浏览器，依赖体量远超这项功能本身的正当性。
//
// 因此服务端只做它能可靠完成的部分——识别公式、原样保留其源码、加上标记以便排版——而文章页
// 仅在出现该标记时才加载 MathJax。没有 JavaScript 时，LaTeX 源码仍然在页面上且可读：
// 不伪造任何东西，也不丢弃任何东西。

// mathFenceRegex spots a fenced code block start/end so "$" inside a code sample is untouched.
// mathFenceRegex 识别围栏代码块的起始/结束，使代码示例里的 "$" 不被处理。
var mathFenceRegex = regexp.MustCompile("^\\s*(```|~~~)")

// mathBlockRegex matches a display formula: $$ … $$ possibly spanning lines.
// mathBlockRegex 匹配独立公式：$$ … $$，可跨行。
var mathBlockRegex = regexp.MustCompile(`(?s)\$\$(.+?)\$\$`)

// mathInlineRegex matches an inline formula.
//
// The rules mirror what Obsidian accepts, which is also what keeps prices out of it:
//   - the opening "$" must not itself be preceded by "$" or an alphanumeric, so the first "$" of
//     a "$$" pair and the tail of "US$5" are not openers;
//   - the opening "$" must be followed by a non-space, so "$ 100" is not math;
//   - the closing "$" must be preceded by a non-space, so "100 $" is not math — and, critically,
//     this is a mandatory character rather than an optional one, because an optional trailing
//     class accepts an empty match and turns "$5 到 $10" into a formula reading "5 到 ";
//   - the body may not contain "$" or a newline;
//   - the body is length-capped, and the optional tail means a single-character body such as
//     "$a$" still matches (an earlier version required two characters and silently dropped
//     every one-character formula).
//
// mathInlineRegex 匹配行内公式。
//
// 规则与 Obsidian 接受的一致，这也是把价格排除在外的原因：
//   - 起始 "$" 之前不得紧邻 "$" 或字母数字，因此 "$$" 对的第一个 "$" 与 "US$5" 的尾部都不算起始；
//   - 起始 "$" 之后必须是非空白，因此 "$ 100" 不是公式；
//   - 结束 "$" 之前必须是非空白，因此 "100 $" 不是公式——关键在于这是一个**必需**字符而不是
//     可选字符，因为可选的尾部字符类会接受空匹配，把 "$5 到 $10" 变成正文为 "5 到 " 的公式；
//   - 公式体内不得包含 "$" 或换行；
//   - 公式体有长度上限，避免正文里一个孤立的 "$" 吞掉半个段落。
var mathInlineRegex = regexp.MustCompile(`(^|[^$\p{L}\p{N}])\$([^\s$](?:[^$\n]{0,496}[^\s$])?)\$`)

// mathPlaceholderPrefix marks a region that must not be re-scanned for inline math (a display
// formula, or an inline code span).
//
// Using a placeholder rather than an HTML tag keeps this function independent of the renderer:
// the placeholder is swapped back before goldmark sees the text.
//
// mathPlaceholderPrefix 标记一块不应再次扫描行内公式的区域（独立公式或行内代码）。
//
// 用占位符而不是 HTML 标签，使本函数不依赖渲染器：占位符在交给 goldmark 之前会被换回。
const mathPlaceholderPrefix = "\x00math:"

// preprocessMath rewrites $…$ and $$…$$ into elements the page can typeset.
//
// It runs before Markdown conversion. Fenced blocks and inline code spans are protected first,
// because a shell command or a code sample containing "$" must survive untouched.
//
// preprocessMath 把 $…$ 与 $$…$$ 改写为页面可以排版的元素。
//
// 它在 Markdown 转换之前运行。围栏代码块与行内代码片段会先被保护起来，
// 因为包含 "$" 的 shell 命令或代码示例必须原样保留。
func preprocessMath(content string) string {
	if !strings.Contains(content, "$") {
		return content
	}

	// Display formulas are extracted first and replaced by placeholders, because "$$x$$" would
	// otherwise be shredded by the inline pass: it would match the text between the middle two
	// dollars and leave the outer two behind.
	//
	// 独立公式先被提取并替换为占位符，否则 "$$x$$" 会被行内处理拆散：
	// 它会匹配中间两个美元符号之间的内容，并把最外侧两个留在原地。
	blocks := []string{}
	content = mathBlockRegex.ReplaceAllStringFunc(content, func(m string) string {
		body := strings.TrimSpace(m[2 : len(m)-2])
		if body == "" {
			return m
		}
		blocks = append(blocks, body)
		return "\n\x00mathblock:" + itoaSmall(len(blocks)-1) + "\x00\n"
	})

	lines := strings.Split(content, "\n")
	inFence := false
	fenceMark := ""

	for i, line := range lines {
		if m := mathFenceRegex.FindStringSubmatch(line); m != nil {
			if !inFence {
				inFence, fenceMark = true, strings.TrimSpace(m[1])
			} else if strings.HasPrefix(strings.TrimSpace(line), fenceMark) {
				inFence, fenceMark = false, ""
			}
			continue
		}
		if inFence {
			continue
		}
		lines[i] = replaceMathOutsideCode(line)
	}

	joined := strings.Join(lines, "\n")

	// Restore the display formulas extracted at the top.
	// 还原开头提取出来的独立公式。
	for i, body := range blocks {
		joined = strings.Replace(joined,
			"\x00mathblock:"+itoaSmall(i)+"\x00",
			"<div class=\"math-block\" role=\"math\">"+htmlEscapeText(body)+"</div>",
			1)
	}

	return joined
}

// replaceMathOutsideCode handles one line: inline code spans are protected, then inline math is
// rewritten.
//
// replaceMathOutsideCode 处理单行：先保护行内代码片段，再改写行内公式。
func replaceMathOutsideCode(line string) string {
	if !strings.Contains(line, "$") {
		return line
	}

	// Protect `code spans` so their "$" is inert.
	// 保护 `行内代码`，使其中的 "$" 不参与匹配。
	protected := []string{}
	line = inlineCodeRegex.ReplaceAllStringFunc(line, func(m string) string {
		protected = append(protected, m)
		return mathPlaceholderPrefix + itoaSmall(len(protected)-1) + "\x00"
	})

	line = mathInlineRegex.ReplaceAllStringFunc(line, func(m string) string {
		// Group 1 is the required non-dollar prefix (possibly empty at line start); the body is
		// everything between the two "$" that follow it.
		//
		// 第 1 组是必需的「非 $」前缀（行首时为空）；公式体是其后两个 "$" 之间的内容。
		sub := mathInlineRegex.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		prefix, body := sub[1], sub[2]
		if strings.TrimSpace(body) == "" {
			return m
		}
		return prefix + "<span class=\"math-inline\" role=\"math\">" + htmlEscapeText(body) + "</span>"
	})

	// Restore the protected code spans.
	// 还原被保护的行内代码。
	for i, original := range protected {
		line = strings.Replace(line, mathPlaceholderPrefix+itoaSmall(i)+"\x00", original, 1)
	}
	return line
}

// inlineCodeRegex matches a `code span` (single backticks, no newline).
// inlineCodeRegex 匹配 `行内代码`（单反引号、无换行）。
var inlineCodeRegex = regexp.MustCompile("`[^`\n]+`")

// htmlEscapeText escapes the characters that would otherwise be interpreted as markup.
//
// The math body is emitted as raw HTML (the surrounding document is rendered with
// html.WithUnsafe), so "<" and "&" must be escaped here — a formula such as a < b would
// otherwise open a tag.
//
// htmlEscapeText 转义会被解释为标记的字符。
//
// 公式体作为原始 HTML 输出（外围文档用 html.WithUnsafe 渲染），因此这里的 "<" 与 "&"
// 必须转义——否则像 a < b 这样的公式会打开一个标签。
func htmlEscapeText(s string) string {
	if !strings.ContainsAny(s, `<>&`) {
		return s
	}
	return mathHTMLEscaper.Replace(s)
}

var mathHTMLEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

// HasMathMarkup reports whether rendered article HTML contains any formula, so the article page
// only does work for formulas when there is something to typeset.
//
// It is exported because the template layer needs it: the decision belongs next to the markup
// that produces those class names, not duplicated in the router.
//
// HasMathMarkup 判断已渲染的文章 HTML 中是否含公式，使文章页只在确实有东西需要排版时才做事。
//
// 之所以导出，是因为模板层需要它：这个判断应当与产出这些类名的标记放在一起，
// 而不是在路由里重复一遍。
func HasMathMarkup(html string) bool {
	return strings.Contains(html, `class="math-block"`) || strings.Contains(html, `class="math-inline"`)
}
