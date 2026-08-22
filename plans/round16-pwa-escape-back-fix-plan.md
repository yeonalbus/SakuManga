# Round16 修复 PWA 逃逸 / 返回状态丢失 / 排行榜空内容

## 背景与问题

| 编号 | 类型 | 内容 |
|------|------|------|
| Bug1 | iPad | PWA 横屏黑边仍存在；且新增：点击侧栏选项卡（工具/系统/阅读/在线）时出现 Safari 顶栏（关闭/返回/字体大小/URL/分享/刷新/浏览器打开）+ 底部窄条——PWA 逃逸到 Safari 标签页模式 |
| Bug3 | iPad | 返回仍固定回首页（应回到上个界面并保留状态：抽卡 8 本界面/书架界面） |
| Bug6 | 在线 | 排行榜点本子返回后排行榜不显示内容（需回首页再进排行榜） |

---

## 根因分析（已通读代码确认，三问题同一条根因链）

### 核心根因：Round15 引入的 PWA 整页导航导致逃逸

- detailNav.ts `openComicDetailInNewTab` 在 `isStandalonePWA()` 为 true 时执行 `window.location.href = href`（整页导航）；
- **iOS PWA 中 `location.href` 整页导航会退出 standalone 上下文** → 页面在新 Safari 标签页渲染 → 显示 Safari 顶栏/底部工具栏（用户描述的「iPad 顶栏」）；
- 用户点击侧栏选项卡「手气不错/工具/系统」等是 SPA 路由，本身不会逃逸——但**一旦之前某次进入详情触发了 location.href 逃逸，后续所有页面都停留在 Safari 标签页模式**（顶栏常驻）；「手气不错→离线首页顶栏空白条」= Safari 顶栏随滚动收起的正常行为。

### Bug3 返回固定回首页

- 逃逸后 `window.opener=null`、`sessionStorage` 标记不共享 → `consumeBackState` 未命中 → 详情页 handleBack 走 Round15 加的 `isStandalonePWA() → router.push(首页)` 分支 → 固定回首页，丢失来源页与状态。
- **正确逻辑**：返回应优先 `consumeBackState`（来源页 + 滚动 + 页码），未命中才回首页。

### Bug6 排行榜返回空

- 排行榜（OnlineTop）`onActivated` 返回时调 `restoreListState()` → `takeListState` 消费状态 + `fetchToplist()`；
- 逃逸导致的**整页导航**使 keep-alive 失效 → 返回时组件重新 onMounted → `takeListState` 已消费但数据未加载完成/`currentTl` 重置 → 空内容；
- 修复逃逸后，keep-alive 正常保留，排行榜返回即恢复。

---

## 修复方案

### 一、根治 PWA 逃逸：取消所有整页导航，改为 SPA 路由跳转

**1a. utils/detailNav.ts** `openComicDetailInNewTab`：
   - **删除** `window.location.href = href`（PWA 逃逸元凶）；
   - PWA 下改为**同标签 SPA 跳转**：detailNav 是纯工具无 router，由调用方（组件内 useRouter）执行；
   - 做法：detailNav 保留 `recordBackState`（SPA 内 sessionStorage 共享），新增 `openComicDetailInApp(comic)` 供调用方用 router.push 跳转；

**1b. 统一跳转入口**：
   - ItemCard.vue 离线卡片、OnlineDetailPanel.openFullDetail、useDetailPanel.openDetail 窄屏分支——全部改用 router.push（SPA），不再 window.open/location.href；
   - 保留 backState 记录（push 前调用，SPA 内 sessionStorage 共享，返回命中）；
   - 非 PWA（桌面浏览器）：仍可 window.open 新标签（保持现状）。

**1c. isStandalonePWA 保留**，仅用于「选择 SPA 跳转 vs 新标签」，不再触发 location.href。

### 二、Bug3 返回逻辑修正（回到上个界面并保留状态）

**2a. OnlineDetail/OfflineDetail handleBack**：
   - **先** consumeBackState(gid) 命中 → rememberListState + router.replace(fromPath)（已有，保留）；
   - 未命中且 PWA → 回当前模式首页（保留）；
   - 确认 backState 在 SPA 跳转后能命中（sessionStorage 共享）。

**2b. 状态记录完整性**：SPA 跳转前 recordBackState 已写 saku_back_<id>（fromPath/top/page），返回 router.replace 后列表页 takeListState 恢复滚动/页码 → 抽卡 8 本、书架、排行榜均回到离开时位置。

### 三、Bug6 排行榜返回空（依赖逃逸修复，另加防御）

**3a. OnlineTop.vue restoreListState 防御**：takeListState 消费状态后若返回的 page/top 存在但数据未加载，保证 fetchToplist 必定执行（现状已执行，主要靠逃逸修复后 keep-alive 不丢）；
**3b.** 确认 onActivated 在 keep-alive 恢复时不重复消费：takeListState 一次性（取走即删），onMounted 与 onActivated 共用 restoreListState——返回时仅 onActivated 触发，消费一次；若首次挂载 onMounted 已消费则返回时无状态可取（符合预期，数据在 keep-alive 内存中）。

---

## 涉及文件

| 文件 | 改动 |
|------|------|
| src/utils/detailNav.ts | 删除 location.href；新增 SPA 跳转辅助 |
| src/components/ItemCard.vue | 离线卡片 SPA 跳转（PWA）/ window.open（桌面） |
| src/components/OnlineDetailPanel.vue | openFullDetail SPA 跳转 |
| src/composables/useDetailPanel.ts | 窄屏 openDetail SPA 跳转 |
| src/views/online/OnlineDetail.vue / offline/OfflineDetail.vue | handleBack 修正 |
| src/views/online/OnlineTop.vue | 排行榜返回数据防御（可选） |
| scripts/verify-round16.mjs | 新增验证脚本 |
| plans/round16-pwa-escape-back-fix-plan.md | 本计划 |

---

## 决策点（请确认）

- **D1 PWA 下详情跳转方式**：
  - A（推荐）：**一律 SPA 路由跳转**（router.push，同标签，backState 记录来源）——PWA/桌面窄屏统一，返回必命中来源；桌面宽屏详情面板不受影响；
  - B：PWA 下 SPA、桌面保持 window.open（现状修正，桌面新标签行为保留）。
- **D2 桌面端离线卡片跳转**：
  - A（推荐）：桌面也改 SPA 同标签（返回语义统一，避免桌面 sessionStorage 继承差异）；
  - B：桌面保持 window.open 新标签（需 window.opener 判定返回）。
- **D3 排行榜空内容防御**：
  - A（推荐）：仅依赖逃逸修复（keep-alive 恢复即正常），不做额外改动；
  - B：加 onActivated 强制 refetch 防御（数据可能陈旧）。

---

## 验证方案

1. npm run type-check + 构建 + 同步 dist；
2. scripts/verify-round16.mjs：
   - 逃逸根治：代码级断言 detailNav 无 location.href 整页导航；断言所有调用方改 SPA；
   - Bug3：模拟 PWA standalone（context 注入 matchMedia）→ 离线首页点卡片进详情 → 返回 → 断言回到列表且 scrollTop/页码保留；抽卡场景同样断言；
   - Bug6：进入排行榜 → 点卡片 → 返回 → 断言排行榜内容仍在（无空态）；
   - Bug1：代码级断言无 location.href 逃逸路径（黑边 dvh 兜底已在 Round15 保留）。
3. 回归：Round15 验证脚本复跑。

---

## 提交计划

1. fix(frontend): 修复 PWA 逃逸（取消整页导航改 SPA 跳转）+ 返回保留来源页状态 + 排行榜返回空防御；
2. chore(build): 同步内嵌前端构建产物到 backend/webui/dist。

## 决策确认（用户已确认 2026-08-21）

- D1=A：一律 SPA 路由跳转（router.push 同标签）；
- D2=A：桌面也改 SPA 同标签；
- D3=A：仅依赖逃逸修复（keep-alive 恢复即正常）。

## 实施结果

- detailNav.ts：删除 `window.location.href`（PWA 逃逸元凶）；新增 `buildDetailRoute` + `recordBackStateForDetail`（SPA 跳转前记录来源）；
- ItemCard.vue：离线卡片默认 SPA 同标签跳转（`openDetailNav`），中键/Ctrl 仍新标签；
- OnlineDetailPanel / useDetailPanel：窄屏分支改 SPA 跳转（不再 window.open）；
- OnlineDetail / OfflineDetail handleBack：backState 优先（返回来源页+状态），PWA 兜底回首页；
- OnlineTop：依赖逃逸修复（keep-alive 保留），无额外改动。
- `scripts/verify-round16.mjs` 全部通过：PWA 模拟下 SPA 跳转进详情、返回来源页、列表数据保留（非整页刷新）；逃逸根治代码级断言。

## 待确认后再开工

请确认 D1~D3 后开始实现。
