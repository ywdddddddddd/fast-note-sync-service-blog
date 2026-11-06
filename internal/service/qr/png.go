package qr

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// PNG 把矩阵渲染成 PNG。scale 是每个模块的像素数，quiet 是四周静区宽度（单位同样是模块）。
//
// 静区不是装饰：二维码规范要求四周留 4 个模块的空白，缺了它，扫描器会把卡片上相邻的
// 深色文字当成码的一部分。这里的默认调用方传 4。
func PNG(m *Matrix, scale, quiet int, dark, light color.Color) ([]byte, error) {
	if scale < 1 {
		scale = 1
	}
	if quiet < 0 {
		quiet = 0
	}
	dim := (m.Size + quiet*2) * scale
	img := image.NewRGBA(image.Rect(0, 0, dim, dim))
	for y := 0; y < dim; y++ {
		for x := 0; x < dim; x++ {
			img.Set(x, y, light)
		}
	}
	for my := 0; my < m.Size; my++ {
		for mx := 0; mx < m.Size; mx++ {
			if !m.Mod[my][mx] {
				continue
			}
			x0 := (mx + quiet) * scale
			y0 := (my + quiet) * scale
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set(x0+dx, y0+dy, dark)
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
