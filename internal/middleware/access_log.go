package middleware

import (
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AccessLogWithLogger creates access log middleware with logger (supports dependency injection)
// AccessLogWithLogger 创建带日志器的访问日志中间件（支持依赖注入）
//
// Privacy hardening (2026-10): the original implementation leaked share credentials. Its
// sensitive-key list held "share-token" while the share pipeline actually emits "share_token",
// so every attachment URL printed its 54-character share token in clear text, and the request
// path was never sanitized at all. IP and User-Agent were recorded verbatim.
//
// 隐私加固（2026-10）：原实现的敏感键表里写的是 "share-token"，而分享管线实际发出的参数名是
// "share_token"，于是每个附件 URL 都把 54 字符的分享令牌明文写进日志；请求路径则完全没有脱敏；
// IP 与 User-Agent 也是原样记录。
//
// Contract kept unchanged: same logger, same level (Info), same field names, so existing log
// consumers and the "log.level: info" switch behave exactly as before.
// 保持不变的契约：同一个 logger、同一级别（Info）、同样的字段名，既有日志消费方与
// "log.level: info" 开关的行为完全不变。
func AccessLogWithLogger(logger *zap.Logger) gin.HandlerFunc {
	return AccessLogWithRecorder(logger, nil)
}

// AccessLogRecorder receives the already-masked fields of one request, for callers that keep a
// structured record in addition to the text log.
//
// The callback is handed masked values only. That is not a convention but the type's reason for
// existing: the blog visit log must never be able to store a raw IP or a query string, so it is
// never offered one. A recorder that wants them has nothing to receive.
//
// AccessLogRecorder 接收一次请求中已打码的字段，供那些除文本日志之外还要保留结构化记录的调用方使用。
//
// 回调只会拿到已打码的值。这不是约定，而是这个类型存在的理由：博客访问日志绝不能存下原始 IP
// 或查询串，因此根本不会把它交给回调。想要这些值的记录器没有任何东西可收。
type AccessLogRecorder func(method, path string, status int, ms int64, ip, client string)

// AccessLogWithRecorder is AccessLogWithLogger plus an optional structured record.
//
// recorder may be nil, in which case behaviour is identical to the logger-only form. The text log
// is written either way: it is the existing contract with log consumers, and the structured log
// is an addition rather than a replacement.
//
// AccessLogWithRecorder 是 AccessLogWithLogger 加上可选的结构化记录。
//
// recorder 可以为 nil，此时行为与「仅文本日志」形式完全一致。文本日志两种情况都会写：
// 它是与既有日志消费方之间的契约，而结构化日志是新增，不是替代。
func AccessLogWithRecorder(logger *zap.Logger, recorder AccessLogRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {

		path := maskPathTokens(c.Request.URL.Path)
		query := sanitizeQuery(c.Request.URL.RawQuery)

		startTime := time.Now()
		c.Next()

		timeCost := time.Since(startTime)
		maskedIP := maskIP(c.ClientIP())
		client := sanitizeUserAgent(c.Request.UserAgent())

		logger.Info(path,
			zap.String("method", c.Request.Method),
			zap.String("url", path+"?"+query),
			zap.String("start-time", startTime.Format("2006-01-02 15:04:05")),
			zap.Duration("time-cost", timeCost),
			zap.String("ip", maskedIP),
			zap.String("user-agent", client),
			zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()),
		)

		if recorder != nil {
			// Only blog pages reach the structured log; the filter lives in the recorder rather
			// than here so that this middleware keeps its single responsibility.
			//
			// 只有博客页面进入结构化日志；过滤放在记录器里而不是这里，
			// 使这个中间件保持单一职责。
			recorder(c.Request.Method, path, c.Writer.Status(), timeCost.Milliseconds(), maskedIP, client)
		}
	}
}

// BlogVisitRecorder is the visit-log middleware for the blog's own routes.
//
// It exists separately from AccessLogWithRecorder because the blog is not served from the API
// group: the public pages are registered on the engine directly, so a middleware attached to the
// API group never sees them. Measured: after six /blog requests no visit file had been written.
//
// It deliberately does not write the text log — that is the API middleware's job, and writing it
// twice would double every blog line in log.log.
//
// BlogVisitRecorder 是博客自身路由的访问日志中间件。
//
// 它与 AccessLogWithRecorder 分开存在，因为博客并不从 API 组提供服务：
// 公开页面直接注册在 engine 上，因此挂在 API 组上的中间件永远看不到它们。
// 实测：六次 /blog 请求之后没有任何访问文件被写出。
//
// 它刻意不写文本日志——那是 API 中间件的职责，写两遍会让 log.log 里每个博客行都翻倍。
func BlogVisitRecorder(recorder AccessLogRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if recorder == nil {
			c.Next()
			return
		}

		// Masking happens here, before the recorder is offered anything, so the visit log can
		// never hold a raw address even if a future recorder asks for one.
		//
		// 打码发生在这里、在把任何东西交给记录器之前，因此即使将来某个记录器索要原始地址，
		// 访问日志也绝不会持有它。
		path := maskPathTokens(c.Request.URL.Path)
		maskedIP := maskIP(c.ClientIP())
		client := browserFamily(c.Request.UserAgent())

		startTime := time.Now()
		c.Next()

		recorder(c.Request.Method, path, c.Writer.Status(), time.Since(startTime).Milliseconds(), maskedIP, client)
	}
}

// browserFamily reduces a User-Agent to a coarse family name.
//
// The visit log states that it records a browser family, so it must record a browser family: a
// stored full UA is a device and version fingerprint, and the fact that the text log keeps one is
// a separate decision that belongs to that log.
//
// Measured before this fix: the file held
// "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36" —
// that is, the declared privacy posture and the actual bytes disagreed.
//
// browserFamily 把 User-Agent 归约为粗粒度的族名。
//
// 访问日志声明它记录的是浏览器族，因此它必须记录浏览器族：存下完整 UA 就是存下设备与版本指纹，
// 而文本日志保留完整 UA 是另一个属于那个日志的决定。
//
// 修复前实测：文件里存着
// "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36"——
// 也就是说，声明的隐私姿态与实际写入的字节不一致。
func browserFamily(ua string) string {
	if ua == "" {
		return ""
	}
	l := strings.ToLower(ua)

	// Order matters: every Chrome UA contains "safari", and every Edge UA contains "chrome".
	// 顺序有意义：每个 Chrome 的 UA 都含 "safari"，每个 Edge 的 UA 都含 "chrome"。
	switch {
	case strings.Contains(l, "edg/") || strings.Contains(l, "edge/"):
		return "Edge"
	case strings.Contains(l, "opr/") || strings.Contains(l, "opera"):
		return "Opera"
	case strings.Contains(l, "firefox/"):
		return "Firefox"
	case strings.Contains(l, "chrome/") || strings.Contains(l, "crios/"):
		return "Chrome"
	case strings.Contains(l, "safari/"):
		return "Safari"
	case strings.Contains(l, "bot") || strings.Contains(l, "crawler") || strings.Contains(l, "spider"):
		return "Bot"
	case strings.HasPrefix(l, "curl/"):
		return "curl"
	case strings.HasPrefix(l, "wget/"):
		return "wget"
	case strings.Contains(l, "python-requests") || strings.HasPrefix(l, "python/"):
		return "Python"
	case strings.Contains(l, "okhttp"):
		return "OkHttp"
	case strings.Contains(l, "go-http-client"):
		return "Go"
	}
	return "Other"
}

// maskValue is what a redacted parameter is replaced with. It is deliberately a plain word:
// the query is re-encoded with url.Values.Encode(), and a symbol-heavy placeholder such as
// "******" comes out as "%2A%2A%2A%2A%2A%2A", which is unreadable in a log viewer.
//
// maskValue 是敏感参数被替换后的统一占位符。刻意使用普通单词：查询串会用
// url.Values.Encode() 重新编码，像 "******" 这样以符号为主的占位符会变成
// "%2A%2A%2A%2A%2A%2A"，在日志查看器里完全不可读。
const maskValue = "REDACTED"

// sensitiveKeyList holds normalized key names that always carry a credential.
// sensitiveKeyList 保存归一化后一定承载凭据的键名。
var sensitiveKeyList = map[string]bool{
	"token":         true,
	"password":      true,
	"passwd":        true,
	"pwd":           true,
	"access_token":  true,
	"share_token":   true,
	"refresh_token": true,
	"id_token":      true,
	"api_key":       true,
	"apikey":        true,
	"secret":        true,
	"client_secret": true,
	"authorization": true,
	"auth":          true,
	"bearer":        true,
	"jwt":           true,
	"signature":     true,
	"sig":           true,
	"code":          true,
}

// sensitiveKeySuffixes catches namespaced variants such as `x_share_token`, `user_password` or
// `x_custom_api_key` — a header-style prefix must not defeat the redaction.
//
// sensitiveKeySuffixes 覆盖带前缀的变体，例如 `x_share_token`、`user_password` 或
// `x_custom_api_key` —— 带风格的头部前缀不应该让脱敏失效。
var sensitiveKeySuffixes = []string{"_token", "_password", "_passwd", "_secret", "_apikey", "_key"}

// normalizeKey lowercases a key and folds '-' into '_' so that `share-token`, `share_token`
// and `Share-Token` all collapse to the same identity.
//
// normalizeKey 把键名小写并把 '-' 折叠成 '_'，使 `share-token`、`share_token`、`Share-Token`
// 归一到同一个身份。
func normalizeKey(k string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(k)), "-", "_")
}

func isSensitiveKey(k string) bool {
	n := normalizeKey(k)
	if n == "" {
		return false
	}
	if sensitiveKeyList[n] {
		return true
	}
	for _, suf := range sensitiveKeySuffixes {
		if strings.HasSuffix(n, suf) {
			return true
		}
	}
	return false
}

// sanitizeQuery masks sensitive parameters in query string.
//
// On parse failure the original code returned the raw query untouched — that is the failure
// mode where a malformed request still leaks a credential verbatim. Here an unparseable query
// is replaced by a redacted placeholder instead of being echoed.
//
// sanitizeQuery 遮蔽请求查询字符串中的敏感参数值。
//
// 解析失败时，原实现直接返回原始 query —— 这正是"畸形请求照样明文泄漏凭据"的失效模式。
// 现在无法解析的 query 会被替换为脱敏占位符，而不是原样回显。
func sanitizeQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		// Do not echo the raw value: we cannot tell which part of it is a credential.
		// 不回显原值：无法判断其中哪一段是凭据。
		return "[unparsable-query-redacted]"
	}
	changed := false
	for k := range values {
		if isSensitiveKey(k) {
			values.Set(k, maskValue)
			changed = true
		}
	}
	if changed {
		return values.Encode()
	}
	return rawQuery
}

// pathTokenRegex matches a credential-looking segment immediately after a share-ish prefix.
// The first group keeps everything up to and including the separator, so the replacement is
// just prefix + mask.
//
// pathTokenRegex 匹配分享类前缀之后紧跟的、形如凭据的长段。第一组保留前缀（含分隔符），
// 因此替换结果就是「前缀 + 掩码」。
//
// RE2 has no backreferences, so the prefix is re-stated rather than referenced.
// RE2 不支持反向引用，因此前缀在此重新书写而不是引用。
var pathTokenRegex = regexp.MustCompile(`(?i)(/(?:share|s|api/share)/[^/]{1,32}/)([A-Za-z0-9_\-]{16,})`)

// maskPathTokens redacts credential-looking path segments.
//
// The native share page carries its token in the path (`/share/<side>/<token>`), and the raw
// path was previously logged verbatim. It is not reachable through the API group today, but
// this is defence in depth: the blog endpoints added later put signed identifiers in paths too.
//
// maskPathTokens 遮蔽路径中形如凭据的段。
//
// 原生分享页把令牌放在路径里（`/share/<side>/<token>`），而原实现把路径原样写进日志。
// 该路由目前不经过 API 组，但这是纵深防御：后续新增的博客端点同样会把签名标识放进路径。
func maskPathTokens(p string) string {
	if p == "" {
		return p
	}
	return pathTokenRegex.ReplaceAllString(p, "${1}"+maskValue)
}

// maskIP hides the host portion of a client address while keeping enough of it to be useful
// for abuse investigation. IPv4 keeps the /24 network, IPv6 keeps the /48 prefix.
//
// Unparseable values are returned as "unknown" rather than echoed, so a header-borne payload
// cannot smuggle arbitrary content into the log.
//
// maskIP 遮蔽客户端地址的主机部分，同时保留足够信息用于滥用排查：IPv4 保留 /24 网段，
// IPv6 保留 /48 前缀。
//
// 无法解析的值返回 "unknown" 而不是原样回显，避免经请求头夹带的载荷进入日志。
func maskIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return "unknown"
	}
	// Strip a port if the client address carried one.
	// 若地址带端口则去掉端口。
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "unknown"
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String()[:strings.LastIndex(v4.String(), ".")] + ".x"
	}
	// IPv6: keep the leading hextets up to /48, stopping at the "::" compression point so a
	// compressed address never yields a malformed "a:b:::x" prefix.
	// IPv6：保留前导 hextet 至 /48，在 "::" 压缩点停止，避免压缩地址拼出 "a:b:::x" 这类畸形前缀。
	kept := make([]string, 0, 3)
	for _, h := range strings.Split(parsed.String(), ":") {
		if h == "" || len(kept) == 3 {
			break
		}
		kept = append(kept, h)
	}
	if len(kept) == 0 {
		return "unknown"
	}
	return strings.Join(kept, ":") + "::x"
}

// sanitizeUserAgent truncates the agent string and strips control characters. The full UA is a
// stable device fingerprint, and an unbroken field also lets a client inject newlines into the
// log; 200 characters keep the browser/OS family readable without preserving the whole identity.
//
// sanitizeUserAgent 截断 agent 字符串并去除控制字符。完整 UA 是稳定的设备指纹，且不设限的字段
// 允许客户端向日志注入换行；200 字符足以读出浏览器/系统族，又不保留完整身份。
func sanitizeUserAgent(ua string) string {
	ua = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, ua)
	ua = strings.TrimSpace(ua)
	const maxUA = 200
	if len(ua) > maxUA {
		return ua[:maxUA] + "…"
	}
	return ua
}
