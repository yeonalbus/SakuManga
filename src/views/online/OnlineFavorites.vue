<script setup lang="ts">
import { ref, watch } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import type { OnlineComic } from '@/types/comic'
import { useUI } from '@/composables/useUI'

const { toast } = useUI()

const activeFav = ref(0) // 当前选中的收藏夹 0 ~ 9
const currentPage = ref(1)
const totalPages = ref(1)
const isLoading = ref(false)
const favComicList = ref<OnlineComic[]>([])

// E 站 10 色 Favorite 代表配色
const favColors = [
  '#7f7f7f', // Fav 0 (灰色)
  '#f00000', // Fav 1 (红色)
  '#ff7800', // Fav 2 (橙色)
  '#f0d000', // Fav 3 (黄色)
  '#00a0a0', // Fav 4 (绿色)
  '#98e020', // Fav 5 (浅绿)
  '#00a0a0', // Fav 6 (青色)
  '#0000f0', // Fav 7 (蓝色)
  '#a000a0', // Fav 8 (紫色)
  '#f000a0', // Fav 9 (粉色)
]

// 核心：从 Go 后端拉取真实收藏夹列表
const fetchFavData = async () => {
  isLoading.value = true
  try {
    const res = await fetch(
      `http://localhost:8080/api/v1/comics/online/favorites?favcat=${activeFav.value}&page=${currentPage.value}`,
    )
    const data = await res.json()

    if (res.ok) {
      favComicList.value = data.comics || []
      totalPages.value = data.totalPages || 1
    } else {
      toast.error(data.error || '获取收藏夹失败')
    }
  } catch (err) {
    toast.error('网络连接失败')
    console.error(err)
  } finally {
    isLoading.value = false
  }
}

// 切换收藏夹分类时重置页码并重新加载
watch(activeFav, () => {
  currentPage.value = 1
  fetchFavData()
})

const selectFav = (favIndex: number) => {
  activeFav.value = favIndex
}

// 切页响应
const handlePageChange = (page: number) => {
  currentPage.value = page
  window.scrollTo({ top: 0, behavior: 'smooth' })
  fetchFavData()
}

// 初始触发加载
fetchFavData()
</script>

<template>
  <div class="favorites-page">
    <div class="fav-grid">
      <button
        v-for="i in 10"
        :key="i - 1"
        class="fav-btn"
        :class="{ active: activeFav === i - 1 }"
        :style="{
          backgroundColor: activeFav === i - 1 ? favColors[i - 1] : '#1a1a1d',
          borderColor: activeFav === i - 1 ? favColors[i - 1] : '#2a2a2d',
          color: activeFav === i - 1 ? '#ffffff' : '#aaa',
        }"
        @click="selectFav(i - 1)"
      >
        <span class="fav-dot" :style="{ backgroundColor: favColors[i - 1] }"></span>
        Favorites {{ i - 1 }}
      </button>
    </div>

    <div v-if="isLoading" class="loading-state">加载收藏夹数据中...</div>

    <GridContainer
      v-else-if="favComicList.length > 0"
      :items="favComicList"
      :current-page="currentPage"
      :total-pages="totalPages"
      @page-change="handlePageChange"
    />

    <div v-else class="empty-tip">该收藏夹下暂无作品</div>
  </div>
</template>

<style scoped>
.favorites-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 20px;
  min-height: 100%;
}

.loading-state,
.empty-tip {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 300px;
  color: #888;
}

.fav-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 8px;
  padding-bottom: 12px;
  border-bottom: 1px solid #2a2a2a;
}

.fav-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid #2a2a2d;
  border-radius: 6px;
  font-size: 0.82rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
  user-select: none;
}

.fav-btn:hover {
  transform: translateY(-1px);
  filter: brightness(1.15);
}

.fav-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
</style>
