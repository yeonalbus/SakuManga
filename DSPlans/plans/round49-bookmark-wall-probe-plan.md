# Round49 书签栏位改造实施计划

> 依据：
> - `YeonPlans/关于书签栏位的优化方案.md`（珱垣定稿）
> - `DSPlans/YeonPlans-dsedit/关于书签栏位的优化方案-dsedit.md`（AI 落档版，含决策标注）
>
> 状态：M1-M3 已完成（后端），M4 进行中

## 1. 目标

把搜刮书签从「侧栏平铺列表」升级为「独立书签页面 + 侧栏置顶快捷入口」，并新增书签级更新探测：

1. 书签独立页 `/online/bookmarks`（名片墙：封面 / 标题 / 备注 / 更新标记 / 书签标记 / 已囤货，置顶 / 改备注 / 开关等配置折叠进二级）
2. 侧栏书签组：无折叠，标题点击进书签页；常驻「更新探测」按钮 + 最多 5 条置顶书签（点击沿用「跳转 + 定位 + 脉冲」链路）
3. 书签级字段：`role`（newest/oldest/interest 单选）、`note`、`pinned`、`updateWatch`、`hasUpdate`、`bulkDone` 等
4. 每日更新探测：服务端调度器按 `RefreshHour` 触发，对 `updateWatch=true` 的书签各取一页列表，判据 `首条 postedAt > anchor.postedAt`
5. 锚点转移更新（创建时同检索条件提示替换）与锚点缺失优化（迁移后自动续定位）
6. 剪枝：失效检测 / 清理失效（前后端一并退场）

## 2. 现状基线

### 后端

| 文件 | 现状 |
|---|---|
| `internal/models/scrape_bookmark.go` | 9 字段：ID/UserID/Name/Type/Keyword/Config/Anchor/SortKey/CreatedAt |
| `internal/services/scrape_bookmark.go` | List / Create / Rename / UpdateAnchor / Delete / MovePosition / Reorder |
| `internal/services/scrape_bookmark_check.go` | 失效检测（scanBookmarkVisibility + CheckScrapeBookmarks），**M7 剪枝** |
| `internal/handlers/scrape_bookmark.go` | List/Create/Update/Check/MovePosition/Reorder/Delete |
| `internal/router/router.go` | `/api/v1/scrape-bookmarks` 系列路由；`StartTagMaintainScheduler`、`StartUpdateScanScheduler` 启动点 |
| `internal/services/update_scheduler.go` | 调度器范式：`time.Sleep(time.Until(next))`，无 cron 依赖 |
| `internal/services/tag_maintain.go` + `models.TagMaintainSetting` | `RefreshHour` 每日刷新时刻来源 |
| `internal/services/scrape_bookmark_check.go:283` | E 站列表请求范式：`ehRateLimiter.Wait()` + `eh.FetchGalleryList(account, params, setting)` |

### 前端

| 文件 | 现状 |
|---|---|
| `src/types/comic.ts` | `ScrapeBookmark` 契约（含 anchor.invalid / migratedFrom / sortKey） |
| `src/stores/scrapeBookmarksStore.ts` | 后端化 store：加载/迁移/增删改/排序/失效标记/会话级 ⚠️ |
| `src/components/OnlineSidebar.vue` | 侧栏书签组：折叠 + 前 6 条平铺 + 「其余 N 条」展开 + 拖动排序 + 🔍/🧹 |
| `src/components/BookmarkCreateModal.vue` | 创建弹窗（仅写 name） |
| `src/components/SidebarGroupHeader.vue` | 折叠组标题行（书架/书签共用） |
| `src/composables/useBookmarkCheck.ts` | 失效检测编排 + 时间锚迁移，**M7 剪枝检测部分、保留迁移** |
| `src/composables/useBookmarkLocate.ts` | 书签定位循环（seek + 动态越过），M6 接入自动续定位 |
| `src/utils/bookmarkTimeAnchor.ts` | 时间锚候选查找（迁移用） |
| `src/views/online/OnlineHome.vue` | `?bm=<id>` → 恢复 config → `beginLocate` → `runLocate`；「存为书签」入口 |
| `src/router/index.ts` | online 路由段，需加 `/online/bookmarks` |

## 3. 分阶段任务

### M1 后端数据模型与迁移

- [ ] `models/scrape_bookmark.go` 新增字段：`Role` / `Note` / `Pinned` / `UpdateWatch` / `HasUpdate` / `UpdateCheckedAt` / `UpdateError` / `BulkDone` / `BulkDoneAt`，外加内部迁移标记 `FieldsMigrated bool`
- [ ] 新增 `services/scrape_bookmark_migrate.go`：`MigrateScrapeBookmarkFields(db)`
  - 仅处理 `fields_migrated = false` 的记录（幂等）
  - `role`：name 含"最旧" → oldest；含"最新" → newest；其余 → interest
  - `bulkDone`：name 含"已下载完" → true
  - `anchor` JSON 中删除 `invalid` 键（保留其余键原样）
  - 完成后置 `fields_migrated = true`
- [ ] 在启动流程调用迁移（`main.go` / `router.go` 初始化段，AutoMigrate 之后）
- [ ] 验收：`go build ./...` 通过；启动后老书签字段正确回填

### M2 后端接口与探测服务

- [ ] DTO/Input 扩展：List/Create/Update 支持新字段；`Update` 支持部分更新 role/note/pinned/updateWatch/bulkDone
- [ ] 新增 `services/scrape_bookmark_probe.go`：
  - `ProbeScrapeBookmarks(db, eh, userID, account, setting, ids []uint)` → 逐条串行
  - 取书签 config → `toSearchParams()` → 请求一页（不带 seek/next）→ 首条 `updatedAt`
  - 判据：`首条 postedAt > anchor.postedAt` → `hasUpdate=true`；否则 false
  - 失败：写 `updateError`，**不动 hasUpdate**
  - 更新 `updateCheckedAt`
- [ ] handler `POST /api/v1/scrape-bookmarks/probe`（body 可带 ids，缺省=全部 updateWatch=true）
- [ ] 删除手工排序接口 `PUT /:id/position` 与 `POST /order`（书签页改为自动排序）
- [ ] 验收：`go test ./...` 通过；`go vet` 无新增告警

### M3 调度器

- [ ] 新增 `services/scrape_bookmark_scheduler.go`：`StartScrapeBookmarkProbeScheduler(db, ehService)`
  - 触发时刻读 `TagMaintainSetting.RefreshHour`，**不读** `EnableDailyRefresh`
  - 遍历所有用户 → 各用户绑定的 `AccountSetting` → 调探测服务（只测 `updateWatch=true`）
  - 凭据缺失跳过并记录日志
- [ ] `router.go` 启动调度器
- [ ] 验收：日志可见下次触发时间；不因定时任务阻塞启动

### M4 前端数据层

- [ ] `types/comic.ts`：`ScrapeBookmark` 增加 role/note/pinned/updateWatch/hasUpdate/updateCheckedAt/updateError/bulkDone/bulkDoneAt
- [ ] `stores/scrapeBookmarksStore.ts`：
  - fromRaw/restore 兜底新字段
  - `addScrapeBookmark` 支持新字段入参
  - 新增 `updateBookmarkFields`（role/note/pinned/updateWatch/bulkDone）
  - 新增 `probeScrapeBookmarks`（调 /probe 后刷新本地）
  - 新增 `pinnedBookmarks`（上限 5）、`sortedBookmarks`（名称序 + 有更新置前，`Intl.Collator('zh-CN',{numeric:true})`）
  - 删除失效检测相关导出（invalidBookmarks / clearInvalidBookmarks / markBookmarkInvalid / invalidReasonText），保留 `failedAnchorGids` 会话级标记
- [ ] 验收：`npm run type-check` 通过

### M5 前端 UI

- [ ] 新增 `src/views/online/OnlineBookmarks.vue`：3 列名片墙、置顶/备注/开关/已囤货的二级操作、检查更新按钮、空状态
- [ ] `router/index.ts` 加 `/online/bookmarks`
- [ ] `OnlineSidebar.vue` 书签组改造：
  - 无折叠；标题点击 → `/online/bookmarks`
  - 「更新探测」按钮 + 最多 5 条置顶书签（点击沿用 `/online/home?bm=` 跳转）
  - 删除 🔍/🧹、收纳区、拖动排序
- [ ] `BookmarkCreateModal.vue`：加 role 单选 / 备注 / 更新开关 / 已囤货配置
- [ ] `OnlineHome.vue`：创建书签时传新字段；创建前查同 signature 书签 → 转移提示
- [ ] 验收：`npm run type-check` + `npm run build` 通过

### M6 锚点缺失优化

- [ ] `useBookmarkLocate.ts` 的 passed/exhausted 分支：不再调用 runCheck，直接 `migrateByTimeAnchor`（迁移函数移到独立模块或保留在 store）
- [ ] 迁移成功 → 先写库成功 → 重新 `beginLocate` + `runLocate`（自动续定位到新锚点）
- [ ] 验收：构造锚点失效场景，迁移后不刷新即可定位

### M7 剪枝

- [ ] 删除 `services/scrape_bookmark_check.go` 及其测试
- [ ] 删除 handler Check + 路由
- [ ] 删除前端 useBookmarkCheck 的检测编排（保留时间锚迁移）
- [ ] 删除 `anchor.invalid` 字段读写（留存迁移清理）
- [ ] 验收：`go test ./...`、`npm run type-check` 通过；无死代码引用

### M8 验证与收尾

- [ ] 后端 `go test ./...`
- [ ] 前端 `npm run type-check` / `npm run build`
- [ ] 冒烟：启动 → 老数据迁移 → 书签页显示 → 置顶 → 探测 → 转移 → 锚点缺失续定位
- [ ] 不更新版本号 / CHANGELOG（本次明确不做）

## 4. 风险与回滚

| 风险 | 处理 |
|---|---|
| 字段迁移写坏老数据 | 迁移前备份 `manga.db`；迁移在事务内执行；`fields_migrated` 标记保证幂等 |
| 探测请求触发 E 站限流 | 串行 + 复用 `ehRateLimiter`；只有用户显式开启开关的书签才探测 |
| 删除失效检测造成引用遗漏 | M7 前先 grep 全量引用；编译期兜底 |
| 书签页 3 列在窄屏拥挤 | 预留响应式降列（`@media`） |
| 旧 `name` 与固定标题冲突 | `name` 保留不删，仅 UI 不再可编辑 |

## 5. 不做清单

- 书架改造（常驻显示 + 点击标题=全部书架）
- 订阅页去留（只修 `/watched` bug，独立项）
- 版本号 / CHANGELOG 更新
- 书签手动排序 / 拖动排序（Round37 退场）
- 失效检测 / 清理失效（剪枝）

## 6. 进度记录

| 时间 | 内容 |
|---|---|
| 2026-10-31 | M1 完成：`models/scrape_bookmark.go` 新增 9 字段 + `FieldsMigrated`；`scrape_bookmark_migrate.go` 一次性回填（role/bulkDone 按旧名提取、清 anchor.invalid、幂等）；`main.go` 启动调用；迁移单测通过 |
| 2026-10-31 | M2 完成：DTO/Input/Patch 扩展；`scrape_bookmark_probe.go` 探测服务（1 请求/书签、失败不清红点）；`POST /scrape-bookmarks/probe` 路由；`UpdateScrapeBookmarkAnchor` 顺带清 hasUpdate；probe 单测通过 |
| 2026-10-31 | M3 完成：`scrape_bookmark_scheduler.go` 每日探测调度器（复用 RefreshHour、不读 EnableDailyRefresh、多用户按凭据串行）；router 启动 |
| 2026-10-31 | M4 完成：前端 `ScrapeBookmark` 契约扩展；store 兜底/创建/字段更新/探测/置顶/排序（名称序 + 有更新置前）；`bookmarkScope` 新增 `transferKey` 转移判据 |
| 2026-10-31 | M5 完成：新增 `OnlineBookmarks.vue` 名片墙 + `/online/bookmarks` 路由；`OnlineSidebar` 重写（无折叠、标题进页面、置顶 5 条 + 探测按钮）；创建弹窗加 role/备注/开关/已囤货；OnlineHome 接入锚点转移 |
| 2026-10-31 | M6 完成：`useBookmarkMigrate` 提取；`useBookmarkLocate` 失败分支改为「迁移 → relocateRequest 自动续定位」；OnlineHome watch 续定位 |
| 2026-10-31 | M7 完成：删除 `useBookmarkCheck.ts`、`composables` 检测编排、store 检测导出、后端 check 服务/测试/路由/handler；工具函数迁至 `scrape_bookmark_search.go` |
| 2026-10-31 | M8 完成：`go test ./...` 通过；`npm run type-check` / `npm run build` 通过。冒烟（启动服务验证迁移与页面）留待实际运行环境验证 |
