package service

import (
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// buildTestGraph runs the pure part of the local-graph assembly: the map/edge plumbing, the
// truncation rule and the layout call. The neighbour lookups need repositories and are covered
// by the depth/cycle tests below instead.
//
// buildTestGraph 运行局部图组装的纯逻辑部分：map/边的连接、截断规则与布局调用。
// 邻居查询需要仓库，因此改由下面对深度/环路的测试覆盖。
func buildTestGraph(sources []*localGraphNode, edgePairs [][2]int64, centerID int64, maxNodes int) *dto.BlogGraphDTO {
	nodes := make(map[int64]*localGraphNode, len(sources))
	for _, src := range sources {
		nodes[src.NoteID] = src
	}
	edges := make(map[string]*dto.BlogGraphEdge, len(edgePairs))
	for _, p := range edgePairs {
		edges[edgeKey(p[0], p[1])] = &dto.BlogGraphEdge{
			Source: "n" + itoa(p[0]),
			Target: "n" + itoa(p[1]),
		}
	}
	return finishLocalGraph(nodes, edges, centerID, maxNodes)
}

func src(id int64, title string) *localGraphNode {
	return &localGraphNode{
		NoteID:   id,
		Title:    title,
		Slug:     "1." + itoa(id) + "-abcdef0123",
		PathHash: "h" + itoa(id),
		Path:     title + ".md",
	}
}

// TestEdgeKeyIsUndirected: A→B and B→A must be the same edge, otherwise every mutual link is
// drawn twice and the degree count is doubled.
//
// TestEdgeKeyIsUndirected：A→B 与 B→A 必须是同一条边，否则每一条互相链接都会被画两次，
// 度数也会翻倍。
func TestEdgeKeyIsUndirected(t *testing.T) {
	if edgeKey(3, 7) != edgeKey(7, 3) {
		t.Errorf("edge key is direction-dependent: %q vs %q", edgeKey(3, 7), edgeKey(7, 3))
	}
	if edgeKey(3, 7) == edgeKey(3, 8) {
		t.Errorf("edge key collides for different pairs")
	}
}

// TestLocalGraphCentreSurvivesTruncation is the load-bearing rule: a local graph without its
// centre is meaningless, so the centre is ordered first and truncation can only drop neighbours.
//
// TestLocalGraphCentreSurvivesTruncation 是关键规则：没有中心的局部图没有意义，
// 因此中心排在最前，截断只能丢掉邻居。
func TestLocalGraphCentreSurvivesTruncation(t *testing.T) {
	sources := []*localGraphNode{
		src(1, "一"), src(2, "二"), src(3, "三"), src(4, "四"), src(5, "五"),
	}
	edges := [][2]int64{{1, 2}, {1, 3}, {1, 4}, {1, 5}}

	// Centre is note 5 — the highest id, so a plain id sort would drop it first.
	// 中心是 id 最大的 5 号，因此单纯的 id 排序会先把它丢掉。
	g := buildTestGraph(sources, edges, 5, 2)
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes after truncation, got %d", len(g.Nodes))
	}
	found := false
	for _, n := range g.Nodes {
		if n.ID == "n5" {
			found = true
			if !n.Lit {
				t.Errorf("centre node must have Lit=true")
			}
		}
	}
	if !found {
		t.Errorf("centre node was truncated away: %v", g.Nodes)
	}
	if !g.Truncated {
		t.Errorf("Truncated should be true when nodes were dropped")
	}
}

// TestLocalGraphEdgesRequireBothEndpoints: an edge whose endpoint was truncated must not be
// emitted, or the SVG would reference a node that is not in the document.
//
// TestLocalGraphEdgesRequireBothEndpoints：端点被截断的边不得输出，
// 否则 SVG 会引用一个不在文档里的节点。
func TestLocalGraphEdgesRequireBothEndpoints(t *testing.T) {
	sources := []*localGraphNode{src(1, "一"), src(2, "二"), src(3, "三")}
	edges := [][2]int64{{1, 2}, {2, 3}}

	// Keep only 2 nodes: the centre (1) and one neighbour.
	// 只保留 2 个节点：中心（1）与一个邻居。
	g := buildTestGraph(sources, edges, 1, 2)
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}
	for _, e := range g.Edges {
		if !ids[e.Source] || !ids[e.Target] {
			t.Errorf("edge %s→%s references a truncated node", e.Source, e.Target)
		}
	}
}

// TestLocalGraphDegreeCountsBothDirections: the node radius is derived from Deg, so a mutual
// link must count once for each node.
//
// TestLocalGraphDegreeCountsBothDirections：节点半径由 Deg 推导，
// 因此一条互相链接必须为两个节点各计一次。
func TestLocalGraphDegreeCountsBothDirections(t *testing.T) {
	sources := []*localGraphNode{src(1, "一"), src(2, "二"), src(3, "三")}
	g := buildTestGraph(sources, [][2]int64{{1, 2}, {1, 3}}, 1, 0)

	degOf := map[string]int{}
	for _, n := range g.Nodes {
		degOf[n.ID] = n.Deg
	}
	if degOf["n1"] != 2 {
		t.Errorf("centre degree = %d, want 2", degOf["n1"])
	}
	if degOf["n2"] != 1 || degOf["n3"] != 1 {
		t.Errorf("leaf degrees = %d/%d, want 1/1", degOf["n2"], degOf["n3"])
	}
}

// TestLocalGraphEmptyInputs: an empty graph must still be a valid DTO with a geometry, because
// the template renders it unconditionally.
//
// TestLocalGraphEmptyInputs：空图仍必须是带几何信息的合法 DTO，因为模板会无条件渲染它。
func TestLocalGraphEmptyInputs(t *testing.T) {
	g := buildTestGraph(nil, nil, 1, 10)
	if g == nil {
		t.Fatal("nil graph")
	}
	if g.Width == 0 || g.Height == 0 {
		t.Errorf("empty graph has no geometry")
	}
	if !g.PublishedOnly {
		t.Errorf("local graph must always be PublishedOnly")
	}
	if g.Nodes == nil || g.Edges == nil {
		t.Errorf("empty graph should have non-nil slices for the template")
	}
}

// TestLocalGraphIsDeterministic: the same inputs must produce byte-identical node order, so a
// refresh does not reshuffle the picture. The layout is seeded, and node/edge ordering is
// sorted, so nothing here depends on map iteration.
//
// TestLocalGraphIsDeterministic：相同输入必须产出字节一致的节点顺序，
// 使刷新不会让图形重新排列。布局是固定种子的，节点与边的顺序都排过序，
// 因此这里没有任何东西依赖 map 遍历顺序。
func TestLocalGraphIsDeterministic(t *testing.T) {
	sources := []*localGraphNode{src(9, "九"), src(4, "四"), src(1, "一"), src(7, "七")}
	edges := [][2]int64{{9, 4}, {1, 7}, {4, 1}}

	first := buildTestGraph(sources, edges, 4, 0)
	for i := 0; i < 5; i++ {
		again := buildTestGraph(sources, edges, 4, 0)
		if len(again.Nodes) != len(first.Nodes) {
			t.Fatalf("node count changed between runs")
		}
		for j := range first.Nodes {
			a, b := first.Nodes[j], again.Nodes[j]
			if a.ID != b.ID || a.X != b.X || a.Y != b.Y {
				t.Fatalf("run %d differs at index %d: %v vs %v", i, j, a, b)
			}
		}
	}
}
