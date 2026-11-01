<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { API_BASE } from '@/config/api'
import { usePagePreview } from './usePagePreview'

/**
 * Round52：页预览灯箱——原图直出（复用 /raw-page，不缩放，便于判断画质差异）。
 * 操作：← → 翻页、Esc 关闭、「切换」在最近打开的两本之间来回跳（各回各的最后一页）。
 */
const {
  visible,
  current,
  currentTarget,
  swapTarget,
  swapTargetId,
  lastPage,
  closePreview,
  movePreview,
  swapPreview,
} = usePagePreview()

const pageMax = computed(() => currentTarget.value?.pageCount ?? 1)
const imageUrl = computed(() =>
  current.id ? `${API_BASE}/comics/${current.id}/raw-page/${current.page - 1}` : '',
)
const title = computed(() => {
  const target = currentTarget.value
  if (!target) return ''
  return `成员 ${target.label} · 第 ${current.page} / ${target.pageCount} 页`
})
const swapLabel = computed(() => {
  const target = swapTarget.value
  if (!target) return '另一本未打开'
  const page = lastPage[swapTargetId.value] ?? 1
  return `切到 ${target.label} · 第 ${page} 页`
})

function onKey(e: KeyboardEvent) {
  if (!visible.value) return
  if (e.key === 'Escape') closePreview()
  else if (e.key === 'ArrowLeft') movePreview(-1)
  else if (e.key === 'ArrowRight') movePreview(1)
}

onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div v-if="visible" class="lb" @click.self="closePreview">
    <div class="lb-top">
      <span class="lb-title">{{ title }}</span>
      <span class="spacer"></span>
      <button class="lb-btn" :disabled="!swapTarget" @click.stop="swapPreview">{{ swapLabel }}</button>
      <button class="lb-btn" @click.stop="closePreview">关闭</button>
    </div>
    <div class="lb-stage">
      <img :src="imageUrl" :alt="title" />
    </div>
    <button class="lb-nav lb-prev" :disabled="current.page <= 1" @click.stop="movePreview(-1)">‹</button>
    <button class="lb-nav lb-next" :disabled="current.page >= pageMax" @click.stop="movePreview(1)">›</button>
  </div>
</template>

<style scoped>
.lb {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.94);
  z-index: 999;
}
.lb-top {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 44px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 14px;
  background: rgba(0, 0, 0, 0.5);
  color: var(--app-text-2);
  font-size: 12.5px;
}
.lb-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.spacer {
  flex: 1;
}
.lb-btn {
  background: none;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 5px;
  padding: 3px 10px;
  font-size: 12px;
  font-family: inherit;
  cursor: pointer;
}
.lb-btn:hover:not(:disabled) {
  background: var(--app-surface-3-hover);
}
.lb-btn:disabled {
  opacity: 0.38;
  cursor: default;
}
.lb-stage {
  position: absolute;
  top: 44px;
  left: 64px;
  right: 64px;
  bottom: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}
.lb-stage img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}
.lb-nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 40px;
  height: 76px;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 6px;
  color: #ccc;
  font-size: 20px;
  cursor: pointer;
}
.lb-nav:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.14);
}
.lb-nav:disabled {
  opacity: 0.3;
  cursor: default;
}
.lb-prev {
  left: 10px;
}
.lb-next {
  right: 10px;
}
</style>
