<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUI } from '@/composables/useUI'
import {
  bookshelves,
  pinnedBookshelves,
  PIN_LIMIT,
  addBookshelf,
  removeBookshelf,
  renameBookshelf,
  moveShelfToPosition,
  moveShelfToTop,
  setBookshelfPinned,
} from '@/stores/bookshelfStore'
import BookshelfPickerOverlay from '@/components/BookshelfPickerOverlay.vue'
import SortRowMenu from '@/components/SortRowMenu.vue'
// Round22：侧栏置顶书架拖拽排序（把手拖动 / 操作菜单）
import { useDragReorder } from '@/composables/useDragReorder'
import { useUserStore } from '@/stores/userStore'

const router = useRouter()
const route = useRoute() // 1. 引入 useRoute 用于精准匹配 query.id
const { modal, toast } = useUI()

// Round3-任务2：更新/维护入口仅管理员可见
const { isAdmin } = useUserStore()

// 控制书架菜单的展开/折叠状态
const isBookshelfOpen = ref(true)

// Round13：全部书架检索浮层
const showAllShelfPicker = ref(false)
const openShelfPicker = () => {
  showAllShelfPicker.value = true
}
// 置顶书架数量上限提示（已置顶时侧栏每行显示 📌 取消置顶）
const pinnedCount = computed(() => bookshelves.value.filter((b) => b.pinned).length)

const toggleBookshelf = () => {
  isBookshelfOpen.value = !isBookshelfOpen.value
}

// 新建书架
const createNewBookshelf = async () => {
  const name = await modal.prompt('请输入新书架名称', '', '创建书架')
  if (name && name.trim()) {
    addBookshelf(name.trim())
    toast.success(`书架「${name}」创建成功！`)
  }
}

// 重命名书架（Round10）
const renameShelf = async (shelf: { id: string; name: string }) => {
  const name = await modal.prompt('请输入书架新名称', shelf.name, '重命名书架')
  if (name && name.trim() && name.trim() !== shelf.name) {
    await renameBookshelf(shelf.id, name.trim())
    toast.success(`书架已重命名为「${name.trim()}」`)
  }
}

// 删除书架
const handleDeleteShelf = async (shelfId: string, shelfName: string) => {
  const confirmed = await modal.confirm(`确定要删除书架「${shelfName}」吗？`, '删除确认')
  if (confirmed) {
    removeBookshelf(shelfId)
    toast.info(`书架「${shelfName}」已删除`)

    // 2. 核心修正：只有当前正处于被删除的这个书架页面时，才跳回首页
    if (route.query.id === shelfId) {
      router.push('/offline/home')
    }
  }
}

// ─────────────────────────────────────────────────────────────
// Round22：置顶书架拖拽排序（决策 D1：把手为主；D6：菜单用「移到顶部」）
// 置顶组顺序 = 全局书架顺序的子序列；拖拽/菜单换算为全局位置后走 LexoRank 单点移动。
// ─────────────────────────────────────────────────────────────
const pinnedDrag = useDragReorder({
  getScrollContainer: () => document.querySelector('.sidebar'),
  rowSelector: '.sub-nav-item',
  getGhostText: (i) => pinnedBookshelves.value[i]?.name || '',
  onReorder: (from, to) => handlePinnedReorder(from, to),
})

// 解构 ref 供模板自动解包（模板内嵌套对象的 ref 不会自动解包）
const {
  dragging: pinnedDragging,
  dragIndex: pinnedDragIndex,
  indicatorIndex: pinnedIndicatorIndex,
  ghostTop: pinnedGhostTop,
  ghostHeight: pinnedGhostHeight,
  ghostLeft: pinnedGhostLeft,
  ghostWidth: pinnedGhostWidth,
  onHandlePointerDown: pinnedHandleDown,
  consumeSuppressClick: pinnedConsumeClick,
  ghostText: pinnedGhostText,
} = pinnedDrag

/** 拖拽落位（from/to 为置顶组下标；to 为移除被拖项后的插入下标）→ 换算全局位置 */
const handlePinnedReorder = (from: number, to: number) => {
  const pinned = pinnedBookshelves.value
  const moved = pinned[from]
  if (!moved || from === to) return
  const newPinned = [...pinned]
  newPinned.splice(from, 1)
  newPinned.splice(to, 0, moved)
  const removed = bookshelves.value.map((b) => b.id).filter((id) => id !== moved.id)
  const movedPos = newPinned.indexOf(moved)
  const next = movedPos < newPinned.length - 1 ? newPinned[movedPos + 1] : null
  const prev = movedPos > 0 ? newPinned[movedPos - 1] : null
  let target = -1
  if (next) target = removed.indexOf(next.id)
  else if (prev) target = removed.indexOf(prev.id) + 1
  else return
  if (target < 0) return
  void moveShelfToPosition(moved.id, target + 1)
}

/** 操作菜单：移到顶部（置顶组第 1 位） */
const handleShelfMoveTop = (shelfId: string) => {
  const from = pinnedBookshelves.value.findIndex((b) => b.id === shelfId)
  if (from < 0) return
  handlePinnedReorder(from, 0)
}

/** 操作菜单：移动到第 X 位（1-based，作用于置顶组） */
const handleShelfMoveTo = (shelfId: string, position: number) => {
  const pinned = pinnedBookshelves.value
  const from = pinned.findIndex((b) => b.id === shelfId)
  if (from < 0) return
  const to = Math.max(0, Math.min(position - 1, pinned.length - 1))
  handlePinnedReorder(from, to)
}

/** 拖动刚结束时抑制行内点击（避免拖拽落点触发跳转书架） */
const onLinkClick = (e: MouseEvent) => {
  if (pinnedConsumeClick()) {
    e.preventDefault()
    e.stopPropagation()
  }
}
</script>

<template>
  <div class="nav-group">
    <span class="group-title">📚 离线模式</span>
    <router-link to="/offline/home">首页</router-link>
    <!-- Round3-任务2：更新/维护入口仅管理员可见 -->
    <router-link v-if="isAdmin" to="/offline/update">更新</router-link>
    <router-link v-if="isAdmin" to="/offline/maintain">维护</router-link>
    <router-link to="/offline/toplist">排行榜</router-link>
    <router-link to="/offline/history">历史记录</router-link>

    <div class="foldable-item">
      <div class="foldable-header" @click="toggleBookshelf">
        <span>书架</span>
        <span class="arrow" :class="{ open: isBookshelfOpen }">❯</span>
      </div>

      <div v-show="isBookshelfOpen" class="foldable-body">
        <!-- Round13：仅常驻展示置顶书架（最多 PIN_LIMIT 个），其余进「全部书架」浮层 -->
        <div v-if="pinnedBookshelves.length === 0" class="pin-hint">
          💡 在「全部书架」中点击 ⭐ 置顶常用书架（最多 {{ PIN_LIMIT }} 个）
        </div>

        <template v-for="(shelf, idx) in pinnedBookshelves" :key="shelf.id">
          <router-link
            :to="`/offline/bookshelf?id=${shelf.id}`"
            class="sub-nav-item"
            :class="{ active: route.query.id === shelf.id }"
            @click="onLinkClick"
          >
            <span class="shelf-name"><span class="pin-dot">📌</span> {{ shelf.name }}</span>

            <div class="shelf-right-info">
              <span class="shelf-count">{{ shelf.count || 0 }}</span>

              <!-- Round22：拖拽把手（拖动排序；把手触摸不滚动列表） -->
              <span
                class="drag-handle"
                title="拖动排序"
                @pointerdown="(e) => pinnedHandleDown(e as PointerEvent, idx)"
                @click.stop.prevent
              >
                ⠿
              </span>

              <!-- Round22：排序操作菜单（移到顶部 / 移动到第 X 位） -->
              <SortRowMenu
                :total="pinnedBookshelves.length"
                @move-top="handleShelfMoveTop(shelf.id)"
                @move-to="(p) => handleShelfMoveTo(shelf.id, p)"
              />

              <!-- Round13：取消置顶 -->
              <span
                class="unpin-btn"
                title="取消置顶"
                @click.stop.prevent="setBookshelfPinned(shelf.id, false)"
              >
                📌
              </span>
            </div>
          </router-link>
          <!-- Round22：拖拽落位指示线 -->
          <div
            v-if="pinnedDragging && pinnedIndicatorIndex === idx"
            class="drop-line sidebar-drop-line"
          />
        </template>
        <!-- 拖到末尾的落位指示线 -->
        <div
          v-if="pinnedDragging && pinnedIndicatorIndex === pinnedBookshelves.length"
          class="drop-line sidebar-drop-line"
        />

        <!-- Round22：拖拽幽灵卡（fixed 跟随指针） -->
        <Teleport to="body">
          <div
            v-if="pinnedDragging"
            class="drag-ghost"
            :style="{
              top: pinnedGhostTop + 'px',
              left: pinnedGhostLeft + 'px',
              width: pinnedGhostWidth + 'px',
              height: pinnedGhostHeight + 'px',
            }"
          >
            📌 {{ pinnedGhostText(pinnedDragIndex) }}
          </div>
        </Teleport>

        <!-- Round13：全部书架检索浮层入口 -->
        <button class="all-shelf-btn" @click="openShelfPicker">
          🔍 全部书架（{{ bookshelves.length }}）
        </button>
        <button class="add-shelf-btn" @click="createNewBookshelf">➕ 新建书架</button>
      </div>

      <!-- Round13：全部书架检索浮层 -->
      <BookshelfPickerOverlay :open="showAllShelfPicker" mode="navigate" @close="showAllShelfPicker = false" />
    </div>
  </div>

  <!-- 🎲 工具：跨模式全局功能（骰子支持全库、清单有在线/离线双 tab） -->
  <div class="nav-group">
    <span class="group-title">🎲 工具</span>
    <router-link to="/random">手气不错</router-link>
    <router-link to="/reading-list">阅读清单</router-link>
    <!-- 画质升级：仅管理员可见（升级会发起下载消耗 GP，与更新/维护权限一致） -->
    <router-link v-if="isAdmin" to="/offline/upgrade">画质升级</router-link>
  </div>
</template>

<style scoped>
.foldable-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  color: var(--app-text-2);
  cursor: pointer;
  border-radius: 6px;
  font-size: 0.9rem;
  transition: all 0.2s;
}
.foldable-header:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

.arrow {
  font-size: 0.75rem;
  transition: transform 0.2s;
}
.arrow.open {
  transform: rotate(90deg);
}

.foldable-body {
  display: flex;
  flex-direction: column;
  padding-left: 12px;
  margin-top: 2px;
  gap: 2px;
}

.sub-nav-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 8px;
  border-radius: 4px;
  font-size: 0.85rem !important;
  color: var(--app-text-3) !important;
  transition: all 0.2s;
  text-decoration: none;
  background-color: transparent;
}

/* 1. 核心修复：彻底清空 Vue Router 默认下发给所有书架的全局大蓝块背景 */
.sub-nav-item.router-link-active,
.sub-nav-item.router-link-exact-active {
  background-color: transparent !important;
  color: var(--app-text-3) !important;
}

/* 2. 普通悬浮态 */
.sub-nav-item:hover {
  background-color: var(--app-surface-3) !important;
  color: var(--app-text-strong) !important;
}

/* 3. 只有当前 ID 100% 匹配时，我们手绑定的 .active 才独占高亮 */
.sub-nav-item.active {
  background-color: #007acc !important; /* 经典蓝色底块 */
  color: #ffffff !important; /* 白字 */
  font-weight: bold;
}

.shelf-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 130px;
}

.shelf-right-info {
  display: flex;
  align-items: center;
  gap: 4px;
}

/* Round22：侧栏拖拽落位指示线（整行宽） */
.sidebar-drop-line {
  margin-left: 8px;
  margin-right: 8px;
}

.shelf-count {
  font-size: 0.75rem;
  background-color: var(--app-surface-3);
  padding: 1px 6px;
  border-radius: 10px;
  color: var(--app-text-2);
}

.delete-btn,
.rename-btn {
  font-size: 0.75rem;
  color: var(--app-text-2);
  padding: 0 4px;
  border-radius: 3px;
  opacity: 0;
  transition:
    opacity 0.2s,
    color 0.2s;
}

.sub-nav-item:hover .delete-btn,
.sub-nav-item:hover .rename-btn {
  opacity: 1;
}

.delete-btn:hover {
  color: #ef4444 !important;
  background-color: rgba(239, 68, 68, 0.2);
}

.rename-btn:hover {
  color: #3d5afe !important;
  background-color: rgba(61, 90, 254, 0.15);
}

.pin-hint {
  font-size: 0.75rem;
  color: var(--app-text-muted);
  padding: 4px 8px;
  line-height: 1.5;
}

.pin-dot {
  font-size: 0.7rem;
  margin-right: 2px;
}

.all-shelf-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-3);
  padding: 6px 10px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.8rem;
  text-align: left;
  margin-top: 4px;
  transition: all 0.2s;
}
.all-shelf-btn:hover {
  border-color: #007acc;
  color: #007acc;
}

.unpin-btn {
  font-size: 0.75rem;
  color: var(--app-text-2);
  padding: 0 4px;
  border-radius: 3px;
  opacity: 0;
  transition:
    opacity 0.2s,
    color 0.2s;
}
.sub-nav-item:hover .unpin-btn {
  opacity: 1;
}
.unpin-btn:hover {
  color: #e6b800 !important;
  background-color: rgba(230, 184, 0, 0.15);
}

.add-shelf-btn {
  background: transparent;
  border: 1px dashed var(--app-border-3);
  color: var(--app-text-3);
  padding: 6px 10px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.8rem;
  text-align: left;
  margin-top: 4px;
  transition: all 0.2s;
}
.add-shelf-btn:hover {
  border-color: #007acc;
  color: #007acc;
}
</style>
