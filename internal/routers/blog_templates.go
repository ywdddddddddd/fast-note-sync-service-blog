package routers

import (
	"fmt"
	"html/template"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// 静态脚本的版本号：进程启动时刻，36 进制。
//
// 为什么必须让 URL 变化：/blog/ask.js 与 /blog/lightbox.js 走的是「no-cache + 无验证器」，
// 按规范浏览器每次都要重新请求，但微信的 webview（以及部分代理）并不总遵守这一点，
// 部署之后客户端仍在跑旧脚本 —— 表现就是「明明改好了，页面上一点变化都没有」。
// URL 一变，任何缓存层都绕不过去。代价只是每次重启多发一个 query。
var blogAssetVer = strconv.FormatInt(time.Now().Unix(), 36)

// blogAssetFuncs 供模板把版本号拼进 src 属性。
var blogAssetFuncs = template.FuncMap{
	"assetVer": func() string { return blogAssetVer },
}

// Blog templates. Everything is rendered on the server, so the pages work with JavaScript
// disabled and are indexable — the opposite trade-off from the native share page, which is a
// client-rendered SPA.
//
// 博客模板。全部内容在服务端渲染，因此页面在禁用 JavaScript 时依然可用、并能被搜索引擎收录
// ——这与原生分享页（客户端渲染的 SPA）正好相反。

// tocEntry is one heading in the right-hand outline.
// tocEntry 右侧大纲中的一个标题。
type tocEntry struct {
	// ID is an ASCII anchor like "s-3", assigned in document order.
	//
	// It is not the heading's own id. Those are generated from the heading text and are therefore
	// Chinese here; an href carrying the same characters would be percent-encoded by
	// html/template, and `#1-%e7%89%99...` does not match `id="1-牙发育异常..."`. A fragment that
	// matches nothing is a no-op, which is why click-to-jump and the scroll-spy were both dead.
	//
	// An anchor is a jump coordinate, not content: it does not need to keep the heading's words.
	// Renumbering both sides to plain ASCII removes the mismatch instead of trying to make two
	// encoders agree.
	//
	// ID 是形如 "s-3" 的 ASCII 锚点，按文档顺序分配。
	//
	// 它不是标题自身的 id。那些 id 由标题文本生成，在这里因此是中文；而带着同样字符的 href
	// 会被 html/template 百分号编码，`#1-%e7%89%99...` 与 `id="1-牙发育异常..."` 不相等。
	// 匹配不到任何元素的片段是空操作 —— 这正是点击跳转与滚动高亮双双失效的原因。
	//
	// 锚点是一个跳转坐标，不是内容：它不需要保留标题的字。
	// 把两侧一起改成纯 ASCII 序号，是消除这个不匹配，而不是去让两个编码器达成一致。
	ID    string
	Text  string
	Level int
}

// blogHomeView is the data bound to the blog home template.
// blogHomeView 绑定到博客首页模板的数据。
type blogHomeView struct {
	Title     string
	SiteName  string
	Indexable bool
	CSS       template.CSS
	Stats     *dto.BlogStatsDTO
	Tags      []*dto.BlogTagDTO
	Entries   []*dto.BlogEntryDTO
	Nav       []dto.BlogNavNode
	// Page-note entry points for the top bar. See blogPostView.Pages.
	// 顶栏的页面笔记入口。说明见 blogPostView.Pages。
	Pages []dto.BlogNavPageDTO
	// HasShelf shows the 资料 entry. True only when the shelf manifest lists something: an entry
	// leading to an empty shelf is the same apology an empty 热门 block would be.
	//
	// HasShelf 决定是否显示「资料」入口。只有在书目里确实有东西时为真：
	// 一个通往空书架的入口，与一个空的「热门」区块是同一种道歉。
	HasShelf   bool
	ActiveSlug string
	GraphSVG   template.HTML
	GraphNote  string
	Truncated  bool
	Activity   *dto.BlogActivityDTO
	ShowGraph  bool
	AskEnabled bool
	// Visits is the run-total visit count, shown as the dashboard's seventh figure.
	// Visits 是累计访问次数，作为仪表盘的第七个数字显示。
	Visits int64
	// Hot is the popular list: articles with at least one like, most-liked first, five at most.
	// It is empty when nothing has been liked, and the template omits the block entirely then.
	//
	// Hot 是热门列表：至少有一个赞的文章，赞多的在前，最多五条。
	// 没有任何赞时它是空的，此时模板整块省略。
	Hot []*dto.BlogHotItemDTO
	// ReactionJS carries the count-up for the dashboard's seven figures. The interaction-bar half
	// of the script returns early on this page, because there is no bar here.
	//
	// ReactionJS 承载仪表盘七个数字的滚动。脚本里属于互动条的那一半在这个页面上会提前返回，
	// 因为这里没有互动条。
	ReactionJS template.JS
	// EntriesJSON carries the same entries as a compact JSON array, for the two client-rendered
	// views. The cover grid and the wheel need the same fields the list already renders, and
	// rendering that markup twice would add roughly 200 KB to a page that is already 446 KB;
	// this array is about 30 KB.
	//
	// EntriesJSON 以紧凑 JSON 数组承载同一批条目，供两个客户端渲染的视图使用。
	// 封面网格与同心轮需要的字段和列表已经渲染的完全一样，而把那套标记再渲染一遍，
	// 会让一个本已 446 KB 的页面再长出约 200 KB；这个数组只有约 30 KB。
	EntriesJSON template.JS
	// WarpJS carries the warp transition. The home page uses it to leave; every page uses it to
	// fade in when it was reached through the warp.
	//
	// WarpJS 承载穿越过渡。首页用它离开；每个页面用它在自己是经由穿越抵达时渐显。
	WarpJS template.JS
	// ParticlesJS is the ambient field behind the home page, and ParticlesCfg its configuration.
	//
	// The count in that configuration is the number of published notes: the field is a picture of
	// how much is in the blog, and it changes when a note is shared or unshared. It is serialised
	// with encoding/json rather than assembled in the template, for the same reason the reaction
	// config is.
	//
	// ParticlesJS 是首页背后的环境粒子场，ParticlesCfg 是它的配置。
	//
	// 配置里的数量等于已发布笔记数：这片场是「博客里有多少东西」的一张图，
	// 它会随着分享或取消分享而变化。它用 encoding/json 序列化而不是在模板里拼装，
	// 理由与互动条的配置相同。
	ParticlesJS  template.JS
	ParticlesCfg template.JS

	// Discovery / SEO. Canonical and FeedURL are absolute URLs; JSONLD is serialised by the
	// caller so the template performs no encoding of its own.
	// 发现能力 / SEO。Canonical 与 FeedURL 是绝对地址；JSONLD 由调用方序列化，
	// 模板自身不做任何编码。
	Canonical   string
	Description string
	FeedURL     string
	JSONLD      template.JS
	// 备案号：空表示不渲染那一行。见 config.BlogConfig.ICP。
	ICP     string
	MPS     string
	MPSCode string
}

// blogPostView is the data bound to the article template.
// blogPostView 绑定到文章模板的数据。
type blogPostView struct {
	Title     string
	SiteName  string
	Indexable bool
	CSS       template.CSS
	Nav       []dto.BlogNavNode
	// Pages are the page-note entry points shown in the top bar. They are not part of Nav
	// because the tree is the folder structure of articles, and a page has no folder position.
	//
	// Pages 是顶栏里的页面笔记入口。它们不属于 Nav，因为目录树是文章按文件夹的组织方式，
	// 而页面不属于任何文件夹位置。
	Pages []dto.BlogNavPageDTO
	// HasShelf shows the 资料 entry. See blogHomeView.HasShelf.
	// HasShelf 决定是否显示「资料」入口。见 blogHomeView.HasShelf。
	HasShelf   bool
	ActiveSlug string
	PostTitle  string
	PostPath   string
	PostMeta   []string
	Body       template.HTML
	TOC        []tocEntry
	Tags       []string
	AskEnabled bool

	// Discovery / SEO. Canonical and FeedURL are absolute URLs; JSONLD is serialised by the
	// caller so the template performs no encoding of its own.
	// 发现能力 / SEO。Canonical 与 FeedURL 是绝对地址；JSONLD 由调用方序列化，
	// 模板自身不做任何编码。
	Canonical   string
	Description string
	FeedURL     string
	JSONLD      template.JS

	// HasMath is true when the rendered body contains a formula. MathJax is a ~200 KB script,
	// so it is only referenced from articles that actually need it.
	// HasMath 在渲染后的正文含公式时为真。MathJax 是约 200 KB 的脚本，
	// 因此只从真正需要的文章里引入。
	HasMath bool

	// Local neighbourhood graph. ShowLocalGraph is false when the graph is disabled, when the
	// note has no neighbours, or when the article is not in the published set — the card is
	// then omitted entirely rather than shown empty.
	// 局部邻域图。ShowLocalGraph 在图谱被关闭、该笔记没有邻居、或文章不在已发布集合中时为假——
	// 此时整个卡片被省略，而不是显示一个空壳。
	ShowLocalGraph  bool
	LocalGraphSVG   template.HTML
	LocalGraphNote  string
	LocalGraphDepth int

	// GraphDataByDepth holds one data island per local depth, keyed by depth. The slider swaps
	// between them; none of them needs a request.
	// GraphDataByDepth 按深度保存局部数据岛。滑块在它们之间切换，都不需要请求。
	GraphDataByDepth map[int]template.JS
	// GraphDepths lists the depths actually present, in order, so the slider's range reflects
	// what exists rather than a constant that a sparse graph might not fill.
	// GraphDepths 按顺序列出实际存在的深度，使滑块的范围反映真实存在的数据，
	// 而不是一个稀疏图可能填不满的常量。
	GraphDepths []int
	// The fullscreen layer's data is fetched from /blog/graph?scope=global&format=json on demand
	// rather than inlined, so there is no field for it here.
	// 浮层的数据按需从 /blog/graph?scope=global&format=json 获取而不是内联，因此这里没有对应字段。
	// GraphEngineJS is injected as template.JS: html/template would otherwise escape it in a
	// JavaScript context and break the code. It is the only script that touches the canvas — an
	// earlier second script replaced the canvas contents and destroyed the simulation's DOM.
	// GraphEngineJS 以 template.JS 注入：否则 html/template 会在 JavaScript 上下文中转义它并破坏代码。
	// 它是唯一接触画布的脚本——早先的第二个脚本会替换画布内容并摧毁模拟所在的 DOM。
	GraphEngineJS template.JS

	// PreviewJSON is a JSON island: the hover cards' data, already on the page so that a hover
	// never waits on the network. It is injected as template.JS because html/template would
	// otherwise escape the JSON into a string.
	// PreviewJSON 是一块 JSON 岛：悬停卡片的数据，已在页面上，因此悬停从不需要等网络。
	// 它以 template.JS 注入，否则 html/template 会把 JSON 转义成一个字符串。
	PreviewJSON template.JS
	PreviewJS   template.JS

	// ── Interaction bar ──────────────────────────────────────────────────────────────────
	// ── 互动条 ────────────────────────────────────────────────────────────────────────────
	// Like and Dislike are the stored tallies, and both are shown. The user asked for the dislike
	// count to be public; a count that is collected but withheld is worse than either alternative,
	// because it takes the signal and gives nothing back.
	//
	// Like 与 Dislike 是已存储的计数，两者都显示。用户要求把踩数公开；
	// 一个「收集了却扣着不给」的计数比两种做法都糟，因为它拿走了信号，什么也没给回来。
	Like    int64
	Dislike int64
	// Visits is the run-total for the whole blog, shown with the same count-up motion the
	// dashboard uses. It is deliberately not "this article's views": per-article view counts are
	// not stored, and deriving one from the access log would present a 30-day window as a total.
	//
	// Visits 是整个博客的累计访问次数，以与仪表盘相同的数字滚动呈现。
	// 它刻意不是「本文浏览量」：逐篇浏览数并未存储，而从访问日志推导一个，
	// 等于把一个 30 天的窗口当作总数展示。
	Visits int64
	// ReactionJS is the particle engine plus the interaction bar's behaviour, injected as
	// template.JS so html/template does not escape it.
	//
	// ReactionJS 是粒子引擎加互动条的行为，以 template.JS 注入以免 html/template 转义。
	ReactionJS template.JS
	// ReactionCfg is the script's configuration, serialised by the caller with encoding/json
	// rather than assembled in the template.
	//
	// Assembling it in the template would mean interpolating a slug — a string that comes from
	// the URL — into a JSON document by concatenation, which is how a quote in the wrong place
	// turns into broken JavaScript. encoding/json escapes it correctly for free.
	//
	// ReactionCfg 是脚本的配置，由调用方用 encoding/json 序列化，而不是在模板里拼装。
	//
	// 在模板里拼装意味着把一个来自 URL 的 slug 用字符串连接塞进 JSON 文档，
	// 而一个位置不对的引号就是这样变成坏掉的 JavaScript 的。
	ReactionCfg template.JS
	// DislikeBurst enables the dislike particle burst; ParticleCount is how many particles one
	// burst spawns. Both are settings because this blog gets opened in meetings and while
	// screen-sharing, where a brown burst is not always wanted.
	//
	// DislikeBurst 启用点踩的粒子喷发；ParticleCount 是一次喷发产生的粒子数。
	// 两者都是配置项，因为这个博客会在会议与投屏时被打开，那时棕色喷发并非总是合适。
	DislikeBurst  bool
	ParticleCount int
	// WarpJS carries the arriving half of the warp transition, which every page needs because a
	// reader may land on any of them after leaving the home page.
	//
	// WarpJS 承载穿越过渡中「抵达」的那一半；每个页面都需要它，
	// 因为读者离开首页后可能落到其中任何一个。
	WarpJS template.JS
	// 备案号：空表示不渲染那一行。见 config.BlogConfig.ICP。
	ICP     string
	MPS     string
	MPSCode string
}

// The sidebar renders a recursive folder tree using native <details>/<summary>. That gives
// collapse/expand and real nesting with no JavaScript at all, and the open state survives
// nothing — which is fine for a reading view.
//
// 侧栏用原生 <details>/<summary> 递归渲染目录树：无需任何 JavaScript 即可获得折叠/展开与
// 真正的层级嵌套。
// topnav renders the page-note entry points.
//
// It renders nothing at all when there is no page note — an empty <nav> would still take part in
// the flex layout and shift the breadcrumb.
//
// The "about" entry carries: that is what the warp transition binds its departing
// half to. It moved here from the right-hand card, which is gone — the top bar is now the only
// way into the résumé, so the transition follows the entry rather than the other way round.
//
// topnav 渲染页面笔记的入口。
//
// 没有页面笔记时它什么都不输出：一个空的 <nav> 仍会参与弹性布局，把面包屑挤偏。
//
// "about" 那一条带：穿越过渡的「出发」那一半绑的就是它。
// 它从右栏那张卡片迁到这里，卡片已经撤掉——顶栏现在是通往简历的唯一入口，
// 所以过渡跟着入口走，而不是让入口迁就过渡。
// The subscribe entry sits after the shelf link, per the owner's call that function entries come
// last. It is conditional on the same two fields the <link rel="alternate"> uses, because
// blog.indexable=false makes /blog/rss.xml answer 404 — an entry point that leads to an error page
// is worse than no entry point.
//
// The icon is inlined rather than referenced, so the mobile variant (icon only) needs no extra
// request and no font.
//
// 订阅入口排在资料之后，遵循「功能入口放在最后」的决定。它依赖与 <link rel="alternate">
// 相同的两个字段，因为 blog.indexable=false 会让 /blog/rss.xml 返回 404 ——
// 一个通往错误页的入口比没有入口更糟。
//
// 图标内联而不是引用，使移动端的纯图标形态不需要额外请求，也不需要字体。
const blogTopNav = `
{{define "topnav"}}{{if or .Pages .HasShelf .FeedURL}}<nav class="topnav">{{range .Pages}}<a href="{{.URL}}"{{if eq .Alias "about"}}{{end}}>{{.Title}}</a>{{end}}{{if .HasShelf}}<a href="/blog/shelf">资料</a>{{end}}{{if and .Indexable .FeedURL}}<button type="button" class="topnav-feed" data-feed="{{.FeedURL}}" aria-label="订阅《{{.SiteName}}》"><svg class="topnav-feed-icon" width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path d="M2.002 2.725a.75.75 0 0 1 .797-.699C8.79 2.42 13.58 7.21 13.974 13.201a.75.75 0 0 1-1.497.098 10.502 10.502 0 0 0-9.776-9.776.747.747 0 0 1-.7-.798ZM2.84 7.05h-.002a7.002 7.002 0 0 1 6.113 6.111.75.75 0 0 1-1.49.178 5.503 5.503 0 0 0-4.8-4.8.75.75 0 0 1 .179-1.489ZM2 13a1 1 0 1 1 2 0 1 1 0 0 1-2 0Z"/></svg><span class="topnav-feed-text">订阅</span></button>{{end}}</nav>{{end}}{{end}}`

// blogFooterLegal is the shared filing-number block.
//
// 备案号来自配置（blog.icp / blog.mps），空值渲染为空字符串 ——
// 开源仓库里不带任何真实备案号，部署者在 config.yaml 填自己的。
//
// 链接说明：ICP 指向工信部备案系统；公安备案指向官方查询页，带 code 时直接定位到本站记录。
const blogFooterLegal = `
{{define "footer-legal"}}{{if .ICP}}<div class="footer-legal"><a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer nofollow">{{.ICP}}</a></div>{{end}}{{if .MPS}}<div class="footer-legal"><a href="https://beian.mps.gov.cn/#/query/webSearch{{if .MPSCode}}?code={{.MPSCode}}{{end}}" target="_blank" rel="noopener noreferrer nofollow">{{.MPS}}</a></div>{{end}}{{end}}`

const blogNavTree = `
{{define "navnode"}}{{if .Children}}<details class="nav-folder"{{if le .Depth 1}} open{{end}}>
<summary class="nav-head">
<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2Z"/></svg>
<span class="nav-name">{{.Name}}</span><span class="count">{{.Count}}</span>
</summary>
<div class="nav-children">{{range .Children}}{{template "navnode" .}}{{end}}</div>
</details>{{else}}<a class="nav-item{{if .Active}} on{{end}}" href="/blog/{{.Slug}}" title="{{.Name}}">{{.Name}}</a>{{end}}{{end}}`

// blogNavPartial is the sidebar shell; the tree inside is rendered by the "navnode" template.
// blogNavPartial 是侧栏外壳，其中的树由 "navnode" 模板递归渲染。
const blogNavPartial = `
<aside class="panel sticky nav aside">
  <div class="nav-title">目录</div>
  {{if .Nav}}
    {{range .Nav}}{{template "navnode" .}}{{end}}
  {{else}}
    <div class="nav-empty">还没有公开的笔记</div>
  {{end}}
</aside>
<script>
/* 记住目录树的展开状态。

   服务端把前两级默认展开（见 BlogNavNode.Depth），那是无 JavaScript 时的行为。
   这段脚本在此之上只做一件事：把读者显式改动过的文件夹存下来，下次导航时还原。
   于是「展开某个栏目 -> 点进文章 -> 返回」之后，那个栏目仍然是展开的。

   只存读者改过的那些，不存整棵树的快照：新增的文件夹仍按默认展开，
   不会被一份陈旧快照错误地保持折叠。

   标识用 DOM 位置（所有 .nav-folder 的出现序号），不用文件夹名 ——
   同名文件夹在不同层级很常见，用名字会互相串改。 */
(function () {
  var KEY = 'fns:nav:state';
  var folders = document.querySelectorAll('.nav-folder');
  if (!folders.length) { return; }

  var saved = {};
  try { saved = JSON.parse(localStorage.getItem(KEY) || '{}') || {}; } catch (e) { saved = {}; }

  var write = function () {
    try { localStorage.setItem(KEY, JSON.stringify(saved)); } catch (e) { /* 存不下就算了 */ }
  };

  Array.prototype.forEach.call(folders, function (el, i) {
    var k = String(i);
    // 还原读者上次的选择。没有记录时保留服务端给的默认。
    if (Object.prototype.hasOwnProperty.call(saved, k)) {
      el.open = saved[k] === 1;
    }
    el.addEventListener('toggle', function () {
      saved[k] = el.open ? 1 : 0;
      write();
    });
  });
})();
</script>`

var blogHomeTmpl = template.Must(template.New("blog-home").Funcs(blogAssetFuncs).Parse(blogFooterLegal + `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
{{if not .Indexable}}<meta name="robots" content="noindex, nofollow">{{end}}
<meta name="theme-color" content="#eae9e3">
<meta name="description" content="{{.Description}}">
{{if .Canonical}}<link rel="canonical" href="{{.Canonical}}">{{end}}
{{if and .Indexable .FeedURL}}<link rel="alternate" type="application/rss+xml" title="{{.SiteName}}" href="{{.FeedURL}}">{{end}}
<meta property="og:title" content="{{.SiteName}}">
<meta property="og:site_name" content="{{.SiteName}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:type" content="website">
{{if .Canonical}}<meta property="og:url" content="{{.Canonical}}">{{end}}
<meta name="twitter:card" content="summary">
<meta name="twitter:title" content="{{.SiteName}}">
<meta name="twitter:description" content="{{.Description}}">
{{if .JSONLD}}<script type="application/ld+json">{{.JSONLD}}</script>{{end}}
<style>{{.CSS}}</style>
</head>
<body class="is-home">
<div class="backdrop"></div>
<div class="shell">
  <header class="topbar"><div class="topbar-inner">
    <div class="brand">
      <span class="brand-mark"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/></svg></span>
      <div style="min-width:0">
        <div class="brand-name">{{.SiteName}}</div>
        <div class="brand-sub"><span class="bs-wide">{{.Stats.Notes}} 篇 · {{.Stats.Words}} 字 · {{.Stats.Links}} 条链接</span><span class="bs-compact">{{.Stats.Notes}} 篇 · {{.Stats.WordsText}}字 · {{.Stats.Links}} 双链</span></div>
      </div>
    </div>
    <div class="topbar-spacer"></div>
    {{template "topnav" .}}
    <div class="crumb">{{if .Stats.Updated}}最近更新 {{.Stats.Updated}}{{end}}</div>
  </div></header>

  <div class="layout layout-home">
    {{template "nav" .}}
    <main class="content">

      <section class="card card-pad board">
        <h2 class="card-title">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3v16a2 2 0 0 0 2 2h16"/><path d="m19 9-5 5-4-4-3 3"/></svg>
          仪表盘
        </h2>
        <!-- Every figure carries its value twice: as text for readers without JavaScript, and as
             data-countup for the script to animate from zero. The text is the authority — if the
             script never runs, the page still shows the real numbers.

             每个数字都带两份:文本给没有 JavaScript 的读者,data-countup 给脚本从零滚动。
             文本是权威——脚本从不运行,页面仍然显示真实数字。 -->
        <div class="stats">
          <div class="stat"><div class="stat-num" data-countup="{{.Stats.Notes}}">{{.Stats.Notes}}</div><div class="stat-label">公开笔记</div></div>
          <div class="stat"><div class="stat-num" data-countup="{{.Stats.Words}}">{{.Stats.Words}}</div><div class="stat-label">总字数</div></div>
          <div class="stat"><div class="stat-num" data-countup="{{.Stats.Links}}">{{.Stats.Links}}</div><div class="stat-label">双向链接</div></div>
          <div class="stat"><div class="stat-num" data-countup="{{.Stats.Tags}}">{{.Stats.Tags}}</div><div class="stat-label">标签</div></div>
          <div class="stat"><div class="stat-num" data-countup="{{.Stats.Folders}}">{{.Stats.Folders}}</div><div class="stat-label">来源目录</div></div>
          <div class="stat"><div class="stat-num" data-countup="{{.Stats.AvgWords}}">{{.Stats.AvgWords}}</div><div class="stat-label">篇均字数</div></div>
          <!-- The seventh figure is the run-total visit count. It is labelled 访问次数 rather
               than 阅读量 because it includes refreshes and crawlers — see blogReactionLog.addVisit.

               第七个数字是累计访问次数。它标为「访问次数」而不是「阅读量」,因为它包含刷新与爬虫
               —— 见 blogReactionLog.addVisit。 -->
          <div class="stat"><div class="stat-num" data-countup="{{.Visits}}">{{.Visits}}</div><div class="stat-label">访问次数</div></div>
        </div>
      </section>

      {{if .Tags}}
      <section class="card card-pad">
        <h2 class="card-title">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.704 8.704a2.426 2.426 0 0 0 3.42 0l6.58-6.58a2.426 2.426 0 0 0 0-3.42z"/><circle cx="7.5" cy="7.5" r=".5" fill="currentColor"/></svg>
          标签
        </h2>
        <div class="tags">
          {{range .Tags}}<span class="tag {{if ge .Count 8}}s3{{else if ge .Count 4}}s2{{else if ge .Count 2}}s1{{end}}">{{.Name}}<i>{{.Count}}</i></span>{{end}}
        </div>
      </section>
      {{end}}

      <section class="card card-pad">
        <div class="list-head">
          <h2 class="card-title">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12h.01"/><path d="M3 18h.01"/><path d="M3 6h.01"/><path d="M8 12h13"/><path d="M8 18h13"/><path d="M8 6h13"/></svg>
            全部笔记
          </h2>
          {{if .Entries}}<div class="view-switch" role="group" aria-label="笔记显示方式">
            <button type="button" class="vs-btn is-on" data-view="list" aria-pressed="true">列表</button>
            <button type="button" class="vs-btn" data-view="cover" aria-pressed="false">封面</button>
            <button type="button" class="vs-btn" data-view="wheel" aria-pressed="false">动效</button>
          </div>{{end}}
        </div>
        {{if .Entries}}
        <div class="list" data-view-body="list">
          {{range .Entries}}
          <a class="item" href="/blog/{{.Slug}}">
            <h3>{{.Title}}</h3>
            {{if .Summary}}<p>{{.Summary}}</p>{{end}}
            <div class="item-meta">
              {{if .CreatedAt}}<span class="m-date"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2v4"/><path d="M16 2v4"/><rect width="18" height="18" x="3" y="4" rx="2"/><path d="M3 10h18"/></svg>{{.CreatedAt}}</span>{{end}}
              <span class="m-words"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 7V4h16v3"/><path d="M9 20h6"/><path d="M12 4v16"/></svg>{{.Words}} 字</span>
              <span class="m-link"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>出链 {{.LinksOut}} · 入链 {{.LinksIn}}</span>
              {{if .Vault}}<span class="m-cat">{{.Vault}}</span>{{end}}
            </div>
          </a>
          {{end}}
        </div>
        <div class="cover-grid" data-view-body="cover" hidden></div>
        <script type="application/json" id="fns-entries">{{.EntriesJSON}}</script>
        <div id="wheel-root" class="wheel-root" hidden></div>
        {{/* Only the home page has entries to draw, so the script rides along with them. */}}
        <script src="/blog/views.js?v={{assetVer}}" defer></script>
        {{else}}
        <p class="nav-empty">还没有公开分享的笔记。在 Obsidian 里分享一篇，它就会出现在这里。</p>
        {{end}}
      </section>
    </main>

    <aside class="rail">
      {{if .Activity}}
      <div class="panel act">
        <div class="nav-title">活跃度</div>
        <div class="act-stats">
          <div><b>{{.Stats.Notes}}</b><span>笔记</span></div>
          <div><b>{{.Stats.Tags}}</b><span>标签</span></div>
          <div><b>{{.Activity.NoteDays}}</b><span>天</span></div>
        </div>
        <div class="heat" style="--weeks:{{.Activity.Weeks}}">
          {{range .Activity.Days}}<i class="hc lv{{.Lvl}}" title="{{.Date}} · {{.Count}} 篇创建"></i>{{end}}
        </div>
        <div class="heat-months" style="--weeks:{{.Activity.Weeks}}">
          {{range .Activity.Months}}<i>{{.}}</i>{{end}}
        </div>
        <p class="act-note">按笔记的创建日期统计，最近 {{.Activity.Weeks}} 周。单日最多 {{.Activity.Max}} 篇。</p>
      </div>
      {{end}}
      {{if .Hot}}
      <!-- Popular articles, sorted by likes. The whole block is omitted when nothing has been
           liked: an empty "热门" card would be a permanent apology for a feature nobody used, and
           一个空的容器比没有容器更难看 —— 它占据视线却什么也没说。

           热门的整块在没有赞时不渲染:一个空的「热门」卡片等于为没人用过的功能长期道歉。 -->
      <div class="panel hot" style="padding:.9rem 1rem 1.1rem">
        <div class="nav-title">热门</div>
        <ol class="hot">
          {{range .Hot}}<li><a href="/blog/{{.Slug}}" title="{{.Title}}">{{.Title}}</a><span class="hot-n">{{.Like}}</span></li>{{end}}
        </ol>
      </div>
      {{end}}
      {{if .Truncated}}
      <div class="panel" style="padding:.9rem 1rem 1.1rem">
        <div class="nav-title">提示</div>
        <p style="margin:.2rem 0 0;font-size:.75rem;line-height:1.7;color:var(--muted-foreground)">
          图谱节点较多，已截断显示以保证渲染速度。
        </p>
      </div>
      {{end}}
    </aside>
    <!-- 遮罩必须与抽屉同级。.shell 有 z-index:1，会在它内部新建层叠上下文 ——
         遮罩若放在 .shell 外面，body 层上的 z-index:57 会盖住 shell 里那个 z-index:58 的抽屉，
         于是抽屉被自己的遮罩模糊掉。这条约束与阅读页相同，原因也相同。 -->
    <div class="fns-scrim" id="fns-scrim" hidden></div>
  </div>

  <footer class="footer">
    <div>由 <a href="https://github.com/haierkeys/fast-note-sync-service" target="_blank" rel="noopener noreferrer">Fast Note Sync</a> 提供支持</div>
    {{template "footer-legal" .}}
  </footer>
</div>
<!-- 首页的三个入口：爻路（随机漫步）、山（目录）、问道（问答）。
     位置上它们必须在正文之后，因为这是 fixed 元素，出现在 DOM 末尾才不会与流内内容抢位置。
      左起：目录 · 热门 · 漫游 · 活跃 · 问道。
      图标全部来自 Primer Octicons（16 网格、fill、单色），不再自绘 ——
      上一版是手写的 24 网格线性图标，用户反馈"太抽象看不懂"：
      山、S 形竖线、2×2 方块都需要解释，而 icon 的第一职责是不解释就懂。
      Octicons 是 GitHub 自己那套，形状直白、笔画干净、体积小，
      换过去之后同一组图标之间也不再各自为政（以前线宽 1.4 是统一的，
      但造型语言从三角脊线到游泳小人到实心方块，根本不是一套）。 -->
<div class="dock" id="fns-dock" role="group" aria-label="首页入口" data-slot="0">
  <span class="dock-thumb" aria-hidden="true"></span>
  <button type="button" class="dock-b" data-i="0" id="dock-nav" aria-label="目录" aria-expanded="false">
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M0 1.75A.75.75 0 0 1 .75 1h4.253c1.227 0 2.317.59 3 1.501A3.743 3.743 0 0 1 11.006 1h4.245a.75.75 0 0 1 .75.75v10.5a.75.75 0 0 1-.75.75h-4.507a2.25 2.25 0 0 0-1.591.659l-.622.621a.75.75 0 0 1-1.06 0l-.622-.621A2.25 2.25 0 0 0 5.258 13H.75a.75.75 0 0 1-.75-.75Zm7.251 10.324.004-5.073-.002-2.253A2.25 2.25 0 0 0 5.003 2.5H1.5v9h3.757a3.75 3.75 0 0 1 1.994.574ZM8.755 4.75l-.004 7.322a3.752 3.752 0 0 1 1.992-.572H14.5v-9h-3.495a2.25 2.25 0 0 0-2.25 2.25Z"/></svg>
  </button>
  <button type="button" class="dock-b" data-i="1" id="dock-hot" aria-label="热门" aria-expanded="false">
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M9.533.753V.752c.217 2.385 1.463 3.626 2.653 4.81C13.37 6.74 14.498 7.863 14.498 10c0 3.5-3 6-6.5 6S1.5 13.512 1.5 10c0-1.298.536-2.56 1.425-3.286.376-.308.862 0 1.035.454C4.46 8.487 5.581 8.419 6 8c.282-.282.341-.811-.003-1.5C4.34 3.187 7.035.75 8.77.146c.39-.137.726.194.763.607ZM7.998 14.5c2.832 0 5-1.98 5-4.5 0-1.463-.68-2.19-1.879-3.383l-.036-.037c-1.013-1.008-2.3-2.29-2.834-4.434-.322.256-.63.579-.864.953-.432.696-.621 1.58-.046 2.73.473.947.67 2.284-.278 3.232-.61.61-1.545.84-2.403.633a2.79 2.79 0 0 1-1.436-.874A3.198 3.198 0 0 0 3 10c0 2.53 2.164 4.5 4.998 4.5Z"/></svg>
  </button>
  <button type="button" class="dock-b" data-i="2" id="dock-walk" aria-label="漫游 · 随机打开一篇笔记">
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M13.25 1c.966 0 1.75.784 1.75 1.75v10.5c0 .464-.184.909-.513 1.237A1.746 1.746 0 0 1 13.25 15H2.75c-.464 0-.909-.184-1.237-.513A1.746 1.746 0 0 1 1 13.25V2.75C1 1.784 1.784 1 2.75 1h10.5ZM2.75 2.5a.25.25 0 0 0-.25.25v10.5c0 .138.112.25.25.25h10.5a.25.25 0 0 0 .25-.25V2.75a.25.25 0 0 0-.25-.25H2.75Z"/><path d="M5 12a1 1 0 1 0 0-2 1 1 0 0 0 0 2Zm6-6a1 1 0 1 0 0-2 1 1 0 0 0 0 2ZM8 9a1 1 0 1 0 0-2 1 1 0 0 0 0 2Z"/></svg>
  </button>
  <button type="button" class="dock-b" data-i="3" id="dock-live" aria-label="活跃度" aria-expanded="false">
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M6 2c.306 0 .582.187.696.471L10 10.731l1.304-3.26A.751.751 0 0 1 12 7h3.25a.75.75 0 0 1 0 1.5h-2.742l-1.812 4.528a.751.751 0 0 1-1.392 0L6 4.77 4.696 8.03A.75.75 0 0 1 4 8.5H.75a.75.75 0 0 1 0-1.5h2.742l1.812-4.529A.751.751 0 0 1 6 2Z"/></svg>
  </button>
  <button type="button" class="dock-b" data-i="4" id="dock-ask" aria-label="问道 · 向知识库提问" aria-expanded="false" data-ask-open>
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M1.75 1h8.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 10.25 10H7.061l-2.574 2.573A1.458 1.458 0 0 1 2 11.543V10h-.25A1.75 1.75 0 0 1 0 8.25v-5.5C0 1.784.784 1 1.75 1ZM1.5 2.75v5.5c0 .138.112.25.25.25h1a.75.75 0 0 1 .75.75v2.19l2.72-2.72a.749.749 0 0 1 .53-.22h3.5a.25.25 0 0 0 .25-.25v-5.5a.25.25 0 0 0-.25-.25h-8.5a.25.25 0 0 0-.25.25Zm13 2a.25.25 0 0 0-.25-.25h-.5a.75.75 0 0 1 0-1.5h.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 14.25 12H14v1.543a1.458 1.458 0 0 1-2.487 1.03L9.22 12.28a.749.749 0 0 1 .326-1.275.749.749 0 0 1 .734.215l2.22 2.22v-2.19a.75.75 0 0 1 .75-.75h1a.25.25 0 0 0 .25-.25Z"/></svg>
  </button>
</div>
<script>
/* 首页五个入口的接线。

   一、滑块。五个键共用一条轨道上的一个滑块，点哪滑到哪 —— 这是「五段式开关」的手感：
   位置本身在告诉你刚才动了什么。滑块移动 300ms，跳转类的动作等它走完再发生。

   二、抽屉只有一条。「热门」「活跃」「山」三者互斥，且换模块时先收再开 ——
   两条抽屉同时平移会让人分不清哪条是刚打开的；多花 300ms，顺序读得出来。

   三、漫游不需要请求。146 张卡已经在 DOM 里，随机只是取一个 href。
   这也是这一页不分页的理由之一：分了页，这个按钮就得先问服务器要一篇。
   跳转前会 prefetch 目标文档，动画结束的同一帧开始导航，画面因此接得上。

   四、全部失效时（脚本不执行）五个键都不响应，页面本身仍然完整可读 ——
   与目录树、计数、图谱的一贯处理相同：装饰与加速可以没有，内容不能没有。 */
(function () {
  var dock = document.getElementById('fns-dock');
  if (!dock) { return; }
  var scrim = document.getElementById('fns-scrim');
  // 与 .dock-thumb 的过渡时长一致。跳转类入口等这一段时间播完再走，
  // 否则「点了没反应」那 300ms 会让人再点一次。
  var SLIDE = 300;
  var DRAWERS = { nav: 'drawer-nav', hot: 'drawer-hot', live: 'drawer-live' };
  // 跨页进来时让胶囊自己重组一次：读者因此知道这一条的顺序与上一页不同。
  // 只在站内导航时播；首屏进入网站不播，那会显得页面一直在加载。
  (function () {
    var ref = document.referrer || '';
    var sameSite = ref && ref.indexOf(location.protocol + '//' + location.host) === 0;
    /* 必须比 pathname 全等，不能用子串包含：
       从 /blog/about 回 /blog 时，referrer 里同样含 "/blog"，
       子串判断会把这次真实跨页当成"同页刷新"，整段动画不播。
       （/blog 是 /blog/about 的前缀，任何这种前缀关系都会误判。） */
    var samePage = false;
    try { samePage = new URL(ref).pathname === location.pathname; } catch (e) { samePage = false; }
    var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (!sameSite || samePage || reduce) { return; }
    /* 先量出自然宽度，再交给动画当终点。
       量的时候 --dock-w 还没写，.dock 的宽度是 auto，量到的就是真实值；
       先写再量会把宽度锁成上一次的值。 */
    dock.style.setProperty('--dock-w', dock.offsetWidth + 'px');
    dock.classList.add('is-in');
    setTimeout(function () { dock.classList.remove('is-in'); }, 900);
  })();
  var open = null;

  function slide(i) {
    dock.style.setProperty('--slot', i);
    ripple();
  }
  /* 波纹跟着滑块走：这一格刚被点过，它就该是唯一在动的那个。
     不做成常驻，是因为整条胶囊一直在闪就等于没有指示 ——
     读者反而看不出哪一格是刚动过的。

     重播要靠"摘类 → 强制重排 → 挂类"：CSS 动画在类已存在时不会重来，
     少了这两行，连点第二格时波纹根本不动，看起来像没响应。 */
  function ripple() {
    var thumb = dock && dock.querySelector('.dock-thumb');
    if (!thumb) { return; }
    thumb.classList.remove('is-live');
    void thumb.offsetWidth;
    thumb.classList.add('is-live');
  }
  function hum(ms) {
    // 震动是加分项：不支持的设备上什么都不发生，功能不依赖它。
    if (navigator.vibrate) { try { navigator.vibrate(ms); } catch (e) {} }
  }
  function prefetch(href) {
    // 动画期间把目标文档先拉进 HTTP 缓存，命中时跳转几乎是即时的。
    // 服务端若给了 no-store，这里只是白做一次，不会更慢 —— 导航本来也要发这个请求。
    try { fetch(href, { credentials: 'same-origin' }).catch(function () {}); } catch (e) {}
  }
  var HOLD = 380;   // 与 .rail 的 transform 过渡时长一致
  var holdTimer = 0;
  function clearHold() {
    if (holdTimer) { clearTimeout(holdTimer); holdTimer = 0; }
    document.body.classList.remove('drawer-hold-hot', 'drawer-hold-live');
  }
  function setDrawer(name) {
    // 收起时保持"刚才那一条"的卡片几何与单面板内容，直到滑出动画结束。
    //
    // 热门与活跃只有几行内容，打开时是一张小卡片；但 drawer-hot 类一移除，
    // .rail 立刻退回 height:100dvh —— 板子先膨胀到整屏再滑走，
    // 用户看到的就是"内容只有一点点，收起却是一整个屏幕"。
    var prev = open;
    clearHold();
    if (!name && (prev === 'hot' || prev === 'live')) {
      document.body.classList.add('drawer-hold-' + prev);
      holdTimer = setTimeout(function () {
        holdTimer = 0;
        document.body.classList.remove('drawer-hold-hot', 'drawer-hold-live');
      }, HOLD);
    }
    for (var k in DRAWERS) {
      if (Object.prototype.hasOwnProperty.call(DRAWERS, k)) { document.body.classList.remove(DRAWERS[k]); }
    }
    if (name) {
      document.body.classList.add(DRAWERS[name]);
      if (scrim) { scrim.hidden = false; }
    } else if (scrim) {
      scrim.hidden = true;
    }
    open = name;
    // 告诉问答与图谱：我开了。它们各自收自己，不需要知道我是谁。
    if (name) {
      try { document.dispatchEvent(new CustomEvent('fns:panel-open', { detail: { who: 'drawer' } })); } catch (e) {}
    }
    ['hot', 'nav', 'live'].forEach(function (k) {
      var b = document.getElementById('dock-' + k);
      if (b) { b.setAttribute('aria-expanded', name === k ? 'true' : 'false'); }
    });
  }
  function toggle(name) {
    if (open === name) { setDrawer(null); return; }
    if (open) {
      // 先收一条再开另一条 —— 这就是「自回归」。
      setDrawer(null);
      setTimeout(function () { setDrawer(name); }, SLIDE);
    } else {
      setDrawer(name);
    }
  }
  // 穿越的包装：脚本没就绪或用户要减少动效时，退回普通跳转。
  // 直接调 window.fnsWarp 会在它尚未定义时抛错，按钮就变成"点了没反应"——
  // 这正是这次要修的问题，不能再用一个可能不存在的函数去修它。
  function warp(url) {
    if (!url) { return; }
    if (typeof window.fnsWarp === 'function') { window.fnsWarp(url); return; }
    location.href = url;
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

  bind('dock-hot', 1, function () { toggle('hot'); });
  bind('dock-nav', 0, function () { toggle('nav'); });
  bind('dock-live', 3, function () { toggle('live'); });
  // 问道的点击由 ask 脚本上的 data-ask-open 接管，这里只管滑块与震动。
  bind('dock-ask', 4, null);

  // ── 漫游：从已渲染的卡片里随机挑一篇 ────────────────────────────────
  function walk() {
    var cards = document.querySelectorAll('.list .item');
    if (!cards.length) { return false; }
    var href = cards[Math.floor(Math.random() * cards.length)].getAttribute('href');
    if (!href) { return false; }
    // 首页已在 /blog，列表就在眼前，直接挑一张卡片跳，不必绕服务端。
    // 仍然走穿越，让「漫游」与跨页跳转是同一种体验。
    warp(href);
    return true;
  }
  bind('dock-walk', 2, walk);

  // 别人打开了，我收自己。
  document.addEventListener('fns:panel-open', function (ev) {
    var who = ev && ev.detail ? ev.detail.who : '';
    if (who === 'drawer') { return; }
    if (open) { setDrawer(null); }
  });
  if (scrim) { scrim.addEventListener('click', function () { setDrawer(null); }); }
  var panel = document.querySelector('.layout-home .aside');
  if (panel) {
    panel.addEventListener('click', function (e) {
      var a = e.target.closest ? e.target.closest('a') : null;
      if (a) { setDrawer(null); }
    });
  }
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') { setDrawer(null); }
  });
  // 文章页的「漫游」跳到 /blog#walk：到这儿立刻随机一篇，
  // 两个页面的漫游因此是同一条路，不需要服务端多一个接口。
  if (location.hash === '#walk') { walk(); }
  // 拉宽到桌面时抽屉形态消失，状态要一起清掉。
  if (window.matchMedia) {
    var mq = window.matchMedia('(min-width:861px)');
    var reset = function () { if (mq.matches) { setDrawer(null); } };
    if (mq.addEventListener) { mq.addEventListener('change', reset); } else if (mq.addListener) { mq.addListener(reset); }
  }
})();
</script>
{{if .ReactionJS}}<script>{{.ReactionJS}}</script>{{end}}
{{if .WarpJS}}<script>{{.WarpJS}}</script>{{end}}
{{/* ── 粒子场：暂时停用（2026-10-28）───────────────────────────────────────
     停用原因不是性能，而是观感：有人反映这些漂浮的小点像苍蝇。
     代码、样式与配置项全部保留，只把下面两行注释掉 —— 想换一种形态时从原处改，
     而不是从头再写一遍。blogParticlesJS 与 .fns-particles 都还在原位置。
     ─────────────────────────────────────────────────────────────────────── */}}
{{/*
{{if .ParticlesCfg}}<script type="application/json" id="fns-particles-cfg">{{.ParticlesCfg}}</script>{{end}}
{{if .ParticlesJS}}<script>{{.ParticlesJS}}</script>{{end}}
*/}}
<script>
/* 目录滚动高亮 —— 渐进增强：脚本不执行时目录链接仍可点击跳转。

   这里原有一段「图谱拖拽平移 + 滚轮缩放」的代码，它对本页容器里的 svg 施加 style.transform
   并自行接管 wheel 与 mousedown。它与 d3-zoom 抢同一组手势，而且 draw() 直接改 style.transform，
   会把 d3 的缩放抹掉 —— 读者拖动或滚动时，图看起来"卡住"或跳回原状。
   图谱的平移与缩放现在由 d3-zoom 独家负责（见 graph-engine）；这段代码已删除。 */
(function () {
  var links = document.querySelectorAll('.toc a[href^="#"]');
  if (links.length) {
    var pairs = [];
    Array.prototype.forEach.call(links, function (a) {
      var el = document.getElementById(a.getAttribute('href').slice(1));
      if (el) pairs.push({ a: a, el: el });
    });
    var spy = function () {
      var y = window.scrollY + 130, cur = null;
      for (var i = 0; i < pairs.length; i++) {
        if (pairs[i].el.getBoundingClientRect().top + window.scrollY <= y) cur = pairs[i].a;
      }
      Array.prototype.forEach.call(links, function (a) { a.classList.remove('on'); });
      if (cur) cur.classList.add('on');
    };
    window.addEventListener('scroll', spy, { passive: true });
    spy();
  }
})();
</script>
{{if .AskEnabled}}
<div id="ask-root"></div>
<script src="/blog/ask.js?v={{assetVer}}" defer></script>
<script src="/blog/sheet.js?v={{assetVer}}" defer></script>
{{end}}
<script src="/blog/feed.js?v={{assetVer}}" defer></script>
<script src="/blog/lightbox.js?v={{assetVer}}" defer></script>
</body>
</html>
{{define "nav"}}` + blogNavPartial + `{{end}}` + blogNavTree + blogTopNav))

var blogPostTmpl = template.Must(template.New("blog-post").Funcs(blogAssetFuncs).Parse(blogFooterLegal + `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
{{if not .Indexable}}<meta name="robots" content="noindex, nofollow">{{end}}
<meta name="theme-color" content="#eae9e3">
<meta name="description" content="{{.Description}}">
{{if .Canonical}}<link rel="canonical" href="{{.Canonical}}">{{end}}
{{if and .Indexable .FeedURL}}<link rel="alternate" type="application/rss+xml" title="{{.SiteName}}" href="{{.FeedURL}}">{{end}}
<meta property="og:title" content="{{.PostTitle}}">
<meta property="og:site_name" content="{{.SiteName}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:type" content="article">
{{if .Canonical}}<meta property="og:url" content="{{.Canonical}}">{{end}}
<meta name="twitter:card" content="summary">
<meta name="twitter:title" content="{{.PostTitle}}">
<meta name="twitter:description" content="{{.Description}}">
{{if .JSONLD}}<script type="application/ld+json">{{.JSONLD}}</script>{{end}}
{{if .HasMath}}<!-- Formulas are marked up but NOT typeset by the server: there is no production-grade
     TeX renderer in Go, and the only paths to one are shipping a browser engine or a ~1 MB
     JavaScript library. The LaTeX source is in the page and the stylesheet keeps it readable,
     so nothing is lost.

     To enable typesetting, drop a MathJax bundle at /static/mathjax/tex-mml-chtml.js and
     uncomment the single line below. That is deliberately a deployment decision rather than a
     default: the service must stay self-contained and must not reference assets it does not ship.

     <script defer src="/static/mathjax/tex-mml-chtml.js"></script>
-->{{end}}
<style>{{.CSS}}</style>
</head>
<body class="is-post">
<div class="backdrop"></div>
<div class="shell">
  <header class="topbar"><div class="topbar-inner">
    <div class="brand">
      <a class="brand-mark" href="/blog" title="返回首页"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/></svg></a>
      <div style="min-width:0">
        <a class="brand-name" href="/blog">{{.SiteName}}</a>
        <div class="brand-sub">笔记</div>
      </div>
    </div>
    <div class="topbar-spacer"></div>
    {{template "topnav" .}}
    <nav class="crumb"><a href="/blog">首页</a><span>/</span><b>{{.PostTitle}}</b></nav>
  </div></header>

  <div class="layout layout-post">
  <div class="fns-scrim" id="fns-scrim" hidden></div>
    {{template "nav" .}}
    <main class="content">
      <article class="card post">
        <h1 class="pt">{{.PostTitle}}</h1>
        <div class="post-meta">
          {{range .PostMeta}}<span>{{.}}</span>{{end}}
        </div>
        <div class="post-body">{{.Body}}</div>

        <!-- Interaction bar.
             Server-rendered, so the counts are in the page before any script runs and a reader
             without JavaScript still sees real numbers. The script only adds the burst, the
             count-up and the click behaviour.

             互动条。
             服务端渲染，因此计数在任何脚本运行之前就在页面里，没有 JavaScript 的读者看到的也是真实数字。
             脚本只增加粒子喷发、数字滚动与点击行为。 -->
        <div class="react" id="fns-react">
          <button type="button" class="react-btn" data-act="like" aria-pressed="false">
            <span class="react-ico" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M7 22V12l4.5-9a2.5 2.5 0 0 1 3.5 3l-1.2 4H19a2 2 0 0 1 2 2.4l-1.4 6A2 2 0 0 1 17.6 22Z"/><path d="M2 22V12h5"/></svg></span>
            <span class="react-n" data-n="like">{{.Like}}</span>
          </button>
          <button type="button" class="react-btn" data-act="dislike" aria-pressed="false">
            <span class="react-ico" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M17 2v10l-4.5 9a2.5 2.5 0 0 1-3.5-3l1.2-4H5a2 2 0 0 1-2-2.4l1.4-6A2 2 0 0 1 6.4 2Z"/><path d="M22 2v10h-5"/></svg></span>
            <span class="react-n" data-n="dislike">{{.Dislike}}</span>
          </button>
          <button type="button" class="react-btn react-share" data-act="share">
            <span class="react-ico" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8"/><path d="M16 6l-4-4-4 4"/><path d="M12 2v13"/></svg></span>
            <span data-label>分享</span>
          </button>
          <span class="react-spacer"></span>
          <span class="react-visits">
            <span class="react-visits-n" data-countup="{{.Visits}}">{{.Visits}}</span>
            <span class="react-visits-t">次访问</span>
          </span>
        </div>
        <script type="application/json" id="fns-react-cfg">{{.ReactionCfg}}</script>
      </article>

      {{if .ShowLocalGraph}}
      <!-- Graph dock, collapsed by default.

           The open/close behaviour is a deliberate copy of the ask panel's, down to the
           timings: the pill fades out with scale(.9) translateY(6px) while the panel comes in
           from translateY(16px) scale(.975) over .44s cubic-bezier(.22,1,.36,1) with its
           transform-origin at the bottom-right, and the two cross over rather than swapping
           instantly. Consistency between two floating panels on the same page matters more than
           inventing a second motion vocabulary for the same gesture.

           图谱停靠区，默认收起。

           开合行为是对提问面板的刻意复制，连时序一起：胶囊以 scale(.9) translateY(6px) 淡出，
           同时面板以 translateY(16px) scale(.975) 在 .44s cubic-bezier(.22,1,.36,1) 内进场，
           变换原点在右下角，两者交叠而不是瞬间互换。
           同一页面上两个浮动面板保持一致，比给同一个手势发明第二套动效语言更重要。 -->
      <div id="graph-root">
        <button type="button" class="graph-fab" id="graph-fab" aria-expanded="false" aria-controls="graph-panel">
          <span class="graph-fab-ico" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><circle cx="5" cy="6" r="2"/><circle cx="19" cy="6" r="2"/><circle cx="5" cy="18" r="2"/><circle cx="19" cy="18" r="2"/><path d="M7 7.5 10 10"/><path d="m14 10 3-2.5"/><path d="M7 16.5 10 14"/><path d="m14 14 3 2.5"/></svg>
          </span>
          <span class="graph-fab-txt">知识图谱</span>
        </button>

        <div class="graph-backdrop" id="graph-backdrop" hidden></div>

        <section class="graph-panel" id="graph-panel" role="dialog" aria-modal="false" aria-label="知识图谱" hidden>
          <div class="graphcard-tools">
            <label class="graphcard-depth" for="gc-depth">
              <span class="graphcard-depth-t">深度 <b id="gc-depth-v">{{.LocalGraphDepth}}</b></span>
              <input id="gc-depth" class="graphcard-slider" type="range" min="1" max="3" step="1"
                     value="{{.LocalGraphDepth}}" data-slug="{{.ActiveSlug}}"
                     aria-label="图谱深度" disabled>
            </label>
            <div class="graphcard-acts">
              <button type="button" class="graphcard-icon" id="gc-full" aria-label="全屏查看" title="全屏查看">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
              </button>
              <button type="button" class="graphcard-icon" id="gc-expand" aria-label="放大" title="放大">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6"/><path d="M9 21H3v-6"/><path d="M21 3l-7 7"/><path d="M3 21l7-7"/></svg>
              </button>
              <button type="button" class="graphcard-icon" id="gc-collapse" aria-label="收起" title="收起">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>
              </button>
            </div>
          </div>
          <div class="graph-wrap" id="graph-canvas">{{.LocalGraphSVG}}</div>
          <p class="graph-note">{{.LocalGraphNote}}</p>
        </section>

        <!-- Fullscreen layer. Its scale-in, backdrop blur and focus handling come from the
             existing .graphlayer rules, which were verified in an earlier stage; only the
             contents change — a second, larger d3 mount rather than a server SVG swap.

             全屏浮层。它的缩放进入、背景模糊与焦点处理沿用既有的 .graphlayer 规则
             （已在先前阶段验证过）；变的只是内容——第二次更大的 d3 挂载，而不是替换服务端 SVG。 -->
        <div class="graphlayer" id="graph-layer" hidden>
          <div class="graphlayer-panel">
            <div class="graphlayer-head">
              <span class="graphlayer-title" id="graph-layer-title">知识图谱</span>
              <button type="button" class="graphlayer-close" id="graph-layer-close" aria-label="关闭">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
              </button>
            </div>
            <div class="graphlayer-body" id="graph-layer-body"></div>
          </div>
        </div>
      </div>
      {{end}}
    </main>
{{if .ShowLocalGraph}}{{range $d := .GraphDepths}}<script type="application/json" id="graph-data-{{$d}}">{{index $.GraphDataByDepth $d}}</script>
{{end}}<script type="module" id="graph-engine">{{.GraphEngineJS}}</script>{{end}}
{{if .PreviewJSON}}<script type="application/json" id="link-previews">{{.PreviewJSON}}</script>
<script>{{.PreviewJS}}</script>{{end}}
{{if .ReactionJS}}<script>{{.ReactionJS}}</script>{{end}}
{{if .WarpJS}}<script>{{.WarpJS}}</script>{{end}}

    <aside class="rail">
      <div class="panel sticky toc">
        <div class="nav-title">本页目录</div>
        {{if .TOC}}
          {{range .TOC}}<a href="#{{.ID}}" class="{{if eq .Level 3}}lv3{{end}}"><span class="t">{{.Text}}</span></a>{{end}}
        {{else}}
          <div class="toc-empty">本文无小节标题</div>
        {{end}}
      </div>
    </aside>
  </div>

  <footer class="footer">
    <div>由 <a href="https://github.com/haierkeys/fast-note-sync-service" target="_blank" rel="noopener noreferrer">Fast Note Sync</a> 提供支持</div>
    {{template "footer-legal" .}}
  </footer>
</div>
<script>
/* 目录滚动高亮 —— 渐进增强：脚本不执行时目录链接仍可点击跳转。

   这里原有一段「图谱拖拽平移 + 滚轮缩放」的代码，它对本页容器里的 svg 施加 style.transform
   并自行接管 wheel 与 mousedown。它与 d3-zoom 抢同一组手势，而且 draw() 直接改 style.transform，
   会把 d3 的缩放抹掉 —— 读者拖动或滚动时，图看起来"卡住"或跳回原状。
   图谱的平移与缩放现在由 d3-zoom 独家负责（见 graph-engine）；这段代码已删除。 */
(function () {
  var links = document.querySelectorAll('.toc a[href^="#"]');
  if (links.length) {
    var pairs = [];
    Array.prototype.forEach.call(links, function (a) {
      var el = document.getElementById(a.getAttribute('href').slice(1));
      if (el) pairs.push({ a: a, el: el });
    });
    var spy = function () {
      var y = window.scrollY + 130, cur = null;
      for (var i = 0; i < pairs.length; i++) {
        if (pairs[i].el.getBoundingClientRect().top + window.scrollY <= y) cur = pairs[i].a;
      }
      Array.prototype.forEach.call(links, function (a) { a.classList.remove('on'); });
      if (cur) cur.classList.add('on');
    };
    window.addEventListener('scroll', spy, { passive: true });
    spy();
  }
})();
</script>
{{if .AskEnabled}}
<div id="ask-root"></div>
<script src="/blog/ask.js?v={{assetVer}}" defer></script>
<script src="/blog/sheet.js?v={{assetVer}}" defer></script>
{{end}}
<script src="/blog/feed.js?v={{assetVer}}" defer></script>
<script src="/blog/lightbox.js?v={{assetVer}}" defer></script>

  <!-- Mobile edge tabs and their drawers.
       Only rendered on reading pages: the home page has no in-page outline, and its side columns
       are content rather than something a reader wants to move out of the way.

       The two side columns are turned into drawers by CSS alone — no content is duplicated here.
       Copying the tree into a second container would mean two places to keep in sync, and the
       server already rendered exactly one.

       移动端边缘书签与抽屉。
       只在阅读页渲染:首页没有本页目录,而它的侧栏是内容本身,
       不是读者会想暂时让开的东西。

       两个侧栏由 CSS 直接变成抽屉,这里没有复制任何内容。
       把目录树复制进第二个容器意味着两处要同步,而服务端已经恰好渲染了一份。 -->

  <!-- 阅读页的五点胶囊：山（全局目录）· 图谱 · 首页 · 漫游 · 问道。
       左书签撤掉了 —— 全局目录的入口在这里的「山」上，同一个页面不需要两个通往目录的入口。
       右书签保留，它管的是本页大纲，与全局目录不是一件事。 -->
  <div class="dock" id="fns-dock" role="group" aria-label="阅读页入口" data-slot="0">
    <span class="dock-thumb" aria-hidden="true"></span>
    <button type="button" class="dock-b" data-i="0" id="dock-nav" aria-label="目录" aria-expanded="false">
      <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M0 1.75A.75.75 0 0 1 .75 1h4.253c1.227 0 2.317.59 3 1.501A3.743 3.743 0 0 1 11.006 1h4.245a.75.75 0 0 1 .75.75v10.5a.75.75 0 0 1-.75.75h-4.507a2.25 2.25 0 0 0-1.591.659l-.622.621a.75.75 0 0 1-1.06 0l-.622-.621A2.25 2.25 0 0 0 5.258 13H.75a.75.75 0 0 1-.75-.75Zm7.251 10.324.004-5.073-.002-2.253A2.25 2.25 0 0 0 5.003 2.5H1.5v9h3.757a3.75 3.75 0 0 1 1.994.574ZM8.755 4.75l-.004 7.322a3.752 3.752 0 0 1 1.992-.572H14.5v-9h-3.495a2.25 2.25 0 0 0-2.25 2.25Z"/></svg>
    </button>
    <button type="button" class="dock-b" data-i="1" id="dock-graph" aria-label="知识图谱">
      <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="m7.775 3.275 1.25-1.25a3.5 3.5 0 1 1 4.95 4.95l-2.5 2.5a3.5 3.5 0 0 1-4.95 0 .751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018 1.998 1.998 0 0 0 2.83 0l2.5-2.5a2.002 2.002 0 0 0-2.83-2.83l-1.25 1.25a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042Zm-4.69 9.64a1.998 1.998 0 0 0 2.83 0l1.25-1.25a.751.751 0 0 1 1.042.018.751.751 0 0 1 .018 1.042l-1.25 1.25a3.5 3.5 0 1 1-4.95-4.95l2.5-2.5a3.5 3.5 0 0 1 4.95 0 .751.751 0 0 1-.018 1.042.751.751 0 0 1-1.042.018 1.998 1.998 0 0 0-2.83 0l-2.5 2.5a1.998 1.998 0 0 0 0 2.83Z"/></svg>
    </button>
    <button type="button" class="dock-b" data-i="2" id="dock-home" aria-label="回首页">
      <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M6.906.664a1.749 1.749 0 0 1 2.187 0l5.25 4.2c.415.332.657.835.657 1.367v7.019A1.75 1.75 0 0 1 13.25 15h-3.5a.75.75 0 0 1-.75-.75V9H7v5.25a.75.75 0 0 1-.75.75h-3.5A1.75 1.75 0 0 1 1 13.25V6.23c0-.531.242-1.034.657-1.366l5.25-4.2Zm1.25 1.171a.25.25 0 0 0-.312 0l-5.25 4.2a.25.25 0 0 0-.094.196v7.019c0 .138.112.25.25.25H5.5V8.25a.75.75 0 0 1 .75-.75h3.5a.75.75 0 0 1 .75.75v5.25h2.75a.25.25 0 0 0 .25-.25V6.23a.25.25 0 0 0-.094-.195Z"/></svg>
    </button>
    <button type="button" class="dock-b" data-i="3" id="dock-toc" aria-label="本页大纲" aria-expanded="false">
      <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M5.75 2.5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5ZM2 14a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1-6a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM2 4a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>
    </button>
    <button type="button" class="dock-b" data-i="4" id="dock-ask" aria-label="问道 · 向知识库提问" aria-expanded="false" data-ask-open>
      <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M1.75 1h8.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 10.25 10H7.061l-2.574 2.573A1.458 1.458 0 0 1 2 11.543V10h-.25A1.75 1.75 0 0 1 0 8.25v-5.5C0 1.784.784 1 1.75 1ZM1.5 2.75v5.5c0 .138.112.25.25.25h1a.75.75 0 0 1 .75.75v2.19l2.72-2.72a.749.749 0 0 1 .53-.22h3.5a.25.25 0 0 0 .25-.25v-5.5a.25.25 0 0 0-.25-.25h-8.5a.25.25 0 0 0-.25.25Zm13 2a.25.25 0 0 0-.25-.25h-.5a.75.75 0 0 1 0-1.5h.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 14.25 12H14v1.543a1.458 1.458 0 0 1-2.487 1.03L9.22 12.28a.749.749 0 0 1 .326-1.275.749.749 0 0 1 .734.215l2.22 2.22v-2.19a.75.75 0 0 1 .75-.75h1a.25.25 0 0 0 .25-.25Z"/></svg>
    </button>
  </div>

  <script>
  /* 阅读页的入口接线。

     一、两条抽屉：山 → 全局目录（左），右书签 → 本页大纲（右）。
     互斥不靠两个必须互相协调的布尔值，而靠「任何时候 body 上只有一个 drawer class」。

     二、图谱与问道不在这里实现逻辑。图谱直接点原来的那个按钮，
     问了走 ask 脚本的 data-ask-open 约定 —— 只是把入口搬到胶囊上，不复制功能。

     三、首页与漫游是跳转：等滑块走完（300ms）再走，期间先 prefetch 目标文档。
     漫游交给首页处理，因为只有首页手上有 146 张卡可以随机挑。 */
  (function () {
    var b = document.body;
    var dock = document.getElementById('fns-dock');
    var tabToc = document.getElementById('dock-toc');
    var scrim = document.getElementById('fns-scrim');
    if (!dock || !tabToc || !scrim) return;

    var NAV = 'drawer-nav', TOC = 'drawer-toc';
    // 跨页进来时让胶囊自己重组一次：读者因此知道这一条的顺序与上一页不同。
    // 只在站内导航时播；首屏进入网站不播，那会显得页面一直在加载。
    (function () {
      var ref = document.referrer || '';
      var sameSite = ref && ref.indexOf(location.protocol + '//' + location.host) === 0;
      /* 必须比 pathname 全等，不能用子串包含：
       从 /blog/about 回 /blog 时，referrer 里同样含 "/blog"，
       子串判断会把这次真实跨页当成"同页刷新"，整段动画不播。
       （/blog 是 /blog/about 的前缀，任何这种前缀关系都会误判。） */
    var samePage = false;
    try { samePage = new URL(ref).pathname === location.pathname; } catch (e) { samePage = false; }
      var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      if (!sameSite || samePage || reduce) { return; }
      /* 先量出自然宽度，再交给动画当终点。
         量的时候 --dock-w 还没写，.dock 的宽度是 auto，量到的就是真实值；
         先写再量会把宽度锁成上一次的值。 */
      dock.style.setProperty('--dock-w', dock.offsetWidth + 'px');
      dock.classList.add('is-in');
      setTimeout(function () { dock.classList.remove('is-in'); }, 900);
    })();
    var SLIDE = 300;

    function set(mode) {
      b.classList.remove(NAV, TOC);
      if (mode) b.classList.add(mode);
      scrim.hidden = !mode;
      tabToc.setAttribute('aria-expanded', mode === TOC ? 'true' : 'false');
      var navBtn = document.getElementById('dock-nav');
      if (navBtn) navBtn.setAttribute('aria-expanded', mode === NAV ? 'true' : 'false');
      // 抽屉打开时锁住身后页面。否则在遮罩上滑动会滚动下面的正文，
      // 看起来像抽屉在滑，而不是页面停在原地。
      b.style.overflow = mode ? 'hidden' : '';
      // 告诉问答与图谱：我开了。
      if (mode) {
        try { document.dispatchEvent(new CustomEvent('fns:panel-open', { detail: { who: 'drawer' } })); } catch (e) {}
      }
    }
    function slide(i) { dock.style.setProperty('--slot', i); ripple(); }
    function ripple() {
      var thumb = dock && dock.querySelector('.dock-thumb');
      if (thumb) { thumb.classList.add('is-live'); }
    }
    function hum(ms) { if (navigator.vibrate) { try { navigator.vibrate(ms); } catch (e) {} } }
    function prefetch(href) {
      try { fetch(href, { credentials: 'same-origin' }).catch(function () {}); } catch (e) {}
    }
    // 穿越的包装：脚本没就绪或用户要减少动效时，退回普通跳转。
  // 直接调 window.fnsWarp 会在它尚未定义时抛错，按钮就变成"点了没反应"——
  // 这正是这次要修的问题，不能再用一个可能不存在的函数去修它。
  function warp(url) {
    if (!url) { return; }
    if (typeof window.fnsWarp === 'function') { window.fnsWarp(url); return; }
    location.href = url;
  }
  function bind(id, slot, action) {
      var el = document.getElementById(id);
      if (!el) return;
      el.addEventListener('click', function (e) {
        slide(slot); hum(10);
        if (action) action(e);
      });
    }

    bind('dock-nav', 0, function () { set(b.classList.contains(NAV) ? null : NAV); });
    bind('dock-graph', 1, function () {
      var fab = document.getElementById('graph-fab');
      if (fab) fab.click();
    });
    bind('dock-home', 2, function () {
      warp('/blog');
    });

    // 问道由 ask 脚本的 data-ask-open 接管，这里只管滑块与震动。
    bind('dock-ask', 4, null);

    tabToc.addEventListener('click', function () {
      set(b.classList.contains(TOC) ? null : TOC);
    });
  // 别人打开了，我收自己。
  document.addEventListener('fns:panel-open', function (ev) {
    var who = ev && ev.detail ? ev.detail.who : '';
    if (who === 'drawer') { return; }
    if (b.classList.contains('drawer-toc') || b.classList.contains('drawer-nav')) { set(null); }
  });
    scrim.addEventListener('click', function () { set(null); });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') set(null);
    });
    // 点抽屉里的链接后关闭它。否则目标内容落在遮罩后面，
    // 读者得去关一个他并没有打开的东西。
    var links = document.querySelectorAll('.aside a[href], .rail a[href]');
    Array.prototype.forEach.call(links, function (a) {
      a.addEventListener('click', function () { setTimeout(function () { set(null); }, 60); });
    });
  })();
  </script>
</body>
</html>
{{define "nav"}}` + blogNavPartial + `{{end}}` + blogNavTree + blogTopNav))

// blogPageView is the data for a page note (about).
//
// It deliberately carries no graph, no link previews and no local-neighbourhood card. A page is a
// destination rather than a stopping point on the way to other notes, so none of those have
// anything to say on it.
//
// blogPageView 是页面笔记（关于）的数据。
//
// 它刻意不携带图谱、链接预览与局部邻域卡片。页面本身是终点，而不是通往其它笔记的中转站，
// 因此这些在它上面都无事可做。
type blogPageView struct {
	Title      string
	SiteName   string
	Indexable  bool
	CSS        template.CSS
	PageTitle  string
	PageAlias  string
	// Pages are the page-note entry points shown in the top bar, with this page's own alias
	// already removed by the caller. Same meaning as blogPostView.Pages.
	//
	// Pages 是顶栏里的页面笔记入口，调用方已剔除本页自己的别名。含义同 blogPostView.Pages。
	Pages []dto.BlogNavPageDTO
	// HasShelf shows the 资料 entry. See blogHomeView.HasShelf.
	// HasShelf 决定是否显示「资料」入口。见 blogHomeView.HasShelf。
	HasShelf   bool
	Avatar     string
	UpdatedAt  string
	Path       string
	Body       template.HTML
	TOC        []tocEntry
	AskEnabled bool

	// Canonical and FeedURL are absolute; JSONLD is serialised by the caller.
	// Canonical 与 FeedURL 是绝对地址；JSONLD 由调用方序列化。
	Canonical   string
	Description string
	FeedURL     string
	JSONLD      template.JS

	// HasMath mirrors the article page: MathJax is only referenced where a formula exists.
	// HasMath 与文章页一致：只有存在公式时才引入 MathJax。
	HasMath bool
	// WarpJS is the arriving half of the warp transition. See blogPostView.WarpJS.
	// WarpJS 是穿越过渡中「抵达」的那一半。见 blogPostView.WarpJS。
	WarpJS template.JS
	// 备案号：空表示不渲染那一行。见 config.BlogConfig.ICP。
	ICP     string
	MPS     string
	MPSCode string
}

// blogPageTmpl renders a page note at its alias route.
//
// It shares the skeleton with the home and article templates — same backdrop, topbar, footer and
// design tokens — so an alias reads as part of the site. Two things differ, both deliberate:
//
//  1. It does not invoke the "nav" template, so the folder tree is absent and the middle column
//     gets the width instead. The tree is an entry point to articles; a page is where a reader
//     arrives to read one thing.
//  2. The middle column carries .layout-page, which is what the print stylesheet keys on to drop
//     the furniture and lay the page out as a document.
//
// blogPageTmpl 在别名路由上渲染页面笔记。
//
// 它与首页、文章页模板共用骨架——同样的点阵背景、顶栏、页脚与设计令牌——因此别名页读起来
// 仍是本站的一部分。有两处不同，都是有意的：
//
//  1. 不调用 "nav" 模板，因此没有目录树，宽度归中栏。目录树是通往文章的入口，
//     而页面是读者专程来读一件事的地方。
//  2. 中栏带 .layout-page，打印样式表以此为据去掉页面家具、把内容按文档排版。
var blogPageTmpl = template.Must(template.New("blog-page").Funcs(blogAssetFuncs).Parse(blogFooterLegal + `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.PageTitle}} · {{.SiteName}}</title>
{{if not .Indexable}}<meta name="robots" content="noindex, nofollow">{{end}}
<meta name="theme-color" content="#eae9e3">
<meta name="description" content="{{.Description}}">
{{if .Canonical}}<link rel="canonical" href="{{.Canonical}}">{{end}}
{{if and .Indexable .FeedURL}}<link rel="alternate" type="application/rss+xml" title="{{.SiteName}}" href="{{.FeedURL}}">{{end}}
<meta property="og:title" content="{{.PageTitle}}">
<meta property="og:site_name" content="{{.SiteName}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:type" content="profile">
{{if .Canonical}}<meta property="og:url" content="{{.Canonical}}">{{end}}
<meta name="twitter:card" content="summary">
<meta name="twitter:title" content="{{.PageTitle}}">
<meta name="twitter:description" content="{{.Description}}">
{{if .JSONLD}}<script type="application/ld+json">{{.JSONLD}}</script>{{end}}
{{if .HasMath}}<!-- Same deployment note as the article page: formulas are marked up but not
     typeset by the server. To enable typesetting, drop a MathJax bundle at
     /static/mathjax/tex-mml-chtml.js and uncomment the line below.

     与文章页相同的部署说明：公式已被标记，但不由服务端排版。
     若要启用排版，把 MathJax 放到 /static/mathjax/tex-mml-chtml.js 并取消下面一行的注释。

     <script defer src="/static/mathjax/tex-mml-chtml.js"></script>
-->{{end}}
<style>{{.CSS}}</style>
</head>
<body class="is-page">
<div class="backdrop"></div>
<div class="shell">
  <header class="topbar"><div class="topbar-inner">
    <div class="brand">
      <a class="brand-mark" href="/blog" title="返回首页"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/></svg></a>
      <div style="min-width:0">
        <a class="brand-name" href="/blog">{{.SiteName}}</a>
        <div class="brand-sub">笔记</div>
      </div>
    </div>
    <div class="topbar-spacer"></div>
    {{template "topnav" .}}
    <nav class="crumb"><a href="/blog">首页</a><span>/</span><b>{{.PageTitle}}</b></nav>
  </div></header>

  <div class="layout layout-page{{if not .TOC}} layout-page-single{{end}}">
  <div class="fns-scrim" id="fns-scrim" hidden></div>
    <main class="content">
      <article class="card page">
        {{if .Avatar}}<div class="page-avatar"><img src="{{.Avatar}}" alt="" loading="lazy" decoding="async"></div>{{end}}
        <h1 class="pt">{{.PageTitle}}</h1>
        {{if .UpdatedAt}}<div class="page-meta"><span>最后更新 {{.UpdatedAt}}</span></div>{{end}}
        <div class="page-body post-body">{{.Body}}</div>
      </article>
    </main>

{{if .TOC}}
    <aside class="rail">
      <div class="panel sticky toc">
        <div class="nav-title">本页目录</div>
        {{range .TOC}}<a href="#{{.ID}}" class="{{if eq .Level 3}}lv3{{end}}"><span class="t">{{.Text}}</span></a>{{end}}
      </div>
    </aside>
{{end}}
  </div>

  <footer class="footer">
    <div>由 <a href="https://github.com/haierkeys/fast-note-sync-service" target="_blank" rel="noopener noreferrer">Fast Note Sync</a> 提供支持</div>
    {{template "footer-legal" .}}
  </footer>
</div>
{{if .WarpJS}}<script>{{.WarpJS}}</script>{{end}}
{{if .TOC}}<script>
/* 目录滚动高亮 —— 与文章页同一段渐进增强；脚本不执行时目录链接仍可点击跳转。 */
(function () {
  var links = document.querySelectorAll('.toc a[href^="#"]');
  if (!links.length) return;
  var pairs = [];
  Array.prototype.forEach.call(links, function (a) {
    var el = document.getElementById(a.getAttribute('href').slice(1));
    if (el) pairs.push({ a: a, el: el });
  });
  var spy = function () {
    var y = window.scrollY + 130, cur = null;
    for (var i = 0; i < pairs.length; i++) {
      if (pairs[i].el.getBoundingClientRect().top + window.scrollY <= y) cur = pairs[i].a;
    }
    Array.prototype.forEach.call(links, function (a) { a.classList.remove('on'); });
    if (cur) cur.classList.add('on');
  };
  window.addEventListener('scroll', spy, { passive: true });
  spy();
})();
</script>{{end}}
{{if .AskEnabled}}
<div id="ask-root"></div>
<script src="/blog/ask.js?v={{assetVer}}" defer></script>
<script src="/blog/sheet.js?v={{assetVer}}" defer></script>
{{end}}
<script src="/blog/feed.js?v={{assetVer}}" defer></script>
<script src="/blog/lightbox.js?v={{assetVer}}" defer></script>

  <!-- Mobile edge tabs and their drawers.
       Only rendered on reading pages: the home page has no in-page outline, and its side columns
       are content rather than something a reader wants to move out of the way.

       The two side columns are turned into drawers by CSS alone — no content is duplicated here.
       Copying the tree into a second container would mean two places to keep in sync, and the
       server already rendered exactly one.

       移动端边缘书签与抽屉。
       只在阅读页渲染:首页没有本页目录,而它的侧栏是内容本身,
       不是读者会想暂时让开的东西。

       两个侧栏由 CSS 直接变成抽屉,这里没有复制任何内容。
       把目录树复制进第二个容器意味着两处要同步,而服务端已经恰好渲染了一份。 -->
  <!-- 页面笔记是终点，不是中转站：全局目录在这里本来就不存在，
       所以只留一个本页大纲的书签。 -->

  <!-- 页面笔记的三点胶囊：首页 · 漫游 · 问道。
       比阅读页少两格，因为这两种东西在这一页上不存在 —— 全局目录没有（页面是终点），
       局部图谱也没有（本页不携带图谱）。少两格比给对方一个点了没反应的键诚实。 -->
  <div class="dock" id="fns-dock" role="group" aria-label="页面入口" data-slot="0">
    <span class="dock-thumb" aria-hidden="true"></span>
    <button type="button" class="dock-b" data-i="0" id="dock-toc" aria-label="本页大纲" aria-expanded="false">
      <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M5.75 2.5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5Zm0 5h8.5a.75.75 0 0 1 0 1.5h-8.5a.75.75 0 0 1 0-1.5ZM2 14a1 1 0 1 1 0-2 1 1 0 0 1 0 2Zm1-6a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM2 4a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>
    </button>
    <button type="button" class="dock-b" data-i="1" id="dock-home" aria-label="回首页">
      <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M6.906.664a1.749 1.749 0 0 1 2.187 0l5.25 4.2c.415.332.657.835.657 1.367v7.019A1.75 1.75 0 0 1 13.25 15h-3.5a.75.75 0 0 1-.75-.75V9H7v5.25a.75.75 0 0 1-.75.75h-3.5A1.75 1.75 0 0 1 1 13.25V6.23c0-.531.242-1.034.657-1.366l5.25-4.2Zm1.25 1.171a.25.25 0 0 0-.312 0l-5.25 4.2a.25.25 0 0 0-.094.196v7.019c0 .138.112.25.25.25H5.5V8.25a.75.75 0 0 1 .75-.75h3.5a.75.75 0 0 1 .75.75v5.25h2.75a.25.25 0 0 0 .25-.25V6.23a.25.25 0 0 0-.094-.195Z"/></svg>
    </button>
    <button type="button" class="dock-b" data-i="2" id="dock-ask" aria-label="问道 · 向知识库提问" aria-expanded="false" data-ask-open>
      <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M1.75 1h8.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 10.25 10H7.061l-2.574 2.573A1.458 1.458 0 0 1 2 11.543V10h-.25A1.75 1.75 0 0 1 0 8.25v-5.5C0 1.784.784 1 1.75 1ZM1.5 2.75v5.5c0 .138.112.25.25.25h1a.75.75 0 0 1 .75.75v2.19l2.72-2.72a.749.749 0 0 1 .53-.22h3.5a.25.25 0 0 0 .25-.25v-5.5a.25.25 0 0 0-.25-.25h-8.5a.25.25 0 0 0-.25.25Zm13 2a.25.25 0 0 0-.25-.25h-.5a.75.75 0 0 1 0-1.5h.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 14.25 12H14v1.543a1.458 1.458 0 0 1-2.487 1.03L9.22 12.28a.749.749 0 0 1 .326-1.275.749.749 0 0 1 .734.215l2.22 2.22v-2.19a.75.75 0 0 1 .75-.75h1a.25.25 0 0 0 .25-.25Z"/></svg>
    </button>
  </div>
  <script>
  (function () {
    var b = document.body;
    var tabToc = document.getElementById('dock-toc');
    var scrim = document.getElementById('fns-scrim');
    if (!tabToc || !scrim) return;

    var TOC = 'drawer-toc';
    var dock = document.getElementById('fns-dock');
    var SLIDE = 300;
    // 跨页进来时让胶囊自己重组一次：读者因此知道这一条的顺序与上一页不同。
    // 只在站内导航时播；首屏进入网站不播，那会显得页面一直在加载。
    (function () {
      var ref = document.referrer || '';
      var sameSite = ref && ref.indexOf(location.protocol + '//' + location.host) === 0;
      /* 必须比 pathname 全等，不能用子串包含：
       从 /blog/about 回 /blog 时，referrer 里同样含 "/blog"，
       子串判断会把这次真实跨页当成"同页刷新"，整段动画不播。
       （/blog 是 /blog/about 的前缀，任何这种前缀关系都会误判。） */
    var samePage = false;
    try { samePage = new URL(ref).pathname === location.pathname; } catch (e) { samePage = false; }
      var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      if (!sameSite || samePage || reduce) { return; }
      /* 先量出自然宽度，再交给动画当终点。
         量的时候 --dock-w 还没写，.dock 的宽度是 auto，量到的就是真实值；
         先写再量会把宽度锁成上一次的值。 */
      dock.style.setProperty('--dock-w', dock.offsetWidth + 'px');
      dock.classList.add('is-in');
      setTimeout(function () { dock.classList.remove('is-in'); }, 900);
    })();

    function slide(i) { if (dock) { dock.style.setProperty('--slot', i); ripple(); } }
    function ripple() {
      var thumb = dock && dock.querySelector('.dock-thumb');
      if (thumb) { thumb.classList.add('is-live'); }
    }
    function hum(ms) { if (navigator.vibrate) { try { navigator.vibrate(ms); } catch (e) {} } }
    function prefetch(href) {
      try { fetch(href, { credentials: 'same-origin' }).catch(function () {}); } catch (e) {}
    }
    // 穿越的包装：脚本没就绪或用户要减少动效时，退回普通跳转。
    // 直接调 window.fnsWarp 会在它尚未定义时抛错，按钮就变成"点了没反应"——
    // 这正是这次要修的问题，不能再用一个可能不存在的函数去修它。
    function warp(url) {
      if (!url) { return; }
      if (typeof window.fnsWarp === 'function') { window.fnsWarp(url); return; }
      location.href = url;
    }
    function bind(id, slot, fn) {
      var el = document.getElementById(id);
      if (!el) return;
      el.addEventListener('click', function (e) {
        slide(slot); hum(10);
        if (fn) fn(e);
      });
    }
    bind('dock-home', 1, function () {
      warp('/blog');
    });

    // 问道由 ask 脚本的 data-ask-open 接管，这里只管滑块与震动。
    bind('dock-ask', 2, null);

    function set(open) {
      b.classList.remove(TOC);
      if (open) b.classList.add(TOC);

      scrim.hidden = !open;
      tabToc.setAttribute('aria-expanded', open ? 'true' : 'false');

      // Lock the page behind the drawer. Without it a swipe on the scrim scrolls the article
      // underneath, which reads as the drawer sliding rather than the page staying put.
      //
      // 抽屉打开时锁住身后的页面。否则在遮罩上滑动会滚动下面的正文,
      // 看起来像抽屉在滑,而不是页面停在原地。
      b.style.overflow = open ? 'hidden' : '';
      // 告诉问答与图谱：我开了。
      if (open) {
        try { document.dispatchEvent(new CustomEvent('fns:panel-open', { detail: { who: 'drawer' } })); } catch (e) {}
      }
    }

    tabToc.addEventListener('click', function () {
      set(!b.classList.contains(TOC));
    });
  // 别人打开了，我收自己。
  document.addEventListener('fns:panel-open', function (ev) {
    var who = ev && ev.detail ? ev.detail.who : '';
    if (who === 'drawer') { return; }
    if (b.classList.contains('drawer-toc') || b.classList.contains('drawer-nav')) { set(null); }
  });
    scrim.addEventListener('click', function () { set(null); });

    // Escape closes. A drawer that covers the page and ignores the keyboard is a trap.
    // Escape 关闭。一个盖住页面又不响应键盘的抽屉是一个陷阱。
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') set(null);
    });

    // Following a link inside a drawer closes it. Without this the target lands behind the scrim
    // and the reader has to dismiss something they never opened.
    //
    // 点击抽屉里的链接后关闭它。否则目标内容落在遮罩后面,
    // 读者得去关一个他并没有打开的东西。
    var links = document.querySelectorAll('.aside a[href], .rail a[href]');
    Array.prototype.forEach.call(links, function (a) {
      a.addEventListener('click', function () { setTimeout(function () { set(null); }, 60); });
    });
  })();
  </script>
</body>
</html>` + blogTopNav))

// blogErrorView is the data for the shared error page (404 / not published / blog disabled
// / render failure). It deliberately carries no navigation tree: an error page must not cost
// another full query pass to render.
//
// blogErrorView 是公共错误页（404 / 未公开 / 博客关闭 / 渲染失败）的数据。
// 它刻意不携带导航树：错误页不应为了渲染再付一次完整查询的代价。
type blogErrorView struct {
	Title    string
	SiteName string
	CSS      template.CSS
	Code     int
	Heading  string
	Message  string
	Hints    []string
	// 备案号：空表示不渲染那一行。见 config.BlogConfig.ICP。
	ICP     string
	MPS     string
	MPSCode string
}

// blogErrorTmpl renders the shared error page. Same skeleton as the home and article
// templates (backdrop / topbar / footer, same design tokens) so a dead link lands on a page
// that still looks like the site rather than on a bare line of text.
//
// blogErrorTmpl 渲染公共错误页。骨架与首页、文章页模板相同（点阵背景 / 顶栏 / 页脚，
// 同一套设计令牌），使失效链接落在一个仍然像本站的页面上，而不是一行裸文本。
var blogErrorTmpl = template.Must(template.New("blog-error").Funcs(blogAssetFuncs).Parse(blogFooterLegal + `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<meta name="robots" content="noindex, nofollow">
<meta name="theme-color" content="#eae9e3">
<style>{{.CSS}}</style>
</head>
<body class="is-error">
<div class="backdrop"></div>
<div class="shell">
  <header class="topbar"><div class="topbar-inner">
    <div class="brand">
      <a class="brand-mark" href="/blog" title="返回首页"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/></svg></a>
      <div style="min-width:0">
        <a class="brand-name" href="/blog">{{.SiteName}}</a>
        <div class="brand-sub">笔记</div>
      </div>
    </div>
    <div class="topbar-spacer"></div>
    <nav class="crumb"><a href="/blog">首页</a></nav>
  </div></header>

  <main class="errwrap">
    <section class="card errcard">
      <div class="errcode">{{.Code}}</div>
      <h1 class="errtitle">{{.Heading}}</h1>
      <p class="errmsg">{{.Message}}</p>
      {{if .Hints}}
      <ul class="errhints">{{range .Hints}}<li>{{.}}</li>{{end}}</ul>
      {{end}}
      <div class="erracts"><a class="errbtn" href="/blog">返回首页</a></div>
    </section>
  </main>

  <footer class="footer">
    <div>由 <a href="https://github.com/haierkeys/fast-note-sync-service" target="_blank" rel="noopener noreferrer">Fast Note Sync</a> 提供支持</div>
    {{template "footer-legal" .}}
  </footer>
</div>
</body>
</html>`))

// ── SVG helpers ──────────────────────────────────────────────────────

// graphLabelMinDegree is the degree above which a node earns a visible caption.
//
// Two is the point where a note stops being a leaf: it links out and is linked to, so it is part
// of the graph's skeleton rather than an endpoint. A leaf is still clickable and still named on
// hover — it simply does not get to cover its neighbours with text.
//
// graphLabelMinDegree 是节点获得可见标签所需的度数下限。
//
// 2 是笔记不再是叶子节点的分界：它既有出链也有入链，因此属于图的骨架而不是端点。
// 叶子节点仍可点击、悬停时仍显示名称——它只是不再用文字盖住邻居。
const graphLabelMinDegree = 3

// renderGraphSVG emits the knowledge graph as a self-contained SVG.
//
// Doing the layout on the server means the graph is visible with JavaScript disabled, is part
// of the HTML for indexing, and costs the reader no extra download.
//
// renderGraphSVG 把知识图谱输出为自包含的 SVG。
// 在服务端完成布局，意味着禁用 JavaScript 时图谱依然可见、内容进入 HTML 可被索引，
// 同时读者不需要下载任何额外脚本。
func renderGraphSVG(g *dto.BlogGraphDTO) (template.HTML, string) {
	if g == nil || len(g.Nodes) == 0 {
		if g != nil && g.PublishedOnly {
			return template.HTML(`<p style="padding:2rem 0;text-align:center;color:var(--muted-foreground);font-size:.8125rem">还没有已公开的笔记，图谱会在你分享第一篇笔记后出现。</p>`), ""
		}
		return template.HTML(`<p style="padding:2rem 0;text-align:center;color:var(--muted-foreground);font-size:.8125rem">暂无数据</p>`), ""
	}

	pos := make(map[string]*dto.BlogGraphNode, len(g.Nodes))
	for _, n := range g.Nodes {
		pos[n.ID] = n
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %d %d" role="img" aria-label="知识图谱，%d 个节点、%d 条链接">`,
		g.Width, g.Height, len(g.Nodes), len(g.Edges))

	// One arrowhead per direction, defined once. `marker-end` on the line is what turns a bare
	// line into a statement about which note points at which; the marker is small and shares the
	// line's colour so it reads as an arrowhead rather than as a second element.
	//
	// 每个方向一个箭头，只定义一次。线上的 marker-end 把一条裸线变成「哪篇指向哪篇」的陈述；
	// 箭头很小且与线同色，因此看起来是线头而不是第二个元素。
	b.WriteString(`<defs>` +
		`<marker id="garrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">` +
		`<path class="g-arrow" d="M0,1 L9,5 L0,9 z"/>` +
		`</marker></defs>`)

	// Edges first so nodes paint on top.
	// 先画边，节点才能压在上面。
	b.WriteString(`<g class="g-edges">`)
	for _, e := range g.Edges {
		a, ok1 := pos[e.Source]
		c, ok2 := pos[e.Target]
		if !ok1 || !ok2 {
			continue
		}
		// The line is shortened by the target's radius so the arrowhead lands on the circle's
		// edge rather than under it.
		//
		// 线按目标半径缩短，使箭头落在圆周上而不是被圆盖住。
		x1, y1 := a.X, a.Y
		x2, y2 := c.X, c.Y
		if dx, dy := x2-x1, y2-y1; dx != 0 || dy != 0 {
			d := math.Sqrt(dx*dx + dy*dy)
			if d > c.R+2 {
				shrink := (d - (c.R + 2)) / d
				x2 = x1 + dx*shrink
				y2 = y1 + dy*shrink
			}
		}
		fmt.Fprintf(&b, `<line class="g-edge" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" marker-end="url(#garrow)"/>`,
			x1, y1, x2, y2)
	}
	b.WriteString(`</g><g class="g-nodes">`)

	for i, n := range g.Nodes {
		cls := "g-node"
		if n.Lit {
			cls += " lit"
		}
		if n.Focus {
			cls += " focus"
		}
		// Stagger the entrance so the graph assembles instead of snapping in at once.
		// 错开入场时机，让图谱「生长」出来而不是整体闪现。
		b.WriteString(`<g class="` + cls + `" style="animation-delay:` + fmt.Sprint(i*14) + `ms">`)
		if n.URL != "" {
			b.WriteString(`<a href="` + template.HTMLEscapeString(n.URL) + `">`)
		}
		fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="%.2f"><title>%s</title></circle>`,
			n.X, n.Y, n.R, template.HTMLEscapeString(n.Label))
		// Labels are drawn only for nodes that carry the graph's structure.
		//
		// Measured: with 112 nodes, drawing every caption produced 34 overlapping pairs — at that
		// density most labels are unreadable, and the useful signal (where the hubs are) is lost
		// in the noise. Every node still carries a <title>, so hovering any of them names it.
		//
		// 只为承载图结构的节点画标签。
		//
		// 实测：112 个节点时全部标注产生 34 对重叠——在这个密度下多数标签已不可读，
		// 而真正有用的信息（枢纽在哪里）被噪声淹没。每个节点仍带 <title>，
		// 因此悬停任意节点都能看到它的名称。
		if n.Label != "" && n.Deg >= graphLabelMinDegree {
			// 标签放在圆形正下方；过长标题会被截断，避免压到相邻节点。
			label := n.Label
			if r := []rune(label); len(r) > 10 {
				label = string(r[:10]) + "…"
			}
			// A caption below a node near the bottom edge would fall outside the viewBox and be
			// clipped — measured: two labels at the bottom of the 112-node graph were cut in half.
			// Flipping the caption above the node keeps it inside the canvas, which is what every
			// charting library does at the boundary.
			//
			// 靠近底边的节点，其下方标签会落到 viewBox 之外被裁掉——实测：112 节点图底部有两个
			// 标签被切掉一半。把标签翻到节点上方即可留在画布内，这也是所有图表库在边界处的做法。
			labelY := n.Y + n.R + 12
			if labelY > float64(g.Height)-6 {
				labelY = n.Y - n.R - 6
			}
			fmt.Fprintf(&b, `<text x="%.2f" y="%.2f" text-anchor="middle">%s</text>`,
				n.X, labelY, template.HTMLEscapeString(label))
		}
		if n.URL != "" {
			b.WriteString(`</a>`)
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(`</g></svg>`)

	// The wording has to describe what the graph actually does. It used to say "the larger the
	// circle, the more links it has", which stopped being true the moment node size became fixed
	// — and distance, not size, took over the job of expressing how closely a note is related.
	// A caption that describes a previous design is worse than no caption.
	//
	// 文案必须描述图谱实际做的事。它曾写着「圆越大代表连接越多」，
	// 而在节点尺寸固定的那一刻这句话就不成立了——表达「关联有多近」的职责改由距离承担。
	// 一句描述旧设计的说明比没有说明更糟。
	note := fmt.Sprintf("共 %d 个节点、%d 条链接。离中心越近的代表关联越紧密，深色节点是当前笔记，可以点击进入。",
		len(g.Nodes), len(g.Edges))
	if !g.PublishedOnly {
		note += "浅色节点是尚未公开的笔记，仅用于体现连接关系，不显示标题。"
	}
	return template.HTML(b.String()), note
}

// ── Post-processors ──────────────────────────────────────────────────

// headingRegex captures <h2|h3 id="...">text</h2|h3> for the outline.
// Go's regexp (RE2) has no backreferences, so the closing tag is matched with a character
// class rather than `\1`.
// headingRegex 捕获用于生成大纲的 <h2|h3 id="...">文本</h2|h3>。
// Go 的正则（RE2）不支持反向引用，因此闭合标签用字符类匹配，而不是 `\1`。
var (
	headingRegex = regexp.MustCompile(`(?is)<h([23])\b[^>]*\bid="([^"]+)"[^>]*>(.*?)</h[23]>`)
	tagStripRe   = regexp.MustCompile(`<[^>]+>`)
	anchorRegex  = regexp.MustCompile(`(?is)<a\s+([^>]*?)href="(https?:)?//([^"]+)"([^>]*)>`)
)

// extractTOC builds the right-hand outline from rendered HTML.
// extractTOC 从渲染后的 HTML 中提取右侧大纲。
// externalizeLinks makes every off-site link open in a new tab with a safe rel attribute.
// Internal /blog/ links are left untouched so in-site navigation stays in the same tab.
//
// externalizeLinks 让所有站外链接在新标签页打开，并补上安全的 rel 属性；
// 站内的 /blog/ 链接保持原样，站内跳转仍在本标签进行。
func externalizeLinks(html string) string {
	return anchorRegex.ReplaceAllStringFunc(html, func(m string) string {
		if strings.Contains(m, `target=`) {
			return m
		}
		// Insert the attributes right after the opening tag name.
		// 在开标签名之后插入属性。
		return strings.Replace(m, "<a ", `<a target="_blank" rel="noopener noreferrer" `, 1)
	})
}

// imgTagRegex captures a whole <img …> tag together with its src.
// imgTagRegex 捕获完整的 <img …> 标签及其 src。
var imgTagRegex = regexp.MustCompile(`(?is)<img\b[^>]*?>`)

// sanitizeImages degrades images the server cannot serve.
//
// Notes pasted from WPS/Word routinely embed absolute local paths such as
// /Users/xxx/Library/Containers/com.kingsoft.wpsoffice.mac/…/temp.png. Those files never
// reach the server, so the browser renders a broken-image icon followed by the raw path —
// which is exactly what the tables were showing. A short inline notice is honest and far
// tidier.
//
// Only server-servable sources are kept: site-absolute asset routes (/api/, /static/,
// /assets/), absolute http(s) URLs, and data: URIs. Everything else is replaced.
//
// sanitizeImages 降级「服务端无法提供」的图片。
//
// 从 WPS/Word 粘贴的笔记常内嵌绝对本地路径（如
// /Users/xxx/Library/Containers/com.kingsoft.wpsoffice.mac/…/temp.png）。这些文件从未上传到
// 服务端，浏览器只会渲染出破图图标加一串原始路径——表格里显示的就是这个。
// 换一句简短的行内提示既诚实又整洁得多。
//
// 只保留服务端可提供的来源：站内资源路径（/api/、/static/、/assets/）、绝对 http(s) 地址
// 以及 data: URI，其余一律替换。
func sanitizeImages(html string) string {
	return imgTagRegex.ReplaceAllStringFunc(html, func(tag string) string {
		m := imgSrcAttrRegex.FindStringSubmatch(tag)
		if len(m) < 2 {
			return tag // 没有 src，交给浏览器处理 / no src: leave as-is
		}
		src := strings.TrimSpace(m[1])
		ok := strings.HasPrefix(src, "/api/") ||
			strings.HasPrefix(src, "/static/") ||
			strings.HasPrefix(src, "/assets/") ||
			strings.HasPrefix(src, "http://") ||
			strings.HasPrefix(src, "https://") ||
			strings.HasPrefix(src, "data:")
		if ok {
			return tag
		}
		return `<span class="img-missing">图片未同步到服务端（原路径 ` +
			template.HTMLEscapeString(src) + `）</span>`
	})
}

// imgSrcAttrRegex extracts the src attribute value from an img tag.
// imgSrcAttrRegex 从 img 标签中取出 src 属性值。
var imgSrcAttrRegex = regexp.MustCompile(`(?is)\bsrc\s*=\s*"([^"]*)"`)
