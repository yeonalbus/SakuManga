/**
 * 按「显示宽度」截断文本（Round24）
 *
 * 背景：顶栏名称/章节路径可能过长，超出显示范围时需截断为 `...`。
 * 中西文混排时按「字符数」截断视觉不齐（中文/日文比英文宽），
 * 因此按显示宽度计算：全角/宽字符（中日韩、全角标点、emoji 等）按 2 宽，半角按 1 宽。
 */

/** 单个字符的显示宽度（1 或 2） */
export function charDisplayWidth(ch: string): number {
  const code = ch.codePointAt(0) ?? 0
  // 半角可见字符（ASCII 可打印区）按 1 宽；其余（CJK/全角标点/emoji 等）按 2 宽
  if (code >= 0x20 && code <= 0xff) return 1
  return 2
}

/** 字符串总显示宽度 */
export function displayWidth(str: string): number {
  let w = 0
  for (const ch of str) w += charDisplayWidth(ch)
  return w
}

/**
 * 按显示宽度截断：超出 maxWidth 的部分丢弃并追加 `...`。
 * 极端情况（首个字符就超宽）至少保留 1 个字符，保证有内容可读。
 */
export function truncateByWidth(str: string, maxWidth: number): string {
  if (!str) return ''
  if (maxWidth <= 0) return ''
  if (displayWidth(str) <= maxWidth) return str
  let w = 0
  let out = ''
  for (const ch of str) {
    const cw = charDisplayWidth(ch)
    if (w + cw > maxWidth) break
    w += cw
    out += ch
  }
  if (!out) out = Array.from(str)[0] ?? ''
  return out + '...'
}
