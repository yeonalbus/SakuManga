# Round18.3 修复 iPad PWA 底部条（对照 sun-panel：移除 #app 的 min-height:100dvh）

## 根因（对照 sun-panel 源码确认）

### sun-panel 的布局（[global.less](refs/sunpanel/src__styles__global.less) + [login/index.vue](refs/sunpanel/src__views__login__index.vue)）：
```
html, body, #app { height: 100%; }          // 纯百分比链，无 dvh/svh
body { padding-bottom: env(safe-area-inset-bottom); }
.login-container { height: 100vh; }        // 子元素用 100vh
```

### 我们的差异：
- `#app { height: 100%; min-height: 100dvh; }`  ← **有 min-height:100dvh**（Round17.2 加的）；
- `.app-container { height: 100dvh; 100svh; 100vh; }`；
- 无 `body { padding-bottom: env(safe-area-inset-bottom) }`。

### 推断根因：
- iOS PWA standalone 下 **`100dvh` 偏小**（WebKit bug 313800），`#app` 的 `min-height: 100dvh` 会把整个高度链**锚定在偏小的 dvh** 上；
- 即使 `.app-container` 用 `100vh`（= 完整屏），`#app` 作为父级若被 min-height 限制/或百分比与 vh 混用冲突，容器仍到不了屏幕底；
- sun-panel **全程无 dvh/svh**，纯 100% 链 + 100vh，故正常。

---

## 修复方案（完全对齐 sun-panel）

**1a. [App.vue](src/App.vue) `#app`**：移除 `min-height: 100dvh`，保留 `height: 100%`；
**1b. `html, body`**：确认 `height: 100%`（已有）；
**1c. `.app-container`**：高度仅保留 `100vh`（移除 dvh/svh 混用），保留 `padding-top: var(--safe-top)` 与背景色；
**1d. `body`**：加 `padding-bottom: env(safe-area-inset-bottom)`（sun-panel 同款，底部安全区由 body 让出，而非容器）；
**1e.** 若 100% 链仍不贴底，则按用户上次同意的方向用 `visualViewport.height` JS 动态撑高兜底。

---

## 涉及文件

| 文件 | 改动 |
|------|------|
| src/App.vue | #app 去 min-height:100dvh；app-container 纯 100vh；body 加 padding-bottom safe-area |
| refs/sunpanel/ | sun-panel 参考源码（已拉取） |
| scripts/verify-round18.mjs | 断言 #app 无 min-height:100dvh、app-container 纯 100vh、body 有 padding-bottom safe-area |
| plans/round18-pwa-bottom-bar-plan.md | 追加本方案 |

## 验证方案

1. type-check + 构建 + 同步 dist；
2. verify-round18.mjs：新断言全过；
3. 真机复核：iPad PWA 竖/横屏底部条消失。
