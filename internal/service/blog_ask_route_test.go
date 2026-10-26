package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// OpenCode Go rejects any request without x-opencode-session (HTTP 400 MissingSessionID since
// 2026-09-06), and ignores enable_thinking entirely — it needs thinking:{type:...} or it answers
// with an empty content field and finish_reason=length. These tests lock both behaviours down,
// because neither failure announces itself: one is a 400, the other is a silent blank answer.
//
// OpenCode Go 会拒绝任何缺少 x-opencode-session 的请求（自 2026-09-06 起 HTTP 400 MissingSessionID），
// 并且完全忽略 enable_thinking——它要 thinking:{type:...}，否则返回空 content 与 finish_reason=length。
// 这两条都锁进测试，因为它们的失败方式都不会自我暴露：一个是 400，另一个是静默的空答案。

var askUUIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestAskExpandHeaderValueSubstitutesUUID(t *testing.T) {
	first := askExpandHeaderValue("${uuid}")
	second := askExpandHeaderValue("${uuid}")
	if !askUUIDRe.MatchString(first) {
		t.Fatalf("expected a UUIDv4, got %q", first)
	}
	if first == second {
		t.Fatal("each call must produce a fresh id; upstream routes on it")
	}
	if got := askExpandHeaderValue("static-value"); got != "static-value" {
		t.Fatalf("static value must pass through unchanged, got %q", got)
	}
	if got := askExpandHeaderValue("prefix-${uuid}-suffix"); !strings.HasPrefix(got, "prefix-") ||
		!strings.HasSuffix(got, "-suffix") || !askUUIDRe.MatchString(got[7:len(got)-7]) {
		t.Fatalf("placeholder must be replaced in place, got %q", got)
	}
}

func TestAskRouteUsesThinkingObject(t *testing.T) {
	cases := map[string]bool{
		"https://token.sensenova.cn/v1":       true,
		"https://api.sensenova.example/v1":    true,
		"https://opencode.ai/inference/go/v1": true,
		"https://opencode.ai/inference/v1":    true,
		"https://api.siliconflow.cn/v1":       false,
		"https://api.openai.com/v1":           false,
		"":                                    false,
	}
	for url, want := range cases {
		if got := askRouteUsesThinkingObject(url); got != want {
			t.Errorf("askRouteUsesThinkingObject(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestAskRouteThinkingMode(t *testing.T) {
	cases := []struct {
		name string
		r    AskRoute
		want string
	}{
		// 白名单内的三个取值原样透传（上游接受的枚举）。
		{"enabled passes through", AskRoute{ThinkingLevel: "enabled", EnableThinking: false}, "enabled"},
		{"adaptive passes through", AskRoute{ThinkingLevel: "adaptive"}, "adaptive"},
		{"disabled passes through", AskRoute{ThinkingLevel: "disabled", EnableThinking: true}, "disabled"},
		{"level trimmed and lowercased", AskRoute{ThinkingLevel: "  Enabled  "}, "enabled"},

		// 白名单外的取值必须降级，而不是原样转发。
		// 2026-10-26：配置里写的是 max，而上游已把枚举改成 adaptive/enabled/disabled，
		// 原样转发让整条线路 422，问答彻底不可用。
		{"unknown level with thinking on falls back to enabled",
			AskRoute{ThinkingLevel: "max", EnableThinking: true}, "enabled"},
		{"unknown level with thinking off falls back to disabled",
			AskRoute{ThinkingLevel: "max", EnableThinking: false}, "disabled"},
		{"unknown high with thinking off falls back to disabled",
			AskRoute{ThinkingLevel: "high", EnableThinking: false}, "disabled"},

		{"boolean true without level", AskRoute{EnableThinking: true}, "enabled"},
		{"boolean false without level", AskRoute{}, "disabled"},
	}
	for _, c := range cases {
		if got := askRouteThinkingMode(c.r); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestAskCallRouteSendsConfiguredHeaders proves the header actually reaches the wire, and that a
// configured header can override a default one. Unit tests on the helper alone would not catch a
// wiring mistake between config and request.
//
// TestAskCallRouteSendsConfiguredHeaders 证明自定义头真的上了网线，并且可以覆盖默认头。
// 只测辅助函数发现不了「配置到请求之间」的接线错误。
func TestAskCallRouteSendsConfiguredHeaders(t *testing.T) {
	var session, accept string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session = r.Header.Get("x-opencode-session")
		accept = r.Header.Get("Accept")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	r := AskRoute{
		Name:    "opencode-test",
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Model:   "deepseek-v4.1-flash",
		Headers: map[string]string{
			"x-opencode-session": "${uuid}",
			"Accept":             "application/json",
		},
	}
	var got strings.Builder
	if _, err := (&shareService{}).askCallRoute(context.Background(), r,
		[]upstreamMessage{{Role: "user", Content: "hi"}},
		func(s string) bool { got.WriteString(s); return true }, nil); err != nil {
		t.Fatalf("askCallRoute: %v", err)
	}

	if !askUUIDRe.MatchString(session) {
		t.Fatalf("x-opencode-session must carry a UUIDv4, got %q", session)
	}
	if accept != "application/json" {
		t.Fatalf("configured header must override the default, got Accept=%q", accept)
	}
	if got.String() != "ok" {
		t.Fatalf("stream content not delivered, got %q", got.String())
	}
	if body["model"] != "deepseek-v4.1-flash" {
		t.Fatalf("model not sent, got %v", body["model"])
	}
}

// TestAskCallRouteBooleanThinkingForFlagUpstreams keeps the non-object path intact: OpenCode needs
// the thinking object, but SiliconFlow's Qwen responds to the boolean and must not be sent both.
//
// TestAskCallRouteBooleanThinkingForFlagUpstreams 保住非对象那条路径：OpenCode 要 thinking 对象，
// 而硅基的 Qwen 认布尔开关，且不能两个都发。
func TestAskCallRouteBooleanThinkingForFlagUpstreams(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
	}))
	defer srv.Close()

	r := AskRoute{Name: "flag", BaseURL: srv.URL, APIKey: "k", Model: "Qwen/Qwen3.5-4B", EnableThinking: false}
	if _, err := (&shareService{}).askCallRoute(context.Background(), r,
		[]upstreamMessage{{Role: "user", Content: "hi"}},
		func(string) bool { return true }, nil); err != nil {
		t.Fatalf("askCallRoute: %v", err)
	}
	if v, ok := body["enable_thinking"]; !ok || v != false {
		t.Fatalf("flag-style upstream must receive enable_thinking=false, got %v", body["enable_thinking"])
	}
	if _, ok := body["thinking"]; ok {
		t.Fatal("flag-style upstream must not receive a thinking object")
	}
}
