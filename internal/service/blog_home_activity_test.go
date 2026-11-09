package service

import (
	"testing"
	"time"
)

// TestBlogNoteDatePrefersCreatedAt pins the single date source the home page attributes a note to.
//
// The bug this guards: the entry cards displayed the share record's CreatedAt (2026-11-08) while
// the hero, day counter and heat map read the file mtime (2026-11-07), so the same note showed two
// different dates on one page. CreatedAt must win; the file time is only a fallback, and a note
// with neither must be skipped rather than silently counted as "today" or 1970.
//
// TestBlogNoteDatePrefersCreatedAt 钉住首页给一篇笔记归属日期的唯一来源。
//
// 它防的是这个 bug：条目卡显示分享记录的 CreatedAt（2026-11-08），而 hero、站龄与热力图读的是
// 文件 mtime（2026-11-07），同一篇笔记在同一页上出现两个日期。CreatedAt 必须优先；
// 文件时间只作回退；两者都没有时必须跳过，而不是悄悄记成「今天」或 1970。
func TestBlogNoteDatePrefersCreatedAt(t *testing.T) {
	mtime := time.Date(2026, 11, 7, 12, 0, 0, 0, time.Local).UnixMilli()
	cases := []struct {
		name      string
		createdAt string
		mtime     int64
		want      string
	}{
		{"createdAt wins", "2026-11-08", mtime, "2026-11-08"},
		{"falls back to mtime", "", mtime, "2026-11-07"},
		{"createdAt without mtime", "2026-11-08", 0, "2026-11-08"},
		{"neither yields empty", "", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := blogNoteDate(c.createdAt, c.mtime); got != c.want {
				t.Fatalf("blogNoteDate(%q, %d) = %q, want %q", c.createdAt, c.mtime, got, c.want)
			}
		})
	}
}

// TestBuildActivityMonthsAlignToColumns pins where each month label sits.
//
// The labels used to be emitted as consecutive grid cells, so a 14-week strip rendered 8/9/10/11
// all crowded in the middle. Each label must now carry the week column that contains its month's
// 1st: with 2026-11-09 (a Monday) as "today" and a 14-week window, the grid starts on
// 2026-08-09 (Sunday), which puts September 1st in week 3 (column 4), October 1st in week 7
// (column 8) and November 1st in week 12 (column 13).
//
// TestBuildActivityMonthsAlignToColumns 钉住每个月份标签的位置。
//
// 过去标签作为连续的网格单元输出，14 周色带里的 8/9/10/11 全挤在中间。现在每个标签必须带上
// 包含当月 1 号的那一周列：以 2026-11-09（周一）为「今天」、窗口 14 周时，网格从
// 2026-08-09（周日）开始，于是 9 月 1 日在第 3 周（列 4），10 月 1 日在第 7 周（列 8），
// 11 月 1 日在第 12 周（列 13）。
func TestBuildActivityMonthsAlignToColumns(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.Local)
	earliest := time.Date(2026, 9, 10, 0, 0, 0, 0, time.Local)
	dayCount := map[string]int{
		"2026-09-10": 73,
		"2026-11-08": 1,
	}

	act := buildActivityAt(dayCount, earliest.UnixMilli(), 14, 53, now)
	if act.Weeks != 14 {
		t.Fatalf("Weeks = %d, want 14", act.Weeks)
	}
	if len(act.Months) != len(act.MonthCols) {
		t.Fatalf("Months has %d labels but MonthCols has %d columns", len(act.Months), len(act.MonthCols))
	}
	if act.NoteDays != 2 {
		t.Fatalf("NoteDays = %d, want 2", act.NoteDays)
	}
	if act.Max != 73 {
		t.Fatalf("Max = %d, want 73", act.Max)
	}

	want := map[string]int{"8月": 1, "9月": 4, "10月": 8, "11月": 13}
	got := make(map[string]int, len(act.Months))
	for i, m := range act.Months {
		got[m] = act.MonthCols[i]
	}
	for m, wantCol := range want {
		if got[m] != wantCol {
			t.Errorf("month %q sits at column %d, want %d (got months %v cols %v)",
				m, got[m], wantCol, act.Months, act.MonthCols)
		}
	}
	if len(act.Months) != len(want) {
		t.Errorf("Months = %v, want exactly %d labels", act.Months, len(want))
	}

	// Columns must advance left to right with no repeats: a duplicate would mean two labels
	// stacked on one week, which is exactly the rendering this change removes.
	// 列号必须从左到右递增且不重复：重复意味着两个标签叠在同一周，正是这次改动要消除的渲染。
	seenCol := map[int]bool{}
	prev := 0
	for i, col := range act.MonthCols {
		if col < 1 || col > act.Weeks {
			t.Fatalf("MonthCols[%d] = %d, want within 1..%d", i, col, act.Weeks)
		}
		if col <= prev {
			t.Fatalf("MonthCols not strictly increasing: %v", act.MonthCols)
		}
		if seenCol[col] {
			t.Fatalf("duplicate month column %d in %v", col, act.MonthCols)
		}
		seenCol[col] = true
		prev = col
	}
}
