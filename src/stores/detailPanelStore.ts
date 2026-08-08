import { reactive } from 'vue'

/**
 * 内嵌详情面板跨页面状态（按 route.path 各自记录）
 *
 * 需求：点击卡片 → 左右分栏右侧打开详情面板；切换列表页时面板收起，
 * 返回原页时恢复（滚动位点由 App.vue 的 keep-alive / 原生 DOM 天然保留）。
 *
 * 用模块级 reactive 而非 Pinia，保证路由切换、组件重建后仍能恢复
 * （与 historyStore / bookshelfStore 同款单例模式）。
 */

export interface DetailPanelState {
  gid: string
  token: string
  open: boolean
}

const panelByPath = reactive<Record<string, DetailPanelState>>({})

/** 读取某路径的面板状态（不存在时返回 undefined） */
export function getDetailPanelState(path: string): DetailPanelState | undefined {
  return panelByPath[path]
}

/** 打开某路径的面板（记录 gid/token） */
export function openDetailPanel(path: string, gid: string, token: string) {
  panelByPath[path] = { gid, token, open: true }
}

/** 关闭某路径的面板（保留 gid/token，方便再次打开时快速定位） */
export function closeDetailPanel(path: string) {
  const state = panelByPath[path]
  if (state) state.open = false
}

/**
 * 兼容迁移：将旧 key（route.path）状态迁移到新 key（route.fullPath），
 * 返回 fullPath 对应的状态（可能为 undefined）。fullPath 与 legacyPath 相同时为 no-op。
 */
export function migrateDetailPanel(
  fullPath: string,
  legacyPath: string,
): DetailPanelState | undefined {
  if (fullPath === legacyPath) return panelByPath[fullPath]
  const legacy = panelByPath[legacyPath]
  if (legacy && !panelByPath[fullPath]) {
    panelByPath[fullPath] = legacy
  }
  delete panelByPath[legacyPath]
  return panelByPath[fullPath]
}

/** 清除某路径的面板状态 */
export function clearDetailPanel(path: string) {
  delete panelByPath[path]
}
