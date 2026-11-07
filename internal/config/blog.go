package config

// BlogConfig built-in blog rendering configuration
// BlogConfig 内置博客渲染配置
type BlogConfig struct {
	// Enabled turns on the built-in blog endpoints (/blog and /blog/<slug>).
	// Enabled 是否开启内置博客端点（/blog 与 /blog/<slug>）。
	Enabled *bool `yaml:"enabled" default:"false"`
	// Title is shown in the blog header and page titles.
	// Title 显示在博客页头与页面标题中。
	Title string `yaml:"title" default:"Notes"`
	// UID whose active shares are published by the index page.
	// 0 means the first user (uid = 1).
	// UID 指定索引页发布哪个用户的分享；0 表示首个用户（uid = 1）。
	UID int64 `yaml:"uid" default:"0"`
	// Indexable allows search engines to index blog pages (it drops the noindex directive).
	// Indexable 是否允许搜索引擎收录博客页面（为真时不再输出 noindex）。
	Indexable *bool `yaml:"indexable" default:"true"`
	// ShowIndex lists share entries on the blog index page.
	// ShowIndex 是否在博客首页列出分享条目。
	ShowIndex *bool `yaml:"show-index" default:"true"`
	// GraphScope decides which notes the knowledge graph may draw:
	//   "shared" (default) — only notes that are themselves published
	//   "all"              — every note; unpublished ones appear as anonymous dots without
	//                        titles, so the graph shows connectivity without leaking names
	// GraphScope 决定知识图谱可以绘制哪些笔记：
	//   "shared"（默认）—— 只画本身已发布的笔记
	//   "all"           —— 全部笔记；未发布的以匿名小点呈现、不显示标题，
	//                      既能体现连接关系又不泄露名称
	GraphScope string `yaml:"graph-scope" default:"shared"`
	// MaxGraphNodes bounds the graph so the server-side layout stays fast.
	// MaxGraphNodes 限制图谱节点数，保证服务端布局计算足够快。
	MaxGraphNodes int `yaml:"max-graph-nodes" default:"120"`
	// ShowGraph toggles the knowledge graph on the home page. Defaults to false: the graph
	// proved more trouble than value, so it is off unless explicitly enabled.
	// ShowGraph 控制首页是否展示知识图谱。默认关闭：实测收益不抵折腾，需要时再显式打开。
	ShowGraph *bool `yaml:"show-graph" default:"false"`
	// LocalGraphDepth is the starting depth of the in-article graph (1–3). The depth slider
	// raises it per view; this value decides what a reader sees before touching anything.
	// LocalGraphDepth 是文章内图谱的起始深度（1–3）。深度滑块按次查看上调它；
	// 本值决定读者在未做任何操作时看到的内容。
	LocalGraphDepth int `yaml:"local-graph-depth" default:"1"`

	// ParticleCount is how many particles one like/dislike burst spawns (30–400).
	//
	// It is a setting rather than a constant because the right number depends on the machine and
	// on how much the effect is wanted: on a weak laptop 400 particles is a stutter, and someone
	// who finds the burst distracting wants a number near the floor rather than an off switch.
	//
	// ParticleCount 是一次点赞/点踩喷发产生的粒子数（30–400）。
	//
	// 它是配置项而不是常量，因为合适的数值取决于机器与对效果的期望：在弱的笔记本上 400 颗粒子会卡，
	// 而觉得喷发分心的人想要的是一个接近下限的数字，而不是一个开关。
	ParticleCount int `yaml:"particle-count" default:"150"`

	// DislikeBurstEnabled turns the dislike particle burst on or off. Default on, as asked; a
	// pointer rather than a bool so "unset" is distinguishable from "explicitly off" and the
	// default survives a hand-edited config that omits the key.
	//
	// DislikeBurstEnabled 开关点踩的粒子喷发。按要求默认开启；
	// 用指针而不是布尔值，使「未设置」与「显式关闭」可区分，默认值也能在手工编辑删掉该键时存活。
	DislikeBurstEnabled *bool `yaml:"dislike-burst" default:"true"`

	// FeedMode decides which articles enter /blog/rss.xml and the per-section feeds:
	//   "recent" (default) — only articles created within FeedRecentDays
	//   "all"              — every published article, newest first
	// The window matters for friendly-link syndication: a new subscriber should see a small
	// burst, not the whole archive. With FeedMaxItems this is the standard stateless way to
	// keep a first fetch light; RSS itself has no notion of "who is new".
	//
	// FeedMode 决定哪些文章进入 /blog/rss.xml 与各栏目订阅源：
	//   "recent"（默认）—— 只收最近 FeedRecentDays 天内创建的文章
	//   "all"           —— 全部已发布文章，按新到旧
	// 时间窗对友链场景很关键：新订阅者看到的应该是一小批，而不是整个归档。
	// 它与 FeedMaxItems 一起构成「首次拉取保持轻量」的标准无状态做法——RSS 协议本身
	// 没有谁是新订阅者的概念。
	FeedMode string `yaml:"feed-mode" default:"recent"`

	// FeedRecentDays is the "recent" window in days (clamped to 1..365 on read/update).
	// FeedRecentDays 是 recent 模式的时间窗（天），读取与更新时夹取在 1..365。
	FeedRecentDays int `yaml:"feed-recent-days" default:"30"`

	// FeedMaxItems caps how many entries any feed carries (clamped to 1..50).
	// Five (the default) keeps a first subscribe to ~5 items; conditional GET (ETag/304) keeps
	// the unchanged polls between updates at zero bytes.
	//
	// FeedMaxItems 限制每份订阅源的条数（读取与更新时夹取在 1..50）。
	// 默认 5 让首次订阅最多看到 5 条；条件请求（ETag/304）让两次更新之间的轮询零流量。
	FeedMaxItems int `yaml:"feed-max-items" default:"5"`

	// ICP is the site's ICP filing number, rendered in the footer as a link to beian.miit.gov.cn.
	//
	// Empty means "render nothing" — which is the right default for an open-source project:
	// a filing number belongs to one operator's domain, so shipping a placeholder (or anyone
	// else's real number) in the repository would be wrong. Whoever deploys this fills in their
	// own, and the footer line only exists for them.
	//
	// ICP 是本站的 ICP 备案号，渲染在页脚并链接到 beian.miit.gov.cn。
	//
	// 留空表示不渲染 —— 这对开源项目才是正确的默认值：备案号属于某个运营者的具体域名，
	// 仓库里放占位符（或别人的真号）都是错的。部署者填自己的，那一行页脚才存在。
	ICP string `yaml:"icp"`

	// MPS is the site's public-security filing number, rendered in the footer as a link to the
	// official query page. Same reasoning as ICP: empty means no line.
	//
	// When non-empty, MPSCode is appended to the query URL so the link lands on this site's
	// record rather than the generic search form. If MPSCode is empty the plain query page is
	// used, so a deployment that only has the number still gets a working link.
	//
	// MPS 是本站的公安联网备案号，渲染在页脚并链接到官方查询页。留空则不渲染，理由同 ICP。
	//
	// 非空时会把 MPSCode 拼进查询地址，使链接直接定位到本站记录而不是通用搜索页；
	// MPSCode 为空则用普通查询页，只有号的部署也仍然拿到可用链接。
	MPS string `yaml:"mps"`

	// MPSCode is the numeric code used by the official query page (the digits in the MPS number).
	// MPSCode 是官方查询页使用的数字编码（即 MPS 号里的那串数字）。
	MPSCode string `yaml:"mps-code"`
}

// DislikeBurst reports whether the dislike burst is enabled, defaulting to on.
//
// 未配置时为真 —— 用户要求的默认值是开启，因此缺失这个键必须读作「开」而不是「关」。
func (b BlogConfig) DislikeBurst() bool {
	if b.DislikeBurstEnabled == nil {
		return true
	}
	return *b.DislikeBurstEnabled
}

// Particles returns the particle count clamped to the range the settings UI exposes.
//
// A hand-edited config can hold anything; a value of 0 would make the burst invisible and
// 100000 would hang a weak machine, so the range is enforced here rather than trusted.
//
// Particles 返回夹到设置界面所暴露区间内的粒子数。
//
// 手工编辑的配置可以是任何值；0 会让喷发看不见，100000 会卡死弱机器，
// 因此这个区间在这里强制，而不是被信任。
func (b BlogConfig) Particles() int {
	n := b.ParticleCount
	if n < blogMinParticles {
		n = blogDefaultParticles
	}
	if n > blogMaxParticles {
		n = blogMaxParticles
	}
	return n
}

const (
	blogMinParticles     = 30
	blogMaxParticles     = 400
	blogDefaultParticles = 150
)
