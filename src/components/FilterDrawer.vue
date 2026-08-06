<script setup lang="ts">
import { ref, reactive, watch } from 'vue'
import type { SearchConfig } from '@/types/comic'
import { useTagSuggest, type TagSuggestion } from '@/composables/useTagSuggest'
import TagChip from '@/components/TagChip.vue'
// Round3-任务6：负向 chips（「- 」前缀红色/删除线区分）
import { isNegativeItem } from '@/utils/tagFilter'

const props = defineProps<{
  visible: boolean
  config?: SearchConfig
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
  (e: 'apply', filters: Partial<SearchConfig>): void
}>()

interface CategoryConfig {
  key: string
  label: string
  color: string
}

// 🟢 1. 分类列表定义 (与数据源 100% 一致)
const categories: CategoryConfig[] = [
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

// 🟢 2. 抽屉内部响应式状态
const filterState = reactive({
  keywords: [] as string[],
  activeCategories: new Set<string>(categories.map((c) => c.key)),
  minRating: 0,
  minPages: undefined as number | undefined,
  maxPages: undefined as number | undefined,
  onlyDownloaded: false,
  language: 'All',
  onlyRemoved: false,
  onlyTorrents: false,
  disableLangFilter: false,
  disableUploaderFilter: false,
  disableTagFilter: false,
})

// 关键词输入框的临时响应式变量
const inputKeyword = ref('')
const inputFocused = ref(false)

// ─── Round3-任务5：tag 联想（支持负向「- 」前缀解析，复用 /tags/suggest）───
const { suggestions, loading, refresh, clear: clearSuggest } = useTagSuggest(
  () => inputKeyword.value,
)

// 选中联想项：负向项以「- namespace:key」压入队列
const pickSuggestion = (sug: TagSuggestion) => {
  if (!filterState.keywords.includes(sug.insertText)) {
    filterState.keywords.push(sug.insertText)
  }
  inputKeyword.value = ''
  clearSuggest()
  inputFocused.value = false
}

// 🟢 3. 打开抽屉时回填当前域的 Config
watch(
  () => props.visible,
  (isOpen) => {
    if (isOpen) {
      inputFocused.value = false
      clearSuggest()
    }
    if (isOpen && props.config) {
      filterState.keywords = [...(props.config.keywords || [])]
      filterState.activeCategories = new Set(
        props.config.activeCategories || categories.map((c) => c.key),
      )
      filterState.minRating = props.config.minRating || 0
      filterState.minPages = props.config.minPages
      filterState.maxPages = props.config.maxPages
      filterState.onlyDownloaded = !!props.config.onlyDownloaded
      // ─── E-Hentai 高级筛选回填 ───
      filterState.language = props.config.language || 'All'
      filterState.onlyRemoved = !!props.config.onlyRemoved
      filterState.onlyTorrents = !!props.config.onlyTorrents
      filterState.disableLangFilter = !!props.config.disableLangFilter
      filterState.disableUploaderFilter = !!props.config.disableUploaderFilter
      filterState.disableTagFilter = !!props.config.disableTagFilter
    }
  },
  { immediate: true },
)

// 切换分类选中状态
const toggleCategory = (key: string) => {
  if (filterState.activeCategories.has(key)) {
    filterState.activeCategories.delete(key)
  } else {
    filterState.activeCategories.add(key)
  }
}

// 🟢 4. 键盘 Enter 事件逻辑：有字压入队列，框为空按 Enter 则清空队列
const handleKeydownEnter = () => {
  const text = inputKeyword.value.trim()
  if (text) {
    if (!filterState.keywords.includes(text)) {
      filterState.keywords.push(text)
    }
    inputKeyword.value = ''
    clearSuggest()
  } else {
    filterState.keywords = []
    clearSuggest()
  }
}

// 🟢 5. 移除单个关键词
const removeKeyword = (index: number) => {
  filterState.keywords.splice(index, 1)
}

// 关闭抽屉
const handleClose = () => {
  clearSuggest()
  inputFocused.value = false
  emit('update:visible', false)
}

// 重置抽屉
const handleReset = () => {
  inputKeyword.value = ''
  clearSuggest()
  inputFocused.value = false
  filterState.keywords = []
  filterState.activeCategories = new Set(categories.map((c) => c.key))
  filterState.minRating = 0
  filterState.minPages = undefined
  filterState.maxPages = undefined
  filterState.onlyDownloaded = false
  filterState.language = 'All'
  filterState.onlyRemoved = false
  filterState.onlyTorrents = false
  filterState.disableLangFilter = false
  filterState.disableUploaderFilter = false
  filterState.disableTagFilter = false
}

// 应用筛选条件并通知外部处理
const handleApply = () => {
  emit('apply', {
    keywords: [...filterState.keywords],
    activeCategories: Array.from(filterState.activeCategories),
    minRating: filterState.minRating,
    minPages: filterState.minPages,
    maxPages: filterState.maxPages,
    onlyDownloaded: filterState.onlyDownloaded,
    // ─── E-Hentai 高级筛选全量透传 ───
    language: filterState.language,
    onlyRemoved: filterState.onlyRemoved,
    onlyTorrents: filterState.onlyTorrents,
    disableLangFilter: filterState.disableLangFilter,
    disableUploaderFilter: filterState.disableUploaderFilter,
    disableTagFilter: filterState.disableTagFilter,
  })
  handleClose()
}
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="visible" class="filter-backdrop" @click="handleClose"></div>
    </Transition>

    <Transition name="slide">
      <div v-if="visible" class="filter-drawer">
        <div class="drawer-header">
          <button class="icon-btn" title="重置筛选" @click="handleReset">🔄</button>
          <h2 class="drawer-title">筛选</h2>
          <button class="icon-btn apply-btn" title="完成" @click="handleApply">✓</button>
        </div>

        <div class="drawer-body">
          <div class="category-grid">
            <button
              v-for="cat in categories"
              :key="cat.key"
              class="cat-chip"
              :class="{ disabled: !filterState.activeCategories.has(cat.key) }"
              :style="{
                backgroundColor: filterState.activeCategories.has(cat.key)
                  ? cat.color
                  : 'var(--app-border-2)',
                color: filterState.activeCategories.has(cat.key) ? '#ffffff' : 'var(--app-text-3)',
              }"
              @click="toggleCategory(cat.key)"
            >
              {{ cat.label }}
            </button>
          </div>

          <div class="form-group">
            <label class="input-label">
              关键词过滤队列
              <span class="tip-text">(Enter 压入，框为空按 Enter 清空)</span>
            </label>

            <div
              v-if="filterState.keywords && filterState.keywords.length > 0"
              class="keyword-chips-wrapper"
            >
              <span
                v-for="(kw, index) in filterState.keywords"
                :key="kw + index"
                class="filter-kw-chip"
                :class="{ 'is-negative': isNegativeItem(kw) }"
              >
                <span class="kw-text">{{ kw }}</span>
                <span class="remove-x" title="删除此关键词" @click.stop="removeKeyword(index)">
                  ✕
                </span>
              </span>
            </div>

            <input
              v-model="inputKeyword"
              type="text"
              class="dark-input"
              placeholder="输入关键词后按 Enter 压入队列，前缀「- 」表示排除..."
              @focus="inputFocused = true"
              @blur="inputFocused = false"
              @keydown.enter.prevent="handleKeydownEnter"
            />

            <!-- ─── Round3-任务5：tag 联想（正向 / 负向「- 」前缀）─── -->
            <div
              v-if="inputFocused && (suggestions.length > 0 || loading)"
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

          <div class="form-row">
            <span class="row-label">语言</span>
            <select v-model="filterState.language" class="dark-select">
              <option value="All">All (全部)</option>
              <option value="Chinese">Chinese (中文)</option>
              <option value="Japanese">Japanese (日文)</option>
              <option value="English">English (英文)</option>
            </select>
          </div>

          <div class="form-row">
            <span class="row-label">仅搜索移除了的画廊</span>
            <label class="toggle-switch">
              <input v-model="filterState.onlyRemoved" type="checkbox" />
              <span class="slider"></span>
            </label>
          </div>

          <div class="form-row">
            <span class="row-label">只显示有种子的画廊</span>
            <label class="toggle-switch">
              <input v-model="filterState.onlyTorrents" type="checkbox" />
              <span class="slider"></span>
            </label>
          </div>

          <div class="form-row">
            <span class="row-label">页数范围</span>
            <div class="page-range-box">
              <input
                v-model.number="filterState.minPages"
                type="number"
                class="number-input"
                placeholder="Min"
              />
              <span class="range-text">到</span>
              <input
                v-model.number="filterState.maxPages"
                type="number"
                class="number-input"
                placeholder="Max"
              />
            </div>
          </div>

          <div class="form-row">
            <span class="row-label">最低评分</span>
            <select v-model="filterState.minRating" class="dark-select mini">
              <option :value="0">0 ⭐ (全部)</option>
              <option :value="1">1 ⭐</option>
              <option :value="2">2 ⭐</option>
              <option :value="3">3 ⭐</option>
              <option :value="4">4 ⭐</option>
              <option :value="5">5 ⭐</option>
            </select>
          </div>

          <div class="form-row">
            <span class="row-label">禁用语言过滤</span>
            <label class="toggle-switch">
              <input v-model="filterState.disableLangFilter" type="checkbox" />
              <span class="slider"></span>
            </label>
          </div>

          <div class="form-row">
            <span class="row-label">禁用上传者过滤</span>
            <label class="toggle-switch">
              <input v-model="filterState.disableUploaderFilter" type="checkbox" />
              <span class="slider"></span>
            </label>
          </div>

          <div class="form-row">
            <span class="row-label">禁用标签过滤</span>
            <label class="toggle-switch">
              <input v-model="filterState.disableTagFilter" type="checkbox" />
              <span class="slider"></span>
            </label>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* 遮罩背景 */
.filter-backdrop {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.6);
  z-index: 1999;
  backdrop-filter: blur(2px);
}

/* 抽屉主容器 */
.filter-drawer {
  position: fixed;
  top: 0;
  right: 0;
  width: 320px;
  height: 100vh;
  background-color: var(--app-bg-deep);
  border-left: 1px solid var(--app-border-2);
  z-index: 2000;
  display: flex;
  flex-direction: column;
  box-shadow: -10px 0 30px rgba(0, 0, 0, 0.8);
  color: var(--app-text-2);
}

/* 顶栏 Header */
.drawer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--app-border-2);
}

.drawer-title {
  font-size: 1.2rem;
  font-weight: bold;
  color: var(--app-text-strong);
  margin: 0;
}

.icon-btn {
  background: transparent;
  border: none;
  font-size: 1.2rem;
  color: var(--app-text-2);
  cursor: pointer;
  padding: 4px;
  transition: transform 0.2s;
}

.icon-btn:hover {
  color: var(--app-text-strong);
  transform: scale(1.1);
}
.apply-btn {
  font-weight: bold;
  color: #10b981;
}

/* 滚动主区 */
.drawer-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 分类网格 */
.category-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.cat-chip {
  padding: 10px 0;
  border-radius: 6px;
  border: none;
  font-size: 0.88rem;
  font-weight: bold;
  cursor: pointer;
  transition: all 0.2s ease;
  text-align: center;
}

.cat-chip.disabled {
  text-decoration: line-through;
  opacity: 0.5;
}

/* 表单组 */
.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.input-label {
  font-size: 0.85rem;
  color: var(--app-text-3);
}

.tip-text {
  font-size: 0.72rem;
  color: var(--app-text-muted);
  font-weight: normal;
  margin-left: 4px;
}

/* 多关键词队列气泡盒 */
.keyword-chips-wrapper {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 4px;
  padding: 6px;
  background-color: var(--app-surface-2);
  border-radius: 6px;
  border: 1px dashed var(--app-border-3);
}

.filter-kw-chip {
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

.filter-kw-chip.is-negative {
  background-color: rgba(255, 77, 109, 0.12);
  border-color: #ff4d6d;
  color: #ff4d6d;
  text-decoration: line-through;
}

.remove-x {
  color: var(--app-text-3);
  font-size: 0.75rem;
  cursor: pointer;
  transition: color 0.15s;
}

.remove-x:hover {
  color: var(--app-text-strong);
}

.dark-input {
  background-color: transparent;
  border: none;
  border-bottom: 1px solid var(--app-border-3);
  color: var(--app-text-strong);
  padding: 6px 0;
  font-size: 0.9rem;
  outline: none;
  transition: border-color 0.2s;
}

.dark-input:focus {
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

/* 表单行 */
.form-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.9rem;
  color: var(--app-text-2);
}

.dark-select {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
  border: 1px solid var(--app-border-3);
  padding: 6px 12px;
  border-radius: 6px;
  outline: none;
  font-size: 0.85rem;
}

.dark-select.mini {
  width: 80px;
}

.page-range-box {
  display: flex;
  align-items: center;
  gap: 6px;
}

.number-input {
  width: 54px;
  height: 32px;
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 4px;
  color: var(--app-text-strong);
  text-align: center;
  font-size: 0.85rem;
  outline: none;
}

.range-text {
  font-size: 0.8rem;
  color: var(--app-text-3);
}

/* 开关 Toggle Switch */
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
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
  border-radius: 24px;
  transition: 0.3s;
}

.slider:before {
  position: absolute;
  content: '';
  height: 18px;
  width: 18px;
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
  transform: translateX(20px);
  background-color: #7c4dff;
}

/* 动画过渡 */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.25s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.slide-enter-active,
.slide-leave-active {
  transition: transform 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.slide-enter-from,
.slide-leave-to {
  transform: translateX(100%);
}

/* 📱 移动形态（<1024px）：筛选抽屉占满宽度并适配底部安全区 */
@media (max-width: 1024px) {
  .filter-drawer {
    width: 100vw;
    max-width: 100vw;
    padding-bottom: var(--safe-bottom);
  }
}
</style>
