package api_router

import (
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/config"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }
func ip(i int) *int       { return &i }

// TestMergeAIRoutesKeepsHiddenFields 锁住一个曾经让全站问答挂掉的行为：
// 保存设置页回传的线路表时，页面上看不见的字段必须原样保留。
//
// 2026-10-26：OpenCode 线路的 headers（x-opencode-session）被清成 {}，
// 网关回 400 MissingSessionID。api-key 与 base-url 都还在，配置文件看上去像没被动过，
// 所以很难往"保存设置"上想。
func TestMergeAIRoutesKeepsHiddenFields(t *testing.T) {
	old := []config.AIRouteSetting{
		{
			Name:           "GPT-6 Astra",
			BaseURL:        "https://opencode.ai/inference/go/v1",
			APIKey:         "oc_sk_x",
			Model:          "deepseek-v4.1-flash",
			Headers:        map[string]string{"x-opencode-session": "${uuid}"},
			EnableThinking: bp(true),
			ThinkingLevel:  "enabled",
			MaxTokens:      ip(8000),
			Retries:        ip(1),
		},
	}

	// 设置页只回传它认识的字段，而且模型名被改了一处。
	in := []dto.AIRouteRequest{{
		Name:    sp("GPT-6 Astra"),
		BaseURL: sp("https://opencode.ai/inference/go/v1"),
		APIKey:  sp("oc_sk_x"),
		Model:   sp("deepseek-v4.2-flash"),
	}}

	got := mergeAIRoutes(old, in)
	if len(got) != 1 {
		t.Fatalf("线路数 = %d，期望 1", len(got))
	}
	r := got[0]

	if r.Model != "deepseek-v4.2-flash" {
		t.Errorf("Model 应被请求覆盖，得到 %q", r.Model)
	}
	if r.Headers["x-opencode-session"] != "${uuid}" {
		t.Errorf("Headers 被清空或改写：%v（这正是 MissingSessionID 的成因）", r.Headers)
	}
	if r.ThinkingLevel != "enabled" {
		t.Errorf("ThinkingLevel 丢失：%q", r.ThinkingLevel)
	}
	if r.EnableThinking == nil || !*r.EnableThinking {
		t.Errorf("EnableThinking 丢失：%v", r.EnableThinking)
	}
	if r.MaxTokens == nil || *r.MaxTokens != 8000 {
		t.Errorf("MaxTokens 丢失：%v", r.MaxTokens)
	}
	if r.Retries == nil || *r.Retries != 1 {
		t.Errorf("Retries 丢失：%v", r.Retries)
	}
}

// TestMergeAIRoutesAddsNewRoute 新增线路（配置里没有同 base-url）时不能被旧值污染。
func TestMergeAIRoutesAddsNewRoute(t *testing.T) {
	old := []config.AIRouteSetting{{
		BaseURL: "https://a.example/v1",
		APIKey:  "key-a",
		Headers: map[string]string{"x-a": "1"},
	}}

	in := []dto.AIRouteRequest{
		{BaseURL: sp("https://a.example/v1"), APIKey: sp("key-a")},
		{BaseURL: sp("https://b.example/v1"), APIKey: sp("key-b"), Model: sp("m")},
	}

	got := mergeAIRoutes(old, in)
	if len(got) != 2 {
		t.Fatalf("线路数 = %d，期望 2", len(got))
	}
	if got[0].Headers["x-a"] != "1" {
		t.Errorf("老线路的头丢了：%v", got[0].Headers)
	}
	if len(got[1].Headers) != 0 {
		t.Errorf("新线路不该继承别的线路的头：%v", got[1].Headers)
	}
	if got[1].APIKey != "key-b" {
		t.Errorf("新线路 api-key = %q", got[1].APIKey)
	}
}

// TestMergeAIRoutesDropsEmptyRow 空行仍应被丢弃，避免留下永远不会成功的线路。
func TestMergeAIRoutesDropsEmptyRow(t *testing.T) {
	got := mergeAIRoutes(nil, []dto.AIRouteRequest{{}, {BaseURL: sp("https://x.example/v1")}})
	if len(got) != 1 {
		t.Fatalf("线路数 = %d，期望 1（空行应被丢弃）", len(got))
	}
}
