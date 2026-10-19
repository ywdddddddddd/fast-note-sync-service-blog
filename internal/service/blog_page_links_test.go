package service

import (
	"testing"
)

func TestBlogPageLinksFindsAbout(t *testing.T) {
	all := []*blogHomeSource{
		{NoteID: 1, Path: "压力阈值-1.md", Title: "压力阈值-1"},
		{NoteID: 459, Path: "简历/关于.md", Title: "关于.md",
			Content: "---\ntitle: 关于我\npage: about\n---\n正文", Page: "about"},
	}
	links := blogPageLinks(all)
	if len(links) != 1 {
		t.Fatalf("blogPageLinks = %d entries, want 1", len(links))
	}
	if links[0].Alias != "about" || links[0].URL != "/blog/about" {
		t.Fatalf("unexpected link: %+v", links[0])
	}
	if links[0].Title != "关于我" {
		t.Fatalf("title = %q, want 关于我", links[0].Title)
	}
	if got := blogPageLinks(nil); len(got) != 0 {
		t.Fatalf("empty input produced %d links", len(got))
	}
}
