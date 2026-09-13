<script setup lang="ts">
import { ref, watch, computed, onMounted, onUnmounted, onActivated, nextTick } from 'vue'
import { useRoute, onBeforeRouteLeave } from 'vue-router'
import GridContainer from '@/components/GridContainer.vue'
import OnlineLoadBar from '@/components/OnlineLoadBar.vue'
import FloatingToolbar from '@/components/FloatingToolbar.vue' // 👈 引入悬浮球
import BatchDownloadBar from '@/components/BatchDownloadBar.vue'
import OnlineDetailPanel from '@/components/OnlineDetailPanel.vue'
import BookmarkCreateModal from '@/components/BookmarkCreateModal.vue' // Round27：搜刮书签创建弹窗
import { useOnlineStore } from '@/stores/onlineStore'
import { onlineSearchConfig, applySearchOptionsInherit } from '@/stores/searchStore'
import {
  addScrapeBookmark,
  bookmarkedGids,
  getBookmarkById,
  snapshotOnlineSearchConfig,
  cloneSearchConfig,
  markAnchorFailed,
  bookmarkLocationLabel,
} from '@/stores/scrapeBookmarksStore'
import type { ScrapeBookmark } from '@/types/comic'
import { useBatchSelection } from '@/composables/useBatchSelection'
import { useDetailPanel } from '@/composables/useDetailPanel'
import { useUI } from '@/composables/useUI'
// Round33：书签跳转失败时的单条失效探测
import { useBookmarkCheck } from '@/composables/useBookmarkCheck'
// Round33：检索参数构造（与首页搜索共用同一实现）
import { buildOnlineSearchParams } from '@/utils/onlineSearchParams'
// Round3-任务6：负向排除（在线端"抓取后本地丢弃"）
import { matchExcludes, parseKeywordQueue } from '@/utils/tagFilter'
// Round7-任务8：列表状态记忆 + 提供者（新标签返回本页恢复滚动位置）
import {
  rememberListState,
  takeListState,
  setListStateProvider,
  getMainContent,
} from '@/utils/scrollMemory'

const onlineStore = useOnlineStore()
const { toast } = useUI()

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

// ─── Round27：书签跳转待定位锚点（消费后清空；数据就绪时滚动定位 + 脉冲高亮）───
const pendingAnchorGid = ref<string | null>(null)
/** Round33：待定位锚点所属书签 id（定位失败时用于单条失效探测） */
const pendingBookmarkId = ref<string | null>(null)

// 🆕 URL 驱动搜索：进入 /online/home?kw=xxx（新标签页/分享链接等）时，把关键词写入搜索配置
// Round27：?bm=<bookmarkId>（搜刮书签跳转）优先于 kw 分支——整体恢复创建时快照，
// 不 applySearchOptionsInherit（书签要完整还原当时筛选状态，而非按偏好重置）。
// 必须在下方 watch 注册之前执行，避免初始设置触发一次多余搜索。
const kwFromUrl = route.query.kw
const bmFromUrl = route.query.bm
if (typeof bmFromUrl === 'string' && bmFromUrl.trim()) {
  const bm = getBookmarkById(bmFromUrl.trim())
  if (bm) {
    onlineSearchConfig.value = cloneSearchConfig(bm.config)
    pendingAnchorGid.value = bm.anchor?.gid || null
    pendingBookmarkId.value = bm.id // Round33：定位失败时据此做单条失效探测
    // 消费后清理 URL 上的 bm 参数（replaceState 不触发路由/keep-alive 重建）：
    // 避免残留 bm 在刷新/后续搜索时把列表重新拉回书签状态
    const url = new URL(window.location.href)
    if (url.searchParams.has('bm')) {
      url.searchParams.delete('bm')
      window.history.replaceState(null, '', url.pathname + url.search)
    }
  } else if (typeof kwFromUrl === 'string' && kwFromUrl.trim()) {
    // 书签已被删除：退化为普通 kw 搜索
    applySearchOptionsInherit('online')
    onlineSearchConfig.value.keyword = kwFromUrl.trim()
  }
} else if (typeof kwFromUrl === 'string' && kwFromUrl.trim()) {
  applySearchOptionsInherit('online')
  onlineSearchConfig.value.keyword = kwFromUrl.trim()
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
  // Round33：参数构造抽为公共函数（书签时间锚迁移复用同一套检索条件）
  onlineStore.fetchInitial(buildOnlineSearchParams(onlineSearchConfig.value))
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

// ─── Round27：书签锚点定位 ───
// 数据就绪（含 loadMore 追加）后查找锚定卡片：滚动到卡片中部 + 脉冲高亮一次
watch(
  () => onlineStore.comics,
  () => {
    const gid = pendingAnchorGid.value
    if (!gid) return
    nextTick(() => {
      const el = document.querySelector<HTMLElement>(`.item-card[data-gid="${gid}"]`)
      if (el) {
        el.scrollIntoView({ block: 'center', behavior: 'smooth' })
        el.classList.add('bookmark-pulse')
        window.setTimeout(() => el.classList.remove('bookmark-pulse'), 2600)
        pendingAnchorGid.value = null
      }
    })
  },
  { flush: 'post' },
)

// 兜底：列表已到尽头（hasMore=false）仍未定位到锚点
// Round28：明确提示 + 标记失效；Round33：改为「单条探测确认」，避免结果漂移被误判为失效
watch(
  () => onlineStore.hasMore,
  async (hasMore) => {
    if (!hasMore && pendingAnchorGid.value) {
      const gid = pendingAnchorGid.value
      const bmId = pendingBookmarkId.value
      pendingAnchorGid.value = null
      pendingBookmarkId.value = null
      markAnchorFailed(gid)
      if (bmId) {
        // 主动探测：区分「真的失效」与「仅不在当前搜索结果中」
        const { runCheck } = useBookmarkCheck()
        const summary = await runCheck([bmId])
        if (summary && summary.migrated > 0) {
          toast.info('锚点已迁移到同时刻的画廊；请从侧栏重新打开该书签')
        } else if (summary && summary.invalid === 0 && summary.errors === 0) {
          toast.info('锚定画廊仍有效，但不在当前搜索结果中（结果可能已变化）')
        }
        return
      }
      toast.warning('书签锚定的画廊已失效（可能被删除或更换），书签已保留，跳转仅恢复位置')
    }
  },
)

// ─── Round27/Round29：搜刮书签创建流程（锚定即创建）───
const bmModalOpen = ref(false)
const pickMode = ref(false) // 锚定卡片拾取模式
/** 已拾取待确认的锚定信息（点卡片后写入，弹窗确认时使用） */
const pendingAnchor = ref<ScrapeBookmark['anchor']>(null)

// 书签位置信息：类型 / 搜索词（基于当前生效搜索配置）
const bmType = computed<'home' | 'search'>(() =>
  onlineSearchConfig.value.keyword?.trim() ? 'search' : 'home',
)
const bmKeyword = computed(() => onlineSearchConfig.value.keyword?.trim() || '')
/** 位置标签（首页 / 搜索: xxx），与侧栏展示共用同一实现 */
const bmLocationLabel = computed(() =>
  bookmarkLocationLabel({ type: bmType.value, keyword: bmKeyword.value }),
)

// Round29：点「存为书签」直接进入拾取模式（弹窗推迟到选中卡片后出现）
const handleBookmarkCreate = () => {
  pendingAnchor.value = null
  bmModalOpen.value = false
  pickMode.value = true
}

// 创建书签（弹窗「确认」）：Round28 后端化为异步（乐观 + 失败回滚），失败返回 null 并已提示
const handleBookmarkSave = async (payload: { name: string }) => {
  const anchor = pendingAnchor.value
  if (!anchor) return // 未锚定不应走到这里（按钮已不提供纯位置保存）
  const bm = await addScrapeBookmark(
    payload.name,
    bmType.value,
    bmKeyword.value,
    snapshotOnlineSearchConfig(),
    anchor,
  )
  if (!bm) return // 保存失败已由 store toast 提示
  bmModalOpen.value = false
  pickMode.value = false
  pendingAnchor.value = null
  toast.success(`书签已保存（${bmLocationLabel.value}）`)
}

// 弹窗「取消」：回到拾取模式，可改锚定其他卡片
const handleBookmarkCancel = () => {
  bmModalOpen.value = false
  pendingAnchor.value = null
  pickMode.value = true
}

// 拾取模式点击拦截（capture 阶段：命中卡片 → 记录锚定并弹出确认窗；空白 → 退出拾取）
const handlePickClick = (e: MouseEvent) => {
  if (!pickMode.value) return
  const target = e.target as HTMLElement
  const card = target.closest?.('.item-card[data-gid]') as HTMLElement | null
  if (card) {
    e.stopPropagation()
    const gid = card.dataset.gid || ''
    const comic = filteredComics.value.find((c) => c.id === gid)
    // Round33：记录卡片在列表中的位置（老书签无发布时间时的迁移兜底）
    const listIndex = filteredComics.value.findIndex((c) => c.id === gid)
    // Round29：自动记录锚定画廊的发布时间（E 站 posted 日期 → OnlineComic.updatedAt）
    pendingAnchor.value = comic
      ? {
          gid,
          token: comic.source === 'online' ? comic.token : undefined,
          title: comic.title,
          postedAt: comic.updatedAt || undefined,
          listIndex: listIndex >= 0 ? listIndex : undefined,
        }
      : { gid }
    // 弹窗此时才出现（展示自动记录的位置 + 发布时间）
    pickMode.value = false
    bmModalOpen.value = true
    return
  }
  // 点击空白：退出拾取
  pickMode.value = false
}

// Round7-任务8：恢复/记忆列表滚动位置 + 注册列表状态提供者（无限滚动，page 恒为 1）
const restoreListState = async () => {
  const saved = takeListState('/online/home')
  if (saved && saved.top > 0) {
    await nextTick()
    requestAnimationFrame(() => {
      const el = getMainContent()
      if (el && el.scrollHeight > 0) el.scrollTop = saved.top
    })
  }
  setListStateProvider('/online/home', () => ({
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
  rememberListState('/online/home', {
    top: getMainContent()?.scrollTop || 0,
    page: 1,
  })
})

onUnmounted(() => {
  if (writeBackTimer) clearTimeout(writeBackTimer)
})
</script>

<template>
  <!-- @click.capture：拾取模式下拦截点击（命中卡片→锚定；空白→取消），
       capture 阶段 stopPropagation 可阻止 ItemCard 自身 click 导航 -->
  <div class="online-home-view" @click.capture="handlePickClick">
    <div class="online-split" :class="{ 'panel-open': isPanelOpen, picking: pickMode }">
      <div class="split-main">
        <GridContainer
          :items="filteredComics"
          :selectable="!pickMode"
          :select-mode="selectMode"
          :selected-ids="selectedIds"
          :panel-mode="isWide"
          :panel-open="isPanelOpen"
          :bookmarked-gids="bookmarkedGids"
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
          :show-bookmark="true"
          @refresh="initSearch"
          @seek-change="(date) => onlineStore.seekToDate(date)"
          @detail-toggle="togglePanel"
          @bookmark-create="handleBookmarkCreate"
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

    <!-- Round27：拾取模式提示条（pointer-events:none，不拦截点击） -->
    <Transition name="bm-fade">
      <div v-if="pickMode" class="pick-hint" aria-hidden="true">
        🎯 点击要锚定的画廊卡片（点击空白处取消）
      </div>
    </Transition>

    <!-- Round27/Round29：搜刮书签确认弹窗（点卡片后出现，自动带出位置 + 发布时间） -->
    <BookmarkCreateModal
      :show="bmModalOpen"
      :type="bmType"
      :keyword="bmKeyword"
      :location-label="bmLocationLabel"
      :anchor-title="pendingAnchor?.title"
      :posted-at="pendingAnchor?.postedAt"
      @close="handleBookmarkCancel"
      @create="handleBookmarkSave"
    />
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

/* ─── Round27：拾取模式视觉 ─── */
/* 拾取提示条：悬浮于列表上方，不拦截点击 */
.pick-hint {
  position: fixed;
  top: calc(16px + var(--safe-top));
  left: 50%;
  transform: translateX(-50%);
  z-index: 1500;
  pointer-events: none;
  background-color: rgba(0, 0, 0, 0.82);
  border: 1px solid #ffc107;
  color: #ffd54f;
  font-size: 0.85rem;
  font-weight: 600;
  padding: 8px 18px;
  border-radius: 20px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.5);
  white-space: nowrap;
}

/* 拾取模式下所有卡片显示金色虚线框，提示可点选 */
.online-split.picking :deep(.item-card) {
  outline: 2px dashed rgba(255, 193, 7, 0.75);
  outline-offset: 2px;
  box-shadow: 0 0 0 4px rgba(255, 193, 7, 0.12);
}

.bm-fade-enter-active,
.bm-fade-leave-active {
  transition: opacity 0.18s ease;
}
.bm-fade-enter-from,
.bm-fade-leave-to {
  opacity: 0;
}
</style>
