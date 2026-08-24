<script setup lang="ts">
// Round13：书架检索浮层（侧栏「全部书架」入口 + 离线多选「加入书架」共用）
// - mode="navigate"：点击书架 → 跳转书架页（侧栏用）
// - mode="add"：点击书架 → 批量加入选中的离线漫画（多选工具条用）
import { ref, computed, watch } from "vue"
import { useRouter } from "vue-router"
import { useUI } from "@/composables/useUI"
import {
  bookshelves,
  computedBookshelves,
  PIN_LIMIT,
  addBookshelf,
  setBookshelfPinned,
  renameBookshelf,
  removeBookshelf,
  moveShelfToPosition,
  moveShelfToTop,
} from "@/stores/bookshelfStore"
// Round22：全部书架浮层拖拽排序（把手拖动 / 操作菜单；作用于全局书架顺序）
import { useDragReorder } from "@/composables/useDragReorder"
import SortRowMenu from "@/components/SortRowMenu.vue"

const props = withDefaults(
  defineProps<{
    open: boolean
    /** navigate=进入书架页；add=把 selectedCount 本加入所选书架 */
    mode?: "navigate" | "add"
    /** add 模式：已选数量展示 */
    selectedCount?: number
  }>(),
  { mode: "navigate", selectedCount: 0 },
)

const emit = defineEmits<{
  (e: "close"): void
  (e: "add", shelfId: string): void
}>()

const router = useRouter()
const { modal, toast } = useUI()

const keyword = ref("")
const busyId = ref("") // 正在处理的书架 id（防连点）

// 全部书架（含新建后刷新）：优先后端实时 count
const allShelves = computed(() => computedBookshelves.value)

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return allShelves.value
  return allShelves.value.filter((s) => s.name.toLowerCase().includes(kw))
})

const pinnedCount = computed(() => bookshelves.value.filter((b) => b.pinned).length)

watch(
  () => props.open,
  (v) => { if (v) keyword.value = "" },
)

const close = () => emit("close")

// navigate：点击进入书架页
const handleEnter = (shelfId: string) => {
  close()
  router.push(`/offline/bookshelf?id=${shelfId}`)
}

// add：点击把已选作品加入该书架
const handleAdd = async (shelfId: string) => {
  if (busyId.value) return
  busyId.value = shelfId
  emit("add", shelfId)
  busyId.value = ""
}

// 置顶 / 取消置顶（上限 PIN_LIMIT）
const togglePin = async (shelfId: string, currently: boolean) => {
  if (busyId.value) return
  if (!currently && pinnedCount.value >= PIN_LIMIT) {
    toast.warning(`侧栏最多置顶 ${PIN_LIMIT} 个书架，请先取消其他置顶`)
    return
  }
  busyId.value = shelfId
  await setBookshelfPinned(shelfId, !currently)
  busyId.value = ""
  toast.success(currently ? "已取消置顶" : `已置顶（侧栏常驻 ${Math.min(pinnedCount.value + (currently ? 0 : 1), PIN_LIMIT)}/${PIN_LIMIT}）`)
}

const handleRename = async (shelf: { id: string; name: string }) => {
  const name = await modal.prompt("请输入书架新名称", shelf.name, "重命名书架")
  if (name && name.trim() && name.trim() !== shelf.name) {
    await renameBookshelf(shelf.id, name.trim())
    toast.success(`书架已重命名为「${name.trim()}」`)
  }
}

const handleDelete = async (shelfId: string, shelfName: string) => {
  const confirmed = await modal.confirm(`确定要删除书架「${shelfName}」吗？`, "删除确认")
  if (confirmed) {
    await removeBookshelf(shelfId)
    toast.info(`书架「${shelfName}」已删除`)
  }
}

const handleCreate = async () => {
  const name = await modal.prompt("请输入新书架名称", "", "创建书架")
  if (name && name.trim()) {
    await addBookshelf(name.trim())
    toast.success(`书架「${name.trim()}」创建成功！`)
  }
}

// ─────────────────────────────────────────────────────────────
// Round22：全部书架拖拽排序（把手拖动 / 移到顶部 / 移动到第 X 位，作用于全局顺序）
// ─────────────────────────────────────────────────────────────
const pickerDrag = useDragReorder({
  getScrollContainer: () => document.querySelector(".picker-list"),
  rowSelector: ".picker-item",
  getGhostText: (i) => filtered.value[i]?.name || "",
  onReorder: (from, to) => {
    const moved = filtered.value[from]
    if (!moved || from === to) return
    void moveShelfToPosition(moved.id, to + 1)
  },
})

// 解构 ref 供模板自动解包（模板内嵌套对象的 ref 不会自动解包）
const {
  dragging: pickerDragging,
  dragIndex: pickerDragIndex,
  indicatorIndex: pickerIndicatorIndex,
  ghostTop: pickerGhostTop,
  ghostHeight: pickerGhostHeight,
  ghostLeft: pickerGhostLeft,
  ghostWidth: pickerGhostWidth,
  onHandlePointerDown: pickerHandleDown,
  ghostText: pickerGhostText,
} = pickerDrag

/** 操作菜单：移到顶部（全局第 1 位） */
const handleShelfMoveTop = (shelfId: string) => {
  void moveShelfToTop(shelfId)
}

/** 操作菜单：移动到第 X 位（1-based，全局书架顺序） */
const handleShelfMoveTo = (shelfId: string, position: number) => {
  void moveShelfToPosition(shelfId, position)
}
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="open" class="picker-mask" @click.self="close">
        <div class="picker-panel">
          <div class="picker-header">
            <h3 class="picker-title">
              {{ mode === "add" ? `📥 加入书架（已选 ${selectedCount} 部）` : "🔍 全部书架" }}
            </h3>
            <button class="picker-close" @click="close">✕</button>
          </div>

          <!-- 搜索框 -->
          <input v-model="keyword" class="picker-search" type="text" placeholder="🔎 搜索书架名称…" />

          <div class="picker-list">
            <div v-if="filtered.length === 0" class="picker-empty">
              {{ keyword ? "无匹配书架" : "暂无书架，点击下方「新建书架」创建" }}
            </div>

            <template v-for="(shelf, idx) in filtered" :key="shelf.id">
              <div class="picker-item" :class="{ pinned: shelf.pinned }">
                <button class="item-main" @click="mode === 'add' ? handleAdd(shelf.id) : handleEnter(shelf.id)">
                  <span class="pin-badge" :class="{ on: shelf.pinned }">📌</span>
                  <span class="shelf-name" :title="shelf.name">{{ shelf.name }}</span>
                  <span class="shelf-count">{{ shelf.count || 0 }}</span>
                </button>

                <div class="item-actions">
                  <button class="mini-btn pin" :title="shelf.pinned ? '取消置顶' : '置顶到侧栏'" @click="togglePin(shelf.id, !!shelf.pinned)">
                    {{ shelf.pinned ? "取消置顶" : "置顶" }}
                  </button>
                  <!-- Round22：拖拽把手（拖动排序；把手触摸不滚动列表） -->
                  <span
                    class="drag-handle"
                    title="拖动排序"
                    @pointerdown="(e) => pickerHandleDown(e as PointerEvent, idx)"
                    @click.stop.prevent
                  >
                    ⠿
                  </span>
                  <!-- Round22：排序操作菜单（移到顶部 / 移动到第 X 位） -->
                  <SortRowMenu
                    :total="filtered.length"
                    @move-top="handleShelfMoveTop(shelf.id)"
                    @move-to="(p) => handleShelfMoveTo(shelf.id, p)"
                  />
                  <button class="mini-btn rename" title="重命名" @click="handleRename(shelf)">✎</button>
                  <button class="mini-btn danger" title="删除" @click="handleDelete(shelf.id, shelf.name)">✕</button>
                </div>
              </div>
              <!-- Round22：拖拽落位指示线 -->
              <div
                v-if="pickerDragging && pickerIndicatorIndex === idx"
                class="drop-line picker-drop-line"
              />
            </template>
            <!-- 拖到末尾的落位指示线 -->
            <div
              v-if="pickerDragging && pickerIndicatorIndex === filtered.length"
              class="drop-line picker-drop-line"
            />

            <!-- Round22：拖拽幽灵卡（fixed 跟随指针） -->
            <Teleport to="body">
              <div
                v-if="pickerDragging"
                class="drag-ghost"
                :style="{
                  top: pickerGhostTop + 'px',
                  left: pickerGhostLeft + 'px',
                  width: pickerGhostWidth + 'px',
                  height: pickerGhostHeight + 'px',
                }"
              >
                📚 {{ pickerGhostText(pickerDragIndex) }}
              </div>
            </Teleport>
          </div>

          <div class="picker-footer">
            <button class="create-btn" @click="handleCreate">➕ 新建书架</button>
            <button class="close-btn" @click="close">关闭</button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.picker-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(2px);
}
.picker-panel {
  width: 440px;
  max-width: 92vw;
  max-height: 78vh;
  display: flex;
  flex-direction: column;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 12px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.6);
  overflow: hidden;
}
.picker-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border-2);
}
.picker-title {
  font-size: 0.98rem;
  font-weight: 600;
  color: var(--app-text-strong);
  margin: 0;
}
.picker-close {
  background: transparent;
  border: none;
  color: var(--app-text-3);
  font-size: 1rem;
  cursor: pointer;
  padding: 2px 6px;
}
.picker-close:hover { color: var(--app-text-strong); }
.picker-search {
  margin: 12px 16px 4px;
  padding: 9px 12px;
  border-radius: 8px;
  border: 1px solid var(--app-border-3);
  background: var(--app-input-bg);
  color: var(--app-fg);
  font-size: 0.88rem;
  outline: none;
}
.picker-search:focus { border-color: #007acc; }
.picker-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px 16px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.picker-empty {
  text-align: center;
  color: var(--app-text-3);
  font-size: 0.85rem;
  padding: 24px 0;
}
.picker-item {
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--app-bg-deep);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 8px 10px;
  transition: border-color 0.15s;
}
.picker-item.pinned {
  border-color: rgba(255, 200, 80, 0.45);
  background: rgba(255, 200, 80, 0.06);
}
.item-main {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  background: transparent;
  border: none;
  color: var(--app-text-2);
  font-size: 0.88rem;
  cursor: pointer;
  text-align: left;
  min-width: 0;
  padding: 2px 0;
}
.item-main:hover .shelf-name { color: var(--app-text-strong); }
.pin-badge { font-size: 0.8rem; opacity: 0.35; }
.pin-badge.on { opacity: 1; }
.shelf-name {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.shelf-count {
  font-size: 0.75rem;
  background: var(--app-surface-3);
  padding: 1px 8px;
  border-radius: 10px;
  color: var(--app-text-2);
  flex-shrink: 0;
}
.item-actions {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
}
.mini-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  font-size: 0.72rem;
  padding: 2px 7px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.15s;
}
.mini-btn.pin { color: #e6b800; border-color: rgba(230, 184, 0, 0.4); }
.mini-btn.pin:hover { background: rgba(230, 184, 0, 0.15); }
.mini-btn.rename:hover { color: #3d5afe; border-color: #3d5afe; }
.mini-btn.danger:hover { color: #ef4444; border-color: #ef4444; }
/* Round22：浮层拖拽落位指示线（行内边距） */
.picker-drop-line {
  margin-left: 10px;
  margin-right: 10px;
}
.picker-footer {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border-2);
}
.create-btn {
  background: transparent;
  border: 1px dashed var(--app-border-3);
  color: var(--app-text-3);
  padding: 7px 14px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.82rem;
  transition: all 0.15s;
}
.create-btn:hover { border-color: #007acc; color: #007acc; }
.close-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 7px 18px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.82rem;
}
.close-btn:hover { color: var(--app-text-strong); }
.fade-enter-active, .fade-leave-active { transition: opacity 0.18s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
