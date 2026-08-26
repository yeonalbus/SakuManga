<script setup lang="ts">
/**
 * ReaderSidebar —— 阅读器侧栏抽屉（Round24）
 *
 * 三视图 Tab：缩略图网格 / 章节大纲（≤3 级树）/ 书签列表。
 * 仅本地阅读器使用；数据（章节/书签）由父组件从后端获取并传入。
 * 交互：跳页、书签增删、章节添加（名称+层级+父节点）/删除。
 */
import { ref, computed } from 'vue'

export interface SidebarChapter {
  id: number
  parentId: number
  level: number
  title: string
  pageIndex: number // 起始物理页（0-based）
  orderNo: number
}
export interface SidebarPage {
  physical: number // 物理索引（0-based）
  url: string
}

type Tab = 'thumbs' | 'outline' | 'bookmarks'

const props = defineProps<{
  open: boolean
  activeTab: Tab
  pages: SidebarPage[] // 可见页序列（物理索引 + raw-page URL）
  currentPhysical: number
  physicalTotal: number
  chapters: SidebarChapter[]
  bookmarks: number[]
}>()

const emit = defineEmits<{
  (e: 'update:activeTab', v: Tab): void
  (e: 'close'): void
  (e: 'jump', physical: number): void
  (e: 'toggleBookmark', physical: number): void
  (e: 'addChapter', payload: { title: string; level: number; parentId: number; pageIndex: number }): void
  (e: 'removeChapter', id: number): void
}>()

// ---------------------------------------------------------------
// 当前物理页的标记状态
// ---------------------------------------------------------------
const currentIsBookmarked = computed(() => props.bookmarks.includes(props.currentPhysical))
const chaptersOfCurrent = computed(() =>
  props.chapters.filter((c) => c.pageIndex === props.currentPhysical),
)

// ---------------------------------------------------------------
// 章节树（flat → 树渲染）
// ---------------------------------------------------------------
const chapterChildren = (parentId: number) =>
  props.chapters.filter((c) => c.parentId === parentId)
const rootChapters = computed(() => chapterChildren(0))

/** 当前物理页所属章节路径（起始页 ≤ 当前物理页 且最靠后，同页取最深） */
const chapterPath = computed<SidebarChapter[]>(() => {
  let leaf: SidebarChapter | null = null
  for (const c of props.chapters) {
    if (c.pageIndex > props.currentPhysical) continue
    if (!leaf) leaf = c
    else if (c.pageIndex > leaf.pageIndex || (c.pageIndex === leaf.pageIndex && c.level > leaf.level))
      leaf = c
  }
  if (!leaf) return []
  const path: SidebarChapter[] = []
  let cur: SidebarChapter | null = leaf
  while (cur) {
    path.unshift(cur)
    cur = props.chapters.find((c) => c.id === cur!.parentId) ?? null
  }
  return path
})
const chapterPathText = computed(() =>
  chapterPath.value.length ? chapterPath.value.map((c) => c.title).join(' › ') : '—',
)
const isPathOf = (c: SidebarChapter, depth: number) => chapterPath.value[depth]?.id === c.id

// ---------------------------------------------------------------
// 添加章节表单（内联）
// ---------------------------------------------------------------
const showAddForm = ref(false)
const addTitle = ref('')
const addLevel = ref<1 | 2 | 3>(1)
const addParentId = ref(0)
/** 可挂子节点的候选父节点（level < 3 的现存节点） */
const parentCandidates = computed(() =>
  props.chapters.filter((c) => c.level < 3).map((c) => ({ id: c.id, label: c.title })),
)
const submitAddChapter = () => {
  const title = addTitle.value.trim()
  if (!title) return
  emit('addChapter', {
    title,
    level: addLevel.value,
    parentId: addParentId.value,
    pageIndex: props.currentPhysical,
  })
  addTitle.value = ''
  addParentId.value = 0
  showAddForm.value = false
}

// ---------------------------------------------------------------
// 跳页 / 标记
// ---------------------------------------------------------------
const jumpAndClose = (physical: number) => {
  emit('jump', physical)
  emit('close')
}
</script>

<template>
  <Transition name="proto-fade">
    <div v-if="open" class="rs-mask" @click="emit('close')"></div>
  </Transition>
  <Transition name="proto-slide">
    <aside v-if="open" class="rs-drawer" @click.stop>
      <div class="rs-tabs">
        <button
          class="rs-tab"
          :class="{ active: activeTab === 'thumbs' }"
          @click="emit('update:activeTab', 'thumbs')"
        >
          ▦ 缩略图
        </button>
        <button
          class="rs-tab"
          :class="{ active: activeTab === 'outline' }"
          @click="emit('update:activeTab', 'outline')"
        >
          𖧕 大纲
        </button>
        <button
          class="rs-tab"
          :class="{ active: activeTab === 'bookmarks' }"
          @click="emit('update:activeTab', 'bookmarks')"
        >
          ★ 书签<template v-if="bookmarks.length"> ({{ bookmarks.length }})</template>
        </button>
      </div>

      <div class="rs-current">
        <span class="rs-cur-label">当前物理页 P{{ currentPhysical + 1 }}</span>
        <span v-if="chapterPath.length" class="rs-cur-path" :title="chapterPathText">{{ chapterPathText }}</span>
        <div class="rs-cur-actions">
          <button
            class="rs-btn"
            :class="{ on: currentIsBookmarked }"
            @click="emit('toggleBookmark', currentPhysical)"
          >
            {{ currentIsBookmarked ? '★ 已书签' : '☆ 书签' }}
          </button>
          <button class="rs-btn" @click="showAddForm = !showAddForm">＋ 章节</button>
        </div>
      </div>

      <!-- 添加章节内联表单 -->
      <div v-if="showAddForm" class="rs-form">
        <input v-model="addTitle" class="rs-input" placeholder="章节名称" @keyup.enter="submitAddChapter" />
        <div class="rs-form-row">
          <label class="rs-fld">
            层级
            <select v-model.number="addLevel" class="rs-select">
              <option :value="1">一级</option>
              <option :value="2">二级</option>
              <option :value="3">三级</option>
            </select>
          </label>
          <label class="rs-fld">
            父级
            <select v-model.number="addParentId" class="rs-select">
              <option :value="0">（无，作为根）</option>
              <option v-for="p in parentCandidates" :key="p.id" :value="p.id">{{ p.label }}</option>
            </select>
          </label>
        </div>
        <div class="rs-form-actions">
          <button class="rs-btn ok" :disabled="!addTitle.trim()" @click="submitAddChapter">确定</button>
          <button class="rs-btn" @click="showAddForm = false">取消</button>
        </div>
      </div>

      <div class="rs-body">
        <!-- ▦ 缩略图网格 -->
        <div v-if="activeTab === 'thumbs'" class="rs-grid">
          <div
            v-for="p in pages"
            :key="p.physical"
            class="rs-thumb"
            :class="{ current: p.physical === currentPhysical }"
            :title="`P${p.physical + 1}`"
            @click="jumpAndClose(p.physical)"
          >
            <img :src="p.url" :alt="`P${p.physical + 1}`" loading="lazy" />
            <span class="rs-tnum">{{ p.physical + 1 }}</span>
            <span v-if="bookmarks.includes(p.physical)" class="rs-badge bm">★</span>
            <span v-if="chapters.some((c) => c.pageIndex === p.physical)" class="rs-badge ch">📑</span>
          </div>
          <div v-if="pages.length === 0" class="rs-empty">暂无可见页面</div>
        </div>

        <!-- 𖧕 章节大纲 -->
        <div v-else-if="activeTab === 'outline'" class="rs-outline">
          <div v-if="rootChapters.length === 0" class="rs-empty">
            暂无章节标记<br />
            <span class="rs-empty-sub">在当前页点「＋ 章节」添加</span>
          </div>
          <div v-else>
            <div v-for="c1 in rootChapters" :key="c1.id" class="rs-node lv1">
              <div
                class="rs-node-row"
                :class="{ current: isPathOf(c1, 0) }"
                @click="jumpAndClose(c1.pageIndex)"
              >
                <span class="rs-node-name">{{ c1.title }}</span>
                <span class="rs-node-page">P{{ c1.pageIndex + 1 }}</span>
                <button class="rs-del" title="删除章节（连同子级）" @click.stop="emit('removeChapter', c1.id)">✕</button>
              </div>
              <div v-for="c2 in chapterChildren(c1.id)" :key="c2.id" class="rs-node lv2">
                <div
                  class="rs-node-row"
                  :class="{ current: isPathOf(c2, 1) }"
                  @click="jumpAndClose(c2.pageIndex)"
                >
                  <span class="rs-node-name">{{ c2.title }}</span>
                  <span class="rs-node-page">P{{ c2.pageIndex + 1 }}</span>
                  <button class="rs-del" title="删除章节（连同子级）" @click.stop="emit('removeChapter', c2.id)">✕</button>
                </div>
                <div
                  v-for="c3 in chapterChildren(c2.id)"
                  :key="c3.id"
                  class="rs-node lv3"
                >
                  <div
                    class="rs-node-row"
                    :class="{ current: isPathOf(c3, 2) }"
                    @click="jumpAndClose(c3.pageIndex)"
                  >
                    <span class="rs-node-name">{{ c3.title }}</span>
                    <span class="rs-node-page">P{{ c3.pageIndex + 1 }}</span>
                    <button class="rs-del" title="删除章节（连同子级）" @click.stop="emit('removeChapter', c3.id)">✕</button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- ★ 书签 -->
        <div v-else class="rs-bookmarks">
          <div v-if="bookmarks.length === 0" class="rs-empty">
            暂无书签<br />
            <span class="rs-empty-sub">当前页点「☆ 书签」快速收藏</span>
          </div>
          <div v-else class="rs-bm-list">
            <button
              v-for="b in bookmarks"
              :key="b"
              class="rs-bm-item"
              :class="{ current: b === currentPhysical }"
              @click="jumpAndClose(b)"
            >
              <span class="rs-bm-star">★</span>
              <span class="rs-bm-label">书签</span>
              <span class="rs-bm-page">P{{ b + 1 }}</span>
              <span class="rs-del" title="移除书签" @click.stop="emit('toggleBookmark', b)">✕</span>
            </button>
          </div>
        </div>
      </div>
    </aside>
  </Transition>
</template>

<style scoped>
.rs-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: 4190;
}
.rs-drawer {
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  width: min(360px, 86vw);
  background: var(--app-surface-2, #1a1a1e);
  border-right: 1px solid var(--app-border-2);
  z-index: 4200;
  display: flex;
  flex-direction: column;
  box-shadow: 4px 0 24px rgba(0, 0, 0, 0.4);
  color: var(--app-text-strong);
}
.proto-slide-enter-active,
.proto-slide-leave-active {
  transition: transform 0.22s ease;
}
.proto-slide-enter-from,
.proto-slide-leave-to {
  transform: translateX(-100%);
}
.proto-fade-enter-active,
.proto-fade-leave-active {
  transition: opacity 0.2s ease;
}
.proto-fade-enter-from,
.proto-fade-leave-to {
  opacity: 0;
}

.rs-tabs {
  display: flex;
  border-bottom: 1px solid var(--app-border-2);
}
.rs-tab {
  flex: 1;
  background: transparent;
  border: none;
  color: var(--app-text-2);
  padding: 12px 4px;
  cursor: pointer;
  font-size: 0.85rem;
  border-bottom: 2px solid transparent;
}
.rs-tab.active {
  color: var(--app-text-strong);
  border-bottom-color: var(--app-accent, #4f8bff);
}

.rs-current {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 12px 10px;
  border-bottom: 1px solid var(--app-border-2);
}
.rs-cur-label {
  font-size: 0.8rem;
  font-weight: 600;
}
.rs-cur-path {
  font-size: 0.7rem;
  color: var(--app-text-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.rs-cur-actions {
  display: flex;
  gap: 8px;
  margin-top: 6px;
}
.rs-btn {
  background: var(--app-surface-3, #242428);
  border: 1px solid var(--app-border-2);
  color: var(--app-text-strong);
  padding: 4px 10px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.75rem;
}
.rs-btn.on {
  background: rgba(255, 211, 79, 0.18);
  border-color: #ffd34f;
  color: #ffd34f;
}
.rs-btn.ok {
  background: var(--app-accent, #4f8bff);
  color: #fff;
  border-color: transparent;
}

.rs-form {
  padding: 10px 12px;
  border-bottom: 1px solid var(--app-border-2);
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--app-surface-2, #1b1b1f);
}
.rs-input {
  background: var(--app-surface-3, #242428);
  border: 1px solid var(--app-border-2);
  color: var(--app-text-strong);
  border-radius: 5px;
  padding: 5px 8px;
  font-size: 0.8rem;
}
.rs-form-row {
  display: flex;
  gap: 8px;
}
.rs-fld {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 0.7rem;
  color: var(--app-text-2);
}
.rs-select {
  background: var(--app-surface-3, #242428);
  border: 1px solid var(--app-border-2);
  color: var(--app-text-strong);
  border-radius: 5px;
  padding: 4px 6px;
  font-size: 0.78rem;
}
.rs-form-actions {
  display: flex;
  gap: 8px;
}

.rs-body {
  flex: 1;
  overflow-y: auto;
  padding: 10px 12px 16px;
}
.rs-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
  gap: 8px;
}
.rs-thumb {
  position: relative;
  aspect-ratio: 3 / 4;
  border-radius: 6px;
  overflow: hidden;
  border: 2px solid transparent;
  cursor: pointer;
  background: var(--app-surface-2, #1b1b1f);
}
.rs-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.rs-thumb.current {
  border-color: var(--app-accent, #4f8bff);
  box-shadow: 0 0 0 1px var(--app-accent, #4f8bff);
}
.rs-tnum {
  position: absolute;
  left: 3px;
  bottom: 3px;
  font-size: 0.6rem;
  background: rgba(0, 0, 0, 0.65);
  color: #fff;
  padding: 1px 4px;
  border-radius: 4px;
}
.rs-badge {
  position: absolute;
  top: 3px;
  right: 3px;
  font-size: 0.7rem;
  background: rgba(0, 0, 0, 0.5);
  border-radius: 4px;
  padding: 1px 3px;
}
.rs-badge.bm {
  color: #ffd34f;
}
.rs-badge.ch {
  color: #8fd0ff;
}
.rs-empty {
  text-align: center;
  color: var(--app-text-2);
  padding: 30px 0;
  font-size: 0.85rem;
}
.rs-empty-sub {
  font-size: 0.72rem;
  opacity: 0.8;
}

.rs-outline {
  display: flex;
  flex-direction: column;
}
.rs-node {
  margin-top: 2px;
}
.lv1 {
  margin-top: 8px;
}
.lv2 {
  margin-left: 16px;
}
.lv3 {
  margin-left: 32px;
}
.rs-node-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  cursor: pointer;
}
.rs-node-row:hover {
  background: var(--app-surface-3, #242428);
}
.rs-node-row.current {
  background: color-mix(in srgb, var(--app-accent, #4f8bff) 22%, transparent);
}
.rs-node-name {
  flex: 1;
  font-size: 0.82rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rs-node-page {
  font-size: 0.7rem;
  color: var(--app-text-2);
  flex: none;
}
.rs-del {
  background: transparent;
  border: none;
  color: var(--app-text-2);
  font-size: 0.7rem;
  cursor: pointer;
  padding: 0 2px;
  flex: none;
  opacity: 0.6;
}
.rs-del:hover {
  opacity: 1;
  color: #ff8a8a;
}

.rs-bm-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.rs-bm-item {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--app-surface-3, #242428);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 8px 10px;
  cursor: pointer;
  color: var(--app-text-strong);
  text-align: left;
}
.rs-bm-item.current {
  border-color: var(--app-accent, #4f8bff);
}
.rs-bm-item:hover {
  background: var(--app-surface-2, #1b1b1f);
}
.rs-bm-star {
  color: #ffd34f;
}
.rs-bm-label {
  flex: 1;
  font-size: 0.82rem;
}
.rs-bm-page {
  font-size: 0.72rem;
  color: var(--app-text-2);
}
</style>
