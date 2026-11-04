package service

import "testing"

// blogTitleNumber has to read four notations and must not invent a number where there is none.
// A wrong reading here reorders a reader's table of contents, which is worse than not sorting.
func TestBlogTitleNumber(t *testing.T) {
	cases := []struct {
		title     string
		primary   int
		secondary int
		ok        bool
	}{
		{"第6章：口腔颌面部感染", 6, 0, true},
		{"第6章-2：牙体缺损的修复", 6, 2, true},
		{"第6章（3）", 6, 3, true},
		{"第一章 绪论", 1, 0, true},
		{"第十二章 麻醉", 12, 0, true},
		{"第二十五章 总结", 25, 0, true},
		{"第一百零三章", 103, 0, true},
		{"一、绪论", 1, 0, true},
		{"3. 概述", 3, 0, true},
		{"3、概述", 3, 0, true},
		{"Chapter 3 The Cell", 3, 0, true},
		{"Ch.7 Review", 7, 0, true},
		{"12 标题", 12, 0, true},
		{"绪论", 0, 0, false},
		{"前言", 0, 0, false},
		{"A Letter", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, c := range cases {
		p, s, ok := blogTitleNumber(c.title)
		if ok != c.ok || p != c.primary || s != c.secondary {
			t.Errorf("blogTitleNumber(%q) = (%d,%d,%v), want (%d,%d,%v)",
				c.title, p, s, ok, c.primary, c.secondary, c.ok)
		}
	}
}

// The key must make plain string comparison behave like counting order, across notations, and put
// unnumbered titles last.
func TestBlogTitleOrderKey(t *testing.T) {
	ordered := []string{
		"第1章 绪论",
		"第2章 临床接诊",
		"第3章 麻醉",
		"第9章 拔牙",
		"第10章 感染",
		"第11章 创伤",
	}
	for i := 1; i < len(ordered); i++ {
		a := blogTitleOrderKey(ordered[i-1])
		b := blogTitleOrderKey(ordered[i])
		if !(a < b) {
			t.Errorf("expected %q before %q, keys %q >= %q", ordered[i-1], ordered[i], a, b)
		}
	}

	// Chinese numerals must land in the same sequence as Arabic ones.
	mixed := []string{"第三章", "第5章", "第十二章"}
	keys := []string{
		blogTitleOrderKey(mixed[0]),
		blogTitleOrderKey(mixed[1]),
		blogTitleOrderKey(mixed[2]),
	}
	if !(keys[0] < keys[1] && keys[1] < keys[2]) {
		t.Errorf("mixed notations not ordered: %v", keys)
	}

	// Anything unnumbered sorts after everything numbered.
	numbered := blogTitleOrderKey("第99章 最后一章")
	unnumbered := blogTitleOrderKey("附录")
	if !(numbered < unnumbered) {
		t.Errorf("unnumbered %q should sort after numbered %q", unnumbered, numbered)
	}

	// Secondary number breaks the tie before the title does.
	a := blogTitleOrderKey("第6章-1 第一节")
	b := blogTitleOrderKey("第6章-2 第二节")
	if !(a < b) {
		t.Errorf("secondary number ignored: %q !< %q", a, b)
	}
}
