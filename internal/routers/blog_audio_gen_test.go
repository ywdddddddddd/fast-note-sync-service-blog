package routers

import (
	"strings"
	"testing"
)

// TestBlogAudioExtractNestedBlocks is the regression test for the highlight outage: Obsidian
// renders list items as <li><p>…</p></li>, and an extractor that stops at the <li> loses half
// the article. The expected text is the browser's buildArticleText output for the same body.
//
// TestBlogAudioExtractNestedBlocks 是这次高亮失效的回归测试：Obsidian 把列表项渲染成
// <li><p>…</p></li>，在 <li> 处停下的提取器会丢掉半个正文。期望值就是浏览器 buildArticleText
// 对同一段 HTML 的输出。
func TestBlogAudioExtractNestedBlocks(t *testing.T) {
	body := `<p>开头</p>
<ul>
  <li><p>列表里的段落</p></li>
  <li>列表直属文本</li>
</ul>
<blockquote><p>引用里的段落</p></blockquote>
<p>公式 <span class="math-inline">\rightarrow</span> 之后</p>
<pre><code>代码不能朗读</code></pre>`

	full, sentences := blogAudioExtract(body)
	want := "开头\n列表里的段落\n列表直属文本\n引用里的段落\n公式 之后"
	if full != want {
		t.Fatalf("extracted text mismatch:\n got: %q\nwant: %q", full, want)
	}
	if len(sentences) < 5 {
		t.Fatalf("sentences = %d, want at least 5: %#v", len(sentences), sentences)
	}
	for _, s := range sentences {
		if strings.Contains(s, "rightarrow") || strings.Contains(s, "代码不能朗读") {
			t.Errorf("excluded content leaked into transcript: %q", s)
		}
	}
}

// TestBlogAudioBuildTimingsEntityFragments covers the failure that stretched 1.203's timeline
// to 28 minutes for a 22-minute MP3: edge-tts reports ">" as SSML pieces ("&", "g", …), an exact
// search misses them, and the old sequential interpolator pushed every later sentence forward.
//
// TestBlogAudioBuildTimingsEntityFragments 覆盖 1.203 时间轴被拉到 28 分钟（音频只有 22 分钟）
// 的故障：edge-tts 把 ">" 报告成 SSML 碎片（"&"、"g"……），逐字搜索匹配不到，旧的顺序插值把
// 其后每一句都往后推。
func TestBlogAudioBuildTimingsEntityFragments(t *testing.T) {
	full := "甲乙丙丁 从 A>B 再到 C。\n随后是结尾。"
	sentences := []string{"甲乙丙丁 从 A>B 再到 C", "随后是结尾"}
	bounds := []blogAudioBound{
		{T: "甲乙丙丁", O: 100, D: 300},
		{T: "从", O: 500, D: 100},
		{T: "A", O: 700, D: 100},
		{T: "&", O: 800, D: 50},
		{T: "g", O: 850, D: 50},
		{T: "B", O: 950, D: 100},
		{T: "再到", O: 1100, D: 200},
		{T: "C", O: 1400, D: 100},
		{T: "随后", O: 1600, D: 200},
		{T: "是", O: 1900, D: 100},
		{T: "结尾", O: 2100, D: 300},
	}
	out, duration := blogAudioBuildTimings(full, sentences, bounds)
	if len(out) != 2 {
		t.Fatalf("sentences = %d", len(out))
	}
	if out[0].S != 100 || out[0].E != 1500 {
		t.Errorf("first sentence = [%d,%d], want [100,1500]", out[0].S, out[0].E)
	}
	if out[1].S < 1500 || out[1].E != 2400 {
		t.Errorf("second sentence = [%d,%d], want start>=1500 end=2400", out[1].S, out[1].E)
	}
	if duration != 2400 {
		t.Errorf("duration = %d, want last real word end 2400", duration)
	}
}

// TestBlogAudioBuildTimingsInterpolatesGap makes sure an unmatched sentence between two matched
// ones is placed inside the real interval, not appended after the previous estimate.
//
// TestBlogAudioBuildTimingsInterpolatesGap 确认两句已匹配之间的未匹配句落在真实区间内，而不是
// 接在上一句估算之后。
func TestBlogAudioBuildTimingsInterpolatesGap(t *testing.T) {
	full := "第一句。\n中间没有边界。\n最后一句。"
	sentences := []string{"第一句", "中间没有边界", "最后一句"}
	bounds := []blogAudioBound{
		{T: "第一句", O: 100, D: 300},
		{T: "最后一句", O: 3000, D: 300},
	}
	out, duration := blogAudioBuildTimings(full, sentences, bounds)
	if out[1].S < out[0].E || out[1].E > out[2].S {
		t.Errorf("middle sentence [%d,%d] outside [%d,%d]", out[1].S, out[1].E, out[0].E, out[2].S)
	}
	if duration != 3300 {
		t.Errorf("duration = %d, want 3300", duration)
	}
	if out[2].S != 3000 || out[2].E != 3300 {
		t.Errorf("last sentence = [%d,%d], want [3000,3300]", out[2].S, out[2].E)
	}
}
