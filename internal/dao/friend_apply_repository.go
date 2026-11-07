package dao

import (
	"context"
	"time"

	"github.com/haierkeys/fast-note-sync-service/internal/domain"
	"github.com/haierkeys/fast-note-sync-service/internal/model"
	"github.com/haierkeys/fast-note-sync-service/pkg/timex"
	"gorm.io/gorm"
)

// friendApplyRepository is the persistence side of the friends-page application form.
//
// It follows the hand-written repository pattern used by OIDC identities rather than the
// generated one used by shares: this table has a single write and no query surface, so there is
// nothing for gorm.io/gen to buy, and a generated package would be a second place to regenerate
// on every schema tweak.
//
// friendApplyRepository 是友链页申请表单的持久化侧。
//
// 它沿用 OIDC 身份那种手写仓储，而不是分享用的生成式仓储：这张表只有一次写入、没有查询面，
// gorm.io/gen 买不到任何东西，而生成包会让每次改模式都多一处要重新生成的地方。
type friendApplyRepository struct {
	dao *Dao
}

// NewFriendApplyRepository returns a repository bound to the main database.
// NewFriendApplyRepository 返回绑定主库的仓储。
func NewFriendApplyRepository(dao *Dao) domain.FriendApplyRepository {
	return &friendApplyRepository{dao: dao}
}

func init() {
	RegisterModel(ModelConfig{
		Name:     "FriendApply",
		IsMainDB: true,
	})
}

// db returns the main connection, ensuring the table exists on first use.
//
// The onceKey carries both the repository name and the table name for the same reason the OIDC
// repository does: a future shard/prefix variant would then get its own migration exactly once,
// instead of sharing a key that only names the repository.
//
// db 返回主库连接，并在首次使用时确保表已建好。
//
// onceKey 同时带上仓储名与表名，理由与 OIDC 仓储相同：将来若出现分片或前缀变体，
// 它会各自恰好迁移一次，而不是共享一个只写了仓储名的键。
func (r *friendApplyRepository) db() *gorm.DB {
	db := r.dao.ResolveDB()
	r.dao.QueryWithOnceInit(func(g *gorm.DB) {
		model.AutoMigrate(g, "FriendApply")
	}, "friend_apply#friend_apply")
	return db
}

// Create inserts one application.
//
// CreatedAt is set here rather than by GORM because the model follows the project's timex
// convention (autoCreateTime:false), which exists so timestamps come from one clock in one
// timezone across every table.
//
// Create 插入一条申请。
//
// CreatedAt 在这里赋值而不是交给 GORM，因为模型遵循项目的 timex 约定
// （autoCreateTime:false）——那套约定是为了让所有表的时间戳都来自同一个时钟、同一个时区。
func (r *friendApplyRepository) Create(ctx context.Context, apply *domain.FriendApply) error {
	m := &model.FriendApply{
		UID:       apply.UID,
		Name:      apply.Name,
		URL:       apply.URL,
		Avatar:    apply.Avatar,
		Desc:      apply.Desc,
		Email:     apply.Email,
		Feed:      apply.Feed,
		Message:   apply.Message,
		RemoteIP:  apply.RemoteIP,
		Status:    apply.Status,
		CreatedAt: timex.Now(),
	}
	if err := r.db().WithContext(ctx).Create(m).Error; err != nil {
		return err
	}
	apply.ID = m.ID
	apply.CreatedAt = time.Time(m.CreatedAt)
	return nil
}

var _ domain.FriendApplyRepository = (*friendApplyRepository)(nil)
