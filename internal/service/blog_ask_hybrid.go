package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/haierkeys/fast-note-sync-service/pkg/safego"
	"github.com/haierkeys/fast-note-sync-service/pkg/util"
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
		if !s.askVectors.beginBuild(opts.UID) {
			// Another question is already building it. Proceed on BM25 rather than wait.
			// 另一个提问正在构建它。直接走 BM25，而不是等它。
			return nil, nil
		}

		// A persisted copy is tried first: reading ~1.9 MB costs milliseconds, while a rebuild
		// costs a dozen embedding round trips and ten-odd seconds.
		//
		// 先尝试持久化副本：读约 1.9 MB 只要几毫秒，而重建要十来次嵌入往返、十几秒。
		if loaded := s.loadAskVectorIndex(s.askVectorPath(opts.UID), corpus.notes, cfg, fingerprint); loaded != nil {
			s.askVectors.put(opts.UID, loaded)
			s.askVectors.endBuild(opts.UID)
			idx = loaded
		} else {
			notes := corpus.notes
			// A panic in a background goroutine terminates the whole process — it is not
			// scoped to the request that spawned it. The vector build touches text slicing
			// and third-party JSON, which is exactly where a boundary bug hides, so it must
			// not be able to take the blog down: recovering here degrades the feature to BM25
			// instead.
			//
			// safego.Go rather than a bare `go func()` with an inline recover, because the
			// inline form threw away the one piece of evidence that matters. An earlier
			// boundary bug (byte offset used as a rune offset) crashed the service every
			// 15–60 seconds and the log recorded only the sentence, never the frame: the
			// recover caught the panic and the stack was lost with it. safego.Go logs
			// debug.Stack() alongside the value, so the next occurrence is diagnosable
			// without a rebuild.
			//
			// 后台 goroutine 里的 panic 会终止整个进程——它并不局限于发起它的那个请求。
			// 向量构建涉及文本切片与第三方 JSON，而这正是边界缺陷的藏身处，
			// 因此不能让它把博客带下去：在这里 recover，功能退化为 BM25 即可。
			//
			// 用 safego.Go 而不是带内联 recover 的裸 go func()，是因为内联那种写法丢掉了
			// 唯一要紧的证据。此前的一个边界缺陷（把字节偏移当 rune 偏移用）每 15–60 秒
			// 让服务崩一次，日志里只留下那句话、没有一帧调用栈：recover 接住了 panic，
			// 堆栈也随之消失。safego.Go 会把 debug.Stack() 和 panic 值一起记下，
			// 使下一次发生时不必重新编译就能定位。
			safego.Go(s.logger, func() {
				defer s.askVectors.endBuild(opts.UID)
				built, err := s.buildVectorIndex(context.Background(), opts.UID, notes, cfg, fingerprint)
				if err != nil {
					s.logger.Warn("blog ask: vector index build failed, staying on BM25")
					return
				}
				s.askVectors.put(opts.UID, built)
				if err := saveAskVectorIndex(s.askVectorPath(opts.UID), built); err != nil {
					// Persistence is an optimisation, never a requirement: the index is
					// already in memory and this question's answer is unaffected.
					// 持久化只是优化，从来不是必需：索引已在内存中，本次回答不受影响。
					s.logger.Warn("blog ask: could not persist vector index")
				}
			})
			return nil, nil
		}
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
	size := askChunkSize(cfg)
	overlap := askChunkOverlap(cfg)

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

	// Flatten into one contiguous slice. The upstream returns one slice per input, but the
	// index is stored and searched as a single array; keeping the per-chunk slices would mean
	// hundreds of allocations and a per-vector copy on every save.
	// 摊平成一整块连续切片。上游按输入各返回一个切片，而索引的存储与检索都按单个数组进行；
	// 保留逐块切片意味着数百次分配，以及每次保存时的逐向量拷贝。
	dim := 0
	total := 0
	for _, v := range vectors {
		if dim == 0 {
			dim = len(v)
		}
		if len(v) != dim {
			return nil, fmt.Errorf("embedding returned inconsistent dimensions: %d then %d", dim, len(v))
		}
		total += len(v)
	}
	if dim == 0 {
		return nil, fmt.Errorf("embedding returned no vectors")
	}

	flat := make([]float32, 0, total)
	for _, v := range vectors {
		flat = append(flat, v...)
	}

	norms := make([]float32, len(vectors))
	for i := range vectors {
		norms[i] = askNorm(flat[i*dim : (i+1)*dim])
	}

	return &askVectorIndex{
		chunks:      chunks,
		vectors:     flat,
		norms:       norms,
		dim:         dim,
		built:       time.Now(),
		fingerprint: fingerprint,
	}, nil
}

// fingerprint identifies the corpus contents so a note edit invalidates the index.
//
// Built from each note's id and its own content digest. Using the length instead would miss
// every edit that preserves length — and, once the vectors are persisted, that means serving
// embeddings of text that no longer exists.
//
// fingerprint 标识语料内容，使笔记修改后索引失效。
//
// 由每篇笔记的 id 与其自身内容摘要构成。改用长度会漏掉所有长度不变的编辑——
// 而在向量被持久化之后，那意味着继续使用已不存在的文本的嵌入。
func (c *askCorpus) fingerprint() string {
	var b strings.Builder
	for _, n := range c.notes {
		b.WriteString(strconv.FormatInt(n.ID, 10))
		b.WriteByte(':')
		b.WriteString(n.ContentHash)
		b.WriteByte('|')
	}
	return util.EncodeHash32(b.String())
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

	// ── Stage 1.6: Bleve full-text index ──
	// The Bleve index already holds every note's path and content under the cjk analyzer, and
	// it is the same index the regular search API reads, so Q&A and search can never disagree
	// about what matches. It only returns ids, with no score, and its MatchQuery joins terms
	// with AND — passing the whole question would almost never match, because a question
	// contains words no note repeats. Probing with a few of the longest tokens instead turns
	// it into the inverted-index lookup it is good at.
	//
	// 阶段 1.6：Bleve 全文索引。
	// Bleve 索引在 cjk 分析器下已保存每篇笔记的路径与正文，且它正是常规搜索接口读取的
	// 同一份索引，因此问答与搜索对「什么算命中」永远不会相互矛盾。它只返回 id、没有分数，
	// 而 MatchQuery 用 AND 连接各词——直接传整个问题几乎必然查不到，因为问题里的词
	// 没有哪篇笔记会全部复述。改用若干最长的检索词去探测，才是它擅长的倒排查找。
	if s.ftsRepo != nil {
		if vaultID, probes := askBleveProbes(corpus, tokens); vaultID != 0 && len(probes) > 0 {
			added := 0
			returned := 0
			for _, probe := range probes {
				ids, err := s.ftsRepo.Search(ctx, probe, vaultID, opts.UID, 24, 0)
				if err != nil {
					continue
				}
				returned += len(ids)
				for _, id := range ids {
					if _, seen := cands[id]; seen {
						continue
					}
					note, ok := corpus.byID[id]
					if !ok {
						// The index can hold ids for notes this corpus excluded, e.g. a note
						// that was unshared after being indexed. Skipping is the privacy rule.
						// 索引里可能含有被本语料排除的 id，例如分享取消后仍留在索引中的笔记。
						// 跳过它就是隐私规则。
						continue
					}
					cands[id] = &askCandidate{
						noteID: id,
						text:   askExcerpt(note.Plain, tokens, 1200),
						via:    "全文索引命中",
					}
					order = append(order, id)
					added++
				}
			}
			// One line per question: enough to tell whether the index channel contributes
			// anything at all, which is the only question worth asking of a fourth recall path.
			// 每次提问一行：足以判断索引通道是否有任何贡献，而这是评估第四条召回路径时
			// 唯一值得问的问题。
			s.logger.Info("blog ask: bleve channel",
				zap.Int("probes", len(probes)),
				zap.Int("returned", returned),
				zap.Int("added", added))
			if added > 0 {
				channels += "+全文索引"
			}
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

// askBleveProbes picks the vault to query and the tokens worth sending.
//
// Only the longest tokens are used. A bigram like 颌第 appears in dozens of dental notes and
// would return most of the vault, which costs a query and tells the reranker nothing; a
// trigram or a whole-run token is specific enough that a hit means something.
//
// askBleveProbes 选出要查询的仓库与值得发送的检索词。
//
// 只取最长的检索词。像「颌第」这样的 bigram 出现在几十篇牙科笔记里，查它会返回大半个仓库，
// 既花掉一次查询又没给重排器任何信息；三元组或整串检索词则足够具体，命中就意味着什么。
func askBleveProbes(corpus *askCorpus, tokens []string) (int64, []string) {
	vaultID := int64(0)
	for _, n := range corpus.notes {
		if n.VaultID != 0 {
			vaultID = n.VaultID
			break
		}
	}
	if vaultID == 0 {
		return 0, nil
	}

	// Longest first, capped: each probe is one index query.
	// 先取最长的，并设上限：每个检索词就是一次索引查询。
	ordered := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if len([]rune(t)) >= 3 {
			ordered = append(ordered, t)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return len([]rune(ordered[i])) > len([]rune(ordered[j]))
	})
	if len(ordered) > askMaxBleveProbes {
		ordered = ordered[:askMaxBleveProbes]
	}
	return vaultID, ordered
}

// askMaxBleveProbes caps how many index queries one question may issue.
// askMaxBleveProbes 限制单次提问允许发出的索引查询次数。
const askMaxBleveProbes = 4

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
