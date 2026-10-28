package service

import (
	"context"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"github.com/haierkeys/fast-note-sync-service/pkg/util"
	"go.uber.org/zap"
)

// Article covers.
//
// A cover is declared in the note's frontmatter as three flat keys:
//
//	cover: 图片/牙发育-1.png
//	cover-focus: 50% 35%
//	cover-zoom: 1.4
//
// Flat keys rather than a nested mapping because frontmatter here is read line by line, not
// parsed as YAML: a malformed frontmatter must cost one line of metadata, not the whole note.
//
// 文章封面。
//
// 封面在笔记的 frontmatter 里用三个平铺的键声明（见上方示例）。
//
// 之所以用平铺键而不是嵌套映射：这里的 frontmatter 是逐行读取的，不是按 YAML 解析的——
// 一段畸形的 frontmatter 只应该损失一行元数据，而不应该连累整篇笔记。

// blogCover describes how one article's cover should be drawn.
// blogCover 描述一篇文章的封面该怎么画。
type blogCover struct {
	// URL is an absolute-izable path served by the share file endpoint.
	// URL 是分享文件端点提供的、可绝对化的路径。
	URL string
	// Kind is "image" or "video", decided by extension.
	// Kind 是 "image" 或 "video"，由扩展名决定。
	Kind string
	// MIME is the content type of the referenced file, for feed enclosures.
	// MIME 是所引用文件的内容类型，供订阅源的附件字段使用。
	MIME string
	// FocusX, FocusY are percentages in [0,100]; they name the point of the image that must stay
	// visible when the square crop is applied.
	//
	// FocusX、FocusY 是 [0,100] 之间的百分比，指的是正方形裁切时必须保留的那个点。
	FocusX float64
	FocusY float64
	// Zoom is a multiplier on the drawn size; 1 means "exactly cover the square".
	// Zoom 是绘制尺寸的倍数；1 表示正好铺满正方形。
	Zoom float64
}

// Defaults chosen so that a note carrying only `cover:` still looks intentional.
// 这些默认值让只写了 `cover:` 的笔记看起来仍然是刻意为之。
const (
	blogCoverDefaultFocus = 50.0
	blogCoverDefaultZoom  = 1.0
	blogCoverMinZoom      = 1.0
	blogCoverMaxZoom      = 4.0
	// blogCoverCandidateLimit bounds the suffix query when a bare filename has to be resolved.
	// Two files may share a name; more than this many sharing one name means the vault itself
	// needs cleaning, not that the reader needs a longer list.
	//
	// blogCoverCandidateLimit 限定裸文件名解析时的后缀查询规模。
	// 两个文件可能同名；同名超过这个数量说明该整理仓库了，而不是读者需要更长的列表。
	blogCoverCandidateLimit = 32
)

// blogCoverMediaType is one entry of the extension table: how to draw the file, and what to call
// it on the wire.
//
// MIME is carried alongside Kind rather than derived later from the URL, because the served
// address is `/api/share/file?id=…&share_token=…` and has no extension in it. The feed needs a
// content type for `<enclosure type>` and `<media:content type>`, and every extension that can
// reach this table is already known here — so the answer is decided once, at parse time, instead
// of being guessed back out of a query string.
//
// blogCoverMediaType 是扩展名表的条目：该怎么画，以及在网络传输时该叫什么类型。
//
// MIME 与 Kind 并列保存，而不是事后从 URL 推导，因为对外地址是
// `/api/share/file?id=…&share_token=…`，里面没有扩展名。订阅源需要 `<enclosure type>` 与
// `<media:content type>` 的内容类型，而能走进这张表的扩展名这里本来就已经知道——
// 所以答案在解析时决定一次，而不是事后从查询串里猜回来。
type blogCoverMediaType struct {
	Kind string
	MIME string
}

// blogCoverExtensions decides kind by extension, and also decides whether a reference is a cover
// at all: anything else (a PDF, a note) is not something a square card can show.
//
// blogCoverExtensions 按扩展名判断类型，同时判断一个引用是否算封面：
// 其它格式（PDF、笔记）都不是正方形卡片能展示的东西。
var blogCoverExtensions = map[string]blogCoverMediaType{
	".png":  {Kind: "image", MIME: "image/png"},
	".jpg":  {Kind: "image", MIME: "image/jpeg"},
	".jpeg": {Kind: "image", MIME: "image/jpeg"},
	".webp": {Kind: "image", MIME: "image/webp"},
	".gif":  {Kind: "image", MIME: "image/gif"},
	".avif": {Kind: "image", MIME: "image/avif"},
	".bmp":  {Kind: "image", MIME: "image/bmp"},
	".svg":  {Kind: "image", MIME: "image/svg+xml"},
	".mp4":  {Kind: "video", MIME: "video/mp4"},
	".webm": {Kind: "video", MIME: "video/webm"},
	".mov":  {Kind: "video", MIME: "video/quicktime"},
	".m4v":  {Kind: "video", MIME: "video/x-m4v"},
}

// parseBlogCover reads the three cover keys out of a note's frontmatter block.
//
// It returns nil when there is no cover, when the value is not a recognised media type, or when
// the frontmatter is absent. Every failure is the same failure: the article is drawn without a
// cover, which is exactly what a note that never declared one looks like. That collapse is
// deliberate — a broken cover must not be able to make a note fail to render.
//
// parseBlogCover 从笔记的 frontmatter 块里读出三个封面键。
//
// 没有封面、值的类型不认识、或者根本没有 frontmatter，都返回 nil。所有失败都是同一种失败：
// 这篇文章不带封面，与从未声明过封面的笔记表现完全一致。这种合并是有意的——
// 一个坏掉的封面不应该有能力让一篇笔记渲染失败。
func parseBlogCover(content, notePath string) (ref string, cover *blogCover) {
	block := frontmatterRegex.FindString(content)
	if strings.TrimSpace(block) == "" {
		return "", nil
	}

	raw := strings.TrimSpace(frontmatterValue(block, fmCoverRegex))
	if raw == "" {
		return "", nil
	}
	// YAML habit: authors quote paths that contain spaces or brackets.
	// YAML 习惯：作者会给含空格或方括号的路径加引号。
	raw = strings.Trim(raw, `"'`)

	// An Obsidian embed written straight into the key (`cover: "![[图.png]]"`) is normalised to
	// its bare target, so both spellings work.
	// 直接写成 Obsidian 嵌入形式（`cover: "![[图.png]]"`）会归约成裸目标，两种写法都成立。
	ref = blogCoverRefTarget(raw)
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}

	media, ok := blogCoverExtensions[strings.ToLower(path.Ext(ref))]
	if !ok {
		return "", nil
	}

	cover = &blogCover{
		Kind:   media.Kind,
		MIME:   media.MIME,
		FocusX: blogCoverDefaultFocus,
		FocusY: blogCoverDefaultFocus,
		Zoom:   blogCoverDefaultZoom,
	}

	if fx, fy, ok := parseBlogCoverFocus(frontmatterValue(block, fmCoverFocusRegex)); ok {
		cover.FocusX, cover.FocusY = fx, fy
	}
	if z, ok := parseBlogCoverZoom(frontmatterValue(block, fmCoverZoomRegex)); ok {
		cover.Zoom = z
	}
	return ref, cover
}

// frontmatterValue returns the first capture group of keyRegex inside the frontmatter block.
// frontmatterValue 返回 frontmatter 块里 keyRegex 的第一个捕获组。
func frontmatterValue(block string, keyRegex interface{ FindStringSubmatch(string) []string }) string {
	m := keyRegex.FindStringSubmatch(block)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// parseBlogCoverFocus accepts "50% 35%", "50 35" and "50%,35%" — the spellings a human or a
// plugin is likely to produce.
//
// Values are clamped rather than rejected: a focus outside the image is meaningless but harmless,
// and refusing it would silently drop the author's crop instead of pinning it to the edge.
//
// parseBlogCoverFocus 接受 "50% 35%"、"50 35"、"50%,35%" 这些人或插件容易写出的形式。
//
// 越界的值被夹取而不是拒绝：焦点落在图外没有意义但也不会有害，
// 而拒绝它等于悄悄丢弃作者的裁切意图，而不是把它贴到边缘。
func parseBlogCoverFocus(raw string) (x, y float64, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, false
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t'
	})
	if len(fields) < 2 {
		return 0, 0, false
	}
	fx, err1 := strconv.ParseFloat(strings.TrimSuffix(fields[0], "%"), 64)
	fy, err2 := strconv.ParseFloat(strings.TrimSuffix(fields[1], "%"), 64)
	if err1 != nil || err2 != nil || math.IsNaN(fx) || math.IsNaN(fy) {
		return 0, 0, false
	}
	return clampPercent(fx), clampPercent(fy), true
}

// parseBlogCoverZoom accepts a bare multiplier and clamps it to the supported range.
// parseBlogCoverZoom 接受一个裸倍数，并夹取到支持范围。
func parseBlogCoverZoom(raw string) (float64, bool) {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), "x"))
	if raw == "" {
		return 0, false
	}
	z, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(z) || z <= 0 {
		return 0, false
	}
	if z < blogCoverMinZoom {
		z = blogCoverMinZoom
	}
	if z > blogCoverMaxZoom {
		z = blogCoverMaxZoom
	}
	return z, true
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// resolveBlogCoverFile finds the image or video a cover reference names.
//
// Resolution order, and why it is not simply GetByPathLike:
//
//  1. Paths built relative to the note's own folder. This is exact, and it is the common case:
//     a note that says `cover: 图.png` almost always means the file next to it.
//  2. A suffix query, which may return several files. Here the choice is made by proximity —
//     the candidate sharing the longest path prefix with the note wins, ties broken by full path.
//     GetByPathLike would have returned whichever row the database happened to order first, so
//     two folders each holding `图.png` would show the same cover on unrelated articles.
//  3. Nothing. The caller draws the placeholder.
//
// resolveBlogCoverFile 找出封面引用所指的图片或视频。
//
// 解析顺序，以及为什么不能简单地用 GetByPathLike：
//
//  1. 按笔记自身所在目录拼接的路径。这是精确匹配，也是常见情形：
//     写 `cover: 图.png` 的笔记几乎总是指向和它并排的那个文件。
//  2. 后缀查询，可能命中多个文件。这里按贴近程度选择——与笔记共享最长路径前缀的候选胜出，
//     并列时按完整路径排序。若用 GetByPathLike，返回的会是数据库碰巧排在前面的那一行，
//     于是两个目录下各有一个 `图.png` 时，互不相关的文章会显示同一张封面。
//  3. 什么都没找到，调用方画占位。
func (s *shareService) resolveBlogCoverFile(ctx context.Context, uid, vaultID int64, notePath, ref string) (*domain.File, error) {
	if s.fileRepo == nil || ref == "" {
		return nil, nil
	}

	for _, candidate := range buildSharePathCandidates(notePath, ref) {
		file, err := s.fileRepo.GetByPath(ctx, candidate, vaultID, uid)
		if err == nil && file != nil && file.Action != domain.FileActionDelete {
			return file, nil
		}
	}

	suffix := normalizeShareVaultPath(ref)
	if suffix == "" {
		return nil, nil
	}
	list, err := s.fileRepo.ListByPathSuffix(ctx, suffix, vaultID, uid, blogCoverCandidateLimit)
	if err != nil || len(list) == 0 {
		return nil, nil
	}
	return pickNearestFile(list, notePath), nil
}

// pickNearestFile chooses the candidate closest to the note in the folder tree.
//
// "Closest" is the length of the shared path prefix counted by whole segments, so
// `口腔笔记/口组笔记/x.png` beats `图片/x.png` for a note under `口腔笔记/口组笔记/`. Counting
// segment prefixes rather than raw characters matters: `口腔笔记A/` and `口腔笔记B/` share seven
// characters but no directory at all.
//
// Ties are broken by full path so the result is deterministic — two builds of the same vault must
// not disagree about which image an article shows.
//
// pickNearestFile 选择在目录树中离笔记最近的候选。
//
// "最近"按整段计的共同路径前缀长度，因此对于 `口腔笔记/口组笔记/` 下的笔记，
// `口腔笔记/口组笔记/x.png` 会胜过 `图片/x.png`。按整段而不是按字符计很重要：
// `口腔笔记A/` 与 `口腔笔记B/` 共享七个字符，却连一个目录都不共享。
//
// 并列时按完整路径排序，使结果确定——同一份仓库的两次构建不应对"这篇文章显示哪张图"给出不同答案。
func pickNearestFile(list []*domain.File, notePath string) *domain.File {
	if len(list) == 0 {
		return nil
	}
	if len(list) == 1 {
		return list[0]
	}

	noteSegs := strings.Split(strings.Trim(path.Dir(filepathSlash(notePath)), "/"), "/")

	best := list[0]
	bestShared := -1
	for _, f := range list {
		shared := sharedDirSegments(noteSegs, strings.Split(strings.Trim(path.Dir(filepathSlash(f.Path)), "/"), "/"))
		if shared > bestShared {
			best, bestShared = f, shared
			continue
		}
		if shared == bestShared && f.Path < best.Path {
			best = f
		}
	}
	return best
}

// sharedDirSegments counts how many leading path segments two directory paths agree on.
// sharedDirSegments 统计两个目录路径在开头共同拥有的段数。
func sharedDirSegments(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) {
		if !strings.EqualFold(a[n], b[n]) {
			return n
		}
		n++
	}
	return n
}

// filepathSlash normalises separators so the same logic works for vault paths stored with either.
//
// It is written here rather than imported because path/filepath's Version is OS-dependent, and this
// code must give identical answers on the Linux server and on a Windows development box.
//
// filepathSlash 归一化分隔符，使同一套逻辑对两种分隔符存储的仓库路径都成立。
//
// 这里自己写而不是引入 filepath，是因为 filepath 的行为依赖操作系统，
// 而这段代码必须在 Linux 服务器和 Windows 开发机上给出完全相同的答案。
func filepathSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// blogCoverDTO converts the internal cover into the shape templates and JSON consume.
//
// A nil cover stays nil rather than becoming an empty struct: the templates branch on presence
// (`{{if .Cover}}`), and an empty struct would make every card claim to have a cover it cannot draw.
//
// blogCoverDTO 把内部封面转换成模板与 JSON 使用的形状。
//
// nil 封面保持 nil 而不是变成空结构体：模板按存在与否分支（`{{if .Cover}}`），
// 而空结构体会让每张卡片都声称自己有封面、却画不出来。
func blogCoverDTO(c *blogCover) *dto.BlogCoverDTO {
	if c == nil || c.URL == "" {
		return nil
	}
	return &dto.BlogCoverDTO{
		URL:    c.URL,
		Kind:   c.Kind,
		FocusX: c.FocusX,
		FocusY: c.FocusY,
		Zoom:   c.Zoom,
	}
}

// blogCoverRefTarget normalises the several ways an author may spell a cover reference down to a
// bare vault path.
//
// Accepted: `图.png`, `![[图.png]]`, `[[图.png]]`, `![[图.png|任意别名]]`, `![[图.png#任意锚点]]`.
// The alias and anchor are dropped because a square cover has no room for either, and because the
// frontmatter key is not a place where Obsidian's display syntax means anything.
//
// extractObsidianEmbedPath cannot be reused directly here: it takes the text *inside* the
// brackets, and the frontmatter value is the whole `![[...]]` string. Measured before this
// wrapper existed, `cover: "![[牙发育-1.png]]"` was parsed as a file literally named
// "![[牙发育-1.png]]" and every such cover silently fell back to the placeholder.
//
// blogCoverRefTarget 把作者可能写出的几种封面引用归一成裸仓库路径。
//
// 接受：`图.png`、`![[图.png]]`、`[[图.png]]`、`![[图.png|任意别名]]`、`![[图.png#任意锚点]]`。
// 别名与锚点被丢弃，因为正方形封面容不下它们，也因为 frontmatter 的键不是 Obsidian
// 展示语法能表达意义的地方。
//
// 这里不能直接复用 extractObsidianEmbedPath：它接收的是方括号*内部*的文本，
// 而 frontmatter 的值是整串 `![[...]]`。在这个包装函数存在之前实测，
// `cover: "![[牙发育-1.png]]"` 会被解析成一个名字就叫 "![[牙发育-1.png]]" 的文件，
// 于是所有这种写法的封面都悄悄退化成了占位。
func blogCoverRefTarget(raw string) string {
	s := strings.TrimSpace(raw)
	for _, p := range []string{"![[", "[["} {
		if strings.HasPrefix(s, p) && strings.HasSuffix(s, "]]") {
			inner := strings.TrimSuffix(strings.TrimPrefix(s, p), "]]")
			return strings.TrimSpace(extractObsidianEmbedPath(inner))
		}
	}
	return strings.TrimSpace(extractObsidianEmbedPath(s))
}

// blogCoverCSSFocus renders the focus point as a background-position / object-position value.
// blogCoverCSSFocus 把焦点渲染成 background-position / object-position 的取值。
func blogCoverCSSFocus(x, y float64) string {
	return strconv.FormatFloat(x, 'f', -1, 64) + "% " + strconv.FormatFloat(y, 'f', -1, 64) + "%"
}

// ensureCoverResource makes sure a cover image is authorized by the note's share record.
//
// This is self-healing rather than an error path, because the alternative symptom has no
// explanation anywhere the user can see: /api/share/file authorizes against the share record's
// Resources, and a cover picked from outside the article body was never in that list. The note
// renders, every body image renders, and only the cover is a 403.
//
// The write is synchronous on purpose. Building the URL before the row is updated would hand the
// browser an address that stays unauthorized for the milliseconds until the UPDATE lands, so the
// first page load after setting a cover would show a broken image — the exact symptom being fixed.
//
// A failed write is not fatal: the in-memory Resources map has already been extended, so this
// request's token still resolves the address, and the next page load retries the write.
//
// ensureCoverResource 确保封面图片被这篇笔记的分享记录授权。
//
// 这是自愈而不是错误处理，因为另一种症状在用户能看到的任何地方都没有解释：
// /api/share/file 按分享记录的 Resources 授权，而从正文之外挑来的封面从来不在那份清单里。
// 笔记渲染正常、正文每张图都正常，只有封面是 403。
//
// 写库刻意同步。在行更新之前构造 URL，会在 UPDATE 落地前的几毫秒里交给浏览器一个仍未授权的
// 地址，于是设完封面后的第一次页面加载就是破图——正是这里要修的症状。
//
// 写库失败不致命：内存里的 Resources 已经扩过，本次请求的令牌仍能解析该地址，
// 下一次页面加载会重试写入。
func (s *shareService) ensureCoverResource(ctx context.Context, uid int64, sh *domain.UserShare, file *domain.File) bool {
	if sh == nil || file == nil {
		return false
	}
	fileID := strconv.FormatInt(file.ID, 10)
	if util.Inarray(sh.Resources["file"], fileID) {
		return true
	}

	if sh.Resources == nil {
		sh.Resources = make(map[string][]string)
	}
	sh.Resources["file"] = append(sh.Resources["file"], fileID)

	if s.repo != nil {
		if err := s.repo.UpdateResources(ctx, uid, sh.ID, sh.Resources); err != nil && s.logger != nil {
			s.logger.Warn("blog cover: persist share resources failed",
				zap.Int64("shareID", sh.ID),
				zap.Int64("fileID", file.ID),
				zap.Error(err),
			)
		}
	}
	return true
}
