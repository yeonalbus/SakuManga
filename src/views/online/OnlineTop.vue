<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/Pagination.vue'
import FloatingToolbar from '@/components/FloatingToolbar.vue'
import BatchDownloadBar from '@/components/BatchDownloadBar.vue'
import OnlineDetailPanel from '@/components/OnlineDetailPanel.vue'
import type { OnlineComic } from '@/types/comic'
import { useUI } from '@/composables/useUI'
import { useBatchSelection } from '@/composables/useBatchSelection'
import { useDetailPanel } from '@/composables/useDetailPanel'
import { http } from '@/utils/request'

interface RankedOnlineComic extends OnlineComic {
  score: number
  rank: number
}

// 排行榜类型（与后端 toplist.php 的 tl 参数对应；默认 Yesterday tl=15）
const TOPLIST_TYPES: { tl: string; label: string }[] = [
  { tl: '15', label: 'Galleries Yesterday' },
  { tl: '13', label: 'Galleries Past Month' },
  { tl: '12', label: 'Galleries Past Year' },
  { tl: '11', label: 'Galleries All-Time' },
]

const { toast } = useUI()

// 左右分栏详情面板（宽屏桌面生效；窄屏回退全屏详情路由）
const { isWide, isPanelOpen, panelGid, panelToken, openDetail, closePanel, togglePanel } =
  useDetailPanel()

const comics = ref<RankedOnlineComic[]>([])
const isLoading = ref(true)
const currentTl = ref('15') // 当前排行榜类型（默认 Yesterday）
const currentPage = ref(1)
const totalPages = ref(1)

// 当前排行榜类型显示标签
const currentLabel = computed(
  () => TOPLIST_TYPES.find((t) => t.tl === currentTl.value)?.label ?? 'Galleries Yesterday',
)

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
} = useBatchSelection(() => comics.value)

// 从 Go 后端按 (tl, page) 读取排行榜（每页 50 条，1~200 页）
const fetchToplist = async () => {
  exitSelectMode()
  isLoading.value = true
  try {
    const data = await http<{
      comics: RankedOnlineComic[]
      totalPages: number
      currentPage: number
    }>(`/comics/online/toplist?tl=${currentTl.value}&page=${currentPage.value}`)

    comics.value = data.comics || []
    totalPages.value = data.totalPages || 1
  } catch (err) {
    toast.error('网络连接失败')
    console.error(err)
  } finally {
    isLoading.value = false
  }
}

// 分页组件触发切页
const handlePageChange = (page: number) => {
  if (page === currentPage.value) return
  currentPage.value = page
  fetchToplist()
}

// 操作菜单「排行榜选择」切换类型：重置到第 1 页重新加载
const handleToplistSelect = (tl: string) => {
  if (tl === currentTl.value) return
  currentTl.value = tl
  currentPage.value = 1
  fetchToplist()
}

// 操作菜单「刷新列表」
const handleRefresh = () => {
  fetchToplist()
}

onMounted(() => {
  fetchToplist()
})
</script>

<template>
  <div class="leaderboard-page">
    <h2 class="page-title">🏆 官方排行榜 · {{ currentLabel }}</h2>

    <div class="online-split" :class="{ 'panel-open': isPanelOpen }">
      <div class="split-main">
        <div v-if="isLoading" class="loading-state">加载排行榜中...</div>

        <template v-else-if="comics.length > 0">
          <GridContainer
            :items="comics"
            :selectable="true"
            :select-mode="selectMode"
            :selected-ids="selectedIds"
            :panel-mode="isWide"
            :panel-open="isPanelOpen"
            @longpress="handleLongPress"
            @select="handleSelect"
            @open="openDetail"
          >
            <!-- 底部：离线式页码分页（1~200，每页 50 条） -->
            <template #footer>
              <Pagination
                :current-page="currentPage"
                :total-pages="totalPages"
                @change="handlePageChange"
              />
            </template>
          </GridContainer>
        </template>

        <div v-else class="empty-tip">暂无榜单数据</div>

        <!-- 右下角悬浮操作球：排行榜选择（替换日期跳转）+ 详情页面 -->
        <FloatingToolbar
          :show-toplist="true"
          :toplist-current="currentTl"
          :show-detail="true"
          @toplist-select="handleToplistSelect"
          @detail-toggle="togglePanel"
          @refresh="handleRefresh"
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
.leaderboard-page {
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding-bottom: 30px;
}

.loading-state,
.empty-tip {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 240px;
  color: var(--app-text-3);
}

.page-title {
  font-size: 1.3rem;
  color: var(--app-text-strong);
}

/* 📱 窄屏适配 */
@media (max-width: 767px) {
  .leaderboard-page {
    padding: 0 12px 24px;
    gap: 16px;
  }

  .page-title {
    font-size: 1.05rem;
  }
}
</style>
