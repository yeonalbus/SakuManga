// src/api/download.ts
// 下载任务相关 API：快捷单卡入队 + 批量加入下载队列 + 优先级修改
// 统一使用「下载设置」里的默认方案（零弹窗），与详情页 handleStartDownload 保持一致的取值逻辑。
import { http } from '@/utils/request'
import { downloadSettings } from '@/stores/downloadSettings'
import type { OnlineComic } from '@/types/comic'

/** 后端 BatchCreateResult（POST /downloads/batch 返回） */
export interface BatchDownloadResult {
  created: number
  skipped: number // 已存在进行中任务（gid 去重）
  failed: number
  errors: string[]
  taskIds: string[]
}

/** 单个待下载画廊（gid + token 即可入队） */
export interface DownloadTarget {
  gid: string
  token: string
  title?: string
  coverUrl?: string
}

/** 按「下载设置 → 默认下载配置」计算下载模式与归档类型 */
export function resolveDefaultDownloadScheme(): {
  mode: 'gallery' | 'archive'
  archiveType: '' | 'original' | 'resample'
} {
  switch (downloadSettings.defaultDownloadScheme) {
    case 'gallery':
    case 'galleryOriginal':
      // 画廊（逐图）模式始终下载原图，archiveType 不参与
      return { mode: 'gallery', archiveType: '' }
    case 'archiveResample':
      return { mode: 'archive', archiveType: 'resample' }
    case 'archiveOriginal':
    default:
      return { mode: 'archive', archiveType: 'original' }
  }
}

/**
 * 单个画廊入队（卡片 hover 快捷下载用，零弹窗默认方案）
 * @throws 后端去重/鉴权错误（调用方 catch 后 toast 展示）
 */
export async function createDownloadTask(
  comic: Pick<OnlineComic, 'id' | 'token'> & Partial<Pick<OnlineComic, 'title' | 'coverUrl'>>,
): Promise<void> {
  const { mode, archiveType } = resolveDefaultDownloadScheme()
  await http('/downloads', {
    method: 'POST',
    body: JSON.stringify({
      gid: comic.id,
      token: comic.token,
      title: comic.title ?? '',
      coverUrl: comic.coverUrl ?? '',
      mode,
      archiveType,
    }),
  })
}

/**
 * 批量创建下载任务（服务端逐条去重，返回聚合统计）
 */
export async function batchCreateDownloads(
  targets: DownloadTarget[],
): Promise<BatchDownloadResult> {
  const { mode, archiveType } = resolveDefaultDownloadScheme()
  return await http<BatchDownloadResult>('/downloads/batch', {
    method: 'POST',
    body: JSON.stringify({
      tasks: targets.map((t) => ({
        gid: t.gid,
        token: t.token,
        title: t.title ?? '',
        coverUrl: t.coverUrl ?? '',
      })),
      mode,
      archiveType,
    }),
  })
}

// ─────────────────────────────────────────────────────────────
// 任务契约与优先级
// ─────────────────────────────────────────────────────────────

/** 后端 DownloadTask 契约（GET /downloads 列表项；POST /downloads/:id/priority 返回更新后的任务） */
export interface DownloadTask {
  id: string
  gid: string
  token: string
  title: string
  coverUrl?: string
  mode: 'gallery' | 'archive'
  archiveType?: 'original' | 'resample'
  status: 'queued' | 'downloading' | 'paused' | 'completed' | 'error' | 'error_lock' | 'cancelled'
  priority: number
  group?: string
  totalFiles: number
  doneFiles: number
  totalBytes: number
  doneBytes: number
  speed: number
  archivePath?: string
  extractPath?: string
  error?: string
  updateForComicId?: string
  // 任务发起者用户名（仅管理员可见；后端由 ListTasks 批量填充，非持久化字段）
  username?: string
  createdAt: string
  updatedAt: string
}

/**
 * 修改任务优先级 POST /downloads/:id/priority
 * 提升优先级会触发抢占式调度：正在运行的低优先级任务被置回排队（进度保留），
 * 高优先级任务优先竞争全局线程额度（计划书 5.5）。
 * @throws 后端错误（completed / cancelled 任务不允许修改）
 */
export async function setTaskPriority(id: string, priority: number): Promise<DownloadTask> {
  return await http<DownloadTask>(`/downloads/${id}/priority`, {
    method: 'POST',
    body: JSON.stringify({ priority }),
  })
}
