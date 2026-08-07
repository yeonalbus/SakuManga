<script setup lang="ts">
import { ref, watch } from 'vue'

// 排行榜类型选择弹窗（优化排行榜功能）
// 4 种类型对应 e-hentai toplist.php 的 tl 参数：
//   11 = Galleries All-Time / 12 = Galleries Past Year / 13 = Galleries Past Month / 15 = Galleries Yesterday
// 选中 → emit('select', tl)；取消/遮罩/✕ → emit('close')
const props = defineProps<{
  show: boolean
  current?: string // 当前生效的 tl
}>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'select', tl: string): void
}>()

interface ToplistType {
  tl: string
  name: string
  label: string
}

const TYPES: ToplistType[] = [
  { tl: '15', name: 'yesterday', label: 'Galleries Yesterday' },
  { tl: '13', name: 'pastmonth', label: 'Galleries Past Month' },
  { tl: '12', name: 'pastyear', label: 'Galleries Past Year' },
  { tl: '11', name: 'alltime', label: 'Galleries All-Time' },
]

const selected = ref('')
watch(
  () => props.show,
  (v) => {
    if (v) {
      // 打开时默认选中当前生效的类型
      selected.value = props.current && TYPES.some((t) => t.tl === props.current) ? props.current : TYPES[0].tl
    }
  },
)

const confirm = () => {
  if (!selected.value) return
  emit('select', selected.value)
}
</script>

<template>
  <Teleport to="body">
    <Transition name="tm-fade">
      <div v-if="show" class="tm-overlay" @click.self="emit('close')">
        <div class="tm-panel">
          <div class="tm-header">
            <span class="tm-title">🏆 排行榜选择</span>
            <button class="tm-close" title="关闭" @click="emit('close')">✕</button>
          </div>

          <div class="tm-body">
            <div class="tm-list">
              <button
                v-for="t in TYPES"
                :key="t.tl"
                class="tm-item"
                :class="{ active: selected === t.tl }"
                @click="selected = t.tl"
              >
                <span class="tm-item-label">{{ t.label }}</span>
                <span class="tm-item-check">{{ selected === t.tl ? '✓' : '' }}</span>
              </button>
            </div>
            <p class="tm-hint">每页展示 50 个画廊，支持 1 ~ 200 页翻页</p>
          </div>

          <div class="tm-footer">
            <button class="tm-btn cancel" @click="emit('close')">取消</button>
            <button class="tm-btn ok" :disabled="!selected" @click="confirm">确定</button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.tm-overlay {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background-color: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}

.tm-panel {
  width: 380px;
  max-width: 100%;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 12px;
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.tm-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border-2);
}

.tm-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--app-text-strong);
}

.tm-close {
  background: transparent;
  border: none;
  color: var(--app-text-3);
  font-size: 15px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 6px;
}

.tm-close:hover {
  color: var(--app-text-strong);
  background-color: var(--app-surface-3);
}

.tm-body {
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.tm-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tm-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 12px;
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  color: var(--app-text-2);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s ease;
  text-align: left;
}

.tm-item:hover {
  border-color: #00a896;
  color: var(--app-text-strong);
}

.tm-item.active {
  border-color: #00a896;
  background-color: rgba(0, 168, 150, 0.12);
  color: var(--app-text-strong);
  font-weight: 600;
}

.tm-item-check {
  color: #00a896;
  font-weight: 700;
}

.tm-hint {
  font-size: 12px;
  color: var(--app-text-3);
}

.tm-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border-2);
}

.tm-btn {
  padding: 8px 20px;
  border: none;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.tm-btn.cancel {
  background-color: var(--app-surface-3);
  color: var(--app-text-2);
}

.tm-btn.cancel:hover {
  color: var(--app-text-strong);
}

.tm-btn.ok {
  background-color: #00a896;
  color: #ffffff;
}

.tm-btn.ok:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.tm-btn.ok:not(:disabled):hover {
  background-color: #00c4af;
}

.tm-fade-enter-active,
.tm-fade-leave-active {
  transition: opacity 0.2s ease;
}

.tm-fade-enter-from,
.tm-fade-leave-to {
  opacity: 0;
}

.tm-fade-enter-active .tm-panel,
.tm-fade-leave-active .tm-panel {
  transition: transform 0.2s ease;
}

.tm-fade-enter-from .tm-panel,
.tm-fade-leave-to .tm-panel {
  transform: translateY(12px) scale(0.96);
}
</style>
