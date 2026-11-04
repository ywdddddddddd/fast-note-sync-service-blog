package routers

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

// The three theme icons are real Lottie animations, not hand-drawn approximations of one.
//
// They come from useAnimations (MIT, attribution optional) as 32x32 artboards at 30fps, 40 frames
// — 1.33s each. The owner's instruction was to fetch the actual files rather than redraw something
// lighter in CSS, so these are the upstream bytes, unmodified.
//
// All three are drawn in pure black. That is what makes the theme-aware tinting honest: the
// stylesheet inverts them in dark mode instead of the Go side rewriting colour keyframes, so there
// is exactly one copy of each animation and the colour lives in CSS where the theme already lives.
//
// 三个主题图标是真的 Lottie 动画，不是用 CSS 手绘一个近似物。
//
// 来自 useAnimations（MIT，署名非必需），32x32 画板、30fps、40 帧 —— 每个 1.33 秒。
// 站主要求取真实文件而不是用 CSS 重画一个更轻的东西，所以这里是上游字节，未作改动。
//
// 三个都是纯黑描边。这正是配色得以诚实的原因：暗色模式下由样式表反相，
// 而不是让 Go 侧去改写颜色关键帧 —— 于是每个动画只有一份，颜色留在主题本来就在的 CSS 里。
var (
	//go:embed assets/lottie_light.min.js
	lottiePlayerJS []byte

	//go:embed assets/lottie-moon.json
	lottieMoonJSON []byte

	//go:embed assets/lottie-sun.json
	lottieSunJSON []byte

	//go:embed assets/lottie-smartphone.json
	lottieSmartphoneJSON []byte
)

// lottieIcons maps the three theme states onto their artwork.
// lottieIcons 把三个主题状态映射到各自的图。
var lottieIcons = map[string][]byte{
	"moon.json":       lottieMoonJSON,
	"sun.json":        lottieSunJSON,
	"smartphone.json": lottieSmartphoneJSON,
}

// serveLottiePlayer serves the lottie-web runtime.
//
// It is served rather than inlined into the page because it is 164 KB: as a separate file it is
// fetched once and then cached across every page and every visit, while inlining would put it in
// the HTML of every page that carries a theme button.
//
// serveLottiePlayer 提供 lottie-web 运行时。
//
// 用独立文件而不是内联进页面，因为它有 164 KB：独立文件只取一次，之后跨页面、跨访问都命中缓存；
// 内联则会出现在每一个带主题按钮的页面的 HTML 里。
func serveLottiePlayer(c *gin.Context) {
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=604800, immutable")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(lottiePlayerJS)
}

// serveLottieIcon serves one animation's JSON.
//
// Name is validated against the map rather than against a path shape: the request never reaches the
// filesystem, so an unknown name is a 404 from a closed set instead of a path that has to be proven
// safe.
//
// serveLottieIcon 提供某个动画的 JSON。
//
// Name 是拿去查表的，而不是拿去校验路径形状：请求从不落到文件系统，
// 因此未知名字是从一个封闭集合里查不到的 404，而不是一条需要被证明安全的路径。
func serveLottieIcon(c *gin.Context) {
	body, ok := lottieIcons[c.Param("name")]
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=604800, immutable")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(body)
}

// blogThemeLottieJS wires the three animations to the theme button.
//
// It is injected by blogThemeScript only after a theme button is found, so a page without one — the
// article page hides the whole top bar — never downloads the 164 KB player. That is the reason this
// is a loader rather than a <script> tag in the head: six heads across four template files would
// each have to remember to include it, and the one that forgot would silently keep a static icon.
//
// The static SVG stays in the markup as the baseline. The animation only replaces it once the
// player has actually drawn a frame, so a page that never loads this file still shows the right
// symbol, and a reader on a slow link does not see the button go blank in the meantime.
//
// blogThemeLottieJS 把三个动画接到主题按钮上。
//
// 它由 blogThemeScript 在找到主题按钮之后才注入，因此没有该按钮的页面——文章页把整条顶栏都藏了——
// 永远不会下载那 164 KB 播放器。这也正是它做成加载器而不是 head 里一条 <script> 的原因：
// 四个模板文件里的六个 head 每个都得记得引入它，而漏掉的那一个会安静地停在静态图标上。
//
// 静态 SVG 留在标记里作为基线。动画只在播放器真的画出一帧之后才取代它，
// 于是从不加载这个文件的页面仍然显示正确的符号，而慢链路上的读者也不会看到按钮中途变空。
const blogThemeLottieJS = `
(function () {
  function init() {
    var btn = document.getElementById('fns-theme');
    if (!btn || btn.getAttribute('data-lottie') === 'off') { return; }
    var els = btn.querySelectorAll('.tl');
    if (!els.length) { return; }
    var still = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    var s = document.createElement('script');
    s.src = '/blog/lottie.js';
    s.defer = true;
    s.onload = boot;
    s.onerror = function () { btn.setAttribute('data-lottie', 'off'); };
    document.head.appendChild(s);

    function boot() {
      if (!window.lottie) { return; }
      var anims = {};
      var waiting = els.length;

      function done() {
        waiting--;
        if (waiting > 0) { return; }
        btn.classList.add('has-lottie');
        // Freeze on the first frame of the state the page loaded in: the animation answers a
        // press, it is not an idle loop.
        //
        // 停在页面载入时那个状态的第一帧：动画是回应一次按压的，不是待机循环。
        var cur = anims[btn.getAttribute('data-mode')];
        if (cur) { cur.goToAndStop(0, true); }
      }

      for (var i = 0; i < els.length; i++) {
        (function (el) {
          var path = el.getAttribute('data-src');
          var state = el.getAttribute('data-state');
          if (!path) { done(); return; }
          var a = lottie.loadAnimation({
            container: el,
            renderer: 'svg',
            loop: false,
            autoplay: false,
            path: path,
            rendererSettings: { progressiveLoad: true, hideOnTransparent: true }
          });
          a.addEventListener('DOMLoaded', done);
          a.addEventListener('data_failed', function () {
            btn.setAttribute('data-lottie', 'off');
            btn.classList.remove('has-lottie');
            done();
          });
          anims[state] = a;
        })(els[i]);
      }

      // The theme script's own click handler runs first and has already written the new data-mode
      // by the time this one fires, so the state read here is the state the reader just switched
      // to - the icon that plays is the one the button now shows.
      //
      // 主题脚本自己的点击处理先运行，在这个处理被触发时已经把新的 data-mode 写好了，
      // 因此这里读到的正是读者刚切到的那个状态 - 播放的就是按钮此刻显示的那一枚。
      btn.addEventListener('click', function () {
        if (still) { return; }
        var a = anims[btn.getAttribute('data-mode')];
        if (a) { a.goToAndPlay(0, true); }
      });
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
`
