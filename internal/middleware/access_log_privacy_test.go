package middleware

import (
	"strings"
	"testing"
)

// TestSanitizeQuery_MasksShareToken is the regression that motivated this file: the share
// pipeline emits the parameter as `share_token` (underscore) while the original sensitive-key
// list only held `share-token` (hyphen), so the 54-character share token was logged verbatim.
//
// TestSanitizeQuery_MasksShareToken 就是本次加固的起因：分享管线发出的参数名是
// `share_token`（下划线），而原敏感键表里只有 `share-token`（连字符），
// 于是 54 字符的分享令牌被明文写进日志。
func TestSanitizeQuery_MasksShareToken(t *testing.T) {
	token := "AAAAAAAAAF4AAAAAAAAAAQAAAABq_A5t7P00YHfF7nzSzW_-O4cdHw"
	cases := []struct {
		name  string
		query string
		leak  string
	}{
		{"share_token underscore", "id=1&share_token=" + token, token},
		{"share-token hyphen", "id=1&share-token=" + token, token},
		{"Share-Token mixed case", "id=1&Share-Token=" + token, token},
		{"token", "rid=9&token=" + token, token},
		{"share_token with password", "share_token=" + token + "&password=hunter2", "hunter2"},
		{"prefixed variant", "x_share_token=" + token, token},
		{"api key variant", "x_custom_api_key=SECRETKEY9988", "SECRETKEY9988"},
		{"apikey no separator", "apikey=SECRETKEY9988", "SECRETKEY9988"},
		{"secret", "client_secret=SECRETKEY9988", "SECRETKEY9988"},
		{"authorization", "authorization=SECRETKEY9988", "SECRETKEY9988"},
	}
	for _, c := range cases {
		got := sanitizeQuery(c.query)
		if strings.Contains(got, c.leak) {
			t.Errorf("%s: credential leaked in %q", c.name, got)
		}
		if !strings.Contains(got, maskValue) {
			t.Errorf("%s: no redaction marker in %q", c.name, got)
		}
	}
}

// TestSanitizeQuery_KeepsNonSensitive ensures ordinary parameters stay readable — the log is
// still useful for debugging.
// TestSanitizeQuery_KeepsNonSensitive 确保普通参数保持可读，日志仍可用于排查。
func TestSanitizeQuery_KeepsNonSensitive(t *testing.T) {
	got := sanitizeQuery("rid=289&rtp=note&sort=created_at&page=2")
	for _, want := range []string{"rid=289", "rtp=note", "sort=created_at", "page=2"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q to survive redaction, got %q", want, got)
		}
	}
	if strings.Contains(got, maskValue) {
		t.Errorf("non-sensitive query was redacted: %q", got)
	}
}

// TestSanitizeQuery_EmptyAndMalformed: an unparseable query must not be echoed back, because
// the original implementation returned it verbatim — the exact failure mode where a malformed
// request still leaked its credential.
//
// TestSanitizeQuery_EmptyAndMalformed：无法解析的 query 绝不能原样回显，因为原实现正是直接
// 返回原始字符串 —— 这正是"畸形请求照样泄漏凭据"的失效模式。
func TestSanitizeQuery_EmptyAndMalformed(t *testing.T) {
	if got := sanitizeQuery(""); got != "" {
		t.Errorf("empty query should stay empty, got %q", got)
	}
	bad := "token=%zz&share_token=" + strings.Repeat("A", 54)
	got := sanitizeQuery(bad)
	if strings.Contains(got, strings.Repeat("A", 54)) {
		t.Errorf("malformed query echoed a credential: %q", got)
	}
	if got != "[unparsable-query-redacted]" {
		t.Errorf("malformed query should be replaced by the placeholder, got %q", got)
	}
}

// TestMaskPathTokens covers the native share page, whose token lives in the path rather than
// the query.
// TestMaskPathTokens 覆盖原生分享页 —— 它的令牌在路径里而非查询串里。
func TestMaskPathTokens(t *testing.T) {
	token := "AAAAAAAAAF4AAAAAAAAAAQAAAABq_A5t7P00YHfF7nzSzW_-O4cdHw"
	cases := []struct{ in, mustContain, mustNotContain string }{
		{"/share/424/" + token, "/share/424/" + maskValue, token},
		{"/api/share/file/" + token, maskValue, token},
		{"/s/1.94-690a504f02", "/s/1.94-690a504f02", maskValue}, // slug is a public address, not a credential
		{"/api/health", "/api/health", maskValue},
		{"/blog/1.94-690a504f02", "/blog/1.94-690a504f02", maskValue},
	}
	for _, c := range cases {
		got := maskPathTokens(c.in)
		if !strings.Contains(got, c.mustContain) {
			t.Errorf("maskPathTokens(%q) = %q, want it to contain %q", c.in, got, c.mustContain)
		}
		if c.mustNotContain != "" && strings.Contains(got, c.mustNotContain) {
			t.Errorf("maskPathTokens(%q) = %q, must not contain %q", c.in, got, c.mustNotContain)
		}
	}
}

// TestMaskIP pins the privacy contract: the host part is hidden, the network part survives.
// TestMaskIP 固定隐私契约：主机部分被遮蔽，网段部分保留。
func TestMaskIP(t *testing.T) {
	cases := []struct{ in, want string }{
		{"127.0.0.1", "127.0.0.x"},
		{"192.168.1.7", "192.168.1.x"},
		{"10.121.19.48", "10.121.19.x"},
		{"203.0.113.9:54321", "203.0.113.x"},
		{"2001:db8::1", "2001:db8::x"},
		{"fe80::1", "fe80::x"},
		{"", "unknown"},
		{"not-an-ip", "unknown"},
		{"<script>alert(1)</script>", "unknown"},
	}
	for _, c := range cases {
		if got := maskIP(c.in); got != c.want {
			t.Errorf("maskIP(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSanitizeUserAgent verifies truncation and control-character removal: an unbounded field
// lets a client inject newlines into the log, and the full UA is a stable device fingerprint.
//
// TestSanitizeUserAgent 验证截断与控制字符清除：不设限的字段允许客户端向日志注入换行，
// 而完整 UA 是稳定的设备指纹。
func TestSanitizeUserAgent(t *testing.T) {
	if got := sanitizeUserAgent("curl/8.21.0"); got != "curl/8.21.0" {
		t.Errorf("short UA changed: %q", got)
	}
	got := sanitizeUserAgent("Mozilla/5.0\nFAKE-INJECTED-LOG-LINE\t\u0000end")
	if strings.ContainsAny(got, "\n\t\x00") {
		t.Errorf("control characters survived: %q", got)
	}
	if !strings.Contains(got, "Mozilla/5.0") {
		t.Errorf("expected content lost: %q", got)
	}
	long := strings.Repeat("A", 500)
	gotLong := sanitizeUserAgent(long)
	if len(gotLong) > 220 {
		t.Errorf("UA not truncated, len=%d", len(gotLong))
	}
	if !strings.HasSuffix(gotLong, "…") {
		t.Errorf("truncated UA should end with an ellipsis marker, got tail %q", gotLong[len(gotLong)-5:])
	}
}

// TestIsSensitiveKey documents the normalization contract.
// TestIsSensitiveKey 记录归一化契约。
func TestIsSensitiveKey(t *testing.T) {
	sensitive := []string{"token", "TOKEN", "share_token", "share-token", "Share-Token",
		"password", "user_password", "api_key", "apikey", "x_custom_api_key", "secret", "jwt", "sig"}
	for _, k := range sensitive {
		if !isSensitiveKey(k) {
			t.Errorf("isSensitiveKey(%q) = false, want true", k)
		}
	}
	benign := []string{"rid", "rtp", "page", "sort", "limit", "id", "vault", "path", ""}
	for _, k := range benign {
		if isSensitiveKey(k) {
			t.Errorf("isSensitiveKey(%q) = true, want false", k)
		}
	}
}
