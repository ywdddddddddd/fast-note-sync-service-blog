package service

import (
	"regexp"
	"strings"
)

// Page containers: the three block forms a page note may use for layout.
//
// This is a whitelist, and that is the whole point rather than an implementation detail. The
// project excludes user-authored HTML and CSS; accepting an arbitrary container name would be
// exactly that, because `::: anything` would become `<div class="anything">`. Three names keep the
// feature a layout vocabulary instead of a templating language.
//
// A name outside the list is left as literal text. That is deliberate, and it is the only one of
// the three possible behaviours that is honest: erroring would fail the whole page over a typo,
// and silently dropping the marker would leave the author wondering where their content went.
// Print the marker and the author sees their own mistake.
//
// 页面容器：页面笔记可用的三种块级排版形式。
//
// 这是白名单，而这一点正是它的全部意义，不是实现细节。本项目排除用户自写的 HTML 与 CSS；
// 接受任意容器名恰恰就是那个东西——因为 `::: 任意名` 会变成 `<div class="任意名">`。
// 三个名字让这个能力停留在「排版词汇」而不是变成「模板语言」。
//
// 不在名单里的名字原样保留为文本。这是有意的，也是三种可能行为里唯一诚实的那个：
// 报错会因为一个错字让整页失败；静默丢弃标记则会让作者纳闷自己的内容去哪了。
// 把标记打印出来，作者就看见了自己的错。
// pageContainerNames is the whitelist of container names a page note may use.
//
// A name that is not here is left verbatim in the output rather than silently dropped: an author
// who mistypes the name should see the markers on the page, not wonder where their block went.
//
// profile-meta was added for the résumé page: seven short facts about a person are a definition
// list, not a paragraph, and rendering them as a run of lines wastes the width and reads worse
// than the two-column grid they fit into.
//
// pageContainerNames 是页面笔记可使用的容器名白名单。
//
// 不在这里的名字会在输出里原样保留，而不是被静默丢弃：写错名字的作者应当在页面上看到那些标记，
// 而不是纳闷自己那一块去哪了。
//
// profile-meta 是为简历页加的：关于一个人的七条短事实是一份定义列表而不是一段文字，
// 把它们渲染成连续的行浪费了宽度，也不如它们本就适合的两列网格好读。
var pageContainerNames = []string{"profile-head", "profile-meta", "timeline", "links"}

// Markers are HTML comments rather than the <div> itself.
//
// Writing `<div class="pc-timeline">` directly does not work: goldmark treats a raw HTML block as
// extending until a blank line, so a list written on the line right after the tag would be
// swallowed as raw HTML instead of becoming a <ul>. Routing through comments — each surrounded by
// blank lines — lets the content between them be parsed as ordinary Markdown, and the comments are
// then swapped for the real tags once the HTML exists.
//
// 标记用 HTML 注释，而不是直接写 <div>。
//
// 直接写 `<div class="pc-timeline">` 不行：goldmark 认为原始 HTML 块一直延伸到空行为止，
// 于是紧接在标签后一行写的列表会被当作原始 HTML 吞掉，而不是变成 <ul>。
// 经由注释（每条前后各加空行）可以让它们之间的内容按普通 Markdown 解析，
// 等 HTML 生成之后再把这些注释换成真正的标签。
const (
	pageContainerOpenPrefix  = "<!--fns-pc-s:"
	pageContainerClosePrefix = "<!--fns-pc-e:"
	pageContainerSuffix      = "-->"
)

// pageContainerOpenRegex matches `::: name` on its own line. The name is captured even when it is
// not in the whitelist, so the caller can decide between rendering a container and leaving the
// text alone.
// pageContainerOpenRegex 匹配独占一行的 `::: name`。即使名字不在白名单里也会捕获，
// 以便调用方在「渲染容器」与「原样保留」之间做判断。
var pageContainerOpenRegex = regexp.MustCompile(`(?m)^[ \t]*:::[ \t]*([A-Za-z][A-Za-z0-9_-]*)[ \t]*$`)

// pageContainerCloseRegex matches the bare `:::` line that closes a container.
// pageContainerCloseRegex 匹配用于结束容器的单独一行 `:::`。
var pageContainerCloseRegex = regexp.MustCompile(`(?m)^[ \t]*:::[ \t]*$`)

// preprocessPageContainers rewrites the whitelisted `::: name` … `:::` blocks into the comment
// markers that survive Markdown rendering, and returns the text with the close markers in place.
//
// The close marker is emitted for a whitelisted container only. A non-whitelisted opener is left
// untouched, and its `:::` closer is left untouched too — otherwise an unknown name would still
// consume the closer and could unbalance a later container.
//
// preprocessPageContainers 把白名单内的 `::: name` … `:::` 块改写为能穿过 Markdown 渲染的
// 注释标记，并返回带结束标记的文本。
//
// 只有白名单内的容器才会写上结束标记。非白名单的开始行原样不动，它的 `:::` 结束行也原样不动——
// 否则一个未知名字仍会消耗掉那行结束符，并可能让后面的容器错配。
func preprocessPageContainers(md string) string {
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines)+8)
	// openName is the container currently being marked, or "" when outside one. Nesting is not
	// supported: the first closer ends the open container, which is the behaviour a reader of the
	// source would expect from `::: name` … `:::`.
	//
	// openName 是当前正在标记的容器；不在容器内时为空串。不支持嵌套：
	// 第一个结束行就关掉当前容器，这也正是源码读者对 `::: name` … `:::` 的预期。
	openName := ""

	for _, line := range lines {
		if openName == "" {
			if m := pageContainerOpenRegex.FindStringSubmatch(line); m != nil {
				name := m[1]
				if !pageContainerAllowed(name) {
					// Not ours. Emit verbatim so the author sees the name they typed.
					// 不是我们的容器。原样输出，让作者看到自己写的名字。
					out = append(out, line)
					continue
				}
				openName = name
				out = append(out, pageContainerOpenPrefix+name+pageContainerSuffix, "")
				continue
			}
			out = append(out, line)
			continue
		}

		// Inside a container: a bare `:::` closes it.
		if pageContainerCloseRegex.MatchString(line) {
			out = append(out, "", pageContainerClosePrefix+openName+pageContainerSuffix)
			openName = ""
			continue
		}
		out = append(out, line)
	}

	// An unclosed container still gets its close marker, so the rest of the page stays inside a
	// sane layout instead of leaking out of an unclosed element.
	//
	// 未闭合的容器同样补上结束标记，使余下的页面留在一个正常的版式里，
	// 而不是从一个未闭合的元素里漏出去。
	if openName != "" {
		out = append(out, "", pageContainerClosePrefix+openName+pageContainerSuffix)
	}
	return strings.Join(out, "\n")
}

// pageContainerAllowed reports whether name is in the layout whitelist.
// pageContainerAllowed 判断名字是否在排版白名单内。
func pageContainerAllowed(name string) bool {
	for _, n := range pageContainerNames {
		if n == name {
			return true
		}
	}
	return false
}

// pageContainerPairRegex finds a marker pair in rendered HTML so it can be swapped for real tags.
//
// Go's regexp is RE2, which has no backreferences — the obvious `\1` that ties the closing name to
// the opening one does not compile. The names are therefore captured independently and compared in
// the callback, which also lets a mismatched pair fall through to "leave the markers alone"
// instead of silently wrapping one container in another's class.
//
// `(?s)` spans newlines because the rendered block sits between the markers, and the non-greedy
// middle keeps two containers from being treated as one span.
//
// pageContainerPairRegex 在渲染后的 HTML 里找到一对标记，以便替换成真正的标签。
//
// Go 的 regexp 是 RE2，没有反向引用——那个把结束名与开始名绑在一起的 `\1` 根本编译不过。
// 因此两个名字各自捕获、在回调里比较；这也让名字不匹配的一对落到「标记原样保留」，
// 而不是把一个容器静默地包进另一个容器的类名里。
//
// `(?s)` 让匹配跨换行，因为渲染后的内容位于两条标记之间；中间的非贪婪匹配则避免把两个容器
// 当成一段。
var pageContainerPairRegex = regexp.MustCompile(
	`(?s)` + regexp.QuoteMeta(pageContainerOpenPrefix) + `([A-Za-z][A-Za-z0-9_-]*)` + regexp.QuoteMeta(pageContainerSuffix) +
		`(.*?)` +
		regexp.QuoteMeta(pageContainerClosePrefix) + `([A-Za-z][A-Za-z0-9_-]*)` + regexp.QuoteMeta(pageContainerSuffix))

// applyPageContainers swaps the surviving markers for `<div class="pc-NAME">` wrappers.
//
// A marker whose name is no longer whitelisted is left as the comment it is, which renders as
// nothing and is therefore the least noisy failure. That path is unreachable while the two
// functions share one list; it exists so a future narrowing of the list cannot emit a class name
// that no stylesheet defines.
//
// applyPageContainers 把留存的标记换成 `<div class="pc-NAME">` 包裹层。
//
// 名字已不在白名单里的标记会保留为它本身（一条注释），渲染出来是空的，因而是最不吵的失败形态。
// 只要这两个函数共用同一份名单，这条路径就不可达；留着它是为了日后收窄名单时，
// 不会写一个没有任何样式定义的类名。
func applyPageContainers(html string) string {
	if !strings.Contains(html, pageContainerOpenPrefix) {
		return html
	}
	return pageContainerPairRegex.ReplaceAllStringFunc(html, func(match string) string {
		m := pageContainerPairRegex.FindStringSubmatch(match)
		// The two names must agree. a mismatched pair means the source had an unclosed container,
		// and wrapping on a guess would put the content under the wrong layout.
		//
		// 两个名字必须一致。不匹配说明源码里有未闭合的容器，
		// 靠猜去包裹会把内容放到错误的版式下。
		if m == nil || m[1] != m[3] || !pageContainerAllowed(m[1]) {
			return match
		}
		return `<div class="pc pc-` + m[1] + `">` + m[2] + `</div>`
	})
}
