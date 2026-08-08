/**
 * 全局卡片展示模式 Store（card / compact）
 * 由原 appStore 拆分而来，负责视图模式状态 + 持久化
 */
import { ref, watch } from 'vue'
import type { CardViewMode } from '@/types/comic'
import { loadStorage, safeSetItem } from '@/utils/storage'

/** 当前视图模式，默认 'card'（卡片），可切换为 'compact'（名片） */
export const viewMode = ref<CardViewMode>(loadStorage('app_view_mode', 'card'))

watch(viewMode, (newVal) => {
  safeSetItem('app_view_mode', JSON.stringify(newVal))
})

/** 在 card / compact 之间切换 */
export const toggleViewMode = () => {
  viewMode.value = viewMode.value === 'card' ? 'compact' : 'card'
}

/* ─────────────────────────────────────────
   每行画廊数（card / compact 各自独立，范围 1-5）
   - 默认：卡片 4 列 / 名片 2 列（与原有桌面布局一致）
   - 用户自定义后 GridContainer 注入 CSS 变量覆盖各断点列数
   ───────────────────────────────────────── */
export const DEFAULT_CARD_COLUMNS = 4
export const DEFAULT_COMPACT_COLUMNS = 2
export const MIN_COLUMNS = 1
export const MAX_COLUMNS = 5

const clampColumns = (n: number): number => {
  if (!Number.isFinite(n)) return DEFAULT_CARD_COLUMNS
  return Math.min(MAX_COLUMNS, Math.max(MIN_COLUMNS, Math.round(n)))
}

/** 卡片模式每行画廊数（1-5） */
export const cardColumns = ref<number>(
  clampColumns(loadStorage<number>('app_card_columns', DEFAULT_CARD_COLUMNS)),
)
/** 名片模式每行画廊数（1-5） */
export const compactColumns = ref<number>(
  clampColumns(loadStorage<number>('app_compact_columns', DEFAULT_COMPACT_COLUMNS)),
)

watch(cardColumns, (n) => {
  safeSetItem('app_card_columns', JSON.stringify(clampColumns(n)))
})
watch(compactColumns, (n) => {
  safeSetItem('app_compact_columns', JSON.stringify(clampColumns(n)))
})
