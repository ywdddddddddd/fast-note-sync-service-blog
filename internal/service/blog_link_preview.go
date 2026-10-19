package service

import (
	"context"
	"sort"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// Link previews.
//
// A reader hovering an internal link should learn what is on the other side without leaving the
// page. The data is assembled on the server and handed to the page as a small JSON island,
// rather than fetched on hover, for two reasons:
//
//   - Hover is the wrong place to start a network round trip. The card must appear within a
//     frame or two, which is only possible if the data is already there.
//   - The page only needs the notes it actually links to — typically a handful — so the island
//     stays small. Inlining previews for every note in the vault would be the wrong trade, which
//     is why the caller passes the list of targets it found in the rendered body.
//
// 链接预览。
//
// 读者悬停在内部链接上时，应当不必离开当前页面就能知道对面是什么。数据在服务端组装成一小块
// JSON 交给页面，而不是在悬停时才去取，原因有两点：
//
//   - 悬停不是发起网络往返的时机。卡片必须在两三帧内出现，这只有在数据已经在手边时才可能。
//   - 页面只需要它真正链接到的那些笔记——通常屈指可数——因此这块 JSON 保持很小。
//     为仓库中每一篇笔记内联预览是错误的取舍，这正是调用方传入「在已渲染正文中找到的目标」
//     这份清单的原因。

// previewSlugLimit bounds how many targets are looked up in one call. A page linking to hundreds
// of notes would otherwise turn one article render into a full-vault scan.
//
// previewSlugLimit 限制单次调用查询多少个目标。否则一个链接到数百篇笔记的页面会把一次文章渲染
// 变成全仓库扫描。
const previewSlugLimit = 60

// GetLinkPreviews returns preview metadata for the given slugs, skipping anything that is not a
// published note.
//
// Only published notes are returned: a preview is a second way to leak a title, so it follows the
// same rule as the graph and the index. A slug that is unknown simply has no entry, and the
// client then shows no card.
//
// GetLinkPreviews 返回给定 slug 的预览元数据，跳过所有不是已发布笔记的目标。
//
// 只返回已发布笔记：预览是泄露标题的第二条途径，因此它遵循与图谱、索引相同的规则。
// 未知的 slug 干脆没有条目，客户端便不显示卡片。
func (s *shareService) GetLinkPreviews(ctx context.Context, uid int64, slugs []string) (map[string]*dto.BlogPreviewDTO, error) {
	out := make(map[string]*dto.BlogPreviewDTO, len(slugs))
	if len(slugs) == 0 {
		return out, nil
	}

	// Deduplicate and cap. Order is not preserved because the result is a map anyway; sorting is
	// only for determinism of the lookup set.
	//
	// 去重并限流。结果本身是 map，因此不保留顺序；排序只是为了让查询集合是确定的。
	uniq := make(map[string]struct{}, len(slugs))
	for _, s0 := range slugs {
		if s0 != "" {
			uniq[s0] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(uniq))
	for s0 := range uniq {
		ordered = append(ordered, s0)
	}
	sort.Strings(ordered)
	if len(ordered) > previewSlugLimit {
		ordered = ordered[:previewSlugLimit]
	}

	// shareID → slug for the targets, so the index's metadata can be matched back to the slug the
	// page asked about. The index is keyed by note id, and the slug encodes the share id.
	//
	// shareID → slug，用于把索引里的元数据匹配回页面所询问的 slug。
	// 索引以笔记 id 为键，而 slug 编码了分享 id。
	want := make(map[int64]string, len(ordered))
	for _, slug := range ordered {
		sid, ok := shareIDFromSlug(slug)
		if !ok {
			continue
		}
		want[sid] = slug
	}
	if len(want) == 0 {
		return out, nil
	}

	idx, err := s.loadLocalGraphIndex(ctx, uid)
	if err != nil {
		return nil, err
	}

	// Vault names are resolved at most once per vault for this call. A page whose links all live
	// in one vault would otherwise issue one vault query per link.
	//
	// 仓库名在本次调用中每个仓库最多解析一次。否则一个所有链接都在同一仓库的页面会为每条链接
	// 发一次仓库查询。
	vaultNames := map[int64]string{}
	nameFor := func(vaultID int64) string {
		if name, ok := vaultNames[vaultID]; ok {
			return name
		}
		name := vaultLabelFor(s, ctx, uid, vaultID)
		vaultNames[vaultID] = name
		return name
	}

	for noteID, shareID := range idx.published {
		slug, ok := want[shareID]
		if !ok {
			continue
		}
		node := idx.byNoteID[noteID]
		if node == nil {
			continue
		}
		out[slug] = &dto.BlogPreviewDTO{
			Slug:  slug,
			Title: node.Title,
			Path:  node.Path,
			Vault: nameFor(node.VaultID),
		}
	}
	return out, nil
}

// shareIDFromSlug extracts the share id from a blog slug of the form "<uid>.<sid>-<hmac>".
//
// The signature is not verified here: the slug came from a link this service itself rendered, and
// resolving it through the published set is what actually authorises the preview. Parsing is only
// a way to line the target up with the index.
//
// shareIDFromSlug 从形如 "<uid>.<sid>-<hmac>" 的博客 slug 中取出分享 id。
//
// 这里不校验签名：该 slug 来自本服务自己渲染出的链接，而真正决定是否授权预览的是
// 「能否在已发布集合中找到它」。解析只是把目标与索引对齐的手段。
func shareIDFromSlug(slug string) (int64, bool) {
	dot := strings.IndexByte(slug, '.')
	if dot < 0 {
		return 0, false
	}
	rest := slug[dot+1:]
	dash := strings.IndexByte(rest, '-')
	if dash > 0 {
		rest = rest[:dash]
	}
	if rest == "" {
		return 0, false
	}
	n := int64(0)
	for _, r := range rest {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int64(r-'0')
		if n > 1<<40 {
			return 0, false
		}
	}
	return n, true
}

// vaultLabelFor resolves a vault's display name, with the same fallback the index uses.
//
// vaultLabelFor 解析仓库显示名，使用与索引相同的回退值。
func vaultLabelFor(s *shareService, ctx context.Context, uid, vaultID int64) string {
	if s.vaultRepo == nil {
		return ""
	}
	v, err := s.vaultRepo.GetByID(ctx, vaultID, uid)
	if err != nil || v == nil {
		return ""
	}
	return v.Name
}
