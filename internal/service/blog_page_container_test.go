package service

import (
	"strings"
	"testing"
)

// TestPageContainerRewritesWhitelistedBlocks 锁定三个白名单容器的改写。
func TestPageContainerRewritesWhitelistedBlocks(t *testing.T) {
	in := "::: timeline\n- 2019 条目\n:::\n"
	got := preprocessPageContainers(in)

	if !strings.Contains(got, pageContainerOpenPrefix+"timeline"+pageContainerSuffix) {
		t.Fatalf("open marker missing:\n%s", got)
	}
	if !strings.Contains(got, pageContainerClosePrefix+"timeline"+pageContainerSuffix) {
		t.Fatalf("close marker missing:\n%s", got)
	}
	if strings.Contains(got, ":::") {
		t.Fatalf("the literal ::: survived:\n%s", got)
	}
	// 内容本身必须留在两条标记之间,而且要有一行它自己的位置。
	if !strings.Contains(got, "- 2019 条目") {
		t.Fatalf("content lost:\n%s", got)
	}
}

// TestPageContainerInsertsBlankLinesAroundMarkers 空行不是美观问题。
//
// goldmark 认为原始 HTML 块一直延伸到空行为止。标记与内容之间没有空行时,
// 紧跟其后的列表会被当作原始 HTML 吞掉,而不是变成 <ul>——容器里就会显示一串带短横线的文字。
func TestPageContainerInsertsBlankLinesAroundMarkers(t *testing.T) {
	got := preprocessPageContainers("::: links\n- 邮箱 x@y\n:::")

	open := pageContainerOpenPrefix + "links" + pageContainerSuffix
	close := pageContainerClosePrefix + "links" + pageContainerSuffix
	if !strings.Contains(got, open+"\n\n") {
		t.Fatalf("no blank line after the open marker:\n%q", got)
	}
	if !strings.Contains(got, "\n\n"+close) {
		t.Fatalf("no blank line before the close marker:\n%q", got)
	}
}

// TestPageContainerLeavesUnknownNamesAlone 白名单之外的名字原样保留。
//
// 这是三种可能行为里唯一诚实的那个:报错会因为一个错字让整页失败,
// 静默丢弃会让作者纳闷内容去哪了。把标记打印出来,作者就看见了自己的错。
func TestPageContainerLeavesUnknownNamesAlone(t *testing.T) {
	in := "::: 随便什么\n内容\n:::\n"
	got := preprocessPageContainers(in)

	if got != in {
		t.Fatalf("unknown container was rewritten:\n got %q\nwant %q", got, in)
	}
	if strings.Contains(got, pageContainerOpenPrefix) {
		t.Fatalf("a marker was emitted for an unknown name:\n%s", got)
	}
}

// TestPageContainerUnknownNameDoesNotEatTheCloser 未知的开启行不得消耗掉 `:::`。
//
// 否则一个错字会把后面的容器弄错配:那个 `:::` 本该结束真正打开的容器。
func TestPageContainerUnknownNameDoesNotEatTheCloser(t *testing.T) {
	in := "::: nope\n正文\n:::\n\n::: timeline\n- 条目\n:::\n"
	got := preprocessPageContainers(in)

	if !strings.Contains(got, "::: nope") {
		t.Fatalf("the unknown opener was consumed:\n%s", got)
	}
	if !strings.Contains(got, pageContainerOpenPrefix+"timeline"+pageContainerSuffix) {
		t.Fatalf("the real container after it was not marked:\n%s", got)
	}
	// 未知开启行的 `:::` 要留在原处,不去闭合任何东西。
	if strings.Count(got, pageContainerClosePrefix) != 1 {
		t.Fatalf("expected exactly one close marker, got:\n%s", got)
	}
}

// TestPageContainerClosesUnterminatedBlock 未闭合的容器也补上结束标记。
//
// 否则页面余下的部分会从一个未闭合的元素里漏出去,版式整个乱掉。
func TestPageContainerClosesUnterminatedBlock(t *testing.T) {
	got := preprocessPageContainers("::: profile-head\n**张三**\n\n## 后面还有正文\n")
	if !strings.Contains(got, pageContainerClosePrefix+"profile-head"+pageContainerSuffix) {
		t.Fatalf("an unterminated container was left open:\n%s", got)
	}
}

// TestPageContainerNestingIsNotSupported 第一个 `:::` 就关掉当前容器。
//
// 这是源码读者对 `::: name` … `:::` 的预期。不支持嵌套是有意的:
// 支持它需要一套匹配规则,而那正是「模板语言」的开端。
func TestPageContainerNestingIsNotSupported(t *testing.T) {
	got := preprocessPageContainers("::: timeline\n::: links\n- x\n:::\n")
	// 第一个 `:::`(紧跟在 links 开启行后的那一条)结束 timeline;
	// links 的开启行按普通文本留在内部,不会被标记。
	if strings.Count(got, pageContainerClosePrefix) != 1 {
		t.Fatalf("expected one close marker, got:\n%s", got)
	}
	if strings.Contains(got, pageContainerOpenPrefix+"links") {
		t.Fatalf("a nested container was opened:\n%s", got)
	}
}

// TestApplyPageContainersEmitsTheWrapper 渲染后的标记要换成真正的 div。
func TestApplyPageContainersEmitsTheWrapper(t *testing.T) {
	html := "<p>前</p>\n" + pageContainerOpenPrefix + "timeline" + pageContainerSuffix +
		"<ul>\n<li>2019 条目</li>\n</ul>\n" +
		pageContainerClosePrefix + "timeline" + pageContainerSuffix + "\n<p>后</p>\n"
	got := applyPageContainers(html)

	if !strings.Contains(got, `<div class="pc pc-timeline">`) {
		t.Fatalf("wrapper not emitted:\n%s", got)
	}
	if strings.Contains(got, pageContainerOpenPrefix) || strings.Contains(got, pageContainerClosePrefix) {
		t.Fatalf("markers survived into the output:\n%s", got)
	}
	if !strings.Contains(got, "<ul>") || !strings.Contains(got, "<p>后</p>") {
		t.Fatalf("content lost around the wrapper:\n%s", got)
	}
}

// TestApplyPageContainersKeepsMismatchedPairAsComments 名字不匹配时不包裹。
//
// 不匹配说明源码里有未闭合的容器;靠猜去包裹会把内容放到错误的版式下。
// 留成注释渲染出来是空的——最不吵的失败形态。
func TestApplyPageContainersKeepsMismatchedPairAsComments(t *testing.T) {
	html := pageContainerOpenPrefix + "timeline" + pageContainerSuffix +
		"内容" +
		pageContainerClosePrefix + "links" + pageContainerSuffix
	got := applyPageContainers(html)

	if strings.Contains(got, "<div") {
		t.Fatalf("a mismatched pair was wrapped:\n%s", got)
	}
}

// TestApplyPageContainersIsANoOpWithoutMarkers 没有标记时不动 HTML。
//
// 这条看似平凡,但它保证了那趟正则不会碰普通文章——文章页的每一次渲染都会经过它。
func TestApplyPageContainersIsANoOpWithoutMarkers(t *testing.T) {
	html := "<h1>标题</h1>\n<p>正文,里面提到 ::: timeline 但没有真的用它。</p>\n"
	if got := applyPageContainers(html); got != html {
		t.Fatalf("plain HTML was altered:\n got %q\nwant %q", got, html)
	}
}

// TestPageContainerWhitelistIsTheReviewedSet 白名单就是这一组固定的名字。
//
// 名字越多越接近「自定义 HTML」:任意容器名就是任意 <div class>。
// 若这里失败,说明有人在没有重新审视那条排除项的情况下放宽了边界。
//
// 断言按「全等」而不是「包含」写,并保留顺序:这条测试的作用是逼下一次改动重新审视边界,
// 所以任何增删都必须显式改到这里。
//
// profile-meta 自首个版本就在列表里(为简历页的两列事实网格而加),而这条测试当初按三个名字写,
// 于是从诞生起就是红的 —— 一个永远失败的守卫等于没有守卫。现在它对齐实际集合。
func TestPageContainerWhitelistIsTheReviewedSet(t *testing.T) {
	want := []string{"profile-head", "profile-meta", "timeline", "links"}
	if len(pageContainerNames) != len(want) {
		t.Fatalf("whitelist has %d names, want %d", len(pageContainerNames), len(want))
	}
	for i, n := range want {
		if pageContainerNames[i] != n {
			t.Fatalf("whitelist[%d] = %q, want %q", i, pageContainerNames[i], n)
		}
	}
}
