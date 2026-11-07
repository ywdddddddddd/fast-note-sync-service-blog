package routers

import (
	"container/list"
	"context"
	_ "embed"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
)

// The friends page's flight animation and the avatar proxy that feeds its planet texture.
//
// 友链页的飞行动画，以及给它提供星球贴图的头像代理。

// The animation is a single self-contained IIFE: the three.js build and the scene code travel
// together, which is why this is one embedded file rather than a chain of modules. It is 3.2 MB,
// so it is served as its own cacheable asset and only requested after a card is clicked — a
// reader who never clicks a friend never downloads it.
//
// 动画是一个自包含的 IIFE：three.js 包体与场景代码一起走，因此这是一个嵌入文件而不是一串模块。
// 它有 3.2 MB，所以作为独立可缓存资产提供，并且只在卡片被点击后请求——从不点友链的读者
// 永远不会下载它。
//
//go:embed assets/warp_flight.js
var blogWarpFlightJS []byte

const (
	blogWarpFlightURL   = "/blog/assets/warp-flight.js"
	blogFlyAvatarURL    = "/blog/fly/avatar"
	blogFlyAvatarMax    = 5 << 20 // 5 MB, the ceiling the handoff fixed
	blogFlyAvatarCacheN = 32
	blogFlyAvatarTTL    = 5 * time.Second
)

// registerBlogFlightRoutes serves the animation bundle and the same-origin avatar proxy.
//
// Both routes live in the blog group so the visit recorder sees them; both are behind the blog
// switch, so turning the blog off also stops the animation being fetchable.
//
// registerBlogFlightRoutes 提供动画包与同源头像代理。
//
// 两条路由都在博客组里，因此访问记录器能看到它们；两者都受博客开关约束，
// 关闭博客也就关闭了动画的可取性。
func registerBlogFlightRoutes(r *gin.RouterGroup, appContainer *app.App) {
	r.GET(blogWarpFlightURL, func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		// The page appends ?v=<process start> to this URL, so a deployment can be cached as
		// immutable without ever serving yesterday's bundle after a restart.
		//
		// 页面会在这个 URL 后追加 ?v=<进程启动时刻>，因此部署后可以按 immutable 缓存，
		// 重启后也绝不会继续提供昨天的包。
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "public, max-age=604800, immutable")
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write(blogWarpFlightJS)
	})

	r.GET(blogFlyAvatarURL, func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		blogAvatarProxyInstance.serve(c)
	})
}

// blogAvatarProxy fetches a friend's avatar server-side and re-serves it from this origin.
//
// Why a proxy at all: a WebGL texture is subject to CORS, and the measured avatar hosts (starting
// with bioez.xyz) send no Access-Control-Allow-Origin. Direct use as a texture therefore fails
// silently — the planet just keeps its procedural look. A same-origin copy removes the browser's
// cross-origin question entirely.
//
// The constraints are the security boundary, not decoration:
//   - http(s) with a host only, so the fetch cannot be a file:// read.
//   - The dialer refuses loopback/private/link-local addresses, so a public URL cannot be used to
//     probe this machine's own services or the LAN.
//   - Content-Type must be an image. image/svg+xml is additionally refused: an SVG served from
//     this origin can run script when opened directly, which would turn a friend's avatar URL
//     into same-origin XSS.
//   - 5 MB and 5 seconds, so one bad host cannot pin a request or the cache.
//
// blogAvatarProxy 在服务端抓取友链头像，并从本源站回吐。
//
// 为什么需要代理：WebGL 贴图受 CORS 约束，而实测的头像主机（从 bioez.xyz 开始）不发送
// Access-Control-Allow-Origin。直接当贴图用会静默失败——星球只是继续用程序化外观。
// 一份同源副本让浏览器的跨源问题彻底不存在。
//
// 这些约束是安全边界而不是装饰：
//   - 只接受带主机的 http(s)，使抓取不可能是 file:// 读取。
//   - 拨号器拒绝回环/私有/链路本地地址，使一个公网 URL 无法用来探测本机服务或局域网。
//   - Content-Type 必须是图片。image/svg+xml 额外拒绝：从本源站提供的 SVG 被直接打开时可以执行脚本，
//     那会把友链的头像地址变成同源 XSS。
//   - 5 MB 与 5 秒，使一个坏主机不能占住请求或缓存。
type blogAvatarProxy struct {
	mu    sync.Mutex
	order *list.List
	items map[string]*list.Element
	max   int
	// client is a field so tests can hand in httptest's client, whose transport is allowed to
	// reach loopback. Production always uses the guarded transport built below.
	//
	// client 做成字段，使测试能传入 httptest 的客户端（其传输允许访问回环）。
	// 生产始终使用下面构建的带防护传输。
	client *http.Client
}

type blogAvatarCacheEntry struct {
	key         string
	body        []byte
	contentType string
}

// blogAvatarProxyInstance is the process-wide proxy, with one shared LRU.
// blogAvatarProxyInstance 是进程级代理，共用一份 LRU。
var blogAvatarProxyInstance = newBlogAvatarProxy(&http.Client{
	Timeout: blogFlyAvatarTTL,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           blogPublicOnlyDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: blogFlyAvatarTTL,
	},
})

func newBlogAvatarProxy(client *http.Client) *blogAvatarProxy {
	return &blogAvatarProxy{
		order:  list.New(),
		items:  make(map[string]*list.Element, blogFlyAvatarCacheN),
		max:    blogFlyAvatarCacheN,
		client: client,
	}
}

// serve handles one proxy request.
// serve 处理一次代理请求。
func (p *blogAvatarProxy) serve(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("u"))
	if raw == "" || len(raw) > 2048 || !blogProxyURLOK(raw) {
		c.Status(http.StatusNotFound)
		return
	}

	if entry, ok := p.get(raw); ok {
		c.Header("Cache-Control", "public, max-age=3600")
		c.Data(http.StatusOK, entry.contentType, entry.body)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), blogFlyAvatarTTL)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	req.Header.Set("Accept", "image/*")
	req.Header.Set("User-Agent", "fast-note-sync/blog avatar proxy")
	resp, err := p.client.Do(req)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		c.Status(http.StatusNotFound)
		return
	}

	contentType := blogImageContentType(resp.Header.Get("Content-Type"))
	if contentType == "" {
		c.Status(http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, blogFlyAvatarMax+1))
	if err != nil || len(body) == 0 || len(body) > blogFlyAvatarMax {
		c.Status(http.StatusNotFound)
		return
	}

	p.put(raw, contentType, body)
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, contentType, body)
}

// get returns a cached image and moves it to the front.
// get 返回缓存的图片并将它移到队首。
func (p *blogAvatarProxy) get(key string) (*blogAvatarCacheEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if el, ok := p.items[key]; ok {
		p.order.MoveToFront(el)
		return el.Value.(*blogAvatarCacheEntry), true
	}
	return nil, false
}

// put inserts or refreshes one entry, evicting the least recently used beyond the cap.
// put 插入或刷新一条记录，超出上限时淘汰最久未使用的。
func (p *blogAvatarProxy) put(key, contentType string, body []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if el, ok := p.items[key]; ok {
		entry := el.Value.(*blogAvatarCacheEntry)
		entry.body = body
		entry.contentType = contentType
		p.order.MoveToFront(el)
		return
	}
	el := p.order.PushFront(&blogAvatarCacheEntry{key: key, body: body, contentType: contentType})
	p.items[key] = el
	for p.order.Len() > p.max {
		back := p.order.Back()
		if back == nil {
			break
		}
		p.order.Remove(back)
		delete(p.items, back.Value.(*blogAvatarCacheEntry).key)
	}
}

// blogProxyURLOK reports whether raw is an absolute http(s) URL with a host.
//
// This is the same scheme/host rule the service uses for friend links, repeated here because
// routers cannot import the unexported helper and this file needs it before any DNS work.
//
// blogProxyURLOK 报告 raw 是否是带主机的绝对 http(s) 地址。
//
// 规则与服务层给友链用的 scheme/host 检查相同，在这里重复一份是因为路由包无法导入那个未导出的
// 辅助函数，而这个文件在做任何 DNS 工作之前就需要它。
func blogProxyURLOK(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return u.Host != ""
	}
	return false
}

// blogImageContentType accepts raster image types and rejects everything else.
//
// SVG is excluded on purpose: it is an image to a decoder but a document to a browser, and one
// served from this origin can run script. Avatars are photographs or icons; the planet texture
// cannot use an SVG anyway.
//
// blogImageContentType 接受栅格图片类型并拒绝其余一切。
//
// SVG 是有意排除的：对解码器它是图片，对浏览器它是文档，而一份从本源站提供的 SVG 可以执行脚本。
// 头像本来就是照片或图标；星球贴图也用不了 SVG。
func blogImageContentType(raw string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	mediaType = strings.ToLower(mediaType)
	if !strings.HasPrefix(mediaType, "image/") || mediaType == "image/svg+xml" {
		return ""
	}
	return mediaType
}

// blogPublicOnlyDialContext refuses to connect to anything that is not a public address.
//
// Resolution happens first, every answer is checked, and the connection is made to the checked
// literal. Dialling the literal rather than the hostname is what closes the DNS-rebinding gap:
// the name is not resolved a second time by the dialer.
//
// blogPublicOnlyDialContext 拒绝连接任何非公网地址。
//
// 先解析、逐一检查每个答案、再连到被检查过的字面量。连字面量而不是主机名，正是为了堵住
// DNS 重绑定缝隙：拨号器不会把名字再解析一遍。
func blogPublicOnlyDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("avatar proxy: no address for %s", host)
	}
	dialer := &net.Dialer{Timeout: blogFlyAvatarTTL}
	for _, candidate := range ips {
		if !blogPublicIP(candidate.IP) {
			continue
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
	}
	return nil, fmt.Errorf("avatar proxy: %s has no public address", host)
}

// blogPublicIP reports whether an address is safe to fetch from.
//
// The list is wider than "not private" on purpose: loopback, unspecified, multicast and
// link-local are all unreachable to a public avatar host, and each one is a way to make this
// server talk to something it should not.
//
// blogPublicIP 报告一个地址是否可以抓取。
//
// 这个列表比「非私有」更宽是有意的：回环、未指定、组播与链路本地对一个公网头像主机都不可达，
// 而每一种都是让这台服务器与不该说话的对象通信的途径。
func blogPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	return !ip.IsPrivate()
}
