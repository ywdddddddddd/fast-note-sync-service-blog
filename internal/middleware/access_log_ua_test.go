package middleware

import "testing"

// TestBrowserFamily pins the reduction that keeps the visit log's stated privacy posture true.
//
// Measured before the fix: the visit file held a full User-Agent string
// ("Mozilla/5.0 (Windows NT 10.0; Win64; x64) ... Chrome/120.0 ...") while the panel told the
// reader only a browser family was kept. The order of the cases below is the part that is easy to
// get wrong: every Chrome UA contains "Safari", and every Edge UA contains "Chrome".
//
// TestBrowserFamily 固定这个归约，它使访问日志声明的隐私姿态成立。
//
// 修复前实测：访问文件里存着完整 User-Agent 字符串，而面板告诉读者只保留了浏览器族。
// 下面分支的顺序正是最容易写错的地方：每个 Chrome 的 UA 都含 "Safari"，每个 Edge 的 UA 都含 "Chrome"。
func TestBrowserFamily(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		// Real user agents, including the two ordering traps.
		// 真实 User-Agent，包含两个顺序陷阱。
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", "Chrome"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0", "Edge"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", "Safari"},
		{"Mozilla/5.0 (Windows NT 10.0; rv:121.0) Gecko/20100101 Firefox/121.0", "Firefox"},
		{"Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36 OPR/106.0", "Opera"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0 Mobile/15E148 Safari/604.1", "Chrome"},

		// Non-browser clients, which is what a self-hosted blog mostly sees besides readers.
		// 非浏览器客户端，这是自建博客除读者之外主要会看到的。
		{"curl/8.21.0", "curl"},
		{"Wget/1.21.4", "wget"},
		{"python-requests/2.31.0", "Python"},
		{"okhttp/4.12.0", "OkHttp"},
		{"Go-http-client/2.0", "Go"},
		{"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", "Bot"},
		{"SomeCrawler/1.0", "Bot"},

		{"", ""},
		{"totally unknown agent", "Other"},
	}

	for _, c := range cases {
		if got := browserFamily(c.ua); got != c.want {
			t.Errorf("browserFamily(%q) = %q, want %q", short(c.ua), got, c.want)
		}
	}
}

// TestBrowserFamilyNeverReturnsTheInput is the property that matters for privacy: whatever comes
// in, what goes out is a family name — never a version, never a platform, never the raw string.
//
// TestBrowserFamilyNeverReturnsTheInput 是隐私上真正重要的性质：
// 无论输入什么，输出都只是一个族名——绝不含版本、绝不含平台、绝不是原始字符串。
func TestBrowserFamilyNeverReturnsTheInput(t *testing.T) {
	inputs := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36",
		"curl/8.21.0",
		"totally unknown agent",
	}
	allowed := map[string]bool{
		"": true, "Other": true, "Chrome": true, "Safari": true, "Firefox": true,
		"Edge": true, "Opera": true, "Bot": true, "curl": true, "wget": true,
		"Python": true, "OkHttp": true, "Go": true,
	}
	for _, in := range inputs {
		got := browserFamily(in)
		if !allowed[got] {
			t.Errorf("browserFamily returned %q, which is not a family in the allow-list", got)
		}
		if got == in {
			t.Errorf("browserFamily echoed its input")
		}
		if len(got) > 16 {
			t.Errorf("browserFamily returned a %d-character value (%q); a family name is short", len(got), got)
		}
	}
}

func short(s string) string {
	if len(s) <= 40 {
		return s
	}
	return s[:40] + "..."
}
