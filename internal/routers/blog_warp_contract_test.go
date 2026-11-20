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

	// ④ 减少动效时不建遮罩，导航类链接原地跳转。
	//
	// 注意这里守的是「导航」而不是「全部」：正文内链接即使减少动效也仍然新标签打开，
	// 因为那条规则（不覆盖读者正在读的页面）与动画偏好无关。
	//
	// Note the guard is about navigation, not everything: a body link still opens its own tab
	// even under reduced motion, because that rule — do not replace the page being read — is
	// separate from a preference about animation.
	if !strings.Contains(js, "if (!newTab && STILL) { location.href = url; return; }") {
		t.Error("减少动效时导航没有退化成普通跳转")
	}
	if !strings.Contains(js, "if (STILL) { return; }") {
		t.Error("减少动效时仍然创建遮罩")
	}

	// ④b 正文内链接在播完后新标签打开，被拦截才退回同标签。
	//
	// 失败形态（改之前）：正文里的站内链接走 location.href，读者点一条引用就丢掉了正在读的
	// 那篇文章，滚动位置与播放进度全部归零。
	//
	// The failure this forbids: a body link used to navigate in place, so a reader who followed
	// one reference lost the article they were reading, along with its scroll position.
	if !strings.Contains(js, "function isReadLink") {
		t.Error("缺少正文链接判定：正文里的站内链接仍会覆盖当前页")
	}
	if !strings.Contains(js, "a.closest('.post-body')") {
		t.Error("正文链接判定没有限定在 .post-body 内")
	}
	if !strings.Contains(js, "function openTab") {
		t.Error("缺少新标签出口：播完后无法在新标签打开")
	}
	// openTab 只在 window.open 抛错时算失败 —— 返回值不可信。本站实测：内嵌浏览器开了新标签
	// 却返回 null 且焦点不动，与真实 Chrome「null = 被拦截」无法区分。把 null 当失败会连源页
	// 一起导航，读者既得一个标签又丢了文章。站主的规则「不要覆盖当前页」从这里推出：
	// 读不到结果时按已打开处理。
	//
	// openTab counts only a thrown error as failure; the return value cannot be trusted. Measured
	// on this site: an embedded browser opened the tab and returned null with no focus change,
	// indistinguishable from Chrome's "null = blocked". Treating null as failure navigates the
	// source page too. The owner's rule — never replace the page — follows from that: an unreadable
	// result counts as opened.
	if !strings.Contains(js, "catch (e) { return false; }") {
		t.Error("openTab 没有把抛错与被拦截区分开：缺少唯一可信的失败信号")
	}
	if strings.Contains(js, "return null;") {
		t.Error("openTab 仍返回 null 让调用方判断：实测无法与成功区分，会把源页一起导航")
	}
	if !strings.Contains(js, "function openThen") {
		t.Error("缺少 openThen：截断的判定散落在三处调用点")
	}
	// 且不能把 'noopener' 写进特性串 —— 那样成功也返回 null。
	//
	// And 'noopener' must not go into the feature string: then success also returns null.
	if strings.Contains(js, "'_blank', 'noopener'") {
		t.Error("noopener 写进了 window.open 特性串：成功时也返回 null，拦截无法判断")
	}
	// 新标签路径必须收尾遮罩，否则读者回来时看到的是一张盖住的页面。
	//
	// The new-tab path must finish the overlay, or a returning reader finds the page still
	// covered.
	if !strings.Contains(js, "departing = false;\n          finish();") {
		t.Error("新标签路径没有 finish()：遮罩会留在原处，后续点击被吞")
	}

	// ── 以下四条对应站主三次退回的具体失败 ──────────────────────────────

	// ⑤ 入站必须有动画：首访（没有「不要播放」标记）无条件 start()。
	//
	// 失败形态：首访没有标记 → 没有遮罩 → 读者先看白页再看站点。
	if !strings.Contains(js, "  } else {\n    start();\n  }") {
		t.Error("没有在文档一加载就调用 start()：入站不会有动画")
	}
	// 但经由过渡抵达的页面不播 —— 动画已经在出发页播完了。
	//
	// 失败形态：两页各播一遍，等待翻倍，限速下实测 2.9s + 2.9s，慢链路抵达那遍拉到 5s。
	if !strings.Contains(js, "KEY_QUIET") {
		t.Error("缺少「不要播放」标记：抵达页会重播一遍，等待翻倍")
	}
	if !strings.Contains(js, "if (quiet) { sessionStorage.removeItem(KEY_QUIET); }") {
		t.Error("标记不是一次性的：之后的首访也会被静音")
	}
	if strings.Contains(js, "if (arrived)") {
		t.Error("仍以 sessionStorage 标记作为创建遮罩的条件：首访没有动画")
	}

	// ⑥ 每一次导航播完整的一遍，且不继承相位。
	//
	// 失败形态：相位写进 sessionStorage 并在目标页累加，值越滚越大；
	// 实测第二次跳转读到 phase=12997，从 (12997 % 2900) ≈ 1400ms 处开始，557ms 就结束 ——
	// 站主原话「动画一闪而过」。现在时钟属于本文档，从零开始。
	if !strings.Contains(js, "var startedAt = 0") {
		t.Error("缺少每文档的起始时刻：动画会继承上一页的相位")
	}
	if strings.Contains(js, "fns:ink-phase") {
		t.Error("仍有跨文档相位交接：多次导航后动画会从半途开始")
	}
	// 播完整的一遍才起跳。
	if !strings.Contains(js, "elapsedNow() >= PASS_MS") {
		t.Error("起跳条件没有要求播完一整遍：笔会被从字母中间拽走")
	}
	// 且必须同时等到目标页落地（或播完后再给一小段宽限）。
	if !strings.Contains(js, "warmDestination(url).then(function () { ready = true; })") {
		t.Error("没有等目标页落地就起跳：下一页会在自己的遮罩里继续等")
	}
	if !strings.Contains(js, "GRACE_MS") {
		t.Error("缺少宽限期：慢封面会把「播完就进」变成「等封面加载完」")
	}
	// 出发时不能立刻导航 —— 那会把文档在笔画半途拆掉。
	if strings.Contains(js, "if (view) { setHint('LOADING'); }\n    location.href = url;") {
		t.Error("depart 仍立刻跳转：动画来不及播，只看到一闪")
	}

	// ⑦ 返回不是抵达：pageshow 的 persisted 分支直接显示，不重播。
	if !strings.Contains(js, "ev.persisted") {
		t.Error("pageshow 没有处理 bfcache 恢复：按返回会全白")
	}
	if !strings.Contains(js, "classList.remove('fns-warping', 'fns-arrived')") {
		t.Error("恢复时没有摘掉 fns-warping：返回得到一张白页")
	}
	// 返回后 departing 必须复位。
	//
	// 失败形态：离开时 departing=true 被 bfcache 原样带回；不复位时点击委托先
	// preventDefault，再进 depart() 会被 `if (departing) { return; }` 吞掉，
	// 页面上所有站内链接都「点了没反应」，刷新才恢复。
	// 只在 persisted 分支内找：文件开头的变量声明里也有 departing = false。
	persistedAt := strings.Index(js, "if (ev.persisted) {")
	if persistedAt < 0 {
		t.Error("找不到 pageshow 的 persisted 分支")
	} else {
		branch := js[persistedAt:]
		end := strings.Index(branch, "return;")
		if end < 0 {
			t.Error("persisted 分支没有 return")
		} else if !strings.Contains(branch[:end], "departing = false") {
			t.Error("bfcache 恢复分支没有复位 departing：返回后站内链接全部失效")
		}
	}

	// ⑧ 就绪判定要覆盖图片，而不只是文档。
	//
	// 失败形态：栏目页十几张封面每张约 700ms，只等文档会让读者看笔写完之后继续看转圈。
	if !strings.Contains(js, "function documentSettled") {
		t.Error("缺少就绪判定：遮罩会在内容到达之前抬起")
	}
	if !strings.Contains(js, "var imgs = document.images") {
		t.Error("没有等图片落定：封面还会在遮罩抬起之后继续加载")
	}
	if !strings.Contains(js, "warmDestination") {
		t.Error("缺少目标页预热：出发时不会预取下一屏的封面")
	}
	// 预热必须等图片，而不是只发起请求。
	if !strings.Contains(js, "im.onload = one") {
		t.Error("预热只发起请求不等完成：后续页仍会读条")
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
	// Each of the five reading templates must inject {{inkjs}} from its own <head>.
	// 五个阅读模板各自都要在自己的 <head> 里注入 {{inkjs}}。
	cases := []struct {
		name string
		src  string
	}{
		{"home/list/archive (blogChromePartials)", blogChromePartials},
		{"post", blogPostTmpl.Tree.Root.String()},
		{"page", blogPageTmpl.Tree.Root.String()},
		{"shelf", blogShelfPage},
		{"reader", blogReaderPage},
	}
	for _, c := range cases {
		// Match on the action name, not the full "{{inkjs}}": html/template rewrites a Tree in
		// place when it is first executed, turning the action into "{{inkjs | ...escaper}}". A
		// full-string match therefore passes when this test runs alone and fails as soon as any
		// other test has rendered the template — which is exactly what happened.
		// 匹配动作名而不是完整的 "{{inkjs}}"：html/template 在首次执行时会就地改写 Tree，
		// 把动作变成 "{{inkjs | ...escaper}}"。所以整串匹配在单独跑时通过，
		// 一旦别的测试渲染过这个模板就失败 —— 之前正是这样。
		if !strings.Contains(c.src, "{{inkjs") {
			t.Errorf("%s 的 head 没有注入 inkjs：这一页的入站动画会缺席", c.name)
		}
	}

	// The error page deliberately has none: a loading screen in front of a 404 is pure delay.
	// 错误页刻意没有：在 404 前面放加载画面是纯粹的拖延。
	if strings.Contains(blogErrorTmpl.Tree.Root.String(), "{{inkjs") {
		t.Error("错误页注入了 inkjs：404 前面会挡一层加载画面")
	}

	// And nothing may still inject WarpJS at the end of <body>, which is what made the overlay
	// arrive a beat after the page had already been painted.
	// 也不能再有任何模板在 body 末尾注入 WarpJS —— 那正是遮罩晚于页面绘制一步出现的原因。
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
