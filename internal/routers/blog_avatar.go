package routers

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
)

// The card page's portrait is embedded in the binary rather than read from the vault.
//
// It is not content: it does not change when a note changes, it has no attachments row, and it
// must still render on a machine whose storage has not been seeded yet. Embedding also means the
// URL is stable across deployments, so it can be cached hard without a version query.
//
// 名片页的头像是嵌进二进制的，而不是从仓库读。
//
// 它不是内容：笔记变动它不变，它在附件表里没有行，而且在一台尚未灌入存储的机器上也必须能显示。
// 嵌入还让 URL 在部署之间保持稳定，因此可以放心长缓存，不需要版本参数。
//
//go:embed assets/avatar.jpg
var blogAvatarJPEG []byte

// The site icon is the owner's own artwork, embedded for the same reasons as the portrait: it is
// not vault content, it must survive a machine with empty storage, and a stable URL can be cached.
//
// 站点图标是站主自己的图案，嵌入的理由与头像相同：它不是仓库内容、在存储为空的机器上也必须能出图，
// 而稳定的 URL 才可以放心缓存。
//
//go:embed assets/icon.svg
var blogIconSVG []byte

const blogIconURL = "/blog/icon.svg"

// registerBlogIconRoute serves the embedded icon.
// registerBlogIconRoute 提供内嵌站点图标。
func registerBlogIconRoute(r *gin.Engine, appContainer *app.App) {
	r.GET(blogIconURL, func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		c.Header("Cache-Control", "public, max-age=604800")
		c.Data(http.StatusOK, "image/svg+xml", blogIconSVG)
	})
	// Browsers still ask for /favicon.ico unprompted, and a 404 there leaves a blank tab icon in
	// clients that ignore the <link> element. Redirecting is enough: they follow it once.
	//
	// 浏览器仍会不请自来地请求 /favicon.ico，而那里的 404 会让忽略 link 元素的客户端留一个空白标签图标。
	// 重定向就够了：它们会跟随一次。
	r.GET("/favicon.ico", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, blogIconURL)
	})
}

// blogAvatarURL is the single place the default portrait address is written.
// blogAvatarURL 是默认头像地址的唯一出处。
const blogAvatarURL = "/blog/avatar.jpg"

// registerBlogAvatarRoute serves the embedded portrait.
// registerBlogAvatarRoute 提供内嵌头像。
func registerBlogAvatarRoute(r *gin.Engine, appContainer *app.App) {
	r.GET(blogAvatarURL, func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		// One day, not a year: the bytes can change when the owner swaps the portrait, and this
		// URL carries no fingerprint to make a year safe.
		//
		// 一天而不是一年：站主换头像时字节会变，而这个 URL 没有指纹让「一年」变得安全。
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(http.StatusOK, "image/jpeg", blogAvatarJPEG)
	})
}
