package service

import (
	"strings"
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// TestBlogSectionName pins the folder-name normalization.
//
// The input strings are the vault's real folder names, not invented ones: "（1）口腔笔记" is what
// the tree actually contains, and the trailing-number and dash forms are what a hand-renamed copy
// looks like. A normalization that only handles the tidy case would pass an invented test and
// fail on the data.
func TestBlogSectionName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"（1）口腔笔记", "口腔笔记"},
		{"（2）健康生活", "健康生活"},
		{"(3)能力提升", "能力提升"},
		{"口腔（1）", "口腔"},
		{"3-牙周", "牙周"},
		{"  牙体牙髓  ", "牙体牙髓"},
		{"（1）", blogUncategorizedSection},
		{"", blogUncategorizedSection},
	}
	for _, c := range cases {
		if got := blogSectionName(c.in); got != c.want {
			t.Errorf("blogSectionName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBlogEntrySection pins where a note is filed.
//
// The first path segment is the section by construction — GetBlogHome splits the same value to
// build the tree, so a note cannot sit in a folder and be listed under a different section.
func TestBlogEntrySection(t *testing.T) {
	cases := []struct{ in, want string }{
		{"（1）口腔笔记/修复笔记/嵌体.md", "口腔笔记"},
		{"（2）健康生活/睡眠/昼夜节律.md", "健康生活"},
		{`（3）能力提升\写作\卡片.md`, "能力提升"},
		{"随手记.md", blogRootSectionLabel},
		{"/（1）口腔笔记/a.md", "口腔笔记"},
	}
	for _, c := range cases {
		if got := blogEntrySection(c.in); got != c.want {
			t.Errorf("blogEntrySection(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBuildBlogSections rejects the two mistakes that would be invisible in a screenshot: counting
// a top-level note as a section, and ordering by name instead of by size.
func TestBuildBlogSections(t *testing.T) {
	nav := []dto.BlogNavNode{
		{Name: "（1）口腔笔记", Count: 40, Children: []dto.BlogNavNode{{Name: "修复笔记", Count: 20}}},
		{Name: "（2）健康生活", Count: 12, Children: []dto.BlogNavNode{{Name: "睡眠", Count: 12}}},
		{Name: "（3）能力提升", Count: 30, Children: []dto.BlogNavNode{{Name: "写作", Count: 30}}},
		{Name: "顶层单篇", Slug: "lonely", Count: 1},
	}
	got := buildBlogSections(nav)
	if len(got) != 3 {
		t.Fatalf("want 3 sections (top-level note excluded), got %d", len(got))
	}
	want := []string{"口腔笔记", "能力提升", "健康生活"}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("section[%d] = %q, want %q (order must be by count desc)", i, got[i].Name, w)
		}
		if got[i].Slug != w {
			t.Errorf("section[%d].Slug = %q, want %q", i, got[i].Slug, w)
		}
	}
}

// TestBlogPaginate covers the boundaries the router relies on: exact multiple, partial last page,
// empty set, and a hand-typed page far out of range.
func TestBlogPaginate(t *testing.T) {
	mk := func(n int) []*dto.BlogEntryDTO {
		out := make([]*dto.BlogEntryDTO, n)
		for i := range out {
			out[i] = &dto.BlogEntryDTO{Slug: "s"}
		}
		return out
	}
	cases := []struct {
		n, page         int
		wantLen, wantPg int
	}{
		{146, 1, 15, 10},
		{146, 10, 11, 10},
		{146, 11, 11, 10}, // clamped: 11 is past the end
		{150, 10, 15, 10},
		{0, 1, 0, 1},
		{3, 1, 3, 1},
		{0, 7, 0, 1},
	}
	for _, c := range cases {
		got, pages := blogPaginate(mk(c.n), c.page, blogPageSize)
		if len(got) != c.wantLen || pages != c.wantPg {
			t.Errorf("paginate(n=%d,page=%d) = (len %d, pages %d), want (%d, %d)",
				c.n, c.page, len(got), pages, c.wantLen, c.wantPg)
		}
	}
}

// TestBlogPagerLinks pins the URL asymmetry: page one is the bare path.
func TestBlogPagerLinks(t *testing.T) {
	links := blogPagerLinks("/blog/all", 1, 3)
	if links == nil {
		t.Fatal("want pager for 3 pages, got nil")
	}
	if links[0].URL != "/blog/all" {
		t.Errorf("page 1 URL = %q, want %q", links[0].URL, "/blog/all")
	}
	if links[1].URL != "/blog/all/2" {
		t.Errorf("page 2 URL = %q, want %q", links[1].URL, "/blog/all/2")
	}
	if !links[0].Current || links[1].Current {
		t.Error("current flag landed on the wrong page")
	}
	if got := blogPagerLinks("/blog/all", 1, 1); got != nil {
		t.Errorf("single page must produce no pager, got %d links", len(got))
	}
}

// TestBlogArchiveYears pins ordering: newest year first, undated last, and no silent drop.
func TestBlogArchiveYears(t *testing.T) {
	entries := []*dto.BlogEntryDTO{
		{CreatedAt: "2024-06-01"},
		{CreatedAt: "2026-10-14"},
		{CreatedAt: "2025-01-02"},
		{CreatedAt: ""},
		{CreatedAt: "2026-03-03"},
	}
	years := blogArchiveYears(entries)
	if len(years) != 4 {
		t.Fatalf("want 4 year blocks (incl. 未知), got %d", len(years))
	}
	if years[0].Year != "2026" || years[0].Count != 2 {
		t.Errorf("first block = %s/%d, want 2026/2", years[0].Year, years[0].Count)
	}
	if years[1].Year != "2025" || years[2].Year != "2024" {
		t.Errorf("year order = %s, %s, want 2025, 2024", years[1].Year, years[2].Year)
	}
	if years[3].Year != "未知" {
		t.Errorf("last block = %q, want 未知", years[3].Year)
	}
}

// TestParseBlogProfile uses the résumé note verbatim.
//
// This is the one test that guards the card page from a silent regression: if the note's shape is
// what breaks, this fails and the fix is obvious; if the parser is what breaks, the card loses its
// name and nobody would notice from a diff.
func TestParseBlogProfile(t *testing.T) {
	const content = `---
title: 关于我
page: about
avatar:
---

::: profile-head
# 爻问道 （wen）

*口腔医学 · 本科在读 
兴趣：LLM与医学、效率工具、芭乐、Vibe coding*
:::

::: profile-meta
- 性别 **男**
- 年龄 **22**
- 所在地 **浙江 · 金华**
- 婚姻 **单身未婚** 
:::

## 碎碎念

正文不算名片。

::: links
- GitHub **https://github.com/ywdddddddddd**
- 邮箱 **ywddddddddddd@qq.com**

 微信公众号     
![[公众号二维码.jpg|162]]
**路漫漫其修远兮·吾将上下而求索**
:::
`
	p := parseBlogProfile(content)
	if p.Name != "爻问道 （wen）" {
		t.Errorf("Name = %q", p.Name)
	}
	if !strings.HasPrefix(p.Tagline, "口腔医学 · 本科在读") {
		t.Errorf("Tagline = %q, want it to start with 口腔医学", p.Tagline)
	}
	if strings.Contains(p.Tagline, "\n") {
		t.Errorf("Tagline must be one line, got %q", p.Tagline)
	}
	if p.Location != "浙江 · 金华" {
		t.Errorf("Location = %q, want 浙江 · 金华", p.Location)
	}
	if len(p.Links) != 2 {
		t.Fatalf("want 2 contact links, got %d: %+v", len(p.Links), p.Links)
	}
	if p.Links[0].Label != "GitHub" || p.Links[0].Href != "https://github.com/ywdddddddddd" {
		t.Errorf("first link = %+v", p.Links[0])
	}
	if p.Links[1].Kind != "mail" || !strings.HasPrefix(p.Links[1].Href, "mailto:") {
		t.Errorf("mail pill = %+v, want kind=mail with mailto: prefix", p.Links[1])
	}
}

// TestBlogProfilePills pins the pill order, which is a design decision rather than data order.
//
// The location pill's kind depends on data: 浙江 · 金华 has a pinned map URL, so it is a link.
// A place without one stays plain text — both shapes are asserted, because the fallback is the
// part that silently breaks when the map table is edited.
//
// TestBlogProfilePills 钉住药丸顺序，那是设计决定而不是数据顺序。
//
// 地区药丸的 Kind 取决于数据：浙江 · 金华 有钉好的地图 URL，所以它是链接。
// 没有地图的地方保持纯文本——两种形态都要断言，因为地图表被改动时，
// 悄悄坏掉的正是兜底那条。
func TestBlogProfilePills(t *testing.T) {
	p := &dto.BlogProfileDTO{
		Location: "浙江 · 金华",
		Links: []dto.BlogProfileLinkDTO{
			{Label: "邮箱", Href: "mailto:a@b.com", Kind: "mail", Icon: "mail"},
			{Label: "GitHub", Href: "https://github.com/x", Kind: "link", Icon: "mark-github"},
		},
	}
	pills := blogProfilePills(p, "/blog/rss.xml")
	if len(pills) != 4 {
		t.Fatalf("want 4 pills (location, github, mail, rss), got %d", len(pills))
	}
	want := []string{"link", "link", "mail", "rss"}
	for i, w := range want {
		if pills[i].Kind != w {
			t.Errorf("pill[%d].Kind = %q, want %q", i, pills[i].Kind, w)
		}
	}
	if pills[0].Label != "浙江 · 金华" {
		t.Errorf("first pill label = %q, want the location", pills[0].Label)
	}
	if !strings.Contains(pills[0].Href, "bing.com/maps") {
		t.Errorf("location pill href = %q, want the pinned map", pills[0].Href)
	}

	// A place with no pinned map must degrade to text, not to a dead link.
	// 没有钉地图的地方必须退化成文本，而不是一个空链接。
	other := blogProfilePills(&dto.BlogProfileDTO{Location: "火星 · 奥林匹斯山"}, "")
	if len(other) != 1 {
		t.Fatalf("want 1 pill, got %d", len(other))
	}
	if other[0].Kind != "text" || other[0].Href != "" {
		t.Errorf("unpinned location = %+v, want text with no href", other[0])
	}
}
