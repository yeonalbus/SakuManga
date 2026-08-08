// src/stores/downloadTasksStore.ts
// 跨页面共享「下载中」任务状态：
// 轮询 GET /downloads?status=active，按 gid 提供 isGidDownloading 判定，
// 供 ItemCard 展示「下载中」角标并禁用快捷下载，避免用户对同一画廊反复点击。
import { ref } from 'vue'
import { http } from '@/utils/request'
import { fetchOfflineComics } from '@/stores/comicStore'
import { useUserStore } from '@/stores/userStore'

/** 活动下载任务（仅需要 gid 等少量字段用于角标判定） */
interface ActiveDownloadTask {
  id: string
  gid: string
  status?: string
  title?: string
}

/** 活动任务 gid 集合（queued / downloading / paused / error / error_lock） */
const activeGids = ref<Set<string>>(new Set())

const POLL_INTERVAL = 4000 // 比「下载」页 2s 放慢，避免在线列表频繁请求

let pollTimer: ReturnType<typeof setInterval> | null = null
let subscriberCount = 0

/** 上一次轮询的活动任务 gid 集合（用于检测任务完成/取消，触发离线书库缓存刷新） */
let lastActiveGids = new Set<string>()
/** 离线书库刷新防重入：避免多个任务同时完成导致并发重复请求 */
let refreshPending = false

/** 任务完成/取消后刷新离线书库缓存（需求2：下载新版本后删除旧版本，前端需及时反映） */
function refreshOfflineComicsAfterTask(): void {
  if (refreshPending) return
  refreshPending = true
  fetchOfflineComics().finally(() => {
    refreshPending = false
  })
}

/** 拉取一次活动任务列表（失败静默保留上次状态，避免抖动报错）。
 *  分页拉取全部活动任务（每页 500），避免大量任务时漏检导致角标/去重失效。 */
export async function fetchActiveDownloads(): Promise<void> {
  // 无下载许可用户不轮询（中心制：无许可用户隐藏下载能力，避免无意义请求）
  const userStore = useUserStore()
  if (!userStore.isAdmin && !userStore.user?.allowDownload) return
  try {
    const tasks: ActiveDownloadTask[] = []
    let page = 1
    const PAGE_SIZE = 500
    for (;;) {
      const res = await http<{ tasks?: ActiveDownloadTask[] }>('/downloads', {
        params: { status: 'active', page, size: PAGE_SIZE },
      })
      const batch = res.tasks || []
      tasks.push(...batch)
      if (batch.length < PAGE_SIZE) break
      page += 1
    }
    const current = new Set(tasks.map((t) => String(t.gid)).filter(Boolean))
    // 检测任务完成/取消：gid 从 active 集合消失（completed/cancelled 不在 active 内）→ 刷新离线书库缓存
    if (lastActiveGids.size > 0) {
      for (const gid of lastActiveGids) {
        if (!current.has(gid)) {
          refreshOfflineComicsAfterTask()
          break
        }
      }
    }
    lastActiveGids = current
    activeGids.value = current
  } catch {
    // 静默：保持上次状态，等待下次轮询
  }
}

/** 指定 gid 是否已有进行中下载任务 */
export function isGidDownloading(gid: string | number | null | undefined): boolean {
  return !!gid && activeGids.value.has(String(gid))
}

/** 本地把某个 gid 标记为下载中（创建任务成功后立即生效，无需等下次轮询） */
export function markGidActive(gid: string | number | null | undefined): void {
  if (!gid) return
  const set = new Set(activeGids.value)
  set.add(String(gid))
  activeGids.value = set
}

/**
 * 订阅活动任务轮询（引用计数：任一在线卡片挂载时启动，全部卸载后停止）
 * @returns 取消订阅函数
 */
export function subscribeActiveDownloads(): () => void {
  subscriberCount += 1
  if (!pollTimer) {
    fetchActiveDownloads()
    pollTimer = setInterval(fetchActiveDownloads, POLL_INTERVAL)
  }
  return () => {
    subscriberCount = Math.max(0, subscriberCount - 1)
    if (subscriberCount === 0 && pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }
}
