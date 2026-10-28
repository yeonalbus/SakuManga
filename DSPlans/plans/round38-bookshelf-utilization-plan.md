# Round38：书架利用率优化 —— 未读仪表盘 + 抽卡台

> 起因：书架定位是「系列书籍的集合」，用户已手工整理出 22 个系列书架，但实际只有置顶的 2 个在用，其余 20 个几乎不被打开。
> 本 Round 不解决「怎么整理」，只解决「整理好的东西怎么被消费掉」。

## 一、现状体检（运行库实测）

数据来源：`Y:\SakuManga\manga.db`（用户实机运行库，只读查询）。

| 指标 | 数值 |
| --- | --- |
| 书架总数 | 22（置顶 2 个：配菜 8 本、很好用 1 本） |
| 架内收藏（去重） | 114 本（含失效引用 1 个 → **有效 113 本**） |
| 其中从未读过（`read_count = 0`） | **75 本（占有效本子 66.4%）** |
| 已读（`read_count > 0`） | 38 本 |
| 架内失效引用 | 1 个（`破魔の巫女系列`；后端统计已排除：不计未读、不作封面） |
| 离线库总量 | 3986 本（书架覆盖率 2.9%） |
| 历史记录 | 335 条 |

按书架拆开（节选，按已读降序）：

| 书架 | 架内 | 已读 |
| --- | --- | --- |
| 配菜 📌 | 8 | **8** |
| 很好用 📌 | 1 | **1** |
| スレイブボール洗脳 | 15 | 7 |
| マリー触手堕ち | 5 | 5 |
| onigirikao | 16 | **1** |
| 魔法少女ゆーしゃちゃん | 10 | **0** |
| 退魔ノ隷刻 | 9 | **0** |
| JK退魔部 / 聖僧査官白蓮 / 魔薬捜査官 / 聖裁の乙女 / 銀兎神装アイリス / 退魔部敗北録 | 5 / 4 / 4 / 3 / 3 / 2 | **全 0** |

### 病因判断

1. **书架是「收藏动作的终点」，不是「阅读动作的起点」**：114 本收藏里 75 本从未打开；越新建的书架越零消费（后 12 个书架几乎全 0 已读）。
2. **用得动的恰好是已读 100% 的**（配菜、很好用）→ 书架本身能用，只是没跟阅读接上。
3. **大架反而读不动**（onigirikao 16 本读 1、魔法少女 10 本读 0）→ 缺「小步消费」机制。
4. **系统语义是反的**：抽卡页有 `recoExcludeShelf`（抽卡时排除已在书架的本子），却没有「从书架里抽」——代码把书架定义成「已处理区」而非「待消费区」。

结论：只做「总览页」只能让书架更好看，不会提高打开率；必须把**消费动作**做进书架入口。

## 二、讨论结论（与用户对齐）

| 议题 | 结论 | 理由 |
| --- | --- | --- |
| 路线选择 | **B：未读仪表盘 + 抽卡台** | 一次改动同时治「入口深度」与「收藏后不读」；纯陈列墙只治入口，而用户不缺入口、缺理由 |
| 书架墙形态 | 封面网格 + `未读 N/M` 徽标 + 每卡「🎲 抽一本未读」 | 把「76 本债务」可视化，并把「打开书架」直接变成「开始阅读」 |
| 「未读」口径 | **`readCount == 0`** | 与后端抽卡「排除读过的」（`read_count <= 0`）口径完全一致，前端/后端语义统一 |
| 封面策略 | 首批自动取架内第一本；**手动指定留 R5** | 自动封面零后端改动即可上线；手动指定需新增 `cover_comic_id` 字段与接口，排在后续 |
| 统计口径来源 | **后端 `GET /bookshelves` 返回 `unreadCount` / `coverUrl`** | 避免书架墙耦合「全库已加载」这一隐含前提，未来离线列表改懒加载也不会坏 |
| 首批范围 | **R1 + R2 + R3** | 先落地「看得见 → 能直接开读」的最小闭环，上线后按真实回头率决定 R4/R5/R6 |
| 自动归档 / 漏收提示（R6） | 本 Round 不做 | 用户明确表示痛点不在整理；数据虽支持（夢双月 架内 3 / 库内 11、退魔ノ隷刻 9 / 19），仍待首批效果验证 |

## 三、设计原则

1. **每个入口必须带消费动作**：书架墙是菜单，不是陈列馆。
2. **未读是唯一主指标**：徽标、排序、抽卡全部围绕它。
3. **封面即「系列的脸」**：默认自动，可升级为手指定。
4. **不加重前端负担**：复用路由守卫已加载的 `offlineComics`，书架墙零额外请求。

## 四、R1｜书架墙总览页

新增页面 `src/views/offline/OfflineBookshelfWall.vue`，路由 `/offline/bookshelves`（name `OfflineBookshelfWall`）。

### 页面结构

```
📚 书架墙                                    [🔎 搜索书架名称…]
─────────────────────────────────────────────────────────────
22 个书架 · 收藏 114 本 · 未读 76 本          ← 统计条（点「未读 76 本」过滤有未读的书架）
排序：[自定义拖拽 ▾]  未读最多 / 本数 / 名称
─────────────────────────────────────────────────────────────
┌────────┐ ┌────────┐ ┌────────┐
│ [封面] │ │ [封面] │ │ [封面] │   ← 封面：架内第一本 / 渐变占位+首字
│ 配菜📌 │ │ 夢双月 │ │ 魔法…  │
│ ✓ 已清空│ │ 未读2/3 │ │ 未读10/10│  ← 未读徽标（0 → ✓ 已清空）
│ 8 本   │ │ 3 本   │ │ 10 本  │
│ 🎲 抽一本│ │ 🎲 抽一本│ │ 🎲 抽一本│
└────────┘ └────────┘ └────────┘
```

### 交互细节

- 卡片主体点击 → 现有书架页 `/offline/bookshelf?id=<shelfId>`。
- 卡片右上 hover 操作：📌 置顶/取消、✎ 重命名、✕ 删除（复用 `bookshelfStore` 现有 action 与 `modal` 确认）。
- 卡片底部 `🎲 抽一本未读` → 走 R3 的结果卡流程；该架无未读时按钮置灰并提示「已清空」。
- 排序「自定义」支持拖拽，复用 `src/composables/useDragReorder.ts`（把手触发、幽灵卡、落位指示线）。
- 拖拽落位后走 `moveShelfToPosition`（LexoRank 单点移动，已有防抖持久化）。
- 封面图片 `onerror` 回退占位（封面缓存可能未生成）。
- 统计时过滤架内已失效的 comicId（当前 1 个）。

### 改动文件

| 文件 | 改动 |
| --- | --- |
| `src/views/offline/OfflineBookshelfWall.vue` | **新增**：书架墙页面（统计条 / 排序 / 搜索 / 卡片网格 / 拖拽 / 卡片操作） |
| `src/router/index.ts` | 注册 `/offline/bookshelves` |
| `src/stores/bookshelfStore.ts` | `BookshelfDTO` 与本地映射加 `unreadCount` / `coverUrl`；新增 `shelfStats`（总计书架数 / 收藏数 / 未读数）与 `shelfUnreadCount(shelf)` 辅助；`loadBookshelves` 透传新字段 |
| `src/types/comic.ts` | `Bookshelf` 加 `unreadCount?: number`、`coverUrl?: string` |
| `backend/internal/handlers/library.go` | `GetBookshelves` 响应加 `unreadCount` 与 `coverUrl`（见下） |

### 后端数据契约

`GET /api/v1/bookshelves` 每个书架新增两个字段：

```json
{
  "id": "shelf-1786118069618",
  "name": "配菜",
  "count": 8,
  "comicIds": ["..."],
  "pinned": true,
  "sortKey": 1000,
  "sortKeys": { "<comicId>": 1000 },
  "unreadCount": 3,
  "coverUrl": "/api/v1/comics/<comicId>/cover"
}
```

实现要点（`GetBookshelves` 内，量级仅 114 本）：

1. 收集全部书架的 `comicIds`（`sortedComicIDs(ids, sk)` 后的展示顺序）。
2. 一次 `SELECT id, read_count, cover_url FROM offline_comics WHERE id IN ?` 取回。
3. 内存聚合：`unreadCount` = 架内仍存在且 `read_count <= 0` 的本子数；`coverUrl` = 展示顺序中第一本仍存在的本子的 `cover_url`（`offline_comics.cover_url` 形如 `/api/v1/comics/<id>/cover`）。
4. 失效 id 不计入 `unreadCount`，也不作为封面来源。

> 兼容性：`count` 语义保持为架内 id 数量（不变），前端徽标用 `unreadCount / count`。

## 五、R2｜侧栏入口与未读徽标

| 文件 | 改动 |
| --- | --- |
| `src/components/OfflineSidebar.vue` | ① `🔍 全部书架（22）` 改为跳转 `/offline/bookshelves`；② 置顶书架徽标 `8` → `3/8`（未读/总），未读 0 显示 `8 ✓`；③ 书架折叠区底部加一行 `📖 未读 76 本`（点击跳书架墙并自动过滤未读） |

- `BookshelfPickerOverlay` 浮层**保留**：多选「加入书架」场景（`mode="add"`，用于 OfflineHome / OfflineHistory / OfflineBookshelf / RandomView）不变。
- 侧栏徽标数据来自 `/bookshelves` 的 `unreadCount`，不依赖 `offlineComics` 是否加载。

## 六、R3｜整架导入本地阅读清单

> **二轮调整（用户反馈）**：初版此处为「🎲 抽一本未读」。按用户要求改为**导入阅读清单**——
> 不再在书架里抽卡，而是把整架作品一键追加到候读队列，免去每次进阅读清单页点「➕ 从书架导入」。
> 初版抽卡组件 `ShelfPickOverlay.vue` 与抽取逻辑已删除，未读统计（徽标/统计条）保留。

新增 `src/composables/useShelfImport.ts`：

- `importShelfToReadingList(shelf)` → `{ added, skipped, total }`：按书架展示顺序（LexoRank 权值序）遍历，
  增量追加到本地清单末尾，已在清单中的跳过；仅影响清单本身，不触碰书架与本地库。
- 语义与阅读清单页现有导入**完全一致**（用户 2026-09-13 确认：追加到末尾、导入后留在书架不跳转）。

入口（书架墙每卡 + 书架页头部各一个）：

```
📋 导入清单
   ↓
toast：已将「夢双月」3 本导入阅读清单（跳过 0 本）
      /「夢双月」的作品都已在阅读清单中，无新增
      / 书架「X」暂无可导入作品
```

- 空书架按钮置灰禁用。
- 导入后**留在书架**，便于连续导入多个系列（用户确认）。
- 底层复用 `readingStore.addToReadingList`（store 内部 200ms 防抖合并为一次后端写入）。

## 七、R5｜手动指定封面（二轮补做）

| 层 | 改动 |
| --- | --- |
| 后端 | `models.Bookshelf` 加 `CoverComicID`（AutoMigrate 自动加列）；`GetBookshelves` 返回 `coverComicId`，`coverUrl` 按「手指定优先」解析；新增 `PUT /bookshelves/:id/cover`（`comicId` 空串 = 恢复自动；非架内本子 → 400；他人书架 → 404）；`services/comic_refs.go` 同步封面引用（本子被替换 → 迁移；被删除 → 清空回退自动） |
| 前端 | 新增 `components/ShelfCoverPickerOverlay.vue`（架内本子缩略图网格点选 + 「↺ 恢复自动封面」）；`stores/bookshelfStore.ts` 新增 `setBookshelfCover`；`types/comic.ts` 加 `coverComicId` |
| 入口 | 书架墙卡片 hover 工具条 `🖼`；书架页头部 `🖼 封面` |

封面优先级：**手动指定 > 架内展示顺序第一本 > 首字占位砖**。

## 八、后续待办

| 编号 | 内容 | 状态 |
| --- | --- | --- |
| R4 | 抽卡页「书架池」（`random.go` 加 `includeShelfIDs` + 前端范围下拉） | **用户确认不做**（2026-09-13：没有该需求） |
| R5 | 手动指定封面 | **已补做**（见上） |
| R6 | 系列续读（详情页/阅读器「同系列未读还有 N 本」）与漏收提示 | **用户确认不做**（同上） |

## 八、验收清单（2026-09-13 实机跑通）

数据基准（已用隔离环境实测校准）：**22 个书架 / 架内 114 id（有效 113）/ 已读 38 / 未读 75**。

- [x] 书架墙显示 22 张卡片，封面按架内第一本渲染，无封面时显示占位块（实测封面接口 142 次请求全 200）
- [x] 统计条数值正确（22 / 113 / 75）
- [x] 未读徽标与实库一致：配菜 `✓ 已清空`、很好用 `✓ 已清空`、魔法少女ゆーしゃちゃん `未读 10/10`、onigirikao `未读 15/16`、スレイブボール洗脳 `未读 8/15`、夢双月 `未读 2/3`
- [x] 点「未读 75 本」只显示有未读的书架（16 张卡片，配菜/很好用/マリー/異花/触手/铃兰被过滤）
- [x] 卡片点击进入对应书架页（`/offline/bookshelf?id=<id>`，实测标题 `onigirikao`）
- [x] 卡片操作可用：置顶 / 重命名 / 删除
- [x] 排序模式（单列拖拽）显示 22 行，落位走 LexoRank 单点持久化
- [x] 侧栏「🔍 全部书架（22）」跳书架墙；多选「加入书架」浮层（add 模式）仍正常
- [x] 侧栏置顶书架徽标显示 `未读/总`，未读 0 显示 `8 ✓`
- [x] 「📋 导入清单」：整架作品增量追加到本地清单（实测 0 → 3 本）；重复导入提示「已在阅读清单中，无新增」且不产生重复项
- [x] 书架页头部同样有「📋 导入清单」入口；空书架按钮置灰禁用
- [x] 读书后未读数下降：模拟阅读一本后 未读 75 → 74、onigirikao `15/16` → `14/16`
- [x] **封面无重叠**：封面加载成功后不再渲染占位首字（23/23 张已加载、重叠数 0；封面区中心点命中的是 `img` 而非 `span`）
- [x] **手动指定封面**：浮层候选数 = 架内有效本子数；选第 3 本后后端 `coverComicId`/`coverUrl` 与书架墙卡片封面同步
- [x] **恢复自动封面**：`coverComicId` 清空且 `coverUrl` 回退架内第一本
- [x] 旧库升级自动加列（副本库 `cover_comic_id` 实测 0 → 1，无需手写迁移）
- [x] 失效引用（1 个）不影响未读统计与封面渲染
- [x] `cd backend && go test ./...` 通过（新增 `TestGetBookshelvesUnreadAndCover` / `TestLoadBookshelfStatsEmpty` / `TestGetBookshelvesManualCover` / `TestSetBookshelfCover` / 封面引用迁移断言）
- [x] `npm run type-check` 通过
- [x] 实机无 console error / pageerror

## 九、实施记录（首批 R1+R2+R3 已完成）

### 交付物

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 后端 | `backend/internal/handlers/library.go` | `GetBookshelves` 返回 `unreadCount` / `coverUrl`；新增 `loadBookshelfStats` 一次 `IN` 查询聚合（失效引用排除、封面取展示顺序中第一个带封面的本子） |
| 后端 | `backend/internal/handlers/library_test.go` | 新增 2 个用例：未读/封面聚合（含失效引用、封面回退、跨用户隔离）、空书架集合零值统计 |
| 前端 | `src/views/offline/OfflineBookshelfWall.vue` | **新增**书架墙（统计条 / 未读过滤 / 搜索 / 排序方式 / 卡片网格 / 排序模式拖拽 / 卡片操作） |
| 前端 | `src/components/ShelfPickOverlay.vue` | **新增**抽一本未读结果卡浮层（再抽一张不重复、空池升级全库未读、开始阅读） |
| 前端 | `src/composables/useShelfPick.ts` | **新增**未读口径与抽取动作（`shelfUnreadCount` / `shelfUnreadComics` / `shelfWallSummary` / `pickUnreadFromShelf` / `pickUnreadFromAllShelves`） |
| 前端 | `src/stores/bookshelfStore.ts` | DTO 与本地映射透传 `unreadCount` / `coverUrl`；新增 `refreshBookshelves`（失败保留旧数据）；加入/移出本子后静默刷新统计 |
| 前端 | `src/types/comic.ts` | `Bookshelf` 加 `unreadCount` / `coverUrl` |
| 前端 | `src/components/OfflineSidebar.vue` | 「全部书架」改跳书架墙（保留多选 add 浮层）、置顶徽标 `未读/总`（清零转绿 `8 ✓`）、新增 `📖 未读 N 本` 入口 |
| 前端 | `src/views/offline/OfflineBookshelf.vue` | 书架页头部新增「🎲 抽一本未读」入口（带未读数角标） |
| 前端 | `src/router/index.ts` | 注册 `/offline/bookshelves` |
| 文档 | `PROJECT_TREE.md` | 结构树与「改哪个功能找哪个文件」对照表同步 |

### 与计划的偏差

1. **拖拽排序落地为「排序模式」**：书架墙默认是封面网格（多列），`useDragReorder` 是按「行中线 + 行下标」设计的垂直模型，网格直拖的落位指示线会失准。故与书架页 Round22 一致，进入排序模式后用单列紧凑行拖拽（交互语言统一，无新原语）。
2. **封面首批为自动（架内第一本）**：手动指定封面（R5）需新增 `cover_comic_id` 列与接口，按讨论排在后续。
3. **侧栏 `navigate` 浮层入口移除**：书架墙已覆盖浮层的全部管理能力（置顶/改名/删除/排序）并多出未读维度，浮层仅保留 `add` 模式（多选加入书架）。
4. **统计条「收藏 113 本」**：前端去重后仅统计**仍存在**的本子（114 个 id − 1 个失效 = 113），与后端未读口径一致。

### 实机验证

- 环境：隔离库副本（`Y:\SakuManga\manga.db` 复制到 `Test/pw-bug/round38/`）+ 本次编译的后端（`-headless`，8081）+ `dist` 经 `proxy-server.mjs`（5200）；脚本与截图见 `Test/pw-bug/round38/`（该目录已被 .gitignore 忽略，勿提交）。
- 结论：验收清单全部通过，无 console error / pageerror。
- 部署提醒：前端需 `npm run build-only` 并同步 `backend/webui/dist`；后端需重新编译打包 exe，替换 `Y:\SakuManga\SakuManga.exe` 后重启（本 Round 无数据库结构变更）。


### 二轮（2026-09-13 用户反馈）

| 项 | 内容 |
| --- | --- |
| ① 封面重叠 bug | **根因**：占位首字带 `opacity: 0.45`，而 `opacity < 1` 会创建**层叠上下文**，使其与 `z-index: auto` 的绝对定位封面图处于同一层，绘制顺序改由 DOM 顺序决定——img 在前、字在后，于是**字压在封面上**（实测 `elementFromPoint(封面中心)` 命中的是 `span.cover-fallback`，而 `imgLoaded=true`）。<br>**修复**：改用封面状态机 `loading / ok / fail`——加载成功即**不渲染**占位字（不再依赖层级遮挡）；同时给封面 `z-index: 1`、徽标 `z-index: 2`、卡片工具条 `z-index: 3`，保证加载中与 hover 操作不被封面盖住。书架墙卡片与排序模式行两处同结构一并修复。 |
| ② 抽卡 → 导入清单 | 删除 `ShelfPickOverlay.vue` 与抽取逻辑（`useShelfPick.ts` → 重命名为 `useShelfStats.ts`，只保留未读统计）；新增 `useShelfImport.ts`；书架墙卡片与书架页头部按钮改为「📋 导入清单」。语义沿用阅读清单页既有导入（追加末尾、已在清单跳过），导入后留在书架不跳转（均经用户确认）。 |
| ③ R5 补做 | 手动指定封面全链路落地（后端字段/接口/引用迁移 + 前端浮层与两处入口）。R4、R6 经用户确认**不做**。 |

二轮实机回归（`Test/pw-bug/round38/verify-round38b.mjs`，隔离库副本 23 架）：

- 封面：23/23 张已加载、**重叠数 0**、封面区中心命中 `img`
- 导入：`0 → 3` 本；重复导入提示「已在阅读清单中，无新增」且不产生重复项；书架页入口存在
- 指定封面：浮层 3 个候选 → 选第 3 本后后端 `coverComicId`/`coverUrl` 与书架墙卡片同步；「恢复自动封面」回退架内第一本
- 旧库升级：`cover_comic_id` 列由 AutoMigrate 自动添加（0 → 1）
- 无 console error / pageerror

## 十、风险与注意

- **AutoMigrate / 部署**：本 Round 首批不新增数据库列（仅 `GetBookshelves` 响应扩展），无需迁移；R5 才加 `cover_comic_id`，加列后需重启 `Y:\SakuManga\SakuManga.exe` 生效。
- **封面 404**：封面缓存可能未生成 → `onerror` 回退占位，不显示破图。
- **未读语义**：`read_count` 仅在离线阅读器内自增（在线浏览不计），故「未读」严格等于「没用本地阅读器打开过」，与讨论口径一致。
- **性能**：`GetBookshelves` 多一次 `IN` 查询（114 id）可忽略；书架墙渲染 22 张卡片无虚拟化必要。
- **前端兜底**：若后端 `unreadCount` 缺失（旧后端），前端回退用 `offlineComics` 本地计算，保证降级可用。
- **层叠上下文陷阱（二轮教训）**：`opacity < 1`、`transform`、`filter` 等都会创建层叠上下文，使其与 `z-index: auto` 的定位元素**同层**，此时绘制顺序由 DOM 顺序决定，而非「定位元素必在上」。同类场景（占位/骨架与真实内容叠加）一律改为**互斥渲染**，不要依赖层级遮挡。
- **封面缓存**：`coverUrl` 指回 `/api/v1/comics/<id>/cover`，首次访问需后端生成缓存；生成期间图片未完成加载，此时仍显示占位首字（状态机 `loading`），加载成功即自动移除，不会重叠。
- **R5 部署**：新增 `cover_comic_id` 列由 AutoMigrate 自动添加，需重启 `Y:\SakuManga\SakuManga.exe` 生效（无手写迁移）。
