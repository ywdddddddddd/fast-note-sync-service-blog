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

    // Never start a sheet drag on top of a control.
    //
    // The drag strip is the top 56px of the sheet, and the graph card puts its toolbar — the depth
    // slider and the three icon buttons — inside exactly that strip. Pressing one of those buttons
    // therefore also began a drag, and a drag calls setPointerCapture on the sheet, which retargets
    // every later pointer event to the sheet itself. The button got pointerdown and then lost the
    // pointerup and the click: the press animation played and nothing happened. Measured on the live
    // page, the same press reported
    //   pointerdown target=svg (inside the button)
    //   pointerup   target=graph-panel
    // which is why no click ever fired.
    //
    // 永远不要在一个控件上面开始拖抽屉。
    //
    // 拖拽热区是抽屉顶部那 56px，而图谱卡片把它的工具条——深度滑块和三颗图标按钮——正好放在那条里。
    // 于是按其中任何一颗也会启动拖拽，而拖拽会对抽屉调用 setPointerCapture，
    // 把之后所有指针事件重定向到抽屉本身。按钮收到了 pointerdown，然后丢掉了 pointerup 与 click：
    // 按压动效照常播放，功能毫无反应。线上实测同一次按压报的是
    //   pointerdown target=svg（按钮内部）
    //   pointerup   target=graph-panel
    // 这就是 click 从未触发的原因。
    if (ev.target && ev.target.closest &&
        ev.target.closest('button, a, input, select, textarea, label, [role="button"], [contenteditable="true"]')) {
      return;
    }

    var rect = el.getBoundingClientRect();
    // Only the top strip drags. Making the whole panel draggable would fight the content's own
    // scrolling: the reader would try to scroll a long nav tree and instead resize the sheet.
    //
    // The strip has to be bounded on all four sides. This used to test only the bottom edge —
    // ev.clientY - rect.top > DRAG_ZONE — which is negative for any press *above* the panel, so
    // every click anywhere on the page sailed through: the press claimed the pointer with
    // setPointerCapture, added is-dragging, and the element actually pressed never saw its click.
    // With the panel open the whole page stopped responding, and the scrim — which closes the panel
    // on click — stopped closing it too, because the scrim never received the click either.
    //
    // 只有顶部一条可拖。整块可拖会和内容滚动打架：读者想滚长目录，结果把面板拉了。
    //
    // 这条热区必须四边都卡住。这里曾经只判断下边界——
    // ev.clientY - rect.top 大于 DRAG_ZONE——而点在面板**上方**时这个值是负数，于是整页任何一处
    // 按下都能通过：那次按压会用 setPointerCapture 抢走指针、加上 is-dragging，
    // 真正被按的元素再也收不到自己的 click。面板一开整页就失去响应，
    // 连「点遮罩关面板」也失效了，因为遮罩同样收不到 click。
    if (ev.clientX < rect.left || ev.clientX > rect.right) return;
    if (ev.clientY < rect.top || ev.clientY > rect.top + DRAG_ZONE) return;

    drag = {
      el: el,
      startY: ev.clientY,
      startH: rect.height,
      moved: false,
      samples: []
    };
    el.classList.add('is-dragging');
    el.classList.remove('is-snapping', 'is-spring');
    if (springRaf) { cancelAnimationFrame(springRaf); springRaf = 0; }
    // Pointer capture keeps the gesture alive even when the finger leaves the strip.
    // 指针捕获让手势在手指离开那条热区后仍然有效。
    try { el.setPointerCapture(ev.pointerId); } catch (e) {}
  }

  function onMove(ev) {
    if (!drag) return;
    var dy = drag.startY - ev.clientY;          // 向上为正 // up is positive
    if (Math.abs(dy) > 3) drag.moved = true;
    var vh = window.innerHeight;

    // Rubber band instead of a wall.
    //
    // A hard clamp tells the finger "this is the end" by refusing to move at all, which reads as the
    // gesture having failed. Past the limit the sheet keeps following, at 25% (top) and 35% (bottom)
    // of the finger's travel, and a hard ceiling keeps a violent fling from throwing it off screen.
    //
    // 橡皮筋而不是一堵墙。
    //
    // 硬夹等于用「完全不动」告诉手指「到头了」，那读起来像手势失灵。
    // 越过边界之后面板继续跟随，顶部按位移的 25%、底部按 35%，
    // 再压一个硬上限，免得一次猛甩把它甩出屏幕。
    var raw = drag.startH + dy;
    var maxH = vh * 0.96, minH = 60;
    var next = raw;
    if (raw > maxH) { next = maxH + (raw - maxH) * 0.25; }
    if (raw < minH) { next = minH - (minH - raw) * 0.35; }
    if (next > maxH + 48) { next = maxH + 48; }
    if (next < 24) { next = 24; }

    drag.el.style.setProperty('--sheet-h', ((next / vh) * 100).toFixed(2) + 'vh');

    // Keep only the last 120ms of travel: that is the window the release velocity is read from.
    // A longer window would let a slow drift at the start of the gesture outweigh the flick at the
    // end, and the sheet would settle in a direction nobody asked for.
    //
    // 只保留最近 120ms 的位移：松手速度从这个窗口里读。
    // 窗口更长会让手势开头那段慢速移动压过结尾的那一下，面板就会停在没人要求的方向上。
    var now = (ev.timeStamp || performance.now());
    drag.samples.push({ t: now, y: ev.clientY });
    while (drag.samples.length > 2 && now - drag.samples[0].t > 120) { drag.samples.shift(); }

    if (drag.moved) ev.preventDefault();
  }

  function onUp() {
    if (!drag) return;
    var el = drag.el;
    var frac = currentVFrac(el);
    el.classList.remove('is-dragging');

    // Release velocity, px per ms, positive while the finger was still moving up.
    // 松手速度，单位 px/ms，手指仍在向上时为正。
    var vel = 0;
    var s = drag.samples;
    if (s.length >= 2) {
      var a = s[0], b = s[s.length - 1];
      var dt = b.t - a.t;
      if (dt > 0) { vel = (a.y - b.y) / dt; }
    }

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

    // The stop is chosen by where the sheet is, then nudged by how it was thrown: a flick that ends
    // just short of a stop still reaches it, and a flick that would land on the stop it came from
    // carries past it. Without the nudge the sheet always snaps backwards from a fast throw, which
    // is the one motion that makes a gesture feel ignored.
    //
    // 停靠点先按面板现在的位置选，再按甩动的力度修正：
    // 一次差一点就到停靠点的甩动仍然会到达它；一次本该落回起点的甩动会顺势越过。
    // 少了这个修正，快速甩动总是往回吸，而那是唯一一种让手势显得被无视的运动。
    var target = nearestStop(frac);
    if (Math.abs(vel) > 0.55) {
      var dir = vel > 0 ? 1 : -1;
      for (var i = 0; i < STOPS.length; i++) {
        if (dir > 0 && STOPS[i] > frac) { target = STOPS[i]; break; }
        if (dir < 0 && STOPS[STOPS.length - 1 - i] < frac) { target = STOPS[STOPS.length - 1 - i]; break; }
      }
    }

    drag = null;
    springTo(el, frac, vel, target);
  }

  // A real spring, integrated per frame.
  //
  // The old code added a class and let a CSS transition take over, which cannot use the gesture's
  // velocity: every release started from zero and the sheet ignored how it was thrown. A spring
  // that starts with the finger's own velocity is the difference between "it followed me and let go"
  // and "it jumped to a stop".
  //
  // 真正的弹簧，逐帧积分。
  //
  // 原来的做法是加一个类、交给 CSS 过渡 —— 而过渡无法使用手势的速度：
  // 每次松手都从零开始，面板完全无视它刚才是怎么被甩出去的。
  // 一个带着手指速度起步的弹簧，就是「它跟着我、然后松开了」与「它跳到一个位置」的差别。
  var springRaf = 0;

  function springTo(el, fromFrac, velPxPerMs, targetFrac) {
    if (springRaf) { cancelAnimationFrame(springRaf); springRaf = 0; }
    var vh = window.innerHeight;
    var x = fromFrac * vh;
    var v = velPxPerMs;
    var target = targetFrac * vh;
    var last = 0;

    // k = 0.0018, c = 0.075 → ω ≈ 0.042/ms, ζ ≈ 0.88: settles in roughly half a second with one
    // small overshoot. Stiffer reads as a snap, softer reads as the sheet being slow to obey.
    // k = 0.0018、c = 0.075 → ω ≈ 0.042/ms、ζ ≈ 0.88：约半秒收敛，只有一次很小的过冲。
    // 更硬读起来像啪一下，更软读起来像面板反应迟钝。
    var K = 0.0018, C = 0.075;

    el.classList.add('is-spring');

    function step(now) {
      if (!last) { last = now; }
      var dt = Math.min(48, Math.max(1, now - last));
      last = now;
      var a = (target - x) * K - v * C;
      v += a * dt;
      x += v * dt;
      if (Math.abs(target - x) < 0.5 && Math.abs(v) < 0.05) {
        el.style.setProperty('--sheet-h', ((target / vh) * 100).toFixed(2) + 'vh');
        el.classList.remove('is-spring');
        springRaf = 0;
        return;
      }
      el.style.setProperty('--sheet-h', ((x / vh) * 100).toFixed(2) + 'vh');
      springRaf = requestAnimationFrame(step);
    }
    springRaf = requestAnimationFrame(step);
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
