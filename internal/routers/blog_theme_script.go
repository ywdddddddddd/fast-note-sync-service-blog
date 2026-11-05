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

    // ── 滚动三件套：底部进度条、顶栏浮起、站名回顶 ─────────────────────
    //
    // 一个 scroll 监听同时喂这三件事。它们都以同一个 scrollY 为准，
    // 拆成三个监听会在同一帧里读三次布局，而且阈值一旦不一致就会互相打脸
    // （条子走了一半、顶栏却还没浮起来）。
    //
    // One scroll listener feeds all three. They all read the same scrollY; three listeners would
    // read layout three times per frame, and the moment the thresholds disagreed the page would
    // contradict itself — a bar at fifty percent over a bar that has not lifted yet.
    (function () {
      var reduce = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
      var docEl = document.documentElement;
      var topbar = document.querySelector('.topbar');

      // The bar is created here rather than in six template files: every page that can scroll needs
      // it, and six copies of the same tag is six places to forget.
      //
      // 这条进度条在这里创建，而不是写进六个模板：每个能滚动的页面都需要它，
      // 而同一段标记抄六遍就是六个会被忘掉的地方。
      var bar = document.createElement('div');
      bar.className = 'read-bar';
      bar.setAttribute('aria-hidden', 'true');
      bar.appendChild(document.createElement('i'));
      document.body.appendChild(bar);

      var span = 0;
      function measure() {
        span = Math.max(0, docEl.scrollHeight - window.innerHeight);
        update();
      }

      function update() {
        var y = window.scrollY || docEl.scrollTop || 0;

        // Progress is meaningless on a page that does not scroll, so it stays hidden there.
        // 页面本身不能滚动时进度没有意义，那里就保持隐藏。
        var p = span > 0 ? Math.min(1, Math.max(0, y / span)) : 0;
        bar.style.setProperty('--p', p.toFixed(4));
        bar.classList.toggle('is-on', p > 0.002);

        if (topbar) {
          var e = Math.min(1, Math.max(0, (y - 8) / 56));
          topbar.style.setProperty('--tb-e', e.toFixed(3));
          topbar.classList.toggle('is-scrolled', y > 8);
        }
      }

      // The scroll position of a document that is still loading is not the scroll position of the
      // document the reader gets: images, fonts and the graph all change the height after load.
      // Re-measure whenever the document resizes instead of trusting the first reading.
      //
      // 还在加载中的文档的滚动位置，并不是读者最后拿到的那个文档的滚动位置：
      // 图片、字体、图谱都会在 load 之后改变高度。所以不该相信第一次读数，
      // 而是只要文档尺寸变了就重量一次。
      window.addEventListener('scroll', update, { passive: true });
      window.addEventListener('resize', measure, { passive: true });
      window.addEventListener('load', measure);
      if (window.ResizeObserver) {
        try { new ResizeObserver(measure).observe(docEl); } catch (e) {}
      }
      measure();

      // 站名：不在顶部时回顶，已经在顶部时照常跳首页。
      //
      // 这让一个链接的行为随滚动位置变化，所以只在 y 超过 8px 时才接管；
      // 8px 以内一律放行，保证「想回首页」这个最常做的事永远不会被截住。
      //
      // The wordmark scrolls to the top when the page is scrolled, and behaves as a plain link to
      // the home page when it is not. That does make a link's behaviour depend on scroll position,
      // which is why the override starts only past 8px: inside that band the link is always a link,
      // and "go home" — the most common thing anyone wants from it — can never be intercepted.
      var brandName = document.querySelector('.brand-name');
      if (brandName) {
        brandName.addEventListener('click', function (ev) {
          var y = window.scrollY || docEl.scrollTop || 0;
          if (y <= 8) { return; }
          ev.preventDefault();
          window.scrollTo({ top: 0, behavior: reduce ? 'auto' : 'smooth' });
        });
      }
    })();

    // ── 模糊进入 ───────────────────────────────────────────────────────
    //
    // 容器不是「淡入」而是「从模糊里聚焦出来」：opacity 与 filter 一起走完，
    // 读到的是东西正在成形，而不是一块半透明的方块在变亮。
    //
    // The reveal is a focus pull, not a fade: opacity and filter finish together, so what the reader
    // sees is something coming into focus rather than a translucent box getting brighter.
    //
    // Two rules keep this from becoming a bug:
    //   - The class is applied by script only. Without JavaScript nothing is ever hidden, so a page
    //     that fails to run this still shows every word.
    //   - filter is removed entirely (not left at blur(0)) once the reveal is done: a non-none filter
    //     makes the element a containing block for fixed descendants and breaks sticky children
    //     inside it.
    //
    // 两条规则让它不至于变成 bug：
    //   - 这个类只由脚本添加。没有 JavaScript 时什么都从未被隐藏，
    //     因此一段没能执行到这里的页面仍然把每个字都显示出来。
    //   - 浮现结束后把 filter 整个去掉（而不是留在 blur(0)）：
    //     非 none 的 filter 会成为其内部 fixed 后代的包含块，并破坏内部的 sticky 子元素。
    (function () {
      var reduce = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
      if (reduce) { return; }

      // The list pages put their cards inside .cards, the article page is a single .card, the shelf
      // has its own column class, and the home page's栏目 list lives inside a drawer as plain rows
      // under ul.rec. Selecting element names instead of roles would have missed all four — this
      // list is the actual markup, checked against the rendered DOM rather than guessed from CSS.
      //
      // 列表页的卡片在 .cards 里，文章页是一整块 .card，资料页有它自己的列类名，
      // 而首页的「栏目」是抽屉里 ul.rec 下面的普通行。
      // 凭角色去猜元素名会四处全部落空——这份清单来自实际渲染出来的 DOM，不是从 CSS 推的。
      var SEL = '.cards > *, .post-card, .card, .post-body, .page-body, .aside, .rail, .footer,' +
                ' .shelf-col, .shelf-card, .sheet-rec > ul > li';
      var nodes = Array.prototype.slice.call(document.querySelectorAll(SEL));
      if (!nodes.length) { return; }

      var vh = window.innerHeight || 800;
      var io = null;
      if (window.IntersectionObserver) {
        io = new IntersectionObserver(function (entries) {
          for (var i = 0; i < entries.length; i++) {
            if (!entries[i].isIntersecting) { continue; }
            entries[i].target.classList.add('is-in');
            io.unobserve(entries[i].target);
          }
        }, { rootMargin: '0px 0px -8% 0px', threshold: 0.01 });
      }

      var above = 0;
      for (var i = 0; i < nodes.length; i++) {
        var el = nodes[i];
        if (el.classList.contains('reveal')) { continue; }
        el.classList.add('reveal');
        var r = el.getBoundingClientRect();
        // A zero-sized box means the container is display:none right now — the card lives inside a
        // drawer that has not been opened yet. Its rect reports 0,0, which the first-screen test
        // reads as "above the fold" and reveals instantly, so the card would already be settled by
        // the time the drawer opened and the reader would never see it arrive. Observing instead
        // lets it animate at the moment the drawer actually shows it.
        //
        // 盒子尺寸为零说明容器此刻是 display:none —— 卡片躺在还没打开的抽屉里。
        // 它的 rect 会报 0,0，而「是否在首屏」的判断会把 0 当成「在顶上」，于是立刻标成已浮现，
        // 等抽屉真的打开时它早已落定，读者永远看不到它进场。改成交给观察器，
        // 让它在抽屉把这张卡片显示出来的那一刻播放。
        if (r.width === 0 && r.height === 0) {
          if (io) { io.observe(el); } else { el.classList.add('is-in'); }
          continue;
        }
        // First screen: stagger by order so the page assembles rather than appearing all at once.
        // 首屏：按顺序错开，页面是「组装起来」的，不是一瞬间全出现。
        if (r.top < vh * 0.92) {
          el.style.transitionDelay = Math.min(above * 60, 240) + 'ms';
          above++;
          (function (node) {
            window.requestAnimationFrame(function () {
              window.requestAnimationFrame(function () { node.classList.add('is-in'); });
            });
          })(el);
        } else if (io) {
          io.observe(el);
        } else {
          el.classList.add('is-in');
        }
      }

      // The delay is a one-shot; leaving it on would also delay hover transitions on that element.
      // 这个延迟只用于这一次进场；留着它会给该元素之后的悬停过渡也加上延迟。
      document.addEventListener('transitionend', function (ev) {
        if (ev.propertyName === 'translate' && ev.target && ev.target.classList) {
          ev.target.style.transitionDelay = '';
        }
      });
    })();

    // ── 资料页书架接滚轮 ───────────────────────────────────────────────
    //
    // 书架是一条横向轨道，而鼠标只有竖向滚轮。不加这段，读者看到一排书，
    // 却只能去拖那条又细又难抓的横滚动条。
    //
    // 到两端就把滚动交还给页面：如果在这里继续吞掉 wheel，读者会被书架困住——
    // 想看下一页，滚动条却只在书架里打转。
    //
    // The shelf is a horizontal track and a mouse has only a vertical wheel. Without this the reader
    // sees a row of books and has to hunt for a thin scrollbar.
    //
    // At either end the wheel is handed back to the page. Swallowing it there would trap the reader
    // in the shelf: they want the rest of the page, but the wheel only circles inside the rail.
    Array.prototype.forEach.call(document.querySelectorAll('.shelf-rail'), function (rail) {
      rail.addEventListener('wheel', function (ev) {
        if (ev.ctrlKey) { return; }
        // A trackpad's horizontal gesture already belongs to the rail; only vertical intent is
        // translated.
        // 触控板的横向手势本来就归这条轨道；这里只翻译竖向意图。
        if (Math.abs(ev.deltaY) <= Math.abs(ev.deltaX)) { return; }
        var max = rail.scrollWidth - rail.clientWidth;
        if (max <= 1) { return; }
        var before = rail.scrollLeft;
        var next = before + ev.deltaY;
        if (next < 0) { next = 0; }
        if (next > max) { next = max; }
        if (next === before) { return; }
        ev.preventDefault();
        rail.scrollLeft = next;
      }, { passive: false });
    });

    // ── 卡片 3D 倾斜 ───────────────────────────────────────────────────
    //
    // 8° 上限、封面 6px 反向位移、光标处一层柔光。只在能悬停的精确指针上跑：
    // 触屏没有 hover，倾斜会卡在最后一次触碰的位置不动。
    //
    // 选卡片而不是选角色：列表页是 .post-card，资料页是 .shelf-card，
    // 而文章页那个 .card 装的是整篇正文——把正文整块倾斜会让人反胃。
    // 所以 .card 只在它内部真的有封面时才参与。
    //
    // 8 degrees, a 6px counter-move on the cover, and a soft light that follows the pointer. Only on
    // a hovering fine pointer: a touch screen has no hover, so the tilt would freeze wherever the
    // last touch left it.
    //
    // Cards are selected by class rather than by role: the list uses .post-card, the shelf uses
    // .shelf-card, and the article's own .card holds the entire essay — tilting that would make
    // people queasy. So .card joins only when it actually contains a cover.
    (function () {
      var fine = window.matchMedia && window.matchMedia('(hover:hover) and (pointer:fine)');
      if (!fine || !fine.matches) { return; }
      if (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) { return; }

      var MAX_DEG = 8;
      var PARALLAX = 6;

      var list = Array.prototype.slice.call(
        document.querySelectorAll('.post-card, .sectioncard, .shelf-card'));
      Array.prototype.forEach.call(document.querySelectorAll('.card'), function (el) {
        if (el.querySelector('.cover')) { list.push(el); }
      });

      function clamp(v) { return v < -1 ? -1 : (v > 1 ? 1 : v); }

      for (var i = 0; i < list.length; i++) {
        (function (card) {
          if (card.classList.contains('tilt')) { return; }
          card.classList.add('tilt');

          var sheen = document.createElement('span');
          sheen.className = 'tilt-sheen';
          sheen.setAttribute('aria-hidden', 'true');
          card.appendChild(sheen);

          card.addEventListener('pointermove', function (ev) {
            if (ev.pointerType !== 'mouse') { return; }
            var r = card.getBoundingClientRect();
            if (!r.width || !r.height) { return; }
            var nx = clamp((ev.clientX - r.left) / r.width - 0.5) * 2;   // -1..1
            var ny = clamp((ev.clientY - r.top) / r.height - 0.5) * 2;
            card.style.setProperty('--ry', (nx * MAX_DEG).toFixed(2) + 'deg');
            card.style.setProperty('--rx', (-ny * MAX_DEG).toFixed(2) + 'deg');
            card.style.setProperty('--px', (-nx * PARALLAX).toFixed(2) + 'px');
            card.style.setProperty('--py', (-ny * PARALLAX).toFixed(2) + 'px');
            card.style.setProperty('--mx', ((nx * 0.5 + 0.5) * 100).toFixed(1) + '%');
            card.style.setProperty('--my', ((ny * 0.5 + 0.5) * 100).toFixed(1) + '%');
            card.classList.add('is-tilting');
          });

          // Leaving puts everything back to zero rather than to the last value: a card stuck at
          // three degrees is a card that looks broken.
          // 离开时把每个值归零，而不是留在最后一个值上：一张卡在三度角停住，看起来就是坏了。
          card.addEventListener('pointerleave', function () {
            card.classList.remove('is-tilting');
            card.style.setProperty('--ry', '0deg');
            card.style.setProperty('--rx', '0deg');
            card.style.setProperty('--px', '0px');
            card.style.setProperty('--py', '0px');
          });
        })(list[i]);
      }
    })();
  });
  if (window.matchMedia && window.matchMedia('(prefers-color-scheme:dark)').addEventListener) {
    window.matchMedia('(prefers-color-scheme:dark)').addEventListener('change', function () {
      if (mode() === 'system') { apply('system'); }
    });
  }
})();
`
