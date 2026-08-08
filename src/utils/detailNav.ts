/**
 * 漫画详情「新标签导航」与「返回语义」公共工具（S10 / S11）
 *
 * 背景：S10 统一「在线窄屏 / 离线卡片」点击在浏览器新标签打开完整详情页；
 * 详情页返回按钮（S11）需区分两种来源：
 *   1. 由本应用 window.open 打开的新标签 → 返回 = 关闭标签（window.close）；
 *   2. 同标签路由跳转 → 返回 = 浏览器历史回退 / 首页兜底。
 *
 * 判定依据（双保险）：
 *   - window.opener 存在：window.open 建立的同源父子窗口关系；
 *   - sessionStorage 标记 saku_newtab_<id>：父标签在 window.open「之前」写入，
 *     新标签因「同源首次导航会复制 opener 的 sessionStorage」继承该标记；
 *     关闭标签即自动清理该会话，无需手动清除。
 */

const NEWTAB_KEY_PREFIX = 'saku_newtab_'

export type ComicNavSource = 'online' | 'offline'

export interface ComicNavTarget {
  /** 漫画 id（离线 = 本地记录 id；在线 = 画廊 gid） */
  id: string
  /** 在线画廊 token（离线场景忽略） */
  token?: string
  /** 来源模式；缺省按 online 处理 */
  source?: ComicNavSource
}

/** 写入「由本应用新标签打开」标记。必须在 window.open 之前调用，新标签才能继承 */
export function markComicOpenedInNewTab(id: string): void {
  if (!id) return
  try {
    sessionStorage.setItem(`${NEWTAB_KEY_PREFIX}${id}`, '1')
  } catch {
    /* 隐私模式 / 存储不可用时静默降级（仍有 window.opener 判定兜底） */
  }
}

/** 读取并消费新标签标记（读取后删除，避免残留污染同标签后续返回语义） */
export function consumeComicNewTabMark(id: string): boolean {
  if (!id) return false
  try {
    const key = `${NEWTAB_KEY_PREFIX}${id}`
    const hit = sessionStorage.getItem(key) === '1'
    if (hit) sessionStorage.removeItem(key)
    return hit
  } catch {
    return false
  }
}

/** 构造详情页路由 URL（应用 createWebHistory() 无 base，直接拼接绝对路径） */
export function buildDetailHref(comic: ComicNavTarget): string {
  if (!comic?.id) return ''
  if (comic.source === 'offline') {
    return `/offline/detail?id=${encodeURIComponent(comic.id)}`
  }
  const params = new URLSearchParams()
  params.set('id', comic.id)
  if (comic.token) params.set('token', comic.token)
  return `/online/detail?${params.toString()}`
}

/** 在新浏览器标签打开漫画详情，并写入新标签标记（供 S11 返回语义判断） */
export function openComicDetailInNewTab(comic: ComicNavTarget): void {
  if (!comic?.id) return
  markComicOpenedInNewTab(comic.id)
  const href = buildDetailHref(comic)
  if (href) window.open(href, '_blank')
}

/**
 * 判断当前详情页是否由本应用新标签打开：
 * - window.opener 非空（window.open 建立的父子关系）；
 * - 或 sessionStorage 新标签标记命中（opener 被导航 / 关闭时兜底）。
 * 命中则详情页返回按钮应 window.close() 关闭标签。
 */
export function isDetailNewTab(id: string): boolean {
  if (window.opener) return true
  return consumeComicNewTabMark(id)
}
