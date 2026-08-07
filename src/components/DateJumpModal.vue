<script setup lang="ts">
import { ref, computed, watch } from 'vue'

// 通用日期跳转弹窗（Round4 任务八）
// 两个 Tab：选择节点（8 个相对今天预设） / 选择日期（<input type="date">）
// 确定 → emit('confirm', 'YYYY-MM-DD')；取消/遮罩/✕ → emit('close')
const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm', date: string): void
}>()

type Tab = 'node' | 'custom'
const activeTab = ref<Tab>('node')
const selectedNodeDate = ref('')
const customDate = ref('')

// 8 个预设节点（相对今天的偏移）
interface Preset {
  label: string
  days?: number
  months?: number
  years?: number
}
const PRESETS: Preset[] = [
  { label: '昨天', days: 1 },
  { label: '三天前', days: 3 },
  { label: '一周前', days: 7 },
  { label: '两周前', days: 14 },
  { label: '一个月前', months: 1 },
  { label: '半年前', months: 6 },
  { label: '一年前', years: 1 },
  { label: '两年前', years: 2 },
]

const toDateStr = (d: Date) => {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

const pickNode = (p: Preset) => {
  const now = new Date()
  const d = new Date(now)
  if (p.days) d.setDate(now.getDate() - p.days)
  if (p.months) d.setMonth(now.getMonth() - p.months)
  if (p.years) d.setFullYear(now.getFullYear() - p.years)
  selectedNodeDate.value = toDateStr(d)
}

// 当前生效的日期（决定确定按钮可用性与提交值）
const effectiveDate = computed(() =>
  activeTab.value === 'node' ? selectedNodeDate.value : customDate.value,
)

const confirm = () => {
  if (!effectiveDate.value) return
  emit('confirm', effectiveDate.value)
}

// 每次打开时重置状态，并默认选中今天，保证开箱即用
watch(
  () => props.show,
  (v) => {
    if (v) {
      activeTab.value = 'node'
      customDate.value = ''
      selectedNodeDate.value = toDateStr(new Date())
    }
  },
)
</script>

<template>
  <Teleport to="body">
    <Transition name="dm-fade">
      <div v-if="show" class="dm-overlay" @click.self="emit('close')">
        <div class="dm-panel">
          <div class="dm-header">
            <span class="dm-title">📅 日期跳转</span>
            <button class="dm-close" title="关闭" @click="emit('close')">✕</button>
          </div>

          <div class="dm-tabs">
            <button
              class="dm-tab"
              :class="{ active: activeTab === 'node' }"
              @click="activeTab = 'node'"
            >
              选择节点
            </button>
            <button
              class="dm-tab"
              :class="{ active: activeTab === 'custom' }"
              @click="activeTab = 'custom'"
            >
              选择日期
            </button>
          </div>

          <div class="dm-body">
            <template v-if="activeTab === 'node'">
              <div class="preset-grid">
                <button
                  v-for="p in PRESETS"
                  :key="p.label"
                  class="preset-btn"
                  @click="pickNode(p)"
                >
                  {{ p.label }}
                </button>
              </div>
              <div class="preset-preview">
                将跳转至：<b>{{ selectedNodeDate || '—' }}</b>
              </div>
            </template>
            <template v-else>
              <input
                type="date"
                class="dm-date-input"
                min="2007-01-01"
                v-model="customDate"
              />
            </template>
          </div>

          <div class="dm-footer">
            <button class="dm-btn cancel" @click="emit('close')">取消</button>
            <button class="dm-btn ok" :disabled="!effectiveDate" @click="confirm">确定</button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.dm-overlay {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background-color: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}

.dm-panel {
  width: 360px;
  max-width: 100%;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 12px;
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.dm-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border-2);
}

.dm-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--app-text-strong);
}

.dm-close {
  background: transparent;
  border: none;
  color: var(--app-text-3);
  font-size: 15px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 6px;
}

.dm-close:hover {
  color: var(--app-text-strong);
  background-color: var(--app-surface-3);
}

.dm-tabs {
  display: flex;
  gap: 4px;
  padding: 10px 12px 0;
}

.dm-tab {
  flex: 1;
  padding: 8px 0;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--app-text-3);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.dm-tab.active {
  color: var(--app-text-strong);
  border-bottom-color: #00a896;
  font-weight: 600;
}

.dm-body {
  padding: 14px 16px;
  min-height: 96px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.preset-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
}

.preset-btn {
  padding: 9px 4px;
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  color: var(--app-text-2);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.preset-btn:hover {
  border-color: #00a896;
  color: var(--app-text-strong);
}

.preset-preview {
  font-size: 12px;
  color: var(--app-text-3);
}

.preset-preview b {
  color: #00a896;
  font-family: monospace;
}

.dm-date-input {
  width: 100%;
  padding: 10px 12px;
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  color: var(--app-text-strong);
  font-size: 14px;
  outline: none;
  color-scheme: dark;
}

.dm-date-input:focus {
  border-color: #00a896;
}

.dm-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border-2);
}

.dm-btn {
  padding: 8px 20px;
  border: none;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.dm-btn.cancel {
  background-color: var(--app-surface-3);
  color: var(--app-text-2);
}

.dm-btn.cancel:hover {
  color: var(--app-text-strong);
}

.dm-btn.ok {
  background-color: #00a896;
  color: #ffffff;
}

.dm-btn.ok:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.dm-btn.ok:not(:disabled):hover {
  background-color: #00c4af;
}

.dm-fade-enter-active,
.dm-fade-leave-active {
  transition: opacity 0.2s ease;
}

.dm-fade-enter-from,
.dm-fade-leave-to {
  opacity: 0;
}

.dm-fade-enter-active .dm-panel,
.dm-fade-leave-active .dm-panel {
  transition: transform 0.2s ease;
}

.dm-fade-enter-from .dm-panel,
.dm-fade-leave-to .dm-panel {
  transform: translateY(12px) scale(0.96);
}
</style>
