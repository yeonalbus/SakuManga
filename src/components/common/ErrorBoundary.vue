<script setup lang="ts">
import { ref, onErrorCaptured } from 'vue'
import { reportError } from '@/utils/errorReporter'

// 错误边界（问题8）：捕获子树内渲染/生命周期错误，避免单个组件报错
// 导致整个 Vue 树卸载白屏（如搜索栏联想渲染时 tag 数据异常）。
// onErrorCaptured 返回 false 阻止错误继续冒泡到根实例，显示降级 UI。
const hasError = ref(false)
const errorMessage = ref('')
const errorStack = ref('')

onErrorCaptured((err, _instance, info) => {
  errorMessage.value = err instanceof Error ? err.message : String(err)
  errorStack.value = err instanceof Error ? err.stack || '' : ''
  hasError.value = true
  reportError(
    'error',
    err instanceof Error ? err.message : err,
    err instanceof Error ? err.stack : undefined,
    `ErrorBoundary:${info}`,
  )
  return false
})

const reload = () => {
  window.location.reload()
}
</script>

<template>
  <div v-if="hasError" class="error-boundary">
    <div class="error-box">
      <span class="error-icon">⚠️</span>
      <span class="error-text">该区域渲染出错，错误已自动记录日志。可点击下方按钮刷新页面。</span>
      <span v-if="errorMessage" class="error-detail">{{ errorMessage }}</span>
      <button class="reload-btn" @click="reload">刷新页面</button>
    </div>
  </div>
  <slot v-else />
</template>

<style scoped>
.error-boundary {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100%;
  padding: 24px;
}
.error-box {
  max-width: 480px;
  padding: 20px 24px;
  border: 1px solid var(--app-border, #2a2a2a);
  border-radius: 8px;
  background: var(--app-surface, #1a1a1a);
  color: var(--app-fg, #e0e0e0);
  display: flex;
  flex-direction: column;
  gap: 10px;
  font-size: 0.85rem;
}
.error-icon {
  font-size: 1.4rem;
}
.error-text {
  line-height: 1.5;
}
.error-detail {
  font-family: monospace;
  font-size: 0.75rem;
  color: #ff8a80;
  word-break: break-all;
  max-height: 120px;
  overflow: auto;
}
.reload-btn {
  align-self: flex-start;
  padding: 6px 14px;
  border: none;
  border-radius: 6px;
  background: var(--app-accent, #007acc);
  color: #fff;
  cursor: pointer;
}
.reload-btn:hover {
  opacity: 0.85;
}
</style>
