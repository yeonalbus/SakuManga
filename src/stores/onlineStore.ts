import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { OnlineComic, FilterParams } from '@/types/comic' // 引入之前更新的数据契约
import { fetchOnlineComicsApi } from '@/api/comic' // 假设的底层 API 请求函数

export const useOnlineStore = defineStore('onlineStore', () => {
  // ──────────────────────────────────────────
  // 1. 响应式状态定义
  // ──────────────────────────────────────────
  const comics = ref<OnlineComic[]>([]) // 当前已加载的画廊列表
  const nextGid = ref<string | undefined>(undefined) // 向下加载锚点
  const prevGid = ref<string | undefined>(undefined) // 向上加载锚点
  const currentSeek = ref<string>('') // 当前选中的日期定位 (YYYY-MM-DD)
  const hasMore = ref<boolean>(true) // 是否还能继续加载[cite: 3]
  const isLoading = ref<boolean>(false)
  const error = ref<string | null>(null)

  // 基础搜索/筛选过滤条件保存
  const currentFilter = ref<FilterParams>({})

  // ──────────────────────────────────────────
  // 2. 核心动作 Action
  // ──────────────────────────────────────────

  /**
   * 初始化/重置加载（首次进入页面、触发新搜索时使用）
   */
  const fetchInitial = async (params: FilterParams = {}) => {
    isLoading.value = true
    error.value = null
    currentFilter.value = params

    // 重置状态与游标队列[cite: 1]
    comics.value = []
    nextGid.value = undefined
    prevGid.value = undefined
    currentSeek.value = params.seek || ''
    hasMore.value = true

    try {
      const res = await fetchOnlineComicsApi(params)
      comics.value = res.comics
      nextGid.value = res.next
      prevGid.value = res.prev
      hasMore.value = res.hasMore ?? !!res.next
    } catch (err: any) {
      error.value = err?.message || '获取画廊列表失败'
    } finally {
      isLoading.value = false
    }
  }

  /**
   * 下滑加载更多 (Load More)[cite: 1]
   */
  const loadMore = async () => {
    // 处于加载中、没有更多数据或没有 nextGid 时拦截
    if (isLoading.value || !hasMore.value || !nextGid.value) return

    isLoading.value = true
    error.value = null

    try {
      const res = await fetchOnlineComicsApi({
        ...currentFilter.value,
        next: nextGid.value, // 传入 nextGid 游标
      })

      // 流式追加数据到列表尾部[cite: 1]
      comics.value.push(...res.comics)
      nextGid.value = res.next
      hasMore.value = res.hasMore ?? !!res.next
    } catch (err: any) {
      error.value = err?.message || '加载更多失败'
    } finally {
      isLoading.value = false
    }
  }

  /**
   * 按日期跳转 (Seek to Date)[cite: 1]
   * @param dateStr 目标日期，如 "2023-05-20"
   */
  const seekToDate = async (dateStr: string) => {
    if (!dateStr || isLoading.value) return
    currentSeek.value = dateStr

    // 重新发起带着 seek 参数的全新请求[cite: 1]
    await fetchInitial({
      ...currentFilter.value,
      seek: dateStr,
    })
  }

  /**
   * 🟢 向上加载较新内容 (Load Before)
   */
  const loadBefore = async () => {
    if (isLoading.value || !prevGid.value) return

    isLoading.value = true
    error.value = null

    try {
      const res = await fetchOnlineComicsApi({
        ...currentFilter.value,
        prev: prevGid.value, // 传入向上游标
      })

      // 使用 unshift 向列表头部前置插入新数据
      comics.value.unshift(...(res.comics || []))
      // 更新上游游标
      prevGid.value = res.prev
    } catch (err: any) {
      error.value = err?.message || '加载较新内容失败'
    } finally {
      isLoading.value = false
    }
  }

  // remember to export loadBefore in the return block!
  return {
    comics,
    nextGid,
    prevGid,
    currentSeek,
    hasMore,
    isLoading,
    error,
    fetchInitial,
    loadMore,
    loadBefore, // 👈 导出 loadBefore
    seekToDate,
  }
})
