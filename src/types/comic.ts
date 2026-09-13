// ==========================================
// 1. 基础类型与枚举
// ==========================================

/** 漫画来源体系：在线还是本地 */
export type ComicSource = 'online' | 'offline'

/** 卡片展示模式 (配合 store/viewMode.ts 与 ItemCard.vue) */
export type CardViewMode = 'card' | 'compact' // 卡片模式 / 名片模式

// ==========================================
// 2. 核心数据契约 (基础接口 + 在线/离线扩展)
// ==========================================

/** 所有漫画（在线/离线）通用的基础属性，专门供 ItemCard 和 GridContainer 使用 */
export interface BaseComic {
  id: string // 唯一标识 (本地 UUID 或在线 GID)
  title: string // 标题
  coverUrl: string // 封面图 (本地路径或远程 URL)
  source: ComicSource // 标识来源
  tags: string[] // 标签列表
  category?: string // 分类 (提至基类：Doujinshi, Manga 等)
  rating?: number // 评分/星级
  pageCount?: number // 总页数 (统一命名为 pageCount)
  readCount?: number // 统一：阅读/点击总次数 (替代混淆的 clickCount)
  updatedAt: string // 更新/扫描时间
  isDownloaded?: boolean // 是否已下载到本地（全局绿标）
  rank?: number // 榜单/热门排名序号（可选）
}

/** 在线漫画特有属性 (如 E-Hentai 收藏、热门等) */
export interface OnlineComic extends BaseComic {
  source: 'online'
  token?: string // 详情页 token/key
  uploader?: string // 上传者
  isFavorite?: boolean // 是否加入在线收藏
  favIndex?: number // 0-9，对应 E 站的 Favorite 0 ~ Favorite 9
}

/** 本地离线漫画特有属性 (如 本地书架、书目维护等) */
export interface OfflineComic extends BaseComic {
  source: 'offline'
  localPath: string // 本地存储绝对/相对路径
  fileSize?: number // 文件大小 (bytes)
  hasError?: boolean // 修正语义：标记是否有损坏或缺失页面 (原 needsUpdate)
  bookshelfId?: string // 关联的离线书架 ID

  // ─── 问题1/2/3：标题、时间与来源字段 ───
  titleJpn?: string // 日文原名（优先显示，问题2）
  addedAt?: string // 首次入库时间（问题1 排序）
  fileModifiedAt?: string // 本地文件夹/归档文件修改时间（问题1 排序）
  publishedAt?: string | null // 发布时间（问题1 排序）
  scanPathID?: string // 来源额外路径 ID；空=下载导入（问题3）
  sourceLabel?: string // 来源标签（额外路径 Name；空=下载，问题3）

  // ─── 需求2：本地 tag 搜索 ───
  tagRaws?: string[] // 原始 tag 字符串（含命名空间，如 "female:cat ears"），供本地 tag 搜索/语言过滤精确匹配

  // ─── Round20-Bug1：E 站画廊 GID（离线历史按 gid 合并去重/引用迁移）───
  gid?: string

  // ─── Round23：自定义删除页面（隐藏页软删除）───
  hiddenPagesList?: number[] // 隐藏的物理页索引（0-based 原文件索引）
  originalPageCount?: number // 原始物理页数（隐藏页后 pageCount 为有效页数）
}

/** 本地书架定义 */
export interface Bookshelf {
  id: string
  name: string
  count: number
  comicIds?: string[] // 该书架收录的漫画 ID 列表（Round22 起为按权值排序后的展示顺序）
  pinned?: boolean // 侧栏置顶（Round13）
  /** Round22 LexoRank：书架列表权值（单书架移动只更新此项） */
  sortKey?: number
  /** Round22 LexoRank：书架内本子权值表 {comicId: weight}；缺失权值的项回退 comicIds 数组顺序 */
  sortKeys?: Record<string, number>
}

/** 联合类型：UI 层统一处理的单项对象 */
export type ComicItem = OnlineComic | OfflineComic

// ==========================================
// 3. 章节与阅读器数据结构
// ==========================================

export interface ComicPage {
  pageIndex: number
  imageUrl: string
  isSprite?: boolean
  offsetX?: number
  offsetY?: number
  width?: number
  height?: number
}

export interface Chapter {
  id: string
  comicId: string
  title: string
  chapterIndex: number
  pages?: ComicPage[]
}

// ==========================================
// 4. 用户交互与状态 (阅读历史)
// ==========================================

/** 历史记录项 (用于 OnlineHistory / OfflineHistory / ReadingList) */
export interface HistoryRecord {
  comicId: string
  source: ComicSource
  title: string // 保持与 BaseComic.title 一致，去除冗余前缀
  coverUrl: string
  lastChapterTitle?: string
  lastPageIndex: number // 上次看到第几页
  pageCount: number // 统一使用 pageCount，避免与 totalPageCount 混淆
  lastReadAt: string // 最后阅读时间戳
}

// ==========================================
// 5. 搜索、筛选与分页大一统契约
// ==========================================

/** 线下模式：标准数字页码状态 */
export interface OfflinePaginationState {
  currentPage: number
  pageSize: number
  totalItems: number
  totalPages: number
}

/** 线上模式：游标/流式加载状态 */
export interface OnlineCursorState {
  nextGid?: string // 加载下一页的游标锚点
  prevGid?: string // 加载上一页的游标锚点
  seek?: string // 按日期跳转的时间标识 (DateTime ISO 字符串)
  hasMore: boolean // 是否有更多数据
  isLoading: boolean // 是否处于加载中（用于触发 Loading 态）
}

/**
 * 搜索/筛选配置（顶栏、筛选抽屉与页面间共享的“生效中筛选条件”）
 * 由 appStore 拆出的 searchStore 使用
 */
export interface SearchConfig {
  keyword: string // 顶栏搜索词
  keywords: string[] // 筛选抽屉中的多关键词队列
  activeCategories: string[] // 激活的分类（默认全选）
  minRating: number // 最低评分 (在线映射 f_srdd 星级)
  minPages?: number // 最少页数 (仅离线生效)
  maxPages?: number // 最多页数 (仅离线生效)
  onlyDownloaded: boolean // 是否仅显示已下载

  // ─── E-Hentai 高级筛选 (f_* 参数，本地图库按语义降级应用) ───
  language: string // 语言: All | Chinese | Japanese | English
  onlyRemoved: boolean // f_sh=on 仅搜索移除了的画廊 (仅在线)
  onlyTorrents: boolean // f_sto=on 只显示有种子的画廊 (仅在线)
  disableLangFilter: boolean // f_sfl=on 禁用语言过滤 (离线=忽略语言选择)
  disableUploaderFilter: boolean // f_sfu=on 禁用上传者过滤 (仅在线)
  disableTagFilter: boolean // f_sft=on 禁用 Tag 过滤 (离线=关键词仅匹配标题)

  // ─── Round3-任务6：负向排除（`- ` 前缀：负向 tag 精确匹配 / 负向关键词子串匹配）───
  excludeTags?: string[] // 负向 tag（如 "female:yuri"，精确匹配 namespace:key）
  excludeKeywords?: string[] // 负向关键词（如 "3d"，子串匹配标题/tag/上传者）
}

/**
 * 搜索与筛选统一配置契约 (扩展游标与页码参数)
 */
export interface FilterParams {
  keyword?: string
  tags?: string[]
  categories?: string[]
  source?: ComicSource
  minRating?: number
  minPages?: number
  maxPages?: number
  onlyDownloaded?: boolean
  sortBy?:
    'updatedAt' | 'title' | 'rating' | 'readCount' | 'addedAt' | 'publishedAt' | 'fileModifiedAt'
  sortOrder?: 'asc' | 'desc'

  // ─── E-Hentai 高级筛选入参 ───
  language?: string // All | Chinese | Japanese | English (在线并入 f_search)
  onlyRemoved?: boolean // f_sh=on
  onlyTorrents?: boolean // f_sto=on
  disableLangFilter?: boolean // f_sfl=on
  disableUploaderFilter?: boolean // f_sfu=on
  disableTagFilter?: boolean // f_sft=on

  // ─── Round3-任务6：负向排除 ───
  excludeTags?: string[]
  excludeKeywords?: string[]

  // ─── 线上游标与线下页码入参 ───
  page?: number // 线下页码
  next?: string // 线上向下游标
  prev?: string // 线上向上游标
  seek?: string // 线上按日期跳转 (YYYY-MM-DD)
}

// ==========================================
// 6. 随机抽卡契约
// ==========================================

/** 随机抽卡统一返回项（在线/离线混合 DTO，兼容 BaseComic 供 ItemCard 直接渲染） */
export interface RandomComicItem {
  id: string
  title: string
  coverUrl: string
  source: ComicSource
  category?: string
  rating?: number
  tags: string[]
  pageCount?: number
  readCount?: number
  updatedAt: string
  isDownloaded?: boolean
  token?: string
  uploader?: string
  isFavorite?: boolean
  localPath?: string
  fileSize?: number
  hasError?: boolean

  // ─── Round32 阶段二：偏好推荐（仅推荐模式的离线结果填充）───
  matchedTags?: string[] // 命中理由（贡献最高的前 3 个 tag）
  score?: number // 推荐得分（说明用）
}

/** 随机抽卡入参 */
export interface RandomComicParams {
  count: number
  source: 'all' | 'online' | 'offline'
  keyword?: string
  keywords?: string[] // 抽卡专用过滤器的多关键词队列
  categories?: string[] // 在线/离线均生效
  minRating?: number // 在线映射 f_srdd 星级，离线按 rating 过滤
  minPages?: number // 仅离线生效
  maxPages?: number // 仅离线生效
  language?: string // 在线并入 f_search，离线按 language:xx 标签匹配（All|Chinese|Japanese|English）
  onlyDownloaded?: boolean // 仅已下载（仅离线生效）

  // ─── 抽卡专用过滤器：在线高级筛选（E-Hentai f_* 参数，仅在线/全库生效） ───
  onlyRemoved?: boolean // f_sh=on 仅搜索移除了的画廊
  onlyTorrents?: boolean // f_sto=on 只显示有种子的画廊
  disableLangFilter?: boolean // f_sfl=on 禁用语言过滤
  disableUploaderFilter?: boolean // f_sfu=on 禁用上传者过滤
  disableTagFilter?: boolean // f_sft=on 禁用 Tag 过滤

  // ─── Round3-任务6：负向排除（离线过滤 + 在线本地丢弃）───
  excludeTags?: string[]
  excludeKeywords?: string[]

  // ─── Round32 阶段二：卡池模式与推荐参数 ───
  mode?: RandomPoolMode // random=纯随机（默认，与旧版一致）｜recommend=本地偏好推荐
  recoTheta?: number // 偏好侧重：0=纯库藏，1=纯阅读（默认 0.6）
  recoExplore?: number // 探索率 ε：防 XP 固化（默认 0.15）
  recoTemp?: number // 采样温度 T：越大越平缓（默认 0.5）
  recoExcludeRead?: boolean // 排除读过的（read_count > 0）
  recoExcludeShelf?: boolean // 排除已在书架的本子
}

/** 卡池模式：纯随机 / 本地偏好推荐（推荐只作用于本地库，在线部分保持纯随机） */
export type RandomPoolMode = 'random' | 'recommend'

/** 随机抽卡响应 */
export interface RandomComicResponse {
  comics: RandomComicItem[]
  count: number
  warning?: string // 在线失败降级时的提示
  mode?: RandomPoolMode // 后端回声的卡池模式
  onlineRandom?: boolean // true = 在线部分为纯随机（推荐模式下的既有语义标注）
}

// ==========================================
// 7. 搜刮书签契约（Round27）
// ==========================================

/** 书签跳转类型：首页 / 搜索内容 */
export type ScrapeBookmarkType = 'home' | 'search'

/**
 * 搜刮书签：某次在线浏览的「位置快照」。
 * - config：创建时深拷贝的搜索/筛选状态（onlineSearchConfig 全量）
 * - anchor：锚定的画廊卡片（gid/token/title/postedAt），跳转后滚动定位 + 视觉突出
 *   Round29：锚定改为必需（去掉「仅保存位置」），postedAt = 锚定画廊发布时间
 *   Round33：新增失效标记 invalid / 迁移来源 migratedFrom / 列表位置 listIndex
 */
export interface ScrapeBookmark {
  id: string // 唯一标识（bm_ 前缀）
  name: string // 自定义名称（可为空串：留空时侧栏展示位置 + 发布时间）
  type: ScrapeBookmarkType
  keyword: string // type=search 时的搜索词（冗余于 config.keyword，便于侧栏展示）
  config: SearchConfig // 创建时深拷贝快照
  anchor: {
    gid: string
    token?: string
    title?: string
    postedAt?: string // 锚定画廊发布时间（如 "2026-09-12 01:20"）
    /** Round33：失效标记（检测确认后写入；null/缺省 = 未确认失效） */
    invalid?: { kind: ScrapeBookmarkInvalidKind; at: number } | null
    /** Round33：迁移来源（原锚点快照，供撤销与展示） */
    migratedFrom?: {
      gid: string
      token?: string
      title?: string
      postedAt?: string
    } | null
    /** Round33：保存时在列表中的位置（老书签无 postedAt 时的迁移兜底） */
    listIndex?: number
  } | null
  /**
   * Round37：侧栏拖动排序的 LexoRank 权值（越小越靠前）。
   * 0 = 升级前老书签未赋权，读取顺序按 id 兜底（等价于原创建顺序）；
   * 首次拖动时由 ensureBookmarkWeights 按当前展示顺序全量赋 1000*(i+1)。
   */
  sortKey?: number
  createdAt: number
}

/**
 * 书签失效类型（Round35：语义 = 书签级可见性）
 * - 'unreachable'：在书签自带的搜索&筛选条件下已看不到锚定画廊（新判定唯一产出）
 * - 'removed' | 'copyright' | 'invalid'：Round33 旧实现的画廊状态判定结果，仅历史数据兼容
 */
export type ScrapeBookmarkInvalidKind = 'unreachable' | 'removed' | 'copyright' | 'invalid'

/** 失效检测单条结果（后端 POST /scrape-bookmarks/check 返回） */
export interface ScrapeBookmarkCheckResult {
  id: number
  /** Round35：unreachable = 原检索条件下不可见（失效）；ok = 仍可见；error = 未判定 */
  status: 'ok' | 'unreachable' | 'removed' | 'copyright' | 'invalid' | 'replaced' | 'error'
  message?: string
  /** status=replaced：精确迁移目标（E 站标记的新版本画廊） */
  newVersion?: { gid: string; token?: string; title?: string; postedAt?: string }
  /** status=ok：刷新后的元信息（标题可能改名、发布时间补齐） */
  refreshed?: { gid: string; token?: string; title?: string; postedAt?: string }
}

// ==========================================
// 8. XP 词云契约（Round32 阶段一）
// ==========================================

/** 词云视图：library=库藏（搜集广度）｜reading=阅读（真实消耗） */
export type XpCloudView = 'library' | 'reading'

/** 词云分组：core=核心 XP｜ip=角色原作｜misc=其他｜all=全部（含画师） */
export type XpCloudGroup = 'core' | 'ip' | 'misc' | 'all'

/** 词云词条 */
export interface XpCloudTag {
  namespace: string
  key: string
  name: string // 中文翻译（词典缺失时回退 key）
  group: 'core' | 'ip' | 'misc'
  weight: number // 当前视图归一权重（0~1，字号依据）
  libWeight: number // 库藏视图归一权重
  readWeight: number // 阅读视图归一权重
  comicCount: number // 含该 tag 的本数
}

/** 画师/社团条目（不进词云，独立 Top 列表） */
export interface XpCloudArtist {
  namespace: string
  key: string
  name: string
  comicCount: number
  readWeight: number
}

/** 统计元信息（覆盖率提示 / 重算入口用） */
export interface XpCloudMeta {
  totalComics: number // 本地库总本数
  taggedComics: number // 参与统计（有有效 tag）的本数
  readSignalComics: number // 阅读信号非零的本数
  lastRebuildAt: number // 上次全量重建时间戳(ms)
  formulaVersion: number
}

/** 词云查询响应 */
export interface XpCloudResult {
  tags: XpCloudTag[]
  artists: XpCloudArtist[]
  meta: XpCloudMeta
}
