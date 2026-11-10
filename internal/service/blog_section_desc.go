package service

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
)

// Section descriptions live in the vault, in a reserved note inside each folder.
//
// The owner asked to edit the one-line summary on a section card from Obsidian. The line used to
// be a hard-coded map in this package, which meant every copy change was a server deploy and the
// folder list in the map silently went stale as folders were renamed. A note in the folder is the
// one place where the text can live next to the notes it describes and still be edited with the
// same editor, sync pipeline and history as everything else.
//
// The file name is reserved, not the frontmatter key: the plugin creates the note, the server
// looks it up by suffix, and a folder without one simply falls back to its own name. The note does
// not need to be shared — the repository query reads it directly — and it is skipped everywhere
// articles are assembled, so an accidental folder-level share cannot publish it as a card.
//
// 栏目说明住在仓库里：每个目录一个保留名的笔记。
//
// 站主要求能在 Obsidian 里改栏目卡片上的那句说明。这句话原本写死在本包里，改一次文案就要发一次
// 服务端，而且目录改名后 map 里的键会悄悄过期。把说明放进目录里的一个笔记，是让文案与它描述的
// 笔记待在一起、又能共用同一个编辑器/同步管线/历史的唯一位置。
//
// 保留的是文件名而不是 frontmatter 键：插件负责创建，服务端按后缀查找，没有说明笔记的目录
// 回退到目录名。这个笔记不需要分享——仓储查询会直接读它——并且它会在所有文章装配处被跳过，
// 所以即使误用「分享整个文件夹」也不会把它发布成一张卡片。
const blogSectionNoteName = "_栏目说明.md"

// fmSectionDescRegex reads the one flat key the plugin writes.
//
// Flat keys for the same reason the cover keys are flat: frontmatter here is read line by line,
// not parsed as YAML, so a malformed value costs one line of metadata instead of the whole note.
//
// fmSectionDescRegex 读取插件写入的那一个平铺键。
//
// 与封面键一样用平铺键：这里的 frontmatter 是逐行读取的，不按 YAML 解析，
// 因此一个畸形的值只损失一行元数据，而不是整篇笔记。
var fmSectionDescRegex = regexp.MustCompile(`(?mi)^blog-desc:[ \t]*(.*?)[ \t]*$`)

// blogIsSectionNote reports whether a vault path points at a section description note.
//
// 判断仓库路径是否指向栏目说明笔记。
func blogIsSectionNote(p string) bool {
	p = strings.ReplaceAll(p, "\\", "/")
	return path.Base(p) == blogSectionNoteName
}

// blogSectionDescFromContent returns the description declared in a note's frontmatter.
//
// Only the frontmatter block is inspected: a `blog-desc:` line quoted inside the prose must not
// become the section summary. Values written by the plugin are JSON/YAML double-quoted strings;
// unquoted values are accepted too, because a hand-edited note is a normal thing to find.
//
// blogSectionDescFromContent 返回笔记 frontmatter 里声明的说明。
//
// 只看 frontmatter 块：正文里引用的一行 `blog-desc:` 不应该变成栏目摘要。插件写入的是
// JSON/YAML 双引号字符串；也接受不带引号的值，因为手改过的笔记是正常情况。
func blogSectionDescFromContent(content string) string {
	block := frontmatterRegex.FindString(content)
	if block == "" {
		return ""
	}
	raw := strings.TrimSpace(frontmatterValue(block, fmSectionDescRegex))
	if raw == "" {
		return ""
	}
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		if unquoted, err := strconv.Unquote(raw); err == nil {
			return strings.TrimSpace(unquoted)
		}
	}
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		return strings.TrimSpace(strings.ReplaceAll(raw[1:len(raw)-1], "''", "'"))
	}
	return raw
}

// blogSectionDescsFromNotes maps a normalized folder name to its description.
//
// The folder is the note's parent directory, normalized with blogSectionName so `（1）修复笔记`
// and `修复笔记` describe the same section the cards aggregate. Notes without a usable
// description are skipped rather than stored as empty strings, which keeps the fallback decision
// in one place (blogAggregateSections).
//
// blogSectionDescsFromNotes 把归一化目录名映射到说明。
//
// 目录取笔记的父目录，并用 blogSectionName 归一化，使「（1）修复笔记」与「修复笔记」对应到
// 卡片聚合的同一个版块。没有可用说明的笔记直接跳过而不是存空串，
// 这样回退逻辑只留在 blogAggregateSections 一处。
func blogSectionDescsFromNotes(notes []*domain.Note) map[string]string {
	if len(notes) == 0 {
		return nil
	}
	sorted := make([]*domain.Note, 0, len(notes))
	for _, n := range notes {
		if n != nil {
			sorted = append(sorted, n)
		}
	}
	// Deterministic when two folders normalize to the same name: the last path in sort order
	// wins, so the same query always yields the same page.
	//
	// 两个目录归一化后同名时保持确定性：排序后最后一个路径生效，因此同一查询总是得到同一页面。
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	out := make(map[string]string, len(sorted))
	for _, n := range sorted {
		if !blogIsSectionNote(n.Path) {
			continue
		}
		dir := path.Dir(strings.ReplaceAll(n.Path, "\\", "/"))
		if dir == "." || dir == "/" || dir == "" {
			continue
		}
		desc := blogSectionDescFromContent(n.Content)
		if desc == "" {
			continue
		}
		out[blogSectionName(path.Base(dir))] = desc
	}
	return out
}

// loadBlogSectionDescs reads every section note for the user.
//
// It reads all notes (not just shares) because the note is deliberately unshared metadata. The
// query is a path-suffix match, so it costs one indexed scan and ~20 rows even though the vault
// holds more than a thousand notes.
//
// loadBlogSectionDescs 读取该用户的全部栏目说明笔记。
//
// 读全部笔记（不只是分享）是因为这个笔记本来就不分享。查询按路径后缀匹配，
// 即使仓库有一千多篇笔记，也只是 ~20 行的有界扫描。
func (s *shareService) loadBlogSectionDescs(ctx context.Context, uid int64) map[string]string {
	notes, err := s.noteRepo.ListByPathSuffix(ctx, uid, "/"+blogSectionNoteName)
	if err != nil {
		// A missing description is cosmetic: the page falls back to the folder name, and a blog
		// render must not fail because a metadata note could not be read.
		//
		// 缺说明只影响文案：页面会回退到目录名，博客渲染不应该因为一篇元数据笔记读不到而失败。
		return nil
	}
	return blogSectionDescsFromNotes(notes)
}
