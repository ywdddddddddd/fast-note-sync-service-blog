package routers

import (
	"github.com/haierkeys/fast-note-sync-service/internal/app"
)

// The interaction bar and its particle engine.
//
// Served as one inline script rather than a /static asset: it is small, only article pages need
// it, and it must read two server-side settings (the particle count and whether the dislike
// burst is on). Inlining removes a round trip and a cache-busting version parameter for a file
// that would otherwise never change.
//
// 互动条及其粒子引擎。
//
// 作为一段内联脚本提供，而不是 /static 资源：它很小；只有文章页需要它；
// 它必须读取两个服务端设置（粒子数与点踩喷发是否开启）。
// 内联省掉一次往返，也省掉一个本来永远不会变的文件的缓存失效版本号。
//
// ── Why a canvas overlay rather than CSS animation ────────────────────────────────────────
//
// The particles need per-particle gravity, drag and a 3D spin. CSS keyframes can draw a fixed
// arc, but every particle would then follow the same path and the effect reads as a screenshot of
// an explosion rather than an explosion. A canvas gives each particle its own velocity, angle,
// size, colour and spin — which is what the effect actually is.
//
// ── 为什么用 canvas 覆盖层而不是 CSS 动画 ─────────────────────────────────────────────────
//
// 粒子需要逐粒子的重力、风阻与 3D 自转。CSS 关键帧能画出固定的弧线，但那样每颗粒子都走同一条路径，
// 效果读起来是一张爆炸的截图，而不是一次爆炸。canvas 让每颗粒子有自己的速度、角度、大小、
// 颜色与自转 —— 那才是这个效果本身。

const blogReactionScript = `
(function () {
  // The count-up runs on every page that has figures, including the home page, which has no
  // interaction bar. It is therefore set up before the bar's own guard below — an earlier version
  // returned early when the bar's config island was missing, which silently disabled the
  // dashboard's count-up on the home page.
  //
  // 数字滚动在每个有数字的页面上运行，包括没有互动条的首页。
  // 因此它在下面互动条的守卫之前装配 —— 早先的版本在互动条的配置岛不存在时提前返回，
  // 那会让首页仪表盘的数字滚动静默失效。

  // Honour the reader's motion preference. The particle burst is decoration, so it is skipped
  // entirely; the count-up is content (a number arriving), so it is shortened rather than removed.
  //
  // 尊重读者的动效偏好。粒子喷发是装饰，因此直接跳过；数字滚动是内容（一个数字抵达），
  // 因此只缩短而不取消。
  var STILL = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);

  // ── Count-up ────────────────────────────────────────────────────────────────────────────
  // The final value is what the server rendered into the element, so a reader without JavaScript
  // sees the real number rather than a zero.
  //
  // ── 数字滚动 ────────────────────────────────────────────────────────────────────────────
  // 终值是服务端已渲染进元素的内容，因此没有 JavaScript 的读者看到的是真实数字而不是零。
  function countUp(el, to) {
    if (!el) return;
    if (STILL || to === 0) { el.textContent = String(to); return; }
    var dur = 900, t0 = null;
    function frame(ts) {
      if (t0 === null) t0 = ts;
      var p = Math.min(1, (ts - t0) / dur);
      // easeOutCubic: fast arrival, soft landing. A linear ramp reads as a progress bar.
      // easeOutCubic：快速抵达、柔和落定。线性上升读起来像进度条。
      var e = 1 - Math.pow(1 - p, 3);
      el.textContent = String(Math.round(to * e));
      if (p < 1) requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);
  }

  Array.prototype.forEach.call(document.querySelectorAll('[data-countup]'), function (el) {
    countUp(el, parseInt(el.getAttribute('data-countup'), 10) || 0);
  });

  // Everything past here belongs to the interaction bar, which only article pages have.
  // 从这里往后都属于互动条，只有文章页有它。
  var cfgEl = document.getElementById('fns-react-cfg');
  if (!cfgEl) return;
  var CFG = {};
  try { CFG = JSON.parse(cfgEl.textContent) || {}; } catch (e) {}
  var COUNT = Math.max(30, Math.min(400, CFG.count || 150));
  var DISLIKE_BURST = CFG.dislikeBurst !== false;
  var SLUG = CFG.slug || '';

  // ── Particle engine ────────────────────────────────────────────────────────────────────
  // One shared overlay canvas, created on first use and reused. A canvas per burst would leak a
  // stacking context each time, and a reader who taps repeatedly is exactly the one who notices.
  //
  // ── 粒子引擎 ────────────────────────────────────────────────────────────────────────────
  // 一个共享的覆盖层 canvas，首次使用时创建然后复用。每次喷发都新建会每次泄漏一个层叠上下文，
  // 而反复点击的读者恰恰是最容易注意到这一点的人。
  var canvas = null, ctx = null, particles = [], rafId = 0, last = 0;

  function ensureCanvas() {
    if (canvas) return !!ctx;
    canvas = document.createElement('canvas');
    canvas.className = 'fns-fx';
    canvas.setAttribute('aria-hidden', 'true');
    document.body.appendChild(canvas);
    ctx = canvas.getContext('2d');
    if (ctx) resize();
    return !!ctx;
  }

  function resize() {
    if (!canvas || !ctx) return;
    // Back the canvas with device pixels so particles are not blurry on retina, then scale the
    // context once and work in CSS pixels.
    //
    // 用设备像素作而后备，使粒子在视网膜屏上不模糊；随后缩放一次上下文，之后都用 CSS 像素工作。
    var dpr = Math.min(2, window.devicePixelRatio || 1);
    canvas.width = Math.floor(window.innerWidth * dpr);
    canvas.height = Math.floor(window.innerHeight * dpr);
    canvas.style.width = window.innerWidth + 'px';
    canvas.style.height = window.innerHeight + 'px';
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }

  // Warm and bright, matching nothing else on the page on purpose: this is the one moment the
  // site is allowed to be loud.
  // 暖而亮，刻意与页面上其它东西都不同：这是本站唯一被允许喧闹的时刻。
  var LIKE_COLORS = ['#e5484d', '#f5a524', '#f2c94c', '#30a46c', '#0091ff', '#8e4ec6', '#ff7ab6'];
  // The dislike variant. Browns in the same value range as the like burst, so it reads as the
  // same physical event in a different material rather than a different effect.
  // 点踩那一版。棕色系与点赞喷发处在同一明度范围，使它读起来是同一种物理事件换了一种材质。
  var DISLIKE_COLORS = ['#7a4f21', '#8d5a26', '#5c3d1a', '#a3712f', '#6b4423', '#946b2d'];

  function spawn(x, y, isDislike) {
    var palette = isDislike ? DISLIKE_COLORS : LIKE_COLORS;
    for (var i = 0; i < COUNT; i++) {
      var angle = Math.random() * Math.PI * 2;
      // A wide but uneven speed spread: mostly slow, a few fly far. A uniform spread looks like
      // a ring rather than a burst.
      // 速度分布宽而不均：多数慢、少数飞得远。均匀分布看起来像圆环而不是喷发。
      var speed = 2 + Math.pow(Math.random(), 2) * 11;
      particles.push({
        x: x, y: y,
        vx: Math.cos(angle) * speed,
        vy: Math.sin(angle) * speed - 4,
        g: 0.32,
        // Drag varies per particle so the cloud decelerates unevenly and does not look computed.
        // 风阻逐粒子变化，使这团粒子不均匀地减速，不会显得像是算出来的。
        drag: 0.975 + Math.random() * 0.012,
        size: isDislike ? 3 + Math.random() * 5 : 2 + Math.random() * 4,
        color: palette[(Math.random() * palette.length) | 0],
        // Two spin axes with independent rates, so a particle can be caught mid-flip. The
        // z-axis scaling below is what sells it as a solid rather than a flat dot.
        // 两个自转轴、各自角速度，使粒子可能被看到翻转的中间态。
        // 下面的 z 轴缩放是让它像一个立体物而非扁平圆点的地方。
        rx: Math.random() * Math.PI, ry: Math.random() * Math.PI,
        vrx: (Math.random() - 0.5) * 0.24, vry: (Math.random() - 0.5) * 0.24,
        life: 1, decay: 0.006 + Math.random() * 0.008,
        // Mixed shards and rounds: a single shape reads as a template.
        // 方形碎片与圆形混合：只用一种形状会读起来像套模板。
        square: Math.random() < 0.35
      });
    }
  }

  function tick(ts) {
    rafId = 0;
    if (!ctx) return;
    // Frame-rate independence: a 120 Hz display must not run the physics twice as fast.
    // 帧率无关：120Hz 的屏幕不该让物理跑两倍快。
    var dt = last ? Math.min(3, (ts - last) / 16.67) : 1;
    last = ts;
    ctx.clearRect(0, 0, window.innerWidth, window.innerHeight);

    var alive = 0;
    for (var i = 0; i < particles.length; i++) {
      var p = particles[i];
      if (p.life <= 0) continue;
      p.vy += p.g * dt;
      p.vx *= Math.pow(p.drag, dt);
      p.vy *= Math.pow(p.drag, dt);
      p.x += p.vx * dt;
      p.y += p.vy * dt;
      p.rx += p.vrx * dt;
      p.ry += p.vry * dt;
      p.life -= p.decay * dt;
      if (p.life <= 0) continue;
      // Off-screen and falling: done. It cannot come back, and keeping it costs a draw per frame.
      // 出屏且下行：结束。它回不来了，留着它每帧都要多画一次。
      if (p.x < -40 || p.x > window.innerWidth + 40 || p.y > window.innerHeight + 40) {
        p.life = 0;
        continue;
      }
      alive++;
      ctx.save();
      ctx.globalAlpha = p.life < 1 ? p.life : 1;
      ctx.translate(p.x, p.y);
      ctx.rotate(p.ry);
      // Flatten along one axis by the spin's cosine — the cheapest convincing 3D turn.
      // 用自转角度的余弦在某个轴上压扁 —— 最省的可信 3D 翻转。
      ctx.scale(1, Math.abs(Math.cos(p.rx)) * 0.85 + 0.15);
      ctx.fillStyle = p.color;
      if (p.square) {
        ctx.fillRect(-p.size / 2, -p.size / 2, p.size, p.size);
      } else {
        ctx.beginPath();
        ctx.arc(0, 0, p.size / 2, 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.restore();
    }

    if (alive === 0) {
      particles.length = 0;
      ctx.clearRect(0, 0, window.innerWidth, window.innerHeight);
      last = 0;
      return;
    }
    rafId = requestAnimationFrame(tick);
  }

  function burst(el, isDislike) {
    if (STILL) return;
    if (!ensureCanvas()) return;
    var r = el.getBoundingClientRect();
    spawn(r.left + r.width / 2, r.top + r.height / 2, isDislike);
    if (!rafId) {
      last = 0;
      rafId = requestAnimationFrame(tick);
    }
  }

  window.addEventListener('resize', function () {
    // Only while something is on screen; otherwise the next burst resizes.
    // 只在屏幕上有东西时处理；否则留给下一次喷发。
    if (rafId) resize();
  });

  // ── The bar ────────────────────────────────────────────────────────────────────────────
  var bar = document.getElementById('fns-react');
  if (!bar) return;
  bar.classList.add('js');

  var likeBtn = bar.querySelector('[data-act="like"]');
  var dislikeBtn = bar.querySelector('[data-act="dislike"]');
  var likeN = bar.querySelector('[data-n="like"]');
  var dislikeN = bar.querySelector('[data-n="dislike"]');
  var shareBtn = bar.querySelector('[data-act="share"]');

  // The reader's own choice, kept locally so a reload does not forget it. It is a local
  // preference, not an identity: none of it is sent anywhere, and the server counts every
  // reaction it receives without trying to tell readers apart.
  //
  // 读者自己的选择，本地保存使刷新不会忘记。它是本地偏好而不是身份：
  // 它的任何内容都不会被发送出去，服务端只统计收到的每一次反应，并不试图区分读者。
  var KEY = 'fns:react:' + SLUG;
  var mine = null;
  try {
    var saved = JSON.parse(localStorage.getItem(KEY) || 'null');
    if (saved === 'like' || saved === 'dislike') mine = saved;
  } catch (e) {}

  function paint() {
    if (likeBtn) {
      likeBtn.classList.toggle('on', mine === 'like');
      likeBtn.setAttribute('aria-pressed', mine === 'like' ? 'true' : 'false');
    }
    if (dislikeBtn) {
      dislikeBtn.classList.toggle('on', mine === 'dislike');
      dislikeBtn.setAttribute('aria-pressed', mine === 'dislike' ? 'true' : 'false');
    }
  }

  // Apply the remembered choice on load.
  //
  // Without this call the stored value is written but never read back: the reader presses like,
  // the button lights up, the page is reloaded, and the button is dark again while localStorage
  // still says "like". Measured exactly that way — the store had the value, the button did not
  // show it.
  //
  // 加载时应用记住的选择。
  //
  // 没有这次调用，存进去的值会被写入却从不被读回：读者点了赞、按钮亮起、刷新页面，
  // 然后按钮又暗了，而 localStorage 里明明还写着 "like"。实测就是这么表现的——值在，
  // 按钮不显示它。
  paint();

  function flashError() {
    // A failed reaction is shown as a failure rather than a silently flipped button — the reader
    // pressed something and is owed an answer.
    // 失败的反应显示为失败而不是悄悄翻面的按钮 —— 读者按了东西，就该得到一个交代。
    bar.classList.add('err');
    window.setTimeout(function () { bar.classList.remove('err'); }, 1400);
  }

  function send(kind, delta) {
    if (!SLUG) return;
    fetch('/blog/react', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ slug: SLUG, kind: kind, delta: delta })
    }).then(function (r) {
      if (!r.ok) throw new Error('react failed');
      return r.json();
    }).then(function (t) {
      // The server's tally wins: two tabs reacting at once then agree instead of drifting.
      // 以服务端计数为准：两个标签页同时反应时会一致，而不是各自漂移。
      if (likeN) likeN.textContent = String(t.like);
      if (dislikeN) dislikeN.textContent = String(t.dislike);
    }).catch(flashError);
  }

  // One function for every case. The two buttons differ only in which counter they move and
  // whether a burst is wanted, so the state machine lives here once:
  //
  //   current === kind  -> retract: -1 to kind,         no burst
  //   current === null  -> add:     +1 to kind,         burst if allowed
  //   current === other -> switch:  -1 to other, +1 to kind, burst if allowed
  //
  // 一个函数处理所有情况。两个按钮的差别只有「移动哪个计数」与「要不要喷发」，
  // 因此状态机只写一次：
  //
  //   当前 === 这种     -> 取消：对这种 -1，不喷发
  //   当前 === 无       -> 新增：对这种 +1，允许时喷发
  //   当前 === 另一种   -> 切换：对另一种 -1、对这种 +1，允许时喷发
  function act(kind) {
    var prev = mine;
    var next = prev === kind ? null : kind;

    if (prev === 'like') send('like', -1);
    if (prev === 'dislike') send('dislike', -1);
    if (next) send(kind, 1);

    mine = next;
    try {
      if (next) localStorage.setItem(KEY, JSON.stringify(next));
      else localStorage.removeItem(KEY);
    } catch (e) { /* private mode: the choice is simply not remembered */ }
    paint();

    if (next) {
      var el = kind === 'like' ? likeBtn : dislikeBtn;
      if (kind === 'like' || DISLIKE_BURST) burst(el, kind === 'dislike');
    }
  }

  if (likeBtn) likeBtn.addEventListener('click', function () { act('like'); });
  if (dislikeBtn) dislikeBtn.addEventListener('click', function () { act('dislike'); });

  // ── Share ──────────────────────────────────────────────────────────────────────────────
  // Copy to clipboard, as asked. The copied text is the title and the address on two lines, so
  // pasting into a chat produces something readable rather than a bare link.
  //
  // ── 分享 ───────────────────────────────────────────────────────────────────────────────
  // 按要求复制到剪贴板。复制内容是标题与地址两行，因此粘进聊天窗口得到的是可读的东西，
  // 而不是一条裸链接。
  function fallbackCopy(text, done) {
    // For a non-secure origin the async clipboard API is unavailable. A textarea plus
    // execCommand is deprecated, but it is the only path left there, and sharing is a feature
    // rather than decoration.
    //
    // 非安全来源下异步剪贴板 API 不可用。textarea 加 execCommand 已废弃，
    // 但那是那里仅剩的路径，而分享是功能而不是装饰。
    var ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.cssText = 'position:fixed;top:-1000px;left:-1000px';
    document.body.appendChild(ta);
    ta.select();
    var ok = false;
    try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
    document.body.removeChild(ta);
    if (ok) done(); else flashError();
  }

  if (shareBtn) shareBtn.addEventListener('click', function () {
    var url = location.href.split('#')[0];
    var title = document.title || '';
    var text = title ? title + '\n' + url : url;
    function done() {
      shareBtn.classList.add('done');
      var label = shareBtn.querySelector('[data-label]');
      if (!label) return;
      var prev = label.textContent;
      label.textContent = '已复制';
      window.setTimeout(function () {
        label.textContent = prev;
        shareBtn.classList.remove('done');
      }, 1600);
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(done, function () { fallbackCopy(text, done); });
    } else {
      fallbackCopy(text, done);
    }
  });
})();
`

// blogReactionJS returns the interaction bar's script.
//
// It is a function rather than an exported constant so the template receives a template.JS at the
// point of use; nothing here depends on the request, which keeps the response identical for every
// reader and therefore friendly to a shared cache.
//
// blogReactionJS 返回互动条的脚本。
//
// 它是函数而不是导出的常量，使模板在使用处直接拿到 template.JS；这里没有任何依赖请求的东西，
// 因此每个读者得到的响应完全相同，对共享缓存友好。
func blogReactionJS() string {
	return blogReactionScript
}

// blogParticleCount reads the burst size from config.
// blogParticleCount 从配置读取喷发粒子数。
func blogParticleCount(appContainer *app.App) int {
	if appContainer == nil {
		return 150
	}
	return appContainer.Config().Blog.Particles()
}

// blogDislikeBurst reads whether the dislike burst is enabled, defaulting to on.
// blogDislikeBurst 读取点踩喷发是否开启，默认开启。
func blogDislikeBurst(appContainer *app.App) bool {
	if appContainer == nil {
		return true
	}
	return appContainer.Config().Blog.DislikeBurst()
}
