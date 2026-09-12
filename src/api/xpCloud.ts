// src/api/xpCloud.ts
import { http } from '@/utils/request'
import type { XpCloudGroup, XpCloudMeta, XpCloudResult, XpCloudView } from '@/types/comic'

/**
 * XP 词云查询（Round32 阶段一）
 *
 * 后端：GET /api/v1/offline/xp-cloud
 * - 阅读侧信号按当前登录用户隔离（阅读次数 / 离线历史 / 个人评分）
 * - 库藏侧为全库共享；词条名走 E 站翻译词典，词典缺失时回退 key
 */
export const fetchXpCloudApi = async (params?: {
  group?: XpCloudGroup
  view?: XpCloudView
  limit?: number
}): Promise<XpCloudResult> => {
  const query = new URLSearchParams()
  query.append('group', params?.group ?? 'core')
  query.append('view', params?.view ?? 'library')
  if (params?.limit && params.limit > 0) query.append('limit', String(params.limit))
  return await http<XpCloudResult>(`/offline/xp-cloud?${query.toString()}`)
}

/**
 * 全量重算 XP 统计表（仅管理员）
 *
 * 用于公式调整、数据异常或增量链路漏接后的手动兜底。
 */
export const rebuildXpCloudApi = async (): Promise<{ message: string; meta?: XpCloudMeta }> => {
  return await http<{ message: string; meta?: XpCloudMeta }>('/offline/xp-cloud/rebuild', {
    method: 'POST',
  })
}
