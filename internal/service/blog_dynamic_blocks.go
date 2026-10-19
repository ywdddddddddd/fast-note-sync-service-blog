package service

import (
	"regexp"
	"strings"
)

// Graceful degradation for formats the server cannot evaluate.
//
// The blog is a static, server-rendered site with no Obsidian runtime. Four block types need
// their own treatment:
//
//	mermaid     a diagram; the source is meaningless to a reader, but the diagram is the content
//	dataview    a live query over the vault; the query text is not the content
//	dataviewjs  same, but imperative
//	base        Obsidian Bases, a live view over notes
//	plantuml    a diagram in a preprocessor dialect with no Go implementation
//	canvas      a JSON scene; rendering it needs a canvas editor
//	excalidraw  same, a JSON scene
//
// Every one of them is content in Obsidian and nothing in a static page. Two rules decide what
// to do:
//
//  1. Never show the raw source as if it were the content. A dataview query rendered as a code
//     block tells the reader "the author wrote a query" — that is noise, not information.
//  2. Never silently delete it either. The reader should know that something dynamic existed,
//     and the author should be able to see which notes depend on a plugin.
//
// So each becomes a small labelled card that names what it was and, where the content is
// genuinely readable (a diagram's source, a query), keeps it behind a <details> toggle.
//
// 服务端无法求值的格式的优雅降级。
//
// 博客是无 Obsidian 运行时的静态服务端渲染站点。有七种块需要单独处理：
//
//	mermaid     图表；源码对读者没有意义，但图本身就是内容
//	dataview    对仓库的实时查询；查询文本不是内容
//	dataviewjs  同上，但是命令式
//	base        Obsidian Bases，对笔记的实时视图
//	plantuml    使用带预处理器方言的图表，Go 侧无实现
//	canvas      JSON 场景；渲染它需要画布编辑器
//	excalidraw  同上，JSON 场景
//
// 它们在 Obsidian 里都是内容，在静态页面里什么都不是。两条规则决定怎么做：
//
//  1. 绝不把原始源码当作内容展示。把 dataview 查询渲染成代码块是在告诉读者
//     「作者写了一个查询」——那是噪声，不是信息。
//  2. 也绝不静默删除。读者应当知道这里原本有动态内容，作者也应当能看出哪些笔记依赖插件。
//
// 因此每一类都变成一张带标签的小卡片，说明它原本是什么；而在内容确实可读时
//（图表源码、查询），把源码放在 <details> 折叠里保留。

// dynamicFenceRegex captures the opening fence of a block whose language the server cannot
// evaluate. The language tag is case-insensitive because notes are written by hand — a fence
// written as ```Mermaid must be recognised, and without the (?i) flag it was not.
//
// dynamicFenceRegex 捕获服务端无法求值的块的起始围栏。语言标签不区分大小写，因为笔记是手写的——
// 写成 ```Mermaid 的围栏必须被识别，而缺少 (?i) 标记时它没有被识别到。
var dynamicFenceRegex = regexp.MustCompile("(?mi)^([ \t]*)(`{3,}|~{3,})[ \t]*(mermaid|dataviewjs|dataview|base|plantuml|excalidraw|canvas)[ \t]*$")

// dynamicKinds describes how each unsupported block is presented.
//
// label is what the reader sees; note explains the situation in one sentence; keepSource decides
// whether the source is worth keeping behind a toggle. A diagram's source is worth keeping (a
// reader with a Mermaid renderer elsewhere can use it); a dataview query is kept for the same
// reason, and because it tells the author what the page depends on.
//
// dynamicKinds 描述每种不支持的块如何呈现。
//
// label 是读者看到的东西；note 用一句话解释情况；keepSource 决定源码是否值得留在折叠里。
// 图表源码值得保留（在别处有 Mermaid 渲染器的读者可以用它）；dataview 查询同理，
// 而且它能让作者看出这个页面依赖什么。
var dynamicKinds = map[string]struct {
	label      string
	note       string
	keepSource bool
}{
	"mermaid": {
		label:      "Mermaid 图表",
		note:       "静态站点不渲染交互图表；下方是原始定义，可复制到支持 Mermaid 的工具中查看。",
		keepSource: true,
	},
	"dataview": {
		label:      "Dataview 查询",
		note:       "这是 Obsidian Dataview 的实时查询，结果取决于本地仓库，静态页面无法计算。",
		keepSource: true,
	},
	"dataviewjs": {
		label:      "DataviewJS 脚本",
		note:       "这是 Obsidian DataviewJS 脚本，需要本地仓库才能运行，静态页面无法执行。",
		keepSource: true,
	},
	"base": {
		label:      "Obsidian Base",
		note:       "这是 Obsidian Bases 视图，由本地笔记实时生成，静态页面没有数据源。",
		keepSource: true,
	},
	"plantuml": {
		label:      "PlantUML 图表",
		note:       "静态站点不渲染 PlantUML；下方是原始定义，可复制到支持 PlantUML 的工具中查看。",
		keepSource: true,
	},
	"canvas": {
		label:      "Canvas 画布",
		note:       "这是 Obsidian Canvas 画布，只能在 Obsidian 中查看。",
		keepSource: false,
	},
	"excalidraw": {
		label:      "Excalidraw 绘图",
		note:       "这是 Excalidraw 绘图，只能在支持它的编辑器中查看。",
		keepSource: false,
	},
}

// preprocessDynamicBlocks replaces unsupported fenced blocks with a labelled card.
//
// Math placeholders must already be in place when this runs, otherwise a "$" inside a dataview
// query would be rewritten into a formula span and then swallowed by this pass.
//
// preprocessDynamicBlocks 把不支持的围栏块替换为带标签的卡片。
//
// 本函数运行时数学占位符必须已经就位，否则 dataview 查询里的 "$" 会先被改写成公式标记，
// 再被这一遍吞掉。
func preprocessDynamicBlocks(content string) string {
	if !strings.Contains(content, "```") && !strings.Contains(content, "~~~") {
		return content
	}

	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))

	for i := 0; i < len(lines); i++ {
		m := dynamicFenceRegex.FindStringSubmatch(lines[i])
		if m == nil {
			out = append(out, lines[i])
			continue
		}

		indent, fence, lang := m[1], m[2], strings.ToLower(m[3])
		spec, known := dynamicKinds[lang]
		if !known {
			out = append(out, lines[i])
			continue
		}

		// Collect the body up to the matching closing fence. A closing fence must be at least as
		// long as the opening one (CommonMark), which is why the length is compared rather than
		// the string.
		//
		// 收集正文直到匹配的结束围栏。结束围栏必须不短于起始围栏（CommonMark），
		// 因此比较的是长度而不是字符串。
		body := make([]string, 0, 16)
		j := i + 1
		closed := false
		for ; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if strings.HasPrefix(t, strings.Repeat(string(fence[0]), len(fence))) &&
				strings.Trim(t, string(fence[0])) == "" {
				closed = true
				break
			}
			body = append(body, lines[j])
		}
		if !closed {
			// Unterminated fence: leave it to the Markdown renderer rather than swallowing the
			// rest of the document.
			// 未闭合围栏：交给 Markdown 渲染器，而不是吞掉文档剩余部分。
			out = append(out, lines[i])
			continue
		}
		i = j

		out = append(out, renderDynamicCard(indent, lang, spec.label, spec.note, spec.keepSource, strings.Join(body, "\n")))
	}

	return strings.Join(out, "\n")
}

// renderDynamicCard builds the placeholder card.
//
// It emits raw HTML, which survives because the renderer runs with html.WithUnsafe. The source
// is HTML-escaped: it is user text going into an HTML text node.
//
// renderDynamicCard 构建占位卡片。
//
// 它输出原始 HTML，这能通过是因为渲染器开启了 html.WithUnsafe。源码经过 HTML 转义：
// 它是进入 HTML 文本节点的用户文本。
func renderDynamicCard(indent, lang, label, note string, keepSource bool, body string) string {
	var b strings.Builder
	b.WriteString(indent)
	b.WriteString(`<div class="dynblock" data-kind="`)
	b.WriteString(calloutHTMLReplacer.Replace(lang))
	b.WriteString(`">`)
	b.WriteString("\n")
	b.WriteString(indent)
	b.WriteString(`<div class="dynblock-head"><span class="dynblock-tag">`)
	b.WriteString(calloutHTMLReplacer.Replace(label))
	b.WriteString(`</span></div>`)
	b.WriteString("\n")
	b.WriteString(indent)
	b.WriteString(`<p class="dynblock-note">`)
	b.WriteString(calloutHTMLReplacer.Replace(note))
	b.WriteString(`</p>`)
	b.WriteString("\n")

	if keepSource && strings.TrimSpace(body) != "" {
		b.WriteString(indent)
		b.WriteString(`<details class="dynblock-src"><summary>查看原始定义</summary>`)
		b.WriteString("\n")
		b.WriteString(indent)
		b.WriteString("<pre><code>")
		b.WriteString(calloutHTMLReplacer.Replace(body))
		b.WriteString("</code></pre>")
		b.WriteString("\n")
		b.WriteString(indent)
		b.WriteString(`</details>`)
		b.WriteString("\n")
	}

	b.WriteString(indent)
	b.WriteString(`</div>`)
	return b.String()
}

// pdfEmbedMaxBytes caps inline PDF embedding at 20 MB.
//
// The limit exists because an <embed> makes the viewer fetch the whole document on page load,
// and a scanned textbook running to hundreds of megabytes would turn opening an article into a
// download. Larger files keep the link-only treatment.
//
// pdfEmbedMaxBytes 把 PDF 内联上限设为 20 MB。
//
// 该限制的存在是因为 <embed> 会让阅读器在页面加载时就取回整个文档，
// 而一本数百 MB 的扫描教材会把「打开一篇文章」变成一次下载。更大的文件保持仅链接的处理。
const pdfEmbedMaxBytes = 20 * 1024 * 1024
