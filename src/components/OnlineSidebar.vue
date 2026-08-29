<script setup lang="ts">
// 🔖 搜刮书签（Round27）：侧栏快速跳转 + hover 删除
import { useRouter } from 'vue-router'
import { scrapeBookmarks, removeScrapeBookmark } from '@/stores/scrapeBookmarksStore'
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
  if (ok) {
    removeScrapeBookmark(bm.id)
    toast.success(`书签「${bm.name}」已删除`)
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

  <!-- 🔖 搜刮书签（Round27）：点击跳转恢复位置，hover 显示删除 -->
  <div class="nav-group">
    <span class="group-title">🔖 书签</span>
    <template v-if="scrapeBookmarks.length > 0">
      <button
        v-for="bm in scrapeBookmarks"
        :key="bm.id"
        class="bookmark-item"
        :title="
          bm.anchor
            ? `锚定画廊: ${bm.anchor.title || bm.anchor.gid}（点击跳转）`
            : '未锚定卡片（点击跳转）'
        "
        @click="handleBookmarkJump(bm)"
      >
        <span class="bm-icon">{{ bm.type === 'search' ? '🔍' : '🏠' }}</span>
        <span class="bm-name">{{ bm.name }}</span>
        <span class="bm-delete" title="删除书签" @click.stop="handleBookmarkRemove(bm)">✕</span>
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
/* 书签按钮：与 App.vue 的 .nav-menu a 视觉一致 */
.bookmark-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  background: transparent;
  border: none;
  color: var(--app-text-2);
  padding: 8px 12px;
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

.bm-icon {
  font-size: 0.85rem;
  flex-shrink: 0;
}

.bm-name {
  flex: 1;
  min-width: 0;
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
