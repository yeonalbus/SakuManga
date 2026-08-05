/**
 * 登录后用户库数据初始化：并行加载书架 / 历史 / 阅读清单 / 个人评分，
 * 并在后端数据为空时把旧 localStorage 数据（app_bookshelves / app_online_history /
 * app_offline_history / app_online_reading_list / app_offline_reading_list）迁移到后端。
 */
import { http } from '@/utils/request'
import type { ComicItem } from '@/types/comic'
import { loadBookshelves, migrateLegacyBookshelves } from './bookshelfStore'
import {
  loadHistory,
  syncHistory,
  onlineHistoryList,
  offlineHistoryList,
  type HistoryItem,
} from './historyStore'
import { loadReadingList, onlineReadingList, offlineReadingList } from './readingStore'
import { loadMyRatings } from './ratingStore'

/** 旧 localStorage 迁移是否已执行（避免重复） */
let migrated = false

/** 迁移旧 localStorage 的在线/离线历史到后端 */
const migrateLegacyHistory = async (source: 'online' | 'offline') => {
  const list = source === 'online' ? onlineHistoryList : offlineHistoryList
  // 后端已有数据：无需迁移，直接丢弃旧缓存
  if (list.value.length > 0) {
    localStorage.removeItem(source === 'online' ? 'app_online_history' : 'app_offline_history')
    return
  }
  const raw = localStorage.getItem(
    source === 'online' ? 'app_online_history' : 'app_offline_history',
  )
  if (!raw) return
  try {
    const legacy = JSON.parse(raw) as HistoryItem[]
    if (!Array.isArray(legacy) || legacy.length === 0) return
    for (const item of legacy) {
      if (item?.comic?.id) {
        await syncHistory(source, item.comic)
      }
    }
    localStorage.removeItem(source === 'online' ? 'app_online_history' : 'app_offline_history')
    await loadHistory(source)
  } catch (e) {
    console.error('迁移历史失败:', e)
  }
}

/** 迁移旧 localStorage 的在线/离线阅读清单到后端 */
const migrateLegacyReadingList = async (source: 'online' | 'offline') => {
  const list = source === 'online' ? onlineReadingList : offlineReadingList
  if (list.value.length > 0) {
    localStorage.removeItem(
      source === 'online' ? 'app_online_reading_list' : 'app_offline_reading_list',
    )
    return
  }
  const raw = localStorage.getItem(
    source === 'online' ? 'app_online_reading_list' : 'app_offline_reading_list',
  )
  if (!raw) return
  try {
    const legacy = JSON.parse(raw) as ComicItem[]
    if (!Array.isArray(legacy) || legacy.length === 0) return
    await http('/reading-list', {
      method: 'PUT',
      body: JSON.stringify({ source, items: legacy }),
    })
    localStorage.removeItem(
      source === 'online' ? 'app_online_reading_list' : 'app_offline_reading_list',
    )
    await loadReadingList(source)
  } catch (e) {
    console.error('迁移阅读清单失败:', e)
  }
}

/**
 * 加载当前登录用户的全部库数据（书架/历史/阅读清单/评分），并执行首次登录的旧数据迁移。
 * 登录成功后或页面刷新恢复会话时调用。
 */
export const loadUserLibrary = async () => {
  await Promise.all([
    loadBookshelves(),
    loadHistory('online'),
    loadHistory('offline'),
    loadReadingList('online'),
    loadReadingList('offline'),
    loadMyRatings(),
  ])

  if (migrated) return
  migrated = true

  await migrateLegacyBookshelves()
  await migrateLegacyHistory('online')
  await migrateLegacyHistory('offline')
  await migrateLegacyReadingList('online')
  await migrateLegacyReadingList('offline')
}
