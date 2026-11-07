package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/haierkeys/fast-note-sync-service/internal/config"
	"github.com/haierkeys/fast-note-sync-service/internal/domain"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"go.uber.org/zap"
)

// fakeFriendApplyRepo records what the service tried to store.
// fakeFriendApplyRepo 记录服务尝试存储的内容。
type fakeFriendApplyRepo struct {
	rows []*domain.FriendApply
	err  error
}

func (f *fakeFriendApplyRepo) Create(_ context.Context, apply *domain.FriendApply) error {
	if f.err != nil {
		return f.err
	}
	f.rows = append(f.rows, apply)
	return nil
}

// TestValidateBlogFriendApply covers the rules a hand-written POST can violate.
//
// The browser's maxlength and type=input are conveniences; the service is the boundary. Each
// case names one boundary so a failure says which rule moved.
//
// TestValidateBlogFriendApply 覆盖手写 POST 能违反的规则。
//
// 浏览器的 maxlength 与 input type 是便利，服务层才是边界。每条用例只点名一条边界，
// 这样失败时能直接看出是哪条规则变了。
func TestValidateBlogFriendApply(t *testing.T) {
	valid := dto.BlogFriendApplyDTO{
		Name: "小倪的博客", URL: "https://ni.example.com", Avatar: "https://ni.example.com/a.png",
		Desc: "记录", Email: "ni@example.com", Feed: "https://ni.example.com/rss.xml", Message: "换友链",
	}
	with := func(mut func(*dto.BlogFriendApplyDTO)) *dto.BlogFriendApplyDTO {
		in := valid
		mut(&in)
		return &in
	}

	cases := []struct {
		name    string
		in      *dto.BlogFriendApplyDTO
		wantErr string
	}{
		{name: "合法", in: &valid},
		{name: "站名为空", in: with(func(v *dto.BlogFriendApplyDTO) { v.Name = "   " }), wantErr: "站名不能为空"},
		{name: "站名超长", in: with(func(v *dto.BlogFriendApplyDTO) { v.Name = strings.Repeat("名", 61) }), wantErr: "站名过长"},
		{name: "链接为空", in: with(func(v *dto.BlogFriendApplyDTO) { v.URL = "" }), wantErr: "链接不能为空"},
		{name: "链接 scheme 非法", in: with(func(v *dto.BlogFriendApplyDTO) { v.URL = "javascript:alert(1)" }), wantErr: "http/https"},
		{name: "链接无主机", in: with(func(v *dto.BlogFriendApplyDTO) { v.URL = "https://" }), wantErr: "http/https"},
		{name: "链接超长", in: with(func(v *dto.BlogFriendApplyDTO) { v.URL = "https://x.example.com/" + strings.Repeat("a", 300) }), wantErr: "链接过长"},
		{name: "头像 scheme 非法", in: with(func(v *dto.BlogFriendApplyDTO) { v.Avatar = "data:image/png;base64,AA" }), wantErr: "头像链接需为"},
		{name: "订阅 scheme 非法", in: with(func(v *dto.BlogFriendApplyDTO) { v.Feed = "file:///etc/passwd" }), wantErr: "订阅链接需为"},
		{name: "描述超长", in: with(func(v *dto.BlogFriendApplyDTO) { v.Desc = strings.Repeat("字", 121) }), wantErr: "描述过长"},
		{name: "邮箱无 @", in: with(func(v *dto.BlogFriendApplyDTO) { v.Email = "not-an-email" }), wantErr: "邮箱格式不正确"},
		{name: "邮箱超长", in: with(func(v *dto.BlogFriendApplyDTO) { v.Email = strings.Repeat("a", 115) + "@x.com" }), wantErr: "邮箱过长"},
		{name: "留言超长", in: with(func(v *dto.BlogFriendApplyDTO) { v.Message = strings.Repeat("留", 301) }), wantErr: "留言过长"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBlogFriendApply(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("合法输入被拒: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("错误 = %v, 期望包含 %q", err, tc.wantErr)
			}
		})
	}

	// 可选字段整块留空必须合法：表单里除站名与链接外都不是必填。
	sparse := dto.BlogFriendApplyDTO{Name: "只有名字", URL: "https://x.example.com"}
	if err := ValidateBlogFriendApply(&sparse); err != nil {
		t.Fatalf("只填必填项应合法, got %v", err)
	}

	// 修剪。带首尾空白的合法输入应被接受，而且入库值是修剪后的。
	trimmed := dto.BlogFriendApplyDTO{
		Name: "  名字  ", URL: " https://x.example.com ", Avatar: " https://x.example.com/a.png ",
		Desc: " 描述 ", Email: " a@b.com ", Feed: " https://x.example.com/rss.xml ", Message: " 留言 ",
	}
	if err := ValidateBlogFriendApply(&trimmed); err != nil {
		t.Fatalf("带空白输入应合法: %v", err)
	}
	if trimmed.Name != "名字" || trimmed.URL != "https://x.example.com" || trimmed.Message != "留言" {
		t.Fatalf("字段未被修剪: %#v", trimmed)
	}
}

// TestBlogFriendApplyBarkBody pins the notification shape and the two clips that matter.
//
// Bark renders this in a fixed-width notification, so the total is bounded in runes. The URL is
// clipped before the message so one long address cannot swallow the human-written part.
//
// TestBlogFriendApplyBarkBody 固定通知形状与两处截断。
//
// Bark 在固定宽度通知里渲染这段文本，因此总量按 rune 限制。URL 先于留言截断，
// 使一条长地址不能吞掉人写的那部分。
func TestBlogFriendApplyBarkBody(t *testing.T) {
	in := &dto.BlogFriendApplyDTO{
		Name: "小倪", Desc: "记录生活", URL: "https://ni.example.com", Message: "想换友链\n第二行",
	}
	body := blogFriendApplyBarkBody(in)
	for _, want := range []string{"小倪｜记录生活", "https://ni.example.com", "留言：想换友链 第二行"} {
		if !strings.Contains(body, want) {
			t.Errorf("body 缺少 %q: %q", want, body)
		}
	}
	if strings.Contains(body, "\n第二行") {
		t.Errorf("留言中的换行未被压平: %q", body)
	}
	// 三个事实各占一行：拼装后的总长截断不得把换行一起压平。
	if !strings.Contains(body, "小倪｜记录生活\nhttps://ni.example.com\n留言：") {
		t.Errorf("body 未保留换行: %q", body)
	}
	if n := utf8.RuneCountInString(body); n > 180 {
		t.Errorf("body 超过 180 rune: %d", n)
	}

	long := &dto.BlogFriendApplyDTO{
		Name:    strings.Repeat("名", 40),
		Desc:    strings.Repeat("述", 60),
		URL:     "https://x.example.com/" + strings.Repeat("p", 200),
		Message: strings.Repeat("留", 200),
	}
	clipped := blogFriendApplyBarkBody(long)
	if n := utf8.RuneCountInString(clipped); n > 180 {
		t.Errorf("长字段 body 超过 180 rune: %d", n)
	}
	if !strings.Contains(clipped, "…") {
		t.Errorf("截断处没有省略号: %q", clipped)
	}
	if !strings.Contains(clipped, "留言：") {
		t.Errorf("长 URL 不应吞掉留言行: %q", clipped)
	}

	if got := blogFriendApplyTargetURL("https://x.example.com/"); got != "https://x.example.com/blog/friends" {
		t.Errorf("target url = %q", got)
	}
	if got := blogFriendApplyTargetURL(""); got != "/blog/friends" {
		t.Errorf("空 base 的 target url = %q", got)
	}
}

// TestApplyFriendStoresWithoutKey covers the default configuration: an empty bark-key stores the
// application and makes no outbound request at all.
//
// TestApplyFriendStoresWithoutKey 覆盖默认配置：bark-key 为空时只入库，不发起任何外部请求。
func TestApplyFriendStoresWithoutKey(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &fakeFriendApplyRepo{}
	svc := NewFriendApplyService(repo, zap.NewNop(), &config.BlogConfig{BarkKey: "", BarkServer: srv.URL})
	in := &dto.BlogFriendApplyDTO{Name: "小倪", URL: "https://ni.example.com", Message: "换友链"}
	if err := svc.ApplyFriend(context.Background(), 7, in, "203.0.113.9", "https://yaowendao.asia"); err != nil {
		t.Fatalf("ApplyFriend: %v", err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("入库行数 = %d, want 1", len(repo.rows))
	}
	row := repo.rows[0]
	if row.UID != 7 || row.Name != "小倪" || row.RemoteIP != "203.0.113.9" || row.Status != domain.FriendApplyStatusNew {
		t.Fatalf("入库行字段不对: %#v", row)
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Fatalf("bark-key 为空时不应推送，实际请求 %d 次", got)
	}
}

// TestApplyFriendPushesCorrectPayload verifies the JSON Bark actually receives.
//
// The device key travels in the body, never the URL, and the tap target is the friends page.
// The response is 500 from the fake server, which this test also uses to prove degradation: the
// application is already stored and ApplyFriend still returns nil.
//
// TestApplyFriendPushesCorrectPayload 验证 Bark 实际收到的 JSON。
//
// 设备 key 放在请求体里、绝不在 URL 中，点击目标是友链页。假服务器故意回 500，本用例同时
// 证明降级：申请已经入库，ApplyFriend 仍然返回 nil。
func TestApplyFriendPushesCorrectPayload(t *testing.T) {
	type payload struct {
		DeviceKey string `json:"device_key"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		Group     string `json:"group"`
		URL       string `json:"url"`
	}
	got := make(chan payload, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/push" {
			t.Errorf("path = %q, want /push", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var p payload
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Errorf("bark body 不是 JSON: %v", err)
		}
		got <- p
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	repo := &fakeFriendApplyRepo{}
	svc := NewFriendApplyService(repo, zap.NewNop(), &config.BlogConfig{BarkKey: "test-device-key", BarkServer: srv.URL})
	in := &dto.BlogFriendApplyDTO{
		Name: "小倪", URL: "https://ni.example.com", Desc: "记录生活", Message: "换友链",
	}
	if err := svc.ApplyFriend(context.Background(), 1, in, "203.0.113.10", "https://yaowendao.asia/"); err != nil {
		t.Fatalf("推送失败不得让 ApplyFriend 失败: %v", err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("推送失败时也必须已入库, 行数 = %d", len(repo.rows))
	}

	p := <-got
	if p.DeviceKey != "test-device-key" {
		t.Errorf("device_key = %q", p.DeviceKey)
	}
	if p.Title != "友链申请" || p.Group != "友链" {
		t.Errorf("title/group = %q/%q", p.Title, p.Group)
	}
	if p.URL != "https://yaowendao.asia/blog/friends" {
		t.Errorf("click url = %q", p.URL)
	}
	if !strings.Contains(p.Body, "小倪｜记录生活") || !strings.Contains(p.Body, "https://ni.example.com") ||
		!strings.Contains(p.Body, "留言：换友链") {
		t.Errorf("body = %q", p.Body)
	}
}

// TestApplyFriendStorageFailure verifies a database error is returned and no push is attempted.
//
// The order matters: if the row cannot be stored, there is nothing to announce, and pushing
// anyway would tell the owner about an application the service cannot show them.
//
// TestApplyFriendStorageFailure 验证数据库错误会返回，且不尝试推送。
//
// 顺序很重要：行存不下来就没有东西可通知，照推只会告诉站主一条服务自己都拿不出来的申请。
func TestApplyFriendStorageFailure(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	repo := &fakeFriendApplyRepo{err: errors.New("disk full")}
	svc := NewFriendApplyService(repo, zap.NewNop(), &config.BlogConfig{BarkKey: "k", BarkServer: srv.URL})
	in := &dto.BlogFriendApplyDTO{Name: "小倪", URL: "https://ni.example.com"}
	if err := svc.ApplyFriend(context.Background(), 1, in, "203.0.113.11", ""); err == nil {
		t.Fatal("存储失败应返回错误")
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Fatalf("存储失败后不应推送，实际请求 %d 次", got)
	}
}

// TestApplyFriendRejectsInvalidBeforeStorage verifies validation precedes the write and the push.
// TestApplyFriendRejectsInvalidBeforeStorage 验证校验先于写库与推送。
func TestApplyFriendRejectsInvalidBeforeStorage(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	repo := &fakeFriendApplyRepo{}
	svc := NewFriendApplyService(repo, zap.NewNop(), &config.BlogConfig{BarkKey: "k", BarkServer: srv.URL})
	in := &dto.BlogFriendApplyDTO{Name: "", URL: "https://ni.example.com"}
	if err := svc.ApplyFriend(context.Background(), 1, in, "", ""); err == nil {
		t.Fatal("非法输入应返回错误")
	}
	if len(repo.rows) != 0 || atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("非法输入不得入库或推送: rows=%d hits=%d", len(repo.rows), hits)
	}
}
