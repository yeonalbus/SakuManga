import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { OnlineComic, FilterParams } from '@/types/comic' // 对应你的 comic.ts 类型文件[cite: 4]

export const useSubStore = defineStore('subStore', () => {
  // ─── 状态定义 ───
  const comics = ref<OnlineComic[]>([])
  const prevGid = ref<string | undefined>(undefined)
  const nextGid = ref<string | undefined>(undefined)
  const hasMore = ref<boolean>(false)
  const isLoading = ref<boolean>(false)
  const error = ref<string | null>(null)

  // 保存当前的筛选与搜索配置[cite: 4]
  const currentParams = ref<FilterParams>({})

  /**
   * 将后端 Go 返回的 OnlineComicDTO 转换为前端 OnlineComic 契约[cite: 1, 4]
   */
  const transformDTO = (dtoList: any[]): OnlineComic[] => {
    return dtoList.map((item) => ({
      ...item,
      source: 'online',
      // 兼容映射：后端 Go 字段为 clickCount，前端契约统一使用 readCount[cite: 1, 4]
      readCount: item.readCount ?? item.clickCount ?? 0,
    }))
  }

  /**
   * 核心 API 请求：针对 E 站订阅端点 (/api/v1/online/watched)
   */
  const fetchSubComics = async (
    params: FilterParams,
    mode: 'replace' | 'append' | 'prepend' = 'replace',
  ) => {
    if (isLoading.value) return
    isLoading.value = true
    error.value = null

    try {
      // 构造 URL 查询参数，对齐 Go 的 SearchParams[cite: 1]
      const query = new URLSearchParams()
      if (params.keyword) query.append('keyword', params.keyword)
      if (params.categories && params.categories.length > 0) {
        params.categories.forEach((cat) => query.append('categories', cat))
      }
      if (params.next) query.append('next', params.next)
      if (params.prev) query.append('prev', params.prev)
      if (params.seek) query.append('seek', params.seek)

      // 请求后端订阅 API（接口路径可调整为项目实际路由）
      const response = await fetch(`/api/v1/online/watched?${query.toString()}`)
      if (!response.ok) {
        throw new Error(`HTTP 状态异常: ${response.status}`)
      }

      // 解析 OnlineComicResult 数据[cite: 1]
      const resData = await response.json()
      const newComics = transformDTO(resData.comics || [])

      if (mode === 'append') {
        // 向下滑动：追加列表底部[cite: 5]
        comics.value = [...comics.value, ...newComics]
      } else if (mode === 'prepend') {
        // 向上加载较新数据：追加列表顶部[cite: 5]
        comics.value = [...newComics, ...comics.value]
      } else {
        // 初始化 / 刷新 / 跳转：重置列表
        comics.value = newComics
      }

      // 更新游标与状态标识[cite: 1]
      nextGid.value = resData.next
      prevGid.value = resData.prev
      hasMore.value = !!resData.hasMore
    } catch (err: any) {
      console.error('[subStore] 获取订阅列表失败:', err)
      error.value = err.message || '获取订阅失败，请重试'
    } finally {
      isLoading.value = false
    }
  }

  /**
   * 1. 首次/重新初始化订阅列表[cite: 5]
   */
  const fetchInitial = async (params: FilterParams = {}) => {
    currentParams.value = { ...params }
    nextGid.value = undefined
    prevGid.value = undefined
    await fetchSubComics(currentParams.value, 'replace')
  }

  /**
   * 2. 向上加载较新内容 (利用 prev 游标)[cite: 1, 5]
   */
  const loadBefore = async () => {
    if (!prevGid.value || isLoading.value) return
    const params: FilterParams = {
      ...currentParams.value,
      prev: prevGid.value,
      next: undefined,
      seek: undefined,
    }
    await fetchSubComics(params, 'prepend')
  }

  /**
   * 3. 向下滑动加载更多 (利用 next 游标)[cite: 1, 5]
   */
  const loadMore = async () => {
    if (!nextGid.value || !hasMore.value || isLoading.value) return
    const params: FilterParams = {
      ...currentParams.value,
      next: nextGid.value,
      prev: undefined,
      seek: undefined,
    }
    await fetchSubComics(params, 'append')
  }

  /**
   * 4. 按日期/时间跳转 (利用 seek 游标)[cite: 1, 5]
   */
  const seekToDate = async (date: string) => {
    const params: FilterParams = {
      ...currentParams.value,
      seek: date,
      next: undefined,
      prev: undefined,
    }
    await fetchSubComics(params, 'replace')
  }

  return {
    comics,
    prevGid,
    nextGid,
    hasMore,
    isLoading,
    error,
    fetchInitial,
    loadBefore,
    loadMore,
    seekToDate,
  }
})
