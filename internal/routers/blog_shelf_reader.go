package routers

// The reader is deliberately its own file rather than inlined the way the lightbox is. It only
// exists on two routes, and a 9 KB script on every article to serve two pages would be a tax on
// the reading pages that never use it.
//
// 阅读器刻意独立成一个文件，而不是像灯箱那样内联。它只出现在两条路由上，
// 为两个页面让每篇文章都背 9 KB 脚本，等于向从不使用它的阅读页收税。
//
// Three decisions worth knowing before editing this:
//
// 1. Two <img> elements, not one. A single element cannot cross-fade itself; swapping src shows
//    the old frame until the new one decodes, which reads as a flicker rather than a turn.
// 2. Zoom uses the CSS `zoom` property on the stage, not `transform: scale()`. A transform does
//    not change layout size, so a scaled sheet overflows a scroll container that believes its
//    content still fits — the far edges become unreachable. `zoom` reflows, and native scrolling
//    keeps working at every level.
// 3. Every page is fetched on demand and nothing else is. Preloading is limited to the immediate
//    neighbours; that limit is the traffic contract, not an optimisation.
//
// 动这个文件之前有三件事值得知道：
//
// 1. 用两个 <img>，不是一个。单个元素无法与自己做交叉淡切；换 src 会先显示旧帧直到新帧解码，
//    读起来是闪一下而不是翻过去。
// 2. 缩放用舞台上的 CSS `zoom` 属性，不用 `transform: scale()`。transform 不改变布局尺寸，
//    于是放大后的纸面会溢出到一个仍以为自己装得下的滚动容器里——两个远端变得不可达。
//    `zoom` 会重排，原生滚动在任何倍率下都继续工作。
// 3. 每一页都按需取，别的都不取。预加载只限左右各一页；这个限制是流量契约，不是优化。
const blogReaderJS = `
(function () {
  'use strict';

  var root = document.getElementById('fns-reader');
  if (!root) return;

  var book = root.getAttribute('data-book') || '';
  var total = parseInt(root.getAttribute('data-pages'), 10) || 0;
  if (!book || total < 1) return;

  var stack = document.getElementById('fns-reader-stack');
  var stage = document.getElementById('fns-reader-stage');
  var sheetA = document.getElementById('fns-sheet-a');
  var sheetB = document.getElementById('fns-sheet-b');
  var input = document.getElementById('fns-reader-input');
  var rail = document.getElementById('fns-reader-rail');
  var hintEl = document.getElementById('fns-reader-hint');
  var zoomLabel = document.getElementById('fns-reader-zoom');
  var backBtn = document.getElementById('fns-reader-back');
  var arrowPrev = document.getElementById('fns-arrow-prev');
  var arrowNext = document.getElementById('fns-arrow-next');
  var track = document.getElementById('fns-reader-track');
  var prevBtn = document.getElementById('fns-reader-prev');
  var nextBtn = document.getElementById('fns-reader-next');
  var zoomInBtn = document.getElementById('fns-reader-zoom-in');
  var zoomOutBtn = document.getElementById('fns-reader-zoom-out');
  var fsBtn = document.getElementById('fns-reader-fs');

  var STILL = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  var KEY = 'fns:read:' + book;
  var TURN_MS = 340;

  var sheets = [sheetA, sheetB];
  var front = 0;
  var page = 1;
  var busy = false;
  var zoom = 1;
  var hintTimer = 0;
  var touchX = 0;
  var touchY = 0;
  var touching = false;

  function pageURL(n) { return '/blog/shelfimg/' + book + '/' + n; }
  function clamp(n) { return n < 1 ? 1 : (n > total ? total : n); }
  function pct() { return Math.round(page / total * 100) + '%'; }

  function stored() {
    try {
      var n = parseInt(localStorage.getItem(KEY), 10);
      return n > 0 ? n : 0;
    } catch (e) { return 0; }
  }

  function initialPage() {
    var m = /(?:^|[#&])p=(\d+)/.exec(location.hash || '');
    if (m) return clamp(parseInt(m[1], 10));
    var s = stored();
    if (s) return clamp(s);
    return 1;
  }

  function toast(msg) {
    if (!hintEl) return;
    hintEl.textContent = msg;
    hintEl.hidden = false;
    window.clearTimeout(hintTimer);
    hintTimer = window.setTimeout(function () { hintEl.hidden = true; }, 1600);
  }

  function syncUI() {
    if (input) input.value = String(page);
    if (rail) rail.style.width = pct();
    root.setAttribute('data-page', String(page));
    if (prevBtn) prevBtn.disabled = page <= 1;
    if (nextBtn) nextBtn.disabled = page >= total;
    if (arrowPrev) arrowPrev.disabled = page <= 1;
    if (arrowNext) arrowNext.disabled = page >= total;
    try { localStorage.setItem(KEY, String(page)); } catch (e) {}
    // replaceState, not assignment: turning pages must not fill the back button with forty entries.
    // 用 replaceState 而不是赋值：翻页不该在后退按钮里塞进四十条记录。
    try {
      if (history.replaceState) history.replaceState(null, '', '#p=' + page);
    } catch (e) {}
  }

  function preload() {
    var around = [page - 1, page + 1];
    for (var i = 0; i < around.length; i++) {
      var n = around[i];
      if (n >= 1 && n <= total) {
        var img = new Image();
        img.src = pageURL(n);
      }
    }
  }

  function go(target, dir) {
    target = clamp(target);
    if (busy || target === page) return;
    if (!dir) dir = target > page ? 1 : -1;

    var cur = sheets[front];
    var nxt = sheets[1 - front];

    busy = true;
    // The incoming sheet sits above: the page is pushed on from the side, it does not fade up
    // from underneath the one already there.
    // 新页在上层：页面是从侧面推进来的，而不是在旧页底下浮上来。
    nxt.style.zIndex = '3';
    cur.style.zIndex = '2';
    nxt.style.opacity = '0';
    nxt.style.transform = STILL ? 'none'
      : (dir > 0 ? 'translateX(14%) rotateY(14deg)' : 'translateX(-14%) rotateY(-14deg)');

    function finish() {
      stack.classList.remove('is-anim');
      cur.style.opacity = '0';
      cur.style.transform = 'none';
      cur.style.zIndex = '1';
      nxt.style.zIndex = '2';
      front = 1 - front;
      page = target;
      busy = false;
      syncUI();
      preload();
    }

    function begin() {
      if (STILL) {
        nxt.style.transform = 'none';
        nxt.style.opacity = '1';
        finish();
        return;
      }
      // Two frames: the first lets the browser settle the start position, the second turns the
      // transition on. One frame is not enough — the start and end would collapse into one style
      // calculation and nothing would animate.
      //
      // 两帧：第一帧让浏览器把起点样式解算掉，第二帧才打开过渡。
      // 只给一帧不够——起点与终点会被合成一次样式计算，什么都不会动。
      window.requestAnimationFrame(function () {
        window.requestAnimationFrame(function () {
          stack.classList.add('is-anim');
          nxt.style.transform = 'none';
          nxt.style.opacity = '1';
          cur.style.transform = dir > 0 ? 'translateX(-10%) rotateY(-10deg)' : 'translateX(10%) rotateY(10deg)';
          cur.style.opacity = '0';
          window.setTimeout(finish, TURN_MS + 40);
        });
      });
    }

    nxt.src = pageURL(target);
    if (nxt.complete && nxt.naturalWidth > 0) {
      begin();
    } else {
      nxt.onload = function () { nxt.onload = null; begin(); };
      nxt.onerror = function () {
        nxt.onerror = null;
        nxt.style.opacity = '0';
        nxt.style.transform = 'none';
        nxt.style.zIndex = '1';
        cur.style.zIndex = '2';
        busy = false;
        toast('第 ' + target + ' 页没能加载');
      };
    }
  }

  // jumpTo is for the progress bar's landing point: the page-turn animation fights itself when
  // you land repeatedly, so swap the image outright instead.
  //
  // 进度条落点用这个：连续跳转时翻页动画只会跟自己打架，直接换图才对。
  function jumpTo(target) {
    target = clamp(target);
    if (target === page) { syncUI(); return; }
    var cur = sheets[front];
    cur.src = pageURL(target);
    page = target;
    syncUI();
    preload();
  }

  function setZoom(z) {
    zoom = Math.min(3, Math.max(1, Math.round(z * 10) / 10));
    if (stage) stage.style.zoom = zoom === 1 ? '' : String(zoom);
    if (zoomLabel) zoomLabel.textContent = Math.round(zoom * 100) + '%';
    if (stage) stage.classList.toggle('is-zoomed', zoom > 1.01);
  }

  function close() {
    if (STILL) { location.href = '/blog/shelf'; return; }
    root.classList.add('is-closing');
    window.setTimeout(function () { location.href = '/blog/shelf'; }, 240);
  }

  // ── init ────────────────────────────────────────────────────────────────────────────────────
  sheetA.style.zIndex = '2';
  sheetA.style.opacity = '1';
  sheetB.style.zIndex = '1';
  sheetB.style.opacity = '0';

  page = initialPage();
  if (page !== 1) sheetA.src = pageURL(page);
  syncUI();
  preload();

  if (prevBtn) prevBtn.addEventListener('click', function () { go(page - 1, -1); });
  if (nextBtn) nextBtn.addEventListener('click', function () { go(page + 1, 1); });

  if (backBtn) backBtn.addEventListener('click', function (e) {
    e.preventDefault();
    close();
  });

  if (input) {
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') {
        var n = parseInt(input.value, 10);
        if (isNaN(n)) { input.value = String(page); return; }
        go(clamp(n), 0);
        input.blur();
      }
    });
    input.addEventListener('blur', function () { input.value = String(page); });
  }

  if (zoomInBtn) zoomInBtn.addEventListener('click', function () { setZoom(zoom + 0.25); });
  if (zoomOutBtn) zoomOutBtn.addEventListener('click', function () { setZoom(zoom - 0.25); });

  if (fsBtn) fsBtn.addEventListener('click', function () {
    if (document.fullscreenElement) {
      if (document.exitFullscreen) document.exitFullscreen();
    } else if (root.requestFullscreen) {
      root.requestFullscreen();
    }
  });

  if (stage) {
    stage.addEventListener('dblclick', function (e) {
      e.preventDefault();
      setZoom(zoom > 1.01 ? 1 : 2);
    });
    stage.addEventListener('wheel', function (e) {
      if (!e.ctrlKey && !e.metaKey) return;
      e.preventDefault();
      setZoom(zoom + (e.deltaY < 0 ? 0.2 : -0.2));
    }, { passive: false });

    stage.addEventListener('touchstart', function (e) {
      if (e.touches.length !== 1 || zoom > 1.01) { touching = false; return; }
      touching = true;
      touchX = e.touches[0].clientX;
      touchY = e.touches[0].clientY;
    }, { passive: true });

    stage.addEventListener('touchend', function (e) {
      if (!touching) return;
      touching = false;
      var t = e.changedTouches[0];
      var dx = t.clientX - touchX;
      var dy = t.clientY - touchY;
      // Horizontal only, and only when it clearly dominates: a slightly diagonal swipe is
      // someone scrolling the page, not turning it.
      // 只认水平方向，且必须明显占优：略微斜的划动是有人在滚页面，不是在翻页。
      if (Math.abs(dx) < 55 || Math.abs(dx) < Math.abs(dy)) return;
      go(page + (dx < 0 ? 1 : -1), dx < 0 ? 1 : -1);
    }, { passive: true });
  }

  // ── 桌面端箭头：鼠标一动浮上来，停 2.4 秒淡回去 ──────────────────────────
  var hideTimer = 0;
  var lastPoke = 0;

  function hideArrows() {
    if (arrowPrev) arrowPrev.classList.remove('is-on');
    if (arrowNext) arrowNext.classList.remove('is-on');
  }

  function showArrows() {
    if (!arrowPrev || !arrowNext) return;
    arrowPrev.classList.add('is-on');
    arrowNext.classList.add('is-on');
    window.clearTimeout(hideTimer);
    hideTimer = window.setTimeout(hideArrows, 2400);
  }

  // 箭头贴着纸边，不是贴着视口边：屏幕上纸只占中间一条，贴视口的箭头会离纸三四百像素，
  // 看着跟这本书没关系。位置随窗口尺寸重算。
  function placeArrows() {
    if (!arrowPrev || !arrowNext || !stack) return;
    var r = stack.getBoundingClientRect();
    var w = 56, gap = 10;
    var vw = window.innerWidth;
    arrowPrev.style.left = Math.max(6, r.left - w - gap) + 'px';
    arrowNext.style.left = Math.min(vw - w - 6, r.right + gap) + 'px';
  }

  if (arrowPrev && arrowNext) {
    placeArrows();
    window.addEventListener('resize', placeArrows);
    root.addEventListener('mousemove', function () {
      var now = Date.now();
      if (now - lastPoke < 180) return;
      lastPoke = now;
      showArrows();
    }, { passive: true });
    root.addEventListener('mouseleave', hideArrows);
    arrowPrev.addEventListener('click', function () { go(page - 1, -1); });
    arrowNext.addEventListener('click', function () { go(page + 1, 1); });
    showArrows();
  }

  // ── 进度条拖动 ──────────────────────────────────────────────────────────
  var dragging = false;

  function ratioAt(e) {
    var r = track.getBoundingClientRect();
    var cx = (e.touches && e.touches[0]) ? e.touches[0].clientX : e.clientX;
    var x = cx - r.left;
    return Math.min(1, Math.max(0, r.width ? x / r.width : 0));
  }

  function previewSeek(e) {
    var p = Math.max(1, Math.min(total, Math.round(ratioAt(e) * total)));
    if (input) input.value = String(p);
    if (rail) rail.style.width = Math.round(p / total * 100) + '%';
    return p;
  }

  if (track) {
    // Pointer Events 一套就同时覆盖鼠标、触摸和手写笔；touch-action:none 已在 CSS 里关掉
    // 浏览器自己的滚动手势，否则拖动会被页面滚动抢走。
    track.addEventListener('pointerdown', function (e) {
      dragging = true;
      track.classList.add('is-dragging');
      try { track.setPointerCapture(e.pointerId); } catch (err) {}
      previewSeek(e);
      e.preventDefault();
    });
    track.addEventListener('pointermove', function (e) {
      if (dragging) previewSeek(e);
    });
    track.addEventListener('pointerup', function (e) {
      if (!dragging) return;
      dragging = false;
      track.classList.remove('is-dragging');
      jumpTo(previewSeek(e));
    });
    track.addEventListener('pointercancel', function () {
      dragging = false;
      track.classList.remove('is-dragging');
      syncUI();
    });
  }

  document.addEventListener('keydown', function (e) {
    var tag = e.target && e.target.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA') {
      if (e.key === 'Escape' && e.target.blur) e.target.blur();
      return;
    }
    switch (e.key) {
      case 'ArrowLeft': case 'PageUp': go(page - 1, -1); break;
      case 'ArrowRight': case 'PageDown': go(page + 1, 1); break;
      case 'Home': go(1, -1); break;
      case 'End': go(total, 1); break;
      case 'Escape': close(); break;
      case '+': case '=': setZoom(zoom + 0.2); break;
      case '-': case '_': setZoom(zoom - 0.2); break;
      case '0': setZoom(1); break;
      default: return;
    }
  });
})();
`
