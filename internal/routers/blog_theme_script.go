package routers

// blogThemeScript is the shared head script: theme application (before first paint), the blog
// dropdown and the mobile menu. It lives in one constant because six separate <head> blocks across
// four template files need it, and a copy per file would drift the moment one of them is edited.
//
// blogThemeScript 是共用的 head 脚本：主题应用（首屏之前）、博客下拉与移动菜单。
// 它放在一个常量里，因为四个模板文件里有六个独立 <head> 需要它，
// 而每处一份副本会在任何一处被修改时开始漂移。
const blogThemeScript = `
/* Theme, applied before the stylesheet is parsed.
   Reading localStorage and pinning html.dark/html.light here — in the head, synchronously — is
   the only way to avoid the white flash a deferred script causes: by the time a deferred script
   runs, the browser has already painted one frame of the wrong background.

   主题，在样式表解析之前应用。
   在 head 里同步读取 localStorage 并钉住 html.dark/html.light，是避免白闪的唯一办法：
   延迟脚本运行的时候，浏览器已经画了一帧错误的底色。 */
(function () {
  var KEY = 'fns:theme';
  function mode() { try { return localStorage.getItem(KEY) || 'system'; } catch (e) { return 'system'; } }
  function darkNow(m) {
    return m === 'dark' || (m === 'system' && window.matchMedia && window.matchMedia('(prefers-color-scheme:dark)').matches);
  }
  function apply(m) {
    var h = document.documentElement;
    h.classList.remove('dark', 'light');
    if (m === 'dark') { h.classList.add('dark'); }
    else if (m === 'light') { h.classList.add('light'); }
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) { meta.setAttribute('content', darkNow(m) ? '#1a1a1a' : '#eae9e3'); }
    var b = document.getElementById('fns-theme');
    if (b) {
      b.setAttribute('data-mode', m);
      b.setAttribute('title', m === 'system' ? '主题：跟随系统' : (m === 'dark' ? '主题：暗色' : '主题：亮色'));
    }
  }
  window.fnsApplyTheme = apply;
  apply(mode());
  document.addEventListener('DOMContentLoaded', function () {
    var b = document.getElementById('fns-theme');
    if (b) {
      b.addEventListener('click', function () {
        var order = ['system', 'dark', 'light'];
        var next = order[(order.indexOf(mode()) + 1) % 3];
        try { localStorage.setItem(KEY, next); } catch (e) {}
        apply(next);
      });
    }
    apply(mode());

    // Blog dropdown: click to open, click outside or Esc to close. It is a button rather than a
    // <details> element because details cannot animate its close — the content is display:none the
    // instant it is toggled, so the panel would vanish instead of retracting.
    //
    // 博客下拉：点击展开，点外部或 Esc 收起。用按钮而不是 <details>，
    // 因为 details 的收起无法做动画——切换那一刻内容就是 display:none，
    // 面板会瞬间消失而不是收回。
    var dd = document.getElementById('fns-dd');
    var ddBtn = document.getElementById('fns-dd-btn');
    if (dd && ddBtn) {
      ddBtn.addEventListener('click', function (ev) {
        ev.stopPropagation();
        var on = dd.classList.toggle('is-open');
        ddBtn.setAttribute('aria-expanded', on ? 'true' : 'false');
      });
      document.addEventListener('click', function (ev) {
        if (dd.classList.contains('is-open') && !dd.contains(ev.target)) {
          dd.classList.remove('is-open');
          ddBtn.setAttribute('aria-expanded', 'false');
        }
      });
      document.addEventListener('keydown', function (ev) {
        if (ev.key === 'Escape') {
          dd.classList.remove('is-open');
          ddBtn.setAttribute('aria-expanded', 'false');
        }
      });
    }

    // Mobile menu. The panel is a sibling of the bar and opened by a body class, so the same
    // click also has to close it — a menu whose only exit is reloading the page is a trap.
    //
    // 移动菜单。面板是顶栏的兄弟节点、由 body 类控制开合，因此同一次点击也要能关掉它——
    // 一个只能靠刷新退出的菜单是陷阱。
    var burger = document.getElementById('fns-burger');    if (burger) {
      burger.addEventListener('click', function () {
        var on = document.body.classList.toggle('mnav-open');
        burger.setAttribute('aria-expanded', on ? 'true' : 'false');
      });
      var panel = document.getElementById('fns-mnav');
      if (panel) {
        panel.addEventListener('click', function (ev) {
          if (ev.target && ev.target.closest && ev.target.closest('a')) {
            document.body.classList.remove('mnav-open');
            burger.setAttribute('aria-expanded', 'false');
          }
        });
      }
      document.addEventListener('keydown', function (ev) {
        if (ev.key === 'Escape') {
          document.body.classList.remove('mnav-open');
          burger.setAttribute('aria-expanded', 'false');
        }
      });
    }
  });
  if (window.matchMedia && window.matchMedia('(prefers-color-scheme:dark)').addEventListener) {
    window.matchMedia('(prefers-color-scheme:dark)').addEventListener('change', function () {
      if (mode() === 'system') { apply('system'); }
    });
  }
})();
`
