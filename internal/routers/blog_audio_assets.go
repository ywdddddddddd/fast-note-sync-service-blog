package routers

// The audio player is deliberately tiny: a single deferred script and a scoped stylesheet.
// It is only served/loaded from articles that actually have an entry in audio.json.
//
// 播放器刻意做小：一个 defer 脚本加一段限定作用域的样式，并且只有 audio.json 里确有条目的文章
// 才会加载它。

const blogAudioCSS = `
/* ── 听全文 ─────────────────────────────────────────────────────────── */
.listen-line{
  display:inline-flex;align-items:center;gap:.4rem;
  margin:-.35rem 0 1.5rem;padding:0;border:0;background:none;
  font:inherit;font-size:.8125rem;color:var(--muted-foreground);
  cursor:pointer;transition:color .16s ease;
}
.listen-line:hover{color:var(--foreground)}
.listen-line svg{width:15px;height:15px;flex:none}
body.is-listening .listen-line{display:none}
/* The desktop top bar keeps its brand and progress, but its navigation yields the right-hand
   side to the exit button. visibility (not display) keeps the bar's layout from jumping. */
body.is-listening .topnav,body.is-listening .crumb{visibility:hidden}

.fns-audio-ctl{
  position:fixed;left:0;right:0;top:0;height:64px;z-index:70;
  display:flex;align-items:center;justify-content:center;
  pointer-events:none;
}
.fns-audio-play{
  pointer-events:auto;position:relative;
  width:42px;height:42px;border-radius:50%;
  border:1px solid rgba(0,0,0,.06);
  background:rgba(52,50,45,.10);
  -webkit-backdrop-filter:blur(10px);backdrop-filter:blur(10px);
  color:var(--foreground);display:flex;align-items:center;justify-content:center;
  cursor:pointer;box-shadow:0 2px 10px rgba(0,0,0,.06);
  transition:background .18s ease,transform .18s ease;
}
.fns-audio-play:hover{background:rgba(52,50,45,.17)}
.fns-audio-play:active{transform:scale(.94)}
.fns-audio-play svg{
  position:absolute;width:15px;height:15px;
  transition:opacity .16s ease,transform .16s ease;
}
.fns-audio-play .ico-play{opacity:0;transform:scale(.55) rotate(-25deg)}
.fns-audio-play .ico-pause{opacity:1;transform:none}
.fns-audio-play.is-paused .ico-play{opacity:1;transform:none}
.fns-audio-play.is-paused .ico-pause{opacity:0;transform:scale(.55) rotate(25deg)}
.fns-audio-exit{
  pointer-events:auto;position:absolute;right:18px;top:50%;transform:translateY(-50%);
  width:34px;height:34px;border-radius:50%;
  border:1px solid rgba(0,0,0,.06);
  background:rgba(52,50,45,.08);
  -webkit-backdrop-filter:blur(10px);backdrop-filter:blur(10px);
  color:var(--muted-foreground);display:flex;align-items:center;justify-content:center;
  cursor:pointer;transition:background .18s ease,color .18s ease;
}
.fns-audio-exit:hover{background:rgba(52,50,45,.15);color:var(--foreground)}
.fns-audio-exit svg{width:13px;height:13px}

/* Sentence highlight. We paint our own fixed rectangles from the current sentence's client
   rects instead of the Custom Highlight API: Safari's highlight registry can keep the previous
   range painted after the registry entry is replaced, which is exactly the "old sentence stays
   green" residue. Clearing every mark before drawing the next one makes that impossible. */
.fns-audio-mark{
  position:fixed;z-index:6;pointer-events:none;
  background:rgba(75,122,67,.15);border-radius:3px;
}
/* Dark mode: the light green at 15% over #1a1a1a is nearly invisible. Lift both the colour and
   the alpha, and do it for the explicit toggle (html.dark) and for the system preference. */
html.dark .fns-audio-mark{background:rgba(122,168,112,.24)}
@media (prefers-color-scheme:dark){
  html:not(.light) .fns-audio-mark{background:rgba(122,168,112,.24)}
}
html.dark .fns-audio-play{background:rgba(237,237,237,.13);border-color:rgba(255,255,255,.10)}
html.dark .fns-audio-play:hover{background:rgba(237,237,237,.20)}
html.dark .fns-audio-exit{background:rgba(237,237,237,.10);border-color:rgba(255,255,255,.10)}
html.dark .fns-audio-exit:hover{background:rgba(237,237,237,.18)}
@media (prefers-color-scheme:dark){
  html:not(.light) .fns-audio-play{background:rgba(237,237,237,.13);border-color:rgba(255,255,255,.10)}
  html:not(.light) .fns-audio-play:hover{background:rgba(237,237,237,.20)}
  html:not(.light) .fns-audio-exit{background:rgba(237,237,237,.10);border-color:rgba(255,255,255,.10)}
  html:not(.light) .fns-audio-exit:hover{background:rgba(237,237,237,.18)}
}
.listen-line[aria-busy="true"]{opacity:.55;pointer-events:none}
.fns-audio-modal{
  position:fixed;inset:0;z-index:80;display:flex;align-items:center;justify-content:center;
  padding:20px;background:rgba(0,0,0,.18);
}
.fns-audio-card{
  width:min(360px,100%);background:var(--card);border:1px solid var(--border);
  border-radius:14px;box-shadow:0 24px 60px rgba(0,0,0,.18);padding:22px 22px 16px;
}
.fns-audio-card h3{margin:0 0 10px;font-size:16px;font-weight:650;display:flex;align-items:center}
.fns-audio-card p{margin:0;font-size:13px;line-height:1.8;color:var(--muted-foreground,#5c5a56)}
.fns-audio-card p.fns-audio-sub{margin-top:8px;font-size:12px;color:var(--muted-foreground)}
.fns-audio-actions{display:flex;justify-content:flex-end;gap:10px;margin-top:18px}
.fns-audio-btn{
  border:1px solid var(--border);background:var(--card);color:var(--foreground);
  border-radius:999px;padding:7px 14px;font-size:12px;cursor:pointer;
}
.fns-audio-btn.primary{background:var(--foreground);color:var(--card);border-color:var(--foreground)}
.fns-audio-spinner{
  width:16px;height:16px;border:2px solid rgba(0,0,0,.12);border-top-color:var(--foreground);
  border-radius:50%;animation:fns-audio-spin .8s linear infinite;display:inline-block;
  margin-right:8px;flex:none;
}
html.dark .fns-audio-spinner{border-color:rgba(255,255,255,.14);border-top-color:var(--foreground)}
@media (prefers-color-scheme:dark){
  html:not(.light) .fns-audio-spinner{border-color:rgba(255,255,255,.14);border-top-color:var(--foreground)}
}
@keyframes fns-audio-spin{to{transform:rotate(360deg)}}
@media (prefers-reduced-motion: reduce){.fns-audio-spinner{animation:none}}

@media (max-width:860px){
  .fns-audio-ctl{
    height:calc(56px + env(safe-area-inset-top,0px));
    padding-top:env(safe-area-inset-top,0px);
    background:rgba(255,255,255,.84);
    -webkit-backdrop-filter:blur(12px);backdrop-filter:blur(12px);
    border-bottom:1px solid var(--border);
  }
  html.dark .fns-audio-ctl{background:rgba(26,26,26,.86)}
  @media (prefers-color-scheme:dark){
    html:not(.light) .fns-audio-ctl{background:rgba(26,26,26,.86)}
  }
  .fns-audio-play{width:40px;height:40px}
  .fns-audio-exit{right:14px;width:32px;height:32px}
}
@media (prefers-reduced-motion: reduce){
  .fns-audio-play,.fns-audio-play svg,.fns-audio-exit{transition:none}
}
`

// blogAudioScript is served from /blog/audio.js. It is written as ES5 on purpose: the blog
// still gets opened on older in-app browsers, and the player is small enough that a build
// step would cost more than it saves.
//
// blogAudioScript 由 /blog/audio.js 提供。刻意写成 ES5：博客仍会被较旧的 App 内置浏览器打开，
// 而播放器小到不值得为它引入构建步骤。
const blogAudioScript = `
(function () {
  'use strict';
  var trigger = document.getElementById('fns-listen');
  if (!trigger) return;

  var body = document.body;
  var src = trigger.getAttribute('data-src') || '';
  var timingsURL = trigger.getAttribute('data-timings') || '';
  var slug = trigger.getAttribute('data-slug') || '';
  var ready = trigger.getAttribute('data-ready') === '1';
  var expectHash = trigger.getAttribute('data-text-hash') || '';
  var storeKey = 'fns-audio:' + (src.split('?')[0] || location.pathname);
  var reduce = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);

  var audio = null, ctl = null, playBtn = null, exitBtn = null;
  var sentences = [], blocks = [], fullText = '';
  var current = -1, followPausedUntil = 0, saveTimer = 0, lastSave = 0;
  var marks = [], currentRange = null, markRaf = 0;
  var exiting = false;

  function norm(s) { return (s || '').replace(/\s+/g, ' ').trim(); }
  function blockChild(tag) {
    return /^(ul|ol|pre|table|figure|blockquote|p|h[1-6]|div|hr)$/.test(tag);
  }

  // The article's speakable blocks. The generator uses exactly the same selector and the
  // same "own text" rule, so the transcript and the page agree character for character.
  //
  // 文章里可朗读的块。生成脚本使用完全相同的选择器与「自身文本」规则，逐字一致。
  function blockMap(el) {
    var chars = [], map = [];
    function walk(node) {
      for (var n = node.firstChild; n; n = n.nextSibling) {
        if (n.nodeType === 3) {
          var s = n.nodeValue || '';
          for (var i = 0; i < s.length; i++) { chars.push(s[i]); map.push([n, i]); }
        } else if (n.nodeType === 1) {
          var tag = (n.tagName || '').toLowerCase();
          if (blockChild(tag)) continue;
          if (n.classList && (n.classList.contains('math') || n.classList.contains('math-inline') ||
              n.classList.contains('math-block') || n.classList.contains('katex') ||
              n.classList.contains('fns-audio-ignore'))) continue;
          walk(n);
        }
      }
    }
    walk(el);

    var out = [], om = [], lastSpace = false;
    for (var i = 0; i < chars.length; i++) {
      var ch = chars[i];
      if (/\s/.test(ch)) {
        if (!lastSpace && out.length) { out.push(' '); om.push(map[i]); lastSpace = true; }
      } else {
        out.push(ch); om.push(map[i]); lastSpace = false;
      }
    }
    while (out.length && out[out.length - 1] === ' ') { out.pop(); om.pop(); }
    return { text: out.join(''), map: om };
  }

  function buildArticleText() {
    var root = document.querySelector('.post-body');
    if (!root) return '';
    var els = root.querySelectorAll('h2,h3,h4,p,li');
    blocks = [];
    var full = '';
    for (var i = 0; i < els.length; i++) {
      var el = els[i];
      if (el.closest && el.closest('pre,code,table,figure,.quiz,.fns-quiz,.fns-graph,.math,.math-inline,.math-block,.katex,.fns-audio-ignore')) continue;
      var m = blockMap(el);
      if (!m.text) continue;
      blocks.push({ start: full.length, text: m.text, map: m.map, el: el });
      full += m.text + '\n';
    }
    if (full.charAt(full.length - 1) === '\n') full = full.slice(0, -1);
    return full;
  }

  function locate(offset) {
    for (var i = 0; i < blocks.length; i++) {
      var b = blocks[i];
      if (offset >= b.start && offset < b.start + b.text.length) {
        return { block: b, local: offset - b.start };
      }
    }
    return null;
  }

  function rangeFor(start, end) {
    var a = locate(start), b = locate(end - 1);
    if (!a || !b || !a.block.map[a.local] || !b.block.map[b.local]) return null;
    var sa = a.block.map[a.local], sb = b.block.map[b.local];
    var r = document.createRange();
    r.setStart(sa[0], sa[1]);
    r.setEnd(sb[0], sb[1] + 1);
    return r;
  }

  function clearMarks() {
    for (var i = 0; i < marks.length; i++) {
      if (marks[i].parentNode) marks[i].parentNode.removeChild(marks[i]);
    }
    marks = [];
    currentRange = null;
  }

  // One rectangle per line box: a sentence that wraps gets several marks, and the page can
  // scroll underneath them without the marks drifting because they are re-laid out on scroll.
  //
  // 每个行盒一个矩形：换行的句子会有多个遮罩；它们按当前 rect 重排，页面滚动时不会漂移。
  function layoutMarks() {
    if (!currentRange) return;
    var rects = currentRange.getClientRects();
    // A range over inline elements returns one rect per text run, so the same visual line can
    // arrive as a dozen overlapping boxes. Group by line and merge, or overlapping translucent
    // boxes stack into a darker, uneven band.
    //
    // range 跨内联元素时会给同一行返回多个文本 run 的矩形；按行分组合并，否则半透明盒子叠加
    // 会变成深浅不一的色带。
    var groups = {}, valid = [];
    for (var i = 0; i < rects.length; i++) {
      var r = rects[i];
      if (!(r.width > 0 && r.height > 0)) continue;
      var key = Math.round(r.top / 2) + ':' + Math.round(r.height / 2);
      var g = groups[key];
      if (!g) {
        g = groups[key] = { left: r.left, right: r.right, top: r.top, bottom: r.bottom };
        valid.push(g);
      } else {
        if (r.left < g.left) g.left = r.left;
        if (r.right > g.right) g.right = r.right;
        if (r.top < g.top) g.top = r.top;
        if (r.bottom > g.bottom) g.bottom = r.bottom;
      }
    }
    while (marks.length > valid.length) {
      var extra = marks.pop();
      if (extra.parentNode) extra.parentNode.removeChild(extra);
    }
    while (marks.length < valid.length) {
      var m = document.createElement('div');
      m.className = 'fns-audio-mark';
      document.body.appendChild(m);
      marks.push(m);
    }
    for (var j = 0; j < valid.length; j++) {
      marks[j].style.left = valid[j].left + 'px';
      marks[j].style.top = valid[j].top + 'px';
      marks[j].style.width = (valid[j].right - valid[j].left) + 'px';
      marks[j].style.height = (valid[j].bottom - valid[j].top) + 'px';
    }
  }

  function scheduleMarks() {
    if (markRaf) return;
    markRaf = window.requestAnimationFrame(function () {
      markRaf = 0;
      layoutMarks();
    });
  }

  function paint(i) {
    clearMarks();
    if (i < 0 || i >= sentences.length) return;
    var r = sentences[i].range;
    if (!r) return;
    currentRange = r;
    layoutMarks();
  }

  function sentenceAt(ms) {
    var lo = 0, hi = sentences.length - 1, ans = -1;
    while (lo <= hi) {
      var mid = (lo + hi) >> 1;
      if (sentences[mid].s <= ms) { ans = mid; lo = mid + 1; } else { hi = mid - 1; }
    }
    return ans;
  }

  function scrollToCurrent(i) {
    if (Date.now() < followPausedUntil) return;
    var r = sentences[i] && sentences[i].range;
    if (!r) return;
    var rect = r.getBoundingClientRect();
    var h = window.innerHeight;
    if (rect.top >= h * 0.28 && rect.bottom <= h * 0.72) return;
    var top = window.scrollY + rect.top - h * 0.42;
    if (top < 0) top = 0;
    window.scrollTo({ top: top, behavior: reduce ? 'auto' : 'smooth' });
  }

  function save() {
    if (!audio || !isFinite(audio.currentTime)) return;
    try {
      localStorage.setItem(storeKey, JSON.stringify({ t: audio.currentTime, ts: Date.now() }));
    } catch (e) {}
  }

  function loadSaved() {
    try {
      var raw = localStorage.getItem(storeKey);
      if (!raw) return null;
      var o = JSON.parse(raw);
      return o && typeof o.t === 'number' ? o : null;
    } catch (e) { return null; }
  }

  function setPausedUI() {
    if (!playBtn) return;
    var paused = !audio || audio.paused;
    playBtn.classList.toggle('is-paused', paused);
    playBtn.setAttribute('aria-label', paused ? '播放' : '暂停');
  }

  function setMediaSession() {
    if (!('mediaSession' in navigator)) return;
    try {
      var titleEl = document.querySelector('.post .pt') || document.querySelector('h1');
      var brandEl = document.querySelector('.brand-name');
      navigator.mediaSession.metadata = new MediaMetadata({
        title: titleEl ? titleEl.textContent.trim() : document.title,
        artist: brandEl ? brandEl.textContent.trim() : '',
        album: '听全文',
        artwork: [{ src: location.origin + '/blog/icon.svg', sizes: 'any', type: 'image/svg+xml' }]
      });
      navigator.mediaSession.setActionHandler('play', function () { if (audio) audio.play(); });
      navigator.mediaSession.setActionHandler('pause', function () { if (audio) audio.pause(); });
      navigator.mediaSession.setActionHandler('seekbackward', function () {
        if (audio) audio.currentTime = Math.max(0, audio.currentTime - 15);
      });
      navigator.mediaSession.setActionHandler('seekforward', function () {
        if (audio) audio.currentTime = Math.min(audio.duration || 1e9, audio.currentTime + 15);
      });
      navigator.mediaSession.setActionHandler('seekto', function (d) {
        if (!audio || d.seekTime == null) return;
        if (d.fastSeek && 'fastSeek' in audio) audio.fastSeek(d.seekTime);
        else audio.currentTime = d.seekTime;
      });
    } catch (e) {}
  }

  function clearMediaSession() {
    if (!('mediaSession' in navigator)) return;
    try { navigator.mediaSession.metadata = null; } catch (e) {}
    ['play', 'pause', 'seekbackward', 'seekforward', 'seekto'].forEach(function (a) {
      try { navigator.mediaSession.setActionHandler(a, null); } catch (e) {}
    });
  }

  function makeControls() {
    ctl = document.createElement('div');
    ctl.className = 'fns-audio-ctl';
    ctl.setAttribute('role', 'group');
    ctl.setAttribute('aria-label', '听全文控制');

    playBtn = document.createElement('button');
    playBtn.type = 'button';
    playBtn.className = 'fns-audio-play';
    playBtn.setAttribute('aria-label', '暂停');
    playBtn.innerHTML =
      '<svg class="ico-play" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M4 2.5v11l9-5.5z"/></svg>' +
      '<svg class="ico-pause" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M4 2h3v12H4zM9 2h3v12H9z"/></svg>';
    playBtn.addEventListener('click', function () {
      if (!audio) return;
      if (audio.paused) audio.play(); else audio.pause();
    });

    exitBtn = document.createElement('button');
    exitBtn.type = 'button';
    exitBtn.className = 'fns-audio-exit';
    exitBtn.setAttribute('aria-label', '退出听全文');
    exitBtn.innerHTML = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8"/></svg>';
    exitBtn.addEventListener('click', exit);

    ctl.appendChild(playBtn);
    ctl.appendChild(exitBtn);
    body.appendChild(ctl);
  }

  function onTime() {
    if (!audio) return;
    var ms = audio.currentTime * 1000;
    var i = sentenceAt(ms);
    if (i !== current) {
      current = i;
      paint(i);
      if (i >= 0) scrollToCurrent(i);
    }
    if (Date.now() - lastSave > 5000) { lastSave = Date.now(); save(); }
    if ('mediaSession' in navigator && audio.duration && isFinite(audio.duration)) {
      try {
        navigator.mediaSession.setPositionState({
          duration: audio.duration,
          playbackRate: audio.playbackRate || 1,
          position: Math.min(audio.currentTime, audio.duration)
        });
      } catch (e) {}
    }
    // Images/fonts finishing late can shift the text under a fixed mark; re-laying it out on
    // every timeupdate (about 4 Hz) keeps the rectangle on the sentence without a second timer.
    //
    // 图片/字体晚到会把文字顶移；每次 timeupdate（约 4Hz）重排一次，不需要额外定时器。
    scheduleMarks();
  }

  function exit() {
    if (exiting) return;
    exiting = true;
    save();
    clearMarks();
    clearMediaSession();
    if (saveTimer) { clearInterval(saveTimer); saveTimer = 0; }
    if (audio) {
      try { audio.pause(); } catch (e) {}
      try { audio.removeAttribute('src'); audio.load(); } catch (e) {}
      if (audio.parentNode) audio.parentNode.removeChild(audio);
      audio = null;
    }
    if (ctl && ctl.parentNode) ctl.parentNode.removeChild(ctl);
    ctl = null; playBtn = null; exitBtn = null;
    body.classList.remove('is-listening');
    sentences = []; blocks = []; fullText = ''; current = -1;
    trigger.removeAttribute('aria-busy');
    exiting = false;
  }

  function applyTimings(data) {
    if (!data || !data.sentences || !data.sentences.length) return;
    fullText = buildArticleText();
    if (!fullText) return;
    var cursor = 0;
    sentences = [];
    for (var i = 0; i < data.sentences.length; i++) {
      var s = data.sentences[i];
      var idx = fullText.indexOf(s.t, cursor);
      if (idx < 0) idx = fullText.indexOf(s.t);
      var r = null;
      if (idx >= 0) {
        r = rangeFor(idx, idx + s.t.length);
        cursor = idx + s.t.length;
      }
      sentences.push({ s: s.s, e: s.e, t: s.t, range: r });
    }
    current = -1;
    if (audio && audio.currentTime > 0) {
      current = sentenceAt(audio.currentTime * 1000);
      paint(current);
    }
  }

  function verifyAndApply(data) {
    if (!data || !data.textHash) { applyTimings(data); return; }
    if (!window.crypto || !crypto.subtle || !window.TextEncoder) { applyTimings(data); return; }
    crypto.subtle.digest('SHA-256', new TextEncoder().encode(fullText || buildArticleText()))
      .then(function (buf) {
        var bytes = new Uint8Array(buf), hex = '';
        for (var i = 0; i < bytes.length; i++) hex += ('0' + bytes[i].toString(16)).slice(-2);
        if (data.textHash && hex !== data.textHash) {
          // The page and the audio were generated from different text. Keep playing (the voice
          // is still this article) but do not paint a highlight that no longer points at the
          // spoken sentence.
          //
          // 页面与音频来自不同文本。继续播放（声音仍是这篇文章），但不画已经对不上的高亮。
          if (window.console) console.warn('[fns-audio] transcript does not match this page; highlight disabled');
          sentences = [];
          return;
        }
        applyTimings(data);
      })
      .catch(function () { applyTimings(data); });
  }

  var modal = null, pollTimer = 0, pollStart = 0;

  function closeModal() {
    if (pollTimer) { window.clearInterval(pollTimer); pollTimer = 0; }
    if (modal && modal.parentNode) modal.parentNode.removeChild(modal);
    modal = null;
  }

  function makeModal() {
    closeModal();
    var wrap = document.createElement('div');
    wrap.className = 'fns-audio-modal';
    var card = document.createElement('div');
    card.className = 'fns-audio-card';
    wrap.appendChild(card);
    wrap.addEventListener('click', function (e) { if (e.target === wrap) closeModal(); });
    document.body.appendChild(wrap);
    modal = wrap;
    return card;
  }

  function modalButton(text, primary, onClick) {
    var b = document.createElement('button');
    b.type = 'button';
    b.className = 'fns-audio-btn' + (primary ? ' primary' : '');
    b.textContent = text;
    b.addEventListener('click', onClick);
    return b;
  }

  // The browser can estimate the wait from the same text it would send to the server; no extra
  // request is needed just to render the confirmation.
  //
  // 浏览器用与服务端相同的正文估算等待时间，仅为了显示确认框不需要额外请求。
  function estimateSec() {
    var text = buildArticleText();
    return 15 + Math.round(text.length / 70);
  }

  function minutesText(sec) {
    var m = Math.max(1, Math.ceil((sec || 0) / 60));
    return '约 ' + m + ' 分钟';
  }

  function showConfirm(est) {
    var card = makeModal();
    var h = document.createElement('h3');
    h.textContent = '听全文';
    var p = document.createElement('p');
    p.textContent = '首次收听需要先生成语音，预计' + minutesText(est || estimateSec()) +
      '。生成完成后会自动播放，是否继续？';
    var actions = document.createElement('div');
    actions.className = 'fns-audio-actions';
    actions.appendChild(modalButton('取消', false, closeModal));
    actions.appendChild(modalButton('生成并播放', true, function () {
      closeModal();
      requestGenerate();
    }));
    card.appendChild(h);
    card.appendChild(p);
    card.appendChild(actions);
  }

  function requestStatus() {
    return fetch('/blog/audio/status?slug=' + encodeURIComponent(slug) + '&t=' + Date.now(), { cache: 'no-store' })
      .then(function (r) { return r.json(); });
  }

  function requestGenerate() {
    trigger.setAttribute('aria-busy', 'true');
    fetch('/blog/audio/prepare', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ slug: slug })
    }).then(function (r) {
      return r.json().then(function (b) { return { ok: r.ok, body: b }; });
    }).then(function (res) {
      trigger.removeAttribute('aria-busy');
      var b = res.body || {};
      if (!res.ok) { showError(b.error || '生成请求失败，请稍后重试'); return; }
      if (b.status === 'ready') { begin(b.src, b.timings); return; }
      if (b.status === 'queued' || b.status === 'generating') { showBusy(b.status, b.estimateSec); return; }
      if (b.status === 'failed') { showError(b.error || '生成失败'); return; }
      showConfirm(b.estimateSec);
    }).catch(function () {
      trigger.removeAttribute('aria-busy');
      showError('网络错误，请稍后重试');
    });
  }

  function showBusy(status, est) {
    var card = makeModal();
    var h = document.createElement('h3');
    var spin = document.createElement('span');
    spin.className = 'fns-audio-spinner';
    var title = document.createElement('span');
    title.textContent = status === 'generating' ? '正在生成语音…' : '排队等待生成…';
    h.appendChild(spin);
    h.appendChild(title);
    var p = document.createElement('p');
    p.className = 'fns-audio-sub';
    var started = Date.now();
    function tick() {
      var elapsed = Math.round((Date.now() - started) / 1000);
      p.textContent = '预计' + minutesText(est) + ' · 已用 ' + elapsed + ' 秒';
    }
    tick();
    var actions = document.createElement('div');
    actions.className = 'fns-audio-actions';
    actions.appendChild(modalButton('后台生成', false, closeModal));
    card.appendChild(h);
    card.appendChild(p);
    card.appendChild(actions);

    pollStart = started;
    pollTimer = window.setInterval(function () {
      requestStatus().then(function (st) {
        if (!modal) return;
        if (st.status === 'ready') {
          closeModal();
          begin(st.src, st.timings);
          return;
        }
        if (st.status === 'failed') {
          closeModal();
          showError(st.error || '生成失败');
          return;
        }
        tick();
      }).catch(function () { /* transient network error: keep polling */ });
    }, 2000);
  }

  function showError(msg) {
    var card = makeModal();
    var h = document.createElement('h3');
    h.textContent = '生成失败';
    var p = document.createElement('p');
    p.textContent = msg || '请稍后重试';
    var actions = document.createElement('div');
    actions.className = 'fns-audio-actions';
    actions.appendChild(modalButton('关闭', false, closeModal));
    actions.appendChild(modalButton('重试', true, function () {
      closeModal();
      requestGenerate();
    }));
    card.appendChild(h);
    card.appendChild(p);
    card.appendChild(actions);
  }

  // The entry is present on every article. A ready article plays immediately; one without
  // audio first checks the server (a cached page may be stale), then offers the estimate.
  //
  // 每篇文章都有入口。已有音频的直接播放；没有的先问服务端（页面可能被缓存而状态已变），
  // 再给出预计时间让用户确认。
  function entryClick() {
    if (audio) return;
    if (ready) { begin(src, timingsURL); return; }
    trigger.setAttribute('aria-busy', 'true');
    requestStatus().then(function (st) {
      trigger.removeAttribute('aria-busy');
      if (st.status === 'ready') { begin(st.src, st.timings); return; }
      if (st.status === 'queued' || st.status === 'generating') { showBusy(st.status, st.estimateSec); return; }
      if (st.status === 'failed') { showError(st.error || '生成失败'); return; }
      showConfirm(st.estimateSec);
    }).catch(function () {
      trigger.removeAttribute('aria-busy');
      showConfirm();
    });
  }

  function begin(srcArg, timingsArg) {
    if (audio) return;
    closeModal();
    src = srcArg || src;
    timingsURL = timingsArg || timingsURL;
    trigger.removeAttribute('aria-busy');
    var saved = loadSaved();
    body.classList.add('is-listening');
    trigger.setAttribute('aria-busy', 'true');
    makeControls();

    audio = document.createElement('audio');
    audio.preload = 'none';
    audio.src = src;
    audio.style.display = 'none';
    body.appendChild(audio);

    audio.addEventListener('loadedmetadata', function () {
      if (saved && saved.t > 3 && isFinite(audio.duration) && saved.t < audio.duration - 3) {
        try { audio.currentTime = saved.t; } catch (e) {}
      }
    });
    audio.addEventListener('timeupdate', onTime);
    audio.addEventListener('play', setPausedUI);
    audio.addEventListener('pause', function () { setPausedUI(); save(); });
    audio.addEventListener('ended', function () {
      try { localStorage.removeItem(storeKey); } catch (e) {}
      exit();
    });
    setPausedUI();

    // Start inside the click handler: iOS grants media playback to the gesture, and an await
    // before play() can lose that grant. The transcript is fetched in parallel and only adds
    // the highlight when it arrives.
    //
    // 在点击处理器里直接开播：iOS 把媒体播放权限绑在这次手势上，await 之后再 play() 可能丢权限。
    // 时间轴并行拉取，到了之后只负责补高亮。
    var p = audio.play();
    if (p && p.catch) p.catch(function () { setPausedUI(); });

    setMediaSession();
    saveTimer = window.setInterval(save, 5000);

    if (timingsURL) {
      fetch(timingsURL, { cache: 'no-cache' })
        .then(function (r) { if (!r.ok) throw new Error('timings ' + r.status); return r.json(); })
        .then(verifyAndApply)
        .catch(function (e) { if (window.console) console.warn('[fns-audio] timings unavailable', e); })
        .then(function () { trigger.removeAttribute('aria-busy'); });
    }
  }

  // A manual scroll means the reader is looking somewhere else; following yields for five
  // seconds and then resumes on the next sentence. Programmatic smooth scrolling does not
  // emit these events, so the two never fight.
  //
  // 手动滚动表示读者正在看别处；跟随让位五秒，之后在下一句恢复。程序化平滑滚动不触发这些事件，
  // 两者不会互相打断。
  function pauseFollow() { followPausedUntil = Date.now() + 5000; }
  ['wheel', 'touchmove'].forEach(function (ev) {
    window.addEventListener(ev, pauseFollow, { passive: true });
  });
  window.addEventListener('keydown', function (e) {
    if (/^(ArrowUp|ArrowDown|PageUp|PageDown|Home|End| )$/.test(e.key)) pauseFollow();
  });
  window.addEventListener('pagehide', save);
  // The marks are viewport-fixed; their source range is not. Recompute on scroll/resize so a
  // highlight follows the text instead of staying at the old screen position.
  //
  // 遮罩是视口固定的，源 range 不是；滚动/缩放时重算，让高亮跟着文字走而不是停在旧位置。
  window.addEventListener('scroll', scheduleMarks, { passive: true });
  window.addEventListener('resize', scheduleMarks);

  trigger.addEventListener('click', entryClick);
})();
`
