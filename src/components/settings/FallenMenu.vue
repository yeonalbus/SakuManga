<script setup lang="ts">
/**
 * Round54：阵亡名单面板（全局单一名单）。
 * 分组显示：EH 版权/下架、无法恢复（解锁失败 / 非 E 站 / 路径已移除）。
 * x 按钮 = 用户手动标记「已恢复」：划线 + 置底（保留显示），可撤销。
 */
import { computed } from 'vue'
import type { FallenComic } from '@/stores/scanPathStore'

const props = defineProps<{
  open: boolean
  items: FallenComic[]
  loading: boolean
}>()

const emit = defineEmits<{
  (e: 'dismiss', id: number, dismissed: boolean): void
  (e: 'reclassify'): void
  (e: 'close'): void
}>()

// 组内排序：未 dismissed 在前，dismissed 置底
const sortGroup = (arr: FallenComic[]) => [
  ...arr.filter((f) => !f.dismissed),
  ...arr.filter((f) => f.dismissed),
]

const groups = computed(() => {
  const removed = props.items.filter((f) => f.reason === 'removed')
  const others = props.items.filter((f) => f.reason !== 'removed')
  return [
    { key: 'removed', label: 'EH 版权 / 下架', items: sortGroup(removed) },
    { key: 'others', label: '无法恢复（解锁失败 / 非 E 站 / 路径已移除）', items: sortGroup(others) },
  ].filter((g) => g.items.length > 0)
})
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="mask" @click.self="emit('close')">
      <div class="panel">
        <div class="panel-head">
          <span class="panel-title">阵亡名单（{{ items.length }}）</span>
          <div class="head-actions">
            <!-- 存量异常任务（旧逻辑失败/用户取消）一键转入阵亡 -->
            <button
              class="reclassify-btn"
              title="把已取消/出错的恢复任务中「404/下架/解压失败」的条目直接转入阵亡（不会重新下载）"
              @click="emit('reclassify')"
            >
              异常任务转入阵亡
            </button>
            <button class="close-btn" title="关闭" @click="emit('close')">×</button>
          </div>
        </div>

        <div class="panel-body">
          <div v-if="loading" class="empty">加载中…</div>
          <template v-else>
            <div v-for="group in groups" :key="group.key" class="group">
              <div class="group-label">{{ group.label }}（{{ group.items.length }}）</div>
              <div
                v-for="f in group.items"
                :key="f.id"
                class="fallen-item"
                :class="{ dismissed: f.dismissed }"
              >
                <div class="item-main">
                  <div class="item-title" :title="f.title">{{ f.title }}</div>
                  <div class="item-meta">
                    <span v-if="f.sourcePath" class="item-path">{{ f.sourcePath }}</span>
                    <span v-if="f.gid">gid={{ f.gid }}</span>
                  </div>
                </div>
                <button
                  class="dismiss-btn"
                  :title="f.dismissed ? '撤销划线' : '标记已恢复（划线并置底）'"
                  @click="emit('dismiss', f.id, !f.dismissed)"
                >
                  ×
                </button>
              </div>
            </div>
            <div v-if="groups.length === 0" class="empty">暂无阵亡记录</div>
          </template>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.mask {
  position: fixed;
  inset: 0;
  z-index: 1000;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
}

.panel {
  width: min(720px, calc(100vw - 32px));
  max-height: min(600px, calc(100vh - 64px));
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 10px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border-2);
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-strong);
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.reclassify-btn {
  padding: 5px 10px;
  border-radius: 6px;
  border: 1px solid var(--app-border-3);
  background: var(--app-input-bg);
  color: var(--app-text-2);
  font-size: 12px;
  cursor: pointer;
}

.reclassify-btn:hover {
  border-color: #f0b429;
  color: #f0b429;
}

.close-btn {
  width: 26px;
  height: 26px;
  border-radius: 6px;
  border: none;
  background: transparent;
  color: var(--app-text-2);
  font-size: 18px;
  cursor: pointer;
}

.close-btn:hover {
  background: var(--app-input-bg);
  color: var(--app-text-strong);
}

.panel-body {
  flex: 1;
  overflow-y: auto;
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.group-label {
  font-size: 12px;
  color: var(--app-text-3, var(--app-text-2));
  opacity: 0.8;
  padding: 2px 0;
}

.fallen-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
}

.fallen-item.dismissed {
  opacity: 0.55;
}

.fallen-item.dismissed .item-title {
  text-decoration: line-through;
}

.item-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item-title {
  font-size: 13px;
  color: var(--app-text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.item-meta {
  display: flex;
  gap: 10px;
  font-size: 11px;
  color: var(--app-text-2);
  min-width: 0;
}

.item-path {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dismiss-btn {
  flex-shrink: 0;
  width: 26px;
  height: 26px;
  border-radius: 6px;
  border: 1px solid var(--app-border-3);
  background: var(--app-input-bg);
  color: var(--app-text-2);
  font-size: 16px;
  line-height: 1;
  cursor: pointer;
}

.dismiss-btn:hover {
  border-color: #ff7588;
  color: #ff7588;
}

.empty {
  padding: 24px 0;
  text-align: center;
  font-size: 13px;
  color: var(--app-text-2);
}
</style>
