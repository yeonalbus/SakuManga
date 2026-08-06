<script setup lang="ts">
// 🎯 重构后 TopBar 只剩单行搜索栏：
// - 骰子（手气不错）→ /random（侧边栏「🎲 工具」入口）
// - 阅读清单 → /reading-list（侧边栏「🎲 工具」入口）
// - 筛选入口 → 移入 SearchBar 搜索框内（FilterDrawer）
import SearchBar from './SearchBar.vue'
</script>

<template>
  <header class="top-bar">
    <div class="search-wrapper">
      <SearchBar />
    </div>
  </header>
</template>

<style scoped>
.top-bar {
  height: 56px;
  background-color: var(--app-surface);
  border-bottom: 1px solid var(--app-border);
  display: flex;
  align-items: center;
  padding: 0 16px;
  gap: 12px; /* 内部零件之间的间距 */
}

/* 搜索栏外壳，自动吸收剩余宽度 */
.search-wrapper {
  flex: 1;
  display: flex;
  justify-content: center;
}

/* 📱 移动形态（<1024px）：单行搜索栏，适配 iOS 安全区 */
@media (max-width: 1024px) {
  .top-bar {
    height: auto;
    min-height: calc(48px + var(--safe-top));
    padding-top: calc(4px + var(--safe-top));
    padding-right: calc(12px + var(--safe-right));
    padding-bottom: 4px;
  }
}

/* 🖥️ 移动形态：给汉堡按钮让位（仅移动布局；桌面形态侧栏常驻、无汉堡，无需让位） */
/* ⚠️ :global() 必须包裹完整选择器（含子类名），否则 scoped 编译会丢弃类名、规则直接作用在 <html> 上（曾导致整个页面隐藏/平移出视口白屏） */
:global(html[data-layout='mobile'] .top-bar) {
  /* 🖥️ 移动形态（含宽视口手动移动）下悬浮覆盖在内容上方：
     必须同时补偿 iOS 安全区（safe-top/safe-right），且高度自适（height:auto），
     使顶栏总高 = 48px + safe-top，与 App.vue 的 main-content padding-top:
     calc(56px + var(--safe-top)) 严格匹配，避免详情页操作栏/返回钮被顶进 Header 覆盖区 */
  height: auto;
  min-height: calc(48px + var(--safe-top));
  padding-top: calc(4px + var(--safe-top));
  padding-bottom: 4px;
  padding-left: calc(56px + var(--safe-left));
  padding-right: calc(12px + var(--safe-right));
  /* 搜索栏随滚动显隐：悬浮覆盖在内容上方（配合 App.vue 的 main-content 顶部补偿） */
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 50;
  transition: transform 0.25s ease;
}
/* 滚动隐藏：仅移动形态下，向下滚动收起顶栏（App.vue 在 main-content 滚动时切换 html.topbar-hidden） */
:global(html[data-layout='mobile'].topbar-hidden .top-bar) {
  transform: translateY(-100%);
}
</style>
