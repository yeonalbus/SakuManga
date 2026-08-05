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
    const items = data.items || []
    if (source === 'online') onlineReadingList.value = items
    else offlineReadingList.value = items
  } catch (e) {
    console.error('加载阅读清单失败:', e)
  }
}

/** 把指定来源的整个队列保存到后端（幂等整体覆盖） */
const saveReadingList = (source: 'online' | 'offline') => {
  const list = source === 'online' ? onlineReadingList.value : offlineReadingList.value
  http('/reading-list', {
    method: 'PUT',
    body: JSON.stringify({ source, items: list }),
  }).catch((e) => console.error('保存阅读清单失败:', e))
}

/** 统一添加/移除阅读清单：已存在则移除，否则加入 */
export const toggleReadingList = (comic: ComicItem) => {
  const source = comic.source === 'online' ? 'online' : 'offline'
  const targetList = source === 'online' ? onlineReadingList : offlineReadingList
  const index = targetList.value.findIndex((item) => item.id === comic.id)

  if (index >= 0) {
    targetList.value.splice(index, 1)
  } else {
    targetList.value.push(comic)
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
    return list[0]
  }

  if (currentIndex >= list.length - 1) {
    return null
  }

  return list[currentIndex + 1]
}
