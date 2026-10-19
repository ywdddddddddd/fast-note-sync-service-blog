package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestBlogReactionAddAndRetract 赞与取消赞。
//
// 按钮的第二次点击是「取消」，所以 −1 必须真的减回去 —— 若只做加法，
// 点错的读者就没有退路，而这是用户明确要求的行为（「再点取消」）。
func TestBlogReactionAddAndRetract(t *testing.T) {
	dir := t.TempDir()
	b := newBlogReactionLog(dir, nil)

	if got := b.react("s1", "like", 1); got.Like != 1 {
		t.Fatalf("after +1 like = %+v, want Like 1", got)
	}
	if got := b.react("s1", "like", 1); got.Like != 2 {
		t.Fatalf("after a second +1 like = %+v, want Like 2", got)
	}
	if got := b.react("s1", "like", -1); got.Like != 1 {
		t.Fatalf("after retract = %+v, want Like 1", got)
	}
	// 踩与赞互不干扰。
	if got := b.react("s1", "dislike", 1); got.Dislike != 1 || got.Like != 1 {
		t.Fatalf("dislike must not touch like: %+v", got)
	}
	// 不同 slug 各自独立。
	if got := b.react("s2", "like", 1); got.Like != 1 {
		t.Fatalf("s2 = %+v, want Like 1", got)
	}
	if got := b.snapshot(); got.Notes["s1"].Like != 1 || got.Notes["s2"].Like != 1 {
		t.Fatalf("slugs must be independent: %+v", got.Notes)
	}
}

// TestBlogReactionClampsAtZero 计数在零处夹住。
//
// 一个游离的 −1（过期的标签页、重放的请求）不能把计数压成负数 —— 那会渲染成「−1 赞」。
func TestBlogReactionClampsAtZero(t *testing.T) {
	b := newBlogReactionLog(t.TempDir(), nil)
	if got := b.react("s1", "like", -1); got.Like != 0 {
		t.Fatalf("a lone -1 produced Like %d, want 0", got.Like)
	}
	b.react("s1", "like", 1)
	b.react("s1", "like", -1)
	if got := b.react("s1", "like", -1); got.Like != 0 {
		t.Fatalf("retract below zero produced Like %d, want 0", got.Like)
	}
}

// TestBlogReactionConcurrentLikesDoNotLoseWrites 并发下不丢写入。
//
// 这是整个文件存在的理由：数据库本来会白送这项性质，而文件实现必须自己挣来。
// 若 update 不是在锁内「读—改—写」一体的，两个同时到达的赞就会变成一个。
func TestBlogReactionConcurrentLikesDoNotLoseWrites(t *testing.T) {
	b := newBlogReactionLog(t.TempDir(), nil)
	const n = 200
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.react("hot", "like", 1)
		}()
	}
	wg.Wait()
	if got := b.snapshot().Notes["hot"]; got == nil || got.Like != n {
		t.Fatalf("lost writes: got %+v, want Like %d", got, n)
	}
}

// TestBlogReactionSurvivesRestart 计数写进了文件。
//
// 这一条同时验证了原子写：如果写的是主文件而不是临时文件加改名，
// 崩溃会留下半截 JSON，而半截 JSON 是数据丢失。
func TestBlogReactionSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	b1 := newBlogReactionLog(dir, nil)
	b1.react("s1", "like", 1)
	b1.react("s1", "dislike", 1)
	b1.addVisit()
	b1.addVisit()
	b1.addVisit()

	fresh := newBlogReactionLog(dir, nil)
	snap := fresh.snapshot()
	if snap.Visits != 3 {
		t.Fatalf("visits = %d, want 3", snap.Visits)
	}
	if got := snap.Notes["s1"]; got == nil || got.Like != 1 || got.Dislike != 1 {
		t.Fatalf("tally did not survive: %+v", got)
	}
	// 临时文件不应残留。
	if _, err := os.Stat(filepath.Join(dir, blogReactionFileName+".tmp")); !os.IsNotExist(err) {
		t.Fatal("the temporary file was left behind")
	}
}

// TestBlogReactionMissingFileIsNotAnError 文件不存在不是错误。
//
// 那是「还没有人反应过」的博客的状态，而每次首次访问都报一次错会把日志淹掉。
func TestBlogReactionMissingFileIsNotAnError(t *testing.T) {
	b := newBlogReactionLog(t.TempDir(), nil)
	snap := b.snapshot()
	if snap.Visits != 0 || len(snap.Notes) != 0 {
		t.Fatalf("expected an empty store, got %+v", snap)
	}
}

// TestBlogReactionCorruptFileIsKept 损坏的文件被保留而不被静默丢弃。
//
// 计数是唯一的副本，因此它被另存为 .corrupt 而不是消失。
func TestBlogReactionCorruptFileIsKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, blogReactionFileName)
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := newBlogReactionLog(dir, nil)
	b.snapshot() // 触发读取

	raw, err := os.ReadFile(path + ".corrupt")
	if err != nil {
		t.Fatalf("the unreadable file was not kept: %v", err)
	}
	if string(raw) != "{ this is not json" {
		t.Fatalf("kept the wrong bytes: %q", raw)
	}
	// 并且仍然可以继续用（从空开始）。
	if got := b.react("s1", "like", 1); got.Like != 1 {
		t.Fatalf("store is unusable after a corrupt file: %+v", got)
	}
}

// TestBlogReactionNilStoreIsUsable nil 存储是合法的。
//
// 未配置存储目录时博客必须照常工作，只是没有计数。
func TestBlogReactionNilStoreIsUsable(t *testing.T) {
	b := newBlogReactionLog("", nil)
	if b != nil {
		t.Fatalf("an empty directory must produce a nil store, got %+v", b)
	}
	// 所有方法都要容忍 nil。
	if got := b.react("s1", "like", 1); got.Like != 0 {
		t.Fatalf("nil store returned %+v", got)
	}
	b.addVisit()
	if snap := b.snapshot(); snap.Visits != 0 || snap.Notes == nil {
		t.Fatalf("nil snapshot = %+v", snap)
	}
}

// TestBlogReactionSnapshotIsACopy 快照是副本而不是活值。
//
// 把一张可能正被并发 update 修改的 map 交出去，正是读取与写入竞态成 panic 的方式。
func TestBlogReactionSnapshotIsACopy(t *testing.T) {
	b := newBlogReactionLog(t.TempDir(), nil)
	b.react("s1", "like", 1)

	snap := b.snapshot()
	snap.Notes["s1"].Like = 999
	snap.Visits = 999
	// 改动快照不得影响存储。
	if got := b.snapshot(); got.Notes["s1"].Like != 1 || got.Visits != 0 {
		t.Fatalf("snapshot aliased the store: %+v", got)
	}
}

// TestBlogReactionFileShape 落盘结构是稳定的。
//
// slug 为键、字段名为 like/dislike/visits —— 一旦改动，已存在的文件就读不回来了。
func TestBlogReactionFileShape(t *testing.T) {
	dir := t.TempDir()
	b := newBlogReactionLog(dir, nil)
	b.react("1.17-4567610782", "like", 1)
	b.addVisit()

	raw, err := os.ReadFile(filepath.Join(dir, blogReactionFileName))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["visits"]; !ok {
		t.Fatal("missing the visits key")
	}
	notes, ok := m["notes"].(map[string]interface{})
	if !ok {
		t.Fatalf("notes is not an object: %T", m["notes"])
	}
	tally, ok := notes["1.17-4567610782"].(map[string]interface{})
	if !ok {
		t.Fatalf("the slug key is missing: %v", notes)
	}
	if _, ok := tally["like"]; !ok {
		t.Fatal("missing the like key")
	}
	// dislike 即使为 0 也要写出，使结构稳定、便于人读。
	if _, ok := tally["dislike"]; !ok {
		t.Fatal("missing the dislike key")
	}
}
