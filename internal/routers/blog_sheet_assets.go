package routers

// Bottom-sheet dragging for the five narrow-screen panels.
//
// Served from /blog/sheet.js so the three blog templates share one implementation instead of
// three copies of the same pointer maths.
//
// 窄屏五个面板的底部 sheet 拖拽。
//
// 由 /blog/sheet.js 提供，三个博客模板共用一份实现，避免同一套指针运算写三遍。
//
// Why the drag lives here rather than in each page's inline script: the panels are rendered by
// the server into three different layouts, but the interaction is identical. One file means one
// place to fix when the maths is wrong — and it is the kind of maths that is wrong the first time.
//
// 为什么拖拽放在这里而不是各页内联脚本：面板由服务端渲染进三种不同布局，但交互完全相同。
// 一份文件意味着算错时只有一个地方要改 —— 而这正是那种第一次写就会算错的东西。
const blogSheetScript = `
(function () {
  // The three stops the reader can land on. 40% is the opening height: enough to read the first
  // few rows and decide whether to keep pulling, without covering the article.
  //
  // 读者可以停靠的三档。40% 是打开高度：够看清头几行、决定要不要继续往上拉，
  // 又不会把正文整块盖住。
  var STOPS = [0.40, 0.75, 0.92];
  var MIN = 0.28;          // 拉到比这更低就收起来 // 低于这个比例视为"收起"
  var DRAG_ZONE = 56;      // 顶部这一段是抓手热区（与 ::before 对齐）// 顶部抓手热区

  var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // Panels differ per page, so they are looked up lazily on every gesture rather than cached:
  // a page can have .aside only after the nav template renders, and the graph panel mounts later.
  //
  // 各页面板不同，因此每次手势开始时才查找而不是缓存：
  // 某些页面的 .aside 要等 nav 模板渲染后才存在，图谱面板更晚挂载。
  function activeSheet() {
    var body = document.body;
    if (body.classList.contains('drawer-nav')) {
      var a = document.querySelector('.aside');
      if (a) return a;
    }
    if (body.classList.contains('drawer-toc') ||
        body.classList.contains('drawer-hot') ||
        body.classList.contains('drawer-live')) {
      var r = document.querySelector('.rail');
      if (r) return r;
    }
    var g = document.querySelector('#graph-root.gr-on .graph-panel');
    if (g) return g;
    return null;
  }

  // Only the tall panels are draggable. 热门/活跃 deliberately are not: their height follows
  // their content, and pulling them would do nothing, which reads as a broken gesture rather
  // than a constrained one.
  //
  // 只有高面板可拖。热门/活跃刻意不可拖：它们的高度跟着内容走，拉也不会有变化，
  // 那种手感读起来像故障而不是限制。
  function draggable(el) {
    if (!el) return false;
    return el.classList.contains('aside') || el.classList.contains('rail') || el.classList.contains('graph-panel');
  }

  function currentVFrac(el) {
    return el.getBoundingClientRect().height / window.innerHeight;
  }

  function setFrac(el, f) {
    el.style.setProperty('--sheet-h', (f * 100).toFixed(2) + 'vh');
  }

  function nearestStop(f) {
    var best = STOPS[0], bestD = Infinity;
    for (var i = 0; i < STOPS.length; i++) {
      var d = Math.abs(STOPS[i] - f);
      if (d < bestD) { bestD = d; best = STOPS[i]; }
    }
    return best;
  }

  var drag = null;

  function onDown(ev) {
    if (ev.button !== 0 && ev.pointerType === 'mouse') return;
    var el = activeSheet();
    if (!draggable(el)) return;

    var rect = el.getBoundingClientRect();
    // Only the top strip drags. Making the whole panel draggable would fight the content's own
    // scrolling: the reader would try to scroll a long nav tree and instead resize the sheet.
    //
    // 只有顶部一条可拖。整块可拖会和内容滚动打架：读者想滚长目录，结果把面板拉了。
    if (ev.clientY - rect.top > DRAG_ZONE) return;

    drag = {
      el: el,
      startY: ev.clientY,
      startH: rect.height,
      moved: false
    };
    el.classList.add('is-dragging');
    el.classList.remove('is-snapping');
    // Pointer capture keeps the gesture alive even when the finger leaves the strip.
    // 指针捕获让手势在手指离开那条热区后仍然有效。
    try { el.setPointerCapture(ev.pointerId); } catch (e) {}
  }

  function onMove(ev) {
    if (!drag) return;
    var dy = drag.startY - ev.clientY;          // 向上为正 // up is positive
    if (Math.abs(dy) > 3) drag.moved = true;
    var next = drag.startH + dy;
    var vh = window.innerHeight;
    // Clamp so the sheet cannot be pushed below the bottom edge or above the viewport.
    // 夹住上下界：既不能被推到底边以下，也不能高过视口。
    var maxH = vh * 0.96;
    if (next < 60) next = 60;
    if (next > maxH) next = maxH;
    drag.el.style.setProperty('--sheet-h', ((next / vh) * 100).toFixed(2) + 'vh');
    if (drag.moved) ev.preventDefault();
  }

  function onUp() {
    if (!drag) return;
    var el = drag.el;
    var frac = currentVFrac(el);
    el.classList.remove('is-dragging');

    // Below the floor the gesture means "put it away", not "make it 28% tall".
    // 低于下限时这个手势的意思是"收起来"，而不是"变成 28% 高"。
    if (frac < MIN) {
      drag = null;
      // Reuse the page's own close path where it exists, so aria and scrim stay in sync.
      // 有现成关闭路径的复用它，使 aria 与遮罩状态保持一致。
      var dock = document.getElementById('dock-nav') || document.getElementById('dock-toc');
      if (el.classList.contains('graph-panel')) {
        var fab = document.getElementById('graph-fab');
        if (fab) { fab.click(); }
        else { el.style.removeProperty('--sheet-h'); }
      } else if (dock) {
        dock.click();
      } else {
        el.style.removeProperty('--sheet-h');
      }
      return;
    }

    var snapped = nearestStop(frac);
    el.classList.add('is-snapping');
    setFrac(el, snapped);
    window.setTimeout(function () { el.classList.remove('is-snapping'); }, 320);
    drag = null;
  }

  // Open at 40% every time the sheet is opened fresh. A remembered height sounds nice but in
  // practice the reader wants the same predictable starting point: a sheet that reopens at
  // whatever height it was left at feels like it is guessing.
  //
  // 每次打开都回到 40%。记住高度听起来贴心，实际读者要的是可预期的起点：
  // 一个在"上次停在的高度"重新弹出的面板，读起来像在猜。
  function resetOnOpen() {
    var el = activeSheet();
    if (!draggable(el)) return;
    if (el.classList.contains('graph-panel')) return;   // 图谱自己按 92% 打开 // graph opens tall
    if (el.classList.contains('rail') && (document.body.classList.contains('drawer-hot') ||
        document.body.classList.contains('drawer-live'))) return;   // 短面板不设高度
    setFrac(el, STOPS[0]);
  }

  document.addEventListener('pointerdown', onDown, { passive: true });
  document.addEventListener('pointermove', onMove, { passive: false });
  document.addEventListener('pointerup', onUp, { passive: true });
  document.addEventListener('pointercancel', onUp, { passive: true });

  // The drawer classes are toggled by each page's own script; watch them so every open starts
  // from the same place without those scripts having to know about this file.
  //
  // drawer 类由各页脚本自己切换；这里观察它们，使每次打开都从同一位置开始，
  // 而那些脚本不必知道本文件存在。
  var lastSig = '';
  function watch() {
    var body = document.body;
    var sig = [body.classList.contains('drawer-nav'), body.classList.contains('drawer-toc'),
               body.classList.contains('drawer-hot'), body.classList.contains('drawer-live')].join('');
    if (sig !== lastSig) {
      lastSig = sig;
      if (!reduce) { resetOnOpen(); }
    }
    window.requestAnimationFrame(watch);
  }
  window.requestAnimationFrame(watch);

  // ── 目录树的展开/收起动画 ──────────────────────────────────────────────────
  //
  // <details> 的开关是瞬时的，而目录树是读者最爱反复点的地方 —— 瞬移会让整棵树跳一下，
  // 当前位置随之丢失。这里接管点击，量出内容高度，手动做一次高度过渡。
  //
  // 用委托而不是给每个 summary 挂监听：目录树由服务端递归渲染，深度不定，
  // 而且读者展开某个文件夹时它里面的子文件夹此刻才出现。
  //
  // ── Animated folder toggle ───────────────────────────────────────────────
  //
  // <details> opens instantly, and the tree is the control readers click most. Taking over the
  // click and measuring the content is what makes the height animatable without knowing it ahead
  // of time. Delegated because the tree is rendered recursively and children appear on expand.
  var FOLD_MS = 300;

  function navKids(det) {
    for (var i = 0; i < det.children.length; i++) {
      if (det.children[i].classList && det.children[i].classList.contains('nav-children')) {
        return det.children[i];
      }
    }
    return null;
  }

  function openFolder(det) {
    var kids = navKids(det);
    if (!kids) { det.open = true; return; }
    // 先定 0、再量目标、再给具体值 —— 量之前必须让内容可见，所以 open 要在量之前设。
    kids.style.height = '0px';
    det.open = true;
    var target = kids.scrollHeight;
    // 强制一次重排，否则浏览器把 0 → target 合并成一步，动画不会被绘制。
    void kids.offsetHeight;
    kids.style.height = target + 'px';
    window.setTimeout(function () {
      // 收起状态必须清掉这个死高度，否则内容继续增长时会被裁掉。
      if (det.open) { kids.style.height = ''; }
    }, FOLD_MS);
  }

  function closeFolder(det) {
    var kids = navKids(det);
    if (!kids) { det.open = false; return; }
    var target = kids.scrollHeight;
    kids.style.height = target + 'px';
    void kids.offsetHeight;
    kids.style.height = '0px';
    window.setTimeout(function () {
      det.open = false;
      kids.style.height = '';
    }, FOLD_MS);
  }

  document.addEventListener('click', function (ev) {
    if (!ev.target || !ev.target.closest) return;
    var head = ev.target.closest('.nav-folder > .nav-head');
    if (!head) return;
    var det = head.parentElement;
    if (!det || det.tagName.toLowerCase() !== 'details') return;
    if (reduce) return;             // 减少动效：交给浏览器原生开关
    ev.preventDefault();
    var kids = navKids(det);
    // 连点保护：动画途中再点，忽略。否则两段高度过渡会互相覆盖。
    if (kids && kids.dataset.animating === '1') return;
    if (kids) {
      kids.dataset.animating = '1';
      window.setTimeout(function () { kids.dataset.animating = ''; }, FOLD_MS);
    }
    if (det.open) { closeFolder(det); } else { openFolder(det); }
  });

  // Keyboard changes the visual viewport on mobile. Re-anchor so the sheet does not end up
  // taller than what is actually visible.
  // 键盘改变移动端视觉视口；重新锚定，避免面板比实际可见区还高。
  if (window.visualViewport) {
    window.visualViewport.addEventListener('resize', function () {
      var el = activeSheet();
      if (!draggable(el)) return;
      if (el.classList.contains('rail') && (document.body.classList.contains('drawer-hot') ||
          document.body.classList.contains('drawer-live'))) return;
      var frac = currentVFrac(el);
      if (frac > 0.92) setFrac(el, 0.92);
    });
  }
})();
`
