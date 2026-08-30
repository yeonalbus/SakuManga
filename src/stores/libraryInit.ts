/**
 * 登录后用户库数据初始化：并行加载书架 / 历史 / 阅读清单 / 个人评分，
 * 并在后端数据为空时把旧 localStorage 数据（app_bookshelves / app_online_history /
 * app_offline_history / app_online_reading_list / app_offline_reading_list）迁移到后端。
 */
import { http } from '@/utils/request'
import type { ComicItem } from '@/types/comic'
import { loadBookshelves, migrateLegacyBookshelves } from './bookshelfStore'
// Round28：换用户重新加载依赖 token 对比
import { TOKEN_KEY } from '@/config/api'
// Round20-Bug4/D2：先加载离线漫画列表，使 loadHistory('offline') 能剔除本地库已不存在的孤儿历史
import { fetchOfflineComics } from './comicStore'
import {
  loadHistory,
  syncHistory,
  onlineHistoryList,
  offlineHistoryList,
  type HistoryItem,
} from './historyStore'
import { loadReadingList, onlineReadingList, offlineReadingList } from './readingStore'
import { loadMyRatings } from './ratingStore'
// Round28：搜刮书签后端化（登录后加载，多端同步）
import { loadScrapeBookmarks } from './scrapeBookmarksStore'

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
  // Round20-Bug4/D2：先拉取离线漫画列表，loadHistory('offline') 据此剔除本地库已不存在的孤儿历史
  await fetchOfflineComics()
  await Promise.all([
    loadBookshelves(),
    loadHistory('online'),
    loadHistory('offline'),
    loadReadingList('online'),
    loadReadingList('offline'),
    loadMyRatings(),
    loadScrapeBookmarks(), // Round28：搜刮书签后端化（含旧 localStorage 迁移）
  ])

  if (migrated) return
  migrated = true

  await migrateLegacyBookshelves()
  await migrateLegacyHistory('online')
  await migrateLegacyHistory('offline')
  await migrateLegacyReadingList('online')
  await migrateLegacyReadingList('offline')
}

/** 库数据加载 Promise 缓存：登录 / 会话恢复 / 路由守卫共用，避免重复并发加载 */
let libraryReady: Promise<void> | null = null
/** 当前缓存所属用户 token：换用户（登出再登录）时强制重新加载，避免张冠李戴 */
let libraryLoadedForToken: string | null = null

/**
 * 确保当前登录用户的库数据（书架/历史/阅读清单/评分/搜刮书签）已加载完成。
 * - 同一 token 复用缓存 Promise，避免重复并发加载；
 * - 换用户（token 变化）强制重新加载（书签/历史按用户隔离）；
 * - 加载失败允许下次重试（缓存清空）；
 * - 供路由守卫 await：保证 URL 驱动的书签跳转（?bm=xxx）在组件 setup 前数据就绪。
 */
export const ensureLibraryLoaded = (): Promise<void> => {
  const token = localStorage.getItem(TOKEN_KEY) || ''
  if (libraryReady && libraryLoadedForToken === token) {
    return libraryReady
  }
  libraryLoadedForToken = token
  libraryReady = loadUserLibrary().catch((e) => {
    console.error('初始化库数据失败:', e)
    libraryReady = null
  })
  return libraryReady
}
