package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Feed readers must be recognisable, or the subscription metric reads as zero.
//
// These are the real UA strings as published by the services, not invented ones: the whole point
// of this table is that Feedly's announcement contains neither "bot" nor "crawler" nor any
// browser token, so it fell through every branch and was filed under "Other".
//
// 阅读器必须能被识别，否则订阅指标读作零。
//
// 这些是各服务真实发布的 UA 字符串，不是编造的：这张表的意义正在于 Feedly 的自述里既没有
// "bot" 也没有 "crawler"，也没有任何浏览器标识，于是它从每一条分支里漏下去，被归入 "Other"。
func TestBrowserFamilyRecognisesFeedReaders(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		{"Feedly/1.0 (+http://www.feedly.com/fetcher.html; 4 subscribers; like FeedFetcher-Google)", "Feedly"},
		{"Inoreader/1.0 (+http://www.inoreader.com; 12 subscribers)", "Inoreader"},
		{"NewsBlur Feed Fetcher - 3 subscribers - http://www.newsblur.com", "NewsBlur"},
		{"Miniflux/2.1.4", "Miniflux"},
		{"FreshRSS/1.24.1 (Linux; https://freshrss.org)", "FreshRSS"},
		{"Tiny Tiny RSS/21.11 (http://tt-rss.org/)", "TinyTinyRSS"},
		{"NetNewsWire (RSS Reader; https://netnewswire.com/)", "NetNewsWire"},
		{"Reeder/5.0 (iPhone; iOS 18.0)", "Reeder"},
		{"TheOldReader/1.0", "TheOldReader"},
		{"FeedFetcher-Google; (+http://www.google.com/feedfetcher.html)", "FeedFetcher"},
		{"SimplePie/1.3 (Feed Parser; http://simplepie.org; Allow like Gecko)", "SimplePie"},
		{"Universal Feed Parser/6.0", "FeedParser"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			if got := browserFamily(c.ua); got != c.want {
				t.Errorf("browserFamily(%q) = %q, want %q", c.ua, got, c.want)
			}
		})
	}
}

// The reader branches must not have swallowed the browser branches they were inserted in front
// of: a real browser UA must still resolve to its own family.
//
// 新增的阅读器分支不得吃掉它们插在前面那些浏览器分支：真实浏览器 UA 仍必须归到自己的族。
func TestBrowserFamilyStillRecognisesBrowsers(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36", "Chrome"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", "Safari"},
		{"Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36 Edg/120.0", "Edge"},
		{"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0", "Firefox"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			if got := browserFamily(c.ua); got != c.want {
				t.Errorf("browserFamily(%q) = %q, want %q", c.ua, got, c.want)
			}
		})
	}
}

// Only the literal from=rss survives into the stored path. This is the whole privacy bargain of
// the change: one marker in, everything else still dropped — including the share_token that
// image URLs carry, which is the one parameter that must never reach a log file.
//
// 只有字面量 from=rss 会进入存储的路径。这正是这次改动的全部隐私交易：放行一个标记，
// 其余照旧丢弃——包括图片地址携带的 share_token，那是绝不能进入日志文件的参数。
func TestBlogVisitRecorderKeepsOnlyFromRss(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		url  string
		want string
	}{
		{"marker kept", "/blog/1.2-abcd?from=rss", "/blog/1.2-abcd?from=rss"},
		{"other value dropped", "/blog/1.2-abcd?from=twitter", "/blog/1.2-abcd"},
		{"no query", "/blog/1.2-abcd", "/blog/1.2-abcd"},
		{
			// The image URL is the dangerous one: it carries a live share_token.
			// 图片地址是最危险的那个：它带着一个有效的 share_token。
			name: "share token never stored",
			url:  "/api/share/file?id=1968&share_token=AAAAALAAAAAAAAAAAQAAAABrCSdop1vbgRtM14RAaPWRq7m5Cw",
			want: "/api/share/file",
		},
		{
			name: "marker kept but token dropped",
			url:  "/blog/1.2-abcd?from=rss&share_token=AAAAALAAAAAAAAAAAQAAAABrCSdop1vbgRtM14RAaPWRq7m5Cw",
			want: "/blog/1.2-abcd?from=rss",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotPath string
			r := gin.New()
			r.Use(BlogVisitRecorder(func(method, path string, status int, ms int64, ip, client string) {
				gotPath = path
			}))
			r.GET("/blog/:slug", func(c *gin.Context) { c.Status(http.StatusOK) })
			r.GET("/api/share/file", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, c.url, nil)
			r.ServeHTTP(w, req)

			if gotPath != c.want {
				t.Errorf("记录的路径 = %q, want %q", gotPath, c.want)
			}
		})
	}
}
