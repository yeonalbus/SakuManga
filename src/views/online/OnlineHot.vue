<script setup lang="ts">
import { ref, onMounted, onActivated, nextTick } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import FloatingToolbar from '@/components/FloatingToolbar.vue'
import BatchDownloadBar from '@/components/BatchDownloadBar.vue'
import OnlineDetailPanel from '@/components/OnlineDetailPanel.vue'
import type { OnlineComic } from '@/types/comic'
import { useUI } from '@/composables/useUI'
import { useBatchSelection } from '@/composables/useBatchSelection'
import { useDetailPanel } from '@/composables/useDetailPanel'
import { http } from '@/utils/request'
// Round7-任务8：列表状态记忆 + 提供者（新标签返回本页恢复滚动位置）
import { onBeforeRouteLeave } from 'vue-router'
import {
  rememberListState,
  takeListState,
  setListStateProvider,
  getMainContent,
} from '@/utils/scrollMemory'

const { toast } = useUI()
const hotComics = ref<OnlineComic[]>([])
const isLoading = ref(true)

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
  exitSelectMode,
} = useBatchSelection(() => hotComics.value)

// 拉取真实热门数据
const fetchPopularComics = async () => {
  exitSelectMode()
  isLoading.value = true
  try {
    const data = await http<{ comics: OnlineComic[] }>('/comics/online/popular')
    hotComics.value = data.comics || []
  } catch (err) {
    toast.error('网络连接失败')
    console.error(err)
  } finally {
    isLoading.value = false
  }
}

// Round7-任务8：恢复/记忆列表滚动位置 + 注册列表状态提供者（无分页，page 恒为 1）
const restoreListState = async () => {
  const saved = takeListState('/online/hot')
  if (saved && saved.top > 0) {
    await nextTick()
    requestAnimationFrame(() => {
      const el = getMainContent()
      if (el && el.scrollHeight > 0) el.scrollTop = saved.top
    })
  }
  setListStateProvider('/online/hot', () => ({
    top: getMainContent()?.scrollTop || 0,
    page: 1,
  }))
}

onMounted(() => {
  fetchPopularComics()
  restoreListState()
})

// keep-alive 缓存下「同标签返回」只触发 onActivated，同样恢复列表状态
let activatedOnce = false
onActivated(() => {
  if (activatedOnce) restoreListState()
  activatedOnce = true
})

onBeforeRouteLeave(() => {
  rememberListState('/online/hot', {
    top: getMainContent()?.scrollTop || 0,
    page: 1,
  })
})
</script>

<template>
  <div class="page-wrapper">
    <div class="online-split" :class="{ 'panel-open': isPanelOpen }">
      <div class="split-main">
        <!-- 仅在首次无数据且加载中时显示全局居中 Loading -->
        <div v-if="isLoading && hotComics.length === 0" class="loading-state">
          正在拉取全站热门...
        </div>

        <!-- 列表展示 -->
        <GridContainer
          v-else-if="hotComics.length > 0"
          :items="hotComics"
          :selectable="true"
          :select-mode="selectMode"
          :selected-ids="selectedIds"
          :panel-mode="isWide"
          :panel-open="isPanelOpen"
          @longpress="handleLongPress"
          @select="handleSelect"
          @open="openDetail"
        />

        <!-- 空数据状态 -->
        <div v-else class="empty-tip">暂无热门数据</div>

        <!-- 右下角悬浮球：提供一键刷新与回到顶部 + 详情页面切换 -->
        <FloatingToolbar
          :show-detail="isWide"
          @refresh="fetchPopularComics"
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

.loading-state,
.empty-tip {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 300px;
  color: var(--app-text-3);
}
</style>
