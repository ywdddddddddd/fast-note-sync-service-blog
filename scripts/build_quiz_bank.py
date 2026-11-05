#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Build the quiz bank JSON consumed by internal/service/blog_quiz.go.

Sources (read-only, outside the repo):
    C:\\Users\\12917\\逆向研究\\题库导出\\merged_all.csv
    C:\\Users\\12917\\逆向研究\\题库导出\\unknown_school.csv

Output:
    internal/service/assets/quiz/bank.json

Why this script exists rather than a runtime import: the two CSVs are 10 MB of
Excel-flavoured data with a BOM, embedded newlines and a dozen fields we do not
need. Cleaning it once here keeps the binary small and the Go side free of CSV
quirks. The bank is fixed, so the output is committed and embedded.

Run:  python scripts/build_quiz_bank.py
"""

from __future__ import annotations

import json
import re
import sys
from collections import Counter
from pathlib import Path

try:
    import pandas as pd
except ImportError:  # pragma: no cover
    sys.exit("pandas required: " + sys.executable)

SRC = [
    Path(r"C:\Users\12917\逆向研究\题库导出\merged_all.csv"),
    Path(r"C:\Users\12917\逆向研究\题库导出\unknown_school.csv"),
]
OUT = Path(__file__).resolve().parent.parent / "internal" / "service" / "assets" / "quiz" / "bank.json"

# Question kinds. Keep the numbers stable — the Go side switches on them.
K_SINGLE, K_MULTI, K_JUDGE, K_SUBJ = 1, 2, 3, 4

OBJECTIVE_PREFIX = ("单选题", "多选题", "判断题")
SUBJECTIVE = {"名词解释", "简答题", "论述题", "病例分析题"}

# Trailing junk some rows carry: a bare exam year shoved into the stem.
YEAR_NOISE = re.compile(r"(?<![0-9(（])(20[0-2][0-9])(?![0-9)）年])")
WS = re.compile(r"[ \t\u3000]+")


def clean_text(s: str) -> str:
    if not isinstance(s, str):
        return ""
    s = s.replace("\r\n", "\n").replace("\r", "\n")
    s = WS.sub(" ", s)
    # A stray newline inside a stem is line-wrap noise, not structure.
    s = re.sub(r"\s*\n\s*", "", s)
    return s.strip()


def strip_year(s: str) -> str:
    """Drop exam-year noise glued onto a stem, but keep years inside brackets.

    `口腔癌有舌癌、…五种临床类型。2021` -> drops the trailing 2021.
    `（人卫8版P23）` is recovery, untouched. `2002-2024` style ranges are kept.
    """
    out = YEAR_NOISE.sub("", s)
    return out.strip()


OPT_SPLIT = re.compile(r"\s*\|\s*")
OPT_LEAD = re.compile(r"^\s*[A-Ha-h]\s*[.．、:：]\s*")


def parse_options(raw: str) -> list[str]:
    """`A. 上颌尖牙 | B. 上颌中切牙` -> ["上颌尖牙", "上颌中切牙"]."""
    if not isinstance(raw, str) or not raw.strip():
        return []
    parts = OPT_SPLIT.split(raw.replace("\n", " "))
    out = []
    for p in parts:
        p = OPT_LEAD.sub("", p).strip()
        if p:
            out.append(p)
    return out


def norm_answer_letters(raw: str) -> str:
    if not isinstance(raw, str):
        return ""
    letters = re.findall(r"[A-Ha-h]", raw)
    return "".join(sorted({c.upper() for c in letters}))


def norm_stem_key(s: str) -> str:
    """Dedup key: letters/digits/CJK only, so punctuation and spacing do not split twins."""
    return re.sub(r"[^\w\u4e00-\u9fff]", "", s).lower()


def main() -> None:
    frames = []
    for p in SRC:
        if not p.exists():
            sys.exit(f"missing source: {p}")
        frames.append(pd.read_csv(p, dtype=str, keep_default_na=False, encoding="utf-8-sig"))
    df = pd.concat(frames, ignore_index=True)
    total_rows = len(df)

    records: dict[tuple, dict] = {}
    skipped = Counter()

    for row in df.itertuples(index=False):
        d = row._asdict()
        qtype = clean_text(d.get("type", ""))
        stem = strip_year(clean_text(d.get("stem", "")))
        answer_raw = d.get("answer", "")
        recovery = clean_text(d.get("recovery", ""))
        subject = clean_text(d.get("subject", ""))
        chapter = clean_text(d.get("chapter", ""))

        if not stem:
            skipped["empty_stem"] += 1
            continue
        # A 名词解释 stem IS the term ("牙体缺损"), so the floor differs by kind: an
        # objective stem shorter than six characters is a fragment, a term is not.
        min_len = 2 if qtype in SUBJECTIVE else 6
        if len(stem) < min_len:
            skipped["stem_too_short"] += 1
            continue

        if qtype.startswith(OBJECTIVE_PREFIX):
            opts = parse_options(d.get("options", ""))
            ans = norm_answer_letters(answer_raw)
            if not opts or not ans:
                skipped["objective_no_options_or_answer"] += 1
                continue
            if any(ord(c) - ord("A") >= len(opts) for c in ans):
                skipped["answer_out_of_range"] += 1
                continue

            if qtype.startswith("单选题"):
                if len(ans) != 1:
                    skipped["single_multi_answer"] += 1
                    continue
                kind = K_SINGLE
            elif qtype.startswith("多选题"):
                # A single-letter "multiple choice" is just a single choice in disguise.
                kind = K_MULTI if len(ans) > 1 else K_SINGLE
            else:  # 判断题
                if len(opts) != 2 or ans not in ("A", "B"):
                    skipped["judge_malformed"] += 1
                    continue
                kind = K_JUDGE

            rec = {"k": kind, "s": stem, "o": opts, "a": ans}

        elif qtype in SUBJECTIVE:
            body = clean_text(answer_raw)
            if len(body) < 8:
                skipped["subjective_no_answer"] += 1
                continue
            rec = {"k": K_SUBJ, "t": qtype, "s": stem, "a": body}

        else:
            skipped["unknown_type:" + (qtype or "<empty>")] += 1
            continue

        if recovery:
            rec["r"] = recovery
        if subject:
            rec["u"] = subject
        if chapter:
            rec["c"] = chapter

        # Same question asked by twenty schools is one practice item, not twenty.
        # Keep the copy carrying the richest explanation.
        key = (rec["k"], norm_stem_key(stem))
        prev = records.get(key)
        if prev is None or len(rec.get("r", "")) > len(prev.get("r", "")):
            records[key] = rec

    items = list(records.values())
    items.sort(key=lambda x: (x["k"], x["s"]))

    OUT.parent.mkdir(parents=True, exist_ok=True)
    payload = {"v": 1, "q": items}
    raw = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    OUT.write_text(raw, encoding="utf-8")

    by_kind = Counter(r["k"] for r in items)
    labels = {K_SINGLE: "单选", K_MULTI: "多选", K_JUDGE: "判断", K_SUBJ: "主观"}
    with_subject = sum(1 for r in items if r.get("u"))
    with_chapter = sum(1 for r in items if r.get("c"))
    with_recovery = sum(1 for r in items if r.get("r"))

    print(f"源行数        {total_rows}")
    print(f"输出题数      {len(items)}   去重丢弃 {total_rows - len(items)}")
    for k, name in labels.items():
        print(f"  {name:<6} {by_kind.get(k, 0)}")
    print(f"有学科        {with_subject}  ({with_subject / len(items):.1%})")
    print(f"有章节        {with_chapter}  ({with_chapter / len(items):.1%})")
    print(f"有教材出处    {with_recovery}  ({with_recovery / len(items):.1%})")
    print(f"体积          {len(raw.encode('utf-8')) / 1e6:.2f} MB -> {OUT}")
    print("\n跳过原因:")
    for reason, n in skipped.most_common(12):
        print(f"  {reason:<32} {n}")


if __name__ == "__main__":
    main()
