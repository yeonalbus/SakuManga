<script setup lang="ts">
// Round49：书签独立页面（名片墙）
//
// 定位：书签的主战场。侧栏只保留置顶书签 + 更新探测入口（快捷操作），
// 全量管理与浏览都在这里。
//
// - 卡片：封面（3:4）+ 固定标题（位置字段 + 非默认筛选摘要）+ 备注 +
//   书签标记 / 已囤货 / 更新红点
// - 排序：有更新的排最前，同层按名称序（store 的 sortedBookmarks）
// - 配置项（置顶 / 标记切换 / 备注 / 更新开关 / 已囤货 / 删除）走卡片底部操作行
// - 点击卡片 = 跳转到对应在线列表并定位锚点（沿用 `/online/home?bm=<id>` 链路）
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import BookmarkNoteModal from '@/components/BookmarkNoteModal.vue'
import {
  sortedBookmarks,
  scrapeBookmarks,
  bookmarkDisplayTitle,
  updateBookmarkFields,
  probeScrapeBookmarks,
  removeScrapeBookmark,
  PINNED_BOOKMARK_LIMIT,
} from '@/stores/scrapeBookmarksStore'
import type { ScrapeBookmark, ScrapeBookmarkRole } from '@/types/comic'
import { isStandalonePWA } from '@/utils/detailNav'
import { useUI } from '@/composables/useUI'

const router = useRouter()
const { toast, modal } = useUI()

const probing = ref(false)

const roleLabels: Record<ScrapeBookmarkRole, string> = {
  newest: '追新',
  oldest: '续扫',
  interest: '兴趣',
}
const roleOrder: ScrapeBookmarkRole[] = ['newest', 'oldest', 'interest']

const total = computed(() => scrapeBookmarks.value.length)
const updateCount = computed(() => scrapeBookmarks.value.filter((b) => b.hasUpdate).length)
const pinnedCount = computed(() => scrapeBookmarks.value.filter((b) => b.pinned).length)

/** 点击卡片：与侧栏同口径（PWA 同标签 / 桌面新标签）跳到在线列表并定位锚点 */
const jumpTo = (bm: ScrapeBookmark) => {
  const routeObj = { path: '/online/home', query: { bm: bm.id } }
  if (isStandalonePWA()) {
    router.push(routeObj).catch(() => {})
  } else {
    window.open(router.resolve(routeObj).href, '_blank')
  }
}

/** 置顶/取消置顶（上限与后端一致） */
const togglePin = async (bm: ScrapeBookmark) => {
  if (!bm.pinned && pinnedCount.value >= PINNED_BOOKMARK_LIMIT) {
    toast.warning(`置顶已达上限（${PINNED_BOOKMARK_LIMIT} 条），请先取消其他置顶`)
    return
  }
  const ok = await updateBookmarkFields(bm.id, { pinned: !bm.pinned })
  if (ok) toast.success(bm.pinned ? '已取消置顶' : '已置顶到侧栏')
}

/** 标记切换：追新 → 续扫 → 兴趣 循环 */
const cycleRole = async (bm: ScrapeBookmark) => {
  const idx = roleOrder.indexOf(bm.role)
  const next = roleOrder[(idx + 1) % roleOrder.length]
  const ok = await updateBookmarkFields(bm.id, { role: next })
  if (ok) toast.success(`标记已改为「${roleLabels[next]}」`)
}

/** 更新开关（关掉时后端/前端都会顺带清红点） */
const toggleWatch = async (bm: ScrapeBookmark) => {
  const ok = await updateBookmarkFields(bm.id, { updateWatch: !bm.updateWatch })
  if (ok) toast.success(bm.updateWatch ? '已关闭更新探测' : '已开启更新探测')
}

/** 已囤货标记（勾选时记录确认时间作证据） */
const toggleBulk = async (bm: ScrapeBookmark) => {
  const ok = await updateBookmarkFields(bm.id, { bulkDone: !bm.bulkDone })
  if (ok) toast.success(bm.bulkDone ? '已取消已囤货标记' : '已标记为已囤货')
}

/** 备注编辑：打开专用弹窗（3 行矩形输入框 + 宽度/行数校验，超限拒绝保存） */
const noteEditing = ref<ScrapeBookmark | null>(null)
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

const remove = async (bm: ScrapeBookmark) => {
  const ok = await modal.confirm(`确定删除书签「${bookmarkDisplayTitle(bm)}」吗？`, '删除书签')
  if (!ok) return
  if (await removeScrapeBookmark(bm.id)) toast.success('书签已删除')
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
  // 进入书签页时不做自动探测（探测是显式动作，避免每次进页都打 E 站）
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

      <div v-if="sortedBookmarks.length === 0" class="bm-page-empty">
        还没有书签。在在线列表页点「存为书签」即可创建。
      </div>

      <div v-else class="bm-grid">
        <article
          v-for="bm in sortedBookmarks"
          :key="bm.id"
          class="bm-card"
          :class="{ 'has-update': bm.hasUpdate, pinned: bm.pinned }"
          @click="jumpTo(bm)"
        >
          <div class="bm-card-cover">
            <img
              v-if="bm.anchor?.coverUrl"
              :src="bm.anchor.coverUrl"
              alt=""
              loading="lazy"
            />
            <div v-else class="bm-cover-placeholder">🖼</div>
            <span v-if="bm.pinned" class="bm-pin-flag" title="已置顶到侧栏">📌</span>
            <span v-if="bm.hasUpdate" class="bm-update-dot" title="锚点之后有新内容" />
          </div>

          <div class="bm-card-body">
            <div class="bm-card-title" :title="bm.anchor?.title || bookmarkDisplayTitle(bm)">
              {{ bookmarkDisplayTitle(bm) }}
            </div>

            <div class="bm-card-badges">
              <button
                class="bm-role"
                :class="bm.role"
                :title="`书签标记：${roleLabels[bm.role]}（点击切换）`"
                @click.stop="cycleRole(bm)"
              >
                {{ roleLabels[bm.role] }}
              </button>
              <span v-if="bm.updateWatch" class="bm-watch" title="已开启更新探测">🔔</span>
              <span v-if="bm.bulkDone" class="bm-bulk" title="已囤货">🧾 已囤货</span>
            </div>

            <div class="bm-card-note" :class="{ empty: !bm.note }">
              {{ bm.note || '（暂无备注）' }}
            </div>

            <div class="bm-card-actions" @click.stop>
              <button class="bm-act" :class="{ on: bm.pinned }" :title="bm.pinned ? '取消置顶' : '置顶到侧栏'" @click="togglePin(bm)">
                📌
              </button>
              <button class="bm-act" title="修改备注" @click="editNote(bm)">✏️</button>
              <button class="bm-act" :class="{ on: bm.updateWatch }" :title="bm.updateWatch ? '关闭更新探测' : '开启更新探测'" @click="toggleWatch(bm)">
                {{ bm.updateWatch ? '🔔' : '🔕' }}
              </button>
              <button class="bm-act" :class="{ on: bm.bulkDone }" :title="bm.bulkDone ? '取消已囤货' : '标记已囤货'" @click="toggleBulk(bm)">
                🧾
              </button>
              <button class="bm-act danger" title="删除书签" @click="remove(bm)">✕</button>
            </div>
          </div>
        </article>
      </div>
    </div>

    <!-- Round49：备注编辑弹窗（3 行矩形输入框 + 宽度/行数校验，超限拒绝保存） -->
    <BookmarkNoteModal
      :show="!!noteEditing"
      :value="noteEditing?.note || ''"
      @close="noteEditing = null"
      @save="saveNote"
    />
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

/* ─── 名片墙：3 列（窄屏降列）─── */
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

.bm-card {
  display: flex;
  gap: 10px;
  padding: 10px;
  border-radius: 10px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  cursor: pointer;
  min-width: 0;
  transition:
    border-color 0.15s ease,
    background-color 0.15s ease;
}
.bm-card:hover {
  border-color: var(--app-text-muted);
  background-color: var(--app-surface-3);
}
.bm-card.pinned {
  border-color: rgba(230, 184, 0, 0.5);
}
.bm-card.has-update {
  border-color: #007acc;
}

/* 封面：宽:高 = 3:4 */
.bm-card-cover {
  position: relative;
  flex-shrink: 0;
  width: 88px;
  height: 117px;
  border-radius: 6px;
  overflow: hidden;
  background-color: var(--app-surface-3);
}

.bm-card-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.bm-cover-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.4rem;
  color: var(--app-text-muted);
}

.bm-pin-flag {
  position: absolute;
  top: 3px;
  left: 3px;
  font-size: 0.75rem;
  line-height: 1;
  filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.6));
}

/* 更新红点：左上角 */
.bm-update-dot {
  position: absolute;
  top: 5px;
  left: 5px;
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background-color: #ff4d4f;
  box-shadow: 0 0 0 2px rgba(255, 77, 79, 0.25);
}
.bm-pin-flag + .bm-update-dot {
  left: 24px;
}

.bm-card-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.bm-card-title {
  font-size: 0.86rem;
  font-weight: 600;
  color: var(--app-text-strong);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bm-card-badges {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.bm-role {
  border: 1px solid var(--app-border-3);
  background-color: var(--app-surface-3);
  color: var(--app-text-2);
  border-radius: 4px;
  padding: 1px 6px;
  font-size: 0.7rem;
  cursor: pointer;
}
.bm-role:hover {
  color: var(--app-text-strong);
}
.bm-role.newest {
  border-color: rgba(0, 122, 204, 0.5);
  color: #4fa3e3;
}
.bm-role.oldest {
  border-color: rgba(76, 195, 138, 0.5);
  color: #4cc38a;
}
.bm-role.interest {
  border-color: var(--app-border-3);
  color: var(--app-text-3);
}

.bm-watch {
  font-size: 0.72rem;
}

.bm-bulk {
  font-size: 0.7rem;
  color: #e6b800;
}

/* 备注：固定三行高度，空备注也占位（防卡片高度不一）
   宽度锁定 19em（约 19 个中文字符/行），与备注输入限制（每行 19 字）对齐 */
.bm-card-note {
  font-size: 0.75rem;
  line-height: 1.45;
  color: var(--app-text-2);
  height: calc(1.45em * 3);
  max-width: 19em;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  word-break: break-all;
}
.bm-card-note.empty {
  color: var(--app-text-muted);
}

.bm-card-actions {
  display: flex;
  gap: 2px;
  margin-top: auto;
}

.bm-act {
  background: transparent;
  border: 1px solid transparent;
  border-radius: 4px;
  padding: 2px 5px;
  font-size: 0.75rem;
  line-height: 1;
  color: var(--app-text-3);
  cursor: pointer;
  opacity: 0.65;
  transition: all 0.15s ease;
}
.bm-act:hover {
  opacity: 1;
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}
.bm-act.on {
  opacity: 1;
}
.bm-act.danger:hover {
  color: #ef4444;
  background-color: rgba(239, 68, 68, 0.15);
}
</style>
