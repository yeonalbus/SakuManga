# Round37：搜刮书签拖动排序 + 分段收纳

> 起因：搜刮书签目前平铺在在线模式侧栏，书签一多既不好看、又会把「🎲 工具 / 系统」组顶出视口。
> 本 Round 落地「拖动排序」并解决平铺长度失控问题。

## 一、讨论结论（与用户对齐）

| 议题 | 结论 | 理由 |
| --- | --- | --- |
| 要不要做成书架那样的抽屉/浮层 | **不做** | 抽屉是「组织工具」，书签是「临时位置快照」，生命周期短、数量级小；多一次点击直接惩罚最高频动作（一键回现场），与功能定位冲突；浮层还带来遮罩/焦点/滚动位置的额外状态管理成本 |
| 平铺长度失控怎么办 | **分段收纳（C2）** | 常驻区前 N 条平铺 + 「⋯ 其余 N 条」**原位内联展开**（无遮罩、无焦点转移，展开后仍是原位单击直达） |
| 常驻条数 N | **6** | 每行约 40px，6 条约 240px，1080p 下不挤掉下方分组，同时保持拖拽可及 |
| 新建书签插在哪 | **追加到末尾** | 侧栏顺序稳定 → 位置记忆/肌肉记忆成立（这是「快速定位」的前提）；刚创建的书签创建时即已跳转定位，不依赖侧栏查找 |
| 行内拖拽把手怎么摆 | **左侧图标位扩成 hover 操作区（⠿ ✕）** | 日常显示类型图标 🔍/🏠，hover 时该区由 16px 扩到 34px、图标淡出换成 ⠿ + ✕；右侧日期右对齐**不受影响**（不牺牲发布时间可读性），把手位置固定 |
| 要不要快捷键检索面板（Ctrl+K） | **本 Round 不做** | 等分段收纳上线后看实际书签数量再定；避免为可能不需要的场景提前投入 |

### 与书架「置顶 + 浮层」模式的语义区别

书架的侧栏常驻组是**用户手选 ⭐ 置顶**（引入 `pinned` 字段的第二套状态）；本 Round 的书签常驻区是**拖动排序 + 自动截断前 N**，不引入新状态，只是复用书架的拖拽/排序基建与视觉语言。

## 二、后端

与 Round22 书架排序**同构**（LexoRank 单点移动），AutoMigrate 自动加列，老数据无需手写迁移。

| 文件 | 改动 |
| --- | --- |
| `internal/models/scrape_bookmark.go` | 新增 `SortKey float64`（`gorm:"default:0"`） |
| `internal/services/scrape_bookmark.go` | DTO 加 `sortKey`；`ListScrapeBookmarks` 改 `Order("sort_key ASC, id ASC")`；`CreateScrapeBookmark` 赋 `max(sort_key)+1000`；新增 `MoveScrapeBookmarkPosition` / `ReorderScrapeBookmarks` |
| `internal/handlers/scrape_bookmark.go` | 新增 `MovePosition` / `Reorder` |
| `internal/router/router.go` | `PUT /scrape-bookmarks/:id/position`、`POST /scrape-bookmarks/order` |
| `internal/services/scrape_bookmark_test.go` | 补排序输出 / 单点移动 / 全量重置 / 跨用户隔离用例 |

### 排序语义（兼容升级）

- 老数据 `sort_key = 0` → 按 `id ASC` 兜底，**升级前后侧栏顺序完全一致**（原实现即 `Order("id ASC")`）。
- 新建书签 `sort_key = max+1000`：老数据全 0 时新书签 = 1000，落在末尾 → 满足「追加末尾」。
- `sort_key` 同为 0 只可能出现在「从未拖动过」的状态，此时按 id 兜底即创建顺序，无歧义。
- 一旦拖动 → 前端先按**当前展示顺序**全量赋权（`1000*(i+1)`，含新书签本身），覆盖掉历史 `max+1000` 值，不会出现权值撞值导致的顺序错乱。

### 路由命名说明

`PUT /scrape-bookmarks/order` 会与既有 `PUT /scrape-bookmarks/:id` 在 Gin 路由树中触发 wildcard/static 冲突 panic；
故与书架一致，全量重置走 **`POST /scrape-bookmarks/order`**，单点移动走 `PUT /scrape-bookmarks/:id/position`（`:id` 的子路径，安全）。

## 三、前端

| 文件 | 改动 |
| --- | --- |
| `src/types/comic.ts` | `ScrapeBookmark` 加 `sortKey?: number` |
| `src/stores/scrapeBookmarksStore.ts` | `fromRaw` 恢复 `sortKey`；新增 `moveBookmarkToPosition`（**本地同步重排** + 单点防抖持久化；老数据首次拖动走「全量赋权 + 落位」一次提交）、`persistBookmarkOrder`、`flushPendingBookmarkSort`；删除回滚排序改用 `sortKey` |
| `src/components/OnlineSidebar.vue` | 分段收纳 + 拖动排序 + 组折叠（记忆 localStorage）+ 触摸设备把手/删除常驻显示 |

### 侧栏结构

```
🔖 书签  ▾                    🔍  🧹        ← 组标题（点标题折叠，状态记忆）
  🔍 某某关键词        09-12                ← 常驻区：前 6 条，顺序由拖动决定
  🏠 首页              09-10                    hover：⠿ 拖动 / ✕ 删除
  ⋯ 其余 12 条                              ← 仅总数 > 6 时出现，点击原位内联展开
```

- 展开后书签列表容器限高（`max-height`）并内部滚动，避免继续顶掉下方分组。
- 拖动复用 `src/composables/useDragReorder.ts`（把手触发、幽灵卡、落位指示线、边缘自动滚动），滚动容器为该书签列表元素本身。
- 书签 ≤ 6 时形态与改造前一致，**零额外点击**。
- 触摸设备（`@media (hover: none)`）无 hover：把手 ⠿ 与删除 ✕ 常驻显示、类型图标让位——否则移动端既无法拖动排序，也没有删除入口（✕ 原本只在 hover 出现）。

## 四、验收清单

- [ ] 书签少于等于 6 条：侧栏形态与旧版一致，点击直达不受影响
- [ ] 书签超过 6 条：出现「⋯ 其余 N 条」，点击原位展开、再点收起
- [ ] 展开后列表限高滚动，下方「🎲 工具 / 系统」分组仍可见
- [ ] 拖动把手可排序常驻区内条目；展开后可在全列表内跨区搬移
- [ ] 拖动落位不误触跳转（suppressClick 消费）
- [ ] 刷新 / 换端后顺序保持（后端持久化）
- [ ] 老库升级后侧栏顺序不变（`sort_key=0` 按 id 兜底）
- [ ] 新建书签追加到末尾，不顶动既有常驻顺序
- [ ] 组折叠状态刷新后保持
- [ ] 触摸设备（无 hover）下把手 ⠿ 与删除 ✕ 常驻可见、可拖动
- [ ] `go test ./...` 与 `npm run type-check` 通过
