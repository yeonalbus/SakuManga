<script setup lang="ts">
import { useRouter } from 'vue-router'
import OnlineDetail from '@/views/online/OnlineDetail.vue'

const props = defineProps<{
  open: boolean
  gid: string
  token: string
}>()

defineEmits<{
  (e: 'close'): void
}>()

const router = useRouter()

/** 左上角标题作为触发键：点击在新浏览器标签打开完整详情（等价中键 / Ctrl / Meta + 点击） */
const openFullDetail = () => {
  if (!props.open || !props.gid) return
  const href = router.resolve({
    path: '/online/detail',
    query: { id: props.gid, token: props.token },
  }).href
  window.open(href, '_blank')
}
</script>

<template>
  <aside v-show="open" class="detail-panel">
    <header class="detail-panel-header">
      <button
        class="detail-panel-title"
        :disabled="!open || !gid"
        title="在新标签页打开完整详情（等同中键 / Ctrl + 点击）"
        @click="openFullDetail"
      >
        🖼️ 画廊详情 <span class="open-hint">↗</span>
      </button>
      <button v-if="open" class="detail-panel-close" title="收起详情" @click="$emit('close')">
        ✕ 收起
      </button>
    </header>

    <div v-if="open && gid" class="detail-panel-body">
      <OnlineDetail embedded :gid="gid" :token="token" />
    </div>
    <div v-else class="detail-panel-empty">
      <div class="empty-icon">🖼️</div>
      <p>点击左侧画廊卡片<br />在此查看详情</p>
    </div>
  </aside>
</template>

<style scoped>
/* ─────────────────────────────────────────
   左右分栏容器（由各列表页 <div class="online-split"> 使用）
   宽屏桌面：左列表 + 右详情面板（面板 sticky 跟随滚动）；
   窄屏 / 强制移动：单列（面板由父级 v-if="isWide" 控制不渲染）
   ───────────────────────────────────────── */
:global(.online-split) {
  display: block;
  height: 100%;
}

:global(.online-split .split-main) {
  min-width: 0;
  min-height: 0;
}

@media (min-width: 1025px) {
  /* 默认：单列（详情面板收起，列表占满整行） */
  :global(html:not([data-layout='mobile']) .online-split) {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 16px;
    align-items: start;
  }

  /* 面板展开：左列表 + 右详情面板 */
  :global(html:not([data-layout='mobile']) .online-split.panel-open) {
    grid-template-columns: minmax(0, 1fr) minmax(360px, 420px);
  }

  :global(html:not([data-layout='mobile']) .online-split.panel-open .detail-panel) {
    position: sticky;
    top: 0;
    /* 56px 全局 TopBar + main-content 上下 24px 留白 */
    height: calc(100dvh - 104px);
    height: calc(100vh - 104px);
  }
}

/* ─────────────────────────────────────────
   详情面板本体（仅 isWide 时由父级渲染）
   ───────────────────────────────────────── */
.detail-panel {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  background: var(--app-surface);
  border: 1px solid var(--app-border-3);
  border-radius: 10px;
}

.detail-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--app-border-3);
  flex-shrink: 0;
}

.detail-panel-title {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: transparent;
  border: 1px solid transparent;
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--app-text-strong);
  padding: 3px 8px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.detail-panel-title:hover:not(:disabled) {
  color: #ff7588;
  background: var(--app-surface-2);
  border-color: var(--app-border-3);
}

.detail-panel-title:disabled {
  cursor: default;
  color: var(--app-text-3);
}

.open-hint {
  font-size: 0.75rem;
  opacity: 0.75;
}

.detail-panel-close {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 0.78rem;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s ease;
}

.detail-panel-close:hover {
  color: var(--app-text-strong);
  border-color: var(--app-border-3);
  background: var(--app-surface-2);
}

.detail-panel-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.detail-panel-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 24px;
  text-align: center;
  color: var(--app-text-3);
  font-size: 0.85rem;
  line-height: 1.6;
}

.empty-icon {
  font-size: 2.4rem;
  opacity: 0.7;
}
</style>
