# -*- coding: utf-8 -*-
"""把落档文档第一部分的围栏代码块内容，替换为会话 JSONL 里对应 user/message 的原话（逐字节）。

用法：python _inject.py <session.jsonl> <doc.md> <start_key>
说明：从 start_key 命中的第一条用户消息开始，按出现顺序与文档代码块一一对应；
     代码块内容 = 原话 text（含原始换行符），若原文不以换行结尾则补一个换行以便围栏闭合。
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
            # 截图占位行是提取脚本加的标记，不属于原话
            text = re.sub(r"^(【截图】\n)+", "", text)
            texts.append(text)
            images.append(sum(1 for it in items if it.get("type") == "image"))
    return texts, images


def main():
    jsonl, doc_path, start_key = sys.argv[1], sys.argv[2], sys.argv[3]
    texts, _ = load_texts(jsonl, start_key)

    doc = open(doc_path, encoding="utf-8", newline="").read()
    part1 = doc.split("## 第二部分")[0]
    blocks = list(re.finditer(r"(?m)^(`{3,})\n(.*?)^\1$", part1, re.S))
    print(f"原话 {len(texts)} 条 / 文档代码块 {len(blocks)} 个")
    if len(texts) != len(blocks):
        raise SystemExit("数量不一致，拒绝注入")

    # 倒序替换：前面的偏移不受影响
    for idx in range(len(blocks) - 1, -1, -1):
        want = texts[idx]
        if not want.endswith("\n"):
            want = want + "\n"
        m = blocks[idx]
        doc = doc[: m.start(2)] + want + doc[m.end(2) :]

    open(doc_path, "w", encoding="utf-8", newline="").write(doc)
    print("injected OK")


if __name__ == "__main__":
    main()
