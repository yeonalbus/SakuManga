# Round22 排序重构（拖拽 + 移动到/置顶 + LexoRank）与快捷加入移除扩大化计划

## 背景与目标

| 编号 | 类型 | 内容 |
|------|------|------|
| 优化1 | 排序 | 「书架列表」与「书架内本子」两处排序重构：① 拖拽排序（把手图标 / 长按 300ms，拖动时列表仍可上下滑动）；② 操作菜单「移动到第 X 位」「快捷置顶」；③ 浮点分段权值（LexoRank）减少频繁排序的写入开销；④ 去掉现有 ↑↓ 按钮 |
| 优化2 | 快捷加入/移除 | ① 快捷加入扩大支持面：现仅「离线首页」「书架内部」，扩大到「手气不错」「历史记录」；② 快捷移除：书架内部多选后批量移出所选本子 |

---

## 一、现状分析

### 1.1 书架列表排序现状（Round10 + Round13 遗留）

| 位置 | 现状 | 问题 |
|------|------|------|
| 侧栏置顶书架（OfflineSidebar.vue L106-120） | 每行 hover 显示 ↑/↓，调 `moveBookshelf(id, ±1)` | 每次移动重写**全量**顺序（POST /bookshelves/reorder，ids 数组下标即 sort_order），O(n) 写入 |
| 「全部书架」浮层（BookshelfPickerOverlay.vue L142-143） | 每行 ↑/↓，同上 | 同上 |
| 后端（library.go） | `ORDER BY sort_order asc, name asc`；`ReorderBookshelves` 循环 Update sort_order=下标 | 相邻交换 = 全量重排，频繁拖拽开销大 |

### 1.2 书架内本子排序现状（Round10）

- OfflineBookshelf.vue：header「↕ 排序」进入**专用竖排排序视图**，每行 ↑/↓（`moveSortItem`）；
- 完成时调 `reorderShelfComics(shelfId, validIds)` → PUT `/bookshelves/:id/order`，**整包重写 comicIds JSON 数组**（顺序即展示顺序）；
- 展示顺序 = `shelfComics` computed 按 comicIds 数组顺序映射。

### 1.3 快捷加入现状（Round13）

| 页面 | 机制 |
|------|------|
| 离线首页 OfflineHome.vue | 长按卡片（600ms）→ 多选工具条「📥 加入书架」→ BookshelfPickerOverlay(add 模式) → `addComicsToShelf`（POST /bookshelves/:id/comics/batch） |
| 书架内部 OfflineBookshelf.vue | 同上模式（选择工具条 + 加入书架 + 管理员删除） |

两页的选择工具条/加入逻辑**高度重复**（各自约 80 行相似代码），未抽公共逻辑。

### 1.4 快捷移除现状

- 仅离线详情页单本移除（OfflineDetail.toggleShelfCheck → `removeComicFromShelf`）；
- 后端仅单删接口 `DELETE /bookshelves/:id/comics?comicId=xxx`，**无批量移除**；
- 书架内部多选工具条只有「加入书架 / 删除」，**没有「移出书架」**。

### 1.5 目标页现状（快捷加入扩大对象）

| 页面 | 现状 |
|------|------|
| 手气不错 RandomView.vue | 结果区 `<ItemCard mode="card">` **不可多选**，无加入书架入口；结果可含在线/离线混合 |
| 离线历史 OfflineHistory.vue | GridContainer **无 selectable**，无选择模式 |
| 在线历史 OnlineHistory.vue | 已有长按多选（useBatchSelection，用于批量下载 BatchDownloadBar），**无加入书架** |

---

## 二、方案设计

## A. 排序优化（书架列表 + 书架内本子）

### A.1 拖拽交互

**通用拖拽原语（新 composable `useDragReorder`）**：

- 触发方式二选一（见决策 D1）：
  - **独立拖拽把手图标**（⠿ / ≡）：仅把手上 `pointerdown` 进入拖拽，把手上 `touch-action: none`，不拦截列表其他区域的滚动；
  - **长按 300ms**：在行内按下 300ms 未移动 → 进入拖拽（区别于书架卡片 600ms 多选长按，互不冲突）。
- **拖动时列表仍可上下滑动**（决策 D2）：
  - 被拖行**悬浮**（fixed 定位跟随指针），列表保持原生滚动（不 preventDefault 列表自身的 wheel/touchmove）；
  - 拖近容器上下边缘时 **rAF 边缘自动滚动**（速度随距离增大）；
  - 松手按指针 Y 与各行中线比对计算落位索引 → 插入 → 仅更新该单项权值（见 A.3）。
- 落位预览：目标间隙显示插入指示线（placeholder）。

**应用位置**：

| 位置 | 说明 |
|------|------|
| 书架列表·侧栏（OfflineSidebar） | 置顶书架行右侧加把手图标（hover 显隐；移动端常显）；拖动落位按**全局书架顺序索引**换算（置顶组顺序 = 全局顺序子序列，沿用 Round10 语义） |
| 书架列表·全部书架浮层（BookshelfPickerOverlay） | 每行加把手图标，可拖到任意位置 |
| 书架内本子（OfflineBookshelf 排序视图） | 保留「↕ 排序」专用竖排视图（决策 D3），每行加把手图标；行内长按 300ms 亦可拖拽 |

### A.2 操作菜单（移动到 / 快捷置顶）

每行提供「⋯」操作菜单（侧栏行、浮层行、排序视图行三处）：

- **移动到第 X 位**：弹输入框（1..N，1-based，越界自动钳制）→ 目标位置取前后邻项权值中点，仅更新该项；
- **快捷置顶**：移到第 1 位（`weight[first] - 1`）。
- 命名注意（决策 D6）：书架列表行的「置顶」与 Round13「置顶到侧栏」（pinned）语义冲突，建议书架列表菜单用「**移到顶部**」，书架内本子用「**置顶**」。

### A.3 LexoRank 浮点分段权值（核心）

**权值模型**：

- 初始权值：`index * 1000`（0, 1000, 2000, …），惰性生成（旧数据首次进入排序/拖动时按当前数组顺序赋权，顺序不变）；
- 插入邻项 a、b 之间：`w = (w_a + w_b) / 2`（如 1000 与 2000 之间 → 1500）；
- 插入顶部：`w = w_first - 1`；插入底部：`w = w_last + 1`；
- **精度用尽判定**：中点取整保留 6 位小数；当两邻项 `gap < 1e-6`（取整后无中间值）→ 触发**一次异步全量重置**（全部重赋 `index * 1000`，POST 全量，防抖合并，不阻塞拖拽）；
- 单次移动只写**一项权值**，取代现状的整包数组重写，显著降低频繁排序开销。

**存储方案（决策 D4/D5）**：

| 对象 | 现状 | 改造 |
|------|------|------|
| 书架列表 | `sort_order int`（全量下标重写） | 新增 `sort_key REAL` 列（AutoMigrate 加列）；查询 `ORDER BY sort_key asc, sort_order asc, name asc`（sort_order 留作旧数据回退）；单书架移动 = 新端点 `PUT /bookshelves/:id/position` 只更新 sort_key |
| 书架内本子 | `comicIds` JSON 数组（顺序即排序） | 新增 `sort_keys TEXT`（JSON 权值表 `{"<comicId>": <weight>}`）；comicIds 保留为成员集合 + 旧数据回退；单本移动 = `PUT /bookshelves/:id/order` 扩展单键模式 `{comicId, sortKey}`；全量重置仍走 `{comicIds: [...]}`（向后兼容 Round10） |
| 展示顺序 | comicIds 数组顺序 | 有 sort_keys 的按权值升序；无权值的（旧数据）按 comicIds 数组顺序排后（惰性赋权后自然归位） |

**后端联动**：加入/移出/批量加入时同步维护 sort_keys（新增条目默认追加末尾权值、删除条目同步删权值）。

### A.4 去掉 ↑↓ 按钮

| 位置 | 动作 |
|------|------|
| OfflineSidebar.vue ↑/↓ | 删除，改拖拽把手 + ⋯ 菜单 |
| BookshelfPickerOverlay.vue ↑/↓ | 删除，改拖拽把手 + ⋯ 菜单 |
| OfflineBookshelf.vue 排序视图 `moveSortItem` ↑/↓ | 删除，改拖拽 + ⋯ 菜单 |
| bookshelfStore.`moveBookshelf` | 删除（无调用方），保留 `reorderBookshelves` 供全量重置使用 |

## B. 快捷加入移除优化

### B.1 快捷加入扩大支持面

**抽公共逻辑**（消除 OfflineHome/OfflineBookshelf 现有重复）：

- 新 composable `useShelfQuickAdd(getPageItems)`：封装 selectMode / selectedIds / 长按进入 / 点选 / 全选本页 / 打开书架浮层 / 批量加入（`addComicsToShelf`）/ 退出；
- 新组件 `ShelfQuickAddToolbar.vue`：已选 N 部 / 全选本页 / 📥 加入书架 /（可选 🗑️ 移出书架）/（管理员 🗑️ 删除）/ 取消；
- OfflineHome、OfflineBookshelf 改用公共逻辑（行为不变，纯重构）。

**新增页面**：

| 页面 | 方案 |
|------|------|
| 手气不错 RandomView.vue | 结果区 ItemCard 加 `selectable`；选择工具条 + BookshelfPickerOverlay(add)；**在线结果处理见决策 D7**（推荐仅离线结果可加入） |
| 离线历史 OfflineHistory.vue | GridContainer 加 `selectable` + 选择模式 + 工具条（沿用离线首页模式） |
| 在线历史 OnlineHistory.vue | **决策 D8**（推荐本轮不加：在线本入书架会“隐形”——书架当前仅展示离线本；维持批量下载） |

### B.2 书架内多选快捷移除

- OfflineBookshelf 选择工具条新增「🗑️ **移出书架**」按钮（仅当前为具体书架时显示，「全部离线作品」视图隐藏）；
- 点击 → 确认框（提示将移除所选 N 本的书架引用，**不删本地文件/历史**）→ `removeComicsFromShelf(shelfId, ids)`（新 store 函数，本地乐观更新 + 批量调后端）→ toast 结果；
- 后端新增批量移除端点（决策 D10）：`DELETE /bookshelves/:id/comics/batch`（body `{comicIds}`），与批量加入 `/comics/batch` 对称；同步清理 sort_keys 中的权值。

---

## 三、涉及文件

### 后端

| 文件 | 改动 |
|------|------|
| `backend/internal/models/models.go` | Bookshelf 新增 `SortKey float64`（REAL）、`SortKeys string`（TEXT JSON 权值表） |
| `backend/internal/handlers/library.go` | GetBookshelves 排序改为 `sort_key asc, sort_order asc, name asc` 且响应带 sortKey/sortKeys；CreateBookshelf 赋 `sort_key = max + 1000`；新增 `PUT /bookshelves/:id/position`（单书架移动）；`ReorderBookshelfComics` 扩展单键模式 `{comicId, sortKey}` + 全量模式向后兼容（全量时重建 sort_keys=1000*i）；新增批量移除 handler；add/remove/batch 同步维护 sort_keys |
| `backend/internal/router/router.go` | 注册 `PUT /bookshelves/:id/position`、`DELETE /bookshelves/:id/comics/batch` |
| `backend/internal/handlers/library_test.go` | 新增：position 端点、单键 order、批量移除、sort_keys 同步用例 |

### 前端

| 文件 | 改动 |
|------|------|
| `src/types/comic.ts` | Bookshelf 接口加 `sortKey?: number`、`sortKeys?: Record<string, number>` |
| `src/utils/lexoRank.ts`（新） | `between(a,b)` / `topOf(first)` / `bottomOf(last)` / `round6` / `needsReweight(a,b)` / `initialWeights(n)` |
| `src/composables/useDragReorder.ts`（新） | 拖拽原语：把手/长按触发、悬浮跟随、边缘自动滚动、落位计算、列表滚动保持 |
| `src/stores/bookshelfStore.ts` | loadBookshelves 解析 sortKey/sortKeys；新增 `moveShelfToPosition` / `moveShelfToTop` / `moveComicToPosition` / `moveComicToTop` / `reweightAllShelves` / `reweightShelfComics`（防抖 + 全量重置）/ `removeComicsFromShelf`；删除 `moveBookshelf` |
| `src/composables/useShelfQuickAdd.ts`（新） | 快捷加入共享逻辑 |
| `src/components/ShelfQuickAddToolbar.vue`（新） | 多选工具条（含可选「移出书架」插槽位） |
| `src/components/OfflineSidebar.vue` | 删 ↑/↓；加把手图标 + ⋯ 菜单（移到顶部/移动到…） |
| `src/components/BookshelfPickerOverlay.vue` | 删 ↑/↓；加把手图标 + ⋯ 菜单 |
| `src/views/offline/OfflineBookshelf.vue` | 排序视图改拖拽 + 每行 ⋯ 菜单；选择工具条加「移出书架」；接入公共 composable/工具条 |
| `src/views/offline/OfflineHome.vue` | 改用公共 composable/工具条（行为不变） |
| `src/views/offline/OfflineHistory.vue` | 加选择模式 + 快捷加入 |
| `src/views/RandomView.vue` | 结果区加选择模式 + 快捷加入（离线结果） |
| `src/views/online/OnlineHistory.vue` | 视 D8 决策（默认不改） |
| `scripts/verify-round22.mjs`（新） | 验证脚本 |

---

## 四、决策点（待商议，见提问）

- **D1 拖拽触发方式**：书架内本子与书架列表分别用「把手图标」还是「长按 300ms」？
- **D2 拖动时滚动的交互形态**：悬浮拖拽 + 边缘自动滚动（推荐）还是其他？
- **D3 书架内本子排序入口**：保留专用「↕ 排序」竖排视图改造（推荐）还是主网格直接拖拽？
- **D4 LexoRank 存储**：书架内本子用 `sort_keys` JSON 权值表（推荐）还是独立子表？
- **D5 精度阈值**：float64 + 中点保留 6 位小数 + gap < 1e-6 触发全量重置（推荐）还是调整？
- **D6 「置顶」命名**：书架列表菜单用「移到顶部」避免与「置顶到侧栏」混淆（推荐）还是统一用「置顶」？
- **D7 手气不错在线结果**：仅离线结果可快捷加入（推荐）还是允许在线本入书架？
- **D8 历史记录范围**：仅离线历史页（推荐）还是离线 + 在线历史页都支持快捷加入？
- **D9 移出书架的确认交互**：点击后弹确认框（推荐）还是直接移除？

---

## 五、验证方案

1. `npm run type-check` + `go test ./...` 无回归；`npm run build` + 同步 dist；
2. `scripts/verify-round22.mjs`：代码级校验（lexoRank 工具、position/单键 order/批量移除端点存在、↑↓ 相关代码移除、useShelfQuickAdd 接入面）；
3. 手动验证矩阵：
   - 排序：书架列表把手拖拽（侧栏/浮层）可落位；拖动中列表可滚动、边缘自动滚动；「移动到第 3 位」精确落位；「移到顶部」生效；连续在同一间隙插入 ~20 次触发一次全量重置且顺序不丢；
   - 书架内本子：排序视图拖拽 + 长按 300ms；↑↓ 按钮消失；保存后顺序正确、刷新后保持；
   - 快捷加入：手气不错（离线结果）、离线历史页长按多选 → 加入书架 → 已存在跳过计数正确；随机/历史页新抽卡/刷新后选择态清空；
   - 快捷移除：书架内多选 → 移出书架 → 本子从书架消失、本地文件与历史不受影响、其他书架不受影响；
   - 旧数据兼容：升级前已有书架（无 sort_key/sort_keys）顺序不变（回退链生效）。

---

## 六、涉及回归项

- Round10（书架排序 / 书架内排序 / 重命名 / 删除）：`POST /bookshelves/reorder`、`PUT /bookshelves/:id/order` 全量模式保留，不回归；
- Round13（置顶到侧栏 / 批量快捷加入 / 全部书架浮层）：置顶语义与按钮保留，仅排序 UI 更换；
- Round20（离线历史按 gid 去重）：离线历史页新增多选不影响去重逻辑；
- Round21（PC 新标签跳转 / 返回）：ItemCard 卡片交互不因 selectable 变化受影响（长按进入选择后抑制点击导航逻辑不变）；
- 长按冲突：书架卡片 600ms 多选长按与排序视图 300ms 拖拽长按位于不同界面/元素，互不干扰。

---

## 决策确认（用户已确认 2026-08-22）

- D1 = 把手图标为主 + 长按 300ms 补充（两处排序均按此实现；书架卡片 600ms 多选长按不受影响）；
- D2 = 悬浮拖拽 + 列表保持原生滚动 + 边缘自动滚动，松手按位置落位；
- D3 = 保留「↕ 排序」专用竖排视图并改造为拖拽 + 操作菜单（不改主网格手势）；
- D4 = 书架行新增 `sort_keys` JSON 权值表；comicIds 保留为成员集合 + 旧数据回退；
- D5 = float64 权值，中点保留 6 位小数，gap < 1e-6 视为精度用尽 → 异步全量重置（1000*i）；
- D6 = 书架列表排序菜单用「移到顶部」，书架内本子菜单用「置顶」；
- D7 = 手气不错仅离线结果可快捷加入（scope=offline 或离线结果项）；在线结果不显示加入入口；
- D8 = 快捷加入仅离线历史页（OnlineHistory 不改，维持批量下载）；
- D9 = 书架内多选「移出书架」点击后弹确认框（不删本地文件/历史）；
- D10 = 批量移除端点 `DELETE /bookshelves/:id/comics/batch`（body `{comicIds}`，与批量加入对称）。

## 待确认后再开工

决策已全部确认，等待开工指令。

---

## 实施结果（2026-08-24）

### 后端
- `models.go`：Bookshelf 新增 `SortKey float64`（书架列表权值）、`SortKeys string`（书架内本子权值表 JSON）；
- `library.go`：
  - GetBookshelves 排序改为 `sort_key asc, sort_order asc, name asc`，响应携带 `sortKey`/`sortKeys`，comicIds 按权值顺序返回（`sortedComicIDs`：有权值升序在前、无权值按数组顺序排后）；
  - CreateBookshelf 赋 `sort_key = 当前最大权值 + 1000`；
  - 新增 `MoveBookshelfPosition`（PUT /bookshelves/:id/position，单书架移动只更新一项权值）；
  - `ReorderBookshelfComics` 扩展为三态：单键 `{comicId, sortKey}` / 批量 `{sortKeys: {id: w}}`（防抖合并）/ 全量 `{comicIds}`（Round10 兼容，重建权值 1000*i）；
  - 新增 `BatchRemoveComicsFromBookshelf`（DELETE /bookshelves/:id/comics/batch，同步清理权值表）；
  - 单删/批量移除同步清理 sortKeys；`ReorderBookshelves` 全量重置同步重建 sort_key=1000*i；
- `router.go`：注册 `/bookshelves/:id/position` 与 `/bookshelves/:id/comics/batch`；
- 单测 7 组新增用例全过（position / 单键 order / 全量 order / 批量移除 / 权值排序响应 / 混合态排序）。

### 前端
- `src/utils/lexoRank.ts`（新）：between / needsReweight / reweightAll / orderByWeights / weightForInsert（float64 + 6 位小数中点 + MIN_GAP=1e-6）；
- `src/stores/bookshelfStore.ts`：`moveShelfToPosition`/`moveShelfToTop`/`moveComicToPosition`/`moveComicToTop`/`ensureShelfComicWeights`/`orderedShelfComicIds`/`removeComicsFromShelf`/`flushPendingSort`，300ms 防抖合并持久化（书架列表按项、书架内按书架合并 map 批量）；删除旧 `moveBookshelf`；
- `src/composables/useDragReorder.ts`（新）：把手即时拖拽 + 行内长按 300ms（位移>8px 视为滚动不触发）、悬浮幽灵卡、原生滚动保持、rAF 边缘自动滚动、落位指示线、拖动后抑制点击；
- `src/components/SortRowMenu.vue`（新）：⋯ 操作菜单（移到顶部/置顶 文案可配 + 移动到第 X 位弹窗输入）；
- `src/components/ShelfQuickAddToolbar.vue`（新）+ `src/composables/useShelfQuickAdd.ts`（新）：多选快捷加入/移出共享逻辑；
- `src/components/OfflineSidebar.vue` / `BookshelfPickerOverlay.vue`：移除 ↑↓，接入把手拖拽 + 操作菜单（侧栏拖拽按置顶组换算全局位置）；
- `src/views/offline/OfflineBookshelf.vue`：排序视图改拖拽（把手 + 长按 300ms）+ 每行 ⋯ 菜单（置顶/移动到第 X 位）；「取消」= 冲刷增量后全量还原进入时顺序，「完成」= 冲刷增量；选择工具条新增「🗑️ 移出书架」（确认框，不删本地/历史）；展示顺序改由权值推导；
- `src/views/offline/OfflineHome.vue`：改用共享 composable/工具条（行为不变，纯重构）；
- `src/views/offline/OfflineHistory.vue`：新增长按多选 + 快捷加入书架；
- `src/views/RandomView.vue`：结果区可多选（仅离线结果响应），快捷加入书架，重新抽卡自动清空选择态；
- `src/styles/dragSort.css`（新，main.ts 引入）：把手 / 幽灵卡 / 落位指示线全局样式。

### 验证
- `npm run type-check`、oxlint、`npm run build` 全过；`go test ./...` 全过；
- `scripts/verify-round22.mjs`（静态断言）全过；
- `scripts/e2e-round22.mjs`（浏览器端到端，msedge headless + 测试库临时副本）17 项全过：排序视图把手拖拽 / 行内长按 300ms 拖拽 / 移动到第 3 位 / 置顶 / 刷新持久化 / 侧栏把手拖拽 / 浮层把手拖拽 / 浮层移到顶部 / 书架内多选移出（本地库不受影响）/ 离线历史快捷加入 / 手气不错（离线）快捷加入 / 取消选择模式。
