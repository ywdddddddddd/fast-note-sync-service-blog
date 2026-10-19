package service

import (
	"strings"
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
)

// TestParseSizeFromAlt covers the adjustable-media convention and, more importantly, the cases
// where a numeric caption must NOT become a size.
//
// TestParseSizeFromAlt 覆盖 adjustable-media 的约定，更重要的是覆盖「数字图注不得变成尺寸」的情形。
func TestParseSizeFromAlt(t *testing.T) {
	cases := []struct {
		in      string
		wantW   int
		wantH   int
		wantOK  bool
		comment string
	}{
		// Values actually present in the vault.
		// 仓库中真实出现的取值。
		{"196", 196, 0, true, "vault value"},
		{"313", 313, 0, true, "vault value"},
		{"634", 634, 0, true, "vault value"},
		{"640x480", 640, 480, true, "width x height"},

		// Must stay a caption.
		// 必须保持为图注。
		{"", 0, 0, false, "empty"},
		{"图 1", 0, 0, false, "text caption"},
		{"2026", 0, 0, false, "year-like value is out of range"},
		{"5", 0, 0, false, "too small to be a width"},
		{"99999", 0, 0, false, "too large to be a width"},
		{"1201", 0, 0, false, "just above the ceiling"},
		{"19", 0, 0, false, "just below the floor"},
		{"1200", 1200, 0, true, "exactly at the ceiling"},
		{"12px", 0, 0, false, "unit suffix"},
		{"abc", 0, 0, false, "non-numeric"},
		{"100x", 0, 0, false, "missing height"},
		{"x100", 0, 0, false, "missing width"},
		{"100x5", 0, 0, false, "height below floor"},
		{"-100", 0, 0, false, "negative"},
	}

	for _, c := range cases {
		w, h, ok := parseSizeFromAlt(c.in)
		if ok != c.wantOK || w != c.wantW || h != c.wantH {
			t.Errorf("parseSizeFromAlt(%q) = (%d, %d, %v), want (%d, %d, %v)  [%s]",
				c.in, w, h, ok, c.wantW, c.wantH, c.wantOK, c.comment)
		}
	}
}

// TestAtoiPositive covers the small helper's boundaries.
// TestAtoiPositive 覆盖小工具的边界。
func TestAtoiPositive(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int
		ok   bool
	}{
		{"0", 0, true},
		{"7", 7, true},
		{"313", 313, true},
		{"", 0, false},
		{"  ", 0, false},
		{"1a", 0, false},
		{"-1", 0, false},
		{"100001", 0, false},
	} {
		got, ok := atoiPositive(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("atoiPositive(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestRenderPDFReference pins the 20 MB boundary: at or below it the document is embedded, above
// it only a link is emitted together with the reason.
//
// TestRenderPDFReference 固定 20 MB 边界：不超过则内联文档，超过则只给出链接并附上原因。
func TestRenderPDFReference(t *testing.T) {
	const url = "/api/share/file?id=9&share_token=T"

	small := renderPDFReference(url, "notes/handout.pdf", 1024*1024, "讲义")
	if !strings.Contains(small, "<embed") {
		t.Errorf("1 MB PDF should be embedded: %s", small)
	}
	if !strings.Contains(small, `type="application/pdf"`) {
		t.Errorf("embed is missing its media type: %s", small)
	}
	if !strings.Contains(small, "讲义") {
		t.Errorf("alt text should be used as the label: %s", small)
	}

	exactly := renderPDFReference(url, "notes/handout.pdf", pdfEmbedMaxBytes, "")
	if !strings.Contains(exactly, "<embed") {
		t.Errorf("a PDF exactly at the cap should be embedded: %s", exactly)
	}

	large := renderPDFReference(url, "notes/book.pdf", pdfEmbedMaxBytes+1, "")
	if strings.Contains(large, "<embed") {
		t.Errorf("a PDF over the cap must not be embedded: %s", large)
	}
	if !strings.Contains(large, "文件较大") {
		t.Errorf("oversized PDF should say why it is a link only: %s", large)
	}
	// A PDF with no recorded size falls back to a link without claiming it is large.
	// 没有记录大小的 PDF 回退为链接，且不声称它很大。
	unknown := renderPDFReference(url, "notes/x.pdf", 0, "")
	if strings.Contains(unknown, "文件较大") {
		t.Errorf("unknown size must not be reported as large: %s", unknown)
	}
	if !strings.Contains(unknown, "x.pdf") {
		t.Errorf("label should fall back to the file name: %s", unknown)
	}
}

// TestRenderPDFReferenceEscapesLabel: the label can come from user text (the alt), so it must not
// be able to break out of the attribute or the text node.
//
// TestRenderPDFReferenceEscapesLabel：标签可能来自用户文本（alt），因此不能让它突破属性或文本节点。
func TestRenderPDFReferenceEscapesLabel(t *testing.T) {
	got := renderPDFReference("/x", "a.pdf", 100, `"><script>alert(1)</script>`)
	if strings.Contains(got, "<script>") {
		t.Errorf("label was not escaped: %s", got)
	}
}

// TestDetectMediaKindByExtPDF: the missing pdf branch is what turned an embedded PDF into a
// broken <img>, so this pins the classification.
//
// TestDetectMediaKindByExtPDF：缺失的 pdf 分支正是「嵌入的 PDF 变成坏图片」的原因，
// 因此这里固定该分类。
func TestDetectMediaKindByExtPDF(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"a/b/document.pdf", "pdf"},
		{"A/B/DOCUMENT.PDF", "pdf"}, // the helper lowercases the extension itself
		{"a/b/photo.png", "image"},
		{"a/b/clip.mp4", "video"},
		{"a/b/song.mp3", "audio"},
		{"a/b/notes.md", ""},
	} {
		if got := detectMediaKindByExt(c.in); got != c.want {
			t.Errorf("detectMediaKindByExt(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPDFEndToEnd runs the real pipeline over an embedded PDF and checks it does not come out as
// an <img>.
//
// TestPDFEndToEnd 在真实管线上跑一个嵌入的 PDF，检查它不会以 <img> 形式出来。
func TestPDFEndToEnd(t *testing.T) {
	md := newTestMarkdown()

	fileRefs := map[string]*domain.File{
		"handout.pdf": {ID: 42, Path: "notes/handout.pdf", Size: 2 * 1024 * 1024},
	}
	rewritten := rewriteMarkdownImageLinks("见 ![讲义](handout.pdf) 结尾", fileRefs, "TOKEN", "")
	got := render(t, md, rewritten)

	if strings.Contains(got, "<img") {
		t.Errorf("PDF was rendered as an image: %s", got)
	}
	if !strings.Contains(got, "<embed") {
		t.Errorf("PDF was not embedded: %s", got)
	}
	if !strings.Contains(got, "/api/share/file?id=42") {
		t.Errorf("embed src was not rewritten: %s", got)
	}
}

// TestNumericAltEndToEnd: a numeric alt becomes a width, and a textual alt does not.
// TestNumericAltEndToEnd：数字 alt 变成宽度，文本 alt 不变。
func TestNumericAltEndToEnd(t *testing.T) {
	md := newTestMarkdown()

	fileRefs := map[string]*domain.File{
		"photo.jpg": {ID: 7, Path: "notes/photo.jpg", Size: 1000},
	}

	numeric := render(t, md, rewriteMarkdownImageLinks("![634](photo.jpg)", fileRefs, "T", ""))
	if !strings.Contains(numeric, `width:634px`) {
		t.Errorf("numeric alt was not applied as a width: %s", numeric)
	}

	textual := render(t, md, rewriteMarkdownImageLinks("![图 1](photo.jpg)", fileRefs, "T", ""))
	if strings.Contains(textual, "width:") {
		t.Errorf("textual alt must not become a size: %s", textual)
	}
	if !strings.Contains(textual, "图 1") {
		t.Errorf("textual alt was lost: %s", textual)
	}

	// A year-shaped caption is the realistic false positive and must stay a caption.
	// 年份形态的图注是最现实的误判来源，必须保持为图注。
	year := render(t, md, rewriteMarkdownImageLinks("![2026](photo.jpg)", fileRefs, "T", ""))
	if strings.Contains(year, "width:") {
		t.Errorf("year-like caption became a size: %s", year)
	}
}
