package api_router

import (
	"testing"

	pkgapp "github.com/haierkeys/fast-note-sync-service/pkg/app"
)

// TestWithoutSupporterNames verifies the redaction the public /api/support endpoint performs.
//
// The endpoint is unauthenticated and its records describe third parties — patrons of the
// upstream plugin's author. The message and the amount are what those people wrote for public
// display; the name is the part that turns a thank-you page into a list of people who can be
// contacted, cross-referenced, or phished.
//
// Two properties matter and both are asserted here:
//   - the name does not survive into the returned slice;
//   - the input slice is not modified, because it is the app container's cache. Zeroing the
//     field in place would erase names for every future caller.
//
// TestWithoutSupporterNames 验证公开 /api/support 端点执行的脱敏。
//
// 该端点免认证，其记录描述的是第三方——上游插件作者的赞助者。留言与金额是那些人写给公开看的；
// 姓名才是把致谢页变成「可联系、可关联、可钓鱼的人员名单」的那一部分。
//
// 这里断言两个性质，两个都重要：
//   - 姓名不会进入返回的切片；
//   - 输入切片不被修改，因为它是 app 容器的缓存。就地清零会让将来每个调用方都失去姓名。
func TestWithoutSupporterNames(t *testing.T) {
	original := []pkgapp.SupportRecord{
		{Time: "2026/05/22 14:36:59", Item: "自定义金额", Amount: "128.00", Unit: "¥",
			Message: "感谢开发这么棒的插件", Name: "JR"},
		{Time: "2026/03/27 00:36:52", Item: "任意打赏", Amount: "128.00", Unit: "¥",
			Message: "一直在用，希望越做越好", Name: "Geeson"},
	}

	out := withoutSupporterNames(original)

	if len(out) != len(original) {
		t.Fatalf("条数应保持 %d，实际 %d", len(original), len(out))
	}
	for i, rec := range out {
		if rec.Name != "" {
			t.Fatalf("第 %d 条的姓名应被清空，实际 %q", i, rec.Name)
		}
		// Everything else is what the patron wrote for display; dropping it would break the
		// page the endpoint exists to serve.
		// 其余字段是赞助者写给公开展示的；丢掉它们会破坏这个端点存在的意义。
		if rec.Amount != original[i].Amount || rec.Message != original[i].Message ||
			rec.Time != original[i].Time || rec.Item != original[i].Item {
			t.Fatalf("第 %d 条除姓名外的字段应保持不变: %+v", i, rec)
		}
	}

	// The cache must be untouched.
	// 缓存必须未被改动。
	if original[0].Name != "JR" || original[1].Name != "Geeson" {
		t.Fatalf("输入切片不应被就地修改: %+v", original)
	}
}

// TestWithoutSupporterNamesEmpty covers the empty-input path: the endpoint must answer an empty
// list rather than a nil that a client could mistake for an error.
//
// TestWithoutSupporterNamesEmpty 覆盖空输入路径：端点应返回空列表，
// 而不是一个客户端可能误认为错误的 nil。
func TestWithoutSupporterNamesEmpty(t *testing.T) {
	out := withoutSupporterNames(nil)
	if out == nil {
		t.Fatalf("应返回空切片而非 nil")
	}
	if len(out) != 0 {
		t.Fatalf("空输入应得到空输出，实际 %d 条", len(out))
	}
}
