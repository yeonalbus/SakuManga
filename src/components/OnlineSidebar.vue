<script setup lang="ts">
// 🔖 搜刮书签（Round49 侧栏，第 2 版）
//
// 样式口径：与「书架」同构——标题行 + 缩进子项（.foldable-body / .sub-nav-item），
// 但书签组恒定平铺、无折叠箭头、不显示计数与图标。
//
// 布局：
//   书签            ← 点击进入书签页
//     检查更新        ← 手动触发更新探测
//     置顶书签 1..5   ← 点击跳转到在线列表并定位锚点
//
// 置顶为空时给一行纯文字引导（在书签页置顶）。
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  pinnedBookmarks,
  bookmarkDisplayTitle,
  probeScrapeBookmarks,
  PINNED_BOOKMARK_LIMIT,
} from '@/stores/scrapeBookmarksStore'
import type { ScrapeBookmark } from '@/types/comic'
import { isStandalonePWA } from '@/utils/detailNav'
import { useUI } from '@/composables/useUI'
import SidebarGroupHeader from '@/components/SidebarGroupHeader.vue'

const router = useRouter()
const { toast } = useUI()

const probing = ref(false)

/** 进入书签页 */
const goBookmarks = () => {
  router.push('/online/bookmarks').catch(() => {})
}

/** 点击置顶书签：跳转到对应在线列表并定位锚点（PWA 同标签 / 桌面新标签） */
const jumpToBookmark = (bm: ScrapeBookmark) => {
  const routeObj = { path: '/online/home', query: { bm: bm.id } }
  if (isStandalonePWA()) {
    router.push(routeObj).catch(() => {})
  } else {
    window.open(router.resolve(routeObj).href, '_blank')
  }
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

  <!-- 🔖 搜刮书签（Round49：书架同构——标题行 + 缩进子项；无折叠、无图标、无计数） -->
  <div class="nav-group">
    <div class="foldable-item">
      <SidebarGroupHeader title="书签" :collapsible="false" @activate="goBookmarks" />

      <div class="foldable-body">
        <!-- 第一项：检查更新（普通子项，位于标题与置顶书签之间） -->
        <button class="sub-nav-item" :disabled="probing" @click="runProbe">
          <span class="bm-pinned-title">{{ probing ? '检查中…' : '检查更新' }}</span>
        </button>

        <!-- 置顶书签：只显示标题 + 有更新时的红点 -->
        <button
          v-for="bm in pinnedBookmarks"
          :key="bm.id"
          class="sub-nav-item"
          :class="{ 'has-update': bm.hasUpdate }"
          :title="`${bookmarkDisplayTitle(bm)}（点击跳转并定位）`"
          @click="jumpToBookmark(bm)"
        >
          <span class="bm-pinned-title">{{ bookmarkDisplayTitle(bm) }}</span>
          <span v-if="bm.hasUpdate" class="bm-pinned-dot" title="锚点之后有新内容" />
        </button>

        <span v-if="pinnedBookmarks.length === 0" class="pin-hint">
          在书签页置顶常用书签（最多 {{ PINNED_BOOKMARK_LIMIT }} 个）
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 与书架（OfflineSidebar）同一套缩进/子项视觉 */
.foldable-item {
  display: flex;
  flex-direction: column;
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
  width: 100%;
  padding: 6px 8px;
  border: none;
  border-radius: 4px;
  font-size: 0.85rem !important;
  color: var(--app-text-3) !important;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  background-color: transparent;
  transition: all 0.2s;
}

.sub-nav-item:hover:not(:disabled) {
  background-color: var(--app-surface-3) !important;
  color: var(--app-text-strong) !important;
}

.sub-nav-item:disabled {
  opacity: 0.6;
  cursor: default;
}

.bm-pinned-title {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bm-pinned-dot {
  flex-shrink: 0;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background-color: #ff4d4f;
}

.pin-hint {
  font-size: 0.72rem;
  line-height: 1.5;
  color: var(--app-text-muted);
  padding: 2px 8px;
}
</style>
