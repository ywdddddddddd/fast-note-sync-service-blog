package service

import (
	"regexp"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// The card page's identity comes out of the résumé page note, not out of configuration.
//
// That choice has one real cost and one real benefit. The cost: the parser depends on the note's
// shape, so rewriting the résumé can empty the card. The benefit: there is exactly one place to
// edit your own name, and it is the page a reader already reaches by clicking "关于". A second
// copy in YAML would drift the first time one of them is edited.
//
// Everything below therefore fails soft. A missing block yields an empty field, the template
// omits it, and the card degrades to whatever is left rather than erroring the whole page.
//
// 名片页的身份信息来自简历页笔记，而不是配置。
//
// 这个选择有一个真实代价和一个真实收益。代价：解析器依赖笔记的形状，重写简历会把名片清空。
// 收益：改自己的名字只有一个地方，而且那正是读者点「关于」就能到达的页面。
// 在 YAML 里存第二份，会在其中一处被编辑的那天开始漂移。
//
// 因此下面的一切都是软失败。缺一个容器就得到空字段，模板省略它，名片降级为剩下的部分，
// 而不是让整页报错。

var (
	// blogProfileHeadRegex captures the body of a `::: profile-head` block. The closing marker is
	// a bare `:::` line, and the body is allowed to span lines.
	//
	// blogProfileHeadRegex 捕获 `::: profile-head` 块的正文。结束标记是独占一行的裸 `:::`，
	// 正文允许跨行。
	blogProfileHeadRegex = regexp.MustCompile(`(?s):::[ \t]*profile-head[ \t]*\r?\n(.*?)\r?\n[ \t]*:::[ \t]*(?:\r?\n|$)`)
	blogProfileMetaRegex = regexp.MustCompile(`(?s):::[ \t]*profile-meta[ \t]*\r?\n(.*?)\r?\n[ \t]*:::[ \t]*(?:\r?\n|$)`)
	blogProfileLinkRegex = regexp.MustCompile(`(?s):::[ \t]*links[ \t]*\r?\n(.*?)\r?\n[ \t]*:::[ \t]*(?:\r?\n|$)`)

	// A heading is the name; the first italic run is the one-line description.
	// 标题是名字；第一段斜体是那一行自我描述。
	blogProfileNameRegex = regexp.MustCompile(`(?m)^#{1,3}[ \t]+(.+?)[ \t]*$`)
	blogProfileTagRegex  = regexp.MustCompile(`(?s)\*(.+?)\*`)

	// `- 所在地 **浙江 · 金华**` — the label is free text, the value is bold.
	// `- 所在地 **浙江 · 金华**` —— 标签是自由文本，值是加粗的。
	blogProfileMetaLine = regexp.MustCompile(`(?m)^[-*][ \t]*([^*\n]+?)[ \t]*\*\*(.+?)\*\*[ \t]*$`)

	// `- GitHub **https://github.com/x**` / `- 邮箱 **a@b.com**`
	// The value may be bare (mail) or a URL; the leading `-` is what separates a contact line
	// from the free text around it.
	//
	// `- GitHub **https://github.com/x**` / `- 邮箱 **a@b.com**`
	// 值可以是裸地址（邮箱）或 URL；行首的 `-` 把联系人行与周围的自由文本区分开。
	blogProfileLinkLine = regexp.MustCompile(`(?m)^[-*][ \t]*(.+?)[ \t]*\*\*[ \t]*((?:https?://|mailto:)[^\s*]+|[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,})[ \t]*\*\*[ \t]*$`)

	// A wiki image embed, e.g. `![[公众号二维码.jpg|162]]`. Only the target matters here.
	// wiki 图片嵌入，例如 `![[公众号二维码.jpg|162]]`。这里只关心目标名。
	blogProfileImageRef = regexp.MustCompile(`!\[\[([^\]|]+)(?:\|[^\]]*)?\]\]`)
)

// profileLocationKeys are the meta labels treated as "where you are".
//
// Three spellings because the same fact gets written three ways in practice, and a card that
// shows a location only when the author happened to type 所在地 would be a coin flip.
//
// profileLocationKeys 是被当作「你在哪」的 meta 标签。
//
// 三种写法是因为同一个事实在实践中会被写成三种；只在作者恰好写了「所在地」时才显示地区的名片，
// 等于抛硬币。
var profileLocationKeys = []string{"所在地", "地区", "城市", "位置", "现居"}

// parseBlogProfile extracts the card-page identity from a page note's markdown.
//
// It takes the raw note content (front matter included) rather than rendered HTML: the markers
// are still there, the emphasis is still `**`, and nothing has to be un-escaped. The page-note
// container pipeline rewrites `::: name` into comment markers before Markdown runs, so parsing
// the source is also the shape that is guaranteed to exist before any rendering decision.
//
// parseBlogProfile 从页面笔记的 Markdown 里提取名片身份。
//
// 它接收笔记原文（含 front matter）而不是渲染后的 HTML：标记还在、强调还是 `**`、也不需要
// 反转义。页面笔记容器管线会在 Markdown 之前把 `::: name` 改写成注释标记，所以解析源码
// 也是「任何渲染决定之前一定存在」的那个形状。
func parseBlogProfile(content string) *dto.BlogProfileDTO {
	p := &dto.BlogProfileDTO{}

	if m := blogProfileHeadRegex.FindStringSubmatch(content); m != nil {
		body := m[1]
		if h := blogProfileNameRegex.FindStringSubmatch(body); h != nil {
			p.Name = stripProfileMarkup(h[1])
		}
		// The description is the first italic run that is not the name line.
		//
		// Taking "the first italic run" rather than "the first paragraph" is what makes the
		// existing résumé parse correctly: its description is a two-line italic block, and a
		// paragraph-based rule would return only the first line.
		//
		// 描述取第一段斜体，而不是第一段正文。这条规则正是现有简历能正确解析的原因：
		// 它的描述是一段跨两行的斜体，按段落切只会拿到第一行。
		if t := blogProfileTagRegex.FindStringSubmatch(body); t != nil {
			p.Tagline = collapseSpaces(stripProfileMarkup(t[1]))
		}
	}

	if m := blogProfileMetaRegex.FindStringSubmatch(content); m != nil {
		for _, line := range blogProfileMetaLine.FindAllStringSubmatch(m[1], -1) {
			label := strings.TrimSpace(line[1])
			value := stripProfileMarkup(strings.TrimSpace(line[2]))
			p.Meta = append(p.Meta, dto.BlogProfileMetaDTO{Label: label, Value: value})
			for _, key := range profileLocationKeys {
				if strings.Contains(label, key) && p.Location == "" {
					p.Location = value
				}
			}
		}
	}

	if m := blogProfileLinkRegex.FindStringSubmatch(content); m != nil {
		body := m[1]
		for _, line := range blogProfileLinkLine.FindAllStringSubmatch(body, -1) {
			label := stripProfileMarkup(strings.TrimSpace(line[1]))
			href := strings.TrimSpace(line[2])
			kind := blogProfileLinkKind(label, href)
			if kind == "mail" && !strings.HasPrefix(href, "mailto:") {
				href = "mailto:" + href
			}
			p.Links = append(p.Links, dto.BlogProfileLinkDTO{
				Label: label,
				Href:  href,
				Icon:  blogProfileLinkIcon(label, href),
				Kind:  kind,
			})
		}
		// The QR image is recorded but not resolved to a URL here: turning a wiki embed into the
		// attachment's served path needs a file lookup, and a card that silently guesses the URL
		// would show a broken image exactly when it matters. Left empty until the lookup exists,
		// which means the WeChat pill is simply absent rather than broken.
		//
		// 二维码图片记录名字，但不在这里解析成 URL：把 wiki 嵌入变成附件的访问路径需要一次
		// 文件查找，而名片靠猜 URL 会在最要紧的时候显示一张破图。在查找能力到位之前留空，
		// 于是微信药丸是「不出现」，而不是「坏了」。
		if img := blogProfileImageRef.FindStringSubmatch(body); img != nil {
			p.QRImage = ""
			_ = img
		}
	}

	return p
}

// stripProfileMarkup removes the inline emphasis a Markdown author types but a card does not show.
// stripProfileMarkup 去掉 Markdown 作者会写、但名片不显示的强调标记。
func stripProfileMarkup(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "*")
	return strings.TrimSpace(s)
}

// collapseSpaces folds a multi-line description into one line.
// collapseSpaces 把多行描述折成一行。
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// blogProfileLinkKind classifies a contact line so the template knows how to render it.
// blogProfileLinkKind 给联系人行分类，使模板知道该怎么渲染。
func blogProfileLinkKind(label, href string) string {
	lower := strings.ToLower(label) + " " + strings.ToLower(href)
	switch {
	case strings.HasPrefix(href, "mailto:"), strings.Contains(lower, "@"):
		return "mail"
	case strings.Contains(lower, "github"), strings.Contains(lower, "gitee"):
		return "link"
	default:
		return "link"
	}
}

// blogProfileLinkIcon maps a contact label onto the octicon the pill draws.
//
// Matching on the label rather than on the domain is deliberate: people write "GitHub" and link
// to a mirror, or write "博客" and link to github.io. The label is what the author meant, and the
// icon's job is to agree with the words next to it.
//
// blogProfileLinkIcon 把联系人标签映射到药丸绘制的 octicon。
//
// 按标签而不是按域名匹配是有意的：人们会把「GitHub」链到镜像站，或把「博客」链到 github.io。
// 标签才是作者的意思，而图标的职责是与它旁边的文字保持一致。
func blogProfileLinkIcon(label, href string) string {
	lower := strings.ToLower(label)
	switch {
	case strings.Contains(lower, "github"), strings.Contains(lower, "gitee"):
		return "mark-github"
	case strings.Contains(lower, "邮箱"), strings.Contains(lower, "mail"), strings.Contains(lower, "email"):
		return "mail"
	case strings.Contains(lower, "学术"), strings.Contains(lower, "scholar"),
		strings.Contains(lower, "论文"), strings.Contains(lower, "主页"):
		return "mortar-board"
	case strings.Contains(lower, "知乎"), strings.Contains(lower, "博客"), strings.Contains(lower, "blog"):
		return "globe"
	default:
		_ = href
		return "link"
	}
}

// blogProfilePills assembles the pill row in the order the card shows it.
//
// The order is fixed here rather than left to the note's line order, because it is a design
// decision: where you are, then where your code is, then how to reach you, then the feed. A note
// that lists email first should not produce a different-looking card than one that lists GitHub
// first — two résumés edited on different days must render the same card.
//
// blogProfilePills 按名片展示的顺序组装药丸行。
//
// 顺序固定在这里，而不是交给笔记的行顺序，因为这是一条设计决定：
// 你在哪、你的代码在哪、怎么联系你、然后是订阅源。先写邮箱的笔记与先写 GitHub 的笔记，
// 不应该产出两张不一样的名片——两份在不同日子编辑过的简历必须渲染成同一张名片。
// blogLocationLinks pins a place label to the map that should open for it.
//
// Keyed without spaces so that "浙江 · 金华" and "浙江·金华" hit the same entry: the label comes
// from a hand-written résumé line, and the spacing around the dot is whatever the author typed
// that day.
//
// blogLocationLinks 把一个地名钉到应当打开的那张地图。
//
// 键去掉空格，使「浙江 · 金华」与「浙江·金华」命中同一条：标签来自手写的简历行，
// 圆点两侧的空格取决于作者那天怎么打的。
var blogLocationLinks = map[string]string{
	"浙江·金华": "https://cn.bing.com/maps/directions?ty=18&vdpid=7840081814210215937&v=2&sV=1&qpvt=%E6%B5%99%E6%B1%9F%E9%87%91%E5%8D%8E&FORM=MIRE&style=r&rtp=%7Epos.nvcw6dtz3jhw_29.079158782958984%252C119.64734649658203_%25E6%25B5%2599%25E6%25B1%259F%25E7%259C%2581%25E9%2587%2591%25E5%258D%258E%25E5%25B8%2582_&cp=nvgzqptzqtrz&lvl=9.7",
}

// blogLocationLink returns the map URL for a place label, or "" when there is none.
// blogLocationLink 返回地名对应的地图 URL，没有时返回空串。
func blogLocationLink(label string) string {
	key := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\u00a0', '\u3000':
			return -1
		}
		return r
	}, label)
	return blogLocationLinks[key]
}

func blogProfilePills(p *dto.BlogProfileDTO, feedURL string) []dto.BlogProfileLinkDTO {	if p == nil {
		return nil
	}
	out := make([]dto.BlogProfileLinkDTO, 0, len(p.Links)+2)
	if p.Location != "" {
		// The location pill is a link when someone has pinned a map for that place, and plain text
		// otherwise. Hard-coded rather than derived: a map URL that opens on the right spot is a
		// hand-made artifact — a search URL built from the label would land on a results page, not
		// on the place.
		//
		// 地区药丸在有对应地图时是链接，否则是纯文本。写死而不是推导：一个能落在正确位置的地图
		// URL 是手工产物——用标签拼出来的搜索地址只会落到结果页，而不是那个地方。
		if href := blogLocationLink(p.Location); href != "" {
			out = append(out, dto.BlogProfileLinkDTO{Label: p.Location, Href: href, Kind: "link", Icon: "location"})
		} else {
			out = append(out, dto.BlogProfileLinkDTO{Label: p.Location, Kind: "text", Icon: "location"})
		}
	}
	var github, mail, scholar, other []dto.BlogProfileLinkDTO
	for _, l := range p.Links {
		switch {
		case l.Icon == "mark-github":
			github = append(github, l)
		case l.Kind == "mail":
			mail = append(mail, l)
		case l.Icon == "mortar-board":
			scholar = append(scholar, l)
		default:
			other = append(other, l)
		}
	}
	out = append(out, github...)
	out = append(out, mail...)
	out = append(out, scholar...)
	out = append(out, other...)
	if feedURL != "" {
		out = append(out, dto.BlogProfileLinkDTO{Label: "订阅", Href: feedURL, Kind: "rss", Icon: "rss"})
	}
	if p.QRImage != "" {
		out = append(out, dto.BlogProfileLinkDTO{Label: "公众号", Kind: "qr", Icon: "qr"})
	}
	return out
}
