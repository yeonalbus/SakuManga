# SakuHentai 四项优化计划书（Round 5）

> 目标：围绕 4 项优化需求产出可落地的实施蓝图，配套 Bug 诊断见 [`round5-diagnostic-report.md`](plans/round5-diagnostic-report.md)。
> 本文档供实现阶段（Code 模式）按「八、分阶段实施」逐步执行。

---

## 一、总览

| # | 需求 | 交付物 | 已确认决策 |
|---|---|---|---|
| 1 | 优化设置布局 | 设置页全面重组，接入 Profile / MyTags | ✅ 全面重组（按主题分组） |
| 2 | 优化更新体验 | 下载完成后主动消除更新标记 | — |
| 3 | 优化维护体验 | (1) 下载后数据库比对 (2) 404 区分删除/网络故障并跳过 | — |
| 4 | 优化下载体验 | (1) 修复单归档串行 (2) 新增优先级 + 最大归档并发数 (3) 修复并发 404 | ✅ 方案 A 统一全局门控 + 抢占式优先级 |

> 第 4 项三项子需求在「统一全局并发门控」架构下**合并实现**（详见第五节），互为表里。

---

## 二、需求 1：设置布局优化

### 现状

- [`SettingsView.vue`](src/views/SettingsView.vue:1) 现有 **13 个菜单项**：account / eh / style / reader / preference / network / download / tag-maintain / update-scan / advanced / logs / security / about。
- `src/components/settings/` 实际有 **16 个组件**，其中 [`ProfileSettings.vue`](src/components/settings/ProfileSettings.vue:1)（E 站 Profile）与 [`MyTagsSettings.vue`](src/components/settings/MyTagsSettings.vue:1)（我的标签）**未接入** SettingsView。
- 新增功能（下载并发、更新扫描、日志、维护）与旧菜单结构不匹配。

### 方案：按主题重组为 7 大分组

| 分组 | 菜单项 | 组件 |
|---|---|---|
| 账户与安全 | account | [`AccountSettings.vue`](src/components/settings/AccountSettings.vue:1)（E 站账户） |
| | profile | [`ProfileSettings.vue`](src/components/settings/ProfileSettings.vue:1)（E 站 Profile，**新接入**） |
| | security | [`SecuritySettings.vue`](src/components/settings/SecuritySettings.vue:1) |
| E 站连接 | eh | [`EHSettings.vue`](src/components/settings/EHSettings.vue:1) |
| | network | [`NetworkSettings.vue`](src/components/settings/NetworkSettings.vue:1)（代理/网络） |
| 阅读体验 | style | [`StyleSettings.vue`](src/components/settings/StyleSettings.vue:1) |
| | reader | [`ReaderSettings.vue`](src/components/settings/ReaderSettings.vue:1) |
| | preference | [`PreferenceSettings.vue`](src/components/settings/PreferenceSettings.vue:1) |
| 下载管理 | download | [`DownloadSettings.vue`](src/components/settings/DownloadSettings.vue:1)（**新增并发/优先级控件**，见需求 4） |
| 离线维护 | update-scan | [`UpdateScanSettings.vue`](src/components/settings/UpdateScanSettings.vue:1) |
| 标签管理 | my-tags | [`MyTagsSettings.vue`](src/components/settings/MyTagsSettings.vue:1)（**新接入**） |
| | tag-maintain | [`TagMaintainSettings.vue`](src/components/settings/TagMaintainSettings.vue:1)（本地 Tag 维护） |
| 高级与日志 | advanced | [`AdvancedSettings.vue`](src/components/settings/AdvancedSettings.vue:1) |
| | logs | [`LogSettings.vue`](src/components/settings/LogSettings.vue:1) |
| 关于 | about | [`AboutSettings.vue`](src/components/settings/AboutSettings.vue:1) |

### 实施要点

1. `SettingsView.vue` 的菜单 `menuKey` 数组重组为上述 7 组（沿用 `adminOnly` 过滤逻辑）。
2. `ProfileSettings.vue` / `MyTagsSettings.vue` 目前以「返回按钮 + 页面容器」方式独立承载，需接入后与其它设置组件保持一致的子面板渲染方式（改用统一 `<component :is>` 分发，去掉内部 back 按钮或改为返回分组）。
3. `DownloadSettings.vue` 内新增「并发控制」分区（见需求 4 前端部分）。

---

## 三、需求 2：更新体验优化（下载完成消除更新标记）

### 现状

- [`download.go`](backend/internal/services/download.go:205) `finalizeUpdate` 仅在 `task.UpdateForComicID != ""`（即「更新任务」）时，清除旧漫画的 `NeedsUpdate/NewGID/NewToken/UpdateNote`。
- **普通下载任务完成后不清除更新标记**：若用户在「更新」页手动下载了新版（或下载了某本被标记需要更新的漫画），完成后 `NeedsUpdate` 仍为 true。

### 方案

扩展下载完成收尾逻辑，新增可复用函数（放 [`offline.go`](backend/internal/services/offline.go:1) 或新 `offline_clear.go`）：

```go
// ClearOfflineUpdateByGID 按下载任务的新 GID 反向匹配更新列表并清除更新标记
func ClearOfflineUpdateByGID(db *gorm.DB, gid string) error {
    return db.Model(&models.OfflineComic{}).
        Where("needs_update = ? AND (new_gid = ? OR gid = ?)", true, gid, gid).
        Updates(map[string]interface{}{
            "needs_update": false, "new_gid": "", "new_token": "",
            "update_note": "", "updated_at": time.Now(),
        }).Error
}
```

在 [`runTask`](backend/internal/services/download.go:160) 的任务 completed 分支（[`finalizeUpdate`](backend/internal/services/download.go:205) 之后）统一调用：

```go
// 需求2：任何下载任务完成后，主动清除更新列表中同 GID 的更新标记
if task.Status == StatusCompleted && task.GID != "" {
    if err := ClearOfflineUpdateByGID(m.db, task.GID); err != nil { /* log */ }
}
```

> 复用现有 `clearOfflineUpdate` 的字段清空语义，避免与维护查重、老化判定产生新状态冲突。完成后刷新前端「更新」列表即见标记消除。

---

## 四、需求 3：维护体验优化

### 3(1) 下载完成后主动比对数据库

#### 方案

在下载任务 completed 收尾处新增 `ReconcileOfflineAfterDownload(db, task)`：

1. **GID 去重**：按 `task.GID` 查询 `offline_comics`，若已存在同 GID 记录且 `local_path` 不同 → 判定为重复来源，记录到维护查重结果（复用 [`maintainDedupWithProgress`](backend/internal/services/offline.go:522) 的规则 2 gid 维度），提示用户去维护页处理。
2. **ParentGID 回写**：读取本次下载产物 `metadata/ametadata` 中的 `parent_gid`，若本地记录为空则回写 `OfflineComic.ParentGID`（供维护规则 3 与更新 B 段复用）。
3. **PageCount 校正**：metadata 的 `filecount` > 本地 `PageCount` → 更新 `PageCount`。
4. **Aged 状态复位**：若该漫画曾被标记 `AgedStatus=true`，本次成功下载新版后复位 `AgedStatus/AgedCheckedAt`，使其重新参与后续扫描。

> 复用 [`metadata.go`](backend/internal/services/metadata.go:26) 已解析的元数据；实现位置可复用 [`ScanAndSaveDirectory`](backend/internal/services/scanner.go:1) 后的结果做轻量比对，不重复扫描。

### 3(2) 404 区分「画廊被删除」与「网络故障」并跳过

#### 现状

- [`eh_reader.go`](backend/internal/services/eh_reader.go:1) `classifyGalleryUnavailable` 已将 404 分类为 `ErrGalleryUnavailable{Kind: "removed" / "copyright"}`。
- [`offline.go`](backend/internal/services/offline.go:103) `checkUpdatesWithProgress` 中 `FetchGalleryDetail` 出错仅 `log` + `continue`，**不持久化任何「已删除」状态** → 下次维护/更新检测仍会重复联网。

#### 方案

1. **数据模型**（见第六节）：`OfflineComic` 新增 `RemovedStatus bool` + `RemovedAt int64`。
2. **持久化**：在 `checkUpdatesWithProgress` 的 A 段错误分支与 `ageCheckWithProgress` 中：
   ```go
   var gu *ErrGalleryUnavailable
   if errors.As(err, &gu) && (gu.Kind == "removed" || gu.Kind == "copyright") {
       c.RemovedStatus = true
       c.RemovedAt = time.Now().UnixMilli()
       changed[c.ID] = true
   } else {
       // 网络故障：仅 log，不标记删除
   }
   ```
3. **跳过**：`filterOfflineUpdateEnabled`（[`offline.go`](backend/internal/services/offline.go:884)）增加 `RemovedStatus != true` 条件；`maintainDedupWithProgress` 的候选集同样过滤（一次性跳过被删画廊，避免重复联网）。
4. **前端**：[`OfflineUpdate.vue`](src/views/offline/OfflineUpdate.vue:1) 更新列表对 `removedStatus=true` 项显示「画廊已被删除/移除」徽标，并提供「移出列表 / 删除本地记录」（复用 [`DeleteOfflineComic`](backend/internal/services/offline.go:915)）。

> 区分依据：`ErrGalleryUnavailable.Kind` 语义——`removed/copyright` = 画廊被删（持久化跳过）；其它 HTTP/网络错误 = 临时故障（不标记，下次重试）。

---

## 五、需求 4：下载体验优化（核心）

> 4(1) 拆分为两个**独立现象**（详见 [`round5-diagnostic-report.md`](plans/round5-diagnostic-report.md) 3.2）：
> - **现象 A「单文件只有 1 线程」**（单文件/多文件每文件均约 2mb/s，未用满 10 线程）：代码层面直接原因是 **Range 探测失败（404）→ 强制单线程 `downloadZip`**，**与线程池无关**。**JHentai 实证同一套 `start=1` H@H 直链 + Range 分片可行**（[`_getDownloadUrl`](JHentai/HathDownload/archive_download_service.dart:965) + [`_generateDownloadTask`](JHentai/HathDownload/archive_download_service.dart:610)）→ **优先修复探测与分块路径**，详见 **5.7**。
> - **现象 B「多文件串行」**：根因是线程池**全有或全无语义**（首个归档 `acquire(10)` 占满），由**统一全局并发门控**（方案 A）+ `MaxArchiveConcurrency` 解决。
> 4(2) 新增控制项、4(3) 并发 404 修复在统一全局并发门控（方案 A）下实现；4(3) 的 404/410 分诊与 3(2) 的「画廊被删」判定可借鉴 JHentai 成熟实现（见 **5.8**）。

### 5.1 全局并发额度池（升级 [`archive_thread_pool.go`](backend/internal/services/archive_thread_pool.go:1)）

把仅服务归档的 `archiveThreadPool` 升级为**全局下载并发额度池**：

- 保留 `acquire(taskID, n, stop) / releaseAll(taskID) / adjust(taskID, target) / wakeAll()` 接口与 `sync.Cond` 阻塞唤醒机制（现有测试 [`archive_download_test.go`](backend/internal/services/archive_download_test.go:1) 可复用）。
- 全局 `max` = `ArchiveThreads`（默认 10，即 E 站线程上限）。
- **新增画廊接入**：画廊引擎在启动下载前 `acquire(min(ConcurrentImageDownloads, 余量))`，逐图下载占用额度，完成 `releaseAll`。
- **新增假死语义**：额度不足时 `acquire` 阻塞（假死）→ **不消耗 GP、不爬取画廊**；空位出现 `wakeAll` 唤醒。归档的 `acquire` **提前到 `resolveArchiveDownloadURL` 之前**（详见 5.3）。
- **等待任务排序唤醒（借鉴 JHentai `_tryWakeWaitingTasks`）**：阻塞等待的任务按入队时间（`CreatedAt`）排序，空位出现时按序唤醒，保证公平性（详见 5.8）。

### 5.2 MaxArchiveConcurrency 最大归档并发数

新增 `DownloadSetting.MaxArchiveConcurrency`（范围 1-10，且 `<= ArchiveThreads`，前端滑块联动约束）。

**每归档额度分配规则**（用户给定示例的工程化定义）：

```
base      = ArchiveThreads / MaxArchiveConcurrency   // 向下取整
remainder = ArchiveThreads % MaxArchiveConcurrency
前 remainder 个归档各分配 (base + 1) 线程，其余分配 base 线程
若 base == 0（ArchiveThreads < MaxArchiveConcurrency）→ 只有前 ArchiveThreads 个归档各 1 线程，其余等待
```

| Max 并发 | 归档线程 | 分配结果 |
|---|---|---|
| 10 | 10 | base=1, rem=0 → 每归档 1 线程（用户示例 ✅） |
| 6 | 10 | base=1, rem=4 → 前 4 个归档 2 线程，后 2 个 1 线程（近似用户「第一个 3 其余 2」的均分化表达） |
| 10 | 6 | base=0 → 只下载前 6 个归档，每归档 1 线程（用户示例 ✅） |

> 实现：归档任务在 `acquire` 阶段以「任务启动序号」确定其在并发窗口内的排位，按上述规则申请线程数；`ControlArchiveConcurrency=true` 且任务启动序号超出 `MaxArchiveConcurrency` 时阻塞等待（归档并发数上限）。

**`ControlArchiveConcurrency` 开关**：保留；关闭时 `MaxArchiveConcurrency` 不生效（等价旧行为：无并发上限、每归档可拿满 `ArchiveThreads`）。

### 5.3 归档引擎改造（[`download_archive.go`](backend/internal/services/download_archive.go:926)）

1. **acquire 提前**：将 `archivePool.acquire` 从 `downloadArchiveFile`（zip 下载阶段）**提前到 `resolveArchiveDownloadURL`（archiver.php 解锁）之前**——额度不足时阻塞假死，不提前消耗 GP。
2. **每归档额度 = `base`/`base+1`**（按 5.2 规则），替代现在「固定 `threads = ArchiveThreads`」。
3. 保留断点续传、`.part`、zip 校验、H@H 直链单线程路径等既有逻辑。

### 5.4 画廊引擎改造（[`download_gallery.go`](backend/internal/services/download_gallery.go:132)）

1. **接入全局额度池**：`downloadAll` 开始前 `acquire(min(ConcurrentImageDownloads, 余量))`，逐图下载占额度，结束 `releaseAll` → 多画廊并行总线程 ≤ 10（修复路径一 N×10 超限）。
2. **补充停止能力**：为抢占式优先级提供中断手段——仿照归档 `stopDownload`，给 `galleryDownloader` 增加停止标记 + 中断进行中图片请求的机制（复用 `.part` 断点续传恢复）。

### 5.5 优先级 + 抢占式调度（[`download_scheduler.go`](backend/internal/services/download_scheduler.go:1)）

1. **出队排序**：queued 任务按 `Priority DESC, CreatedAt ASC` 出队。
2. **抢占式**：新任务入队 / 优先级提升时调用 `preemptLowerPriority(newPriority)`：
   - 找出比 `newPriority` 低、正在 running 且占满全局额度的任务 → 立即 `stopArchiveDownload` / 画廊 stop，释放额度；
   - 被抢占任务置 `status=queued`（或 `paused` + 抢占标记 `preempted=true`），高优先级任务入队调度；
   - 高优先级完成后自动恢复被抢占任务（重新入队）。
3. **画廊 + 归档统一抢占**：两者都需暴露 stop 能力（归档已有 `stopArchiveDownload`，画廊新增）。

### 5.6 设置变更通知（[`download.go`](backend/internal/services/download.go:753) `SaveSettings`）

- `MaxArchiveConcurrency` 或 `ArchiveThreads` 变化 → `notifyArchiveThreadsChange` + 唤醒额度池重新分配（`wakeAll`）。
- 新增 **优先级变化通知**：`SetTaskPriority` 后触发 `preemptLowerPriority`。

### 5.7 现象 A「单文件用满线程」：对照 JHentai 修复探测与分块路径

> 背景：单归档的线程数由 [`probeArchiveDownload`](backend/internal/services/archive_chunk.go:377) 的 Range 探测结果决定——返回 **206 + Content-Range** 才启用分块多线程（`useChunk`，[`download_archive.go`](backend/internal/services/download_archive.go:949)）；返回 **404** 则强制回退单线程 [`downloadZip`](backend/internal/services/download_archive.go:783)（[`archive_chunk.go`](backend/internal/services/archive_chunk.go:415) 注释将其解释为「H@H 不支持 Range」）。带 `start` 参数的 H@H 直链（[`resolveHathdlDownloadURL`](backend/internal/services/download_archive.go:546) 强制 `start=1`）也被 [`isHathStreamDownloadURL`](backend/internal/services/download_archive.go:514) 判定为流式 → 单线程。**当前实测单文件恒为 1 线程（约 2mb/s）。**

> ⚠️ **JHentai 反证**：JHentai 用**同一套 `start=1` H@H 直链 + 多 Isolate Range 分片**成功下载（[`_getDownloadUrl`](JHentai/HathDownload/archive_download_service.dart:965) 强制 `start=1` + [`_generateDownloadTask`](JHentai/HathDownload/archive_download_service.dart:610) `isolateCount: archiveDownloadIsolateCount`），证明**「H@H 不支持 Range」不是绝对结论**。我们探测 404 更可能是探测方式/时机/头差异所致。因此修复方向是**对照 JHentai 复现并修复我们的探测与分块路径**：

| 对照项 | JHentai 做法（[`JHentai/HathDownload`](JHentai/HathDownload/archive_download_service.dart:1)） | SakuHentai 现状（待修复） |
|---|---|---|
| 探测 | `JDownloadTask` 先 `fetchContentLength`（轻量探测拿总大小） | GET + `Range: bytes=0-0`（[`probeArchiveDownload`](backend/internal/services/archive_chunk.go:377)），404 即回退单线程 |
| 分片 | 多 Isolate 对同一 URL 分段 Range 并行下载 | `useChunk=false` 时整体单线程 [`downloadZip`](backend/internal/services/download_archive.go:783) |
| 直链判定 | 带 `start=1` 直链可直接多线程分片 | [`isHathStreamDownloadURL`](backend/internal/services/download_archive.go:514) 把带 start 判为流式单线程 |

**实施步骤**：
1. 先触发一次归档下载，观察 `[ARCHIVER]` 日志探测码（206 vs 404）与下载域名（复现基线）。
2. **按 JHentai 方式改造探测**：改为轻量探测（HEAD 或小 Range），核对 Referer/UA/探测时机（解锁后稍等节点就绪再探测）。
3. **重新评估 `isHathStreamDownloadURL`**：带 `start=1` 直链不应直接判流式单线程，应参与 Range 探测分块（若第 2 步探测 206 则恢复分块多线程）。
4. 修复后复测：单文件应可用满 `ArchiveThreads`（≈20mb/s）。若修复后**仍稳定复现 404**，才认定服务端限制，走兜底方案（`MaxArchiveConcurrency` 多文件并行 + 画廊逐图多线程提升总吞吐）。

> 补充：即使归档分块受限，**画廊逐图模式天然多线程**（[`download_gallery.go`](backend/internal/services/download_gallery.go:132) `ConcurrentImageDownloads` 逐图并发），单本漫画走逐图下载仍可获得多线程提速，可作现象 A 的替代方案向用户说明。

### 5.8 借鉴 JHentai 的成熟实现（4(3) / 3(2) / 4(2)）

| 借鉴点 | JHentai 实现 | 落地到 SakuHentai |
|---|---|---|
| **假死 + 空位唤醒**（方案 A 实现范式） | [`waitingIsolate`](JHentai/HathDownload/archive_download_service.dart:1012) 状态 + [`_tryWakeWaitingTasks`](JHentai/HathDownload/archive_download_service.dart:682) 统计 running 任务 activeIsolateCount（active=0 时用 isolateCount 兜底），按 insertTime 排序逐个唤醒 | 全局额度池 `acquire` 阻塞（假死）+ `wakeAll` 空位唤醒；等待任务按入队时间排序（见 5.1） |
| **运行中并发数热更新**（4(2)「及时暂停/开启线程」） | [`_onIsolateCountChange`](JHentai/HathDownload/archive_download_service.dart:705) 对 running 任务 `changeIsolateCount` | `SaveSettings` / 优先级变更时对 running 归档调用 `adjust(taskID, target)` 实时增减线程（见 5.6） |
| **410/404/429 body 分诊**（4(3)） | [`_check410Or404Reason`](JHentai/HathDownload/archive_download_service.dart:637)：「too many downloaded bytes」/「too many different locations」/「IP quota exhausted」→ 需重新解锁；「Expired or invalid session」→ 暂停；其余 → 重解析 URL 重试；429 → 暂停（[`archive_download_service.dart`](JHentai/HathDownload/archive_download_service.dart:1037)） | 扩展现有 `classifyArchiveLockBody`（[`archive_chunk.go`](backend/internal/services/archive_chunk.go:355)），分诊后按语义置锁定原因 / 重新解锁 / 暂停 |
| **404 画廊删除判定**（3(2)） | [`a404Page2GalleryDeletedHint`](JHentai/HathDownload/eh_spider_parser.dart:976) 解析 `.d > p`：「This gallery has been removed」→ removed；「copyright claim by (.*)」→ copyright；配合 [`_convertExceptionIfGalleryDeleted`](JHentai/HathDownload/eh_request.dart:1018) | `ErrGalleryUnavailable` 分诊为 removed/copyright，持久化 `RemovedStatus` |
| **取消归档释放会话** | [`cancelArchive`](JHentai/HathDownload/archive_download_service.dart:216) 发 `invalidate_sessions: 1`（[`requestCancelArchive`](JHentai/HathDownload/eh_request.dart:861)） | 410/超限/取消时主动 `invalidate_sessions` 释放归档会话配额（可选增强） |

---

## 六、数据模型变更

### [`models/download.go`](backend/internal/models/download.go:1) — `DownloadSetting` 新增

```go
MaxArchiveConcurrency int `gorm:"default:1" json:"maxArchiveConcurrency"` // 最大归档并发数 1-10
```

> ✅ **默认值已确认（用户决策 #4）**：默认 1 = 保持单归档全线程（单本速度最快）；用户需要多归档并行时再到设置页调高。

### [`models/models.go`](backend/internal/models/models.go:28) — `OfflineComic` 新增

```go
RemovedStatus bool  `gorm:"default:false" json:"removedStatus"` // 画廊被删除/版权移除（需求 3(2)）
RemovedAt     int64 `json:"removedAt,omitempty"`                // 标记时间戳(ms)
```

### 迁移

- `AutoMigrate` 自动补列（`DownloadSetting`、`OfflineComic`）。
- 兼容旧数据：`GetSettings` 中 `MaxArchiveConcurrency == 0` 时补默认值（仿 [`download.go`](backend/internal/services/download.go:718) 既有迁移范式）。

---

## 七、API 与前端变更

### 后端 API

| 接口 | 说明 |
|---|---|
| `POST /api/v1/downloads/:id/priority` | body `{priority: int}`，修改任务优先级并触发抢占调度（新增） |
| `GET/POST /api/v1/downloads/settings` | 复用，字段扩展 `maxArchiveConcurrency` |
| `GET /api/v1/offline/updates` | 复用，`OfflineComic` 序列化自动带 `removedStatus/removedAt` |

### 前端

1. **`src/stores/downloadSettings.ts`**：`DownloadSettings` 接口与默认值补 `maxArchiveConcurrency`。
2. **`src/components/settings/DownloadSettings.vue`**：新增「并发控制」分区：
   - 最大归档并发数滑块（1-10，联动 `archiveThreads`，校验 `<= ArchiveThreads`）；
   - `controlArchiveConcurrency` 开关说明与联动。
3. **`src/views/DownloadsView.vue`**：任务列表新增**优先级列**（数字/星标展示 + 内联修改，调用 `POST :id/priority`）；「下载优先级」默认 0。
4. **`src/views/SettingsView.vue`**：按第二节重组菜单，接入 Profile/MyTags。
5. **`src/views/offline/OfflineUpdate.vue`**：展示「画廊已删除」徽标（需求 3(2)）。
6. **`src/api/download.ts`**：新增 `setTaskPriority` 方法。

---

## 八、分阶段实施（todo 蓝图）

> 每步可独立交付与验证，按依赖顺序执行。

1. **数据模型 + 迁移**：`DownloadSetting.MaxArchiveConcurrency`、`OfflineComic.RemovedStatus/RemovedAt`，`AutoMigrate` 补列 + 旧数据默认值迁移。
2. **全局并发额度池升级**：`archive_thread_pool.go` 支持画廊申请、假死阻塞语义、`MaxArchiveConcurrency` 每归档额度分配；更新 [`archive_download_test.go`](backend/internal/services/archive_download_test.go:1) 断言。
3. **现象 A：修复探测与分块路径（对照 JHentai）**：触发归档下载观察探测基线（206/404）→ 按 **5.7** 改造探测（轻量探测 + 复核 Referer/时机）→ 重新评估 `isHathStreamDownloadURL`（带 start 直链参与 Range 分块）→ 复测单文件用满线程；仅修复后仍稳定 404 才走兜底（多文件并行 + 画廊逐图）。
4. **归档引擎改造**：`download_archive.go` acquire 提前到解锁之前 + 每归档额度按 5.2 规则。
5. **画廊引擎改造**：`download_gallery.go` 接入额度池 + 新增停止能力。
6. **优先级 + 抢占调度**：`download_scheduler.go` 出队排序 + `preemptLowerPriority`；`download.go` 优先级设置方法 + 变更通知；新增 `POST /downloads/:id/priority` 接口。
7. **需求 2**：`ClearOfflineUpdateByGID` + `runTask` 收尾调用。
8. **需求 3(1)**：`ReconcileOfflineAfterDownload`（GID 去重 / ParentGID 回写 / PageCount 校正 / Aged 复位）。
9. **需求 3(2)**：`ErrGalleryUnavailable` 持久化 `RemovedStatus` + 两处跳过过滤 + 前端徽标。
10. **设置布局重组**：`SettingsView.vue` 7 分组 + 接入 Profile/MyTags；`DownloadSettings.vue` 并发控件；`downloadSettings.ts` store 扩展。
11. **下载列表优先级 UI**：`DownloadsView.vue` 优先级列 + 修改交互；`api/download.ts` 新方法。
12. **联调 + 回归**：全链路（创建→并发调度→完成→更新标记消除→维护跳过）验证；确认并发 ≤ 10 无 404；更新相关测试。

---

## 九、决策点确认记录

| # | 决策点 | 结论 |
|---|---|---|
| 1 | 4.3 修复方案 | ✅ 方案 A：统一全局并发门控（额度不足假死、不耗 GP） |
| 2 | 优先级变更策略 | ✅ 抢占式：高优先级立即暂停低优先级 running 任务，完成后自动恢复 |
| 3 | 设置布局方向 | ✅ 全面重组：7 大分组，接入 Profile / MyTags |
| 4 | `MaxArchiveConcurrency` 默认值 | ✅ **默认 1**（已确认）：保持单归档全线程，用户按需调高 |
| 5 | 画廊下载是否也纳入全局门控 | ✅ 纳入（`min(ConcurrentImageDownloads, 余量)`），保证总并发 ≤ 10 |
| 6 | `ControlArchiveConcurrency` 开关 | ✅ 保留，与 `MaxArchiveConcurrency` 联动（关闭时不限制） |
| 7 | 4(1) 现象 A（单文件慢）修复 | ✅ **按 JHentai 实证修复探测与分块路径**（`start=1` 直链 + Range 分片可行）；仅修复后仍稳定 404 才认服务端限制，走多文件并行 / 画廊逐图兜底（见 5.7） |

---

## 十、风险与注意

- **画廊引擎新增停止能力**是抢占式的前提，需与 `.part` 断点续传结合，避免中断后丢进度。
- `MaxArchiveConcurrency` 与 `ArchiveThreads` 需前端联动校验（并发 ≤ 线程数），后端 `SaveSettings` 同样兜底钳制。
- 全局额度池改造需同步更新 [`archive_download_test.go`](backend/internal/services/archive_download_test.go:1) 既有断言，防止回归。
- 3(2) 的 `RemovedStatus` 只针对「画廊被删」（`Kind=removed/copyright`），网络故障**不得**标记，避免误跳过。
- **4(1) 现象 A 探测修复可能反复**：JHentai 实证 `start=1` 直链 + Range 分片可行，但 H@H 节点行为可能因节点/时段/域名而异（部分节点拒绝 Range 仍可能发生）；改造后需在真实账号下复测确认，若部分节点仍 404 需保留单线程兜底（详见 5.7）。
