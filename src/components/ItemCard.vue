<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { viewMode } from '@/stores/viewMode'
import { useRouter } from 'vue-router'
import type { ComicItem, OnlineComic, OfflineComic, CardViewMode } from '@/types/comic'
import { addHistory } from '@/stores/historyStore'
import TagChip from '@/components/TagChip.vue'
import { createDownloadTask } from '@/api/download'
import {
  isGidDownloading,
  markGidActive,
  subscribeActiveDownloads,
} from '@/stores/downloadTasksStore'
import { useUI } from '@/composables/useUI'

// 恢复 const props 变量定义，并补回 mode 与 size
// 新增选择相关 props：selectable=是否允许长按进入选择；selectMode=是否处于选择模式；selected=是否被选中
// 🟢 修复名片模式失效：mode 不再设置默认值，否则 props.mode 恒为 'card'，
// 导致下方 props.mode || viewMode.value 永远走卡片模式，全局 viewMode(compact) 无法生效。
const props = withDefaults(
  defineProps<{
    comic: ComicItem
    mode?: CardViewMode
    size?: 'large' | 'normal' | 'small'
    selectable?: boolean
    selectMode?: boolean
    selected?: boolean
    /** 左右分栏面板模式：宽屏在线列表页开启后，点击卡片改发 open 事件而非跳转路由 */
    panelMode?: boolean
    /** Round3-任务3：隐藏副标题（排行榜等场景只保留单一标题，日文优先） */
    hideSubtitle?: boolean
  }>(),
  {
    size: 'normal',
    selectable: false,
    selectMode: false,
    selected: false,
    panelMode: false,
    hideSubtitle: false,
  },
)

const emit = defineEmits<{
  (e: 'longpress', comic: ComicItem): void
  (e: 'select', comic: ComicItem): void
  (e: 'open', comic: ComicItem): void
}>()

const router = useRouter()
const { toast } = useUI()

// 核心交互状态：是否展开 Tag 面板 (点击封面图切换)
const showTags = ref(false)

const toggleTags = () => {
  showTags.value = !showTags.value
}

// 当前生效的展示模式：显式传入 mode 时优先（如榜单大卡片），
// 未传时回退到全局 viewMode（支持 card/compact 切换）
const currentMode = computed(() => props.mode ?? viewMode.value)

// --------------------------------------------------
// 1. 分类 (Category) 经典 E 站调色盘映射
// --------------------------------------------------
const categoryColors: Record<string, string> = {
  Doujinshi: '#ff7588',
  Manga: '#ff9800',
  'Artist CG': '#e91e63',
  'Game CG': '#4caf50',
  Western: '#8bc34a',
  'Non-H': '#2196f3',
  'Image Set': '#3f51b5',
  Cosplay: '#9c27b0',
  'Asian Porn': '#9e9e9e',
  Misc: '#607d8b',
}

const getCategoryColor = (cat?: string) => {
  if (!cat) return '#607d8b'
  return categoryColors[cat] || '#607d8b'
}

// --------------------------------------------------
// 2. 经典 Fav 0 ~ 9 调色盘 (在线收藏夹)
// --------------------------------------------------
const favColors: Record<number, string> = {
  0: '#7f7f7f',
  1: '#f00000',
  2: '#ff7800',
  3: '#cbb000',
  4: '#00a000',
  5: '#00a0c0',
  6: '#0000f0',
  7: '#a000a0',
  8: '#505050',
  9: '#000000',
}

const onlineComic = computed(() => {
  return props.comic.source === 'online' ? (props.comic as OnlineComic) : null
})

// --------------------------------------------------
// 3. 长按选择（pointerdown 计时 600ms 触发）
// --------------------------------------------------
const LONG_PRESS_MS = 600
let longPressTimer: ReturnType<typeof setTimeout> | null = null
const longPressed = ref(false)

const clearLongPressTimer = () => {
  if (longPressTimer) {
    clearTimeout(longPressTimer)
    longPressTimer = null
  }
}

const handlePointerDown = () => {
  // 选择模式中不做长按检测；非可选中卡片不响应。
  // 在线与离线卡片均支持长按选择（批量下载/批量删除）
  if (props.selectMode || !props.selectable) return
  longPressed.value = false
  clearLongPressTimer()
  longPressTimer = setTimeout(() => {
    longPressed.value = true
    emit('longpress', props.comic)
  }, LONG_PRESS_MS)
}

const handlePointerUp = () => {
  clearLongPressTimer()
}

// --------------------------------------------------
// 4. 点击卡片主体：选择模式切换选中，否则跳转详情页
// --------------------------------------------------
// 🆕 中键 / Ctrl / Meta + 点击 → 新浏览器标签打开详情（web 原生优势）
const openInNewTab = () => {
  if (!props.comic?.id) return
  addHistory(props.comic)
  let url = ''
  if (props.comic.source === 'online') {
    const token = onlineComic.value?.token || ''
    url = router.resolve({ path: '/online/detail', query: { id: props.comic.id, token } }).href
  } else {
    url = router.resolve(`/offline/detail?id=${props.comic.id}`).href
  }
  window.open(url, '_blank')
}

const handleCardClick = (event?: MouseEvent) => {
  // 长按已触发选择 → 抑制随后的 click 导航
  if (longPressed.value) {
    longPressed.value = false
    return
  }

  // 选择模式 → 切换选中
  if (props.selectMode) {
    emit('select', props.comic)
    return
  }

  if (!props.comic || !props.comic.id) return

  // 🆕 中键 / Ctrl / Meta + 点击 → 新标签打开详情
  if (event && (event.button === 1 || event.ctrlKey || event.metaKey)) {
    event.preventDefault()
    openInNewTab()
    return
  }

  // 🆕 左右分栏面板模式（宽屏在线列表页）：交给父级打开内嵌详情面板
  if (props.panelMode && props.comic.source === 'online') {
    emit('open', props.comic)
    return
  }

  addHistory(props.comic)

  if (props.comic.source === 'online') {
    // 🟢 在线模式：传递 id (GID) 和 token
    const token = onlineComic.value?.token || ''
    router.push({
      path: '/online/detail',
      query: { id: props.comic.id, token },
    })
  } else {
    router.push(`/offline/detail?id=${props.comic.id}`)
  }
}

// --------------------------------------------------
// 4.1 在线卡片 hover 快捷下载（零弹窗，按「下载设置」默认方案直接入队）
// 仅在线且带 token 的卡片可用；选择模式中隐藏，避免与勾选框冲突
// --------------------------------------------------
const isQuickDownloading = ref(false)

// 该 gid 是否已有进行中下载任务（由 downloadTasksStore 轮询维护）
const isDownloading = computed(() => {
  if (props.comic.source !== 'online') return false
  return isGidDownloading(props.comic.id)
})

const canQuickDownload = computed(
  () =>
    props.comic.source === 'online' &&
    !!onlineComic.value?.token &&
    !props.selectMode &&
    !isDownloading.value,
)

const handleQuickDownload = async () => {
  if (!canQuickDownload.value || isQuickDownloading.value) return
  if (!onlineComic.value) return
  // 双重防护：轮询状态尚未同步前拦截重复点击
  if (isDownloading.value) {
    toast.info('该画廊已在下载中，请勿重复下载')
    return
  }
  isQuickDownloading.value = true
  try {
    await createDownloadTask(onlineComic.value)
    markGidActive(onlineComic.value.id)
    toast.success('已加入下载队列')
  } catch (err) {
    toast.error(`加入下载队列失败：${(err as Error)?.message || '未知错误'}`)
  } finally {
    isQuickDownloading.value = false
  }
}

// 名片模式下缩略图点击：选择模式切换选中，否则展开/收起 Tag
const handleThumbClick = () => {
  if (props.selectMode) {
    emit('select', props.comic)
    return
  }
  toggleTags()
}

// 统一解析 Tags，确保永远返回 string[] 数组
const normalizedTags = computed<string[]>(() => {
  if (Array.isArray(props.comic.tags)) {
    return props.comic.tags
  }
  if (typeof props.comic.tags === 'string') {
    try {
      const parsed = JSON.parse(props.comic.tags)
      if (Array.isArray(parsed)) return parsed
    } catch {
      return [props.comic.tags]
    }
  }
  return []
})

// 封面加载失败时的默认占位图
const defaultCover =
  'data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 24 24" fill="none" stroke="%2355555a" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M21 15l-5-5L5 21"/></svg>'

// 在线封面加载失败自动重试：加载失败不再直接换占位图跳过（否则需整页刷新才恢复）。
// 反复请求需规避 E 站反爬——采用递增退避 + 随机抖动错峰，避免整页卡片同时重试触发限流；
// 最多重试 3 次，仍失败才显示占位图。离线封面（本地文件）失败一般即文件缺失，直接占位。
const COVER_RETRY_DELAYS = [2000, 4000, 6000]
const MAX_COVER_RETRY = COVER_RETRY_DELAYS.length
const coverRetryCount = ref(0)
const coverFailed = ref(false)
let coverRetryTimer: ReturnType<typeof setTimeout> | null = null

const coverSrc = computed(() => {
  if (coverFailed.value) return defaultCover
  const raw = props.comic.coverUrl
  if (!raw) return defaultCover
  if (props.comic.source === 'online' && coverRetryCount.value > 0) {
    // 重试时追加 cache-buster，强制绕过 cover-proxy 的 86400s 浏览器缓存重新请求
    const sep = raw.includes('?') ? '&' : '?'
    return `${raw}${sep}retry=${coverRetryCount.value}`
  }
  return raw
})

const handleImgError = () => {
  if (props.comic.source !== 'online' || coverRetryCount.value >= MAX_COVER_RETRY) {
    coverFailed.value = true
    return
  }
  const delay = COVER_RETRY_DELAYS[coverRetryCount.value] + Math.random() * 1500
  coverRetryTimer = setTimeout(() => {
    coverRetryCount.value += 1 // coverSrc 变化 → 触发 <img> 重新加载
  }, delay)
}

// comic 切换（列表刷新/翻页复用组件）时重置重试状态，避免继承旧封面的失败结果
watch(
  () => props.comic.coverUrl,
  () => {
    coverRetryCount.value = 0
    coverFailed.value = false
    if (coverRetryTimer) {
      clearTimeout(coverRetryTimer)
      coverRetryTimer = null
    }
  },
)

// 订阅活动下载轮询（仅在线卡片需要，离线卡片不发起轮询）
let unsubscribeActiveDownloads: (() => void) | null = null

onMounted(() => {
  if (props.comic.source === 'online') {
    unsubscribeActiveDownloads = subscribeActiveDownloads()
  }
})

onUnmounted(() => {
  unsubscribeActiveDownloads?.()
  unsubscribeActiveDownloads = null
  if (coverRetryTimer) clearTimeout(coverRetryTimer)
})

// --------------------------------------------------
// 5. 问题2：日语标题优先双行显示（titleJpn 为空时回退到原 title）
// --------------------------------------------------
const jpnTitle = computed(() => {
  if (props.comic.source !== 'offline') return ''
  return ((props.comic as OfflineComic).titleJpn || '').trim()
})

const displayTitle = computed(() => jpnTitle.value || props.comic.title)

const subTitle = computed(() => {
  return jpnTitle.value && jpnTitle.value !== props.comic.title ? props.comic.title : ''
})

// --------------------------------------------------
// 6. 问题3：来源角标（额外路径 Name；下载导入为「下载」时不在卡片重复标注）
// --------------------------------------------------
const comicSourceBadge = computed(() => {
  if (props.comic.source !== 'offline') return ''
  const label = ((props.comic as OfflineComic).sourceLabel || '').trim()
  return label && label !== '下载' ? label : ''
})
</script>

<template>
  <div
    class="item-card"
    :class="[currentMode, size || 'normal', { 'select-mode': selectMode, selected }]"
    @click="handleCardClick"
    @pointerdown="handlePointerDown"
    @pointerup="handlePointerUp"
    @pointerleave="handlePointerUp"
    @pointercancel="handlePointerUp"
  >
    <!-- 选择模式的勾选框覆盖层 -->
    <span
      v-if="selectMode"
      class="select-checkbox"
      :class="{ checked: selected }"
      @click.stop="emit('select', comic)"
    >
      <span class="check-mark">{{ selected ? '✓' : '' }}</span>
    </span>

    <!-- 🪪 名片模式 (Compact) -->
    <template v-if="currentMode === 'compact'">
      <div
        class="compact-thumb-box"
        @click.stop="handleThumbClick"
        :title="showTags ? '点击收起 Tag' : '点击查看完整 Tag 列表'"
      >
        <!-- 🟢 加上 referrerpolicy="no-referrer" 防止封面防盗链报错 -->
        <img
          :src="coverSrc"
          :alt="comic.title"
          class="thumb-img"
          loading="lazy"
          referrerpolicy="no-referrer"
          @error="handleImgError"
        />

        <span v-if="comic.rank" class="rank-badge" :class="{ 'top-3': comic.rank <= 3 }">
          #{{ comic.rank }}
        </span>

        <span class="tag-indicator" :class="{ active: showTags }">
          {{ showTags ? '▲ 隐' : '🏷️ Tag' }}
        </span>

        <!-- ⬇️ 在线卡片 hover 快捷下载按钮（零弹窗） -->
        <button
          v-if="canQuickDownload"
          class="quick-download-btn compact"
          :class="{ loading: isQuickDownloading }"
          :disabled="isQuickDownloading"
          title="加入下载队列"
          @click.stop="handleQuickDownload"
          @pointerdown.stop
          @pointerup.stop
        >
          {{ isQuickDownloading ? '…' : '⬇' }}
        </button>
      </div>

      <div class="compact-main-content">
        <div class="compact-title-wrap">
          <h4 class="compact-title" :title="displayTitle || comic.title">{{ displayTitle }}</h4>
          <span v-if="subTitle && !hideSubtitle" class="compact-subtitle" :title="comic.title">{{
            subTitle
          }}</span>
        </div>

        <div v-if="!showTags" class="compact-normal-panel">
          <div class="meta-row">
            <span class="cat-badge" :style="{ backgroundColor: getCategoryColor(comic.category) }">
              {{ comic.category || (comic.source === 'online' ? 'Doujinshi' : 'Local') }}
            </span>

            <span
              v-if="onlineComic?.isFavorite && onlineComic.favIndex !== undefined"
              class="fav-dot"
              :style="{ backgroundColor: favColors[onlineComic.favIndex] || '#7f7f7f' }"
            >
              ★
            </span>

            <span v-if="comic.rating" class="rating-text">⭐ {{ comic.rating }}</span>

            <span v-if="comic.pageCount" class="pages-text">{{ comic.pageCount }} 页</span>

            <span v-if="comic.updatedAt" class="date-text">{{ comic.updatedAt }}</span>
          </div>

          <div class="status-row">
            <span v-if="isDownloading" class="downloaded-badge downloading"> ⏳ 下载中 </span>
            <span v-else-if="comic.isDownloaded" class="downloaded-badge"> ✓ 已下载 </span>
            <span v-if="comicSourceBadge" class="source-label-badge">{{ comicSourceBadge }}</span>
          </div>
        </div>

        <div v-else class="compact-tags-panel" @click.stop>
          <div class="tags-scroll-container">
            <TagChip v-for="(tag, idx) in normalizedTags" :key="`${tag}-${idx}`" :tag="tag" />
            <span v-if="normalizedTags.length === 0" class="empty-tag-text"> 暂无标签 </span>
          </div>
        </div>
      </div>
    </template>

    <!-- 🎴 大卡片模式 (Card Mode) -->
    <template v-else>
      <div class="card-cover-wrapper">
        <!-- 🟢 防盗链保护 -->
        <img
          :src="coverSrc"
          :alt="comic.title"
          class="cover-img"
          loading="lazy"
          referrerpolicy="no-referrer"
          @error="handleImgError"
        />
        <span class="card-cat-badge" :style="{ backgroundColor: getCategoryColor(comic.category) }">
          {{ comic.category || 'Manga' }}
        </span>
        <span
          v-if="onlineComic?.isFavorite && onlineComic.favIndex !== undefined"
          class="card-fav-badge"
          :style="{ backgroundColor: favColors[onlineComic.favIndex] || '#7f7f7f' }"
        >
          ★
        </span>

        <!-- 🟢 补齐下载状态标志（下载中优先展示，避免重复点击） -->
        <span v-if="isDownloading" class="card-downloaded-badge downloading">⏳ 下载中</span>
        <span v-else-if="comic.isDownloaded" class="card-downloaded-badge">✓ 已下载</span>
        <span v-if="comic.pageCount" class="card-pages-badge">{{ comic.pageCount }}P</span>

        <!-- ⬇️ 在线卡片 hover 快捷下载按钮（零弹窗） -->
        <button
          v-if="canQuickDownload"
          class="quick-download-btn"
          :class="{ loading: isQuickDownloading }"
          :disabled="isQuickDownloading"
          title="加入下载队列"
          @click.stop="handleQuickDownload"
          @pointerdown.stop
          @pointerup.stop
        >
          {{ isQuickDownloading ? '加入中…' : '⬇ 下载' }}
        </button>
      </div>

      <div class="card-info-footer">
        <div class="card-title-wrap">
          <h4 class="card-title" :title="displayTitle || comic.title">{{ displayTitle }}</h4>
          <!-- 🏆 排行榜大卡片（size=large，仅领奖台使用）：只显示主标题，隐藏副标题，
               避免「日语标题 + 中文副标题」双标题把卡片撑高撑宽（离线 NO.1 过大的 Bug）。
               Round3-任务3：hideSubtitle 用于排行榜普通卡片同样只保留单一标题 -->
          <span
            v-if="subTitle && size !== 'large' && !hideSubtitle"
            class="card-subtitle"
            :title="comic.title"
            >{{ subTitle }}</span
          >
        </div>
        <div class="card-tags-row">
          <TagChip v-for="tag in normalizedTags.slice(0, 3)" :key="tag" :tag="tag" />
        </div>
        <div class="card-bottom-meta">
          <span class="rating">⭐ {{ comic.rating || '5.0' }}</span>
          <span class="source-tag" :class="[comic.source, { extra: !!comicSourceBadge }]">
            {{ comic.source === 'online' ? '在线' : comicSourceBadge || '本地' }}
          </span>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.item-card {
  position: relative;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  cursor: pointer;
  user-select: none;
  transition: all 0.2s ease;
  overflow: hidden;
}

.item-card:hover {
  background-color: var(--app-surface-2-hover);
  border-color: var(--app-border-3);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}

/* 名片模式 (Compact) */
.item-card.compact {
  display: flex;
  height: 115px;
  padding: 8px;
  gap: 12px;
  box-sizing: border-box;
}

.compact-thumb-box {
  position: relative;
  width: 80px;
  height: 100%;
  flex-shrink: 0;
  border-radius: 6px;
  overflow: hidden;
  background-color: var(--app-input-bg);
}

.thumb-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.2s ease;
}

.compact-thumb-box:hover .thumb-img {
  transform: scale(1.05);
}

.rank-badge {
  position: absolute;
  top: 4px;
  left: 4px;
  background-color: rgba(0, 0, 0, 0.75);
  color: var(--app-text-2);
  font-size: 10px;
  font-weight: 700;
  padding: 1px 4px;
  border-radius: 3px;
}

.rank-badge.top-3 {
  color: #ffd700;
  background-color: rgba(0, 0, 0, 0.85);
}

.tag-indicator {
  position: absolute;
  bottom: 4px;
  right: 4px;
  background-color: rgba(0, 0, 0, 0.75);
  color: var(--app-text-2);
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  transition: all 0.2s ease;
}

.tag-indicator.active {
  background-color: #ff7588;
  color: #ffffff;
}

.compact-main-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-width: 0;
}

.compact-title-wrap {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.compact-title {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-strong);
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.compact-subtitle {
  font-size: 11px;
  color: var(--app-text-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.compact-normal-panel {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--app-text-3);
  flex-wrap: wrap;
}

.cat-badge {
  padding: 2px 7px;
  border-radius: 4px;
  color: #ffffff;
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.fav-dot {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  color: #ffffff;
  font-size: 9px;
}

.rating-text {
  color: #ffb74d;
}

.pages-text,
.date-text {
  color: var(--app-text-3);
  font-size: 11px;
}

.status-row {
  min-height: 18px;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  display: flex;
  align-items: center;
}

.downloaded-badge {
  display: inline-block;
  color: #4caf50;
  background-color: rgba(76, 175, 80, 0.12);
  border: 1px solid rgba(76, 175, 80, 0.3);
  font-size: 11px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 4px;
}

/* 下载中：蓝色脉冲，与绿色「已下载」区分 */
.downloaded-badge.downloading {
  color: #2196f3;
  background-color: rgba(33, 150, 243, 0.12);
  border-color: rgba(33, 150, 243, 0.35);
  animation: badge-pulse 1.6s ease-in-out infinite;
}

@keyframes badge-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.55;
  }
}

.compact-tags-panel {
  flex: 1;
  margin-top: 4px;
  overflow: hidden;
}

.tags-scroll-container {
  max-height: 52px;
  overflow-y: auto;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding-right: 4px;
}

.tags-scroll-container::-webkit-scrollbar {
  width: 4px;
}
.tags-scroll-container::-webkit-scrollbar-thumb {
  background-color: var(--app-border-3);
  border-radius: 2px;
}

.empty-tag-text {
  font-size: 11px;
  color: var(--app-text-3);
}

/* 大卡片模式 (Card Mode) */
.item-card.card {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.card-cover-wrapper {
  position: relative;
  width: 100%;
  aspect-ratio: 3 / 4;
  background-color: var(--app-input-bg);
  overflow: hidden;
}

.cover-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.card-cat-badge {
  position: absolute;
  top: 6px;
  left: 6px;
  padding: 2px 6px;
  border-radius: 4px;
  color: #ffffff;
  font-size: 10px;
  font-weight: 600;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.4);
}

.card-fav-badge {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  color: #ffffff;
  font-size: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.card-downloaded-badge {
  position: absolute;
  bottom: 6px;
  left: 6px;
  color: #4caf50;
  background-color: rgba(0, 0, 0, 0.85);
  border: 1px solid rgba(76, 175, 80, 0.4);
  font-size: 10px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 3px;
}

/* 下载中：蓝色脉冲（卡片封面） */
.card-downloaded-badge.downloading {
  color: #64b5f6;
  border-color: rgba(33, 150, 243, 0.5);
  animation: badge-pulse 1.6s ease-in-out infinite;
}

.card-pages-badge {
  position: absolute;
  bottom: 6px;
  right: 6px;
  background-color: rgba(0, 0, 0, 0.75);
  color: #ffffff;
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  font-family: monospace;
}

/* 在线 hover 快捷下载按钮：默认居中浮于封面上，hover 时显现 */
.quick-download-btn {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  z-index: 8;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 6px 14px;
  border: 1px solid rgba(255, 255, 255, 0.25);
  border-radius: 6px;
  background-color: rgba(0, 0, 0, 0.75);
  color: #ffffff;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  opacity: 0;
  transition:
    opacity 0.15s ease,
    background-color 0.15s ease;
}

.quick-download-btn:hover {
  background-color: #ff7588;
  border-color: rgba(255, 117, 136, 0.6);
}

.item-card:hover .quick-download-btn,
.quick-download-btn:focus-visible {
  opacity: 1;
}

.quick-download-btn.loading {
  cursor: wait;
  opacity: 1;
}

/* 名片模式的按钮：缩小并置于缩略图右下角 */
.quick-download-btn.compact {
  left: auto;
  top: auto;
  right: 6px;
  bottom: 6px;
  transform: none;
  padding: 2px 7px;
  font-size: 10px;
  border-radius: 4px;
}

.card-info-footer {
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 1;
}

.card-title-wrap {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.card-title {
  margin: 0;
  font-size: 13px;
  font-weight: 500;
  color: var(--app-text-strong);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  line-height: 1.3;
}

.card-subtitle {
  font-size: 11px;
  color: var(--app-text-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.card-tags-row {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.card-bottom-meta {
  margin-top: auto;
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 11px;
  color: var(--app-text-muted);
}

.source-tag.online {
  color: #a891e3;
}

.source-tag.offline {
  color: #ff7588;
}

.source-tag.extra {
  background-color: rgba(61, 90, 254, 0.92);
  color: #ffffff;
  padding: 1px 6px;
  border-radius: 4px;
}

.source-label-badge {
  display: inline-block;
  background-color: rgba(61, 90, 254, 0.92);
  color: #ffffff;
  font-size: 10px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 4px;
}

/* 选择模式：卡片高亮 + 勾选框 */
.item-card.select-mode {
  cursor: default;
}

.item-card.select-mode:hover {
  box-shadow: 0 0 0 2px rgba(255, 117, 136, 0.35);
}

.item-card.selected {
  border-color: #ff7588;
  box-shadow: 0 0 0 2px rgba(255, 117, 136, 0.45);
}

.select-checkbox {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 10;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 2px solid rgba(255, 255, 255, 0.6);
  background-color: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.15s ease;
}

.select-checkbox .check-mark {
  color: #ffffff;
  font-size: 13px;
  font-weight: 700;
  line-height: 1;
}

.select-checkbox.checked {
  background-color: #ff7588;
  border-color: #ff7588;
}
</style>
