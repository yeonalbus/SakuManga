<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
import type { OnlineComic } from '@/types/comic'
// 🎯 核心引入：读取全局在线清单与切换函数
import { onlineReadingList, toggleReadingList } from '@/stores/appStore'
import { addHistory, recordComicClick } from '@/stores/appStore'

const route = useRoute()
const router = useRouter()
const { toast, modal } = useUI()

// 1. 当前选中的 Tab ('info' | 'preview' | 'comments')
const activeTab = ref<'info' | 'preview' | 'comments'>('info')

// 2. 模拟当前的在线漫画详情数据
const comic = ref<OnlineComic>({
  id: (route.query.id as string) || 'online-101',
  title: '[fau0101] みんなの頼れる修道女 [中文汉化] [全彩]',
  coverUrl: 'https://via.placeholder.com/300x400/1e293b/ffffff?text=Nun+Comic',
  source: 'online',
  tags: [
    'female:big breasts',
    'female:nun',
    'female:hood',
    'male:vore',
    'language:chinese',
    'full color',
  ],
  rating: 4.6,
  pageCount: 83,
  updatedAt: '2026-07-29 13:29',
  category: 'Doujinshi',
  uploader: 'zhangyuan016',
  isFavorite: true,
  favIndex: 2, // Fav 2: 橙色
  isDownloaded: false,
})

// --------------------------------------------------
// 📑 阅读清单连贯状态响应
// --------------------------------------------------
// 动态计算当前作品是否已经在清单中
const isInReadingList = computed(() =>
  onlineReadingList.value.some((item) => item.id === comic.value.id),
)

// 点击切换：加入或移出清单
const handleAddToReadingList = () => {
  toggleReadingList(comic.value)
  if (isInReadingList.value) {
    toast.success(`已将《${comic.value.title}》加入在线阅读清单 📑`)
  } else {
    toast.info(`已从在线阅读清单中移除《${comic.value.title}》`)
  }
}

// --------------------------------------------------
// 其他已有函数保持不变...
// --------------------------------------------------
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

const parsedTags = computed(() => {
  return comic.value.tags.map((t) => {
    if (t.includes(':')) {
      const [ns, name] = t.split(':')
      return { ns, name, raw: t }
    }
    return { ns: 'misc', name: t, raw: t }
  })
})

const previewPages = Array.from({ length: comic.value.pageCount || 24 }, (_, i) => ({
  pageIndex: i + 1,
  url: `https://via.placeholder.com/150x200/222225/888888?text=P.${i + 1}`,
}))

const comments = ref([
  { id: 1, user: 'E_Master', date: '2026-07-29 14:10', content: '画风极其精致！', score: 5 },
  { id: 2, user: 'Knight_9', date: '2026-07-29 15:30', content: '推荐下载保存。', score: 4.5 },
])
const newComment = ref('')

const handleSelectFavorite = async () => {
  const chosenIndex = await modal.prompt(
    '请选择收藏夹 (输入 0 ~ 9)：',
    String(comic.value.favIndex ?? 0),
    '设置在线收藏',
  )
  if (chosenIndex !== null) {
    const idx = parseInt(chosenIndex, 10)
    if (!isNaN(idx) && idx >= 0 && idx <= 9) {
      comic.value.isFavorite = true
      comic.value.favIndex = idx
      toast.success(`已保存至 Favorite ${idx}`)
    } else {
      toast.error('请输入 0 到 9 之间的数字')
    }
  }
}

const handleDownload = () => {
  comic.value.isDownloaded = true
  toast.success('已加入下载队列，正在后台离线下载...')
}

const handleAddComment = () => {
  if (!newComment.value.trim()) return
  comments.value.unshift({
    id: Date.now(),
    user: '我 (User)',
    date: '刚刚',
    content: newComment.value.trim(),
    score: 5,
  })
  newComment.value = ''
  toast.success('评论发表成功')
}

const handleStartReading = () => {
  router.push({
    path: '/reader',
    query: {
      id: comic.value.id,
      source: 'online',
    },
  })
}
</script>

<template>
  <div class="detail-page">
    <div class="top-action-bar">
      <button class="back-btn" @click="handleBack">‹ 返回</button>

      <!-- 顶部右侧动作栏区域 -->
      <div class="right-actions">
        <!-- 🟢 动态加入/移除清单按钮 -->
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
              ? { backgroundColor: favColors[comic.favIndex || 0], color: '#fff' }
              : {}
          "
          @click="handleSelectFavorite"
        >
          ❤️ {{ comic.isFavorite ? `Fav ${comic.favIndex}` : '加入收藏' }}
        </button>

        <button
          class="action-btn dl-btn"
          :class="{ downloaded: comic.isDownloaded }"
          @click="handleDownload"
        >
          {{ comic.isDownloaded ? '✓ 已离线' : '⬇️ 离线下载' }}
        </button>

        <button class="read-btn" @click="handleStartReading">📖 立即阅读</button>
      </div>
    </div>

    <h1 class="comic-main-title">{{ comic.title }}</h1>

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
        🖼️ 预览切片 ({{ comic.pageCount }}P)
      </button>
      <button
        class="tab-item"
        :class="{ active: activeTab === 'comments' }"
        @click="activeTab = 'comments'"
      >
        💬 社区评论 ({{ comments.length }})
      </button>
    </div>

    <div v-if="activeTab === 'info'" class="tab-content info-tab">
      <div class="info-layout">
        <div class="cover-box">
          <img :src="comic.coverUrl" :alt="comic.title" />
        </div>

        <div class="metadata-box">
          <div class="meta-row">
            <span class="label">上传作者:</span>
            <span class="value link">{{ comic.uploader }}</span>
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
            <span class="label">发布时间:</span>
            <span class="value">{{ comic.updatedAt }}</span>
          </div>

          <div class="tags-section">
            <h3 class="section-title">🏷️ Tag 属性云</h3>
            <div class="tag-cloud">
              <div v-for="tag in parsedTags" :key="tag.raw" class="tag-chip">
                <span class="tag-ns">{{ tag.ns }}</span>
                <span class="tag-name">{{ tag.name }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-if="activeTab === 'preview'" class="tab-content preview-tab">
      <div class="preview-grid">
        <div
          v-for="page in previewPages"
          :key="page.pageIndex"
          class="preview-card"
          @click="toast.info(`跳转至第 ${page.pageIndex} 页阅读`)"
        >
          <img :src="page.url" loading="lazy" />
          <span class="page-num">{{ page.pageIndex }}</span>
        </div>
      </div>
    </div>

    <div v-if="activeTab === 'comments'" class="tab-content comments-tab">
      <div class="comment-input-box">
        <textarea v-model="newComment" placeholder="分享你的阅读感受..." rows="3"></textarea>
        <button class="send-btn" @click="handleAddComment">发表评论</button>
      </div>

      <div class="comments-list">
        <div v-for="item in comments" :key="item.id" class="comment-card">
          <div class="comment-header">
            <span class="user-name">{{ item.user }}</span>
            <span class="comment-time">{{ item.date }}</span>
          </div>
          <p class="comment-body">{{ item.content }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.detail-page {
  padding: 20px;
  max-width: 1100px;
  margin: 0 auto;
  color: #e0e0e0;
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
.dl-btn {
  background: #007acc;
  color: #fff;
}
.dl-btn.downloaded {
  background: #10b981;
}
.read-btn {
  background: #00a896;
  color: #fff;
  font-weight: bold;
}

.comic-main-title {
  font-size: 1.3rem;
  margin: 0 0 16px 0;
  color: #fff;
  line-height: 1.4;
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

/* Tab 1: Info */
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
  margin-top: 16px;
}
.section-title {
  font-size: 0.95rem;
  color: #aaa;
  margin-bottom: 10px;
}

.tag-cloud {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.tag-chip {
  display: inline-flex;
  font-size: 0.78rem;
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid #333;
}
.tag-ns {
  background: #2a2a30;
  color: #888;
  padding: 2px 8px;
}
.tag-name {
  background: rgba(0, 122, 204, 0.2);
  color: #007acc;
  padding: 2px 8px;
}

/* Tab 2: Preview */
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
}
.preview-card img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.preview-card:hover {
  border-color: #007acc;
  transform: scale(1.02);
}
.page-num {
  position: absolute;
  bottom: 4px;
  right: 4px;
  background: rgba(0, 0, 0, 0.7);
  padding: 1px 6px;
  font-size: 0.7rem;
  border-radius: 3px;
}

/* Tab 3: Comments */
.comment-input-box {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 20px;
}

.comment-input-box textarea {
  background: #1e1e22;
  border: 1px solid #333;
  color: #fff;
  padding: 10px;
  border-radius: 6px;
  outline: none;
}

.send-btn {
  align-self: flex-end;
  background: #007acc;
  color: #fff;
  border: none;
  padding: 6px 16px;
  border-radius: 4px;
  cursor: pointer;
}

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

/* 🟢 新增：已加入清单的高亮状态 */
.add-reading-btn.active {
  background-color: rgba(0, 122, 204, 0.2);
  border-color: #007acc;
  color: #007acc;
  font-weight: bold;
}
</style>
