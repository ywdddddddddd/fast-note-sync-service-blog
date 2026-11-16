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

/* Sentence highlight. The Custom Highlight API paints without touching the DOM; browsers
   without it fall back to a class on the containing block, which is still useful and never
   changes the article's markup. */
::highlight(fns-audio){background-color:rgba(75,122,67,.13)}
.fns-audio-hl{background:rgba(75,122,67,.13);border-radius:3px;box-shadow:0 0 0 4px rgba(75,122,67,.13)}

@media (max-width:860px){
  .fns-audio-ctl{
    height:calc(56px + env(safe-area-inset-top,0px));
    padding-top:env(safe-area-inset-top,0px);
    background:rgba(255,255,255,.84);
    -webkit-backdrop-filter:blur(12px);backdrop-filter:blur(12px);
    border-bottom:1px solid var(--border);
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
  var expectHash = trigger.getAttribute('data-text-hash') || '';
  var storeKey = 'fns-audio:' + (src.split('?')[0] || location.pathname);
  var reduce = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  var canHighlight = !!(window.CSS && CSS.highlights && window.Highlight && window.Range);

  var audio = null, ctl = null, playBtn = null, exitBtn = null;
  var sentences = [], blocks = [], fullText = '';
  var current = -1, followPausedUntil = 0, saveTimer = 0, lastSave = 0;
  var fallbackEl = null;
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
          if (n.classList && (n.classList.contains('math') || n.classList.contains('katex') ||
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
      if (el.closest && el.closest('pre,code,table,figure,.quiz,.fns-quiz,.fns-graph,.fns-audio-ignore')) continue;
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

  function clearHighlight() {
    if (canHighlight) { try { CSS.highlights.delete('fns-audio'); } catch (e) {} }
    if (fallbackEl) { fallbackEl.classList.remove('fns-audio-hl'); fallbackEl = null; }
  }

  function paint(i) {
    clearHighlight();
    if (i < 0 || i >= sentences.length) return;
    if (canHighlight && sentences[i].range) {
      try {
        var h = new Highlight(sentences[i].range);
        CSS.highlights.set('fns-audio', h);
        return;
      } catch (e) {}
    }
    // Fallback: mark the whole containing block. It is coarser than a sentence but keeps the
    // reading position visible on browsers without Custom Highlight.
    //
    // 回退：标出整块。比逐句粗，但在不支持 Custom Highlight 的浏览器里仍能看见读到哪。
    var r = sentences[i].range;
    if (!r) return;
    var node = r.startContainer.nodeType === 1 ? r.startContainer : r.startContainer.parentElement;
    var block = node && node.closest ? node.closest('p,li,h2,h3,h4') : null;
    if (block) { block.classList.add('fns-audio-hl'); fallbackEl = block; }
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
  }

  function exit() {
    if (exiting) return;
    exiting = true;
    save();
    clearHighlight();
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

  function start() {
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

  trigger.addEventListener('click', start);
})();
`
