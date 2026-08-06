<script setup lang="ts">
import { computed } from 'vue'
import { onlineHistoryList, clearHistory } from '@/stores/historyStore'
import GridContainer from '@/components/GridContainer.vue'

// 动态提取在线浏览过的漫画列表
const comics = computed(() => onlineHistoryList.value.map((item) => item.comic))

const handleClear = () => {
  clearHistory('online')
}
</script>

<template>
  <div class="history-view">
    <div class="view-header">
      <h2 class="view-title">🌐 在线浏览历史 ({{ comics.length }})</h2>
      <button v-if="comics.length > 0" class="clear-btn" @click="handleClear">
        🗑️ 清空在线历史
      </button>
    </div>

    <GridContainer v-if="comics.length > 0" :items="comics" />
    <div v-else class="empty-tip">暂无在线浏览记录</div>
  </div>
</template>

<style scoped>
.history-view {
  padding: 20px;
  min-height: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.view-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
}

.view-title {
  font-size: 1.2rem;
  font-weight: 600;
  color: var(--app-text-strong);
  margin: 0;
}

.clear-btn {
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: #ef4444;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
}

.clear-btn:hover {
  background-color: rgba(239, 68, 68, 0.15);
  border-color: #ef4444;
}

.empty-tip {
  margin-top: 60px;
  text-align: center;
  color: var(--app-text-muted);
  font-size: 0.95rem;
}
</style>
