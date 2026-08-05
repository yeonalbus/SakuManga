<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { onlineSearchConfig, offlineSearchConfig } from '@/stores/appStore'
import { useUI } from '@/composables/useUI'
import TagChip from '@/components/TagChip.vue'
import type { TagItem } from '@/stores/tagStore'
import { http } from '@/utils/request'

const router = useRouter()
const route = useRoute()
const { toast } = useUI()

const keyword = ref('')
const isFocused = ref(false)

const suggestedTags = ref<TagItem[]>([])
let suggestTimer: number | null = null

// 🎯 1. 监听当前域 Store 的 keyword 变化，同步反显到搜索框输入框内（如点击 TagChip 时）
const activeStoreKeyword = computed(() => {
  return route.path.startsWith('/offline')
    ? offlineSearchConfig.value.keyword
    : onlineSearchConfig.value.keyword
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

// 🎯 2. 重置并回归首页函数
const resetToHome = () => {
  const isOffline = route.path.startsWith('/offline')
  if (isOffline) {
    offlineSearchConfig.value.keyword = ''
    if (!route.path.startsWith('/offline/home')) router.push('/offline/home')
  } else {
    onlineSearchConfig.value.keyword = ''
    if (!route.path.startsWith('/online/home')) router.push('/online/home')
  }
}

// 🎯 3. 监听输入框内容：变空则重置回归首页；有字则请求热度联想
watch(keyword, (newVal) => {
  const q = newVal.trim()
  if (!q) {
    suggestedTags.value = []
    // 当删空文字且 Store 里还有关键字时，自动回归首页
    if (activeStoreKeyword.value !== '') {
      resetToHome()
    }
    return
  }

  if (suggestTimer) clearTimeout(suggestTimer)
  suggestTimer = window.setTimeout(async () => {
    try {
      suggestedTags.value = await http<TagItem[]>('/tags/suggest', { params: { q, limit: 8 } })
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
    searchHistory.value = saved
      ? JSON.parse(saved)
      : ['female:nun', 'language:chinese', 'full color']
  } catch (e) {
    searchHistory.value = []
  }
}

const saveSearchHistory = () => {
  localStorage.setItem('app_search_history', JSON.stringify(searchHistory.value.slice(0, 10)))
}

const filteredHistory = computed(() => {
  const kw = keyword.value.toLowerCase().trim()
  if (!kw) return searchHistory.value
  return searchHistory.value.filter((h) => h.toLowerCase().includes(kw))
})

// 触发搜索
const triggerSearch = (queryText?: string) => {
  const finalQuery = (queryText !== undefined ? queryText : keyword.value).trim()

  if (!finalQuery) {
    resetToHome()
    return
  }

  keyword.value = finalQuery

  if (!searchHistory.value.includes(finalQuery)) {
    searchHistory.value.unshift(finalQuery)
    saveSearchHistory()
  }

  isFocused.value = false
  const isOffline = route.path.startsWith('/offline')

  if (isOffline) {
    offlineSearchConfig.value.keyword = finalQuery
    if (!route.path.startsWith('/offline/home')) router.push('/offline/home')
  } else {
    onlineSearchConfig.value.keyword = finalQuery
    if (!route.path.startsWith('/online/home')) router.push('/online/home')
  }
}

// 清空按钮点击事件
const handleClearInput = () => {
  keyword.value = ''
  resetToHome()
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
  if (searchBarRef.value && !searchBarRef.value.contains(e.target as Node)) {
    isFocused.value = false
  }
}

onMounted(() => {
  loadSearchHistory()
  window.addEventListener('click', handleOutsideClick)
})

onUnmounted(() => {
  window.removeEventListener('click', handleOutsideClick)
})
</script>

<template>
  <div ref="searchBarRef" class="search-bar-container">
    <div class="input-wrapper" :class="{ focused: isFocused }">
      <span class="search-icon">🔍</span>
      <input
        v-model="keyword"
        type="text"
        class="search-input"
        placeholder="搜索标题、作者或 Tag (支持中英文联想)..."
        @focus="isFocused = true"
        @keyup.enter="triggerSearch()"
      />
      <button v-if="keyword" class="clear-input-btn" @click="handleClearInput">✕</button>
      <button class="search-submit-btn" @click="triggerSearch()">搜索</button>
    </div>

    <div
      v-if="isFocused && (filteredHistory.length > 0 || suggestedTags.length > 0)"
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

      <div v-if="suggestedTags.length > 0" class="dropdown-section">
        <div class="section-header">🔥 热门 Tag 推荐</div>
        <div class="vertical-tag-list">
          <div
            v-for="tag in suggestedTags"
            :key="`${tag.namespace}:${tag.key}`"
            class="vertical-tag-item"
            @click="
              triggerSearch(tag.namespace !== 'other' ? `${tag.namespace}:${tag.key}` : tag.key)
            "
          >
            <TagChip :tag="tag" />
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
  background-color: #1a1a1d;
  border: 1px solid #333;
  border-radius: 20px;
  padding: 4px 6px 4px 12px;
  transition: all 0.2s ease;
}

.input-wrapper.focused {
  border-color: #007acc;
  box-shadow: 0 0 10px rgba(0, 122, 204, 0.25);
  background-color: #222225;
}

.search-icon {
  font-size: 0.9rem;
  color: #666;
  margin-right: 8px;
}

.search-input {
  flex: 1;
  background: transparent;
  border: none;
  color: #fff;
  font-size: 0.88rem;
  outline: none;
}

.clear-input-btn {
  background: transparent;
  border: none;
  color: #666;
  cursor: pointer;
  padding: 0 6px;
  font-size: 0.8rem;
}
.clear-input-btn:hover {
  color: #fff;
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

.search-dropdown {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  background-color: #1e1e22;
  border: 1px solid #333;
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
  color: #888;
  font-weight: bold;
}

.clear-history-text-btn {
  background: transparent;
  border: none;
  color: #666;
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
  background-color: #2a2a2e;
  border: 1px solid #3d3d42;
  color: #ccc;
  padding: 3px 10px;
  border-radius: 16px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.history-chip:hover {
  background-color: #333338;
  border-color: #007acc;
  color: #fff;
}

.delete-chip-btn {
  font-size: 0.7rem;
  color: #666;
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
  background-color: rgba(255, 255, 255, 0.05);
}

.tag-count-badge {
  font-size: 0.72rem;
  color: #ff9800;
  font-weight: 500;
}
</style>
