<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
import type { OnlineComic } from '@/types/comic'
import TagChip from '@/components/TagChip.vue'
import { onlineReadingList, toggleReadingList, addHistory } from '@/stores/appStore'

const route = useRoute()
const router = useRouter()
const { toast, modal } = useUI()

const activeTab = ref<'info' | 'preview' | 'comments'>('info')
const isLoading = ref(true)

// 详情页扩展数据模型
interface GalleryDetail extends OnlineComic {
  subTitle?: string
  previewPages: { pageIndex: number; url: string }[]
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

// 1. 获取画廊真实详情
const fetchDetail = async () => {
  const gid = route.query.id as string
  const token = route.query.token as string

  if (!gid || !token) {
    toast.error('画廊 ID 或 Token 参数缺失！')
    return
  }

  isLoading.value = true
  try {
    const res = await fetch(
      `http://localhost:8081/api/v1/comics/online/detail?id=${gid}&token=${token}`,
    )
    const data = await res.json()

    if (res.ok) {
      comic.value = {
        ...data,
        isFavorite: !!data.isFavorite, // 强转为 boolean
        favIndex: data.favIndex ?? 0, // 使用 ?? 空值合并运算符，避免 0 被当作 false
        tags: data.tags || [],
        previewPages: data.previewPages || [],
        comments: data.comments || [],
      }
      addHistory(comic.value)
    } else {
      toast.error(data.error || '获取详情失败')
    }
  } catch (err) {
    toast.error('网络连接失败')
  } finally {
    isLoading.value = false
  }
}

// 2. 标签按 Namespace 智能分组 (画廊标准排版)
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

// 3. 阅读清单与动作响应
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

// 🟢 1. 点击选择收藏夹 (0 ~ 9)
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
        const res = await fetch('http://localhost:8081/api/v1/comics/online/favorite', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            gid: comic.value.id,
            token: comic.value.token,
            favCat: idx,
            note: '',
          }),
        })

        if (res.ok) {
          comic.value.isFavorite = true
          comic.value.favIndex = idx
          toast.success(`已成功存入 Favorite ${idx}`)
        } else {
          const errData = await res.json()
          toast.error(errData.error || '设置收藏失败')
        }
      } catch {
        toast.error('网络请求失败')
      }
    } else {
      toast.error('请输入 0 到 9 之间的数字')
    }
  }
}

// 🟢 2. 长按取消收藏
const handleRemoveFavorite = async () => {
  if (!comic.value.isFavorite) return

  const confirm = window.confirm(`确定要从收藏夹移除《${comic.value.title}》吗？`)
  if (!confirm) return

  try {
    const res = await fetch('http://localhost:8081/api/v1/comics/online/favorite', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        gid: comic.value.id,
        token: comic.value.token,
      }),
    })

    if (res.ok) {
      comic.value.isFavorite = false
      toast.success('已从收藏夹移除')
    } else {
      const errData = await res.json()
      toast.error(errData.error || '取消收藏失败')
    }
  } catch {
    toast.error('网络请求失败')
  }
}

// 🟢 3. 长按判定逻辑
let pressTimer: number | null = null
let isLongPress = false

const handlePressStart = () => {
  isLongPress = false
  pressTimer = window.setTimeout(() => {
    isLongPress = true
    handleRemoveFavorite()
  }, 700) // 长按超过 700ms 识别为取消收藏
}

const handlePressEnd = () => {
  if (pressTimer) {
    clearTimeout(pressTimer)
    pressTimer = null
  }
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
          🖼️ 预览切片 ({{ comic.previewPages?.length || 0 }}P)
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
        <div v-if="comic.previewPages?.length" class="preview-grid">
          <div
            v-for="page in comic.previewPages"
            :key="page.pageIndex"
            class="preview-card"
            @click="handleStartReading(page.pageIndex)"
          >
            <img :src="page.url" loading="lazy" referrerpolicy="no-referrer" />
            <span class="page-num">P{{ page.pageIndex }}</span>
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
}

.preview-card img {
  width: 100%;
  height: 100%;
  object-fit: cover;
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
