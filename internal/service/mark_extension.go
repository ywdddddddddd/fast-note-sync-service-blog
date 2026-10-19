package service

import (
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Obsidian highlight syntax: ==highlighted text==  →  <mark>highlighted text</mark>
//
// goldmark v1.8.6 ships Table / Strikethrough / Linkify / TaskList / GFM / Footnote /
// Typographer / DefinitionList / CJK, but has no mark/highlight extension, so this is
// implemented the same way the upstream Strikethrough extension is: a delimiter processor, an
// inline parser and a node renderer.
//
// Doing it as a real inline parser (rather than a regex pre-pass) is what keeps code spans and
// fenced blocks safe: by the time inline parsing runs, those have already been consumed as
// CodeSpan / FencedCodeBlock nodes, so "==" inside them is never seen by this parser.
//
// Obsidian 的高亮语法：==高亮文本==  →  <mark>高亮文本</mark>
//
// goldmark v1.8.6 自带 Table / Strikethrough / Linkify / TaskList / GFM / Footnote /
// Typographer / DefinitionList / CJK，但没有 mark/highlight 扩展，因此这里按上游
// Strikethrough 扩展的同一方式实现：分隔符处理器 + 行内解析器 + 节点渲染器。
//
// 之所以实现为真正的行内解析器（而不是正则预处理），是为了保证代码片段与围栏代码块安全：
// 行内解析运行时，那些内容已经被消费为 CodeSpan / FencedCodeBlock 节点，
// 其中的 "==" 根本不会到达这个解析器。

// KindMark is the AST node kind for highlighted text.
// KindMark 是高亮文本的 AST 节点类型。
var KindMark = gast.NewNodeKind("Mark")

// markNode is an inline node wrapping highlighted text.
// markNode 是包裹高亮文本的行内节点。
type markNode struct {
	gast.BaseInline
}

// Kind implements gast.Node.
func (n *markNode) Kind() gast.NodeKind { return KindMark }

// Dump implements gast.Node (used by AST debug printing).
func (n *markNode) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

func newMarkNode() *markNode { return &markNode{} }

type markDelimiterProcessor struct{}

func (p *markDelimiterProcessor) IsDelimiter(b byte) bool { return b == '=' }

func (p *markDelimiterProcessor) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	return opener.Char == closer.Char
}

func (p *markDelimiterProcessor) OnMatch(consumes int) gast.Node { return newMarkNode() }

var defaultMarkDelimiterProcessor = &markDelimiterProcessor{}

type markParser struct{}

var defaultMarkParser = &markParser{}

func (s *markParser) Trigger() []byte { return []byte{'='} }

func (s *markParser) Parse(parent gast.Node, block text.Reader, pc parser.Context) gast.Node {
	before := block.PrecendingCharacter()
	line, segment := block.PeekLine()

	// The third argument is the minimum run length, and it must be 2.
	//
	// It used to be 1, which made a single "=" a valid delimiter. Two of them on one line — and
	// prose is full of those: "头部=颅部+面部", "上界=额鼻突+中鼻突", any formula — were then paired
	// and everything between them rendered as <mark>. On this vault, 77 published notes contain
	// isolated "=" and none contain "==", so the whole highlight feature was misfiring on ordinary
	// text rather than being unused: an author who never wrote a highlight still saw whole lines
	// highlighted, with no way to tell why.
	//
	// Requiring 2 makes the delimiter mean what Obsidian means by it: "==" opens and closes a
	// highlight, "=" is just an equals sign.
	//
	// 第三个参数是最小连续长度，必须是 2。
	//
	// 它原本是 1，于是单个 "=" 就是合法定界符。一行里出现两个 —— 而正文里到处是：
	// 「头部=颅部+面部」「上界=额鼻突+中鼻突」、任何公式 —— 就会被配成一对，
	// 中间的一切渲染成 <mark>。在这个仓库上，77 篇已发布笔记含孤立 "="，没有一篇含 "==",
	// 所以高亮功能不是「没被使用」，而是在普通文本上误触发：
	// 一个从未写过任何高亮的作者，仍然看到整行整行被高亮，且无从判断原因。
	//
	// 要求 2 之后，定界符的含义才与 Obsidian 一致："==" 开合一个高亮，"=" 就只是一个等号。
	node := parser.ScanDelimiter(line, before, 2, defaultMarkDelimiterProcessor)
	// OriginalLength > 2 rejects "===" (a setext underline candidate and a common typo);
	// a preceding "=" rejects "a===b", where the run is not a delimiter pair.
	// OriginalLength > 2 会拒绝 "==="（setext 下划线候选，也是常见误输入）；
	// 前置的 "=" 会拒绝 "a===b" 这种并非成对分隔符的连续等号。
	if node == nil || node.OriginalLength > 2 || before == '=' {
		return nil
	}

	node.Segment = segment.WithStop(segment.Start + node.OriginalLength)
	block.Advance(node.OriginalLength)
	pc.PushDelimiter(node)
	return node
}

func (s *markParser) CloseBlock(parent gast.Node, pc parser.Context) {}

// markHTMLRenderer renders highlight nodes as <mark>.
// markHTMLRenderer 把高亮节点渲染为 <mark>。
type markHTMLRenderer struct {
	html.Config
}

func newMarkHTMLRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &markHTMLRenderer{Config: html.NewConfig()}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *markHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindMark, r.renderMark)
}

func (r *markHTMLRenderer) renderMark(
	w util.BufWriter, source []byte, n gast.Node, entering bool) (gast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<mark>")
	} else {
		_, _ = w.WriteString("</mark>")
	}
	return gast.WalkContinue, nil
}

// markExtension wires the parser and renderer into a goldmark instance.
// markExtension 把解析器与渲染器接入 goldmark 实例。
type markExtension struct{}

// Mark is the ==highlight== extension.
// Mark 是 ==高亮== 扩展。
var Mark = &markExtension{}

func (e *markExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(defaultMarkParser, 500),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(newMarkHTMLRenderer(), 500),
	))
}
