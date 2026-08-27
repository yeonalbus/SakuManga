/**
 * 搜索/筛选配置 Store：在线与离线作用域隔离的“生效中筛选条件”
 * 由原 appStore 拆分而来
 */
import { ref, watch } from 'vue'
import type { SearchConfig } from '@/types/comic'
import { preferenceSettings } from '@/stores/preferenceSettings'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 生成一份默认搜索/筛选配置 */
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
  // ─── E-Hentai 高级筛选 (f_* 参数) 默认值 ───
  language: 'All',
  onlyRemoved: false,
  onlyTorrents: false,
  disableLangFilter: false,
  disableUploaderFilter: false,
  disableTagFilter: false,
})

// ─── 搜索配置持久化（Bug：筛选继承）───
// 搜索跳转在桌面端走「新标签页」（全新 JS 上下文），若筛选只存内存必然丢失，
// 导致「搜索后跳转的页面重置筛选内容」。此处仿 preferenceSettings 持久化模式：
// 初始化从 localStorage 恢复，变更自动落盘（aggressive 回收级联，配额紧张也能写入）。
const ONLINE_KEY = 'saku_search_config_online'
const OFFLINE_KEY = 'saku_search_config_offline'

/** 从 localStorage 恢复配置：与默认值逐字段合并，数组字段异常时回退默认 */
const restoreSearchConfig = (raw: unknown): SearchConfig => {
  const d = createDefaultSearchConfig()
  if (!raw || typeof raw !== 'object') return d
  const r = raw as Partial<SearchConfig>
  return {
    ...d,
    ...r,
    activeCategories:
      Array.isArray(r.activeCategories) && r.activeCategories.length > 0
        ? r.activeCategories
        : d.activeCategories,
    keywords: Array.isArray(r.keywords) ? r.keywords : d.keywords,
    keyword: typeof r.keyword === 'string' ? r.keyword : d.keyword,
    language: typeof r.language === 'string' ? r.language : d.language,
  }
}

/** 在线 / 离线 搜索配置（作用域隔离，持久化恢复 + 自动落盘） */
export const onlineSearchConfig = ref<SearchConfig>(
  restoreSearchConfig(loadStorage(ONLINE_KEY, null)),
)
export const offlineSearchConfig = ref<SearchConfig>(
  restoreSearchConfig(loadStorage(OFFLINE_KEY, null)),
)

watch(
  onlineSearchConfig,
  (val) => {
    saveStorage(ONLINE_KEY, val)
  },
  { deep: true },
)
watch(
  offlineSearchConfig,
  (val) => {
    saveStorage(OFFLINE_KEY, val)
  },
  { deep: true },
)

/** 重置指定作用域配置 */
export const resetSearchConfig = (scope: 'online' | 'offline') => {
  if (scope === 'online') {
    onlineSearchConfig.value = createDefaultSearchConfig()
  } else {
    offlineSearchConfig.value = createDefaultSearchConfig()
  }
}

/** 依据「搜索选项继承」偏好，在提交新搜索前将筛选选项重置为策略允许的状态（keyword 由调用方维护） */
export const applySearchOptionsInherit = (scope: 'online' | 'offline') => {
  const inherit = preferenceSettings.searchOptionsInherit
  // 继承全部：保持当前筛选条件不变
  if (inherit === 'all') return
  const cfg = scope === 'online' ? onlineSearchConfig.value : offlineSearchConfig.value
  const d = createDefaultSearchConfig()
  // 不继承：连分类选择也一并重置
  if (inherit === 'none') {
    cfg.activeCategories = d.activeCategories
  }
  cfg.minRating = d.minRating
  cfg.minPages = d.minPages
  cfg.maxPages = d.maxPages
  cfg.onlyDownloaded = d.onlyDownloaded
  cfg.language = d.language
  cfg.onlyRemoved = d.onlyRemoved
  cfg.onlyTorrents = d.onlyTorrents
  cfg.disableLangFilter = d.disableLangFilter
  cfg.disableUploaderFilter = d.disableUploaderFilter
  cfg.disableTagFilter = d.disableTagFilter
  cfg.keywords = []
}

// 兼容导出（防止旧组件引入 globalFilters 导致语法报错）
export const globalFilters = ref<unknown>(null)
export const setGlobalFilters = (filters: unknown) => {
  globalFilters.value = filters
}

/** 订阅专用（OnlineSub 页）的搜索/分类过滤参数 */
export const subSearchConfig = ref<{
  keyword: string
  activeCategories: string[]
}>({
  keyword: '',
  activeCategories: [],
})
