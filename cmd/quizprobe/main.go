// Command quizprobe is an offline measurement tool, not part of the server.
//
// It runs the same matching code the render path will use against a directory of notes and
// prints, for each note, the questions that would be offered — plus a summary that says how
// many notes would end up with too few questions. The point is to calibrate the thresholds
// in blog_quiz.go with evidence instead of taste, and to catch the failure mode where a
// note matches twenty questions that all belong to a different chapter.
//
// Run:
//
//	go run ./cmd/quizprobe -dir "D:\obsnote\Obsidian\口腔\（1）口腔笔记" -show 6
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	fns "github.com/haierkeys/fast-note-sync-service/internal/service"
)

type row struct {
	path       string
	subject    string
	chars      int
	objective  int
	subjective int
	topScore   float64
}

func main() {
	dir := flag.String("dir", "", "笔记根目录（递归）")
	show := flag.Int("show", 5, "详细打印前 N 篇")
	topObj := flag.Int("obj", 10, "每篇取多少道客观题")
	topSubj := flag.Int("subj", 6, "每篇取多少道主观题")
	only := flag.String("only", "", "只详细打印路径含该子串的笔记")
	sweep := flag.Bool("sweep", false, "扫描不同阈值下的覆盖情况")
	cover := flag.Float64("min", 0, "覆盖默认 MinScore（0 表示用默认值）")
	shared := flag.Int("shared", 0, "覆盖默认 minShared（0 表示用默认值）")
	long := flag.Int("long", -1, "覆盖默认 minLongShared（-1 表示用默认值）")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir 必填")
		os.Exit(2)
	}

	fmt.Printf("题库规模 %d 题\n", fns.QuizBankSize())
	p50, p90, p99, mx := fns.QuizBankIDFStats()
	fmt.Printf("idf 分位  p50=%.2f  p90=%.2f  p99=%.2f  max=%.2f\n\n", p50, p90, p99, mx)

	opt := fns.DefaultQuizOptions()
	if *cover > 0 {
		opt.MinScore = *cover
	}
	if *shared > 0 {
		opt.MinShared = *shared
	}
	if *long >= 0 {
		opt.MinLongShared = *long
	}
	fmt.Printf("阈值 minShared=%d minLong=%d minScore=%.2f boost=%.2f penalty=%.2f\n\n",
		opt.MinShared, opt.MinLongShared, opt.MinScore, opt.SubjectBoost, opt.SubjectPenalty)

	var rows []row
	left := *show
	// sweepCounts[i] counts, for threshold sweepCover[i], how many notes reach >=10.
	type noteText struct {
		path    string
		subject string
		plain   string
	}
	var corpus []noteText
	sweepCover := []float64{0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 5.0}
	sweepHits := make([]int, len(sweepCover))
	sweepAvg := make([]float64, len(sweepCover))

	err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		plain := stripMarkdown(string(raw))
		subject := subjectOf(path)
		corpus = append(corpus, noteText{rel(*dir, path), subject, plain})

		obj, sub := fns.QuizMatchWith(plain, subject, *topObj, *topSubj, opt)

		r := row{
			path:       rel(*dir, path),
			subject:    subject,
			chars:      len([]rune(plain)),
			objective:  len(obj),
			subjective: len(sub),
		}
		if len(obj) > 0 {
			r.topScore = obj[0].Score
		}
		rows = append(rows, r)

		if left > 0 && (*only == "" || strings.Contains(r.path, *only)) {
			left--
			printNote(r, obj, sub)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *sweep {
		for i, c := range sweepCover {
			o := opt
			o.MinScore = c
			total := 0
			enough := 0
			for _, n := range corpus {
				obj, _ := fns.QuizMatchWith(n.plain, n.subject, 50, 0, o)
				total += len(obj)
				if len(obj) >= 10 {
					enough++
				}
			}
			sweepHits[i] = enough
			if len(corpus) > 0 {
				sweepAvg[i] = float64(total) / float64(len(corpus))
			}
		}
		fmt.Println("━━ 阈值扫描（每篇最多取 50 道客观题）")
		fmt.Printf("%-8s %-12s %s\n", "MinScore", "≥10题篇数", "篇均题数")
		for i, c := range sweepCover {
			fmt.Printf("%-8.1f %-12d %.1f\n", c, sweepHits[i], sweepAvg[i])
		}
		fmt.Println()
	}

	printSummary(rows)
}

func printNote(r row, obj, sub []fns.QuizHit) {
	fmt.Printf("━━ %s\n", r.path)
	fmt.Printf("   学科=%s 正文%d字 | 客观%d 主观%d | 首题分=%.3f\n",
		r.subject, r.chars, r.objective, r.subjective, r.topScore)
	for i, h := range obj {
		if i >= 4 {
			break
		}
		fmt.Printf("   [%.2f cov=%.2f Σ=%.1f] %s\n", h.Score, h.Coverage, h.SharedIDF, truncate(h.Stem, 44))
		fmt.Printf("            共用词 %s\n", sharedDetail(h.Shared, 6))
	}
	for i, h := range sub {
		if i >= 2 {
			break
		}
		fmt.Printf("   [%.2f cov=%.2f Σ=%.1f] %s\n", h.Score, h.Coverage, h.SharedIDF, truncate(h.Stem, 44))
	}
	fmt.Println()
}

func printSummary(rows []row) {
	if len(rows) == 0 {
		return
	}

	buckets := map[string]int{}
	for _, r := range rows {
		switch {
		case r.objective >= 10:
			buckets["客观≥10"]++
		case r.objective >= 5:
			buckets["客观5-9"]++
		case r.objective >= 1:
			buckets["客观1-4"]++
		default:
			buckets["客观0"]++
		}
	}

	fmt.Println("━━ 汇总")
	fmt.Printf("笔记总数 %d\n", len(rows))
	for _, k := range []string{"客观≥10", "客观5-9", "客观1-4", "客观0"} {
		fmt.Printf("  %-8s %d 篇\n", k, buckets[k])
	}

	var noObj []string
	for _, r := range rows {
		if r.objective == 0 {
			noObj = append(noObj, r.path)
		}
	}
	if len(noObj) > 0 {
		fmt.Printf("\n配不到客观题的 %d 篇：\n", len(noObj))
		sort.Strings(noObj)
		for _, p := range noObj {
			fmt.Printf("  · %s\n", p)
		}
	}
}

// subjectOf maps a note's folder to the bank's subject value. The folder is the only signal
// in this vault that survives renames, and getting it wrong for 种植笔记 is expected — that
// directory has no matching subject in the bank at all.
func subjectOf(path string) string {
	p := filepath.ToSlash(path)
	switch {
	case strings.Contains(p, "修复笔记"):
		return "口腔修复学"
	case strings.Contains(p, "口外笔记"):
		return "口腔颌面外科学"
	case strings.Contains(p, "口组笔记"):
		return "口腔组织病理学"
	case strings.Contains(p, "口解笔记"):
		return "口腔解剖生理学"
	case strings.Contains(p, "正畸笔记"):
		return "口腔正畸学"
	case strings.Contains(p, "牙体笔记"):
		return "牙体牙髓病学"
	case strings.Contains(p, "种植笔记"):
		return "口腔颌面外科学"
	}
	return ""
}

var (
	reFront = regexp.MustCompile(`(?s)^---\n.*?\n---\n`)
	reCode  = regexp.MustCompile("(?s)```.*?```")
	reIMG   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reLINK  = regexp.MustCompile(`\[\[([^\]|]*)(?:\|([^\]]*))?\]\]`)
	reMDLNK = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	reTAG   = regexp.MustCompile(`<[^>]+>`)
	reEMPH  = regexp.MustCompile("[*_~`]+")
	reBULL  = regexp.MustCompile(`(?m)^\s*[-*+>#]+\s*`)
)

// stripMarkdown approximates what the server will feed the matcher. It is intentionally
// crude: the production path uses the service's own blogPlainText, and this probe only has
// to be close enough to calibrate a threshold.
func stripMarkdown(s string) string {
	s = reFront.ReplaceAllString(s, "")
	s = reCode.ReplaceAllString(s, " ")
	s = reIMG.ReplaceAllString(s, " ")
	s = reLINK.ReplaceAllStringFunc(s, func(m string) string {
		sub := reLINK.FindStringSubmatch(m)
		if len(sub) > 2 && sub[2] != "" {
			return sub[2]
		}
		if len(sub) > 1 {
			return sub[1]
		}
		return ""
	})
	s = reMDLNK.ReplaceAllString(s, "$1")
	s = reTAG.ReplaceAllString(s, " ")
	s = reEMPH.ReplaceAllString(s, "")
	s = reBULL.ReplaceAllString(s, "")
	return s
}

func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(r)
}

func truncate(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// sharedDetail renders shared terms together with how common each one is in the bank.
// Without the df number every term looks equally convincing in the log.
func sharedDetail(terms []string, limit int) string {
	parts := make([]string, 0, limit)
	for i, t := range terms {
		if i >= limit {
			break
		}
		df, _ := fns.QuizTermStats(t)
		parts = append(parts, fmt.Sprintf("%s(%d)", t, df))
	}
	return strings.Join(parts, " ")
}
