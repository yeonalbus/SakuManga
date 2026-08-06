<script setup lang="ts">
import { useUI } from '@/composables/useUI'

const { modalState, handleConfirm, handleCancel } = useUI()
</script>

<template>
  <Transition name="fade">
    <div v-if="modalState.isOpen" class="modal-mask" @click.self="handleCancel">
      <div class="modal-card">
        <h3 class="modal-title">{{ modalState.title }}</h3>
        <p class="modal-message">{{ modalState.message }}</p>

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
          <button class="btn btn-confirm" @click="handleConfirm">
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
  max-width: 400px;
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

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
