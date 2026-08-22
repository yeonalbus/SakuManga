# Round18.2 修复 iPad PWA 底部截断条（改用 100vh 全屏 + 背景铺满）

## 现象与根因（用户两张截图确认）

| 现象 | 结论 |
|------|------|
| 底部窄条 = 纯背景色空白，无边框 | 是容器高度不足露出的背景，非元素 |
| 窄条横向略超内容区，截断卡片网格 | 容器底部到不了真实屏幕底 |
| 深色/浅色主题都有 | 与颜色无关，是布局高度问题 |
| 顶部无空隙，底部却有一条 | 顶部贴近（safe-top 生效），底部 dvh 算小 |

### 根因：dvh/百分号被偏小的布局视口钳制

- `.app-container` 用 `height: 100dvh / 100svh`，但父链 `#app`、`html/body` 都是 `height: 100%`；
- **iPad PWA standalone 下，`html/body` 的 100% 与 `#app` 的 100% 基于「布局视口」**（WebKit bug 313800：比真实屏幕矮一截），`.app-container` 的 `100dvh` 被这个偏小的父级钳制 → 容器高度 < 真实屏幕 → 底部漏出背景条；
- `position: fixed; inset: 0`（Round17.2）同样贴布局视口 ≠ 真实屏幕；
- **`100vh` 在 iOS standalone 下 = 完整屏幕（含状态栏）**，比 `100dvh`/`svh` 更接近真实屏幕——sunpanel 全屏蓝图正是如此。

---

## 修复方案

**1a. [App.vue](src/App.vue) `.app-container`**：
   - 去掉 `padding-top: var(--safe-top)`（竖屏 safe-top 用 padding 下推反而在 dvh 偏小时把顶部也漏出，且 iPad 竖屏无刘海 safe-top 本就 ≈0）；
   - `height: 100vh` 作为**首要单位**（iOS standalone 下等于完整屏），`100dvh`/`100svh` 仅作兜底；
   - 背景 `background-color: var(--app-bg)`（已设，保留，安全区区域同色）；
   - 保留文档流（非 fixed）。

**1b. `.main-content`**：`padding: 24px` 保留；滚动 `overflow-y: auto` 保留（卡片网格可滚动）。

**1c. html/body/#app**：保留 `height: 100%` + `background-color: var(--app-bg)`（铺满到状态栏/安全区）。

---

## 涉及文件

| 文件 | 改动 |
|------|------|
| src/App.vue | app-container：去 padding-top，height 首用 100vh |
| scripts/verify-round18.mjs | 断言 app-container 无 padding-top、height 含 100vh 首项 |
| plans/round18-pwa-bottom-bar-plan.md | 追加本方案 |

---

## 验证方案

1. type-check + 构建 + 同步 dist；
2. verify-round18.mjs：断言容器无 padding-top、height:100vh 优先；
3. 真机复核：iPad PWA 竖/横屏底部截断条消失、内容铺满。

## Round18.2 实施结果

- 用户确认：保留 padding-top（防误触下拉通知栏）+ 同意 100vh 方案；
- `.app-container`：`100vh` 最后声明（最终生效 = iOS standalone 完整屏），dvh/svh 前置兜底；padding-top 保留；
- 构建产物确认 `.app-container{height:100vh;...}` 生效；
- `verify-round18.mjs` 全部通过（去 fixed / padding-top 保留 / 100vh 最后 / html 背景色）。

## 待确认

- 方案核心 = 用 iOS standalone 下等于完整屏的 `100vh` 替代被偏小布局视口钳制的 `100dvh`。
- 若 100vh 仍出现底部条，则退一步用 `visualViewport.height` JS 动态撑高（DEV 文章验证的 display 翻转重算技巧）。
