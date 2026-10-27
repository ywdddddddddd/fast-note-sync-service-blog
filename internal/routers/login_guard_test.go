package routers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestLoginGuardPerAddress covers the reason this limiter exists.
//
// The defect it was written for: MethodLimiter keys on c.Request.RequestURI, so
// /api/user/login had one bucket for the whole internet. Two consequences, and the second
// is the one that matters — anyone could spend the operator's budget and lock them out for
// a few seconds, and a slow guessing run from a single address was never limited at all.
// So the cases below are: one address is capped, a *different* address is unaffected, and
// loopback is exempt.
//
// TestLoginGuardPerAddress 覆盖这个限流器存在的理由。
//
// 它要修的缺陷是：MethodLimiter 用 c.Request.RequestURI 作键，于是 /api/user/login 成了全互联网
// 共享的一个桶。两个后果，而后者才是要害——任何人都能花掉运维的额度并把他锁在外面几秒，
// 而来自单一地址的慢速猜解完全不受限。因此下面的用例是：一个地址会被压住、
// 另一个地址不受影响、回环豁免。
func TestLoginGuardPerAddress(t *testing.T) {
	g := newLoginGuard()
	now := time.Unix(1_700_000_000, 0)
	g.resetForTest(func() time.Time { return now })

	const alice = "198.51.100.10"
	const bob = "198.51.100.11"

	// Alice spends her burst.
	// Alice 用完她的突发额度。
	for i := 1; i <= loginBurst; i++ {
		if !g.allow(alice) {
			t.Fatalf("第 %d 次应放行（burst=%d）", i, loginBurst)
		}
	}
	if g.allow(alice) {
		t.Fatalf("burst 用尽后的下一次应被拒")
	}

	// Bob must be unaffected: a shared bucket is the bug, not the fix.
	// Bob 必须不受影响：共享一个桶是要修的缺陷，不是解法。
	if !g.allow(bob) {
		t.Fatalf("另一个地址不应受到 alice 的额度影响")
	}

	// Loopback is exempt so ops scripts are not throttled.
	// 回环豁免，以便运维脚本不被拖住。
	for i := 0; i < 100; i++ {
		if !g.allow("127.0.0.1") {
			t.Fatalf("回环应始终放行")
		}
	}

	// After a minute the bucket is full again — but "full" is burst, not loginPerMinute.
	// The rate sets how fast it refills (10/min ⇒ one token every 6 s); the capacity sets how
	// many can sit ready (5). So a recovered caller gets burst attempts, then one per 6 s —
	// which is the intended shape: enough for a person who mistyped, too slow to guess with.
	//
	// 一分钟之后桶重新填满——但「满」是 burst，不是 loginPerMinute。
	// 速率决定回填速度（10/分 ⇒ 每 6 秒一个令牌），容量决定能囤几个（5）。
	// 于是恢复后的调用方先拿到 burst 次，之后每 6 秒一次——这正是想要的形状：
	// 够一个打错字的人用，慢到无法用来猜。
	now = now.Add(time.Minute)
	got := 0
	for i := 0; i < loginPerMinute+5; i++ {
		if g.allow(alice) {
			got++
		}
	}
	if got != loginBurst {
		t.Fatalf("一分钟后应恰好回填到 burst=%d，实测 %d", loginBurst, got)
	}

	// Six seconds is one token at 10/minute.
	// 在 10 次/分下，六秒恰好换回一个令牌。
	now = now.Add(6 * time.Second)
	if !g.allow(alice) {
		t.Fatalf("六秒后应回填一个令牌")
	}
}

// TestLoginKeyNormalises verifies the two spellings of one caller collapse into one bucket.
// TestLoginKeyNormalises 验证同一调用方的两种写法会塌进同一个桶。
func TestLoginKeyNormalises(t *testing.T) {
	cases := []struct{ in, want string }{
		{"127.0.0.1", "127.0.0.1"},
		{"127.0.0.1:0", "127.0.0.1"},
		{"198.51.100.7:44321", "198.51.100.7"},
		{"  198.51.100.7  ", "198.51.100.7"},
		{"[::1]:51000", "::1"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := loginKey(tc.in); got != tc.want {
			t.Fatalf("loginKey(%q) = %q, 期望 %q", tc.in, got, tc.want)
		}
	}
}

// TestLoginRateLimitedRefusesBeforeHandler proves the wrapper stops the request at the gate.
//
// The point is not that it returns 429; it is that the credential handler never runs. bcrypt
// is intentionally expensive, so letting a refused request reach the hash would turn a
// rate limit into the very amplifier it is meant to prevent.
//
// TestLoginRateLimitedRefusesBeforeHandler 证明包装在闸门处就停住了请求。
//
// 要点不是它返回 429，而是凭据处理器根本没有运行。bcrypt 故意很贵，若让被拒的请求走到
// 哈希那一步，限流就变成了它本该防的那种放大器。
func TestLoginRateLimitedRefusesBeforeHandler(t *testing.T) {
	g := loginGuardInstance
	g.resetForTest(func() time.Time { return time.Unix(1_700_000_000, 0) })
	t.Cleanup(func() { g.resetForTest(time.Now) })

	calls := 0
	inner := func(c *gin.Context) { calls++; c.Status(http.StatusOK) }

	// A resolver that trusts loopback only, so the test peer is a real external address.
	// 只信回环的 resolver，因此测试对端是一个真实的外部地址。
	ident := newClientIdentityResolver([]string{"127.0.0.1"})
	h := loginRateLimited(inner, ident)

	do := func() int {
		c := newGuardTestContext("203.0.113.50:40000", nil)
		h(c)
		return c.Writer.Status()
	}

	for i := 1; i <= loginBurst; i++ {
		if code := do(); code != http.StatusOK {
			t.Fatalf("第 %d 次应放行，得到 %d", i, code)
		}
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Fatalf("超出 burst 后应 429，得到 %d", code)
	}
	if calls != loginBurst {
		t.Fatalf("处理器应被调用 %d 次，实际 %d —— 被拒的请求不应到达它", loginBurst, calls)
	}
}

// TestLoginRateLimitedPassesLoopback confirms the exemption the ops scripts rely on.
// TestLoginRateLimitedPassesLoopback 确认运维脚本依赖的回环豁免。
func TestLoginRateLimitedPassesLoopback(t *testing.T) {
	g := loginGuardInstance
	g.resetForTest(func() time.Time { return time.Unix(1_700_000_000, 0) })
	t.Cleanup(func() { g.resetForTest(time.Now) })

	calls := 0
	inner := func(c *gin.Context) { calls++; c.Status(http.StatusOK) }
	ident := newClientIdentityResolver([]string{"127.0.0.1"})
	h := loginRateLimited(inner, ident)

	for i := 0; i < loginBurst*4; i++ {
		c := newGuardTestContext("127.0.0.1:50000", nil)
		h(c)
		if c.Writer.Status() != http.StatusOK {
			t.Fatalf("回环请求应始终放行，第 %d 次得到 %d", i+1, c.Writer.Status())
		}
	}
	if calls != loginBurst*4 {
		t.Fatalf("回环请求应全部到达处理器，期望 %d 次，实际 %d", loginBurst*4, calls)
	}
}
