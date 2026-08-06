<script setup lang="ts">
import { ref, computed } from 'vue'
import RandomPicker from './RandomPicker.vue'
import FilterDrawer from './FilterDrawer.vue'
import SearchBar from './SearchBar.vue'
import ReadingList from './ReadingList.vue'
import { useUI } from '@/composables/useUI'
import { useModeStore } from '@/stores/modeStore'
// 按领域引入搜索配置，避免依赖 appStore 聚合层
import { onlineSearchConfig, offlineSearchConfig } from '@/stores/searchStore'
import type { FilterParams } from '@/types/comic'

const { toast } = useUI()
const modeStore = useModeStore()
const isFilterOpen = ref(false)

// 🟢 1. 使用全局模式状态感知当前是在线还是离线模块
//（/settings、/downloads 等页面保持进入前的模式，与侧边栏 / ModeToggle 保持一致）
const currentScope = computed<'online' | 'offline'>(() => modeStore.currentMode)

// 🟢 2. 拿到当前生效的 SearchConfig 对象
const activeSearchConfig = computed(() => {
  return currentScope.value === 'offline' ? offlineSearchConfig.value : onlineSearchConfig.value
})

// 🟢 3. 保存筛选设置到对应的域中，不互相串味
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
  <header class="top-bar">
    <RandomPicker />

    <button class="filter-trigger-btn" @click="isFilterOpen = true">
      <span class="filter-icon">⚙️</span>
      <span class="filter-label">筛选</span>
    </button>

    <FilterDrawer
      v-model:visible="isFilterOpen"
      :config="activeSearchConfig"
      @apply="handleApplyFilters"
    />

    <div class="search-wrapper">
      <SearchBar />
    </div>

    <ReadingList />
  </header>
</template>

<style scoped>
.top-bar {
  height: 56px;
  background-color: var(--app-surface);
  border-bottom: 1px solid var(--app-border);
  display: flex;
  align-items: center;
  padding: 0 16px;
  gap: 12px; /* 内部零件之间的间距 */
}

/* 筛选按钮样式 */
.filter-trigger-btn {
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 6px 12px;
  border-radius: 16px;
  font-size: 0.85rem;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 4px;
  transition: all 0.2s;
  flex-shrink: 0;
}

.filter-trigger-btn:hover {
  background-color: var(--app-surface-3-hover);
  border-color: var(--app-accent);
  color: var(--app-text-strong);
}

/* 搜索栏外壳，自动吸收剩余宽度 */
.search-wrapper {
  flex: 1;
  display: flex;
  justify-content: center;
}

/* 📱 移动形态（<1024px）：顶栏改两行布局（首行工具按钮、第二行搜索栏占满），并适配 iOS 安全区 */
@media (max-width: 1024px) {
  .top-bar {
    height: auto;
    min-height: 48px;
    flex-wrap: wrap;
    padding-top: calc(4px + var(--safe-top));
    padding-right: calc(12px + var(--safe-right));
    padding-bottom: 4px;
    gap: 4px 8px;
  }
  /* 搜索栏独占第二行、占满宽度，避免与抽卡/筛选按钮挤成一团 */
  .search-wrapper {
    flex: 1 0 100%;
  }
  /* 移动端精简：筛选只显示图标 */
  .filter-label {
    display: none;
  }
}

/* 🖥️ 移动形态：给汉堡按钮让位（仅移动布局；桌面形态侧栏常驻、无汉堡，无需让位） */
/* ⚠️ :global() 必须包裹完整选择器（含子类名），否则 scoped 编译会丢弃类名、规则直接作用在 <html> 上（曾导致整个页面隐藏/平移出视口白屏） */
:global(html[data-layout='mobile'] .top-bar) {
  padding-left: calc(56px + var(--safe-left));
  /* 搜索栏随滚动显隐：悬浮覆盖在内容上方（配合 App.vue 的 main-content 顶部补偿） */
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 50;
  transition: transform 0.25s ease;
}
:global(html[data-layout='mobile'] .filter-label) {
  display: none;
}
/* 滚动隐藏：仅移动形态下，向下滚动收起顶栏（App.vue 在 main-content 滚动时切换 html.topbar-hidden） */
:global(html[data-layout='mobile'].topbar-hidden .top-bar) {
  transform: translateY(-100%);
}
</style>
