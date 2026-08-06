<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { getNextComicInQueue } from '@/stores/readingStore'
import { readerSettings, parseReadDirection } from '@/stores/readerSettings'
import type { OnlineComic } from '@/types/comic'
import { fetchOfflineComics } from '@/stores/comicStore'
import { http } from '@/utils/request'
import { API_BASE, TOKEN_KEY } from '@/config/api'

// 屏幕常亮 Wake Lock 的类型声明（避免 any）
interface WakeLockManager {
  request(type: 'screen'): Promise<WakeLockSentinel>
}
// 电量 API 的类型声明（避免 any）
interface BatteryManager {
  level: number
  charging: boolean
  chargingTime: number
  dischargingTime: number
}

const router = useRouter()
const route = useRoute()
const { toast, modal } = useUI()

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
const showThumbnailsPanel = ref(false) // 缩略图面板显隐
const isZoomed = ref(false) // 双击放大状态
const isLoading = ref(false) // 加载中

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
const loadComicPages = async () => {
  // 🎯 核心防刷：如果路由里根本没有 id（说明正在退出/跳转到其他页面），直接终止，绝不发请求
  const realId = route.query.id as string
  if (!realId) return

  isLoading.value = true
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
      // 📚 离线模式：请求本地画廊页列表接口
      const data = await http<{ total?: number; pages?: unknown[] }>(`/comics/${realId}/pages`)
      let pageCount = 0
      if (typeof data.total === 'number') {
        pageCount = data.total
      } else if (Array.isArray(data.pages)) {
        pageCount = data.pages.length
      }
      if (pageCount === 0) {
        toast.error('该画廊没有任何页面')
        isLoading.value = false
        return
      }
      totalPages.value = pageCount
      pageUrls.value = Array.from(
        { length: pageCount },
        (_, i) => `${API_BASE}/comics/${realId}/page/${i}`,
      )
    }

    // 恢复起始页码：优先路由 page 参数（预览图点击进入），其次历史进度
    const targetPage = Number(route.query.page)
    let startPage = 1
    if (Number.isInteger(targetPage) && targetPage >= 1 && targetPage <= totalPages.value) {
      startPage = targetPage
    } else {
      const last = getSavedPage(source.value, realId)
      startPage = Math.min(Math.max(1, last), totalPages.value)
    }
    currentPage.value = startPage
    if (isWebtoon.value) scrollToPage(startPage)
    // 缩略图进度条默认隐藏（按需通过底部区域 / 顶栏 ▦ 按钮唤起）
    showThumbnailsPanel.value = false
    // 初始就近补全当前页附近（在线模式空页懒加载）
    preloadNearby(startPage - 1)
  } catch (err) {
    console.error('加载画廊失败:', err)
    const msg = err instanceof Error ? err.message : '加载画廊失败'
    // 离线模式 404：漫画 id 可能已从本地库移除（列表/历史缓存过期），友好提示 + 刷新列表
    if (source.value === 'offline' && /找不到该漫画|not found|404/i.test(msg)) {
      const confirmed = await modal.confirm(
        '该漫画可能已从本地库移除，或本地扫描数据已过期。\n是否刷新离线列表后返回？',
        '找不到该漫画',
      )
      if (confirmed) {
        // 刷新失败也要保证能返回，避免阅读器卡死在该错误态
        try {
          await fetchOfflineComics()
        } catch (refreshErr) {
          console.error('刷新离线列表失败:', refreshErr)
        }
        router.back()
      }
      isLoading.value = false
      return
    }
    toast.error(msg)
  } finally {
    isLoading.value = false
  }
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
// --------------------------------------------------
const PROGRESS_STORAGE_KEY = 'saku_comic_progress'

// 1. 获取全量进度 Map { [source:id]: pageNumber }
const getProgressMap = (): Record<string, number> => {
  try {
    return JSON.parse(localStorage.getItem(PROGRESS_STORAGE_KEY) || '{}')
  } catch {
    return {}
  }
}

// 2. 保存当前作品的阅读进度（在线/离线分开存储，避免 id 冲突）
const saveProgress = (src: 'online' | 'offline', id: string, page: number) => {
  if (!id) return
  const map = getProgressMap()
  map[`${src}:${id}`] = page
  localStorage.setItem(PROGRESS_STORAGE_KEY, JSON.stringify(map))
}

// 3. 读取指定作品的历史进度（无记录则默认第 1 页）
const getSavedPage = (src: 'online' | 'offline', id: string): number => {
  const map = getProgressMap()
  return map[`${src}:${id}`] || 1
}

// --------------------------------------------------
// 📖 连贯读取队列调度核心
// --------------------------------------------------
const handleNextInQueue = async () => {
  // 查找队列里的下一作品
  const nextComic = getNextComicInQueue(comicId.value, source.value)

  if (nextComic) {
    const confirmed = await modal.confirm(
      `《${nextComic.title}》\n是否直接继续阅读清单中的下一本？`,
      '当前本子已全部读完 📖',
    )

    if (confirmed) {
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
    }
  } else {
    // 队列中已经没有更多本子了
    await modal.alert('清单中的所有本子都已经全部读完啦！🎉', '阅读完毕')
  }
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

// 中间三等分区：拉起阅读器设置边栏
const handleMidClick = () => {
  if (didDrag) return
  showSettings.value = !showSettings.value
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

// 缩略图跳页（跳转后保持进度条打开，便于连续导航）
const jumpToPage = (page: number) => {
  currentPage.value = page
  if (isWebtoon.value) {
    scrollToPage(page)
  }
}

// --------------------------------------------------
// ▦ 缩略图进度条：底部横条，窗口化渲染当前页附近 ±15 页
// --------------------------------------------------
const THUMB_RADIUS = 15
const thumbStrip = ref<HTMLElement | null>(null)
/** 窗口内待渲染的页码列表（随当前页滑动） */
const thumbPages = computed(() => {
  const list: number[] = []
  const start = Math.max(1, currentPage.value - THUMB_RADIUS)
  const end = Math.min(totalPages.value, currentPage.value + THUMB_RADIUS)
  for (let p = start; p <= end; p++) list.push(p)
  return list
})
/** 各缩略图的 DOM 引用（用于当前页滚动居中） */
const thumbStripItemRefs = ref<Record<number, HTMLElement | null>>({})
const setThumbRef = (page: number, el: unknown) => {
  thumbStripItemRefs.value[page] = (el as HTMLElement) || null
}
/** 当前页缩略图滚动居中 */
const scrollThumbIntoView = () => {
  nextTick(() => {
    if (!showThumbnailsPanel.value) return
    const el = thumbStripItemRefs.value[currentPage.value]
    if (el && thumbStrip.value) {
      el.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'center' })
    }
  })
}

// 缩略图进度条显隐（点击底部区域 / 顶栏 ▦ 按钮切换；功能开关关闭时不响应）
const toggleThumbStrip = () => {
  if (didDrag) return
  if (!readerSettings.showThumbnails) return
  showThumbnailsPanel.value = !showThumbnailsPanel.value
  if (showThumbnailsPanel.value) {
    scrollThumbIntoView()
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
      wakeLockSentinel.release()
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

// 4. 时钟与电量刷新
let clockTimer: ReturnType<typeof setInterval> | null = null
const currentTime = ref('')
const batteryLevel = ref('100%')
const updateStatusInfo = async () => {
  const now = new Date()
  currentTime.value = now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

  if ('getBattery' in navigator) {
    try {
      const getBattery = (navigator as Navigator & { getBattery?: () => Promise<BatteryManager> })
        .getBattery
      if (getBattery) {
        const b = await getBattery()
        batteryLevel.value = `${Math.round(b.level * 100)}%`
      }
    } catch {
      batteryLevel.value = '100%'
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
  // 缩略图进度条默认隐藏（按需通过底部区域 / 顶栏 ▦ 按钮唤起）
  showThumbnailsPanel.value = false
  updateStatusInfo()
  clockTimer = setInterval(updateStatusInfo, 30000) // 30秒更新一次状态
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeyDown)
  window.removeEventListener('resize', handleResize)
  if (autoTurnTimer) clearInterval(autoTurnTimer)
  if (clockTimer) clearInterval(clockTimer)
  if (wakeLockSentinel) wakeLockSentinel.release()
})

// --------------------------------------------------
// 监听与调度
// --------------------------------------------------

// 监听当前页码变化：实时触发预加载 + 保存进度
watch(currentPage, (newPg) => {
  if (comicId.value) {
    saveProgress(source.value, comicId.value, newPg)
  }
  nextTick(() => {
    if (!isWebtoon.value) {
      preloadImages(newPg - 1)
    }
    // 在线模式：就近补全当前页附近，保证翻页即时可用
    preloadNearby(newPg - 1)
    // 缩略图进度条跟随当前页滚动居中
    scrollThumbIntoView()
  })
})

// 监听路由 ID 切换时重新加载页列表
watch(
  () => route.query.id,
  (newId) => {
    if (!newId) return
    currentPage.value = 1
    isZoomed.value = false
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

    <Transition name="fade-top">
      <div v-if="showControls" class="floating-header">
        <button class="back-btn" @click="router.back()">‹ 退出阅读</button>

        <div class="header-info">
          <span class="comic-title">📖 作品ID: {{ comicId }}</span>
          <span class="source-tag">{{
            source === 'online' ? '🌐 在线流加载' : '📚 本地挂载'
          }}</span>
        </div>

        <div class="status-widgets">
          <span v-if="readerSettings.showClock" class="widget-item">🕒 {{ currentTime }}</span>
          <span v-if="readerSettings.showBattery" class="widget-item"> 🔋 {{ batteryLevel }} </span>
          <span v-if="readerSettings.showProgress" class="page-indicator"
            >{{ currentPage }} / {{ totalPages }}</span
          >
          <button
            v-if="readerSettings.showThumbnails"
            class="settings-btn"
            title="缩略图"
            @click.stop="showThumbnailsPanel = !showThumbnailsPanel"
          >
            ▦
          </button>
          <button class="settings-btn" @click.stop="showSettings = !showSettings" title="阅读设置">
            ⚙️
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
        class="canvas-stage"
        @click="handleStageClick"
        @mousedown="onCanvasMouseDown"
        @mousemove="onCanvasMouseMove"
        @mouseup="onCanvasMouseUp"
        @mouseleave="onCanvasMouseLeave"
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
          class="click-zone bottom-zone"
          @click.stop="toggleThumbStrip"
          title="显示/隐藏缩略图"
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
        <div v-if="readerSettings.showScrollbar && readerSettings.showProgress" class="slider-row">
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

        <div v-if="readerSettings.showBottomStatus" class="status-row">
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

          <div class="setting-item switch-row">
            <label>显示时钟</label>
            <input v-model="readerSettings.showClock" type="checkbox" class="toggle-switch" />
          </div>

          <div class="setting-item switch-row">
            <label>显示进度</label>
            <input v-model="readerSettings.showProgress" type="checkbox" class="toggle-switch" />
          </div>

          <div class="setting-item switch-row">
            <label>显示电量</label>
            <input v-model="readerSettings.showBattery" type="checkbox" class="toggle-switch" />
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

    <!-- ▦ 缩略图进度条（底部横条，当前页附近窗口化展示，点击跳页） -->
    <Transition name="fade-bottom">
      <div v-if="showThumbnailsPanel" class="thumb-strip" @click.stop>
        <div class="thumb-strip-track" ref="thumbStrip">
          <div
            v-for="p in thumbPages"
            :key="p"
            :ref="(el) => setThumbRef(p, el)"
            class="thumb-strip-item"
            :class="{ active: p === currentPage }"
            @click="jumpToPage(p)"
          >
            <img v-if="pageUrls[p - 1]" :src="pageUrls[p - 1]" :alt="`P${p}`" loading="lazy" />
            <div v-else class="thumb-strip-placeholder"></div>
            <span class="thumb-strip-num">{{ p }}</span>
          </div>
        </div>
        <div class="thumb-strip-progress">
          <span>P{{ currentPage }}</span>
          <span>/</span>
          <span>{{ totalPages }}</span>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.reader-viewport {
  position: fixed;
  inset: 0;
  background-color: #0d0d0f;
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
  background: rgba(18, 18, 22, 0.92);
  backdrop-filter: blur(10px);
  z-index: 3010;
  padding: 12px 24px;
  display: flex;
  align-items: center;
}

.floating-header {
  top: 0;
  justify-content: space-between;
  border-bottom: 1px solid #2d2d32;
}

.floating-footer {
  bottom: 0;
  flex-direction: column;
  gap: 12px;
  border-top: 1px solid #2d2d32;
}

.back-btn {
  background: #242428;
  border: 1px solid #3a3a3d;
  color: #fff;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
}

.status-widgets {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 0.85rem;
  color: #aaa;
}

.settings-btn {
  background: transparent;
  border: none;
  font-size: 1.2rem;
  cursor: pointer;
  padding: 2px 6px;
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
}

.click-zone {
  position: absolute;
  z-index: 3005;
}
/* 顶部 / 底部：唤起阅读设置边栏 */
.top-zone {
  top: 0;
  left: 0;
  right: 0;
  height: 72px;
}
.bottom-zone {
  bottom: 0;
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
  background: #000;
  overflow: hidden;
}

.manga-page-img {
  max-height: 100vh;
  box-shadow: 0 0 20px rgba(0, 0, 0, 0.8);
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
  color: #666;
  background: #000;
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
  color: #666;
  background: #000;
  user-select: none;
}

.placeholder-spinner {
  width: 34px;
  height: 34px;
  border: 3px solid #2a2a2f;
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
  color: #666;
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
  color: #888;
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
  background: #000;
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
  background: #18181c;
  z-index: 3020;
  border-left: 1px solid #2d2d32;
  padding: 20px;
  display: flex;
  flex-direction: column;
  box-shadow: -5px 0 25px rgba(0, 0, 0, 0.5);
  color: #eee;
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
  color: #aaa;
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
  color: #aaa;
  font-size: 0.85rem;
}

.direction-text {
  color: #007acc;
  font-size: 0.85rem;
}

.setting-select {
  background: #242428;
  border: 1px solid #38383c;
  color: #fff;
  padding: 4px 8px;
  border-radius: 4px;
}

.setting-range {
  accent-color: #007acc;
  cursor: pointer;
}

.toggle-switch {
  accent-color: #007acc;
  width: 18px;
  height: 18px;
  cursor: pointer;
}

.divider {
  border: none;
  border-top: 1px solid #2a2a2d;
  margin: 4px 0;
}

.full-settings-btn {
  background: #242428;
  border: 1px solid #3a3a3d;
  color: #007acc;
  padding: 10px 16px;
  border-radius: 6px;
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.2s ease;
}

.full-settings-btn:hover {
  background-color: #2e2e33;
  border-color: #007acc;
}

/* ▦ 缩略图进度条（底部横条） */
.thumb-strip {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 128px;
  background: rgba(14, 14, 17, 0.94);
  backdrop-filter: blur(10px);
  border-top: 1px solid #2d2d32;
  z-index: 3012;
  padding-top: 10px;
  box-shadow: 0 -6px 20px rgba(0, 0, 0, 0.5);
  cursor: default;
}

.thumb-strip-track {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  overflow-x: auto;
  overflow-y: hidden;
  height: 96px;
  padding: 2px 12px;
  scrollbar-width: thin;
  scrollbar-color: #3a3a3f transparent;
}

.thumb-strip-track::-webkit-scrollbar {
  height: 6px;
}

.thumb-strip-track::-webkit-scrollbar-thumb {
  background: #3a3a3f;
  border-radius: 3px;
}

.thumb-strip-item {
  position: relative;
  flex: 0 0 auto;
  width: 62px;
  height: 86px;
  border-radius: 4px;
  overflow: hidden;
  border: 2px solid transparent;
  cursor: pointer;
  background: #121214;
  transition:
    border-color 0.15s,
    transform 0.15s;
}

.thumb-strip-item img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.thumb-strip-item.active {
  border-color: #007acc;
  transform: translateY(-2px);
}

.thumb-strip-placeholder {
  width: 100%;
  height: 100%;
  background: linear-gradient(180deg, #1a1a1e, #0d0d0f);
}

.thumb-strip-num {
  position: absolute;
  bottom: 2px;
  right: 2px;
  background: rgba(0, 0, 0, 0.8);
  padding: 0 4px;
  font-size: 0.62rem;
  border-radius: 2px;
  color: #eee;
}

.thumb-strip-progress {
  position: absolute;
  top: 8px;
  right: 14px;
  display: flex;
  gap: 4px;
  font-size: 0.72rem;
  color: #aaa;
  background: rgba(0, 0, 0, 0.55);
  padding: 2px 8px;
  border-radius: 10px;
  pointer-events: none;
}

/* 底部状态信息行 */
.status-row {
  display: flex;
  justify-content: space-between;
  width: 100%;
  max-width: 600px;
  font-size: 0.8rem;
  color: #888;
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

.page-slider {
  flex: 1;
  accent-color: #007acc;
}

.control-row {
  display: flex;
  gap: 12px;
}

.control-btn {
  background: #242428;
  border: 1px solid #38383c;
  color: #ccc;
  padding: 6px 14px;
  border-radius: 16px;
  font-size: 0.82rem;
  cursor: pointer;
}

.control-btn.active {
  background: #007acc;
  border-color: #007acc;
  color: #fff;
}
</style>
