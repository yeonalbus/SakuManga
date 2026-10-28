结构就像一套“三明治”，主要分为三大块：

- **script** 逻辑与数据（大脑）
- **template HTML** 界面结构（骨架）
- **style CSS** 视觉外观（皮肤）

### 第一块：script setup lang="ts"（逻辑层）

<script setup lang="ts">
// 在 Vue 3 setup 中，导入组件后即可直接在 template 里当标签使用
import ModeToggle from '@/components/ModeToggle.vue'
</script>

- **setup：**
  这是 Vue 3 的现代语法糖。
  加了这个关键字后，你在里面定义的变量、导入的组件，不需要手写繁琐的声明，可以直接在下方的 <template> 里使用。

- **lang="ts"：**
  表示这里写的是 TypeScript 代码，而不是普通的 JavaScript。

- **import ModeToggle ...：**
  `引入组件`
  从 @/components/ModeToggle.vue 引入了一个叫 ModeToggle 的子组件（按钮/零件）。

### 第二块：script setup lang="ts"（逻辑层）

<template>
  <div class="app-container"> <!-- 最外层容器 -->

    <!-- 左侧：固定不动的侧边栏 -->
    <aside class="sidebar">
      <div class="logo-area">
        <span class="logo">E-Manager</span>
        <!-- 这里调用了刚才在 script 里 import 进来的零件组件 -->
        <ModeToggle />
      </div>

      <!-- 导航菜单 -->
      <nav class="nav-menu">
        <div class="nav-group">
          <span class="group-title">🌐 在线模式</span>
          <!-- router-link 是 Vue 专用的跳转标签，点击后更新 URL 路径 -->
          <router-link to="/online/home">首页</router-link>
          <router-link to="/online/favorites">我的收藏</router-link>
        </div>
        <!-- 其余导航组省略... -->
      </nav>
    </aside>

    <!-- 右侧：动态变化的页面主体内容 -->
    <main class="main-content">
      <!-- 核心点：这个标签就是路由占位符（屏幕） -->
      <router-view></router-view>
    </main>

  </div>
</template>

- **<ModeToggle/>：**
  这就是在调用 components/ModeToggle.vue。
  Vue 允许你像写原生 HTML 标签一样调用自定义组件。

- **<router-link to="...">：**
  它最终会被渲染成 HTML 的 <a> 标签，但点击它时不会让浏览器刷新整个页面，而是通过代码切换路由。

- **<router-view></router-view>：**
  这是最核心的区域。
  无论你怎么点击左侧的导航，左侧栏是不动的，改变的只有 <router-view> 里面的内容。
  你访问 /online/home 时，这里就会把 OnlineHome.vue 渲染进来。

### 第三块：<style>（样式层）

- **左右两栏布局实现：**
  CSS
  .app-container {
  display: flex; /* 使用弹性盒模型，让左右侧边栏和主区域并排 _/
  height: 100vh; /_ 占满整个屏幕高度 _/
  width: 100vw;
  }
  .main-content {
  flex: 1; /_ 让右侧主区域自动撑满剩余宽度 _/
  overflow-y: auto; /_ 当内容超出时在右侧区域内滚动，不影响侧边栏 */
  }

- **自动高亮效果的实现（回应我们前面讨论的逻辑）：**
  CSS
  .nav-menu a.router-link-active {
  background-color: #007acc; /* 设为蓝色背景 */
  color: #fff;
  font-weight: bold;
  }
  Vue Router 会在用户处于某个页面时，自动给对应链接加上 .router-link-active 类名。
  你在 CSS 里定义了这个类名的颜色是蓝色（#007acc），高亮效果就是这样自动触发的。
