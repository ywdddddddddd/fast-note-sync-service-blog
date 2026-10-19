package routers

import (
	"regexp"
	"strconv"
	"strings"
)

// The outline and the headings have to agree on their anchors, so they are produced together.
//
// Doing it in two passes is how they drifted apart in the first place: the headings keep whatever
// id goldmark generated from the heading text — Chinese, in this vault — while the template
// percent-encodes the same characters inside an href, and `#1-%e7%89%99...` matches no element
// called `1-牙发育异常...`. A fragment that matches nothing is a no-op, so the outline's links did
// nothing and the scroll-spy never found an element to mark.
//
// One pass that both collects the outline and rewrites each heading's id to the same ASCII anchor
// cannot drift, because there is no second traversal to disagree with.
//
// 大纲与标题必须在锚点上一致，因此一起产出。
//
// 分两遍做正是它们当初走散的方式：标题保留 goldmark 依标题文本生成的 id——在这个仓库里是中文——
// 而模板会把 href 里同样的字符做百分号编码，于是 `#1-%e7%89%99...` 匹配不到任何叫
// `1-牙发育异常...` 的元素。匹配不到任何元素的片段是空操作，所以大纲的链接什么也没做，
// 滚动高亮也永远找不到可标记的元素。
//
// 一遍同时收集大纲并把每个标题的 id 改写成同一个 ASCII 锚点，则不可能走散：
// 没有第二次遍历可以与它不一致。

// headingIDAttrRegex matches an id attribute on an h2/h3 opening tag, including its quotes, so a
// single replacement can swap the whole attribute without disturbing the rest of the tag.
//
// headingIDAttrRegex 匹配 h2/h3 开标签上的 id 属性（含引号），使一次替换就能换掉整个属性，
// 而不动标签的其余部分。
var headingIDAttrRegex = regexp.MustCompile(`(?i)(<h[23]\b[^>]*?)\s+id="[^"]*"`)

// extractTOC collects the outline and renumbers the headings to match it.
//
// Headings with no visible text are skipped, and their id is left as generated: the outline would
// have nothing to show for them, so giving them an anchor would create an entry point that is not
// in the outline.
//
// extractTOC 收集大纲并把标题重编号以与之匹配。
//
// 没有可见文本的标题被跳过，其 id 保持原样：大纲对它们无话可写，
// 给它们一个锚点只会造出一个不在大纲里的入口。
func extractTOC(html string) ([]tocEntry, string) {
	ms := headingRegex.FindAllStringSubmatchIndex(html, 80)
	if len(ms) == 0 {
		return nil, html
	}
	out := make([]tocEntry, 0, len(ms))
	var b strings.Builder
	b.Grow(len(html) + 64)
	lastEnd := 0

	for _, m := range ms {
		// Submatch indices: m[0:2] = whole, m[2:4] = level, m[4:6] = id, m[6:8] = text.
		// 子匹配下标：m[0:2] 整体、m[2:4] 级别、m[4:6] 的 id、m[6:8] 的文本。
		fullStart, fullEnd := m[0], m[1]
		level := html[m[2]:m[3]]
		text := strings.TrimSpace(tagStripRe.ReplaceAllString(html[m[6]:m[7]], ""))
		if text == "" {
			continue
		}
		lvl := 2
		if level == "3" {
			lvl = 3
		}
		anchor := "s-" + strconv.Itoa(len(out)+1)
		out = append(out, tocEntry{ID: anchor, Text: text, Level: lvl})

		// Copy everything up to this heading, then the heading with its id replaced.
		// 先复制到这个标题之前的内容，再写入替换过 id 的标题。
		b.WriteString(html[lastEnd:fullStart])
		tag := html[fullStart:fullEnd]
		replaced := headingIDAttrRegex.ReplaceAllString(tag, "${1} id=\""+anchor+"\"")
		if replaced == tag {
			// The tag has no id attribute at all. Add one after the tag name so the anchor still
			// exists — otherwise the outline would link to a heading that cannot be reached.
			//
			// 标签完全没有 id 属性。在标签名之后补一个，使锚点仍然存在——
			// 否则大纲会链到一个无法到达的标题。
			replaced = regexp.MustCompile(`(?i)^<h([23])`).ReplaceAllString(tag, "<h$1 id=\""+anchor+"\"")
		}
		b.WriteString(replaced)
		lastEnd = fullEnd
	}
	b.WriteString(html[lastEnd:])
	return out, b.String()
}
