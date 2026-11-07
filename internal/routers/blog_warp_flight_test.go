package routers

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestWarpFlightAssetShape guards the few properties the transformation checklist names.
//
// It cannot prove the animation looks right — that needs a browser — but it can prove the bundle
// still carries the scene, has the new uniform/API, and no longer carries the HUD or the sample's
// debug handle. The byte budget also catches an accidental re-extraction of the wrong script.
//
// TestWarpFlightAssetShape 守住改造检查单里可被机械验证的几条。
//
// 它不能证明动画好看——那需要浏览器——但能证明包体仍带着场景、带上了新的 uniform 与 API，
// 且不再带 HUD 或样例的调试句柄。字节预算还能抓住「抽错了脚本」这类事故。
func TestWarpFlightAssetShape(t *testing.T) {
	if len(blogWarpFlightJS) < 3_000_000 {
		t.Fatalf("animation bundle is only %d bytes; the three.js build may be missing", len(blogWarpFlightJS))
	}
	s := string(blogWarpFlightJS)
	if !strings.HasPrefix(s, "(()=>{") {
		t.Error("bundle must start with the sample's IIFE wrapper")
	}
	if !strings.HasSuffix(strings.TrimSpace(s), "})();") {
		t.Error("bundle must end with the IIFE close")
	}
	for _, want := range []string{
		"window.__WFLIGHT={start:start,skip:skip}",
		"uAvatar:{value:null},uAvatarMix:{value:0}",
		"FRAME+dt*N6*2",
		"warp-flight-stage",
		// A transformed ancestor would anchor the fixed overlay to itself; the bundle must
		// re-parent the overlay to body before it measures the stage.
		//
		// 带 transform 的祖先会把 fixed 遮罩锚到自身；包体必须在测量舞台之前把遮罩挂到 body。
		"document.body.appendChild(ov)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("bundle missing %q", want)
		}
	}
	for _, forbidden := range []string{
		`getElementById("hud")`,
		`getElementById("play")`,
		`getElementById("bar")`,
		`getElementById("frames")`,
		"__WARP",
		"加载飞行器",
	} {
		if strings.Contains(s, forbidden) {
			t.Errorf("bundle still contains removed HUD/debug marker %q", forbidden)
		}
	}
}

// Unlike ask/react there is no local baseline script, so the bucket applies to every address —
// which is also what lets the operator reproduce the 429 with three curls.
//
// TestBlogGuardFriendApply 验证每小时三次的规则，以及回环不豁免。
//
// 与提问/表态不同，本地没有基线脚本，因此桶对所有地址生效——这也让运维者能用三次 curl 复现 429。
func TestBlogGuardFriendApply(t *testing.T) {
	g := newBlogGuard()
	now := time.Unix(1_700_000_000, 0)
	g.resetForTest(func() time.Time { return now })

	const ip = "198.51.100.60"
	for i := 1; i <= blogFriendApplyBurst; i++ {
		if !g.allowFriendApply(ip) {
			t.Fatalf("第 %d 次申请应放行（burst=%d）", i, blogFriendApplyBurst)
		}
	}
	if g.allowFriendApply(ip) {
		t.Fatal("burst 用尽后的第四次申请应被拒")
	}

	// One token every twenty minutes at 3/hour.
	// 3 次/小时意味着每二十分钟回填一个令牌。
	now = now.Add(20 * time.Minute)
	if !g.allowFriendApply(ip) {
		t.Fatal("二十分钟后应回填一个令牌")
	}
	if g.allowFriendApply(ip) {
		t.Fatal("回填的令牌已被用掉，紧接着的申请应被拒")
	}

	// Another address is another bucket.
	// 另一个地址是另一个桶。
	if !g.allowFriendApply("198.51.100.61") {
		t.Fatal("另一地址不应受此桶影响")
	}

	// Loopback is NOT exempt, so the operator can see the limit locally.
	// 回环不豁免，使运维者能在本地看到限制。
	for i := 1; i <= blogFriendApplyBurst; i++ {
		if !g.allowFriendApply("127.0.0.1") {
			t.Fatalf("回环第 %d 次应放行", i)
		}
	}
	if g.allowFriendApply("127.0.0.1") {
		t.Fatal("回环地址用尽额度后也应被拒")
	}
}

// proxyTestContext builds a gin context for the avatar proxy.
// proxyTestContext 构造头像代理的 gin 上下文。
func proxyTestContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c, w
}

// proxyServe runs one proxy request and flushes the deferred status.
//
// Gin's response writer defers WriteHeader until the handler returns; a test that calls serve
// directly has to flush it, or every 404 reads as the recorder's initial 200.
//
// proxyServe 执行一次代理请求并冲刷延迟写出的状态。
//
// gin 的响应写入器把 WriteHeader 推迟到处理器返回；直接调用 serve 的测试必须自己冲刷，
// 否则每个 404 都会读成记录器的初始 200。
func proxyServe(p *blogAvatarProxy, target string) (*gin.Context, *httptest.ResponseRecorder) {
	c, w := proxyTestContext(target)
	p.serve(c)
	c.Writer.WriteHeaderNow()
	return c, w
}

// TestAvatarProxyBadInputIs404 covers every input the proxy must refuse before any network work.
// TestAvatarProxyBadInputIs404 覆盖代理在做任何网络工作前必须拒绝的输入。
func TestAvatarProxyBadInputIs404(t *testing.T) {
	p := newBlogAvatarProxy(http.DefaultClient)
	cases := []string{
		"/blog/fly/avatar",
		"/blog/fly/avatar?u=",
		"/blog/fly/avatar?u=file%3A%2F%2F%2Fetc%2Fpasswd",
		"/blog/fly/avatar?u=javascript%3Aalert(1)",
		"/blog/fly/avatar?u=https%3A%2F%2F",
	}
	for _, target := range cases {
		_, w := proxyServe(p, target)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, w.Code)
		}
	}
}

// TestAvatarProxyServesImageAndCaches covers the happy path and the LRU hit.
//
// The second request must not reach the origin: the point of the cache is that a page whose
// planet texture is fetched once does not turn into a request storm on the friend's host.
//
// TestAvatarProxyServesImageAndCaches 覆盖正常路径与缓存命中。
//
// 第二次请求不得到达源站：缓存的用途就是一次贴图抓取不会变成对友链主机的请求风暴。
func TestAvatarProxyServesImageAndCaches(t *testing.T) {
	var hits int32
	payload := []byte("\x89PNG\r\n\x1a\nfake")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	p := newBlogAvatarProxy(srv.Client())
	target := "/blog/fly/avatar?u=" + url.QueryEscape(srv.URL+"/a.png")
	for i := 0; i < 2; i++ {
		_, w := proxyServe(p, target)
		if w.Code != http.StatusOK {
			t.Fatalf("第 %d 次: status = %d, want 200", i+1, w.Code)
		}
		if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
			t.Fatalf("第 %d 次: content-type = %q", i+1, got)
		}
		if !bytes.Equal(w.Body.Bytes(), payload) {
			t.Fatalf("第 %d 次: body 不匹配", i+1)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("源站被请求 %d 次, want 1（第二次应命中缓存）", got)
	}
}

// TestAvatarProxyRejectsNonImage covers the content-type boundary.
// TestAvatarProxyRejectsNonImage 覆盖 content-type 边界。
func TestAvatarProxyRejectsNonImage(t *testing.T) {
	cases := []string{
		"text/html; charset=utf-8",
		"application/json",
		"image/svg+xml",
		"",
	}
	for _, contentType := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			_, _ = io.WriteString(w, "whatever")
		}))
		p := newBlogAvatarProxy(srv.Client())
		_, w := proxyServe(p, "/blog/fly/avatar?u="+url.QueryEscape(srv.URL+"/a"))
		srv.Close()
		if w.Code != http.StatusNotFound {
			t.Errorf("content-type %q: status = %d, want 404", contentType, w.Code)
		}
	}
}

// TestAvatarProxyRejectsOversize covers the 5 MB ceiling.
// TestAvatarProxyRejectsOversize 覆盖 5 MB 上限。
func TestAvatarProxyRejectsOversize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(bytes.Repeat([]byte("x"), blogFlyAvatarMax+1))
	}))
	defer srv.Close()

	p := newBlogAvatarProxy(srv.Client())
	_, w := proxyServe(p, "/blog/fly/avatar?u="+url.QueryEscape(srv.URL+"/big.png"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// TestAvatarProxyLRUEvicts verifies the cache keeps at most 32 entries and drops the oldest.
// TestAvatarProxyLRUEvicts 验证缓存最多保留 32 条并淘汰最旧的。
func TestAvatarProxyLRUEvicts(t *testing.T) {
	p := newBlogAvatarProxy(http.DefaultClient)
	for i := 0; i < blogFlyAvatarCacheN+1; i++ {
		p.put(string(rune('a'+i)), "image/png", []byte{byte(i)})
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.order.Len() != blogFlyAvatarCacheN {
		t.Fatalf("order len = %d, want %d", p.order.Len(), blogFlyAvatarCacheN)
	}
	if _, ok := p.items["a"]; ok {
		t.Fatal("最旧的条目应被淘汰")
	}
	if _, ok := p.items["b"]; !ok {
		t.Fatal("第二旧的条目应保留")
	}
}

// TestBlogImageContentType pins the accepted and refused media types.
// TestBlogImageContentType 固定接受与拒绝的媒体类型。
func TestBlogImageContentType(t *testing.T) {
	cases := map[string]string{
		"image/png":                "image/png",
		"image/jpeg; charset=x":    "image/jpeg",
		"IMAGE/WEBP":               "image/webp",
		"image/svg+xml":            "",
		"text/html":                "",
		"application/octet-stream": "",
		"":                         "",
	}
	for raw, want := range cases {
		if got := blogImageContentType(raw); got != want {
			t.Errorf("blogImageContentType(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestBlogPublicIP covers the addresses the proxy dialer must refuse.
// TestBlogPublicIP 覆盖代理拨号器必须拒绝的地址。
func TestBlogPublicIP(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1":       false,
		"::1":             false,
		"10.1.2.3":        false,
		"192.168.1.10":    false,
		"172.16.0.1":      false,
		"169.254.1.1":     false,
		"0.0.0.0":         false,
		"1.1.1.1":         true,
		"8.8.8.8":         true,
		"2606:4700::1111": true,
	}
	for raw, want := range cases {
		ip := netParseIP(raw)
		if got := blogPublicIP(ip); got != want {
			t.Errorf("blogPublicIP(%s) = %v, want %v", raw, got, want)
		}
	}
}

// TestBlogPublicOnlyDialContextRejectsLoopback verifies the guard is actually wired into dialling,
// not just available as a helper.
//
// TestBlogPublicOnlyDialContextRejectsLoopback 验证防护真的接在拨号上，而不只是一个可用的辅助函数。
func TestBlogPublicOnlyDialContextRejectsLoopback(t *testing.T) {
	_, err := blogPublicOnlyDialContext(t.Context(), "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("回环地址应被拨号器拒绝")
	}
}

// netParseIP is a tiny alias so the test table reads as addresses, not as conversions.
// netParseIP 是一个小别名，让用例表读起来是地址而不是转换调用。
func netParseIP(raw string) net.IP {
	return net.ParseIP(raw)
}
