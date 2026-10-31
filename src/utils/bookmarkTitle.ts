/**
 * 书签标题工具（Round49 v3）
 *
 * 主标题优先级：用户自定义名称 > 标准 tag 的翻译中文名 > 副标题内容
 * 副标题永远 = 位置字段 + 非默认筛选（bookmarkDisplayTitle）
 *
 * 「标准 tag」判定：检索词形如 `namespace:"key"` / `namespace:key`
 * （如 `artist:"fan no hitori$"`、`female:"big breasts"`、`language:chinese`）。
 * 关键词搜索（无 namespace 前缀）不查翻译，直接回退。
 *
 * 翻译数据源：前端标签词典（useTagStore.translate，来自 /tags/dictionary）。
 */
import type { ScrapeBookmark } from '@/types/comic'
import { bookmarkDisplayTitle } from '@/stores/scrapeBookmarksStore'

/** 归一化 tag key：去首尾引号、去尾部 `$`（前缀匹配符）、下划线归一为空格 */
const normalizeTagKey = (raw: string): string =>
  raw
    .replace(/^["']+/, '')
    .replace(/["']+$/, '')
    .replace(/\$+$/, '')
    .replace(/_/g, ' ')
    .trim()

/**
 * 从书签检索条件中提取第一个标准 tag，返回 `namespace:key`（小写 namespace）。
 * 找不到（关键词搜索 / 空）返回空串。
 */
export const extractBookmarkTagKey = (
  bm: Pick<ScrapeBookmark, 'keyword' | 'config'>,
): string => {
  const candidates: string[] = []
  const kw = (bm.keyword || bm.config?.keyword || '').trim()
  if (kw) candidates.push(kw)
  if (Array.isArray(bm.config?.keywords)) candidates.push(...bm.config.keywords)

  for (const raw of candidates) {
    const s = (raw || '').trim()
    if (!s) continue
    // 带引号：artist:"fan no hitori$"
    const quoted = s.match(/([A-Za-z_]+)\s*:\s*"([^"]*)"/)
    if (quoted) {
      const key = normalizeTagKey(quoted[2])
      if (key) return `${quoted[1].toLowerCase()}:${key}`
      continue
    }
    // 不带引号：language:chinese / artist:some_artist
    const plain = s.match(/([A-Za-z_]+)\s*:\s*(\S+)/)
    if (plain) {
      const key = normalizeTagKey(plain[2])
      if (key) return `${plain[1].toLowerCase()}:${key}`
    }
  }
  return ''
}

/** 翻译查询函数签名（与 useTagStore.translate 一致，便于纯函数测试） */
export type TagTranslateFn = (raw: string) => { name: string; key: string }

/**
 * 无翻译时直接显示 tag key 本身的命名空间。
 *
 * - artist：画师名的"正确显示"经常就是原名（英文 ID / 日文名 / 中文昵称都可能）；
 * - uploader：E 站上传者名，词典里没有该命名空间，且 `formatFSearchTag` 生成的是
 *   `uploader:"Jean Lenegre$"` / `uploader:someuser$` 形式，同样不应退化成
 *   `搜索: uploader:"Jean Lenegre$"` 长串。
 */
const KEY_SELF_DISPLAY_NAMESPACES = ['artist', 'uploader']

/**
 * 书签主标题：
 *   自定义名称 > 标准 tag 翻译名 > (artist / uploader) tag key 本身 > 副标题内容
 */
export const bookmarkTitleWith = (
  bm: Pick<ScrapeBookmark, 'name' | 'keyword' | 'config' | 'type'>,
  translate: TagTranslateFn,
): string => {
  const custom = (bm.name || '').trim()
  if (custom) return custom
  const tag = extractBookmarkTagKey(bm)
  if (tag) {
    const item = translate(tag)
    if (item.name && item.name !== item.key) return item.name
    // artist / uploader 类无翻译：显示名字本身（如 jndfh、Jean Lenegre）
    const ns = tag.slice(0, tag.indexOf(':'))
    if (KEY_SELF_DISPLAY_NAMESPACES.includes(ns)) return tag.slice(tag.indexOf(':') + 1)
  }
  return bookmarkDisplayTitle(bm)
}
