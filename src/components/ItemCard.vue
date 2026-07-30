<script setup lang="ts">
import { ref, computed } from 'vue'
import { viewMode } from '@/stores/viewMode'
import { useRouter } from 'vue-router'
import type { ComicItem, OnlineComic } from '@/types/comic'
import { addHistory } from '@/stores/appStore' // 🟢 引入历史记录函数

interface RankedComicItem extends ComicItem {
  rank?: number
}

const props = defineProps<{
  comic: RankedComicItem
  mode?: 'card' | 'compact'
  size?: 'large' | 'normal' | 'small'
}>()

const router = useRouter()

// 🟢 核心交互状态：是否展开 Tag 面板 (点击封面图切换)
const showTags = ref(false)

const toggleTags = () => {
  showTags.value = !showTags.value
}

// 当前生效的展示模式 (默认为全局 viewMode，可被 prop 覆盖)
const currentMode = computed(() => props.mode || viewMode.value)

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
// 3. 点击卡片主体（封面图除外）跳转详情页
// --------------------------------------------------
const handleCardClick = () => {
  if (!props.comic || !props.comic.id) return
  addHistory(props.comic)
  if (props.comic.source === 'online') {
    router.push(`/online/detail?id=${props.comic.id}`)
  } else {
    router.push(`/offline/detail?id=${props.comic.id}`)
  }
}
</script>

<template>
  <div class="item-card" :class="[currentMode, size || 'normal']" @click="handleCardClick">
    <template v-if="currentMode === 'compact'">
      <div
        class="compact-thumb-box"
        @click.stop="toggleTags"
        :title="showTags ? '点击收起 Tag' : '点击查看完整 Tag 列表'"
      >
        <img :src="comic.coverUrl" :alt="comic.title" class="thumb-img" loading="lazy" />

        <span v-if="comic.rank" class="rank-badge" :class="{ 'top-3': comic.rank <= 3 }">
          #{{ comic.rank }}
        </span>

        <span class="tag-indicator" :class="{ active: showTags }">
          {{ showTags ? '▲ 隐' : '🏷️ Tag' }}
        </span>
      </div>

      <div class="compact-main-content">
        <h4 class="compact-title" :title="comic.title">
          {{ comic.title }}
        </h4>

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

            <span class="pages-text">{{ comic.pageCount || 0 }} 页</span>

            <span class="date-text">{{ comic.updatedAt }}</span>
          </div>

          <div class="status-row">
            <span v-if="comic.isDownloaded" class="downloaded-badge"> ✓ 已下载 </span>
          </div>
        </div>

        <div v-else class="compact-tags-panel" @click.stop>
          <div class="tags-scroll-container">
            <span v-for="tag in comic.tags" :key="tag" class="tag-chip">
              {{ tag }}
            </span>
            <span v-if="!comic.tags || comic.tags.length === 0" class="empty-tag-text">
              暂无标签
            </span>
          </div>
        </div>
      </div>
    </template>

    <template v-else>
      <div class="card-cover-wrapper">
        <img :src="comic.coverUrl" :alt="comic.title" class="cover-img" loading="lazy" />
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
        <span class="card-pages-badge">{{ comic.pageCount || 0 }}P</span>
      </div>

      <div class="card-info-footer">
        <h4 class="card-title" :title="comic.title">{{ comic.title }}</h4>
        <div class="card-tags-row">
          <span v-for="tag in (comic.tags || []).slice(0, 3)" :key="tag" class="card-tag">
            {{ tag }}
          </span>
        </div>
        <div class="card-bottom-meta">
          <span class="rating">⭐ {{ comic.rating || '5.0' }}</span>
          <span class="source-tag" :class="comic.source">
            {{ comic.source === 'online' ? '在线' : '本地' }}
          </span>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.item-card {
  background-color: #1a1a1e;
  border: 1px solid #26262a;
  border-radius: 8px;
  cursor: pointer;
  user-select: none;
  transition: all 0.2s ease;
  overflow: hidden;
}

.item-card:hover {
  background-color: #222226;
  border-color: #38383e;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}

/* ============================================================== */
/* 🪪 EHViewer 经典名片模式 (Compact) */
/* ============================================================== */
.item-card.compact {
  display: flex;
  height: 115px; /* 固定标准名片高度 */
  padding: 8px;
  gap: 12px;
  box-sizing: border-box;
}

/* 1. 左侧缩略图容器 */
.compact-thumb-box {
  position: relative;
  width: 80px;
  height: 100%;
  flex-shrink: 0;
  border-radius: 6px;
  overflow: hidden;
  background-color: #121214;
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
  color: #a0a0a5;
  font-size: 10px;
  font-weight: 700;
  padding: 1px 4px;
  border-radius: 3px;
}

.rank-badge.top-3 {
  color: #ffd700;
  background-color: rgba(0, 0, 0, 0.85);
}

/* 封面上的 Tag 开关浮层提示 */
.tag-indicator {
  position: absolute;
  bottom: 4px;
  right: 4px;
  background-color: rgba(0, 0, 0, 0.75);
  color: #d0d0d0;
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  transition: all 0.2s ease;
}

.tag-indicator.active {
  background-color: #ff7588;
  color: #ffffff;
}

/* 2. 右侧主体内容区 */
.compact-main-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-width: 0; /* 允许 flex 子项缩小以截断文本 */
}

.compact-title {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: #ffffff;
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* A. 常态视图面板 */
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
  color: #88888c;
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
  color: #77777c;
  font-size: 11px;
}

.status-row {
  min-height: 18px;
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

/* B. 展开态：Tag 滚动面板 */
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

/* 细体滚动条 */
.tags-scroll-container::-webkit-scrollbar {
  width: 4px;
}
.tags-scroll-container::-webkit-scrollbar-thumb {
  background-color: #38383e;
  border-radius: 2px;
}

.tag-chip {
  background-color: #26262a;
  color: #a0a0a5;
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 3px;
  border: 1px solid #333338;
}

.empty-tag-text {
  font-size: 11px;
  color: #55555a;
}

/* ============================================================== */
/* 🎴 大卡片模式 (Card Mode) */
/* ============================================================== */
.item-card.card {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.card-cover-wrapper {
  position: relative;
  width: 100%;
  aspect-ratio: 3 / 4;
  background-color: #121214;
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

.card-info-footer {
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 1;
}

.card-title {
  margin: 0;
  font-size: 13px;
  font-weight: 500;
  color: #ffffff;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  line-height: 1.3;
}

.card-tags-row {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.card-tag {
  background-color: #26262a;
  color: #88888c;
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
}

.card-bottom-meta {
  margin-top: auto;
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 11px;
  color: #66666c;
}

.source-tag.online {
  color: #a891e3;
}

.source-tag.offline {
  color: #ff7588;
}
</style>
