<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import ModeToggle from '@/components/ModeToggle.vue'
import OnlineSidebar from '@/components/OnlineSidebar.vue' // 引入两个侧边栏组件
import OfflineSidebar from '@/components/OfflineSidebar.vue' // 引入两个侧边栏组件
import TopBar from '@/components/TopBar.vue' // 引入组合好的顶栏
import GlobalToast from '@/components/common/GlobalToast.vue'
import GlobalModal from '@/components/common/GlobalModal.vue'
import { useTagStore } from '@/stores/tagStore'

const tagStore = useTagStore()

onMounted(() => {
  // 🚀 应用启动时异步获取翻译字典
  tagStore.fetchTagDictionary()
})

const route = useRoute()

// 1. 用一个 ref 存储当前模式，默认设为 'online'
const currentMode = ref<'online' | 'offline'>('online')

// 2. 监听路由路径变化，更新模式记忆
watch(
  () => route.path,
  (newPath) => {
    if (newPath.startsWith('/online')) {
      currentMode.value = 'online'
    } else if (newPath.startsWith('/offline')) {
      currentMode.value = 'offline'
    }
    // 注意：如果是 /downloads 或 /settings，既不是 online 也不是 offline，
    // currentMode 会保持上一次的值不变！
  },
  { immediate: true }, // 页面刚加载时立即执行一次
)
</script>

<template>
  <div class="app-layout">
    <GlobalToast />
    <GlobalModal />
  </div>

  <div class="app-container">
    <!-- 左侧导航栏 -->
    <aside class="sidebar">
      <div class="logo-area">
        <span class="logo">E-Manager</span>
        <ModeToggle />
      </div>

      <!-- 导航菜单 -->
      <nav class="nav-menu">
        <!-- 在线/离线菜单，用 v-if / v-else 切换对应的组件 -->
        <OnlineSidebar v-if="currentMode === 'online'" />
        <OfflineSidebar v-else />
        <!-- 全局通用的系统菜单与历史记录：放在侧边栏底部或独立组里 -->
        <div class="nav-group">
          <span class="group-title">⚙️ 系统</span>
          <router-link to="/downloads">下载列表</router-link>
          <router-link to="/settings">系统设置</router-link>
        </div>
      </nav>
    </aside>

    <!-- 2. 右侧主体包装层（包含顶栏 + 内容区） -->
    <div class="right-wrapper">
      <!-- 顶部操作栏 直接调用整合好的顶栏组件 -->
      <TopBar />

      <!-- 页面主体显示区 -->
      <main class="main-content">
        <router-view v-slot="{ Component }">
          <keep-alive>
            <component :is="Component" :key="$route.fullPath" />
          </keep-alive>
        </router-view>
      </main>
    </div>
  </div>
</template>

<style>
/* 全局基础重置 */
* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}
body {
  background-color: #121212;
  color: #e0e0e0;
  font-family:
    system-ui,
    -apple-system,
    sans-serif;
}

.app-container {
  display: flex;
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}

/* 侧边栏样式 */
.sidebar {
  width: 240px;
  background-color: #1a1a1a;
  border-right: 1px solid #2a2a2a;
  display: flex;
  flex-direction: column;
  padding: 20px 10px;
}

/* 让 Logo 标题和按钮在同一行并排显示 */
.logo-area {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 5px 20px 5px;
  border-bottom: 1px solid #2a2a2a;
  margin-bottom: 15px;
}

.logo {
  font-size: 1.1rem;
  font-weight: bold;
  color: #007acc;
}

.nav-group {
  margin-bottom: 20px;
  display: flex;
  flex-direction: column;
}
.group-title {
  font-size: 0.75rem;
  color: #666;
  margin-bottom: 8px;
  padding-left: 10px;
}

.nav-menu a {
  color: #aaa;
  text-decoration: none;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 0.9rem;
  margin-bottom: 2px;
  transition: all 0.2s;
}
.nav-menu a:hover {
  background-color: #2a2a2a;
  color: #fff;
}
.nav-menu a.router-link-active {
  background-color: #007acc;
  color: #fff;
  font-weight: bold;
}

/* 右侧主体包装层：垂直排列顶栏和内容 */
.right-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
}

/* 顶部操作栏 */
.top-bar {
  height: 56px;
  background-color: #1a1a1a;
  border-bottom: 1px solid #2a2a2a;
  display: flex;
  align-items: center;
  padding: 0 20px;
  gap: 12px; /* 控制图标与搜索框之间的间距 */
}

.top-bar-left {
  display: flex;
  gap: 8px;
}

.search-box {
  flex: 1; /* 自动拉伸占满剩余空间 */
}

/* 主内容区（扣除顶栏高度后，内部独立滚动） */
.main-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}
</style>
