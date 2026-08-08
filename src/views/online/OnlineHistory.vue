<script setup lang="ts">
import { computed, onMounted, onUnmounted, nextTick } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { onlineHistoryList, clearHistory } from '@/stores/historyStore'
import GridContainer from '@/components/GridContainer.vue'
import BatchDownloadBar from '@/components/BatchDownloadBar.vue'
import OnlineDetailPanel from '@/components/OnlineDetailPanel.vue'
import type { OnlineComic } from '@/types/comic'
import { useBatchSelection } from '@/composables/useBatchSelection'
import { useDetailPanel } from '@/composables/useDetailPanel'
// Round7-任务4：列表状态记忆（新标签返回本页恢复滚动）+ 列表状态提供者
import {
  rememberListState,
  takeListState,
  setListStateProvider,
  clearListStateProvider,
  getMainContent,
} from '@/utils/scrollMemory'

// 动态提取在线浏览过的漫画列表（本页均为 online 来源）
const comics = computed<OnlineComic[]>(() =>
  onlineHistoryList.value.map((item) => item.comic as OnlineComic),
)

// 左右分栏详情面板（宽屏桌面生效；窄屏回退全屏详情路由）
const { isWide, isPanelOpen, panelGid, panelToken, panelFromHistory, openDetail, closePanel } =
  useDetailPanel()

// Round7-任务4：返回本页时恢复滚动位置；注册提供者供打开详情前捕获状态
onMounted(async () => {
  const saved = takeListState('/online/history')
  if (saved && saved.top > 0) {
    await nextTick()
    requestAnimationFrame(() => {
      const el = getMainContent()
      if (el && el.scrollHeight > 0) el.scrollTop = saved.top
    })
  }
  setListStateProvider('/online/history', () => ({
    top: getMainContent()?.scrollTop || 0,
    page: 1,
  }))
})

onUnmounted(() => {
  clearListStateProvider('/online/history')
})

onBeforeRouteLeave(() => {
  rememberListState('/online/history', {
    top: getMainContent()?.scrollTop || 0,
    page: 1,
  })
})

// 长按多选 → 批量下载
const {
  selectMode,
  selectedIds,
  selectedTargets,
  handleLongPress,
  handleSelect,
  toggleSelectAll,
  handleBatchClose,
} = useBatchSelection(() => comics.value)

const handleClear = () => {
  clearHistory('online')
}
</script>

<template>
  <div class="history-view">
    <div class="view-header">
      <h2 class="view-title">🌐 在线浏览历史 ({{ comics.length }})</h2>
      <button v-if="comics.length > 0" class="clear-btn" @click="handleClear">
        🗑️ 清空在线历史
      </button>
    </div>

    <div class="online-split" :class="{ 'panel-open': isPanelOpen }">
      <div class="split-main">
        <!-- Round7-任务6：历史入口卡片，「立即阅读」始终从上次位置开始 -->
        <GridContainer
          v-if="comics.length > 0"
          :items="comics"
          :selectable="true"
          :select-mode="selectMode"
          :selected-ids="selectedIds"
          :panel-mode="isWide"
          :panel-open="isPanelOpen"
          :from-history="true"
          @longpress="handleLongPress"
          @select="handleSelect"
          @open="(c) => openDetail(c, { fromHistory: true })"
        />
        <div v-else class="empty-tip">暂无在线浏览记录</div>

        <!-- 批量下载工具条（长按卡片进入选择模式后出现） -->
        <BatchDownloadBar
          v-if="selectMode"
          :selected="selectedTargets"
          @select-all="toggleSelectAll"
          @close="handleBatchClose"
        />
      </div>

      <!-- 右侧内嵌详情面板（仅宽屏桌面渲染） -->
      <OnlineDetailPanel
        v-if="isWide"
        :open="isPanelOpen"
        :gid="panelGid"
        :token="panelToken"
        :from-history="panelFromHistory"
        @close="closePanel"
      />
    </div>
  </div>
</template>

<style scoped>
.history-view {
  padding: 12px 4px;
  min-height: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.view-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
}

.view-title {
  font-size: 1.2rem;
  font-weight: 600;
  color: var(--app-text-strong);
  margin: 0;
}

.clear-btn {
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: #ef4444;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
}

.clear-btn:hover {
  background-color: rgba(239, 68, 68, 0.15);
  border-color: #ef4444;
}

.empty-tip {
  margin-top: 60px;
  text-align: center;
  color: var(--app-text-muted);
  font-size: 0.95rem;
}
</style>
