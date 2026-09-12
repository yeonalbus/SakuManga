<script setup lang="ts">
import { computed, onMounted, onActivated, nextTick, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRouter } from 'vue-router'
import ItemCard from '@/components/ItemCard.vue'
// 🟢 1. 从 comicStore 引入真实的全局离线漫画数据
import { offlineComics } from '@/stores/comicStore'
import type { OfflineComic, XpCloudArtist, XpCloudGroup, XpCloudResult, XpCloudTag, XpCloudView } from '@/types/comic'
import { detectDeviceClass } from '@/utils/device'
// Round7-任务8：列表状态记忆 + 提供者（新标签返回本页恢复滚动位置）
import {
  rememberListState,
  takeListState,
  setListStateProvider,
  getMainContent,
} from '@/utils/scrollMemory'
// Round32 阶段一：XP 词云面板
import XpWordCloud, { type XpCloudStats } from '@/components/XpWordCloud.vue'
import ArtistTopList from '@/components/ArtistTopList.vue'
import { fetchXpCloudApi, rebuildXpCloudApi } from '@/api/xpCloud'
import { formatFSearchTag } from '@/utils/tagFilter'
// Round32 视觉改版：词云展示参数（持久化，用户可自行调节）
import {
  xpCloudSettings,
  resetXpCloudSettings,
  xpCloudDirtyCount,
  XP_CLOUD_DEFAULTS,
} from '@/stores/xpCloudSettings'
import { offlineSearchConfig } from '@/stores/searchStore'
import { useModeStore } from '@/stores/modeStore'
import { useUserStore } from '@/stores/userStore'
import { useUI } from '@/composables/useUI'

interface RankedOfflineComic extends OfflineComic {
  rank: number
}

// 🟢 2. 核心计算属性：根据阅读/点击次数（readCount）降序排列并生成 TOP 25
const rankedComics = computed<RankedOfflineComic[]>(() => {
  const sorted = [...offlineComics.value].sort((a, b) => {
    const countA = a.readCount || 0
    const countB = b.readCount || 0
    return countB - countA
  })

  // 截取前 25 名，并自动打上 1 ~ 25 名的 rank 标签
  return sorted.slice(0, 25).map((comic, index) => ({
    ...comic,
    rank: index + 1,
  }))
})

// 🟢 3. 动态派生前三名与剩余榜单
const topThree = computed(() => rankedComics.value.slice(0, 3))
// 手机端阉割领奖台：隐藏 podium，榜单展示完整 TOP 25；iPad/桌面保留领奖台（第 4-25 名）
const isMobileDevice = detectDeviceClass() === 'mobile'
const restItemsForView = computed(() =>
  isMobileDevice ? rankedComics.value.slice(0, 25) : rankedComics.value.slice(3, 25),
)

// 辅助展示函数：获取漫画的实际阅读/点击次数
const getComicReadCount = (item: OfflineComic) => {
  return item.readCount || 0
}

// Round7-任务8：恢复/记忆列表滚动位置 + 注册列表状态提供者（无分页，page 恒为 1）
const restoreListState = async () => {
  const saved = takeListState('/offline/toplist')
  if (saved && saved.top > 0) {
    await nextTick()
    requestAnimationFrame(() => {
      const el = getMainContent()
      if (el && el.scrollHeight > 0) el.scrollTop = saved.top
    })
  }
  setListStateProvider('/offline/toplist', () => ({
    top: getMainContent()?.scrollTop || 0,
    page: 1,
  }))
}

onMounted(restoreListState)

// keep-alive 缓存下「同标签返回」只触发 onActivated，同样恢复列表状态
let activatedOnce = false
onActivated(() => {
  if (activatedOnce) restoreListState()
  activatedOnce = true
})

onBeforeRouteLeave(() => {
  rememberListState('/offline/toplist', {
    top: getMainContent()?.scrollTop || 0,
    page: 1,
  })
})

// ─────────────────────────────────────────────────────────────
// Round32 阶段一：XP 词云面板
//
// 数据源：后端统计表（/offline/xp-cloud），阅读侧按当前用户隔离。
// 视图（库藏/阅读）× 分组（核心XP/角色原作/其他）四组合结果按需拉取并缓存。
// ─────────────────────────────────────────────────────────────

const router = useRouter()
const modeStore = useModeStore()
const { isAdmin } = useUserStore()
const { toast } = useUI()

type PanelTab = 'rank' | 'cloud'

const activeTab = ref<PanelTab>('rank')
const cloudView = ref<XpCloudView>('library')
const cloudGroup = ref<Exclude<XpCloudGroup, 'all'>>('core')
const cloudLoading = ref(false)
const cloudError = ref('')
const cloudData = ref<XpCloudResult | null>(null)
const rebuilding = ref(false)

/** 组合结果缓存（key = view:group） */
const cloudCache = new Map<string, XpCloudResult>()

const loadCloud = async (force = false) => {
  const key = `${cloudView.value}:${cloudGroup.value}`
  if (!force && cloudCache.has(key)) {
    cloudData.value = cloudCache.get(key) as XpCloudResult
    return
  }
  cloudLoading.value = true
  cloudError.value = ''
  try {
    const result = await fetchXpCloudApi({
      group: cloudGroup.value,
      view: cloudView.value,
      limit: 140,
    })
    cloudCache.set(key, result)
    cloudData.value = result
  } catch (err) {
    cloudError.value = err instanceof Error ? err.message : '词云统计加载失败'
    cloudData.value = null
  } finally {
    cloudLoading.value = false
  }
}

// 首次进入词云页时才拉取（默认标签页不产生额外请求）
watch(activeTab, (tab) => {
  if (tab === 'cloud' && !cloudData.value) void loadCloud()
})
watch([cloudView, cloudGroup], () => {
  if (activeTab.value === 'cloud') void loadCloud()
})

/** 覆盖率提示：阅读信号过于稀疏时提醒以库藏视图为主 */
const coverageHint = computed(() => {
  const meta = cloudData.value?.meta
  if (!meta || meta.totalComics === 0) return ''
  const ratio = meta.readSignalComics / meta.totalComics
  if (cloudView.value !== 'reading') return ''
  if (ratio >= 0.05) return ''
  return `阅读信号仅覆盖 ${meta.readSignalComics} / ${meta.totalComics} 本（多读几本后会更准确），当前建议以「库藏」视图为主`
})

/** 词条/画师点击：按当前模式走既有快捷搜索语义（离线带关键词跳首页，在线跳搜索页） */
const quickSearch = (namespace: string, key: string) => {
  const queryTag = formatFSearchTag(namespace, key, false)
  if (modeStore.isOffline) {
    offlineSearchConfig.value.keyword = queryTag
    if (router.currentRoute.value.path !== '/offline/home') router.push('/offline/home')
  } else {
    router.push({ path: '/online/home', query: { kw: queryTag } })
  }
}

const handleTagSelect = (tag: XpCloudTag) => quickSearch(tag.namespace, tag.key)
const handleArtistSelect = (artist: XpCloudArtist) => quickSearch(artist.namespace, artist.key)

/** 管理员：全量重算统计表（公式调整/异常兜底） */
const handleRebuild = async () => {
  if (rebuilding.value) return
  rebuilding.value = true
  try {
    await rebuildXpCloudApi()
    cloudCache.clear()
    await loadCloud(true)
    toast.success('XP 统计已重算')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '重算失败')
  } finally {
    rebuilding.value = false
  }
}

const lastRebuildText = computed(() => {
  const ts = cloudData.value?.meta.lastRebuildAt || 0
  if (!ts) return '尚未重建'
  const d = new Date(ts)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
})

// ─────────────────────────────────────────────────────────────
// 词云展示参数（Round32 视觉改版）：词数/字号/间距/截断/译名过滤
// 参数持久化在 stores/xpCloudSettings.ts，改动即时重排，无需重新请求
// ─────────────────────────────────────────────────────────────
const showCloudParams = ref(false)
const cloudStats = ref<XpCloudStats | null>(null)
/** 词云画布高度：桌面 520 / 移动 360 */
const cloudHeight = computed(() => (isMobileDevice ? 360 : 520))
const cloudParamDirty = computed(() => xpCloudDirtyCount())

const handleCloudStats = (stats: XpCloudStats) => {
  cloudStats.value = stats
}

const resetCloudParams = () => {
  resetXpCloudSettings()
  toast.success('词云参数已恢复默认')
}
</script>

<template>
  <div class="leaderboard-page">
    <!-- Round32：榜单 / 词云 双视图切换 -->
    <div class="panel-switch">
      <button
        class="switch-btn"
        :class="{ active: activeTab === 'rank' }"
        @click="activeTab = 'rank'"
      >
        📊 阅读榜
      </button>
      <button
        class="switch-btn"
        :class="{ active: activeTab === 'cloud' }"
        @click="activeTab = 'cloud'"
      >
        ☁️ XP 词云
      </button>
    </div>

    <template v-if="activeTab === 'rank'">
      <h2 class="page-title">📊 本地个人阅读频次榜 (TOP 25)</h2>

      <div v-if="!isMobileDevice" class="podium-section">
        <div v-if="topThree[1]" class="podium-item rank-2-wrapper">
          <div class="podium-crown">🥈 NO.2</div>
          <ItemCard
            :comic="topThree[1]"
            :rank="2"
            size="large"
            mode="card"
            :hide-subtitle="true"
            :hide-tags="true"
            :hide-bottom-meta="true"
            :title-lines="2"
          />
          <div class="read-count">{{ getComicReadCount(topThree[1]) }} 次阅读</div>
        </div>

        <div v-if="topThree[0]" class="podium-item rank-1-wrapper">
          <div class="podium-crown crown-gold">👑 NO.1</div>
          <ItemCard
            :comic="topThree[0]"
            :rank="1"
            size="large"
            mode="card"
            :hide-subtitle="true"
            :hide-tags="true"
            :hide-bottom-meta="true"
            :title-lines="2"
          />
          <div class="read-count gold-text">{{ getComicReadCount(topThree[0]) }} 次阅读</div>
        </div>

        <div v-if="topThree[2]" class="podium-item rank-3-wrapper">
          <div class="podium-crown">🥉 NO.3</div>
          <ItemCard
            :comic="topThree[2]"
            :rank="3"
            size="large"
            mode="card"
            :hide-subtitle="true"
            :hide-tags="true"
            :hide-bottom-meta="true"
            :title-lines="2"
          />
          <div class="read-count">{{ getComicReadCount(topThree[2]) }} 次阅读</div>
        </div>
      </div>

      <div v-if="restItemsForView.length > 0" class="rest-section">
        <h3 class="section-subtitle">{{ isMobileDevice ? '🏆 TOP 25' : '第 4 - 25 名' }}</h3>
        <div class="card-grid">
          <div v-for="item in restItemsForView" :key="item.id" class="grid-item-wrapper">
            <ItemCard
              :comic="item"
              :rank="item.rank"
              mode="card"
              :hide-subtitle="true"
              :hide-tags="true"
              :hide-bottom-meta="true"
              :title-lines="2"
            />
            <div class="sub-read-count">{{ getComicReadCount(item) }} 次阅读</div>
          </div>
        </div>
      </div>
    </template>

    <!-- ─── XP 词云面板 ─── -->
    <template v-else>
      <h2 class="page-title">☁️ XP 词云</h2>

      <div class="cloud-toolbar">
        <div class="segmented">
          <button
            class="seg-btn"
            :class="{ active: cloudView === 'library' }"
            @click="cloudView = 'library'"
          >
            库藏
          </button>
          <button
            class="seg-btn"
            :class="{ active: cloudView === 'reading' }"
            @click="cloudView = 'reading'"
          >
            阅读
          </button>
        </div>
        <div class="segmented">
          <button
            class="seg-btn"
            :class="{ active: cloudGroup === 'core' }"
            @click="cloudGroup = 'core'"
          >
            核心 XP
          </button>
          <button class="seg-btn" :class="{ active: cloudGroup === 'ip' }" @click="cloudGroup = 'ip'">
            角色 / 原作
          </button>
          <button
            class="seg-btn"
            :class="{ active: cloudGroup === 'misc' }"
            @click="cloudGroup = 'misc'"
          >
            其他
          </button>
        </div>
        <!-- Round32 视觉改版：词云展示参数（词数/字号/间距/截断/译名过滤） -->
        <button
          class="cloud-param-btn"
          :class="{ open: showCloudParams }"
          @click="showCloudParams = !showCloudParams"
        >
          ⚙️ 词云参数
          <span v-if="cloudParamDirty > 0" class="reco-badge">{{ cloudParamDirty }}</span>
          <span class="ft-arrow">{{ showCloudParams ? '▲' : '▼' }}</span>
        </button>
      </div>

      <!-- 词云参数面板：改动即时重排（无需重新请求数据） -->
      <div v-if="showCloudParams" class="cloud-params">
        <div class="param-row">
          <label class="param-label">
            词数<b>{{ xpCloudSettings.wordCount }}</b>
          </label>
          <input
            v-model.number="xpCloudSettings.wordCount"
            class="param-slider"
            type="range"
            min="30"
            max="200"
            step="10"
          />
          <span class="param-hint">放不下时自动少放（以留白换观感）</span>
        </div>

        <div class="param-row">
          <label class="param-label">
            字号<b>{{ xpCloudSettings.fontMin }}~{{ xpCloudSettings.fontMax }}</b>
          </label>
          <div class="param-slider-pair">
            <input
              v-model.number="xpCloudSettings.fontMin"
              class="param-slider"
              type="range"
              min="10"
              max="20"
              step="1"
            />
            <input
              v-model.number="xpCloudSettings.fontMax"
              class="param-slider"
              type="range"
              min="28"
              max="64"
              step="2"
            />
          </div>
          <span class="param-hint">
            大词与尾词的字号差距（默认 {{ XP_CLOUD_DEFAULTS.fontMin }}~{{ XP_CLOUD_DEFAULTS.fontMax }}）
          </span>
        </div>

        <div class="param-row">
          <label class="param-label">
            行距系数<b>{{ xpCloudSettings.lineRatio.toFixed(2) }}</b>
          </label>
          <input
            v-model.number="xpCloudSettings.lineRatio"
            class="param-slider"
            type="range"
            min="1.1"
            max="1.9"
            step="0.05"
          />
          <span class="param-hint">越大词间距越松（1.2 即旧版密排观感）</span>
        </div>

        <div class="param-row">
          <label class="param-label">
            字间距<b>{{ xpCloudSettings.padX.toFixed(2) }}</b>
          </label>
          <input
            v-model.number="xpCloudSettings.padX"
            class="param-slider"
            type="range"
            min="0"
            max="0.8"
            step="0.05"
          />
          <span class="param-hint">词与词之间的横向缝隙</span>
        </div>

        <div class="param-row">
          <label class="param-label">
            截断字数<b>{{ xpCloudSettings.truncChars }}</b>
          </label>
          <input
            v-model.number="xpCloudSettings.truncChars"
            class="param-slider"
            type="range"
            min="8"
            max="24"
            step="1"
          />
          <span class="param-hint">超过则截断加省略号（英文按词边界断开）</span>
        </div>

        <div class="param-row">
          <label class="param-label">
            超长过滤<b>{{ xpCloudSettings.maxChars }}</b>
          </label>
          <input
            v-model.number="xpCloudSettings.maxChars"
            class="param-slider"
            type="range"
            min="12"
            max="40"
            step="1"
          />
          <span class="param-hint">超过该字数的词条不参与词云（只影响展示，不影响统计与推荐）</span>
        </div>

        <div class="param-checks">
          <label class="param-check">
            <input v-model="xpCloudSettings.onlyTranslated" type="checkbox" />
            仅显示中文译名（过滤拉丁转写长名）
          </label>
          <button class="param-reset" @click="resetCloudParams">恢复默认</button>
        </div>
      </div>

      <div v-if="coverageHint" class="cloud-hint">💡 {{ coverageHint }}</div>

      <div v-if="cloudLoading" class="cloud-state">统计加载中…</div>
      <div v-else-if="cloudError" class="cloud-state error">
        {{ cloudError }}
        <button class="retry-btn" @click="loadCloud(true)">重试</button>
      </div>
      <div v-else-if="cloudData" class="cloud-body">
        <XpWordCloud
          :tags="cloudData.tags"
          :height="cloudHeight"
          @select="handleTagSelect"
          @stats="handleCloudStats"
        />

        <!-- 布局统计：让用户直观理解参数影响（放入 X 条 / 排除 Y 条） -->
        <div v-if="cloudStats" class="cloud-stats">
          <span>数据源 <b>{{ cloudStats.source }}</b> 条</span>
          <span>· 实际放入 <b>{{ cloudStats.placed }}</b> 条</span>
          <span v-if="cloudStats.overflow > 0">· 放不下丢弃 <b>{{ cloudStats.overflow }}</b> 条</span>
          <span v-if="cloudStats.droppedLong > 0">
            · 超长排除 <b>{{ cloudStats.droppedLong }}</b> 条
          </span>
          <span v-if="cloudStats.droppedNoTrans > 0">
            · 无译名排除 <b>{{ cloudStats.droppedNoTrans }}</b> 条
          </span>
        </div>

        <ArtistTopList :artists="cloudData.artists" @select="handleArtistSelect" />

        <div class="cloud-meta">
          <span>
            库藏 {{ cloudData.meta.taggedComics }} / {{ cloudData.meta.totalComics }} 本参与统计 ·
            阅读信号 {{ cloudData.meta.readSignalComics }} 本
          </span>
          <span class="meta-right">
            上次重算 {{ lastRebuildText }}
            <button v-if="isAdmin" class="rebuild-btn" :disabled="rebuilding" @click="handleRebuild">
              {{ rebuilding ? '重算中…' : '重算统计' }}
            </button>
          </span>
        </div>
        <p class="cloud-tip">
          点击词条可直接搜索本地库；「阅读」视图按阅读次数 + 历史近期性 + 个人评分加权。
        </p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.leaderboard-page {
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 20px;
  padding-bottom: 30px;
}

/* ─── 顶部双视图切换 ─── */
.panel-switch {
  display: flex;
  gap: 8px;
}

.switch-btn {
  padding: 7px 16px;
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
  background: transparent;
  color: var(--app-text-2);
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.15s;
}

.switch-btn:hover {
  color: var(--app-text-strong);
}

.switch-btn.active {
  background: var(--app-accent, #4d9cff);
  border-color: transparent;
  color: #fff;
  font-weight: 600;
}

/* ─── 词云参数面板（Round32 视觉改版） ─── */
.cloud-param-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
  background: transparent;
  color: var(--app-text-2);
  font-size: 0.82rem;
  cursor: pointer;
  transition: all 0.15s;
}

.cloud-param-btn:hover,
.cloud-param-btn.open {
  color: var(--app-text-strong);
  border-color: var(--app-accent, #4d9cff);
}

.cloud-params {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px dashed var(--app-border-2);
  background: var(--app-surface-1, rgba(255, 255, 255, 0.02));
}

.param-row {
  display: grid;
  grid-template-columns: 150px 1fr;
  align-items: center;
  gap: 4px 12px;
}

.param-label {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 0.82rem;
  color: var(--app-text-2);
  white-space: nowrap;
}

.param-label b {
  color: var(--app-text-strong);
  font-variant-numeric: tabular-nums;
}

.param-slider {
  width: 100%;
  accent-color: #7c4dff;
}

.param-slider-pair {
  display: flex;
  gap: 10px;
}

.param-hint {
  grid-column: 2;
  font-size: 0.72rem;
  color: var(--app-text-3);
}

.param-checks {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 14px;
  font-size: 0.82rem;
  color: var(--app-text-2);
}

.param-check {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
}

.param-reset {
  margin-left: auto;
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid var(--app-border-2);
  background: transparent;
  color: var(--app-text-2);
  font-size: 0.78rem;
  cursor: pointer;
}

.param-reset:hover {
  color: var(--app-text-strong);
}

.reco-badge {
  min-width: 16px;
  padding: 0 5px;
  border-radius: 8px;
  background: var(--app-accent, #4d9cff);
  color: #fff;
  font-size: 0.7rem;
  line-height: 16px;
  text-align: center;
}

.ft-arrow {
  font-size: 0.7rem;
}

/* 布局统计行 */
.cloud-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  font-size: 0.73rem;
  color: var(--app-text-3);
}

.cloud-stats b {
  color: var(--app-text-2);
}

/* ─── 词云面板 ─── */
.cloud-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.segmented {
  display: inline-flex;
  padding: 3px;
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
  background: var(--app-bg-2, transparent);
}

.seg-btn {
  padding: 5px 12px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--app-text-2);
  font-size: 0.82rem;
  cursor: pointer;
  transition: all 0.15s;
}

.seg-btn:hover {
  color: var(--app-text-strong);
}

.seg-btn.active {
  background: var(--app-accent, #4d9cff);
  color: #fff;
  font-weight: 600;
}

.cloud-hint {
  padding: 8px 12px;
  border-radius: 6px;
  border-left: 3px solid #ffb74d;
  background: rgba(255, 183, 77, 0.1);
  color: var(--app-text-2);
  font-size: 0.8rem;
  line-height: 1.5;
}

.cloud-state {
  padding: 40px 0;
  text-align: center;
  color: var(--app-text-3);
  font-size: 0.85rem;
}

.cloud-state.error {
  color: #ff8a80;
}

.retry-btn,
.rebuild-btn {
  margin-left: 8px;
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid var(--app-border-2);
  background: transparent;
  color: var(--app-text-2);
  font-size: 0.78rem;
  cursor: pointer;
}

.retry-btn:hover,
.rebuild-btn:hover:not(:disabled) {
  color: var(--app-text-strong);
}

.rebuild-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

.cloud-body {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.cloud-meta {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 8px;
  font-size: 0.75rem;
  color: var(--app-text-3);
}

.meta-right {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.cloud-tip {
  margin: 0;
  font-size: 0.75rem;
  color: var(--app-text-3);
  line-height: 1.6;
}

.page-title {
  font-size: 1.3rem;
  color: var(--app-text-strong);
  margin: 0;
}

.podium-section {
  display: flex;
  justify-content: center;
  align-items: flex-end;
  gap: 20px;
  padding: 20px 0;
  border-bottom: 1px solid var(--app-border-2);
}

.podium-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 170px;
}

.rank-1-wrapper {
  width: 210px;
  transform: translateY(-10px);
}

.podium-crown {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--app-text-2);
  margin-bottom: 8px;
}

.crown-gold {
  color: #ffd700;
  font-size: 1rem;
}

.read-count {
  margin-top: 8px;
  font-size: 0.85rem;
  color: var(--app-text-3);
}

.gold-text {
  color: #ffd700;
  font-weight: 600;
}

.rest-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-subtitle {
  font-size: 1.1rem;
  color: var(--app-text-2);
  margin: 0;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 16px;
}

.grid-item-wrapper {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.sub-read-count {
  margin-top: 6px;
  font-size: 0.78rem;
  color: var(--app-text-3);
}

/* 📱 窄屏适配：领奖台三卡收紧（flex 均分 + max-width，极端窄屏自动收缩不溢出） */
@media (max-width: 767px) {
  .leaderboard-page {
    padding: 12px;
    padding-bottom: 24px;
    gap: 16px;
  }

  .page-title {
    font-size: 1.05rem;
  }

  .switch-btn {
    flex: 1;
    padding: 7px 10px;
    font-size: 0.85rem;
  }

  .cloud-toolbar {
    gap: 8px;
  }

  .seg-btn {
    padding: 5px 9px;
    font-size: 0.78rem;
  }

  .cloud-meta {
    flex-direction: column;
  }

  .podium-section {
    gap: 8px;
    padding: 14px 0;
  }
  .podium-item {
    flex: 1;
    min-width: 0;
    max-width: 104px;
  }
  .rank-1-wrapper {
    flex: 1;
    max-width: 124px;
    transform: translateY(-6px);
  }
  .podium-crown {
    font-size: 0.72rem;
    margin-bottom: 6px;
    white-space: nowrap;
  }
  .crown-gold {
    font-size: 0.78rem;
  }
  .read-count {
    font-size: 0.72rem;
    text-align: center;
    white-space: nowrap;
  }

  .card-grid {
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 12px;
  }
}
</style>
