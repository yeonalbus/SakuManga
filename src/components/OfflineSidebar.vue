<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { bookshelves, addBookshelf, removeBookshelf } from '@/stores/bookshelfStore'
import { useUserStore } from '@/stores/userStore'

const router = useRouter()
const route = useRoute() // 1. 引入 useRoute 用于精准匹配 query.id
const { modal, toast } = useUI()

// Round3-任务2：更新/维护入口仅管理员可见
const { isAdmin } = useUserStore()

// 控制书架菜单的展开/折叠状态
const isBookshelfOpen = ref(true)

const toggleBookshelf = () => {
  isBookshelfOpen.value = !isBookshelfOpen.value
}

// 新建书架
const createNewBookshelf = async () => {
  const name = await modal.prompt('请输入新书架名称', '', '创建书架')
  if (name && name.trim()) {
    addBookshelf(name.trim())
    toast.success(`书架「${name}」创建成功！`)
  }
}

// 删除书架
const handleDeleteShelf = async (shelfId: string, shelfName: string) => {
  const confirmed = await modal.confirm(`确定要删除书架「${shelfName}」吗？`, '删除确认')
  if (confirmed) {
    removeBookshelf(shelfId)
    toast.info(`书架「${shelfName}」已删除`)

    // 2. 核心修正：只有当前正处于被删除的这个书架页面时，才跳回首页
    if (route.query.id === shelfId) {
      router.push('/offline/home')
    }
  }
}
</script>

<template>
  <div class="nav-group">
    <span class="group-title">📚 离线模式</span>
    <router-link to="/offline/home">首页</router-link>
    <!-- Round3-任务2：更新/维护入口仅管理员可见 -->
    <router-link v-if="isAdmin" to="/offline/update">更新</router-link>
    <router-link v-if="isAdmin" to="/offline/maintain">维护</router-link>
    <router-link to="/offline/toplist">排行榜</router-link>
    <router-link to="/offline/history">历史记录</router-link>

    <div class="foldable-item">
      <div class="foldable-header" @click="toggleBookshelf">
        <span>书架</span>
        <span class="arrow" :class="{ open: isBookshelfOpen }">❯</span>
      </div>

      <div v-show="isBookshelfOpen" class="foldable-body">
        <router-link
          v-for="shelf in bookshelves"
          :key="shelf.id"
          :to="`/offline/bookshelf?id=${shelf.id}`"
          class="sub-nav-item"
          :class="{ active: route.query.id === shelf.id }"
        >
          <span class="shelf-name">{{ shelf.name }}</span>

          <div class="shelf-right-info">
            <span class="shelf-count">{{ shelf.count || 0 }}</span>

            <span
              class="delete-btn"
              title="删除书架"
              @click.stop.prevent="handleDeleteShelf(shelf.id, shelf.name)"
            >
              ✕
            </span>
          </div>
        </router-link>

        <button class="add-shelf-btn" @click="createNewBookshelf">➕ 新建书架</button>
      </div>
    </div>
  </div>

  <!-- 🎲 工具：跨模式全局功能（骰子支持全库、清单有在线/离线双 tab） -->
  <div class="nav-group">
    <span class="group-title">🎲 工具</span>
    <router-link to="/random">手气不错</router-link>
    <router-link to="/reading-list">阅读清单</router-link>
  </div>
</template>

<style scoped>
.foldable-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  color: var(--app-text-2);
  cursor: pointer;
  border-radius: 6px;
  font-size: 0.9rem;
  transition: all 0.2s;
}
.foldable-header:hover {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
}

.arrow {
  font-size: 0.75rem;
  transition: transform 0.2s;
}
.arrow.open {
  transform: rotate(90deg);
}

.foldable-body {
  display: flex;
  flex-direction: column;
  padding-left: 12px;
  margin-top: 2px;
  gap: 2px;
}

.sub-nav-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 8px;
  border-radius: 4px;
  font-size: 0.85rem !important;
  color: var(--app-text-3) !important;
  transition: all 0.2s;
  text-decoration: none;
  background-color: transparent;
}

/* 1. 核心修复：彻底清空 Vue Router 默认下发给所有书架的全局大蓝块背景 */
.sub-nav-item.router-link-active,
.sub-nav-item.router-link-exact-active {
  background-color: transparent !important;
  color: var(--app-text-3) !important;
}

/* 2. 普通悬浮态 */
.sub-nav-item:hover {
  background-color: var(--app-surface-3) !important;
  color: var(--app-text-strong) !important;
}

/* 3. 只有当前 ID 100% 匹配时，我们手绑定的 .active 才独占高亮 */
.sub-nav-item.active {
  background-color: #007acc !important; /* 经典蓝色底块 */
  color: #ffffff !important; /* 白字 */
  font-weight: bold;
}

.shelf-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 130px;
}

.shelf-right-info {
  display: flex;
  align-items: center;
  gap: 6px;
}

.shelf-count {
  font-size: 0.75rem;
  background-color: var(--app-surface-3);
  padding: 1px 6px;
  border-radius: 10px;
  color: var(--app-text-2);
}

.delete-btn {
  font-size: 0.75rem;
  color: var(--app-text-2);
  padding: 0 4px;
  border-radius: 3px;
  opacity: 0;
  transition:
    opacity 0.2s,
    color 0.2s;
}

.sub-nav-item:hover .delete-btn {
  opacity: 1;
}

.delete-btn:hover {
  color: #ef4444 !important;
  background-color: rgba(239, 68, 68, 0.2);
}

.add-shelf-btn {
  background: transparent;
  border: 1px dashed var(--app-border-3);
  color: var(--app-text-3);
  padding: 6px 10px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.8rem;
  text-align: left;
  margin-top: 4px;
  transition: all 0.2s;
}
.add-shelf-btn:hover {
  border-color: #007acc;
  color: #007acc;
}
</style>
