/**
 * 搜刮书签 Store（Round27）
 *
 * 管理「搜刮书签」列表：某次在线浏览的位置快照（搜索词 + 筛选状态 + 锚定卡片）。
 * 持久化到 localStorage（仿 searchStore 模式：初始化恢复 + watch 自动落盘，
 * 落盘走 saveStorage 的 aggressive 配额回收级联，配额紧张也能写入）。
 */
import { ref, computed, watch } from 'vue'
import type { ScrapeBookmark, SearchConfig } from '@/types/comic'
import { onlineSearchConfig } from '@/stores/searchStore'
import { loadStorage, saveStorage } from '@/utils/storage'

const STORAGE_KEY = 'saku_scrape_bookmarks'

// ─── 数据校验与恢复（localStorage 数据损坏 / 版本不兼容时丢弃异常项）───
const isValidAnchor = (a: unknown): boolean =>
  !!a && typeof a === 'object' && typeof (a as { gid?: unknown }).gid === 'string'

const isValidBookmark = (b: unknown): b is ScrapeBookmark => {
  if (!b || typeof b !== 'object') return false
  const r = b as Record<string, unknown>
  return (
    typeof r.id === 'string' &&
    typeof r.name === 'string' &&
    (r.type === 'home' || r.type === 'search') &&
    typeof r.keyword === 'string' &&
    typeof r.createdAt === 'number' &&
    !!r.config && typeof r.config === 'object' &&
    (r.anchor === null || isValidAnchor(r.anchor))
  )
}

const restoreBookmarks = (raw: unknown): ScrapeBookmark[] =>
  Array.isArray(raw) ? raw.filter(isValidBookmark) : []

/** 书签列表（响应式，变更自动落盘） */
export const scrapeBookmarks = ref<ScrapeBookmark[]>(
  restoreBookmarks(loadStorage(STORAGE_KEY, null)),
)

watch(
  scrapeBookmarks,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

// ─── 派生状态 ───

/** 全部书签锚定的 gid 集合（供 ItemCard 常驻角标判定） */
export const bookmarkedGids = computed<Set<string>>(() => {
  const set = new Set<string>()
  for (const bm of scrapeBookmarks.value) {
    if (bm.anchor?.gid) set.add(bm.anchor.gid)
  }
  return set
})

/** 某 gid 是否被任一书签锚定 */
export const isGidBookmarked = (gid: string): boolean => bookmarkedGids.value.has(gid)

/** 按 id 取书签（跳转恢复用；不存在返回 undefined） */
export const getBookmarkById = (id: string): ScrapeBookmark | undefined =>
  scrapeBookmarks.value.find((b) => b.id === id)

// ─── 动作 ───

/** 生成唯一 id（规避 crypto.randomUUID 的兼容性/安全上下文限制） */
const genBookmarkId = (): string =>
  `bm_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`

/** 深拷贝 SearchConfig（纯 JSON 数据，JSON 往返安全）。快照与恢复共用同一实现 */
export const cloneSearchConfig = (cfg: SearchConfig): SearchConfig =>
  JSON.parse(JSON.stringify(cfg)) as SearchConfig

/**
 * 快照当前在线搜索/筛选状态（创建书签时调用）。
 * 注意：书签恢复整体覆盖 onlineSearchConfig，不走「搜索选项继承」，
 * 因此这里深拷贝全量字段，保证恢复的是创建时的完整现场。
 */
export const snapshotOnlineSearchConfig = (): SearchConfig =>
  cloneSearchConfig(onlineSearchConfig.value)

/** 新增书签（name 为空时按类型生成默认名） */
export const addScrapeBookmark = (
  name: string,
  type: 'home' | 'search',
  keyword: string,
  config: SearchConfig,
  anchor: ScrapeBookmark['anchor'],
): ScrapeBookmark => {
  const trimmed = name.trim()
  const bookmark: ScrapeBookmark = {
    id: genBookmarkId(),
    name:
      trimmed ||
      (type === 'search'
        ? `搜索: ${keyword.trim() || '未命名'}`
        : `首页快照 ${new Date().toLocaleTimeString('zh-CN', {
            hour: '2-digit',
            minute: '2-digit',
          })}`),
    type,
    keyword,
    config: cloneSearchConfig(config),
    anchor,
    createdAt: Date.now(),
  }
  scrapeBookmarks.value.push(bookmark)
  return bookmark
}

/** 删除书签 */
export const removeScrapeBookmark = (id: string): void => {
  scrapeBookmarks.value = scrapeBookmarks.value.filter((b) => b.id !== id)
}

/** 重命名书签（空名忽略） */
export const renameScrapeBookmark = (id: string, name: string): void => {
  const trimmed = name.trim()
  if (!trimmed) return
  const bm = scrapeBookmarks.value.find((b) => b.id === id)
  if (bm) bm.name = trimmed
}
