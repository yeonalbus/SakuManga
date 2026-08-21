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
  pinned?: boolean
  createdAt?: string
  updatedAt?: string
}

/** 书架列表（内存态，由后端驱动） */
export const bookshelves = ref<Bookshelf[]>([])

/**
 * 把后端/旧缓存返回的 comicIds 统一转为数组。
 * 后端历史版本可能返回 JSON 数组字符串（如 `["a","b"]`）而非数组，
 * 直接透传给上层会导致 .filter/.includes 抛错（comicIds.filter is not a function）。
 */
const toComicIdArray = (raw: unknown): string[] => {
  if (Array.isArray(raw)) return raw.filter((x): x is string => typeof x === 'string')
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      return Array.isArray(parsed)
        ? parsed.filter((x): x is string => typeof x === 'string')
        : []
    } catch {
      return []
    }
  }
  return []
}

/** 从后端加载当前用户的书架 */
export const loadBookshelves = async () => {
  try {
    const data = await http<{ bookshelves: BookshelfDTO[] }>('/bookshelves')
    bookshelves.value = (data.bookshelves || []).map((s) => ({
      id: s.id,
      name: s.name,
      count: s.count || 0,
      comicIds: toComicIdArray(s.comicIds),
      pinned: !!s.pinned,
    }))
  } catch (e) {
    // 后端不可用时回退旧 localStorage 数据，保证离线调试可用
    bookshelves.value = loadStorage<Bookshelf[]>('app_bookshelves', []).map((s) => ({
      id: s.id,
      name: s.name,
      count: s.count || 0,
      comicIds: toComicIdArray(s.comicIds),
    }))
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
        // 旧缓存 comicIds 可能为 JSON 数组字符串，统一转数组后再迁移
        for (const cid of toComicIdArray(shelf.comicIds)) {
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

/** 置顶/取消置顶书架（Round13，侧栏常驻高频；后端存标记，前端控制上限） */
export const setBookshelfPinned = async (id: string, pinned: boolean) => {
  const shelf = bookshelves.value.find((b) => b.id === id)
  if (shelf) shelf.pinned = pinned
  try {
    await http(`/bookshelves/${id}/pin`, {
      method: 'PUT',
      body: JSON.stringify({ pinned }),
    })
  } catch (e) {
    console.error('更新书架置顶失败:', e)
  }
}

/** 侧栏置顶书架列表（Round13）：pinned 的按全局 sort_order 顺序取前 5 个 */
export const PIN_LIMIT = 5
export const pinnedBookshelves = computed(() =>
  bookshelves.value.filter((b) => b.pinned).slice(0, PIN_LIMIT),
)

/** 批量将漫画加入书架（Round13：离线多选快捷加入；去重，返回 {added, skipped}） */
export const addComicsToShelf = async (shelfId: string, comicIds: string[]): Promise<{ added: number; skipped: number }> => {
  const unique = Array.from(new Set(comicIds.filter(Boolean)))
  const shelf = bookshelves.value.find((b) => b.id === shelfId)
  let added = 0
  let skipped = 0
  if (shelf && unique.length > 0) {
    if (!shelf.comicIds) shelf.comicIds = []
    for (const cid of unique) {
      if (shelf.comicIds.includes(cid)) skipped++
      else {
        shelf.comicIds.push(cid)
        added++
      }
    }
    if (added > 0) shelf.count = (shelf.count || 0) + added
  }
  try {
    const res = await http<{ added?: number; skipped?: number }>(`/bookshelves/${shelfId}/comics/batch`, {
      method: 'POST',
      body: JSON.stringify({ comicIds: unique }),
    })
    if (res?.added !== undefined) added = res.added
    if (res?.skipped !== undefined) skipped = res.skipped
  } catch (e) {
    console.error('批量加入书架失败:', e)
  }
  return { added, skipped }
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


/** 按自定义顺序整体重排书架内项目（Round10，PUT /bookshelves/:id/order） */
export const reorderShelfComics = async (shelfId: string, comicIds: string[]) => {
  const shelf = bookshelves.value.find((b) => b.id === shelfId)
  if (shelf) {
    shelf.comicIds = [...comicIds]
    shelf.count = comicIds.length
  }
  try {
    await http(`/bookshelves/${shelfId}/order`, {
      method: 'PUT',
      body: JSON.stringify({ comicIds }),
    })
  } catch (e) {
    console.error('保存书架排序失败:', e)
  }
}

/** 重命名书架（Round10，PUT /bookshelves/:id） */
export const renameBookshelf = async (id: string, name: string) => {
  const trimmed = name.trim()
  if (!trimmed) return
  const shelf = bookshelves.value.find((b) => b.id === id)
  if (shelf) shelf.name = trimmed
  try {
    await http(`/bookshelves/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ name: trimmed }),
    })
  } catch (e) {
    console.error('重命名书架失败:', e)
  }
}

/** 按自定义顺序批量重排书架列表（Round10，POST /bookshelves/reorder） */
export const reorderBookshelves = async (ids: string[]) => {
  const orderMap = new Map(ids.map((id, i) => [id, i]))
  bookshelves.value = [...bookshelves.value].sort((a, b) => {
    const ia = orderMap.get(a.id) ?? Number.MAX_SAFE_INTEGER
    const ib = orderMap.get(b.id) ?? Number.MAX_SAFE_INTEGER
    return ia - ib
  })
  try {
    await http('/bookshelves/reorder', {
      method: 'POST',
      body: JSON.stringify({ ids }),
    })
  } catch (e) {
    console.error('保存书架顺序失败:', e)
  }
}

/** 上移/下移单个书架（Round10，侧栏 ↑/↓） */
export const moveBookshelf = async (id: string, dir: -1 | 1) => {
  const idx = bookshelves.value.findIndex((b) => b.id === id)
  const newIdx = idx + dir
  if (idx < 0 || newIdx < 0 || newIdx >= bookshelves.value.length) return
  const arr = [...bookshelves.value]
  const [item] = arr.splice(idx, 1)
  arr.splice(newIdx, 0, item)
  await reorderBookshelves(arr.map((b) => b.id))
}

/** 书架展示列表：数量优先使用后端实时 count，缺失时回退 comicIds 长度 */
export const computedBookshelves = computed(() => {
  return bookshelves.value.map((shelf) => ({
    ...shelf,
    count: shelf.count || shelf.comicIds?.length || 0,
  }))
})
