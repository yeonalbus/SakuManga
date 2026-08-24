<script setup lang="ts">
// src/components/SortRowMenu.vue
// Round22：排序行操作菜单（⋯）——「移到顶部/置顶」+「移动到第 X 位」（决策 D6：书架列表「移到顶部」，书架内本子「置顶」）
import { ref } from 'vue'
import { useUI } from '@/composables/useUI'

const props = withDefaults(
  defineProps<{
    /** 列表长度（移动到第 X 位的上限） */
    total: number
    /** 「移到顶部/置顶」按钮文案（决策 D6：书架列表用「移到顶部」，书架内本子用「置顶」） */
    topLabel?: string
  }>(),
  { topLabel: '移到顶部' },
)

const emit = defineEmits<{
  (e: 'move-top'): void
  (e: 'move-to', position: number): void
}>()

const { modal, toast } = useUI()
const open = ref(false)

const toggle = (e: MouseEvent) => {
  e.stopPropagation()
  e.preventDefault()
  open.value = !open.value
}

const close = () => {
  open.value = false
}

const handleMoveTop = (e: MouseEvent) => {
  e.stopPropagation()
  e.preventDefault()
  open.value = false
  emit('move-top')
}

const handleMoveTo = async (e: MouseEvent) => {
  e.stopPropagation()
  e.preventDefault()
  open.value = false
  const pos = await modal.prompt(`请输入目标位置（1 ~ ${props.total}）：`, '', '移动到第 X 位')
  if (!pos || !pos.trim()) return
  const n = Math.floor(Number(pos))
  if (!Number.isFinite(n) || n < 1 || n > props.total) {
    toast.warning(`请输入 1 ~ ${props.total} 之间的数字`)
    return
  }
  emit('move-to', n)
}
</script>

<template>
  <div class="sort-row-menu" :class="{ open }">
    <button class="menu-trigger" title="排序操作" @click="toggle" @pointerdown.stop>⋯</button>
    <div v-if="open" class="menu-pop" @pointerdown.stop @click.stop>
      <button class="menu-item" @click="handleMoveTop">📌 {{ topLabel }}</button>
      <button class="menu-item" @click="handleMoveTo">🎯 移动到第 {{ total }} 位以内…</button>
    </div>
    <!-- 点击外部关闭 -->
    <div v-if="open" class="menu-backdrop" @click="close" />
  </div>
</template>

<style scoped>
.sort-row-menu {
  position: relative;
  flex-shrink: 0;
}

.menu-trigger {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--app-text-3);
  font-size: 0.9rem;
  font-weight: 700;
  cursor: pointer;
  line-height: 1;
  transition:
    background-color 0.15s,
    color 0.15s;
}

.menu-trigger:hover,
.sort-row-menu.open .menu-trigger {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

.menu-pop {
  position: absolute;
  right: 0;
  top: calc(100% + 4px);
  z-index: 60;
  min-width: 150px;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  padding: 4px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.menu-item {
  background: transparent;
  border: none;
  color: var(--app-text-2);
  font-size: 0.82rem;
  text-align: left;
  padding: 7px 10px;
  border-radius: 6px;
  cursor: pointer;
  white-space: nowrap;
  transition:
    background-color 0.15s,
    color 0.15s;
}

.menu-item:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

.menu-backdrop {
  position: fixed;
  inset: 0;
  z-index: 50;
  background: transparent;
}
</style>
