package service

import (
	"strings"
	"sync"
	"unicode"

	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"
)

// Heading id generation.
//
// goldmark's built-in generator drops every multi-byte character:
//
//	for i := 0; i < len(value); {
//	    v := value[i]
//	    l := util.UTF8Len(v)
//	    i += int(l)
//	    if l != 1 { continue }   // ← every CJK / accented character is skipped here
//	    ...
//	}
//
// Consequences on a Chinese-language vault:
//
//	## 测试标题   → id="heading"
//	## 第二标题   → id="heading-1"
//	## 1. 根管    → id="1-"
//
// So every heading id degenerates into "heading" plus a running counter. Adding or removing one
// heading renumbers all the later ones, which breaks every external deep link and any bookmark
// a reader saved. The in-page outline still works (extractTOC reads the ids back out of the
// rendered HTML), which is exactly why the defect stayed invisible.
//
// 标题 id 生成。
//
// goldmark 内置生成器会丢弃每一个多字节字符：
//
//	for i := 0; i < len(value); {
//	    v := value[i]
//	    l := util.UTF8Len(v)
//	    i += int(l)
//	    if l != 1 { continue }   // ← 所有中日韩 / 带重音字符都在这里被跳过
//	    ...
//	}
//
// 在一个中文仓库里后果是：
//
//	## 测试标题   → id="heading"
//	## 第二标题   → id="heading-1"
//	## 1. 根管    → id="1-"
//
// 于是所有标题 id 都退化成 "heading" 加一个递增序号。增删任意一个标题都会让之后的所有
// 标题重新编号，从而破坏所有外部深链接以及读者保存的书签。本页目录仍然「看起来正常」
// （extractTOC 是从渲染后的 HTML 里回读 id 的），这正是该缺陷长期不可见的原因。

type blogHeadingIDs struct {
	mu     sync.Mutex
	values map[string]bool
}

func newBlogHeadingIDs() *blogHeadingIDs {
	return &blogHeadingIDs{values: make(map[string]bool, 256)}
}

// Generate implements parser.IDs.
//
// It keeps every Unicode letter and digit (CJK included), folds whitespace and the joining
// punctuation "-"/"_" into a single "-", lowercases ASCII, and drops everything else. The result
// is stable: the same heading text always produces the same id, and a heading that moves does
// not renumber its neighbours.
//
// Generate 实现 parser.IDs。
//
// 它保留所有 Unicode 字母与数字（含中日韩），把空白以及与连接有关的标点 "-"/"_" 折叠为单个
// "-"，对 ASCII 转小写，其余字符丢弃。结果是稳定的：相同标题文本永远产生相同 id，
// 而某个标题移动时不会让相邻标题重新编号。
func (s *blogHeadingIDs) Generate(value []byte, kind gast.NodeKind) []byte {
	value = util.TrimLeftSpace(value)
	value = util.TrimRightSpace(value)

	var b strings.Builder
	pendingDash := false
	for _, r := range string(value) {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '-' || r == '_':
			// Collapse runs of separators into one dash, and never emit a leading dash.
			// 把连续分隔符合并成一个短横，且不输出前导短横。
			if b.Len() > 0 {
				pendingDash = true
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if pendingDash {
				b.WriteByte('-')
				pendingDash = false
			}
			b.WriteRune(unicode.ToLower(r))
		default:
			// Punctuation and symbols are dropped, but they do not join words: "a.b" and
			// "a b" must not collapse onto the same id.
			// 标点与符号被丢弃，但不连接词："a.b" 与 "a b" 不应塌成同一个 id。
			pendingDash = b.Len() > 0
		}
	}

	result := b.String()
	if result == "" {
		if kind == gast.KindHeading {
			result = "heading"
		} else {
			result = "id"
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.values[result] {
		s.values[result] = true
		return []byte(result)
	}
	for i := 1; ; i++ {
		candidate := result + "-" + itoaSmall(i)
		if !s.values[candidate] {
			s.values[candidate] = true
			return []byte(candidate)
		}
	}
}

// Put implements parser.IDs.
func (s *blogHeadingIDs) Put(value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[string(value)] = true
}

// itoaSmall renders a small positive int without pulling in strconv.
// itoaSmall 渲染一个小的正整数，避免引入 strconv。
func itoaSmall(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 && i > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// blogHeadingIDOptions returns the per-conversion parse options that install the id generator
// above.
//
// These must be passed to Markdown.Convert, not to goldmark.WithParserOptions: WithContext
// returns a parser.ParseOption (func(*ParseConfig)), while WithParserOptions requires a
// parser.Option (an interface with SetParserOption(*Config)). The two are not interchangeable,
// and passing the former there does not compile.
//
// Going through Convert is also what makes each document start from a fresh table, since
// Convert builds a new ParseConfig per call.
//
// blogHeadingIDOptions 返回安装上述 id 生成器的「每次转换」解析选项。
//
// 它们必须传给 Markdown.Convert，而不是 goldmark.WithParserOptions：WithContext 返回
// parser.ParseOption（func(*ParseConfig)），而 WithParserOptions 要求 parser.Option
// （带 SetParserOption(*Config) 的接口）。两者不可互换，把前者传进去无法通过编译。
//
// 走 Convert 还带来另一个好处：Convert 每次调用都会新建 ParseConfig，
// 因此每篇文档都从一张全新的表开始。
func blogHeadingIDOptions() []parser.ParseOption {
	return []parser.ParseOption{
		parser.WithContext(parser.NewContext(parser.WithIDs(newBlogHeadingIDs()))),
	}
}
