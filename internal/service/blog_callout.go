package service

import (
	"regexp"
	"strings"
)

// Obsidian callouts / admonitions.
//
// Syntax in a note:
//
//	> [!note] 可选标题
//	> 正文（可多行、可含列表、可含代码块）
//
// goldmark has no callout support, and a plain blockquote would render the marker as literal
// text. Callouts are turned into raw HTML before the Markdown pass, because the body must stay
// Markdown: the body is rendered recursively with the same goldmark instance and its HTML is
// embedded, so a callout containing a table, a code block or a wiki link renders correctly.
//
// WithUnsafe() (already on) is what lets the resulting HTML survive the renderer.
//
// Obsidian 的 callout / admonition。
//
// 笔记中的语法：
//
//	> [!note] 可选标题
//	> 正文（可多行、可含列表、可含代码块）
//
// goldmark 不支持 callout，而按普通引用块渲染会把标记当字面文本输出。因此 callout 在 Markdown
// 渲染之前被转成原始 HTML；正文必须保持 Markdown —— 正文用同一个 goldmark 实例递归渲染，
// 其 HTML 被嵌入，所以包含表格、代码块或 wiki 内链的 callout 也能正确渲染。
//
// 已开启的 WithUnsafe() 正是让这段 HTML 能通过渲染器的前提。

// calloutStartRegex matches the opening line: "> [!type]" with optional fold marker and title.
// calloutStartRegex 匹配起始行："> [!type]"，可选折叠标记与标题。
var calloutStartRegex = regexp.MustCompile(`^>\s*\[!([A-Za-z][A-Za-z0-9_-]*)\]([+-]?)[ \t]*(.*)$`)

// calloutQuoteRegex matches a continuation line of the same blockquote.
// calloutQuoteRegex 匹配同一引用块的后续行。
var calloutQuoteRegex = regexp.MustCompile(`^>\s?(.*)$`)

// calloutFenceRegex spots a fenced code block, so a ">" inside a fence is not treated as a
// continuation line of the callout.
//
// calloutFenceRegex 识别围栏代码块，使围栏内的 ">" 不会被当作 callout 的续行。
var calloutFenceRegex = regexp.MustCompile("^\\s*(```|~~~)")

// calloutKnownTypes maps Obsidian's built-in callout names to (icon, colour family). Unknown
// types still render — they fall back to the "note" presentation rather than being dropped, so
// a custom callout degrades to a readable box instead of raw text.
//
// calloutKnownTypes 把 Obsidian 内置 callout 名映射到（图标，色系）。未知类型仍然渲染 ——
// 回退到 note 的呈现方式而不是被丢弃，因此自定义 callout 会退化为可读的方框而非原始文本。
var calloutKnownTypes = map[string]struct {
	icon  string
	label string
}{
	"note":      {"i", "笔记"},
	"info":      {"i", "信息"},
	"tip":       {"✦", "提示"},
	"hint":      {"✦", "提示"},
	"important": {"!", "重要"},
	"warning":   {"!", "警告"},
	"caution":   {"!", "注意"},
	"attention": {"!", "注意"},
	"danger":    {"!", "危险"},
	"error":     {"✕", "错误"},
	"bug":       {"✕", "缺陷"},
	"success":   {"✓", "完成"},
	"check":     {"✓", "完成"},
	"done":      {"✓", "完成"},
	"question":  {"?", "疑问"},
	"help":      {"?", "疑问"},
	"faq":       {"?", "问答"},
	"quote":     {"❝", "引用"},
	"cite":      {"❝", "引用"},
	"example":   {"≡", "示例"},
	"abstract":  {"≡", "摘要"},
	"summary":   {"≡", "摘要"},
	"tldr":      {"≡", "摘要"},
	"todo":      {"☐", "待办"},
}

// calloutColors maps a callout type to one of the CSS colour families defined in the blog
// stylesheet. Keeping the list short is deliberate: the blog's palette is warm-grey plus one
// accent, and every extra hue makes the page cheaper-looking.
//
// calloutColors 把 callout 类型映射到博客样式表里定义的某个色系。列表刻意很短：
// 博客调色板是暖灰加一个强调色，每多一种色相都会让页面显得更廉价。
var calloutColors = map[string]string{
	"warning":   "warn",
	"caution":   "warn",
	"attention": "warn",
	"danger":    "danger",
	"error":     "danger",
	"bug":       "danger",
	"success":   "ok",
	"check":     "ok",
	"done":      "ok",
	"tip":       "tip",
	"hint":      "tip",
	"quote":     "quote",
	"cite":      "quote",
}

// calloutHTMLEscaper escapes text that goes into an HTML attribute or a text node inside the
// generated wrapper. Only the title and the type name take this path; the body never does,
// because the body is rendered by goldmark.
//
// calloutHTMLEscaper 转义进入生成包裹层的 HTML 属性或文本节点的内容。只有标题与类型名走这条路；
// 正文不走，因为正文由 goldmark 渲染。
var calloutHTMLReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
)

// preprocessCallouts rewrites callout blockquotes into raw HTML wrappers around their rendered
// bodies.
//
// The render callback is the caller's Markdown renderer, so this function does not need to know
// which goldmark instance is in use and cannot accidentally create a second one (two instances
// would disagree about extensions).
//
// preprocessCallouts 把 callout 引用块改写为「原始 HTML 包裹层 + 已渲染正文」。
//
// render 回调就是调用方的 Markdown 渲染器，因此本函数不需要知道用的是哪个 goldmark 实例，
// 也不可能意外创建第二个（两个实例会在扩展上产生分歧）。
func preprocessCallouts(content string, render func(string) string) string {
	if !strings.Contains(content, "[!") {
		return content
	}

	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	fenceMark := ""

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Track fenced blocks so callout markers inside code samples stay literal.
		// 跟踪围栏块，使代码示例里的 callout 标记保持字面输出。
		if m := calloutFenceRegex.FindStringSubmatch(line); m != nil {
			if !inFence {
				inFence, fenceMark = true, m[0]
			} else if strings.Contains(m[0], strings.TrimSpace(fenceMark)[:1]) {
				inFence, fenceMark = false, ""
			}
			out = append(out, line)
			continue
		}
		if inFence {
			out = append(out, line)
			continue
		}

		m := calloutStartRegex.FindStringSubmatch(line)
		if m == nil {
			out = append(out, line)
			continue
		}

		ctype := strings.ToLower(m[1])
		fold := m[2]
		title := strings.TrimSpace(m[3])

		// Collect the block body: consecutive ">" lines, plus blank lines that are followed by
		// another ">" line (a paragraph break inside the callout).
		//
		// 收集块正文：连续的 ">" 行，以及后面仍跟着 ">" 行的空行（callout 内部的段落分隔）。
		body := make([]string, 0, 8)
		j := i + 1
		for j < len(lines) {
			if bm := calloutQuoteRegex.FindStringSubmatch(lines[j]); bm != nil {
				body = append(body, bm[1])
				j++
				continue
			}
			// A blank line continues the callout only if the next line is also quoted.
			// 空行只有在下一行同样被引用时才延续 callout。
			if strings.TrimSpace(lines[j]) == "" && j+1 < len(lines) && calloutQuoteRegex.MatchString(lines[j+1]) {
				body = append(body, "")
				j++
				continue
			}
			break
		}
		i = j - 1

		out = append(out, renderCallout(ctype, fold, title, strings.Join(body, "\n"), render))
	}

	return strings.Join(out, "\n")
}

// renderCallout builds the HTML wrapper for one callout.
// renderCallout 为单个 callout 构建 HTML 包裹层。
func renderCallout(ctype, fold, title, body string, render func(string) string) string {
	presentation, known := calloutKnownTypes[ctype]
	if !known {
		presentation = calloutKnownTypes["note"]
	}
	color, ok := calloutColors[ctype]
	if !ok {
		color = "note"
	}
	if title == "" {
		title = presentation.label
	}

	// Obsidian's "-" means "collapsed by default", "+" means "expanded". Both render as a
	// native <details> element so the toggle works without JavaScript.
	//
	// The two facts are tracked separately on purpose: an earlier version used the presence of
	// the " open" attribute as the "is foldable" test, which silently dropped the collapsed
	// form — the most common one — because its attribute string is empty by construction.
	//
	// Obsidian 的 "-" 表示默认折叠，"+" 表示默认展开。两者都渲染为原生 <details> 元素，
	// 因此开关无需 JavaScript 即可工作。
	//
	// 这两个事实刻意分开记录：早先的版本用 " open" 属性是否存在来充当「是否可折叠」的判断，
	// 结果静默丢掉了折叠形态（也是最常见的那种）—— 因为它的属性串按构造就是空的。
	foldable := fold == "+" || fold == "-"
	foldOpen := fold == "+"

	bodyHTML := strings.TrimSpace(render(body))

	var b strings.Builder
	b.WriteString(`<div class="callout callout-` + color + `" data-callout="` + calloutHTMLReplacer.Replace(ctype) + `">`)
	b.WriteString("\n")
	b.WriteString(`<div class="callout-head"><span class="callout-ico" aria-hidden="true">` +
		calloutHTMLReplacer.Replace(presentation.icon) + `</span><span class="callout-title">` +
		calloutHTMLReplacer.Replace(title) + `</span></div>`)
	b.WriteString("\n")
	b.WriteString(`<div class="callout-body">`)
	b.WriteString("\n")
	if foldable {
		b.WriteString("<details")
		if foldOpen {
			b.WriteString(" open")
		}
		b.WriteString("><summary>")
		b.WriteString(calloutHTMLReplacer.Replace(title))
		b.WriteString("</summary>")
		b.WriteString("\n")
		b.WriteString(bodyHTML)
		b.WriteString("\n</details>")
	} else {
		b.WriteString(bodyHTML)
	}
	b.WriteString("\n</div>\n</div>")
	return b.String()
}
