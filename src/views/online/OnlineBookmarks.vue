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
import {
  scrapeBookmarks,
  updateBookmarkFields,
  probeScrapeBookmarks,
  PINNED_BOOKMARK_LIMIT,
} from '@/stores/scrapeBookmarksStore'
import { bookmarkTitleWith } from '@/utils/bookmarkTitle'
import { useTagStore } from '@/stores/tagStore'
import { useUI } from '@/composables/useUI'
import type { ScrapeBookmark } from '@/types/comic'

const tagStore = useTagStore()
const { toast } = useUI()

const probing = ref(false)
const noteEditing = ref<ScrapeBookmark | null>(null)

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

const pinnedList = computed(() =>
  scrapeBookmarks.value.filter((b) => b.pinned).slice().sort(byUpdateThenTitle),
)
const normalList = computed(() =>
  scrapeBookmarks.value.filter((b) => !b.pinned).slice().sort(byUpdateThenTitle),
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
          <h2 class="bm-page-title">🔖 书签</h2>
          <span class="bm-page-stat">
            共 {{ total }} 条 · 置顶 {{ pinnedCount }}/{{ PINNED_BOOKMARK_LIMIT }} · 有更新
            {{ updateCount }}
          </span>
        </div>
        <button class="bm-probe-btn" :disabled="probing" @click="runProbe">
          {{ probing ? '⏳ 探测中…' : '🔄 检查更新' }}
        </button>
      </header>

      <div v-if="total === 0" class="bm-page-empty">
        还没有书签。在在线列表页点「存为书签」即可创建。
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
