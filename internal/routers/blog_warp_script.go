package routers

// The hand-written loader. The artwork, the bezier coordinates, the mask-reveal technique and the
// timing weights are taken verbatim from the sample the owner supplied (博客加载.html) — see
// blog_warp_fixture_test.go, which parses that file and asserts this script still matches it.
//
// Two things are deliberately different from the sample, and only two:
//
//  1. mask_s and mask_B each hold TWO subpaths in the sample. getPointAtLength walks a
//     multi-subpath path by jumping between them, so the pen teleported 127 units at the seam of
//     "B" while the reveal kept drawing — the "pen does not follow the stroke" the owner kept
//     reporting. They are split into four single-subpath segments here, with the weights divided
//     in proportion to subpath length so the total timing is unchanged.
//
//  2. The sample plays once and lifts the pen off at the end. This one loops until the page is
//     ready, so a wrap-around air stroke carries the pen from the end of the flourish back to the
//     first stroke instead of teleporting.
//
// 手写加载动画。字形、贝塞尔坐标、蒙版揭示手法与时序权重，逐字取自站主给的示例
// （博客加载.html）—— 见 blog_warp_fixture_test.go，它解析那个文件并断言本脚本仍然与之一致。
//
// 与示例的差别只有两处，没有第三处：
//
//  1. 示例里 mask_s 与 mask_B 各自含有「两条」子路径。getPointAtLength 走多子路径时会在两者之间跳，
//     于是蒙版还在揭示、「B」的接缝处笔却瞬移了 127 单位 —— 这就是站主反复报的「书写不跟手」。
//     这里拆成四条单子路径的段落，权重按子路径长度比例分配，总时长不变。
//
//  2. 示例播一遍就把笔抬走。这一版要循环到页面就绪，所以补一条回绕抬笔，
//     把笔从底部弧线的末端带回第一笔，而不是瞬移。
const blogWarpScript = `
(function () {
  // ── Blog pages only ───────────────────────────────────────────────────────────────────
  // The error page has no shell and no reading to wait for; a loading screen in front of a 404 is
  // pure delay.
  // 只处理博客页。错误页没有外壳、也没有内容要等；在 404 前面放加载画面是纯粹的拖延。
  if (location.pathname.indexOf('/blog') !== 0) { return; }

  var STILL = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);

  // One-shot marker meaning "the pass already played on the page you left; show yourself directly".
  // It carries no timing, so it cannot accumulate the way a stored phase did.
  // 一次性标记，意思是「你离开的那一页已经播完了，直接显示自己」。
  // 它不带任何时间，因此不会像存下来的相位那样累积。
  var KEY_QUIET = 'fns:ink-quiet';

  // The sample's own base duration, unchanged.
  // 示例自己的基准时长，未改动。
  var PASS_MS = 2400;

  // ── Timing, verbatim from the sample ──────────────────────────────────────────────────
  //
  // weight is relative cost, not length: "endao" is the long run of cursive and the flourish is a
  // long shallow arc, so equal lengths would draw them at visibly different speeds.
  //
  // ── 时序，逐字取自示例 ────────────────────────────────────────────────────────────────
  //
  // weight 是相对代价而不是长度：「endao」是最长的一段连笔，底部弧线则是又长又浅的一道，
  // 只看长度会让它们用肉眼可见的不同速度画出来。
  //
  // s and B are split. The two numbers in the comment on each are (sample weight, sample weight)
  // divided by subpath length: 0.95 over 32/142 units, 1.45 over 155/379 units.
  // s 与 B 拆开了。各行注释里的两个数，是把示例的权重按子路径长度拆开所得：
  // 0.95 分成 32/142 单位，1.45 分成 155/379 单位。
  var SEGDEFS = [
    { id: 'W',         weight: 1.25, endAngle: 42 },
    { id: 'endao',     weight: 2.70, endAngle: 38 },
    { id: 's1',        weight: 0.17, endAngle: 40 },
    { id: 's2',        weight: 0.78, endAngle: 40 },
    { id: 'B1',        weight: 0.42, endAngle: 36 },
    { id: 'B2',        weight: 1.03, endAngle: 36 },
    { id: 'log',       weight: 2.20, endAngle: 42 },
    { id: 'Underline', weight: 1.40, endAngle: 35 }
  ];

  // Pen-up travel, from the sample, plus the wrap that looping needs.
  // 抬笔位移，取自示例，另加循环需要的回绕。
  var AIRDEFS = [
    { from: 0, to: 1, weight: 0.18 },
    { from: 1, to: 2, weight: 0.16 },
    { from: 2, to: 3, weight: 0.04 },
    { from: 3, to: 4, weight: 0.32 },
    { from: 4, to: 5, weight: 0.08 },
    { from: 5, to: 6, weight: 0.18 },
    { from: 6, to: 7, weight: 0.08 },
    { from: 7, to: 0, weight: 0.30 }
  ];

  // Path geometry, verbatim from the sample's mask. One subpath each.
  // 路径几何，逐字取自示例的 mask。每条一条子路径。
  var D = {
    W: 'M 130,195 C 115,200 110,230 115,258 C 122,298 140,318 160,318 C 178,318 194,290 205,255 C 218,290 234,318 252,318 C 274,318 290,270 298,205 C 302,185 292,180 282,184 C 274,188 274,204 282,224 C 290,248 304,268 318,268',
    endao: 'M 318,268 C 328,268 344,242 355,228 C 362,218 354,210 344,216 C 334,222 328,244 330,266 C 332,284 345,294 360,290 C 375,286 385,268 392,252 C 396,242 404,236 412,240 C 418,245 414,265 410,288 C 420,258 435,236 450,236 C 462,236 462,254 458,288 C 470,262 485,238 502,238 C 486,242 474,258 474,275 C 474,292 485,300 498,298 C 510,296 520,280 526,260 L 530,165 C 530,154 524,154 522,165 L 518,295 C 518,303 528,303 538,292 C 548,272 562,240 580,240 C 565,244 554,258 554,274 C 554,290 565,298 576,296 C 588,294 598,278 602,260 L 600,295 C 600,302 610,302 620,290 C 630,272 642,240 660,240 C 645,244 634,258 634,274 C 634,290 645,298 658,296 C 674,294 682,274 682,256 C 682,242 672,240 664,244 C 655,248 662,258 674,258',
    s1: 'M 698,198 C 696,208 692,220 687,228',
    s2: 'M 698,280 C 708,252 722,238 732,240 C 738,242 738,252 730,260 C 720,272 718,282 724,290 C 730,298 742,294 750,286',
    B1: 'M 798,165 C 794,180 782,235 778,295 C 776,306 768,308 762,300',
    B2: 'M 788,178 C 804,162 842,162 848,195 C 852,216 836,234 814,240 C 840,244 860,262 858,285 C 856,304 834,310 812,306 C 796,303 794,294 804,290 C 814,286 828,292 836,282 C 842,272 832,252 806,248',
    log: 'M 852,284 C 872,244 902,158 912,158 C 918,158 916,172 905,210 L 888,295 C 888,302 896,302 906,290 C 916,272 928,240 946,240 C 932,244 920,258 920,274 C 920,290 932,298 944,296 C 958,294 966,274 966,256 C 966,242 958,240 950,244 C 942,248 948,258 960,258 C 972,244 984,240 998,240 C 982,244 972,258 972,274 C 972,290 982,298 994,296 C 1005,294 1014,280 1018,260 L 1016,330 C 1012,375 994,405 956,412',
    Underline: 'M 956,412 C 908,416 835,410 740,400 C 560,380 370,380 230,398 C 175,405 150,398 165,386 C 185,372 240,375 320,378'
  };

  // Pressure swells, verbatim from the sample. These are what make the lettering read as written
  // rather than plotted; the sample keeps them in a separate group from the hairlines.
  // 按压加粗，逐字取自示例。它们让字形读起来像「写」而不是「描」；示例把它们与细骨分开成组。
  var SWELLS = [
    { d: 'M 115,225 C 117,250 128,285 145,310', w: 7.5 },
    { d: 'M 205,255 C 218,290 232,315 245,316', w: 6.8 },
    { d: 'M 352,228 C 342,238 332,254 334,272', w: 5.5 },
    { d: 'M 412,240 C 418,250 415,268 412,286', w: 6.5 },
    { d: 'M 454,246 C 460,256 460,270 458,286', w: 6.5 },
    { d: 'M 498,242 C 484,248 476,262 476,276 C 476,290 484,298 495,298', w: 5.5 },
    { d: 'M 528,185 L 522,295', w: 7.5 },
    { d: 'M 578,244 C 564,248 556,262 556,276 C 556,288 564,296 575,296', w: 5.5 },
    { d: 'M 602,260 L 600,294', w: 6.5 },
    { d: 'M 658,244 C 644,248 636,262 636,276 C 636,288 644,296 655,296', w: 5.5 },
    { d: 'M 794,180 C 790,215 780,260 778,295', w: 8.0 },
    { d: 'M 830,166 C 844,172 850,186 846,204 C 842,220 830,230 816,236', w: 6.0 },
    { d: 'M 834,244 C 852,250 858,266 854,284 C 850,298 834,306 816,304', w: 7.0 },
    { d: 'M 915,178 L 892,295', w: 8.0 },
    { d: 'M 944,244 C 930,248 922,262 922,276 C 922,288 930,296 942,296', w: 5.5 },
    { d: 'M 996,244 C 982,248 974,262 974,276 C 974,288 982,296 994,296', w: 5.5 },
    { d: 'M 1018,260 L 1016,335 C 1012,375 994,405 956,412', w: 7.5 },
    { d: 'M 850,409 C 680,392 480,385 270,394', w: 4.8 }
  ];

  // Wet ink sheen, verbatim from the sample.
  // 未干墨光，逐字取自示例。
  var SHEEN = [
    { d: 'M 122,240 C 126,268 136,295 148,310', w: 2.2 },
    { d: 'M 526,195 L 522,285', w: 2.0 },
    { d: 'M 792,190 C 788,225 780,268 778,292', w: 2.4 },
    { d: 'M 912,190 L 894,285', w: 2.2 },
    { d: 'M 1016,325 C 1012,365 996,392 965,408', w: 2.2 }
  ];

  var NS = 'http://www.w3.org/2000/svg';

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

  // ── Theme ─────────────────────────────────────────────────────────────────────────────
  //
  // The sample is a light-paper demo: near-black ink on cream. On this site the overlay is painted
  // in the reader's theme, so the ink gradient flips and the guide lines dim. The pen itself does
  // not change — an 18K nib with gold furniture reads correctly on both.
  //
  // 主题
  //
  // 示例是浅纸演示：米色底上的近黑墨。本站的遮罩按读者当前主题上色，
  // 所以墨色渐变要翻转、参考线要压暗。钢笔本身不变 —— 18K 笔尖配金件在两种底色上都成立。
  function isDark() {
    var de = document.documentElement;
    if (de.classList.contains('dark')) { return true; }
    if (de.classList.contains('light')) { return false; }
    try { return !!(window.matchMedia && window.matchMedia('(prefers-color-scheme:dark)').matches); } catch (e) {}
    return false;
  }

  // The critical shell declarations are inline, not left to the stylesheet: this script runs in
  // <head>, which is before the <style> block that follows it. A bare div there paints unstyled —
  // measured on a 390x844 viewport, an overlay 390 wide and 529 tall with an <svg> drawing at its
  // intrinsic 1100x500.
  // 外壳的关键声明内联，不留给样式表：本脚本跑在 <head> 里，按定义在它后面的 <style> 之前。
  // 那里一个裸 div 会以无样式状态被画出 —— 在 390x844 视口上实测到遮罩 390 宽、529 高，
  // 而 <svg> 按内在的 1100x500 作画。
  var SHELL_STYLE = 'position:fixed;top:0;right:0;bottom:0;left:0;' +
    'z-index:2147483000;pointer-events:none;overflow:hidden;';

  // ── Build ─────────────────────────────────────────────────────────────────────────────
  function buildOverlay() {
    var dark = isDark();
    var paper = dark ? '#1a1a1a' : '#fbfaf7';
    var inkA = dark ? '#eef2fa' : '#141a29';
    var inkB = dark ? '#e2e8f4' : '#0f1422';
    var inkC = dark ? '#f4f7fd' : '#1c2333';
    var guide = dark ? '#3a3d44' : '#d8ccbb';

    var root = htmlEl('div', { 'class': 'fns-ink', 'aria-hidden': 'true' });
    root.setAttribute('style', SHELL_STYLE + 'background:' + paper + ';');

    var svg = svgEl('svg', {
      viewBox: '0 0 1100 500',
      preserveAspectRatio: 'xMidYMid meet',
      'class': 'fns-ink-svg',
      // Intrinsic size, and a bounded width before the stylesheet lands.
      // 内在尺寸，并在样式表到达之前就限住宽度。
      width: '1100',
      height: '500',
      style: 'display:block;width:min(86vw,760px);height:auto;max-height:64vh;'
    });

    var defs = svgEl('defs');

    function grad(id, stops, x2, y2) {
      var g = svgEl('linearGradient', { id: id, x1: '0%', y1: '0%', x2: x2, y2: y2 });
      stops.forEach(function (s) {
        g.appendChild(svgEl('stop', { offset: s[0], 'stop-color': s[1], 'stop-opacity': s.length > 2 ? s[2] : '1' }));
      });
      defs.appendChild(g);
    }

    // The sample's gradients, with the ink colours following the theme.
    // 示例的渐变，墨色随主题。
    grad('fnsDeepInk', [['0%', inkA], ['50%', inkB], ['100%', inkC]], '100%', '80%');
    grad('fnsWetSheen', [['0%', '#79a9f5', '0'], ['50%', '#a4c7ff', '0.8'], ['100%', '#79a9f5', '0']], '100%', '0%');
    grad('fnsGoldSheen', [['0%', '#ffe8a3'], ['35%', '#d8ab4e'], ['70%', '#9a7322'], ['100%', '#644810']], '100%', '100%');
    grad('fnsRhodium', [['0%', '#cad0d9'], ['45%', '#ffffff'], ['75%', '#dbe0e8'], ['100%', '#9ea5b0']], '100%', '0%');
    grad('fnsBarrel', [['0%', '#121317'], ['40%', '#2d323b'], ['70%', '#181a1f'], ['100%', '#0d0e12']], '100%', '0%');

    // Ink bleed on the artwork.
    // 字形上的墨迹微渗。
    var inkSpread = svgEl('filter', { id: 'fnsInkSpread', x: '-10%', y: '-10%', width: '120%', height: '120%' });
    inkSpread.appendChild(svgEl('feDropShadow', {
      dx: '0.5', dy: '1.2', stdDeviation: '0.8', 'flood-color': dark ? '#000000' : '#0b101d', 'flood-opacity': '0.18'
    }));
    defs.appendChild(inkSpread);

    // The pen's cast shadow. The sample animates the blur, offset and alpha ids; they are declared
    // here so the ids exist, and the renderer nudges them as the pen moves.
    // 钢笔投影。示例会驱动 blur/offset/alpha 三个 id；这里声明它们，好让渲染器随笔移动微调。
    var penShadow = svgEl('filter', { id: 'fnsPenShadow', x: '-60%', y: '-60%', width: '220%', height: '220%' });
    penShadow.appendChild(svgEl('feGaussianBlur', { in: 'SourceAlpha', stdDeviation: '5', id: 'fnsShadowBlur' }));
    penShadow.appendChild(svgEl('feOffset', { dx: '16', dy: '22', id: 'fnsShadowOffset' }));
    var comp = svgEl('feComponentTransfer');
    comp.appendChild(svgEl('feFuncA', { type: 'linear', slope: '0.22', id: 'fnsShadowAlpha' }));
    penShadow.appendChild(comp);
    var merge = svgEl('feMerge');
    merge.appendChild(svgEl('feMergeNode'));
    merge.appendChild(svgEl('feMergeNode', { in: 'SourceGraphic' }));
    penShadow.appendChild(merge);
    defs.appendChild(penShadow);

    var wetGlow = svgEl('filter', { id: 'fnsWetGlow' });
    wetGlow.appendChild(svgEl('feGaussianBlur', { stdDeviation: '1.5', result: 'glow' }));
    var glowMerge = svgEl('feMerge');
    glowMerge.appendChild(svgEl('feMergeNode', { in: 'glow' }));
    glowMerge.appendChild(svgEl('feMergeNode', { in: 'SourceGraphic' }));
    wetGlow.appendChild(glowMerge);
    defs.appendChild(wetGlow);

    // ── The reveal mask ────────────────────────────────────────────────────────────────
    //
    // This is what the sample does and what my earlier rewrites got wrong: the artwork is not
    // stroked progressively. It sits complete under a mask whose white strokes are 26 units wide,
    // and only the dashes of those strokes are animated. That is why the lettering keeps its
    // thick/thin character — the pen is revealing finished ink, not drawing a thin line.
    //
    // ── 揭示蒙版 ──────────────────────────────────────────────────────────────────────
    //
    // 这是示例的做法，也是我前几次重写搞错的地方：字形不是被一点点描出来的。
    // 它完整地待在蒙版之下，蒙版里是 26 单位宽的白色描边，只有这些描边的 dash 在动。
    // 这就是为什么字形保留了粗细变化 —— 笔是在揭示已经写好的墨，而不是在画一条细线。
    var mask = svgEl('mask', {
      id: 'fnsMask', maskUnits: 'userSpaceOnUse', x: '0', y: '0', width: '1100', height: '500'
    });

    var segEls = {};
    SEGDEFS.forEach(function (seg) {
      var p = svgEl('path', {
        id: 'fnsMask' + seg.id,
        d: D[seg.id],
        fill: 'none',
        stroke: '#fff',
        'stroke-width': '26',
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round'
      });
      mask.appendChild(p);
      segEls[seg.id] = p;
    });
    defs.appendChild(mask);
    svg.appendChild(defs);

    // ── Guide lines ────────────────────────────────────────────────────────────────────
    // From the sample; they read as a craftsman's ruling and cost nothing.
    // 参考线。取自示例；读起来像匠人的辅助线，代价为零。
    var guides = svgEl('g', { opacity: '0.28', stroke: guide, 'stroke-dasharray': '3 7', 'stroke-width': '0.8' });
    [[160, '3 7'], [240, '4 6'], [300, 'none'], [410, '3 7']].forEach(function (row) {
      guides.appendChild(svgEl('line', {
        x1: '80', y1: String(row[0]), x2: '1020', y2: String(row[0]),
        'stroke-dasharray': row[1], 'stroke-width': row[0] === 300 ? '1.1' : '0.8'
      }));
    });
    svg.appendChild(guides);

    // ── The artwork, complete, under the mask ──────────────────────────────────────────
    // ── 完整字形，位于蒙版之下 ──────────────────────────────────────────────────────────
    var art = svgEl('g', { mask: 'url(#fnsMask)', filter: 'url(#fnsInkSpread)' });

    var hair = svgEl('g', {
      fill: 'none', stroke: 'url(#fnsDeepInk)',
      'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2.6'
    });
    SEGDEFS.forEach(function (seg) { hair.appendChild(svgEl('path', { d: D[seg.id] })); });
    art.appendChild(hair);

    var swell = svgEl('g', { fill: 'none', stroke: 'url(#fnsDeepInk)', 'stroke-linecap': 'round' });
    SWELLS.forEach(function (s) { swell.appendChild(svgEl('path', { d: s.d, 'stroke-width': String(s.w) })); });
    art.appendChild(swell);

    var sheen = svgEl('g', {
      fill: 'none', stroke: 'url(#fnsWetSheen)', 'stroke-linecap': 'round',
      opacity: '0.6', filter: 'url(#fnsWetGlow)'
    });
    SHEEN.forEach(function (s) { sheen.appendChild(svgEl('path', { d: s.d, 'stroke-width': String(s.w) })); });
    art.appendChild(sheen);

    svg.appendChild(art);

    // The wet meniscus at the nib.
    // 笔尖处的湿润墨点。
    var inkPoint = svgEl('circle', {
      id: 'fnsNibInk', cx: '-100', cy: '-100', r: '3',
      fill: dark ? '#e8eefb' : '#0e1422', opacity: '0', filter: 'url(#fnsWetGlow)'
    });
    svg.appendChild(inkPoint);

    // ── The pen ────────────────────────────────────────────────────────────────────────
    //
    // Structure and every coordinate from the sample: origin at the nib tip, body scaled 0.85.
    // ── 钢笔 ──────────────────────────────────────────────────────────────────────────
    //
    // 结构与每一个坐标都取自示例：原点在笔尖，笔身缩放 0.85。
    var pen = svgEl('g', { id: 'fnsPen', transform: 'translate(-300, -300)', filter: 'url(#fnsPenShadow)' });
    var body = svgEl('g', { transform: 'scale(0.85)' });

    body.appendChild(svgEl('path', { d: 'M -15,-130 L 15,-130 L 13.5,-72 L -13.5,-72 Z', fill: 'url(#fnsBarrel)' }));
    body.appendChild(svgEl('path', { d: 'M -4,-130 L 3,-130 L 2.5,-72 L -3.5,-72 Z', fill: '#ffffff', opacity: '0.12' }));
    body.appendChild(svgEl('rect', {
      x: '-14.5', y: '-76', width: '29', height: '5', rx: '1',
      fill: 'url(#fnsGoldSheen)', stroke: '#6d4f13', 'stroke-width': '0.6'
    }));
    body.appendChild(svgEl('line', { x1: '-14', y1: '-73.5', x2: '14', y2: '-73.5', stroke: '#fff', 'stroke-width': '0.8', opacity: '0.6' }));
    body.appendChild(svgEl('path', {
      d: 'M 0,0 C 2.8,-7 8.5,-22 12,-34 C 15.5,-46 17.5,-58 15.5,-72 L -15.5,-72 C -17.5,-58 -15.5,-46 -12,-34 C -8.5,-22 -2.8,-7 0,0 Z',
      fill: 'url(#fnsGoldSheen)', stroke: '#745214', 'stroke-width': '0.7'
    }));
    body.appendChild(svgEl('path', {
      d: 'M 0,-7 C 2.2,-14 6.8,-26 9.5,-36 C 11.8,-46 12.8,-56 11.2,-68 L -11.2,-68 C -12.8,-56 -11.8,-46 -9.5,-36 C -6.8,-26 -2.2,-14 0,-7 Z',
      fill: 'url(#fnsRhodium)', stroke: '#929aa6', 'stroke-width': '0.4'
    }));
    body.appendChild(svgEl('circle', { cx: '0', cy: '-35', r: '2.8', fill: '#12151b', stroke: '#684a10', 'stroke-width': '0.7' }));
    body.appendChild(svgEl('line', { x1: '0', y1: '0', x2: '0', y2: '-32.2', stroke: '#12151b', 'stroke-width': '0.75' }));
    body.appendChild(svgEl('path', { d: 'M -7.5,-48 C -5,-55 -3,-58 0,-58 C 3,-58 5,-55 7.5,-48', fill: 'none', stroke: '#aa8227', 'stroke-width': '0.6' }));
    body.appendChild(svgEl('path', { d: 'M -5.5,-43 C -3.5,-48 0,-50 0,-50 C 0,-50 3.5,-48 5.5,-43', fill: 'none', stroke: '#aa8227', 'stroke-width': '0.5' }));

    var mark = svgEl('text', {
      x: '0', y: '-40', 'font-size': '3.8', 'font-family': 'Georgia, serif',
      'font-weight': 'bold', fill: '#7a5813', 'text-anchor': 'middle', 'letter-spacing': '0.4'
    });
    mark.textContent = '18K';
    body.appendChild(mark);

    body.appendChild(svgEl('circle', { cx: '0', cy: '0', r: '1.1', fill: '#f4f6fa', stroke: '#667080', 'stroke-width': '0.3' }));

    pen.appendChild(body);
    svg.appendChild(pen);

    var hint = htmlEl('div', { 'class': 'fns-ink-hint' });
    hint.innerHTML = '<span class="fns-ink-word">LOADING</span><i class="fns-ink-dot"></i>';

    root.appendChild(svg);
    root.appendChild(hint);

    return {
      root: root, svg: svg, pen: pen, inkPoint: inkPoint, hint: hint,
      dark: dark
    };
  }

  // ── Timeline ─────────────────────────────────────────────────────────────────────────
  var segments = null;
  var timeline = null;
  var totalWeight = 0;
  var primed = false;
  var render = null;
  var startedAt = 0;

  function elapsedNow() { return (performance.now ? performance.now() : Date.now()) - startedAt; }

  function prime(layer) {
    if (primed || !layer) { return; }

    segments = SEGDEFS.map(function (def) {
      var el = layer.svg.querySelector('#fnsMask' + def.id);
      return { id: def.id, weight: def.weight, endAngle: def.endAngle, el: el, length: 0 };
    });

    totalWeight = 0;
    segments.forEach(function (seg) {
      // The sample's exact dash setup, including the +5 slack that keeps the round cap from
      // peeking at offset zero.
      // 示例原样的 dash 设置，包括那个 +5 余量 —— 没有它，圆头帽会在 offset 归零时露出一点。
      seg.length = seg.el.getTotalLength();
      seg.el.style.strokeDasharray = seg.length + ' ' + (seg.length + 5);
      seg.el.style.strokeDashoffset = String(seg.length);
      totalWeight += seg.weight;
    });
    AIRDEFS.forEach(function (t) { totalWeight += t.weight; });

    timeline = [];
    var acc = 0;
    for (var i = 0; i < segments.length; i++) {
      var seg = segments[i];
      var t0 = acc / totalWeight;
      acc += seg.weight;
      var t1 = acc / totalWeight;
      timeline.push({ type: 'draw', seg: seg, start: t0, end: t1 });

      for (var a = 0; a < AIRDEFS.length; a++) {
        if (AIRDEFS[a].from !== i) { continue; }
        var air = AIRDEFS[a];
        var a0 = acc / totalWeight;
        acc += air.weight;
        var a1 = acc / totalWeight;
        timeline.push({
          type: 'air',
          fromPt: seg.el.getPointAtLength(seg.length),
          toPt: segments[air.to].el.getPointAtLength(0),
          start: a0, end: a1
        });
      }
    }

    render = makeRenderer(layer);
    primed = true;
  }

  // The sample's easing: a symmetric ease-in-out, which is what makes the stroke accelerate out of
  // a corner and settle into the next one.
  // 示例的缓动：对称的慢入慢出，正是它让笔画从转角加速出来、再收进下一个转角。
  function handEase(t) { return t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2; }

  function makeRenderer(layer) {
    // The rendered layer is captured here as a parameter, never read from a module-level variable.
    // A previous version read the module variable inside this closure and crashed with
    // "Cannot read properties of null" once that variable had been cleared after the first pass.
    // 被渲染的那一层作为参数在这里捕获，不读模块级变量。
    // 早先的版本在闭包里读模块变量，第一次播完那个变量被清空后就崩了。
    function paint(progress) {
      var p = Math.max(0, Math.min(1, progress));
      var penPos = null;
      var penAngle = 40;
      var lifting = false;

      for (var i = 0; i < timeline.length; i++) {
        var item = timeline[i];

        if (item.type === 'draw') {
          var seg = item.seg;
          if (p <= item.start) {
            seg.el.style.strokeDashoffset = String(seg.length);
          } else if (p >= item.end) {
            seg.el.style.strokeDashoffset = '0';
          } else {
            var strokeT = (p - item.start) / (item.end - item.start);
            var eased = handEase(strokeT);
            var drawn = seg.length * eased;
            seg.el.style.strokeDashoffset = String(seg.length - drawn);

            // Exactly the sample's pen tracking: the nib sits at the point the mask has just
            // revealed, and leans by the local tangent.
            // 与示例完全相同的笔迹跟踪：笔尖就停在蒙版刚刚揭示到的那一点上，并按局部切线倾斜。
            var pt = seg.el.getPointAtLength(drawn);
            var nextPt = seg.el.getPointAtLength(Math.min(seg.length, drawn + 2));
            penPos = pt;
            var dx = nextPt.x - pt.x;
            var dy = nextPt.y - pt.y;
            var drift = Math.atan2(dy, Math.abs(dx) + 0.1) * (12 / Math.PI);
            penAngle = 39 + drift;
          }
        } else if (p > item.start && p < item.end) {
          lifting = true;
          var airT = (p - item.start) / (item.end - item.start);
          var easedAir = 0.5 - 0.5 * Math.cos(airT * Math.PI);
          var arc = -22 * Math.sin(airT * Math.PI);

          penPos = {
            x: item.fromPt.x + (item.toPt.x - item.fromPt.x) * easedAir,
            y: item.fromPt.y + (item.toPt.y - item.fromPt.y) * easedAir + arc
          };
          penAngle = 40 - Math.sin(airT * Math.PI) * 4;
        }
      }

      if (penPos) {
        layer.pen.setAttribute('transform', 'translate(' + penPos.x + ',' + penPos.y + ') rotate(' + penAngle + ')');
        layer.pen.style.opacity = '1';
        layer.inkPoint.setAttribute('cx', String(penPos.x));
        layer.inkPoint.setAttribute('cy', String(penPos.y));
        layer.inkPoint.setAttribute('opacity', lifting ? '0' : '0.55');
      } else {
        // At the seam of a loop the pen waits above the first stroke rather than vanishing.
        // 循环的接缝处，笔停在第一笔上方，而不是消失。
        var home = segments[0].el.getPointAtLength(0);
        layer.pen.setAttribute('transform', 'translate(' + home.x + ',' + (home.y - 46) + ') rotate(39)');
        layer.pen.style.opacity = '1';
        layer.inkPoint.setAttribute('opacity', '0');
      }
    }
    return paint;
  }

  // ── Lifecycle ────────────────────────────────────────────────────────────────────────
  var view = null;
  var finished = false;
  var raf = 0;

  function setHint(word) {
    var w = view && view.hint.querySelector('.fns-ink-word');
    if (w && w.textContent !== word) { w.textContent = word; }
  }

  function imagesSettled() {
    var imgs = document.images;
    for (var i = 0; i < imgs.length; i++) {
      var img = imgs[i];
      if (img.complete) { continue; }
      // A lazy image below the fold has not been asked for; waiting would hold the overlay until the
      // reader scrolls.
      // 视口之外的懒加载图还没被请求，等它会把遮罩扣到读者滚动为止。
      if (img.loading === 'lazy') { continue; }
      return false;
    }
    return true;
  }

  function documentSettled() {
    if (document.readyState !== 'complete') { return false; }
    return imagesSettled();
  }

  function finish() {
    if (finished) { return; }
    finished = true;
    if (raf) { cancelAnimationFrame(raf); raf = 0; }

    document.documentElement.classList.remove('fns-warping');
    document.documentElement.classList.add('fns-arrived');

    var node = view && view.root;
    if (node) {
      node.classList.add('is-done');
      window.setTimeout(function () {
        if (node && node.parentNode) { node.parentNode.removeChild(node); }
      }, 420);
    }
    view = null;

    window.setTimeout(function () {
      document.documentElement.classList.remove('fns-arrived');
    }, 700);
  }

  function tick() {
    if (finished) { return; }
    if (!primed && view) { prime(view); }
    if (render) { render((elapsedNow() % PASS_MS) / PASS_MS); }
    raf = requestAnimationFrame(tick);
  }

  function start() {
    if (view) { return; }
    if (STILL) { return; }

    view = buildOverlay();
    primed = false;
    render = null;

    document.documentElement.appendChild(view.root);
    document.documentElement.classList.add('fns-warping');

    void view.root.offsetWidth;
    startedAt = performance.now ? performance.now() : Date.now();
    prime(view);
    if (render) { render(0); }

    raf = requestAnimationFrame(tick);
  }

  // ── Fetching the destination ─────────────────────────────────────────────────────────
  function warmDestination(url) {
    return new Promise(function (resolve) {
      var done = false;
      function settle() { if (!done) { done = true; resolve(); } }
      var bail = setTimeout(settle, 6000);

      fetch(url, { credentials: 'same-origin' })
        .then(function (r) { return r.text(); })
        .then(function (html) {
          var re = /<img[^>]+src="([^"]+)"/g;
          var m, seen = {}, srcs = [];
          while ((m = re.exec(html)) !== null) {
            var src = m[1].replace(/&amp;/g, '&');
            if (seen[src]) { continue; }
            seen[src] = 1;
            srcs.push(src);
          }
          srcs = srcs.slice(0, 14);
          if (!srcs.length) { clearTimeout(bail); settle(); return; }

          var left = srcs.length;
          var settled = false;
          function one() {
            left--;
            if (left <= 0 && !settled) { settled = true; clearTimeout(bail); settle(); }
          }
          srcs.forEach(function (s) {
            try {
              var im = new Image();
              im.onload = one;
              im.onerror = one;
              im.src = s;
            } catch (e) { one(); }
          });
        })
        .catch(function () { clearTimeout(bail); settle(); });
    });
  }

  // ── Departure: one pass, fetching in parallel, leave when both are done ──────────────
  //
  // The owner's sequence: 点击立刻播放 → 加载 → 播放完就进入新界面.
  // 站主的时序：点击立刻播放 → 加载 → 播放完就进入新界面。
  var departing = false;

  function depart(url) {
    if (!url) { return; }
    if (STILL) { location.href = url; return; }
    if (departing) { return; }
    departing = true;

    // A fresh overlay and a fresh clock, so the pass always starts at the first stroke.
    // 新的遮罩、新的时钟，这一遍永远从第一笔开始。
    if (view) {
      if (view.root && view.root.parentNode) { view.root.parentNode.removeChild(view.root); }
      view = null;
    }
    finished = false;
    start();
    startedAt = performance.now ? performance.now() : Date.now();
    if (render) { render(0); }
    setHint('LOADING');

    var ready = false;
    var left = false;
    // The pass is the authority; the warm-up gets a short grace period. Waiting for a slow cover
    // would turn "when the pass is done" into "when the covers are done".
    // 这一遍是准绳，预热只给很短的宽限。等一张慢封面会把「这一遍播完」变成「等封面加载完」。
    var GRACE_MS = 500;

    warmDestination(url).then(function () { ready = true; });

    function leave() {
      if (left) { return; }
      left = true;
      setHint('READY');
      try { sessionStorage.setItem(KEY_QUIET, '1'); } catch (e) {}
      setTimeout(function () { location.href = url; }, 180);
    }

    function watch() {
      if (left) { return; }
      var el = elapsedNow();
      if (el >= PASS_MS && (ready || el >= PASS_MS + GRACE_MS)) { leave(); return; }
      requestAnimationFrame(watch);
    }
    requestAnimationFrame(watch);
  }

  window.fnsWarp = function (url) { depart(url); };

  // ── Click delegation ─────────────────────────────────────────────────────────────────
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

  // ── Arrival and back/forward ─────────────────────────────────────────────────────────
  //
  // pageshow fires on every load, including a restore from bfcache. A restore hands the document
  // back exactly as it was left — for a page that was departed from, still carrying fns-warping and
  // a half-finished overlay. Nothing else would ever clear that, so the reader got a white page.
  //
  // pageshow 在每次加载时都会触发，包括从 bfcache 恢复。恢复交还的是文档离开时的原样 ——
  // 对一个「从它出发过」的页面来说，仍带着 fns-warping 和一张画到一半的遮罩。
  // 没有别的代码会清掉它，于是读者拿到一张白页。
  window.addEventListener('pageshow', function (ev) {
    if (ev.persisted) {
      document.documentElement.classList.remove('fns-warping', 'fns-arrived');
      if (view && view.root && view.root.parentNode) { view.root.parentNode.removeChild(view.root); }
      view = null;
      primed = false;
      finished = true;
      return;
    }
    if (STILL) { return; }
    // A fresh document: hold the overlay for one whole pass and until the page can be looked at.
    // 新文档：留住遮罩，直到完整播完一遍、并且页面真的可以被看。
    revealWhenReady();
  });

  function revealWhenReady() {
    var noticed = false;
    function step() {
      if (finished) { return; }
      if (elapsedNow() >= PASS_MS && documentSettled()) {
        if (!noticed) { noticed = true; setHint('READY'); }
        finish();
        return;
      }
      requestAnimationFrame(step);
    }
    requestAnimationFrame(step);
  }

  // ── Entry ────────────────────────────────────────────────────────────────────────────
  //
  // Read before the overlay is built, so a page reached through a transition never paints one.
  // 在构建遮罩之前读，于是经由过渡抵达的页面根本不会画出遮罩。
  var quiet = false;
  try {
    quiet = sessionStorage.getItem(KEY_QUIET) === '1';
    if (quiet) { sessionStorage.removeItem(KEY_QUIET); }
  } catch (e) {}

  if (quiet) {
    document.documentElement.classList.add('fns-arrived');
    window.setTimeout(function () {
      document.documentElement.classList.remove('fns-arrived');
    }, 700);
  } else {
    start();
  }
})();

`

// blogWarpJS returns the loading-overlay script.
//
// Injected from <head>, not at the end of <body>: the overlay has to exist before the first painted
// frame, otherwise the reader sees the bare page and then a loading screen — the ordering the owner
// reported as "no loading animation on entry".
//
// blogWarpJS 返回加载遮罩脚本。
//
// 从 <head> 注入，而不是 body 末尾：遮罩必须在第一个被绘制的帧之前就存在，
// 否则读者先看到光秃秃的页面、再看到加载画面 —— 那正是站主报的「入站时没有加载动画」。
func blogWarpJS() string {
	return blogWarpScript
}
