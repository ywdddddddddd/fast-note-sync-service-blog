package routers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// newGuardTestContext builds a gin context with a chosen TCP peer and headers.
// newGuardTestContext 构造一个带指定 TCP 对端与请求头的 gin 上下文。
func newGuardTestContext(remoteAddr string, headers map[string]string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/blog/ask", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	return c
}

// TestBlogClientIdentity covers the trust decision the rate limits rest on.
//
// A wrong answer here is not cosmetic: if the identity could be chosen by the client, every
// limit below would be bypassable by rotating one header value. So the cases that matter are
// the adversarial ones — a foreign X-Forwarded-For sent through nginx must be ignored, and a
// direct connection must not be able to impersonate anyone by header at all.
//
// TestBlogClientIdentity 覆盖限流所依赖的信任判断。
//
// 这里答错不是外观问题：如果身份可以由客户端选择，下面每一条限制都能靠变换一个头的值绕过。
// 因此真正要紧的是那些对抗性用例——经 nginx 传来的伪造 X-Forwarded-For 必须被忽略，
// 直连也必须完全无法靠请求头冒充任何人。
func TestBlogClientIdentity(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{
			name:       "直连：只看对端，忽略伪造头",
			remoteAddr: "203.0.113.9:44321",
			headers:    map[string]string{"X-Real-IP": "1.2.3.4", "X-Forwarded-For": "5.6.7.8"},
			want:       "203.0.113.9",
		},
		{
			name:       "经 nginx：用 X-Real-IP（被覆盖写）",
			remoteAddr: "127.0.0.1:51000",
			headers:    map[string]string{"X-Real-IP": "111.47.84.40", "X-Forwarded-For": "1.1.1.1, 111.47.84.40"},
			want:       "111.47.84.40",
		},
		{
			name:       "经 nginx 但只有 XFF：取最后一项（代理追加的）",
			remoteAddr: "127.0.0.1:51001",
			headers:    map[string]string{"X-Forwarded-For": "9.9.9.9, 111.47.84.40"},
			want:       "111.47.84.40",
		},
		{
			name:       "本机直连、无任何头：就是回环（诊断脚本豁免的依据）",
			remoteAddr: "127.0.0.1:51002",
			headers:    nil,
			want:       "127.0.0.1",
		},
		{
			name:       "IPv6 回环经代理",
			remoteAddr: "[::1]:51003",
			headers:    map[string]string{"X-Real-IP": "111.47.84.40"},
			want:       "111.47.84.40",
		},
		{
			name:       "X-Real-IP 不是 IP：退回 XFF 最后一项",
			remoteAddr: "127.0.0.1:51004",
			headers:    map[string]string{"X-Real-IP": "not-an-ip", "X-Forwarded-For": "111.47.84.40"},
			want:       "111.47.84.40",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newGuardTestContext(tc.remoteAddr, tc.headers)
			if got := blogClientIdentity(c); got != tc.want {
				t.Fatalf("blogClientIdentity = %q, 期望 %q", got, tc.want)
			}
		})
	}
}

// TestBlogGuardAskBucket verifies the frequency rule and the loopback exemption.
//
// The numbers are the decided ones: 6 per minute with a burst of 2. A burst of 2 means the
// third rapid request is refused rather than the seventh — the bucket refills at one token per
// ten seconds, so six questions in a minute works out to two immediate and then one per ten
// seconds. In practice the per-address generation lock already serialises one visitor (an
// answer takes 10–30 s), so this bucket only ever bites a script.
//
// TestBlogGuardAskBucket 验证频次规则与回环豁免。
//
// 数值是已定的：6 次/分、burst 2。burst 2 意味着被拒的是第三个快速请求而不是第七个——桶以
// 每十秒一个令牌回填，所以「一分钟六个」实际表现为先放两个、之后每十秒一个。
// 实践中按地址的生成锁已经把同一位访客串行化了（一个回答要 10–30 秒），
// 因此这个桶真正咬到的只有脚本。
func TestBlogGuardAskBucket(t *testing.T) {
	g := newBlogGuard()
	now := time.Unix(1_700_000_000, 0)
	g.resetForTest(func() time.Time { return now })

	const ip = "198.51.100.7"
	for i := 1; i <= blogAskBurst; i++ {
		if !g.allowAsk(ip) {
			t.Fatalf("第 %d 次请求应放行（burst=%d）", i, blogAskBurst)
		}
	}
	if g.allowAsk(ip) {
		t.Fatalf("burst 用尽后的下一次请求应被拒")
	}

	// Ten seconds buys exactly one more token at 6/minute.
	// 在 6 次/分下，十秒恰好换回一个令牌。
	now = now.Add(10 * time.Second)
	if !g.allowAsk(ip) {
		t.Fatalf("十秒后应回填一个令牌")
	}
	if g.allowAsk(ip) {
		t.Fatalf("回填的令牌已被用掉，紧接着的请求应被拒")
	}

	// Loopback is exempt so the retrieval baseline script (7 questions, sub-second) still runs.
	// 回环地址豁免，使检索基线脚本（7 个问题、亚秒级）仍能跑完。
	for i := 0; i < 50; i++ {
		if !g.allowAsk("127.0.0.1") {
			t.Fatalf("回环地址应豁免频次限制，第 %d 次被拒", i+1)
		}
	}
}

// TestBlogGuardGenerationLock verifies one in-flight generation per address.
//
// The lock must be exclusive while held, must be reacquirable the instant it is released (the
// browser's next question arrives immediately after `done`), and must not be shared between
// addresses.
//
// TestBlogGuardGenerationLock 验证每地址同时只有一次生成。
//
// 锁在持有时必须互斥，在释放的瞬间必须可再取（浏览器的下一个问题紧随 done 事件到达），
// 且不得在不同地址之间共享。
func TestBlogGuardGenerationLock(t *testing.T) {
	g := newBlogGuard()
	g.resetForTest(func() time.Time { return time.Unix(1_700_000_000, 0) })

	const ip = "198.51.100.20"
	release := g.tryLockGeneration(ip)
	if release == nil {
		t.Fatal("首次生成应拿到锁")
	}
	if second := g.tryLockGeneration(ip); second != nil {
		t.Fatal("同一地址的第二次生成应被拒")
	}
	if other := g.tryLockGeneration("198.51.100.21"); other == nil {
		t.Fatal("另一地址的生成不应受此锁影响")
	} else {
		other()
	}

	release()
	if again := g.tryLockGeneration(ip); again == nil {
		t.Fatal("释放后应能再次拿到锁")
	} else {
		again()
	}

	// Loopback is exempt: the baseline script fires seven questions in parallel.
	// 回环地址豁免：基线脚本会并行发七个问题。
	first := g.tryLockGeneration("127.0.0.1")
	if first == nil {
		t.Fatal("回环地址应豁免生成锁")
	}
	if second := g.tryLockGeneration("127.0.0.1"); second == nil {
		t.Fatal("回环地址应可并发")
	} else {
		second()
	}
	first()
}

// TestBlogGuardReactRules verifies frequency, dedup, the kind dimension and retraction.
//
// Four properties, each of which has a failure mode worth naming. Dedup must key on kind, or a
// like followed by a dislike overwrites the record and the second is refused. Retraction
// (delta < 0) must always pass, or a misclick is permanent. Frequency must be checked before
// dedup, so a tight loop reads as "too frequent" rather than "already voted". And a vote must
// expire with the date, so a reader can react again tomorrow.
//
// TestBlogGuardReactRules 验证频次、去重、类型维度与撤销。
//
// 四条性质，每一条的失效方式都值得点名。去重必须以类型为键，否则先赞后踩会覆盖记录、
// 第二次被拒。撤销（delta < 0）必须永远放行，否则一次误点就是永久的。频次必须先于去重检查，
// 使紧凑循环读起来是「太频繁」而不是「已表过态」。表态还必须随日期过期，使读者明天能再表态。
func TestBlogGuardReactRules(t *testing.T) {
	g := newBlogGuard()
	now := time.Unix(1_700_000_000, 0) // 2023-11-15 (UTC)
	g.resetForTest(func() time.Time { return now })

	const ip, slug = "198.51.100.30", "1.136-98a7a75922"

	if d := g.allowReact(ip, slug, "like", 1); d != reactAllowed {
		t.Fatalf("首次点赞应放行，实际 %v", d)
	}
	if d := g.allowReact(ip, slug, "like", 1); d != reactAlreadyVoted {
		t.Fatalf("同日同类型重复点赞应被去重，实际 %v", d)
	}
	if d := g.allowReact(ip, slug, "dislike", 1); d != reactAllowed {
		t.Fatalf("换成点踩应放行（类型必须参与去重键），实际 %v", d)
	}
	if d := g.allowReact(ip, slug, "like", -1); d != reactAllowed {
		t.Fatalf("撤销点赞必须永远放行，实际 %v", d)
	}
	if d := g.allowReact(ip, slug, "like", -1); d != reactAllowed {
		t.Fatalf("重复撤销也应放行（幂等），实际 %v", d)
	}

	// A different note is a different vote.
	// 另一篇笔记是另一次表态。
	if d := g.allowReact(ip, "1.137-aaaaaaaaaa", "like", 1); d != reactAllowed {
		t.Fatalf("另一篇笔记的点赞应放行，实际 %v", d)
	}

	// Tomorrow the same vote is allowed again.
	// 到了明天，同一次表态可以重来。
	now = now.Add(24 * time.Hour)
	if d := g.allowReact(ip, slug, "like", 1); d != reactAllowed {
		t.Fatalf("次日同类型表态应放行，实际 %v", d)
	}

	// Frequency: a tight loop exhausts the bucket before dedup notices.
	// 频次：紧凑循环会先用尽令牌桶。
	const flooder = "198.51.100.31"
	for i := 0; i < blogReactBurst; i++ {
		if d := g.allowReact(flooder, slug, "like", -1); d != reactAllowed {
			t.Fatalf("第 %d 次（burst 内）应放行，实际 %v", i+1, d)
		}
	}
	if d := g.allowReact(flooder, slug, "like", -1); d != reactTooFrequent {
		t.Fatalf("burst 用尽后应判为太频繁，实际 %v", d)
	}

	// Loopback is exempt so the operator can verify behaviour by hand.
	// 回环地址豁免，使运维者能手工验证行为。
	for i := 0; i < 100; i++ {
		if d := g.allowReact("127.0.0.1", slug, "like", 1); d != reactAllowed {
			t.Fatalf("回环地址应豁免表态限制，第 %d 次被拒", i+1)
		}
	}
}

// TestBlogGuardSweepKeepsBusyEntries verifies eviction cannot release a live generation lock.
//
// The idle window is ten minutes, but an answer's slot is held for as long as the answer
// streams. If the sweeper ignored `busy`, a slow upstream could outlive the window and have its
// lock deleted underneath it, letting the same address start a second paid generation.
//
// TestBlogGuardSweepKeepsBusyEntries 验证清理不会释放仍在使用的生成锁。
//
// 空闲窗口是十分钟，而回答的名额要持有到回答输出完毕。若清理忽略 busy，
// 一条缓慢的上游就可能活过这个窗口，其锁被从底下删掉，从而让同一地址开出第二个付费生成。
func TestBlogGuardSweepKeepsBusyEntries(t *testing.T) {
	g := newBlogGuard()
	now := time.Unix(1_700_000_000, 0)
	g.resetForTest(func() time.Time { return now })

	const busyIP, idleIP = "198.51.100.40", "198.51.100.41"
	release := g.tryLockGeneration(busyIP)
	if release == nil {
		t.Fatal("应拿到生成锁")
	}
	if !g.allowAsk(idleIP) {
		t.Fatal("应能记录空闲地址")
	}

	// Jump past the idle window and force a sweep by touching the guard again.
	// 跳过空闲窗口，并通过再次访问来触发清理。
	now = now.Add(blogGuardIdleTTL + time.Minute)
	g.allowAsk("198.51.100.42")

	g.mu.Lock()
	_, busyKept := g.gens[busyIP]
	_, askEvicted := g.asks[idleIP]
	g.mu.Unlock()

	if !busyKept {
		t.Fatal("正在生成中的地址被清理了，锁会被从底下释放")
	}
	if askEvicted {
		t.Fatal("空闲地址的令牌桶没有被清理，内存会随地址数无界增长")
	}
	release()
}
