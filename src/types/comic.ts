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
}

/** 本地书架定义 */
export interface Bookshelf {
  id: string
  name: string
  count: number
  comicIds?: string[] // 该书架收录的漫画 ID 列表
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

/** 分页控件接口（配合 Pagination.vue） */
export interface PaginationState {
  currentPage: number
  pageSize: number
  totalItems: number
  totalPages: number
}

/**
 * 搜索与筛选统一配置契约 (整合原 FilterParams 与 SearchConfig)
 * 供 FilterBar / SearchBar / Drawer 使用
 */
export interface FilterParams {
  keyword?: string // 搜索框输入的文本
  tags?: string[] // 选中的标签列表
  categories?: string[] // 选中的分类列表 (支持多选，统一命名为复数)
  source?: ComicSource // 来源限定
  minRating?: number // 最低评分要求 (1 ~ 5)
  minPages?: number // 最少页数
  maxPages?: number // 最多页数
  onlyDownloaded?: boolean // 是否仅看已下载作品
  sortBy?: 'updatedAt' | 'title' | 'rating' | 'readCount' // 排序字段 (准确对齐 BaseComic)
  sortOrder?: 'asc' | 'desc' // 排序顺序
}
