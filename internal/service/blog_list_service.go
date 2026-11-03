package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// ErrBlogSectionNotFound is returned when a requested section does not exist.
//
// It is a sentinel rather than an empty list because the two cases need different answers: an
// empty section page is a 200 with an empty state, while an unknown section name is a 404 — the
// reader typed or pasted a URL that never described anything.
//
// ErrBlogSectionNotFound 在请求的版块不存在时返回。
//
// 它是哨兵错误而不是空清单，因为两种情况需要不同的答案：空版块页是 200 + 空状态，
// 而未知版块名是 404——读者输入或粘贴了一条从未描述任何东西的 URL。
var ErrBlogSectionNotFound = errors.New("blog: section not found")

// GetBlogList assembles one page of /blog/all or /blog/c/<section>.
//
// It reuses GetBlogHome rather than re-reading the vault: the home assembly already resolves every
// published note, its cover, its word count and its link tally, and a second query path would be a
// second place for "how many notes are published" to be computed. The graph layout is the one
// expensive step that is skipped here, by passing maxNodes = 0.
//
// GetBlogList 组装 /blog/all 或 /blog/c/<版块> 的一页。
//
// 它复用 GetBlogHome 而不是重新读一遍仓库：首页组装已经解析了每一篇已发布笔记及其封面、字数、
// 链接计数，而第二条查询路径会成为「有多少篇已发布」的第二个计算处。
// 唯一昂贵的一步是图谱布局，这里通过传 maxNodes = 0 跳过它。
func (s *shareService) GetBlogList(ctx context.Context, uid int64, baseURL string, publishedOnly bool, section string, page int) (*dto.BlogListDTO, error) {
	home, err := s.GetBlogHome(ctx, uid, baseURL, publishedOnly, 0)
	if err != nil {
		return nil, err
	}

	entries := home.Entries
	kicker := "全部笔记"
	basePath := strings.TrimRight(baseURL, "/") + "/blog/all"

	if section != "" {
		sec := blogSectionOf(home.Sections, section)
		if sec == nil {
			return nil, ErrBlogSectionNotFound
		}
		entries = blogFilterSection(entries, section)
		kicker = sec.Name
		basePath = strings.TrimRight(baseURL, "/") + "/blog/c/" + url.PathEscape(section)
	}

	total := len(entries)
	pageEntries, totalPages := blogPaginate(entries, page, blogPageSize)

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
		Entries:    pageEntries,
		Total:      total,
		Sections:   home.Sections,
		Hot:        nil, // filled by the router from the reaction tallies // 由路由层从反应计数填充
		Page:       page,
		TotalPages: totalPages,
		Pages:      blogPagerLinks(basePath, page, totalPages),
		PrevURL:    prevURL,
		NextURL:    nextURL,
		AllEntries: entries,
		Recs:       home.Recs,
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
