package dto

// BlogQuizQuestionDTO is one question as the browser receives it.
//
// ID is a content-derived key rather than a bank row number: the bank is regenerated from
// CSVs between deploys, and row numbers are not stable across that. The wrong-question book
// is keyed by this ID, so a shift in numbering must not silently invalidate a reader's
// history.
//
// BlogQuizQuestionDTO 是浏览器收到的一道题。
//
// ID 由内容推导而不是题库行号：题库在两次部署之间会从 CSV 重新生成，行号并不稳定。
// 错题本以这个 ID 为键，因此编号一变不该让读者的历史无声失效。
type BlogQuizQuestionDTO struct {
	ID      string   `json:"id"`
	Kind    int      `json:"kind"` // 1 单选 2 多选 3 判断 4 主观 5 填空
	Stem    string   `json:"stem"`
	Options []string `json:"options,omitempty"`
	Answer  string   `json:"answer,omitempty"` // objective: letters; subjective/bank: full text
	Explain string   `json:"explain,omitempty"`
	Source  string   `json:"source,omitempty"` // chapter or subject, for the "出处" line
	// School and Year are the exam the question was taken from. Rendered as one small line
	// under the stem — 四川大学 · 2015 — because a reader deciding whether a question is
	// representative cares who set it and when.
	//
	// School 与 Year 是这道题的来源考试，在题干下方渲染成一行小字——四川大学 · 2015。
	// 读者判断这道题是否有代表性时，会在意是谁、哪一年出的。
	School string `json:"school,omitempty"`
	Year   string `json:"year,omitempty"`
}

// BlogQuizDTO is what the article page carries. It deliberately holds counts and a rank
// strip, never the questions themselves — the questions arrive only through POST so that
// they never enter the page HTML or any GET response.
//
// BlogQuizDTO 是文章页携带的内容。它刻意只带计数与名次条，绝不含题目本身：
// 题目只经 POST 下发，因此既不会进入页面 HTML，也不会出现在任何 GET 响应里。
type BlogQuizDTO struct {
	NotePath   string            `json:"notePath"`
	Objective  int               `json:"objective"`
	Subjective int               `json:"subjective"`
	Top        []BlogQuizRankDTO `json:"top,omitempty"`
	Me         *BlogQuizRankDTO  `json:"me,omitempty"`
}

// BlogQuizRankDTO is one row of a leaderboard. Name is always the masked form.
//
// BlogQuizRankDTO 是榜单的一行。Name 永远是打码后的形态。
type BlogQuizRankDTO struct {
	Name    string  `json:"name"`
	Rate    float64 `json:"rate"`
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	MS      int64   `json:"ms"`
	Rank    int     `json:"rank"`
	Self    bool    `json:"self,omitempty"`
}

// BlogQuizPickDTO is the response to "start practising": both pools, answers included.
//
// BlogQuizPickDTO 是"开始练习"的响应：两个题池，含答案。
type BlogQuizPickDTO struct {
	NotePath   string                `json:"notePath"`
	Objective  []BlogQuizQuestionDTO `json:"objective"`
	Subjective []BlogQuizQuestionDTO `json:"subjective"`
}

// BlogQuizSubmitDTO is one finished attempt.
//
// Wrong carries the IDs the reader got wrong, computed by the browser for display speed.
// The server recounts from Selections and never trusts the count it is given.
//
// BlogQuizSubmitDTO 是一次完成的作答。
//
// Wrong 携带读者答错的题 ID，由浏览器算出以便立即展示。服务端按 Selections 重算，
// 从不相信它收到的数字。
type BlogQuizSubmitDTO struct {
	Email      string            `json:"email"`
	NotePath   string            `json:"notePath"`
	Selections map[string]string `json:"selections"` // question id -> chosen letters / text
	Total      int               `json:"total"`
	MS         int64             `json:"ms"`
}

// BlogQuizResultDTO is what submit returns: the authoritative score plus the refreshed
// leaderboard rows.
//
// BlogQuizResultDTO 是提交的返回：权威得分加上刷新后的榜单行。
type BlogQuizResultDTO struct {
	Correct     int               `json:"correct"`
	Total       int               `json:"total"`
	Rate        float64           `json:"rate"`
	WrongIDs    []string          `json:"wrongIds"`
	Top         []BlogQuizRankDTO `json:"top"`
	Me          *BlogQuizRankDTO  `json:"me"`
	TotalRank   int               `json:"totalRank"`   // position on the all-site board
	TotalCount  int               `json:"totalCount"`  // number of readers on it
	TotalPoints int               `json:"totalPoints"` // cumulative correct answers
}

// BlogQuizWrongDTO is one entry of the wrong-question book.
//
// BlogQuizWrongDTO 是错题本的一条。
type BlogQuizWrongDTO struct {
	Question  BlogQuizQuestionDTO `json:"question"`
	NotePath  string              `json:"notePath"`
	NoteSlug  string              `json:"noteSlug"`
	NoteTitle string              `json:"noteTitle"`
	Wrong     int                 `json:"wrong"`
	At        string              `json:"at"`
}
