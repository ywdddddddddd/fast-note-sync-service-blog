package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAIRouteHeadersAndThinkingLevelParse locks the two fields added for OpenCode Go: a route
// header map (whose value carries the ${uuid} placeholder) and a thinking level that overrides the
// boolean switch. Both are load-bearing: without the header the upstream answers HTTP 400
// MissingSessionID, and without the object form it ignores enable_thinking and returns an empty
// content field. A yaml tag typo here would reproduce exactly those two failures in production
// with no local symptom at all, which is why this is asserted rather than assumed.
//
// TestAIRouteHeadersAndThinkingLevelParse 锁住为 OpenCode Go 新增的两个字段：线路请求头表
// （值里带 ${uuid} 占位符）与覆盖布尔开关的 thinking 档位。两者都是承重的：
// 没有头，上游回 HTTP 400 MissingSessionID；没有对象形态，它忽略 enable_thinking 并返回空 content。
// 这里的 yaml tag 写错，会在生产上精确复现这两种故障，而本地毫无症状——所以要断言，不能假设。
func TestAIRouteHeadersAndThinkingLevelParse(t *testing.T) {
	const doc = `
ai:
  enabled: true
  routes:
    - name: opencode-go-deepseek-v4.1-flash
      base-url: https://opencode.ai/inference/go/v1
      api-key: oc_sk_test
      model: deepseek-v4.1-flash
      max-tokens: 8000
      enable-thinking: true
      thinking-level: max
      headers:
        x-opencode-session: "${uuid}"
`
	var root struct {
		AI AISettings `yaml:"ai"`
	}
	if err := yaml.Unmarshal([]byte(doc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(root.AI.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(root.AI.Routes))
	}
	r := root.AI.Routes[0]

	if r.ThinkingLevel != "max" {
		t.Errorf("thinking-level = %q, want max", r.ThinkingLevel)
	}
	if got := r.Headers["x-opencode-session"]; got != "${uuid}" {
		t.Errorf("header x-opencode-session = %q, want the literal ${uuid} placeholder", got)
	}
	if got := r.AIRouteMaxTokens(); got != 8000 {
		t.Errorf("max-tokens = %d, want 8000 (a long reasoning trace must not empty the answer)", got)
	}
	if !r.AIRouteThinking() {
		t.Error("enable-thinking should be true")
	}
	if !r.AIRouteEnabled() {
		t.Error("route should default to enabled")
	}
}
