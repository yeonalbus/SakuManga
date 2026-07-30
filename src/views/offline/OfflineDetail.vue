<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
// 🎯 核心引入：读取 bookshelves 以及离线清单响应状态与切换函数
import { bookshelves, offlineReadingList, toggleReadingList } from '@/stores/appStore'
import type { OfflineComic } from '@/types/comic'
import { recordComicClick } from '@/stores/appStore'

const route = useRoute()
const router = useRouter()
const { toast, modal } = useUI()

// 1. 模拟本地离线本子详情数据
const comic = ref<OfflineComic>({
  id: (route.query.id as string) || 'offline-201',
  title: '📖 [本地扫描] 深度学习资料包作品 Vol.01',
  coverUrl: 'https://via.placeholder.com/300x400/222225/cccccc?text=Offline+Doc',
  source: 'offline',
  category: 'Doujinshi', // 👈 补全分类字段
  tags: ['female:nun', 'language:chinese', 'artist:hiten', 'full color', '高清解压'],
  rating: 4.2,
  pageCount: 142,
  updatedAt: '2026-07-28 20:15',
  localPath: 'D:/Comics/Collection_2026/Vol_01.zip',
  fileSize: 184549376,
  readCount: 18,
  needsUpdate: false,
})

// 分类颜色的映射表 (对齐 E 站经典分类色彩)
const getCategoryColor = (cat?: string) => {
  switch (cat) {
    case 'Doujinshi':
      return '#ff7588'
    case 'Manga':
      return '#ff9800'
    case 'Artist CG':
      return '#e91e63'
    case 'Game CG':
      return '#4caf50'
    case 'Non-H':
      return '#2196f3'
    default:
      return '#9e9e9e'
  }
}

// --------------------------------------------------
// 📑 本地阅读清单状态响应
// --------------------------------------------------
// 动态判断当前本地作品是否在离线清单中
const isInReadingList = computed(() =>
  offlineReadingList.value.some((item) => item.id === comic.value.id),
)

// 切换加入 / 移除本地清单
const handleAddToReadingList = () => {
  toggleReadingList(comic.value)
  if (isInReadingList.value) {
    toast.success(`已将《${comic.value.title}》加入本地阅读清单 📑`)
  } else {
    toast.info(`已从本地阅读清单中移除《${comic.value.title}》`)
  }
}

// --------------------------------------------------
// 其他函数保持不变
// --------------------------------------------------
const handleBack = () => {
  if (window.history.length > 1) {
    router.back()
  } else {
    router.push('/offline/home')
  }
}

const formattedSize = computed(() => {
  if (!comic.value.fileSize) return '未知'
  const mb = comic.value.fileSize / (1024 * 1024)
  return `${mb.toFixed(1)} MB`
})

const myLocalRating = ref(comic.value.rating || 0)
const setRating = (star: number) => {
  myLocalRating.value = star
  comic.value.rating = star
  toast.success(`个人打分更新为 ${star} 星`)
}

const copyPath = () => {
  navigator.clipboard.writeText(comic.value.localPath)
  toast.info('本地文件路径已复制到剪贴板')
}

const handleAddTag = async () => {
  const newTag = await modal.prompt(
    '请输入要新增的 Tag 名称（如 artist:xxx）：',
    '',
    '新增本地标签',
  )
  if (newTag && newTag.trim()) {
    const trimmed = newTag.trim()
    if (!comic.value.tags.includes(trimmed)) {
      comic.value.tags.push(trimmed)
      toast.success(`标签「${trimmed}」添加成功！`)
    } else {
      toast.warning('该标签已存在')
    }
  }
}

const handleRemoveTag = (index: number) => {
  const removed = comic.value.tags.splice(index, 1)
  toast.info(`已移除标签「${removed[0]}」`)
}

const toggleShelfCheck = (shelfId: string) => {
  const shelf = bookshelves.value.find((s) => s.id === shelfId)
  if (!shelf) return

  if (!shelf.comicIds) shelf.comicIds = []
  const idx = shelf.comicIds.indexOf(comic.value.id)

  if (idx >= 0) {
    shelf.comicIds.splice(idx, 1)
    shelf.count = Math.max(0, shelf.count - 1)
    toast.info(`从书架「${shelf.name}」中移出`)
  } else {
    shelf.comicIds.push(comic.value.id)
    shelf.count++
    toast.success(`已加入书架「${shelf.name}」`)
  }
}

const isComicInShelf = (shelfId: string) => {
  const shelf = bookshelves.value.find((s) => s.id === shelfId)
  return shelf?.comicIds?.includes(comic.value.id) ?? false
}

const handleStartReading = () => {
  if (!comic.value || !comic.value.id) return

  // 1) 触发 Store 中的全局数据自增（写回 LocalStorage，更新排行榜）
  recordComicClick(comic.value.id)

  // 2) 实时更新当前页面组件的 readCount，让 UI 上的 "(已读 X 次)" 瞬间刷新
  comic.value.readCount = (comic.value.readCount || 0) + 1

  // 3) 正式跳转进入阅读器
  router.push(`/reader?id=${comic.value.id}&source=offline`)
}
</script>

<template>
  <div class="offline-detail-page">
    <div class="top-bar">
      <button class="back-btn" @click="handleBack">‹ 返回</button>
      <div class="right-actions">
        <!-- 🟢 动态本地阅读清单按钮 -->
        <button
          class="add-reading-btn"
          :class="{ active: isInReadingList }"
          @click="handleAddToReadingList"
        >
          {{ isInReadingList ? '✓ 已在清单' : '📑 加入清单' }}
        </button>

        <button class="read-btn" @click="handleStartReading">
          📖 继续阅读 (已读 {{ comic.readCount || 0 }} 次)
        </button>
      </div>
    </div>

    <div class="main-layout">
      <div class="left-cover">
        <img :src="comic.coverUrl" :alt="comic.title" />
      </div>

      <div class="right-panel">
        <div v-if="comic.category" class="category-wrapper">
          <span
            class="category-badge"
            :style="{ backgroundColor: getCategoryColor(comic.category) }"
          >
            {{ comic.category }}
          </span>
        </div>

        <h1 class="title">{{ comic.title }}</h1>

        <div class="rating-box">
          <span class="label">我的个人评分：</span>
          <div class="stars">
            <span
              v-for="star in 5"
              :key="star"
              class="star-icon"
              :class="{ active: star <= myLocalRating }"
              @click="setRating(star)"
            >
              ★
            </span>
          </div>
          <span class="rating-num">({{ myLocalRating }} / 5.0)</span>
        </div>

        <div class="info-card">
          <div class="card-header">
            <h3 class="card-title">🏷️ 本地作品标签 (Tags)</h3>
            <button class="add-tag-btn" @click="handleAddTag">➕ 添加 Tag</button>
          </div>

          <div class="tags-cloud">
            <span v-for="(tag, idx) in comic.tags" :key="tag" class="tag-chip">
              <span class="tag-text">{{ tag }}</span>
              <span class="remove-tag" title="删除此标签" @click.stop="handleRemoveTag(idx)"
                >✕</span
              >
            </span>

            <span v-if="!comic.tags || comic.tags.length === 0" class="empty-tag-tip">
              暂无标签，点击右上方按钮添加...
            </span>
          </div>
        </div>

        <div class="info-card">
          <h3 class="card-title">💾 本地文件属性</h3>
          <div class="info-grid">
            <div class="info-item">
              <span class="k">文件大小:</span>
              <span class="v">{{ formattedSize }}</span>
            </div>
            <div class="info-item">
              <span class="k">总页数:</span>
              <span class="v">{{ comic.pageCount }} 页</span>
            </div>
            <div class="info-item">
              <span class="k">最后扫描:</span>
              <span class="v">{{ comic.updatedAt }}</span>
            </div>
            <div class="info-item full">
              <span class="k">存储路径:</span>
              <span class="v path-text">{{ comic.localPath }}</span>
              <button class="copy-btn" @click="copyPath">复制</button>
            </div>
          </div>
        </div>

        <div class="info-card">
          <h3 class="card-title">📚 归属本地书架 (可多选)</h3>
          <div class="bookshelves-list">
            <div
              v-for="shelf in bookshelves"
              :key="shelf.id"
              class="shelf-checkbox-item"
              :class="{ checked: isComicInShelf(shelf.id) }"
              @click="toggleShelfCheck(shelf.id)"
            >
              <input type="checkbox" :checked="isComicInShelf(shelf.id)" readonly />
              <span class="shelf-name">{{ shelf.name }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.offline-detail-page {
  padding: 20px;
  max-width: 1000px;
  margin: 0 auto;
  color: #e0e0e0;
}

.top-bar {
  display: flex;
  justify-content: space-between; /* 两端对齐：左边放返回，右边放按钮组 */
  align-items: center;
  margin-bottom: 20px;
}

.back-btn {
  background: transparent;
  border: 1px solid #333;
  color: #aaa;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}
.back-btn:hover {
  color: #fff;
  border-color: #555;
}

.read-btn {
  background: #00a896;
  color: #fff;
  border: none;
  padding: 8px 20px;
  border-radius: 6px;
  font-weight: bold;
  cursor: pointer;
}

.main-layout {
  display: flex;
  gap: 28px;
}

.left-cover img {
  width: 240px;
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.6);
  border: 1px solid #333;
}

.right-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.title {
  font-size: 1.3rem;
  margin: 0;
  color: #fff;
}

/* 打分 */
.rating-box {
  display: flex;
  align-items: center;
  gap: 10px;
  background: #1a1a1d;
  padding: 10px 14px;
  border-radius: 6px;
  border: 1px solid #2a2a2d;
}

.stars {
  display: flex;
  gap: 4px;
  cursor: pointer;
}
.star-icon {
  font-size: 1.2rem;
  color: #444;
  transition: color 0.15s;
}
.star-icon.active {
  color: #ffc107;
}
.rating-num {
  color: #888;
  font-size: 0.85rem;
}

/* 基础卡片 */
.info-card {
  background: #1a1a1d;
  border: 1px solid #2a2a2d;
  border-radius: 8px;
  padding: 16px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.card-title {
  font-size: 0.95rem;
  color: #aaa;
  margin: 0;
}

/* Tag 标签云样式 */
.add-tag-btn {
  background: transparent;
  border: 1px dashed #007acc;
  color: #007acc;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.2s;
}
.add-tag-btn:hover {
  background: rgba(0, 122, 204, 0.2);
}

.tags-cloud {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tag-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(0, 122, 204, 0.15);
  color: #007acc;
  border: 1px solid rgba(0, 122, 204, 0.3);
  padding: 3px 10px;
  border-radius: 12px;
  font-size: 0.8rem;
}

.remove-tag {
  font-size: 0.75rem;
  color: #888;
  cursor: pointer;
  border-radius: 50%;
  padding: 0 2px;
}
.remove-tag:hover {
  color: #ef4444;
}

.empty-tag-tip {
  font-size: 0.8rem;
  color: #555;
  font-style: italic;
}

/* 文件属性网格 */
.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  font-size: 0.88rem;
  margin-top: 12px;
}

.info-item {
  display: flex;
  gap: 8px;
}
.info-item.full {
  grid-column: span 2;
  align-items: center;
}
.info-item .k {
  color: #888;
}
.info-item .v {
  color: #ddd;
}

.path-text {
  font-family: monospace;
  background: #242428;
  padding: 2px 8px;
  border-radius: 4px;
  color: #007acc;
  font-size: 0.8rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 320px;
}

.copy-btn {
  background: #2a2a2e;
  border: 1px solid #444;
  color: #ccc;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.75rem;
  cursor: pointer;
  margin-left: 6px;
}

/* 书架勾选列表 */
.bookshelves-list {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 12px;
}

.shelf-checkbox-item {
  display: flex;
  align-items: center;
  gap: 8px;
  background: #242428;
  border: 1px solid #333;
  padding: 6px 12px;
  border-radius: 6px;
  cursor: pointer;
  user-select: none;
  transition: all 0.15s;
}

.shelf-checkbox-item:hover {
  border-color: #007acc;
}

.shelf-checkbox-item.checked {
  background: rgba(0, 122, 204, 0.15);
  border-color: #007acc;
  color: #007acc;
}

.right-actions {
  display: flex;
  align-items: center;
  gap: 12px; /* 📑 加入清单 和 📖 继续阅读 之间的间距 */
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

/* 🟢 新增：已加入本地清单的高亮状态 */
.add-reading-btn.active {
  background-color: rgba(0, 122, 204, 0.2);
  border-color: #007acc;
  color: #007acc;
  font-weight: bold;
}

/* 按钮通用基础微调 */
.read-btn {
  flex-shrink: 0;
}

/* 分类 Badge 样式 */
.category-wrapper {
  margin-bottom: 8px;
}

.category-badge {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 4px;
  color: #ffffff;
  font-size: 0.8rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
}
</style>
