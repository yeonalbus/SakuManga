<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { pinnedBookshelves, PIN_LIMIT, addBookshelf } from '@/stores/bookshelfStore'
import { useUserStore } from '@/stores/userStore'
import AppIcon from '@/components/icons/AppIcon.vue'

const route = useRoute()
const { modal, toast } = useUI()
const { isAdmin } = useUserStore()

// 新建书架
const createNewBookshelf = async () => {
  const name = await modal.prompt('请输入新书架名称', '', '创建书架')
  if (name && name.trim()) {
    addBookshelf(name.trim())
    toast.success(`书架「${name.trim()}」创建成功！`)
  }
}
</script>

<template>
  <div class="nav-group">
    <span class="group-title"><AppIcon name="books" :size="13" /> 离线模式</span>
    <router-link to="/offline/home">首页</router-link>
    <!-- Round3-任务2：更新/维护入口仅管理员可见 -->
    <router-link v-if="isAdmin" to="/offline/update">更新</router-link>
    <router-link v-if="isAdmin" to="/offline/maintain">维护</router-link>
    <!-- Round44：忽略清单独立页（维护页的已忽略内容都收在这里） -->
    <router-link v-if="isAdmin" to="/offline/ignore">忽略清单</router-link>
    <router-link to="/offline/toplist">排行榜</router-link>
    <router-link to="/offline/history">历史记录</router-link>

    <!-- Round55：书架入口与子项布局向在线「书签」栏看齐 -->
    <router-link to="/offline/bookshelves">书架</router-link>

    <div class="shelf-children">
      <button class="shelf-create-btn" @click="createNewBookshelf">新建书架</button>

      <router-link
        v-for="shelf in pinnedBookshelves"
        :key="shelf.id"
        :to="`/offline/bookshelf?id=${shelf.id}`"
        class="shelf-child-item"
        :class="{ active: route.path === '/offline/bookshelf' && route.query.id === shelf.id }"
        :title="shelf.name"
      >
        <span class="shelf-child-title">{{ shelf.name }}</span>
      </router-link>

      <span v-if="pinnedBookshelves.length === 0" class="shelf-pin-hint">
        在书架页置顶常用书架（最多 {{ PIN_LIMIT }} 个）
      </span>
    </div>
  </div>

  <!-- 🎲 工具：跨模式全局功能（骰子支持全库、清单有在线/离线双 tab） -->
  <div class="nav-group">
    <span class="group-title"><AppIcon name="dice" :size="13" /> 工具</span>
    <router-link to="/random">手气不错</router-link>
    <router-link to="/reading-list">阅读清单</router-link>
    <!-- 画质升级：与工具组其他组件一致对所有登录用户显示（下载许可由后端按用户校验） -->
    <router-link to="/upgrade">画质升级</router-link>
  </div>
</template>

<style scoped>
/* 子项容器：相对组左边缩进 32px（与 OnlineSidebar .bm-children 对齐） */
.shelf-children {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-left: 32px;
  margin-top: 2px;
}

/* 新建书架：与「检查更新」同款边框按钮 */
.shelf-create-btn {
  box-sizing: border-box;
  width: 100%;
  margin: 2px 0 4px;
  padding: 5px 10px;
  border-radius: 4px;
  border: 1px solid var(--app-border-3);
  background: transparent;
  color: var(--app-text-3);
  font-size: 0.8rem;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  transition: all 0.2s;
}
.shelf-create-btn:hover {
  border-color: #007acc;
  color: #007acc;
}

/* 置顶书架子项 */
.shelf-child-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 6px 8px;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--app-text-muted) !important;
  font-size: 0.8rem !important;
  font-family: inherit;
  text-align: left;
  text-decoration: none;
  cursor: pointer;
  transition: all 0.2s;
}

/* 清空 Vue Router 默认给 /offline/bookshelf 下发的全局高亮，仅在精确匹配 query.id 时高亮 */
.shelf-child-item.router-link-active,
.shelf-child-item.router-link-exact-active {
  background-color: transparent !important;
  color: var(--app-text-muted) !important;
}

.shelf-child-item:hover {
  background-color: var(--app-surface-3) !important;
  color: var(--app-text-strong) !important;
}

.shelf-child-item.active {
  background-color: #007acc !important;
  color: #ffffff !important;
  font-weight: 600;
}

.shelf-child-title {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.shelf-pin-hint {
  font-size: 0.72rem;
  line-height: 1.5;
  color: var(--app-text-muted);
  padding: 2px 8px;
}
</style>
