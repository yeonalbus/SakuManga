<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { offlineHistoryList, clearHistory } from '@/stores/historyStore'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/Pagination.vue'
// 问题3：主滚动容器是 #main-content，翻页回顶必须用它而非 window
// 任务五：列表状态记忆（页码 + 滚动位置），返回时「从哪里来回哪里去」
import {
  scrollMainToTop,
  rememberListState,
  takeListState,
  getMainContent,
} from '@/utils/scrollMemory'

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

// 任务五：进入页面时恢复页码；滚动位置在渲染后恢复
onMounted(async () => {
  const saved = takeListState('/offline/history')
  if (saved?.page && saved.page > 1) {
    currentPage.value = saved.page
  }
  if (saved && saved.top > 0) {
    await nextTick()
    requestAnimationFrame(() => {
      const el = getMainContent()
      if (el && el.scrollHeight > 0) el.scrollTop = saved.top
    })
  }
})

// 任务五：离开列表页时保存「页码 + 滚动位置」
onBeforeRouteLeave(() => {
  rememberListState('/offline/history', {
    top: getMainContent()?.scrollTop || 0,
    page: currentPage.value,
  })
})

const handlePageChange = (newPage: number) => {
  currentPage.value = newPage
  // 问题3：真实滚动容器是 #main-content，window.scrollTo 无效
  scrollMainToTop('smooth')
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
  padding: 12px 4px;
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
  border-bottom: 1px solid var(--app-border-2);
}

.view-title {
  font-size: 1.2rem;
  font-weight: 600;
  color: var(--app-text-strong);
  margin: 0;
}

.clear-btn {
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
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
  color: var(--app-text-muted);
  font-size: 0.95rem;
}
</style>
