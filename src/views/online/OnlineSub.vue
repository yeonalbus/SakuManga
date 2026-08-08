<script setup lang="ts">
import { watch, onMounted, onActivated, nextTick } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import OnlineLoadBar from '@/components/OnlineLoadBar.vue'
import FloatingToolbar from '@/components/FloatingToolbar.vue'
import BatchDownloadBar from '@/components/BatchDownloadBar.vue'
import OnlineDetailPanel from '@/components/OnlineDetailPanel.vue'
import { useSubStore } from '@/stores/subStore' // 🟢 对应订阅专用的 Pinia Store
import { subSearchConfig } from '@/stores/searchStore' // 🟢 对应订阅专用的搜索/分类配置
import { useBatchSelection } from '@/composables/useBatchSelection'
import { useDetailPanel } from '@/composables/useDetailPanel'
// Round7-任务8：列表状态记忆 + 提供者（新标签返回本页恢复滚动位置）
import { onBeforeRouteLeave } from 'vue-router'
import {
  rememberListState,
  takeListState,
  setListStateProvider,
  getMainContent,
} from '@/utils/scrollMemory'

const subStore = useSubStore()

// 左右分栏详情面板（宽屏桌面生效；窄屏回退全屏详情路由）
const { isWide, isPanelOpen, panelGid, panelToken, openDetail, closePanel, togglePanel } =
  useDetailPanel()

// 长按多选 → 批量下载
const {
  selectMode,
  selectedIds,
  selectedTargets,
  handleLongPress,
  handleSelect,
  toggleSelectAll,
  handleBatchClose,
} = useBatchSelection(() => subStore.comics)

// 初始化/重新加载订阅数据
const initSearch = () => {
  const cfg = subSearchConfig.value
  subStore.fetchInitial({
    keyword: cfg.keyword || '',
    categories: cfg.activeCategories,
  })
}

// 监听订阅检索配置变更（如搜索框输入、分类勾选）
watch(
  subSearchConfig,
  () => {
    initSearch()
  },
  { deep: true },
)

// Round7-任务8：恢复/记忆列表滚动位置 + 注册列表状态提供者（无限滚动，page 恒为 1）
const restoreListState = async () => {
  const saved = takeListState('/online/sub')
  if (saved && saved.top > 0) {
    await nextTick()
    requestAnimationFrame(() => {
      const el = getMainContent()
      if (el && el.scrollHeight > 0) el.scrollTop = saved.top
    })
  }
  setListStateProvider('/online/sub', () => ({
    top: getMainContent()?.scrollTop || 0,
    page: 1,
  }))
}

onMounted(() => {
  initSearch()
  restoreListState()
})

// keep-alive 缓存下「同标签返回」只触发 onActivated，同样恢复列表状态
let activatedOnce = false
onActivated(() => {
  if (activatedOnce) restoreListState()
  activatedOnce = true
})

onBeforeRouteLeave(() => {
  rememberListState('/online/sub', {
    top: getMainContent()?.scrollTop || 0,
    page: 1,
  })
})
</script>

<template>
  <div class="page-wrapper">
    <div class="online-split" :class="{ 'panel-open': isPanelOpen }">
      <div class="split-main">
        <GridContainer
          :items="subStore.comics"
          :selectable="true"
          :select-mode="selectMode"
          :selected-ids="selectedIds"
          :panel-mode="isWide"
          :panel-open="isPanelOpen"
          @longpress="handleLongPress"
          @select="handleSelect"
          @open="openDetail"
        >
          <!-- 1. 顶部插槽：存在向上游标时，显示加载较新内容按钮 -->
          <template #header>
            <div v-if="subStore.prevGid" class="top-load-bar">
              <button class="pill-btn" :disabled="subStore.isLoading" @click="subStore.loadBefore">
                ⬆️ {{ subStore.isLoading ? '加载中...' : '加载较新内容' }}
              </button>
            </div>
          </template>

          <!-- 2. 底部插槽：向下滑动流式加载 -->
          <template #footer>
            <OnlineLoadBar
              :is-loading="subStore.isLoading"
              :has-more="subStore.hasMore"
              :error="subStore.error"
              @load-more="subStore.loadMore"
            />
          </template>
        </GridContainer>

        <!-- 右下角悬浮操作球：支持手动刷新与按日期跳转 (seek) + 详情页面切换 -->
        <FloatingToolbar
          :show-detail="isWide"
          @refresh="initSearch"
          @seek-change="(date) => subStore.seekToDate(date)"
          @detail-toggle="togglePanel"
        />

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
        @close="closePanel"
      />
    </div>
  </div>
</template>

<style scoped>
.page-wrapper {
  padding: 12px 4px;
  min-height: 100%;
}

.top-load-bar {
  padding: 8px 0;
  text-align: center;
}

.pill-btn {
  background: transparent;
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
  border-radius: 20px;
  padding: 6px 18px;
  font-size: 0.82rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.pill-btn:hover:not(:disabled) {
  border-color: #00a896;
  color: var(--app-text-strong);
}

.pill-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
