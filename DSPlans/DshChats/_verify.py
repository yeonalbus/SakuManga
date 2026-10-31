# -*- coding: utf-8 -*-
"""校验：落档文档第一部分每个代码块内容与 JSONL 对应 user/message 的 text 字段逐字节一致"""
import json
import os
import re
import sys
import io

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')

SRC = r'G:\SakuProjects\SakuManga\DSPlans\.tmp-session.jsonl'
DOC = r'G:\SakuProjects\SakuManga\DSPlans\DshChats\round49-bookmark-ui-iteration.md'
KEY = '首先是优化这个输入框'

# ── 取 JSONL 侧原话 ────────────────────────────────────────
started, texts, images = False, [], []
with open(SRC, 'rb') as f:
    for raw in f:
        raw = raw.strip()
        if not raw:
            continue
        ev = json.loads(raw.decode('utf-8'))
        if ev.get('type') != 'user/message':
            continue
        items = [it for it in ((ev.get('data') or {}).get('content') or [])]
        t = [it.get('text') or '' for it in items if it.get('type') == 'text']
        if not started:
            if not any(KEY in x for x in t):
                continue
            started = True
        texts.append('\n'.join(t))
        images.append(sum(1 for it in items if it.get('type') == 'image'))

# ── 取文档侧（第一部分围栏代码块）──────────────────────────
doc = open(DOC, encoding='utf-8', newline='').read()  # newline='' 禁止把 \r\n 归一化
part1 = doc.split('## 第二部分：过程记录')[0]
blocks = re.findall(r'(?m)^(`{3,})\n(.*?)^\1$', part1, re.S)

print(f'JSONL 用户消息 {len(texts)} 条 / 文档代码块 {len(blocks)} 个')
ok = True
if len(texts) != len(blocks):
    ok = False
    print('!! 数量不一致')

for i, (got_text, want_text) in enumerate(zip([b[1] for b in blocks], texts), 1):
    # 围栏代码块必须以换行结束才能闭合，允许「原文 + 一个结构换行」
    if got_text == want_text or got_text == want_text + '\n':
        continue
    ok = False
    print(f'!! 第 {i} 条不一致：doc={len(got_text)}B vs json={len(want_text)}B')
    # 找第一个不同位置
    n = min(len(got_text), len(want_text))
    for j in range(n):
        if got_text[j] != want_text[j]:
            print('   首个差异 @', j, repr(got_text[max(0, j - 40):j + 40]))
            print('   json     @', j, repr(want_text[max(0, j - 40):j + 40]))
            break
    else:
        print('   前缀一致，长度不同')

# [截图] 占位数量核对
ph = [len(re.findall(r'\[截图\]', seg)) for seg in re.findall(r'(?s)### \d+\..*?(?=\n### |\Z)', part1)]
print('占位数：', ph)
print('截图数：', images)
if ph[:len(images)] != images:
    ok = False
    print('!! [截图] 占位与截图数量不一致')

print('RESULT:', 'PASS' if ok else 'FAIL')
