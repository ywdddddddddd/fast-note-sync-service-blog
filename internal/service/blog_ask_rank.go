package service

import (
	"math"
	"sort"
	"strings"
)

// BM25 ranking for the blog Q&A retriever.
//
// The previous scorer summed raw substring counts: title×8 + path×3 + body×1, damped by
// log(length). Three properties of that formula caused the bad results measured on this vault:
//
//  1. No IDF. A token that appears in every note ("牙", "组织") contributed as much as a rare,
//     decisive one, so noise notes accumulated score simply by sharing vocabulary.
//  2. No TF saturation. Fifty occurrences scored fifty times one occurrence, letting a long
//     note win on volume alone.
//  3. Path scored ×3. Every note under one folder shares the same directory prefix, so a token
//     matching that prefix lifted the *entire folder*. Measured: "上颌第一磨牙的牙冠形态"
//     ranked 3、前磨牙 first (48.59) and the correct 4、上颌第一磨牙 fifth (42.10).
//
// BM25 fixes the first two by construction. The third is fixed by dropping the path field
// from scoring entirely — folder membership is an editorial signal that the wiki-link
// expansion pass already covers, and it is not evidence about what a note contains.
//
// 博客问答的 BM25 排序。
//
// 旧打分是裸子串计数相加：标题×8 + 路径×3 + 正文×1，再按长度取对数衰减。该公式的三个性质
// 直接造成了实测到的坏结果：
//
//  1. 没有 IDF。出现在每篇笔记里的词（「牙」「组织」）与罕见而决定性的词贡献相同，
//     噪音笔记只要共享词汇就能堆分。
//  2. 没有词频饱和。出现五十次就是一次的五十倍分，长笔记仅凭体量就能赢。
//  3. 路径计 ×3。同一目录下所有笔记共享同一串目录前缀，命中该前缀的检索词会抬升
//     *整个目录*。实测「上颌第一磨牙的牙冠形态」把《3、前磨牙》排到第一（48.59），
//     正确答案《4、上颌第一磨牙》掉到第五（42.10）。
//
// BM25 从公式上解决前两个问题；第三个通过把路径字段完全移出打分来解决——目录归属是
// 编辑信号，已由 wiki 链接扩展环节覆盖，它并不是「这篇笔记讲了什么」的证据。
const (
	// askBM25K1 controls term-frequency saturation: how quickly extra occurrences stop
	// helping. 1.2 is the standard value for short documents.
	// askBM25K1 控制词频饱和速度，即额外出现多少次之后不再加分。1.2 是短文档的常用值。
	askBM25K1 = 1.2

	// askBM25B controls length normalisation: 0 ignores length, 1 fully normalises by it.
	// 0.75 is the usual middle ground.
	// askBM25B 控制长度归一化：0 忽略长度，1 完全按长度归一。0.75 是常用的中间值。
	askBM25B = 0.75

	// Field weights. The title states what a note is about, so a hit there is worth several
	// body hits — but it still passes through TF saturation, unlike the old linear ×8.
	// 字段权重。标题陈述了笔记「是关于什么的」，因此一次标题命中值若干次正文命中——
	// 但它同样要经过词频饱和，不像旧实现的线性 ×8。
	askWeightTitle = 3.0
	askWeightBody  = 1.0

	// askSingleRuneWeight damps one-character tokens. The tokenizer emits a single rune only
	// when a Han run is one character long, and such a token matches almost everything.
	// askSingleRuneWeight 压低单字检索词。分词器只在汉字串长度为一时输出单字，
	// 而单字几乎能匹配任何东西。
	askSingleRuneWeight = 0.35

	// askLongTokenWeight lifts trigrams and whole-run tokens. A longer token is strictly more
	// specific than the bigrams it contains: 上颌第 distinguishes the upper first molar from
	// the lower one, while 颌第 does not. IDF already rewards rarity, but two tokens can share
	// a document frequency and still differ in precision, which is what this weight restores.
	//
	// askLongTokenWeight 提升三元组与整串检索词的权重。更长的检索词严格比它所包含的
	// bigram 更精确：「上颌第」能区分上颌第一磨牙与下颌第一磨牙，而「颌第」不能。
	// IDF 已经奖励了罕见度，但两个词可以有相同的文档频率却精确度不同，
	// 这个权重补回的正是这一层。
	askLongTokenWeight = 1.6

	// askScoreFloorRatio drops hits below this fraction of the top score.
	//
	// BM25 scores are comparable only within one query, so an absolute threshold would be
	// meaningless; a ratio keeps the cut relative. Without any floor, a note scoring 1.14
	// against a top score of 7.23 still entered the prompt.
	//
	// askScoreFloorRatio 丢弃低于最高分该比例的命中。
	//
	// BM25 分数只在同一次查询内有可比性，因此绝对阈值没有意义，用比例才能保持相对性。
	// 完全不设下限时，最高分 7.23 的查询里连 1.14 分的笔记也会进入提示词。
	askScoreFloorRatio = 0.15
)

// askRetrieve scores every published note against the tokens with BM25.
//
// Two passes over the corpus: the first collects document frequencies and average length, the
// second computes scores. Only the query's own tokens are counted, so no inverted index has to
// be built — a full scan of ~175 notes costs milliseconds, whereas an all-bigram index would be
// hundreds of thousands of entries rebuilt every cache TTL.
//
// askRetrieve 用 BM25 给每篇已发布笔记打分。
//
// 对语料做两遍扫描：第一遍收集文档频率与平均长度，第二遍计算分数。只统计查询自身的检索词，
// 因此无需建倒排索引——全扫约 175 篇笔记是毫秒级，而全量 bigram 索引会是十几万条目、
// 且每个缓存周期都要重建。
func askRetrieve(c *askCorpus, tokens []string, topK int) []*askHit {
	if len(tokens) == 0 || len(c.notes) == 0 {
		return nil
	}

	n := len(c.notes)
	lowerTitle := make([]string, n)
	lowerBody := make([]string, n)
	lens := make([]int, n)

	df := make(map[string]int, len(tokens))
	totalLen := 0
	for i, note := range c.notes {
		lowerTitle[i] = strings.ToLower(note.Title)
		lowerBody[i] = strings.ToLower(note.Plain)
		lens[i] = len([]rune(note.Plain))
		totalLen += lens[i]
		for _, t := range tokens {
			if strings.Contains(lowerTitle[i], t) || strings.Contains(lowerBody[i], t) {
				df[t]++
			}
		}
	}

	avgLen := float64(totalLen) / float64(n)
	if avgLen < 1 {
		avgLen = 1
	}

	idf := make(map[string]float64, len(tokens))
	for _, t := range tokens {
		d := float64(df[t])
		// The +1 keeps the value positive even for a token present in every document.
		// 加 1 保证即使某个词出现在每一篇里，权重也仍然为正。
		w := math.Log(1 + (float64(n)-d+0.5)/(d+0.5))
		switch runes := len([]rune(t)); {
		case runes == 1:
			w *= askSingleRuneWeight
		case runes >= 3:
			w *= askLongTokenWeight
		}
		idf[t] = w
	}

	hits := make([]*askHit, 0, n)
	for i, note := range c.notes {
		var score float64
		var matched []string
		norm := askBM25K1 * (1 - askBM25B + askBM25B*float64(lens[i])/avgLen)
		for _, t := range tokens {
			tf := float64(strings.Count(lowerTitle[i], t))*askWeightTitle +
				float64(strings.Count(lowerBody[i], t))*askWeightBody
			if tf <= 0 {
				continue
			}
			matched = append(matched, t)
			score += idf[t] * tf * (askBM25K1 + 1) / (tf + norm)
		}
		if score <= 0 {
			continue
		}
		hits = append(hits, &askHit{note: note, score: score, matched: matched, via: "直接命中"})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score == hits[j].score {
			// Tie-break on recency so the order is deterministic across identical queries.
			// 同分时按更新时间排序，使相同查询的结果顺序稳定。
			return hits[i].note.Sort > hits[j].note.Sort
		}
		return hits[i].score > hits[j].score
	})

	if len(hits) > 0 {
		floor := hits[0].score * askScoreFloorRatio
		cut := len(hits)
		for i, h := range hits {
			if h.score < floor {
				cut = i
				break
			}
		}
		hits = hits[:cut]
	}

	if topK > 0 && len(hits) > topK {
		hits = hits[:topK]
	}
	return hits
}
