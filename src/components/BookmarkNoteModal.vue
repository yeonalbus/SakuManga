<script setup lang="ts">
// Round49：书签备注编辑弹窗
//
// 规则（与名片显示区一致）：
//   - 总宽度 ≤ 110 半角单位（= 55 个中文字符，1 中文 = 2 半角）
//   - 最多 3 行；每行 ≤ 19 个中文字符（38 半角单位）
// 超限：计数实时标红；点「确定」时显示错误原因并拒绝保存（绝不静默截断）。
import { computed, ref, watch } from 'vue'
import AppIcon from '@/components/icons/AppIcon.vue'
import { noteCounterText, validateNote } from '@/utils/noteLimit'

const props = defineProps<{
  show: boolean
  /** 打开时的备注内容 */
  value: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save', note: string): void
}>()

const text = ref('')
/** 点过「确定」后才展示错误提示（避免一打开就标红输入框） */
const showError = ref(false)

const validation = computed(() => validateNote(text.value))
const counter = computed(() => noteCounterText(validation.value))
const overLimit = computed(() => !validation.value.ok)

watch(
  () => props.show,
  (v) => {
    if (v) {
      text.value = props.value || ''
      showError.value = false
    }
  },
)

const handleConfirm = () => {
  if (!validation.value.ok) {
    showError.value = true
    return
  }
  emit('save', text.value.trim())
}
</script>

<template>
  <Teleport to="body">
    <Transition name="bn-fade">
      <div v-if="show" class="bn-overlay" @click.self="emit('close')">
        <div class="bn-panel">
          <div class="bn-header">
            <span class="bn-title">编辑备注</span>
            <button class="bn-close" title="关闭" @click="emit('close')">
              <AppIcon name="close" :size="14" />
            </button>
          </div>

          <div class="bn-body">
            <div class="bn-label-row">
              <span class="bn-label">备注（上限 110 个半角字符 · 约 55 个中文字符 · 3 行）</span>
              <span class="bn-counter" :class="{ over: overLimit }">{{ counter }}</span>
            </div>
            <textarea
              v-model="text"
              class="bn-textarea"
              :class="{ invalid: overLimit && showError }"
              rows="3"
              placeholder="例如：2026-10 已筛完这批"
              @input="showError = false"
            />
            <span v-if="showError && !validation.ok" class="bn-error">
              {{ validation.error }}（未保存）
            </span>
          </div>

          <div class="bn-footer">
            <button class="bn-btn primary" @click="handleConfirm">确定</button>
            <button class="bn-btn ghost" @click="emit('close')">取消</button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* 与 BookmarkCreateModal 同一套暗色弹窗骨架 */
.bn-overlay {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background-color: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}

.bn-panel {
  width: 100%;
  max-width: 420px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 12px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.55);
  overflow: hidden;
}

.bn-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--app-border-2);
}

.bn-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--app-text-strong);
}

.bn-close {
  background: transparent;
  border: none;
  color: var(--app-text-muted);
  cursor: pointer;
  font-size: 0.85rem;
  padding: 4px;
  border-radius: 6px;
}
.bn-close:hover {
  color: var(--app-text-strong);
  background-color: var(--app-surface-3);
}

.bn-body {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 16px;
}

.bn-label-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}

.bn-label {
  font-size: 0.75rem;
  color: var(--app-text-3);
}

/* 计数：正常灰；超限红 */
.bn-counter {
  flex-shrink: 0;
  font-size: 0.75rem;
  font-variant-numeric: tabular-nums;
  color: var(--app-text-muted);
  transition: color 0.15s ease;
}
.bn-counter.over {
  color: #ef4444;
  font-weight: 600;
}

/* 3 行矩形输入框 */
.bn-textarea {
  box-sizing: border-box;
  width: 100%;
  min-height: 84px;
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  color: var(--app-text-strong);
  font-size: 0.88rem;
  line-height: 1.5;
  padding: 8px 10px;
  outline: none;
  resize: vertical;
  transition: border-color 0.15s ease;
}
.bn-textarea:focus {
  border-color: #00a896;
}
.bn-textarea.invalid {
  border-color: #ef4444;
}

.bn-error {
  font-size: 0.72rem;
  color: #ef4444;
}

.bn-footer {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border-2);
}

.bn-btn {
  flex: 1;
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.bn-btn:hover {
  background-color: var(--app-surface-3-hover);
  color: var(--app-text-strong);
}
.bn-btn.primary {
  background-color: #00a896;
  border-color: #00a896;
  color: #ffffff;
}
.bn-btn.primary:hover {
  background-color: #00c4af;
  color: #ffffff;
}
.bn-btn.ghost {
  background: transparent;
  border-color: transparent;
}

.bn-fade-enter-active,
.bn-fade-leave-active {
  transition: opacity 0.18s ease;
}
.bn-fade-enter-from,
.bn-fade-leave-to {
  opacity: 0;
}
</style>
