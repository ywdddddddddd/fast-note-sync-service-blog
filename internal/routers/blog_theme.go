package routers

// Blog theme: the design language of the native share page, re-expressed as plain CSS.
//
// The share page ships Tailwind + shadcn/ui compiled into a JavaScript bundle, which is not
// something a server-rendered page can reuse. So the tokens were extracted from
// `theme-switcher-*.css` and translated by hand: same warm-grey palette, same translucent
// borders, same double-layer shadows, same 1.75 line-height and typographic rhythm.
//
// 博客主题：把原生分享页的设计语言用纯 CSS 重新表达。
//
// 分享页的 Tailwind + shadcn/ui 是编译进 JS 包里的，服务端渲染页面无法直接复用。
// 因此这里的令牌是从 `theme-switcher-*.css` 中提取后手工翻译过来的：同一套暖灰配色、
// 同样的半透明边框、同样的双层阴影、同样的 1.75 行高与排版节奏。

const blogCSS = `
/* ── 自定义字体：与分享页同源 ──────────────────────────────────────
   分享页通过 /static/fonts/local.css 加载 CustomFont，博客页直接引用同一份文件，
   两边字形完全一致。

   这份文件是**子集**：原字库 10.2 MB / 46786 字形，含完整中文字库；现在只保留
   博客实际会用到的那批字——全部笔记正文（含未发布笔记，明天发布也不缺字）、
   模板里的界面文字、ASCII 与常用标点，共 3782 个码位，压到 927 KB。
   原字体覆盖的 3743 个字一个没少；缺的 39 个是 emoji 与 Symbol 私用区字符，
   原字体本来就没有，浏览器会按字体栈回退。

   实测动机：读者第一次访问要下 10 MB 字库，期间 font-display:swap 显示的是
   回退字体，观感就是「字体坏了 + 加载慢」。换域名后浏览器缓存归零，这个问题
   才暴露出来。

   ?v= 是内容指纹。同一个 URL 换了字节而浏览器按 30 天缓存旧文件，读者会一直
   用旧字库；改 URL 是最省事且不会失效的破缓存办法。
   font-display:swap 让文字先用回退字体渲染，字体到达后替换，避免首屏空白。 */
@font-face{
  font-family:"CustomFont";
  src:url("/static/fonts/font.woff2?v=61bbee1cc3") format("woff2");
  font-weight:normal;font-style:normal;font-display:swap;
}
:root{
  --font-sans:"CustomFont",-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;
  --background:#fff;
  --foreground:#34322d;
  --card:#fff;
  --muted:#f3f3f3;
  --muted-foreground:#858481;
  --border:#0000001f;
  --primary:#34322d;
  --primary-foreground:#fff;
  --accent:#f3f3f3;
  --radius:.5rem;
  --shadow-sm:0 1px 3px 0px #0000001a,0 1px 2px -1px #0000001a;
  --shadow-md:0 1px 3px 0px #0000001a,0 2px 4px -1px #0000001a;
  --sidebar:#fbfbfb;
  --c1:#34322d;--c2:#5c5a56;--c3:#858481;--c4:#adacaa;--c5:#d6d5d4;
  --dot:#d1d1d1;
  /* Knowledge-graph accent. The reference component uses a single solid green for the
     focused node; kept as its own token so it can be tuned without touching the
     warm-grey palette. */
  --graph-accent:#4b7a43;
}
@media (prefers-color-scheme:dark){
  :root{
    --background:#1a1a1a;--foreground:#ededed;--card:#222220;
    --muted:#2a2a28;--muted-foreground:#a1a1a1;--border:#ffffff1f;
    --primary:#ededed;--primary-foreground:#1a1a1a;--accent:#2a2a28;
    --sidebar:#1e1e1c;--dot:#333;
    /* Knowledge-graph accent, lifted for contrast against #1a1a1a: the reference
       component's #4b7a43 is too dark to read on a dark background. */
    --graph-accent:#7aa870;
    --shadow-sm:0 1px 3px 0px #0000004d,0 1px 2px -1px #0000004d;
    --shadow-md:0 1px 3px 0px #0000004d,0 2px 4px -1px #0000004d;
  }
}
*,*::before,*::after{box-sizing:border-box}
html{-webkit-text-size-adjust:100%;scroll-behavior:smooth}
body{
  margin:0;background:var(--background);color:var(--foreground);
  /* 与分享页同一套字体：CustomFont 排在最前，回退栈与 local.css 保持一致。
     由于 @font-face 自带中文字形，不再需要按平台挑中文字体。 */
  font-family:var(--font-sans);
  font-size:16px;font-weight:400;line-height:1.75;
  -webkit-font-smoothing:antialiased;-moz-osx-font-smoothing:grayscale;
}
a{color:inherit;text-decoration:none;transition:color .18s ease,opacity .18s ease}
img{max-width:100%;height:auto;border-radius:.5rem}
:focus-visible{outline:2px solid var(--primary);outline-offset:2px;border-radius:3px}

/* ── 点阵背景（沿用登录页的 dot grid，20px 间距）─────────────────────────
   Dot-grid backdrop, same as the native .auth-page-container (20px pitch). */
.backdrop{
  position:fixed;inset:0;z-index:0;pointer-events:none;
  background-image:radial-gradient(circle,var(--dot) 1px,transparent 1px);
  background-size:20px 20px;background-position:center;
  opacity:.5;
}
.shell{position:relative;z-index:1;min-height:100vh;display:flex;flex-direction:column}

/* ── 顶栏：粘性 + 毛玻璃 ───────────────────────────────────────────── */
.topbar{
  position:sticky;top:0;z-index:30;
  border-bottom:1px solid var(--border);
  background:color-mix(in srgb,var(--card) 72%,transparent);
  -webkit-backdrop-filter:blur(12px);backdrop-filter:blur(12px);
  transition:background .3s ease;
}
.topbar-inner{
  max-width:1440px;margin:0 auto;padding:.7rem 1.5rem;
  display:flex;align-items:center;gap:1rem;
}
.brand{display:flex;align-items:center;gap:.7rem;min-width:0}
.brand-mark{
  width:34px;height:34px;flex:0 0 34px;border-radius:.75rem;
  display:flex;align-items:center;justify-content:center;
  background:color-mix(in srgb,var(--primary) 10%,transparent);color:var(--primary);
}
.brand-mark svg{width:18px;height:18px}
.brand-name{font-size:1.0625rem;font-weight:700;letter-spacing:-.01em;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.brand-sub{font-size:.75rem;color:var(--muted-foreground);white-space:nowrap}
.topbar-spacer{flex:1}
.crumb{font-size:.8125rem;color:var(--muted-foreground);display:flex;align-items:center;gap:.4rem;min-width:0}
.crumb b{color:var(--foreground);font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}

/* ── 三栏骨架 ─────────────────────────────────────────────────────── */
/* 栅格几何提成变量。
   浮动入口(提问)要落在内容区的左下角而不是视口左下角 ——
   否则固定偏移会压在左侧目录栏上。把这三件事写成变量,
   断点只需改变量,不必在别处把同一组数字再算一遍。 */
:root{
  --layout-max:1440px;
  --layout-pad:1.5rem;
  --layout-pad-top:1.75rem;   /* .layout 的 padding-top,与下方规则保持一致 */
  --topbar-h:74px;            /* 实测 .topbar-inner 的高度 */
  --nav-w:15rem;
  --layout-gap:2rem;
}
.layout{
  flex:1;width:100%;max-width:1440px;margin:0 auto;padding:1.75rem 1.5rem 4rem;
  display:grid;grid-template-columns:var(--nav-w) minmax(0,1fr) 13rem;
  gap:var(--layout-gap);align-items:start;
}
.panel{
  border:1px solid var(--border);border-radius:1rem;background:var(--card);
  box-shadow:var(--shadow-sm);
}
.sticky{position:sticky;top:5rem;max-height:calc(100vh - 6.5rem);overflow-y:auto}
/* 侧栏的粘性定位必须落在 grid 项目（.aside / .rail）本身。
   之前它加在侧栏内部的 .panel 上，而该 panel 的父容器高度就等于自身高度，
   粘性元素没有任何可移动空间，于是整栏跟着页面一起滚走——
   这就是目录「被翻上去、看不到」的原因。 */
.aside,.rail{position:sticky;top:5rem;align-self:start;max-height:calc(100vh - 6.5rem);overflow-y:auto}
/* 右侧栏从右上入口的下方开始。
   入口贴视口右边缘、右侧栏也贴视口右边缘,两者争的是同一条边 ——
   实测入口正压在「本页目录」标题上。让位与贴边并不互斥:
   入口仍贴右边缘,只是右侧栏整体下移一个入口的高度加间距,标题就不再被盖住。
   sticky 的偏移必须同步下移,否则滚动时它又滑回入口下面并与之重叠。
   左侧栏不动:那里没有入口。 */
.rail{
  /* 与左栏、中栏同顶边。
     这里原有一段 margin-top:4.25rem，用来给右上角的提问入口让位；那让右栏比另外两栏低 68px，
     三栏的顶边因此不在一条线上。用户要求两栏对齐，而为浮动入口腾空间不该由内容来付。

     底部留出空间，是因为两个浮动入口现在同处右下：目录最后几项会落到它们下面，
     读到那里的人正好也会想点它们。 */
  padding-bottom:6rem;
}
.aside>.panel,.rail>.panel{box-shadow:none;border:0;background:transparent;max-height:none;overflow:visible}
.rail>.panel{border:1px solid var(--border);background:var(--card);box-shadow:var(--shadow-sm)}
/* 无法由服务端提供的图片（例如从 WPS 粘贴后残留的本地绝对路径）降级为提示，
   避免浏览器显示破图图标加一串原始路径。 */
.img-missing{
  display:inline-block;margin:.35rem 0;padding:.3rem .6rem;
  border:1px dashed var(--border);border-radius:.5rem;
  font-size:.75rem;color:var(--muted-foreground);background:color-mix(in srgb,var(--muted) 40%,transparent);
  word-break:break-all;
}

/* ── 左：目录导航（复刻 flomo 的分组列表）─────────────────────────── */
.nav{padding:.9rem .6rem 1.1rem}
.nav-title{
  font-size:.6875rem;font-weight:700;letter-spacing:.08em;text-transform:uppercase;
  color:var(--muted-foreground);padding:.55rem .6rem .35rem;
}
.nav-group{margin-bottom:.35rem}
.nav-group>.nav-head{
  display:flex;align-items:center;gap:.45rem;
  padding:.4rem .6rem;border-radius:.5rem;
  font-size:.8125rem;font-weight:600;color:var(--foreground);
  cursor:default;
}
.nav-group>.nav-head svg{width:13px;height:13px;color:var(--muted-foreground);flex:0 0 13px}
.nav-group>.nav-head .count{margin-left:auto;font-size:.6875rem;color:var(--muted-foreground);font-weight:500}
.nav-item{
  display:block;padding:.34rem .6rem .34rem 1.55rem;border-radius:.5rem;
  font-size:.8125rem;color:var(--muted-foreground);
  border-left:2px solid transparent;margin-left:.05rem;
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
}
.nav-item:hover{background:var(--accent);color:var(--foreground)}
.nav-item.on{
  color:var(--primary);background:color-mix(in srgb,var(--primary) 5%,transparent);
  border-left-color:var(--primary);font-weight:600;
}
.nav-empty{font-size:.8125rem;color:var(--muted-foreground);padding:.5rem .6rem;font-style:italic}

/* ── 中：内容 ─────────────────────────────────────────────────────── */
.content{display:flex;flex-direction:column;gap:1.25rem;min-width:0}
.card{border:1px solid var(--border);border-radius:1rem;background:var(--card);box-shadow:var(--shadow-sm)}
.card-pad{padding:1.25rem 1.4rem}
.card-title{
  display:flex;align-items:center;gap:.5rem;
  font-size:.8125rem;font-weight:700;color:var(--muted-foreground);
  letter-spacing:.02em;margin:0 0 .9rem;
}
.card-title svg{width:14px;height:14px}

/* 仪表盘 */
.stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(6.5rem,1fr));gap:.6rem}
.stat{
  border:1px solid var(--border);border-radius:.75rem;padding:.7rem .8rem;
  background:color-mix(in srgb,var(--muted) 40%,transparent);
}
.stat-num{font-size:1.35rem;font-weight:700;letter-spacing:-.02em;line-height:1.25}
.stat-label{font-size:.6875rem;color:var(--muted-foreground);margin-top:.1rem}

/* 知识图谱配色。变量名与参考实现对齐（--graph-main / --graph-muted / --graph-label），
   客户端脚本经探针元素解析后交给 d3 绘制的 SVG 使用；服务端静态 SVG 直接吃同一批变量。

   为什么不再映射到 --c3/--c4：映射后节点与博客底色同属暖灰，图谱整体糊成一片，
   丢了参考实现那种「浅底 + 中灰节点 + 一条深色高亮」的清爽对比。

   连线宽度 1.8px 是刻意偏离参考实现的 1px —— 读者反馈参考那种细度几乎看不见结构。
   不透明度交给脚本逐帧控制（hover 分级 + lerp），这里不设固定值，
   否则会像上一版那样被 .graphcard 规则一路继承下来反而更淡。 */
:root{
  --graph-main:#4b7a43;
  --graph-muted:#999999;
  --graph-label:#8e8e90;
  /* 连线的静止色。脚本按 active 在主色与它之间切换；CSS 只给宽度，
     因为 CSS 的 stroke 会覆盖脚本写在属性上的值。 */
  --graph-edge:#c2c2c2;
}
.graph-wrap{border:1px solid var(--border);border-radius:.75rem;overflow:hidden;background:var(--background)}
.graph-wrap svg{display:block;width:100%;height:auto}
/* 只给宽度,颜色交给脚本。
   CSS 的 stroke 会覆盖脚本写在属性上的值 —— 与标签 opacity 是同一类陷阱:
   脚本按 active 把边染成主色,而这里的 stroke 会把它顶回灰色,悬停就没有任何反馈。
   深色模式的 .g-edge 规则同理,只保留宽度相关。 */
.g-edge{stroke-width:1.8;stroke-linecap:round}
.g-node circle{fill:var(--graph-muted);stroke:none}
/* 命中区：透明、不绘制，只为把可悬停范围从 13px 的圆点扩到约 26px，
   否则读者必须像素级对准才能触发高亮。 */
.g-node .g-node-hit{fill:transparent;stroke:none}
/* 当前笔记：参考实现额外画一个 r+1.5 的描边环，只描边不填充。
   必须用 :not() 把环排除在下面的实心填充之外 —— 否则 .g-node.focus circle 的
   特异性高于 .g-node .g-node-ring，环会被填成实心，看起来就是一个更大的点。 */
.g-node .g-node-ring{fill:none;stroke:var(--graph-main);stroke-width:1.2}
.g-node.focus circle:not(.g-node-ring){fill:var(--graph-main)}
.g-node text,.g-node .g-node-label{
  /* 10.5px 在 384px 宽的卡片里偏大;参考实现是 16px 配 scale(1/3),即实际约 5.3px。
     这里取 8px —— 比参考略大(卡片也比参考宽),但不再让放大后的标签挤成一片。 */
  font-size:8px;fill:var(--graph-label);pointer-events:none;
  paint-order:stroke;stroke:var(--background);stroke-width:3px;stroke-linejoin:round;
  font-family:ui-sans-serif,system-ui,"PingFang SC","Microsoft YaHei",sans-serif;
  pointer-events:none;
}
/* .lit 曾用 --primary 填充，但每个节点都是已发布，所以它没有任何区分作用，
   只剩一个与图谱配色无关的绿。去掉，只留 hover 反馈。 */
.g-node a:hover circle{fill:var(--graph-main)}
.graph-note{font-size:.75rem;color:var(--graph-label);margin-top:.6rem}

/* 标签云 */
.tags{display:flex;flex-wrap:wrap;gap:.4rem}
.tag{
  display:inline-flex;align-items:center;gap:.3rem;
  padding:.2rem .55rem;border-radius:.5rem;border:1px solid var(--border);
  font-size:.75rem;color:var(--muted-foreground);background:color-mix(in srgb,var(--muted) 35%,transparent);
}
.tag:hover{color:var(--foreground);border-color:var(--c4)}
.tag i{font-style:normal;font-size:.6875rem;color:var(--c4)}
.tag.s1{font-size:.8125rem}
.tag.s2{font-size:.875rem;font-weight:600;color:var(--foreground)}
.tag.s3{font-size:.9375rem;font-weight:700;color:var(--foreground)}

/* 笔记列表 */
.list{display:flex;flex-direction:column}
.item{
  display:block;padding:.95rem .2rem;border-bottom:1px solid var(--border);
  transition:padding-left .18s ease;
}
.item:last-child{border-bottom:0}
.item:hover{padding-left:.55rem}
.item h3{margin:0 0 .3rem;font-size:1.0625rem;font-weight:600;letter-spacing:-.01em;line-height:1.45}
.item:hover h3{color:var(--primary)}
.item p{margin:0;font-size:.8125rem;color:var(--muted-foreground);line-height:1.7;
  display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.item-meta{display:flex;flex-wrap:wrap;gap:.85rem;margin-top:.45rem;font-size:.6875rem;color:var(--c3)}
.item-meta span{display:inline-flex;align-items:center;gap:.25rem}
.item-meta svg{width:11px;height:11px}

/* ── 右：目录 / 侧栏 ──────────────────────────────────────────────── */
.rail{display:flex;flex-direction:column;gap:1.25rem}
.toc{padding:.9rem .5rem 1rem}
.toc a{
  /* 底部 padding 比顶部大，是给下划线留位置。
     <a> 必须 overflow:hidden 才能让长标题单行截断，而 overflow 同时会裁掉任何越出盒子的
     ::after —— 下划线因此要么落在盒内，要么根本看不见。这 .22rem 就是它待的地方。 */
  display:block;padding:.32rem .6rem .5rem 1.15rem;
  font-size:.8125rem;color:var(--muted-foreground);border-radius:.5rem;
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
  /* 菱形与下划线都相对它定位。 */
  position:relative;
}

/* 当前项的菱形标记。
   用旋转 45° 的方块画，而不是 <span>◆</span> 这样的字符：
   菱形的字符宽度随字体而变，垂直位置依赖基线，而这是要精确对齐到文字行中心的东西。
   一个方块加 transform 在任何字体下都在同一个位置。

   空心而不是实心：目录项本身已经用颜色和字重表示「当前」，
   再加一个实心块会让整行显得比它实际的重要。 */
.toc a::before{
  content:"";position:absolute;
  left:.34rem;top:.72rem;
  width:.4rem;height:.4rem;
  border:1.5px solid currentColor;border-radius:1px;
  /* 从缩小状态长出来，配合旋转 —— 见下面的过渡。 */
  transform:rotate(45deg) scale(.2);
  opacity:0;
  transition:transform .34s cubic-bezier(.22,1,.36,1),opacity .22s ease;
}
.toc a.on::before{transform:rotate(45deg) scale(1);opacity:1}
/* 悬停时给出一个淡的预览，让读者知道菱形是「指向」而不是「已选中」。 */
.toc a:hover::before{opacity:.38;transform:rotate(45deg) scale(.8)}

/* 下划线。
   挂在包住文字的那个 span 上，而不是挂在 <a> 上：<a> 是块级、撑满整行，
   而读者看到的下划线应当只属于文字本身。挂在 <a> 上会让它横贯整行，
   与文字长度无关 —— 一个四字标题和一行二十字的标题得到一样长的线。

   从左侧以 scaleX 从 0 长到 1：水平伸展与「这一项被指向了」的方向一致。 */
.toc a .t{
  position:relative;display:inline;
}
.toc a .t::after{
  /* 正数：落在文字下方、但仍在 <a> 的底部 padding 之内，因此不会被 overflow:hidden 裁掉。 */
  content:"";position:absolute;left:0;right:0;bottom:-.3rem;height:2px;
  border-radius:2px;
  background:currentColor;opacity:.7;
  transform:scaleX(0);transform-origin:left center;
  transition:transform .26s cubic-bezier(.22,1,.36,1),opacity .26s ease;
}
.toc a:hover{background:var(--accent);color:var(--foreground)}
.toc a:hover .t::after{transform:scaleX(1);opacity:.45}
.toc a.on{color:var(--primary);background:color-mix(in srgb,var(--primary) 5%,transparent);font-weight:600}
.toc a.on .t::after{transform:scaleX(1);opacity:.9}
/* 键盘可达性：Tab 到某一项时同样展开，否则只有鼠标用户能看到这个反馈。 */
.toc a:focus-visible .t::after{transform:scaleX(1);opacity:.9}
.toc a.lv3{padding-left:1.9rem;font-size:.78125rem}
.toc a.lv3::before{left:1.09rem}

@media (prefers-reduced-motion:reduce){
  .toc a::before,.toc a .t::after{transition:none}
}
.toc-empty{font-size:.75rem;color:var(--muted-foreground);padding:.4rem .6rem;font-style:italic}

/* ── 文章页正文：完全对齐分享页的排版节奏 ────────────────────────── */
.post{padding:2.5rem 2.75rem 3.25rem;max-width:900px;margin:0 auto;width:100%}
.post h1.pt{font-size:1.75rem;font-weight:700;letter-spacing:-.02em;line-height:1.35;margin:0 0 .5rem}
.post-meta{
  display:flex;flex-wrap:wrap;gap:.9rem;font-size:.75rem;color:var(--muted-foreground);
  padding-bottom:1.4rem;margin-bottom:1.6rem;border-bottom:1px solid var(--border);
}
.post-body{font-size:1rem}
/* 标题六级：字号严格递减，且最小一级 h6(17px) 仍大于正文(16px) 与表格(14px)。
   此前 h1/h2 合用一档(1.25rem)、h4 与正文同号(1rem)，而 h5/h6 完全没有定义，
   于是落回浏览器默认的 0.83em / 0.67em（≈13.3px / 10.7px），比正文还小。
   这里把六级显式铺满，并用字重与上边距同步递减来强化层级。 */
.post-body h1{font-size:1.5rem;font-weight:700;letter-spacing:-.02em;margin:2.2rem 0 .8rem;scroll-margin-top:7rem}
.post-body h2{font-size:1.375rem;font-weight:700;letter-spacing:-.015em;margin:2rem 0 .75rem;scroll-margin-top:7rem}
.post-body h3{font-size:1.25rem;font-weight:600;letter-spacing:-.01em;margin:1.8rem 0 .65rem;scroll-margin-top:7rem}
.post-body h4{font-size:1.1875rem;font-weight:600;margin:1.5rem 0 .55rem;scroll-margin-top:7rem}
.post-body h5{font-size:1.125rem;font-weight:600;margin:1.3rem 0 .5rem;scroll-margin-top:7rem}
.post-body h6{font-size:1.0625rem;font-weight:600;margin:1.2rem 0 .45rem;scroll-margin-top:7rem}
.post-body p{margin:.75rem 0;line-height:1.75;color:color-mix(in srgb,var(--foreground) 90%,transparent)}
.post-body ul,.post-body ol{margin:.75rem 0;padding-left:1.5rem}
.post-body li{margin:.375rem 0;line-height:1.75}
.post-body blockquote{
  margin:1.1rem 0;padding:.2rem 0 .2rem 1rem;
  border-left:3px solid var(--c4);color:var(--muted-foreground);
}
.post-body hr{border:0;border-top:1px solid var(--border);margin:2rem 0}
.post-body img{margin:.6rem 0}
.post-body code{
  font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;font-size:.875em;
  background:var(--muted);padding:.12em .38em;border-radius:.25rem;
}
.post-body pre{
  background:var(--muted);border:1px solid var(--border);border-radius:.75rem;
  padding:.9rem 1.05rem;overflow-x:auto;font-size:.8125rem;line-height:1.65;
}
.post-body pre code{background:none;padding:0}
/* 表格：宽表在窄栏里会撑破卡片（此前正是因为缺少横向滚动，右侧列被直接裁掉，
   连带其中的图片一起看不见）。改为表格自身横向滚动，并给单元格设最小宽度，
   既不会溢出容器，也不会把多列表格挤成一条。 */
.post-body table{
  display:block;width:100%;max-width:100%;
  overflow-x:auto;overscroll-behavior-x:contain;
  border-collapse:collapse;margin:1rem 0;font-size:.875rem;
}
.post-body th,.post-body td{
  border:1px solid var(--border);padding:.45rem .6rem;text-align:left;
  min-width:6rem;word-break:break-word;vertical-align:top;
}
.post-body th{background:var(--muted);font-weight:600}
/* 单元格里的图片：解除单元格最小宽度对图片的挤压，避免图片消失 */
.post-body td img,.post-body th img{max-width:100%;height:auto;display:block;margin:.3rem 0}
/* 内链：站内跳转，用主色下划线区分 */
.post-body a[href^="/blog/"]{
  color:var(--primary);font-weight:500;
  border-bottom:1px solid color-mix(in srgb,var(--primary) 35%,transparent);
}
.post-body a[href^="/blog/"]:hover{border-bottom-color:var(--primary)}
/* 外链：加小箭头并新窗口打开 */
.post-body a[href^="http"]::after{
  content:"↗";font-size:.75em;margin-left:.15em;color:var(--muted-foreground);
}
.post-body a[href^="http"]{border-bottom:1px solid var(--border)}
.post-body a[href^="http"]:hover{border-bottom-color:var(--c3)}

/* ── 页脚 ─────────────────────────────────────────────────────────── */
.footer{
  border-top:1px solid var(--border);background:color-mix(in srgb,var(--muted) 35%,transparent);
  padding:1.1rem 1.5rem;text-align:center;
  font-size:.75rem;color:var(--muted-foreground);
}
.footer a{font-weight:600;color:var(--primary)}
.footer a:hover{text-decoration:underline}
/* 备案号:法定必须展示,且要能点进工信部查询页。刻意比正文更淡——它是义务,
   不是设计元素;但必须可读,所以只用 muted 色而不再降低不透明度。 */
.footer-legal{margin-top:.5rem;font-size:.7rem;letter-spacing:.01em}
.footer-legal a{font-weight:500;color:var(--muted-foreground)}
.footer-legal a:hover{color:var(--primary)}

/* ── 左：可折叠的嵌套目录树 ──────────────────────────────────────────
   用原生 <details>/<summary>，折叠与嵌套都不需要 JavaScript。 */
.nav-folder{margin:.05rem 0}
.nav-folder>summary{
  display:flex;align-items:center;gap:.4rem;
  padding:.34rem .5rem;border-radius:.5rem;
  font-size:.8125rem;font-weight:600;color:var(--foreground);
  cursor:pointer;list-style:none;user-select:none;
}
.nav-folder>summary::-webkit-details-marker{display:none}
.nav-folder>summary::before{
  content:"";width:0;height:0;flex:0 0 0;margin-right:.15rem;
  border-left:4px solid var(--c3);
  border-top:3.5px solid transparent;border-bottom:3.5px solid transparent;
  transition:transform .18s ease;
}
.nav-folder[open]>summary::before{transform:rotate(90deg)}
.nav-folder>summary:hover{background:var(--accent)}
.nav-folder>summary svg{width:13px;height:13px;color:var(--muted-foreground);flex:0 0 13px}
.nav-folder>summary .nav-name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.nav-folder>summary .count{
  margin-left:auto;font-size:.6875rem;color:var(--muted-foreground);font-weight:500;
}
.nav-children{margin-left:.75rem;padding-left:.5rem;border-left:1px solid var(--border)}
/* 树内的叶子不再需要固定大缩进，缩进由层级本身提供 */
.nav-children .nav-item{padding-left:.5rem;margin-left:0}

/* ── 右：活跃度热力图 ─────────────────────────────────────────────── */
.act{padding:.9rem 1rem 1.1rem}
.act-stats{display:flex;gap:1.1rem;margin:.15rem 0 .85rem}
.act-stats>div{display:flex;flex-direction:column}
.act-stats b{font-size:1.0625rem;font-weight:700;letter-spacing:-.01em;line-height:1.25}
.act-stats span{font-size:.625rem;color:var(--muted-foreground)}
.heat{
  display:grid;grid-auto-flow:column;grid-auto-columns:1fr;
  grid-template-rows:repeat(7,1fr);gap:2px;
}
.hc{display:block;aspect-ratio:1/1;border-radius:2px;background:var(--muted)}
.hc.lv0{background:color-mix(in srgb,var(--muted) 70%,transparent)}
.hc.lv1{background:color-mix(in srgb,var(--primary) 26%,transparent)}
.hc.lv2{background:color-mix(in srgb,var(--primary) 48%,transparent)}
.hc.lv3{background:color-mix(in srgb,var(--primary) 72%,transparent)}
.hc.lv4{background:var(--primary)}
.heat-months{
  display:grid;grid-auto-flow:column;grid-auto-columns:1fr;gap:2px;margin-top:.4rem;
}
.heat-months i{font-style:normal;font-size:.5625rem;color:var(--c4);white-space:nowrap}
.act-note{margin:.7rem 0 0;font-size:.625rem;line-height:1.65;color:var(--c3)}

/* ── 知识图谱：入场动画与标签显隐 ──────────────────────────────────────
   这里必须区分两种 SVG,否则 CSS 会盖掉脚本写在属性上的透明度：

   - 服务端静态 SVG（无 .graph-svg 类，无 JavaScript 时可见）：标签常显，
     因为一张不会动的图如果不标名字，读者无从知道哪个点是什么。
   - 客户端交互图（有 .graph-svg 类）：标签的 opacity 完全由脚本按缩放控制。
     若这里的规则也命中它，CSS 的 opacity 会覆盖属性值，
     结果是标签无视缩放始终显示 —— 那正是「文字挤在一起」的成因。 */
svg:not(.graph-svg) .g-node{animation:g-in .45s ease backwards}
svg:not(.graph-svg) .g-node text{opacity:.92;transition:opacity .18s ease}
svg:not(.graph-svg) .g-node:hover text{opacity:1}
.g-node a{cursor:pointer}
@keyframes g-in{from{opacity:0}to{opacity:1}}
@media (prefers-reduced-motion:reduce){
  svg:not(.graph-svg) .g-node{animation:none}
  svg:not(.graph-svg) .g-node text{transition:none}
}

/* ── 响应式 ───────────────────────────────────────────────────────── */
/* 此前这里用 display:none 收掉侧栏：<=1180px 干掉 .rail（活跃度日历／本页目录），
   <=860px 再干掉 .aside（导航树），结果手机上这三块组件整体消失。
   改为把三栏网格降级成「正文在上、侧栏依次堆叠在下」：组件始终可达，
   阅读区仍排在第一位。 */
@media (max-width:1180px){
  :root{--nav-w:14rem}
  .layout{grid-template-columns:14rem minmax(0,1fr)}
  /* 右栏跨满整行，落到正文所在行的下方 */
  .rail{
    grid-column:1/-1;order:3;
    position:static;max-height:none;overflow:visible;
    /* 右侧栏不再占据右侧一列,右上入口也就不在它上方,让位的上边距要撤掉。 */
    margin-top:0;
  }
  .rail>.panel{position:static}
}
@media (max-width:860px){
  /* 单列时目录栏落到正文下方,不再占据左侧,因此入口贴回视口左下。 */
  :root{--layout-pad:.9rem;--nav-w:0rem;--layout-gap:0rem;--layout-pad-top:1rem}
  .layout{
    grid-template-columns:minmax(0,1fr);
    padding:1rem .9rem 3rem;gap:1.25rem;
  }
  /* 单列顺序按页面类型区分。首页的正文是近百篇的长列表，若把日历与目录树
     排在它后面，等于换个方式藏起来——所以首页改为「日历 → 目录树 → 正文」；
     文章页仍以正文为先，目录树与本页目录随后。 */
  .layout-home .rail{order:1}
  .layout-home .aside{order:2}
  .layout-home .content{order:3}
  .layout-post .content{order:1}
  .layout-post .aside{order:2}
  .layout-post .rail{order:3}
  .aside,.rail{
    grid-column:1/-1;
    position:static;
  }
  /* 侧栏不再吸顶。目录树给一个上限并保留内部滚动，否则十来个来源目录
     的整棵树会占满数屏，把正文压到最底下。 */
  .aside{max-height:52vh;overflow-y:auto}
  .rail{max-height:none;overflow:visible;margin-top:0}
  .aside>.panel,.rail>.panel{position:static}
  .post{padding:1.5rem 1.25rem 2.5rem}
  .topbar-inner{padding:.6rem 1rem}
  .brand-sub{display:none}
  /* 首页顶栏在窄屏只剩一行：品牌 + 顶栏入口 + 「最近更新」塞不下。
     与阅读页隐藏整个顶栏是同一条判断——「你在哪」不如「能去哪」重要。
     差别在于首页顶栏同时承载品牌与入口，所以只收掉面包屑这一格，而不是整条收掉。 */
  body.is-home .crumb{display:none}
}
/* ── 错误页（404 / 未公开 / 站点关闭 / 渲染失败）────────────────────── */
/* 与首页、文章页共用 .shell / .topbar / .footer 骨架；居中卡片，复用既有令牌与动效语汇。 */
.errwrap{flex:1;display:flex;align-items:center;justify-content:center;padding:3rem 1.5rem 4rem}
.errcard{max-width:34rem;width:100%;text-align:center;padding:2.6rem 2rem 2.2rem}
.errcode{font-size:.6875rem;font-weight:700;letter-spacing:.14em;text-transform:uppercase;color:var(--muted-foreground)}
.errtitle{font-size:1.375rem;font-weight:700;letter-spacing:-.015em;line-height:1.4;margin:.85rem 0 .6rem}
.errmsg{font-size:.9375rem;line-height:1.75;color:var(--muted-foreground);margin:0;
  color:color-mix(in srgb,var(--foreground) 72%,transparent)}
.errhints{list-style:none;margin:1.3rem 0 0;padding:0;display:flex;flex-direction:column;gap:.45rem;text-align:left}
.errhints li{position:relative;padding-left:1.05rem;font-size:.8125rem;line-height:1.7;color:var(--muted-foreground)}
.errhints li::before{content:"";position:absolute;left:.3rem;top:.72em;width:4px;height:4px;border-radius:50%;background:var(--c4)}
.erracts{margin-top:1.7rem;display:flex;gap:.6rem;justify-content:center;flex-wrap:wrap}
.errbtn{display:inline-flex;align-items:center;gap:.4rem;padding:.5rem .95rem;border:1px solid var(--border);
  border-radius:.5rem;background:var(--card);color:var(--foreground);font-size:.8125rem;font-weight:600;
  transition:border-color .18s ease,background .18s ease,transform .18s ease}
.errbtn:hover{border-color:color-mix(in srgb,var(--primary) 45%,var(--border));background:var(--accent);transform:translateY(-1px)}
.errbtn:active{transform:scale(.97)}
@media (max-width:860px){
  .errwrap{padding:2rem 1rem 3rem}
  .errcard{padding:2rem 1.25rem 1.8rem}
  .errtitle{font-size:1.25rem}
}
@media print{.errwrap{padding:0;display:block}}
@media (prefers-reduced-motion:reduce){.errbtn{transition:none}.errbtn:hover{transform:none}}

/* ── Callouts / Admonitions（服务端渲染，无 JS 依赖）────────────────── */
/* 色系刻意只有四族：中性（note）、提示（tip）、警示（warn）、危险（danger）。
   博客调色板是暖灰加少量强调色，每多一种色相都会让页面显得更廉价。 */
.callout{margin:1.1rem 0;border:1px solid var(--border);border-radius:.75rem;overflow:hidden;
  background:color-mix(in srgb,var(--muted) 30%,transparent)}
.callout-head{display:flex;align-items:baseline;gap:.5rem;padding:.6rem .85rem;
  border-bottom:1px solid var(--border);background:color-mix(in srgb,var(--muted) 45%,transparent)}
.callout-ico{display:inline-flex;align-items:center;justify-content:center;width:1.05rem;height:1.05rem;
  flex:0 0 1.05rem;border-radius:50%;font-size:.6875rem;font-weight:700;line-height:1;
  background:color-mix(in srgb,var(--primary) 14%,transparent);color:var(--primary)}
.callout-title{font-size:.8125rem;font-weight:700;letter-spacing:.01em;color:var(--foreground)}
.callout-body{padding:.15rem .85rem .5rem}
.callout-body>:first-child{margin-top:.6rem}
.callout-body>:last-child{margin-bottom:.6rem}
.callout-body details{margin:.5rem 0}
.callout-body summary{font-size:.8125rem;font-weight:600;cursor:pointer;padding:.2rem 0}
.callout-body summary::marker{color:var(--c4)}
/* 色系：只改图标与头部左侧的强调条，不改正文颜色——正文始终是正文。 */
.callout-tip .callout-ico{background:color-mix(in srgb,var(--primary) 18%,transparent)}
.callout-warn{border-color:color-mix(in srgb,#c2503f 22%,var(--border))}
.callout-warn .callout-head{background:color-mix(in srgb,#c2503f 7%,transparent);
  border-bottom-color:color-mix(in srgb,#c2503f 18%,var(--border))}
.callout-warn .callout-ico{background:color-mix(in srgb,#c2503f 16%,transparent);color:#c2503f}
.callout-danger{border-color:color-mix(in srgb,#c2503f 34%,var(--border))}
.callout-danger .callout-head{background:color-mix(in srgb,#c2503f 12%,transparent);
  border-bottom-color:color-mix(in srgb,#c2503f 26%,var(--border))}
.callout-danger .callout-ico{background:#c2503f;color:#fff}
.callout-quote .callout-body>:first-child{font-style:italic}
/* 高亮 ==text==：唯一允许使用黄色的位置，且只作为文本底色，不参与界面配色。 */
.post-body mark{background:color-mix(in srgb,#f5d76e 55%,transparent);color:inherit;
  padding:.05em .15em;border-radius:.2rem}
/* 脚注（goldmark extension.Footnote 的产物） */
.post-body .footnotes{margin-top:2.2rem;padding-top:1.1rem;border-top:1px solid var(--border);
  font-size:.8125rem;color:var(--muted-foreground)}
.post-body .footnotes hr{display:none}
.post-body .footnotes ol{padding-left:1.2rem}
.post-body .footnotes li{margin:.3rem 0;line-height:1.7}
.post-body sup a{text-decoration:none;font-weight:600}
@media (max-width:860px){.callout-body{padding-left:.7rem;padding-right:.7rem}}
@media print{.callout{break-inside:avoid}}

/* ── 数学公式（LaTeX 源码 + 可选 MathJax 排版）──────────────────────── */
/* 服务端不排版公式（Go 生态没有生产级 TeX→HTML 渲染器），只保留源码并加标记。
   文章页仅在出现 .math-* 时加载 MathJax；无 JS 时下面的样式让 LaTeX 源码本身可读。 */
.math-block{margin:1.1rem 0;padding:.9rem 1.05rem;border:1px solid var(--border);border-radius:.75rem;
  background:color-mix(in srgb,var(--muted) 30%,transparent);overflow-x:auto;
  font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;
  font-size:.875rem;line-height:1.7;text-align:center;color:var(--foreground)}
.math-inline{padding:.05em .2em;border-radius:.25rem;
  background:color-mix(in srgb,var(--muted) 45%,transparent);
  font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;
  font-size:.9375em;color:var(--foreground)}
/* MathJax 接管后，去掉源码态的底色与等宽字体 */
mjx-container{overflow-x:auto;overflow-y:hidden;max-width:100%}
mjx-container[display="true"]{margin:.9rem 0}
.math-block:has(mjx-container){padding:0;border:0;background:none;text-align:center}
.math-inline:has(mjx-container){padding:0;background:none}
@media print{.math-block{break-inside:avoid}}

/* ── 笔记嵌入（Obsidian 的嵌入语法）────────────────────────────────── */
/* 复用 callout 的视觉语言，但用虚线边框与略小的字号表示「这是引用来的内容」。
   只有已发布的笔记可被嵌入；未发布目标在服务端就降级为纯文本，不会到达这里。 */
.callout[data-callout="note"] .callout-body .callout{border-style:dashed}
.callout-body>.callout{margin:.7rem 0}
.callout-body .callout-body{font-size:.9375rem}
@media print{.callout{break-inside:avoid}}

/* ── 文章内知识图谱（收起 / 展开两态）─────────────────────────────────── */
/* 停靠方式已改:此前图谱是正文旁边的第二列(grid 的 21.5rem),为给它腾位置而压窄每一段——
   读者是来读文章的,这张图却在每一行上收他们的宽度。现在它脱离文档流,正文保持图谱出现之前的宽度。

   开合动效逐条复刻提问面板（见本文件 知识库问答 区块的 .ask-fab/.ask-panel/.ask-backdrop）:
   同一页面上两个浮动面板若用两套动效语言,读者会把差异读成故障,而不是风格。 */
/* 右下角有两个浮动入口:提问与图谱。实测它们位置完全重合,提问的按钮盖住了图谱的,
   于是图谱入口点不到。
   参考实现用 .floating-actions 的 column-reverse 把右下角按钮堆成一列;
   这里不引入新容器(避免改动已验证的提问布局),而是让图谱这一层落到提问之上 ——
   提问按钮高度约 49px 加上 1.5rem 的下边距,再留一点间隙。 */
#graph-root{--gr-z:41}
.graph-fab{
  position:fixed;
  right:1.5rem;
  bottom:1.25rem;z-index:var(--gr-z);
  display:inline-flex;align-items:center;gap:.5rem;
  padding:.7rem 1.05rem .7rem .85rem;
  border:1px solid var(--border);border-radius:999px;
  background:color-mix(in srgb,var(--card) 88%,transparent);
  backdrop-filter:blur(12px);-webkit-backdrop-filter:blur(12px);
  box-shadow:var(--shadow-md,0 8px 30px rgba(0,0,0,.12));
  color:var(--foreground);font:inherit;font-size:.875rem;font-weight:600;
  cursor:pointer;
  transition:transform .22s cubic-bezier(.22,1,.36,1),box-shadow .22s ease,background .22s ease;
}
.graph-fab:hover{transform:translateY(-2px);background:var(--card);box-shadow:0 14px 40px rgba(0,0,0,.16)}
.graph-fab:active{transform:translateY(0) scale(.97)}
.graph-fab-ico{display:inline-flex;width:1.15rem;height:1.15rem;color:var(--graph-main)}
.graph-fab-ico svg{width:100%;height:100%}
#graph-root.gr-on .graph-fab{opacity:0;pointer-events:none;transform:scale(.9) translateY(6px)}

.graph-backdrop{
  position:fixed;inset:0;z-index:calc(var(--gr-z) - 1);
  background:color-mix(in srgb,var(--foreground) 22%,transparent);
  backdrop-filter:blur(2px);-webkit-backdrop-filter:blur(2px);
  opacity:0;transition:opacity .24s ease;
}
#graph-root.gr-in .graph-backdrop{opacity:1}

.graph-panel{
  position:fixed;z-index:var(--gr-z);
  right:1.25rem;bottom:1.25rem;
  width:24rem;max-width:calc(100vw - 2.5rem);
  display:flex;flex-direction:column;
  border:1px solid var(--border);border-radius:1.15rem;
  background:var(--card);box-shadow:0 24px 70px rgba(0,0,0,.22);
  padding:.7rem;
  opacity:0;transform:translateY(16px) scale(.975);transform-origin:100% 100%;
  transition:opacity .34s ease,transform .44s cubic-bezier(.22,1,.36,1);
}
#graph-root.gr-in .graph-panel{opacity:1;transform:none}
/* [hidden] 的 display:none 会被上面 .graph-panel 的 display:flex 盖掉。
   少了这一条,面板收起后仍以 opacity:0 占据右下角并参与命中测试。
   提问面板踩过同一个坑,见其注释。 */
.graph-panel[hidden],.graph-backdrop[hidden]{display:none}

/* 服务端静态 SVG 的容器边框（无 JavaScript 时的降级外观）。 */
.graphcard{border:1px solid var(--border);border-radius:.75rem;background:var(--card);
  padding:.6rem;overflow:hidden}
.graphcard .graph-wrap{border:0;border-radius:.5rem;background:color-mix(in srgb,var(--background) 96%,var(--muted))}
/* 交互画布:客户端力模拟绘制的那张图。给出固定高度,使卡片在模拟收敛过程中不跳动 ——
   尺寸由布局决定的图形会在稳定过程中不断改变高度,那看起来像页面在抖。 */
#graph-canvas{height:18rem}
#graph-canvas .graph-svg{display:block;width:100%;height:100%;touch-action:none}
/* 可拖动的节点给出抓取光标 —— 这是这张图「可交互」唯一的静态提示。 */
.graph-svg .g-node{cursor:grab}
.graph-svg .g-node.is-dragging{cursor:grabbing}
.graph-svg .g-node text{user-select:none}
@media (prefers-reduced-motion:reduce){.graph-svg .g-node{transition:none}}

/* 工具栏改为正常流,位于图表之上。
   它原本是 position:absolute 浮在图上,实测会压住顶部节点的圆点与标签 ——
   参考实现的 #graph-controls 就放在图表上方的标题栏里,不覆盖任何内容。
   半透明白 + 毛玻璃的观感保留。 */
.graphcard-tools{position:relative;z-index:2;margin:0 .1rem .5rem;
  display:flex;align-items:center;gap:.35rem;
  padding:.3rem .5rem .3rem .65rem;border-radius:999px;
  background:color-mix(in srgb,var(--card) 82%,transparent);
  backdrop-filter:blur(10px);-webkit-backdrop-filter:blur(10px);
  border:1px solid var(--border);
  box-shadow:var(--shadow-sm)}
.graphcard-depth{display:flex;align-items:center;gap:.45rem;min-width:0;flex:1}
.graphcard-depth-t{font-size:.75rem;color:var(--muted-foreground);white-space:nowrap}
.graphcard-depth-t b{color:var(--foreground);font-weight:700}
.graphcard-slider{-webkit-appearance:none;appearance:none;flex:1;min-width:3rem;height:4px;
  border-radius:999px;background:color-mix(in srgb,var(--foreground) 18%,transparent);outline:none}
.graphcard-slider::-webkit-slider-thumb{-webkit-appearance:none;width:1.05rem;height:1.05rem;border-radius:50%;
  background:var(--card);border:1.5px solid color-mix(in srgb,var(--foreground) 22%,transparent);
  box-shadow:0 1px 3px #00000026;cursor:pointer}
.graphcard-slider::-moz-range-thumb{width:1.05rem;height:1.05rem;border-radius:50%;
  background:var(--card);border:1.5px solid color-mix(in srgb,var(--foreground) 22%,transparent);
  box-shadow:0 1px 3px #00000026;cursor:pointer}
.graphcard-slider:disabled{opacity:.55}
.graphcard-slider:disabled::-webkit-slider-thumb{cursor:default}
.graphcard-acts{display:flex;align-items:center;gap:.15rem}
/* 图标刻意比文字更浅:它们是次要操作,不应与「深度」争夺注意力。 */
.graphcard-icon{display:inline-flex;align-items:center;justify-content:center;width:1.55rem;height:1.55rem;
  border:0;border-radius:.5rem;background:transparent;padding:0;cursor:pointer;
  color:color-mix(in srgb,var(--muted-foreground) 78%,transparent);
  transition:background .18s ease,color .18s ease}
.graphcard-icon svg{width:1rem;height:1rem}
.graphcard-icon:hover{background:var(--accent);color:var(--foreground)}
.graphcard-icon:active{transform:scale(.94)}

/* ── 图谱配色 ──────────────────────────────────────────────────────────
   直接用参考图实测的色值,不再映射到博客的 --c3/--c4。

   为什么放弃映射:映射后节点与博客底色同色系(都是暖灰),图谱整体糊成一片,
   失去了参考图那种「浅底 + 中灰节点 + 一条深绿」的清爽对比。参考图是冷灰,
   与博客的暖灰并置时本来就该分开看 —— 图谱是一个内嵌的可视化组件,不是正文的一部分。

   实测色值:节点 #999999 实心 / 高亮 #4b7a43 / 标签 #8e8e90。
   连线原图为 1px #d3d3d3,在近白背景上过淡(读者几乎看不见图的结构),
   故加粗到 1.8px 并提到 #c2c2c2 —— 保留冷灰性质,只补足对比。 */
/* 图谱配色只在上面的 :root + 基础规则里定义一次。这一段曾经把同样的颜色又写了一遍，
   于是同一份真相有两个副本，其中一份还漏掉了不透明度重置 —— 线因此一直是半透明的。
   现在这里只保留无法由变量表达的两项：标签字重与停靠卡片的说明文字。 */
.graphcard .g-node.focus text,.graphlayer .g-node.focus text{font-weight:600;fill:var(--foreground)}
.graphcard .graph-note{font-size:.75rem;color:var(--graph-label);margin:.55rem .25rem .1rem;line-height:1.6}

/* 深色模式:参考实现没有深色版本,以下色值由本主题自定,原则与浅色一致 ——
   节点中灰、连线略淡于节点、高亮仍是绿。只覆盖变量,规则本身不重复。 */
@media (prefers-color-scheme:dark){
  :root{
    --graph-main:#7aa870;
    --graph-muted:#8c8c8c;
    --graph-label:#9a9a9a;
  }
  /* 边色由脚本控制,深色模式只改节点与标签的变量。 */
}

@media (max-width:1180px){
  /* 中窄屏:停靠卡片会让正文可读区太窄,因此改为可拖动的右下角小尺寸,
     而不是把它塞回文档流——一旦回到流里,文字就又要为它让宽度。 */
  .graph-panel{width:19rem}
  .graph-fab{right:.9rem;bottom:.9rem}
}
@media (max-width:860px){
  /* 移动端:整个停靠卡片消失。在手机上把这张图塞进浮动面板比直接滑过去更糟,
     而且它会长期盖住正文——这正是本轮要解决的问题本身。 */
  #graph-root{display:none}
}
@media print{#graph-root{display:none}}

/* ── 图谱全屏浮层（桌面端专用）────────────────────────────────────── */
/* 移动端(<=860px)完全禁用:在手机上把这张图塞进浮层比直接滑过去更糟,
   因此不是「更小的浮层」而是「没有浮层」——控件在此宽度下直接隐藏。 */
@media (min-width:861px){
  .graphlayer{position:fixed;inset:0;z-index:44;display:flex;align-items:center;justify-content:center;
    padding:4vmin;
    /* 背景毛玻璃:后面的页面内容被虚化而不是被遮死,读者仍知道自己在哪一页。 */
    background:color-mix(in srgb,var(--background) 62%,transparent);
    backdrop-filter:blur(14px) saturate(120%);
    -webkit-backdrop-filter:blur(14px) saturate(120%)}
  .graphlayer[hidden]{display:none}
  .graphlayer-panel{width:min(96vw,80rem);max-height:92vh;display:flex;flex-direction:column;
    border:1px solid var(--border);border-radius:1rem;background:var(--card);overflow:hidden;
    box-shadow:0 24px 64px -12px #00000040,0 4px 16px -4px #00000026;
    transform:scale(.94);opacity:0}
  /* 平滑放大:从 .94 到 1,配合居中定位,视觉上像是卡片长大而不是新页面弹出。 */
  .graphlayer.is-in .graphlayer-panel{transform:scale(1);opacity:1;
    transition:transform .34s cubic-bezier(.22,1,.36,1),opacity .22s ease}
  .graphlayer-head{display:flex;align-items:center;justify-content:space-between;gap:1rem;
    padding:.85rem 1.1rem;border-bottom:1px solid var(--border)}
  .graphlayer-title{font-size:.875rem;font-weight:700;color:var(--foreground)}
  .graphlayer-close{display:inline-flex;align-items:center;justify-content:center;width:1.9rem;height:1.9rem;
    border:0;border-radius:.55rem;background:transparent;padding:0;cursor:pointer;color:var(--muted-foreground);
    transition:background .18s ease,color .18s ease}
  .graphlayer-close svg{width:1.05rem;height:1.05rem}
  .graphlayer-close:hover{background:var(--accent);color:var(--foreground)}
  .graphlayer-body{flex:1;min-height:0;overflow:auto;padding:.9rem 1.1rem 1.1rem}
/* 浮层内的图形容器需要确定高度。
   它此前只靠内联 style 的 100%，而父级是 flex:1;min-height:0，
   百分比高度在高度未定的 flex 子项里算出来是 0 —— 于是挂载时的尺寸守卫判定为「太小」并直接放弃，
   浮层里什么也没画（实测 nodes=0）。
   给出 min-height 使容器有可测量的高度，与卡片内固定 18rem 的做法一致。 */
.graphlayer-host{width:100%;height:100%;min-height:24rem}
.graphlayer-host .graph-svg{display:block;width:100%;height:100%;touch-action:none}
  .graphlayer-body .graph-frag{height:100%}
  .graphlayer-body svg{width:100%;height:auto;max-height:74vh}
  .graphlayer-loading,.graphlayer-error{padding:3rem 0;text-align:center;
    font-size:.875rem;color:var(--muted-foreground)}
  /* 全屏浮层内标签略大:那里的画布是视口尺寸,10.5px 会显得很小。 */
  .graphlayer .g-node text{font-size:11.5px}
}
/* 工具条在小屏上保留 —— 见文件末尾「移动端阅读布局」里的说明。
   这里原有一条 .graphcard-tools{display:none},理由是「桌面端专用控件在小屏上消失」;
   但深度滑条与全屏在窄屏上比在桌面上更有用,那条判断不成立。 */
@media (prefers-reduced-motion:reduce){
  .graphlayer-panel{transform:none;opacity:1}
  .graphlayer.is-in .graphlayer-panel{transition:none}
}
@media print{.graphlayer{display:none}}

/* ── 动态块降级卡片(Mermaid / Dataview / Bases / PlantUML / Canvas)──────── */
/* 服务端无法求值这些格式,但绝不展示原始源码当作内容,也绝不静默删除:
   读者应当知道这里原本有动态内容。中性的虚线边框把它与真实内容区分开。 */
.dynblock{margin:1.1rem 0;border:1px dashed color-mix(in srgb,var(--foreground) 22%,transparent);
  border-radius:.75rem;padding:.85rem 1rem;background:color-mix(in srgb,var(--muted) 22%,transparent)}
.dynblock-head{display:flex;align-items:center;gap:.5rem}
.dynblock-tag{font-size:.75rem;font-weight:700;letter-spacing:.02em;
  color:var(--muted-foreground);text-transform:none}
.dynblock-tag::before{content:"◇";margin-right:.35rem;opacity:.6}
.dynblock-note{margin:.5rem 0 0;font-size:.8125rem;line-height:1.7;color:var(--muted-foreground)}
.dynblock-src{margin:.6rem 0 0}
.dynblock-src summary{font-size:.75rem;color:var(--muted-foreground);cursor:pointer}
.dynblock-src pre{margin:.5rem 0 0;border-radius:.5rem;font-size:.75rem;
  max-height:20rem;overflow:auto;background:var(--muted)}
@media (max-width:860px){.dynblock{padding:.7rem .8rem}}
@media print{.dynblock{break-inside:avoid}}

/* ── PDF 内联(<=20MB)与仅链接(>20MB)────────────────────────────── */
/* 内联对一份小讲义是真实改进:读者留在文章里。但 <embed> 会在页面加载时就取回整个文档,
   因此超过上限时保持仅链接,并说明原因。 */
.pdfembed{margin:1.2rem 0;padding:0;border:1px solid var(--border);border-radius:.75rem;overflow:hidden;
  background:color-mix(in srgb,var(--muted) 25%,transparent)}
.pdfembed embed{display:block;width:100%;height:min(70vh,44rem);border:0}
.pdfembed figcaption{padding:.5rem .85rem;border-top:1px solid var(--border);
  font-size:.75rem;color:var(--muted-foreground)}
.pdfembed figcaption a{color:var(--muted-foreground)}
.pdfembed figcaption a:hover{color:var(--foreground)}
.pdflink{display:inline-flex;align-items:center;gap:.3rem;padding:.25rem .6rem;border-radius:.5rem;
  border:1px solid var(--border);background:color-mix(in srgb,var(--muted) 35%,transparent);
  text-decoration:none;font-size:.875rem}
.pdflink:hover{border-color:color-mix(in srgb,var(--primary) 45%,var(--border))}
.pdfnote{font-size:.75rem;color:var(--muted-foreground);margin-left:.4rem}
@media print{.pdfembed embed{height:auto}}

/* ── 链接悬停预览(桌面端专用)──────────────────────────────────── */
/* 与图谱浮层同一门控:861px 以下不显示,减少动效时不做过渡。
   卡片是附加信息,不是内容 —— 它绝不能挡住它正在解释的那条链接。 */
@media (min-width:861px){
  .lpreview{position:absolute;z-index:42;width:min(22rem,80vw);pointer-events:none;
    padding:.6rem .75rem;border:1px solid var(--border);border-radius:.6rem;background:var(--card);
    box-shadow:0 10px 30px -8px #00000033,0 2px 8px -2px #0000001f;
    transform:translateY(-2px);opacity:0}
  .lpreview[hidden]{display:none}
  .lpreview.is-in{transform:translateY(0);opacity:1;
    transition:transform .18s cubic-bezier(.22,1,.36,1),opacity .14s ease}
  .lpreview-title{font-size:.8125rem;font-weight:600;color:var(--foreground);line-height:1.45}
  .lpreview-path{margin-top:.2rem;font-size:.6875rem;color:var(--muted-foreground);
    white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
}
@media (max-width:860px){.lpreview{display:none}}
@media (prefers-reduced-motion:reduce){.lpreview{transform:none;opacity:1}
  .lpreview.is-in{transition:none}}
@media print{.lpreview{display:none}}

@media print{
  .topbar,.aside,.rail,.footer,.backdrop{display:none}
  .layout{display:block;padding:0}
  .post{box-shadow:none;border:0;padding:0}
}

/* ── 知识库问答（AI 入口）─────────────────────────────────────────────
   配色沿用博客的暖灰令牌，只把主色用在「正在发生」的元素上：悬浮按钮、
   引用编号、引用角标。整体节奏偏慢——出现要能看清，收束要留余韵；
   所有动效都在 prefers-reduced-motion 下退化为静态。 */
#ask-root{
  --ask-r:1.15rem;--ask-z:60;
  /* 提问入口叠在知识图谱入口之上的高度。
     图谱入口是 bottom:1.25rem、高约 2.6rem，再加上 .6rem 的间隙。
     写成一个变量，是因为两个入口必须同时移动：分别改两处数字，早晚会只改一处。 */
  --fab-stack-2:4.6rem;
}
/* 两个浮动入口共处右侧一列,右边缘对齐:提问在右上,图谱在右下。

   它们最初都在右下,位置完全重合,图谱入口因此点不到。
   错开的方向是一上一下分开,而不是把提问搬到页面别的角 ——
   两者共享同一条右边线,视觉上属于同一组控件。

   位置按与 .layout 相同的栅格几何算出,而不是写一个近似的 rem:
     右边缘 = 布局右内边距 + 居中留白
     顶边   = 顶栏高 + 布局上内边距
   实测对照(视口 1440):右侧栏是 x=1208..1416 / y=102;
   写死 1.25rem 与 5.25rem 时按钮落在 1420 / 84 —— 右边缘多出 4px、顶边高出 18px,
   于是它悬在顶栏与右侧栏之间的缝里,与右侧栏并不齐平。 */
.ask-fab{
  position:fixed;
  /* 右下角，叠在知识图谱入口之上。
     它原本在右上角，而那正是右栏卡片的顶边所在——右栏与左栏对齐之后，那个位置会盖住内容。
     浮动入口不该由正文给它腾地方，两个入口同角堆叠则是更小的一处妥协。 */
  right:1.5rem;
  bottom:var(--fab-stack-2,4.6rem);
  z-index:var(--ask-z);
  display:inline-flex;align-items:center;gap:.5rem;
  padding:.7rem 1.05rem .7rem .85rem;
  border:1px solid var(--border);border-radius:999px;
  background:color-mix(in srgb,var(--card) 88%,transparent);
  backdrop-filter:blur(12px);-webkit-backdrop-filter:blur(12px);
  box-shadow:var(--shadow-md,0 8px 30px rgba(0,0,0,.12));
  color:var(--foreground);font:inherit;font-size:.875rem;font-weight:600;
  cursor:pointer;
  transition:transform .22s cubic-bezier(.22,1,.36,1),box-shadow .22s ease,background .22s ease;
}
.ask-fab:hover{transform:translateY(-2px);background:var(--card);
  box-shadow:0 14px 40px rgba(0,0,0,.16)}
.ask-fab:active{transform:translateY(0) scale(.97)}
.ask-fab-ico{display:inline-flex;width:1.15rem;height:1.15rem;color:var(--primary)}
.ask-fab-ico svg{width:100%;height:100%}
#ask-root.ask-on .ask-fab{opacity:0;pointer-events:none;transform:scale(.9) translateY(6px)}


.ask-backdrop{
  position:fixed;inset:0;z-index:calc(var(--ask-z) - 1);
  background:color-mix(in srgb,var(--foreground) 22%,transparent);
  backdrop-filter:blur(2px);-webkit-backdrop-filter:blur(2px);
  opacity:0;transition:opacity .24s ease;
}
#ask-root.ask-in .ask-backdrop{opacity:1}

.ask-panel{
  position:fixed;z-index:var(--ask-z);
  /* 与入口同列同边：面板从按钮所在的那个角张开。
     入口移到右下之后，面板也必须从右下长大，否则它会从头上的某个位置展开，
     与手指点的位置对不上。 */
  right:1.5rem;
  bottom:var(--fab-stack-2,4.6rem);
  width:min(29rem,calc(100vw - 3rem));
  height:min(40rem,calc(100vh - 3rem));
  display:flex;flex-direction:column;
  border:1px solid var(--border);border-radius:var(--ask-r);
  background:var(--card);box-shadow:0 24px 70px rgba(0,0,0,.22);
  overflow:hidden;
  /* 原点在右下：面板从它所属的那个角长大，收起时也回到那里。 */
  opacity:0;transform:translateY(14px) scale(.975);transform-origin:100% 100%;
  transition:opacity .34s ease,transform .44s cubic-bezier(.22,1,.36,1);
}
#ask-root.ask-in .ask-panel{opacity:1;transform:none}

/* hidden 属性只对应浏览器默认样式里的 [hidden]{display:none}，会被上面那条
   .ask-panel 的 display:flex 直接盖掉。少了这一条，面板「关闭」后仍以
   opacity:0 占据右下角并参与命中测试，输入区正好压在悬浮按钮上，
   于是点按钮点到的其实是面板——手机上表现为点不动、点错位置。 */
.ask-panel[hidden],.ask-backdrop[hidden]{display:none}

.ask-head{
  display:flex;align-items:center;gap:.75rem;
  padding:.85rem 1rem;border-bottom:1px solid var(--border);
  background:color-mix(in srgb,var(--muted) 35%,transparent);
}
.ask-title{display:flex;align-items:center;gap:.5rem;font-size:.9375rem;font-weight:700;min-width:0}
.ask-sub{font-size:.75rem;font-weight:500;color:var(--muted-foreground);white-space:nowrap;
  overflow:hidden;text-overflow:ellipsis}
.ask-dot{width:.5rem;height:.5rem;border-radius:50%;background:var(--primary);flex:0 0 .5rem;
  box-shadow:0 0 0 0 color-mix(in srgb,var(--primary) 55%,transparent);animation:ask-pulse 2.4s ease-out infinite}
@keyframes ask-pulse{0%{box-shadow:0 0 0 0 color-mix(in srgb,var(--primary) 50%,transparent)}
  70%{box-shadow:0 0 0 .55rem transparent}100%{box-shadow:0 0 0 0 transparent}}
.ask-close{margin-left:auto;display:inline-flex;width:1.9rem;height:1.9rem;align-items:center;justify-content:center;
  border:0;border-radius:.55rem;background:transparent;color:var(--muted-foreground);cursor:pointer;
  transition:background .18s ease,color .18s ease}
.ask-close:hover{background:var(--accent);color:var(--foreground)}
.ask-close svg{width:1rem;height:1rem}

.ask-body{flex:1;overflow-y:auto;padding:1rem;display:flex;flex-direction:column;gap:.9rem;
  overscroll-behavior:contain;scrollbar-width:thin}

.ask-intro{padding:.4rem .1rem}
.ask-intro-t{margin:0 0 .6rem;font-size:.8125rem;color:var(--muted-foreground)}
.ask-chips{display:flex;flex-wrap:wrap;gap:.4rem}
.ask-chip{
  padding:.42rem .7rem;border:1px solid var(--border);border-radius:999px;
  background:color-mix(in srgb,var(--muted) 40%,transparent);
  color:var(--foreground);font:inherit;font-size:.8125rem;cursor:pointer;
  transition:border-color .18s ease,background .18s ease,transform .18s ease}
.ask-chip:hover{border-color:color-mix(in srgb,var(--primary) 45%,var(--border));
  background:color-mix(in srgb,var(--primary) 8%,transparent);transform:translateY(-1px)}

.ask-msg{display:flex;flex-direction:column;gap:.5rem;animation:ask-in .48s cubic-bezier(.22,1,.36,1) backwards}
@keyframes ask-in{from{opacity:0;transform:translateY(10px)}to{opacity:1;transform:none}}
.ask-me{align-items:flex-end}
.ask-me .ask-bubble{
  max-width:88%;padding:.6rem .85rem;border-radius:1rem 1rem .3rem 1rem;
  background:var(--primary);color:var(--primary-foreground,#fff);
  font-size:.9375rem;line-height:1.6;word-break:break-word}
.ask-bubble{max-width:100%}
.ask-answer{font-size:.9375rem;line-height:1.8;color:color-mix(in srgb,var(--foreground) 92%,transparent)}
/* 回答按短语分块落定：每块播放一次短促的上浮淡入，像琴键依次按下。
   正在接收的尾块不播动画，否则每个 delta 都会把它抖一下。 */
.ask-blk{animation:ask-blk-in .58s cubic-bezier(.22,1,.36,1) backwards}
.ask-blk-live{animation:none;opacity:1}
@keyframes ask-blk-in{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:none}}
.ask-answer p{margin:.4rem 0}
.ask-answer ul,.ask-answer ol{margin:.4rem 0;padding-left:1.25rem}
.ask-answer li{margin:.22rem 0}
.ask-answer .ask-h{font-weight:700;margin:.6rem 0 .25rem}
.ask-answer strong{font-weight:700}
.ask-answer:empty::after{
  content:"";display:block;width:1.9rem;height:.95rem;border-radius:.3rem;
  background:linear-gradient(90deg,var(--muted) 0%,color-mix(in srgb,var(--primary) 30%,var(--muted)) 50%,var(--muted) 100%);
  background-size:200% 100%;animation:ask-shimmer 1.15s linear infinite}
@keyframes ask-shimmer{from{background-position:120% 0}to{background-position:-120% 0}}

/* 检索过程的可视化。节奏刻意放缓：原来图标持续旋转、底部还有一条来回扫的光带，
   两者叠加让每一步都像被硬切进来。现在改为静态图标 + 逐行柔和淡入。 */
.ask-trace{border:1px solid var(--border);border-radius:.85rem;
  background:color-mix(in srgb,var(--muted) 22%,transparent);padding:.65rem .75rem;
  display:flex;flex-direction:column;gap:.5rem}
.ask-steps{display:flex;flex-direction:column;gap:.32rem}
.ask-step{display:flex;align-items:flex-start;gap:.45rem;
  font-size:.8125rem;line-height:1.65;color:var(--muted-foreground);
  animation:ask-line-in .55s cubic-bezier(.22,1,.36,1) backwards}
.ask-step-ico{display:inline-flex;width:.95rem;height:.95rem;flex:0 0 .95rem;
  color:var(--c3);margin-top:.18rem}
.ask-step-ico svg{width:100%;height:100%}
.ask-step.is-live{color:var(--foreground)}
.ask-step.is-live .ask-step-ico{color:var(--primary)}
.ask-step.is-done .ask-step-ico{color:color-mix(in srgb,var(--primary) 55%,var(--c4))}
.ask-step.is-error .ask-step-ico{color:#c2503f}
.ask-step-t{flex:1}

.ask-sources{display:flex;flex-direction:column;gap:.3rem}

/* 模型的推理流（思考模型）。限高 + 底部渐隐：推理可达数千字，
   全量铺开会把回答挤出视野；点标题可展开回看。 */
.ask-thought{
  border:1px dashed var(--border);border-radius:.75rem;
  padding:.5rem .6rem;background:color-mix(in srgb,var(--muted) 16%,transparent);
  animation:ask-line-in .5s cubic-bezier(.22,1,.36,1) backwards}
.ask-thought-h{
  display:flex;align-items:center;gap:.4rem;width:100%;
  border:0;background:transparent;padding:0;cursor:pointer;
  font:inherit;font-size:.75rem;font-weight:600;color:var(--muted-foreground);text-align:left}
.ask-thought-ico{display:inline-flex;width:.9rem;height:.9rem;flex:0 0 .9rem;color:var(--primary)}
.ask-thought-ico svg{width:100%;height:100%}
.ask-thought.is-done .ask-thought-ico{color:color-mix(in srgb,var(--primary) 55%,var(--c4))}
.ask-thought-t{flex:1}
.ask-thought-n{font-size:.6875rem;font-weight:500;color:var(--c4)}
.ask-thought-b{
  margin:.4rem 0 0;font-size:.75rem;line-height:1.72;
  color:var(--muted-foreground);white-space:pre-wrap;word-break:break-word;
  max-height:7.5rem;overflow:hidden;
  -webkit-mask-image:linear-gradient(180deg,#000 55%,transparent);
  mask-image:linear-gradient(180deg,#000 55%,transparent)}
.ask-thought.is-open .ask-thought-b{max-height:none;mask-image:none;-webkit-mask-image:none}
/* 检索明细：默认收起的过程信息，字号刻意小于正文与引文，避免与回答争夺注意力。 */
.ask-src-toggle{
  display:flex;align-items:center;gap:.35rem;width:100%;
  padding:.1rem 0;border:0;background:transparent;cursor:pointer;
  font:inherit;font-size:.75rem;font-weight:600;color:var(--muted-foreground);text-align:left;
  animation:ask-line-in .5s cubic-bezier(.22,1,.36,1) backwards;
  transition:color .26s ease}
.ask-src-toggle:hover{color:var(--foreground)}
.ask-src-tg-ico{display:inline-flex;width:.8rem;height:.8rem;flex:0 0 .8rem;
  color:var(--primary);transition:transform .3s cubic-bezier(.22,1,.36,1)}
.ask-src-tg-ico svg{width:100%;height:100%}
.ask-src-toggle.is-open .ask-src-tg-ico{transform:rotate(90deg)}
.ask-src-toggle-t{flex:1}
.ask-src-list{margin:.1rem 0 0 .55rem;padding:0 0 0 .6rem;list-style:none;
  border-left:1px solid var(--border);
  display:flex;flex-direction:column;gap:.14rem}
.ask-src-list li{display:flex;flex-wrap:wrap;align-items:baseline;gap:.4rem;
  animation:ask-line-in .5s cubic-bezier(.22,1,.36,1) backwards}
.ask-src-list a{font-size:.75rem;color:color-mix(in srgb,var(--primary) 88%,var(--foreground));
  text-decoration:none;
  border-bottom:1px solid color-mix(in srgb,var(--primary) 22%,transparent);
  transition:border-color .28s ease,color .28s ease}
.ask-src-list a:hover{border-bottom-color:var(--primary)}
.ask-src-arrow{font-size:.7em;margin-left:.14em;color:var(--c4);
  display:inline-block;transition:transform .28s ease}
.ask-src-list a:hover .ask-src-arrow{transform:translate(1px,-1px)}
.ask-src-via{font-size:.6875rem;color:var(--c4)}
@keyframes ask-line-in{from{opacity:0;transform:translateY(5px)}to{opacity:1;transform:none}}
/* 回答开始后轨迹折叠成一行摘要：步骤与来源列表合计会占掉数百像素，
   把用户真正在等的答案挤到视野之外。点「过程」可随时展开回看。 */
.ask-trace-sum{
  display:flex;align-items:center;gap:.4rem;width:100%;
  padding:.1rem 0;border:0;background:transparent;cursor:pointer;
  font:inherit;font-size:.8125rem;font-weight:600;color:var(--foreground);text-align:left}
.ask-sum-ico{display:inline-flex;width:.95rem;height:.95rem;flex:0 0 .95rem;color:var(--primary)}
.ask-sum-ico svg{width:100%;height:100%}
.ask-sum-t{flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ask-sum-x{font-size:.6875rem;font-weight:600;color:var(--primary);
  padding:.1rem .4rem;border-radius:.4rem;background:color-mix(in srgb,var(--primary) 12%,transparent)}
.ask-trace.is-min .ask-steps,.ask-trace.is-min .ask-sources{display:none}
.ask-trace.is-min.is-open .ask-steps{display:flex}
.ask-trace.is-min.is-open .ask-sources{display:flex}

/* 回答里的引用角标 */
.ask-cite{
  display:inline-flex;align-items:center;justify-content:center;
  min-width:1.05rem;height:1.05rem;padding:0 .22rem;margin:0 .12rem;
  border:0;border-radius:.35rem;
  background:color-mix(in srgb,var(--primary) 14%,transparent);
  color:var(--primary);font-size:.6875rem;font-weight:700;line-height:1;
  vertical-align:baseline;cursor:pointer;
  transition:background .16s ease,transform .16s ease}
.ask-cite:hover{background:color-mix(in srgb,var(--primary) 26%,transparent);transform:translateY(-1px)}

/* 末尾引文卡片 */
.ask-cites{border-top:1px dashed var(--border);padding-top:.6rem;display:flex;flex-direction:column;gap:.35rem}
.ask-cites-head{display:flex;align-items:center;gap:.35rem;font-size:.6875rem;font-weight:700;
  letter-spacing:.06em;text-transform:uppercase;color:var(--muted-foreground);margin-bottom:.1rem}
.ask-cites-head svg{width:.85rem;height:.85rem}
.ask-cite-card{
  display:flex;gap:.55rem;padding:.5rem .6rem;border-radius:.7rem;
  border:1px solid var(--border);background:color-mix(in srgb,var(--muted) 22%,transparent);
  text-decoration:none;color:inherit;
  animation:ask-line-in .5s cubic-bezier(.22,1,.36,1) backwards;
  transition:border-color .3s ease,background .3s ease,box-shadow .3s ease}
.ask-cite-card:hover{border-color:color-mix(in srgb,var(--primary) 45%,var(--border));
  background:color-mix(in srgb,var(--primary) 7%,transparent)}
.ask-cite-card.is-flash{border-color:var(--primary);
  box-shadow:0 0 0 4px color-mix(in srgb,var(--primary) 16%,transparent);
  transition:border-color .3s ease,box-shadow .5s ease}
.ask-cite-n{flex:0 0 1.15rem;height:1.15rem;display:inline-flex;align-items:center;justify-content:center;
  border-radius:.35rem;background:color-mix(in srgb,var(--primary) 16%,transparent);
  color:var(--primary);font-size:.6875rem;font-weight:700}
.ask-cite-b{display:flex;flex-direction:column;gap:.1rem;min-width:0}
.ask-cite-t{font-size:.8125rem;font-weight:600}
.ask-cite-x{font-size:.75rem;color:var(--muted-foreground);line-height:1.6;
  display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.ask-err{color:#c2503f;font-size:.875rem}
.ask-none{color:var(--muted-foreground);font-size:.875rem}

/* 重试 / 停止：回答之下的轻量操作行 */
.ask-acts{display:flex;flex-wrap:wrap;gap:.4rem;
  animation:ask-line-in .5s cubic-bezier(.22,1,.36,1) backwards}
.ask-act{
  display:inline-flex;align-items:center;gap:.32rem;
  padding:.34rem .68rem;border:1px solid var(--border);border-radius:999px;
  background:color-mix(in srgb,var(--muted) 30%,transparent);
  color:var(--muted-foreground);font:inherit;font-size:.75rem;font-weight:600;
  cursor:pointer;
  transition:color .26s ease,border-color .26s ease,background .26s ease,transform .26s ease}
.ask-act svg{width:.85rem;height:.85rem}
.ask-act:hover{color:var(--foreground);
  border-color:color-mix(in srgb,var(--primary) 40%,var(--border));
  background:color-mix(in srgb,var(--primary) 8%,transparent);
  transform:translateY(-1px)}
.ask-act:active{transform:translateY(0) scale(.97)}
.ask-act.is-retry{color:var(--primary);
  border-color:color-mix(in srgb,var(--primary) 38%,var(--border))}
.ask-act.is-stop{color:#c2503f;
  border-color:color-mix(in srgb,#c2503f 34%,var(--border))}
.ask-act.is-stop:hover{color:#a83c2c;background:color-mix(in srgb,#c2503f 8%,transparent)}

/* 输入区 */
.ask-form{border-top:1px solid var(--border);padding:.7rem .8rem .55rem;
  background:color-mix(in srgb,var(--muted) 24%,transparent)}
.ask-field{display:flex;align-items:flex-end;gap:.5rem;
  border:1px solid var(--border);border-radius:.95rem;background:var(--card);
  padding:.35rem .35rem .35rem .7rem;
  transition:border-color .2s ease,box-shadow .2s ease}
.ask-field:focus-within{border-color:color-mix(in srgb,var(--primary) 55%,var(--border));
  box-shadow:0 0 0 3px color-mix(in srgb,var(--primary) 12%,transparent)}
.ask-input{flex:1;border:0;outline:0;background:transparent;resize:none;
  font:inherit;font-size:.9375rem;line-height:1.6;color:var(--foreground);
  padding:.3rem 0;max-height:8.25rem;scrollbar-width:thin}
.ask-input::placeholder{color:var(--muted-foreground)}
.ask-send{flex:0 0 auto;display:inline-flex;width:2rem;height:2rem;align-items:center;justify-content:center;
  border:0;border-radius:.7rem;background:var(--primary);color:var(--primary-foreground,#fff);
  cursor:pointer;transition:transform .18s ease,opacity .18s ease}
.ask-send svg{width:1.05rem;height:1.05rem}
.ask-send:hover{transform:translateY(-1px)}
.ask-send:active{transform:scale(.94)}
.ask-send.is-busy{opacity:.55;pointer-events:none}
.ask-send.is-busy svg{animation:ask-spin 1s linear infinite}
.ask-hint{margin:.4rem 0 0;font-size:.6875rem;color:var(--muted-foreground);text-align:center}

/* 平板与手机：面板铺满可用空间，输入区留出安全区 */
@media (max-width:860px){
  /* 移动端同样在右下角:此断点下图谱入口整体隐藏(#graph-root),
     所以右下角是空的,不存在与它冲突的问题。 */
  .ask-fab{
    right:1rem;left:auto;
    /* top 必须显式复位:固定定位元素同时有 top 与 bottom 时以 top 为准,
       只写 bottom 会让它继续停在桌面版的位置。 */
    top:auto;bottom:1rem;
    padding:.62rem .95rem .62rem .8rem;
  }
  .ask-panel{
    right:0;bottom:0;left:0;top:auto;
    width:100%;height:min(88vh,100%);
    border-radius:1.25rem 1.25rem 0 0;transform-origin:50% 100%;
    transform:translateY(24px);
    /* 键盘弹起时脚本会内联改写 height/bottom（见 ask 脚本的 Soft-keyboard avoidance）。
       把这两个属性也纳入过渡，面板才会随键盘平滑升降，而不是瞬间跳位。
       用较短时长：键盘动画本身约 200ms，跟随得比它慢会显得拖沓。 */
    transition:opacity .34s ease,transform .44s cubic-bezier(.22,1,.36,1),
      height .2s ease,bottom .2s ease;
  }
  .ask-body{padding:.85rem}
  .ask-form{padding-bottom:calc(.55rem + env(safe-area-inset-bottom,0px))}
  .ask-backdrop{background:color-mix(in srgb,var(--foreground) 30%,transparent)}
}
@media (max-width:420px){
  .ask-fab-txt{display:none}
  .ask-fab{padding:.62rem;border-radius:50%}
  .ask-me .ask-bubble{max-width:94%}
}
@media (prefers-reduced-motion:reduce){
  .graph-fab,.graph-panel,.graph-backdrop,
  .ask-fab,.ask-panel,.ask-backdrop,.ask-msg,.ask-chip,.ask-src-list,.ask-src-list a,
  .ask-src-list li,.ask-src-toggle,.ask-step,.ask-cite,.ask-cite-card,.ask-send,.ask-blk{
    transition:none;animation:none}
  .ask-answer:empty::after{animation:none}
  .ask-dot{animation:none}
}

/* ── 页面笔记（关于）───────────────────────────────────────────────── */
/* 页面与文章共用骨架，但布局不同：左栏的目录树不出现（见模板，它不调用 nav 模板），
   因此这里把栅格改成「正文 + 可选的右栏目录」。宽度归正文，这正是简历需要的。 */
.layout-page{grid-template-columns:minmax(0,1fr) 13rem}
/* 没有小节标题时模板会加上这个类，栅格只剩一列并居中限宽。
   用类名而不是 :not(:has(.rail)) 是有意的：:has() 是较新的选择器，
   在不支持它的浏览器上那条规则会整条失效，于是右栏留下一个空列，页面看起来是坏的。
   由模板决定类名不依赖任何新特性。 */
.layout-page-single{grid-template-columns:minmax(0,1fr);max-width:56rem;margin-inline:auto}

/* 顶栏里的页面笔记入口。
   放在面包屑左侧、与它同一条基线：两者都是「我在哪、能去哪」的指路信息，
   分开摆会让顶栏出现两簇方向感互相打架的链接。 */
.topnav{display:flex;align-items:center;gap:.15rem;margin-left:.4rem;flex:none}
.topnav a{
  padding:.3rem .6rem;border-radius:.5rem;
  font-size:.8125rem;font-weight:500;
  color:var(--muted);text-decoration:none;
  transition:color .16s ease,background-color .16s ease;
}
.topnav a:hover{color:var(--foreground);background:color-mix(in srgb,var(--muted) 34%,transparent)}

/* ── 首页加载动画：各组件由下滑入 ──────────────────────────────────────── */
/* 编排顺序(与用户标注一致)：仪表盘 → 全部笔记逐条 → 侧栏。
   左栏最后动，因为它是一张导航卡，读者先看内容再看路。

   为什么用 nth-child 写死前几条的延迟，而不是让 JS 算：
   用 JS 就要等到脚本跑起来才加动画，而那时元素已经以最终状态画过一帧 ——
   于是每一次加载都会先闪一下再开始滑。纯 CSS 的动画在首次绘制之前就已经就绪。

   为什么延迟设上限：仓库里有 112 条笔记。每条 +38ms 意味着最后一条要等 4 秒，
   而那已经不是动画，是加载。前 12 条依次入场，之后的一起出现 ——
   读者的视线到不了第 13 条，所以那里发生什么没有区别。 */
@keyframes fns-rise{
  from{opacity:0;transform:translateY(16px)}
  to{opacity:1;transform:none}
}

.stats .stat,
.list .item,
.rail>.panel{
  /* both：起始帧在动画开始前就应用，避免元素先以最终状态出现再跳回起点。 */
  animation:fns-rise .52s cubic-bezier(.22,1,.36,1) both;
}

/* 仪表盘：七张卡片，间距 55ms。第一张不延迟，使页面一出现就有东西在动。 */
.stats .stat:nth-child(1){animation-delay:0ms}
.stats .stat:nth-child(2){animation-delay:55ms}
.stats .stat:nth-child(3){animation-delay:110ms}
.stats .stat:nth-child(4){animation-delay:165ms}
.stats .stat:nth-child(5){animation-delay:220ms}
.stats .stat:nth-child(6){animation-delay:275ms}
.stats .stat:nth-child(7){animation-delay:330ms}

/* 条目：从第 12 张卡片之后起算，逐条 38ms。
   卡片 380ms 结束 + 条目起点 60ms，使两者交叠而不是排队。 */
.list .item:nth-child(1){animation-delay:380ms}
.list .item:nth-child(2){animation-delay:418ms}
.list .item:nth-child(3){animation-delay:456ms}
.list .item:nth-child(4){animation-delay:494ms}
.list .item:nth-child(5){animation-delay:532ms}
.list .item:nth-child(6){animation-delay:570ms}
.list .item:nth-child(7){animation-delay:608ms}
.list .item:nth-child(8){animation-delay:646ms}
.list .item:nth-child(9){animation-delay:684ms}
.list .item:nth-child(10){animation-delay:722ms}
.list .item:nth-child(11){animation-delay:760ms}
.list .item:nth-child(12){animation-delay:798ms}
/* 第 13 条起：与第 12 条同时，不再递增。见上方关于上限的说明。 */
.list .item:nth-child(n+13){animation-delay:798ms}

/* 侧栏：左栏与右栏各一张卡，最后入场。 */
.aside>.panel:nth-child(1),.rail>.panel:nth-child(1){animation-delay:300ms}
.aside>.panel:nth-child(2),.rail>.panel:nth-child(2){animation-delay:400ms}
.aside>.panel:nth-child(3),.rail>.panel:nth-child(3){animation-delay:500ms}

/* 减少动效：不滑入，直接呈现。
   这不是"把时长调到 0"就够的 —— animation:both 会让元素在起点停住一小会儿，
   而那一下空白本身就是一次闪烁。 */
@media (prefers-reduced-motion:reduce){
  .stats .stat,
  .list .item,
  .rail>.panel{animation:none}
}

/* ── 首页环境粒子场 ─────────────────────────────────────────────────── */
/* 铺满视口，落在点阵背景之上、内容之下（.backdrop 是 z-index:0，.shell 是 z-index:1）。
   它因此在卡片之间的留白与页面边缘可见，而不会被正文压住。

   这里没有 z-index：它与 .backdrop 同为 auto，靠 DOM 顺序排在后面。
   给一个负值会把它推到 body 背景之后，什么都看不见。 */
.fns-particles{
  position:fixed;inset:0;
  pointer-events:none;   /* 关键：它盖住整个视口，可点击的话页面上每一样东西都点不到。 */
  z-index:0;
}
/* 首页内容的底衬只做在首页，避免在文章页把粒子也挡掉（那些页面不注入粒子）。 */
.layout-home{position:relative;z-index:1}

/* ── 穿越过渡 ───────────────────────────────────────────────────────── */
/* 覆盖层。pointer-events:none：它盖住整个视口，若可点击，导航过程中页面上的东西都点不到。 */
.fns-warp{position:fixed;inset:0;z-index:200;pointer-events:none}

/* 出发：页面在星空之下淡出并轻微放大，像是被甩在身后。 */
html.fns-warping body{opacity:0;transform:scale(1.03);transition:opacity .7s ease,transform 1.1s cubic-bezier(.4,0,.9,.4)}

/* 抵达：内容渐显。从略小、略淡的状态浮上来，与放大的离开方向相对，读起来像一次接续。 */
html.fns-arrived .shell{opacity:0;transform:scale(.985)}
html.fns-arrived body{}
@keyframes fns-arrive-fade{from{opacity:0;transform:scale(.985)}to{opacity:1;transform:none}}
html.fns-arrived .shell{animation:fns-arrive-fade .62s cubic-bezier(.22,1,.36,1) both}

@media (prefers-reduced-motion:reduce){
  html.fns-warping body{opacity:1;transform:none;transition:none}
  html.fns-arrived .shell{animation:none;opacity:1;transform:none}
}

/* ── 热门(首页右栏)──────────────────────────────────────────────────── */
.hot{margin:.5rem 0 0;padding:0;list-style:none;counter-reset:h}
.hot li{
  display:flex;align-items:baseline;gap:.5rem;
  padding:.3rem 0;
  font-size:.8125rem;line-height:1.5;
}
/* 名次用计数器而不是手写数字:排序一变，名次跟着变，不必改数据。 */
.hot li::before{
  counter-increment:h;content:counter(h);
  flex:0 0 1.05rem;
  font-size:.6875rem;font-variant-numeric:tabular-nums;
  color:var(--muted-foreground);opacity:.65;
}
.hot a{
  flex:1;min-width:0;
  color:var(--foreground);text-decoration:none;
  /* 标题可能很长，单行截断而不是换行 —— 换行会让五条的列表高度参差不齐。 */
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
}
.hot a:hover{color:var(--primary)}
.hot-n{
  flex:0 0 auto;
  font-size:.6875rem;font-variant-numeric:tabular-nums;
  color:#c2410c;
}

/* ── 文章底部互动条 ──────────────────────────────────────────────────── */
.react{
  display:flex;align-items:center;gap:.35rem;
  margin:1.8rem 0 0;padding-top:1.1rem;
  border-top:1px solid var(--border);
}
.react-btn{
  display:inline-flex;align-items:center;gap:.42rem;
  padding:.46rem .78rem;
  border:1px solid transparent;border-radius:.55rem;
  background:transparent;color:var(--muted-foreground);
  font-size:.8125rem;font-variant-numeric:tabular-nums;
  cursor:pointer;
  transition:color .16s ease,background-color .16s ease,border-color .16s ease;
}
.react-btn:hover{color:var(--foreground);background:color-mix(in srgb,var(--muted) 34%,transparent)}
.react-ico{display:inline-flex;width:1.05rem;height:1.05rem;flex:0 0 1.05rem}
.react-ico svg{width:100%;height:100%}

/* 已赞 / 已踩：实心高亮。
   用 background 与 color 而不是只换图标：按下之后要能一眼看出「这是我做过的」，
   而一个只变色的描边在灰底上几乎看不出来。 */
.react-btn.on{
  color:var(--foreground);
  background:color-mix(in srgb,var(--foreground) 9%,transparent);
  border-color:color-mix(in srgb,var(--foreground) 16%,transparent);
}
.react-btn.on .react-ico svg{fill:currentColor;fill-opacity:.16}
/* 点赞与点踩的高亮色不同，否则两者同时可见时读者分不清自己按的是哪个。 */
.react-btn[data-act="like"].on{color:#c2410c;border-color:color-mix(in srgb,#c2410c 26%,transparent);background:color-mix(in srgb,#c2410c 8%,transparent)}
.react-btn[data-act="dislike"].on{color:#6b4423;border-color:color-mix(in srgb,#6b4423 26%,transparent);background:color-mix(in srgb,#6b4423 8%,transparent)}

.react-spacer{flex:1}
.react-share{margin-left:.1rem}
.react-share.done{color:#2f7d4f}
.react-visits{display:inline-flex;align-items:baseline;gap:.28rem;font-size:.8125rem;color:var(--muted-foreground)}
.react-visits-n{font-variant-numeric:tabular-nums;font-weight:600;color:var(--foreground)}

/* 反应失败：一次短暂的边线提示。
   不弹 toast —— 读者刚把一个按钮按下去，答案应该出现在同一个地方。 */
.react.err{border-top-color:#c2410c}

/* 粒子覆盖层。pointer-events:none 是关键：它盖住整个视口，
   若可点击，页面上的每一样东西都会变得点不到。 */
.fns-fx{position:fixed;inset:0;z-index:60;pointer-events:none}

@media (max-width:560px){
  .react{flex-wrap:wrap;gap:.2rem}
  .react-visits{margin-left:auto}
  .react-btn{padding:.5rem .6rem}
}

.page{padding:2.6rem 2.8rem 3rem}
.page .pt{margin-top:0}

/* 头像与标题的两栏头。
   没有头像时整块不出现（模板里就没有这个元素），而不是留一个灰色占位——
   占位会让人以为图加载失败了。 */
.page-avatar{float:left;margin:.35rem 1.4rem .9rem 0}
.page-avatar img{
  display:block;width:6.5rem;height:6.5rem;object-fit:cover;
  border-radius:50%;border:1px solid var(--border);
  background:color-mix(in srgb,var(--muted) 40%,transparent);
}
.page-meta{margin:-.2rem 0 1.6rem;font-size:.8125rem;color:var(--muted)}

/* ── 三个排版容器 ──────────────────────────────────────────────────── */
/* 容器是增强，不是前提：样式整体失效时，里面的 Markdown 仍会以普通段落与列表呈现，
   内容不会丢。这是这套设计必须满足的条件（见 blog_page_container.go）。 */
.pc{margin:1.3rem 0}

/* profile-meta —— 基本信息。
   两列网格而不是一行一条：这些是「邮箱/性别/年龄/所在地/学历/户籍/民族」这类短事实，
   写成连续的段落会白白占满宽度，而读者要找的是其中一条，不是读完全部。
   窄屏降为一列。 */
.pc-profile-meta{
  margin:1.3rem 0;
  display:grid;grid-template-columns:repeat(2,minmax(0,1fr));
  gap:.55rem 2.4rem;
}
/* 容器里的 Markdown 列表就是「标签 值」的序列。
   去掉项目符号与缩进 —— 网格本身已经是结构，再加一层符号是两套结构说同一件事。 */
.pc-profile-meta>ul{
  display:contents;list-style:none;margin:0;padding:0;
}
.pc-profile-meta li{
  display:flex;align-items:baseline;gap:.6rem;
  min-width:0;
  font-size:.875rem;line-height:1.7;
  padding-bottom:.5rem;
  border-bottom:1px solid color-mix(in srgb,var(--border) 60%,transparent);
}
/* 标签固定宽度，使所有值左边缘对齐成一条竖线 —— 那是表格可读性的来源。
   用 ch 而不是 rem：它跟着字号走，改字号时不会错位。 */
.pc-profile-meta li>strong,
.pc-profile-meta li>em{
  flex:0 0 5.5em;
  font-style:normal;font-weight:500;
  color:var(--muted-foreground);
}
.pc-profile-meta li>span{
  min-width:0;overflow-wrap:anywhere;
}
@media (max-width:560px){
  .pc-profile-meta{grid-template-columns:minmax(0,1fr)}
}

/* profile-head —— 姓名与一句话介绍 */
.pc-profile-head{
  margin:.2rem 0 1.6rem;
  padding-bottom:1.1rem;
  border-bottom:1px solid var(--border);
}
.pc-profile-head>p{margin:0}
.pc-profile-head strong{
  display:block;
  font-size:1.7rem;font-weight:600;line-height:1.25;
  letter-spacing:-.01em;color:var(--foreground);
}
.pc-profile-head strong+br{display:none}
.pc-profile-head>p{color:var(--muted);font-size:1rem;margin-top:.35rem}

/* timeline —— 经历 */
.pc-timeline{margin:1.5rem 0 1.8rem}
.pc-timeline>ul{
  list-style:none;margin:0;padding:0 0 0 1.35rem;
  border-left:2px solid color-mix(in srgb,var(--muted) 55%,transparent);
}
.pc-timeline>ul>li{
  position:relative;
  padding:.34rem 0 .34rem .2rem;
  /* 避免打印时在一条经历中间断开 */
  break-inside:avoid;page-break-inside:avoid;
}
/* 每条一个圆点，压在轴线上 */
.pc-timeline>ul>li::before{
  content:"";position:absolute;left:-1.35rem;top:.95rem;
  width:.5rem;height:.5rem;margin-left:-.29rem;
  border-radius:50%;
  background:var(--muted);
}
/* 首条之外用稍暗的点，使最近一条更醒目 */
.pc-timeline>ul>li+li::before{background:color-mix(in srgb,var(--muted) 60%,transparent)}
/* 时间与条目之间用等宽数字对齐，使多条时间成一条竖线 */
.pc-timeline>ul>li code,
.pc-timeline>ul>li>strong:first-child{
  font-variant-numeric:tabular-nums;
  font-weight:600;color:var(--foreground);
}

/* links —— 联系方式的卡片行 */
.pc-links>ul{
  list-style:none;margin:0;padding:0;
  display:flex;flex-wrap:wrap;gap:.6rem;
}
.pc-links>ul>li{margin:0}
.pc-links a{
  display:inline-flex;align-items:center;gap:.45rem;
  padding:.5rem .85rem;
  border:1px solid var(--border);border-radius:.6rem;
  background:color-mix(in srgb,var(--muted) 26%,transparent);
  color:var(--foreground);font-size:.875rem;text-decoration:none;
  transition:border-color .16s ease,background-color .16s ease;
}
.pc-links a:hover{
  border-color:color-mix(in srgb,var(--foreground) 30%,transparent);
  background:color-mix(in srgb,var(--muted) 45%,transparent);
}

@media (max-width:900px){
  .layout-page{grid-template-columns:minmax(0,1fr)}
  .page{padding:1.9rem 1.5rem 2.2rem}
}
@media (max-width:560px){
  /* 窄屏改为上下堆叠居中：并排的头像与名字在手机上会把名字挤成两三个字一行 */
  .page-avatar{float:none;margin:0 0 1rem;text-align:center}
  .page-avatar img{margin:0 auto;width:5.5rem;height:5.5rem}
  .pc-profile-head{text-align:center}
  .pc-links>ul{justify-content:center}
}

/* 打印：这一页最常见的动作就是打出来或存成 PDF。
   去掉所有页面家具，保留内容版式，并把链接展开为「标签 (地址)」——
   印在纸上的链接点不开，不写出地址就等于没给。 */
@media print{
  .topbar,.footer,.ask-fab,.ask-panel,.ask-backdrop,.rail,.graph-fab{display:none !important}
  .backdrop{display:none}
  .shell{min-height:0}
  .layout,.layout-page{display:block;max-width:none;padding:0;margin:0}
  .page{border:0;box-shadow:none;background:none;padding:0}
  .page-avatar img{border-color:#999}
  /* 链接展开。只对正文里的链接，不含已经带了地址的容器链接。 */
  .page-body a[href^="http"]::after{
    content:" (" attr(href) ")";
    font-size:.8125em;color:#555;word-break:break-all;
  }
  .pc-links a::after{content:none}
  .pc-timeline>ul{border-left-color:#999}
  .pc-timeline>ul>li::before{background:#999}
  .pc-links a{border-color:#999;background:none}
  h1,h2,h3{break-after:avoid;page-break-after:avoid}
  img,table,pre,.callout,.pc{break-inside:avoid;page-break-inside:avoid}
  @page{margin:1.6cm}
}

/* ── 移动端阅读布局:边缘书签 + 抽屉 ──────────────────────────────────── */
/* 作用域严格限定在阅读页(body.is-post / body.is-page)。
   首页不参与:它的侧栏是内容本身(目录树、日历、热门),而不是阅读时可以临时让开的东西。

   为什么侧栏直接变成抽屉,而不是把内容复制进新的抽屉容器:
   那些内容已经由服务端渲染了一份,复制就意味着两处要保持同步;
   而 position:fixed 加一个 transform 就能让同一份 DOM 换一种呈现,没有第二份可走散的东西。 */

/* 书签本体。
   绑定在屏幕边缘的垂直中线,只露出一个窄条。
   视觉语言与右下角的图谱/问答入口同一套:卡片底、细边、磨砂、圆角 —— 它是同一种东西,
   只是贴着墙。 */
.fns-tab{
  display:none;                 /* 桌面端不渲染;断点内才显示。 */
  /* 层序:遮罩 57、抽屉 58、书签 59。
     书签必须在遮罩之上 —— 抽屉打开时,未被点的那一侧要保持可点,
     否则「打开一个再点另一个切换」就没有入口,读者只能先关再开。
     被点的那一侧本来就已隐藏,所以它压在抽屉上也不会露出。 */
  position:fixed;top:50%;z-index:59;
  align-items:center;justify-content:center;
  width:1.85rem;height:4.4rem;
  padding:0;
  border:1px solid var(--border);
  background:color-mix(in srgb,var(--card) 70%,transparent);
  -webkit-backdrop-filter:blur(12px);backdrop-filter:blur(12px);
  color:var(--muted-foreground);
  cursor:pointer;
  transform:translateY(-50%);
  transition:opacity .26s ease,transform .34s cubic-bezier(.22,1,.36,1),
             color .2s ease,background .2s ease;
}
.fns-tab-l{left:0;border-left:0;border-radius:0 .9rem .9rem 0}
.fns-tab-r{right:0;border-right:0;border-radius:.9rem 0 0 .9rem}
.fns-tab:active{color:var(--foreground)}
.fns-tab-ico{display:inline-flex;width:1rem;height:1rem}
.fns-tab-ico svg{width:100%;height:100%}

/* 遮罩。 */
.fns-scrim{
  display:none;
  position:fixed;inset:0;z-index:57;
  background:color-mix(in srgb,var(--foreground) 26%,transparent);
  opacity:0;
  /* 关闭态:不模糊、不拦点击。
     这两条不是「顺手加的保险」—— 没有它们时,一个 opacity:0 的遮罩仍然会被合成,
     backdrop-filter 会继续作用于身后的一切,而 pointer-events 默认是 auto。
     表现就是「没开抽屉,整页却是糊的,而且点哪儿都没反应」。 */
  -webkit-backdrop-filter:none;backdrop-filter:none;
  pointer-events:none;
  /* 把绘制与模糊限制在这个盒子里,不让它扩散到同级的固定层。 */
  contain:paint;
  transition:opacity .3s ease,backdrop-filter .3s ease;
}
.fns-scrim[hidden]{display:none}

@media (max-width:860px){
  /* ── 顶栏让位给透明的顶部操作区 ─────────────────────────────────── */
  /* 阅读页隐藏品牌与面包屑:屏幕窄到只能容下一行时,「你在哪」不如「能去哪」重要,
     而面包屑在文章页的标题上方已经重复了一次。 */
  body.is-post .topbar,
  body.is-page .topbar{display:none}

  /* ── 书签与遮罩 ─────────────────────────────────────────────────── */
  body.is-post .fns-tab,
  body.is-page .fns-tab{display:inline-flex}
  body.is-post .fns-scrim,
  body.is-page .fns-scrim{display:block}
  body.is-post .fns-scrim[hidden],
  body.is-page .fns-scrim[hidden]{display:none}

  /* 抽屉打开时,被点的那一侧书签让开 —— 它此刻的位置正是抽屉的边缘,
     留着它会压在抽屉上,变成一条无处可点的窄条。另一侧保持可见,以便直接切换。 */
  body.drawer-nav .fns-tab-l{opacity:0;pointer-events:none;transform:translateY(-50%) translateX(-115%)}
  body.drawer-toc .fns-tab-r{opacity:0;pointer-events:none;transform:translateY(-50%) translateX(115%)}

  /* ── 两个侧栏变成抽屉 ───────────────────────────────────────────── */
  body.is-post .aside,body.is-page .aside,
  body.is-post .rail,body.is-page .rail{
    /* display 必须显式写回。本断点前面有一条 .aside,.rail{display:none} 收掉了侧栏,
       只改 position 会得到一个「fixed 但不可见」的抽屉。 */
    display:block;
    position:fixed;top:0;bottom:0;z-index:58;
    width:min(82vw,19rem);
    max-height:none;margin:0;
    padding:1.1rem .9rem calc(1.1rem + env(safe-area-inset-bottom,0px));
    overflow-y:auto;
    background:var(--card);
    box-shadow:0 0 60px rgba(0,0,0,.16);
    /* 与页面其它动效同一条缓动曲线,使抽屉和书签读起来是一次动作。 */
    transition:transform .38s cubic-bezier(.22,1,.36,1);
    overscroll-behavior:contain;
    /* 自成合成层。遮罩的 backdrop-filter 在移动端 WebKit 上会把同级的 fixed 元素
       一起卷进它的合成组,抽屉的文字因此也跟着糊掉 —— 这两个属性把抽屉摘出来。 */
    isolation:isolate;
    will-change:transform;
  }
  body.is-post .aside,body.is-page .aside{
    left:0;border-radius:0 1rem 1rem 0;
    transform:translateX(-101%);
  }
  body.is-post .rail,body.is-page .rail{
    right:0;border-radius:1rem 0 0 1rem;
    transform:translateX(101%);
  }
  /* 抽屉里的卡片不再需要自己的边框与阴影 —— 它们现在住在一个已经有边界的容器里。 */
  body.is-post .aside>.panel,body.is-page .aside>.panel,
  body.is-post .rail>.panel,body.is-page .rail>.panel{
    position:static;box-shadow:none;border:0;background:transparent;
  }
  body.drawer-nav .aside{transform:none}
  body.drawer-toc .rail{transform:none}

  /* 遮罩的打开态。基础规则给的是 opacity:0(为了能过渡),
     这里必须显式设回 1 —— 否则遮罩「显示」却完全透明,
     抽屉看起来像浮在正文上,而不是把正文压到后面去。 */
  body.drawer-nav .fns-scrim,
  body.drawer-toc .fns-scrim{
    opacity:1;pointer-events:auto;
    -webkit-backdrop-filter:blur(2px);backdrop-filter:blur(2px);
  }

  /* ── 正文占满全宽,顶部留出操作区 ───────────────────────────────── */
  body.is-post .layout,body.is-page .layout{
    grid-template-columns:minmax(0,1fr);
    /* 顶栏已经不在了,但操作区的两个按钮仍在顶部:给它们让出一条高度,
       否则正文第一行会被压在按钮下面。 */
    padding:calc(3.5rem + env(safe-area-inset-top,0px)) .9rem 3rem;
  }

  /* ── 顶部操作区:左图谱、右问答,透明 ───────────────────────────── */
  /* 不使用包裹容器:两个按钮各自是 fixed 且只有自身大小,它们之间与四周的空白天然可滑。
     一个 pointer-events:none 的空壳做不到更多,只会多一层没有内容的 DOM。 */
  body.is-post #graph-root,
  body.is-page #graph-root{display:block}   /* 本断点内原本整体隐藏。 */

  .graph-fab,.ask-fab{
    top:calc(.55rem + env(safe-area-inset-top,0px));
    bottom:auto;
    padding:.5rem;
    border-radius:50%;
    background:color-mix(in srgb,var(--card) 62%,transparent);
  }
  /* 顶部只剩图标:窄屏上文字会把按钮撑到互相挤压,而这两个图标已经是通用形状。 */
  .graph-fab .graph-fab-txt,
  .ask-fab .ask-fab-txt{display:none}
  .graph-fab{left:.85rem;right:auto}
  .ask-fab{right:.85rem;left:auto}

  /* 图谱面板:居中浮层,四周留边。
     桌面端它是右下角的停靠卡片;窄屏上那个位置既放不下图,也和两个书签抢地方。
     居中并留边之后,图的可用宽度是屏幕减去两侧边距,而书签仍然露在两侧。 */
  #graph-root .graph-panel{
    right:auto;left:50%;bottom:auto;top:50%;
    width:calc(100vw - 2rem);max-width:26rem;
    height:min(78vh,40rem);
    padding:.6rem;
    border-radius:1.15rem;
    transform:translate(-50%,-50%) translateY(14px) scale(.975);
    transform-origin:50% 50%;
    isolation:isolate;
  }
  /* 打开态必须重新给出居中的 transform —— 桌面那条 .gr-in .graph-panel{transform:none}
     会把它拉回左上角。 */
  #graph-root.gr-in .graph-panel{
    transform:translate(-50%,-50%);
  }
  #graph-canvas{height:auto;flex:1;min-height:0}

  /* 工具条恢复。
     现有规则以「桌面端专用控件在小屏上消失」为由收掉了它,
     但那条理由在这里不成立:窄屏更需要全屏,深度滑条在手机上也一样可调。 */
  .graphcard-tools{display:flex}
  /* 居中浮层,四角全圆。
     原来的 border-radius 是「上圆下平」—— 那是给贴屏幕底部的抽屉写的。
     面板移到顶部悬浮之后,下面两角就成了露在外面的直角。 */
  .ask-panel{
    right:.6rem;left:.6rem;
    top:calc(3.4rem + env(safe-area-inset-top,0px));bottom:auto;
    width:auto;height:min(74vh,38rem);
    border-radius:1.15rem;
    transform-origin:50% 0%;
    isolation:isolate;
  }
}

/* 抽屉打开时锁滚动由脚本设置 body.style.overflow,这里不重复。
   但需保证 iOS 上循环滚动不会穿透到 body。 */
body.drawer-nav,body.drawer-toc{overscroll-behavior:contain}

/* ── 图片灯箱 ────────────────────────────────────────────────────────────────
   层级取 90：高于问答面板(60)与图谱层，因为看图时它必须是唯一可交互的表面。
   舞台的 left/top/width/height 由脚本动画(FLIP)，因此过渡只写这几个属性；
   用 transform 会让圆角随缩放变形，这一点在脚本注释里说明了。
   ─────────────────────────────────────────────────────────────────────────── */
html.lb-lock{overflow:hidden}

.lb-root{
  position:fixed;inset:0;z-index:90;
  -webkit-tap-highlight-color:transparent;
}
.lb-backdrop{
  position:absolute;inset:0;
  background:color-mix(in srgb,#0c0d0e 86%,transparent);
  -webkit-backdrop-filter:blur(6px);backdrop-filter:blur(6px);
  opacity:0;transition:opacity .3s ease;
}
.lb-root.lb-in .lb-backdrop{opacity:1}

.lb-stage{
  position:absolute;overflow:hidden;
  background:transparent;
  border-radius:12px;
  /* contain:paint 让内部缩放不溢出圆角裁切区 */
  contain:paint;
  transition:left .34s cubic-bezier(.22,1,.36,1),top .34s cubic-bezier(.22,1,.36,1),
    width .34s cubic-bezier(.22,1,.36,1),height .34s cubic-bezier(.22,1,.36,1),
    border-radius .34s ease,opacity .18s ease,transform .2s ease;
  touch-action:none;
  cursor:grab;
}
.lb-stage:active{cursor:grabbing}
/* 切图时整体淡出再淡入，关闭才走几何回弹 */
.lb-stage.lb-swap{opacity:0}

.lb-img{
  display:block;width:100%;height:100%;object-fit:contain;
  opacity:0;transition:opacity .26s ease;
  transform-origin:50% 50%;
  will-change:transform;
  -webkit-user-select:none;user-select:none;
  pointer-events:auto;
}
.lb-img.lb-ready{opacity:1}

/* 骨架只在图片尚未解码完成时可见(慢网、点开还在加载的图)；
   正常路径下缩略图早已缓存，这一层会立即淡出。 */
.lb-shimmer{
  position:absolute;inset:0;border-radius:inherit;
  background:linear-gradient(100deg,
    color-mix(in srgb,#ffffff 8%,transparent) 20%,
    color-mix(in srgb,#ffffff 22%,transparent) 40%,
    color-mix(in srgb,#ffffff 8%,transparent) 60%);
  background-size:220% 100%;
  animation:lb-shine 1.15s linear infinite;
  opacity:1;transition:opacity .3s ease;
  pointer-events:none;
}
.lb-shimmer.lb-off{opacity:0}
@keyframes lb-shine{from{background-position:120% 0}to{background-position:-120% 0}}

.lb-bar{
  position:absolute;left:50%;bottom:calc(1.4rem + env(safe-area-inset-bottom,0px));
  transform:translateX(-50%);
  display:flex;align-items:center;gap:.55rem;
  padding:.35rem .5rem;border-radius:999px;
  background:color-mix(in srgb,#1a1b1c 72%,transparent);
  -webkit-backdrop-filter:blur(10px);backdrop-filter:blur(10px);
  opacity:0;transition:opacity .3s ease .06s;
}
.lb-root.lb-in .lb-bar{opacity:1}

.lb-btn{
  display:inline-flex;align-items:center;justify-content:center;
  width:2.1rem;height:2.1rem;padding:0;
  border:0;border-radius:50%;
  background:color-mix(in srgb,#ffffff 12%,transparent);
  color:#f2f2f0;cursor:pointer;
  transition:background .18s ease,transform .18s ease,opacity .18s ease;
}
.lb-btn svg{width:1.15rem;height:1.15rem}
.lb-btn:hover{background:color-mix(in srgb,#ffffff 22%,transparent)}
.lb-btn:active{transform:scale(.93)}

.lb-close{
  position:absolute;right:calc(1rem + env(safe-area-inset-right,0px));
  top:calc(1rem + env(safe-area-inset-top,0px));
  width:2.35rem;height:2.35rem;
  background:color-mix(in srgb,#1a1b1c 66%,transparent);
  -webkit-backdrop-filter:blur(10px);backdrop-filter:blur(10px);
  opacity:0;transition:opacity .3s ease .06s,background .18s ease,transform .18s ease;
}
.lb-root.lb-in .lb-close{opacity:1}

.lb-count{
  min-width:3.2rem;text-align:center;
  font-size:.75rem;font-variant-numeric:tabular-nums;
  color:color-mix(in srgb,#f2f2f0 78%,transparent);
}

@media (max-width:860px){
  .lb-bar{bottom:calc(1rem + env(safe-area-inset-bottom,0px))}
  .lb-close{right:calc(.7rem + env(safe-area-inset-right,0px));top:calc(.7rem + env(safe-area-inset-top,0px))}
}
@media (prefers-reduced-motion:reduce){
  .lb-backdrop,.lb-stage,.lb-img,.lb-shimmer,.lb-bar,.lb-btn,.lb-close{transition:none}
  .lb-shimmer{animation:none}
}
`
