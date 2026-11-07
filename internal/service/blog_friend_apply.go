package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/haierkeys/fast-note-sync-service/internal/config"
	"github.com/haierkeys/fast-note-sync-service/internal/domain"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"go.uber.org/zap"
)

// Friend applications from the friends page.
//
// The page is public, so the form is the one place a stranger can put text into this server's
// database. Two consequences shape everything below:
//
//   - The row is written before the notification is attempted. A push that fails must not lose
//     the application — the database is the durable copy, Bark is only how the owner hears about
//     it.
//   - Every value is trimmed and bounded before it is stored. The browser's maxlength is a
//     convenience, not a guarantee; the same rules have to hold for a hand-written POST.
//
// 友链页的申请。
//
// 页面是公开的，因此这个表单是陌生人能把文本写进这台服务器数据库的唯一地方。两件事决定了
// 下面的全部设计：
//
//   - 先写库，再尝试通知。推送失败不得丢掉申请——数据库是持久副本，Bark 只是站主得知它的方式。
//   - 每个值在入库前都去空白并限制长度。浏览器的 maxlength 是便利，不是保证；
//     手写一个 POST 时同一条规则必须仍然成立。

// Field limits, in runes. They match the friends copy card rather than inventing new numbers:
// a value accepted here should fit the frontmatter a later approval pastes it into.
//
// 字段上限，按 rune 计。取值与友链复制卡一致，而不是另发明一套数字：
// 这里接受的値，应当放得进日后批准时粘贴它的 frontmatter。
const (
	blogFriendApplyNameMax    = 60
	blogFriendApplyURLMax     = 300
	blogFriendApplyDescMax    = 120
	blogFriendApplyEmailMax   = 120
	blogFriendApplyMessageMax = 300
)

// barkPushTimeout bounds the notification call. The application is already stored by then, so a
// slow Bark server must not hold the applicant's browser open for longer than this.
//
// barkPushTimeout 限制通知调用的时长。此时申请已经入库，因此一个缓慢的 Bark 服务
// 不得把申请人的浏览器挂住超过这个时间。
const barkPushTimeout = 5 * time.Second

// FriendApplyService stores applications and announces them.
// FriendApplyService 存储申请并发出通知。
type FriendApplyService interface {
	// ApplyFriend validates, stores and (when configured) pushes one application.
	// ApplyFriend 校验、存储并按配置推送一条申请。
	ApplyFriend(ctx context.Context, uid int64, in *dto.BlogFriendApplyDTO, remoteIP, baseURL string) error
}

// friendApplyService is the concrete implementation.
// friendApplyService 是具体实现。
type friendApplyService struct {
	repo   domain.FriendApplyRepository
	logger *zap.Logger
	config *config.BlogConfig
	client *http.Client
}

// NewFriendApplyService wires the repository, logger and blog configuration.
//
// The config is passed as a pointer so a reloaded config.yaml is observed without rebuilding the
// service; the push path only reads it at call time.
//
// NewFriendApplyService 装配仓储、日志与博客配置。
//
// 配置以指针传入，使重新加载的 config.yaml 无需重建服务即可生效；推送路径只在调用时读取它。
func NewFriendApplyService(repo domain.FriendApplyRepository, logger *zap.Logger, cfg *config.BlogConfig) FriendApplyService {
	return &friendApplyService{
		repo:   repo,
		logger: logger,
		config: cfg,
		client: &http.Client{Timeout: barkPushTimeout},
	}
}

// ValidateBlogFriendApply trims and checks one application in place, returning a message meant for
// the reader rather than for a log.
//
// Why validation lives here and not in the router: the rules are about what this service is
// willing to store, and a second caller (a test, a future API) must get the same answer. The
// router's job is only to turn the error into a 400.
//
// ValidateBlogFriendApply 就地修剪并校验一条申请，返回的是给读者看的话，而不是给日志看的。
//
// 校验放在这里而不是路由层，是因为这些规则说的是「本服务愿意存什么」，第二个调用方
// （测试、将来的 API）必须得到同一个答案。路由只负责把错误变成 400。
func ValidateBlogFriendApply(in *dto.BlogFriendApplyDTO) error {
	in.Name = strings.TrimSpace(in.Name)
	in.URL = strings.TrimSpace(in.URL)
	in.Avatar = strings.TrimSpace(in.Avatar)
	in.Desc = strings.TrimSpace(in.Desc)
	in.Email = strings.TrimSpace(in.Email)
	in.Feed = strings.TrimSpace(in.Feed)
	in.Message = strings.TrimSpace(in.Message)

	if in.Name == "" {
		return errors.New("站名不能为空")
	}
	if utf8.RuneCountInString(in.Name) > blogFriendApplyNameMax {
		return fmt.Errorf("站名过长（最多 %d 字）", blogFriendApplyNameMax)
	}
	if in.URL == "" {
		return errors.New("链接不能为空")
	}
	if utf8.RuneCountInString(in.URL) > blogFriendApplyURLMax {
		return fmt.Errorf("链接过长（最多 %d 字）", blogFriendApplyURLMax)
	}
	if !blogTrustedExternalURL(in.URL) {
		return errors.New("链接需为 http/https 地址")
	}
	if in.Avatar != "" {
		if utf8.RuneCountInString(in.Avatar) > blogFriendApplyURLMax {
			return fmt.Errorf("头像链接过长（最多 %d 字）", blogFriendApplyURLMax)
		}
		if !blogTrustedExternalURL(in.Avatar) {
			return errors.New("头像链接需为 http/https 地址")
		}
	}
	if in.Feed != "" {
		if utf8.RuneCountInString(in.Feed) > blogFriendApplyURLMax {
			return fmt.Errorf("订阅链接过长（最多 %d 字）", blogFriendApplyURLMax)
		}
		if !blogTrustedExternalURL(in.Feed) {
			return errors.New("订阅链接需为 http/https 地址")
		}
	}
	if utf8.RuneCountInString(in.Desc) > blogFriendApplyDescMax {
		return fmt.Errorf("描述过长（最多 %d 字）", blogFriendApplyDescMax)
	}
	if in.Email != "" {
		if utf8.RuneCountInString(in.Email) > blogFriendApplyEmailMax {
			return fmt.Errorf("邮箱过长（最多 %d 字）", blogFriendApplyEmailMax)
		}
		if !strings.Contains(in.Email, "@") {
			return errors.New("邮箱格式不正确")
		}
	}
	if utf8.RuneCountInString(in.Message) > blogFriendApplyMessageMax {
		return fmt.Errorf("留言过长（最多 %d 字）", blogFriendApplyMessageMax)
	}
	return nil
}

// ApplyFriend validates, stores and then attempts the Bark push.
//
// The push error is logged and swallowed deliberately: the row is already durable, and telling
// the applicant "failed" because a notification did not go through would push them to submit a
// duplicate. A 200 here means "stored", not "the owner has seen it".
//
// ApplyFriend 校验、存储，然后尝试 Bark 推送。
//
// 推送错误被记录后吞掉是有意的：行已经持久化，而因为通知没发出去就告诉申请人「失败」，
// 只会逼他再提交一条重复的。这里的 200 意思是「已入库」，不是「站主已经看到」。
func (s *friendApplyService) ApplyFriend(ctx context.Context, uid int64, in *dto.BlogFriendApplyDTO, remoteIP, baseURL string) error {
	if err := ValidateBlogFriendApply(in); err != nil {
		return err
	}
	row := &domain.FriendApply{
		UID:      uid,
		Name:     in.Name,
		URL:      in.URL,
		Avatar:   in.Avatar,
		Desc:     in.Desc,
		Email:    in.Email,
		Feed:     in.Feed,
		Message:  in.Message,
		RemoteIP: remoteIP,
		Status:   domain.FriendApplyStatusNew,
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return err
	}
	s.pushBark(ctx, in, baseURL)
	return nil
}

// pushBark announces a stored application, returning nothing because every failure is already a
// log line by the time the caller could act on it.
//
// An empty key is not an error: it is the default for an open-source checkout and for a local
// test, and the documented meaning is "store but do not announce".
//
// pushBark 通知一条已入库的申请；不返回错误，因为等调用方能够处理时，任何失败都已经只是一行日志。
//
// 空 key 不是错误：它是开源检出与本地测试的默认值，其含义已写明为「入库但不通知」。
func (s *friendApplyService) pushBark(ctx context.Context, in *dto.BlogFriendApplyDTO, baseURL string) {
	if s.config == nil {
		return
	}
	key := strings.TrimSpace(s.config.BarkKey)
	if key == "" {
		return
	}
	server := strings.TrimSpace(s.config.BarkServer)
	if server == "" {
		server = "https://api.day.app"
	}
	endpoint := strings.TrimRight(server, "/") + "/push"

	payload := struct {
		DeviceKey string `json:"device_key"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		Group     string `json:"group"`
		URL       string `json:"url"`
	}{
		DeviceKey: key,
		Title:     "友链申请",
		Body:      blogFriendApplyBarkBody(in),
		Group:     "友链",
		URL:       blogFriendApplyTargetURL(baseURL),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		s.logWarn("friend apply: marshal bark payload failed", zap.Error(err))
		return
	}

	pushCtx, cancel := context.WithTimeout(ctx, barkPushTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(pushCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		s.logWarn("friend apply: build bark request failed", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		// No key and no applicant data in this line: only the transport failure.
		// 这一行不含 key、也不含申请人数据：只记传输失败。
		s.logWarn("friend apply: bark push failed", zap.Error(err))
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		s.logWarn("friend apply: bark push rejected", zap.Int("status", resp.StatusCode))
	}
}

func (s *friendApplyService) logWarn(msg string, fields ...zap.Field) {
	if s.logger != nil {
		s.logger.Warn(msg, fields...)
	}
}

// blogFriendApplyBarkBody builds the one-line-per-fact notification body.
//
// Bark renders the body in a fixed-width notification, so the total is clipped at 180 runes and
// each part gets its own budget first. Building the parts independently is what keeps a long URL
// from eating the message: a single clip over the joined string would spend the whole allowance
// on whichever field happened to be first.
//
// blogFriendApplyBarkBody 组装「一行一个事实」的通知正文。
//
// Bark 在固定宽度的通知里渲染正文，因此总长截到 180 个 rune，且每一部分先分到自己的预算。
// 分部分构造正是为了让一条长 URL 不会吃掉留言：对拼接后的整串做一次截断，
// 会把全部额度花在恰好排在最前的那个字段上。
func blogFriendApplyBarkBody(in *dto.BlogFriendApplyDTO) string {
	head := blogBarkClip(in.Name, 24)
	if in.Desc != "" {
		head += "｜" + blogBarkClip(in.Desc, 32)
	}
	lines := []string{head, blogBarkClip(in.URL, 100)}
	if in.Message != "" {
		lines = append(lines, "留言："+blogBarkClip(in.Message, 60))
	}
	return blogBarkClipTotal(strings.Join(lines, "\n"), 180)
}

// blogBarkClipTotal bounds the assembled body without touching its line breaks.
//
// It is separate from blogBarkClip because that helper flattens newlines: using it for the final
// pass would erase the very separators the body was just assembled around. Fields get the
// flattening clip; the joined body gets this one.
//
// blogBarkClipTotal 限制组装后的正文总长，但不碰它的换行。
//
// 它与 blogBarkClip 分开，是因为后者会压平换行：拿它做最后一遍，会把刚刚围绕其组装的分隔符
// 全部抹掉。字段用会压平的那个，拼接后的正文用这个。
func blogBarkClipTotal(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// blogBarkClip flattens newlines and tabs, then clips to max runes.
//
// Flattening first is the point: a YAML-style folded value in the note could carry a newline,
// and a notification with an unexpected line break reads as two facts. The ellipsis marks a
// clipped value instead of letting it end mid-word with no sign.
//
// blogBarkClip 先把换行与制表符压平，再截到 max 个 rune。
//
// 先压平是关键：笔记里折叠写的值可能带换行，而一条多出意外断行的通知会读成两个事实。
// 截断处补省略号，而不是让值在一个词中间毫无提示地结束。
func blogBarkClip(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// blogFriendApplyTargetURL normalises the notification's tap target.
//
// It exists as a tiny helper because the same join appears in the payload and in tests, and an
// empty base must not produce a URL that starts with "//" (which a phone reads as a protocol-
// relative host).
//
// blogFriendApplyTargetURL 规范化通知的点击目标。
//
// 抽成一个小函数，是因为同一个拼接同时出现在载荷与测试里，而空 base 不得产生以 "//" 开头的地址
// （手机会把它读成协议相对的主机名）。
func blogFriendApplyTargetURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "/blog/friends"
	}
	return base + "/blog/friends"
}

var _ FriendApplyService = (*friendApplyService)(nil)
