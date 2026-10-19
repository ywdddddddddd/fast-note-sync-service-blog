package service

import (
	"math"
	"math/rand"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// Node radii. Fixed, and the only distinction is the centre — see the comment in layoutGraph.
//
// 节点半径。固定值，唯一区别是中心节点——原因见 layoutGraph 中的注释。
const (
	graphNodeRadius  = 7.0
	graphFocusRadius = 9.5
)

// layoutGraph positions nodes with a deterministic force-directed (Fruchterman-Reingold)
// relaxation so the knowledge graph can be emitted as plain server-side SVG. No JavaScript
// layout library is shipped to the reader, which keeps the page working without JS.
//
// layoutGraph 用确定性的力导向（Fruchterman-Reingold）松弛算法计算节点坐标，
// 使知识图谱可以直接由服务端输出 SVG。读者端不加载任何 JavaScript 布局库，
// 页面在无 JS 环境下依然可用。
func layoutGraph(nodes []*dto.BlogGraphNode, edges []*dto.BlogGraphEdge, w, h float64, iters int) {
	n := len(nodes)
	if n == 0 {
		return
	}
	if n == 1 {
		nodes[0].X, nodes[0].Y, nodes[0].R = w/2, h/2, graphFocusRadius
		return
	}

	idx := make(map[string]int, n)
	for i, nd := range nodes {
		idx[nd.ID] = i
	}

	// Fixed seed → identical layout on every request. The page is server-rendered, so a
	// layout that reshuffles on each refresh would read as a visual bug.
	// 固定随机种子 → 每次请求得到完全相同的布局。页面是服务端渲染的，
	// 每次刷新都重新排布会让人以为是故障。
	rng := rand.New(rand.NewSource(0x5EED))

	// Initial placement: ring plus slight jitter (converges better than pure noise on small graphs).
	// 初始摆放：圆环 + 轻微抖动（小图上比纯随机收敛更稳）。
	for i, nd := range nodes {
		ang := 2 * math.Pi * float64(i) / float64(n)
		nd.X = w/2 + (w*0.32)*math.Cos(ang) + (rng.Float64()-0.5)*12
		nd.Y = h/2 + (h*0.32)*math.Sin(ang) + (rng.Float64()-0.5)*12
	}

	// Radius is fixed. It used to grow with degree, which made a densely linked hub look like
	// the most important thing on screen and a leaf like a speck — but size then encoded a
	// quantity the reader has no way to read off the picture, and the graph became a bar chart
	// wearing a network's clothes. Distance from the centre now carries that meaning instead.
	//
	// The centre keeps one larger radius, because "which node am I looking at" is the one thing
	// the picture must answer at a glance.
	//
	// 半径固定。此前它随度数增长，使连接密集的枢纽看起来是屏幕上最重要的东西，而叶子看起来像一粒灰尘——
	// 但尺寸于是编码了一个读者无法从图上读出的量，图谱变成了一张穿着网络外衣的柱状图。
	// 现在改由「到中心的距离」承载这个含义。
	//
	// 中心节点保留一个更大的半径，因为「我在看的是哪一个节点」是这张图必须一眼回答的事。
	for _, nd := range nodes {
		if nd.Focus {
			nd.R = graphFocusRadius
		} else {
			nd.R = graphNodeRadius
		}
	}

	// Resolve edges to indices once.
	type eidx struct{ a, b int }
	es := make([]eidx, 0, len(edges))
	for _, e := range edges {
		a, ok1 := idx[e.Source]
		b, ok2 := idx[e.Target]
		if ok1 && ok2 && a != b {
			es = append(es, eidx{a, b})
		}
	}

	k := math.Sqrt(w * h / float64(n)) // ideal edge length // 理想边长
	temp := w / 8.0
	dx := make([]float64, n)
	dy := make([]float64, n)

	for it := 0; it < iters; it++ {
		for i := range dx {
			dx[i], dy[i] = 0, 0
		}

		// Pairwise repulsion (O(n²); the node budget is bounded upstream).
		// 点对之间的斥力（O(n²)；上游已限制节点预算）。
		for i := 0; i < n; i++ {
			xi, yi := nodes[i].X, nodes[i].Y
			for j := i + 1; j < n; j++ {
				ux := xi - nodes[j].X
				uy := yi - nodes[j].Y
				d2 := ux*ux + uy*uy
				if d2 < 1e-4 {
					ux, uy, d2 = 0.31, 0.17, 1e-4
				}
				d := math.Sqrt(d2)
				// Stronger repulsion than the textbook Fruchterman-Reingold constant: these
				// graphs are small and dense, and with label-sized nodes the default pushes
				// everything into one unreadable clump in the middle.
				// 比教科书 FR 常数更强的斥力：这里的图谱小而密集，且节点要占标签的宽度，
				// 用默认常数会把所有节点挤成中间一团，完全看不清。
				f := k * k / d2 * 2.4
				ux, uy = ux/d*f, uy/d*f
				dx[i] += ux
				dy[i] += uy
				dx[j] -= ux
				dy[j] -= uy
			}
		}

		// Attraction along edges.
		// 沿边的引力。
		for _, e := range es {
			ux := nodes[e.a].X - nodes[e.b].X
			uy := nodes[e.a].Y - nodes[e.b].Y
			d := math.Sqrt(ux*ux + uy*uy)
			if d < 1e-4 {
				continue
			}
			f := d * d / k
			ux, uy = ux/d*f, uy/d*f
			dx[e.a] -= ux
			dy[e.a] -= uy
			dx[e.b] += ux
			dy[e.b] += uy
		}

		// Displace within the current temperature, keeping nodes inside the viewport.
		// 在温度限幅内位移，并保证节点留在视口内。
		pad := 46.0
		for i := 0; i < n; i++ {
			d := math.Sqrt(dx[i]*dx[i] + dy[i]*dy[i])
			if d > 1e-6 {
				lim := math.Min(d, temp)
				nodes[i].X += dx[i] / d * lim
				nodes[i].Y += dy[i] / d * lim
			}
			if nodes[i].X < pad {
				nodes[i].X = pad
			} else if nodes[i].X > w-pad {
				nodes[i].X = w - pad
			}
			if nodes[i].Y < pad {
				nodes[i].Y = pad
			} else if nodes[i].Y > h-pad {
				nodes[i].Y = h - pad
			}
		}
		temp *= 0.94
	}

	// Label-aware extent: a node's caption is usually far wider than its circle, and ignoring
	// that is exactly why captions used to overlap. Seeding the spacing from the label's
	// half-width (≈5.6px per glyph at 10.5px) keeps the text legible.
	//
	// 计入标签的占位：标签通常比圆宽得多，忽略它正是先前文字互相压住的原因。
	// 以标签半宽（10.5px 字号下每字约 5.6px）作为间距基准，保证文字可读。
	extent := func(i int) float64 {
		lw := float64(len([]rune(nodes[i].Label))) * 5.6
		if lw > nodes[i].R {
			return lw
		}
		return nodes[i].R
	}

	// Final anti-overlap pass so labels and circles never collide.
	// 收尾防重叠，确保圆与标签不互相压住。
	for pass := 0; pass < 40; pass++ {
		moved := false
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				need := extent(i) + extent(j) + 6
				ux := nodes[j].X - nodes[i].X
				uy := nodes[j].Y - nodes[i].Y
				d := math.Sqrt(ux*ux + uy*uy)
				if d >= need || d < 1e-6 {
					continue
				}
				push := (need - d) / 2
				ux, uy = ux/d*push, uy/d*push
				nodes[j].X += ux
				nodes[j].Y += uy
				nodes[i].X -= ux
				nodes[i].Y -= uy
				moved = true
			}
		}
		if !moved {
			break
		}
		// Anti-overlap between labels: nodes already spread by the force pass may still have
		// captions touching, so relax once more using label widths, then clamp inside the frame.
		// 标签防重叠：经力导向铺开后的节点，其文字仍可能相邻，因此按标签宽度再松弛一轮，
		// 随后收回画布边界内。
		for i := range nodes {
			p := extent(i) + 8
			if nodes[i].X < p {
				nodes[i].X = p
			}
			if nodes[i].X > w-p {
				nodes[i].X = w - p
			}
			if nodes[i].Y < nodes[i].R+8 {
				nodes[i].Y = nodes[i].R + 8
			}
			if nodes[i].Y > h-nodes[i].R-8 {
				nodes[i].Y = h - nodes[i].R - 8
			}
		}
	}

	// Two decimals keep the emitted SVG compact and byte-stable.
	// 保留两位小数，使输出的 SVG 更紧凑、字节更稳定。
	for _, nd := range nodes {
		nd.X = math.Round(nd.X*100) / 100
		nd.Y = math.Round(nd.Y*100) / 100
		nd.R = math.Round(nd.R*100) / 100
	}
}
