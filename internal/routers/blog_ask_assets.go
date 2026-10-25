package routers

// Blog Q&A front-end. Served from /blog/ask.js so both blog templates can pull in the same
// component without duplicating a few hundred lines of inline script.
//
// Design notes:
//   - POST + ReadableStream rather than EventSource: EventSource cannot send a JSON body, and
//     the question belongs in the body, not the query string.
//   - The panel is mounted lazily into #ask-root, which the templates render, so the pages stay
//     readable (and fully functional) with JavaScript disabled.
//   - Reduced-motion users get the same information without the entrance animations.
//
// 博客问答前端。由 /blog/ask.js 提供，两个博客模板共用同一组件，避免重复几百行内联脚本。
//
// 设计要点：
//   - 使用 POST + ReadableStream 而非 EventSource：EventSource 无法发送 JSON 请求体，
//     而问题应当放在请求体里，不该出现在查询串中。
//   - 面板延迟挂载到模板渲染出的 #ask-root，因此禁用 JavaScript 时页面依旧可读、可用。
//   - 偏好减少动态效果的用户会拿到同样的信息，只是没有入场动画。
const blogAskScript = `
(function () {
  'use strict';
  var host = document.getElementById('ask-root');
  if (!host || !window.fetch) return;

  var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // 微信内置浏览器（iOS 用 WKWebView、Android 用 X5）键盘弹出时不改变布局视口，
  // 也不给 visualViewport 有效高度，而 position:fixed 又不跟随键盘——
  // 所以那套「钉在视觉视口上」的办法在微信里必然失败。
  //
  // 微信里改成：面板回到文档流，让原生「把聚焦元素滚进视野」的行为接管，
  // 并在聚焦后主动滚到底，保证输入框一定在键盘上方。
  //
  // WeChat's in-app browser keeps the layout viewport unchanged when the keyboard opens and
  // gives no usable visualViewport height, while position:fixed does not follow the keyboard.
  // Pinning to the visual viewport therefore cannot work there. Fall back to the document flow
  // and let the native "scroll focused element into view" behaviour do the job.
  var inWeChat = /MicroMessenger/i.test(navigator.userAgent);
  if (inWeChat) document.documentElement.classList.add('is-wx');
  var OPEN = false, BUSY = false, CTRL = null;


  // ── Build the shell ────────────────────────────────────────────────────────
  host.innerHTML =
    '<button class="ask-fab" type="button" aria-label="向知识库提问" aria-expanded="false">' +
      '<span class="ask-fab-ico" aria-hidden="true">' +
        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">' +
          '<path d="M12 3a7 7 0 0 1 7 7c0 2-1 3-1 3l1 4h-5a7 7 0 1 1-2-14z"/>' +
          '<circle cx="9.5" cy="10" r=".9" fill="currentColor" stroke="none"/>' +
          '<circle cx="14.5" cy="10" r=".9" fill="currentColor" stroke="none"/>' +
        '</svg>' +
      '</span>' +
      '<span class="ask-fab-txt">提问</span>' +
    '</button>' +
    '<div class="ask-backdrop" hidden></div>' +
    '<section class="ask-panel" role="dialog" aria-modal="false" aria-label="知识库问答" hidden>' +
      '<header class="ask-head">' +
        '<div class="ask-title">' +
          '<span class="ask-dot" aria-hidden="true"></span>' +
          '<span>知识库问答</span>' +
          '<span class="ask-sub">基于已发布笔记</span>' +
        '</div>' +
        '<button class="ask-close" type="button" aria-label="关闭">' +
          '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>' +
        '</button>' +
      '</header>' +
      '<div class="ask-body" id="ask-body">' +
        '<div class="ask-intro">' +
          '<p class="ask-intro-t">可以问什么</p>' +
          '<div class="ask-chips">' +
            '<button type="button" class="ask-chip">切牙的形态特点有哪些</button>' +
            '<button type="button" class="ask-chip">牙体缺损的修复方式</button>' +
            '<button type="button" class="ask-chip">种植学基础包括什么</button>' +
          '</div>' +
        '</div>' +
      '</div>' +
      '<form class="ask-form" autocomplete="off">' +
        '<div class="ask-field">' +
          '<textarea class="ask-input" rows="1" placeholder="输入问题，Enter 发送，Shift+Enter 换行" maxlength="500"></textarea>' +
          '<button class="ask-send" type="submit" aria-label="发送">' +
            '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h13M12 5l7 7-7 7"/></svg>' +
          '</button>' +
        '</div>' +
        '<p class="ask-hint">回答由模型依据笔记片段生成，请以原文为准</p>' +
      '</form>' +
    '</section>';

  var fab      = host.querySelector('.ask-fab');

  var panel    = host.querySelector('.ask-panel');
  var backdrop = host.querySelector('.ask-backdrop');
  var body     = host.querySelector('#ask-body');
  var form     = host.querySelector('.ask-form');
  var input    = host.querySelector('.ask-input');
  var sendBtn  = host.querySelector('.ask-send');
  var closeBtn = host.querySelector('.ask-close');

  // ── 外部入口 ────────────────────────────────────────────────────────────────
  // 首页窄屏的入口在模板里（底部那条 dock 的三个点：爻路／目录／问道），不在这个脚本里。
  // 这里只提供一条通用约定：任何带 data-ask-open 的元素都能打开面板。
  // 于是入口可以是按钮、可以是链接，脚本不必知道它在哪、长什么样 ——
  // 之前这里动态造过一个悬浮钮，并在首页把它塞进顶栏；顶栏是拇指最难够到的位置，
  // 而底部那条三点已经给了问答一个合适的地方。
  //
  // The home page's entry now lives in the template. This script only publishes the contract:
  // any element carrying data-ask-open opens the panel.
  var openers = document.querySelectorAll('[data-ask-open]');
  Array.prototype.forEach.call(openers, function (el) {
    el.addEventListener('click', function (e) {
      e.preventDefault();
      open();
      el.setAttribute('aria-expanded', 'true');
    });
  });

  // ── Open / close ───────────────────────────────────────────────────────────
  function open() {
    if (OPEN) return;
    OPEN = true;
    backdrop.hidden = false;
    panel.hidden = false;
    fab.setAttribute('aria-expanded', 'true');
    host.classList.add('ask-on');
    bindViewport();
    syncViewport();
    // Two frames so the element is laid out before the transition starts.
    requestAnimationFrame(function () { requestAnimationFrame(function () {
      host.classList.add('ask-in');
    }); });
    setTimeout(function () { input.focus(); }, reduce ? 0 : 180);
  }
  function close() {
    if (!OPEN) return;
    OPEN = false;
    host.classList.remove('ask-in');
    fab.setAttribute('aria-expanded', 'false');
    // transitionend can be missed entirely (interrupted transition, or no property actually
    // changing), and the cleanup below is what re-enables the floating button — so it must not
    // depend on that event alone.
    var settled = false;
    var guard = 0;
    var done = function () {
      if (settled) return;
      settled = true;
      clearTimeout(guard);
      unbindViewport();
      panel.hidden = true; backdrop.hidden = true; host.classList.remove('ask-on');
      // 外部入口的展开态也要收回去，否则读屏软件会一直认为面板还开着。
      Array.prototype.forEach.call(openers, function (el) { el.setAttribute('aria-expanded', 'false'); });
      panel.removeEventListener('transitionend', done);
    };
    if (reduce) { done(); return; }
    guard = setTimeout(done, 420);
    panel.addEventListener('transitionend', done);
  }
  fab.addEventListener('click', function () { OPEN ? close() : open(); });
  closeBtn.addEventListener('click', close);
  backdrop.addEventListener('click', close);
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && OPEN) close();
  });

  // ── Soft-keyboard avoidance ────────────────────────────────────────────────
  //
  // On mobile the panel is bottom-anchored at 88vh. A soft keyboard shrinks the *visual*
  // viewport while leaving the layout viewport — and therefore vh units and fixed positioning —
  // untouched, so the panel's lower half ends up behind the keyboard and the input field is
  // half hidden.
  //
  // The panel is therefore pinned to the visual viewport while it is open: height follows the
  // visible area, and bottom is pushed up by however much the keyboard occludes. Desktop keeps
  // the untouched CSS layout because the media query gate below never lets the inline styles
  // apply.
  //
  // 移动端软键盘弹出时收缩的是视觉视口，而布局视口——以及依赖它的 vh 单位与固定定位——不动，
  // 于是面板下半部分落到键盘后面，输入框被盖掉一半。
  //
  // 因此面板打开期间钉在视觉视口上：高度跟随可见区域，底边按键盘遮挡量顶起。
  // 桌面端由下面的媒体查询守卫拦住，内联样式不会生效，仍走原 CSS 布局。
  var viewport = window.visualViewport;
  var viewportRaf = 0;
  var narrowQuery = window.matchMedia ? window.matchMedia('(max-width:860px)') : null;

  function syncViewport() {
    if (!OPEN || !viewport) return;
    if (viewportRaf) return;
    viewportRaf = requestAnimationFrame(function () {
      viewportRaf = 0;
      if (!OPEN) return;
      if (narrowQuery && !narrowQuery.matches) {
        panel.style.height = '';
        panel.style.bottom = '';
        return;
      }
      // Pin the panel to the visual viewport: top = how far the page was pushed up,
      // height = what is actually visible. The earlier version set only height and bottom,
      // then scrolled the transcript to the end — with the keyboard up those three add up and
      // hand the panel a negative top, which is why the input box vanished past the edge.
      //
      // 面板直接钉在视觉视口上：top 取页面被顶起的量，height 取真正可见的高度。
      // 旧写法只设 height 与 bottom、再把对话滚到底——键盘弹起时三者相加会算出负的 top，
      // 输入框就是这么被挤出屏幕的。
      // 取两者较小值：键盘升降过程中 visualViewport 与 innerHeight 会短暂不同步，
      // 用较大的那个就会把面板底部撑到键盘下面去。
      //
      // Take the smaller of the two: during the keyboard animation visualViewport and
      // innerHeight disagree for a frame or two, and trusting the larger one leaves the
      // panel's bottom edge sitting behind the keyboard.
      var visH = viewport ? viewport.height : window.innerHeight;
      var visT = viewport ? viewport.offsetTop : 0;
      var h = Math.min(visH, window.innerHeight);
      panel.style.top = Math.max(0, Math.round(visT)) + 'px';
      panel.style.height = Math.round(h) + 'px';
      panel.style.bottom = 'auto';
    });
  }

  // iOS 只在键盘动画结束后补发一次 resize，动画期间拿到的高度仍是旧值。
  // 所以聚焦后自己轮询 1.5 秒，把中间那几帧补上；稳定后自动停。
  //
  // iOS dispatches visualViewport resize only after the keyboard animation settles, so
  // reading during the animation gives the old height. Poll for 1.5s after focus to catch
  // the intermediate frames, then stop.
  var pollTimer = 0;
  function startViewportPoll() {
    if (pollTimer) return;
    var until = Date.now() + 1500;
    pollTimer = setInterval(function () {
      syncViewport();
      if (Date.now() > until) { clearInterval(pollTimer); pollTimer = 0; }
    }, 120);
  }
  function stopViewportPoll() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = 0; }
  }

  function bindViewport() {
    if (!viewport) return;
    viewport.addEventListener('resize', syncViewport);
    viewport.addEventListener('scroll', syncViewport);
    window.addEventListener('resize', syncViewport);
    window.addEventListener('orientationchange', syncViewport);
    startViewportPoll();
  }
  function unbindViewport() {
    stopViewportPoll();
    if (viewport) {
      viewport.removeEventListener('resize', syncViewport);
      viewport.removeEventListener('scroll', syncViewport);
    }
    window.removeEventListener('resize', syncViewport);
    window.removeEventListener('orientationchange', syncViewport);
    if (viewportRaf) { cancelAnimationFrame(viewportRaf); viewportRaf = 0; }
    panel.style.height = '';
    panel.style.bottom = '';
    panel.style.top = '';
  }

  // ── Auto-grow textarea ─────────────────────────────────────────────────────
  function grow() {
    input.style.height = 'auto';
    input.style.height = Math.min(input.scrollHeight, 132) + 'px';
  }
  // 微信里无法感知键盘高度，只能把面板整体压矮到键盘上方：聚焦时降到 46vh，
  // iPhone 的键盘约占 40% 屏高，这样输入框稳定落在键盘之上。
  // 绝不能滚页面——面板一打开就 focus，滚到底会让「点一下入口」变成「跳到页尾」。
  //
  // WeChat gives no keyboard height, so the panel itself gets shorter while typing rather than
  // scrolling the page — the panel focuses its input on open, and scrolling there would turn a
  // tap on the entry into a jump to the bottom of the page.
  function setWeChatTyping(on) {
    if (!inWeChat) return;
    document.documentElement.classList.toggle('is-wx-typing', !!on);
    syncViewport();
  }

  input.addEventListener('focus', function () {
    startViewportPoll();
    setWeChatTyping(true);
  });
  input.addEventListener('blur', function () {
    startViewportPoll();
    setWeChatTyping(false);
  });
  input.addEventListener('input', grow);
  input.addEventListener('keydown', function (e) {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); form.requestSubmit(); }
  });

  host.addEventListener('click', function (e) {
    var chip = e.target.closest && e.target.closest('.ask-chip');
    if (!chip) return;
    input.value = chip.textContent.trim();
    grow();
    form.requestSubmit();
  });

  // ── Markdown-lite for streamed answers ─────────────────────────────────────
  // Deliberately tiny: the model is instructed to answer in short paragraphs and bullets, so
  // this covers the shapes that actually appear while keeping raw HTML out of the DOM.
  function esc(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }
  function render(text) {
    var lines = esc(text).split(/\n/);
    var out = [], list = null, items = [];

    for (var i = 0; i < lines.length; i++) {
      var ln = lines[i];
      if (/^\s*[-*+]\s+/.test(ln)) { list = 'ul'; items.push('<li>' + ln.replace(/^\s*[-*+]\s+/, '') + '</li>'); continue; }
      if (/^\s*\d+[.)]\s+/.test(ln)) { list = 'ol'; items.push('<li>' + ln.replace(/^\s*\d+[.)]\s+/, '') + '</li>'); continue; }
      if (items.length) { out.push('<' + list + '>' + items.join('') + '</' + list + '>'); items = []; list = null; }
      if (/^\s*#{1,6}\s+/.test(ln)) { out.push('<p class="ask-h">' + ln.replace(/^\s*#{1,6}\s+/, '') + '</p>'); continue; }
      if (ln.trim() === '') { continue; }
      out.push('<p>' + ln + '</p>');
    }
    if (items.length) { out.push('<' + list + '>' + items.join('') + '</' + list + '>'); }
    var html = out.join('');
    // [1] [2] -> citation chips
    html = html.replace(/\[(\d{1,2})\]/g, '<button class="ask-cite" type="button" data-cite="$1">$1</button>');
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    return html;
  }

  // ── Message scaffolding ────────────────────────────────────────────────────
  function clearIntro() {
    var intro = body.querySelector('.ask-intro');
    if (intro) intro.remove();
  }
  function addUser(text) {
    var el = document.createElement('div');
    el.className = 'ask-msg ask-me';
    el.innerHTML = '<div class="ask-bubble">' + esc(text).replace(/\n/g, '<br>') + '</div>';
    body.appendChild(el);
    scroll();
    return el;
  }
  function addBot() {
    var el = document.createElement('div');
    el.className = 'ask-msg ask-bot';
    el.innerHTML =
      '<div class="ask-trace" hidden>' +
        '<div class="ask-fold">' +
          '<div class="ask-steps"></div>' +
          '<div class="ask-sources"></div>' +
        '</div>' +
      '</div>' +
      '<div class="ask-bubble ask-answer"></div>' +
      '<div class="ask-cites" hidden></div>' +
      '<div class="ask-acts" hidden></div>';
    body.appendChild(el);
    scroll();
    return el;
  }

  // Retry / stop controls, rendered after the answer.
  // The question is remembered per message so "again" does not need the input box.
  function showActions(bot, question, state) {
    var box = bot.querySelector('.ask-acts');
    box.hidden = false;
    box.innerHTML = '';

    function mk(label, cls, handler) {
      var b = document.createElement('button');
      b.type = 'button';
      b.className = 'ask-act ' + cls;
      b.innerHTML = label;
      b.addEventListener('click', handler);
      box.appendChild(b);
      return b;
    }

    var retryIco = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-2.6-6.4"/><path d="M21 3v6h-6"/></svg>';
    var stopIco = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"><rect x="7" y="7" width="10" height="10" rx="1.5"/></svg>';

    if (state === 'streaming') {
      mk(stopIco + '<span>停止</span>', 'is-stop', function () {
        if (CTRL) CTRL.abort();
      });
      return;
    }
    mk(retryIco + '<span>' + (state === 'failed' ? '重试' : '重新生成') + '</span>',
       state === 'failed' ? 'is-retry' : '', function () {
      // Drop this exchange and ask the same question again, so the panel shows one answer
      // rather than a growing stack of duplicates.
      var user = bot.previousElementSibling;
      bot.remove();
      if (user && user.classList.contains('ask-me')) user.remove();
      send(question);
    });
  }
  // Follow the stream only while the reader is already at the bottom. Yanking the view down on
  // every delta makes it impossible to scroll back and re-read mid-answer.
  //
  // 只在读者本来贴着底部时才跟随。每来一段就强行拉到底，就没法在回答中途往回翻着看。
  var FOLLOW = true;
  body.addEventListener('scroll', function () {
    FOLLOW = body.scrollHeight - body.scrollTop - body.clientHeight < 96;
  }, { passive: true });

  function scroll() {
    if (FOLLOW) body.scrollTop = body.scrollHeight;
  }

  // ── Retrieval trace ────────────────────────────────────────────────────────
  // 一套统一的手绘线性图标：24×24、1.8 描边、圆头圆角、纯描边无填充。
  // 之前几个图标混了 fill 与复杂造型（链条 vs 实心圆眼），并排放着像两套东西。
  //
  // One consistent line set: 24×24, 1.8 stroke, round caps, stroke-only. The previous mix
  // (a chain glyph next to a filled-circle face) read as two different icon families.
  function ico(d) {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" ' +
           'stroke-linecap="round" stroke-linejoin="round">' + d + '</svg>';
  }
  var STAGE_ICON = {
    // 问号 —— 先弄清楚问的是什么
    question: ico('<circle cx="12" cy="12" r="9"/><path d="M9.2 9.4a2.9 2.9 0 1 1 3.9 2.7c-.7.3-1.1.9-1.1 1.6v.3"/><path d="M12 17.2v.1"/>'),
    // 放大镜 —— 翻箱倒柜地找
    search:   ico('<circle cx="10.5" cy="10.5" r="6.5"/><path d="m20 20-4.7-4.7"/>'),
    // 灯泡 —— 想怎么讲
    idea:     ico('<path d="M9 18.5h6"/><path d="M10 21.5h4"/><path d="M12 2.5a6.5 6.5 0 0 0-3.9 11.7c.5.4.9.9.9 1.5v.8h6v-.8c0-.6.4-1.1.9-1.5A6.5 6.5 0 0 0 12 2.5z"/>'),
    // 漏斗 —— 从一堆里筛出几篇
    funnel:   ico('<path d="M4 5h16l-6.2 7.3v6.2L10.2 20v-7.7z"/>'),
    // 眼镜 —— 正在看这几篇
    eye:      ico('<circle cx="6.5" cy="14.5" r="3.5"/><circle cx="17.5" cy="14.5" r="3.5"/><path d="M10 14.5h4"/><path d="M3.2 11.6 4.8 7.6"/><path d="M20.8 11.6 19.2 7.6"/>'),
    // 铅笔 —— 开始写
    pencil:   ico('<path d="M4 20h3.6L20 7.6a2.5 2.5 0 0 0-3.6-3.6L4 16.4z"/><path d="m14.4 5.6 4 4"/>'),
    // 闪电 —— 由谁接手（保留原造型）
    bolt:     ico('<path d="M13 2 3 14h7l-1 8 10-12h-7l1-8z"/>'),
    // 分叉 —— 换一条路
    fork:     ico('<circle cx="6" cy="5" r="2"/><circle cx="6" cy="19" r="2"/><circle cx="18" cy="12" r="2"/><path d="M8 5h4a4 4 0 0 1 4 4v1"/><path d="M8 19h4a4 4 0 0 0 4-4v-1"/>'),
    // 火焰 —— 正在努力烧烤
    flame:    ico('<path d="M12 21c3.6 0 6-2.4 6-5.8 0-4.3-3.8-6.2-3.8-10.4-2.9 1.9-4.3 4.3-4.3 6.7 0-1-.7-1.9-1.4-2.9C7.3 10.1 6 12.4 5.8 15.2 5.8 18.6 8.4 21 12 21z"/>'),
    // 眩晕 —— 被知识砸晕
    dizzy:    ico('<circle cx="12" cy="12" r="9"/><path d="M8.6 10.2h.01"/><path d="M15.4 10.2h.01"/><path d="M8.2 15.4c1.4-1.2 2.9-1.2 4.2 0 1.3 1.2 2.8 1.2 4.2 0"/>'),
    // 生气 —— 被拖下工作台
    angry:    ico('<circle cx="12" cy="12" r="9"/><path d="M8.3 9.1 10 10.5"/><path d="M15.7 9.1 14 10.5"/><path d="M9.2 16.2c1.7-1.4 3.9-1.4 5.6 0"/>'),
    // 叹号 —— 出岔子
    alert:    ico('<circle cx="12" cy="12" r="9"/><path d="M12 7.5v5.5"/><path d="M12 16.6v.1"/>'),
    think:    ico('<path d="M9 18.5h6"/><path d="M10 21.5h4"/><path d="M12 2.5a6.5 6.5 0 0 0-3.9 11.7c.5.4.9.9.9 1.5v.8h6v-.8c0-.6.4-1.1.9-1.5A6.5 6.5 0 0 0 12 2.5z"/>'),
    link:     ico('<path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/>'),
    route:    ico('<path d="M13 2 3 14h7l-1 8 10-12h-7l1-8z"/>')
  };
  function step(trace, kind, text) {
    trace.hidden = false;
    var steps = trace.querySelector('.ask-steps');
    var prev = steps.querySelector('.ask-step.is-live');
    if (prev) { prev.classList.remove('is-live'); prev.classList.add('is-done'); }
    var el = document.createElement('div');
    el.className = 'ask-step is-live';
    // 后端几十毫秒内就能把八步全推过来，而每条入场动画只有 .55s——
    // 不加错峰，八条会同时刷出，读者根本看不清过程。按插入序号递增延迟，
    // 配合已有的 animation-fill-mode: backwards（延迟期间不可见），就是依次出现。
    //
    // The server pushes every step within tens of milliseconds while each entrance animation
    // is only .55s, so without staggering they all flash in at once. Delay by insertion order;
    // combined with the existing backwards fill mode, they appear one after another.
    var idx = steps.children.length;
    if (!reduce) el.style.animationDelay = (idx * 110) + 'ms';
    el.innerHTML = '<span class="ask-step-ico">' + (STAGE_ICON[kind] || STAGE_ICON.think) + '</span>' +
                   '<span class="ask-step-t">' + esc(text) + '</span>';
    steps.appendChild(el);
    scroll();
    return el;
  }

  // ── SSE over fetch ─────────────────────────────────────────────────────────
  function send(question) {
    if (BUSY) return;
    BUSY = true;
    sendBtn.classList.add('is-busy');
    clearIntro();
    addUser(question);
    var bot = addBot();
    var trace = bot.querySelector('.ask-trace');
    var answer = bot.querySelector('.ask-answer');
    var citesBox = bot.querySelector('.ask-cites');

    var raw = '';
    var cites = [];
    var curStep = null;
    var thinkAt = 0;
    var minimized = false;
    var sourceCount = 0;
    var stream = { committed: 0, tail: null };
    var thoughtState = { buf: '' };
    var failed = false;

    // If the model takes a while to produce its first token, say so rather than looking stuck.
    function armThinkTimer() {
      clearTimeout(thinkAt);
      thinkAt = setTimeout(function () {
        if (!raw) curStep = step(trace, 'idea', '想一下怎么讲你才听得懂…');
      }, 1400);
    }

    CTRL = new AbortController();
    showActions(bot, question, 'streaming');
    fetch('/api/blog/ask', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question: question }),
      signal: CTRL.signal
    }).then(function (res) {
      if (!res.ok) {
        return res.text().then(function (t) {
          // Prefer the server's own sentence over the status line. The endpoint answers
          // 400/404/429 with {"code":..,"message":".."} so the reason ("提问太频繁…") can reach
          // the reader; printing the raw body next to "HTTP 429" would bury it in punctuation.
          //
          // 优先用服务端自己的那句话，而不是状态行。该端点用 {"code":..,"message":".."} 回答
          // 400/404/429，好让原因（「提问太频繁…」）能被读者看到；
          // 把原始响应体印在 "HTTP 429" 旁边只会把它埋进标点里。
          var msg = '';
          try { msg = (JSON.parse(t) || {}).message || ''; } catch (e) {}
          throw new Error(msg || ('HTTP ' + res.status + (t ? '：' + t.slice(0, 160) : '')));
        });
      }
      var reader = res.body.getReader();
      var dec = new TextDecoder('utf-8');
      var buf = '';
      armThinkTimer();

      return (function pump() {
        return reader.read().then(function (r) {
          if (r.done) return;
          buf += dec.decode(r.value, { stream: true });
          var parts = buf.split('\n\n');
          buf = parts.pop();
          for (var i = 0; i < parts.length; i++) {
            var block = parts[i].trim();
            if (block.indexOf('data:') !== 0) continue;
            var ev;
            try { ev = JSON.parse(block.slice(5).trim()); } catch (e) { continue; }
            handle(ev);
          }
          return pump();
        });
      })();
    }).catch(function (err) {
      if (err && err.name === 'AbortError') {
        trace.hidden = false;
        curStep = step(trace, 'alert', '已经喊停');
        if (!raw) answer.innerHTML = '<p class="ask-none">已停止生成。</p>';
        return;
      }
      clearTimeout(thinkAt);
      failed = true;
      trace.hidden = false;
      step(trace, 'think', '出岔子了').classList.add('is-error');
      answer.innerHTML = '<p class="ask-err">' + esc(err.message || String(err)) + '</p>';
      scroll();
    }).then(function () {
      clearTimeout(thinkAt);
      BUSY = false;
      CTRL = null;
      sendBtn.classList.remove('is-busy');
      if (curStep) { curStep.classList.remove('is-live'); curStep.classList.add('is-done'); }
      showActions(bot, question, failed ? 'failed' : 'done');
    });

    function handle(ev) {
      if (ev.type === 'stage') {
        // 图标由服务端在事件里给出；关键词推断只在老事件上兜底。
        // The icon comes from the event; keyword guessing is only a fallback for old events.
        curStep = step(trace, ev.icon || 'think', ev.stage);
        return;
      }
      if (ev.type === 'retrieval') {
        sourceCount = (ev.sources || []).length;
        // renderSources 自己那一行就是这一步，不要再补一条 —— 两条说的是同一件事，
        // 展开思维链时读起来像重复了一次。
        //
        // renderSources already contributes this line; adding another says the same thing twice.
        renderSources(trace, ev.sources || []);
        armThinkTimer();
        return;
      }
      if (ev.type === 'route') {
        curStep = step(trace, 'route', '现在工作由 ' + (ev.route || ev.model) + ' 接手');
        return;
      }
      if (ev.type === 'thinking') {
        clearTimeout(thinkAt);
        if (!minimized) { minimized = true; scheduleMinimize(trace); }
        feedThought(trace, thoughtState, ev.text || '', ev.thoughtChars || 0);
        return;
      }
      if (ev.type === 'delta') {
        clearTimeout(thinkAt);
        if (!minimized) { minimized = true; scheduleMinimize(trace); }
        finishThought(trace);
        raw += ev.text;
        feedAnswer(answer, stream, raw);
        scroll();
        return;
      }
      if (ev.type === 'citations') {
        cites = ev.citations || [];
        return;
      }
      if (ev.type === 'error') {
        failed = true;
        trace.hidden = false;
        step(trace, 'think', '出岔子了').classList.add('is-error');
        if (!raw) answer.innerHTML = '<p class="ask-err">' + esc(ev.message || '未知错误') + '</p>';
        scroll();
        return;
      }
      if (ev.type === 'done') {
        if (!raw) answer.innerHTML = '<p class="ask-none">没有生成内容。</p>';
        // Flush any trailing fragment that never hit a boundary, so the last sentence is not
        // left sitting in the un-animated tail element.
        if (stream.tail) {
          stream.tail.classList.remove('ask-blk-live');
          stream.tail = null;
        }
        renderCites(citesBox, cites);
        // 生成结束时平滑落位。此前是直接改 scrollTop，画面一下子「跳」到末尾；
        // 用 smooth 走完最后一段，读者能看清引文是从哪里长出来的。
        //
        // Glide to the end on completion. Assigning scrollTop snapped the view to the bottom;
        // a smooth scroll lets the reader see where the citations came from.
        if (FOLLOW && body.scrollTo) {
          body.scrollTo({ top: body.scrollHeight, behavior: reduce ? 'auto' : 'smooth' });
        } else {
          scroll();
        }
      }
    }
  }

  // Once answer text starts arriving, collapse the trace to a single summary line.
  // The steps and the source list together run to several hundred pixels, which pushed the
  // answer itself below the fold — the one thing the reader is actually waiting for.
  // 折叠推迟到步骤播完：八步各延迟 110ms、入场 .55s，最后一条大约在 1.3s 落定。
  // 生成一开始就折叠，等于把刚铺垫的过程全收走，这一步白做。
  //
  // Delay the collapse until the staggered steps have landed (last one ~1.3s). Collapsing the
  // instant generation starts throws away the sequence that was just set up.
  var minimizeTimer = 0;
  function scheduleMinimize(trace) {
    if (minimizeTimer) clearTimeout(minimizeTimer);
    minimizeTimer = setTimeout(function () {
      minimizeTimer = 0;
      minimizeTrace(trace, sourceCount);
    }, reduce ? 0 : 900);
  }

  function minimizeTrace(trace, n) {
    trace.classList.add('is-min');
    var sum = document.createElement('button');
    sum.type = 'button';
    sum.className = 'ask-trace-sum';
    sum.setAttribute('aria-expanded', 'false');
    sum.innerHTML =
      '<span class="ask-sum-ico" aria-hidden="true">' + STAGE_ICON.flame + '</span>' +
      '<span class="ask-sum-t">wendao 正在努力烧烤</span>' +
      '<span class="ask-sum-x">火</span>';
    sum.addEventListener('click', function () {
      var open = trace.classList.toggle('is-open');
      sum.setAttribute('aria-expanded', open ? 'true' : 'false');
      sum.querySelector('.ask-sum-x').textContent = open ? '收' : '火';
    });
    trace.insertBefore(sum, trace.firstChild);
    var live = trace.querySelector('.ask-step.is-live');
    if (live) { live.classList.remove('is-live'); live.classList.add('is-done'); }
  }

  // ── Answer streaming: commit the text one phrase at a time ─────────────────
  // Re-rendering the whole answer on every delta made new text appear instantly in one lump,
  // which reads as a hard cut. Instead the text is committed in phrase-sized blocks, each of
  // which plays its own short entrance, so the answer settles in piece by piece.
  //
  // 回答流式渲染：按短语逐块提交。
  // 每个 delta 都整段重绘会让新文字一次性跳出来，观感是硬切。改为按短语大小的块提交，
  // 每块播放自己的入场动画，于是回答是一段一段落定的。
  function blockBoundary(text, from) {
    var MIN = 10;
    for (var i = from + MIN; i < text.length; i++) {
      var ch = text[i];
      if (ch === '\n') return i + 1;
      if ('。！？；'.indexOf(ch) >= 0) return i + 1;
      if (ch === '.' && text[i + 1] === ' ') return i + 1;
    }
    // Long unpunctuated run (tables, long clauses): fall back to the next space so blocks
    // still land instead of stalling until the very end.
    if (text.length - from > 120) {
      for (var j = from + 90; j < text.length; j++) {
        if (/\s/.test(text[j])) return j + 1;
      }
    }
    return from;
  }

  function feedAnswer(answer, st, raw) {
    var end = blockBoundary(raw, st.committed);
    while (end > st.committed) {
      var seg = raw.slice(st.committed, end);
      if (seg.replace(/\s/g, '')) {
        var el = document.createElement('span');
        el.className = 'ask-blk';
        el.innerHTML = render(seg);
        answer.appendChild(el);
      }
      st.committed = end;
      end = blockBoundary(raw, st.committed);
    }
    var tail = raw.slice(st.committed);
    if (tail) {
      if (!st.tail) {
        st.tail = document.createElement('span');
        st.tail.className = 'ask-blk ask-blk-live';
        answer.appendChild(st.tail);
      }
      st.tail.innerHTML = render(tail);
    } else if (st.tail) {
      st.tail.remove();
      st.tail = null;
    }
  }

  // ── Model reasoning stream (thinking models) ───────────────────────────────
  // Reasoning can run to thousands of characters, so only a tail window is kept in the DOM and
  // the block is capped in height with a fade. The point is to show that the model is working,
  // not to reproduce its entire internal monologue.
  //
  // 模型推理流（思考模型）。
  // 推理内容可达数千字，因此只在 DOM 里保留尾部窗口，并限高加渐隐。
  // 目的是表明模型确实在推理，而不是复现它完整的内心独白。
  var THOUGHT_KEEP = 1400;

  function feedThought(trace, st, text, total) {
    var box = trace.querySelector('.ask-thought');
    if (!box) {
      box = document.createElement('div');
      box.className = 'ask-thought';
      box.innerHTML =
        '<button type="button" class="ask-thought-h">' +
          '<span class="ask-thought-ico" aria-hidden="true">' +
            '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M12 3a7 7 0 0 1 7 7c0 2-1 3-1 3l1 4h-5a7 7 0 1 1-2-14z"/><circle cx="9.5" cy="10" r=".9" fill="currentColor" stroke="none"/><circle cx="14.5" cy="10" r=".9" fill="currentColor" stroke="none"/></svg>' +
          '</span>' +
          '<span class="ask-thought-t">正在推理</span>' +
          '<span class="ask-thought-n"></span>' +
        '</button>' +
        '<div class="ask-thought-b"></div>';
      box.querySelector('.ask-thought-h').addEventListener('click', function () {
        box.classList.toggle('is-open');
      });
      // Reasoning sits between the retrieval steps and the answer.
      var anchor = trace.querySelector('.ask-sources');
      trace.insertBefore(box, anchor || null);
      st.buf = '';
    }
    st.buf += text;
    if (st.buf.length > THOUGHT_KEEP) {
      st.buf = '…' + st.buf.slice(-THOUGHT_KEEP);
    }
    box.querySelector('.ask-thought-b').textContent = st.buf;
    box.querySelector('.ask-thought-n').textContent = total + ' 字';
    scroll();
  }

  function finishThought(trace) {
    var box = trace.querySelector('.ask-thought');
    if (!box || box.classList.contains('is-done')) return;
    box.classList.add('is-done');
    var t = box.querySelector('.ask-thought-t');
    if (t) t.textContent = '推理完成';
  }

  function renderSources(trace, list) {
    if (!list.length) return;
    // 插进 .ask-steps 当场追加，而不是塞进 .ask-sources。
    // 后者是 steps 的兄弟节点，天生只能排在所有步骤之后 ——
    // 于是「正在浏览 N 篇」永远出现在「由 X 接手」下面，顺序看起来就是乱的。
    //
    // Append into .ask-steps at the current position instead of the sibling .ask-sources:
    // a sibling can only ever render after every step, which is why this line kept showing up
    // below the model hand-off.
    var steps = trace.querySelector('.ask-steps');
    var box = document.createElement('div');
    box.className = 'ask-src';
    steps.appendChild(box);
    // Collapsed by default. The retrieval list is process detail, and twelve rows of it pushed
    // the answer — the thing being waited for — below the fold. The citations at the end of the
    // answer are the readable version; this stays as a fold-out audit trail.
    // 默认收起。检索列表属于过程细节，十二行足以把用户真正在等的回答挤出视野；
    // 回答末尾的引文卡片才是可读版本，这里只作为可展开的溯源记录。
    box.innerHTML =
      '<button type="button" class="ask-src-toggle" aria-expanded="false">' +
        '<span class="ask-step-ico" aria-hidden="true">' + STAGE_ICON.eye + '</span>' +
        '<span class="ask-src-toggle-t">正在浏览 ' + list.length + ' 篇相关笔记</span>' +
      '</button>' +
      '<ul class="ask-src-list" hidden>' +
      list.map(function (s, i) {
        return '<li style="animation-delay:' + (i * 55) + 'ms">' +
                 '<a href="' + s.url + '" target="_blank" rel="noopener noreferrer">' +
                   esc(s.title) + '<span class="ask-src-arrow" aria-hidden="true">↗</span>' +
                 '</a>' +
                 '<span class="ask-src-via">' + esc(s.via) + '</span>' +
               '</li>';
      }).join('') +
      '</ul>';

    var toggle = box.querySelector('.ask-src-toggle');
    var listEl = box.querySelector('.ask-src-list');
    toggle.addEventListener('click', function () {
      var open = listEl.hidden;
      listEl.hidden = !open;
      toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
      toggle.classList.toggle('is-open', open);
    });
  }

  function renderCites(box, list) {
    if (!list.length) return;
    box.hidden = false;
    box.innerHTML = '<div class="ask-cites-head">' +
        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>' +
        '引用笔记</div>' +
      list.map(function (c, i) {
        return '<a class="ask-cite-card" id="ask-cite-' + c.index + '" href="' + c.url + '" ' +
                 'style="animation-delay:' + (i * 55) + 'ms">' +
                 '<span class="ask-cite-n">' + c.index + '</span>' +
                 '<span class="ask-cite-b">' +
                   '<span class="ask-cite-t">' + esc(c.title) + '</span>' +
                   (c.excerpt ? '<span class="ask-cite-x">' + esc(c.excerpt) + '</span>' : '') +
                 '</span>' +
               '</a>';
      }).join('');
  }

  // Citation chips in the answer jump to the matching card.
  host.addEventListener('click', function (e) {
    var b = e.target.closest && e.target.closest('.ask-cite');
    if (!b) return;
    var card = document.getElementById('ask-cite-' + b.getAttribute('data-cite'));
    if (!card) return;
    card.scrollIntoView({ behavior: reduce ? 'auto' : 'smooth', block: 'nearest' });
    card.classList.add('is-flash');
    setTimeout(function () { card.classList.remove('is-flash'); }, 1200);
  });

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    var q = input.value.trim();
    if (!q || BUSY) return;
    input.value = '';
    grow();
    send(q);
  });

  // Stop an in-flight answer when the page is hidden for good.
  window.addEventListener('pagehide', function () { if (CTRL) CTRL.abort(); });
})();
`
