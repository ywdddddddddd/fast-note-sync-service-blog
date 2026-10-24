package routers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The shelf is a folder of PDFs that visitors read page by page in the browser. The PDF bytes
// never leave the server: the browser only ever receives one rendered page at a time.
//
// That is the whole point of the design, and it is worth stating plainly because the obvious
// alternative looks easier. Shipping the PDF and rendering it with PDF.js means the file is
// delivered to every reader in full — a 20 MB book costs 20 MB of traffic the moment someone
// opens it, even if they read three pages, and the file sits in the devtools network panel for
// anyone who wants it. Rendering server-side turns that into ~120 KB per page actually looked at,
// and there is no request that ever returns the document.
//
// What this does NOT do: it cannot stop a screenshot, a screen recording, or someone saving the
// page images one by one. Nothing in a browser can. It removes the one-click download, and it
// removes the per-visit cost of the whole file.
//
// 书架是一叠放在服务器上的 PDF，访问者在浏览器里逐页阅读。PDF 字节永远不离开服务器：
// 浏览器只会一次拿到一个渲染好的页面。
//
// 这正是这套设计的全部意义，值得直说，因为显而易见的替代方案看起来更省事。把 PDF 发下去
// 用 PDF.js 渲染，意味着每个读者都完整收到那个文件——一本 20 MB 的书，有人点开一次就是
// 20 MB 流量，哪怕他只看了三页；而且那个文件就躺在 devtools 的网络面板里，谁想要都能拿。
// 服务端渲染把它变成「真正被看过的每一页约 120 KB」，并且不存在任何返回整个文档的请求。
//
// 这套做法做不到的事：拦不住截屏、录屏，也拦不住有人把页面图片一张张存下来。
// 浏览器里没有任何东西能拦住这些。它去掉的是「一键下载」，以及「每次访问都付整个文件的钱」。

const (
	shelfRoot       = "storage/blog-shelf"
	shelfManifest   = "shelf.json"
	shelfRenderDPI  = 150
	shelfJPEGOption = "quality=80,optimize=y"

	// shelfRenderConcurrency bounds how many pdftoppm processes may run at once. The machine has
	// two cores; a stray burst would otherwise queue a dozen rasterisers and take the whole
	// service down with them. Callers wait for a slot rather than being refused: a shelf page asks
	// for every cover at once, and turning three of them into broken images would be a worse
	// outcome than making the page 200 ms slower.
	//
	// shelfRenderConcurrency 限制同时运行的 pdftoppm 进程数。机器只有两核；否则一串请求
	// 会排进十几个光栅化进程，把整个服务一起拖下去。调用方是排队等一个名额，而不是被拒绝：
	// 书架页会一次性索取全部封面，把其中三张变成裂图比让页面慢 200 ms 更糟。
	shelfRenderConcurrency = 3

	// shelfRenderTimeout is generous on purpose: a dense scanned page at 150 dpi can take a couple
	// of seconds on this hardware, and a timeout that fires mid-render wastes the work already done.
	//
	// shelfRenderTimeout 刻意给得宽：一页密集的扫描件在这台机器上 150 dpi 可能要一两秒，
	// 而在渲染中途超时等于把已经做完的工作全丢掉。
	shelfRenderTimeout = 30 * time.Second

	// shelfWaitTimeout is how long a caller waits for a render slot before giving up. It has to be
	// long enough to drain a shelf page's worth of covers, and short enough that a client keeps
	// its connection.
	//
	// shelfWaitTimeout 是调用方等一个渲染名额的上限。它要长到能排完一屏封面的量，
	// 又要短到让客户端还愿意保持连接。
	shelfWaitTimeout = 25 * time.Second
)

// shelfBook is one manifest entry.
//
// Mode decides where the pages come from:
//
//	"pdf"   (default) — the PDF lives in books/, pdftoppm renders a page on demand
//	"pages"           — no PDF at all; the images live in pages/<id>/, pre-rendered elsewhere
//
// The second mode exists because a 600 dpi scanned textbook is 1–2 GB, and the server only ever
// needs 150 dpi pages of it. Ship the pages, not the book: 20x less to upload, no rasteriser, no
// 88 MB decode buffer per page. The trade is that DPI can no longer be changed server-side, and
// there is no PDF to hand out — which is the point.
//
// shelfBook 是书目里的一条。
//
// Mode 决定页从哪来：
//
//	"pdf"   （默认）—— PDF 放在 books/，pdftoppm 按需渲染某一页
//	"pages"          —— 没有 PDF；图片在 pages/<id>/ 下，预先在别处渲染好
//
// 第二种模式的存在理由：600 dpi 扫描教材有 1–2 GB，而服务器自始至终只需要它 150 dpi 的页。
// 传页、不传书：上传量少 20 倍，不需要光栅化器，也没有每页 88 MB 的解码缓冲。
// 代价是服务器侧不能再改 DPI，也没有 PDF 可以给出去——而这正是目的。
type shelfBook struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Author string   `json:"author"`
	Note   string   `json:"note"`
	File   string   `json:"file"`
	Added  string   `json:"added"`
	Tags   []string `json:"tags"`

	// Mode is "pdf" (default) or "pages". Empty means pdf.
	// Mode 是 "pdf"（默认）或 "pages"。空值即 pdf。
	Mode string `json:"mode"`
	// Cover is an optional file name under covers/ for a hand-made cover. Without it the cover
	// falls back to page one, which for a scanned textbook is usually the cover spread anyway.
	// Cover 是可选的封面文件名（在 covers/ 下）。没有它就用第一页兜底，
	// 而扫描教材的第一页通常本来就是了。
	Cover string `json:"cover"`
	// PagesHint is required in pages mode: there is no PDF to count.
	// PagesHint 在 pages 模式下必须给：没有 PDF 可数。
	PagesHint int `json:"pages"`

	// Pages is resolved lazily from the file (pdf mode) or the directory (pages mode).
	// Pages 惰性取得：pdf 模式数文件，pages 模式数目录。
	Pages int `json:"-"`
}

// shelfIsPages reports whether this book is served from pre-rendered page images.
// shelfIsPages 报告这本书是否走预渲染页图。
func shelfIsPages(b shelfBook) bool {
	return strings.EqualFold(strings.TrimSpace(b.Mode), "pages")
}

// shelfPageDir is where a pages-mode book keeps its images.
// shelfPageDir 是 pages 模式书籍存放图片的目录。
func shelfPageDir(b shelfBook) string {
	return filepath.Join(shelfRoot, "pages", b.ID)
}

// shelfPageFile returns the image path for one page of a pages-mode book, trying the extensions
// the converter can produce. It returns "" when the page is missing, which callers treat as 404.
//
// shelfPageFile 返回 pages 模式书籍某一页的图片路径，按转换器可能产出的扩展名依次尝试。
// 页缺失时返回 ""，调用方当作 404。
func shelfPageFile(b shelfBook, n int) string {
	dir := shelfPageDir(b)
	for _, ext := range []string{".avif", ".webp", ".jpg"} {
		p := filepath.Join(dir, fmt.Sprintf("%04d%s", n, ext))
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			return p
		}
	}
	return ""
}

// shelfPageCountMode counts pages in pages mode by listing the directory. In pdf mode it defers
// to pdfinfo. The manifest's pages field is only a hint used when the directory cannot be read.
//
// shelfPageCountMode 在 pages 模式下按目录数页；pdf 模式交给 pdfinfo。
// 书目里的 pages 字段只是目录读不到时的兜底。
func shelfPageCountMode(ctx context.Context, b shelfBook) int {
	if !shelfIsPages(b) {
		return shelfPageCount(ctx, b)
	}
	entries, err := os.ReadDir(shelfPageDir(b))
	if err != nil {
		return b.PagesHint
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".avif", ".webp", ".jpg":
			n++
		}
	}
	if n == 0 {
		return b.PagesHint
	}
	return n
}

type shelfManifestFile struct {
	Books []shelfBook `json:"books"`
}

// shelfIDPattern is the only thing allowed as a book id: it appears in URLs and in cache paths,
// so anything that could walk out of a directory must be rejected before it is used.
//
// shelfIDPattern 是允许作为书 id 的唯一形式：它会出现在 URL 与缓存路径里，
// 所以任何可能走出目录的字符都必须在被使用之前拒掉。
var shelfIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

var (
	shelfMu       sync.Mutex
	shelfCached   *shelfManifestFile
	shelfCacheSig string

	shelfPagesMu  sync.Mutex
	shelfPagesMap = map[string]int{}

	// The renderer is a counting semaphore rather than a worker pool: a page renders exactly once,
	// so there is nothing to queue per book, only a hard cap on how many processes may exist.
	//
	// 渲染器是一个计数信号量而不是工作池：同一页只会渲染一次，
	// 所以没有「按书排队」这回事，只需要限制同时存在的进程数。
	shelfRenderSem = make(chan struct{}, shelfRenderConcurrency)
)

// shelfBooks returns the manifest, re-reading it only when the file changed. Every blog page asks
// whether the shelf has anything in it (to decide whether the top bar shows the entry), so this
// must not stat-and-decode on every request.
//
// shelfBooks 返回书目，只在文件变化时重读。每个博客页面都要问「书架里有没有东西」
// （用于决定顶栏是否给出入口），所以这里不能每次请求都读一遍再解析。
func shelfBooks() []shelfBook {
	path := filepath.Join(shelfRoot, shelfManifest)

	st, err := os.Stat(path)
	if err != nil {
		shelfMu.Lock()
		shelfCached, shelfCacheSig = nil, ""
		shelfMu.Unlock()
		return nil
	}
	sig := strconv.FormatInt(st.ModTime().UnixNano(), 10) + ":" + strconv.FormatInt(st.Size(), 10)

	shelfMu.Lock()
	defer shelfMu.Unlock()
	if shelfCached != nil && shelfCacheSig == sig {
		return shelfCached.Books
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		shelfCached, shelfCacheSig = nil, ""
		return nil
	}
	var mf shelfManifestFile
	if err := json.Unmarshal(raw, &mf); err != nil {
		// A broken manifest is a publishing mistake, not a page fault: the shelf disappears and
		// the entry point goes with it, rather than the whole blog failing to render.
		//
		// 书目损坏是发布失误，不是页面故障：书架消失、入口跟着消失，
		// 而不是让整个博客渲染失败。
		shelfCached, shelfCacheSig = nil, ""
		return nil
	}

	kept := make([]shelfBook, 0, len(mf.Books))
	for _, b := range mf.Books {
		if !shelfIDPattern.MatchString(b.ID) {
			continue
		}
		// A pages-mode book has no PDF at all — its file field is empty by design, so the PDF
		// name check only applies to pdf mode. Getting this wrong silently drops every
		// pre-rendered book from the shelf (it did, on the first deploy).
		//
		// pages 模式的书根本没有 PDF——file 字段按设计就是空的，所以 PDF 文件名检查只适用于 pdf 模式。
		// 这里写错的后果是每一本预渲染的书都被无声地从书架上剔除（第一次部署就是这么错的）。
		if !shelfIsPages(b) && !shelfSafeFileName(b.File) {
			continue
		}
		if strings.TrimSpace(b.Title) == "" {
			b.Title = b.ID
		}
		kept = append(kept, b)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Added > kept[j].Added })

	shelfCached, shelfCacheSig = &shelfManifestFile{Books: kept}, sig
	return kept
}

// shelfSafeFileName rejects anything that could address a file outside the books folder. The
// manifest is written by the author rather than by a visitor, but a path traversal in it would
// still hand out an arbitrary file, so it is checked rather than assumed.
//
// shelfSafeFileName 拒绝任何可能指向 books 目录之外的文件名。书目由作者书写而不是访客输入，
// 但其中一处路径穿越仍然会交出任意文件，所以这里检查而不假设。
func shelfSafeFileName(name string) bool {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return false
	}
	if !strings.EqualFold(filepath.Ext(name), ".pdf") {
		return false
	}
	// ".pdf" on its own passes the extension test — filepath.Ext treats the leading dot as the
	// separator. It is harmless (still inside books/) but it is never what an author meant.
	//
	// 单独的 ".pdf" 能通过扩展名检查——filepath.Ext 把开头那个点当成了分隔符。
	// 它没有危害（仍在 books/ 内），但绝不会是作者的本意。
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return base != "" && base != "."
}

// shelfCoverNameOK validates a hand-made cover file name. Same reasoning as shelfSafeFileName:
// the name comes from a manifest the author writes, but a traversal in it would still hand out an
// arbitrary file, so it is checked rather than assumed.
//
// shelfCoverNameOK 校验手做封面的文件名。理由同 shelfSafeFileName：名字来自作者写的书目，
// 但其中一处路径穿越仍然会交出任意文件，所以检查而不假设。
func shelfCoverNameOK(name string) bool {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".avif":
		return true
	}
	return false
}

// shelfHasBooks answers the one question every blog page asks: does the top bar get a 资料 entry?
// It deliberately checks only the manifest, not each file's readability — pdfinfo per book on
// every page view would be a real cost, and a listed-but-unreadable book is a publishing mistake
// that the shelf page itself reports.
//
// shelfHasBooks 回答每个博客页面都会问的那一个问题：顶栏要不要给出「资料」入口？
// 它刻意只看书目，不看每个文件是否可读——每次打开页面都为每本书跑一次 pdfinfo 是实打实的成本，
// 而「列了但读不出来」是发布失误，由书架页自己报告。
func shelfHasBooks() bool {
	return len(shelfBooks()) > 0
}

func shelfBookByID(id string) (shelfBook, bool) {
	if !shelfIDPattern.MatchString(id) {
		return shelfBook{}, false
	}
	for _, b := range shelfBooks() {
		if b.ID == id {
			return b, true
		}
	}
	return shelfBook{}, false
}

func shelfPDFPath(b shelfBook) string {
	return filepath.Join(shelfRoot, "books", b.File)
}

// shelfPageCount reads the page count out of the PDF with pdfinfo and remembers it for the life of
// the process. A missing or unreadable file yields zero, which every caller treats as "not a book".
//
// shelfPageCount 用 pdfinfo 从 PDF 里数出页数，并在进程生命周期内记住它。
// 文件缺失或读不出来时返回 0，所有调用方都把它当作「这不是一本可读的书」。
func shelfPageCount(ctx context.Context, b shelfBook) int {
	path := shelfPDFPath(b)
	if _, err := os.Stat(path); err != nil {
		return 0
	}
	key := b.ID
	shelfPagesMu.Lock()
	if n, ok := shelfPagesMap[key]; ok {
		shelfPagesMu.Unlock()
		return n
	}
	shelfPagesMu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "pdfinfo", path).Output()
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "Pages:"); ok {
			n, _ = strconv.Atoi(strings.TrimSpace(v))
			break
		}
	}
	if n <= 0 {
		return 0
	}
	shelfPagesMu.Lock()
	shelfPagesMap[key] = n
	shelfPagesMu.Unlock()
	return n
}

// shelfCachePath is where a rendered page lives once produced. The name carries the DPI, the JPEG
// options and the source PDF's modification stamp: changing any of them produces a new cache
// population instead of silently serving images rendered from a file that no longer exists.
//
// Because the name changes whenever the book does, the response can be cached for a long time
// without a revalidation dance — a page image is immutable for a given (book, page, settings).
//
// shelfCachePath 是一页渲染完成后的落点。文件名带着 DPI、JPEG 参数，以及源 PDF 的修改时间戳：
// 改动其中任何一项都会产生一批新缓存，而不是悄悄端出按旧文件渲染的图。
//
// 因为书一变名字就变，所以响应可以缓存很久而不用来回校验——
// 对给定的（书、页、参数）来说，页图是不变的。
func shelfCachePath(b shelfBook, page int) string {
	return filepath.Join(shelfRoot, "cache", b.ID, fmt.Sprintf("p%d-%d-%s-%s.jpg",
		page, shelfRenderDPI, shelfJpegOptionTag(), shelfBookStamp(b)))
}

// shelfBookStamp is a short, file-name-safe stamp of the source PDF's modification time and size.
// A missing file yields "0", which is fine: rendering such a book fails anyway.
//
// shelfBookStamp 是源 PDF 修改时间与大小的短时间戳，可安全用于文件名。
// 文件缺失时得到 "0"，这没关系：那种书本来就渲染不出来。
func shelfBookStamp(b shelfBook) string {
	st, err := os.Stat(shelfPDFPath(b))
	if err != nil {
		return "0"
	}
	return strconv.FormatInt(st.ModTime().Unix(), 36) + strconv.FormatInt(st.Size(), 36)
}

// shelfJpegOptionTag makes the option string safe for a file name.
// shelfJpegOptionTag 把参数串变成可用于文件名的形式。
func shelfJpegOptionTag() string {
	r := strings.NewReplacer("quality=", "q", ",optimize=y", "", "=", "", ",", "-")
	return r.Replace(shelfJPEGOption)
}

func shelfCoverPath(b shelfBook) string {
	return filepath.Join(shelfRoot, "covers", b.ID+"-"+shelfJpegOptionTag()+"-"+shelfBookStamp(b)+".jpg")
}

// shelfRenderPage makes sure the JPEG for one page exists on disk and returns its path.
//
// The render is serialised per page by the file system itself: two concurrent requests for the
// same page both run pdftoppm into distinct temporary files and then race on rename, which is
// atomic, so the loser's bytes are simply discarded. Doing it with a per-page lock instead would
// have to be cleaned up and could outlive a request that was cancelled.
//
// shelfRenderPage 确保某一页的 JPEG 已在磁盘上，并返回它的路径。
//
// 同一页的并发由文件系统本身串行化：两个请求各自渲染到自己的临时文件，然后在 rename 上竞争，
// 而 rename 是原子的，输的那份字节直接丢弃。改用逐页锁的话，锁需要被清理，
// 而且可能比一个已被取消的请求活得更久。
func shelfRenderPage(ctx context.Context, b shelfBook, page int) (string, error) {
	dst := shelfCachePath(b, page)
	if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
		return dst, nil
	}

	// Wait for a slot rather than refusing outright. A grid page with twelve visible covers would
	// otherwise turn three of them into broken images; waiting turns that into a slightly slower
	// first paint, which is the better failure.
	//
	// 排队等一个名额，而不是直接拒绝。一个屏上可见十二张封面的页面，
	// 否则会有三张变成裂图；等待只是首屏慢一点，那是更好的失败方式。
	waitCtx, waitCancel := context.WithTimeout(ctx, shelfWaitTimeout)
	defer waitCancel()
	select {
	case shelfRenderSem <- struct{}{}:
		defer func() { <-shelfRenderSem }()
	case <-waitCtx.Done():
		return "", fmt.Errorf("waiting for a render slot: %w", waitCtx.Err())
	}

	// Re-check after acquiring the slot: another request may have finished this exact page while
	// we were waiting for it.
	// 拿到名额后重新检查：在等待期间，另一个请求可能刚好渲染完了同一页。
	if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
		return dst, nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}

	prefix := dst + ".tmp" + strconv.FormatInt(time.Now().UnixNano(), 36)
	cctx, cancel := context.WithTimeout(ctx, shelfRenderTimeout)
	defer cancel()

	// -singlefile keeps the output at <prefix>.jpg instead of <prefix>-<page>.jpg, so the text
	// after the last dot is always the real format.
	//
	// -singlefile 让输出落在 <prefix>.jpg 而不是 <prefix>-<page>.jpg，
	// 于是最后一个点之后的永远是真实格式。
	cmd := exec.CommandContext(cctx, "pdftoppm",
		"-jpeg",
		"-r", strconv.Itoa(shelfRenderDPI),
		"-jpegopt", shelfJPEGOption,
		"-f", strconv.Itoa(page),
		"-l", strconv.Itoa(page),
		"-singlefile",
		shelfPDFPath(b),
		prefix,
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(prefix + ".jpg")
		return "", fmt.Errorf("pdftoppm page %d: %w (%s)", page, err, strings.TrimSpace(stderr.String()))
	}
	produced := prefix + ".jpg"
	st, err := os.Stat(produced)
	if err != nil || st.Size() == 0 {
		_ = os.Remove(produced)
		return "", fmt.Errorf("pdftoppm produced no image for page %d", page)
	}
	if err := os.Rename(produced, dst); err != nil {
		_ = os.Remove(produced)
		return "", err
	}
	return dst, nil
}

// shelfRenderCover renders page one as the cover. A book without a cover is not a failure — the
// template falls back to a typographic jacket — so every error here is swallowed into "no cover".
//
// shelfRenderCover 把第一页渲染成封面。没有封面的书不是故障——模板会退回到排版书壳——
// 所以这里所有错误都收敛成「没有封面」。
func shelfRenderCover(ctx context.Context, b shelfBook) string {
	dst := shelfCoverPath(b)
	if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
		return dst
	}
	if _, err := shelfRenderPage(ctx, b, 1); err != nil {
		return ""
	}
	src := shelfCachePath(b, 1)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return ""
	}
	// Copy rather than symlink: the cache directory is disposable and may be cleared wholesale,
	// while covers are cheap to keep and must survive a cache purge.
	//
	// 用复制而不是符号链接：缓存目录是可丢弃的、可能被整体清空，
	// 而封面很便宜、并且必须能扛过一次清缓存。
	if err := copyFile(src, dst); err != nil {
		return ""
	}
	return dst
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
