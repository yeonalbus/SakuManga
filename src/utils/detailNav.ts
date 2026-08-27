/**
 * 漫画详情「新标签导航」与「返回语义」公共工具（S10 / S11 / Round21 平台分流）
 *
 * 背景：S10 统一「在线窄屏 / 离线卡片」点击在浏览器新标签打开完整详情页；
 * 详情页返回按钮（S11）需区分两种来源：
 *   1. 由本应用 window.open 打开的新标签 → 返回 = 关闭标签（window.close）；
 *   2. 同标签路由跳转 → 返回 = 浏览器历史回退 / 首页兜底。
 *
 * Round21（平台分流）：PC 桌面（宽视口 + 非 PWA）内容类跳转恢复新标签，
 * PWA / 窄屏维持 SPA 同标签。返回关闭标签仅当「opener 存活（来源标签在）+
 * 当前路由 == 标签入口路由（防误关）+ 非单标签」三者同时成立，否则回来源。
 */

import { captureActiveListState } from '@/utils/scrollMemory'
// Round21：openContentTab 弹窗被拦截时降级同标签跳转（需访问 router 实例）
import router from '@/router'

// Round21：新标签「标签入口路由」记录（返回时判定「当前页==入口才允许 close」）
const TAB_ENTRY_KEY_PREFIX = 'saku_tab_entry_'

/**
 * Round15-Bug3：判断是否独立 PWA 窗口（iOS 添加到主屏幕 / Android standalone）。
 * PWA 下 window.open 受限（不产生可继承 sessionStorage 的新标签），
 * 返回语义需走「同标签导航 + 来源记录」，否则 router.back() 会触发整页刷新。
 */
export function isStandalonePWA(): boolean {
  if (typeof window === 'undefined') return false
  if (window.matchMedia('(display-mode: standalone)').matches) return true
  if ((window.navigator as unknown as { standalone?: boolean }).standalone === true) return true
  return false
}

/**
 * Round21：PC 桌面判定——内容类跳转（详情/阅读）是否开新标签。
 * 条件：宽视口（≥1025px）且非 PWA standalone 且非强制移动形态。
 * PWA（iPad/Android 主屏）与窄屏一律同标签 SPA（Round15/16 修复不得回归）。
 */
export function contentOpensNewTab(): boolean {
  if (typeof window === 'undefined') return false
  if (isStandalonePWA()) return false
  const layout = document.documentElement.getAttribute('data-layout')
  if (layout === 'mobile') return false
  return window.matchMedia('(min-width: 1025px)').matches
}

// ── Round21：标签入口路由记录（防误关） ──

/** 写入「该 id 被以新标签打开时的入口路由」（仅 window.open 成功后才写） */
function setTabEntry(id: string, href: string): void {
  if (!id || !href) return
  try {
    sessionStorage.setItem(`${TAB_ENTRY_KEY_PREFIX}${id}`, href)
  } catch {
    /* 存储不可用时静默降级 */
  }
}

/** 读取标签入口路由（不消费，标签生命周期内保留；关闭后随会话消失） */
function readTabEntry(id: string): string | undefined {
  if (!id) return undefined
  try {
    return sessionStorage.getItem(`${TAB_ENTRY_KEY_PREFIX}${id}`) || undefined
  } catch {
    return undefined
  }
}

/**
 * Round21：统一内容页打开（详情/阅读器）。
 * - 应开新标签（PC 桌面 / force 强制）→ 记录来源 + 入口路由，window.open；
 *   弹窗被拦截（open 返回 null）→ 不写入口标记，降级同标签跳转；
 * - 不应开新标签（PWA/窄屏）→ 同标签 SPA 跳转。
 * Round26-Bug：来源状态（recordBackStateForDetail）无条件记录——此前仅新标签分支记录，
 * PWA 同标签跳转（详情「阅读」、阅读清单、面板阅读等）退出时 consumeBackState 未命中，
 * 只能 history.back() 逐帧回退（穿过所有中间页 / PWA 栈异常）。统一记录后，
 * 阅读器/详情退出可一步 replace 回进入前的页面。
 * @returns true=已在新标签打开
 */
export function openContentTab(opts: { href: string; id: string }, forceNewTab = false): boolean {
  const { href, id } = opts
  if (!href || !id) return false
  const shouldNewTab = forceNewTab || contentOpensNewTab()
  // 无条件记录来源（新标签继承 sessionStorage 需在 window.open 之前写入；同标签供退出回源）
  recordBackStateForDetail({ id })
  if (!shouldNewTab) {
    router.push(href)
    return false
  }
  const w = window.open(href, '_blank')
  if (w) {
    setTabEntry(id, href)
    return true
  }
  // 弹窗被拦截：降级同标签跳转（不写标记，避免残留导致后续误判新标签）
  router.push(href)
  return false
}

/**
 * Round21：返回时是否应关闭当前标签页。
 * 规则（决策 D2=A+自定义 / D5=A）：
 * ① 存在来源标签（opener 存活）——关闭后用户仍有可回页面；
 * ② 当前路由 == 该标签被打开时的入口路由——标签内后续 SPA 导航不误关。
 * 仅剩单标签（无 opener / opener 已关）或已离开入口页 → false（回来源不关）。
 */
export function shouldCloseTab(id: string, currentFullPath: string): boolean {
  if (!id || typeof window === 'undefined') return false
  if (!window.opener || window.opener.closed) return false
  const entry = readTabEntry(id)
  return !!entry && entry === currentFullPath
}

export type ComicNavSource = 'online' | 'offline'

export interface ComicNavTarget {
  /** 漫画 id（离线 = 本地记录 id；在线 = 画廊 gid） */
  id: string
  /** 在线画廊 token（离线场景忽略） */
  token?: string
  /** 来源模式；缺省按 online 处理 */
  source?: ComicNavSource
  /** Round7：历史入口打开详情时标记，详情页「立即阅读」从上次位置开始 */
  resume?: boolean
}

/** 构造详情页路由 URL（应用 createWebHistory() 无 base，直接拼接绝对路径） */
export function buildDetailHref(comic: ComicNavTarget): string {
  if (!comic?.id) return ''
  if (comic.source === 'offline') {
    const base = `/offline/detail?id=${encodeURIComponent(comic.id)}`
    return comic.resume ? `${base}&resume=1` : base
  }
  const params = new URLSearchParams()
  params.set('id', comic.id)
  if (comic.token) params.set('token', comic.token)
  if (comic.resume) params.set('resume', '1')
  return `/online/detail?${params.toString()}`
}

// ─────────────────────────────────────────────────────────────
// Round7：返回来源列表状态记录
// 父列表页在 openComicDetailInNewTab 前把 { fromPath, top, page } 写入
// sessionStorage（key = saku_back_<id>）。新标签在 opener 已关闭时，
// 读取并回填到内存列表状态缓存（rememberListState），再 router.replace 回
// 来源列表，由列表页 takeListState 恢复滚动与页码。
// ─────────────────────────────────────────────────────────────

/**
 * Round26-Bug：来源记录升级为「栈」结构（同 id 可压多层）。
 *
 * 背景：此前为单值覆盖，存在一个致命漏洞——详情页 A 点「阅读」（在线阅读 id=gid=A）
 * 会用「来源=详情页 A」覆盖 saku_back_A，导致退出阅读回到详情页后，详情页自身的
 * 来源（列表页）丢失，再退出详情只能回首页而非原始列表。
 *
 * 栈语义：每次进入内容页（详情/阅读器）push 一层来源；退出时 pop 最近一层。
 * 例：列表 → 详情A(push 列表) → 阅读A(push 详情A) → 退出阅读(pop→详情A)
 *     → 退出详情(pop→列表) —— 全程一步回退，不丢来源。
 *
 * 兼容旧格式：历史遗留的单值 JSON（DetailBackState 对象）按单元素栈读取。
 */
const BACK_KEY_PREFIX = 'saku_back_'

/** 新标签返回时所需的来源列表状态 */
export interface DetailBackState {
  /** 来源列表路由路径（path 级别，不带 query；用作 rememberListState 的 key） */
  fromPath: string
  /** Round17.2：来源列表完整路径（含 query，如 /offline/bookshelf?id=xxx，router.replace 用） */
  fromFullPath?: string
  /** 来源列表滚动位置 */
  top: number
  /** 来源列表分页页码（可选） */
  page?: number
}

/** 解析存储值：兼容旧单值对象与新数组栈；非法返回空栈 */
function parseBackStack(raw: string | null): DetailBackState[] {
  if (!raw) return []
  try {
    const p = JSON.parse(raw)
    if (Array.isArray(p)) {
      return p.filter((x) => x && typeof x === 'object') as DetailBackState[]
    }
    if (p && typeof p === 'object') return [p as DetailBackState]
    return []
  } catch {
    return []
  }
}

/** 记录返回来源列表状态（同 id 压栈；必须在 window.open 之前调用，新标签才能继承 sessionStorage） */
export function recordBackState(id: string, state: DetailBackState): void {
  if (!id || !state?.fromPath) return
  try {
    const key = `${BACK_KEY_PREFIX}${id}`
    const stack = parseBackStack(sessionStorage.getItem(key))
    // 去重：来源路径与栈顶相同则替换（防同一页重复触发堆积相同帧）
    const top = stack[stack.length - 1]
    if (top && top.fromFullPath === state.fromFullPath && top.fromPath === state.fromPath) {
      stack[stack.length - 1] = state
    } else {
      stack.push(state)
    }
    // 深度上限 8，防极端长链撑爆存储
    sessionStorage.setItem(key, JSON.stringify(stack.slice(-8)))
  } catch {
    /* 隐私模式 / 存储不可用时静默降级（window.opener 判定仍可用） */
  }
}

/** 读取并消费返回来源列表状态（弹栈最近一层；弹空后删除） */
export function consumeBackState(id: string): DetailBackState | undefined {
  if (!id) return undefined
  try {
    const key = `${BACK_KEY_PREFIX}${id}`
    const stack = parseBackStack(sessionStorage.getItem(key))
    if (stack.length === 0) return undefined
    const state = stack.pop()
    if (stack.length > 0) sessionStorage.setItem(key, JSON.stringify(stack))
    else sessionStorage.removeItem(key)
    if (state && typeof state.fromPath === 'string' && typeof state.top === 'number') {
      return state
    }
    return undefined
  } catch {
    return undefined
  }
}

/** 在新浏览器标签打开漫画详情，并写入新标签标记（供 S11 返回语义判断） */
/** 记录来源列表状态（SPA 跳转前调用，sessionStorage 共享供返回恢复） */
export function recordBackStateForDetail(comic: ComicNavTarget): void {
  if (!comic?.id) return
  // Round17-Bug3：无条件记录来源路径（即使未滚动 scrollTop=0 也要写 fromPath），
  // 否则返回时 consumeBackState 未命中 → 走首页兜底而非回到来源页。
  // top/page 有则带上（恢复滚动/页码），无则回落 0。
  const listState = captureActiveListState()
  recordBackState(comic.id, {
    fromPath: window.location.pathname,
    // Round17.2：保留完整路径（含 query），返回时才能回到带书架 id 的页面
    fromFullPath: window.location.pathname + window.location.search,
    top: listState?.top ?? 0,
    page: listState?.page,
  })
}

/**
 * 打开漫画详情（Round16：统一 SPA 同标签跳转，根治 PWA 逃逸）。
 * - 调用方（组件内 useRouter）拿到 href 后 router.push 到该路径；
 * - 跳转前调用 recordBackStateForDetail 记录来源（返回恢复滚动/页码）；
 * - 不再使用 window.location.href / window.open（iOS PWA 下整页导航会逃逸到 Safari 标签页）。
 */
export function buildDetailRoute(comic: ComicNavTarget): { path: string; query: Record<string, string> } | null {
  if (!comic?.id) return null
  if (comic.source === 'offline') {
    return { path: '/offline/detail', query: { id: comic.id, ...(comic.resume ? { resume: '1' } : {}) } }
  }
  const query: Record<string, string> = { id: comic.id }
  if (comic.token) query.token = comic.token
  if (comic.resume) query.resume = '1'
  return { path: '/online/detail', query }
}

/**
 * 在新浏览器标签打开漫画详情（强制新标签：中键/Ctrl 点击、对比页入口）。
 * Round21：统一走 openContentTab（记录来源 + 入口路由；open 被拦截时降级同标签）。
 */
export function openComicDetailInNewTab(comic: ComicNavTarget): void {
  if (!comic?.id) return
  const href = buildDetailHref(comic)
  if (href) openContentTab({ href, id: comic.id }, true)
}
