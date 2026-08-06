<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import OnlineLoadBar from '@/components/OnlineLoadBar.vue'
import FloatingToolbar from '@/components/FloatingToolbar.vue'
import BatchDownloadBar from '@/components/BatchDownloadBar.vue'
import type { OnlineComic } from '@/types/comic'
import { useUI } from '@/composables/useUI'
import { useBatchSelection } from '@/composables/useBatchSelection'
import { batchCreateDownloads } from '@/api/download'
import { http } from '@/utils/request'
import OnlineDetailPanel from '@/components/OnlineDetailPanel.vue'
import { useDetailPanel } from '@/composables/useDetailPanel'

const { toast } = useUI()

// 左右分栏详情面板（宽屏桌面生效；窄屏回退全屏详情路由）
const { isWide, isPanelOpen, panelGid, panelToken, openDetail, closePanel } = useDetailPanel()

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
} = useBatchSelection(() => favComicList.value)

// 「一键全部下载」当前已加载的收藏
const isDownloadingAll = ref(false)
const downloadAllFavorites = async () => {
  if (isDownloadingAll.value || favComicList.value.length === 0) return
  isDownloadingAll.value = true
  try {
    const targets = favComicList.value
      .filter((c) => !!c.token)
      .map((c) => ({ gid: c.id, token: c.token!, title: c.title, coverUrl: c.coverUrl }))
    if (targets.length === 0) {
      toast.warning('当前收藏夹中没有可下载的作品')
      return
    }
    const res = await batchCreateDownloads(targets)
    if (res.failed > 0) {
      toast.error(`加入失败 ${res.failed} 部：${(res.errors || []).join('；')}`)
    } else if (res.skipped > 0) {
      toast.success(`成功加入 ${res.created} 部，跳过已存在的 ${res.skipped} 部`)
    } else {
      toast.success(`已将 ${res.created} 部收藏加入下载队列`)
    }
  } catch (err) {
    toast.error(`一键全部下载失败：${(err as Error)?.message || '未知错误'}`)
  } finally {
    isDownloadingAll.value = false
  }
}

const activeFav = ref(0)
const favComicList = ref<OnlineComic[]>([])
const isLoading = ref(false)
const hasMore = ref(true)
const nextGid = ref<string>('')
const prevGid = ref<string>('')
const errorMsg = ref<string | null>(null)

// 🟢 排序模式: 'favorited' (按收藏时间) | 'published' (按发布时间)
const sortMode = ref<'favorited' | 'published'>('favorited')

const favColors = [
  '#7f7f7f',
  '#f00000',
  '#ff7800',
  '#f0d000',
  '#00a0a0',
  '#98e020',
  '#00a0a0',
  '#0000f0',
  '#a000a0',
  '#f000a0',
]

// 🟢 1. 对标 JHenTai: handleChangeSortOrder (切换排序并完全重置)
const handleChangeSortOrder = async (newMode: 'favorited' | 'published') => {
  if (isLoading.value || sortMode.value === newMode) return

  isLoading.value = true

  try {
    // 🟢 1. 先调用独立的后端接口触发 E 站 Session/Cookie 变更
    await http('/comics/online/favorites/sort', {
      method: 'POST',
      body: JSON.stringify({ sortMode: newMode }),
      headers: {
        'Content-Type': 'application/json',
      },
    })

    sortMode.value = newMode
    toast.success(`排序已切换为：${newMode === 'favorited' ? '按收藏时间' : '按发布时间'}`)

    // 🟢 2. 对标 JHenTai: 完全重置本地状态与游标
    favComicList.value = []
    nextGid.value = ''
    prevGid.value = ''
    hasMore.value = true

    // 🟢 3. 视图置顶
    const mainEl = document.querySelector('.main-content')
    if (mainEl) {
      mainEl.scrollTo({ top: 0, behavior: 'smooth' })
    }

    // 🟢 4. 发起普通的收藏夹列表查询 (此时 E 站服务端已按新 Cookie 生效)
    await fetchFavInitial()
  } catch (err: unknown) {
    toast.error('切换排序失败: ' + (err instanceof Error ? err.message : '未知错误'))
  } finally {
    isLoading.value = false
  }
}

// 2. 初始化 / 全新拉取第一页
const fetchFavInitial = async (seekDate?: string) => {
  exitSelectMode()
  isLoading.value = true
  errorMsg.value = null
  favComicList.value = []
  nextGid.value = ''
  prevGid.value = ''
  hasMore.value = true

  try {
    const query = new URLSearchParams({
      favcat: activeFav.value.toString(),
      sort: sortMode.value, // 🟢 明确传递当前排序模式
    })
    if (seekDate) {
      query.append('seek', seekDate)
    }

    const data = await http<{
      comics: OnlineComic[]
      next?: string
      prev?: string
      hasMore?: boolean
    }>(`/comics/online/favorites?${query.toString()}`)

    favComicList.value = data.comics || []
    nextGid.value = cleanCursor(data.next, false)
    prevGid.value = cleanCursor(data.prev, true)
    hasMore.value = data.hasMore ?? !!nextGid.value
  } catch (err: unknown) {
    errorMsg.value = err instanceof Error ? err.message : '获取收藏夹数据失败'
    toast.error('网络连接失败')
  } finally {
    isLoading.value = false
  }
}

// 3. 向下滑动加载更多 (Load More)
const loadMoreFav = async () => {
  if (isLoading.value || !hasMore.value || !nextGid.value) return

  isLoading.value = true
  errorMsg.value = null

  try {
    const query = new URLSearchParams({
      favcat: activeFav.value.toString(),
      next: nextGid.value,
      sort: sortMode.value, // 🟢 补全 sort
    })

    const data = await http<{
      comics: OnlineComic[]
      next?: string
      prev?: string
      hasMore?: boolean
    }>(`/comics/online/favorites?${query.toString()}`)

    favComicList.value.push(...(data.comics || []))
    nextGid.value = cleanCursor(data.next, false)
    hasMore.value = data.hasMore ?? !!data.next
  } catch (err: unknown) {
    errorMsg.value = err instanceof Error ? err.message : '加载更多失败'
    toast.error('加载更多失败')
  } finally {
    isLoading.value = false
  }
}

// 4. 向上加载较新内容 (Load Before)
const loadBeforeFav = async () => {
  if (isLoading.value || !prevGid.value) return

  isLoading.value = true
  errorMsg.value = null

  try {
    const query = new URLSearchParams({
      favcat: activeFav.value.toString(),
      prev: prevGid.value,
      sort: sortMode.value, // 🟢 补全 sort
    })

    const data = await http<{
      comics: OnlineComic[]
      next?: string
      prev?: string
      hasMore?: boolean
    }>(`/comics/online/favorites?${query.toString()}`)

    const incoming = data.comics || []
    const existingIds = new Set(favComicList.value.map((c) => c.id))
    const uniqueIncoming = incoming.filter((c) => !existingIds.has(c.id))

    if (uniqueIncoming.length > 0) {
      favComicList.value.unshift(...uniqueIncoming)
    }

    prevGid.value = cleanCursor(data.prev, true)
  } catch (err: unknown) {
    errorMsg.value = err instanceof Error ? err.message : '加载较新内容失败'
    toast.error('加载较新内容失败')
  } finally {
    isLoading.value = false
  }
}

watch(activeFav, () => {
  // 切换收藏夹分类时同样重置游标
  nextGid.value = ''
  prevGid.value = ''
  fetchFavInitial()
})

const selectFav = (favIndex: number) => {
  activeFav.value = favIndex
}

const cleanCursor = (cursor?: string, isPrev = false): string => {
  if (!cursor || cursor === '0-0' || cursor === '0') {
    return ''
  }
  // 只有向后翻页（next）时，"1-0" 才是无效边界；向前翻页（prev）时 "1-0" 是有效游标
  if (!isPrev && cursor === '1-0') {
    return ''
  }
  return cursor
}

onMounted(() => {
  fetchFavInitial()
})
</script>

<template>
  <div class="favorites-page">
    <div class="fav-grid">
      <button
        v-for="i in 10"
        :key="i - 1"
        class="fav-btn"
        :class="{ active: activeFav === i - 1 }"
        :style="{
          backgroundColor: activeFav === i - 1 ? favColors[i - 1] : 'var(--app-surface-2)',
          borderColor: activeFav === i - 1 ? favColors[i - 1] : 'var(--app-border-2)',
          color: activeFav === i - 1 ? '#ffffff' : 'var(--app-text-2)',
        }"
        @click="selectFav(i - 1)"
      >
        <span class="fav-dot" :style="{ backgroundColor: favColors[i - 1] }"></span>
        Favorites {{ i - 1 }}
      </button>
    </div>

    <!-- 一键全部下载当前已加载收藏 -->
    <div class="fav-actions">
      <button
        class="fav-all-btn"
        :disabled="isDownloadingAll || favComicList.length === 0"
        @click="downloadAllFavorites"
      >
        {{ isDownloadingAll ? '加入中…' : `⬇ 一键全部下载（${favComicList.length}）` }}
      </button>
    </div>

    <div class="online-split">
      <div class="split-main">
        <GridContainer
          v-if="favComicList.length > 0"
          :items="favComicList"
          :selectable="true"
          :select-mode="selectMode"
          :selected-ids="selectedIds"
          :panel-mode="isWide"
          @longpress="handleLongPress"
          @select="handleSelect"
          @open="openDetail"
        >
          <template #header>
            <div v-if="prevGid" class="top-load-bar">
              <button class="pill-btn" :disabled="isLoading" @click="loadBeforeFav">
                ⬆️ {{ isLoading ? '加载中...' : '加载较新内容' }}
              </button>
            </div>
          </template>

          <template #footer>
            <OnlineLoadBar
              :is-loading="isLoading"
              :has-more="hasMore"
              :error="errorMsg"
              @load-more="loadMoreFav"
            />
          </template>
        </GridContainer>

        <div v-else-if="isLoading" class="loading-state">加载收藏夹数据中...</div>
        <div v-else class="empty-tip">该收藏夹下暂无作品</div>

        <!-- 🟢 传入 showSort、sortMode，绑定 toggle-sort 事件 -->
        <FloatingToolbar
          :show-sort="true"
          :sort-mode="sortMode"
          @refresh="() => fetchFavInitial()"
          @seek-change="(date) => fetchFavInitial(date)"
          @toggle-sort="
            () => handleChangeSortOrder(sortMode === 'favorited' ? 'published' : 'favorited')
          "
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
.favorites-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
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

.fav-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 8px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
}

.fav-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid var(--app-border-2);
  border-radius: 6px;
  font-size: 0.82rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
  user-select: none;
}

.fav-btn:hover {
  transform: translateY(-1px);
  filter: brightness(1.15);
}

.fav-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

/* 🟢 顶部按钮栏样式 */
.top-load-bar {
  display: flex;
  justify-content: center;
  padding: 8px 0 16px;
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

.fav-actions {
  display: flex;
  justify-content: flex-start;
}

.fav-all-btn {
  background-color: #ff7588;
  border: 1px solid #ff7588;
  color: #ffffff;
  padding: 7px 16px;
  border-radius: 8px;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
}

.fav-all-btn:hover:not(:disabled) {
  background-color: #ff5f74;
  border-color: #ff5f74;
}

.fav-all-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
