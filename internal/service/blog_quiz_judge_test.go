package service

import (
	"testing"

	"github.com/haierkeys/fast-note-sync-service/internal/dto"
)

// TestQuizJudgeAttemptUsesServerQuestionSet is the anti-forgery test for the exercise endpoint.
//
// The three properties that matter, in the order a cheater would try them:
//
//   - selections keyed by ids the server never published are ignored — a script cannot add
//     questions, and therefore cannot add correct answers;
//   - questions the client omitted are counted wrong rather than dropped from the denominator;
//   - the denominator is len(server pool), so a payload that claims a smaller total cannot
//     turn one correct answer into 100%.
//
// TestQuizJudgeAttemptUsesServerQuestionSet 是练习接口的防伪测试。
//
// 三条性质按作弊者的尝试顺序排列：
//
//   - 服务端从未发布过的 id 被忽略——脚本无法增加题目，也就无法增加正确答案；
//   - 客户端漏掉的题计为答错，而不是从分母里消失；
//   - 分母是服务端题池的长度，因此声称更小 total 的载荷不能把一题答对变成 100%。
func TestQuizJudgeAttemptUsesServerQuestionSet(t *testing.T) {
	objective := []dto.BlogQuizQuestionDTO{
		{ID: "q1", Kind: quizKindSingle, Options: []string{"甲", "乙", "丙", "丁"}, Answer: "B"},
		{ID: "q2", Kind: quizKindMulti, Options: []string{"甲", "乙", "丙", "丁"}, Answer: "AC"},
		{ID: "q3", Kind: quizKindJudge, Options: []string{"对", "错"}, Answer: "A"},
		{ID: "q4", Kind: quizKindBlank, Answer: "纵𬌗曲线"},
	}

	selections := map[string]string{
		"q1":         "B",     // correct
		"q2":         "CA",    // correct: order and duplicate separators must not matter
		"q3":         "B",     // wrong
		"forged-001": "A",     // never published by the server
		"forged-002": "ABCD",  // never published by the server
		"q4":         "纵牙合曲线", // wrong: similar but below the blank threshold
	}

	correct, wrongIDs, total := quizJudgeAttempt(objective, selections)
	if correct != 2 {
		t.Fatalf("correct = %d, 期望 2（q1 与 q2）", correct)
	}
	if total != len(objective) {
		t.Fatalf("total = %d, 期望 %d（服务端题数，而非客户端声称值）", total, len(objective))
	}
	if len(wrongIDs) != 2 || wrongIDs[0] != "q3" || wrongIDs[1] != "q4" {
		t.Fatalf("wrongIDs = %v, 期望 [q3 q4]（伪造 id 不得出现在结果里）", wrongIDs)
	}
}

// TestQuizJudgeAttemptCountsMissingSelectionsAsWrong covers the shrink-the-denominator attack:
// submitting answers for one question out of ten must not produce a 1/1 score.
//
// TestQuizJudgeAttemptCountsMissingSelectionsAsWrong 覆盖「缩小分母」攻击：
// 十道题只提交一道答案，不能得到 1/1。
func TestQuizJudgeAttemptCountsMissingSelectionsAsWrong(t *testing.T) {
	objective := make([]dto.BlogQuizQuestionDTO, 0, 10)
	for i := 0; i < 10; i++ {
		objective = append(objective, dto.BlogQuizQuestionDTO{
			ID:      "q" + string(rune('0'+i)),
			Kind:    quizKindSingle,
			Options: []string{"甲", "乙"},
			Answer:  "A",
		})
	}
	correct, wrongIDs, total := quizJudgeAttempt(objective, map[string]string{"q0": "A"})
	if correct != 1 || total != 10 || len(wrongIDs) != 9 {
		t.Fatalf("correct=%d total=%d wrong=%d, 期望 1/10 与 9 道错题", correct, total, len(wrongIDs))
	}
}

// TestQuizSelectionValidRejectsOptionsThePageCannotProduce pins the option check separately
// from the score: a four-option question accepts A–D and nothing else.
//
// TestQuizSelectionValidRejectsOptionsThePageCannotProduce 把选项校验与得分分开钉住：
// 四选项的题只接受 A–D，别的都不行。
func TestQuizSelectionValidRejectsOptionsThePageCannotProduce(t *testing.T) {
	single := dto.BlogQuizQuestionDTO{Kind: quizKindSingle, Options: []string{"甲", "乙", "丙", "丁"}, Answer: "A"}
	cases := []struct {
		name   string
		q      dto.BlogQuizQuestionDTO
		picked string
		want   bool
	}{
		{"四选项选 A", single, "A", true},
		{"四选项选 D", single, "D", true},
		{"小写与分隔符归一", single, " d ", true},
		{"四选项选 E（超出选项表）", single, "E", false},
		{"四选项选 H（超出选项表）", single, "H", false},
		{"空选择", single, "", false},
		{"非字母选择", single, "甲", false},
		{"多选超范围", dto.BlogQuizQuestionDTO{Kind: quizKindMulti, Options: []string{"甲", "乙", "丙"}, Answer: "AB"}, "ABCD", false},
		{"多选范围内", dto.BlogQuizQuestionDTO{Kind: quizKindMulti, Options: []string{"甲", "乙", "丙"}, Answer: "AB"}, "BA", true},
		{"八选项上限", dto.BlogQuizQuestionDTO{Kind: quizKindMulti, Options: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}, Answer: "H"}, "H", true},
		{"超过八选项仍只认到 H", dto.BlogQuizQuestionDTO{Kind: quizKindMulti, Options: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}, Answer: "A"}, "I", false},
		{"填空题放行文本", dto.BlogQuizQuestionDTO{Kind: quizKindBlank, Answer: "任意"}, "任 意", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := quizSelectionValid(tc.q, tc.picked); got != tc.want {
				t.Fatalf("quizSelectionValid(%q, %q) = %v, 期望 %v", tc.q.Answer, tc.picked, got, tc.want)
			}
		})
	}
}

// TestQuizNormalizeMS pins the discard/clamp policy for the client-measured elapsed time.
//
// TestQuizNormalizeMS 钉住客户端耗时「丢弃/夹取」的策略。
func TestQuizNormalizeMS(t *testing.T) {
	cases := []struct {
		in, want int64
	}{
		{-1, 0},
		{-3_600_000, 0},
		{0, 0},
		{1, 1},
		{1_500, 1_500},
		{blogQuizMSMax, blogQuizMSMax},
		{blogQuizMSMax + 1, blogQuizMSMax},
		{24 * 3600 * 1000, blogQuizMSMax},
	}
	for _, tc := range cases {
		if got := quizNormalizeMS(tc.in); got != tc.want {
			t.Fatalf("quizNormalizeMS(%d) = %d, 期望 %d", tc.in, got, tc.want)
		}
	}
}

// TestQuizBoardTreatsUnknownTimeAsWorst is the leaderboard-side half of the ms fix.
//
// A zero or negative time is what a discarded measurement (or a forged payload from before the
// fix) looks like. It must sort behind a genuine recorded time at the same rate, and the order
// among several unknown times must be stable rather than map-iteration order.
//
// TestQuizBoardTreatsUnknownTimeAsWorst 是 ms 修复的榜单一侧。
//
// 零或负数是被丢弃的测量值（或修复前的伪造载荷）的样子。在正确率相同时它必须排在真实用时之后，
// 而多个未知用时之间的顺序必须稳定，不能取决于 map 遍历顺序。
func TestQuizBoardTreatsUnknownTimeAsWorst(t *testing.T) {
	s := &shareService{blogQuiz: &blogQuizLog{store: blogQuizStore{
		Notes: map[string]map[string]*blogQuizBest{
			"note/a": {
				"aaaa": {Rate: 1, Correct: 10, Total: 10, MS: 0},
				"bbbb": {Rate: 1, Correct: 10, Total: 10, MS: 1_500},
				"cccc": {Rate: 1, Correct: 10, Total: 10, MS: -20},
			},
		},
		Totals: map[string]*blogQuizTotal{},
		Emails: map[string]string{},
		Wrong:  map[string]map[string]*blogQuizWrong{},
	}}}

	rows := s.quizBoardLocked("note/a", "", 10)
	if len(rows) != 3 {
		t.Fatalf("榜单应有 3 行，实际 %d", len(rows))
	}
	if rows[0].MS != 1_500 {
		t.Fatalf("榜首应是真实用时 1500ms 的记录，实际 %+v", rows[0])
	}
	if rows[1].MS > 0 || rows[2].MS > 0 {
		t.Fatalf("未知用时（0 与负数）必须排在真实用时之后，实际 %+v", rows)
	}

	// The same ranking rule must apply to the reader's own "my rank" row.
	// 同一条排序规则也必须作用于读者自己的「我的名次」。
	me := s.quizMeLocked("note/a", "aaaa")
	if me == nil || me.Rank != 2 {
		t.Fatalf("ms=0 的读者应排第 2（次于 1500ms 的记录），实际 %+v", me)
	}
}

// TestQuizFasterNeverPrefersUnknownTime states the comparison in isolation, because both the
// submit path and the board consume it.
//
// TestQuizFasterNeverPrefersUnknownTime 单独钉住这个比较函数，因为提交路径与榜单都消费它。
func TestQuizFasterNeverPrefersUnknownTime(t *testing.T) {
	if quizFaster(0, 1_000) {
		t.Fatal("ms=0 不应比 1000ms 更快")
	}
	if quizFaster(-5, 1_000) {
		t.Fatal("负耗时不应比真实用时更快")
	}
	if !quizFaster(1_000, 0) {
		t.Fatal("真实用时应排在 ms=0 之前")
	}
	if !quizFaster(800, 1_000) {
		t.Fatal("800ms 应比 1000ms 快")
	}
	if quizFaster(1_000, 800) {
		t.Fatal("1000ms 不应比 800ms 快")
	}
}
