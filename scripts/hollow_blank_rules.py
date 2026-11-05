#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Fill in the blanks the model could not reach, using rules.

Why this exists alongside hollow_blank.py: the model route that this project can reach is
rate-limited to the point where a 1200-sentence batch does not finish in a useful time
(measured: 47 sentences before the first long cooldown). The sentences themselves are
formulaic — most are "A 为 B", "A 包括 B、C、D", "A 起自 B" — so a rule pass covers a large
share of them deterministically and for free. Whatever the model already produced is kept
and never overwritten: a rule can only add.

Rules, in priority order:
    1. an enumeration (a span joined by 、) of 4–20 characters
    2. the predicate tail after a strong cue (起自 / 位于 / 又称 / 分为 / 是指 …)
    3. a measurement (number + unit)
    4. the predicate tail after a weak cue (为 / 是), only when short

Anything that fails all four is left unhollowed. A sentence whose only candidate span is the
whole sentence is not a fill-in question, it is a dictation.

Usage:
    python scripts/hollow_blank_rules.py
"""

from __future__ import annotations

import csv
import json
import re
import sys
from pathlib import Path

SRC = [
    Path(r"C:\Users\12917\逆向研究\题库导出\merged_all.csv"),
    Path(r"C:\Users\12917\逆向研究\题库导出\unknown_school.csv"),
]
OUT = Path(__file__).resolve().parent.parent / "internal" / "service" / "assets" / "quiz" / "blank.json"

MIN_CUT, MAX_CUT = 2, 20

STRONG_CUES = ["起自", "止于", "位于", "又称", "称为", "简称", "是指", "分为", "包括", "主要有", "常见于", "多见于", "表现为"]
WEAK_CUES = ["为", "是", "有", "由"]

TRAIL = "。．.；;：: "
# Longer units first. With `分` ahead of `分钟`, 磷酸锌凝固2~5分钟 produced the cut `5分`
# and left `____钟` behind — a blank that splits a word in half and an answer no reader would
# type. Measured on the first rule pass.
#
# 长单位在前。若把「分」排在「分钟」之前，磷酸锌凝固2~5分钟 会被切成 5分，留下「____钟」——
# 一个把词劈成两半的空，以及一个没人会照着写的答案。第一遍规则实测到过。
NUM_UNIT = re.compile(
    r"\d+(?:\.\d+)?\s*(?:%|％|小时|分钟|个月|周|秒|mm|cm|ml|mL|kg|mg|g|L|度|°|年|月|天|分|个|种|倍)"
)


def clean_span(s: str) -> str:
    return s.strip(TRAIL + "，,、 ")


def span_ok(span: str, sentence: str) -> bool:
    return (
        MIN_CUT <= len(span) <= MAX_CUT
        and span in sentence
        and len(span) < len(sentence) - 2
    )


def enumeration_tail(s: str) -> str:
    """The last enumeration run joined by 、, taken as a whole."""
    parts = [p for p in re.split(r"[，,]", s) if p]
    for chunk in reversed(parts):
        if "、" not in chunk:
            continue
        items = [clean_span(x) for x in chunk.split("、")]
        items = [x for x in items if x]
        if len(items) < 2:
            continue
        joined = "、".join(items)
        if span_ok(joined, s):
            return joined
    return ""


def after_cue(s: str, cues: list[str]) -> str:
    """Text following the last occurrence of any cue, trimmed to the sentence end."""
    best = -1
    for cue in cues:
        idx = s.rfind(cue)
        if idx >= 0:
            best = max(best, idx + len(cue))
    if best < 0:
        return ""
    return clean_span(s[best:])


def hollow(sentence: str) -> str:
    s = sentence.strip().rstrip(TRAIL)
    if len(s) < MIN_CUT + 3:
        return ""

    cand = _pick(s)
    if not cand:
        return ""
    # A cut that ends in a conjunction means the source sentence itself was truncated upstream
    # (二段式骨内种植体的基本组成是植入体、基台和。 — the third item is simply missing).
    # Blanking half a broken list teaches a broken fact, so these are dropped rather than
    # trimmed into something that looks complete.
    #
    # 挖空片段以连词结尾，说明源句子本身在上游就被截断了
    # （二段式骨内种植体的基本组成是植入体、基台和。——第三项根本不在）。
    # 把断掉的列举挖掉一半，教的是一个断掉的事实，因此直接丢弃，而不是修剪成一个看着完整的答案。
    if cand[-1] in "和与及或等的":
        return ""
    return cand


def _pick(s: str) -> str:
    body = s

    # 1) enumeration, anywhere in the sentence
    cand = enumeration_tail(body)
    if span_ok(cand, s):
        return cand

    # 2) strong cue: the predicate tail is usually the fact being tested
    tail = after_cue(body, STRONG_CUES)
    if span_ok(tail, s):
        return tail
    # Too long? Prefer the enumeration inside the tail if there is one.
    if tail:
        inner = enumeration_tail(tail)
        if span_ok(inner, s):
            return inner

    # 3) a measurement
    m = NUM_UNIT.search(body)
    if m:
        cand = clean_span(m.group(0))
        if span_ok(cand, s):
            return cand

    # 4) weak cue, only when the remainder is short enough to be a single fact
    tail = after_cue(body, WEAK_CUES)
    if span_ok(tail, s):
        return tail

    return ""


def main() -> None:
    rows = []
    for p in SRC:
        with p.open(encoding="utf-8-sig", newline="") as f:
            rows.extend(csv.DictReader(f))

    out_path = OUT
    existing: dict[str, dict] = {}
    if out_path.exists():
        try:
            for it in json.loads(out_path.read_text(encoding="utf-8")).get("items", []):
                if it.get("cut") and it.get("masked"):
                    existing[it["stem"]] = it
        except json.JSONDecodeError:
            sys.exit(f"{out_path} is not valid JSON")
    print(f"已有（模型产出）{len(existing)} 条，本次只补空缺")

    seen = set()
    added = 0
    rejected = 0
    for r in rows:
        if r.get("type") != "填空题":
            continue
        stem = (r.get("stem") or "").strip()
        answer = (r.get("answer") or "").strip()
        if not stem or stem != answer or stem in seen:
            continue
        seen.add(stem)
        if stem in existing:
            continue
        # Exam years pasted into the sentence are noise, not content.
        if re.search(r"20[0-2][0-9]", stem):
            rejected += 1
            continue

        cut = hollow(stem)
        if not cut:
            rejected += 1
            continue
        idx = stem.index(cut)
        existing[stem] = {
            "stem": stem,
            "masked": stem[:idx] + "____" + stem[idx + len(cut):],
            "cut": cut,
            "school": (r.get("school") or "").strip(),
            "year": (r.get("year") or "").strip(),
            "subject": (r.get("subject") or "").strip(),
            "chapter": (r.get("chapter") or "").strip(),
        }
        added += 1

    items = list(existing.values())
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(
        json.dumps({"v": 1, "items": items}, ensure_ascii=False, separators=(",", ":")),
        encoding="utf-8",
    )
    print(f"规则新增 {added}，规则判为不可挖 {rejected}")
    print(f"合计 {len(items)} 条 -> {out_path}  {out_path.stat().st_size / 1024:.0f} KB")


if __name__ == "__main__":
    main()
