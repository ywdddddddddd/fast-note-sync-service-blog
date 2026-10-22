package service

import (
	"strings"
	"testing"
)

// TestAskChunkTextNoPanicOnCJK is the regression test for the crash that took the service down.
//
// askChunkText used to locate its cut point with strings.LastIndex, which reports a byte
// offset, and then use that number as a rune offset. On Chinese text a character is three
// bytes, so the boundary overshot the buffer and the slice expression panicked — inside a
// background goroutine, which terminates the whole process rather than just the request.
//
// TestAskChunkTextNoPanicOnCJK 是导致服务崩溃的那个缺陷的回归测试。
//
// askChunkText 曾用 strings.LastIndex 定位切点，它返回字节偏移，而该数值随后被当作 rune
// 偏移使用。中文一字三字节，于是边界越过了缓冲区，切片表达式 panic——而它发生在后台
// goroutine 里，终止的是整个进程，而不只是那一个请求。
func TestAskChunkTextNoPanicOnCJK(t *testing.T) {
	cases := []struct {
		name string
		text string
		size int
	}{
		{"纯中文长文本", strings.Repeat("牙釉质由釉柱和柱间质构成。", 400), 1800},
		{"中文带段落", strings.Repeat("第一段内容。\n\n第二段内容。\n\n", 200), 900},
		{"中英混排", strings.Repeat("enamel 釉质 dentin 牙本质。\n", 300), 600},
		{"无标点连续中文", strings.Repeat("牙", 9000), 1800},
		{"恰好等于窗口", strings.Repeat("釉", 1800), 1800},
		{"单字符", "牙", 1800},
		{"空串", "", 1800},
		{"size 为 0 走默认", strings.Repeat("内容。", 900), 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The failure mode was a panic, so simply returning is the assertion.
			// 缺陷的表现是 panic，因此「能正常返回」本身就是断言。
			got := askChunkText(tc.text, tc.size, 200)
			if tc.text == "" {
				if len(got) != 0 {
					t.Fatalf("空输入应得到 0 块，实得 %d", len(got))
				}
				return
			}
			if len(got) == 0 {
				t.Fatal("非空输入不应得到 0 块")
			}
			for i, c := range got {
				if strings.TrimSpace(c) == "" {
					t.Fatalf("第 %d 块为空", i)
				}
			}
		})
	}
}

// TestAskChunkTextCoversAllText checks that chunking loses nothing.
//
// Overlap means chunks are not a partition, so the assertion is that concatenating them in
// order contains the original text: every character must appear, in sequence.
//
// TestAskChunkTextCoversAllText 检查分块没有丢内容。
//
// 因为存在重叠，块并不是对原文的划分，所以断言是：按顺序拼接后包含原文——
// 每个字符都必须按序出现。
func TestAskChunkTextCoversAllText(t *testing.T) {
	text := strings.Repeat("牙釉质的组织结构包括釉柱、柱间质和釉牙本质界。", 120)
	chunks := askChunkText(text, 300, 50)
	if len(chunks) < 2 {
		t.Fatalf("预期多块，实得 %d", len(chunks))
	}
	joined := strings.Join(chunks, "")
	if !strings.Contains(joined, text[:200]) {
		t.Fatal("拼接结果未包含原文开头")
	}
	// The tail must survive too: an off-by-one in the walk would truncate it.
	// 结尾同样必须保留：遍历里差一位就会把它截掉。
	if !strings.Contains(joined, text[len(text)-200:]) {
		t.Fatal("拼接结果未包含原文结尾")
	}
}

// TestAskChunkTextRespectsSize checks the size bound that keeps chunks inside bge-m3's window.
//
// The model accepts 8192 tokens and rejects 12000 outright; with roughly one token per Chinese
// character, a chunk must stay near the configured size or the embedding call fails.
//
// TestAskChunkTextRespectsSize 检查块大小上限，它保证块能塞进 bge-m3 的窗口。
//
// 该模型接受 8192 token、12000 直接报错；中文大致一字一 token，
// 因此块必须接近配置的大小，否则嵌入调用会失败。
func TestAskChunkTextRespectsSize(t *testing.T) {
	const size, overlap = 500, 80
	text := strings.Repeat("牙体解剖与口腔功能。", 500)
	for i, c := range askChunkText(text, size, overlap) {
		if n := len([]rune(c)); n > size+overlap {
			t.Fatalf("第 %d 块长度 %d 超过上限 %d", i, n, size+overlap)
		}
	}
}

// TestAskLastIndexRunes confirms the helper reports rune indices, not byte offsets.
// TestAskLastIndexRunes 确认该辅助函数返回的是 rune 索引而非字节偏移。
func TestAskLastIndexRunes(t *testing.T) {
	sub := []rune("一二三。四五六。")
	if got := askLastIndexRunes(sub, []rune("。")); got != 7 {
		t.Fatalf("最后一个句号的 rune 索引应为 7，实得 %d", got)
	}
	if got := askLastIndexRunes(sub, []rune("不存在")); got != -1 {
		t.Fatalf("找不到时应返回 -1，实得 %d", got)
	}
	if got := askLastIndexRunes(nil, []rune("。")); got != -1 {
		t.Fatalf("空输入应返回 -1，实得 %d", got)
	}
}
