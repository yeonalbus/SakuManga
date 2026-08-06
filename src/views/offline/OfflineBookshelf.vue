<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRoute } from 'vue-router'
// 🟢 1. 按领域引入：漫画数据源来自 comicStore，书架信息来自 bookshelfStore
import { offlineComics, deleteOfflineComics } from '@/stores/comicStore'
import { computedBookshelves } from '@/stores/bookshelfStore'
import type { Bookshelf, OfflineComic, ComicItem } from '@/types/comic'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/Pagination.vue'
import { useUI } from '@/composables/useUI'

const { toast, modal } = useUI()

const route = useRoute()

// 1. 获取当前路由中的书架 ID (?id=xxx)
const currentShelfId = computed(() => (route.query.id as string) || '')

// 🟢 2. 从 Store 中查找当前书架信息
// 说明：fallback 不再引用 shelfComics，避免与下方 computed 形成循环依赖
const currentShelf = computed<Bookshelf>(() => {
  return (
    computedBookshelves.value.find((s) => s.id === currentShelfId.value) || {
      id: currentShelfId.value || 'all',
      name: '全部离线作品',
      // 无匹配书架时 count 展示由 shelfComics.length 负责，这里仅补默认值保证类型完整
      count: 0,
    }
  )
})

// 🟢 3. 核心计算：根据当前书架 ID 动态过滤 Store 里的离线漫画
const shelfComics = computed<OfflineComic[]>(() => {
  if (!currentShelfId.value) {
    // 如果没有传 id 参数，默认展示全部离线漫画
    return offlineComics.value
  }
  // 匹配 bookshelfId 或包含在 comicIds 中的漫画
  return offlineComics.value.filter((comic) => {
    return (
      comic.bookshelfId === currentShelfId.value ||
      (currentShelf.value.comicIds && currentShelf.value.comicIds.includes(comic.id))
    )
  })
})

// 4. 分页逻辑
const currentPage = ref(1)
const pageSize = 24

// 切换书架时仅需将页码重置为 1
watch(currentShelfId, () => {
  currentPage.value = 1
})

const totalPages = computed(() => Math.ceil(shelfComics.value.length / pageSize) || 1)

const currentPageItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return shelfComics.value.slice(start, start + pageSize)
})

const handlePageChange = (page: number) => {
  currentPage.value = page
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

// --------------------------------------------------
// 长按选择 / 批量删除
// --------------------------------------------------
const selectMode = ref(false)
const selectedIds = ref<string[]>([])

const toggleSelect = (comic: ComicItem) => {
  const idx = selectedIds.value.indexOf(comic.id)
  if (idx >= 0) selectedIds.value.splice(idx, 1)
  else selectedIds.value.push(comic.id)
}

const handleLongPress = (comic: ComicItem) => {
  if (comic.source !== 'offline') return
  selectMode.value = true
  toggleSelect(comic)
}

const handleSelect = (comic: ComicItem) => toggleSelect(comic)

const exitSelectMode = () => {
  selectMode.value = false
  selectedIds.value = []
}

const toggleSelectAllPage = () => {
  const pageIds = currentPageItems.value.map((c) => c.id)
  const allSelected = pageIds.every((id) => selectedIds.value.includes(id))
  if (allSelected) {
    selectedIds.value = selectedIds.value.filter((id) => !pageIds.includes(id))
  } else {
    selectedIds.value = Array.from(new Set([...selectedIds.value, ...pageIds]))
  }
}

const handleDeleteSelected = async () => {
  if (selectedIds.value.length === 0) return
  const confirmed = await modal.confirm(
    `确定要删除选中的 ${selectedIds.value.length} 部作品吗？\n将同时移除书架与历史记录中的引用。`,
    '删除选中作品',
  )
  if (!confirmed) return
  const alsoDeleteFile = await modal.confirm(
    '是否同时删除本地文件？\n选择「确定」将永久删除磁盘上的漫画文件，无法恢复。',
    '删除本地文件',
  )
  const okCount = await deleteOfflineComics(selectedIds.value, alsoDeleteFile)
  if (okCount > 0) {
    toast.success(
      alsoDeleteFile ? `已删除 ${okCount} 部作品及其本地文件` : `已删除 ${okCount} 部作品`,
    )
  } else {
    toast.error('删除失败，请重试')
  }
  exitSelectMode()
}
</script>

<template>
  <div class="offline-bookshelf-view">
    <div class="shelf-header">
      <div class="title-area">
        <span class="icon">📁</span>
        <h2 class="shelf-title">{{ currentShelf.name }}</h2>
        <span class="shelf-badge">{{ shelfComics.length }} 部作品</span>
      </div>
    </div>

    <!-- 选择模式工具条 -->
    <div v-if="selectMode" class="select-toolbar">
      <span class="select-count">已选 {{ selectedIds.length }} 部</span>
      <button class="toolbar-btn" @click="toggleSelectAllPage">全选本页</button>
      <button
        class="toolbar-btn danger"
        :disabled="selectedIds.length === 0"
        @click="handleDeleteSelected"
      >
        🗑️ 删除
      </button>
      <button class="toolbar-btn" @click="exitSelectMode">取消</button>
    </div>

    <!-- 使用 #footer 插槽挂载页码组件 -->
    <GridContainer
      :items="currentPageItems"
      :selectable="true"
      :select-mode="selectMode"
      :selected-ids="selectedIds"
      @longpress="handleLongPress"
      @select="handleSelect"
    >
      <template #footer>
        <Pagination
          v-if="totalPages >= 1"
          :current-page="currentPage"
          :total-pages="totalPages"
          @change="handlePageChange"
        />
      </template>
    </GridContainer>
  </div>
</template>

<style scoped>
.offline-bookshelf-view {
  padding: 20px;
  min-height: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.shelf-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
}

.title-area {
  display: flex;
  align-items: center;
  gap: 10px;
}

.icon {
  font-size: 1.5rem;
}

.shelf-title {
  margin: 0;
  font-size: 1.3rem;
  font-weight: 600;
  color: var(--app-text-strong);
}

.shelf-badge {
  background-color: var(--app-surface-3);
  color: var(--app-text-2);
  font-size: 0.8rem;
  padding: 2px 8px;
  border-radius: 12px;
  margin-left: 8px;
}

.select-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
}

.select-count {
  color: var(--app-text-strong);
  font-size: 0.95rem;
  font-weight: 500;
}

.toolbar-btn {
  background-color: var(--app-border-2);
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 6px 14px;
  font-size: 0.85rem;
  cursor: pointer;
  transition:
    background-color 0.2s,
    border-color 0.2s;
}

.toolbar-btn:hover:not(:disabled) {
  background-color: var(--app-surface-3-hover);
  border-color: var(--app-border-3);
}

.toolbar-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.toolbar-btn.danger {
  color: #ff7588;
  border-color: #ff7588;
}

.toolbar-btn.danger:hover:not(:disabled) {
  background-color: rgba(255, 117, 136, 0.12);
}
</style>
