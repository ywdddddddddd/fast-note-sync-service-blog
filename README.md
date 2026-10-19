# Fast Note Sync Service · 博客魔改版

> 这是 [`haierkeys/fast-note-sync-service`](https://github.com/haierkeys/fast-note-sync-service) 的个人魔改分支备份。
> **官方文档、安装说明、上游更新请直接看官方仓库** —— 本文件只讲魔改了什么。

官方的 WebSocket 同步、REST API、MCP 通道、认证与存储后端**全部保留未动**。这个分支只在一个地方生长:**在服务端直接渲染出一整套博客**，不依赖任何前端框架，不引用任何外部 CDN —— 所有模板、样式、脚本都内联在 Go 源码里，编译进那一个二进制。

配套的插件改动在 [`obsidian-fast-note-sync-blog`](https://github.com/ywdddddddddd/obsidian-fast-note-sync-blog)。

---

## 一、博客是什么形态

访问 `/blog` 得到一个静态感很强的多页站点:

| 路由 | 内容 |
| --- | --- |
| `/blog` | 首页:7 项仪表盘 + 全部笔记列表 + 目录树 + 活跃度日历 + 热门榜 + 关于我 |
| `/blog/<slug>` | 文章页:正文 + 本页目录 + 点赞/点踩/分享 |
| `/blog/about` | 简历页(由带 `page: about` 的笔记渲染) |
| `/blog/feed.xml` | RSS |
| `/api/share/file?id=<id>&share_token=<t>` | 附件(图片/音视频/PDF) |

`slug` 形如 `1.17-4567610782`(`<uid>.<share_id>-<hmac10>`),不是数据库里存的值，而是现算的。

**只渲染已发布的笔记。** 未发布的笔记不会泄漏到博客的任何位置 —— 这一条在转写、图谱、热门、搜索里各校验一次。

---

## 二、渲染管线

`renderNoteBody` 是唯一的正文渲染入口。**顺序是约定，不是随手排的**:

```
1. frontmatter            剥掉 YAML 头
2. expandTransclusions    ![[笔记]] → 目标正文(深度 3，防环)
3. rewriteWikiLinks       [[笔记]] → /blog/<slug>；残留 ![[x]] → 纯文本
4. preprocessMath         数学公式
5. preprocessDynamicBlocks dataview / js 块
6. preprocessCallouts     > [!note] 折叠块
7. 页面容器                 ::: profile-head / profile-meta / timeline / links
8. goldmark               Markdown → HTML
```

### 为什么是这个顺序

`expandTransclusions` 必须第一个跑:它消费 `![[Note]]` 这种语法，任何在它之前跑的步骤都可能改写或删掉那些括号。它插入的是 **Markdown 源码** 而不是 HTML，所以后续每一步都会把被嵌入的笔记当成"这篇笔记自己的文字"来处理 —— 行为因此天然一致。

`rewriteWikiLinks` 会**无条件**把残留的 `![[x]]` 变成纯文本。对笔记嵌入这是对的(第 2 步没能展开，说明目标未发布)；对**文件**则错了。所以附件重写必须在它之前完成 —— 见下一节。

### 附件重写

`![[图片.png]]` 与 `![](路径)` 走两条不同的路:

- 分享页经 `GetSharedNote` → `resolveSharedNoteFiles` → 附件重写，两者都能出图
- 页面路径(RenderBlogPage)原本**直接调 renderNoteBody**，绕过了那一步，于是 `![[图片]]` 落进 `rewriteWikiLinks` 手里变成文件名文字

现在页面路径也走那三步:解析短链 → 签发分享 token → 取回重写后的正文。**新增任何"页面类"渲染入口时，附件重写不会自动发生。**

### 标题 id

goldmark 内置的 id 生成器会丢弃所有多字节字符，中文标题全部塌成 `heading`、`heading-1`。现在用自建生成器，并且**目录与锚点在同一次遍历里编号**为 `s-N`，两者不可能对不上。

早期试过"中文标题转 id"，结果 `href="#1-%e7%89%99..."` 与 `id="1-牙发育异常..."` 永远不相等 —— `html/template` 在属性上下文里会 percent-encode 非 ASCII，`template.URL` 也挡不住。

---

## 三、Markdown 方言

判据是"Obsidian 里怎么写，博客就怎么显示"。

| 语法 | 处理 |
| --- | --- |
| `==高亮==` | 自建 mark 扩展(goldmark v1.8.6 没有)。**定界符最小长度 2** |
| 单换行 | `html.WithHardWraps()` — 单换行渲染为 `<br>`，与 Obsidian 默认一致 |
| `![[笔记]]` | 转写目标正文，仅已发布，深度 3，防环 |
| `![[图片]]` / `![](路径)` | 改写为 `/api/share/file?id=...` |
| `> [!note]` | callout |
| `$...$` / `$$...$$` | 数学公式 |
| ` ```dataview ` | 动态块 |
| `::: profile-head` 等 | 页面容器(仅页面笔记) |

### 两个曾经的真实缺陷

**单个 `=` 触发高亮。** `ScanDelimiter(line, before, 1, ...)` 的第三个参数是最小连续长度，原本传 `1`，于是单个等号就是合法定界符。一行里出现两个 —— 而正文里到处是(`头部=颅部+面部`、`替牙间隙 =（Ⅲ+Ⅳ+Ⅴ）－（3+4+5）`)—— 就被配成一对，中间整段渲染成 `<mark>`。实测这个仓库里 **15 篇**笔记含孤立等号、**0 篇**含 `==`，所以高亮功能不是"没被使用"，而是在普通文本上持续误触发。改成 `2` 之后 `=` 回归普通字符，`==` 仍然高亮。

**单换行折叠成空格。** CommonMark 规定单换行不产生新段落，而 Obsidian 的默认行为恰恰相反。实测 **114 篇里有 69 篇**用单换行书写，最严重的一篇有 192 处 —— 也就是说博客的大多数文章都在整篇挤成一块。启用 `WithHardWraps` 之后与 Obsidian 一致。

---

## 四、页面容器

带 `page: about` frontmatter 的笔记会被渲染成独立页面，可用四个区块:

```
::: profile-head      姓名与一句话
::: profile-meta      基本信息(两列网格，窄屏一列)
::: timeline          时间线
::: links             联系方式
```

机制:标记先以 **HTML 注释** 的形态穿过 Markdown 渲染，渲染完成后换成真正的 `<div class="pc pc-xxx">`。不能直接写 `<div>` —— goldmark 把原始 HTML 块视为"直到空行才结束"，紧跟着的列表会被吞成 HTML 而不是变成 `<ul>`。

不在白名单里的名字**原样保留**在输出里，不是被静默丢弃:写错名字的人应当在页面上看到那些标记，而不是纳闷内容去哪了。

---

## 五、互动

### 点赞 / 点踩 / 访问次数

存在 `storage/logs/blog-reactions.json`:

```json
{ "visits": 216, "notes": { "<slug>": { "like": 3, "dislike": 0 } } }
```

并发正确性靠**一把互斥锁包住整个读-改-写**，落盘用临时文件 + rename。文件损坏时保留一份 `.corrupt` 再重置，不静默丢数据。

**不加表**的理由:这张数据每次点赞都要写，而点赞是博客上最热的写操作；用一张表意味着每个赞一行事务，而它总量只是"几百个计数"。代价是不能 SQL 查询 —— 而这里从来不需要。

`visits` 是**单调计数器**，每次成功渲染 +1，不是从访问日志求和。它叫「访问次数」而不是「阅读量」，因为它把刷新和爬虫也算进去了 —— 名字应当诚实。

`POST /blog/react` **公开**，因为博客本身公开。返回权威计数，所以两个标签页点同一个赞结果一致。`kind` 校验，`delta` 钳在 ±1。前端用 localStorage 记"这台设备的偏好"，**刻意不做跨设备去重** —— 那需要账号体系。

### 热门榜

纯点赞数，取前 5，点赞相同时新的在前。**点赞为 0 时整块不渲染**，而不是显示一个空标题。

---

## 六、首页动效

### 全屏粒子场

节点数 = 仪表盘「公开笔记」那同一个数，所以两处不可能在"有多少篇笔记"上互相矛盾 —— 它们不是两个接近的值，而是同一个值。

两个非显然的决定:

**连线距离由数据算**:`clamp(sqrt(w*h/COUNT)*1.5, 90, 190)`。参照配置写的固定 `100` 是按 200 个粒子调的；112 个粒子在 1440×950 上平均最近邻间距约 110px，**比范围还大**，几乎画不出连线，整片场看起来像散落的灰尘。固定值在两端都会坏(165px 在 112 时刚好，400 时成实心网)。

**初始分布用抖动网格而非随机**:完全随机会留团块与空斑，读起来是"随机"而不是"一片场"；间距均匀才让连线读成一张网。

硬上限 **220**(连线是 O(n²)，超过之后文章列表会开始卡)，封顶时在控制台说明一句。

### 星域穿越

点「关于我」卡片时播放星域推进(`z -= 4` 投影，300 颗星)再进入简历页。这是**跨文档过渡**(先播放再跳转)，不是客户端换页 —— 因此后退按钮、地址栏、直接访问走的都是真实路径。一次性 `sessionStorage` 标记；Ctrl/⌘/Shift/Alt/中键点击不劫持。

### 加载动画

纯 CSS `nth-child` 延迟，无 JS、无闪烁。延迟在 12 项处封顶 —— 112 篇 × 38ms 是 4 秒，那是加载而不是动画。

---

## 七、知识图谱

d3-force 物理 + d3-drag 拖动 + d3-zoom 缩放，不是手写的。入口在右下(桌面)/ 顶部左(移动端)。

- **局部图谱**:当前笔记的邻居，深度可调(1–3)，节点上限 400
- **全局图谱**:浮层，可全屏

图谱是**条件渲染**的 —— 一篇没有邻居的笔记没有图谱入口。这是数据决定的，不是缺陷。

`frontend/static/js/d3-graph.min.js` 是构建产物，用 `?v=<sha256[:12]>` 做内容指纹。**它原地覆盖，所以没有指纹时每个读者会一直用首次访问缓存的那一份。**

---

## 八、移动端

全站只有一个断点:**860px**。

阅读页(`.is-post` / `.is-page`):

- **顶栏让位** —— 隐藏品牌与面包屑。窄到只能容下一行时，「你在哪」不如「能去哪」重要，而面包屑在文章标题上方已经重复了一次
- **顶部操作区** —— 左图谱、右问答，两个圆形磨砂按钮。没有包裹容器:它们各自是 `fixed` 且只有自身大小，之间的空白天然可滑
- **边缘书签** —— 贴在屏幕垂直中线，左=全局目录、右=本页目录。点开滑出抽屉，遮罩覆盖正文，被点那侧的书签隐藏、另一侧保持可见

**首页不参与** —— 它的侧栏是内容本身(目录树、日历、热门)，不是阅读时可以临时让开的东西。

三个踩过的坑写在这里，因为它们会再犯:

1. **侧栏由 CSS 直接变成抽屉，不复制内容** —— 那些内容服务端已经渲染了一份
2. **书签 z-index 59 > 抽屉 58 > 遮罩 57**，书签必须在遮罩之上，否则"打开一个再点另一个切换"没有入口
3. **同一个 z-index 数字在两个堆叠上下文里不可比** —— `.shell` 带 `z-index:1`，它的子元素设 58 只在内部有意义；而遮罩挂在 `body` 下、57，比较的是根上下文，57 > 1，于是遮罩把整个 `.shell`(连同抽屉)一起压住。**数字没写错，是它们不在同一个上下文里。** 遮罩必须和抽屉同级(都放在 `.layout` 内)

---

## 九、数据存储

**全部在 `storage/`。这是数据，不是产物。**

```
storage/
  database/
    db.sqlite3                      user / auth_token / auth_token_log
    db_user_1.sqlite3               note(元数据，正文不在这里)
    db_user_share_1.sqlite3         user_share
    db_user_file_1.sqlite3          file(附件元数据)
    db_user_sync_log_1.sqlite3      同步日志
  vault/u_<uid>/note/n_<id>/
    content.txt                     正文(权威)
    snapshot.txt                    客户端快照
  vault/u_<uid>/file/f_<id>/file.dat 附件二进制
  logs/blog-reactions.json          点赞/点踩/访问计数
```

三条必须知道的约定:

1. **笔记正文不在数据库里。** `note.content` 恒为空串，正文在 `content.txt`。判断内容是否同步，永远看 `content.txt`
2. **附件二进制在 `f_<fileID>/file.dat`。** `file.save_path` 那一列**永远是空的** —— 路径在读取时由 `GetFileFolderPath` 惰性算出。看到 `save_path` 为空**不是**附件丢了
3. **`snapshot.txt` 不是"上一版"，是客户端当前快照**

---

## 十、构建与运行

```bash
# 依赖
export GOPROXY=https://goproxy.cn,direct
export GOSUMDB=sum.golang.google.cn

# 编译(注意是 . 而不是 ./...)
go build -o fast-note-sync-service .

# 运行
./fast-note-sync-service -c config/config.yaml
```

健康检查:`GET /api/health`。

### 重建 d3 包

`frontend/static/js/d3-graph.min.js` 是 d3-force / d3-drag / d3-zoom 的最小打包。改完之后必须**重新生成资源指纹**(`blog_assets_gen.go` 里的 `d3GraphAssetVersion`),否则读者永远用缓存里那份。

---

## 十一、测试

```bash
go test ./internal/routers/ ./internal/service/
```

`internal/routers` 全绿。`internal/service` 有一条**预存在的失败**:`TestGitSyncService_UpdateConfig_InvalidatesCache` —— 与本分支的改动无关，长期未修。

有价值的测试集中在:

- `mark_extension_test.go` — 锁住 `=` 不触发、`==` 仍高亮、`===` 被拒、未闭合不吞后文、hardWrap 不破坏块级结构
- `blog_reaction_test.go` — 含 200 goroutine 并发点赞不丢写
- `blog_page_*_test.go`、`blog_heading_ids_test.go`、`blog_transclusion_test.go`

---

## 十二、与上游的关系

本仓库 `origin` 指向这份备份，`upstream` 指向官方:

```bash
git fetch upstream
git log --oneline upstream/master ^HEAD   # 看上游多了什么
git merge upstream/master                 # 合并
```

魔改集中在两类文件里，合并冲突基本可预期:

- **新增文件**:`internal/routers/blog_*.go`、`internal/service/blog_*.go`、`internal/config/blog.go`、`internal/dto/blog_dto.go`
- **修改的上游文件**:`share_service.go`(最大)、`router_api.go`、`dao/note_repository.go`、`middleware/access_log.go`

---

## 十三、已知问题

1. **`config.yaml` 有两个 `blog:` 段** —— 一个四空格缩进(死的)、一个顶格(生效的)。改错不会报错，只表现为"配置不生效"。建议合并
2. **`home.Nav.Pages` 为空的原因未查明** —— 首页顶栏原本想放「关于」入口，`len(.Pages)==0` 而 `len(.Nav)==3`。绕过做法是改调 `GetBlogNav(...).Pages`。功能正常，但这个空值将来可能以别的形式再冒出来
3. **`TestGitSyncService_UpdateConfig_InvalidatesCache`** 长期失败，与博客无关
4. **附件数触到 `attachmentSyncLimit=500`** —— vault 2 的 `file_count` 正好 500，新的附件元数据会登记但可能排不进同步队列。附件二进制本身正常
5. **窗口很矮时侧栏仍可能与浮动按钮重叠** —— `.rail{padding-bottom:6rem}` 是固定值

---

## 许可

与上游一致(见 `LICENSE`)。本仓库是个人备份，未向上游提 PR。
