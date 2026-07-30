<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { onlineSearchConfig, offlineSearchConfig } from '@/stores/appStore'
import { useUI } from '@/composables/useUI'

const router = useRouter()
const route = useRoute()
const { toast } = useUI()

// 输入框关键词与焦点状态
const keyword = ref('')
const isFocused = ref(false)

// 预设常用 Tag 候选词库 (E 站经典命名规范)
const candidateTags = [
  'female:nun',
  'female:big breasts',
  'female:glasses',
  'female:sole female',
  'male:sole male',
  'male:shotacon',
  'language:chinese',
  'language:translated',
  'artist:hiten',
  'artist:tsukino',
  'parody:original',
  'full color',
  'uncensored',
]

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

// --------------------------------------------------
// 核心逻辑：横排历史匹配 + 竖排 Tag 联想
// --------------------------------------------------

// 1. 横排历史记录（输入为空时显示全部，输入后只保留最匹配的）
const filteredHistory = computed(() => {
  const kw = keyword.value.toLowerCase().trim()
  if (!kw) return searchHistory.value
  return searchHistory.value.filter((h) => h.toLowerCase().includes(kw))
})

// 2. 竖排 Tag 智能联想（仅当有输入内容时显示匹配项）
const filteredTags = computed(() => {
  const kw = keyword.value.toLowerCase().trim()
  if (!kw) return []
  return candidateTags.filter((tag) => tag.toLowerCase().includes(kw))
})

// 触发搜索动作
const triggerSearch = (queryText?: string) => {
  const finalQuery = (queryText !== undefined ? queryText : keyword.value).trim()
  keyword.value = finalQuery

  const isOffline = route.path.startsWith('/offline')

  // 🟢 写入对应域的 keyword，保留分类等其他 Filter 选项不变！
  if (isOffline) {
    offlineSearchConfig.value.keyword = finalQuery
    if (!route.path.startsWith('/offline/home')) router.push('/offline/home')
  } else {
    onlineSearchConfig.value.keyword = finalQuery
    if (!route.path.startsWith('/online/home')) router.push('/online/home')
  }
}

// 删除单条历史
const removeHistoryItem = (item: string, e: Event) => {
  e.stopPropagation()
  searchHistory.value = searchHistory.value.filter((h) => h !== item)
  saveSearchHistory()
}

// 清空所有历史
const clearAllHistory = (e: Event) => {
  e.stopPropagation()
  searchHistory.value = []
  saveSearchHistory()
  toast.info('搜索历史已清空')
}

// 点击外部关闭 Dropdown
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
        placeholder="搜索标题、作者或 Tag (例如 female:nun)..."
        @focus="isFocused = true"
        @keyup.enter="triggerSearch()"
      />
      <button v-if="keyword" class="clear-input-btn" @click="keyword = ''">✕</button>
      <button class="search-submit-btn" @click="triggerSearch()">搜索</button>
    </div>

    <div
      v-if="isFocused && (filteredHistory.length > 0 || filteredTags.length > 0)"
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

      <div v-if="filteredTags.length > 0" class="dropdown-section">
        <div class="section-header">🏷️ Tag 智能联想</div>
        <div class="vertical-tag-list">
          <div
            v-for="tag in filteredTags"
            :key="tag"
            class="vertical-tag-item"
            @click="triggerSearch(tag)"
          >
            <span class="tag-icon">🏷️</span>
            <span class="tag-name">{{ tag }}</span>
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

/* 输入框包装壳 */
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

/* 下拉 Dropdown 容器 */
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

/* ─── 历史记录：“圈圈”胶囊气泡（横向排布） ─── */
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
  border-radius: 16px; /* 圈圈胶囊造型 */
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

/* ─── Tag 智能联想：竖向条列展示 ─── */
.vertical-tag-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.vertical-tag-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 0.85rem;
  color: #007acc;
  cursor: pointer;
  transition: background-color 0.15s;
}

.vertical-tag-item:hover {
  background-color: rgba(0, 122, 204, 0.15);
  color: #0099ff;
}

.tag-icon {
  font-size: 0.8rem;
  opacity: 0.8;
}
</style>
