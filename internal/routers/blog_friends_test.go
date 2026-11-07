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

// TestBlogFriendCopyText 六字段卡的顺序与缺项省略。
//
// 顺序是站名/简述/邮箱/链接/订阅/标识；可选三项缺失时整行消失，不能留下「邮箱：」这样的空值行。
func TestBlogFriendCopyText(t *testing.T) {
	cases := []struct {
		name string
		site *dto.BlogSiteInfoDTO
		want string
	}{
		{
			name: "FullSixFields",
			site: &dto.BlogSiteInfoDTO{
				Desc:   "一个记录学习与生活的地方",
				Email:  "me@example.com",
				Avatar: "https://example.com/a.png",
			},
			want: "站名：问道\n简述：一个记录学习与生活的地方\n邮箱：me@example.com\n" +
				"链接：https://yaowendao.asia/blog\n订阅：https://yaowendao.asia/blog/rss.xml\n" +
				"标识：https://example.com/a.png",
		},
		{
			name: "OnlyDesc",
			site: &dto.BlogSiteInfoDTO{Desc: "只有简述"},
			want: "站名：问道\n简述：只有简述\n" +
				"链接：https://yaowendao.asia/blog\n订阅：https://yaowendao.asia/blog/rss.xml",
		},
		{
			name: "OnlyEmailAndAvatar",
			site: &dto.BlogSiteInfoDTO{Email: "me@example.com", Avatar: "https://example.com/a.png"},
			want: "站名：问道\n邮箱：me@example.com\n" +
				"链接：https://yaowendao.asia/blog\n订阅：https://yaowendao.asia/blog/rss.xml\n" +
				"标识：https://example.com/a.png",
		},
		{
			name: "NilSiteKeepsThreeLines",
			site: nil,
			want: "站名：问道\n链接：https://yaowendao.asia/blog\n订阅：https://yaowendao.asia/blog/rss.xml",
		},
		{
			name: "WhitespaceOnlyFieldsAreOmitted",
			site: &dto.BlogSiteInfoDTO{Desc: "   ", Email: "", Avatar: "\t"},
			want: "站名：问道\n链接：https://yaowendao.asia/blog\n订阅：https://yaowendao.asia/blog/rss.xml",
		},
		{
			name: "NewlinesAreFlattened",
			site: &dto.BlogSiteInfoDTO{Desc: "第一行\n第二行", Email: "me@example.com\r\nsecond@example.com"},
			want: "站名：问道\n简述：第一行 第二行\n邮箱：me@example.com second@example.com\n" +
				"链接：https://yaowendao.asia/blog\n订阅：https://yaowendao.asia/blog/rss.xml",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := blogFriendCopyText("问道", "https://yaowendao.asia/", tc.site)
			if got != tc.want {
				t.Fatalf("blogFriendCopyText() = %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "简述：\n") || strings.Contains(got, "邮箱：\n") || strings.Contains(got, "标识：\n") {
				t.Fatalf("copy card contains an empty value line: %q", got)
			}
		})
	}
}

// TestBlogShareDataAvatar 分享卡头像：site.avatar 覆盖内嵌图，空值回退内嵌图。
func TestBlogShareDataAvatar(t *testing.T) {
	post := &dto.BlogPostDTO{
		Title:     "标题",
		CreatedAt: "2026-11-07",
		HTML:      "<p>正文</p>",
		Path:      "（1）技术/示例.md",
	}
	withSite := blogShareData(post, "摘要", "问道", "https://example.com/site.png")
	if !strings.Contains(withSite, `"avatar":"https://example.com/site.png"`) {
		t.Fatalf("share data did not use site avatar: %s", withSite)
	}
	if strings.Contains(withSite, `"avatar":"`+blogAvatarURL+`"`) {
		t.Fatalf("share data kept the embedded avatar alongside the site avatar: %s", withSite)
	}

	fallback := blogShareData(post, "摘要", "问道", "")
	if !strings.Contains(fallback, `"avatar":"`+blogAvatarURL+`"`) {
		t.Fatalf("share data did not fall back to the embedded avatar: %s", fallback)
	}

	if got := blogShareData(nil, "摘要", "问道", ""); got != "" {
		t.Fatalf("blogShareData(nil) = %q, want empty", got)
	}
}

// TestBlogProfileAvatar 名片头像三档兜底：简历 → site.avatar → 内嵌。
func TestBlogProfileAvatar(t *testing.T) {
	cases := []struct{ note, site, want string }{
		{"https://note.example.com/a.png", "https://site.example.com/a.png", "https://note.example.com/a.png"},
		{"", "https://site.example.com/a.png", "https://site.example.com/a.png"},
		{"", "", blogAvatarURL},
	}
	for _, tc := range cases {
		if got := blogProfileAvatar(tc.note, tc.site); got != tc.want {
			t.Errorf("blogProfileAvatar(%q, %q) = %q, want %q", tc.note, tc.site, got, tc.want)
		}
	}
}

// TestFriendsPageRendersCardsGroupsAndButton 友链页的三样东西：卡片、分组标题、复制按钮。
func TestFriendsPageRendersCardsGroupsAndButton(t *testing.T) {
	friends := []dto.BlogFriendDTO{
		{Name: "张三", URL: "https://zhangsan.example.com", Desc: "记录生活与技术", Group: "技术博客",
			Email: "zhangsan@example.com", Feed: "https://zhangsan.example.com/rss.xml"},
		{Name: "李四", URL: "https://lisi.example.com", Avatar: "https://lisi.example.com/av.png", Group: "生活"},
		{Name: "王五", URL: "https://wangwu.example.com", Desc: "没有分组"},
	}
	copyText := "站名：Notes\n简述：记录\n邮箱：me@example.com\n链接：https://x.example.com/blog\n" +
		"订阅：https://x.example.com/blog/rss.xml\n标识：https://x.example.com/a.png"
	out := renderFriendsPage(t, blogPageView{
		PageAlias:       "friends",
		PageTitle:       "友链",
		SiteName:        "Notes",
		FriendGroups:    blogFriendGroups(friends),
		FriendsCopyText: copyText,
	})

	for _, want := range []string{
		`href="https://zhangsan.example.com"`,
		`target="_blank"`,
		`rel="noopener noreferrer nofollow"`,
		`data-name="张三"`,
		`data-avatar="https://lisi.example.com/av.png"`,
		`>张三<`,
		`>记录生活与技术<`,
		`class="friends-group-title">技术博客<`,
		`class="friends-group-title">生活<`,
		`src="https://lisi.example.com/av.png"`,
		`onerror="this.remove()"`,
		`class="friends-copy"`,
		`复制本站信息，可添加友链`,
		`data-copy="` + copyText + `"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("friends page missing %q", want)
		}
	}
	if strings.Contains(out, "想交换友链？复制本站信息后发给我。") {
		t.Error("old copy hint must be gone")
	}
	// email 与 feed 只存不显示：它们不应出现在卡片标记里。
	if strings.Contains(out, "zhangsan@example.com") {
		t.Error("friend email must not be rendered on the page")
	}
	if strings.Contains(out, "https://zhangsan.example.com/rss.xml") {
		t.Error("friend feed must not be rendered on the page")
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

// TestFriendsPageApplyFormAndFlight 友链页新增的两块：申请表单与飞行动画遮罩。
//
// The form's field names are the wire contract with POST /blog/friends/apply; the overlay's ids
// are what the inline loader and warp_flight.js agree on. A typo in either would fail silently in
// the browser, so both are asserted here.
//
// 表单的 name 是与 POST /blog/friends/apply 的线上契约；遮罩的 id 是内联加载器与
// warp_flight.js 之间的约定。任一处写错在浏览器里都是静默失败，因此都在这里断言。
func TestFriendsPageApplyFormAndFlight(t *testing.T) {
	out := renderFriendsPage(t, blogPageView{
		PageAlias: "friends",
		FriendGroups: blogFriendGroups([]dto.BlogFriendDTO{
			{Name: "小倪", URL: "https://ni.example.com", Avatar: "https://ni.example.com/a.png"},
		}),
		FriendsCopyText: "站名：Notes",
	})
	for _, want := range []string{
		`id="friends-apply"`,
		`name="name"`,
		`name="url"`,
		`name="avatar"`,
		`name="desc"`,
		`name="email"`,
		`name="feed"`,
		`name="message"`,
		`class="friends-submit"`,
		`>提交申请<`,
		`已收到你的申请，通过后会出现在上面。`,
		`/blog/friends/apply`,
		`id="warp-flight"`,
		`id="warp-flight-stage"`,
		`id="warp-flight-title"`,
		`/blog/assets/warp-flight.js?v=`,
		`prefers-reduced-motion`,
		`正在飞往（`,
		`textContent`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("friends page missing %q", want)
		}
	}
	// 站名必须经 textContent 注入，不能拼进 HTML 字符串。
	if !strings.Contains(out, "title.textContent = '正在飞往（' + name + '）的星球'") {
		t.Error("flight title must be injected with textContent, not innerHTML")
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
	for _, forbidden := range []string{`class="friends-copy"`, `class="friend-card"`, `id="friends-apply"`, `id="warp-flight"`} {
		if strings.Contains(out, forbidden) {
			t.Errorf("about page must not render %s", forbidden)
		}
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
