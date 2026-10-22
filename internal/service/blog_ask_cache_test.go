package service

import (
	"testing"
)

// TestSameIDSet covers the revalidation primitive the corpus cache depends on.
//
// A false "same" here means a revoked note keeps being quotable for the rest of the TTL, so
// the comparison must be exact on membership and count, and must not depend on order — the
// repository does not promise one.
//
// TestSameIDSet 覆盖语料缓存所依赖的校验原语。
//
// 这里若误判为「相同」，被取消分享的笔记就在剩余 TTL 内继续可被引用，
// 因此比较必须在成员与数量上都精确，且不能依赖顺序——仓库并不承诺顺序。
func TestSameIDSet(t *testing.T) {
	cases := []struct {
		name string
		a, b []int64
		want bool
	}{
		{"完全相同", []int64{1, 2, 3}, []int64{1, 2, 3}, true},
		{"顺序不同", []int64{3, 1, 2}, []int64{1, 2, 3}, true},
		{"两边皆空", nil, []int64{}, true},
		{"少一个（取消分享）", []int64{1, 2, 3}, []int64{1, 2}, false},
		{"多一个（新增分享）", []int64{1, 2}, []int64{1, 2, 3}, false},
		{"同数量但不同成员", []int64{1, 2, 3}, []int64{1, 2, 4}, false},
		{"一方为空", []int64{1}, nil, false},
		{"重复元素不等价于缺失", []int64{1, 1, 2}, []int64{1, 2, 2}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameIDSet(tc.a, tc.b); got != tc.want {
				t.Fatalf("sameIDSet(%v, %v) = %v, 期望 %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestAskChunkTextDeterminism guards the assumption that lets the vector index persist only
// its vectors.
//
// Chunks are rebuilt from the note text at load time rather than stored, which is only sound if
// chunking the same input twice yields the same passages in the same order. If it drifted, a
// persisted vector would attach to the wrong passage and the answer would quote text the
// question never matched.
//
// TestAskChunkTextDeterminism 守住「向量索引只持久化向量」所依赖的前提。
//
// 块在加载时从笔记正文重新切分而不是存下来，这只有在「同一输入两次切分得到相同段落与
// 相同顺序」时才成立。一旦漂移，持久化的向量就会挂到错误的段落上，
// 回答会引用问题从未匹配到的文字。
func TestAskChunkTextDeterminism(t *testing.T) {
	text := "牙釉质由釉柱和柱间质构成。\n\n牙本质含牙本质小管。\n\n牙髓含血管与神经。"
	first := askChunkText(text, 40, 10)
	if len(first) == 0 {
		t.Fatal("切分结果为空")
	}
	for i := 0; i < 20; i++ {
		again := askChunkText(text, 40, 10)
		if len(again) != len(first) {
			t.Fatalf("第 %d 次块数变化: %d != %d", i, len(again), len(first))
		}
		for j := range again {
			if again[j] != first[j] {
				t.Fatalf("第 %d 次第 %d 块内容漂移", i, j)
			}
		}
	}
}
