package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
)

// Blog reactions and the run-total visit counter.
//
// One small JSON file holds everything: a per-note tally of likes and dislikes, and a single
// monotonic visit count for the whole blog. It is a file rather than a database table because the
// shape is "a few hundred counters" — a table would mean a migration, a repository interface and a
// mock, for data whose entire query is "read the whole thing" and whose entire write is "bump two
// numbers and write it back".
//
// The one property a database would have given us for free, and that this has to earn by hand, is
// not losing writes under concurrency. That is what the mutex below is for, and why every mutation
// goes through `update` rather than touching the map directly.
//
// 博客反应（赞/踩）与累计访问计数。
//
// 全部放在一个小 JSON 文件里：每篇笔记的赞数与踩数，以及整个博客的一个只增访问计数。
// 用文件而不是数据库表，是因为它的形状就是「几百个计数器」——建表意味着一次迁移、一个仓储接口
// 和一份 mock，而这些数据的全部查询是「整份读出来」，全部写入是「加两个数再写回去」。
//
// 数据库本可以白送、而这个实现必须自己挣来的一项性质，是并发下不丢写入。
// 那正是下面这把锁的用途，也是每一次修改都必须走 update、而不是直接碰 map 的原因。

const blogReactionFileName = "blog-reactions.json"

// blogReactionStore is the on-disk shape.
//
// Notes is keyed by slug rather than by note id on purpose: a slug is what a reader's browser has,
// so a reaction arriving over HTTP can be matched without resolving the note first. The cost is
// that a re-shared note starts its count over, which is the honest outcome — a new slug is a new
// address.
//
// blogReactionStore 是落盘的结构。
//
// Notes 以 slug 为键而不是笔记 id，是有意的：读者浏览器手里就是 slug，
// 因此一个从 HTTP 到达的反应不必先解析笔记就能对上。代价是重新分享会让计数从头开始，
// 而这是诚实的结果——新地址就是新地址。
type blogReactionStore struct {
	// Visits is the run-total of blog page views. It only ever increases, which is why it is not
	// derived from the access log: that log keeps 30 days, so a number summed from it would fall
	// over time, and a dashboard figure that goes down because a month passed is read as a bug.
	//
	// Visits 是博客页面的累计访问次数。它只增不减，这正是它不取自访问日志的原因：
	// 日志只保留 30 天，从中求和得到的数字会随时间下降，而一个因为「过了一个月」就变小的仪表盘数字，
	// 会被读成故障。
	Visits int64 `json:"visits"`
	// Notes maps slug -> tally.
	// Notes 是 slug -> 计数。
	Notes map[string]*blogReactionTally `json:"notes"`
}

// blogReactionTally is one note's counters.
//
// Dislikes is stored and returned even though it is a number an author may not enjoy seeing: the
// user asked for it to be public, and a dislike count that is collected but hidden is worse — it
// takes the signal and gives nothing back.
//
// blogReactionTally 是单篇笔记的计数。
//
// Dislikes 会被存储并返回，尽管它可能是作者不愿看到的数字：用户要求公开它，
// 而一个「收集了却藏起来」的踩数更糟——它拿走了信号，什么也没给回来。
type blogReactionTally struct {
	Like    int64 `json:"like"`
	Dislike int64 `json:"dislike"`
}

// blogReactionLog owns the reaction file.
//
// The mutex guards both the in-memory copy and the file, and it is held across the write. Writing
// inside the lock costs latency on a reaction, but a reaction is a rare, human-paced event, and the
// alternative — reading the file, releasing the lock, then writing — is how two simultaneous likes
// become one.
//
// blogReactionLog 持有反应文件。
//
// 这把锁同时保护内存副本与文件，并且在写入期间一直持有。在锁内写会让一次反应多等一会儿，
// 但反应是罕见且以人的节奏发生的事件；而另一种做法——读文件、放锁、再写——
// 正是两个同时到达的赞变成一个的原因。
type blogReactionLog struct {
	path string
	log  *zap.Logger

	mu    sync.Mutex
	store blogReactionStore
	// loaded records whether an initial read has been attempted, so a read failure is reported
	// once and does not turn into a warning per reaction.
	//
	// loaded 记录是否已尝试过首次读取，使一次读取失败只被报一次，而不是每次反应都告警。
	loaded bool
	// disabled is set after a write failure so a read-only volume produces one warning rather
	// than one per request.
	//
	// disabled 在写入失败后置位，使只读卷产生一次告警而不是每个请求一次。
	disabled bool
}

// newBlogReactionLog returns a store rooted at dir, or nil when no directory is configured.
//
// A nil store is valid and every method tolerates it: the blog must keep working on a host with no
// writable storage, minus the counters.
//
// newBlogReactionLog 返回以 dir 为根的反应存储；未配置目录时返回 nil。
//
// nil 存储是合法的，且每个方法都能容忍它：在没有可写存储的主机上，博客必须照常工作，只是没有计数。
func newBlogReactionLog(dir string, logger *zap.Logger) *blogReactionLog {
	if dir == "" {
		return nil
	}
	return &blogReactionLog{
		path:  filepath.Join(dir, blogReactionFileName),
		log:   logger,
		store: blogReactionStore{Notes: map[string]*blogReactionTally{}},
	}
}

// loadLocked reads the file into memory once. The caller holds the mutex.
//
// A missing file is not an error: it is the state of a blog nobody has reacted to yet.
//
// loadLocked 把文件读入内存一次。调用方持有锁。
//
// 文件不存在不是错误：那是还没有人反应过的博客的状态。
func (b *blogReactionLog) loadLocked() {
	if b.loaded {
		return
	}
	b.loaded = true
	raw, err := os.ReadFile(b.path)
	if err != nil {
		if !os.IsNotExist(err) && b.log != nil {
			b.log.Warn("blog reactions: read failed, starting empty",
				zap.String("path", b.path), zap.Error(err))
		}
		return
	}
	if err := json.Unmarshal(raw, &b.store); err != nil {
		// A corrupt file is not discarded silently: the counters are the only copy, so it is kept
		// beside the new one and the failure is reported.
		//
		// 损坏的文件不会被静默丢弃：计数是唯一的副本，因此它被保留在新文件旁边，并报告这次失败。
		if b.log != nil {
			b.log.Error("blog reactions: file is not valid JSON, starting empty and keeping a copy",
				zap.String("path", b.path), zap.Error(err))
		}
		_ = os.WriteFile(b.path+".corrupt", raw, 0o644)
	}
	if b.store.Notes == nil {
		b.store.Notes = map[string]*blogReactionTally{}
	}
}

// saveLocked writes the store out atomically. The caller holds the mutex.
//
// It writes to a temporary file and renames, because a crash during a direct write would leave a
// truncated JSON document, and a truncated document is data loss rather than a failed update.
//
// saveLocked 原子地写出存储。调用方持有锁。
//
// 它先写临时文件再改名，因为直接写时若崩溃会留下被截断的 JSON，而被截断的文档是数据丢失，
// 不是一次失败的更新。
func (b *blogReactionLog) saveLocked() error {
	raw, err := json.Marshal(b.store)
	if err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, b.path)
}

// update runs fn against the store under the lock and persists the result.
//
// The error is returned rather than swallowed so a caller can decide; in practice the only caller
// reports it once and carries on, because a blog that cannot count a like should still serve the
// page.
//
// update 在锁内对存储执行 fn 并落盘。
//
// 错误会被返回而不是吞掉，以便调用方决定；实际上唯一的调用方只报告一次然后继续，
// 因为一个数不了赞的博客仍然应该能把页面送出去。
func (b *blogReactionLog) update(fn func(s *blogReactionStore)) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.disabled {
		return nil
	}
	b.loadLocked()
	fn(&b.store)
	if err := b.saveLocked(); err != nil {
		b.disabled = true
		if b.log != nil {
			b.log.Error("blog reactions: write failed, counters are off from now on",
				zap.String("path", b.path), zap.Error(err))
		}
		return err
	}
	return nil
}

// snapshot returns a copy of the current store.
//
// A copy rather than the live value: callers render it, and handing out the map that a concurrent
// update may be mutating is how a read races a write into a panic.
//
// snapshot 返回当前存储的副本。
//
// 用副本而不是活值：调用方会渲染它，而把一张可能正被并发 update 修改的 map 交出去，
// 正是读取与写入竞态成 panic 的方式。
func (b *blogReactionLog) snapshot() blogReactionStore {
	if b == nil {
		return blogReactionStore{Notes: map[string]*blogReactionTally{}}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.loadLocked()
	out := blogReactionStore{Visits: b.store.Visits, Notes: make(map[string]*blogReactionTally, len(b.store.Notes))}
	for k, v := range b.store.Notes {
		if v == nil {
			continue
		}
		cp := *v
		out.Notes[k] = &cp
	}
	return out
}

// addVisit bumps the run-total visit counter.
//
// The blog home counts once per request and each article counts once, so the total roughly tracks
// page views rather than unique readers. It is labelled 访问次数 rather than 阅读量 for that reason:
// it includes refreshes and crawlers, and calling it readers would be a claim the data cannot
// support.
//
// addVisit 累加访问计数。
//
// 首页每次请求计一次，每篇文章计一次，因此这个总数大致对应页面浏览量而不是独立读者。
// 正因如此它标为「访问次数」而不是「阅读量」：它包含刷新与爬虫，
// 称其为读者数是数据支撑不了的说法。
func (b *blogReactionLog) addVisit() {
	_ = b.update(func(s *blogReactionStore) { s.Visits++ })
}

// react applies one reaction and returns the note's new tally.
//
// `delta` is +1 to add a reaction and -1 to take it back, which is how the button's second click
// un-reacts: the client sends the direction it is moving. Rejecting a second like instead would
// mean the button could only ever be pressed once, and a reader who misclicks has no way back.
//
// The counters are clamped at zero so a stray -1 (a stale tab, a replayed request) cannot drive a
// count negative, which would render as "-1 赞".
//
// react 应用一次反应并返回该笔记的新计数。
//
// delta 为 +1 表示新增、-1 表示撤销，这就是按钮第二次点击取消赞的方式：客户端发送它正在移动的方向。
// 若改成「拒绝第二次赞」，按钮就永远只能按一次，而点错的读者无法挽回。
//
// 计数在零处夹住，使一个游离的 -1（过期的标签页、重放的请求）无法把计数压成负数，
// 那会渲染成「-1 赞」。
func (b *blogReactionLog) react(slug, kind string, delta int64) blogReactionTally {
	var out blogReactionTally
	_ = b.update(func(s *blogReactionStore) {
		t := s.Notes[slug]
		if t == nil {
			t = &blogReactionTally{}
			s.Notes[slug] = t
		}
		switch kind {
		case "like":
			t.Like = clampCount(t.Like + delta)
		case "dislike":
			t.Dislike = clampCount(t.Dislike + delta)
		}
		out = *t
	})
	return out
}

// clampCount keeps a counter at or above zero.
// clampCount 让计数保持在零或以上。
func clampCount(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
