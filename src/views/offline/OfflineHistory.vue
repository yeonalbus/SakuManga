<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { offlineHistoryList, clearHistory, loadHistory } from '@/stores/historyStore'
// Round20-Bug1/D2：进入历史页先刷新离线列表与历史（按 gid 去重 + 剔除本地库已移除的孤儿条目）
import { fetchOfflineComics } from '@/stores/comicStore'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/Pagination.vue'
// Round22：历史页多选快捷加入书架（共享 composable + 工具条 + 书架浮层）
import { useShelfQuickAdd } from '@/composables/useShelfQuickAdd'
import ShelfQuickAddToolbar from '@/components/ShelfQuickAddToolbar.vue'
import BookshelfPickerOverlay from '@/components/BookshelfPickerOverlay.vue'
// 问题3：主滚动容器是 #main-content，翻页回顶必须用它而非 window
// 任务五：列表状态记忆（页码 + 滚动位置），返回时「从哪里来回哪里去」
// Round7-任务4：注册列表状态提供者，供打开详情（新标签）前捕获 { top, page }
import {
  scrollMainToTop,
  rememberListState,
  takeListState,
  setListStateProvider,
  clearListStateProvider,
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
  // Round20-Bug1/D2：刷新离线列表与历史（服务端更新替换换 id / 删除漫画后，历史可能残留孤儿条目）
  await fetchOfflineComics()
  await loadHistory('offline')
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
  // Round7-任务4：注册列表状态提供者（打开详情新标签前捕获 { top, page }）
  setListStateProvider('/offline/history', () => ({
    top: getMainContent()?.scrollTop || 0,
    page: currentPage.value,
  }))
})

onUnmounted(() => {
  clearListStateProvider('/offline/history')
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

// Round22：多选快捷加入书架（历史项均为离线漫画）
const quickAdd = useShelfQuickAdd(() => currentPageItems.value)

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

    <!-- Round22：多选快捷加入工具条 -->
    <ShelfQuickAddToolbar
      v-if="quickAdd.selectMode.value"
      :count="quickAdd.selectedIds.value.length"
      @select-all="quickAdd.toggleSelectAllPage()"
      @add="quickAdd.openShelfPicker()"
      @close="quickAdd.exitSelectMode()"
    />

    <!-- Round7-任务6：历史入口卡片，点击详情后「立即阅读」始终从上次位置开始 -->
    <GridContainer
      v-if="comics.length > 0"
      :items="currentPageItems"
      :from-history="true"
      :selectable="true"
      :select-mode="quickAdd.selectMode.value"
      :selected-ids="quickAdd.selectedIds.value"
      @longpress="quickAdd.handleLongPress"
      @select="quickAdd.handleSelect"
    >
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

    <!-- Round22：多选快捷加入书架（检索浮层 add 模式） -->
    <BookshelfPickerOverlay
      :open="quickAdd.showShelfPicker.value"
      mode="add"
      :selected-count="quickAdd.selectedIds.value.length"
      @close="quickAdd.showShelfPicker.value = false"
      @add="quickAdd.handleAddToShelf"
    />
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
