// src/utils/errorReporter.ts
// 前端全局错误上报（问题8）：本地 localStorage 环形缓冲（始终可用）+ 后端文件落盘（尽力而为）。
// 浏览器无法直接写文件，因此通过新增的 POST /api/v1/client/log 端点让后端追加写盘，
// 用于诊断「搜索栏输入特定内容时页面消失」等难以本地复现的前端崩溃。
import { API_BASE } from '@/config/api'
import { advancedSettings } from '@/stores/advancedSettings'
import { safeSetItem } from '@/utils/storage'

export interface AppErrorEntry {
  ts: string
  level: 'error' | 'warn' | 'info'
  message: string
  stack?: string
  url?: string
  info?: string
}

const RING_KEY = 'app_error_log'
const RING_MAX = 50

// 从 localStorage 读取环形缓冲（外部数据不可信，任何异常都退回空数组）
const readRing = (): AppErrorEntry[] => {
  try {
    const raw = localStorage.getItem(RING_KEY)
    if (!raw) return []
    const arr = JSON.parse(raw)
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
}

/**
 * 上报一条错误/告警：
 *  1. 写入 localStorage 环形缓冲（最多 RING_MAX 条，永不出错）；
 *  2. 尽力而为地 POST 到后端落盘。
 *
 * 注意：这里必须使用原生 fetch 而非 utils/request 的 http()——
 * 后者在 401 时会清 token 并抛错，可能反过来打断上报链路本身。
 */
export function reportError(
  level: AppErrorEntry['level'],
  message: unknown,
  stack?: string,
  info?: string,
): void {
  // 高级设置「开启日志」关闭时，不再本地记录与后端上报（1.0 收敛接线）
  if (!advancedSettings.enableLogs) return

  const msg = typeof message === 'string' ? message : String(message ?? 'unknown error')
  const entry: AppErrorEntry = {
    ts: new Date().toISOString(),
    level,
    message: msg,
    stack,
    url: typeof window !== 'undefined' ? window.location.href : undefined,
    info,
  }

  // 1. 本地环形缓冲（隐私模式等场景下 localStorage 不可用时静默降级；
  //    配额满时 safeSetItem 自动回收低价值缓存，保证诊断日志仍能落盘）
  try {
    const ring = readRing()
    ring.push(entry)
    safeSetItem(RING_KEY, JSON.stringify(ring.slice(-RING_MAX)))
  } catch {
    /* ignore */
  }

  // 2. 后端文件落盘（尽力而为，失败不产生二次错误）
  try {
    void fetch(`${API_BASE}/client/log`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(entry),
    }).catch(() => {
      /* 上报失败静默忽略 */
    })
  } catch {
    /* ignore */
  }
}

// 导出最近 N 条错误（供设置页/调试面板展示）
export function getRecentErrors(limit = 20): AppErrorEntry[] {
  return readRing().slice(-limit)
}
