package service

import (
	"testing"
)

// TestShareIDFromSlug covers the parser that lines a link target up with the metadata index.
//
// A wrong parse cannot leak anything — resolution against the published set is what authorises a
// preview — but it would silently produce no preview at all, which is the kind of failure nobody
// reports.
//
// TestShareIDFromSlug 覆盖把链接目标与元数据索引对齐的解析器。
//
// 解析错误不会泄露任何东西——真正授权预览的是「能否在已发布集合中找到它」——
// 但它会静默地完全不产生预览，而这是没人会来报的那种故障。
func TestShareIDFromSlug(t *testing.T) {
	cases := []struct {
		in     string
		want   int64
		wantOK bool
		why    string
	}{
		{"1.94-690a504f02", 94, true, "normal slug"},
		{"1.100-b67e702d6f", 100, true, "three-digit sid"},
		{"7.5-abcdef0123", 5, true, "other uid"},
		{"1.94", 94, true, "no signature segment"},
		{"1.94-", 94, true, "empty signature"},

		{"", 0, false, "empty"},
		{"nodot-abcdef0123", 0, false, "no separator"},
		{"1.-abcdef0123", 0, false, "empty sid"},
		{"1.abc-abcdef0123", 0, false, "non-numeric sid"},
		{"1.94x-abcdef0123", 0, false, "trailing junk in sid"},
		{"1.9999999999999-x", 0, false, "absurdly large sid"},
	}
	for _, c := range cases {
		got, ok := shareIDFromSlug(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("shareIDFromSlug(%q) = (%d, %v), want (%d, %v)  [%s]",
				c.in, got, ok, c.want, c.wantOK, c.why)
		}
	}
}

// TestPreviewSlugLimitIsSane pins the cap to a value a single article could plausibly need, so a
// future edit cannot quietly turn one article render into a whole-vault lookup.
//
// TestPreviewSlugLimitIsSane 把上限钉在一个单篇文章可能需要的量级，使未来的修改无法悄悄把
// 一次文章渲染变成全仓库查询。
func TestPreviewSlugLimitIsSane(t *testing.T) {
	if previewSlugLimit < 10 {
		t.Errorf("previewSlugLimit = %d is too small; a normal article links to more targets", previewSlugLimit)
	}
	if previewSlugLimit > 500 {
		t.Errorf("previewSlugLimit = %d is too large; it would defeat the point of capping", previewSlugLimit)
	}
}

// TestGetLinkPreviewsEmptyInput: asking for nothing must not touch the database.
//
// TestGetLinkPreviewsEmptyInput：什么都不问就不应触碰数据库。
func TestGetLinkPreviewsEmptyInput(t *testing.T) {
	// A nil repository set would panic if the function tried to use it, so this also proves the
	// early return happens before any lookup.
	// 若函数尝试使用仓库，nil 仓库会让它 panic，因此这同时证明提前返回发生在任何查询之前。
	s := &shareService{}

	out, err := s.GetLinkPreviews(t.Context(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected no entries, got %d", len(out))
	}

	out, err = s.GetLinkPreviews(t.Context(), 1, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected no entries for an empty slice, got %d", len(out))
	}
}

// TestGetLinkPreviewsIgnoresUnparsableSlugs: links that do not carry a share id can never be
// authorised, so they must be dropped before any lookup rather than resolved by guessing.
//
// TestGetLinkPreviewsIgnoresUnparsableSlugs：不携带分享 id 的链接永远无法被授权，
// 因此必须在任何查询之前丢弃，而不是靠猜测去解析。
func TestGetLinkPreviewsIgnoresUnparsableSlugs(t *testing.T) {
	s := &shareService{}

	out, err := s.GetLinkPreviews(t.Context(), 1, []string{"not-a-slug", "/blog/", "1.abc-x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected no entries for unparsable slugs, got %d", len(out))
	}
}
