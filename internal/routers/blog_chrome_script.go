package routers

// The card page's two scripts live here rather than inside the template so the markup stays
// readable: the dock wiring alone is a hundred lines of state machine, and inlining it pushed the
// page template past the point where the HTML could be scanned.
//
// 名片页的两个脚本放在这里而不是模板里，是为了让标记保持可读：
// 单是胶囊接线就有一百行状态机，内联它会把页面模板推到 HTML 无法扫读的长度。

// blogNavDockJS is the capsule used on the list and archive pages.
//
// Those pages have no sheets to open — the folder tree, the recommendations and the heat map all
// live on the card page — so their keys are plain links back to the places those things actually
// are. It is honest about it: the labels say where they go, and there is no key that pretends to
// open something on a page that does not contain it.
//
// blogNavDockJS 是列表页与归档页使用的胶囊。
//
// 这两页没有可打开的面板——目录树、推荐、热力图都在名片页——因此它们的键是普通链接，
// 指回那些东西真正所在的地方。这一点是诚实的：标签写着它们去哪，
// 也没有任何一个键假装在这一页上打开一个它并不包含的东西。
func blogNavDockJS() string {
	return `(function () {
  var dock = document.getElementById('fns-dock');
  if (!dock) { return; }
  var orb = document.getElementById('fns-orb');
  function setOpen(on) {
    dock.classList.toggle('is-open', on);
    if (orb) { orb.setAttribute('aria-expanded', on ? 'true' : 'false'); }
  }
  if (orb) {
    orb.addEventListener('click', function (ev) {
      ev.stopPropagation();
      var opening = !dock.classList.contains('is-open');
      // Expanding announces itself first: the orb plays once, then the dock opens. The animation is
      // the announcement, not decoration.
      //
      // A second press while it plays is dropped rather than queued. Two presses must not leave the
      // dock in a state the reader did not ask for, and they must not stack two callbacks onto one
      // animation.
      //
      // 展开先自我通报：球播一遍，然后胶囊打开。这段动画是告知，不是装饰。
      //
      // 播放期间的第二次按压直接丢弃，而不是排队。两次按压不该把胶囊留在一个读者没要求的状态里，
      // 也不该在同一个动画上叠两个回调。
      if (opening && window.fnsOrbPlay) {
        if (orb.getAttribute('data-playing') === '1') { return; }
        orb.setAttribute('data-playing', '1');
        window.fnsOrbPlay(function () {
          orb.removeAttribute('data-playing');
          setOpen(true);
        });
        return;
      }
      setOpen(!dock.classList.contains('is-open'));
    });
  }
  document.addEventListener('click', function (ev) {
    if (dock.classList.contains('is-open') && !dock.contains(ev.target)) { setOpen(false); }
  });
  Array.prototype.forEach.call(dock.querySelectorAll('a.dock-b'), function (a) {
    a.addEventListener('click', function () { setOpen(false); });
  });

  var toastTimer = 0;
  function toast(msg) {
    var el = document.getElementById('fns-dock-toast');
    if (!el) {
      el = document.createElement('div');
      el.id = 'fns-dock-toast';
      el.className = 'dock-toast';
      el.setAttribute('role', 'status');
      document.body.appendChild(el);
    }
    el.textContent = msg;
    el.classList.add('is-on');
    if (toastTimer) { clearTimeout(toastTimer); }
    toastTimer = setTimeout(function () { el.classList.remove('is-on'); }, 2800);
  }
  var ask = document.getElementById('dock-ask');
  if (ask && ask.hasAttribute('data-ask-off')) {
    ask.addEventListener('click', function () {
      toast(ask.getAttribute('title') || '问答未启用');
    });
  }

  // Cover spinners: hidden as soon as each image resolves. An error counts as resolved too — a
  // broken cover should stop spinning, not advertise itself forever.
  //
  // 封面指示器：每张图一解析就隐藏。加载失败也算作已解析——坏掉的封面应该停止转动，
  // 而不是永远宣称自己在加载。
  Array.prototype.forEach.call(document.querySelectorAll('.post-card'), function (card) {
    var img = card.querySelector('img.cover, video.cover');
    if (!img) { return; }
    var done = function () { card.classList.add('is-cover-ready'); };
    if (img.tagName === 'IMG') {
      if (img.complete && img.naturalWidth > 0) { done(); return; }
      img.addEventListener('load', done);
      img.addEventListener('error', done);
    } else {
      img.addEventListener('loadeddata', done);
      img.addEventListener('error', done);
    }
  });
})();`
}

// blogDockJS wires the three-key capsule on the card page.
//
// The capsule keeps only the keys this page can actually act on:
//
//  1. 目录 opens the site navigation as a bottom sheet, because this page has no rail to borrow.
//  2. 漫游 navigates to /blog/walk — there are no cards here to pick from.
//  3. 问道 opens the knowledge-base panel when AI is enabled.
//
// 推荐 and 活跃度 are deliberately absent: the recommendation block belongs to the list rail, and
// the heat map is already a card on this page. A key that opens a sheet with the same content, or
// scrolls to a block the reader can see, is a second control for one thing.
//
// blogDockJS 接线名片页的三键胶囊。
//
// 只保留这一页真能行动的键：
//
//  1. 目录 以底部面板打开全站导航——这一页没有右栏可借。
//  2. 漫游 跳到 /blog/walk——这里没有卡片可挑。
//  3. 问道 在 AI 启用时打开知识库面板。
//
// 有意去掉 推荐 与 活跃度：推荐本属于列表页右栏，热力图本来就是这一页的一张卡。
// 一个打开同样内容的面板、或滚动到读者已经看得见的一块，等于给同一件事配了第二个控件。
func blogDockJS() string {
	return `(function () {
  var dock = document.getElementById('fns-dock');
  if (!dock) { return; }
  var scrim = document.getElementById('fns-scrim');
  var SLIDE = 300;
  var open = null;
  var sheetIds = { nav: 'dock-nav' };

  function hum(ms) { if (navigator.vibrate) { try { navigator.vibrate(ms); } catch (e) {} } }
  function slide(i) { dock.style.setProperty('--slot', i); }

  // One transient line above the capsule. It exists because the capsule has a fixed shape but not
  // every key can always act, and "nothing happened" is the worst possible answer.
  // 胶囊上方的一行临时提示。它存在的原因是：胶囊形状固定，但并非每个键随时都能行动，
  // 而「什么都没发生」是最糟的答复。
  var toastTimer = 0;
  function toast(msg) {
    var el = document.getElementById('fns-dock-toast');
    if (!el) {
      el = document.createElement('div');
      el.id = 'fns-dock-toast';
      el.className = 'dock-toast';
      el.setAttribute('role', 'status');
      document.body.appendChild(el);
    }
    el.textContent = msg;
    el.classList.add('is-on');
    if (toastTimer) { clearTimeout(toastTimer); }
    toastTimer = setTimeout(function () { el.classList.remove('is-on'); }, 2800);
  }

  function close() {
    document.body.classList.remove('drawer-nav');
    if (scrim) { scrim.hidden = true; }
    for (var k in sheetIds) {
      if (Object.prototype.hasOwnProperty.call(sheetIds, k)) {
        var b = document.getElementById(sheetIds[k]);
        if (b) { b.setAttribute('aria-expanded', 'false'); }
      }
    }
    open = null;
  }

  function openSheet(name) {
    if (open === name) { close(); return; }
    var was = open;
    close();
    var go = function () {
      document.body.classList.add('drawer-' + name);
      if (scrim) { scrim.hidden = false; }
      var b = document.getElementById(sheetIds[name]);
      if (b) { b.setAttribute('aria-expanded', 'true'); }
      open = name;
      try { document.dispatchEvent(new CustomEvent('fns:panel-open', { detail: { who: 'drawer' } })); } catch (e) {}
    };
    // 换面板时先收再开，两条抽屉同时平移会让人分不清哪条是刚打开的。
    if (was) { setTimeout(go, SLIDE); } else { go(); }
  }

  function bind(id, slot, action) {
    var b = document.getElementById(id);
    if (!b) { return; }
    b.addEventListener('click', function (e) {
      slide(slot);
      hum(10);
      if (action) { action(e); }
    });
  }

  bind('dock-nav', 0, function () { openSheet('nav'); });
  bind('dock-walk', 1, function () { location.href = '/blog/walk'; });

  // 问道：只有 AI 真的启用时才有挂载点与脚本。
  // 未启用时按钮仍然存在（胶囊形状固定），但点它必须给出解释——
  // 一个按下毫无反应的键，读者只会以为站点坏了。
  //
  // 问道 only works when AI is enabled: only then are the mount point and the script rendered.
  // The key stays (the capsule has a fixed shape), but pressing it has to explain itself —
  // a key that does nothing reads as a broken site.
  var ask = document.getElementById('dock-ask');
  if (ask && ask.hasAttribute('data-ask-off')) {
    ask.addEventListener('click', function () {
      toast(ask.getAttribute('title') || '问答未启用');
    });
  } else {
    bind('dock-ask', 4, null);
  }

  // Collapsed capsule: one round key until the reader asks for the row.
  //
  // It starts collapsed on every load and collapses again as soon as a click lands outside it. Both
  // rules come from the same observation: a five-key bar parked over the bottom of a phone screen
  // is a permanent obstruction for the moments when none of the five is wanted.
  //
  // 收起的胶囊：在读者要那一排之前，它只是一个圆键。
  //
  // 每次载入都从收起态开始，并且任何落在它外面的点击都会让它重新收起。两条规则出自同一个观察：
  // 一条常驻在手机屏幕底部的五键栏，在五个键都用不上的时候就是一种长期遮挡。
  var orb = document.getElementById('fns-orb');
  var collapsible = dock.classList.contains('dock-collapsible');
  function setDockOpen(on) {
    dock.classList.toggle('is-open', on);
    if (orb) { orb.setAttribute('aria-expanded', on ? 'true' : 'false'); }
  }
  if (collapsible && orb) {
    orb.addEventListener('click', function (ev) {
      ev.stopPropagation();
      var opening = !dock.classList.contains('is-open');
      // Expanding announces itself first: the orb plays once, then the dock opens. The animation is
      // the announcement, not decoration.
      //
      // A second press while it plays is dropped rather than queued: two presses must not stack two
      // callbacks onto one animation, nor leave the dock in a state nobody asked for.
      //
      // 展开先自我通报：球播一遍，然后胶囊打开。这段动画是告知，不是装饰。
      //
      // 播放期间的第二次按压直接丢弃，而不是排队：两次按压不该在同一个动画上叠两个回调，
      // 也不该把胶囊留在一个没人要求的状态里。
      if (opening && window.fnsOrbPlay) {
        if (orb.getAttribute('data-playing') === '1') { return; }
        orb.setAttribute('data-playing', '1');
        window.fnsOrbPlay(function () {
          orb.removeAttribute('data-playing');
          setDockOpen(true);
        });
        return;
      }
      setDockOpen(!dock.classList.contains('is-open'));
    });
    document.addEventListener('click', function (ev) {
      if (!dock.classList.contains('is-open')) { return; }
      if (dock.contains(ev.target)) { return; }
      setDockOpen(false);
    });
    // Any key that opens a sheet closes the capsule behind it: leaving the row open under an open
    // sheet means the reader has to dismiss two things to get back to the page.
    //
    // 任何打开面板的键都在身后把胶囊收起：在面板打开时还让这一排留在原地，
    // 意味着读者要关两次才能回到页面。
    Array.prototype.forEach.call(dock.querySelectorAll('.dock-b'), function (b) {
      b.addEventListener('click', function () {
        if (b.hasAttribute('data-ask-off') || b.hasAttribute('data-ask-open')) { return; }
        setDockOpen(false);
      });
    });
  }

  if (scrim) { scrim.addEventListener('click', close); }
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape') { close(); } });
  document.addEventListener('fns:panel-open', function (ev) {
    var who = ev && ev.detail ? ev.detail.who : '';
    if (who !== 'drawer' && open) { close(); }
  });
  var sheet = document.querySelectorAll('.card-sheet a');
  Array.prototype.forEach.call(sheet, function (a) {
    a.addEventListener('click', function () { close(); });
  });
})();`
}

// blogTintJS paints each card's hover tint from its own cover.
//
// The reference site extracts a dominant colour at build time and injects two CSS variables. This
// site has no build step, so the extraction happens in the browser: one 1×1 canvas draw per cover,
// on the frame after the image decodes. A 1×1 draw is the cheapest possible sampler — the browser
// does the averaging for us, and we never hold the full bitmap.
//
// If the image fails, is a video, or the canvas is tainted, the card simply keeps the neutral tint
// the stylesheet already provides. That is why nothing here throws: a card without its own colour
// is a normal card, not a broken one.
//
// blogTintJS 从每张卡片自己的封面取色，画它的悬停色调。
//
// 参照站在构建期抽主色并注入两个 CSS 变量。本站没有构建步骤，所以抽取发生在浏览器里：
// 每张封面在解码后的那一帧画一次 1×1 canvas。1×1 是最便宜的取样方式——求平均由浏览器完成，
// 我们从不持有完整位图。
//
// 图片失败、是视频、或 canvas 被污染时，卡片就保留样式表已经给出的中性色。
// 这也是这里什么都不抛错的原因：一张没有自己颜色的卡片是一张正常卡片，不是一张坏卡片。
func blogTintJS() string {
	return `(function () {
  var cards = document.querySelectorAll('.post-card');
  if (!cards.length) { return; }
  var canvas = document.createElement('canvas');
  canvas.width = 1; canvas.height = 1;
  var ctx = canvas.getContext && canvas.getContext('2d', { willReadFrequently: true });
  if (!ctx) { return; }

  function tint(img) {
    if (!img.complete || !img.naturalWidth) { return; }
    if ((img.currentSrc || img.src || '').indexOf('data:') === 0) { return; }
    try {
      ctx.clearRect(0, 0, 1, 1);
      ctx.drawImage(img, 0, 0, 1, 1);
      var d = ctx.getImageData(0, 0, 1, 1).data;
      if (!d || d[3] < 8) { return; }
      var card = img.closest('.post-card');
      if (!card) { return; }
      card.style.setProperty('--tint-r', d[0]);
      card.style.setProperty('--tint-g', d[1]);
      card.style.setProperty('--tint-b', d[2]);
      card.classList.add('has-tint');
    } catch (e) { /* 污染或跨域：保留中性色 */ }
  }

  function watch(card) {
    var img = card.querySelector('img.cover');
    if (!img) { return; }
    if (img.complete) { tint(img); return; }
    img.addEventListener('load', function () { tint(img); }, { once: true });
  }

  if ('IntersectionObserver' in window) {
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (en) {
        if (!en.isIntersecting) { return; }
        io.unobserve(en.target);
        watch(en.target);
      });
    }, { rootMargin: '200px' });
    Array.prototype.forEach.call(cards, function (c) { io.observe(c); });
  } else {
    Array.prototype.forEach.call(cards, watch);
  }
})();`
}

// blogMailPillJS drives the 邮箱 pill on the card page.
//
// Why the pill is not a mailto: link any more. A reader wrote in: clicking 邮箱 launched Outlook,
// an app they had not used in months and whose linked Gmail still needed re-verification. They
// tried to finish the verification, the 继续 button in the browser would not respond, the browser
// froze, and they closed the site and Outlook and left. The address itself was what they wanted —
// they would have written the mail in Gmail in a tab they already had open.
//
// So the pill now does two things, neither of which launches anything: hover reveals the address,
// click copies it. A reader who does want a mail client has the address on screen and can hand it
// to whatever they prefer; a reader who lives in a browser tab is done in one click.
//
// The hover half is desktop-only by nature (a phone has no hover), which is why the click half is
// the primary action and not a secondary one: on touch, the tap is the whole interaction.
//
// blogMailPillJS 驱动名片页的「邮箱」药丸。
//
// 为什么这枚药丸不再是一个 mailto: 链接。有读者写信反馈：点「邮箱」拉起的是 Outlook——
// 一个几个月没用、里面绑定的 Gmail 仍需重新验证的应用。他试着完成验证，浏览器里的「继续」
// 按钮没有反应，浏览器卡死，于是他关掉网站和 Outlook 走了。他真正想要的是地址本身——
// 他会用已经开着的 Gmail 标签页写那封信。
//
// 因此这枚药丸现在做两件事，都不启动任何程序：悬停显示地址，点击复制。想用邮件客户端的读者
// 地址就在屏幕上，交给任何客户端都可以；在浏览器里写邮件的读者一次点击就结束了。
//
// 悬停这一半天然只属于桌面（手机没有悬停），所以点击才是主操作而不是补充：在触摸设备上，
// 那一次点按就是全部交互。
func blogMailPillJS() string {
	return `(function () {
  var pills = document.querySelectorAll('[data-copy-pill]');
  if (!pills.length) { return; }

  var toastEl = null, toastTimer = 0;
  function toast(msg) {
    if (!toastEl) {
      toastEl = document.createElement('div');
      toastEl.className = 'pill-toast';
      toastEl.setAttribute('role', 'status');
      document.body.appendChild(toastEl);
    }
    toastEl.textContent = msg;
    toastEl.classList.add('is-on');
    if (toastTimer) { clearTimeout(toastTimer); }
    toastTimer = setTimeout(function () { toastEl.classList.remove('is-on'); }, 1900);
  }

  // copyText never throws: the async clipboard API is unavailable outside a secure context and
  // refuses without a user gesture, so the textarea route is not a legacy fallback — it is the
  // path that works on the plain-http case other people's servers still run.
  //
  // copyText 从不抛错：异步剪贴板 API 在非安全上下文里不存在、在没有用户手势时会拒绝，
  // 所以 textarea 那条路不是历史包袱——在别人的服务器仍然跑着的纯 http 上，它能用。
  function copyText(text, done) {
    function fallback() {
      var ta = document.createElement('textarea');
      ta.value = text;
      ta.setAttribute('readonly', '');
      ta.style.cssText = 'position:fixed;top:0;left:-9999px;opacity:0';
      document.body.appendChild(ta);
      ta.select();
      ta.setSelectionRange(0, text.length);
      var ok = false;
      try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
      ta.remove();
      done(ok);
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(function () { done(true); }, fallback);
      return;
    }
    fallback();
  }

  Array.prototype.forEach.call(pills, function (wrap) {
    var btn = wrap.querySelector('[data-copy]');
    if (!btn) { return; }
    var addr = btn.getAttribute('data-copy') || '';
    if (!addr) { return; }
    btn.addEventListener('click', function () {
      copyText(addr, function (ok) {
        toast(ok ? ('邮箱已复制　' + addr) : ('复制失败，请手动选择：' + addr));
        if (ok) { btn.classList.add('is-copied'); setTimeout(function () { btn.classList.remove('is-copied'); }, 1200); }
      });
    });
  });
})();`
}
