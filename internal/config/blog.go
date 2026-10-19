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
