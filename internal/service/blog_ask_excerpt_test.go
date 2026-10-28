package service

import (
	"strings"
	"testing"
)

// TestAskExcerptSpreadsAcrossSections 锁住一个真实翻车场景。
//
// 2026-10-28：问「拔牙的并发症有哪些」，《第4章：牙及牙槽外科》21153 字。
// 「并发症」在适应证章节出现过（讲哪些情况不宜拔牙），而真正的并发症正文
// 散在 2000~7000 字与文末。旧实现只取**最靠前**的命中点截 1200 字，
// 窗口落在适应证段，模型只看到一句"拔牙造成菌血症 50%~80%"，
// 于是如实回答"其余没有相关内容"——它没说错，是我们只给了它那一小段。
//
// 这条测试要保证：关键词出现多处时，摘录必须覆盖多个位置，而不是只有开头。
func TestAskExcerptSpreadsAcrossSections(t *testing.T) {
	// 复刻真实笔记的分布：命中点之间隔数千字（那篇笔记 21153 字，
	// 「出血」在 2321、「干槽症」在 6603、文末还有考点清单）。
	// 分隔段必须比窗口宽，否则几个窗口会挤在同一段里，测不出"跨章节覆盖"。
	var b strings.Builder
	// 开头这句在真实笔记里是「拔牙造成菌血症发生率50%~80%」，
	// 而「拔牙」是查询词之一 —— 所以它确实是一个合法命中点。
	b.WriteString("一、适应证与并发症概述 拔牙造成菌血症发生率50%~80%。")
	b.WriteString(strings.Repeat("无关的适应证内容。", 400)) // ≈4000 字
	b.WriteString("七、并发症及处理 术后出血是最常见并发症。干槽症多发生于术后3-4天。")
	b.WriteString(strings.Repeat("无关的护理内容。", 400))
	b.WriteString("术中并发症包括牙根折断、断根进入上颌窦、软组织损伤。")

	full := b.String()
	tokens := []string{"并发症", "出血", "干槽症"}
	got := askExcerpt(full, tokens, 1200)

	if n := len([]rune(got)); n > 1200*2 {
		t.Errorf("摘录明显超出预算：%d 字（预算 1200）", n)
	}
	for _, want := range []string{"菌血症", "干槽症", "牙根折断"} {
		if !strings.Contains(got, want) {
			t.Errorf("摘录应覆盖多处命中，缺少 %q", want)
		}
	}
	// 多个不连续窗口之间要有省略标记，否则模型不知道这里有跳段。
	if !strings.Contains(got, "\n…\n") {
		t.Errorf("多窗口之间缺少省略标记")
	}
}

// TestAskExcerptNoHitsFallsBack 无命中时回退到开头，且不 panic（rune 安全的旧 bug 回归）。
func TestAskExcerptNoHitsFallsBack(t *testing.T) {
	full := strings.Repeat("中文内容。", 500)
	got := askExcerpt(full, []string{"完全不存在的词"}, 600)
	if got == "" {
		t.Fatal("无命中时也应返回可用的开头片段")
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("被截断时应带省略号：%q", got[:20])
	}
}

// TestAskExcerptShortTextUnchanged 短于预算的笔记原样返回，不加省略号。
func TestAskExcerptShortTextUnchanged(t *testing.T) {
	full := "短笔记，没有超过预算。"
	if got := askExcerpt(full, []string{"笔记"}, 1200); got != full {
		t.Errorf("短文本应原样返回，得到 %q", got)
	}
}

// TestAskExcerptBudgetRespected 极端预算下也不越界。
func TestAskExcerptBudgetRespected(t *testing.T) {
	full := strings.Repeat("并发症与出血。", 400)
	for _, budget := range []int{200, 600, 1200, 2600} {
		got := askExcerpt(full, []string{"并发症", "出血"}, budget)
		if n := len([]rune(got)); n > budget*3 {
			t.Errorf("预算 %d 下摘录 %d 字，明显越界", budget, n)
		}
	}
}
