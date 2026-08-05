/**
 * 性能设置 Store
 *
 * 统一管理「设置中心 → 性能」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - PerformanceSettings.vue 与下载页动画逻辑读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 性能设置项集合 */
export interface PerformanceSettings {
  maxAnimationGalleries: number // 下载页支持列表动画的最大画廊个数
}

const STORAGE_KEY = 'saku_performance_settings'

/** 默认值 */
const defaultSettings: PerformanceSettings = {
  maxAnimationGalleries: 30,
}

/** 响应式性能设置（自动持久化） */
export const performanceSettings = reactive<PerformanceSettings>({
  ...defaultSettings,
  ...loadStorage<Partial<PerformanceSettings>>(STORAGE_KEY, {}),
})

watch(
  performanceSettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

/** 恢复默认性能设置 */
export function resetPerformanceSettings(): void {
  Object.assign(performanceSettings, defaultSettings)
}
