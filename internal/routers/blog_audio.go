package routers

// Blog audio ("听全文") plumbing: the manifest the article page reads, the public sources list
// the local bulk generator matches against, the on-demand prepare/status endpoints, and the
// byte route that serves the MP3s.
//
// The article page always renders the entry. If audio.json has no entry yet, the player offers
// to generate it after showing an estimate; the TTS sidecar does the work and the manifest is
// updated, so the next visitor (and the current one) can play immediately.
//
// 博客「听全文」的服务端部分：文章页读取的清单、本地批量生成器用来匹配的公开来源表、按需
// prepare/status 端点，以及提供 MP3 字节的下载路由。
//
// 文章页始终渲染入口。audio.json 还没有条目时，播放器先显示预计时间，确认后由 TTS sidecar 生成
// 并更新清单，因此下一位访客（以及当前访客）随后都能直接播放。

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
	Slug     string
	Src      string
	Timings  string
	TextHash string
	Ready    bool
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

type blogAudioStatusResponse struct {
	Status      string `json:"status"`
	EstimateSec int64  `json:"estimateSec,omitempty"`
	Error       string `json:"error,omitempty"`
	Src         string `json:"src,omitempty"`
	Timings     string `json:"timings,omitempty"`
	TextHash    string `json:"textHash,omitempty"`
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

// blogAudioManifestCached returns the manifest, reloading it when the file's mtime changes.
// The generator rewrites it on every run, and re-reading a small JSON file on each article
// render would be wasted work.
//
// blogAudioManifestCached 返回清单，并在文件 mtime 变化时重载。生成器每次运行都会重写它，
// 而每次渲染文章都重读一个小 JSON 是白费工。
func blogAudioManifestCached(appContainer *app.App) *blogAudioManifest {
	p := blogAudioManifestPath(appContainer)
	st, err := os.Stat(p)
	if err != nil {
		return nil
	}

	blogAudioManifestMu.Lock()
	if blogAudioManifestVal != nil && blogAudioManifestAt == st.ModTime().UnixNano() {
		m := blogAudioManifestVal
		blogAudioManifestMu.Unlock()
		return m
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
	return &m
}

// blogAudioViewForArticle always returns a view so the entry exists on every article. Ready is
// false when audio.json has no usable entry yet; the player then asks the server to generate.
//
// blogAudioViewForArticle 总是返回一个视图，使每篇文章都有入口。audio.json 还没有可用条目时
// Ready 为 false，播放器随后请求服务端生成。
func blogAudioViewForArticle(appContainer *app.App, slug string) *blogPostAudioView {
	v := &blogPostAudioView{
		Slug:    slug,
		Src:     "/blog/audio/" + slug + ".mp3",
		Timings: "/blog/audio/" + slug + ".timings.json",
	}
	m := blogAudioManifestCached(appContainer)
	if m == nil {
		return v
	}
	e, ok := m.Items[slug]
	if !ok || e.File == "" {
		return v
	}
	q := ""
	if h := strings.TrimSpace(e.TextHash); h != "" {
		q = "?v=" + blogAudioHashPrefix(h)
	}
	timings := e.Timings
	if timings == "" {
		timings = strings.TrimSuffix(e.File, filepath.Ext(e.File)) + ".timings.json"
	}
	v.Ready = true
	v.Src = "/blog/audio/" + e.File + q
	v.Timings = "/blog/audio/" + timings + q
	v.TextHash = e.TextHash
	return v
}

// blogAudioReady returns a ready response only when the manifest entry exists and its MP3 is
// actually on disk. A manifest row whose file was deleted must not claim readiness.
//
// blogAudioReady 只在清单条目存在且 MP3 真在磁盘上时返回 ready。文件被删掉的清单行不能声称就绪。
func blogAudioReady(appContainer *app.App, slug string) *blogAudioStatusResponse {
	m := blogAudioManifestCached(appContainer)
	if m == nil {
		return nil
	}
	e, ok := m.Items[slug]
	if !ok || e.File == "" {
		return nil
	}
	st, err := os.Stat(filepath.Join(blogAudioDir(appContainer), e.File))
	if err != nil || st.IsDir() {
		return nil
	}
	timings := e.Timings
	if timings == "" {
		timings = strings.TrimSuffix(e.File, filepath.Ext(e.File)) + ".timings.json"
	}
	q := ""
	if h := strings.TrimSpace(e.TextHash); h != "" {
		q = "?v=" + blogAudioHashPrefix(h)
	}
	return &blogAudioStatusResponse{
		Status:   blogAudioReadyStatus,
		Src:      "/blog/audio/" + e.File + q,
		Timings:  "/blog/audio/" + timings + q,
		TextHash: e.TextHash,
	}
}

// registerBlogAudioRoutes wires the audio surfaces. All of them must be registered before the
// "/blog/:slug" catch-all, for the same reason as the feed and the shelf: these are endpoints,
// not notes.
//
// registerBlogAudioRoutes 注册音频入口。和其它字面端点一样，必须排在 "/blog/:slug" 之前：
// 这些是接口，不是笔记。
func registerBlogAudioRoutes(r *gin.RouterGroup, appContainer *app.App, shareSvc service.ShareService) {
	gen := newBlogAudioGen(appContainer)

	// The player bundle. no-cache for the same reason as ask.js: the asset ver query is only
	// a cache-buster when the version actually changes, and a stale player is harder to debug
	// than one extra request.
	//
	// 播放器脚本。与 ask.js 相同，使用 no-cache。
	r.GET("/blog/audio.js", func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.String(http.StatusOK, blogAudioScript)
	})

	// First click on an article without audio asks the server to build it. The handler renders
	// the note once to know the current text hash, so a stale manifest entry is detected and a
	// changed article is regenerated instead of playing the previous voice.
	//
	// 第一次点击没有音频的文章时，请求服务端生成。处理器渲染一次笔记以取得当前正文哈希，
	// 因此过期的清单条目会被发现，改过的文章会重新生成而不是播放旧嗓音。
	r.POST("/blog/audio/prepare", func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		var req struct {
			Slug string `json:"slug"`
		}
		// The body is one short slug; anything larger is not a client this endpoint serves.
		// 请求体只有一个短 slug；比这更大的都不属于这个端点要服务的客户端。
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		slug := strings.TrimSpace(req.Slug)
		if !blogAudioSlugRe.MatchString(slug) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slug"})
			return
		}
		post, err := shareSvc.RenderBlogBySlug(c.Request.Context(), slug)
		if err != nil {
			// Never distinguish "never existed" from "unpublished" for a public caller.
			// 对公开调用者绝不区分「从未存在」与「已取消发布」。
			c.JSON(http.StatusNotFound, gin.H{"error": "not available"})
			return
		}
		body := sanitizeImages(externalizeLinks(post.HTML))
		_, body = extractTOC(body)
		fullText, sentences := blogAudioExtract(body)
		if strings.TrimSpace(fullText) == "" {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "no speakable text"})
			return
		}
		textHash := blogAudioTextHash(fullText)

		// A job already in flight wins over the manifest. During a regeneration the old row is
		// still on disk; reporting it as ready is exactly what made the player accept the
		// previous, shorter audio and skip the job it had just requested.
		//
		// 正在进行的任务优先于清单。重生成期间旧条目仍在磁盘上；把它报告为 ready 正是播放器
		// 接受上一版更短音频、把它刚请求的任务晾在一边的原因。
		if st := gen.status(slug); st != nil {
			switch st.Status {
			case blogAudioQueued, blogAudioGenerating:
				c.JSON(http.StatusOK, blogAudioStatusResponse{
					Status:      st.Status,
					EstimateSec: st.Estimate,
				})
				return
			}
		}

		if ready := blogAudioReady(appContainer, slug); ready != nil && ready.TextHash == textHash {
			c.JSON(http.StatusOK, ready)
			return
		}

		estimate := blogAudioEstimate(fullText)
		job := &blogAudioJob{
			Slug:      slug,
			TextHash:  textHash,
			Text:      fullText,
			Sentences: sentences,
			Voice:     blogAudioDefaultVoice,
			Estimate:  estimate,
		}
		status, est, ok := gen.enqueue(job)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "生成队列已满，请稍后再试"})
			return
		}
		if status == blogAudioReadyStatus {
			// enqueue only returns ready after checking the manifest and the file. Return the
			// real URLs instead of a bare status, which the client cannot play.
			//
			// enqueue 只有在核对过清单与文件后才返回 ready。这里返回真实 URL，而不是客户端无法
			// 播放的裸状态。
			if ready := blogAudioReady(appContainer, slug); ready != nil {
				c.JSON(http.StatusOK, ready)
				return
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "生成状态已失效，请重试"})
			return
		}
		c.JSON(http.StatusOK, blogAudioStatusResponse{Status: status, EstimateSec: est})
	})

	// Poll target while a job is running. no-store: this is per-request state, never a cache
	// candidate.
	//
	// 生成期间的轮询目标。no-store：这是逐请求状态，绝不能进缓存。
	r.GET("/blog/audio/status", func(c *gin.Context) {
		if _, ok := blogEnabled(c, appContainer); !ok {
			return
		}
		slug := strings.TrimSpace(c.Query("slug"))
		if !blogAudioSlugRe.MatchString(slug) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slug"})
			return
		}
		c.Header("Cache-Control", "no-store")

		// Job state first. A stale manifest row exists throughout a regeneration, and reading it
		// before the queue is how polling used to stop at "ready" with the old audio.
		//
		// 先看任务状态。重生成全程旧清单行都在，先读清单再读队列正是轮询拿着旧音频停在
		// "ready" 的原因。
		if st := gen.status(slug); st != nil {
			switch st.Status {
			case blogAudioQueued, blogAudioGenerating:
				c.JSON(http.StatusOK, blogAudioStatusResponse{
					Status:      st.Status,
					EstimateSec: st.Estimate,
				})
				return
			case blogAudioFailed:
				c.JSON(http.StatusOK, blogAudioStatusResponse{
					Status: blogAudioFailed,
					Error:  st.Error,
				})
				return
			case blogAudioReadyStatus:
				if ready := blogAudioReady(appContainer, slug); ready != nil && ready.TextHash == st.TextHash {
					c.JSON(http.StatusOK, ready)
					return
				}
				// The state outlived its files or the manifest was replaced. Let the client
				// ask for a fresh generation instead of offering a URL that 404s.
				//
				// 状态比文件活得久，或清单已被替换。让客户端重新请求生成，而不是给出会 404 的 URL。
				c.JSON(http.StatusOK, blogAudioStatusResponse{Status: "missing"})
				return
			}
		}

		if ready := blogAudioReady(appContainer, slug); ready != nil {
			c.JSON(http.StatusOK, ready)
			return
		}
		c.JSON(http.StatusOK, blogAudioStatusResponse{Status: "missing"})
	})

	r.GET("/blog/audio/:file", func(c *gin.Context) {
		cfg, ok := blogEnabled(c, appContainer)
		if !ok {
			return
		}
		name := c.Param("file")

		// The source table is public by design: it contains only the path hash (not the path)
		// and the already-public slug of notes that are currently shared. The local bulk
		// generator needs it to know which of its `audio: true` notes are actually published.
		//
		// 来源表是刻意公开的：它只包含路径哈希（不是路径）和当前已分享笔记的公开 slug。
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
		// 只接受扁平的生成文件名。正则拒绝分隔符与 ".."。
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
			// 不可变：URL 带有正文哈希前 10 位，重新生成的文章会得到新 URL。
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
		// 用 ServeFile 而不是 c.FileFromFS：前者实现 Range。
		http.ServeFile(c.Writer, c.Request, p)
	})
}
