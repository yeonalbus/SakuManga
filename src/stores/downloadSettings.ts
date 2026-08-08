/**
 * 下载设置 Store
 *
 * 统一管理「设置中心 → 下载」的配置：
 * - 后端 DownloadSetting（单例 ID=1）为唯一事实来源，前端通过 API 双向同步
 * - localStorage 仅作为离线兜底缓存，跨页面共享
 * - 下载任务创建与下载引擎读取的是后端设置，因此前端修改必须同步到后端才会生效
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'
import { http } from '@/utils/request'
import { API_BASE, TOKEN_KEY } from '@/config/api'
import { onPageHide } from '@/utils/pageHideFlush'

/** 速度限制时间间隔单位 */
export type SpeedLimitInterval = '1s' | '2s' | '5s'

/** 默认下载配置（四选一，替代旧的「默认选中下载原图」布尔值） */
export type DownloadDefaultScheme =
  | 'gallery' // 画廊下载（逐图，原图）
  | 'galleryOriginal' // 画廊原图（逐图，原图）
  | 'archiveResample' // 归档压缩（H@H 压缩包）
  | 'archiveOriginal' // 归档原图（H@H 原图包）

/** 默认下载配置可选列表（设置页下拉用） */
export const DEFAULT_DOWNLOAD_SCHEME_OPTIONS: { value: DownloadDefaultScheme; label: string }[] = [
  { value: 'gallery', label: '画廊下载' },
  { value: 'galleryOriginal', label: '画廊原图' },
  { value: 'archiveResample', label: '归档压缩' },
  { value: 'archiveOriginal', label: '归档原图' },
]

/** 下载设置项集合（与后端 models.DownloadSetting 对齐） */
export interface DownloadSettings {
  // ── 下载路径 ──
  archivePath: string // 压缩包路径
  extractPath: string // 解压后的文件夹存储路径
  singleImageSavePath: string // 单张图片保存路径

  // ── 下载行为 ──
  defaultDownloadScheme: DownloadDefaultScheme // 默认下载配置（四选一）
  concurrentImageDownloads: number // 同时下载图片数量
  speedLimitImages: number // 速度限制（图片）
  speedLimitInterval: SpeedLimitInterval // 速度限制（间隔）
  downloadAllGalleriesSamePriority: boolean // 同一优先级下同时下载所有画廊

  // ── 归档设置 ──
  archiveThreads: number // 归档下载线程数
  controlArchiveConcurrency: boolean // 控制归档下载并发数
  maxArchiveConcurrency: number // 最大归档并发数（1-10，且 ≤ archiveThreads；默认 1）
  deleteZipAfterArchiveDownload: boolean // 归档下载完成后删除原压缩包
  autoReduceThreadsOnEOF: boolean // 归档下载遇 EOF（连接中断）自动降低线程数规避
  autoUnlockArchiveOnLock: boolean // 归档任务遇锁(error_lock)时自动消耗 GP 解锁重试（默认关闭）

  // ── 下载任务 ──
  autoResumeTasks: boolean // 自动恢复下载任务

  // ── 自动更新画廊 ──
  autoUpdateGallery: boolean // 是否自动更新画廊
  autoUpdateScheme: 'gallery' | 'archive' // 更新下载方案
  autoUpdateFallbackToGallery: boolean // 无 H@H 时自动降级为画廊下载
  autoUpdateDeleteOriginal: boolean // 下载新版本后是否删除旧版本文件夹
}

const STORAGE_KEY = 'saku_download_settings'

/** 默认值（与后端 defaultDownloadSetting 保持一致） */
const defaultSettings: DownloadSettings = {
  // 默认保存到程序目录下（后端启动时会切换到 exe 所在目录），首次使用建议修改为你自己的下载目录
  archivePath: 'downloads\\Archive',
  extractPath: 'downloads\\Gallery',
  singleImageSavePath: 'downloads\\Gallery',

  defaultDownloadScheme: 'archiveOriginal',
  concurrentImageDownloads: 10,
  speedLimitImages: 99,
  speedLimitInterval: '1s',
  downloadAllGalleriesSamePriority: true,

  archiveThreads: 10,
  controlArchiveConcurrency: true,
  maxArchiveConcurrency: 1,
  deleteZipAfterArchiveDownload: true,
  autoReduceThreadsOnEOF: true,
  autoUnlockArchiveOnLock: false,

  autoResumeTasks: true,

  autoUpdateGallery: false,
  autoUpdateScheme: 'archive',
  autoUpdateFallbackToGallery: true,
  autoUpdateDeleteOriginal: true,
}

/** 仅提取前端接口所需字段，避免后端/localStorage 多余字段（id/updatedAt/旧分组键）污染 */
const SETTING_KEYS: (keyof DownloadSettings)[] = [
  'archivePath',
  'extractPath',
  'singleImageSavePath',
  'defaultDownloadScheme',
  'concurrentImageDownloads',
  'speedLimitImages',
  'speedLimitInterval',
  'downloadAllGalleriesSamePriority',
  'archiveThreads',
  'controlArchiveConcurrency',
  'maxArchiveConcurrency',
  'deleteZipAfterArchiveDownload',
  'autoReduceThreadsOnEOF',
  'autoUnlockArchiveOnLock',
  'autoResumeTasks',
  'autoUpdateGallery',
  'autoUpdateScheme',
  'autoUpdateFallbackToGallery',
  'autoUpdateDeleteOriginal',
]

/** 从后端或 localStorage 数据中提取前端字段 */
function pickSetting(data: Partial<DownloadSettings>): Partial<DownloadSettings> {
  const picked: Record<string, unknown> = {}
  for (const key of SETTING_KEYS) {
    if (data[key] !== undefined) {
      picked[key] = data[key]
    }
  }
  return picked as Partial<DownloadSettings>
}

/** 响应式下载设置（自动持久化到 localStorage + 同步后端） */
const storedLegacy = loadStorage<Partial<DownloadSettings> & { defaultDownloadOriginal?: boolean }>(
  STORAGE_KEY,
  {},
)

// 旧版 localStorage 迁移：defaultDownloadOriginal(布尔) → defaultDownloadScheme(四选一)
// （旧版 true=归档原图，false=归档压缩，保持用户既有偏好）
const storedInit: Partial<DownloadSettings> = pickSetting(storedLegacy)
if (
  storedInit.defaultDownloadScheme === undefined &&
  typeof storedLegacy.defaultDownloadOriginal === 'boolean'
) {
  storedInit.defaultDownloadScheme = storedLegacy.defaultDownloadOriginal
    ? 'archiveOriginal'
    : 'archiveResample'
}

export const downloadSettings = reactive<DownloadSettings>({
  ...defaultSettings,
  ...storedInit,
})

/** 同步后端期间置位，避免 fetch 覆盖触发 watch → save 的循环 */
let syncing = false

/** 保存后端防抖计时器：避免滑块等高频修改触发大量 POST */
let saveTimer: ReturnType<typeof setTimeout> | null = null

/** 是否存在尚未同步到后端的本地改动（页面隐藏时据此 keepalive 兜底 flush，避免防抖窗口内关闭丢失） */
let pendingSave = false

watch(
  downloadSettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
    if (!syncing) {
      pendingSave = true
      scheduleSaveDownloadSettings()
    }
  },
  { deep: true },
)

/** 从后端拉取下载设置（后端为唯一事实来源，覆盖本地值） */
export async function fetchDownloadSettings(): Promise<void> {
  syncing = true
  try {
    const data = await http<Partial<DownloadSettings> & { defaultDownloadOriginal?: boolean }>(
      '/downloads/settings',
    )
    // 兼容旧版后端仍返回 defaultDownloadOriginal 布尔值的情况
    if (
      data.defaultDownloadScheme === undefined &&
      typeof data.defaultDownloadOriginal === 'boolean'
    ) {
      data.defaultDownloadScheme = data.defaultDownloadOriginal
        ? 'archiveOriginal'
        : 'archiveResample'
    }
    Object.assign(downloadSettings, pickSetting(data))
  } catch (err) {
    console.warn('[downloadSettings] 拉取后端设置失败，沿用本地缓存:', err)
  } finally {
    syncing = false
  }
}

/** 防抖保存：将当前设置写入后端（由 watch 触发） */
function scheduleSaveDownloadSettings(): void {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => {
    saveTimer = null
    http<unknown>('/downloads/settings', {
      method: 'POST',
      body: JSON.stringify(downloadSettings),
    })
      .then(() => {
        pendingSave = false
      })
      .catch((err) => {
        console.warn('[downloadSettings] 保存后端设置失败:', err)
      })
  }, 300)
}

/** 立即保存当前设置到后端（返回是否成功） */
export async function saveDownloadSettingsNow(): Promise<boolean> {
  try {
    await http<unknown>('/downloads/settings', {
      method: 'POST',
      body: JSON.stringify(downloadSettings),
    })
    return true
  } catch (err) {
    console.warn('[downloadSettings] 保存后端设置失败:', err)
    return false
  }
}

/** 恢复默认下载设置 */
export function resetDownloadSettings(): void {
  Object.assign(downloadSettings, defaultSettings)
}

/** 仅重置三个下载路径为默认值 */
export function resetDownloadPaths(): void {
  downloadSettings.archivePath = defaultSettings.archivePath
  downloadSettings.extractPath = defaultSettings.extractPath
  downloadSettings.singleImageSavePath = defaultSettings.singleImageSavePath
}

/** 判断三个下载路径是否仍为默认值（用于首次使用引导提示） */
export function isUsingDefaultDownloadPaths(): boolean {
  return (
    downloadSettings.archivePath === defaultSettings.archivePath &&
    downloadSettings.extractPath === defaultSettings.extractPath &&
    downloadSettings.singleImageSavePath === defaultSettings.singleImageSavePath
  )
}

// 应用启动即拉取一次后端设置，保证任意页面读取到最新值
fetchDownloadSettings()

// 页面隐藏（关闭/刷新/切后台）时兜底 flush：若防抖窗口内存在未落盘的改动，
// 立即用 keepalive fetch 同步到后端（带鉴权头），避免「改完立即关闭页面」导致改动丢失。
// 回调幂等：pendingSave 置位才上报，pagehide 与 beforeunload 双触发只上报一次。
onPageHide(() => {
  if (!pendingSave) return
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  pendingSave = false
  const token = localStorage.getItem(TOKEN_KEY)
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = `Bearer ${token}`
  try {
    void fetch(`${API_BASE}/downloads/settings`, {
      method: 'POST',
      headers,
      body: JSON.stringify(downloadSettings),
      keepalive: true,
    }).catch(() => {
      /* 页面卸载中失败可忽略（尽力而为） */
    })
  } catch {
    /* 兜底失败静默（页面即将卸载） */
  }
})
