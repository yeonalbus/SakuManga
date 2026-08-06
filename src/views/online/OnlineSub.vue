<script setup lang="ts">
import { watch, onMounted } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import OnlineLoadBar from '@/components/OnlineLoadBar.vue'
import FloatingToolbar from '@/components/FloatingToolbar.vue'
import { useSubStore } from '@/stores/subStore' // 🟢 对应订阅专用的 Pinia Store
import { subSearchConfig } from '@/stores/searchStore' // 🟢 对应订阅专用的搜索/分类配置

const subStore = useSubStore()

// 初始化/重新加载订阅数据
const initSearch = () => {
  const cfg = subSearchConfig.value
  subStore.fetchInitial({
    keyword: cfg.keyword || '',
    categories: cfg.activeCategories,
  })
}

// 监听订阅检索配置变更（如搜索框输入、分类勾选）
watch(
  subSearchConfig,
  () => {
    initSearch()
  },
  { deep: true },
)

onMounted(() => {
  initSearch()
})
</script>

<template>
  <div class="page-wrapper">
    <GridContainer :items="subStore.comics">
      <!-- 1. 顶部插槽：存在向上游标时，显示加载较新内容按钮 -->
      <template #header>
        <div v-if="subStore.prevGid" class="top-load-bar">
          <button class="pill-btn" :disabled="subStore.isLoading" @click="subStore.loadBefore">
            ⬆️ {{ subStore.isLoading ? '加载中...' : '加载较新内容' }}
          </button>
        </div>
      </template>

      <!-- 2. 底部插槽：向下滑动流式加载 -->
      <template #footer>
        <OnlineLoadBar
          :is-loading="subStore.isLoading"
          :has-more="subStore.hasMore"
          :error="subStore.error"
          @load-more="subStore.loadMore"
        />
      </template>
    </GridContainer>

    <!-- 右下角悬浮操作球：支持手动刷新与按日期跳转 (seek) -->
    <FloatingToolbar @refresh="initSearch" @seek-change="(date) => subStore.seekToDate(date)" />
  </div>
</template>

<style scoped>
.page-wrapper {
  padding: 12px 4px;
  min-height: 100%;
}

.top-load-bar {
  padding: 8px 0;
  text-align: center;
}

.pill-btn {
  background: transparent;
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
  border-radius: 20px;
  padding: 6px 18px;
  font-size: 0.82rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.pill-btn:hover:not(:disabled) {
  border-color: #00a896;
  color: var(--app-text-strong);
}

.pill-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
