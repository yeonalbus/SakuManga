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
import { http } from '@/utils/request'

/** 额外扫描路径 */
export interface ExtraScanPath {
  id: string
  path: string
  includeSubfolders: boolean
  lastScanned?: number
  comicCount?: number
  // 问题3/4：可配置显示名（来源标签）+ 是否参与离线更新/维护查重
  name?: string
  enableOfflineUpdate?: boolean
  /** Round54：路径不存在（整条目录被删）——显示「建议进行恢复」提示 */
  pathMissing?: boolean
}

/** 扫描模式：full=全文件夹扫描，incremental=增量式更新 */
export type ScanMode = 'full' | 'incremental'

/** 扫描进度（对应后端 ScanProgress） */
export interface ScanProgress {
  pathId: string
  mode: ScanMode
  phase: 'counting' | 'scanning' | 'reconciling' | 'done'
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
  /** D2：递归图片数为 0 被跳过的目录数 */
  skippedEmpty?: number
  /** D4：不可读归档（rar/7z）被跳过的条数 */
  skippedUnsupported?: number
  /** D17：增量模式已存在跳过数 */
  skippedExisting?: number
  /** D17：同 gid 冲突跳过数 */
  skippedGid?: number
  /** D17：写库失败数 */
  failed?: number
  /** 跳过项摘要（前若干条，供告警展示） */
  warnings?: string[]
  /** Round54：扫描根路径不存在（整条路径判失效） */
  pathMissing?: boolean
  /** Round54：对账待检查记录数 / 已检查数 */
  reconcileTotal?: number
  reconcileProcessed?: number
  /** Round54：对账结果（失效 / 候选 / 阵亡 / 自愈） */
  missingCount?: number
  candidateCount?: number
  fallenCount?: number
  healedCount?: number
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
    const list = await http<ScanProgress[]>('/scan-paths/scan-progress')
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
    const progress = await http<ScanProgress>(`/scan-paths/${id}/scan`, {
      method: 'POST',
      body: JSON.stringify({ mode }),
    })
    scanProgress.value = { ...scanProgress.value, [progress.pathId]: progress }
    ensurePolling()
    return true
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
    scanPaths.value = await http<ExtraScanPath[]>('/scan-paths')
  } catch (err) {
    console.error('连接 Go 后端失败，请检查 backend 服务是否启动:', err)
  }
}

/** 添加新路径（可附带显示名），成功返回 true */
export const addScanPath = async (path: string, name = ''): Promise<boolean> => {
  try {
    const newPath = await http<ExtraScanPath>('/scan-paths', {
      method: 'POST',
      body: JSON.stringify({ path: path.trim(), name: name.trim() }),
    })
    scanPaths.value.push(newPath)
    return true
  } catch (err) {
    console.error('添加路径失败:', err)
    throw err
  }
}

/** 切换是否包含子文件夹 */
export const toggleSubfolders = async (id: string, includeSubfolders: boolean) => {
  const item = scanPaths.value.find((p) => p.id === id)
  if (item) {
    item.includeSubfolders = includeSubfolders
    try {
      await http(`/scan-paths/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ includeSubfolders }),
      })
    } catch (err) {
      console.error('更新子文件夹状态失败:', err)
    }
  }
}

/** 更新路径显示名（问题3：来源标签） */
export const updateScanPathName = async (id: string, name: string) => {
  const item = scanPaths.value.find((p) => p.id === id)
  const trimmed = name.trim()
  if (item) {
    item.name = trimmed
    try {
      await http(`/scan-paths/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ name: trimmed }),
      })
    } catch (err) {
      console.error('更新路径名称失败:', err)
    }
  }
}

/** 切换该路径是否参与离线更新/维护查重（问题4） */
export const updateEnableOfflineUpdate = async (id: string, enabled: boolean) => {
  const item = scanPaths.value.find((p) => p.id === id)
  if (item) {
    item.enableOfflineUpdate = enabled
    try {
      await http(`/scan-paths/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ enableOfflineUpdate: enabled }),
      })
    } catch (err) {
      console.error('更新离线维护开关失败:', err)
    }
  }
}

/** 查询删除该路径会连带清理的漫画数量（二次确认文案用） */
export const getScanPathImpact = async (id: string): Promise<number> => {
  const res = await http<{ comicCount: number }>(`/scan-paths/${id}/impact`)
  return res?.comicCount ?? 0
}

/** 移除指定路径（后端会连带清理该路径下已入库的漫画记录，磁盘文件保留）。
 *  成功返回被清理的漫画数量；失败抛出错误（保持本地列表与后端一致）。 */
export const removeScanPath = async (id: string): Promise<number> => {
  const res = await http<{ removedComics?: number }>(`/scan-paths/${id}`, {
    method: 'DELETE',
  })
  scanPaths.value = scanPaths.value.filter((p) => p.id !== id)
  clearScanProgress(id)
  return res?.removedComics ?? 0
}

// --------------------------------------------------
// Round54：恢复与阵亡
// --------------------------------------------------

/** 恢复功能状态（后端由未终结任务推导） */
export interface RecoveryStatus {
  phase: 'idle' | 'ready' | 'recovering'
  candidateCount: number
  activeTasks: number
  lockedPathIds: string[]
}

/** 恢复方案（档位）：H@H 原图 / H@H 压缩 / 画廊逐图 */
export type RecoveryScheme = 'archive-original' | 'archive-resample' | 'gallery'

/** 恢复启动结果 */
export interface RecoveryStartResult {
  created: number
  skipped: number
  failed: number
  errors?: string[]
}

/** 阵亡条目（全局名单） */
export interface FallenComic {
  id: number
  gid?: string
  title: string
  titleJpn?: string
  coverUrl?: string
  category?: string
  fileSize?: number
  pageCount?: number
  reason: 'removed' | 'unlock-failed' | 'no-ehentai' | 'path-removed'
  sourcePath?: string
  dismissed: boolean
  dismissedAt?: string
  createdAt?: string
}

/** 恢复状态（全局） */
export const recoveryStatus = ref<RecoveryStatus>({
  phase: 'idle',
  candidateCount: 0,
  activeTasks: 0,
  lockedPathIds: [],
})

/** 阵亡名单（缓存最近一页） */
export const fallenItems = ref<FallenComic[]>([])
export const fallenTotal = ref(0)

/** 拉取恢复状态 */
export const fetchRecoveryStatus = async () => {
  try {
    recoveryStatus.value = await http<RecoveryStatus>('/offline/recovery/status')
  } catch (err) {
    console.error('拉取恢复状态失败:', err)
  }
}

/** 启动恢复（档位由弹窗选择） */
export const startRecovery = async (scheme: RecoveryScheme): Promise<RecoveryStartResult> => {
  return await http<RecoveryStartResult>('/offline/recovery/start', {
    method: 'POST',
    body: JSON.stringify({ scheme }),
  })
}

/** 取消恢复（gids 为空 = 全部取消；候选保留可重新启动） */
export const cancelRecovery = async (gids?: string[]): Promise<{ cancelled: number }> => {
  return await http<{ cancelled: number }>('/offline/recovery/cancel', {
    method: 'POST',
    body: JSON.stringify(gids && gids.length > 0 ? { gids } : {}),
  })
}

/** 拉取阵亡名单（全局，按需刷新） */
export const fetchFallen = async (page = 1, size = 200) => {
  try {
    const res = await http<{ items: FallenComic[]; total: number }>(
      `/offline/fallen?page=${page}&size=${size}`,
    )
    fallenItems.value = res.items ?? []
    fallenTotal.value = res.total ?? 0
  } catch (err) {
    console.error('拉取阵亡名单失败:', err)
  }
}

/** 阵亡条目 x toggle（划线与撤销划线） */
export const dismissFallen = async (id: number, dismissed: boolean) => {
  await http('/offline/fallen/dismiss', {
    method: 'POST',
    body: JSON.stringify({ id, dismissed }),
  })
  const item = fallenItems.value.find((f) => f.id === id)
  if (item) {
    item.dismissed = dismissed
    item.dismissedAt = dismissed ? new Date().toISOString() : undefined
  }
  // dismissed 置底：本地重排（未 dismissed 按原顺序在前，dismissed 在最后）
  const active = fallenItems.value.filter((f) => !f.dismissed)
  const done = fallenItems.value.filter((f) => f.dismissed)
  fallenItems.value = [...active, ...done]
}
