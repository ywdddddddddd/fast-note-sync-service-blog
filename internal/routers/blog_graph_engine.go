package routers

import (
	"encoding/json"
	"html/template"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// Interactive knowledge graph.
//
// Rewritten after reading the reference implementation
// (obsidian-digital-garden, src/site/_includes/components/graphScript.njk) instead of guessing
// from screenshots. What that reading changed:
//
//   - Physics is d3-force, not hand-rolled: forceManyBody + forceCenter + forceLink +
//     forceCollide + forceRadial, with the reference's constants.
//   - Drag is d3-drag with the fx/fy + alphaTarget pattern, and a click is told apart from a drag
//     by time and distance (dt < 500ms && dist < 5px).
//   - Zoom is d3-zoom with scaleExtent [0.25, 4], followed by a zoom-to-fit once the layout
//     settles.
//   - Labels start at alpha 0 and fade in with zoom: alpha = max((k-1)/3.75, 0). The reference
//     does this because a permanently visible caption per node is unreadable at rest — which was
//     the original complaint that started this rewrite.
//   - Hover drives an alpha hierarchy (nodes 1/0.1, links 0.8/0.05) smoothed by a per-frame lerp
//     of 0.15, and gives active links the accent colour. That, not any single effect, is what
//     reads as "graceful".
//   - The reference's current node carries an extra r+1.5 ring.
//
// Three deliberate departures from the reference are marked where they occur: the renderer is SVG
// rather than PixiJS; the graph is docked rather than placed in a sidebar; and node radius is
// fixed rather than degree-derived, so that distance — not size — encodes how closely a note is
// related.
//
// 交互式知识图谱。
//
// 本文件在读完参考实现（obsidian-digital-garden 的 graphScript.njk）之后重写，而不再凭截图猜测。
//
// 三处对参考实现的有意偏离在各自位置标注：渲染用 SVG 而不是 PixiJS；图谱停靠在页面上而不是放进侧栏；
// 节点半径固定而不是按度数推导，使「距离」而不是「大小」编码关联的紧密程度。

// graphEngineJS returns the engine script with its asset version substituted.
//
// The module's import specifier lives inside a Go raw string, so it cannot interpolate a
// constant. Substituting the fingerprint on the way out keeps the version in one place and makes
// a stale cached bundle impossible rather than merely unlikely.
//
// graphEngineJS 返回替换过资源版本的引擎脚本。
//
// 模块的 import 说明符位于 Go 原始字符串内，无法插值一个常量。
// 在输出时替换指纹把版本集中在一处，并使「读者拿到过期缓存」从不太可能变成不可能。
func graphEngineJS() template.JS {
	return template.JS(strings.Replace(blogGraphEngineJS, "__D3V__", d3GraphAssetVersion, 1))
}

// graphDataJSON serialises the graph for the client simulation.
//
// graphDataJSON 把图谱序列化给客户端模拟使用。
func graphDataJSON(g *dto.BlogGraphDTO) template.JS {
	if g == nil || len(g.Nodes) == 0 {
		return template.JS("")
	}

	type node struct {
		ID    string  `json:"id"`
		Label string  `json:"label"`
		URL   string  `json:"url,omitempty"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
		R     float64 `json:"r"`
		Hop   int     `json:"hop"`
		Focus bool    `json:"focus,omitempty"`
	}
	type edge struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Dir    string `json:"dir,omitempty"`
		Hop    int    `json:"hop"`
	}
	payload := struct {
		Width  int    `json:"w"`
		Height int    `json:"h"`
		Nodes  []node `json:"nodes"`
		Edges  []edge `json:"edges"`
	}{
		Width:  g.Width,
		Height: g.Height,
		Nodes:  make([]node, 0, len(g.Nodes)),
		Edges:  make([]edge, 0, len(g.Edges)),
	}
	for _, n := range g.Nodes {
		payload.Nodes = append(payload.Nodes, node{
			ID: n.ID, Label: n.Label, URL: n.URL,
			X: n.X, Y: n.Y, R: n.R, Hop: n.Hop, Focus: n.Focus,
		})
	}
	for _, e := range g.Edges {
		payload.Edges = append(payload.Edges, edge{
			Source: e.Source, Target: e.Target, Dir: e.Dir, Hop: e.Hop,
		})
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return template.JS("")
	}
	return template.JS(raw)
}

// blogGraphEngineJS is the graph's client behaviour: d3 physics, d3 drag, d3 zoom, SVG paint,
// the depth slider, and the fullscreen layer.
//
// Why one script owns all of it: an earlier version had a second script that re-fetched a
// server-rendered SVG and assigned it with container.innerHTML. That assignment destroyed the
// elements the simulation was drawing into, so the graph went motionless, the zoom binding died
// with the replaced subtree, and the depth control appeared to do nothing but replace the picture.
// Nothing in a second script could have fixed that; the fix is that exactly one script owns the
// container.
//
// No backticks appear below — this is a Go raw string.
//
// blogGraphEngineJS 是图谱的客户端行为：d3 物理、d3 拖动、d3 缩放、SVG 绘制、深度滑块与全屏浮层。
//
// 为什么由一个脚本统管全部：早先的版本有第二个脚本，它会重新取一份服务端渲染的 SVG
// 并用 container.innerHTML 赋值。那次赋值销毁了模拟正在绘制的元素，
// 于是图失去动作、缩放绑定随被替换的子树一起死掉，而深度控件看起来只是换了一张图。
// 第二个脚本里没有任何改法能解决这件事；正确的修法是让容器只有一个属主。
//
// 下面没有反引号——这是 Go 原始字符串。
const blogGraphEngineJS string = `import { forceSimulation, forceManyBody, forceCenter, forceLink, forceCollide, forceRadial, drag, zoom, zoomIdentity, select } from '/static/js/d3-graph.min.js?v=__D3V__';
(function(){
  'use strict';

  var NS = 'http://www.w3.org/2000/svg';
  var STILL = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  var DESKTOP = window.matchMedia('(min-width: 861px)');

  var root = document.getElementById('graph-root');
  if (!root) { return; }
  var fab = document.getElementById('graph-fab');
  var panel = document.getElementById('graph-panel');
  var backdrop = document.getElementById('graph-backdrop');
  var collapseBtn = document.getElementById('gc-collapse');
  var canvas = document.getElementById('graph-canvas');
  var slider = document.getElementById('gc-depth');
  var depthOut = document.getElementById('gc-depth-v');
  var btnFull = document.getElementById('gc-full');
  var btnExpand = document.getElementById('gc-expand');
  var layer = document.getElementById('graph-layer');
  var layerTitle = document.getElementById('graph-layer-title');
  var layerBody = document.getElementById('graph-layer-body');
  var layerClose = document.getElementById('graph-layer-close');

  // ── colours ────────────────────────────────────────────────────────
  // Resolved from CSS variables through a probe element so that calc()/hsl() values work, as the
  // reference does.
  // 经探针元素从 CSS 变量解析，使 calc()/hsl() 也能用，与参考实现一致。
  var probe = document.createElement('div');
  probe.style.display = 'none';
  document.body.appendChild(probe);
  function resolve(name, fallback) {
    var raw = getComputedStyle(document.body).getPropertyValue(name).trim();
    if (!raw) { return fallback; }
    probe.style.color = raw;
    var got = getComputedStyle(probe).color;
    var m = got.match(/(\d+),\s*(\d+),\s*(\d+)/);
    return m ? 'rgb(' + m[1] + ',' + m[2] + ',' + m[3] + ')' : fallback;
  }
  var COLOR_MAIN = resolve('--graph-main', '#4b7a43');
  var COLOR_EDGE = resolve('--graph-edge', '#c2c2c2');
  document.body.removeChild(probe);

  // ── data islands ───────────────────────────────────────────────────
  function readIsland(id) {
    var el = document.getElementById(id);
    if (!el) { return null; }
    try { return JSON.parse(el.textContent || 'null'); } catch (e) { return null; }
  }

  var islands = {};
  (function collect() {
    var ids = document.querySelectorAll('[id^="graph-data-"]');
    for (var i = 0; i < ids.length; i++) {
      var id = ids[i].id;
      var d = id.replace('graph-data-', '');
      if (d === 'global') { continue; }
      var data = readIsland(id);
      if (data && data.nodes && data.nodes.length) { islands[d] = data; }
    }
  })();

  // The slider's range comes from the data actually present, so a graph that stops at depth 2
  // does not offer a third position that renders nothing.
  //
  // 滑块范围取自真实存在的数据，因此一个只到深度 2 的图不会提供一个渲染出空白的第三档。
  var DEPTHS = Object.keys(islands).map(Number).sort(function(a, b){ return a - b; });
  if (!DEPTHS.length) { return; }
  var depth = DEPTHS.indexOf(parseInt(slider && slider.value, 10)) >= 0
    ? parseInt(slider.value, 10) : DEPTHS[DEPTHS.length - 1];
  if (slider) {
    slider.min = DEPTHS[0];
    slider.max = DEPTHS[DEPTHS.length - 1];
    slider.value = depth;
    slider.disabled = DEPTHS.length < 2;
  }
  if (depthOut) { depthOut.textContent = depth; }

  var globalData = null;       // fetched on first use // 首次使用时获取
  var globalFetching = false;
  var globalCallbacks = [];

  function withGlobalData(cb) {
    if (globalData) { cb(globalData); return; }
    globalCallbacks.push(cb);
    if (globalFetching) { return; }
    globalFetching = true;
    fetch('/blog/graph?scope=global&format=json', { credentials: 'same-origin' })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (data) {
        globalData = data;
        var waiting = globalCallbacks; globalCallbacks = [];
        waiting.forEach(function (fn) { fn(data); });
      })
      .catch(function () {
        var waiting = globalCallbacks; globalCallbacks = [];
        waiting.forEach(function (fn) { fn(null); });
      });
  }

  // ── mount ──────────────────────────────────────────────────────────
  // One function builds a complete, independent graph in a container. The card and the fullscreen
  // layer are two mounts of the same code, so they cannot drift apart in behaviour — and only the
  // fixed fullscreen mount needs a resize listener.
  //
  // 一个函数在容器里构建一幅完整且独立的图。卡片与全屏浮层是同一段代码的两次挂载，
  // 因此它们的行为不会各自漂移；只有尺寸固定的浮层挂载需要 resize 监听。
  function mountGraph(host, data, opts) {
    opts = opts || {};
    if (!host || !data || !data.nodes || !data.nodes.length) { return null; }
    var W = host.clientWidth || 0;
    var H = host.clientHeight || 0;
    // Width is a hard requirement — without it there is no horizontal space to lay anything out.
    // Height falls back rather than refusing: a flex child can legitimately report 0 while its
    // parent is still settling, and refusing there is what left the fullscreen layer empty.
    //
    // 宽度是硬条件——没有它就没有横向空间可布局。
    // 高度改为回退而不是拒绝：flex 子项在父级尚未稳定时确实可能报 0，
    // 而在这里拒绝正是让全屏浮层空着的原因。
    if (W < 40) { return null; }
    if (H < 40) {
      var ph = host.parentElement ? host.parentElement.clientHeight : 0;
      H = ph > 80 ? ph : 380;
    }

    while (host.firstChild) { host.removeChild(host.firstChild); }

    var svg = document.createElementNS(NS, 'svg');
    svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
    svg.setAttribute('role', 'img');
    svg.setAttribute('aria-label', '知识图谱');
    svg.classList.add('graph-svg');
    var gLinks = document.createElementNS(NS, 'g');
    gLinks.setAttribute('class', 'g-edges');
    var gNodes = document.createElementNS(NS, 'g');
    gNodes.setAttribute('class', 'g-nodes');
    svg.appendChild(gLinks);
    svg.appendChild(gNodes);
    host.appendChild(svg);

    // Project the server's coordinates into this container so the first frame is already
    // sensible and matches the no-JavaScript picture.
    // 把服务端坐标投影到本容器，使第一帧就已合理，且与无 JavaScript 的图一致。
    var sc = 1;
    if (data.w > 0 && data.h > 0) { sc = Math.min(W / data.w, H / data.h); }
    var ox = (W - data.w * sc) / 2, oy = (H - data.h * sc) / 2;

    var nodes = data.nodes.map(function (n) {
      return {
        id: n.id, label: n.label, url: n.url || '', r: n.r || 7,
        hop: n.hop || 0, focus: !!n.focus,
        x: (n.x || 0) * sc + ox, y: (n.y || 0) * sc + oy,
        el: null, body: null, labelEl: null,
        baseAlpha: 1, targetAlpha: 1, curAlpha: 1, active: false
      };
    });
    var byId = {};
    nodes.forEach(function (n) { byId[n.id] = n; });

    var links = data.edges.map(function (e) {
      var line = document.createElementNS(NS, 'line');
      line.setAttribute('class', 'g-edge');
      line.setAttribute('stroke', COLOR_EDGE);
      gLinks.appendChild(line);
      return {
        source: e.source, target: e.target, dir: e.dir || '', hop: e.hop || 1,
        el: line, baseAlpha: 0.4, targetAlpha: 0.4, curAlpha: 0.4, active: false
      };
    }).filter(function (l) { return byId[l.source] && byId[l.target]; });

    nodes.forEach(function (n) {
      var g = document.createElementNS(NS, 'g');
      g.setAttribute('class', 'g-node' + (n.focus ? ' focus' : ''));
      // Identity lives on the element so hover never has to search an array. The array search was
      // the original approach and it failed invisibly: the event arrived, closest() resolved, and
      // the loop still matched nothing.
      // 身份写在元素上，使悬停无需搜索数组。那个搜索是原本的做法，它以完全不可见的方式失败了：
      // 事件到了、closest() 解析成功，循环却什么都没匹配上。
      g.setAttribute('data-node-id', n.id);

      if (n.url) {
        var a = document.createElementNS(NS, 'a');
        a.setAttribute('href', n.url);
        a.appendChild(g);
        n.el = a;
      } else {
        n.el = g;
      }
      n.body = g;

      // Transparent hit target larger than the dot: the dot is ~13px across, and the caption
      // above it is pointer-events:none, so the area a reader most naturally aims at would
      // otherwise do nothing.
      // 透明的命中区比圆点大：圆点直径约 13px，而它上方的标签是 pointer-events:none，
      // 否则读者最自然会指向的区域毫无反应。
      var hit = document.createElementNS(NS, 'circle');
      hit.setAttribute('r', n.r + 6);
      hit.setAttribute('class', 'g-node-hit');
      g.appendChild(hit);

      var c = document.createElementNS(NS, 'circle');
      c.setAttribute('r', n.r);
      c.setAttribute('class', 'g-node-dot');
      var t = document.createElementNS(NS, 'title');
      t.textContent = n.label;
      c.appendChild(t);
      g.appendChild(c);

      if (n.focus) {
        var ring = document.createElementNS(NS, 'circle');
        ring.setAttribute('r', n.r + 1.5);
        ring.setAttribute('class', 'g-node-ring');
        g.insertBefore(ring, c);
      }

      // Labels start hidden and fade in with zoom, as the reference does.
      // 标签初始隐藏，随缩放渐显，与参考实现一致。
      var lb = document.createElementNS(NS, 'text');
      lb.setAttribute('class', 'g-node-label');
      lb.setAttribute('text-anchor', 'middle');
      lb.setAttribute('opacity', '0');
      lb.textContent = n.label.length > 14 ? n.label.slice(0, 14) + '…' : n.label;
      g.appendChild(lb);
      n.labelEl = lb;

      gNodes.appendChild(n.el);
    });

    // ── hover ────────────────────────────────────────────────────────
    var hovered = null;
    function setHover(id) {
      hovered = id;
      var near = {};
      if (id !== null) {
        near[id] = true;
        links.forEach(function (l) {
          if (l.source === id || l.target === id) { near[l.source] = true; near[l.target] = true; }
        });
      }
      links.forEach(function (l) {
        l.active = id !== null && (l.source === id || l.target === id);
        l.targetAlpha = id === null ? l.baseAlpha : (l.active ? 0.8 : 0.05);
      });
      nodes.forEach(function (n) {
        n.active = id !== null && !!near[n.id] && n.id !== id;
        n.targetAlpha = id === null ? n.baseAlpha : (near[n.id] ? 1 : 0.1);
      });
      wake();
    }

    function nodeIdFrom(ev) {
      var el = ev.target;
      if (!el || !el.closest) { return null; }
      var g = el.closest('.g-node');
      return g ? g.getAttribute('data-node-id') : null;
    }

    function onOver(ev) { var id = nodeIdFrom(ev); if (id) { setHover(id); } }
    function onOut(ev) {
      var to = ev.relatedTarget;
      if (to && to.closest && to.closest('.g-node')) { return; }
      setHover(null);
    }
    svg.addEventListener('pointerover', onOver, true);
    svg.addEventListener('pointerout', onOut, true);

    // ── physics ──────────────────────────────────────────────────────
    // charge / link distance / radial strength / collide iterations follow the reference. The one
    // addition is that a link's rest length grows with hop, so a deeper relationship settles
    // further out — the requirement this graph has and the reference does not.
    // charge / 链接距离 / 径向强度 / collide 迭代次数取自参考实现。
    // 唯一增加的是边的静止长度随 hop 增长，使关系越深落得越远——
    // 这是本图有、而参考实现没有的需求。
    var LINK_BASE = opts.linkBase || 46;
    var LINK_PER_HOP = 34;
    var simNodes = nodes.map(function (n) {
      return { id: n.id, x: n.x, y: n.y, r: n.r, node: n };
    });
    var simById = {};
    simNodes.forEach(function (s) { simById[s.id] = s; });
    var simLinks = links.map(function (l) {
      return {
        source: l.source, target: l.target,
        distance: LINK_BASE + Math.max(0, l.hop - 1) * LINK_PER_HOP,
        link: l
      };
    });

    var radius = Math.min(W, H) / 3;
    var simulation = forceSimulation(simNodes)
      .force('charge', forceManyBody().strength(-170))
      .force('center', forceCenter(W / 2, H / 2).strength(0.5))
      .force('link', forceLink(simLinks).id(function (d) { return d.id; })
        .distance(function (l) { return l.distance; }))
      .force('collide', forceCollide(function (d) { return d.r + 3; }).iterations(3))
      .force('radial', forceRadial(radius, W / 2, H / 2).strength(0.05))
      .stop();

    // The simulation always runs. It used to be pre-run to rest under prefers-reduced-motion,
    // which meant the graph appeared already settled and nothing ever moved — the exact behaviour
    // reported as "still static". The force layout is not decoration: it shows which notes belong
    // together, so removing the motion removes the feature. The reference does not check the
    // setting at all.
    //
    // 模拟始终运行。它原先在 prefers-reduced-motion 下预先跑到静止，
    // 于是图一出现就是收敛后的样子、什么都不动——正是被报告为「一直静态」的那个表现。
    // 力导向布局不是装饰：它展示哪些笔记彼此相关，去掉动效等于去掉功能。
    // 参考实现完全不检查这个设置。

    // ── paint ────────────────────────────────────────────────────────
    var SMOOTH = 0.15;
    var raf = null;
    // Declared here, above the loop that reads them: var hoisting would otherwise leave them
    // undefined until their own line runs, which is fine for a boolean but not for reasoning about
    // when the loop may first see them.
    // 声明在读取它们的循环之前：否则 var 提升会让它们在自己的那一行执行前保持 undefined，
    // 对布尔值虽无碍，但不利于判断循环第一次看到它们时是什么状态。
    var fitted = false;
    var fitInFlight = false;

    // "Is the simulation still moving?" must consider alphaTarget as well as alpha: while a node
    // is held, alphaTarget is 1 and the graph is deliberately at full energy, so testing alpha
    // alone could report quiet and let the loop skip repaints in the middle of a drag.
    //
    // 「模拟是否仍在运动」必须同时看 alphaTarget 与 alpha：节点被按住时 alphaTarget 为 1，
    // 图被刻意保持在满能量，只看 alpha 可能报告为已静默，使循环在拖动途中跳过重绘。
    function needsPaint() {      if (simulation.alphaTarget() > 0 || simulation.alpha() > simulation.alphaMin()) { return true; }
      for (var i = 0; i < nodes.length; i++) {
        if (Math.abs(nodes[i].curAlpha - nodes[i].targetAlpha) > 0.002) { return true; }
      }
      for (var j = 0; j < links.length; j++) {
        if (Math.abs(links[j].curAlpha - links[j].targetAlpha) > 0.002) { return true; }
      }
      return false;
    }

    function paint() {
      for (var i = 0; i < nodes.length; i++) {
        var n = nodes[i];
        var s = simById[n.id];
        if (!s) { continue; }
        n.curAlpha += (n.targetAlpha - n.curAlpha) * SMOOTH;
        n.body.setAttribute('transform', 'translate(' + s.x.toFixed(2) + ',' + s.y.toFixed(2) + ')');
        n.body.setAttribute('opacity', n.curAlpha.toFixed(3));
        if (n.labelEl) {
          n.labelEl.setAttribute('x', '0');
          n.labelEl.setAttribute('y', (-n.r - 4).toFixed(2));
        }
      }
      for (var j = 0; j < links.length; j++) {
        var l = links[j];
        var a = simById[l.source], b = simById[l.target];
        if (!a || !b) { continue; }
        l.curAlpha += (l.targetAlpha - l.curAlpha) * SMOOTH;
        var ux = b.x - a.x, uy = b.y - a.y;
        var d = Math.sqrt(ux * ux + uy * uy) || 1;
        var trim = Math.min(b.r, d - 1);
        l.el.setAttribute('x1', a.x.toFixed(2));
        l.el.setAttribute('y1', a.y.toFixed(2));
        l.el.setAttribute('x2', (b.x - ux / d * trim).toFixed(2));
        l.el.setAttribute('y2', (b.y - uy / d * trim).toFixed(2));
        l.el.setAttribute('stroke-opacity', l.curAlpha.toFixed(3));
        // Colour follows activity: a hovered note's links light up and the rest recede. Opacity
        // alone reads as "fading out", which is half the message.
        // 颜色跟随活跃状态：被悬停笔记的链接亮起，其余退开。只改不透明度读起来是「正在淡出」，那是半个信息。
        l.el.setAttribute('stroke', l.active ? COLOR_MAIN : COLOR_EDGE);
      }
    }

    // The loop advances the simulation itself.
    //
    // It was created with .stop(), which disables d3's internal timer on purpose so a render loop
    // can own the timing — but the loop then only called paint(), which reads coordinates and
    // never changes them, so positions stayed at their server values forever and the graph was
    // motionless while repainting sixty times a second.
    //
    // 循环自己推进模拟。
    //
    // 模拟以 .stop() 创建，刻意关掉 d3 的内部定时器，好让渲染循环掌握时序——
    // 但循环随后只调 paint()，而它读坐标、从不改坐标，于是位置永远停在服务端给的值上，
    // 图在每秒六十次重绘的同时纹丝不动。
    function ticked() {
      if (simulation.alphaTarget() > 0 || simulation.alpha() > simulation.alphaMin()) {
        simulation.tick();
      }
      // Fit once the layout is close enough to its final shape.
      //
      // Waiting for alpha to fall all the way to alphaMin took about eight seconds in practice —
      // the loop advances the simulation one tick per frame, so the wait stretches with whatever
      // the frame rate happens to be. For those eight seconds the nodes keep drifting, and then
      // the view abruptly reframes around wherever they ended up. Fitting at a small threshold
      // instead makes the change land while the layout is already visually settled, and the
      // remaining drift is a few pixels that nobody notices.
      //
      // 在布局足够接近最终形态时就适配。
      //
      // 等 alpha 一路降到 alphaMin，实测要八秒左右——循环每帧只推进一次模拟，
      // 于是等待时长会随帧率伸缩。这八秒里节点持续漂移，然后视图突然按它们最终的位置重新取景。
      // 改成在一个很小的阈值处适配，这次变化就发生在布局视觉上已经安定时，
      // 残留的漂移只有几个像素，没人会察觉。
      if (!fitted && !fitInFlight && simulation.alpha() <= 0.02) {
        fitted = true;
        fitInFlight = true;
        animateFit();
      }
      if (needsPaint()) { paint(); }
      raf = requestAnimationFrame(ticked);
    }

    function wake() {
      if (raf) { return; }
      raf = requestAnimationFrame(ticked);
    }

    paint();
    // Give the layout a moment of real physics before it settles.
    // 让布局先跑一小段真实物理再自行收敛。
    // A one-shot alpha for the opening run, and alphaTarget only for the drag.
    //
    // Both forms were measured. A one-shot alpha decays on its own, so the opening animation
    // settles by itself — which is what should happen. alphaTarget is different: it pulls alpha
    // toward a fixed level every tick, so setting it here as well would keep the graph in motion
    // forever and it would never come to rest. The reference uses alphaTarget only for the drag
    // (graphScript.njk:318 for start, :332 for end) and lets the initial run decay naturally.
    //
    // 开场用一次性的 alpha，alphaTarget 只用于拖动。
    //
    // 两种写法都实测过。一次性 alpha 会自行衰减，开场动画自己收敛下来——这本来就该发生。
    // alphaTarget 不同：它每帧把 alpha 拉向一个固定值，所以在这里一并设置会让图永远处于运动状态、
    // 永不静止。参考实现只在拖动时用 alphaTarget（graphScript.njk:318 开始、:332 结束），
    // 初始运行则交由自然衰减。
    simulation.alpha(0.9);
    wake();

    // ── zoom ─────────────────────────────────────────────────────────
    var currentTransform = zoomIdentity;

    function applyZoom(t) {
      currentTransform = t;
      gLinks.setAttribute('transform', 'translate(' + t.x + ',' + t.y + ') scale(' + t.k + ')');
      gNodes.setAttribute('transform', 'translate(' + t.x + ',' + t.y + ') scale(' + t.k + ')');
      var labelAlpha = Math.max((t.k - 1) / 3.75, 0);
      nodes.forEach(function (n) {
        if (!n.labelEl) { return; }
        if (n.id === hovered) { n.labelEl.setAttribute('opacity', '1'); return; }
        n.labelEl.setAttribute('opacity', labelAlpha.toFixed(3));
      });
    }

    // Zoom owns pan; the node drag below owns node dragging.
    //
    // These two gestures share mousedown, so something has to decide between them. The decision
    // belongs to the drag filter, not to the zoom filter: d3-zoom's filter runs first and, when it
    // returns false, d3-zoom calls stopImmediatePropagation — which also silences the drag handler
    // registered on the same element. Filtering nodes out of zoom therefore disabled both gestures
    // at once. Zoom is left with its default filter and simply never sees a node press because the
    // drag handler stops propagation on the way.
    //
    // 缩放负责平移，下面的节点拖动负责拖节点。
    //
    // 两者共用 mousedown，必须有东西在它们之间做决断。这个决断属于拖动的 filter，不属于缩放的 filter：
    // d3-zoom 的 filter 先执行，而它返回 false 时会调用 stopImmediatePropagation ——
    // 这同时会掐掉注册在同一元素上的拖动处理器。把节点从缩放里排除，等于一次禁掉了两种手势。
    // 因此让缩放沿用默认 filter；节点按下时由拖动处理器在传播途中截住，缩放根本不会看到它。
    var zoomBehavior = zoom()
      .extent([[0, 0], [W, H]])
      .scaleExtent([0.25, 4])
      .on('zoom', function (ev) { applyZoom(ev.transform); });

    select(svg).call(zoomBehavior);

    // ── drag ─────────────────────────────────────────────────────────
    // Bound to each node group, not to the svg.
    //
    // The reference binds it the same way. Bound to the container, d3-drag receives every
    // pointerdown on the surface; the filter keeps non-node drags out of the node logic, but the
    // zoom listener on the same element still sees the event and pans. Per-node binding also
    // gives each node the correct subject without a closest() lookup.
    //
    // 绑定在每个节点组上，而不是 svg。
    //
    // 参考实现也是这么绑的。绑在容器上时，d3-drag 会收到整个平面上的每一次 pointerdown；
    // filter 虽能把非节点的拖动挡在节点逻辑之外，但同一元素上的 zoom 监听器仍然会收到事件并平移。
    // 逐节点绑定还让每个节点自带正确的 subject，无需 closest() 查找。
    var dragStartTime = 0, dragStartPos = null, moved = false;
    // Pointer velocity, used only to carry a flick past the release.
    // 指针速度，仅用于把"甩"延续到松手之后。
    var velX = 0, velY = 0, lastMoveX = 0, lastMoveY = 0, lastMoveTime = 0;

    var dragBehavior = drag()
      .container(function () { return svg; })
      .subject(function (ev) {
        var g = ev.target && ev.target.closest ? ev.target.closest('.g-node') : null;
        if (!g) { return null; }
        var s = simById[g.getAttribute('data-node-id')];
        if (!s) { return null; }
        return {
          x: s.x * currentTransform.k + currentTransform.x,
          y: s.y * currentTransform.k + currentTransform.y,
          sim: s, node: s.node
        };
      })
        .on('start', function (ev) {
          if (!ev.subject) { return; }
          // Keep the press from also reaching the zoom behaviour on the svg.
          //
          // d3-drag stops immediate propagation, which suppresses other listeners on the same
          // element — but it does not stop bubbling, so the svg's zoom/mousedown still fires and
          // pans while a node is being dragged. That is the "it just slides around" behaviour.
          // Stopping propagation here, on the node, is what separates the two gestures.
          //
          // 阻止这次按下继续传到 svg 上的缩放行为。
          //
          // d3-drag 会 stopImmediatePropagation，它压掉同一元素上的其他监听器，
          // 但不阻止冒泡 —— 于是 svg 的 zoom/mousedown 照样触发，拖节点时整张图跟着平移。
          // 这就是"图在滑动"的观感。在节点这一层截住传播，两种手势才分得开。
          if (ev.sourceEvent && ev.sourceEvent.stopPropagation) { ev.sourceEvent.stopPropagation(); }
          ev.subject.sim.fx = ev.subject.sim.x;
          ev.subject.sim.fy = ev.subject.sim.y;
          dragStartTime = Date.now();
          dragStartPos = { x: ev.x, y: ev.y };
          moved = false;
          velX = 0; velY = 0;
          lastMoveX = ev.x; lastMoveY = ev.y; lastMoveTime = Date.now();
          // Hold the simulation at full energy for the whole drag, as the reference does.
          // A one-shot alpha() here decayed away mid-drag, after which the rest of the graph
          // stopped responding to the node being moved.
          //
          // 在整段拖动期间把模拟保持在满能量，与参考实现一致。
          // 这里用一次性的 alpha() 会在拖动途中衰减掉，此后图的其余部分不再对被拖动的节点做出反应。
          simulation.alphaTarget(1);
          wake();
        })
        .on('drag', function (ev) {
          if (!ev.subject) { return; }
          ev.subject.sim.fx = (ev.x - currentTransform.x) / currentTransform.k;
          ev.subject.sim.fy = (ev.y - currentTransform.y) / currentTransform.k;
          // Track pointer velocity so a flick can be carried into the release.
          // 记录指针速度，使"甩"这个动作能延续到松手之后。
          var now = Date.now();
          var dtms = now - lastMoveTime;
          if (dtms > 0) {
            var vx = (ev.x - lastMoveX) / dtms, vy = (ev.y - lastMoveY) / dtms;
            // Smooth slightly: a single frame's delta is noisy, and the flick should feel like a
            // direction, not like the last millisecond of jitter.
            // 稍作平滑：单帧位移有噪声，而"甩"应当是一种方向感，而不是最后一毫秒的抖动。
            velX = velX * 0.4 + vx * 0.6;
            velY = velY * 0.4 + vy * 0.6;
          }
          lastMoveX = ev.x; lastMoveY = ev.y; lastMoveTime = now;
          moved = true;
        })
        .on('end', function (ev) {
          if (!ev.subject) { return; }
          var sim = ev.subject.sim;
          // Hand the flick to the node before releasing it.
          //
          // With fx/fy cleared outright, a flick stops dead: the node is let go at zero velocity
          // and the layout simply re-balances around where it happened to be. Continuity of motion
          // is what makes a drag read as physical rather than as teleporting a dot. So the release
          // keeps the node pinned for a short carry, advancing it every frame by a velocity that
          // decays linearly, and only then clears fx/fy and lets the forces take over.
          //
          // 松手前把"甩"的动量交给节点。
          //
          // 直接清掉 fx/fy 时，一次甩动会戛然而止：节点以零速度被放开，布局只是在它恰好停下的位置重新平衡。
          // 运动的连续性才让拖动读起来像是物理动作，而不是把一个小圆点瞬移过去。
          // 因此松手后先保持节点被钉住一小段时间，每帧按线性衰减的速度推进它，
          // 然后再清掉 fx/fy，交给力模型接管。
          var carry = STILL ? 0 : 340;   // ms // 毫秒
          if (carry > 0 && (Math.abs(velX) > 0.05 || Math.abs(velY) > 0.05)) {
            var simX = (ev.x - currentTransform.x) / currentTransform.k;
            var simY = (ev.y - currentTransform.y) / currentTransform.k;
            var vSimX = velX / currentTransform.k, vSimY = velY / currentTransform.k;
            var t0 = Date.now();
            var carryTick = function () {
              var p = (Date.now() - t0) / carry;
              if (p >= 1) {
                sim.fx = null; sim.fy = null;
                simulation.alphaTarget(0);
                return;
              }
              // Linear decay: full velocity at release, zero when the carry ends.
              // 线性衰减：松手瞬间是全速，滑行结束时归零。
              var v = 1 - p;
              sim.fx = sim.x + vSimX * (carry / 1000) * v * 0.35;
              sim.fy = sim.y + vSimY * (carry / 1000) * v * 0.35;
              window.requestAnimationFrame(carryTick);
            };
            // Keep the node pinned to the pointer's last position for the first frame.
            // 第一帧先把节点固定在指针的最后位置。
            sim.fx = simX; sim.fy = simY;
            simulation.alphaTarget(0.35);
            wake();
            window.requestAnimationFrame(carryTick);
          } else {
            sim.fx = null; sim.fy = null;
            simulation.alphaTarget(0);
          }
          velX = 0; velY = 0;
          var dt = Date.now() - dragStartTime;
          var dx = ev.x - dragStartPos.x, dy = ev.y - dragStartPos.y;
          if (dt < 500 && Math.sqrt(dx * dx + dy * dy) < 5 && ev.subject.node.url) {
            window.location = ev.subject.node.url;
          }
        });

    // One binding per node. See the note above on why this is not a container binding.
    // 逐节点各绑一次。为何不是容器绑定，见上。
    nodes.forEach(function (n) { select(n.body).call(dragBehavior); });

    // Suppress the click that ends a drag so it does not also navigate via the <a> wrapper.
    // 阻止结束拖动的那次点击经由 <a> 包装再次跳转。
    svg.addEventListener('click', function (ev) {
      if (moved) { ev.preventDefault(); ev.stopPropagation(); moved = false; }
    }, true);

    // ── zoom to fit ──────────────────────────────────────────────────
    function fitTarget() {
      var minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
      simNodes.forEach(function (s) {
        if (s.x == null) { return; }
        minX = Math.min(minX, s.x); maxX = Math.max(maxX, s.x);
        minY = Math.min(minY, s.y); maxY = Math.max(maxY, s.y);
      });
      if (minX === Infinity) { return null; }
      var pad = opts.fitPad || 56;
      var gw = (maxX - minX) + pad * 2, gh = (maxY - minY) + pad * 2;
      var k = Math.min(W / gw, H / gh, 2);
      var cx = (minX + maxX) / 2, cy = (minY + maxY) / 2;
      return zoomIdentity.translate(W / 2 - cx * k, H / 2 - cy * k).scale(k);
    }

    // Fit by interpolating the transform frame by frame, not by assigning it once.
    //
    // Assigning it once is what produced the reported jump: zoomBehavior.transform applies the new
    // scale immediately, so the graph snapped to a different size with no motion at all, and the
    // snap arrived a fixed 3.2s after the graph appeared — unrelated to when the layout actually
    // settled. Both halves were wrong. The target is now taken once the simulation has come to
    // rest, and it is approached over ~600ms so the change reads as the view settling rather than
    // as a glitch.
    //
    // Driving it through zoomBehavior.transform every frame (rather than writing the transform
    // straight onto the groups) keeps d3-zoom's internal state in step, so a reader who scrolls
    // immediately afterwards continues from where the fit left off instead of from the stale
    // pre-fit scale.
    //
    // 逐帧插值完成适配，而不是一次性赋值。
    //
    // 一次性赋值正是被报告的那次跳变：zoomBehavior.transform 立刻套用新缩放，
    // 图毫无过渡地换了个大小；而且它固定在图出现后 3.2 秒到达，与布局真正收敛的时刻无关。
    // 两头都不对。现在目标值在模拟静止后才取，并用约 600ms 逼近，
    // 于是这次变化读起来是视图安定下来，而不是一次故障。
    //
    // 每帧经由 zoomBehavior.transform 驱动（而不是直接把 transform 写到分组上），
    // 是为了让 d3-zoom 的内部状态同步；这样读者紧接着滚动时，会从适配结束的位置继续，
    // 而不是从一个过期的适配前缩放继续。
    function animateFit() {
      var target = fitTarget();
      if (!target) { fitInFlight = false; return; }
      var start = currentTransform;
      var t0 = (window.performance && performance.now) ? performance.now() : Date.now();
      var dur = STILL ? 0 : 600;
      if (dur === 0) {
        select(svg).call(zoomBehavior.transform, target);
        fitInFlight = false;
        return;
      }
      var step = function (now) {
        var p = Math.min((now - t0) / dur, 1);
        var e = 1 - Math.pow(1 - p, 3);   // easeOutCubic // 三次缓出
        var k = start.k + (target.k - start.k) * e;
        var x = start.x + (target.x - start.x) * e;
        var y = start.y + (target.y - start.y) * e;
        select(svg).call(zoomBehavior.transform, zoomIdentity.translate(x, y).scale(k));
        if (p < 1) { window.requestAnimationFrame(step); }
        else { fitInFlight = false; }
      };
      window.requestAnimationFrame(step);
    }

    // Public: the fullscreen mount refits on resize.
    function fitNow() { animateFit(); }

    return {
      // Exposed so the shell can drive the depth slider without rebuilding the container.
      // 暴露给外壳，使深度滑块无需重建容器即可驱动它。
      destroy: function () {
        if (raf) { cancelAnimationFrame(raf); raf = null; }
        simulation.stop();
        select(svg).on('.zoom', null).on('.drag', null);
        while (host.firstChild) { host.removeChild(host.firstChild); }
      },
      refit: fitNow
    };
  }

  // ── card mount, driven by the depth slider ─────────────────────────
  var cardMount = null;

  function mountCard() {
    if (cardMount) { cardMount.destroy(); cardMount = null; }
    var data = islands[String(depth)];
    if (!data) { return; }
    if (canvas) { canvas.hidden = false; }
    cardMount = mountGraph(canvas, data, {});
  }

  var mountTimer = null;
  function scheduleMountCard() {
    // Rebuilding on every input event would rebuild the whole simulation dozens of times during a
    // single drag. Debouncing keeps the control responsive without that waste.
    // 每次 input 都重建会在一次拖动中重建整场模拟几十次。防抖让控件保持跟手而不浪费。
    if (mountTimer) { clearTimeout(mountTimer); }
    mountTimer = setTimeout(mountCard, 120);
  }

  if (slider) {
    slider.addEventListener('input', function () {
      depth = parseInt(slider.value, 10) || DEPTHS[DEPTHS.length - 1];
      if (depthOut) { depthOut.textContent = String(depth); }
      scheduleMountCard();
    });
    slider.addEventListener('change', function () {
      depth = parseInt(slider.value, 10) || DEPTHS[DEPTHS.length - 1];
      if (depthOut) { depthOut.textContent = String(depth); }
      scheduleMountCard();
    });
  }

  // ── fullscreen layer ───────────────────────────────────────────────
  var layerMount = null;
  var lastFocus = null;

  function openLayer(kind) {
    if (!DESKTOP.matches || !layer) { return; }
    lastFocus = document.activeElement;
    layer.hidden = false;
    if (layerTitle) {
      layerTitle.textContent = kind === 'global' ? '全站图谱' : '本文关联';
    }
    if (layerBody) {
      layerBody.innerHTML = '<div class="graphlayer-loading">正在生成图谱…</div>';
    }
    if (!STILL) {
      layer.classList.remove('is-in');
      void layer.offsetWidth;
      layer.classList.add('is-in');
    }
    openPanel();

    function draw(data) {
      if (!layerBody) { return; }
      if (!data || !data.nodes || !data.nodes.length) {
        layerBody.innerHTML = '<div class="graphlayer-error">图谱暂时无法加载。</div>';
        return;
      }
      layerBody.innerHTML = '';
      var host = document.createElement('div');
      host.className = 'graphlayer-host';
      layerBody.appendChild(host);
      // Two frames: the layer was display:none until a moment ago, so its container has no
      // measurable height yet and the mount guard would refuse. Measured before this fix: the
      // layer opened with nodes=0 because the guard saw a zero-sized host.
      //
      // 过两帧：浮层在此前是 display:none，容器还没有可测量的高度，挂载守卫会拒绝。
      // 修复前实测：浮层打开后 nodes=0，因为守卫看到的容器尺寸为零。
      requestAnimationFrame(function () { requestAnimationFrame(function () {
        if (layerBody.firstChild !== host) { return; }
        layerMount = mountGraph(host, data, { linkBase: 80, fitPad: 80 });
      }); });
    }

    if (kind === 'global') {
      withGlobalData(draw);
    } else {
      draw(islands[String(depth)]);
    }
  }

  function closeLayer() {
    if (!layer || layer.hidden) { return; }
    layer.hidden = true;
    layer.classList.remove('is-in');
    if (layerMount) { layerMount.destroy(); layerMount = null; }
    if (layerBody) { layerBody.innerHTML = ''; }
    if (lastFocus && lastFocus.focus) { lastFocus.focus(); }
  }

  if (btnFull) { btnFull.addEventListener('click', function () { openLayer('global'); }); }
  if (btnExpand) { btnExpand.addEventListener('click', function () { openLayer('local'); }); }
  if (layerClose) { layerClose.addEventListener('click', closeLayer); }
  if (layer) {
    layer.addEventListener('click', function (ev) { if (ev.target === layer) { closeLayer(); } });
  }
  // Leaving the desktop breakpoint with a modal open that its own controls can no longer dismiss
  // is a trap.
  // 在浮层打开时缩到桌面断点以下，而它自己的控件已无法关闭它，是个陷阱。
  function onBreak(e) { if (!e.matches) { closeLayer(); } }
  if (DESKTOP.addEventListener) { DESKTOP.addEventListener('change', onBreak); }
  else if (DESKTOP.addListener) { DESKTOP.addListener(onBreak); }

  // ── panel open / close ─────────────────────────────────────────────
  // A copy of the ask panel's open/close, including the two-frame trick and the guarded
  // transitionend: the panel is laid out with hidden=false first, then gr-on hides the pill, and
  // only after two animation frames does gr-in start the transition — adding it in the same frame
  // would animate from a state the browser never painted.
  //
  // 提问面板 open/close 的复制，连双帧技巧与带兜底的 transitionend 一起：
  // 先 hidden=false 让面板完成布局，再由 gr-on 隐藏胶囊，只有过了两帧才加 gr-in 启动过渡——
  // 在同一帧里加，动画会从一个浏览器从未绘制过的状态开始。
  var OPEN = false;

  function openPanel() {
    if (OPEN) { return; }
    OPEN = true;
    if (backdrop) { backdrop.hidden = false; }
    panel.hidden = false;
    fab.setAttribute('aria-expanded', 'true');
    root.classList.add('gr-on');
    requestAnimationFrame(function () { requestAnimationFrame(function () {
      root.classList.add('gr-in');
      // The card was display:none until this moment, so it measured 0x0 and nothing could be
      // drawn. Now that it is laid out, build it.
      // 卡片在此刻之前是 display:none，量到 0x0，什么都画不出来。现在它已布局，构建它。
      if (!cardMount) { mountCard(); }
    }); });
  }

  function closePanel() {
    if (!OPEN) { return; }
    OPEN = false;
    root.classList.remove('gr-in');
    fab.setAttribute('aria-expanded', 'false');
    var settled = false;
    var guard = 0;
    var done = function () {
      if (settled) { return; }
      settled = true;
      clearTimeout(guard);
      panel.hidden = true;
      if (backdrop) { backdrop.hidden = true; }
      root.classList.remove('gr-on');
      panel.removeEventListener('transitionend', done);
    };
    if (STILL) { done(); return; }
    guard = setTimeout(done, 420);
    panel.addEventListener('transitionend', done);
  }

  fab.addEventListener('click', openPanel);
  if (collapseBtn) { collapseBtn.addEventListener('click', closePanel); }
  if (backdrop) { backdrop.addEventListener('click', closePanel); }

  // One Escape handler for both layers, in priority order.
  //
  // Two separate handlers both firing on the same keydown is a real defect, not a tidiness
  // question: the layer's handler ran first and set layer.hidden = true, and the panel's handler
  // — whose guard was "close only if no layer is open" — then read that freshly-updated state as
  // "no layer is open" and closed the panel too. Measured consequence: a single Escape closed
  // both, leaving the panel's own buttons unreachable and looking like broken controls.
  //
  // 浮层与面板共用同一个 Escape 处理器，按优先级处理。
  //
  // 两个独立处理器在同一次 keydown 上都触发，这是真实缺陷而不是整洁问题：
  // 浮层处理器先跑并把 layer.hidden 置为 true，而面板处理器的守卫是「没有浮层打开时才关闭」——
  // 它随即把刚被更新的状态读成「没有浮层打开」，于是连面板一起关掉。
  // 实测后果：一次 Escape 关掉两者，面板自己的按钮随即不可达，看起来像控件坏了。
  document.addEventListener('keydown', function (ev) {
    if (ev.key !== 'Escape') { return; }
    if (layer && !layer.hidden) { closeLayer(); return; }
    if (OPEN) { closePanel(); }
  });

  // The fullscreen mount has a fixed size only after the layer is visible; refit on resize.
  // 浮层挂载只有在浮层可见后才有确定尺寸；尺寸变化时重新适配。
  window.addEventListener('resize', function () {
    if (layerMount) { layerMount.refit(); }
  });
})();`
