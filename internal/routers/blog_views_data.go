package routers

import (
	"encoding/json"
	"html/template"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// Client-side view data.
//
// Two of the three home-page views are drawn in script: the cover grid because it is a layout the
// server would otherwise have to emit twice, and the wheel because it is geometry that only exists
// once the viewport size is known. Both read this one array.
//
// 客户端视图数据。
//
// 首页三种视图里有两个由脚本绘制：封面网格是因为让服务端再输出一遍同样的内容等于把页面
// 撑大一倍；同心轮是因为它的几何只在视口尺寸已知之后才存在。两者读的都是这一个数组。

// blogCardEntry is one entry as the client views need it.
//
// The keys are short on purpose. This array carries every published note — 145 today, and it
// grows — and it is serialised into the HTML of the most-visited page on the site. `"title"` to
// `"t"` is not obfuscation, it is 3 KB that buys nothing on every single visit.
//
// blogCardEntry 是客户端视图所需的单条条目。
//
// 键名刻意很短。这个数组承载每一篇已发布笔记——现在是 145 条，而且还会增长——
// 并且被序列化进全站访问量最高那个页面的 HTML 里。把 `"title"` 写成 `"t"` 不是混淆，
// 而是每次访问都白拿回 3 KB。
type blogCardEntry struct {
	Title   string            `json:"t"`
	Slug    string            `json:"s"`
	Date    string            `json:"d,omitempty"`
	Words   int               `json:"w,omitempty"`
	Vault   string            `json:"v,omitempty"`
	Summary string            `json:"x,omitempty"`
	Cover   *dto.BlogCoverDTO `json:"c,omitempty"`
}

// blogSummaryJSONChars bounds the excerpt carried into the client views.
//
// The hover bubble shows two lines; 90 characters is what the list already shows, and the full
// home-page excerpt is exactly that. Longer text would be truncated by the bubble anyway, so
// shipping it would only inflate the payload.
//
// blogSummaryJSONChars 限定进入客户端视图的摘要长度。
//
// 悬停气泡显示两行；90 字正是列表已经显示的长度，也正是首页摘要的长度。
// 更长的文本反正会被气泡截断，带上它只会让载荷变大。
const blogSummaryJSONChars = 90

// blogEntriesJSON serialises the entries for the client views.
//
// It never returns an error: a page that cannot describe its own notes is still worth serving, so
// a marshalling failure degrades to an empty array and the two script-drawn views simply have
// nothing to draw — the server-rendered list is unaffected.
//
// blogEntriesJSON 把条目序列化给客户端视图。
//
// 它从不返回错误：一个无法描述自己笔记的页面仍然值得提供，因此序列化失败时降级为空数组，
// 两个由脚本绘制的视图就是没有东西可画——服务端渲染的列表不受影响。
func blogEntriesJSON(entries []*dto.BlogEntryDTO) template.JS {
	cards := make([]blogCardEntry, 0, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		summary := e.Summary
		if r := []rune(summary); len(r) > blogSummaryJSONChars {
			summary = string(r[:blogSummaryJSONChars])
		}
		cards = append(cards, blogCardEntry{
			Title:   e.Title,
			Slug:    e.Slug,
			Date:    e.CreatedAt,
			Words:   e.Words,
			Vault:   e.Vault,
			Summary: summary,
			Cover:   e.Cover,
		})
	}

	body, err := json.Marshal(cards)
	if err != nil {
		return template.JS("[]")
	}
	return template.JS(body)
}
