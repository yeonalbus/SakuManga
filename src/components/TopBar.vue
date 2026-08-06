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
      <span>⚙️ 筛选</span>
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
  background-color: #1a1a1a;
  border-bottom: 1px solid #2a2a2a;
  display: flex;
  align-items: center;
  padding: 0 16px;
  gap: 12px; /* 内部零件之间的间距 */
}

/* 筛选按钮样式 */
.filter-trigger-btn {
  background-color: #242428;
  border: 1px solid #3a3a3d;
  color: #ccc;
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
  background-color: #2e2e33;
  border-color: #007acc;
  color: #fff;
}

/* 搜索栏外壳，自动吸收剩余宽度 */
.search-wrapper {
  flex: 1;
  display: flex;
  justify-content: center;
}

/* 📱 窄屏适配：顶栏给汉堡按钮让位，并适配 iOS 安全区 */
@media (max-width: 767px) {
  .top-bar {
    height: calc(56px + var(--safe-top));
    padding-top: var(--safe-top);
    padding-left: calc(56px + var(--safe-left));
    padding-right: calc(12px + var(--safe-right));
    gap: 6px;
  }
}
</style>
