/**
 * 搜刮书签「作用域」判定（Round36）
 *
 * 语义（与用户对齐）：书签是「某次浏览的位置快照」，它的标记/高亮只在
 * **创建时那份搜索&筛选条件（config 快照）等价**的前提下显示——
 * 首页条件下标记的书签，不会因为你换了关键词/改了筛选项而在别的列表里重新高亮；
 * 同一张画廊若在另一个条件下也被标记过（另一条书签），则由那条书签在自己的作用域内生效。
 *
 * 口径（可选 A：严格快照等价）：
 *   纳入：keyword、keywords（含 `- ` 负向项）、activeCategories、minRating、language、
 *         onlyRemoved、onlyTorrents、disableLangFilter、disableUploaderFilter、disableTagFilter、
 *         excludeTags、excludeKeywords
 *   排除：minPages / maxPages / onlyDownloaded（离线专用字段，在线不生效；
 *         纳入会导致「改离线设置 → 在线书签标记消失」的怪现象）
 *   归一化：trim、空串归位、数组去重排序（顺序不影响检索结果，避免同一组条件被判成不同作用域）、
 *          「全部分类」与「空分类」折叠为同一形态（后端 CalculateFCats 对二者都返回 0 = 不排除任何分类）。
 */
import type { SearchConfig } from '@/types/comic'

/**
 * E 站全部分类（与 `searchStore.createDefaultSearchConfig` / `FilterDrawer.categories` 保持一致）。
 * 这里刻意不 import searchStore：该模块顶层会读 localStorage 并注册 watch，
 * 被工具函数引入会拖入副作用，也会让 node 侧单测无法运行。
 */
const ALL_CATEGORY_SET = new Set([
  'Doujinshi',
  'Manga',
  'Artist CG',
  'Game CG',
  'Image Set',
  'Cosplay',
  'Non-H',
  'Western',
  'Asian Porn',
  'Misc',
])

/** 归一化后的作用域（字段顺序固定，可直接序列化成指纹比较） */
export interface NormalizedScope {
  keyword: string
  keywords: string[]
  activeCategories: string[]
  minRating: number
  language: string
  onlyRemoved: boolean
  onlyTorrents: boolean
  disableLangFilter: boolean
  disableUploaderFilter: boolean
  disableTagFilter: boolean
  excludeTags: string[]
  excludeKeywords: string[]
}

/** 字符串数组归一化：trim → 去空 → 去重 → 排序 */
const normalizeList = (items?: string[] | null): string[] => {
  if (!Array.isArray(items)) return []
  const set = new Set<string>()
  for (const raw of items) {
    const s = (raw || '').trim()
    if (s) set.add(s)
  }
  return Array.from(set).sort()
}

/** 分类归一化：空集与「全选」等价（后端两者都视作不排除任何分类）→ 统一折叠为空数组 */
const normalizeCategories = (items?: string[] | null): string[] => {
  const list = normalizeList(items)
  if (list.length === 0) return []
  if (list.length === ALL_CATEGORY_SET.size && list.every((c) => ALL_CATEGORY_SET.has(c))) return []
  return list
}

/** 把任意来源的搜索配置（当前生效配置 / 书签 config 快照）归一化为可比较形态 */
export const normalizeScope = (cfg?: Partial<SearchConfig> | null): NormalizedScope => {
  const c = (cfg || {}) as Partial<SearchConfig>
  return {
    keyword: (c.keyword || '').trim(),
    keywords: normalizeList(c.keywords),
    activeCategories: normalizeCategories(c.activeCategories),
    minRating: typeof c.minRating === 'number' && Number.isFinite(c.minRating) ? c.minRating : 0,
    language: (c.language || '').trim() || 'All',
    onlyRemoved: !!c.onlyRemoved,
    onlyTorrents: !!c.onlyTorrents,
    disableLangFilter: !!c.disableLangFilter,
    disableUploaderFilter: !!c.disableUploaderFilter,
    disableTagFilter: !!c.disableTagFilter,
    excludeTags: normalizeList(c.excludeTags),
    excludeKeywords: normalizeList(c.excludeKeywords),
  }
}

/** 作用域指纹（归一化后序列化；字段顺序固定，可直接用作 Set key / 相等比较） */
export const scopeKey = (cfg?: Partial<SearchConfig> | null): string =>
  JSON.stringify(normalizeScope(cfg))

/**
 * 两份搜索&筛选配置是否属于同一作用域。
 * 用于「书签标记只能在当时的条件下显示」的判定。
 */
export const isSameOnlineScope = (
  a?: Partial<SearchConfig> | null,
  b?: Partial<SearchConfig> | null,
): boolean => scopeKey(a) === scopeKey(b)
