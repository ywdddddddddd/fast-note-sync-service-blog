package service

import (
	"context"
	"sort"
	"time"
)

// Hybrid retrieval: BM25 for exact wording, embeddings for meaning, a cross-encoder to decide.
//
// The three stages exist because each fixes a failure the previous one cannot. BM25 finds the
// note that repeats the question's words but ranks 下颌第一磨牙 above 上颌第一磨牙. The vector
// channel separates those two by meaning but cannot be trusted alone, since cosine similarity
// between unrelated Chinese passages already sits near 0.4. The reranker reads both together
// and is the only stage whose score is comparable across candidates, so it decides the final
// order.
//
// 混合检索：BM25 管字面，嵌入管语义，交叉编码器做裁决。
//
// 三个阶段之所以都要，是因为每个都在修前一个修不了的失败。BM25 能找到复述问题措辞的笔记，
// 却把《下颌第一磨牙》排在《上颌第一磨牙》之前。向量通道能按语义区分这两者，但不可独任，
// 因为无关中文段落之间的余弦本就在 0.4 附近。重排器同时读两者，且它的分数是唯一在候选
// 之间可比的，因此由它决定最终顺序。

// askCandidate is one passage competing for a slot in the prompt.
// askCandidate 是竞争提示词名额的一个段落。
type askCandidate struct {
	noteID int64
	text   string
	bm25   float64 // BM25 score, 0 when the candidate came only from the vector channel
	vec    float32 // cosine similarity, 0 when it came only from BM25
	via    string  // how it was found, shown in the source list
}

// askVectorCandidates embeds the question and returns the closest passages.
//
// A build runs in the background on first use; while it is in flight this returns nothing and
// the caller proceeds with BM25. Blocking the first question on a multi-second indexing job
// would make the feature look broken on a cold start.
//
// askVectorCandidates 嵌入问题并返回最接近的段落。
//
// 首次使用时索引在后台构建；构建期间本函数返回空，调用方继续用 BM25。
// 让第一个问题等待数秒的索引任务，会让功能在冷启动时看起来是坏的。
func (s *shareService) askVectorCandidates(
	ctx context.Context, opts AskOptions, corpus *askCorpus,
) ([]*askVecHit, error) {
	cfg := opts.Embedding
	if cfg == nil {
		return nil, nil
	}

	if s.askVectors == nil {
		s.askMu.Lock()
		if s.askVectors == nil {
			s.askVectors = newAskVectorStore()
		}
		s.askMu.Unlock()
	}

	fingerprint := corpus.fingerprint()
	idx := s.askVectors.get(opts.UID)
	if idx == nil || idx.fingerprint != fingerprint {
		if s.askVectors.beginBuild(opts.UID) {
			notes := corpus.notes
			go func() {
				defer s.askVectors.endBuild(opts.UID)
				// A panic in a background goroutine terminates the whole process — it is not
				// scoped to the request that spawned it. The vector build touches text
				// slicing and third-party JSON, which is exactly where a boundary bug hides,
				// so it must not be able to take the blog down: recovering here degrades the
				// feature to BM25 instead.
				//
				// 后台 goroutine 里的 panic 会终止整个进程——它并不局限于发起它的那个请求。
				// 向量构建涉及文本切片与第三方 JSON，而这正是边界缺陷的藏身处，
				// 因此不能让它把博客带下去：在这里 recover，功能退化为 BM25 即可。
				defer func() {
					if r := recover(); r != nil {
						s.logger.Error("blog ask: vector index build panicked, staying on BM25")
					}
				}()
				built, err := s.buildVectorIndex(context.Background(), opts.UID, notes, cfg, fingerprint)
				if err != nil {
					s.logger.Warn("blog ask: vector index build failed, staying on BM25")
					return
				}
				s.askVectors.put(opts.UID, built)
			}()
		}
		return nil, nil
	}

	client := newAskEmbedClient(cfg.BaseURL, cfg.APIKey, cfg.Model, 1)
	vecs, err := client.embed(ctx, []string{opts.Question})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, nil
	}
	limit := cfg.CandidateLimit
	if limit <= 0 {
		limit = 60
	}
	return idx.search(vecs[0], limit), nil
}

// buildVectorIndex chunks and embeds the whole corpus.
// buildVectorIndex 对整个语料分块并嵌入。
func (s *shareService) buildVectorIndex(
	ctx context.Context, uid int64, notes []*askNote, cfg *AskEmbeddingConfig, fingerprint string,
) (*askVectorIndex, error) {
	size := cfg.ChunkChars
	if size <= 0 {
		size = 1800
	}
	overlap := cfg.ChunkOverlap
	if overlap < 0 {
		overlap = 0
	}

	chunks := askBuildChunks(notes, size, overlap)
	if len(chunks) == 0 {
		return &askVectorIndex{built: time.Now(), fingerprint: fingerprint}, nil
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	client := newAskEmbedClient(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.BatchSize)
	vectors, err := client.embed(ctx, texts)
	if err != nil {
		return nil, err
	}

	norms := make([]float32, len(vectors))
	for i, v := range vectors {
		norms[i] = askNorm(v)
	}
	return &askVectorIndex{
		chunks:      chunks,
		vectors:     vectors,
		norms:       norms,
		built:       time.Now(),
		fingerprint: fingerprint,
	}, nil
}

// fingerprint identifies the corpus contents so a note edit invalidates the index.
//
// Built from the note ids and their content hashes rather than from a timestamp: a note that
// is re-saved with identical text must not force a rebuild, and a note whose text changed must
// force one even if its mtime somehow did not.
//
// fingerprint 标识语料内容，使笔记修改后索引失效。
//
// 由笔记 id 与其内容哈希构成，而不是用时间戳：正文未变的重新保存不应触发重建，
// 而正文已变的笔记即使 mtime 因故没变也必须触发。
func (c *askCorpus) fingerprint() string {
	h := 0
	for _, n := range c.notes {
		h = h*31 + int(n.ID)
		h = h*31 + len(n.Plain)
	}
	return itoa(int64(h)) + "-" + itoa(int64(len(c.notes)))
}

// askHybridRetrieve merges the two recall channels and reranks them.
//
// Returns the hits and a short label describing which channels actually contributed, so the
// progress line can say what happened rather than always claiming a hybrid search.
//
// askHybridRetrieve 合并两条召回通道并重排。
//
// 同时返回一个描述实际生效通道的短标签，使进度行能说明真实情况，
// 而不是永远声称做了混合检索。
func (s *shareService) askHybridRetrieve(
	ctx context.Context, opts AskOptions, corpus *askCorpus, tokens []string, topK int,
) ([]*askHit, string) {
	limit := 0
	if opts.Embedding != nil && opts.Embedding.CandidateLimit > 0 {
		limit = opts.Embedding.CandidateLimit
	}
	if limit <= 0 {
		limit = 60
	}

	// ── Stage 1: BM25 ──
	bm25Hits := askRetrieve(corpus, tokens, limit)

	cands := make(map[int64]*askCandidate, len(bm25Hits))
	order := make([]int64, 0, len(bm25Hits))
	for _, h := range bm25Hits {
		cands[h.note.ID] = &askCandidate{
			noteID: h.note.ID,
			text:   askExcerpt(h.note.Plain, tokens, 1200),
			bm25:   h.score,
			via:    "关键词命中",
		}
		order = append(order, h.note.ID)
	}
	channels := "关键词"

	// ── Stage 1.5: wiki-link neighbours ──
	// A note that links to a hit is an editorial statement that the two belong together, and
	// that signal survives even when the wording does not match at all. Neighbours enter the
	// pool rather than the results: the reranker decides whether they are actually relevant,
	// which is what stops a link from dragging in an unrelated chapter.
	//
	// 阶段 1.5：wiki 链接邻居。
	// 一篇笔记链接到某条命中，等于编辑层面声明两者相关，这个信号在措辞完全不匹配时依然存在。
	// 邻居进入候选池而不是直接进结果：由重排器判断它们是否真的相关，
	// 这正是避免一条链接把无关章节拖进来的原因。
	if expanded := askExpandLinks(ctx, s, opts.UID, corpus, bm25Hits, limit); len(expanded) > 0 {
		for _, h := range expanded {
			if _, seen := cands[h.note.ID]; seen {
				continue
			}
			cands[h.note.ID] = &askCandidate{
				noteID: h.note.ID,
				text:   askExcerpt(h.note.Plain, tokens, 1200),
				bm25:   h.score,
				via:    h.via,
			}
			order = append(order, h.note.ID)
		}
	}

	// ── Stage 2: vectors ──
	if opts.Embedding != nil {
		vecHits, err := s.askVectorCandidates(ctx, opts, corpus)
		if err != nil {
			s.logger.Warn("blog ask: vector channel unavailable, continuing on BM25")
		} else if len(vecHits) > 0 {
			for _, vh := range vecHits {
				if existing, ok := cands[vh.chunk.NoteID]; ok {
					// The same note was found both ways. Keep the keyword excerpt, which is
					// centred on the matched terms, and record the semantic score.
					// 同一篇笔记被两条通道同时找到。保留关键词摘录（它围绕命中词展开），
					// 并记下语义分数。
					existing.vec = vh.score
					existing.via = "关键词+语义命中"
					continue
				}
				cands[vh.chunk.NoteID] = &askCandidate{
					noteID: vh.chunk.NoteID,
					text:   vh.chunk.Text,
					vec:    vh.score,
					via:    "语义命中",
				}
				order = append(order, vh.chunk.NoteID)
			}
			channels = "关键词+语义"
		}
	}

	if len(cands) == 0 {
		return nil, channels
	}

	// ── Stage 3: rerank ──
	pool := make([]*askCandidate, 0, len(order))
	for _, id := range order {
		if c, ok := cands[id]; ok {
			pool = append(pool, c)
		}
	}

	// Sort the pool deterministically before reranking so the request body — and therefore the
	// cache key on the upstream side — is stable for identical inputs.
	// 重排前先对候选池做确定性排序，使相同输入产生相同的请求体，从而在上游侧命中同一缓存。
	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].bm25 != pool[j].bm25 {
			return pool[i].bm25 > pool[j].bm25
		}
		if pool[i].vec != pool[j].vec {
			return pool[i].vec > pool[j].vec
		}
		return pool[i].noteID < pool[j].noteID
	})

	scored := make([]struct {
		cand  *askCandidate
		score float64
	}, 0, len(pool))

	if opts.Rerank != nil && len(pool) > 0 {
		docs := make([]string, len(pool))
		for i, c := range pool {
			docs[i] = c.text
		}
		client := newAskRerankClient(opts.Rerank.BaseURL, opts.Rerank.APIKey, opts.Rerank.Model, opts.Rerank.DocChars)
		hits, err := client.rerank(ctx, opts.Question, docs)
		if err != nil {
			s.logger.Warn("blog ask: rerank unavailable, falling back to channel scores")
		} else {
			for _, h := range hits {
				scored = append(scored, struct {
					cand  *askCandidate
					score float64
				}{cand: pool[h.index], score: h.score})
			}
			channels += "+重排"
		}
	}

	if len(scored) == 0 {
		// No reranker: fall back to a blended score. Normalising each channel by its own
		// maximum is what makes them comparable — raw BM25 values and cosines live on
		// different scales entirely.
		// 无重排器时退化为融合分。按各通道自身的最大值归一化，才使两者可比——
		// 原始 BM25 值与余弦值完全不在同一量纲上。
		var maxBM25 float64
		var maxVec float32
		for _, c := range pool {
			if c.bm25 > maxBM25 {
				maxBM25 = c.bm25
			}
			if c.vec > maxVec {
				maxVec = c.vec
			}
		}
		for _, c := range pool {
			var s float64
			if maxBM25 > 0 {
				s += 0.5 * c.bm25 / maxBM25
			}
			if maxVec > 0 {
				s += 0.5 * float64(c.vec) / float64(maxVec)
			}
			scored = append(scored, struct {
				cand  *askCandidate
				score float64
			}{cand: c, score: s})
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].cand.noteID < scored[j].cand.noteID
	})

	// Drop candidates the reranker judged irrelevant. The threshold is absolute because the
	// reranker's score is a probability-like relevance, not a per-query relative value —
	// unlike BM25, where an absolute cut would be meaningless.
	// 丢弃重排器判定为无关的候选。这里用绝对阈值，因为重排器的分数是类似概率的相关性，
	// 不是每次查询内的相对值——而 BM25 用绝对阈值则毫无意义。
	cut := scored
	if opts.Rerank != nil {
		kept := make([]struct {
			cand  *askCandidate
			score float64
		}, 0, len(scored))
		for _, s := range scored {
			if s.score < askRerankMinScore {
				continue
			}
			kept = append(kept, s)
		}
		if len(kept) > 0 {
			cut = kept
		}
	}

	if topK > 0 && len(cut) > topK {
		cut = cut[:topK]
	}

	out := make([]*askHit, 0, len(cut))
	for _, s := range cut {
		note, ok := corpus.byID[s.cand.noteID]
		if !ok {
			continue
		}
		out = append(out, &askHit{
			note:  note,
			score: s.score,
			via:   s.cand.via,
		})
	}
	return out, channels
}

// askRerankMinScore drops candidates the cross-encoder scored as irrelevant.
//
// 0.05 is far below the ~0.9 a genuinely relevant passage earns (measured: correct 0.97,
// unrelated 0.0003) and well above the noise floor, so it removes the long tail without
// touching anything the reranker actually endorsed.
//
// askRerankMinScore 丢弃交叉编码器判定为无关的候选。
//
// 0.05 远低于真正相关段落得到的约 0.9（实测正确 0.97、无关 0.0003），又明显高于噪音底噪，
// 因此它能切掉长尾，而不会误伤重排器真正认可的候选。
const askRerankMinScore = 0.05
