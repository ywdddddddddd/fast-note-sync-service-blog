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

	// Initial and InitialHue drive the placeholder cover a card draws when the note has no image.
	//
	// The reference site gives every card a faded cover; this vault has a cover declared on two
	// notes out of nine hundred, so most cards would be plain text and the whole card rhythm would
	// disappear. Rather than invent an image, the card draws a warm gradient block with the
	// title's first character. The hue is derived from that character, so cards differ from each
	// other while staying inside the site's warm range — and the moment a note gains a real cover
	// the placeholder is simply not rendered.
	//
	// Initial 与 InitialHue 驱动卡片在没有图片时绘制的占位封面。
	//
	// 参照站每张卡都有淡化封面；这个仓库九百多篇里只有两篇声明了 cover，
	// 于是多数卡片会是纯文字，整页的卡片节奏就消失了。与其凭空造一张图，
	// 卡片画一块暖色渐变 + 标题首字。色相由该字推导，使卡片彼此不同又都落在站点的暖色区间——
	// 而一篇笔记一旦有了真封面，占位块就不再渲染。
	Initial    string `json:"initial,omitempty"`
	InitialHue int    `json:"initialHue,omitempty"`
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

	// DaysOnline is how long the blog has existed, counted from the earliest published note to
	// today. It stands in for the reference site's "Days Online" card: the start date is derived
	// from data rather than configured, so it cannot drift away from the archive.
	//
	// DaysOnline 是博客存在的天数，从最早一篇已发布笔记算到今天。
	// 它对应参照站的「Days Online」卡片：起始日期由数据推导而不是配置，因此不会与归档脱节。
	DaysOnline int `json:"daysOnline"`
	// Since is that earliest note's date, shown beside the day count so the number is checkable.
	// Since 是最早一篇的日期，与天数并排显示，使这个数字可核对。
	Since string `json:"since"`
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

	// Sections are the top-level folders, in the order the nav dropdown and the list rail show
	// them. Profile is the card-page identity parsed from the résumé note.
	//
	// Sections 是顶层目录，顺序即顶栏下拉与列表页右栏的顺序。
	// Profile 是从简历笔记解析出的名片身份。
	Sections []*BlogSectionDTO `json:"sections,omitempty"`
	Profile  *BlogProfileDTO   `json:"profile,omitempty"`

	// Pills is Profile already arranged into the pill row, in the order the card renders it. It is
	// assembled here rather than in the router because the order is a presentation rule that must
	// not be re-derived by every caller — and because the RSS pill needs the same base URL the
	// home assembly already receives.
	//
	// Pills 是已经把 Profile 排成药丸行的结果，顺序即名片渲染的顺序。它在这里组装而不是在路由层，
	// 因为顺序是一条不该被每个调用方重新推导的呈现规则——也因为 RSS 那枚药丸需要首页组装
	// 本来就收到的同一个 base URL。
	Pills []BlogProfileLinkDTO `json:"pills,omitempty"`

	// Recs are the second-level folders with recent activity; see blogSectionRecs.
	// Recs 是最近有变动的二级文件夹；见 blogSectionRecs。
	Recs []*BlogSectionRecDTO `json:"recs,omitempty"`
}

// BlogPostDTO a rendered blog post
// BlogPostDTO 渲染后的博客文章
type BlogPostDTO struct {
	Slug      string `json:"slug"`      // Short link slug // 短链 slug
	Title     string `json:"title"`     // Post title // 标题
	HTML      string `json:"html"`      // Rendered HTML body // 渲染后的 HTML 正文
	Path      string `json:"path"`      // Source note path // 来源笔记路径
	CreatedAt string `json:"createdAt"` // Creation date // 创建日期

	// Cover is the article hero image, or nil.
	//
	// Only an absolute http(s) reference is filled in here. A cover that points at a local
	// attachment needs the file endpoint resolved against a vault id, which the article render
	// path does not carry; those notes fall back to a title without a hero rather than to a
	// broken image. The list cards solve the same problem differently — they resolve attachments
	// because they already have the vault, and they draw a placeholder when there is none.
	//
	// Cover 是文章页的 hero 图，可能为 nil。
	//
	// 这里只填绝对的 http(s) 引用。指向本地附件的封面需要按 vault id 解析文件端点，
	// 而文章渲染路径不携带它；那些笔记退化成没有 hero 的标题，而不是一张破图。
	// 列表卡片用另一种办法解决同一个问题——它们本来就带着 vault，因此能解析附件，
	// 而没有封面时画占位块。
	Cover *BlogCoverDTO `json:"cover,omitempty"`
}

// BlogFriendDTO is one entry of a friends page, read from the page note's frontmatter.
//
// Name and URL are the only required fields; the rest are optional and the template omits what is
// absent rather than rendering an empty element. Group is the heading the entry is listed under;
// entries with no group render first, without a heading.
//
// BlogFriendDTO 是友链页的一条，来自页面笔记的 frontmatter。
//
// 只有 Name 与 URL 必填，其余可选，模板对缺失的字段整块省略而不是渲染空元素。
// Group 是这条所属的分组标题；没有分组的条目排在最前，且不带标题头。
type BlogFriendDTO struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Avatar string `json:"avatar,omitempty"`
	Desc   string `json:"desc,omitempty"`
	Group  string `json:"group,omitempty"`
	// Email and Feed are carried but not rendered. They exist so a future blogroll can contact
	// the owner or poll the feed without re-reading the note; the friends page itself shows cards,
	// not contact details. Feed has already passed the http(s) check when it arrives here.
	//
	// Email 与 Feed 只存不显示。它们为日后的友链聚合保留「怎么联系、怎么订阅」两项数据，
	// 使聚合不必重读笔记；友链页本身只画卡片，不展示联系方式。Feed 到达这里时已通过 http(s) 校验。
	Email string `json:"email,omitempty"`
	Feed  string `json:"feed,omitempty"`
}

// BlogSiteInfoDTO is the friends note's own `site:` block: the information a visitor copies when
// applying for a mutual link.
//
// It is deliberately separate from BlogFriendDTO: an entry describes someone else's site, while
// this block describes this site. Title, URL and feed are not stored here because the server
// already knows them (config title and request base URL), and duplicating them in the note would
// let the copy card disagree with the page it sits on.
//
// BlogSiteInfoDTO 是友链笔记自己的 `site:` 块：访客申请互换友链时要复制走的本站信息。
//
// 它与 BlogFriendDTO 刻意分开：条目描述别人的站点，这个块描述本站。站名、地址与订阅源不存
// 在这里，因为服务端本来就知道（配置标题与请求基地址），在笔记里再写一遍只会让复制卡
// 与它所在的页面说法不一致。
type BlogSiteInfoDTO struct {
	Desc   string `json:"desc,omitempty"`
	Email  string `json:"email,omitempty"`
	Avatar string `json:"avatar,omitempty"`
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
	// Friends is the card list of a friends page, already validated and in note order. Empty for
	// every other alias, and empty when the frontmatter has no valid entries.
	// Friends 是友链页的卡片列表，已通过校验并保持笔记里的顺序。其它别名恒为空；
	// frontmatter 里没有合法条目时也为空。
	Friends []BlogFriendDTO `json:"friends,omitempty"`
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

	// Summary is the plain-text excerpt the recommendation list shows under the title. The block
	// is a recommendation, not a leaderboard: it deliberately hides Like, and a title alone reads
	// as a link list rather than something worth opening.
	//
	// Summary 是推荐列表显示的纯文本摘要。这个区块是推荐而不是排行榜：
	// 它有意隐藏 Like，而只有标题会读成一份链接清单，不像值得点开的东西。
	Summary string `json:"summary"`
}

// BlogSectionDTO is one top-level folder rendered as a section (版块).
//
// The reference site calls these categories. Here they come straight from the folder tree, so a
// new top-level folder becomes a section without any configuration — and a folder renamed by
// accident becomes a section too, which is why Name is normalized before it reaches the template.
//
// BlogSectionDTO 是一个顶层目录，作为「版块」渲染。
//
// 参照站把它们叫分类。这里的版块直接来自目录树，因此新建一个顶层目录就多一个版块，无需配置；
// 手滑改名的目录也会变成版块，所以 Name 在进入模板前会先归一化。
type BlogSectionDTO struct {
	Name  string `json:"name"`  // display name // 显示名
	Slug  string `json:"slug"`  // URL segment, the raw folder name // URL 段，原目录名
	Count int    `json:"count"` // published notes inside // 其下已发布笔记数
}

// BlogProfileLinkDTO is one pill under the name on the card page.
//
// Kind exists because the pills are not all links: the WeChat entry opens a QR overlay, and the
// RSS entry opens an explanation card. Href is what a link needs; a pill whose Kind needs none
// leaves it empty, and the template must not emit a dead href for it.
//
// BlogProfileLinkDTO 是名片页姓名下方的一枚药丸。
//
// Kind 的存在是因为药丸并不都是链接：微信那一枚打开二维码浮层，RSS 那一枚打开说明卡。
// Href 是链接需要的；Kind 不需要 Href 的药丸留空，模板不能为它输出一个死链。
type BlogProfileLinkDTO struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Icon  string `json:"icon"` // octicon name the template maps to a path // 模板映射到 path 的图标名
	Kind  string `json:"kind"` // link | mail | rss | qr // 行为类型
}

// BlogSectionRecDTO is one folder presented as a recommendation or as a sub-section card.
//
// Href is the folder's own page, not the newest article inside it. That changed when the taxonomy
// gained a middle level: a first-level folder now has a page listing its second-level folders, and
// a second-level folder has a page listing its notes, so "recently updated" can open the thing that
// was updated rather than a single note that happened to be newest.
//
// Slug is kept because it still names the newest article, which the template no longer links to but
// which stays useful when a folder page would be a dead end (see blogAggregateSections).
//
// BlogSectionRecDTO 是一个作为推荐或二级栏目卡呈现的文件夹。
//
// Href 是这个文件夹自己的页面，而不是它里面最新的那篇文章。分类多了中间一层之后这一点变了：
// 一级文件夹现在有一页列出它的二级文件夹，二级文件夹有一页列出它的笔记，
// 于是「最近更新」可以打开那个被更新的东西，而不是恰好最新的某一篇。
//
// Slug 保留着，它仍然指着最新那篇；模板不再用它当链接，但在栏目页会变成死路时还有用
// （见 blogAggregateSections）。
type BlogSectionRecDTO struct {
	Name    string `json:"name"`   // 目录名 // folder name
	Parent  string `json:"parent"` // 一级目录名，一级栏目自身为空 // parent section, empty for a top-level one
	Desc    string `json:"desc"`   // 手写的一句话 // hand-written one-liner
	Slug    string `json:"slug"`   // 该栏目最新一篇的 slug // newest article in the folder
	Href    string `json:"href"`   // 该栏目自己的页面 // the folder's own page
	Count   int    `json:"count"`  // 已发布篇数 // published notes inside
	Words   int    `json:"words"`  // 该栏目总字数 // total characters inside
	Updated string `json:"updated"` // 该栏目最新变更日期 // newest change date

	// Cover is picked at random from the covers of the notes inside the folder, one per request.
	// The owner asked for this explicitly: a folder has no cover of its own, and drawing a
	// placeholder for it would throw away cover art the author already chose.
	//
	// Cover 从该目录内笔记的封面里随机取一张，每次请求重抽。
	// 这是站主明确要求的：目录本身没有封面，而为它画一个占位等于把作者已经选好的封面图丢掉。
	Cover *BlogCoverDTO `json:"cover,omitempty"`

	// Initial and InitialHue drive the placeholder cover when no note inside has one.
	// Initial / InitialHue 在目录内没有任何封面时驱动占位封面。
	Initial    string `json:"initial,omitempty"`
	InitialHue int    `json:"initialHue,omitempty"`
}

// BlogProfileMetaDTO is one key/value row of the card's "关于" block.
//
// The résumé note already carries these as `- 所在地 **浙江 · 金华**`. Rendering them as rows
// instead of a paragraph is the reference site's About/Education shape: a label column of fixed
// width and a value column that aligns down the page.
//
// BlogProfileMetaDTO 是名片「关于」区块里的一行键值。
//
// 简历笔记里本来就有这些，写成 `- 所在地 **浙江 · 金华**`。把它们渲染成行而不是一段文字，
// 正是参照站 About/Education 的形状：固定宽度的标签列 + 沿页面对齐的值列。
type BlogProfileMetaDTO struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// BlogProfileDTO is the identity block the card page shows.
//
// Every field is optional. The block is parsed out of the résumé page note, and a note that has
// been rewritten is allowed to lose a field: the template omits what is empty rather than showing
// a placeholder, because a card page with "No name" on it is worse than one with no name line.
//
// BlogProfileDTO 是名片页展示的身份块。
//
// 每个字段都是可选的。这块数据从简历页笔记解析而来，一篇被改写过的笔记允许丢掉某个字段：
// 模板会省略空值而不是显示占位文案 —— 一张写着「未命名」的名片页，比一张没有姓名行的更糟。
type BlogProfileDTO struct {
	Name     string               `json:"name"`
	Tagline  string               `json:"tagline"`
	Location string               `json:"location"`
	Avatar   string               `json:"avatar"`
	Links    []BlogProfileLinkDTO `json:"links"`
	QRImage  string               `json:"qrImage"`
	// Meta keeps the résumé's short facts in their authored order. It is not sorted: the author
	// wrote them in the order they want them read, and a "natural" sort would put 年龄 before 性别
	// for no reason a reader could name.
	//
	// Meta 按作者书写顺序保留简历里的短事实。它不排序：作者是按想被阅读的顺序写的，
	// 而一次「自然」排序会把年龄排到性别前面，读者说不出这有什么道理。
	Meta []BlogProfileMetaDTO `json:"meta"`
}

// BlogPageLinkDTO is one page-number button in the pager.
// BlogPageLinkDTO 分页器里的一个页码按钮。
type BlogPageLinkDTO struct {
	Num     int    `json:"num"`
	URL     string `json:"url"`
	Current bool   `json:"current"`
}

// BlogListDTO is everything the /blog/all, section and archives pages need.
//
// Entries is already sliced to the current page; Total counts the whole set. Keeping both means
// the template never has to know about paging arithmetic, and the empty state is just Total == 0.
//
// BlogListDTO 是 /blog/all、版块页与归档页所需的全部数据。
//
// Entries 已经切到当前页，Total 是全集大小。两者都给，模板就不必懂分页算术，
// 空状态也就只是 Total == 0。
type BlogListDTO struct {
	Title      string
	Kicker     string // 全部笔记 / 版块名 / 二级栏目名 // section or sub-section name
	Section    string // current section slug, empty on /blog/all // 当前版块，全部页为空
	Sub        string // current sub-section name, empty above it // 当前二级栏目，其上层为空
	Entries    []*BlogEntryDTO
	Total      int
	Sections   []*BlogSectionDTO
	Hot        []*BlogHotItemDTO
	Page       int
	TotalPages int
	Pages      []BlogPageLinkDTO
	PrevURL    string
	NextURL    string
	Canonical  string

	// Subs are the child folders a section page shows instead of a note list. It is nil everywhere
	// else — and nil rather than empty on a section whose notes all sit directly inside it, which
	// is what tells the template to fall back to the note list.
	//
	// Subs 是版块页用来代替笔记清单的子目录卡片。其它页面为空——
	// 而一个「笔记全都直接放在版块里」的版块得到 nil 而不是空切片，
	// 模板据此回退到笔记清单。
	Subs []*BlogSectionRecDTO

	// ArchiveYears groups entries by year for /blog/archives. It is nil elsewhere.
	//
	// ArchiveYears 为 /blog/archives 按年分组；其它页面为空。
	ArchiveYears []*BlogArchiveYearDTO

	// AllEntries is the full filtered set, carried only so the router can rank recommendations
	// against everything rather than against the current page. It never reaches a client.
	//
	// AllEntries 是过滤后的全集，只为让路由层依据全部条目而不是当前页来排推荐。它从不抵达客户端。
	AllEntries []*BlogEntryDTO `json:"-"`

	// Recs are the same second-level folder recommendations the card page shows.
	// Recs 与名片页相同的那组二级栏目推荐。
	Recs []*BlogSectionRecDTO `json:"recs,omitempty"`
}

// BlogArchiveYearDTO is one year block on the archives page.
// BlogArchiveYearDTO 归档页里的一个年份块。
type BlogArchiveYearDTO struct {
	Year    string
	Count   int
	Entries []*BlogEntryDTO
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

	// Feed settings for /blog/rss.xml and the per-section feeds. Same story as the graph
	// block: config.yaml-only values are invisible to the settings page, and the feed length
	// and window are exactly what a friendly-link partner experiences on first subscribe.
	//
	// 订阅源设置，作用于 /blog/rss.xml 与各栏目源。与图谱区块同样的理由：只存在于
	// config.yaml 的值对设置页不可见，而条数与时间窗恰恰是友链伙伴首次订阅时体验到的东西。
	FeedMode       string `json:"feedMode"`       // recent | all // 按时间窗 / 全部
	FeedRecentDays int    `json:"feedRecentDays"` // recent window in days // 时间窗（天）
	FeedMaxItems   int    `json:"feedMaxItems"`   // Entry cap per feed // 每份源的条数上限
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

	FeedMode       *string `json:"feedMode"`       // recent | all // 按时间窗 / 全部
	FeedRecentDays *int    `json:"feedRecentDays"` // recent window in days // 时间窗（天）
	FeedMaxItems   *int    `json:"feedMaxItems"`   // Entry cap per feed // 每份源的条数上限
}

// BlogPostItem one published post with an absolute URL
// BlogPostItem 一篇已发布文章及其完整地址
type BlogPostItem struct {
	Title     string `json:"title"`     // Post title // 标题
	URL       string `json:"url"`       // Absolute post URL // 文章完整地址
	Path      string `json:"path"`      // Source note path // 来源笔记路径
	CreatedAt string `json:"createdAt"` // Creation date // 创建日期
}
