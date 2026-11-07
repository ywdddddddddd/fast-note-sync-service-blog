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
	// The theme script is a template function rather than a field on every view: there are six
	// separate <head> blocks across four files, and adding a field would have meant touching six
	// view structs — every one of which could be forgotten, leaving that page stuck in light mode
	// after the reader had chosen dark.
	//
	// 主题脚本做成模板函数而不是每个 view 的字段：四个文件里有六个独立 <head>，
	// 加字段意味着改六个 view 结构体——而其中任何一个都可能被漏掉，
	// 那一页就会在读者选了暗色之后依然停在亮色。
	"themejs": func() template.JS {
		// Both halves travel together: the loader exists only to drive the three-state button that
		// the first half owns. Splitting them would put a 164 KB downloader on pages that never
		// render a button.
		//
		// 两半一起走：加载器的唯一用途是驱动前半段拥有的那个三态按钮。
		// 把它们拆开，会让从不渲染按钮的页面也背上 164 KB 的下载器。
		return template.JS(blogThemeScript + blogThemeLottieJS)
	},
	// inkjs is the loading overlay, and it belongs in <head> rather than at the end of <body>.
	//
	// It is a separate function from themejs because the two have different audiences: themejs runs
	// on the error page too (it needs the theme, it has no top bar), while the overlay is deliberately
	// absent there — a loading screen in front of a 404 is pure delay. Only the five reading templates
	// call this one.
	//
	// Why <head> at all: the overlay has to be on screen at the first painted frame. Injected at the
	// end of <body> it arrives after the browser has already painted the bare page, which is what the
	// owner reported as "no loading animation on entry" — the animation existed, it simply started
	// a second and a half too late to be seen behind a white page.
	//
	// inkjs 是加载遮罩，它属于 <head> 而不是 <body> 末尾。
	//
	// 它与 themejs 分成两个函数，因为两者的受众不同：themejs 在错误页上也要跑
	// （它需要主题，但没有顶栏），而遮罩在那里是刻意缺席的 —— 在 404 前面放加载画面是纯粹的拖延。
	// 只有五个阅读模板会调用这一个。
	//
	// 为什么要放进 <head>：遮罩必须在第一个被绘制的帧上就在屏幕上。
	// 注入到 <body> 末尾时，它抵达的时候浏览器已经把光秃秃的页面画出来了，
	// 而那正是站主报的「入站时没有加载动画」—— 动画本身存在，只是晚了一秒半才出现，
	// 完全被一张白页挡住。
	"inkjs": func() template.JS {
		return template.JS(blogWarpJS())
	},
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
	// PostCover is the article hero, filled only for absolute cover references. See
	// dto.BlogPostDTO.Cover for why a local attachment is not resolved here.
	//
	// PostCover 是文章 hero，仅对绝对的封面引用填充。
	// 本地附件为何不在这里解析，见 dto.BlogPostDTO.Cover。
	PostCover  *dto.BlogCoverDTO
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
	// Quiz carries the counts and leaderboard strip for the chapter-end exercise. It never
	// carries questions: those travel only over POST, so they cannot end up in the page
	// source or in a crawl.
	//
	// Quiz 携带章末练习的计数与榜单条，绝不含题目：题目只经 POST 传输，因此不可能进入
	// 页面源码或被爬取。
	Quiz *dto.BlogQuizDTO
	// QuizJS is the exercise UI. It is a separate script from ReactionJS because the two
	// have nothing to share and the exercise half is large.
	//
	// QuizJS 是练习界面。它与 ReactionJS 分开，因为两者没有可共享的东西，而练习这一半不小。
	QuizJS  template.JS
	QuizCSS template.CSS
	// ShareData is the JSON blob the share cards are drawn from, and ShareJS is the drawing
	// code. They are separate fields because the data has to be serialised by the caller
	// (the template must not re-encode it), while the script is one constant string.
	//
	// ShareData 是画分享卡用的 JSON，ShareJS 是绘制代码。两者分开：数据必须由调用方
	// 序列化（模板不该再做一次编码），而脚本是一个常量字符串。
	ShareData template.JS
	ShareJS   template.JS
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

// blogHomeTmpl is the card page. Its markup lives in blog_templates_redesign.go so the three
// redesigned pages share one set of partials instead of three copies of the same head.
//
// blogHomeTmpl 是名片页。它的标记放在 blog_templates_redesign.go 里，
// 使三个改版页面共用一套片段，而不是三份相同的 head。
var blogHomeTmpl = blogCardTmpl

var blogPostTmpl = template.Must(template.New("blog-post").Funcs(blogAssetFuncs).Parse(blogFooterLegal + `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<link rel="icon" type="image/svg+xml" href="/blog/icon.svg">
<link rel="apple-touch-icon" href="/blog/icon.svg">
<script>{{themejs}}</script>
<script>{{inkjs}}</script>
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
      <a class="brand-mark" href="/blog" title="返回首页"><img src="/blog/icon-mark.svg" alt="" width="34" height="34"></a>
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
        <a class="back-pill" href="/blog/all"><svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M7.78 12.53a.75.75 0 0 1-1.06 0L2.47 8.28a.75.75 0 0 1 0-1.06l4.25-4.25a.751.751 0 0 1 1.042.018.751.751 0 0 1 .018 1.042L4.81 7h7.44a.75.75 0 0 1 0 1.5H4.81l2.97 2.97a.75.75 0 0 1 0 1.06Z"/></svg>返回全部笔记</a>
        {{/* 正文页不渲染封面。封面只用于分享卡与列表卡片：文章页顶部那张大图是
                整页最重的视觉元素，而它出现在读者已经点进来之后——此时读者要的是正文，
                一张横幅只会把标题推下去。 */}}
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

        {{if .Quiz}}
        <!-- Chapter-end exercise.
             Only the shell is server-rendered. The questions themselves arrive from
             POST /blog/quiz/pick, so neither the page source nor any GET response ever
             contains a question — a crawler that does not run scripts and does not POST
             cannot see the bank.

             章末练习。
             只有外壳是服务端渲染的。题目本身来自 POST /blog/quiz/pick，因此页面源码与任何 GET
             响应都不含题干——不执行脚本、不发 POST 的爬虫看不到题库。 -->
        <section class="quiz" id="fns-quiz" data-slug="{{.ActiveSlug}}" data-note="{{.Quiz.NotePath}}">
          <div class="quiz-head">
            <span class="quiz-title">章末练习</span>
            <span class="quiz-sub">客观题 {{.Quiz.Objective}} 道 · 背诵对照 {{.Quiz.Subjective}} 条</span>
          </div>
          {{if .Quiz.Top}}
          <ol class="quiz-board">
            {{range .Quiz.Top}}<li{{if .Self}} class="self"{{end}}><span class="r">{{.Rank}}</span><span class="n">{{.Name}}</span><span class="s">{{.Correct}}/{{.Total}}</span></li>{{end}}
          </ol>
          {{end}}
          <div class="quiz-actions">
            <button type="button" class="quiz-btn primary" id="quiz-start">开始测验</button>
            <button type="button" class="quiz-btn" id="quiz-recall">背诵对照</button>
          </div>
          <div class="quiz-stage" id="quiz-stage" hidden></div>
        </section>
        <style>{{.QuizCSS}}</style>
        <script>{{.QuizJS}}</script>
        {{end}}

        {{if .ShareData}}<script id=fns-share-data type=application/json>{{.ShareData}}</script>{{end}}
        <script>{{.ShareJS}}</script>
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
    <div>由 <a href="https://obsidian.md" target="_blank" rel="noopener noreferrer">Obsidian</a> 提供内容支持</div>
    {{template "footer-legal" .}}
  </footer>
</div>
<button type="button" class="read-prog" id="fns-prog" aria-label="阅读进度，点击回到顶部">
  <svg viewBox="0 0 42 42" aria-hidden="true">
    <circle class="rp-track" cx="21" cy="21" r="18"></circle>
    <circle class="rp-bar" cx="21" cy="21" r="18"></circle>
  </svg>
  <svg class="rp-ico" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M8 3.5 3.5 8l.9.9L7.5 5.8V13h1V5.8l3.1 3.1.9-.9Z"/></svg>
</button>
<script>
/* 目录滚动高亮 —— 渐进增强：脚本不执行时目录链接仍可点击跳转。

   这里原有一段「图谱拖拽平移 + 滚轮缩放」的代码，它对本页容器里的 svg 施加 style.transform
   并自行接管 wheel 与 mousedown。它与 d3-zoom 抢同一组手势，而且 draw() 直接改 style.transform，
   会把 d3 的缩放抹掉 —— 读者拖动或滚动时，图看起来"卡住"或跳回原状。
   图谱的平移与缩放现在由 d3-zoom 独家负责（见 graph-engine）；这段代码已删除。 */
(function () {
  var links = document.querySelectorAll('.toc a[href^="#"]');
  var pairs = [];
  Array.prototype.forEach.call(links, function (a) {
    var el = document.getElementById(a.getAttribute('href').slice(1));
    if (el) pairs.push({ a: a, el: el });
  });

  // ── 顶栏两态 + 阅读进度环 ─────────────────────────────────────────
  //
  // 一个滚动监听同时喂三件事：顶栏进入「浮动」态、圆环的进度、目录的已读标记。
  // 分成三个监听会让同一帧里读三次布局，而且三者的判断阈值必须一致才有意义——
  // 圆环走到一半、目录却还停在前言，那种不同步比少一个功能更让人分心。
  //
  // One scroll listener feeds three things: the bar's floating state, the ring's progress and the
  // outline's read marks. Three listeners would read layout three times per frame, and more to the
  // point the thresholds have to agree — a ring at halfway with the outline still on the intro is
  // worse than having no ring at all.
  var bar = document.querySelector('.topbar');
  var prog = document.getElementById('fns-prog');
  var barEl = prog ? prog.querySelector('.rp-bar') : null;
  var R = 18, C = 2 * Math.PI * R;
  if (barEl) {
    barEl.style.strokeDasharray = C.toFixed(2);
    barEl.style.strokeDashoffset = C.toFixed(2);
  }

  function onScroll() {
    var y = window.scrollY || document.documentElement.scrollTop || 0;
    if (bar) { bar.classList.toggle('is-scrolled', y > 8); }

    if (prog) {
      // The ring only appears once the reader has committed to reading; showing it on the first
      // screenful puts a control over the title for no reason.
      prog.classList.toggle('is-on', y > 240);
      var span = document.documentElement.scrollHeight - window.innerHeight;
      var p = span > 0 ? Math.min(1, Math.max(0, y / span)) : 0;
      if (barEl) { barEl.style.strokeDashoffset = (C * (1 - p)).toFixed(2); }
    }

    var probe = y + 130, cur = -1;
    for (var i = 0; i < pairs.length; i++) {
      if (pairs[i].el.getBoundingClientRect().top + y <= probe) { cur = i; }
    }
    for (var j = 0; j < pairs.length; j++) {
      pairs[j].a.classList.toggle('on', j === cur);
      pairs[j].a.classList.toggle('readed', j < cur);
    }
  }

  window.addEventListener('scroll', onScroll, { passive: true });
  window.addEventListener('resize', onScroll, { passive: true });
  onScroll();

  if (prog) {
    prog.addEventListener('click', function () {
      var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      window.scrollTo({ top: 0, behavior: reduce ? 'auto' : 'smooth' });
    });
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
<script src="/blog/code.js?v={{assetVer}}" defer></script>

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
  <div class="dock dock-collapsible" id="fns-dock" role="group" aria-label="阅读页入口" data-slot="0">
    <button type="button" class="dock-orb" id="fns-orb" aria-label="展开页面入口" aria-expanded="false">
      <span class="taiji-lottie" data-lottie="code">
        <svg class="taiji" viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="12" cy="12" r="9.1" fill="none" stroke="currentColor" stroke-width="1.35"/>
        <path d="M12 2.9a4.55 4.55 0 0 1 0 9.1 4.55 4.55 0 0 0 0 9.1" fill="currentColor"/>
        <circle cx="12" cy="7.45" r="1.3" fill="var(--card)"/>
        <circle cx="12" cy="16.55" r="1.3" fill="currentColor"/>
      </svg>
      </span>
    </button>
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
// Collapsed capsule, same behaviour as the card page: one round key until asked, and it closes
// again the moment a click lands outside it.
// 收起的胶囊，与名片页同一套行为：在读者要之前只是一个圆键，而任何落在它外面的点击都会让它收回。
(function () {
  var orb = document.getElementById('fns-orb');
  if (!dock || !orb) { return; }
  function setOpen(on) {
    dock.classList.toggle('is-open', on);
    orb.setAttribute('aria-expanded', on ? 'true' : 'false');
  }
  orb.addEventListener('click', function (ev) {
    ev.stopPropagation();
    var opening = !dock.classList.contains('is-open');
    // Expanding announces itself first: the orb plays once, then the dock opens. A second press
    // while it plays is dropped rather than queued.
    //
    // 展开先自我通报：球播一遍，然后胶囊打开。播放期间的第二次按压直接丢弃，而不是排队。
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
  document.addEventListener('click', function (ev) {
    if (dock.classList.contains('is-open') && !dock.contains(ev.target)) { setOpen(false); }
  });
  Array.prototype.forEach.call(dock.querySelectorAll('.dock-b'), function (b) {
    b.addEventListener('click', function () { setOpen(false); });
  });
})();
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

// blogFriendCardView is one card in the friends grid.
//
// Initial is precomputed because a Go template cannot take the first character of a string
// without a helper, and registering a helper would put it in every template's function map for a
// single page's benefit.
//
// blogFriendCardView 是友链网格里的一张卡片。
//
// Initial 预先算好，因为 Go 模板无法在不加辅助函数的情况下取字符串的首个字符，
// 而注册一个辅助函数，会让它仅为一个页面的用途出现在所有模板的函数表里。
type blogFriendCardView struct {
	dto.BlogFriendDTO
	Initial string
}

// blogFriendGroupView is one section of the friends grid.
//
// An empty Title is the ungrouped bucket: the template renders its entries but omits the heading.
// That is how a note with no `group` at all becomes a single flat grid — same markup, one less
// element — rather than needing a second rendering path that would drift from the grouped one.
//
// blogFriendGroupView 是友链网格里的一个区块。
//
// Title 为空表示「未分组」这一桶：模板照常渲染其中的条目，只是不渲染标题。
// 于是一篇完全没有 `group` 的笔记就是一张平铺网格——同样的标记，少一个元素——
// 而不需要第二条会与分组路径渐渐走样的渲染分支。
type blogFriendGroupView struct {
	Title   string
	Friends []blogFriendCardView
}

// blogFriendGroups groups entries by Group, preserving the order in which each group first
// appears. Ungrouped entries go first and keep their relative order.
//
// Group titles are compared after trimming, which parseBlogFriends already did. Two entries with
// the same title share one section; a title that differs only by surrounding spaces cannot happen
// here because the parser trims both.
//
// blogFriendGroups 按 Group 分组，并保持每个分组首次出现的顺序。未分组条目排在最前，
// 组内保持原有相对顺序。
//
// 分组标题比较的是 parseBlogFriends 已去空白的值。同一标题的两条共用一个区块；
// 只差首尾空格的标题在这里不会出现，因为解析器已经把两边都修剪过。
func blogFriendGroups(friends []dto.BlogFriendDTO) []blogFriendGroupView {
	if len(friends) == 0 {
		return nil
	}
	groups := make([]blogFriendGroupView, 0, 4)
	index := make(map[string]int, 4)
	ungrouped := make([]blogFriendCardView, 0, len(friends))
	for _, f := range friends {
		card := blogFriendCardView{BlogFriendDTO: f, Initial: blogFriendInitial(f.Name)}
		if f.Group == "" {
			ungrouped = append(ungrouped, card)
			continue
		}
		i, ok := index[f.Group]
		if !ok {
			groups = append(groups, blogFriendGroupView{Title: f.Group})
			i = len(groups) - 1
			index[f.Group] = i
		}
		groups[i].Friends = append(groups[i].Friends, card)
	}
	if len(ungrouped) == 0 && len(groups) > 0 {
		return groups
	}
	out := make([]blogFriendGroupView, 0, len(groups)+1)
	out = append(out, blogFriendGroupView{Friends: ungrouped})
	return append(out, groups...)
}

// blogFriendInitial returns the fallback letter shown when a friend has no avatar, or whose avatar
// fails to load. It is the first rune, not byte, so a Chinese name yields its first character
// rather than a broken fragment. An empty name falls back to a middle dot so the circle is never
// blank.
//
// blogFriendInitial 返回友链没有头像、或头像加载失败时显示的首字母占位。
// 取的是首个 rune 而不是字节，因此中文名得到的是第一个汉字，而不是一个碎掉的字节。
// 空名字退化为中点，使圆形色块不会是空白。
func blogFriendInitial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		return strings.ToUpper(string(r))
	}
	return "·"
}

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
	Title     string
	SiteName  string
	Indexable bool
	CSS       template.CSS
	PageTitle string
	PageAlias string
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

	// FriendGroups is the friends page card list, already grouped. Nil for every other alias, and
	// for a friends page whose frontmatter produced no valid entries; the template then renders
	// only the "copy site info" block.
	//
	// FriendGroups 是已分组的友链卡片列表。其它别名、以及 frontmatter 没有合法条目的友链页均为 nil；
	// 此时模板只渲染「复制本站信息」区块。
	FriendGroups []blogFriendGroupView
	// FriendsCopyText is the payload of the copy button. It is set only on the friends page, which
	// is also the only page the template renders that button on.
	//
	// FriendsCopyText 是复制按钮的内容。只有友链页会赋值，而模板也只在友链页渲染这个按钮。
	FriendsCopyText string

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
	// WaveJS is the dot-grid wave background. Only the card page and page notes carry it — see
	// blogWaveJS for why a list page does not.
	// WaveJS 是点阵波纹背景。只有名片页与页面笔记带它——列表页为何不带，见 blogWaveJS。
	WaveJS template.JS
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
<link rel="icon" type="image/svg+xml" href="/blog/icon.svg">
<link rel="apple-touch-icon" href="/blog/icon.svg">
<script>{{themejs}}</script>
<script>{{inkjs}}</script>
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
      <a class="brand-mark" href="/blog" title="返回首页"><img src="/blog/icon-mark.svg" alt="" width="34" height="34"></a>
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
        {{if eq .PageAlias "friends"}}
        <div class="friends">
          {{range .FriendGroups}}
          <section class="friends-group">
            {{if .Title}}<h2 class="friends-group-title">{{.Title}}</h2>{{end}}
            <div class="friends-grid">
              {{range .Friends}}
              <a class="friend-card" href="{{.URL}}" target="_blank" rel="noopener noreferrer nofollow" data-name="{{.Name}}" data-avatar="{{.Avatar}}">
                <span class="friend-avatar" aria-hidden="true"><span class="friend-initial">{{.Initial}}</span>{{if .Avatar}}<img src="{{.Avatar}}" alt="" loading="lazy" decoding="async" referrerpolicy="no-referrer" onerror="this.remove()">{{end}}</span>
                <span class="friend-main">
                  <span class="friend-name">{{.Name}}</span>
                  {{if .Desc}}<span class="friend-desc">{{.Desc}}</span>{{end}}
                </span>
              </a>
              {{end}}
            </div>
          </section>
          {{end}}
          <div class="friends-apply">
            <button type="button" class="friends-copy" data-copy="{{.FriendsCopyText}}">复制本站信息</button>
            <span class="friends-hint">复制本站信息，可添加友链</span>
          </div>
          <script>
          /* 「复制本站信息」：优先用 Clipboard API，被拒绝或不可用时退回 execCommand。
             两种都失败时按钮短暂显示「复制失败」——读者按了按钮，答案必须出现在同一个地方。
             这段脚本只服务于友链页，所以跟着那个区块一起渲染，而不是进全局脚本。 */
          (function () {
            var btn = document.querySelector('.friends-copy');
            if (!btn) return;
            var label = btn.textContent;
            var timer = null;
            function restore() {
              btn.textContent = label;
              btn.classList.remove('is-done');
            }
            function flash(text, cls) {
              btn.textContent = text;
              btn.classList.add(cls);
              if (timer) window.clearTimeout(timer);
              timer = window.setTimeout(restore, 1600);
            }
            function fallback(text) {
              var ta = document.createElement('textarea');
              ta.value = text;
              ta.setAttribute('readonly', '');
              ta.style.position = 'fixed';
              ta.style.left = '-9999px';
              document.body.appendChild(ta);
              ta.select();
              var ok = false;
              try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
              document.body.removeChild(ta);
              if (ok) flash('已复制', 'is-done'); else flash('复制失败', 'is-fail');
            }
            btn.addEventListener('click', function () {
              var text = btn.getAttribute('data-copy') || '';
              if (!text) return;
              if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(text).then(function () {
                  flash('已复制', 'is-done');
                }, function () {
                  fallback(text);
                });
              } else {
                fallback(text);
              }
            });
          })();
          </script>

          <div class="friends-form-wrap">
            <form class="friends-form" id="friends-apply" novalidate>
              <h2 class="friends-form-title">申请友链</h2>
              <div class="friends-form-grid">
                <label class="friends-field"><span class="friends-field-label">站名 <em>*</em></span><input type="text" name="name" maxlength="60" required autocomplete="organization"></label>
                <label class="friends-field"><span class="friends-field-label">链接 <em>*</em></span><input type="url" name="url" maxlength="300" required inputmode="url" autocomplete="url" placeholder="https://"></label>
                <label class="friends-field"><span class="friends-field-label">头像</span><input type="url" name="avatar" maxlength="300" inputmode="url" placeholder="https://"></label>
                <label class="friends-field"><span class="friends-field-label">描述</span><input type="text" name="desc" maxlength="120"></label>
                <label class="friends-field"><span class="friends-field-label">邮箱</span><input type="email" name="email" maxlength="120" autocomplete="email"></label>
                <label class="friends-field"><span class="friends-field-label">订阅</span><input type="url" name="feed" maxlength="300" inputmode="url" placeholder="https://"></label>
                <label class="friends-field friends-field-wide"><span class="friends-field-label">留言</span><textarea name="message" rows="3" maxlength="300"></textarea></label>
              </div>
              <div class="friends-form-foot">
                <button type="submit" class="friends-submit">提交申请</button>
                <span class="friends-form-msg" id="friends-apply-msg" role="status" aria-live="polite"></span>
              </div>
            </form>
            <p class="friends-form-done" id="friends-apply-done" hidden>已收到你的申请，通过后会出现在上面。</p>
          </div>

          <script>
          /* 友链申请：提交 JSON，成功就地把表单换成一句话；错误用服务端返回的中文消息。
             只服务于友链页，因此跟着区块一起渲染，与复制按钮同一风格。 */
          (function () {
            var form = document.getElementById('friends-apply');
            if (!form) return;
            var done = document.getElementById('friends-apply-done');
            var msg = document.getElementById('friends-apply-msg');
            var btn = form.querySelector('.friends-submit');
            var label = btn ? btn.textContent : '';
            var FIELDS = ['name', 'url', 'avatar', 'desc', 'email', 'feed', 'message'];

            function setMsg(text, bad) {
              if (!msg) return;
              msg.textContent = text || '';
              msg.classList.toggle('is-error', !!bad);
            }

            function payload() {
              var out = {};
              Array.prototype.forEach.call(FIELDS, function (key) {
                var el = form.elements[key];
                out[key] = el && el.value ? el.value.trim() : '';
              });
              return out;
            }

            function busy(on) {
              if (!btn) return;
              btn.disabled = !!on;
              btn.textContent = on ? '提交中…' : label;
            }

            form.addEventListener('submit', function (ev) {
              ev.preventDefault();
              setMsg('');
              busy(true);
              fetch('/blog/friends/apply', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload())
              }).then(function (res) {
                return res.text().then(function (text) {
                  var data = null;
                  try { data = text ? JSON.parse(text) : null; } catch (e) { data = null; }
                  return { ok: res.ok, data: data };
                });
              }).then(function (res) {
                busy(false);
                if (res.ok) {
                  form.hidden = true;
                  if (done) done.hidden = false;
                  return;
                }
                setMsg((res.data && res.data.error) || '提交失败，请稍后再试', true);
              }, function () {
                busy(false);
                setMsg('网络异常，请稍后再试', true);
              });
            });
          })();
          </script>
        </div>

        <!-- 遮罩运行时由脚本挂到 document.body：页面「穿越抵达」的缩放 transform 留在
             .shell 上，而 fixed 元素在带 transform 的祖先里会锚到该祖先（只剩文章列宽）。
             先挂 body 再取消 hidden，量到的才是视口尺寸。 -->
        <div class="warp-flight" id="warp-flight" hidden aria-hidden="true">
          <div class="warp-flight-stage" id="warp-flight-stage"></div>
          <div class="warp-flight-title" id="warp-flight-title"></div>
        </div>
        <script>
        /* 点击友链卡片 → 播放「星际穿越」→ 白闪后同标签页跳转。
           页面只负责遮罩、艺术字与加载器；动画本体在 /blog/assets/warp-flight.js。
           任何一步失败都直接跳转：过渡是锦上添花，不能成为去不了对方站点的原因。
           reduced-motion 直接跳转，不下载 3.2 MB 的动画包。 */
        (function () {
          var still = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
          var overlay = document.getElementById('warp-flight');
          var stage = document.getElementById('warp-flight-stage');
          var title = document.getElementById('warp-flight-title');
          var cards = document.querySelectorAll('.friend-card');
          if (!overlay || !stage || !title || !cards.length) return;

          var loading = false, failed = false, current = null;

          function go(url) { if (url) window.location.href = url; }

          function lock(on) { document.body.classList.toggle('warp-flight-lock', !!on); }

          function ensureFlight(cb) {
            if (window.__WFLIGHT) { cb(true); return; }
            if (failed) { cb(false); return; }
            if (loading) return;
            loading = true;
            var s = document.createElement('script');
            s.src = '/blog/assets/warp-flight.js?v={{assetVer}}';
            s.async = true;
            s.onload = function () { loading = false; cb(!!window.__WFLIGHT); };
            s.onerror = function () { failed = true; loading = false; cb(false); };
            document.head.appendChild(s);
          }

          function fly(card) {
            var url = card.getAttribute('href') || '';
            if (!url) return;
            var name = card.getAttribute('data-name') || '';
            var avatar = card.getAttribute('data-avatar') || '';
            if (still) { go(url); return; }
            current = url;
            /* .shell 上可能残留「穿越抵达」的缩放 transform。带 transform 的祖先会让
               position:fixed 锚到祖先而不是视口，遮罩会缩成文章列宽、艺术字被裁掉。
               先挂到 body，再取消 hidden / 开始测量，遮罩才是整屏。 */
            if (overlay.parentNode !== document.body) document.body.appendChild(overlay);
            title.textContent = '正在飞往（' + name + '）的星球';
            overlay.hidden = false;
            overlay.classList.add('is-on');
            lock(true);
            ensureFlight(function (ok) {
              if (!ok) { go(url); return; }
              try {
                window.__WFLIGHT.start({ name: name, url: url, avatar: avatar });
              } catch (e) {
                go(url);
              }
            });
          }

          Array.prototype.forEach.call(cards, function (card) {
            card.addEventListener('click', function (ev) {
              if (ev.defaultPrevented || ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;
              if (!card.getAttribute('href')) return;
              ev.preventDefault();
              fly(card);
            });
          });

          /* 点击画面随时跳过；skip 在 start 之前调用也安全——
             动画脚本会看到 SKIPPED，并在 init 完成时直接触发完成回调。 */
          overlay.addEventListener('click', function () {
            if (window.__WFLIGHT && window.__WFLIGHT.skip) { window.__WFLIGHT.skip(); }
            else { go(current); }
          });
        })();
        </script>
        {{end}}
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
    <div>由 <a href="https://obsidian.md" target="_blank" rel="noopener noreferrer">Obsidian</a> 提供内容支持</div>
    {{template "footer-legal" .}}
  </footer>
</div>
{{if .WaveJS}}<script>{{.WaveJS}}</script>{{end}}
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
<script src="/blog/code.js?v={{assetVer}}" defer></script>

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
  <div class="dock dock-collapsible" id="fns-dock" role="group" aria-label="页面入口" data-slot="0">
    <button type="button" class="dock-orb" id="fns-orb" aria-label="展开页面入口" aria-expanded="false">
      <span class="taiji-lottie" data-lottie="code">
        <svg class="taiji" viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="12" cy="12" r="9.1" fill="none" stroke="currentColor" stroke-width="1.35"/>
        <path d="M12 2.9a4.55 4.55 0 0 1 0 9.1 4.55 4.55 0 0 0 0 9.1" fill="currentColor"/>
        <circle cx="12" cy="7.45" r="1.3" fill="var(--card)"/>
        <circle cx="12" cy="16.55" r="1.3" fill="currentColor"/>
      </svg>
      </span>
    </button>
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
// Collapsed capsule, same behaviour as the card page: one round key until asked, and it closes
// again the moment a click lands outside it.
// 收起的胶囊，与名片页同一套行为：在读者要之前只是一个圆键，而任何落在它外面的点击都会让它收回。
(function () {
  var orb = document.getElementById('fns-orb');
  if (!dock || !orb) { return; }
  function setOpen(on) {
    dock.classList.toggle('is-open', on);
    orb.setAttribute('aria-expanded', on ? 'true' : 'false');
  }
  orb.addEventListener('click', function (ev) {
    ev.stopPropagation();
    var opening = !dock.classList.contains('is-open');
    // Expanding announces itself first: the orb plays once, then the dock opens. A second press
    // while it plays is dropped rather than queued.
    //
    // 展开先自我通报：球播一遍，然后胶囊打开。播放期间的第二次按压直接丢弃，而不是排队。
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
  document.addEventListener('click', function (ev) {
    if (dock.classList.contains('is-open') && !dock.contains(ev.target)) { setOpen(false); }
  });
  Array.prototype.forEach.call(dock.querySelectorAll('.dock-b'), function (b) {
    b.addEventListener('click', function () { setOpen(false); });
  });
})();
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
<link rel="icon" type="image/svg+xml" href="/blog/icon.svg">
<link rel="apple-touch-icon" href="/blog/icon.svg">
<script>{{themejs}}</script>
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
      <a class="brand-mark" href="/blog" title="返回首页"><img src="/blog/icon-mark.svg" alt="" width="34" height="34"></a>
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
    <div>由 <a href="https://obsidian.md" target="_blank" rel="noopener noreferrer">Obsidian</a> 提供内容支持</div>
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
