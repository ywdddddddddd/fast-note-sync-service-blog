package routers

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestInkMatchesSample 是这一轮返工的护栏。
//
// 站主的原话是「书写不跟手…你就不能抄源码吗」。前几版不跟手的根因，是我自己重写了字形路径：
// 笔的坐标来自 A 组曲线，被揭示的墨来自 B 组曲线，两者越写越远。
//
// 这条测试直接解析站主给的示例文件，逐段比对坐标与权重，任何擅自改动都会失败。
// 它要求示例文件存在；不存在时跳过（CI 里没有那个文件，本地开发机上有）。
func TestInkMatchesSample(t *testing.T) {
	candidates := []string{
		filepath.Join("..", "..", "..", "..", "_local-test", "sample-blog-loading.html"),
		`C:\Users\12917\Downloads\博客加载.html`,
	}
	var raw string
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil {
			raw = string(b)
			break
		}
	}
	if raw == "" {
		t.Skip("找不到示例文件，跳过与源码的比对")
	}

	js := blogWarpJS()

	// ── 1. 每一段字形坐标必须与示例逐字相同 ───────────────────────────────
	// 示例里 s 与 B 各含两条子路径，本脚本把它们拆开了，所以比对的是「子路径」而不是「id」。
	for _, id := range []string{"W", "endao", "log", "Underline"} {
		sample := samplePath(t, raw, "mask_"+id)
		if !strings.Contains(js, sample) {
			t.Errorf("%s 的路径与示例不一致。\n示例: %s", id, sample)
		}
	}

	// s 与 B 拆成两段后，每一段都必须是示例那条 path 的一个子路径，原文出现。
	for _, pair := range [][2]string{
		{"s1", "M 698,198 C 696,208 692,220 687,228"},
		{"s2", "M 698,280 C 708,252 722,238 732,240 C 738,242 738,252 730,260 C 720,272 718,282 724,290 C 730,298 742,294 750,286"},
		{"B1", "M 798,165 C 794,180 782,235 778,295 C 776,306 768,308 762,300"},
		{"B2", "M 788,178 C 804,162 842,162 848,195 C 852,216 836,234 814,240 C 840,244 860,262 858,285 C 856,304 834,310 812,306 C 796,303 794,294 804,290 C 814,286 828,292 836,282 C 842,272 832,252 806,248"},
	} {
		if !strings.Contains(js, pair[1]) {
			t.Errorf("%s 的路径缺失或与示例的子路径不字面相同:\n%s", pair[0], pair[1])
		}
	}

	// 示例里 s 与 B 是单条含两条子路径的 path，本脚本必须已经拆开。
	if strings.Contains(js, "M 698,198 C 696,208 692,220 687,228 M 698,280") {
		t.Error("s 仍是一条含两个子路径的 path：笔会在接缝处跳")
	}
	if strings.Contains(js, "M 798,165 C 794,180 782,235 778,295 C 776,306 768,308 762,300 M 788,178") {
		t.Error("B 仍是一条含两个子路径的 path：笔会在接缝处跳")
	}

	// ── 2. 权重必须与示例成比例 ─────────────────────────────────────────
	// 示例: W 1.25 / endao 2.70 / s 0.95 / B 1.45 / log 2.20 / Underline 1.40
	for _, c := range []struct {
		id     string
		weight string
	}{
		{"W", "weight: 1.25"},
		{"endao", "weight: 2.70"},
		{"log", "weight: 2.20"},
		{"Underline", "weight: 1.40"},
	} {
		if !strings.Contains(js, c.weight) {
			t.Errorf("%s 的权重与示例不符，应为 %s", c.id, c.weight)
		}
	}
	// s 拆成 0.17 + 0.78 = 0.95；B 拆成 0.42 + 1.03 = 1.45。
	for _, w := range []string{"weight: 0.17", "weight: 0.78", "weight: 0.42", "weight: 1.03"} {
		if !strings.Contains(js, w) {
			t.Errorf("拆分后的权重缺失: %s", w)
		}
	}

	// ── 3. 基准时长与缓动取自示例 ───────────────────────────────────────
	if !strings.Contains(js, "var PASS_MS = 2400;") {
		t.Error("基准时长不是示例的 2400ms")
	}
	if !strings.Contains(js, "t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2") {
		t.Error("缓动函数与示例不一致")
	}

	// ── 4. 必须是蒙版揭示，而不是逐段描边 ───────────────────────────────
	//
	// 示例用 26 单位宽的白色蒙版描边揭示完整的艺术字；我早先的重写直接给艺术字做 dash 动画，
	// 于是笔在画一条细线，而字形本该是粗细变化的墨 —— 这也是「不跟手」的一部分。
	if !strings.Contains(js, "stroke-width': '26'") {
		t.Error("蒙版描边不再是 26 单位：揭示效果与示例不同")
	}
	if !strings.Contains(js, "mask: 'url(#fnsMask)'") {
		t.Error("字形没有被蒙版遮罩：变成了逐段描边而不是揭示")
	}
	// 每一段都只能有一条子路径。
	if n := strings.Count(js, "id: 's1'"); n != 1 {
		t.Errorf("s1 段应恰好定义一次，实际 %d", n)
	}

	// ── 5. 钢笔结构与示例一致 ───────────────────────────────────────────
	for _, frag := range []string{
		"M -15,-130 L 15,-130 L 13.5,-72 L -13.5,-72 Z", // 握位
		"x: '-14.5', y: '-76', width: '29', height: '5', rx: '1'", // 金环
		"M 0,0 C 2.8,-7 8.5,-22 12,-34",                  // 笔尖外形起点
		"cx: '0', cy: '-35', r: '2.8'",                   // 呼吸孔
		"transform: 'scale(0.85)'",                        // 笔身缩放
	} {
		if !strings.Contains(js, frag) {
			t.Errorf("钢笔结构缺少示例中的片段: %s", frag)
		}
	}
	if !strings.Contains(js, "translate(-300, -300)") {
		t.Error("钢笔初始位移与示例不一致")
	}
}

// samplePath 从示例 HTML 里取出 id 对应 path 的 d 属性。
func samplePath(t *testing.T, raw, id string) string {
	t.Helper()
	re := regexp.MustCompile(`id="` + regexp.QuoteMeta(id) + `"\s+d="([^"]+)"`)
	m := re.FindStringSubmatch(raw)
	if m == nil {
		t.Fatalf("示例里找不到 %s 的 d 属性", id)
	}
	// 示例的 d 属性跨行书写，折叠成单行并与脚本里的写法对齐。
	fields := strings.Fields(m[1])
	return strings.Join(fields, " ")
}
