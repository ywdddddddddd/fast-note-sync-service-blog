package service

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// Notes inside a folder are ordered by their own heading number, not by when they were created.
//
// Creation order is the wrong order for a course. "第6章" was written before "第1章" because the
// author jumped to the interesting part first, so a folder read in creation order opens on chapter
// six and wanders. A folder of chapters is a table of contents; a table of contents that does not
// count upwards is not one.
//
// The heading number wins, but the number can be written four ways in this vault, so all four are
// reduced to the same shape before comparing:
//
//	第6章：口腔颌面部感染      Arabic after a Chinese classifier
//	第一章 绪论                Chinese numerals
//	Chapter 3 / Ch.3          English word
//	3. 概述                    Bare leading number
//
// A note with no heading number sorts after every numbered one, by name. It has no place in the
// sequence, so putting it at the end keeps the sequence intact instead of interleaving a stray
// item that happens to start with a letter early in the alphabet.
//
// 文件夹里的笔记按它自己的标题编号排，而不是按创建时间。
//
// 对一门课来说创建顺序就是错的顺序。「第6章」写在「第1章」之前，是因为作者先跳去写了最有意思
// 的那部分，于是按创建顺序读的文件夹会从第六章开始、然后到处乱走。一个装着章节的文件夹是一张
// 目录，而一张不递增的目录不是目录。
//
// 标题编号优先，但这个仓库里同一个编号有四种写法，所以比较之前先把四种归成同一个形状：
//
//	第6章：口腔颌面部感染      中文量词后接阿拉伯数字
//	第一章 绪论                中文数字
//	Chapter 3 / Ch.3          英文词
//	3. 概述                    裸的前导数字
//
// 没有标题编号的笔记排在所有有编号的之后，按名称排。它不属于那个序列，放到末尾能保住序列的
// 完整，而不是把一个恰好以字母表靠前字母开头的东西插进中间。

// blogCNNumber maps the numerals used inside Chinese headings.
// blogCNNumber 是中文标题里会用到的数字字符。
var blogCNNumber = map[rune]int{
	'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4,
	'五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
}

// blogCNUnit is the place value carried by 十 / 百 / 千.
// blogCNUnit 是十／百／千所带的位值。
var blogCNUnit = map[rune]int{'十': 10, '百': 100, '千': 1000}

// blogTitleLead matches a heading number at the start of a title, in any of the four notations.
//
// Each alternative captures a number in group 1. Chinese numerals are matched by character class
// rather than enumerated, so 二十五 and 十一 both come through as one run.
//
// blogTitleLead 匹配标题开头的编号，四种写法都认。
//
// 每个分支都在第 1 组里捕获一个数字。中文数字按字符类匹配而不是逐一枚举，
// 因此「二十五」和「十一」都会整体作为一段被取到。
var blogTitleLead = []*regexp.Regexp{
	// 第6章 / 第6节 / 第十二回 —— the classifier may be 章/节/讲/回/卷/篇/课
	regexp.MustCompile(`^第\s*([0-9]+|[零〇一二两三四五六七八九十百千]+)\s*[章节讲回卷篇课]`),
	// 一、绪论 / 1、概述 / 3. 标题 / 3 标题 —— bare leading number with a separator
	regexp.MustCompile(`^([0-9]+|[零〇一二两三四五六七八九十百千]+)\s*[、.．,，)）]\s*`),
	// Chapter 3 / Ch.3 / Part 2
	regexp.MustCompile(`(?i)^(?:chapter|chap|ch|part|lesson|unit)\s*\.?\s*([0-9]+)`),
	// 3 标题（纯数字开头，后面是空格）
	regexp.MustCompile(`^([0-9]+)\s+\S`),
}

// blogTitleTrail matches a secondary number right after the primary one: 第6章-2 / 第6章（2）.
// blogTitleTrail 匹配紧跟在主编号之后的次编号：第6章-2 / 第6章（2）。
var blogTitleTrail = regexp.MustCompile(`^[\s]*[-－—–.．（(]\s*([0-9]+)\s*[)）]?`)

// blogCNToInt converts a run of Chinese numerals to an integer. ok is false when the run is empty
// or contains a character this converter does not know, which keeps an odd heading from silently
// sorting as zero.
//
// Handles the forms that actually appear in headings: 一 through 九十九, plus 百 and 千 compounds
// like 一百二十三. The algorithm is the standard one — a digit before a unit multiplies it, a digit
// with no following unit is the tail.
//
// blogCNToInt 把一段中文数字转成整数。该段为空或含未知字符时 ok 为 false，
// 以免一个奇怪的标题被悄悄当成零来排序。
//
// 覆盖标题里真实出现的写法：一 到 九十九，以及 一百二十三 这类带百／千的复合数。
// 算法是标准做法——单位前面的数字与单位相乘，最后没有单位的数字是尾数。
func blogCNToInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	total, section, seen := 0, 0, false
	for _, r := range s {
		if d, ok := blogCNNumber[r]; ok {
			section = d
			seen = true
			continue
		}
		if u, ok := blogCNUnit[r]; ok {
			if section == 0 {
				// 十一 / 十二 —— a leading 十 means ten, not zero tens.
				// 十一 / 十二 —— 前导的十表示十，而不是零个十。
				section = 1
			}
			total += section * u
			section = 0
			seen = true
			continue
		}
		return 0, false
	}
	if !seen {
		return 0, false
	}
	return total + section, true
}

// blogTitleNumber pulls the heading number out of a title.
//
// It returns the primary number, an optional secondary number (0 when absent), and whether a
// primary number was found at all.
//
// blogTitleNumber 从标题里取出编号。
//
// 返回主编号、可选的次编号（没有则为 0），以及是否找到了主编号。
func blogTitleNumber(title string) (primary, secondary int, ok bool) {
	t := strings.TrimSpace(title)
	if t == "" {
		return 0, 0, false
	}
	for _, re := range blogTitleLead {
		m := re.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		primary, ok = blogParseHeadingNumber(m[1])
		if !ok {
			continue
		}
		if trail := blogTitleTrail.FindStringSubmatch(t[len(m[0]):]); trail != nil {
			if n, ok2 := blogParseHeadingNumber(trail[1]); ok2 {
				secondary = n
			}
		}
		return primary, secondary, true
	}
	return 0, 0, false
}

// blogParseHeadingNumber reads either an Arabic or a Chinese numeral.
// blogParseHeadingNumber 读取阿拉伯数字或中文数字。
func blogParseHeadingNumber(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	return blogCNToInt(s)
}

// blogTitleOrderKey builds a comparison key that sorts numbered headings in counting order.
//
// Numbers are zero-padded to six digits so plain string comparison is numeric comparison, which
// keeps the key inspectable (a struct comparator would hide the same information behind a longer
// function). Unnumbered titles get 999999 in both slots so they land after everything numbered.
//
// blogTitleOrderKey 生成按计数顺序排列标题的比较键。
//
// 数字补零到六位，使纯字符串比较等价于数值比较，同时让键本身可读
// （换成结构体比较器会把同样的信息藏进一个更长的函数里）。
// 没有编号的标题两个槽位都给 999999，于是它们落在所有有编号的之后。
func blogTitleOrderKey(title string) string {
	const unnumbered = "999999"
	primary, secondary, ok := blogTitleNumber(title)
	if !ok {
		return unnumbered + "\x00" + unnumbered + "\x00" + title
	}
	return padOrderNumber(primary) + "\x00" + padOrderNumber(secondary) + "\x00" + title
}

// padOrderNumber is %06d without the allocation of fmt for a value that is already an int.
// padOrderNumber 是 %06d，只是省掉 fmt 那一次分配。
func padOrderNumber(n int) string {
	if n < 0 {
		n = 0
	}
	s := strconv.Itoa(n)
	if len(s) >= 6 {
		return s
	}
	return strings.Repeat("0", 6-len(s)) + s
}

// blogSortByHeading orders a note list by heading number, then by title.
//
// Ties on the same primary number are broken by the secondary one first (第6章-1 before 第6章-2),
// then lexicographically — but lexicographically with numeric awareness, so 第10章 still follows
// 第9章 rather than 第1章.
//
// blogSortByHeading 按标题编号、再按标题给笔记清单排序。
//
// 主编号相同的先按次编号拆开（第6章-1 在 第6章-2 之前），再按字典序——
// 但那种带数字意识的字典序，所以 第10章 仍然跟在 第9章 后面，而不是 第1章。
func blogSortByHeading(entries []*dto.BlogEntryDTO) {
	if len(entries) < 2 {
		return
	}
	keys := make(map[*dto.BlogEntryDTO]string, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		// Title first: it is what the reader sees. Path is the fallback for a note whose title is
		// empty, which happens when a file has no heading and no filename worth showing.
		//
		// 优先用 Title：那是读者看到的东西。标题为空的笔记（既没有标题也没有像样的文件名）用 Path 兜底。
		label := e.Title
		if strings.TrimSpace(label) == "" {
			label = e.Path
		}
		keys[e] = blogTitleOrderKey(label)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		ki, kj := keys[entries[i]], keys[entries[j]]
		if ki != kj {
			return ki < kj
		}
		return blogNaturalLess(entries[i].Title, entries[j].Title)
	})
}
