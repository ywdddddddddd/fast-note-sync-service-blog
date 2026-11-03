package routers

// The card page's two scripts live here rather than inside the template so the markup stays
// readable: the dock wiring alone is a hundred lines of state machine, and inlining it pushed the
// page template past the point where the HTML could be scanned.
//
// 名片页的两个脚本放在这里而不是模板里，是为了让标记保持可读：
// 单是胶囊接线就有一百行状态机，内联它会把页面模板推到 HTML 无法扫读的长度。

// blogDockJS wires the five-key capsule on the card page.
//
// Three behaviours differ from the previous page, and each one follows from the card page having
// no article list and no right rail:
//
//  1. 漫游 navigates to /blog/walk instead of picking a card off the page — there are no cards
//     here, and the server route already exists for the article page's own capsule.
//  2. 活跃 scrolls to the heat map instead of opening a drawer. The heat map is a block on this
//     page, not a rail panel; a drawer that shows the same figure twice would be worse than a
//     scroll that moves you to it.
//  3. 目录 and 推荐 still open as bottom sheets, but their contents live in hidden containers
//     rather than in a rail, because this page has no rail to borrow.
//
// blogDockJS 接线名片页的五键胶囊。
//
// 三处行为与旧首页不同，每一处都来自「名片页没有文章列表、没有右栏」这件事：
//
//  1. 漫游 跳到 /blog/walk，而不是从页面里挑一张卡——这里没有卡片，
//     而服务端那条路由本来就是为文章页的胶囊准备的。
//  2. 活跃 滚动到热力图，而不是开抽屉。热力图是这一页上的一块，不是右栏面板；
//     一个把同一个数字再显示一遍的抽屉，比一次把你带到它面前的滚动更糟。
//  3. 目录 与 推荐 仍以底部面板打开，但内容放在隐藏容器里而不是右栏——这一页没有右栏可借。
func blogDockJS() string {
	return `(function () {
  var dock = document.getElementById('fns-dock');
  if (!dock) { return; }
  var scrim = document.getElementById('fns-scrim');
  var SLIDE = 300;
  var open = null;
  var sheetIds = { nav: 'dock-nav', rec: 'dock-hot' };

  function hum(ms) { if (navigator.vibrate) { try { navigator.vibrate(ms); } catch (e) {} } }
  function slide(i) { dock.style.setProperty('--slot', i); }

  function close() {
    document.body.classList.remove('drawer-nav', 'drawer-rec');
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
  bind('dock-hot', 1, function () { openSheet('rec'); });
  bind('dock-walk', 2, function () { location.href = '/blog/walk'; });
  bind('dock-live', 3, function () {
    var el = document.getElementById('activity');
    if (!el) { return; }
    var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    el.scrollIntoView({ behavior: reduce ? 'auto' : 'smooth', block: 'center' });
  });
  bind('dock-ask', 4, null);

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
