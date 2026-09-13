/**
 * 本地书架 Store：书架 CRUD 与动态作品数量统计
 * 持久化迁移到后端 /bookshelves API（按登录用户隔离），本地仅保留内存态。
 * 首次登录时会把旧 localStorage（app_bookshelves）数据迁移到后端。
 * Round22：书架列表 / 书架内本子采用 LexoRank 浮点权值排序（单点移动只更新一项权值，
 * 防抖合并持久化；精度用尽时异步全量重置）。
 */
import { ref, computed } from 'vue'
import type { Bookshelf } from '@/types/comic'
import { http } from '@/utils/request'
import { loadStorage } from '@/utils/storage'
import { between, needsReweight, reweightAll, orderByWeights } from '@/utils/lexoRank'

/** 后端 Bookshelf 记录结构 */
interface BookshelfDTO {
  id: string
  name: string
  count?: number
  comicIds?: string[]
  pinned?: boolean
  sortKey?: number
  sortKeys?: Record<string, number>
  /** Round38：后端聚合的架内未读数（read_count<=0 且本子仍存在） */
  unreadCount?: number
  /** Round38：封面地址（手指定优先，否则展示顺序第一本） */
  coverUrl?: string
  /** Round38-R5：手动指定的封面本子 id（空 = 自动） */
  coverComicId?: string
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

/** 后端 DTO → 本地 Bookshelf（Round38：透传未读数与封面） */
const mapShelfDTO = (s: BookshelfDTO): Bookshelf => ({
  id: s.id,
  name: s.name,
  count: s.count || 0,
  comicIds: toComicIdArray(s.comicIds),
  pinned: !!s.pinned,
  sortKey: typeof s.sortKey === 'number' ? s.sortKey : 0,
  sortKeys: s.sortKeys && typeof s.sortKeys === 'object' ? s.sortKeys : {},
  unreadCount: typeof s.unreadCount === 'number' ? s.unreadCount : undefined,
  coverUrl: typeof s.coverUrl === 'string' ? s.coverUrl : undefined,
  coverComicId: typeof s.coverComicId === 'string' ? s.coverComicId : '',
})

/** 从后端加载当前用户的书架 */
export const loadBookshelves = async () => {
  try {
    const data = await http<{ bookshelves: BookshelfDTO[] }>('/bookshelves')
    bookshelves.value = (data.bookshelves || []).map(mapShelfDTO)
  } catch (e) {
    // 后端不可用时回退旧 localStorage 数据，保证离线调试可用
    bookshelves.value = loadStorage<Bookshelf[]>('app_bookshelves', []).map((s) => ({
      id: s.id,
      name: s.name,
      count: s.count || 0,
      comicIds: toComicIdArray(s.comicIds),
      sortKey: 0,
      sortKeys: {},
    }))
    console.error('加载书架失败:', e)
  }
}

/**
 * Round38：静默刷新书架列表（书架墙挂载 / 加入移出本子后的统计校正）。
 * 与 loadBookshelves 的区别：请求失败时**保留现有数据**，不回退旧 localStorage，
 * 避免一次网络抖动把已加载的书架清空。
 */
export const refreshBookshelves = async () => {
  // Round38：先冲刷未持久化的排序改动，避免刷新回来的旧权值覆盖本地顺序
  flushPendingSort()
  try {
    const data = await http<{ bookshelves: BookshelfDTO[] }>('/bookshelves')
    bookshelves.value = (data.bookshelves || []).map(mapShelfDTO)
  } catch (e) {
    console.error('刷新书架失败:', e)
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
        // Round38：新书架为空架，未读 0、无封面
        unreadCount: 0,
        coverUrl: '',
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
  // Round38：静默刷新未读/封面统计（新加入的本子通常未读，徽标需即时反映）
  void refreshBookshelves()
}

/** 置顶/取消置顶书架（Round13，侧栏常驻高频；后端存标记，前端控制上限） */export const setBookshelfPinned = async (id: string, pinned: boolean) => {
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

/**
 * Round38-R5：设置/清除书架封面（comicId 传空串 = 恢复自动封面「架内第一本」）。
 * coverUrl 由后端按「手指定优先」解析，故写入后刷新一次书架列表拿到新封面地址。
 */
export const setBookshelfCover = async (id: string, comicId: string) => {
  const shelf = bookshelves.value.find((b) => b.id === id)
  if (shelf) shelf.coverComicId = comicId
  try {
    await http(`/bookshelves/${id}/cover`, {
      method: 'PUT',
      body: JSON.stringify({ comicId }),
    })
  } catch (e) {
    console.error('设置书架封面失败:', e)
  }
  await refreshBookshelves()
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
  // Round38：有实际新增时刷新未读/封面统计
  if (added > 0) void refreshBookshelves()
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
  // Round38：移出后刷新未读/封面统计（被移出的本子可能未读，封面也可能需要回退到下一本）
  void refreshBookshelves()
}


// ══════════════════════════════════════════════════════════════
// Round22：LexoRank 排序（书架列表 sortKey / 书架内本子 sortKeys）
// 单点移动只更新一项权值并防抖合并持久化；精度用尽时异步全量重置 1000*i。
// ══════════════════════════════════════════════════════════════

/** 书架内本子展示顺序：按权值升序（有权值在前），无权值项按 comicIds 数组顺序排后 */
export const orderedShelfComicIds = (shelf: Bookshelf | undefined): string[] => {
  const ids = shelf?.comicIds || []
  return orderByWeights(ids, (id) => shelf?.sortKeys?.[id])
}

/**
 * 确保书架内所有本子都有权值（旧数据惰性迁移）。
 * 存在无权值项时本地赋 1000*i（保持当前展示顺序不变）并全量持久化一次。
 */
export const ensureShelfComicWeights = async (shelfId: string) => {
  const shelf = bookshelves.value.find((b) => b.id === shelfId)
  if (!shelf || !shelf.comicIds || shelf.comicIds.length === 0) return
  if (!shelf.sortKeys) shelf.sortKeys = {}
  const hasMissing = shelf.comicIds.some((id) => shelf.sortKeys?.[id] === undefined)
  if (!hasMissing) return
  const { weights } = reweightAll(shelf.comicIds)
  shelf.sortKeys = Object.fromEntries(weights)
  try {
    await http(`/bookshelves/${shelfId}/order`, {
      method: 'PUT',
      body: JSON.stringify({ comicIds: [...shelf.comicIds] }),
    })
  } catch (e) {
    console.error('初始化书架权值失败:', e)
  }
}

// ── 书架列表（sortKey）防抖持久化 ──
let shelfFlushTimer: ReturnType<typeof setTimeout> | null = null
const pendingShelfPositions = new Map<string, number>()

const scheduleShelfPositionFlush = (shelfId: string, weight: number) => {
  pendingShelfPositions.set(shelfId, weight)
  if (shelfFlushTimer) clearTimeout(shelfFlushTimer)
  shelfFlushTimer = setTimeout(() => {
    void flushShelfPositions()
  }, 300)
}

const flushShelfPositions = async () => {
  shelfFlushTimer = null
  const pending = new Map(pendingShelfPositions)
  pendingShelfPositions.clear()
  for (const [shelfId, weight] of pending) {
    try {
      await http(`/bookshelves/${shelfId}/position`, {
        method: 'PUT',
        body: JSON.stringify({ sortKey: weight }),
      })
    } catch (e) {
      console.error('保存书架顺序失败:', e)
    }
  }
}

// ── 书架内本子（sortKeys）防抖持久化 ──
let comicFlushTimer: ReturnType<typeof setTimeout> | null = null
const pendingComicWeights = new Map<string, Map<string, number>>()

const scheduleComicWeightsFlush = (shelfId: string, comicId: string, weight: number) => {
  if (!pendingComicWeights.has(shelfId)) pendingComicWeights.set(shelfId, new Map())
  pendingComicWeights.get(shelfId)!.set(comicId, weight)
  if (comicFlushTimer) clearTimeout(comicFlushTimer)
  comicFlushTimer = setTimeout(() => {
    void flushComicWeights()
  }, 300)
}

const flushComicWeights = async () => {
  comicFlushTimer = null
  const pending = new Map(pendingComicWeights)
  pendingComicWeights.clear()
  for (const [shelfId, weights] of pending) {
    try {
      await http(`/bookshelves/${shelfId}/order`, {
        method: 'PUT',
        body: JSON.stringify({ sortKeys: Object.fromEntries(weights) }),
      })
    } catch (e) {
      console.error('保存书架排序失败:', e)
    }
  }
}

/** 立即冲刷未持久化的排序改动（离开页面 / 完成排序时调用） */
export const flushPendingSort = () => {
  if (shelfFlushTimer) {
    clearTimeout(shelfFlushTimer)
    void flushShelfPositions()
  }
  if (comicFlushTimer) {
    clearTimeout(comicFlushTimer)
    void flushComicWeights()
  }
}

/**
 * 把书架移动到列表第 target 位（0-based），返回新权值；null 表示触发全量重置（无需再单点持久化）。
 */
const applyShelfMove = (id: string, target: number): number | null => {
  const arr = [...bookshelves.value]
  const idx = arr.findIndex((b) => b.id === id)
  if (idx < 0 || target === idx) return null
  const [item] = arr.splice(idx, 1)
  arr.splice(target, 0, item)
  const prevW = target > 0 ? arr[target - 1].sortKey : undefined
  const nextW = target < arr.length - 1 ? arr[target + 1].sortKey : undefined
  if (prevW !== undefined && nextW !== undefined && needsReweight(prevW, nextW)) {
    // 精度用尽：按新顺序全量重置 1000*i（该项天然落在目标位），异步持久化一次
    const { weights } = reweightAll(arr.map((b) => b.id))
    arr.forEach((b) => {
      b.sortKey = weights.get(b.id) ?? 0
    })
    bookshelves.value = arr
    void reorderBookshelves(arr.map((b) => b.id))
    return null
  }
  const w = between(prevW, nextW)
  item.sortKey = w
  bookshelves.value = arr
  return w
}

/** 把书架移动到第 position 位（1-based） */
export const moveShelfToPosition = async (id: string, position: number) => {
  const target = Math.max(0, Math.min(position - 1, Math.max(0, bookshelves.value.length - 1)))
  const w = applyShelfMove(id, target)
  if (w !== null) scheduleShelfPositionFlush(id, w)
}

/** 书架列表「移到顶部」（第 1 位；与 Round13「置顶到侧栏」pinned 语义区分） */
export const moveShelfToTop = async (id: string) => {
  await moveShelfToPosition(id, 1)
}

/**
 * 把书架内本子移动到第 position 位（1-based，相对该书架展示顺序）。
 * 仅更新该本子的 LexoRank 权值，防抖持久化。
 */
export const moveComicToPosition = async (shelfId: string, comicId: string, position: number) => {
  await ensureShelfComicWeights(shelfId)
  const shelf = bookshelves.value.find((b) => b.id === shelfId)
  if (!shelf) return
  const ids = orderedShelfComicIds(shelf)
  const target = Math.max(0, Math.min(position - 1, Math.max(0, ids.length - 1)))
  const idx = ids.indexOf(comicId)
  if (idx < 0 || target === idx) return
  const arr = [...ids]
  arr.splice(idx, 1)
  arr.splice(target, 0, comicId)
  const prevW = target > 0 ? shelf.sortKeys?.[arr[target - 1]] : undefined
  const nextW = target < arr.length - 1 ? shelf.sortKeys?.[arr[target + 1]] : undefined
  if (prevW !== undefined && nextW !== undefined && needsReweight(prevW, nextW)) {
    // 精度用尽：按新顺序全量重置 1000*i 并持久化一次
    const { weights } = reweightAll(arr)
    shelf.sortKeys = Object.fromEntries(weights)
    shelf.comicIds = arr
    void reorderShelfComics(shelfId, arr)
    return
  }
  const w = between(prevW, nextW)
  shelf.sortKeys = { ...shelf.sortKeys, [comicId]: w }
  shelf.comicIds = arr
  scheduleComicWeightsFlush(shelfId, comicId, w)
}

/** 书架内本子「置顶」（移到第 1 位） */
export const moveComicToTop = async (shelfId: string, comicId: string) => {
  await moveComicToPosition(shelfId, comicId, 1)
}

/** 按自定义顺序整体重排书架内项目（Round10 全量模式；Round22 精度用尽全量重置复用） */
export const reorderShelfComics = async (shelfId: string, comicIds: string[]) => {
  const shelf = bookshelves.value.find((b) => b.id === shelfId)
  const { weights } = reweightAll(comicIds)
  if (shelf) {
    shelf.comicIds = [...comicIds]
    shelf.count = comicIds.length
    shelf.sortKeys = Object.fromEntries(weights)
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

/** 按自定义顺序批量重排书架列表（Round10 全量模式；Round22 全量重置同步重建 sortKey=1000*i） */
export const reorderBookshelves = async (ids: string[]) => {
  const orderMap = new Map(ids.map((id, i) => [id, i]))
  bookshelves.value = [...bookshelves.value].sort((a, b) => {
    const ia = orderMap.get(a.id) ?? Number.MAX_SAFE_INTEGER
    const ib = orderMap.get(b.id) ?? Number.MAX_SAFE_INTEGER
    return ia - ib
  })
  bookshelves.value.forEach((b, i) => {
    b.sortKey = (i + 1) * 1000
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

/** 批量将漫画移出书架（Round22，多选快捷移除；同步清理权值，不删本地文件/历史） */
export const removeComicsFromShelf = async (shelfId: string, comicIds: string[]) => {
  const set = new Set(comicIds.filter(Boolean))
  const shelf = bookshelves.value.find((b) => b.id === shelfId)
  if (shelf) {
    const before = (shelf.comicIds || []).length
    shelf.comicIds = (shelf.comicIds || []).filter((c) => !set.has(c))
    const removed = before - (shelf.comicIds || []).length
    if (removed > 0) shelf.count = Math.max(0, (shelf.count || 0) - removed)
    if (shelf.sortKeys) {
      for (const id of set) delete shelf.sortKeys[id]
    }
  }
  try {
    await http(`/bookshelves/${shelfId}/comics/batch`, {
      method: 'DELETE',
      body: JSON.stringify({ comicIds: [...set] }),
    })
  } catch (e) {
    console.error('批量移出书架失败:', e)
  }
  // Round38：批量移出后刷新未读/封面统计
  void refreshBookshelves()
}

/** 书架展示列表：数量优先使用后端实时 count，缺失时回退 comicIds 长度 */
export const computedBookshelves = computed(() => {
  return bookshelves.value.map((shelf) => ({
    ...shelf,
    count: shelf.count || shelf.comicIds?.length || 0,
  }))
})
