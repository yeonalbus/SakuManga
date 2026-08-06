<script setup lang="ts">
import { ref, onMounted } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import FloatingToolbar from '@/components/FloatingToolbar.vue'
import type { OnlineComic } from '@/types/comic'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'

const { toast } = useUI()
const hotComics = ref<OnlineComic[]>([])
const isLoading = ref(true)

// 拉取真实热门数据
const fetchPopularComics = async () => {
  isLoading.value = true
  try {
    const data = await http<{ comics: OnlineComic[] }>('/comics/online/popular')
    hotComics.value = data.comics || []
  } catch (err) {
    toast.error('网络连接失败')
    console.error(err)
  } finally {
    isLoading.value = false
  }
}

onMounted(() => {
  fetchPopularComics()
})
</script>

<template>
  <div class="page-wrapper">
    <!-- 仅在首次无数据且加载中时显示全局居中 Loading -->
    <div v-if="isLoading && hotComics.length === 0" class="loading-state">正在拉取全站热门...</div>

    <!-- 列表展示 -->
    <GridContainer v-else-if="hotComics.length > 0" :items="hotComics" />

    <!-- 空数据状态 -->
    <div v-else class="empty-tip">暂无热门数据</div>

    <!-- 右下角悬浮球：提供一键刷新与回到顶部 -->
    <FloatingToolbar @refresh="fetchPopularComics" />
  </div>
</template>

<style scoped>
.page-wrapper {
  padding: 20px;
  min-height: 100%;
}

.loading-state,
.empty-tip {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 300px;
  color: var(--app-text-3);
}
</style>
