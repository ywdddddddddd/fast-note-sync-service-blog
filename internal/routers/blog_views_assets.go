package routers

// Home page view switching: list, cover grid, and the concentric wheel.
//
// The list stays server-rendered — it is the view that must work without script. The other two
// are drawn here, both from the one JSON array the template embeds.
//
// 首页视图切换：列表、封面网格、同心轮。
//
// 列表保持服务端渲染——它是必须能在无脚本时工作的那个视图。另外两个在这里绘制，
// 数据都来自模板内嵌的那一个 JSON 数组。
//
// Everything in this file obeys one constraint that shapes all of it: Go raw strings cannot
// contain backticks, so no JS template literals anywhere. Concatenation only.
//
// 本文件的一切都受同一条约束影响：Go 的原始字符串不能包含反引号，因此任何地方都不能用
// JS 模板字面量，只能用拼接。
const blogViewsScript = `
(function () {
  'use strict';

  var STORAGE_KEY = 'fns:view';
  var MOBILE_QUERY = '(max-width: 860px)';

  var root = document.documentElement;
  var dataEl = document.getElementById('fns-entries');
  if (!dataEl) { return; }

  var entries = [];
  try {
    entries = JSON.parse(dataEl.textContent || '[]');
  } catch (e) {
    entries = [];
  }
  if (!Array.isArray(entries) || !entries.length) { return; }

  var listBody = document.querySelector('[data-view-body="list"]');
  var coverBody = document.querySelector('[data-view-body="cover"]');
  var wheelRoot = document.getElementById('wheel-root');
  var switchEl = document.querySelector('.view-switch');
  if (!listBody || !coverBody || !wheelRoot || !switchEl) { return; }

  // Move the wheel out of the note card before doing anything with it.
  //
  // It is position:fixed, but it is rendered inside <section class="card card-pad">, and any
  // transformed, filtered or animated ancestor turns position:fixed into "fixed relative to that
  // ancestor" — the wheel then sizes itself to the card instead of the viewport and the site's
  // top bar draws over it. Measured before this line existed: the wheel started 76 px down the
  // page, exactly the height of the header, and its circle was clipped to the card's width.
  //
  // 在动它之前先把轮子移出笔记卡片。
  //
  // 它是 position:fixed，但被渲染在 <section class="card card-pad"> 里面，而任何带
  // transform、filter 或动画的祖先都会把 position:fixed 变成「相对该祖先固定」——
  // 于是轮子的尺寸按卡片而不是视口计算，顶栏还会压在它上面。
  // 这一行存在之前实测：轮子从页面往下 76px 处开始，正好是顶栏高度，圆也被裁到卡片宽度。
  if (wheelRoot.parentNode !== document.body) {
    document.body.appendChild(wheelRoot);
  }

  var buttons = Array.prototype.slice.call(switchEl.querySelectorAll('.vs-btn'));
  var current = 'list';
  var coverBuilt = false;
  var wheel = null;

  function isMobile() {
    return window.matchMedia(MOBILE_QUERY).matches;
  }

  function readStored() {
    try {
      var v = window.localStorage.getItem(STORAGE_KEY);
      if (v === 'cover' || v === 'wheel' || v === 'list') { return v; }
    } catch (e) { /* private mode */ }
    return 'list';
  }

  function store(v) {
    try { window.localStorage.setItem(STORAGE_KEY, v); } catch (e) { /* ignore */ }
  }

  // One frame builder for both views. A note without a cover gets a flat colour decided by the
  // stylesheet — never a derived tint and never a gradient. Measured on this vault: with 150
  // uncovered notes, a per-title gradient palette turned the whole wheel into one continuous
  // beige smear and no card read as an object.
  //
  // 两个视图共用一个画框构建器。没有封面的笔记得到由样式表决定的平色——
  // 绝不派生颜色，也绝不画渐变。在本仓库实测：150 篇都没有封面时，
  // 按标题派生的渐变调色板让整个轮子连成一片米色糊，没有任何一张卡片读起来是一个物体。
  function coverFrame(entry, forWheel) {
    var frame = document.createElement('div');
    frame.className = forWheel ? 'wheel-frame' : 'cover-frame';

    var cover = entry.c;
    if (cover && cover.url) {
      var el;
      if (cover.kind === 'video') {
        el = document.createElement('video');
        el.muted = true;
        el.loop = true;
        el.autoplay = true;
        el.playsInline = true;
        el.setAttribute('muted', '');
        el.setAttribute('playsinline', '');
        el.setAttribute('preload', 'metadata');
        el.setAttribute('tabindex', '-1');
      } else {
        el = document.createElement('img');
        el.loading = 'lazy';
        el.decoding = 'async';
        el.alt = '';
      }
      el.src = cover.url;
      if (cover.focusX !== undefined) {
        frame.style.setProperty('--fx', cover.focusX + '%');
        frame.style.setProperty('--fy', cover.focusY + '%');
        el.style.objectPosition = cover.focusX + '% ' + cover.focusY + '%';
      }
      if (cover.zoom !== undefined) {
        frame.style.setProperty('--cz', String(cover.zoom));
      }
      frame.appendChild(el);
    } else {
      frame.classList.add(forWheel ? 'wheel-frame--blank' : 'cover-frame--blank');
    }
    return frame;
  }

  function iconDate() {
    var ns = 'http://www.w3.org/2000/svg';
    var s = document.createElementNS(ns, 'svg');
    s.setAttribute('viewBox', '0 0 24 24');
    s.setAttribute('fill', 'none');
    s.setAttribute('stroke', 'currentColor');
    s.setAttribute('stroke-width', '2');
    s.setAttribute('stroke-linecap', 'round');
    s.setAttribute('stroke-linejoin', 'round');
    var p1 = document.createElementNS(ns, 'path');
    p1.setAttribute('d', 'M8 2v4');
    var p2 = document.createElementNS(ns, 'path');
    p2.setAttribute('d', 'M16 2v4');
    var r = document.createElementNS(ns, 'rect');
    r.setAttribute('width', '18');
    r.setAttribute('height', '18');
    r.setAttribute('x', '3');
    r.setAttribute('y', '4');
    r.setAttribute('rx', '2');
    var p3 = document.createElementNS(ns, 'path');
    p3.setAttribute('d', 'M3 10h18');
    s.appendChild(p1); s.appendChild(p2); s.appendChild(r); s.appendChild(p3);
    return s;
  }

  function iconWords() {
    var ns = 'http://www.w3.org/2000/svg';
    var s = document.createElementNS(ns, 'svg');
    s.setAttribute('viewBox', '0 0 24 24');
    s.setAttribute('fill', 'none');
    s.setAttribute('stroke', 'currentColor');
    s.setAttribute('stroke-width', '2');
    s.setAttribute('stroke-linecap', 'round');
    s.setAttribute('stroke-linejoin', 'round');
    var a = document.createElementNS(ns, 'path');
    a.setAttribute('d', 'M4 7V4h16v3');
    var b = document.createElementNS(ns, 'path');
    b.setAttribute('d', 'M9 20h6');
    var c = document.createElementNS(ns, 'path');
    c.setAttribute('d', 'M12 4v16');
    s.appendChild(a); s.appendChild(b); s.appendChild(c);
    return s;
  }

  function buildCoverGrid() {
    if (coverBuilt) { return; }
    coverBuilt = true;

    var frag = document.createDocumentFragment();
    for (var i = 0; i < entries.length; i++) {
      var e = entries[i];
      var card = document.createElement('a');
      card.className = 'cover-card';
      card.href = '/blog/' + e.s;

      card.appendChild(coverFrame(e, false));

      var body = document.createElement('div');
      body.className = 'cover-card-body';

      var h = document.createElement('h3');
      h.className = 'cover-card-title';
      h.textContent = e.t || '';
      body.appendChild(h);

      var meta = document.createElement('div');
      meta.className = 'cover-card-meta';
      if (e.d) {
        var ds = document.createElement('span');
        ds.appendChild(iconDate());
        ds.appendChild(document.createTextNode(e.d));
        meta.appendChild(ds);
      }
      if (e.w) {
        var ws = document.createElement('span');
        ws.appendChild(iconWords());
        ws.appendChild(document.createTextNode(e.w + ' 字'));
        meta.appendChild(ws);
      }
      if (e.v) {
        var vs = document.createElement('span');
        vs.className = 'm-cat';
        vs.textContent = e.v;
        meta.appendChild(vs);
      }
      body.appendChild(meta);
      card.appendChild(body);
      frag.appendChild(card);
    }
    coverBody.appendChild(frag);
  }

  // ── The wheel ────────────────────────────────────────────────────────────
  // Slot counts are the owner's: eight inner, fourteen outer. Damping constants and both radii
  // are copied from the reference page, not tuned by hand — guessing at them is what made the
  // first attempt feel sticky and look sparse.
  //
  // 槽位数量是使用者定的：内圈 8、外圈 14。阻尼常数与两个半径都取自参照页，不是手调的——
  // 上一版靠估，手感发黏、画面发疏。
  var INNER_SLOTS = 8;
  var OUTER_SLOTS = 14;
  var TAU = Math.PI * 2;
  var BASE_SPIN = 0.00075;
  var FRICTION = 0.94;
  var WHEEL_GAIN = 0.00009;
  var MAX_WHEEL = 0.030;

  function createWheel() {
    var stage = document.createElement('div');
    stage.className = 'wheel-stage';

    // Two dashed guide circles, straight from the reference. They are barely visible by design —
    // 1px dashed at 5% — but without them the cards read as floating tiles rather than as
    // gondolas hung on a wheel.
    //
    // 两条虚线轨道，直接取自参照页。它们刻意几乎看不见——1px 虚线、5%——
    // 但没有它们，卡片读起来是漂浮的瓷砖，而不是挂在轮子上的吊舱。
    var trackInner = document.createElement('div');
    trackInner.className = 'wheel-track';
    var trackOuter = document.createElement('div');
    trackOuter.className = 'wheel-track';

    var vignette = document.createElement('div');
    vignette.className = 'wheel-vignette';

    var titleWrap = document.createElement('div');
    titleWrap.className = 'wheel-title';
    var tb = document.createElement('b');
    tb.textContent = '全部笔记';
    titleWrap.appendChild(tb);

    var close = document.createElement('button');
    close.type = 'button';
    close.className = 'wheel-close';
    close.setAttribute('aria-label', '退出动效模式');
    var ns = 'http://www.w3.org/2000/svg';
    var csvg = document.createElementNS(ns, 'svg');
    csvg.setAttribute('viewBox', '0 0 16 16');
    csvg.setAttribute('width', '15');
    csvg.setAttribute('height', '15');
    csvg.setAttribute('aria-hidden', 'true');
    var cpath = document.createElementNS(ns, 'path');
    cpath.setAttribute('d', 'M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.75.75 0 1 1 1.06 1.06L9.06 8l3.22 3.22a.75.75 0 1 1-1.06 1.06L8 9.06l-3.22 3.22a.75.75 0 0 1-1.06-1.06L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z');
    csvg.appendChild(cpath);
    close.appendChild(csvg);

    wheelRoot.appendChild(trackInner);
    wheelRoot.appendChild(trackOuter);
    wheelRoot.appendChild(vignette);
    wheelRoot.appendChild(stage);
    wheelRoot.appendChild(titleWrap);
    wheelRoot.appendChild(close);

    var state = {
      stage: stage,
      titleWrap: titleWrap,
      vignette: vignette,
      trackInner: trackInner,
      trackOuter: trackOuter,
      inner: [],
      outer: [],
      rInner: 0,
      rOuter: 0,
      card: 0,
      cx: 0,
      cy: 0,
      angleInner: 0,
      angleOuter: 0,
      totalOuter: 0,
      lapDone: [],
      wheelVel: 0,
      hoverFactor: 1,
      hoverCount: 0,
      raf: 0,
      running: false
    };

    close.addEventListener('click', function () { setView('cover'); });

    // A bubble below a card near the bottom of the viewport would leave the screen, so it is
    // flipped above. This cannot be done in CSS: the decision depends on where the wheel has
    // carried the card, which changes every frame.
    //
    // 靠近视口底部的卡片，其下方气泡会跑出屏幕，因此翻到上方。这不能用 CSS 做：
    // 判断依据是轮子把卡片带到了哪里，而那个位置每帧都在变。
    function flipTip(a) {
      var r = a.getBoundingClientRect();
      if (r.top + r.height * 0.62 > window.innerHeight * 0.72) {
        a.classList.add('tip-above');
      } else {
        a.classList.remove('tip-above');
      }
    }

    function makeCard(entry) {
      var a = document.createElement('a');
      a.className = 'wheel-card';
      a.href = '/blog/' + entry.s;
      a.setAttribute('draggable', 'false');

      a.appendChild(coverFrame(entry, true));

      if (!entry.c || !entry.c.url) {
        a.classList.add('wheel-card--blank');
      }

      var lbl = document.createElement('div');
      lbl.className = 'wheel-label';
      lbl.textContent = entry.t || '';
      a.appendChild(lbl);

      var tip = document.createElement('div');
      tip.className = 'wheel-tip';
      tip.textContent = entry.t || '';
      a.appendChild(tip);

      a.addEventListener('mouseenter', function () {
        state.hoverCount++;
        flipTip(a);
      });
      a.addEventListener('mouseleave', function () {
        state.hoverCount = Math.max(0, state.hoverCount - 1);
      });
      a.addEventListener('focus', function () { state.hoverCount++; flipTip(a); });
      a.addEventListener('blur', function () { state.hoverCount = Math.max(0, state.hoverCount - 1); });

      return a;
    }

    function buildRing() {
      while (state.stage.firstChild) { state.stage.removeChild(state.stage.firstChild); }
      state.inner = [];
      state.outer = [];
      state.lapDone = [];
      state.totalOuter = 0;

      var n = Math.min(INNER_SLOTS, entries.length);
      for (var i = 0; i < n; i++) {
        var el = makeCard(entries[i]);
        state.stage.appendChild(el);
        state.inner.push({ el: el, offset: (i / n) * TAU, index: i, hoverT: 0 });
      }

      var outerCount = Math.min(OUTER_SLOTS, Math.max(0, entries.length - n));
      for (var j = 0; j < outerCount; j++) {
        var idx = n + j;
        var el2 = makeCard(entries[idx % entries.length]);
        state.stage.appendChild(el2);
        state.outer.push({ el: el2, offset: (j / outerCount) * TAU, index: idx % entries.length, slot: j, hoverT: 0 });
        state.lapDone[j] = -1;
      }
    }

    function measure() {
      var vw = window.innerWidth;
      var vh = window.innerHeight;
      var base = Math.min(vw, vh);

      // Geometry straight from the reference page: inner ring at 0.38*base, outer at 0.62*base.
      //
      // The wheel is meant to be larger than the viewport — that is the whole effect. At
      // 1440x900 the outer ring is 558 px in radius, so it runs 108 px past the top and bottom
      // edge and the reader sees an arc of a much bigger machine rather than a circle that was
      // carefully shrunk to fit. My first version solved for "nothing may leave the screen", got
      // a 726 px circle in a 900 px window, and read as a sparse scatter of pale tiles.
      //
      // 几何直接取自参照页：内圈 0.38*base、外圈 0.62*base。
      //
      // 轮子本来就该大于视口——效果正在于此。1440x900 下外圈半径 558px，
      // 上下各越出 108px，读者看到的是一台更大的机器的弧，而不是一个被小心裁到刚好装下的圆。
      // 我的第一版按「任何东西都不许出屏」去解，得到 900 高的窗口里一个 726 的圆，
      // 效果是一片稀疏的浅色块。
      state.cx = vw / 2;
      state.cy = vh / 2;
      state.rInner = base * 0.38;
      state.rOuter = base * 0.62;

      // The reference hardcodes 130 px cards, but its outer ring carries ten of them on a 669 px
      // radius — a 420 px neighbour gap. Ours carries fourteen on 558, so at 130 px the gaps stay
      // 119 px wide and every card reads as an isolated tile instead of a gondola on a wheel.
      // 0.19*base puts neighbours 250 px apart with 171 px cards, which is the density the
      // reference actually has once the slot counts are equalised.
      //
      // 参照页把卡片写死 130px，但它的外圈是在 669px 半径上放十张——相邻间隙 420px。
      // 我们是 558 半径放十四张，卡在 130px 时间隙仍有 119px，
      // 每张都读成一块孤立的瓷砖，而不是轮子上的吊舱。
      // 0.19*base 让相邻中心距 250px、卡片 171px，把槽位数拉平之后这才是参照页真正的密度。
      var c = Math.max(96, Math.min(196, base * 0.19));
      state.card = c;

      // 580 px in the reference, scaled the same way. It must stay inside the inner ring or it
      // washes the inner cards out — which is exactly what happened when it was sized by viewport
      // alone and came out wider than the whole ring.
      // 参照页是 580px，按同样的方式缩放。它必须留在内圈里面，否则会把内圈卡片洗白——
      // 之前按视口取值、算出比整个内圈还宽的结果，正是这个后果。
      var vg = Math.min(base * 0.54, state.rInner * 2 - c);
      vg = Math.max(160, vg);
      state.vignette.style.width = Math.round(vg) + 'px';
      state.vignette.style.height = Math.round(vg) + 'px';
      state.titleWrap.style.maxWidth = Math.round(vg * 0.92) + 'px';

      // Guide circles follow the two radii exactly.
      // 轨道虚线跟着两个半径走。
      state.trackInner.style.width = Math.round(state.rInner * 2) + 'px';
      state.trackInner.style.height = Math.round(state.rInner * 2) + 'px';
      state.trackOuter.style.width = Math.round(state.rOuter * 2) + 'px';
      state.trackOuter.style.height = Math.round(state.rOuter * 2) + 'px';

      var i, it;
      for (i = 0; i < state.inner.length; i++) {
        it = state.inner[i];
        it.el.style.width = c + 'px';
        it.el.style.height = c + 'px';
      }
      for (i = 0; i < state.outer.length; i++) {
        it = state.outer[i];
        it.el.style.width = c + 'px';
        it.el.style.height = c + 'px';
      }
    }

    // Outer-ring refill: each slot advances to the next note while it is at the bottom of the
    // circle, where the vignette and the other cards cover the change. One lap is one advance —
    // swapping on every pass through the bottom would replace each card fourteen times per turn.
    //
    // 外圈补位：每个槽位在转到圆的下方时，换成下一篇还没上场的笔记，那里有中心遮罩和别的卡片
    // 遮着这次替换。一圈只前进一步——每次经过下方都换，会让每张卡片每圈被替换十四次。
    function swapSlot(slot) {
      var n = Math.min(INNER_SLOTS, entries.length);
      var pool = entries.length - n;
      // Nothing to rotate through: with one ring's worth of outer notes the wheel already shows
      // every one of them.
      // 没有可轮换的余地：外圈笔记只有一屏的量时，轮子已经把它们全摆出来了。
      if (pool <= OUTER_SLOTS) { return; }

      var rel = (slot.index - n + OUTER_SLOTS) % pool;
      var nextIndex = n + rel;
      if (nextIndex === slot.index) { return; }

      slot.index = nextIndex;
      var entry = entries[nextIndex % entries.length];

      var fresh = makeCard(entry);
      fresh.style.width = state.card + 'px';
      fresh.style.height = state.card + 'px';
      state.stage.replaceChild(fresh, slot.el);
      slot.el = fresh;
      slot.hoverT = 0;
    }

    function tick() {
      if (!state.running) { return; }

      var target = state.hoverCount > 0 ? 0 : 1;
      state.hoverFactor += (target - state.hoverFactor) * 0.09;
      if (Math.abs(target - state.hoverFactor) < 0.0008) { state.hoverFactor = target; }

      state.wheelVel *= FRICTION;
      if (Math.abs(state.wheelVel) < 1e-6) { state.wheelVel = 0; }

      var omega = (BASE_SPIN + state.wheelVel) * state.hoverFactor;

      state.angleInner = (state.angleInner - omega) % TAU;
      state.angleOuter = (state.angleOuter + omega) % TAU;
      // Unwrapped accumulator: the swap guard needs to know which full turn the ring is on, and
      // the modulo value cannot answer that.
      // 不取模的累计量：换批守卫需要知道外圈正处在第几圈，而取模后的值回答不了这个问题。
      state.totalOuter += omega;
      var lap = Math.floor(state.totalOuter / TAU);

      var i, it, ang, px, py, lift, scale, hoverTarget;
      for (i = 0; i < state.inner.length; i++) {
        it = state.inner[i];
        ang = it.offset + state.angleInner;
        px = state.cx + state.rInner * Math.cos(ang);
        py = state.cy + state.rInner * Math.sin(ang);
        hoverTarget = it.el.matches(':hover') ? 1 : 0;
        it.hoverT += (hoverTarget - it.hoverT) * 0.16;
        if (Math.abs(it.hoverT - hoverTarget) < 0.001) { it.hoverT = hoverTarget; }
        lift = 9 * it.hoverT;
        scale = 1 + 0.065 * it.hoverT;
        it.el.style.transform = 'translate3d(' + (px - state.card / 2) + 'px,' + (py - state.card / 2 - lift) + 'px,0) scale(' + scale + ')';
      }
      for (i = 0; i < state.outer.length; i++) {
        it = state.outer[i];
        ang = it.offset + state.angleOuter;
        px = state.cx + state.rOuter * Math.cos(ang);
        py = state.cy + state.rOuter * Math.sin(ang);
        hoverTarget = it.el.matches(':hover') ? 1 : 0;
        it.hoverT += (hoverTarget - it.hoverT) * 0.16;
        if (Math.abs(it.hoverT - hoverTarget) < 0.001) { it.hoverT = hoverTarget; }
        lift = 9 * it.hoverT;
        scale = 1 + 0.065 * it.hoverT;
        it.el.style.transform = 'translate3d(' + (px - state.card / 2) + 'px,' + (py - state.card / 2 - lift) + 'px,0) scale(' + scale + ')';

        if (py > state.cy + state.rOuter * 0.66 && state.lapDone[it.slot] !== lap) {
          state.lapDone[it.slot] = lap;
          swapSlot(it);
        }
      }

      state.raf = window.requestAnimationFrame(tick);
    }

    function onWheel(e) {
      if (!state.running) { return; }
      var d = e.deltaY;
      if (e.deltaMode === 1) { d *= 16; }
      else if (e.deltaMode === 2) { d *= window.innerHeight; }
      state.wheelVel += d * WHEEL_GAIN;
      if (state.wheelVel > MAX_WHEEL) { state.wheelVel = MAX_WHEEL; }
      if (state.wheelVel < -MAX_WHEEL) { state.wheelVel = -MAX_WHEEL; }
    }

    function onKey(e) {
      if (!state.running) { return; }
      if (e.key === 'Escape' || e.key === 'Esc') {
        e.preventDefault();
        setView('cover');
      }
    }

    function onResize() {
      if (!state.running) { return; }
      measure();
    }

    state.start = function () {
      if (state.running) { return; }
      buildRing();
      measure();
      state.running = true;
      state.hoverCount = 0;
      state.hoverFactor = 1;
      state.wheelVel = 0;
      window.addEventListener('wheel', onWheel, { passive: true });
      document.addEventListener('keydown', onKey);
      window.addEventListener('resize', onResize);
      state.raf = window.requestAnimationFrame(tick);
    };

    state.stop = function () {
      state.running = false;
      if (state.raf) { window.cancelAnimationFrame(state.raf); state.raf = 0; }
      window.removeEventListener('wheel', onWheel);
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', onResize);
      state.hoverCount = 0;
    };

    return state;
  }

  function setView(view, opts) {
    if (view === 'wheel' && isMobile()) { view = 'list'; }
    var silent = opts && opts.silent;

    current = view;

    for (var i = 0; i < buttons.length; i++) {
      var on = buttons[i].getAttribute('data-view') === view;
      buttons[i].classList.toggle('is-on', on);
      buttons[i].setAttribute('aria-pressed', on ? 'true' : 'false');
    }

    if (view === 'cover') { buildCoverGrid(); }

    listBody.hidden = view !== 'list';
    coverBody.hidden = view !== 'cover';

    if (view === 'wheel') {
      if (!wheel) { wheel = createWheel(); }
      wheelRoot.hidden = false;
      root.classList.add('wheel-open');
      void wheelRoot.offsetWidth;
      wheelRoot.classList.add('is-in');
      wheel.start();
    } else {
      root.classList.remove('wheel-open');
      if (wheel) {
        wheel.stop();
        wheelRoot.classList.remove('is-in');
        window.setTimeout(function () {
          if (current !== 'wheel') { wheelRoot.hidden = true; }
        }, 420);
      }
    }

    if (!silent) { store(view); }
  }

  switchEl.addEventListener('click', function (e) {
    var btn = e.target.closest ? e.target.closest('.vs-btn') : null;
    if (!btn) { return; }
    var v = btn.getAttribute('data-view');
    if (!v || v === current) { return; }
    setView(v);
  });

  // Restore the previous choice, except on mobile where the switch is not offered at all.
  // 恢复上次的选择，但移动端例外——那里根本不提供这个开关。
  var initial = readStored();
  if (initial !== 'list' && !isMobile()) {
    setView(initial, { silent: true });
  }

  // Following the breakpoint across a rotation or a window drag: the wheel cannot survive being
  // squeezed into a phone width, and the cover grid would turn into a one-column wall.
  // 跟随断点跨越旋转或窗口拖动：同心轮经不起被压进手机宽度，封面网格则会变成一列长墙。
  var mq = window.matchMedia(MOBILE_QUERY);
  var onMq = function () {
    if (mq.matches && current !== 'list') {
      setView('list', { silent: true });
    }
  };
  if (mq.addEventListener) { mq.addEventListener('change', onMq); }
  else if (mq.addListener) { mq.addListener(onMq); }
})();
`
