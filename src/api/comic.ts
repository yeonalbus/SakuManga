// src/api/comic.ts
import { http } from '@/utils/request' // 自动使用你的 API_BASE
import type { FilterParams, OnlineComic } from '@/types/comic' // 引入前面修改的契约

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

  // 3. 发起网络请求 (自动拼接 API_BASE + /comics/online)
  return await http<OnlineComicResponse>(`/comics/online?${query.toString()}`)
}
