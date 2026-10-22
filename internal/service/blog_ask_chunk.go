package service

import "strings"

// Chunking for the vector channel.
//
// Why chunk at all: a whole note embedded as one vector describes its dominant topic and
// nothing else. bge-m3 accepts 8192 tokens and rejects 12000 (measured: HTTP 400), so a long
// chapter cannot be embedded whole either. Splitting into ~1800-character passages gives each
// vector one topic, and lets the answer quote the paragraph that actually matched instead of
// a whole note.
//
// 为什么必须分块：整篇笔记压成一个向量时，它只描述最突出的那个主题，别的都被抹平；
// 而 bge-m3 接受 8192 token、12000 直接报错（实测 HTTP 400），长章节本来也塞不进去。
// 切成约 1800 字符的段落，每个向量描述一个主题，回答也能引用真正命中的那一段，
// 而不是整篇笔记。

// askChunk is one passage of a note.
// askChunk 是一篇笔记中的一个段落块。
type askChunk struct {
	NoteID int64
	Index  int    // position within the note, 0-based
	Text   string // the passage itself
	Start  int    // rune offset of the passage in the note's plain text
}

// askChunkText splits plain text into overlapping passages.
//
// Boundaries are chosen at paragraph breaks first, then at sentence punctuation, and only as a
// last resort mid-sentence — a passage that starts in the middle of a clause embeds poorly.
// The overlap exists so a sentence straddling a boundary is fully present in at least one
// chunk; without it, the passage that answers the question can be cut in half and match
// neither neighbour well.
//
// askChunkText 把纯文本切成互相重叠的段落。
//
// 边界优先取段落分隔，其次取句末标点，实在不行才在句中切——从句中间开始的段落嵌入质量很差。
// 重叠的存在是为了让跨边界的那句话至少完整出现在一个块里；否则真正回答问题的那段可能被
// 拦腰截断，两边都不匹配。
func askChunkText(text string, size, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if size <= 0 {
		size = 1800
	}
	if overlap < 0 || overlap >= size {
		overlap = 0
	}

	runes := []rune(text)
	if len(runes) <= size {
		return []string{string(runes)}
	}

	var out []string
	start := 0
	for start < len(runes) {
		end := start + size
		if end >= len(runes) {
			out = append(out, string(runes[start:]))
			break
		}

		// Search backwards for a natural boundary within the last 30% of the window. Cutting
		// early is better than cutting badly: the next chunk picks up the remainder.
		//
		// The window and the separators are both rune slices. strings.LastIndex would return a
		// byte offset, and adding that to a rune offset silently overshoots on any non-ASCII
		// text — a Chinese character is three bytes, so the computed cut ran past the end of
		// the buffer and the slice expression panicked, taking the whole process down with it.
		//
		// 在窗口后 30% 的范围内向前找自然边界。早切一点好过切得难看：剩下的内容由下一块承接。
		//
		// 窗口与分隔符都是 rune 切片。strings.LastIndex 返回的是字节偏移，把它加到 rune 偏移上，
		// 在任何非 ASCII 文本上都会静默越界——一个汉字占三字节，算出的 cut 会跑到缓冲区之外，
		// 切片表达式随即 panic，并连带整个进程一起退出。
		lowest := start + size*7/10
		cut := -1
		for _, sep := range []string{"\n\n", "\n", "。", "！", "？", "；", ". ", "! ", "? ", "; "} {
			sepRunes := []rune(sep)
			if i := askLastIndexRunes(runes[lowest:end], sepRunes); i >= 0 {
				if cand := lowest + i + len(sepRunes); cand > cut {
					cut = cand
				}
			}
		}
		if cut <= start {
			cut = end
		}
		if cut > len(runes) {
			cut = len(runes)
		}

		out = append(out, string(runes[start:cut]))
		if cut >= len(runes) {
			break
		}
		next := cut - overlap
		if next <= start {
			// Overlap must not stall the walk: always advance by at least one rune.
			// 重叠不能让推进停住：至少前进一个字符。
			next = start + 1
		}
		start = next
	}
	return out
}

// askLastIndexRunes returns the rune index of the last occurrence of sep within sub, or -1.
//
// This exists because strings.LastIndex works in bytes. Mixing its result with rune offsets is
// the defect that crashed the service: on Chinese text the byte offset of a separator is up to
// three times its rune offset, so the computed boundary landed past the end of the buffer.
//
// askLastIndexRunes 返回 sep 在 sub 中最后一次出现的 rune 索引，找不到返回 -1。
//
// 需要这个函数是因为 strings.LastIndex 按字节工作。把它的结果与 rune 偏移混用，
// 正是让服务崩溃的那个缺陷：在中文文本里，分隔符的字节偏移最多是 rune 偏移的三倍，
// 于是算出的边界落到了缓冲区之外。
func askLastIndexRunes(sub, sep []rune) int {
	if len(sep) == 0 || len(sep) > len(sub) {
		return -1
	}
	for i := len(sub) - len(sep); i >= 0; i-- {
		match := true
		for j := range sep {
			if sub[i+j] != sep[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// askBuildChunks chunks every note in the corpus.
// askBuildChunks 为语料中的每篇笔记分块。
func askBuildChunks(notes []*askNote, size, overlap int) []*askChunk {
	total := 0
	parts := make([][]string, len(notes))
	for i, n := range notes {
		parts[i] = askChunkText(n.Plain, size, overlap)
		total += len(parts[i])
	}

	out := make([]*askChunk, 0, total)
	for i, n := range notes {
		offset := 0
		for j, p := range parts[i] {
			out = append(out, &askChunk{
				NoteID: n.ID,
				Index:  j,
				Text:   p,
				Start:  offset,
			})
			offset += len([]rune(p))
		}
	}
	return out
}
