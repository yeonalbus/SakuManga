<script setup lang="ts">
/**
 * Round39：侧栏折叠分组标题行（书架 / 书签共用）——抽屉标题的唯一样式与结构来源
 *
 * 背景：书签标题原先复用全局 .group-title（0.75rem / muted 灰，自带 🔖 前缀、
 * 折叠箭头在左），与书架标题行（0.9rem / text-2、箭头在右）两套观感并存。
 * 本组件把标题行统一到书架口径：
 *   [标题][角标][actions 插槽] ← 伸缩空隙 → [❯]
 * 整行可点击切换折叠，箭头展开时旋转 90°；actions 区点击不冒泡（避免误触折叠）。
 *
 * Round49：新增 `collapsible=false` 变体——书签组恒定平铺、不显示箭头，
 * 整行点击改为 emit('activate')（进入书签页）。书架仍走默认折叠模式，行为不变。
 */
const props = defineProps<{
  /** 标题文字（如「书架」「书签」） */
  title: string
  /** 展开状态（折叠模式下驱动箭头旋转；collapsible=false 时忽略） */
  open?: boolean
  /** 是否可折叠：false = 恒定平铺、无箭头，点击整行触发 activate */
  collapsible?: boolean
  /** 计数角标（如折叠时的书签总数），不传则不渲染 */
  badge?: number | string
  /** 角标 hover 提示 */
  badgeTitle?: string
}>()

const emit = defineEmits<{ toggle: []; activate: [] }>()

const handleClick = () => {
  if (props.collapsible === false) emit('activate')
  else emit('toggle')
}
</script>

<template>
  <div class="fold-header" @click="handleClick">
    <span class="fh-title">{{ title }}</span>
    <span v-if="badge !== undefined && badge !== ''" class="fh-badge" :title="badgeTitle">{{ badge }}</span>
    <!-- 操作区：@click.stop 防止点分区按钮时连带折叠分组 -->
    <span class="fh-actions" @click.stop>
      <slot name="actions" />
    </span>
    <span v-if="collapsible !== false" class="arrow" :class="{ open }">❯</span>
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
