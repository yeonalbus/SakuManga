// src/api/download.ts
// 下载任务相关 API：快捷单卡入队 + 批量加入下载队列
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

/** 按「下载设置」默认方案计算下载模式与归档类型 */
export function resolveDefaultDownloadScheme(): {
  mode: 'gallery' | 'archive'
  archiveType: '' | 'original' | 'resample'
} {
  const mode = downloadSettings.autoUpdateScheme === 'gallery' ? 'gallery' : 'archive'
  const archiveType =
    mode === 'archive' ? (downloadSettings.defaultDownloadOriginal ? 'original' : 'resample') : ''
  return { mode, archiveType }
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
