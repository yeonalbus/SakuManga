import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getDetailPanelState, openDetailPanel, closeDetailPanel } from '@/stores/detailPanelStore'

/**
 * 在线列表页「左右分栏详情面板」Composable
 *
 * - 宽屏桌面（min-width:1025px 且非强制移动形态）：点击卡片 → 右侧内嵌详情面板，
 *   列表滚动位点由 keep-alive / 原生 DOM 天然保留，无需额外处理。
 * - 窄屏 / 强制移动形态：回退为全屏详情路由（与旧行为一致）。
 *
 * 面板开启状态按 route.path 存入 detailPanelStore，供组件因查询变化重建时恢复。
 */
const WIDE_QUERY = '(min-width: 1025px)'

export function useDetailPanel() {
  const route = useRoute()
  const router = useRouter()

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

    // 恢复当前路径上次打开的面板（组件因查询变化重建时生效）
    const saved = getDetailPanelState(route.path)
    if (saved?.open && saved.gid) {
      panelGid.value = saved.gid
      panelToken.value = saved.token
      isPanelOpen.value = true
    }
  })

  onUnmounted(() => {
    mql?.removeEventListener('change', syncWide)
    mql = null
    layoutObserver?.disconnect()
    layoutObserver = null
  })

  /** 打开详情：宽屏 → 内嵌面板；否则 → 全屏详情路由 */
  const openDetail = (comic: { id: string; token?: string }) => {
    if (!comic?.id) return
    if (isWide.value && comic.token) {
      panelGid.value = comic.id
      panelToken.value = comic.token
      isPanelOpen.value = true
      openDetailPanel(route.path, comic.id, comic.token)
    } else {
      router.push({ path: '/online/detail', query: { id: comic.id, token: comic.token || '' } })
    }
  }

  /** 收起面板 */
  const closePanel = () => {
    isPanelOpen.value = false
    panelGid.value = ''
    panelToken.value = ''
    closeDetailPanel(route.path)
  }

  return { isWide, isPanelOpen, panelGid, panelToken, openDetail, closePanel }
}
