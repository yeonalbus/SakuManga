# -*- coding: utf-8 -*-
"""
Round49 对话落档提取脚本

- 输入：G:/SakuProjects/SakuManga/DSPlans/.tmp-session.jsonl（UTF-8，每行一个 JSON 事件）
- 起点：第一个包含「首先是优化这个输入框」的 user/message 事件
- 终点：文件最后一行
- 输出：G:/SakuProjects/SakuManga/DSPlans/DshChats/round49-bookmark-ui-iteration.md

用户原话直接取自 user/message 的 data.content（type=text 的 text 字段），
逐字节写入围栏代码块；type=image 的项以 [截图] 占位（放在代码块外，避免污染原文）。
agent/inbox/spliced 等重复事件一律忽略。
"""
import json
import os
import sys

SRC = r'G:\SakuProjects\SakuManga\DSPlans\.tmp-session.jsonl'
DST_DIR = r'G:\SakuProjects\SakuManga\DSPlans\DshChats'
DST = os.path.join(DST_DIR, 'round49-bookmark-ui-iteration.md')
PART2 = os.path.join(DST_DIR, '_part2.md')
KEY = '首先是优化这个输入框'

# 每条用户消息的主题标题（顺序与区间内 user/message 出现顺序一致）
TITLES = [
    '备注输入框优化（大矩形框 / 显示上限 / 超限报错 / 3 行 × 19 字）',
    '计数口径：当前宽度/110（半角单位）',
    '统一边栏样式 + 检查更新下移',
    '书签条目不要和在线模式隔开 + 子项辨识度（先谈方案）',
    '缩进 32px / 徽章加回 / 检查更新改颜色框并显示数量 + 备注校验误报',
    '开工吧',
    '名称与显示优化（先要 HTML 样板）',
    '样板 5 点反馈 + 三类书签的副标题规则',
    '红点换金框 + 置顶真置顶 + 四区固定高度',
    '5 点全认可，落实',
    '翻译代码项目里已有 / 迁移方式由 AI 决定',
    '标记改三项 chip / artist 豁免 / 显示超框 / 封面丢失',
    '5 点确认 + 封面问题补充描述',
    '动工',
    '没看到三个 chip',
    '收尾：本段落档指令',
    '让子代理检索本地对话文件',
]

HEADER = '''# 书签栏位 UI 迭代 · 对话落档（Round49）

> 用途：记录「未正式落档、通过对话直接驱动 AI 实现」的迭代过程，供后续 AI 与珱垣回看。
> 第一部分：珱垣原话，逐字复制，**禁止任何改动**（保留错别字、口语、标点、换行）。
> 第二部分：过程记录（方案 / 决策 / 实现 / 踩坑），由 AI 整理。

## 第一部分：用户原话（逐字）
'''


def fence_for(text: str) -> str:
    """选取比正文中最长反引号串更长的围栏，保证正文不被截断。"""
    longest = 0
    run = 0
    for ch in text:
        if ch == '`':
            run += 1
            longest = max(longest, run)
        else:
            run = 0
    return '`' * max(3, longest + 1)


def load_user_messages():
    """返回 [(行号, [('text', str) | ('image', None), ...]), ...]，从起点事件到最后一行。"""
    started = False
    out = []
    with open(SRC, 'rb') as f:
        for line_no, raw in enumerate(f, 1):
            raw = raw.strip()
            if not raw:
                continue
            ev = json.loads(raw.decode('utf-8'))
            if ev.get('type') != 'user/message':
                continue
            data = ev.get('data') or {}
            items = []
            for it in (data.get('content') or []):
                kind = it.get('type')
                if kind == 'text':
                    items.append(('text', it.get('text') or ''))
                elif kind == 'image':
                    items.append(('image', None))
            texts = [t for k, t in items if k == 'text']
            if not started:
                if not any(KEY in t for t in texts):
                    continue
                started = True
            out.append((line_no, items))
    return out


def build_part1(messages):
    blocks = []
    for idx, (line_no, items) in enumerate(messages, 1):
        imgs = sum(1 for k, _ in items if k == 'image')
        text = '\n'.join(t for k, t in items if k == 'text')
        title = TITLES[idx - 1] if idx - 1 < len(TITLES) else '（未命名）'
        parts = [f'### {idx}. {title}', '']
        if imgs:
            parts.append(' '.join(['[截图]'] * imgs))
            parts.append('')
        fence = fence_for(text)
        body = text if text.endswith('\n') else text + '\n'
        parts.append(fence + '\n' + body + fence)
        blocks.append('\n'.join(parts))
    return '\n\n'.join(blocks)


def main():
    messages = load_user_messages()
    if not messages:
        raise SystemExit('未找到起点事件')
    if len(messages) != len(TITLES):
        raise SystemExit(f'用户消息数 {len(messages)} 与标题数 {len(TITLES)} 不一致，请先核对 TITLES')
    print(f'用户消息 {len(messages)} 条，行号 {messages[0][0]} → {messages[-1][0]}')

    os.makedirs(DST_DIR, exist_ok=True)
    with open(PART2, encoding='utf-8') as f:
        part2 = f.read().rstrip('\n')

    doc = HEADER.rstrip('\n') + '\n\n' + build_part1(messages) + '\n\n' + part2 + '\n'
    with open(DST, 'w', encoding='utf-8', newline='\n') as f:
        f.write(doc)
    print(f'已写入 {DST}（{doc.count(chr(10))} 行）')


if __name__ == '__main__':
    main()
