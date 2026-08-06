// src/api/comic.ts
import { http } from '@/utils/request' // 自动使用你的 API_BASE
import type {
  FilterParams,
  OnlineComic,
  RandomComicParams,
  RandomComicResponse,
} from '@/types/comic' // 引入前面修改的契约

/** 后端接口返回的契约结构 */
export interface OnlineComicResponse {
  comics: OnlineComic[]
  next?: string
  prev?: string
  hasMore?: boolean
}

/**
 * 线上获取漫画列表 API
 */
export const fetchOnlineComicsApi = async (
  params: FilterParams = {},
): Promise<OnlineComicResponse> => {
  const query = new URLSearchParams()

  // 1. 拼接游标与搜索入参
  if (params.keyword) query.append('keyword', params.keyword)
  if (params.next) query.append('next', params.next)
  if (params.prev) query.append('prev', params.prev)
  if (params.seek) query.append('seek', params.seek)

  // 2. 拼接分类数组
  if (params.categories && params.categories.length > 0) {
    params.categories.forEach((cat) => query.append('categories', cat))
  }

  // 3. 拼接 E-Hentai 高级筛选参数 (后端据此生成 advsearch=1 与 f_* 参数)
  if (params.minRating && params.minRating > 0) {
    query.append('minRating', String(params.minRating)) // f_srdd=x
  }
  if (params.language && params.language !== 'All') {
    query.append('language', params.language) // 追加 language:xxx 到 f_search
  }
  if (params.onlyRemoved) query.append('onlyRemoved', '1') // f_sh=on
  if (params.onlyTorrents) query.append('onlyTorrents', '1') // f_sto=on
  if (params.disableLangFilter) query.append('disableLangFilter', '1') // f_sfl=on
  if (params.disableUploaderFilter) query.append('disableUploaderFilter', '1') // f_sfu=on
  if (params.disableTagFilter) query.append('disableTagFilter', '1') // f_sft=on

  // 4. 发起网络请求 (自动拼接 API_BASE + /comics/online)
  return await http<OnlineComicResponse>(`/comics/online?${query.toString()}`)
}

/**
 * 🎲 随机抽卡 API
 * 后端：离线 SQL ORDER BY RANDOM()，在线抓随机页采样，all 混合降级
 */
export const fetchRandomComicsApi = async (
  params: RandomComicParams,
): Promise<RandomComicResponse> => {
  const query = new URLSearchParams()

  query.append('count', String(params.count))
  query.append('source', params.source)

  // 1. 抽卡专用过滤器逐项下发
  if (params.keyword) query.append('keyword', params.keyword)
  // 多关键词队列逐项下发（在线由后端合并 f_search，离线 AND 匹配）
  if (params.keywords && params.keywords.length > 0) {
    params.keywords.forEach((k) => query.append('keywords', k))
  }
  if (params.categories && params.categories.length > 0) {
    params.categories.forEach((cat) => query.append('categories', cat))
  }
  if (params.minRating && params.minRating > 0) query.append('minRating', String(params.minRating))
  if (params.minPages && params.minPages > 0) query.append('minPages', String(params.minPages))
  if (params.maxPages && params.maxPages > 0) query.append('maxPages', String(params.maxPages))
  if (params.language && params.language !== 'All') query.append('language', params.language)
  if (params.onlyDownloaded) query.append('onlyDownloaded', 'true')

  // 2. 在线高级筛选透传（后端据此生成 advsearch=1 与 f_* 参数）
  if (params.onlyRemoved) query.append('onlyRemoved', '1')
  if (params.onlyTorrents) query.append('onlyTorrents', '1')
  if (params.disableLangFilter) query.append('disableLangFilter', '1')
  if (params.disableUploaderFilter) query.append('disableUploaderFilter', '1')
  if (params.disableTagFilter) query.append('disableTagFilter', '1')

  // 3. 发起网络请求 (自动拼接 API_BASE + /comics/random)
  return await http<RandomComicResponse>(`/comics/random?${query.toString()}`)
}
