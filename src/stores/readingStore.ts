/**
 * 阅读清单（队列）Store：在线/离线两个队列的增删、清空与下一本调度
 * 由原 appStore 拆分而来
 */
import { ref, watch } from 'vue'
import type { ComicItem } from '@/types/comic'
import { loadStorage } from '@/utils/storage'

/** 在线阅读清单 */
export const onlineReadingList = ref<ComicItem[]>(loadStorage('app_online_reading_list', []))
/** 离线阅读清单 */
export const offlineReadingList = ref<ComicItem[]>(loadStorage('app_offline_reading_list', []))

watch(
  onlineReadingList,
  (val) => {
    localStorage.setItem('app_online_reading_list', JSON.stringify(val))
  },
  { deep: true },
)

watch(
  offlineReadingList,
  (val) => {
    localStorage.setItem('app_offline_reading_list', JSON.stringify(val))
  },
  { deep: true },
)

/** 统一添加/移除阅读清单：已存在则移除，否则加入 */
export const toggleReadingList = (comic: ComicItem) => {
  const targetList = comic.source === 'online' ? onlineReadingList : offlineReadingList
  const index = targetList.value.findIndex((item) => item.id === comic.id)

  if (index >= 0) {
    targetList.value.splice(index, 1)
  } else {
    targetList.value.push(comic)
  }
}

/** 清空指定来源的阅读清单 */
export const clearReadingList = (source: 'online' | 'offline') => {
  if (source === 'online') onlineReadingList.value = []
  else offlineReadingList.value = []
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
