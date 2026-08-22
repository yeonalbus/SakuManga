# Round19 阅读界面颜色跟随主题（深色→深色，浅色→浅色）

## 现状问题

阅读器 src/views/ComicReader.vue 的 <style scoped> 全部使用**硬编码深色值**，与全局主题（<html data-theme> + CSS 变量）完全脱钩：

| 硬编码值 | 用途 |
|---------|------|
| #0d0d0f | 阅读器画布底色（图片间隙） |
| rgba(18,18,22,.92) | 浮动顶栏/底栏背景 |
| #2d2d32 | 浮动条 / 抽屉 / 缩略图条边框 |
| #242428 / #3a3a3d / #38383c | 返回按钮 / 控件按钮 / 下拉框 |
| #18181c / #eee | 阅读设置抽屉背景 / 文字 |
| #000 | 图片容器 / 加载占位 / webtoon 条目背景 |
| #aaa / #888 / #666 | 状态文字层级 |
| #121214 / #1a1a1e→#0d0d0f | 缩略图条底色 / 占位渐变 |
| #007acc | 强调色（accent） |

→ 无论用户主题设成浅色还是深色，阅读器恒为深色。

## 方案：阅读器配色全部接回主题 CSS 变量

App.vue 全局变量已按 data-theme 提供明暗两套 --app-*（bg-deep / surface-2 / surface-3 / border-2 / border-3 / text-* / accent 等），
其中多数与阅读器现有深色值同值（如 --app-bg-deep:#0d0d0f、--app-surface-3:#242428、--app-text-2:#aaa）。

因此策略：**能复用 --app-* 的直接复用；仅有几处无对应变量（且两主题值不同）的补专用 --reader-* 变量**：

### App.vue 新增（:root 深色默认 + :root[data-theme='light'] 覆盖）

| 新变量 | 深色 | 浅色 | 用途 |
|--------|------|------|------|
| --reader-page-bg | #000 | #fff | 图片容器 / 加载占位底色（浅色=白纸阅读感） |
| --reader-bar-bg | rgba(18,18,22,.92) | rgba(255,255,255,.92) | 浮动顶/底栏（保留毛玻璃） |
| --reader-thumb-bg | rgba(14,14,17,.94) | rgba(245,245,247,.94) | 缩略图进度条 |
| --reader-img-shadow | 0 0 20px rgba(0,0,0,.8) | 0 0 20px rgba(0,0,0,.15) | 页图投影（浅色下柔和） |
| --reader-reveal-bg | rgba(0,0,0,.55) | rgba(255,255,255,.85) | 悬浮呼出按钮 ⋯ 底色 |
| --reader-reveal-color | #ddd | #333 | 悬浮呼出按钮文字 |
| --reader-reveal-border | rgba(255,255,255,.25) | rgba(0,0,0,.15) | 悬浮呼出按钮边框 |
| --reader-reveal-hover | rgba(0,0,0,.75) | rgba(255,255,255,.95) | 悬浮呼出按钮 hover |

### ComicReader.vue 替换映射（完整清单见实施 diff）

- 画布 #0d0d0f → var(--app-bg-deep)
- 浮动条背景 → var(--reader-bar-bg)；边框 #2d2d32 → var(--app-border-2)
- 返回/控件按钮 #242428/#3a3a3d/#38383c → var(--app-surface-3)/var(--app-border-3)
- 抽屉 #18181c/边框/#eee → var(--app-surface-2)/var(--app-border-2)/var(--app-fg)
- 图片容器/占位/webtoon 条目 #000 → 容器 var(--reader-page-bg)，webtoon 条目 var(--app-bg-deep)
- 文字层级 #aaa/#888/#666 → var(--app-text-2)/var(--app-text-3)/var(--app-text-muted)
- 强调 #007acc → var(--app-accent)；分隔线 #2a2a2d → var(--app-border)
- 缩略图条 → var(--reader-thumb-bg)；条目底 #121214 → var(--app-bg-alt)；占位渐变 → var(--app-surface)→var(--app-bg-deep)；滚动条 → var(--app-border-3)
- 页图投影 → var(--reader-img-shadow)；呼出按钮 → --reader-reveal-*
- 加载转圈 #2a2a2f → var(--app-border-3)（顶部色 #7aa2f7 保留）

**保持不变的语义色**（两主题下都成立）：
- 亮度滤镜 overlay #000（降亮度用，始终黑）
- 缩略图数字角标 / 进度 chip 的深色半透明底（压在图片上需要对比度）
- 手柄连接绿 #4ade80
- .control-btn.active 强调蓝底白字（accent 两主题均为蓝）

### App.vue 附带调整

- .main-content.reader-fullscreen 的 background-color:#000 → var(--app-bg)（阅读器固定定位铺满，仅过渡/边缘时可见，跟随主题更一致）

## 涉及文件

| 文件 | 改动 |
|------|------|
| src/App.vue | 新增 --reader-* 变量（明暗两套）；reader-fullscreen 背景接主题 |
| src/views/ComicReader.vue | 硬编码深色全部替换为主题变量 |
| scripts/verify-round19.mjs | 断言：ComicReader 无硬编码深色、App.vue 含 --reader-* 明暗两套 |
| plans/round19-reader-theme-plan.md | 本计划 |

## 验证方案

1. npm run type-check + npm run build-only；
2. node scripts/verify-round19.mjs 全过；
3. 本地起服务截图（Playwright 注入 data-theme=light / dark）对比阅读器底色；
4. 同步 dist + git 提交。
