/**
 * 高级设置 Store
 *
 * 统一管理「设置中心 → 高级」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - AdvancedSettings.vue 与各功能模块读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 高级设置项集合 */
export interface AdvancedSettings {
  enableLogs: boolean // 开启日志（需要重启）
  recordAllLogs: boolean // 记录全部日志（需要重启）
  checkUpdatesOnStartup: boolean // 启动应用时检查更新
  detectClipboardLinks: boolean // 检测剪切板中的画廊链接
  noImageMode: boolean // 无图模式
}

const STORAGE_KEY = 'saku_advanced_settings'

/** 默认值 */
const defaultSettings: AdvancedSettings = {
  enableLogs: true,
  recordAllLogs: false,
  checkUpdatesOnStartup: true,
  detectClipboardLinks: true,
  noImageMode: false,
}

/** 响应式高级设置（自动持久化） */
export const advancedSettings = reactive<AdvancedSettings>({
  ...defaultSettings,
  ...loadStorage<Partial<AdvancedSettings>>(STORAGE_KEY, {}),
})

watch(
  advancedSettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

/** 恢复默认高级设置 */
export function resetAdvancedSettings(): void {
  Object.assign(advancedSettings, defaultSettings)
}
