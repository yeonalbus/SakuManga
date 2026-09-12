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
import XpWordCloud from '@/components/XpWordCloud.vue'
import ArtistTopList from '@/components/ArtistTopList.vue'
import { fetchXpCloudApi, rebuildXpCloudApi } from '@/api/xpCloud'
import { formatFSearchTag } from '@/utils/tagFilter'
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
      </div>

      <div v-if="coverageHint" class="cloud-hint">💡 {{ coverageHint }}</div>

      <div v-if="cloudLoading" class="cloud-state">统计加载中…</div>
      <div v-else-if="cloudError" class="cloud-state error">
        {{ cloudError }}
        <button class="retry-btn" @click="loadCloud(true)">重试</button>
      </div>
      <div v-else-if="cloudData" class="cloud-body">
        <XpWordCloud :tags="cloudData.tags" :height="isMobileDevice ? 300 : 400" @select="handleTagSelect" />

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
