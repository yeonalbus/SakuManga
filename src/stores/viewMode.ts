/**
 * 全局卡片展示模式 Store（card / compact）
 * 由原 appStore 拆分而来，负责视图模式状态 + 持久化
 */
import { ref, watch } from 'vue'
import type { CardViewMode } from '@/types/comic'
import { loadStorage } from '@/utils/storage'

/** 当前视图模式，默认 'card'（卡片），可切换为 'compact'（名片） */
export const viewMode = ref<CardViewMode>(loadStorage('app_view_mode', 'card'))

watch(viewMode, (newVal) => {
  localStorage.setItem('app_view_mode', JSON.stringify(newVal))
})

/** 在 card / compact 之间切换 */
export const toggleViewMode = () => {
  viewMode.value = viewMode.value === 'card' ? 'compact' : 'card'
}
