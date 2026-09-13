/**
 * Round38：书架 → 本地阅读清单增量导入
 *
 * 背景：原先只能进「阅读清单」页点「➕ 从书架导入」再选书架；本 composable 把同一动作
 * 前移到书架墙卡片与书架页头部，形成「整理好 → 一键入候读队列」的闭环。
 *
 * 语义与阅读清单页现有导入**完全一致**（用户 2026-09-13 确认）：
 * - 按书架展示顺序（LexoRank 权值序）遍历；
 * - 增量追加到清单末尾，已在清单中的跳过；
 * - 仅影响清单本身，不触碰书架与本地库。
 */
import type { Bookshelf } from '@/types/comic'
import { offlineComics } from '@/stores/comicStore'
import { addToReadingList, offlineReadingList } from '@/stores/readingStore'
import { orderedShelfComicIds } from '@/stores/bookshelfStore'

export interface ShelfImportResult {
  /** 实际追加进清单的本子数 */
  added: number
  /** 跳过的本子数（已在清单中 / 本地库无匹配） */
  skipped: number
  /** 架内仍在本地库的有效本子数（added + 「已在清单中」的数量） */
  total: number
}

/**
 * 把整架作品增量导入本地阅读清单。
 * 返回值供调用方给出「新增 N 本 / 跳过 M 本 / 无新增」的精确提示。
 */
export const importShelfToReadingList = (shelf: Bookshelf | undefined): ShelfImportResult => {
  const result: ShelfImportResult = { added: 0, skipped: 0, total: 0 }
  if (!shelf) return result

  const byId = new Map(offlineComics.value.map((c) => [c.id, c]))
  const existing = new Set(offlineReadingList.value.map((c) => c.id))

  for (const cid of orderedShelfComicIds(shelf)) {
    const comic = byId.get(cid)
    if (!comic) {
      result.skipped++ // 失效引用 / 本地库无匹配
      continue
    }
    result.total++
    if (existing.has(cid)) {
      result.skipped++
      continue
    }
    addToReadingList(comic) // store 内部 200ms 防抖合并为一次后端写入
    existing.add(cid)
    result.added++
  }
  return result
}
