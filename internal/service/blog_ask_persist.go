package service

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Persistence for the vector index.
//
// Why this is worth a file at all: building the index costs one embedding call per batch —
// measured at roughly 15 round trips and 10–20 seconds for this vault. Without persistence that
// cost is paid again after every restart, and until it finishes the Q&A entry silently runs on
// BM25 alone, which is the weakest of the three channels.
//
// The file holds vectors and nothing else. Chunk text is not stored: it is a deterministic
// function of the note bodies, which the corpus already has in memory, so re-cutting them at
// load time costs milliseconds and saves ~2.4 MB of duplication. That trade is only sound
// because askChunkText is deterministic, which TestAskChunkTextDeterminism pins down.
//
// 向量索引的持久化。
//
// 为什么值得一个文件：建索引的代价是每批一次嵌入调用——本仓库实测约 15 次往返、10–20 秒。
// 没有持久化，每次重启都要重付一次，而在它完成之前问答入口会静默地只用 BM25，
// 也就是三条通道里最弱的那条。
//
// 文件只装向量，不装块文本：块是笔记正文的确定性函数，而正文语料已在内存中，
// 因此加载时重新切分只要几毫秒，还省掉约 2.4 MB 的重复存储。
// 这个取舍成立的前提是 askChunkText 的确定性，由 TestAskChunkTextDeterminism 守住。

const (
	askVectorFileMagic   = "FNSV"
	askVectorFileVersion = 1
	// Header: magic(4) + version(1) + dim(4) + count(4) + fpLen(4) = 17 bytes.
	// 头部：magic(4) + version(1) + dim(4) + count(4) + fpLen(4) = 17 字节。
	askVectorHeaderBytes = 4 + 1 + 4 + 4 + 4
	// askVectorMaxFileBytes bounds what will be read back. 460 chunks × 1024 dims × 4 B is
	// ~1.9 MB; the ceiling leaves room for a vault several times this size while refusing a
	// file that is obviously not ours or has been corrupted into a huge length field.
	// askVectorMaxFileBytes 限制可读回的文件大小。460 块 × 1024 维 × 4 B 约 1.9 MB；
	// 上限为仓库规模数倍于此留出空间，同时拒绝那些显然不属于我们、
	// 或长度字段已被破坏成巨大值的文件。
	askVectorMaxFileBytes = 256 << 20 // 256 MiB
)

// askVectorPath returns the on-disk location for one user's vectors, or "" when the blog has
// no writable data directory (in which case persistence is simply skipped).
//
// askVectorPath 返回某个用户向量文件的落盘位置；博客没有可写数据目录时返回空串
// （此时直接跳过持久化）。
func (s *shareService) askVectorPath(uid int64) string {
	dir := blogDataDir(s.config)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "ask", "u_"+strconv.FormatInt(uid, 10)+".vec")
}

// saveAskVectorIndex writes the index atomically.
//
// The write goes to a temporary file in the same directory and is then renamed over the target.
// A crash mid-write would otherwise leave a truncated file that the next start would read as a
// valid-looking header with missing vectors — the worst possible outcome, because it would
// silently degrade every answer instead of failing loudly and rebuilding.
//
// saveAskVectorIndex 原子地写入索引。
//
// 先写到同目录的临时文件，再改名覆盖目标。否则写到一半崩溃会留下一个被截断的文件，
// 下次启动会把它读成「头部合法但向量缺失」——这是最糟的结果，
// 因为它会静默地让每个回答变差，而不是明确失败并重建。
func saveAskVectorIndex(path string, idx *askVectorIndex) error {
	if path == "" || idx == nil || idx.dim <= 0 {
		return nil
	}
	count := len(idx.chunks)
	if count == 0 {
		return nil
	}
	if len(idx.vectors) != count*idx.dim {
		return fmt.Errorf("vector length %d does not match %d chunks × %d dims",
			len(idx.vectors), count, idx.dim)
	}

	fp := []byte(idx.fingerprint)
	buf := make([]byte, askVectorHeaderBytes+len(fp)+len(idx.vectors)*4)
	copy(buf[0:4], askVectorFileMagic)
	buf[4] = askVectorFileVersion
	binary.LittleEndian.PutUint32(buf[5:9], uint32(idx.dim))
	binary.LittleEndian.PutUint32(buf[9:13], uint32(count))
	binary.LittleEndian.PutUint32(buf[13:17], uint32(len(fp)))
	copy(buf[askVectorHeaderBytes:], fp)

	off := askVectorHeaderBytes + len(fp)
	for _, v := range idx.vectors {
		binary.LittleEndian.PutUint32(buf[off:off+4], math.Float32bits(v))
		off += 4
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loadAskVectorIndex reads a persisted index and reattaches it to freshly cut chunks.
//
// The index is accepted only when the fingerprint matches the current corpus AND the chunk
// count the vectors were built from equals the count produced now. The fingerprint alone
// catches edits; the count check catches the narrower case where the corpus is byte-identical
// but the chunking parameters in config were changed, which would silently misalign every
// vector against a different passage.
//
// loadAskVectorIndex 读取持久化索引，并把它重新挂到新切出的块上。
//
// 只有当指纹与当前语料一致、且向量构建时的块数与现在切出的块数相等时才接受。
// 指纹负责发现编辑；块数校验负责更窄的一种情形——语料字节完全相同、但配置里的分块参数变了，
// 那会让每个向量都静默地对错段落。
func (s *shareService) loadAskVectorIndex(
	path string, notes []*askNote, cfg *AskEmbeddingConfig, wantFingerprint string,
) *askVectorIndex {
	if path == "" {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil || info.Size() <= askVectorHeaderBytes || info.Size() > askVectorMaxFileBytes {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) <= askVectorHeaderBytes {
		return nil
	}
	if string(raw[0:4]) != askVectorFileMagic {
		return nil
	}
	if raw[4] != askVectorFileVersion {
		return nil
	}

	dim := int(binary.LittleEndian.Uint32(raw[5:9]))
	count := int(binary.LittleEndian.Uint32(raw[9:13]))
	fpLen := int(binary.LittleEndian.Uint32(raw[13:17]))
	if dim <= 0 || count <= 0 || fpLen < 0 {
		return nil
	}
	headerEnd := askVectorHeaderBytes + fpLen
	if headerEnd > len(raw) {
		return nil
	}
	if string(raw[askVectorHeaderBytes:headerEnd]) != wantFingerprint {
		// The corpus changed since the vectors were written. Rebuilding is the only correct
		// response; using them would attach old embeddings to new text.
		// 语料自写入向量后已改变。重建是唯一正确的应对；继续使用会把旧嵌入挂到新文本上。
		return nil
	}
	if want := headerEnd + count*dim*4; want != len(raw) {
		// A truncated or extended file. Both mean the contents cannot be trusted.
		// 文件被截断或被追加。两者都意味着内容不可信。
		return nil
	}

	chunks := askBuildChunks(notes, askChunkSize(cfg), askChunkOverlap(cfg))
	if len(chunks) != count {
		// Same notes, different chunking parameters. Every vector would point at the wrong
		// passage, so this is treated as a miss rather than a partial load.
		// 笔记相同但分块参数不同。每个向量都会指向错误的段落，
		// 因此按未命中处理，而不是部分加载。
		return nil
	}

	vectors := make([]float32, count*dim)
	off := headerEnd
	for i := range vectors {
		vectors[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[off : off+4]))
		off += 4
	}

	norms := make([]float32, count)
	for i := 0; i < count; i++ {
		norms[i] = askNorm(vectors[i*dim : (i+1)*dim])
	}

	return &askVectorIndex{
		chunks:      chunks,
		vectors:     vectors,
		norms:       norms,
		dim:         dim,
		built:       time.Now(),
		fingerprint: wantFingerprint,
	}
}

// askChunkSize and askChunkOverlap resolve the configured chunking parameters with defaults,
// so the build and the load can never disagree about them.
// askChunkSize 与 askChunkOverlap 解析配置的分块参数并套用默认值，
// 使构建与加载两侧不可能对它们产生分歧。
func askChunkSize(cfg *AskEmbeddingConfig) int {
	if cfg != nil && cfg.ChunkChars > 0 {
		return cfg.ChunkChars
	}
	return 1800
}

func askChunkOverlap(cfg *AskEmbeddingConfig) int {
	if cfg != nil && cfg.ChunkOverlap >= 0 {
		return cfg.ChunkOverlap
	}
	return 200
}
