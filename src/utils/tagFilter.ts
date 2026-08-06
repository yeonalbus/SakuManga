// src/utils/tagFilter.ts
// Round3-任务6：负向排除过滤工具
// 语义：`- ` 前缀统一表示"排除"。支持两类负向规则：
//   1. 负向 tag（excludeTags）：对 namespace:key 做精确匹配（如 "female:yuri"）
//   2. 负向关键词（excludeKeywords）：对 标题/日文标题/标签/上传者 做子串匹配（如 "3d"）
// 在线端为"抓取后本地丢弃"（E-Hentai 不支持服务端排除语法），离线端在过滤管道内剔除。
import type { ComicItem, OfflineComic, OnlineComic } from '@/types/comic'

/** 归一化 tag：小写 + 下划线/空格互换 + 去首尾空格，用于 namespace:key 精确匹配 */
const normalizeTag = (raw: string): string => {
  return (raw || '')
    .toLowerCase()
    .trim()
    .replace(/_/g, ' ')
}

/**
 * 收集作品的"原始 tag 集合"（namespace:key 形态，全部归一化）：
 * - 离线：优先取 tagRaws（已归一为小写、下划线→空格），并额外并入 tags 中含冒号的条目
 * - 在线：tags 本身即原始 namespace:key（如 "female:big breasts"），全部并入
 */
const collectRawTags = (comic: ComicItem): string[] => {
  const out: string[] = []
  const push = (arr?: string[]) => {
    if (Array.isArray(arr)) {
      for (const t of arr) {
        const s = normalizeTag(t)
        if (s) out.push(s)
      }
    }
  }
  push((comic as OfflineComic).tagRaws)
  if (Array.isArray(comic.tags)) {
    for (const t of comic.tags) {
      const s = normalizeTag(t)
      // 含冒号的视为原始 tag；不含冒号的是离线翻译名，仅作关键词匹配目标
      if (s.includes(':')) out.push(s)
    }
  }
  return Array.from(new Set(out))
}

/**
 * 收集作品用于"关键词子串匹配"的文本池（全部小写）：
 * 标题、日文标题、标签（原始 + 翻译名）、上传者
 */
const collectSearchTexts = (comic: ComicItem): string[] => {
  const texts: string[] = []
  const push = (s?: string) => {
    const t = (s || '').toLowerCase().trim()
    if (t) texts.push(t)
  }
  push(comic.title)
  if (comic.source === 'offline') push((comic as OfflineComic).titleJpn)
  if (Array.isArray(comic.tags)) {
    for (const t of comic.tags) push(t)
  }
  if (comic.source === 'online') push((comic as OnlineComic).uploader)
  return texts
}

/** 负向排除规则（由 SearchConfig / FilterParams / RandomComicParams 的 exclude* 字段提供） */
export interface ExcludeRule {
  excludeTags?: string[]
  excludeKeywords?: string[]
}

/**
 * 判断作品是否命中任一负向规则。
 * @returns true=保留（未命中任何排除项）；false=应被剔除
 */
export const matchExcludes = (comic: ComicItem, rule: ExcludeRule): boolean => {
  const tags = rule.excludeTags || []
  const keywords = rule.excludeKeywords || []
  if (tags.length === 0 && keywords.length === 0) return true

  // 负向 tag：原始 tag 集合精确匹配
  if (tags.length > 0) {
    const rawTags = collectRawTags(comic)
    for (const et of tags) {
      const norm = normalizeTag(et)
      if (norm && rawTags.includes(norm)) return false
    }
  }

  // 负向关键词：文本池子串匹配
  if (keywords.length > 0) {
    const pool = collectSearchTexts(comic).join(' ')
    for (const ek of keywords) {
      const kw = (ek || '').toLowerCase().trim()
      if (kw && pool.includes(kw)) return false
    }
  }

  return true
}

/** 解析结果：正向关键词 / 负向 tag / 负向关键词 三队列 */
export interface ParsedKeywordQueue {
  positive: string[]
  excludeTags: string[]
  excludeKeywords: string[]
}

/**
 * 把关键词队列按 `- ` 前缀拆分为正向 / 负向两部分：
 * - `- female:yuri` → excludeTags（含冒号视为 tag）
 * - `- 3d` → excludeKeywords
 * - 其余 → positive
 */
export const parseKeywordQueue = (items: string[] | undefined): ParsedKeywordQueue => {
  const positive: string[] = []
  const excludeTags: string[] = []
  const excludeKeywords: string[] = []
  for (const raw of items || []) {
    const item = (raw || '').trim()
    if (!item) continue
    if (item.startsWith('-')) {
      const rest = item.replace(/^-\s*/, '').trim()
      if (!rest) continue
      if (rest.includes(':')) excludeTags.push(rest)
      else excludeKeywords.push(rest)
    } else {
      positive.push(item)
    }
  }
  return { positive, excludeTags, excludeKeywords }
}

/** 判断字符串是否为"负向排除项"（以 `- ` 开头） */
export const isNegativeItem = (raw: string): boolean => {
  return /^-\s*\S/.test((raw || '').trim())
}
