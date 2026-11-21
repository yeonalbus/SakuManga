# Round53：删除「额外的画廊扫描路径」必须连带清理该路径下的漫画

> 起因：珱垣报告「删除『额外的画廊扫描路径』后，路径下的漫画还残留在程序中」。
> 本文记录根因、修复方案与存量数据处理步骤。**已在代码侧落地并通过测试；线上存量清理待珱垣备份后执行。**

---

## 1. 现象与根因

删除设置页的扫描路径后，离线库中该路径下的漫画仍然存在（可搜索、可打开），来源标签从原来的自定义名称变成「下载」。

根因链：

1. **删除接口只删路径配置**：`handlers.DeleteScanPath` 原本仅执行
   `DELETE FROM extra_scan_paths WHERE id = ?`，`offline_comics` 中
   `ScanPathID = <该路径 ID>` 的记录一条都没处理（无事务、无级联、无计数）。
2. **离线列表全量返回**：`handlers.GetOfflineComics` 直接 `Find(&comics)` 取全表，
   只用 `ScanPathID` 反查路径名打标签——查不到也照常返回，因此残留漫画仍完整出现在离线库。

同一根因派生的连带故障：

| # | 位置 | 后果 |
|---|------|------|
| 1 | `handlers/comic.go` 来源标签 fallback | 路径删除后查不到 `Name` → 标签退化成「下载」，把额外路径漫画误标为下载导入 |
| 2 | `services/offline.go` `filterOfflineUpdateEnabled` / `filterOfflineLocalCandidates` | 候选集只按「现存路径中关闭离线维护的 ID」过滤；路径删除后其 ID 不在 `disabled` 集合 → 若原路径曾关闭离线维护，删除路径反而让这些孤儿**重新纳入联网更新检测与本地查重** |
| 3 | `services/scanner.go` 增量扫描 | 遇 `local_path` 已存在直接 return，不回填 `ScanPathID`；重新添加同一路径（新时间戳 ID）后增量扫描**无法自愈**，只有全量扫描 Save 覆盖才修正 |
| 4 | `services/scan_manager.go` | 删除时不停正在进行的扫描 goroutine，会继续以旧路径 ID 落库 |

线上实例（`Y:\SakuManga\manga.db`）实测：`extra_scan_paths` 仅剩 1 条（`Z:\Comics`），
而 `offline_comics` 中 `scan_path_id = path-1795254030497579500`（路径记录已不存在）
的残留漫画 **349 本**，均位于 `Z:\AI_Wife\AIDraw\Gallery\Posted\...`。

---

## 2. 已定决策（珱垣确认）

| 决策点 | 结论 |
|--------|------|
| 删除路径时漫画记录 | **级联删除记录，保留磁盘文件**（撤出来源 ≠ 销毁文件） |
| 存量 349 本孤儿 | 提供一次性清理工具，须先备份 `manga.db` 后执行 |
| 前端交互 | **加二次确认弹窗，显示该路径下已入库的本数** |
| 扫描中删除 | 拒绝（409），等扫描完成后再删 |

---

## 3. 改动清单

### 3.1 服务层

- **`services.DeleteScanPathWithComics(db, pathID)`**（新增，`offline.go`）——删除流程唯一入口：
  1. 路径记录不存在 → `(0, nil)` 幂等，**且不按任意 id 清理漫画**（否则伪造 id 即成为按
     `scan_path_id` 批量删库的后门）；
  2. 该路径正在扫描（`ScanManager.IsScanning`）→ `ErrScanPathInProgress`；
  3. 先清理漫画（失败则整体失败、路径保留可重试），再删路径记录。
- **`services.DeleteOfflineComicsByScanPath(db, pathID)`**（新增）——成百上千本必须的批量实现：
  - 一条 SQL 批量删除；引用清理走批量版；XP 统计走一次异步全量重建（`XpRebuildAsync`），
    而非逐本全用户差分；
  - 同 `gid` 的其他记录（其他路径 / 下载导入）作为引用迁移目标，避免误清用户历史/书架引用。
- **`services.CleanupComicReferencesBatch(db, oldIDs, migrate)`**（新增，`comic_refs.go`）——
  原单本版改为委托批量版（单一实现，避免两份逻辑漂移）：书架与阅读清单各只全表扫一次。
- **`ScanManager.IsScanning(pathID)`**（新增）——扫描态判定。

### 3.2 HTTP 层

- `DELETE /api/v1/admin/scan-paths/:id` 改为薄映射：成功 `{message, removedComics}`，
  扫描中 `409`，失败 `500`（带已完成清理数量）。
- 新增 `GET /api/v1/scan-paths/:id/impact` → `{comicCount}`（只读，前端二次确认用）。

### 3.3 前端

- `stores/scanPathStore.ts`：新增 `getScanPathImpact(id)`；`removeScanPath(id)` 改为
  **先请求成功再改本地列表**并返回被清理的本数（失败抛错，避免本地与后端不一致）。
- `components/settings/ExtraScanPathsSettings.vue`：删除按钮改为二次确认
  （`modal.confirm`，`tone: danger`，正文显示「该路径下已入库的 N 本漫画将一并从离线库移除」，
  details 说明磁盘文件保留）；成功 toast 清理数量并 `fetchOfflineComics()` 同步离线库内存列表；
  删除中按钮 disabled 防连点。

### 3.4 一次性清理工具

`backend/cmd_debug/cleanup_orphan_scanpaths`：

```bash
cd backend

# 只读统计（不动库）
go run ./cmd_debug/cleanup_orphan_scanpaths -db Y:\SakuManga\manga.db

# 真正执行（先备份 + 停线上进程）
go run ./cmd_debug/cleanup_orphan_scanpaths -db Y:\SakuManga\manga.db -apply
```

- 只处理「`scan_path_id` 非空且路径表中不存在」的记录；路径仍存在的漫画不动；磁盘文件保留。
- 引用清理复用线上同一函数；执行后把 `xp_meta.dirty` 置 1，
  线上进程下次查询词云时自动全量重建，清掉已删本的贡献。

---

## 4. 验证

- 新增单测（全部通过）：
  - `services.TestDeleteOfflineComicsByScanPathRemovesRecords`：只删目标路径、引用迁移与剔除、
    书架封面回退、重复调用幂等；
  - `services.TestDeleteOfflineComicsByScanPathGuards`：空/空白 id 与 nil db 不得删除任何记录；
  - `services.TestDeleteScanPathWithComicsRejectsWhileScanning`：扫描中拒绝且零删除，结束后可删；
  - `services.TestScanManagerIsScanning`：未启动 / 进行中 / 已完成；
  - `handlers.TestDeleteScanPathCascadesComics`：impact 计数 + 级联删除 + 范围隔离 + 幂等；
  - `handlers.TestDeleteScanPathGhostIdKeepsComics`：路径不存在时不得按该 id 批量删库。
- `cd backend && go test ./...` 全绿；`npm run type-check` 通过；改动文件 eslint 无告警。
- 线上库只读 dry-run：识别孤儿来源 1 组、349 本，样例路径均在 `Z:\AI_Wife\AIDraw\Gallery\Posted`。

---

## 5. 存量数据处理步骤（待珱垣操作）

1. 备份 `Y:\SakuManga\manga.db`；
2. 停止线上进程（避免 SQLite 写锁冲突）；
3. 执行 dry-run 核对数量（应为 349 本 / 1 组）；
4. 执行 `-apply`；
5. 重启线上进程；离线库刷新后该批漫画消失，`Z:\AI_Wife\AIDraw\Gallery\Posted` 下磁盘文件仍在；
6. 若需重新纳入：在设置中重新添加该路径并**全量扫描**（增量模式不会回填 `ScanPathID`）。

---

## 6. 范围外记录

- 设置页「移除路径」按钮对非管理员也显示（点击会被后端 403）——既有问题，未在本次改动。
- 仓库工作区存在 `backend/webui/dist/**` 与 `YeonPlans/**` 的大量未暂存删除
  （非本次改动产生），本次提交未触碰，需珱垣确认后处理。
