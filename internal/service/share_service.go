// Package service implements the business logic layer
// Package service 实现业务逻辑层
package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	pkgapp "github.com/haierkeys/fast-note-sync-service/pkg/app"
	"github.com/haierkeys/fast-note-sync-service/pkg/code"
	"github.com/haierkeys/fast-note-sync-service/pkg/shortlink"
	"github.com/haierkeys/fast-note-sync-service/pkg/timex"
	"github.com/haierkeys/fast-note-sync-service/pkg/util"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// attachmentRegex regular expression for attachment links
// attachmentRegex 附件链接正则表达式
var (
	attachmentRegex = regexp.MustCompile(`!\[\[(.*?)\]\]`)
	// Obsidian wiki syntax that may survive the share pipeline and would otherwise leak
	// verbatim into the blog HTML (goldmark only understands standard CommonMark).
	// For the built-in blog we resolve [[target|label]] to the linked note's blog page when
	// that note is itself shared, and degrade it to plain text otherwise.
	// 分享管线之后仍可能残留的 Obsidian wiki 语法；goldmark 只认标准 CommonMark，若不处理
	// 就会原样出现在博客 HTML 里。内置博客会把 [[目标|别名]] 解析为被链接笔记的博客页面
	// （若该笔记也已分享），否则降级为纯文本。
	wikiEmbedRegex = regexp.MustCompile(`!\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)
	wikiLinkRegex  = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)
	// markdownImageRegex regular expression for markdown images
	// markdownImageRegex markdown 图片正则表达式
	markdownImageRegex = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	// htmlImageRegex regular expression for html images
	// htmlImageRegex html 图片正则表达式
	htmlImageRegex = regexp.MustCompile(`(?i)<img\b([^>]*?)\bsrc\s*=\s*(['"])(.*?)['"]([^>]*)>`)
	// htmlVideoRegex regular expression for html video tags whose src is on
	// the tag itself (the alternative form using nested <source> children is
	// captured separately by htmlSourceRegex below).
	// htmlVideoRegex 匹配 src 写在标签自身上的 HTML <video>（嵌套 <source>
	// 形式由下方 htmlSourceRegex 单独捕获）。
	htmlVideoRegex = regexp.MustCompile(`(?i)<video\b([^>]*?)\bsrc\s*=\s*(['"])(.*?)['"]([^>]*)>`)
	// htmlAudioRegex regular expression for html audio tags whose src is on
	// the tag itself.
	// htmlAudioRegex 匹配 src 写在标签自身上的 HTML <audio>。
	htmlAudioRegex = regexp.MustCompile(`(?i)<audio\b([^>]*?)\bsrc\s*=\s*(['"])(.*?)['"]([^>]*)>`)
	// htmlSourceRegex regular expression for the <source> tag, typically
	// nested inside <video>/<audio>; commonly written self-closing.
	// htmlSourceRegex 匹配 <source> 标签，通常嵌套在 <video>/<audio> 内，且
	// 多以自闭合形式书写。
	htmlSourceRegex = regexp.MustCompile(`(?i)<source\b([^>]*?)\bsrc\s*=\s*(['"])(.*?)['"]([^>]*)>`)
)

// htmlMediaRegexes lists the HTML media tags whose `src` attribute should be
// extracted as a shared-note file reference and rewritten to a share API URL
// during the share render pipeline.
//
// htmlMediaRegexes 列出在分享渲染流程中需要将 src 属性当作分享笔记文件引用
// 提取并改写为分享 API URL 的 HTML 媒体标签。
var htmlMediaRegexes = []*regexp.Regexp{
	htmlVideoRegex,
	htmlAudioRegex,
	htmlSourceRegex,
}

// ShareService defines the share business service interface
// ShareService 定义分享业务服务接口
type ShareService interface {
	// ShareGenerate generates and stores share token
	// ShareGenerate 生成并存储分享 Token
	ShareGenerate(ctx context.Context, uid int64, vaultName string, path string, pathHash string, password string, expireAt int64) (*dto.ShareCreateResponse, error)

	// VerifyShare verifies share token and its status
	// VerifyShare 验证分享 Token 及其状态
	VerifyShare(ctx context.Context, token string, rid string, rtp string, password string) (*pkgapp.ShareEntity, error)

	// ResolveLocalShortLink resolves a built-in short link slug (short-link.mode = local)
	// to the current share URL, re-issuing the token so old short links keep working.
	// ResolveLocalShortLink 解析内置短链 slug（short-link.mode = local），
	// 重新签发令牌后返回当前分享地址，使旧短链长期有效。
	ResolveLocalShortLink(ctx context.Context, slug string, baseURL string) (string, error)

	// ListBlogEntries lists the active shares of a user, for the built-in blog index.
	// ListBlogEntries 列出某用户的有效分享，用于内置博客索引页。
	ListBlogEntries(ctx context.Context, uid int64) ([]*dto.BlogEntryDTO, error)

	// RenderBlogBySlug renders one shared note as HTML, addressed by its built-in short link slug.
	// RenderBlogBySlug 通过内置短链 slug 将单篇分享笔记渲染为 HTML。
	RenderBlogBySlug(ctx context.Context, slug string) (*dto.BlogPostDTO, error)

	// RenderBlogPage renders the published note that declares the given page alias, or a
	// not-found error when no note does.
	//
	// RenderBlogPage 渲染声明了指定页面别名的那一篇已发布笔记；没有任何笔记声明它时返回未找到。
	RenderBlogPage(ctx context.Context, uid int64, alias string) (*dto.BlogPageDTO, error)

	// GetBlogHome assembles the blog home page datasets: dashboard stats, tags, folder
	// navigation and the knowledge graph. publishedOnly keeps the graph scoped to notes that
	// are themselves published, so a public page never leaks unshared note titles.
	// GetBlogHome 组装博客首页数据：仪表盘统计、标签、目录导航与知识图谱。
	// publishedOnly 将图谱限制为「本身已发布」的笔记，避免公开页面泄露未分享笔记的标题。
	GetBlogHome(ctx context.Context, uid int64, baseURL string, publishedOnly bool, maxNodes int) (*dto.BlogHomeDTO, error)

	// GetBlogNav builds only the folder navigation tree, for pages that need the sidebar
	// but none of the home-page datasets (graph, heat map, tag cloud, stats).
	// GetBlogNav 只构建目录导航树，供需要侧栏但不需要首页其它数据集
	// （图谱、热力图、标签云、统计）的页面使用。
	GetBlogNav(ctx context.Context, uid int64) (*dto.BlogNavDTO, error)

	// GetLocalGraph builds the sub-graph around one article: the notes it links to plus the
	// notes linking to it, expanded to the requested depth. Only published notes may appear.
	// GetLocalGraph 构建单篇文章周围的子图：它链接到的笔记与链接到它的笔记，按指定深度展开。
	// 只有已发布的笔记可以出现。
	GetLocalGraph(ctx context.Context, slug string, depth, maxNodes int) (*dto.BlogGraphDTO, error)

	// GetLinkPreviews returns hover-card metadata for the given internal-link slugs, omitting
	// anything that is not a published note.
	// GetLinkPreviews 返回给定内部链接 slug 的悬停卡片元数据，省略所有非已发布笔记。
	GetLinkPreviews(ctx context.Context, uid int64, slugs []string) (map[string]*dto.BlogPreviewDTO, error)

	// GetBlogAccessLog returns a page of blog visits plus an aggregate summary.
	// GetBlogAccessLog 返回一页博客访问记录以及聚合摘要。
	GetBlogAccessLog(ctx context.Context, days, limit int) (*BlogAccessLogPage, error)

	// GetBlogReactions returns the like/dislike tallies and the run-total visit count.
	// GetBlogReactions 返回赞/踩计数与累计访问次数。
	GetBlogReactions() *dto.BlogReactionsDTO

	// ReactBlogNote applies one reaction to a note and returns its new tally.
	// ReactBlogNote 对一篇笔记应用一次反应并返回其新计数。
	ReactBlogNote(slug, kind string, delta int64) (*dto.BlogReactionTallyDTO, error)

	// AddBlogVisit bumps the run-total visit counter.
	// AddBlogVisit 累加访问计数。
	AddBlogVisit()

	// RecordBlogAccess stores one already-masked blog visit. Called by the access-log
	// middleware, which owns the masking rules.
	// RecordBlogAccess 存下一次已打码的博客访问。由访问日志中间件调用，打码规则归它所有。
	RecordBlogAccess(method, path string, status int, ms int64, ip, client string)

	// Ask runs the blog's retrieval-augmented Q&A: search the published corpus, expand through
	// wiki links, then stream an answer with citations. Events are pushed to emit in order.
	// Ask 执行博客的检索增强问答：检索已发布语料、沿 wiki 链接扩展，再流式产出带引用的回答。
	// 事件按顺序推送给 emit。
	Ask(ctx context.Context, opts AskOptions, emit func(AskEvent) error) error

	// GetSharedNote retrieves shared note details
	// GetSharedNote 获取分享的单条笔记详情
	GetSharedNote(ctx context.Context, shareToken string, noteID int64, password string) (*dto.NoteDTO, error)

	// GetSharedFile retrieves shared file content
	// GetSharedFile 获取分享的文件内容
	GetSharedFile(ctx context.Context, shareToken string, fileID int64, password string) (content []byte, contentType string, mtime int64, etag string, fileName string, err error)

	// GetSharedFileInfo retrieves shared file metadata and path for zero-copy download
	// GetSharedFileInfo 获取分享文件的元数据和路径，用于零拷贝下载
	GetSharedFileInfo(ctx context.Context, shareToken string, fileID int64, password string) (savePath string, contentType string, mtime int64, etag string, fileName string, err error)

	// RecordView aggregates access statistics in memory
	// RecordView 在内存中聚合访问统计
	RecordView(uid int64, id int64)

	// StopShare revokes a share
	// StopShare 撤销分享
	StopShare(ctx context.Context, uid int64, id int64) error

	// UpdateSharePassword updates password for a share based on resource info
	// UpdateSharePassword 根据资源信息更新分享密码
	UpdateSharePassword(ctx context.Context, uid int64, vaultName string, path string, pathHash string, password string, expireAt int64) error

	// CreateShortLink generates a short link for a share
	// CreateShortLink 为分享生成短链
	CreateShortLink(ctx context.Context, uid int64, vaultName string, path string, pathHash string, baseURL string, longURL string, isForce bool) (string, error)

	// ListShares lists all shares of a user with sorting and pagination
	// ListShares 列出用户的所有分享（支持排序和分页）
	ListShares(ctx context.Context, uid int64, sortBy string, sortOrder string, pager *pkgapp.Pager) ([]*dto.ShareListItem, int, error)

	// GetShareByPath retrieves share info by path
	// GetShareByPath 根据路径获取分享信息
	GetShareByPath(ctx context.Context, uid int64, vaultName string, pathHash string) (*domain.UserShare, error)

	// StopShareByPath revokes a share by path
	// StopShareByPath 根据路径撤销分享
	StopShareByPath(ctx context.Context, uid int64, vaultName string, pathHash string) error

	// GetActiveNotePathsByVault returns active shared note paths for a vault
	// GetActiveNotePathsByVault 返回指定 vault 下所有有效分享的笔记路径列表
	GetActiveNotePathsByVault(ctx context.Context, uid int64, vaultName string) ([]string, error)

	// Shutdown shuts down the service and flushes remaining data
	// Shutdown 关闭服务并同步最后的数据
	Shutdown(ctx context.Context) error
}

// aggStats aggregated statistics
// aggStats 聚合统计
type aggStats struct {
	uid          int64     // User ID // 用户 ID
	viewCount    int64     // View count // 访问计数
	lastViewedAt time.Time // Last viewed at // 最后访问时间
}

// shareService implementation of ShareService interface
// shareService 实现 ShareService 接口
type shareService struct {
	repo         domain.UserShareRepository // Share repository // 分享仓库
	tokenManager pkgapp.TokenManager        // Token manager // Token 管理器
	noteRepo     domain.NoteRepository      // Note repository // 笔记仓库
	noteLinkRepo domain.NoteLinkRepository  // Note link repository (knowledge graph) // 笔记链接仓库（知识图谱）
	fileRepo     domain.FileRepository      // File repository // 文件仓库
	vaultRepo    domain.VaultRepository     // Vault repository // 仓库仓库
	logger       *zap.Logger                // Logger // 日志器
	// blogAccess is nil when no log directory is configured; every method on it tolerates
	// that, so the blog keeps working on a read-only or misconfigured host.
	// blogAccess 在未配置日志目录时为 nil；它的每个方法都能容忍这一点，
	// 因此博客在只读或配置有误的主机上照常工作。
	blogAccess *blogAccessLog // Blog visit log // 博客访问日志
	// blogReactions holds the like/dislike tallies and the run-total visit counter. Nil when no
	// storage directory is configured; every method tolerates that.
	//
	// blogReactions 保存赞/踩计数与累计访问计数。未配置存储目录时为 nil；每个方法都能容忍它。
	blogReactions *blogReactionLog  // Blog reactions and visit total // 博客反应与访问总数
	config        *ServiceConfig    // Service configuration // 服务配置
	markdown      goldmark.Markdown // Markdown renderer for the built-in blog // 内置博客的 Markdown 渲染器

	// Blog Q&A: cached published-note corpus and per-route pacing state.
	// 博客问答：已发布笔记语料缓存，以及各线路的节流状态。
	askMu      sync.Mutex
	askRuntime *askRuntime

	// Statistics buffer
	// 统计缓冲区
	bufferMu    sync.Mutex          // Buffer mutex // 缓冲区互斥锁
	statsBuffer map[int64]*aggStats // Stats buffer // 统计缓冲区
	ticker      *time.Ticker        // Sync ticker // 同步定时器
	stopCh      chan struct{}       // Stop channel // 停止信号
	doneCh      chan struct{}       // Done channel // 完成信号
}

// NewShareService creates ShareService instance
// blogDataDir returns the directory the blog keeps its runtime files in, or "" to disable them.
//
// An empty result disables the files rather than erroring: a service that cannot write them should
// still serve the blog.
//
// blogDataDir 返回博客存放运行时文件的目录；返回空串表示停用它们。
//
// 空结果会停用这些文件而不是报错：写不了它们的服务仍然应该能提供博客。
func blogDataDir(config *ServiceConfig) string {
	if config == nil {
		return ""
	}
	return config.BlogAccessLogDir
}

// NewShareService 创建 ShareService 实例
func NewShareService(repo domain.UserShareRepository, tokenManager pkgapp.TokenManager, noteRepo domain.NoteRepository, noteLinkRepo domain.NoteLinkRepository, fileRepo domain.FileRepository, vaultRepo domain.VaultRepository, logger *zap.Logger, config *ServiceConfig) ShareService {
	s := &shareService{
		repo:         repo,
		tokenManager: tokenManager,
		blogAccess:   newBlogAccessLog(blogDataDir(config), logger),
		// The reaction file shares the blog's data directory. It is not "the access log directory"
		// in meaning, only in location: both are runtime state the blog owns, and adding a second
		// config knob for one small file would be a knob nobody would ever set differently.
		//
		// 反应文件与博客数据目录共用位置。它在含义上并不是「访问日志目录」，只是位置相同：
		// 两者都是博客自己的运行时状态，而为一个很小的文件增设第二个配置项，
		// 会是那个永远没人会设成不同值的配置项。
		blogReactions: newBlogReactionLog(blogDataDir(config), logger),
		noteRepo:      noteRepo,
		noteLinkRepo:  noteLinkRepo,
		fileRepo:      fileRepo,
		vaultRepo:     vaultRepo,
		logger:        logger,
		config:        config,
		markdown: goldmark.New(
			// Extension set. GFM brings Table / Strikethrough / Linkify / TaskList; the rest
			// are the Obsidian-facing additions:
			//
			//   CJK          soft line breaks between CJK characters do not become a space.
			//                Without it, a paragraph written one sentence per line renders with
			//                a stray space at every line end — the single most visible defect on
			//                a Chinese-language note.
			//   Footnote     [^1] references and their definitions.
			//   Typographer  curly quotes, em-dashes, ellipses.
			//   Mark         ==highlight== (the node renderer lives in mark_extension.go,
			//                because goldmark v1.8.6 has no highlight extension).
			//
			// 扩展集合。GFM 带来 Table / Strikethrough / Linkify / TaskList；其余是面向
			// Obsidian 的补充：
			//
			//   CJK          中日韩字符之间的软换行不会变成空格。缺了它，逐句换行书写的段落
			//                会在每一行末尾多出一个空格 —— 这是中文笔记上最显眼的排版缺陷。
			//   Footnote     [^1] 脚注引用与定义。
			//   Typographer  弯引号、破折号、省略号。
			//   Mark         ==高亮==（节点渲染器在 mark_extension.go，
			//                因为 goldmark v1.8.6 没有高亮扩展）。
			goldmark.WithExtensions(
				extension.GFM,
				extension.CJK,
				extension.Footnote,
				extension.Typographer,
				Mark,
			),
			// AutoHeadingID gives every heading a slug, which is what makes the in-page
			// outline anchors work. goldmark does not emit heading ids by default.
			//
			// It is only a fallback here: the id generator is replaced per conversion through
			// blogHeadingIDOptions(), because goldmark's built-in one drops every multi-byte
			// character (on a Chinese vault each heading collapsed to "heading", "heading-1"…,
			// so inserting a heading renumbered all the ones after it). See blog_heading_ids.go.
			//
			// AutoHeadingID 为每个标题生成锚点 id，博客页的本页目录跳转依赖它；
			// goldmark 默认不会输出标题 id。
			//
			// 这里它只是兜底：id 生成器由 blogHeadingIDOptions() 在每次转换时替换，
			// 因为 goldmark 内置的那个会丢弃所有多字节字符（在中文仓库里每个标题都塌成
			// "heading"、"heading-1"……，插入一个标题就会让其后的所有标题重新编号）。
			// 详见 blog_heading_ids.go。
			goldmark.WithParserOptions(parser.WithAutoHeadingID()),
			// Required: the share pipeline rewrites attachment references into inline
			// <img src="/api/share/file?id=..."> tags. Without WithUnsafe the renderer drops
			// raw HTML, so every image in the note would silently disappear.
			// 必须开启：分享管线会把附件引用改写为内联 <img src="/api/share/file?id=..."> 标签，
			// 若不开启 WithUnsafe，渲染器会丢弃原始 HTML，笔记里的图片将全部消失。
			//
			// WithHardWraps makes a single newline render as <br>.
			//
			// This is what Obsidian does by default, and a note written there is written against
			// that expectation: one sentence per line, no blank lines. CommonMark instead folds a
			// single newline into a space, so every such note arrived as one solid block of text.
			// Measured on this vault: 69 of 114 published notes contain runs of soft-wrapped
			// lines, the worst with 192 of them — so this was not an edge case, it was the
			// majority of the blog rendering wrong.
			//
			// extension.CJK (above) only removes the stray space a soft break would insert
			// between two CJK characters. It does not turn the break into a line break, which is
			// why the two together were still one paragraph.
			//
			// Block constructs are unaffected: a newline between two list items, two table rows
			// or two paragraphs is block syntax, not a soft break. Only a wrap *inside* a
			// paragraph becomes a <br> — which is exactly the intent.
			//
			// 开启后单个换行会渲染为 <br>。
			//
			// 这正是 Obsidian 的默认行为，而在那里写的笔记就是照着这个预期写的：
			// 一句一行、不空行。CommonMark 则把单个换行折成一个空格，
			// 于是每一篇这样的笔记到来时都是一整块连在一起的文字。
			// 在这个仓库上实测：114 篇已发布笔记中有 69 篇含有软换行连续行，
			// 最严重的一篇有 192 处 —— 所以这不是边角情况，而是博客的大多数在渲染错误。
			//
			// 上面的 extension.CJK 只消除软换行在两个 CJK 字符之间插入的多余空格，
			// 并不会把换行变成换行 —— 这就是两者同时存在时仍是一段的原因。
			//
			// 块级结构不受影响：两个列表项、两行表格、两个段落之间的换行是块级语法而不是软换行。
			// 只有段落内部的折行会变成 <br>，而这正是本意。
			goldmark.WithRendererOptions(html.WithUnsafe(), html.WithHardWraps()),
		),
		statsBuffer: make(map[int64]*aggStats),
		ticker:      time.NewTicker(5 * time.Minute),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

	go s.startFlushLoop()

	return s
}

// ShareGenerate generates and stores share token
// ShareGenerate 生成并存储分享 Token
func (s *shareService) ShareGenerate(ctx context.Context, uid int64, vaultName string, path string, pathHash string, password string, expireAt int64) (*dto.ShareCreateResponse, error) {
	// 1. Get VaultID
	// 1. 获取 VaultID
	vault, err := s.vaultRepo.GetByName(ctx, vaultName, uid)
	if err != nil {
		return nil, err
	}
	vaultID := vault.ID

	var resolvedResources = make(map[string][]string)
	var mainID int64
	var mainType string

	// 2. Determine type based on suffix
	// 2. 根据后缀判定类型
	isNote := strings.HasSuffix(strings.ToLower(path), ".md")

	if isNote {
		// Try looking up as Note
		// 尝试作为 Note 查找
		note, err := s.noteRepo.GetByPathHash(ctx, pathHash, vaultID, uid)
		if err == nil && note != nil && note.Action != domain.NoteActionDelete {
			mainID = note.ID
			mainType = "note"
			noteIDStr := strconv.FormatInt(note.ID, 10)
			resolvedResources["note"] = []string{noteIDStr}
			fileRefs, err := s.resolveSharedNoteFiles(ctx, uid, note.VaultID, note.Path, note.Content)
			if err != nil {
				s.logger.Warn("ShareGenerate resolveSharedNoteFiles failed", zap.Error(err), zap.String("notePath", note.Path))
			} else {
				for _, file := range fileRefs {
					fileIDStr := strconv.FormatInt(file.ID, 10)
					// Avoid duplicate authorization
					// 避免重复授权
					if !util.Inarray(resolvedResources["file"], fileIDStr) {
						resolvedResources["file"] = append(resolvedResources["file"], fileIDStr)
					}
				}
			}
		} else {
			return nil, code.ErrorNoteNotFound.WithDetails("note not found: " + path)
		}
	} else {
		// Try looking up as File
		// 尝试作为 File 查找
		file, err := s.fileRepo.GetByPathHash(ctx, pathHash, vaultID, uid)
		if err == nil && file != nil && file.Action != domain.FileActionDelete {
			mainID = file.ID
			mainType = "file"
			fileIDStr := strconv.FormatInt(file.ID, 10)
			resolvedResources["file"] = []string{fileIDStr}
		} else {
			return nil, code.ErrorFileNotFound.WithDetails("file not found: " + path)
		}
	}

	// 3. Determine expiration time from client-supplied expireAt
	// 3. 根据客户端传入的 expireAt 确定过期时间
	// expiresAt zero value means the share never expires (expireAt <= 0)
	// expiresAt 零值表示分享永久有效（expireAt <= 0）
	var expiresAt time.Time
	if expireAt > 0 {
		expiresAt = time.Unix(expireAt, 0)
	}

	// pwdHash password bcrypt hash
	// pwdHash 密码 bcrypt 哈希值
	pwdHash := ""
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		pwdHash = string(hash)
	}

	share := &domain.UserShare{
		UID:       uid,
		ResType:   mainType,
		ResID:     mainID,
		Resources: resolvedResources,
		Status:    1,
		ExpiresAt: expiresAt,
		Password:  pwdHash,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Idempotent: revoke any existing active share before creating a new one
	// 幂等：若该资源已有 active 分享，先撤销，避免重复计数
	if existing, err := s.repo.GetByRes(ctx, uid, mainType, mainID); err == nil && existing != nil {
		_ = s.StopShare(ctx, uid, existing.ID)
	}

	if err := s.repo.Create(ctx, uid, share); err != nil {
		return nil, err
	}

	// 4. Generate Token (using underlying SID encryption scheme)
	// 4. 生成 Token (使用底层 SID 加密方案)
	token, err := s.tokenManager.ShareGenerate(share.ID, uid, resolvedResources)
	if err != nil {
		return nil, err
	}

	return &dto.ShareCreateResponse{
		ID:         mainID,
		Type:       mainType,
		Token:      token,
		IsPassword: pwdHash != "",
		ExpiresAt:  expiresAt,
		ShortLink:  share.ShortLink,
	}, nil
}

// VerifyShare verifies share token and its status
// VerifyShare 验证分享 Token 及其状态
func (s *shareService) VerifyShare(ctx context.Context, token string, rid string, rtp string, password string) (*pkgapp.ShareEntity, error) {
	entity, err := s.tokenManager.ShareParse(token)

	if err != nil {
		return nil, err
	}

	share, err := s.repo.GetByID(ctx, entity.UID, entity.SID)

	if err != nil {
		return nil, err
	}

	if share.Status != 1 {
		return nil, domain.ErrShareCancelled
	}

	// Zero ExpiresAt means the share never expires; otherwise reject once past the deadline
	// ExpiresAt 为零值表示分享永久有效；否则超过截止时间即拒绝访问
	if !share.ExpiresAt.IsZero() && time.Now().After(share.ExpiresAt) {
		return nil, domain.ErrShareExpired
	}

	// 添加限速延迟防止暴力枚举攻击
	time.Sleep(time.Millisecond * 100)

	// Add password verification logic with upgrade support
	// 增加密码校验与升级逻辑
	if share.Password != "" {
		if password == "" {
			return nil, domain.ErrSharePasswordRequired
		}

		// 1. Try bcrypt verification first
		// 1. 优先尝试 bcrypt 验证
		err := bcrypt.CompareHashAndPassword([]byte(share.Password), []byte(password))
		if err != nil {
			// 2. If it fails and the stored hash length is 32, it might be the old MD5 hash
			// 2. 如果验证失败且存储的哈希长度为 32，可能为旧的 MD5 哈希
			if len(share.Password) == 32 {
				if util.EncodeMD5(password) == share.Password {
					// 3. Verification succeeds, silently upgrade the password hash to bcrypt in a background goroutine
					// 3. 验证成功，在后台协程中将密码哈希静默升级为 bcrypt
					go func(uid, shareID int64, plainPwd string) {
						newHash, err := bcrypt.GenerateFromPassword([]byte(plainPwd), bcrypt.DefaultCost)
						if err == nil {
							// Update password hash in db, using background context to prevent cancellation
							// 在 db 中更新密码哈希，使用后台 context 避免因请求结束被取消
							_ = s.repo.UpdatePassword(context.Background(), uid, shareID, string(newHash))
						}
					}(share.UID, share.ID, password)
				} else {
					return nil, domain.ErrSharePasswordInvalid
				}
			} else {
				return nil, domain.ErrSharePasswordInvalid
			}
		}
	}

	entity.Resources = share.Resources

	ids, ok := share.Resources[rtp]
	if !ok {
		return nil, domain.ErrShareCancelled // Match type mismatch // 资源类型不匹配
	}

	authorized := false
	for _, id := range ids {
		if id == rid {
			authorized = true
			break
		}
	}

	if !authorized {
		return nil, domain.ErrShareCancelled // Resource not authorized // 资源未授权
	}

	// In-memory record access statistics (delayed 5 minutes update)
	// 内存记录访问统计 (延迟 5 分钟更新)
	s.RecordView(share.UID, share.ID)

	return entity, nil
}

// RecordView aggregates access statistics in memory
// RecordView 在内存中聚合访问统计
func (s *shareService) RecordView(uid int64, id int64) {
	s.bufferMu.Lock()
	defer s.bufferMu.Unlock()

	stats, ok := s.statsBuffer[id]
	if !ok {
		stats = &aggStats{
			uid: uid,
		}
		s.statsBuffer[id] = stats
	}
	stats.viewCount++
	stats.lastViewedAt = time.Now()
}

// startFlushLoop starts periodic synchronization goroutine
// startFlushLoop 启动定时同步协程
func (s *shareService) startFlushLoop() {
	defer close(s.doneCh)
	for {
		select {
		case <-s.ticker.C:
			s.flush()
		case <-s.stopCh:
			s.flush()
			return
		}
	}
}

// flush synchronizes incremental totals in memory to database
// flush 将内存中的增量合计同步到数据库
func (s *shareService) flush() {
	s.bufferMu.Lock()
	if len(s.statsBuffer) == 0 {
		s.bufferMu.Unlock()
		return
	}
	tempBuffer := s.statsBuffer
	s.statsBuffer = make(map[int64]*aggStats)
	s.bufferMu.Unlock()

	ctx := context.Background()
	for id, stats := range tempBuffer {
		if err := s.repo.UpdateViewStats(ctx, stats.uid, id, stats.viewCount, stats.lastViewedAt); err != nil {
			s.logger.Error("failed to flush user_share stats", zap.Int64("id", id), zap.Error(err))
		}
	}
}

// StopShare revokes a share
// StopShare 撤销分享
func (s *shareService) StopShare(ctx context.Context, uid int64, id int64) error {
	return s.repo.UpdateStatus(ctx, uid, id, domain.UserShareStatusRevoked)
}

// UpdateSharePassword updates password and expiration time for a share
// UpdateSharePassword 更新分享的密码与过期时间
func (s *shareService) UpdateSharePassword(ctx context.Context, uid int64, vaultName string, path string, pathHash string, password string, expireAt int64) error {
	// 1. Get VaultID
	vault, err := s.vaultRepo.GetByName(ctx, vaultName, uid)
	if err != nil {
		return err
	}
	if vault == nil {
		return code.ErrorVaultNotFound
	}

	// 2. Get UserShare by resource
	share, err := s.repo.GetByPath(ctx, uid, vault.ID, pathHash)
	if err != nil {
		return err
	}
	if share == nil {
		return code.ErrorShareNotFound
	}

	pwdHash := ""
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		pwdHash = string(hash)
	}
	if err := s.repo.UpdatePassword(ctx, uid, share.ID, pwdHash); err != nil {
		return err
	}

	// expiresAt zero value means the share never expires (expireAt <= 0)
	// expiresAt 零值表示分享永久有效（expireAt <= 0）
	var expiresAt time.Time
	if expireAt > 0 {
		expiresAt = time.Unix(expireAt, 0)
	}
	return s.repo.UpdateExpiresAt(ctx, uid, share.ID, expiresAt)
}

// ListShares lists all shares of a user with sorting, pagination and fills in resource titles
// ListShares 列出用户的所有分享，支持排序、分页，并填充资源标题
func (s *shareService) ListShares(ctx context.Context, uid int64, sortBy string, sortOrder string, pager *pkgapp.Pager) ([]*dto.ShareListItem, int, error) {
	// 1. 获取总数
	count64, err := s.repo.CountByUID(ctx, uid)
	if err != nil {
		return nil, 0, err
	}
	count := int(count64)

	if count == 0 {
		return []*dto.ShareListItem{}, 0, nil
	}

	// 2. 获取分页数据
	shares, err := s.repo.ListByUID(ctx, uid, sortBy, sortOrder, pkgapp.GetPageOffset(pager.Page, pager.PageSize), pager.PageSize)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*dto.ShareListItem, 0, len(shares))

	// Collect noteIDs and fileIDs in bulk to avoid N+1 queries
	// 批量收集 noteIDs 和 fileIDs，避免 N+1 查询
	var noteIDs, fileIDs []int64
	for _, share := range shares {
		switch share.ResType {
		case "note":
			noteIDs = append(noteIDs, share.ResID)
		case "file":
			fileIDs = append(fileIDs, share.ResID)
		}
	}

	// Batch query notes and build id→note map
	// 批量查询 notes，建立 id→note 映射
	noteMap := make(map[int64]*domain.Note)
	if len(noteIDs) > 0 {
		notes, err := s.noteRepo.ListByIDs(ctx, noteIDs, uid)
		if err != nil {
			s.logger.Warn("ListShares: batch query notes failed", zap.Error(err))
		} else {
			for _, n := range notes {
				noteMap[n.ID] = n
			}
		}
	}

	fileMap := make(map[int64]*domain.File)
	if len(fileIDs) > 0 {
		files, err := s.fileRepo.ListByIDs(ctx, fileIDs, uid)
		if err != nil {
			s.logger.Warn("ListShares: batch query files failed", zap.Error(err))
		} else {
			for _, f := range files {
				fileMap[f.ID] = f
			}
		}
	}

	// Query vault names on demand with local cache (vault count is always small)
	// 查询 vault 名称（vault 数量极少，按需缓存）
	vaultNameCache := make(map[int64]string)

	for _, share := range shares {
		item := &dto.ShareListItem{
			ID:           share.ID,
			UID:          share.UID,
			Resources:    share.Resources,
			Status:       share.Status,
			ViewCount:    share.ViewCount,
			LastViewedAt: share.LastViewedAt,
			ExpiresAt:    share.ExpiresAt,
			ShortLink:    share.ShortLink,
			CreatedAt:    share.CreatedAt,
			UpdatedAt:    share.UpdatedAt,
			IsPassword:   share.Password != "",
		}

		// Generate Token and concatenate URL /ResID/token
		// 生成 Token 并拼接 URL /ResID/token
		token, err := s.tokenManager.ShareGenerate(share.ID, uid, share.Resources)
		if err == nil {
			item.URL = "/share/" + strconv.FormatInt(share.ResID, 10) + "/" + token
		}

		// Fill title from preloaded maps, no extra DB queries
		// 从预加载的 map 中回填标题，无额外查询
		switch share.ResType {
		case "note":
			if note, ok := noteMap[share.ResID]; ok && note.Action != domain.NoteActionDelete {
				item.Title = strings.TrimSuffix(filepath.Base(note.Path), ".md")
				item.NotePath = note.Path
				if name, ok := vaultNameCache[note.VaultID]; ok {
					item.VaultName = name
				} else if v, err := s.vaultRepo.GetByID(ctx, note.VaultID, uid); err == nil && v != nil {
					vaultNameCache[note.VaultID] = v.Name
					item.VaultName = v.Name
				}
			}
		case "file":
			if file, ok := fileMap[share.ResID]; ok {
				item.Title = filepath.Base(file.Path)
			}
		}

		items = append(items, item)
	}

	return items, count, nil
}

// GetShareByPath retrieves share info by path
// GetShareByPath 根据路径获取分享信息
func (s *shareService) GetShareByPath(ctx context.Context, uid int64, vaultName string, pathHash string) (*domain.UserShare, error) {
	vault, err := s.vaultRepo.GetByName(ctx, vaultName, uid)
	if err != nil {
		return nil, err
	}

	resID := int64(0)
	resType := ""

	// Check if it's a note or file
	note, err := s.noteRepo.GetByPathHash(ctx, pathHash, vault.ID, uid)
	if err == nil && note != nil {
		resID = note.ID
		resType = "note"
	} else {
		file, err := s.fileRepo.GetByPathHash(ctx, pathHash, vault.ID, uid)
		if err == nil && file != nil {
			resID = file.ID
			resType = "file"
		}
	}

	if resID == 0 {
		return nil, code.ErrorFileNotFound
	}

	// Use precision index query instead of iterating list
	// 使用精确索引查询，替代遍历列表
	share, err := s.repo.GetByRes(ctx, uid, resType, resID)
	if err != nil {
		return nil, code.ErrorFileNotFound // No active share found
	}

	return share, nil
}

// CreateShortLink generates a short link for a share record
// CreateShortLink 为分享记录生成短链
// localShortLinkSecret derives the HMAC key used to sign built-in short links.
// It is sourced from security.share-token-key, so no extra configuration is required.
// Note: changing share-token-key invalidates previously issued short links, exactly as it
// already does for share tokens.
// localShortLinkSecret 派生内置短链的 HMAC 签名密钥，来源为 security.share-token-key，
// 因此无需新增配置；注意修改 share-token-key 会使既有短链失效（与分享 Token 行为一致）。
func (s *shareService) localShortLinkSecret() []byte {
	key := s.config.App.ShortLink.SigKey
	if key == "" {
		key = "fns"
	}
	return []byte(key + "_shortlink_v1")
}

// localShortLinkSig returns the hex HMAC of a short link payload.
// localShortLinkSig 返回短链载荷的十六进制 HMAC。
func (s *shareService) localShortLinkSig(payload string) string {
	h := hmac.New(sha256.New, s.localShortLinkSecret())
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// BuildLocalShortLink builds the slug of a built-in short link (uid.sid-hmac10).
// BuildLocalShortLink 生成内置短链 slug（uid.sid-hmac10）。
func (s *shareService) BuildLocalShortLink(uid int64, sid int64) string {
	payload := fmt.Sprintf("%d.%d", uid, sid)
	return payload + "-" + s.localShortLinkSig(payload)[:10]
}

// ResolveLocalShortLink resolves a built-in short link slug to the current share URL.
// A fresh token is issued on every visit, so the short link keeps working as long as the
// share itself is active — including after the original share token has expired.
// ResolveLocalShortLink 解析内置短链 slug 并返回当前分享地址。每次访问都会重新签发令牌，
// 因此只要分享记录本身有效，短链就一直可用——包括原分享令牌过期之后。
func (s *shareService) ResolveLocalShortLink(ctx context.Context, slug string, baseURL string) (string, error) {
	idx := strings.LastIndex(slug, "-")
	if idx <= 0 || idx >= len(slug)-1 {
		return "", code.ErrorShareNotFound
	}
	payload, sig := slug[:idx], slug[idx+1:]
	if len(sig) != 10 || strings.ContainsAny(payload, "/?#") {
		return "", code.ErrorShareNotFound
	}
	if !hmac.Equal([]byte(sig), []byte(s.localShortLinkSig(payload)[:10])) {
		return "", code.ErrorShareNotFound
	}

	parts := strings.Split(payload, ".")
	if len(parts) != 2 {
		return "", code.ErrorShareNotFound
	}
	uid, errUID := strconv.ParseInt(parts[0], 10, 64)
	sid, errSID := strconv.ParseInt(parts[1], 10, 64)
	if errUID != nil || errSID != nil || uid <= 0 || sid <= 0 {
		return "", code.ErrorShareNotFound
	}

	share, err := s.repo.GetByID(ctx, uid, sid)
	if err != nil || share == nil {
		return "", code.ErrorShareNotFound
	}
	if share.Status != domain.UserShareStatusActive {
		return "", domain.ErrShareCancelled
	}
	if !share.ExpiresAt.IsZero() && time.Now().After(share.ExpiresAt) {
		return "", domain.ErrShareExpired
	}

	token, err := s.tokenManager.ShareGenerate(share.ID, uid, share.Resources)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/share/%d/%s", strings.TrimRight(baseURL, "/"), share.ResID, token), nil
}

// isShareUsable reports whether a share record is currently readable.
// isShareUsable 判断分享记录当前是否可读。
func (s *shareService) isShareUsable(sh *domain.UserShare) bool {
	if sh == nil || sh.Status != domain.UserShareStatusActive {
		return false
	}
	if !sh.ExpiresAt.IsZero() && time.Now().After(sh.ExpiresAt) {
		return false
	}
	return true
}

// parseLocalShortLink verifies a built-in short link slug and returns uid/sid.
// parseLocalShortLink 校验内置短链 slug 并返回 uid/sid。
func (s *shareService) parseLocalShortLink(slug string) (int64, int64, error) {
	idx := strings.LastIndex(slug, "-")
	if idx <= 0 || idx >= len(slug)-1 {
		return 0, 0, code.ErrorShareNotFound
	}
	payload, sig := slug[:idx], slug[idx+1:]
	if len(sig) != 10 || strings.ContainsAny(payload, "/?#") {
		return 0, 0, code.ErrorShareNotFound
	}
	if !hmac.Equal([]byte(sig), []byte(s.localShortLinkSig(payload)[:10])) {
		return 0, 0, code.ErrorShareNotFound
	}
	parts := strings.Split(payload, ".")
	if len(parts) != 2 {
		return 0, 0, code.ErrorShareNotFound
	}
	uid, errUID := strconv.ParseInt(parts[0], 10, 64)
	sid, errSID := strconv.ParseInt(parts[1], 10, 64)
	if errUID != nil || errSID != nil || uid <= 0 || sid <= 0 {
		return 0, 0, code.ErrorShareNotFound
	}
	return uid, sid, nil
}

// ListBlogEntries lists the active shares of a user, for the built-in blog index.
// ListBlogEntries 列出某用户的有效分享，用于内置博客索引页。
func (s *shareService) ListBlogEntries(ctx context.Context, uid int64) ([]*dto.BlogEntryDTO, error) {
	shares, err := s.repo.ListByUID(ctx, uid, "created_at", "desc", 0, blogMaxShares)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.BlogEntryDTO, 0, len(shares))
	seen := make(map[string]struct{}, len(shares))
	for _, sh := range shares {
		if !s.isShareUsable(sh) {
			continue
		}
		entry := &dto.BlogEntryDTO{Slug: s.BuildLocalShortLink(uid, sh.ID)}
		switch sh.ResType {
		case "note":
			if n, e := s.noteRepo.GetByID(ctx, sh.ResID, uid); e == nil && n != nil {
				entry.Path = n.Path
				entry.Title = strings.TrimSuffix(filepath.Base(n.Path), ".md")
			}
		case "file":
			if f, e := s.fileRepo.GetByID(ctx, sh.ResID, uid); e == nil && f != nil {
				entry.Path = f.Path
				entry.Title = filepath.Base(f.Path)
			}
		}
		if entry.Title == "" {
			entry.Title = fmt.Sprintf("Share #%d", sh.ID)
		}

		// Deduplicate by source path. The same note can legitimately have more than one
		// active share — e.g. it was shared, then renamed/moved and shared again, and both
		// records are kept so that previously distributed links keep working. The blog
		// should still list it only once; records are ordered newest first, so the entry
		// that survives is the most recent one.
		// 按来源路径去重。同一篇笔记可以同时存在多条有效分享——例如分享后被移动/重命名又分享了
		// 一次，两条记录都会保留，以便此前分发出去的链接继续可用。博客只列一条；记录按创建
		// 时间倒序返回，因此保留下来的就是最新的那条。
		dedupKey := entry.Path
		if dedupKey == "" {
			dedupKey = fmt.Sprintf("#%d", sh.ID)
		}
		if _, dup := seen[dedupKey]; dup {
			continue
		}
		seen[dedupKey] = struct{}{}

		if !sh.CreatedAt.IsZero() {
			entry.CreatedAt = sh.CreatedAt.Format("2006-01-02")
		}
		out = append(out, entry)
	}
	return out, nil
}

// rewriteWikiLinks converts leftover Obsidian wiki syntax into blog-friendly Markdown.
//
// Attachment embeds (![[file]]) are normally already rewritten into <img>/<video> tags by the
// share pipeline, so what remains is note-to-note links. A link whose target note is itself
// shared becomes a link to that note's blog page — which is what makes cross-note navigation
// work inside the blog. A link to a note that is NOT shared degrades to plain text, so raw
// [[...]] syntax never reaches the rendered page.
//
// rewriteWikiLinks 把残留的 Obsidian wiki 语法转换为适合博客的 Markdown。
// 附件嵌入（![[文件]]）通常已被分享管线改写为 <img>/<video> 标签，因此剩下的主要是笔记间链接。
// 若目标笔记本身也已分享，则转为该笔记的博客页面链接——这正是博客内文章互跳的来源；
// 若目标笔记未分享，则降级为纯文本，确保 [[...]] 语法不会出现在渲染结果中。
func (s *shareService) rewriteWikiLinks(ctx context.Context, uid int64, content string) string {
	if !strings.Contains(content, "[[") {
		return content
	}

	entries, err := s.ListBlogEntries(ctx, uid)
	byName := make(map[string]string)
	byPath := make(map[string]string)
	if err == nil {
		for _, e := range entries {
			if e.Path == "" {
				continue
			}
			ext := filepath.Ext(e.Path)
			noExt := strings.TrimSuffix(e.Path, ext)
			byPath[strings.ToLower(noExt)] = e.Slug
			base := strings.TrimSuffix(filepath.Base(e.Path), ext)
			key := strings.ToLower(base)
			if _, dup := byName[key]; !dup {
				byName[key] = e.Slug
			}
		}
	}

	label := func(target, alias string) string {
		if strings.TrimSpace(alias) != "" {
			return strings.TrimSpace(alias)
		}
		return target
	}

	// 1) Residual embeds: drop the embed syntax, keep the readable name.
	// 1) 残留的嵌入语法：去掉嵌入符号，保留可读名称。
	content = wikiEmbedRegex.ReplaceAllStringFunc(content, func(m string) string {
		sub := wikiEmbedRegex.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		return label(strings.TrimSpace(sub[1]), sub[2])
	})

	// 2) Note-to-note links: point at the target's blog page when it is shared.
	// 2) 笔记间链接：目标已分享时指向其博客页面。
	return wikiLinkRegex.ReplaceAllStringFunc(content, func(m string) string {
		sub := wikiLinkRegex.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		target := strings.TrimSpace(sub[1])
		text := label(target, sub[2])

		clean := target
		if i := strings.IndexAny(clean, "#^"); i >= 0 {
			clean = strings.TrimSpace(clean[:i])
		}
		clean = strings.TrimSuffix(clean, ".md")
		if clean == "" {
			return text
		}

		if slug, ok := byPath[strings.ToLower(clean)]; ok {
			return fmt.Sprintf("[%s](/blog/%s)", text, slug)
		}
		if !strings.Contains(clean, "/") {
			if slug, ok := byName[strings.ToLower(clean)]; ok {
				return fmt.Sprintf("[%s](/blog/%s)", text, slug)
			}
		}
		return text
	})
}

// RenderBlogBySlug renders one shared note as HTML, addressed by its short link slug.
// RenderBlogBySlug 通过短链 slug 将单篇分享笔记渲染为 HTML。
func (s *shareService) RenderBlogBySlug(ctx context.Context, slug string) (*dto.BlogPostDTO, error) {
	uid, sid, err := s.parseLocalShortLink(slug)
	if err != nil {
		return nil, err
	}
	share, err := s.repo.GetByID(ctx, uid, sid)
	if err != nil || share == nil {
		return nil, code.ErrorShareNotFound
	}
	if !s.isShareUsable(share) {
		return nil, domain.ErrShareCancelled
	}
	if share.ResType != "note" {
		return nil, code.ErrorShareNotFound
	}

	// Reuse the share pipeline so attachment links inside the note are rewritten exactly
	// the way they are on the normal share page (titles, file URLs, path handling).
	// 复用分享管线，使正文中的附件引用与常规分享页保持完全一致的改写方式。
	token, err := s.tokenManager.ShareGenerate(share.ID, uid, share.Resources)
	if err != nil {
		return nil, err
	}
	post, err := s.GetSharedNote(ctx, token, share.ResID, "")
	if err != nil {
		return nil, err
	}

	html, err := s.renderNoteBody(ctx, uid, post.Content, preprocessPageContainers)
	if err != nil {
		return nil, err
	}

	out := &dto.BlogPostDTO{
		Slug:  s.BuildLocalShortLink(uid, sid),
		Title: strings.TrimSuffix(filepath.Base(post.Path), ".md"),
		HTML:  html,
		Path:  post.Path,
	}
	if !share.CreatedAt.IsZero() {
		out.CreatedAt = share.CreatedAt.Format("2006-01-02")
	}
	return out, nil
}

// renderNoteBody runs a note's Markdown through the full blog rendering pipeline.
//
// This is shared by article pages and by page notes (about / friends) so that a callout, a
// formula, a transclusion or a wiki link behaves identically in both. Duplicating the sequence
// instead is how the two would drift: the page would quietly stop honouring one of the passes,
// and the difference would only show up in whichever note happened to use it.
//
// `last` is an optional final Markdown pass, applied after callouts. Page container markers ride
// there because a container may wrap a callout, and the container itself must reach the renderer
// as Markdown rather than as pre-rendered HTML.
//
// renderNoteBody 让一篇笔记的 Markdown 走完整的博客渲染管线。
//
// 文章页与页面笔记（about / friends）共用它，因而 callout、公式、嵌入或 wiki 链接在两处的
// 表现完全一致。改成各抄一份，正是两者开始分叉的方式：页面会悄悄不再支持其中某一遍处理，
// 而这个差异只在恰好用到它的那篇笔记上才显形。
//
// last 是可选的最后一遍 Markdown 处理，在 callout 之后执行。页面容器标记走这里，
// 因为容器可能包住一个 callout，而容器本身必须以 Markdown 而不是预渲染 HTML 抵达渲染器。
func (s *shareService) renderNoteBody(ctx context.Context, uid int64, content string, last func(string) string) (string, error) {
	// Strip the frontmatter block before anything else looks at the text.
	//
	// Without this the block is rendered as Markdown, and Markdown treats it as a heading: the
	// opening `---` becomes a thematic break, the `key: value` lines become a paragraph, and the
	// closing `---` — sitting directly under that paragraph — becomes a setext underline, which
	// turns the whole paragraph into an <h2>. Measured on a note whose frontmatter was
	// `title: 关于我 / page: about`: the page showed a stray rule, a heading reading
	// "title: 关于我 page: about", and that heading was picked up by the table of contents.
	//
	// 368 of 2568 notes in this vault carry a frontmatter block, so this was not a corner case
	// waiting for an unusual note — it was waiting for any of them to be published.
	//
	// 在任何处理看到这段文本之前先剥掉 frontmatter 块。
	//
	// 不剥的话它会被当作 Markdown 渲染，而 Markdown 把它当成标题：开头的 `---` 成为分隔线，
	// `键: 值` 几行成为段落，而紧贴其下的收尾 `---` 成为 setext 下划线，
	// 于是整个段落变成了 <h2>。实测一篇 frontmatter 为 `title: 关于我 / page: about` 的笔记：
	// 页面出现一条多余的横线、一个内容为「title: 关于我 page: about」的标题，
	// 而且这个标题还被右侧目录收了上去。
	//
	// 本仓库 2568 篇笔记里有 368 篇带 frontmatter，所以这不是在等一篇特殊的笔记——
	// 它在等其中任何一篇被分享出来。
	content = frontmatterRegex.ReplaceAllString(content, "")
	if strings.TrimSpace(content) == "" {
		// A note that is nothing but frontmatter renders as an empty page. Returning empty is
		// correct here; the alternative is handing Markdown a document that no longer exists.
		//
		// 只有 frontmatter 的笔记会渲染成一个空页面。这里返回空是正确的；
		// 另一种做法是把一份已不存在的文档交给 Markdown。
		return "", nil
	}

	// Order matters.
	//
	//   1. expandTransclusions — must run first: it consumes the ![[Note]] syntax before any
	//      other pass can rewrite or drop it, and it inserts the target's Markdown so the later
	//      passes treat the embedded text exactly like the note's own text.
	//   2. rewriteWikiLinks — turns [[note]] (including any that arrived via transclusion) into
	//      real /blog/ links.
	//   3. preprocessMath — after link rewriting, so it never looks inside a link target.
	//   4. preprocessDynamicBlocks — after math so a "$" inside a dataview query is already
	//      protected, and before callouts so a dynamic block inside a callout is still handled
	//      (the callout body re-enters this pipeline).
	//   5. preprocessCallouts — its bodies are re-rendered through this same instance.
	//   6. last — page containers, if the caller supplied them.
	//
	// 顺序有讲究。
	//
	//   1. expandTransclusions —— 必须最先：它在任何其它处理改写或丢弃之前消费 ![[笔记]] 语法，
	//      并且插入的是目标笔记的 Markdown，使后续处理把嵌入文本与本文正文一视同仁。
	//   2. rewriteWikiLinks —— 把 [[笔记]]（含经嵌入带进来的）变成真实的 /blog/ 链接。
	//   3. preprocessMath —— 在链接改写之后，因此绝不会看到链接目标内部。
	//   4. preprocessDynamicBlocks —— 在数学之后，使 dataview 查询里的 "$" 已被保护；
	//      在 callout 之前，使 callout 内部的动态块仍能被处理（callout 正文会重入本管线）。
	//   5. preprocessCallouts —— 其正文会经由同一实例再次渲染。
	//   6. last —— 调用方提供的页面容器处理。
	expanded := s.expandTransclusions(ctx, uid, content, 0)
	prepared := preprocessCallouts(
		preprocessDynamicBlocks(preprocessMath(s.rewriteWikiLinks(ctx, uid, expanded))),
		func(md string) string {
			var inner bytes.Buffer
			// The callback gets its own id table: callout bodies are separate documents as far as
			// heading ids are concerned, and sharing one table would make a heading inside a
			// callout consume an id that a later top-level heading then has to skip.
			//
			// 回调使用自己的 id 表：就标题 id 而言，callout 正文是独立文档；共用一张表会让
			// callout 内部的标题占用一个 id，导致后续顶层标题不得不跳过它。
			if err := s.markdown.Convert([]byte(preprocessDynamicBlocks(preprocessMath(md))), &inner, blogHeadingIDOptions()...); err != nil {
				return ""
			}
			return inner.String()
		},
	)
	if last != nil {
		prepared = last(prepared)
	}

	var buf bytes.Buffer
	if err := s.markdown.Convert([]byte(prepared), &buf, blogHeadingIDOptions()...); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderBlogPage renders the single published note that declares the given page alias.
//
// It resolves the note from the same source list the blog already builds, so a page is subject to
// exactly the same publication rules as an article: unsharing it, or letting its share expire,
// removes the page. There is no second notion of "published" for pages.
//
// The body goes through renderNoteBody, the same pipeline an article uses, so a callout, a formula,
// a transclusion or a wiki link inside a page behaves the same as it does in an article. What
// differs is only the wrapper template.
//
// RenderBlogPage 渲染声明了指定页面别名的那一篇已发布笔记。
//
// 它从博客本已构建的同一份来源清单里解析笔记，因此页面与文章遵循完全相同的发布规则：
// 取消分享、或让分享过期，页面就消失。页面没有第二套「已发布」的定义。
//
// 正文走 renderNoteBody，也就是文章所用的同一条管线，因此页面里的 callout、公式、嵌入或
// wiki 链接与文章中表现一致。不同的只有外层模板。
// ── Blog reactions ────────────────────────────────────────────────────────────────────────────
// ── 博客反应 ──────────────────────────────────────────────────────────────────────────────────

// GetBlogReactions returns the current tallies.
//
// The whole map is returned rather than one note's tally because the home page needs all of them:
// the "hot" list sorts by likes, and the article page needs one. Two shapes for two callers would
// mean two code paths over the same file.
//
// GetBlogReactions 返回当前计数。
//
// 返回整张表而不是单篇的计数，因为首页需要全部：热门列表按赞数排序，而文章页只需要一个。
// 为两个调用方做两种形状，意味着同一份文件上有两条代码路径。
func (s *shareService) GetBlogReactions() *dto.BlogReactionsDTO {
	snap := s.blogReactions.snapshot()
	out := &dto.BlogReactionsDTO{
		Visits: snap.Visits,
		Notes:  make(map[string]*dto.BlogReactionTallyDTO, len(snap.Notes)),
	}
	for slug, t := range snap.Notes {
		out.Notes[slug] = &dto.BlogReactionTallyDTO{Like: t.Like, Dislike: t.Dislike}
	}
	return out
}

// ReactBlogNote applies one reaction and returns the note's new tally.
//
// kind must be "like" or "dislike"; anything else is rejected rather than ignored, so a typo in a
// client does not silently do nothing.
//
// ReactBlogNote 应用一次反应并返回该笔记的新计数。
//
// kind 必须是 "like" 或 "dislike"；其它取值被拒绝而不是忽略，使客户端的拼写错误不会静默地什么都不做。
func (s *shareService) ReactBlogNote(slug, kind string, delta int64) (*dto.BlogReactionTallyDTO, error) {
	if slug == "" {
		return nil, code.ErrorShareNotFound
	}
	if kind != "like" && kind != "dislike" {
		return nil, code.ErrorInvalidParams.WithDetails("reaction kind must be like or dislike")
	}
	// Clamp the step to its two legal values. A caller sending +5 would otherwise let one request
	// inflate a count, and the button only ever means +1 or -1.
	//
	// 把步长夹到两个合法值。否则一个发送 +5 的调用方就能让一次请求把计数抬高，
	// 而这个按钮的含义只有 +1 与 -1。
	step := int64(1)
	if delta < 0 {
		step = -1
	}
	t := s.blogReactions.react(slug, kind, step)
	return &dto.BlogReactionTallyDTO{Like: t.Like, Dislike: t.Dislike}, nil
}

// AddBlogVisit bumps the run-total visit counter.
// AddBlogVisit 累加访问计数。
func (s *shareService) AddBlogVisit() {
	s.blogReactions.addVisit()
}

func (s *shareService) RenderBlogPage(ctx context.Context, uid int64, alias string) (*dto.BlogPageDTO, error) {
	sources, _, err := s.loadBlogSources(ctx, uid)
	if err != nil {
		return nil, err
	}
	src, dup := pageNote(sources, alias)
	if src == nil {
		return nil, code.ErrorShareNotFound
	}

	// Resolve and rewrite attachments exactly as the article path does.
	//
	// The page path used to hand src.Content straight to renderNoteBody, and that pipeline has no
	// attachment step. "![[图片]]" therefore fell through to rewriteWikiLinks, whose stated job is
	// to "drop the embed syntax, keep the readable name" — correct for a note embed, wrong for a
	// file, so every image on a page rendered as its own file name and nothing else. Obsidian's
	// "![](path)" form was unaffected, which is why the defect could sit here unnoticed: the two
	// syntaxes take different routes and only one of them was broken.
	//
	// Articles never showed it because they enter through GetSharedNote, which resolves each
	// reference and swaps it for a share API URL before rendering. This does the same three steps
	// — parse the short link, mint the share token, fetch the rewritten note — rather than
	// duplicating the rewriting logic here.
	//
	// 与文章路径完全相同地解析并重写附件。
	//
	// 页面路径原本把 src.Content 直接交给 renderNoteBody，而那条管线里没有附件步骤。
	// 于是「![[图片]]」落到了 rewriteWikiLinks 手里，而它声明的职责是「去掉嵌入符号，保留可读名称」——
	// 对笔记嵌入是对的，对文件是错的，结果页面上每一张图片都只渲染成它自己的文件名。
	// Obsidian 的「![](路径)」写法不受影响，这正是这个缺陷能一直待在这里的原因：
	// 两种写法走不同的路，而只有一条是坏的。
	//
	// 文章从来看不到它，因为文章经由 GetSharedNote 进入，那一步会解析每个引用并在渲染前换成
	// 分享 API 地址。这里做的是同样的三步——解析短链、签发分享 token、取回重写后的笔记——
	// 而不是在本地复制一份重写逻辑。
	content := src.Content
	if src.Slug != "" {
		if pageUID, sid, perr := s.parseLocalShortLink(src.Slug); perr == nil {
			if share, serr := s.repo.GetByID(ctx, pageUID, sid); serr == nil && share != nil {
				if token, terr := s.tokenManager.ShareGenerate(share.ID, pageUID, share.Resources); terr == nil {
					if nd, nerr := s.GetSharedNote(ctx, token, share.ResID, ""); nerr == nil && nd != nil {
						content = nd.Content
					}
				}
			}
		}
	}

	html, err := s.renderNoteBody(ctx, uid, content, preprocessPageContainers)
	if err != nil {
		return nil, err
	}

	out := &dto.BlogPageDTO{
		Alias: alias,
		Slug:  src.Slug,
		// The page's own frontmatter title wins over the file name. See blogNoteTitle for why this
		// differs from the article path.
		//
		// 页面自己在 frontmatter 里写的标题优先于文件名。为何与文章路径不同，见 blogNoteTitle。
		Title: blogNoteTitle(src.Content, src.Title),
		// The container markers survive Markdown rendering as HTML comments, and are swapped for
		// real wrappers here — after the body exists, so the content between them was parsed as
		// ordinary Markdown rather than swallowed as a raw HTML block.
		//
		// 容器标记以 HTML 注释的形态穿过 Markdown 渲染，在这里换成真正的包裹层——
		// 在正文生成之后，因此它们之间的内容是按普通 Markdown 解析的，而不是被当作原始 HTML 块吞掉。
		HTML:       applyPageContainers(html),
		Path:       src.Path,
		Avatar:     src.Avatar,
		Duplicates: dup - 1,
	}
	if src.MTime > 0 {
		out.UpdatedAt = time.UnixMilli(src.MTime).Format("2006-01-02")
	}
	return out, nil
}

func (s *shareService) CreateShortLink(ctx context.Context, uid int64, vaultName string, path string, pathHash string, baseURL string, longURL string, isForce bool) (string, error) { // Find vault first to get ID
	vault, err := s.vaultRepo.GetByName(ctx, vaultName, uid)
	if err != nil {
		return "", err
	}
	if vault == nil {
		return "", code.ErrorVaultNotFound
	}

	// Find existing share record by path using vault.ID
	share, err := s.repo.GetByPath(ctx, uid, vault.ID, pathHash)
	if err != nil {
		return "", err
	}

	if share == nil {
		return "", code.ErrorShareNotFound
	}

	// If short link already exists and not forcing regeneration, return it
	if !isForce && share.ShortLink != "" {
		return share.ShortLink, nil
	}

	// Built-in short link mode: generate it locally, with no third-party call at all.
	// The stored value is a stable /s/<slug> URL that resolves to a freshly signed share
	// URL on every visit, so it never expires the way a third-party short link does.
	// 内置短链模式：本地生成，完全不调用第三方。保存的是稳定的 /s/<slug> 地址，
	// 每次访问都会解析为重新签发的分享地址，因此不会像第三方短链那样到期失效。
	if strings.EqualFold(strings.TrimSpace(s.config.App.ShortLink.Mode), "local") {
		shortURL := fmt.Sprintf("%s/s/%s", strings.TrimRight(baseURL, "/"), s.BuildLocalShortLink(uid, share.ID))
		if err := s.repo.UpdateShortLink(ctx, uid, share.ID, shortURL); err != nil {
			return "", err
		}
		return shortURL, nil
	}

	// Prepare short link creation parameters from service config
	sinkBaseURL := s.config.App.ShortLink.BaseURL
	apiKey := s.config.App.ShortLink.APIKey
	password := s.config.App.ShortLink.Password
	cloaking := s.config.App.ShortLink.Cloaking

	// expiration matches the share record
	expiresAt := share.ExpiresAt

	// Use client-provided URL if available; otherwise fall back to generating one
	// 优先使用客户端传入的完整分享 URL，避免因重新生成 token 导致 URL 不一致
	if longURL == "" {
		token, err := s.tokenManager.ShareGenerate(share.ID, uid, share.Resources)
		if err != nil {
			return "", err
		}
		longURL = fmt.Sprintf("%s/share/%d/%s", strings.TrimRight(baseURL, "/"), share.ResID, token)
	}

	client := shortlink.NewSinkCoolClient(sinkBaseURL, apiKey)

	title := ""
	switch share.ResType {
	case "note":
		if note, err := s.noteRepo.GetByID(ctx, share.ResID, uid); err == nil && note != nil {
			title = strings.TrimSuffix(filepath.Base(note.Path), ".md")
		}
	case "file":
		if file, err := s.fileRepo.GetByID(ctx, share.ResID, uid); err == nil && file != nil {
			title = filepath.Base(file.Path)
		}
	}

	shortURL, err := client.Create(longURL, expiresAt, password, cloaking, title)

	if err != nil {
		return "", err
	}

	// Save the generated short link back to database
	if err := s.repo.UpdateShortLink(ctx, uid, share.ID, shortURL); err != nil {
		return "", err
	}

	return shortURL, nil
}

// StopShareByPath revokes a share by path
// StopShareByPath 根据路径撤销分享
func (s *shareService) StopShareByPath(ctx context.Context, uid int64, vaultName string, pathHash string) error {
	share, err := s.GetShareByPath(ctx, uid, vaultName, pathHash)
	if err != nil {
		return err
	}
	return s.StopShare(ctx, uid, share.ID)
}

// GetActiveNotePathsByVault returns active shared note paths for a vault
// GetActiveNotePathsByVault 返回指定 vault 下所有有效分享的笔记路径（两步查询，避免跨库 JOIN）
func (s *shareService) GetActiveNotePathsByVault(ctx context.Context, uid int64, vaultName string) ([]string, error) {
	vault, err := s.vaultRepo.GetByName(ctx, vaultName, uid)
	if err != nil {
		return nil, err
	}

	// Step 1: query active note res_ids from user_shares DB (no cross-DB JOIN)
	// 步骤1：从 user_shares 库查出有效分享的 note res_id 列表（不做跨库 JOIN）
	noteIDs, err := s.repo.ListActiveNoteResIDs(ctx, uid)
	if err != nil {
		return nil, err
	}
	if len(noteIDs) == 0 {
		return []string{}, nil
	}

	// Step 2: batch query notes from notes DB, filter by vault and non-deleted action
	// 步骤2：从 notes 库批量查笔记，按 vault 和非删除状态过滤
	notes, err := s.noteRepo.ListByIDs(ctx, noteIDs, uid)
	if err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(notes))
	for _, n := range notes {
		if n.VaultID == vault.ID && n.Action != domain.NoteActionDelete {
			paths = append(paths, n.Path)
		}
	}
	return paths, nil
}

// GetSharedNote retrieves specific shared note details
// GetSharedNote 获取分享的单条笔记详情
func (s *shareService) GetSharedNote(ctx context.Context, shareToken string, noteID int64, password string) (*dto.NoteDTO, error) {
	ridStr := strconv.FormatInt(noteID, 10)
	shareEntity, err := s.VerifyShare(ctx, shareToken, ridStr, "note", password)
	if err != nil {
		if cObj, ok := err.(*code.Code); ok {
			return nil, cObj
		}
		return nil, code.ErrorInvalidAuthToken.WithDetails(err.Error())
	}

	// Retrieve note directly via ID (using resource owner's UID)
	// 直接通过 ID 获取笔记 (使用资源所有者的 UID)
	note, err := s.noteRepo.GetByID(ctx, noteID, shareEntity.UID)
	if err != nil {
		return nil, code.ErrorNoteNotFound
	}

	// Reject genuinely deleted notes.
	// Rename leaves a tombstone with Rename=1 and must stay readable (share links keep
	// working after a file is renamed/moved — see MigrateResID), but a real delete
	// (single-note delete or a whole folder being removed) sets Rename=0 and must not
	// leak content through previously issued share links.
	// 拒绝真正被删除的笔记：重命名留下的墓碑 Rename=1 必须保持可读（文件改名后分享链接继续
	// 可用），而真正的删除（单篇删除或整个目录被删除）会把 Rename 置 0，此时不得再通过旧分享
	// 链接读出正文。
	if note.Action == domain.NoteActionDelete && note.Rename == 0 {
		return nil, code.ErrorNoteNotFound
	}

	noteDTO := &dto.NoteDTO{
		ID:               note.ID,
		Path:             note.Path,
		PathHash:         note.PathHash,
		Content:          note.Content,
		ContentHash:      note.ContentHash,
		Version:          note.Version,
		Ctime:            note.Ctime,
		Mtime:            note.Mtime,
		UpdatedTimestamp: note.UpdatedTimestamp,
		UpdatedAt:        timex.Time(note.UpdatedAt),
		CreatedAt:        timex.Time(note.CreatedAt),
	}

	fileRefs, err := s.resolveSharedNoteFiles(ctx, shareEntity.UID, note.VaultID, note.Path, noteDTO.Content)
	if err != nil {
		s.logger.Warn("GetSharedNote resolveSharedNoteFiles failed", zap.Error(err), zap.String("notePath", note.Path))
	}

	if len(fileRefs) > 0 {
		updatedResources, changed := mergeShareFileResources(shareEntity.Resources, fileRefs)
		if changed {
			if err := s.repo.UpdateResources(ctx, shareEntity.UID, shareEntity.SID, updatedResources); err != nil {
				s.logger.Warn("GetSharedNote UpdateResources failed", zap.Error(err), zap.Int64("shareID", shareEntity.SID))
			} else {
				shareEntity.Resources = updatedResources
			}
		}
	}

	// Handle Obsidian attachment embedded tags ![[...]]
	// 处理 Obsidian 附件嵌入标签 ![[...]]
	newContent := attachmentRegex.ReplaceAllStringFunc(noteDTO.Content, func(match string) string {
		submatches := attachmentRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}

		inner := submatches[1]
		rawPath := inner
		options := ""

		// Extract resource path (remove part after alias | and anchor #)
		// 提取资源路径（移除别名 | 和锚点 # 之后的部分）
		if idx := strings.IndexAny(inner, "|#"); idx != -1 {
			rawPath = inner[:idx]
			if inner[idx] == '|' {
				options = strings.TrimSpace(inner[idx+1:])
			}
		}
		rawPath = strings.TrimSpace(rawPath)
		if rawPath == "" {
			return match
		}

		file := fileRefs[rawPath]
		if file == nil {
			return match
		}

		apiUrl := buildSharedFileAPIURL(file.ID, shareToken, password)
		lowerPath := strings.ToLower(file.Path)
		ext := filepath.Ext(lowerPath)

		isImage := strings.Contains(".png.jpg.jpeg.gif.svg.webp.bmp", ext) && ext != ""
		isVideo := strings.Contains(".mp4.webm.ogg.mov", ext) && ext != ""
		isAudio := strings.Contains(".mp3.wav.ogg.m4a.flac", ext) && ext != ""

		if isImage {
			width := ""
			height := ""
			if options != "" {
				sizeRe := regexp.MustCompile(`^(\d+)(?:x(\d+))?`)
				sizeMatch := sizeRe.FindStringSubmatch(options)
				if len(sizeMatch) > 1 && sizeMatch[1] != "" {
					width = ` width="` + sizeMatch[1] + `"`
				}
				if len(sizeMatch) > 2 && sizeMatch[2] != "" {
					height = ` height="` + sizeMatch[2] + `"`
				}
			}
			return `<img src="` + apiUrl + `" alt="` + rawPath + `"` + width + height + ` />`
		} else if isVideo {
			return `<video src="` + apiUrl + `" controls style="max-width:100%"></video>`
		} else if isAudio {
			return `<audio src="` + apiUrl + `" controls></audio>`
		} else {
			return `<a href="` + apiUrl + `" target="_blank">📎 ` + rawPath + `</a>`
		}
	})
	newContent = rewriteMarkdownImageLinks(newContent, fileRefs, shareToken, password)
	newContent = rewriteHTMLImageSources(newContent, fileRefs, shareToken, password)
	// Rewrite any inline HTML <video>/<audio>/<source> the user wrote
	// directly in their note. Each tag has the same `src` capture shape as
	// <img>, so reuse rewriteHTMLMediaSources for all three. This pairs with
	// the htmlMediaRegexes loop in extractSharedNoteFileRefs which gates
	// share authorization; without the rewrite, src would still point at the
	// raw vault path and the share viewer's <video>/<audio> player could not
	// fetch the file (it has no auth context for the vault file route).
	// 把用户直接写在笔记里的内联 HTML <video>/<audio>/<source> 一并改写。
	// 三者 src 的捕获形态与 <img> 完全一致，因此复用 rewriteHTMLMediaSources。
	// 该步骤与 extractSharedNoteFileRefs 中的 htmlMediaRegexes 循环成对使用：
	// 提取负责把 src 路径放进分享授权资源，重写则把 src 替换为分享 API URL，
	// 否则分享视图里的播放器拿到的是原始 vault 路径，没有授权也就无法加载。
	newContent = rewriteHTMLMediaSources(newContent, htmlVideoRegex, "video", fileRefs, shareToken, password)
	newContent = rewriteHTMLMediaSources(newContent, htmlAudioRegex, "audio", fileRefs, shareToken, password)
	newContent = rewriteHTMLMediaSources(newContent, htmlSourceRegex, "source", fileRefs, shareToken, password)
	noteDTO.Content = newContent

	return noteDTO, nil
}

func (s *shareService) resolveSharedNoteFiles(ctx context.Context, uid int64, vaultID int64, notePath string, content string) (map[string]*domain.File, error) {
	rawRefs := extractSharedNoteFileRefs(content)
	if len(rawRefs) == 0 {
		return map[string]*domain.File{}, nil
	}

	result := make(map[string]*domain.File, len(rawRefs))
	for _, rawRef := range rawRefs {
		file, err := s.resolveSharedFileReference(ctx, uid, vaultID, notePath, rawRef)
		if err != nil {
			return nil, err
		}
		if file != nil {
			result[rawRef] = file
		}
	}
	return result, nil
}

func (s *shareService) resolveSharedFileReference(ctx context.Context, uid int64, vaultID int64, notePath string, rawRef string) (*domain.File, error) {
	ref := strings.TrimSpace(rawRef)
	if !isLocalSharePath(ref) {
		return nil, nil
	}

	for _, candidate := range buildSharePathCandidates(notePath, ref) {
		file, err := s.fileRepo.GetByPath(ctx, candidate, vaultID, uid)
		if err == nil && file != nil && file.Action != domain.FileActionDelete {
			return file, nil
		}
	}

	normalizedRef := normalizeShareVaultPath(ref)
	if normalizedRef != "" && !strings.Contains(normalizedRef, "/") {
		file, err := s.fileRepo.GetByPathLike(ctx, normalizedRef, vaultID, uid)
		if err == nil && file != nil && file.Action != domain.FileActionDelete {
			return file, nil
		}
	}

	// 精确候选与「不含目录的引用」兜底全部落空后，再退化为 basename 后缀匹配。
	//
	// 覆盖从 WPS/Word 粘贴进来的 HTML 引用，例如
	//   <img src="images/<hash>.png">
	// 这类「带目录的相对引用」在服务端的实体常常落在另一个目录
	// （如 图片/images/<hash>.png，或附件被整理后重新归类），按 noteDir
	// 拼出的精确路径永远对不上；而 Obsidian 原生嵌入 ![[<hash>.png]] 正因
	// ref 不含 '/' 才一直享有上面那条 GetByPathLike 兜底。
	// 缺了这一段，同一张图用 wiki 语法能显示、改用 HTML 语法就会降级成
	// 「图片未同步到服务端」。
	if base := path.Base(ref); base != "." && base != "/" && base != "" && base != ref {
		file, err := s.fileRepo.GetByPathLike(ctx, base, vaultID, uid)
		if err == nil && file != nil && file.Action != domain.FileActionDelete {
			return file, nil
		}
	}

	return nil, nil
}

func extractSharedNoteFileRefs(content string) []string {
	seen := make(map[string]struct{})
	refs := make([]string, 0)

	for _, match := range attachmentRegex.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		ref := extractObsidianEmbedPath(match[1])
		if ref == "" || !isLocalSharePath(ref) {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}

	for _, match := range markdownImageRegex.FindAllStringSubmatch(content, -1) {
		if len(match) < 3 {
			continue
		}
		ref, _, _ := parseMarkdownLinkTarget(match[2])
		ref = strings.TrimSpace(ref)
		if ref == "" || !isLocalSharePath(ref) {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}

	for _, match := range htmlImageRegex.FindAllStringSubmatch(content, -1) {
		if len(match) < 4 {
			continue
		}
		ref := strings.TrimSpace(match[3])
		if ref == "" || !isLocalSharePath(ref) {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}

	// HTML media tags (<video src=...>, <audio src=...>, <source src=...>)
	// share the same `src` capture position as <img>, so a single loop over
	// htmlMediaRegexes is enough to extract their refs into the file map.
	// Without this, shared notes containing raw HTML like
	//   <video src="assets/clip.mp4" controls></video>
	// would never have "assets/clip.mp4" added to fileLinks, leaving the
	// downstream rewriter unable to swap the path for an authorized share
	// API URL and the embedded player unable to load the file.
	// HTML 媒体标签（<video src=...>、<audio src=...>、<source src=...>）的
	// `src` 捕获位与 <img> 一致，因此遍历 htmlMediaRegexes 即可。如果不在此
	// 处提取，分享笔记中如下这类原生 HTML 的 src 永远不会进入 fileLinks，
	// 后续的重写器也就无法把路径替换为带授权信息的分享 API URL，前端的播放
	// 器自然就加载不到文件。
	for _, re := range htmlMediaRegexes {
		for _, match := range re.FindAllStringSubmatch(content, -1) {
			if len(match) < 4 {
				continue
			}
			ref := strings.TrimSpace(match[3])
			if ref == "" || !isLocalSharePath(ref) {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			refs = append(refs, ref)
		}
	}

	return refs
}

func extractObsidianEmbedPath(inner string) string {
	rawPath := inner
	if idx := strings.IndexAny(inner, "|#"); idx != -1 {
		rawPath = inner[:idx]
	}
	return strings.TrimSpace(rawPath)
}

func parseMarkdownLinkTarget(raw string) (target string, start int, end int) {
	start = 0
	for start < len(raw) {
		switch raw[start] {
		case ' ', '\t', '\n':
			start++
		default:
			goto targetStart
		}
	}
	return "", -1, -1

targetStart:
	if raw[start] == '<' {
		end = strings.IndexByte(raw[start+1:], '>')
		if end == -1 {
			return "", -1, -1
		}
		end += start + 2
		return raw[start+1 : end-1], start, end
	}

	end = len(raw)
	escaped := false
	for i := start; i < len(raw); i++ {
		if escaped {
			escaped = false
			continue
		}
		switch raw[i] {
		case '\\':
			escaped = true
		case ' ', '\t', '\n':
			end = i
			return raw[start:end], start, end
		}
	}

	return raw[start:end], start, end
}

// detectMediaKindByExt returns "image", "video", "audio", or "" based on the
// file extension of p. Extensions are matched case-insensitively.
// detectMediaKindByExt 根据 p 的扩展名返回 "image"、"video"、"audio" 或 ""，
// 大小写不敏感。
func detectMediaKindByExt(p string) string {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".bmp":
		return "image"
	case ".mp4", ".webm", ".mov", ".mkv", ".avi", ".ogv":
		return "video"
	case ".mp3", ".wav", ".m4a", ".flac", ".aac", ".ogg":
		return "audio"
	case ".pdf":
		return "pdf"
	}
	return ""
}

// parseSizeFromAlt reads a width (or width x height) from an alt value.
//
// It is deliberately conservative. A caption such as "2026" or "1" is a plausible string and an
// implausible picture width, so the range check is what separates the two; and because a wrong
// size silently distorts the layout, anything ambiguous is left as a caption.
//
// parseSizeFromAlt 从 alt 值读取宽度（或 宽 x 高）。
//
// 它刻意保守。像 "2026" 或 "1" 这样的图注是合理的字符串但作为图片宽度不合理，
// 因此区间检查就是区分两者的依据；而错误的尺寸会静默破坏版式，所以任何含糊的情况都保留为图注。
func parseSizeFromAlt(alt string) (int, int, bool) {
	alt = strings.TrimSpace(alt)
	if alt == "" {
		return 0, 0, false
	}
	w, h := 0, 0
	if i := strings.IndexByte(alt, 'x'); i > 0 {
		var ok bool
		if w, ok = atoiPositive(alt[:i]); !ok {
			return 0, 0, false
		}
		if h, ok = atoiPositive(alt[i+1:]); !ok {
			return 0, 0, false
		}
	} else {
		var ok bool
		if w, ok = atoiPositive(alt); !ok {
			return 0, 0, false
		}
	}
	// The ceiling is the load-bearing choice here. The article column is 900px wide and every
	// image gets max-width:100%, so a width above roughly the column's own size has no visible
	// effect — which means large numbers are far more likely to be captions than widths.
	// Measured: the vault's real sizes are 196-634; a year-shaped caption such as "2026" must
	// stay a caption, and it does, because 2026 is above the ceiling.
	//
	// 上界是这里的关键取舍。内容列宽 900px，且每张图都带 max-width:100%，
	// 因此大于约等于列宽的数字不会产生可见效果——这说明大数字更可能是图注而不是宽度。
	// 实测：仓库中的真实尺寸是 196–634；像 "2026" 这样的年份形态图注必须保持为图注，
	// 而它确实如此，因为 2026 高于上界。
	const minWidth, maxWidth = 20, 1200
	if w < minWidth || w > maxWidth {
		return 0, 0, false
	}
	if h != 0 && (h < minWidth || h > maxWidth) {
		return 0, 0, false
	}
	return w, h, true
}

// atoiPositive parses a non-negative decimal integer without pulling in strconv.
// atoiPositive 解析一个非负十进制整数，避免引入 strconv。
func atoiPositive(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
		if n > 100000 {
			return 0, false
		}
	}
	return n, true
}

// renderPDFReference decides between an inline viewer and a plain link.
//
// Inline embedding is a real improvement for a small handout — the reader stays in the article —
// but an <embed> makes the browser fetch the entire document at page load, so a scanned textbook
// would turn opening a page into a download. Above the cap the link-only treatment is kept, and
// the reader is told why.
//
// renderPDFReference 在内联阅读器与普通链接之间做选择。
//
// 对一份小讲义来说内联确实是改进——读者留在文章里——但 <embed> 会让浏览器在页面加载时就取回
// 整个文档，于是一本扫描教材会把「打开一页」变成一次下载。超过上限时保持仅链接的处理，
// 并告诉读者原因。
func renderPDFReference(apiURL, path string, size int64, alt string) string {
	label := alt
	if strings.TrimSpace(label) == "" {
		label = noteTitle(path)
	}
	esc := func(s string) string { return calloutHTMLReplacer.Replace(s) }

	if size > 0 && size <= pdfEmbedMaxBytes {
		return `<figure class="pdfembed"><embed src="` + apiURL + `" type="application/pdf" ` +
			`title="` + esc(label) + `" /><figcaption><a href="` + apiURL +
			`" target="_blank" rel="noopener noreferrer">` + esc(label) + `（在新标签页打开）</a></figcaption></figure>`
	}

	reason := "（文件较大，未内联）"
	if size == 0 {
		reason = ""
	}
	return `<a class="pdflink" href="` + apiURL + `" target="_blank" rel="noopener noreferrer">📄 ` +
		esc(label) + `</a><span class="pdfnote">` + reason + `</span>`
}

func rewriteMarkdownImageLinks(content string, fileRefs map[string]*domain.File, shareToken string, password string) string {
	return markdownImageRegex.ReplaceAllStringFunc(content, func(match string) string {
		submatches := markdownImageRegex.FindStringSubmatch(match)
		if len(submatches) < 3 {
			return match
		}

		target, start, end := parseMarkdownLinkTarget(submatches[2])
		if target == "" || start < 0 || end < 0 {
			return match
		}

		file := fileRefs[strings.TrimSpace(target)]
		if file == nil {
			return match
		}

		apiURL := buildSharedFileAPIURL(file.ID, shareToken, password)
		alt := submatches[1]

		// Dispatch by file extension so that markdown image syntax pointing
		// at a video or audio file (e.g. `![alt](clip.mp4)`) is rendered as
		// a proper <video>/<audio> HTML element in the share view, instead
		// of a broken <img>. This mirrors the wiki-style ![[]] dispatch
		// already done in the attachment rewriter above, and is the server-
		// side counterpart of the webgui's rewriteMarkdownImageLinks media
		// dispatch.
		// 按文件扩展名分发：让 Markdown 图片语法指向视频/音频的引用
		// （如 `![alt](clip.mp4)`）在分享视图中渲染为 <video>/<audio> HTML，
		// 而不是坏掉的 <img>。与上面 wiki 风格 ![[]] 附件重写器的分发逻辑
		// 一致，也是 webgui 端 rewriteMarkdownImageLinks 媒体分发的服务端
		// 对应物。
		switch detectMediaKindByExt(file.Path) {
		case "video":
			return `<video src="` + apiURL + `" controls style="max-width:100%"></video>`
		case "audio":
			return `<audio src="` + apiURL + `" controls></audio>`
		case "pdf":
			return renderPDFReference(apiURL, file.Path, file.Size, alt)
		}

		// An alt that is a bare number is a width, not a caption.
		//
		// Measured in this vault: 24 occurrences of `![N](image.jpg)` with N in
		// {196, 281, 313, 314, 376, 634}, all inside the range a picture actually occupies and
		// repeated in groups of four (the same image referenced from four notes). That is the
		// obsidian-adjustable-media convention, which puts the size in the alt slot because
		// Markdown image syntax has nowhere else to put it.
		//
		// Guarded so a genuine numeric caption is not turned into a size: the number must parse,
		// must be plausible as a width, and the target must actually be an image.
		//
		// 纯数字的 alt 是宽度，不是图注。
		//
		// 本仓库实测：24 处 `![N](image.jpg)`，N 取值 {196, 281, 313, 314, 376, 634}，
		// 全部落在图片实际占据的范围内，并且每四个一组重复（同一张图被四篇笔记引用）。
		// 这是 obsidian-adjustable-media 的约定：把尺寸放在 alt 槽位，因为 Markdown 图片语法
		// 没有别的地方可放。
		//
		// 加上守卫，避免把真正的数字图注当成尺寸：数字必须能解析、必须作为宽度合理、
		// 且目标确实是一张图片。
		if w, h, ok := parseSizeFromAlt(alt); ok && detectMediaKindByExt(file.Path) == "image" {
			// max-width keeps a large requested width from overflowing the column, and the
			// explicit height/aspect handling is why the height form is written separately:
			// pinning both dimensions must not also force max-width:100% onto the height.
			//
			// max-width 防止请求的较大宽度溢出内容列；高度形态单独书写的原因正在于此：
			// 同时钉住两个尺寸时，不应把 max-width:100% 也套到高度上。
			style := ` style="width:` + itoa(int64(w)) + `px;max-width:100%;height:auto"`
			if h > 0 {
				style = ` style="width:` + itoa(int64(w)) + `px;height:` + itoa(int64(h)) + `px;max-width:100%"`
			}
			return `<img src="` + apiURL + `" alt=""` + style + ` />`
		}

		// Image (default) — preserve the original markdown image form, only
		// substituting the URL.
		replacementTarget := apiURL
		rawTarget := submatches[2][start:end]
		if strings.HasPrefix(rawTarget, "<") && strings.HasSuffix(rawTarget, ">") {
			replacementTarget = "<" + replacementTarget + ">"
		}

		return "![" + alt + "](" + submatches[2][:start] + replacementTarget + submatches[2][end:] + ")"
	})
}

func rewriteHTMLImageSources(content string, fileRefs map[string]*domain.File, shareToken string, password string) string {
	return htmlImageRegex.ReplaceAllStringFunc(content, func(match string) string {
		submatches := htmlImageRegex.FindStringSubmatch(match)
		if len(submatches) < 5 {
			return match
		}

		file := fileRefs[strings.TrimSpace(submatches[3])]
		if file == nil {
			return match
		}

		return "<img" + submatches[1] + "src=" + submatches[2] + buildSharedFileAPIURL(file.ID, shareToken, password) + submatches[2] + submatches[4] + ">"
	})
}

// rewriteHTMLMediaSources rewrites the `src` attribute on every HTML media
// tag matched by `re` (e.g. <video>, <audio>, <source>) to the per-file
// share API URL when the original src resolves through fileRefs. The match
// shape is identical to htmlImageRegex (groups: beforeSrc, quote, rawSrc,
// afterSrc), so the replacement reuses the same template, including
// preservation of any trailing `/` from a self-closing form like
// `<source ... />` (which falls inside `afterSrc`).
//
// This is the pipeline counterpart of extractSharedNoteFileRefs's media
// loop: extraction populates fileRefs, rewriting then swaps paths for
// authorized share URLs so the embedded player can fetch the file.
//
// rewriteHTMLMediaSources 将匹配到的 HTML 媒体标签（<video>/<audio>/<source>）
// 的 src 属性改写为按文件 ID 生成的分享 API URL；正则的捕获位与
// htmlImageRegex 完全一致，所以替换模板可以复用，包括把 `<source ... />`
// 这种自闭合形式的尾部 `/`（落在 afterSrc 中）保留下来。
//
// 该函数与 extractSharedNoteFileRefs 中的媒体提取循环成对使用：先在提取阶段
// 把 src 加入 fileRefs，然后在此处把 src 替换为带授权的分享 URL，前端播放器
// 才能拉到对应文件。
func rewriteHTMLMediaSources(content string, re *regexp.Regexp, tagName string, fileRefs map[string]*domain.File, shareToken string, password string) string {
	return re.ReplaceAllStringFunc(content, func(match string) string {
		submatches := re.FindStringSubmatch(match)
		if len(submatches) < 5 {
			return match
		}

		file := fileRefs[strings.TrimSpace(submatches[3])]
		if file == nil {
			return match
		}

		return "<" + tagName + submatches[1] + "src=" + submatches[2] + buildSharedFileAPIURL(file.ID, shareToken, password) + submatches[2] + submatches[4] + ">"
	})
}

func buildSharedFileAPIURL(fileID int64, shareToken string, password string) string {
	apiURL := "/api/share/file?id=" + strconv.FormatInt(fileID, 10) + "&share_token=" + shareToken
	if password != "" {
		apiURL += "&password=" + password
	}
	return apiURL
}

func mergeShareFileResources(resources map[string][]string, fileRefs map[string]*domain.File) (map[string][]string, bool) {
	merged := cloneShareResources(resources)
	allowed := make(map[string]struct{}, len(merged["file"]))
	for _, id := range merged["file"] {
		allowed[id] = struct{}{}
	}

	changed := false
	for _, file := range fileRefs {
		id := strconv.FormatInt(file.ID, 10)
		if _, ok := allowed[id]; ok {
			continue
		}
		merged["file"] = append(merged["file"], id)
		allowed[id] = struct{}{}
		changed = true
	}

	if changed {
		sort.Strings(merged["file"])
	}

	return merged, changed
}

func cloneShareResources(resources map[string][]string) map[string][]string {
	cloned := make(map[string][]string, len(resources))
	for key, values := range resources {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func buildSharePathCandidates(notePath string, rawRef string) []string {
	ref := strings.TrimSpace(strings.ReplaceAll(rawRef, "\\", "/"))
	if ref == "" || !isLocalSharePath(ref) {
		return nil
	}

	// Standard markdown image links require characters such as spaces or
	// non-ASCII to be percent-encoded (e.g. "图片/foo%201.jpg"), but the
	// canonical path stored in the file table is the literal Obsidian path
	// (e.g. "图片/foo 1.jpg"). Decode percent-encoding here so the DB lookup
	// matches regardless of which link form the note used.
	// 标准 Markdown 图片链接中的空格、非 ASCII 字符等需要百分号编码（如
	// "图片/foo%201.jpg"），但文件表里存的是 Obsidian 原始字面路径
	// （如 "图片/foo 1.jpg"）。这里解码百分号编码，让数据库查找在任何
	// 链接形式下都能命中。
	if decoded, err := url.PathUnescape(ref); err == nil && decoded != "" {
		ref = decoded
	}

	candidates := make([]string, 0, 2)
	addCandidate := func(candidate string) {
		candidate = normalizeShareVaultPath(candidate)
		if candidate == "" {
			return
		}
		for _, existing := range candidates {
			if existing == candidate {
				return
			}
		}
		candidates = append(candidates, candidate)
	}

	noteDir := normalizeShareVaultPath(path.Dir(strings.ReplaceAll(notePath, "\\", "/")))
	if noteDir == "." {
		noteDir = ""
	}
	if noteDir != "" {
		addCandidate(path.Join(noteDir, ref))
	} else {
		addCandidate(ref)
	}
	if strings.Contains(ref, "/") && !strings.HasPrefix(ref, "./") && !strings.HasPrefix(ref, "../") {
		addCandidate(ref)
	}

	return candidates
}

func normalizeShareVaultPath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}

	cleaned := path.Clean(p)
	if cleaned == "." || cleaned == "/" || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

func isLocalSharePath(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}

	lowerRef := strings.ToLower(ref)
	switch {
	case strings.HasPrefix(ref, "#"):
		return false
	case strings.HasPrefix(ref, "/"):
		return false
	case strings.HasPrefix(lowerRef, "http://"):
		return false
	case strings.HasPrefix(lowerRef, "https://"):
		return false
	case strings.HasPrefix(lowerRef, "//"):
		return false
	case strings.HasPrefix(lowerRef, "data:"):
		return false
	case strings.HasPrefix(lowerRef, "mailto:"):
		return false
	case strings.HasPrefix(lowerRef, "tel:"):
		return false
	case strings.HasPrefix(lowerRef, "obsidian://"):
		return false
	default:
		return true
	}
}

// GetSharedFile retrieves shared file content
// GetSharedFile 获取分享的文件内容
func (s *shareService) GetSharedFile(ctx context.Context, shareToken string, fileID int64, password string) (content []byte, contentType string, mtime int64, etag string, fileName string, err error) {
	ridStr := strconv.FormatInt(fileID, 10)
	shareEntity, err := s.VerifyShare(ctx, shareToken, ridStr, "file", password)
	if err != nil {
		if cObj, ok := err.(*code.Code); ok {
			return nil, "", 0, "", "", cObj
		}
		return nil, "", 0, "", "", code.ErrorInvalidAuthToken.WithDetails(err.Error())
	}

	// 1. Get resource owner's UID
	// 1. 获取资源所有者的 UID
	ownerUID := shareEntity.UID

	// 2. Confirm path hash (get file metadata from fileRepo)
	// 2. 确认路径哈希 (从 fileRepo 获取文件元数据)
	file, err := s.fileRepo.GetByID(ctx, fileID, ownerUID)
	if err != nil {
		return nil, "", 0, "", "", code.ErrorFileNotFound
	}

	if file.Action == domain.FileActionDelete {
		return nil, "", 0, "", "", code.ErrorFileNotFound
	}

	// Read physical file content
	// 读取物理文件内容
	content, err = os.ReadFile(file.SavePath)
	if err != nil {
		return nil, "", 0, "", "", code.ErrorFileReadFailed.WithDetails(err.Error())
	}

	// Identify file MIME type
	// 识别文件 MIME 类型
	ext := filepath.Ext(file.Path)
	contentType = mime.TypeByExtension(ext)
	if contentType == "" {
		// If extension cannot be identified, perform content sniffing
		// 如果扩展名识别不到, 进行内容嗅探
		contentType = http.DetectContentType(content)
	}

	// Compute etag in real-time using byte-based hash for consistency with binary files
	// 使用基于字节的哈希实时计算 etag，确保与二进制文件一致
	etag = util.EncodeHash32Bytes(content)

	return content, contentType, file.Mtime, etag, file.Path, nil

}

// GetSharedFileInfo retrieves shared file metadata and path for zero-copy download
// GetSharedFileInfo 获取分享文件的元数据和路径，用于零拷贝下载
func (s *shareService) GetSharedFileInfo(ctx context.Context, shareToken string, fileID int64, password string) (savePath string, contentType string, mtime int64, etag string, fileName string, err error) {
	ridStr := strconv.FormatInt(fileID, 10)
	shareEntity, err := s.VerifyShare(ctx, shareToken, ridStr, "file", password)
	if err != nil {
		if cObj, ok := err.(*code.Code); ok {
			return "", "", 0, "", "", cObj
		}
		return "", "", 0, "", "", code.ErrorInvalidAuthToken.WithDetails(err.Error())
	}

	// 1. Get resource owner's UID
	ownerUID := shareEntity.UID

	// 2. Confirm path hash (get file metadata from fileRepo)
	file, err := s.fileRepo.GetByID(ctx, fileID, ownerUID)
	if err != nil {
		return "", "", 0, "", "", code.ErrorFileNotFound
	}

	if file.Action == domain.FileActionDelete {
		return "", "", 0, "", "", code.ErrorFileNotFound
	}

	// Identify file MIME type
	ext := filepath.Ext(file.Path)
	contentType = mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Use file's content hash as ETag
	etag = file.ContentHash
	if etag == "" {
		etag = file.PathHash
	}

	return file.SavePath, contentType, file.Mtime, etag, filepath.Base(file.Path), nil
}

// Shutdown shuts down the service and flushes remaining data
// Shutdown 关闭服务并同步最后的数据
func (s *shareService) Shutdown(ctx context.Context) error {
	s.ticker.Stop()
	close(s.stopCh)

	// Wait for periodic synchronization goroutine to end (i.e., last flush completed)
	// 等待定时同步协程结束（即最后一次 flush 完成）
	select {
	case <-s.doneCh:
		s.logger.Info("ShareService background flush loop stopped")
		return nil
	case <-ctx.Done():
		s.logger.Warn("ShareService shutdown timeout, some data might not be flushed")
		return ctx.Err()
	}
}
