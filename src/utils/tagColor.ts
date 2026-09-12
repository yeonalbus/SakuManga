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

const GROUP_MAP: Record<string, XpGroupKey> = {
  female: 'core',
  male: 'core',
  mixed: 'core',
  other: 'core',
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
