package service

import (
	"context"
	"sort"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"github.com/haierkeys/fast-note-sync-service/pkg/code"
)

// Local knowledge graph: the sub-graph around one article.
//
// Why this is a separate entry point from GetBlogHome: the article page needs a small graph
// centred on the note being read, not the whole vault.
//
// The privacy rule is the same as everywhere else, and stricter than the home page's: an
// unpublished note never appears — not even as an anonymous dot. A dot would still leak
// "something is linked here", and unlike the home graph there is no reason for the article page
// to show the vault's shape at all.
//
// 局部知识图谱：围绕单篇文章的子图。
//
// 为什么它与 GetBlogHome 是两个入口：文章页需要的是以正在阅读的笔记为中心的小图，而不是整个仓库。
//
// 隐私规则与其它地方一致，且比首页更严格：未发布的笔记绝不出现——连匿名圆点都不出现。
// 圆点仍会泄露「这里有关联」，而且与首页图谱不同，文章页没有任何理由展示整个仓库的形状。
//
// Data access is metadata-only. An earlier version reused the home-page loader, which reads the
// content file of every published note; to build a 9-node graph that meant 112 SQL round trips
// and 112 file reads for five fields, measured as a +0.17 s regression per article view.
//
// 数据访问只取元数据。早先的版本复用了首页加载器，它会读取每篇已发布笔记的正文文件；
// 为了构建一个 9 节点的图，代价是 112 次 SQL 往返与 112 次文件读取，实测为每次打开文章 +0.17 s。

// localGraphMaxDepth bounds the slider's range. Each extra level multiplies the neighbour
// lookups, and beyond three hops a "local" graph stops being local.
//
// localGraphMaxDepth 限制滑块的取值范围。每多一层都会成倍增加邻居查询，
// 而超过三跳的「局部」图已经不再是局部的。
const localGraphMaxDepth = 3

// Canvas geometry. Kept in sync with the home graph's aspect ratio so both use the same
// stylesheet and the same layout constants.
//
// 画布几何。与首页图谱保持同一比例，使两者共用同一套样式与布局常量。
const (
	localGraphWidth  = 1120
	localGraphHeight = 760
)

// localGraphNode is one node of the local graph: just enough to draw it and to keep walking.
// localGraphNode 是局部图的一个节点：刚好够画出来并继续遍历。
type localGraphNode struct {
	NoteID   int64
	VaultID  int64
	Path     string
	PathHash string
	Title    string
	Slug     string
	// Hop counts links from the centre: 0 for the centre itself, 1 for a direct neighbour, and
	// so on. The client's spring length is derived from it, which is what makes "the closer the
	// relationship, the nearer the node" true on screen.
	//
	// Hop 是与中心的链接距离：中心为 0，直接邻居为 1，以此类推。
	// 客户端的弹簧长度由它推导，这正是「关联越深越近」在屏幕上成立的原因。
	Hop int
}

// localGraphIndex holds the metadata the traversal needs.
//
// byNoteID / byPathHash are filled lazily per BFS level rather than up front: the whole point of
// this rewrite is that a 9-node graph must not load 112 notes to find its 9.
//
// localGraphIndex 保存遍历所需的元数据。
//
// byNoteID / byPathHash 按 BFS 层级惰性填充，而不是一次性预取：本次重写的要点正是
// 「9 个节点的图不应该为了找到这 9 个而加载 112 篇笔记」。
type localGraphIndex struct {
	// published maps noteID → shareID for every currently usable note share.
	// published 记录每一条当前可用的笔记分享：noteID → shareID。
	published  map[int64]int64
	byNoteID   map[int64]*localGraphNode
	byPathHash map[string]*localGraphNode
}

// GetLocalGraph builds the neighbourhood graph around one shared note, addressed by its slug.
//
// The slug carries both the owner uid and the share id, so this single argument resolves the
// centre without the caller passing anything else.
//
// GetLocalGraph 构建以一篇已分享笔记为中心的邻域图，通过其 slug 定位。
//
// slug 同时携带所有者 uid 与分享 id，因此单个参数就能解析中心节点，调用方无需再传别的。
func (s *shareService) GetLocalGraph(ctx context.Context, slug string, depth, maxNodes int) (*dto.BlogGraphDTO, error) {
	if depth < 1 {
		depth = 1
	}
	if depth > localGraphMaxDepth {
		depth = localGraphMaxDepth
	}

	uid, sid, err := s.parseLocalShortLink(slug)
	if err != nil {
		return nil, err
	}
	share, err := s.repo.GetByID(ctx, uid, sid)
	if err != nil || share == nil {
		return nil, code.ErrorShareNotFound
	}
	if !s.isShareUsable(share) {
		return nil, domain.ErrShareCancelled
	}
	if share.ResType != "note" {
		return nil, code.ErrorShareNotFound
	}

	idx, err := s.loadLocalGraphIndex(ctx, uid)
	if err != nil {
		return nil, err
	}
	if _, ok := idx.published[share.ResID]; !ok {
		// The article is readable but not in the published set. Return an empty graph rather
		// than an error: the page itself still renders.
		// 文章可读但不在已发布集合中。返回空图而不是错误：页面本身仍要渲染。
		return emptyLocalGraph(), nil
	}

	centerNode := idx.byNoteID[share.ResID]
	if centerNode == nil {
		return emptyLocalGraph(), nil
	}

	nodes := map[int64]*localGraphNode{centerNode.NoteID: centerNode}
	edges := map[string]*dto.BlogGraphEdge{}
	frontier := []*localGraphNode{centerNode}

	for level := 0; level < depth && len(frontier) > 0; level++ {
		next, newEdges := s.expandLocalLevel(ctx, uid, frontier, idx, nodes, level+1)
		for _, e := range newEdges {
			if _, seen := edges[e.key]; !seen {
				edges[e.key] = e.edge
			}
		}
		frontier = next
	}

	return finishLocalGraph(nodes, edges, centerNode.NoteID, maxNodes), nil
}

// localGraphEdge pairs an edge with its identity, so the caller can de-duplicate without
// recomputing the key.
// localGraphEdge 把一条边与它的标识配在一起，使调用方无需重新计算键即可去重。
type localGraphEdge struct {
	key  string
	edge *dto.BlogGraphEdge
}

// expandLocalLevel walks one BFS level.
//
// All neighbour metadata for the level is fetched in a single batch call, so the query count
// scales with the graph's depth rather than with the vault's size. The link-table lookups stay
// per-node because the repository exposes no batch form for them — they are cheap index hits,
// whereas the note lookup was the expensive part (one content read each).
//
// expandLocalLevel 遍历一层 BFS。
//
// 该层所有邻居的元数据用一次批量调用取回，因此查询次数随图的深度增长，而不随仓库规模增长。
// 链接表查询仍是逐节点的，因为仓库没有提供批量形式——它们是廉价的索引命中，
// 而笔记查询才是昂贵的部分（每次一次正文读取）。
func (s *shareService) expandLocalLevel(
	ctx context.Context,
	uid int64,
	frontier []*localGraphNode,
	idx *localGraphIndex,
	known map[int64]*localGraphNode,
	hop int,
) ([]*localGraphNode, []localGraphEdge) {
	if s.noteLinkRepo == nil {
		return nil, nil
	}

	// The index already holds every published note, so a neighbour is a pure map lookup: no
	// per-node note query, and an unpublished target simply resolves to nothing — which is the
	// privacy rule, expressed as an ordinary lookup miss.
	//
	// 索引里已含全部已发布笔记，因此邻居就是一次 map 查找：没有逐节点笔记查询，
	// 而未发布的目标自然解析为空——这就是隐私规则，表现为一次普通的查找未命中。
	next := make([]*localGraphNode, 0, 8)
	outEdges := make([]localGraphEdge, 0, 8)
	seenNext := map[int64]bool{}

	// dir is "out" when the frontier note is the one that wrote the link, "in" when the
	// frontier note is the target. The renderer turns that into an arrowhead.
	//
	// dir 为 "out" 表示边界笔记是链接的发出方，"in" 表示它是目标方。渲染器据此画箭头。
	addEdge := func(from, to *localGraphNode, dir string) {
		key := edgeKey(from.NoteID, to.NoteID)

		// Stamp the far endpoint's hop before reading it. It has to happen here rather than at
		// the moment the node joins the frontier, because this is the first place the two ends of
		// the edge are known together — and a hop read before it is written is always zero, which
		// is exactly the bug: every edge came out at hop 0 and every node therefore settled at the
		// same distance from the centre.
		//
		// 在读之前先把远端节点的 hop 写上。必须在这里而不是在它加入边界集合时做，
		// 因为这里是这条边两端第一次同时可知；而「读在写之前」永远是零——
		// 这正是那个缺陷：每条边的 hop 都是 0，于是每个节点与中心的距离都一样远。
		if to.Hop <= 0 {
			to.Hop = hop
		}
		edgeHop := from.Hop
		if to.Hop > edgeHop {
			edgeHop = to.Hop
		}

		outEdges = append(outEdges, localGraphEdge{key: key, edge: &dto.BlogGraphEdge{
			Source: "n" + itoa(from.NoteID),
			Target: "n" + itoa(to.NoteID),
			Dir:    dir,
			Hop:    edgeHop,
		}})
		if _, ok := known[to.NoteID]; !ok && !seenNext[to.NoteID] {
			seenNext[to.NoteID] = true
			next = append(next, to)
		}
		// Record the node in the caller's map. Missing this line is what produced a graph of
		// seven edges and one node: the edges were collected, but the node map was never
		// updated, so finishLocalGraph dropped every edge whose endpoints it could not find.
		//
		// 把节点写回调用方的 map。漏掉这一行会产出「七条边、一个节点」的图：
		// 边被收集了，但节点 map 从未更新，于是 finishLocalGraph 丢弃了所有端点找不到的边。
		known[to.NoteID] = to
	}

	for _, cur := range frontier {
		if links, err := s.noteLinkRepo.GetOutlinks(ctx, cur.NoteID, uid); err == nil {
			for _, l := range links {
				if l == nil || l.IsEmbed || l.TargetPathHash == "" {
					continue
				}
				if nb, ok := idx.byPathHash[l.TargetPathHash]; ok && nb.NoteID != cur.NoteID {
					addEdge(cur, nb, "out")
				}
			}
		}
		if cur.PathHash != "" {
			if links, err := s.noteLinkRepo.GetBacklinks(ctx, cur.PathHash, cur.VaultID, uid); err == nil {
				for _, l := range links {
					if l == nil || l.IsEmbed {
						continue
					}
					if nb, ok := idx.byNoteID[l.SourceNoteID]; ok && nb.NoteID != cur.NoteID {
						addEdge(cur, nb, "in")
					}
				}
			}
		}
	}

	// Deterministic order: the layout is seeded, but a map iteration order would still change
	// which nodes survive truncation from run to run.
	// 确定性顺序：布局是固定种子的，但 map 遍历顺序仍会改变截断时哪些节点被保留。
	sort.Slice(next, func(i, j int) bool { return next[i].NoteID < next[j].NoteID })
	return next, outEdges
}

// loadLocalGraphIndex reads the share list once — no note content, no per-note query — and
// returns the set of notes that are currently published.
//
// loadLocalGraphIndex 只读一次分享列表——不读笔记正文、不做逐篇查询——并返回当前已发布的笔记集合。
func (s *shareService) loadLocalGraphIndex(ctx context.Context, uid int64) (*localGraphIndex, error) {
	shares, err := s.repo.ListByUID(ctx, uid, "created_at", "desc", 0, blogMaxShares)
	if err != nil {
		return nil, err
	}

	idx := &localGraphIndex{
		published:  make(map[int64]int64, len(shares)),
		byNoteID:   make(map[int64]*localGraphNode, len(shares)),
		byPathHash: make(map[string]*localGraphNode, len(shares)),
	}
	ids := make([]int64, 0, len(shares))
	for _, sh := range shares {
		if !s.isShareUsable(sh) || sh.ResType != "note" {
			continue
		}
		if _, dup := idx.published[sh.ResID]; dup {
			continue
		}
		idx.published[sh.ResID] = sh.ID
		ids = append(ids, sh.ResID)
	}
	if len(ids) == 0 {
		return idx, nil
	}

	// One batch call for the whole published set, instead of one call per note. This is the
	// difference between 2 queries and 112 — and it deliberately still builds the complete
	// path-hash index, because traversal must resolve an outlink target (given as a path hash)
	// without guessing which vault to scope the lookup to.
	//
	// 对已发布全集做一次批量查询，而不是每篇一次。这就是 2 次查询与 112 次查询的差别——
	// 而且它刻意仍然构建完整的路径哈希索引，因为遍历必须能解析出链目标（以路径哈希给出），
	// 而无需猜测该用哪个仓库作为查询范围。
	metas, err := s.noteRepo.ListByIDsMeta(ctx, ids, uid)
	if err != nil {
		return nil, err
	}
	for id, n := range metas {
		if n == nil || n.Action == "delete" {
			continue
		}
		shareID, ok := idx.published[id]
		if !ok {
			continue
		}
		node := &localGraphNode{
			NoteID:   n.ID,
			VaultID:  n.VaultID,
			Path:     n.Path,
			PathHash: n.PathHash,
			Title:    noteTitle(n.Path),
			Slug:     s.BuildLocalShortLink(uid, shareID),
		}
		idx.byNoteID[node.NoteID] = node
		if node.PathHash != "" {
			idx.byPathHash[node.PathHash] = node
		}
	}
	return idx, nil
}

// noteTitle is the display name of a note: its basename without the markdown extension.
// noteTitle 是笔记的显示名：去掉 markdown 扩展名的文件名。
func noteTitle(path string) string {
	base := path
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(base, ".md")
}

// emptyLocalGraph is the shape the template expects when there is nothing to draw.
// emptyLocalGraph 是没有内容可画时模板期望的形状。
func emptyLocalGraph() *dto.BlogGraphDTO {
	return &dto.BlogGraphDTO{
		Nodes:         []*dto.BlogGraphNode{},
		Edges:         []*dto.BlogGraphEdge{},
		Width:         localGraphWidth,
		Height:        localGraphHeight,
		PublishedOnly: true,
	}
}

// finishLocalGraph converts the collected nodes and edges into a laid-out graph DTO.
//
// The centre is placed first so that truncation can never drop it: a local graph without its
// centre would be meaningless.
//
// finishLocalGraph 把收集到的节点与边转换为已布局的图谱 DTO。
//
// 中心节点排在最前，使截断绝不会丢掉它——没有中心的局部图没有意义。
func finishLocalGraph(
	nodes map[int64]*localGraphNode,
	edges map[string]*dto.BlogGraphEdge,
	centerID int64,
	maxNodes int,
) *dto.BlogGraphDTO {
	list := make([]*localGraphNode, 0, len(nodes))
	for _, src := range nodes {
		list = append(list, src)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].NoteID == centerID {
			return true
		}
		if list[j].NoteID == centerID {
			return false
		}
		return list[i].NoteID < list[j].NoteID
	})

	truncated := false
	if maxNodes > 0 && len(list) > maxNodes {
		list = list[:maxNodes]
		truncated = true
	}
	if len(list) == 0 {
		return emptyLocalGraph()
	}

	keep := make(map[string]struct{}, len(list))
	graphNodes := make([]*dto.BlogGraphNode, 0, len(list))
	for _, src := range list {
		id := "n" + itoa(src.NoteID)
		keep[id] = struct{}{}
		graphNodes = append(graphNodes, &dto.BlogGraphNode{
			ID:    id,
			Label: src.Title,
			URL:   "/blog/" + src.Slug,
			Lit:   true, // every node here is published; see the privacy note above
			Focus: src.NoteID == centerID,
			Hop:   src.Hop,
		})
	}

	// Keep only edges whose both endpoints survived.
	// 只保留两端都存活的边。
	graphEdges := make([]*dto.BlogGraphEdge, 0, len(edges))
	for _, e := range edges {
		if _, a := keep[e.Source]; !a {
			continue
		}
		if _, b := keep[e.Target]; !b {
			continue
		}
		graphEdges = append(graphEdges, e)
	}
	sort.Slice(graphEdges, func(i, j int) bool {
		if graphEdges[i].Source != graphEdges[j].Source {
			return graphEdges[i].Source < graphEdges[j].Source
		}
		return graphEdges[i].Target < graphEdges[j].Target
	})

	deg := make(map[string]int, len(graphNodes))
	for _, e := range graphEdges {
		deg[e.Source]++
		deg[e.Target]++
	}
	for _, n := range graphNodes {
		n.Deg = deg[n.ID]
	}

	layoutGraph(graphNodes, graphEdges, localGraphWidth, localGraphHeight, 1100)

	return &dto.BlogGraphDTO{
		Nodes:         graphNodes,
		Edges:         graphEdges,
		Width:         localGraphWidth,
		Height:        localGraphHeight,
		Truncated:     truncated,
		PublishedOnly: true,
	}
}

// edgeKey gives an undirected edge a stable identity, so A→B and B→A are one edge.
// edgeKey 为无向边给出稳定标识，使 A→B 与 B→A 是同一条边。
func edgeKey(a, b int64) string {
	if a > b {
		a, b = b, a
	}
	return itoa(a) + "|" + itoa(b)
}
