<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  onlineSearchConfig,
  offlineSearchConfig,
  applySearchOptionsInherit,
} from '@/stores/searchStore'
import { useUI } from '@/composables/useUI'
import { useModeStore } from '@/stores/modeStore'
import TagChip from '@/components/TagChip.vue'
import FilterDrawer from '@/components/FilterDrawer.vue'
import type { TagItem } from '@/stores/tagStore'
import type { FilterParams } from '@/types/comic'
import { http } from '@/utils/request'
import { safeSetItem } from '@/utils/storage'
// 🎯 f_search 标准语法格式化：联想点击时按模式输出（在线标准语法 / 离线裸格式）
// 🎯 多 tag 联想：extractSuggestQuery 提取输入串的「最后一个 token」作为联想词
import { formatFSearchTag, extractSuggestQuery } from '@/utils/tagFilter'

const router = useRouter()
const route = useRoute()
const modeStore = useModeStore()
const { toast } = useUI()

const keyword = ref('')
const isFocused = ref(false)
// 搜索框 DOM 引用：联想点击后保持焦点，便于连续输入多个 tag
const searchInputRef = ref<HTMLInputElement | null>(null)

const suggestedTags = ref<TagItem[]>([])
let suggestTimer: number | null = null
// 请求序号守卫（S4）：每次输入递增，丢弃过期响应，避免旧请求后到覆盖新输入结果
let suggestSeq = 0
// 联想点击抑制标记：点击联想项后 Vue 重渲染会把下拉 DOM 分离，其冒泡的 click 会被
// handleOutsideClick 误判为「点击外部」而关闭联想。此处用短生命周期标记抑制该误判。
let suppressOutsideClose = false

// 🎯 1. 监听当前域 Store 的 keyword 变化，同步反显到搜索框输入框内（如点击 TagChip 时）
const activeStoreKeyword = computed(() => {
  return modeStore.isOffline ? offlineSearchConfig.value.keyword : onlineSearchConfig.value.keyword
})

watch(
  activeStoreKeyword,
  (newKw) => {
    if (newKw !== undefined && newKw !== keyword.value) {
      keyword.value = newKw
    }
  },
  { immediate: true },
)

// 🎯 2. 仅清空当前域 Store 关键词（不导航）。懒更新（S9/D7）下删空/✕ 不再调用它，
// 只有用户显式点「搜索」（triggerSearch 空查询分支）才清空当前搜索并回归首页。
const clearStoreKeyword = () => {
  if (modeStore.isOffline) {
    offlineSearchConfig.value.keyword = ''
  } else {
    onlineSearchConfig.value.keyword = ''
  }
}

// 🎯 3. 监听输入框内容：删空仅清空筛选（不导航）；有字则以「最后一个 token」请求联想（多 tag 支持）
watch(keyword, (newVal) => {
  const { query } = extractSuggestQuery(newVal)
  const wholeEmpty = !newVal.trim()

  // 输入整体为空：懒更新（S9/D7）——仅取消未发请求、废弃在途响应、清空联想，
  // 不写 Store、不导航；URL 保留 kw、列表不重置，由用户显式点「搜索」才清空当前搜索。
  if (wholeEmpty) {
    if (suggestTimer) clearTimeout(suggestTimer)
    suggestSeq++
    suggestedTags.value = []
    return
  }

  // 有输入但最后一个 token 无有效联想词（如刚选完一个完整 tag）：仅清空联想列表
  if (!query) {
    if (suggestTimer) clearTimeout(suggestTimer)
    suggestSeq++
    suggestedTags.value = []
    return
  }

  if (suggestTimer) clearTimeout(suggestTimer)
  const seq = ++suggestSeq
  // 🧹 发新请求前先清空旧联想，避免旧结果在新结果到达前残留/闪烁（修复3a）
  suggestedTags.value = []
  suggestTimer = window.setTimeout(async () => {
    try {
      // 只发「未完成短语」，避免整串（如 group:"da hootch$" large）在字典里无子串匹配
      const data = await http<TagItem[]>('/tags/suggest', { params: { q: query, limit: 8 } })
      // 仅当序号最新且未完成短语未再变化时写入（S4）
      if (seq === suggestSeq && extractSuggestQuery(keyword.value).query === query) {
        // 🧹 相关性过滤（修复3a）：仅保留与查询词相关的结果——
        // 带冒号（用户显式输入命名空间）按 ns:key 匹配；裸词按 key/中文名匹配，
        // 避免旧后端 matchNS 子串匹配把命名空间命中（如 "la"→female:stockings）混入
        const ql = query.toLowerCase()
        const hasColon = ql.includes(':')
        suggestedTags.value = (Array.isArray(data) ? data : []).filter((t) => {
          if (!t || typeof t !== 'object') return false
          const key = (t.key || '').toLowerCase()
          const name = (t.name || '').toLowerCase()
          const nsKey = `${t.namespace || ''}:${key}`.toLowerCase()
          return hasColon ? nsKey.includes(ql) : key.includes(ql) || name.includes(ql)
        })
      }
    } catch (e) {
      console.error('获取标签联想失败:', e)
    }
  }, 150)
})

// 搜索历史记录 (持久化存在 localStorage)
const searchHistory = ref<string[]>([])

const loadSearchHistory = () => {
  try {
    const saved = localStorage.getItem('app_search_history')
    const parsed = saved ? JSON.parse(saved) : ['female:nun', 'language:chinese', 'full color']
    // 防御（问题8）：历史数据可能残留非字符串条目，统一清洗为字符串
    searchHistory.value = Array.isArray(parsed)
      ? parsed.filter((h): h is string => typeof h === 'string').slice(0, 10)
      : []
  } catch {
    searchHistory.value = []
  }
}

const saveSearchHistory = () => {
  // 配额保护：localStorage 满时走 safeSetItem 自动回收，避免 setItem 抛 QuotaExceededError
  safeSetItem('app_search_history', JSON.stringify(searchHistory.value.slice(0, 10)))
}

const filteredHistory = computed(() => {
  const kw = keyword.value.toLowerCase().trim()
  if (!kw) return searchHistory.value
  return searchHistory.value.filter((h) => typeof h === 'string' && h.toLowerCase().includes(kw))
})

// 联想渲染防御（问题8）：后端 /tags/suggest 返回异常条目时逐项清洗，
// 避免 name/count 类型异常导致 TagChip 的 .replace() / toLocaleString() 抛错白屏。
const safeSuggestedTags = computed<TagItem[]>(() => {
  if (!Array.isArray(suggestedTags.value)) return []
  return suggestedTags.value
    .filter((t): t is TagItem => !!t && typeof t === 'object')
    .map((t) => ({
      namespace: typeof t.namespace === 'string' ? t.namespace : 'other',
      key: typeof t.key === 'string' ? t.key : '',
      name: typeof t.name === 'string' ? t.name : '',
      intro: typeof t.intro === 'string' ? t.intro : undefined,
      count: typeof t.count === 'number' && Number.isFinite(t.count) ? t.count : 0,
    }))
    .filter((t) => t.key !== '')
    .slice(0, 8)
})

// 🔗 E-Hentai 直链 / 裸 gid/token 解析
const resolveEHDetailLink = (text: string): { id: string; token: string } | null => {
  // 画廊直链：https://exhentai.org/g/<gid>/<token>/
  const gLink = text.match(/(?:exhentai|e-hentai)\.org\/g\/(\d{1,10})\/([0-9a-fA-F]{10})/i)
  if (gLink) return { id: gLink[1], token: gLink[2] }
  // 分页直链：https://exhentai.org/s/<page>/<gid>-<token>
  const sLink = text.match(
    /(?:exhentai|e-hentai)\.org\/s\/[0-9a-zA-Z]+\/(\d{1,10})-([0-9a-fA-F]{10})/i,
  )
  if (sLink) return { id: sLink[1], token: sLink[2] }
  // 裸 gid/token 形式：2887644/32e22f8cb4
  const bare = text.match(/^(\d{1,10})\/([0-9a-fA-F]{10})$/)
  if (bare) return { id: bare[1], token: bare[2] }
  return null
}

// 触发搜索
const triggerSearch = (queryText?: string) => {
  const finalQuery = (queryText !== undefined ? queryText : keyword.value).trim()

  if (!finalQuery) {
    // 空查询：清空筛选后回归当前模式首页（仅「用户点击搜索」这一显式动作触发；
    // 清空搜索栏本身不跳转，回归首页由这里负责）
    if (activeStoreKeyword.value !== '') clearStoreKeyword()
    const isOffline = modeStore.isOffline
    if (isOffline) {
      if (!route.path.startsWith('/offline/home')) router.push('/offline/home')
    } else {
      if (!route.path.startsWith('/online/home')) router.push('/online/home')
    }
    return
  }

  // 🔗 E-Hentai 直链跳转：识别后直接进入在线画廊详情页，不作为普通搜索
  const ehLink = resolveEHDetailLink(finalQuery)
  if (ehLink) {
    isFocused.value = false
    router.push({ path: '/online/detail', query: { id: ehLink.id, token: ehLink.token } })
    return
  }

  // 🎯 依据「搜索选项继承」偏好，提交新搜索前重置不应继承的筛选条件
  applySearchOptionsInherit(currentScope.value)

  keyword.value = finalQuery

  if (!searchHistory.value.includes(finalQuery)) {
    searchHistory.value.unshift(finalQuery)
    saveSearchHistory()
  }

  isFocused.value = false
  const isOffline = modeStore.isOffline

  if (isOffline) {
    offlineSearchConfig.value.keyword = finalQuery
    if (!route.path.startsWith('/offline/home')) router.push('/offline/home')
  } else {
    onlineSearchConfig.value.keyword = finalQuery
    if (!route.path.startsWith('/online/home')) router.push('/online/home')
  }
}

// 清空按钮：懒更新（S9/D7）——仅清空输入框，不写 Store、不导航；
// URL 保留 kw、列表不重置，由用户显式点「搜索」才清空当前搜索并回归首页
const handleClearInput = () => {
  keyword.value = ''
}

// 联想点击：仅替换「最后一个 token」，保留前面已选的 tag，并保持输入焦点便于连续输入多 tag
const handleTagSuggestClick = (tag: TagItem) => {
  const { prefix, negative } = extractSuggestQuery(keyword.value)
  const inserted = formatFSearchTag(tag.namespace, tag.key, modeStore.isOffline)
  keyword.value = prefix
    ? `${prefix} ${negative ? '-' : ''}${inserted}`
    : `${negative ? '-' : ''}${inserted}`
  // 抑制紧随其后的冒泡 click 被 handleOutsideClick 误判为「点击外部」而关闭联想
  suppressOutsideClose = true
  isFocused.value = true
  searchInputRef.value?.focus()
  setTimeout(() => {
    suppressOutsideClose = false
  }, 0)
}

const removeHistoryItem = (item: string, e: Event) => {
  e.stopPropagation()
  searchHistory.value = searchHistory.value.filter((h) => h !== item)
  saveSearchHistory()
}

const clearAllHistory = (e: Event) => {
  e.stopPropagation()
  searchHistory.value = []
  saveSearchHistory()
  toast.info('搜索历史已清空')
}

const searchBarRef = ref<HTMLElement | null>(null)
const handleOutsideClick = (e: MouseEvent) => {
  // 联想点击后（含被分离的下拉 DOM）冒泡到此的 click 一律跳过，避免误关联想
  if (suppressOutsideClose) return
  if (searchBarRef.value && !searchBarRef.value.contains(e.target as Node)) {
    isFocused.value = false
  }
}

// 📱 移动形态（≤1024px）：搜索框提示文案精简，节省横向空间
const narrowMql = window.matchMedia('(max-width: 1024px)')
const isNarrow = ref(narrowMql.matches)
const onNarrowChange = (e: MediaQueryListEvent) => {
  isNarrow.value = e.matches
}
const searchPlaceholder = computed(() =>
  isNarrow.value ? '搜索标题、作者或 Tag...' : '搜索标题、作者或 Tag (支持中英文联想)...',
)

onMounted(() => {
  loadSearchHistory()
  window.addEventListener('click', handleOutsideClick)
  narrowMql.addEventListener('change', onNarrowChange)
})

onUnmounted(() => {
  window.removeEventListener('click', handleOutsideClick)
  narrowMql.removeEventListener('change', onNarrowChange)
})

// 🎛️ 筛选并入搜索：入口在搜索框内（FilterDrawer 全屏抽屉，按当前模式分域保存不串味）
const isFilterOpen = ref(false)

const currentScope = computed<'online' | 'offline'>(() => modeStore.currentMode)

const activeSearchConfig = computed(() => {
  return currentScope.value === 'offline' ? offlineSearchConfig.value : onlineSearchConfig.value
})

// 激活态红点：当前域存在任一非默认筛选条件时点亮（纯搜索词 keyword 不计入筛选）
const hasActiveFilters = computed(() => {
  const c = activeSearchConfig.value
  return (
    (c.keywords && c.keywords.length > 0) ||
    (c.activeCategories && c.activeCategories.length < 10) ||
    (c.minRating && c.minRating > 0) ||
    (c.minPages && c.minPages > 0) ||
    (c.maxPages && c.maxPages > 0) ||
    !!c.onlyDownloaded ||
    (c.language && c.language !== 'All') ||
    !!c.onlyRemoved ||
    !!c.onlyTorrents ||
    !!c.disableLangFilter ||
    !!c.disableUploaderFilter ||
    !!c.disableTagFilter
  )
})

// 保存筛选设置到对应的域中，不互相串味
const handleApplyFilters = (filters: Partial<FilterParams>) => {
  if (currentScope.value === 'offline') {
    Object.assign(offlineSearchConfig.value, filters)
  } else {
    Object.assign(onlineSearchConfig.value, filters)
  }
  toast.success(`[${currentScope.value === 'offline' ? '离线' : '在线'}] 筛选条件已生效`)
}
</script>

<template>
  <div ref="searchBarRef" class="search-bar-container">
    <div class="input-wrapper" :class="{ focused: isFocused }">
      <span class="search-icon">🔍</span>
      <input
        ref="searchInputRef"
        v-model="keyword"
        type="text"
        class="search-input"
        :placeholder="searchPlaceholder"
        @focus="isFocused = true"
        @keyup.enter="triggerSearch()"
      />
      <!-- 🎛️ 筛选入口（原 TopBar 齿轮按钮移入搜索框内，激活态显示红点） -->
      <button class="filter-trigger-btn" title="筛选" @click="isFilterOpen = true">
        <span class="filter-icon">⚙️</span>
        <span v-if="hasActiveFilters" class="filter-active-dot"></span>
      </button>
      <button v-if="keyword" class="clear-input-btn" @click="handleClearInput">✕</button>
      <button class="search-submit-btn" @click="triggerSearch()">搜索</button>
    </div>

    <FilterDrawer
      v-model:visible="isFilterOpen"
      :config="activeSearchConfig"
      @apply="handleApplyFilters"
    />

    <div
      v-if="isFocused && (filteredHistory.length > 0 || safeSuggestedTags.length > 0)"
      class="search-dropdown"
    >
      <div v-if="filteredHistory.length > 0" class="dropdown-section">
        <div class="section-header">
          <span>🕒 {{ keyword.trim() ? '历史匹配' : '搜索历史' }}</span>
          <button v-if="!keyword.trim()" class="clear-history-text-btn" @click="clearAllHistory">
            清空
          </button>
        </div>

        <div class="history-chips-wrapper">
          <span
            v-for="item in filteredHistory"
            :key="item"
            class="history-chip"
            @click="triggerSearch(item)"
          >
            <span class="history-text">{{ item }}</span>
            <span class="delete-chip-btn" title="删除记录" @click="removeHistoryItem(item, $event)"
              >✕</span
            >
          </span>
        </div>
      </div>

      <div v-if="safeSuggestedTags.length > 0" class="dropdown-section">
        <div class="section-header">🔥 热门 Tag 推荐</div>
        <div class="vertical-tag-list">
          <div
            v-for="tag in safeSuggestedTags"
            :key="`${tag.namespace}:${tag.key}`"
            class="vertical-tag-item"
            @click="handleTagSuggestClick(tag)"
          >
            <!-- disableQuickSearch：联想下拉里点击芯片不触发 TagChip 快捷搜索（在线新开标签/离线跳转），
                而是冒泡给父级 handleTagSuggestClick 把 tag 插入输入框，支持连续输入多 tag（修复3b） -->
            <TagChip :tag="tag" :disable-quick-search="true" />
            <span v-if="tag.count" class="tag-count-badge"
              >🔥 {{ tag.count.toLocaleString() }}</span
            >
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.search-bar-container {
  position: relative;
  width: 100%;
  max-width: 480px;
}

.input-wrapper {
  display: flex;
  align-items: center;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 20px;
  padding: 3px 6px 3px 12px;
  transition: all 0.2s ease;
}

.input-wrapper.focused {
  border-color: #007acc;
  box-shadow: 0 0 10px rgba(0, 122, 204, 0.25);
  background-color: var(--app-surface-2-hover);
}

.search-icon {
  font-size: 0.9rem;
  color: var(--app-text-muted);
  margin-right: 8px;
}

.search-input {
  flex: 1;
  background: transparent;
  border: none;
  color: var(--app-text-strong);
  font-size: 0.88rem;
  outline: none;
}

.clear-input-btn {
  background: transparent;
  border: none;
  color: var(--app-text-muted);
  cursor: pointer;
  padding: 0 6px;
  font-size: 0.8rem;
}
.clear-input-btn:hover {
  color: var(--app-text-strong);
}

.search-submit-btn {
  background-color: #007acc;
  color: #fff;
  border: none;
  border-radius: 14px;
  padding: 4px 14px;
  font-size: 0.8rem;
  cursor: pointer;
  font-weight: 500;
  transition: opacity 0.2s;
}
.search-submit-btn:hover {
  opacity: 0.85;
}

/* 🎛️ 筛选入口按钮：搜索框内右侧，激活态红点提示当前有筛选条件 */
.filter-trigger-btn {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  color: var(--app-text-muted);
  cursor: pointer;
  padding: 4px 6px;
  margin-left: 2px;
  border-radius: 50%;
  font-size: 0.9rem;
  transition: all 0.2s;
  flex-shrink: 0;
}

.filter-trigger-btn:hover {
  color: #007acc;
  background-color: var(--app-surface-3);
}

.filter-active-dot {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background-color: #ef4444;
  border: 1px solid var(--app-surface-2);
}

.search-dropdown {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  border-radius: 12px;
  box-shadow: 0 10px 28px rgba(0, 0, 0, 0.6);
  padding: 12px;
  z-index: 1000;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.dropdown-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.75rem;
  color: var(--app-text-3);
  font-weight: bold;
}

.clear-history-text-btn {
  background: transparent;
  border: none;
  color: var(--app-text-muted);
  font-size: 0.72rem;
  cursor: pointer;
}
.clear-history-text-btn:hover {
  color: #ef4444;
}

.history-chips-wrapper {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.history-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 3px 10px;
  border-radius: 16px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.history-chip:hover {
  background-color: var(--app-surface-3-hover);
  border-color: #007acc;
  color: var(--app-text-strong);
}

.delete-chip-btn {
  font-size: 0.7rem;
  color: var(--app-text-muted);
  border-radius: 50%;
  padding: 0 2px;
}

.delete-chip-btn:hover {
  color: #ef4444;
}

/* 联想列表与热度展示 */
.vertical-tag-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.vertical-tag-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 8px;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 0.15s;
}

.vertical-tag-item:hover {
  background-color: var(--app-surface-2-hover);
}

.tag-count-badge {
  font-size: 0.72rem;
  color: #ff9800;
  font-weight: 500;
}
</style>
