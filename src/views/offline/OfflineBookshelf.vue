<script setup lang="ts">
import { ref, computed, watch, onMounted, onActivated, nextTick, onBeforeUnmount } from 'vue'
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router'
// 🟢 1. 按领域引入：漫画数据源来自 comicStore，书架信息来自 bookshelfStore
import { offlineComics, fetchOfflineComics, deleteOfflineComics } from '@/stores/comicStore'
import {
  bookshelves,
  computedBookshelves,
  loadBookshelves,
  orderedShelfComicIds,
  ensureShelfComicWeights,
  moveComicToPosition,
  reorderShelfComics,
  renameBookshelf,
  removeBookshelf,
  removeComicsFromShelf,
  flushPendingSort,
} from '@/stores/bookshelfStore'
import BookshelfPickerOverlay from '@/components/BookshelfPickerOverlay.vue'
import ShelfQuickAddToolbar from '@/components/ShelfQuickAddToolbar.vue'
import SortRowMenu from '@/components/SortRowMenu.vue'
// Round38：抽一本未读（书架消费入口）
import ShelfPickOverlay from '@/components/ShelfPickOverlay.vue'
import { shelfUnreadCount } from '@/composables/useShelfPick'
import type { Bookshelf, OfflineComic, ComicItem } from '@/types/comic'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/Pagination.vue'
import { useUI } from '@/composables/useUI'
// Round22：多选快捷加入共享逻辑 + 拖拽排序原语
import { useShelfQuickAdd } from '@/composables/useShelfQuickAdd'
import { useDragReorder } from '@/composables/useDragReorder'
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
// Round22：展示顺序 = LexoRank 权值顺序（orderedShelfComicIds；本地乐观更新后保持一致）；
// 未在 comicIds 中但 bookshelfId 匹配（历史遗留归属）的漫画排在末尾。
const shelfComics = computed<OfflineComic[]>(() => {
  if (!currentShelfId.value) {
    // 如果没有传 id 参数，默认展示全部离线漫画
    return offlineComics.value
  }
  const ids = orderedShelfComicIds(currentShelf.value)
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
  // Round22：离开前冲刷未持久化的排序改动
  flushPendingSort()
})

onBeforeUnmount(() => {
  flushPendingSort()
})

// --------------------------------------------------
// Round22：长按选择 / 快捷加入 / 快捷移除（共享 composable）
// --------------------------------------------------
const quickAdd = useShelfQuickAdd(() => currentPageItems.value as unknown as ComicItem[])

const handleDeleteSelected = async () => {
  const ids = [...quickAdd.selectedIds.value]
  if (ids.length === 0) return
  const confirmed = await modal.confirm(
    `确定要删除选中的 ${ids.length} 部作品吗？\n将同时移除书架与历史记录中的引用。`,
    '删除选中作品',
  )
  if (!confirmed) return
  const alsoDeleteFile = await modal.confirm(
    '是否同时删除本地文件？\n选择「确定」将永久删除磁盘上的漫画文件，无法恢复。',
    '删除本地文件',
  )
  const okCount = await deleteOfflineComics(ids, alsoDeleteFile)
  if (okCount > 0) {
    toast.success(
      alsoDeleteFile ? `已删除 ${okCount} 部作品及其本地文件` : `已删除 ${okCount} 部作品`,
    )
  } else {
    toast.error('删除失败，请重试')
  }
  quickAdd.exitSelectMode()
}

// Round22：书架内多选「快捷移除」（决策 D9：弹确认框；不删本地文件/历史）
const handleRemoveSelected = async () => {
  const ids = [...quickAdd.selectedIds.value]
  if (ids.length === 0 || !currentShelfId.value) return
  const confirmed = await modal.confirm(
    `确定将这 ${ids.length} 部作品移出书架「${currentShelf.value.name}」吗？\n（不会删除本地文件与历史记录）`,
    '移出书架',
  )
  if (!confirmed) return
  await removeComicsFromShelf(currentShelfId.value, ids)
  toast.success(`已从书架移出 ${ids.length} 部作品`)
  quickAdd.exitSelectMode()
}

// --------------------------------------------------
// Round22：书架内项目自定义排序（拖拽 + 操作菜单，取代 Round10 的 ↑/↓）
// 展示顺序 = LexoRank 权值顺序；每次拖拽只更新一本的权值（防抖持久化）。
// --------------------------------------------------
const sortMode = ref(false)
/** 排序视图顺序（computed 跟随 store 乐观更新） */
const sortIds = computed<string[]>(() => {
  if (!currentShelfId.value) return []
  const ids = orderedShelfComicIds(currentShelf.value)
  const seen = new Set(ids)
  const extra = shelfComics.value.filter((c) => !seen.has(c.id)).map((c) => c.id)
  return [...ids, ...extra]
})
/** 排序视图用漫画查找表（避免模板内反复 find） */
const sortComicMap = computed(() => new Map(offlineComics.value.map((c) => [c.id, c])))
/** 进入排序时快照（取消时全量还原用） */
const enteredSortIds = ref<string[]>([])

/** 进入排序模式：先惰性迁移权值（旧数据赋 1000*i），再进入 */
const enterSortMode = async () => {
  if (!currentShelfId.value) {
    toast.info('「全部离线作品」视图不支持排序，请进入具体书架')
    return
  }
  await ensureShelfComicWeights(currentShelfId.value)
  enteredSortIds.value = [...sortIds.value]
  sortMode.value = true
}

/** 拖拽落位（to 为移除被拖项后的插入下标）→ 本地顺序跟随 store 乐观更新 + 单点权值持久化 */
const sortDrag = useDragReorder({
  getScrollContainer: () => document.querySelector('.sort-mini-list'),
  rowSelector: '.sort-mini-card',
  // 决策 D1：把手为主 + 行内长按 300ms 补充（长按期间位移视为滚动不触发）
  longPressMs: 300,
  getGhostText: (i) => sortComicMap.value.get(sortIds.value[i])?.title || sortIds.value[i] || '',
  onReorder: (from, to) => {
    if (!currentShelfId.value) return
    const movedId = sortIds.value[from]
    if (!movedId) return
    const target = Math.max(0, Math.min(to, sortIds.value.length - 1))
    void moveComicToPosition(currentShelfId.value, movedId, target + 1)
  },
})

// 解构 ref 供模板自动解包（模板内嵌套对象的 ref 不会自动解包）
const {
  dragging: sortDragging,
  dragIndex: sortDragIndex,
  indicatorIndex: sortIndicatorIndex,
  ghostTop: sortGhostTop,
  ghostHeight: sortGhostHeight,
  ghostLeft: sortGhostLeft,
  ghostWidth: sortGhostWidth,
  onHandlePointerDown: sortHandleDown,
  onRowPointerDown: sortRowDown,
  ghostText: sortGhostText,
} = sortDrag

/** 排序菜单：置顶（移到第 1 位） */
const handleSortMoveTop = (idx: number) => {
  if (!currentShelfId.value || idx <= 0) return
  const movedId = sortIds.value[idx]
  if (!movedId) return
  void moveComicToPosition(currentShelfId.value, movedId, 1)
}

/** 排序菜单：移动到第 X 位（1-based） */
const handleSortMoveTo = (idx: number, position: number) => {
  if (!currentShelfId.value) return
  const movedId = sortIds.value[idx]
  if (!movedId) return
  const target = Math.max(1, Math.min(position, sortIds.value.length))
  void moveComicToPosition(currentShelfId.value, movedId, target)
}

/** 退出排序：完成=冲刷增量改动；取消=冲刷后全量还原进入时顺序 */
const exitSortMode = async (save: boolean) => {
  if (!save && currentShelfId.value && enteredSortIds.value.length > 0) {
    await flushPendingSort()
    await reorderShelfComics(currentShelfId.value, [...enteredSortIds.value])
  } else if (save) {
    flushPendingSort()
    toast.success('书架排序已保存')
  }
  sortMode.value = false
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

// --------------------------------------------------
// Round38：抽一本未读（书架即「系列菜单」，进去就能直接开读）
// --------------------------------------------------
const pickOpen = ref(false)

/** 当前书架未读数（后端聚合值优先；为 0 时按钮置灰） */
const currentUnread = computed(() => shelfUnreadCount(currentShelf.value))
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
          <!-- Round38：抽一本未读（消费入口，未读清零后置灰） -->
          <button
            class="shelf-op-btn pick-accent"
            :disabled="currentUnread === 0"
            :title="
              currentUnread === 0
                ? '该系列已全部读过'
                : `从「${currentShelf.name}」随机抽一本没读过的`
            "
            @click="pickOpen = true"
          >
            🎲 抽一本未读
            <span v-if="currentUnread > 0" class="unread-chip">{{ currentUnread }}</span>
          </button>
          <button class="shelf-op-btn" title="自定义排序" @click="enterSortMode">↕ 排序</button>
          <button class="shelf-op-btn" title="重命名书架" @click="handleRenameCurrent">✎ 改名</button>
          <button class="shelf-op-btn danger" title="删除书架" @click="handleDeleteCurrent">
            🗑️ 删除
          </button>
        </template>
      </div>
    </div>

    <!-- Round22：书架内项目排序视图（拖拽把手 + 操作菜单，取代 Round10 的 ↑/↓） -->
    <div v-if="sortMode" class="sort-mode-panel">
      <div class="sort-toolbar">
        <span class="sort-hint">
          拖动把手排序（长按行 300ms 亦可），拖动中列表仍可滚动（共 {{ sortIds.length }} 本）
        </span>
        <div class="sort-actions">
          <button class="shelf-op-btn" @click="exitSortMode(false)">取消</button>
          <button class="shelf-op-btn" @click="exitSortMode(true)">✓ 完成排序</button>
        </div>
      </div>
      <div class="sort-mini-list">
        <template v-for="(cid, idx) in sortIds" :key="cid">
          <!-- 决策 D1：行内长按 300ms 亦可拖拽（长按期间位移视为滚动） -->
          <div
            class="sort-mini-card"
            :class="{ 'sort-current': true }"
            @pointerdown="(e) => sortRowDown(e as PointerEvent, idx)"
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
              <!-- Round22：拖拽把手（把手触摸不滚动列表） -->
              <span
                class="drag-handle"
                title="拖动排序"
                @pointerdown="(e) => sortHandleDown(e as PointerEvent, idx)"
                @click.stop.prevent
              >
                ⠿
              </span>
              <!-- Round22：排序操作菜单（置顶 / 移动到第 X 位；决策 D6：书架内本子用「置顶」） -->
              <SortRowMenu
                :total="sortIds.length"
                top-label="置顶"
                @move-top="handleSortMoveTop(idx)"
                @move-to="(p) => handleSortMoveTo(idx, p)"
              />
            </div>
          </div>
          <!-- Round22：拖拽落位指示线 -->
          <div
            v-if="sortDragging && sortIndicatorIndex === idx"
            class="drop-line sort-drop-line"
          />
        </template>
        <!-- 拖到末尾的落位指示线 -->
        <div
          v-if="sortDragging && sortIndicatorIndex === sortIds.length"
          class="drop-line sort-drop-line"
        />

        <!-- Round22：拖拽幽灵卡（fixed 跟随指针） -->
        <Teleport to="body">
          <div
            v-if="sortDragging"
            class="drag-ghost"
            :style="{
              top: sortGhostTop + 'px',
              left: sortGhostLeft + 'px',
              width: sortGhostWidth + 'px',
              height: sortGhostHeight + 'px',
            }"
          >
            {{ sortGhostText(sortDragIndex) }}
          </div>
        </Teleport>
      </div>
    </div>

    <!-- Round22：多选快捷加入/移除工具条（共享组件） -->
    <ShelfQuickAddToolbar
      v-if="quickAdd.selectMode.value"
      :count="quickAdd.selectedIds.value.length"
      :show-remove="!!currentShelfId"
      :show-delete="userStore.isAdmin"
      @select-all="quickAdd.toggleSelectAllPage()"
      @add="quickAdd.openShelfPicker()"
      @remove="handleRemoveSelected"
      @delete="handleDeleteSelected"
      @close="quickAdd.exitSelectMode()"
    />

    <!-- 使用 #footer 插槽挂载页码组件（排序模式隐藏网格） -->
    <GridContainer
      v-if="!sortMode"
      :items="currentPageItems"
      :selectable="true"
      :select-mode="quickAdd.selectMode.value"
      :selected-ids="quickAdd.selectedIds.value"
      @longpress="quickAdd.handleLongPress"
      @select="quickAdd.handleSelect"
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

    <!-- Round13/22：多选快捷加入书架（检索浮层 add 模式） -->
    <BookshelfPickerOverlay
      :open="quickAdd.showShelfPicker.value"
      mode="add"
      :selected-count="quickAdd.selectedIds.value.length"
      @close="quickAdd.showShelfPicker.value = false"
      @add="quickAdd.handleAddToShelf"
    />

    <!-- Round38：抽一本未读（结果卡；shelfId 为空 = 全部书架） -->
    <ShelfPickOverlay
      :open="pickOpen"
      :shelf-id="currentShelfId"
      :shelf-name="currentShelf.name"
      @close="pickOpen = false"
    />
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

/* Round38：抽一本未读（书架消费入口，与普通操作按钮区分主次） */
.shelf-op-btn.pick-accent {
  border-color: rgba(255, 200, 80, 0.5);
  color: #e6b800;
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.shelf-op-btn.pick-accent:hover:not(:disabled) {
  background-color: rgba(255, 200, 80, 0.15);
  border-color: #e6b800;
  color: #e6b800;
}

.shelf-op-btn.pick-accent:disabled {
  opacity: 0.4;
  cursor: default;
}

.unread-chip {
  font-size: 0.72rem;
  background: rgba(255, 200, 80, 0.2);
  border-radius: 8px;
  padding: 0 6px;
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
  /* 长按拖拽时防止文本选中 */
  user-select: none;
  -webkit-user-select: none;
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
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

/* Round22：排序视图拖拽落位指示线（行内边距） */
.sort-drop-line {
  margin-left: 6px;
  margin-right: 6px;
}
</style>
