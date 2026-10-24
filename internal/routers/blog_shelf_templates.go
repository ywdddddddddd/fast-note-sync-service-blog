package routers

import (
	"html/template"
)

// The shelf grid and the reader are two pages that share the blog's skeleton — same backdrop,
// same design tokens, same easing — but not its three-column layout. The shelf is a wall of
// covers; the reader is a single sheet. Neither wants a sidebar.
//
// 书架网格与阅读器是两个共用博客骨架的页面——同样的底衬、同样的设计令牌、同样的缓动——
// 但不共用三栏布局。书架是一面封面墙，阅读器是一张纸。两者都不需要侧栏。

// shelfCard is one cover on the wall. Everything here is display data; the book id is the only
// thing that travels in a URL.
//
// shelfCard 是墙上的一张封面。这里的一切都是显示数据；书的 id 是唯一会进入 URL 的东西。
type shelfCard struct {
	ID       string
	Title    string
	Author   string
	Note     string
	Tags     []string
	Added    string
	Pages    int
	CoverURL string
}

type shelfView struct {
	Title     string
	SiteName  string
	Indexable bool
	CSS       template.CSS
	Cards     []shelfCard
	Count     int

	Canonical   string
	Description string
	FeedURL     string
	JSONLD      template.JS
}

type readerView struct {
	Title     string
	SiteName  string
	Indexable bool
	CSS       template.CSS

	ID        string
	BookTitle string
	Author    string
	Note      string
	PageCount int
	// Version stamps image URLs so a re-rendered book busts the 30-day cache.
	// Version 给图片 URL 打戳，重做后击穿 30 天缓存。
	Version   string
	FirstPage string // URL of page 1, server-rendered so the page shows something without JS

	Canonical   string
	Description string
	FeedURL     string
	JSONLD      template.JS
}

// The grid. Cards are revealed in sequence by pure CSS animation delay, the same trick the home
// page's note list uses — a script-driven stagger would render one frame in the final state first.
//
// 网格。卡片由纯 CSS 的动画延迟依次出现，与首页笔记列表同一个手法——
// 用脚本做错峰会先以最终状态画出一帧。
const blogShelfPage = `<!DOCTYPE html>
<html lang="zh-CN"{{if not .Indexable}} data-noindex{{end}}>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>{{.Title}}</title>
{{if not .Indexable}}<meta name="robots" content="noindex,nofollow">{{end}}
<meta name="description" content="{{.Description}}">
<link rel="canonical" href="{{.Canonical}}">
<link rel="alternate" type="application/rss+xml" title="{{.SiteName}}" href="{{.FeedURL}}">
<meta property="og:type" content="website">
<meta property="og:title" content="{{.SiteName}} · 资料">
<meta property="og:description" content="{{.Description}}">
<meta name="twitter:card" content="summary">
{{if .JSONLD}}<script type="application/ld+json">{{.JSONLD}}</script>{{end}}
<style>{{.CSS}}</style>
</head>
<body class="is-shelf">
<div class="backdrop"></div>
<div class="shell">
  <header class="topbar"><div class="topbar-inner">
    <div class="brand">
      <a class="brand-mark" href="/blog" title="返回首页"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/></svg></a>
      <div style="min-width:0">
        <a class="brand-name" href="/blog">{{.SiteName}}</a>
        <div class="brand-sub">资料</div>
      </div>
    </div>
    <div class="topbar-spacer"></div>
    <nav class="crumb"><a href="/blog">首页</a><span>/</span><b>资料</b></nav>
  </div></header>

  <main class="shelf">
    <div class="shelf-head">
      <h1 class="shelf-h1">资料</h1>
      <p class="shelf-sub">共 {{.Count}} 份文档 · 点击封面在线阅读</p>
    </div>

    {{if .Cards}}
    <div class="shelf-grid">
      {{range .Cards}}
      <a class="shelf-card" href="/blog/shelf/{{.ID}}">
        <div class="shelf-cover">
          <div class="shelf-jacket"><span>{{.Title}}</span></div>
          <img src="{{.CoverURL}}" alt="{{.Title}} 封面" loading="lazy" decoding="async" onerror="this.remove()">
          <div class="shelf-sheen"></div>
        </div>
        <div class="shelf-info">
          <h2 class="shelf-title">{{.Title}}</h2>
          {{if .Author}}<p class="shelf-author">{{.Author}}</p>{{end}}
          {{if .Note}}<p class="shelf-note">{{.Note}}</p>{{end}}
          <div class="shelf-foot"><span>{{.Pages}} 页</span>{{if .Added}}<span>{{.Added}}</span>{{end}}</div>
        </div>
      </a>
      {{end}}
    </div>
    {{else}}
    <div class="shelf-empty">
      <p>书架还是空的。</p>
      <p class="shelf-empty-hint">把 PDF 放进服务器的 <code>storage/blog-shelf/books/</code>，再在 <code>shelf.json</code> 里登记它。</p>
    </div>
    {{end}}
  </main>

  <footer class="footer">
    <div>由 <a href="https://github.com/haierkeys/fast-note-sync-service" target="_blank" rel="noopener noreferrer">Fast Note Sync</a> 提供支持</div>
    <div class="footer-legal"><a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer nofollow">浙ICP备2026089991号-1</a></div>
  </footer>
</div>
</body>
</html>`

// The reader. Page one is rendered server-side so the sheet is never blank, and every other page
// arrives only when asked for — that is the traffic contract, so nothing here may preload the
// whole book.
//
// 阅读器。第一页由服务端渲染，所以纸面不会是空白；其余每一页只在被索取时才到来——
// 这就是流量契约，因此这里任何地方都不许预载整本书。
const blogReaderPage = `<!DOCTYPE html>
<html lang="zh-CN"{{if not .Indexable}} data-noindex{{end}}>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>{{.BookTitle}} · {{.SiteName}}</title>
{{if not .Indexable}}<meta name="robots" content="noindex,nofollow">{{end}}
<meta name="description" content="{{.Description}}">
<link rel="canonical" href="{{.Canonical}}">
<meta property="og:type" content="article">
<meta property="og:title" content="{{.BookTitle}}">
<meta property="og:description" content="{{.Description}}">
<meta name="twitter:card" content="summary">
{{if .JSONLD}}<script type="application/ld+json">{{.JSONLD}}</script>{{end}}
<style>{{.CSS}}</style>
</head>
<body class="is-reader">
<div class="reader" id="fns-reader" data-book="{{.ID}}" data-pages="{{.PageCount}}" data-ver="{{.Version}}">

  <header class="reader-bar">
    <a class="reader-back" href="/blog/shelf" id="fns-reader-back" aria-label="返回资料">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><path d="m12 19-7-7 7-7"/></svg>
    </a>
    <div class="reader-head">
      <div class="reader-title">{{.BookTitle}}</div>
      {{if .Author}}<div class="reader-by">{{.Author}}</div>{{end}}
    </div>
    <div class="reader-tools">
      <button type="button" class="reader-btn" id="fns-reader-zoom-out" aria-label="缩小">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/></svg>
      </button>
      <span class="reader-zoom" id="fns-reader-zoom">100%</span>
      <button type="button" class="reader-btn" id="fns-reader-zoom-in" aria-label="放大">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14"/><path d="M5 12h14"/></svg>
      </button>
      <button type="button" class="reader-btn reader-btn-fs" id="fns-reader-fs" aria-label="全屏">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 3H5a2 2 0 0 0-2 2v3"/><path d="M21 8V5a2 2 0 0 0-2-2h-3"/><path d="M3 16v3a2 2 0 0 0 2 2h3"/><path d="M16 21h3a2 2 0 0 0 2-2v-3"/></svg>
      </button>
    </div>
  </header>

  <div class="reader-stage" id="fns-reader-stage">
    <div class="reader-stack" id="fns-reader-stack">
      <img class="reader-sheet" id="fns-sheet-a" src="{{.FirstPage}}" alt="{{.BookTitle}} 第 1 页" decoding="async" fetchpriority="high">
      <img class="reader-sheet" id="fns-sheet-b" alt="" decoding="async">
    </div>
    <div class="reader-tail" aria-hidden="true">末页</div>
  </div>

  <!-- 桌面端的翻页箭头：平时淡出，鼠标一动才浮上来。移动端隐藏，用底部的按钮。 -->
  <button type="button" class="reader-arrow reader-arrow-prev" id="fns-arrow-prev" aria-label="上一页">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"/></svg>
  </button>
  <button type="button" class="reader-arrow reader-arrow-next" id="fns-arrow-next" aria-label="下一页">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>
  </button>

  <footer class="reader-ctl">
    <button type="button" class="reader-nav" id="fns-reader-prev" aria-label="上一页">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"/></svg>
    </button>
    <div class="reader-jump">
      <input id="fns-reader-input" type="text" inputmode="numeric" value="1" aria-label="页码" autocomplete="off">
      <span class="reader-of">/ {{.PageCount}}</span>
    </div>
    <div class="reader-track" id="fns-reader-track">
      <div class="reader-rail"><i id="fns-reader-rail"></i></div>
    </div>
    <button type="button" class="reader-nav" id="fns-reader-next" aria-label="下一页">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>
    </button>
  </footer>

  <div class="reader-hint" id="fns-reader-hint" hidden></div>
</div>
<script src="/blog/reader.js" defer></script>
</body>
</html>`

var blogShelfTmpl = template.Must(template.New("blog-shelf").Parse(blogShelfPage))
var blogReaderTmpl = template.Must(template.New("blog-reader").Parse(blogReaderPage))
