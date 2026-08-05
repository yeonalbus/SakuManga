<script setup lang="ts">
import { ref, computed } from 'vue'
import { offlineHistoryList, clearHistory } from '@/stores/historyStore'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/Pagination.vue'

// 动态提取离线浏览过的漫画列表
const comics = computed(() => offlineHistoryList.value.map((item) => item.comic))

// 分页逻辑
const currentPage = ref(1)
const pageSize = 24

const totalPages = computed(() => Math.ceil(comics.value.length / pageSize) || 1)

const currentPageItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return comics.value.slice(start, start + pageSize)
})

const handlePageChange = (newPage: number) => {
  currentPage.value = newPage
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

const handleClear = () => {
  clearHistory('offline')
}
</script>

<template>
  <div class="history-view">
    <div class="view-header">
      <h2 class="view-title">📚 本地浏览历史 ({{ comics.length }})</h2>
      <button v-if="comics.length > 0" class="clear-btn" @click="handleClear">
        🗑️ 清空本地历史
      </button>
    </div>

    <GridContainer v-if="comics.length > 0" :items="currentPageItems">
      <!-- 通过 #footer 插槽挂载数字分页组件 -->
      <template #footer>
        <Pagination
          v-if="totalPages >= 1"
          :current-page="currentPage"
          :total-pages="totalPages"
          @change="handlePageChange"
        />
      </template>
    </GridContainer>

    <div v-else class="empty-tip">暂无本地浏览记录</div>
  </div>
</template>

<style scoped>
.history-view {
  padding: 20px;
  min-height: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.view-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 12px;
  border-bottom: 1px solid #2a2a2e;
}

.view-title {
  font-size: 1.2rem;
  font-weight: 600;
  color: #fff;
  margin: 0;
}

.clear-btn {
  background-color: #2a2a2e;
  border: 1px solid #3d3d42;
  color: #ef4444;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
}

.clear-btn:hover {
  background-color: rgba(239, 68, 68, 0.15);
  border-color: #ef4444;
}

.empty-tip {
  margin-top: 60px;
  text-align: center;
  color: #66666c;
  font-size: 0.95rem;
}
</style>
