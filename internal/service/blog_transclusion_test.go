package service

import (
	"strings"
	"testing"
)

// The expandTransclusions function needs a *shareService with a working loader; the tests below
// exercise the pure parts (extension filtering, blockquote indentation, depth guard) plus the
// degradation contract, which is the security-relevant half.
//
// expandTransclusions 需要带可用 loader 的 *shareService；下面的测试覆盖纯逻辑部分
//（扩展名过滤、引用块缩进、深度守卫）以及降级契约——后者才是与安全相关的那一半。

// TestTransclusionFileExtsStayOnAttachmentPath: a file embed must never be treated as a note.
// TestTransclusionFileExtsStayOnAttachmentPath：文件嵌入绝不能被当作笔记处理。
func TestTransclusionFileExtsStayOnAttachmentPath(t *testing.T) {
	files := []string{
		"image.png", "photo.JPG", "doc.pdf", "clip.mp4", "audio.mp3",
		"archive.zip", "drawing.excalidraw", "vector.svg",
	}
	for _, f := range files {
		if !transclusionFileExts[strings.ToLower(filepathExt(f))] {
			t.Errorf("%q should be classified as an attachment", f)
		}
	}
	notes := []string{"笔记", "Another note", "folder/子笔记", "笔记.md", "a.b.c"}
	for _, n := range notes {
		if transclusionFileExts[strings.ToLower(filepathExt(n))] {
			t.Errorf("%q should not be classified as an attachment", n)
		}
	}
}

// TestFilepathExt pins the small helper's edge cases.
// TestFilepathExt 固定小工具的边界情形。
func TestFilepathExt(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a.md", ".md"},
		{"dir/a.md", ".md"},
		{"dir.b/a", ""},
		{".obsidian/x", ""},
		{"a", ""},
		{"a.b.c", ".c"},
		{"notes/1. intro.md", ".md"},
	}
	for _, c := range cases {
		if got := filepathExt(c.in); got != c.want {
			t.Errorf("filepathExt(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestTransclusionEmbedRegexMatchesOnlyEmbedForm checks that the regex sees ![[...]] and leaves
// plain [[...]] to the wiki-link pass.
//
// TestTransclusionEmbedRegexMatchesOnlyEmbedForm 检查正则只识别 ![[...]]，
// 把普通 [[...]] 留给 wiki 链接处理。
func TestTransclusionEmbedRegexMatchesOnlyEmbedForm(t *testing.T) {
	// Embed forms the regex must accept.
	// 正则必须接受的嵌入形式。
	for _, in := range []string{"![[笔记]]", "![[笔记|别名]]", "![[folder/子笔记]]", "![[笔记#小节]]", "![[笔记#小节|别名]]"} {
		if m := transclusionEmbedRegex.FindStringSubmatch(in); m == nil {
			t.Errorf("embed %q not matched", in)
		}
	}
	// A plain wiki link must not match.
	// 普通 wiki 链接不得匹配。
	if m := transclusionEmbedRegex.FindStringSubmatch("[[笔记]]"); m != nil {
		t.Errorf("plain wiki link was matched as an embed: %v", m)
	}
}

// TestIndentAsBlockquote covers the transformation that keeps an expanded body inside the
// callout wrapper, including the blank-line rule (a bare ">" keeps the quote alive).
//
// TestIndentAsBlockquote 覆盖把展开正文保持在 callout 包裹层内的变换，
// 包括空行规则（单独的 ">" 让引用块保持连续）。
func TestIndentAsBlockquote(t *testing.T) {
	got := indentAsBlockquote("第一行\n\n第二行")
	want := "> 第一行\n>\n> 第二行"
	if got != want {
		t.Errorf("indentAsBlockquote = %q, want %q", got, want)
	}
	if strings.Contains(indentAsBlockquote("a\n\n\nb"), "\n\n") {
		t.Errorf("blank lines must become bare '>' so the blockquote does not break")
	}
	if strings.HasPrefix(indentAsBlockquote("\n开头空行"), "\n") {
		t.Errorf("leading newline should be trimmed")
	}
}

// TestAliasOr covers the label fallback.
// TestAliasOr 覆盖标签回退。
func TestAliasOr(t *testing.T) {
	if got := aliasOr("目标", "别名"); got != "别名" {
		t.Errorf("aliasOr = %q, want 别名", got)
	}
	if got := aliasOr("目标", ""); got != "目标" {
		t.Errorf("aliasOr = %q, want 目标", got)
	}
}

// TestTransclusionDepthGuard is the cycle protection: at or beyond the limit nothing is
// expanded, so a self-referencing chain terminates.
//
// TestTransclusionDepthGuard 是防环保护：达到或超过上限时不再展开，
// 因此自引用的链条会终止。
func TestTransclusionDepthGuard(t *testing.T) {
	s := &shareService{}
	// depth >= max must be a no-op regardless of content.
	// 无论内容如何，depth >= max 都必须是空操作。
	src := "![[任何笔记]]"
	if got := s.expandTransclusions(nil, 1, src, transclusionMaxDepth); got != src {
		t.Errorf("expansion ran at max depth: %q", got)
	}
	if got := s.expandTransclusions(nil, 1, src, transclusionMaxDepth+5); got != src {
		t.Errorf("expansion ran beyond max depth: %q", got)
	}
	// Content without an embed marker is returned untouched.
	// 不含嵌入标记的内容原样返回。
	if got := s.expandTransclusions(nil, 1, "普通正文", 0); got != "普通正文" {
		t.Errorf("plain content was altered: %q", got)
	}
}

// TestTransclusionUnresolvedIsTheSecurityContract covers the decision that matters: an embed
// whose target is not published must degrade to readable text and must never carry content.
//
// It is tested through a pure function rather than through expandTransclusions, because the
// latter needs repositories — and a security rule that can only be exercised with a database is
// a security rule that will not be exercised.
//
// TestTransclusionUnresolvedIsTheSecurityContract 覆盖真正重要的决策：
// 目标未发布的嵌入必须降级为可读文本，且绝不携带内容。
//
// 它通过纯函数测试而不是通过 expandTransclusions —— 后者需要仓库，
// 而一条只能在有数据库时被验证的安全规则，等于一条不会被验证的安全规则。
func TestTransclusionUnresolvedIsTheSecurityContract(t *testing.T) {
	cases := []struct{ name, target, alias, want string }{
		{"no alias", "未公开笔记", "", "未公开笔记"},
		{"alias wins", "未公开笔记", "换个说法", "换个说法"},
		{"path form", "folder/子笔记", "", "folder/子笔记"},
		{"whitespace alias ignored", "目标", "   ", "目标"},
	}
	for _, c := range cases {
		got := transclusionUnresolved(c.target, c.alias)
		if got != c.want {
			t.Errorf("%s: transclusionUnresolved(%q, %q) = %q, want %q",
				c.name, c.target, c.alias, got, c.want)
		}
		// The degradation must never look like expanded content.
		// 降级结果绝不能看起来像展开了的内容。
		if strings.Contains(got, "callout") || strings.Contains(got, "![[") {
			t.Errorf("%s: degradation leaked structure: %q", c.name, got)
		}
	}
}

// TestTransclusionRegexNeverMatchesAPlainLink: only the "!" form is an embed, so a normal wiki
// link cannot become content by accident.
//
// TestTransclusionRegexNeverMatchesAPlainLink：只有带 "!" 的形式才是嵌入，
// 因此普通 wiki 链接不会意外变成内容。
func TestTransclusionRegexNeverMatchesAPlainLink(t *testing.T) {
	for _, in := range []string{"[[笔记]]", "[[笔记|别名]]", "见 [[a]] 与 [[b]]"} {
		if m := transclusionEmbedRegex.FindStringSubmatch(in); m != nil {
			t.Errorf("%q matched as an embed: %v", in, m)
		}
	}
}
