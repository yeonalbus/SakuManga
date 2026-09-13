/**
 * 搜刮书签 Store（Round27 原版 localStorage → Round28 后端化，多端同步）
 *
 * 数据源：后端 SQLite（按登录用户隔离，与历史/书架/评分同模式）。
 * - 初始化：loadScrapeBookmarks()（登录后/会话恢复时由 loadUserLibrary 统一调用）
 * - 增删改：乐观更新 + 失败回滚 + toast 提示
 * - 旧数据迁移：localStorage 有旧书签且后端为空 → 逐个上传后清空本地
 * - 后端不可用降级：只读展示 localStorage 缓存，写操作拒绝并提示
 *
 * 数据安全（BUG2 教训）：恢复校验「宽松兜底」而非「静默丢弃」——
 * 字段缺失/类型异常用默认值补齐，绝不让一条书签因结构小问题悄悄消失。
 */
import { ref, computed } from 'vue'
import type {
  ScrapeBookmark,
  ScrapeBookmarkCheckResult,
  ScrapeBookmarkInvalidKind,
  SearchConfig,
} from '@/types/comic'
import { onlineSearchConfig } from '@/stores/searchStore'
import { loadStorage } from '@/utils/storage'
import { http } from '@/utils/request'
import { useUI } from '@/composables/useUI'

const STORAGE_KEY = 'saku_scrape_bookmarks'

const { toast } = useUI()

// ─── 状态 ───

/** 书签列表（后端驱动，内存态；变化时不再自动落 localStorage） */
export const scrapeBookmarks = ref<ScrapeBookmark[]>([])

/** 是否已完成一次加载（成功或降级） */
export const bookmarkLoaded = ref(false)

/** 后端不可用标记：true 时只读展示 localStorage 缓存，写操作拒绝 */
export const bookmarkSyncFailed = ref(false)

// ─── 宽松恢复（BUG2 修复：字段兜底，不静默丢弃）───

const genBookmarkId = (): string =>
  `bm_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`

/** 宽松恢复 config：逐字段兜底（缺失/类型异常用默认值），不因个别字段异常丢弃整条 */
const restoreConfig = (raw: unknown): SearchConfig => {
  if (!raw || typeof raw !== 'object') return {} as SearchConfig
  const r = raw as Partial<SearchConfig>
  return {
    ...(r as SearchConfig),
    activeCategories: Array.isArray(r.activeCategories) ? r.activeCategories : [],
    keywords: Array.isArray(r.keywords) ? r.keywords : [],
    keyword: typeof r.keyword === 'string' ? r.keyword : '',
    language: typeof r.language === 'string' ? r.language : 'All',
    minRating: typeof r.minRating === 'number' ? r.minRating : 0,
    excludeTags: Array.isArray(r.excludeTags) ? r.excludeTags : [],
    excludeKeywords: Array.isArray(r.excludeKeywords) ? r.excludeKeywords : [],
  }
}

/** 宽松恢复失效标记（Round33；Round35 增加 unreachable） */
const restoreInvalid = (raw: unknown): NonNullable<ScrapeBookmark['anchor']>['invalid'] => {
  if (!raw || typeof raw !== 'object') return null
  const o = raw as Record<string, unknown>
  const kind = o.kind
  if (kind !== 'unreachable' && kind !== 'removed' && kind !== 'copyright' && kind !== 'invalid') {
    return null
  }
  return { kind, at: typeof o.at === 'number' ? o.at : Date.now() }
}

/** 宽松恢复迁移来源（Round33） */
const restoreMigratedFrom = (
  raw: unknown,
): NonNullable<ScrapeBookmark['anchor']>['migratedFrom'] => {
  if (!raw || typeof raw !== 'object') return null
  const o = raw as Record<string, unknown>
  if (typeof o.gid !== 'string') return null
  return {
    gid: o.gid,
    token: typeof o.token === 'string' ? o.token : undefined,
    title: typeof o.title === 'string' ? o.title : undefined,
    postedAt: typeof o.postedAt === 'string' ? o.postedAt : undefined,
  }
}

/** 宽松恢复 anchor：gid 非字符串视为未锚定（null），其余字段可缺省 */
const restoreAnchor = (raw: unknown): ScrapeBookmark['anchor'] => {
  if (raw === null || raw === undefined) return null
  if (typeof raw !== 'object') return null
  const a = raw as Record<string, unknown>
  if (typeof a.gid !== 'string') return null
  return {
    gid: a.gid,
    token: typeof a.token === 'string' ? a.token : undefined,
    title: typeof a.title === 'string' ? a.title : undefined,
    postedAt: typeof a.postedAt === 'string' ? a.postedAt : undefined,
    // Round33：失效标记 / 迁移来源 / 列表位置（宽松透传，不丢字段）
    invalid: restoreInvalid(a.invalid),
    migratedFrom: restoreMigratedFrom(a.migratedFrom),
    listIndex: typeof a.listIndex === 'number' ? a.listIndex : undefined,
  }
}

/** 任意来源（后端 DTO / 旧 localStorage）→ ScrapeBookmark：字段全部兜底 */
const fromRaw = (raw: unknown): ScrapeBookmark | null => {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Record<string, unknown>
  return {
    id: typeof r.id === 'string' || typeof r.id === 'number' ? String(r.id) : genBookmarkId(),
    // Round29：名称允许空串（留空时侧栏改展示「位置 + 发布时间」）
    name: typeof r.name === 'string' ? r.name : '',
    type: r.type === 'search' ? 'search' : 'home',
    keyword: typeof r.keyword === 'string' ? r.keyword : '',
    config: restoreConfig(r.config),
    anchor: restoreAnchor(r.anchor),
    createdAt: typeof r.createdAt === 'number' ? r.createdAt : Date.now(),
  }
}

const restoreList = (raw: unknown): ScrapeBookmark[] =>
  Array.isArray(raw) ? raw.map(fromRaw).filter((b): b is ScrapeBookmark => b !== null) : []

// ─── 派生状态（不变）───

/** 全部书签锚定的 gid 集合（供 ItemCard 常驻角标判定） */
export const bookmarkedGids = computed<Set<string>>(() => {
  const set = new Set<string>()
  for (const bm of scrapeBookmarks.value) {
    if (bm.anchor?.gid) set.add(bm.anchor.gid)
  }
  return set
})

/** 某 gid 是否被任一书签锚定 */
export const isGidBookmarked = (gid: string): boolean => bookmarkedGids.value.has(gid)

/** 按 id 取书签（跳转恢复用；不存在返回 undefined） */
export const getBookmarkById = (id: string): ScrapeBookmark | undefined =>
  scrapeBookmarks.value.find((b) => b.id === id)

// ─── 锚点失效标记（Round28：BUG2 修复）───

/**
 * 本会话内已确认「锚定画廊失效」的 gid 集合（跳转后列表加载完仍定位不到锚点）。
 * 侧栏据此给对应书签打 ⚠️ 标记；仅会话级，不持久化（失效状态随下次跳转重新判定）。
 */
export const failedAnchorGids = ref<Set<string>>(new Set())

/** 标记某 gid 的锚定画廊失效（跳转失败时调用） */
export const markAnchorFailed = (gid: string): void => {
  if (!gid) return
  failedAnchorGids.value = new Set(failedAnchorGids.value).add(gid)
}

/**
 * 清除某 gid 的会话级失效标记（Round34：书签定位成功后调用）。
 * 此前只写不清，导致「一次定位失败 → 侧栏 ⚠️ 永久残留」。
 */
export const clearAnchorFailed = (gid: string): void => {
  if (!gid || !failedAnchorGids.value.has(gid)) return
  const next = new Set(failedAnchorGids.value)
  next.delete(gid)
  failedAnchorGids.value = next
}

// ─── 展示辅助（Round29：位置 + 发布时间）───

/**
 * 书签位置标签：`搜索: <词>` 或 `首页`。
 * 供侧栏/弹窗展示「上次搜刮到了哪里」（页面上下文 = 搜索词 + 筛选状态）。
 */
export const bookmarkLocationLabel = (bm: Pick<ScrapeBookmark, 'type' | 'keyword'>): string => {
  const kw = (bm.keyword || '').trim()
  return bm.type === 'search' && kw ? `搜索: ${kw}` : '首页'
}

// ─── 深拷贝与快照（不变）───

/** 深拷贝 SearchConfig（纯 JSON 数据，JSON 往返安全）。快照与恢复共用同一实现 */
export const cloneSearchConfig = (cfg: SearchConfig): SearchConfig =>
  JSON.parse(JSON.stringify(cfg)) as SearchConfig

/** 快照当前在线搜索/筛选状态（创建书签时调用）。整体覆盖恢复，不走「搜索选项继承」 */
export const snapshotOnlineSearchConfig = (): SearchConfig =>
  cloneSearchConfig(onlineSearchConfig.value)

// ─── 后端动作 ───

/** 后端不可用时统一拒绝写操作 */
const rejectWhenOffline = (): boolean => {
  if (!bookmarkSyncFailed.value) return false
  toast.error('书签服务不可用，暂无法修改书签（仅可查看本地缓存）')
  return true
}

/**
 * 初始化加载（登录后 / 会话恢复统一调用）：
 * 后端拉取 → 迁移旧 localStorage 数据 → 失败降级只读本地缓存。
 */
export const loadScrapeBookmarks = async (): Promise<void> => {
  try {
    const data = await http<{ items?: unknown[] }>('/scrape-bookmarks')
    scrapeBookmarks.value = restoreList(data.items)
    bookmarkSyncFailed.value = false
    await migrateLegacyLocal()
  } catch (e) {
    console.error('加载书签失败，降级本地缓存:', e)
    bookmarkSyncFailed.value = true
    scrapeBookmarks.value = restoreList(loadStorage(STORAGE_KEY, null))
    if (scrapeBookmarks.value.length > 0) {
      toast.warning('书签同步失败，当前显示本地缓存')
    }
  } finally {
    bookmarkLoaded.value = true
  }
}

/** 旧 localStorage 书签迁移：后端为空 → 逐个上传后清本地；后端已有 → 丢弃本地旧缓存 */
const migrateLegacyLocal = async (): Promise<void> => {
  const legacy = restoreList(loadStorage(STORAGE_KEY, null))
  if (legacy.length === 0) {
    localStorage.removeItem(STORAGE_KEY)
    return
  }
  if (scrapeBookmarks.value.length > 0) {
    // 后端已有数据（如其他设备已同步）→ 本地旧缓存作废
    localStorage.removeItem(STORAGE_KEY)
    return
  }
  let failed = false
  for (const bm of legacy) {
    try {
      await http('/scrape-bookmarks', {
        method: 'POST',
        body: JSON.stringify({
          name: bm.name,
          type: bm.type,
          keyword: bm.keyword,
          config: bm.config,
          anchor: bm.anchor,
        }),
      })
    } catch {
      failed = true
      break
    }
  }
  if (failed) {
    toast.warning('旧书签迁移失败，稍后会自动重试')
    return
  }
  localStorage.removeItem(STORAGE_KEY)
  toast.success(`已同步 ${legacy.length} 条本地书签到云端`)
  // 重新拉取（拿到后端真实 id）
  try {
    const data = await http<{ items?: unknown[] }>('/scrape-bookmarks')
    scrapeBookmarks.value = restoreList(data.items)
  } catch {
    /* 迁移已成功，忽略刷新失败 */
  }
}

/** 新增书签：乐观插入 + 后端创建 + 失败回滚。成功返回真实书签，失败返回 null */
export const addScrapeBookmark = async (
  name: string,
  type: 'home' | 'search',
  keyword: string,
  config: SearchConfig,
  anchor: ScrapeBookmark['anchor'],
): Promise<ScrapeBookmark | null> => {
  const temp: ScrapeBookmark = {
    id: genBookmarkId(),
    // Round29：名称默认留空（不再自动生成「首页快照 HH:MM」）——留空时侧栏展示「位置 + 发布时间」
    name: name.trim(),
    type,
    keyword,
    config: cloneSearchConfig(config),
    anchor,
    createdAt: Date.now(),
  }
  // 降级模式：拒绝写操作
  if (rejectWhenOffline()) return null
  // 乐观插入
  scrapeBookmarks.value.push(temp)
  try {
    const res = await http<{ item?: unknown }>('/scrape-bookmarks', {
      method: 'POST',
      body: JSON.stringify({
        name: temp.name,
        type,
        keyword,
        config: cloneSearchConfig(config),
        anchor,
      }),
    })
    const real = fromRaw(res.item)
    if (real) {
      scrapeBookmarks.value = scrapeBookmarks.value.map((b) => (b.id === temp.id ? real : b))
      return real
    }
    return temp
  } catch (e) {
    scrapeBookmarks.value = scrapeBookmarks.value.filter((b) => b.id !== temp.id)
    console.error('创建书签失败:', e)
    toast.error('书签保存失败，请重试')
    return null
  }
}

/** 删除书签：乐观移除 + 后端删除 + 失败回滚。返回是否成功 */
export const removeScrapeBookmark = async (id: string): Promise<boolean> => {
  if (rejectWhenOffline()) return false
  const target = scrapeBookmarks.value.find((b) => b.id === id)
  if (!target) return false
  scrapeBookmarks.value = scrapeBookmarks.value.filter((b) => b.id !== id)
  try {
    await http(`/scrape-bookmarks/${encodeURIComponent(id)}`, { method: 'DELETE' })
    return true
  } catch (e) {
    scrapeBookmarks.value.push(target)
    scrapeBookmarks.value.sort((a, b) => a.createdAt - b.createdAt)
    console.error('删除书签失败:', e)
    toast.error('书签删除失败，请重试')
    return false
  }
}

/** 重命名书签（空名忽略）。返回是否成功 */
export const renameScrapeBookmark = async (id: string, name: string): Promise<boolean> => {
  const trimmed = name.trim()
  if (!trimmed) return false
  if (rejectWhenOffline()) return false
  const bm = scrapeBookmarks.value.find((b) => b.id === id)
  if (!bm) return false
  const prevName = bm.name
  bm.name = trimmed
  try {
    await http(`/scrape-bookmarks/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify({ name: trimmed }),
    })
    return true
  } catch (e) {
    bm.name = prevName
    console.error('重命名书签失败:', e)
    toast.error('书签重命名失败，请重试')
    return false
  }
}

// ─────────────────────────────────────────────────────────────
// Round33：失效检测 / 迁移 / 失效判定
// ─────────────────────────────────────────────────────────────

/** 书签是否已确认失效（检测后写入 invalid 标记） */
export const isBookmarkInvalid = (bm: ScrapeBookmark): boolean => !!bm.anchor?.invalid

/**
 * 失效原因文案（Round35：不细分原因）
 *
 * 语义：书签是「位置快照」——失效只意味着「在书签当时的搜索&筛选条件下已看不到锚定画廊，
 * 因此无法得知上次搜刮到的位置」，用户并不关心底层原因是删除/下架/被移除。
 */
export const invalidReasonText = (bm: ScrapeBookmark): string => {
  if (!bm.anchor?.invalid) return ''
  return '在当前搜索&筛选条件下已看不到锚定画廊（书签已失效）'
}

/** 全部失效书签 */
export const invalidBookmarks = computed<ScrapeBookmark[]>(() =>
  scrapeBookmarks.value.filter((b) => isBookmarkInvalid(b)),
)

/** 书签是否由迁移而来（带迁移来源） */
export const isBookmarkMigrated = (bm: ScrapeBookmark): boolean => !!bm.anchor?.migratedFrom?.gid

/**
 * 更新书签锚点（迁移 / 失效标记写回）：乐观更新 + 后端 PUT + 失败回滚。
 * anchor 原样序列化（保留 invalid / migratedFrom / listIndex 等扩展字段）。
 */
export const updateBookmarkAnchor = async (
  id: string,
  anchor: ScrapeBookmark['anchor'],
): Promise<boolean> => {
  if (rejectWhenOffline()) return false
  const bm = scrapeBookmarks.value.find((b) => b.id === id)
  if (!bm) return false
  const prev = bm.anchor ? JSON.parse(JSON.stringify(bm.anchor)) : null
  bm.anchor = anchor ? JSON.parse(JSON.stringify(anchor)) : null
  try {
    await http(`/scrape-bookmarks/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify({ anchor }),
    })
    return true
  } catch (e) {
    bm.anchor = prev
    console.error('书签锚点更新失败:', e)
    toast.error('书签锚点更新失败，请重试')
    return false
  }
}

/**
 * 批量失效检测：ids 为空 = 检测全部。
 *
 * Round35：后端改为「按各书签原始搜索&筛选条件复刻检索 + 动态越过判定」，
 * 因此单次检测可能翻若干页列表（每条书签耗时取决于当日画廊密度）——
 * 这里给足超时（10 分钟），避免被默认 60s 超时打断而误报检测失败。
 */
export const checkScrapeBookmarks = async (
  ids?: string[],
): Promise<ScrapeBookmarkCheckResult[]> => {
  const numeric = (ids ?? []).map((i) => Number(i)).filter((n) => Number.isFinite(n))
  const res = await http<{ results?: ScrapeBookmarkCheckResult[] }>('/scrape-bookmarks/check', {
    method: 'POST',
    body: JSON.stringify(ids && ids.length > 0 ? { ids: numeric } : {}),
    signal: AbortSignal.timeout(10 * 60 * 1000),
  })
  return res.results || []
}

/**
 * 应用检测结果到书签（写入失效标记）。
 * replaced / ok 由调用方按策略处理（自动精确迁移 / 刷新元信息），此处只处理判定性状态。
 */
export const markBookmarkInvalid = async (
  id: string,
  kind: ScrapeBookmarkInvalidKind,
): Promise<boolean> => {
  const bm = scrapeBookmarks.value.find((b) => b.id === id)
  if (!bm || !bm.anchor) return false
  return updateBookmarkAnchor(id, {
    ...bm.anchor,
    invalid: { kind, at: Date.now() },
  })
}

/** 清除失效标记（检测恢复正常时） */
export const clearBookmarkInvalid = async (id: string): Promise<boolean> => {
  const bm = scrapeBookmarks.value.find((b) => b.id === id)
  if (!bm || !bm.anchor || !bm.anchor.invalid) return true
  const next = { ...bm.anchor }
  delete next.invalid
  return updateBookmarkAnchor(id, next)
}

/** 一键清理全部失效书签，返回删除数量 */
export const clearInvalidBookmarks = async (): Promise<number> => {
  const targets = invalidBookmarks.value.map((b) => b.id)
  let removed = 0
  for (const id of targets) {
    if (await removeScrapeBookmark(id)) removed++
  }
  return removed
}
