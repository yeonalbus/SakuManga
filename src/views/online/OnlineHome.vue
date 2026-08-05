<script setup lang="ts">
import { watch, onMounted } from 'vue'
import GridContainer from '@/components/GridContainer.vue'
import OnlineLoadBar from '@/components/OnlineLoadBar.vue'
import FloatingToolbar from '@/components/FloatingToolbar.vue' // 👈 引入悬浮球
import { useOnlineStore } from '@/stores/onlineStore'
import { onlineSearchConfig } from '@/stores/searchStore'

const onlineStore = useOnlineStore()

const initSearch = () => {
  const cfg = onlineSearchConfig.value
  onlineStore.fetchInitial({
    keyword: cfg.keyword || '',
    categories: cfg.activeCategories,
  })
}

watch(
  onlineSearchConfig,
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
  <div class="online-home-view">
    <GridContainer :items="onlineStore.comics">
      <!-- 🟢 1. 顶部插槽：存在向上游标时，显示加载较新内容按钮 -->
      <template #header>
        <div v-if="onlineStore.prevGid" class="top-load-bar">
          <button
            class="pill-btn"
            :disabled="onlineStore.isLoading"
            @click="onlineStore.loadBefore"
          >
            ⬆️ {{ onlineStore.isLoading ? '加载中...' : '加载较新内容' }}
          </button>
        </div>
      </template>

      <!-- 2. 底部插槽：向下滑动流式加载 -->
      <template #footer>
        <OnlineLoadBar
          :is-loading="onlineStore.isLoading"
          :has-more="onlineStore.hasMore"
          :error="onlineStore.error"
          @load-more="onlineStore.loadMore"
        />
      </template>
    </GridContainer>

    <!-- 右下角悬浮操作球 -->
    <FloatingToolbar @refresh="initSearch" @seek-change="(date) => onlineStore.seekToDate(date)" />
  </div>
</template>

<style scoped>
.online-home-view {
  padding: 20px;
  min-height: 100%;
}

.top-load-bar {
  padding: 8px 0;
}

.pill-btn {
  background: transparent;
  color: #aaa;
  border: 1px solid #3a3a3a;
  border-radius: 20px;
  padding: 6px 18px;
  font-size: 0.82rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.pill-btn:hover:not(:disabled) {
  border-color: #00a896;
  color: #fff;
}

.pill-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
