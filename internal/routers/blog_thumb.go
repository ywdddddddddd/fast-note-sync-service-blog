package routers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
)

// Card covers are original files: one list page referenced a 10.96 MB PNG and a 4.69 MB JPEG, and
// on a link measured at 20–80 KB/s that is a minute of waiting for images that render at 516×168.
//
// This route makes them affordable without touching how covers are stored: it asks the existing
// share-file endpoint for the original, shrinks it once with Pillow, and serves the result from a
// disk cache keyed by file id. The second reader of the same cover downloads ~150 KB instead of
// ~11 MB.
//
// 卡片封面用的是原图：一个列表页同时引用了 10.96 MB 的 PNG 与 4.69 MB 的 JPEG，
// 而在实测 20–80 KB/s 的链路上，为几张显示尺寸只有 516×168 的图要等一分钟。
//
// 这条路由让它们变得可负担，同时不改封面的存储方式：向既有的分享文件端点要原图，
// 用 Pillow 缩一次，之后按文件 id 从磁盘缓存提供。同一张封面的第二个读者拿到的是约 150 KB，
// 而不是约 11 MB。

const (
	blogThumbWidth   = 1200
	blogThumbQuality = 82
)

// thumbOrigin is the loopback origin used to fetch the original file.
//
// It must not be derived from the request: a public Host would send the server out through the
// reverse proxy and back, doubling latency for a file it already has on disk. The port is
// overridable because the local test site listens on 9100 while production listens on 9000.
//
// thumbOrigin 是取原图时使用的回环地址。
//
// 它不能从请求推导：用一个公网 Host 会让服务器绕出反向代理再回来，为一个本地已有文件的请求
// 付两倍延迟。端口可覆盖，因为本地测试站监听 9100，而线上是 9000。
func thumbOrigin() string {
	if v := os.Getenv("FNS_BLOG_THUMB_ORIGIN"); v != "" {
		return v
	}
	return "http://127.0.0.1:9000"
}

func thumbCacheDir() string {
	return filepath.Join(os.TempDir(), "fns-blog-thumbs")
}

// registerBlogThumbRoute serves downscaled covers.
func registerBlogThumbRoute(r *gin.Engine, appContainer *app.App) {
	r.GET("/blog/thumb", func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		id := c.Query("id")
		token := c.Query("share_token")
		if id == "" || token == "" {
			c.Status(http.StatusNotFound)
			return
		}

		// The original URL is what the browser would have used; it is also the fallback whenever
		// downscaling is impossible, so the page never ends up with a hole where a cover was.
		//
		// 原图 URL 是浏览器本来会用的地址，也是缩略失败时的兜底，
		// 这样页面上永远不会出现一个本该有封面却被挖空的洞。
		original := "/api/share/file?id=" + url.QueryEscape(id) + "&share_token=" + url.QueryEscape(token)

		dir := thumbCacheDir()
		dst := filepath.Join(dir, fmt.Sprintf("%s-%d.jpg", id, blogThumbWidth))
		if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
			c.Header("Cache-Control", "public, max-age=604800")
			c.File(dst)
			return
		}

		if err := buildThumb(original, dst); err != nil {
			// Downscaling is an optimisation; failing it must not fail the image.
			// 缩略是优化项；它失败不能让图片失败。
			c.Redirect(http.StatusFound, original)
			return
		}
		c.Header("Cache-Control", "public, max-age=604800")
		c.File(dst)
	})
}

// buildThumb downloads the original and writes a downscaled JPEG to dst.
func buildThumb(original, dst string) error {
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Get(thumbOrigin() + original)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("origin status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Same volume as the cache, so the rename below is atomic.
	// 与缓存同一卷，下面的 rename 才是原子的。
	tmp, err := os.CreateTemp(filepath.Dir(dst), "src-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Pillow rather than hand-written Go scaling: the server has Pillow 13 and the standard
	// library's nearest-neighbour resize would visibly alias a photograph.
	//
	// 用 Pillow 而不是手写 Go 缩放：服务器上有 Pillow 13，
	// 而标准库的最近邻缩放会让照片出现可见锯齿。
	const script = `import sys
from PIL import Image
src, dst, w = sys.argv[1], sys.argv[2], int(sys.argv[3])
im = Image.open(src)
im = im.convert("RGB")
im.thumbnail((w, w * 10), Image.LANCZOS)
im.save(dst, "JPEG", quality=int(sys.argv[4]), optimize=True)
`
	out, err := exec.Command("python3", "-c", script, tmpPath, dst, fmt.Sprint(blogThumbWidth), fmt.Sprint(blogThumbQuality)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pillow: %v: %s", err, string(out))
	}
	if st, err := os.Stat(dst); err != nil || st.Size() == 0 {
		return fmt.Errorf("thumb not written")
	}
	return nil
}
