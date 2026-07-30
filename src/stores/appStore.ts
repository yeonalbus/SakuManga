import { ref, watch, computed } from 'vue'
import type {
  Bookshelf,
  ComicItem,
  CardViewMode,
  OnlineComic,
  OfflineComic,
  SearchConfig,
} from '@/types/comic'
import { generateOnlineComics, generateOfflineComics } from '@/utils/mockData'

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

export const offlineComics = ref<OfflineComic[]>(
  loadStorage('app_offline_comics', generateOfflineComics(30)),
)

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
  keywords: [], // 👈 默认空队列
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
  minRating: 1,
  minPages: undefined,
  maxPages: undefined,
  onlyDownloaded: false,
})

// 🟢 在线与离线隔离的独立 Config
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
export const globalFilters = ref<any>(null)
export const setGlobalFilters = (filters: any) => {
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
// 🎯 全局历史记录 (History) 管理
// --------------------------------------------------
export interface HistoryItem {
  comic: ComicItem
  readAt: string // 阅读时间
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

/** 🎯 核心分流记录函数：根据 comic.source 自动写入对应历史队列 */
export const addHistory = (comic: ComicItem) => {
  if (!comic || !comic.id) return

  // 1. 判断该漫画来源于 online 还是 offline
  const targetList = comic.source === 'online' ? onlineHistoryList : offlineHistoryList

  // 2. 排重（过滤掉该列表中已存在的同 ID 记录）
  targetList.value = targetList.value.filter((item) => item.comic.id !== comic.id)

  // 3. 将最新浏览的作品压入对应列表的最前面（最新浏览置顶）
  targetList.value.unshift({
    comic,
    readAt: new Date().toLocaleString(),
  })

  // 4. 最多保留 50 条历史记录
  if (targetList.value.length > 50) {
    targetList.value.pop()
  }
}

/** 🎯 清空指定来源的历史记录 */
export const clearHistory = (source: 'online' | 'offline') => {
  if (source === 'online') {
    onlineHistoryList.value = []
  } else {
    offlineHistoryList.value = []
  }
}
