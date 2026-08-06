/**
 * 漫画数据 Store：在线/离线漫画列表、本地数据拉取与阅读统计
 * 由原 appStore 拆分而来，负责漫画相关数据源与派生计算
 */
import { ref, watch, computed } from 'vue'
import type { OnlineComic, OfflineComic } from '@/types/comic'
import { http } from '@/utils/request'
import { offlineReadingList } from '@/stores/readingStore'
// 使用命名空间导入 bookshelfStore / historyStore，避免与本文件产生「值初始化时序」上的循环依赖问题
import * as bookshelfStore from '@/stores/bookshelfStore'
import * as historyStore from '@/stores/historyStore'

// --------------------------------------------------
// 在线 / 离线漫画数据源（在线列表由后端 /comics/online 接口驱动）
// --------------------------------------------------

/**
 * 在线漫画列表。
 * 注意：在线列表的实际数据源是 onlineStore（经 /comics/online 接口加载），
 * 本 ref 仅用于收藏状态联动（见 historyStore.syncOnlineFavorite），不再作为页面数据源。
 */
export const onlineComics = ref<OnlineComic[]>([])

/** 离线漫画列表（从后端 /comics/offline 拉取，本地持久化缓存） */
export const offlineComics = ref<OfflineComic[]>([])

/** 后端 /comics/offline 返回的原始行（tags 为 TagItem 对象数组） */
interface OfflineComicRaw {
  id: string
  title?: string
  tags?: unknown
  tagRaws?: string[]
  [key: string]: unknown
}

/** 从 Go 后端拉取全部离线漫画，并在源头把 tags 字符串解析为数组 */
export const fetchOfflineComics = async () => {
  try {
    const rawData = await http<OfflineComicRaw[]>('/comics/offline')
    offlineComics.value = rawData.map((item) => {
      const tagItems = Array.isArray(item.tags)
        ? (item.tags as Array<{ name?: string; key?: string; namespace?: string }>)
        : []
      return {
        ...item,
        // 需求2：tags 归一化为「翻译名」字符串数组，供卡片展示与 tag 搜索
        tags:
          tagItems.length > 0
            ? tagItems.map((t) => (t && (t.name || t.key)) || '').filter(Boolean)
            : typeof item.tags === 'string'
              ? JSON.parse((item.tags as string) || '[]')
              : [],
        // 需求2：原始 tag 串（namespace:key，_ 归一为空格并小写），供 tag 搜索/语言过滤精确匹配
        tagRaws:
          tagItems.length > 0
            ? tagItems
                .map((t) => {
                  const key = ((t && t.key) || '').replace(/_/g, ' ').toLowerCase()
                  if (!key) return ''
                  return `${(t && t.namespace) || 'other'}:${key}`
                })
                .filter(Boolean)
            : Array.isArray(item.tagRaws)
              ? item.tagRaws.map((r) => r.replace(/_/g, ' ').toLowerCase())
              : [],
      } as OfflineComic
    })
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

// --------------------------------------------------
// 本地画廊删除
// --------------------------------------------------

/**
 * 删除本地画廊（记录 + 可选物理文件）。
 * 同步清理前端状态：离线列表、离线阅读清单、本地书架引用
 * （后端会同步清理 DB 中的书架与历史记录引用）。
 * 返回删除成功的数量。
 */
export const deleteOfflineComics = async (ids: string[], deleteFile = false): Promise<number> => {
  let okCount = 0
  for (const id of ids) {
    try {
      await http(`/comics/${id}?deleteFile=${deleteFile ? 'true' : 'false'}`, { method: 'DELETE' })
      okCount++
    } catch (err) {
      console.error('删除离线漫画失败:', err)
    }
  }

  if (okCount > 0) {
    const removedIds = new Set(ids)
    // 1. 离线列表
    offlineComics.value = offlineComics.value.filter((c) => !removedIds.has(c.id))
    // 2. 离线阅读清单
    offlineReadingList.value = offlineReadingList.value.filter((c) => !removedIds.has(c.id))
    // 3. 本地书架引用（comicIds）
    for (const shelf of bookshelfStore.bookshelves.value) {
      if (shelf.comicIds?.length) {
        shelf.comicIds = shelf.comicIds.filter((cid) => !removedIds.has(cid))
      }
    }
    // 4. 本地离线历史（localStorage 存储）
    historyStore.offlineHistoryList.value = historyStore.offlineHistoryList.value.filter(
      (h) => !removedIds.has(h.comic.id),
    )
  }
  return okCount
}
