<script setup lang="ts">
// 🔖 搜刮书签 · 侧栏（Round49 第 3 版）
//
// 结构：「书签」作为在线模式组的最后一项（与首页/订阅同款链接样式），
// 子项（检查更新、置顶书签）缩进 32px 挂在其下方——不再单独成组、不再有组间距。
//
// 子项辨识度：缩进 + 更小字号 + 淡色文字；置顶书签右侧显示淡色角色徽章
// （追新/续扫/兴趣）+ 有更新时的红点。检查更新用书架同款边框按钮，并显示
// 当前开启更新探测的书签数量。暂不使用 emoji 图标（等矢量图标方案统一替换）。
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  scrapeBookmarks,
  pinnedBookmarks,
  bookmarkDisplayTitle,
  probeScrapeBookmarks,
  PINNED_BOOKMARK_LIMIT,
} from '@/stores/scrapeBookmarksStore'
import type { ScrapeBookmark, ScrapeBookmarkRole } from '@/types/comic'
import { isStandalonePWA } from '@/utils/detailNav'
import { useUI } from '@/composables/useUI'

const router = useRouter()
const { toast } = useUI()

const probing = ref(false)

const roleLabels: Record<ScrapeBookmarkRole, string> = {
  newest: '追新',
  oldest: '续扫',
  interest: '兴趣',
}

/** 开启更新探测的书签数量（检查更新按钮上显示） */
const watchCount = computed(() => scrapeBookmarks.value.filter((b) => b.updateWatch).length)

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
    <!-- 书签：与其它导航项同款链接，点击进入书签页（进入后自动蓝底高亮） -->
    <router-link to="/online/bookmarks">书签</router-link>

    <!-- 书签子项：缩进 32px，字号更小、颜色更淡，与上级条目形成层级 -->
    <div class="bm-children">
      <!-- 检查更新：书架同款边框按钮 + 开启探测的数量 -->
      <button class="bm-probe-btn" :disabled="probing" @click="runProbe">
        {{ probing ? '检查中…' : `检查更新（${watchCount}）` }}
      </button>

      <!-- 置顶书签：标题 + 角色徽章 + 有更新的红点 -->
      <button
        v-for="bm in pinnedBookmarks"
        :key="bm.id"
        class="bm-child-item"
        :title="`${bookmarkDisplayTitle(bm)}（点击跳转并定位）`"
        @click="jumpToBookmark(bm)"
      >
        <span class="bm-child-title">{{ bookmarkDisplayTitle(bm) }}</span>
        <span class="bm-role-badge" :class="bm.role">{{ roleLabels[bm.role] }}</span>
        <span v-if="bm.hasUpdate" class="bm-pinned-dot" title="锚点之后有新内容" />
      </button>

      <span v-if="pinnedBookmarks.length === 0" class="bm-pin-hint">
        在书签页置顶常用书签（最多 {{ PINNED_BOOKMARK_LIMIT }} 个）
      </span>
    </div>
  </div>
</template>

<style scoped>
/* 子项容器：相对组左边缩进 32px */
.bm-children {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-left: 32px;
  margin-top: 2px;
}

/* 检查更新：仿书架「全部书架」边框按钮 */
.bm-probe-btn {
  box-sizing: border-box;
  width: 100%;
  margin: 2px 0 4px;
  padding: 5px 10px;
  border-radius: 4px;
  border: 1px solid var(--app-border-3);
  background: transparent;
  color: var(--app-text-3);
  font-size: 0.8rem;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  transition: all 0.2s;
}
.bm-probe-btn:hover:not(:disabled) {
  border-color: #007acc;
  color: #007acc;
}
.bm-probe-btn:disabled {
  opacity: 0.6;
  cursor: default;
}

/* 置顶书签子项 */
.bm-child-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 6px 8px;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--app-text-muted);
  font-size: 0.8rem;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  transition: all 0.2s;
}
.bm-child-item:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

.bm-child-title {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 淡色角色徽章（追新/续扫/兴趣） */
.bm-role-badge {
  flex-shrink: 0;
  font-size: 0.66rem;
  line-height: 1.4;
  padding: 0 4px;
  border-radius: 3px;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-muted);
}
.bm-role-badge.newest {
  border-color: rgba(0, 122, 204, 0.45);
  color: #4fa3e3;
}
.bm-role-badge.oldest {
  border-color: rgba(76, 195, 138, 0.45);
  color: #4cc38a;
}

.bm-pinned-dot {
  flex-shrink: 0;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background-color: #ff4d4f;
}

.bm-pin-hint {
  font-size: 0.72rem;
  line-height: 1.5;
  color: var(--app-text-muted);
  padding: 2px 8px;
}
</style>
