<!-- eslint-disable vue/multi-word-component-names -->
<script setup lang="ts">
import { ref, computed } from 'vue'

const props = withDefaults(
  defineProps<{
    currentPage?: number
    totalPages?: number
  }>(),
  {
    currentPage: 1,
    totalPages: 1,
  },
)

const emit = defineEmits<{
  (e: 'change', page: number): void
}>()

// 手动跳转输入框绑定的页码
const inputPage = ref('')

// 安全触发切页
const goToPage = (page: number) => {
  if (page < 1 || page > props.totalPages || page === props.currentPage) return
  emit('change', page)
}

// 点击“跳页”或按下 Enter
const handleInputJump = () => {
  const target = parseInt(inputPage.value, 10)
  if (!isNaN(target) && target >= 1 && target <= props.totalPages) {
    goToPage(target)
    inputPage.value = ''
  }
}

// --------------------------------------------------
// 1. 动态计算中心连续 5 个页码 (以当前页为中心)
// --------------------------------------------------
const centerPages = computed(() => {
  const pages: number[] = []
  let start = Math.max(1, props.currentPage - 2)
  let end = Math.min(props.totalPages, props.currentPage + 2)

  // 边界平滑处理：确保在两端时也能尽量凑满 5 个页码
  if (props.currentPage <= 3) {
    end = Math.min(props.totalPages, 5)
  } else if (props.currentPage >= props.totalPages - 2) {
    start = Math.max(1, props.totalPages - 4)
  }

  for (let i = start; i <= end; i++) {
    pages.push(i)
  }
  return pages
})

// --------------------------------------------------
// 2. 向左/向右 跨度计算 (-20, -10, +10, +20)
// --------------------------------------------------
const minus20Page = computed(() => props.currentPage - 20)
const minus10Page = computed(() => props.currentPage - 10)
const plus10Page = computed(() => props.currentPage + 10)
const plus20Page = computed(() => props.currentPage + 20)

// --------------------------------------------------
// 3. 省略号显示判定
// --------------------------------------------------
const showLeftEllipsis = computed(() => {
  const minShown = minus10Page.value >= 1 ? minus10Page.value : centerPages.value[0]
  return minShown > 2
})

const showRightEllipsis = computed(() => {
  const maxShown =
    plus20Page.value <= props.totalPages
      ? plus20Page.value
      : plus10Page.value <= props.totalPages
        ? plus10Page.value
        : centerPages.value[centerPages.value.length - 1]
  return maxShown < props.totalPages - 1
})
</script>

<template>
  <div class="custom-pagination">
    <div class="pill-btn status">{{ currentPage }} of {{ totalPages }}</div>

    <button v-if="currentPage > 1" class="pill-btn nav-btn" @click="goToPage(1)">« 首页</button>

    <span v-if="showLeftEllipsis" class="symbol">...</span>

    <button v-if="minus20Page >= 1" class="pill-btn offset" @click="goToPage(minus20Page)">
      {{ minus20Page }}
    </button>

    <button v-if="minus10Page >= 1" class="pill-btn offset" @click="goToPage(minus10Page)">
      {{ minus10Page }}
    </button>

    <button
      v-if="currentPage > 1"
      class="pill-btn nav-arrow"
      title="上一页"
      @click="goToPage(currentPage - 1)"
    >
      «
    </button>

    <button
      v-for="p in centerPages"
      :key="p"
      class="pill-btn page-num"
      :class="{ active: p === currentPage }"
      @click="goToPage(p)"
    >
      {{ p }}
    </button>

    <button
      v-if="currentPage < totalPages"
      class="pill-btn nav-arrow"
      title="下一页"
      @click="goToPage(currentPage + 1)"
    >
      »
    </button>

    <button v-if="plus10Page <= totalPages" class="pill-btn offset" @click="goToPage(plus10Page)">
      {{ plus10Page }}
    </button>

    <button v-if="plus20Page <= totalPages" class="pill-btn offset" @click="goToPage(plus20Page)">
      {{ plus20Page }}
    </button>

    <span v-if="showRightEllipsis" class="symbol">...</span>

    <button v-if="currentPage < totalPages" class="pill-btn nav-btn" @click="goToPage(totalPages)">
      尾页 »
    </button>

    <div class="jump-box">
      <input
        v-model="inputPage"
        type="text"
        class="jump-input"
        placeholder=""
        @keyup.enter="handleInputJump"
      />
      <button class="pill-btn jump-btn" @click="handleInputJump">跳页</button>
    </div>
  </div>
</template>

<style scoped>
.custom-pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-wrap: wrap;
  gap: 6px;
  padding: 16px 0;
  user-select: none;
}

/* 椭圆型胶囊统一按钮基类 */
.pill-btn {
  background-color: transparent;
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
  border-radius: 20px; /* 椭圆胶囊造型 */
  padding: 3px 12px;
  font-size: 0.82rem;
  cursor: pointer;
  transition: all 0.15s ease;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  height: 28px;
  box-sizing: border-box;
}

.pill-btn:hover {
  border-color: #00a896;
  color: var(--app-text-strong);
}

/* 激活状态 (青蓝色高亮) */
.pill-btn.active {
  background-color: #00a896;
  border-color: #00a896;
  color: #fff;
  font-weight: bold;
}

/* 状态展示 */
.pill-btn.status {
  cursor: default;
  color: var(--app-text-3);
  border-color: var(--app-border-2);
  padding: 3px 14px;
}

/* 单步箭头样式 */
.pill-btn.nav-arrow {
  font-weight: bold;
  padding: 3px 8px;
}

/* 间隔符号 */
.symbol {
  color: var(--app-text-muted);
  font-size: 0.85rem;
  padding: 0 4px;
}

/* 跳转输入框容器 */
.jump-box {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: 6px;
}

.jump-input {
  width: 46px;
  height: 26px;
  background-color: transparent;
  border: none;
  border-bottom: 1px solid var(--app-border-3);
  color: var(--app-text-strong);
  text-align: center;
  font-size: 0.85rem;
  outline: none;
  transition: border-color 0.2s;
}

.jump-input:focus {
  border-bottom-color: #00a896;
}

/* 📱 窄屏适配：隐藏 ±10/±20 快捷跳页与首页/尾页，减少按钮换行堆叠 */
@media (max-width: 767px) {
  .offset,
  .nav-btn {
    display: none;
  }
}
</style>
