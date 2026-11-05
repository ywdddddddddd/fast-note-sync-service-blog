package routers

import (
	"strings"
	"testing"
)

// TestWarpScriptContract 锁住过渡脚本的契约。
//
// 三代实现，每一代都是站主验收退回后重写的：
//
//	星空（2026-10 之前）→ 只有「关于我」一条入口
//	星空 + 事件委托（2026-10-28）→ 覆盖所有文章链接
//	钢笔手写 + 遮罩常驻（2026-11-06）→ 覆盖所有站内导航，遮罩在每一页都预先存在
//
// 第三代是站主用录屏退回三次具体失败之后写的，因此这里的断言直接对应那三条：
// 入站要有动画、返回不能全白、动画播完不能还在读条。
func TestWarpScriptContract(t *testing.T) {
	js := blogWarpJS()

	// ① 胶囊里的漫游/回首页是按钮而不是链接，委托拦不到，必须有一个可调用的出口。
	if !strings.Contains(js, "window.fnsWarp = function") {
		t.Error("缺少 window.fnsWarp 出口：胶囊的 JS 跳转将无法播放过渡")
	}

	// ② 站内链接靠委托 —— 图谱节点与目录树是后插入的，遍历时还不存在。
	if !strings.Contains(js, "document.addEventListener('click'") {
		t.Error("缺少点击委托：站内链接不会触发过渡")
	}

	// ③ 只拦普通左键。Ctrl/⌘/中键表示「在别处打开」。
	for _, guard := range []string{"ev.button !== 0", "ev.metaKey", "ev.ctrlKey"} {
		if !strings.Contains(js, guard) {
			t.Errorf("缺少修饰键/按键守卫：%s", guard)
		}
	}

	// ④ 减少动效时退化成普通跳转，且根本不建遮罩。
	if !strings.Contains(js, "if (STILL) { location.href = url; return; }") {
		t.Error("减少动效时没有退化成普通跳转")
	}
	if !strings.Contains(js, "if (STILL) { return; }") {
		t.Error("减少动效时仍然创建遮罩")
	}

	// ── 以下四条对应站主三次退回的具体失败 ──────────────────────────────

	// ⑤ 入站必须有动画：遮罩由 head 脚本无条件创建，而不是等一个 sessionStorage 标记。
	//
	// 失败形态：首访没有标记 → 没有遮罩 → 读者先看白页再看站点。
	if !strings.Contains(js, "  start();\n})();") {
		t.Error("没有在文档一加载就调用 start()：入站不会有动画")
	}
	if strings.Contains(js, "if (arrived)") {
		t.Error("仍以 sessionStorage 标记作为创建遮罩的条件：首访没有动画")
	}
	// 相位交接仍要靠 sessionStorage，但那是「从哪一帧继续」，不是「要不要播」。
	if !strings.Contains(js, "fns:ink-phase") {
		t.Error("缺少相位交接：每次导航笔都会从 W 重来")
	}

	// ⑥ 返回不能全白：bfcache 恢复必须摘掉 fns-warping。
	//
	// 失败形态：返回时旧文档原样恢复，带着 fns-warping，而上一版靠 body{opacity:0}
	// 让页面消失 —— 那条规则让整页全白且永不恢复。
	if !strings.Contains(js, "ev.persisted") {
		t.Error("pageshow 没有处理 bfcache 恢复：按返回会全白")
	}
	if !strings.Contains(js, "classList.remove('fns-warping', 'fns-arrived')") {
		t.Error("恢复时没有摘掉 fns-warping：返回得到一张白页")
	}

	// ⑦ 动画播完不能还在读条：等到文档与图片都落定才抬起遮罩。
	//
	// 失败形态：按固定遍数抬起，而栏目页十几张封面每张约 700ms，读者看笔写完之后继续看转圈。
	if !strings.Contains(js, "function documentSettled") {
		t.Error("缺少就绪判定：遮罩会在内容到达之前抬起")
	}
	if !strings.Contains(js, "var imgs = document.images") {
		t.Error("没有等图片落定：封面还会在遮罩抬起之后继续加载")
	}
	if !strings.Contains(js, "passDone && documentSettled()") {
		t.Error("抬起条件不是「写完一遍且页面就绪」")
	}
	if !strings.Contains(js, "warmDestination") {
		t.Error("缺少目标页预热：出发时不会预取下一屏的封面")
	}

	// ⑧ 笔必须与字形重合：每笔一条子路径。
	//
	// 失败形态：示例把「s」与「B」各折成一条含两个子路径的 path，
	// getPointAtLength 会在子路径之间跳 —— 实测「B」的接缝处笔一步走了 127 单位，
	// 于是笔飘在字形外面。
	if n := strings.Count(js, "M 798,165"); n != 1 {
		t.Errorf("B 的主干路径应只出现一次，实际 %d", n)
	}
	if strings.Contains(js, "M 698,198 C 696,208 692,220 687,228 M 698,280") {
		t.Error("s 仍是单条含两个子路径的 path：笔会跳到下一段")
	}
	if strings.Contains(js, "M 798,165 C 794,180 782,235 778,295 C 776,306 768,308 762,300 M 788,178") {
		t.Error("B 仍是单条含两个子路径的 path：笔会跳到下一段")
	}

	// ⑨ 遮罩挂 html 下。挂 body 下会被出发时的 transform 钉在文档顶部。
	if !strings.Contains(js, "document.documentElement.appendChild(view.root)") {
		t.Error("遮罩没有挂到 html 下：页面的 transform 会把它钉在文档顶部")
	}
	if strings.Contains(js, "document.body.appendChild(view.root)") {
		t.Error("遮罩被挂到了 body 下：页面 transform 会捕获它")
	}

	// ⑩ 错误页不装遮罩。
	if !strings.Contains(js, "location.pathname.indexOf('/blog') !== 0") {
		t.Error("缺少非博客页守卫：404 前面会挡一层加载画面")
	}
}

// TestInkScriptIsInHead 守一条站主直接报过的缺陷。
//
// 遮罩必须由 <head> 里的脚本创建。注入到 </body> 之前时，浏览器已经把光秃秃的页面画出来了，
// 动画存在但晚了一拍才出现 —— 站主的原话是「入站时没有加载动画」。
//
// 这条测试断言的是模板接线，而不是脚本内容：五个阅读模板的 head 里都要有 inkjs，
// 且没有任何一处仍在 body 末尾塞 WarpJS。
func TestInkScriptIsInHead(t *testing.T) {
	var heads int
	for _, tmpl := range []string{
		blogChromePartials,
		blogPostTmpl.Tree.Root.String(),
		blogPageTmpl.Tree.Root.String(),
		blogShelfPage,
		blogReaderPage,
	} {
		if strings.Contains(tmpl, "<script>{{inkjs}}</script>") {
			heads++
		}
	}
	if heads < 5 {
		t.Errorf("只有 %d 个 head 注入了 inkjs，应为 5 个", heads)
	}

	for _, tmpl := range []string{
		blogPostTmpl.Tree.Root.String(),
		blogPageTmpl.Tree.Root.String(),
		blogCardTmpl.Tree.Root.String(),
		blogListTmpl.Tree.Root.String(),
		blogErrorTmpl.Tree.Root.String(),
	} {
		if strings.Contains(tmpl, "{{.WarpJS}}") {
			t.Error("仍有模板在 body 里注入 WarpJS：遮罩会晚一步出现")
		}
	}
}

// TestWarpScriptIsRawStringSafe 守一条踩过四次的坑。
//
// blogWarpScript 是 Go 原始字符串（反引号定界）。正文里出现任何一个反引号 ——
// 包括注释里为了排版而写的 —— 都会提前结束这个字符串，
// 编译时报 unexpected name after top level declaration。
func TestWarpScriptIsRawStringSafe(t *testing.T) {
	js := blogWarpJS()
	if i := strings.IndexByte(js, '`'); i >= 0 {
		lo := i - 60
		if lo < 0 {
			lo = 0
		}
		hi := i + 60
		if hi > len(js) {
			hi = len(js)
		}
		t.Fatalf("脚本正文第 %d 字节出现反引号，Go 原始字符串会被提前截断：%q", i, js[lo:hi])
	}
}
