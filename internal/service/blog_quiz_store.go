package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
	"go.uber.org/zap"
)

// ── Storage ────────────────────────────────────────────────────────────────────────────────
//
// One JSON file, same shape of thinking as blog-reactions.json: an in-memory copy is
// authoritative, every mutation takes the lock, and the write goes through a temp file and
// a rename so a crash cannot leave a truncated document behind.
//
// 一个 JSON 文件，与 blog-reactions.json 同一套心智：内存副本是权威，每次写入持锁，
// 落盘先写临时文件再改名，因此崩溃不会留下被截断的文档。

const blogQuizFileName = "blog-quiz.json"

// blogQuizMSMax is the longest elapsed time one attempt may report: thirty minutes.
//
// The browser measures with Date.now(), so an honest attempt is seconds to a few minutes.
// Anything longer is a stale tab, a clock jump, or a forged payload; clamping keeps the
// leaderboard's speed tie-break meaningful. Negative values are discarded outright, because a
// negative time would sort ahead of every honest attempt.
//
// blogQuizMSMax 是单次作答允许上报的最长耗时：三十分钟。
//
// 浏览器用 Date.now() 计时，诚实作答是几秒到几分钟。更长的值是搁置的标签页、时钟跳变或伪造
// 载荷；夹到上限让榜单的速度并列规则仍然有意义。负值直接丢弃——负数会排到所有诚实作答之前。
const blogQuizMSMax int64 = 30 * 60 * 1000

// blogQuizBest is one reader's best attempt on one note.
//
// blogQuizBest 是一个读者在一篇笔记上的最好成绩。
type blogQuizBest struct {
	Rate    float64 `json:"rate"`
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	MS      int64   `json:"ms"`
	At      int64   `json:"at"`
}

// blogQuizTotal is the all-site cumulative figure for one reader.
//
// blogQuizTotal 是一个读者的全站累计数字。
type blogQuizTotal struct {
	Correct int   `json:"correct"`
	Notes   int   `json:"notes"`
	At      int64 `json:"at"`
}

// blogQuizWrong is one wrong-question entry. Mastered entries are kept rather than deleted:
// "I used to get this wrong" is information the reader may want back, and deleting on
// success would also make the book flicker between visits.
//
// blogQuizWrong 是错题本的一条。已掌握的条目保留而不删除："我以前错这题"是读者可能想回看的
// 信息；而且答对即删会让错题本在两次访问之间忽长忽短。
type blogQuizWrong struct {
	NotePath  string `json:"notePath"`
	NoteSlug  string `json:"noteSlug"`
	NoteTitle string `json:"noteTitle"`
	Wrong     int    `json:"wrong"`
	At        int64  `json:"at"`
	Mastered  bool   `json:"mastered"`
}

type blogQuizStore struct {
	// Notes is notePath -> emailHash -> best. Keyed by path rather than slug because a
	// re-share changes the slug and the history would be orphaned.
	//
	// Notes 是 notePath → emailHash → 最好成绩。用路径而不是 slug 作键：重新分享会换 slug，
	// 那会把历史变成孤儿。
	Notes  map[string]map[string]*blogQuizBest `json:"notes"`
	Totals map[string]*blogQuizTotal           `json:"totals"`
	// Emails maps hash -> masked display form. The raw address is never written.
	//
	// Emails 是 hash → 打码后的展示形态。原文地址从不落盘。
	Emails map[string]string                    `json:"emails"`
	Wrong  map[string]map[string]*blogQuizWrong `json:"wrong"`
}

type blogQuizLog struct {
	mu    sync.Mutex
	path  string
	log   *zap.Logger
	store blogQuizStore
	ready bool
}

func newBlogQuizLog(dir string, logger *zap.Logger) *blogQuizLog {
	if dir == "" {
		return nil
	}
	return &blogQuizLog{
		path: filepath.Join(dir, blogQuizFileName),
		log:  logger,
	}
}

func (b *blogQuizLog) ensure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
}

func (b *blogQuizLog) ensureLocked() {
	if b.ready {
		return
	}
	b.ready = true
	b.store = blogQuizStore{
		Notes:  map[string]map[string]*blogQuizBest{},
		Totals: map[string]*blogQuizTotal{},
		Emails: map[string]string{},
		Wrong:  map[string]map[string]*blogQuizWrong{},
	}
	raw, err := os.ReadFile(b.path)
	if err != nil {
		return
	}
	var loaded blogQuizStore
	if err := json.Unmarshal(raw, &loaded); err != nil {
		if b.log != nil {
			b.log.Error("blog quiz: file is not valid JSON, starting empty and keeping a copy",
				zap.String("path", b.path), zap.Error(err))
		}
		_ = os.WriteFile(b.path+".broken-"+time.Now().Format("20060102-150405"), raw, 0o644)
		return
	}
	if loaded.Notes != nil {
		b.store.Notes = loaded.Notes
	}
	if loaded.Totals != nil {
		b.store.Totals = loaded.Totals
	}
	if loaded.Emails != nil {
		b.store.Emails = loaded.Emails
	}
	if loaded.Wrong != nil {
		b.store.Wrong = loaded.Wrong
	}
}

func (b *blogQuizLog) saveLocked() {
	raw, err := json.Marshal(&b.store)
	if err != nil {
		return
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		if b.log != nil {
			b.log.Error("blog quiz: write failed", zap.String("path", tmp), zap.Error(err))
		}
		return
	}
	if err := os.Rename(tmp, b.path); err != nil && b.log != nil {
		b.log.Error("blog quiz: rename failed", zap.String("path", b.path), zap.Error(err))
	}
}

func (b *blogQuizLog) update(fn func(s *blogQuizStore)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
	fn(&b.store)
	b.saveLocked()
}

// ── Session cache ──────────────────────────────────────────────────────────────────────────
//
// Matching a note against ~13k questions costs a few milliseconds; doing it on every page
// view would be wasteful, and doing it once per deploy would break the "a new note gets
// exercises without a deploy" requirement. So it is cached per slug and invalidated by the
// hash of the note's rendered text.
//
// 一篇笔记对上约一万三千道题要几毫秒；每次访问都算一遍是浪费，而只在部署时算一次又违背
// "新笔记无需部署就有练习"这条要求。于是按 slug 缓存，用笔记渲染文本的哈希作失效键。

type blogQuizSession struct {
	notePath   string
	hash       string
	objective  []dto.BlogQuizQuestionDTO
	subjective []dto.BlogQuizQuestionDTO
	byID       map[string]dto.BlogQuizQuestionDTO
	at         time.Time
}

var (
	blogQuizSessionsMu sync.Mutex
	blogQuizSessions   = map[string]*blogQuizSession{}
)

const blogQuizSessionCap = 256

func blogQuizSessionGet(slug, hash string) *blogQuizSession {
	blogQuizSessionsMu.Lock()
	defer blogQuizSessionsMu.Unlock()
	s, ok := blogQuizSessions[slug]
	if !ok || s.hash != hash {
		return nil
	}
	s.at = time.Now()
	return s
}

func blogQuizSessionPut(slug string, s *blogQuizSession) {
	blogQuizSessionsMu.Lock()
	defer blogQuizSessionsMu.Unlock()
	s.at = time.Now()
	blogQuizSessions[slug] = s
	if len(blogQuizSessions) <= blogQuizSessionCap {
		return
	}
	// Evict the least recently touched entry. A real LRU list would be more precise, but
	// the corpus here is a few hundred notes and the consequence of a wrong eviction is one
	// recomputation.
	//
	// 淘汰最久未触碰的一条。真正的 LRU 会更精确，但这里的规模是几百篇笔记，
	// 淘汰错一条的代价只是一次重算。
	var oldestKey string
	var oldest time.Time
	for k, v := range blogQuizSessions {
		if oldestKey == "" || v.at.Before(oldest) {
			oldestKey, oldest = k, v.at
		}
	}
	if oldestKey != "" && oldestKey != slug {
		delete(blogQuizSessions, oldestKey)
	}
}

// ── Text preparation ───────────────────────────────────────────────────────────────────────

var (
	quizTagRe = regexp.MustCompile(`<[^>]+>`)
	// Interpreted string, not a raw one: Go's regexp has no \u escape, so the ideographic
	// space must reach the pattern as an actual rune. Written as a raw string it would be
	// the two characters backslash-u and the compiler would panic at init time.
	//
	// 这里用解释型字符串而不是原始字符串：Go 的 regexp 没有 \u 转义，全角空格必须以真实字符
	// 进入模式。写成原始字符串的话进到模式里的是反斜杠加 u 两个字符，程序会在 init 阶段 panic。
	quizSpaceRe = regexp.MustCompile("[ \t\u3000]+")
	quizBreakRe = regexp.MustCompile(`\s*\n\s*`)
)

// quizPlainFromHTML strips markup so the matcher sees words, not tags. It works on the
// rendered article rather than the markdown source because that is what the render path
// already holds — and because going back for the source would mean a second database read
// per page view just to power a feature most readers never open.
//
// quizPlainFromHTML 去掉标记，让匹配器看到的是文字而不是标签。它以渲染后的文章为输入，
// 因为渲染路径本来就有它；若回头去取原文，就意味着每次访问多一次数据库读取，
// 而多数读者根本不打开练习。
func quizPlainFromHTML(s string) string {
	s = strings.NewReplacer("</p>", "\n", "</li>", "\n", "<br>", "\n", "<br/>", "\n", "</h1>", "\n",
		"</h2>", "\n", "</h3>", "\n", "</h4>", "\n", "</tr>", "\n", "</td>", " ").Replace(s)
	s = quizTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = quizSpaceRe.ReplaceAllString(s, " ")
	s = quizBreakRe.ReplaceAllString(s, "\n")
	return strings.TrimSpace(s)
}

func quizHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// quizSubjectOf maps a note path to a bank subject. Only the folder name is used, because it
// is the one part of a path that survives note renames.
//
// quizSubjectOf 把笔记路径映射到题库学科。只用目录名——它是路径里唯一能在笔记改名后活下来的部分。
func quizSubjectOf(notePath string) string {
	p := strings.ReplaceAll(notePath, "\\", "/")
	switch {
	case strings.Contains(p, "修复笔记"):
		return "口腔修复学"
	case strings.Contains(p, "口外笔记"), strings.Contains(p, "种植笔记"):
		return "口腔颌面外科学"
	case strings.Contains(p, "口组笔记"):
		return "口腔组织病理学"
	case strings.Contains(p, "口解笔记"):
		return "口腔解剖生理学"
	case strings.Contains(p, "正畸笔记"):
		return "口腔正畸学"
	case strings.Contains(p, "牙体笔记"):
		return "牙体牙髓病学"
	}
	return ""
}

// quizQuestionDTO converts a hit into the wire form and derives a content-stable id.
//
// quizQuestionDTO 把命中转成传输形态，并派生一个与内容绑定的 id。
func quizQuestionDTO(h QuizHit) dto.BlogQuizQuestionDTO {
	id := h.Stem + "|" + h.Answer
	sum := sha256.Sum256([]byte(id))
	src := h.Chapter
	if src == "" {
		src = h.Subject
	}
	return dto.BlogQuizQuestionDTO{
		ID:      hex.EncodeToString(sum[:10]),
		Kind:    h.Kind,
		Stem:    h.Stem,
		Options: h.Options,
		Answer:  h.Answer,
		Explain: h.Recovery,
		Source:  src,
		School:  h.School,
		Year:    h.Year,
	}
}

// buildQuizSession runs the matcher for one note and caches the outcome.
//
// buildQuizSession 为一篇笔记跑一次匹配并缓存结果。
func buildQuizSession(slug, notePath, plain string) *blogQuizSession {
	obj, sub := QuizMatch(plain, quizSubjectOf(notePath), 10, 6)
	sess := &blogQuizSession{
		notePath:   notePath,
		hash:       quizHash(plain),
		objective:  make([]dto.BlogQuizQuestionDTO, 0, len(obj)),
		subjective: make([]dto.BlogQuizQuestionDTO, 0, len(sub)),
		byID:       make(map[string]dto.BlogQuizQuestionDTO, len(obj)+len(sub)),
	}
	for _, h := range obj {
		q := quizQuestionDTO(h)
		sess.objective = append(sess.objective, q)
		sess.byID[q.ID] = q
	}
	for _, h := range sub {
		q := quizQuestionDTO(h)
		sess.subjective = append(sess.subjective, q)
		sess.byID[q.ID] = q
	}
	blogQuizSessionPut(slug, sess)
	return sess
}

// ── Public service surface ────────────────────────────────────────────────────────────────

// BlogQuizPreview returns the counts and leaderboard strip the article page renders. It does
// not return questions; those only ever travel over POST.
//
// BlogQuizPreview 返回文章页要渲染的计数与榜单条。它不返回题目；题目只经 POST 传输。
func (s *shareService) BlogQuizPreview(post *dto.BlogPostDTO) *dto.BlogQuizDTO {
	if post == nil || post.Path == "" || post.HTML == "" {
		return nil
	}
	plain := quizPlainFromHTML(post.HTML)
	if len([]rune(plain)) < 200 {
		return nil
	}
	// Only the dental-notes tree gets exercises. The vault also holds 健康生活, 能力提升 and
	// 随笔 folders, and a note about sleep hygiene must not be quizzed on oral pathology just
	// because it shares the word 压力. The folder mapping is the gate, not the match score —
	// a score threshold would still let an outlier through on some future note.
	//
	// 只有口腔笔记树下的笔记有练习。仓库里还有健康生活、能力提升、随笔等目录，
	// 而一篇讲睡眠的笔记不该仅仅因为出现了「压力」二字就被考口腔病理。这里用目录归属作闸门，
	// 而不是匹配分数——阈值总会让将来的某篇笔记漏进来。
	subject := quizSubjectOf(post.Path)
	if subject == "" {
		return nil
	}
	hash := quizHash(plain)
	sess := blogQuizSessionGet(post.Slug, hash)
	if sess == nil {
		sess = buildQuizSession(post.Slug, post.Path, plain)
	}
	if len(sess.objective) == 0 && len(sess.subjective) == 0 {
		return nil
	}
	out := &dto.BlogQuizDTO{
		NotePath:   sess.notePath,
		Objective:  len(sess.objective),
		Subjective: len(sess.subjective),
	}
	if s.blogQuiz != nil {
		s.blogQuiz.ensure()
		s.blogQuiz.mu.Lock()
		out.Top = s.quizBoardLocked(sess.notePath, "", 5)
		s.blogQuiz.mu.Unlock()
	}
	return out
}

// BlogQuizPick returns both question pools for one note. The session is rebuilt on demand:
// a server restart empties the cache, and a page open before the restart must still be able
// to start practising.
//
// BlogQuizPick 返回一篇笔记的两个题池。会话按需重建：重启会清空缓存，
// 而重启前打开的页面仍然必须能开始练习。
func (s *shareService) BlogQuizPick(ctx context.Context, slug string) (*dto.BlogQuizPickDTO, error) {
	post, err := s.RenderBlogBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	plain := quizPlainFromHTML(post.HTML)
	sess := blogQuizSessionGet(slug, quizHash(plain))
	if sess == nil {
		sess = buildQuizSession(slug, post.Path, plain)
	}
	return &dto.BlogQuizPickDTO{
		NotePath:   sess.notePath,
		Objective:  sess.objective,
		Subjective: sess.subjective,
	}, nil
}

// BlogQuizSubmit re-judges an attempt server-side and records the outcome.
//
// The browser is trusted for nothing but its own selections: correct/total/rate are all
// recomputed here from the cached session, every selection is checked against the options the
// server published, and the client's Total is ignored entirely. A reader who edits the page's
// JavaScript can change what their screen says; they cannot change what is stored.
//
// BlogQuizSubmit 在服务端重新判一次作答并记录结果。
//
// 除了读者选了什么，浏览器不被信任任何东西：正确数、总数、正确率全部在这里按缓存的题目重算，
// 每个选择都要对照服务端下发的选项校验，而客户端的 Total 完全不被读取。
// 改了页面 JS 的读者能改变自己屏幕上显示的东西，改变不了存下来的东西。
func (s *shareService) BlogQuizSubmit(ctx context.Context, in *dto.BlogQuizSubmitDTO) (*dto.BlogQuizResultDTO, error) {
	post, err := s.RenderBlogBySlug(ctx, in.Slug)
	if err != nil {
		return nil, err
	}
	plain := quizPlainFromHTML(post.HTML)
	sess := blogQuizSessionGet(in.Slug, quizHash(plain))
	if sess == nil {
		sess = buildQuizSession(in.Slug, post.Path, plain)
	}

	// in.Total exists only so the wire shape stays compatible. The denominator is the server's
	// own question count: a payload that claims total=1 cannot turn one correct guess into
	// 100%, and one that omits half the questions cannot shrink the denominator to match.
	//
	// in.Total 只为保持传输结构兼容而存在。分母是服务端自己的题数：声称 total=1 的载荷
	// 不能把一次蒙对变成 100%，漏掉一半题目的载荷也不能把分母缩到与自己一致。
	correct, wrongIDs, total := quizJudgeAttempt(sess.objective, in.Selections)
	if total == 0 {
		return &dto.BlogQuizResultDTO{}, nil
	}

	// The elapsed time is a client measurement and is treated as such: negatives become
	// "unknown" and anything past thirty minutes is clamped to the ceiling. It is stored and
	// used only as a tie-break, never as evidence that an answer was correct.
	//
	// 耗时是客户端测量值，也按客户端测量值对待：负数变成「未知」，超过三十分钟的夹到上限。
	// 它只被存储并用作并列时的次序，从不用来证明答案正确。
	ms := quizNormalizeMS(in.MS)

	hash := quizEmailHash(in.Email)
	masked := quizMaskEmail(in.Email)
	out := &dto.BlogQuizResultDTO{
		Correct:  correct,
		Total:    total,
		Rate:     float64(correct) / float64(total),
		WrongIDs: wrongIDs,
	}
	if hash == "" {
		return out, nil
	}

	if s.blogQuiz != nil {
		now := time.Now().Unix()
		s.blogQuiz.update(func(st *blogQuizStore) {
			if st.Emails == nil {
				st.Emails = map[string]string{}
			}
			st.Emails[hash] = masked

			byNote := st.Notes[sess.notePath]
			if byNote == nil {
				byNote = map[string]*blogQuizBest{}
				st.Notes[sess.notePath] = byNote
			}
			prev := byNote[hash]
			rate := float64(correct) / float64(total)
			better := prev == nil || rate > prev.Rate || (rate == prev.Rate && ms > 0 && (prev.MS <= 0 || ms < prev.MS))
			if better {
				byNote[hash] = &blogQuizBest{Rate: rate, Correct: correct, Total: total, MS: ms, At: now}
			}

			tot := st.Totals[hash]
			if tot == nil {
				tot = &blogQuizTotal{}
				st.Totals[hash] = tot
			}
			if better {
				tot.Correct += correct - bestCorrectOf(prev)
			}
			tot.Notes = len(byNote)
			tot.At = now

			if st.Wrong == nil {
				st.Wrong = map[string]map[string]*blogQuizWrong{}
			}
			book := st.Wrong[hash]
			if book == nil {
				book = map[string]*blogQuizWrong{}
				st.Wrong[hash] = book
			}
			for _, id := range wrongIDs {
				item := book[id]
				if item == nil {
					item = &blogQuizWrong{}
					book[id] = item
				}
				item.NotePath = sess.notePath
				item.NoteSlug = in.Slug
				item.NoteTitle = post.Title
				item.Wrong++
				item.At = now
				item.Mastered = false
			}
			wrongSet := make(map[string]struct{}, len(wrongIDs))
			for _, id := range wrongIDs {
				wrongSet[id] = struct{}{}
			}
			for id, item := range book {
				if item.NotePath != sess.notePath {
					continue
				}
				if _, stillWrong := wrongSet[id]; !stillWrong {
					item.Mastered = true
				}
			}
		})

		s.blogQuiz.ensure()
		s.blogQuiz.mu.Lock()
		out.Top = s.quizBoardLocked(sess.notePath, hash, 8)
		out.Me = s.quizMeLocked(sess.notePath, hash)
		out.TotalRank, out.TotalCount, out.TotalPoints = s.quizTotalRankLocked(hash)
		s.blogQuiz.mu.Unlock()
	}
	return out, nil
}

// BlogQuizWrongBook returns one reader's outstanding wrong questions, newest first.
//
// BlogQuizWrongBook 返回某位读者尚未掌握的错题，最近的在前。
func (s *shareService) BlogQuizWrongBook(email string) []dto.BlogQuizWrongDTO {
	hash := quizEmailHash(email)
	if hash == "" || s.blogQuiz == nil {
		return nil
	}
	s.blogQuiz.ensure()
	s.blogQuiz.mu.Lock()
	defer s.blogQuiz.mu.Unlock()

	book := s.blogQuiz.store.Wrong[hash]
	if len(book) == 0 {
		return nil
	}
	// The question text is not stored in the book: it lives in the per-note session, and
	// keeping a copy in the ranking file would let the two drift.
	//
	// 错题本不存题干：题干在按笔记划分的会话里，在榜单文件里再存一份会让两边漂移。
	out := make([]dto.BlogQuizWrongDTO, 0, len(book))
	for id, item := range book {
		if item.Mastered {
			continue
		}
		q, ok := s.quizQuestionByIDLocked(item.NotePath, id)
		if !ok {
			continue
		}
		out = append(out, dto.BlogQuizWrongDTO{
			Question:  q,
			NotePath:  item.NotePath,
			NoteSlug:  item.NoteSlug,
			NoteTitle: item.NoteTitle,
			Wrong:     item.Wrong,
			At:        time.Unix(item.At, 0).Format("2006-01-02"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
	return out
}

// quizQuestionByIDLocked looks a question up in the cached session for a note. Called with
// blogQuiz.mu held; it takes the session lock separately.
//
// quizQuestionByIDLocked 在缓存的会话里查一道题。调用时持有 blogQuiz.mu，它自己另取会话锁。
func (s *shareService) quizQuestionByIDLocked(notePath, id string) (dto.BlogQuizQuestionDTO, bool) {
	blogQuizSessionsMu.Lock()
	defer blogQuizSessionsMu.Unlock()
	for _, sess := range blogQuizSessions {
		if sess.notePath != notePath {
			continue
		}
		if q, ok := sess.byID[id]; ok {
			return q, true
		}
	}
	return dto.BlogQuizQuestionDTO{}, false
}

// ── Judging ───────────────────────────────────────────────────────────────────────────────

// quizJudgeAttempt scores a submission against the server's own question set.
//
// Selections is a lookup table, never a source of questions: the loop walks the session's
// objective pool, so an id the server does not know is unreachable and an id it knows but the
// client omitted counts as wrong. Total is len(objective) — nothing the client can shrink.
//
// Each selection is also checked against the options the server published; letters past the
// end of the option list (or past H, the last letter the page renders) are not something the
// rendered page could have produced, so they are marked wrong rather than silently matched.
//
// quizJudgeAttempt 用服务端自己的题集给一次提交判分。
//
// Selections 只是一张查找表，绝不是题目来源：循环遍历会话里的客观题池，因此服务端不认识的 id
// 无法触达，而服务端认识、客户端却漏掉的 id 计为答错。总数是 len(objective)——客户端缩不小它。
//
// 每个选择还要对照服务端下发的选项校验；超出选项表末尾（或超出页面最后渲染的 H）的字母，
// 不是渲染页面能产生的东西，因此判错而不是静默比对。
func quizJudgeAttempt(objective []dto.BlogQuizQuestionDTO, selections map[string]string) (correct int, wrongIDs []string, total int) {
	total = len(objective)
	wrongIDs = make([]string, 0, 4)
	for _, q := range objective {
		picked, ok := selections[q.ID]
		if ok && quizSelectionValid(q, picked) && quizAnswerMatches(q.Kind, q.Answer, picked) {
			correct++
			continue
		}
		wrongIDs = append(wrongIDs, q.ID)
	}
	return correct, wrongIDs, total
}

// quizSelectionValid reports whether picked only names options this question actually has.
//
// Blank questions carry typed text and are judged by similarity, so they always pass here.
// Subjective questions never reach this function (they are self-assessed). For objective
// questions the ceiling is the shorter of the option count and eight — the page renders A–H
// only, and quizLetters reads no letter past H.
//
// quizSelectionValid 判断 picked 是否只引用了这道题真正拥有的选项。
//
// 填空题携带手打文本、按相似度判分，因此这里一律放行。主观题不会走到这里（它们是自评的）。
// 客观题的上限取选项数与 8 的较小者——页面只渲染 A–H，而 quizLetters 也只读得到 H。
func quizSelectionValid(q dto.BlogQuizQuestionDTO, picked string) bool {
	if q.Kind == quizKindBlank || q.Kind == quizKindSubj {
		return true
	}
	limit := len(q.Options)
	if limit > 8 {
		limit = 8
	}
	if limit == 0 {
		return false
	}
	letters := quizLetters(picked)
	if letters == "" {
		return false
	}
	for i := 0; i < len(letters); i++ {
		if int(letters[i]-'A') >= limit {
			return false
		}
	}
	return true
}

// quizNormalizeMS turns a client-reported elapsed time into the value the leaderboard may use.
//
// Zero means "unknown" rather than "instant": the browser's timer can be missing, and a zero
// must never win a speed tie-break (see quizFaster). Negative values are not credible and
// become zero; values beyond the thirty-minute ceiling are clamped rather than discarded, so an
// honest reader who left the page open overnight still keeps a recorded score.
//
// quizNormalizeMS 把客户端上报的耗时变成榜单可用的值。
//
// 零表示「未知」而不是「瞬间完成」：浏览器计时可能缺失，而零绝不能赢下速度并列（见 quizFaster）。
// 负值不可信，归零；超过三十分钟上限的值夹到上限而不是丢弃，使把页面开了一夜的诚实读者
// 仍然保有一次已记录的成绩。
func quizNormalizeMS(ms int64) int64 {
	switch {
	case ms <= 0:
		return 0
	case ms > blogQuizMSMax:
		return blogQuizMSMax
	default:
		return ms
	}
}

// quizFaster reports whether a should be listed ahead of b when rates tie.
//
// Zero means "no credible time": it is what a discarded or never-recorded measurement becomes,
// and it must sort *behind* every positive time. The previous comparison was a bare `<`, so a
// forged ms=0 (or a legacy negative) won every tie-break on the board.
//
// quizFaster 判断正确率并列时 a 是否应排在 b 之前。
//
// 零表示「没有可信用时」：被丢弃或从未记录的测量值都会变成它，而它必须排在所有正数用时之后。
// 旧的比较是裸的 `<`，于是一个伪造的 ms=0（或历史遗留的负数）能赢下榜单上的每一次并列。
func quizFaster(a, b int64) bool {
	if a <= 0 {
		return false
	}
	if b <= 0 {
		return true
	}
	return a < b
}

// quizAnswerMatches compares a submitted selection with the stored answer.
//
// Objective answers are letter sets, so order and separators must not matter: a reader who
// clicks B then A has given the same answer as one who clicks A then B. Subjective questions
// never reach here — they are self-assessed.
//
// quizAnswerMatches 把提交的选择与保存的答案比较。
//
// 客观题的答案是字母集合，因此顺序与分隔符都不该影响结果：先点 B 再点 A 的读者
// 与先点 A 再点 B 的读者给的是同一个答案。主观题不会走到这里——它们是自评的。
func quizAnswerMatches(kind int, answer, picked string) bool {
	if kind == quizKindBlank {
		want := quizNormalizeText(answer)
		got := quizNormalizeText(picked)
		if want == "" || got == "" {
			return false
		}
		if got == want {
			return true
		}
		// Hollowed sentences ask the reader to retype a span of the sentence they just read,
		// and a correct recollection rarely reproduces it character for character: 软腭与硬腭
		// 交界的部位 comes back as 软腭与硬腭交界处. Exact matching would mark that wrong and
		// destroy the exercise; unbounded fuzziness would mark 骨上袋 correct for 骨下袋.
		// The line is drawn at three quarters of the最长 side, measured by longest common
		// subsequence so that inserted or dropped characters cost proportionally.
		//
		// 挖空题要求读者把刚看过的句子里的某一段重新打出来，而"记住了"很少逐字复现：
		// 软腭与硬腭交界的部位 会被写成 软腭与硬腭交界处。逐字比对会把这种判错，练习就废了；
		// 而无限宽松又会把 骨上袋 判成 骨下袋。线画在较长一边的四分之三处，用最长公共子序列
		// 度量，使多写或少写一个字按比例扣分。
		return quizSimilarity(got, want) >= 0.75
	}
	want := quizLetters(answer)
	got := quizLetters(picked)
	if len(want) == 0 || len(got) == 0 {
		return false
	}
	if kind != quizKindMulti {
		return want == got
	}
	ws := make([]byte, 0, len(want))
	gs := make([]byte, 0, len(got))
	for i := 0; i < len(want); i++ {
		ws = append(ws, want[i])
	}
	for i := 0; i < len(got); i++ {
		gs = append(gs, got[i])
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i] < ws[j] })
	sort.Slice(gs, func(i, j int) bool { return gs[i] < gs[j] })
	return string(ws) == string(gs)
}

func quizLetters(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if r >= 'A' && r <= 'H' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	seen := make([]byte, 0, len(out))
	for i := 0; i < len(out); i++ {
		dup := false
		for _, c := range seen {
			if c == out[i] {
				dup = true
				break
			}
		}
		if !dup {
			seen = append(seen, out[i])
		}
	}
	sort.Slice(seen, func(i, j int) bool { return seen[i] < seen[j] })
	return string(seen)
}

// quizNormalizeText reduces a typed answer to what the reader meant, ignoring how they
// typed it: full-width punctuation, spacing, and case carry no information in a term like
// 中1／3段 versus 中1/3段.
//
// It deliberately does NOT do fuzzy matching. The accepted answer is a verbatim span lifted
// from the sentence the reader just studied, so a near-miss usually means they half-remembered
// it — and telling them they were right would remove the only signal this exercise produces.
//
// quizNormalizeText 把输入的答案归一成读者"想写的东西"，忽略书写形式：全角标点、空格、
// 大小写，在「中1／3段」与「中1/3段」之间不承载任何信息。
//
// 它刻意不做模糊匹配。正确答案是从读者刚看过的句子里逐字摘出来的片段，差一点通常意味着
// 记了一半——那时判他正确，就把这个练习唯一能给出的信号抹掉了。
func quizNormalizeText(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			r = r - 0xFF00 + 0x20 // full-width ASCII -> half-width
		case r == 0x3000:
			continue
		case r == '％':
			r = '%'
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		switch r {
		case '，', '。', '、', '；', '：', '（', '）', '【', '】', '《', '》', '“', '”', '‘', '’',
			',', '.', ';', ':', '(', ')', '[', ']', '"', '\'', '·', '-', '_':
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// quizSimilarity is the longest common subsequence over the longer of the two strings.
// Subsequence rather than edit distance because the failure mode here is a missing or extra
// character in the middle of a remembered phrase, and that should cost the same whether it
// is an insertion or a deletion.
//
// quizSimilarity 是两串中较长者上的最长公共子序列比例。用子序列而不是编辑距离，是因为这里的
// 失败形态是"记住的短语中间多一个或少一个字"，而多与少应当扣同样的分。
func quizSimilarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			if ra[i-1] == rb[j-1] {
				cur[j] = prev[j-1] + 1
			} else if prev[j] >= cur[j-1] {
				cur[j] = prev[j]
			} else {
				cur[j] = cur[j-1]
			}
		}
		prev, cur = cur, prev
	}
	longer := len(ra)
	if len(rb) > longer {
		longer = len(rb)
	}
	return float64(prev[len(rb)]) / float64(longer)
}

// ── Leaderboards ──────────────────────────────────────────────────────────────────────────

// quizBoardLocked builds one note's board, ordered by rate then speed.
//
// Rate alone would let a reader who answered two questions beat one who answered ten, so
// the tie-break is speed and the row always shows correct/total. Ties beyond that fall back
// to name order to keep the list stable between renders.
//
// quizBoardLocked 构建一篇笔记的榜单，按正确率、再按用时排序。
//
// 只看正确率会让人答两道题就压过答十道题的人，因此并列时比速度，并且每行都显示正确数/总数。
// 再并列则按名字排序，使两次渲染之间的顺序稳定。
func (s *shareService) quizBoardLocked(notePath, selfHash string, limit int) []dto.BlogQuizRankDTO {
	byNote := s.blogQuiz.store.Notes[notePath]
	if len(byNote) == 0 {
		return nil
	}
	type item struct {
		hash string
		best *blogQuizBest
	}
	items := make([]item, 0, len(byNote))
	for h, b := range byNote {
		items = append(items, item{h, b})
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].best, items[j].best
		if a.Rate != b.Rate {
			return a.Rate > b.Rate
		}
		// A recorded time always outranks an unknown one; two unknown times fall through to
		// the hash so the order is stable instead of depending on map iteration.
		//
		// 有记录的用时永远排在未知用时之前；两个未知用时落到哈希比较，
		// 使顺序稳定，而不是取决于 map 遍历顺序。
		aTimed, bTimed := a.MS > 0, b.MS > 0
		if aTimed != bTimed {
			return aTimed
		}
		if aTimed && a.MS != b.MS {
			return a.MS < b.MS
		}
		return items[i].hash < items[j].hash
	})
	out := make([]dto.BlogQuizRankDTO, 0, limit)
	for i, it := range items {
		if i >= limit {
			break
		}
		out = append(out, dto.BlogQuizRankDTO{
			Name:    s.quizNameLocked(it.hash),
			Rate:    it.best.Rate,
			Correct: it.best.Correct,
			Total:   it.best.Total,
			MS:      it.best.MS,
			Rank:    i + 1,
			Self:    selfHash != "" && it.hash == selfHash,
		})
	}
	return out
}

func (s *shareService) quizMeLocked(notePath, hash string) *dto.BlogQuizRankDTO {
	byNote := s.blogQuiz.store.Notes[notePath]
	best := byNote[hash]
	if best == nil {
		return nil
	}
	rank := 1
	for h, b := range byNote {
		if h == hash {
			continue
		}
		if b.Rate > best.Rate || (b.Rate == best.Rate && quizFaster(b.MS, best.MS)) {
			rank++
		}
	}
	return &dto.BlogQuizRankDTO{
		Name:    s.quizNameLocked(hash),
		Rate:    best.Rate,
		Correct: best.Correct,
		Total:   best.Total,
		MS:      best.MS,
		Rank:    rank,
		Self:    true,
	}
}

func (s *shareService) quizTotalRankLocked(hash string) (rank, total int, points int) {
	totals := s.blogQuiz.store.Totals
	me := totals[hash]
	rank = 1
	for h, t := range totals {
		if h == hash {
			continue
		}
		if t.Correct > me.Correct || (t.Correct == me.Correct && t.At < me.At) {
			rank++
		}
	}
	if me == nil {
		rank = 0
	} else {
		points = me.Correct
	}
	return rank, len(totals), points
}

func (s *shareService) quizNameLocked(hash string) string {
	if n, ok := s.blogQuiz.store.Emails[hash]; ok && n != "" {
		return n
	}
	if len(hash) >= 4 {
		return "读者" + hash[:4]
	}
	return "匿名读者"
}

// quizEmailHash is a truncated SHA-256 of the lowercased address. Truncation is deliberate:
// 64 bits is far beyond what a name collision needs here, and a shorter key keeps the JSON
// readable when someone has to look into the file by hand.
//
// quizEmailHash 是小写地址的截断 SHA-256。截断是刻意的：64 位远超这里的重名分辨需要，
// 而更短的键让 JSON 在必须手查文件时仍然可读。
func quizEmailHash(email string) string {
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" || !strings.Contains(e, "@") {
		return ""
	}
	sum := sha256.Sum256([]byte(e))
	return hex.EncodeToString(sum[:8])
}

// quizMaskEmail keeps the first character and the domain: z***@qq.com.
//
// quizMaskEmail 保留首字符与域名：z***@qq.com。
func quizMaskEmail(email string) string {
	e := strings.TrimSpace(email)
	at := strings.LastIndex(e, "@")
	if at <= 0 {
		return "***"
	}
	local := []rune(e[:at])
	head := "***"
	if len(local) > 0 {
		head = string(local[:1]) + "***"
	}
	return head + e[at:]
}

func bestCorrectOf(b *blogQuizBest) int {
	if b == nil {
		return 0
	}
	return b.Correct
}
