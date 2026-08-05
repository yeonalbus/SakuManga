<script setup lang="ts">
import { ref } from 'vue'

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
const selectedDate = ref('')

const toggleMenu = () => {
  isOpen.value = !isOpen.value
}

const handleScrollTop = () => {
  const mainEl = document.querySelector('.main-content')
  if (mainEl) {
    mainEl.scrollTo({ top: 0, behavior: 'smooth' })
  } else {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
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

const handleDateSubmit = () => {
  if (selectedDate.value) {
    emit('seek-change', selectedDate.value)
    isOpen.value = false
  }
}
</script>

<template>
  <div class="floating-toolbar">
    <Transition name="fab-fade">
      <div v-if="isOpen" class="fab-menu">
        <button class="menu-item" title="回到顶部" @click="handleScrollTop">
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

        <div class="menu-item date-item" title="选择日期后按 Enter 跳转">
          <span class="icon">📅</span>
          <input
            type="date"
            class="date-picker-input"
            min="2007-01-01"
            v-model="selectedDate"
            @keyup.enter="handleDateSubmit"
          />
        </div>
      </div>
    </Transition>

    <button class="fab-trigger" :class="{ active: isOpen }" title="操作菜单" @click="toggleMenu">
      <span class="trigger-icon">{{ isOpen ? '✕' : '⚙️' }}</span>
    </button>
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
  background-color: rgba(26, 26, 26, 0.92);
  backdrop-filter: blur(8px);
  border: 1px solid #333;
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
  color: #ccc;
  padding: 8px 12px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 0.85rem;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.menu-item:hover {
  background-color: #2a2a2a;
  color: #fff;
}

.date-item {
  display: flex;
  align-items: center;
}

.date-picker-input {
  background: transparent;
  border: none;
  color: #00a896;
  font-size: 0.82rem;
  font-weight: bold;
  outline: none;
  cursor: pointer;
  width: 115px;
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
  background-color: #2a2a2a;
  border: 1px solid #444;
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
