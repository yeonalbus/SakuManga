/**
 * 搜索/筛选配置 Store：在线与离线作用域隔离的“生效中筛选条件”
 * 由原 appStore 拆分而来
 */
import { ref } from 'vue'
import type { SearchConfig } from '@/types/comic'

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

/** 在线 / 离线 搜索配置（作用域隔离） */
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
