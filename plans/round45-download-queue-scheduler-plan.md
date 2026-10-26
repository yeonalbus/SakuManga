# Round45：下载队列重构（无界队列 + 统一调度器）+ 归档失败禁止自动降级（已确认）

> 状态：**方案已确认（珱垣拍板），待动工**。
> 起因：2026-10-26 实机（`Y:\SakuManga`）两个 Bug：
> 1. 普通下载任务归档失败被自动降级为画廊逐图下载，落地画质不达标，还需再到「图片质量升级」重下一遍；
> 2. 排队 704 个任务不进「下载中」，必须手动「暂停 → 恢复」才有一批开始下载。
> 取证基线：实机 `manga.db`（2026-10-26 21:32 快照）+ `logs/download_log-2026-10-24/25/26.log` + `logs/update_log-2026-10-25/26.log`。

---

## 一、实测归因

### 1.1 Bug1：归档失败自动降级（三层原因叠加）

| # | 位置 | 事实 |
|---|------|------|
| A | `services/log_store.go:149-168` `classifyLogLine` | 按行内 `[update]` 子串分流，「更新」分类优先级最高 |
| B | `services/download.go:384-406` `fallbackArchiveToGallery` | 日志前缀硬编码 `[DOWNLOAD-WARN] [update]`，且被**普通下载任务**调用 → 降级记录整批落进 `update_log` |
| C | `services/download_archive.go:400-416` | `resolveHathdlDownloadURL` 的**任何**错误（含 20s 超时 / EOF）都被包装成 `errHathdlOnly` |
| D | `services/download_archive.go:243-256` | 收到 `errHathdlOnly` 即 `fallbackArchiveToGallery`（改 `mode=gallery` 后重新入队） |
| E | `services/download_archive.go:245,277`、`services/download.go:355-378` | 开关 `autoUpdateFallbackToGallery` **仅在 `UpdateForComicID != ""`（更新任务）时被检查**，普通下载无条件降级 |

实测数据：

- `update_log-2026-10-26.log` 16 行、**100%** 是普通下载任务的降级日志；`update_log-2026-10-25.log` 4205 行中含 32 条同类降级。
- 降级失败原因全部是网络层：`请求 H@H 下载页失败: Get https://xxx.hath.network/... context deadline exceeded` / `EOF`（客户端 `Timeout: 20s`，`eh_auth.go:155`）。
- 数据库 `download_settings.auto_update_fallback_to_gallery = 0`（用户已关闭），但 8 个 gid 仍被降级：落地 `Z:\SakuHentai\Comics\<gid> - <标题>`、`download_scheme='gallery'`，因此进入「图片质量升级」候选列表，需再点一次升级。

### 1.2 Bug2：队列丢任务（结构性缺陷）

| # | 位置 | 事实 |
|---|------|------|
| A | `services/download.go:140` | `queue := make(chan string, 256)`，`workers = 16` |
| B | `services/download.go:556-565` `Enqueue` | `select/default`：队列满 → 只打一条 WARN，**任务永久留在 queued，无任何补偿** |
| C | `services/download.go:224-225` | `acquireSlot()` 后无条件 `defer releaseSlot()`；等待期间任务被暂停/取消时 `acquireSlot` 未占槽位却仍释放 → `runningTotal/runningByPri` 计数错乱 |
| D | `services/archive_thread_pool.go:195-236` `archiveSlot` | `MaxArchiveConcurrency=1` 时 15 个协程阻塞在 `cond.Wait`，各自占住一个 worker |
| E | `services/download.go:956-977` `RestoreTasks` | 重启时一次性 `Enqueue` 全部 queued/downloading，超过 256 的部分同样丢弃 |

实测数据：

- `download_log-2026-10-24.log`：入队成功 347 / **入队失败（队列已满）175**；
- `download_log-2026-10-25.log`：入队成功 437 / **入队失败 718**；
- 10-24 23:28 起 517 个、10-25 13:51–14:03 起 1066 个批量任务（共 1583），终态 completed 869 / cancelled 3 / error 13，**其余 704 个长期 queued**（`update_for_comic_id` 全空、`force_delete_original` 全 0，确认是普通下载任务）；
- `download_log-2026-10-25/26` 中「归档并发数已满（1/1），排队等待…」合计 12491 条 → worker 被阻塞等待占满，channel 停止被消费，遂批量丢弃；
- 唯一的复活路径是 `ResumeTask`（`download.go:596-615`）重新 `Enqueue`，即用户观察到的「暂停 → 恢复」；
- 10-26 用户手动暂停 297 次 / 恢复 301 次，仅补入 341 个。

---

## 二、目标与非目标

### 目标

1. **绝不自动降级**：归档下载失败一律 `error`（可重试），任务 `mode` 保持 `archive`，不再落地为画廊逐图。
2. **失败分级**：网络层错误（超时/EOF/连接重置/5xx）标注为可重试并自动退避重试若干次；结构性错误（H@H 下载页 200 但解析不到直链）直接报错并给出明确文案。
3. **队列不丢**：无界 pending 队列 + 统一调度器，任何 `queued` 任务都必达调度器；批量 1000+ 不丢一个。
4. **等待不占执行线程**：并发窗口（`MaxArchiveConcurrency`）、线程配额（全局 10 线程）、优先级串并行门控全部前移到调度器判定，执行协程只在获牌后启动。
5. **日志分类修正**：`[update]` 只用于真正的更新语义；分类改为「标签位置化解析」。
6. **前端清理**：移除「无 H@H 时自动降级为画廊下载」开关（后端字段保留但废弃）。

### 非目标

- 不改下载引擎的下载算法（分块 / Range 续传 / 解压 / 入库 / 元数据）；
- 不改任务状态枚举（仍 7 态：queued / downloading / paused / completed / error / error_lock / cancelled），前端零改动；
- 不改 E 站 API 调用流程（archiver.php / H@H 直链）；
- 不引入新的持久化表 / 迁移。

---

## 三、方案设计

### 3.1 数据模型与状态语义（不变）

| 状态 | 重构后语义 |
|------|-----------|
| `queued` | 在 pending 队列中等待调度（可能在等优先级、等归档并发窗口、等线程配额） |
| `downloading` | 已获牌（槽位 + 线程额度），执行协程在跑 |
| `paused` / `cancelled` | 已从 pending 移除并中断引擎（语义不变） |
| `error` / `error_lock` / `completed` | 终态（`error` 可 retry → 回 pending） |

不新增 `waiting` 状态：等待即 `queued`，前端「排队中」语义更准确，且无需改前端状态映射。

### 3.2 无界 pending 队列

新增 `backend/internal/services/download_queue.go`：

```go
type pendingQueue struct {           // 无界，按「优先级降序 + 创建时间升序」取队首
    mu   sync.Mutex
    items map[string]*models.DownloadTask  // id → 快照（用于排序，状态以 DB 为准）
    cond *sync.Cond                        // 信号（任务入队 / 资源释放 / 设置变更）
}
func (q *pendingQueue) Push(t *models.DownloadTask)   // 幂等：已存在则更新优先级
func (q *pendingQueue) Remove(id string)              // 暂停/取消/抢占时移除
func (q *pendingQueue) Peek() (*models.DownloadTask, bool)
func (q *pendingQueue) Pop() *models.DownloadTask
func (q *pendingQueue) Len() int
func (q *pendingQueue) Wake()                         // 广播唤醒（等价 Cond.Broadcast）
```

约束：无容量上限；`Enqueue` 永不失败；重复入队幂等（同一 task 不会被执行两次）。

`DownloadManager` 字段变化：

- 删除：`queue chan string`、`workers int`、`taskRunMu/taskRunning`（并入 `running` 集合）；
- 新增：`pending *pendingQueue`、`running map[string]*reservation`（任务 ID → 已预留资源）、`signal chan struct{}`（可选，或直接用 cond）；
- 保留：`schedMu`、`runningTotal`、`runningArchive`、`runningByPri`、`runningTasks`（抢占所需），改为由调度器统一维护。

### 3.3 统一调度器（`download_scheduler.go` 重构）

```go
func (m *DownloadManager) Start() {
    go m.schedulerLoop()        // 替代 16 个 worker
    if AutoResumeTasks { m.RestoreTasks() }
}

func (m *DownloadManager) schedulerLoop() {
    for {
        m.waitSignal()                       // pending 变化 / 资源释放 / 设置变更 / 1s 兜底 ticker
        for {
            task, ok := m.pickNext()          // 队首；跳过 DB 状态已非 queued 的（自愈清理）
            if !ok { break }
            if !m.tryReserve(task) { break }  // 非阻塞：优先级门控 + 归档并发窗口 + 线程配额
            m.pending.Pop()
            go m.executeTask(task)            // markRunning → dispatchEngine → 收尾 → releaseReservation
        }
    }
}
```

**`tryReserve`（非阻塞，持 `schedMu` + `archivePool.mu`，顺序固定 schedMu → pool.mu）**：

1. 优先级串并行门控（沿用 `slotAvailable` 语义）：
   - `downloadAllGalleriesSamePriority=false`：`runningTotal > 0` → 不放行（串行）；
   - `true`：`task.Priority < maxRunningPriority()` → 不放行。
2. 归档任务：`len(archives) >= archiveEffConcurrency(threads, maxConcurrency)` → 不放行；否则按 `archiveQuota(threads, maxConcurrency, idx)` 申请线程，`active+need > 10` → 不放行。
3. 画廊任务：`avail = 10 - active`，`avail <= 0` → 不放行；否则线程数取 `min(ConcurrentImageDownloads, avail)`。
4. 成功后写入 `runningTasks / runningByPri / runningTotal / runningArchive`、`archivePool.perTask`，返回 `*reservation{threads, kind, window}`。

**`executeTask`**：

```go
func (m *DownloadManager) executeTask(task *models.DownloadTask) {
    defer m.releaseReservation(task.ID)     // 释放线程额度 + 并发窗口 + 槽位计数
    if err := m.markRunning(task); err != nil { return }
    if st := m.reloadStatus(task.ID); st != downloading { return }  // 抢占让位
    m.dispatchEngine(task, reservation)     // 引擎使用预留线程数，不再自行阻塞申请
    m.finalizeUpdate(task)                  // 保留
    m.postDownloadBookkeeping(task)         // ClearOfflineUpdateByGID / Reconcile / MarkLibraryChanged
    if st := m.reloadStatus(task.ID); st == queued { m.Enqueue(task.ID) } // 抢占后自动重新排队
}
```

引擎内改动：

- `archiveDownloader.run()`：删除 `g.acquireSlot()` 与其 `defer g.releaseSlot()`（`download_archive.go:232-238`），改为 `g.threads = reservation.threads`；`onArchiveThreadsChange` 仍走 `archivePool.adjust`（perTask 已由预留写入）。
- `galleryDownloader.downloadAll()`：删除 `acquirePartial`（`download_gallery.go:149-159`），并发直接用 `reservation.threads`。
- `archiveThreadPool`：
  - 新增 `tryArchiveSlot(taskID, threads, maxConcurrency) int`（非阻塞，0=失败）与 `tryAcquirePartial(taskID, n) int`（非阻塞）；
  - 保留 `adjust / releaseAll / releaseArchive / wakeAll / currentActive`；
  - `archiveSlot / acquirePartial`（阻塞版）删除或降级为测试辅助。

**唤醒时机（`Wake()`）**：任务创建 / 恢复 / 重试 / 解锁 / 抢占回退 / 任务执行结束释放资源 / `SaveSettings`（线程数、并发窗口、串并行改变）/ 1s ticker 兜底。

**抢占**：`preemptLowerPriority` 逻辑保留，`preemptOne` 增加 `m.pending.Remove(id)` 后回队首语义不变（置 queued → 中断 → 等引擎退出 → `Enqueue` → `Wake`）。

**暂停 / 取消**：`PauseTask` / `CancelTask` 先 `m.pending.Remove(id)`（防调度器刚发牌），再保留现有中断与等待逻辑。

**幂等与自愈**：

- `Enqueue`：若任务已在 `pending` 或 `running` 中则直接返回；
- `pickNext`：出队前回查 DB 状态，非 `queued` 则丢弃（覆盖"用户暂停时任务仍在队列"）；
- 1s ticker：即使信号丢失也能推进。

### 3.4 归档失败：绝不降级

1. 删除 `services/download.go:352-406` 的 `fallbackUpdateToGallery` / `fallbackArchiveToGallery`，删除 `runTask` 中的调用点。
2. 删除 `errHathdlOnly` 的「降级」语义（`download_archive.go:566-568`），改为错误分类：

```go
var (
    errArchiveNetworkRetryable = errors.New("归档下载网络请求失败（可重试）")
    errArchiveHathdlStructure  = errors.New("H@H 下载页无可解析直链（无法直链下载，非网络问题）")
)
```

3. `resolveHathdlDownloadURL`：
   - 网络层错误（`net.Error` 超时、`io.EOF`、连接重置、5xx）：**先退避重试 2 次**（1s / 3s），仍失败 → `errArchiveNetworkRetryable`（任务 `error`，文案含「可重试」）；
   - 页面 200 但 `#db > p > a` 解析不到 → `errArchiveHathdlStructure`（任务 `error`）；
   - 归档仍在生成（6 次拿不到 `#continue > a`）→ 现有语义保留（`error`，可重试）。
4. `run()`（`download_archive.go:240-300`）：
   - 删除 `errHathdlOnly` 分支与 `fallbackArchiveToGallery` 调用；
   - 统一 `g.fail(err.Error())` / `g.failOrLock(...)`（配额/限流仍走 `error_lock` 与自动解锁）；
   - zip 校验失败重试逻辑（`retryResolveArchiveDownloadURL`）保留，失败同样不降级。
5. 设置项 `autoUpdateFallbackToGallery`：
   - 后端 `models.DownloadSetting` **保留字段**（避免列删除与 JSON 兼容问题），但**不再被任何代码读取**；
   - `GetSettings` 对旧库该字段为 1 时按 0 归一（避免 UI 误显示）；
   - 前端 `src/stores/downloadSettings.ts` 移除该 key（`SETTING_KEYS` 与类型），`UpdateScanSettings.vue` 删除对应开关条目；旧 localStorage 值由 `pickSetting` 白名单自然过滤。

### 3.5 日志分类修正（`log_store.go`）

改为**标签位置化解析**：

```go
func classifyLogLine(line string) LogCategory {
    tags := parseBracketTags(line)        // 按出现顺序提取 [xxx]
    // ① 显式分类行（LogStore.Printf）：首标签即分类
    if len(tags) > 0 && isCategoryTag(tags[0]) { return tagToCategory(tags[0]) }
    // ② 标准 log 行：首标签为级别（DOWNLOAD / DOWNLOAD-WARN / ARCHIVER ...），内标签从第二个开始
    for _, t := range tags[1:] {
        if c, ok := innerTagCategory(t); ok { return c }   // update / maintain / scan / tagm / offline / ...
    }
    // ③ 按级别与下载类标签
    for _, t := range tags {
        if isDownloadTag(t) { return LogDownload }
    }
    return LogOther
}
```

- `innerTagCategory` 只认白名单内的精确内标签（`update`、`maintain`、`scan`、`tagm`、`offline`、`client-log`、`Toplist`、`TagEngine`），不做子串匹配；
- 消息正文里出现 `[update]` 字样不再影响分类；
- 单测 `log_store_test.go` 增加：正文含 `[update]` 的下载日志 → `LogDownload`；`[DOWNLOAD-WARN] [archive-fallback]` → `LogDownload`。

### 3.6 前端（最小改动）

| 文件 | 改动 |
|------|------|
| `src/stores/downloadSettings.ts` | 删除 `autoUpdateFallbackToGallery` 类型字段与 `SETTING_KEYS` 项 |
| `src/components/settings/UpdateScanSettings.vue` | 删除「无 H@H 时自动降级为画廊下载」开关块（`:91-100`） |
| `src/views/DownloadsView.vue` | 可选：新增「全部暂停 / 全部恢复 / 立即调度」按钮（见 3.7） |

### 3.7 可选增强（本轮统一评估，默认可砍）

- 后端 `POST /api/v1/downloads/pause-all`、`/resume-all`、`/reschedule`（均仅管理员）：
  - `reschedule`：把所有 `queued` 任务重新压入 pending 并 `Wake`（`RestoreTasks` 的轻量版）；
  - 前端下载页顶部批量按钮，作为运维自救手段。
- 若时间紧，本轮仅做 `reschedule`（后端 + 一个按钮），另两个后置。

---

## 四、改动清单（文件级）

| # | 文件 | 动作 |
|---|------|------|
| 1 | `backend/internal/services/download_queue.go`（新增） | 无界 pending 队列 + 排序 + cond 信号 |
| 2 | `backend/internal/services/download_scheduler.go` | 重写：`schedulerLoop` / `pickNext` / `tryReserve` / `releaseReservation` / 保留抢占 |
| 3 | `backend/internal/services/download.go` | 删 `queue/workers/worker()`、`acquireSlot` 调用与无条件 `defer releaseSlot`；`Enqueue` 改入 pending；删两个 fallback 函数与调用；`runTask` 拆为 `executeTask` + 收尾函数 |
| 4 | `backend/internal/services/archive_thread_pool.go` | 增 `tryArchiveSlot` / `tryAcquirePartial`（非阻塞）；阻塞版移除或仅测试用 |
| 5 | `backend/internal/services/download_archive.go` | 删引擎内额度阻塞申请；`errHathdlOnly` → 错误分类 + 退避重试；删降级调用 |
| 6 | `backend/internal/services/download_gallery.go` | 删 `acquirePartial` 阻塞申请，改用预留线程数 |
| 7 | `backend/internal/services/log_store.go` | 标签位置化分类 |
| 8 | `backend/internal/models/download.go` | 注释标记 `AutoUpdateFallbackToGallery` 废弃（字段保留） |
| 9 | `backend/internal/services/download.go`（GetSettings） | 旧库降级开关归一为 0 |
| 10 | `src/stores/downloadSettings.ts`、`src/components/settings/UpdateScanSettings.vue` | 删开关 |
| 11 | `backend/internal/services/log_store_test.go` | 新增分类用例 |
| 12 | `backend/internal/services/download_queue_test.go`（新增） | 队列单测 |
| 13 | `backend/internal/services/download_scheduler_test.go` | 适配新结构（`runningTasks` 由调度器维护）、新增并发窗口/优先级/批量回归 |
| 14 | `backend/internal/services/download_nofallback_test.go`（新增） | 归档失败不降级回归 |

---

## 五、实施步骤（分步提交，各自可回滚）

1. **Step 1 — Bug1 修复（独立可发布）**
   删降级路径 + 错误分类 + 退避重试 + 日志标签修正 + 前端删开关 + 单测。
   验证：`cd backend && go test ./...`；`npm run type-check`。
   提交：`fix(download): 归档失败不再自动降级为画廊并修正日志分类`。

2. **Step 2 — Queue 层**
   新增 `download_queue.go` + 单测（无界、幂等、排序、移除）。
   提交：`feat(download): 新增无界待调度队列`。

3. **Step 3 — 调度器切换**
   `schedulerLoop` 替换 worker 池；资源预留前移；引擎改用预留额度；`Enqueue/Restore/Pause/Cancel/Retry/Unlock/抢占` 全部改接 pending。
   提交：`refactor(download): worker 池改为无界队列统一调度器`。

4. **Step 4 — 测试与回归**
   单测：批量 1000 个不丢、并发窗口 1、优先级抢占、暂停→恢复、重启恢复、串行/并行模式。
   实机：Y 盘部署后观察 `download_log` 无「入队失败」；704 个存量 queued 自动开始推进。
   提交：`test(download): 补队列与调度回归用例`。

5. **Step 5 — 文档与收尾**
   更新 `CHANGELOG.md`（如适用）与本计划状态；`git push`。

---

## 六、验收标准

1. **不丢任务**：批量创建 1000 个任务后，全部处于 pending 或 running（日志无「入队失败（队列已满）」）；进程重启后仍全部推进。
2. **不占线程**：`MaxArchiveConcurrency=1` 时只有 1 个任务 `downloading`，其余 `queued`；每完成 1 个，1s 内有下一个自动补位。
3. **可中断**：任意任务暂停 / 取消 / 被抢占 / 重试后都能回到队列或被正确终止，无重复执行、无槽位泄漏（`runningTotal` 与 `archivePool.active` 在任务结束后归零）。
4. **绝不降级**：模拟 H@H 直链超时 / EOF → 任务 `error`、`mode` 仍 `archive`、不生成 `<gid> - <标题>` 画廊目录、`download_scheme` 不写 `gallery`；错误文案含「可重试」。
5. **日志分类**：普通下载任务不再出现在 `update_log`；`[update]` 仅出现在真实更新检测 / 自动更新 / 更新标记清理日志。
6. **质量门**：`cd backend && go test ./...` 全绿；`npm run type-check` 通过；生产构建可启动并自动恢复 704 个存量任务。

---

## 七、风险与回滚

| 风险 | 处置 |
|------|------|
| 调度器重构触及核心路径 | 分两个 commit（Bug1 / Bug2），Step 2-3 合并前用单测覆盖；出问题按 commit revert |
| 预留额度与实际下载线程不一致（线程调大/调小） | 保留 `archivePool.adjust`，`SaveSettings` 变更后同步运行中任务并 `Wake` |
| 调度器唤醒丢失导致停滞 | 1s 兜底 ticker + `Peek` 时回查 DB 状态自愈 |
| 旧库 704 个 queued 任务一次性涌入 | 无界队列天然支持；调度器按窗口节流，首次启动观察日志即可 |
| 死锁 | 固定锁顺序 `schedMu → archivePool.mu`；引擎侧不回调调度器锁 |
| 回滚后旧数据被新语义「提前完成」 | 本次不改状态枚举与引擎内核，回滚仅需 revert 队列相关 commit |

## 八、待确认事项（默认按此执行，可否决）

1. `autoUpdateFallbackToGallery` 字段**保留但废弃**（推荐，避免 GORM 删列与 JSON 契约变动）。
2. 归档失败默认文案带「可重试」提示，不加自动重试按钮（用户手动「重试」即可）。
3. 3.7 的批量操作接口：本轮只做 `reschedule`（重新调度），`pause-all/resume-all` 后置。
