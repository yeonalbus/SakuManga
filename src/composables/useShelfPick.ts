/**
 * Round38：书架消费入口（未读统计 + 抽一本未读）
 *
 * 背景：书架此前只有「整理」没有「消费」——22 个书架里 114 本收藏有 76 本从未打开。
 * 本 composable 提供书架墙/侧栏/书架页共用的未读口径（readCount <= 0）与随机抽取动作，
 * 让「打开书架」直接接上「开始阅读」。
 *
 * 未读口径与后端抽卡「排除读过的」（read_count <= 0）完全一致。
 */
import { computed } from 'vue'
import type { Bookshelf, OfflineComic } from '@/types/comic'
import { offlineComics } from '@/stores/comicStore'
import { bookshelves, orderedShelfComicIds } from '@/stores/bookshelfStore'

/** id → 离线本子索引（书架墙/侧栏/书架页共用，避免各处重复建 Map） */
const offlineById = computed(() => new Map(offlineComics.value.map((c) => [c.id, c])))

/** 架内仍存在的本子（按书架展示顺序），失效引用（本子已删除）被剔除 */
export const shelfComicsOf = (shelf: Bookshelf | undefined): OfflineComic[] => {
  if (!shelf) return []
  const byId = offlineById.value
  const out: OfflineComic[] = []
  for (const id of orderedShelfComicIds(shelf)) {
    const c = byId.get(id)
    if (c) out.push(c)
  }
  return out
}

/**
 * 架内未读数：**后端聚合值优先**（/bookshelves 的 unreadCount，不依赖本页是否加载全库），
 * 后端字段缺失（旧后端）时回退前端本地计算，保证降级可用。
 */
export const shelfUnreadCount = (shelf: Bookshelf | undefined): number => {
  if (!shelf) return 0
  if (typeof shelf.unreadCount === 'number') return shelf.unreadCount
  return shelfComicsOf(shelf).filter((c) => (c.readCount || 0) <= 0).length
}

/** 架内未读本子（抽一本未读的候选池） */
export const shelfUnreadComics = (shelf: Bookshelf | undefined): OfflineComic[] =>
  shelfComicsOf(shelf).filter((c) => (c.readCount || 0) <= 0)

/** 书架墙统计条：书架数 / 收藏本数（去重后仍存在的）/ 未读总数（去重，避免同一本跨架重复计入） */
export const shelfWallSummary = computed(() => {
  const collected = new Set<string>()
  for (const shelf of bookshelves.value) {
    for (const id of shelf.comicIds || []) collected.add(id)
  }
  const byId = offlineById.value
  let alive = 0
  let unread = 0
  for (const id of collected) {
    const c = byId.get(id)
    if (!c) continue // 失效引用不计入统计
    alive++
    if ((c.readCount || 0) <= 0) unread++
  }
  return { shelfCount: bookshelves.value.length, collected: alive, unread }
})

/** 从候选池随机取一本（excludeId 用于「再抽一张」不重复同一本） */
const randomPick = (pool: OfflineComic[], excludeId?: string): OfflineComic | null => {
  if (pool.length === 0) return null
  let candidates = pool
  if (excludeId && pool.length > 1) {
    const filtered = pool.filter((c) => c.id !== excludeId)
    if (filtered.length > 0) candidates = filtered
  }
  return candidates[Math.floor(Math.random() * candidates.length)]
}

/** 从指定书架抽一本未读；返回 null = 该架已清空（全部读完或本子已失效） */
export const pickUnreadFromShelf = (shelfId: string, excludeId?: string): OfflineComic | null => {
  const shelf = bookshelves.value.find((b) => b.id === shelfId)
  return randomPick(shelfUnreadComics(shelf), excludeId)
}

/** 跨全部书架抽一本未读（同一本只出现一次）；返回 null = 全线清空 */
export const pickUnreadFromAllShelves = (excludeId?: string): OfflineComic | null => {
  const seen = new Set<string>()
  const pool: OfflineComic[] = []
  for (const shelf of bookshelves.value) {
    for (const c of shelfUnreadComics(shelf)) {
      if (seen.has(c.id)) continue
      seen.add(c.id)
      pool.push(c)
    }
  }
  return randomPick(pool, excludeId)
}
