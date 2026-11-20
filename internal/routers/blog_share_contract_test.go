package routers

import (
	"strings"
	"testing"
)

// TestShareScriptMobileQuote pins the quote-share trigger, which was reported as "手机上完全不可用".
//
// The old implementation listened for mouseup only. A long-press selection on iOS and Android
// fires touch events and selectionchange, never mouseup, so the button never appeared — not
// intermittently, never. The guards below keep the three parts that make it work: a
// selectionchange listener (the cross-platform trigger), the pointerdown shield (without it the
// tap that follows a selection clears it and the bar vanishes before the button is hit), and a
// bar that is not a single button (the owner asked for a 微信-like row).
//
// TestShareScriptMobileQuote 锁住划词分享的触发，它被报告为「手机上完全不可用」。
//
// 旧实现只监听 mouseup。iOS 与安卓上的长按选词发出的是 touch 与 selectionchange，
// 从不发 mouseup，于是按钮永远不出现——不是偶发，是从不。下面的断言守住让它能用的三件事：
// selectionchange 监听（跨平台入口）、pointerdown 自保（没有它，选词之后那一次点按会清掉选区，
// 工具条在按钮被点到之前就消失了）、以及工具条不是单个按钮（站主要的是微信那样一排）。
func TestShareScriptMobileQuote(t *testing.T) {
	js := blogShareJS()

	if !strings.Contains(js, "document.addEventListener('selectionchange'") {
		t.Error("缺少 selectionchange 监听：手机长按选词不会触发工具条")
	}
	if !strings.Contains(js, "document.addEventListener('touchend'") {
		t.Error("缺少 touchend 补触发：部分浏览器手势结束时不再发 selectionchange")
	}
	if !strings.Contains(js, "qbar.addEventListener('pointerdown'") {
		t.Error("工具条没有在 pointerdown 上自保：点击前选区会被清掉，按钮点不到")
	}
	if !strings.Contains(js, "data-q=\"copy\"") {
		t.Error("工具条缺少「复制」项")
	}
	if !strings.Contains(js, "data-q=\"img\"") {
		t.Error("工具条缺少「分享图片」项")
	}
	// 工具条必须两端都出现，而不是只在手机 —— 站主选了「两端统一」。
	//
	// The bar must appear on both platforms, not only on phones — the owner chose one bar for both.
	if strings.Contains(js, "matchMedia('(pointer: coarse)')") {
		t.Error("工具条被限制为触摸设备：桌面端也要同一套")
	}
}

// TestShareScriptUserChoiceOfShare pins the iOS share fix.
//
// The owner's screenshot showed the card arriving in WeChat as a 2 MB file bubble, and the system
// share sheet offering almost nothing. Two causes: <a download> on iOS can only write to Files,
// and a PNG of that size is treated as a document. The fix is JPEG plus navigator.share, and the
// preview layer exists because navigator.share needs a fresh user gesture — the click that started
// the drawing is long expired by the time the covers have loaded.
//
// TestShareScriptUserChoiceOfShare 锁住 iOS 的分享修复。
//
// 站主的截图显示卡片在微信里变成一个 2 MB 的文件气泡，系统分享面板里几乎没有可选项。
// 两个原因：iOS 上的 <a download> 只能写进「文件」，而这么大的 PNG 被当作文档处理。
// 修法是 JPEG 加 navigator.share，而预览层的存在是因为 navigator.share 需要新鲜的用户手势——
// 等封面加载完时，发起绘制的那次点击早已过期。
func TestShareScriptUserChoiceOfShare(t *testing.T) {
	js := blogShareJS()

	if !strings.Contains(js, "'image/jpeg'") {
		t.Error("卡片没有导出为 JPEG：iOS 上仍会是体积巨大的文件卡")
	}
	if strings.Contains(js, "'image/png'") {
		t.Error("仍在导出 PNG：体积大且微信按文件处理")
	}
	if !strings.Contains(js, "navigator.share") {
		t.Error("没有调用系统分享：分享面板里看不到相册与隔空投送")
	}
	if !strings.Contains(js, "navigator.canShare") {
		t.Error("没有先用 canShare 探测文件分享能力：不支持的浏览器会静默失败")
	}
	if !strings.Contains(js, "fns-share-prev") {
		t.Error("缺少预览层：无法先看清卡片，也没有新鲜手势可供分享")
	}
	// 用户取消分享不算失败，不能顺手把图存下来。
	//
	// A cancelled share is not a failure; it must not silently save the file instead.
	if !strings.Contains(js, "AbortError") {
		t.Error("没有忽略用户取消：取消分享会被当成失败并自动保存")
	}
	// 不支持文件分享的浏览器要退回保存，而不是让按钮看起来没反应。
	//
	// Browsers without file sharing must fall back to saving, not to a dead button.
	if !strings.Contains(js, "doSave();") {
		t.Error("缺少不支持分享时的保存兜底")
	}
}

// TestMailPillIsCopyNotMailto pins the 邮箱 pill's contract.
//
// A reader reported that clicking 邮箱 launched Outlook — an app they had not opened in months,
// holding an unverified Gmail account — and that the verification flow froze their browser. The
// pill must never launch anything now: hover shows the address, click copies it.
//
// TestMailPillIsCopyNotMailto 锁住「邮箱」药丸的契约。
//
// 有读者反馈点「邮箱」会拉起 Outlook——一个几个月没打开、里面还绑着未验证 Gmail 的应用——
// 而验证流程把浏览器卡死了。这枚药丸现在不能启动任何程序：悬停显示地址，点击复制。
func TestMailPillIsCopyNotMailto(t *testing.T) {
	js := blogMailPillJS()

	if strings.Contains(js, "location.href") || strings.Contains(js, "window.open") {
		t.Error("邮箱药丸脚本里有导航：它只该复制，不该去任何地方")
	}
	if !strings.Contains(js, "navigator.clipboard") {
		t.Error("没有用 Clipboard API")
	}
	if !strings.Contains(js, "execCommand('copy')") {
		t.Error("缺少 execCommand 兜底：非安全上下文里 Clipboard API 不存在")
	}

	// The template must render the pill as a button carrying the address, and must still show the
	// address on hover — a phone has no hover, which is why the click is the primary action.
	//
	// The pill is a {{define}}d sub-template, so it is read by name: the root tree's String() does
	// not contain it, and asserting against the root would pass no matter what the pill said.
	//
	// 模板必须把这枚药丸渲染成带地址的按钮，并且仍然在悬停时显示地址——手机没有悬停，
	// 这正是点击才是主操作的原因。
	//
	// 药丸是一个 {{define}} 出来的子模板，因此按名字取：根树的 String() 里没有它，
	// 拿根树断言会不管药丸怎么写都通过。
	pill := blogCardTmpl.Lookup("pill")
	if pill == nil {
		t.Fatal("找不到 pill 子模板")
	}
	tmpl := pill.Tree.Root.String()
	if !strings.Contains(tmpl, "data-copy-pill") {
		t.Error("模板没有把药丸标成复制型")
	}
	if !strings.Contains(tmpl, "data-copy=") {
		t.Error("模板没有把地址交给复制按钮")
	}
	if !strings.Contains(tmpl, "pill-tip") {
		t.Error("模板没有渲染悬停气泡")
	}
	// A mail pill must not be an <a href="mailto:">: that is the exact element that handed the
	// click to the OS default client.
	//
	// 邮箱药丸不能是一个 mailto: 链接：那正是把点击交给系统默认客户端的那一行。
	if strings.Contains(tmpl, `href="{{.Href}}"{{if eq .Kind "mail"}}`) {
		t.Error("邮箱药丸仍是 mailto: 链接：点击会再次拉起系统邮件客户端")
	}
}
