<script setup lang="ts">
/**
 * Round39：侧栏折叠分组标题行（书架 / 书签共用）——抽屉标题的唯一样式与结构来源
 *
 * 背景：书签标题原先复用全局 .group-title（0.75rem / muted 灰，自带 🔖 前缀、
 * 折叠箭头在左），与书架标题行（0.9rem / text-2、箭头在右）两套观感并存。
 * 本组件把标题行统一到书架口径：
 *   [标题][角标][actions 插槽] ← 伸缩空隙 → [❯]
 * 整行可点击切换折叠，箭头展开时旋转 90°；actions 区点击不冒泡（避免误触折叠）。
 */
defineProps<{
  /** 标题文字（如「书架」「书签」） */
  title: string
  /** 展开状态（驱动箭头旋转） */
  open: boolean
  /** 计数角标（如折叠时的书签总数），不传则不渲染 */
  badge?: number | string
  /** 角标 hover 提示 */
  badgeTitle?: string
}>()

const emit = defineEmits<{ toggle: [] }>()
</script>

<template>
  <div class="fold-header" @click="emit('toggle')">
    <span class="fh-title">{{ title }}</span>
    <span v-if="badge !== undefined && badge !== ''" class="fh-badge" :title="badgeTitle">{{ badge }}</span>
    <!-- 操作区：@click.stop 防止点「🔎 检测 / 🧹 清理」时连带折叠分组 -->
    <span class="fh-actions" @click.stop>
      <slot name="actions" />
    </span>
    <span class="arrow" :class="{ open }">❯</span>
  </div>
</template>

<style scoped>
/* 字体/内边距/hover 与书架标题行原 .foldable-header 保持一致（本次作为统一基准） */
.fold-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  color: var(--app-text-2);
  cursor: pointer;
  border-radius: 6px;
  font-size: 0.9rem;
  transition: all 0.2s;
}

.fold-header:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

.fh-title {
  flex-shrink: 0;
}

.fh-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
}

/* 折叠态计数角标（沿用原书签组 .bm-total 视觉） */
.fh-badge {
  flex-shrink: 0;
  font-size: 0.7rem;
  color: var(--app-text-muted);
  background-color: var(--app-surface-3);
  padding: 0 5px;
  border-radius: 8px;
}

/* 折叠箭头固定行右端；展开时旋转 90° */
.arrow {
  margin-left: auto;
  font-size: 0.75rem;
  transition: transform 0.2s;
}

.arrow.open {
  transform: rotate(90deg);
}
</style>
