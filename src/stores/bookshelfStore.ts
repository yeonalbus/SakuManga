/**
 * 本地书架 Store：书架 CRUD 与动态作品数量统计
 * 由原 appStore 拆分而来
 */
import { ref, watch, computed } from 'vue'
import type { Bookshelf } from '@/types/comic'
import { loadStorage } from '@/utils/storage'
import { offlineComics } from './comicStore'

const defaultBookshelves: Bookshelf[] = [
  { id: 'shelf-1', name: '热血必看', count: 12, comicIds: [] },
  { id: 'shelf-2', name: '纯爱战神', count: 5, comicIds: [] },
  { id: 'shelf-3', name: '待分类本地本', count: 28, comicIds: [] },
]

/** 书架列表（持久化） */
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

/** 动态计算书架真实作品数量（基于离线漫画归属） */
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
