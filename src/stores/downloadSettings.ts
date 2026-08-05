/**
 * 下载设置 Store
 *
 * 统一管理「设置中心 → 下载」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - DownloadSettings.vue 与后续下载/归档功能读写同一份数据
 * 由原 DownloadSettings 组件内 ref 抽取而来，参照 readerSettings 持久化模式
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 速度限制时间间隔单位 */
export type SpeedLimitInterval = '1s' | '2s' | '5s'

/** 下载设置项集合 */
export interface DownloadSettings {
  // ── 下载路径 ──
  archivePath: string // 压缩包路径
  extractPath: string // 解压后的文件夹存储路径
  singleImageSavePath: string // 单张图片保存路径

  // ── 下载行为 ──
  defaultDownloadOriginal: boolean // 默认选中下载原图
  defaultDownloadGroup: string // 默认分组（下载）
  concurrentImageDownloads: number // 同时下载图片数量
  speedLimitImages: number // 速度限制（图片）
  speedLimitInterval: SpeedLimitInterval // 速度限制（间隔）
  downloadAllGalleriesSamePriority: boolean // 同一优先级下同时下载所有画廊

  // ── 归档设置 ──
  defaultArchiveGroup: string // 默认分组（归档）
  archiveThreads: number // 归档下载线程数
  controlArchiveConcurrency: boolean // 控制归档下载并发数
  deleteZipAfterArchiveDownload: boolean // 归档下载完成后删除原压缩包

  // ── 下载任务 ──
  autoResumeTasks: boolean // 自动恢复下载任务

  // ── 自动更新画廊 ──
  autoUpdateGallery: boolean // 是否自动更新画廊
  autoUpdateScheme: 'gallery' | 'archive' // 更新下载方案
  autoUpdateFallbackToGallery: boolean // 无 H@H 时自动降级为画廊下载
  autoUpdateDeleteOriginal: boolean // 下载新版本后是否删除旧版本文件夹
}

const STORAGE_KEY = 'saku_download_settings'

/** 默认值（与下载功能现状保持一致） */
// ⚠️ 当前为测试路径（避免与用户既有仓库 Z:\Comics 冲突），真实路径待设置完善后调整
const defaultSettings: DownloadSettings = {
  archivePath: 'G:\\EhentaiWebProject\\Download_ZIP',
  extractPath: 'G:\\EhentaiWebProject\\Gallery',
  singleImageSavePath: 'G:\\EhentaiWebProject\\Gallery',

  defaultDownloadOriginal: true,
  defaultDownloadGroup: '默认',
  concurrentImageDownloads: 10,
  speedLimitImages: 99,
  speedLimitInterval: '1s',
  downloadAllGalleriesSamePriority: true,

  defaultArchiveGroup: '默认',
  archiveThreads: 10,
  controlArchiveConcurrency: true,
  deleteZipAfterArchiveDownload: true,

  autoResumeTasks: true,

  autoUpdateGallery: false,
  autoUpdateScheme: 'archive',
  autoUpdateFallbackToGallery: true,
  autoUpdateDeleteOriginal: true,
}

/** 响应式下载设置（自动持久化） */
export const downloadSettings = reactive<DownloadSettings>({
  ...defaultSettings,
  ...loadStorage<Partial<DownloadSettings>>(STORAGE_KEY, {}),
})

watch(
  downloadSettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

/** 恢复默认下载设置 */
export function resetDownloadSettings(): void {
  Object.assign(downloadSettings, defaultSettings)
}
