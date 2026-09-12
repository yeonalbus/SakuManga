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

// ─── Round29：双行展示（行1 名称/位置，行2 发布时间 · 位置）───

/** 行1 主文本：用户填了名称则显示名称，否则显示位置（首页 / 搜索: xxx） */
const primaryText = (bm: ScrapeBookmark): string =>
  bm.name.trim() || bookmarkLocationLabel(bm)

/** 行2 发布时间（锚定画廊的 E 站 posted 日期；老书签可能没有） */
const postedText = (bm: ScrapeBookmark): string => bm.anchor?.postedAt?.trim() || ''

/** 行2 位置：仅当行1 已被名称占用时才补显示位置，避免重复 */
const secondaryLocation = (bm: ScrapeBookmark): string =>
  bm.name.trim() ? bookmarkLocationLabel(bm) : ''

/** 悬停提示：完整信息（位置 + 时间 + 锚定画廊标题） */
const bookmarkTooltip = (bm: ScrapeBookmark): string => {
  if (isAnchorFailed(bm)) {
    return `⚠️ 锚定画廊已失效（可能被删除或更换）——${bm.anchor?.title || bm.anchor?.gid}`
  }
  const parts: string[] = [bookmarkLocationLabel(bm)]
  const posted = postedText(bm)
  if (posted) parts.push(posted)
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

  <!-- 🔖 搜刮书签（Round27 / Round29 双行展示）：点击跳转恢复位置，hover 显示删除 -->
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
        <!-- 行1：位置/名称 + 失效标记 + 删除 -->
        <span class="bm-line1">
          <span class="bm-icon">{{ bm.type === 'search' ? '🔍' : '🏠' }}</span>
          <span v-if="isAnchorFailed(bm)" class="bm-warn" title="锚定画廊已失效">⚠️</span>
          <span class="bm-primary">{{ primaryText(bm) }}</span>
          <span class="bm-delete" title="删除书签" @click.stop="handleBookmarkRemove(bm)">✕</span>
        </span>
        <!-- 行2：发布时间（老书签无时间时退化为位置） -->
        <span v-if="postedText(bm) || secondaryLocation(bm)" class="bm-line2">
          <span v-if="postedText(bm)" class="bm-time" :title="`锚定画廊发布时间 ${postedText(bm)}`">
            📅 {{ postedText(bm) }}
          </span>
          <span v-if="secondaryLocation(bm)" class="bm-loc" :title="secondaryLocation(bm)">
            📍 {{ secondaryLocation(bm) }}
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
/* 书签按钮：与 App.vue 的 .nav-menu a 视觉一致；Round29 改双行布局（行1 位置/名称，行2 时间·位置） */
.bookmark-item {
  display: flex;
  flex-direction: column;
  gap: 1px;
  width: 100%;
  background: transparent;
  border: none;
  color: var(--app-text-2);
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 0.9rem;
  margin-bottom: 2px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: left;
}

.bookmark-item:hover {
  background-color: var(--app-surface-hover);
  color: var(--app-fg);
}

/* 行1：icon + 主文本 + 删除按钮 */
.bm-line1 {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-width: 0;
}

.bm-icon {
  font-size: 0.85rem;
  flex-shrink: 0;
}

/* ⚠️ 失效锚点标记（Round28）：与 icon 并列，弱化色 */
.bm-warn {
  font-size: 0.8rem;
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

/* 行2：发布时间 + 位置（小字次要色，缩进对齐 icon 之后的文本列） */
.bm-line2 {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding-left: 20px;
  font-size: 0.7rem;
  line-height: 1.4;
  color: var(--app-text-muted);
}

.bm-time,
.bm-loc {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bm-delete {
  flex-shrink: 0;
  color: var(--app-text-muted);
  font-size: 0.75rem;
  padding: 2px 4px;
  border-radius: 4px;
  opacity: 0;
  transition: all 0.15s ease;
}

.bookmark-item:hover .bm-delete {
  opacity: 1;
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
