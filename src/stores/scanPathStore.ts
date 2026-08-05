/**
 * 额外扫描路径 Store：与 Go 后端 /scan-paths 接口交互
 * 由原 appStore 拆分而来
 *
 * 扫描逻辑说明：
 * - 后端 /scan-paths/:id/scan 现在为「异步启动」，立即返回进度对象，扫描在 goroutine 中执行，
 *   不再阻塞 HTTP 请求（解决了原同步扫描导致的「前端 60s 超时/切页后看起来被截断」问题）。
 * - 前端通过 GET /scan-paths/scan-progress 轮询全部路径进度，进度映射保存在 scanProgress 中，
 *   切页再回来时组件 onMounted 会调用 refreshScanProgress() 恢复进度展示。
 */
import { ref } from 'vue'
import { API_BASE } from '@/config/api'

/** 额外扫描路径 */
export interface ExtraScanPath {
  id: string
  path: string
  includeSubfolders: boolean
  lastScanned?: number
  comicCount?: number
}

/** 扫描模式：full=全文件夹扫描，incremental=增量式更新 */
export type ScanMode = 'full' | 'incremental'

/** 扫描进度（对应后端 ScanProgress） */
export interface ScanProgress {
  pathId: string
  mode: ScanMode
  phase: 'counting' | 'scanning' | 'done'
  total: number
  processed: number
  found: number
  skipped: number
  currentTitle: string
  error?: string
  done: boolean
  startedAt: number
  finishedAt?: number
  comicCount?: number
}

/** 扫描路径列表 */
export const scanPaths = ref<ExtraScanPath[]>([])

/** 各路径扫描进度映射 pathId -> progress */
export const scanProgress = ref<Record<string, ScanProgress>>({})

/** 是否存在活跃（未完成）的扫描任务 */
export const hasActiveScan = () => Object.values(scanProgress.value).some((p) => !p.done)

/** 获取某路径的扫描进度（可能为 undefined） */
export const getScanProgress = (id: string) => scanProgress.value[id]

/** 从后端拉取全部扫描进度并写入本地映射（同时同步已完成任务的统计） */
export const refreshScanProgress = async () => {
  try {
    const res = await fetch(`${API_BASE}/scan-paths/scan-progress`)
    if (!res.ok) return
    const list = (await res.json()) as ScanProgress[]
    const map: Record<string, ScanProgress> = {}
    for (const p of list) {
      if (!p.pathId) continue
      map[p.pathId] = p
      // 扫描完成 → 同步路径记录的统计（供列表直接展示）
      if (p.done) {
        const item = scanPaths.value.find((s) => s.id === p.pathId)
        if (item && !p.error) {
          item.lastScanned = p.finishedAt || Date.now()
          item.comicCount = p.comicCount ?? p.found
        }
      }
    }
    scanProgress.value = map
  } catch (err) {
    console.error('拉取扫描进度失败:', err)
  }
}

// --------------------------------------------------
// 内部轮询：800ms 拉取一次进度，无活跃任务时自动停止
// --------------------------------------------------
let pollTimer: ReturnType<typeof setInterval> | null = null

const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

/** 启动内部轮询（幂等）；无活跃任务时自动停止 */
export const ensurePolling = () => {
  if (pollTimer) return
  pollTimer = setInterval(async () => {
    await refreshScanProgress()
    if (!hasActiveScan()) stopPolling()
  }, 800)
}

// --------------------------------------------------
// 扫描动作
// --------------------------------------------------

/**
 * 异步启动一次扫描（full / incremental）。
 * 成功返回 true；失败抛出带信息的 Error（调用方可 toast 展示）。
 */
export const startScanPath = async (id: string, mode: ScanMode = 'full'): Promise<boolean> => {
  try {
    const res = await fetch(`${API_BASE}/scan-paths/${id}/scan`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode }),
    })
    if (res.ok) {
      const progress = (await res.json()) as ScanProgress
      scanProgress.value = { ...scanProgress.value, [progress.pathId]: progress }
      ensurePolling()
      return true
    }
    const data = await res.json().catch(() => ({}))
    throw new Error(data.error || `HTTP ${res.status}`)
  } catch (err) {
    console.error('触发扫描失败:', err)
    throw err
  }
}

/** 清除某路径的扫描进度（供扫描完成提示后收起进度条） */
export const clearScanProgress = (id: string) => {
  const next = { ...scanProgress.value }
  delete next[id]
  scanProgress.value = next
  if (!hasActiveScan()) stopPolling()
}

// --------------------------------------------------
// 路径 CRUD（保持原逻辑）
// --------------------------------------------------

/** 从 Go 后端拉取所有扫描路径 */
export const fetchScanPaths = async () => {
  try {
    const res = await fetch(`${API_BASE}/scan-paths`)
    if (res.ok) {
      scanPaths.value = await res.json()
    }
  } catch (err) {
    console.error('连接 Go 后端失败，请检查 backend 服务是否启动:', err)
  }
}

/** 添加新路径，成功返回 true */
export const addScanPath = async (path: string): Promise<boolean> => {
  try {
    const res = await fetch(`${API_BASE}/scan-paths`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: path.trim() }),
    })
    if (res.ok) {
      const newPath = await res.json()
      scanPaths.value.push(newPath)
      return true
    }
  } catch (err) {
    console.error('添加路径失败:', err)
  }
  return false
}

/** 切换是否包含子文件夹 */
export const toggleSubfolders = async (id: string, includeSubfolders: boolean) => {
  const item = scanPaths.value.find((p) => p.id === id)
  if (item) {
    item.includeSubfolders = includeSubfolders
    try {
      await fetch(`${API_BASE}/scan-paths/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ includeSubfolders }),
      })
    } catch (err) {
      console.error('更新子文件夹状态失败:', err)
    }
  }
}

/** 移除指定路径 */
export const removeScanPath = async (id: string) => {
  scanPaths.value = scanPaths.value.filter((p) => p.id !== id)
  clearScanProgress(id)
  try {
    await fetch(`${API_BASE}/scan-paths/${id}`, {
      method: 'DELETE',
    })
  } catch (err) {
    console.error('删除路径失败:', err)
  }
}
