# Round12 小详情面板展开/收起布局稳定计划（点开画廊保持视线焦点）

## 背景与问题

| 编号 | 类型 | 内容 |
|------|------|------|
| Bug1 | 布局 | 桌面端点击卡片展开右侧小详情面板后，卡片网格列数/宽度变化，被点开的画廊被挤出视线焦点（如 4 列变 3 列，第4行第4列的画廊被推下去，眼前变成原第3行第4列的内容） |
| Bug2 | 布局 | iPad 端同样现象更明显：列数不变仍被挤走；关闭面板后页面乱滑动 |

---

## 根因分析（已通读代码确认）

### 布局链路（宽屏在线列表共用同一套机制）

1. **面板打开 = 列表列变窄**：所有在线列表页用 `.online-split` 网格容器（[OnlineDetailPanel.vue](src/components/OnlineDetailPanel.vue) 的 `:global(.online-split)` 样式）：
   - 面板收起：`grid-template-columns: minmax(0, 1fr)`（列表独占整行）；
   - 面板展开（`.panel-open`）：`grid-template-columns: minmax(0, 1fr) minmax(360px, 420px)` —— 左侧列表**立即被压缩掉 360~420px**。
2. **列数注入变化**：[GridContainer.vue](src/components/GridContainer.vue) 在「面板自动适配列数」开启（`styleSettings.autoPanelColumns`，默认开）且宽屏时，按面板开/关注入 `--card-cols`：
   - 收起 → `cardPanelClosedCols`（默认 5，用户可能配成 4）；
   - 展开 → `cardPanelOpenCols`（默认 3）。
   → 面板一开，网格从 N 列变 3 列，卡片全部重排。
3. **卡片高度随列宽变化**：[ItemCard.vue](src/components/ItemCard.vue) 封面用 `aspect-ratio: 3/4` + `width:100%` → 列宽变 → 卡片高度变 → 行高变。
4. **滚动容器是 `#main-content`**（App.vue），浏览器保留 `scrollTop` 不变，但网格重排后内容位移 → 被点卡片不再处于原来的视口位置。

### iPad 现象解释

- iPad 横屏 ≥1025px 时 `isWide=true`，面板渲染（`v-if="isWide"`）；若用户未开启「面板自动适配列数」或列数设置使开/关相同，则列数不变，但**列表列宽仍被面板压缩 360~420px** → 卡片变窄变矮 → 行高变化 → 被点卡片同样被挤走；
- 关闭面板时宽度恢复 → 行高再次变化 + 内容总高变化 → 「乱滑动」。
- 无论列数是否变化，**补偿方案同一套逻辑覆盖两种场景**。

---

## 修复设计：滚动补偿（锚定被点击卡片，保持其视口位置不动）

### 核心思路

在面板开/关的**同一次渲染**后（`nextTick` + 双 `requestAnimationFrame` 等布局稳定），把主滚动容器 `#main-content` 的 `scrollTop` 平移「被锚定卡片视口位置的位移量」，让该卡片回到展开前的视口 Y 坐标：

```
打开面板：
  1. 记录被点击卡片元素 el 的视口偏移 delta0 = el.getBoundingClientRect().top - mainContent.getBoundingClientRect().top
  2. isPanelOpen = true（触发网格重排）
  3. await nextTick() + 双 rAF（布局稳定：列宽/列数/卡片高度均已重排）
  4. delta1 = 同一元素新的视口偏移（Vue v-for 按 key 复用 DOM，元素引用不变）
  5. mainContent.scrollTop += (delta1 - delta0)  ← 卡片回到原视口位置

关闭面板：同一逻辑（锚定面板对应卡片；不可见时锚定视口内最顶卡片）
```

### 关键实现点

**1a. [ItemCard.vue](src/components/ItemCard.vue)** 根节点加 `:data-gid="comic.id"`（在线=GID，离线=本地 id），供 `useDetailPanel` 通过 `[data-gid]` 选择器精确找到被点卡片。

**1b. [useDetailPanel.ts](src/composables/useDetailPanel.ts)**（6 个在线列表页共用的唯一入口）新增补偿逻辑：
- `openDetail(comic)`：展开前捕获卡片元素 → 展开后补偿；
- `closePanel()`：展开期间锚定 panelGid 对应卡片（若仍在 DOM）→ 关闭后补偿；
- `togglePanel()`（工具栏切换）：无具体卡片时锚定**视口内最顶可见卡片**；
- 面板**已处于展开态**时再次点击其他卡片 → 布局不变，跳过补偿（delta≈0 自然无感）。
- 补偿仅宽屏（`isWide`）生效；窄屏走新标签详情，不涉及。

**1c. 抑制浏览器原生 scroll anchoring 干扰**：给 `#main-content` 或 `.card-grid` 加 `overflow-anchor: none`，避免浏览器自选锚点与我们的显式补偿叠加双重位移。

**1d. 时序稳健性**：卡片高度由 `aspect-ratio` 决定（不依赖图片加载），`nextTick` + 双 rAF 足以等布局稳定；极端情况（补偿后被 clamp）浏览器自动截断，可接受。

---

## 涉及文件（均为前端，无后端改动）

| 文件 | 改动 |
|------|------|
| `src/components/ItemCard.vue` | 根节点加 `data-gid` 属性 |
| `src/composables/useDetailPanel.ts` | 开/关/切换三入口滚动补偿 |
| `src/App.vue` 或 `GridContainer.vue` | `overflow-anchor: none`（防原生锚定干扰） |
| `scripts/verify-round12.mjs` | 新增验证脚本 |
| `plans/round12-panel-focus-plan.md` | 本计划 |

---

## 决策点（请确认）

- **D1 面板展开时的列数策略**：
  - A（推荐）：**保留现有「面板自动适配列数」**（展开后变 3 列，卡片更大更清晰）+ 滚动补偿保持点击卡片不动；
  - B：面板展开时**列数保持不变**（卡片变窄但行位置基本不变，跳动更小，但卡片变小）。
  - 注：即便选 B，因列宽仍被压缩 360~420px、卡片高度变化，补偿逻辑依然需要，只是位移更小。
- **D2 锚定精度**：
  - A（推荐）：**精确锚定** —— 被点击卡片回到原视口位置（完全不动）；
  - B：宽松锚定 —— 仅保证仍在可视区内（`scrollIntoView nearest`），位置可偏移。
- **D3 关闭面板的行为**：
  - A（推荐）：关闭时**同样补偿**，视野保持稳定（解决 iPad「关闭后乱滑动」）；
  - B：关闭时不补偿，恢复展开前的滚动位置。

---

## 验证方案

1. `npm run type-check` + `npm run build`，同步 `backend/webui/dist`；
2. 新增 `scripts/verify-round12.mjs`：本地起后端（`go run . --headless`）后，用 **Playwright route 拦截** mock `/api/v1/comics/online`（列表）与 `/comics/online/detail`（详情），喂入固定假数据（≥16 条）——不依赖 E 站网络，确定性验证：
   - 桌面 1280px：卡片网格 N 列（如 4）→ 点击第 4 行第 4 列卡片 → 面板展开 → 断言该卡片 `getBoundingClientRect().top` 位移 ≤ 2px（仍在视线焦点）；
   - 断言网格变为 3 列（若 D1=A）；
   - 点击「✕ 收起」→ 断言视口稳定、卡片仍可见；
   - iPad 视口（如 1180px 横屏）：重复上述流程（列数不变场景），断言同样稳定；
   - 关闭面板后页面不「乱滑动」（scrollTop 变化 ≤ 2px）。
3. 回归：`go test ./...`（后端无改动，确保不破坏）。

---

## 提交计划（按模块拆分）

1. `fix(frontend): 小详情面板展开/收起滚动补偿（点开画廊保持视线焦点）` —— 前端代码 + 计划 + 验证脚本；
2. `chore(build): 同步内嵌前端构建产物到 backend/webui/dist`。

## 决策确认（用户已确认 2026-08-21）

- **D1 = A**：保留「面板自动适配列数」（展开 4列→3列），配合滚动补偿；
- **D2 = A**：精确锚定 —— 被点卡片回到原视口位置（完全不动）；
- **D3 = A**：关闭面板时同样补偿，视野稳定（解决 iPad 关闭后乱滑动）。

## 实施结果

- `ItemCard.vue` 根节点加 `data-gid`；`useDetailPanel.ts` 三入口滚动补偿（nextTick + 双 rAF）；`App.vue` 加 `overflow-anchor:none`。
- `scripts/verify-round12.mjs`：桌面（4→3列）与 iPad 横屏（4列不变）双场景全部通过（展开/收起卡片视口位移 ≤ 2px）。

## 待确认后再开工

以上计划基于代码通读确认的根因。请确认 D1~D3 后开始实现。
