// src/composables/useVersionCheck.ts
//
// 版本更新检测（Round29：设置 → 关于软件 → 版本红点提醒）
//
// 后端 GET /api/v1/system/check-update?current=<版本> 比较 GitHub 最新 release；
// 前端只需在「打开设置页」时触发一次，结果缓存 6 小时（localStorage），
// 避免高频访问 GitHub。失败结果不落缓存（下次进设置页自动重试）。
import { ref } from 'vue'
import { http } from '@/utils/request'
import { loadStorage, saveStorage } from '@/utils/storage'
// 版本唯一来源 package.json（与 AboutSettings 一致）
import { version } from '../../package.json'

/** 后端 /system/check-update 返回结构 */
export interface ReleaseCheckInfo {
  ok: boolean
  current?: string
  latest?: string
  tag?: string
  url?: string
  hasUpdate: boolean
  checkedAt?: number
  error?: string
}

interface CachedCheck {
  savedAt: number
  data: ReleaseCheckInfo
}

const CACHE_KEY = 'saku_update_check_v1'
const CACHE_TTL_MS = 6 * 60 * 60 * 1000 // 6 小时

/** 当前是否正在检测（供 UI 按钮 loading 态） */
const checking = ref(false)
/** 最近一次检测结果（成功失败均保留，组件共享模块级状态） */
const info = ref<ReleaseCheckInfo | null>(null)
/** 并发去重：同一时刻只发一个请求 */
let inflight: Promise<ReleaseCheckInfo> | null = null

const readCache = (): CachedCheck | null => loadStorage<CachedCheck | null>(CACHE_KEY, null)

const isFresh = (c: CachedCheck): boolean =>
  // 缓存新鲜 且 比较基准与当前版本一致（升级后旧缓存失效，避免误报）
  c.data?.current === version && Date.now() - c.savedAt < CACHE_TTL_MS

async function doRequest(): Promise<ReleaseCheckInfo> {
  const data = await http<ReleaseCheckInfo>('/system/check-update', {
    params: { current: version },
  })
  // 仅成功结果落缓存；失败不缓存，进设置页自然重试
  if (data && data.ok) {
    saveStorage(CACHE_KEY, { savedAt: Date.now(), data } as CachedCheck)
  }
  return data
}

/**
 * 检查 GitHub 是否有新版本。force=true 忽略 6h 缓存强制刷新（手动「检查更新」按钮）。
 * 网络异常不抛错：返回 ok=false 的结果对象，由调用方决定提示文案。
 */
export function checkVersion(force = false): Promise<ReleaseCheckInfo> {
  const cached = readCache()
  if (!force && cached && isFresh(cached) && cached.data) {
    info.value = cached.data
    return Promise.resolve(cached.data)
  }
  if (inflight) return inflight

  checking.value = true
  inflight = doRequest()
    .then((data) => {
      info.value = data
      return data
    })
    .catch((err) => {
      const fail: ReleaseCheckInfo = {
        ok: false,
        hasUpdate: false,
        error: err instanceof Error ? err.message : '检测失败',
      }
      info.value = fail
      return fail
    })
    .finally(() => {
      checking.value = false
      inflight = null
    })
  return inflight
}

/** 供组件使用：响应式状态 + 触发函数 */
export function useVersionCheck() {
  return { checking, info, checkVersion }
}
