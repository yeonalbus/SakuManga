# SakuHentai 诊断报告

> 针对用户提出的 4 处 BUG 与若干需求现状的诊断结论。
> 配套实施计划见 [`round4-nine-features-plan.md`](plans/round4-nine-features-plan.md)。

---

## 1. 幽灵文件：维护结果与更新列表不一致

### 现象

- 漫画 `[たいらー] たんプリまんが (名探偵プリキュア!) [中国翻訳]` 在「更新」页仍显示「检测到更新版本：最新版 gid=4086937，中间版本：4012290 → 4019697 → 4051934」。
- 「维护」页显示「恭喜！本地画库暂无重复或异常项」。
- 平板上「维护」页展示的是**维护前**的旧状态，点击删除提示「文件或记录不存在」。
- 在线更新检测数 2736+47=2783 与本地文件数吻合。

### 根因（两部分）

**A. 陈旧的内存缓存（真正的 BUG）**

- 维护查重结果存放在**进程内存全局变量** `offlineMaintainRes`（见 [`offline_task.go`](backend/internal/services/offline_task.go) 的 `StoreMaintainDedupResult / GetMaintainDedupResult`）。
- `GET /offline/maintain/result` 直接返回该内存对象（见 [`handlers/offline.go`](backend/internal/handlers/offline.go) 的 `GetMaintainResult`）。
- 该缓存**不随删除操作失效、不随会话结束清空、跨设备共享**（前后端各设备连的是同一个后端进程）。
- 桌面端维护并删除后，缓存未失效 → 平板再次进入时 `onMounted` 读到 `status==='success'` 便直接 `loadResult()` 展示旧列表（见 [`OfflineMaintain.vue`](src/views/offline/OfflineMaintain.vue:253)）。
- 点击旧列表里已删除的项 → `DeleteOfflineComic` 里 `db.First` 返回 `ErrRecordNotFound` → 前端收到「文件或记录不存在」（见 [`offline.go`](backend/internal/services/offline.go:641)）。

**结论：您猜测的「只是在缓存里过了一遍」准确命中了 A 部分。**

**B. 更新列表仍显示该漫画属于「符合预期」（非残留）**

- 更新列表是 **DB 查询**（`WHERE needs_update = true`），并非缓存。
- 维护规则 3（父子版本）只有在**新版本已存在于本地**时，才把旧版本标记为「建议删除」（见 [`offline.go`](backend/internal/services/offline.go) 的父-子规则：`removeSet[c.ID]` 需在新版本已在 `gidToComic` 中时才成立）。
- 该漫画的最新版 gid=4086937 **尚未下载到本地**，因此不满足删除条件 → 维护显示「暂无重复」；同时 `needs_update=true` → 更新列表保留它，且 `updateNote` 正确写出中间版本链。
- 2736+47=2783 与本地文件数一致，说明扫描与计数本身没错。

**结论：B 部分不是数据残留，而是「新版本未下载时旧版应保留在更新列表」的既定语义。建议在 UI 上把 `updateNote` 文案展示清楚，避免误解为幽灵文件。**

### 修复方向（详见计划书任务二）

1. 维护结果为**内存缓存 + 时效失效**：新增 `FinishedAt/RunID`，删除成功后置为过期，前端读到过期结果提示重新扫描。
2. 删除容错：`ErrRecordNotFound` 转为「已不存在，视为已删除」。
3. 维护删除后刷新更新列表，保证两页即时一致。

---

## 2. 排行榜计数不持久化

### 现象

- 排行榜能正常累计，但刷新页面后计数归零。

### 根因

- [`comicStore.ts`](src/stores/comicStore.ts:113) `recordComicClick` **只修改前端内存 store**（`comic.readCount = (comic.readCount || 0) + 1`）并写入 localStorage，**没有任何后端调用**。
- 排行榜 [`OfflineToplist.vue`](src/views/offline/OfflineToplist.vue:42) 的 `rankedComics` 基于前端 `offlineComics` 按 `readCount` 排序。
- 刷新页面时 [`fetchOfflineComics`](src/stores/comicStore.ts:56) 用后端 DB 返回值**整体覆盖** store → DB 中 `readCount` 始终为 0（后端 [`models.go`](backend/internal/models/models.go:44) 有该字段但从未被递增）→ 计数归零。
- 与后端在线榜单 [`toplist.go`](backend/internal/services/toplist.go:1)（E 站 toplist.php）是**两个不同功能**，与本次 BUG 无关。

### 修复方向（详见计划书任务九）

新增 `POST /comics/:id/click` 后端原子 `read_count + 1`；前端 `recordComicClick` 本地自增后 fire-and-forget 上报。

---

## 3. 离线首页翻页不回到顶部

### 现象

- 点击底部翻页后停留在页面底部，没有回到顶部。

### 根因

- [`OfflineHome.vue`](src/views/offline/OfflineHome.vue:222) 与 [`OfflineHistory.vue`](src/views/offline/OfflineHistory.vue:21) 的 `handlePageChange` 使用 `window.scrollTo({ top: 0, behavior: 'smooth' })`。
- 但应用的真实滚动容器是 `#main-content`（overflow-y: auto，见 [`scrollMemory.ts`](src/utils/scrollMemory.ts:31) 头部注释），`window` 本身并不滚动 → 调用无效。
- 对照：[`FloatingToolbar.vue`](src/components/FloatingToolbar.vue:31) 的 `handleScrollTop` 正确查询 `.main-content`，可作为参照。

### 修复方向（详见计划书任务三）

改用 `getMainContent()?.scrollTo({ top: 0, behavior: 'smooth' })`，并抽公共工具 `scrollMainToTop()` 统一。

---

## 4. 离线详情返回位置错误

### 现象

- 在离线详情点返回，期望「从哪里来回哪里去」，实际回到首页**第一页底部**。

### 根因

- 全局滚动记忆（[`router/index.ts`](src/router/index.ts:181) 的 `rememberScroll` + [`scrollMemory.ts`](src/utils/scrollMemory.ts:20) 的 `restoreScroll`）只按 `path` 存 `scrollTop`，**没有保存分页页码与列表状态**。
- 用户在第 N 页进详情：离开时记住的是第 N 页对应的 `scrollTop`；返回后 `currentPage` 已重置为 1，但 `restoreScroll` 试图恢复第 N 页的滚动值 → 第 1 页内容更短 → 浏览器把滚动值**钳制到第 1 页底部**。
- 附加因素：`fetchOfflineComics` 是异步的，`afterEach` 立即 `restoreScroll` 时列表可能尚未渲染（高度为 0），恢复位置进一步失准。

### 修复方向（详见计划书任务五）

滚动记忆升级为「列表状态记忆」`{ top, page }`，返回时先恢复 `page`、数据就绪后再恢复 `scrollTop`。

---

## 5. 其余需求现状梳理

| 需求 | 现状 | 结论 |
|---|---|---|
| 双列对比（更新/维护） | 更新卡片与维护项均不可点击，无对比视图；`OnlineDetailPanel` 内嵌的是「在线」详情，本地无等价紧凑面板 | 需新增独立路由 `/offline/compare` + 紧凑离线详情面板；维护结果 DTO 需补 `pairComic` 成对对象 |
| 每周自动扫描 + Aged Status | 已有可复用的调度范式 [`tag_scheduler.go`](backend/internal/services/tag_scheduler.go:1) 与设置范式 [`TagMaintainSetting`](backend/internal/models/models.go:74) | 新增 `UpdateScanSetting` 单例 + `update_scheduler.go` + `OfflineComic.AgedStatus` |
| 日志监控/查询（四类日归档） | 仅前端错误日志 [`client_log.go`](backend/internal/handlers/client_log.go:30)（`logs/client.log`），无分类、无监控、无查询 | 新增 `log_store.go` 四类日归档 + 查询/尾随/清理接口 + 设置页「日志」栏 |
| 「开启日志/清除日志」 | [`AdvancedSettings.vue`](src/components/settings/AdvancedSettings.vue:4) 开关仅门控前端错误上报（语义不清）；「清除日志」一键全清 | 拆「系统日志」与「前端错误上报」两开关；清除改为按类别+时间范围 |
| 日期跳页 | 仅在线订阅页 [`FloatingToolbar.vue`](src/components/FloatingToolbar.vue:110) 有裸 date 输入框（需按 Enter），无预设节点、无确认按钮；离线首页无入口 | 新增 `DateJumpModal`（节点/日期双 Tab + 确定），订阅页与离线首页共用 |

---

## 6. 总结

- **真正的 BUG 有 4 处**，均已定位到具体函数与根因：维护结果陈旧缓存、点击计数不落库、翻页滚动容器错误、滚动记忆未含分页状态。
- **幽灵文件的「更新列表残留」是既定语义**而非数据残留，需以 UI 文案消除误解，真正要修的是缓存时效与删除容错。
- **其余为新增能力**，均有现成范式可复用（Tag 维护调度/设置、`OnlineDetailPanel`、`client.log`、`seekToDate`），工程量集中在双列对比与日志系统两处。
