package routers

import (
	"bytes"
	"strings"
	"testing"
)

// TestMathTemplateLoadsKatex guards the include that actually makes formulas render. The service
// side marks math up and the assets exist; forgetting the template branch leaves the reader with
// LaTeX source and no visible change, which is exactly the reported symptom.
//
// TestMathTemplateLoadsKatex 保护真正让公式排版的引入。服务端已标记、资源也已存在；漏掉模板分支
// 会让读者继续看到 LaTeX 源码，正是报告的症状。
func TestMathTemplateLoadsKatex(t *testing.T) {
	var buf bytes.Buffer
	if err := blogPostTmpl.Execute(&buf, blogPostView{
		Title:    "t · s",
		SiteName: "s",
		HasMath:  true,
	}); err != nil {
		t.Fatalf("article template: %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		`/static/katex/katex.min.css?v=0.16.11`,
		`/static/katex/katex.min.js?v=0.16.11`,
		`/static/katex/render.js?v=0.16.11`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("article template missing %q", want)
		}
	}
}

func TestPageTemplateLoadsKatex(t *testing.T) {
	var buf bytes.Buffer
	if err := blogPageTmpl.Execute(&buf, blogPageView{
		PageTitle: "p · s",
		SiteName:  "s",
		HasMath:   true,
	}); err != nil {
		t.Fatalf("page template: %v", err)
	}
	if !strings.Contains(buf.String(), `/static/katex/katex.min.js?v=0.16.11`) {
		t.Errorf("page template does not load KaTeX")
	}
}
