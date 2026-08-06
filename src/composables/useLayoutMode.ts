/**
 * 布局模式 Composable
 *
 * 将「当前设备生效的布局模式」（auto / desktop / mobile）解析为「有效布局形态」，
 * 并写到 <html data-layout="desktop|mobile"> 上，供各组件 CSS 通过属性选择器消费。
 *
 * - auto    → 跟随视口：宽度 ≤1024px 为 mobile，否则 desktop（与全局 @media 断点一致）
 * - desktop → 强制桌面形态（覆盖窄视口的移动形态；窄屏防溢出规则不受影响）
 * - mobile  → 强制移动形态（宽视口也应用移动布局）
 *
 * 额外维护 data-layout-force 标记：仅「手动 desktop/mobile」时存在，
 * 供需要区分「自动渐进 vs 手动强制」的规则使用（如 GridContainer 网格列数）。
 *
 * 与主题 data-theme（main.ts applyTheme）并列挂在 <html> 上，二者互不影响。
 */
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { getEffectiveMode } from '@/stores/styleSettings'

/** 移动形态断点（与全局各组件 @media (max-width: 1024px) 保持一致，覆盖 iPad 竖屏 768-1032） */
const MOBILE_BREAKPOINT = '(max-width: 1024px)'

export type EffectiveLayout = 'desktop' | 'mobile'

/** 解析 layoutMode → 有效布局形态 */
export function resolveEffectiveLayout(mode: string, isNarrow: boolean): EffectiveLayout {
  if (mode === 'desktop') return 'desktop'
  if (mode === 'mobile') return 'mobile'
  return isNarrow ? 'mobile' : 'desktop'
}

/** 把当前布局模式写到 <html> 属性（data-layout + data-layout-force） */
export function applyLayoutMode(): void {
  const mode = getEffectiveMode()
  const isNarrow = window.matchMedia(MOBILE_BREAKPOINT).matches
  const layout = resolveEffectiveLayout(mode, isNarrow)
  const el = document.documentElement
  el.setAttribute('data-layout', layout)
  if (mode === 'auto') {
    el.removeAttribute('data-layout-force')
  } else {
    el.setAttribute('data-layout-force', '')
  }
}

export function useLayoutMode() {
  const effectiveLayout = ref<EffectiveLayout>(
    resolveEffectiveLayout(getEffectiveMode(), window.matchMedia(MOBILE_BREAKPOINT).matches),
  )

  const sync = () => {
    const isNarrow = window.matchMedia(MOBILE_BREAKPOINT).matches
    const layout = resolveEffectiveLayout(getEffectiveMode(), isNarrow)
    effectiveLayout.value = layout
    applyLayoutMode()
  }

  let mql: MediaQueryList | null = null

  onMounted(() => {
    mql = window.matchMedia(MOBILE_BREAKPOINT)
    mql.addEventListener('change', sync)
    applyLayoutMode() // 挂载后立即应用，保证刷新后 <html> 属性正确
  })
  onUnmounted(() => {
    mql?.removeEventListener('change', sync)
  })

  // 当前设备槽位的布局模式被改写（setLayoutMode）时立即重新解析
  watch(getEffectiveMode, sync)

  return { effectiveLayout }
}
