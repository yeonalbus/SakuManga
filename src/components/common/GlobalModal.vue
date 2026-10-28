<script setup lang="ts">
import { useUI } from '@/composables/useUI'
// Round14：手柄确认/取消（A=确认 B=取消，可自定义按键防与翻页键冲突）
import { useGamepad } from '@/composables/useGamepad'

const { modalState, handleConfirm, handleCancel } = useUI()

// 仅在 modal 打开期间把确认/取消按键接到手柄；同时放行 Enter 确认（已有输入框行为）
useGamepad({
  onConfirm: () => {
    if (modalState.isOpen) handleConfirm()
  },
  onCancel: () => {
    if (modalState.isOpen && modalState.mode !== 'alert') handleCancel()
  },
})
</script>

<template>
  <Transition name="fade">
    <div v-if="modalState.isOpen" class="modal-mask" @click.self="handleCancel">
      <div class="modal-card">
        <h3 class="modal-title">{{ modalState.title }}</h3>
        <p class="modal-message">{{ modalState.message }}</p>

        <div v-if="modalState.details" class="modal-details" :class="modalState.detailsTone">
          <span class="details-icon">⚠</span>
          <span>{{ modalState.details }}</span>
        </div>

        <div v-if="modalState.mode === 'prompt'" class="modal-input-wrapper">
          <input
            v-model="modalState.inputValue"
            type="text"
            class="modal-input"
            placeholder="请输入..."
            @keyup.enter="handleConfirm"
            autofocus
          />
        </div>

        <div class="modal-actions">
          <button v-if="modalState.mode !== 'alert'" class="btn btn-cancel" @click="handleCancel">
            {{ modalState.cancelText }}
          </button>
          <button class="btn btn-confirm" :class="modalState.tone" @click="handleConfirm">
            {{ modalState.confirmText }}
          </button>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.modal-mask {
  position: fixed;
  inset: 0;
  z-index: 9998;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
}

.modal-card {
  width: 90%;
  max-width: 430px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  border-radius: 12px;
  padding: 20px 24px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5);
}

.modal-title {
  margin: 0 0 10px 0;
  font-size: 18px;
  color: var(--app-text-strong);
}

.modal-message {
  margin: 0 0 16px 0;
  font-size: 14px;
  color: var(--app-text-2);
  line-height: 1.5;
}

/* Round47：确认弹窗的附加警告（损坏压缩包 / GP 重复消耗） */
.modal-details {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: -4px 0 16px;
  padding: 9px 11px;
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--app-text-2);
}

.modal-details.danger {
  border: 1px solid rgba(230, 57, 70, 0.32);
  background: rgba(230, 57, 70, 0.1);
}

.modal-details.warn {
  border: 1px solid rgba(230, 162, 60, 0.34);
  background: rgba(230, 162, 60, 0.1);
}

.modal-details .details-icon {
  flex: none;
  line-height: 1.5;
}

.modal-details.danger .details-icon {
  color: #ff6b6b;
}

.modal-details.warn .details-icon {
  color: #e6a23c;
}

.modal-input-wrapper {
  margin-bottom: 20px;
}

.modal-input {
  width: 100%;
  padding: 10px 12px;
  background: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  color: var(--app-text-strong);
  outline: none;
  font-size: 14px;
  box-sizing: border-box;
}

.modal-input:focus {
  border-color: #007acc;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

.btn {
  padding: 8px 16px;
  border-radius: 6px;
  border: none;
  font-size: 14px;
  cursor: pointer;
  transition: opacity 0.2s;
}

.btn:hover {
  opacity: 0.85;
}
.btn-cancel {
  background: var(--app-surface-3);
  color: var(--app-text-2);
}
.btn-confirm {
  background: #007acc;
  color: #fff;
}

.btn-confirm.danger {
  background: #c62828;
}

.btn-confirm.warn {
  background: #e6a23c;
  color: #231604;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
