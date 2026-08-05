<script setup lang="ts">
import { ref } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import type { OfflineComic } from '@/types/comic'

const currentPage = ref(1)
const totalPages = ref(2)

// GridContainer 的 items 需要完整的 ComicItem 结构，这里补齐必要字段
const items = ref<OfflineComic[]>(
  Array.from({ length: 24 }, (_, i) => ({
    id: `offline-up-${i + 1}`,
    title: `🔄 本地检测到有新话/增补的作品 #${i + 1}`,
    coverUrl: '',
    source: 'offline' as const,
    tags: [],
    updatedAt: '2026-07-28',
    localPath: '',
  })),
)

const handlePageChange = (page: number) => {
  currentPage.value = page
}
</script>

<template>
  <div class="page-wrapper">
    <GridContainer
      :items="items"
      :current-page="currentPage"
      :total-pages="totalPages"
      @page-change="handlePageChange"
    />
  </div>
</template>

<style scoped>
.page-wrapper {
  height: 100%;
}
</style>
