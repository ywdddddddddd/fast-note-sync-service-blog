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
	// TintJS 从每张卡片自己的封面取色，用来画悬停色；WarpJS 是页面过渡。
	TintJS template.JS
	WarpJS template.JS
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
	// Hot feeds the recommendation sheet the capsule opens. The card page itself shows no
	// recommendation block — that was an explicit decision — but the capsule's second key still
	// needs something to open, and a key that opens an empty sheet is the same apology as an
	// empty block.
	//
	// Hot 供胶囊打开的推荐面板使用。名片页本身不展示推荐区块——那是明确的决定——
	// 但胶囊的第二个键仍需要有东西可开，而一个打开空面板的键，与一个空区块是同一种道歉。
	Hot        []*dto.BlogHotItemDTO
	StatsJS    template.JS
	DockJS     template.JS
	AskEnabled bool
}

// blogListView is /blog/all and /blog/c/<section>.
//
// Kicker carries the h1: "全部笔记" or the section name. Section stays empty on /blog/all so the
// template can mark "no section" without a second boolean.
//
// blogListView 是 /blog/all 与 /blog/c/<版块>。
//
// Kicker 承载 h1：「全部笔记」或版块名。在 /blog/all 上 Section 保持为空，
// 使模板无需第二个布尔量就能判断「当前不分版块」。
type blogListView struct {
	blogChrome
	Kicker     string
	Section    string
	Entries    []*dto.BlogEntryDTO
	Total      int
	Page       int
	TotalPages int
	PagerLinks []dto.BlogPageLinkDTO
	PrevURL    string
	NextURL    string
	Hot        []*dto.BlogHotItemDTO
	AskEnabled bool
}

// blogArchiveView is /blog/archives: one block per year, watermarked.
// blogArchiveView 是 /blog/archives：每年一块，带水印年份。
type blogArchiveView struct {
	blogChrome
	Years []*dto.BlogArchiveYearDTO
	Total int
}

// blogNavPageURL builds the address of a page-note entry.
// blogNavPageURL 构造页面笔记入口的地址。
func blogNavPageURL(alias string) string {
	if alias == "" {
		return "/blog"
	}
	return "/blog/" + alias
}
