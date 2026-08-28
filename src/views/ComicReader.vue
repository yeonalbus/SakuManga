<script setup lang="ts">
// Round26-Bug：显式声明组件名——App.vue 的 keep-alive 以 :exclude="['ComicReader']"
// 排除阅读器缓存（缓存中 deactivated 实例的 watch 仍活跃，会把详情页劫持成 /reader）。
defineOptions({ name: 'ComicReader' })
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { getNextComicInQueue, getPrevComicInQueue, onlineReadingList, offlineReadingList } from '@/stores/readingStore'
// Round11-Bug3：阅读器进度写回时恢复标题/封面（在线模式从清单/历史取，避免把历史污染成 gid 乱码）
// Round20-Bug2：离线 404 + 纯数字 id 时无 token 也兜底解析在线 token 自动切在线
import { onlineHistoryList, offlineHistoryList, resolveOnlineToken } from '@/stores/historyStore'
import { readerSettings, parseReadDirection } from '@/stores/readerSettings'
import { useGamepad } from '@/composables/useGamepad'
import type { OnlineComic, OfflineComic, ComicItem } from '@/types/comic'
// Round24：侧栏抽屉（缩略图/章节大纲/书签，仅本地）
import ReaderSidebar, { type SidebarChapter, type SidebarPage } from '@/components/reader/ReaderSidebar.vue'
// Round24：顶栏文字按显示宽度截断
import { truncateByWidth } from '@/utils/truncate'
// Round20-Bug4：孤儿引用清理（历史/清单/书架）与列表刷新
import { fetchOfflineComics, offlineComics, recordComicClick, purgeOrphanOfflineRefs } from '@/stores/comicStore'
import { http } from '@/utils/request'
import { API_BASE, TOKEN_KEY } from '@/config/api'
// Round3-任务1：阅读进度按账号写回后端 /history
import { syncHistory } from '@/stores/historyStore'
import { useUserStore } from '@/stores/userStore'
// Round20：加载失败诊断上报（Bug2 PWA 跳转链 / Bug4 书架 404 取证）
import { reportError } from '@/utils/errorReporter'
// Round21：阅读器返回按统一决策（PC 新标签=关标签，单标签/深链回来源）
import { shouldCloseTab, consumeBackState } from '@/utils/detailNav'
import { rememberListState } from '@/utils/scrollMemory'
// Round7-任务1：本地进度存储统一委托公共工具（与详情页「立即阅读」恢复共用同一实现）
import {
  getProgressStorageKey,
  getProgressMap as getSharedProgressMap,
  saveProgress as saveSharedProgress,
  resolveResumePage,
  isResumeFromLastPageEnabled,
} from '@/utils/readingProgress'

// 屏幕常亮 Wake Lock 的类型声明（避免 any）
interface WakeLockManager {
  request(type: 'screen'): Promise<WakeLockSentinel>
}

const router = useRouter()
const route = useRoute()
const { toast, modal } = useUI()
const userStore = useUserStore()

// --------------------------------------------------
// 1. 基础参数与基础控制
// --------------------------------------------------
const comicId = computed(() => (route.query.id as string) || '')
const source = computed<'online' | 'offline'>(
  () => (route.query.source as 'online' | 'offline') || 'offline',
)

const currentPage = ref(1)
const totalPages = ref(0)
const showControls = ref(true) // 悬浮控制条显隐
const isFullscreen = ref(false) // 全屏状态
const showSettings = ref(false) // 显示设置面板
const isZoomed = ref(false) // 双击放大状态
const isLoading = ref(false) // 加载中
// Round20-Bug2/Bug4：页列表加载失败的错误层（显示重试/返回，替代裸 toast）
const loadError = ref('')

// --------------------------------------------------
// 📖 阅读方向布局（联动 readerSettings.readDirection）
// --------------------------------------------------
const layout = computed(() => parseReadDirection(readerSettings.readDirection))
const isRTL = computed(() => layout.value.isRTL)
const isDoublePage = computed(() => layout.value.isDoublePage)
const isWebtoon = computed(() => layout.value.isWebtoon)
/** 反转翻页方向后的“有效 RTL” */
const effectiveRTL = computed(() =>
  readerSettings.reverseTurnDirection ? !isRTL.value : isRTL.value,
)

const pageUrls = ref<string[]>([])
// Round24：离线物理索引锚点——可见物理页序列（0-based 原文件索引），
// 图片一律按物理索引直读 /raw-page（URL 稳定可缓存，修复隐藏页后错位问题）。
const physicalIndices = ref<number[]>([])

// 在阅读器内切换 单页/双页（同步写回全局设置）
const togglePageLayout = () => {
  const dir = readerSettings.readDirection
  if (isWebtoon.value) {
    readerSettings.readDirection = 'rtl_double'
    return
  }
  if (dir === 'rtl_double') readerSettings.readDirection = 'rtl_single'
  else if (dir === 'rtl_single') readerSettings.readDirection = 'rtl_double'
  else if (dir === 'ltr_double') readerSettings.readDirection = 'ltr_single'
  else if (dir === 'ltr_single') readerSettings.readDirection = 'ltr_double'
}

// 在阅读器内切换 RTL / LTR
const toggleDirection = () => {
  const dir = readerSettings.readDirection
  if (isWebtoon.value) {
    readerSettings.readDirection = 'ltr_single'
    return
  }
  if (dir === 'rtl_double') readerSettings.readDirection = 'ltr_double'
  else if (dir === 'rtl_single') readerSettings.readDirection = 'ltr_single'
  else if (dir === 'ltr_double') readerSettings.readDirection = 'rtl_double'
  else if (dir === 'ltr_single') readerSettings.readDirection = 'rtl_single'
}

// 切换 Webtoon 连续滚动模式
const toggleWebtoon = () => {
  readerSettings.readDirection = isWebtoon.value ? 'rtl_double' : 'webtoon'
}

const directionLabel = computed(() => {
  const map: Record<string, string> = {
    rtl_double: '从右至左 · 双列',
    rtl_single: '从右至左 · 单列',
    ltr_double: '从左至右 · 双列',
    ltr_single: '从左至右 · 单列',
    webtoon: '连续滚动 (Webtoon)',
  }
  return map[readerSettings.readDirection] || readerSettings.readDirection
})

// 封面/页图代理 URL：浏览器 <img> 无法携带 Authorization 头，追加 query token 通过认证
const coverProxyUrl = (url: string) => {
  const token = localStorage.getItem(TOKEN_KEY) || ''
  return `${API_BASE}/comics/cover-proxy?url=${encodeURIComponent(url)}&token=${encodeURIComponent(token)}`
}

// --------------------------------------------------
// 🖼️ 页列表加载（在线 / 离线分流）
// --------------------------------------------------
// Round26-Bug：组件卸载/路由离开后终止一切飞行逻辑（自愈链跳转、状态更新）。
// keep-alive 已 exclude 本组件（退出即销毁），此标记为兜底防线。
let disposed = false
const loadComicPages = async () => {
  // 🎯 核心防刷：如果路由里根本没有 id（说明正在退出/跳转到其他页面），直接终止，绝不发请求
  const realId = route.query.id as string
  if (!realId) return
  // Round26-Bug：双保险——仅当实际处于 /reader 路由时才允许加载。
  // 历史 bug：本组件曾被 keep-alive 缓存，deactivated 后 watch 仍活跃，
  // 用户在详情/主页操作时以「当前页面 URL 的 query」（无 source → 默认 offline）触发加载，
  // 在线 gid 走离线接口 404 → 自愈链 router.replace 劫持当前页面成 /reader。
  if (route.path !== '/reader') return

  isLoading.value = true
  loadError.value = '' // Round20-Bug2/Bug4：重载/切换漫画时清空错误层
  resetImgStates()
  try {
    if (source.value === 'online') {
      // 🌐 在线模式：请求 E 站原图 URL 列表，统一走封面代理加载
      const tok = route.query.token as string
      if (!tok) {
        toast.error('缺少画廊 token，无法在线阅读')
        isLoading.value = false
        return
      }
      const data = await http<{ total?: number; urls?: string[] }>('/comics/online/pages', {
        params: { id: realId, token: tok },
      })
      const urls = data.urls || []
      const total = data.total && data.total > 0 ? data.total : urls.length
      totalPages.value = total
      // 🎯 以真实总页数初始化完整长度占位数组（空串=未解析），已有 URL 直接填入
      // 后续由 ensurePageLoaded 就近补全，解决「加载多少算多少」的尾页误判
      const proxied = Array.from({ length: total }, () => '')
      urls.forEach((u, i) => {
        if (i < total && u) {
          proxied[i] = coverProxyUrl(u)
        }
      })
      pageUrls.value = proxied
      onlineId = realId
      onlineToken = tok
    } else {
      // 📚 离线模式：物理索引锚点（Round24）
      // /pages 返回 originalTotal(物理页数) + hiddenPages(隐藏物理索引)；
      // 前端构建「可见物理序列」，图片按物理索引直读 /raw-page（URL 稳定可缓存，
      // 修复隐藏页后「有效序号」URL 强缓存错位的问题）。
      const data = await http<{
        total?: number
        pages?: unknown[]
        originalTotal?: number
        hiddenPages?: number[]
      }>(`/comics/${realId}/pages`)
      const physicalTotal = typeof data.originalTotal === 'number' ? data.originalTotal : 0
      const hidden = new Set(data.hiddenPages || [])
      const visible: number[] = []
      if (physicalTotal > 0) {
        for (let p = 0; p < physicalTotal; p++) {
          if (!hidden.has(p)) visible.push(p)
        }
      } else if (typeof data.total === 'number') {
        for (let i = 0; i < data.total; i++) visible.push(i)
      } else if (Array.isArray(data.pages)) {
        for (let i = 0; i < data.pages.length; i++) visible.push(i)
      }
      if (visible.length === 0) {
        toast.error('该画廊没有任何页面')
        isLoading.value = false
        return
      }
      physicalIndices.value = visible
      totalPages.value = visible.length
      pageUrls.value = visible.map((phys) => `${API_BASE}/comics/${realId}/raw-page/${phys}`)
      // Round24：侧栏标记数据（仅离线）
      void fetchMarks()
    }

    // 恢复起始页码：优先路由 page 参数（预览图点击进入，可见序号），其次历史进度
    const targetPage = Number(route.query.page)
    let startPage = 1
    if (Number.isInteger(targetPage) && targetPage >= 1 && targetPage <= totalPages.value) {
      startPage = targetPage
    } else {
      // 统一恢复逻辑：本地缓存 + 后端进度取更远位置（与详情页「立即阅读」resolveResumePage 一致）
      const last = await resolveResumePage(source.value, realId, {
        fromHistory: route.query.resume === '1',
        resumePreference: isResumeFromLastPageEnabled(),
      })
      if (last !== null) {
        if (source.value === 'offline') {
          // Round24：离线进度按物理索引存储；旧数据（有效序号）作废——定位不到时回退第 1 页
          const idx = physicalIndices.value.indexOf(last)
          startPage = idx >= 0 ? idx + 1 : 1
        } else {
          startPage = Math.min(Math.max(1, last), totalPages.value)
        }
      }
    }
    currentPage.value = startPage
    if (isWebtoon.value) scrollToPage(startPage)
    // 初始就近补全当前页附近（在线模式空页懒加载）
    preloadNearby(startPage - 1)
  } catch (err) {
    console.error('加载画廊失败:', err)
    const msg = err instanceof Error ? err.message : '加载画廊失败'
    // Round20-Bug2/Bug4：离线模式 404 自愈链（替代原阻塞确认框）
    if (source.value === 'offline' && /找不到该漫画|not found|404/i.test(msg)) {
      // bug3 兜底：本地库漫画 id 是 md5 hex（含字母），纯数字 id 只可能是 E 站 gid。
      // 若以离线模式打开纯数字 gid，说明 source 被误传为 offline（如阅读清单快照缺失 source），
      // 此时不应误报「漫画已移除」，而是尝试自动纠正为在线模式（决策 D4=A：无 token 也先解析）。
      const isOnlineGid = /^\d+$/.test(realId)
      if (isOnlineGid) {
        let tok = route.query.token as string
        if (!tok) {
          tok = await resolveOnlineToken(realId)
        }
        if (tok) {
          // Round26-Bug：跳转前再次校验实例存活与路由归属，杜绝卸载/离场后残留跳转
          if (disposed || route.path !== '/reader') return
          toast.info('检测到该画廊属于在线资源，已自动切换为在线模式')
          await router.replace({
            path: '/reader',
            query: { ...route.query, source: 'online', token: tok || route.query.token },
          })
          // watch 只监听 route.query.id，source 变化不会自动重载，需手动重新加载
          await loadComicPages()
          return
        }
        toast.error('该漫画不在本地库中，且无法解析在线画廊 token，无法阅读')
        return
      }
      // md5 本地 id：刷新离线列表确认漫画是否仍存在（更新替换/删除/扫描重建会换 id 或删记录）
      await fetchOfflineComics()
      const stillExists = offlineComics.value.some((c) => c.id === realId)
      if (!stillExists) {
        // 决策 D2=A：孤儿引用自动剔除；非阻塞提示（不再弹「是否刷新后返回」确认框）
        purgeOrphanOfflineRefs(realId)
        toast.error('该漫画已从本地库移除（可能已更新替换或删除），已刷新离线列表')
      } else {
        toast.warning('漫画加载失败，请重试')
      }
      reportError(
        'warn',
        `离线阅读 404：id=${realId}`,
        err instanceof Error ? err.stack : String(err),
        `route=${route.fullPath}`,
      )
      loadError.value = msg
      return
    }
    // 在线加载失败：可重试错误层（决策 D4=A），不再裸 toast 后留白屏
    reportError(
      'warn',
      `在线阅读加载失败：id=${realId}`,
      err instanceof Error ? err.stack : String(err),
      `route=${route.fullPath}`,
    )
    loadError.value = msg
  } finally {
    isLoading.value = false
  }
}

/** Round20-Bug2/Bug4：错误层「重试」——清空错误态后重新加载页列表 */
const retryLoad = () => {
  loadError.value = ''
  loadComicPages()
}

/**
 * Round21：阅读器「退出阅读 / 返回」统一决策——
 * PC 新标签打开的阅读器（入口路由匹配 + 来源标签存活）→ 关闭标签；
 * 仅剩单标签 / 标签内深链 / PWA 同标签 → 回来源（consumeBackState）→ 模式首页兜底。
 * Round26-Bug：兜底不再用 history.back() 逐帧回退——同标签 SPA 下阅读器/详情/搜索
 * 跳转层层压栈，back() 会一层层穿过所有中间页（含被劫持的阅读器帧），甚至因 PWA
 * history 栈混入整页加载帧而退出应用。所有阅读入口现已在跳转前记录来源（backState），
 * 未命中只可能是深链/分享直达 → 直接 replace 回来源模式首页，一步到位。
 */
const handleReaderBack = () => {
  const id = comicId.value
  if (shouldCloseTab(id, route.fullPath)) {
    window.close()
    return
  }
  const backState = consumeBackState(id)
  if (backState) {
    rememberListState(backState.fromPath, { top: backState.top, page: backState.page })
    router.replace(backState.fromFullPath || backState.fromPath)
    return
  }
  router.replace(source.value === 'online' ? '/online/home' : '/offline/home')
}

// 按预加载数量（在线/本地分别配置）预先拉取后续图片
const preloadImages = (currentIndex: number) => {
  const depth =
    source.value === 'online' ? readerSettings.preloadOnline : readerSettings.preloadOffline
  // 双页模式下当前屏幕已在看 [currentIndex] 与 [currentIndex + 1]，从 +2 开始预载
  const offset = isDoublePage.value && currentIndex > 0 ? 2 : 1

  for (let i = 0; i < depth; i++) {
    const nextIdx = currentIndex + offset + i
    // 跳过空串占位页（其加载交由 ensurePageLoaded 就近补全）
    if (nextIdx < pageUrls.value.length && pageUrls.value[nextIdx]) {
      const img = new Image()
      img.src = pageUrls.value[nextIdx]
    }
  }
}

// --------------------------------------------------
// 🛰️ 在线就近加载：按需补全空页（黑底占位 + 懒加载进度）
// --------------------------------------------------
const isOnline = computed(() => source.value === 'online')
let onlineId = ''
let onlineToken = ''
/** 去重：正在请求中的页索引（0-based），避免并发重复请求 */
const pendingLoads = new Set<number>()

// 就近补全指定页（0-based）；已有 URL 或正在请求则跳过
const ensurePageLoaded = async (idx: number) => {
  if (!isOnline.value) return
  if (idx < 0 || idx >= pageUrls.value.length) return
  if (pageUrls.value[idx]) return
  if (pendingLoads.has(idx)) return
  pendingLoads.add(idx)
  try {
    const data = await http<{ url?: string; total?: number }>('/comics/online/page', {
      params: { id: onlineId, token: onlineToken, index: idx + 1 },
    })
    // 后端返回真实总页数时校准（避免早期误判的 total 偏小）
    if (data.total && data.total > 0 && data.total !== totalPages.value) {
      if (data.total > totalPages.value) {
        const prev = pageUrls.value
        totalPages.value = data.total
        pageUrls.value = [...prev, ...Array.from({ length: data.total - prev.length }, () => '')]
      } else {
        totalPages.value = data.total
      }
    }
    if (data.url) {
      pageUrls.value[idx] = coverProxyUrl(data.url)
    }
  } catch (err) {
    // 单页就近补全失败不致命，保留占位（翻到该页时黑底 + 重试提示）
    console.warn(`就近加载第 ${idx + 1} 页失败:`, err)
  } finally {
    pendingLoads.delete(idx)
  }
}

// 就近补全当前页附近若干页（EhentaiViewer 式：跳转到 P20 不会从 P1 逐页加载）
const preloadNearby = (center: number) => {
  if (!isOnline.value) return
  const radius = Math.max(readerSettings.preloadOnline, 4)
  const start = Math.max(0, center - radius)
  const end = Math.min(pageUrls.value.length - 1, center + radius)
  for (let i = start; i <= end; i++) {
    if (!pageUrls.value[i]) {
      ensurePageLoaded(i)
    }
  }
}

// --------------------------------------------------
// 🖼️ 图片加载兜底：loading / error 时显示黑色占位
// --------------------------------------------------
type ImgState = 'loading' | 'loaded' | 'error'
const imgStates = ref<Record<number, ImgState>>({})
// 图片原始宽高（用于智能双页判定）
const imgDims = ref<Record<number, { w: number; h: number }>>({})

// 图片加载成功/失败时更新状态（同时记录原始宽高）
const markImgLoaded = (index: number, e: Event) => {
  imgStates.value[index] = 'loaded'
  const img = e.target as HTMLImageElement
  if (img && img.naturalWidth > 0) {
    imgDims.value[index] = { w: img.naturalWidth, h: img.naturalHeight }
  }
}
const markImgError = (index: number) => {
  imgStates.value[index] = 'error'
}
// 切换作品时清空全部图片状态
const resetImgStates = () => {
  imgStates.value = {}
  imgDims.value = {}
}

// --------------------------------------------------
// 📖 阅读历史进度持久化 (localStorage)
// 统一委托 src/utils/readingProgress.ts（与详情页「立即阅读」恢复逻辑共用同一实现）
// --------------------------------------------------
const currentUid = (): string => String(userStore.user?.id ?? 'anonymous')
const progressStorageKey = (): string => getProgressStorageKey(currentUid())
const getProgressMap = (): Record<string, number> => getSharedProgressMap(currentUid())
const saveProgress = (src: 'online' | 'offline', id: string, page: number): void =>
  saveSharedProgress(currentUid(), src, id, page)

// Round24：当前显示序号对应的物理索引（离线按物理索引存进度；在线保持显示序号）
const currentPhysicalIndex = (): number =>
  source.value === 'offline'
    ? (physicalIndices.value[currentPage.value - 1] ?? -1)
    : currentPage.value

// --------------------------------------------------
// Round24：侧栏抽屉（缩略图网格 / 章节大纲 / 书签，仅本地）
// --------------------------------------------------
const sidebarOpen = ref(false)
const sidebarTab = ref<'thumbs' | 'outline' | 'bookmarks'>('thumbs')
const readerChapters = ref<SidebarChapter[]>([])
const readerBookmarks = ref<number[]>([])

/** 当前物理索引（0-based，离线；在线无侧栏） */
const currentPhysicalIndex0 = computed(() =>
  source.value === 'offline' ? (physicalIndices.value[currentPage.value - 1] ?? -1) : -1,
)
/** 侧栏可见页序列（物理索引 ↔ raw-page URL，与 pageUrls 同序） */
const sidebarPages = computed<SidebarPage[]>(() =>
  physicalIndices.value.map((phys, i) => ({ physical: phys, url: pageUrls.value[i] ?? '' })),
)

// 拉取书签 / 章节（仅离线）
const fetchMarks = async () => {
  if (source.value !== 'offline' || !comicId.value) return
  try {
    const bm = await http<{ bookmarks?: number[] }>(`/comics/${comicId.value}/bookmarks`)
    readerBookmarks.value = bm.bookmarks || []
    const ch = await http<{ chapters?: SidebarChapter[] }>(`/comics/${comicId.value}/chapters`)
    readerChapters.value = ch.chapters || []
  } catch (err) {
    console.warn('加载书签/章节失败:', err)
  }
}

const openSidebar = () => {
  sidebarOpen.value = !sidebarOpen.value
  if (sidebarOpen.value) void fetchMarks()
}

// 跳转（物理索引 → 显示序号）
const jumpFromSidebar = (phys: number) => {
  const idx = physicalIndices.value.indexOf(phys)
  if (idx >= 0) {
    currentPage.value = idx + 1
    if (isWebtoon.value) scrollToPage(idx + 1)
  }
}

// 书签切换（整体覆盖）
const toggleBookmark = async (phys: number) => {
  if (source.value !== 'offline' || !comicId.value) return
  const next = readerBookmarks.value.includes(phys)
    ? readerBookmarks.value.filter((b) => b !== phys)
    : [...readerBookmarks.value, phys].sort((a, b) => a - b)
  try {
    const res = await http<{ bookmarks?: number[] }>(`/comics/${comicId.value}/bookmarks`, {
      method: 'PUT',
      body: JSON.stringify({ bookmarks: next }),
    })
    readerBookmarks.value = res.bookmarks || next
  } catch {
    toast.error('书签保存失败')
  }
}

// 添加章节
const addChapter = async (payload: { title: string; level: number; parentId: number; pageIndex: number }) => {
  if (source.value !== 'offline' || !comicId.value) return
  try {
    await http(`/comics/${comicId.value}/chapters`, {
      method: 'POST',
      body: JSON.stringify(payload),
    })
    await fetchMarks()
    toast.success('章节已添加')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '章节添加失败')
  }
}

// 更新章节（名称 / 层级 / 父级归属；起始页原样带回）
const updateChapter = async (payload: {
  id: number
  title: string
  level: number
  parentId: number
  pageIndex: number
}) => {
  if (source.value !== 'offline' || !comicId.value) return
  try {
    await http(`/comics/${comicId.value}/chapters/${payload.id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    })
    await fetchMarks()
    toast.success('章节已更新')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '章节更新失败')
  }
}

// 删除章节（连同子树）
const removeChapter = async (id: number) => {
  if (source.value !== 'offline' || !comicId.value) return
  try {
    await http(`/comics/${comicId.value}/chapters/${id}`, { method: 'DELETE' })
    readerChapters.value = readerChapters.value.filter((c) => c.id !== id)
    toast.success('章节已删除')
  } catch {
    toast.error('章节删除失败')
  }
}

// Round3-任务1：当前阅读作品元信息（供后端进度写回；离线优先取库内真实条目）
// Round11-Bug3：fallback 不得把 title 设为 comicId（会把后端历史标题污染成 gid 乱码、封面丢失）。
// 从阅读清单 / 历史记录中恢复真实标题与封面；仍缺失时 title 留空串，由后端 AddHistory 空值不覆盖保护。
const currentComicMeta = computed<ComicItem | null>(() => {
  if (!comicId.value) return null
  if (source.value === 'offline') {
    const found = offlineComics.value.find((c) => c.id === comicId.value)
    if (found) return found
    const fromList = offlineReadingList.value.find((c) => c.id === comicId.value)
    if (fromList) return fromList
    const fromHistory = offlineHistoryList.value.find((h) => h.comic.id === comicId.value)?.comic
    if (fromHistory) return fromHistory
  } else {
    const fromList = onlineReadingList.value.find((c) => c.id === comicId.value)
    if (fromList) return fromList
    const fromHistory = onlineHistoryList.value.find((h) => h.comic.id === comicId.value)?.comic
    if (fromHistory) return fromHistory
  }
  return {
    id: comicId.value,
    title: '',
    coverUrl: '',
    source: source.value,
    tags: [],
    updatedAt: '',
    pageCount: totalPages.value,
  } as ComicItem
})

// Round24：顶栏标题显示日文原名（优先），缺失回退普通标题/ID
const readerTitle = computed(() => {
  const meta = currentComicMeta.value
  if (meta) {
    const jpn = (meta as Partial<OfflineComic>).titleJpn
    if (jpn) return jpn
    if (meta.title) return meta.title
  }
  return comicId.value
})

// Round24：顶栏名称/章节路径按「显示宽度」截断（宽度阈值可由设置选择紧凑/宽松）
const headerTitleLimit = computed(() => (readerSettings.headerTextWidth === 'loose' ? 16 : 12))
const headerPathLimit = computed(() => (readerSettings.headerTextWidth === 'loose' ? 30 : 22))
const readerTitleDisplay = computed(() =>
  truncateByWidth(readerTitle.value, headerTitleLimit.value),
)

// 当前物理页所属章节路径（顶栏小字，复用侧栏同款定位逻辑）
const currentChapterPath = computed<SidebarChapter[]>(() => {
  const phys = currentPhysicalIndex0.value
  if (phys < 0 || readerChapters.value.length === 0) return []
  let leaf: SidebarChapter | null = null
  for (const c of readerChapters.value) {
    if (c.pageIndex > phys) continue
    if (!leaf) leaf = c
    else if (c.pageIndex > leaf.pageIndex || (c.pageIndex === leaf.pageIndex && c.level > leaf.level))
      leaf = c
  }
  if (!leaf) return []
  const path: SidebarChapter[] = []
  let cur: SidebarChapter | null = leaf
  while (cur) {
    path.unshift(cur)
    cur = readerChapters.value.find((c) => c.id === cur!.parentId) ?? null
  }
  return path
})
const readerChapterPathText = computed(() => {
  const text = currentChapterPath.value.map((c) => c.title).join(' › ')
  return text ? truncateByWidth(text, headerPathLimit.value) : ''
})

// Round3-任务1：翻页进度 debounce 写回后端（避免高频请求；离线/后端不可用时静默失败）
let progressSyncTimer: ReturnType<typeof setTimeout> | null = null
const scheduleSyncProgress = () => {
  if (progressSyncTimer) clearTimeout(progressSyncTimer)
  progressSyncTimer = setTimeout(() => {
    if (!currentComicMeta.value || totalPages.value <= 0) return
    // Round7-任务1：第 1 页不写回后端进度，避免把历史进度清零（第 1 页无需恢复）
    if (currentPage.value <= 1) return
    // Round24：进度按物理索引写回（离线），在线保持显示序号
    const phys = currentPhysicalIndex()
    if (phys < 0) return
    syncHistory(source.value, currentComicMeta.value, {
      lastPageIndex: phys,
      totalPageCount: totalPages.value,
    })
  }, 1000)
}

// 📖 连贯读取队列调度核心
// --------------------------------------------------
// Round14：force=true 时跳过确认框直接切本（手柄双击快速确认用）
const handleNextInQueue = async (force = false) => {
  // 查找队列里的下一作品
  const nextComic = getNextComicInQueue(comicId.value, source.value)

  if (nextComic) {
    if (!force) {
      const confirmed = await modal.confirm(
        `《${nextComic.title}》\n是否直接继续阅读清单中的下一本？`,
        '当前本子已全部读完 📖',
      )
      if (!confirmed) return
    }
    toast.success(`自动无缝切入：《${nextComic.title}》`)

    // 问题5：连续切本用 replace（阅读器在历史栈上始终只有一帧），
    // 退出按钮 back() 才能一步回到进入阅读前的页面，而不是逐本回退。
    const query: Record<string, string> = {
      id: nextComic.id,
      source: nextComic.source,
    }
    if (nextComic.source === 'online') {
      query.token = (nextComic as OnlineComic).token || ''
    }
    router.replace({ path: '/reader', query })
  } else {
    // 队列中已经没有更多本子了
    await modal.alert('清单中的所有本子都已经全部读完啦！🎉', '阅读完毕')
  }
}

// Round14：第一页双击「上一页」→ 切到队列上一本（force 直接切换，不弹确认）
const handlePrevInQueue = async (force = false) => {
  const prevComic = getPrevComicInQueue(comicId.value, source.value)
  if (!prevComic) {
    toast.info('已经是第一本了')
    return
  }
  if (!force) {
    const confirmed = await modal.confirm(
      `《${prevComic.title}》\n是否回退到清单中的上一本？`,
      '回退上一本',
    )
    if (!confirmed) return
  }
  toast.success(`回退到：《${prevComic.title}》`)
  const query: Record<string, string> = {
    id: prevComic.id,
    source: prevComic.source,
  }
  if (prevComic.source === 'online') {
    query.token = (prevComic as OnlineComic).token || ''
  }
  router.replace({ path: '/reader', query })
}

// ── Round14：手柄双击快速确认切本 ──
// 记录「下一页/上一页」最近一次触发时间，窗口内再次触发且处于边界页 → 强制切本
let lastNextAt = 0
let lastPrevAt = 0
const isAtLastPage = computed(
  () => totalPages.value > 0 && currentPage.value >= totalPages.value,
)
const isAtFirstPage = computed(() => currentPage.value <= 1)

const handleGamepadNext = (t?: number) => {
  const now = t ?? Date.now()
  const windowMs = readerSettings.gamepadDoubleTapWindow
  // 双击确认：边界页 + 配置开启 + 上次触发在窗口内
  if (
    readerSettings.gamepadDoubleTapConfirm &&
    isAtLastPage.value &&
    now - lastNextAt <= windowMs
  ) {
    lastNextAt = 0 // 消费双击，防三连击再次触发
    void handleNextInQueue(true)
    return
  }
  lastNextAt = now
  turnByPhysicalDirection('next')
}

const handleGamepadPrev = (t?: number) => {
  const now = t ?? Date.now()
  const windowMs = readerSettings.gamepadDoubleTapWindow
  if (
    readerSettings.gamepadDoubleTapConfirm &&
    isAtFirstPage.value &&
    now - lastPrevAt <= windowMs
  ) {
    lastPrevAt = 0
    void handlePrevInQueue(true)
    return
  }
  lastPrevAt = now
  turnByPhysicalDirection('prev')
}

// --------------------------------------------------
// 📐 智能双页判定：根据图片横纵与屏幕宽度避免裁剪
// --------------------------------------------------
const screenWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1024)
const handleResize = () => {
  screenWidth.value = window.innerWidth
}
/** 屏幕足够宽才允许双页（窄屏/手机强制单页，避免横向图被裁剪） */
const canUseDoublePage = computed(() => screenWidth.value >= 640)
/** 某页是否为纵向（未加载时按非纵向保守处理） */
const isPortrait = (idx: number): boolean => {
  const d = imgDims.value[idx]
  // 未加载时乐观视为纵向：保证双列模式初始即可双页，加载后若确为横向再降级单页
  if (!d) return true
  return d.h > d.w
}
/** 两张图能否并排双页（均纵向 + 有下一页 + 屏幕够宽） */
const canDoubleShow = (idxA: number, idxB: number): boolean => {
  if (!canUseDoublePage.value) return false
  if (idxB >= totalPages.value) return false
  return isPortrait(idxA) && isPortrait(idxB)
}
/** 以 p（1-based 起始页）计算该展示单元的大小（1 或 2） */
const getUnitSizeAt = (p: number): number => {
  if (!isDoublePage.value) return 1
  if (!canUseDoublePage.value) return 1
  if (p === 1 && readerSettings.singleCover) return 1
  if (canDoubleShow(p - 1, p)) return 2
  return 1
}
/** 当前展示单元大小 */
const unitSize = computed(() => getUnitSizeAt(currentPage.value))
/** 找到当前页之前一个单元的起始页（用于「上一页」导航） */
const getPrevUnitStart = (page: number): number => {
  let start = 1
  let last = 1
  while (start < page) {
    last = start
    const size = getUnitSizeAt(start)
    if (size < 1) break
    start += size
  }
  return last
}

// --------------------------------------------------
// 📖 计算双页模式渲染视图（智能判定后决定单页/双页）
// --------------------------------------------------
const visiblePageIndices = computed(() => {
  if (!isDoublePage.value) {
    return [currentPage.value - 1]
  }
  if (unitSize.value === 2) {
    const pageA = currentPage.value - 1
    const pageB = currentPage.value
    return isRTL.value ? [pageB, pageA] : [pageA, pageB]
  }
  return [currentPage.value - 1]
})

// --------------------------------------------------
// 🔄 翻页逻辑与清单调度
// --------------------------------------------------
const handlePrevPage = () => {
  // Webtoon 模式：滚动到上一张
  if (isWebtoon.value) {
    if (currentPage.value > 1) {
      currentPage.value -= 1
      scrollToPage(currentPage.value)
    } else {
      toast.info('已经是第一页了')
    }
    return
  }

  // 双页/单页模式：跳转到上一展示单元（步进随单元大小自适应）
  const prevStart = getPrevUnitStart(currentPage.value)
  if (prevStart >= 1 && prevStart < currentPage.value) {
    currentPage.value = prevStart
  } else {
    toast.info('已经是第一页了')
  }
}

const handleNextPage = async () => {
  // Webtoon 模式：滚动到下一张
  if (isWebtoon.value) {
    if (currentPage.value < totalPages.value) {
      currentPage.value += 1
      scrollToPage(currentPage.value)
    } else {
      await handleNextInQueue()
    }
    return
  }

  const step = unitSize.value
  if (currentPage.value + step <= totalPages.value) {
    currentPage.value += step
  } else {
    // 读到最后一页再往后按，触发连贯调度
    await handleNextInQueue()
  }
}

// 点击左/右半屏区域翻页（结合 RTL 方向与“禁用点击翻页”设置）
const handleLeftClick = () => {
  if (didDrag) return
  if (readerSettings.disableTapTurnGesture) return
  if (effectiveRTL.value) handleNextPage()
  else handlePrevPage()
}
const handleRightClick = () => {
  if (didDrag) return
  if (readerSettings.disableTapTurnGesture) return
  if (effectiveRTL.value) handlePrevPage()
  else handleNextPage()
}

// 中间三等分区：单击画面切换上下控制条显隐（Round24，触屏友好；设置改从 ⚙️ 进入）
const handleMidClick = () => {
  if (didDrag) return
  showControls.value = !showControls.value
  showSettings.value = false
}

// Webtoon 滚动容器
const webtoonContainer = ref<HTMLElement | null>(null)

// 平滑滚动到指定页
const scrollToPage = (page: number) => {
  nextTick(() => {
    const container = webtoonContainer.value
    if (!container) return
    const child = container.children[page - 1] as HTMLElement | undefined
    if (child) {
      child.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  })
}

// Webtoon 滚动时同步当前页码
const onWebtoonScroll = () => {
  const container = webtoonContainer.value
  if (!container) return
  const top = container.scrollTop + container.clientHeight * 0.25
  let active = 1
  for (let i = 0; i < container.children.length; i++) {
    const el = container.children[i] as HTMLElement
    if (top >= el.offsetTop) {
      active = i + 1
    }
  }
  if (active !== currentPage.value) {
    currentPage.value = active
  }
}

// 跳页（页码输入框等入口共用）
const jumpToPage = (page: number) => {
  currentPage.value = page
  if (isWebtoon.value) {
    scrollToPage(page)
  }
}

// --------------------------------------------------
// ⚙️ Web API 功能实现（自动翻页/常亮/电量/时钟/全屏）
// --------------------------------------------------

// 1. 自动翻页定时器
let autoTurnTimer: ReturnType<typeof setInterval> | null = null
watch([() => readerSettings.autoTurnInterval, currentPage], ([interval]) => {
  if (autoTurnTimer) clearInterval(autoTurnTimer)
  if (interval > 0) {
    autoTurnTimer = setInterval(() => {
      if (currentPage.value < totalPages.value) {
        handleNextPage()
      } else {
        if (autoTurnTimer) clearInterval(autoTurnTimer)
      }
    }, interval * 1000)
  }
})

// 2. 屏幕常亮 Wake Lock API
let wakeLockSentinel: WakeLockSentinel | null = null
watch(
  () => readerSettings.keepAwake,
  async (val) => {
    const wakeLock = (navigator as Navigator & { wakeLock?: WakeLockManager }).wakeLock
    if (val && wakeLock) {
      try {
        wakeLockSentinel = await wakeLock.request('screen')
        toast.success('已开启屏幕常亮')
      } catch {
        toast.info('当前浏览器不支持屏幕常亮锁')
      }
    } else if (wakeLockSentinel) {
      wakeLockSentinel.release().catch(() => {})
      wakeLockSentinel = null
    }
  },
)

// 3. 全屏切换
const toggleFullscreen = () => {
  if (!document.fullscreenElement) {
    document.documentElement.requestFullscreen()
    isFullscreen.value = true
  } else {
    if (document.exitFullscreen) {
      document.exitFullscreen()
      isFullscreen.value = false
    }
  }
}

// 5. 键盘快捷键绑定
const handleKeyDown = (e: KeyboardEvent) => {
  if (isWebtoon.value) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowRight' || e.key === ' ') {
      handleNextPage()
    } else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') {
      handlePrevPage()
    } else if (e.key.toLowerCase() === 'f') {
      toggleFullscreen()
    } else if (e.key === 'Escape') {
      showControls.value = !showControls.value
    }
    return
  }

  if (e.key === 'ArrowLeft') {
    if (effectiveRTL.value) {
      handleNextPage()
    } else {
      handlePrevPage()
    }
  } else if (e.key === 'ArrowRight') {
    if (effectiveRTL.value) {
      handlePrevPage()
    } else {
      handleNextPage()
    }
  } else if (e.key === ' ') {
    handleNextPage()
  } else if (e.key.toLowerCase() === 'f') {
    toggleFullscreen()
  } else if (e.key === 'Escape') {
    showControls.value = !showControls.value
  }
}

// 5.1 物理方向翻页：手柄 / 滑动翻页共用
// 「物理方向」语义与键盘 ArrowRight/ArrowLeft 一致，自动兼容 RTL 反转与 Webtoon 滚动
const turnByPhysicalDirection = (dir: 'next' | 'prev') => {
  if (isWebtoon.value) {
    if (dir === 'next') handleNextPage()
    else handlePrevPage()
    return
  }
  if (effectiveRTL.value) {
    if (dir === 'next') handlePrevPage()
    else handleNextPage()
    return
  }
  if (dir === 'next') handleNextPage()
  else handlePrevPage()
}

// 5.2 手柄绑定（8BitDo Micro：D-Pad右/A=下一页，D-Pad左/B=上一页，Start/Select=设置）
const { isConnected: gamepadConnected, gamepadName } = useGamepad({
  onNext: handleGamepadNext,
  onPrev: handleGamepadPrev,
  onToggle: () => {
    showSettings.value = !showSettings.value
  },
})

// 5.3 触摸滑动翻页（基础版：横向位移阈值 + 与点击/拖拽做手势仲裁）
const canvasStage = ref<HTMLElement | null>(null)
const SWIPE_THRESHOLD = 60 // 横向位移阈值 (px)
let touchStart: { x: number; y: number } | null = null
let suppressNextClick = false

const onTouchStart = (e: TouchEvent) => {
  const t = e.changedTouches[0]
  if (!t) return
  touchStart = { x: t.clientX, y: t.clientY }
}

const onTouchMove = (e: TouchEvent) => {
  // 非 Webtoon 模式：横向意图明显时阻止默认（防浏览器后退/手势干扰）
  if (isWebtoon.value || !touchStart) return
  const t = e.changedTouches[0]
  if (!t) return
  const dx = t.clientX - touchStart.x
  const dy = t.clientY - touchStart.y
  if (Math.abs(dx) > Math.abs(dy) && Math.abs(dx) > 8) {
    e.preventDefault()
  }
}

const onTouchEnd = (e: TouchEvent) => {
  if (!touchStart) return
  const t = e.changedTouches[0]
  const dx = t ? t.clientX - touchStart.x : 0
  const dy = t ? t.clientY - touchStart.y : 0
  touchStart = null
  // 仲裁：横向位移须超过阈值，且明显大于纵向（排除滚动/点按）
  if (Math.abs(dx) < SWIPE_THRESHOLD) return
  if (Math.abs(dx) < Math.abs(dy) * 1.2) return
  // 抑制随后的 click（防热区点击再翻一页 / 误切换控制条）
  suppressNextClick = true
  setTimeout(() => {
    suppressNextClick = false
  }, 350)
  if (dx < 0)
    turnByPhysicalDirection('next') // 向左滑 = 下一页方向
  else turnByPhysicalDirection('prev') // 向右滑 = 上一页方向
}

const onCaptureClick = (e: Event) => {
  if (suppressNextClick) {
    e.stopPropagation()
    e.preventDefault()
    suppressNextClick = false
  }
}

// 6. 双击放大（受 allowDoubleTapZoom 设置控制）
const handleDoubleClick = () => {
  if (!readerSettings.allowDoubleTapZoom) return
  isZoomed.value = !isZoomed.value
  toast.info(isZoomed.value ? '已放大' : '已还原')
}

// 7. 单击拖拽放大（受 allowSingleClickDragZoom 设置控制，仅单页/双页模式）
const DRAG_ZOOM_SCALE = 1.8
const dragPos = ref({ x: 0, y: 0 })
const dragOrigin = ref({ x: 0, y: 0 })
const isDragZoom = ref(false)
let didDrag = false

const dragZoomStyle = computed(() => {
  if (!isDragZoom.value) return {}
  return {
    transform: `translate(${dragPos.value.x}px, ${dragPos.value.y}px) scale(${DRAG_ZOOM_SCALE})`,
    transition: 'transform 0.1s ease',
    zIndex: 10,
  }
})

const onCanvasMouseDown = (e: MouseEvent) => {
  if (e.button !== 0 || !readerSettings.allowSingleClickDragZoom) return
  dragOrigin.value = { x: e.clientX, y: e.clientY }
  isDragZoom.value = true
  didDrag = false
}

const onCanvasMouseMove = (e: MouseEvent) => {
  if (!isDragZoom.value) return
  const dx = e.clientX - dragOrigin.value.x
  const dy = e.clientY - dragOrigin.value.y
  if (Math.abs(dx) > 4 || Math.abs(dy) > 4) didDrag = true
  dragPos.value = { x: dx, y: dy }
}

const onCanvasMouseUp = () => {
  if (!isDragZoom.value) return
  isDragZoom.value = false
  dragPos.value = { x: 0, y: 0 }
  // 延迟到 click 事件之后重置，以抑制拖拽产生的误翻页
  setTimeout(() => {
    didDrag = false
  }, 0)
}

const onCanvasMouseLeave = () => {
  if (!isDragZoom.value) return
  isDragZoom.value = false
  dragPos.value = { x: 0, y: 0 }
  didDrag = false
}

// 画布点击：拖拽放大后不切换控制条显隐
const handleStageClick = () => {
  if (didDrag) return
  showControls.value = !showControls.value
}

// --------------------------------------------------
// 生命周期
// --------------------------------------------------
onMounted(() => {
  window.addEventListener('keydown', handleKeyDown)
  // 监听窗口尺寸变化：智能双页判定依赖屏幕宽度
  window.addEventListener('resize', handleResize)
  // 沉浸模式：进入阅读器时隐藏顶部/底部控制条
  showControls.value = !readerSettings.immersiveMode
  // 滑动翻页：非 passive 监听 touchmove 以允许 preventDefault
  const stage = canvasStage.value
  if (stage) {
    stage.addEventListener('touchmove', onTouchMove, { passive: false })
  }
  // 捕获阶段拦截滑动产生的误点击
  document.addEventListener('click', onCaptureClick, true)
})

onUnmounted(() => {
  // Round26-Bug：置位卸载标记，终止仍在飞行的异步逻辑（loadComicPages / 自愈链跳转）
  disposed = true
  window.removeEventListener('keydown', handleKeyDown)
  window.removeEventListener('resize', handleResize)
  const stage = canvasStage.value
  if (stage) {
    stage.removeEventListener('touchmove', onTouchMove)
  }
  document.removeEventListener('click', onCaptureClick, true)
  if (autoTurnTimer) clearInterval(autoTurnTimer)
  if (wakeLockSentinel) wakeLockSentinel.release().catch(() => {})
  // Round7-任务1：退出时立即 flush 未完成的后端进度同步（仅当前页 > 1 时，
  // 避免第 1 页入口快速退出把后端已有进度清零）
  if (progressSyncTimer) {
    clearTimeout(progressSyncTimer)
    progressSyncTimer = null
    if (currentPage.value > 1 && currentComicMeta.value && totalPages.value > 0) {
      const phys = currentPhysicalIndex()
      if (phys >= 0) {
        syncHistory(source.value, currentComicMeta.value, {
          lastPageIndex: phys,
          totalPageCount: totalPages.value,
        })
      }
    }
  }
})

// --------------------------------------------------
// 监听与调度
// --------------------------------------------------

// 监听当前页码变化：实时触发预加载 + 保存进度
watch(currentPage, (newPg) => {
  if (comicId.value) {
    // Round24：进度存物理索引（离线），在线保持显示序号
    const phys = currentPhysicalIndex()
    if (phys >= 0) saveProgress(source.value, comicId.value, phys)
    // Round3-任务1：翻页 debounce 写回后端（按账号），页面数就绪后再同步
    if (totalPages.value > 0) {
      scheduleSyncProgress()
    }
  }
  nextTick(() => {
    if (!isWebtoon.value) {
      preloadImages(newPg - 1)
    }
    // 在线模式：就近补全当前页附近，保证翻页即时可用
    preloadNearby(newPg - 1)
  })
})

// 监听路由 ID 切换时重新加载页列表
// Round26-Bug：非 /reader 路由时禁止触发加载（历史 bug：组件被 keep-alive 缓存时，
// deactivated 后 watch 仍活跃，详情/主页路由变化会误触发本加载逻辑并劫持页面）。
watch(
  () => route.query.id,
  (newId) => {
    if (!newId) return
    if (route.path !== '/reader') return
    currentPage.value = 1
    isZoomed.value = false
    // Round14-Bug1：进入/切换离线漫画即记录阅读次数（详情页不再单独计次，避免双计）。
    // 覆盖清单入口、详情入口、书架入口与连续切本（router.replace 换 id）。
    if (source.value === 'offline') recordComicClick(comicId.value)
    loadComicPages()
  },
  { immediate: true },
)
</script>

<template>
  <div class="reader-viewport" :class="{ 'rtl-mode': isRTL }">
    <div
      v-if="readerSettings.customBrightness"
      class="brightness-overlay"
      :style="{ opacity: (100 - readerSettings.brightnessValue) / 100 }"
    ></div>

    <!-- Round20-Bug2/Bug4：页列表加载失败错误层（重试/返回，替代裸 toast 白屏） -->
    <div v-if="loadError" class="reader-error-overlay">
      <div class="reader-error-card">
        <div class="reader-error-icon">⚠️</div>
        <p class="reader-error-title">漫画加载失败</p>
        <p class="reader-error-msg">{{ loadError }}</p>
        <div class="reader-error-actions">
          <button class="reader-error-btn" @click="retryLoad">🔄 重试</button>
          <button class="reader-error-btn primary" @click="handleReaderBack">‹ 返回</button>
        </div>
      </div>
    </div>

    <Transition name="fade-top">
      <div v-if="showControls" class="floating-header">
        <button class="topbar-btn" title="退出阅读" @click="handleReaderBack">‹</button>

        <!-- Round24：侧栏抽屉开关（仅本地；横屏/窄屏均折叠保沉浸） -->
        <button
          v-if="source === 'offline'"
          class="topbar-btn"
          :class="{ active: sidebarOpen }"
          @click.stop="openSidebar"
          title="侧栏（缩略图/大纲/书签）"
        >
          ☰
        </button>

        <div class="header-info">
          <div class="title-row">
            <span class="comic-title" :title="readerTitle">{{ readerTitleDisplay }}</span>
            <span class="source-tag">{{
              source === 'online' ? '🌐 在线流加载' : '📚 本地挂载'
            }}</span>
          </div>
          <span v-if="readerChapterPathText" class="chapter-path" :title="readerChapterPathText">{{
            readerChapterPathText
          }}</span>
        </div>

        <div class="status-widgets">
          <span
            v-if="gamepadConnected"
            class="widget-item gamepad-indicator"
            title="游戏手柄已连接"
          >
            🎮<span class="gamepad-name">{{ gamepadName }}</span>
          </span>
          <button class="topbar-btn" @click.stop="showSettings = !showSettings" title="阅读设置">
            ⚙️
          </button>
          <button class="topbar-btn" @click.stop="showControls = false" title="隐藏控制条">
            ✕
          </button>
        </div>
      </div>
    </Transition>

    <!-- 🎞️ Webtoon 连续滚动模式 -->
    <div
      v-if="isWebtoon"
      ref="webtoonContainer"
      class="webtoon-container"
      @scroll="onWebtoonScroll"
      @click="showControls = !showControls"
    >
      <div
        v-for="(url, i) in pageUrls"
        :key="i"
        class="webtoon-item"
        :class="{ 'img-error': imgStates[i] === 'error' }"
        :style="readerSettings.imageGap > 0 ? { marginBottom: `${readerSettings.imageGap}px` } : {}"
      >
        <!-- 空串占位页（在线就近加载未完成）：黑色底 + 加载提示 -->
        <div v-if="!url" class="webtoon-placeholder">
          <span class="placeholder-spinner"></span>
          <span class="placeholder-text">P{{ i + 1 }} 加载中…</span>
        </div>
        <img
          v-else
          :src="url"
          class="webtoon-img"
          :class="[
            `fit-${readerSettings.pageFit}`,
            {
              zoomed: isZoomed && i + 1 === currentPage,
              'img-hidden': imgStates[i] === 'loading' || imgStates[i] === 'error',
            },
          ]"
          :alt="`P${i + 1}`"
          loading="lazy"
          @load="(ev) => markImgLoaded(i, ev)"
          @error="markImgError(i)"
          @dblclick.stop="handleDoubleClick"
        />
        <div v-if="imgStates[i] === 'error'" class="webtoon-item-label">图片加载失败</div>
      </div>
      <div v-if="isLoading" class="webtoon-loading">加载中...</div>
    </div>

    <!-- 📄 单页 / 双页模式 -->
    <template v-else>
      <div
        ref="canvasStage"
        class="canvas-stage"
        @click="handleStageClick"
        @mousedown="onCanvasMouseDown"
        @mousemove="onCanvasMouseMove"
        @mouseup="onCanvasMouseUp"
        @mouseleave="onCanvasMouseLeave"
        @touchstart="onTouchStart"
        @touchend="onTouchEnd"
      >
        <div class="click-zone top-zone" @click.stop="handleMidClick" title="打开阅读设置"></div>
        <div
          v-if="!readerSettings.disableTapTurnGesture"
          class="click-zone prev-zone"
          @click.stop="handleLeftClick"
          :title="effectiveRTL ? '下一页' : '上一页'"
        ></div>
        <div class="click-zone mid-zone" @click.stop="handleMidClick" title="打开阅读设置"></div>
        <div
          v-if="!readerSettings.disableTapTurnGesture"
          class="click-zone next-zone"
          @click.stop="handleRightClick"
          :title="effectiveRTL ? '上一页' : '下一页'"
        ></div>

        <div
          class="images-wrapper"
          :class="{
            'double-page': unitSize === 2,
            'turn-anim': readerSettings.enableTurnAnimation,
          }"
          :key="readerSettings.enableTurnAnimation ? `page-${currentPage}` : 'static'"
          :style="[
            dragZoomStyle,
            isDoublePage && readerSettings.imageGap > 0
              ? { gap: `${readerSettings.imageGap}px` }
              : {},
          ]"
        >
          <div
            v-for="pageIdx in visiblePageIndices"
            :key="pageIdx"
            class="page-item"
            :class="[
              `fit-${readerSettings.pageFit}`,
              { 'img-error': imgStates[pageIdx] === 'error' },
            ]"
          >
            <!-- 空串占位页（在线就近加载未完成）：黑色底 + 加载提示 -->
            <div v-if="!pageUrls[pageIdx]" class="page-placeholder">
              <span class="placeholder-spinner"></span>
              <span class="placeholder-text">P{{ pageIdx + 1 }} 加载中…</span>
            </div>
            <img
              v-else
              :src="pageUrls[pageIdx]"
              class="manga-page-img"
              :class="{
                zoomed: isZoomed && !isDragZoom && visiblePageIndices.includes(currentPage - 1),
                'img-hidden': imgStates[pageIdx] === 'loading' || imgStates[pageIdx] === 'error',
              }"
              :alt="`P${pageIdx + 1}`"
              @load="(ev) => markImgLoaded(pageIdx, ev)"
              @error="markImgError(pageIdx)"
              @dblclick.stop="handleDoubleClick"
            />
            <div v-if="imgStates[pageIdx] === 'error'" class="page-item-label">图片加载失败</div>
          </div>
        </div>

        <div v-if="isLoading" class="stage-loading">加载中...</div>
      </div>
    </template>

    <Transition name="fade-bottom">
      <div v-if="showControls" class="floating-footer">
        <div v-if="readerSettings.showBottomBar" class="slider-row">
          <button class="step-btn" @click="handlePrevPage">‹</button>
          <input
            :value="currentPage"
            @input="jumpToPage(Number(($event.target as HTMLInputElement).value))"
            type="range"
            min="1"
            :max="Math.max(totalPages, 1)"
            class="page-slider"
          />
          <button class="step-btn" @click="handleNextPage">›</button>
        </div>

        <div v-if="readerSettings.enableBottomMenu" class="control-row">
          <button class="control-btn" :class="{ active: isDoublePage }" @click="togglePageLayout">
            {{ isDoublePage ? '📖 双页模式' : '📄 单页模式' }}
          </button>

          <button class="control-btn" :class="{ active: isRTL }" @click="toggleDirection">
            {{ isRTL ? '◀ RTL' : 'LTR ▶' }}
          </button>

          <button class="control-btn" :class="{ active: isWebtoon }" @click="toggleWebtoon">
            {{ isWebtoon ? '🎞️ 滚动模式' : '📜 Webtoon' }}
          </button>

          <button class="control-btn" @click="toggleFullscreen">
            {{ isFullscreen ? '📉 退出全屏' : '📺 全屏' }}
          </button>
        </div>

        <div v-if="readerSettings.showBottomBar" class="status-row">
          <span>{{ directionLabel }}</span>
          <span>{{ currentPage }} / {{ totalPages }} P</span>
        </div>
      </div>
    </Transition>

    <!-- ⚙️ 阅读器内设置抽屉 -->
    <Transition name="slide-left">
      <div v-if="showSettings" class="settings-drawer" @click.stop>
        <div class="drawer-header">
          <h3>⚙️ 阅读设置</h3>
          <button class="close-btn" @click="showSettings = false">✕</button>
        </div>

        <div class="drawer-body">
          <div class="setting-item">
            <label>阅读方向</label>
            <span class="direction-text">{{ directionLabel }}</span>
          </div>

          <div class="setting-item">
            <label>页面缩放</label>
            <select v-model="readerSettings.pageFit" class="setting-select">
              <option value="contain">匹配屏幕</option>
              <option value="cover">覆盖屏幕</option>
              <option value="width">适应宽度</option>
            </select>
          </div>

          <div class="setting-item column">
            <div class="setting-label-row">
              <label>自动翻页(秒)</label>
              <span>{{
                readerSettings.autoTurnInterval === 0
                  ? '关闭'
                  : `${readerSettings.autoTurnInterval}秒`
              }}</span>
            </div>
            <input
              v-model.number="readerSettings.autoTurnInterval"
              type="range"
              min="0"
              max="20"
              step="1"
              class="setting-range"
            />
          </div>

          <hr class="divider" />

          <div class="setting-item switch-row">
            <label>屏幕常亮</label>
            <input v-model="readerSettings.keepAwake" type="checkbox" class="toggle-switch" />
          </div>

          <div class="setting-item column">
            <div class="setting-label-row">
              <label>页面间隔 (px)</label>
              <span>{{ readerSettings.imageGap }}</span>
            </div>
            <input
              v-model.number="readerSettings.imageGap"
              type="range"
              min="0"
              max="20"
              step="5"
              class="setting-range"
            />
          </div>

          <hr class="divider" />

          <div class="setting-item switch-row">
            <label>自定义屏幕亮度</label>
            <input
              v-model="readerSettings.customBrightness"
              type="checkbox"
              class="toggle-switch"
            />
          </div>

          <div v-if="readerSettings.customBrightness" class="setting-item column">
            <div class="setting-label-row">
              <label>屏幕亮度</label>
              <span>{{ readerSettings.brightnessValue }}%</span>
            </div>
            <input
              v-model.number="readerSettings.brightnessValue"
              type="range"
              min="20"
              max="100"
              class="setting-range"
            />
          </div>

          <hr class="divider" />

          <button class="full-settings-btn" @click="router.push('/settings')">
            📋 前往完整阅读设置
          </button>
        </div>
      </div>
    </Transition>

    <!-- Round24：侧栏抽屉（缩略图/章节大纲/书签，仅本地） -->
    <ReaderSidebar
      v-model:active-tab="sidebarTab"
      :open="sidebarOpen"
      :pages="sidebarPages"
      :current-physical="currentPhysicalIndex0"
      :physical-total="totalPages"
      :chapters="readerChapters"
      :bookmarks="readerBookmarks"
      @close="sidebarOpen = false"
      @jump="jumpFromSidebar"
      @toggle-bookmark="toggleBookmark"
      @add-chapter="addChapter"
      @update-chapter="updateChapter"
      @remove-chapter="removeChapter"
    />
  </div>
</template>

<style scoped>
.reader-viewport {
  position: fixed;
  inset: 0;
  background-color: var(--app-bg-deep); /* Round19：跟随主题 */
  z-index: 3000;
  display: flex;
  flex-direction: column;
  user-select: none;
  overflow: hidden;
}

/* 屏幕亮度黑级滤镜 overlay */
.brightness-overlay {
  position: absolute;
  inset: 0;
  background-color: #000;
  pointer-events: none;
  z-index: 3008;
}

/* 浮动顶栏/底栏 */
.floating-header,
.floating-footer {
  position: absolute;
  left: 0;
  right: 0;
  background: var(--reader-bar-bg);
  backdrop-filter: blur(10px);
  z-index: 3010;
  padding: 12px 24px;
  display: flex;
  align-items: center;
}

.floating-header {
  top: 0;
  justify-content: space-between;
  gap: 10px; /* Round24：拉开按钮间距，避免挤成一团 */
  border-bottom: 1px solid var(--app-border-2);
}

.floating-footer {
  bottom: 0;
  flex-direction: column;
  gap: 12px;
  border-top: 1px solid var(--app-border-2);
}

/* Round24：顶栏按钮对齐原型（有底、边框、圆角，随主题；浅色=浅底深字、深色=深底浅字） */
.topbar-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3, var(--app-border-2));
  color: var(--app-text-strong);
  padding: 6px 11px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 1rem;
  white-space: nowrap;
  line-height: 1;
}
.topbar-btn.active {
  background: var(--app-accent);
  border-color: transparent;
  color: #fff;
}

.status-widgets {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 0.85rem;
  color: var(--app-text-2);
}
/* Round24：标题区占满中间——名称行（含来源标签）+ 章节路径小字，按原型两行布局 */
.header-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
  justify-content: center;
}
.header-info .title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.header-info .comic-title {
  font-size: 0.9rem;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.header-info .source-tag {
  font-size: 0.72rem;
  color: var(--app-text-2);
  white-space: nowrap;
  flex: none;
}
.header-info .chapter-path {
  font-size: 0.72rem;
  color: var(--app-text-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 控制条隐藏时的悬浮呼出按钮（底部居中，层级高于点击热区） */
.controls-reveal {
  position: fixed;
  left: 50%;
  transform: translateX(-50%);
  bottom: calc(16px + var(--safe-bottom, 0px));
  z-index: 3015;
  width: 44px;
  height: 44px;
  border-radius: 50%;
  border: 1px solid var(--reader-reveal-border);
  background: var(--reader-reveal-bg);
  color: var(--reader-reveal-color);
  font-size: 1.1rem;
  line-height: 1;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  transition:
    opacity 0.2s,
    background 0.2s;
}
.controls-reveal:hover {
  background: var(--reader-reveal-hover);
}

/* 呼出按钮淡入淡出 */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.25s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* 手柄连接指示器 */
.gamepad-indicator {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #4ade80;
}
.gamepad-name {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 主画布及图片缩放类 */
.canvas-stage {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  width: 100%;
  height: 100%;
  /* 纵向平移交给浏览器，横向滑动由 JS 手势接管 */
  touch-action: pan-y;
}

.click-zone {
  position: absolute;
  z-index: 3005;
}
/* 顶部：唤起阅读设置边栏 */
.top-zone {
  top: 0;
  left: 0;
  right: 0;
  height: 72px;
}
/* 中部三等分：左/右翻页、中唤起边栏 */
.prev-zone {
  top: 72px;
  bottom: 72px;
  left: 0;
  width: 33.33%;
}
.mid-zone {
  top: 72px;
  bottom: 72px;
  left: 33.33%;
  width: 33.34%;
}
.next-zone {
  top: 72px;
  bottom: 72px;
  right: 0;
  width: 33.33%;
}

.images-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  max-width: 100%;
  gap: 0;
}

/* 单页/双页图片容器：黑色兜底 + fit 布局 */
.page-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  max-width: 100%;
  background: var(--reader-page-bg);
  overflow: hidden;
}

.manga-page-img {
  max-height: 100vh;
  box-shadow: var(--reader-img-shadow);
}

/* 加载中 / 加载失败时隐藏图片，露出容器黑色兜底 */
.manga-page-img.img-hidden,
.webtoon-img.img-hidden {
  opacity: 0;
}

.page-item.fit-contain .manga-page-img {
  object-fit: contain;
  max-width: 100%;
}

.page-item.fit-cover .manga-page-img {
  object-fit: cover;
}

.page-item.fit-cover {
  width: 100vw;
  height: 100vh;
}

.page-item.fit-width {
  width: 100%;
}

.page-item.fit-width .manga-page-img {
  width: 100%;
  max-height: none;
}

/* 在线就近加载：空页黑色占位 + 加载动画 */
.page-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  color: var(--app-text-muted);
  background: var(--reader-page-bg);
  user-select: none;
}

.webtoon-placeholder {
  width: 100%;
  height: 60vh;
  min-height: 300px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  color: var(--app-text-muted);
  background: var(--reader-page-bg);
  user-select: none;
}

.placeholder-spinner {
  width: 34px;
  height: 34px;
  border: 3px solid var(--app-border-3);
  border-top-color: #7aa2f7;
  border-radius: 50%;
  animation: placeholder-spin 0.8s linear infinite;
}

@keyframes placeholder-spin {
  to {
    transform: rotate(360deg);
  }
}

.placeholder-text {
  font-size: 0.85rem;
  letter-spacing: 0.5px;
}

.images-wrapper.double-page .page-item {
  max-width: 50vw;
}

/* 图片加载失败提示 */
.page-item-label,
.webtoon-item-label {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--app-text-muted);
  font-size: 0.9rem;
  pointer-events: none;
  user-select: none;
}

/* 双击放大 */
.manga-page-img.zoomed,
.webtoon-img.zoomed {
  transform: scale(1.6);
  transition: transform 0.2s ease;
  z-index: 2;
}

/* 翻页动画（开启「翻页动画」设置时生效） */
.images-wrapper.turn-anim {
  animation: page-turn-in 0.3s ease;
}

@keyframes page-turn-in {
  from {
    opacity: 0;
    transform: translateX(20px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

.reader-viewport.rtl-mode .images-wrapper.turn-anim {
  animation-name: page-turn-in-rtl;
}

@keyframes page-turn-in-rtl {
  from {
    opacity: 0;
    transform: translateX(-20px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

/* 加载提示 */
.stage-loading,
.webtoon-loading {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  color: var(--app-text-3);
  font-size: 0.9rem;
  z-index: 3006;
}

/* 🎞️ Webtoon 连续滚动容器 */
.webtoon-container {
  flex: 1;
  overflow-y: auto;
  height: 100%;
  scroll-behavior: smooth;
  -webkit-overflow-scrolling: touch;
}

.webtoon-item {
  position: relative;
  width: 100%;
  background: var(--reader-page-bg);
}

.webtoon-img {
  display: block;
  width: 100%;
  max-width: 100%;
}

.webtoon-img.fit-contain {
  object-fit: contain;
}

/* 侧滑设置抽屉样式 */
.settings-drawer {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 300px;
  background: var(--app-surface-2);
  z-index: 3020;
  border-left: 1px solid var(--app-border-2);
  padding: 20px;
  display: flex;
  flex-direction: column;
  box-shadow: -5px 0 25px rgba(0, 0, 0, 0.5);
  color: var(--app-fg);
  overflow-y: auto;
}

.drawer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.close-btn {
  background: transparent;
  border: none;
  color: var(--app-text-2);
  font-size: 1.2rem;
  cursor: pointer;
}

.drawer-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.setting-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.9rem;
}

.setting-item.column {
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
}

.setting-label-row {
  display: flex;
  justify-content: space-between;
  color: var(--app-text-2);
  font-size: 0.85rem;
}

.direction-text {
  color: var(--app-accent);
  font-size: 0.85rem;
}

.setting-select {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-strong);
  padding: 4px 8px;
  border-radius: 4px;
}

.setting-range {
  accent-color: var(--app-accent);
  cursor: pointer;
}

.toggle-switch {
  accent-color: var(--app-accent);
  width: 18px;
  height: 18px;
  cursor: pointer;
}

.divider {
  border: none;
  border-top: 1px solid var(--app-border);
  margin: 4px 0;
}

.full-settings-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-accent);
  padding: 10px 16px;
  border-radius: 6px;
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.2s ease;
}

.full-settings-btn:hover {
  background-color: var(--app-surface-3-hover);
  border-color: var(--app-accent);
}

/* 底部状态信息行 */
.status-row {
  display: flex;
  justify-content: space-between;
  width: 100%;
  max-width: 600px;
  font-size: 0.8rem;
  color: var(--app-text-3);
}

/* 动画效果 */
.slide-left-enter-active,
.slide-left-leave-active {
  transition: transform 0.25s ease;
}

.slide-left-enter-from,
.slide-left-leave-to {
  transform: translateX(100%);
}

.slide-right-enter-active,
.slide-right-leave-active {
  transition: transform 0.25s ease;
}

.slide-right-enter-from,
.slide-right-leave-to {
  transform: translateX(100%);
}

.fade-top-enter-active,
.fade-top-leave-active,
.fade-bottom-enter-active,
.fade-bottom-leave-active {
  transition: opacity 0.2s ease;
}

.fade-top-enter-from,
.fade-top-leave-to,
.fade-bottom-enter-from,
.fade-bottom-leave-to {
  opacity: 0;
}

.slider-row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  max-width: 600px;
}

/* Round24：底部翻页按钮对齐原型（方形圆角、有底边框；此前无样式=默认浏览器按钮） */
.step-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-2);
  color: var(--app-text-strong);
  width: 34px;
  height: 34px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 1.1rem;
  flex: none;
  line-height: 1;
}

.page-slider {
  flex: 1;
  accent-color: var(--app-accent);
}

.control-row {
  display: flex;
  gap: 12px;
}

.control-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 6px 14px;
  border-radius: 16px;
  font-size: 0.82rem;
  cursor: pointer;
}

.control-btn.active {
  background: var(--app-accent);
  border-color: var(--app-accent);
  color: #fff;
}

/* 📱 移动形态（<1024px）：安全区 + 压缩 UI（手机/iPad 竖屏） */
@media (max-width: 1024px) {
  /* 顶部/底部浮动条避开刘海屏与 Home 条 */
  .floating-header {
    padding: calc(10px + var(--safe-top)) 12px 10px;
  }
  .floating-footer {
    padding: 10px 12px calc(10px + var(--safe-bottom));
  }
  /* 作品标题超长截断，避免挤压状态栏 */
  .header-info .comic-title {
    max-width: 36vw;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .status-widgets {
    gap: 6px;
  }
  /* 点击热区顶部 72px 太占屏，收窄到 56px */
  .top-zone {
    height: 56px;
  }
  .prev-zone,
  .mid-zone,
  .next-zone {
    top: 56px;
    bottom: 56px;
  }
  /* 阅读器内部设置抽屉占满屏宽 */
  .settings-drawer {
    width: 100vw;
    max-width: 100vw;
    padding: 20px 16px calc(20px + var(--safe-bottom));
  }
  /* Webtoon 滚动容器适配底部安全区 */
  .webtoon-container {
    padding-bottom: var(--safe-bottom);
  }
}

/* ─────────────────────────────────────────
   Round20-Bug2/Bug4：页列表加载失败错误层
   （纯主题变量，无硬编码色值，符合 Round19 规范）
   ───────────────────────────────────────── */
.reader-error-overlay {
  position: fixed;
  inset: 0;
  z-index: 9990;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.8);
  backdrop-filter: blur(4px);
}

.reader-error-card {
  width: min(88vw, 420px);
  padding: 28px 26px;
  border-radius: 14px;
  background: var(--reader-bar-bg);
  border: 1px solid var(--app-border-3);
  text-align: center;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.reader-error-icon {
  font-size: 2.4rem;
}

.reader-error-title {
  margin: 0;
  font-size: 1.05rem;
  font-weight: 700;
  color: var(--app-fg);
}

.reader-error-msg {
  margin: 0;
  font-size: 0.85rem;
  color: var(--app-text-2);
  line-height: 1.6;
  word-break: break-all;
  max-height: 40vh;
  overflow-y: auto;
}

.reader-error-actions {
  display: flex;
  justify-content: center;
  gap: 12px;
  margin-top: 8px;
}

.reader-error-btn {
  padding: 9px 22px;
  border-radius: 8px;
  border: 1px solid var(--app-border-3);
  background: var(--app-surface-3);
  color: var(--app-text-strong);
  font-size: 0.9rem;
  cursor: pointer;
  transition: opacity 0.15s;
}

.reader-error-btn:hover {
  opacity: 0.85;
}

.reader-error-btn.primary {
  background: var(--app-accent);
  border-color: transparent;
  color: #fff;
}
</style>
