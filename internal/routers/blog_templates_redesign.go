package routers

import "html/template"

// blogChromePartials is the shared shell: <head>, the top bar, the footer, the mobile drawer's
// folder tree.
//
// It is one string rather than four copies because the four head blocks used to drift: the home
// page carried an og:image the article page did not, and a change to the filing number had to be
// made in every template. A partial cannot drift from itself.
//
// blogChromePartials 是共用的外壳：<head>、顶栏、页脚、移动端抽屉的目录树。
//
// 它是一份字符串而不是四份拷贝，因为四段 head 曾经各自漂移：首页带着一个文章页没有的
// og:image，而改一个备案号要在每个模板里改一遍。片段不会与自己不一致。
var blogChromePartials = `
{{define "bloghead"}}<!DOCTYPE html>
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
{{end}}

{{define "blogtopbar"}}<header class="topbar"><div class="topbar-inner">
  <a class="brand" href="/blog">
    <span class="brand-mark"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/></svg></span>
    <span class="brand-name">{{.SiteName}}</span>
  </a>
  <div class="topbar-spacer"></div>
  <nav class="topnav" aria-label="主导航">
    <details class="dd">
      <summary class="dd-sum">博客<svg class="dd-caret" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M4.427 6.427a.75.75 0 0 1 1.06 0L8 8.94l2.513-2.513a.75.75 0 1 1 1.06 1.06L8.53 10.53a.75.75 0 0 1-1.06 0L4.427 7.487a.75.75 0 0 1 0-1.06Z"/></svg></summary>
      <div class="dd-body">
        <a href="/blog/all">全部笔记</a>
        {{range .Sections}}<a href="/blog/c/{{.Slug}}">{{.Name}}<span class="dd-n">{{.Count}}</span></a>{{end}}
        <a href="/blog/archives">归档</a>
      </div>
    </details>
    {{range .Pages}}<a href="{{.URL}}">{{.Title}}</a>{{end}}
    {{if .HasShelf}}<a href="/blog/shelf">资料</a>{{end}}
    {{if and .Indexable .FeedURL}}<button type="button" class="topnav-feed" data-feed="{{.FeedURL}}" aria-label="订阅《{{.SiteName}}》"><svg class="topnav-feed-icon" width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path d="M2.002 2.725a.75.75 0 0 1 .797-.699C8.79 2.42 13.58 7.21 13.974 13.201a.75.75 0 0 1-1.497.098 10.502 10.502 0 0 0-9.776-9.776.747.747 0 0 1-.7-.798ZM2.84 7.05h-.002a7.002 7.002 0 0 1 6.113 6.111.75.75 0 0 1-1.49.178 5.503 5.503 0 0 0-4.8-4.8.75.75 0 0 1 .179-1.489ZM2 13a1 1 0 1 1 2 0 1 1 0 0 1-2 0Z"/></svg><span class="topnav-feed-text">订阅</span></button>{{end}}
  </nav>
  <button type="button" class="burger" id="fns-burger" aria-label="菜单" aria-expanded="false" aria-controls="fns-mnav"><svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M1 2.75A.75.75 0 0 1 1.75 2h12.5a.75.75 0 0 1 0 1.5H1.75A.75.75 0 0 1 1 2.75Zm0 5A.75.75 0 0 1 1.75 7h12.5a.75.75 0 0 1 0 1.5H1.75A.75.75 0 0 1 1 7.75ZM1.75 12h12.5a.75.75 0 0 1 0 1.5H1.75a.75.75 0 0 1 0-1.5Z"/></svg></button>
</div></header>
<div class="mnav" id="fns-mnav" hidden>
  <div class="mnav-title">博客</div>
  <a href="/blog/all">全部笔记</a>
  {{range .Sections}}<a href="/blog/c/{{.Slug}}">{{.Name}}<span class="dd-n">{{.Count}}</span></a>{{end}}
  <a href="/blog/archives">归档</a>
  <div class="mnav-title">页面</div>
  {{range .Pages}}<a href="{{.URL}}">{{.Title}}</a>{{end}}
  {{if .HasShelf}}<a href="/blog/shelf">资料</a>{{end}}
</div>
{{end}}

{{define "blogfooter"}}<footer class="footer footer-duo">
  <div class="footer-top">
    <a href="/blog/archives">归档</a>
    <a href="/blog/all">全部笔记</a>
    {{if and .Indexable .FeedURL}}<a href="{{.FeedURL}}">RSS</a>{{end}}
    <span class="footer-mark">由 <a href="https://github.com/haierkeys/fast-note-sync-service" target="_blank" rel="noopener noreferrer">Fast Note Sync</a> 提供支持</span>
  </div>
  <div class="footer-bottom">{{template "footer-legal" .}}</div>
</footer>{{end}}

{{define "pillicon"}}<svg class="pill-i" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">{{if eq .Icon "location"}}<path d="M8 16s6-5.686 6-10A6 6 0 0 0 2 6c0 4.314 6 10 6 10Zm0-7a3 3 0 1 1 0-6 3 3 0 0 1 0 6Z"/>{{else if eq .Icon "mark-github"}}<path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/>{{else if eq .Icon "mail"}}<path d="M1.75 2h12.5c.966 0 1.75.784 1.75 1.75v8.5A1.75 1.75 0 0 1 14.25 14H1.75A1.75 1.75 0 0 1 0 12.25v-8.5C0 2.784.784 2 1.75 2Zm.25 1.5v.56l6 3.75 6-3.75v-.56a.25.25 0 0 0-.25-.25H2.25a.25.25 0 0 0-.25.25Zm12.5 2.44-6 3.75-6-3.75v5.81c0 .138.112.25.25.25h11.5a.25.25 0 0 0 .25-.25V5.94Z"/>{{else if eq .Icon "mortar-board"}}<path d="M8 .5 0 4.5l8 4 8-4-8-4Zm-5.5 6.2v3.6c0 .4.2.7.6.9 1.4.8 3.1 1.3 4.9 1.3s3.5-.5 4.9-1.3c.4-.2.6-.5.6-.9V6.7L8 9.8 2.5 6.7Z"/>{{else if eq .Icon "rss"}}<path d="M2.002 2.725a.75.75 0 0 1 .797-.699C8.79 2.42 13.58 7.21 13.974 13.201a.75.75 0 0 1-1.497.098 10.502 10.502 0 0 0-9.776-9.776.747.747 0 0 1-.7-.798ZM2.84 7.05h-.002a7.002 7.002 0 0 1 6.113 6.111.75.75 0 0 1-1.49.178 5.503 5.503 0 0 0-4.8-4.8.75.75 0 0 1 .179-1.489ZM2 13a1 1 0 1 1 2 0 1 1 0 0 1-2 0Z"/>{{else if eq .Icon "qr"}}<path d="M1.5 1h4A.5.5 0 0 1 6 1.5v4a.5.5 0 0 1-.5.5h-4A.5.5 0 0 1 1 5.5v-4A.5.5 0 0 1 1.5 1Zm.5 1v3h3V2H2Zm8-1h4a.5.5 0 0 1 .5.5v4a.5.5 0 0 1-.5.5h-4a.5.5 0 0 1-.5-.5v-4a.5.5 0 0 1 .5-.5Zm.5 1v3h3V2h-3Zm-9.5 8h4a.5.5 0 0 1 .5.5v4a.5.5 0 0 1-.5.5h-4a.5.5 0 0 1-.5-.5v-4a.5.5 0 0 1 .5-.5Zm.5 1v3h3v-3H2Zm7-1h1.5v1.5H9V10h1.5Zm2.5 0H13v1.5h-1.5V10H11Zm-2.5 2.5H11V14H9.5v-1.5H9V11h1.5v1.5Zm2.5 0H13v1.5h-1.5v-1.5H11V11h1.5v1.5Z"/>{{else if eq .Icon "globe"}}<path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0Zm5.9 5H11.6a13.4 13.4 0 0 0-1.3-3.4A6.5 6.5 0 0 1 13.9 5ZM8 1.6c.6.8 1.2 2 1.6 3.4H6.4C6.8 3.6 7.4 2.4 8 1.6ZM6.1 1.6A13.4 13.4 0 0 0 4.8 5H2.1a6.5 6.5 0 0 1 4-3.4ZM1.6 6.5h2.8a15 15 0 0 0 0 3H1.6a6.5 6.5 0 0 1 0-3Zm3 4.5c.2 1 .5 2 1 2.9a6.5 6.5 0 0 1-3.5-2.9h2.5Zm3.4 3.4a11 11 0 0 1-1.6-3.4h3.2a11 11 0 0 1-1.6 3.4Zm2.3-4.4c.1-1 .1-2 0-3H5.7a13 13 0 0 0 0 3h4.6Zm.6 3.9c.4-.9.7-1.9.9-2.9h2.5a6.5 6.5 0 0 1-3.4 2.9Zm1.1-4.4c.1-1 .1-2 0-3h2.8a6.5 6.5 0 0 1 0 3h-2.8Z"/>{{else}}<path d="M7.775 3.275a.75.75 0 0 0 .45 0l2.7-1.2a.75.75 0 0 0 0-1.35l-5-2.2a.75.75 0 0 0-.45 0l-5 2.2a.75.75 0 0 0 0 1.35l2.7 1.2v3.4a.75.75 0 0 0 .37.65l2.5 1.4a.75.75 0 0 0 .75 0l2.5-1.4a.75.75 0 0 0 .37-.65v-3.4Z" transform="translate(3.1 5.6)"/>{{end}}</svg>{{end}}

{{define "pill"}}{{if eq .Kind "text"}}<span class="pill">{{template "pillicon" .}}<b>{{.Label}}</b></span>{{else if eq .Kind "rss"}}<button type="button" class="pill" data-feed="{{.Href}}">{{template "pillicon" .}}<b>{{.Label}}</b></button>{{else if eq .Kind "qr"}}<button type="button" class="pill" data-qr="1">{{template "pillicon" .}}<b>{{.Label}}</b></button>{{else}}<a class="pill" href="{{.Href}}"{{if eq .Kind "mail"}}{{else}} target="_blank" rel="noopener noreferrer"{{end}}>{{template "pillicon" .}}<b>{{.Label}}</b></a>{{end}}{{end}}

{{define "card"}}<li class="post-card">
  <a class="card-link" href="/blog/{{.Slug}}">
    {{if .Cover}}{{if eq .Cover.Kind "video"}}<video class="cover" src="{{.Cover.URL}}" muted playsinline preload="metadata" aria-hidden="true"></video>{{else}}<img class="cover" src="{{.Cover.URL}}" alt="" loading="lazy" width="1200" height="630" style="object-position:{{.Cover.FocusX}}% {{.Cover.FocusY}}%">{{end}}{{end}}
    <span class="card-date">{{.CreatedAt}}</span>
    <div class="card-body">
      <h2 class="card-h">{{.Title}}</h2>
      {{if .Summary}}<p class="card-sum">{{.Summary}}</p>{{end}}
      <div class="card-meta"><span>{{.Words}} 字</span>{{if .Vault}}<span>{{.Vault}}</span>{{end}}<span>出链 {{.LinksOut}}</span></div>
    </div>
  </a>
</li>{{end}}

{{define "pager"}}<nav class="pager" aria-label="分页">{{range .PagerLinks}}{{if .Current}}<span class="pg is-on" aria-current="page">{{.Num}}</span>{{else}}<a class="pg" href="{{.URL}}" aria-label="第 {{.Num}} 页">{{.Num}}</a>{{end}}{{end}}</nav>{{end}}
` + `{{define "blognav"}}` + blogNavPartial + `{{end}}` + blogNavTree

// blogCardTmpl is the card page.
//
// The order below is the reference site's order, and it is a reading order rather than a layout
// preference: who this is, what they do, how much they have written, when they wrote it. The heat
// map closes the page because it is the only block that rewards a second look.
//
// blogCardTmpl 是名片页。
//
// 下面的顺序就是参照站的顺序，而它是阅读顺序而不是排版偏好：
// 这是谁、做什么、写了多少、什么时候写的。热力图收尾，因为它是唯一值得再看一眼的区块。
var blogCardTmpl = template.Must(template.New("blog-card").Funcs(blogAssetFuncs).Parse(blogFooterLegal + blogChromePartials + `
{{template "bloghead" .}}
<body class="is-home">
<div class="backdrop"></div>
<div class="shell">
{{template "blogtopbar" .}}

<main class="card-wrap">
  <section class="hero" data-anim="1">
    <div class="hero-avatar">
      {{if .Profile}}{{if .Profile.Avatar}}<img src="{{.Profile.Avatar}}" alt="{{.Profile.Name}}">{{else}}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8Z"/><path d="M4 21c0-3.9 3.6-7 8-7s8 3.1 8 7"/></svg>{{end}}{{end}}
    </div>
    {{if .Profile}}{{if .Profile.Name}}<h1 class="hero-name">{{.Profile.Name}}</h1>{{end}}{{if .Profile.Tagline}}<p class="hero-tag">{{.Profile.Tagline}}</p>{{end}}{{end}}
    {{if .Pills}}<div class="pills">{{range .Pills}}{{template "pill" .}}{{end}}</div>{{end}}
  </section>

  {{if .Profile}}{{if .Profile.Meta}}
  <section class="card card-pad kv-block" data-anim="2">
    <div class="sec-head"><h2 class="sec-label">关于</h2></div>
    <dl class="kv">
      {{range .Profile.Meta}}<div class="kv-row"><dt>{{.Label}}</dt><dd>{{.Value}}</dd></div>{{end}}
    </dl>
  </section>
  {{end}}{{end}}

  <section class="stats-row" data-anim="3">
    <div class="stat-card">
      <div class="stat-ico"><svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M4.75 0a.75.75 0 0 1 .75.75V2h5V.75a.75.75 0 0 1 1.5 0V2h1.25c.966 0 1.75.784 1.75 1.75v10.5A1.75 1.75 0 0 1 13.25 16H2.75A1.75 1.75 0 0 1 1 14.25V3.75C1 2.784 1.784 2 2.75 2H4V.75A.75.75 0 0 1 4.75 0ZM2.5 5.5v8.75c0 .138.112.25.25.25h10.5a.25.25 0 0 0 .25-.25V5.5h-11Z"/></svg></div>
      <div class="stat-num">{{.Stats.DaysOnline}}</div>
      <div class="stat-label">站龄 · 天</div>
      {{if .Stats.Since}}<div class="stat-sub">自 {{.Stats.Since}}</div>{{end}}
    </div>
    <div class="stat-card">
      <div class="stat-ico"><svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M1.75 1h12.5c.966 0 1.75.784 1.75 1.75v10.5A1.75 1.75 0 0 1 14.25 15H1.75A1.75 1.75 0 0 1 0 13.25V2.75C0 1.784.784 1 1.75 1Zm.25 1.5v10.25h12.5V2.5H2Zm2 2.5h4v1.5H4V5Zm0 3h7v1.5H4V8Zm0 3h5v1.5H4V11Z"/></svg></div>
      <div class="stat-num stat-date">{{if .Stats.Updated}}{{.Stats.Updated}}{{else}}—{{end}}</div>
      <div class="stat-label">最后更新</div>
      <div class="stat-sub">{{.Stats.Notes}} 篇公开笔记</div>
    </div>
    <div class="stat-card">
      <div class="stat-ico"><svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M2 1.75C2 .784 2.784 0 3.75 0h6.586c.464 0 .909.184 1.237.513l2.914 2.914c.329.328.513.773.513 1.237v9.586A1.75 1.75 0 0 1 13.25 16H3.75A1.75 1.75 0 0 1 2 14.25V1.75Zm1.5 0v12.5c0 .138.112.25.25.25h9.5a.25.25 0 0 0 .25-.25V4.5H10.75A1.75 1.75 0 0 1 9 2.75V1.5H3.75a.25.25 0 0 0-.25.25Zm7.5.75v.25c0 .138.112.25.25.25h.25L11 2.5Z"/></svg></div>
      <div class="stat-num">{{.Stats.WordsText}}</div>
      <div class="stat-label">总字数</div>
      <div class="stat-sub">{{.Stats.Tags}} 个标签 · {{.Stats.Links}} 条双链</div>
    </div>
  </section>

  {{if .Activity}}
  <section class="card card-pad act-card" id="activity" data-anim="4">
    <div class="sec-head">
      <h2 class="sec-label">活跃度</h2>
      <span class="sec-note">按笔记创建日期 · 最近 {{.Activity.Weeks}} 周</span>
    </div>
    <div class="heat" style="--weeks:{{.Activity.Weeks}}">
      {{range .Activity.Days}}<i class="hc lv{{.Lvl}}" title="{{.Date}} · {{.Count}} 篇"></i>{{end}}
    </div>
    <div class="heat-months" style="--weeks:{{.Activity.Weeks}}">{{range .Activity.Months}}<i>{{.}}</i>{{end}}</div>
    <div class="act-foot">
      <span>{{.Activity.NoteDays}} 天有笔记</span>
      <span class="heat-legend">少
        <i class="hc lv0"></i><i class="hc lv1"></i><i class="hc lv2"></i><i class="hc lv3"></i><i class="hc lv4"></i>
      多</span>
      <span>单日最多 {{.Activity.Max}} 篇</span>
    </div>
  </section>
  {{end}}
</main>

{{template "blogfooter" .}}
</div>

<!-- The two sheets the mobile capsule opens. They are always in the DOM and moved by class, not
     by the hidden attribute: a display:none panel cannot animate in, and toggling hidden from a
     class-driven stylesheet is the kind of two-owner state that produces "the button did nothing".

     移动胶囊打开的两块面板。它们始终在 DOM 里，由 class 移动而不是 hidden 属性：
     display:none 的面板无法滑入，而用 class 驱动的样式表再去切换 hidden，
     正是那种会产生「点了没反应」的双所有者状态。 -->
<div class="card-sheets">
  <aside class="card-sheet sheet-nav" aria-label="目录">{{template "blognav" .}}</aside>
  {{if .Hot}}<aside class="card-sheet sheet-rec" aria-label="推荐">
    <div class="nav-title">推荐</div>
    <ul class="rec">
      {{range .Hot}}<li><a href="/blog/{{.Slug}}">{{.Title}}</a>{{if .Summary}}<p>{{.Summary}}</p>{{end}}</li>{{end}}
    </ul>
  </aside>{{end}}
</div>

<div class="fns-scrim" id="fns-scrim" hidden></div>
<div class="dock" id="fns-dock" role="group" aria-label="页面入口" data-slot="0">
  <span class="dock-thumb" aria-hidden="true"></span>
  <button type="button" class="dock-b" data-i="0" id="dock-nav" aria-label="目录" aria-expanded="false"><svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M0 1.75A.75.75 0 0 1 .75 1h4.253c1.227 0 2.317.59 3 1.501A3.743 3.743 0 0 1 11.006 1h4.245a.75.75 0 0 1 .75.75v10.5a.75.75 0 0 1-.75.75h-4.507a2.25 2.25 0 0 0-1.591.659l-.622.621a.75.75 0 0 1-1.06 0l-.622-.621A2.25 2.25 0 0 0 5.258 13H.75a.75.75 0 0 1-.75-.75Zm7.251 10.324.004-5.073-.002-2.253A2.25 2.25 0 0 0 5.003 2.5H1.5v9h3.757a3.75 3.75 0 0 1 1.994.574ZM8.755 4.75l-.004 7.322a3.752 3.752 0 0 1 1.992-.572H14.5v-9h-3.495a2.25 2.25 0 0 0-2.25 2.25Z"/></svg></button>
  <button type="button" class="dock-b" data-i="1" id="dock-hot" aria-label="推荐" aria-expanded="false"><svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M8 .25a.75.75 0 0 1 .673.418l1.882 3.815 4.21.612a.75.75 0 0 1 .416 1.279l-3.046 2.97.719 4.192a.751.751 0 0 1-1.088.791L8 12.347l-3.766 1.98a.75.75 0 0 1-1.088-.79l.72-4.194L.818 6.374a.75.75 0 0 1 .416-1.28l4.21-.611L7.327.668A.75.75 0 0 1 8 .25Z"/></svg></button>
  <button type="button" class="dock-b" data-i="2" id="dock-walk" aria-label="漫游 · 随机打开一篇笔记"><svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M13.25 1c.966 0 1.75.784 1.75 1.75v10.5c0 .464-.184.909-.513 1.237A1.746 1.746 0 0 1 13.25 15H2.75c-.464 0-.909-.184-1.237-.513A1.746 1.746 0 0 1 1 13.25V2.75C1 1.784 1.784 1 2.75 1h10.5ZM2.75 2.5a.25.25 0 0 0-.25.25v10.5c0 .138.112.25.25.25h10.5a.25.25 0 0 0 .25-.25V2.75a.25.25 0 0 0-.25-.25H2.75Z"/><path d="M5 12a1 1 0 1 0 0-2 1 1 0 0 0 0 2Zm6-6a1 1 0 1 0 0-2 1 1 0 0 0 0 2ZM8 9a1 1 0 1 0 0-2 1 1 0 0 0 0 2Z"/></svg></button>
  <button type="button" class="dock-b" data-i="3" id="dock-live" aria-label="活跃度"><svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M6 2c.306 0 .582.187.696.471L10 10.731l1.304-3.26A.751.751 0 0 1 12 7h3.25a.75.75 0 0 1 0 1.5h-2.742l-1.812 4.528a.751.751 0 0 1-1.392 0L6 4.77 4.696 8.03A.75.75 0 0 1 4 8.5H.75a.75.75 0 0 1 0-1.5h2.742l1.812-4.529A.751.751 0 0 1 6 2Z"/></svg></button>
  <button type="button" class="dock-b" data-i="4" id="dock-ask" aria-label="问道 · 向知识库提问" aria-expanded="false" data-ask-open><svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M1.75 1h8.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 10.25 10H7.061l-2.574 2.573A1.458 1.458 0 0 1 2 11.543V10h-.25A1.75 1.75 0 0 1 0 8.25v-5.5C0 1.784.784 1 1.75 1ZM1.5 2.75v5.5c0 .138.112.25.25.25h1a.75.75 0 0 1 .75.75v2.19l2.72-2.72a.749.749 0 0 1 .53-.22h3.5a.25.25 0 0 0 .25-.25v-5.5a.25.25 0 0 0-.25-.25h-8.5a.25.25 0 0 0-.25.25Zm13 2a.25.25 0 0 0-.25-.25h-.5a.75.75 0 0 1 0-1.5h.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 14.25 12H14v1.543a1.458 1.458 0 0 1-2.487 1.03L9.22 12.28a.749.749 0 0 1 .326-1.275.749.749 0 0 1 .734.215l2.22 2.22v-2.19a.75.75 0 0 1 .75-.75h1a.25.25 0 0 0 .25-.25Z"/></svg></button>
</div>

<script>{{.DockJS}}</script>
{{if .StatsJS}}<script>{{.StatsJS}}</script>{{end}}
{{if .WarpJS}}<script>{{.WarpJS}}</script>{{end}}
{{if .AskEnabled}}
<div id="ask-root"></div>
<script src="/blog/ask.js?v={{assetVer}}" defer></script>
<script src="/blog/sheet.js?v={{assetVer}}" defer></script>
{{end}}
<script src="/blog/feed.js?v={{assetVer}}" defer></script>
<script src="/blog/lightbox.js?v={{assetVer}}" defer></script>
</body>
</html>
`))

// blogListTmpl renders /blog/all and /blog/c/<section>.
//
// The rail carries exactly one block. That is not a placeholder for more: the reference site's
// list rail is Collections + Tags, and this site dropped both — sections moved to the nav dropdown
// and the tag cloud was removed outright — so the rail holds the recommendations and nothing else.
//
// blogListTmpl 渲染 /blog/all 与 /blog/c/<版块>。
//
// 右栏只承载一块。这不是「以后还会加」的占位：参照站的列表右栏是 Collections + Tags，
// 而本站两块都不要——版块挪进了顶栏下拉，标签云直接删除——所以右栏只有推荐。
var blogListTmpl = template.Must(template.New("blog-list").Funcs(blogAssetFuncs).Parse(blogFooterLegal + blogChromePartials + `
{{template "bloghead" .}}
<body class="is-list">
<div class="backdrop"></div>
<div class="shell">
{{template "blogtopbar" .}}

<div class="list-wrap">
  <main class="list-main">
    <h1 class="list-kicker">{{.Kicker}}<span class="list-count">{{.Total}}</span></h1>
    {{if .Entries}}
    <ul class="cards">
      {{range .Entries}}{{template "card" .}}{{end}}
    </ul>
    {{if gt .TotalPages 1}}{{template "pager" .}}{{end}}
    {{else}}
    <p class="empty">这里还没有公开的笔记。</p>
    {{end}}
  </main>

  <aside class="list-rail">
    {{if .Hot}}
    <div class="card card-rail">
      <h2 class="rail-title">推荐</h2>
      <ul class="rec">
        {{range .Hot}}<li><a href="/blog/{{.Slug}}">{{.Title}}</a>{{if .Summary}}<p>{{.Summary}}</p>{{end}}</li>{{end}}
      </ul>
    </div>
    {{end}}
  </aside>
</div>

{{template "blogfooter" .}}
</div>
{{if .TintJS}}<script>{{.TintJS}}</script>{{end}}
{{if .WarpJS}}<script>{{.WarpJS}}</script>{{end}}
<script src="/blog/feed.js?v={{assetVer}}" defer></script>
<script src="/blog/lightbox.js?v={{assetVer}}" defer></script>
</body>
</html>
`))

// blogArchivesTmpl renders /blog/archives.
//
// The year is a watermark rather than a heading: it is absolutely positioned, transparent at 10%,
// and deliberately does not occupy layout. That is what makes a 5000px page scannable — the eye
// catches a giant faint number while scrolling and knows the year changed without reading a single
// date.
//
// blogArchivesTmpl 渲染 /blog/archives。
//
// 年份是水印而不是标题：绝对定位、10% 透明、有意不占布局。这正是让一张 5000px 的页面可扫读的原因——
// 滚动时眼睛会抓住一个巨大的淡色数字，于是知道年份变了，而不必读任何一个日期。
var blogArchivesTmpl = template.Must(template.New("blog-archives").Funcs(blogAssetFuncs).Parse(blogFooterLegal + blogChromePartials + `
{{template "bloghead" .}}
<body class="is-archive">
<div class="backdrop"></div>
<div class="shell">
{{template "blogtopbar" .}}

<main class="arc-wrap">
  <h1 class="list-kicker">归档<span class="list-count">{{.Total}}</span></h1>
  {{range .Years}}
  <section class="arc-year">
    <h2 class="arc-wm" aria-hidden="true">{{.Year}}</h2>
    <div class="arc-head"><span>{{.Year}}</span><span class="arc-n">{{.Count}} 篇</span></div>
    <ul class="arc-list">
      {{range .Entries}}<li><a href="/blog/{{.Slug}}"><time datetime="{{.CreatedAt}}">{{.CreatedAt}}</time><span class="arc-t">{{.Title}}</span></a></li>{{end}}
    </ul>
  </section>
  {{end}}
</main>

{{template "blogfooter" .}}
</div>
{{if .WarpJS}}<script>{{.WarpJS}}</script>{{end}}
<script src="/blog/feed.js?v={{assetVer}}" defer></script>
<script src="/blog/lightbox.js?v={{assetVer}}" defer></script>
</body>
</html>
`))
