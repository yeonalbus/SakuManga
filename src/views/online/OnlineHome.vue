<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import { onlineComics, onlineSearchConfig } from '@/stores/appStore'

// 🟢 核心：同时对 keyword + activeCategories + rating + pages 求交集 (AND 管道)
const filteredComics = computed(() => {
  const cfg = onlineSearchConfig.value
  const searchBarKw = (cfg.keyword || '').toLowerCase().trim()

  return onlineComics.value.filter((comic) => {
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

// 分页逻辑
const currentPage = ref(1)
const pageSize = 24

// 只要搜索词或筛选条件改变，自动回到第 1 页
watch(
  onlineSearchConfig,
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
  <div class="online-home-view">
    <GridContainer
      :items="currentPageItems"
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
</style>
