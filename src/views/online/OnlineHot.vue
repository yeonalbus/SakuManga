<script setup lang="ts">
import { ref, onMounted } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import type { OnlineComic } from '@/types/comic'
import { useUI } from '@/composables/useUI'

const { toast } = useUI()
const hotComics = ref<OnlineComic[]>([])
const isLoading = ref(true)

// 拉取真实热门数据
const fetchPopularComics = async () => {
  isLoading.value = true
  try {
    const res = await fetch('http://localhost:8081/api/v1/comics/online/popular')
    const data = await res.json()

    if (res.ok) {
      hotComics.value = data.comics || []
    } else {
      toast.error(data.error || '获取热门失败')
    }
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
    <div v-if="isLoading" class="loading-state">正在拉取全站热门...</div>
    <GridContainer v-else-if="hotComics.length > 0" :items="hotComics" />
    <div v-else class="empty-tip">暂无热门数据</div>
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
  color: #888;
}
</style>
