<script setup lang="ts">
import { viewMode } from '@/stores/viewMode'
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
}>()

const emit = defineEmits<{
  (e: 'longpress', comic: ComicItem): void
  (e: 'select', comic: ComicItem): void
}>()
</script>

<template>
  <div class="grid-container-wrapper">
    <!-- 🟢 1. 顶部扩展插槽（向上加载更多 / 较新内容） -->
    <div v-if="$slots.header" class="grid-header">
      <slot name="header" />
    </div>

    <!-- 2. 网格主体 -->
    <div class="card-grid" :class="viewMode">
      <ItemCard
        v-for="item in items"
        :key="item.id"
        :comic="item"
        :selectable="selectable"
        :select-mode="selectMode"
        :selected="selectedIds?.includes(item.id) ?? false"
        @longpress="(c) => emit('longpress', c)"
        @select="(c) => emit('select', c)"
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
  min-height: 100%;
  gap: 20px;
}

/* 网格基础布局 */
.card-grid {
  display: grid;
  gap: 16px;
  flex: 1;
}

/* 卡片模式 (card)：桌面 4 列网格 */
.card-grid.card {
  grid-template-columns: repeat(4, 1fr);
}

/* 名片模式 (compact)：桌面 2 列网格 */
.card-grid.compact {
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.grid-header,
.grid-footer {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 8px 0;
}

/* 📱 响应式列数：
   - iPad 竖屏(≤1024px)：card 3 列
   - 手机/小平板(≤768px)：card 2 列
   - 手机竖屏(≤480px)：compact 单列，卡片更易点按 */
@media (max-width: 1024px) {
  .card-grid.card {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (max-width: 768px) {
  .card-grid.card {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 480px) {
  .card-grid.card {
    grid-template-columns: repeat(2, 1fr);
    gap: 10px;
  }
  .card-grid.compact {
    grid-template-columns: 1fr;
  }
}
</style>
