/**
 * 本地书架 Store：书架 CRUD 与动态作品数量统计
 * 持久化迁移到后端 /bookshelves API（按登录用户隔离），本地仅保留内存态。
 * 首次登录时会把旧 localStorage（app_bookshelves）数据迁移到后端。
 */
import { ref, computed } from 'vue'
import type { Bookshelf } from '@/types/comic'
import { http } from '@/utils/request'
import { loadStorage } from '@/utils/storage'

/** 后端 Bookshelf 记录结构 */
interface BookshelfDTO {
  id: string
  name: string
  count?: number
  comicIds?: string[]
  createdAt?: string
  updatedAt?: string
}

/** 书架列表（内存态，由后端驱动） */
export const bookshelves = ref<Bookshelf[]>([])

/** 从后端加载当前用户的书架 */
export const loadBookshelves = async () => {
  try {
    const data = await http<{ bookshelves: BookshelfDTO[] }>('/bookshelves')
    bookshelves.value = (data.bookshelves || []).map((s) => ({
      id: s.id,
      name: s.name,
      count: s.count || 0,
      comicIds: s.comicIds || [],
    }))
  } catch (e) {
    // 后端不可用时回退旧 localStorage 数据，保证离线调试可用
    bookshelves.value = loadStorage('app_bookshelves', [])
    console.error('加载书架失败:', e)
  }
}

/**
 * 首次登录数据迁移：后端书架为空时，把旧 localStorage 书架逐个创建到后端。
 * 仅在没有后端数据时执行一次，迁移完成后清理旧缓存键。
 */
export const migrateLegacyBookshelves = async () => {
  try {
    if (bookshelves.value.length > 0) return
    const raw = localStorage.getItem('app_bookshelves')
    if (!raw) return
    const legacy = JSON.parse(raw) as Bookshelf[]
    if (!Array.isArray(legacy) || legacy.length === 0) return

    for (const shelf of legacy) {
      if (!shelf?.name) continue
      const created = await createBookshelf(shelf.name)
      if (created) {
        for (const cid of shelf.comicIds || []) {
          await addComicToShelf(created.id, cid)
        }
      }
    }
    localStorage.removeItem('app_bookshelves')
  } catch (e) {
    console.error('迁移书架失败:', e)
  }
}

/** 调后端创建书架并同步到本地列表 */
const createBookshelf = async (name: string): Promise<Bookshelf | null> => {
  try {
    const res = await http<{ data: BookshelfDTO }>('/bookshelves', {
      method: 'POST',
      body: JSON.stringify({ name }),
    })
    if (res?.data) {
      const shelf: Bookshelf = {
        id: res.data.id,
        name: res.data.name,
        count: 0,
        comicIds: [],
      }
      bookshelves.value.push(shelf)
      return shelf
    }
  } catch (e) {
    console.error('创建书架失败:', e)
  }
  return null
}

/** 新建书架 */
export const addBookshelf = async (name: string) => {
  if (!name.trim()) return
  await createBookshelf(name.trim())
}

/** 删除书架（乐观更新本地 + 后端） */
export const removeBookshelf = async (id: string) => {
  bookshelves.value = bookshelves.value.filter((b) => b.id !== id)
  try {
    await http(`/bookshelves/${id}`, { method: 'DELETE' })
  } catch (e) {
    console.error('删除书架失败:', e)
  }
}

/** 将作品加入书架（本地乐观更新 + 后端同步） */
export const addComicToShelf = async (shelfId: string, comicId: string) => {
  const shelf = bookshelves.value.find((b) => b.id === shelfId)
  if (shelf) {
    if (!shelf.comicIds) shelf.comicIds = []
    if (!shelf.comicIds.includes(comicId)) {
      shelf.comicIds.push(comicId)
      shelf.count = (shelf.count || 0) + 1
    }
  }
  try {
    await http(`/bookshelves/${shelfId}/comics`, {
      method: 'POST',
      body: JSON.stringify({ comicId }),
    })
  } catch (e) {
    console.error('加入书架失败:', e)
  }
}

/** 将作品移出书架（本地乐观更新 + 后端同步） */
export const removeComicFromShelf = async (shelfId: string, comicId: string) => {
  const shelf = bookshelves.value.find((b) => b.id === shelfId)
  if (shelf) {
    const before = (shelf.comicIds || []).length
    shelf.comicIds = (shelf.comicIds || []).filter((c) => c !== comicId)
    const removed = before - (shelf.comicIds || []).length
    if (removed > 0) {
      shelf.count = Math.max(0, (shelf.count || 0) - removed)
    }
  }
  try {
    await http(`/bookshelves/${shelfId}/comics?comicId=${comicId}`, { method: 'DELETE' })
  } catch (e) {
    console.error('移出书架失败:', e)
  }
}

/** 书架展示列表：数量优先使用后端实时 count，缺失时回退 comicIds 长度 */
export const computedBookshelves = computed(() => {
  return bookshelves.value.map((shelf) => ({
    ...shelf,
    count: shelf.count || shelf.comicIds?.length || 0,
  }))
})
