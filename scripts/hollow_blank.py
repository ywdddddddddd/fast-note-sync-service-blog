#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Hollow out the contaminated fill-in-the-blank questions.

The bank stores 1378 fill-in questions whose `stem` is byte-identical to `answer`:
the "question" is the finished sentence, with no gap anywhere. Replacing `answer`
with a blank therefore blanks the entire stem (measured: 1378/1378). What we need is
not the answer text but *which span of the sentence is worth removing*.

This script asks a language model for exactly that span — the substring, not a rewrite
— and then verifies locally that the returned span really occurs in the stem. A span
that does not match is dropped, so a bad model degrades into fewer questions rather
than into wrong ones.

Output is merged into the quiz bank by build_quiz_bank.py.

Usage:
    python scripts/hollow_blank.py --routes <path-to-ai-yaml-fragment> [--limit N]

The routes file is a copy of the service's `ai:` config stanza. It contains API keys
and must never be committed.
"""

from __future__ import annotations

import argparse
import json
import os
import random
import re
import sys
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path

SRC = [
    Path(r"C:\Users\12917\逆向研究\题库导出\merged_all.csv"),
    Path(r"C:\Users\12917\逆向研究\题库导出\unknown_school.csv"),
]
OUT = Path(__file__).resolve().parent.parent / "internal" / "service" / "assets" / "quiz" / "blank.json"

BATCH = 12
MIN_CUT, MAX_CUT = 3, 20

SYSTEM = (
    "你是口腔医学题库编辑。用户给你若干「填空原句」：每条都是一个完整的陈述句，"
    "句子本身就是要背的答案，原文里没有任何空缺。\n"
    "你的任务是给每条挑出一段**最值得挖掉的原文片段**，使剩下的句子成为一个好的填空题。\n"
    "挑选标准：\n"
    "1. 挖掉的应该是最需要背住的信息——专有名词、数值与单位、并列项、分类名、机制或定义中的核心短语；\n"
    "2. 长度 3 到 20 个汉字，过短没有区分度，过长等于把整句删掉；\n"
    "3. **必须是原句中的连续子串，一字不改**，不要改写字词、不要加省略号；\n"
    "4. 挖空后剩下的部分仍要能读懂句意，能作为提示；\n"
    "5. 如果这句话无法合理挖空（本身是碎片、病句、错字粘连、或整句只有三五个字），cut 返回空字符串。\n"
    "只输出 JSON，形如 {\"items\":[{\"i\":1,\"cut\":\"…\"}]}，不要任何解释文字。"
)


def load_routes(path: Path) -> list[dict]:
    txt = path.read_text(encoding="utf-8-sig")
    out = []
    for block in txt.split("- name:")[1:]:
        base = re.search(r"base-url:\s*(\S+)", block)
        key = re.search(r"api-key:\s*(\S+)", block)
        model = re.search(r"model:\s*(\S+)", block)
        enabled = re.search(r"enabled:\s*(\w+)", block)
        if not (base and key and model):
            continue
        if enabled and enabled.group(1).lower() == "false":
            continue
        headers = {}
        # The service config nests extra headers two levels under `headers:`; capture the
        # whole tail and pick `name: "value"` pairs until the block ends.
        h = re.search(r"headers:\s*\n((?:\s+[\w-]+:.*\n?)+)", block)
        if h:
            for line in h.group(1).splitlines():
                m = re.match(r'\s+([\w-]+):\s*"?([^"\n]+)"?\s*$', line)
                if m:
                    headers[m.group(1)] = m.group(2).strip()
        out.append({
            "base": base.group(1).strip("\"'"),
            "key": key.group(1).strip("\"'"),
            "model": model.group(1).strip("\"'"),
            "headers": headers,
        })
    return out


def needs_thinking_object(base_url: str) -> bool:
    """Mirrors askRouteUsesThinkingObject in blog_ask.go.

    Both SenseNova and OpenCode ignore `enable_thinking` and switch reasoning through a
    `thinking` object. Sending the boolean there fails silently — empty content, no error —
    which for a batch job would look like "the model had nothing to say".
    """
    u = base_url.lower()
    return "sensenova" in u or "opencode.ai" in u


def chat(route: dict, user_text: str, timeout: int = 180) -> str:
    body = {
        "model": route["model"],
        "messages": [
            {"role": "system", "content": SYSTEM},
            {"role": "user", "content": user_text},
        ],
        "temperature": 0,
        "stream": False,
        "max_tokens": 4000,
    }
    if needs_thinking_object(route["base"]):
        # `disabled` rather than `enabled`: this job is pure extraction, and under a modest
        # max_tokens an upstream that spends its budget on reasoning returns empty content.
        #
        # 用 disabled 而不是 enabled：这是纯抽取任务，而推理型上游在 max_tokens 不高时会把预算
        # 花在 reasoning 上、返回空 content。
        body["thinking"] = {"type": "disabled"}

    headers = {"Content-Type": "application/json", "Authorization": "Bearer " + route["key"]}
    for k, v in route.get("headers", {}).items():
        headers[k] = v.replace("${uuid}", str(uuid.uuid4()))

    req = urllib.request.Request(
        route["base"].rstrip("/") + "/chat/completions",
        data=json.dumps(body, ensure_ascii=False).encode("utf-8"),
        headers=headers,
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        data = json.loads(resp.read().decode("utf-8"))
    return data["choices"][0]["message"]["content"]


def parse_items(raw: str) -> dict[int, str]:
    raw = raw.strip()
    # Models like to wrap JSON in a fenced block even when told not to.
    raw = re.sub(r"^```(?:json)?\s*|\s*```$", "", raw)
    start, end = raw.find("{"), raw.rfind("}")
    if start < 0 or end < 0:
        return {}
    try:
        doc = json.loads(raw[start:end + 1])
    except json.JSONDecodeError:
        return {}
    out = {}
    for item in doc.get("items", []):
        try:
            i = int(item.get("i"))
        except (TypeError, ValueError):
            continue
        cut = item.get("cut") or ""
        if isinstance(cut, str):
            out[i] = cut.strip()
    return out


def load_candidates() -> list[dict]:
    import csv

    rows = []
    for p in SRC:
        with p.open(encoding="utf-8-sig", newline="") as f:
            rows.extend(csv.DictReader(f))

    seen = set()
    out = []
    for r in rows:
        if r.get("type") != "填空题":
            continue
        stem = (r.get("stem") or "").strip()
        answer = (r.get("answer") or "").strip()
        if not stem or stem != answer:
            continue
        if not (MIN_CUT + 2 <= len(stem) <= 80):
            continue
        if re.search(r"20[0-2][0-9]", stem):
            # Exam years pasted into the sentence are noise, not content.
            continue
        if stem in seen:
            continue
        seen.add(stem)
        out.append({
            "s": stem,
            "school": (r.get("school") or "").strip(),
            "year": (r.get("year") or "").strip(),
            "subject": (r.get("subject") or "").strip(),
            "chapter": (r.get("chapter") or "").strip(),
        })
    return out


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--routes", required=True)
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--sleep", type=float, default=1.5)
    ap.add_argument("--out", default=str(OUT))
    args = ap.parse_args()

    routes = load_routes(Path(args.routes))
    if not routes:
        sys.exit("no usable route in routes file")

    cands = load_candidates()
    if args.limit:
        cands = cands[: args.limit]
    print(f"候选填空句 {len(cands)} 条，可用线路 {len(routes)} 条")

    out_path = Path(args.out)
    done: dict[str, dict] = {}
    if out_path.exists():
        try:
            prev = json.loads(out_path.read_text(encoding="utf-8"))
            for it in prev.get("items", []):
                done[it["stem"]] = it
        except json.JSONDecodeError:
            pass

    todo = [c for c in cands if c["s"] not in done]
    print(f"已完成 {len(done)}，本次待处理 {len(todo)}")

    ri = 0
    failures = 0
    dead: set[int] = set()
    for start in range(0, len(todo), BATCH):
        chunk = todo[start:start + BATCH]
        payload = json.dumps(
            {"items": [{"i": i + 1, "s": c["s"]} for i, c in enumerate(chunk)]},
            ensure_ascii=False,
        )
        items = {}
        attempts = 0
        while attempts < len(routes) * 3:
            attempts += 1
            k = (ri + attempts) % len(routes)
            if k in dead:
                continue
            route = routes[k]
            try:
                raw = chat(route, payload)
                items = parse_items(raw)
                if items:
                    ri = k
                    break
                # Parsed nothing: the upstream answered but not in the shape asked for.
                print(f"  unparsable reply from {route['model']}", file=sys.stderr)
            except urllib.error.HTTPError as e:
                if e.code in (401, 403, 404):
                    # A rejected key stays rejected; retrying it just burns the batch's clock.
                    # 401/403/404 是"这把 key 不被接受"，重试只会白耗时间。
                    dead.add(k)
                    print(f"  HTTP {e.code} {route['model']} marked dead", file=sys.stderr)
                elif e.code == 429:
                    print(f"  HTTP 429 {route['model']}, cooling down", file=sys.stderr)
                    time.sleep(10)
                else:
                    print(f"  HTTP {e.code} {route['model']}", file=sys.stderr)
                    time.sleep(3)
            except Exception as e:  # noqa: BLE001 - any transport failure moves to the next route
                print(f"  error {type(e).__name__}: {e}", file=sys.stderr)
                time.sleep(3)
        if not items:
            failures += 1
            print(f"  批 {start // BATCH + 1} 全线路失败，跳过", file=sys.stderr)

        for i, c in enumerate(chunk, start=1):
            cut = items.get(i, "")
            ok = (
                cut
                and MIN_CUT <= len(cut) <= MAX_CUT
                and cut in c["s"]
                and len(cut) < len(c["s"]) - 2
            )
            if ok:
                idx = c["s"].index(cut)
                masked = c["s"][:idx] + "____" + c["s"][idx + len(cut):]
                done[c["s"]] = {
                    "stem": c["s"],
                    "masked": masked,
                    "cut": cut,
                    "school": c["school"],
                    "year": c["year"],
                    "subject": c["subject"],
                    "chapter": c["chapter"],
                }
            else:
                done[c["s"]] = {
                    "stem": c["s"],
                    "masked": "",
                    "cut": "",
                    "school": c["school"],
                    "year": c["year"],
                    "subject": c["subject"],
                    "chapter": c["chapter"],
                }

        out_path.parent.mkdir(parents=True, exist_ok=True)
        kept = [v for v in done.values() if v["cut"]]
        out_path.write_text(
            json.dumps({"v": 1, "items": kept}, ensure_ascii=False, separators=(",", ":")),
            encoding="utf-8",
        )
        print(f"批 {start // BATCH + 1}/{(len(todo) + BATCH - 1) // BATCH}  累计产出 {len(kept)}")
        time.sleep(args.sleep)

    kept = [v for v in done.values() if v["cut"]]
    print(f"\n完成：{len(kept)} 条可挖空 / {len(done)} 条尝试，失败批次 {failures}")
    print(f"输出 {out_path}  {out_path.stat().st_size / 1024:.0f} KB")


if __name__ == "__main__":
    main()
