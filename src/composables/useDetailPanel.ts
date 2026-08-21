import { ref, nextTick, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import {
  getDetailPanelState,
  openDetailPanel,
  closeDetailPanel,
  migrateDetailPanel,
} from '@/stores/detailPanelStore'
import { openComicDetailInNewTab } from '@/utils/detailNav'
// Round12：面板开/关引起网格重排后，滚动补偿保持被锚定卡片在视线焦点
import { getMainContent } from '@/utils/scrollMemory'

/**
 * 在线列表页「左右分栏详情面板」Composable
 *
 * - 宽屏桌面（min-width:1025px 且非强制移动形态）：点击卡片 → 右侧内嵌详情面板，
 *   列表滚动位点由 keep-alive / 原生 DOM 天然保留，无需额外处理。
 * - 窄屏 / 强制移动形态：回退为全屏详情路由（与旧行为一致）。
 *
 * 交互约定（Round5 优化）：
 * - 面板「✕ 收起」仅隐藏面板，保留 gid/token，可随时通过操作菜单「详情页面」重新唤起。
 * - 面板已关闭时点击卡片：不再打开面板，而是用 window.open 新标签打开完整详情页
 *   （与面板标题「画廊详情 ↗」行为完全一致）。
 *
 * 面板开启状态按 route.fullPath 存入 detailPanelStore（含 query），供组件因查询变化重建时恢复；
 * 旧 route.path key 读取时自动迁移（S3）。
 */
const WIDE_QUERY = '(min-width: 1025px)'

// ─────────────────────────────────────────────────────────────
// Round12 滚动补偿（点开画廊保持视线焦点）
//
// 根因：面板展开时 .online-split 列宽被压缩 360~420px，且 GridContainer
// 按面板状态注入 --card-cols（如 4→3），卡片网格重排；#main-content 保留
// scrollTop 不变，导致被点卡片离开原视口位置（桌面「挤下去」/ iPad「挤走」）。
// 方案：展开前捕获锚定卡片相对滚动容器的视口偏移 → 等布局稳定（nextTick +
// 双 rAF）后，把 scrollTop 平移位移差，让锚定卡片回到原视口位置。
// ─────────────────────────────────────────────────────────────

/** 卡片元素相对 #main-content 顶部的视口偏移（null = 不可测） */
const cardViewportOffset = (el: HTMLElement | null): number | null => {
  if (!el) return null
  const main = getMainContent()
  if (!main) return null
  return el.getBoundingClientRect().top - main.getBoundingClientRect().top
}

/** 按 gid 在当前列表内查找卡片元素（ItemCard 根节点带 data-gid） */
const findCardByGid = (gid: string): HTMLElement | null => {
  const main = getMainContent()
  if (!main || !gid) return null
  return main.querySelector<HTMLElement>(`.item-card[data-gid="${gid}"]`)
}

/** 视口内最顶部的可见卡片（关闭面板/工具栏切换时的锚点） */
const findTopmostVisibleCard = (): HTMLElement | null => {
  const main = getMainContent()
  if (!main) return null
  const mainTop = main.getBoundingClientRect().top
  let best: HTMLElement | null = null
  main.querySelectorAll<HTMLElement>('.item-card').forEach((el) => {
    const rect = el.getBoundingClientRect()
    if (rect.bottom <= mainTop) return // 完全在视口上方
    if (!best || rect.top < best.getBoundingClientRect().top) best = el
  })
  return best
}

/**
 * 延迟到布局稳定后，平移 #main-content.scrollTop，使锚定卡片回到原视口位置。
 * 若锚定元素在等待期间被移除（列表刷新等），静默放弃。
 */
const compensateAfterLayout = async (anchor: HTMLElement | null, before: number | null) => {
  if (!anchor || before === null) return
  await nextTick()
  await new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r())))
  const main = getMainContent()
  if (!main || !document.contains(anchor)) return
  const after = cardViewportOffset(anchor)
  if (after === null) return
  main.scrollTop += after - before
}

export function useDetailPanel() {
  const route = useRoute()

  const isWide = ref(false)
  const isPanelOpen = ref(false)
  const panelGid = ref('')
  const panelToken = ref('')
  // Round7-任务6：面板当前内容来源是否为历史页（影响「立即阅读」起始页）
  const panelFromHistory = ref(false)

  let mql: MediaQueryList | null = null
  let layoutObserver: MutationObserver | null = null

  // 宽屏判定：视口 > 1025px 且非强制移动形态（与 <html data-layout> 保持一致）
  const syncWide = () => {
    const layout = document.documentElement.getAttribute('data-layout')
    const wideViewport = window.matchMedia(WIDE_QUERY).matches
    isWide.value = wideViewport && layout !== 'mobile'
  }

  onMounted(() => {
    mql = window.matchMedia(WIDE_QUERY)
    mql.addEventListener('change', syncWide)
    layoutObserver = new MutationObserver(syncWide)
    layoutObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-layout'],
    })
    syncWide()

    // 恢复当前路径上次打开的面板（按 fullPath，含 query；兼容旧 route.path key）
    const saved =
      getDetailPanelState(route.fullPath) ?? migrateDetailPanel(route.fullPath, route.path)
    if (saved?.open && saved.gid) {
      panelGid.value = saved.gid
      panelToken.value = saved.token
      isPanelOpen.value = true
    } else if (saved?.gid) {
      // 面板处于收起状态：保留 gid/token 以便「详情页面」重新唤起
      panelGid.value = saved.gid
      panelToken.value = saved.token
    }
  })

  onUnmounted(() => {
    mql?.removeEventListener('change', syncWide)
    mql = null
    layoutObserver?.disconnect()
    layoutObserver = null
  })

  /**
   * 打开详情（宽屏点击卡片一律打开/切换小详情面板）：
   * - 宽屏 → 自动打开/切换右侧小详情面板；新标签仅保留给 Ctrl/中键点击与「画廊详情 ↗」
   * - 窄屏 / 强制移动 → 新标签打开完整详情（面板不渲染，回退全屏详情）
   */
  const openDetail = (
    comic: { id: string; token?: string },
    opts: { fromHistory?: boolean } = {},
  ) => {
    if (!comic?.id) return
    // Round12：捕获被点击卡片作为滚动补偿锚点（面板未开时才需要，已开则布局不变）
    const wasOpen = isPanelOpen.value
    const anchorEl = isWide.value && !wasOpen ? findCardByGid(comic.id) : null
    const anchorBefore = isWide.value && !wasOpen ? cardViewportOffset(anchorEl) : null
    panelGid.value = comic.id
    panelToken.value = comic.token || ''
    panelFromHistory.value = !!opts.fromHistory
    if (isWide.value) {
      // 宽屏：点击卡片自动打开/切换小详情面板（OnlineDetail 对空 token 有兜底）
      isPanelOpen.value = true
      openDetailPanel(route.fullPath, comic.id, comic.token || '', opts.fromHistory)
      // Round12：布局稳定后把被点卡片平移回原视口位置（保持视线焦点）
      if (!wasOpen) void compensateAfterLayout(anchorEl, anchorBefore)
    } else {
      // 窄屏 / 强制移动：面板不渲染，新标签打开完整详情
      openComicDetailInNewTab({
        id: comic.id,
        token: comic.token || '',
        source: 'online',
        resume: opts.fromHistory,
      })
    }
  }

  /**
   * 收起面板：仅隐藏，保留 gid/token（供「详情页面」重新唤起）
   */
  const closePanel = () => {
    // Round12：关闭时同样补偿（解决 iPad「关闭后乱滑动」）。
    // 锚点优先面板对应卡片（若仍可见），否则视口内最顶可见卡片。
    const wasOpen = isPanelOpen.value
    const anchorEl = isWide.value && wasOpen
      ? (findCardByGid(panelGid.value) ?? findTopmostVisibleCard())
      : null
    const anchorBefore = isWide.value && wasOpen ? cardViewportOffset(anchorEl) : null
    isPanelOpen.value = false
    closeDetailPanel(route.fullPath)
    if (wasOpen) void compensateAfterLayout(anchorEl, anchorBefore)
  }

  /**
   * 切换面板显隐（操作菜单「详情页面」栏目）：
   * - 已打开 → 收起
   * - 已关闭且有上次内容 → 重新唤起
   * - 已关闭且从未点选卡片 → 打开占位面板（提示先选卡片），而非 toast/静默
   */
  const togglePanel = () => {
    if (isPanelOpen.value) {
      closePanel()
    } else {
      // Round12：工具栏打开面板（无点击卡片）→ 锚定视口内最顶可见卡片保持视野稳定
      const anchorEl = isWide.value ? findTopmostVisibleCard() : null
      const anchorBefore = isWide.value ? cardViewportOffset(anchorEl) : null
      isPanelOpen.value = true
      // 已点选过卡片 → 唤起该内容；否则打开占位空态（不写 store，避免持久化空态）
      if (panelGid.value) {
        openDetailPanel(route.fullPath, panelGid.value, panelToken.value)
      }
      void compensateAfterLayout(anchorEl, anchorBefore)
    }
  }

  return {
    isWide,
    isPanelOpen,
    panelGid,
    panelToken,
    panelFromHistory,
    openDetail,
    closePanel,
    togglePanel,
  }
}
