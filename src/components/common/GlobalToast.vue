<script setup lang="ts">
import { useUI } from '@/composables/useUI'

const { toasts } = useUI()
</script>

<template>
  <div class="toast-container">
    <TransitionGroup name="toast-slide">
      <div v-for="item in toasts" :key="item.id" class="toast-item" :class="item.type">
        <span class="icon">
          <template v-if="item.type === 'success'">✓</template>
          <template v-else-if="item.type === 'error'">✕</template>
          <template v-else-if="item.type === 'warning'">!</template>
          <template v-else>ℹ</template>
        </span>
        <span class="message">{{ item.message }}</span>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-container {
  position: fixed;
  top: 24px;
  right: 24px;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: 10px;
  pointer-events: none;
}

.toast-item {
  pointer-events: auto;
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 220px;
  padding: 12px 18px;
  border-radius: 8px;
  background: var(--app-surface-3);
  color: var(--app-text-strong);
  font-size: 14px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  border: 1px solid var(--app-border-3);
}

.toast-item.info {
  border-left: 4px solid #007acc;
}
.toast-item.success {
  border-left: 4px solid #4caf50;
}
.toast-item.warning {
  border-left: 4px solid #ff9800;
}
.toast-item.error {
  border-left: 4px solid #f44336;
}

.icon {
  font-weight: bold;
  font-size: 14px;
}
.info .icon {
  color: #007acc;
}
.success .icon {
  color: #4caf50;
}
.warning .icon {
  color: #ff9800;
}
.error .icon {
  color: #f44336;
}

/* 动画效果 */
.toast-slide-enter-active,
.toast-slide-leave-active {
  transition: all 0.25s ease;
}
.toast-slide-enter-from {
  opacity: 0;
  transform: translateX(30px);
}
.toast-slide-leave-to {
  opacity: 0;
  transform: translateY(-20px);
}
</style>
