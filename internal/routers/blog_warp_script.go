package routers

// The warp transition: the home page's "about me" entry point dissolves the page into a starfield
// and the résumé fades in on the other side.
// ── Why a cross-document transition and not a client-side swap ───────────────────────────
//
// Intercepting the click, fetching /blog/about and swapping the DOM would make the transition
// seamless with no help from the other page. It would also mean the browser's back button,
// history, the address bar and a fresh reader's direct visit to /blog/about all take a different
// code path than the one this animation exercises — and the path nobody exercises is the one that
// breaks.
//
// So the animation plays first, then the browser really navigates. The destination page reads a
// one-shot flag out of sessionStorage and fades its content in, which is what makes the two halves
// read as one movement rather than two.
//
// ── 为什么是跨文档过渡，而不是客户端换页 ────────────────────────────────────────────────
//
// 拦下点击、取回 /blog/about、替换 DOM，可以让过渡不依赖目标页就做到无缝。
// 但那也意味着浏览器的后退键、历史、地址栏，以及新读者直接访问 /blog/about，
// 走的都是与这段动画不同的另一条代码路径 —— 而没人走的路径正是会坏的那条。
//
// 所以先播动画，再让浏览器真的导航。目标页从 sessionStorage 里读一个一次性标记并让内容渐显，
// 这才使前后两半读起来是一次运动，而不是两次。
//
// ── Starfield ──────────────────────────────────────────────────────────────────────────
//
// The projection is the classic one: each star has a depth z, and z decreases so the stars rush
// past the viewer. Screen position is x·(focal/z), which is why a star near the eye flies off the
// edge and a distant one barely moves — that divergence is the whole sensation.
//
// Two changes to the version that was handed to me:
//
//  1. The stars accelerate. A constant z step reads as drifting; a step that grows reads as
//     falling into something.
//  2. The background is derived from the site's own ink rather than pure black, so the transition
//     belongs to the page it starts from instead of being a hard cut to another visual world.
//
// ── 星空 ──────────────────────────────────────────────────────────────────────────────
//
// 透视是经典那一种：每颗星有一个深度 z，z 递减，于是星星从观察者身边冲过。
// 屏幕坐标是 x·(焦距/z)，这就是为什么近处的星会飞出边缘而远处的几乎不动 —— 这个发散感就是全部。
//
// 相对交到我手上的版本有两处改动：
//
//  1. 星星会加速。恒定的 z 步长读起来是漂移，逐步变大的步长读起来是坠入。
//  2. 背景取自站点自己的墨色而不是纯黑，使这段过渡属于它出发的那个页面，
//     而不是一次硬切到另一个视觉世界。
const blogWarpScript = `
(function () {
  // The destination reads this to know it should fade in. sessionStorage rather than localStorage
  // because the fade belongs to this one navigation, not to every future visit.
  // 目标页读它来决定是否渐显。用 sessionStorage 而不是 localStorage，
  // 因为这次渐显属于这一次导航，而不是此后每一次访问。
  var FLAG = 'fns:warp';

  // ── The arriving half runs on every page ────────────────────────────────────────────────
  // This block comes first on purpose. It used to sit at the bottom, after the guard that returns
  // when the home page's entry point is missing — which meant it never ran on the résumé page, the
  // one page it exists for.
  //
  // ── 抵达的那一半在每个页面上运行 ────────────────────────────────────────────────────────
  // 这一块刻意放在最前。它原本在末尾，位于「首页入口不存在就返回」的守卫之后 ——
  // 那意味着它在简历页上永远不会运行，而简历页正是它唯一存在的理由。
  var arrived = null;
  try { arrived = sessionStorage.getItem(FLAG); } catch (e) {}
  if (arrived) {
    try { sessionStorage.removeItem(FLAG); } catch (e) {}
    document.documentElement.classList.add('fns-arrived');
    // Removed after the fade so it does not sit on the document forever.
    // 淡入结束后移除，免得它永远留在文档上。
    window.setTimeout(function () {
      document.documentElement.classList.remove('fns-arrived');
    }, 900);
  }

  // ── The departing half needs the home page's entry point ────────────────────────────────
  // ── 出发的那一半需要首页的入口 ──────────────────────────────────────────────────────────
  var link = document.getElementById('fns-warp');
  if (!link) return;

  var STILL = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);

  link.addEventListener('click', function (ev) {
    // Only the plain left click. Ctrl/cmd/middle click mean "open elsewhere", and hijacking those
    // is how a link stops behaving like a link.
    // 只接管普通的左键点击。Ctrl/⌘/中键表示「在别处打开」，抢走它们就是一个链接不再像链接的方式。
    if (ev.defaultPrevented || ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;
    if (STILL) return;   // Reduced motion: let the browser navigate plainly. // 减少动效：交给浏览器普通导航。

    ev.preventDefault();
    try { sessionStorage.setItem(FLAG, '1'); } catch (e) {}

    var w = window.innerWidth, h = window.innerHeight;
    var canvas = document.createElement('canvas');
    canvas.className = 'fns-warp';
    canvas.setAttribute('aria-hidden', 'true');
    canvas.width = w; canvas.height = h;
    canvas.style.width = w + 'px'; canvas.style.height = h + 'px';
    document.body.appendChild(canvas);

    // Fade the page out under the starfield so the two are one movement.
    // 让页面在星空之下淡出，使两者成为一次运动。
    document.documentElement.classList.add('fns-warping');

    var ctx = canvas.getContext('2d');
    // Ink colour from the stylesheet, so the transition matches whichever theme is active.
    // 从样式表取墨色，使这段过渡匹配当前主题。
    var ink = getComputedStyle(document.documentElement).getPropertyValue('--foreground').trim() || '#2c2c2c';
    var paper = getComputedStyle(document.body).backgroundColor || '#eae9e3';

    var focal = 128;
    var stars = [];
    for (var i = 0; i < 300; i++) stars.push({
      x: (Math.random() - 0.5) * w,
      y: (Math.random() - 0.5) * h,
      z: Math.random() * w,
      // A little colour spread keeps it from looking like dust on the lens.
      // 一点色散使它不至于像镜头上的灰。
      hue: Math.random()
    });

    var step = 4, t0 = null, DURATION = 1150;
    function tick(ts) {
      if (t0 === null) t0 = ts;
      var p = Math.min(1, (ts - t0) / DURATION);
      // Accelerating: 4 px per frame at the start, about 7× that at the end.
      // 加速：起始每帧 4px，结束时约为它的 7 倍。
      step = 4 + Math.pow(p, 2.4) * 26;

      ctx.fillStyle = paper;
      ctx.fillRect(0, 0, w, h);

      for (var i = 0; i < stars.length; i++) {
        var s = stars[i];
        s.z -= step;
        if (s.z <= 0.5) { s.z = w; s.x = (Math.random() - 0.5) * w; s.y = (Math.random() - 0.5) * h; continue; }
        var k = focal / s.z;
        var x = s.x * k + w / 2;
        var y = s.y * k + h / 2;
        if (x < -20 || x > w + 20 || y < -20 || y > h + 20) continue;
        // The near-star trail: draw a short streak back along the direction of travel instead of
        // a dot. A dot at high speed reads as a static speck; the streak is what the eye reads
        // as motion.
        // 近星拖尾：沿运动方向的反向画一小段，而不是一个点。
        // 高速下的点会读成静止的斑点，而拖尾才是眼睛读成移动的东西。
        var len = Math.min(46, (1 - s.z / w) * 46);
        ctx.globalAlpha = Math.min(1, (1 - s.z / w) * 1.6);
        ctx.strokeStyle = s.hue < 0.18 ? '#c2410c' : (s.hue < 0.30 ? '#8e4ec6' : ink);
        ctx.lineWidth = Math.min(2.2, 0.5 + len * 0.05);
        ctx.beginPath();
        ctx.moveTo(x, y);
        ctx.lineTo(x + (x - w / 2) * -0.045 * (len / 22), y + (y - h / 2) * -0.045 * (len / 22));
        ctx.stroke();
      }
      ctx.globalAlpha = 1;

      if (p < 1) {
        requestAnimationFrame(tick);
      } else {
        location.href = link.getAttribute('href');
      }
    }
    requestAnimationFrame(tick);
  });
})();
`

// blogWarpJS returns the warp script.
//
// Served on every blog page rather than only the two that use it: the departing half is inert
// without the home page's entry point, and the arriving half has to be present on whichever page a
// reader lands on after the warp. Splitting it into two files by page would mean keeping two
// halves of one behaviour in sync across three templates.
//
// blogWarpJS 返回穿越脚本。
//
// 它在每个博客页面上提供，而不是只在用到它的那两个上：出发的那一半在没有首页入口时是惰性的，
// 而抵达的那一半必须出现在读者穿越后落到的任何页面上。按页面拆成两个文件，
// 意味着把同一个行为的两半在三个模板之间保持同步。
func blogWarpJS() string {
	return blogWarpScript
}
