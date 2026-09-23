/**
 * 「加载较新内容」的滚动锚定（Round36）
 *
 * 背景：在线列表向上加载较新内容时，新数据被 unshift 前置插入，而真实滚动容器
 * `#main-content` 的 scrollTop 数值不变（App.vue 还显式设了 `overflow-anchor: none`，
 * 浏览器原生 scroll anchoring 不介入）——后果是内容整体下推约一页，视口「倒退」到
 * 新加载页的开头，用户必须往下滑回原来那一本才能接着搜刮，节奏被打断。
 *
 * 方案（样板实测：5/4/3/2 列下垂直位移均 ≤ 0.5px）：
 *   1. 点击时记录锚点卡 = 当前列表第一本（DOM 里第一张 `.item-card`）
 *      相对 `#main-content` 可见区顶部的偏移
 *   2. 执行加载（store 的 loadBefore / 页面自实现的 loadBeforeFav），取得新增条数
 *   3. 等布局稳定（nextTick + 双 rAF，与 useDetailPanel 的补偿时机一致）
 *   4. `scrollTop += 锚点卡加载后偏移 − 加载前偏移`，让锚点卡像素级原地不动
 *   5. 给锚点卡加脉冲金框（与书签定位同款视觉，独立类名避免定时器互扰）
 *
 * ⚠️ 已知局限（已与珱垣确认接受）：E 站每页固定 25 张，4/3/2 列时 25 不整除，
 * 新页最后一行会与旧页首行混排——锚点卡垂直位置仍精确保持，但会「横跳一格」
 * （第 1 列 → 第 2 列）。彻底消除需页边界强制换行（`grid-column-start: 1`），
 * 代价是每页最后一行留空、顶部出现「孤卡」，观感更差，故不采用。
 *
 * 与书签定位（Round34）的关系：定位循环正在跑时 isLoading 守卫会拦下本次加载，
 * loader 返回 0 → 本组合式不做补偿也不高亮，两者不冲突。
 */
import { nextTick } from 'vue'
import { getMainContent } from '@/utils/scrollMemory'

/** 锚点卡脉冲类名（样式见 ItemCard.vue，复用书签脉冲的同一组 keyframes） */
const ANCHOR_PULSE_CLASS = 'load-anchor-pulse'
/** 脉冲总时长：0.9s × 3 次（与书签定位脉冲一致） */
const ANCHOR_PULSE_MS = 2800

/** 各锚点卡的脉冲定时器（避免连续点击时旧定时器提前摘掉新动画） */
const pulseTimers = new WeakMap<HTMLElement, number>()

/** 卡片元素相对 `#main-content` 可见区顶部的偏移（null = 不可测） */
const viewportOffset = (el: HTMLElement | null): number | null => {
  const main = getMainContent()
  if (!el || !main) return null
  return el.getBoundingClientRect().top - main.getBoundingClientRect().top
}

/** 等布局稳定：nextTick + 双 rAF（与 useDetailPanel 的补偿时机一致） */
const afterLayoutStable = async (): Promise<void> => {
  await nextTick()
  await new Promise<void>((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
  )
}

/** 脉冲高亮锚点卡（连续点击时重启动画并重置定时器） */
const pulseAnchor = (el: HTMLElement): void => {
  const prev = pulseTimers.get(el)
  if (prev !== undefined) window.clearTimeout(prev)
  el.classList.remove(ANCHOR_PULSE_CLASS)
  void el.offsetWidth // 强制重排：保证同一张卡连续两次点击时动画能重播
  el.classList.add(ANCHOR_PULSE_CLASS)
  pulseTimers.set(
    el,
    window.setTimeout(() => {
      el.classList.remove(ANCHOR_PULSE_CLASS)
      pulseTimers.delete(el)
    }, ANCHOR_PULSE_MS),
  )
}

export const usePrependAnchor = () => {
  /**
   * 执行「加载较新内容」并保持视口位置（锚点卡像素级不动 + 脉冲高亮）。
   *
   * @param loader 真正发起加载的函数，需返回本次新增条数
   *               （0 = 被守卫拦下 / 已到最新，此时不动滚动也不高亮）
   */
  const loadNewerKeepingPosition = async (loader: () => Promise<number>): Promise<void> => {
    const main = getMainContent()
    // 锚点卡 = 加载前列表第一本（DOM 顺序 = filteredComics 顺序，负向过滤后的第一张）
    const anchorEl = main?.querySelector<HTMLElement>('.item-card') ?? null
    const anchorGid = anchorEl?.dataset.gid ?? ''
    const offsetBefore = viewportOffset(anchorEl)
    const heightBefore = main?.scrollHeight ?? 0

    const added = await loader()
    // 没有新内容插入 → 视口本就没变，无需补偿
    if (!main || added <= 0) return

    await afterLayoutStable()

    const anchorAfterEl = anchorGid
      ? main.querySelector<HTMLElement>(`.item-card[data-gid="${anchorGid}"]`)
      : null

    // 优先按锚点卡实测偏移补偿（跨列数 / 跨形态都精确）；
    // 兜底用容器高度差（锚点卡被移除或 DOM 被替换时）
    const offsetAfter = viewportOffset(anchorAfterEl)
    main.scrollTop +=
      offsetBefore !== null && offsetAfter !== null
        ? offsetAfter - offsetBefore
        : main.scrollHeight - heightBefore

    if (anchorAfterEl) pulseAnchor(anchorAfterEl)
  }

  return { loadNewerKeepingPosition }
}
