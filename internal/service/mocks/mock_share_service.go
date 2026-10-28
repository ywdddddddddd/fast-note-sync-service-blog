// Package mocks provides testify/mock implementations for service interfaces.
// Package mocks 提供服务接口的 testify/mock 实现。
package mocks

import (
	"context"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"github.com/haierkeys/fast-note-sync-service/internal/service"
	pkgapp "github.com/haierkeys/fast-note-sync-service/pkg/app"
	"github.com/stretchr/testify/mock"
)

// MockShareService is a testify/mock implementation of service.ShareService.
// MockShareService 是 service.ShareService 的 testify/mock 实现。
type MockShareService struct {
	mock.Mock
}

// Ensure MockShareService implements service.ShareService at compile time.
// 编译期确保 MockShareService 实现了 service.ShareService 接口。
var _ service.ShareService = (*MockShareService)(nil)

func (m *MockShareService) ShareGenerate(ctx context.Context, uid int64, vaultName string, path string, pathHash string, password string, expireAt int64) (*dto.ShareCreateResponse, error) {
	args := m.Called(ctx, uid, vaultName, path, pathHash, password, expireAt)
	if v := args.Get(0); v != nil {
		return v.(*dto.ShareCreateResponse), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockShareService) VerifyShare(ctx context.Context, token string, rid string, rtp string, password string) (*pkgapp.ShareEntity, error) {
	args := m.Called(ctx, token, rid, rtp, password)
	if v := args.Get(0); v != nil {
		return v.(*pkgapp.ShareEntity), args.Error(1)
	}
	return nil, args.Error(1)
}

// ResolveLocalShortLink resolves a built-in short link slug.
// ResolveLocalShortLink 解析内置短链 slug。
func (m *MockShareService) ResolveLocalShortLink(ctx context.Context, slug string, baseURL string) (string, error) {
	args := m.Called(ctx, slug, baseURL)
	return args.String(0), args.Error(1)
}

// ListBlogEntries lists blog index entries.
// ListBlogEntries 列出博客索引条目。
func (m *MockShareService) ListBlogEntries(ctx context.Context, uid int64) ([]*dto.BlogEntryDTO, error) {
	args := m.Called(ctx, uid)
	if v := args.Get(0); v != nil {
		return v.([]*dto.BlogEntryDTO), args.Error(1)
	}
	return nil, args.Error(1)
}

// Ask runs the retrieval-augmented Q&A stream.
// Ask 执行检索增强问答流。
//
// NOTE: this method was missing, so the compile-time assertion below failed and the whole
// mocks package (hence internal/service tests) could not build. It was not introduced by
// the blog work — the interface gained Ask when the Q&A feature landed, and the mock was
// never updated to match.
// 注意：该方法此前缺失，导致下方的编译期断言失败、mocks 包（进而 internal/service 的测试）
// 无法编译。这不是博客改造引入的——问答功能落地时接口加上了 Ask，而 mock 没有同步。
func (m *MockShareService) Ask(ctx context.Context, opts service.AskOptions, emit func(service.AskEvent) error) error {
	args := m.Called(ctx, opts, emit)
	return args.Error(0)
}

// GetLocalGraph builds the sub-graph around one article.
// GetLocalGraph 构建单篇文章周围的子图。
func (m *MockShareService) GetLocalGraph(ctx context.Context, slug string, depth, maxNodes int) (*dto.BlogGraphDTO, error) {
	args := m.Called(ctx, slug, depth, maxNodes)
	if v := args.Get(0); v != nil {
		return v.(*dto.BlogGraphDTO), args.Error(1)
	}
	return nil, args.Error(1)
}

// RecordBlogAccess stores one already-masked blog visit.
// RecordBlogAccess 存下一次已打码的博客访问。
func (m *MockShareService) RecordBlogAccess(method, path string, status int, ms int64, ip, client string) {
	m.Called(method, path, status, ms, ip, client)
}

// GetBlogAccessLog returns a page of blog visits.
// GetBlogAccessLog 返回一页博客访问记录。
func (m *MockShareService) GetBlogAccessLog(ctx context.Context, days, limit int) (*service.BlogAccessLogPage, error) {
	args := m.Called(ctx, days, limit)
	if v := args.Get(0); v != nil {
		return v.(*service.BlogAccessLogPage), args.Error(1)
	}
	return nil, args.Error(1)
}

// GetBlogFeed returns the newest published articles with rendered bodies.
// GetBlogFeed 返回最新的已发布文章及渲染好的正文。
func (m *MockShareService) GetBlogFeed(ctx context.Context, uid int64, baseURL string, limit int) ([]*service.BlogFeedEntry, error) {
	args := m.Called(ctx, uid, baseURL, limit)
	if v := args.Get(0); v != nil {
		return v.([]*service.BlogFeedEntry), args.Error(1)
	}
	return nil, args.Error(1)
}

// GetLinkPreviews returns hover-card metadata for internal links.
// GetLinkPreviews 返回内部链接的悬停卡片元数据。
func (m *MockShareService) GetLinkPreviews(ctx context.Context, uid int64, slugs []string) (map[string]*dto.BlogPreviewDTO, error) {
	args := m.Called(ctx, uid, slugs)
	if v := args.Get(0); v != nil {
		return v.(map[string]*dto.BlogPreviewDTO), args.Error(1)
	}
	return nil, args.Error(1)
}

// GetBlogNav builds only the folder navigation tree.
// GetBlogNav 只构建目录导航树。
func (m *MockShareService) GetBlogNav(ctx context.Context, uid int64) (*dto.BlogNavDTO, error) {
	args := m.Called(ctx, uid)
	if v := args.Get(0); v != nil {
		return v.(*dto.BlogNavDTO), args.Error(1)
	}
	return nil, args.Error(1)
}

// RenderBlogBySlug renders a blog post addressed by its short link slug.
// RenderBlogBySlug 通过短链 slug 渲染博客文章。
func (m *MockShareService) RenderBlogBySlug(ctx context.Context, slug string) (*dto.BlogPostDTO, error) {
	args := m.Called(ctx, slug)
	if v := args.Get(0); v != nil {
		return v.(*dto.BlogPostDTO), args.Error(1)
	}
	return nil, args.Error(1)
}

// GetBlogReactions 返回赞/踩计数与累计访问次数。
func (m *MockShareService) GetBlogReactions() *dto.BlogReactionsDTO {
	args := m.Called()
	if v := args.Get(0); v != nil {
		return v.(*dto.BlogReactionsDTO)
	}
	return nil
}

// ReactBlogNote 对一篇笔记应用一次反应并返回其新计数。
func (m *MockShareService) ReactBlogNote(slug, kind string, delta int64) (*dto.BlogReactionTallyDTO, error) {
	args := m.Called(slug, kind, delta)
	if v := args.Get(0); v != nil {
		return v.(*dto.BlogReactionTallyDTO), args.Error(1)
	}
	return nil, args.Error(1)
}

// AddBlogVisit 累加访问计数。
func (m *MockShareService) AddBlogVisit() {
	m.Called()
}

// RenderBlogPage 渲染声明了指定页面别名的那一篇已发布笔记。
func (m *MockShareService) RenderBlogPage(ctx context.Context, uid int64, alias string) (*dto.BlogPageDTO, error) {
	args := m.Called(ctx, uid, alias)
	if v := args.Get(0); v != nil {
		return v.(*dto.BlogPageDTO), args.Error(1)
	}
	return nil, args.Error(1)
}

// GetBlogHome assembles the blog home page datasets.
// GetBlogHome 组装博客首页数据。
func (m *MockShareService) GetBlogHome(ctx context.Context, uid int64, baseURL string, publishedOnly bool, maxNodes int) (*dto.BlogHomeDTO, error) {
	args := m.Called(ctx, uid, baseURL, publishedOnly, maxNodes)
	if v := args.Get(0); v != nil {
		return v.(*dto.BlogHomeDTO), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockShareService) GetSharedNote(ctx context.Context, shareToken string, noteID int64, password string) (*dto.NoteDTO, error) {
	args := m.Called(ctx, shareToken, noteID, password)
	if v := args.Get(0); v != nil {
		return v.(*dto.NoteDTO), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockShareService) GetSharedFile(ctx context.Context, shareToken string, fileID int64, password string) ([]byte, string, int64, string, string, error) {
	args := m.Called(ctx, shareToken, fileID, password)
	var content []byte
	if v := args.Get(0); v != nil {
		content = v.([]byte)
	}
	return content, args.String(1), args.Get(2).(int64), args.String(3), args.String(4), args.Error(5)
}

func (m *MockShareService) GetSharedFileInfo(ctx context.Context, shareToken string, fileID int64, password string) (string, string, int64, string, string, error) {
	args := m.Called(ctx, shareToken, fileID, password)
	return args.String(0), args.String(1), args.Get(2).(int64), args.String(3), args.String(4), args.Error(5)
}

func (m *MockShareService) RecordView(uid int64, id int64) {
	m.Called(uid, id)
}

func (m *MockShareService) StopShare(ctx context.Context, uid int64, id int64) error {
	args := m.Called(ctx, uid, id)
	return args.Error(0)
}

func (m *MockShareService) UpdateSharePassword(ctx context.Context, uid int64, vaultName string, path string, pathHash string, password string, expireAt int64) error {
	args := m.Called(ctx, uid, vaultName, path, pathHash, password, expireAt)
	return args.Error(0)
}

func (m *MockShareService) CreateShortLink(ctx context.Context, uid int64, vaultName string, path string, pathHash string, baseURL string, longURL string, isForce bool) (string, error) {
	args := m.Called(ctx, uid, vaultName, path, pathHash, baseURL, longURL, isForce)
	return args.String(0), args.Error(1)
}

func (m *MockShareService) ListShares(ctx context.Context, uid int64, sortBy string, sortOrder string, pager *pkgapp.Pager) ([]*dto.ShareListItem, int, error) {
	args := m.Called(ctx, uid, sortBy, sortOrder, pager)
	if v := args.Get(0); v != nil {
		return v.([]*dto.ShareListItem), args.Int(1), args.Error(2)
	}
	return nil, args.Int(1), args.Error(2)
}

func (m *MockShareService) GetShareByPath(ctx context.Context, uid int64, vaultName string, pathHash string) (*domain.UserShare, error) {
	args := m.Called(ctx, uid, vaultName, pathHash)
	if v := args.Get(0); v != nil {
		return v.(*domain.UserShare), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockShareService) StopShareByPath(ctx context.Context, uid int64, vaultName string, pathHash string) error {
	args := m.Called(ctx, uid, vaultName, pathHash)
	return args.Error(0)
}

func (m *MockShareService) GetActiveNotePathsByVault(ctx context.Context, uid int64, vaultName string) ([]string, error) {
	args := m.Called(ctx, uid, vaultName)
	if v := args.Get(0); v != nil {
		return v.([]string), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockShareService) Shutdown(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}
