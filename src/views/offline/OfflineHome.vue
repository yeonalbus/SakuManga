<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/Pagination.vue'
// 🟢 1. 正确引入离线数据源 (offlineComics) 与 离线专属搜索筛选配置 (offlineSearchConfig)
import { offlineComics, fetchOfflineComics, deleteOfflineComics } from '@/stores/comicStore'
import { offlineSearchConfig } from '@/stores/searchStore'
import { useUI } from '@/composables/useUI'
import type { ComicItem } from '@/types/comic'

const { toast, modal } = useUI()

onMounted(() => {
  fetchOfflineComics()
})

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

const route = useRoute()

// 🟢 2. 核心过滤管道：兼顾 URL 中的 ?q= 搜索词 与 TopBar 传进来的离线筛选配置 (求交集)
const filteredComics = computed(() => {
  const cfg = offlineSearchConfig.value
  const searchBarKw = (cfg.keyword || '').toLowerCase().trim()

  // f_sft 禁用 Tag 过滤的本地语义：开启后关键词只匹配标题，不再匹配 Tag
  const matchTagEnabled = !cfg.disableTagFilter

  // 语言筛选 (E 站语言过滤的本地映射)：未禁用语言过滤时按 language:xxx Tag 过滤
  const langTag =
    !cfg.disableLangFilter && cfg.language && cfg.language !== 'All'
      ? `language:${cfg.language.toLowerCase()}`
      : ''

  return offlineComics.value.filter((comic) => {
    // 关卡 1：顶栏 SearchBar 的主搜索词匹配
    if (searchBarKw) {
      const matchTitle = comic.title.toLowerCase().includes(searchBarKw)
      const matchTag =
        matchTagEnabled && comic.tags?.some((t) => t.toLowerCase().includes(searchBarKw))
      if (!matchTitle && !matchTag) return false
    }

    // 🟢 关卡 2：筛选抽屉中的“多关键词队列”过滤 (必须同时匹配队列里的每一个词)
    if (cfg.keywords && cfg.keywords.length > 0) {
      const allMatched = cfg.keywords.every((filterKw: string) => {
        const lowerKw = filterKw.toLowerCase()
        const matchTitle = comic.title.toLowerCase().includes(lowerKw)
        const matchTag =
          matchTagEnabled && comic.tags?.some((t) => t.toLowerCase().includes(lowerKw))
        return matchTitle || matchTag
      })

      if (!allMatched) return false // 只要有一个词不满足，就过滤掉
    }

    // 🟢 关卡 2.5：语言过滤 (仅当语言选择非 All 且未禁用语言过滤时生效)
    if (langTag) {
      const hasLang = comic.tags?.some((t) => t.toLowerCase() === langTag)
      if (!hasLang) return false
    }

    // 关卡 3：分类匹配
    if (cfg.activeCategories && cfg.activeCategories.length > 0) {
      if (comic.category && !cfg.activeCategories.includes(comic.category)) {
        return false
      }
    }

    // 关卡 4：评分与页数范围过滤
    if (cfg.minRating !== undefined && (comic.rating || 0) < cfg.minRating) {
      return false
    }

    // 🎯 补全页数范围过滤：
    if (cfg.minPages !== undefined && cfg.minPages > 0 && (comic.pageCount || 0) < cfg.minPages) {
      return false
    }

    if (cfg.maxPages !== undefined && cfg.maxPages > 0 && (comic.pageCount || 0) > cfg.maxPages) {
      return false
    }

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

    <GridContainer
      :items="currentPageItems"
      :selectable="true"
      :select-mode="selectMode"
      :selected-ids="selectedIds"
      @longpress="handleLongPress"
      @select="handleSelect"
    >
      <!-- 通过 #footer 插槽挂载数字分页组件 -->
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
.offline-home-view {
  padding: 20px;
  min-height: 100%;
}

.select-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-bottom: 12px;
  margin-bottom: 16px;
  border-bottom: 1px solid #2a2a2a;
  position: sticky;
  top: 0;
  z-index: 10;
  background-color: #1a1a1e;
}

.select-count {
  color: #ffffff;
  font-size: 0.95rem;
  font-weight: 500;
}

.toolbar-btn {
  background-color: #26262a;
  color: #e0e0e0;
  border: 1px solid #3a3a3f;
  border-radius: 6px;
  padding: 6px 14px;
  font-size: 0.85rem;
  cursor: pointer;
  transition:
    background-color 0.2s,
    border-color 0.2s;
}

.toolbar-btn:hover:not(:disabled) {
  background-color: #333338;
  border-color: #4a4a4f;
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
