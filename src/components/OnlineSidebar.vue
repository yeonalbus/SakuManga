<script setup lang="ts">
// 🔖 搜刮书签（Round27）：侧栏快速跳转 + hover 删除
// Round28：后端化（多端同步）+ 失效锚点 ⚠️ 标记（BUG2 修复）
// Round33：失效检测入口 + 一键清理 + 迁移来源展示
// Round37：拖动排序（LexoRank）+ 分段收纳（前 6 条平铺，其余原位内联展开）+ 组折叠记忆
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  scrapeBookmarks,
  removeScrapeBookmark,
  failedAnchorGids,
  bookmarkLocationLabel,
  isBookmarkInvalid,
  invalidBookmarks,
  invalidReasonText,
  isBookmarkMigrated,
  clearInvalidBookmarks,
  // Round37：拖动排序
  moveBookmarkToPosition,
  flushPendingBookmarkSort,
} from '@/stores/scrapeBookmarksStore'
import type { ScrapeBookmark } from '@/types/comic'
import { isStandalonePWA } from '@/utils/detailNav'
import { useUI } from '@/composables/useUI'
import { useBookmarkCheck } from '@/composables/useBookmarkCheck'
// Round37：拖动原语（与书架列表 / 全部书架浮层共用）
import { useDragReorder } from '@/composables/useDragReorder'
import { loadStorage, saveStorage } from '@/utils/storage'

const router = useRouter()
const { toast, modal } = useUI()
const { checking, runCheck } = useBookmarkCheck()

// 点击书签跳转：PWA 同标签 / 桌面新标签（与搜索分流一致，isStandalonePWA 判定）
const handleBookmarkJump = (bm: ScrapeBookmark) => {
  // Round37：拖动刚结束时抑制本次点击（避免拖拽落点误触跳转）
  if (bmConsumeClick()) return
  const routeObj = { path: '/online/home', query: { bm: bm.id } }
  if (isStandalonePWA()) {
    router.push(routeObj).catch(() => {})
  } else {
    const url = router.resolve(routeObj).href
    window.open(url, '_blank')
  }
}

/** 书签展示名（名称优先，无名则位置标签） */
const displayName = (bm: ScrapeBookmark): string =>
  bm.name.trim() || bookmarkLocationLabel(bm)

const handleBookmarkRemove = async (bm: ScrapeBookmark) => {
  const ok = await modal.confirm(`确定删除书签「${displayName(bm)}」吗？`, '删除书签')
  if (!ok) return
  // Round28：后端化后为异步删除（乐观 + 失败回滚），成功才提示
  const removed = await removeScrapeBookmark(bm.id)
  if (removed) toast.success(`书签「${displayName(bm)}」已删除`)
}

/** 某书签是否已在锚点失效状态（持久化标记 或 本会话定位失败） */
const isAnchorFailed = (bm: ScrapeBookmark): boolean =>
  isBookmarkInvalid(bm) || (!!bm.anchor && failedAnchorGids.value.has(bm.anchor.gid))

/** Round33：检测失效（手动全量） */
const handleCheck = async () => {
  await runCheck()
}

/** Round33：一键清理失效书签 */
const handleClearInvalid = async () => {
  const n = invalidBookmarks.value.length
  if (n === 0) return
  const ok = await modal.confirm(`确定删除全部 ${n} 条失效书签吗？`, '清理失效书签')
  if (!ok) return
  const removed = await clearInvalidBookmarks()
  toast.success(`已清理 ${removed} 条失效书签`)
}

/** 失效书签数量（用于清理入口显隐） */
const invalidCount = computed(() => invalidBookmarks.value.length)

// ─── Round30：邮件列表式双行排版（行1 主文本 + 日期；行2 搜索词 + 时间）───

/** 行1 主文本：名称优先；无名时搜索类型显示搜索词、首页类型显示「首页」 */
const primaryText = (bm: ScrapeBookmark): string => {
  const name = bm.name.trim()
  if (name) return name
  const kw = (bm.keyword || '').trim()
  return bm.type === 'search' && kw ? kw : '首页'
}

/** 行2 副文本：固定显示搜索词（仅当行1 已被名称占用，避免与主文本重复） */
const subText = (bm: ScrapeBookmark): string => {
  if (!bm.name.trim()) return ''
  const kw = (bm.keyword || '').trim()
  return bm.type === 'search' ? kw : ''
}

/** 发布时间拆两行：`2026-09-12 01:47` → 日期 + 时间（缺则留空，老书签无此字段） */
const postedParts = (bm: ScrapeBookmark): { date: string; time: string } => {
  const raw = bm.anchor?.postedAt?.trim() || ''
  if (!raw) return { date: '', time: '' }
  const [date, time = ''] = raw.split(/\s+/)
  return { date, time }
}

/** 类型图标：搜索 / 首页 */
const typeIcon = (bm: ScrapeBookmark): string => (bm.type === 'search' ? '🔍' : '🏠')

/** 悬停提示：完整信息（失效原因 / 位置 / 时间 / 迁移来源 / 锚定画廊标题） */
const bookmarkTooltip = (bm: ScrapeBookmark): string => {
  const parts: string[] = []
  if (isAnchorFailed(bm)) {
    parts.push(`⚠️ ${invalidReasonText(bm) || '会话内定位失败：当前搜索&筛选条件下未找到锚定画廊'}`)
  }
  parts.push(bookmarkLocationLabel(bm))
  const { date, time } = postedParts(bm)
  if (date) parts.push(`${date} ${time}`.trim())
  if (isBookmarkMigrated(bm) && bm.anchor?.migratedFrom) {
    parts.push(`已从「${bm.anchor.migratedFrom.title || bm.anchor.migratedFrom.gid}」迁移`)
  } else if (bm.anchor?.title) {
    parts.push(bm.anchor.title)
  } else if (!bm.anchor) {
    parts.push('未锚定卡片')
  }
  return parts.join(' · ') + '（点击跳转）'
}

// ─────────────────────────────────────────────────────────────
// Round37：分段收纳 + 组折叠（状态记忆）
//
// 取舍（与用户对齐）：不做书架那样的浮层抽屉——书签的核心诉求是「一点回现场」，
// 多一层浮层等于给最高频动作加点击成本。改为常驻前 N 条平铺 + 其余「原位内联展开」，
// 书签不多时形态与旧版完全一致（零额外点击），多了也不会把下方分组顶出视口。
// ─────────────────────────────────────────────────────────────

/** 常驻区条数（前 N 条平铺，其余进收纳区） */
const RESIDENT_LIMIT = 6

const GROUP_OPEN_KEY = 'saku_bookmark_group_open'
const OVERFLOW_OPEN_KEY = 'saku_bookmark_overflow_open'

/** 书签组折叠状态（记忆；折叠后仅剩标题行） */
const groupOpen = ref(loadStorage<boolean>(GROUP_OPEN_KEY, true))
/** 收纳区展开状态（记忆） */
const overflowOpen = ref(loadStorage<boolean>(OVERFLOW_OPEN_KEY, false))

watch(groupOpen, (v) => saveStorage(GROUP_OPEN_KEY, v))
watch(overflowOpen, (v) => saveStorage(OVERFLOW_OPEN_KEY, v))

const hasOverflow = computed(() => scrapeBookmarks.value.length > RESIDENT_LIMIT)
const overflowCount = computed(() => Math.max(0, scrapeBookmarks.value.length - RESIDENT_LIMIT))

/**
 * 实际渲染的书签：未展开时只渲染常驻区。
 * 未展开时索引与完整列表前 N 位一一对应，故拖动落位可直接换算为全局位置。
 */
const renderedBookmarks = computed(() =>
  overflowOpen.value ? scrapeBookmarks.value : scrapeBookmarks.value.slice(0, RESIDENT_LIMIT),
)

const toggleGroup = () => {
  groupOpen.value = !groupOpen.value
}

const toggleOverflow = () => {
  overflowOpen.value = !overflowOpen.value
}

// ─────────────────────────────────────────────────────────────
// Round37：拖动排序（把手拖动 → LexoRank 单点移动）
// ─────────────────────────────────────────────────────────────

/** 书签列表滚动容器（展开后限高滚动；拖动落位与边缘自动滚动均以它为准） */
const bookmarkListEl = ref<HTMLElement | null>(null)

const bmDrag = useDragReorder({
  getScrollContainer: () => bookmarkListEl.value,
  rowSelector: '.bookmark-item',
  getGhostText: (i) => {
    const bm = renderedBookmarks.value[i]
    return bm ? primaryText(bm) : ''
  },
  // to 为「移除被拖项后数组」的插入下标 → 目标位置 = to + 1（1-based）
  onReorder: (from, to) => {
    const moved = renderedBookmarks.value[from]
    if (!moved) return
    void moveBookmarkToPosition(moved.id, to + 1)
  },
})

// 解构 ref 供模板自动解包（模板内嵌套对象的 ref 不会自动解包）
const {
  dragging: bmDragging,
  dragIndex: bmDragIndex,
  indicatorIndex: bmIndicatorIndex,
  ghostTop: bmGhostTop,
  ghostLeft: bmGhostLeft,
  ghostWidth: bmGhostWidth,
  ghostHeight: bmGhostHeight,
  onHandlePointerDown: bmHandleDown,
  consumeSuppressClick: bmConsumeClick,
  ghostText: bmGhostText,
} = bmDrag

// 离开页面时冲刷未落库的排序改动（拖动防抖 300ms）
onBeforeUnmount(flushPendingBookmarkSort)
</script>

<template>
  <div class="nav-group">
    <span class="group-title">🌐 在线模式</span>
    <router-link to="/online/home">首页</router-link>
    <router-link to="/online/sub">订阅</router-link>
    <router-link to="/online/hot">热门</router-link>
    <router-link to="/online/toplist">排行榜</router-link>
    <router-link to="/online/favorites">收藏</router-link>
    <router-link to="/online/history">历史记录</router-link>
  </div>

  <!-- 🔖 搜刮书签（Round27 / Round30 邮件列表式排版；Round33 检测与清理入口；Round37 拖动排序 + 分段收纳）
       行1：图标位（hover 让位给 ⠿ ✕） + 主文本 + 日期；行2：搜索词 + 时间 -->
  <div class="nav-group">
    <span class="group-title bm-group-title">
      <span class="bm-title" title="折叠 / 展开书签组" @click="toggleGroup">
        <span class="bm-arrow" :class="{ open: groupOpen }">❯</span>
        <span>🔖 书签</span>
        <span v-if="!groupOpen && scrapeBookmarks.length > 0" class="bm-total">{{ scrapeBookmarks.length }}</span>
      </span>
      <span class="bm-actions">
        <button
          class="bm-action"
          :class="{ spinning: checking }"
          :disabled="checking"
          :title="checking ? '检测中…' : '检测失效书签（按各书签原始搜索&筛选条件复核锚定画廊是否仍可见）'"
          @click="handleCheck"
        >
          {{ checking ? '⏳' : '🔍' }}
        </button>
        <button
          v-if="invalidCount > 0"
          class="bm-action danger"
          :title="`清理 ${invalidCount} 条失效书签`"
          @click="handleClearInvalid"
        >
          🧹<span class="bm-badge">{{ invalidCount }}</span>
        </button>
      </span>
    </span>

    <template v-if="groupOpen">
      <template v-if="scrapeBookmarks.length > 0">
        <!-- 书签列表（展开收纳区时限高滚动，避免顶掉下方「🎲 工具 / 系统」分组） -->
        <div ref="bookmarkListEl" class="bm-list" :class="{ scrollable: overflowOpen && hasOverflow }">
          <template v-for="(bm, idx) in renderedBookmarks" :key="bm.id">
            <button
              class="bookmark-item"
              :class="{ 'anchor-failed': isAnchorFailed(bm) }"
              :title="bookmarkTooltip(bm)"
              @click="handleBookmarkJump(bm)"
            >
              <!-- 行1 -->
              <span class="bm-line1">
                <!-- 左侧图标位：平时显示类型图标，hover 时扩宽让位给「⠿ 拖动 / ✕ 删除」（日期不受影响） -->
                <span class="bm-lead">
                  <span class="bm-icon">{{ typeIcon(bm) }}</span>
                  <span
                    class="bm-drag"
                    title="拖动排序"
                    @pointerdown="(e) => bmHandleDown(e as PointerEvent, idx)"
                    @click.stop.prevent
                  >
                    ⠿
                  </span>
                  <span class="bm-delete" title="删除书签" @click.stop="handleBookmarkRemove(bm)">✕</span>
                </span>
                <span v-if="isAnchorFailed(bm)" class="bm-warn" :title="invalidReasonText(bm) || '会话内定位失败：当前搜索&筛选条件下未找到锚定画廊'">⚠️</span>
                <span class="bm-primary">{{ primaryText(bm) }}</span>
                <span v-if="postedParts(bm).date" class="bm-date">{{ postedParts(bm).date }}</span>
              </span>
              <!-- 行2（日期/时间或副文本存在时才渲染） -->
              <span v-if="subText(bm) || postedParts(bm).time" class="bm-line2">
                <span class="bm-sub" :title="subText(bm)">{{ subText(bm) }}</span>
                <span v-if="postedParts(bm).time" class="bm-time" :title="`锚定画廊发布时间 ${postedParts(bm).date} ${postedParts(bm).time}`">
                  {{ postedParts(bm).time }}
                </span>
              </span>
            </button>
            <!-- Round37：拖拽落位指示线 -->
            <div v-if="bmDragging && bmIndicatorIndex === idx" class="drop-line bm-drop-line" />
          </template>
          <!-- 拖到末尾的落位指示线 -->
          <div
            v-if="bmDragging && bmIndicatorIndex === renderedBookmarks.length"
            class="drop-line bm-drop-line"
          />
        </div>

        <!-- Round37：收纳区入口（原位内联展开，无浮层/无遮罩，展开后仍是原位单击直达） -->
        <button v-if="hasOverflow" class="bm-more" @click="toggleOverflow">
          {{ overflowOpen ? '▴ 收起' : `⋯ 其余 ${overflowCount} 条` }}
        </button>
      </template>
      <span v-else class="bm-empty">暂无书签</span>
    </template>

    <!-- Round37：拖拽幽灵卡（fixed 跟随指针） -->
    <Teleport to="body">
      <div
        v-if="bmDragging"
        class="drag-ghost"
        :style="{
          top: bmGhostTop + 'px',
          left: bmGhostLeft + 'px',
          width: bmGhostWidth + 'px',
          height: bmGhostHeight + 'px',
        }"
      >
        🔖 {{ bmGhostText(bmDragIndex) }}
      </div>
    </Teleport>
  </div>

  <!-- 🎲 工具：跨模式全局功能（骰子支持全库、清单有在线/离线双 tab） -->
  <div class="nav-group">
    <span class="group-title">🎲 工具</span>
    <router-link to="/random">手气不错</router-link>
    <router-link to="/reading-list">阅读清单</router-link>
    <router-link to="/upgrade">画质升级</router-link>
  </div>
</template>

<style scoped>
/* 书签按钮：与 App.vue 的 .nav-menu a 视觉一致；
   Round30：邮件列表式双行排版（行1 图标+主文本+日期，行2 搜索词+时间），日期/时间右对齐 */
.bookmark-item {
  display: flex;
  flex-direction: column;
  gap: 1px;
  width: 100%;
  background: transparent;
  border: none;
  color: var(--app-text-2);
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 0.86rem;
  margin-bottom: 2px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: left;
}

.bookmark-item:hover {
  background-color: var(--app-surface-hover);
  color: var(--app-fg);
}

/* 行1：图标位 + 主文本 + 日期 */
.bm-line1 {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-width: 0;
}

/* 左侧图标位（固定 16px）：平时放类型图标；
   Round37：hover 时扩到 34px，同位置换成「⠿ 拖动 / ✕ 删除」，右侧日期不受影响 */
.bm-lead {
  position: relative;
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  transition: width 0.15s ease;
}

.bookmark-item:hover .bm-lead {
  width: 34px;
}

.bm-icon,
.bm-drag,
.bm-delete {
  position: absolute;
  top: 0;
  height: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
  border-radius: 4px;
  transition:
    opacity 0.15s ease,
    color 0.15s ease,
    background-color 0.15s ease;
}

.bm-icon {
  left: 0;
  width: 16px;
  font-size: 0.8rem;
  opacity: 1;
}

/* Round37：拖动把手（touch-action:none 保证把手上触摸拖动不触发侧栏滚动） */
.bm-drag {
  left: 0;
  width: 16px;
  font-size: 0.78rem;
  color: var(--app-text-muted);
  cursor: grab;
  touch-action: none;
  user-select: none;
  opacity: 0;
}

.bm-drag:active {
  cursor: grabbing;
}

.bm-drag:hover {
  color: var(--app-text-strong);
  background-color: var(--app-surface-3);
}

/* 删除按钮：hover 时出现在把手右侧 */
.bm-delete {
  left: 18px;
  width: 16px;
  font-size: 0.75rem;
  color: var(--app-text-muted);
  cursor: pointer;
  opacity: 0;
}

.bm-delete:hover {
  color: #ef4444;
  background-color: var(--app-surface-3);
}

.bookmark-item:hover .bm-icon {
  opacity: 0;
}

.bookmark-item:hover .bm-drag,
.bookmark-item:hover .bm-delete {
  opacity: 1;
}

/* 📱 触摸设备（无 hover）：把手与删除常驻显示、图标位直接让位。
   否则移动端永远无法拖动排序，也没有任何删除入口（✕ 原本只在 hover 出现）。 */
@media (hover: none) {
  .bm-lead {
    width: 34px;
  }

  .bm-icon {
    opacity: 0;
  }

  .bm-drag,
  .bm-delete {
    opacity: 1;
  }
}

/* ⚠️ 失效锚点标记（Round28）：主文本前，弱化色 */
.bm-warn {
  font-size: 0.78rem;
  flex-shrink: 0;
  opacity: 0.9;
}

/* 失效书签整体弱化 + 主文本划线，提示不再可跳转定位 */
.bookmark-item.anchor-failed .bm-primary {
  color: var(--app-text-muted);
  text-decoration: line-through;
}

.bm-primary {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 日期（行1 右侧，不参与省略，主文本过长时优先让位） */
.bm-date {
  flex-shrink: 0;
  font-size: 0.72rem;
  color: var(--app-text-3);
  font-variant-numeric: tabular-nums;
}

/* 行2：搜索词 + 时间（缩进与行1 主文本列对齐：图标位 16px + gap 6px） */
.bm-line2 {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding-left: 22px;
  font-size: 0.7rem;
  line-height: 1.4;
  color: var(--app-text-muted);
}

.bm-sub {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bm-time {
  flex-shrink: 0;
  color: var(--app-text-3);
  font-variant-numeric: tabular-nums;
}

.bm-empty {
  font-size: 0.78rem;
  color: var(--app-text-muted);
  padding: 6px 12px;
}

/* ─── Round37：分段收纳 ─── */

.bm-list {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

/* 收纳区展开后限高滚动：书签再多也不会把下方分组顶出视口 */
.bm-list.scrollable {
  max-height: min(46vh, 420px);
  overflow-y: auto;
  overscroll-behavior: contain;
}

.bm-list.scrollable::-webkit-scrollbar {
  width: 6px;
}

.bm-list.scrollable::-webkit-scrollbar-thumb {
  background-color: var(--app-border-3);
  border-radius: 3px;
}

/* 收纳区入口：⋯ 其余 N 条 / ▴ 收起 */
.bm-more {
  background: transparent;
  border: none;
  border-radius: 6px;
  color: var(--app-text-3);
  font-size: 0.76rem;
  text-align: left;
  padding: 4px 10px;
  margin-top: 2px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.bm-more:hover {
  background-color: var(--app-surface-hover);
  color: var(--app-text-strong);
}

/* Round37：拖拽落位指示线（行内边距） */
.bm-drop-line {
  margin-left: 10px;
  margin-right: 10px;
}

/* ─── Round33：书签分组标题行（折叠 + 检测 / 清理入口）─── */
.bm-group-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

/* 折叠触发区（标题文字 + 箭头） */
.bm-title {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  cursor: pointer;
}

.bm-arrow {
  font-size: 0.7rem;
  transition: transform 0.2s;
}

.bm-arrow.open {
  transform: rotate(90deg);
}

/* 折叠时展示总数，避免「组里到底有没有东西」的疑问 */
.bm-total {
  font-size: 0.7rem;
  color: var(--app-text-muted);
  background-color: var(--app-surface-3);
  padding: 0 5px;
  border-radius: 8px;
}

.bm-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.bm-action {
  position: relative;
  background: transparent;
  border: none;
  color: var(--app-text-muted);
  font-size: 0.72rem;
  line-height: 1;
  padding: 2px 4px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.bm-action:hover:not(:disabled) {
  color: var(--app-text-strong);
  background-color: var(--app-surface-3);
}

.bm-action:disabled {
  cursor: default;
  opacity: 0.8;
}

.bm-action.spinning {
  animation: bm-spin 1.2s linear infinite;
}

@keyframes bm-spin {
  to {
    transform: rotate(360deg);
  }
}

.bm-action.danger:hover {
  color: #ef4444;
}

/* 失效数量角标 */
.bm-badge {
  position: absolute;
  top: -4px;
  right: -4px;
  min-width: 12px;
  height: 12px;
  padding: 0 2px;
  border-radius: 6px;
  background-color: #ef4444;
  color: #ffffff;
  font-size: 0.6rem;
  line-height: 12px;
  text-align: center;
}
</style>
