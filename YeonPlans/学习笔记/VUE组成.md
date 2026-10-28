Vue 属于“单页面应用（SPA）” --- 整个网站其实只有一个 index.html，其他所有的 .vue 文件本质上都是在给这个网页拼积木。

### 1. 核心文件的分工

- **App.vue（电视机外壳 / 主舞台）**
  它是整个前端的总根组件。
  比如页面顶部的 Header、底部的 Footer，这些全局固定的东西通常都写在这里。
  最重要的是，它里面放了一个特殊的占位符：<router-view />（相当于电视屏幕）。

- **src/router/index.ts（节目表 / 导播台）**
  它是一个纯配置文件，不包含具体的界面样式。
  它的作用是记录映射关系：“当用户切到哪个频道（URL），就把哪个页面组件放到 App.vue 的 <router-view /> 屏幕上。”

- **views/（具体的电视节目 / 页面级组件）**
  这里放的都是完整的页面（比如在线首页 OnlineHome.vue、设置页 SettingsView.vue）。
  它们受路由调度，只有当网址匹配时，才会整体展示在 App.vue 里。

- **components/（节目里的道具 / 零部件）**
  这里放的是可重复使用的小零件（比如一个自定义的搜索框、一个状态标签、一个漂亮的卡片）。
  这些小零件不能被路由直接跳转，而是被拼进 views/ 里的各个大页面中使用。

### 2. 它们是怎么串联起来的？

```
[用户访问 /online/home]
         │
         ▼
[src/router/index.ts] 收到请求，翻查路由表：
"找到路径 /online/home 对应的是 '@/views/online/OnlineHome.vue'"
         │
         ▼
[views/online/OnlineHome.vue] 被调取加载
（这个页面内部可能引用了 components/Card.vue 零部件）
         │
         ▼
[App.vue] 接收到加载好的页面，填充进自己的 <router-view /> 占位区域
         │
         ▼
【用户在浏览器中看到了完美的界面】
```
