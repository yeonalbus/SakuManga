<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import ModeToggle from '@/components/ModeToggle.vue'
import OnlineSidebar from '@/components/OnlineSidebar.vue' // 引入两个侧边栏组件
import OfflineSidebar from '@/components/OfflineSidebar.vue' // 引入两个侧边栏组件
import TopBar from '@/components/TopBar.vue' // 引入组合好的顶栏
import GlobalToast from '@/components/common/GlobalToast.vue'
import GlobalModal from '@/components/common/GlobalModal.vue'
import ErrorBoundary from '@/components/common/ErrorBoundary.vue'
import { useTagStore } from '@/stores/tagStore'
import { useUserStore } from '@/stores/userStore'
import { useModeStore } from '@/stores/modeStore'
import { useLayoutMode } from '@/composables/useLayoutMode'

const tagStore = useTagStore()
const userStore = useUserStore()
const modeStore = useModeStore()

// 🍔 窄屏汉堡抽屉状态（<1024px 生效；桌面端侧边栏常驻，汉堡按钮隐藏）
const isSidebarOpen = ref(false)
const closeSidebar = () => {
  isSidebarOpen.value = false
}
const handleResize = () => {
  // 回到宽屏时强制收起抽屉，避免残留遮罩层挡住内容
  if (window.innerWidth >= 1024) isSidebarOpen.value = false
}
onMounted(() => window.addEventListener('resize', handleResize))
onUnmounted(() => window.removeEventListener('resize', handleResize))

// 🖥️ 布局模式：把 styleSettings.layoutMode 解析为有效形态并写到 <html data-layout>（auto/desktop/mobile）
useLayoutMode()

// 📱 搜索栏随滚动显隐（ehentaiviewer 参考）：仅移动形态下（data-layout=mobile），
// main-content 向下滚动收起顶栏、向上滚动呼出；切回桌面形态后 CSS 规则不生效，自动恢复显示
let lastScrollTop = 0
const handleMainScroll = (e: Event) => {
  const el = e.target as HTMLElement
  const st = el.scrollTop
  const movingDown = st > lastScrollTop
  lastScrollTop = st
  const htmlEl = document.documentElement
  if (movingDown && st > 80) htmlEl.classList.add('topbar-hidden')
  else htmlEl.classList.remove('topbar-hidden')
}

onMounted(() => {
  // 🚀 应用启动时异步获取翻译字典
  tagStore.fetchTagDictionary()
})

const route = useRoute()

// 监听路由路径变化，更新模式记忆（模式由全局 modeStore 维护，作为单一数据源）
// 只有进入 /online/* 或 /offline/* 时才更新；
// /downloads、/settings 等页面会保持上一次的模式不变，避免 UI 误判！
watch(
  () => route.path,
  (newPath) => {
    if (newPath.startsWith('/online')) {
      modeStore.setMode('online')
    } else if (newPath.startsWith('/offline')) {
      modeStore.setMode('offline')
    }
  },
  { immediate: true }, // 页面刚加载时立即执行一次
)
</script>

<template>
  <div class="app-layout">
    <GlobalToast />
    <GlobalModal />
  </div>

  <div
    v-if="userStore.isAuthenticated"
    class="app-container"
    :class="{ 'sidebar-open': isSidebarOpen }"
  >
    <!-- 🍔 汉堡按钮（窄屏显示，fixed 悬浮于顶栏左侧；桌面端隐藏） -->
    <button
      class="menu-toggle"
      :class="{ active: isSidebarOpen }"
      aria-label="打开菜单"
      @click="isSidebarOpen = !isSidebarOpen"
    >
      <span class="menu-toggle-bar"></span>
      <span class="menu-toggle-bar"></span>
      <span class="menu-toggle-bar"></span>
    </button>

    <!-- 抽屉遮罩（窄屏抽屉打开时显示，点击关闭） -->
    <div v-if="isSidebarOpen" class="sidebar-overlay" @click="closeSidebar"></div>

    <!-- 左侧导航栏（错误边界包裹：单区渲染错误不影响其他区域） -->
    <ErrorBoundary>
      <aside class="sidebar">
        <div class="logo-area">
          <span class="logo">E-Manager</span>
          <ModeToggle />
        </div>

        <!-- 导航菜单：点击任意链接后自动收起抽屉（窄屏） -->
        <nav class="nav-menu" @click="closeSidebar">
          <!-- 在线/离线菜单，用 v-if / v-else 切换对应的组件 -->
          <OnlineSidebar v-if="modeStore.isOnline" />
          <OfflineSidebar v-else />
          <!-- 全局通用的系统菜单与历史记录：放在侧边栏底部或独立组里 -->
          <div class="nav-group">
            <span class="group-title">⚙️ 系统</span>
            <router-link to="/downloads">下载列表</router-link>
            <router-link to="/settings">系统设置</router-link>
          </div>
        </nav>
      </aside>
    </ErrorBoundary>

    <!-- 2. 右侧主体包装层（包含顶栏 + 内容区） -->
    <div class="right-wrapper">
      <!-- 顶部操作栏 直接调用整合好的顶栏组件（搜索栏在顶栏内，错误边界兜底） -->
      <ErrorBoundary>
        <TopBar />
      </ErrorBoundary>

      <!-- 页面主体显示区 -->
      <ErrorBoundary>
        <main id="main-content" class="main-content" @scroll="handleMainScroll">
          <router-view v-slot="{ Component }">
            <keep-alive>
              <component :is="Component" :key="$route.fullPath" />
            </keep-alive>
          </router-view>
        </main>
      </ErrorBoundary>
    </div>
  </div>

  <!-- 未登录时仅渲染登录视图（路由守卫保证此时只会是 /login） -->
  <router-view v-else />
</template>

<style>
/* 全局主题 CSS 变量：通过 <html data-theme> 切换（见 main.ts applyTheme） */
/* 语义层级（dark 默认值，与现有深色界面视觉一致）：
   bg / surface(卡片) / surface-2(次级卡片) / surface-3(控件) / input
   text-strong / fg(正文) / text-2(次要) / text-3(弱化) / text-muted(更弱)
   border(卡片分隔) / border-2(次级边框) / border-3(控件边框) */
:root {
  --app-bg: #121212;
  --app-fg: #e0e0e0;
  --app-surface: #1a1a1a;
  --app-surface-hover: #2a2a2a;
  --app-border: #2a2a2a;
  --app-accent: #007acc;

  /* 新增：深色/次级背景 */
  --app-bg-deep: #0d0d0f;
  --app-bg-alt: #121214;
  /* 新增：次级表面（卡片/设置项）与第三级表面（按钮/输入框） */
  --app-surface-2: #1a1a1e;
  --app-surface-2-hover: #222226;
  --app-surface-3: #242428;
  --app-surface-3-hover: #2e2e33;
  --app-input-bg: #121214;
  /* 新增：文字层级 */
  --app-text-strong: #ffffff;
  --app-text-2: #aaa;
  --app-text-3: #888;
  --app-text-muted: #666;
  /* 新增：边框层级 */
  --app-border-2: #26262a;
  --app-border-3: #38383e;
}

:root[data-theme='light'] {
  --app-bg: #f5f5f7;
  --app-fg: #1c1c1e;
  --app-surface: #ffffff;
  --app-surface-hover: #ececef;
  --app-border: #e2e2e6;
  --app-accent: #0066b8;

  /* 新增：深色/次级背景（浅色下为浅灰底） */
  --app-bg-deep: #ececef;
  --app-bg-alt: #f0f0f2;
  /* 新增：次级/第三级表面 */
  --app-surface-2: #ffffff;
  --app-surface-2-hover: #f0f0f3;
  --app-surface-3: #f5f5f7;
  --app-surface-3-hover: #e8e8ec;
  --app-input-bg: #ffffff;
  /* 新增：文字层级（浅色下加深，保证可读性） */
  --app-text-strong: #1c1c1e;
  --app-text-2: #55555a;
  --app-text-3: #6b6b72;
  --app-text-muted: #8a8a92;
  /* 新增：边框层级 */
  --app-border-2: #dcdce0;
  --app-border-3: #d0d0d5;
}

/* 响应式断点与 iOS 安全区变量（供全局各组件参考） */
:root {
  /* 断点：<1024 为移动形态（侧边栏收进抽屉，覆盖 iPad 竖屏）；<480 为手机竖屏 */
  --bp-tablet: 1024px;
  --bp-phone: 480px;
  /* 安全区：iOS 刘海屏 / 底部 Home 条；非 iOS 或无安全区时为 0 */
  --safe-top: env(safe-area-inset-top, 0px);
  --safe-bottom: env(safe-area-inset-bottom, 0px);
  --safe-left: env(safe-area-inset-left, 0px);
  --safe-right: env(safe-area-inset-right, 0px);
}

/* 全局基础重置 */
* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}
/* 移动端防误触：禁止双击缩放/长按选择，交给组件自行处理 */
html,
body {
  touch-action: manipulation;
  overscroll-behavior-y: none;
  overflow-x: hidden; /* 兜底防横向溢出（iPad 移动模式等） */
  width: 100%;
}
body {
  background-color: var(--app-bg);
  color: var(--app-fg);
  font-family:
    system-ui,
    -apple-system,
    sans-serif;
}

.app-container {
  display: flex;
  height: 100vh;
  /* 移动端动态视口：避免浏览器地址栏收起/展开导致布局跳动 */
  height: 100dvh;
  width: 100vw;
  overflow: hidden;
}

/* 侧边栏样式 */
.sidebar {
  width: 240px;
  background-color: var(--app-surface);
  border-right: 1px solid var(--app-border);
  display: flex;
  flex-direction: column;
  padding: 20px 10px;
  flex-shrink: 0;
}

/* 让 Logo 标题和按钮在同一行并排显示 */
.logo-area {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 5px 20px 5px;
  border-bottom: 1px solid var(--app-border);
  margin-bottom: 15px;
}

.logo {
  font-size: 1.1rem;
  font-weight: bold;
  color: var(--app-accent);
}

.nav-group {
  margin-bottom: 20px;
  display: flex;
  flex-direction: column;
}
.group-title {
  font-size: 0.75rem;
  color: var(--app-text-muted);
  margin-bottom: 8px;
  padding-left: 10px;
}

.nav-menu a {
  color: var(--app-text-2);
  text-decoration: none;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 0.9rem;
  margin-bottom: 2px;
  transition: all 0.2s;
}
.nav-menu a:hover {
  background-color: var(--app-surface-hover);
  color: var(--app-fg);
}
.nav-menu a.router-link-active {
  background-color: var(--app-accent);
  color: #fff;
  font-weight: bold;
}

/* 右侧主体包装层：垂直排列顶栏和内容 */
.right-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100vh;
  height: 100dvh;
  overflow: hidden;
  min-width: 0; /* 允许内部内容收缩，避免溢出 */
}

/* 顶部操作栏 */
.top-bar {
  height: 56px;
  background-color: var(--app-surface);
  border-bottom: 1px solid var(--app-border);
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
  /* iOS 橡皮筋回弹修复：contain 阻止滚出容器边缘露出 body 背景（底部超大黑框根因） */
  overscroll-behavior-y: contain;
  background-color: var(--app-bg);
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}

/* ─────────────────────────────────────────
   🍔 汉堡按钮：默认隐藏，仅窄屏显示
   ───────────────────────────────────────── */
.menu-toggle {
  display: none;
}
.menu-toggle-bar {
  display: block;
  width: 22px;
  height: 2px;
  background: var(--app-fg);
  border-radius: 2px;
  transition:
    transform 0.3s ease,
    opacity 0.3s ease;
}
.menu-toggle.active .menu-toggle-bar:nth-child(1) {
  transform: translateY(7px) rotate(45deg);
}
.menu-toggle.active .menu-toggle-bar:nth-child(2) {
  opacity: 0;
}
.menu-toggle.active .menu-toggle-bar:nth-child(3) {
  transform: translateY(-7px) rotate(-45deg);
}

/* ─────────────────────────────────────────
   📱 移动形态（<1024px）：侧边栏收进左侧抽屉（覆盖 iPad 竖屏 768-1032）
   ───────────────────────────────────────── */
@media (max-width: 1024px) {
  .menu-toggle {
    display: flex;
    position: fixed;
    top: calc(8px + var(--safe-top));
    left: calc(10px + var(--safe-left));
    z-index: 70;
    width: 40px;
    height: 40px;
    border: none;
    background: transparent;
    cursor: pointer;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    gap: 5px;
    -webkit-tap-highlight-color: transparent;
  }

  /* 侧边栏：从常驻改为 fixed 抽屉，默认藏在屏幕左侧外 */
  .sidebar {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    width: 240px;
    z-index: 65;
    transform: translateX(-100%);
    transition: transform 0.3s ease;
    padding-top: calc(20px + var(--safe-top));
    padding-bottom: calc(20px + var(--safe-bottom));
    box-shadow: 4px 0 16px rgba(0, 0, 0, 0.4);
  }
  .app-container.sidebar-open .sidebar {
    transform: translateX(0);
  }

  /* 抽屉遮罩 */
  .sidebar-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 60;
  }

  .right-wrapper {
    width: 100vw;
    position: relative; /* 让悬浮的 TopBar 相对该容器定位 */
  }

  /* 主内容区减小留白，充分利用屏幕 */
  .main-content {
    padding: 8px;
    /* 顶部补偿悬浮 TopBar（重构后单行搜索栏 ≈ 56px + 安全区）与原有留白 */
    padding-top: calc(56px + var(--safe-top));
    padding-bottom: calc(8px + var(--safe-bottom)); /* 底部 Home 条安全区，滚动到底不贴屏 */
  }
}

/* ─────────────────────────────────────────
   🖥️ 布局模式（<html data-layout>）形态覆盖
   - html[data-layout='mobile']：手动「移动」时，宽视口也应用抽屉侧栏
   - html[data-layout='desktop']：手动「桌面」时，窄视口也保持侧栏常驻（覆盖上方 @media）
   ───────────────────────────────────────── */
html[data-layout='mobile'] .menu-toggle {
  display: flex;
  position: fixed;
  top: calc(8px + var(--safe-top));
  left: calc(10px + var(--safe-left));
  z-index: 70;
  width: 40px;
  height: 40px;
  border: none;
  background: transparent;
  cursor: pointer;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  gap: 5px;
  -webkit-tap-highlight-color: transparent;
}
html[data-layout='mobile'] .sidebar {
  position: fixed;
  top: 0;
  left: 0;
  bottom: 0;
  width: 240px;
  z-index: 65;
  transform: translateX(-100%);
  transition: transform 0.3s ease;
  padding-top: calc(20px + var(--safe-top));
  padding-bottom: calc(20px + var(--safe-bottom));
  box-shadow: 4px 0 16px rgba(0, 0, 0, 0.4);
}
html[data-layout='mobile'] .app-container.sidebar-open .sidebar {
  transform: translateX(0);
}
html[data-layout='mobile'] .sidebar-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: 60;
}
html[data-layout='mobile'] .right-wrapper {
  width: 100vw;
  position: relative; /* 让悬浮的 TopBar 相对该容器定位 */
}
html[data-layout='mobile'] .main-content {
  padding: 8px;
  padding-top: calc(56px + var(--safe-top)); /* 顶部补偿悬浮 TopBar（单行搜索栏） */
  padding-bottom: calc(8px + var(--safe-bottom)); /* 底部 Home 条安全区 */
}

/* 手动强制桌面：窄视口下覆盖 @media 的抽屉形态，保持侧栏常驻 */
html[data-layout='desktop'] .menu-toggle {
  display: none;
}
html[data-layout='desktop'] .sidebar {
  position: static;
  top: auto;
  left: auto;
  bottom: auto;
  z-index: auto;
  width: 240px;
  transform: none;
  padding: 20px 10px;
  box-shadow: none;
}
html[data-layout='desktop'] .app-container.sidebar-open .sidebar {
  transform: none;
}
html[data-layout='desktop'] .sidebar-overlay {
  display: none;
}
html[data-layout='desktop'] .right-wrapper {
  width: auto;
}
html[data-layout='desktop'] .main-content {
  padding: 24px;
}
</style>
