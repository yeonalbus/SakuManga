# Round10 书架/评分/清单修复与自定义排序计划

## 背景与问题

用户反馈 3 个 Bug + 3 项优化：

| 编号 | 类型 | 内容 |
|------|------|------|
| Bug1 | 显示 | 直接在本地书架页刷新 / 在本子详情界面点击书架 → 书架无法显示书籍，需先回主页再刷新 |
| Bug2 | 显示 | 离线卡片（ItemCard）评分丢失，显示为 "⭐ —" |
| Bug3 | 筛选 | 离线星级筛选失效，无法按本地配置的星级过滤/显示 |
| Opt1 | 优化 | 自定义排序：清单、书架内项目、书架顺序按自定义顺序排列 |
| Opt2 | 优化 | 单本移出：支持只从清单移出单本画廊 |
| Opt3 | 优化 | 书架修改：支持书架删除、改名 |

---

## 一、Bug1 书架显示问题

### 现状与根因（已确认）

- 书架页数据流：`shelfComics` computed（[OfflineBookshelf.vue:44](src/views/offline/OfflineBookshelf.vue:44)）从 **`offlineComics` store** 过滤出 `comic.bookshelfId === shelfId || comicIds.includes(id)` 的漫画。
- `offlineComics` 由 `fetchOfflineComics()` 填充，**调用点只有**：OfflineHome 的 onMounted/onActivated（刷新）、ComicReader、OfflineDetail（仅 404 时）、downloadTasksStore。
- `OfflineBookshelf` 的 `restoreListState()`（[OfflineBookshelf.vue:82](src/views/offline/OfflineBookshelf.vue:82)）**只恢复页码/滚动位置，从不拉取离线列表**。
- 因此：
  - 直接在 `/offline/bookshelf?id=xxx` **刷新页面**（新页面进程，store 为空）→ 书架空白；
  - 从详情页（新标签打开，store 未填充）点侧栏书架 → 同样空白；
  - 先回主页（触发 fetch）再进书架 → 正常。与用户描述完全吻合。

### 修复

**1a. [`OfflineBookshelf.vue:82`](src/views/offline/OfflineBookshelf.vue:82) `restoreListState()`**
- 在恢复页码/滚动**之前**先 `await fetchOfflineComics()`（与 OfflineHome 一致，onMounted 与 onActivated 共用同一函数，两处同时修复）。
- 防御性补充：`bookshelves` store 为空时 `await loadBookshelves()`（覆盖「刷新后 loadUserLibrary 尚未完成」的竞态）。

**1b. 书架展示顺序对齐 comicIds（与 Opt1-1b 一并做）**
- `shelfComics` 改为按 `currentShelf.comicIds` 的**数组顺序**输出（未在 comicIds 中但 bookshelfId 匹配的排在末尾），为自定义排序打基础。

---

## 二、Bug2 离线卡片评分丢失

### 现状与根因（已确认）

- 个人评分走 /ratings API → `ratingStore.myRatings`（[ratingStore.ts:9](src/stores/ratingStore.ts:9)），与 `OfflineComic.Rating`（E 站社区评分，本地下载画廊多为 0）**是两个独立数据源**。
- `ItemCard` 只读 `comic.rating`（社区评分）：
  - 大卡片 [ItemCard.vue:525](src/components/ItemCard.vue:525)：`comic.rating ? comic.rating.toFixed(1) : "—"`
  - 名片 [ItemCard.vue:442](src/components/ItemCard.vue:442)：`v-if="comic.rating"` 才显示
- `OfflineDetail.setRating`（[OfflineDetail.vue:204](src/views/offline/OfflineDetail.vue:204)）只改详情页本地 `comic.value.rating`，**不写回列表 store**，也不参与卡片展示。
- 结果：详情页打分后回列表，卡片仍显示 "—"。

### 修复

**2a. [`ratingStore.ts`](src/stores/ratingStore.ts) 新增统一「生效评分」函数**

```ts
/** 生效评分：离线优先个人评分，回退社区评分；在线保持社区评分 */
export const getEffectiveRating = (comic: Pick<ComicItem, "id" | "source" | "rating">): number => {
  if (comic.source === "offline") {
    const mine = myRatings.value[comic.id]
    if (mine && mine > 0) return mine
  }
  const r = Number(comic.rating)
  return Number.isFinite(r) && r > 0 ? r : 0
}
```

**2b. [`ItemCard.vue`](src/components/ItemCard.vue) 两处评分展示改用生效评分**
- 大卡片（525）：`displayRating ? displayRating.toFixed(1) : "—"`
- 名片（442）：`v-if="displayRating"` 且统一 toFixed(1)
- 对非 number 做 `Number()` 防御（避免后端脏数据导致 `.toFixed is not a function`）。

---

## 三、Bug3 离线星级筛选失效

### 现状与根因（已确认）

- 离线筛选在 [OfflineHome.vue:204](src/views/offline/OfflineHome.vue:204)：`(comic.rating || 0) < cfg.minRating` 用社区评分过滤。
- 本地下载画廊 rating 多为 0 → FilterDrawer 的 1-5 ⭐ 星级（[FilterDrawer.vue:322](src/components/FilterDrawer.vue:322)）永远筛不出按个人评分应命中的作品。

### 修复

**3a. [`OfflineHome.vue:204`](src/views/offline/OfflineHome.vue:204)** 过滤条件改为 `getEffectiveRating(comic) < cfg.minRating`。
- 修复后：按个人评分（1-5 星）筛选生效；未评分的作品按社区评分参与过滤，语义与卡片展示一致。

**3b.（可选，决策点 D1）随机抽卡离线 minRating**
- 后端 [random.go:175](backend/internal/handlers/random.go:175) 的 `rating >= minRating` 只查社区评分列。如需个人评分参与需 join comic_ratings。默认**本轮不动**。

---

## 四、Opt1 自定义排序

### 1a. 清单（阅读清单）自定义排序 —— 纯前端

- 现状：ReadingList.Items 为 JSON 数组，**顺序即队列顺序**，后端整体覆盖保存（[library.go SaveReadingList](backend/internal/handlers/library.go:480)），前端 [ReadingListView.vue](src/views/ReadingListView.vue) 无排序 UI。
- 改动：
  - `readingStore` 新增 `moveInReadingList(comic, dir)`：在对应来源队列中交换相邻位置 → 复用现有防抖 PUT 持久化。
  - `ReadingListView` 每行新增 ↑/↓ 按钮（hover 显示、移动端常显，与现有 ✕ 一致）。
- 无需后端改动。

### 1b. 书架内项目自定义排序 —— 前端 + 一个后端接口

- 现状：Bookshelf.ComicIDs 为 JSON 数组（后端保存顺序），但展示端 `shelfComics` 按 offlineComics store 顺序过滤，**不遵循 comicIds 顺序**；无排序 UI。
- 改动：
  - **后端** `library.go` 新增 `PUT /bookshelves/:id/order`（body: `{ comicIds: [...] }`）：校验书架归属后整体覆盖 ComicIDs（复用现有 parse/join），更新 Count。
  - **前端** `bookshelfStore` 新增 `reorderShelfComics(shelfId, comicIds)`（本地更新 + 调接口）。
  - **前端** `OfflineBookshelf`：
    - `shelfComics` 按 comicIds 顺序输出（见 1b 修复项）；
    - 新增「排序」入口 → 进入竖排排序视图（复用阅读清单 mini-card 样式，每行封面+标题+↑/↓），「完成」保存。分页网格内逐位移动体验差，故排序用独立竖排视图（**决策点 D2**）。

### 1c. 书架顺序自定义排序 —— 后端 + 前端

- 现状：`GetBookshelves` 按 `name asc` 排序（[library.go:106](backend/internal/handlers/library.go:106)），Bookshelf 模型无顺序字段。
- 改动：
  - **后端** `models.Bookshelf` 增加 `SortOrder int` 列（AutoMigrate 自动加列；旧数据为 0，按 name 排序行为不变，重排一次即建立新序）。
  - `GetBookshelves` 改 `Order("sort_order asc, name asc")`；`CreateBookshelf` 赋 `SortOrder = 现有最大 + 1`。
  - 新增 `POST /bookshelves/reorder`（body: `{ ids: [...] }`）：按数组下标写 sort_order。
  - **前端** `bookshelfStore` 新增 `reorderBookshelves(ids)`；`OfflineSidebar` 每个书架 hover 显示 ↑/↓ 按钮 → 本地重排 + 调接口。

---

## 五、Opt2 单本移出（只从清单移出）

### 现状

- ReadingListView 已有每行 ✕ 移出（[ReadingListView.vue:121](src/views/ReadingListView.vue:121)）；详情页「📑 加入清单 / ✓ 已在清单」是 toggle 语义（[OfflineDetail.vue:330](src/views/offline/OfflineDetail.vue:330) 等）。

### 改动

- `readingStore` 新增显式 `removeFromReadingList(comic)`（幂等、非 toggle），供各入口复用。
- 详情页（在线/离线）按钮逻辑拆分为明确的「加入清单」/「从清单移出」两个动作（文案与 toast 明确化），**只影响清单本身**，不触碰本地库/书架/历史。
- （**决策点 D3**）阅读器连贯阅读：读完一本后的「继续阅读下一本」对话框可增加「移出清单」选项（当前本从队列剔除后再切下一本），实现「读一本剔一本」。

---

## 六、Opt3 书架修改（删除、改名）

### 现状

- **删除**：侧栏已有 ✕ + confirm（[OfflineSidebar.vue:31](src/components/OfflineSidebar.vue:31) → [bookshelfStore.removeBookshelf:124](src/stores/bookshelfStore.ts:124) → 后端 DeleteBookshelf），已完备。
- **改名**：后端 `PUT /bookshelves/:id` 已支持 name（[library.go UpdateBookshelf:158](backend/internal/handlers/library.go:158)），**前端无入口**。

### 改动

- `bookshelfStore` 新增 `renameBookshelf(id, name)`（调用后端 PUT + 本地更新）。
- `OfflineSidebar` 每个书架 hover 增加「✎」改名按钮（与现有 ✕ 并排），prompt 输入新名称。
- （**决策点 D4**）书架页 header 同时提供「改名」「删除」入口（当前页书架），与侧栏一致。

---

## 七、决策点汇总

| 编号 | 决策项 | 用户选择 | 备选 |
|------|--------|------|------|
| D1 | 随机抽卡离线星级是否按个人评分过滤 | ✅ 本轮不动（只修书库） | 后端 join ComicRating 取 MAX |
| D2 | 书架内项目排序交互形式 | ✅ 独立竖排排序视图（↑/↓，复用清单样式） | 网格卡片 hover ↑/↓ 本页内交换 |
| D3 | 阅读器连贯阅读是否提供「移出清单」 | ✅ 不做（保持现状） | 对话框增加移出选项 |
| D4 | 书架改名/删除入口位置 | ✅ 侧栏 hover 按钮 + 书架页 header 按钮 | 仅侧栏 |

---

## 八、验证

1. `cd backend && go test ./...`
2. `npm run type-check`
3. 本地起后端 + `npm run dev`，脚本/手工验证：
   - Bug1：新标签直接打开 /offline/bookshelf?id=xxx → 能显示书籍；详情页 → 侧栏点书架 → 能显示
   - Bug2：详情页打分 → 回列表/书架/排行榜卡片显示 ⭐ N
   - Bug3：离线筛选选 3 ⭐ → 仅显示个人评分 >=3 的作品
   - Opt1：清单 / 书架内项目 / 书架顺序重排 → 刷新/重新登录后顺序保持
   - Opt2：详情页与清单页单本移出，本地库与书架不受影响
   - Opt3：书架改名、删除（含删除当前所在书架跳回首页）
4. `npm run build` 后同步产物至 `backend/webui/dist`（项目惯例），git 提交（按模块拆分）