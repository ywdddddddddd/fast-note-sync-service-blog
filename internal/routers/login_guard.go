package routers

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// Per-address rate limiting for the unauthenticated login and register endpoints.
//
// Why this exists next to pkg/limiter instead of replacing it: the existing MethodLimiter keys
// its bucket on c.Request.RequestURI, which is the path with the query stripped — no peer
// address at all. That makes /api/user/login one bucket shared by the whole internet: five
// requests a second from anyone and the operator cannot log in either, while an attacker who
// stays under one request per second is never limited at all. Both directions are wrong, and
// the second one is the one that matters — a slow online guessing run is exactly the attack
// a login limit is supposed to bound.
//
// The existing limiter is left in place. It still does something useful (a hard ceiling on
// total volume per path), and removing it would be a bigger change than this problem needs.
// The two compose: this one decides per caller, that one decides per endpoint.
//
// 针对免认证登录/注册端点的「按地址」限流。
//
// 为什么它与 pkg/limiter 并存而不是取代它：现有 MethodLimiter 用 c.Request.RequestURI 作键，
// 那是去掉查询串的路径，完全不含对端地址。于是 /api/user/login 成了全互联网共享的一个桶：
// 任何人每秒五次，运维自己也登不进去；而攻击者压到每秒一次以下就永远不受限。两个方向都不对，
// 而后一个才是要害——慢速在线猜口令恰恰是登录限流本该约束的攻击。
//
// 现有局限流保留。它仍在做有用的事（对单路径总量设硬上限），拆掉它比解决这个问题本身动静更大。
// 两者叠加：这一层决定「谁在用」，那一层决定「这个端点总共能用多少」。

const (
	// loginPerMinute is what one address may attempt per minute.
	//
	// Ten: a person typing a password gets it right in one or two tries and occasionally
	// mistypes an old password before recalling the new one. Ten leaves room for that and
	// still caps an online guessing run at ten per minute per address, which against any
	// non-trivial password is not a useful attack rate.
	//
	// loginPerMinute 是一个地址每分钟可尝试的次数。
	//
	// 取 10：人输密码一两次就对，偶尔会先试旧密码再想起新的。10 次够这些，同时把在线猜口令
	// 压到每地址每分钟 10 次——对任何不算平凡的密码，这都不是一个有用的攻击速率。
	loginPerMinute = 10
	// loginBurst lets a correct password go through on the first try even if the caller was
	// just rate-limited by someone else behind the same NAT.
	//
	// loginBurst 让正确口令第一次就能过，即便调用方刚被同一 NAT 后的其他人耗掉额度。
	loginBurst = 5

	// loginGuardIdleTTL is how long an idle address keeps its bucket. Same reasoning as
	// blogGuardIdleTTL: without eviction the map grows with every address ever seen.
	//
	// loginGuardIdleTTL 是一个空闲地址保留令牌桶的时长。理由同 blogGuardIdleTTL：
	// 不清理的话表会随「历史上见过的地址数」增长。
	loginGuardIdleTTL = 10 * time.Minute
	// loginGuardSweepEvery throttles eviction to one map walk per interval.
	// loginGuardSweepEvery 把清理限制为每间隔一次全表遍历。
	loginGuardSweepEvery = time.Minute
)

// loginGuardEntry is one address's bucket plus when it was last used.
// loginGuardEntry 是一个地址的令牌桶，外加最后使用时间。
type loginGuardEntry struct {
	lim  *rate.Limiter
	seen time.Time
}

// loginGuard is the per-address limiter for the unauthenticated credential endpoints.
//
// Keyed by the resolver's identity, not by c.ClientIP(): the latter's trust model is gin's,
// and gin trusts every proxy unless told otherwise. Using the same resolver as the blog
// endpoints keeps one definition of "who is calling" in the process.
//
// loginGuard 是免认证凭据端点的按地址限流器。
//
// 键取自 resolver 的 identity 而不是 c.ClientIP()：后者的信任模型是 gin 的，而 gin 在未被
// 告知时信任所有代理。与博客端点用同一个 resolver，使进程内只有一份「谁在调用」的定义。
type loginGuard struct {
	mu        sync.Mutex
	entries   map[string]*loginGuardEntry
	lastSweep time.Time
	now       func() time.Time
}

// loginGuardInstance is process-wide, like blogGuardInstance: the limits describe the server,
// not a request, and a per-test value would need plumbing through every caller.
//
// loginGuardInstance 与 blogGuardInstance 一样是进程级的：限制描述的是服务器而不是某次请求，
// 做成按测试注入的值就要把它穿过每一个调用方。
var loginGuardInstance = newLoginGuard()

func newLoginGuard() *loginGuard {
	return &loginGuard{
		entries: make(map[string]*loginGuardEntry),
		now:     time.Now,
	}
}

// allow reports whether this caller may attempt a credential check now.
//
// Loopback is exempt for the same reason blogGuard exempts it: ops scripts and local
// diagnostics run from 127.0.0.1 and must not be throttled by a limit aimed at the internet.
// That exemption is only sound because the identity can no longer be chosen by the caller —
// see the comment in middleware.Proxy about the removed CF-Connecting-IP rewrite.
//
// allow 返回该调用方此刻是否可以进行一次凭据校验。
//
// 回环豁免的理由与 blogGuard 相同：运维脚本与本地诊断从 127.0.0.1 发起，不该被一条面向
// 互联网的限制拖住。这个豁免成立的前提是身份不再由调用方自选——见 middleware.Proxy 中
// 关于已删除的 CF-Connecting-IP 改写的那段说明。
func (g *loginGuard) allow(key string) bool {
	if isLoopbackAddr(key) {
		return true
	}
	now := g.now()

	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked(now)

	e, ok := g.entries[key]
	if !ok {
		e = &loginGuardEntry{lim: rate.NewLimiter(rate.Limit(float64(loginPerMinute)/60.0), loginBurst)}
		g.entries[key] = e
	}
	e.seen = now
	return e.lim.AllowN(now, 1)
}

// sweepLocked drops idle entries. The caller holds g.mu.
//
// Unlike blogGuard's sweep this has no "busy" exception to honour: a login attempt is a
// single short request, so an entry not touched within the idle window belongs to nobody.
//
// sweepLocked 丢弃空闲条目。调用方持有 g.mu。
//
// 与 blogGuard 的清理不同，这里没有「正在使用」的例外要照顾：一次登录尝试是单个短请求，
// 空闲窗口内没被碰过的条目已经不属于任何人了。
func (g *loginGuard) sweepLocked(now time.Time) {
	if !g.lastSweep.IsZero() && now.Sub(g.lastSweep) < loginGuardSweepEvery {
		return
	}
	g.lastSweep = now
	for k, e := range g.entries {
		if now.Sub(e.seen) > loginGuardIdleTTL {
			delete(g.entries, k)
		}
	}
}

// resetForTest clears accumulated state. Test-only helper.
// resetForTest 清除累计状态。仅供测试。
func (g *loginGuard) resetForTest(now func() time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.entries = make(map[string]*loginGuardEntry)
	g.lastSweep = time.Time{}
	g.now = now
}

// loginKey normalises an identity for use as a map key.
//
// peerHost already strips the port for direct connections, but the resolver can also return
// the proxy's own address form; normalising here means "127.0.0.1" and "127.0.0.1:0" cannot
// become two buckets for the same caller.
//
// loginKey 把身份规范化成可作 map 键的形式。
//
// peerHost 对直连已经去掉了端口，但 resolver 也可能返回代理自己的地址形式；
// 在这里规范化意味着 "127.0.0.1" 与 "127.0.0.1:0" 不会变成同一个调用方的两个桶。
func loginKey(identity string) string {
	if host, _, err := net.SplitHostPort(strings.TrimSpace(identity)); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(strings.TrimSpace(identity))
}

// loginRateLimited wraps a credential handler with the per-address limit.
//
// It returns 429 with the same envelope the rest of the API uses, so a client that already
// handles rate limiting on /api/blog/ask needs no new case. The message names the wait rather
// than the policy: "too many attempts, try again later" is actionable, "rate limited" is not.
//
// The check runs before the handler, so a refused attempt never reaches the password hash —
// which is where the cost lives (bcrypt is deliberately expensive).
//
// loginRateLimited 用按地址的限制包住一个凭据处理器。
//
// 返回 429，信封与 API 其余部分一致，因此已经会处理 /api/blog/ask 限流的客户端不需要新增分支。
// 文案给出的是等待而不是政策：「尝试过多，请稍后再试」可执行，「已限流」不可执行。
//
// 检查在处理器之前运行，因此被拒的尝试永远到不了口令哈希——而成本恰恰在那里（bcrypt 故意很贵）。
func loginRateLimited(handler gin.HandlerFunc, ident *clientIdentityResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !loginGuardInstance.allow(loginKey(ident.identity(c))) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    http.StatusTooManyRequests,
				"message": "尝试次数过多，请稍后再试",
			})
			return
		}
		handler(c)
	}
}
