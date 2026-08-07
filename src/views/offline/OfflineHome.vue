<script setup lang="ts">
import { ref, computed, watch, onMounted, onActivated, nextTick } from 'vue'
import { useRoute, onBeforeRouteLeave } from 'vue-router'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/Pagination.vue'
// Round4 任务八：离线首页新增悬浮球，提供刷新 + 日期跳页入口
import FloatingToolbar from '@/components/FloatingToolbar.vue'
// 🟢 1. 正确引入离线数据源 (offlineComics) 与 离线专属搜索筛选配置 (offlineSearchConfig)
import { offlineComics, fetchOfflineComics, deleteOfflineComics } from '@/stores/comicStore'
import { offlineSearchConfig } from '@/stores/searchStore'
import { useUI } from '@/composables/useUI'
import { useUserStore } from '@/stores/userStore'
import type { ComicItem, OfflineComic } from '@/types/comic'
// Round3-任务6：负向排除（`- ` 前缀：负向 tag 精确匹配 / 负向关键词子串匹配）
import { matchExcludes, parseKeywordQueue } from '@/utils/tagFilter'
// 问题3：主滚动容器是 #main-content，翻页回顶必须用它而非 window
// 任务五：列表状态记忆（页码 + 滚动位置），返回时「从哪里来回哪里去」
import { scrollMainToTop, rememberListState, takeListState, getMainContent } from '@/utils/scrollMemory'

const { toast, modal } = useUI()
const userStore = useUserStore()

// 任务五：进入页面时恢复上次离开的列表状态（页码）；滚动位置在数据就绪后恢复
onMounted(async () => {
  const saved = takeListState('/offline/home')
  if (saved?.page && saved.page > 1) {
    currentPage.value = saved.page
  }
  await fetchOfflineComics()
  // 数据就绪（列表已渲染）后再恢复滚动位置，避免内容高度为 0 导致恢复失准
  if (saved && saved.top > 0) {
    await nextTick()
    requestAnimationFrame(() => {
      const el = getMainContent()
      if (el && el.scrollHeight > 0) el.scrollTop = saved.top
    })
  }
})

// 需求2：App.vue 用 keep-alive 缓存路由，重新进入书库页（相同 fullPath）不会触发 onMounted，
// 导致「下载新版本后删除旧版本」等后端变更不反映到前端。改为每次重新激活书库页时刷新离线数据源。
let activatedOnce = false
onActivated(() => {
  if (activatedOnce) {
    fetchOfflineComics()
  }
  activatedOnce = true
})

// 任务五：离开列表页时保存「页码 + 滚动位置」，返回时恢复
onBeforeRouteLeave(() => {
  rememberListState('/offline/home', {
    top: getMainContent()?.scrollTop || 0,
    page: currentPage.value,
  })
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

// 需求2：本地 tag 搜索——同时匹配翻译名(tags)与原始 tag 串(tagRaws)，二者在 store 中均已归一为 string[]
const comicTagStrings = (comic: ComicItem): string[] => {
  const out: string[] = []
  if (Array.isArray(comic.tags)) out.push(...(comic.tags as string[]))
  const raws = (comic as OfflineComic).tagRaws
  if (Array.isArray(raws)) out.push(...raws)
  return out.map((t) => t.toLowerCase()).filter(Boolean)
}

// 🟢 2. 核心过滤管道：兼顾 URL 中的 ?q= 搜索词 与 TopBar 传进来的离线筛选配置 (求交集)
const filteredComics = computed(() => {
  const cfg = offlineSearchConfig.value
  // ─── Round3-任务6：顶栏主搜索词同样支持「- 」负向前缀（拆出正向匹配 + 并入负向规则）───
  const searchBarParsed = parseKeywordQueue((cfg.keyword || '').trim() ? [cfg.keyword || ''] : [])
  const searchBarKw = searchBarParsed.positive.join(' ').toLowerCase().trim()

  // ─── Round3-任务6：把关键词队列按「- 」前缀拆分为正向 / 负向两部分 ───
  const parsedQueue = parseKeywordQueue(cfg.keywords)

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
        matchTagEnabled && comicTagStrings(comic).some((t) => t.includes(searchBarKw))
      if (!matchTitle && !matchTag) return false
    }

    // 🟢 关卡 2：筛选抽屉中的“多关键词队列”过滤 (必须同时匹配队列里的每一个正向词)
    // Round3-任务6：负向项（`- ` 前缀）不再参与正向匹配，交由末尾负向关卡剔除
    if (parsedQueue.positive.length > 0) {
      const allMatched = parsedQueue.positive.every((filterKw: string) => {
        const lowerKw = filterKw.toLowerCase()
        const matchTitle = comic.title.toLowerCase().includes(lowerKw)
        const matchTag = matchTagEnabled && comicTagStrings(comic).some((t) => t.includes(lowerKw))
        return matchTitle || matchTag
      })

      if (!allMatched) return false // 只要有一个正向词不满足，就过滤掉
    }

    // 🟢 关卡 2.5：语言过滤 (仅当语言选择非 All 且未禁用语言过滤时生效)
    if (langTag) {
      const hasLang = (comic as OfflineComic).tagRaws?.some((t) => t.toLowerCase() === langTag)
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

    // ─── Round3-任务6：负向关卡（负向 tag 精确匹配 + 负向关键词子串匹配）───
    const excludeRule = {
      excludeTags: [
        ...(cfg.excludeTags || []),
        ...parsedQueue.excludeTags,
        ...searchBarParsed.excludeTags,
      ],
      excludeKeywords: [
        ...(cfg.excludeKeywords || []),
        ...parsedQueue.excludeKeywords,
        ...searchBarParsed.excludeKeywords,
      ],
    }
    if (!matchExcludes(comic, excludeRule)) return false

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

// 🟢 4. 时间排序（问题1：SegmentedControl + 升降序图标，前端本地排序）
const sortOptions = [
  { value: 'addedAt', label: '入库时间' },
  { value: 'publishedAt', label: '发布时间' },
  { value: 'fileModifiedAt', label: '修改时间' },
  { value: 'updatedAt', label: '更新时间' },
] as const
type SortKey = (typeof sortOptions)[number]['value']
// Round3-任务4：默认按发布时间降序
const sortBy = ref<SortKey>('publishedAt')
const sortDesc = ref(true)

// 空时间字段始终排最后（无论升降序）
const sortedComics = computed(() => {
  const dir = sortDesc.value ? -1 : 1
  return [...filteredComics.value].sort((a, b) => {
    const ra = (a as unknown as Record<string, unknown>)[sortBy.value]
    const rb = (b as unknown as Record<string, unknown>)[sortBy.value]
    const av = ra ? new Date(ra as string).getTime() : Number.NEGATIVE_INFINITY
    const bv = rb ? new Date(rb as string).getTime() : Number.NEGATIVE_INFINITY
    if (!isFinite(av) && !isFinite(bv)) return 0
    if (!isFinite(av)) return 1
    if (!isFinite(bv)) return -1
    return (av - bv) * dir
  })
})

const currentPageItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return sortedComics.value.slice(start, start + pageSize)
})

const handlePageChange = (newPage: number) => {
  currentPage.value = newPage
  // 问题3：真实滚动容器是 #main-content，window.scrollTo 无效
  scrollMainToTop('smooth')
}

// Round4 任务八：按日期跳页——基于当前排序（默认发布时间降序）找到首个到达该日期的项并定位其所在页
const seekToDate = (date: string) => {
  const target = new Date(date).getTime()
  if (isNaN(target)) {
    toast.error('日期无效')
    return
  }
  const timeOf = (c: ComicItem) => {
    const raw = (c as unknown as Record<string, unknown>)[sortBy.value]
    return raw ? new Date(raw as string).getTime() : Number.NEGATIVE_INFINITY
  }
  const list = sortedComics.value
  let idx = -1
  if (sortDesc.value) {
    // 降序（新→旧）：首个 时间<=目标 的项即该日期的起点
    idx = list.findIndex((c) => {
      const t = timeOf(c)
      return isFinite(t) && t <= target
    })
  } else {
    // 升序（旧→新）：最后一个 时间<=目标 的项
    for (let i = list.length - 1; i >= 0; i--) {
      const t = timeOf(list[i])
      if (isFinite(t) && t <= target) {
        idx = i
        break
      }
    }
  }
  if (idx < 0) {
    toast.info('未找到该日期之前的作品')
    return
  }
  currentPage.value = Math.floor(idx / pageSize) + 1
  scrollMainToTop('smooth')
}
</script>

<template>
  <div class="offline-home-view">
    <!-- 选择模式工具条 -->
    <div v-if="selectMode" class="select-toolbar">
      <span class="select-count">已选 {{ selectedIds.length }} 部</span>
      <button class="toolbar-btn" @click="toggleSelectAllPage">全选本页</button>
      <button
        v-if="userStore.isAdmin"
        class="toolbar-btn danger"
        :disabled="selectedIds.length === 0"
        @click="handleDeleteSelected"
      >
        🗑️ 删除
      </button>
      <button class="toolbar-btn" @click="exitSelectMode">取消</button>
    </div>

    <!-- 时间排序控件（问题1） -->
    <div class="sort-controls">
      <div class="segmented-control">
        <button
          v-for="opt in sortOptions"
          :key="opt.value"
          class="seg-option"
          :class="{ active: sortBy === opt.value }"
          @click="((sortBy = opt.value), (currentPage = 1))"
        >
          {{ opt.label }}
        </button>
      </div>
      <button
        class="sort-direction"
        :title="sortDesc ? '切换为升序' : '切换为降序'"
        @click="((sortDesc = !sortDesc), (currentPage = 1))"
      >
        {{ sortDesc ? '↓' : '↑' }}
      </button>
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

    <!-- Round4 任务八：悬浮球（刷新 + 日期跳页） -->
    <FloatingToolbar @refresh="fetchOfflineComics" @seek-change="seekToDate" />
  </div>
</template>

<style scoped>
.offline-home-view {
  padding: 12px 4px;
  min-height: 100%;
}

.select-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-bottom: 12px;
  margin-bottom: 16px;
  border-bottom: 1px solid var(--app-border-2);
  position: sticky;
  top: 0;
  z-index: 10;
  background-color: var(--app-surface-2);
}

.select-count {
  color: var(--app-text-strong);
  font-size: 0.95rem;
  font-weight: 500;
}

.toolbar-btn {
  background-color: var(--app-border-2);
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 6px 14px;
  font-size: 0.85rem;
  cursor: pointer;
  transition:
    background-color 0.2s,
    border-color 0.2s;
}

.toolbar-btn:hover:not(:disabled) {
  background-color: var(--app-surface-3-hover);
  border-color: var(--app-border-3);
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

/* 时间排序控件（问题1） */
.sort-controls {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.segmented-control {
  display: inline-flex;
  background-color: var(--app-border-2);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  overflow: hidden;
}

.seg-option {
  background: transparent;
  border: none;
  color: var(--app-text-3);
  padding: 6px 16px;
  font-size: 0.85rem;
  cursor: pointer;
  transition:
    background-color 0.2s,
    color 0.2s;
}

.seg-option:hover {
  color: var(--app-text-2);
}

.seg-option.active {
  background-color: #3d5afe;
  color: #ffffff;
}

.sort-direction {
  background-color: var(--app-border-2);
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  padding: 6px 14px;
  font-size: 0.95rem;
  cursor: pointer;
  transition: background-color 0.2s;
}

.sort-direction:hover {
  background-color: var(--app-surface-3-hover);
}
</style>
