package service

import (
	"context"
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
	"github.com/haierkeys/fast-note-sync-service/internal/domain/mocks"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"github.com/stretchr/testify/mock"
)

// Tests for the two properties the graph's on-screen behaviour depends on: each node carries its
// distance from the centre, and each edge carries its direction.
//
// The client's force model reads Hop to decide how far apart two nodes settle, so a wrong Hop is
// not a cosmetic problem — it would place a direct neighbour of the centre at the far edge of the
// picture. And Dir is what lets the renderer draw an arrowhead instead of a bare line; a graph of
// undirected lines cannot answer "which note points at which", which is the only question a
// knowledge graph exists to answer.
//
// 覆盖图谱屏幕行为所依赖的两个性质：每个节点带有到中心的距离，每条边带有方向。
//
// 客户端力模型读 Hop 决定两点的距离，因此 Hop 错了不是外观问题——
// 它会把中心的直接邻居摆到画面最边上。而 Dir 让渲染器能画箭头而不是裸线；
// 一张全是无向线的图回答不了「哪篇指向哪篇」，而那正是知识图谱唯一存在的意义。

func twoHopLinkRepo(t *testing.T) *mocks.MockNoteLinkRepository {
	t.Helper()
	links := new(mocks.MockNoteLinkRepository)
	// 100 (centre) → 201 → 301
	// 100 is also pointed at by 401.
	links.On("GetOutlinks", mock.Anything, int64(100), int64(1)).Return([]*domain.NoteLink{
		{SourceNoteID: 100, TargetPathHash: "h201", IsEmbed: false},
	}, nil)
	links.On("GetOutlinks", mock.Anything, int64(201), int64(1)).Return([]*domain.NoteLink{
		{SourceNoteID: 201, TargetPathHash: "h301", IsEmbed: false},
	}, nil)
	links.On("GetOutlinks", mock.Anything, mock.Anything, mock.Anything).
		Return([]*domain.NoteLink{}, nil)
	links.On("GetBacklinks", mock.Anything, "h100", int64(2), int64(1)).Return([]*domain.NoteLink{
		{SourceNoteID: 401, TargetPathHash: "h100", IsEmbed: false},
	}, nil)
	links.On("GetBacklinks", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]*domain.NoteLink{}, nil)
	return links
}

func idxWith(ids ...int64) *localGraphIndex {
	idx := &localGraphIndex{
		published:  map[int64]int64{},
		byNoteID:   map[int64]*localGraphNode{},
		byPathHash: map[string]*localGraphNode{},
	}
	for i, id := range ids {
		n := linkNode(id, "h"+itoa(id))
		n.Hop = 0
		idx.published[id] = int64(i + 1)
		idx.byNoteID[id] = n
		idx.byPathHash[n.PathHash] = n
	}
	return idx
}

// TestLocalGraphHopIncreasesPerLevel is the property the force model depends on.
// TestLocalGraphHopIncreasesPerLevel 是力模型所依赖的性质。
func TestLocalGraphHopIncreasesPerLevel(t *testing.T) {
	links := twoHopLinkRepo(t)
	s := &shareService{noteLinkRepo: links}

	idx := idxWith(100, 201, 301, 401)
	center := idx.byNoteID[100]
	center.Hop = 0

	nodes := map[int64]*localGraphNode{100: center}
	edges := map[string]*localGraphEdge{}
	frontier := []*localGraphNode{center}

	for level := 1; level <= 2 && len(frontier) > 0; level++ {
		next, newEdges := s.expandLocalLevel(context.Background(), 1, frontier, idx, nodes, level)
		for _, e := range newEdges {
			if _, seen := edges[e.key]; !seen {
				edges[e.key] = &e
			}
		}
		frontier = next
	}

	if got := nodes[100].Hop; got != 0 {
		t.Errorf("centre Hop = %d, want 0", got)
	}
	if got := nodes[201].Hop; got != 1 {
		t.Errorf("direct neighbour Hop = %d, want 1", got)
	}
	if got := nodes[301].Hop; got != 2 {
		t.Errorf("second-level node Hop = %d, want 2", got)
	}
	// The backlink neighbour sits at hop 1 too: distance is counted in links, not in direction.
	// 入链邻居同样位于 hop 1：距离按链接数计，不按方向计。
	if got := nodes[401].Hop; got != 1 {
		t.Errorf("backlink neighbour Hop = %d, want 1", got)
	}
}

// TestLocalGraphEdgeCarriesDirectionAndHop: an edge must say which way it points and how deep it
// sits, because the renderer draws from Dir and the force model stretches by Hop.
//
// TestLocalGraphEdgeCarriesDirectionAndHop：边必须说明它指向哪边、位于多深，
// 因为渲染器据 Dir 画箭头、力模型据 Hop 拉伸。
func TestLocalGraphEdgeCarriesDirectionAndHop(t *testing.T) {
	links := twoHopLinkRepo(t)
	s := &shareService{noteLinkRepo: links}

	idx := idxWith(100, 201, 301, 401)
	center := idx.byNoteID[100]
	center.Hop = 0

	nodes := map[int64]*localGraphNode{100: center}
	next, level1 := s.expandLocalLevel(context.Background(), 1, []*localGraphNode{center}, idx, nodes, 1)

	var outEdge, inEdge *localGraphEdge
	for i := range level1 {
		switch level1[i].edge.Target {
		case "n201":
			outEdge = &level1[i]
		case "n401":
			inEdge = &level1[i]
		}
	}

	if outEdge == nil {
		t.Fatalf("outlink edge missing; got %+v", level1)
	}
	if outEdge.edge.Dir != "out" {
		t.Errorf("outlink edge Dir = %q, want \"out\"", outEdge.edge.Dir)
	}
	if outEdge.edge.Hop != 1 {
		t.Errorf("outlink edge Hop = %d, want 1", outEdge.edge.Hop)
	}

	if inEdge == nil {
		t.Fatalf("backlink edge missing; got %+v", level1)
	}
	if inEdge.edge.Dir != "in" {
		t.Errorf("backlink edge Dir = %q, want \"in\"", inEdge.edge.Dir)
	}

	// Second level: the edge from 201 to 301 must carry hop 2, not 1 — the spring that spans it
	// is what pushes the far node out.
	// 第二层：201 到 301 的边必须带 hop 2 而非 1——横跨它的那根弹簧正是把远端节点推出去的东西。
	nextNodes := nodes
	nextFrontier := next
	_, level2 := s.expandLocalLevel(context.Background(), 1, nextFrontier, idx, nextNodes, 2)
	if len(level2) != 1 {
		t.Fatalf("level 2 produced %d edges, want 1", len(level2))
	}
	if level2[0].edge.Hop != 2 {
		t.Errorf("second-level edge Hop = %d, want 2", level2[0].edge.Hop)
	}
	if nextNodes[301] == nil || nextNodes[301].Hop != 2 {
		t.Errorf("node 301 Hop = %v, want 2", nextNodes[301])
	}
}

// TestFinishLocalGraphPropagatesHop: the hop must survive into the DTO the renderer receives.
// TestFinishLocalGraphPropagatesHop：hop 必须一路传到渲染器拿到的 DTO。
func TestFinishLocalGraphPropagatesHop(t *testing.T) {
	center := linkNode(100, "h100")
	center.Hop = 0
	n1 := linkNode(201, "h201")
	n1.Hop = 1
	n2 := linkNode(301, "h301")
	n2.Hop = 2

	nodes := map[int64]*localGraphNode{100: center, 201: n1, 301: n2}
	edges := map[string]*dto.BlogGraphEdge{
		edgeKey(100, 201): {Source: "n100", Target: "n201", Dir: "out", Hop: 1},
		edgeKey(201, 301): {Source: "n201", Target: "n301", Dir: "out", Hop: 2},
	}

	g := finishLocalGraph(nodes, edges, 100, 0)
	if len(g.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(g.Nodes))
	}
	byID := map[string]int{}
	for _, n := range g.Nodes {
		byID[n.ID] = n.Hop
		if n.R == 0 {
			t.Errorf("node %s has radius 0", n.ID)
		}
	}
	if byID["n100"] != 0 || byID["n201"] != 1 || byID["n301"] != 2 {
		t.Errorf("hop not propagated: %+v", byID)
	}
	// Radius must not depend on degree any more: the centre is larger purely because it is the
	// centre, and every other node shares one radius.
	// 半径不再依赖度数：中心更大只因为它是中心，其余节点共用一个半径。
	var centerR, otherR float64
	for _, n := range g.Nodes {
		if n.Focus {
			centerR = n.R
		} else if otherR == 0 {
			otherR = n.R
		}
	}
	if centerR <= otherR {
		t.Errorf("centre radius %.2f should exceed the shared radius %.2f", centerR, otherR)
	}
}
