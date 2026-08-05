/**
 * 漫画数据 Store：在线/离线漫画列表、本地数据拉取与阅读统计
 * 由原 appStore 拆分而来，负责漫画相关数据源与派生计算
 */
import { ref, watch, computed } from 'vue'
import type { OnlineComic, OfflineComic } from '@/types/comic'
import { generateOnlineComics } from '@/utils/mockData'
import { loadStorage } from '@/utils/storage'
import { API_BASE } from '@/config/api'

// --------------------------------------------------
// 在线 / 离线漫画数据源（支持 localStorage 持久化）
// --------------------------------------------------

/** 在线漫画列表（mock 兜底，正常由后端 /comics/online 接口驱动） */
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

/** 离线漫画列表（从后端 /comics/offline 拉取，本地持久化缓存） */
export const offlineComics = ref<OfflineComic[]>([])

/** 后端 /comics/offline 返回的原始行（tags 为 JSON 字符串） */
interface OfflineComicRaw {
  id: string
  title?: string
  tags?: unknown
  [key: string]: unknown
}

/** 从 Go 后端拉取全部离线漫画，并在源头把 tags 字符串解析为数组 */
export const fetchOfflineComics = async () => {
  try {
    const res = await fetch(`${API_BASE}/comics/offline`)
    if (res.ok) {
      const rawData = (await res.json()) as OfflineComicRaw[]
      offlineComics.value = rawData.map((item) => ({
        ...item,
        tags:
          typeof item.tags === 'string'
            ? JSON.parse((item.tags as string) || '[]')
            : item.tags || [],
      })) as OfflineComic[]
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

// --------------------------------------------------
// 阅读统计与排行榜
// --------------------------------------------------

/** 记录离线漫画阅读频次自增（详情页阅读按钮调用） */
export const recordComicClick = (comicId: string) => {
  const comic = offlineComics.value.find((c) => c.id === comicId)
  if (comic) {
    comic.readCount = (comic.readCount || 0) + 1
  }
}

/** 本地排行榜计算属性：按阅读次数降序 */
export const rankedOfflineComics = computed(() => {
  return [...offlineComics.value].sort((a, b) => (b.readCount || 0) - (a.readCount || 0))
})
