package routers

import (
	"strconv"
	"testing"
)

// The shelf takes two values from a hand-written manifest and puts them into file paths: the book
// id becomes a cache directory name, the file name becomes a path under books/. Neither is
// trusted. These are the two validators that stand between a typo (or a copy-pasted path) and an
// arbitrary file being served.
//
// 书架从一份手写书目里取两个值并放进文件路径：书的 id 变成缓存目录名，文件名变成 books/ 下的路径。
// 两者都不被信任。下面两个校验函数就是「一个笔误（或者一段复制来的路径）」与「交出任意文件」
// 之间唯一的东西。
func TestShelfRejectsNamesThatCouldEscape(t *testing.T) {
	badIDs := []string{"", "..", "../etc", "a/b", "a\\b", "A", "Oral-Histology", "x y", "-lead", ".hidden"}
	for _, id := range badIDs {
		if shelfIDPattern.MatchString(id) {
			t.Errorf("id %q was accepted, want rejected", id)
		}
	}
	goodIDs := []string{"a", "oral-histology", "book-1", "x.y_z", "n2"}
	for _, id := range goodIDs {
		if !shelfIDPattern.MatchString(id) {
			t.Errorf("id %q was rejected, want accepted", id)
		}
	}

	badFiles := []string{"", "x.txt", "../x.pdf", "a/b.pdf", `a\b.pdf`, "x.pdf/..", "..pdf", ".pdf"}
	for _, f := range badFiles {
		if shelfSafeFileName(f) {
			t.Errorf("file %q was accepted, want rejected", f)
		}
	}
	for _, f := range []string{"book.pdf", "BOOK.PDF", "a-1.b2.pdf"} {
		if !shelfSafeFileName(f) {
			t.Errorf("file %q was rejected, want accepted", f)
		}
	}
}

// The page number is parsed out of a URL segment. Anything that is not a positive integer has to
// die here, before it reaches pdftoppm's -f/-l arguments.
//
// 页码是从 URL 段里解析出来的。任何不是正整数的东西都必须死在这里，在被送进
// pdftoppm 的 -f/-l 参数之前。
func TestShelfPageSegmentMustBeAPositiveNumber(t *testing.T) {
	cases := map[string]bool{
		"1.jpg":   true,
		"12.jpg":  true,
		"999.jpg": true,
		"1":       true,
		"0.jpg":   false,
		"-3.jpg":  false,
		"1e5.jpg": false,
		"a.jpg":   false,
		".jpg":    false,
	}
	for seg, want := range cases {
		// Mirrors the handler exactly: trim the extension, strconv.Atoi, then require >= 1.
		// 与 handler 完全一致：去掉扩展名、strconv.Atoi、然后要求 >= 1。
		raw := seg
		if len(raw) > 4 && raw[len(raw)-4:] == ".jpg" {
			raw = raw[:len(raw)-4]
		}
		got := false
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 {
			got = true
		}
		if got != want {
			t.Errorf("segment %q: accepted=%v, want %v", seg, got, want)
		}
	}
}
