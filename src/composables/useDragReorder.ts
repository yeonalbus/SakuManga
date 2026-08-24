// src/composables/useDragReorder.ts
// Round22：拖拽排序原语（书架列表 / 书架内本子排序视图共用）。
//
// 交互约定（决策 D1/D2）：
// - 触发：独立拖拽把手（pointerdown 即拖）或行内长按 300ms（可选，默认仅把手）；
// - 拖动中：被拖行「悬浮」为 fixed 幽灵跟随指针，列表保持原生滚动（不拦截 wheel/touch）；
// - 拖近容器上下边缘：rAF 自动滚动，滚动期间实时重算落位；
// - 松手：按指针 Y 与其余行中线比对得出落位索引 → onReorder(fromIndex, toIndex)
//   （toIndex 为「移除被拖行后数组」中的插入下标，调用方按 splice(from,1)+splice(to,0,item) 落位）。

import { ref, onBeforeUnmount } from 'vue'

export interface UseDragReorderOptions {
  /** 可滚动容器获取器（行查找与边缘自动滚动目标） */
  getScrollContainer: () => HTMLElement | null
  /** 行元素选择器（容器内按当前 DOM 顺序查找全部行） */
  rowSelector: string
  /** 行内长按触发毫秒；0 或省略 = 仅把手触发 */
  longPressMs?: number
  /** 幽灵卡内容文本（按行下标取标题等） */
  getGhostText: (index: number) => string
  /** 落位回调（from=原下标，to=移除被拖行后数组的插入下标） */
  onReorder: (fromIndex: number, toIndex: number) => void
}

export function useDragReorder(opts: UseDragReorderOptions) {
  const dragging = ref(false)
  const dragIndex = ref(-1)
  /** 落位插入下标（移除被拖行后数组空间） */
  const dropIndex = ref(-1)
  /** 幽灵卡 top（fixed 视口坐标） */
  const ghostTop = ref(0)
  /** 幽灵卡 left（fixed 视口坐标，跟随滚动容器左侧，左右各留 8px） */
  const ghostLeft = ref(0)
  /** 幽灵卡宽度（跟随滚动容器宽度；决策 D4：禁止视口全宽导致横向超出浮层/侧边栏） */
  const ghostWidth = ref(0)
  /** 幽灵卡高度（取被拖行实测高度） */
  const ghostHeight = ref(0)
  /** 拖动刚结束时置位，供行内 click 处理器抑制本次点击 */
  const suppressClick = ref(false)

  /** 展示用落位指示下标（原数组空间；dragging 时为有效值） */
  const indicatorIndex = ref(-1)

  let grabOffsetY = 0
  let lastClientY = 0
  let longPressTimer: ReturnType<typeof setTimeout> | null = null
  let scrollRaf = 0
  let scrollSpeed = 0

  const rows = (): HTMLElement[] => {
    const c = opts.getScrollContainer()
    if (!c) return []
    return Array.from(c.querySelectorAll(opts.rowSelector)) as HTMLElement[]
  }

  /** 按指针 Y 计算落位：遍历其余行（跳过被拖行），命中行中线则插其前 */
  const computeDropIndex = (y: number): number => {
    const list = rows()
    const from = dragIndex.value
    let k = 0
    for (let i = 0; i < list.length; i++) {
      if (i === from) continue
      const r = list[i].getBoundingClientRect()
      if (y < r.top + r.height / 2) return k
      k++
    }
    return k
  }

  const syncIndicator = () => {
    const k = dropIndex.value
    const from = dragIndex.value
    indicatorIndex.value = k >= from ? k + 1 : k
  }

  const startAutoScroll = () => {
    if (scrollRaf) return
    const tick = () => {
      if (!dragging.value) {
        scrollRaf = 0
        return
      }
      const c = opts.getScrollContainer()
      if (c && scrollSpeed !== 0) {
        c.scrollTop += scrollSpeed
        if (lastClientY !== 0) {
          dropIndex.value = computeDropIndex(lastClientY)
          syncIndicator()
        }
      }
      scrollRaf = requestAnimationFrame(tick)
    }
    scrollRaf = requestAnimationFrame(tick)
  }

  /**
   * 幽灵卡跟随指针，并钳制在滚动容器可视矩形内。
   * 决策 D3：拖近容器上/下边缘时指针会停在边缘附近，若不钳制，
   * 幽灵卡 top 会用负值/超底值把卡片推出显示范围（fixed 相对视口，
   * 但被浮层 panel 的 overflow:hidden 或视口裁切）。落位计算仍用真实指针 y。
   */
  const clampGhostTop = (top: number): number => {
    const c = opts.getScrollContainer()
    if (!c) return top
    const r = c.getBoundingClientRect()
    const minTop = r.top
    // 上界再留 1px 余量，避免幽灵卡边界与容器底边视觉贴合时被裁 1px
    const maxTop = Math.max(minTop, r.bottom - ghostHeight.value - 1)
    return Math.max(minTop, Math.min(top, maxTop))
  }

  /**
   * 幽灵卡横向跟随滚动容器。
   * 决策 D4：fixed 定位若用 CSS left/right 相对视口，卡片会横跨整个屏幕，
   * 在窄容器（书架浮层 440px / 侧边栏）场景大幅横向超出显示范围；
   * 故横向基准改为滚动容器可视矩形，左右各留 8px。
   */
  const syncGhostX = () => {
    const c = opts.getScrollContainer()
    if (!c) {
      ghostLeft.value = 8
      ghostWidth.value = window.innerWidth - 16
      return
    }
    const r = c.getBoundingClientRect()
    ghostLeft.value = r.left + 8
    ghostWidth.value = Math.max(0, r.width - 16)
  }

  const onPointerMove = (e: PointerEvent) => {
    if (!dragging.value) return
    lastClientY = e.clientY
    syncGhostX()
    ghostTop.value = clampGhostTop(e.clientY - grabOffsetY)
    dropIndex.value = computeDropIndex(e.clientY)
    syncIndicator()
    // 边缘自动滚动：靠近容器上下 48px 内加速（速度随距离增大，上限 24px/帧）
    const c = opts.getScrollContainer()
    if (c) {
      const r = c.getBoundingClientRect()
      const margin = 48
      if (e.clientY < r.top + margin) {
        scrollSpeed = -Math.min(24, (r.top + margin - e.clientY) / 3 + 4)
      } else if (e.clientY > r.bottom - margin) {
        scrollSpeed = Math.min(24, (e.clientY - (r.bottom - margin)) / 3 + 4)
      } else {
        scrollSpeed = 0
      }
    }
  }

  const onPointerUp = () => {
    if (!dragging.value) return
    const from = dragIndex.value
    const to = dropIndex.value
    endDrag()
    suppressClick.value = true
    if (from >= 0 && to >= 0 && from !== to) opts.onReorder(from, to)
  }

  const endDrag = () => {
    dragging.value = false
    dragIndex.value = -1
    dropIndex.value = -1
    indicatorIndex.value = -1
    scrollSpeed = 0
    if (scrollRaf) cancelAnimationFrame(scrollRaf)
    scrollRaf = 0
    if (longPressTimer) clearTimeout(longPressTimer)
    longPressTimer = null
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    window.removeEventListener('pointercancel', onPointerUp)
  }

  const beginDrag = (index: number, clientY: number) => {
    if (dragging.value || index < 0) return
    const list = rows()
    const row = list[index]
    if (!row) return
    const rect = row.getBoundingClientRect()
    grabOffsetY = clientY - rect.top
    ghostHeight.value = rect.height
    ghostTop.value = clampGhostTop(rect.top)
    syncGhostX()
    dragIndex.value = index
    dropIndex.value = index
    indicatorIndex.value = index
    dragging.value = true
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerUp)
    startAutoScroll()
  }

  /** 拖拽把手：按下即拖 */
  const onHandlePointerDown = (e: PointerEvent, index: number) => {
    if (dragging.value || e.button !== 0) return
    e.preventDefault()
    e.stopPropagation()
    beginDrag(index, e.clientY)
  }

  /** 行内长按触发（仅当配置 longPressMs>0 时生效；位移过大视为滚动不触发） */
  const onRowPointerDown = (e: PointerEvent, index: number) => {
    if (dragging.value || e.button !== 0) return
    const ms = opts.longPressMs ?? 0
    if (ms <= 0) return
    const startX = e.clientX
    const startY = e.clientY
    let moved = false
    const onMove = (ev: PointerEvent) => {
      if (Math.abs(ev.clientX - startX) > 8 || Math.abs(ev.clientY - startY) > 8) moved = true
    }
    window.addEventListener('pointermove', onMove)
    const cleanup = () => window.removeEventListener('pointermove', onMove)
    window.addEventListener('pointerup', cleanup, { once: true })
    window.addEventListener('pointercancel', cleanup, { once: true })
    if (longPressTimer) clearTimeout(longPressTimer)
    longPressTimer = setTimeout(() => {
      cleanup()
      if (moved) return // 长按期间发生了滚动/位移 → 视为滚动而非拖拽
      e.preventDefault()
      beginDrag(index, e.clientY)
    }, ms)
  }

  /** 行点击抑制检查（拖动刚结束时置位，视图点击处理器应先消费） */
  const consumeSuppressClick = (): boolean => {
    if (suppressClick.value) {
      suppressClick.value = false
      return true
    }
    return false
  }

  onBeforeUnmount(endDrag)

  return {
    dragging,
    dragIndex,
    dropIndex,
    indicatorIndex,
    ghostTop,
    ghostLeft,
    ghostWidth,
    ghostHeight,
    onHandlePointerDown,
    onRowPointerDown,
    consumeSuppressClick,
    ghostText: (index: number) => opts.getGhostText(index),
  }
}