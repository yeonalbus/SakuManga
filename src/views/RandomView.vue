<script setup lang="ts">
import { ref, computed } from 'vue'
import { useUI } from '@/composables/useUI'
import ItemCard from '@/components/ItemCard.vue'
import { fetchRandomComicsApi } from '@/api/comic'
import { onlineSearchConfig, offlineSearchConfig } from '@/stores/searchStore'
import { useModeStore } from '@/stores/modeStore'
import type {
  ComicItem,
  OnlineComic,
  OfflineComic,
  RandomComicItem,
  RandomComicParams,
} from '@/types/comic'

const modeStore = useModeStore()
const { toast } = useUI()

// 1. 数量控制逻辑
const selectedCountOption = ref<1 | 4 | 8 | 'custom'>(1)
const customCountInput = ref<number>(12) // 自定义输入的数量

const targetCount = computed(() => {
  if (selectedCountOption.value === 'custom') {
    return Math.max(1, customCountInput.value || 1)
  }
  return selectedCountOption.value
})

// 2. 范围制定控制（all/online/offline）
const scopeType = ref<'all' | 'online' | 'offline'>('all')

// 3. 抽卡状态与结果存储
const isSpinning = ref(false)
const hasDrawn = ref(false)
const drawnComics = ref<ComicItem[]>([])

// 后端统一 DTO → 前端卡片渲染项
const toComicItem = (item: RandomComicItem): ComicItem => {
  if (item.source === 'online') {
    return {
      id: item.id,
      title: item.title,
      coverUrl: item.coverUrl,
      source: 'online',
      tags: item.tags ?? [],
      category: item.category,
      rating: item.rating,
      pageCount: item.pageCount,
      readCount: item.readCount,
      updatedAt: item.updatedAt,
      isDownloaded: item.isDownloaded,
      token: item.token,
      uploader: item.uploader,
      isFavorite: item.isFavorite,
    } as OnlineComic
  }
  return {
    id: item.id,
    title: item.title,
    coverUrl: item.coverUrl,
    source: 'offline',
    tags: item.tags ?? [],
    category: item.category,
    rating: item.rating,
    pageCount: item.pageCount,
    readCount: item.readCount,
    updatedAt: item.updatedAt,
    isDownloaded: item.isDownloaded,
    localPath: item.localPath ?? '',
    fileSize: item.fileSize,
    hasError: item.hasError,
  } as OfflineComic
}

// 🎲 抽卡核心逻辑：对接后端 /comics/random
// view 化后直接继承当前模式（在线/离线）对应域的全局筛选配置，不再提供「继承全局筛选」开关
const handleStartDraw = async () => {
  isSpinning.value = true

  try {
    // 1. 依据当前作用域继承对应的全局筛选配置
    const config =
      modeStore.currentMode === 'online' ? onlineSearchConfig.value : offlineSearchConfig.value

    const params: RandomComicParams = {
      count: targetCount.value,
      source: scopeType.value,
    }

    const kw = (config.keyword || '').trim()
    if (kw) params.keyword = kw
    // 问题1：继承筛选抽屉的“多关键词队列”（在线由后端合并 f_search，离线按 AND 匹配）
    if (config.keywords && config.keywords.length > 0) {
      params.keywords = config.keywords.filter((k) => k.trim())
    }
    // 分类在线/离线均继承（问题6：离线抽卡不再跳过分类过滤）
    if (config.activeCategories.length > 0) {
      params.categories = [...config.activeCategories]
    }
    // 语言筛选：离线按 disableLangFilter 决定是否应用（D11：language 可选继承）
    if (
      scopeType.value === 'offline' &&
      config.language &&
      config.language !== 'All' &&
      !config.disableLangFilter
    ) {
      params.language = config.language
    }
    if (config.minRating > 0) params.minRating = config.minRating
    if (config.minPages && config.minPages > 0) params.minPages = config.minPages
    if (config.maxPages && config.maxPages > 0) params.maxPages = config.maxPages

    // 2. 调用后端随机接口（离线 SQL RANDOM + 在线随机页采样）
    const res = await fetchRandomComicsApi(params)

    // 3. 统一映射为卡片可渲染的 ComicItem
    drawnComics.value = res.comics.map(toComicItem)
    isSpinning.value = false
    hasDrawn.value = true

    if (res.warning) {
      toast.warning(res.warning)
    } else if (drawnComics.value.length === 0) {
      toast.error('当前范围内没有符合条件的作品！')
    } else if (drawnComics.value.length < targetCount.value) {
      toast.warning(`符合条件的作品仅有 ${drawnComics.value.length} 本，已为你全数抽出！`)
    } else {
      toast.success(`成功抽出 ${drawnComics.value.length} 本作品！`)
    }
  } catch (err) {
    isSpinning.value = false
    hasDrawn.value = false
    toast.error(err instanceof Error ? err.message : '抽卡失败，请稍后重试')
  }
}
</script>

<template>
  <div class="random-view">
    <!-- 1. 页面标题 Header -->
    <div class="page-header">
      <div class="header-title">
        <span>🎲 随机本子抽卡</span>
        <span class="subtitle">摆脱选择困难症</span>
      </div>
    </div>

    <!-- 2. 控制面板 -->
    <div class="control-panel">
      <div class="panel-top-row">
        <!-- 数量指定 -->
        <div class="control-group">
          <label class="group-label">数量：</label>
          <div class="count-selector">
            <button
              v-for="num in [1, 4, 8]"
              :key="num"
              class="pill-btn"
              :class="{ active: selectedCountOption === num }"
              @click="selectedCountOption = num as 1 | 4 | 8"
            >
              {{ num }} 本
            </button>

            <button
              class="pill-btn custom"
              :class="{ active: selectedCountOption === 'custom' }"
              @click="selectedCountOption = 'custom'"
            >
              自定义
            </button>

            <div v-if="selectedCountOption === 'custom'" class="custom-input-box">
              <input v-model.number="customCountInput" type="number" min="1" max="50" />
              <span class="unit">本</span>
            </div>
          </div>
        </div>

        <!-- 范围制定 -->
        <div class="control-group">
          <label class="group-label">范围：</label>
          <select v-model="scopeType" class="dark-select">
            <option value="all">🌐+📚 全库 (在线+本地)</option>
            <option value="online">🌐 仅在线图库</option>
            <option value="offline">📚 仅本地画库</option>
          </select>
        </div>
      </div>

      <!-- 开始抽卡大按钮 -->
      <button class="draw-btn" :disabled="isSpinning" @click="handleStartDraw">
        {{ isSpinning ? '🎰 正在洗牌抽卡中...' : hasDrawn ? '🔄 重新抽取' : '🎴 开始抽卡！' }}
      </button>
    </div>

    <!-- 3. 结果展示区 -->
    <div class="results-container">
      <div v-if="!hasDrawn && !isSpinning" class="empty-placeholder">
        <span class="placeholder-icon">🃏</span>
        <p>选择好数量与范围，点击“开始抽卡”即可随机挑选作品</p>
      </div>

      <div v-else-if="isSpinning" class="spinning-placeholder">
        <div class="loading-cards">
          <span class="card-flip">🎴</span>
          <span class="card-flip delay-1">🎴</span>
          <span class="card-flip delay-2">🎴</span>
        </div>
        <p>命运的轮盘转动中...</p>
      </div>

      <!-- 真实抽出的卡片网格 -->
      <div v-else class="results-grid">
        <div v-for="(comic, index) in drawnComics" :key="comic.id" class="drawn-item-card">
          <div class="card-badge">NO.{{ index + 1 }}</div>
          <ItemCard :comic="comic" mode="card" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.random-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 4px;
}

/* 1. 页面标题 */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-title {
  display: flex;
  align-items: baseline;
  gap: 10px;
  font-size: 1.15rem;
  font-weight: bold;
  color: var(--app-text-strong);
}

.subtitle {
  font-size: 0.8rem;
  color: var(--app-text-3);
  font-weight: normal;
}

/* 2. 控制面板 */
.control-panel {
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 12px;
  padding: 14px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
}

.panel-top-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.control-group {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.88rem;
}

.group-label {
  color: var(--app-text-2);
  flex-shrink: 0;
}

.count-selector {
  display: flex;
  align-items: center;
  gap: 6px;
}

.pill-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 3px 10px;
  border-radius: 12px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.2s;
}

.pill-btn.active {
  background: #007acc;
  border-color: #007acc;
  color: #fff;
  font-weight: bold;
}

.custom-input-box {
  display: flex;
  align-items: center;
  gap: 2px;
  background: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  padding: 1px 6px;
  border-radius: 4px;
}

.custom-input-box input {
  width: 40px;
  background: transparent;
  border: none;
  color: var(--app-text-strong);
  text-align: center;
  font-size: 0.82rem;
  outline: none;
}

.unit {
  font-size: 0.75rem;
  color: var(--app-text-3);
}

.dark-select {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
  border: 1px solid var(--app-border-3);
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 0.82rem;
  outline: none;
}

.draw-btn {
  width: 100%;
  background: linear-gradient(135deg, #007acc, #005999);
  color: #fff;
  border: none;
  padding: 8px;
  border-radius: 6px;
  font-size: 0.9rem;
  font-weight: bold;
  cursor: pointer;
  transition: opacity 0.2s;
}

.draw-btn:hover {
  opacity: 0.9;
}
.draw-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 3. 展示区域 */
.results-container {
  flex: 1;
  min-height: 50vh;
}

.empty-placeholder,
.spinning-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--app-text-muted);
  gap: 8px;
  padding: 40px 0;
}

.placeholder-icon {
  font-size: 2.5rem;
}

.loading-cards {
  display: flex;
  gap: 6px;
}
.card-flip {
  font-size: 2rem;
  animation: pulse 1s infinite alternate;
}
.delay-1 {
  animation-delay: 0.2s;
}
.delay-2 {
  animation-delay: 0.4s;
}

@keyframes pulse {
  from {
    transform: scale(0.8);
    opacity: 0.5;
  }
  to {
    transform: scale(1.1);
    opacity: 1;
  }
}

/* 抽卡卡片网格布局 */
.results-grid {
  width: 100%;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 16px;
  align-items: start;
}

/* 抽卡卡片外壳：必须实底 + 裁剪，防止穿透 */
.drawn-item-card {
  position: relative;
  background-color: var(--app-surface-2) !important;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid var(--app-border-3);
  cursor: pointer;
  transition:
    transform 0.2s,
    border-color 0.2s;
}

.drawn-item-card:hover {
  transform: translateY(-4px);
  border-color: #007acc;
}

.card-badge {
  position: absolute;
  top: 6px;
  left: 6px;
  background: #007acc;
  color: #fff;
  font-size: 0.7rem;
  font-weight: bold;
  padding: 2px 6px;
  border-radius: 4px;
  z-index: 10;
  pointer-events: none;
}

/* 封面保护：防止无图时高度塌陷导致卡片打架 */
:deep(.card-cover-wrapper) {
  position: relative;
  width: 100%;
  aspect-ratio: 3 / 4;
  background-color: var(--app-border-2);
  overflow: hidden;
}

:deep(.cover-img) {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

:deep(.item-card) {
  background-color: var(--app-surface-2) !important;
  height: 100%;
}

/* 📱 移动形态（<1024px）：控制面板垂直堆叠 + 结果网格固定 2 列 */
@media (max-width: 1024px) {
  .panel-top-row {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;
  }
  .control-group {
    justify-content: space-between;
  }
  .count-selector {
    flex-wrap: wrap;
  }
  .results-container {
    min-height: 40vh;
  }
  .results-grid {
    grid-template-columns: repeat(2, 1fr);
    gap: 10px;
  }
}
</style>
