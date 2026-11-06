package qr

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteSamplePNG 把两张样张用的二维码写到 QR_OUT 目录，供设计稿直接引用。
// 它不做断言——真正的断言在 TestMatchesReference 里；这里只是把已验证的矩阵落成文件。
func TestWriteSamplePNG(t *testing.T) {
	dir := os.Getenv("QR_OUT")
	if dir == "" {
		t.Skip("未设置 QR_OUT")
	}
	const url = "https://yaowendao.asia/blog/1.192-2dee79e43b"
	m, err := Encode(url, -1)
	if err != nil {
		t.Fatal(err)
	}
	light, err := PNG(m, 10, 4, color.RGBA{52, 50, 45, 255}, color.RGBA{255, 255, 255, 0})
	if err != nil {
		t.Fatal(err)
	}
	dark, err := PNG(m, 10, 4, color.RGBA{237, 237, 237, 255}, color.RGBA{28, 28, 26, 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qr_light.png"), light, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qr_dark.png"), dark, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("version=%d size=%d mask=%d", m.Version, m.Size, m.Mask)
}
