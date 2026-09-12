<script setup lang="ts">
// 🔖 搜刮书签（Round27）：侧栏快速跳转 + hover 删除
// Round28：后端化（多端同步）+ 失效锚点 ⚠️ 标记（BUG2 修复）
import { useRouter } from 'vue-router'
import {
  scrapeBookmarks,
  removeScrapeBookmark,
  failedAnchorGids,
  bookmarkLocationLabel,
} from '@/stores/scrapeBookmarksStore'
import type { ScrapeBookmark } from '@/types/comic'
import { isStandalonePWA } from '@/utils/detailNav'
import { useUI } from '@/composables/useUI'

const router = useRouter()
const { toast, modal } = useUI()

// 点击书签跳转：PWA 同标签 / 桌面新标签（与搜索分流一致，isStandalonePWA 判定）
const handleBookmarkJump = (bm: ScrapeBookmark) => {
  const routeObj = { path: '/online/home', query: { bm: bm.id } }
  if (isStandalonePWA()) {
    router.push(routeObj).catch(() => {})
  } else {
    const url = router.resolve(routeObj).href
    window.open(url, '_blank')
  }
}

const handleBookmarkRemove = async (bm: ScrapeBookmark) => {
  const ok = await modal.confirm(`确定删除书签「${bm.name}」吗？`, '删除书签')
  if (!ok) return
  // Round28：后端化后为异步删除（乐观 + 失败回滚），成功才提示
  const removed = await removeScrapeBookmark(bm.id)
  if (removed) toast.success(`书签「${bm.name}」已删除`)
}

/** 某书签是否已在本会话内确认锚点失效（⚠️ 标记） */
const isAnchorFailed = (bm: ScrapeBookmark): boolean =>
  !!bm.anchor && failedAnchorGids.value.has(bm.anchor.gid)

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

/** 悬停提示：完整信息（位置 + 时间 + 锚定画廊标题） */
const bookmarkTooltip = (bm: ScrapeBookmark): string => {
  if (isAnchorFailed(bm)) {
    return `⚠️ 锚定画廊已失效（可能被删除或更换）——${bm.anchor?.title || bm.anchor?.gid}`
  }
  const parts: string[] = [bookmarkLocationLabel(bm)]
  const { date, time } = postedParts(bm)
  if (date) parts.push(`${date} ${time}`.trim())
  if (bm.anchor?.title) parts.push(bm.anchor.title)
  else if (!bm.anchor) parts.push('未锚定卡片')
  return parts.join(' · ') + '（点击跳转）'
}
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

  <!-- 🔖 搜刮书签（Round27 / Round30 邮件列表式排版）
       行1：图标位（hover 让位给 ✕） + 主文本 + 日期；行2：搜索词 + 时间 -->
  <div class="nav-group">
    <span class="group-title">🔖 书签</span>
    <template v-if="scrapeBookmarks.length > 0">
      <button
        v-for="bm in scrapeBookmarks"
        :key="bm.id"
        class="bookmark-item"
        :class="{ 'anchor-failed': isAnchorFailed(bm) }"
        :title="bookmarkTooltip(bm)"
        @click="handleBookmarkJump(bm)"
      >
        <!-- 行1 -->
        <span class="bm-line1">
          <!-- 左侧图标位：平时显示类型图标，hover 时让位给删除 ✕（避免与右侧日期抢空间） -->
          <span class="bm-lead">
            <span class="bm-icon">{{ typeIcon(bm) }}</span>
            <span class="bm-delete" title="删除书签" @click.stop="handleBookmarkRemove(bm)">✕</span>
          </span>
          <span v-if="isAnchorFailed(bm)" class="bm-warn" title="锚定画廊已失效">⚠️</span>
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
    </template>
    <span v-else class="bm-empty">暂无书签</span>
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

/* 左侧图标位（固定 16px）：平时放类型图标，hover 时同位置换成删除 ✕ */
.bm-lead {
  position: relative;
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.bm-icon,
.bm-delete {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
  transition: opacity 0.15s ease;
}

.bm-icon {
  font-size: 0.8rem;
  opacity: 1;
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

/* 删除按钮：占用左侧图标位（hover 时图标淡出、✕ 淡入） */
.bm-delete {
  color: var(--app-text-muted);
  font-size: 0.75rem;
  border-radius: 4px;
  opacity: 0;
  cursor: pointer;
}

.bookmark-item:hover .bm-delete {
  opacity: 1;
}

.bookmark-item:hover .bm-icon {
  opacity: 0;
}

.bm-delete:hover {
  color: #ef4444;
  background-color: var(--app-surface-3);
}

.bm-empty {
  font-size: 0.78rem;
  color: var(--app-text-muted);
  padding: 6px 12px;
}
</style>
