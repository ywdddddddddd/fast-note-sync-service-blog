package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// ErrBlogSectionNotFound is returned when a requested section or sub-section does not exist.
//
// It is a sentinel rather than an empty list because the two cases need different answers: an
// empty section page is a 200 with an empty state, while an unknown section name is a 404 — the
// reader typed or pasted a URL that never described anything.
//
// ErrBlogSectionNotFound 在请求的版块或二级栏目不存在时返回。
//
// 它是哨兵错误而不是空清单，因为两种情况需要不同的答案：空版块页是 200 + 空状态，
// 而未知版块名是 404——读者输入或粘贴了一条从未描述任何东西的 URL。
var ErrBlogSectionNotFound = errors.New("blog: section not found")

// GetBlogList assembles one page of /blog/all, /blog/c/<section> or /blog/c/<section>/<sub>.
//
// One function serves all three because they differ only in which set they take and which card
// shape they use. Splitting them would duplicate the rail assembly, and the rail is exactly the
// part that had already drifted between pages once.
//
// The three levels read top-down:
//
//	/blog/all                  note list, 15/page, rail = first-level sections
//	/blog/c/<section>          sub-section cards, rail = that section's sub-sections
//	/blog/c/<section>/<sub>    note list, 6/page, no rail
//
// A section whose notes all sit directly inside it has no sub-sections to draw, so it falls back
// to the note list rather than rendering an empty page. Falling back is the honest answer there:
// the reader asked for that section's content and the content is the notes.
//
// GetBlogList 组装 /blog/all、/blog/c/<版块> 或 /blog/c/<版块>/<二级> 的一页。
//
// 一个函数服务三层，因为它们的差别只在于取哪一批、用哪种卡片。拆开会复制一遍右栏组装，
// 而右栏正是曾经在页面之间发生过漂移的那部分。
//
// 三层自上而下：
//
//	/blog/all                  笔记清单，15 篇/页，右栏 = 一级版块
//	/blog/c/<版块>             二级栏目卡，右栏 = 该版块的二级栏目
//	/blog/c/<版块>/<二级>      笔记清单，6 篇/页，无右栏
//
// 一个「笔记全都直接放在版块里」的版块没有可画的二级栏目，于是回退到笔记清单，
// 而不是渲染一个空页。回退在这里是诚实的答案：读者要的是那个版块的内容，而内容就是那些笔记。
func (s *shareService) GetBlogList(ctx context.Context, uid int64, baseURL string, publishedOnly bool, section, sub string, page int) (*dto.BlogListDTO, error) {
	home, err := s.GetBlogHome(ctx, uid, baseURL, publishedOnly, 0)
	if err != nil {
		return nil, err
	}

	root := strings.TrimRight(baseURL, "/")
	entries := home.Entries
	kicker := "全部笔记"
	basePath := root + "/blog/all"
	size := blogPageSize
	var subs, recs []*dto.BlogSectionRecDTO

	switch {
	case section == "":
		// /blog/all — everything, newest first, recommending the top level of the taxonomy.
		recs = blogAggregateSections(home.Entries, "", 1, blogRecLimit)

	case sub == "":
		// /blog/c/<section> — a table of contents for one section.
		sec := blogSectionOf(home.Sections, section)
		if sec == nil {
			return nil, ErrBlogSectionNotFound
		}
		kicker = sec.Name
		basePath = root + "/blog/c/" + url.PathEscape(section)
		inSection := blogFilterSection(home.Entries, section)
		subs = blogAggregateSections(home.Entries, section, 2, 0)
		recs = blogAggregateSections(home.Entries, section, 2, blogRecLimit)
		if len(subs) == 0 {
			// No second level under this section: show its notes instead of an empty shelf.
			subs = nil
			entries = inSection
			break
		}
		// The body is the folder cards, so there are no note entries to slice. Total still counts
		// notes, because that is the number the reader is actually asking about.
		entries = nil

	default:
		// /blog/c/<section>/<sub> — the notes of one sub-section.
		sec := blogSectionOf(home.Sections, section)
		if sec == nil || !blogSubSectionOf(home.Entries, section, sub) {
			return nil, ErrBlogSectionNotFound
		}
		kicker = sub
		basePath = root + "/blog/c/" + url.PathEscape(section) + "/" + url.PathEscape(sub)
		entries = blogFilterSubSection(home.Entries, section, sub)
		// A sub-section is the last level, so its notes are a sequence rather than a feed: order
		// them by their own heading number. /blog/all keeps newest-first because "everything" has
		// no sequence to preserve — its only useful order is recency.
		//
		// 二级栏目是最后一层，因此它的笔记是一个序列而不是一条信息流：按标题编号排。
		// /blog/all 保持最新在前，因为「全部」没有序列要保住——它唯一有用的顺序就是新旧。
		blogSortByHeading(entries)
		size = blogSubPageSize
		// No rail here: the reader has already narrowed to one course, and a list of sibling
		// courses beside it would only invite them to leave.
		recs = nil
	}

	total := len(entries)
	if subs != nil {
		total = len(blogFilterSection(home.Entries, section))
	}
	pageEntries, totalPages := blogPaginate(entries, page, size)

	prevURL, nextURL := "", ""
	if page > 1 && page <= totalPages {
		prevURL = blogPageURL(basePath, page-1)
	}
	if page < totalPages {
		nextURL = blogPageURL(basePath, page+1)
	}

	return &dto.BlogListDTO{
		Title:      home.Title,
		Kicker:     kicker,
		Section:    section,
		Sub:        sub,
		Entries:    pageEntries,
		Subs:       subs,
		Total:      total,
		Sections:   home.Sections,
		Hot:        nil, // filled by the router from the reaction tallies // 由路由层从反应计数填充
		Page:       page,
		TotalPages: totalPages,
		Pages:      blogPagerLinks(basePath, page, totalPages),
		PrevURL:    prevURL,
		NextURL:    nextURL,
		AllEntries: entries,
		Recs:       recs,
	}, nil
}

// GetBlogArchives returns every published note grouped by year.
//
// The archive is deliberately unpaged: its whole point is that a reader can scroll the entire
// history in one gesture, and a pager would break exactly that. The entries carry no summary and
// no cover — the archive is a list of dates and titles, which is why 146 notes still fit.
//
// GetBlogArchives 返回按年份分组的全部已发布笔记。
//
// 归档有意不分页：它的全部意义在于读者可以一次手势滚完整个历史，而分页器恰好会破坏这一点。
// 条目不带摘要也不带封面——归档是一份日期与标题的清单，这也是 146 篇仍然放得下的原因。
func (s *shareService) GetBlogArchives(ctx context.Context, uid int64, baseURL string, publishedOnly bool) ([]*dto.BlogArchiveYearDTO, int, error) {
	home, err := s.GetBlogHome(ctx, uid, baseURL, publishedOnly, 0)
	if err != nil {
		return nil, 0, err
	}
	return blogArchiveYears(home.Entries), len(home.Entries), nil
}
