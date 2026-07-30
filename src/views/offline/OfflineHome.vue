<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import GridContainer from '@/components/GridContainer.vue'
// 🟢 1. 正确引入离线数据源 (offlineComics) 与 离线专属搜索筛选配置 (offlineSearchConfig)
import { offlineComics, offlineSearchConfig } from '@/stores/appStore'

const route = useRoute()

// 🟢 2. 核心过滤管道：兼顾 URL 中的 ?q= 搜索词 与 TopBar 传进来的离线筛选配置 (求交集)
const filteredComics = computed(() => {
  const cfg = offlineSearchConfig.value
  const searchBarKw = (cfg.keyword || '').toLowerCase().trim()

  return offlineComics.value.filter((comic) => {
    // 关卡 1：顶栏 SearchBar 的主搜索词匹配
    if (searchBarKw) {
      const matchTitle = comic.title.toLowerCase().includes(searchBarKw)
      const matchTag = comic.tags?.some((t) => t.toLowerCase().includes(searchBarKw))
      if (!matchTitle && !matchTag) return false
    }

    // 🟢 关卡 2：筛选抽屉中的“多关键词队列”过滤 (必须同时匹配队列里的每一个词)
    if (cfg.keywords && cfg.keywords.length > 0) {
      const allMatched = cfg.keywords.every((filterKw) => {
        const lowerKw = filterKw.toLowerCase()
        const matchTitle = comic.title.toLowerCase().includes(lowerKw)
        const matchTag = comic.tags?.some((t) => t.toLowerCase().includes(lowerKw))
        return matchTitle || matchTag
      })

      if (!allMatched) return false // 只要有一个词不满足，就过滤掉
    }

    // 关卡 3：分类匹配
    if (cfg.activeCategories && cfg.activeCategories.length > 0) {
      if (comic.category && !cfg.activeCategories.includes(comic.category)) {
        return false
      }
    }

    // 关卡 4：最低评分与页数范围...
    if (cfg.minRating && (comic.rating || 0) < cfg.minRating) return false

    return true
  })
})
// 🟢 3. 分页控制逻辑
const currentPage = ref(1)
const pageSize = 24

// 当搜索词 (route.query.q) 或 离线筛选条件 (offlineSearchConfig) 发生变化时，自动跳回第 1 页
watch(
  [() => route.query.q, offlineSearchConfig],
  () => {
    currentPage.value = 1
  },
  { deep: true },
)

const totalPages = computed(() => Math.ceil(filteredComics.value.length / pageSize) || 1)

const currentPageItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return filteredComics.value.slice(start, start + pageSize)
})

const handlePageChange = (newPage: number) => {
  currentPage.value = newPage
  window.scrollTo({ top: 0, behavior: 'smooth' })
}
</script>

<template>
  <div class="offline-home-view">
    <GridContainer
      :items="currentPageItems"
      :current-page="currentPage"
      :total-pages="totalPages"
      @page-change="handlePageChange"
    />
  </div>
</template>

<style scoped>
.offline-home-view {
  padding: 20px;
  min-height: 100%;
}
</style>
