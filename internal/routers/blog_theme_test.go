package routers

import (
	"strings"
	"testing"
)

// TestBlogCSSBalanced 校验 blogCSS 的大括号配平。
//
// 为什么需要这条看起来多余的测试：2026-10-26 的一次样式改动漏了一个右括号，
// 于是 .dock-thumb.is-live 这条规则把**它之后的所有 CSS 都当成了嵌套**（Chrome 支持
// CSS Nesting，不会报错，只会让后面的选择器全部匹配不到）。后果是：
//   - 两个悬浮钮的 display:none 失效，移动端同时出现胶囊和悬浮钮
//   - 抽屉的面板过滤失效，热门与活跃两个面板叠在一起显示
//   - 该位置之后的其它样式也一并失效
//
// 编译通过、单元测试通过、页面也能正常打开 —— 只有肉眼能看出来。
// 这类错误必须在构建期拦住，所以用一条纯文本的配平检查兜底。
//
// TestBlogCSSBalanced guards brace balance in blogCSS. A single missing closing brace makes
// every following rule nest inside the previous one; Chrome accepts that silently, so the
// styles simply stop applying with no error anywhere.
func TestBlogCSSBalanced(t *testing.T) {
	css := blogCSS

	// 去掉注释再数，否则注释里的括号会误判。
	var b strings.Builder
	inComment := false
	for i := 0; i < len(css); i++ {
		c := css[i]
		if inComment {
			if c == '*' && i+1 < len(css) && css[i+1] == '/' {
				inComment = false
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(css) && css[i+1] == '*' {
			inComment = true
			i++
			continue
		}
		b.WriteByte(c)
	}
	body := b.String()

	if inComment {
		t.Fatalf("blogCSS 结束时仍在注释里：缺少 */")
	}

	depth := 0
	line := 1
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '\n':
			line++
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				t.Fatalf("blogCSS 第 %d 行出现多余的 }", line)
			}
		}
	}
	if depth != 0 {
		t.Fatalf("blogCSS 大括号不配平，结尾还剩 %d 个未闭合的 {；"+
			"多出的一个 { 会让它之后的所有规则被当成嵌套，静默失效", depth)
	}
}

// TestBlogCSSHidesMobileFabs 确认移动端隐藏悬浮钮的规则确实存在。
// 这不是重复 TestBlogCSSBalanced：那条测结构，这条测「具体某条语义规则还在」。
// 两者曾经同时被同一个括号错误绕过 —— 结构坏了，语义规则就整块消失。
func TestBlogCSSHidesMobileFabs(t *testing.T) {
	for _, want := range []string{
		"body.is-post .graph-fab",
		"body.is-post .ask-fab",
		"body.is-home .ask-fab",
		"body.drawer-hot .layout-home .rail>.panel:not(.hot){display:none}",
		"body.drawer-live .layout-home .rail>.panel:not(.act){display:none}",
	} {
		if !strings.Contains(blogCSS, want) {
			t.Errorf("blogCSS 缺少关键规则：%s", want)
		}
	}
}
