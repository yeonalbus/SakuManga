# Round15 五处 BUG 修复计划

## 背景与问题

| 编号 | 类型 | 内容 |
|------|------|------|
| Bug1 | iPad | PWA 横屏底部黑边（上边栏顶住顶部）；竖屏转横屏可临时消除 |
| Bug2 | 订阅 | 订阅界面实际显示首页内容；EX 站订阅看不到内容（E 站订阅机制为表站功能）|
| Bug3 | iPad | PWA 中点击页面内返回按钮导致页面刷新、状态丢失 |
| Bug4 | 阅读 | 阅读界面（/reader）搜索栏和左上角 logo 错误显示在顶部/左上角 |
| Bug5 | 移动 | 收起键盘后搜索历史栏未正确收起（需切换页面消除） |

---

## 一、Bug1 iPad PWA 横屏底部黑边

### 现状与根因

- `index.html` 已配置 `viewport-fit=cover` + `apple-mobile-web-app-status-bar-style=black-translucent`；
- `App.vue` `.app-container` 用 `height: 100vh; height: 100dvh`（[App.vue:273](src/App.vue:273)），全局 safe-area 变量已定义；
- **iOS PWA 横屏**：`100dvh` 在部分 iPadOS 版本横屏 PWA 下取值偏大（不含刘海安全区偏移），导致内容高度 > 可视区 → 底部露出 body 背景（黑边）；同时 `--safe-top`（左侧刘海 = 横屏时的左侧 insets）未正确应用到内容容器。
- 「竖屏转横屏可消除」：旋转触发 resize + orientationchange 后浏览器重新计算 dvh 才正确。

### 修复

**1a. [App.vue](src/App.vue)** `.app-container` 高度兜底链：`100vh` → `100dvh` → `100svh`，并在横屏时用 `min-height: -webkit-fill-available` 兜底（iOS PWA 经典问题）；
**1b. [App.vue](src/App.vue)** `orientationchange` 监听：旋转后强制触发一次重布局（读 `window.innerHeight` 触发 reflow）并刷新 `--safe-top/bottom/left/right`（iOS 横屏 safe-area 变化）；
**1c. [index.html](index.html)** 保留现状（viewport-fit 已正确）；若 1a/1b 不足，增加 `apple-mobile-web-app-status-bar-style=default` 备选（黑边也常与 black-translucent 状态栏重叠有关）。

---

## 二、Bug2 订阅界面显示问题（固定指向 e-hentai）

### 现状与根因

- 订阅（Watched）是 **E 站表站（e-hentai.org）功能**；EX 站（exhentai.org）的 /watched 无内容（返回首页或空）；
- [eh_auth.go:20](backend/internal/services/eh_auth.go:20) `GetBaseURL`：账号 `IsEx=true` + 设置 Site=exhentai 时返回 `https://exhentai.org/` → `FetchWatchedList`（[eh_sub.go:31](backend/internal/services/eh_sub.go:31)）用 EX 域名请求 /watched → 无内容；
- 用户思路确认：**订阅固定指向 e-hentai 表站**（登录 Cookie 是 ex 账号，访问表站 /watched 仍能通过 Cookie 认证读取订阅列表）。

### 修复

**2a. [eh_sub.go](backend/internal/services/eh_sub.go)** `FetchWatchedList` 不再用 `GetBaseURL`，**固定硬编码 `https://e-hentai.org/`** 拼接 /watched；
**2b. [handler 层确认]** `GetWatchedComics` 无其他 baseURL 依赖；
**2c. 说明**：订阅 Cookie 用 ex 账号（sk/igneous 等）访问表站 /watched 是 E 站允许的（表站需要 ex 权限查看部分内容），返回内容即该账号订阅列表。

---

## 三、Bug3 iPad PWA 返回按钮导致刷新

### 现状与根因

- 离线卡片点击 → `openComicDetailInNewTab`（[detailNav.ts](src/utils/detailNav.ts)）用 `window.open` 打开详情，返回语义依赖：
  1. `window.opener` 判定；
  2. `sessionStorage` 标记 `saku_newtab_<id>`（[detailNav.ts:50](src/utils/detailNav.ts:50)）。
- **iPad PWA 独立窗口**：`window.open` 在 PWA standalone 模式下**行为受限/不可用**（iOS PWA 不支持新窗口，实际是当前窗口导航或直接失败）→ 详情在新标签打开时 sessionStorage 不与父窗口共享（PWA 单窗口场景下 `markComicOpenedInNewTab` 写入的标记在新导航中不保留）；
- 返回时 `window.opener=null`、`consumeBackState` 未命中、`isDetailNewTab` 未命中 → 走 `window.history.length > 1 ? router.back() : push`；PWA 下 history 栈异常（详情是新窗口导航、父列表不在栈内）→ `router.back()` 跳到空白/触发整页刷新，keep-alive 状态（滚动/页码）丢失。

### 修复（PWA 检测 + 显式返回来源）

**3a. [utils/detailNav.ts](src/utils/detailNav.ts)** 新增 PWA 检测 `isStandalonePWA()`（`window.matchMedia("(display-mode: standalone)")` 或 `navigator.standalone === true`）；
**3b. PWA 下不使用 `window.open`**：`openComicDetailInNewTab` 改为「同标签路由跳转 + sessionStorage/内存记录来源列表状态」（`saku_back_<id>` 已存在，可直接复用 recordBackState），详情返回走 `consumeBackState → rememberListState → router.replace(来源)`，不依赖 opener/新标签标记；
**3c. [OnlineDetail/OfflineDetail handleBack](src/views/online/OnlineDetail.vue:367)** 返回链路优先 `consumeBackState`（已有），并补 PWA 分支：standalone 且无 backState 时 `router.push(当前模式首页)` 而非 `router.back()`；
**3d. 回归**：桌面新标签打开详情（window.open）行为不变（非 standalone 走原逻辑）。

---

## 四、Bug4 阅读界面显示搜索栏和 logo

### 现状与根因

- App.vue 全局渲染侧栏（含 logo）与 TopBar（含搜索栏），`/reader` 路由不豁免（[App.vue:85](src/App.vue:85)）；
- ComicReader 进入时只控制自身内部控件显隐（`showControls`），**未隐藏 App 级 TopBar/侧栏**；
- 阅读器是沉浸式全屏，全局 TopBar（搜索栏）叠在顶部、侧栏 logo 叠在左上角 → 显示错误。

### 修复（阅读器路由隐藏全局外壳）

**4a. [App.vue](src/App.vue)** 用 `useRoute()` 判断 `route.path === "/reader"`：
- 阅读器时隐藏 `.top-bar` 与 `.sidebar`（`v-show` 或条件 class），并给 `#main-content` 加全屏 class（padding 归零、无 TopBar 占位）；
- 顶部 TopBar 高度补偿：`main-content` 的 `padding-top`（移动形态 `calc(56px + safe-top)`）在阅读器下置 0；
**4b. 保留阅读器自己的退出/返回按钮**（ComicReader 顶部控制条已有）。

---

## 五、Bug5 移动端搜索历史收起逻辑

### 现状与根因

- 搜索历史/联想面板 `v-if="isFocused && ..."`（[SearchBar.vue:348](src/components/SearchBar.vue:348)）；
- `isFocused` 只在 `@focus`（聚焦）与 `handleOutsideClick`（点击外部，[SearchBar.vue:251](src/components/SearchBar.vue:251)）改变；
- **iOS 点键盘「收起」按钮**：不触发 input blur、也不触发 click 外部 → `isFocused` 保持 true → 面板不收起。

### 修复

**5a. [SearchBar.vue](src/components/SearchBar.vue)** 增加键盘收起监听：
- `onblur` 事件补挂到 input（`@blur="isFocused = false"`，延迟 ~120ms 让联想点击的 mousedown 先触发，避免误关）；
- **iOS 视觉视口监听**：`window.visualViewport` 高度变化（键盘弹起/收起）时，若高度恢复为完整视口（键盘已收起）→ `isFocused = false`；
**5b. 兜底**：`@touchend` 点击面板外部区域（mask 或页面）同样关闭。

---

## 六、涉及文件

| 文件 | 改动 |
|------|------|
| `src/App.vue` | Bug1 dvh 兜底 + orientationchange + Bug4 阅读器隐藏外壳 |
| `index.html` | Bug1 备选状态栏样式（按需） |
| `backend/internal/services/eh_sub.go` | Bug2 订阅固定 e-hentai 表站 |
| `src/utils/detailNav.ts` | Bug3 PWA 检测 + 同标签返回来源记录 |
| `src/views/online/OnlineDetail.vue` / `offline/OfflineDetail.vue` | Bug3 返回链路 PWA 分支 |
| `src/components/SearchBar.vue` | Bug5 blur + visualViewport 监听收起 |
| `scripts/verify-round15.mjs` | 新增验证脚本 |
| `plans/round15-five-bugs-plan.md` | 本计划 |

---

## 七、决策点（请确认）

- **D1 Bug1 修复力度**：
  - A（推荐）：CSS dvh/svh 兜底 + orientationchange 重算 safe-area（轻量、覆盖常见场景）；
  - B：A + 引入动态视口 polyfill（visualViewport 监听全量重算，更稳但代码多）。
- **D2 Bug2 订阅固定表站**：
  - A（推荐）：无条件固定 e-hentai（订阅本就有表站语义）；
  - B：仅当当前 Site=exhentai 且账号 IsEx 时强制表站（其他场景仍按设置）。
- **D3 Bug3 PWA 返回**：
  - A（推荐）：PWA 下离线/在线卡片一律「同标签路由跳转 + 记录来源」（返回不刷新、恢复滚动页码）；
  - B：仅修复离线（在线窄屏 PWA 少见）。
- **D4 Bug4 阅读器隐藏外壳**：
  - A（推荐）：/reader 路由隐藏 TopBar + 侧栏 + 移动形态 padding 归零；
  - B：仅隐藏 TopBar（侧栏保留，桌面仍可切导航）。
- **D5 Bug5 收起时机**：
  - A（推荐）：blur + visualViewport 高度恢复双保险；
  - B：仅 blur（实现最简，但 iOS 点键盘收起不触发 blur 时无效）。

---

## 八、验证方案

1. `go test ./...`（订阅 baseURL 逻辑若有单测更新）；`npm run type-check` + 构建 + 同步 dist；
2. `scripts/verify-round15.mjs`：
   - Bug2：mock 请求断言 /watched 请求 URL 固定 `https://e-hentai.org/watched`（不随 IsEx 变 exhentai）；
   - Bug4：进入 /reader 断言 `.top-bar`/`.sidebar` 不可见、main-content 无 padding；
   - Bug5：聚焦搜索框（isFocused=true）→ 模拟 blur / visualViewport 缩小后恢复 → 断言 `.search-dropdown` 隐藏；
   - Bug3：PWA 场景（context 模拟 standalone）→ 卡片点击详情 → 返回 → 断言回到列表且 scrollTop/页码保留（代码级 + DOM 级）；
   - Bug1：桌面无法真机复现 iPad，做代码级断言（dvh 兜底样式存在 + orientationchange 监听已挂）。
3. 回归：Round12/13/14 验证脚本复跑。

---

## 九、提交计划（按模块拆分）

1. `fix(backend): 订阅固定 e-hentai 表站`；
2. `fix(frontend): iPad PWA 横屏黑边/返回刷新/阅读器外壳/搜索历史收起`；
3. `chore(build): 同步内嵌前端构建产物到 backend/webui/dist`。

## 决策确认（用户已确认 2026-08-21）

- D1=A、D2=A、D3=A、D4=A、D5=A（均为推荐方案）。

## 实施结果

- Bug1：App.vue `.app-container`/`.right-wrapper` 加 `100svh` 兜底 + `orientationchange` 重算 safe-area（--vp-h 触发 reflow）；
- Bug2：`eh_sub.go` FetchWatchedList 固定 `https://e-hentai.org/`（不再随 IsEx 走 exhentai）；
- Bug3：`detailNav.ts` 新增 `isStandalonePWA()`；PWA 下 `openComicDetailInNewTab` 改同标签导航；两个详情页 handleBack 加 PWA 分支（回首页而非 router.back）；
- Bug4：App.vue 阅读器路由隐藏 TopBar/侧栏 + `main-content.reader-fullscreen`（padding 归零、黑底）；
- Bug5：SearchBar 增加 blur（延迟 120ms 防误关）+ visualViewport 高度恢复检测收起搜索历史。
- `scripts/verify-round15.mjs` 全部通过（Bug4/5 DOM 级、Bug3/1 代码级）。

## 待确认后再开工

请确认 D1~D5 后开始实现。
