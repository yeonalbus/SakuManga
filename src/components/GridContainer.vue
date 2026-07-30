<script setup lang="ts">
import { viewMode } from '@/stores/viewMode'
import type { ComicItem } from '@/types/comic' // 1. 引入统一数据类型
import ItemCard from './ItemCard.vue'
import Pagination from './PagiNation.vue' // 2. 统一组件命名拼写

defineProps<{
  items: ComicItem[] // 3. 严格约束列表输入为 ComicItem[]
  currentPage?: number
  totalPages?: number
}>()

const emit = defineEmits<{
  (e: 'page-change', page: number): void
}>()
</script>

<template>
  <div class="grid-container-wrapper">
    <div class="card-grid" :class="viewMode">
      <ItemCard v-for="item in items" :key="item.id" :comic="item" />
    </div>

    <Pagination
      v-if="totalPages && totalPages >= 1"
      :current-page="currentPage || 1"
      :total-pages="totalPages"
      @change="(page) => emit('page-change', page)"
    />
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

/* ─── 1. 卡片模式 (card)：4 列网格 ─── */
.card-grid.card {
  grid-template-columns: repeat(4, 1fr);
}

/* ─── 2. 名片模式 (compact)：2 列网格 ─── */
.card-grid.compact {
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}
</style>
