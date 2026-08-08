/**
 * 阅读进度公共工具：localStorage 进度读写 + 起始页恢复决策
 *
 * 供详情页「立即阅读」与阅读器共用，保证恢复逻辑唯一实现。
 * - 进度 key 按账号隔离：`saku_comic_progress:<uid>`（未登录回退 anonymous）
 * - `resolveResumePage`：历史入口总是恢复；非历史入口遵循偏好开关；无记录返回 null（= 第 1 页）
 */
import { safeSetItem, MAX_PROGRESS_ENTRIES } from '@/utils/storage'
import { useUserStore } from '@/stores/userStore'
import { preferenceSettings } from '@/stores/preferenceSettings'
import { getHistoryProgress } from '@/stores/historyStore'
import type { ComicSource } from '@/types/comic'

/** 进度存储 key（按账号隔离） */
export const getProgressStorageKey = (uid: string): string => `saku_comic_progress:${uid}`

/** 读取指定账号的全量进度 Map { [source:id]: pageNumber } */
export const getProgressMap = (uid: string): Record<string, number> => {
  try {
    return JSON.parse(localStorage.getItem(getProgressStorageKey(uid)) || '{}')
  } catch {
    return {}
  }
}

/** 保存指定账号的作品阅读进度（在线/离线分开存储，避免 id 冲突）。
 *  容量上限 MAX_PROGRESS_ENTRIES 统一引用 storage.ts 的权威定义（300），
 *  与配额回收阈值保持一致，避免「写入保 500 / 回收裁 300」的漏网区间。 */
export const saveProgress = (uid: string, src: ComicSource, id: string, page: number): void => {
  if (!id) return
  const map = getProgressMap(uid)
  const key = `${src}:${id}`
  // 先删旧 key 再写入，保证「最近读的」排到对象末尾，容量裁剪时优先保留
  delete map[key]
  map[key] = page
  const keys = Object.keys(map)
  if (keys.length > MAX_PROGRESS_ENTRIES) {
    const extra = keys.length - MAX_PROGRESS_ENTRIES
    for (let i = 0; i < extra; i++) {
      delete map[keys[i]]
    }
  }
  safeSetItem(getProgressStorageKey(uid), JSON.stringify(map))
}

/** 读取指定作品的历史进度（无记录默认第 1 页） */
export const getSavedPage = (uid: string, src: ComicSource, id: string): number => {
  const map = getProgressMap(uid)
  return map[`${src}:${id}`] || 1
}

/**
 * 解析「立即阅读」起始页（1-based）。
 * @param source 作品来源
 * @param id     作品 id（离线 md5 / 在线 gid）
 * @param opts   决策入参：
 *  - fromHistory：是否来自历史页入口（总是恢复，无视偏好）
 *  - resumePreference：非历史入口时是否允许恢复（= preferenceSettings.resumeFromLastPage）
 * @returns 起始页码；无阅读记录或未启用恢复时返回 null（调用方按第 1 页处理）
 */
export const resolveResumePage = async (
  source: ComicSource,
  id: string,
  opts: { fromHistory?: boolean; resumePreference?: boolean } = {},
): Promise<number | null> => {
  if (!id) return null
  // 决策：历史入口总是恢复；否则偏好开关开启才恢复；否则第 1 页
  const allowResume = opts.fromHistory || opts.resumePreference
  if (!allowResume) return null

  // 本地进度（无记录返回 1）
  const userStore = useUserStore()
  const uid = String(userStore.user?.id ?? 'anonymous')
  const local = getSavedPage(uid, source, id)
  // 后端进度（按账号精确读取，无记录返回 null）
  const serverPage = await getHistoryProgress(source, id)

  const candidates = [local, serverPage ?? 0].filter((v) => typeof v === 'number' && v > 1)
  if (candidates.length === 0) return null
  return Math.max(...candidates)
}

/** 便捷封装：读取偏好开关（默认关） */
export const isResumeFromLastPageEnabled = (): boolean => !!preferenceSettings.resumeFromLastPage
