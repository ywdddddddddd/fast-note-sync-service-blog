package service

import (
	"math/rand/v2"
	"net/url"
	"sort"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// Folders are recommended at the level the reader is currently standing on.
//
// The vault has two useful levels: a section ("口腔笔记", a whole subject) and a sub-section
// ("修复笔记", one course or series). Ranking a fixed level produced a rail that was right on one
// page and wrong on the next — on /blog/all it offered sub-sections the reader had no context for,
// and on a section page it offered the very folders that page was already about. So the level is
// now an argument, not a constant.
//
// 推荐取读者当前所站的那一层。
//
// 仓库有两层有用的结构：版块（「口腔笔记」，一个完整学科）与二级栏目（「修复笔记」，一门课或一个系列）。
// 固定取某一层会造出一条「在这页对、在下一页错」的右栏 —— 在 /blog/all 上它给出读者还没有上下文
// 的二级栏目，而在版块页上它给出的正是那一页本身在讲的文件夹。所以层级现在是参数，不是常量。
const blogRecLimit = 5

// blogSubPageSize is how many notes a sub-section page carries.
//
// Six rather than the list page's fifteen: a sub-section is one course, and a course is read in
// sittings. Six cards is roughly one sitting, while fifteen turns the page into a scroll with no
// sense of having finished anything.
//
// blogSubPageSize 是二级栏目页一页承载的笔记数。
//
// 取六而不是列表页的十五：二级栏目是一门课，而一门课是按「一次坐下」读完的。
// 六张卡大约就是一坐，十五张会把页面变成一个滚不到底的清单。
const blogSubPageSize = 6

// blogSectionBlurbs are hand-written one-liners for the folders that exist today.
//
// They are hard-coded on purpose. The owner asked for fixed copy because folder contents do not
// change often, and a generated summary would drift from the folder the moment it is edited.
// A folder that is not in this map falls back to its own name — see the note in
// blogAggregateSections.
//
// blogSectionBlurbs 是为现有文件夹手写的一句话说明。
//
// 有意写死。站主要求固定文案，因为文件夹内容不常变，而自动生成的摘要会在文件夹被编辑的那一刻
// 开始漂移。不在这张表里的文件夹回退到它自己的名字——理由见 blogAggregateSections。
var blogSectionBlurbs = map[string]string{
	"口腔笔记":      "牙体牙髓、口腔外科与修复的临床记录。",
	"健康生活":      "作息、训练与饮食，以及它们对身体的影响。",
	"能力提升":      "科研、技术与方法论上的长期投入。",
	"修复笔记":      "牙体缺损的修复路径：嵌体、高嵌体、全冠与桩核。",
	"口外笔记":      "口腔颌面外科：麻醉、拔牙、感染与创伤处理。",
	"口组笔记":      "口腔组织病理：牙体与牙周组织的发生、结构与病变。",
	"口解笔记":      "口腔解剖生理：牙体形态、咬合与颌位关系。",
	"正畸笔记":      "错𬌗畸形的诊断、矫治器与力学设计。",
	"牙体牙髓":      "龋病、牙髓病与根尖周病的诊断与治疗。",
	"种植笔记":      "种植体植入、骨结合与上部修复。",
	"牙体笔记":      "牙体形态与牙体预备的基础记录。",
	"彭教授的根管教学":  "根管治疗的临床要点与教学示范。",
	"健身":        "训练计划、动作模式与恢复。",
	"光线":        "自然光与人工光的取舍，以及它对作息的影响。",
	"压力":        "压力的来源、生理反应与调节手段。",
	"睡眠":        "睡眠结构、昼夜节律与常见睡眠问题。",
	"饮食":        "宏量营养素、进食时机与日常搭配。",
	"科研学术":      "论文阅读、研究方法与写作。",
	"技术实践":      "工具、环境与踩坑记录。",
}

// blogEntryFolderAt returns the folder name at the given depth of a note's vault-relative path.
//
// depth 1 is the section, depth 2 the sub-section. An empty result means the note does not reach
// that depth: a note sitting directly in a section has no sub-section, which is why it must not be
// counted as one named "".
//
// The depth-1 case falls back to the root label instead of "", matching blogEntrySection: a note
// with no folder is filed somewhere, not nowhere.
//
// blogEntryFolderAt 返回笔记仓库内相对路径在指定层级上的目录名。
//
// 第 1 层是版块，第 2 层是二级栏目。返回空串表示这条笔记到不了那一层：直接放在版块里的笔记
// 没有二级栏目，因此不能把它算成一个叫空串的栏目。
//
// 第 1 层回退到根目录标签而不是空串，与 blogEntrySection 一致：没有目录的笔记是被归了档，不是没处放。
func blogEntryFolderAt(path string, depth int) string {
	if depth < 1 {
		return ""
	}
	p := strings.TrimPrefix(strings.ReplaceAll(path, "\\", "/"), "/")
	segs := strings.Split(p, "/")
	// The final segment is the note itself, so depth d needs at least d+1 segments.
	//
	// 最后一段是笔记本身，因此第 d 层至少需要 d+1 段。
	if len(segs) < depth+1 {
		if depth == 1 {
			return blogRootSectionLabel
		}
		return ""
	}
	return blogSectionName(segs[depth-1])
}

// blogAggregateSections groups notes by the folder they live in at one depth and ranks the folders.
//
// parent narrows depth 2 to the children of one section; it is ignored at depth 1. limit > 0 asks
// for a recommendation rail, which keeps only folders with at least two notes and truncates the
// result. limit <= 0 asks for the complete set of child folders, which is what a section page
// renders — there, a folder holding a single note still deserves a card, because hiding it would
// make the page silently disagree with the folder tree the reader just came from.
//
// Covers are drawn from the notes inside each folder, one at random per call. A folder has no cover
// of its own, and inventing a placeholder while the author has already chosen real cover art for
// the notes inside would be throwing that art away. The pick is re-rolled on every request, which is
// what the owner asked for: the same folder shows a different cover each time the page is opened.
//
// blogAggregateSections 按笔记在某一层所属的目录分组，并对目录排序。
//
// parent 把第 2 层收窄到某一个版块的子目录；在第 1 层忽略。limit > 0 表示要一条推荐右栏，
// 此时只保留至少两篇笔记的目录并截断结果。limit <= 0 表示要全部子目录，即版块页主体的渲染内容——
// 在那里，只有一篇笔记的目录也该有卡片，因为把它藏起来会让页面悄悄背离读者刚走过来的那棵目录树。
//
// 封面取自各目录内的笔记，每次调用随机一张。目录本身没有封面，而在作者已经为里面的笔记选好真实封面图
// 的情况下另画一个占位，等于把那批图丢掉。每次请求重抽，这也是站主要求的：
// 同一个目录每次打开页面都换一张封面。
func blogAggregateSections(entries []*dto.BlogEntryDTO, parent string, depth, limit int) []*dto.BlogSectionRecDTO {
	if depth < 1 {
		depth = 1
	}
	recommend := limit > 0

	type agg struct {
		name   string
		latest string
		words  int
		count  int
		slug   string
		covers []*dto.BlogCoverDTO
	}
	byKey := make(map[string]*agg, 32)
	order := make([]string, 0, 32)

	for _, e := range entries {
		if e == nil {
			continue
		}
		name := blogEntryFolderAt(e.Path, depth)
		if name == "" {
			continue
		}
		if depth > 1 && parent != "" && blogEntryFolderAt(e.Path, depth-1) != parent {
			continue
		}
		a := byKey[name]
		if a == nil {
			a = &agg{name: name}
			byKey[name] = a
			order = append(order, name)
		}
		a.count++
		a.words += e.Words
		// CreatedAt, not MTime.
		//
		// The list card prints CreatedAt, so ranking by file mtime made the rail contradict the
		// cards: a note dated 11-02 could sit under a folder the rail placed at 10-27. Two places
		// describing the same note with two different dates is not a nuance — it reads as a bug.
		//
		// 用 CreatedAt 而不是 MTime。
		//
		// 列表卡印的是 CreatedAt，按文件 mtime 排序会让右栏和卡片互相矛盾：
		// 一篇标着 11-02 的笔记，可能被归进右栏写着 10-27 的目录。同一个笔记在两个地方
		// 显示两个日期，读起来就是 bug，而不是细节。
		if e.CreatedAt >= a.latest {
			a.latest = e.CreatedAt
			a.slug = e.Slug
		}
		if e.Cover != nil {
			a.covers = append(a.covers, e.Cover)
		}
	}

	out := make([]*dto.BlogSectionRecDTO, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		if recommend && (a.count < 2 || a.slug == "") {
			// A one-note folder is not a section a reader can browse — recommending it produces a
			// link that lands on a single article, which is exactly what the deleted "popular"
			// list did.
			//
			// 只有一篇的文件夹不是读者能浏览的栏目——推荐它只会给出一个落在单篇文章上的链接，
			// 而那正是被删掉的热门列表在做的事。
			continue
		}
		desc, ok := blogSectionBlurbs[a.name]
		if !ok {
			// The owner's instruction: an unknown folder uses its own name as the summary until
			// someone writes a real line for it. It reads redundant, and that is the point — a
			// redundant line is obviously unfinished, whereas a clever generated one hides that.
			//
			// 站主的指示：未收录的文件夹先用它自己的名字当摘要，等人来写真正的一句话。
			// 它读起来是重复的，而这正是重点——重复的一行一眼看得出是待补，
			// 而一句机器生成的漂亮话会把这件事藏起来。
			desc = a.name
		}
		rec := &dto.BlogSectionRecDTO{
			Name:    a.name,
			Desc:    desc,
			Slug:    a.slug,
			Href:    blogFolderHref(parent, a.name),
			Count:   a.count,
			Words:   a.words,
			Updated: a.latest,
		}
		if depth > 1 || parent != "" {
			rec.Parent = parent
		}
		if len(a.covers) > 0 {
			rec.Cover = a.covers[rand.IntN(len(a.covers))]
		} else {
			rec.Initial, rec.InitialHue = blogEntryInitial(a.name)
		}
		out = append(out, rec)
	}

	if recommend {
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Updated != out[j].Updated {
				return out[i].Updated > out[j].Updated
			}
			if out[i].Count != out[j].Count {
				return out[i].Count > out[j].Count
			}
			return blogNaturalLess(out[i].Name, out[j].Name)
		})
		if len(out) > limit {
			out = out[:limit]
		}
		return out
	}

	// A full child-folder list is sorted by name, not by recency.
	//
	// Recency ordering is right for a five-item rail and wrong for a page of forty folders: every
	// new note would move a folder to the top, and a reader who learned "修复笔记 is third" would
	// find it somewhere else next week. A section page is a table of contents, and a table of
	// contents that rearranges itself is not one.
	//
	// 完整的子目录清单按名称排，不按时间。
	//
	// 时间序适合五个条目的右栏，不适合一页四十个目录：每来一篇新笔记就会把某个目录顶到最前，
	// 而记住「修复笔记排第三」的读者下周会发现它在别处。版块页是一张目录，
	// 一张会自己重排的目录不是目录。
	sort.SliceStable(out, func(i, j int) bool { return blogNaturalLess(out[i].Name, out[j].Name) })
	return out
}

// blogFolderHref is the page for one folder. A top-level folder has its own section page; a
// second-level folder lives under its parent's.
//
// blogFolderHref 是一个文件夹的页面地址。一级文件夹有自己的版块页，二级文件夹住在其父级之下。
func blogFolderHref(parent, name string) string {
	if parent == "" {
		return "/blog/c/" + url.PathEscape(name)
	}
	return "/blog/c/" + url.PathEscape(parent) + "/" + url.PathEscape(name)
}

// blogFilterSubSection keeps the notes that live in one sub-section.
// blogFilterSubSection 保留属于某个二级栏目的笔记。
func blogFilterSubSection(entries []*dto.BlogEntryDTO, section, sub string) []*dto.BlogEntryDTO {
	out := make([]*dto.BlogEntryDTO, 0, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		if blogEntryFolderAt(e.Path, 1) != section {
			continue
		}
		if sub != "" && blogEntryFolderAt(e.Path, 2) != sub {
			continue
		}
		out = append(out, e)
	}
	return out
}

// blogSubSectionOf reports whether a sub-section exists under a section.
// blogSubSectionOf 报告某个二级栏目是否存在于该版块之下。
func blogSubSectionOf(entries []*dto.BlogEntryDTO, section, sub string) bool {
	for _, e := range entries {
		if e == nil {
			continue
		}
		if blogEntryFolderAt(e.Path, 1) == section && blogEntryFolderAt(e.Path, 2) == sub {
			return true
		}
	}
	return false
}
