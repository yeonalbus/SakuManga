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
  ScrapeBookmarkProbeResult,
  ScrapeBookmarkRole,
  SearchConfig,
} from '@/types/comic'
import { onlineSearchConfig } from '@/stores/searchStore'
import { isSameOnlineScope } from '@/utils/bookmarkScope'
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
    // Round49：锚定画廊封面（书签页名片用；老书签无此字段 → undefined）
    coverUrl: typeof a.coverUrl === 'string' ? a.coverUrl : undefined,
    // Round33：失效标记 / 迁移来源 / 列表位置（宽松透传，不丢字段）
    invalid: restoreInvalid(a.invalid),
    migratedFrom: restoreMigratedFrom(a.migratedFrom),
    listIndex: typeof a.listIndex === 'number' ? a.listIndex : undefined,
  }
}

/** Round49：归一化书签标记（仅 newest/oldest/interest，其余归 interest） */
const normalizeRole = (raw: unknown): ScrapeBookmarkRole => {
  if (raw === 'newest' || raw === 'oldest' || raw === 'interest') return raw
  return 'interest'
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
    // Round37：排序权值（老数据 0 / 缺失 → 0，排序时按原列表顺序兜底）
    sortKey: typeof r.sortKey === 'number' && Number.isFinite(r.sortKey) ? r.sortKey : 0,
    createdAt: typeof r.createdAt === 'number' ? r.createdAt : Date.now(),

    // Round49：书签项配置兜底（老数据缺字段 → 默认值；role 初值由后端迁移按名字回填）
    role: normalizeRole(r.role),
    note: typeof r.note === 'string' ? r.note : '',
    pinned: r.pinned === true,
    updateWatch: r.updateWatch === true,
    hasUpdate: r.hasUpdate === true,
    updateCheckedAt: typeof r.updateCheckedAt === 'number' ? r.updateCheckedAt : 0,
    updateError: typeof r.updateError === 'string' ? r.updateError : '',
    bulkDone: r.bulkDone === true,
    bulkDoneAt: typeof r.bulkDoneAt === 'number' ? r.bulkDoneAt : 0,
  }
}

const restoreList = (raw: unknown): ScrapeBookmark[] =>
  Array.isArray(raw) ? raw.map(fromRaw).filter((b): b is ScrapeBookmark => b !== null) : []

/**
 * Round37：展示顺序比较器（sortKey 升序，同权值保持原数组顺序——sort 稳定排序）。
 * 与后端 `ORDER BY sort_key ASC, id ASC` 语义一致。
 */
const compareBookmarks = (a: ScrapeBookmark, b: ScrapeBookmark): number =>
  (a.sortKey ?? 0) - (b.sortKey ?? 0)

/**
 * Round37：本地估算下一个排序权值（末项 +1000，与后端「追加到末尾」一致）。
 * 仅用于乐观插入的临时书签占位，后端返回真实 sortKey 后会覆盖。
 */
const nextLocalSortKey = (): number => {
  const list = scrapeBookmarks.value
  return list.length > 0 ? (list[list.length - 1].sortKey ?? 0) + 1000 : 1000
}

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

// ─── Round36：书签作用域（标记只在「当时的搜索&筛选条件」下显示）───

/**
 * 给定搜索&筛选条件下、书签标记应生效的 gid 集合。
 *
 * 语义：书签是位置快照，标记只在**创建时那份 config 快照等价**的条件下显示——
 * 首页条件下标记的书签，不会因为换了关键词/改了筛选而在其它列表里高亮；
 * 同一 gid 若在另一个条件下也被标记（另一条书签），则由那条书签在自己的作用域内生效。
 */
export const bookmarkedGidsFor = (scope: Partial<SearchConfig> | null | undefined): Set<string> => {
  const set = new Set<string>()
  for (const bm of scrapeBookmarks.value) {
    if (!bm.anchor?.gid) continue
    if (isSameOnlineScope(bm.config, scope)) set.add(bm.anchor.gid)
  }
  return set
}

/** 当前在线搜索&筛选条件下的书签标记集合（OnlineHome 列表角标/高亮据此判定） */
export const scopedBookmarkedGids = computed<Set<string>>(() =>
  bookmarkedGidsFor(onlineSearchConfig.value),
)

/** 按 id 取书签（跳转恢复用；不存在返回 undefined） */
export const getBookmarkById = (id: string): ScrapeBookmark | undefined =>
  scrapeBookmarks.value.find((b) => b.id === id)

// ─────────────────────────────────────────────────────────────
// Round49：书签展示与排序（名称序 + 有更新置前）
// ─────────────────────────────────────────────────────────────

/** 侧栏置顶书签上限（与后端 PinnedBookmarkLimit 一致） */
export const PINNED_BOOKMARK_LIMIT = 5

/** 语言筛选项 → 中文标签（标题摘要用） */
const languageLabel = (lang: string): string => {
  switch (lang) {
    case 'Chinese':
      return '中文'
    case 'Japanese':
      return '日语'
    case 'English':
      return '英语'
    default:
      return lang
  }
}

/**
 * Round49：书签副标题（固定，不支持手改）。
 * = 位置字段 + 非默认筛选摘要（仅删除 / 语言 / 评分下限）。
 * 位置字段沿用旧 bookmarkLocationLabel：`搜索: <词>` 或 `首页`。
 * 这样首页与"删除页"（onlyRemoved）不再重名，书签页里可区分。
 *
 * 注：主标题（自定义名 > 翻译名 > 本函数输出）见 utils/bookmarkTitle.ts。
 */
export const bookmarkDisplayTitle = (
  bm: Pick<ScrapeBookmark, 'type' | 'keyword' | 'config'>,
): string => {
  const kw = (bm.keyword || bm.config?.keyword || '').trim()
  const base = bm.type === 'search' && kw ? `搜索: ${kw}` : '首页'
  const extras: string[] = []
  if (bm.config?.onlyRemoved) extras.push('仅删除')
  const lang = bm.config?.language
  if (lang && lang !== 'All') extras.push(languageLabel(lang))
  if (bm.config?.minRating && bm.config.minRating > 0) extras.push(`评分≥${bm.config.minRating}`)
  return extras.length > 0 ? `${base} · ${extras.join(' · ')}` : base
}

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
  extra?: Partial<Pick<ScrapeBookmark, 'role' | 'note' | 'updateWatch' | 'bulkDone'>>,
): Promise<ScrapeBookmark | null> => {
  const temp: ScrapeBookmark = {
    id: genBookmarkId(),
    // Round29：名称默认留空（Round49：标题改由 bookmarkDisplayTitle 生成，name 仅作历史保留）
    name: name.trim(),
    type,
    keyword,
    config: cloneSearchConfig(config),
    anchor,
    // Round37：乐观插入追加到末尾（后端返回真实权值后覆盖）
    sortKey: nextLocalSortKey(),
    createdAt: Date.now(),

    // Round49：书签项配置（创建时可选带 role/备注/更新开关/已囤货）
    role: extra?.role ?? 'interest',
    note: (extra?.note ?? '').trim(),
    pinned: false,
    updateWatch: extra?.updateWatch ?? false,
    hasUpdate: false,
    updateCheckedAt: 0,
    updateError: '',
    bulkDone: extra?.bulkDone ?? false,
    bulkDoneAt: extra?.bulkDone ? Date.now() : 0,
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
        // Round49：书签项配置随创建一并写入
        role: temp.role,
        note: temp.note,
        updateWatch: temp.updateWatch,
        bulkDone: temp.bulkDone,
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
    // Round37：回滚排序改用 sortKey（新语义），同权值保持数组顺序（sort 稳定排序）
    scrapeBookmarks.value.sort(compareBookmarks)
    console.error('删除书签失败:', e)
    toast.error('书签删除失败，请重试')
    return false
  }
}

/** 重命名书签（Round49：允许空名 → 清空自定义名称，恢复默认显示）。返回是否成功 */
export const renameScrapeBookmark = async (id: string, name: string): Promise<boolean> => {
  const trimmed = name.trim()
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
// Round33：迁移（Round49：失效检测已剪枝，仅保留迁移相关）
// ─────────────────────────────────────────────────────────────

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

// ─────────────────────────────────────────────────────────────
// Round49：书签项配置更新 / 更新探测
// ─────────────────────────────────────────────────────────────

/** 书签项配置的部分更新（乐观 + 失败回滚） */
export type BookmarkFieldsPatch = Partial<
  Pick<ScrapeBookmark, 'role' | 'note' | 'pinned' | 'updateWatch' | 'bulkDone'>
>

export const updateBookmarkFields = async (
  id: string,
  patch: BookmarkFieldsPatch,
): Promise<boolean> => {
  if (rejectWhenOffline()) return false
  const bm = scrapeBookmarks.value.find((b) => b.id === id)
  if (!bm) return false
  const prev = {
    role: bm.role,
    note: bm.note,
    pinned: bm.pinned,
    updateWatch: bm.updateWatch,
    bulkDone: bm.bulkDone,
    bulkDoneAt: bm.bulkDoneAt,
    hasUpdate: bm.hasUpdate,
  }
  // 乐观更新（置顶超限由后端 400 拒绝，随后回滚）
  if (patch.role !== undefined) bm.role = normalizeRole(patch.role)
  if (patch.note !== undefined) bm.note = patch.note.trim()
  if (patch.pinned !== undefined) bm.pinned = patch.pinned
  if (patch.updateWatch !== undefined) {
    bm.updateWatch = patch.updateWatch
    if (!patch.updateWatch) bm.hasUpdate = false
  }
  if (patch.bulkDone !== undefined) {
    bm.bulkDone = patch.bulkDone
    bm.bulkDoneAt = patch.bulkDone ? Date.now() : 0
  }
  try {
    await http(`/scrape-bookmarks/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    })
    return true
  } catch (e) {
    bm.role = prev.role
    bm.note = prev.note
    bm.pinned = prev.pinned
    bm.updateWatch = prev.updateWatch
    bm.bulkDone = prev.bulkDone
    bm.bulkDoneAt = prev.bulkDoneAt
    bm.hasUpdate = prev.hasUpdate
    console.error('书签配置更新失败:', e)
    toast.error(e instanceof Error ? e.message : '书签配置更新失败，请重试')
    return false
  }
}

/**
 * 手动触发更新探测（ids 为空 = 全部开启更新开关的书签）。
 *
 * 后端对每条书签各发 1 次列表请求（只取一页），判据：
 * 列表首条 postedAt > 书签.anchor.postedAt。
 * 失败（skipped / error）只记 updateError，**不动红点**。
 */
export const probeScrapeBookmarks = async (
  ids?: string[],
): Promise<ScrapeBookmarkProbeResult[]> => {
  const numeric = (ids ?? []).map((i) => Number(i)).filter((n) => Number.isFinite(n))
  const res = await http<{ results?: ScrapeBookmarkProbeResult[] }>(
    '/scrape-bookmarks/probe',
    {
      method: 'POST',
      body: JSON.stringify(ids && ids.length > 0 ? { ids: numeric } : {}),
      signal: AbortSignal.timeout(120 * 1000),
    },
  )
  const results = res.results || []
  for (const r of results) {
    const bm = scrapeBookmarks.value.find((b) => b.id === String(r.id))
    if (!bm) continue
    bm.updateCheckedAt = r.checkedAt
    if (r.status === 'ok') {
      bm.hasUpdate = r.hasUpdate
      bm.updateError = ''
    } else {
      bm.updateError = r.message || '未判定'
    }
  }
  return results
}
