/**
 * 阅读历史 Store：在线/离线历史记录维护 + 收藏状态跨页面联动
 * 由原 appStore 拆分而来
 */
import { ref, watch } from 'vue'
import type { ComicItem, OnlineComic } from '@/types/comic'
import { loadStorage } from '@/utils/storage'
import { onlineComics } from './comicStore'
import { onlineReadingList } from './readingStore'

/** 历史记录项 */
export interface HistoryItem {
  comic: ComicItem
  readAt: string
}

/** 在线 / 离线历史列表（持久化） */
export const onlineHistoryList = ref<HistoryItem[]>(loadStorage('app_online_history', []))
export const offlineHistoryList = ref<HistoryItem[]>(loadStorage('app_offline_history', []))

watch(
  onlineHistoryList,
  (val) => {
    localStorage.setItem('app_online_history', JSON.stringify(val))
  },
  { deep: true },
)

watch(
  offlineHistoryList,
  (val) => {
    localStorage.setItem('app_offline_history', JSON.stringify(val))
  },
  { deep: true },
)

/** 新增/覆盖历史记录（去重 + 最多保留 50 条） */
export const addHistory = (comic: ComicItem) => {
  if (!comic || !comic.id) return

  const targetList = comic.source === 'online' ? onlineHistoryList : offlineHistoryList
  targetList.value = targetList.value.filter((item) => item.comic.id !== comic.id)

  targetList.value.unshift({
    comic,
    readAt: new Date().toLocaleString(),
  })

  if (targetList.value.length > 50) {
    targetList.value.pop()
  }
}

/** 清空指定来源的历史记录 */
export const clearHistory = (source: 'online' | 'offline') => {
  if (source === 'online') {
    onlineHistoryList.value = []
  } else {
    offlineHistoryList.value = []
  }
}

/**
 * 全局同步更新指定在线画廊的收藏状态
 * 同步范围：在线主列表 (onlineComics) / 在线阅读清单 (onlineReadingList) / 在线历史 (onlineHistoryList)
 */
export const updateOnlineFavoriteState = (gid: string, isFavorite: boolean, favIndex?: number) => {
  // 1. 同步更新在线主列表
  const itemInList = onlineComics.value.find((c) => c.id === gid)
  if (itemInList) {
    itemInList.isFavorite = isFavorite
    itemInList.favIndex = favIndex
  }

  // 2. 同步更新在线阅读清单
  const itemInQueue = onlineReadingList.value.find((c) => c.id === gid)
  if (itemInQueue && itemInQueue.source === 'online') {
    ;(itemInQueue as OnlineComic).isFavorite = isFavorite
    ;(itemInQueue as OnlineComic).favIndex = favIndex
  }

  // 3. 同步更新在线历史记录
  const itemInHistory = onlineHistoryList.value.find((h) => h.comic.id === gid)
  if (itemInHistory && itemInHistory.comic.source === 'online') {
    ;(itemInHistory.comic as OnlineComic).isFavorite = isFavorite
    ;(itemInHistory.comic as OnlineComic).favIndex = favIndex
  }
}
