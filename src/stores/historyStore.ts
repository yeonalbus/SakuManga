/**
 * 阅读历史 Store：在线/离线历史记录维护 + 收藏状态跨页面联动
 * 持久化迁移到后端 /history API（按登录用户隔离），本地仅保留内存态（最近 50 条）。
 */
import { ref } from 'vue'
import type { ComicItem, OnlineComic } from '@/types/comic'
import { http } from '@/utils/request'
import { onlineComics } from './comicStore'
import { onlineReadingList } from './readingStore'

/** 历史记录项 */
export interface HistoryItem {
  comic: ComicItem
  readAt: string
}

/** 后端 ReadHistory 记录结构 */
export interface HistoryRecordDTO {
  id?: number
  comicId: string
  source: 'online' | 'offline'
  comicTitle: string
  coverUrl: string
  lastChapterTitle?: string
  lastPageIndex?: number
  totalPageCount?: number
  lastReadAt: string
}

/** 前端保留的历史展示上限 */
const MAX_HISTORY = 50

/** 在线 / 离线历史列表（内存态） */
export const onlineHistoryList = ref<HistoryItem[]>([])
export const offlineHistoryList = ref<HistoryItem[]>([])

/** 后端记录 → 前端 HistoryItem（title 与 pageCount 归一化） */
const toHistoryItem = (r: HistoryRecordDTO): HistoryItem => ({
  comic: {
    id: r.comicId,
    title: r.comicTitle,
    coverUrl: r.coverUrl,
    source: r.source,
    pageCount: r.totalPageCount || undefined,
  } as ComicItem,
  readAt: r.lastReadAt,
})

/** 从后端加载指定来源的历史记录 */
export const loadHistory = async (source: 'online' | 'offline') => {
  try {
    const data = await http<{ items: HistoryRecordDTO[]; total: number }>(
      `/history?source=${source}&limit=${MAX_HISTORY}`,
    )
    const items = (data.items || []).map(toHistoryItem)
    if (source === 'online') onlineHistoryList.value = items
    else offlineHistoryList.value = items
  } catch (e) {
    console.error('加载历史失败:', e)
  }
}

/** 向后端写入一条历史记录（upsert，后端负责上限淘汰） */
export const syncHistory = async (source: 'online' | 'offline', comic: ComicItem) => {
  try {
    await http('/history', {
      method: 'POST',
      body: JSON.stringify({
        comicId: comic.id,
        source,
        comicTitle: comic.title || '',
        coverUrl: comic.coverUrl || '',
        lastChapterTitle: '',
        lastPageIndex: 0,
        totalPageCount: comic.pageCount || 0,
      }),
    })
  } catch (e) {
    console.error('同步历史失败:', e)
  }
}

/** 新增/覆盖历史记录（本地去重 + 后端 upsert） */
export const addHistory = (comic: ComicItem) => {
  if (!comic || !comic.id) return

  const source = comic.source === 'online' ? 'online' : 'offline'
  const targetList = source === 'online' ? onlineHistoryList : offlineHistoryList
  targetList.value = targetList.value.filter((item) => item.comic.id !== comic.id)

  targetList.value.unshift({
    comic,
    readAt: new Date().toLocaleString(),
  })

  if (targetList.value.length > MAX_HISTORY) {
    targetList.value.pop()
  }

  syncHistory(source, comic)
}

/** 清空指定来源的历史记录（本地 + 后端） */
export const clearHistory = (source: 'online' | 'offline') => {
  if (source === 'online') {
    onlineHistoryList.value = []
  } else {
    offlineHistoryList.value = []
  }
  http(`/history?source=${source}`, { method: 'DELETE' }).catch(() => {})
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
