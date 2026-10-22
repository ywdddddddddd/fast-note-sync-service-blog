package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Vector channel for blog Q&A.
//
// BM25 can only match wording. The failures that motivated this file are precisely the ones
// wording cannot fix: asking for 上颌第一磨牙 ranked 下颌第一磨牙 first, because the two titles
// share four of five bigrams; asking 牙齿为什么会酸痛 retrieved tube-separation notes, because
// no note contains that phrasing. A cross-encoder over embeddings answers "is this passage
// about what was asked", which is a different question from "does it contain these substrings".
//
// 博客问答的向量通道。
//
// BM25 只能匹配措辞。促使写这个文件的失败，恰恰是措辞层面修不好的那些：问「上颌第一磨牙」
// 时「下颌第一磨牙」排第一，因为两个标题五个 bigram 里有四个相同；问「牙齿为什么会酸痛」
// 时检索到器械分离的笔记，因为没有笔记包含这种说法。基于嵌入的交叉编码器回答的是
// 「这段文字是否在讲所问之事」，与「它是否包含这些子串」是两个不同的问题。

// askEmbedClient calls an OpenAI-compatible /embeddings endpoint.
// askEmbedClient 调用 OpenAI 兼容的 /embeddings 端点。
type askEmbedClient struct {
	baseURL string
	apiKey  string
	model   string
	batch   int
	http    *http.Client
}

func newAskEmbedClient(baseURL, apiKey, model string, batch int) *askEmbedClient {
	if batch <= 0 {
		batch = 32
	}
	return &askEmbedClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		batch:   batch,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

type askEmbedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// embed returns one vector per input text, in input order.
//
// The upstream may return the vectors out of order, so they are placed by their index field
// rather than by arrival position — a silent reordering here would attach every vector to the
// wrong passage and still look plausible.
//
// embed 为每个输入文本返回一个向量，顺序与输入一致。
//
// 上游可能乱序返回，因此按 index 字段落位而不是按到达顺序——这里若静默错位，
// 每个向量都会挂到错误的段落上，而结果看起来仍然合理。
func (c *askEmbedClient) embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, len(texts))
	for start := 0; start < len(texts); start += c.batch {
		end := start + c.batch
		if end > len(texts) {
			end = len(texts)
		}
		chunk := texts[start:end]

		payload, err := json.Marshal(map[string]any{
			"model": c.model,
			"input": chunk,
		})
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("embedding request failed: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("embedding upstream %d: %s", resp.StatusCode, truncateForError(string(body)))
		}

		var parsed askEmbedResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("embedding response is not JSON: %w", err)
		}
		if parsed.Error != nil {
			return nil, fmt.Errorf("embedding upstream error: %s", parsed.Error.Message)
		}
		if len(parsed.Data) != len(chunk) {
			return nil, fmt.Errorf("embedding returned %d vectors for %d inputs", len(parsed.Data), len(chunk))
		}
		for _, d := range parsed.Data {
			if d.Index < 0 || d.Index >= len(chunk) {
				return nil, fmt.Errorf("embedding index %d out of range", d.Index)
			}
			out[start+d.Index] = d.Embedding
		}
	}
	return out, nil
}

func truncateForError(s string) string {
	s = strings.TrimSpace(s)
	// Slice by runes, not bytes: cutting a multi-byte character in half produces invalid
	// UTF-8, which then renders as replacement characters in the log line meant to explain
	// the failure.
	// 按 rune 而不是字节切片：把多字节字符截成两半会产生非法 UTF-8，
	// 在那条本该解释失败的日志里显示成替换字符。
	if r := []rune(s); len(r) > 240 {
		return string(r[:240]) + "…"
	}
	return s
}

// askVecHit is one scored passage.
// askVecHit 是一个被打分的段落。
type askVecHit struct {
	chunk *askChunk
	score float32
}

// askVectorIndex holds the embedded corpus.
// askVectorIndex 保存已嵌入的语料。
type askVectorIndex struct {
	chunks  []*askChunk
	vectors [][]float32
	norms   []float32
	built   time.Time
	// fingerprint identifies the corpus this index was built from, so a note edit invalidates
	// it instead of serving vectors that no longer describe the text.
	// fingerprint 标识该索引所依据的语料，使笔记修改后索引失效，而不是继续返回
	// 与当前文本不再对应的向量。
	fingerprint string
}

// askCosine returns the cosine similarity of two vectors.
//
// The index stores precomputed norms because this runs once per chunk per question; taking the
// square root again inside the loop would double the cost for no benefit.
//
// askCosine 返回两个向量的余弦相似度。
//
// 索引里预存了模长，因为该函数每个问题要为每个块跑一次；在循环里重新开方只会白白翻倍开销。
func askCosine(a, b []float32, normA, normB float32) float32 {
	if len(a) != len(b) || normA == 0 || normB == 0 {
		return 0
	}
	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot / (normA * normB)
}

func askNorm(v []float32) float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return float32(math.Sqrt(sum))
}

// search returns the best-scoring passages above a relative floor.
// search 返回得分最高的若干段落，并施加相对下限。
func (idx *askVectorIndex) search(q []float32, limit int) []*askVecHit {
	if idx == nil || len(idx.chunks) == 0 || len(q) == 0 {
		return nil
	}
	qNorm := askNorm(q)
	if qNorm == 0 {
		return nil
	}

	hits := make([]*askVecHit, 0, len(idx.chunks))
	for i, c := range idx.chunks {
		if i >= len(idx.vectors) {
			break
		}
		s := askCosine(q, idx.vectors[i], qNorm, idx.norms[i])
		// Cosine on bge-m3 for unrelated Chinese text sits near 0.35–0.45, so a fixed zero
		// threshold would admit everything. Keep only meaningfully similar passages and let
		// the reranker make the final call.
		// bge-m3 上无关中文文本的余弦通常在 0.35–0.45，所以固定零阈值等于全收。
		// 这里只保留确有相似度的段落，最终判断交给重排器。
		if s < askVecMinSimilarity {
			continue
		}
		hits = append(hits, &askVecHit{chunk: c, score: s})
	}

	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// askVecMinSimilarity is the cosine floor for admitting a passage into the rerank pool.
//
// Set deliberately low: the reranker is the component that decides relevance, and dropping a
// passage here is unrecoverable, while keeping a weak one only costs one slot in a 60-item
// pool.
//
// askVecMinSimilarity 是段落进入重排池的余弦下限。
//
// 刻意设得低：决定相关性的是重排器，而在这里丢掉段落是不可挽回的；保留一个弱候选
// 只占用 60 个名额中的一个。
const askVecMinSimilarity = 0.30

// askVectorStore owns the per-user index and its build lifecycle.
//
// The index is built in the background after the corpus is available: embedding a vault takes
// several seconds, and blocking the first question on it would make the feature feel broken.
// Until it is ready, Ask falls back to BM25 alone rather than failing.
//
// askVectorStore 持有按用户划分的索引及其构建生命周期。
//
// 索引在语料就绪后于后台构建：嵌入整个仓库需要数秒，若让第一个问题等它，功能会显得是坏的。
// 就绪之前 Ask 回退到纯 BM25，而不是直接失败。
type askVectorStore struct {
	mu      sync.RWMutex
	indexes map[int64]*askVectorIndex
	building map[int64]bool
}

func newAskVectorStore() *askVectorStore {
	return &askVectorStore{
		indexes:  make(map[int64]*askVectorIndex),
		building: make(map[int64]bool),
	}
}

func (st *askVectorStore) get(uid int64) *askVectorIndex {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.indexes[uid]
}

func (st *askVectorStore) put(uid int64, idx *askVectorIndex) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.indexes[uid] = idx
}

// beginBuild marks a build as in progress and reports whether the caller should run it.
// A concurrent question during the build returns false and proceeds with BM25 only.
//
// beginBuild 标记开始构建，并返回调用方是否应当执行构建。
// 构建期间到达的并发提问返回 false，仅用 BM25 继续。
func (st *askVectorStore) beginBuild(uid int64) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.building[uid] {
		return false
	}
	st.building[uid] = true
	return true
}

func (st *askVectorStore) endBuild(uid int64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.building, uid)
}
