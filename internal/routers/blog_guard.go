package routers

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// Abuse control for the two public writes the blog exposes: /api/blog/ask and /blog/react.
//
// Both endpoints are unauthenticated by design — a reader who can read the page may ask about it
// and react to it — so they are the only places where an anonymous visitor can make this server
// spend money or disk. The rules here are deliberately coarse (per-visitor buckets, one
// generation at a time, one vote per day per kind) rather than clever: at this traffic level the
// goal is to stop a script, not to model human behaviour.
//
// The retrieval cap is NOT here. It lives in the service layer, where it can be scoped to the
// 2–3-second retrieval call instead of the whole 10–30-second request; see
// service/blog_ask_guard.go for why that distinction matters.
//
// 针对博客暴露的两个公开写接口的滥用控制：/api/blog/ask 与 /blog/react。
//
// 两者刻意不鉴权——能读到页面的读者就可以就它提问、对它表态——因此它们是匿名访客唯一
// 能让这台服务器花钱或写盘的地方。这里的规则刻意粗放（按访客的令牌桶、同一时刻一次生成、
// 每种表态每天一次）而不是精巧：在这个流量级别，目标是挡住脚本，不是模拟人的行为。
//
// 检索名额不在这里。它在服务层，因为只有在那里才能把闸的作用域限定在 2–3 秒的检索调用上，
// 而不是整个 10–30 秒的请求；这一区分的理由见 service/blog_ask_guard.go。

const (
	// blogAskPerMinute is how many questions one visitor may start per minute.
	//
	// Six is well above what a reader does (the observed pattern is one question, read the
	// answer, ask a follow-up) and well below what a script needs to be useful: an attacker
	// aiming to burn the upstream quota gets six paid retrieval cycles per minute per address
	// instead of unlimited ones, which makes the attack either slow or expensive in proxies.
	//
	// blogAskPerMinute 是一位访客每分钟可以发起的提问数。
	//
	// 6 远高于读者的实际行为（实测模式是问一个、读答案、再追问），又远低于脚本要产生效果所需：
	// 想烧掉上游额度的攻击者，每个地址每分钟只能拿到 6 次付费检索，而不是无限次，
	// 这让攻击要么很慢，要么在代理上很贵。
	blogAskPerMinute = 6
	// blogAskBurst lets a reader fire a couple of quick follow-ups without a refill wait.
	// blogAskBurst 允许读者连发两个追问而无需等待回填。
	blogAskBurst = 2

	// blogReactPerMinute is how many reaction calls one visitor may make per minute.
	//
	// Twenty, because the button is a toggle: misclick, undo, change to the other kind, undo
	// again is four calls in ten seconds and none of it is abuse. The per-day dedup below is
	// what actually stops ballot stuffing; this bucket only stops a tight loop.
	//
	// blogReactPerMinute 是一位访客每分钟可以发起的表态调用数。
	//
	// 取 20，因为按钮是开关式的：点错、撤销、改成另一种、再撤销，十秒内就是四次调用，
	// 而这些都不是滥用。真正挡住刷量的是下面的每日去重；这个桶只挡住紧凑循环。
	blogReactPerMinute = 20
	blogReactBurst     = 20

	// blogGuardIdleTTL is how long an idle visitor keeps its bucket.
	//
	// Without eviction the two maps grow with the number of distinct addresses ever seen, which
	// on a public blog is unbounded. Ten minutes is longer than any plausible visit and short
	// enough that a scan of a few thousand addresses does not become permanent memory.
	//
	// blogGuardIdleTTL 是一位访客空闲多久后丢弃其令牌桶。
	//
	// 如果不清理，两张表就会随「历史上见过的不同地址数」增长，而这在一个公开博客上是无界的。
	// 十分钟比任何一次可信的访问都长，又短到让一次数千地址的扫描不会变成永久内存。
	blogGuardIdleTTL = 10 * time.Minute
	// blogGuardSweepEvery throttles the eviction pass so it costs one map walk per minute at most.
	// blogGuardSweepEvery 限制清理频率，使它的代价最多是每分钟一次全表遍历。
	blogGuardSweepEvery = time.Minute
)

// blogGuardEntry is one visitor's rate bucket, plus (in the ask map only) whether that visitor
// currently has an answer streaming.
//
// busy lives on the same entry as the bucket rather than in a second map because both are
// keyed by the same address and share the same lifetime: one address, one entry, one eviction.
//
// blogGuardEntry 是一位访客的令牌桶，并（仅在 ask 表中）记录该访客当前是否有回答正在流式输出。
//
// busy 与令牌桶放在同一条记录上而不是放进第二张表，因为两者以同一地址为键、寿命也相同：
// 一个地址一条记录、一次清理。
type blogGuardEntry struct {
	lim  *rate.Limiter
	busy bool
	seen time.Time
}

// blogGuard holds the per-visitor state for the blog's public write endpoints.
//
// 博客公开写接口的按访客状态。
type blogGuard struct {
	mu sync.Mutex

	asks   map[string]*blogGuardEntry // ask frequency buckets // 提问频次桶
	reacts map[string]*blogGuardEntry // reaction frequency buckets // 表态频次桶
	gens   map[string]*blogGuardEntry // generation locks (one answer per address) // 生成锁（每地址一次回答）

	// votes dedups one reaction per (day, address, slug, kind). The day is stored with each
	// key and compared on read, so yesterday's keys are dead on arrival and no timer is needed.
	//
	// votes 对每个 (日期, 地址, slug, 类型) 去重一次。日期写在键里并在读取时比对，
	// 因此昨天的键一进来就是过期的，不需要定时器。
	votes map[string]struct{}

	lastSweep time.Time
	now       func() time.Time // injectable for tests // 供测试注入
}

// newBlogGuard returns an empty guard.
// newBlogGuard 返回一个空的守卫。
func newBlogGuard() *blogGuard {
	return &blogGuard{
		asks:   make(map[string]*blogGuardEntry),
		reacts: make(map[string]*blogGuardEntry),
		gens:   make(map[string]*blogGuardEntry),
		votes:  make(map[string]struct{}),
		now:    time.Now,
	}
}

// blogGuardInstance is the process-wide guard.
//
// A package-level instance rather than a field on the service: the state is bound to the HTTP
// edge (client identity, per-connection concurrency), it needs no configuration, and keeping it
// out of the service interface means no mock churn for a limiter that has no business being
// faked in a service test.
//
// blogGuardInstance 是进程级的守卫。
//
// 用包级实例而不是服务上的字段：这份状态绑在 HTTP 边缘（客户端身份、按连接并发），
// 不需要任何配置；把它排除在服务接口之外，也意味着不必为一个本不该在服务测试里被伪造的
// 限流器去改 mock。
var blogGuardInstance = newBlogGuard()

// clientIdentityResolver turns a request into the address the rate limits are keyed on.
//
// Not c.ClientIP(): gin trusts every proxy unless SetTrustedProxies is configured (it is not),
// so ClientIP takes the left-most X-Forwarded-For entry — a header the client writes. Any
// visitor could rotate their own bucket key by sending a different XFF value, which would make
// every limit below decorative.
//
// What is trustworthy: a header written by a proxy we operate, on a connection that provably
// came from that proxy. nginx overwrites X-Real-IP with $remote_addr (it sets, it does not
// append, so the last writer wins and the last writer is nginx). So the rule is: if and only if
// the TCP peer is in the configured trusted-proxy set, X-Real-IP is authoritative; on every
// other connection the peer address is the only fact available and headers are ignored outright.
//
// The trusted set is why this is a type and not a helper: it used to be hard-coded to loopback,
// which was right for "nginx on the same host" and wrong for the deployment this blog is moving
// to — a reverse proxy on the ICP-filing host, one hop away. In that topology every request
// would arrive from the proxy's public address, so a loopback-only rule would key *all visitors*
// on one address: six questions per minute for the whole internet, and one generation lock held
// by whoever asked first. The failure would look like a limit working too well, which is exactly
// the kind of defect that survives a smoke test.
//
// 把请求解析成限流所键的地址。
//
// 不用 c.ClientIP()：未配置 SetTrustedProxies 时 gin 信任所有代理，于是取 X-Forwarded-For
// 最左项——那是客户端自己写的头；换一个值就换掉桶键，下面每条限制都变成装饰。
//
// 可信的东西是「我们自己运维的代理写的头，且连接确实来自那个代理」。nginx 用 $remote_addr
// 覆盖写 X-Real-IP（是写，不是追加，所以最后写入者即 nginx）。规则因此是：当且仅当 TCP 对端
// 落在配置的可信代理集合里，X-Real-IP 才是权威的；其余连接只看对端地址，完全忽略这些头。
//
// 可信集合之所以做成类型而不是一个函数：它原本硬编码为回环，这对「同机 nginx」是对的，
// 对即将迁移到的部署是错的——反代在备案接入机上，隔了一跳。那种拓扑下所有请求都来自代理的
// 公网地址，只信回环的规则会把**全体访客**键到同一个地址上：全站每分钟 6 次提问，
// 一把生成锁被最先提问的人占住。这个缺陷的表现是「限制生效得过于彻底」，
// 恰恰是冒烟测试抓不到的那一类。
type clientIdentityResolver struct {
	ips  []net.IP
	nets []*net.IPNet
}

// newClientIdentityResolver compiles the trusted-proxy list once, at route registration.
//
// An empty list means loopback only — the same default middleware.Proxy applies, so both layers
// of the process agree about who is a proxy. Disagreement here is worse than either choice: the
// middleware would rewrite RemoteAddr from a proxy that this type then refuses to trust.
//
// newClientIdentityResolver 在注册路由时把可信代理列表编译一次。
//
// 空列表表示只信回环——与 middleware.Proxy 的默认值一致，使进程内两层对「谁是代理」判断一致。
// 两层不一致比任选其一都糟：中间件会采纳某个代理重写的 RemoteAddr，而这个类型随后拒绝信任它。
func newClientIdentityResolver(trusted []string) *clientIdentityResolver {
	if len(trusted) == 0 {
		trusted = []string{"127.0.0.1", "::1"}
	}
	r := &clientIdentityResolver{}
	for _, entry := range trusted {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			r.ips = append(r.ips, ip)
			continue
		}
		if _, subnet, err := net.ParseCIDR(entry); err == nil {
			r.nets = append(r.nets, subnet)
		}
	}
	return r
}

// trustsPeer reports whether a TCP peer is one of the configured proxies.
// trustsPeer 判断某个 TCP 对端是否属于配置中的代理。
func (r *clientIdentityResolver) trustsPeer(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, t := range r.ips {
		if t.Equal(ip) {
			return true
		}
	}
	for _, n := range r.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// identity returns the rate-limit key for a request.
// identity 返回该请求的限流键。
func (r *clientIdentityResolver) identity(c *gin.Context) string {
	peer := peerHost(c.Request.RemoteAddr)

	// Not a proxy we run: this is a direct connection, and its address is the only fact.
	// 不是我们运维的代理：这是直连，对端地址是唯一的事实。
	if r == nil || !r.trustsPeer(peer) {
		return peer
	}

	// A proxy we run. Prefer the header nginx sets (overwrites, so it cannot be pre-seeded).
	// 是我们运维的代理。优先用 nginx 设置的那个头（覆盖写，因此无法被预先塞值）。
	if real := strings.TrimSpace(c.GetHeader("X-Real-IP")); real != "" {
		if net.ParseIP(real) != nil {
			return real
		}
	}
	// Fall back to the last XFF entry — the one the nearest proxy appended. Anything further
	// left was supplied by the client and is not evidence.
	// 回退到 XFF 最后一项——最近一跳代理追加的那一项。更靠左的由客户端提供，不是证据。
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		last := strings.TrimSpace(parts[len(parts)-1])
		if net.ParseIP(last) != nil {
			return last
		}
	}
	return peer
}

// peerHost strips the port from RemoteAddr, tolerating a bare address.
// peerHost 去掉 RemoteAddr 的端口，兼容不带端口的写法。
func peerHost(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// isLoopbackAddr reports whether an address is the local machine.
// isLoopbackAddr 判断地址是否为本机回环。
func isLoopbackAddr(host string) bool {
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// entryLocked returns (creating if needed) the entry for key in m. The caller holds g.mu.
// entryLocked 返回 m 中 key 的记录（必要时创建）。调用方持有 g.mu。
func (g *blogGuard) entryLocked(m map[string]*blogGuardEntry, key string, perMinute, burst int, now time.Time) *blogGuardEntry {
	e, ok := m[key]
	if !ok {
		e = &blogGuardEntry{
			lim: rate.NewLimiter(rate.Limit(float64(perMinute)/60.0), burst),
		}
		m[key] = e
	}
	e.seen = now
	return e
}

// sweepLocked drops entries that are idle and not currently streaming. The caller holds g.mu.
//
// Entries with busy set are never dropped, however stale their timestamp looks: a long answer
// can legitimately outlive the idle window, and evicting its lock would let the same visitor
// start a second generation while the first is still paying for itself upstream.
//
// sweepLocked 丢弃空闲且当前未在流式输出的记录。调用方持有 g.mu。
//
// busy 为真的记录绝不会被丢弃，无论其时间戳看起来多旧：一个很长的回答可以合法地超过空闲窗口，
// 而丢掉它的锁会让同一位访客在第一个生成还在为上游付费时再开一个。
func (g *blogGuard) sweepLocked(now time.Time) {
	if now.Sub(g.lastSweep) < blogGuardSweepEvery {
		return
	}
	g.lastSweep = now
	for _, m := range []map[string]*blogGuardEntry{g.asks, g.reacts, g.gens} {
		for k, e := range m {
			if e.busy {
				continue
			}
			if now.Sub(e.seen) > blogGuardIdleTTL {
				delete(m, k)
			}
		}
	}
}

// allowAsk reports whether this visitor may start a question now.
//
// Loopback is exempt: the baseline script (tokengen/ask_baseline.py) fires seven questions in
// a few hundred milliseconds, and a 6/minute bucket would reject the seventh — turning a
// diagnostic tool that proves the retrieval pipeline works into one that reports a rate-limit
// error. The exemption is only reachable from the machine itself.
//
// allowAsk 返回该访客此刻是否可以发起提问。
//
// 回环地址豁免：基线脚本（tokengen/ask_baseline.py）在几百毫秒内发完七个问题，
// 而 6 次/分的桶会拒掉第七个——把「证明检索管线正常」的诊断工具变成一个报限流错误的工具。
// 该豁免只能从本机到达。
func (g *blogGuard) allowAsk(key string) bool {
	if isLoopbackAddr(key) {
		return true
	}
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked(now)
	return g.entryLocked(g.asks, key, blogAskPerMinute, blogAskBurst, now).lim.AllowN(now, 1)
}

// tryLockGeneration reserves this visitor's single generation slot.
//
// Returns a release function on success and nil on refusal. Non-blocking on purpose: a visitor
// with an answer already streaming is told to wait, rather than queued behind their own
// previous question for up to half a minute.
//
// tryLockGeneration 预占该访客唯一的生成名额。
//
// 成功返回释放函数，拒绝时返回 nil。刻意非阻塞：已经有回答在流式输出的访客会被直接告知等待，
// 而不是被排在自己上一个问题后面等半分钟。
func (g *blogGuard) tryLockGeneration(key string) func() {
	if isLoopbackAddr(key) {
		return func() {}
	}
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked(now)
	e := g.entryLocked(g.gens, key, blogAskPerMinute, blogAskBurst, now)
	if e.busy {
		return nil
	}
	e.busy = true
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		e.busy = false
		e.seen = g.now()
	}
}

// reactDecision is the outcome of a reaction gate check.
// reactDecision 是表态闸检查的结果。
type reactDecision int

const (
	reactAllowed reactDecision = iota
	reactTooFrequent
	reactAlreadyVoted
)

// allowReact decides whether a reaction call may proceed.
//
// Two independent rules, in this order: frequency, then one vote per (day, address, slug,
// kind). Frequency first because it is the cheaper check and because a burst of identical
// requests should read as "too frequent", not as "already voted".
//
// Dedup applies only to *adding* a reaction (delta > 0). A retraction must always be accepted:
// the button is a toggle, and if undoing were blocked by the same rule that recorded the vote,
// a reader who misclicks could never take it back — the count would be stuck at a value its
// author no longer asserts, which is worse than a duplicated vote.
//
// allowReact 决定一次表态调用是否可以继续。
//
// 两条独立规则，顺序是：频次、然后每个 (日期, 地址, slug, 类型) 一次。
// 先查频次，因为它更便宜，而且一串完全相同的请求读起来应该是「太频繁」，
// 而不是「已经表过态」。
//
// 去重只作用于「新增表态」（delta > 0）。撤销必须永远被接受：按钮是开关式的，
// 如果撤销被记录这一次表态的同一条规则挡住，点错的读者就再也撤不回来——
// 计数会卡在一个作者已不再主张的数值上，那比重复一次表态更糟。
func (g *blogGuard) allowReact(key, slug, kind string, delta int64) reactDecision {
	if isLoopbackAddr(key) {
		return reactAllowed
	}
	now := g.now()
	day := now.Format("2006-01-02")

	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked(now)

	if !g.entryLocked(g.reacts, key, blogReactPerMinute, blogReactBurst, now).lim.AllowN(now, 1) {
		return reactTooFrequent
	}

	if delta <= 0 {
		return reactAllowed
	}

	// kind is part of the key on purpose: keying by (day, address, slug) alone would let a like
	// and a dislike overwrite each other, so a reader who liked and then changed their mind
	// would be refused the second — the change of mind would look like a duplicate.
	//
	// 类型必须进键：只按 (日期, 地址, slug) 做键会让赞与踩互相覆盖，
	// 于是先赞后改主意的读者会在第二次被拒——那次改主意看起来像重复。
	voteKey := day + "|" + key + "|" + slug + "|" + kind
	if _, seen := g.votes[voteKey]; seen {
		return reactAlreadyVoted
	}
	g.votes[voteKey] = struct{}{}
	return reactAllowed
}

// resetVotesForTest clears accumulated state. Test-only helper.
// resetVotesForTest 清除累计状态。仅供测试。
func (g *blogGuard) resetForTest(now func() time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.asks = make(map[string]*blogGuardEntry)
	g.reacts = make(map[string]*blogGuardEntry)
	g.gens = make(map[string]*blogGuardEntry)
	g.votes = make(map[string]struct{})
	g.lastSweep = time.Time{}
	g.now = now
}
