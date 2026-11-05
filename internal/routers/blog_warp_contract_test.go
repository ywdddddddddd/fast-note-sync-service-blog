package routers

import (
	"strings"
	"testing"
)

// TestWarpScriptContract 锁住过渡脚本的契约。
//
// 它原先断言「脚本会 getElementById('fns-warp')」—— 那是只有「关于我」一条入口时的做法。
// 2026-10-28 之后过渡覆盖站内文章链接（用户要求「点任意文章」都播），
// 实现改成事件委托 + 暴露 window.fnsWarp 给胶囊的 JS 跳转用。
//
// 2026-11-06 又一次改写：动画从星空换成手写钢笔（站主指定），
// 并把触发范围从「文章链接」扩到「所有站内导航」，因为站主要求「进入返回」都播。
//
// 这条测试守的是替换后的契约本身，而不是旧实现的痕迹。
func TestWarpScriptContract(t *testing.T) {
	js := blogWarpJS()

	// ① 胶囊里的漫游/回首页是按钮而不是链接，委托拦不到，必须有一个可调用的出口。
	if !strings.Contains(js, "window.fnsWarp = warpTo") {
		t.Error("缺少 window.fnsWarp 出口：胶囊的 JS 跳转将无法播放过渡")
	}

	// ② 站内链接靠委托，而不是逐个绑定 —— 图谱节点与目录树是后插入的，
	//    遍历时还不存在。
	if !strings.Contains(js, "document.addEventListener('click'") {
		t.Error("缺少点击委托：站内链接不会触发过渡")
	}

	// ③ 只拦普通左键。Ctrl/⌘/中键表示「在别处打开」，抢走它们等于让链接不再像链接。
	for _, guard := range []string{"ev.button !== 0", "ev.metaKey", "ev.ctrlKey"} {
		if !strings.Contains(js, guard) {
			t.Errorf("缺少修饰键/按键守卫：%s", guard)
		}
	}

	// ④ 抵达的那一半仍然靠 sessionStorage 标记，否则目标页会白屏等一整遍书写。
	if !strings.Contains(js, "fns:warp") {
		t.Error("缺少抵达标记，目标页不会渐显")
	}

	// ⑤ 减少动效时直接普通跳转，不做遮罩动画。
	if !strings.Contains(js, "if (STILL) { location.href = url; return; }") {
		t.Error("减少动效时没有退化成普通跳转")
	}

	// ⑥ 遮罩必须挂在 <html> 下。
	//
	// 挂在 <body> 下会踩一个真实的坑：出发时 body 拿到 transform，
	// 而被 transform 的元素是其 fixed 后代的包含块，遮罩就被钉在文档顶部。
	// 实测旧版 scrollY=1176 时画布矩形 y=-1176，整个在屏幕外 —— 滚下去的读者什么都看不到。
	if !strings.Contains(js, "document.documentElement.appendChild(view.root)") {
		t.Error("遮罩没有挂到 html 下：页面的 transform 会把它钉在文档顶部")
	}
	if strings.Contains(js, "document.body.appendChild(view.root)") {
		t.Error("遮罩被挂到了 body 下：页面 transform 会捕获它")
	}

	// ⑦ 循环播放，且加载完要补完当前这一遍，不能把笔画截在半路。
	if !strings.Contains(js, "elapsed % passMs") {
		t.Error("缺少循环：动画不会在长加载下重复播放")
	}
	if !strings.Contains(js, "wantsDone") {
		t.Error("缺少收尾意图标记：加载完会立刻抽走笔，笔画断在半路")
	}

	// ⑧ 硬上限。目标页永远不返回时，读者不能被锁在加载画面上。
	if !strings.Contains(js, "6000") {
		t.Error("缺少等待上限：目标页无响应会把读者困在加载画面")
	}

	// ⑨ 触发范围包含非文章页面（列表、归档、书架、首页），因为站主要求「进入返回」都播。
	//    这里守的是「不再只认 /blog/<slug>」这一点：脚本里不应再有单段路径的正则白名单。
	if !strings.Contains(js, "target.pathname === location.pathname") {
		t.Error("缺少同页判断：指向同一页面的链接会盖上一层白播的遮罩")
	}
	if strings.Contains(js, "/^\\/blog\\/[^/?#]+$/") {
		t.Error("仍在使用「只拦 /blog/<slug>」的正则：列表页与归档不会播过渡")
	}

	// ⑩ 二进制与订阅源路由不能加遮罩，否则是在一个文件下载前面放三秒加载画面。
	for _, ext := range []string{"xml", "json", "png", "svg"} {
		if !strings.Contains(js, ext) {
			t.Errorf("资源扩展名白名单缺少 %s：下载链接会套上加载遮罩", ext)
		}
	}
}

// TestWarpScriptIsRawStringSafe 守一条踩过四次的坑。
//
// blogWarpScript 是 Go 原始字符串（反引号定界）。脚本正文里出现任何一个反引号 ——
// 包括注释里为了排版而写的 —— 都会提前结束这个字符串，
// 编译时报 unexpected name after top level declaration。
//
// 这条测试放在这里而不是靠人记：它直接扫常量正文里的反引号数量。
// 定界符本身不计入，所以数量必须是偶数且每个都成对。
func TestWarpScriptIsRawStringSafe(t *testing.T) {
	js := blogWarpJS()
	// 字符串已经构建完成，任何残留反引号都说明它跑进了正文里。
	// 正文里合法使用反引号的地方是 ES6 模板字符串，本项目不用。
	if i := strings.IndexByte(js, '`'); i >= 0 {
		t.Fatalf("脚本正文第 %d 字节出现反引号，Go 原始字符串会被提前截断：%q",
			i, js[maxInt(0, i-40):minInt(len(js), i+40)])
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
