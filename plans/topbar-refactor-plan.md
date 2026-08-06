# TopBar 重构计划：骰子/阅读清单 view 化 + 筛选并入搜索

> 状态：已落盘（待第一步样式修复真机验证后实施）
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
