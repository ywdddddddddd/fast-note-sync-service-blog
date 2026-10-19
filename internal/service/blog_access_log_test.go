package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Blog access log tests.
//
// These run against a temporary directory, not the live one, because the point is to pin the
// storage contract: what is written, what is refused, and what a read is allowed to cost.
//
// 博客访问日志测试。
//
// 它们跑在临时目录而不是线上目录上，因为要固定的是存储契约：
// 写了什么、拒了什么、一次读取允许有多大代价。

func newTestAccessLog(t *testing.T) (*blogAccessLog, string) {
	t.Helper()
	dir := t.TempDir()
	return newBlogAccessLog(dir, nil), dir
}

func readLines(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), blogAccessLogPrefix) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(line) != "" {
				out = append(out, line)
			}
		}
	}
	return out
}

// TestAccessLogWritesOnlyBlogPaths is the storage contract: the panel is about blog readers, and
// API traffic must not end up in the visit log.
//
// TestAccessLogWritesOnlyBlogPaths 是存储契约：面板关注博客读者，API 流量不得进入访问日志。
func TestAccessLogWritesOnlyBlogPaths(t *testing.T) {
	l, dir := newTestAccessLog(t)

	l.record(blogAccessEntry{Time: "2026-10-18T10:00:00Z", Method: "GET", Path: "/blog", Status: 200})
	l.record(blogAccessEntry{Time: "2026-10-18T10:00:01Z", Method: "GET", Path: "/blog/1.94-abcdef1234", Status: 200})
	l.record(blogAccessEntry{Time: "2026-10-18T10:00:02Z", Method: "GET", Path: "/blog/rss.xml", Status: 200})

	// Must be refused.
	// 必须被拒绝。
	l.record(blogAccessEntry{Time: "2026-10-18T10:00:03Z", Method: "GET", Path: "/api/notes", Status: 200})
	l.record(blogAccessEntry{Time: "2026-10-18T10:00:04Z", Method: "POST", Path: "/api/blog/config", Status: 200})
	l.record(blogAccessEntry{Time: "2026-10-18T10:00:05Z", Method: "GET", Path: "/static/app.js", Status: 200})

	lines := readLines(t, dir)
	if len(lines) != 3 {
		t.Fatalf("stored %d lines, want 3 (blog pages only)\ngot: %v", len(lines), lines)
	}
	for _, l0 := range lines {
		if !strings.Contains(l0, `"p":"/blog`) {
			t.Errorf("non-blog path was stored: %s", l0)
		}
	}
}

// TestAccessLogReadIsNewestFirstAndBounded: a read must never be able to return an unbounded
// number of records, and it must return the most recent ones.
//
// TestAccessLogReadIsNewestFirstAndBounded：一次读取绝不能返回无界数量的记录，并且必须返回最近的。
func TestAccessLogReadIsNewestFirstAndBounded(t *testing.T) {
	l, _ := newTestAccessLog(t)
	for i := 0; i < 40; i++ {
		l.record(blogAccessEntry{
			Time: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
			Path: "/blog/p" + itoa(int64(i)),
		})
	}

	got := l.read(1, 10)
	if len(got) != 10 {
		t.Fatalf("read returned %d entries, want 10", len(got))
	}
	// The last-written entry must come first.
	// 最后写入的条目必须排在最前。
	if got[0].Path != "/blog/p39" {
		t.Errorf("newest-first violated: first = %q", got[0].Path)
	}
}

// TestAccessLogReadClampsInputs: the clamps are what stop a request from asking the server to
// read an unbounded amount of file.
//
// TestAccessLogReadClampsInputs：这些夹取是阻止请求让服务器读取无边界文件量的关键。
func TestAccessLogReadClampsInputs(t *testing.T) {
	l, _ := newTestAccessLog(t)
	l.record(blogAccessEntry{Time: time.Now().UTC().Format("2006-01-02T15:04:05Z"), Path: "/blog"})

	// Absurd inputs must not panic and must not read outside the window.
	// 荒谬的输入不得 panic，也不得读取窗口之外的内容。
	if got := l.read(-5, -5); len(got) > blogAccessLogMaxQuery {
		t.Errorf("negative inputs produced %d entries", len(got))
	}
	if got := l.read(9999, 9999); len(got) > blogAccessLogMaxQuery {
		t.Errorf("oversized inputs produced %d entries, cap is %d", len(got), blogAccessLogMaxQuery)
	}
}

// TestAccessLogTornLineIsSkipped: a half-written final line (a crash mid-append) must not break
// the whole query.
//
// TestAccessLogTornLineIsSkipped：半写的末行（追加中崩溃）不得破坏整次查询。
func TestAccessLogTornLineIsSkipped(t *testing.T) {
	l, dir := newTestAccessLog(t)
	l.record(blogAccessEntry{Time: "2026-10-18T10:00:00Z", Path: "/blog"})

	// Append a torn line by hand.
	// 手工追加一行截断内容。
	files, _ := os.ReadDir(dir)
	f, err := os.OpenFile(filepath.Join(dir, files[0].Name()), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, _ = f.WriteString(`{"t":"2026-10-18T10:00:01Z","p":"/blog/tor`)
	_ = f.Close()

	got := l.read(1, 10)
	if len(got) != 1 {
		t.Fatalf("read returned %d entries, want 1 (the torn line must be skipped)", len(got))
	}
	if got[0].Path != "/blog" {
		t.Errorf("unexpected entry: %+v", got[0])
	}
}

// TestAccessLogDisabledOnWriteFailure: a host where the file cannot be written must keep serving
// the blog rather than failing every request.
//
// TestAccessLogDisabledOnWriteFailure：在文件写不出的主机上，博客必须继续可用，
// 而不是让每个请求都失败。
func TestAccessLogDisabledOnWriteFailure(t *testing.T) {
	// A path whose parent is a file, so OpenFile must fail.
	// 父路径是一个文件，因此 OpenFile 必然失败。
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	l := newBlogAccessLog(blocker, nil)

	// Must not panic, and must mark itself disabled so the failure is reported once.
	// 不得 panic，并且必须标记为已停用，使该失败只被报告一次。
	l.record(blogAccessEntry{Time: "2026-10-18T10:00:00Z", Path: "/blog"})
	if !l.disabled {
		t.Errorf("a write failure should disable the log")
	}
	l.record(blogAccessEntry{Time: "2026-10-18T10:00:01Z", Path: "/blog"})
	if got := len(readLines(t, dir)); got != 0 {
		t.Errorf("no file should have been created, found %d lines", got)
	}
}

// TestSummarizeAccess covers the aggregate the panel shows.
// TestSummarizeAccess 覆盖面板展示的聚合。
func TestSummarizeAccess(t *testing.T) {
	got := summarizeAccess([]blogAccessEntry{
		{Path: "/blog/a", Status: 200, Time: "2026-10-18T10:00:00Z"},
		{Path: "/blog/a", Status: 200, Time: "2026-10-18T11:00:00Z"},
		{Path: "/blog/b", Status: 404, Time: "2026-10-17T10:00:00Z"},
		{Path: "/blog/c", Status: 500, Time: "2026-10-17T11:00:00Z"},
	})

	if got.Total != 4 {
		t.Errorf("Total = %d, want 4", got.Total)
	}
	if got.Status4xx != 1 || got.Status5xx != 1 {
		t.Errorf("status buckets = (%d, %d), want (1, 1)", got.Status4xx, got.Status5xx)
	}
	if len(got.ByPath) != 3 || got.ByPath[0].Path != "/blog/a" || got.ByPath[0].Count != 2 {
		t.Errorf("ByPath not sorted by count desc: %+v", got.ByPath)
	}
	if len(got.ByDay) != 2 || got.ByDay[0].Path != "2026-10-18" {
		t.Errorf("ByDay not newest-first: %+v", got.ByDay)
	}
}

// TestSummarizeAccessEmpty: an empty log must produce empty slices, not nil, so the panel does not
// have to special-case null.
//
// TestSummarizeAccessEmpty：空日志必须产生空切片而不是 nil，使面板不必为 null 做特例处理。
func TestSummarizeAccessEmpty(t *testing.T) {
	got := summarizeAccess(nil)
	if got.Total != 0 {
		t.Errorf("Total = %d, want 0", got.Total)
	}
	if got.ByPath == nil {
		t.Errorf("ByPath must be an empty slice, not nil")
	}
}

// TestAccessLogNilSafety: the service tolerates a nil log, which is what happens when no log
// directory is configured.
//
// TestAccessLogNilSafety：服务能容忍 nil 日志，这是未配置日志目录时的情形。
func TestAccessLogNilSafety(t *testing.T) {
	var l *blogAccessLog
	l.record(blogAccessEntry{Path: "/blog"}) // must not panic // 不得 panic
	if got := l.read(7, 10); got != nil {
		t.Errorf("nil log returned %d entries", len(got))
	}

	s := &shareService{}
	s.RecordBlogAccess("GET", "/blog", 200, 1, "x", "y") // must not panic // 不得 panic
}

// TestGetBlogAccessLogWithoutLog: the API must answer, not error, when logging is off.
// TestGetBlogAccessLogWithoutLog：日志关闭时接口必须作答而不是报错。
func TestGetBlogAccessLogWithoutLog(t *testing.T) {
	s := &shareService{}
	page, err := s.GetBlogAccessLog(t.Context(), 7, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page == nil {
		t.Fatal("nil page")
	}
	if page.Retention != blogAccessLogMaxDays {
		t.Errorf("Retention = %d, want %d", page.Retention, blogAccessLogMaxDays)
	}
	if !strings.Contains(page.Note, "IP") {
		t.Errorf("the privacy note must state what is and is not stored, got %q", page.Note)
	}
}
