# Round3 六大功能迭代计划

> 目标：离线历史续看（按账号）、账号权限收紧、离线排行榜单标题、离线首页默认排序、tag 联想补全、负向排除（tag + 关键词，离线过滤 + 在线本地丢弃）。

## 一、已确认的决策（与用户对齐）

| # | 决策点 | 结论 |
|---|--------|------|
| 1 | 离线历史续看交互 | 仅把进度按账号写入后端 `/history` 并在阅读器进入时读取，页面入口保持现状 |
| 2 | 账号权限 | `/offline/updates` 与 `/offline/maintain` 收紧为仅管理员，前端隐藏非管理员入口 |
| 3 | 排行榜标题 | 单一标题：优先 `titleJpn`（日文），次之主标题，不渲染副标题 |
| 4 | 离线首页默认排序 | 默认 `publishedAt`（发布时间）降序 |
| 5 | 联想补全范围 | 全局筛选（SearchBar/FilterDrawer）+ 抽卡过滤器均支持 tag 联想 |
| 6 | 负向排除 | 离线+在线都生效；支持负向 tag（精确匹配 `namespace:key`）+ 负向关键词（子串匹配标题/tag/上传者）；在线为本地丢弃；抽卡过滤器同步支持 |

## 二、负向排除技术评估（对应"担忧点"）

1. **可行性**：E-Hentai 的 `f_search` **不支持** `-tag` / `-keyword` 排除语法（区别于 Danbooru）。因此在线端"服务端真排除"不可行，只能降级为 **"抓取后本地丢弃"**。
2. **反爬虫风险评估（结论：低风险）**：
   - 列表页：本地丢弃**不改变请求模式与频率**（每页照常请求一次，仅丢弃命中条目，每页显示条数变少，游标 `next` 继续）。不触发额外请求。
   - 随机抽卡：若在线采样池丢弃后数量不足，可"继续随机采样补位"，受现有 `maxSamplingPages = 12` 上限约束（约 300 条候选池），请求量有硬顶。可选增强，首版可先"前端丢弃 + 数量不足提示"。
   - 结论：**无需引入新的请求行为，反爬风险可控**，可以放心实施。
3. **tag vs 关键词**：两者都可以支持。
   - 负向 tag：对 `tagRaws`（已归一为 `namespace:key` 小写）做**精确匹配**（含前缀归一，如 `female:yuri` 精确排除；用户输入 `- female:yuri` 即排除该 tag）。
   - 负向关键词：对 `标题（title/titleJpn）`、`tags（含翻译名）`、`uploader` 做**子串匹配**排除。
   - 语义：`-` 前缀统一表示"排除"。UI 上正向关键词与负向排除以不同样式 chips 区分。

## 三、任务分解与改动落点

### 任务 1：离线历史续看（按账号隔离进度）

现状：
- 后端 `/history` 已按 `user_id` 隔离，`HistoryRecord` 已有 `last_page_index`/`total_page_count`/`last_read_at`（见 `AddHistory`）。
- 前端 [`historyStore.ts`](../../src/stores/historyStore.ts) 的 `syncHistory()` 硬编码 `lastPageIndex: 0`，未回传进度。
- 阅读器进度存在 localStorage（`saku_comic_progress`，`key = source:id`），**未按账号隔离、未写回后端**（见 [`ComicReader.vue`](../../src/views/ComicReader.vue:321)）。
- 离线历史已由 `ItemCard` 点击触发 `addHistory()` 写入基础信息，但无进度。

改动：
1. **后端** [`library.go`](../../backend/internal/handlers/library.go) `GetHistory`：新增可选 `comicId` 查询参数，按 `user_id + source + comic_id` 返回单条记录（用于阅读器精确读取进度）。
2. **前端** [`historyStore.ts`](../../src/stores/historyStore.ts)：
   - 扩展 `syncHistory(source, comic, opts)` 支持 `lastPageIndex`/`totalPageCount` 写入。
   - 新增 `getHistoryProgress(source, comicId)`：调 `/history?source=..&comicId=..` 返回 `lastPageIndex`（无则 `null`）。
3. **前端** [`ComicReader.vue`](../../src/views/ComicReader.vue)：
   - 进入时：先以 localStorage 立即定位，随后异步用 `getHistoryProgress` 拉取后端进度校准（后端有记录则覆盖跳转）。
   - 翻页时：`debounce`（如 1s）调用 `syncHistory(source, comic, { lastPageIndex, totalPageCount })`，仅当 id/title 可用时写入。
   - localStorage 进度 key 加 `userId` 前缀（从 `useUserStore` 取），实现按账号隔离的降级缓存（未登录/后端不可用时回退）。
   - 注意：`getSavedPage` 的读取 key 同步改为带 userId 前缀。

验收：账号 A 读到第 20 页退出，账号 B 登录后进同一本回到第 1 页；A 再进回到第 20 页。

### 任务 2：账号权限收紧（离线更新/维护仅管理员）

现状：见 [`router.go`](../../backend/internal/router/router.go:192)，`/offline/updates/*` 与 `/offline/maintain/*` 位于受保护但**非 admin** 分组；入口在 [`OfflineSidebar.vue`](../../src/components/OfflineSidebar.vue:46)。

改动：
1. **后端** [`router.go`](../../backend/internal/router/router.go)：将以下路由移入 `admin` 分组：
   - `POST /offline/updates/check`、`GET /offline/updates/check/progress`、`GET /offline/updates/check/result`、`GET /offline/updates`、`POST /offline/updates/download`
   - `GET /offline/maintain`、`GET /offline/maintain/progress`、`GET /offline/maintain/result`、`POST /offline/maintain/remove`
2. **前端** [`OfflineSidebar.vue`](../../src/components/OfflineSidebar.vue)：`更新`/`维护` 两个入口用 `useUserStore().isAdmin` 控制显隐（非管理员不渲染）。
3. **前端** [`router/index.ts`](../../src/router/index.ts)：为 `/offline/update`、`/offline/maintain` 增加 `meta.requiresAdmin`，全局 `beforeEach` 校验非管理员跳转到 `/offline/home`（前端兜底，防直接输 URL）。
4. **核对**：输出一份权限清单（见"四、权限清单"），确认下载任务仍保持 `admin 或 allowDownload`（[`download.go`](../../backend/internal/handlers/download.go) 内校验），不做额外收紧。

验收：member 账号看不到更新/维护入口，直接访问 URL 被跳转/403；admin 正常。

### 任务 3：离线排行榜单一标题

现状：`ItemCard` 已有 `displayTitle = titleJpn || title`；副标题 `card-subtitle` 仅 `size === 'large'`（领奖台）隐藏，**普通卡片仍渲染副标题**（见 [`ItemCard.vue`](../../src/components/ItemCard.vue:490)）；[`OfflineToplist.vue`](../../src/views/offline/OfflineToplist.vue:70) 的 rest 列表用普通 size，会显示副标题。

改动：
1. **前端** [`ItemCard.vue`](../../src/components/ItemCard.vue)：新增 `hideSubtitle?: boolean` prop；为真时隐藏 `compact-subtitle` 与 `card-subtitle`（大卡片本来已隐藏）。
2. **前端** [`OfflineToplist.vue`](../../src/views/offline/OfflineToplist.vue)：领奖台与 rest 列表的 `ItemCard` 均传 `:hide-subtitle="true"`，实现"排行榜仅单一标题（日文优先，次主标题）"。
3. 核对 title/titleJpn 实际存储：确认扫描器将日文原名存入 `titleJpn`、英文主标题存入 `title`；若 `title` 存的是中文翻译则需在排序/展示层明确优先级（计划默认 title=英文，若不符由执行阶段按数据核对调整）。

验收：排行榜所有卡片仅一行标题（日文优先），无中/英副标题。

### 任务 4：离线首页默认排序

改动：**前端** [`OfflineHome.vue`](../../src/views/offline/OfflineHome.vue:175)：`sortBy` 默认值 `'addedAt'` → `'publishedAt'`，`sortDesc` 保持 `true`。切换逻辑不变，仅改默认值。

验收：进入离线首页默认按发布时间降序。

### 任务 5：tag 联想补全（筛选 + 抽卡）

现状：后端 `/tags/suggest` 已存在（[`Suggest()`](../../backend/internal/services/tag_engine.go:461)）；[`SearchBar.vue`](../../src/components/SearchBar.vue:56) 已接入。`FilterDrawer` 关键词输入（[`FilterDrawer.vue`](../../src/components/FilterDrawer.vue:204)）与抽卡关键词输入（[`RandomView.vue`](../../src/views/RandomView.vue:67)）无联想。

改动：
1. **公共**：新增 `src/composables/useTagSuggest.ts`：
   - 复用 `/tags/suggest`，`debounce 150ms`，返回 TagItem 列表（参照 SearchBar 的安全清洗逻辑）。
   - 支持 `- ` 前缀：输入以 `-` 开头时，将查询串去前缀后请求联想，标记为"排除候选"。
2. **前端** [`FilterDrawer.vue`](../../src/components/FilterDrawer.vue)：关键词输入框下方加入联想下拉；点击正向候选压入 `keywords`；点击 `- ` 候选压入负向排除队列（见任务 6 数据结构）。
3. **前端** [`RandomView.vue`](../../src/views/RandomView.vue)：抽卡过滤器关键词输入接入同一联想。

验收：筛选与抽卡输入"yuri"出现联想列表，可点选；输入"- fema"出现负向联想。

### 任务 6：负向排除（tag + 关键词）

改动：
1. **类型** [`src/types/comic.ts`](../../src/types/comic.ts)：`SearchConfig`、`FilterParams`、`RandomComicParams` 各增加 `excludeTags?: string[]`、`excludeKeywords?: string[]`。
2. **公共**：新增 `src/utils/tagFilter.ts`：
   - `matchExcludes(comic, { excludeTags, excludeKeywords })`：
     - 负向 tag：对 `comic.tagRaws`（离线）或 `comic.tags` 归一后的 `namespace:key`（在线）做**精确匹配**。
     - 负向关键词：对 `title`、`titleJpn`、`tags`（翻译名）、`uploader` 做**子串匹配**。
     - 命中任一排除项即返回 `false`（剔除）。
   - 提供 `parseKeywordQueue(items)`：把关键词队列按 `- ` 前缀拆分为 `{ positive: [], negativeTags: [], negativeKeywords: [] }`（`- female:yuri` → negativeTags；`- xxx` → negativeKeywords）。
3. **前端** [`OfflineHome.vue`](../../src/views/offline/OfflineHome.vue)：`filteredComics` 管道增加"负向关卡"（在现有各关卡后）。
4. **前端** [`OnlineHome.vue`](../../src/views/online/OnlineHome.vue)：新增 `filteredComics = computed(() => matchExcludes(onlineStore.comics, ...))`，`GridContainer :items` 改为引用该 computed（本地丢弃，游标继续）。
5. **前端** [`RandomView.vue`](../../src/views/RandomView.vue)：`buildParams()` 下发排除项；对 `drawnComics` 再做一次前端 `matchExcludes` 兜底过滤，数量不足时 `toast.warning` 提示。
6. **UI**：负向 chips 用红/删除线样式与正向区分；`- ` 前缀候选在联想下拉中标注"排除"。
7. **可选增强（后端）**：[`random.go`](../../backend/internal/handlers/random.go) / [`eh_random.go`](../../backend/internal/services/eh_random.go)：接受 `excludeTags`/`excludeKeywords`，在线采样池构建时丢弃命中项并继续采样补位（受 `maxSamplingPages` 约束）。
8. **可选扩展**：在线 `Sub`/`Hot`/`Favorites`/`Top` 数据源独立（`subStore.comics`/`hotComics`/`favComicList`/toplist），如需负向过滤可复用 `utils/tagFilter` 逐一接入；默认首版只做离线首页 + 在线首页 + 抽卡。

验收：离线/在线列表与抽卡中，`- female:yuri` 排除所有含该 tag 的作品；`- 3d` 排除标题/标签/上传者含"3d"的作品；抽卡结果不足时给出提示。

## 四、权限清单（任务 2 核对产物）

| 分组 | 代表接口 | 说明 |
|------|----------|------|
| 公开（无需登录） | `/auth/login`、封面/页图代理、离线封面/页图 | 浏览器 `<img>` 无法携带 Authorization，维持公开 |
| 登录即可 | `/comics/online`、`/comics/random`、`/history`、`/ratings`、`/reading-list`、`/bookshelves`、`/tags/suggest`、`/online/watched`、`/eh/*` 等 | 全部登录用户 |
| 登录+下载许可 | `/downloads/*` | handler 内校验 `admin 或 allowDownload` |
| **仅管理员（新增收紧）** | `/offline/updates/*`、`/offline/maintain/*` | **本次从登录组移入 admin 组** |
| 仅管理员（原有） | `/users/*`、`/server/setting`、`/network/proxy`、`/tags/sync/*`、`/tags/progress`、`/offline/tags/*`、`/comics/:id/tags`、`/admin/history` | 维持现状 |

## 五、风险与注意事项

1. **在线列表本地丢弃的空位**：每页 25 条被剔除后变稀疏，`loadMore` 游标继续。首版接受该行为；如需"翻页补满"会显著增加复杂度，列入后续增强。
2. **抽卡在线数量不足**：前端丢弃后 `drawnComics` 可能 < count，做 warning 提示；后端采样补位为可选增强。
3. **进度写回频率**：阅读器翻页写后端需 debounce，避免高频请求；离线时静默失败（不影响阅读）。
4. **localStorage 兼容**：进度 key 加 userId 前缀后，旧 key（无前缀）数据作为一次性迁移或直接弃用（首版直接弃用即可，避免复杂迁移）。
5. **排行榜标题**：需在实施时核对 `title`/`titleJpn` 实际内容，确认"次英文标题"的存储语义；若不符需调整扫描/展示层。

## 六、建议实施顺序

1. 任务 4（最小改动，先行验证）
2. 任务 2（权限收紧，后端 + 前端）
3. 任务 3（排行榜单标题）
4. 任务 1（历史续看，前后端）
5. 任务 6 类型 + 工具函数（先落地公共层）
6. 任务 5（联想，依赖任务 6 的 `- ` 解析）
7. 任务 6 各页面接入 + 抽卡
8. 整体回归验证
