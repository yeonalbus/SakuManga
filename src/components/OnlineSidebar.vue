<script setup lang="ts">
// 🔖 搜刮书签（Round49 重构）
//
// 书签主战场已迁到独立页面 `/online/bookmarks`；侧栏只保留：
//   - 标题行：点击进入书签页 + 计数 + 手动「检查更新」按钮
//   - 置顶书签（最多 5 条）：点击沿用「跳转到在线列表并定位锚点」链路
// 不再折叠、不再内联展开全部、不再拖动排序、不再有失效检测/清理按钮
// （Round37 的分段收纳与失效检测已在 Round49 剪枝）。
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  pinnedBookmarks,
  scrapeBookmarks,
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

const total = computed(() => scrapeBookmarks.value.length)

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

  <!-- 🔖 搜刮书签（Round49：常驻置顶 + 进书签页；不折叠、不收纳、不拖动） -->
  <div class="nav-group bm-nav-group">
    <div class="bm-group-head" title="打开书签页" @click="goBookmarks">
      <span class="bm-group-title">🔖 书签</span>
      <span class="bm-group-count">{{ total }}</span>
      <button
        class="bm-probe-btn"
        :class="{ spinning: probing }"
        :disabled="probing"
        :title="probing ? '探测中…' : '检查更新（对开启更新开关的书签）'"
        @click.stop="runProbe"
      >
        {{ probing ? '⏳' : '🔄' }}
      </button>
    </div>

    <div v-if="pinnedBookmarks.length === 0" class="bm-pin-hint">
      💡 在书签页点 📌 置顶常用书签（最多 {{ PINNED_BOOKMARK_LIMIT }} 个）
    </div>

    <button
      v-for="bm in pinnedBookmarks"
      :key="bm.id"
      class="bm-pinned-item"
      :class="{ 'has-update': bm.hasUpdate }"
      :title="`${bookmarkDisplayTitle(bm)}（点击跳转并定位）`"
      @click="jumpToBookmark(bm)"
    >
      <span class="bm-pinned-title">{{ bookmarkDisplayTitle(bm) }}</span>
      <span v-if="bm.hasUpdate" class="bm-pinned-dot" title="锚点之后有新内容" />
      <span class="bm-pinned-role" :class="bm.role">{{ roleLabels[bm.role] }}</span>
    </button>
  </div>
</template>

<style scoped>
.bm-nav-group {
  gap: 2px;
}

/* 标题行：点击整行进书签页；探测按钮独立触发（stop 冒泡） */
.bm-group-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-radius: 6px;
  color: var(--app-text-2);
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.2s;
}
.bm-group-head:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

.bm-group-title {
  flex-shrink: 0;
}

.bm-group-count {
  font-size: 0.7rem;
  color: var(--app-text-muted);
  background-color: var(--app-surface-3);
  padding: 0 5px;
  border-radius: 8px;
}

.bm-probe-btn {
  margin-left: auto;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 4px;
  padding: 2px 5px;
  font-size: 0.75rem;
  line-height: 1;
  color: var(--app-text-3);
  cursor: pointer;
  transition: all 0.15s ease;
}
.bm-probe-btn:hover:not(:disabled) {
  color: var(--app-text-strong);
  background-color: var(--app-surface-3);
}
.bm-probe-btn:disabled {
  opacity: 0.6;
  cursor: default;
}
.bm-probe-btn.spinning {
  opacity: 1;
}

.bm-pin-hint {
  font-size: 0.72rem;
  line-height: 1.5;
  color: var(--app-text-muted);
  padding: 2px 12px 6px;
}

/* 置顶书签项：一行（标题 + 红点 + 标记） */
.bm-pinned-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 6px 10px 6px 14px;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--app-text-3);
  font-size: 0.85rem;
  text-align: left;
  cursor: pointer;
  transition: all 0.2s;
}
.bm-pinned-item:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}
.bm-pinned-item.has-update {
  color: var(--app-text-strong);
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

.bm-pinned-role {
  flex-shrink: 0;
  font-size: 0.66rem;
  padding: 0 4px;
  border-radius: 3px;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-muted);
}
.bm-pinned-role.newest {
  border-color: rgba(0, 122, 204, 0.5);
  color: #4fa3e3;
}
.bm-pinned-role.oldest {
  border-color: rgba(76, 195, 138, 0.5);
  color: #4cc38a;
}
</style>
