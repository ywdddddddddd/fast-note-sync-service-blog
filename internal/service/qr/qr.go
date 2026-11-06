// Package qr 是一个最小的二维码编码器：字节模式、纠错等级 M、版本 1–10。
//
// 为什么自己写而不是引依赖：这台机器连不上 proxy.golang.org（公司网络里被挡），
// 拉不到 skip2/go-qrcode 之类；而这个功能又必须离线可用——分享卡上的二维码一旦
// 依赖外部服务，断网就是一张废图，而且每篇文章地址都会被发给第三方。
//
// 只做到版本 10 是因为够用且可控：版本 10 在 M 等级下能装 216 字节，而本站的文章
// 地址最长不到 60 字节。表越长越容易抄错，而抄错一位就是一个扫不出来的码。
//
// 实现按 ISO/IEC 18004 的顺序：编码 → 分块 → Reed-Solomon → 交织 → 铺矩阵 → 选掩码。
package qr

import (
	"errors"
	"fmt"
	"math"
)

// Level 是纠错等级。目前只实现 M——分享卡是屏幕/打印两用，M 的容错率（约 15%）
// 在不把码撑大的前提下足够。
type Level int

const (
	LevelM Level = iota
)

// blockSpec 描述某个版本在 M 等级下的分块结构。
//
// g1N/g1D：第一组块数与每块数据码字数；g2N/g2D：第二组（每块比 g1 多一个码字）。
type blockSpec struct {
	total int // 该版本总码字数
	ec    int // 每块的纠错码字数
	g1N   int
	g1D   int
	g2N   int
	g2D   int
}

// specM 是版本 1–10 在纠错等级 M 下的参数表。
var specM = []blockSpec{
	{}, // 版本 0 占位
	{26, 10, 1, 16, 0, 0},
	{44, 16, 1, 28, 0, 0},
	{70, 26, 1, 44, 0, 0},
	{100, 18, 2, 32, 0, 0},
	{134, 24, 2, 43, 0, 0},
	{172, 16, 4, 27, 0, 0},
	{196, 18, 4, 31, 0, 0},
	{242, 22, 2, 38, 2, 39},
	{292, 22, 3, 36, 2, 37},
	{346, 26, 4, 43, 1, 44},
}

// alignPos 是各版本校正图形中心坐标。
var alignPos = [][]int{
	{},        // 1
	{6, 18},   // 2
	{6, 22},   // 3
	{6, 26},   // 4
	{6, 30},   // 5
	{6, 34},   // 6
	{6, 22, 38}, // 7
	{6, 24, 42}, // 8
	{6, 26, 46}, // 9
	{6, 28, 50}, // 10
}

// Matrix 是一个已生成的二维码：Size×Size 的模块，Mod[y][x] 为真表示深色。
type Matrix struct {
	Version int
	Size    int
	Mask    int
	Mod     [][]bool
}

// ErrTooLong 表示内容超出本实现支持的最大容量。
var ErrTooLong = errors.New("qr: content too long")

// Encode 把 text 编成二维码。mask 传 -1 表示按标准罚分自动选择，传 0–7 则强制使用该掩码。
// 强制掩码只用于测试：对照参考实现逐位比对时必须固定同一个掩码，否则两边都"正确"
// 却选到不同掩码，矩阵自然不同。
func Encode(text string, mask int) (*Matrix, error) {
	data := []byte(text)
	ver, err := pickVersion(len(data))
	if err != nil {
		return nil, err
	}
	codewords, err := buildCodewords(data, ver)
	if err != nil {
		return nil, err
	}
	if mask < 0 {
		best, bestScore := -1, 0
		for m := 0; m < 8; m++ {
			mx := buildMatrix(codewords, ver, m)
			if s := penalty(mx); best < 0 || s < bestScore {
				best, bestScore = m, s
			}
		}
		mask = best
	}
	return buildMatrix(codewords, ver, mask), nil
}

// pickVersion 选出版本：数据位 = 4 位模式 + 计数字段 + 内容 + 终止符，按码字向上取整。
func pickVersion(n int) (int, error) {
	for v := 1; v <= 10; v++ {
		ccBits := 8
		if v >= 10 {
			ccBits = 16
		}
		needBits := 4 + ccBits + n*8
		capacity := dataCodewords(specM[v]) * 8
		if needBits <= capacity {
			return v, nil
		}
	}
	return 0, ErrTooLong
}

func dataCodewords(s blockSpec) int { return s.g1N*s.g1D + s.g2N*s.g2D }

// buildCodewords 依次完成 编码 → 分块 → 纠错 → 交织。
func buildCodewords(data []byte, ver int) ([]byte, error) {
	s := specM[ver]
	ccBits := 8
	if ver >= 10 {
		ccBits = 16
	}

	bits := make([]bool, 0, dataCodewords(s)*8)
	putBits := func(val, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, (val>>uint(i))&1 == 1)
		}
	}
	putBits(0b0100, 4)   // 字节模式
	putBits(len(data), ccBits)
	for _, b := range data {
		putBits(int(b), 8)
	}

	// 终止符最多 4 位，然后补到字节边界
	totalBits := dataCodewords(s) * 8
	if len(bits)+4 <= totalBits {
		bits = append(bits, make([]bool, 4)...)
	} else {
		bits = append(bits, make([]bool, totalBits-len(bits))...)
	}
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}

	cw := make([]byte, len(bits)/8)
	for i := range cw {
		var v byte
		for j := 0; j < 8; j++ {
			if bits[i*8+j] {
				v |= 1 << uint(7-j)
			}
		}
		cw[i] = v
	}
	// 交替填充 0xEC / 0x11，直到数据码字装满
	for pad := 0; len(cw) < totalBits/8; pad++ {
		if pad%2 == 0 {
			cw = append(cw, 0xEC)
		} else {
			cw = append(cw, 0x11)
		}
	}

	// 分块并逐块算纠错码
	blocks := make([][]byte, 0, s.g1N+s.g2N)
	ecBlocks := make([][]byte, 0, s.g1N+s.g2N)
	pos := 0
	for i := 0; i < s.g1N; i++ {
		blk := cw[pos : pos+s.g1D]
		pos += s.g1D
		blocks = append(blocks, blk)
		ecBlocks = append(ecBlocks, rsEncode(blk, s.ec))
	}
	for i := 0; i < s.g2N; i++ {
		blk := cw[pos : pos+s.g2D]
		pos += s.g2D
		blocks = append(blocks, blk)
		ecBlocks = append(ecBlocks, rsEncode(blk, s.ec))
	}

	// 交织：先按列取完数据码字，再按列取完纠错码字
	out := make([]byte, 0, s.total)
	maxD := s.g1D
	if s.g2N > 0 && s.g2D > maxD {
		maxD = s.g2D
	}
	for i := 0; i < maxD; i++ {
		for _, blk := range blocks {
			if i < len(blk) {
				out = append(out, blk[i])
			}
		}
	}
	for i := 0; i < s.ec; i++ {
		for _, blk := range ecBlocks {
			out = append(out, blk[i])
		}
	}
	if len(out) != s.total {
		return nil, fmt.Errorf("qr: 交织后码字数 %d，应为 %d", len(out), s.total)
	}
	return out, nil
}

// gfExp/gfLog 是 GF(256) 的反对数/对数表，本原多项式 0x11D。
var gfExp [512]byte
var gfLog [256]byte

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[byte(x)] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

// rsEncode 用生成多项式做除法，返回 ecCount 个纠错码字。
func rsEncode(data []byte, ecCount int) []byte {
	gen := rsGenerator(ecCount)
	res := make([]byte, ecCount)
	for _, b := range data {
		factor := b ^ res[0]
		copy(res, res[1:])
		res[ecCount-1] = 0
		if factor != 0 {
			for i := 0; i < ecCount; i++ {
				res[i] ^= gfMul(gen[i], factor)
			}
		}
	}
	return res
}

// rsGenerator 生成 (x-α^0)(x-α^1)…(x-α^(n-1)) 的系数，最高次项省略。
func rsGenerator(n int) []byte {
	g := []byte{1}
	for i := 0; i < n; i++ {
		next := make([]byte, len(g)+1)
		for j, c := range g {
			next[j] ^= c
			next[j+1] ^= gfMul(c, gfExp[i])
		}
		g = next
	}
	return g[1:]
}

// buildMatrix 铺函数图形与数据，并施加掩码。
func buildMatrix(codewords []byte, ver, mask int) *Matrix {
	size := ver*4 + 17
	mod := make([][]bool, size)
	funcMod := make([][]bool, size) // 标记函数图形，数据不可覆盖
	for i := range mod {
		mod[i] = make([]bool, size)
		funcMod[i] = make([]bool, size)
	}
	set := func(x, y int, dark bool) {
		if x < 0 || y < 0 || x >= size || y >= size {
			return
		}
		mod[y][x] = dark
		funcMod[y][x] = true
	}

	// 定位图形 + 分隔带
	for _, p := range [][2]int{{0, 0}, {size - 7, 0}, {0, size - 7}} {
		for dy := -1; dy <= 7; dy++ {
			for dx := -1; dx <= 7; dx++ {
				x, y := p[0]+dx, p[1]+dy
				if x < 0 || y < 0 || x >= size || y >= size {
					continue
				}
				inner := dx >= 0 && dx <= 6 && dy >= 0 && dy <= 6
				dark := false
				if inner {
					edge := dx == 0 || dx == 6 || dy == 0 || dy == 6
					core := dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4
					dark = edge || core
				}
				set(x, y, dark)
			}
		}
	}
	// 定时图形
	for i := 8; i < size-8; i++ {
		set(i, 6, i%2 == 0)
		set(6, i, i%2 == 0)
	}
	// 校正图形
	if ver >= 2 {
		pos := alignPos[ver-1]
		for _, cy := range pos {
			for _, cx := range pos {
				if (cx == 6 && cy == 6) || (cx == 6 && cy == size-7) || (cx == size-7 && cy == 6) {
					continue
				}
				for dy := -2; dy <= 2; dy++ {
					for dx := -2; dx <= 2; dx++ {
						d := max(abs(dx), abs(dy))
						set(cx+dx, cy+dy, d != 1)
					}
				}
			}
		}
	}
	// 固定深色模块
	set(8, size-8, true)

	// 格式信息（先占位，掩码确定后写入真实值；这里先写以标记为函数区）
	drawFormat(set, size, 0)
	// 版本信息（版本 ≥7）
	if ver >= 7 {
		drawVersion(set, size, ver)
	}

	// 数据填充：从右下角起两列一组，蛇形向上
	i := 0
	bitAt := func(idx int) bool {
		if idx/8 >= len(codewords) {
			return false
		}
		return codewords[idx/8]>>uint(7-idx%8)&1 == 1
	}
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5 // 跳过定时列
		}
		for vert := 0; vert < size; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				upward := ((right+1)/2)%2 == 0
				y := vert
				if upward {
					y = size - 1 - vert
				}
				if funcMod[y][x] {
					continue
				}
				mod[y][x] = bitAt(i)
				i++
			}
		}
	}

	// 施加掩码
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if funcMod[y][x] {
				continue
			}
			if maskAt(mask, x, y) {
				mod[y][x] = !mod[y][x]
			}
		}
	}
	drawFormat(set, size, mask)

	return &Matrix{Version: ver, Size: size, Mask: mask, Mod: mod}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maskAt(m, x, y int) bool {
	switch m {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (y/2+x/3)%2 == 0
	case 5:
		return (x*y)%2+(x*y)%3 == 0
	case 6:
		return ((x*y)%2+(x*y)%3)%2 == 0
	case 7:
		return ((x+y)%2+(x*y)%3)%2 == 0
	}
	return false
}

// drawFormat 写入 15 位格式信息：2 位纠错等级 + 3 位掩码，BCH(15,5) 后异或 0x5412。
func drawFormat(set func(x, y int, dark bool), size, mask int) {
	data := 0 << 3 // 等级 M = 00
	data |= mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412

	bit := func(i int) bool { return (bits>>uint(i))&1 == 1 }
	// 左上角
	for i := 0; i <= 5; i++ {
		set(8, i, bit(i))
	}
	set(8, 7, bit(6))
	set(8, 8, bit(7))
	set(7, 8, bit(8))
	for i := 9; i <= 14; i++ {
		set(14-i, 8, bit(i))
	}
	// 右上 / 左下
	for i := 0; i <= 7; i++ {
		set(size-1-i, 8, bit(i))
	}
	for i := 8; i <= 14; i++ {
		set(8, size-15+i, bit(i))
	}
}

// drawVersion 写入 18 位版本信息，仅版本 ≥7 需要。
func drawVersion(set func(x, y int, dark bool), size, ver int) {
	rem := ver
	for i := 0; i < 12; i++ {
		rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
	}
	bits := ver<<12 | rem
	for i := 0; i < 18; i++ {
		bit := (bits>>uint(i))&1 == 1
		x := size - 11 + i%3
		y := i / 3
		set(x, y, bit)
		set(y, x, bit)
	}
}

// penalty 实现标准里的四条罚分规则，用于自动选掩码。
func penalty(m *Matrix) int {
	size := m.Size
	score := 0

	// 规则 1：同色连续 ≥5
	runScore := func(get func(i, j int) bool) int {
		s := 0
		for i := 0; i < size; i++ {
			run := 1
			for j := 1; j < size; j++ {
				if get(i, j) == get(i, j-1) {
					run++
				} else {
					if run >= 5 {
						s += 3 + (run - 5)
					}
					run = 1
				}
			}
			if run >= 5 {
				s += 3 + (run - 5)
			}
		}
		return s
	}
	score += runScore(func(i, j int) bool { return m.Mod[i][j] })
	score += runScore(func(i, j int) bool { return m.Mod[j][i] })

	// 规则 2：2×2 同色块
	for y := 0; y < size-1; y++ {
		for x := 0; x < size-1; x++ {
			v := m.Mod[y][x]
			if v == m.Mod[y][x+1] && v == m.Mod[y+1][x] && v == m.Mod[y+1][x+1] {
				score += 3
			}
		}
	}

	// 规则 3：形如 10111010000 / 00001011101 的图案（行、列各扫一遍）
	pat1 := []bool{true, false, true, true, true, false, true, false, false, false, false}
	pat2 := []bool{false, false, false, false, true, false, true, true, true, false, true}
	scanLine := func(at func(i int) bool) int {
		s := 0
		for start := 0; start+11 <= size; start++ {
			ok1, ok2 := true, true
			for t := 0; t < 11; t++ {
				v := at(start + t)
				if v != pat1[t] {
					ok1 = false
				}
				if v != pat2[t] {
					ok2 = false
				}
			}
			if ok1 {
				s += 40
			}
			if ok2 {
				s += 40
			}
		}
		return s
	}
	for y := 0; y < size; y++ {
		row := y
		score += scanLine(func(i int) bool { return m.Mod[row][i] })
	}
	for x := 0; x < size; x++ {
		col := x
		score += scanLine(func(i int) bool { return m.Mod[i][col] })
	}

	// 规则 4：深色比例偏离 50%。
	// 用浮点而不是整数百分比：先整数除会截断，比例落在 49.x% 时被当成 49，
	// 罚分就少一档；几个掩码分差很小的时候，这一档足以让它选错。
	dark := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if m.Mod[y][x] {
				dark++
			}
		}
	}
	ratio := float64(dark) / float64(size*size) * 100
	dev := int(math.Abs(ratio-50) / 5)
	score += dev * 10

	return score
}

