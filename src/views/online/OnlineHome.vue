<script setup lang="ts">
import { watch, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import GridContainer from '@/components/GridContainer.vue'
import OnlineLoadBar from '@/components/OnlineLoadBar.vue'
import FloatingToolbar from '@/components/FloatingToolbar.vue' // 👈 引入悬浮球
import BatchDownloadBar from '@/components/BatchDownloadBar.vue'
import OnlineDetailPanel from '@/components/OnlineDetailPanel.vue'
import { useOnlineStore } from '@/stores/onlineStore'
import { onlineSearchConfig } from '@/stores/searchStore'
import { useBatchSelection } from '@/composables/useBatchSelection'
import { useDetailPanel } from '@/composables/useDetailPanel'
// Round3-任务6：负向排除（在线端"抓取后本地丢弃"）
import { matchExcludes, parseKeywordQueue } from '@/utils/tagFilter'

const onlineStore = useOnlineStore()

// ─── Round3-任务6：在线列表负向过滤（负向项不参与服务端搜索，仅渲染前本地剔除）───
const filteredComics = computed(() => {
  const cfg = onlineSearchConfig.value
  const parsed = parseKeywordQueue(cfg.keywords)
  // Round3-任务6：顶栏主搜索词同样支持「- 」负向前缀（并入本地负向规则）
  const searchBarParsed = parseKeywordQueue(cfg.keyword?.trim() ? [cfg.keyword] : [])
  const rule = {
    excludeTags: [
      ...(cfg.excludeTags || []),
      ...parsed.excludeTags,
      ...searchBarParsed.excludeTags,
    ],
    excludeKeywords: [
      ...(cfg.excludeKeywords || []),
      ...parsed.excludeKeywords,
      ...searchBarParsed.excludeKeywords,
    ],
  }
  return onlineStore.comics.filter((comic) => matchExcludes(comic, rule))
})

const route = useRoute()
// 左右分栏详情面板（宽屏桌面生效；窄屏回退全屏详情路由）
const { isWide, isPanelOpen, panelGid, panelToken, openDetail, closePanel, togglePanel } =
  useDetailPanel()

// 🆕 URL 驱动搜索：进入 /online/home?kw=xxx（新标签页/分享链接等）时，把关键词写入搜索配置
// 必须在下方 watch 注册之前执行，避免初始设置触发一次多余搜索
const kwFromUrl = route.query.kw
if (typeof kwFromUrl === 'string' && kwFromUrl.trim()) {
  onlineSearchConfig.value.keyword = kwFromUrl.trim()
  onlineSearchConfig.value.keywords = []
}

// 长按多选 → 批量下载
const {
  selectMode,
  selectedIds,
  selectedTargets,
  handleLongPress,
  handleSelect,
  toggleSelectAll,
  handleBatchClose,
} = useBatchSelection(() => filteredComics.value)

const initSearch = () => {
  const cfg = onlineSearchConfig.value
  // E-Hentai 的 f_search 支持空格分隔多词（隐式 AND），因此把
  // 顶栏主搜索词与筛选抽屉的“多关键词队列”合并为一条 f_search 字符串。
  // Round3-任务6：负向项（`- ` 前缀）不参与服务端搜索（E 站不支持排除语法），
  // 只保留正向词下发，负向剔除交由 filteredComics 本地完成。
  const parsed = parseKeywordQueue(cfg.keywords)
  // Round3-任务6：顶栏主搜索词的「- 」负向部分不参与服务端搜索（E 站不支持排除语法），只取正向词
  const searchBarParsed = parseKeywordQueue(cfg.keyword?.trim() ? [cfg.keyword] : [])
  const kwTokens = [...searchBarParsed.positive, ...parsed.positive]
    .map((t) => t.trim())
    .filter(Boolean)
  onlineStore.fetchInitial({
    keyword: kwTokens.join(' '),
    categories: cfg.activeCategories,
    // ─── E-Hentai 高级筛选全量下发 ───
    minRating: cfg.minRating,
    language: cfg.language,
    onlyRemoved: cfg.onlyRemoved,
    onlyTorrents: cfg.onlyTorrents,
    disableLangFilter: cfg.disableLangFilter,
    disableUploaderFilter: cfg.disableUploaderFilter,
    disableTagFilter: cfg.disableTagFilter,
  })
}

// 🆕 把当前关键词写回 URL（history.replaceState）
// 不能用 router.replace：keep-alive 按 $route.fullPath 缓存，query 变化会触发组件重建
// → 滚动/面板状态丢失 + 重复请求；replaceState 只改地址栏，不触发重建
let writeBackTimer: ReturnType<typeof setTimeout> | null = null
const writeKeywordToUrl = () => {
  if (writeBackTimer) clearTimeout(writeBackTimer)
  writeBackTimer = setTimeout(() => {
    if (route.path !== '/online/home') return
    const kw = onlineSearchConfig.value.keyword?.trim() || ''
    const url = new URL(window.location.href)
    if (kw) {
      url.searchParams.set('kw', kw)
    } else {
      url.searchParams.delete('kw')
    }
    window.history.replaceState(null, '', url.pathname + url.search)
  }, 600)
}

watch(
  onlineSearchConfig,
  () => {
    initSearch()
    writeKeywordToUrl()
  },
  { deep: true },
)

onMounted(() => {
  initSearch()
})

onUnmounted(() => {
  if (writeBackTimer) clearTimeout(writeBackTimer)
})
</script>

<template>
  <div class="online-home-view">
    <div class="online-split" :class="{ 'panel-open': isPanelOpen }">
      <div class="split-main">
        <GridContainer
          :items="filteredComics"
          :selectable="true"
          :select-mode="selectMode"
          :selected-ids="selectedIds"
          :panel-mode="isWide"
          :panel-open="isPanelOpen"
          @longpress="handleLongPress"
          @select="handleSelect"
          @open="openDetail"
        >
          <!-- 🟢 1. 顶部插槽：存在向上游标时，显示加载较新内容按钮 -->
          <template #header>
            <div v-if="onlineStore.prevGid" class="top-load-bar">
              <button
                class="pill-btn"
                :disabled="onlineStore.isLoading"
                @click="onlineStore.loadBefore"
              >
                ⬆️ {{ onlineStore.isLoading ? '加载中...' : '加载较新内容' }}
              </button>
            </div>
          </template>

          <!-- 2. 底部插槽：向下滑动流式加载 -->
          <template #footer>
            <OnlineLoadBar
              :is-loading="onlineStore.isLoading"
              :has-more="onlineStore.hasMore"
              :error="onlineStore.error"
              @load-more="onlineStore.loadMore"
            />
          </template>
        </GridContainer>

        <!-- 右下角悬浮操作球 -->
        <FloatingToolbar
          :show-detail="isWide"
          @refresh="initSearch"
          @seek-change="(date) => onlineStore.seekToDate(date)"
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
.online-home-view {
  padding: 12px 4px;
  min-height: 100%;
}

.top-load-bar {
  padding: 8px 0;
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
