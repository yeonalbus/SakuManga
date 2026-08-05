/**
 * 滚动位置记忆工具
 *
 * 应用的主滚动容器是 App.vue 中的 .main-content（overflow-y: auto），
 * 而不是 window。进入详情页等短暂加载状态时，内容高度收缩会把容器
 * scrollTop 压回 0，导致返回列表时"回到原点"。
 *
 * 这里按路由路径（path 级别，不带 query）记录离开时的滚动位置，
 * 返回该页面时再取用并恢复到 .main-content 上。
 */

const scrollCache = new Map<string, number>()

/** 记录某路由路径离开时的滚动位置（仅记录 > 0 的值，避免污染缓存） */
export const rememberScroll = (path: string, top: number): void => {
  if (top > 0) scrollCache.set(path, top)
}

/** 取用并移除某路由路径保存的滚动位置（一次性消费） */
export const takeScroll = (path: string): number | undefined => {
  const top = scrollCache.get(path)
  scrollCache.delete(path)
  return top
}

/** 获取主滚动容器元素 */
export const getMainContent = (): HTMLElement | null =>
  document.querySelector<HTMLElement>('#main-content')

/** 尝试立即恢复滚动位置；若目标尚未渲染完成（高度为 0），则用 rAF 重试 */
export const restoreScroll = (path: string, retries = 5): void => {
  const saved = takeScroll(path)
  if (saved === undefined) return

  let attempt = 0
  const tryRestore = (): void => {
    const el = getMainContent()
    if (el) {
      el.scrollTop = saved
      // 若目标内容尚未渲染（高度为 0，浏览器会把 scrollTop 钳制为 0），重试
      if (el.scrollTop === 0 && el.scrollHeight > 0 && attempt < retries) {
        attempt += 1
        requestAnimationFrame(tryRestore)
        return
      }
      if (el.scrollTop !== saved && attempt < retries) {
        attempt += 1
        requestAnimationFrame(tryRestore)
        return
      }
    }
  }

  requestAnimationFrame(tryRestore)
}
