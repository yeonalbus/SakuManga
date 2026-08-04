<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

const props = withDefaults(
  defineProps<{
    isLoading?: boolean
    hasMore?: boolean
    error?: string | null
  }>(),
  {
    isLoading: false,
    hasMore: true,
    error: null,
  },
)

const emit = defineEmits<{
  (e: 'load-more'): void
}>()

const loadTriggerRef = ref<HTMLElement | null>(null)
let observer: IntersectionObserver | null = null

onMounted(() => {
  observer = new IntersectionObserver(
    (entries) => {
      const target = entries[0]
      if (target.isIntersecting && props.hasMore && !props.isLoading && !props.error) {
        emit('load-more')
      }
    },
    { rootMargin: '200px' },
  )

  if (loadTriggerRef.value) {
    observer.observe(loadTriggerRef.value)
  }
})

onUnmounted(() => {
  if (observer) observer.disconnect()
})
</script>

<template>
  <div ref="loadTriggerRef" class="online-load-bar">
    <div v-if="isLoading" class="status-item loading">
      <span class="spinner"></span>
      <span>正在加载更多内容...</span>
    </div>

    <div v-else-if="error" class="status-item error" @click="emit('load-more')">
      <span>加载失败: {{ error }}，点击重试</span>
    </div>

    <div v-else-if="!hasMore" class="status-item no-more">
      <span>已加载全部内容</span>
    </div>

    <button v-else class="pill-btn" @click="emit('load-more')">加载更多</button>
  </div>
</template>

<style scoped>
.online-load-bar {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 20px 0;
  min-height: 40px;
  width: 100%;
}

.status-item {
  font-size: 0.85rem;
  color: #888;
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-item.error {
  color: #ff5252;
  cursor: pointer;
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid #3a3a3a;
  border-top-color: #00a896;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.pill-btn {
  background: transparent;
  color: #aaa;
  border: 1px solid #3a3a3a;
  border-radius: 20px;
  padding: 4px 16px;
  font-size: 0.82rem;
  cursor: pointer;
}

.pill-btn:hover {
  border-color: #00a896;
  color: #fff;
}
</style>
