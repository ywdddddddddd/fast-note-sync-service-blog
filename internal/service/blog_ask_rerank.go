package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Cross-encoder reranking for blog Q&A.
//
// BM25 and cosine similarity are both computed from representations made independently of the
// question, which is why they can be confidently wrong. A cross-encoder reads the question and
// the passage together and outputs one relevance score, which is what makes it worth a network
// round trip: measured on this vault, it scored the correct passage 0.97 and an unrelated one
// 0.0003, a separation the bi-encoders never produced.
//
// 博客问答的交叉编码器重排。
//
// BM25 与余弦相似度都由独立于问题生成的表示算出，因此它们会「自信地出错」。
// 交叉编码器同时读问题和段落，输出一个相关性分数，这正是它值得一次网络往返的原因：
// 在本仓库实测中，它给正确段落 0.97、给无关段落 0.0003，这种分离度是双塔模型给不出的。

// askRerankClient calls an OpenAI-compatible /rerank endpoint.
// askRerankClient 调用 OpenAI 兼容的 /rerank 端点。
type askRerankClient struct {
	baseURL  string
	apiKey   string
	model    string
	docChars int
	http     *http.Client
}

func newAskRerankClient(baseURL, apiKey, model string, docChars int) *askRerankClient {
	if docChars <= 0 {
		docChars = 1200
	}
	return &askRerankClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiKey:   apiKey,
		model:    model,
		docChars: docChars,
		http:     &http.Client{Timeout: 90 * time.Second},
	}
}

// askRerankHit pairs an input position with its relevance score.
// askRerankHit 把输入位置与其相关性分数配在一起。
type askRerankHit struct {
	index int
	score float64
}

type askRerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// rerank scores every document against the query and returns the hits sorted by relevance.
//
// Documents are truncated to docChars before sending. The reranker scores the whole set in one
// request, so passing full-length notes would exceed the request budget; the truncation keeps
// the head of each passage, which is where the matching sentence almost always sits because
// chunking cut the passage at a natural boundary.
//
// rerank 对每个文档与问题打分，并按相关性排序返回。
//
// 文档在发送前截断到 docChars。重排器在一次请求里对整组打分，传入全长笔记会超出请求预算；
// 截断保留每段开头，而命中的那句话几乎总在开头附近，因为分块是在自然边界处切开的。
func (c *askRerankClient) rerank(ctx context.Context, query string, docs []string) ([]askRerankHit, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	trimmed := make([]string, len(docs))
	for i, d := range docs {
		r := []rune(d)
		if len(r) > c.docChars {
			trimmed[i] = string(r[:c.docChars])
		} else {
			trimmed[i] = d
		}
	}

	payload, err := json.Marshal(map[string]any{
		"model":     c.model,
		"query":     query,
		"documents": trimmed,
		"top_n":     len(trimmed),
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/rerank", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank request failed: %w", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank upstream %d: %s", resp.StatusCode, truncateForError(string(body)))
	}

	var parsed askRerankResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("rerank response is not JSON: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("rerank upstream error: %s", parsed.Error.Message)
	}

	hits := make([]askRerankHit, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		if r.Index < 0 || r.Index >= len(docs) {
			return nil, fmt.Errorf("rerank index %d out of range", r.Index)
		}
		hits = append(hits, askRerankHit{index: r.Index, score: r.RelevanceScore})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	return hits, nil
}
