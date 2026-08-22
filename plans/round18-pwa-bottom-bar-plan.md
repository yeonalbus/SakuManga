# Round18 修复 iPad PWA 底部白色窄条（sunpanel 方案：顶部让出 safe-area 下推内容）

## 现象与线索

| 线索 | 结论 |
|------|------|
| 仅 iPad PWA（standalone）出现；桌面/手机/Safari 均无 | iOS PWA 特有 |
| 横竖屏都有、滚动到底部无变化、不影响交互 | 静态孤儿条（安全区残余） |
| 窄条像 Home Indicator 把界面顶上去，随后 Home Indicator 消失只留窄条 | 底部安全区区域未被正确占据 |
| sunpanel / sillytavern 同为 PWA 无此问题，做法是「顶部留出 safe-area 高度 + 内容往下移」 | 修复方向：顶部让位下推，底部不再溢出 |
| 用户确认：方向 = 顶部留出 safe-area + 内容下移；背景色 = var(--app-bg) | 决策已定 |

---

## 根因

- [App.vue](src/App.vue) `.app-container` 用 `position: fixed; inset: 0`（[312-324 行](src/App.vue:312)）：
  - `top:0` 使内容**顶到状态栏下沿**（延伸进状态栏区域）；
  - `bottom:0` 参考的底边是**布局视口**（iOS standalone 下比真实屏幕矮，WebKit bug 313800）→ 容器底部贴不到真实屏幕底 → 屏幕最底部漏出安全区残余白条；
- `fixed inset-0` 把上下都钉死，无法「往下挤」——与 sunpanel 思路冲突。

---

## 修复方案（对齐 sunpanel：顶部让位下推，底部贴底）

**1a. [App.vue](src/App.vue) `.app-container`**：
   - 去掉 `position: fixed; inset: 0`；
   - 改回文档流：`height: 100vh / 100dvh / 100svh`（兜底链保留），`width: 100vw`，`overflow: hidden`；
   - **顶部 `padding-top: var(--safe-top)`**：把整个应用内容（含侧栏 + 顶栏）**往下推到状态栏之下**，让出顶部安全区；
   - 背景色 `background-color: var(--app-bg)`（html 已设，容器也铺上保证安全区同色）。

**1b. [App.vue](src/App.vue) `.right-wrapper`**：
   - 改回 `height: 100%`（在文档流 flex 容器内撑满剩余高度），`min-height: 0` 保留；
   - 不再依赖 fixed 容器。

**1c. [App.vue](src/App.vue) html**：保留 Round17.2 已加的 `background-color: var(--app-bg)`（D1=A，安全区区域同色兜底）。

**1d. 顶部 TopBar 内部**：`.top-bar` 高 56px 不变；由于 app-container 已整体下移 safe-top，TopBar 自然位于状态栏下方。

---

## 涉及文件

| 文件 | 改动 |
|------|------|
| src/App.vue | app-container 去 fixed、加 padding-top safe-top；right-wrapper height:100% |
| scripts/verify-round18.mjs | 代码级断言（无 fixed inset-0、有 padding-top safe-top、html 背景色） |
| plans/round18-pwa-bottom-bar-plan.md | 本计划 |

---

## 验证方案

1. type-check + 构建 + 同步 dist；
2. verify-round18.mjs：断言 app-container 无 position:fixed、有 padding-top: var(--safe-top)、html/body 背景色；
3. 真机复核：iPad PWA 横竖屏底部白条消失、顶部内容从状态栏下方开始。

## 实施结果

- `.app-container`：去掉 `position: fixed; inset: 0`（会把上下钉死，底部贴不到真实屏幕底漏白条）；改文档流 + `padding-top: var(--safe-top)` 顶部让位下推（sunpanel 思路），保留 dvh/svh 兜底；
- `.right-wrapper`：改回 `height: 100%`（文档流 flex 撑满）；
- `.main-content`（mobile 形态）：`padding-top: calc(56px + var(--safe-top))` → `56px`（外层已下移，去重复 safe-top）；
- html/body 已设 `background-color: var(--app-bg)`（D1=A，安全区残余同色兜底）。
- `verify-round18.mjs` 全部通过（去 fixed / 加 safe-top / dvh 兜底 / html 背景色）。

## 决策已确认（用户 2026-08-22）

- 方向：顶部留出 safe-area 高度 + 内容往下移，底部不再溢出；
- 背景色：var(--app-bg)。
