import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useUI } from '@/composables/useUI'
import {
  getDetailPanelState,
  openDetailPanel,
  closeDetailPanel,
  migrateDetailPanel,
} from '@/stores/detailPanelStore'
import { openComicDetailInNewTab } from '@/utils/detailNav'

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

export function useDetailPanel() {
  const route = useRoute()
  const { toast } = useUI()

  const isWide = ref(false)
  const isPanelOpen = ref(false)
  const panelGid = ref('')
  const panelToken = ref('')

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
   * 打开详情：
   * - 窄屏 / 强制移动 → 全屏详情路由
   * - 宽屏且面板已打开 → 切换面板内容
   * - 宽屏且面板已关闭 → window.open 新标签打开完整详情（与「画廊详情 ↗」一致），
   *   同时记录 gid/token，便于操作菜单「详情页面」重新唤起
   */
  const openDetail = (comic: { id: string; token?: string }) => {
    if (!comic?.id) return
    if (isWide.value && comic.token) {
      panelGid.value = comic.id
      panelToken.value = comic.token
      if (isPanelOpen.value) {
        isPanelOpen.value = true
        openDetailPanel(route.fullPath, comic.id, comic.token)
      } else {
        // S10：宽屏且面板已关闭 → 新标签打开完整详情（统一入口，写新标签标记）
        openComicDetailInNewTab({ id: comic.id, token: comic.token, source: 'online' })
      }
    } else {
      // S10：在线窄屏 → 新标签打开完整详情（与宽屏面板关闭行为一致）
      openComicDetailInNewTab({ id: comic.id, token: comic.token || '', source: 'online' })
    }
  }

  /**
   * 收起面板：仅隐藏，保留 gid/token（供「详情页面」重新唤起）
   */
  const closePanel = () => {
    isPanelOpen.value = false
    closeDetailPanel(route.fullPath)
  }

  /**
   * 切换面板显隐（操作菜单「详情页面」栏目）：
   * - 已打开 → 收起
   * - 已关闭且有上次内容 → 重新唤起
   */
  const togglePanel = () => {
    if (isPanelOpen.value) {
      closePanel()
    } else if (panelGid.value) {
      isPanelOpen.value = true
      openDetailPanel(route.fullPath, panelGid.value, panelToken.value)
    } else {
      // 未选中任何卡片时给出反馈，而非静默 no-op（S3）
      toast.info('请先点选一张卡片')
    }
  }

  return { isWide, isPanelOpen, panelGid, panelToken, openDetail, closePanel, togglePanel }
}
