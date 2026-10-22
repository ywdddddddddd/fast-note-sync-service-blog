package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildPersistFixture returns a saved index plus the inputs needed to load it back.
// buildPersistFixture 返回一个已保存的索引，以及把它读回来所需的输入。
func buildPersistFixture(t *testing.T, dir string, dim int) (string, []*askNote, *AskEmbeddingConfig, string, *askVectorIndex) {
	t.Helper()

	notes := []*askNote{{
		ID:          7,
		Path:        "笔记/测试.md",
		Title:       "测试",
		Plain:       strings.Repeat("牙釉质由釉柱和柱间质构成。", 120),
		ContentHash: "deadbeef",
	}}
	cfg := &AskEmbeddingConfig{ChunkChars: 120, ChunkOverlap: 20}
	chunks := askBuildChunks(notes, cfg.ChunkChars, cfg.ChunkOverlap)
	if len(chunks) < 2 {
		t.Fatalf("fixture 应产生多块，实得 %d", len(chunks))
	}

	vectors := make([]float32, len(chunks)*dim)
	for i := range vectors {
		vectors[i] = float32(i%17) * 0.125
	}
	norms := make([]float32, len(chunks))
	for i := 0; i < len(chunks); i++ {
		norms[i] = askNorm(vectors[i*dim : (i+1)*dim])
	}

	fp := "fp-test-123"
	idx := &askVectorIndex{
		chunks:      chunks,
		vectors:     vectors,
		norms:       norms,
		dim:         dim,
		fingerprint: fp,
	}
	path := filepath.Join(dir, "u_1.vec")
	if err := saveAskVectorIndex(path, idx); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	return path, notes, cfg, fp, idx
}

// TestAskVectorPersistenceRoundTrip is the core assertion: what was written comes back
// bit-identical and still points at the same passages.
//
// TestAskVectorPersistenceRoundTrip 是核心断言：写下去的东西原样回来，
// 并且仍然指向同一批段落。
func TestAskVectorPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path, notes, cfg, fp, want := buildPersistFixture(t, dir, 16)

	got := (&shareService{}).loadAskVectorIndex(path, notes, cfg, fp)
	if got == nil {
		t.Fatal("加载返回 nil")
	}
	if got.dim != want.dim {
		t.Fatalf("dim = %d, 期望 %d", got.dim, want.dim)
	}
	if len(got.chunks) != len(want.chunks) {
		t.Fatalf("块数 = %d, 期望 %d", len(got.chunks), len(want.chunks))
	}
	if len(got.vectors) != len(want.vectors) {
		t.Fatalf("向量长度 = %d, 期望 %d", len(got.vectors), len(want.vectors))
	}
	for i := range want.vectors {
		if got.vectors[i] != want.vectors[i] {
			t.Fatalf("第 %d 个分量 = %v, 期望 %v", i, got.vectors[i], want.vectors[i])
		}
	}
	for i := range want.chunks {
		if got.chunks[i].Text != want.chunks[i].Text {
			t.Fatalf("第 %d 块文本不一致", i)
		}
	}
	// Norms are recomputed at load time, so they must match what the build computed.
	// 模长在加载时重算，因此必须与构建时算出的值一致。
	for i := range want.norms {
		if diff := got.norms[i] - want.norms[i]; diff > 1e-6 || diff < -1e-6 {
			t.Fatalf("第 %d 个模长 = %v, 期望 %v", i, got.norms[i], want.norms[i])
		}
	}
}

// TestAskVectorPersistenceRejects covers every way a persisted index can be wrong.
//
// Each of these must return nil rather than a partial index: a misaligned vector silently
// attaches an embedding to the wrong passage, which degrades every answer without ever
// looking like a failure.
//
// TestAskVectorPersistenceRejects 覆盖持久化索引可能出错的每一种方式。
//
// 每一种都必须返回 nil 而不是部分索引：错位的向量会把嵌入静默挂到错误的段落上，
// 让每个回答都变差，却永远不会表现得像一次失败。
func TestAskVectorPersistenceRejects(t *testing.T) {
	t.Run("指纹不匹配", func(t *testing.T) {
		dir := t.TempDir()
		path, notes, cfg, _, _ := buildPersistFixture(t, dir, 8)
		if got := (&shareService{}).loadAskVectorIndex(path, notes, cfg, "different-fp"); got != nil {
			t.Fatal("语料已变时应返回 nil")
		}
	})

	t.Run("分块参数变化导致块数不符", func(t *testing.T) {
		dir := t.TempDir()
		path, notes, _, fp, _ := buildPersistFixture(t, dir, 8)
		// Same notes and fingerprint, but a much larger chunk size produces fewer chunks.
		// 笔记与指纹相同，但大得多的块尺寸会产生更少的块。
		otherCfg := &AskEmbeddingConfig{ChunkChars: 100000, ChunkOverlap: 0}
		if got := (&shareService{}).loadAskVectorIndex(path, notes, otherCfg, fp); got != nil {
			t.Fatal("块数不符时应返回 nil")
		}
	})

	t.Run("文件被截断", func(t *testing.T) {
		dir := t.TempDir()
		path, notes, cfg, fp, _ := buildPersistFixture(t, dir, 8)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw[:len(raw)-8], 0o644); err != nil {
			t.Fatal(err)
		}
		if got := (&shareService{}).loadAskVectorIndex(path, notes, cfg, fp); got != nil {
			t.Fatal("文件被截断时应返回 nil")
		}
	})

	t.Run("magic 不符", func(t *testing.T) {
		dir := t.TempDir()
		path, notes, cfg, fp, _ := buildPersistFixture(t, dir, 8)
		raw, _ := os.ReadFile(path)
		raw[0] = 'X'
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := (&shareService{}).loadAskVectorIndex(path, notes, cfg, fp); got != nil {
			t.Fatal("magic 不符时应返回 nil")
		}
	})

	t.Run("文件不存在", func(t *testing.T) {
		dir := t.TempDir()
		if got := (&shareService{}).loadAskVectorIndex(
			filepath.Join(dir, "missing.vec"), nil, nil, "fp"); got != nil {
			t.Fatal("文件不存在时应返回 nil")
		}
	})

	t.Run("空路径", func(t *testing.T) {
		if got := (&shareService{}).loadAskVectorIndex("", nil, nil, "fp"); got != nil {
			t.Fatal("空路径时应返回 nil")
		}
	})
}

// TestSaveAskVectorIndexRejectsMismatchedLength makes sure a corrupt in-memory index never
// reaches the disk, where it would outlive the process that could have rebuilt it.
//
// TestSaveAskVectorIndexRejectsMismatchedLength 确保损坏的内存索引不会被写到磁盘上——
// 一旦落盘，它会比那个本可以重建它的进程活得更久。
func TestSaveAskVectorIndexRejectsMismatchedLength(t *testing.T) {
	dir := t.TempDir()
	idx := &askVectorIndex{
		chunks:      []*askChunk{{NoteID: 1}, {NoteID: 2}},
		vectors:     []float32{1, 2, 3}, // should be 2*dim
		dim:         4,
		fingerprint: "fp",
	}
	if err := saveAskVectorIndex(filepath.Join(dir, "x.vec"), idx); err == nil {
		t.Fatal("长度不匹配时应报错")
	}
}
