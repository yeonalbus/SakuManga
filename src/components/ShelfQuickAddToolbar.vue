<script setup lang="ts">
// src/components/ShelfQuickAddToolbar.vue
// Round22：离线多选工具条（已选 N 部 / 全选本页 / 加入书架 / 移出书架(可选) / 删除(可选) / 取消）
// 与 useShelfQuickAdd 配套；书架内页面通过 showRemove 追加「移出书架」动作。
defineProps<{
  count: number
  /** 显示「🗑️ 移出书架」（仅书架内部多选移除场景） */
  showRemove?: boolean
  /** 显示「🗑️ 删除」（管理员删除离线记录场景） */
  showDelete?: boolean
  /** 全选按钮禁用（无可选页项时） */
  selectAllDisabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'select-all'): void
  (e: 'add'): void
  (e: 'remove'): void
  (e: 'delete'): void
  (e: 'close'): void
}>()
</script>

<template>
  <div class="select-toolbar">
    <span class="select-count">已选 {{ count }} 部</span>
    <button class="toolbar-btn" :disabled="selectAllDisabled" @click="emit('select-all')">
      全选本页
    </button>
    <button class="toolbar-btn" :disabled="count === 0" @click="emit('add')">📥 加入书架</button>
    <button
      v-if="showRemove"
      class="toolbar-btn"
      :disabled="count === 0"
      @click="emit('remove')"
    >
      🗑️ 移出书架
    </button>
    <button
      v-if="showDelete"
      class="toolbar-btn danger"
      :disabled="count === 0"
      @click="emit('delete')"
    >
      🗑️ 删除
    </button>
    <button class="toolbar-btn" @click="emit('close')">取消</button>
  </div>
</template>

<style scoped>
.select-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-bottom: 12px;
  margin-bottom: 16px;
  border-bottom: 1px solid var(--app-border-2);
  position: sticky;
  top: 0;
  z-index: 10;
  background-color: var(--app-surface-2);
  flex-wrap: wrap;
}

.select-count {
  color: var(--app-text-strong);
  font-size: 0.95rem;
  font-weight: 500;
}

.toolbar-btn {
  background-color: var(--app-border-2);
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 6px 14px;
  font-size: 0.85rem;
  cursor: pointer;
  transition:
    background-color 0.2s,
    border-color 0.2s;
}

.toolbar-btn:hover:not(:disabled) {
  background-color: var(--app-surface-3-hover);
  border-color: var(--app-border-3);
}

.toolbar-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.toolbar-btn.danger {
  color: #ff7588;
  border-color: #ff7588;
}

.toolbar-btn.danger:hover:not(:disabled) {
  background-color: rgba(255, 117, 136, 0.12);
}
</style>
