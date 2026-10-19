package service

import (
	"context"
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
	"github.com/haierkeys/fast-note-sync-service/internal/domain/mocks"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"github.com/stretchr/testify/mock"
)

// The tests in this file cover the one link in the chain that the earlier suite skipped:
// "link table → index → node map". The previous tests built the node map by hand and called
// finishLocalGraph, so a rewrite could silently stop collecting nodes and every test would still
// pass — which is exactly what happened (a graph of seven edges and one node shipped, and
// finishLocalGraph then dropped all seven edges because their endpoints were "missing").
//
// 本文件的测试覆盖此前套件跳过的那一环：「链接表 → 索引 → 节点 map」。
// 之前的测试手工构造节点 map 再调 finishLocalGraph，于是一次重写可以静默停止收集节点而所有测试
// 仍然通过——这正是实际发生的事（一个「七条边、一个节点」的图被交出去，随后 finishLocalGraph
// 以「端点缺失」为由丢掉了全部七条边）。

func linkNode(id int64, hash string) *localGraphNode {
	return &localGraphNode{
		NoteID:   id,
		VaultID:  2,
		Path:     "folder/n" + itoa(id) + ".md",
		PathHash: hash,
		Title:    "n" + itoa(id),
		Slug:     "1." + itoa(id) + "-abcdef0123",
	}
}

// TestExpandLocalLevelCollectsNodes is the regression test for that defect: one BFS level must
// return the newly discovered nodes *and* record them in the caller's map.
//
// TestExpandLocalLevelCollectsNodes 是该缺陷的回归测试：一层 BFS 必须既返回新发现的节点，
// 又把它们写入调用方的 map。
func TestExpandLocalLevelCollectsNodes(t *testing.T) {
	links := new(mocks.MockNoteLinkRepository)
	links.On("GetOutlinks", mock.Anything, int64(100), int64(1)).Return([]*domain.NoteLink{
		{SourceNoteID: 100, TargetPathHash: "h201", IsEmbed: false},
		{SourceNoteID: 100, TargetPathHash: "h202", IsEmbed: false},
		// An embed is not a knowledge link and must not become a node.
		// 嵌入不是知识关联，不得成为节点。
		{SourceNoteID: 100, TargetPathHash: "h203", IsEmbed: true},
		// A target that is not published resolves to nothing — the privacy rule as a lookup miss.
		// 未发布的目标解析为空——隐私规则表现为一次查找未命中。
		{SourceNoteID: 100, TargetPathHash: "hMISSING", IsEmbed: false},
	}, nil)
	links.On("GetBacklinks", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]*domain.NoteLink{}, nil)

	s := &shareService{noteLinkRepo: links}

	idx := &localGraphIndex{
		published:  map[int64]int64{100: 1, 201: 2, 202: 3},
		byNoteID:   map[int64]*localGraphNode{},
		byPathHash: map[string]*localGraphNode{},
	}
	center := linkNode(100, "h100")
	idx.byNoteID[100] = center
	idx.byPathHash["h100"] = center
	known := map[int64]*localGraphNode{100: center}
	idx.byNoteID[201] = linkNode(201, "h201")
	idx.byPathHash["h201"] = idx.byNoteID[201]
	idx.byNoteID[202] = linkNode(202, "h202")
	idx.byPathHash["h202"] = idx.byNoteID[202]

	next, edges := s.expandLocalLevel(context.Background(), 1, []*localGraphNode{center}, idx, known, 1)

	if len(next) != 2 {
		t.Errorf("next frontier = %d nodes, want 2 (published targets only)", len(next))
	}
	if len(edges) != 2 {
		t.Errorf("edges = %d, want 2", len(edges))
	}
	// The load-bearing assertion: the caller's map must have grown. Without it,
	// finishLocalGraph sees only the centre and discards every edge.
	// 关键断言：调用方的 map 必须增长。缺少它，finishLocalGraph 只会看到中心节点并丢弃全部边。
	if len(known) != 3 {
		t.Errorf("caller map has %d nodes after expansion, want 3 — the nodes were returned but not recorded", len(known))
	}
	for _, id := range []int64{201, 202} {
		if known[id] == nil {
			t.Errorf("node %d missing from the caller map", id)
		}
	}
	if known[203] != nil {
		t.Errorf("an embed target became a node")
	}
}

// TestExpandLocalLevelThenFinishProducesEdges ties the two halves together: after expansion,
// finishLocalGraph must keep the edges rather than dropping them as dangling.
//
// TestExpandLocalLevelThenFinishProducesEdges 把两半连起来：展开之后，finishLocalGraph
// 必须保留边，而不是把它们当作悬空边丢弃。
func TestExpandLocalLevelThenFinishProducesEdges(t *testing.T) {
	links := new(mocks.MockNoteLinkRepository)
	links.On("GetOutlinks", mock.Anything, int64(100), int64(1)).Return([]*domain.NoteLink{
		{SourceNoteID: 100, TargetPathHash: "h201", IsEmbed: false},
		{SourceNoteID: 100, TargetPathHash: "h202", IsEmbed: false},
	}, nil)
	links.On("GetBacklinks", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]*domain.NoteLink{}, nil)

	s := &shareService{noteLinkRepo: links}

	idx := &localGraphIndex{
		published:  map[int64]int64{100: 1, 201: 2, 202: 3},
		byNoteID:   map[int64]*localGraphNode{},
		byPathHash: map[string]*localGraphNode{},
	}
	for id, hash := range map[int64]string{100: "h100", 201: "h201", 202: "h202"} {
		n := linkNode(id, hash)
		idx.byNoteID[id] = n
		idx.byPathHash[hash] = n
	}
	center := idx.byNoteID[100]
	known := map[int64]*localGraphNode{100: center}

	next, levelEdges := s.expandLocalLevel(context.Background(), 1, []*localGraphNode{center}, idx, known, 1)

	edgeMap := make(map[string]*dto.BlogGraphEdge, len(levelEdges))
	for _, e := range levelEdges {
		edgeMap[e.key] = e.edge
	}

	g := finishLocalGraph(known, edgeMap, center.NoteID, 0)

	if len(g.Nodes) != 3 {
		t.Errorf("finishLocalGraph nodes = %d, want 3", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Errorf("finishLocalGraph edges = %d, want 2 (they must not be dropped as dangling)", len(g.Edges))
	}
	if len(next) != 2 {
		t.Errorf("frontier = %d, want 2", len(next))
	}
}

// TestExpandLocalLevelBacklinks covers the reverse direction, where the neighbour is discovered
// by note id rather than by path hash.
//
// TestExpandLocalLevelBacklinks 覆盖反向：邻居通过笔记 id 而非路径哈希被发现。
func TestExpandLocalLevelBacklinks(t *testing.T) {
	links := new(mocks.MockNoteLinkRepository)
	links.On("GetOutlinks", mock.Anything, mock.Anything, mock.Anything).
		Return([]*domain.NoteLink{}, nil)
	links.On("GetBacklinks", mock.Anything, "h100", int64(2), int64(1)).Return([]*domain.NoteLink{
		{SourceNoteID: 301, TargetPathHash: "h100", IsEmbed: false},
	}, nil)

	s := &shareService{noteLinkRepo: links}

	idx := &localGraphIndex{
		published:  map[int64]int64{100: 1, 301: 9},
		byNoteID:   map[int64]*localGraphNode{},
		byPathHash: map[string]*localGraphNode{},
	}
	center := linkNode(100, "h100")
	idx.byNoteID[100] = center
	idx.byPathHash["h100"] = center
	src := linkNode(301, "h301")
	idx.byNoteID[301] = src
	idx.byPathHash["h301"] = src

	known := map[int64]*localGraphNode{100: center}
	next, edges := s.expandLocalLevel(context.Background(), 1, []*localGraphNode{center}, idx, known, 1)

	if len(next) != 1 || next[0].NoteID != 301 {
		t.Errorf("backlink neighbour not discovered: %+v", next)
	}
	if len(edges) != 1 {
		t.Errorf("edges = %d, want 1", len(edges))
	}
	if known[301] == nil {
		t.Errorf("backlink neighbour not recorded in the caller map")
	}
	// The centre must never link to itself.
	// 中心节点绝不自连。
	for _, e := range edges {
		if e.edge.Source == e.edge.Target {
			t.Errorf("self edge emitted: %+v", e.edge)
		}
	}
}
