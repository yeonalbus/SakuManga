/**
 * 书签备注的输入限制（Round49 修正 v2）
 *
 * 规则（与名片显示区一致）：
 *   - 总宽度：110 个半角单位 = 55 个中文字符（1 个中文字符 = 2 个半角字符宽度）
 *   - 显示行数：最多 3 行（按「每行 19 个中文字符 = 38 半角单位」折算，
 *     含浏览器自动折行；显式换行与自动折行一视同仁）
 *
 * 为什么不再按「显式行」卡每行 19 字：textarea 会自动软换行，
 * 用户没按回车时文本里根本没有 \n，按显式行校验会把一整段误报成「每行超限」。
 * 正确判据是「折行后总行数 ≤ 3」：
 *   每段占行数 = ceil(该段宽度 / 38)，空段也算 1 行，各段求和。
 *
 * 超限一律「报错并拒绝保存」，不做静默截断（用户明确要求）。
 */

/** 备注上限：55 个中文字符 */
export const NOTE_MAX_CN_CHARS = 55
/** 备注上限（半角宽度单位）= 110 */
export const NOTE_MAX_WIDTH = NOTE_MAX_CN_CHARS * 2
/** 最多显示行数 */
export const NOTE_MAX_LINES = 3
/** 每行容量：19 个中文字符 */
export const NOTE_MAX_LINE_CN_CHARS = 19
/** 每行容量（半角宽度单位）= 38 */
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
  /** 显式行数（按 \n 切分） */
  lines: number
  /** 折算后的显示行数（含自动折行） */
  displayLines: number
  overWidth: boolean
  overLines: boolean
  ok: boolean
  /** 超限原因（ok 时为空串） */
  error: string
}

/** 校验备注文本 */
export const validateNote = (text: string): NoteValidation => {
  const segments = (text ?? '').split('\n')
  let width = 0
  let displayLines = 0
  for (const seg of segments) {
    let segWidth = 0
    for (const ch of seg) {
      const w = charDisplayWidth(ch)
      segWidth += w
      width += w
    }
    // 每段按「每行 38 半角」折算显示行数；空段也占 1 行
    displayLines += Math.max(1, Math.ceil(segWidth / NOTE_MAX_LINE_WIDTH))
  }
  const overWidth = width > NOTE_MAX_WIDTH
  const overLines = displayLines > NOTE_MAX_LINES
  const ok = !overWidth && !overLines
  let error = ''
  if (overLines) {
    error = `最多 ${NOTE_MAX_LINES} 行（含自动折行）`
  } else if (overWidth) {
    error = '输入超出限制'
  }
  return {
    width,
    cnChars: Math.round((width / 2) * 10) / 10,
    lines: segments.length,
    displayLines,
    overWidth,
    overLines,
    ok,
    error,
  }
}

/** 计数展示文本：半角单位宽度 / 上限（如 `62/110`） */
export const noteCounterText = (v: NoteValidation): string => `${v.width}/${NOTE_MAX_WIDTH}`
