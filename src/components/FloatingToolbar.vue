<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { preferenceSettings } from '@/stores/preferenceSettings'
import { scrollMainToTop } from '@/utils/scrollMemory'
// Round4 任务八：日期跳转改为弹窗（选择节点 / 指定日期）
import DateJumpModal from '@/components/DateJumpModal.vue'

// 🟢 1. 增加控制 Props（模板中直接使用 showSort / sortMode）
withDefaults(
  defineProps<{
    showSort?: boolean // 是否显示排序按钮（仅收藏夹传 true）
    sortMode?: 'favorited' | 'published' // 当前排序模式
  }>(),
  {
    showSort: false,
    sortMode: 'favorited',
  },
)

const emit = defineEmits<{
  (e: 'refresh'): void
  (e: 'seek-change', date: string): void
  (e: 'toggle-sort'): void // 🟢 触发排序切换事件
}>()

const isOpen = ref(false)
// Round4 任务八：日期跳转弹窗显隐
const dateJumpOpen = ref(false)

const toggleMenu = () => {
  isOpen.value = !isOpen.value
}

const handleScrollTop = () => {
  // 统一走公共工具：真实滚动容器是 #main-content
  scrollMainToTop('smooth')
  isOpen.value = false
}

const handleRefresh = () => {
  emit('refresh')
  isOpen.value = false
}

// 🟢 点击切换排序
const handleToggleSort = () => {
  emit('toggle-sort')
  isOpen.value = false
}

// Round4 任务八：打开日期跳转弹窗
const handleOpenDateJump = () => {
  dateJumpOpen.value = true
  isOpen.value = false
}

// 弹窗确认后向上抛出日期（YYYY-MM-DD），由页面按各自逻辑 seek
const handleSeekConfirm = (date: string) => {
  emit('seek-change', date)
  dateJumpOpen.value = false
}

// 🖥️ 偏好设置：控制「回到顶部」按钮的显隐
const showScrollTop = ref(preferenceSettings.hideScrollToTopBtn !== 'always')

let lastScrollTop = 0
const handleMainScroll = (e: Event) => {
  const el = e.target as HTMLElement
  const st = el.scrollTop
  const movingDown = st > lastScrollTop
  lastScrollTop = st
  // 'scrolling_down'：向下滚动隐藏、向上滚动显示；其余模式由初始值决定
  if (preferenceSettings.hideScrollToTopBtn === 'scrolling_down') {
    showScrollTop.value = !movingDown || st <= 10
  }
}

onMounted(() => {
  const mainEl = document.querySelector('.main-content')
  mainEl?.addEventListener('scroll', handleMainScroll, { passive: true })
})
onUnmounted(() => {
  const mainEl = document.querySelector('.main-content')
  mainEl?.removeEventListener('scroll', handleMainScroll)
})
</script>

<template>
  <div class="floating-toolbar">
    <Transition name="fab-fade">
      <div v-if="isOpen" class="fab-menu">
        <button v-if="showScrollTop" class="menu-item" title="回到顶部" @click="handleScrollTop">
          <span class="icon">⬆️</span>
          <span class="label">回到顶部</span>
        </button>

        <!-- 🟢 2. 仅在 showSort 为 true 时显示的单按钮一键切换 -->
        <button
          v-if="showSort"
          class="menu-item sort-item"
          :title="sortMode === 'favorited' ? '切换为：按发布时间排序' : '切换为：按收藏时间排序'"
          @click="handleToggleSort"
        >
          <span class="icon">{{ sortMode === 'favorited' ? '⭐' : '🕒' }}</span>
          <span class="label">
            {{ sortMode === 'favorited' ? '按收藏时间' : '按发布时间' }}
          </span>
        </button>

        <button class="menu-item" title="刷新页面" @click="handleRefresh">
          <span class="icon">🔄</span>
          <span class="label">刷新列表</span>
        </button>

        <!-- Round4 任务八：日期项改为按钮 → 打开日期跳转弹窗 -->
        <button class="menu-item" title="按日期跳转" @click="handleOpenDateJump">
          <span class="icon">📅</span>
          <span class="label">日期跳转</span>
        </button>
      </div>
    </Transition>

    <button class="fab-trigger" :class="{ active: isOpen }" title="操作菜单" @click="toggleMenu">
      <span class="trigger-icon">{{ isOpen ? '✕' : '⚙️' }}</span>
    </button>

    <DateJumpModal :show="dateJumpOpen" @close="dateJumpOpen = false" @confirm="handleSeekConfirm" />
  </div>
</template>

<style scoped>
/* 右下角固定定位 */
.floating-toolbar {
  position: fixed;
  right: 32px;
  bottom: 32px;
  z-index: 999;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 10px;
}

/* 展开菜单卡片 */
.fab-menu {
  display: flex;
  flex-direction: column;
  gap: 4px;
  background-color: var(--app-surface-2);
  backdrop-filter: blur(8px);
  border: 1px solid var(--app-border-3);
  padding: 8px;
  border-radius: 12px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  background: transparent;
  border: none;
  color: var(--app-text-2);
  padding: 8px 12px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 0.85rem;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.menu-item:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

/* 圆形悬浮球主按钮 */
.fab-trigger {
  width: 46px;
  height: 46px;
  border-radius: 50%;
  background-color: #00a896;
  color: #ffffff;
  border: none;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.1rem;
  transition: all 0.2s ease;
}

.fab-trigger:hover {
  transform: scale(1.08);
  background-color: #00c4af;
}

.fab-trigger.active {
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  transform: rotate(90deg);
}

/* 弹出动画 */
.fab-fade-enter-active,
.fab-fade-leave-active {
  transition: all 0.2s ease;
}

.fab-fade-enter-from,
.fab-fade-leave-to {
  opacity: 0;
  transform: translateY(12px) scale(0.92);
}
</style>
