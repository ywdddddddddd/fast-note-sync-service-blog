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
