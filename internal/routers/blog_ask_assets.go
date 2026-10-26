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

// ── 三方互斥的唯一约定 ──────────────────────────────────────────────────────
// 图谱、问答、抽屉是三个独立脚本，各自维护自己的开关状态。它们之间只需要一条消息：
// 「我开了」。谁开都广播同一个事件，其余各方收到就关自己 —— 谁也不需要知道别人叫什么。
//
// 为什么不用共享的全局变量：那要求三个脚本按固定顺序加载，且有初始化先后的硬依赖。
// 一个事件没有顺序问题，没人在听也不报错。
function broadcastOpen(who) {
  try {
    document.dispatchEvent(new CustomEvent('fns:panel-open', { detail: { who: who } }));
  } catch (e) {}
}


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
  // 底部胶囊那一格（以及任何带 data-ask-open 的元素）都能开合面板。
  // 这里发布约定，脚本不必知道入口在哪、长什么样。
  //
  // 开关是 toggle 而不是 open：同一个键按第二下必须收回去。只绑 open 时，
  // 打开之后底部那一格就再没有作用 —— 实测点击前后 className 都是 ask-on ask-in。
  //
  // 互斥同样在这里：打开时广播 fns:panel-open，图谱与抽屉各自收到就关自己。
  // 不共享闭包变量，因为三方分别定义在三个脚本里，共享变量意味着其中一个必须先存在。
  var openers = document.querySelectorAll('[data-ask-open]');
  Array.prototype.forEach.call(openers, function (el) {
    el.addEventListener('click', function (e) {
      e.preventDefault();
      if (OPEN) { close(); } else { open(); }
    });
  });
  // 面板自己关掉时把 aria 同步回去（多个入口时逐个复位）。
  function syncOpeners() {
    Array.prototype.forEach.call(openers, function (el) {
      el.setAttribute('aria-expanded', OPEN ? 'true' : 'false');
    });
  }

  // ── Open / close ───────────────────────────────────────────────────────────
  function open() {
    if (OPEN) return;
    OPEN = true;
    backdrop.hidden = false;
    panel.hidden = false;
    fab.setAttribute('aria-expanded', 'true');
    syncOpeners();
    host.classList.add('ask-on');
    // 告诉图谱与抽屉：我开了。它们各自收自己，不需要知道我是谁。
    broadcastOpen('ask');
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
      syncOpeners();
      panel.removeEventListener('transitionend', done);
    };
    if (reduce) { done(); return; }
    guard = setTimeout(done, 420);
    panel.addEventListener('transitionend', done);
  }
  fab.addEventListener('click', function () { OPEN ? close() : open(); });
  closeBtn.addEventListener('click', close);
  backdrop.addEventListener('click', close);
  // 别人打开了，我收自己。延迟 0 让它排在对方那轮同步代码之后 ——
  // 否则我在对方「已经打开但还没稳定」的中间态里执行关闭，动画会打架。
  document.addEventListener('fns:panel-open', function (ev) {
    var who = ev && ev.detail ? ev.detail.who : '';
    if (who === 'ask') { return; }
    if (OPEN) { close(); }
  });
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

  // 「键盘弹起」的判据：每个信号各自相对**自己的初始值**下降了多少。
  //
  // 这是这一块最容易写错的地方，两种错法都踩过：
  //
  //   错法一：拿 innerHeight 和 visualViewport.height 相减。
  //     Android X5 上两者一起变小，差值恒为 0，接管永不触发。
  //
  //   错法二：拿当前可视高度和「曾经见过的最大 innerHeight」相减。
  //     Windows 微信的网页窗口里 innerHeight=1108 而 visualViewport.height=560，
  //     这个几百像素的差是**静态的**（窗口结构），跟键盘无关，
  //     于是被当成「键盘吃了 548px」，面板按一个虚构的可视高度被推到半空。
  //
  // 正确做法是各比各的：键盘弹起只会让某个信号比它刚加载时更小。
  //   iOS：visualViewport 下降 300 多，innerHeight 不动；
  //   Android：两者都下降；
  //   Windows 微信：两者都不下降（那 548 的差是常量）→ 干净地不接管。
  // 取两个下降量的较大值，任一侧能反映键盘就够了。
  var BASE_IH = window.innerHeight || 0;
  var BASE_VV = viewport && viewport.height > 0 ? viewport.height : BASE_IH;
  function noteBaseHeights() {
    var ih = window.innerHeight || 0;
    if (ih > BASE_IH) BASE_IH = ih;
    var vh = viewport && viewport.height > 0 ? viewport.height : 0;
    if (vh > BASE_VV) BASE_VV = vh;
  }

  function clearPanelGeometry() {
    panel.style.top = '';
    panel.style.height = '';
    panel.style.bottom = '';
    panel.style.paddingBottom = '';
  }

  // 面板比可视高度多铺这么多，多出来的部分一定是被键盘盖住的。
  //
  // 为什么需要这一段：visualViewport.height 在 iOS 微信里比 webview 真实可见区
  // 少算一截（实测 iPhone 14 上约 27px），于是按它定高的面板底边会悬在键盘上方，
  // 露出下面一条模糊正文 —— 看起来就是「留了一节」。
  // 那个差值拿不到，也不该去猜：换个写法让它变得无害 ——
  //   面板照可视高度铺，再往下多铺 40px；
  //   同时给面板补 40px 底部内边距，把 input 顶回原来的位置。
  // 于是无论这 40px 里有多少真的被键盘吃掉：
  //   算得准时，40px 全在键盘后面，输入框正好贴着键盘；
  //   少算 27px 时，多出的 13px 是面板自己的底色，不是露出来的正文。
  // 两种都比「悬空露背景」好，而且不需要知道那 27px 究竟是什么。
  //
  // 桌面端不接管，Safari 的 visualViewport 本来就是准的 —— 那边多出的 40px
  // 同样落在键盘后面，看不见，所以不需要按内核分开。
  var PANEL_SLACK = 40;

  function syncViewport() {
    if (!OPEN) return;
    if (viewportRaf) return;
    viewportRaf = requestAnimationFrame(function () {
      viewportRaf = 0;
      if (!OPEN) return;
      // 桌面宽屏永不接管：那里没有软键盘，CSS 那份定位就够了。
      if (narrowQuery && !narrowQuery.matches) { clearPanelGeometry(); return; }

      // 当前真正看得见的高度：取两者较小值。
      // 键盘升降过程中 visualViewport 与 innerHeight 会短暂不同步，
      // 用较大的那个就会把面板底部撑到键盘下面去。
      noteBaseHeights();
      var visH = viewport ? viewport.height : window.innerHeight;
      var visT = viewport ? viewport.offsetTop : 0;
      // 有效性守卫：页面刚加载、面板刚插入时 visualViewport.height 可能还是 0 或 undefined。
      // 曾经没有这一条，h 被算成负数，面板塌成一条白线 —— 那不是微信的问题，
      // 是读到了一个还没初始化的值。读到无效值就退回样式表。
      if (!(visH > 200)) { clearPanelGeometry(); return; }
      var h = Math.min(visH, window.innerHeight);

      // 键盘吃掉的高度，按各信号相对自己初始值的下降量取最大。
      // 阈值 150：iOS/Android 的软键盘都会吃掉 250px 以上，而地址栏、工具栏的
      // 收缩在 120px 以内 —— 用 150 把两者分开，避免浏览地址栏就把面板跳一下。
      var dropIH = BASE_IH - (window.innerHeight || 0);
      var dropVV = BASE_VV - visH;
      var kb = Math.max(dropIH, dropVV);
      if (!(kb >= 150)) { clearPanelGeometry(); return; }

      // 键盘弹起时胶囊（position:fixed，不跟随键盘）也被盖在键盘后面了，
      // 所以这里不再给它让位 —— 让了就是在输入框和键盘之间凭空塞一条空白。
      // 面板底边就是键盘顶边，输入框贴着键盘上方，和豆包那种聊天窗一致。
      if (!(h > 160)) { clearPanelGeometry(); return; }
      panel.style.top = Math.max(0, Math.round(visT)) + 'px';
      panel.style.height = Math.round(h + PANEL_SLACK) + 'px';
      panel.style.paddingBottom = PANEL_SLACK + 'px';
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

  // 横竖屏切换后两个基准都失效（844 → 390 会被当成「键盘吃掉了 454」），
  // 必须重取。等一拍再取，内核这会儿还在改尺寸。
  function onOrientation() {
    setTimeout(function () {
      BASE_IH = window.innerHeight || 0;
      BASE_VV = viewport && viewport.height > 0 ? viewport.height : BASE_IH;
      syncViewport();
    }, 260);
  }

  function bindViewport() {
    // window 的两个事件始终绑：visualViewport 不存在的老内核靠 innerHeight 的 resize 也能走通。
    window.addEventListener('resize', syncViewport);
    window.addEventListener('orientationchange', onOrientation);
    if (viewport) {
      viewport.addEventListener('resize', syncViewport);
      viewport.addEventListener('scroll', syncViewport);
    }
    startViewportPoll();
  }
  function unbindViewport() {
    stopViewportPoll();
    if (viewport) {
      viewport.removeEventListener('resize', syncViewport);
      viewport.removeEventListener('scroll', syncViewport);
    }
    window.removeEventListener('resize', syncViewport);
    window.removeEventListener('orientationchange', onOrientation);
    if (viewportRaf) { cancelAnimationFrame(viewportRaf); viewportRaf = 0; }
    clearPanelGeometry();
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
  // 只做一个状态标记：加 is-wx-typing 类，记录「键盘正开着」。
  // 面板几何不在这里算 —— 全部交给 syncViewport()，它对所有内核用同一套逻辑。
  // 之前这里根据 UA 分支改高度/改上限，微信里恰好就是那条分支把布局弄坏。
  function setWeChatTyping(on) {
    if (!inWeChat) return;
    document.documentElement.classList.toggle('is-wx-typing', !!on);
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
        // 摘要行常驻：它显示"当前走到哪一步"，永远占一行。明细收在它下面。
        // 这样可见高度自始至终是 1.65em，不存在折叠时的纵向跳动。
        '<button type="button" class="ask-trace-sum" aria-expanded="false">' +
          '<span class="ask-sum-ico" aria-hidden="true"></span>' +
          '<span class="ask-sum-t"></span>' +
          '<span class="ask-sum-x">明细</span>' +
        '</button>' +
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
  // 步骤图标：Primer Octicons，16 网格、fill、单色。
  // 上一版是自绘的 24 网格线性图标，各有各的造型语言（三角脊线、实心圆眼、分叉圆点），
  // 并排放着像几套东西拼起来的；用户反馈「太抽象，看不懂」。
  // Octicons 是 GitHub 自己那套，形状直白、笔画干净，同一 family 里不会有风格冲突。
  function oct(d) {
    return '<svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">' + d + '</svg>';
  }
  var STAGE_ICON = {
    question: oct('<path d="M0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8Zm8-6.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13ZM6.92 6.085h.001a.749.749 0 1 1-1.342-.67c.169-.339.436-.701.849-.977C6.845 4.16 7.369 4 8 4a2.756 2.756 0 0 1 1.637.525c.503.377.863.965.863 1.725 0 .448-.115.83-.329 1.15-.205.307-.47.513-.692.662-.109.072-.22.138-.313.195l-.006.004a6.24 6.24 0 0 0-.26.16.952.952 0 0 0-.276.245.75.75 0 0 1-1.248-.832c.184-.264.42-.489.692-.661.103-.067.207-.132.313-.195l.007-.004c.1-.061.182-.11.258-.161a.969.969 0 0 0 .277-.245C8.96 6.514 9 6.427 9 6.25a.612.612 0 0 0-.262-.525A1.27 1.27 0 0 0 8 5.5c-.369 0-.595.09-.74.187a1.01 1.01 0 0 0-.34.398ZM9 11a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z"></path>'),
    search: oct('<path d="M10.68 11.74a6 6 0 0 1-7.922-8.982 6 6 0 0 1 8.982 7.922l3.04 3.04a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215ZM11.5 7a4.499 4.499 0 1 0-8.997 0A4.499 4.499 0 0 0 11.5 7Z"></path>'),
    idea: oct('<path d="M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984.424 1.625.984 2.304l.214.253c.223.264.47.556.673.848.284.411.537.896.621 1.49a.75.75 0 0 1-1.484.211c-.04-.282-.163-.547-.37-.847a8.456 8.456 0 0 0-.542-.68c-.084-.1-.173-.205-.268-.32C3.201 7.75 2.5 6.766 2.5 5.25 2.5 2.31 4.863 0 8 0s5.5 2.31 5.5 5.25c0 1.516-.701 2.5-1.328 3.259-.095.115-.184.22-.268.319-.207.245-.383.453-.541.681-.208.3-.33.565-.37.847a.751.751 0 0 1-1.485-.212c.084-.593.337-1.078.621-1.489.203-.292.45-.584.673-.848.075-.088.147-.173.213-.253.561-.679.985-1.32.985-2.304 0-2.06-1.637-3.75-4-3.75ZM5.75 12h4.5a.75.75 0 0 1 0 1.5h-4.5a.75.75 0 0 1 0-1.5ZM6 15.25a.75.75 0 0 1 .75-.75h2.5a.75.75 0 0 1 0 1.5h-2.5a.75.75 0 0 1-.75-.75Z"></path>'),
    funnel: oct('<path d="M5.75 2.5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5ZM2 14a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1-6a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM2 4a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"></path>'),
    eye: oct('<path d="M8 2c1.981 0 3.671.992 4.933 2.078 1.27 1.091 2.187 2.345 2.637 3.023a1.62 1.62 0 0 1 0 1.798c-.45.678-1.367 1.932-2.637 3.023C11.67 13.008 9.981 14 8 14c-1.981 0-3.671-.992-4.933-2.078C1.797 10.83.88 9.576.43 8.898a1.62 1.62 0 0 1 0-1.798c.45-.677 1.367-1.931 2.637-3.022C4.33 2.992 6.019 2 8 2ZM1.679 7.932a.12.12 0 0 0 0 .136c.411.622 1.241 1.75 2.366 2.717C5.176 11.758 6.527 12.5 8 12.5c1.473 0 2.825-.742 3.955-1.715 1.124-.967 1.954-2.096 2.366-2.717a.12.12 0 0 0 0-.136c-.412-.621-1.242-1.75-2.366-2.717C10.824 4.242 9.473 3.5 8 3.5c-1.473 0-2.825.742-3.955 1.715-1.124.967-1.954 2.096-2.366 2.717ZM8 10a2 2 0 1 1-.001-3.999A2 2 0 0 1 8 10Z"></path>'),
    pencil: oct('<path d="M11.013 1.427a1.75 1.75 0 0 1 2.474 0l1.086 1.086a1.75 1.75 0 0 1 0 2.474l-8.61 8.61c-.21.21-.47.364-.756.445l-3.251.93a.75.75 0 0 1-.927-.928l.929-3.25c.081-.286.235-.547.445-.758l8.61-8.61Zm.176 4.823L9.75 4.81l-6.286 6.287a.253.253 0 0 0-.064.108l-.558 1.953 1.953-.558a.253.253 0 0 0 .108-.064Zm1.238-3.763a.25.25 0 0 0-.354 0L10.811 3.75l1.439 1.44 1.263-1.263a.25.25 0 0 0 0-.354Z"></path>'),
    bolt: oct('<path d="M9.504.43a1.516 1.516 0 0 1 2.437 1.713L10.415 5.5h2.123c1.57 0 2.346 1.909 1.22 3.004l-7.34 7.142a1.249 1.249 0 0 1-.871.354h-.302a1.25 1.25 0 0 1-1.157-1.723L5.633 10.5H3.462c-1.57 0-2.346-1.909-1.22-3.004L9.503.429Zm1.047 1.074L3.286 8.571A.25.25 0 0 0 3.462 9H6.75a.75.75 0 0 1 .694 1.034l-1.713 4.188 6.982-6.793A.25.25 0 0 0 12.538 7H9.25a.75.75 0 0 1-.683-1.06l2.008-4.418.003-.006a.036.036 0 0 0-.004-.009l-.006-.006-.008-.001c-.003 0-.006.002-.009.004Z"></path>'),
    fork: oct('<path d="M9.5 3.25a2.25 2.25 0 1 1 3 2.122V6A2.5 2.5 0 0 1 10 8.5H6a1 1 0 0 0-1 1v1.128a2.251 2.251 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.5 0v1.836A2.493 2.493 0 0 1 6 7h4a1 1 0 0 0 1-1v-.628A2.25 2.25 0 0 1 9.5 3.25Zm-6 0a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0Zm8.25-.75a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5ZM4.25 12a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Z"></path>'),
    flame: oct('<path d="M9.533.753V.752c.217 2.385 1.463 3.626 2.653 4.81C13.37 6.74 14.498 7.863 14.498 10c0 3.5-3 6-6.5 6S1.5 13.512 1.5 10c0-1.298.536-2.56 1.425-3.286.376-.308.862 0 1.035.454C4.46 8.487 5.581 8.419 6 8c.282-.282.341-.811-.003-1.5C4.34 3.187 7.035.75 8.77.146c.39-.137.726.194.763.607ZM7.998 14.5c2.832 0 5-1.98 5-4.5 0-1.463-.68-2.19-1.879-3.383l-.036-.037c-1.013-1.008-2.3-2.29-2.834-4.434-.322.256-.63.579-.864.953-.432.696-.621 1.58-.046 2.73.473.947.67 2.284-.278 3.232-.61.61-1.545.84-2.403.633a2.79 2.79 0 0 1-1.436-.874A3.198 3.198 0 0 0 3 10c0 2.53 2.164 4.5 4.998 4.5Z"></path>'),
    dizzy: oct('<path d="M1.896 4.559a6.25 6.25 0 0 1 8.839 0 .75.75 0 0 1-1.06 1.061 4.75 4.75 0 1 0 0 6.717L13.03 8.98l-1.553-1.554A.25.25 0 0 1 11.654 7h4.096a.25.25 0 0 1 .25.25v4.096a.25.25 0 0 1-.427.177l-1.482-1.482-3.356 3.356a6.25 6.25 0 0 1-8.839-8.838Z"></path>'),
    angry: oct('<path d="M0 8a8 8 0 0 1 16 0v5.25a.75.75 0 0 1-1.5 0V8a6.5 6.5 0 1 0-13 0v5.25a.75.75 0 0 1-1.5 0Zm3-1.25C3 5.784 3.784 5 4.75 5h6.5c.966 0 1.75.784 1.75 1.75v1.5A1.75 1.75 0 0 1 11.25 10h-6.5A1.75 1.75 0 0 1 3 8.25Zm1.47-.53a.75.75 0 0 0 0 1.06l1.5 1.5a.75.75 0 0 0 1.06 0L8 7.81l.97.97a.75.75 0 0 0 1.06 0l1.5-1.5a.749.749 0 0 0-.326-1.275.749.749 0 0 0-.734.215l-.97.97-.97-.97a.75.75 0 0 0-1.06 0l-.97.97-.97-.97a.75.75 0 0 0-1.06 0Zm1.03 6.03a.75.75 0 0 1 .75-.75h3.5a.75.75 0 0 1 0 1.5h-3.5a.75.75 0 0 1-.75-.75Z"></path>'),
    alert: oct('<path d="M6.457 1.047c.659-1.234 2.427-1.234 3.086 0l6.082 11.378A1.75 1.75 0 0 1 14.082 15H1.918a1.75 1.75 0 0 1-1.543-2.575Zm1.763.707a.25.25 0 0 0-.44 0L1.698 13.132a.25.25 0 0 0 .22.368h12.164a.25.25 0 0 0 .22-.368Zm.53 3.996v2.5a.75.75 0 0 1-1.5 0v-2.5a.75.75 0 0 1 1.5 0ZM9 11a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z"></path>'),
    think: oct('<path d="M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984.424 1.625.984 2.304l.214.253c.223.264.47.556.673.848.284.411.537.896.621 1.49a.75.75 0 0 1-1.484.211c-.04-.282-.163-.547-.37-.847a8.456 8.456 0 0 0-.542-.68c-.084-.1-.173-.205-.268-.32C3.201 7.75 2.5 6.766 2.5 5.25 2.5 2.31 4.863 0 8 0s5.5 2.31 5.5 5.25c0 1.516-.701 2.5-1.328 3.259-.095.115-.184.22-.268.319-.207.245-.383.453-.541.681-.208.3-.33.565-.37.847a.751.751 0 0 1-1.485-.212c.084-.593.337-1.078.621-1.489.203-.292.45-.584.673-.848.075-.088.147-.173.213-.253.561-.679.985-1.32.985-2.304 0-2.06-1.637-3.75-4-3.75ZM5.75 12h4.5a.75.75 0 0 1 0 1.5h-4.5a.75.75 0 0 1 0-1.5ZM6 15.25a.75.75 0 0 1 .75-.75h2.5a.75.75 0 0 1 0 1.5h-2.5a.75.75 0 0 1-.75-.75Z"></path>'),
    link: oct('<path d="M2.75 3.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h2a.75.75 0 0 1 .75.75v2.19l2.72-2.72a.749.749 0 0 1 .53-.22h4.5a.25.25 0 0 0 .25-.25v-2.5a.75.75 0 0 1 1.5 0v2.5A1.75 1.75 0 0 1 13.25 13H9.06l-2.573 2.573A1.458 1.458 0 0 1 4 14.543V13H2.75A1.75 1.75 0 0 1 1 11.25v-7.5C1 2.784 1.784 2 2.75 2h5.5a.75.75 0 0 1 0 1.5ZM16 1.25v4.146a.25.25 0 0 1-.427.177L14.03 4.03l-3.75 3.75a.749.749 0 0 1-1.275-.326.749.749 0 0 1 .215-.734l3.75-3.75-1.543-1.543A.25.25 0 0 1 11.604 1h4.146a.25.25 0 0 1 .25.25Z"></path>'),
    route: oct('<path d="M3.75 6.5a.25.25 0 0 0-.25.25v6.5c0 .138.112.25.25.25h8.5a.25.25 0 0 0 .25-.25v-6.5a.25.25 0 0 0-.25-.25h-1a.75.75 0 0 1 0-1.5h1c.966 0 1.75.784 1.75 1.75v6.5A1.75 1.75 0 0 1 12.25 15h-8.5A1.75 1.75 0 0 1 2 13.25v-6.5C2 5.784 2.784 5 3.75 5h1a.75.75 0 0 1 0 1.5ZM7.823.177a.25.25 0 0 1 .354 0l2.896 2.896a.25.25 0 0 1-.177.427H8.75v5.75a.75.75 0 0 1-1.5 0V3.5H5.104a.25.25 0 0 1-.177-.427Z"></path>')
  };
  // 摘要行点击 = 展开明细。展开是读者主动的行为，那一刻的位移是可接受的；
  // 不展开时它恒定一行，所以整个生成过程不产生任何布局位移。
  document.addEventListener('click', function (e) {
    var sum = e.target.closest && e.target.closest('.ask-trace-sum');
    if (!sum) return;
    var trace = sum.parentNode;
    var open = trace.classList.toggle('is-open');
    sum.setAttribute('aria-expanded', open ? 'true' : 'false');
    var x = sum.querySelector('.ask-sum-x');
    if (x) x.textContent = open ? '收起' : '明细';
  });

  // 步骤只向前走，且同一条文案不重复出现。
  //
  // 修的是录屏里那个「想一下怎么讲你才听得懂…」出现两次的问题：
  // 它由两条独立路径产生 —— 后端 stage 事件，以及前端 1.4 秒无输出时的本地兜底计时器。
  // 两条都触发时就会写两条一模一样的行。判重按**文案**而不是按来源，
  // 因为读者看到的是文案；至于它是谁发的，对读者没有意义。
  //
  // 另外：已经 done 的步骤不再获得 is-live，所以任何时刻最多只有一条带光标的行 ——
  // 上一版某个 delta 到达后仍有一条 live 残留，看起来像两个阶段同时在跑。
  var STEP_SEEN = null;
  function step(trace, kind, text) {
    trace.hidden = false;
    var steps = trace.querySelector('.ask-steps');
    var key = String(text).trim();
    // 上一条文案相同：只把它重新标成 live，不新建节点。
    var last = steps.lastElementChild;
    if (last && STEP_SEEN === key) {
      var liveOld = steps.querySelector('.ask-step.is-live');
      if (liveOld && liveOld !== last) {
        liveOld.classList.remove('is-live');
        liveOld.classList.add('is-done');
      }
      last.classList.remove('is-done');
      last.classList.add('is-live');
      return last;
    }
    var prev = steps.querySelector('.ask-step.is-live');
    if (prev) { prev.classList.remove('is-live'); prev.classList.add('is-done'); }
    var el = document.createElement('div');
    el.className = 'ask-step is-live';
    el.innerHTML = '<span class="ask-step-ico">' + (STAGE_ICON[kind] || STAGE_ICON.think) + '</span>' +
                   '<span class="ask-step-t">' + esc(text) + '</span>';
    steps.appendChild(el);
    STEP_SEEN = key;

    // 摘要行跟着换字：它是唯一可见的那一行。
    // 图标也一起换 —— 摘要图标如果固定不动，读者就看不出阶段真的在推进。
    var sum = trace.querySelector('.ask-trace-sum');
    if (sum) {
      var ico = sum.querySelector('.ask-sum-ico');
      if (ico) ico.innerHTML = STAGE_ICON[kind] || STAGE_ICON.think;
      var t = sum.querySelector('.ask-sum-t');
      if (t) t.textContent = key;
    }
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

    // 不再有本地「还在想」兜底计时器。
    //
    // 它曾经存在的理由是「模型迟迟不出首 token 时让面板看起来不像卡住」。
    // 但它和后端的 stage 事件是两条独立来源，会写出同一条文案两次 ——
    // 录屏里那句「想一下怎么讲你才听得懂…」前后各出现一次，正是这个原因。
    // 后端在每个真实阶段都会发 stage，已经覆盖了「让用户知道在动」这件事；
    // 前端再补一条纯属重复。等待态的确定性由 step() 的文案判重保证。
    function armThinkTimer() { /* 保留空壳：调用点较多，逐处删改风险大于收益 */ }

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

  // 生成开始时把还亮着的步骤收尾。
  //
  // 这里曾经做的是"把几百像素的步骤列表折叠成一行摘要"，那一下折叠是整个面板里
  // 最明显的一次布局跳动 —— 正文被顶上去几十像素，正在读数的人直接失焦。
  // 现在摘要行从 addBot 起就在（见 addBot 的注释），步骤明细从一开始就收在它下面，
  // 所以这里只剩下"把最后一条 live 标记成 done"这一件事，不产生任何高度变化。
  var minimizeTimer = 0;
  function scheduleMinimize(trace) {
    if (minimizeTimer) clearTimeout(minimizeTimer);
    minimizeTimer = setTimeout(function () {
      minimizeTimer = 0;
      var live = trace.querySelector('.ask-step.is-live');
      if (live) { live.classList.remove('is-live'); live.classList.add('is-done'); }
    }, reduce ? 0 : 900);
  }

  // ── Answer streaming: append-only, stable-prefix commit ────────────────────
  //
  // 流式渲染只做一件事：把已确认的文本追加进 DOM，永不回头改已经渲染过的部分。
  //
  // 上一版按「短语边界」切块、每块单独 render，产生了两类可见故障：
  //   ① 一个块可能正好切在列表项中间，于是孤立的 "- xxx" 被当成一条完整列表渲染；
  //      等后文补齐，整块重排 —— 读者看到「先打出一行错的，下一秒又被抹掉归位」。
  //   ② 每块都有入场动画，加上短语边界本身要等标点，正文像被一段段塞进来。
  //
  // 现在的切分点是「最后一个绝对安全的位置」：
  //   只在一个段落真正闭合之后提交（遇到空行），或遇到明确的行内结束符。
  //   未闭合的尾部（可能还有下文）单独留在 tail 节点里，但**不做 markdown 列表/标题解析** ——
  //   它只是一段纯文本，等闭合后一次性升格成正式块。
  //   这样既保证「一个字都不重绘」，也不会把半截列表渲染成成品。
  //
  // 另一层保障：整篇文本永远只有一次 render 结果参与排版，没有幽灵底字。
  // 尾块天然最多一个段落：commitBoundary 只在段落级安全位置提交，
  // 所以不需要"尾部长了就强切"的兜底。曾经按空格强切，会切在 "## " / "- " 这类
  // 标记内部的空格上，切出只剩标记的碎片，render 成一个空标题，那段文字凭空消失。

  // 尾块的显示文本：只剥块级标记，不动行内标记。
  function tailDisplay(s) {
    var t = s
      .replace(/^[ \t]*#{1,6}[ \t]+/gm, '')
      .replace(/^[ \t]*[-*+][ \t]+/gm, '')
      .replace(/^[ \t]*\d+[.)][ \t]+/gm, '');
    // 末尾只剩一个残缺的块标记（"#" / "##" / "-" / "1."）：这一帧先不显示它。
    // 标记是一个字符一个字到达的，"#" 先到、"## " 后到，中间那帧会把字面的 # 露出来，
    // 下一帧又被剥掉 —— 读者看到的就是一个符号闪了一下。等空格到齐再显示内容。
    return t.replace(/(^|\n)[ \t]*(?:#{1,6}|[-*+]|\d+[.)])$/, '$1');
  }

  function commitBoundary(text) {
    // 段落边界优先：空行之后一定安全。
    var idx = text.lastIndexOf('\n\n');
    if (idx >= 0) return idx + 2;
    // 没有空行时看单行结束符；列表项必须整行结束才算安全（"- xxx" 后面可能还有续行）。
    var m = /\n(?!\s*[-*+]\s|\s*\d+[.)]\s|\s*#{1,6}\s)/.exec(text);
    if (m) return m.index + 1;
    return -1;
  }

  function feedAnswer(answer, st, raw) {
    var cut = commitBoundary(raw);
    if (cut > st.committed) {
      var seg = raw.slice(st.committed, cut);
      if (seg.replace(/\s/g, '')) {
        var el = document.createElement('span');
        el.className = 'ask-blk';
        el.innerHTML = render(seg);
        // 必须插在尾块**之前**。
        //
        // 尾块是常驻节点，装的是"还没闭合、更靠后"的文字。新提交的块若用 appendChild，
        // 就会落到尾块后面 —— 屏幕上先出现后文（尾块），再把前文塞到它下面。
        // 视觉表现正是用户指出的「先凭空冒出一行后文的列表项，下一秒又被抹掉归位」。
        // insertBefore 让提交顺序与文本顺序一致，这才是 chunk 乱序的根因。
        if (st.tail) { answer.insertBefore(el, st.tail); } else { answer.appendChild(el); }
      }
      st.committed = cut;
    }
    var tail = raw.slice(st.committed);
    if (tail) {
      if (!st.tail) {
        st.tail = document.createElement('span');
        st.tail.className = 'ask-blk ask-blk-live';
        answer.appendChild(st.tail);
      }
      // 尾部只当纯文本显示，不解析成节点 ——
      // 半截语法一旦被解析成元素，补齐时必然重排，那正是"先打错再覆写"的来源。
      // 但块级标记要剥掉：否则读者会在流式过程中看到字面的 "## 标题" 和 "- 条文"。
      // 只剥块级（标题前缀、列表前缀），行内标记（** / 反引号）保留 ——
      // 行内标记被剥掉再补回来，会看到明显的加粗闪动。
      st.tail.textContent = tailDisplay(tail);
    } else if (st.tail) {
      st.tail.remove();
      st.tail = null;
    }
  }

  // ── Model reasoning: a status line, not a transcript ───────────────────────
  // 推理链默认不展示内容。
  //
  // 上一版把推理全文（限 1400 字）铺进一个可展开的大卡片里，字数实时跳动；
  // 推理结束时这个巨大容器瞬间塌成单行，正文同时上顶 —— 实测页面纵向跳动数十像素，
  // 读数跳到一半的人完全失去焦点。这是 CLS 的典型来源，而且收益很小：
  // 读者真正想看的是答案，不是模型的内心独白。
  //
  // 现在只有一行固定高度（1.65em）的状态条：左侧一个字数的滚动计数。
  // 它从头到尾不改高度、不插入大块内容，所以视口不会因为推理开始或结束而移动。
  // 想留痕就给「过程」一个可展开入口，展开是读者主动的行为，那时的位移可接受。
  function feedThought(trace, st, text, total) {
    var box = trace.querySelector('.ask-thought');
    if (!box) {
      box = document.createElement('div');
      box.className = 'ask-thought';
      box.innerHTML =
        '<span class="ask-thought-ico" aria-hidden="true">' +
          '<svg viewBox="0 0 16 16" fill="currentColor"><path d="M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984.424 1.625.984 2.304l.214.253c.223.264.47.556.673.848.284.411.537.896.621 1.49a.75.75 0 0 1-1.484.211c-.04-.282-.163-.547-.37-.847a8.456 8.456 0 0 0-.542-.68c-.084-.1-.173-.205-.268-.32C3.201 7.75 2.5 6.766 2.5 5.25 2.5 2.31 4.863 0 8 0s5.5 2.31 5.5 5.25c0 1.516-.701 2.5-1.328 3.259-.095.115-.184.22-.268.319-.207.245-.383.453-.541.681-.208.3-.33.565-.37.847a.751.751 0 0 1-1.485-.212c.084-.593.337-1.078.621-1.489.203-.292.45-.584.673-.848.075-.088.147-.173.213-.253.561-.679.985-1.32.985-2.304 0-2.06-1.637-3.75-4-3.75ZM5.75 12h4.5a.75.75 0 0 1 0 1.5h-4.5a.75.75 0 0 1 0-1.5ZM6 15.25a.75.75 0 0 1 .75-.75h2.5a.75.75 0 0 1 0 1.5h-2.5a.75.75 0 0 1-.75-.75Z"/></svg>' +
        '</span>' +
        '<span class="ask-thought-t">正在推理</span>' +
        '<span class="ask-thought-n"></span>';
      var fold = trace.querySelector('.ask-fold') || trace;
      fold.insertBefore(box, fold.querySelector('.ask-sources'));
    }
    // 只更新文字，不追加内容节点：字数变化不会改变行高。
    box.querySelector('.ask-thought-n').textContent = total ? total + ' 字' : '';
    // 摘要行也要跟着动。推理内容本身不展示（那是"隐藏思考链"的要求），
    // 但"正在推理 N 字"是读者唯一能看到的进度信号 —— 一次回答可能有几百个 thinking 事件、
    // 首个答案 token 要到 6.9 秒，中间什么都不动会像是卡死。
    var sum = trace.querySelector('.ask-trace-sum');
    if (sum) {
      var ico = sum.querySelector('.ask-sum-ico');
      if (ico) ico.innerHTML = STAGE_ICON.idea;
      var t = sum.querySelector('.ask-sum-t');
      if (t) t.textContent = '正在推理' + (total ? ' · ' + total + ' 字' : '…');
    }
  }

  function finishThought(trace) {
    var box = trace.querySelector('.ask-thought');
    if (!box || box.classList.contains('is-done')) return;
    box.classList.add('is-done');
    var t = box.querySelector('.ask-thought-t');
    if (t) t.textContent = '推理完成';
    // 摘要行切到"正在写"，这是真正向前推进一步，不是回退。
    var sum = trace.querySelector('.ask-trace-sum');
    if (sum) {
      var ico = sum.querySelector('.ask-sum-ico');
      if (ico) ico.innerHTML = STAGE_ICON.pencil;
      var st = sum.querySelector('.ask-sum-t');
      if (st) st.textContent = '正在写…';
    }
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

  // 引用区从答案一开始就存在（空着，占一个极小的下边距），
  // 而不是等 done 事件到了才 hidden=false。
  //
  // 上一版是「全部打完后，8 条引用一条条弹出来」——每一步都带 55ms 延迟入场，
  // 于是已经滚到底部的回答被强行向下推最多 8 次。读者刚读完最后一句，
  // 视线就跟着跳。这个现象的根因不是动画，而是**内容在结束时凭空长出来**。
  // 现在容器始终在场，卡片灌进已有容器，高度增长发生在读者视线下方而不是把上文顶走；
  // 若 viewport 已贴底，scroll 跟随，视觉上是内容向下延伸。
  function renderCites(box, list) {
    if (!list.length) { box.hidden = true; return; }
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
