/**
 * E-Hentai 命名空间配色与分组工具（TagChip 胶囊与 XP 词云共用）
 *
 * - tagBgColor：胶囊背景色板，沿用 TagChip 既有调色盘（抽取后视觉零变化）
 * - tagTextColor：词云文字色板，比背景色更亮，保证深色主题下的文字可读性
 * - xpGroupOf：命名空间分组，与后端 services.XpGroupOf 完全对齐（口径漂移防线）
 */

/** 命名空间 → 胶囊背景色（TagChip 用） */
const NAMESPACE_BG: Record<string, string> = {
  artist: '#c62828',
  group: '#ad1457',
  character: '#2e7d32',
  parody: '#00838f',
  female: '#ad1457',
  male: '#1565c0',
  reclass: '#4527a0',
  language: '#424242',
}

/** 命名空间 → 词云文字色（亮色板，深色背景下对比度优先） */
const NAMESPACE_TEXT: Record<string, string> = {
  female: '#ff7ab8',
  male: '#5aa9ff',
  mixed: '#c08cff',
  other: '#9aa4b2',
  character: '#66d17a',
  parody: '#35c9d6',
  artist: '#ff7b72',
  group: '#ff8fab',
  location: '#c9b458',
}

/** 胶囊背景色（未收录命名空间回退中性深灰） */
export const tagBgColor = (namespace: string): string =>
  NAMESPACE_BG[(namespace || '').toLowerCase()] || '#37474f'

/** 词云文字色（未收录命名空间回退中性灰） */
export const tagTextColor = (namespace: string): string =>
  NAMESPACE_TEXT[(namespace || '').toLowerCase()] || '#9aa4b2'

/** 词云命名空间分组（与后端 XpGroupOf 对齐） */
export type XpGroupKey = 'core' | 'ip' | 'artist' | 'misc' | 'skip'

/** 词云样式主题（跟随 App 的 <html data-theme>） */
export type XpTheme = 'dark' | 'light'

/**
 * 词云配色的命名空间主色（HSL），深色/浅色主题各一套。
 * 词云是「大面积彩色文字」：深色底需要高明度亮色，浅色底需要低明度深色，
 * 否则会出现白底上粉字几乎看不清的问题。
 */
const NAMESPACE_HUE_DARK: Record<string, { h: number; s: number; l: number }> = {
  female: { h: 335, s: 82, l: 68 },
  male: { h: 210, s: 82, l: 66 },
  mixed: { h: 275, s: 70, l: 72 },
  other: { h: 215, s: 14, l: 72 },
  character: { h: 142, s: 52, l: 62 },
  parody: { h: 187, s: 66, l: 60 },
  artist: { h: 8, s: 70, l: 64 },
  group: { h: 330, s: 66, l: 66 },
  location: { h: 45, s: 62, l: 62 },
}

const NAMESPACE_HUE_LIGHT: Record<string, { h: number; s: number; l: number }> = {
  female: { h: 332, s: 74, l: 42 },
  male: { h: 210, s: 80, l: 40 },
  mixed: { h: 272, s: 62, l: 44 },
  other: { h: 215, s: 14, l: 42 },
  character: { h: 142, s: 52, l: 31 },
  parody: { h: 189, s: 78, l: 30 },
  artist: { h: 6, s: 68, l: 42 },
  group: { h: 328, s: 62, l: 40 },
  location: { h: 35, s: 70, l: 34 },
}

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

/**
 * 词云文字色：命名空间主色 + 按权重（t = 1 最大词）调制明度/饱和/透明度，
 * 形成「大词突出、尾词淡化」的景深层次。
 *
 * 深色主题：大词更亮更饱和，尾词降透明度；
 * 浅色主题：大词更深更饱和（白底上对比度更高），尾词也略淡但保持可读。
 */
export const xpTagColor = (namespace: string, t = 1, theme: XpTheme = 'dark'): string => {
  const palette = theme === 'light' ? NAMESPACE_HUE_LIGHT : NAMESPACE_HUE_DARK
  const base = palette[(namespace || '').toLowerCase()] || palette.other
  const k = clamp(t, 0, 1)
  // 明度方向：深色底「越重要越亮」，浅色底「越重要越深」
  const lDir = theme === 'light' ? -1 : 1
  const l = clamp(
    base.l + lDir * (k - 0.55) * 18,
    theme === 'light' ? 16 : 34,
    theme === 'light' ? 60 : 86,
  )
  const s = clamp(base.s + (k - 0.55) * 24, 10, 96)
  const alpha =
    theme === 'light' ? 0.62 + 0.38 * Math.pow(k, 0.55) : 0.5 + 0.5 * Math.pow(k, 0.55)
  return `hsla(${base.h}, ${s}%, ${l}%, ${alpha.toFixed(2)})`
}

/** 读取当前 App 主题（`<html data-theme>`；缺省为深色） */
export const readXpTheme = (): XpTheme => {
  if (typeof document === 'undefined') return 'dark'
  return document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark'
}

const GROUP_MAP: Record<string, XpGroupKey> = {
  female: 'core',
  male: 'core',
  mixed: 'core',
  // Round32：other 归「其他」分组——它是标记/元信息命名空间（马赛克修正、无修正、全彩等），
  // 不反映体态/属性偏好；核心 XP 只保留 female / male / mixed。
  other: 'misc',
  character: 'ip',
  parody: 'ip',
  artist: 'artist',
  group: 'artist',
  language: 'skip',
  reclass: 'skip',
}

/** 命名空间 → 词云分组（未知命名空间归入 misc） */
export const xpGroupOf = (namespace: string): XpGroupKey =>
  GROUP_MAP[(namespace || '').toLowerCase()] || 'misc'
