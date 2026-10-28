package dto

// BlogEntryDTO one entry of the blog index
// BlogEntryDTO 博客索引中的单条条目
type BlogEntryDTO struct {
	Slug      string `json:"slug"`      // Short link slug // 短链 slug
	Title     string `json:"title"`     // Entry title // 标题
	Path      string `json:"path"`      // Source note path // 来源笔记路径
	CreatedAt string `json:"createdAt"` // Creation date // 创建日期
	Summary   string `json:"summary"`   // Plain-text excerpt // 纯文本摘要
	Words     int    `json:"words"`     // Character count // 字数
	LinksOut  int    `json:"linksOut"`  // Outgoing wiki links // 出链数
	LinksIn   int    `json:"linksIn"`   // Incoming wiki links // 入链数
	Vault     string `json:"vault"`     // Vault name // 仓库名

	// Cover is the resolved cover image or video, or nil when the note declares none — or
	// declares one that could not be found. An absent cover is not an error state; the card
	// draws a placeholder instead.
	//
	// Cover 是解析出的封面图片或视频；笔记没有声明、或声明的文件找不到时都是 nil。
	// 没有封面不是错误状态，卡片会画一个占位。
	Cover *BlogCoverDTO `json:"cover,omitempty"`
}

// BlogCoverDTO is one article's cover plus the crop the author chose.
//
// The fields map 1:1 onto CSS: URL becomes the img/video src, FocusX/FocusY become
// object-position, and Zoom becomes the scale applied inside the square. Keeping them separate
// rather than pre-composing a transform string means the template never has to parse anything.
//
// BlogCoverDTO 是一篇文章的封面以及作者选择的裁切。
//
// 字段与 CSS 一一对应：URL 成为 img/video 的 src，FocusX/FocusY 成为 object-position，
// Zoom 成为正方形内部施加的缩放。把它们分开而不是预先拼成一个 transform 字符串，
// 使模板永远不需要解析任何东西。
type BlogCoverDTO struct {
	URL    string  `json:"url"`
	Kind   string  `json:"kind"`   // image | video // 图片或视频
	FocusX float64 `json:"focusX"` // percent, 0-100 // 百分比
	FocusY float64 `json:"focusY"` // percent, 0-100 // 百分比
	Zoom   float64 `json:"zoom"`   // multiplier, 1-4 // 倍数
}

// BlogNavItem one note inside a navigation group
// BlogNavItem 导航分组中的单篇笔记
type BlogNavItem struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
	Path  string `json:"path"`
}

// BlogNavGroup a navigation group, keyed by its source folder
// BlogNavGroup 导航分组，按来源目录聚合
type BlogNavGroup struct {
	Name  string        `json:"name"`  // Folder name // 目录名
	Items []BlogNavItem `json:"items"` // Notes in this folder // 该目录下的笔记
}

// BlogNavNode is one node of the folder tree. A node with a Slug is a note; a node with
// Children is a folder. Folders carry their own note count so the template can render the
// count without walking children.
// BlogNavNode 目录树中的一个节点：带 Slug 的是笔记，带 Children 的是文件夹。
// 文件夹自带笔记数，模板无需遍历子节点即可渲染计数。
type BlogNavNode struct {
	Name     string        `json:"name"`
	Slug     string        `json:"slug,omitempty"`
	Children []BlogNavNode `json:"children,omitempty"`
	Count    int           `json:"count"`
	Active   bool          `json:"active,omitempty"` // is the current page // 是否为当前页
	// Depth is 1 for a top-level folder. The template uses it to decide which folders start
	// expanded: a tree that opens every folder at once makes the reader scroll past a screenful of
	// text to reach a栏目, while a tree that opens none makes every article two clicks away.
	//
	// Depth 在顶层文件夹为 1。模板用它决定哪些文件夹默认展开：
	// 全部展开会让读者滚过一屏文字才能到达某个栏目；全不展开则每篇文章都要两次点击。
	Depth int `json:"depth,omitempty"`
}

// BlogDayDTO one day of the activity heat map.
// BlogDayDTO 活跃度热力图中的一天。
type BlogDayDTO struct {
	Date  string `json:"date"`  // yyyy-MM-dd
	Count int    `json:"count"` // notes touched that day // 当日有改动的笔记数
	Week  int    `json:"week"`  // column index // 列索引
	Dow   int    `json:"dow"`   // day of week, 0=Sunday // 星期几，0=周日
	Lvl   int    `json:"lvl"`   // intensity level 0-4 // 强度等级 0-4
}

// BlogActivityDTO the whole heat map plus its headline counters.
// BlogActivityDTO 热力图及其头部统计。
type BlogActivityDTO struct {
	Days     []BlogDayDTO `json:"days"`
	Weeks    int          `json:"weeks"`
	NoteDays int          `json:"noteDays"` // days with at least one note // 有笔记的天数
	Max      int          `json:"max"`      // busiest day count // 单日最多
	Months   []string     `json:"months"`   // month labels under the grid // 网格下方的月份标签
}

// BlogNavDTO the whole left-hand navigation tree.
// BlogNavDTO 左侧导航树的全部内容。
// BlogNavPageDTO is one page-note entry point in the top bar.
//
// Pages are deliberately not part of Tree: the tree is the folder structure of articles, and a page
// has no folder position that means anything. Listing them separately is what lets the top bar
// offer them without the sidebar pretending they live somewhere.
//
// BlogNavPageDTO 是顶栏里的一个页面笔记入口。
//
// 页面刻意不放进 Tree：目录树是文章按文件夹的组织方式，而页面不属于任何一个有意义的文件夹位置。
// 分开列出，才让顶栏可以给出这些入口，而侧栏不必假装它们住在某处。
type BlogNavPageDTO struct {
	Alias string `json:"alias"` // e.g. "about" // 如 about
	Title string `json:"title"` // Display label // 显示名称
	URL   string `json:"url"`   // e.g. "/blog/about" // 如 /blog/about
}

type BlogNavDTO struct {
	Tree  []BlogNavNode    `json:"tree"`
	Total int              `json:"total"`
	Pages []BlogNavPageDTO `json:"pages"`
}

// BlogGraphNode one node of the knowledge graph
// BlogGraphNode 知识图谱中的一个节点
type BlogGraphNode struct {
	ID    string  `json:"id"`    // Stable node id // 稳定节点 id
	Label string  `json:"label"` // Display label // 显示名称
	URL   string  `json:"url"`   // Target URL; empty when not published // 目标地址；未发布时为空
	X     float64 `json:"x"`     // Layout coordinate // 布局坐标
	Y     float64 `json:"y"`     // Layout coordinate // 布局坐标
	R     float64 `json:"r"`     // Radius, derived from degree // 半径，由度数推导
	Deg   int     `json:"deg"`   // Degree // 度数
	Lit   bool    `json:"lit"`   // Published // 是否已发布
	// Hop is the distance from the graph's centre, in links. The client's force model uses it
	// directly: a one-hop neighbour is pulled close, a three-hop one is pushed out. That is what
	// makes "the closer the relationship, the nearer the node" true on screen rather than only in
	// the data.
	//
	// Hop 是该节点到图中心的跳数。客户端力模型直接使用它：一跳邻居被拉近，三跳被推远。
	// 这正是「关联越深越近」在屏幕上成立、而不只是数据上成立的原因。
	Hop int `json:"hop"`
	// Focus marks the node the current view is centred on (the note being read). It is
	// deliberately separate from Lit: Lit answers "is this note published", which is true for
	// every node in a published-only graph, so using it to mean "this one is special" made
	// every node special — which is how all 112 captions ended up drawn on top of each other.
	// Focus 标记当前视图所聚焦的节点（正在阅读的那篇笔记）。它与 Lit 刻意分开：
	// Lit 回答「这篇是否已发布」，在只含已发布内容的图里对每个节点都为真，
	// 因此用它表示「这个特殊」会让每个节点都特殊——112 个标签正是这样叠在一起的。
	Focus bool `json:"focus"`
}

// BlogAccessItem is one visit as the management panel sees it.
//
// There is no field for a referrer, a cookie, a session or a full user agent, and that absence is
// the design: a value the server never stores cannot be leaked by the panel, by a backup, or by
// anyone who later reads the file.
//
// BlogAccessItem 是管理面板所看到的一次访问。
//
// 这里没有来源页、cookie、会话或完整 User-Agent 的字段，而这种缺席正是设计：
// 服务端从未存下的值，不会被面板、备份或日后读取该文件的任何人泄露。
type BlogAccessItem struct {
	Time   string `json:"time"`   // UTC, second precision // UTC，秒级
	Method string `json:"method"` // HTTP method // HTTP 方法
	Path   string `json:"path"`   // Path only, query string discarded at write time // 仅路径，查询串在写入时已丢弃
	Status int    `json:"status"` // HTTP status // HTTP 状态码
	MS     int64  `json:"ms"`     // Duration in milliseconds // 耗时（毫秒）
	IP     string `json:"ip"`     // Masked at write time // 写入时已打码
	Client string `json:"client"` // Browser family only // 仅浏览器族
}

// BlogAccessCount is one row of an aggregate.
// BlogAccessCount 是聚合结果的一行。
type BlogAccessCount struct {
	Path  string `json:"key"`
	Count int    `json:"count"`
}

// BlogAccessSummary is what an owner actually reads: totals, not raw visits.
// BlogAccessSummary 是主人真正要看的内容：总计，而不是原始访问。
type BlogAccessSummary struct {
	Total     int               `json:"total"`
	Status4xx int               `json:"status4xx"`
	Status5xx int               `json:"status5xx"`
	ByPath    []BlogAccessCount `json:"byPath"`
	ByDay     []BlogAccessCount `json:"byDay"`
	Uniques   int               `json:"uniques"`
	Note      string            `json:"-"`
}

// BlogPreviewDTO is the hover-card payload for one internal link.
//
// It intentionally carries no body text: a preview needs to tell the reader what a link points
// at, and pulling an excerpt would mean reading the target note's content file for every link on
// every page. Title, path and vault are already in the metadata index and answer the question.
//
// BlogPreviewDTO 是单条内部链接的悬停卡片数据。
//
// 它刻意不携带正文：预览需要告诉读者链接指向什么，而抓取摘录意味着为每个页面上的每条链接
// 去读目标笔记的正文文件。标题、路径与仓库已在元数据索引里，足以回答这个问题。
type BlogPreviewDTO struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Path  string `json:"path"`
	Vault string `json:"vault"`
}

// BlogGraphEdge a directed link between two notes
// BlogGraphEdge 两篇笔记之间的一条有向链接
type BlogGraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	// Dir is the link's direction, kept so the renderer can draw an arrowhead instead of a bare
	// line. A knowledge graph's whole point is which note points at which, and an undirected line
	// throws that away.
	//
	// Dir 是链接方向，保留它使渲染器能画出箭头而不是一条裸线。
	// 知识图谱的意义正在于「哪篇指向哪篇」，无向线把这个信息丢掉了。
	Dir string `json:"dir,omitempty"`
	// Hop is the deeper of the edge's two endpoints, so an edge's spring length matches the
	// further node it connects.
	//
	// Hop 是该边两个端点中较深的一个，使边的弹簧长度与它所连接的更远那个节点一致。
	Hop int `json:"hop"`
}

// BlogGraphDTO a pre-laid-out knowledge graph, ready to be emitted as SVG
// BlogGraphDTO 已完成布局的知识图谱，可直接输出为 SVG
type BlogGraphDTO struct {
	Nodes         []*BlogGraphNode `json:"nodes"`
	Edges         []*BlogGraphEdge `json:"edges"`
	Width         int              `json:"width"`
	Height        int              `json:"height"`
	Truncated     bool             `json:"truncated"`     // Node budget reached // 是否触达节点上限
	PublishedOnly bool             `json:"publishedOnly"` // Whether the scope was restricted to published notes // 作用域是否限于已发布笔记
}

// BlogTagDTO one tag with its occurrence count
// BlogTagDTO 单个标签及其出现次数
type BlogTagDTO struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// BlogStatsDTO dashboard statistics shown on the blog home page
// BlogStatsDTO 博客首页展示的仪表盘统计
type BlogStatsDTO struct {
	Notes    int    `json:"notes"`    // Published notes // 已发布笔记数
	Words    int    `json:"words"`    // Total characters // 总字数
	Links    int    `json:"links"`    // Total wiki links among published notes // 已发布笔记间的链接总数
	Tags     int    `json:"tags"`     // Distinct tags // 标签种数
	Folders  int    `json:"folders"`  // Distinct source folders // 来源目录数
	AvgWords int    `json:"avgWords"` // Average characters per note // 篇均字数
	Updated  string `json:"updated"`  // Most recent source change // 最近一次源文件变更时间
	// WordsText is Words in a human-sized form ("82.8万"). The raw count is six digits, which
	// does not fit the single-line status strip that the mobile home page shows in place of the
	// seven-cell dashboard; on the desktop the raw number stays the authority.
	//
	// WordsText 是 Words 的人读形式（"82.8万"）。原始数字是六位数，塞不进移动端首页那一行
	// 状态条——它在窄屏上取代了原来七格仪表盘。桌面端仍以原始数字为准。
	WordsText string `json:"wordsText"`
}

// BlogHomeDTO everything the blog home page needs
// BlogHomeDTO 博客首页所需的全部数据
type BlogHomeDTO struct {
	Title    string           `json:"title"`
	HomeURL  string           `json:"homeUrl"`
	Stats    *BlogStatsDTO    `json:"stats"`
	Tags     []*BlogTagDTO    `json:"tags"`
	Entries  []*BlogEntryDTO  `json:"entries"`
	Nav      *BlogNavDTO      `json:"nav"`
	Graph    *BlogGraphDTO    `json:"graph"`
	Activity *BlogActivityDTO `json:"activity"`
}

// BlogPostDTO a rendered blog post
// BlogPostDTO 渲染后的博客文章
type BlogPostDTO struct {
	Slug      string `json:"slug"`      // Short link slug // 短链 slug
	Title     string `json:"title"`     // Post title // 标题
	HTML      string `json:"html"`      // Rendered HTML body // 渲染后的 HTML 正文
	Path      string `json:"path"`      // Source note path // 来源笔记路径
	CreatedAt string `json:"createdAt"` // Creation date // 创建日期
}

// BlogPageDTO a rendered page note (about). A page is a note that carries a `page:` marker in its
// frontmatter; it renders at an alias route and is kept out of the article list, the feed, the
// graph and the statistics.
//
// Slug is carried as well as Alias on purpose. The alias is the stable address a reader can
// remember, while the slug is what the note actually is — the page can be opened either way, and
// both must agree about which note they show.
//
// BlogPageDTO 渲染后的页面笔记（关于）。页面是 frontmatter 里带 `page:` 标记的笔记；
// 它渲染在别名路由上，并被排除在文章列表、订阅源、图谱与统计之外。
//
// 同时带上 Slug 与 Alias 是有意的。别名是读者能记住的稳定地址，而 slug 才是这篇笔记本身——
// 两种地址都能打开它，且两者必须指向同一篇。
type BlogPageDTO struct {
	Alias string `json:"alias"` // Page alias, e.g. "about" // 页面别名，如 about
	Slug  string `json:"slug"`  // Short link slug of the underlying note // 底层笔记的短链 slug
	Title string `json:"title"` // Page title // 标题
	HTML  string `json:"html"`  // Rendered HTML body // 渲染后的 HTML 正文
	Path  string `json:"path"`  // Source note path // 来源笔记路径
	// Avatar is the frontmatter avatar path, or "" — the template omits the element entirely
	// rather than rendering an empty placeholder.
	// Avatar 是 frontmatter 声明的头像路径，可能为空——模板会整块省略而不是渲染一个空占位。
	Avatar string `json:"avatar"`
	// UpdatedAt is the note's own time, shown as "最后更新".
	// UpdatedAt 是笔记自身的时间，显示为「最后更新」。
	UpdatedAt string `json:"updatedAt"`
	// Duplicates counts other published notes declaring the same alias. More than zero means an
	// author may be editing a file that is not the one being shown, so the settings page says so.
	// Duplicates 是声明了同一别名的其它已发布笔记数。大于零意味着作者可能在编辑另一份文件，
	// 因此设置页会说明这件事。
	Duplicates int `json:"duplicates"`
}

// BlogReactionTallyDTO is one note's reaction counters.
//
// Dislike is returned as well as like: the user asked for the dislike count to be public, and a
// count that is collected but withheld is worse than either alternative — it takes the signal and
// gives nothing back.
//
// BlogReactionTallyDTO 是单篇笔记的反应计数。
//
// Dislike 与 Like 一并返回：用户要求把踩数公开，而一个「收集了却扣着不给」的计数比两种做法都糟——
// 它拿走了信号，什么也没给回来。
type BlogReactionTallyDTO struct {
	Like    int64 `json:"like"`
	Dislike int64 `json:"dislike"`
}

// BlogReactionsDTO carries every tally plus the run-total visit count.
//
// Visits is monotonic. It is not summed from the access log because that log keeps 30 days, and a
// dashboard figure that falls because a month passed reads as a bug rather than as a window.
//
// BlogReactionsDTO 携带全部计数以及累计访问次数。
//
// Visits 只增不减。它不由访问日志求和而来，因为那份日志只保留 30 天，
// 而一个因为过了一个月就变小的仪表盘数字，会被读成故障而不是一个时间窗口。
type BlogReactionsDTO struct {
	Visits int64                            `json:"visits"`
	Notes  map[string]*BlogReactionTallyDTO `json:"notes"`
}

// BlogHotItemDTO is one entry in the popular list.
//
// BlogHotItemDTO 是热门列表里的一条。
type BlogHotItemDTO struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Like  int64  `json:"like"`
}

// BlogConfigDTO current blog settings, as reported to clients (e.g. the Obsidian plugin)
// BlogConfigDTO 当前博客设置，用于返回给客户端（如 Obsidian 插件）
type BlogConfigDTO struct {
	Enabled   bool   `json:"enabled"`   // Blog switch // 博客开关
	Title     string `json:"title"`     // Site title // 站点标题
	Indexable bool   `json:"indexable"` // Allow search-engine indexing // 是否允许收录
	ShowIndex bool   `json:"showIndex"` // List entries on the index page // 首页是否列出条目
	UID       int64  `json:"uid"`       // Published user id // 发布者 uid
	HomeURL   string `json:"homeUrl"`   // Absolute blog home URL // 博客首页完整地址

	// Graph settings. These were reachable only by editing config.yaml by hand, which meant the
	// one thing a reader notices first — whether a graph appears at all — was also the one thing
	// the settings page could not turn on.
	//
	// 图谱设置。此前只能手工编辑 config.yaml 才能改到，也就是说读者最先注意到的那件事
	// ——是否出现图谱——恰恰是设置页无法开启的那一件事。
	GraphScope      string `json:"graphScope"`      // shared | all // 仅已分享 / 全部
	MaxGraphNodes   int    `json:"maxGraphNodes"`   // Node budget for the global graph // 全站图谱节点上限
	ShowGraph       bool   `json:"showGraph"`       // Render graphs at all // 是否渲染图谱
	LocalGraphDepth int    `json:"localGraphDepth"` // Default article-graph depth, 1..3 // 文章图谱默认深度
}

// BlogConfigRequest partial update of the blog settings (nil fields are left unchanged)
// BlogConfigRequest 博客设置的部分更新（为 nil 的字段保持不变）
type BlogConfigRequest struct {
	Enabled   *bool   `json:"enabled"`   // Blog switch // 博客开关
	Title     *string `json:"title"`     // Site title // 站点标题
	Indexable *bool   `json:"indexable"` // Allow indexing // 是否允许收录
	ShowIndex *bool   `json:"showIndex"` // List entries on index page // 首页是否列出条目

	// Pointers so that a partial update stays partial: nil means "leave this alone", which is what
	// lets the panel send one changed switch instead of the whole form.
	//
	// 用指针使部分更新保持「部分」：nil 表示「不要动它」，这样面板可以只发送改动的那一个开关，
	// 而不是整张表单。
	GraphScope      *string `json:"graphScope"`
	MaxGraphNodes   *int    `json:"maxGraphNodes"`
	ShowGraph       *bool   `json:"showGraph"`
	LocalGraphDepth *int    `json:"localGraphDepth"`
}

// BlogPostItem one published post with an absolute URL
// BlogPostItem 一篇已发布文章及其完整地址
type BlogPostItem struct {
	Title     string `json:"title"`     // Post title // 标题
	URL       string `json:"url"`       // Absolute post URL // 文章完整地址
	Path      string `json:"path"`      // Source note path // 来源笔记路径
	CreatedAt string `json:"createdAt"` // Creation date // 创建日期
}
