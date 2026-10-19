package service

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"go.uber.org/zap"
)

// Blog access log.
//
// The blog's visitors are the only audience this service has that is not its owner, so knowing
// what they read is the point of running a public site. That creates an obligation as well as a
// feature, and the design follows from taking it seriously:
//
//   - Nothing identifying is stored. No cookie, no session id, no user id, no referrer, no full
//     user agent. Only what is needed to answer "which articles are read, and roughly when".
//   - The IP is masked at write time, not at read time. A masked value that was never stored
//     cannot leak later; a mask applied on read is one refactor away from being forgotten.
//   - Query strings are stripped rather than filtered. A blog URL has no meaningful parameters,
//     so storing none is strictly safer than trying to decide which ones are safe.
//   - Storage is a bounded append-only file, not a database table. This is high-volume,
//     low-value-density data; it does not belong next to the notes.
//   - Retention is enforced by age and by bytes, so an unattended server cannot fill its disk.
//
// 博客访问日志。
//
// 博客的访客是本服务唯一不是它主人的读者群，因此知道他们读了什么是运行一个公开站点的意义所在。
// 这既带来功能也带来义务，而下面的设计来自认真对待这份义务：
//
//   - 不存任何可识别信息。没有 cookie、没有会话 id、没有用户 id、没有来源页、没有完整 User-Agent。
//     只存回答「哪些文章被读过、大致什么时候」所必需的东西。
//   - IP 在写入时就打码，而不是读取时。从未被存下的值不可能在日后泄露；
//     而读取时才打码的实现，距离被某次重构忘掉只差一步。
//   - 查询串是直接丢弃而不是过滤。博客 URL 没有有意义的参数，所以「一个都不存」
//     比「判断哪些安全」严格更安全。
//   - 存储是有界的追加文件，不是数据库表。这是高写入量、低价值密度的数据，不属于笔记旁边。
//   - 保留期按时间与字节双重约束，使无人看管的服务器不会写满磁盘。

const (
	// blogAccessLogPrefix names the per-day files.
	// blogAccessLogPrefix 是每日文件的前缀。
	blogAccessLogPrefix = "blog-access-"
	// blogAccessLogExt is the file suffix; one JSON object per line.
	// blogAccessLogExt 是文件后缀；每行一个 JSON 对象。
	blogAccessLogExt = ".jsonl"
	// blogAccessLogMaxDays bounds how far back a query may reach.
	// blogAccessLogMaxDays 限定查询最多回溯多少天。
	blogAccessLogMaxDays = 30
	// blogAccessLogMaxLinesPerDay bounds one day's file. At roughly 200 bytes per line this is
	// about 4 MB per day, which is more than a personal blog will produce and small enough that
	// thirty days of it stays trivial.
	//
	// blogAccessLogMaxLinesPerDay 限定单日文件的行数。按每行约 200 字节计，这大约是每天 4 MB，
	// 超过个人博客的产出量，同时使三十天的总量仍然很小。
	blogAccessLogMaxLinesPerDay = 20000
	// blogAccessLogMaxQuery bounds how many records a single query returns.
	// blogAccessLogMaxQuery 限定单次查询返回多少条记录。
	blogAccessLogMaxQuery = 500
)

// blogAccessEntry is one stored visit.
//
// Every field is either a timestamp, a path, or a value that has already been masked. The struct
// is the privacy contract: adding a field here is adding a field to what is retained, which is
// why there is no place to put a full IP or a referrer even if a caller wanted to.
//
// blogAccessEntry 是一次已存储的访问。
//
// 每个字段要么是时间戳、要么是路径、要么是已经打过码的值。这个结构体就是隐私契约：
// 在这里增加字段就是增加被保留的内容，因此即使调用方想存完整 IP 或来源页，也没有地方可放。
type blogAccessEntry struct {
	Time   string `json:"t"`            // UTC, second precision // UTC，秒级
	Method string `json:"m"`            // GET only, in practice // 实际只有 GET
	Path   string `json:"p"`            // blog path, no query string // 博客路径，无查询串
	Status int    `json:"s"`            // HTTP status // HTTP 状态码
	MS     int64  `json:"d"`            // duration in ms // 耗时（毫秒）
	IP     string `json:"ip,omitempty"` // already masked // 已打码
	Client string `json:"c,omitempty"`  // browser family only // 仅浏览器族
	Title  string `json:"ti,omitempty"` // article title when known // 已知时的文章标题
}

// blogAccessLog writes and reads the per-day visit files.
//
// The writer is guarded because gin serves requests concurrently; the reader takes a snapshot of
// the day's lines and does not hold the lock while parsing, so a slow query cannot stall traffic.
//
// blogAccessLog 读写按日划分的访问文件。
//
// 写入端加锁是因为 gin 并发处理请求；读取端先取走当日行的快照，解析期间不持锁，
// 因此慢查询不会拖住流量。
type blogAccessLog struct {
	dir string
	log *zap.Logger

	mu sync.Mutex
	// day is the date the current file belongs to, so the counter can be reset without a stat.
	// day 是当前文件所属日期，使计数器无需 stat 即可重置。
	day   string
	lines int
	// disabled is set once a write failure has been observed, so a read-only or full volume
	// produces one warning rather than one per request.
	//
	// disabled 在观察到一次写入失败后置位，使只读或已满的卷产生一次告警而不是每个请求一次。
	disabled bool
}

// newBlogAccessLog creates the log rooted at the given directory.
// newBlogAccessLog 在给定目录下创建访问日志。
func newBlogAccessLog(dir string, logger *zap.Logger) *blogAccessLog {
	return &blogAccessLog{dir: dir, log: logger}
}

// record appends one visit.
//
// The caller must pass values that have already been masked; this function does not decide what
// is sensitive. That separation is deliberate: the masking rules live next to the middleware that
// owns them, and this file only decides what is retained.
//
// record 追加一次访问。
//
// 调用方必须传入已经打过码的值；本函数不判断什么属于敏感信息。这个分工是有意的：
// 打码规则与拥有它们的中间件放在一起，而本文件只决定保留什么。
func (b *blogAccessLog) record(e blogAccessEntry) {
	if b == nil || b.dir == "" {
		return
	}
	// Blog pages only. The panel is about who read the blog, and storing API traffic here would
	// both bloat the files and turn a reading log into a general request log.
	//
	// 仅博客页面。这个面板关注的是谁读了博客，而把 API 流量存进来既会让文件膨胀，
	// 也会把「阅读日志」变成「通用请求日志」。
	if !strings.HasPrefix(e.Path, "/blog") {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.disabled {
		return
	}

	day := time.Now().UTC().Format("2006-01-02")
	if day != b.day {
		b.day = day
		b.lines = 0
	}
	if b.lines >= blogAccessLogMaxLinesPerDay {
		return
	}

	payload, err := json.Marshal(e)
	if err != nil {
		return
	}

	path := filepath.Join(b.dir, blogAccessLogPrefix+day+blogAccessLogExt)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		b.disable("open", err)
		return
	}
	defer f.Close()

	if _, err := f.Write(append(payload, '\n')); err != nil {
		b.disable("write", err)
		return
	}
	b.lines++
	b.pruneLocked(day)
}

// disable turns logging off after a failure, reporting it once.
// disable 在失败后关闭日志记录，并只报告一次。
func (b *blogAccessLog) disable(op string, err error) {
	b.disabled = true
	if b.log != nil {
		b.log.Warn("blog access log disabled",
			zap.String("op", op), zap.String("error", err.Error()))
	}
}

// pruneLocked removes day files older than the retention window.
//
// It runs after a write rather than on a timer: the only moment a file can become too old is when
// a new day's file has just been created, and a timer would need its own lifecycle.
//
// pruneLocked 删除超出保留期的日期文件。
//
// 它在写入之后运行而不是定时运行：一个文件变旧的唯一时刻就是新一天的文件刚被创建时，
// 而定时器还需要自己的生命周期管理。
func (b *blogAccessLog) pruneLocked(day string) {
	if b.lines != 1 {
		return // only when a new day's file was just started // 仅在新一天的文件刚创建时
	}
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -blogAccessLogMaxDays).Format("2006-01-02")
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, blogAccessLogPrefix) || !strings.HasSuffix(name, blogAccessLogExt) {
			continue
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(name, blogAccessLogPrefix), blogAccessLogExt)
		if stamp < cutoff {
			_ = os.Remove(filepath.Join(b.dir, name))
		}
	}
}

// read returns the most recent entries, newest first.
//
// days is clamped to the retention window; limit is clamped to the per-query cap. Both clamps
// exist so that a request cannot ask the server to read an unbounded amount of file.
//
// read 返回最近的记录，最新的在前。
//
// days 会被限制在保留期内；limit 会被限制在单次查询上限内。两道限制都是为了让请求
// 无法要求服务器读取无边界的文件量。
func (b *blogAccessLog) read(days, limit int) []blogAccessEntry {
	if b == nil || b.dir == "" {
		return nil
	}
	if days < 1 {
		days = 1
	}
	if days > blogAccessLogMaxDays {
		days = blogAccessLogMaxDays
	}
	if limit < 1 {
		limit = 50
	}
	if limit > blogAccessLogMaxQuery {
		limit = blogAccessLogMaxQuery
	}

	now := time.Now().UTC()
	out := make([]blogAccessEntry, 0, limit)

	// Walk back day by day, newest first, and stop as soon as enough records are collected.
	// Walking the filenames is cheaper than reading and sorting every file inside the window.
	//
	// 从最新一天往前逐日遍历，凑够条数就停。遍历文件名比读入并排序窗口内所有文件更省。
	for i := 0; i < days && len(out) < limit; i++ {
		day := now.AddDate(0, 0, -i).Format("2006-01-02")
		path := filepath.Join(b.dir, blogAccessLogPrefix+day+blogAccessLogExt)
		lines := b.snapshot(path)
		// Within a day the file is append-only, so iterating backwards yields newest-first
		// without sorting.
		// 一天之内文件只追加，因此倒序迭代即可得到「最新在前」，无需排序。
		for j := len(lines) - 1; j >= 0 && len(out) < limit; j-- {
			var e blogAccessEntry
			if err := json.Unmarshal(lines[j], &e); err != nil {
				continue // a torn final line must not break the whole query // 截断的末行不应破坏整次查询
			}
			out = append(out, e)
		}
	}
	return out
}

// snapshot reads a file into bounded line slices without holding the write lock.
// snapshot 把文件读成有界的行切片，且不持有写锁。
func (b *blogAccessLog) snapshot(path string) [][]byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// A line is at most a few hundred bytes; the larger buffer only guards against a corrupted
	// file whose lines were never terminated.
	// 一行最多几百字节；更大的缓冲只是为了防备行未被终止的损坏文件。
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lines := make([][]byte, 0, 256)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		// Copy: the scanner reuses its buffer.
		// 复制：扫描器会复用它的缓冲。
		line := make([]byte, len(sc.Bytes()))
		copy(line, sc.Bytes())
		lines = append(lines, line)
	}
	return lines
}

// summary aggregates a set of entries into the counts the panel shows.
//
// The aggregation happens here rather than in the client so that the panel never needs to receive
// every raw visit: totals are what an owner actually reads.
//
// summary 把一组记录聚合为面板展示的计数。
//
// 聚合在这里完成而不是在客户端，使面板永远不需要接收每一条原始访问：主人真正要看的是总计。
func summarizeAccess(entries []blogAccessEntry) dto.BlogAccessSummary {
	s := dto.BlogAccessSummary{
		ByPath: make([]dto.BlogAccessCount, 0, 8),
	}
	if len(entries) == 0 {
		return s
	}

	perPath := map[string]int{}
	perDay := map[string]int{}
	s.Status4xx, s.Status5xx = 0, 0

	for _, e := range entries {
		s.Total++
		perPath[e.Path]++
		if len(e.Time) >= 10 {
			perDay[e.Time[:10]]++
		}
		switch {
		case e.Status >= 500:
			s.Status5xx++
		case e.Status >= 400:
			s.Status4xx++
		}
	}

	for p, n := range perPath {
		s.ByPath = append(s.ByPath, dto.BlogAccessCount{Path: p, Count: n})
	}
	sort.Slice(s.ByPath, func(i, j int) bool {
		if s.ByPath[i].Count != s.ByPath[j].Count {
			return s.ByPath[i].Count > s.ByPath[j].Count
		}
		return s.ByPath[i].Path < s.ByPath[j].Path
	})
	if len(s.ByPath) > 20 {
		s.ByPath = s.ByPath[:20]
	}

	for d, n := range perDay {
		s.ByDay = append(s.ByDay, dto.BlogAccessCount{Path: d, Count: n})
	}
	sort.Slice(s.ByDay, func(i, j int) bool { return s.ByDay[i].Path > s.ByDay[j].Path })
	if len(s.ByDay) > 14 {
		s.ByDay = s.ByDay[:14]
	}

	return s
}

// BlogAccessLogPage is what the management API returns.
//
// BlogAccessLogPage 是管理接口返回的内容。
type BlogAccessLogPage struct {
	Entries   []dto.BlogAccessItem  `json:"entries"`
	Summary   dto.BlogAccessSummary `json:"summary"`
	Days      int                   `json:"days"`
	Retention int                   `json:"retentionDays"`
	// Note is shown verbatim in the panel, so the privacy posture is stated where the data is,
	// not only in documentation.
	//
	// Note 会原样显示在面板里，使隐私声明出现在数据所在之处，而不只是写在文档里。
	Note string `json:"note"`
}

// RecordBlogAccess stores one visit's masked fields.
//
// It exists as a method on the service so that the middleware never needs to know where visit
// records live: the middleware reports what happened, and this decides whether it is worth
// keeping and where it goes.
//
// RecordBlogAccess 存下一次访问中已打码的字段。
//
// 它作为服务上的方法存在，使中间件永远不需要知道访问记录存在哪里：
// 中间件报告发生了什么，而这里决定值不值得留、以及留在哪里。
func (s *shareService) RecordBlogAccess(method, path string, status int, ms int64, ip, client string) {
	if s == nil || s.blogAccess == nil {
		return
	}
	s.blogAccess.record(blogAccessEntry{
		Time:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Method: method,
		Path:   path,
		Status: status,
		MS:     ms,
		IP:     ip,
		Client: client,
	})
}

// GetBlogAccessLog returns a page of visits plus an aggregate summary.
// GetBlogAccessLog 返回一页访问记录以及聚合摘要。
func (s *shareService) GetBlogAccessLog(ctx context.Context, days, limit int) (*BlogAccessLogPage, error) {
	_ = ctx
	page := &BlogAccessLogPage{
		Entries:   []dto.BlogAccessItem{},
		Days:      days,
		Retention: blogAccessLogMaxDays,
		Note: "只记录博客页面的访问。不存储 cookie、会话标识、用户标识、来源页或完整 User-Agent；" +
			"IP 在写入时即已打码，查询串在写入前已丢弃。",
	}
	if s.blogAccess == nil {
		page.Summary = summarizeAccess(nil)
		return page, nil
	}

	entries := s.blogAccess.read(days, limit)
	page.Summary = summarizeAccess(entries)
	for _, e := range entries {
		page.Entries = append(page.Entries, dto.BlogAccessItem{
			Time: e.Time, Method: e.Method, Path: e.Path, Status: e.Status,
			MS: e.MS, IP: e.IP, Client: e.Client,
		})
	}
	return page, nil
}
