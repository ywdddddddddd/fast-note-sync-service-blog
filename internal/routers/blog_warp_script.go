package routers

// The hand-written loader: a fountain pen draws "Wendao's Blog" over a paper-toned overlay while
// the page behind it loads, and the overlay lifts only once the page can actually be looked at.
// ── Why a cross-document transition and not a client-side swap ───────────────────────────
//
// Intercepting the click, fetching the destination, swapping the DOM would make the transition
// seamless with no help from the other page. It would also mean the browser's back button,
// history, the address bar and a fresh reader's direct visit take a different code path than the
// one this animation exercises — and the path nobody exercises is the one that breaks.
//
// 为什么是跨文档过渡，而不是客户端换页
//
// 拦下点击、取回目标页、替换 DOM，可以让过渡不依赖目标页就做到无缝。
// 但那也意味着浏览器的后退键、历史、地址栏，以及新读者直接访问，
// 走的都是与这段动画不同的另一条代码路径 —— 而没人走的路径正是会坏的那条。
//
// ── The overlay is created in <head>, on every page, every time ─────────────────────────
//
// 2026-11-06, second round. The first version only painted the overlay when a sessionStorage flag
// said "you arrived from a transition". That produced three concrete failures the owner filmed:
//
//  1. A first visit played nothing at all: no flag, no overlay, so the reader watched a white
//     page and then the site appear.
//  2. Pressing back restored the previous document from bfcache with html.fns-warping still on it
//     and body at opacity 0 — a completely white page, forever, because nothing ever removed the
//     class.
//  3. The overlay lifted after a fixed number of passes, not after the page was usable. One
//     section page carries a dozen covers and each takes ~700ms to arrive, so the reader was left
//     watching placeholders spin after the pen had finished.
//
// The inverse design fixes all three at once: the overlay is painted by the head script on every
// blog page before the first frame, it stays until the page is genuinely ready, and it is torn
// down by the same code that created it.
//
// 每一页、每一次，遮罩都在 head 里创建
//
// 2026-11-06 第二轮。第一版只在 sessionStorage 标记说「你是过渡过来的」时才画遮罩。
// 站主录屏里三个具体失败都是它造成的：
//
//  1. 首次访问什么都不播：没有标记就没有遮罩，读者先看到一张白页，然后站点出现。
//  2. 按返回时 bfcache 把旧文档原样恢复，带着 html.fns-warping 和 body 的 opacity:0 ——
//     整页全白且永远不会恢复，因为没有任何代码去摘掉那个类。
//  3. 遮罩在固定遍数之后抬起，而不是在页面真的可用之后。一个栏目页有十几张封面，
//     每张约 700ms 才到，于是笔写完了，读者还在盯着转圈的占位图。
//
// 反过来设计的这套一次性修好三件：遮罩由 head 脚本在首帧之前画上，一直留到页面确实就绪，
// 并且由创建它的同一份代码拆掉。
//
// ── Phase handoff ───────────────────────────────────────────────────────────────────────
//
// The pen must not restart from "W" on every navigation, or the two documents read as two
// animations. The departure writes the animation's start time into sessionStorage; the destination
// reads it and continues from the same phase, so the line is one continuous performance across the
// document boundary.
//
// 相位交接
//
// 笔不能每次导航都从「W」重来，否则两个文档读起来就是两段动画。
// 出发页把动画起始时刻写进 sessionStorage，目标页读它并从同一相位继续，
// 于是这条线跨过文档边界仍然是一次连续的书写。
//
// ── The overlay must be viewport-sized from any scroll position ──────────────────────────
//
// The overlay is a child of <html>, never <body>: the departure fade once put a transform on body,
// and a transformed element becomes the containing block for its fixed descendants. Measured at
// scrollY=1176 the canvas sat at y=-1176, entirely off-screen.
//
// 遮罩必须在任何滚动位置都完整
//
// 遮罩是 <html> 的子节点，绝不是 <body> 的：出发淡出曾在 body 上放 transform，
// 而被 transform 的元素会成为其 fixed 后代的包含块。实测 scrollY=1176 时画布在 y=-1176，
// 整个在屏幕外。
const blogWarpScript = `
(function () {
  // ── Guard: blog pages only ──────────────────────────────────────────────────────────────
  // The same head script is shared by the error page, which has no shell and no reading to wait
  // for. A loading screen in front of a 404 would be pure delay.
  // 同一个 head 脚本也被错误页共用，而错误页没有外壳、也没有内容要等。
  // 在 404 前面放加载画面是纯粹的拖延。
  if (location.pathname.indexOf('/blog') !== 0) { return; }

  var STILL = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);

  var KEY_PHASE = 'fns:ink-phase';
  var KEY_DEPART = 'fns:ink-depart';

  // ── Phase ─────────────────────────────────────────────────────────────────────────────
  //
  // One clock for the whole site: performance.timeOrigin is when this document was created, and
  // adding the phase offset stored by the departure page reconstructs "how long has the pen been
  // writing" across the navigation.
  //
  // 全站共用一只钟：performance.timeOrigin 是这个文档创建的时刻，
  // 加上出发页存下的相位偏移，就能在跨文档之后重建「笔已经写了多久」。
  var phaseOffset = 0;
  try {
    var savedPhase = sessionStorage.getItem(KEY_PHASE);
    if (savedPhase) { phaseOffset = parseFloat(savedPhase) || 0; }
  } catch (e) {}

  function elapsedNow() { return phaseOffset + (performance.now ? performance.now() : Date.now() - timeOriginFallback()); }
  var __t0 = Date.now();
  function timeOriginFallback() { return __t0; }

  // ── Readiness ─────────────────────────────────────────────────────────────────────────
  //
  // "Ready" means: the document finished parsing, and every image already in the document has
  // either loaded or failed. Covers are the slow part — one section page carries a dozen at ~700ms
  // each — so waiting on the document alone is what left the reader watching spinners.
  //
  // Images that fail must not hold the overlay forever: a broken cover is the page's problem and
  // the reader still wants the article. They count as settled once the browser gives up on them.
  //
  // 就绪
  //
  // 「就绪」指：文档解析完成，且文档里每一张图要么已加载、要么已失败。
  // 封面是慢的那一环 —— 一个栏目页有十几张、每张约 700ms ——
  // 所以只等文档，正是让读者盯着转圈图标的原因。
  //
  // 失败的图不能永远扣着遮罩：坏掉的封面是页面的问题，读者仍然要读正文。
  // 浏览器放弃它们之后就算落定。
  function imagesSettled() {
    var imgs = document.images;
    for (var i = 0; i < imgs.length; i++) {
      var img = imgs[i];
      if (img.complete) { continue; }
      // A lazy image below the fold has not been asked for yet, and waiting for it would hold the
      // overlay until the reader scrolls. Lazy work belongs to scrolling, not to first paint.
      // 视口之外的懒加载图还没被请求，等它会把遮罩扣到读者滚动为止。
      // 懒加载属于滚动，不属于首屏。
      if (img.loading === 'lazy') { continue; }
      return false;
    }
    return true;
  }

  function documentSettled() {
    if (document.readyState !== 'complete') { return false; }
    return imagesSettled();
  }

  // ── The artwork ───────────────────────────────────────────────────────────────────────
  //
  // From the sample file the owner supplied. Eight pen-down strokes rather than the sample's six:
  // the sample folded "s" and "B" into one path each with two subpaths, and getPointAtLength walks
  // a multi-subpath path by jumping between them. Measured at the seam of "B": the pen travelled 127
  // units in a single step, so it floated away from the letter being drawn. One subpath per stroke
  // keeps the pen on the ink.
  //
  // 字形
  //
  // 取自站主给的示例文件。八次落笔而不是示例的六次：示例把「s」和「B」各自折成了一条含两个子路径的
  // path，而 getPointAtLength 走多子路径时会在子路径之间跳。实测「B」的接缝处笔一步走了 127 单位，
  // 于是它飘离了正在画的字母。每笔一条子路径，笔才留在墨上。
  var SEGMENTS = [
    {
      weight: 1.05,
      hair: 'M 130,195 C 115,200 110,230 115,258 C 122,298 140,318 160,318 C 178,318 194,290 205,255 C 218,290 234,318 252,318 C 274,318 290,270 298,205 C 302,185 292,180 282,184 C 274,188 274,204 282,224 C 290,248 304,268 318,268',
      swells: [
        { d: 'M 115,225 C 117,250 128,285 145,310', w: 7.5 },
        { d: 'M 205,255 C 218,290 232,315 245,316', w: 6.8 }
      ]
    },
    {
      weight: 2.10,
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
      weight: 0.30,
      hair: 'M 698,198 C 696,208 692,220 687,228',
      swells: []
    },
    {
      weight: 0.75,
      hair: 'M 698,280 C 708,252 722,238 732,240 C 738,242 738,252 730,260 C 720,272 718,282 724,290 C 730,298 742,294 750,286',
      swells: []
    },
    {
      weight: 0.55,
      hair: 'M 798,165 C 794,180 782,235 778,295 C 776,306 768,308 762,300',
      swells: [ { d: 'M 794,180 C 790,215 780,260 778,295', w: 8.0 } ]
    },
    {
      weight: 0.95,
      hair: 'M 788,178 C 804,162 842,162 848,195 C 852,216 836,234 814,240 C 840,244 860,262 858,285 C 856,304 834,310 812,306 C 796,303 794,294 804,290 C 814,286 828,292 836,282 C 842,272 832,252 806,248',
      swells: [
        { d: 'M 830,166 C 844,172 850,186 846,204 C 842,220 830,230 816,236', w: 6.0 },
        { d: 'M 834,244 C 852,250 858,266 854,284 C 850,298 834,306 816,304', w: 7.0 }
      ]
    },
    {
      weight: 1.70,
      hair: 'M 852,284 C 872,244 902,158 912,158 C 918,158 916,172 905,210 L 888,295 C 888,302 896,302 906,290 C 916,272 928,240 946,240 C 932,244 920,258 920,274 C 920,290 932,298 944,296 C 958,294 966,274 966,256 C 966,242 958,240 950,244 C 942,248 948,258 960,258 C 972,244 984,240 998,240 C 982,244 972,258 972,274 C 972,290 982,298 994,296 C 1005,294 1014,280 1018,260 L 1016,330 C 1012,375 994,405 956,412',
      swells: [
        { d: 'M 915,178 L 892,295', w: 8.0 },
        { d: 'M 944,244 C 930,248 922,262 922,276 C 922,288 930,296 942,296', w: 5.5 },
        { d: 'M 996,244 C 982,248 974,262 974,276 C 974,288 982,296 994,296', w: 5.5 },
        { d: 'M 1018,260 L 1016,335 C 1012,375 994,405 956,412', w: 7.5 }
      ]
    },
    {
      weight: 1.15,
      hair: 'M 956,412 C 908,416 835,410 740,400 C 560,380 370,380 230,398 C 175,405 150,398 165,386 C 185,372 240,375 320,378',
      swells: [ { d: 'M 850,409 C 680,392 480,385 270,394', w: 4.8 } ]
    }
  ];

  // Pen-up travel. The last entry wraps the underline back to the start of "Wendao", so the seam of
  // a repeat is a movement rather than a teleport.
  // 抬笔位移。最后一条把底部弧线接回「Wendao」的起笔，于是重复的接缝是一次移动而不是瞬移。
  var AIR = [
    { from: 0, to: 1, weight: 0.14 },
    { from: 1, to: 2, weight: 0.12 },
    { from: 2, to: 3, weight: 0.08 },
    { from: 3, to: 4, weight: 0.26 },
    { from: 4, to: 5, weight: 0.10 },
    { from: 5, to: 6, weight: 0.14 },
    { from: 6, to: 7, weight: 0.06 },
    { from: 7, to: 0, weight: 0.30 }
  ];

  var NS = 'http://www.w3.org/2000/svg';
  var PASS_MS = 2900;

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

  // ── Build ─────────────────────────────────────────────────────────────────────────────
  //
  // The two critical style declarations are written inline, not left to the stylesheet.
  //
  // This script runs in <head>, which by definition is before the <style> block that follows it.
  // Creating a bare div there means the first frames show an unstyled block: no fixed positioning,
  // no background, and an <svg> at its intrinsic 1100x500 painting over the page. Measured on a
  // 390x844 viewport the overlay reported 390x529 with a 1100x500 artwork — the sheet was sized by
  // its own content because none of the rules had been parsed yet.
  //
  // Inlining position/inset/z-index/background/overflow costs a few bytes and makes the overlay
  // correct from its very first painted frame, whether or not the stylesheet has arrived. Every
  // other declaration (flex centring, colours, transitions, the lettering) stays in the sheet.
  //
  // 两个关键样式内联写，而不是留给样式表。
  //
  // 这个脚本跑在 <head> 里，按定义就在它后面的 <style> 之前。在那里造一个裸 div，
  // 最初几帧呈现的就是一个没有样式的块：没有 fixed、没有底色，
  // 而 <svg> 以自己 1100x500 的内在尺寸画在页面上。
  // 在 390x844 的视口上实测，遮罩报 390x529、画面 1100x500 —— 纸面被自己的内容撑出了尺寸，
  // 因为当时一条规则都还没被解析。
  //
  // 内联 position/inset/z-index/背景/overflow 只多几个字节，
  // 却让遮罩从它被画出来的第一帧起就是对的，无论样式表到没到。
  // 其余声明（flex 居中、配色、过渡、字形）仍留在样式表里。
  var SHELL_STYLE = 'position:fixed;top:0;right:0;bottom:0;left:0;' +
    'z-index:2147483000;pointer-events:none;overflow:hidden;' +
    'background:#fff;';

  function buildOverlay() {
    var root = htmlEl('div', { 'class': 'fns-ink', 'aria-hidden': 'true' });

    // The paper colour has to match the theme from the first frame too, and the theme is decided by
    // the theme script that runs just before this one — so read it here rather than waiting for the
    // stylesheet's var(--background).
    // 纸色也必须从第一帧就与主题一致，而主题由恰好跑在前面的主题脚本决定 ——
    // 所以在这里读它，而不是等样式表的 var(--background)。
    var earlyBg = '#fff';
    if (document.documentElement.classList.contains('dark')) { earlyBg = '#1a1a1a'; }
    else if (!document.documentElement.classList.contains('light')) {
      // The system state adds no class; the media query decides, and it is a preference the script
      // can read directly.
      // 跟随系统时两个类都不加，由媒体查询决定；而这是一个脚本能直接读到的偏好。
      try {
        if (window.matchMedia && window.matchMedia('(prefers-color-scheme:dark)').matches) { earlyBg = '#1a1a1a'; }
      } catch (e) {}
    }
    root.setAttribute('style', SHELL_STYLE + 'background:' + earlyBg + ';');

    var svg = svgEl('svg', {
      viewBox: '0 0 1100 500',
      preserveAspectRatio: 'xMidYMid meet',
      'class': 'fns-ink-svg',
      // Intrinsic width/height, not only CSS: an <svg> without them has no intrinsic ratio and
      // measured 0x0 inside a flex column with height:auto.
      // 内在宽高，不只是 CSS：没有它们的 <svg> 没有内在比例，在 height:auto 的 flex 纵列里量到 0x0。
      width: '1100',
      height: '500',
      // Bounded before the stylesheet lands, so the artwork cannot paint at 1100px wide over a
      // 390px phone screen for the first few frames.
      // 在样式表到达之前就限住，字形不会在最初几帧里以 1100px 宽画在 390px 的手机屏上。
      style: 'display:block;width:min(86vw,760px);height:auto;max-height:64vh;'
    });

    SEGMENTS.forEach(function (seg) {
      var g = svgEl('g', { 'class': 'fns-ink-seg' });
      var hair = svgEl('path', { d: seg.hair, 'class': 'fns-ink-hair' });
      g.appendChild(hair);
      seg.el = hair;
      seg.swellEls = [];
      seg.swells.forEach(function (sw) {
        var p = svgEl('path', { d: sw.d, 'class': 'fns-ink-swell', 'stroke-width': sw.w });
        g.appendChild(p);
        seg.swellEls.push(p);
      });
      svg.appendChild(g);
    });

    // The pen. Its origin is the nib tip, so rotation happens about the point that touches paper.
    // 钢笔。原点在笔尖，于是旋转绕着接触纸面的那个点发生。
    var pen = svgEl('g', { 'class': 'fns-ink-pen' });
    var tip = svgEl('g', { 'class': 'fns-ink-pen-body' });

    tip.appendChild(svgEl('path', {
      'class': 'fns-ink-pen-barrel',
      d: 'M -15,-132 L 15,-132 L 13.2,-72 L -13.2,-72 Z'
    }));
    tip.appendChild(svgEl('path', {
      'class': 'fns-ink-pen-collar',
      d: 'M -14.6,-78 L 14.6,-78 L 14.6,-72 L -14.6,-72 Z'
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

  var view = null;
  var timeline = null;
  var render = null;
  var primed = false;

  // Lengths need layout, so this runs once the overlay is in the document and laid out.
  // 长度需要布局，所以在遮罩进入文档并完成布局之后跑一次。
  function prime() {
    if (primed || !view) { return; }
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
      seg.startPt = seg.el.getPointAtLength(0);
    }
    for (var k = 0; k < AIR.length; k++) { total += AIR[k].weight; }

    timeline = [];
    var acc = 0;
    for (var m = 0; m < SEGMENTS.length; m++) {
      var s2 = SEGMENTS[m];
      var start = acc / total;
      acc += s2.weight;
      var end = acc / total;
      timeline.push({ type: 'draw', seg: s2, start: start, end: end });
      for (var a = 0; a < AIR.length; a++) {
        if (AIR[a].from !== m) { continue; }
        var aStart = acc / total;
        acc += AIR[a].weight;
        var aEnd = acc / total;
        timeline.push({
          type: 'air',
          fromPt: s2.el.getPointAtLength(s2.length),
          toPt: SEGMENTS[AIR[a].to].el.getPointAtLength(0),
          start: aStart, end: aEnd
        });
      }
    }
    render = makeRenderer();
    primed = true;
  }

  function handEase(t) { return t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2; }

  function makeRenderer() {
    function paint(p) {
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
          if (p <= item.start) { hairOff = seg.length; frac = 0; }
          else if (p >= item.end) { hairOff = 0; frac = 1; }
          else {
            var local = (p - item.start) / (item.end - item.start);
            frac = handEase(local);
            hairOff = seg.length - seg.length * frac;
          }
          seg.el.style.strokeDashoffset = hairOff;
          for (var j = 0; j < seg.swellEls.length; j++) {
            var sl = seg.swellLen[j];
            seg.swellEls[j].style.strokeDashoffset = sl - sl * frac;
          }
          if (p > item.start && p < item.end) {
            var drawn = seg.length * frac;
            penPos = seg.el.getPointAtLength(drawn);
            var next = seg.el.getPointAtLength(Math.min(seg.length, drawn + 2));
            var drift = Math.atan2(next.y - penPos.y, Math.abs(next.x - penPos.x) + 0.1) * (12 / Math.PI);
            penAngle = 39 + drift;
          }
        } else {
          if (p > item.start && p < item.end) {
            lifting = true;
            var ln = (p - item.start) / (item.end - item.start);
            var eased = 0.5 - 0.5 * Math.cos(ln * Math.PI);
            penPos = {
              x: item.fromPt.x + (item.toPt.x - item.fromPt.x) * eased,
              y: item.fromPt.y + (item.toPt.y - item.fromPt.y) * eased - 22 * Math.sin(ln * Math.PI)
            };
            penAngle = 40 - Math.sin(ln * Math.PI) * 4;
          }
        }
      }

      if (penPos) {
        view.pen.style.opacity = '1';
        view.pen.setAttribute('transform', 'translate(' + penPos.x + ',' + penPos.y + ') rotate(' + penAngle + ')');
        if (lifting) { view.pen.classList.add('is-lifting'); }
        else { view.pen.classList.remove('is-lifting'); }
      } else {
        // Between strokes — and so also at the seam of a repeat — the pen waits above the next
        // stroke rather than disappearing, which is what the sample did at the end of its one pass.
        // 笔画之间 —— 也就是每一遍的接缝处 —— 笔停在下一笔上方，而不是消失；示例在它那一遍结尾是消失的。
        var home = SEGMENTS[0].startPt;
        view.pen.style.opacity = '1';
        view.pen.setAttribute('transform', 'translate(' + home.x + ',' + (home.y - 46) + ') rotate(39)');
        view.pen.classList.remove('is-lifting');
      }
    }
    return paint;
  }

  // ── The one overlay for this document ─────────────────────────────────────────────────
  //
  // Created immediately, before the first paint, and removed by finish(). Everything else in this
  // file is either feeding it a phase or asking whether the page is ready to be looked at.
  //
  // 本文档唯一的那层遮罩
  //
  // 首帧之前就创建，由 finish() 拆掉。这个文件里其余的东西，
  // 要么是给它喂相位，要么在问页面是不是已经可以被看了。
  var finished = false;
  var raf = 0;

  function setHint(word) {
    var w = view.hint.querySelector('.fns-ink-word');
    if (w && w.textContent !== word) { w.textContent = word; }
  }

  function finish(immediate) {
    if (finished) { return; }
    finished = true;
    if (raf) { cancelAnimationFrame(raf); raf = 0; }

    // The navigation lock is released here and nowhere else. Releasing it in two places is how the
    // back button ended up restoring a page with the lock still on.
    // 导航锁只在这里释放，别处都不释放。在别处释放，正是返回键恢复出一个还带着锁的页面的原因。
    document.documentElement.classList.remove('fns-warping');
    document.documentElement.classList.add('fns-arrived');

    if (view && view.root) {
      view.root.classList.add('is-done');
      var node = view.root;
      window.setTimeout(function () {
        if (node && node.parentNode) { node.parentNode.removeChild(node); }
      }, 420);
    }
    view = null;

    window.setTimeout(function () {
      document.documentElement.classList.remove('fns-arrived');
    }, 700);

    try { sessionStorage.removeItem(KEY_DEPART); } catch (e) {}
  }

  function tick() {
    if (finished) { return; }
    if (!primed && view) { prime(); }
    if (render) {
      var el = elapsedNow();
      render((el % PASS_MS) / PASS_MS);

      // Only once the pen has completed at least one pass AND the page is settled does the overlay
      // lift. A pass boundary is the only place the pen is off the paper, so lifting there means
      // the final frame is the finished signature rather than a half-drawn letter.
      //
      // 只有当笔至少写完一遍、并且页面已经就绪时，遮罩才抬起。
      // 一遍的边界是笔唯一离开纸面的时刻，在那里抬起，最后一帧就是写好的签名，而不是半个字母。
      var passDone = el >= PASS_MS;
      if (passDone && documentSettled()) {
        setHint('READY');
        finish(false);
        return;
      }
      if (documentSettled()) { setHint('READY'); }
      else if (passDone) { setHint('LOADING'); }
    }
    raf = requestAnimationFrame(tick);
  }

  function start() {
    if (finished || view) { return; }

    // Reduced motion: no overlay at all. A reader who asked the system to stop animating should not
    // be given a 3-second screen in front of every click either.
    // 减少动效：完全不建遮罩。要求系统停止动画的读者，也不该每次点击都先看三秒画面。
    if (STILL) { return; }

    view = buildOverlay();

    // The overlay is a child of <html>, never <body>: a transform on body would capture it.
    // 遮罩是 <html> 的子节点，绝不是 <body> 的：body 上的 transform 会捕获它。
    document.documentElement.appendChild(view.root);
    document.documentElement.classList.add('fns-warping');

    // Draw the first frame synchronously so there is never a frame of unstyled or empty overlay
    // between the tag being parsed and the animation starting.
    // 同步画出第一帧，于是从标签被解析到动画开始之间，不存在一帧空白或没有样式的遮罩。
    void view.root.offsetWidth;
    prime();
    if (render) { render((elapsedNow() % PASS_MS) / PASS_MS); }

    raf = requestAnimationFrame(tick);
  }

  // ── Departure ─────────────────────────────────────────────────────────────────────────
  //
  // Navigation is not intercepted. The overlay is already up (every page has one), so a departure
  // only has to: remember the phase, warm the destination, and get out of the way.
  //
  // 出发
  //
  // 不拦导航。遮罩本来就在（每一页都有），所以一次出发只需要：记住相位、预热目标页、然后让开。
  function warmDestination(url) {
    // Prefetch the document and every cover it names. The covers are the slow part — a section page
    // carries a dozen at ~700ms each — and without this the destination shows its own overlay while
    // they arrive, which is the "animation finished but it is still loading" the owner filmed.
    //
    // 预取目标文档以及它引用的每一张封面。封面是慢的那一环 —— 一个栏目页十几张、每张约 700ms ——
    // 少了这一步，目标页会带着自己的遮罩等它们到齐，
    // 也就是站主录到的「动画播完了还在加载」。
    try {
      fetch(url, { credentials: 'same-origin' })
        .then(function (r) { return r.text(); })
        .then(function (html) {
          var re = /<img[^>]+src="([^"]+)"/g;
          var m, seen = {}, list = [];
          while ((m = re.exec(html)) !== null) {
            var src = m[1].replace(/&amp;/g, '&');
            if (seen[src]) { continue; }
            seen[src] = 1;
            list.push(src);
          }
          // Cap the warm-up: a page with hundreds of images should not spawn hundreds of requests
          // that compete with the document the reader is about to need.
          // 给预热设上限：一整页几百张图时，不该放几百个请求去和读者马上要用的文档抢带宽。
          list.slice(0, 14).forEach(function (s) {
            try { var im = new Image(); im.decoding = 'async'; im.src = s; } catch (e) {}
          });
        })
        .catch(function () {});
    } catch (e) {}
  }

  function depart(url) {
    if (!url) { return false; }
    try { sessionStorage.setItem(KEY_PHASE, String(elapsedNow())); } catch (e) {}
    try { sessionStorage.setItem(KEY_DEPART, '1'); } catch (e) {}
    warmDestination(url);
    return true;
  }

  window.fnsWarp = function (url) {
    if (!url) { return; }
    if (STILL) { location.href = url; return; }
    depart(url);
    // Let the overlay repaint its hint before the document is torn down, so the reader is not left
    // looking at a stale label for the last few milliseconds.
    // 让遮罩在文档被拆掉之前重画一次提示，读者不会在最后几毫秒里盯着一个过期的标签。
    if (view) { setHint('LOADING'); }
    location.href = url;
  };

  // ── Click delegation ──────────────────────────────────────────────────────────────────
  //
  // Only plain left clicks. Ctrl/cmd/middle mean "open elsewhere", and hijacking those is how a
  // link stops behaving like a link.
  //
  // 点击代理。只接管普通左键，Ctrl/⌘/中键表示「在别处打开」，抢走它们等于让链接不再像链接。
  var SKIP = /^(?:mailto:|tel:|javascript:|data:|blob:)/i;

  document.addEventListener('click', function (ev) {
    if (ev.defaultPrevented || ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) { return; }
    var a = ev.target && ev.target.closest ? ev.target.closest('a[href]') : null;
    if (!a) { return; }
    if (a.hasAttribute('download')) { return; }
    if (a.target && a.target !== '_self') { return; }

    var href = a.getAttribute('href') || '';
    if (!href || href.charAt(0) === '#') { return; }
    if (SKIP.test(href)) { return; }

    var target;
    try { target = new URL(href, location.href); } catch (e) { return; }
    if (target.origin !== location.origin) { return; }
    if (target.pathname === location.pathname && target.search === location.search) { return; }

    // Feeds, assets and downloads must not get a loading screen in front of them.
    // 订阅源、资源与下载不该在它们前面加一层加载画面。
    var last = target.pathname.split('/').pop() || '';
    if (/\.(?:xml|json|js|css|png|jpe?g|webp|gif|svg|ico|pdf|zip|txt|md|woff2?)$/i.test(last)) { return; }

    if (STILL) { return; }

    ev.preventDefault();
    window.fnsWarp(target.href);
  });

  // ── Arrival, back/forward, and the bfcache trap ───────────────────────────────────────
  //
  // pageshow fires on every load, including a restore from bfcache. A restore hands back the old
  // document exactly as it was left — which, for a page that was departed from, means html.fns-warping
  // still applied and body at opacity 0. Nothing else in the page would ever take that off, so the
  // reader got a white page and no way back. persisted=true is exactly that case, and it is the
  // only place that can fix it.
  //
  // pageshow 在每次加载时都会触发，包括从 bfcache 恢复。恢复交还的是旧文档离开时的原样 ——
  // 对一个「从它出发过」的页面来说，就是 html.fns-warping 仍然生效、body 停在 opacity 0。
  // 页面里没有任何别的代码会把它摘掉，于是读者拿到一张白页且没有回去的路。
  // persisted=true 正是这种情况，也只有这里能修它。
  window.addEventListener('pageshow', function (ev) {
    if (ev.persisted) {
      // Drop every trace of a previous departure, then show the document as it is.
      // 抹掉上一次出发留下的所有痕迹，然后照原样显示这个文档。
      document.documentElement.classList.remove('fns-warping', 'fns-arrived');
      if (view && view.root && view.root.parentNode) { view.root.parentNode.removeChild(view.root); }
      view = null;
      primed = false;
      finished = true;
      try { sessionStorage.removeItem(KEY_DEPART); } catch (e) {}
      return;
    }
    // A restored document already ran start() on its first load; a fresh one has not, and this is
    // the earliest point every page reaches exactly once.
    // 恢复的文档在它第一次加载时已经跑过 start()；新文档还没跑过，而这里是每一页恰好到达一次的
    // 最早时刻。
    start();
  });

  // Before the first paint. DOMContentLoaded is too late for the overlay to be there on frame one,
  // but the head script itself already runs early enough to create the node — this just starts the
  // clock on a document that may still be parsing.
  // 首帧之前。DOMContentLoaded 对「第一帧就有遮罩」来说太晚了，
  // 但 head 脚本本身跑得足够早，已经能创建这个节点 —— 这里只是在一个可能还在解析的文档上启动时钟。
  start();
})();

`

// blogWarpJS returns the transition script.
//
// Served on every blog page and injected in <head>, not at the end of the body: the overlay has to
// exist before the first frame or the reader sees the bare page and then a loading screen, which is
// exactly the ordering the owner reported as "no loading animation on entry".
//
// blogWarpJS 返回过渡脚本。
//
// 它在每个博客页面上提供，并注入到 <head> 而不是 body 末尾：遮罩必须在首帧之前就存在，
// 否则读者先看到光秃秃的页面、再看到加载画面 —— 那正是站主报的「入站时没有加载动画」。
func blogWarpJS() string {
	return blogWarpScript
}
