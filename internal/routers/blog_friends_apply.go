package routers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"github.com/haierkeys/fast-note-sync-service/internal/service"
	"go.uber.org/zap"
)

// POST /blog/friends/apply — the friends page's application form.
//
// It follows the same shape as /blog/react: public by design, JSON in, a Chinese message out, and
// a per-address bucket in front of the write. Three differences from that endpoint are worth
// naming:
//
//   - Validation runs before the rate limit. A typo costs the applicant nothing; only a
//     submission that would actually be stored consumes one of the three hourly slots.
//   - A storage failure is a 500 with a generic message. The database error itself names tables
//     and drivers, which is not the reader's business.
//   - A push failure is still a 200, because the row is already durable. That decision lives in
//     the service; this handler only knows the row was written.
//
// POST /blog/friends/apply —— 友链页的申请表单。
//
// 它与 /blog/react 同一形状：刻意公开、JSON 入、中文消息出，写入之前有一个按地址的令牌桶。
// 与那个端点的三处不同值得点名：
//
//   - 校验先于限流。写错不花申请人的额度；只有真正会被存下的提交，才消耗每小时三个名额中的一个。
//   - 存储失败是 500 加一句通用消息。数据库错误本身会带出表名与驱动名，那不是读者该关心的事。
//   - 推送失败仍然是 200，因为行已经持久化。这个决定在服务层；本处理器只知道行写成功了。
func registerBlogFriendApplyRoutes(r *gin.RouterGroup, appContainer *app.App, ident *clientIdentityResolver) {
	r.POST("/blog/friends/apply", func(c *gin.Context) {
		cfg, ok := blogEnabled(c, appContainer)
		if !ok {
			return
		}
		friendSvc := appContainer.FriendApplyService
		if friendSvc == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "申请暂不可用，请稍后再试"})
			return
		}

		// The body is bounded before it is parsed. Without this a hand-written POST could make
		// the service allocate an arbitrarily large string; 64 KB is far above eight short fields
		// and far below anything that matters.
		//
		// 请求体在解析前先设上限。没有这一条，手写的 POST 能让服务分配任意大的字符串；
		// 64 KB 远高于八个短字段所需，又远低于任何值得在意的量。
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)

		var in dto.BlogFriendApplyDTO
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
			return
		}
		if err := service.ValidateBlogFriendApply(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		identity := ident.identity(c)
		if !blogGuardInstance.allowFriendApply(identity) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "提交太频繁，请稍后再试"})
			return
		}

		if err := friendSvc.ApplyFriend(c.Request.Context(), blogUID(cfg.UID), &in, identity, blogBaseURL(c, appContainer)); err != nil {
			// The error is logged with its detail; the response deliberately is not.
			// 错误按原样记日志；响应刻意不带它。
			if appContainer.Logger() != nil {
				appContainer.Logger().Error("blog friend apply failed", zap.Error(err))
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "提交失败，请稍后再试"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}
