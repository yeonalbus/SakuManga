import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export type AppMode = 'online' | 'offline'

/**
 * 全局在线/离线模式（单一数据源）
 *
 * 只有进入 /online/* 或 /offline/* 路由时才更新 currentMode；
 * 当用户停留在 /settings、/downloads、/reader 等非模式页面时，
 * currentMode 会保持进入这些页面之前的值不变，从而保证：
 *   - ModeToggle 按钮不会在设置/下载页误判成「离线」
 *   - 侧边栏显示正确的在线/离线菜单
 *   - 顶栏搜索、筛选、TagChip 快捷搜索使用与当前模式一致的配置
 */
export const useModeStore = defineStore('modeStore', () => {
  const currentMode = ref<AppMode>('online')

  const isOnline = computed(() => currentMode.value === 'online')
  const isOffline = computed(() => currentMode.value === 'offline')

  const setMode = (mode: AppMode) => {
    currentMode.value = mode
  }

  return { currentMode, isOnline, isOffline, setMode }
})
