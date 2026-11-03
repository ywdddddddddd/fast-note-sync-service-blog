package service

import (
	"regexp"
	"sort"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// Sections are the top-level folders of the vault, rendered as the reference site renders its
// categories: a dropdown in the top bar, an h1 on the list page, a block in the right rail.
//
// They are derived, never configured. A folder tree is already the author's own taxonomy — asking
// them to maintain a second copy of it in YAML would create two sources of truth for the same
// fact, and the copy in the config would go stale the first time a folder is renamed.
//
// 版块是仓库的顶层目录，渲染方式与参照站的分类一致：顶栏下拉、列表页 h1、右栏一块。
//
// 它们是推导出来的，从不配置。目录树本来就是作者自己的分类法——让他在 YAML 里再维护一份，
// 等于为同一个事实造两个真相源，而配置里那份会在第一次改目录名时过期。
const (
	// blogUncategorizedSection collects notes that sit directly in the vault root. Rendering them
	// under their own label is honest: the alternative is either hiding them or inventing a
	// section name that does not exist in the tree.
	//
	// blogUncategorizedSection 收纳直接放在仓库根部的笔记。为它们单列一个版块是诚实的做法：
	// 另一种选择要么把它们藏起来，要么编一个树里并不存在的版块名。
	blogUncategorizedSection = "未分类"

	// blogRootSectionLabel is what a note with no folder at all is filed under. It is the same
	// label blogNavName already uses for the empty path, so the sidebar and the section list
	// cannot disagree.
	//
	// blogRootSectionLabel 是没有目录的笔记所属的版块名。它与 blogNavName 对空路径的既有
	// 说法一致，使侧栏与版块列表不会互相矛盾。
	blogRootSectionLabel = "根目录"
)

// blogSectionNoise strips the numbering people put in front of folder names.
//
// The vault's top-level folders are literally named "（1）口腔笔记" / "（2）健康生活". That
// numbering is a reading order, not part of the name: it belongs in a README, and in a URL it
// becomes a percent-encoded full-width bracket. Stripping it here — once, in the data layer —
// means the template, the URL and the aria-label all get the same clean string.
//
// The trailing form ("口腔（1）") is stripped too, because a hand-renamed copy of a folder picks
// up the suffix instead of the prefix and the two must collapse to one section.
//
// blogSectionNoise 剥掉人们写在目录名前面的编号。
//
// 仓库的顶层目录字面就叫「（1）口腔笔记」「（2）健康生活」。那个编号是阅读顺序，不是名字的一部分：
// 它属于 README，进了 URL 就变成百分号编码的全角括号。在这里（数据层，只做一次）剥掉它，
// 模板、URL 与 aria-label 就都拿到同一个干净的字符串。
//
// 尾部形态（「口腔（1）」）同样剥掉：手工复制一份目录时拿到的是后缀而不是前缀，
// 而两者必须收敛成同一个版块。
var blogSectionNoise = []*regexp.Regexp{
	regexp.MustCompile(`^\s*[（(]\s*\d+\s*[）)]\s*`),
	regexp.MustCompile(`[（(]\s*\d+\s*[）)]\s*$`),
	regexp.MustCompile(`^\s*\d+\s*[-_.、]\s*`),
}

// blogSectionName normalizes a folder name into a section name.
//
// An empty result falls back to 未分类 rather than being dropped: a folder named "（1）" is a real
// folder with real notes in it, and silently losing it would hide them from every list page.
//
// blogSectionName 把目录名归一化成版块名。
//
// 结果为空时回退到「未分类」而不是丢弃：一个叫「（1）」的目录是真实存在的、装着真笔记的目录，
// 静默丢掉它会让那些笔记从所有列表页消失。
func blogSectionName(raw string) string {
	s := strings.TrimSpace(raw)
	for _, re := range blogSectionNoise {
		s = re.ReplaceAllString(s, "")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return blogUncategorizedSection
	}
	return s
}

// blogEntrySection is the section a note belongs to, derived from its vault-relative path.
//
// The path is the same value the folder tree splits on (see GetBlogHome), so the two views of
// the vault cannot disagree about where a note lives. A note with no folder is filed under the
// root label rather than an empty section, which keeps the section page list closed.
//
// blogEntrySection 是笔记所属的版块，由仓库内相对路径推导。
//
// 这个路径正是目录树切分时用的同一个值（见 GetBlogHome），因此仓库的两种视图不会对
// 「笔记在哪」产生分歧。没有目录的笔记归入「根目录」而不是空版块，使版块清单保持闭合。
func blogEntrySection(path string) string {
	p := strings.ReplaceAll(path, "\\", "/")
	p = strings.TrimPrefix(p, "/")
	if i := strings.Index(p, "/"); i > 0 {
		return blogSectionName(p[:i])
	}
	return blogRootSectionLabel
}

// buildBlogSections turns the top level of the folder tree into sections.
//
// Only folders become sections. A top-level note is not a section — it is one article, and giving
// it a section of its own would produce a dropdown full of single-entry categories.
//
// Ordering is by note count, descending, with natural name order breaking ties: the first entries
// in a navigation menu should be the places with something in them, and a stable tie-break keeps
// the list from reshuffling between requests.
//
// buildBlogSections 把目录树顶层变成版块。
//
// 只有文件夹成为版块。顶层单篇不是版块——它就是一篇文章，给它单开版块会让下拉里塞满
// 只有一条记录的分类。
//
// 排序按笔记数降序，同数时按自然序比较名称：导航菜单的头几项应该是装着东西的地方，
// 而稳定的次级排序让这份清单不会在请求之间重排。
func buildBlogSections(nav []dto.BlogNavNode) []*dto.BlogSectionDTO {
	out := make([]*dto.BlogSectionDTO, 0, len(nav))
	for _, n := range nav {
		if len(n.Children) == 0 {
			continue
		}
		name := blogSectionName(n.Name)
		out = append(out, &dto.BlogSectionDTO{Name: name, Slug: name, Count: n.Count})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return blogNaturalLess(out[i].Name, out[j].Name)
	})
	return out
}

// blogSectionOf returns the section with the given slug, or nil.
//
// The slug is the normalized display name, because that is what the URL carries: a reader who
// shares /blog/c/口腔笔记 is sharing something a human can read back, and percent-encoded
// full-width brackets in a permalink are a defect rather than a detail.
//
// blogSectionOf 按 slug 返回版块，找不到时返回 nil。
//
// slug 就是归一化后的显示名，因为 URL 携带的正是它：分享 /blog/c/口腔笔记 的人在分享一个
// 人能读回来的东西，而永久链接里出现百分号编码的全角括号是缺陷，不是细节。
func blogSectionOf(sections []*dto.BlogSectionDTO, slug string) *dto.BlogSectionDTO {
	for _, s := range sections {
		if s.Slug == slug {
			return s
		}
	}
	return nil
}
