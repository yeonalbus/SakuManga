/**
 * 鼠标滚轮设置 Store
 *
 * 统一管理「设置中心 → 鼠标滚轮」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - MouseWheelSettings.vue 与阅读器滚轮逻辑读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 鼠标滚轮设置项集合 */
export interface MouseWheelSettings {
  wheelSpeed: number // 鼠标滚轮速度
}

const STORAGE_KEY = 'saku_mouse_wheel_settings'

/** 默认值 */
const defaultSettings: MouseWheelSettings = {
  wheelSpeed: 5.0,
}

/** 响应式鼠标滚轮设置（自动持久化） */
export const mouseWheelSettings = reactive<MouseWheelSettings>({
  ...defaultSettings,
  ...loadStorage<Partial<MouseWheelSettings>>(STORAGE_KEY, {}),
})

watch(
  mouseWheelSettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

/** 恢复默认鼠标滚轮设置 */
export function resetMouseWheelSettings(): void {
  Object.assign(mouseWheelSettings, defaultSettings)
}
