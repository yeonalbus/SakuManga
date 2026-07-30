<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUI } from '@/composables/useUI'
import ItemCard from '@/components/ItemCard.vue'
import {
  onlineComics,
  offlineComics,
  onlineSearchConfig,
  offlineSearchConfig,
} from '@/stores/appStore'
import type { ComicItem } from '@/types/comic'

const router = useRouter()
const route = useRoute()
const { toast } = useUI()

// 1. 弹窗显隐控制
const isModalOpen = ref(false)

watch(
  () => route.fullPath,
  () => {
    isModalOpen.value = false
  },
)

// 2. 数量控制逻辑
const selectedCountOption = ref<1 | 4 | 8 | 'custom'>(1)
const customCountInput = ref<number>(12) // 自定义输入的数量

const targetCount = computed(() => {
  if (selectedCountOption.value === 'custom') {
    return Math.max(1, customCountInput.value || 1)
  }
  return selectedCountOption.value
})

// 3. 范围制定控制
const scopeType = ref<'all' | 'online' | 'offline'>('all')
const useGlobalFilter = ref(true)

// 4. 抽卡状态与结果存储
const isSpinning = ref(false)
const hasDrawn = ref(false)
const drawnComics = ref<ComicItem[]>([])

// 🎲 抽卡核心逻辑
// 🎲 抽卡核心逻辑
const handleStartDraw = () => {
  isSpinning.value = true

  let rawOnline = onlineComics.value
  let rawOffline = offlineComics.value

  // 1. 如果勾选了“继承全局筛选”，分别用在线/离线的搜索关键字过滤
  if (useGlobalFilter.value) {
    const onKw = (onlineSearchConfig.value.keyword || '').toLowerCase().trim()
    if (onKw) {
      rawOnline = rawOnline.filter(
        (c) =>
          c.title.toLowerCase().includes(onKw) ||
          c.tags?.some((t) => t.toLowerCase().includes(onKw)),
      )
    }

    const offKw = (offlineSearchConfig.value.keyword || '').toLowerCase().trim()
    if (offKw) {
      rawOffline = rawOffline.filter((c) => {
        const matchTitle = c.title.toLowerCase().includes(offKw)
        const tagsArr = Array.isArray(c.tags) ? c.tags : []
        const matchTag = tagsArr.some((t: string) => t.toLowerCase().includes(offKw))
        return matchTitle || matchTag
      })
    }
  }

  // 2. 根据选定的范围构建池子
  let pool: ComicItem[] = []
  if (scopeType.value === 'online') {
    pool = rawOnline
  } else if (scopeType.value === 'offline') {
    pool = rawOffline
  } else {
    pool = [...rawOnline, ...rawOffline]
  }

  const requestedCount = targetCount.value
  const poolSize = pool.length

  let finalDrawCount = requestedCount
  let overflowWarning = false

  if (poolSize === 0) {
    toast.error('当前范围内没有符合条件的作品！')
    isSpinning.value = false
    return
  }

  if (requestedCount > poolSize) {
    finalDrawCount = poolSize
    overflowWarning = true
  }

  // 3. 洗牌并取出结果
  setTimeout(() => {
    const shuffled = [...pool].sort(() => Math.random() - 0.5)
    drawnComics.value = shuffled.slice(0, finalDrawCount)

    isSpinning.value = false
    hasDrawn.value = true

    if (overflowWarning) {
      toast.warning(`符合条件的作品仅有 ${poolSize} 本，已为你全数抽出！`)
    } else {
      toast.success(`成功抽出 ${finalDrawCount} 本作品！`)
    }
  }, 400)
}

// 点击跳转详情页
const handleComicClick = (comic: ComicItem) => {
  if (comic.source === 'online') {
    router.push(`/online/detail?id=${comic.id}`)
  } else {
    router.push(`/offline/detail?id=${comic.id}`)
  }
}
</script>

<template>
  <div class="random-picker-container">
    <!-- 顶部 trigger 按键 -->
    <button class="picker-trigger-btn" @click="isModalOpen = true">
      <span class="dice-icon">🎲</span>
      <span>手气不错</span>
    </button>

    <!-- 全屏 Modal 弹窗 -->
    <Teleport to="body">
      <div v-if="isModalOpen" class="modal-backdrop" @click="isModalOpen = false"></div>

      <div v-if="isModalOpen" class="random-modal">
        <!-- 1. 顶栏 Header (固定) -->
        <div class="modal-header">
          <div class="header-title">
            <span>🎲 随机本子抽卡</span>
            <span class="subtitle">摆脱选择困难症</span>
          </div>
          <button class="close-btn" @click="isModalOpen = false">✕</button>
        </div>

        <!-- 2. 上边栏控制面板 (固定在顶部，不随卡片滚动) -->
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

              <label class="checkbox-label">
                <input v-model="useGlobalFilter" type="checkbox" />
                <span>继承全局筛选</span>
              </label>
            </div>
          </div>

          <!-- 开始抽卡大按钮 -->
          <button class="draw-btn" :disabled="isSpinning" @click="handleStartDraw">
            {{ isSpinning ? '🎰 正在洗牌抽卡中...' : hasDrawn ? '🔄 重新抽取' : '🎴 开始抽卡！' }}
          </button>
        </div>

        <!-- 3. 下方独占滚动的卡片展示区 (只有这里产生滚动条) -->
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
            <div
              v-for="(comic, index) in drawnComics"
              :key="comic.id"
              class="drawn-item-card"
              @click="handleComicClick(comic)"
            >
              <div class="card-badge">NO.{{ index + 1 }}</div>
              <ItemCard :comic="comic" mode="card" />
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.random-picker-container {
  display: inline-block;
}

.picker-trigger-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  background-color: #242428;
  border: 1px solid #3a3a3d;
  color: #fff;
  padding: 6px 14px;
  border-radius: 16px;
  font-size: 0.85rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.picker-trigger-btn:hover {
  background-color: #2e2e33;
  border-color: #007acc;
  color: #007acc;
}

/* Modal 背景 */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.75);
  backdrop-filter: blur(3px);
  z-index: 2000;
}

/* Modal 窗口：硬性限定高度，Flex 垂直布局 */
.random-modal {
  position: fixed;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 90vw;
  max-width: 980px;
  height: 85vh; /* 强制模态框高度为 85vh */
  background-color: #161619;
  border: 1px solid #2d2d32;
  border-radius: 12px;
  z-index: 2001;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.8);
  display: flex;
  flex-direction: column; /* 垂直对齐：Header -> Control -> Results */
  overflow: hidden; /* 整体绝对不滚动 */
  color: #e0e0e0;
}

/* 1. Header (固定不滚动) */
.modal-header {
  flex-shrink: 0;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 20px;
  background: #1e1e22;
  border-bottom: 1px solid #2a2a2d;
}

.header-title {
  display: flex;
  align-items: baseline;
  gap: 10px;
  font-size: 1.1rem;
  font-weight: bold;
  color: #fff;
}

.subtitle {
  font-size: 0.8rem;
  color: #888;
  font-weight: normal;
}

.close-btn {
  background: transparent;
  border: none;
  color: #888;
  font-size: 1.1rem;
  cursor: pointer;
}
.close-btn:hover {
  color: #fff;
}

/* 2. 上边栏控制面板 (固定在顶部) */
.control-panel {
  flex-shrink: 0; /* 绝对不缩放、不随下方列表滚动 */
  background: #1d1d21;
  border-bottom: 1px solid #2d2d32;
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
  color: #aaa;
  flex-shrink: 0;
}

.count-selector {
  display: flex;
  align-items: center;
  gap: 6px;
}

.pill-btn {
  background: #28282c;
  border: 1px solid #38383c;
  color: #ccc;
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
  background: #000;
  border: 1px solid #3a3a3d;
  padding: 1px 6px;
  border-radius: 4px;
}

.custom-input-box input {
  width: 40px;
  background: transparent;
  border: none;
  color: #fff;
  text-align: center;
  font-size: 0.82rem;
  outline: none;
}

.unit {
  font-size: 0.75rem;
  color: #888;
}

.dark-select {
  background-color: #28282c;
  color: #fff;
  border: 1px solid #38383c;
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 0.82rem;
  outline: none;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 0.8rem;
  color: #bbb;
  cursor: pointer;
  user-select: none;
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

/* 3. 展示区域：吸收剩余高度，独自滚动 */
.results-container {
  flex: 1; /* 占据模态框全部剩余高度 */
  overflow-y: auto; /* 独占垂直滚动条 */
  padding: 20px;
}

.empty-placeholder,
.spinning-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #666;
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
  background-color: #1a1a1d !important;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid #2d2d32;
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
  background-color: #26262a;
  overflow: hidden;
}

:deep(.cover-img) {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

:deep(.item-card) {
  background-color: #1a1a1d !important;
  height: 100%;
}
</style>
