<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
import type { OnlineComic } from '@/types/comic'
import TagChip from '@/components/TagChip.vue'
import {
  onlineReadingList,
  toggleReadingList,
  addHistory,
  updateOnlineFavoriteState,
} from '@/stores/appStore'
import { http } from '@/utils/request'

const route = useRoute()
const router = useRouter()
const { toast, modal } = useUI()

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

// 详情页扩展数据模型
interface GalleryDetail extends OnlineComic {
  subTitle?: string
  maxPreviewPage?: number
  previewPages: PreviewPageItem[] // 👈 使用新的接口类型
  comments: { id: number; user: string; date: string; content: string }[]
}

const comic = ref<GalleryDetail>({
  id: (route.query.id as string) || '',
  token: (route.query.token as string) || '',
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

// 1. 获取画廊真实详情 (仅抓取 p=0 基础元数据与初始预览图)
const fetchDetail = async () => {
  const gid = route.query.id as string
  const token = route.query.token as string

  if (!gid || !token) {
    toast.error('画廊 ID 或 Token 参数缺失！')
    isLoading.value = false
    return
  }

  isLoading.value = true
  try {
    const data = await http('/comics/online/detail', {
      params: { id: gid, token },
    })

    // 1. fetchDetail 内部的映射：
    const formattedInitialPreviews = (data.previewPages || []).map((item: any) => ({
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
      isFavorite: !!data.isFavorite,
      favIndex: data.favIndex ?? 0,
      tags: data.tags || [],
      previewPages: formattedInitialPreviews, // 👈 使用映射后的预览图列表
      comments: data.comments || [],
    }

    maxPreviewPage.value = data.maxPreviewPage || 1
    currentPreviewPage.value = 1

    addHistory(comic.value)
  } catch (err: any) {
    toast.error(err.message || '获取画廊详情失败')
  } finally {
    isLoading.value = false
  }
}

// 🟢 2. 新增：增量请求下一页预览图
const handleLoadMorePreviews = async () => {
  if (isLoadingMorePreviews.value || currentPreviewPage.value >= maxPreviewPage.value) return

  isLoadingMorePreviews.value = true
  const nextPage = currentPreviewPage.value + 1

  try {
    const newPreviews = await http<any[]>('/comics/online/previews', {
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
  } catch (err: any) {
    toast.error(err.message || '加载更多切片失败')
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

// 5. 点击预览切片直接跳页阅读
const handleStartReading = (targetPage: number = 1) => {
  router.push({
    path: '/reader',
    query: {
      id: comic.value.id,
      token: comic.value.token,
      source: 'online',
      page: targetPage,
    },
  })
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
      } catch (err: any) {
        toast.error(err.message || '设置收藏失败')
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
  } catch (err: any) {
    toast.error(err.message || '取消收藏失败')
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
</script>

<template>
  <div class="detail-page">
    <div v-if="isLoading" class="loading-state">加载中...</div>

    <template v-else>
      <div class="top-action-bar">
        <button class="back-btn" @click="handleBack">‹ 返回</button>

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

          <button class="read-btn" @click="handleStartReading(1)">📖 立即阅读</button>
        </div>
      </div>

      <!-- 标题与英文/译名标题 -->
      <div class="title-header">
        <h1 class="comic-main-title">{{ comic.title }}</h1>
        <h2 v-if="comic.subTitle && comic.subTitle !== comic.title" class="comic-sub-title">
          {{ comic.subTitle }}
        </h2>
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
      <div v-if="activeTab === 'comments'" class="tab-content comments-tab">
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
  </div>
</template>

<style scoped>
.detail-page {
  padding: 20px;
  max-width: 1100px;
  margin: 0 auto;
  color: #e0e0e0;
}

.loading-state,
.empty-box {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 240px;
  color: #777;
  font-size: 14px;
}

.top-action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.back-btn {
  background: transparent;
  border: 1px solid #333;
  color: #aaa;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
}
.back-btn:hover {
  color: #fff;
  border-color: #555;
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
  background: #26262a;
  color: #ccc;
  border: 1px solid #444;
}

.read-btn {
  background: #00a896;
  color: #fff;
  font-weight: bold;
}

.title-header {
  margin-bottom: 16px;
}

.comic-main-title {
  font-size: 1.3rem;
  margin: 0 0 4px 0;
  color: #fff;
  line-height: 1.4;
}

.comic-sub-title {
  font-size: 0.95rem;
  margin: 0;
  color: #88888c;
  font-weight: normal;
  line-height: 1.3;
}

/* Tabs */
.detail-tabs {
  display: flex;
  gap: 12px;
  border-bottom: 1px solid #2a2a2a;
  margin-bottom: 20px;
}

.tab-item {
  background: transparent;
  border: none;
  color: #888;
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
  background: #1a1a1d;
  padding: 20px;
  border-radius: 8px;
  border: 1px solid #2a2a2d;
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
  color: #888;
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
  background: #2a2a2d;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.8rem;
}

.tags-section {
  margin-top: 12px;
}

.section-title {
  font-size: 0.95rem;
  color: #aaa;
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
  color: #77777c;
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
  color: #666;
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
  border: 1px solid #2a2a2a;
  cursor: pointer;
  transition: all 0.2s ease;

  /* 🟢 新增：弹性布局，保证雪碧图切片块居中对齐 */
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: #121214;
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
  color: #eee;
}

/* 🟢 加载更多按钮样式 */
.load-more-box {
  display: flex;
  justify-content: center;
  padding: 12px 0 24px;
}

.load-more-btn {
  background: #202024;
  border: 1px solid #333338;
  color: #007acc;
  padding: 10px 28px;
  border-radius: 6px;
  font-size: 0.9rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.load-more-btn:hover:not(:disabled) {
  background-color: #28282e;
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
  background: #1a1a1d;
  padding: 12px 16px;
  border-radius: 6px;
  border: 1px solid #2a2a2d;
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
  color: #666;
}

.comment-body {
  margin: 0;
  font-size: 0.9rem;
  color: #ccc;
  line-height: 1.4;
}

.add-reading-btn {
  background: #242428;
  border: 1px solid #3a3a3d;
  color: #ccc;
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 0.88rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.add-reading-btn:hover {
  background-color: #2e2e33;
  border-color: #007acc;
  color: #007acc;
}

.add-reading-btn.active {
  background-color: rgba(0, 122, 204, 0.2);
  border-color: #007acc;
  color: #007acc;
  font-weight: bold;
}
</style>
