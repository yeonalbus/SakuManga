<script setup lang="ts">
import { ref, reactive, computed } from 'vue'
import { useUI } from '@/composables/useUI'
import ItemCard from '@/components/ItemCard.vue'
import TagChip from '@/components/TagChip.vue'
import { useTagSuggest, type TagSuggestion } from '@/composables/useTagSuggest'
import { fetchRandomComicsApi } from '@/api/comic'
// Round3-任务6：负向排除（抽卡结果前端兜底过滤）
import { isNegativeItem, matchExcludes, parseKeywordQueue, formatFSearchTag } from '@/utils/tagFilter'
import type {
  ComicItem,
  OnlineComic,
  OfflineComic,
  RandomComicItem,
  RandomComicParams,
  SearchConfig,
} from '@/types/comic'

const { toast } = useUI()

// ======================================================
// 1. 数量控制逻辑
// ======================================================
const selectedCountOption = ref<1 | 4 | 8 | 'custom'>(1)
const customCountInput = ref<number>(12) // 自定义输入的数量

const targetCount = computed(() => {
  if (selectedCountOption.value === 'custom') {
    return Math.max(1, customCountInput.value || 1)
  }
  return selectedCountOption.value
})

// ======================================================
// 2. 抽卡专用过滤器（独立于全局 searchStore，专供抽卡使用）
// ======================================================
const categories = [
  { key: 'Doujinshi', label: 'Doujinshi', color: '#e53935' },
  { key: 'Manga', label: 'Manga', color: '#f57c00' },
  { key: 'Image Set', label: 'Image Set', color: '#3949ab' },
  { key: 'Game CG', label: 'Game CG', color: '#2e7d32' },
  { key: 'Artist CG', label: 'Artist CG', color: '#cddc39' },
  { key: 'Cosplay', label: 'Cosplay', color: '#8e24aa' },
  { key: 'Non-H', label: 'Non-H', color: '#424242' },
  { key: 'Asian Porn', label: 'Asian Porn', color: '#d81b60' },
  { key: 'Western', label: 'Western', color: '#00e676' },
  { key: 'Misc', label: 'Misc', color: '#757575' },
]

const allCategoryKeys = () => categories.map((c) => c.key)

/** 抽卡专用过滤配置：独立于全局筛选，仅作用于本次抽卡 */
const drawConfig = reactive<SearchConfig>({
  keyword: '', // 抽卡面板用多关键词队列（keywords），此处留空
  keywords: [],
  activeCategories: allCategoryKeys(),
  minRating: 0,
  minPages: undefined,
  maxPages: undefined,
  onlyDownloaded: false,
  language: 'All',
  onlyRemoved: false,
  onlyTorrents: false,
  disableLangFilter: false,
  disableUploaderFilter: false,
  disableTagFilter: false,
  // ─── Round3-任务6：负向排除（`- ` 前缀拆解后单独存放，便于后端/前端复用）───
  excludeTags: [],
  excludeKeywords: [],
})

// 过滤器面板展开状态与关键词输入
const filterOpen = ref(false)
const keywordInput = ref('')
const kwInputFocused = ref(false)

// ─── Round3-任务5：tag 联想（支持负向「- 」前缀解析，复用 /tags/suggest）───
// 插入格式按抽卡范围区分：仅离线（scopeType === 'offline'）用裸格式，含在线用 E-Hentai f_search 标准语法
const { suggestions, loading, refresh, clear: clearSuggest } = useTagSuggest(
  () => keywordInput.value,
  8,
  150,
  (namespace, key) => formatFSearchTag(namespace, key, scopeType.value === 'offline'),
)

// 选中联想项：负向项以「- namespace:key」压入队列
const pickSuggestion = (sug: TagSuggestion) => {
  if (!drawConfig.keywords.includes(sug.insertText)) {
    drawConfig.keywords.push(sug.insertText)
  }
  keywordInput.value = ''
  clearSuggest()
  kwInputFocused.value = false
}

// 切换分类选中状态
const toggleCategory = (key: string) => {
  const idx = drawConfig.activeCategories.indexOf(key)
  if (idx >= 0) {
    drawConfig.activeCategories.splice(idx, 1)
  } else {
    drawConfig.activeCategories.push(key)
  }
}

// 关键词队列：有字压入，空按 Enter 清空
const handleKeywordEnter = () => {
  const text = keywordInput.value.trim()
  if (text) {
    if (!drawConfig.keywords.includes(text)) {
      drawConfig.keywords.push(text)
    }
    keywordInput.value = ''
    clearSuggest()
  } else {
    drawConfig.keywords = []
    clearSuggest()
  }
}

const removeKeyword = (index: number) => {
  drawConfig.keywords.splice(index, 1)
}

// 重置抽卡过滤器
const resetFilter = () => {
  keywordInput.value = ''
  clearSuggest()
  kwInputFocused.value = false
  drawConfig.keywords = []
  drawConfig.excludeTags = []
  drawConfig.excludeKeywords = []
  drawConfig.activeCategories = allCategoryKeys()
  drawConfig.minRating = 0
  drawConfig.minPages = undefined
  drawConfig.maxPages = undefined
  drawConfig.onlyDownloaded = false
  drawConfig.language = 'All'
  drawConfig.onlyRemoved = false
  drawConfig.onlyTorrents = false
  drawConfig.disableLangFilter = false
  drawConfig.disableUploaderFilter = false
  drawConfig.disableTagFilter = false
}

// 已生效条件数量（用于徽标与状态提示）
const activeFilterCount = computed(() => {
  let n = 0
  if (drawConfig.keywords.length > 0) n++
  if (drawConfig.activeCategories.length < categories.length) n++
  if (drawConfig.minRating > 0) n++
  if (drawConfig.minPages && drawConfig.minPages > 0) n++
  if (drawConfig.maxPages && drawConfig.maxPages > 0) n++
  if (drawConfig.onlyDownloaded) n++
  if (drawConfig.language !== 'All') n++
  if (drawConfig.onlyRemoved) n++
  if (drawConfig.onlyTorrents) n++
  if (drawConfig.disableLangFilter) n++
  if (drawConfig.disableUploaderFilter) n++
  if (drawConfig.disableTagFilter) n++
  return n
})

// ======================================================
// 3. 范围制定控制（all/online/offline）
// ======================================================
const scopeType = ref<'all' | 'online' | 'offline'>('all')

// 在线高级筛选项仅在包含在线时展示
const showOnlineOptions = computed(() => scopeType.value !== 'offline')

// ======================================================
// 4. 抽卡状态与结果存储
// ======================================================
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

/**
 * 依据抽卡专用过滤器构建随机抽卡入参。
 * - 分类：在线/离线均生效
 * - 语言：在线并入 f_search；离线按 language:xx 标签匹配（禁用语言过滤时不生效）
 * - 在线高级筛选：仅在线/全库下发
 */
const buildParams = (): RandomComicParams => {
  const params: RandomComicParams = {
    count: targetCount.value,
    source: scopeType.value,
  }

  const kw = (drawConfig.keyword || '').trim()
  if (kw) params.keyword = kw
  // Round3-任务6：把队列按「- 」前缀拆分为正向 / 负向，正向下发搜索，负向单独下发
  const parsed = parseKeywordQueue(drawConfig.keywords)
  if (parsed.positive.length > 0) {
    params.keywords = parsed.positive
  }
  if (parsed.excludeTags.length > 0) params.excludeTags = parsed.excludeTags
  if (parsed.excludeKeywords.length > 0) params.excludeKeywords = parsed.excludeKeywords
  if (drawConfig.activeCategories.length > 0) {
    params.categories = [...drawConfig.activeCategories]
  }
  if (drawConfig.minRating > 0) params.minRating = drawConfig.minRating
  if (drawConfig.minPages && drawConfig.minPages > 0) params.minPages = drawConfig.minPages
  if (drawConfig.maxPages && drawConfig.maxPages > 0) params.maxPages = drawConfig.maxPages
  // 语言：离线且禁用语言过滤时不按语言匹配
  if (drawConfig.language && drawConfig.language !== 'All') {
    if (scopeType.value !== 'offline' || !drawConfig.disableLangFilter) {
      params.language = drawConfig.language
    }
  }
  if (drawConfig.onlyDownloaded) params.onlyDownloaded = true

  // 在线高级筛选（仅在线 / 全库时下发，后端离线分支忽略）
  if (drawConfig.onlyRemoved) params.onlyRemoved = true
  if (drawConfig.onlyTorrents) params.onlyTorrents = true
  if (drawConfig.disableLangFilter) params.disableLangFilter = true
  if (drawConfig.disableUploaderFilter) params.disableUploaderFilter = true
  if (drawConfig.disableTagFilter) params.disableTagFilter = true

  return params
}

// 🎲 抽卡核心逻辑：对接后端 /comics/random
// 全库模式由后端"先随机抽本地约一半，再在线补齐剩余"，比例接近 1:1
const handleStartDraw = async () => {
  isSpinning.value = true

  try {
    const res = await fetchRandomComicsApi(buildParams())

    // 防御：后端返回 comics:null 时按空数组处理，避免 null.map 抛错
    let items = (res.comics || []).map(toComicItem)

    // ─── Round3-任务6：前端负向兜底过滤（负向 tag 精确 / 负向关键词子串）───
    const parsed = parseKeywordQueue(drawConfig.keywords)
    const excludeRule = {
      excludeTags: [...(drawConfig.excludeTags || []), ...parsed.excludeTags],
      excludeKeywords: [...(drawConfig.excludeKeywords || []), ...parsed.excludeKeywords],
    }
    const filtered = items.filter((c) => matchExcludes(c, excludeRule))
    const dropped = items.length - filtered.length
    items = filtered

    drawnComics.value = items
    isSpinning.value = false
    hasDrawn.value = true

    if (res.warning) {
      toast.warning(res.warning)
    } else if (items.length === 0) {
      toast.error('当前范围内没有符合条件的作品！')
    } else if (dropped > 0 && items.length < targetCount.value) {
      toast.warning(`负向排除后仅剩 ${items.length} 本（已排除 ${dropped} 本）`)
    } else if (items.length < targetCount.value) {
      toast.warning(`符合条件的作品仅有 ${items.length} 本，已为你全数抽出！`)
    } else {
      toast.success(`成功抽出 ${items.length} 本作品！`)
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

    <!-- 2. 控制面板：数量 → 抽卡专用过滤器 → 范围 → 抽卡按钮 -->
    <div class="control-panel">
      <!-- ① 数量指定 -->
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

      <!-- ② 抽卡专用过滤器（独立于全局筛选，位于数量与范围之间） -->
      <div class="filter-block">
        <div class="filter-toggle-row">
          <button
            class="filter-toggle-btn"
            :class="{ open: filterOpen }"
            @click="filterOpen = !filterOpen"
          >
            <span class="ft-icon">🎛️</span>
            <span class="ft-label">抽卡过滤器</span>
            <span v-if="activeFilterCount > 0" class="filter-badge">{{ activeFilterCount }}</span>
            <span class="ft-arrow">{{ filterOpen ? '▲' : '▼' }}</span>
          </button>
          <span class="filter-status" :class="{ active: activeFilterCount > 0 }">
            {{
              activeFilterCount > 0
                ? `已应用 ${activeFilterCount} 项条件（不影响全局筛选）`
                : '未设置（全库随机，不影响全局筛选）'
            }}
          </span>
        </div>

        <div v-if="filterOpen" class="filter-panel">
          <!-- 关键词队列 -->
          <div class="filter-section">
            <label class="filter-label">
              关键词队列
              <span class="tip-text">(Enter 压入，框为空按 Enter 清空)</span>
            </label>
            <div v-if="drawConfig.keywords.length > 0" class="kw-chips">
              <span
                v-for="(kw, index) in drawConfig.keywords"
                :key="kw + index"
                class="kw-chip"
                :class="{ 'is-negative': isNegativeItem(kw) }"
              >
                <span class="kw-text">{{ kw }}</span>
                <span class="kw-remove" title="删除此关键词" @click.stop="removeKeyword(index)">
                  ✕
                </span>
              </span>
            </div>
            <input
              v-model="keywordInput"
              type="text"
              class="filter-input"
              placeholder="输入关键词后按 Enter 压入队列，前缀「- 」表示排除..."
              @focus="kwInputFocused = true"
              @blur="kwInputFocused = false"
              @keydown.enter.prevent="handleKeywordEnter"
            />

            <!-- ─── Round3-任务5：tag 联想（正向 / 负向「- 」前缀）─── -->
            <div
              v-if="kwInputFocused && (suggestions.length > 0 || loading)"
              class="tag-suggest-box"
            >
              <div v-if="loading" class="suggest-loading">加载中...</div>
              <div
                v-for="sug in suggestions"
                :key="`${sug.namespace}:${sug.key}`"
                class="tag-suggest-item"
                :class="{ negative: sug.isNegative }"
                @mousedown.prevent
                @click="pickSuggestion(sug)"
              >
                <TagChip :tag="sug" />
                <span v-if="sug.count" class="tag-count-badge"
                  >🔥 {{ sug.count.toLocaleString() }}</span
                >
                <span class="suggest-hint">{{ sug.isNegative ? '排除' : '加入' }}</span>
              </div>
            </div>
          </div>

          <!-- 分类 -->
          <div class="filter-section">
            <label class="filter-label">分类</label>
            <div class="cat-grid">
              <button
                v-for="cat in categories"
                :key="cat.key"
                class="cat-chip"
                :class="{ disabled: !drawConfig.activeCategories.includes(cat.key) }"
                :style="{
                  backgroundColor: drawConfig.activeCategories.includes(cat.key)
                    ? cat.color
                    : 'var(--app-border-2)',
                  color: drawConfig.activeCategories.includes(cat.key)
                    ? '#ffffff'
                    : 'var(--app-text-3)',
                }"
                @click="toggleCategory(cat.key)"
              >
                {{ cat.label }}
              </button>
            </div>
          </div>

          <!-- 语言 / 评分 / 页数 -->
          <div class="filter-grid">
            <div class="filter-field">
              <label class="field-label">语言</label>
              <select v-model="drawConfig.language" class="dark-select">
                <option value="All">All (全部)</option>
                <option value="Chinese">Chinese (中文)</option>
                <option value="Japanese">Japanese (日文)</option>
                <option value="English">English (英文)</option>
              </select>
            </div>

            <div class="filter-field">
              <label class="field-label">最低评分</label>
              <select v-model="drawConfig.minRating" class="dark-select">
                <option :value="0">0 ⭐ (全部)</option>
                <option :value="1">1 ⭐</option>
                <option :value="2">2 ⭐</option>
                <option :value="3">3 ⭐</option>
                <option :value="4">4 ⭐</option>
                <option :value="5">5 ⭐</option>
              </select>
            </div>

            <div class="filter-field">
              <label class="field-label">页数范围</label>
              <div class="page-range">
                <input
                  v-model.number="drawConfig.minPages"
                  type="number"
                  class="number-input"
                  placeholder="Min"
                />
                <span class="range-text">~</span>
                <input
                  v-model.number="drawConfig.maxPages"
                  type="number"
                  class="number-input"
                  placeholder="Max"
                />
              </div>
            </div>
          </div>

          <!-- 仅已下载（本地） -->
          <div class="filter-row">
            <span class="row-label">仅已下载（本地）</span>
            <label class="toggle-switch">
              <input v-model="drawConfig.onlyDownloaded" type="checkbox" />
              <span class="slider"></span>
            </label>
          </div>

          <!-- 在线高级筛选（仅在线 / 全库展示） -->
          <template v-if="showOnlineOptions">
            <div class="filter-divider">在线高级筛选</div>
            <div class="filter-row">
              <span class="row-label">仅搜索移除了的画廊</span>
              <label class="toggle-switch">
                <input v-model="drawConfig.onlyRemoved" type="checkbox" />
                <span class="slider"></span>
              </label>
            </div>
            <div class="filter-row">
              <span class="row-label">只显示有种子的画廊</span>
              <label class="toggle-switch">
                <input v-model="drawConfig.onlyTorrents" type="checkbox" />
                <span class="slider"></span>
              </label>
            </div>
            <div class="filter-row">
              <span class="row-label">禁用语言过滤</span>
              <label class="toggle-switch">
                <input v-model="drawConfig.disableLangFilter" type="checkbox" />
                <span class="slider"></span>
              </label>
            </div>
            <div class="filter-row">
              <span class="row-label">禁用上传者过滤</span>
              <label class="toggle-switch">
                <input v-model="drawConfig.disableUploaderFilter" type="checkbox" />
                <span class="slider"></span>
              </label>
            </div>
            <div class="filter-row">
              <span class="row-label">禁用标签过滤</span>
              <label class="toggle-switch">
                <input v-model="drawConfig.disableTagFilter" type="checkbox" />
                <span class="slider"></span>
              </label>
            </div>
          </template>

          <div class="filter-actions">
            <button class="filter-reset" @click="resetFilter">🔄 重置过滤器</button>
          </div>
        </div>
      </div>

      <!-- ③ 范围制定 -->
      <div class="control-group">
        <label class="group-label">范围：</label>
        <select v-model="scopeType" class="dark-select">
          <option value="all">🌐+📚 全库 (在线+本地)</option>
          <option value="online">🌐 仅在线图库</option>
          <option value="offline">📚 仅本地画库</option>
        </select>
      </div>

      <!-- ④ 开始抽卡大按钮 -->
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
  gap: 14px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
}

.control-group {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.88rem;
  flex-wrap: wrap;
}

.group-label {
  color: var(--app-text-2);
  flex-shrink: 0;
}

.count-selector {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
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

/* ───────── 抽卡专用过滤器 ───────── */
.filter-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
  border-top: 1px dashed var(--app-border-3);
  padding-top: 12px;
}

.filter-toggle-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.filter-toggle-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 5px 12px;
  border-radius: 8px;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
}

.filter-toggle-btn:hover {
  border-color: #007acc;
  color: var(--app-text-strong);
}

.filter-toggle-btn.open {
  border-color: #007acc;
  color: var(--app-text-strong);
  background: rgba(0, 122, 204, 0.12);
}

.ft-icon {
  font-size: 0.9rem;
}

.ft-label {
  font-weight: 600;
}

.filter-badge {
  background: #007acc;
  color: #fff;
  font-size: 0.72rem;
  font-weight: bold;
  min-width: 18px;
  height: 18px;
  line-height: 18px;
  text-align: center;
  border-radius: 9px;
  padding: 0 5px;
}

.ft-arrow {
  font-size: 0.7rem;
  color: var(--app-text-3);
}

.filter-status {
  font-size: 0.75rem;
  color: var(--app-text-muted);
}

.filter-status.active {
  color: #10b981;
}

/* 过滤面板 */
.filter-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
  background: var(--app-surface-1);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  padding: 12px 14px;
}

.filter-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.filter-label {
  font-size: 0.82rem;
  color: var(--app-text-3);
}

.tip-text {
  font-size: 0.7rem;
  color: var(--app-text-muted);
  font-weight: normal;
  margin-left: 4px;
}

.kw-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 6px;
  background-color: var(--app-surface-2);
  border-radius: 6px;
  border: 1px dashed var(--app-border-3);
}

.kw-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: #ff7588;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.8rem;
  font-weight: 500;
}

.kw-chip.is-negative {
  background-color: rgba(255, 77, 109, 0.12);
  border-color: #ff4d6d;
  color: #ff4d6d;
  text-decoration: line-through;
}

.kw-remove {
  color: var(--app-text-3);
  font-size: 0.75rem;
  cursor: pointer;
  transition: color 0.15s;
}

.kw-remove:hover {
  color: var(--app-text-strong);
}

.filter-input {
  background-color: transparent;
  border: none;
  border-bottom: 1px solid var(--app-border-3);
  color: var(--app-text-strong);
  padding: 6px 0;
  font-size: 0.88rem;
  outline: none;
  transition: border-color 0.2s;
}

.filter-input:focus {
  border-bottom-color: #007acc;
}

/* ─── Round3-任务5：tag 联想下拉 ─── */
.tag-suggest-box {
  margin-top: 4px;
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  background-color: var(--app-surface-2);
  max-height: 240px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  padding: 4px;
}

.suggest-loading {
  padding: 8px 10px;
  font-size: 0.8rem;
  color: var(--app-text-3);
}

.tag-suggest-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 4px;
  cursor: pointer;
  transition: background-color 0.15s;
}

.tag-suggest-item:hover {
  background-color: var(--app-surface-3);
}

.tag-suggest-item.negative .suggest-hint {
  color: #ff4d6d;
}

.tag-count-badge {
  margin-left: auto;
  font-size: 0.72rem;
  color: var(--app-text-3);
  white-space: nowrap;
}

.suggest-hint {
  font-size: 0.72rem;
  color: #10b981;
  white-space: nowrap;
  border: 1px solid currentColor;
  border-radius: 3px;
  padding: 0 4px;
}

/* 分类网格 */
.cat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
  gap: 6px;
}

.cat-chip {
  padding: 7px 0;
  border-radius: 6px;
  border: none;
  font-size: 0.8rem;
  font-weight: bold;
  cursor: pointer;
  transition: all 0.2s ease;
  text-align: center;
}

.cat-chip.disabled {
  text-decoration: line-through;
  opacity: 0.5;
}

/* 语言 / 评分 / 页数 网格 */
.filter-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 10px;
}

.filter-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.field-label {
  font-size: 0.78rem;
  color: var(--app-text-3);
}

.page-range {
  display: flex;
  align-items: center;
  gap: 6px;
}

.number-input {
  width: 52px;
  height: 30px;
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 4px;
  color: var(--app-text-strong);
  text-align: center;
  font-size: 0.82rem;
  outline: none;
}

.range-text {
  font-size: 0.8rem;
  color: var(--app-text-3);
}

/* 开关行 */
.filter-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.86rem;
  color: var(--app-text-2);
}

.row-label {
  color: var(--app-text-2);
}

.filter-divider {
  font-size: 0.78rem;
  color: var(--app-text-3);
  border-bottom: 1px solid var(--app-border-3);
  padding-bottom: 4px;
}

/* 开关 Toggle Switch */
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 40px;
  height: 22px;
  flex-shrink: 0;
}

.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  inset: 0;
  background-color: var(--app-border-3);
  border-radius: 22px;
  transition: 0.3s;
}

.slider:before {
  position: absolute;
  content: '';
  height: 16px;
  width: 16px;
  left: 3px;
  bottom: 3px;
  background-color: var(--app-text-3);
  border-radius: 50%;
  transition: 0.3s;
}

input:checked + .slider {
  background-color: rgba(124, 77, 255, 0.3);
}

input:checked + .slider:before {
  transform: translateX(18px);
  background-color: #7c4dff;
}

/* 过滤器底部操作 */
.filter-actions {
  display: flex;
  justify-content: flex-end;
  border-top: 1px solid var(--app-border-3);
  padding-top: 10px;
}

.filter-reset {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 4px 14px;
  border-radius: 6px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.2s;
}

.filter-reset:hover {
  border-color: #e53935;
  color: #e53935;
}

/* 抽卡按钮 */
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
  .control-group {
    justify-content: space-between;
  }
  .count-selector {
    flex-wrap: wrap;
  }
  .filter-toggle-row {
    flex-direction: column;
    align-items: flex-start;
  }
  .cat-grid {
    grid-template-columns: repeat(auto-fill, minmax(90px, 1fr));
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
