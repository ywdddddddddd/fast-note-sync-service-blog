package routers

import (
	"encoding/json"
	"html/template"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/service"
)

// Link preview assembly for the article page.
//
// The page already links to other articles; hovering one should say what is behind it. The data
// is collected here, at render time, and written into the document as a JSON island — see
// service.GetLinkPreviews for why this is not fetched on hover.
//
// 文章页的链接预览组装。
//
// 页面本身已经链接到其它文章；悬停其中一条应当说明它背后是什么。数据在这里、在渲染时收集，
// 并以 JSON 岛写入文档——为什么不在悬停时获取，见 service.GetLinkPreviews。

// blogLinkHrefRegex pulls the slug out of an internal article link in the rendered body.
//
// It matches the href this service itself emits (`/blog/<uid>.<sid>-<hmac>`), so there is no
// guessing about link shapes written by hand.
//
// blogLinkHrefRegex 从已渲染正文里的内部文章链接中取出 slug。
//
// 它匹配的是本服务自己输出的 href 形态（`/blog/<uid>.<sid>-<hmac>`），因此不需要猜测手写的链接形态。
var blogLinkHrefRegex = regexp.MustCompile(`href="/blog/(\d+\.\d+-[0-9a-fA-F]{4,})"`)

// blogPreviewMaxLinks bounds how many link targets a single page will resolve.
//
// The island must stay small enough to be worth inlining. A page that links to more than this
// many notes gets previews for the first ones it links to and plain links for the rest — the
// degradation is invisible and costs nothing.
//
// blogPreviewMaxLinks 限制单个页面解析多少个链接目标。
//
// 这块 JSON 必须小到值得内联。链接笔记数超过此值的页面，靠前的目标有预览、其余只是普通链接——
// 这种降级不可见，也不产生代价。
const blogPreviewMaxLinks = 40

// linkPreviewJSON returns the JSON island for the article body's internal links, or an empty
// value when there is nothing to preview.
//
// Every failure path returns an empty island rather than an error: a hover card is a convenience,
// and it must never be able to break the article it decorates.
//
// linkPreviewJSON 返回文章正文内部链接的 JSON 岛，没有可预览目标时返回空值。
//
// 所有失败路径都返回空岛而不是错误：悬停卡片是一种便利，它绝不能破坏它所装饰的文章。
func linkPreviewJSON(shareSvc service.ShareService, c *gin.Context, cfg *blogRuntimeConfig, body string) template.JS {
	if shareSvc == nil || body == "" {
		return template.JS("")
	}

	seen := make(map[string]struct{}, 16)
	slugs := make([]string, 0, 16)
	for _, m := range blogLinkHrefRegex.FindAllStringSubmatch(body, -1) {
		if len(m) < 2 {
			continue
		}
		slug := m[1]
		if _, dup := seen[slug]; dup {
			continue
		}
		seen[slug] = struct{}{}
		slugs = append(slugs, slug)
		if len(slugs) >= blogPreviewMaxLinks {
			break
		}
	}
	if len(slugs) == 0 {
		return template.JS("")
	}

	previews, err := shareSvc.GetLinkPreviews(c.Request.Context(), blogUID(cfg.UID), slugs)
	if err != nil || len(previews) == 0 {
		return template.JS("")
	}

	// json.Marshal escapes <, > and & by default, so the payload cannot close the surrounding
	// <script> element. That is the reason no manual escaping appears here.
	//
	// json.Marshal 默认转义 <、> 与 &，因此该载荷无法闭合外层的 <script> 元素。
	// 这就是这里没有手写转义的原因。
	payload, err := json.Marshal(previews)
	if err != nil {
		return template.JS("")
	}
	return template.JS(payload)
}

// blogLinkPreviewJS renders the hover card.
//
// Desktop only, and it follows the same rules as the graph layer: no JavaScript means the links
// still work (they are ordinary anchors), and reduced motion means the card appears without a
// transition. No backticks appear below because this is a Go raw string.
//
// blogLinkPreviewJS 渲染悬停卡片。
//
// 仅桌面端，且遵循与图谱浮层相同的规则：没有 JavaScript 时链接照常工作（它们本来就是普通锚点），
// 减少动效时卡片直接出现、没有过渡。下面没有反引号，因为这是 Go 原始字符串。
const blogLinkPreviewJS string = `(function(){
  'use strict';

  var island = document.getElementById('link-previews');
  if (!island) { return; }

  var data;
  try { data = JSON.parse(island.textContent || '{}'); } catch (e) { return; }
  if (!data || typeof data !== 'object') { return; }

  var DESKTOP = window.matchMedia('(min-width: 861px)');
  var STILL = window.matchMedia('(prefers-reduced-motion: reduce)');

  var card = null, hideTimer = null, showTimer = null, current = null;

  function build() {
    card = document.createElement('div');
    card.className = 'lpreview';
    card.setAttribute('role', 'tooltip');
    card.hidden = true;
    card.innerHTML =
      '<div class="lpreview-title"></div>' +
      '<div class="lpreview-path"></div>';
    document.body.appendChild(card);
  }

  function position(anchor) {
    var r = anchor.getBoundingClientRect();
    var cw = card.offsetWidth || 320;
    var ch = card.offsetHeight || 96;
    var margin = 10;

    // Below the link by default; above it when there is no room, which is what keeps a link in
    // the last paragraph from pushing the card off the bottom of the viewport.
    //
    // 默认在链接下方；空间不足时放到上方——这让最后一段里的链接不会把卡片推出视口底部。
    var top = r.bottom + margin;
    if (top + ch > window.innerHeight - margin) {
      top = r.top - ch - margin;
    }
    if (top < margin) { top = margin; }

    var left = r.left;
    if (left + cw > window.innerWidth - margin) {
      left = window.innerWidth - cw - margin;
    }
    if (left < margin) { left = margin; }

    card.style.top = Math.round(top + window.scrollY) + 'px';
    card.style.left = Math.round(left + window.scrollX) + 'px';
  }

  function show(anchor, item) {
    if (!DESKTOP.matches) { return; }
    if (!card) { build(); }
    if (showTimer) { clearTimeout(showTimer); }
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }

    card.querySelector('.lpreview-title').textContent = item.title || '';
    var meta = item.path || '';
    if (item.vault) { meta = item.vault + (meta ? ' · ' + meta : ''); }
    card.querySelector('.lpreview-path').textContent = meta;

    current = anchor;
    // A short delay keeps a card from flashing as the pointer crosses a paragraph of links.
    // 短暂延迟避免指针扫过一整段链接时卡片不断闪出。
    showTimer = setTimeout(function() {
      showTimer = null;
      if (current !== anchor) { return; }
      card.hidden = false;
      position(anchor);
      if (!STILL.matches) {
        card.classList.remove('is-in');
        void card.offsetWidth;
        card.classList.add('is-in');
      }
    }, 90);
  }

  function hide(immediate) {
    if (showTimer) { clearTimeout(showTimer); showTimer = null; }
    if (!card || card.hidden) { return; }
    var doHide = function() {
      if (!card.hidden) {
        card.hidden = true;
        card.classList.remove('is-in');
      }
    };
    if (immediate || STILL.matches) { doHide(); return; }
    hideTimer = setTimeout(doHide, 80);
  }

  document.addEventListener('mouseover', function(e) {
    var a = e.target && e.target.closest ? e.target.closest('a[href^="/blog/"]') : null;
    if (!a) { return; }
    var href = a.getAttribute('href') || '';
    var slug = href.slice('/blog/'.length);
    var item = data[slug];
    // A link with no entry is not a failure: it points at an unpublished or unknown note, and
    // the correct behaviour is to show nothing at all.
    //
    // 没有条目的链接不是失败：它指向未发布或未知的笔记，正确行为是什么都不显示。
    if (!item) { return; }
    show(a, item);
  });

  document.addEventListener('mouseout', function(e) {
    var a = e.target && e.target.closest ? e.target.closest('a[href^="/blog/"]') : null;
    if (a) { hide(false); }
  });

  // Scrolling or resizing invalidates the card's coordinates; hiding is simpler and more honest
  // than chasing the anchor.
  // 滚动或缩放会让卡片的坐标失效；隐藏比追着锚点跑更简单也更诚实。
  window.addEventListener('scroll', function() { hide(true); }, { passive: true });
  window.addEventListener('resize', function() { hide(true); });
  if (DESKTOP.addEventListener) {
    DESKTOP.addEventListener('change', function(e) { if (!e.matches) { hide(true); } });
  }
})();`
