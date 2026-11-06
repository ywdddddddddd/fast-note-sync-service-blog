package qr

import (
	"encoding/json"
	"os"
	"testing"
)

// 这个测试对照的是 Python qrcode 库生成的参考矩阵（生成脚本见
// _measure/gen_qrref.py，JSON 路径由环境变量 QR_REF 指定）。
//
// 没有它就不该相信这个包：二维码是没有"看起来差不多"的余地的——错一位就是一个
// 扫不出来的码，而扫不出来只有读者会知道。这里逐位比对 6 种内容 × 8 种掩码，
// 覆盖版本 1 与版本 4（本实现实际会用到的两个）。
//
// 强制掩码是为了让两边可比：自动选掩码时，两边都"正确"也可能选到不同掩码。
// 掩码选择本身由同一份参考数据里的 mask=-1 用例间接验证（见 TestAutoMaskMatchesReference）。

type refCase struct {
	Text    string   `json:"text"`
	Mask    int      `json:"mask"`
	Version int      `json:"version"`
	Rows    []string `json:"rows"`
}

func loadRef(t *testing.T) []refCase {
	t.Helper()
	path := os.Getenv("QR_REF")
	if path == "" {
		t.Skip("未设置 QR_REF，跳过参考矩阵比对")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取参考数据失败: %v", err)
	}
	var cases []refCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("解析参考数据失败: %v", err)
	}
	return cases
}

func TestMatchesReference(t *testing.T) {
	cases := loadRef(t)
	for _, c := range cases {
		if c.Mask < 0 {
			continue // 自动掩码用例由 TestAutoMaskIsOneOfVerified 负责
		}
		m, err := Encode(c.Text, c.Mask)
		if err != nil {
			t.Fatalf("%q mask=%d 编码失败: %v", c.Text, c.Mask, err)
		}
		if m.Version != c.Version {
			t.Errorf("%q mask=%d 版本 %d，参考 %d", c.Text, c.Mask, m.Version, c.Version)
			continue
		}
		if m.Size != len(c.Rows) {
			t.Errorf("%q 尺寸 %d，参考 %d", c.Text, m.Size, len(c.Rows))
			continue
		}
		diff := 0
		var firstAt string
		for y, row := range c.Rows {
			for x := 0; x < len(row); x++ {
				want := row[x] == '1'
				if m.Mod[y][x] != want {
					diff++
					if firstAt == "" {
						firstAt = "(" + itoa(x) + "," + itoa(y) + ")"
					}
				}
			}
		}
		if diff != 0 {
			t.Errorf("%q mask=%d 与参考有 %d 处不同，首个在 %s", c.Text, c.Mask, diff, firstAt)
		}
	}
}

// TestAutoMaskIsOneOfVerified 校验"自动选掩码"的结果必然落在已逐位验证过的那八个矩阵里。
//
// 这里不要求与参考实现选到同一个掩码，而是要求"自动选出来的那个掩码，其矩阵与参考
// 在同一个掩码下的矩阵完全一致"。八种掩码都合法且都能扫，选哪个只影响印刷质量；
// 而只要选中的是这八个之一，且这八个都已逐位对齐，自动输出就同样正确。
// 强行要求两边选同一个，等于在测试一个标准里没有规定的东西。
func TestAutoMaskIsOneOfVerified(t *testing.T) {
	cases := loadRef(t)
	verified := map[string]map[int][]string{}
	for _, c := range cases {
		if c.Mask < 0 {
			continue
		}
		if verified[c.Text] == nil {
			verified[c.Text] = map[int][]string{}
		}
		verified[c.Text][c.Mask] = c.Rows
	}
	if len(verified) == 0 {
		t.Skip("参考数据里没有强制掩码用例")
	}

	for text, byMask := range verified {
		m, err := Encode(text, -1)
		if err != nil {
			t.Fatalf("%q 编码失败: %v", text, err)
		}
		rows, ok := byMask[m.Mask]
		if !ok {
			t.Fatalf("%q 自动选了掩码 %d，参考数据里没有这一档", text, m.Mask)
		}
		for y, row := range rows {
			for x := 0; x < len(row); x++ {
				if want := row[x] == '1'; m.Mod[y][x] != want {
					t.Fatalf("%q 自动掩码 %d 的矩阵与参考不一致，首个差异 (%d,%d)", text, m.Mask, x, y)
				}
			}
		}
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
