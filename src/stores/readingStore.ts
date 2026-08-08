/**
 * 阅读清单（队列）Store：在线/离线两个队列的增删、清空与下一本调度
 * 持久化迁移到后端 /reading-list API（按登录用户隔离），本地仅保留内存态。
 */
import { ref } from 'vue'
import type { ComicItem } from '@/types/comic'
import { http } from '@/utils/request'

/** 在线阅读清单 */
export const onlineReadingList = ref<ComicItem[]>([])
/** 离线阅读清单 */
export const offlineReadingList = ref<ComicItem[]>([])

/** 从后端加载指定来源的阅读清单 */
export const loadReadingList = async (source: 'online' | 'offline') => {
  try {
    const data = await http<{ source: string; items: ComicItem[] }>(
      `/reading-list?source=${source}`,
    )
    // bug3：强制归一化每条记录的 source 与所在清单一致，避免历史遗留快照中
    // source 缺失/错误，导致阅读清单「立即阅读」时把在线 gid 误判为离线模式。
    const items = (data.items || []).map((it) => ({ ...it, source }) as ComicItem)
    if (source === 'online') onlineReadingList.value = items
    else offlineReadingList.value = items
  } catch (e) {
    console.error('加载阅读清单失败:', e)
  }
}

/** 阅读清单后端同步（按 source）：防抖 200ms 合并连续操作 + 串行化保证 PUT 不乱序覆盖 */
const SYNC_DEBOUNCE_MS = 200
const syncTimers: Record<string, ReturnType<typeof setTimeout> | null> = {}
/** 每 source 一条在途写入链，后一次写排队在前一次完成后，避免快速增删时后端收到乱序请求 */
const syncChains: Record<string, Promise<unknown>> = {}

/** 把指定来源的整个队列保存到后端（幂等整体覆盖） */
const saveReadingList = (source: 'online' | 'offline') => {
  if (syncTimers[source]) clearTimeout(syncTimers[source])
  syncTimers[source] = setTimeout(() => {
    syncTimers[source] = null
    const list = source === 'online' ? onlineReadingList.value : offlineReadingList.value
    const body = JSON.stringify({ source, items: list })
    const chain = (syncChains[source] ?? Promise.resolve())
      .catch(() => {}) // 前一次写入失败不阻断后续写入
      .then(() =>
        http('/reading-list', {
          method: 'PUT',
          body,
        }),
      )
      .catch((e) => console.error('保存阅读清单失败:', e))
    syncChains[source] = chain
  }, SYNC_DEBOUNCE_MS)
}

/** 统一添加/移除阅读清单：已存在则移除，否则加入 */
export const toggleReadingList = (comic: ComicItem) => {
  const source = comic.source === 'online' ? 'online' : 'offline'
  const targetList = source === 'online' ? onlineReadingList : offlineReadingList
  const index = targetList.value.findIndex((item) => item.id === comic.id)

  if (index >= 0) {
    targetList.value.splice(index, 1)
  } else {
    // bug3：入队时强制写入 source，防止调用方传入的 comic 缺 source 字段，
    // 从而在后续「立即阅读」时被误判为离线模式（在线 gid 走离线接口 404）。
    targetList.value.push({ ...comic, source } as ComicItem)
  }
  saveReadingList(source)
}

/** 清空指定来源的阅读清单 */
export const clearReadingList = (source: 'online' | 'offline') => {
  if (source === 'online') onlineReadingList.value = []
  else offlineReadingList.value = []
  saveReadingList(source)
}

/** 核心调度：获取队列中指定作品的后一个作品（供阅读器连贯阅读） */
export const getNextComicInQueue = (
  currentId: string,
  source: 'online' | 'offline',
): ComicItem | null => {
  const list = source === 'online' ? onlineReadingList.value : offlineReadingList.value

  if (!list || list.length === 0) return null

  const currentIndex = list.findIndex((item) => item.id === currentId)

  if (currentIndex === -1) {
    // 当前作品不在阅读清单中：不应错误地续读清单首本，直接返回 null（问题9）
    return null
  }

  if (currentIndex >= list.length - 1) {
    return null
  }

  return list[currentIndex + 1]
}
