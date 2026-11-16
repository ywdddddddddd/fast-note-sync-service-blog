package routers

// Blog audio ("听全文") plumbing: the manifest the article page reads, the public sources
// list the local generator matches against, and the byte route that serves the MP3s.
//
// The audio itself is produced offline by tools/blog-audio and uploaded into
// storage/blog-audio. The server never synthesises speech: keeping TTS out of the request
// path means a slow or broken edge-tts run can never delay a page, and the files are
// immutable once generated.
//
// 博客「听全文」的服务端部分：文章页读取的清单、生成脚本用来匹配已分享笔记的公开来源表、
// 以及提供 MP3 字节的下载路由。
//
// 音频由本地 tools/blog-audio 离线生成后上传到 storage/blog-audio。服务端不做语音合成：
// 把 TTS 放在请求路径之外，一次缓慢或失败的 edge-tts 就不可能拖慢页面；生成后的文件也是
// 不可变的。

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/haierkeys/fast-note-sync-service/internal/app"
	"github.com/haierkeys/fast-note-sync-service/internal/service"
	"github.com/haierkeys/fast-note-sync-service/pkg/util"
)

// blogPostAudioView is what the article template needs to render the one-line entry.
// Everything else (sentence timings) is fetched by the player on first click, so an article
// that has audio does not grow by the size of its transcript.
//
// blogPostAudioView 是文章模板渲染那一行入口所需的全部数据。
// 其余内容（句子时间轴）由播放器在首次点击时按需拉取，因此有音频的文章不会因为逐句时间轴
// 而变重。
type blogPostAudioView struct {
	Src      string
	Timings  string
	TextHash string
}

type blogAudioManifestEntry struct {
	File     string `json:"file"`
	Timings  string `json:"timings,omitempty"`
	TextHash string `json:"textHash"`
	Duration int64  `json:"duration"`
	Voice    string `json:"voice"`
}

type blogAudioManifest struct {
	Version int                               `json:"version"`
	Items   map[string]blogAudioManifestEntry `json:"items"`
}

var (
	blogAudioManifestMu  sync.Mutex
	blogAudioManifestVal *blogAudioManifest
	blogAudioManifestAt  int64
)

var blogAudioFileRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// blogAudioDir lives next to the upload directory rather than inside it: uploads are sync
// payloads, while these files are generated assets with a different lifecycle.
//
// blogAudioDir 与上传目录平级而不是在其内部：uploads 是同步载荷，这些是生命周期不同的生成资源。
func blogAudioDir(appContainer *app.App) string {
	save := appContainer.Config().Storage.LocalFS.SavePath
	if save == "" {
		save = "storage/uploads"
	}
	return filepath.Join(filepath.Dir(save), "blog-audio")
}

func blogAudioManifestPath(appContainer *app.App) string {
	return filepath.Join(blogAudioDir(appContainer), "audio.json")
}

// blogAudioLookup returns the manifest entry for one slug, or nil when the article has no audio.
// The manifest is cached by mtime: the generator rewrites it on every run, and re-reading a
// small JSON file on each article render would be wasted work.
//
// blogAudioLookup 返回某个 slug 的清单条目；文章没有音频时返回 nil。
// 清单按 mtime 缓存：生成脚本每次运行都会重写它，而每次渲染文章都重读一个小 JSON 是白费工。
func blogAudioLookup(appContainer *app.App, slug string) *blogPostAudioView {
	p := blogAudioManifestPath(appContainer)
	st, err := os.Stat(p)
	if err != nil {
		return nil
	}

	blogAudioManifestMu.Lock()
	if blogAudioManifestVal != nil && blogAudioManifestAt == st.ModTime().UnixNano() {
		m := blogAudioManifestVal
		blogAudioManifestMu.Unlock()
		return blogAudioEntryView(m, slug)
	}
	blogAudioManifestMu.Unlock()

	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var m blogAudioManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}

	blogAudioManifestMu.Lock()
	blogAudioManifestVal = &m
	blogAudioManifestAt = st.ModTime().UnixNano()
	blogAudioManifestMu.Unlock()

	return blogAudioEntryView(&m, slug)
}

func blogAudioEntryView(m *blogAudioManifest, slug string) *blogPostAudioView {
	e, ok := m.Items[slug]
	if !ok || e.File == "" {
		return nil
	}
	q := ""
	if h := strings.TrimSpace(e.TextHash); h != "" {
		if len(h) > 10 {
			h = h[:10]
		}
		q = "?v=" + h
	}
	timings := e.Timings
	if timings == "" {
		timings = strings.TrimSuffix(e.File, filepath.Ext(e.File)) + ".timings.json"
	}
	return &blogPostAudioView{
		Src:      "/blog/audio/" + e.File + q,
		Timings:  "/blog/audio/" + timings + q,
		TextHash: e.TextHash,
	}
}

// registerBlogAudioRoutes wires the three audio surfaces. All of them must be registered
// before the "/blog/:slug" catch-all, for the same reason as the feed and the shelf: these
// are endpoints, not notes.
//
// registerBlogAudioRoutes 注册音频的三个入口。和其它字面端点一样，必须排在 "/blog/:slug"
// 之前：这些是接口，不是笔记。
func registerBlogAudioRoutes(r *gin.RouterGroup, appContainer *app.App, shareSvc service.ShareService) {
	// The player bundle. no-cache for the same reason as ask.js: the asset ver query is only
	// a cache-buster when the version actually changes, and a stale player is harder to debug
	// than one extra request.
	//
	// 播放器脚本。与 ask.js 相同，使用 no-cache：资源版本号只在其真的变化时才是缓存破坏器，
	// 而一个过期的播放器比多一次请求更难排查。
	r.GET("/blog/audio.js", func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.String(http.StatusOK, blogAudioScript)
	})

	r.GET("/blog/audio/:file", func(c *gin.Context) {
		cfg, ok := blogEnabled(c, appContainer)
		if !ok {
			return
		}
		name := c.Param("file")

		// The source table is public by design: it contains only the path hash (not the path)
		// and the already-public slug of notes that are currently shared. The local generator
		// needs it to know which of its `audio: true` notes are actually published.
		//
		// 来源表是刻意公开的：它只包含路径哈希（不是路径）和当前已分享笔记的公开 slug。
		// 本地生成脚本需要它来判断哪些带 `audio: true` 的笔记真的已发布。
		if name == "sources.json" {
			uid := blogUID(cfg.UID)
			entries, err := shareSvc.ListBlogEntries(c.Request.Context(), uid)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "sources unavailable"})
				return
			}
			type source struct {
				Hash string `json:"h"`
				Slug string `json:"s"`
			}
			out := make([]source, 0, len(entries))
			seen := make(map[string]struct{}, len(entries))
			for _, e := range entries {
				if !strings.HasSuffix(strings.ToLower(e.Path), ".md") {
					continue
				}
				h := util.EncodeHash32(e.Path)
				if _, dup := seen[h]; dup {
					continue
				}
				seen[h] = struct{}{}
				out = append(out, source{Hash: h, Slug: e.Slug})
			}
			c.Header("Cache-Control", "public, max-age=60")
			c.JSON(http.StatusOK, gin.H{"version": 1, "items": out})
			return
		}

		// Only flat, generated names. The regexp rejects separators and "..", so the route can
		// never be talked into reading another file through the storage directory.
		//
		// 只接受扁平的生成文件名。正则拒绝分隔符与 ".."，路由因此不可能被诱导读取存储目录之外的任何文件。
		if !blogAudioFileRe.MatchString(name) {
			c.Status(http.StatusNotFound)
			return
		}
		p := filepath.Join(blogAudioDir(appContainer), name)
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			c.Status(http.StatusNotFound)
			return
		}

		switch {
		case strings.HasSuffix(name, ".mp3"):
			// Immutable: the URL carries the first ten characters of the text hash, so a
			// regenerated article gets a new URL and never serves a stale voice.
			//
			// 不可变：URL 带有正文哈希前 10 位，重新生成的文章会得到新 URL，绝不会播到旧嗓音。
			c.Header("Content-Type", "audio/mpeg")
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasSuffix(name, ".json"):
			c.Header("Content-Type", "application/json; charset=utf-8")
			c.Header("Cache-Control", "public, max-age=60")
		default:
			c.Status(http.StatusNotFound)
			return
		}

		// ServeFile, not c.FileFromFS: the former implements Range, which every mobile audio
		// element needs to seek without downloading the whole file again.
		//
		// 用 ServeFile 而不是 c.FileFromFS：前者实现 Range，手机上的 audio 元素要靠它拖动进度
		// 而不必重新下载整个文件。
		http.ServeFile(c.Writer, c.Request, p)
	})
}
