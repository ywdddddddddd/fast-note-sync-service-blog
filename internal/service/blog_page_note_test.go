package service

import "testing"

// TestBlogNotePageOnlyReadsFrontmatter 锁定标记必须来自 frontmatter。
//
// 这一条是整套页面机制的地基:如果正文里的 `page: about` 也被当成声明,
// 那么任何一篇**讲解这个功能**的笔记(比如本项目自己的文档)都会变成一张页面,
// 而从首页文章列表里消失。这不是假设——本仓库的笔记里就写着这些字。
func TestBlogNotePageOnlyReadsFrontmatter(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "AboutInFrontmatter",
			content: "---\ntitle: 关于我\npage: about\n---\n\n正文",
			want:    "about",
		},
		{
			// 友链页属于博客圈,而博客圈已搁置。有别名却没有路由,会把笔记从文章列表里
			// 拿走却不给读者落点;因此在博客圈做出来之前,它仍是一篇普通笔记。
			name:    "FriendsIsNotAPageWhileTheBlogrollIsOnHold",
			content: "---\npage: friends\n---\n正文",
			want:    "",
		},
		{
			name:    "QuotedInProseIsNotADeclaration",
			content: "---\ntitle: 怎么写简历页\n---\n\n在 frontmatter 里写 page: about 就会变成页面。",
			want:    "",
		},
		{
			name:    "NoFrontmatterAtAll",
			content: "page: about\n\n没有 frontmatter 块。",
			want:    "",
		},
		{
			name:    "UnknownAliasIsNotAPage",
			content: "---\npage: abuot\n---\n正文",
			want:    "",
		},
		{
			name:    "CaseIsFolded",
			content: "---\npage: About\n---\n正文",
			want:    "about",
		},
		{
			name:    "TrailingSpacesTolerated",
			content: "---\npage:   about   \n---\n正文",
			want:    "about",
		},
		{
			name:    "OrdinaryNote",
			content: "---\ntitle: 一篇普通文章\ntags: [a]\n---\n正文",
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := blogNotePage(tc.content); got != tc.want {
				t.Fatalf("blogNotePage() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBlogNoteAvatar 头像路径要能带引号、能带空格,因为 frontmatter 两种写法都常见。
func TestBlogNoteAvatar(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		{"---\navatar: _blog/avatar.jpg\n---\n", "_blog/avatar.jpg"},
		{"---\navatar: \"简历/头像.png\"\n---\n", "简历/头像.png"},
		{"---\navatar: '简历/头像.png'\n---\n", "简历/头像.png"},
		{"---\ntitle: 没有头像\n---\n", ""},
		{"没有 frontmatter", ""},
		// 正文里的 avatar 行同样不该被读到。
		{"---\ntitle: x\n---\navatar: 故意写在正文里\n", ""},
	}
	for _, tc := range cases {
		if got := blogNoteAvatar(tc.content); got != tc.want {
			t.Fatalf("blogNoteAvatar(%q) = %q, want %q", tc.content, got, tc.want)
		}
	}
}

// TestArticlesOnlyDropsPages 锁定过滤:页面笔记离开「文章」的清单一处生效。
func TestArticlesOnlyDropsPages(t *testing.T) {
	in := []*blogHomeSource{
		{NoteID: 1, Title: "压力阈值-1"},
		{NoteID: 2, Title: "关于我", Page: "about"},
		{NoteID: 3, Title: "压力阈值-2"},
	}
	out := articlesOnly(in)
	if len(out) != 2 {
		t.Fatalf("articlesOnly kept %d, want 2", len(out))
	}
	for _, s := range out {
		if s.Page != "" {
			t.Fatalf("page note %q survived the filter", s.Title)
		}
	}
	// 原切片不被改动:转写仍需要它完整。
	if len(in) != 3 {
		t.Fatalf("articlesOnly mutated its input: len=%d, want 3", len(in))
	}
}

// TestPageNotePicksMostRecentlyModifiedAndCounts 多篇同别名时取最近修改的一篇,并报出总数。
//
// 静默取一篇,正是作者改了另一篇却看不到任何变化的原因;把篇数报出来,设置页才能说明。
func TestPageNotePicksMostRecentlyModifiedAndCounts(t *testing.T) {
	in := []*blogHomeSource{
		{NoteID: 1, Title: "旧简历", Page: "about", MTime: 100},
		{NoteID: 2, Title: "新简历", Page: "about", MTime: 900},
		{NoteID: 3, Title: "中间那份", Page: "about", MTime: 500},
		{NoteID: 5, Title: "普通文章"},
	}
	got, count := pageNote(in, "about")
	if got == nil || got.NoteID != 2 {
		t.Fatalf("pageNote(about) = %v, want note 2", got)
	}
	if count != 3 {
		t.Fatalf("pageNote(about) count = %d, want 3", count)
	}
	// 不存在的别名返回 nil,而不是 panic 或挑一个无关的。
	if got, count = pageNote(in, "nope"); got != nil || count != 0 {
		t.Fatalf("pageNote(nope) = (%v, %d), want (nil, 0)", got, count)
	}
}

// TestBlogPageAliasesIsAWhitelist 白名单是这套机制的边界,不是实现细节。
func TestBlogPageAliasesIsAWhitelist(t *testing.T) {
	if _, ok := blogPageAliases["about"]; !ok {
		t.Fatal("about must be registered")
	}
	// 只注册 about。友链页属于已搁置的博客圈,提前注册它会让 `page: friends` 的笔记
	// 从文章列表里消失、却没有页面可去。
	if len(blogPageAliases) != 1 {
		t.Fatalf("whitelist has %d entries, want exactly 1 (about only)", len(blogPageAliases))
	}
	if _, ok := blogPageAliases["friends"]; ok {
		t.Fatal("friends must not be registered while the blogroll is on hold")
	}
}
