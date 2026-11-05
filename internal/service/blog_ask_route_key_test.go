package service

import "testing"

// TestAskRouteKeyIsPerUpstream 守一条生产上真实发生过的故障。
//
// 本站有四条线路的展示名都是「GPT-6 Astra」（OpenCode Go、两个商汤 key、一个 Qwen 兜底）。
// askRouteKey 原先按 Name 取键，于是这四条共用一个冷却条目：OpenCode Go 回 429 时，
// 冷却的是「GPT-6 Astra」，两个商汤兜底与 Qwen 在轮到之前就被 askRouteCooling 跳过。
// 一次上游抖动就让整条故障转移链塌掉，只剩 Fable 5.5。
//
// 这条测试钉住「同名的不同上游必须得到不同的键」，以及「同一条线路跨调用保持同一个键」。
func TestAskRouteKeyIsPerUpstream(t *testing.T) {
	opencode := AskRoute{Name: "GPT-6 Astra", BaseURL: "https://opencode.ai/inference/go/v1", Model: "deepseek-v4.1-flash"}
	sensenova := AskRoute{Name: "GPT-6 Astra", BaseURL: "https://token.sensenova.cn/v1", Model: "deepseek-flash"}
	silicon := AskRoute{Name: "GPT-6 Astra", BaseURL: "https://api.siliconflow.cn/v1", Model: "Qwen/Qwen3.5-4B"}

	keys := map[string]AskRoute{
		askRouteKey(opencode):  opencode,
		askRouteKey(sensenova): sensenova,
		askRouteKey(silicon):   silicon,
	}
	if len(keys) != 3 {
		t.Fatalf("三条同名的不同上游只得到 %d 个互不相同的键，冷却会互相牵连", len(keys))
	}

	// 同一条线路重复取键必须稳定，否则冷却根本记不住。
	if askRouteKey(opencode) != askRouteKey(opencode) {
		t.Error("同一条线路两次取键不一致")
	}

	// 两个商汤 key 共用 base-url 与 model，按身份它们是同一条上游 —— 这不是缺陷：
	// 它们确实指向同一个服务，共享节流是正确的。
	if askRouteKey(sensenova) == askRouteKey(opencode) {
		t.Error("不同上游得到了相同的键")
	}
}

// TestAskRouteKeyFallsBackWithoutBaseURL 守住手工构造的无 base-url 线路仍能得到一个键。
func TestAskRouteKeyFallsBackWithoutBaseURL(t *testing.T) {
	if got := askRouteKey(AskRoute{Name: "only-name"}); got != "only-name" {
		t.Errorf("无 base-url 时应退回 Name，得到 %q", got)
	}
	if got := askRouteKey(AskRoute{Model: "only-model"}); got != "only-model" {
		t.Errorf("既无 base-url 也无 Name 时应退回 Model，得到 %q", got)
	}
}
