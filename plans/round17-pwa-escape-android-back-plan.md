# Round17 修复 iOS PWA 顶栏逃逸 + Android 返回按钮回主界面

## 背景与问题

| 编号 | 类型 | 内容 |
|------|------|------|
| Bug1 | iPad | 点击侧栏选项卡（在线模式/工具/系统 各组链接）时出现 Safari 顶栏（关闭/返回/字体/URL/分享/刷新/浏览器打开）+ 底部窄条——PWA 未以 standalone 启动或点击链接逃逸 |
| Bug3 | Android | SakuHentai 返回按钮 → 回到主界面（应回抽卡/书架原界面）；手机返回手势正常；状态保留（keep-alive 在） |

---

## 根因分析（代码确认 + 参考源码确认）

### Bug1：manifest 缺 start_url/scope + iOS PWA 链接逃逸

- **manifest 缺 `start_url` 和 `scope`**（[public/site.webmanifest](public/site.webmanifest) 只有 name/short_name/icons/theme/background/display）。iOS 对 PWA 识别依赖 `start_url`：缺失时从主屏幕打开可能**不以 standalone 启动**，而是 Safari 标签页模式（顶栏常驻，就是用户看到的）；
- iOS standalone 下**点击 `<a>` 链接（Vue Router router-link 底层就是 `<a href>`）可能触发原生导航逃逸**——WebKit 已知行为（参考 amux 修复：PWA 下 `location.href` 静默失败、需 `<a target="_blank">` 走 SFSafariViewController）；
- 用户「竖屏转横屏可消除」的旧观察 + 现在「点击侧栏出现顶栏」都符合「PWA 未真正 standalone / 中途逃逸」。

### Bug3：backState 在 scrollTop=0 时不写入 → 返回走 PWA 分支回首页

- `recordBackStateForDetail`（[detailNav.ts:132](src/utils/detailNav.ts:132)）依赖 `captureActiveListState()`；
- `captureActiveListState`（[scrollMemory.ts](src/utils/scrollMemory.ts)）在**未注册 provider 且 scrollTop=0** 时返回 `undefined`（`top > 0` 才返回）；
  - RandomView（抽卡）不注册 provider；抽卡结果在顶部未滚动 → top=0 → backState 不写入；
  - 书架页若在顶部同样不写入；
- 详情页 handleBack：`consumeBackState` 未命中 → PWA 分支 `router.push(首页)` → 回主界面；
- 手机返回手势 = `history.back()`（SPA 历史栈还在）→ 正常回原界面。

---

## 修复方案

### 一、Bug1：manifest 补全 + 防链接逃逸

**1a. [public/site.webmanifest](public/site.webmanifest)** 补全：
   - `"start_url": "/"`（iOS standalone 识别关键）；
   - `"scope": "/"`；
   - `"id": "/"`（Android 安装标识）；
   - `"display_override": ["standalone", "minimal-ui"]`（渐进增强）；
   - 图标补 `"purpose": "any maskable"` 兼容。

**1b. 全局防逃逸**（[main.ts](src/main.ts) 或 [App.vue](src/App.vue)）：
   - `document.addEventListener("click")` 捕获阶段拦截：PWA（standalone）下对**同源 `<a href>` 且非 target=_blank** 的点击 `preventDefault()` + 用 Vue Router 导航（router-link 本就走 SPA，此拦截兜底原生 `<a>` 与 iOS 链接捕获行为）；
   - 真正需要外部打开的场景（如 AboutSettings 外链）保持 `<a target="_blank" rel="noopener">`——iOS 会走 SFSafariViewController 应用内浏览器，不逃逸；
   - 移除/改造剩余的 `window.open`（TagChip 在线搜索、SearchBar 直链、OfflineDetail 跳在线）为 SPA 或 target=_blank。

### 二、Bug3：backState 无条件写入 + 返回用 history.back

**2a. [detailNav.ts](src/utils/detailNav.ts)** `recordBackStateForDetail`：
   - **不再依赖 `captureActiveListState` 的返回值**——无条件写入 `{ fromPath: window.location.pathname, top: listState?.top ?? 0, page: listState?.page }`；
   - 保证每次 SPA 跳转都记录来源路径（即使未滚动），返回至少回到来源页。

**2b. [OnlineDetail/OfflineDetail handleBack](src/views/online/OnlineDetail.vue)** PWA 分支：
   - 不再 `router.push(首页)`，改为 `if (window.history.length > 1) router.back() else router.push(首页)`（与手机返回一致，SPA 历史栈正常）；
   - backState 命中仍优先（恢复滚动/页码）。

**2c. [ItemCard.vue](src/components/ItemCard.vue) 在线分支**（[195-205 行](src/components/ItemCard.vue:195)）：
   - 在线卡片 `router.push` 前也调 `recordBackStateForDetail`（当前在线详情返回无来源记录）。

### 三、辅助：页面注册列表状态 provider

**3a. [RandomView.vue](src/views/RandomView.vue)**：注册 `setListStateProvider("/random", ...)`，使抽卡界面返回时恢复滚动/页码（与列表页一致）；
**3b. 书架页已有 provider**（OfflineBookshelf 注册过），确认即可。

---

## 涉及文件

| 文件 | 改动 |
|------|------|
| public/site.webmanifest | 补 start_url/scope/id/display_override/icons purpose |
| src/main.ts 或 App.vue | 全局捕获拦截同源 a 导航（PWA） |
| src/utils/detailNav.ts | recordBackStateForDetail 无条件写入 |
| src/components/ItemCard.vue | 在线分支补 backState 记录 |
| src/views/online/OnlineDetail.vue / offline/OfflineDetail.vue | PWA 分支改 history.back() |
| src/views/RandomView.vue | 注册 listStateProvider |
| src/components/TagChip.vue / SearchBar.vue / settings/AboutSettings.vue / OfflineDetail.vue | window.open 改 SPA 或 target=_blank |
| scripts/verify-round17.mjs | 新增验证脚本 |
| plans/round17-pwa-escape-android-back-plan.md | 本计划 |

---

## 决策点（请确认）

- **D1 manifest 范围**：
  - A（推荐）：补 start_url=/ scope=/ id=/ display_override，完整 PWA 识别（iPad 需重新添加到主屏幕才生效，旧图标需删除重加）；
  - B：仅补 start_url/scope（最小改动）。
- **D2 全局 a 拦截**：
  - A（推荐）：全局捕获拦截同源 a（PWA 下 preventDefault + router 导航），最稳防逃逸；
  - B：只改 manifest + 剩余 window.open（不拦截 a，赌 router-link 不逃逸）。
- **D3 返回策略**：
  - A（推荐）：backState 优先 + history.back() 兜底（与手机返回一致，Android 已验证）；
  - B：backState 优先 + 首页兜底（现状，但 backState 修复后其实已够，只是极端场景回首页）。

---

## 验证方案

1. type-check + 构建 + 同步 dist；
2. scripts/verify-round17.mjs：
   - Bug3：PWA standalone 模拟 + 抽卡界面（无滚动，top=0）→ 点卡片 → 详情 → 返回 → 断言回到 /random 且数据保留；书架同样；
   - Bug1：代码级断言 manifest 含 start_url/scope；断言全局 a 拦截已挂载；
   - 在线卡片分支返回来源页断言；
3. 回归 Round16 验证脚本。

## 决策确认（用户已确认 2026-08-21）

- D1=A：manifest 完整补全（start_url=/ scope=/ id=/ display_override + icons purpose any maskable）；
- D2=A：全局捕获拦截同源 a（PWA 下 preventDefault + Vue Router 导航）；
- D3=A：backState 无条件写入 + history.back() 兜底。

## 实施结果

- public/site.webmanifest：补全 start_url/scope/id/display_override/icons purpose；
- main.ts：`preventPwaLinkEscape` 全局捕获拦截（PWA 下同源 a 点击改 router.push）；
- detailNav.ts：`recordBackStateForDetail` 无条件写入来源路径（top/page 有则带）；
- ItemCard 在线分支补 backState 记录；TagChip/SearchBar/OfflineDetail 在线跳转 PWA 下改 SPA；
- OnlineDetail/OfflineDetail handleBack：移除 PWA push 首页分支，改 history.back() 兜底；
- RandomView 注册 listStateProvider（/random）。
- scripts/verify-round17.mjs 全部通过：top=0 返回来源页、manifest/全局拦截/无条件写入代码级断言。

## 待确认后再开工

请确认 D1~D3 后开始实现。
