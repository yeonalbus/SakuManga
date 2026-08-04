import { ref, watch, computed } from 'vue'
import type {
  Bookshelf,
  ComicItem,
  CardViewMode,
  OnlineComic,
  OfflineComic,
  SearchConfig,
} from '@/types/comic'
import { generateOnlineComics } from '@/utils/mockData'

// 辅助函数：从 localStorage 安全读取 JSON 数据
function loadStorage<T>(key: string, defaultValue: T): T {
  try {
    const saved = localStorage.getItem(key)
    return saved ? JSON.parse(saved) : defaultValue
  } catch (e) {
    console.error(`读取 LocalStorage [${key}] 失败`, e)
    return defaultValue
  }
}

// --------------------------------------------------
// 0. 类型定义 (额外扫描路径)
// --------------------------------------------------
export interface ExtraScanPath {
  id: string
  path: string
  includeSubfolders: boolean
  lastScanned?: number
  comicCount?: number
}

// --------------------------------------------------
// 1. 全局视图模式 (Card / Compact)
// --------------------------------------------------
export const viewMode = ref<CardViewMode>(loadStorage('app_view_mode', 'card'))

watch(viewMode, (newVal) => {
  localStorage.setItem('app_view_mode', JSON.stringify(newVal))
})

export const toggleViewMode = () => {
  viewMode.value = viewMode.value === 'card' ? 'compact' : 'card'
}

// --------------------------------------------------
// 2. 自定义书架列表 (Bookshelves)
// --------------------------------------------------
const defaultBookshelves: Bookshelf[] = [
  { id: 'shelf-1', name: '热血必看', count: 12, comicIds: [] },
  { id: 'shelf-2', name: '纯爱战神', count: 5, comicIds: [] },
  { id: 'shelf-3', name: '待分类本地本', count: 28, comicIds: [] },
]

export const bookshelves = ref<Bookshelf[]>(loadStorage('app_bookshelves', defaultBookshelves))

watch(
  bookshelves,
  (newVal) => {
    localStorage.setItem('app_bookshelves', JSON.stringify(newVal))
  },
  { deep: true },
)

/** 新建书架 */
export const addBookshelf = (name: string) => {
  if (!name.trim()) return
  const newShelf: Bookshelf = {
    id: `shelf-${Date.now()}`,
    name: name.trim(),
    count: 0,
    comicIds: [],
  }
  bookshelves.value.push(newShelf)
}

/** 删除书架 */
export const removeBookshelf = (id: string) => {
  bookshelves.value = bookshelves.value.filter((b) => b.id !== id)
}

// --------------------------------------------------
// 3. 全局在线 / 离线漫画数据源 (支持持久化)
// --------------------------------------------------
export const onlineComics = ref<OnlineComic[]>(
  loadStorage('app_online_comics', generateOnlineComics(50)),
)

watch(
  onlineComics,
  (val) => {
    localStorage.setItem('app_online_comics', JSON.stringify(val))
  },
  { deep: true },
)

export const offlineComics = ref<OfflineComic[]>([])

export const fetchOfflineComics = async () => {
  try {
    const res = await fetch(`${API_BASE}/comics/offline`)
    if (res.ok) {
      const rawData = await res.json()
      // 🎯 在源头把 tags 字符串解析为真正的 JS 数组
      offlineComics.value = rawData.map((item: any) => ({
        ...item,
        tags: typeof item.tags === 'string' ? JSON.parse(item.tags || '[]') : item.tags || [],
      }))
    }
  } catch (err) {
    console.error('拉取离线漫画失败:', err)
  }
}

watch(
  offlineComics,
  (val) => {
    localStorage.setItem('app_offline_comics', JSON.stringify(val))
  },
  { deep: true },
)

/** 🎯 动态计算书架真实作品数量 */
export const computedBookshelves = computed(() => {
  return bookshelves.value.map((shelf) => {
    const actualCount = offlineComics.value.filter(
      (c) => c.bookshelfId === shelf.id || (shelf.comicIds && shelf.comicIds.includes(c.id)),
    ).length

    return {
      ...shelf,
      count: actualCount,
    }
  })
})

/** 🎯 记录阅读频次自增 (供详情页阅读按钮调用) */
export const recordComicClick = (comicId: string) => {
  const comic = offlineComics.value.find((c) => c.id === comicId)
  if (comic) {
    comic.readCount = (comic.readCount || 0) + 1
  }
}

/** 🎯 本地排行榜计算属性 */
export const rankedOfflineComics = computed(() => {
  return [...offlineComics.value].sort((a, b) => (b.readCount || 0) - (a.readCount || 0))
})

// --------------------------------------------------
// 4. 搜索与筛选配置 (在线 / 离线 作用域隔离)
// --------------------------------------------------
export const createDefaultSearchConfig = (): SearchConfig => ({
  keyword: '',
  keywords: [],
  activeCategories: [
    'Doujinshi',
    'Manga',
    'Artist CG',
    'Game CG',
    'Image Set',
    'Cosplay',
    'Non-H',
    'Western',
    'Asian Porn',
    'Misc',
  ],
  minRating: 0,
  minPages: undefined,
  maxPages: undefined,
  onlyDownloaded: false,
})

export const onlineSearchConfig = ref<SearchConfig>(createDefaultSearchConfig())
export const offlineSearchConfig = ref<SearchConfig>(createDefaultSearchConfig())

/** 重置指定作用域配置 */
export const resetSearchConfig = (scope: 'online' | 'offline') => {
  if (scope === 'online') {
    onlineSearchConfig.value = createDefaultSearchConfig()
  } else {
    offlineSearchConfig.value = createDefaultSearchConfig()
  }
}

// 🟢 兼容导出 (防止旧组件引入 globalFilters 导致语法报错)
export const globalFilters = ref<unknown>(null)
export const setGlobalFilters = (filters: unknown) => {
  globalFilters.value = filters
}

// --------------------------------------------------
// 5. 悬浮阅读清单 (Reading List Queue) - 统一状态
// --------------------------------------------------
export const onlineReadingList = ref<ComicItem[]>(loadStorage('app_online_reading_list', []))
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

/** 🎯 统一的添加/移除阅读清单函数 */
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

/** 🎯 核心调度函数：获取队列中的下一个作品 */
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

// --------------------------------------------------
// 6. 全局历史记录 (History) 管理
// --------------------------------------------------
export interface HistoryItem {
  comic: ComicItem
  readAt: string
}

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

export const clearHistory = (source: 'online' | 'offline') => {
  if (source === 'online') {
    onlineHistoryList.value = []
  } else {
    offlineHistoryList.value = []
  }
}

// --------------------------------------------------
// 7. 额外的画廊扫描路径管理 (对齐 Go 后端 API)
// --------------------------------------------------
const API_BASE = '/api/v1'

export const scanPaths = ref<ExtraScanPath[]>([])

/** 从 Go 后端拉取所有扫描路径 */
export const fetchScanPaths = async () => {
  try {
    const res = await fetch(`${API_BASE}/scan-paths`)
    if (res.ok) {
      scanPaths.value = await res.json()
    }
  } catch (err) {
    console.error('连接 Go 后端失败，请检查 backend 服务是否启动:', err)
  }
}

/** 添加新路径 */
export const addScanPath = async (path: string): Promise<boolean> => {
  try {
    const res = await fetch(`${API_BASE}/scan-paths`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: path.trim() }),
    })
    if (res.ok) {
      const newPath = await res.json()
      scanPaths.value.push(newPath)
      return true
    }
  } catch (err) {
    console.error('添加路径失败:', err)
  }
  return false
}

/** 切换是否包含子文件夹 */
export const toggleSubfolders = async (id: string, includeSubfolders: boolean) => {
  const item = scanPaths.value.find((p) => p.id === id)
  if (item) {
    item.includeSubfolders = includeSubfolders
    try {
      await fetch(`${API_BASE}/scan-paths/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ includeSubfolders }),
      })
    } catch (err) {
      console.error('更新子文件夹状态失败:', err)
    }
  }
}

/** 移除指定路径 */
export const removeScanPath = async (id: string) => {
  scanPaths.value = scanPaths.value.filter((p) => p.id !== id)
  try {
    await fetch(`${API_BASE}/scan-paths/${id}`, {
      method: 'DELETE',
    })
  } catch (err) {
    console.error('删除路径失败:', err)
  }
}

/** 触发指定路径的后端的扫描 */
export const updateScanPathStats = async (id: string) => {
  try {
    const res = await fetch(`${API_BASE}/scan-paths/${id}/scan`, {
      method: 'POST',
    })
    if (res.ok) {
      const data = await res.json()
      const item = scanPaths.value.find((p) => p.id === id)
      if (item) {
        item.lastScanned = data.lastScanned
        item.comicCount = data.comicCount
      }
    }
  } catch (err) {
    console.error('触发扫描失败:', err)
  }
}

// --------------------------------------------------
// 🎯 统一导出 useAppStore Hook，确保组件解构正常
// --------------------------------------------------
export function useAppStore() {
  return {
    viewMode,
    toggleViewMode,
    bookshelves,
    addBookshelf,
    removeBookshelf,
    onlineComics,
    offlineComics,
    computedBookshelves,
    recordComicClick,
    rankedOfflineComics,
    onlineSearchConfig,
    offlineSearchConfig,
    resetSearchConfig,
    globalFilters,
    setGlobalFilters,
    onlineReadingList,
    offlineReadingList,
    toggleReadingList,
    clearReadingList,
    getNextComicInQueue,
    onlineHistoryList,
    offlineHistoryList,
    addHistory,
    clearHistory,
    updateOnlineFavoriteState,
    // 扫描路径 API
    scanPaths,
    fetchScanPaths,
    addScanPath,
    toggleSubfolders,
    removeScanPath,
    updateScanPathStats,
  }
}

// --------------------------------------------------
// 在线历史记录
// --------------------------------------------------

// --------------------------------------------------
// 🎯 8. 跨页面状态同步 (收藏状态联动)
// --------------------------------------------------
/** 全局同步更新指定在线画廊的收藏状态 (主列表、阅读清单、历史记录) */
export const updateOnlineFavoriteState = (gid: string, isFavorite: boolean, favIndex?: number) => {
  // 1. 同步更新在线主列表 (onlineComics)
  const itemInList = onlineComics.value.find((c) => c.id === gid)
  if (itemInList) {
    itemInList.isFavorite = isFavorite
    itemInList.favIndex = favIndex
  }

  // 2. 同步更新在线阅读清单 (onlineReadingList)
  const itemInQueue = onlineReadingList.value.find((c) => c.id === gid)
  if (itemInQueue && itemInQueue.source === 'online') {
    ;(itemInQueue as OnlineComic).isFavorite = isFavorite
    ;(itemInQueue as OnlineComic).favIndex = favIndex
  }

  // 3. 同步更新在线历史记录 (onlineHistoryList)
  const itemInHistory = onlineHistoryList.value.find((h) => h.comic.id === gid)
  if (itemInHistory && itemInHistory.comic.source === 'online') {
    ;(itemInHistory.comic as OnlineComic).isFavorite = isFavorite
    ;(itemInHistory.comic as OnlineComic).favIndex = favIndex
  }
}
