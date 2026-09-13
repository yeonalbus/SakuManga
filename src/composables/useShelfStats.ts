/**
 * Round38：书架统计口径（未读仪表盘共用）
 *
 * 未读口径与后端抽卡「排除读过的」（read_count <= 0）完全一致。
 * 覆盖面：书架墙统计条与卡片徽标、侧栏置顶书架徽标、书架页头部入口。
 *
 * 注：Round38 初版此文件还提供「抽一本未读」的抽取函数（useShelfPick.ts），
 * 后按用户需求改为「导入阅读清单」（见 useShelfImport.ts），抽取逻辑已移除。
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
