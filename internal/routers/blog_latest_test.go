package routers

import (
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

func latestEntry(slug, date string) *dto.BlogEntryDTO {
	return &dto.BlogEntryDTO{Slug: slug, CreatedAt: date}
}

func latestSlugs(entries []*dto.BlogEntryDTO) []string {
	slugs := make([]string, len(entries))
	for i, e := range entries {
		slugs[i] = e.Slug
	}
	return slugs
}

func assertLatestSlugs(t *testing.T, got []*dto.BlogEntryDTO, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("条数 = %d，期望 %d（%v）", len(got), len(want), latestSlugs(got))
	}
	for i := range want {
		if got[i].Slug != want[i] {
			t.Fatalf("第 %d 条 = %q，期望 %q（%v）", i, got[i].Slug, want[i], latestSlugs(got))
		}
	}
}

// TestBlogLatestEntriesCutsAndSorts 锁住「最近更新」树的取数规则：
// 跳过空日期、按日期字符串降序、稳定排序、最多 limit 条。
func TestBlogLatestEntriesCutsAndSorts(t *testing.T) {
	t.Run("nil 输入", func(t *testing.T) {
		if got := blogLatestEntries(nil, 3); got != nil {
			t.Fatalf("期望 nil，得到 %v", latestSlugs(got))
		}
	})

	t.Run("空切片", func(t *testing.T) {
		if got := blogLatestEntries([]*dto.BlogEntryDTO{}, 3); got != nil {
			t.Fatalf("期望 nil，得到 %v", latestSlugs(got))
		}
	})

	t.Run("全部为空日期", func(t *testing.T) {
		got := blogLatestEntries([]*dto.BlogEntryDTO{
			latestEntry("a", ""),
			nil,
			latestEntry("b", ""),
		}, 3)
		if got != nil {
			t.Fatalf("期望 nil，得到 %v", latestSlugs(got))
		}
	})

	t.Run("少于三条", func(t *testing.T) {
		got := blogLatestEntries([]*dto.BlogEntryDTO{
			latestEntry("old", "2026-10-01"),
			latestEntry("new", "2026-11-07"),
		}, 3)
		assertLatestSlugs(t, got, []string{"new", "old"})
	})

	t.Run("正好三条", func(t *testing.T) {
		got := blogLatestEntries([]*dto.BlogEntryDTO{
			latestEntry("mid", "2026-10-15"),
			latestEntry("old", "2026-10-01"),
			latestEntry("new", "2026-11-07"),
		}, 3)
		assertLatestSlugs(t, got, []string{"new", "mid", "old"})
	})

	t.Run("多于三条只取最新三条", func(t *testing.T) {
		got := blogLatestEntries([]*dto.BlogEntryDTO{
			latestEntry("2026-10-01", "2026-10-01"),
			latestEntry("2026-11-07", "2026-11-07"),
			latestEntry("2026-09-01", "2026-09-01"),
			latestEntry("2026-10-20", "2026-10-20"),
			latestEntry("2026-08-01", "2026-08-01"),
		}, 3)
		assertLatestSlugs(t, got, []string{"2026-11-07", "2026-10-20", "2026-10-01"})
	})

	t.Run("跳过 nil 与空日期", func(t *testing.T) {
		got := blogLatestEntries([]*dto.BlogEntryDTO{
			nil,
			latestEntry("valid-old", "2026-09-01"),
			latestEntry("no-date", ""),
			nil,
			latestEntry("valid-new", "2026-11-07"),
		}, 3)
		assertLatestSlugs(t, got, []string{"valid-new", "valid-old"})
	})

	t.Run("相同日期保持输入顺序", func(t *testing.T) {
		got := blogLatestEntries([]*dto.BlogEntryDTO{
			latestEntry("first", "2026-11-07"),
			latestEntry("second", "2026-11-07"),
			latestEntry("third", "2026-11-07"),
			latestEntry("older", "2026-10-01"),
		}, 3)
		assertLatestSlugs(t, got, []string{"first", "second", "third"})
	})
}

// TestBlogLatestEntriesRejectsNonPositiveLimit limit <= 0 是调用方错误，
// 返回 nil 比返回「全部」更安全：模板不会因此渲染出没有上限的列表。
func TestBlogLatestEntriesRejectsNonPositiveLimit(t *testing.T) {
	entries := []*dto.BlogEntryDTO{
		latestEntry("a", "2026-11-07"),
		latestEntry("b", "2026-10-01"),
	}
	for _, limit := range []int{0, -1} {
		if got := blogLatestEntries(entries, limit); got != nil {
			t.Fatalf("limit=%d 时期望 nil，得到 %v", limit, latestSlugs(got))
		}
	}
}
