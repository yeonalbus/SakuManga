<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import { onlineSearchConfig } from '@/stores/appStore'
import type { OnlineComic } from '@/types/comic'
import { http } from '@/utils/request'

const comics = ref<OnlineComic[]>([])
const currentPage = ref(1)
const totalPages = ref(1)
const isLoading = ref(false)
const nextCursor = ref('')

// 发起后端请求获取数据
const fetchComics = async () => {
  isLoading.value = true
  try {
    const cfg = onlineSearchConfig.value

    // 1. 构建 GET 查询参数
    const query = new URLSearchParams({
      page: currentPage.value.toString(),
      keyword: cfg.keyword || '',
    })

    // 🟢 如果存在 next 游标则带上
    if (nextCursor.value) {
      query.append('next', nextCursor.value)
    }

    if (cfg.activeCategories && cfg.activeCategories.length > 0) {
      cfg.activeCategories.forEach((cat) => query.append('categories', cat))
    }

    // 🟢 2. 修正：调用正确的 /comics/online 接口，并把 query 参数拼接上去
    const data = await http<{ comics: OnlineComic[]; totalPages: number; currentPage: number }>(
      `/comics/online?${query.toString()}`,
    )

    comics.value = data.comics || []
    totalPages.value = data.totalPages || 1
    nextCursor.value = data.next || '' // 🟢 保存下一页游标
    if (data.currentPage) {
      currentPage.value = data.currentPage
    }
  } catch (err) {
    console.error('网络请求失败:', err)
  } finally {
    isLoading.value = false
  }
}

// 筛选条件变动时重置到第 1 页
watch(
  onlineSearchConfig,
  () => {
    currentPage.value = 1
    fetchComics()
  },
  { deep: true },
)

// 响应来自 GridContainer -> Pagination 的切页事件
const handlePageChange = (newPage: number) => {
  currentPage.value = newPage
  window.scrollTo({ top: 0, behavior: 'smooth' })
  fetchComics()
}

onMounted(() => {
  fetchComics()
})
</script>

<template>
  <div class="online-home-view">
    <div v-if="isLoading" class="loading-state">加载中...</div>
    <GridContainer
      v-else
      :items="comics"
      :current-page="currentPage"
      :total-pages="totalPages"
      @page-change="handlePageChange"
    />
  </div>
</template>

<style scoped>
.online-home-view {
  padding: 20px;
  min-height: 100%;
}

.loading-state {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 300px;
  color: #888;
}
</style>
