<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
import type { OnlineComic } from '@/types/comic'
import TagChip from '@/components/TagChip.vue'
import { onlineReadingList, toggleReadingList } from '@/stores/readingStore'
import { addHistory, updateOnlineFavoriteState, resolveOnlineToken } from '@/stores/historyStore'
import { preferenceSettings } from '@/stores/preferenceSettings'
import { resolveDefaultDownloadScheme } from '@/api/download'
import { isGidDownloading, markGidActive } from '@/stores/downloadTasksStore'
import { http } from '@/utils/request'
import { useUserStore } from '@/stores/userStore'
import { isDetailNewTab, consumeBackState } from '@/utils/detailNav'
import { API_BASE } from '@/config/api'
import { rememberListState } from '@/utils/scrollMemory'
// Round7-任务1/3：起始页确定性恢复（历史入口总是恢复，否则按偏好开关）
import { resolveResumePage, isResumeFromLastPageEnabled } from '@/utils/readingProgress'

const route = useRoute()
const router = useRouter()
const { toast, modal } = useUI()
const userStore = useUserStore()
// 下载权限：管理员或有 allowDownload 许可才展示下载入口（中心制：无许可用户不展示下载能力）
const canDownload = computed(() => userStore.isAdmin || !!userStore.user?.allowDownload)

// 内嵌面板模式：由列表页通过 props 传入 gid/token，无需路由跳转即可切换画廊
const props = withDefaults(
  defineProps<{
    embedded?: boolean
    gid?: string
    token?: string
    localPanel?: boolean // S7 对比左侧：纯本地版展示（强制本地模式、隐藏评论 tab 与版本切换）
    /** Round7-任务6：来源是否为历史页（内嵌面板时由父级传入，「立即阅读」从上次位置开始） */
    fromHistory?: boolean
  }>(),
  { embedded: false, gid: '', token: '', localPanel: false, fromHistory: false },
)

// 有效 gid/token：内嵌面板优先使用 props，全屏路由则读 route.query
const effectiveGid = computed(() => props.gid || (route.query.id as string) || '')
const effectiveToken = computed(() => props.token || (route.query.token as string) || '')

const activeTab = ref<'info' | 'preview' | 'comments'>('info')
const isLoading = ref(true)

// 🟢 新增：预览图分页状态控制
const currentPreviewPage = ref(0) // 初始由 detail 接口获取 p=0
const maxPreviewPage = ref(1) // 从后端返回获取总预览页数
const isLoadingMorePreviews = ref(false)

interface PreviewPageItem {
  pageIndex: number
  url: string
  isSprite?: boolean
  offsetX?: number
  offsetY?: number
  width?: number
  height?: number
}

// 后端 GetOnlineComicDetail / GetOnlineComicPreviews 返回的预览切片 DTO
interface PreviewPageDTO {
  pageIndex: number
  imageUrl?: string // 后端字段名（前端映射为 url）
  url?: string
  isSprite?: boolean
  offsetX?: number
  offsetY?: number
  width?: number
  height?: number
}

// 后端 GetOnlineComicDetail 返回的详情 DTO
interface OnlineDetailDTO {
  id: string
  title: string
  coverUrl: string
  token?: string
  subTitle?: string
  tags: string[]
  rating?: number
  pageCount?: number
  updatedAt: string
  category?: string
  uploader?: string
  isFavorite?: boolean
  favIndex?: number
  isDownloaded?: boolean
  maxPreviewPage?: number
  previewPages?: PreviewPageDTO[]
  comments?: { id: number; user: string; date: string; content: string }[]
  // S1 本地优先：后端附加的本地副本信息（存在同 GID 本地画廊时返回）
  local?: {
    comicId: string
    pageCount: number
    coverUrl: string
    localPath: string
    hasComments: boolean
  }
}

// 详情页扩展数据模型
interface GalleryDetail extends OnlineComic {
  subTitle?: string
  maxPreviewPage?: number
  previewPages: PreviewPageItem[] // 👈 使用新的接口类型
  comments: { id: number; user: string; date: string; content: string }[]
}

// 空壳详情模板（内嵌面板切换画廊时复用）
const createEmptyComic = (gid: string, token: string): GalleryDetail => ({
  id: gid,
  token,
  title: '加载中...',
  subTitle: '',
  coverUrl: '',
  source: 'online',
  tags: [],
  rating: 0,
  pageCount: 0,
  updatedAt: '',
  category: 'Doujinshi',
  uploader: '',
  isFavorite: false,
  favIndex: 0,
  isDownloaded: false,
  previewPages: [],
  comments: [],
})

const comic = ref<GalleryDetail>(createEmptyComic(effectiveGid.value, effectiveToken.value))

// 详情加载失败信息（如画廊已删除/不可用/版权下架），非空时展示错误态
const detailError = ref('')

// S1 本地优先：后端附加的本地副本信息（有本地副本且开启本地优先时非空）
interface LocalVersionInfo {
  comicId: string
  pageCount: number
  coverUrl: string
  localPath: string
  hasComments: boolean
}
const localVersion = ref<LocalVersionInfo | null>(null)
const useOnlineOverride = ref(false) // 手动切回在线版本（不改设置项，仅本次查看）
const isLocalMode = computed(() => !!localVersion.value && !useOnlineOverride.value)
// 在线预览切片缓存（手动切回在线版本时恢复）
const onlinePreviewPages = ref<PreviewPageItem[]>([])
const onlineMaxPreviewPage = ref(1)

// 1. 获取画廊真实详情 (仅抓取 p=0 基础元数据与初始预览图)
const fetchDetail = async () => {
  const gid = effectiveGid.value
  let token = effectiveToken.value

  if (!gid) {
    toast.error('画廊 ID 或 Token 参数缺失！')
    isLoading.value = false
    return
  }
  // Round7-任务4：token 缺失（历史记录 / 手动链接丢失）时按 gid 兜底解析
  if (!token) {
    token = await resolveOnlineToken(gid)
    if (token && comic.value) comic.value.token = token
  }
  if (!token) {
    toast.error('画廊 ID 或 Token 参数缺失！')
    isLoading.value = false
    return
  }

  isLoading.value = true
  detailError.value = ''
  try {
    // S1：开启本地优先时附带 preferLocal=1，后端在本地库存在同 GID 画廊时附加 local 信息
    const data = await http<OnlineDetailDTO>('/comics/online/detail', {
      params: {
        id: gid,
        token,
        ...(props.localPanel || preferenceSettings.preferLocalGallery ? { preferLocal: 1 } : {}),
      },
    })

    // 1. fetchDetail 内部的映射：
    const formattedInitialPreviews = (data.previewPages || []).map((item) => ({
      pageIndex: item.pageIndex,
      url: item.imageUrl || item.url || '',
      // 🟢 追加雪碧图字段透传
      isSprite: !!item.isSprite,
      offsetX: item.offsetX || 0,
      offsetY: item.offsetY || 0,
      width: item.width || 100,
      height: item.height || 130,
    }))

    comic.value = {
      ...data,
      source: 'online',
      isFavorite: !!data.isFavorite,
      favIndex: data.favIndex ?? 0,
      tags: data.tags || [],
      previewPages: formattedInitialPreviews, // 👈 使用映射后的预览图列表
      comments: data.comments || [],
    }

    maxPreviewPage.value = data.maxPreviewPage || 1
    currentPreviewPage.value = 1

    addHistory(comic.value)

    // S1 本地优先：记录本地副本信息与在线预览缓存，供徽章/切回在线版本使用
    onlinePreviewPages.value = [...formattedInitialPreviews]
    onlineMaxPreviewPage.value = data.maxPreviewPage || 1
    localVersion.value = data.local || null
    useOnlineOverride.value = false

    // 本地模式：预览/阅读页图改走本地接口；加载失败则降级为在线预览
    if (isLocalMode.value) {
      const ok = await loadLocalPreviewPages()
      if (!ok) localVersion.value = null
    }
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : '获取画廊详情失败'
    detailError.value = msg
    toast.error(msg)
  } finally {
    isLoading.value = false
  }
}

// S1 本地优先：加载本地页图列表并重建预览切片（0-based index 对应 /comics/:id/page/:index）
const LOCAL_PREVIEW_BATCH = 40
const loadLocalPreviewPages = async (): Promise<boolean> => {
  const lv = localVersion.value
  if (!lv) return false
  try {
    const res = await http<{ total: number; pages?: string[] }>(`/comics/${lv.comicId}/pages`)
    const total = res.total || lv.pageCount || 0
    if (total <= 0) return false
    comic.value.pageCount = total
    const count = Math.min(total, LOCAL_PREVIEW_BATCH)
    comic.value.previewPages = Array.from({ length: count }, (_, i) => ({
      pageIndex: i + 1,
      url: `${API_BASE}/comics/${lv.comicId}/page/${i}`,
      isSprite: false,
      width: 100,
      height: 130,
    }))
    maxPreviewPage.value = Math.ceil(total / LOCAL_PREVIEW_BATCH)
    currentPreviewPage.value = 1
    return true
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '加载本地版本失败，已使用在线预览')
    return false
  }
}

// S1 手动切回在线版本（不改动设置项，仅本次查看使用在线预览）
const switchToOnline = () => {
  useOnlineOverride.value = true
  comic.value.previewPages = [...onlinePreviewPages.value]
  maxPreviewPage.value = onlineMaxPreviewPage.value
  currentPreviewPage.value = 1
}

// S1 从在线版本切回本地版本
const switchToLocal = async () => {
  useOnlineOverride.value = false
  if (localVersion.value) {
    const ok = await loadLocalPreviewPages()
    if (!ok) localVersion.value = null
  }
}

// 🟢 2. 新增：增量请求下一页预览图
const handleLoadMorePreviews = async () => {
  if (isLoadingMorePreviews.value || currentPreviewPage.value >= maxPreviewPage.value) return

  isLoadingMorePreviews.value = true
  const nextPage = currentPreviewPage.value + 1

  try {
    // S1 本地模式：直接追加本地页图（无需网络请求）
    if (isLocalMode.value && localVersion.value) {
      const lv = localVersion.value
      const total = comic.value.pageCount || 0
      const start = comic.value.previewPages.length
      const end = Math.min(start + LOCAL_PREVIEW_BATCH, total)
      for (let i = start; i < end; i++) {
        comic.value.previewPages.push({
          pageIndex: i + 1,
          url: `${API_BASE}/comics/${lv.comicId}/page/${i}`,
          isSprite: false,
          width: 100,
          height: 130,
        })
      }
      currentPreviewPage.value = nextPage
      return
    }

    const newPreviews = await http<PreviewPageDTO[]>('/comics/online/previews', {
      params: {
        id: comic.value.id,
        token: comic.value.token,
        page: nextPage,
      },
    })

    if (Array.isArray(newPreviews) && newPreviews.length > 0) {
      const baseIndex = comic.value.previewPages.length
      // 2. handleLoadMorePreviews 内部的映射：
      const formattedPreviews = newPreviews.map((item, idx) => ({
        pageIndex: baseIndex + idx + 1,
        url: item.imageUrl || item.url || '',
        // 🟢 追加雪碧图字段透传
        isSprite: !!item.isSprite,
        offsetX: item.offsetX || 0,
        offsetY: item.offsetY || 0,
        width: item.width || 100,
        height: item.height || 130,
      }))

      comic.value.previewPages.push(...formattedPreviews)
      currentPreviewPage.value = nextPage
    }
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '加载更多切片失败')
  } finally {
    isLoadingMorePreviews.value = false
  }
}

// 3. 标签按 Namespace 智能分组 (画廊标准排版)
const groupedTags = computed(() => {
  const groups: Record<string, string[]> = {}
  for (const tag of comic.value.tags || []) {
    let ns = 'misc'
    if (tag.includes(':')) {
      ns = tag.split(':')[0].toLowerCase()
    }
    if (!groups[ns]) {
      groups[ns] = []
    }
    groups[ns].push(tag)
  }
  return groups
})

// 4. 阅读清单与动作响应
const isInReadingList = computed(() =>
  onlineReadingList.value.some((item) => item.id === comic.value.id),
)

const handleAddToReadingList = () => {
  toggleReadingList(comic.value)
  if (isInReadingList.value) {
    toast.success(`已将《${comic.value.title}》加入在线阅读清单 📑`)
  } else {
    toast.info(`已从在线阅读清单中移除《${comic.value.title}》`)
  }
}

const handleBack = () => {
  const gid = effectiveGid.value || ''
  // Round7-任务4：opener 存在（来源标签仍打开）→ 直接关闭本标签，来源列表保持原位
  if (window.opener) {
    window.close()
    return
  }
  // Round7-任务4：opener 已关闭 → 回到来源列表并恢复位置（读取打开时记录的状态）
  const backState = consumeBackState(gid)
  if (backState) {
    rememberListState(backState.fromPath, { top: backState.top, page: backState.page })
    router.replace(backState.fromPath)
    return
  }
  // S11：由本应用新标签打开（sessionStorage 标记）→ 关闭标签返回列表
  if (isDetailNewTab(gid)) {
    window.close()
    return
  }
  if (window.history.length > 1) {
    router.back()
  } else {
    router.push('/online/home')
  }
}

const favColors: Record<number, string> = {
  0: '#7f7f7f',
  1: '#f00000',
  2: '#ff7800',
  3: '#f0d000',
  4: '#00a0a0',
  5: '#98e020',
  6: '#00a0a0',
  7: '#0000f0',
  8: '#a000a0',
  9: '#f000a0',
}

// 5. 点击预览切片直接跳页阅读；「立即阅读」按钮不传参 → 显式计算起始页
const handleStartReading = async (targetPage?: number) => {
  let page = targetPage
  // Round7-任务1/3：未显式指定页码（「立即阅读」按钮）→ 计算起始页
  // （历史入口总是恢复，否则按偏好开关；无记录回到第 1 页）
  if (page === undefined) {
    // Round7-任务6：内嵌面板由父级标记来源为历史页；全屏路由看 ?resume=1
    const fromHistory = props.fromHistory || route.query.resume === '1'
    const resumePage = await resolveResumePage('online', comic.value.id, {
      fromHistory,
      resumePreference: isResumeFromLastPageEnabled(),
    })
    page = resumePage ?? 1
  }
  // S1 本地优先：有本地副本且未手动切回在线 → 走本地阅读（/reader source=offline）
  if (isLocalMode.value && localVersion.value) {
    router.push({
      path: '/reader',
      query: {
        id: localVersion.value.comicId,
        source: 'offline',
        page,
      },
    })
    return
  }
  router.push({
    path: '/reader',
    query: {
      id: comic.value.id,
      token: comic.value.token,
      source: 'online',
      page,
    },
  })
}

// 6. 下载功能：GP 面板 + 创建下载任务
const showDownloadPanel = ref(false)
const isLoadingGP = ref(false)
const isCreatingTask = ref(false)

// 后端 DownloadGPInfo / ArchiveInfo 返回结构
interface ArchiveOptionDTO {
  label: string
  name: string
  cost: string
  size: string
}
interface ArchiveInfoDTO {
  gid: string
  token: string
  options: ArchiveOptionDTO[]
  sizeBytes?: number
}
interface GPInfoDTO {
  gp: string
  credits: string
  hath: string
  quotaUsed: number
  quotaMax: number
  archive?: ArchiveInfoDTO
}

const gpInfo = ref<GPInfoDTO | null>(null)
const selectedMode = ref<'gallery' | 'archive'>('archive')
const selectedArchiveType = ref<'original' | 'resample'>('original')

// 归档方案展示（原图/压缩图）
const archiveOptions = computed(() => gpInfo.value?.archive?.options || [])

const handleOpenDownloadPanel = async () => {
  // 去重拦截：已下载到本地 / 已在下载队列中的画廊禁止重复下载
  if (comic.value.isDownloaded) {
    toast.info('该画廊已存入本地，请勿重复下载')
    return
  }
  if (isGidDownloading(comic.value.id)) {
    toast.info('该画廊已加入下载队列，请勿重复下载')
    return
  }
  showDownloadPanel.value = true
  // 默认方案读取「下载设置 → 默认下载配置」（四选一）
  const { mode, archiveType } = resolveDefaultDownloadScheme()
  selectedMode.value = mode
  selectedArchiveType.value = archiveType === 'resample' ? 'resample' : 'original'
  await fetchGPInfo()
}

const fetchGPInfo = async () => {
  isLoadingGP.value = true
  try {
    gpInfo.value = await http<GPInfoDTO>('/downloads/gp-info', {
      params: { gid: comic.value.id, token: comic.value.token },
    })
  } catch (err: unknown) {
    gpInfo.value = null
    toast.error(err instanceof Error ? err.message : '获取 GP 面板信息失败')
  } finally {
    isLoadingGP.value = false
  }
}

const closeDownloadPanel = () => {
  showDownloadPanel.value = false
}

const handleStartDownload = async () => {
  isCreatingTask.value = true
  try {
    await http('/downloads', {
      method: 'POST',
      body: JSON.stringify({
        gid: comic.value.id,
        token: comic.value.token,
        title: comic.value.title,
        coverUrl: comic.value.coverUrl,
        mode: selectedMode.value,
        archiveType: selectedMode.value === 'archive' ? selectedArchiveType.value : '',
      }),
    })
    toast.success('下载任务已创建，可在「下载」页面查看进度')
    markGidActive(comic.value.id)
    showDownloadPanel.value = false
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '创建下载任务失败')
  } finally {
    isCreatingTask.value = false
  }
}

// 点击选择收藏夹 (0 ~ 9)
const handleSelectFavorite = async () => {
  const chosenIndex = await modal.prompt(
    '请选择收藏夹 (输入 0 ~ 9)：',
    String(comic.value.favIndex ?? 0),
    '设置在线收藏',
  )
  if (chosenIndex !== null) {
    const idx = parseInt(chosenIndex, 10)
    if (!isNaN(idx) && idx >= 0 && idx <= 9) {
      try {
        await http('/comics/online/favorite', {
          method: 'POST',
          body: JSON.stringify({
            gid: comic.value.id,
            token: comic.value.token,
            favCat: idx,
            note: '',
          }),
        })

        // 🟢 1. 更新当前详情页的局部响应式状态
        comic.value.isFavorite = true
        comic.value.favIndex = idx

        // 🟢 刷新历史记录并通知 store 联动更新列表与清单里的项
        addHistory(comic.value)
        updateOnlineFavoriteState(comic.value.id, true, idx)

        toast.success(`已成功存入 Favorite ${idx}`)
      } catch (err: unknown) {
        toast.error(err instanceof Error ? err.message : '设置收藏失败')
      }
    } else {
      toast.error('请输入 0 到 9 之间的数字')
    }
  }
}

// 取消收藏
const handleRemoveFavorite = async () => {
  if (!comic.value.isFavorite) return

  const confirm = window.confirm(`确定要从收藏夹移除《${comic.value.title}》吗？`)
  if (!confirm) return

  try {
    await http('/comics/online/favorite', {
      method: 'DELETE',
      body: JSON.stringify({
        gid: comic.value.id,
        token: comic.value.token,
      }),
    })

    // 🟢 1. 重置局部状态
    comic.value.isFavorite = false
    comic.value.favIndex = undefined

    // 🟢 2. 重新调用 addHistory 覆盖缓存，确保历史记录/阅读清单里的红心同步熄灭
    addHistory(comic.value)
    updateOnlineFavoriteState(comic.value.id, false)

    toast.success('已从收藏夹移除')
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '取消收藏失败')
  }
}

// 长按判定逻辑与定时器清理
let pressTimer: number | null = null
let isLongPress = false

const clearPressTimer = () => {
  if (pressTimer !== null) {
    clearTimeout(pressTimer)
    pressTimer = null
  }
}

const handlePressStart = () => {
  clearPressTimer()
  isLongPress = false
  pressTimer = window.setTimeout(() => {
    isLongPress = true
    handleRemoveFavorite()
  }, 700)
}

const handlePressEnd = () => {
  clearPressTimer()
}

const handleFavClick = () => {
  if (isLongPress) {
    isLongPress = false
    return
  }
  handleSelectFavorite()
}

onMounted(() => {
  fetchDetail()
})

onUnmounted(() => {
  clearPressTimer()
})

// 内嵌面板：父级切换 gid/token 时，重置状态并重新拉取详情
watch(
  () => [props.gid, props.token],
  () => {
    if (!props.embedded || !props.gid) return
    comic.value = createEmptyComic(props.gid, props.token)
    detailError.value = ''
    isLoading.value = true
    activeTab.value = 'info'
    currentPreviewPage.value = 0
    maxPreviewPage.value = 1
    showDownloadPanel.value = false
    localVersion.value = null
    useOnlineOverride.value = false
    fetchDetail()
  },
)
</script>

<template>
  <div class="detail-page" :class="{ embedded }">
    <!-- 📱 移动形态：左上角圆形返回悬浮球（fixed，位于悬浮 TopBar 正下方；桌面端隐藏） -->
    <button v-if="!embedded" class="detail-fab-back" @click="handleBack" title="返回上一页">
      ‹
    </button>
    <div v-if="isLoading" class="loading-state">加载中...</div>

    <div v-else-if="detailError" class="error-state">
      <div class="error-icon">⚠️</div>
      <p class="error-msg">{{ detailError }}</p>
      <button v-if="!embedded" class="back-btn" @click="handleBack">‹ 返回</button>
    </div>

    <template v-else>
      <div class="top-action-bar">
        <button v-if="!embedded" class="back-btn" @click="handleBack">‹ 返回</button>

        <div class="right-actions">
          <button
            class="add-reading-btn"
            :class="{ active: isInReadingList }"
            @click="handleAddToReadingList"
          >
            {{ isInReadingList ? '✓ 已在清单' : '📑 加入清单' }}
          </button>

          <button
            class="action-btn fav-btn"
            :style="
              comic.isFavorite
                ? { backgroundColor: favColors[comic.favIndex ?? 0], color: '#fff' }
                : {}
            "
            @mousedown="handlePressStart"
            @mouseup="handlePressEnd"
            @mouseleave="handlePressEnd"
            @touchstart="handlePressStart"
            @touchend="handlePressEnd"
            @contextmenu.prevent
            @click="handleFavClick"
          >
            ❤️ {{ comic.isFavorite ? `Fav ${comic.favIndex ?? 0}` : '加入收藏' }}
          </button>

          <button class="read-btn" @click="handleStartReading()">📖 立即阅读</button>

          <button
            v-if="canDownload"
            class="action-btn download-btn"
            @click="handleOpenDownloadPanel"
          >
            ⬇️ 下载
          </button>
        </div>
      </div>

      <!-- S1 本地优先：本地版本徽章 + 手动切回在线/本地；S7 对比左侧 localPanel 固定展示「本地原版」 -->
      <div
        v-if="localVersion"
        class="local-badge-row"
        :class="{ 'is-online': useOnlineOverride, 'is-static': localPanel }"
      >
        <span class="local-badge" :class="{ online: useOnlineOverride && !localPanel }">
          {{ localPanel ? '📚 本地原版' : useOnlineOverride ? '🌐 在线版本' : '📚 本地版本' }}
        </span>
        <button
          v-if="!localPanel && !useOnlineOverride"
          class="switch-version-btn"
          @click="switchToOnline"
        >
          切回在线版本
        </button>
        <button v-else-if="!localPanel" class="switch-version-btn" @click="switchToLocal">
          使用本地版本
        </button>
      </div>

      <!-- 标题与英文/译名标题 -->
      <div class="title-header">
        <h1 class="comic-main-title">{{ comic.title }}</h1>
        <h2 v-if="comic.subTitle && comic.subTitle !== comic.title" class="comic-sub-title">
          {{ comic.subTitle }}
        </h2>
      </div>

      <!-- 📱 移动形态：功能按钮横向可滚动（标题下方，不换行不撑满；桌面端隐藏） -->
      <div class="detail-actions-bar">
        <button
          class="add-reading-btn"
          :class="{ active: isInReadingList }"
          @click="handleAddToReadingList"
        >
          {{ isInReadingList ? '✓ 已在清单' : '📑 加入清单' }}
        </button>

        <button
          class="action-btn fav-btn"
          :style="
            comic.isFavorite
              ? { backgroundColor: favColors[comic.favIndex ?? 0], color: '#fff' }
              : {}
          "
          @mousedown="handlePressStart"
          @mouseup="handlePressEnd"
          @mouseleave="handlePressEnd"
          @touchstart="handlePressStart"
          @touchend="handlePressEnd"
          @contextmenu.prevent
          @click="handleFavClick"
        >
          ❤️ {{ comic.isFavorite ? `Fav ${comic.favIndex ?? 0}` : '加入收藏' }}
        </button>

        <button class="read-btn" @click="handleStartReading()">📖 立即阅读</button>

        <button
          v-if="canDownload"
          class="action-btn download-btn"
          @click="handleOpenDownloadPanel"
        >
          ⬇️ 下载
        </button>
      </div>

      <!-- 选项卡导航 -->
      <div class="detail-tabs">
        <button
          class="tab-item"
          :class="{ active: activeTab === 'info' }"
          @click="activeTab = 'info'"
        >
          📌 基础信息
        </button>
        <button
          class="tab-item"
          :class="{ active: activeTab === 'preview' }"
          @click="activeTab = 'preview'"
        >
          🖼️ 预览切片 (已载 {{ comic.previewPages?.length || 0 }} / 共 {{ comic.pageCount || 0 }}P)
        </button>
        <button
          v-if="!localPanel && preferenceSettings.showGalleryComments"
          class="tab-item"
          :class="{ active: activeTab === 'comments' }"
          @click="activeTab = 'comments'"
        >
          💬 社区评论 ({{ comic.comments?.length || 0 }})
        </button>
      </div>

      <!-- Tab 1: 基础信息 -->
      <div v-if="activeTab === 'info'" class="tab-content info-tab">
        <div class="info-layout">
          <div class="cover-box">
            <img :src="comic.coverUrl" :alt="comic.title" referrerpolicy="no-referrer" />
          </div>

          <div class="metadata-box">
            <div class="meta-row">
              <span class="label">上传作者:</span>
              <span class="value link">{{ comic.uploader || '匿名' }}</span>
            </div>
            <div class="meta-row">
              <span class="label">作品分类:</span>
              <span class="value category-badge">{{ comic.category }}</span>
            </div>
            <div class="meta-row">
              <span class="label">全站评分:</span>
              <span class="value rating-star">⭐ {{ comic.rating }} / 5.0</span>
            </div>
            <div class="meta-row">
              <span class="label">总页数:</span>
              <span class="value">{{ comic.pageCount || 0 }} 页</span>
            </div>
            <div class="meta-row">
              <span class="label">发布时间:</span>
              <span class="value">{{ comic.updatedAt }}</span>
            </div>

            <!-- 按 Namespace 分组渲染 Tag -->
            <div class="tags-section">
              <h3 class="section-title">🏷️ Tag 属性云</h3>
              <div class="grouped-tag-list">
                <div v-for="(tags, ns) in groupedTags" :key="ns" class="ns-group-row">
                  <span class="ns-label">{{ ns }}:</span>
                  <div class="ns-tags">
                    <TagChip v-for="tag in tags" :key="tag" :tag="tag" />
                  </div>
                </div>
                <div v-if="!comic.tags?.length" class="empty-tip">暂无标签数据</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab 2: 预览切片 -->
      <div v-if="activeTab === 'preview'" class="tab-content preview-tab">
        <div v-if="comic.previewPages?.length" class="preview-container">
          <div class="preview-grid">
            <div
              v-for="page in comic.previewPages"
              :key="page.pageIndex"
              class="preview-card"
              @click="handleStartReading(page.pageIndex)"
            >
              <!-- 🟢 模式 1：普通独立大图 (gdtl) -->
              <img
                v-if="!page.isSprite"
                :src="page.url"
                loading="lazy"
                referrerpolicy="no-referrer"
              />

              <!-- 🟢 模式 2：CSS 雪碧图切片 (gdtm) -->
              <div
                v-else
                class="sprite-crop"
                :style="{
                  width: `${page.width || 100}px`,
                  height: `${page.height || 130}px`,
                  backgroundImage: `url(${page.url})`,
                  backgroundPosition: `-${page.offsetX || 0}px -${page.offsetY || 0}px`,
                  backgroundRepeat: 'no-repeat',
                }"
              ></div>

              <span class="page-num">P{{ page.pageIndex }}</span>
            </div>
          </div>

          <!-- 🟢 点击加载更多预览图按钮 -->
          <div v-if="currentPreviewPage < maxPreviewPage" class="load-more-box">
            <button
              class="load-more-btn"
              :disabled="isLoadingMorePreviews"
              @click="handleLoadMorePreviews"
            >
              {{
                isLoadingMorePreviews
                  ? '正在拉取切片...'
                  : `点击加载更多预览图 (已加载 ${comic.previewPages.length} / 共 ${comic.pageCount} 页)`
              }}
            </button>
          </div>
        </div>
        <div v-else class="empty-box">暂无预览切片数据</div>
      </div>

      <!-- Tab 3: 社区评论 -->
      <div
        v-if="!localPanel && preferenceSettings.showGalleryComments && activeTab === 'comments'"
        class="tab-content comments-tab"
      >
        <div v-if="comic.comments?.length" class="comments-list">
          <div v-for="item in comic.comments" :key="item.id" class="comment-card">
            <div class="comment-header">
              <span class="user-name">{{ item.user }}</span>
              <span class="comment-time">{{ item.date }}</span>
            </div>
            <p class="comment-body">{{ item.content }}</p>
          </div>
        </div>
        <div v-else class="empty-box">该画廊暂无社区评论</div>
      </div>
    </template>

    <!-- ⬇️ 下载面板（GP 信息 + 方案选择） -->
    <div v-if="showDownloadPanel" class="download-mask" @click.self="closeDownloadPanel">
      <div class="download-panel">
        <div class="panel-header">
          <span class="panel-title">⬇️ 下载《{{ comic.title }}》</span>
          <button class="panel-close" @click="closeDownloadPanel">✕</button>
        </div>

        <div class="panel-body">
          <!-- GP / Credits / Hath 余额 -->
          <div v-if="gpInfo" class="gp-summary">
            <div class="gp-item">
              <span class="gp-label">GP</span>
              <span class="gp-value">{{ gpInfo.gp }}</span>
            </div>
            <div class="gp-item">
              <span class="gp-label">Credits</span>
              <span class="gp-value">{{ gpInfo.credits }}</span>
            </div>
            <div class="gp-item">
              <span class="gp-label">H@H</span>
              <span class="gp-value">{{ gpInfo.hath }}</span>
            </div>
            <div class="gp-item">
              <span class="gp-label">配额</span>
              <span class="gp-value">{{ gpInfo.quotaUsed }}/{{ gpInfo.quotaMax }}</span>
            </div>
          </div>
          <div v-else-if="isLoadingGP" class="gp-loading">正在读取 GP 面板信息...</div>
          <div v-else class="gp-loading">GP 面板信息获取失败，可尝试重新打开</div>

          <!-- 方案选择 -->
          <div class="mode-select">
            <button
              class="mode-btn"
              :class="{ active: selectedMode === 'archive' }"
              @click="selectedMode = 'archive'"
            >
              🗜️ 归档下载（H@H）
            </button>
            <button
              class="mode-btn"
              :class="{ active: selectedMode === 'gallery' }"
              @click="selectedMode = 'gallery'"
            >
              🖼️ 画廊下载（逐图）
            </button>
          </div>

          <!-- 归档方案（原图/压缩图）报价 -->
          <div v-if="selectedMode === 'archive'" class="archive-options">
            <div
              v-for="opt in archiveOptions"
              :key="opt.label"
              class="archive-opt"
              :class="{ active: selectedArchiveType === opt.label }"
              @click="selectedArchiveType = opt.label as 'original' | 'resample'"
            >
              <span class="opt-name">{{ opt.name }}</span>
              <span class="opt-cost">{{ opt.cost }}</span>
              <span class="opt-size">{{ opt.size }}</span>
            </div>
            <div v-if="!archiveOptions.length" class="archive-empty">
              未获取到归档方案（可能 H@H 未就绪），归档下载将在任务执行时重新解析
            </div>
          </div>

          <div v-if="selectedMode === 'gallery'" class="gallery-hint">
            📄 画廊下载将逐张保存 {{ comic.pageCount || 0 }} 页图片到压缩包/解压目录
          </div>
        </div>

        <div class="panel-footer">
          <button class="cancel-btn" @click="closeDownloadPanel">取消</button>
          <button class="start-btn" :disabled="isCreatingTask" @click="handleStartDownload">
            {{ isCreatingTask ? '创建中...' : '开始下载' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* S1 本地优先：本地版本徽章行 */
.local-badge-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 10px 0 0;
  padding: 8px 14px;
  border-radius: 8px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
}

.local-badge-row.is-online {
  border-color: rgba(90, 160, 255, 0.4);
}

.local-badge {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-strong);
  padding: 3px 10px;
  border-radius: 6px;
  background-color: rgba(88, 196, 132, 0.15);
  border: 1px solid rgba(88, 196, 132, 0.5);
}

.local-badge.online {
  background-color: rgba(90, 160, 255, 0.15);
  border-color: rgba(90, 160, 255, 0.5);
}

.switch-version-btn {
  margin-left: auto;
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  font-size: 13px;
  padding: 5px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.switch-version-btn:hover {
  border-color: #ff7588;
  color: var(--app-text-strong);
}

.detail-page {
  padding: 20px;
  max-width: 1100px;
  margin: 0 auto;
  color: var(--app-text-2);
}

/* 内嵌面板模式（左右分栏右侧详情）：自适应容器宽度、面板内独立滚动 */
.detail-page.embedded {
  max-width: none;
  margin: 0;
  padding: 16px;
  height: 100%;
  overflow-y: auto;
}

/* 内嵌面板：返回按钮隐藏后功能按钮右对齐；移动端专用元素一律隐藏 */
.detail-page.embedded .top-action-bar {
  justify-content: flex-end;
}

.detail-page.embedded .detail-fab-back,
.detail-page.embedded .detail-actions-bar {
  display: none !important;
}

/* 内嵌面板（360-420px 窄面板）：强制套用移动端单列布局。
   CSS 媒体查询基于视口而非容器，宽屏桌面下窄面板仍会命中桌面布局，
   故在 embedded 下显式复制 <1024px 移动布局的核心规则。 */
.detail-page.embedded .top-action-bar {
  flex-wrap: wrap;
  gap: 10px;
}
.detail-page.embedded .right-actions {
  flex: 1 1 100%;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}
.detail-page.embedded .action-btn,
.detail-page.embedded .add-reading-btn {
  padding: 8px 12px;
  font-size: 0.82rem;
}
.detail-page.embedded .read-btn {
  padding: 8px 14px;
  font-size: 0.84rem;
}
.detail-page.embedded .comic-main-title {
  font-size: 1.1rem;
}
.detail-page.embedded .comic-sub-title {
  font-size: 0.85rem;
}
.detail-page.embedded .detail-tabs {
  gap: 4px;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}
.detail-page.embedded .tab-item {
  padding: 10px 10px;
  font-size: 0.85rem;
  white-space: nowrap;
  flex-shrink: 0;
}
.detail-page.embedded .info-layout {
  flex-direction: column;
  align-items: center;
  gap: 14px;
  padding: 14px;
}
.detail-page.embedded .cover-box img {
  width: 100%;
  max-width: 220px;
  margin: 0 auto;
  display: block;
}
.detail-page.embedded .metadata-box {
  width: 100%;
}
.detail-page.embedded .meta-row .label {
  width: 70px;
}
.detail-page.embedded .ns-group-row {
  gap: 6px;
}
.detail-page.embedded .ns-label {
  width: 60px;
  font-size: 0.75rem;
}
.detail-page.embedded .preview-grid {
  grid-template-columns: repeat(auto-fill, minmax(100px, 1fr));
  gap: 8px;
}

.loading-state,
.empty-box {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 240px;
  color: var(--app-text-3);
  font-size: 14px;
}

.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  height: 300px;
  color: var(--app-text-2);
  text-align: center;
  padding: 24px;
}
.error-state .error-icon {
  font-size: 42px;
}
.error-state .error-msg {
  max-width: 560px;
  color: var(--app-text-2);
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

.top-action-bar {
  /* 显式声明回归正常文档流：严禁绝对/固定定位，避免被悬浮全局 Header 覆盖或遮挡封面 */
  position: static;
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

/* 📱 移动形态（html[data-layout='mobile']）：详情页顶部重设计（方案B改良，2026-08-06）
   - 返回按钮 → 左上角圆形悬浮球（fixed，位于悬浮 TopBar 正下方、避开汉堡；层级：汉堡70 > TopBar50 > 返回球40）
   - 功能按钮 → 标题下方横向可滚动按钮条（overflow-x:auto，图标+文字，不换行不撑满）
   - 原顶部操作栏整行隐藏（返回钮/功能钮已由上述两元素替代）
   - 桌面端：返回球/功能条默认 display:none，原操作栏保留文档流 */
.detail-fab-back {
  display: none;
}

.detail-actions-bar {
  display: none;
}

:global(html[data-layout='mobile'] .detail-page .top-action-bar) {
  display: none;
}

:global(html[data-layout='mobile'] .detail-page .detail-fab-back) {
  display: flex;
  position: fixed;
  top: calc(56px + var(--safe-top) + 8px);
  left: calc(10px + var(--safe-left));
  z-index: 40;
  width: 40px;
  height: 40px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  border: 1px solid var(--app-border-3);
  background-color: var(--app-surface-2);
  color: var(--app-text-strong);
  font-size: 1.2rem;
  line-height: 1;
  cursor: pointer;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.35);
  -webkit-tap-highlight-color: transparent;
}

:global(html[data-layout='mobile'] .detail-page .detail-actions-bar) {
  display: flex;
  align-items: center;
  gap: 8px;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  flex-wrap: nowrap;
  margin-bottom: 16px;
  padding-bottom: 2px;
}

:global(html[data-layout='mobile'] .detail-page .detail-actions-bar .add-reading-btn),
:global(html[data-layout='mobile'] .detail-page .detail-actions-bar .action-btn),
:global(html[data-layout='mobile'] .detail-page .detail-actions-bar .read-btn) {
  flex-shrink: 0;
  white-space: nowrap;
}

.back-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
}
.back-btn:hover {
  color: var(--app-text-strong);
  border-color: var(--app-border-3);
}

.right-actions {
  display: flex;
  gap: 10px;
}

.action-btn {
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 0.88rem;
  border: none;
  cursor: pointer;
  font-weight: 500;
  transition: opacity 0.2s;
}
.action-btn:hover {
  opacity: 0.85;
}

.fav-btn {
  background: var(--app-border-2);
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
}

.read-btn {
  background: linear-gradient(135deg, #ff7588, #ff9a3c);
  color: #fff;
  font-weight: 600;
  padding: 9px 22px;
  border-radius: 8px;
  font-size: 0.9rem;
  border: none;
  cursor: pointer;
  letter-spacing: 0.5px;
  box-shadow: 0 4px 14px rgba(255, 117, 136, 0.35);
  transition:
    transform 0.2s ease,
    box-shadow 0.2s ease,
    filter 0.2s ease;
}

.read-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(255, 117, 136, 0.5);
  filter: brightness(1.06);
}

.read-btn:active {
  transform: translateY(0);
  box-shadow: 0 2px 8px rgba(255, 117, 136, 0.35);
}

.title-header {
  margin-bottom: 16px;
}

.comic-main-title {
  font-size: 1.3rem;
  margin: 0 0 4px 0;
  color: var(--app-text-strong);
  line-height: 1.4;
}

.comic-sub-title {
  font-size: 0.95rem;
  margin: 0;
  color: var(--app-text-3);
  font-weight: normal;
  line-height: 1.3;
}

/* Tabs */
.detail-tabs {
  display: flex;
  gap: 12px;
  border-bottom: 1px solid var(--app-border-2);
  margin-bottom: 20px;
}

.tab-item {
  background: transparent;
  border: none;
  color: var(--app-text-3);
  padding: 10px 16px;
  font-size: 0.95rem;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 0.2s;
}

.tab-item.active {
  color: #007acc;
  border-bottom-color: #007acc;
  font-weight: bold;
}

/* Info Layout */
.info-layout {
  display: flex;
  gap: 24px;
  background: var(--app-surface-2);
  padding: 20px;
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
}

.cover-box img {
  width: 220px;
  border-radius: 6px;
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.5);
}

.metadata-box {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.meta-row {
  display: flex;
  gap: 10px;
  font-size: 0.9rem;
}
.meta-row .label {
  color: var(--app-text-3);
  width: 80px;
  flex-shrink: 0;
}
.meta-row .value.link {
  color: #007acc;
  cursor: pointer;
}
.rating-star {
  color: #ffc107;
  font-weight: bold;
}

.category-badge {
  background: var(--app-border-2);
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.8rem;
}

.tags-section {
  margin-top: 12px;
}

.section-title {
  font-size: 0.95rem;
  color: var(--app-text-2);
  margin: 0 0 10px 0;
}

/* 标签分组排版 */
.grouped-tag-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ns-group-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.ns-label {
  font-size: 0.8rem;
  color: var(--app-text-3);
  width: 75px;
  flex-shrink: 0;
  text-align: right;
  padding-top: 2px;
}

.ns-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  flex: 1;
}

.empty-tip {
  font-size: 0.85rem;
  color: var(--app-text-muted);
}

/* Preview Grid */
.preview-container {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.preview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: 12px;
}

.preview-card {
  position: relative;
  aspect-ratio: 3 / 4;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid var(--app-border-2);
  cursor: pointer;
  transition: all 0.2s ease;

  /* 🟢 新增：弹性布局，保证雪碧图切片块居中对齐 */
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--app-input-bg);
}

.preview-card img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

/* 🟢 新增：雪碧图切片容器 */
.sprite-crop {
  flex-shrink: 0;
  border-radius: 2px;
}

.preview-card:hover {
  border-color: #007acc;
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}

.page-num {
  position: absolute;
  bottom: 4px;
  right: 4px;
  background: rgba(0, 0, 0, 0.8);
  padding: 1px 6px;
  font-size: 0.7rem;
  border-radius: 3px;
  color: var(--app-text-2);
}

/* 🟢 加载更多按钮样式 */
.load-more-box {
  display: flex;
  justify-content: center;
  padding: 12px 0 24px;
}

.load-more-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: #007acc;
  padding: 10px 28px;
  border-radius: 6px;
  font-size: 0.9rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.load-more-btn:hover:not(:disabled) {
  background-color: var(--app-surface-3-hover);
  border-color: #007acc;
  box-shadow: 0 2px 8px rgba(0, 122, 204, 0.2);
}

.load-more-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Comments */
.comments-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.comment-card {
  background: var(--app-surface-2);
  padding: 12px 16px;
  border-radius: 6px;
  border: 1px solid var(--app-border-2);
}

.comment-header {
  display: flex;
  justify-content: space-between;
  font-size: 0.85rem;
  margin-bottom: 6px;
}

.user-name {
  color: #007acc;
  font-weight: bold;
}

.comment-time {
  color: var(--app-text-muted);
}

.comment-body {
  margin: 0;
  font-size: 0.9rem;
  color: var(--app-text-2);
  line-height: 1.4;
}

.add-reading-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 0.88rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.add-reading-btn:hover {
  background-color: var(--app-surface-3-hover);
  border-color: #007acc;
  color: #007acc;
}

.add-reading-btn.active {
  background-color: rgba(0, 122, 204, 0.2);
  border-color: #007acc;
  color: #007acc;
  font-weight: bold;
}

/* 下载按钮 */
.download-btn {
  background: var(--app-surface-3);
  color: #ff7588;
  border: 1px solid #ff7588;
}
.download-btn:hover {
  background: rgba(255, 117, 136, 0.15);
}

/* ⬇️ 下载面板 */
.download-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.65);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(2px);
}

.download-panel {
  width: 420px;
  max-width: 92vw;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.6);
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border-2);
}

.panel-title {
  font-size: 0.98rem;
  font-weight: 600;
  color: var(--app-text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.panel-close {
  background: transparent;
  border: none;
  color: var(--app-text-3);
  font-size: 1rem;
  cursor: pointer;
  padding: 2px 6px;
}
.panel-close:hover {
  color: var(--app-text-strong);
}

.panel-body {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.gp-summary {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  background: var(--app-bg-deep);
  border-radius: 8px;
  padding: 10px;
}

.gp-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}

.gp-label {
  font-size: 0.72rem;
  color: var(--app-text-3);
}

.gp-value {
  font-size: 0.88rem;
  font-weight: 600;
  color: var(--app-text-2);
}

.gp-loading {
  font-size: 0.85rem;
  color: var(--app-text-3);
  padding: 8px 0;
}

.mode-select {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.mode-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 10px 8px;
  border-radius: 8px;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
}

.mode-btn.active {
  border-color: #ff7588;
  color: #ff7588;
  background: rgba(255, 117, 136, 0.1);
  font-weight: 600;
}

.archive-options {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.archive-opt {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--app-bg-deep);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 10px 12px;
  cursor: pointer;
  transition: all 0.2s;
}

.archive-opt.active {
  border-color: #ff7588;
  background: rgba(255, 117, 136, 0.08);
}

.opt-name {
  font-size: 0.88rem;
  color: var(--app-text-strong);
  font-weight: 500;
  flex: 1;
}

.opt-cost {
  font-size: 0.8rem;
  color: #ffc107;
}

.opt-size {
  font-size: 0.8rem;
  color: var(--app-text-3);
}

.archive-empty {
  font-size: 0.8rem;
  color: var(--app-text-3);
  padding: 6px 0;
}

.gallery-hint {
  font-size: 0.82rem;
  color: var(--app-text-3);
  line-height: 1.4;
}

.panel-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border-2);
}

.cancel-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 0.88rem;
  cursor: pointer;
}

.cancel-btn:hover {
  color: var(--app-text-strong);
  border-color: var(--app-border-3);
}

.start-btn {
  background: #ff7588;
  border: none;
  color: #fff;
  padding: 8px 20px;
  border-radius: 6px;
  font-size: 0.9rem;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.2s;
}

.start-btn:hover:not(:disabled) {
  opacity: 0.85;
}

.start-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 📱 移动形态（<1024px）：详情页信息区改单列、顶部操作栏收纳换行、标签分组压缩 */
@media (max-width: 1024px) {
  .detail-page {
    padding: 12px;
  }

  /* 顶部操作栏：返回钮一行，右侧按钮组换行收纳 */
  .top-action-bar {
    flex-wrap: wrap;
    gap: 10px;
  }
  .top-action-bar .back-btn {
    padding: 6px 12px;
  }
  .right-actions {
    flex: 1 1 100%;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 8px;
  }
  .action-btn,
  .add-reading-btn {
    padding: 8px 12px;
    font-size: 0.82rem;
  }
  .read-btn {
    padding: 8px 14px;
    font-size: 0.84rem;
  }

  /* 标题与 Tab */
  .comic-main-title {
    font-size: 1.1rem;
  }
  .comic-sub-title {
    font-size: 0.85rem;
  }
  .detail-tabs {
    gap: 4px;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }
  .tab-item {
    padding: 10px 10px;
    font-size: 0.85rem;
    white-space: nowrap;
    flex-shrink: 0;
  }

  /* 基础信息：封面居中、信息区单列堆叠 */
  .info-layout {
    flex-direction: column;
    align-items: center;
    gap: 14px;
    padding: 14px;
  }
  .cover-box img {
    width: 100%;
    max-width: 220px;
    margin: 0 auto;
    display: block;
  }
  .metadata-box {
    width: 100%;
  }
  .meta-row .label {
    width: 70px;
  }

  /* 标签分组：压缩左侧命名空间标签宽度 */
  .ns-group-row {
    gap: 6px;
  }
  .ns-label {
    width: 60px;
    font-size: 0.75rem;
  }

  /* 预览切片：缩窄最小卡片宽度，让窄屏多列排布 */
  .preview-grid {
    grid-template-columns: repeat(auto-fill, minmax(100px, 1fr));
    gap: 8px;
  }

  /* 下载面板：窄屏贴边 + 内部可滚动，避免超出视口 */
  .download-panel {
    max-width: 100vw;
    max-height: 92dvh;
    overflow-y: auto;
  }
  .gp-summary {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
