package model

import "github.com/haierkeys/fast-note-sync-service/pkg/timex"

const TableNameFriendApply = "friend_apply"

// FriendApply stores one application submitted from the friends page.
//
// The column widths match the validation limits in the service layer: name 60, url/avatar/feed
// 300, desc/email/message 120/120/300. They are repeated here rather than shared because a model
// generated from SQL would carry them too, and a mismatch would surface as silent truncation on
// MySQL rather than as a failed validation.
//
// RemoteIP is kept as text because it is the identity the rate limit fired on, and a visitor may
// arrive over IPv6 (up to 45 characters) or through a trusted proxy's X-Real-IP.
//
// FriendApply 存储一条从友链页提交的申请。
//
// 列宽与服务层校验上限一致：name 60、url/avatar/feed 300、desc/email/message 120/120/300。
// 这里重复一份而不是共享，因为由 SQL 生成的模型也会带着它们；不一致在 MySQL 上会表现为
// 静默截断，而不是一次失败的校验。
//
// RemoteIP 用文本保存，因为它是限流实际键的身份，而访客可能经 IPv6（最长 45 字符）
// 或可信代理的 X-Real-IP 到达。
type FriendApply struct {
	ID        int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id" form:"id"`
	UID       int64      `gorm:"column:uid;not null;index:idx_friend_apply_uid,priority:1;default:0" json:"uid" form:"uid"`
	Name      string     `gorm:"column:name;type:varchar(60);not null;default:''" json:"name" form:"name"`
	URL       string     `gorm:"column:url;type:varchar(300);not null;default:''" json:"url" form:"url"`
	Avatar    string     `gorm:"column:avatar;type:varchar(300);not null;default:''" json:"avatar" form:"avatar"`
	Desc      string     `gorm:"column:descr;type:varchar(120);not null;default:''" json:"desc" form:"desc"`
	Email     string     `gorm:"column:email;type:varchar(120);not null;default:''" json:"email" form:"email"`
	Feed      string     `gorm:"column:feed;type:varchar(300);not null;default:''" json:"feed" form:"feed"`
	Message   string     `gorm:"column:message;type:varchar(300);not null;default:''" json:"message" form:"message"`
	RemoteIP  string     `gorm:"column:remote_ip;type:varchar(64);not null;default:''" json:"remoteIp" form:"remoteIp"`
	Status    int64      `gorm:"column:status;not null;default:0" json:"status" form:"status"`
	CreatedAt timex.Time `gorm:"column:created_at;default:NULL;autoCreateTime:false" json:"createdAt" form:"createdAt"`
}

// TableName FriendApply's table name
// TableName FriendApply 的表名。
func (*FriendApply) TableName() string {
	return TableNameFriendApply
}
