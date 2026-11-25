<script setup lang="ts">
// 书签页（Round49 v3）
//
// - 置顶区（最多 5 条）与普通区之间一条分隔线
// - 区域内有更新的排最前（金色框），其余按名称序（主标题，含翻译名）
// - 卡片高度固定（见 BookmarkCard），内容变化不推动卡片位移
// - 检查更新按钮在页面头部；备注编辑走专用弹窗
import { computed, onMounted, ref } from 'vue'
import BookmarkCard from '@/components/BookmarkCard.vue'
import BookmarkNoteModal from '@/components/BookmarkNoteModal.vue'
import AppIcon from '@/components/icons/AppIcon.vue'
import {
  scrapeBookmarks,
  updateBookmarkFields,
  probeScrapeBookmarks,
  bookmarkDisplayTitle,
  PINNED_BOOKMARK_LIMIT,
} from '@/stores/scrapeBookmarksStore'
import { bookmarkTitleWith } from '@/utils/bookmarkTitle'
import { useTagStore } from '@/stores/tagStore'
import { useUI } from '@/composables/useUI'
import type { ScrapeBookmark, ScrapeBookmarkRole } from '@/types/comic'

const tagStore = useTagStore()
const { toast } = useUI()

const probing = ref(false)
const noteEditing = ref<ScrapeBookmark | null>(null)
const activeRole = ref<'all' | ScrapeBookmarkRole>('all')
const filterQuery = ref('')

const roleTabs: { key: 'all' | ScrapeBookmarkRole; label: string; dot?: ScrapeBookmarkRole }[] = [
  { key: 'all', label: '全部' },
  { key: 'newest', label: '追新', dot: 'newest' },
  { key: 'interest', label: '兴趣', dot: 'interest' },
  { key: 'oldest', label: '续扫', dot: 'oldest' },
]

const collator = new Intl.Collator('zh-CN', { numeric: true })

/** 主标题（自定义名 > 翻译名 > 位置+筛选），用于名称序 */
const titleOf = (bm: ScrapeBookmark) => bookmarkTitleWith(bm, tagStore.translate)

/** 区域内排序：有更新在前 → 名称序 */
const byUpdateThenTitle = (a: ScrapeBookmark, b: ScrapeBookmark): number => {
  if (a.hasUpdate !== b.hasUpdate) return a.hasUpdate ? -1 : 1
  return collator.compare(titleOf(a), titleOf(b))
}

const total = computed(() => scrapeBookmarks.value.length)
const pinnedCount = computed(() => scrapeBookmarks.value.filter((b) => b.pinned).length)
const updateCount = computed(() => scrapeBookmarks.value.filter((b) => b.hasUpdate).length)

const roleCounts = computed<Record<'all' | ScrapeBookmarkRole, number>>(() => {
  const counts: Record<'all' | ScrapeBookmarkRole, number> = {
    all: scrapeBookmarks.value.length,
    newest: 0,
    interest: 0,
    oldest: 0,
  }
  for (const b of scrapeBookmarks.value) {
    if (b.role in counts) counts[b.role] += 1
  }
  return counts
})

const isFiltered = computed(() => activeRole.value !== 'all' || filterQuery.value.trim().length > 0)

const filteredBookmarks = computed(() => {
  const kw = filterQuery.value.trim().toLowerCase()
  return scrapeBookmarks.value.filter((b) => {
    if (activeRole.value !== 'all' && b.role !== activeRole.value) return false
    if (!kw) return true
    const mainTitle = titleOf(b).toLowerCase()
    const subTitle = bookmarkDisplayTitle(b).toLowerCase()
    return mainTitle.includes(kw) || subTitle.includes(kw)
  })
})

const resetFilters = () => {
  activeRole.value = 'all'
  filterQuery.value = ''
}

const pinnedList = computed(() =>
  filteredBookmarks.value.filter((b) => b.pinned).slice().sort(byUpdateThenTitle),
)
const normalList = computed(() =>
  filteredBookmarks.value.filter((b) => !b.pinned).slice().sort(byUpdateThenTitle),
)

const editNote = (bm: ScrapeBookmark) => {
  noteEditing.value = bm
}

const saveNote = async (note: string) => {
  const bm = noteEditing.value
  if (!bm) return
  noteEditing.value = null
  const ok = await updateBookmarkFields(bm.id, { note })
  if (ok) toast.success('备注已保存')
}

/** 手动检查更新：探测全部开启更新开关的书签 */
const runProbe = async () => {
  if (probing.value) return
  probing.value = true
  try {
    const results = await probeScrapeBookmarks()
    if (results.length === 0) {
      toast.info('没有开启更新开关的书签')
      return
    }
    const withUpdate = results.filter((r) => r.status === 'ok' && r.hasUpdate).length
    const failed = results.filter((r) => r.status !== 'ok').length
    if (failed > 0) {
      toast.warning(`探测完成：有更新 ${withUpdate} 条，未判定 ${failed} 条`)
    } else {
      toast.success(`探测完成：有更新 ${withUpdate} 条`)
    }
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '更新探测失败，请重试')
  } finally {
    probing.value = false
  }
}

onMounted(() => {
  // 确保标签词典已加载（翻译名依赖它；App 启动时通常已加载，这里兜底）
  void tagStore.ensureDictionary()
})
</script>

<template>
  <div class="page-wrapper">
    <div class="bm-page">
      <header class="bm-page-head">
        <div class="bm-head-left">
          <h2 class="bm-page-title"><AppIcon name="bookmark" :size="18" /> 书签</h2>
          <span class="bm-page-stat">
            <template v-if="isFiltered">
              筛选命中 {{ filteredBookmarks.length }} / 共 {{ total }} 条 · 置顶 {{ pinnedCount }}/{{
                PINNED_BOOKMARK_LIMIT
              }}
              · 有更新 {{ updateCount }}
            </template>
            <template v-else>
              共 {{ total }} 条 · 置顶 {{ pinnedCount }}/{{ PINNED_BOOKMARK_LIMIT }} · 有更新
              {{ updateCount }}
            </template>
          </span>
        </div>
        <button class="bm-probe-btn" :disabled="probing" @click="runProbe">
          <template v-if="probing">
            <AppIcon name="loading" :size="15" class="spin" /> 探测中…
          </template>
          <template v-else><AppIcon name="refresh" :size="15" /> 检查更新</template>
        </button>
      </header>

      <div v-if="total === 0" class="bm-page-empty">
        还没有书签。在在线列表页点「存为书签」即可创建。
      </div>

      <template v-else>
        <!-- Round55 方案 1：页内单行轻量过滤条（左标记胶囊 + 右紧凑筛选框） -->
        <section class="bm-toolbar" aria-label="书签筛选">
          <div class="bm-role-tabs">
            <button
              v-for="tab in roleTabs"
              :key="tab.key"
              type="button"
              class="bm-tab"
              :class="[{ active: activeRole === tab.key }, tab.key !== 'all' ? `role-${tab.key}` : '']"
              @click="activeRole = tab.key"
            >
              <span v-if="tab.dot" class="bm-tab-dot" :class="tab.dot" />
              {{ tab.label }}
              <span class="bm-tab-cnt">{{ roleCounts[tab.key] }}</span>
            </button>
          </div>
          <div class="bm-filter-box">
            <AppIcon name="search" :size="13" class="bm-filter-icon" />
            <input
              v-model="filterQuery"
              class="bm-filter-input"
              type="text"
              placeholder="筛选当前书签（标题 / 副标题）…"
              autocomplete="off"
            />
            <button
              v-if="filterQuery.length > 0"
              type="button"
              class="bm-filter-clear"
              title="清空"
              @click="filterQuery = ''"
            >
              <AppIcon name="close" :size="11" />
            </button>
          </div>
        </section>

        <div v-if="filteredBookmarks.length === 0" class="bm-page-empty">
          没有匹配的书签
          <button type="button" class="bm-reset-btn" @click="resetFilters">清空筛选</button>
        </div>

        <template v-else>
          <!-- 置顶区 -->
          <div v-if="pinnedList.length" class="bm-grid">
            <BookmarkCard
              v-for="bm in pinnedList"
              :key="bm.id"
              :bm="bm"
              :pinned-count="pinnedCount"
              @edit-note="editNote"
            />
          </div>

          <!-- 置顶区 / 普通区分隔线 -->
          <div v-if="pinnedList.length && normalList.length" class="bm-divider" />

          <!-- 普通区 -->
          <div v-if="normalList.length" class="bm-grid">
            <BookmarkCard
              v-for="bm in normalList"
              :key="bm.id"
              :bm="bm"
              :pinned-count="pinnedCount"
              @edit-note="editNote"
            />
          </div>
        </template>
      </template>

      <!-- 备注编辑弹窗 -->
      <BookmarkNoteModal
        :show="!!noteEditing"
        :value="noteEditing?.note || ''"
        @close="noteEditing = null"
        @save="saveNote"
      />
    </div>
  </div>
</template>

<style scoped>
.bm-page {
  padding: 16px 20px 40px;
  max-width: 1280px;
  margin: 0 auto;
}

.bm-page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.bm-head-left {
  display: flex;
  align-items: baseline;
  gap: 12px;
  min-width: 0;
}

.bm-page-title {
  font-size: 1.1rem;
  margin: 0;
  color: var(--app-text-strong);
}

.bm-page-stat {
  font-size: 0.8rem;
  color: var(--app-text-3);
}

.bm-probe-btn {
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 8px;
  padding: 6px 12px;
  font-size: 0.82rem;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s ease;
}
.bm-probe-btn:hover:not(:disabled) {
  color: var(--app-text-strong);
  border-color: var(--app-text-muted);
}
.bm-probe-btn:disabled {
  opacity: 0.6;
  cursor: default;
}

.bm-page-empty {
  padding: 60px 0;
  text-align: center;
  color: var(--app-text-muted);
  font-size: 0.9rem;
}

.bm-reset-btn {
  margin-left: 8px;
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid var(--app-border-3);
  background-color: var(--app-surface-2);
  color: var(--app-text-2);
  font-size: 0.78rem;
  cursor: pointer;
}
.bm-reset-btn:hover {
  color: var(--app-text-strong);
  border-color: var(--app-text-muted);
}

/* Round55：页内单行轻量过滤条 */
.bm-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
  padding: 8px 10px;
  background-color: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--app-border-2);
  border-radius: 10px;
}

.bm-role-tabs {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.bm-tab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--app-text-2);
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.14s ease;
}
.bm-tab:hover {
  background-color: var(--app-surface-2);
  color: var(--app-text-strong);
}
.bm-tab-cnt {
  font-size: 0.72rem;
  color: var(--app-text-muted);
}
.bm-tab.active {
  background-color: var(--app-surface-2);
  border-color: var(--app-border-3);
  color: var(--app-text-strong);
  font-weight: 600;
}
.bm-tab.active.role-newest {
  border-color: rgba(79, 163, 227, 0.55);
  color: #4fa3e3;
  background-color: rgba(79, 163, 227, 0.1);
}
.bm-tab.active.role-oldest {
  border-color: rgba(76, 195, 138, 0.55);
  color: #4cc38a;
  background-color: rgba(76, 195, 138, 0.1);
}
.bm-tab.active.role-interest {
  border-color: rgba(154, 154, 154, 0.55);
  color: var(--app-text-strong);
  background-color: rgba(255, 255, 255, 0.07);
}

.bm-tab-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  display: inline-block;
}
.bm-tab-dot.newest {
  background-color: #4fa3e3;
}
.bm-tab-dot.interest {
  background-color: #9a9a9a;
}
.bm-tab-dot.oldest {
  background-color: #4cc38a;
}

.bm-filter-box {
  position: relative;
  display: flex;
  align-items: center;
  width: 240px;
  max-width: 100%;
}

.bm-filter-icon {
  position: absolute;
  left: 9px;
  color: var(--app-text-muted);
  pointer-events: none;
}

.bm-filter-input {
  width: 100%;
  height: 30px;
  padding: 0 26px 0 28px;
  border-radius: 7px;
  border: 1px solid var(--app-border-2);
  background-color: var(--app-input-bg);
  color: var(--app-text-strong);
  font-size: 0.8rem;
  outline: none;
  transition: border-color 0.15s ease;
}
.bm-filter-input::placeholder {
  color: var(--app-text-muted);
}
.bm-filter-input:focus {
  border-color: var(--app-border-3);
}

.bm-filter-clear {
  position: absolute;
  right: 6px;
  width: 18px;
  height: 18px;
  border: none;
  border-radius: 50%;
  background-color: var(--app-surface-3);
  color: var(--app-text-2);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
}
.bm-filter-clear:hover {
  color: var(--app-text-strong);
}

@media (max-width: 820px) {
  .bm-filter-box {
    width: 100%;
  }
}

/* 名片墙：3 列（窄屏降列） */
.bm-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
}

@media (max-width: 1100px) {
  .bm-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .bm-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

/* 置顶区 / 普通区分隔线 */
.bm-divider {
  height: 1px;
  background-color: var(--app-border-3);
  margin: 18px 2px;
}
</style>
