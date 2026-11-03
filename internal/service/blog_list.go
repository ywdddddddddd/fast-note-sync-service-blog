package service

import (
	"strconv"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// blogPageSize is how many cards one list page carries.
//
// 15 is the reference site's number and it is not arbitrary: a card is 196px tall, so 15 cards
// plus the pager is about one and a half screens — long enough that the page feels like an
// archive, short enough that reaching the pager is not a scroll.
//
// blogPageSize 是一页列表承载的卡片数。
//
// 15 是参照站的数字，并非随手取：一张卡片 196px 高，15 张加上分页器约一屏半——
// 长到让页面像一份归档，短到不必滚动很久就够到分页器。
const blogPageSize = 15

// blogPaginate slices entries for one page.
//
// The returned page is clamped into range rather than trusted: a hand-typed /blog/all/999 must
// not panic on a slice bound. Callers that need to answer 404 for an out-of-range page compare
// the requested number against totalPages themselves — clamping here keeps the slicing safe
// without silently redefining "page 999" as "the last page" for the caller that asked.
//
// blogPaginate 为一页切分条目。
//
// 返回的页码会被夹到合法区间，而不是被信任：手敲的 /blog/all/999 不能在切片的边界上 panic。
// 需要对越界页返回 404 的调用方自己把请求页码与 totalPages 比较——在这里夹取只是让切片安全，
// 不会把调用方问的「第 999 页」悄悄改写成「最后一页」。
func blogPaginate(entries []*dto.BlogEntryDTO, page, size int) ([]*dto.BlogEntryDTO, int) {
	if size <= 0 {
		size = blogPageSize
	}
	totalPages := (len(entries) + size - 1) / size
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * size
	if start >= len(entries) {
		return nil, totalPages
	}
	end := start + size
	if end > len(entries) {
		end = len(entries)
	}
	return entries[start:end], totalPages
}

// blogPagerLinks builds the pager.
//
// Page one lives at the bare base path, every later page at base/N. That asymmetry is deliberate
// and matches the reference site: /blog/all is the canonical entry point, and a page-one URL that
// carries a /1 would give the same content two addresses.
//
// Every page number is listed. The corpus produces ten pages at 15 per page, and a windowed pager
// with ellipses would add a template branch, a hidden state and a second URL form to save three
// characters of width. When the archive outgrows that, the windowing belongs here, in one place.
//
// blogPagerLinks 构造分页器。
//
// 第一页在裸路径上，之后的每一页在 base/N。这个不对称是有意的，与参照站一致：
// /blog/all 是规范入口，而带 /1 的首页地址会让同一份内容有两个地址。
//
// 所有页码都列出来。15 篇一页时这份语料正好十页，而带省略号的窗口式分页器会多出一个模板分支、
// 一个隐藏状态和第二种 URL 形态，只为省下三个字符的宽度。等归档长到需要窗口化，那个逻辑属于
// 这里，而且只属于这里一处。
func blogPagerLinks(base string, page, totalPages int) []dto.BlogPageLinkDTO {
	if totalPages <= 1 {
		return nil
	}
	base = strings.TrimSuffix(base, "/")
	out := make([]dto.BlogPageLinkDTO, 0, totalPages)
	for n := 1; n <= totalPages; n++ {
		url := base
		if n > 1 {
			url = base + "/" + strconv.Itoa(n)
		}
		out = append(out, dto.BlogPageLinkDTO{Num: n, URL: url, Current: n == page})
	}
	return out
}

// blogPageURL is the address of one page, used for prev/next.
// blogPageURL 是某一页的地址，供上一页/下一页使用。
func blogPageURL(base string, page int) string {
	base = strings.TrimSuffix(base, "/")
	if page <= 1 {
		return base
	}
	return base + "/" + strconv.Itoa(page)
}

// blogArchiveYears groups entries by the year in their creation date, newest year first.
//
// Entries with no date are kept in a 未知 block instead of being dropped. A note without a
// timestamp is usually an import artefact, and hiding it from the only page that claims to list
// everything would make the archive lie about its own completeness.
//
// blogArchiveYears 按创建日期里的年份给条目分组，年份新的在前。
//
// 没有日期的条目收进「未知」块而不是丢弃。缺时间戳的笔记通常是导入产物，
// 把它从唯一声称「列出全部」的页面上藏起来，会让归档对自己的完整性说谎。
func blogArchiveYears(entries []*dto.BlogEntryDTO) []*dto.BlogArchiveYearDTO {
	const undated = "未知"
	order := make([]string, 0, 8)
	byYear := make(map[string][]*dto.BlogEntryDTO, 8)
	for _, e := range entries {
		year := undated
		if len(e.CreatedAt) >= 4 {
			if y := e.CreatedAt[:4]; y >= "1900" && y <= "2999" {
				year = y
			}
		}
		if _, ok := byYear[year]; !ok {
			order = append(order, year)
		}
		byYear[year] = append(byYear[year], e)
	}
	// Newest year first, but 未知 always last: it is not a year, and sorting it as a string would
	// place it between real years by codepoint accident.
	//
	// 年份新的在前，但「未知」永远在最后：它不是一个年份，
	// 按字符串排序只会因为码位巧合把它塞进真实年份之间。
	out := make([]*dto.BlogArchiveYearDTO, 0, len(order))
	for _, y := range order {
		if y == undated {
			continue
		}
		out = append(out, &dto.BlogArchiveYearDTO{Year: y, Count: len(byYear[y]), Entries: byYear[y]})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Year > out[j-1].Year; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if list, ok := byYear[undated]; ok {
		out = append(out, &dto.BlogArchiveYearDTO{Year: undated, Count: len(list), Entries: list})
	}
	return out
}

// blogFilterSection keeps the entries that live in one section.
// blogFilterSection 保留属于某个版块的条目。
func blogFilterSection(entries []*dto.BlogEntryDTO, section string) []*dto.BlogEntryDTO {
	out := make([]*dto.BlogEntryDTO, 0, len(entries))
	for _, e := range entries {
		if blogEntrySection(e.Path) == section {
			out = append(out, e)
		}
	}
	return out
}
