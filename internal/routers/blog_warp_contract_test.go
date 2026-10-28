package routers

import (
	"strings"
	"testing"
)

// TestWarpScriptContract 锁住穿越脚本的契约。
//
// 它原先断言「脚本会 getElementById('fns-warp')」—— 那是只有「关于我」一条入口时的做法。
// 2026-10-28 之后穿越覆盖所有指向 /blog/<slug> 的站内链接（用户要求「点任意文章」都穿越），
// 实现改成事件委托 + 暴露 window.fnsWarp 给胶囊的 JS 跳转用，
// 那个 id 已经没有任何代码引用，因此连同断言一起删掉。
//
// 这条测试守的是替换后的契约本身，而不是旧实现的痕迹。
func TestWarpScriptContract(t *testing.T) {
	js := blogWarpJS()

	// ① 胶囊里的漫游/回首页是按钮而不是链接，委托拦不到，必须有一个可调用的出口。
	if !strings.Contains(js, "window.fnsWarp = warpTo") {
		t.Error("缺少 window.fnsWarp 出口：胶囊的 JS 跳转将无法播放穿越")
	}

	// ② 站内文章链接靠委托，而不是逐个绑定 —— 图谱节点与目录树是后插入的，
	//    遍历时还不存在。
	if !strings.Contains(js, "document.addEventListener('click'") {
		t.Error("缺少点击委托：文章链接不会触发穿越")
	}

	// ③ 只拦普通左键。Ctrl/⌘/中键表示「在别处打开」，抢走它们等于让链接不再像链接。
	for _, guard := range []string{"ev.button !== 0", "ev.metaKey", "ev.ctrlKey"} {
		if !strings.Contains(js, guard) {
			t.Errorf("缺少修饰键/按键守卫：%s", guard)
		}
	}

	// ④ 抵达的那一半仍然靠 sessionStorage 标记，否则目标页会白屏等 1.15 秒。
	if !strings.Contains(js, "fns:warp") {
		t.Error("缺少抵达标记，目标页不会渐显")
	}

	// ⑤ 非文章的字面段必须排除。
	for _, seg := range []string{"'rss.xml'", "'graph'", "'shelf'", "'walk'"} {
		if !strings.Contains(js, seg) {
			t.Errorf("跳转白名单缺少 %s：会对非文章路径播穿越", seg)
		}
	}

	// ⑥ 减少动效时直接普通跳转，不做画布动画。
	if !strings.Contains(js, "if (STILL) { location.href = url; return; }") {
		t.Error("减少动效时没有退化成普通跳转")
	}
}

