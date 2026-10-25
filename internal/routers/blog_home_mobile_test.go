package routers

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// The mobile home page puts one status line where the seven-cell dashboard used to be, and it
// hides two of the four metadata fragments on each card. Both decisions live in CSS while the
// content lives in this template, so the two can drift apart silently: rename a class here and
// the stylesheet keeps matching nothing, with no error anywhere.
//
// These assertions are the joint. They hold the class names the mobile rules select on, and they
// hold the compact word count that makes the strip short enough to fit.
//
// 移动端首页用一行状态条取代了七格仪表盘，并在每张卡片上隐去四段元信息里的两段。
// 这两个决定都在 CSS 里，而内容在这个模板里，于是它们可以安静地脱钩：在这里改掉一个类名，
// 样式表就继续匹配一个不存在的东西，而任何地方都不会报错。
//
// 下面几条断言就是那个接缝。它们钉住移动端规则所选择的类名，也钉住让那一行短到塞得下的
// 压缩字数。
func TestHomeCarriesMobileStatusAndCardHooks(t *testing.T) {
	var buf bytes.Buffer
	err := blogHomeTmpl.Execute(&buf, blogHomeView{
		SiteName:    "问道漫步",
		Description: "测试",
		Stats: &dto.BlogStatsDTO{
			Notes: 146, Words: 828335, Links: 873,
			Tags: 0, Folders: 23, AvgWords: 5673,
			Updated:   "2026-10-24",
			WordsText: "82.8万",
		},
		Entries: []*dto.BlogEntryDTO{{
			Slug: "x", Title: "牙的发育", Summary: "摘要正文",
			CreatedAt: "2026-10-24", Words: 8919,
			LinksOut: 16, LinksIn: 14, Vault: "口腔",
		}},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	// Both variants have to be present. The wide one carries the raw six-digit count for the
	// desktop subtitle; the compact one is what the narrow strip can fit.
	// 两个版本都必须存在：宽的那份给桌面副标题，带原始六位数字；压缩的那份才塞得进窄屏那一行。
	for _, want := range []string{
		`class="bs-wide"`, "828335 字",
		`class="bs-compact"`, "82.8万字 · 873 双链",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("home lacks %q", want)
		}
	}

	// The dashboard is what the narrow layout hides, so its hook must survive template edits.
	// 仪表盘是窄屏要收起来的东西，它挂的标记不能在改模板时丢掉。
	if !strings.Contains(out, `class="card card-pad board"`) {
		t.Fatalf("dashboard lost its .board hook; the mobile rule would no longer find it")
	}

	// Every metadata fragment the mobile rules reorder or hide needs its own class.
	// 移动端要重排或隐去的每一段元信息都需要自己的类名。
	for _, want := range []string{
		`class="m-date"`, `class="m-words"`, `class="m-link"`, `class="m-cat"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("card metadata lacks %q", want)
		}
	}
}
