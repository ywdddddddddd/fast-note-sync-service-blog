package routers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"github.com/haierkeys/fast-note-sync-service/internal/service"
)

// registerBlogQuizRoutes wires the chapter-end exercise endpoints.
//
// Everything here is POST. That is not stylistic: the questions themselves must never appear
// in a URL or a GET response, because a crawler that follows links would then index the whole
// question bank. POST also matches the semantics — every one of these calls carries a reader's
// own attempt.
//
// 这里全部是 POST。这不是风格选择：题目本身绝不该出现在 URL 或 GET 响应里，否则一个跟随链接的
// 爬虫就能把整个题库索引下来。POST 同时也符合语义——这几次调用每次都携带某位读者自己的作答。
func registerBlogQuizRoutes(blog *gin.RouterGroup, shareSvc service.ShareService) {
	// pick returns both pools for one note. Answers travel with the questions because the
	// browser judges as the reader clicks: waiting for a round trip per question would make
	// the exercise feel like a form submission. The server still re-judges on submit.
	//
	// pick 返回一篇笔记的两个题池。答案随题目一起下发，因为浏览器要按点击即时判定：
	// 每道题等一次往返会让练习变成填表。提交时服务端仍会重判一遍。
	blog.POST("/blog/quiz/pick", func(c *gin.Context) {
		var in struct {
			Slug string `json:"slug"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Slug) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		out, err := shareSvc.BlogQuizPick(c.Request.Context(), in.Slug)
		if err != nil || out == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "quiz unavailable"})
			return
		}
		c.JSON(http.StatusOK, out)
	})

	// submit re-judges the attempt server-side and updates the boards.
	//
	// submit 在服务端重新判一次作答并更新榜单。
	blog.POST("/blog/quiz/submit", func(c *gin.Context) {
		var in dto.BlogQuizSubmitDTO
		if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.NotePath) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		out, err := shareSvc.BlogQuizSubmit(c.Request.Context(), &in)
		if err != nil || out == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "submit failed"})
			return
		}
		c.JSON(http.StatusOK, out)
	})

	// wrong returns one reader's outstanding wrong questions. It is POST because the email
	// travels in the body rather than the query string — addresses do not belong in access
	// logs or referrer headers.
	//
	// wrong 返回某位读者尚未掌握的错题。用 POST 是因为邮箱走请求体而不是查询串——
	// 地址不该出现在访问日志或 referer 里。
	blog.POST("/blog/quiz/wrong", func(c *gin.Context) {
		var in struct {
			Email string `json:"email"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": shareSvc.BlogQuizWrongBook(in.Email)})
	})
}
