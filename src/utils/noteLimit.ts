/**
 * 书签备注的输入限制（Round49 修正）
 *
 * 规则（与卡片显示区一致，用户实测）：
 *   - 总宽度：110 个半角单位 = 55 个中文字符（1 个中文字符 = 2 个半角字符宽度）
 *   - 行数：最多 3 行
 *   - 每行：最多 19 个中文字符（38 半角单位）——对应当前名片宽度下的一行容量
 *
 * 超限一律「报错并拒绝保存」，不做静默截断（用户明确要求）。
 */

/** 备注上限：55 个中文字符 */
export const NOTE_MAX_CN_CHARS = 55
/** 备注上限（半角宽度单位）= 110 */
export const NOTE_MAX_WIDTH = NOTE_MAX_CN_CHARS * 2
/** 最多行数 */
export const NOTE_MAX_LINES = 3
/** 每行上限：19 个中文字符 */
export const NOTE_MAX_LINE_CN_CHARS = 19
/** 每行上限（半角宽度单位）= 38 */
export const NOTE_MAX_LINE_WIDTH = NOTE_MAX_LINE_CN_CHARS * 2

/**
 * 字符显示宽度：中文 / 全角标点 / emoji 计 2，ASCII 与半角计 1。
 * 粗略按码点 >= 0x1100 判定全角（覆盖 CJK、全角符号区），对本场景足够。
 */
export const charDisplayWidth = (ch: string): number => {
  const cp = ch.codePointAt(0) ?? 0
  return cp >= 0x1100 ? 2 : 1
}

export interface NoteValidation {
  /** 总显示宽度（半角单位） */
  width: number
  /** 折算为中文字符数（width / 2，保留一位小数） */
  cnChars: number
  /** 行数 */
  lines: number
  overWidth: boolean
  overLines: boolean
  overLineWidth: boolean
  ok: boolean
  /** 超限原因（ok 时为空串） */
  error: string
}

/** 校验备注文本：超限原因优先按「行数 → 单行 → 总宽」顺序给出 */
export const validateNote = (text: string): NoteValidation => {
  const lines = (text ?? '').split('\n')
  let width = 0
  let overLineWidth = false
  for (const line of lines) {
    let lineWidth = 0
    for (const ch of line) {
      const w = charDisplayWidth(ch)
      lineWidth += w
      width += w
    }
    if (lineWidth > NOTE_MAX_LINE_WIDTH) overLineWidth = true
  }
  const overWidth = width > NOTE_MAX_WIDTH
  const overLines = lines.length > NOTE_MAX_LINES
  const ok = !overWidth && !overLines && !overLineWidth
  let error = ''
  if (overLines) {
    error = `最多 ${NOTE_MAX_LINES} 行`
  } else if (overLineWidth) {
    error = `每行最多 ${NOTE_MAX_LINE_CN_CHARS} 个中文字符`
  } else if (overWidth) {
    error = '输入超出限制'
  }
  return {
    width,
    cnChars: Math.round((width / 2) * 10) / 10,
    lines: lines.length,
    overWidth,
    overLines,
    overLineWidth,
    ok,
    error,
  }
}

/** 计数展示文本：半角单位宽度 / 上限（如 `62/110`） */
export const noteCounterText = (v: NoteValidation): string => `${v.width}/${NOTE_MAX_WIDTH}`
