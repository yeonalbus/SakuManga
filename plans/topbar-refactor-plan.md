# TopBar 重构计划：骰子/阅读清单 view 化 + 筛选并入搜索

> 状态：已实施（第二步重构完成，构建 + 产物验证通过；待真机复验）
> 关联：`plans/round2-device-fix-plan.md`（白屏修复）、`plans/layout-mode-plan.md`（三态布局）

## 一、背景与目标

用户报告四类布局问题（iPad/平板横屏顶部过高、底部白条、手机顶部空间大、手机卡片未拉满）。

**第一步·样式修复（已完成）**：

- 顶部收紧：TopBar 移动 padding/gap 收紧 + `App.vue` mobile `padding-top` 110→90px
- 底部白条：GridContainer 移除 `min-height:100%` 与 `.card-grid{flex:1}` 拉伸
- 手机顶部：SearchBar `.input-wrapper` 纵向 padding 4→3px
- 卡片宽度：mobile `main-content` padding 12→8px + 9 个卡片页 view padding 20→`12px 4px`（每侧 8+4=12px）
- 构建通过 + `scripts/verify-layout-css.mjs` 产物验证通过（无直接作用 html 的危险规则）

**第二步·本计划（结构性重构）**：

1. 随机骰子（手气不错）、阅读清单移出 TopBar，**view 化**为全局页面（`/random`、`/reading-list`）
2. **筛选并入搜索**：去掉 TopBar 独立齿轮按钮，入口放进搜索框内
3. TopBar 只剩**单行搜索栏** → 顶部高度根治（mobile `padding-top` 90→~56px）
4. 骰子内样式微调：不再做全局筛选（view 内直接读当前筛选上下文）

## 二、现状（关键文件与行号）

| 文件                                | 现状                                                                       | 涉及行                       |
| ----------------------------------- | -------------------------------------------------------------------------- | ---------------------------- |
| `src/components/TopBar.vue`         | 工具行（骰子+齿轮筛选+阅读清单）+ 搜索行两行布局；移动形态 flex-wrap       | 模板 37-58、移动样式 101-119 |
| `src/components/SearchBar.vue`      | 仅搜索：🔍+输入+清空+搜索按钮+历史/Tag 联想下拉                            | 模板 214-277、样式 279-449   |
| `src/components/RandomPicker.vue`   | Modal 弹窗；scopeType(全部/在线/离线)；useGlobalFilter 继承 modeStore 筛选 | script 1-151、模板 153-256   |
| `src/components/ReadingList.vue`    | 右侧抽屉（在线/离线 tab + badge）                                          | 模板 66-148、样式 150-444    |
| `src/components/FilterDrawer.vue`   | 全屏筛选抽屉（分类/关键词/语言/页数/评分）                                 | 模板 149-297                 |
| `src/router/index.ts`               | 全部扁平路由，无 `/random`、`/reading-list`                                | —                            |
| `src/components/OnlineSidebar.vue`  | 「🌐 在线模式」组                                                          | 1-12                         |
| `src/components/OfflineSidebar.vue` | 「📚 离线模式」组 + 可折叠书架                                             | 42-84                        |
| `src/App.vue`                       | mobile `main-content` padding-top 90px（本次已从 110 下调）                | 428-433、484-488             |

## 三、重构方案（含决策）

### 3.1 骰子 view 化 → `/random`

- 新增 `src/views/RandomView.vue`：把 `RandomPicker.vue` 的 Modal 主体抽取为页面
- 保留：数量控制、范围 scopeType（全部/在线/离线）
- 移除「继承全局筛选」开关：view 内直接读取 `modeStore` 当前 scope 的 `searchConfig` 作为默认筛选（可改），不再与全局耦合
- 结果仍用 `ItemCard` 网格展示
- `RandomPicker.vue` 从 TopBar 移除（文件保留为抽屉入口将不再被引用，视需要删除或保留备用）

### 3.2 阅读清单 view 化 → `/reading-list`

- 新增 `src/views/ReadingListView.vue`：把 `ReadingList.vue` 的抽屉主体抽取为页面
- 保留：在线/离线 tab、badge、移除/下一部操作
- 数据源不变（`readingStore`：onlineReadingList / offlineReadingList）
- `ReadingList.vue` 从 TopBar 移除

### 3.3 筛选并入搜索

- 保留 `FilterDrawer.vue` 全屏抽屉（功能完整，不重建）
- 触发入口从独立齿轮按钮改为 **SearchBar `.input-wrapper` 内右侧筛选图标按钮**（带激活态红点提示当前是否有筛选条件）
- 不放进现有「历史 + Tag 联想」下拉（避免拥挤）
- FilterDrawer 与 `activeSearchConfig` 逻辑从 TopBar 迁入 SearchBar（或 SearchBar 内部挂载）

### 3.4 TopBar 单行化

- TopBar 只保留 SearchBar；删除 RandomPicker / 齿轮按钮 / ReadingList 组件引用
- 删除移动形态两行布局（`flex-wrap`、`.search-wrapper{flex:1 0 100%}`）
- 移动形态下 TopBar 高度 ~56px（单行）

### 3.5 边栏新增栏目（在线模式与系统之间）

- 新增「🎲 工具」栏目：手气不错（`/random`）、阅读清单（`/reading-list`）
- `OnlineSidebar.vue` 与 `OfflineSidebar.vue` **都加**（骰子支持 🌐+📚 全库、清单有在线/离线双 tab，属跨模式全局功能）
- 手机端（边栏为抽屉）通过汉堡 → 边栏进入这两个页面

### 3.6 App.vue 顶部补偿

- mobile `main-content` `padding-top`: `calc(90px + var(--safe-top))` → `calc(56px + var(--safe-top))`（单行 TopBar 后）
- 同步调整 `@media (max-width:1024px)` 与 `html[data-layout='mobile']` 两处

## 四、实施步骤

1. 新建 `src/views/RandomView.vue`（从 RandomPicker 抽取，见 3.1）
2. 新建 `src/views/ReadingListView.vue`（从 ReadingList 抽取，见 3.2）
3. `src/router/index.ts` 注册 `/random`、`/reading-list`（扁平，挂系统/工具组）
4. `OnlineSidebar.vue` / `OfflineSidebar.vue` 新增「🎲 工具」栏目
5. `SearchBar.vue`：input-wrapper 内加筛选图标按钮（激活态红点）+ 挂 FilterDrawer + 承接 activeSearchConfig / handleApplyFilters
6. `TopBar.vue` 重写为单行（移除骰子/齿轮/清单，只留 SearchBar；删移动两行逻辑）
7. `App.vue` mobile `padding-top` 90→56px（两处）
8. `npm run build` + `node scripts/verify-layout-css.mjs` 产物验证
9. 真机复验：手机 / iPad 竖横屏 / 三态

## 五、风险与注意事项

- **scoped CSS `:global(html[...]) .class` 编译 bug**：新增任何 `:global()` 必须包裹完整选择器（含子类名），否则规则直接作用 `<html>` 导致白屏（见 `round2-device-fix-plan.md`）。重构后 TopBar 相关 `:global(html[data-layout='mobile'] ...)` 规则需改写/删除时逐一验证产物
- **路由扁平注册**：不要嵌套布局，保持现有扁平路由风格
- **手机端入口取舍**：TopBar 精简后骰子/清单入口在边栏抽屉，手机端需要先开抽屉（用户已接受此方案）
- **SearchBar 改造**：避免破坏现有历史记录与 Tag 联想下拉逻辑；筛选按钮放在 input 与提交按钮之间
- **FilterDrawer 迁移**：`activeSearchConfig` 按当前模式（在线/离线）分域保存的逻辑需保留，不互相串味
- **旧组件清理**：`RandomPicker.vue` / `ReadingList.vue` 若不再被引用，确认无残留 import 后处理
- **实施记录（2026-08-06）**：新建 `src/views/RandomView.vue`、`src/views/ReadingListView.vue`；注册 `/random`、`/reading-list`；两个侧边栏加「🎲 工具」栏目；SearchBar 内嵌筛选按钮（⚙️+激活红点）+ 挂 FilterDrawer（按域分域保存）；TopBar 单行化（仅 SearchBar）；App.vue mobile padding-top 90→56px；删除旧组件 RandomPicker.vue / ReadingList.vue；`npm run build`（206 模块）+ `verify-layout-css.mjs` 产物验证 0 危险规则 ✅
- **详情页 Header 布局修复（2026-08-06，BUG 报告：Android/iPad 操作栏与返回按钮重叠）**：
  1. `src/components/TopBar.vue` `html[data-layout='mobile']` 悬浮态补安全区：`height:auto` + `min-height: calc(48px + var(--safe-top))` + `padding-top/bottom` + `padding-right: calc(12px + var(--safe-right))`，顶栏总高恒 = 48px + safe-top，与 `App.vue` `main-content` 的 `padding-top: calc(56px + var(--safe-top))` 严格匹配；修复「iPad 横屏 + 手动移动模式」（>1024px 时 @media 不生效）下顶栏不含 safe-top、直接覆盖状态栏的问题
  2. `src/views/online/OnlineDetail.vue` / `src/views/offline/OfflineDetail.vue`：操作栏（`.top-action-bar` / `.top-bar`）显式 `position: static` 回归文档流；移动形态（`html[data-layout='mobile']`）下 `position: sticky; top: calc(56px + var(--safe-top)); z-index: 40`（层级：汉堡 70 > 侧栏 65 > 遮罩 60 > TopBar 50 > 操作栏 40 > 内容），吸附在悬浮 Header 正下方，带 `--app-bg` 背景 + 底边框，与标题/封面清晰上下分界，不被顶进 Header、不遮挡封面
  3. `:global` 一律完整包裹并用页面独有父类限定（`html[data-layout='mobile'] .detail-page .top-action-bar`、`.offline-detail-page .top-bar`），避免 scoped 编译丢类 + 误伤 TopBar 组件同名 `.top-bar`（产物已验证无直接作用 html 的危险规则）
  4. `npm run build`（206 模块）+ `verify-layout-css.mjs` 产物验证 0 危险规则 ✅（待真机复验：Android / iPad 竖横屏 / 三态布局 + 手动移动模式）

- **详情页顶部重设计 v2（2026-08-06，用户否决 sticky「漂浮」方案）**：

  > 用户反馈：sticky 吸附在悬浮 Header 正下方「漂浮着也不好看」，与之前 search「栏位撑太满」是同类问题。用户指定方向：**功能按钮放标题下方 + 返回做成悬浮球（参照主页右下角 FloatingToolbar 圆形球）**，经确认采用**方案 B 改良**——仅移动形态（`html[data-layout='mobile']`）生效，桌面端保留现有顶部操作栏不变。

  **目标**：移动形态下详情页顶部不再有「一整行撑满的悬浮操作栏」；改为 ① 返回按钮 → 左上角圆形悬浮球（fixed，避开悬浮 TopBar）；② 功能按钮 → 标题下方一条横向可滚动按钮条（图标+文字，`overflow-x:auto` 不换行不撑满）。

  **改动点**（均需切回 code 模式实施）：

  1. **OnlineDetail.vue**（`src/views/online/OnlineDetail.vue`）
     - 模板（447-525 区）：`.top-action-bar` 前新增返回悬浮球 `<button class="detail-fab-back" @click="handleBack">‹</button>`；在 `.title-header`（495-500）之后、`.detail-tabs`（503）之前新增 `.detail-actions-bar`，内含 4 个功能按钮（加入清单 463-468 / 收藏 470-486 / 立即阅读 488 / 下载 490）——复用 `.add-reading-btn`、`.action-btn`、`.fav-btn`、`.read-btn`、`.download-btn` 类与同名 handler
     - 样式：删除 sticky 规则（778-786）；新增 base `.detail-fab-back{display:none}`、`.detail-actions-bar{display:none}`；移动形态 `:global(html[data-layout='mobile'] .detail-page .detail-fab-back)` → `position:fixed; top:calc(56px + var(--safe-top) + 8px); left:calc(10px + var(--safe-left)); z-index:40; width:40px; height:40px; border-radius:50%;`（样式参照 `FloatingToolbar.vue` `.fab-trigger`，166-192）；`:global(... .detail-actions-bar)` → `display:flex; overflow-x:auto; -webkit-overflow-scrolling:touch; flex-wrap:nowrap; gap:8px;` 子按钮 `flex-shrink:0; white-space:nowrap`；`:global(... .top-action-bar){display:none}`
     - @media(1024px) 里 `.top-action-bar`/`.right-actions` 换行规则可保留（移动形态已 display:none，无害）

  2. **OfflineDetail.vue**（`src/views/offline/OfflineDetail.vue`）
     - 模板（280-314 区）：`.top-bar` 前新增返回悬浮球；在 `.main-layout`（303）之前新增 `.detail-actions-bar`（加入清单 285-291 / 立即阅读 / 删除 297-299）；**「📖 继续阅读 (已读 {{ comic.readCount || 0 }} 次)」（293-295）→「📖 立即阅读」**（与 Online 一致；`readCount` 在 script 243 行仍自增保留，仅去掉按钮文案展示）
     - 样式：删除 sticky 规则（438-447）；新增返回球 / 功能条（同 OnlineDetail 结构，父类 `.offline-detail-page`）；`:global(... .top-bar){display:none}`

  3. **安全区与层级**：返回球 `top: calc(56px + var(--safe-top) + 8px)` 恒位于悬浮 TopBar（48px+safe-top ≈ 顶栏）正下方；汉堡 70 > TopBar 50 > 返回球 40 > 内容，不与汉堡（TopBar 内左上，`top:8px+safe-top`）重叠

  4. **验证**：`npm run build` + `node scripts/verify-layout-css.mjs`（产物须无直接作用 html 的 data-layout 危险规则）；产物抽查确认 `.detail-fab-back[data-v-*]{display:none}` 与移动形态 fixed 规则并存、旧 sticky 规则已移除

  5. **真机复验**：手机 / iPad 竖横屏 / 三态布局 + 手动移动模式；确认返回球不遮汉堡、功能条可横滚、不遮挡封面、桌面端布局未回归
  6. **实施记录（2026-08-06）**：
     - `src/views/online/OnlineDetail.vue`：模板新增 `<button class="detail-fab-back" @click="handleBack">‹</button>`（detail-page 内最前）+ `.title-header` 后新增 `.detail-actions-bar`（加入清单/收藏/立即阅读/下载，复用原 handler 与类）；移动形态 `:global(html[data-layout='mobile'] .detail-page .top-action-bar){display:none}`；返回球 `position:fixed; top:calc(56px + var(--safe-top) + 8px); left:calc(10px + var(--safe-left)); z-index:40; 40px 圆形`；功能条 `display:flex; overflow-x:auto; -webkit-overflow-scrolling:touch; flex-wrap:nowrap` + 子按钮 `flex-shrink:0; white-space:nowrap`
     - `src/views/offline/OfflineDetail.vue`：同结构改造（功能条置于 `.main-layout` 前：加入清单/立即阅读/删除）+ **「📖 继续阅读 (已读 N 次)」→「📖 立即阅读」**（桌面端同样生效；`readCount` script 243 行仍自增保留）
     - 两页均删除旧 `position:sticky` 规则；`npm run build`（206 模块）+ `verify-layout-css.mjs` 产物验证 0 危险规则 ✅；产物抽查确认 OnlineDetail/OfflineDetail 的 `html[data-layout=mobile] .detail-page|.offline-detail-page` 系列规则正确编译、详情页旧 sticky 已移除
     - 桌面端保持原顶部操作栏（回归文档流 + TopBar 安全区修复）不变
