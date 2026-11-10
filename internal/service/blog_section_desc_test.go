package service

import (
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

func TestBlogIsSectionNote(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"_栏目说明.md", true},
		{"修复笔记/_栏目说明.md", true},
		{"（1）杏海无岸/修复笔记/_栏目说明.md", true},
		{"修复笔记\\_栏目说明.md", true},
		{"修复笔记/口腔_栏目说明.md", false},
		{"修复笔记/_栏目说明.md.bak", false},
		{"修复笔记/修复笔记.md", false},
	}
	for _, c := range cases {
		if got := blogIsSectionNote(c.path); got != c.want {
			t.Fatalf("blogIsSectionNote(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestBlogSectionDescFromContent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "double quoted",
			content: "---\nblog-desc: \"牙体缺损的修复路径：嵌体、高嵌体、全冠与桩核。\"\n---\n",
			want:    "牙体缺损的修复路径：嵌体、高嵌体、全冠与桩核。",
		},
		{
			name:    "quoted with escaped quote",
			content: "---\nblog-desc: \"他说 \\\"好\\\"\"\n---\n",
			want:    `他说 "好"`,
		},
		{
			name:    "unquoted",
			content: "---\nblog-desc: 口腔颌面外科：麻醉、拔牙、感染与创伤处理。\n---\n",
			want:    "口腔颌面外科：麻醉、拔牙、感染与创伤处理。",
		},
		{
			name:    "single quoted",
			content: "---\nblog-desc: '训练计划、动作模式与恢复。'\n---\n",
			want:    "训练计划、动作模式与恢复。",
		},
		{
			name:    "empty value",
			content: "---\nblog-desc:\n---\n",
			want:    "",
		},
		{
			name:    "key absent",
			content: "---\ntitle: 修复笔记\n---\n",
			want:    "",
		},
		{
			name:    "prose only",
			content: "正文里写 blog-desc: 这句话不算。\n",
			want:    "",
		},
		{
			name:    "key outside frontmatter",
			content: "---\ntitle: 修复笔记\n---\nblog-desc: 正文不算。\n",
			want:    "",
		},
	}
	for _, c := range cases {
		if got := blogSectionDescFromContent(c.content); got != c.want {
			t.Fatalf("%s: blogSectionDescFromContent() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBlogSectionDescsFromNotes(t *testing.T) {
	notes := []*domain.Note{
		{
			Path:    "（1）杏海无岸/修复笔记/_栏目说明.md",
			Content: "---\nblog-desc: \"牙体缺损的修复路径。\"\n---\n",
		},
		{
			Path:    "（2）卿本自然/睡眠/_栏目说明.md",
			Content: "---\nblog-desc: \"睡眠结构、昼夜节律。\"\n---\n",
		},
		{
			// Root-level notes describe no folder: ignored.
			Path:    "_栏目说明.md",
			Content: "---\nblog-desc: \"根目录不是栏目。\"\n---\n",
		},
		{
			// Empty value: ignored so the aggregator falls back to the folder name.
			Path:    "（1）杏海无岸/口外笔记/_栏目说明.md",
			Content: "---\nblog-desc:\n---\n",
		},
		{
			Path:    "（1）杏海无岸/口组笔记/_栏目说明.md",
			Content: "---\ntitle: 不是说明\n---\n",
		},
		{
			Path:    "（2）卿本自然\\压力\\_栏目说明.md",
			Content: "---\nblog-desc: \"压力的来源与调节。\"\n---\n",
		},
	}

	got := blogSectionDescsFromNotes(notes)
	want := map[string]string{
		"修复笔记": "牙体缺损的修复路径。",
		"睡眠":   "睡眠结构、昼夜节律。",
		"压力":   "压力的来源与调节。",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d descriptions, want %d: %#v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("desc[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestBlogAggregateSectionsUsesDescriptions(t *testing.T) {
	entries := []*dto.BlogEntryDTO{
		{Path: "（1）杏海无岸/修复笔记/a.md", Slug: "a", CreatedAt: "2026-11-08", Words: 10},
		{Path: "（1）杏海无岸/修复笔记/b.md", Slug: "b", CreatedAt: "2026-11-09", Words: 20},
		{Path: "（1）杏海无岸/口外笔记/c.md", Slug: "c", CreatedAt: "2026-11-07", Words: 30},
	}

	out := blogAggregateSections(entries, "杏海无岸", 2, 0, map[string]string{
		"修复笔记": "牙体缺损的修复路径。",
	})
	if len(out) != 2 {
		t.Fatalf("got %d section cards, want 2", len(out))
	}

	byName := make(map[string]*dto.BlogSectionRecDTO, len(out))
	for _, rec := range out {
		byName[rec.Name] = rec
	}
	if got := byName["修复笔记"].Desc; got != "牙体缺损的修复路径。" {
		t.Fatalf("修复笔记 desc = %q", got)
	}
	if got := byName["口外笔记"].Desc; got != "口外笔记" {
		t.Fatalf("口外笔记 should fall back to its own name, got %q", got)
	}
}
