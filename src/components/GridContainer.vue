<script setup lang="ts">
import { computed } from 'vue'
import {
  viewMode,
  cardColumns,
  compactColumns,
  DEFAULT_CARD_COLUMNS,
  DEFAULT_COMPACT_COLUMNS,
} from '@/stores/viewMode'
import type { ComicItem } from '@/types/comic'
import ItemCard from './ItemCard.vue'

defineProps<{
  items: ComicItem[]
  /** 是否允许长按卡片进入选择模式（仅离线漫画生效） */
  selectable?: boolean
  /** 是否处于选择模式 */
  selectMode?: boolean
  /** 已选中的漫画 id 列表 */
  selectedIds?: string[]
  /** 左右分栏面板模式：开启后点击在线卡片发 open 事件而非跳转路由 */
  panelMode?: boolean
}>()

const emit = defineEmits<{
  (e: 'longpress', comic: ComicItem): void
  (e: 'select', comic: ComicItem): void
  (e: 'open', comic: ComicItem): void
}>()

/** 用户自定义「每行画廊数」时注入 CSS 变量覆盖各断点列数；
    保持默认则不注入，交由视口媒体查询渐进降级 */
const gridStyle = computed<Record<string, string>>(() => {
  const style: Record<string, string> = {}
  if (cardColumns.value !== DEFAULT_CARD_COLUMNS) style['--card-cols'] = String(cardColumns.value)
  if (compactColumns.value !== DEFAULT_COMPACT_COLUMNS) {
    style['--compact-cols'] = String(compactColumns.value)
  }
  return style
})
</script>

<template>
  <div class="grid-container-wrapper">
    <!-- 🟢 1. 顶部扩展插槽（向上加载更多 / 较新内容） -->
    <div v-if="$slots.header" class="grid-header">
      <slot name="header" />
    </div>

    <!-- 2. 网格主体 -->
    <div class="card-grid" :class="viewMode" :style="gridStyle">
      <ItemCard
        v-for="item in items"
        :key="item.id"
        :comic="item"
        :selectable="selectable"
        :select-mode="selectMode"
        :selected="selectedIds?.includes(item.id) ?? false"
        :panel-mode="panelMode"
        @longpress="(c) => emit('longpress', c)"
        @select="(c) => emit('select', c)"
        @open="(c) => emit('open', c)"
      />
    </div>

    <!-- 3. 底部扩展插槽 -->
    <div v-if="$slots.footer" class="grid-footer">
      <slot name="footer" />
    </div>
  </div>
</template>

<style scoped>
.grid-container-wrapper {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 网格基础布局 */
.card-grid {
  display: grid;
  gap: 16px;
}

/* 卡片模式 (card)：桌面 4 列网格（--card-cols 由用户「每行画廊数」自定义注入） */
.card-grid.card {
  grid-template-columns: repeat(var(--card-cols, 4), 1fr);
}

/* 名片模式 (compact)：桌面 2 列网格（--compact-cols 由用户「每行画廊数」自定义注入） */
.card-grid.compact {
  grid-template-columns: repeat(var(--compact-cols, 2), 1fr);
  gap: 12px;
}

.grid-header,
.grid-footer {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 8px 0;
}

/* 📱 响应式列数（未自定义每行画廊数时生效）：
   - iPad 竖屏(≤1024px)：card 3 列
   - 手机/小平板(≤768px)：card 2 列
   - 手机竖屏(≤480px)：compact 单列，卡片更易点按
   一旦用户通过「每行画廊数」注入 --card-cols / --compact-cols，
   各断点统一使用该值，忽略视口渐进。 */
@media (max-width: 1024px) {
  .card-grid.card {
    grid-template-columns: repeat(var(--card-cols, 3), 1fr);
  }
}

@media (max-width: 768px) {
  .card-grid.card {
    grid-template-columns: repeat(var(--card-cols, 2), 1fr);
  }
}

@media (max-width: 480px) {
  .card-grid.card {
    grid-template-columns: repeat(var(--card-cols, 2), 1fr);
    gap: 10px;
  }
  .card-grid.compact {
    grid-template-columns: repeat(var(--compact-cols, 1), 1fr);
  }
}

/* 🖥️ 手动布局模式：仅当用户强制「桌面/移动」时固定列数（覆盖上方 @media 渐进）；
   自动模式不带 data-layout-force，仍按视口渐进（iPad 横屏 3 列、手机 2 列等） */
/* ⚠️ :global() 需包裹完整选择器（含子类名），否则 scoped 编译会丢弃类名、grid 规则直接作用在 <html> 上 */
:global(html[data-layout-force][data-layout='desktop'] .card-grid.card) {
  grid-template-columns: repeat(var(--card-cols, 4), 1fr);
}
:global(html[data-layout-force][data-layout='desktop'] .card-grid.compact) {
  grid-template-columns: repeat(var(--compact-cols, 2), 1fr);
  gap: 12px;
}
:global(html[data-layout-force][data-layout='mobile'] .card-grid.card) {
  grid-template-columns: repeat(var(--card-cols, 2), 1fr);
  gap: 10px;
}
:global(html[data-layout-force][data-layout='mobile'] .card-grid.compact) {
  grid-template-columns: repeat(var(--compact-cols, 1), 1fr);
}
</style>
