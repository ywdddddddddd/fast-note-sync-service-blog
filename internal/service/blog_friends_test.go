package service

import (
	"reflect"
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// TestParseBlogFriends 友链解析的完整规则表。
//
// 这些用例覆盖三层：读得到（frontmatter / 键 / 顺序）、筛得掉（缺字段 / 危险 scheme / 坏 YAML）、
// 不误伤（正文里写着 friends 不算数据）。第三条尤其重要：本仓库的文档就写着这些字，
// 若解析器读整篇正文，任何讲解友链的笔记都会变成一张友链页。
func TestParseBlogFriends(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []dto.BlogFriendDTO
	}{
		{
			name: "FullEntriesKeepOrderAndFields",
			content: "---\npage: friends\ntitle: 友链\nfriends:\n" +
				"  - name: 张三的博客\n    url: https://zhangsan.com\n" +
				"    avatar: https://zhangsan.com/av.png\n    desc: 记录生活与技术\n    group: 技术博客\n" +
				"  - name: 李四\n    url: http://lisi.example.com\n    group: 生活\n" +
				"  - name: 王五\n    url: https://wangwu.example.com\n" +
				"---\n\n欢迎交换友链，申请方式见下。\n",
			want: []dto.BlogFriendDTO{
				{Name: "张三的博客", URL: "https://zhangsan.com", Avatar: "https://zhangsan.com/av.png", Desc: "记录生活与技术", Group: "技术博客"},
				{Name: "李四", URL: "http://lisi.example.com", Group: "生活"},
				{Name: "王五", URL: "https://wangwu.example.com"},
			},
		},
		{
			name: "MissingNameOrURLIsDropped",
			content: "---\nfriends:\n" +
				"  - name: \"\"\n    url: https://empty-name.example.com\n" +
				"  - name: 没有地址\n    url: \"\"\n" +
				"  - name: 合法\n    url: https://ok.example.com\n" +
				"---\n",
			want: []dto.BlogFriendDTO{{Name: "合法", URL: "https://ok.example.com"}},
		},
		{
			name: "DangerousSchemesDropTheWholeEntry",
			content: "---\nfriends:\n" +
				"  - name: 脚本\n    url: javascript:alert(1)\n" +
				"  - name: 内联图\n    url: https://ok.example.com\n    avatar: data:image/png;base64,AAAA\n" +
				"  - name: 文件\n    url: file:///etc/passwd\n" +
				"  - name: 协议相对\n    url: //example.com/x\n" +
				"  - name: 正常\n    url: https://normal.example.com\n" +
				"---\n",
			want: []dto.BlogFriendDTO{{Name: "正常", URL: "https://normal.example.com"}},
		},
		{
			name:    "BrokenYAMLHasNoCards",
			content: "---\npage: friends\nfriends: [\n---\n正文\n",
			want:    nil,
		},
		{
			name:    "FriendsNotAListHasNoCards",
			content: "---\npage: friends\nfriends: https://not-a-list.example.com\n---\n正文\n",
			want:    nil,
		},
		{
			name:    "NoFriendsKeyHasNoCards",
			content: "---\npage: friends\ntitle: 友链\n---\n正文\n",
			want:    nil,
		},
		{
			name:    "NoFrontmatterHasNoCards",
			content: "page: friends\n\nfriends:\n  - name: 正文里的字\n    url: https://body.example.com\n",
			want:    nil,
		},
		{
			name:    "EmptyFrontmatterHasNoCards",
			content: "---\n---\n正文\n",
			want:    nil,
		},
		{
			name: "ValuesAreTrimmed",
			content: "---\nfriends:\n" +
				"  - name: \"  张三  \"\n    url: \"  https://zhangsan.com  \"\n" +
				"    desc: \"  一句话  \"\n    group: \"  技术  \"\n" +
				"---\n",
			want: []dto.BlogFriendDTO{{Name: "张三", URL: "https://zhangsan.com", Desc: "一句话", Group: "技术"}},
		},
		{
			name:    "EmptyListHasNoCards",
			content: "---\npage: friends\nfriends: []\n---\n正文\n",
			want:    nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBlogFriends(tc.content)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseBlogFriends() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestBlogTrustedExternalURL 只放行带主机的 http(s)。
//
// 这条测试是安全边界的守卫：href 里出现 `javascript:` 或 `data:` 时，浏览器会执行它。
// 同时它也守住可用性——`https://` 这种没有主机的地址渲染出来是一张死卡。
func TestBlogTrustedExternalURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://example.com", true},
		{"http://example.com/path?q=1#frag", true},
		{"HTTPS://EXAMPLE.COM", true},
		{"https://例子.中国", true},
		{"https://", false},
		{"http://", false},
		{"javascript:alert(1)", false},
		{"JavaScript:alert(1)", false},
		{"data:image/svg+xml,<svg/>", false},
		{"file:///etc/passwd", false},
		{"ftp://example.com", false},
		{"//example.com/x", false},
		{"/blog/friends", false},
		{"", false},
		{"not a url", false},
	}
	for _, tc := range cases {
		if got := blogTrustedExternalURL(tc.raw); got != tc.want {
			t.Errorf("blogTrustedExternalURL(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}
