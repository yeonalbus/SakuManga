/**
 * 阅读历史 Store：在线/离线历史记录维护 + 收藏状态跨页面联动
 * 持久化迁移到后端 /history API（按登录用户隔离），本地仅保留内存态（最近 50 条）。
 */
import { ref } from 'vue'
import type { ComicItem, OfflineComic, OnlineComic } from '@/types/comic'
import { http } from '@/utils/request'
// 注意：comicStore 以命名空间导入本文件，此处直接导入属既有循环引用模式（运行时取值，无初始化时序问题）
import { onlineComics, offlineComics } from './comicStore'
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
  gid?: string // Round20-Bug1：离线历史的画廊 GID（按 gid 合并去重）
  comicTitle: string
  coverUrl: string
  token?: string
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

/** 后端记录 → 前端 HistoryItem（title 与 pageCount 归一化；在线记录透传 token；离线记录透传 gid） */
const toHistoryItem = (r: HistoryRecordDTO): HistoryItem => ({
  comic: {
    id: r.comicId,
    title: r.comicTitle,
    coverUrl: r.coverUrl,
    source: r.source,
    pageCount: r.totalPageCount || undefined,
    ...(r.source === 'online' ? { token: r.token || '' } : {}),
    ...(r.source === 'offline' && r.gid ? { gid: r.gid } : {}),
  } as ComicItem,
  readAt: r.lastReadAt,
})

/**
 * 历史去重键：优先 gid（离线同 gid 不同 id 视为同一本子），无 gid 回退 comic_id。
 * 与后端 GetHistory/AddHistory 的合并语义一致（Round20-Bug1）。
 */
const historyDedupeKey = (comic: ComicItem): string => {
  const gid = (comic as OfflineComic).gid
  return (comic.source === 'offline' && gid ? gid : comic.id) || comic.id
}

/** 从后端加载指定来源的历史记录 */
export const loadHistory = async (source: 'online' | 'offline') => {
  try {
    const data = await http<{ items: HistoryRecordDTO[]; total: number }>(
      `/history?source=${source}&limit=${MAX_HISTORY}`,
    )
    let items = (data.items || []).map(toHistoryItem)
    if (source === 'offline') {
      // Round20-Bug1：按 gid||id 去重（后端已去重，前端双保险）
      items = dedupeHistoryItems(items)
      // Round20-Bug2/D2：剔除本地库已不存在的孤儿条目（离线列表已加载时）。
      // 注意：offlineComics 为空（尚未加载）时跳过，避免误删。
      if (offlineComics.value.length > 0) {
        const alive = new Set(offlineComics.value.map((c) => c.id))
        items = items.filter((h) => alive.has(h.comic.id))
      }
    }
    if (source === 'online') onlineHistoryList.value = items
    else offlineHistoryList.value = items
  } catch (e) {
    console.error('加载历史失败:', e)
  }
}

/** 按 gid||id 去重历史项，保留最前（最新）一条 */
const dedupeHistoryItems = (items: HistoryItem[]): HistoryItem[] => {
  const seen = new Set<string>()
  const out: HistoryItem[] = []
  for (const it of items) {
    const key = historyDedupeKey(it.comic)
    if (seen.has(key)) continue
    seen.add(key)
    out.push(it)
  }
  return out
}

/** 同步历史的可选进度入参（Round3-任务1：阅读器回传 lastPageIndex/totalPageCount） */
export interface SyncHistoryOptions {
  lastPageIndex?: number
  totalPageCount?: number
}

/** 向后端写入一条历史记录（upsert，后端负责上限淘汰） */
export const syncHistory = async (
  source: 'online' | 'offline',
  comic: ComicItem,
  opts: SyncHistoryOptions = {},
) => {
  try {
    // 仅在显式提供进度时才提交 lastPageIndex/totalPageCount：
    // 卡片点击（无进度入参）不再把后端已有进度清零，避免阅读位置丢失。
    const body: Record<string, unknown> = {
      comicId: comic.id,
      source,
      comicTitle: comic.title || '',
      coverUrl: comic.coverUrl || '',
      lastChapterTitle: '',
      ...(source === 'online' ? { token: (comic as OnlineComic).token || '' } : {}),
      // Round20-Bug1：离线历史透传 gid，供后端按 gid 合并去重
      ...(source === 'offline' ? { gid: (comic as OfflineComic).gid || '' } : {}),
    }
    if (opts.lastPageIndex !== undefined) body.lastPageIndex = opts.lastPageIndex
    if (opts.totalPageCount !== undefined) body.totalPageCount = opts.totalPageCount
    await http('/history', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  } catch (e) {
    console.error('同步历史失败:', e)
  }
}

/**
 * Round3-任务1：按账号读取单条阅读进度（阅读器进入时校准定位）
 * 走 /history?source=..&comicId=.. 精确查询；无有效进度记录返回 null（不抛异常）
 */
export const getHistoryProgress = async (
  source: 'online' | 'offline',
  comicId: string,
): Promise<number | null> => {
  if (!comicId) return null
  try {
    const data = await http<{ items: HistoryRecordDTO[]; total: number }>(
      `/history?source=${source}&comicId=${encodeURIComponent(comicId)}`,
    )
    const first = (data.items || [])[0]
    if (first && typeof first.lastPageIndex === 'number' && first.lastPageIndex > 0) {
      return first.lastPageIndex
    }
    return null
  } catch (e) {
    console.error('读取阅读进度失败:', e)
    return null
  }
}

/**
 * Round7-任务4：在线画廊 token 兜底解析。
 * 历史记录可能丢失 token（旧数据 / 手动写入），打开在线详情需要 token；
 * 后端先查收藏表，再按 gid 抓取画廊页解析。失败返回 ''（不抛异常）。
 */
export const resolveOnlineToken = async (gid: string): Promise<string> => {
  if (!gid) return ''
  try {
    const data = await http<{ gid: string; token: string }>(
      `/comics/online/resolve-token?id=${encodeURIComponent(gid)}`,
    )
    return data?.token || ''
  } catch (e) {
    console.error('解析在线 token 失败:', e)
    return ''
  }
}

/** 新增/覆盖历史记录（本地去重 + 后端 upsert） */
export const addHistory = (comic: ComicItem) => {
  if (!comic || !comic.id) return

  const source = comic.source === 'online' ? 'online' : 'offline'
  const targetList = source === 'online' ? onlineHistoryList : offlineHistoryList
  // Round20-Bug1：按 gid||id 去重（离线同 gid 不同 id 的重复条目一并移除）
  const key = historyDedupeKey(comic)
  targetList.value = targetList.value.filter((item) => historyDedupeKey(item.comic) !== key)

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
