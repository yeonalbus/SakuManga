<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useModeStore } from '@/stores/modeStore'

const router = useRouter()
const modeStore = useModeStore()

// 使用全局模式状态（单一数据源）而非路由路径判断。
// 这样在 /settings、/downloads 等页面时，按钮会保持进入前的模式，
// 不会因路径不以 /online 开头而误判成「离线」。
const isOnline = computed(() => modeStore.isOnline)

const toggleMode = () => {
  // 当前是在线模式，点击切换到离线首页；反之切回在线首页
  if (isOnline.value) {
    router.push('/offline/home')
  } else {
    router.push('/online/home')
  }
}
</script>

<template>
  <button
    type="button"
    class="mode-toggle-btn"
    :title="isOnline ? '当前在线模式，点击切换为离线' : '当前离线模式，点击切换为在线'"
    @click="toggleMode"
  >
    <span class="icon">{{ isOnline ? '☀️' : '🌙' }}</span>
    <span class="label">{{ isOnline ? '在线' : '离线' }}</span>
  </button>
</template>

<style scoped>
.mode-toggle-btn {
  background-color: #242424;
  border: 1px solid #3a3a3a;
  color: #e0e0e0;
  padding: 4px 8px;
  border-radius: 12px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: all 0.2s ease;
  user-select: none;
}

.mode-toggle-btn:hover {
  background-color: #323232;
  border-color: #007acc;
}

.icon {
  font-size: 0.9rem;
}

.label {
  font-size: 0.75rem;
  color: #aaa;
}
</style>
