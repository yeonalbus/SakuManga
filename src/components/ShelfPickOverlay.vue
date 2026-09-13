<script setup lang="ts">
/**
 * Round38：抽一本未读 —— 结果卡浮层（书架墙 / 书架页共用）
 *
 * 交互要点：先出结果卡，再由用户决定「开始阅读 / 再抽一张」。
 * 不抽完直接跳走，否则无法换一张（这是「从书架消费」的必要摩擦消除）。
 */
import { computed, ref, watch } from 'vue'
import { bookshelves } from '@/stores/bookshelfStore'
import {
  pickUnreadFromAllShelves,
  pickUnreadFromShelf,
  shelfUnreadCount,
  shelfWallSummary,
} from '@/composables/useShelfPick'
import { openContentTab } from '@/utils/detailNav'
import type { OfflineComic } from '@/types/comic'

const props = withDefaults(
  defineProps<{
    open: boolean
    /** 指定书架 id；空 = 全部书架 */
    shelfId?: string
    /** 展示用书架名（shelfId 为空时忽略） */
    shelfName?: string
  }>(),
  { shelfId: '', shelfName: '' },
)

const emit = defineEmits<{ (e: 'close'): void }>()

/** 当前抽取范围：shelf=指定书架；all=全部书架（该架清空后的引导升级） */
const scope = ref<'shelf' | 'all'>('shelf')
/** 当前抽出的作品；null = 候选池为空 */
const current = ref<OfflineComic | null>(null)
/** 上一张的 id：「再抽一张」优先避开，避免连续抽到同一本 */
const lastId = ref('')

const targetShelf = computed(() => bookshelves.value.find((b) => b.id === props.shelfId))

const scopeName = computed(() =>
  scope.value === 'all' ? '全部书架' : props.shelfName || targetShelf.value?.name || '书架',
)

/** 当前范围剩余未读数（全部书架用去重汇总，避免同一本跨架重复计入） */
const remainUnread = computed(() =>
  scope.value === 'all'
    ? shelfWallSummary.value.unread
    : shelfUnreadCount(targetShelf.value),
)

/** 抽取一张（excludeId 由上次结果提供） */
const roll = () => {
  const picked =
    scope.value === 'all'
      ? pickUnreadFromAllShelves(lastId.value)
      : pickUnreadFromShelf(props.shelfId, lastId.value)
  current.value = picked
  lastId.value = picked ? picked.id : ''
}

// 打开时重置范围与历史并立即抽一张；切换目标书架时重新抽
watch(
  [() => props.open, () => props.shelfId],
  ([open]) => {
    if (!open) return
    scope.value = 'shelf'
    lastId.value = ''
    current.value = null
    roll()
  },
  { immediate: true },
)

const close = () => emit('close')

/** 该架已清空 → 升级为「全库未读」继续抽 */
const switchToAll = () => {
  scope.value = 'all'
  lastId.value = ''
  current.value = null
  roll()
}

const startRead = () => {
  const comic = current.value
  if (!comic) return
  close()
  // 与详情页「立即阅读」同一入口：PC 新标签 / 窄屏同标签由 openContentTab 分流
  openContentTab({
    href: `/reader?id=${encodeURIComponent(comic.id)}&source=offline`,
    id: comic.id,
  })
}

/** 封面缺失 / 缓存未生成时收起图片（占位块由 CSS 背景承担） */
const onCoverError = (e: Event) => {
  const img = e.target as HTMLImageElement | null
  if (img) img.style.display = 'none'
}
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="open" class="pick-mask" @click.self="close">
        <div class="pick-panel">
          <div class="pick-header">
            <h3 class="pick-title">🎲 {{ scopeName }} · 抽一本未读</h3>
            <button class="pick-close" @click="close">✕</button>
          </div>

          <div v-if="current" class="pick-body">
            <div class="pick-cover">
              <img
                :src="current.coverUrl || ''"
                :alt="current.title"
                loading="lazy"
                @error="onCoverError"
              />
              <span class="pick-cover-fallback">{{ current.title.slice(0, 1) }}</span>
            </div>
            <h4 class="pick-comic-title" :title="current.title">{{ current.title }}</h4>
            <p class="pick-meta">
              <span class="meta-chip">{{ current.pageCount || 0 }} 页</span>
              <span class="meta-chip">该范围剩余未读 {{ remainUnread }} 本</span>
            </p>
          </div>

          <div v-else class="pick-empty">
            <span class="empty-icon">✅</span>
            <p class="empty-title">
              {{ scope === 'shelf' ? '该系列已清空' : '全部书架都已读过一遍' }}
            </p>
            <p class="empty-hint">
              {{
                scope === 'shelf'
                  ? '这个书架里的作品都读过了，可以换全库未读继续抽'
                  : '整理成果已全部消费完，可以去维护里补充新内容'
              }}
            </p>
          </div>

          <div class="pick-footer">
            <template v-if="current">
              <button class="primary-btn" @click="startRead">▶ 开始阅读</button>
              <button class="ghost-btn" @click="roll">🎲 再抽一张</button>
            </template>
            <template v-else>
              <button v-if="scope === 'shelf'" class="primary-btn" @click="switchToAll">
                🎲 抽全库未读
              </button>
              <button class="ghost-btn" @click="close">关闭</button>
            </template>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.pick-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(2px);
}
.pick-panel {
  width: 420px;
  max-width: 92vw;
  max-height: 86vh;
  display: flex;
  flex-direction: column;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 12px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.6);
  overflow: hidden;
}
.pick-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border-2);
}
.pick-title {
  font-size: 0.98rem;
  font-weight: 600;
  color: var(--app-text-strong);
  margin: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.pick-close {
  background: transparent;
  border: none;
  color: var(--app-text-3);
  font-size: 1rem;
  cursor: pointer;
  padding: 2px 6px;
}
.pick-close:hover {
  color: var(--app-text-strong);
}
.pick-body {
  padding: 16px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  overflow-y: auto;
}
.pick-cover {
  position: relative;
  width: 200px;
  height: 280px;
  border-radius: 8px;
  overflow: hidden;
  background: linear-gradient(145deg, var(--app-surface-3), var(--app-bg-deep));
  border: 1px solid var(--app-border-2);
  display: flex;
  align-items: center;
  justify-content: center;
}
.pick-cover img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.pick-cover-fallback {
  font-size: 3rem;
  color: var(--app-text-3);
  opacity: 0.5;
}
.pick-comic-title {
  margin: 0;
  font-size: 0.92rem;
  font-weight: 600;
  color: var(--app-text-strong);
  text-align: center;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.pick-meta {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: center;
  margin: 0;
}
.meta-chip {
  font-size: 0.75rem;
  background: var(--app-surface-3);
  color: var(--app-text-2);
  padding: 2px 10px;
  border-radius: 10px;
}
.pick-empty {
  padding: 32px 20px;
  text-align: center;
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: center;
}
.empty-icon {
  font-size: 2.2rem;
}
.empty-title {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--app-text-strong);
}
.empty-hint {
  margin: 0;
  font-size: 0.8rem;
  color: var(--app-text-3);
  line-height: 1.6;
}
.pick-footer {
  display: flex;
  gap: 10px;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border-2);
}
.primary-btn {
  flex: 1;
  background: #007acc;
  border: none;
  color: #fff;
  padding: 9px 14px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.86rem;
  font-weight: 600;
  transition: background-color 0.15s;
}
.primary-btn:hover {
  background: #0090f0;
}
.ghost-btn {
  flex: 1;
  background: transparent;
  border: 1px dashed var(--app-border-3);
  color: var(--app-text-2);
  padding: 9px 14px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.86rem;
  transition: all 0.15s;
}
.ghost-btn:hover {
  border-color: #007acc;
  color: #007acc;
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.18s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
