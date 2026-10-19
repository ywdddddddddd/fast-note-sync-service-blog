package routers

import (
	"sort"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// blogHotLimit is how many articles the popular list shows.
//
// Five, as asked. It is a constant rather than a setting because the list sits in a sidebar card
// under an explanatory paragraph: a longer list would push the footer past the fold on a laptop,
// and there is nothing to tune.
//
// blogHotLimit 是热门列表显示的条数。
//
// 按用户要求取五条。它是常量而不是配置项，因为这个列表位于侧栏的一张卡片里、在说明文字之下：
// 更长的列表会把页脚推到笔记本屏幕的折线以下，而这里也没有什么可调的。
const blogHotLimit = 5

// blogHotList builds the popular list from the reaction tallies.
//
// Three rules, each of which is a decision rather than an implementation detail:
//
//  1. Only published articles. `entries` is the published set, so intersecting with it means a
//     note whose share was cancelled drops off the list. Showing a title whose link now 404s
//     would be worse than showing nothing.
//  2. At least one like. An article with no likes is not "unpopular", it is unrated, and a list
//     padded with zeros would rank by nothing at all.
//  3. Ties break toward the newer article, which is what the user asked for. "Newer" is decided
//     by the order of `entries`, which the source list already sorts by share creation time
//     (newest first); using a slice index keeps this function free of a second time source that
//     could disagree with the page's own ordering.
//
// The tally map is keyed by slug, and so is every entry, so the join is direct.
//
// blogHotList 依据反应计数构建热门列表。
//
// 三条规则，每一条都是决定而不是实现细节：
//
//  1. 只列已发布的文章。`entries` 就是已发布集合，与它取交集意味着被取消分享的笔记会掉出榜单。
//     展示一个链接已经 404 的标题，比什么都不展示更糟。
//  2. 至少有一个赞。没有赞的文章不是「不受欢迎」，而是「尚未被评价」，
//     而一个用零填充的榜单等于什么也没排。
//  3. 并列时新的在前，这是用户要求的。「新」由 `entries` 的顺序决定——
//     来源清单本就按分享创建时间排序（新的在前）；用切片下标可以让这个函数不必引入第二个时间来源，
//     否则它可能与页面自身的顺序不一致。
//
// 计数表与每个条目都以 slug 为键，因此连接是直接的。
func blogHotList(reactions *dto.BlogReactionsDTO, entries []*dto.BlogEntryDTO, limit int) []*dto.BlogHotItemDTO {
	if reactions == nil || len(reactions.Notes) == 0 || len(entries) == 0 || limit <= 0 {
		return nil
	}
	out := make([]*dto.BlogHotItemDTO, 0, limit)
	for _, e := range entries {
		if e == nil || e.Slug == "" {
			continue
		}
		tally := reactions.Notes[e.Slug]
		if tally == nil || tally.Like <= 0 {
			continue
		}
		out = append(out, &dto.BlogHotItemDTO{Slug: e.Slug, Title: e.Title, Like: tally.Like})
	}
	if len(out) == 0 {
		// Nil rather than an empty slice: the template tests {{if .Hot}}, and both are falsy
		// there, but nil also lets a JSON client tell "no popular articles" from "the field was
		// never populated".
		//
		// 返回 nil 而不是空切片：模板用 {{if .Hot}} 判断，两者在那里都是假，
		// 但 nil 还能让 JSON 客户端分辨「没有热门文章」与「字段从未被填充」。
		return nil
	}
	// Stable sort so the input order (newest first) survives equal like counts.
	// 稳定排序，使赞数相同时保留输入顺序（新的在前）。
	sort.SliceStable(out, func(i, j int) bool { return out[i].Like > out[j].Like })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
