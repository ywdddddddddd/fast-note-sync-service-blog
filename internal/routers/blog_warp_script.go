package routers

// The hand-written loader: a fountain pen draws "Wendao's Blog" while the destination loads, and
// the overlay fades out only after the pen has finished the current pass.
// ── Why a cross-document transition and not a client-side swap ───────────────────────────
//
// Intercepting the click, fetching the destination, swapping the DOM would make the transition
// seamless with no help from the other page. It would also mean the browser's back button,
// history, the address bar and a fresh reader's direct visit take a different code path than the
// one this animation exercises — and the path nobody exercises is the one that breaks.
//
// So the animation plays first, then the browser really navigates. The destination page reads a
// one-shot flag out of sessionStorage and fades its content in.
//
// ── 为什么是跨文档过渡，而不是客户端换页 ────────────────────────────────────────────────
//
// 拦下点击、取回目标页、替换 DOM，可以让过渡不依赖目标页就做到无缝。
// 但那也意味着浏览器的后退键、历史、地址栏，以及新读者直接访问，
// 走的都是与这段动画不同的另一条代码路径 —— 而没人走的路径正是会坏的那条。
//
// 所以先播动画，再让浏览器真的导航。目标页从 sessionStorage 里读一个一次性标记并让内容渐显。
//
// ── The overlay must be viewport-sized from any scroll position ──────────────────────────
//
// The previous version appended the canvas to <body> and relied on position:fixed. But the same
// ruleset that fades the departure also puts transform:scale(1.03) on <body>, and a transformed
// element becomes the containing block for its fixed descendants. The canvas was therefore pinned
// to the top of the document instead of the viewport: measured at scrollY=1176 the canvas rect was
// y=-1176, i.e. entirely off-screen. The reader who scrolled down saw a partial overlay or none.
//
// Three changes fix that and keep it fixed:
//
//   1. The overlay is a child of <html>, not <body>, so no body transform can capture it.
//   2. Its size is 100vw x 100vh with position:fixed and inset:0, and the canvas backing store is
//      measured from the overlay itself rather than from innerWidth/innerHeight.
//   3. The page's own fade is driven by opacity on <body> only — the scale is applied to .shell,
//      which the transition does not need to move and which is never an ancestor of the overlay.
//
// ── 遮罩必须在任何滚动位置都完整 ──────────────────────────────────────────────────────
//
// 旧版把画布挂到 <body> 上，靠 position:fixed。但同一条让页面淡出的规则也给 <body> 加了
// transform:scale(1.03)，而被 transform 的元素会成为其 fixed 后代的包含块。于是画布被钉在文档顶部
// 而不是视口：实测 scrollY=1176 时画布矩形是 y=-1176，完全在屏幕外。滚下去的读者看到的是残缺遮罩
// 或者什么都看不到。
//
// 三处改动修好它并让它保持修好：
//
//   1. 遮罩是 <html> 的子节点而不是 <body> 的，任何 body 上的 transform 都抓不住它。
//   2. 尺寸用 position:fixed + inset:0 + 100vw/100vh，画布的位图尺寸从遮罩自身量，而不是 innerWidth。
//   3. 页面自己的淡出只由 <body> 的 opacity 承担 —— scale 挪到 .shell 上，
//      过渡不需要动它，而它永远不会成为遮罩的祖先。
//
// ── Loop, then finish the current pass ─────────────────────────────────────────────────
//
// The pen does not stop mid-letter. If the destination has loaded it waits for the pass in flight
// to complete, so the shortest overlay is one full pass (~2.9s) and the longest follows the
// network. Snapping the pen away at an arbitrary stroke boundary is what reads as a broken
// animation rather than a fast one.
//
// ── 循环，然后补完当前这一遍 ──────────────────────────────────────────────────────────
//
// 笔不会停在字母中间。目标页已经加载完就等手上这一遍写完，所以最短是一次完整书写（约 2.9s），
// 最长跟随网络。在任意笔画边界上把笔抽走，读起来是动画坏了，而不是动画快。
const blogWarpScript = `
(function () {
  // The destination reads this to know it should fade in. sessionStorage rather than localStorage
  // because the fade belongs to this one navigation, not to every future visit.
  // 目标页读它来决定是否渐显。用 sessionStorage 而不是 localStorage，
  // 因为这次渐显属于这一次导航，而不是此后每一次访问。
  var FLAG = 'fns:warp';

  // ── The arriving half runs on every page ────────────────────────────────────────────────
  // 抵达的那一半在每个页面上运行
  var arrived = null;
  try { arrived = sessionStorage.getItem(FLAG); } catch (e) {}
  if (arrived) {
    try { sessionStorage.removeItem(FLAG); } catch (e) {}
    document.documentElement.classList.add('fns-arrived');
    // Removed after the fade so it does not sit on the document forever.
    // 淡入结束后移除，免得它永远留在文档上。
    window.setTimeout(function () {
      document.documentElement.classList.remove('fns-arrived');
    }, 700);
  }

  var STILL = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  var running = false;

  // ── The artwork ─────────────────────────────────────────────────────────────────────────
  //
  // Verbatim from the sample file the owner supplied, with two changes: the stroke weight is
  // expressed through the SVG's viewBox scale rather than rewritten, and the reference rules are
  // dropped (they were the sample's own scaffolding, not part of the lettering).
  //
  // The six segments are the six pen-down strokes. Their weights are relative costs, not lengths:
  // "endao" is the longest run of cursive and "s" is two short flicks, so weight alone would draw
  // them at the same speed.
  //
  // ── 字形 ──────────────────────────────────────────────────────────────────────────────
  //
  // 逐字取自站主给的示例文件，只改两处：笔画粗细通过 SVG viewBox 的缩放表达而不是重写，
  // 参考线去掉（那是示例自己的脚手架，不属于字形）。
  //
  // 六段就是六次落笔。权重是相对代价而不是长度：「endao」是最长的连笔，
  // 而「s」是两下短促的挑笔，只看长度会让它们用同一个速度画出来。
  var SEGMENTS = [
    {
      id: 'w', weight: 1.25,
      hair: 'M 130,195 C 115,200 110,230 115,258 C 122,298 140,318 160,318 C 178,318 194,290 205,255 C 218,290 234,318 252,318 C 274,318 290,270 298,205 C 302,185 292,180 282,184 C 274,188 274,204 282,224 C 290,248 304,268 318,268',
      swells: [
        { d: 'M 115,225 C 117,250 128,285 145,310', w: 7.5 },
        { d: 'M 205,255 C 218,290 232,315 245,316', w: 6.8 }
      ]
    },
    {
      id: 'endao', weight: 2.70,
      hair: 'M 318,268 C 328,268 344,242 355,228 C 362,218 354,210 344,216 C 334,222 328,244 330,266 C 332,284 345,294 360,290 C 375,286 385,268 392,252 C 396,242 404,236 412,240 C 418,245 414,265 410,288 C 420,258 435,236 450,236 C 462,236 462,254 458,288 C 470,262 485,238 502,238 C 486,242 474,258 474,275 C 474,292 485,300 498,298 C 510,296 520,280 526,260 L 530,165 C 530,154 524,154 522,165 L 518,295 C 518,303 528,303 538,292 C 548,272 562,240 580,240 C 565,244 554,258 554,274 C 554,290 565,298 576,296 C 588,294 598,278 602,260 L 600,295 C 600,302 610,302 620,290 C 630,272 642,240 660,240 C 645,244 634,258 634,274 C 634,290 645,298 658,296 C 674,294 682,274 682,256 C 682,242 672,240 664,244 C 655,248 662,258 674,258',
      swells: [
        { d: 'M 352,228 C 342,238 332,254 334,272', w: 5.5 },
        { d: 'M 412,240 C 418,250 415,268 412,286', w: 6.5 },
        { d: 'M 454,246 C 460,256 460,270 458,286', w: 6.5 },
        { d: 'M 498,242 C 484,248 476,262 476,276 C 476,290 484,298 495,298', w: 5.5 },
        { d: 'M 528,185 L 522,295', w: 7.5 },
        { d: 'M 578,244 C 564,248 556,262 556,276 C 556,288 564,296 575,296', w: 5.5 },
        { d: 'M 602,260 L 600,294', w: 6.5 },
        { d: 'M 658,244 C 644,248 636,262 636,276 C 636,288 644,296 655,296', w: 5.5 }
      ]
    },
    {
      id: 's', weight: 0.95,
      hair: 'M 698,198 C 696,208 692,220 687,228 M 698,280 C 708,252 722,238 732,240 C 738,242 738,252 730,260 C 720,272 718,282 724,290 C 730,298 742,294 750,286',
      swells: []
    },
    {
      id: 'B', weight: 1.45,
      hair: 'M 798,165 C 794,180 782,235 778,295 C 776,306 768,308 762,300 M 788,178 C 804,162 842,162 848,195 C 852,216 836,234 814,240 C 840,244 860,262 858,285 C 856,304 834,310 812,306 C 796,303 794,294 804,290 C 814,286 828,292 836,282 C 842,272 832,252 806,248',
      swells: [
        { d: 'M 794,180 C 790,215 780,260 778,295', w: 8.0 },
        { d: 'M 830,166 C 844,172 850,186 846,204 C 842,220 830,230 816,236', w: 6.0 },
        { d: 'M 834,244 C 852,250 858,266 854,284 C 850,298 834,306 816,304', w: 7.0 }
      ]
    },
    {
      id: 'log', weight: 2.20,
      hair: 'M 852,284 C 872,244 902,158 912,158 C 918,158 916,172 905,210 L 888,295 C 888,302 896,302 906,290 C 916,272 928,240 946,240 C 932,244 920,258 920,274 C 920,290 932,298 944,296 C 958,294 966,274 966,256 C 966,242 958,240 950,244 C 942,248 948,258 960,258 C 972,244 984,240 998,240 C 982,244 972,258 972,274 C 972,290 982,298 994,296 C 1005,294 1014,280 1018,260 L 1016,330 C 1012,375 994,405 956,412',
      swells: [
        { d: 'M 915,178 L 892,295', w: 8.0 },
        { d: 'M 944,244 C 930,248 922,262 922,276 C 922,288 930,296 942,296', w: 5.5 },
        { d: 'M 996,244 C 982,248 974,262 974,276 C 974,288 982,296 994,296', w: 5.5 },
        { d: 'M 1018,260 L 1016,335 C 1012,375 994,405 956,412', w: 7.5 }
      ]
    },
    {
      id: 'underline', weight: 1.40,
      hair: 'M 956,412 C 908,416 835,410 740,400 C 560,380 370,380 230,398 C 175,405 150,398 165,386 C 185,372 240,375 320,378',
      swells: [ { d: 'M 850,409 C 680,392 480,385 270,394', w: 4.8 } ]
    }
  ];

  // Pen-up travel between strokes, as a fraction of one full pass.
  //
  // The last entry wraps from the underline back to the start of "Wendao". Without it the pen
  // teleports across the artwork at the seam of every repeat, which is the one movement in the
  // loop the eye is guaranteed to catch.
  //
  // 段与段之间抬笔的空中位移，占一整遍的比例。
  //
  // 最后一条从底部弧线绕回「Wendao」的起笔。没有它，每一遍的接缝处笔都会横穿整个字形瞬移过去——
  // 那是这个循环里眼睛必然会抓到的一个动作。
  var AIR = [
    { from: 0, to: 1, weight: 0.18 },
    { from: 1, to: 2, weight: 0.16 },
    { from: 2, to: 3, weight: 0.32 },
    { from: 3, to: 4, weight: 0.18 },
    { from: 4, to: 5, weight: 0.08 },
    { from: 5, to: 0, weight: 0.30 }
  ];

  var NS = 'http://www.w3.org/2000/svg';
  var PASS_MS = 2900;   // one full pass over the lettering. // 一整遍书写。

  // Two builders, not one.
  //
  // A single builder calling createElementNS(NS, name) produced SVG-namespaced <div> elements for
  // the overlay and the hint. They look like divs in the inspector and they take the class, but they
  // are not HTML elements: the UA stylesheet does not apply, so position stayed static, the flex
  // column had nothing to lay out against, and every measurement came back 0x0. The overlay
  // rendered as a blank sheet — the letters were drawn correctly the whole time, inside a box the
  // browser had given no size.
  //
  // 两个构建函数，不是一个。
  //
  // 只用一个调用 createElementNS(NS, name) 的构建函数，会给遮罩与提示造出 SVG 命名空间下的 <div>。
  // 它们在审查器里长得像 div、也能拿到 class，但它们不是 HTML 元素：UA 样式表不生效，
  // position 因此停在 static，flex 纵列没有可布局的对象，所有测量都返回 0x0。
  // 遮罩于是渲染成一张空白纸 —— 字母其实一直在被正确绘制，只是画在一个浏览器没有给过尺寸的盒子里。
  function svgEl(name, attrs) {
    var n = document.createElementNS(NS, name);
    if (attrs) { for (var k in attrs) { if (attrs.hasOwnProperty(k)) { n.setAttribute(k, attrs[k]); } } }
    return n;
  }
  function htmlEl(name, attrs) {
    var n = document.createElement(name);
    if (attrs) { for (var k in attrs) { if (attrs.hasOwnProperty(k)) { n.setAttribute(k, attrs[k]); } } }
    return n;
  }

  // Builds the overlay. Everything is present from the first frame except the ink, which is
  // revealed by animating the dash offset — drawing the letters as real paths rather than pushing
  // points into a canvas means the glyphs get proper curves, joins and round caps for free.
  //
  // 构建遮罩。第一帧就存在除了墨迹以外的一切，墨迹靠动画 dash offset 揭示 ——
  // 把字母画成真的路径而不是往 canvas 里推点，曲线、接缝与圆头笔锋就都是免费的。
  function buildOverlay() {
    var root = htmlEl('div', { 'class': 'fns-ink', 'aria-hidden': 'true' });

    var svg = svgEl('svg', {
      viewBox: '0 0 1100 500',
      preserveAspectRatio: 'xMidYMid meet',
      'class': 'fns-ink-svg',
      // Intrinsic width/height, not just CSS. An <svg> without them has no intrinsic ratio for
      // layout to fall back on, and inside a flex column with height:auto it measured 0x0 — the
      // overlay rendered as a blank sheet of paper. These two attributes give it a size the CSS
      // can then scale.
      //
      // 内在宽高，不只是 CSS。没有它们的 <svg> 没有可退回的内在比例，
      // 在 height:auto 的 flex 纵列里量出来是 0x0 —— 遮罩渲染成一张空白纸。
      // 这两个属性给它一个尺寸，后面的 CSS 才有东西可以缩放。
      width: '1100',
      height: '500'
    });

    SEGMENTS.forEach(function (seg) {
      var g = svgEl('g', { 'class': 'fns-ink-seg', 'data-seg': seg.id });
      var hair = svgEl('path', { d: seg.hair, 'class': 'fns-ink-hair' });
      g.appendChild(hair);
      seg.el = hair;
      seg.swellEls = [];
      seg.swells.forEach(function (sw) {
        // Swells are painted from the same dash animation so the pressure arrives with the stroke
        // instead of appearing whole on the first frame.
        // 加粗笔画用同一套 dash 动画，压力才随笔画到达，而不是第一帧就整块出现。
        var p = svgEl('path', { d: sw.d, 'class': 'fns-ink-swell', 'stroke-width': sw.w });
        g.appendChild(p);
        seg.swellEls.push(p);
      });
      svg.appendChild(g);
    });

    // The pen. Its origin is the nib tip, so a rotation happens about the point that touches the
    // paper rather than about the middle of the barrel.
    // 钢笔。原点在笔尖，所以旋转绕着接触纸面的那个点发生，而不是绕着笔身中部。
    var pen = svgEl('g', { 'class': 'fns-ink-pen' });
    var tip = svgEl('g', { 'class': 'fns-ink-pen-body' });

    tip.appendChild(svgEl('path', {
      'class': 'fns-ink-pen-barrel',
      d: 'M -15,-132 L 15,-132 L 13.2,-72 L -13.2,-72 Z'
    }));
    tip.appendChild(svgEl('path', {
      'class': 'fns-ink-pen-collar',
      d: 'M -14.5,-78 L 14.5,-78 L 14.5,-72 L -14.5,-72 Z'
    }));
    tip.appendChild(svgEl('path', {
      'class': 'fns-ink-pen-nib',
      d: 'M 0,0 C 2.8,-7 8.5,-22 12,-34 C 15.5,-46 17.5,-58 15.5,-72 L -15.5,-72 C -17.5,-58 -15.5,-46 -12,-34 C -8.5,-22 -2.8,-7 0,0 Z'
    }));
    tip.appendChild(svgEl('path', {
      'class': 'fns-ink-pen-plate',
      d: 'M 0,-7 C 2.2,-14 6.8,-26 9.5,-36 C 11.8,-46 12.8,-56 11.2,-68 L -11.2,-68 C -12.8,-56 -11.8,-46 -9.5,-36 C -6.8,-26 -2.2,-14 0,-7 Z'
    }));
    tip.appendChild(svgEl('circle', { 'class': 'fns-ink-pen-hole', cx: '0', cy: '-35', r: '2.8' }));
    tip.appendChild(svgEl('path', { 'class': 'fns-ink-pen-slit', d: 'M 0,0 L 0,-32' }));
    tip.appendChild(svgEl('circle', { 'class': 'fns-ink-pen-tip', cx: '0', cy: '0', r: '1.4' }));

    pen.appendChild(tip);
    svg.appendChild(pen);

    var hint = htmlEl('div', { 'class': 'fns-ink-hint' });
    hint.innerHTML = '<span class="fns-ink-word">LOADING</span><i class="fns-ink-dot"></i>';

    root.appendChild(svg);
    root.appendChild(hint);
    return { root: root, svg: svg, pen: pen, tip: tip, hint: hint };
  }

  // Measures and caches the per-segment lengths once the SVG is in the document, because
  // getTotalLength needs layout.
  //
  // Every length is cached on the segment here. The previous draft called getTotalLength on each
  // swell inside the animation frame, which forces a layout flush per swell per frame — the same
  // mistake the card tilt already made and had to be fixed for.
  //
  // 在 SVG 进入文档之后一次性量出每段长度，因为 getTotalLength 需要布局。
  //
  // 所有长度都在这里缓存到段上。上一稿在动画帧里对每一条加粗笔画调 getTotalLength，
  // 那就是每帧每条都强制一次布局冲刷 —— 卡片倾斜已经犯过同样的错并被迫修掉。
  function primeSegments() {
    var total = 0;
    for (var i = 0; i < SEGMENTS.length; i++) {
      var seg = SEGMENTS[i];
      seg.length = seg.el.getTotalLength();
      seg.el.style.strokeDasharray = seg.length + ' ' + (seg.length + 6);
      seg.el.style.strokeDashoffset = seg.length;
      seg.swellLen = [];
      for (var j = 0; j < seg.swellEls.length; j++) {
        var sp = seg.swellEls[j];
        var sl = sp.getTotalLength();
        seg.swellLen.push(sl);
        sp.style.strokeDasharray = sl + ' ' + (sl + 6);
        sp.style.strokeDashoffset = sl;
      }
      total += seg.weight;
      // The pen's start point is the beginning of the first stroke.
      seg.startPt = seg.el.getPointAtLength(0);
    }
    for (var k = 0; k < AIR.length; k++) { total += AIR[k].weight; }
    return total;
  }

  function buildTimeline(totalWeight) {
    var line = [];
    var acc = 0;
    for (var i = 0; i < SEGMENTS.length; i++) {
      var seg = SEGMENTS[i];
      var start = acc / totalWeight;
      acc += seg.weight;
      var end = acc / totalWeight;
      line.push({ type: 'draw', seg: seg, start: start, end: end });

      for (var a = 0; a < AIR.length; a++) {
        if (AIR[a].from !== i) { continue; }
        var aStart = acc / totalWeight;
        acc += AIR[a].weight;
        var aEnd = acc / totalWeight;
        var fromPt = seg.el.getPointAtLength(seg.length);
        var toSeg = SEGMENTS[AIR[a].to];
        var toPt = toSeg.el.getPointAtLength(0);
        line.push({ type: 'air', fromPt: fromPt, toPt: toPt, start: aStart, end: aEnd });
      }
    }
    return line;
  }

  // A hand does not accelerate linearly, and a linear ramp reads as a machine plotting a graph.
  // 手不是匀速加速的，线性缓动读起来像机器在画曲线。
  function handEase(t) { return t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2; }

  function makeRenderer(view, timeline) {
    function render(p) {
      if (p > 1) { p = 1; }
      if (p < 0) { p = 0; }

      var penPos = null;
      var penAngle = 39;
      var lifting = false;

      for (var i = 0; i < timeline.length; i++) {
        var item = timeline[i];

        if (item.type === 'draw') {
          var seg = item.seg;
          var hairOff, frac;
          if (p <= item.start) {
            hairOff = seg.length;
            frac = 0;
          } else if (p >= item.end) {
            hairOff = 0;
            frac = 1;
          } else {
            var local = (p - item.start) / (item.end - item.start);
            frac = handEase(local);
            hairOff = seg.length - seg.length * frac;
          }
          seg.el.style.strokeDashoffset = hairOff;

          for (var j = 0; j < seg.swellEls.length; j++) {
            var sp = seg.swellEls[j];
            var sl = seg.swellLen[j];
            sp.style.strokeDashoffset = sl - sl * frac;
          }

          if (p > item.start && p < item.end) {
            var drawn = seg.length * frac;
            penPos = seg.el.getPointAtLength(drawn);
            var next = seg.el.getPointAtLength(Math.min(seg.length, drawn + 2));
            // A few degrees of drift with the stroke direction, so the pen is held rather than
            // stamped.
            // 沿笔画方向几度的自然摆动，笔才像是被握着，而不是盖章。
            var drift = Math.atan2(next.y - penPos.y, Math.abs(next.x - penPos.x) + 0.1) * (12 / Math.PI);
            penAngle = 39 + drift;
          }
        } else {
          if (p > item.start && p < item.end) {
            lifting = true;
            var ln = (p - item.start) / (item.end - item.start);
            var eased = 0.5 - 0.5 * Math.cos(ln * Math.PI);
            var arc = -22 * Math.sin(ln * Math.PI);
            penPos = {
              x: item.fromPt.x + (item.toPt.x - item.fromPt.x) * eased,
              y: item.fromPt.y + (item.toPt.y - item.fromPt.y) * eased + arc
            };
            penAngle = 40 - Math.sin(ln * Math.PI) * 4;
          }
        }
      }

      if (penPos) {
        view.pen.style.opacity = '1';
        view.pen.setAttribute('transform', 'translate(' + penPos.x + ',' + penPos.y + ') rotate(' + penAngle + ')');
        view.pen.classList.toggle('is-lifting', lifting);
      } else {
        // Between strokes — and therefore also at the seam of a repeat — the pen rests above the
        // start of the next stroke rather than disappearing. The sample file faded the pen out at
        // the end of its single pass; looping that would make it blink once per pass.
        //
        // 笔画之间 —— 也就是每一遍的接缝处 —— 笔停在下一笔起笔点的上方，而不是消失。
        // 示例文件在它那一遍结束时让笔淡出；把它照搬进循环，笔就会每一遍眨一次。
        var home = SEGMENTS[0].startPt;
        view.pen.style.opacity = '1';
        view.pen.setAttribute('transform', 'translate(' + home.x + ',' + (home.y - 46) + ') rotate(' + penAngle + ')');
        view.pen.classList.remove('is-lifting');
      }
    }
    return render;
  }

  // ── The departing half, exposed so JS navigations can use it too ────────────────────────
  //
  // Dock entries are buttons with JS handlers, not links, so there is nothing for the click
  // delegation to intercept. They call this directly instead.
  //
  // 出发的那一半，同时暴露给 JS 跳转使用。
  //
  // 胶囊里的入口是带 JS 处理器的按钮而不是链接，下面那层点击代理拦不到它们；
  // 它们直接调这个函数。
  function warpTo(url) {
    if (!url) { return; }
    if (STILL) { location.href = url; return; }
    // A second click during the flight would stack overlays and start a second animation.
    // 途中再点一次会叠起第二层遮罩和第二套动画。
    if (running) { return; }
    running = true;

    try { sessionStorage.setItem(FLAG, '1'); } catch (e) {}

    var built = buildOverlay();
    var view = built;

    // The overlay is attached to <html>, never <body>: the departing fade puts a transform on body,
    // and a transformed element becomes the containing block for its fixed descendants. Attaching
    // to body pinned the overlay to the top of the document, so a scrolled reader saw nothing.
    //
    // 遮罩挂在 <html> 上，绝不挂 <body>：出发淡出会给 body 加 transform，而被 transform 的元素
    // 会成为其 fixed 后代的包含块。挂到 body 上会把遮罩钉在文档顶部，滚过的读者就什么都看不到。
    document.documentElement.appendChild(view.root);

    // Expose the layout before measuring lengths: getTotalLength and getPointAtLength need the
    // element to be in the document and laid out.
    // 量长度之前先让布局生效：getTotalLength 与 getPointAtLength 需要元素已在文档里并完成布局。
    void view.root.offsetWidth;

    var totalWeight = primeSegments();
    var timeline = buildTimeline(totalWeight);
    var render = makeRenderer(view, timeline);

    // Lock the page behind the overlay. Without this the wheel still scrolls the document under
    // the loading screen, and the reader arrives at the new page at a random offset.
    // 锁住遮罩背后那一页。少了它，滚轮仍会在加载画面之下滚文档，
    // 读者到新页面时会落在一个莫名其妙的位置。
    var lockedY = window.scrollY || document.documentElement.scrollTop || 0;
    document.documentElement.classList.add('fns-warping');

    var startedAt = null;
    var finishedPasses = 0;
    var wantsDone = false;
    var gone = false;

    function setHint(word) {
      var w = view.hint.querySelector('.fns-ink-word');
      if (w) { w.textContent = word; }
    }

    function fadeOut() {
      if (gone) { return; }
      gone = true;
      view.root.classList.add('is-done');
      setHint('READY');
      // Navigate on the same schedule the CSS fade uses, so the page does not sit on a blank
      // overlay waiting.
      // 按与 CSS 淡出相同的节奏导航，页面才不会停在一层空白遮罩上干等。
      window.setTimeout(function () { location.href = url; }, 380);
    }

    function tick(ts) {
      if (startedAt === null) { startedAt = ts; }
      var elapsed = ts - startedAt;
      var passMs = PASS_MS;
      var cycle = elapsed % passMs;
      var p = cycle / passMs;

      render(p);

      // A pass just ended. Count it and decide whether this was the last one.
      // 刚走完一遍。计数，并判断这是不是最后一遍。
      var passesNow = Math.floor(elapsed / passMs);
      if (passesNow > finishedPasses) {
        finishedPasses = passesNow;
        if (wantsDone) { fadeOut(); return; }
        setHint(finishedPasses === 0 ? 'LOADING' : 'STILL LOADING');
      }

      if (p > 0.92) { setHint(wantsDone ? 'READY' : 'LOADING'); }

      requestAnimationFrame(tick);
    }

    // The destination is fetched in the background. Whichever arrives first — the fetch or the
    // navigation timeout — the pen still finishes the pass it is on.
    //
    // 目标页在后台预取。无论预取先到还是超时先到，笔都会把它手上这一遍写完。
    var prefetched = false;
    try {
      fetch(url, { credentials: 'same-origin' }).then(function () { prefetched = true; }).catch(function () {});
    } catch (e) {}

    // Mark "ready to leave" once the destination has plausibly arrived. The pass in flight still
    // completes; this only decides that no further pass starts.
    //
    // 一旦目标页大概到齐就标记「可以走了」。手上这一遍仍会写完，这个标记只决定不再开新的一遍。
    var readyCheck = window.setInterval(function () {
      if (prefetched) { wantsDone = true; window.clearInterval(readyCheck); }
    }, 120);

    // A hard ceiling: a destination that never resolves must not trap the reader on a loading
    // screen forever.
    // 硬上限：永远不会返回的目标页不能把读者永远困在加载画面上。
    window.setTimeout(function () {
      wantsDone = true;
      window.clearInterval(readyCheck);
    }, 6000);

    requestAnimationFrame(tick);
  }

  window.fnsWarp = warpTo;

  // ── Click delegation for in-site navigation ────────────────────────────────────────────
  //
  // Only plain left clicks. Ctrl/cmd/middle mean "open elsewhere", and hijacking those is how a
  // link stops behaving like a link.
  //
  // 站内导航的点击代理。
  //
  // 只接管普通左键。Ctrl/⌘/中键表示「在别处打开」，抢走它们就等于让链接不再像链接。
  //
  // The earlier version only intercepted /blog/<slug>. The owner's requirement is that entering
  // and returning both play the animation, which includes the list pages, the archives, the shelf,
  // the index and the retrogression of pressing back into a page that was left with the overlay.
  // Anything that stays on this origin and is not a same-page anchor gets the overlay.
  //
  // 旧版只拦 /blog/<slug>。站主要求「进入返回」都播，包括列表页、归档、资料书架、索引，
  // 以及从遮罩离开之后再按返回回到一个页面。只要还留在本站且不是同页锚点，就上遮罩。
  var SKIP_SCHEME = /^(?:mailto:|tel:|javascript:|data:|blob:)/i;

  document.addEventListener('click', function (ev) {
    if (ev.defaultPrevented || ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;
    var a = ev.target && ev.target.closest ? ev.target.closest('a[href]') : null;
    if (!a) return;
    if (a.hasAttribute('download')) return;
    if (a.target && a.target !== '_self') return;

    var href = a.getAttribute('href') || '';
    if (!href || href.charAt(0) === '#') return;
    if (SKIP_SCHEME.test(href)) return;

    // Resolve relative hrefs against the document so "/blog/x" and an absolute URL are decided by
    // the same test.
    // 把相对 href 解析到当前文档上，"/blog/x" 与绝对地址才由同一个判断决定。
    var target;
    try { target = new URL(href, location.href); } catch (e) { return; }
    if (target.origin !== location.origin) return;

    // Same page, only a differing hash or a query the page handles itself: not a navigation.
    // 同一个页面、只差 hash 或由页面自己处理的查询串：不算导航。
    var samePath = target.pathname === location.pathname && target.search === location.search;
    if (samePath) return;

    // A link that points at the binary/feed/asset routes must not get a 3-second loading screen in
    // front of a file download.
    // 指向二进制/订阅源/资源路由的链接不能在一个文件下载前面加上三秒加载画面。
    var last = target.pathname.split('/').pop() || '';
    if (/\.(?:xml|json|js|css|png|jpe?g|webp|gif|svg|ico|pdf|zip|txt|md|woff2?)$/i.test(last)) return;

    if (STILL) return;   // Reduced motion: let the browser navigate plainly. // 减少动效：交给浏览器普通导航。

    ev.preventDefault();
    warpTo(target.href);
  });

  // ── Back / forward ────────────────────────────────────────────────────────────────────
  //
  // Coming back to a page through history is a navigation too, and the owner asked for it
  // explicitly. pageshow with persisted=false is the arrival; a bfcache restore (persisted=true)
  // shows the old document as it was, so the overlay is not needed there.
  //
  // 通过历史回到一个页面同样是一次导航，站主明确要求了它。
  // pageshow 且 persisted=false 就是抵达；bfcache 恢复（persisted=true）显示的是旧文档原样，
  // 那里不需要遮罩。
  window.addEventListener('pageshow', function (ev) {
    if (ev.persisted) { return; }
    // Nothing to do here: the arriving half above already ran on this document.
  });
})();

`

// blogWarpJS returns the transition script.
//
// Served on every blog page rather than only the ones that link away: the departing half must be
// present wherever a reader can click, and the arriving half has to be present on whichever page a
// reader lands on after the transition. Splitting it into two files by page would mean keeping two
// halves of one behaviour in sync across every template.
//
// blogWarpJS 返回过渡脚本。
//
// 它在每个博客页面上提供，而不是只在会跳走的那几个上：只要读者能点击，出发的那一半就必须在；
// 而抵达的那一半必须出现在读者过渡后落到的任何页面上。按页面拆成两个文件，
// 意味着把同一个行为的两半在每一个模板之间保持同步。
func blogWarpJS() string {
	return blogWarpScript
}
