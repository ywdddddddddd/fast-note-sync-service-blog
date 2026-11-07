package routers

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// renderFriendsPage executes the page template with the given view.
//
// 用页面模板本身渲染，而不是单独测分组函数：模板才是最终的输出，
// 缺失的分支（无分组、无卡片、非友链页）只有在整页渲染里才会暴露。
func renderFriendsPage(t *testing.T, view blogPageView) string {
	t.Helper()
	var buf bytes.Buffer
	if err := blogPageTmpl.Execute(&buf, view); err != nil {
		t.Fatalf("execute blogPageTmpl: %v", err)
	}
	return buf.String()
}

// TestBlogFriendGroupsOrder 分组按首次出现序；未分组排最前，且组内保持原顺序。
func TestBlogFriendGroupsOrder(t *testing.T) {
	friends := []dto.BlogFriendDTO{
		{Name: "a1", URL: "https://a1.example.com", Group: "技术"},
		{Name: "u1", URL: "https://u1.example.com"},
		{Name: "b1", URL: "https://b1.example.com", Group: "生活"},
		{Name: "a2", URL: "https://a2.example.com", Group: "技术"},
		{Name: "u2", URL: "https://u2.example.com"},
	}
	groups := blogFriendGroups(friends)
	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(groups))
	}
	if groups[0].Title != "" {
		t.Fatalf("first group title = %q, want the ungrouped bucket", groups[0].Title)
	}
	if len(groups[0].Friends) != 2 || groups[0].Friends[0].Name != "u1" || groups[0].Friends[1].Name != "u2" {
		t.Fatalf("ungrouped bucket = %#v, want [u1 u2]", groups[0].Friends)
	}
	if groups[1].Title != "技术" || len(groups[1].Friends) != 2 {
		t.Fatalf("second group = %#v, want 技术 with 2 entries", groups[1])
	}
	if groups[1].Friends[0].Name != "a1" || groups[1].Friends[1].Name != "a2" {
		t.Fatalf("技术 group order = %#v, want [a1 a2]", groups[1].Friends)
	}
	if groups[2].Title != "生活" || len(groups[2].Friends) != 1 {
		t.Fatalf("third group = %#v, want 生活 with 1 entry", groups[2])
	}
}

// TestBlogFriendGroupsFlat 没有任何 group 时是一张平铺网格：只有一个空标题区块。
func TestBlogFriendGroupsFlat(t *testing.T) {
	groups := blogFriendGroups([]dto.BlogFriendDTO{
		{Name: "甲", URL: "https://jia.example.com"},
		{Name: "乙", URL: "https://yi.example.com"},
	})
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1 flat group", len(groups))
	}
	if groups[0].Title != "" || len(groups[0].Friends) != 2 {
		t.Fatalf("flat group = %#v, want empty title and 2 entries", groups[0])
	}
	if got := blogFriendGroups(nil); got != nil {
		t.Fatalf("blogFriendGroups(nil) = %#v, want nil", got)
	}
}

// TestBlogFriendInitial 首字母按 rune 取，中文名不能取到半个字节。
func TestBlogFriendInitial(t *testing.T) {
	cases := []struct{ name, want string }{
		{"张三", "张"},
		{"alice", "A"},
		{"  李四 ", "李"},
		{"", "·"},
		{"   ", "·"},
		{"😀abc", "😀"},
	}
	for _, tc := range cases {
		if got := blogFriendInitial(tc.name); got != tc.want {
			t.Errorf("blogFriendInitial(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestBlogFriendCopyText 复制文本是三行固定格式，地址必须是绝对的。
func TestBlogFriendCopyText(t *testing.T) {
	got := blogFriendCopyText("问道", "https://yaowendao.asia/")
	want := "站名：问道\n地址：https://yaowendao.asia/blog\nRSS：https://yaowendao.asia/blog/rss.xml"
	if got != want {
		t.Fatalf("blogFriendCopyText() = %q, want %q", got, want)
	}
}

// TestFriendsPageRendersCardsGroupsAndButton 友链页的三样东西：卡片、分组标题、复制按钮。
func TestFriendsPageRendersCardsGroupsAndButton(t *testing.T) {
	friends := []dto.BlogFriendDTO{
		{Name: "张三", URL: "https://zhangsan.example.com", Desc: "记录生活与技术", Group: "技术博客"},
		{Name: "李四", URL: "https://lisi.example.com", Avatar: "https://lisi.example.com/av.png", Group: "生活"},
		{Name: "王五", URL: "https://wangwu.example.com", Desc: "没有分组"},
	}
	out := renderFriendsPage(t, blogPageView{
		PageAlias:       "friends",
		PageTitle:       "友链",
		SiteName:        "Notes",
		FriendGroups:    blogFriendGroups(friends),
		FriendsCopyText: "站名：Notes\n地址：https://x.example.com/blog\nRSS：https://x.example.com/blog/rss.xml",
	})

	for _, want := range []string{
		`href="https://zhangsan.example.com"`,
		`target="_blank"`,
		`rel="noopener noreferrer nofollow"`,
		`>张三<`,
		`>记录生活与技术<`,
		`class="friends-group-title">技术博客<`,
		`class="friends-group-title">生活<`,
		`src="https://lisi.example.com/av.png"`,
		`onerror="this.remove()"`,
		`class="friends-copy"`,
		"data-copy=\"站名：Notes\n地址：https://x.example.com/blog\nRSS：https://x.example.com/blog/rss.xml\"",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("friends page missing %q", want)
		}
	}

	// 未分组条目排在最前，且它的区块没有标题头。
	unGrouped := strings.Index(out, ">王五<")
	firstTitle := strings.Index(out, `class="friends-group-title"`)
	if unGrouped < 0 || firstTitle < 0 || unGrouped > firstTitle {
		t.Errorf("ungrouped card should render before the first group heading (card=%d heading=%d)", unGrouped, firstTitle)
	}
	// 首字母占位在没有头像时也要在，且有头像时作为加载失败的退路同样存在。
	if !strings.Contains(out, `>张</span>`) {
		t.Error("missing initial fallback for a friend without an avatar")
	}
}

// TestFriendsPageFlatHasNoHeadings 没有 group 时只有卡片，没有分组标题；复制按钮仍在。
func TestFriendsPageFlatHasNoHeadings(t *testing.T) {
	out := renderFriendsPage(t, blogPageView{
		PageAlias: "friends",
		FriendGroups: blogFriendGroups([]dto.BlogFriendDTO{
			{Name: "甲", URL: "https://jia.example.com"},
			{Name: "乙", URL: "https://yi.example.com"},
		}),
		FriendsCopyText: "站名：Notes\n地址：https://x.example.com/blog\nRSS：https://x.example.com/blog/rss.xml",
	})
	if strings.Contains(out, `class="friends-group-title"`) {
		t.Error("flat list must not render group headings")
	}
	if c := strings.Count(out, `class="friend-card"`); c != 2 {
		t.Errorf("got %d cards, want 2", c)
	}
	if !strings.Contains(out, `class="friends-copy"`) {
		t.Error("copy button must render on the friends page even without groups")
	}
}

// TestFriendsPageWithoutCardsKeepsButton 坏 YAML / 无条目时只掉卡片，页面与复制按钮照常。
func TestFriendsPageWithoutCardsKeepsButton(t *testing.T) {
	out := renderFriendsPage(t, blogPageView{
		PageAlias:       "friends",
		FriendsCopyText: "站名：Notes\n地址：https://x.example.com/blog\nRSS：https://x.example.com/blog/rss.xml",
	})
	if strings.Contains(out, `class="friend-card"`) {
		t.Error("no valid entries must render no cards")
	}
	if !strings.Contains(out, `class="friends-copy"`) {
		t.Error("copy button must still render when the list is empty")
	}
}

// TestFriendsBlockOnlyOnFriendsPage 别的页面即使带着数据也不渲染友链区块或复制按钮。
func TestFriendsBlockOnlyOnFriendsPage(t *testing.T) {
	out := renderFriendsPage(t, blogPageView{
		PageAlias: "about",
		FriendGroups: blogFriendGroups([]dto.BlogFriendDTO{
			{Name: "甲", URL: "https://jia.example.com"},
		}),
		FriendsCopyText: "站名：Notes\n地址：https://x.example.com/blog\nRSS：https://x.example.com/blog/rss.xml",
	})
	if strings.Contains(out, `class="friends-copy"`) {
		t.Error("about page must not render the friends copy button")
	}
	if strings.Contains(out, `class="friend-card"`) {
		t.Error("about page must not render friend cards")
	}
}

// TestFriendsPageEscapesAuthorText frontmatter 是作者输入，名称与描述都要按文本转义。
func TestFriendsPageEscapesAuthorText(t *testing.T) {
	out := renderFriendsPage(t, blogPageView{
		PageAlias: "friends",
		FriendGroups: blogFriendGroups([]dto.BlogFriendDTO{
			{Name: `<script>alert(1)</script>`, URL: "https://evil.example.com", Desc: `"><img src=x>`},
		}),
		FriendsCopyText: "站名：Notes\n地址：https://x.example.com/blog\nRSS：https://x.example.com/blog/rss.xml",
	})
	if strings.Contains(out, `<script>alert(1)</script>`) {
		t.Error("friend name was not escaped")
	}
	if !strings.Contains(out, `&lt;script&gt;alert(1)&lt;/script&gt;`) {
		t.Error("escaped friend name missing from output")
	}
	if strings.Contains(out, `<img src=x>`) {
		t.Error("friend description was not escaped")
	}
}
