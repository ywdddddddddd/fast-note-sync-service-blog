package domain

import (
	"context"
	"time"
)

// FriendApplyStatusNew marks an application nobody has looked at yet.
//
// There is no update path in v1: the Bark notification is the reading interface, so a row is
// written once and never touched again. Keeping the status column anyway is what lets a future
// admin list filter without a migration over rows that have already been stored.
//
// FriendApplyStatusNew 标记一条尚未被查看的申请。
//
// v1 没有更新路径：Bark 通知就是阅读界面，因此一行写入后不再改动。
// 仍然保留 status 列，是为了将来做管理列表时不必对已经存下的行做迁移。
const FriendApplyStatusNew int64 = 0

// FriendApply is one application submitted from the friends page.
//
// It is a contact record, not published content: nothing here is rendered on any public page.
// The fields deliberately mirror the six-field copy card plus a free-text message, so the owner
// can paste an approved entry into the friends note's frontmatter without retyping anything.
//
// FriendApply 是友链页提交的一条申请。
//
// 它是联系记录而不是公开内容：这里没有任何字段会渲染到公开页面上。
// 字段刻意对应六字段复制卡，外加一段自由留言，使站主批准时可以直接把条目粘进友链笔记的
// frontmatter，不必重新录入。
type FriendApply struct {
	ID        int64
	UID       int64
	Name      string
	URL       string
	Avatar    string
	Desc      string
	Email     string
	Feed      string
	Message   string
	RemoteIP  string
	Status    int64
	CreatedAt time.Time
}

// FriendApplyRepository persists friend applications.
//
// Create is the whole interface on purpose. Reading them back would be the admin page this
// version explicitly does not build; adding a list method now would be an unused mock surface.
//
// FriendApplyRepository 持久化友链申请。
//
// 整个接口只有 Create 是有意的。把它们读回来就是本版本明确不做的管理页；
// 现在加一个列表方法，只会多出一块没人用的 mock 面。
type FriendApplyRepository interface {
	Create(ctx context.Context, apply *FriendApply) error
}
