package routers

// Blog image lightbox.
//
// Served from /blog/lightbox.js so both article-bearing templates can pull in the same script,
// the same way ask.js is shared. No third-party library: the blog already ships a hand-built
// graph engine, and a viewer is small enough that a dependency would cost more than it saves.
//
// 博客图片灯箱。
//
// 由 /blog/lightbox.js 提供，两个含正文的模板共用同一份脚本，与 ask.js 的分发方式一致。
// 不引入第三方库：博客已经自带手写的图谱引擎，而一个查看器足够小，
// 引入依赖的代价高于自己写。
//
// Design decisions worth recording, because each was a fork in the road:
//
//   - Geometry (left/top/width/height) is animated rather than a transform. A transform would
//     have been smoother on paper, but the thumbnail's corner radius has to stay visually
//     identical while the image grows, and a scaled radius (r/scale) drifts on non-square
//     images. Animating the box keeps the radius correct for free, and one element is not
//     worth the optimisation.
//   - Switching images crossfades; only closing flies back. Flying back on every switch would
//     double the animation and feel sluggish when stepping through a series of diagrams.
//   - Zoom is anchored at the pointer (wheel, double-click) or at the pinch midpoint, because
//     zooming to the centre forces the reader to re-pan to whatever they were looking at.
//
// 值得记录的三个设计取舍，因为每一处都曾是岔路：
//
//   - 动画的是几何属性（left/top/width/height）而不是 transform。transform 纸面上更顺滑，
//     但缩略图的圆角在图片放大过程中必须保持视觉一致，而按比例反向缩放圆角（r/scale）
//     在非方形图片上会漂移。动盒子属性让圆角天然正确，而单个元素不值得那点优化。
//   - 切图用交叉淡切，只有关闭才飞回。每次切图都飞回会让动画时长翻倍，连续翻看解剖图时很拖沓。
//   - 缩放锚定在指针位置（滚轮、双击）或双指中点，因为以中心缩放会迫使读者重新平移回原来看的地方。

// blogLightboxAssetVersion busts the browser cache when the script below changes.
//
// The script is served with a long max-age. Without a version in the URL, a reader who has
// already loaded it keeps the old copy for the whole cache lifetime — which is exactly the
// failure the graph script hit: a fixed graph kept behaving like the broken one for a year.
//
// blogLightboxAssetVersion 在脚本变化时击穿浏览器缓存。
//
// 脚本以较长的 max-age 投递。URL 里没有版本号时，已经加载过的读者会在整个缓存周期内
// 继续使用旧副本——这正是图谱脚本踩过的坑：一个已修好的图谱在读者那里坏了整整一年。
const blogLightboxAssetVersion = "lb-1"

const blogLightboxScript = `(function () {
  'use strict';

  if (window.__fnsLightbox) return;
  window.__fnsLightbox = true;

  var REDUCE = window.matchMedia && window.matchMedia('(prefers-reduced-motion:reduce)').matches;
  var MIN_SCALE = 1;
  var MAX_SCALE = 5;
  var DOUBLE_TAP_SCALE = 2.5;
  var SWIPE_CLOSE_PX = 110;
  var SWIPE_SWITCH_PX = 60;

  var root = null;      // overlay element
  var stage = null;     // the animated box holding the image
  var view = null;      // inner <img>
  var spinner = null;   // shimmer skeleton (fallback while a slow image decodes)
  var counter = null;
  var btnClose = null;
  var btnPrev = null;
  var btnNext = null;

  var items = [];       // every eligible image in this article
  var current = 0;
  var open = false;
  var originRect = null;   // where the current image started, for the fly-back
  var originRadius = '';
  var scale = 1;
  var panX = 0;
  var panY = 0;

  // Pointer bookkeeping. Two pointers mean pinch; one means pan (zoomed) or swipe (at 1x).
  // 指针记账。两个指针表示捏合；一个表示平移（已放大）或滑动（1x）。
  var pointers = [];
  var pinchStartDist = 0;
  var pinchStartScale = 1;
  var dragStart = null;
  var lastTapAt = 0;
  var lastTapX = 0;
  var lastTapY = 0;
  var moved = false;

  // ── Eligibility ────────────────────────────────────────────────────────────
  //
  // Only body images. The graph layer, the nav icon and any emoji are inline SVG or tiny
  // rasters, and opening a 20px glyph full screen is noise rather than a feature. Images
  // nested in a running table cell count — the anatomy plates live there.
  //
  // 只接管正文图片。图谱层、导航图标和表情都是内联 SVG 或极小位图，把 20px 的符号放全屏
  // 是噪音而不是功能。嵌在表格单元格里的图片算数——解剖图就在那些位置。
  function eligible(img) {
    if (!img || img.tagName !== 'IMG') return false;
    if (img.classList && /(^|\s)(emoji|icon|avatar|badge)(\s|$)/.test(img.className)) return false;
    var src = img.currentSrc || img.getAttribute('src') || '';
    if (!src || src.slice(0, 5) === 'data:') return false;
    // Trust the laid-out box over naturalWidth: a lazy image still decoding reports 0 there,
    // but it is already user-visible and must stay clickable.
    // 以已布局的盒子为准而不是 naturalWidth：仍在解码的懒加载图那里是 0，
    // 但它已经对读者可见，必须保持可点。
    var w = img.clientWidth || img.naturalWidth || 0;
    var h = img.clientHeight || img.naturalHeight || 0;
    if (w < 48 && h < 48) return false;
    return true;
  }

  function collect() {
    items = [];
    var nodes = document.querySelectorAll('.post-body img');
    for (var i = 0; i < nodes.length; i++) {
      if (eligible(nodes[i])) items.push(nodes[i]);
    }
  }

  // ── Build ──────────────────────────────────────────────────────────────────
  function build() {
    root = document.createElement('div');
    root.className = 'lb-root';
    root.setAttribute('role', 'dialog');
    root.setAttribute('aria-modal', 'true');
    root.setAttribute('aria-label', '图片查看');
    root.hidden = true;

    var backdrop = document.createElement('div');
    backdrop.className = 'lb-backdrop';

    stage = document.createElement('div');
    stage.className = 'lb-stage';

    view = document.createElement('img');
    view.className = 'lb-img';
    view.alt = '';
    view.decoding = 'async';
    stage.appendChild(view);

    spinner = document.createElement('div');
    spinner.className = 'lb-shimmer';
    spinner.setAttribute('aria-hidden', 'true');
    stage.appendChild(spinner);

    var bar = document.createElement('div');
    bar.className = 'lb-bar';

    btnPrev = document.createElement('button');
    btnPrev.type = 'button';
    btnPrev.className = 'lb-btn lb-prev';
    btnPrev.setAttribute('aria-label', '上一张');
    btnPrev.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M15 5l-7 7 7 7" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>';

    counter = document.createElement('span');
    counter.className = 'lb-count';

    btnNext = document.createElement('button');
    btnNext.type = 'button';
    btnNext.className = 'lb-btn lb-next';
    btnNext.setAttribute('aria-label', '下一张');
    btnNext.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 5l7 7-7 7" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>';

    btnClose = document.createElement('button');
    btnClose.type = 'button';
    btnClose.className = 'lb-btn lb-close';
    btnClose.setAttribute('aria-label', '关闭');
    btnClose.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>';

    bar.appendChild(btnPrev);
    bar.appendChild(counter);
    bar.appendChild(btnNext);
    root.appendChild(backdrop);
    root.appendChild(stage);
    root.appendChild(bar);
    root.appendChild(btnClose);
    document.body.appendChild(root);

    backdrop.addEventListener('click', close);
    btnClose.addEventListener('click', close);
    btnPrev.addEventListener('click', function (e) { e.stopPropagation(); step(-1); });
    btnNext.addEventListener('click', function (e) { e.stopPropagation(); step(1); });
    stage.addEventListener('click', onStageClick);
    stage.addEventListener('wheel', onWheel, { passive: false });
    stage.addEventListener('pointerdown', onPointerDown);
    stage.addEventListener('pointermove', onPointerMove);
    stage.addEventListener('pointerup', onPointerUp);
    stage.addEventListener('pointercancel', onPointerUp);
    document.addEventListener('keydown', onKey);
  }

  // ── Open / close ───────────────────────────────────────────────────────────
  function openAt(img) {
    if (!root) build();
    collect();
    var idx = items.indexOf(img);
    if (idx < 0) { items.push(img); idx = items.length - 1; }

    current = idx;
    open = true;
    moved = false;
    root.hidden = false;
    document.documentElement.classList.add('lb-lock');

    originRect = img.getBoundingClientRect();
    var cs = window.getComputedStyle(img);
    originRadius = cs.borderRadius && cs.borderRadius !== '0px' ? cs.borderRadius : '10px';

    paint(true);
    // Force a layout read so the FLIP start position is real before the transition begins.
    // 强制读一次布局，使 FLIP 的起点在过渡开始前就是真实值。
    void stage.offsetWidth;
    requestAnimationFrame(function () {
      root.classList.add('lb-in');
      flyToTarget();
    });
  }

  function close() {
    if (!open) return;
    open = false;
    root.classList.remove('lb-in');
    document.documentElement.classList.remove('lb-lock');
    resetZoom(false);

    var done = function () {
      root.hidden = true;
      root.removeEventListener('transitionend', done);
      // The fly-back target may be stale if the page scrolled while the viewer was open.
      // 页面在查看期间滚动过的话，飞回目标可能已过期。
      var home = items[current];
      if (home && home.getBoundingClientRect) originRect = home.getBoundingClientRect();
    };

    if (REDUCE) { done(); return; }
    flyToOrigin();
    root.addEventListener('transitionend', done);
    setTimeout(done, 420);
  }

  // ── FLIP ───────────────────────────────────────────────────────────────────
  //
  // The stage is animated by geometry, so the start state is the thumbnail's own box and the
  // end state is the centred, fitted box. Both are plain pixel values, so the corner radius
  // never distorts the way a scaled radius would.
  //
  // 舞台按几何属性动画，因此起始状态就是缩略图自身的盒子，结束状态是居中并适配后的盒子。
  // 两端都是像素值，圆角不会像按比例缩放那样变形。
  function fitBox(w, h) {
    var vw = window.innerWidth;
    var vh = window.innerHeight;
    var pad = 28;
    var maxW = Math.max(1, vw - pad * 2);
    var maxH = Math.max(1, vh - pad * 2);
    var ratio = Math.min(maxW / w, maxH / h, 1);
    var bw = Math.round(w * ratio);
    var bh = Math.round(h * ratio);
    return { w: bw, h: bh, left: Math.round((vw - bw) / 2), top: Math.round((vh - bh) / 2) };
  }

  function flyToTarget() {
    var natW = view.naturalWidth || originRect.width;
    var natH = view.naturalHeight || originRect.height;
    var box = fitBox(natW, natH);
    stage.style.borderRadius = '12px';
    stage.style.left = box.left + 'px';
    stage.style.top = box.top + 'px';
    stage.style.width = box.w + 'px';
    stage.style.height = box.h + 'px';
  }

  function flyToOrigin() {
    if (!originRect) return;
    stage.style.borderRadius = originRadius;
    stage.style.left = originRect.left + 'px';
    stage.style.top = originRect.top + 'px';
    stage.style.width = originRect.width + 'px';
    stage.style.height = originRect.height + 'px';
  }

  // ── Painting ───────────────────────────────────────────────────────────────
  function paint(initial) {
    var img = items[current];
    if (!img) return;

    var src = img.currentSrc || img.getAttribute('src');
    var alt = img.getAttribute('alt') || '';

    if (initial) {
      // Start exactly on top of the thumbnail so the growth reads as the same object.
      // 起点精确覆盖缩略图，使放大看起来是同一个对象。
      stage.style.transition = 'none';
      stage.style.left = originRect.left + 'px';
      stage.style.top = originRect.top + 'px';
      stage.style.width = originRect.width + 'px';
      stage.style.height = originRect.height + 'px';
      stage.style.borderRadius = originRadius;
      void stage.offsetWidth;
      stage.style.transition = '';
    }

    if (view.getAttribute('src') !== src) {
      view.classList.remove('lb-ready');
      spinner.classList.remove('lb-off');
      view.setAttribute('src', src);
      view.setAttribute('alt', alt);
    }

    var reveal = function () {
      view.classList.add('lb-ready');
      spinner.classList.add('lb-off');
      if (!initial) return;
      flyToTarget();
    };

    if (view.complete && view.naturalWidth) {
      reveal();
    } else {
      view.onload = reveal;
      view.onerror = reveal;
    }

    var multi = items.length > 1;
    counter.textContent = multi ? (current + 1) + ' / ' + items.length : '';
    counter.hidden = !multi;
    btnPrev.hidden = !multi;
    btnNext.hidden = !multi;
  }

  // ── Navigation ─────────────────────────────────────────────────────────────
  function step(dir) {
    if (items.length < 2) return;
    current = (current + dir + items.length) % items.length;
    resetZoom(false);
    // Crossfade rather than fly: the reader is stepping through a set, not entering it.
    // 用淡切而不是飞入：读者是在一组图里翻页，而不是进入其中一张。
    stage.classList.add('lb-swap');
    var done = function () {
      stage.classList.remove('lb-swap');
      stage.removeEventListener('transitionend', done);
    };
    if (REDUCE) { paint(false); flyToTarget(); return; }
    stage.addEventListener('transitionend', done);
    setTimeout(function () {
      paint(false);
      flyToTarget();
      stage.classList.remove('lb-swap');
    }, 120);
  }

  // ── Zoom ───────────────────────────────────────────────────────────────────
  function applyZoom() {
    view.style.transform = 'translate3d(' + panX + 'px,' + panY + 'px,0) scale(' + scale + ')';
  }

  function resetZoom(animate) {
    scale = 1;
    panX = 0;
    panY = 0;
    view.style.transition = animate && !REDUCE ? 'transform .24s cubic-bezier(.22,1,.36,1)' : 'none';
    applyZoom();
    if (animate && !REDUCE) {
      setTimeout(function () { view.style.transition = ''; }, 260);
    }
  }

  function clampPan() {
    // Keep the image from being dragged entirely out of view: the allowed travel is however
    // much of the scaled image can leave the stage while still covering it.
    // 不让图片被拖出视野：允许的位移是放大后的图片仍能覆盖舞台的那部分。
    var maxX = Math.max(0, (stage.clientWidth * scale - stage.clientWidth) / 2);
    var maxY = Math.max(0, (stage.clientHeight * scale - stage.clientHeight) / 2);
    if (panX > maxX) panX = maxX;
    if (panX < -maxX) panX = -maxX;
    if (panY > maxY) panY = maxY;
    if (panY < -maxY) panY = -maxY;
  }

  function zoomAt(nextScale, cx, cy) {
    nextScale = Math.max(MIN_SCALE, Math.min(MAX_SCALE, nextScale));
    var rect = stage.getBoundingClientRect();
    // Anchor: keep the point under the cursor fixed while the scale changes.
    // 锚点：缩放变化时，让指针下方的那一点保持不动。
    var ox = cx - rect.left - rect.width / 2;
    var oy = cy - rect.top - rect.height / 2;
    var k = nextScale / scale;
    panX = ox - (ox - panX) * k;
    panY = oy - (oy - panY) * k;
    scale = nextScale;
    clampPan();
    if (scale <= 1.001) { scale = 1; panX = 0; panY = 0; }
    applyZoom();
  }

  function onWheel(e) {
    if (!open) return;
    e.preventDefault();
    var factor = Math.exp(-e.deltaY * 0.0016);
    zoomAt(scale * factor, e.clientX, e.clientY);
  }

  function onStageClick(e) {
    if (moved) { moved = false; return; }
    // The double-tap test must come before any zoomed-state guard. An earlier version
    // returned early while zoomed, on the reasoning that "a click while zoomed is a pan
    // attempt" — which silently made the second double-tap unable to zoom back out. There is
    // no close action on the stage anyway (closing is the backdrop, the button or Escape), so
    // the guard protected nothing.
    //
    // 双击判定必须放在任何「已缩放」守卫之前。早先的版本在缩放状态下提前返回，理由是
    // 「缩放时的点击是平移意图」——这让第二次双击无法缩回去，而舞台本就没有关闭动作
    // （关闭靠遮罩、按钮或 Esc），那条守卫什么也没保护。
    if (e.target !== view) return;

    var now = Date.now();
    var dt = now - lastTapAt;
    var near = Math.abs(e.clientX - lastTapX) < 28 && Math.abs(e.clientY - lastTapY) < 28;
    if (dt < 320 && near) {
      lastTapAt = 0;
      zoomAt(scale === 1 ? DOUBLE_TAP_SCALE : 1, e.clientX, e.clientY);
      return;
    }
    lastTapAt = now;
    lastTapX = e.clientX;
    lastTapY = e.clientY;
  }

  // ── Pointer gestures ───────────────────────────────────────────────────────
  function midpoint() {
    var a = pointers[0];
    var b = pointers[1];
    return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
  }
  function distance() {
    var a = pointers[0];
    var b = pointers[1];
    return Math.hypot(a.x - b.x, a.y - b.y);
  }

  function onPointerDown(e) {
    if (!open) return;
    pointers.push({ id: e.pointerId, x: e.clientX, y: e.clientY });
    moved = false;
    dragStart = null;

    if (pointers.length === 2) {
      pinchStartDist = distance();
      pinchStartScale = scale;
      return;
    }
    if (pointers.length === 1) {
      dragStart = { x: e.clientX, y: e.clientY, panX: panX, panY: panY, scale: scale };
      if (scale > 1.01) {
        stage.setPointerCapture && stage.setPointerCapture(e.pointerId);
      }
    }
  }

  function onPointerMove(e) {
    if (!open) return;
    var p = null;
    for (var i = 0; i < pointers.length; i++) {
      if (pointers[i].id === e.pointerId) { p = pointers[i]; break; }
    }
    if (!p) return;
    p.x = e.clientX;
    p.y = e.clientY;

    if (pointers.length === 2) {
      if (pinchStartDist > 0) {
        var mid = midpoint();
        zoomAt(pinchStartScale * (distance() / pinchStartDist), mid.x, mid.y);
        moved = true;
      }
      return;
    }

    if (!dragStart) return;
    var dx = e.clientX - dragStart.x;
    var dy = e.clientY - dragStart.y;

    if (dragStart.scale > 1.01) {
      if (Math.abs(dx) > 3 || Math.abs(dy) > 3) moved = true;
      panX = dragStart.panX + dx;
      panY = dragStart.panY + dy;
      clampPan();
      view.style.transition = 'none';
      applyZoom();
      return;
    }

    // At 1x a horizontal swipe steps, a downward swipe dismisses.
    // 在 1x 下，横向滑动翻页，向下滑动关闭。
    if (Math.abs(dx) > 6 || Math.abs(dy) > 6) moved = true;
    if (Math.abs(dy) > Math.abs(dx) && dy > 0) {
      stage.style.transform = 'translate3d(0,' + Math.min(dy, 160) + 'px,0)';
    }
  }

  function onPointerUp(e) {
    if (!open) return;
    pointers = pointers.filter(function (p) { return p.id !== e.pointerId; });

    if (pointers.length === 0 && dragStart) {
      var dx = e.clientX - dragStart.x;
      var dy = e.clientY - dragStart.y;
      stage.style.transform = '';

      if (dragStart.scale <= 1.01) {
        if (Math.abs(dy) > Math.abs(dx) && dy > SWIPE_CLOSE_PX) { close(); }
        else if (Math.abs(dx) > SWIPE_SWITCH_PX && Math.abs(dx) > Math.abs(dy)) {
          step(dx < 0 ? 1 : -1);
        }
      }
      dragStart = null;
    }
    if (pointers.length < 2) pinchStartDist = 0;
  }

  function onKey(e) {
    if (!open) return;
    if (e.key === 'Escape') { e.preventDefault(); close(); }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); step(-1); }
    else if (e.key === 'ArrowRight') { e.preventDefault(); step(1); }
    else if (e.key === '0') { e.preventDefault(); resetZoom(true); }
  }

  // ── Wiring ─────────────────────────────────────────────────────────────────
  //
  // Delegated rather than per-image: the article body is replaced wholesale by some of the
  // blog's navigation paths, and per-image listeners would silently stop firing.
  //
  // 用事件委托而不是逐图绑定：博客的部分导航会整体替换正文，逐图绑定的监听器会静默失效。
  document.addEventListener('click', function (e) {
    var t = e.target;
    if (!t || t.tagName !== 'IMG') return;
    if (!t.closest || !t.closest('.post-body')) return;
    if (!eligible(t)) return;
    e.preventDefault();
    openAt(t);
  });

  window.addEventListener('resize', function () {
    if (!open || scale > 1.01) return;
    flyToTarget();
  });
})();`
