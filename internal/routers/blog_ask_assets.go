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
      // innerHeight covers the whole layout viewport; whatever the visual viewport does not
      // cover — below it or scrolled above it — is what the keyboard occupies.
      // innerHeight 是整个布局视口；视觉视口没有覆盖到的部分（其下方或已在其上方卷走的）
      // 就是键盘占据的高度。
      var occluded = Math.max(0, window.innerHeight - viewport.height - viewport.offsetTop);
      panel.style.height = Math.round(viewport.height * 0.88) + 'px';
      panel.style.bottom = Math.round(occluded) + 'px';
      // Follow the newest turn only while the keyboard is actually up: scrolling to the bottom
      // on every resize would yank the reader away from the history they were re-reading.
      // 只在键盘真的弹起时跟到最新一轮：每次 resize 都滚到底会把正在回看历史的读者拽走。
      if (occluded > 0 && body) body.scrollTop = body.scrollHeight;
    });
  }

  function bindViewport() {
    if (!viewport) return;
    viewport.addEventListener('resize', syncViewport);
    viewport.addEventListener('scroll', syncViewport);
    window.addEventListener('orientationchange', syncViewport);
  }
  function unbindViewport() {
    if (viewport) {
      viewport.removeEventListener('resize', syncViewport);
      viewport.removeEventListener('scroll', syncViewport);
    }
    window.removeEventListener('orientationchange', syncViewport);
    if (viewportRaf) { cancelAnimationFrame(viewportRaf); viewportRaf = 0; }
    panel.style.height = '';
    panel.style.bottom = '';
  }

  // ── Auto-grow textarea ─────────────────────────────────────────────────────
  function grow() {
    input.style.height = 'auto';
    input.style.height = Math.min(input.scrollHeight, 132) + 'px';
  }
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
        '<div class="ask-steps"></div>' +
        '<div class="ask-sources"></div>' +
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
  function scroll() { body.scrollTop = body.scrollHeight; }

  // ── Retrieval trace ────────────────────────────────────────────────────────
  var STAGE_ICON = {
    search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg>',
    link:   '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/></svg>',
    think:  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M12 3a7 7 0 0 1 7 7c0 2-1 3-1 3l1 4h-5a7 7 0 1 1-2-14z"/></svg>',
    route:  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M13 2 3 14h7l-1 8 10-12h-7l1-8z"/></svg>'
  };
  function step(trace, kind, text) {
    trace.hidden = false;
    var steps = trace.querySelector('.ask-steps');
    var prev = steps.querySelector('.ask-step.is-live');
    if (prev) { prev.classList.remove('is-live'); prev.classList.add('is-done'); }
    var el = document.createElement('div');
    el.className = 'ask-step is-live';
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
        if (!raw) curStep = step(trace, 'think', '正在组织语言…');
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
          throw new Error('HTTP ' + res.status + (t ? '：' + t.slice(0, 160) : ''));
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
        curStep = step(trace, 'think', '已停止');
        if (!raw) answer.innerHTML = '<p class="ask-none">已停止生成。</p>';
        return;
      }
      clearTimeout(thinkAt);
      failed = true;
      trace.hidden = false;
      step(trace, 'think', '出错了').classList.add('is-error');
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
        var kind = /检索/.test(ev.stage) ? 'search'
                 : /关联|扩展/.test(ev.stage) ? 'link'
                 : /线路/.test(ev.stage) ? 'route' : 'think';
        curStep = step(trace, kind, ev.stage);
        return;
      }
      if (ev.type === 'retrieval') {
        renderSources(trace, ev.sources || []);
        sourceCount = (ev.sources || []).length;
        curStep = step(trace, 'think', '已选定 ' + sourceCount + ' 篇笔记作为依据');
        armThinkTimer();
        return;
      }
      if (ev.type === 'route') {
        curStep = step(trace, 'route', '由 ' + (ev.model || ev.route) + ' 作答');
        return;
      }
      if (ev.type === 'thinking') {
        clearTimeout(thinkAt);
        if (!minimized) { minimized = true; minimizeTrace(trace, sourceCount); }
        feedThought(trace, thoughtState, ev.text || '', ev.thoughtChars || 0);
        return;
      }
      if (ev.type === 'delta') {
        clearTimeout(thinkAt);
        if (!minimized) { minimized = true; minimizeTrace(trace, sourceCount); }
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
        step(trace, 'think', '出错了').classList.add('is-error');
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
        scroll();
      }
    }
  }

  // Once answer text starts arriving, collapse the trace to a single summary line.
  // The steps and the source list together run to several hundred pixels, which pushed the
  // answer itself below the fold — the one thing the reader is actually waiting for.
  function minimizeTrace(trace, n) {
    trace.classList.add('is-min');
    var sum = document.createElement('button');
    sum.type = 'button';
    sum.className = 'ask-trace-sum';
    sum.setAttribute('aria-expanded', 'false');
    sum.innerHTML =
      '<span class="ask-sum-ico" aria-hidden="true">' +
        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m9 12 2 2 4-4"/><circle cx="12" cy="12" r="9"/></svg>' +
      '</span>' +
      '<span class="ask-sum-t">检索完成 · 依据 ' + n + ' 篇笔记</span>' +
      '<span class="ask-sum-x">过程</span>';
    sum.addEventListener('click', function () {
      var open = trace.classList.toggle('is-open');
      sum.setAttribute('aria-expanded', open ? 'true' : 'false');
      sum.querySelector('.ask-sum-x').textContent = open ? '收起' : '过程';
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
    var box = trace.querySelector('.ask-sources');
    if (!list.length) return;
    box.hidden = false;
    // Collapsed by default. The retrieval list is process detail, and twelve rows of it pushed
    // the answer — the thing being waited for — below the fold. The citations at the end of the
    // answer are the readable version; this stays as a fold-out audit trail.
    // 默认收起。检索列表属于过程细节，十二行足以把用户真正在等的回答挤出视野；
    // 回答末尾的引文卡片才是可读版本，这里只作为可展开的溯源记录。
    box.innerHTML =
      '<button type="button" class="ask-src-toggle" aria-expanded="false">' +
        '<span class="ask-src-tg-ico" aria-hidden="true">' +
          '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>' +
        '</span>' +
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
