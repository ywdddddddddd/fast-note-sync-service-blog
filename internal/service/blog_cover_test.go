package service

import (
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
)

// fm builds a note body with the given frontmatter lines.
// fm 用给定的 frontmatter 行构造一篇笔记正文。
func fm(lines ...string) string {
	out := "---\n"
	for _, l := range lines {
		out += l + "\n"
	}
	return out + "---\n\n正文第一段。\n"
}

// parseBlogCover must read the three keys, and must return the same nil for every kind of
// "cannot use this" — a note without a cover and a note with a broken cover are indistinguishable
// to the reader, and that is the point.
//
// parseBlogCover 必须读出三个键，并且对每一种「用不了」都返回同一个 nil——
// 对读者而言，「没设封面」与「封面坏了」无法区分，而这正是要点。
func TestParseBlogCover(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		wantRef   string
		wantKind  string
		wantCover bool
	}{
		{
			name:      "all three keys",
			content:   fm("cover: 图片/牙发育-1.png", "cover-focus: 50% 35%", "cover-zoom: 1.4"),
			wantRef:   "图片/牙发育-1.png",
			wantKind:  "image",
			wantCover: true,
		},
		{
			name:      "cover only falls back to defaults",
			content:   fm("cover: 封面.jpg"),
			wantRef:   "封面.jpg",
			wantKind:  "image",
			wantCover: true,
		},
		{
			name:      "quoted value",
			content:   fm(`cover: "带 空格 的图.png"`),
			wantRef:   "带 空格 的图.png",
			wantKind:  "image",
			wantCover: true,
		},
		{
			name:      "obsidian embed form",
			content:   fm(`cover: "![[牙发育-1.png]]"`),
			wantRef:   "牙发育-1.png",
			wantKind:  "image",
			wantCover: true,
		},
		{
			name:      "video cover",
			content:   fm("cover: 夹子.mp4"),
			wantRef:   "夹子.mp4",
			wantKind:  "video",
			wantCover: true,
		},
		{
			// A PDF is not something a square card can show; treating it as a cover would put a
			// blank rectangle where the article image belongs.
			// PDF 不是正方形卡片能展示的东西；把它当封面会在本该是文章图片的位置留一块空白。
			name:      "unsupported extension is not a cover",
			content:   fm("cover: 讲义.pdf"),
			wantCover: false,
		},
		{
			name:      "no frontmatter at all",
			content:   "正文，没有 frontmatter。\n",
			wantCover: false,
		},
		{
			name:      "frontmatter without cover",
			content:   fm("title: 某篇笔记", "tags: [口腔]"),
			wantCover: false,
		},
		{
			// `cover:` must not match `cover-focus:` — the modifier key alone is not a cover.
			// `cover:` 不得命中 `cover-focus:`——只有一个修饰键不构成封面。
			name:      "modifier key alone is not a cover",
			content:   fm("cover-focus: 50% 50%", "cover-zoom: 2"),
			wantCover: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ref, cover := parseBlogCover(c.content, "笔记/某篇.md")
			if (cover != nil) != c.wantCover {
				t.Fatalf("有封面 = %v, 期望 %v", cover != nil, c.wantCover)
			}
			if !c.wantCover {
				return
			}
			if ref != c.wantRef {
				t.Errorf("ref = %q, 期望 %q", ref, c.wantRef)
			}
			if cover.Kind != c.wantKind {
				t.Errorf("kind = %q, 期望 %q", cover.Kind, c.wantKind)
			}
		})
	}
}

func TestParseBlogCoverDefaults(t *testing.T) {
	_, cover := parseBlogCover(fm("cover: a.png"), "n.md")
	if cover == nil {
		t.Fatal("应解析出封面")
	}
	if cover.FocusX != 50 || cover.FocusY != 50 {
		t.Errorf("默认焦点 = %v,%v, 期望 50,50", cover.FocusX, cover.FocusY)
	}
	if cover.Zoom != 1 {
		t.Errorf("默认缩放 = %v, 期望 1", cover.Zoom)
	}
}

// Focus parsing accepts the shapes a human or a plugin writes, and clamps rather than rejects.
// 焦点解析接受人或插件会写出的形状，并且夹取而不是拒绝。
func TestParseBlogCoverFocus(t *testing.T) {
	cases := []struct {
		in     string
		x, y   float64
		wantOK bool
	}{
		{"50% 35%", 50, 35, true},
		{"50 35", 50, 35, true},
		{"50%,35%", 50, 35, true},
		{"  12%   88%  ", 12, 88, true},
		{"0% 0%", 0, 0, true},
		{"100% 100%", 100, 100, true},
		// Out of range is meaningless but harmless, so it is pinned to the edge instead of
		// silently discarding the author's crop.
		// 越界没有意义但无害，因此贴到边缘，而不是悄悄丢弃作者的裁切。
		{"-20% 140%", 0, 100, true},
		{"50", 0, 0, false},
		{"", 0, 0, false},
		{"abc def", 0, 0, false},
	}
	for _, c := range cases {
		x, y, ok := parseBlogCoverFocus(c.in)
		if ok != c.wantOK {
			t.Errorf("%q ok = %v, 期望 %v", c.in, ok, c.wantOK)
			continue
		}
		if ok && (x != c.x || y != c.y) {
			t.Errorf("%q = %v,%v, 期望 %v,%v", c.in, x, y, c.x, c.y)
		}
	}
}

func TestParseBlogCoverZoom(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"1", 1, true},
		{"1.4", 1.4, true},
		{"2x", 2, true},
		// Below 1 would leave gaps inside the square; above 4 is no longer a crop but a fragment.
		// 小于 1 会在正方形里留白；大于 4 已经不是裁切而是碎片了。
		{"0.5", 1, true},
		{"99", 4, true},
		{"0", 0, false},
		{"-2", 0, false},
		{"", 0, false},
		{"abc", 0, false},
	}
	for _, c := range cases {
		got, ok := parseBlogCoverZoom(c.in)
		if ok != c.ok {
			t.Errorf("%q ok = %v, 期望 %v", c.in, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("%q = %v, 期望 %v", c.in, got, c.want)
		}
	}
}

// This is the disambiguation the owner asked for by name: two folders both holding 图.png must not
// resolve to the same file for unrelated notes.
//
// 这就是使用者点名要求的那条消歧：两个目录下各有一个 图.png 时，
// 互不相关的笔记不能解析到同一个文件。
func TestPickNearestFile(t *testing.T) {
	deep := &domain.File{ID: 1, Path: "口腔笔记/口组笔记/图.png"}
	shallow := &domain.File{ID: 2, Path: "图片/图.png"}
	other := &domain.File{ID: 3, Path: "口腔笔记B/图.png"}
	sibling := &domain.File{ID: 4, Path: "口腔笔记/图.png"}

	cases := []struct {
		name     string
		list     []*domain.File
		notePath string
		wantID   int64
	}{
		{
			name:     "sibling folder wins over a far one",
			list:     []*domain.File{shallow, deep},
			notePath: "口腔笔记/口组笔记/某篇.md",
			wantID:   1,
		},
		{
			name:     "order in the list does not matter",
			list:     []*domain.File{deep, shallow},
			notePath: "口腔笔记/口组笔记/某篇.md",
			wantID:   1,
		},
		{
			// 口腔笔记B shares seven characters with 口腔笔记 but not one whole segment. A
			// character-wise prefix test would call it a match and beat the genuine sibling folder;
			// counting segments keeps 口腔笔记/图.png as the winner.
			// 口腔笔记B 与 口腔笔记 共享七个字符，却连一段都不共享。
			// 按字符比较会把前者当成"命中"并压过真正的同级目录；按段计则让 口腔笔记/图.png 胜出。
			name:     "character prefix is not a directory match",
			list:     []*domain.File{other, sibling},
			notePath: "口腔笔记/某篇.md",
			wantID:   4,
		},
		{
			name:     "single candidate is returned as-is",
			list:     []*domain.File{deep},
			notePath: "完全/无关/路径.md",
			wantID:   1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pickNearestFile(c.list, c.notePath)
			if got == nil {
				t.Fatal("返回 nil")
			}
			if got.ID != c.wantID {
				t.Errorf("选中 ID=%d (%s), 期望 ID=%d", got.ID, got.Path, c.wantID)
			}
		})
	}
}

// Equal proximity must resolve to the same answer every time, or the same vault would show
// different covers on two builds.
// 同等贴近时必须每次都给出同一个答案，否则同一份仓库在两次构建里会显示不同的封面。
func TestPickNearestFileIsDeterministic(t *testing.T) {
	a := &domain.File{ID: 1, Path: "a/x.png"}
	b := &domain.File{ID: 2, Path: "b/x.png"}

	first := pickNearestFile([]*domain.File{a, b}, "n.md")
	for i := 0; i < 20; i++ {
		got := pickNearestFile([]*domain.File{b, a}, "n.md")
		if got.ID != first.ID {
			t.Fatalf("第 %d 次结果不同: %d vs %d", i, got.ID, first.ID)
		}
	}
}

// sharedDirSegments counts whole segments, never characters.
// sharedDirSegments 按整段计数，绝不按字符。
func TestSharedDirSegments(t *testing.T) {
	cases := []struct {
		a, b []string
		want int
	}{
		{[]string{"口腔笔记", "口组笔记"}, []string{"口腔笔记", "口组笔记"}, 2},
		{[]string{"口腔笔记", "口组笔记"}, []string{"口腔笔记"}, 1},
		{[]string{"口腔笔记A"}, []string{"口腔笔记B"}, 0},
		{[]string{"a", "b", "c"}, []string{"a", "b", "d"}, 2},
		{nil, []string{"a"}, 0},
	}
	for _, c := range cases {
		if got := sharedDirSegments(c.a, c.b); got != c.want {
			t.Errorf("sharedDirSegments(%v,%v) = %d, 期望 %d", c.a, c.b, got, c.want)
		}
	}
}

// Windows and Linux must agree on the same vault path, or covers would resolve differently on the
// server than they do in the editor.
// Windows 与 Linux 必须对同一条仓库路径给出一致答案，否则封面在服务器与编辑器里解析结果不同。
func TestFilepathSlashNormalisesBackslashes(t *testing.T) {
	if got := filepathSlash(`口腔笔记\口组笔记\x.png`); got != "口腔笔记/口组笔记/x.png" {
		t.Errorf("得到 %q", got)
	}
	if got := filepathSlash("a/b.png"); got != "a/b.png" {
		t.Errorf("得到 %q", got)
	}
}
