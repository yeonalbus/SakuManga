<script setup lang="ts">
/**
 * Round38：书架墙（/offline/bookshelves）—— 未读仪表盘 + 抽卡台
 *
 * 背景：书架此前只有「整理」没有「消费」——22 个书架里 114 本收藏有 76 本从未打开，
 * 而侧栏只常驻置顶的 2 个，其余 20 个埋在「全部书架」浮层里且进去只能翻列表。
 * 本页把「未读债务」摆上台面，并给每个书架一个「🎲 抽一本未读」的直接消费入口。
 *
 * 数据：未读数与封面由 GET /bookshelves 聚合返回（后端一次 IN 查询）；
 * 排序沿用 Round22 LexoRank（排序模式下拖拽，单点移动 + 防抖持久化）。
 */
import { computed, onActivated, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { fetchOfflineComics, offlineComics } from '@/stores/comicStore'
import {
  bookshelves,
  PIN_LIMIT,
  addBookshelf,
  renameBookshelf,
  removeBookshelf,
  setBookshelfPinned,
  moveShelfToPosition,
  flushPendingSort,
  refreshBookshelves,
} from '@/stores/bookshelfStore'
import { shelfUnreadCount, shelfWallSummary } from '@/composables/useShelfPick'
import ShelfPickOverlay from '@/components/ShelfPickOverlay.vue'
import { useDragReorder } from '@/composables/useDragReorder'
import { getMainContent } from '@/utils/scrollMemory'
import type { Bookshelf } from '@/types/comic'

const router = useRouter()
const route = useRoute()
const { modal, toast } = useUI()

// ─────────────────────────────────────────────────────────────
// 展示状态：排序方式 / 搜索 / 未读过滤
// ─────────────────────────────────────────────────────────────
type SortMode = 'custom' | 'unread' | 'count' | 'name'

const sortMode = ref<SortMode>('custom')
const keyword = ref('')
/** 统计条点「未读 N 本」后的过滤：只留有未读的书架（清理债务的视角） */
const unreadOnly = ref(false)

/** 统计条数据（收藏本数去重、未读去重，失效引用不计） */
const summary = computed(() => shelfWallSummary.value)

const filtered = computed<Bookshelf[]>(() => {
  let list = [...bookshelves.value]
  if (unreadOnly.value) list = list.filter((s) => shelfUnreadCount(s) > 0)
  const kw = keyword.value.trim().toLowerCase()
  if (kw) list = list.filter((s) => s.name.toLowerCase().includes(kw))
  if (sortMode.value === 'unread') {
    list.sort((a, b) => shelfUnreadCount(b) - shelfUnreadCount(a) || a.name.localeCompare(b.name))
  } else if (sortMode.value === 'count') {
    list.sort((a, b) => (b.count || 0) - (a.count || 0) || a.name.localeCompare(b.name))
  } else if (sortMode.value === 'name') {
    list.sort((a, b) => a.name.localeCompare(b.name))
  }
  // custom：沿用 store 顺序（后端 sort_key 升序；本地移动后同步重排，不再二次排序）
  return list
})

/** 未读徽标文案：0 未读 → ✓ 已清空 */
const unreadLabel = (shelf: Bookshelf): string => {
  const unread = shelfUnreadCount(shelf)
  if (unread === 0) return '✓ 已清空'
  return `未读 ${unread}/${shelf.count || 0}`
}

const toggleUnreadOnly = () => {
  unreadOnly.value = !unreadOnly.value
}

// ─────────────────────────────────────────────────────────────
// 数据加载：进入/激活时刷新统计（阅读器读完回来未读数要减少）
// ─────────────────────────────────────────────────────────────
const loadStats = async () => {
  if (offlineComics.value.length === 0) await fetchOfflineComics()
  await refreshBookshelves()
}

// 侧栏「📖 未读 N 本」入口带 ?unread=1 进入时直接落到未读过滤视图
watch(
  () => route.query.unread,
  (v) => {
    if (v === '1') unreadOnly.value = true
  },
  { immediate: true },
)

onMounted(loadStats)

let activatedOnce = false
onActivated(() => {
  if (activatedOnce) void loadStats()
  activatedOnce = true
})

onBeforeRouteLeave(() => {
  flushPendingSort()
})
onBeforeUnmount(() => {
  flushPendingSort()
})

// ─────────────────────────────────────────────────────────────
// 书架操作：进入 / 新建 / 重命名 / 删除 / 置顶
// ─────────────────────────────────────────────────────────────
const enterShelf = (shelfId: string) => {
  router.push(`/offline/bookshelf?id=${shelfId}`)
}

const handleCreate = async () => {
  const name = await modal.prompt('请输入新书架名称', '', '创建书架')
  if (name && name.trim()) {
    await addBookshelf(name.trim())
    toast.success(`书架「${name.trim()}」创建成功！`)
  }
}

const handleRename = async (shelf: Bookshelf) => {
  const name = await modal.prompt('请输入书架新名称', shelf.name, '重命名书架')
  if (name && name.trim() && name.trim() !== shelf.name) {
    await renameBookshelf(shelf.id, name.trim())
    toast.success(`书架已重命名为「${name.trim()}」`)
  }
}

const handleDelete = async (shelf: Bookshelf) => {
  const confirmed = await modal.confirm(`确定要删除书架「${shelf.name}」吗？`, '删除确认')
  if (!confirmed) return
  await removeBookshelf(shelf.id)
  toast.info(`书架「${shelf.name}」已删除`)
}

const togglePin = async (shelf: Bookshelf) => {
  if (!shelf.pinned) {
    const pinnedCount = bookshelves.value.filter((b) => b.pinned).length
    if (pinnedCount >= PIN_LIMIT) {
      toast.warning(`侧栏最多置顶 ${PIN_LIMIT} 个书架，请先取消其他置顶`)
      return
    }
  }
  await setBookshelfPinned(shelf.id, !shelf.pinned)
  toast.success(shelf.pinned ? '已置顶到侧栏' : '已取消置顶')
}

// ─────────────────────────────────────────────────────────────
// 抽一本未读（结果卡浮层）
// ─────────────────────────────────────────────────────────────
const pickOpen = ref(false)
const pickShelfId = ref('')
const pickShelfName = ref('')

const openPick = (shelf?: Bookshelf) => {
  pickShelfId.value = shelf?.id || ''
  pickShelfName.value = shelf?.name || ''
  pickOpen.value = true
}

// ─────────────────────────────────────────────────────────────
// 排序模式（单列紧凑行拖拽，与书架页 Round22 排序视图同构）
// ─────────────────────────────────────────────────────────────
const sortPanel = ref(false)
/** 排序模式下的书架 id 顺序（本地即时反馈，落位后走 LexoRank 单点持久化） */
const sortIds = ref<string[]>([])

const sortRows = computed(() =>
  sortIds.value
    .map((id) => bookshelves.value.find((s) => s.id === id))
    .filter((s): s is Bookshelf => !!s),
)

const wallDrag = useDragReorder({
  getScrollContainer: () => getMainContent(),
  rowSelector: '.wall-sort-row',
  getGhostText: (i) => bookshelves.value.find((s) => s.id === sortIds.value[i])?.name || '',
  onReorder: (from, to) => {
    const arr = [...sortIds.value]
    const [moved] = arr.splice(from, 1)
    if (!moved) return
    arr.splice(to, 0, moved)
    sortIds.value = arr
    void moveShelfToPosition(moved, to + 1)
  },
})

// 解构 ref 供模板自动解包（嵌套对象的 ref 不会自动解包）
const {
  dragging: sortDragging,
  dragIndex: sortDragIndex,
  indicatorIndex: sortIndicatorIndex,
  ghostTop: sortGhostTop,
  ghostHeight: sortGhostHeight,
  ghostLeft: sortGhostLeft,
  ghostWidth: sortGhostWidth,
  onHandlePointerDown: sortHandleDown,
  ghostText: sortGhostText,
} = wallDrag

const enterSortPanel = () => {
  sortIds.value = bookshelves.value.map((s) => s.id)
  sortPanel.value = true
}

const exitSortPanel = () => {
  sortPanel.value = false
  flushPendingSort()
  toast.success('书架顺序已保存')
}

/** 封面缺失 / 缓存未生成时收起图片，露出占位块 */
const onCoverError = (e: Event) => {
  const img = e.target as HTMLImageElement | null
  if (img) img.style.display = 'none'
}
</script>

<template>
  <div class="shelf-wall-view">
    <div class="wall-header">
      <div class="title-area">
        <span class="icon">📚</span>
        <h2 class="wall-title">书架墙</h2>
        <span class="wall-badge">{{ summary.shelfCount }} 个书架</span>
      </div>
      <div class="header-actions">
        <button class="wall-op-btn" :class="{ active: sortPanel }" @click="sortPanel ? exitSortPanel() : enterSortPanel()">
          ↕ {{ sortPanel ? '完成排序' : '排序' }}
        </button>
        <button class="wall-op-btn" @click="handleCreate">➕ 新建书架</button>
      </div>
    </div>

    <!-- 统计条：把「收藏了没读」的债务摆到台面上 -->
    <div class="stat-bar">
      <span class="stat-item">收藏 <b>{{ summary.collected }}</b> 本</span>
      <span class="stat-sep">·</span>
      <button
        class="stat-item stat-unread"
        :class="{ active: unreadOnly, cleared: summary.unread === 0 }"
        :title="unreadOnly ? '显示全部书架' : '只显示有未读的书架'"
        @click="toggleUnreadOnly"
      >
        <template v-if="summary.unread > 0">未读 <b>{{ summary.unread }}</b> 本</template>
        <template v-else>全部读完 ✓</template>
      </button>
      <span v-if="unreadOnly" class="stat-hint">（已过滤出有未读的书架）</span>
    </div>

    <template v-if="!sortPanel">
      <div class="wall-toolbar">
        <input v-model="keyword" class="wall-search" type="text" placeholder="🔎 搜索书架名称…" />
        <select v-model="sortMode" class="wall-select">
          <option value="custom">自定义顺序</option>
          <option value="unread">未读最多</option>
          <option value="count">本数最多</option>
          <option value="name">名称</option>
        </select>
      </div>

      <div v-if="filtered.length === 0" class="wall-empty">
        {{
          keyword
            ? '无匹配书架'
            : unreadOnly
              ? '🎉 没有待清理的书架，全部读完了'
              : '暂无书架，点击右上「新建书架」创建'
        }}
      </div>

      <div v-else class="wall-grid">
        <div
          v-for="shelf in filtered"
          :key="shelf.id"
          class="wall-card"
          :class="{ pinned: shelf.pinned, cleared: shelfUnreadCount(shelf) === 0 }"
        >
          <button class="card-main" :title="shelf.name" @click="enterShelf(shelf.id)">
            <div class="card-cover">
              <img
                v-if="shelf.coverUrl"
                :src="shelf.coverUrl"
                :alt="shelf.name"
                loading="lazy"
                @error="onCoverError"
              />
              <span class="cover-fallback">{{ shelf.name.slice(0, 1) }}</span>
              <span v-if="shelf.pinned" class="pin-flag">📌</span>
              <span class="unread-badge" :class="{ cleared: shelfUnreadCount(shelf) === 0 }">
                {{ unreadLabel(shelf) }}
              </span>
            </div>
            <div class="card-info">
              <span class="card-name">{{ shelf.name }}</span>
              <span class="card-count">{{ shelf.count || 0 }} 本</span>
            </div>
          </button>

          <div class="card-actions">
            <button
              class="pick-btn"
              :disabled="shelfUnreadCount(shelf) === 0"
              :title="
                shelfUnreadCount(shelf) === 0
                  ? '该系列已全部读过'
                  : `从「${shelf.name}」随机抽一本没读过的`
              "
              @click="openPick(shelf)"
            >
              🎲 抽一本未读
            </button>
          </div>

          <div class="card-tools">
            <button
              class="tool-btn"
              :title="shelf.pinned ? '取消置顶' : '置顶到侧栏'"
              @click="togglePin(shelf)"
            >
              📌
            </button>
            <button class="tool-btn" title="重命名" @click="handleRename(shelf)">✎</button>
            <button class="tool-btn danger" title="删除" @click="handleDelete(shelf)">✕</button>
          </div>
        </div>
      </div>
    </template>

    <!-- 排序模式：单列紧凑行（复用 Round22 拖拽原语） -->
    <div v-else class="wall-sort-panel">
      <div class="sort-toolbar">
        <span class="sort-hint">拖动把手调整书架顺序（共 {{ sortRows.length }} 个）</span>
        <button class="wall-op-btn" @click="exitSortPanel">✓ 完成排序</button>
      </div>
      <div class="sort-list">
        <template v-for="(shelf, idx) in sortRows" :key="shelf.id">
          <div class="wall-sort-row">
            <span class="sort-index">{{ idx + 1 }}</span>
            <div class="sort-cover">
              <img
                v-if="shelf.coverUrl"
                :src="shelf.coverUrl"
                :alt="shelf.name"
                loading="lazy"
                @error="onCoverError"
              />
              <span class="cover-fallback">{{ shelf.name.slice(0, 1) }}</span>
            </div>
            <div class="sort-info">
              <span class="sort-name" :title="shelf.name">{{ shelf.name }}</span>
              <span class="sort-meta">{{ unreadLabel(shelf) }} · {{ shelf.count || 0 }} 本</span>
            </div>
            <span
              class="drag-handle"
              title="拖动排序"
              @pointerdown="(e) => sortHandleDown(e as PointerEvent, idx)"
              @click.stop.prevent
            >
              ⠿
            </span>
          </div>
          <div v-if="sortDragging && sortIndicatorIndex === idx" class="drop-line" />
        </template>
        <div v-if="sortDragging && sortIndicatorIndex === sortRows.length" class="drop-line" />
      </div>

      <!-- 拖拽幽灵卡（fixed 跟随指针） -->
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
          📚 {{ sortGhostText(sortDragIndex) }}
        </div>
      </Teleport>
    </div>

    <!-- 抽一本未读：结果卡（shelfId 为空 = 全部书架） -->
    <ShelfPickOverlay
      :open="pickOpen"
      :shelf-id="pickShelfId"
      :shelf-name="pickShelfName"
      @close="pickOpen = false"
    />
  </div>
</template>

<style scoped>
.shelf-wall-view {
  padding: 12px 4px;
  min-height: 100%;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.wall-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
}

.title-area {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.icon {
  font-size: 1.5rem;
}

.wall-title {
  margin: 0;
  font-size: 1.3rem;
  font-weight: 600;
  color: var(--app-text-strong);
}

.wall-badge {
  background-color: var(--app-surface-3);
  color: var(--app-text-2);
  font-size: 0.8rem;
  padding: 2px 8px;
  border-radius: 12px;
  white-space: nowrap;
}

.header-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.wall-op-btn {
  background-color: var(--app-border-2);
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 5px 11px;
  font-size: 0.78rem;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}

.wall-op-btn:hover {
  background-color: var(--app-surface-3-hover);
  color: var(--app-text-strong);
}

.wall-op-btn.active {
  border-color: #007acc;
  color: #007acc;
}

/* ── 统计条：未读债务可视化 ── */
.stat-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 0.85rem;
  color: var(--app-text-2);
}

.stat-item {
  color: var(--app-text-2);
}

.stat-item b {
  color: var(--app-text-strong);
  font-size: 0.95rem;
}

.stat-sep {
  color: var(--app-text-3);
}

.stat-unread {
  background: rgba(255, 200, 80, 0.1);
  border: 1px solid rgba(255, 200, 80, 0.45);
  color: #e6b800;
  border-radius: 12px;
  padding: 3px 12px;
  cursor: pointer;
  font-size: 0.83rem;
  transition: all 0.15s;
}

.stat-unread b {
  color: #e6b800;
}

.stat-unread:hover {
  background: rgba(255, 200, 80, 0.2);
}

.stat-unread.active {
  background: rgba(255, 200, 80, 0.28);
  border-color: #e6b800;
}

.stat-unread.cleared {
  background: rgba(80, 200, 120, 0.1);
  border-color: rgba(80, 200, 120, 0.4);
  color: #4cc38a;
}

.stat-unread.cleared b {
  color: #4cc38a;
}

.stat-hint {
  font-size: 0.76rem;
  color: var(--app-text-3);
}

/* ── 工具栏 ── */
.wall-toolbar {
  display: flex;
  gap: 10px;
  align-items: center;
}

.wall-search {
  flex: 1;
  min-width: 0;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid var(--app-border-3);
  background: var(--app-input-bg);
  color: var(--app-fg);
  font-size: 0.86rem;
  outline: none;
}

.wall-search:focus {
  border-color: #007acc;
}

.wall-select {
  padding: 8px 10px;
  border-radius: 8px;
  border: 1px solid var(--app-border-3);
  background: var(--app-input-bg);
  color: var(--app-fg);
  font-size: 0.84rem;
  cursor: pointer;
  flex-shrink: 0;
}

.wall-empty {
  text-align: center;
  color: var(--app-text-3);
  font-size: 0.86rem;
  padding: 40px 0;
}

/* ── 书架卡片网格 ── */
.wall-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(158px, 1fr));
  gap: 14px;
}

.wall-card {
  position: relative;
  display: flex;
  flex-direction: column;
  background: var(--app-bg-deep);
  border: 1px solid var(--app-border-2);
  border-radius: 10px;
  overflow: hidden;
  transition:
    border-color 0.15s,
    transform 0.15s;
}

.wall-card:hover {
  border-color: var(--app-border-3);
  transform: translateY(-2px);
}

.wall-card.pinned {
  border-color: rgba(255, 200, 80, 0.45);
}

.wall-card.cleared {
  opacity: 0.72;
}

.card-main {
  display: flex;
  flex-direction: column;
  background: transparent;
  border: none;
  padding: 0;
  cursor: pointer;
  text-align: left;
  color: inherit;
}

.card-cover {
  position: relative;
  width: 100%;
  aspect-ratio: 3 / 4;
  background: linear-gradient(145deg, var(--app-surface-3), var(--app-bg-deep));
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.card-cover img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.cover-fallback {
  font-size: 2.4rem;
  color: var(--app-text-3);
  opacity: 0.45;
  user-select: none;
}

.pin-flag {
  position: absolute;
  top: 6px;
  left: 6px;
  font-size: 0.8rem;
  filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.7));
}

.unread-badge {
  position: absolute;
  right: 6px;
  bottom: 6px;
  font-size: 0.72rem;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.72);
  color: #e6b800;
  border: 1px solid rgba(255, 200, 80, 0.5);
  backdrop-filter: blur(2px);
}

.unread-badge.cleared {
  color: #4cc38a;
  border-color: rgba(80, 200, 120, 0.5);
}

.card-info {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 6px;
  padding: 8px 10px 4px;
}

.card-name {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--app-text-strong);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.card-count {
  font-size: 0.72rem;
  color: var(--app-text-3);
  flex-shrink: 0;
}

.card-actions {
  padding: 6px 10px 10px;
}

.pick-btn {
  width: 100%;
  background: transparent;
  border: 1px dashed var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 6px;
  padding: 6px 8px;
  font-size: 0.78rem;
  cursor: pointer;
  transition: all 0.15s;
}

.pick-btn:hover:not(:disabled) {
  border-color: #007acc;
  color: #007acc;
}

.pick-btn:disabled {
  opacity: 0.4;
  cursor: default;
}

.card-tools {
  position: absolute;
  top: 6px;
  right: 6px;
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.15s;
}

.wall-card:hover .card-tools {
  opacity: 1;
}

.tool-btn {
  background: rgba(0, 0, 0, 0.62);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 5px;
  font-size: 0.72rem;
  padding: 2px 6px;
  cursor: pointer;
  backdrop-filter: blur(2px);
}

.tool-btn:hover {
  color: var(--app-text-strong);
  border-color: #007acc;
}

.tool-btn.danger:hover {
  color: #ff7588;
  border-color: #ff7588;
}

/* 触摸设备无 hover：操作按钮常驻，否则无法置顶/改名/删除 */
@media (hover: none) {
  .card-tools {
    opacity: 1;
  }
}

/* ── 排序模式（单列紧凑行） ── */
.wall-sort-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
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

.sort-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.wall-sort-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  background: var(--app-bg-deep);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
}

.sort-index {
  width: 26px;
  text-align: center;
  font-size: 0.8rem;
  color: var(--app-text-3);
  flex-shrink: 0;
}

.sort-cover {
  position: relative;
  width: 38px;
  height: 50px;
  border-radius: 4px;
  overflow: hidden;
  background: linear-gradient(145deg, var(--app-surface-3), var(--app-bg-deep));
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.sort-cover img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.sort-cover .cover-fallback {
  font-size: 1.1rem;
}

.sort-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.sort-name {
  font-size: 0.86rem;
  font-weight: 600;
  color: var(--app-text-strong);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sort-meta {
  font-size: 0.74rem;
  color: var(--app-text-3);
}
</style>
