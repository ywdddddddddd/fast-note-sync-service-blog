package service

import (
	"context"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"github.com/haierkeys/fast-note-sync-service/pkg/util"
)

// blogMaxShares bounds how many share records one blog request reads.
//
// The index list and the home page used to use different caps (500 vs 1000), which silently
// degraded wiki links to plain text for notes ranked 501-1000: their targets were missing from
// the link map, so the renderer could not resolve them.
//
// blogMaxShares 限制单次博客请求读取的分享记录数。
// 索引列表与首页此前用了不同的上限（500 与 1000），导致排名 501-1000 的笔记里，
// [[内链]] 被静默降级为纯文本——它们的目标不在链接映射表里，渲染器无法解析。
const blogMaxShares = 1000

// Blog home data assembly: dashboard statistics, tag extraction, folder navigation tree and
// the knowledge graph. All of it is derived from the user's own notes at request time, so the
// page always reflects the current state with no build step.
//
// 博客首页数据组装：仪表盘统计、标签提取、目录导航树与知识图谱。
// 全部数据在请求时从用户自己的笔记实时推导，因此页面永远反映当前状态，无需构建步骤。

var (
	// Fenced code blocks: tags inside them must not be treated as tags.
	// 围栏代码块：其中的 # 不应被当成标签。
	fenceRegex = regexp.MustCompile("(?s)```.*?```")

	// Frontmatter tag lines: `tags: [a, b]` / `tags: a, b`
	// frontmatter 的标签行。
	fmTagLineRegex = regexp.MustCompile(`(?mi)^tags:[ \t]*(.+)$`)

	// Inline tags: #标签, allowing CJK, letters, digits, _ - and / for nested tags.
	// 正文内联标签，允许中文、字母数字、下划线、连字符与斜杠（层级标签）。
	inlineTagRegex = regexp.MustCompile(`(?:^|[\s(（>])#([\p{Han}\p{L}\p{N}_][\p{Han}\p{L}\p{N}_/-]{0,49})`)

	// Frontmatter block at the very beginning of a note.
	// 笔记开头的 frontmatter 块。
	frontmatterRegex = regexp.MustCompile(`(?s)\A\s*---\r?\n.*?\r?\n---\r?\n?`)

	// Markdown noise used when building a plain-text excerpt.
	// 生成纯文本摘录时要清理的 Markdown 噪声。
	mdHeadingRegex = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s*`)
	mdQuoteRegex   = regexp.MustCompile(`(?m)^\s{0,3}>\s?`)
	mdListRegex    = regexp.MustCompile(`(?m)^\s{0,3}(?:[-*+]|\d+\.)\s+`)
	mdImageRegex   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	mdEmbedRegex   = regexp.MustCompile(`!\[\[[^\]]*\]\]`)
	mdLinkRegex    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdWikiRegex    = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)
	mdEmphRegex    = regexp.MustCompile("[*_~`]{1,3}")
	mdHRRegex      = regexp.MustCompile(`(?m)^\s{0,3}(?:---+|\*\*\*+|___+)\s*$`)
)

// blogPlainText converts note content into a single-line plain-text excerpt.
// blogPlainText 把笔记正文转换成单行纯文本摘录。
func blogPlainText(content string, limit int) string {
	s := fenceRegex.ReplaceAllString(content, " ")
	s = frontmatterRegex.ReplaceAllString(s, "")
	s = mdHRRegex.ReplaceAllString(s, " ")
	s = mdEmbedRegex.ReplaceAllString(s, " ")
	s = mdImageRegex.ReplaceAllString(s, " ")
	s = mdWikiRegex.ReplaceAllString(s, "$1$2")
	s = mdLinkRegex.ReplaceAllString(s, "$1")
	s = mdHeadingRegex.ReplaceAllString(s, "")
	s = mdQuoteRegex.ReplaceAllString(s, "")
	s = mdListRegex.ReplaceAllString(s, "")
	s = mdEmphRegex.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	if limit > 0 {
		r := []rune(s)
		if len(r) > limit {
			s = strings.TrimRight(string(r[:limit]), " ，,。.") + "…"
		}
	}
	return s
}

// blogCountChars counts visible characters (CJK-aware) excluding whitespace and markup noise.
// blogCountChars 统计可见字符数（按 Unicode 计），排除空白与标记噪声。
func blogCountChars(content string) int {
	s := fenceRegex.ReplaceAllString(content, " ")
	s = frontmatterRegex.ReplaceAllString(s, "")
	s = mdEmphRegex.ReplaceAllString(s, "")
	return len([]rune(strings.Join(strings.Fields(s), "")))
}

// blogParseTags extracts tags from frontmatter and inline `#tag` occurrences.
// blogParseTags 从 frontmatter 与正文内联 #标签 中提取标签。
func blogParseTags(content string) map[string]int {
	out := make(map[string]int)

	for _, m := range fmTagLineRegex.FindAllStringSubmatch(content, 8) {
		raw := strings.Trim(strings.TrimSpace(m[1]), "[]")
		// Tolerate both `tags: [a, b]` and a single value.
		for _, t := range strings.Split(raw, ",") {
			t = strings.Trim(strings.TrimSpace(t), `"'`)
			if t != "" && !strings.ContainsAny(t, " \t") && blogIsMeaningfulTag(t) {
				out[t]++
			}
		}
	}

	body := fenceRegex.ReplaceAllString(content, " ")
	for _, m := range inlineTagRegex.FindAllStringSubmatch(body, -1) {
		tag := strings.Trim(m[1], "/-")
		if blogIsMeaningfulTag(tag) {
			out[tag]++
		}
	}
	return out
}

// Page-note markers. A note carrying `page: about` is a *page*, not an article: it renders at an
// alias route, and it is excluded from everything whose meaning is "an article" — the home entry
// list, the word count, the activity heat map, the folder tree, the knowledge graph, and the feed.
//
// The marker is read only from inside the frontmatter block. Scanning the whole document would
// treat a `page: about` line quoted inside a note's prose as a declaration.
//
// 页面笔记标记。带 `page: about` 的笔记是一张「页面」而不是一篇文章：
// 它渲染在别名路由上，并被排除在所有「文章」含义之外——首页条目列表、字数统计、
// 活跃度热力图、目录树、知识图谱与订阅源。
//
// 标记只从 frontmatter 块内读取。扫描整篇文档会把正文里引用的 `page: about` 当成声明。
var (
	fmPageRegex   = regexp.MustCompile(`(?mi)^page:[ \t]*([A-Za-z][A-Za-z0-9_-]*)[ \t]*$`)
	fmAvatarRegex = regexp.MustCompile(`(?mi)^avatar:[ \t]*(.+?)[ \t]*$`)
	fmTitleRegex  = regexp.MustCompile(`(?mi)^title:[ \t]*(.+?)[ \t]*$`)
)

// blogPageAliases is the whitelist of page aliases this server actually renders a route for.
//
// It is a whitelist rather than "whatever the note declares" on purpose: an unknown value must not
// silently mint a route nobody registered. A typo like `page: abuot` then simply leaves the note
// as an ordinary one, which is visible, instead of creating a page that is reachable only by
// guessing its URL.
//
// `friends` is deliberately absent. A friends page belongs to the blogroll, which is on hold; an
// alias with no route would take the note out of the article list while giving the reader nowhere
// to land. Until the blogroll is built, `page: friends` therefore reads as an ordinary note and
// stays visible as one.
//
// blogPageAliases 是本服务端真正为其渲染路由的页面别名白名单。
//
// 用白名单而不是「笔记声明什么就是什么」是有意的：未知取值不可静默地造出一条没人注册过的路由。
// 写成 `page: abuot` 这样的错字，结果是这篇笔记仍是普通笔记——这是看得见的；
// 而造出一条只能靠猜 URL 才到达的页面，是看不见的。
//
// `friends` 是有意缺席的。友链页属于博客圈，而博客圈已搁置；一个有别名却没有路由的页面，
// 会把笔记从文章列表里拿走，却不让读者落在任何地方。在博客圈做出来之前，
// `page: friends` 因此仍按普通笔记处理，也仍然作为一篇普通笔记可见。
var blogPageAliases = map[string]string{
	"about": "关于",
}

// BlogPageAliases returns the page aliases in a stable order.
//
// The router registers exactly these routes, and it asks for them rather than repeating the names.
// Two lists would eventually disagree, and the failure mode of that disagreement is the bad one:
// a route for an alias the filter does not recognise, which removes a note from the article list
// and gives the reader nowhere to land.
//
// BlogPageAliases 以稳定顺序返回页面别名。
//
// 路由注册的正是这些别名，它来取而不是重抄一遍名字。两份名单早晚会不一致，而不一致的失败形态
// 恰恰是坏的那一个：出现「有路由但过滤器不认」的别名，把笔记从文章列表里拿走却不给读者落点。
func BlogPageAliases() []string {
	out := make([]string, 0, len(blogPageAliases))
	for k := range blogPageAliases {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// blogNotePage returns the declared page alias, or "" for an ordinary note.
// blogNotePage 返回声明的页面别名；普通笔记返回空串。
func blogNotePage(content string) string {
	block := frontmatterRegex.FindString(content)
	if block == "" {
		return ""
	}
	m := fmPageRegex.FindStringSubmatch(block)
	if m == nil {
		return ""
	}
	alias := strings.ToLower(strings.TrimSpace(m[1]))
	if _, ok := blogPageAliases[alias]; !ok {
		return ""
	}
	return alias
}

// blogNoteAvatar returns the avatar path declared in frontmatter, or "".
// blogNoteAvatar 返回 frontmatter 声明的头像路径，没有则为空串。
func blogNoteAvatar(content string) string {
	block := frontmatterRegex.FindString(content)
	if block == "" {
		return ""
	}
	m := fmAvatarRegex.FindStringSubmatch(block)
	if m == nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(m[1]), `"'`)
}

// blogNoteTitle returns the frontmatter `title`, or fallback when there is none.
//
// Only page notes consult this. Article titles keep coming from the file name, which is the
// behaviour every published note already has; switching articles over as well would retitle the 132
// notes in this vault that carry a `title:` field, and that is a visible change to someone else's
// site rather than part of adding a page. It belongs in its own decision.
//
// A page is different because its title is a name a reader sees as a heading — "关于" — while its
// file name is whatever the author called the file. Requiring the two to match would be asking the
// author to rename a file to control a heading.
//
// blogNoteTitle 返回 frontmatter 里的 `title`，没有时返回 fallback。
//
// 只有页面笔记会用它。文章标题仍取自文件名，也就是所有已发布笔记既有的一致行为；
// 把文章也一并改掉会让本仓库里 132 篇带 `title:` 字段的笔记换掉标题，
// 那是对别人站点的可见改动，而不是「加一个页面」的一部分。它应当单独决定。
//
// 页面不同：它的标题是读者当作标题看的名字——比如「关于」——而文件名只是作者当初给文件起的名字。
// 要求两者一致，等于让作者为了控制一个标题去改文件名。
func blogNoteTitle(content, fallback string) string {
	block := frontmatterRegex.FindString(content)
	if block == "" {
		return fallback
	}
	m := fmTitleRegex.FindStringSubmatch(block)
	if m == nil {
		return fallback
	}
	// Quotes are stripped because frontmatter writers add them freely and they are not part of
	// the title.
	//
	// 去掉引号：写 frontmatter 的人会随手加上它们，而它们不属于标题本身。
	t := strings.Trim(strings.TrimSpace(m[1]), `"'`)
	if t == "" {
		return fallback
	}
	return t
}

// blogNavName renders a folder path into a short group label.
// blogNavName 把目录路径渲染成简短的导航分组名。
func blogNavName(dir string) string {
	if dir == "" || dir == "." || dir == "/" {
		return "根目录"
	}
	return filepath.Base(dir)
}

// blogIsMeaningfulTag rejects text that parses like a tag but is really note numbering.
//
// Notes in this vault are frequently titled "10.热牙胶" / "2.根管冲洗". Those bare numbers were
// showing up in the tag cloud as tags, which is noise rather than signal. A tag must contain
// at least one letter (CJK counts as a letter), so pure-digit and pure-punctuation values are
// dropped.
//
// blogIsMeaningfulTag 过滤掉"看起来像标签但其实是笔记编号"的内容。
//
// 这个笔记库里大量文件名为「10.热牙胶」「2.根管冲洗」这类编号，裸数字此前被当成标签显示在标签云里，
// 属于噪声而非信息。因此要求标签至少包含一个字母（中文按字母计），纯数字与纯符号一律剔除。
func blogIsMeaningfulTag(tag string) bool {
	for _, r := range tag {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// blogNaturalLess compares two labels with numeric awareness, so "9." sorts before "10."
// instead of after it. Plain lexicographic order puts "10." before "2." — which is exactly the
// ordering bug seen in the sidebar.
//
// blogNaturalLess 按自然顺序比较两个标题，使「9.」排在「10.」之前而不是之后。
// 纯字典序会把「10.」排在「2.」前面，这正是侧栏里出现的排序问题。
func blogNaturalLess(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		ca, cb := ra[i], rb[j]
		if unicode.IsDigit(ca) && unicode.IsDigit(cb) {
			si, sj := i, j
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			// Fewer digits after stripping leading zeros ⇒ smaller number.
			// 去掉前导零后位数少的一侧数值更小。
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return (len(ra) - i) < (len(rb) - j)
}

// blogHomeSource is the intermediate view of one published note.
// blogHomeSource 单篇已发布笔记的中间视图。
type blogHomeSource struct {
	NoteID    int64
	VaultID   int64
	Vault     string
	Path      string
	PathHash  string
	Title     string
	Slug      string
	Content   string
	MTime     int64
	CreatedAt string
	Summary   string
	Words     int
	Tags      []string
	// Page is the page alias this note declares ("about" / "friends"), or "" for an article.
	// The list is built once and then filtered — it is not dropped here — because transclusion
	// must still be able to embed a page into another note.
	//
	// Page 是这篇笔记声明的页面别名（"about" / "friends"），文章为空串。
	// 在这里只是标记而不是剔除：转写仍需要能把一张页面嵌入到别的笔记里。
	Page string
	// Avatar is the frontmatter avatar path; only meaningful for pages.
	// Avatar 是 frontmatter 里声明的头像路径，仅对页面有意义。
	Avatar string
}

// loadBlogSources resolves every published note the blog may render.
//
// Shared by the home page and the article page: both need the same source list (path, title,
// slug, summary, tags), and duplicating the filter/dedup rules in two places is how the two
// pages would eventually disagree about what is published.
//
// loadBlogSources 解析博客可渲染的全部已发布笔记。
// 首页与文章页共用：两者都需要同一份来源清单（路径、标题、slug、摘要、标签），
// 而把过滤与去重规则抄两份，正是两个页面日后对「什么算已发布」产生分歧的原因。
func (s *shareService) loadBlogSources(ctx context.Context, uid int64) ([]*blogHomeSource, map[string]int, error) {
	shares, err := s.repo.ListByUID(ctx, uid, "created_at", "desc", 0, blogMaxShares)
	if err != nil {
		return nil, nil, err
	}

	vaultNames := make(map[int64]string)
	sources := make([]*blogHomeSource, 0, len(shares))
	seenPath := make(map[string]struct{})
	tagCount := make(map[string]int)

	for _, sh := range shares {
		if !s.isShareUsable(sh) || sh.ResType != "note" {
			continue
		}
		note, err := s.noteRepo.GetByID(ctx, sh.ResID, uid)
		if err != nil || note == nil || note.Action == "delete" {
			continue
		}
		// Same de-duplication rule as the index list: one entry per source path.
		// 与索引列表相同的去重规则：同一来源路径只保留一条。
		if _, dup := seenPath[note.Path]; dup {
			continue
		}
		seenPath[note.Path] = struct{}{}

		name, ok := vaultNames[note.VaultID]
		if !ok {
			if v, e := s.vaultRepo.GetByID(ctx, note.VaultID, uid); e == nil && v != nil {
				name = v.Name
			} else {
				name = "默认仓库"
			}
			vaultNames[note.VaultID] = name
		}

		// The entry list renders a per-note date, and CreatedAt is the one field the home page
		// never populated: the template guards the date span with {{if .CreatedAt}}, so the
		// date was silently absent from every entry while the index list showed it correctly.
		//
		// 条目列表会渲染每篇笔记的日期，而 CreatedAt 正是首页唯一从未填充的字段：
		// 模板用 {{if .CreatedAt}} 守卫日期标签，因此该日期在每条记录上都静默缺失，
		// 而索引列表却显示正常。
		createdAt := ""
		if !sh.CreatedAt.IsZero() {
			createdAt = sh.CreatedAt.Format("2006-01-02")
		}

		src := &blogHomeSource{
			NoteID:   note.ID,
			VaultID:  note.VaultID,
			Vault:    name,
			Path:     note.Path,
			PathHash: note.PathHash,
			Title:    strings.TrimSuffix(filepath.Base(note.Path), ".md"),
			Slug:     s.BuildLocalShortLink(uid, sh.ID),
			Content:  note.Content,
			// The heat map is driven by Ctime — the file's own creation time as the client
			// reports it — because that is the only one of these fields that actually spreads
			// across the vault's history. updated_timestamp is written by the sync pipeline and
			// therefore collapses onto the days the vault was first ingested: measured against
			// this vault it covered 16 distinct days, with 140 notes landing on a single one,
			// which made the calendar look like everything was written on two afternoons.
			// Ctime spanned 37 distinct days over 15 months.
			//
			// Mtime is the fallback (a note edited since import belongs to that day) and
			// updated_timestamp is the last resort for rows where the client sent no times.
			//
			// 热力图以 Ctime 为准——即客户端上报的文件创建时间——因为在这些字段里只有它真正
			// 分布在仓库的历史跨度上。updated_timestamp 由同步管线写入，因而会坍缩到仓库首次
			// 导入的那几天：在本仓库实测只覆盖 16 个不同日期，其中一天堆了 140 篇，
			// 于是日历看起来像所有笔记都是某两个下午写出来的。Ctime 则在 15 个月里覆盖 37 天。
			//
			// 回退顺序为 Mtime（导入后又被编辑过的笔记属于那一天），最后才是
			// updated_timestamp（应对客户端未上报时间的记录）。
			MTime:     firstNonZero(note.Ctime, note.Mtime, note.UpdatedTimestamp),
			CreatedAt: createdAt,
			Summary:   blogPlainText(note.Content, 90),
			Words:     blogCountChars(note.Content),
			Page:      blogNotePage(note.Content),
			Avatar:    blogNoteAvatar(note.Content),
		}
		for tag, n := range blogParseTags(note.Content) {
			tagCount[tag] += n
			src.Tags = append(src.Tags, tag)
		}
		sort.Strings(src.Tags)
		sources = append(sources, src)
	}

	return sources, tagCount, nil
}

// articlesOnly drops page notes from a source list.
//
// Every "an article" question in this file — the entry list, the word count, the activity heat map,
// the folder tree, the graph, and (through home.Entries) the feed — is derived from this list, so
// filtering here is what keeps them consistent. Filtering at each consumer instead would be nine
// places to forget, and the symptom of forgetting one is a résumé appearing where an article
// belongs.
//
// 从来源清单里剔除页面笔记。
//
// 本文件里每一处「一篇文章」的问题——条目列表、字数统计、活跃度热力图、目录树、图谱，
// 以及（经由 home.Entries）订阅源——都从这份清单派生，因此在这一处过滤就是它们保持一致的原因。
// 改成在每个消费处各自过滤，等于留下九处会被忘掉的地方，而忘掉一处的表现是
// 简历出现在了本该是文章的位置上。
func articlesOnly(sources []*blogHomeSource) []*blogHomeSource {
	out := make([]*blogHomeSource, 0, len(sources))
	for _, src := range sources {
		if src.Page != "" {
			continue
		}
		out = append(out, src)
	}
	return out
}

// pageNote returns the single published note carrying the given page alias.
//
// More than one note may declare the same alias. The most recently modified one wins, and the
// caller is told how many others exist so the settings page can say so — silently picking one is
// how an author edits the wrong file and sees nothing change.
//
// pageNote 返回带有指定页面别名的那一篇已发布笔记。
//
// 可能有不止一篇笔记声明同一个别名。此时取最近修改的那一篇，并把其余的篇数回报给调用者，
// 以便设置页说明这件事——静默取一篇，正是作者改了另一篇却看不到任何变化的原因。
func pageNote(sources []*blogHomeSource, alias string) (*blogHomeSource, int) {
	var best *blogHomeSource
	count := 0
	for _, src := range sources {
		if src.Page != alias {
			continue
		}
		count++
		if best == nil || src.MTime > best.MTime {
			best = src
		}
	}
	return best, count
}

// GetBlogHome assembles every dataset the blog home page renders.
//
// publishedOnly restricts the knowledge graph to notes that are themselves published. That is
// the default because the blog is public: drawing the full vault would leak the titles of notes
// the user never chose to publish.
//
// GetBlogHome 组装博客首页所需的全部数据。
// publishedOnly 把知识图谱限制在「本身已发布」的笔记上。这是默认行为，因为博客是公开页面：
// 若绘制整个仓库，会泄露用户并未选择公开的笔记标题。
func (s *shareService) GetBlogHome(ctx context.Context, uid int64, baseURL string, publishedOnly bool, maxNodes int) (*dto.BlogHomeDTO, error) {
	all, tagCount, err := s.loadBlogSources(ctx, uid)
	if err != nil {
		return nil, err
	}
	// Page notes are pages, not articles: they leave the entry list, the word count, the folder
	// tree, the heat map and the graph here, and the feed follows because it is built from Entries.
	// They remain reachable at their alias route.
	//
	// 页面笔记是「页面」而不是「文章」：它们在这里离开条目列表、字数统计、目录树、
	// 热力图与图谱，而订阅源因为是按 Entries 构建的，也随之排除。它们仍可在别名路由上访问。
	sources := articlesOnly(all)

	home := &dto.BlogHomeDTO{
		HomeURL: strings.TrimRight(baseURL, "/") + "/blog",
		// Nav.Pages is filled from `all`, not from the filtered `sources`: the top bar's page
		// entries are exactly the notes that the article list just dropped.
		//
		// Nav.Pages 取自 `all` 而不是过滤后的 `sources`：顶栏的页面入口，
		// 正是文章列表刚刚剔除掉的那些笔记。
		Nav: &dto.BlogNavDTO{
			Tree:  []dto.BlogNavNode{},
			Total: len(sources),
			Pages: blogPageLinks(all),
		},
		Entries: make([]*dto.BlogEntryDTO, 0, len(sources)),
		Tags:    make([]*dto.BlogTagDTO, 0, len(tagCount)),
		Stats:   &dto.BlogStatsDTO{Notes: len(sources), Tags: len(tagCount)},
	}

	// ── Knowledge graph edges (note_link is the real source of truth) ──
	// ── 知识图谱的边（note_link 是真实的权威数据源）──
	byHash := make(map[string]*blogHomeSource, len(sources))
	byNoteID := make(map[int64]*blogHomeSource, len(sources))
	for _, src := range sources {
		if src.PathHash != "" {
			byHash[src.PathHash] = src
		}
		// A wiki link is usually written as `[[note]]` or `[[folder/note]]` rather than as
		// the full vault path, and it never carries the .md extension. The link table stores
		// that raw text, so matching on src.PathHash alone resolved almost nothing — measured
		// on this vault, 13 of 4666 link rows. Indexing every suffix variation lets a link
		// resolve regardless of how much of the path its author typed.
		//
		// wiki 链接通常写成 `[[笔记]]` 或 `[[文件夹/笔记]]`，而不是完整仓库路径，并且从不
		// 带 .md 后缀。链接表存的就是这段原始文本，所以只按 src.PathHash 匹配几乎解析不出
		// 任何东西——实测本仓库 4666 条链接里只有 13 条。把每个后缀变体都索引进去，作者
		// 写了多少层路径都能解析到同一篇笔记。
		for _, variation := range util.GeneratePathVariations(src.Path) {
			h := util.EncodeHash32(variation)
			if h == "" {
				continue
			}
			if _, exists := byHash[h]; !exists {
				byHash[h] = src
			}
		}
		byNoteID[src.NoteID] = src
	}

	linksOut := make(map[int64]int)
	linksIn := make(map[int64]int)
	graphEdges := make([]*dto.BlogGraphEdge, 0, 256)
	graphNodes := make(map[int64]*dto.BlogGraphNode, len(sources))
	extras := make(map[string]*dto.BlogGraphNode)

	for _, src := range sources {
		graphNodes[src.NoteID] = &dto.BlogGraphNode{
			ID:    "n" + itoa(src.NoteID),
			Label: src.Title,
			URL:   "/blog/" + src.Slug,
			Lit:   true,
		}
	}

	for _, src := range sources {
		if s.noteLinkRepo == nil {
			break
		}
		out, err := s.noteLinkRepo.GetOutlinks(ctx, src.NoteID, uid)
		if err != nil {
			continue
		}
		for _, l := range out {
			if l.IsEmbed {
				continue // 嵌入不算知识关联 / embeds are not knowledge links
			}
			target, ok := byHash[l.TargetPathHash]
			var targetID string
			if ok {
				targetID = "n" + itoa(target.NoteID)
				linksOut[src.NoteID]++
				linksIn[target.NoteID]++
			} else if publishedOnly {
				continue // 目标未发布：不出现在公开图谱里 / target not published
			} else {
				// Unpublished neighbour: keep the node anonymous (label withheld) so the
				// graph shows connectivity without leaking any unshared title.
				// 未发布的邻居：保留为匿名节点（不给标题），让图谱体现连接关系但不泄露未分享标题。
				key := "x_" + l.TargetPathHash
				if _, seen := extras[key]; !seen {
					extras[key] = &dto.BlogGraphNode{
						ID:  "x" + l.TargetPathHash,
						Lit: false,
						Deg: 0,
						URL: "",
					}
					extras[key].Label = "未公开笔记"
				}
				targetID = extras[key].ID
			}
			graphEdges = append(graphEdges, &dto.BlogGraphEdge{
				Source: "n" + itoa(src.NoteID),
				Target: targetID,
			})
		}
	}

	// ── Entries, folder tree, activity heat map, stats ──
	// ── 条目、目录树、活跃度热力图、统计 ──
	root := &blogTreeNode{name: "", children: map[string]*blogTreeNode{}}
	totalLinks := 0
	var latest int64
	// earliest bounds the heat map so the window can span the vault's real history instead of
	// a fixed trailing range that most notes fall outside of.
	// earliest 用于界定热力图的窗口，使其横跨仓库真实的历史跨度，
	// 而不是让大多数笔记落在固定回溯窗口之外。
	var earliest int64
	dayCount := map[string]int{}

	for _, src := range sources {
		home.Entries = append(home.Entries, &dto.BlogEntryDTO{
			Slug:      src.Slug,
			Title:     src.Title,
			Path:      src.Path,
			Summary:   src.Summary,
			Words:     src.Words,
			LinksOut:  linksOut[src.NoteID],
			LinksIn:   linksIn[src.NoteID],
			CreatedAt: src.CreatedAt,
			Vault:     src.Vault,
		})
		home.Stats.Words += src.Words
		totalLinks += linksOut[src.NoteID]
		if src.MTime > latest {
			latest = src.MTime
		}
		if src.MTime > 0 {
			dayCount[time.UnixMilli(src.MTime).Format("2006-01-02")]++
			if earliest == 0 || src.MTime < earliest {
				earliest = src.MTime
			}
		}

		// Insert the note into the folder tree one path segment at a time, so the sidebar can
		// render genuine nesting instead of a flat list of top-level groups.
		// 按路径逐级插入目录树，让侧栏能渲染真正的层级嵌套，而不是平铺的一层分组。
		segs := strings.Split(filepath.ToSlash(src.Path), "/")
		node := root
		for i, seg := range segs {
			if i == len(segs)-1 {
				node.addNote(src.Title, src.Slug)
			} else if seg != "" {
				node = node.child(seg)
			}
		}
	}

	home.Nav = &dto.BlogNavDTO{Tree: root.toDTO(), Total: len(sources)}
	home.Activity = buildActivity(dayCount, earliest, 14, 53)

	home.Stats.Links = totalLinks
	home.Stats.Folders = root.folderCount()
	if home.Stats.Notes > 0 {
		home.Stats.AvgWords = home.Stats.Words / home.Stats.Notes
	}
	if latest > 0 {
		home.Stats.Updated = time.UnixMilli(latest).Format("2006-01-02")
	}

	// ── Tag cloud (most used first) ──
	// ── 标签云（按使用次数降序）──
	tags := make([]*dto.BlogTagDTO, 0, len(tagCount))
	for name, n := range tagCount {
		tags = append(tags, &dto.BlogTagDTO{Name: name, Count: n})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count != tags[j].Count {
			return tags[i].Count > tags[j].Count
		}
		return tags[i].Name < tags[j].Name
	})
	home.Tags = tags

	// ── Graph layout ──
	// ── 图谱布局 ──
	nodes := make([]*dto.BlogGraphNode, 0, len(graphNodes)+len(extras))
	for _, n := range graphNodes {
		nodes = append(nodes, n)
	}
	if !publishedOnly {
		for _, n := range extras {
			nodes = append(nodes, n)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	truncated := false
	if maxNodes > 0 && len(nodes) > maxNodes {
		nodes = nodes[:maxNodes]
		truncated = true
		keep := make(map[string]struct{}, len(nodes))
		for _, n := range nodes {
			keep[n.ID] = struct{}{}
		}
		kept := graphEdges[:0]
		for _, e := range graphEdges {
			if _, a := keep[e.Source]; !a {
				continue
			}
			if _, b := keep[e.Target]; !b {
				continue
			}
			kept = append(kept, e)
		}
		graphEdges = kept
	}

	deg := make(map[string]int, len(nodes))
	for _, e := range graphEdges {
		deg[e.Source]++
		deg[e.Target]++
	}
	for _, n := range nodes {
		n.Deg = deg[n.ID]
	}

	// A roomier canvas than the first pass used: captions need horizontal room, and the
	// three-column layout gives the graph roughly this aspect ratio.
	// 画布比初版更宽敞：标签需要横向空间，三栏布局给图谱的也正是这个比例。
	const gw, gh = 1120, 760
	// Guard: maxNodes == 0 means the caller supplied no budget. Without this check a caller
	// that only wants the page to render still pays for an O(n²) relaxation across every node
	// in the vault — including the anonymous nodes minted for unpublished neighbours.
	//
	// 守护：maxNodes == 0 表示调用方没有给出预算。缺少这道判断，只想把页面渲染出来的调用方
	// 仍会为仓库中每个节点（含为未发布邻居铸造的匿名节点）付一次 O(n²) 松弛的代价。
	if maxNodes > 0 {
		layoutGraph(nodes, graphEdges, gw, gh, 1100)
	}

	home.Graph = &dto.BlogGraphDTO{
		Nodes:         nodes,
		Edges:         graphEdges,
		Width:         gw,
		Height:        gh,
		Truncated:     truncated,
		PublishedOnly: publishedOnly,
	}

	return home, nil
}

// GetBlogNav returns only the folder navigation tree.
//
// The article page renders the same sidebar as the home page but none of its other datasets.
// It used to call GetBlogHome with maxNodes=0 and publishedOnly=false purely to reach Nav.Tree,
// which meant every article view re-ran the full home assembly: two N+1 query passes (note
// lookups and per-note outlinks), the force-directed layout, the heat map and the tag cloud —
// and it built anonymous nodes for unpublished notes even though the page shows no graph.
//
// GetBlogNav 只返回目录导航树。
// 文章页渲染的侧栏与首页相同，但不需要首页的其它数据集。此前它调用 GetBlogHome
// 并传 maxNodes=0、publishedOnly=false，只为拿到 Nav.Tree —— 于是每次打开文章都会重跑一遍
// 完整的首页组装：两轮 N+1 查询（笔记查找与逐篇出链）、力导向布局、热力图与标签云；
// 并且即使该页根本不显示图谱，也会为未发布笔记创建匿名节点。
func (s *shareService) GetBlogNav(ctx context.Context, uid int64) (*dto.BlogNavDTO, error) {
	all, _, err := s.loadBlogSources(ctx, uid)
	if err != nil {
		return nil, err
	}
	// The nav tree is a table of contents for articles, so page notes stay out of it.
	// 目录树是文章的目录，因此页面笔记不进其中。
	sources := articlesOnly(all)
	root := &blogTreeNode{name: "", children: map[string]*blogTreeNode{}}
	for _, src := range sources {
		segs := strings.Split(filepath.ToSlash(src.Path), "/")
		node := root
		for i, seg := range segs {
			if i == len(segs)-1 {
				node.addNote(src.Title, src.Slug)
			} else if seg != "" {
				node = node.child(seg)
			}
		}
	}
	return &dto.BlogNavDTO{
		Tree:  root.toDTO(),
		Total: len(sources),
		// Pages ride along because this function has already read every published note: listing
		// them here costs nothing extra, while a separate call would re-read the whole set on
		// every article view just to find out whether a résumé exists.
		//
		// 页面入口随它一起返回，因为这个函数已经读过全部已发布笔记：在这里列出它们不额外花钱，
		// 而单独再查一次，会在每次文章浏览时为「有没有简历」这件事把整份清单重读一遍。
		Pages: blogPageLinks(all),
	}, nil
}

// blogPageLinks turns the page notes in a source list into top-bar entries.
//
// A page is offered only when one actually exists: a link that 404s is worse than no link, and
// the alias whitelist means the set is known in advance rather than discovered by trying.
//
// blogPageLinks 把来源清单里的页面笔记转成顶栏入口。
//
// 只有页面确实存在时才给出入口：一个 404 的链接比没有链接更糟，
// 而别名白名单意味着这个集合是预先知道的，不必靠试。
func blogPageLinks(sources []*blogHomeSource) []dto.BlogNavPageDTO {
	out := make([]dto.BlogNavPageDTO, 0, len(blogPageAliases))
	for _, alias := range BlogPageAliases() {
		src, _ := pageNote(sources, alias)
		if src == nil {
			continue
		}
		title := blogNoteTitle(src.Content, blogPageAliases[alias])
		out = append(out, dto.BlogNavPageDTO{
			Alias: alias,
			Title: title,
			URL:   "/blog/" + alias,
		})
	}
	return out
}

// itoa is a tiny helper so graph ids stay allocation-light.
// itoa 是生成图谱 id 的小工具。
func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// firstNonZero returns the first positive value, used to pick the best available timestamp
// from the several candidates the note table carries.
// firstNonZero 返回第一个正值，用于在笔记表的多个候选时间戳中挑一个可用的。
func firstNonZero(vals ...int64) int64 {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

// ── Folder tree ──────────────────────────────────────────────────────

// blogTreeNode is the mutable tree built while folding note paths into a hierarchy.
// blogTreeNode 是把笔记路径折叠成层级结构时构建的可变树节点。
type blogTreeNode struct {
	name     string
	slug     string // non-empty ⇒ this node is a note // 非空表示这是笔记节点
	children map[string]*blogTreeNode
	order    []string // insertion order, keeps output stable // 插入顺序，保证输出稳定
}

func (n *blogTreeNode) child(name string) *blogTreeNode {
	if n.children == nil {
		n.children = map[string]*blogTreeNode{}
	}
	if c, ok := n.children[name]; ok {
		return c
	}
	c := &blogTreeNode{name: name}
	n.children[name] = c
	n.order = append(n.order, name)
	return c
}

func (n *blogTreeNode) addNote(title, slug string) {
	if n.children == nil {
		n.children = map[string]*blogTreeNode{}
	}
	if _, ok := n.children[title]; ok {
		return
	}
	n.children[title] = &blogTreeNode{name: title, slug: slug}
	n.order = append(n.order, title)
}

// toDTO converts the tree into the template-facing shape: folders first, notes after, with
// numeric-aware ordering inside each group so "9." precedes "10.".
// toDTO 把树转换成模板使用的结构：文件夹在前、笔记在后，组内按数字感知的自然顺序排列，
// 使「9.」排在「10.」之前。
func (n *blogTreeNode) toDTO() []dto.BlogNavNode {
	return n.toDTOAt(1)
}

// toDTOAt is toDTO with the current folder depth, so the template can decide which folders start
// expanded. Depth is carried in the data rather than counted in the template because Go templates
// have no counter that survives recursion, and because the value belongs to the tree.
//
// toDTOAt 是带当前文件夹深度的 toDTO，使模板能决定哪些文件夹默认展开。
// 深度写在数据里而不是在模板里数：Go 模板没有能跨递归存活的计数器，而且这个值本就属于这棵树。
func (n *blogTreeNode) toDTOAt(depth int) []dto.BlogNavNode {
	out := make([]dto.BlogNavNode, 0, len(n.order))
	for _, key := range n.order {
		c, ok := n.children[key]
		if !ok {
			continue
		}
		node := dto.BlogNavNode{Name: c.name, Slug: c.slug, Depth: depth}
		if len(c.children) > 0 {
			node.Children = c.toDTOAt(depth + 1)
			for _, ch := range node.Children {
				node.Count += ch.Count
			}
		} else {
			node.Count = 1
		}
		out = append(out, node)
	}
	sort.SliceStable(out, func(i, j int) bool {
		fi, fj := len(out[i].Children) > 0, len(out[j].Children) > 0
		if fi != fj {
			return fi
		}
		return blogNaturalLess(out[i].Name, out[j].Name)
	})
	return out
}

func (n *blogTreeNode) folderCount() int {
	total := 0
	for _, c := range n.children {
		if len(c.children) > 0 {
			total += 1 + c.folderCount()
		}
	}
	return total
}

// ── Activity heat map ────────────────────────────────────────────────

// buildActivity aggregates note timestamps into a GitHub-style heat map.
//
// The window adapts to the corpus: it always covers at least minWeeks and at most maxWeeks,
// stretching so the earliest published note is visible. A fixed trailing window was the wrong
// shape here — once the map moved to each file's real creation time, most of the vault sat
// fifteen months back and fell outside a four-month strip, leaving it nearly empty.
//
// What this is: each note's own timestamp, not an edit log. So it is a map of when the
// published notes were written (or last touched, for ones edited after import) rather than a
// daily output chart.
//
// buildActivity 把笔记时间戳聚合成 GitHub 风格的热力图。
//
// 窗口随语料自适应：至少 minWeeks 周、至多 maxWeeks 周，并拉伸到能看见最早一篇已发布笔记。
// 固定回溯窗口在这里是错的形状——热力图改用文件真实创建时间之后，仓库里大部分内容位于
// 十五个月前，落在四个月的色带之外，整张图会近乎空白。
//
// 它是「每篇笔记自己的时间戳」，而非编辑日志：反映已发布笔记写于何时
// （导入后又被编辑过的则记其最后一次改动那天），而不是每日产出量。
func buildActivity(dayCount map[string]int, earliestMS int64, minWeeks, maxWeeks int) *dto.BlogActivityDTO {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// A week column starts on Sunday, so the first column is aligned back to the containing week.
	// 每列以周日开头，因此首列要对齐到最早那天所属的那一周。
	spanDays := 0
	if earliestMS > 0 {
		ed := time.UnixMilli(earliestMS)
		ed = time.Date(ed.Year(), ed.Month(), ed.Day(), 0, 0, 0, 0, now.Location())
		spanDays = int(today.Sub(ed).Hours() / 24)
	}
	weeks := minWeeks
	if spanDays > 0 {
		need := (spanDays+int(today.Weekday()))/7 + 1
		if need > weeks {
			weeks = need
		}
	}
	if weeks > maxWeeks {
		weeks = maxWeeks
	}

	start := today.AddDate(0, 0, -int(today.Weekday())-(weeks-1)*7)

	days := make([]dto.BlogDayDTO, 0, weeks*7)
	maxCount, noteDays := 0, 0
	seenMonth := map[string]bool{}
	months := make([]string, 0, 6)

	for w := 0; w < weeks; w++ {
		colFirst := true
		for d := 0; d < 7; d++ {
			day := start.AddDate(0, 0, w*7+d)
			if day.After(today) {
				continue
			}
			key := day.Format("2006-01-02")
			c := dayCount[key]
			if c > maxCount {
				maxCount = c
			}
			if c > 0 {
				noteDays++
			}
			lvl := c
			if lvl > 4 {
				lvl = 4
			}
			days = append(days, dto.BlogDayDTO{Date: key, Count: c, Week: w, Dow: d, Lvl: lvl})
			if colFirst {
				if mk := day.Format("1月"); !seenMonth[mk] {
					seenMonth[mk] = true
					months = append(months, mk)
				}
				colFirst = false
			}
		}
	}
	return &dto.BlogActivityDTO{Days: days, Weeks: weeks, NoteDays: noteDays, Max: maxCount, Months: months}
}
