package service

import (
	"sort"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// Recommendations are second-level folders, ranked by how recently something in them changed.
//
// The previous block ranked articles by likes. That ranked nothing useful: the all-time leader is
// whichever note happened to be shared to a group chat, and it never changes. Recent movement in a
// folder is the one signal a reader can act on — it says "this part of the vault is alive".
//
// 推荐是二级文件夹，按其中最近一次变更的时间排序。
//
// 上一版按点赞数排文章。那没有排出任何有用的东西：榜首永远是某篇恰好被转发到群里的笔记，
// 而且它不再变化。一个文件夹最近的变动，才是读者能据以行动的信号——它说明「仓库的这一块还活着」。
//
// Second level rather than first: a first-level folder here is a whole subject ("口腔笔记", 100
// notes), which is a section, not a recommendation. The second level is a course or a series —
// "修复笔记", 8 notes — which is something a reader picks up and reads.
//
// 取第二层而不是第一层：这里的第一层是一个完整学科（「口腔笔记」，100 篇），那是版块而不是推荐。
// 第二层是一门课或一个系列（「修复笔记」，8 篇），那是读者会拿起来读的东西。
const blogRecLimit = 5

// blogSectionBlurbs are hand-written one-liners for the folders that exist today.
//
// They are hard-coded on purpose. The owner asked for fixed copy because folder contents do not
// change often, and a generated summary would drift from the folder the moment it is edited.
// A folder that is not in this map falls back to its own name — see the note in blogSectionRecs.
//
// blogSectionBlurbs 是为现有文件夹手写的一句话说明。
//
// 有意写死。站主要求固定文案，因为文件夹内容不常变，而自动生成的摘要会在文件夹被编辑的那一刻
// 开始漂移。不在这张表里的文件夹回退到它自己的名字——理由见 blogSectionRecs。
var blogSectionBlurbs = map[string]string{
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

// blogSectionRecs aggregates second-level folders and returns the recently-changed ones.
//
// A folder needs at least two published notes to be a candidate. A one-note folder is not a
// section a reader can browse — recommending it produces a link that lands on a single article,
// which is exactly what the deleted "popular" list did.
//
// 聚合二级文件夹并返回最近有变更的那些。
//
// 一个文件夹至少要有两篇已发布笔记才有资格。只有一篇的文件夹不是读者能浏览的栏目——
// 推荐它只会给出一个落在单篇文章上的链接，而那正是被删掉的热门列表在做的事。
func blogSectionRecs(sources []*blogHomeSource, limit int) []*dto.BlogSectionRecDTO {
	if limit <= 0 {
		limit = blogRecLimit
	}
	type agg struct {
		name   string
		parent string
		latest string
		words  int
		count  int
		slug   string
	}
	byKey := make(map[string]*agg, 32)
	order := make([]string, 0, 32)

	for _, src := range sources {
		if src == nil {
			continue
		}
		segs := strings.Split(strings.ReplaceAll(src.Path, "\\", "/"), "/")
		// Need folder/child/note: a note directly inside a first-level folder has len 2 and is
		// not part of a second-level section.
		//
		// 需要 一级/二级/笔记：直接放在一级目录下的笔记长度为 2，不属于任何二级栏目。
		if len(segs) < 3 {
			continue
		}
		parent := blogSectionName(segs[0])
		name := blogSectionName(segs[1])
		key := parent + "\x00" + name
		a := byKey[key]
		if a == nil {
			a = &agg{name: name, parent: parent}
			byKey[key] = a
			order = append(order, key)
		}
		a.count++
		a.words += src.Words
		// CreatedAt, not MTime.
		//
		// The list card prints CreatedAt, so ranking by file mtime made the rail contradict the
		// cards: a note dated 11-02 could sit under a folder the rail placed at 10-27. Two places
		// describing the same note with two different dates is not a nuance — it reads as a bug.
		// The rail now uses the same column the card shows.
		//
		// 用 CreatedAt 而不是 MTime。
		//
		// 列表卡印的是 CreatedAt，按文件 mtime 排序会让右栏和卡片互相矛盾：
		// 一篇标着 11-02 的笔记，可能被归进右栏写着 10-27 的目录。同一个笔记在两个地方
		// 显示两个日期，读起来就是 bug，而不是细节。右栏现在用卡片展示的同一列。
		if src.CreatedAt >= a.latest {
			a.latest = src.CreatedAt
			a.slug = src.Slug
		}
	}

	out := make([]*dto.BlogSectionRecDTO, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		if a.count < 2 || a.slug == "" {
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
		out = append(out, &dto.BlogSectionRecDTO{
			Name:    a.name,
			Parent:  a.parent,
			Desc:    desc,
			Slug:    a.slug,
			Count:   a.count,
			Words:   a.words,
			Updated: a.latest,
		})
	}

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
