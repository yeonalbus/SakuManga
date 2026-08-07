/**
 * 高级设置 Store
 *
 * 统一管理「设置中心 → 高级」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - AdvancedSettings.vue 与各功能模块读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 *
 * ⚠️ 1.0 收敛：仅保留真正接线生效的最小可用集。
 *   - enableLogs          门控前端 errorReporter 的本地记录与后端落盘上报（前端错误上报）。
 *   - systemLogsEnabled   门控四类操作日志（更新/维护/下载/其他）落盘（Round4 任务七，
 *     由 AdvancedSettings 与后端 /logs/settings 双向同步；本地值仅为展示与即时反馈）。
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 高级设置项集合 */
export interface AdvancedSettings {
  enableLogs: boolean // 前端错误上报（errorReporter 门控）
  systemLogsEnabled: boolean // 启用系统日志（四类操作日志落盘）
}

const STORAGE_KEY = 'saku_advanced_settings'

/** 默认值 */
const defaultSettings: AdvancedSettings = {
  enableLogs: true,
  systemLogsEnabled: true,
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
