<script setup lang="ts">
// 🔖 搜刮书签 · 侧栏（Round49 v4）
//
// 结构：「书签」作为在线模式组的最后一项；子项（检查更新、置顶书签）缩进 32px。
// 置顶书签 = 书签页的置顶区（最多 5 条）；排序与书签页一致：
//   有更新的排最前 + 金色框，其余按名称序（主标题，含翻译名）。
// 侧栏只显示标题（不显示徽章/图标/红点）。
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  scrapeBookmarks,
  probeScrapeBookmarks,
  PINNED_BOOKMARK_LIMIT,
} from '@/stores/scrapeBookmarksStore'
import { bookmarkTitleWith } from '@/utils/bookmarkTitle'
import { useTagStore } from '@/stores/tagStore'
import type { ScrapeBookmark } from '@/types/comic'
import { isStandalonePWA } from '@/utils/detailNav'
import { useUI } from '@/composables/useUI'
import AppIcon from '@/components/icons/AppIcon.vue'

const router = useRouter()
const tagStore = useTagStore()
const { toast } = useUI()

const probing = ref(false)
const collator = new Intl.Collator('zh-CN', { numeric: true })

const titleOf = (bm: ScrapeBookmark) => bookmarkTitleWith(bm, tagStore.translate)

/** 开启更新探测的书签数量（检查更新按钮上显示） */
const watchCount = computed(() => scrapeBookmarks.value.filter((b) => b.updateWatch).length)

/** 侧栏置顶列表：有更新在前 → 名称序，最多 5 条 */
const pinnedList = computed(() =>
  scrapeBookmarks.value
    .filter((b) => b.pinned)
    .slice()
    .sort((a, b) => {
      if (a.hasUpdate !== b.hasUpdate) return a.hasUpdate ? -1 : 1
      return collator.compare(titleOf(a), titleOf(b))
    })
    .slice(0, PINNED_BOOKMARK_LIMIT),
)

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
    <span class="group-title"><AppIcon name="online" :size="13" /> 在线模式</span>
    <router-link to="/online/home">首页</router-link>
    <router-link to="/online/sub">订阅</router-link>
    <router-link to="/online/hot">热门</router-link>
    <router-link to="/online/toplist">排行榜</router-link>
    <router-link to="/online/favorites">收藏</router-link>
    <router-link to="/online/history">历史记录</router-link>
    <router-link to="/online/bookmarks">书签</router-link>

    <div class="bm-children">
      <!-- 检查更新：书架同款边框按钮 + 开启探测的数量 -->
      <button class="bm-probe-btn" :disabled="probing" @click="runProbe">
        {{ probing ? '检查中…' : `检查更新（${watchCount}）` }}
      </button>

      <!-- 置顶书签：只显示标题（有更新 → 置前 + 金框） -->
      <button
        v-for="bm in pinnedList"
        :key="bm.id"
        class="bm-child-item"
        :class="{ updated: bm.hasUpdate }"
        :title="`${titleOf(bm)}（点击跳转并定位）`"
        @click="jumpToBookmark(bm)"
      >
        <span class="bm-child-title">{{ titleOf(bm) }}</span>
      </button>

      <span v-if="pinnedList.length === 0" class="bm-pin-hint">
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
  border: 1px solid transparent;
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

/* 有更新：金色框（与书签页同一规则） */
.bm-child-item.updated {
  border-color: #e6b800;
  color: var(--app-text-2);
}

.bm-child-title {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bm-pin-hint {
  font-size: 0.72rem;
  line-height: 1.5;
  color: var(--app-text-muted);
  padding: 2px 8px;
}
</style>
