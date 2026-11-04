package routers

import (
	"html/template"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// The three content pages (card / list / archives) share one chrome.
//
// Everything below chrome is what differs: the card page owns identity and heat, the list page
// owns cards and a pager, the archive page owns year blocks. Sharing the chrome rather than
// copying it means the <head> metadata, the nav dropdown and the footer are defined once — and
// that matters here because the previous design had four full page templates each carrying its
// own copy of the same twelve head tags.
//
// 三个内容页（名片 / 列表 / 归档）共用一套外壳。
//
// 外壳之下的东西才是各自的：名片页拥有身份与热力，列表页拥有卡片与分页器，归档页拥有年份块。
// 共用外壳而不是复制它，意味着 <head> 元信息、导航下拉与页脚只定义一次——
// 这在本次改造里尤其重要：上一版四个页面模板各自携带同一套十二行 head 标签。
type blogChrome struct {
	Title       string
	SiteName    string
	ICP         string
	MPS         string
	MPSCode     string
	Indexable   bool
	Canonical   string
	Description string
	FeedURL     string
	JSONLD      template.JS
	CSS         template.CSS

	// Pages are the page-note entries ("关于" / "资料") rather than the pager.
	// Pages 是页面笔记入口（「关于」「资料」），不是分页器。
	Pages    []dto.BlogNavPageDTO
	HasShelf bool

	// Sections feed the nav dropdown; Nav feeds the mobile drawer's folder tree.
	// Sections 供顶栏下拉；Nav 供移动端抽屉的目录树。
	Sections []*dto.BlogSectionDTO
	Nav      []dto.BlogNavNode

	// TintJS paints each card's hover colour from its own cover. WarpJS is the page transition.
	// WaveJS is the dot-grid wave background, carried only by the card page and page notes.
	//
	// TintJS 从每张卡片自己的封面取色，用来画悬停色；WarpJS 是页面过渡。
	// WaveJS 是点阵波纹背景，只有名片页与页面笔记带。
	TintJS template.JS
	WarpJS template.JS
	WaveJS template.JS
}

// blogCardView is the card page: identity, heat, and nothing else.
//
// There is no article list here on purpose. The reference site's home page is a personal card,
// not a feed, and the list lives at /blog/all — which is what the nav dropdown's first entry
// points at.
//
// blogCardView 是名片页：身份、热力，其它都没有。
//
// 这里有意不放文章列表。参照站的首页是一张个人名片而不是信息流，列表在 /blog/all——
// 也就是顶栏下拉第一项指向的地方。
type blogCardView struct {
	blogChrome
	Stats     *dto.BlogStatsDTO
	Profile   *dto.BlogProfileDTO
	Pills     []dto.BlogProfileLinkDTO
	Activity  *dto.BlogActivityDTO
	// Recs feed the recommendation sheet the capsule opens and the rail on list pages.
	//
	// Recs 供胶囊打开的推荐面板与列表页右栏使用。
	Recs       []*dto.BlogSectionRecDTO
	StatsJS    template.JS
	DockJS     template.JS
	AskEnabled bool
}

// blogListView is /blog/all, /blog/c/<section> and /blog/c/<section>/<sub>.
//
// Kicker carries the h1: "全部笔记", the section name, or the sub-section name. Section and Sub
// stay empty on the levels above so the template can tell where it is without a second boolean.
//
// Subs is what makes a section page a table of contents: when it is non-nil the body renders child
// folder cards instead of note cards, and Entries is nil. That is one condition in the template
// rather than two page shapes, because the two bodies share a wrapper, a kicker and a card list.
//
// blogListView 是 /blog/all、/blog/c/<版块> 与 /blog/c/<版块>/<二级>。
//
// Kicker 承载 h1：「全部笔记」、版块名或二级栏目名。在更上层 Section 与 Sub 保持为空，
// 使模板无需第二个布尔量就能判断自己在哪一层。
//
// Subs 是版块页成为目录的原因：它非空时主体渲染子目录卡片而不是笔记卡片，此时 Entries 为 nil。
// 模板里这是一个条件而不是两种页形，因为两个主体共用同一个外层、同一个 kicker 和同一种卡片列表。
type blogListView struct {
	blogChrome
	Kicker     string
	Section    string
	Sub        string
	Entries    []*dto.BlogEntryDTO
	Subs       []*dto.BlogSectionRecDTO
	Total      int
	Page       int
	TotalPages int
	PagerLinks []dto.BlogPageLinkDTO
	PrevURL    string
	NextURL    string
	// Recs are the folders one level below this page, or nothing on a sub-section page.
	// Recs 是这一页下一层的文件夹；二级栏目页上没有。
	Recs       []*dto.BlogSectionRecDTO
	DockJS     template.JS
	AskEnabled bool
}

// blogArchiveView is /blog/archives: one block per year, watermarked.
// blogArchiveView 是 /blog/archives：每年一块，带水印年份。
type blogArchiveView struct {
	blogChrome
	Years  []*dto.BlogArchiveYearDTO
	Total  int
	DockJS template.JS
	// AskEnabled is read by the shared navdock partial; a view without it fails at execute time
	// rather than at compile time, which is exactly how /blog/archives turned into a 500.
	//
	// AskEnabled 由共用的 navdock 片段读取；缺少这个字段的 view 会在执行时失败而不是编译时，
	// /blog/archives 变 500 就是这么来的。
	AskEnabled bool
}

// blogNavPageURL builds the address of a page-note entry.
// blogNavPageURL 构造页面笔记入口的地址。
func blogNavPageURL(alias string) string {
	if alias == "" {
		return "/blog"
	}
	return "/blog/" + alias
}
