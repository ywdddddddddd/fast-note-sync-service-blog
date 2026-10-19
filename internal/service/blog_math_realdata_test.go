package service

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestPreprocessMathAgainstRealVaultNote runs the math preprocessing over a real note that
// contains a display formula, if that note is present on this machine.
//
// It is skipped when the file is absent so the suite stays runnable off this host.
//
// TestPreprocessMathAgainstRealVaultNote 在一篇含独立公式的真实笔记上跑数学预处理
// （若该笔记在本机上存在）。
//
// 文件不存在时跳过，使测试套件在其它主机上仍可运行。
func TestPreprocessMathAgainstRealVaultNote(t *testing.T) {
	const path = `D:\fast-note-sync\storage\vault\u_1\note\n_102\content.txt`
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("real note not available: %v", err)
	}
	src := string(raw)
	if !strings.Contains(src, "$$") {
		t.Skip("real note contains no display formula")
	}

	md := newTestMarkdown()
	prepared := preprocessMath(src)
	got := render(t, md, prepared)

	// The display formula must be marked up, and its LaTeX must survive verbatim.
	// 独立公式必须被标记，且其 LaTeX 必须原样保留。
	if !strings.Contains(got, `class="math-block"`) {
		t.Errorf("real note: no math-block in output")
	}
	if !strings.Contains(got, `\begin{array}`) {
		t.Errorf("real note: LaTeX body lost")
	}
	if !HasMathMarkup(got) {
		t.Errorf("real note: hasMathMarkup returned false for a document with a formula")
	}

	// A quick census, printed so the numbers are visible in the test log.
	// 做一次计数并打印，使数字在测试日志中可见。
	blocks := strings.Count(got, `class="math-block"`)
	inlines := strings.Count(got, `class="math-inline"`)
	t.Logf("real note n_102: math-block=%d math-inline=%d rendered=%d bytes", blocks, inlines, len(got))

	f, _ := os.Create(`D:\fast-note-sync\_regress\n102-rendered.html`)
	if f != nil {
		_, _ = io.WriteString(f, got)
		_ = f.Close()
	}
}
