# Round13 快捷加入书架 + 侧栏置顶/检索浮层优化计划

## 背景与需求

| 编号 | 类型 | 内容 |
|------|------|------|
| Opt1 | 优化 | 快捷加入：离线页面多选后直接加入书架（仅离线；后端批量接口） |
| Opt2 | 优化 | 侧栏优化：书架太多会顶掉「工具/系统」设置项；方案 = 侧栏常驻高频（置顶 5 个）+ 检索浮层（改良 C+B 混合）+ 全侧栏滚动 |

---

## 一、Opt2 侧栏优化（用户已确认方案：C+B 混合）

### 设计总览

- 侧栏「书架」区**只常驻展示用户手动置顶（Pin）的最多 5 个书架**，保持侧栏紧凑；
- 提供 **「🔍 全部书架」检索浮层入口**：点击弹出浮层，列出全部书架，支持搜索、进入、置顶/取消置顶、改名、删除、排序；
- **整个侧栏加滚动**（`overflow-y: auto` + 细滚动条）：即使内容再多，「工具」「系统」也能滚动到达（多端适配滑块，兜底防顶掉）；
- **置顶上限 5**：超过时前端提示「侧栏最多置顶 5 个书架」。

### 2a. 后端（书架置顶字段 + 接口）

- [`models.go Bookshelf`](backend/internal/models/models.go:113) 加字段：`Pinned bool `gorm:"default:false" json:"pinned"``（AutoMigrate 自动加列，无需手工迁移）；
- [`library.go GetBookshelves`](backend/internal/handlers/library.go:71) 响应 DTO 增加 `"pinned": s.Pinned`；
- 新增 `PUT /api/v1/bookshelves/:id/pin`（[`library.go`](backend/internal/handlers/library.go) 新 handler `SetBookshelfPinned`）：body `{ pinned: bool }`，校验归属后更新；
- [`router.go`](backend/internal/router/router.go:160) 注册 `api.PUT("/bookshelves/:id/pin", libraryHandler.SetBookshelfPinned)`。

### 2b. 前端 Store

- [`bookshelfStore.ts`](src/stores/bookshelfStore.ts) `Bookshelf` 本地类型加 `pinned?: boolean`；`loadBookshelves` 透传；
- 新增 `setBookshelfPinned(id, pinned)`：乐观更新 + 调 PUT :id/pin；
- 新增 computed：`pinnedBookshelves = bookshelves 中 pinned 的前 5 个（按现有 sort_order 顺序）`；
- 置顶顺序来源：**复用现有 `sort_order`**（书架列表全局顺序）——侧栏置顶区显示置顶书架按 sort_order 的相对顺序，用户可用 ↑/↓ 调整（moveBookshelf 现有逻辑天然一致）。

### 2c. 侧栏 UI（[`OfflineSidebar.vue`](src/components/OfflineSidebar.vue)）

- 书架折叠区改为：
  - 无置顶书架时：提示文案「💡 在「全部书架」中点 ⭐ 置顶常用书架」+「🔍 全部书架」按钮 +「➕ 新建书架」；
  - 有置顶书架时：列出 `pinnedBookshelves`（最多 5 个），每行 = 名字 + 数量 + hover ↑/↓（调整全局顺序）+ ✕ 取消置顶；
  - 底部常驻「🔍 全部书架」按钮 → 打开检索浮层；
- **整个 `.sidebar`（或 `.nav-menu`）加 `overflow-y: auto` + 细滚动条**（Webkit/标准 scrollbar 样式），保证任何情况下「工具」「系统」可滚动到达。

### 2d. 检索浮层（新组件 `BookshelfPickerOverlay.vue`）

- 复刻 Round11 shelf-import 弹层样式（Teleport + mask/panel）；
- **搜索框**：按书架名称实时过滤；
- **书架列表**：每行 = 名称 + 数量 + hover 操作：
  - 「进入」：点击行 → 跳转 `/offline/bookshelf?id=xx` 并关闭；
  - 「⭐ 置顶 / 取消置顶」：未置顶显示 ⭐，已置顶高亮；**已置顶 5 个时其余书架的 ⭐ 置灰 + toast 提示上限**；
  - 「↑/↓」：全局顺序调整（moveBookshelf）；
  - 「✎ 改名」「✕ 删除」（复用现有确认弹窗）；
  - 「➕ 新建书架」：弹输入框创建后自动置顶？→ 默认不自动置顶（避免超限），仅创建并刷新。
- **复用场景**：侧栏「全部书架」（navigate 模式）+ 优化1 多选加入（add 模式，见下）。组件 props：`open / mode: 'navigate' | 'add' / selectedCount?`，emits：`close / pick(shelf)`。

---

## 二、Opt1 快捷加入书架（仅离线页面）

### 2a. 后端批量接口

- 新增 `POST /api/v1/bookshelves/:id/comics/batch`（[`library.go`](backend/internal/handlers/library.go) 新 handler `BatchAddComicsToBookshelf`）：
  - body `{ comicIds: [] }`；
  - 读取书架现有 comicIds → 去重合并（已在书架中的计入 skipped）→ 写回 → 更新 Count；
  - 返回 `{ added: n, skipped: m }`；
- [`router.go`](backend/internal/router/router.go:160) 注册 `api.POST("/bookshelves/:id/comics/batch", libraryHandler.BatchAddComicsToBookshelf)`；
- 单测：`library_test.go`（或 services 测试）覆盖去重/计数/归属校验。

### 2b. 前端批量加入

- [`bookshelfStore.ts`](src/stores/bookshelfStore.ts) 新增 `addComicsToShelf(shelfId, comicIds)`：乐观更新本地（去重 + count）+ 调批量接口，返回 `{added, skipped}`；
- [`OfflineHome.vue`](src/views/offline/OfflineHome.vue) 与 [`OfflineBookshelf.vue`](src/views/offline/OfflineBookshelf.vue) 多选工具条增加 **`📥 加入书架`** 按钮（已选 >0 时可用）→ 打开 BookshelfPickerOverlay（`mode='add'`，显示已选数量）→ 选择书架 → `addComicsToShelf` → toast「已加入 N 本（跳过 M 本已在书架）」→ 退出多选模式；
- 书架选择浮层在 add 模式下点击行即加入并关闭，无需二次确认（toast 反馈即可）。

---

## 三、涉及文件

| 文件 | 改动 |
|------|------|
| `backend/internal/models/models.go` | Bookshelf 加 `Pinned` 字段 |
| `backend/internal/handlers/library.go` | GetBookshelves 输出 pinned；新增 SetBookshelfPinned / BatchAddComicsToBookshelf |
| `backend/internal/router/router.go` | 注册两条新路由 |
| `src/stores/bookshelfStore.ts` | pinned 透传 + setBookshelfPinned + pinnedBookshelves + addComicsToShelf |
| `src/components/OfflineSidebar.vue` | 置顶书架区 + 全部书架入口 + 侧栏滚动 |
| `src/components/BookshelfPickerOverlay.vue` | 新组件：检索浮层（navigate/add 双模式） |
| `src/views/offline/OfflineHome.vue` | 多选工具条加「📥 加入书架」 |
| `src/views/offline/OfflineBookshelf.vue` | 同上 |
| `src/App.vue` | `.sidebar` 或 `.nav-menu` 滚动样式 |
| `scripts/verify-round13.mjs` | 新增验证脚本 |
| `plans/round13-bookshelf-sidebar-plan.md` | 本计划 |

---

## 四、决策点（请确认）

- **D1 置顶上限 5 是否可配置**：
  - A（推荐）：固定 5 个，超限 toast 提醒（实现简单、交互清晰）；
  - B：设置中心加「侧栏置顶上限」数字项（默认 5，可改 1~10）。
- **D2 置顶书架顺序来源**：
  - A（推荐）：复用现有全局 `sort_order`（侧栏置顶区 ↑/↓ 即调整全局顺序，与现有排序交互完全一致）；
  - B：置顶书架使用独立 PinOrder（置顶顺序与全局列表顺序解耦，多一套排序逻辑）。
- **D3 全侧栏滚动范围**：
  - A（推荐）：`.sidebar` 整体滚动（含 logo/模式切换，顶栏 sticky 效果：logo 固定、菜单滚动）——多端最稳；
  - B：仅 `.nav-menu` 滚动（logo 常驻不滚）。

---

## 五、验证方案

1. `go test ./...`（批量接口 + 置顶接口单测）；`npm run type-check` + `npm run build`，同步 `backend/webui/dist`；
2. 新增 `scripts/verify-round13.mjs`（本地后端 + 扫描 MangaExamlpe 2 本）：
   - **Opt1**：离线首页长按多选 2 本 → 加入书架 → 书架 count=2；再选同一本加入 → skipped 提示；书架页多选同样验证；
   - **Opt2**：创建 6 个书架 → 置顶 5 个成功、第 6 个提示上限；侧栏只显示 5 个置顶书架；「全部书架」浮层可搜索/进入/取消置顶；侧栏滚动可用、「工具/系统」可见；
3. 回归：Round12 面板验证脚本复跑（确保侧栏滚动不影响面板布局）。

---

## 六、提交计划（按模块拆分）

1. `feat(backend): 书架置顶字段 + 批量加入接口` —— 后端代码 + 单测；
2. `feat(frontend): 侧栏置顶书架/检索浮层 + 离线多选快捷加入书架` —— 前端 + 计划 + 验证脚本；
3. `chore(build): 同步内嵌前端构建产物到 backend/webui/dist`。

## 决策确认（用户已确认 2026-08-21）

- **D1 = A**：置顶上限固定 5 个，超限 toast 提醒；
- **D2 = A**：置顶书架顺序复用现有全局 sort_order（↑/↓ 即全局排序）；
- **D3 = A**：侧栏整体滚动（logo 常驻、菜单区滚动）。

## 实施结果

- 后端：Bookshelf 加 `Pinned` 字段；`PUT /bookshelves/:id/pin` + `POST /bookshelves/:id/comics/batch`；单测通过。
- 前端：`BookshelfPickerOverlay` 检索浮层（navigate/add 双模式，搜索/置顶/排序/改名/删除/新建）；`OfflineSidebar` 置顶书架区（≤5）+ 全部书架入口；`App.vue` 侧栏整体滚动；`OfflineHome`/`OfflineBookshelf` 多选工具条加「📥 加入书架」。
- `scripts/verify-round13.mjs` 全部通过（多选加入/去重跳过/置顶 5 个/第 6 个拦截/侧栏常驻/系统组可达）。

## 待确认后再开工

请确认 D1~D3 后开始实现。
