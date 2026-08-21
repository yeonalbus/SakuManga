# Round11 在线卡片/历史修复与下载/预览优化计划

## 背景与问题

| 编号 | 类型 | 内容 |
|------|------|------|
| Bug1 | 显示 | 在线卡片（ItemCard）评分丢失，有概率显示 `⭐ —` |
| Bug2 | 显示 | 在线卡片页数显示错误，如 `39P` 被显示为 `959539` |
| Bug3 | 显示 | 本地/在线历史记录封面图丢失、标题显示为乱码/一串字符 |
| Opt1 | 优化 | 快速导入：从指定书架一键导入清单（仅离线；在线取消） |
| Opt2 | 优化 | 下载完成校验：队列完成后按队列更新状态；取消自动全库扫描 |
| Opt3 | 优化 | 预览优化：本地画廊预览 / 修改标题 / 本地备注 / 跳转在线画廊 |

---

## 一、Bug1 在线卡片评分丢失（根因已用真实样本确认）

### 现状与根因

- 在线列表评分来自 E 站列表卡片 `.ir` 元素的 `background-position` 雪碧图，由 `parseRatingFromStyle`（[eh_parser.go:83](backend/internal/services/eh_parser.go:83)）解析，**只按 X 偏移判断整星**：
  - `0px 0px` / `0px -1` → 5.0；`-16px` → 4.0；`-32px` → 3.0；`-48px` → 2.0；`-64px` → 1.0；其余 → **0.0**
- **真实样本（[eh_popular_live.html](testdata_eh/eh_popular_live.html)，2026-08-21 抓取）显示 E 站现在大量使用半星行**：
  - `background-position:0px -21px` = **4.5 星** → 现函数不识别 → 返回 0 → 卡片显示 `⭐ —`（Bug 直接证据，Doujinshi 条目普遍）
  - `-16px -21px` = **3.5 星** → 被误判为 4.0（高估 0.5）；`-32px -21px` = 2.5 → 被误判 3.0
- 排行榜已有一个**完整的 X/Y 半星解析器** `parseToplistRatingFromStyle`（[eh_parser.go:117](backend/internal/services/eh_parser.go:117)），但列表解析未复用。
- 涉及入口：首页/搜索（[eh_gallery.go:165](backend/internal/services/eh_gallery.go:165)）、订阅（[eh_sub.go:130](backend/internal/services/eh_sub.go:130)）、收藏（[favorites.go:108](backend/internal/handlers/favorites.go:108)）、热门（[eh_gallery.go:321](backend/internal/services/eh_gallery.go:321)）。

### 修复

**1a. [`eh_parser.go`](backend/internal/services/eh_parser.go:83)** 将列表评分解析统一为 X/Y 半星算法（复用/合并 `parseToplistRatingFromStyle` 逻辑）：
```
func parseRatingFromStyle(style string) float64  → 按 background-position X/Y 偏移计算
   Y=-1px 满星行：base = 5.0 + X/16.0（0px→5.0, -16px→4.0, -32px→3.0, -48px→2.0, -64px→1.0）
   Y=-21px 半星行：结果再 -0.5（0px→4.5, -16px→3.5, -32px→2.5, -48px→1.5, -64px→0.5）
   解析不到（无 .ir / 非标准样式）→ 保持 0（不误报）
```
- 兼容旧式 `0px 0px`（无 Y）的写法；四舍五入到 0.5 精度。
- 新增单测：用真实样本 `eh_popular_live.html` 断言各偏移的期望星级。

---

## 二、Bug2 在线卡片页数显示错误（959539）

### 现状与根因

- 页数解析用 `pageCountRegex = (\d+)\s*(?:pages|P|页)`（[eh_parser.go:16](backend/internal/services/eh_parser.go:16)），在**整行文本**上 `FindStringSubmatch` 取第一个匹配。
- **根因：`P` 分支无单词边界** —— `(\d+)\s*P` 会误匹配任何「数字 + 大写 P 开头单词」：
  - 真实样本（[eh_search.html](testdata_eh/eh_search.html)）标题如 `(223 Piece 223枚)` → `223 Piece` 被匹配为 `223 P`；
  - 当行内页数文本缺失/格式变化（如 E 站某些布局页数为 `39p` 小写、或标题先于页数出现）时，第一个匹配落到标题里的大数字 → 出现 `959539` 这类异常值。
- 另外：**热门（FetchPopularList）与收藏（favorites）列表根本不解析 pageCount**（DTO 为 0，卡片不显示页数），属功能缺失。

### 修复

**2a. [`eh_parser.go:16`](backend/internal/services/eh_parser.go:16)** 正则收紧：`(\d+)\s*(?:pages?|页|\bP\b)`
- `pages?` 兼容复数/单数（真实热门页出现过 `495 page`）；`\bP\b` 仅匹配独立大写 P（如 `39P`），排除 `Piece`/`Pages` 等单词。
- 匹配结果防御：`pageCount` 上限保护（如 > 100000 视为异常忽略，防脏数据）。

**2b.（决策点 D1）给热门/收藏列表补全 pageCount 与 tags 解析**
- 现状热门/收藏卡片无页数/标签；修复正则后可顺带复用首页同样的解析块补齐。

---

## 三、Bug3 历史记录封面丢失 / 标题乱码（根因已确认）

### 现状与根因

- 阅读器进度写回：`ComicReader.currentComicMeta`（[ComicReader.vue:345](src/views/ComicReader.vue:345)）——
  - 离线：从 `offlineComics` 查真实条目；**查不到时 fallback `title: comicId`（一串字符）、`coverUrl: ""`**；
  - **在线：永远走 fallback**（`title = gid 数字串`、`coverUrl = ""`）。
- 阅读器每 1s debounce 调 `syncHistory(source, currentComicMeta, {lastPageIndex, totalPageCount})`（[ComicReader.vue:370](src/views/ComicReader.vue:370)），把 `comicTitle = gid`、`coverUrl = ""` 提交后端。
- 后端 `AddHistory`（[library.go:334](backend/internal/handlers/library.go:334)）**无条件覆盖** `ComicTitle`/`CoverURL` → 阅读后正常历史被污染：标题变 gid 乱码、封面丢失。
- 「有概率」：只有阅读（含从清单/历史直接进入）才会覆盖；先点卡片（写入正常标题）再阅读 → 必现。

### 修复（前后端双保险）

**3a. 后端 [`library.go AddHistory`](backend/internal/handlers/library.go:334)** 空值不覆盖：
```
if req.ComicTitle != "" { rec.ComicTitle = req.ComicTitle }
if req.CoverURL != "" { rec.CoverURL = req.CoverURL }
（与既有 lastPageIndex 保护一致，防一切调用方误清）
```

**3b. 前端 [`ComicReader.vue:345`](src/views/ComicReader.vue:345) `currentComicMeta` fallback 补全**：
- 离线查不到 → 依次从 `offlineReadingList`、`offlineHistoryList` 取 title/coverUrl；
- 在线 → 从 `onlineReadingList` / `onlineHistoryList` 取（路由只传了 id/source/token/page）；
- 仍缺失 → title 用空字符串（不再用 comicId），由后端 3a 保护保留旧值。

---

## 四、Opt1 快速导入（书架 → 清单，仅离线）

### 现状

- 阅读清单页「➕ 快捷导入」是占位按钮（[ReadingListView.vue:98](src/views/ReadingListView.vue:98) `handleQuickImport` 只弹 toast）。
- 书架数据在 `bookshelfStore.bookshelves`（含 comicIds 顺序）；离线漫画在 `offlineComics`。
- `readingStore.addToReadingList` 已幂等（重复忽略）。

### 设计

**4a. [`ReadingListView.vue`](src/views/ReadingListView.vue)**：
- 快捷导入按钮**仅在「本地清单」tab 显示**（在线 tab 隐藏，满足"在线取消快速导入"）。
- 点击 → 弹出书架选择面板（**决策点 D2**：弹层列表 / 内联展开）。每个书架显示名称+数量，点击该行即导入。
- **增量式**：遍历书架 `comicIds` 顺序 → 在 `offlineComics` 中映射 → 逐个 `addToReadingList`（已在清单中的跳过）→ **追加到清单末尾**（导入 B 时接在 A 后）。
- **初始顺序 = 书架内 comicIds 顺序**（Round10 已保证 comicIds 顺序即书架展示顺序）。
- 导入完成 toast：`已从书架「X」导入 N 本（跳过 M 本已在清单）`。
- 空书架 / 无匹配本地记录时给出提示。

---

## 五、Opt2 下载完成校验

### 现状

- 后端：下载队列空闲 >1 分钟 → `onQueueIdleTimeout`（[download.go:231](backend/internal/services/download.go:231)）自动调用 `AutoRunMaintainDedup`（[maintain_auto.go:119](backend/internal/services/maintain_auto.go:119)）→ **联网全库维护查重**。
- 前端：`downloadTasksStore` 已检测任务完成并 `fetchOfflineComics()` 刷新本地缓存（[downloadTasksStore.ts:27](src/stores/downloadTasksStore.ts:27)）。
- 手动入口：离线更新页「🔍 开始检测」（[OfflineUpdate.vue:249](src/views/offline/OfflineUpdate.vue:249)）已有。

### 设计

**5a. 后端 [`download.go:246`](backend/internal/services/download.go:246)**：**移除**队列空闲后的自动 `AutoRunMaintainDedup` 调用**（决策点 D3：连 idle 定时机制一并移除，或仅移除查重调用保留日志）**。
- 下载完成入库已由 `ScanAndSaveDirectory`（[download_gallery.go:222](backend/internal/services/download_gallery.go:222)）完成，队列完成后无需再全库联网核对。

**5b. 前端 `downloadTasksStore`**：完成任务检测已触发 `fetchOfflineComics()`，即"根据下载队列更新数据库（本地库缓存）状态"，保持现状；可选增强：下载页（DownloadsView）任务全部完成后 toast 提示 + 刷新列表（**决策点 D4**）。

**5c. 全库维护查重改为纯手动**：保留离线更新页「开始检测」入口（现状即是手动）；无需新入口。

---

## 六、Opt3 预览优化（本地画廊）

### 现状

- 离线详情页（[OfflineDetail.vue](src/views/offline/OfflineDetail.vue)）只有单张封面，无页图预览；无标题编辑、无备注、无跳转在线入口。
- 后端已有页图接口 `/comics/:id/page/:index`；`OfflineComic` 模型已含 `GID/Token`（JSON 已随详情响应透传，前端 DTO 未声明）。

### 设计

**6a. 画廊预览**（**决策点 D5：预览区形式与页数**）
- 推荐：详情页右侧新增「预览」区（或 tab），加载前 N 张页图缩略图（默认 12 张，决策点确认），点击某张 → 打开阅读器并定位到该页（`/reader?id=..&source=offline&page=N`）。

**6b. 修改标题（支持恢复原标题）**
- 后端 `OfflineComic` 新增 `OriginalTitle` 列：**首次扫描入库时**记录当时标题（后续扫描更新标题不覆盖此列）；手动修改 title 时此列保持不变。
- 后端新增 `PUT /comics/:id`（body: `{ title?: string }`）：
  - `title` 非空 → 更新 title；
  - `title` 为空字符串 → **恢复** `title = OriginalTitle`（用户删掉自定义标题即还原，便于反复尝试/后悔）；
  - `OriginalTitle` 为空（旧数据）时回退保留现有 title。
- 详情页标题旁加「✎」编辑按钮（prompt 输入；清空输入 = 恢复原标题），保存后刷新详情与离线列表。

**6c. 本地备注**
- 后端 `OfflineComic` 新增 `Remark string` 列（AutoMigrate 自动加列）；列表/详情响应透传；
- 新增 `PUT /comics/:id/remark`（或并入 6b 的 PUT）；
- 详情页新增「备注」卡片（多行文本 + 保存）。

**6d. 跳转在线画廊**
- 详情页「🌐 跳转在线画廊」按钮：读取 `comic.gid`/`comic.token` → 新标签打开 `/online/detail?id=<gid>&token=<token>`；
- 无 gid（本地导入无 E 站关联）时按钮置灰并提示。

---

## 七、决策点汇总（需用户确认）

| 编号 | 决策项 | 推荐 | 备选 |
|------|--------|------|------|
| D1 | 热门/收藏列表是否补全 pageCount/tags | ✅ 补全（复用首页解析块） | 仅修正则不动列表 |
| D2 | 快速导入的书架选择交互 | ✅ 弹层书架列表（名称+数量，点击行即导入） | 内联展开下拉 |
| D3 | 后端空闲自动维护处理 | ✅ 整个移除 idle 定时机制 | 仅移除查重调用，保留日志 |
| D4 | 下载页全部完成后提示/刷新 | ✅ 下载页 toast + 刷新列表 | 保持现状（轮询已覆盖） |
| D5 | 预览区形式与缩略图数量 | ✅ 详情页「预览」tab，前 20 张 | 详情页右侧预览区，前 12 张 |
| D6 | 标题修改范围 | ✅ 仅修改 title，且支持恢复原标题（删空即还原；新增 OriginalTitle 列记录首次入库标题） | 同时允许修改 titleJpn |

---

## 八、验证

1. `cd backend && go test ./...`（新增：评分半星解析单测用真实样本、正则误匹配回归单测）
2. `npm run type-check`
3. 本地起后端 + 浏览器自动化（沿用 `scripts/verify-round10.mjs` 模式新增 round11 脚本）：
   - Bug1：真实热门页（带测试 cookie）评分正确显示 4.5/3.5 等半星
   - Bug2：含 `(223 Piece)` 标题的搜索结果页数正确为实际页数，无 959539 类异常
   - Bug3：在线阅读后历史标题/封面保持正常；空 title/coverUrl 不再覆盖
   - Opt1：书架 A 导入 → 书架 B 增量接续；顺序=书架 comicIds 顺序；在线 tab 无导入按钮
   - Opt2：下载完成后无自动全库扫描日志；手动「开始检测」仍可用
   - Opt3：预览缩略图/点图跳页；改标题；备注保存；跳转在线画廊
4. `npm run build` 同步 `backend/webui/dist`；git 提交（后端 / 前端 / 产物）