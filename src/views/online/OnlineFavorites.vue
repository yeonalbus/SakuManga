<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import OnlineLoadBar from '@/components/OnlineLoadBar.vue'
import FloatingToolbar from '@/components/FloatingToolbar.vue'
import type { OnlineComic } from '@/types/comic'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'

const { toast } = useUI()

// 1. 响应式状态管理
const activeFav = ref(0) // 当前选中的收藏夹 0 ~ 9[cite: 11]
const favComicList = ref<OnlineComic[]>([])
const isLoading = ref(false)
const hasMore = ref(true)
const nextGid = ref<string>('')
const errorMsg = ref<string | null>(null)

// E 站 10 色 Favorite 代表配色[cite: 11]
const favColors = [
  '#7f7f7f', // Fav 0 (灰色)[cite: 11]
  '#f00000', // Fav 1 (红色)[cite: 11]
  '#ff7800', // Fav 2 (橙色)[cite: 11]
  '#f0d000', // Fav 3 (黄色)[cite: 11]
  '#00a0a0', // Fav 4 (绿色)[cite: 11]
  '#98e020', // Fav 5 (浅绿)[cite: 11]
  '#00a0a0', // Fav 6 (青色)[cite: 11]
  '#0000f0', // Fav 7 (蓝色)[cite: 11]
  '#a000a0', // Fav 8 (紫色)[cite: 11]
  '#f000a0', // Fav 9 (粉色)[cite: 11]
]

// 2. 初始化 / 重置加载（切换分类、手动刷新或日期 Seek 时触发）
const fetchFavInitial = async (seekDate?: string) => {
  isLoading.value = true
  errorMsg.value = null
  favComicList.value = []
  nextGid.value = ''
  hasMore.value = true

  try {
    const query = new URLSearchParams({
      favcat: activeFav.value.toString(),
    })
    if (seekDate) {
      query.append('seek', seekDate)
    }

    const data = await http<{
      comics: OnlineComic[]
      next?: string
      hasMore?: boolean
    }>(`/comics/online/favorites?${query.toString()}`)

    favComicList.value = data.comics || []
    nextGid.value = data.next || ''
    hasMore.value = data.hasMore ?? !!data.next
  } catch (err: any) {
    errorMsg.value = err?.message || '获取收藏夹数据失败'
    toast.error('网络连接失败')
  } finally {
    isLoading.value = false
  }
}

// 3. 触底追加加载更多 (Load More)
const loadMoreFav = async () => {
  if (isLoading.value || !hasMore.value || !nextGid.value) return

  isLoading.value = true
  errorMsg.value = null

  try {
    const query = new URLSearchParams({
      favcat: activeFav.value.toString(),
      next: nextGid.value,
    })

    const data = await http<{
      comics: OnlineComic[]
      next?: string
      hasMore?: boolean
    }>(`/comics/online/favorites?${query.toString()}`)

    favComicList.value.push(...(data.comics || []))
    nextGid.value = data.next || ''
    hasMore.value = data.hasMore ?? !!data.next
  } catch (err: any) {
    errorMsg.value = err?.message || '加载更多失败'
    toast.error('加载更多失败')
  } finally {
    isLoading.value = false
  }
}

// 4. 切换收藏夹分类时自动清空并拉取新列表
watch(activeFav, () => {
  fetchFavInitial()
})

const selectFav = (favIndex: number) => {
  activeFav.value = favIndex
}

onMounted(() => {
  fetchFavInitial()
})
</script>

<template>
  <div class="favorites-page">
    <!-- 顶部 10 色收藏夹切换栏 -->
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

    <!-- 图库列表 + 底部触底加载组件 -->
    <GridContainer v-if="favComicList.length > 0" :items="favComicList">
      <template #footer>
        <OnlineLoadBar
          :is-loading="isLoading"
          :has-more="hasMore"
          :error="errorMsg"
          @load-more="loadMoreFav"
        />
      </template>
    </GridContainer>

    <!-- 首次空数据/加载状态 -->
    <div v-else-if="isLoading" class="loading-state">加载收藏夹数据中...</div>
    <div v-else class="empty-tip">该收藏夹下暂无作品</div>

    <!-- 右下角悬浮控制球 -->
    <FloatingToolbar
      @refresh="() => fetchFavInitial()"
      @seek-change="(date) => fetchFavInitial(date)"
    />
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
