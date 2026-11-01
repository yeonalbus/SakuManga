# -*- coding: utf-8 -*-
"""校验：落档文档第一部分每个代码块内容与 JSONL 对应 user/message 的 text 字段逐字节一致。

用法：python _verify.py <session.jsonl> <doc.md> <start_key>
"""
import io
import json
import re
import sys

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

SKIP_PREFIX = ("<system-reminder>", "Current runtime context.")


def load_texts(jsonl_path, start_key):
    started, texts, images = False, [], []
    with open(jsonl_path, "rb") as f:
        for raw in f:
            raw = raw.strip()
            if not raw:
                continue
            ev = json.loads(raw.decode("utf-8"))
            if ev.get("type") != "user/message":
                continue
            items = (ev.get("data") or {}).get("content") or []
            t = [it.get("text") or "" for it in items if it.get("type") == "text"]
            text = "\n".join(t)
            if not text.strip() or text.startswith(SKIP_PREFIX):
                continue
            if not started:
                if start_key not in text:
                    continue
                started = True
            text = re.sub(r"^(【截图】\n)+", "", text)
            texts.append(text)
            images.append(sum(1 for it in items if it.get("type") == "image"))
    return texts, images


def main():
    jsonl, doc_path, start_key = sys.argv[1], sys.argv[2], sys.argv[3]
    texts, images = load_texts(jsonl, start_key)

    doc = open(doc_path, encoding="utf-8", newline="").read()
    part1 = doc.split("## 第二部分")[0]
    blocks = re.findall(r"(?m)^(`{3,})\n(.*?)^\1$", part1, re.S)

    print(f"原话 {len(texts)} 条 / 文档代码块 {len(blocks)} 个")
    ok = True
    if len(texts) != len(blocks):
        ok = False
        print("!! 数量不一致")

    for i, want in enumerate(texts, 1):
        if i > len(blocks):
            break
        got = blocks[i - 1][1]
        if got == want or got == want + "\n":
            continue
        ok = False
        print(f"!! 第 {i} 条不一致：doc={len(got)}B vs json={len(want)}B")
        n = min(len(got), len(want))
        for j in range(n):
            if got[j] != want[j]:
                print("   首个差异 @", j, repr(got[max(0, j - 40) : j + 40]))
                print("   json     @", j, repr(want[max(0, j - 40) : j + 40]))
                break
        else:
            print("   前缀一致，长度不同")

    # [截图] 占位数量核对
    sections = re.findall(r"(?s)### \d+\..*?(?=\n### |\Z)", part1)
    ph = [len(re.findall(r"\[截图\]", s)) for s in sections]
    print("占位数：", ph)
    print("截图数：", images)
    if ph[: len(images)] != images:
        ok = False
        print("!! [截图] 占位与截图数量不一致")

    print("RESULT:", "PASS" if ok else "FAIL")


if __name__ == "__main__":
    main()
