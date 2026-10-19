package service

import (
	"context"
	"regexp"
	"strings"
)

// Note transclusion: ![[Another note]] expands to that note's content.
//
// The security contract is the whole point of this file: **only notes that are themselves
// published may be transcluded**. The blog is public, so expanding an unpublished target would
// publish its text through the back door — the reader never sees a link, they see the content.
// A target that is not published keeps the existing degradation (its readable name as plain
// text), which is what every other unresolved wiki reference already does.
//
// 笔记嵌入：![[另一篇笔记]] 展开为该笔记的正文。
//
// 本文件的核心就是这条安全契约：**只有本身已发布的笔记才能被嵌入**。博客是公开页面，
// 展开一个未发布的目标等于让它的正文从后门公开——读者看到的不是链接，而是内容本身。
// 未发布的目标保持既有降级行为（其可读名称作为纯文本），这与其它所有无法解析的 wiki 引用一致。

// transclusionMaxDepth bounds recursive expansion. A note that transcludes a note that
// transcludes the first one would otherwise expand forever.
//
// transclusionMaxDepth 限制递归展开深度。否则「A 嵌入 B、B 又嵌入 A」会无限展开。
const transclusionMaxDepth = 3

// transclusionEmbedRegex matches "![[target]]" and "![[target|alias]]".
// It deliberately does not match file embeds: the caller filters by extension, because
// "![[image.png]]" must stay on the attachment path.
//
// transclusionEmbedRegex 匹配 "![[目标]]" 与 "![[目标|别名]]"。
// 它刻意不区分是否文件嵌入：由调用方按扩展名过滤，因为 "![[image.png]]" 必须留在附件路径上。
var transclusionEmbedRegex = regexp.MustCompile(`!\[\[([^\]\|#]+)(?:#[^\]\|]*)?(?:\|([^\]]*))?\]\]`)

// transclusionFileExts lists the extensions that belong to the attachment pipeline rather than
// to note transclusion.
//
// transclusionFileExts 列出属于附件管线而非笔记嵌入的扩展名。
var transclusionFileExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true,
	".bmp": true, ".avif": true, ".ico": true,
	".pdf": true,
	".mp4": true, ".mov": true, ".webm": true, ".mkv": true, ".avi": true,
	".mp3": true, ".wav": true, ".m4a": true, ".ogg": true, ".flac": true,
	".zip": true, ".7z": true, ".rar": true, ".excalidraw": true,
}

// transclusionSource is one expandable target: a published note that can be embedded.
// transclusionSource 一个可展开的目标：一篇已发布、可被嵌入的笔记。
type transclusionSource struct {
	NoteID  int64
	Slug    string
	Title   string
	Content string
}

// expandTransclusions rewrites "![[Note]]" into the target note's rendered body.
//
// Only published notes are expandable (see the file comment). Expansion is depth-limited and
// cycle-safe: a note already on the current expansion stack is not expanded again.
//
// The expanded body is inserted as Markdown, not HTML, so the caller's single Markdown pass
// still sees it — wiki links, formulas, callouts and heading ids inside a transcluded note
// therefore behave exactly as they do in a normal note.
//
// expandTransclusions 把 "![[笔记]]" 改写为目标笔记的正文。
//
// 只有已发布的笔记可展开（见文件头注释）。展开受深度限制且防环：
// 已在当前展开栈上的笔记不会再次展开。
//
// 展开的正文以 **Markdown** 而非 HTML 插入，因此调用方的单次 Markdown 渲染仍然能看到它——
// 被嵌入笔记内部的 wiki 链接、公式、callout 与标题 id 的行为与普通笔记完全一致。
func (s *shareService) expandTransclusions(ctx context.Context, uid int64, content string, depth int) string {
	if depth >= transclusionMaxDepth || !strings.Contains(content, "![[") {
		return content
	}

	sources, err := s.loadTransclusionSources(ctx, uid)
	if err != nil || len(sources) == 0 {
		return content
	}

	return transclusionEmbedRegex.ReplaceAllStringFunc(content, func(m string) string {
		sub := transclusionEmbedRegex.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		target := strings.TrimSpace(sub[1])
		alias := strings.TrimSpace(sub[2])
		if target == "" {
			return m
		}

		// Attachments stay on the attachment path.
		// 附件留在附件路径上。
		if ext := strings.ToLower(filepathExt(target)); transclusionFileExts[ext] {
			return m
		}

		key := strings.ToLower(strings.TrimSuffix(target, ".md"))
		src, ok := sources[key]
		if !ok {
			return transclusionUnresolved(target, alias)
		}

		body := s.expandTransclusions(ctx, uid, src.Content, depth+1)
		if strings.TrimSpace(body) == "" {
			return aliasOr(target, alias)
		}

		// The source is embedded under its own heading so the reader can tell where the quoted
		// note begins. The reverse link is written as a normal wiki link so the outer pass turns
		// it into a real /blog/ link.
		//
		// 被嵌入内容放在自己的标题之下，使读者能看出引用从哪里开始。
		// 反向链接写成普通 wiki 链接，由外层处理把它变成真实的 /blog/ 链接。
		title := src.Title
		if alias != "" {
			title = alias
		}
		return "\n\n> [!note] 嵌入自 " + title + "\n>\n" +
			indentAsBlockquote(body) +
			"\n\n"
	})
}

// transclusionUnresolved decides what replaces an embed whose target is not published (or is
// unknown).
//
// This is the security-relevant decision of the whole feature, so it is a pure function with its
// own test rather than a branch buried inside a method that needs a database: the blog is public,
// and expanding an unpublished note would publish its text through the back door — the reader
// sees no link, they see the content.
//
// The answer is always "readable text, no content": the alias when one was given, otherwise the
// target name. It never reads the note.
//
// transclusionUnresolved 决定「目标未发布（或未知）」时用什么替换该嵌入。
//
// 这是整个功能里与安全相关的那个决策，因此它是一个带独立测试的纯函数，而不是埋在一个需要
// 数据库的方法里的分支：博客是公开页面，展开一篇未发布的笔记等于让它的正文从后门公开——
// 读者看不到链接，看到的是内容本身。
//
// 答案永远是「可读文本、不含内容」：给了别名就用别名，否则用目标名。绝不读取该笔记。
func transclusionUnresolved(target, alias string) string {
	return aliasOr(target, alias)
}

// aliasOr returns the alias when present, otherwise the target name.
//
// A whitespace-only alias counts as absent: "![[note|   ]]" is a malformed embed, and falling
// back to the target name produces something readable instead of an invisible label.
//
// aliasOr 有别名时返回别名，否则返回目标名。
//
// 仅含空白的别名视为不存在："![[note|   ]]" 是畸形的嵌入写法，
// 回退到目标名能产出可读内容，而不是一个看不见的标签。
func aliasOr(target, alias string) string {
	if strings.TrimSpace(alias) != "" {
		return alias
	}
	return target
}

// indentAsBlockquote prefixes every line with "> " so the expanded body lives inside the callout
// marker written by expandTransclusions. Blank lines become ">" so the blockquote does not end.
//
// indentAsBlockquote 给每一行加上 "> " 前缀，使展开的正文落在 expandTransclusions 写出的
// callout 标记之内。空行变成 ">"，以免引用块被中断。
func indentAsBlockquote(body string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	var b strings.Builder
	for i, ln := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if strings.TrimSpace(ln) == "" {
			b.WriteString(">")
			continue
		}
		b.WriteString("> ")
		b.WriteString(ln)
	}
	return b.String()
}

// filepathExt is a tiny extension helper kept local so this file does not pull in path/filepath
// for a single call.
//
// filepathExt 是本地的小扩展名工具，避免为一次调用引入 path/filepath。
func filepathExt(p string) string {
	i := strings.LastIndex(p, ".")
	if i < 0 {
		return ""
	}
	// A dot inside the last path segment only counts when it is not the first character, so
	// ".obsidian/x" has no extension while "a.md" does.
	// 最后一个路径段里的点只有不在首位时才计为扩展名，因此 ".obsidian/x" 没有扩展名，
	// 而 "a.md" 有。
	slash := strings.LastIndexAny(p, `/\`)
	if i < slash {
		return ""
	}
	if i == slash+1 {
		return ""
	}
	return p[i:]
}

// loadTransclusionSources builds the lowercase name/path index of every note that may be
// embedded, together with its content.
//
// It reuses the same published-note resolution as the home page, so "what may be transcluded"
// and "what appears in the index" can never drift apart.
//
// loadTransclusionSources 构建「可被嵌入的每一篇笔记」的小写名称/路径索引，并带上其正文。
//
// 它复用与首页相同的「已发布笔记」解析逻辑，因此「什么可以被嵌入」与「什么出现在索引里」
// 不可能产生分歧。
func (s *shareService) loadTransclusionSources(ctx context.Context, uid int64) (map[string]*transclusionSource, error) {
	sources, _, err := s.loadBlogSources(ctx, uid)
	if err != nil {
		return nil, err
	}

	out := make(map[string]*transclusionSource, len(sources)*2)
	for _, src := range sources {
		if src == nil || strings.TrimSpace(src.Content) == "" {
			continue
		}
		entry := &transclusionSource{
			NoteID:  src.NoteID,
			Slug:    src.Slug,
			Title:   src.Title,
			Content: src.Content,
		}
		// Index by basename without extension (the common way notes are embedded) and by full
		// path without extension (unambiguous form).
		//
		// 按不含扩展名的文件名（嵌入笔记的常见写法）与不含扩展名的完整路径（无歧义写法）建索引。
		base := src.Path
		if i := strings.LastIndexAny(base, `/\`); i >= 0 {
			base = base[i+1:]
		}
		base = strings.TrimSuffix(base, ".md")
		key := strings.ToLower(base)
		if _, dup := out[key]; !dup {
			out[key] = entry
		}

		full := strings.ToLower(strings.TrimSuffix(src.Path, ".md"))
		if full != key {
			if _, dup := out[full]; !dup {
				out[full] = entry
			}
		}
	}
	return out, nil
}
