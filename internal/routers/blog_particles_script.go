package routers

// The home page's ambient particle field.
//
// ── Why this is written rather than pulled from a CDN ────────────────────────────────────
//
// The obvious implementation is tsParticles from a CDN, and that is what the config this was
// modelled on assumed. It is not what shipped, for two reasons:
//
//  1. This service is self-contained on purpose: it must not reference assets it does not ship.
//     A blog that breaks because a third-party host is unreachable is not a blog.
//  2. This one runs on an internal address. From there a public CDN may simply not resolve, and
//     the failure mode is a silently blank page rather than an error anyone would notice.
//
// The behaviour below reproduces the config's semantics: polygon particles, a link whenever two
// are within range, gravity, drift, grab on hover and push on click.
//
// ── 为什么自己写，而不是从 CDN 引 ────────────────────────────────────────────────────────
//
// 最省事的做法是从 CDN 引 tsParticles，而这套配置原本也是按那个假设写的。最终没有这样做，有两个原因：
//
//  1. 这个服务刻意保持自包含：它不应当引用自己没有随包发布的资源。
//     一个因为第三方主机不可达就坏掉的博客，不是一个博客。
//  2. 它跑在一个内网地址上。从那里公共 CDN 可能根本解析不了，而失败形态是一个静默的空白页，
//     而不是谁都能注意到的报错。
//
// 下面的行为复现了那份配置的语义：多边形粒子、两个粒子靠近时连线、重力、漂移、
// 悬停抓取与点击推入。
//
// ── The one number that matters ─────────────────────────────────────────────────────────
//
// The particle count is the number of published notes. That is the point of the effect here: the
// field is a picture of how much is in the blog, and it changes when a note is shared or unshared.
//
// ── 唯一重要的那个数字 ──────────────────────────────────────────────────────────────────
//
// 粒子数等于已分享笔记数。这正是这个效果在这里的意义：这片场是「博客里有多少东西」的一张图，
// 它会随着分享或取消分享而变化。
const blogParticlesScript = `
(function () {
  var cfg = document.getElementById('fns-particles-cfg');
  if (!cfg) return;
  var CFG = {};
  try { CFG = JSON.parse(cfg.textContent) || {}; } catch (e) { return; }

  var COUNT = CFG.count | 0;
  if (COUNT <= 0) return;

  // A hard ceiling. Links are O(n²): at 200 particles that is 19,900 distance checks per frame,
  // which is fine; at 2000 it is 2 million and the page stops scrolling. The count is honest up to
  // this point and then stops growing — which is better than an animation that makes the article
  // list stutter on a blog that has been written for years.
  //
  // 硬上限。连线是 O(n²)：200 个粒子是每帧 19,900 次距离计算，没问题；
  // 2000 个就是 200 万次，页面会连滚动都卡。到这个数之前计数是诚实的，之后不再增长——
  // 这比「一个写了很多年的博客上，动画让文章列表开始卡顿」要好。
  var MAX = 220;
  var capped = COUNT > MAX;
  COUNT = Math.min(COUNT, MAX);

  var STILL = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);

  var canvas = document.createElement('canvas');
  canvas.className = 'fns-particles';
  canvas.setAttribute('aria-hidden', 'true');
  document.body.appendChild(canvas);
  var ctx = canvas.getContext('2d');
  if (!ctx) { canvas.remove(); return; }

  var w = 0, h = 0, dpr = 1;
  // Declared before resize(), which assigns them: a var declared after a call site is hoisted but
  // undefined there, and its initialiser would then overwrite what resize() computed. It happens to
  // recompute afterwards, so the value came out right — which is exactly the kind of accident that
  // stops being harmless the moment someone deletes the second call.
  //
  // 声明放在 resize() 之前，因为 resize() 会给它们赋值：在调用点之后声明的 var 虽然被提升，
  // 但在那里是 undefined，而它的初始化式随后会覆盖 resize() 算出的结果。
  // 碰巧后面又重算了一次，所以值是对的 —— 而这类巧合，只要有人删掉第二次调用就不再无害。
  var LINK_DIST = 0, LINK_DIST2 = 0;
  function resize() {
    dpr = Math.min(2, window.devicePixelRatio || 1);
    w = window.innerWidth; h = window.innerHeight;
    canvas.width = Math.floor(w * dpr);
    canvas.height = Math.floor(h * dpr);
    canvas.style.width = w + 'px';
    canvas.style.height = h + 'px';
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    recomputeLinkDist();
  }
  resize();

  // Colours are read from the stylesheet so the field belongs to whichever theme is active.
  // 颜色取自样式表，使这片场属于当前生效的那个主题。
  var css = getComputedStyle(document.documentElement);
  var LINK_COLOR = css.getPropertyValue('--dot').trim() || '#9E9D9B';
  var DOT_COLOR = css.getPropertyValue('--foreground').trim() || '#2c2c2c';

  // The link distance has to follow the particle count, not sit at a fixed number.
  //
  // The config this was modelled on used a fixed 100px for 200 particles. With 112 particles on a
  // 1440×950 viewport the mean nearest-neighbour distance is about 110px — wider than the range —
  // so almost no links were drawn and the field looked like scattered dust instead of a network.
  // Pinning the number either way breaks one end: 165px looks right at 112 and turns into a solid
  // mesh at 400.
  //
  // So it is derived. The mean spacing on a Poisson field of n points in area A is about
  // sqrt(A/n); links become visible at roughly 1.5× that. Clamped so a one-note blog still draws
  // something and a huge one does not become a hairball.
  //
  // 连线距离必须跟着粒子数走，不能钉在固定值上。
  //
  // 作为参照的那份配置固定 100px、按 200 个粒子调的。而 112 个粒子铺在 1440×950 上，
  // 平均最近邻距离约 110px —— 比范围还大，于是几乎画不出连线，
  // 这片场看起来像散落的灰尘而不是一张网。把数字钉死在任何一个值上都会坏掉一端：
  // 165px 在 112 个粒子时刚好，到 400 个就成了一张实心网。
  //
  // 所以它由数据算出来。泊松场中 n 个点铺在面积 A 上的平均间距约为 sqrt(A/n)，
  // 连线在约 1.5 倍处开始成网。两端做了夹取，使一篇笔记的博客仍然画得出东西，
  // 而一个很大的博客不会糊成一团。
  var GRAB_DIST = 140, GRAB_DIST2 = GRAB_DIST * GRAB_DIST;
  var GRAVITY = 0.5, DRIFT = -0.1;

  function recomputeLinkDist() {
    LINK_DIST = Math.max(90, Math.min(190,
      Math.sqrt((window.innerWidth * window.innerHeight) / COUNT) * 1.5));
    LINK_DIST2 = LINK_DIST * LINK_DIST;
  }
  recomputeLinkDist();

  var ps = [];
  function spawn(fromEdge) {
    // Polygons rather than dots: a small triangle reads as a shape at 2px, a dot reads as noise.
    // 多边形而不是圆点：2px 的三角形还能读成一个形状，而圆点只能读成噪点。
    return {
      x: Math.random() * w,
      y: fromEdge ? -8 : Math.random() * h,
      vx: (Math.random() < 0.5 ? -1 : 1) * (1 + Math.random()),
      vy: (Math.random() < 0.5 ? -1 : 1) * (1 + Math.random()),
      size: 1 + Math.random() * 3,
      sides: Math.random() < 0.55 ? 3 : 4,
      angle: Math.random() * Math.PI * 2,
      spin: (Math.random() - 0.5) * 0.012,
      // Each particle blinks on its own clock; syncing them would read as a stutter.
      // 每个粒子按自己的时钟闪烁；同步会让它读成卡顿。
      alpha: 0.6,
      blink: Math.random() * Math.PI * 2,
      blinkRate: 0.012 + Math.random() * 0.018
    };
  }
  // Jittered grid rather than uniform random placement.
  //
  // Uniform random leaves clumps and bald patches — at 112 points that is visible, and it reads as
  // randomness rather than as a field. A grid cell per particle with a jitter of under one cell
  // keeps the spacing even while still looking unplanned, which is what makes the link network
  // read as a network: links appear where density is even, and vanish into the gaps where it is
  // not.
  //
  // The grid is shaped to the viewport rather than square, so a wide window does not get columns
  // of particles packed against the sides.
  //
  // 抖动网格，而不是完全随机撒点。
  //
  // 完全随机会留下团块与空斑 —— 112 个点时这看得出来，而它读起来是「随机」而不是「一片场」。
  // 每个粒子占一个格子、抖动小于一格，间距因而均匀，同时仍然像是未经安排的；
  // 而这正是让连线网络读成一张网的原因：密度均匀处连成线，不均匀处就断开成空隙。
  //
  // 网格按视口形状而不是正方形排布，使宽窗口不会把粒子挤在两侧成列。
  var cols = Math.max(1, Math.round(Math.sqrt(COUNT * w / h)));
  var rows = Math.max(1, Math.ceil(COUNT / cols));
  var made = 0;
  for (var r = 0; r < rows && made < COUNT; r++) {
    for (var c = 0; c < cols && made < COUNT; c++) {
      var q = spawn(false);
      q.x = (c + 0.5 + (Math.random() - 0.5) * 0.72) * (w / cols);
      q.y = (r + 0.5 + (Math.random() - 0.5) * 0.72) * (h / rows);
      ps.push(q);
      made++;
    }
  }

  // Pointer state. Tracked on window because the canvas is pointer-events:none — if it were not,
  // every card and link on the page would become unclickable.
  //
  // 指针状态。在 window 上跟踪，因为 canvas 是 pointer-events:none ——
  // 若不是，页面上每一张卡片和每一个链接都会点不到。
  var mouse = { x: -9999, y: -9999, inside: false };
  window.addEventListener('mousemove', function (e) {
    mouse.x = e.clientX; mouse.y = e.clientY; mouse.inside = true;
  }, { passive: true });
  window.addEventListener('mouseleave', function () { mouse.inside = false; });
  window.addEventListener('click', function (e) {
    // A click pushes nearby particles away. Clicks on interactive elements are left alone:
    // shoving the field because someone opened an article is noise, not feedback.
    //
    // 点击把附近的粒子推开。落在可交互元素上的点击不动：因为有人打开了文章而把这片场推一下，
    // 那是噪声，不是反馈。
    if (e.target && e.target.closest && e.target.closest('a,button,input,textarea,select')) return;
    for (var i = 0; i < ps.length; i++) {
      var p = ps[i];
      var dx = p.x - e.clientX, dy = p.y - e.clientY;
      var d2 = dx * dx + dy * dy;
      if (d2 < 22500 && d2 > 0.01) {  // within 150px
        var d = Math.sqrt(d2);
        p.vx += (dx / d) * 2.6;
        p.vy += (dy / d) * 2.6;
      }
    }
  }, { passive: true });

  // ── Drawing ────────────────────────────────────────────────────────────────────────────
  // Split from the physics so the reduced-motion path can draw exactly one frame of the same
  // picture. Duplicating the draw code for that path is how the two would end up looking
  // different — the frozen frame is supposed to be a still of the animation, not a second design.
  //
  // ── 绘制 ────────────────────────────────────────────────────────────────────────────────
  // 与物理分开，使减少动效的路径能画出同一张图的一帧。为那条路径复制一份绘制代码，
  // 正是两者最终长得不一样的原因 —— 冻结帧应当是动画的一张静帧，而不是第二个设计。
  function drawFrame() {
    ctx.clearRect(0, 0, w, h);

    // Links first, so a node is never hidden by its own edges.
    // 先画连线，使一个节点永远不会被它自己的边遮住。
    ctx.lineWidth = 1;
    ctx.strokeStyle = LINK_COLOR;
    for (var a = 0; a < ps.length; a++) {
      var pa = ps[a];
      for (var b = a + 1; b < ps.length; b++) {
        var pb = ps[b];
        var dx2 = pa.x - pb.x, dy2 = pa.y - pb.y;
        var dd2 = dx2 * dx2 + dy2 * dy2;
        if (dd2 > LINK_DIST2) continue;
        // Fade with distance so a link dissolves instead of ending at a hard radius.
        // 随距离淡出，使连线消散而不是在一个硬半径上突然断掉。
        ctx.globalAlpha = 0.42 * (1 - Math.sqrt(dd2) / LINK_DIST);
        ctx.beginPath();
        ctx.moveTo(pa.x, pa.y);
        ctx.lineTo(pb.x, pb.y);
        ctx.stroke();
      }
      // The grab line to the pointer, at a higher opacity as the config asked.
      // 到指针的抓取线，按配置使用更高的不透明度。
      if (mouse.inside) {
        var mx = pa.x - mouse.x, my = pa.y - mouse.y;
        var md2 = mx * mx + my * my;
        if (md2 < GRAB_DIST2) {
          ctx.globalAlpha = 0.6 * (1 - Math.sqrt(md2) / GRAB_DIST);
          ctx.beginPath();
          ctx.moveTo(pa.x, pa.y);
          ctx.lineTo(mouse.x, mouse.y);
          ctx.stroke();
        }
      }
    }

    for (var k = 0; k < ps.length; k++) {
      var q = ps[k];
      ctx.globalAlpha = q.alpha;
      ctx.fillStyle = DOT_COLOR;
      ctx.beginPath();
      for (var n = 0; n < q.sides; n++) {
        var ang = q.angle + (n * Math.PI * 2) / q.sides;
        var px = q.x + Math.cos(ang) * q.size;
        var py = q.y + Math.sin(ang) * q.size;
        if (n === 0) ctx.moveTo(px, py); else ctx.lineTo(px, py);
      }
      ctx.closePath();
      ctx.fill();
    }
    ctx.globalAlpha = 1;
  }

  function step() {
    for (var i = 0; i < ps.length; i++) {
      var p = ps[i];
      // Gravity pulls down, drift nudges sideways: together they keep the field from settling
      // into the same slow fall, which is what reads as "a screensaver".
      // 重力向下、漂移横向：两者一起使这片场不会沉进同一种缓慢下落，而那种下落读起来像屏保。
      p.vy += GRAVITY * 0.01;
      p.vx += DRIFT * 0.01;

      // Grab: a particle near the pointer is pulled toward it gently.
      // 抓取：靠近指针的粒子被轻微拉向它。
      if (mouse.inside) {
        var gx = mouse.x - p.x, gy = mouse.y - p.y;
        var gd2 = gx * gx + gy * gy;
        if (gd2 < GRAB_DIST2 && gd2 > 1) {
          var gd = Math.sqrt(gd2);
          p.vx += (gx / gd) * 0.06;
          p.vy += (gy / gd) * 0.06;
        }
      }

      p.x += p.vx;
      p.y += p.vy;
      p.angle += p.spin;
      p.blink += p.blinkRate;

      // Bounce, as the config asked. A particle that leaves the screen and respawns is a jump cut;
      // bouncing keeps the count constant, which matters because the count is the point.
      //
      // 按配置弹回。飞出屏幕再重生是一次跳切；弹回保持数量恒定，而数量正是这个效果的重点。
      if (p.x < 0) { p.x = 0; p.vx = Math.abs(p.vx); }
      if (p.x > w) { p.x = w; p.vx = -Math.abs(p.vx); }
      if (p.y < 0) { p.y = 0; p.vy = Math.abs(p.vy); }
      if (p.y > h) { p.y = h; p.vy = -Math.abs(p.vy); }

      // Keep speeds bounded; gravity accumulates and would eventually slam everything into the
      // bottom edge.
      // 限制速度范围；重力会累积，最终把一切都砸到底边。
      var sp2 = p.vx * p.vx + p.vy * p.vy;
      if (sp2 > 9) { var s = 3 / Math.sqrt(sp2); p.vx *= s; p.vy *= s; }

      // 0.5–0.78 rather than 0.45–0.7: the reference config's 0.6 sat on a white page, while this
      // one sits on warm grey behind opaque cards, so the same value reads fainter here.
      // 取 0.5–0.78 而不是 0.45–0.7：参照配置的 0.6 是在纯白页面上，而这里是暖灰纸面、
      // 且上面盖着不透明的卡片，同样的值在这里读起来更淡。
      p.alpha = 0.5 + 0.28 * Math.sin(p.blink);
    }

    drawFrame();
    requestAnimationFrame(step);
  }

  window.addEventListener('resize', resize, { passive: true });

  if (STILL) {
    // Reduced motion: draw one frame and stop.
    //
    // The field is decoration, but a single static frame still shows how much is in the blog,
    // which is the part that carries information. So it is frozen rather than removed — the
    // author still gets the picture, the reader is not asked to watch it move.
    //
    // 减少动效：画一帧然后停下。
    //
    // 这片场是装饰，但一帧静态画面仍然显示出博客里有多少东西，而那才是有信息量的部分。
    // 所以它被冻结而不是被移除：作者仍然得到那张图，读者不被要求盯着它动。
    drawFrame();
    return;
  }

  // The cap is reported once, in the console, so an author who wonders why the field stopped
  // growing has an answer without reading source.
  // 上限只报告一次，输出到控制台，使好奇「为什么这片场不再增长」的作者不必读源码就有答案。
  if (capped) {
    try {
      console.info('[blog] 粒子数已封顶在 ' + MAX + '（已分享笔记 ' + COUNT + ' 篇以上）。连线计算是 O(n²)，不封顶会让列表滚动变卡。');
    } catch (e) {}
  }

  requestAnimationFrame(step);
})();
`

// blogParticlesJS returns the ambient field script.
// blogParticlesJS 返回环境粒子场的脚本。
func blogParticlesJS() string {
	return blogParticlesScript
}
