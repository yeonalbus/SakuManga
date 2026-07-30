<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import { onlineSearchConfig } from '@/stores/appStore'
import type { OnlineComic } from '@/types/comic'

const comics = ref<OnlineComic[]>([])
const currentPage = ref(1)
const totalPages = ref(1)
const isLoading = ref(false)

// 发起后端请求获取数据
const fetchComics = async () => {
  isLoading.value = true
  try {
    const cfg = onlineSearchConfig.value

    // 构建 GET 查询参数
    const query = new URLSearchParams({
      page: currentPage.value.toString(),
      keyword: cfg.keyword || '',
    })

    if (cfg.activeCategories && cfg.activeCategories.length > 0) {
      cfg.activeCategories.forEach((cat) => query.append('categories', cat))
    }

    const res = await fetch(`http://localhost:8081/api/v1/comics/online?${query.toString()}`)
    const data = await res.json()

    if (res.ok) {
      comics.value = data.comics || []
      totalPages.value = data.totalPages || 1
    } else {
      console.error(data.error || '获取在线漫画失败')
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
