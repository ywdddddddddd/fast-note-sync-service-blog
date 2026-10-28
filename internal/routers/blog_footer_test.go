package routers

import (
	"html/template"
	"strings"
	"testing"
)

// footerRender 只渲染页脚片段本身。
//
// 不直接执行 blog-home：那需要完整的 blogHomeView（Stats、Activity…），
// 与这条测试想验证的事毫无关系，构造出来只是噪音。
func footerRender(t *testing.T, v any) string {
	t.Helper()
	tmpl := template.Must(template.New("f").Parse(blogFooterLegal))
	var sb strings.Builder
	if err := tmpl.ExecuteTemplate(&sb, "footer-legal", v); err != nil {
		t.Fatalf("渲染页脚失败：%v", err)
	}
	return sb.String()
}

// TestFooterLegalReadsConfig 锁住一个真实事故：
//
// 2026-10-28 把页脚备案号从模板里删掉后，站点上线就少了备案号 —— 那是必须展示的。
// 同时它也不该硬编码在开源仓库里：备案号属于某个运营者的具体域名，
// 别人 clone 之后页脚带着别人的备案号既不合规也说不通。
//
// 所以它必须来自配置：仓库默认空（不渲染），部署者填了才出现。
func TestFooterLegalReadsConfig(t *testing.T) {
	// 开源默认：空配置不应渲染任何法务行。
	empty := footerRender(t, blogHomeView{})
	if strings.Contains(empty, "beian.miit.gov.cn") {
		t.Error("空配置时不该渲染 ICP 行")
	}
	if strings.Contains(empty, "beian.mps.gov.cn") {
		t.Error("空配置时不该渲染公安备案行")
	}

	// 填了值：两行都要出现，且公安备案的 code 要拼进查询地址。
	filled := footerRender(t, blogHomeView{
		ICP:     "浙ICP备00000000号-1",
		MPS:     "浙公网安备00000000000000号",
		MPSCode: "33078400000000",
	})
	if !strings.Contains(filled, "浙ICP备00000000号-1") {
		t.Error("ICP 号未渲染")
	}
	if !strings.Contains(filled, "浙公网安备00000000000000号") {
		t.Error("公安备案号未渲染")
	}
	if !strings.Contains(filled, "code=33078400000000") {
		t.Error("公安备案链接缺少 code 参数，会落到通用搜索页而不是本站记录")
	}
	if !strings.Contains(filled, "nofollow") {
		t.Error("备案链接应带 nofollow，避免给备案站传递权重")
	}
}

// TestFooterLegalNoCodeWhenEmpty 只有号码没有 code 时，仍要给出可用的查询页链接。
func TestFooterLegalNoCodeWhenEmpty(t *testing.T) {
	html := footerRender(t, blogHomeView{MPS: "浙公网安备00000000000000号"})
	if !strings.Contains(html, "beian.mps.gov.cn") {
		t.Error("公安备案链接缺失")
	}
	if strings.Contains(html, "?code=") {
		t.Error("没有 code 时不该拼一个空的 code 参数")
	}
}
