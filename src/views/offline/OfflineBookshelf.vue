<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRoute } from 'vue-router'
// 🟢 1. 从 appStore 引入全局响应式数据 offlineComics 与 computedBookshelves
import { offlineComics, computedBookshelves } from '@/stores/appStore'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/PagiNation.vue' // 👈 引入页码组件

const route = useRoute()

// 1. 获取当前路由中的书架 ID (?id=xxx)
const currentShelfId = computed(() => (route.query.id as string) || '')

// 🟢 2. 从 Store 中查找当前书架信息（包含动态计算出的真实 count）
const currentShelf = computed(() => {
  return (
    computedBookshelves.value.find((s) => s.id === currentShelfId.value) || {
      id: 'all',
      name: '全部离线作品',
      count: shelfComics.value.length,
    }
  )
})

// 🟢 3. 核心计算：根据当前书架 ID 动态过滤 Store 里的离线漫画
const shelfComics = computed(() => {
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

    <!-- 使用 #footer 插槽挂载页码组件 -->
    <GridContainer :items="currentPageItems">
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
  border-bottom: 1px solid #2a2a2a;
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
  color: #ffffff;
}

.shelf-badge {
  background-color: #2a2a2e;
  color: #a0a0a5;
  font-size: 0.8rem;
  padding: 2px 8px;
  border-radius: 12px;
  margin-left: 8px;
}
</style>
