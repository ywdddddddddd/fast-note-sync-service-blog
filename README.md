# 问道漫步 · Obsidian 笔记直出博客

在 Obsidian 里写，浏览器里看。服务端把笔记直接渲染成一整套静态感很强的站点——**没有前端框架、没有构建步骤、没有一行外部 CDN**，所有模板、样式、脚本都编译进那一个二进制。

这是 [`haierkeys/fast-note-sync-service`](https://github.com/haierkeys/fast-note-sync-service) 的个人分支。上游的 WebSocket 同步、REST API、MCP 通道、认证与存储后端全部原样保留，只在旁边长出了一层博客。

配套的 Obsidian 插件在 [`obsidian-fast-note-sync-blog`](https://github.com/ywdddddddddd/obsidian-fast-note-sync-blog)。

---

## 目录

- [它长什么样](#它长什么样)
- [三分钟跑起来](#三分钟跑起来)
- [发布第一篇文章](#发布第一篇文章)
- [frontmatter 支持哪些键](#frontmatter-支持哪些键)
- [博客功能总览](#博客功能总览)
- [写作语法对照](#写作语法对照)
- [简历页怎么写](#简历页怎么写)
- [资料书架怎么配](#资料书架怎么配)
- [AI 问答怎么配](#ai-问答怎么配)
- [配置项全表](#配置项全表)
- [移动端](#移动端)
- [日常运维](#日常运维)
- [设计说明](#设计说明)
- [已知限制](#已知限制)

---

## 它长什么样

访问 `/blog` 是一个多页站点：

| 地址 | 内容 |
| --- | --- |
| `/blog` | 首页：7 项仪表盘 + 目录树 + 文章列表 + 活跃度日历 + 标签云 + 热门榜 |
| `/blog/<slug>` | 文章页：正文 + 本页目录 + 局部知识图谱 + 点赞/点踩/分享 |
| `/blog/about` | 简历页（由带 `page: about` 的笔记渲染） |
| `/blog/shelf` | 资料书架：封面墙，点进去逐页读 PDF |
| `/blog/shelf/<id>` | 阅读器：单页栅格化阅读，前端拿不到原 PDF |
| `/blog/graph` | 全站知识图谱 |
| `/blog/rss.xml` | RSS 2.0 |

**只渲染已发布的笔记。** 没分享出去的笔记不会以任何形式出现在博客里——文章列表、目录树、标签云、图谱、全文检索各校验一次。

---

## 三分钟跑起来

### 1. 依赖

只有 Go。健康检查、SQLite 驱动、模板全部在标准库里，`CGO_ENABLED=0` 可以静态编译。

资料书架需要服务器上装 `poppler-utils`（提供 `pdftoppm`/`pdfinfo`）——这是整套东西唯一的外部依赖，且只在用书架时需要。

```bash
apt-get install -y poppler-utils
```

### 2. 编译

```bash
export GOPROXY=https://goproxy.cn,direct
export GOSUMDB=sum.golang.google.cn

CGO_ENABLED=0 go build -o fast-note-sync-service .
```

注意是 `.`，不是 `./...`——后者会连 `cmd/` 下的生成器一起编，在低配机器上容易 OOM。

### 3. 最小配置

`config/config.yaml`：

```yaml
server:
  http-port: ":9000"

blog:
  enabled: true
  title: 问道漫步
  uid: 0            # 0 = 第一个用户(uid=1)，即发布谁的分享
  indexable: true   # 允许搜索引擎收录
  show-index: true  # 首页列出文章
```

`uid` 决定博客展示哪个用户的分享。单用户部署填 `0` 就行。

### 4. 启动与验证

```bash
./fast-note-sync-service -c config/config.yaml
```

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9000/blog   # 200
curl -s http://127.0.0.1:9000/api/health
```

### 5. 反代与域名

博客是纯 HTTP，任何反代都能用。两个必须注意的点：

```nginx
location / {
    proxy_pass http://127.0.0.1:9000;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 300s;   # AI 问答是 SSE 长连接，默认 60s 会断
}
```

`X-Forwarded-For` 必须透传：问答接口按真实客户端地址做限流，不传就所有人都算成 `127.0.0.1`，一个人提问全体被限。

---

## 发布第一篇文章

博客没有"新建文章"按钮，也不需要——**「分享」就是「发布」**。

### 发布流程

1. 在 Obsidian 里写好一篇笔记，正常同步到服务端
2. 对这篇笔记创建分享（Obsidian 插件里点分享，或在 Web GUI 的分享页新建）
3. 刷新 `/blog`，文章就出现在列表里

数据上，文件落在 `db_user_share_1.sqlite3` 的 `user_share` 表，博客查的就是 `res_type='note' AND status=1` 的行。

### 文章的地址是怎么来的

```
/blog/<uid>.<share_id>-<hmac10>        形如 /blog/1.136-98a7a75922
```

`share_id` 是分享记录的 id，`hmac10` 由服务端密钥派生。整个 slug 是**每次请求现算的**，数据库里不存。

> **重新分享会换一个 `share_id`，slug 随之改变，旧链接立即失效。** 已经发出去的链接会 404。
> 想让链接稳定，就别删了重发——直接改笔记内容，分享本身不动。

### 文章标题从哪来

**文章标题 = 文件名去掉 `.md`。**

frontmatter 里写 `title:` 不会改变文章标题（只有简历页会用它，见下文）。这样设计是为了避免"改一个 frontmatter 就把整站标题全换掉"，作者想要什么标题，改文件名就好。

### 草稿

没有草稿概念。**没分享 = 草稿**——笔记留在库里同步，只是不进博客。

### 排序

按源文件的创建时间倒序。想置顶就把日期往前调（改笔记的时间属性，或直接改 `created`）。

---

## frontmatter 支持哪些键

键都在笔记开头那段 `---` 里：

```markdown
---
title: 关于我
page: about
avatar: 我的头像.png
tags: [口腔, 笔记]
---
```

| 键 | 作用 | 用在哪 |
| --- | --- | --- |
| `page` | 声明这是一张**独立页面**而不是文章 | 目前只认 `about` |
| `title` | 页面标题 | **仅页面生效**，文章标题用文件名 |
| `avatar` | 头像图片路径（相对 vault） | 页面（简历页） |
| `tags` | `[a, b]` 或 `a, b` 都行 | 标签云、文章标签 |

**标签还有第二种来源**：正文里的 `#标签`。两种会合并去重。

纯数字和纯标点的"标签"会被丢掉——这个库里笔记常用 `10.热牙胶` 这种编号，不拦的话标签云会被数字塞满。标签至少含一个字母（汉字算字母）才保留。

---

## 博客功能总览

### 首页 `/blog`

**7 项仪表盘**，数据全部来自已发布的笔记：

| 指标 | 含义 |
| --- | --- |
| 公开笔记 | 已分享的篇数 |
| 总字数 | 所有已发布笔记的字符数 |
| 双链 | 已发布笔记之间的 wiki 链接总数 |
| 标签 | 去重后的标签种数 |
| 目录 | 来源笔记所在的目录数 |
| 篇均 | 总字数 ÷ 篇数 |
| 更新 | 最近一次源文件变更时间 |

**目录树**按笔记在 vault 里的真实目录层级渲染，可折叠，当前文章所在分支自动展开。

**活跃度日历**是 GitHub 那种格子图，按天统计"当天有几篇笔记有改动"，5 级深浅。

**标签云**按出现次数排序，点进去是筛选后的文章列表。

**热门榜**取点赞前 5。点赞都是 0 时整块不渲染，而不是留一个空标题。

**全屏粒子场**是可选的装饰：节点数就是"公开笔记"那个数字，两处不可能对不上。连线距离按粒子数算出来，不是固定值。

### 文章页 `/blog/<slug>`

正文之外有四块：

**本页目录**（右侧/移动端抽屉）由标题自动生成，锚点用 `s-N` 编号，目录和锚点在同一次遍历里产生，不会对不上。

**局部知识图谱**（文章底部或浮动面板）画当前笔记的邻居，深度 1–3 可调。**没有邻居的文章没有图谱入口**——这是数据决定的，不是坏了。

**点赞 / 点踩**存在 `storage/logs/blog-reactions.json`，一把互斥锁包住整个读-改-写，落盘用临时文件 + rename，并发点赞不丢写。前端用 localStorage 记住本设备的偏好，做得刻意：跨设备去重需要账号体系，这里没有。

**图片灯箱**：正文图片点开放大。双指缩放、单指拖动、双击以指针位置为锚点缩放、滚轮缩放。图片懒加载带骨架占位。

### 简历页 `/blog/about`

由带 `page: about` 的笔记渲染，走独立模板（不是文章页）。详见[简历页怎么写](#简历页怎么写)。

### 资料书架 `/blog/shelf`

封面墙 + 逐页阅读器。**核心约束是 PDF 原始文件不进浏览器**：服务端用 `pdftoppm` 把页面栅格化成 JPEG，前端一次只拿当前页。

访客点开一本 20MB 的书，看 3 页大约 300–400KB，网络面板里没有任何 PDF。省流量和防下载是同一个动作的结果。

诚实的边界：截图、录屏拦不住；一页页翻完整本，总流量仍会接近原文件（只是拆成很多次请求）。这套设计去掉的是「一键下载」和「每次访问都付整本的钱」。

### 全站知识图谱 `/blog/graph`

d3-force 物理布局 + 拖拽 + 缩放，不是手写的。节点大小由度数决定，已发布节点可点击跳转。

`graph-scope: shared` 只画已发布的笔记；`graph-scope: all` 把未发布的也画成匿名小点——能看出连接关系，但不泄露标题。

### AI 问答

右下角（移动端在底部胶囊）的「问道」打开问答面板，基于已发布笔记做检索增强生成。详见[AI 问答怎么配](#ai-问答怎么配)。

### RSS

```
/blog/rss.xml
```

只包含已发布笔记。`indexable: false` 时页面加 `noindex`，但 RSS 照常输出。

---

## 写作语法对照

判据只有一条：**Obsidian 里怎么写，博客就怎么显示。**

| 语法 | 处理 |
| --- | --- |
| `==高亮==` | `<mark>`，定界符最少 2 个（单个 `=` 是普通字符，见下） |
| 单换行 | 渲染成 `<br>`，与 Obsidian 默认一致 |
| `![[笔记]]` | 内嵌目标正文，深度 3，防循环 |
| `![[图片.png]]` | 改写为附件地址 |
| `![](路径)` | 同上 |
| `[[笔记]]` / `[[笔记\|别名]]` | 变站内链接 |
| `> [!note]` | 折叠块（callout） |
| `$x$` / `$$x$$` | 数学公式 |
| `dataview` / `js` 代码块 | 动态块 |
| `::: profile-head` 等 | 页面容器（仅简历页） |
| `#标签` | 归入标签云 |

### 两个真实踩过的坑

**单个 `=` 曾经触发高亮。** 定界符最小长度传了 1，于是 `头部=颅部+面部`、`替牙间隙=（Ⅲ+Ⅳ+Ⅴ）－（3+4+5）` 这类行里出现两个等号就被配成一对，中间整段变成 `<mark>`。这个库里 15 篇笔记含孤立等号、0 篇含 `==`——也就是说高亮功能不是"没人用"，而是在普通文本上持续误触发。改成最小 2 个后 `=` 回归普通字符。

**单换行曾经被折叠成空格。** CommonMark 规定单换行不产生新段落，而 Obsidian 恰恰相反。实测 114 篇里 69 篇用单换行书写，最严重的一篇有 192 处——大多数文章都在整篇挤成一块。现在启用了 `WithHardWraps`。

### 中文标题的锚点

目录和锚点都用 `s-N` 形式的 ASCII id，不用标题原文。早期试过"中文标题转 id"，结果是 `href="#1-%e7%89%99..."` 和 `id="1-牙发育异常..."` 永远不相等——`html/template` 在属性上下文里会把非 ASCII percent-encode，`template.URL` 也挡不住。

---

## 简历页怎么写

建一篇笔记，frontmatter 声明 `page: about`：

```markdown
---
title: 关于我
page: about
avatar: assets/头像.png
---

::: profile-head
## 你的名字
一句话介绍自己
:::

::: profile-meta
- 坐标：城市
- 职业：职业
- 邮箱：you@example.com
:::

::: timeline
- 2024 年 · 某件事
- 2022 年 · 另一件事
:::

::: links
- [GitHub](https://github.com/you)
- [邮箱](mailto:you@example.com)
:::
```

四个容器：

| 容器 | 版式 |
| --- | --- |
| `profile-head` | 头像 + 姓名 + 一句话 |
| `profile-meta` | 两列网格（窄屏一列） |
| `timeline` | 时间线 |
| `links` | 联系方式 |

有三点要知道：

**容器标记先以 HTML 注释的形态穿过 Markdown 渲染**，渲染完再换成 `<div class="pc pc-xxx">`。不能直接写 `<div>`——goldmark 把原始 HTML 块视为"直到空行才结束"，紧跟着的列表会被吞成 HTML 而不是变成 `<ul>`。

**不在白名单里的名字原样保留**，不是被静默丢弃。写错名字的人应该在页面上看见那些标记，而不是纳闷内容去哪了。

**`page:` 只认 `about`**。别名白名单是唯一的，因为"有路由但过滤器不认"会导致笔记从文章列表消失、同时读者无处可去。要加新页面得同时改路由和白名单。

---

## 资料书架怎么配

### 目录结构

```
storage/blog-shelf/
  shelf.json                          书目（唯一需要手写的文件）
  books/<id>.pdf                      原始 PDF —— 私有，不经任何路由暴露
  cache/<id>/p<页>-<dpi>-<参数>-<戳>.jpg   按需渲染的页图
  covers/<id>-<参数>-<戳>.jpg             封面
```

### shelf.json

```json
{
  "books": [
    {
      "id": "oral-histology",
      "title": "口腔组织胚胎学",
      "author": "作者",
      "note": "一句话简介",
      "file": "oral-histology.pdf",
      "tags": ["口腔"],
      "collection": "教材"
    }
  ]
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `id` | 是 | URL 与缓存目录名。改它等于换一本书，缓存全部重建 |
| `title` | 是 | 书名，封面墙上显示 |
| `file` | pdf 模式必填 | `books/` 下的 PDF 文件名 |
| `author` | 否 | 作者 |
| `note` | 否 | 一句话简介 |
| `tags` | 否 | 标签 |
| `collection` | 否 | 归入书架页上的一条横向轨道。留空的书会落进兜底分组，不会消失 |
| `cover` | 否 | `covers/` 下的手做封面。不给就用第一页兜底，扫描教材的第一页通常本来就是封面 |
| `mode` | 否 | `pdf`（默认）或 `pages`。`pages` 表示页面已经预先栅格化好，不走 `pdftoppm` |
| `pages` | pages 模式必填 | 页数。pdf 模式由 `pdfinfo` 数出来，不用给 |

### 注意

`file` 和 `id` 是两个概念：`id` 是这本书的标识（进 URL、进缓存路径），`file` 是 `books/` 里那个 PDF 的实际文件名。两者相同最省事，但不是必须。

`file` 会被校验，只接受安全的纯文件名——写 `../xxx` 这类路径分隔符会被拒。它没有危害（仍在 `books/` 内），但绝不会是作者的本意。

---

## AI 问答怎么配

问答是可选功能，关掉不影响博客其余部分。

```yaml
ai:
  enabled: true
  top-k: 8                      # 检索返回条数
  max-context-chars: 14000      # 送进模型的上下文上限
  cache-ttl: 5m                 # 语料缓存；调小能缩短"改完笔记立刻提问"的陈旧窗口
  max-question-chars: 500
  routes:
    - name: main
      base-url: https://api.siliconflow.cn/v1
      model: Qwen/Qwen3.5-4B
      api-key: sk-xxx
      min-interval: 10s
      timeout: 420s
      max-tokens: 3000
      enable-thinking: false
      retries: 1
  embedding:                    # 向量通道；关掉退化为 BM25 + 重排
    enabled: true
    base-url: https://api.siliconflow.cn/v1
    model: BAAI/bge-m3
    api-key: sk-xxx
    batch-size: 32
    chunk-chars: 1800           # bge-m3 硬上限 8192 token，12000 字符会 400
    chunk-overlap: 200
    candidate-limit: 60
  rerank:                       # 精排；关掉退化为 BM25
    enabled: true
    base-url: https://api.siliconflow.cn/v1
    model: BAAI/bge-reranker-v2-m3
    api-key: sk-xxx
    top-n: 8
    doc-chars: 1200
```

三级降级，缺哪级就退化到下一级：向量召回 → BM25 → 精排。只配一个 `routes` 也能用。

检索范围是**已发布的笔记**，与博客一致。

接口有两道闸：按客户端地址限频，以及同一客户端同时只能有一条回答在流。都用真实 IP 而不是可伪造的头（所以反代必须透传 `X-Forwarded-For`）。

---

## 配置项全表

`blog:` 段：

| 键 | 默认 | 说明 |
| --- | --- | --- |
| `enabled` | `false` | 博客总开关 |
| `title` | `Notes` | 站点标题，出现在浏览器标题与页头 |
| `uid` | `0` | 发布哪个用户的分享，`0` = 第一个用户 |
| `indexable` | `true` | 是否允许搜索引擎收录（关掉则输出 `noindex`） |
| `show-index` | `true` | 首页是否列出文章 |
| `graph-scope` | `shared` | `shared` 只画已发布；`all` 把未发布画成匿名点 |
| `max-graph-nodes` | `120` | 全站图谱节点上限，防布局算太久 |
| `show-graph` | `false` | 首页是否显示图谱 |
| `local-graph-depth` | `1` | 文章内图谱的初始深度（1–3） |
| `particle-count` | `150` | 点赞粒子数，范围 30–400 |
| `dislike-burst` | `true` | 点踩是否也喷粒子 |

> **配置里只能有一个 `blog:` 段。** 这个文件在历史上出现过两个同名段（一个缩进四格、一个顶格），只有一个生效，改错不报错、只表现为"配置不生效"。手改前先 `grep -n '^blog:'` 确认只有一处。

---

## 移动端

全站只有一个断点：**860px**。

### 阅读页的取舍

**顶栏让位** —— 隐藏品牌与面包屑。窄到只能容下一行时，「你在哪」不如「能去哪」重要，而面包屑在文章标题上方已经重复过一次。

**边缘书签** —— 贴在屏幕垂直中线，点开滑出抽屉。被点那侧的书签隐藏、另一侧保持可见，所以"打开左抽屉再切右抽屉"有入口。

**首页不参与** —— 它的侧栏是内容本身（目录树、日历、热门），不是阅读时可以临时让开的东西。

### 底部胶囊

每个页面底部有一枚五格胶囊，滑块跟着手指走：

| 页面 | 五格 |
| --- | --- |
| 首页 | 导航 · 热门 · 漫游 · 活跃 · 问答 |
| 文章 | 导航 · 图谱 · 漫游 · 首页 · 问答 |
| 独立页 | 首页 · 漫游 · 问答 |

「漫游」是随机跳到一篇已发布笔记。图谱、问答、抽屉三者互斥：任何一方打开都会广播一条事件，其余两方收到就关自己——只靠一条消息，不共享全局变量，所以三个脚本可以任意顺序加载。

### 软键盘

问答面板在手机上要处理软键盘。这里有一条不能省的经验：**`position: fixed` 不跟随键盘**，而不同内核给出的信号不一样——iOS 只有 `visualViewport.height` 会变小，Android 连 `innerHeight` 一起变小，Windows 微信则两个都不变但两者之间本来就有几百像素的静态差。

所以判据是「每个信号相对**自己初始值**的下降量，取较大值」，而不是两个信号互相比。互相比会在 Android 上恒为 0（两个一起缩），也会在 Windows 微信上误判成"键盘吃了 548px"——后者的表现是面板被推到一个虚构的高度，下面露出一大截模糊正文。

---

## 日常运维

### 交叉编译并部署

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -p 2 -ldflags="-s -w" -o fns .
scp fns root@<host>:/opt/fns/fns.new

ssh root@<host> '
  set -e
  systemctl stop fns; sleep 2
  fuser /opt/fns/fns && echo "file busy, abort" && exit 1 || true
  TS=$(date +%Y%m%d_%H%M%S)
  mkdir -p /opt/fns/backup/bin_$TS && cp /opt/fns/fns /opt/fns/backup/bin_$TS/fns
  chmod +x /opt/fns/fns.new
  sync; mv -f /opt/fns/fns.new /opt/fns/fns; sync
  systemctl start fns; sleep 8
  systemctl is-active fns
  systemctl show fns -p NRestarts --value
'
```

`fuser` 那一行不是形式主义：scp 断在传输中途会留下一个 `sftp-server` 进程持有写句柄，此时 `mv` 报 `Text file busy`，服务起不来。

### 回滚

```bash
systemctl stop fns
cp /opt/fns/backup/bin_<时间戳>/fns /opt/fns/fns
systemctl start fns
```

### 看状态

```bash
systemctl status fns
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9000/blog
awk '/VmRSS/{printf "%.1f MB\n",$2/1024}' /proc/$(systemctl show fns -p MainPID --value)/status
```

### 已发布多少篇

```bash
python3 -c "import sqlite3;c=sqlite3.connect('file:/opt/fns/storage/database/db_user_share_1.sqlite3?mode=ro',uri=True);print(c.execute(\"select count(*) from user_share where res_type='note' and status=1\").fetchone()[0])"
```

### 数据在哪

**全部在 `storage/`。这是数据，不是产物，备份它。**

```
storage/
  database/
    db.sqlite3                      user / auth_token
    db_user_1.sqlite3               note（元数据）
    db_user_share_1.sqlite3         user_share
    db_user_file_1.sqlite3          file（附件元数据）
  vault/u_<uid>/note/n_<id>/
    content.txt                     正文（权威）
    snapshot.txt                    客户端当前快照
  vault/u_<uid>/file/f_<id>/file.dat  附件二进制
  blog-shelf/                       资料书架（PDF + 缓存）
  logs/blog-reactions.json          点赞 / 点踩 / 访问计数
```

三条必须知道的约定：

`note.content` 列恒为空串，正文在 `content.txt`。判断内容是否同步，永远看 `content.txt`。

`file.save_path` 列恒为空，附件路径在读取时惰性算出。看到它为空**不是**附件丢了。

`snapshot.txt` 不是"上一版"，是客户端当前快照。

### 静态资源缓存

`/blog/ask.js`、`/blog/lightbox.js`、`/blog/reader.js` 都带 `?v=<进程启动时刻>`。这是必要的：源站发的是 `no-cache`，但微信 webview 和部分代理并不总遵守，部署后客户端仍在跑旧脚本，表现就是「明明改好了，页面一点变化都没有」。URL 一变，任何缓存层都绕不过去。

---

## 设计说明

想改代码的人看这几条，都是踩出来的。

**渲染顺序是约定。** `frontmatter → 展开内嵌 → 改写 wiki 链接 → 数学 → 动态块 → callout → 页面容器 → goldmark`。内嵌必须第一个跑：它消费 `![[...]]` 语法，任何在它之前跑的步骤都可能改写那些括号。它插入的是 **Markdown 源码**而不是 HTML，所以后续每一步都会把被嵌入的内容当成"这篇笔记自己的文字"处理，行为天然一致。

**附件重写必须在 wiki 链接重写之前。** `rewriteWikiLinks` 会无条件把残留的 `![[x]]` 变成纯文本——对笔记内嵌这是对的（没展开说明目标未发布），对图片就错了。**新增任何"页面类"渲染入口时，附件重写不会自动发生。**

**图谱是条件渲染的。** 一篇没有邻居的笔记没有图谱入口，这是数据决定的。

**`d3-graph.min.js` 是构建产物**，改完必须重新生成资源指纹，否则读者永远用首次访问缓存的那份。

**移动端 z-index 有个坑要记住**：同一个 z-index 数字在两个堆叠上下文里不可比。`.shell` 带 `z-index:1`，它的子元素设 58 只在内部有意义；而遮罩挂在 `body` 下、57，比较的是根上下文，57 > 1，于是遮罩会把整个 `.shell`（连同抽屉）一起压住。数字没写错，是它们不在同一个上下文里。遮罩必须和抽屉同级。

---

## 已知限制

**没有评论系统。** 没有内嵌评论、邮件通知、Webhook。

**没有自定义 CSS/HTML 注入。** 想要就改模板重编译。

**没有协作者权限。** 单用户模型，`uid` 决定发布谁的内容。

**没有账号系统。** 点赞去重靠 localStorage，换设备就是新访客。

**重新分享会换链接。** 见上文。

**窗口很矮时侧栏可能与浮动按钮重叠。** `.rail{padding-bottom:6rem}` 是固定值。

**`indexable: true` 时 RSS 和页面都会对搜索引擎开放。** 不想被收录就关掉。

---

## 与上游的关系

```bash
git remote add upstream https://github.com/haierkeys/fast-note-sync-service
git fetch upstream
git log --oneline upstream/master ^HEAD    # 看上游多了什么
git merge upstream/master
```

改动集中在两类文件，合并冲突基本可预期：

新增：`internal/routers/blog_*.go`、`internal/service/blog_*.go`、`internal/config/blog.go`、`internal/dto/blog_dto.go`

修改的上游文件：`share_service.go`（最多）、`router_api.go`、`dao/note_repository.go`、`middleware/access_log.go`

测试：

```bash
go test ./internal/routers/ ./internal/service/
```

`internal/service` 里有一条预存在的失败 `TestGitSyncService_UpdateConfig_InvalidatesCache`，与博客无关，长期未修。

---

## 许可

与上游一致，见 `LICENSE`。本仓库是个人分支，未向上游提 PR。
