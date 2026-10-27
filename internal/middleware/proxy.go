package middleware

import (
	"crypto/tls"
	"net"

	"github.com/gin-gonic/gin"
)

// Proxy handles proxy headers and restores original request information based on trusted proxies
// Proxy 根据可信代理处理代理头部并恢复原始请求信息
func Proxy(trustedProxies []string) gin.HandlerFunc {
	// If trustedProxies is empty, default to loopback addresses only
	// 如果 trustedProxies 为空，默认只信任本地回环地址
	if len(trustedProxies) == 0 {
		trustedProxies = []string{"127.0.0.1", "::1"}
	}

	var trustedIPs []net.IP
	var trustedSubnets []*net.IPNet

	for _, proxy := range trustedProxies {
		if ip := net.ParseIP(proxy); ip != nil {
			trustedIPs = append(trustedIPs, ip)
		} else if _, subnet, err := net.ParseCIDR(proxy); err == nil {
			trustedSubnets = append(trustedSubnets, subnet)
		}
	}

	isTrusted := func(ipStr string) bool {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return false
		}
		for _, tip := range trustedIPs {
			if tip.Equal(ip) {
				return true
			}
		}
		for _, subnet := range trustedSubnets {
			if subnet.Contains(ip) {
				return true
			}
		}
		return false
	}

	return func(c *gin.Context) {
		remoteHost, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
		trusted := isTrusted(remoteHost)

		proto := ""
		if trusted {
			proto = c.GetHeader("X-Forwarded-Proto")
		}

		if proto == "" {
			if c.Request.TLS != nil {
				proto = "https"
			} else {
				proto = "http"
			}
		}

		// Update Request URL Scheme
		// 更新请求 URL 的 Scheme
		c.Request.URL.Scheme = proto

		// If protocol is https but TLS is nil (due to proxy termination),
		// we "fake" a TLS state to satisfy libraries that check r.TLS != nil
		// 如果协议是 https 但 TLS 为 nil（由于代理终止），
		// 我们“伪造”一个 TLS 状态以满足检查 r.TLS != nil 的库
		if proto == "https" && c.Request.TLS == nil {
			c.Request.TLS = &tls.ConnectionState{}
		}

		// Detect host from X-Forwarded-Host if connection is trusted
		// 如果连接可信，则从 X-Forwarded-Host 检测主机名
		if trusted {
			if host := c.GetHeader("X-Forwarded-Host"); host != "" {
				c.Request.Host = host
			}
		}

		// CF-Connecting-IP is deliberately NOT trusted here, and this is the one place in the
		// codebase where "no code" is the fix.
		//
		// The removed version rewrote RemoteAddr from that header whenever the TCP peer was
		// loopback, on the reasoning that only a local cloudflared process connects from
		// loopback. That reasoning holds only if cloudflared is the *sole* loopback client.
		// On this deployment it is not: the topology is
		//
		//     visitor → entry nginx (39.105.27.90) → origin nginx → 127.0.0.1:9000 fns
		//
		// so every external visitor arrives from 127.0.0.1 and the guard was true for all of
		// them. Measured before removal: eight requests with eight different forged values of
		// this header were all answered 200, against 200/200/429… with the header held constant
		// — the rate limiter and the reaction dedup were keyed on a value the client chose.
		//
		// The header is also not needed for anything this service does. The real client address
		// already arrives on X-Real-IP, which nginx overwrites with $remote_addr (it sets, it
		// does not append, so a client cannot pre-seed it), and blogGuard already reads that
		// header only when the peer is a configured trusted proxy.
		//
		// 这里刻意不信任 CF-Connecting-IP —— 保留「什么都不写」正是修复本身。
		//
		// 被删掉的版本在对端为回环时用该头改写 RemoteAddr，理由是「只有本地 cloudflared
		// 会从回环连进来」。这个前提只在 cloudflared 是唯一回环客户端时成立。本部署不是：
		// 访客 → 入口机 nginx → 源站 nginx → 127.0.0.1:9000，于是每个外网访客都从
		// 127.0.0.1 到达，那道守卫对所有人成立。删除前实测：同一请求头换八个伪造值全部
		// 回答 200，而固定该头时为 200/200/429…——限流与表态去重都键在一个客户端自选的值上。
		//
		// 本服务也不需要这个头：真实地址已经在 X-Real-IP 里，nginx 用 $remote_addr 覆盖写
		//（是写，不是追加，客户端无法预置），而 blogGuard 只在 TCP 对端是已配置的可信代理时
		// 才读它。

		c.Next()
	}
}
