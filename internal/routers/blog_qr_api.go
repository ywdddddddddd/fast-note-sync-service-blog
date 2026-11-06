package routers

import (
	"image/color"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/service/qr"
)

// registerBlogQRRoutes 挂上分享卡用的二维码图片。
//
// 为什么由服务端出图而不是在浏览器里画：二维码的编码（分块、Reed-Solomon、掩码选择）
// 是一套不能出错的算法，写在服务端可以用参考实现逐位比对（见 internal/service/qr 的测试）；
// 写在页面 JS 里就只能靠肉眼看扫得出来没有。前端拿一张 PNG 是最省事的形态。
//
// 为什么不用外部二维码接口：每篇文章的地址会被发给第三方，而且断网就失效。
//
// qr 包只实现到版本 10 / 纠错等级 M，能装 216 字节；这里把长度卡在 200，
// 超了直接 400，而不是让编码器悄悄截断——截断的二维码扫出来是一个错误的地址。
func registerBlogQRRoutes(blog *gin.RouterGroup) {
	blog.GET("/blog/qr.png", func(c *gin.Context) {
		text := strings.TrimSpace(c.Query("t"))
		if text == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if len(text) > 200 {
			c.Status(http.StatusRequestURITooLong)
			return
		}
		scale := 8
		if v, err := strconv.Atoi(c.Query("s")); err == nil && v >= 1 && v <= 24 {
			scale = v
		}

		m, err := qr.Encode(text, -1)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		// 颜色跟着调用方给：深色卡片上要浅色码，否则贴在深底上看不见。
		dark, light := color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 0}
		if c.Query("c") == "dark" {
			dark, light = color.RGBA{237, 237, 237, 255}, color.RGBA{0, 0, 0, 0}
		}

		buf, err := qr.PNG(m, scale, 4, dark, light)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		// 同一段文字永远对应同一张图，可以长缓存；URL 里带着文字本身，
		// 因此不需要再加版本号。
		c.Header("Content-Type", "image/png")
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Data(http.StatusOK, "image/png", buf)
	})
}
