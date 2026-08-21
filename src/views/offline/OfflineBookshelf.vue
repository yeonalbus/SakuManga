<script setup lang="ts">
import { ref, computed, watch, onMounted, onActivated, nextTick } from 'vue'
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router'
// 🟢 1. 按领域引入：漫画数据源来自 comicStore，书架信息来自 bookshelfStore
import { offlineComics, fetchOfflineComics, deleteOfflineComics } from '@/stores/comicStore'
import {
  bookshelves,
  computedBookshelves,
  loadBookshelves,
  reorderShelfComics,
  renameBookshelf,
  removeBookshelf,
} from '@/stores/bookshelfStore'
import type { Bookshelf, OfflineComic, ComicItem } from '@/types/comic'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/Pagination.vue'
import { useUI } from '@/composables/useUI'
import { useUserStore } from '@/stores/userStore'
// 问题3：主滚动容器是 #main-content，翻页回顶必须用它而非 window
// 任务五：列表状态记忆（页码 + 滚动位置），返回时「从哪里来回哪里去」
import {
  scrollMainToTop,
  rememberListState,
  takeListState,
  setListStateProvider,
  getMainContent,
} from '@/utils/scrollMemory'

const { toast, modal } = useUI()
const userStore = useUserStore()

const route = useRoute()
const router = useRouter()

// 1. 获取当前路由中的书架 ID (?id=xxx)
const currentShelfId = computed(() => (route.query.id as string) || '')

// 🟢 2. 从 Store 中查找当前书架信息
// 说明：fallback 不再引用 shelfComics，避免与下方 computed 形成循环依赖
const currentShelf = computed<Bookshelf>(() => {
  return (
    computedBookshelves.value.find((s) => s.id === currentShelfId.value) || {
      id: currentShelfId.value || 'all',
      name: '全部离线作品',
      // 无匹配书架时 count 展示由 shelfComics.length 负责，这里仅补默认值保证类型完整
      count: 0,
    }
  )
})

// 🟢 3. 核心计算：根据当前书架 ID 动态过滤 Store 里的离线漫画
// Round10：展示顺序遵循书架 comicIds 的数组顺序（自定义排序的基础）；
// 未在 comicIds 中但 bookshelfId 匹配（历史遗留归属）的漫画排在末尾。
const shelfComics = computed<OfflineComic[]>(() => {
  if (!currentShelfId.value) {
    // 如果没有传 id 参数，默认展示全部离线漫画
    return offlineComics.value
  }
  const ids = currentShelf.value.comicIds || []
  const byId = new Map(offlineComics.value.map((c) => [c.id, c]))
  const ordered: OfflineComic[] = []
  const seen = new Set<string>()
  for (const id of ids) {
    const c = byId.get(id)
    if (c && !seen.has(id)) {
      ordered.push(c)
      seen.add(id)
    }
  }
  // bookshelfId 匹配但不在 comicIds 中的（旧数据/引用残留）排在末尾
  for (const c of offlineComics.value) {
    if (!seen.has(c.id) && c.bookshelfId === currentShelfId.value) {
      ordered.push(c)
      seen.add(c.id)
    }
  }
  return ordered
})

// 4. 分页逻辑
const currentPage = ref(1)
const pageSize = 24

// 切换书架时仅需将页码重置为 1
watch(currentShelfId, () => {
  currentPage.value = 1
})

const totalPages = computed(() => Math.ceil(shelfComics.value.length / pageSize) || 1)

const currentPageItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return shelfComics.value.slice(start, start + pageSize)
})

const handlePageChange = (page: number) => {
  currentPage.value = page
  // 问题3：真实滚动容器是 #main-content，window.scrollTo 无效
  scrollMainToTop('smooth')
}

// 任务五/S11：恢复上次离开的列表状态（页码 + 滚动位置）。
// onMounted 与 onActivated 共用：keep-alive 缓存下「同标签返回」只触发 onActivated。
const restoreListState = async () => {
  // Round10-Bug1：书架页直接刷新 / 从详情页新标签进入时，offlineComics 与
  // bookshelves store 可能尚未填充（fetchOfflineComics 只在首页等页面触发），
  // 若不在此拉取，书架将显示为空。onMounted 与 onActivated 共用本函数，两处一并修复。
  await fetchOfflineComics()
  if (bookshelves.value.length === 0) {
    await loadBookshelves()
  }
  const saved = takeListState('/offline/bookshelf')
  if (saved?.page && saved.page > 1) {
    currentPage.value = saved.page
  }
  if (saved && saved.top > 0) {
    await nextTick()
    requestAnimationFrame(() => {
      const el = getMainContent()
      if (el && el.scrollHeight > 0) el.scrollTop = saved.top
    })
  }
  // Round7-任务4：注册列表状态提供者（打开详情新标签前捕获 { top, page }）
  setListStateProvider('/offline/bookshelf', () => ({
    top: getMainContent()?.scrollTop || 0,
    page: currentPage.value,
  }))
}

onMounted(restoreListState)

// S11：keep-alive 缓存下重新激活书架页时同样恢复列表状态
let activatedOnce = false
onActivated(() => {
  if (activatedOnce) {
    restoreListState()
  }
  activatedOnce = true
})

// 任务五：离开列表页时保存「页码 + 滚动位置」
onBeforeRouteLeave(() => {
  rememberListState('/offline/bookshelf', {
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

// --------------------------------------------------
// Round10：书架内项目自定义排序（竖排排序视图）
// --------------------------------------------------
const sortMode = ref(false)
const sortIds = ref<string[]>([])
/** 排序视图用漫画查找表（避免模板内反复 find） */
const sortComicMap = computed(() => new Map(offlineComics.value.map((c) => [c.id, c])))

/** 进入排序模式：以当前书架 comicIds 顺序为初始序（未记录的 bookshelfId 归属项追加末尾） */
const enterSortMode = () => {
  if (!currentShelfId.value) {
    toast.info('「全部离线作品」视图不支持排序，请进入具体书架')
    return
  }
  const ids = currentShelf.value.comicIds || []
  const seen = new Set(ids)
  const extra = shelfComics.value.filter((c) => !seen.has(c.id)).map((c) => c.id)
  sortIds.value = [...ids, ...extra]
  sortMode.value = true
}

const moveSortItem = (index: number, dir: -1 | 1) => {
  const newIndex = index + dir
  if (newIndex < 0 || newIndex >= sortIds.value.length) return
  const arr = [...sortIds.value]
  const [item] = arr.splice(index, 1)
  arr.splice(newIndex, 0, item)
  sortIds.value = arr
}

const exitSortMode = async (save: boolean) => {
  if (save && currentShelfId.value) {
    // 过滤掉当前已不在离线库中的残留引用（按有效书架漫画重建完整顺序）
    const validIds = sortIds.value.filter((id) =>
      shelfComics.value.some((c) => c.id === id),
    )
    await reorderShelfComics(currentShelfId.value, validIds)
    toast.success('书架排序已保存')
  }
  sortMode.value = false
  sortIds.value = []
}

// Round10-Opt3：书架页 header 改名 / 删除当前书架
const handleRenameCurrent = async () => {
  if (!currentShelfId.value) return
  const name = await modal.prompt('请输入书架新名称', currentShelf.value.name, '重命名书架')
  if (name && name.trim() && name.trim() !== currentShelf.value.name) {
    await renameBookshelf(currentShelfId.value, name.trim())
    toast.success(`书架已重命名为「${name.trim()}」`)
  }
}

const handleDeleteCurrent = async () => {
  if (!currentShelfId.value) return
  const confirmed = await modal.confirm(
    `确定要删除书架「${currentShelf.value.name}」吗？`,
    '删除确认',
  )
  if (!confirmed) return
  await removeBookshelf(currentShelfId.value)
  toast.info(`书架「${currentShelf.value.name}」已删除`)
  router.replace('/offline/home')
}
</script>

<template>
  <div class="offline-bookshelf-view">
    <div class="shelf-header">
      <div class="title-area">
        <span class="icon">📁</span>
        <h2 class="shelf-title">{{ currentShelf.name }}</h2>
        <span class="shelf-badge">{{ shelfComics.length }} 部作品</span>
        <!-- Round10-Opt3：书架页 header 操作（排序/改名/删除，仅具体书架视图） -->
        <template v-if="currentShelfId">
          <button class="shelf-op-btn" title="自定义排序" @click="enterSortMode">↕ 排序</button>
          <button class="shelf-op-btn" title="重命名书架" @click="handleRenameCurrent">✎ 改名</button>
          <button class="shelf-op-btn danger" title="删除书架" @click="handleDeleteCurrent">
            🗑️ 删除
          </button>
        </template>
      </div>
    </div>

    <!-- Round10-Opt1b：书架内项目排序视图（竖排 ↑/↓） -->
    <div v-if="sortMode" class="sort-mode-panel">
      <div class="sort-toolbar">
        <span class="sort-hint">使用 ↑/↓ 调整顺序（共 {{ sortIds.length }} 本）</span>
        <div class="sort-actions">
          <button class="toolbar-btn" @click="exitSortMode(false)">取消</button>
          <button class="toolbar-btn primary" @click="exitSortMode(true)">✓ 完成排序</button>
        </div>
      </div>
      <div class="sort-mini-list">
        <div
          v-for="(cid, idx) in sortIds"
          :key="cid"
          class="sort-mini-card"
          :class="{ 'sort-current': true }"
        >
          <span class="sort-index">{{ idx + 1 }}</span>
          <div class="cover-box">
            <img
              :src="sortComicMap.get(cid)?.coverUrl || ''"
              :alt="sortComicMap.get(cid)?.title || cid"
              loading="lazy"
            />
          </div>
          <div class="info-box">
            <h4 class="sort-title" :title="sortComicMap.get(cid)?.title || cid">
              {{ sortComicMap.get(cid)?.title || cid }}
            </h4>
            <span class="sort-meta">{{ sortComicMap.get(cid)?.pageCount || 0 }} 页</span>
          </div>
          <div class="sort-move-btns">
            <button class="move-btn" :disabled="idx === 0" @click="moveSortItem(idx, -1)">↑</button>
            <button
              class="move-btn"
              :disabled="idx === sortIds.length - 1"
              @click="moveSortItem(idx, 1)"
            >
              ↓
            </button>
          </div>
        </div>
      </div>
    </div>

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

    <!-- 使用 #footer 插槽挂载页码组件（排序模式隐藏网格） -->
    <GridContainer
      v-if="!sortMode"
      :items="currentPageItems"
      :selectable="true"
      :select-mode="selectMode"
      :selected-ids="selectedIds"
      @longpress="handleLongPress"
      @select="handleSelect"
    >
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
.offline-bookshelf-view {
  padding: 12px 4px;
  min-height: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.shelf-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
}

.title-area {
  display: flex;
  align-items: center;
  gap: 10px;
}

.icon {
  font-size: 1.5rem;
}

.shelf-title {
  margin: 0;
  font-size: 1.3rem;
  font-weight: 600;
  color: var(--app-text-strong);
}

.shelf-badge {
  background-color: var(--app-surface-3);
  color: var(--app-text-2);
  font-size: 0.8rem;
  padding: 2px 8px;
  border-radius: 12px;
  margin-left: 8px;
}

.select-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
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

/* Round10：书架页 header 操作按钮 */
.shelf-op-btn {
  background-color: var(--app-border-2);
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 0.78rem;
  cursor: pointer;
  transition:
    background-color 0.2s,
    border-color 0.2s;
  white-space: nowrap;
}

.shelf-op-btn:hover {
  background-color: var(--app-surface-3-hover);
  border-color: var(--app-border-3);
  color: var(--app-text-strong);
}

.shelf-op-btn.danger {
  color: #ff7588;
  border-color: rgba(255, 117, 136, 0.4);
}

.shelf-op-btn.danger:hover {
  background-color: rgba(255, 117, 136, 0.12);
}

/* Round10：书架内项目排序视图 */
.sort-mode-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.sort-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--app-border-2);
}

.sort-hint {
  font-size: 0.82rem;
  color: var(--app-text-3);
}

.sort-actions {
  display: flex;
  gap: 8px;
}

.toolbar-btn.primary {
  background-color: #3d5afe;
  color: #ffffff;
  border-color: #3d5afe;
}

.toolbar-btn.primary:hover {
  background-color: #2f48e0;
  border-color: #2f48e0;
}

.sort-mini-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 60vh;
  overflow-y: auto;
  padding-right: 4px;
}

.sort-mini-card {
  display: flex;
  align-items: center;
  gap: 12px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 8px;
}

.sort-mini-card:hover {
  border-color: #007acc;
  background-color: var(--app-surface-3);
}

.sort-index {
  width: 26px;
  height: 26px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background-color: var(--app-surface-3);
  color: var(--app-text-2);
  font-size: 0.78rem;
  font-weight: 700;
}

.cover-box {
  width: 40px;
  height: 56px;
  flex-shrink: 0;
  border-radius: 4px;
  overflow: hidden;
  background-color: var(--app-border-2);
}

.cover-box img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.info-box {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.sort-title {
  margin: 0;
  font-size: 0.85rem;
  color: var(--app-text-strong);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sort-meta {
  font-size: 0.75rem;
  color: var(--app-text-3);
}

.sort-move-btns {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
}

.move-btn {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 4px;
  background-color: var(--app-surface-3);
  color: var(--app-text-2);
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.15s;
}

.move-btn:hover:not(:disabled) {
  background-color: #10b981;
  color: #ffffff;
}

.move-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

/* 📱 移动形态：排序/操作按钮常显 */
@media (max-width: 1024px) {
  .sort-move-btns {
    opacity: 1;
  }
}
</style>
