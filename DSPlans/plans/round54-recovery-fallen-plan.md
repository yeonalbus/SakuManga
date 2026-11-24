# Round54 恢复与阵亡功能设计文档

> 状态：设计定稿（珱垣 2026-11-24 确认）
> 依据：2026-11-24 Z:\Comics 误删事故调查 + 珱垣与 AI 多轮技术讨论
> 实现方式：分阶段执行，每阶段完成后向珱垣报告，确认后推进下一阶段（见第 18 节）

---

## 1. 背景与目标

2026-11-24，珱垣误删 `Z:\Comics`（SakuManga 额外扫描路径，JHentai 时代老库），约 2601 本本子丢失。SakuManga 数据库 `offline_comics` 完整保留了这些本子的 E 站元数据（g_id / token / 标题 / 页数等）。

目标：基于数据库实现「依据数据库的恢复功能」，本次先抢救数据，并沉淀为长期功能：
1. **恢复**：全量扫描发现失效文件后，对可恢复本子一键重下（复用现有下载管线），落地 SakuManga 下载路径，自动更新记录。
2. **阵亡**：记录无法恢复的本子（EH 版权/下架、无法解锁、非 E 站来源、路径已移除），形成全局死亡名单。
3. 恢复与阵亡联动：恢复完成自动剔除阵亡条目；扫描在恢复期间按路径锁定。

## 2. 数据现状（2026-11-24 快照）

| 项目 | 值 |
|---|---|
| `offline_comics` 总记录 | 9195 |
| 被删库记录（`scan_path_id=path-1786004568961504400` → `Z:\Comics`） | 2601 |
| 其中带 g_id + token | 2601 / 2601（100%） |
| 总体积 | 797.67 GB（Image Set 97 本 350.79 GB；Doujinshi 1435 本 166.92 GB；Artist CG 842 本 230.02 GB；Manga 164 本 19.7 GB） |
| 已下架（removed_status=1） | 3 条（必阵亡） |
| 另一批记录 | `Z:\SakuHentai\Comics` 6594 条，完好 |
| 下载落地设置 | archive_path=`Z:\SakuHentai\Zip`，extract_path=`Z:\SakuHentai\Comics` |
| 服务状态 | 下载管理器 `RestoreTasks` 会在启动时把 DB 中 queued/downloading 任务重新入队（auto_resume_tasks=1） |

## 3. 术语与核心原则

- **g_id 是狗牌（唯一身份）**：恢复、阵亡、去重、剔除全部以 g_id 为准，comic_id 仅作关联辅助（更新任务会让 comic_id 变化）。
- **恢复** = 用数据库中的 g_id/token 重新下载并入库，等价于现有「更新任务」语义（`UpdateForComicID != ""`）。
- **阵亡** = 无法/不再恢复的本子，进入全局名单持久展示。
- **候选** = 全量扫描对账产出的「可恢复」失效清单，恢复按钮的数据源。
- **dismissed** = 用户手动 x 掉的阵亡条目（划线 + 置底，保留显示），toggle 可撤销。

## 4. 总体流程

```
用户点「全量」扫描路径
  → 扫描目录入库（现有逻辑）
  → 【新增】对账阶段（仅全量）：
      · 根路径不存在 → 整条路径记录全部判失效
      · 逐条 os.Stat(local_path)
      · 失效且 g_id/token 非空且 removed_status=0 → recovery_candidates
      · 失效且无 g_id/token / removed_status=1 → fallen_comics（no-ehentai / removed）
      · 有效但曾在候选表 → 移除候选（自愈）
  → 对账结果：候选非空 → 「恢复」可点；阵亡非空 → 「阵亡」可点

用户点「恢复」
  → 弹窗选档位（H@H 原图 / H@H 压缩 / 画廊逐图）
  → POST /offline/recovery/start
  → 逐条 CreateTask(mode/archive_type, update_for_comic_id=原记录ID, group="recovery")
  → 统一调度器按并发限制下载

恢复任务完成 → finalizeUpdate：
  → 防御校验（按 g_id 确认新记录已存在且 local_path 有效）
  → 删除旧记录（DeleteOfflineComic 迁移历史/书架/评分引用）
  → 删除候选行（g_id）
  → 删除阵亡行（g_id）
恢复任务确定性失败 → failTask：
  → 写 fallen_comics（removed / unlock-failed）
  → 任务置 cancelled（不再占用「恢复中」集合）
恢复任务网络/配额失败 → 保持 error / error_lock，留在恢复队列可重试

用户点「阵亡」
  → 下拉菜单：分组显示（EH 版权/下架、无法恢复）
  → x 按钮 toggle dismissed：划线 + 置底（保留显示）
  → 恢复完成自动物理剔除
```

## 5. 数据库设计

### 5.1 recovery_candidates（恢复候选，自包含）

| 字段 | 类型 | 说明 |
|---|---|---|
| gid | string PK | 狗牌 |
| token | string | E 站 token |
| title / title_jpn | string | 快照 |
| cover_url | string | 快照 |
| category | string | 快照 |
| page_count / file_size | int / int64 | 快照 |
| local_path | string | 失效原路径（快照） |
| scan_path_id | string | 原属扫描路径（锁推导用） |
| comic_id | string | 原 offline_comics 记录 ID（删除时机关联） |
| created_at / updated_at | datetime | |

唯一索引 `gid`；索引 `scan_path_id`。**自包含**：即使 offline_comics 记录被删（移除路径），候选仍可支撑恢复。

### 5.2 fallen_comics（阵亡名单，全局）

| 字段 | 类型 | 说明 |
|---|---|---|
| id | int PK auto | |
| dedupe_key | string uniqueIndex | 显式去重键：有 g_id → `gid:<gid>`；无 g_id → `cid:<comic_id>`（SQLite 唯一索引对 NULL 不去重，no-ehentai 条目没有 g_id，必须显式键保证 upsert 幂等） |
| gid | string index | 狗牌（可空：no-ehentai 条目为空） |
| comic_id | string | 原记录 ID（可空） |
| title / title_jpn / cover_url / category | string | 快照，不依赖记录 |
| file_size / page_count | int64 / int | 快照 |
| reason | string | `removed` / `unlock-failed` / `no-ehentai` / `path-removed` |
| source_path | string | 来源路径快照 |
| dismissed | bool default false | 用户 x |
| dismissed_at | datetime nullable | 置底排序依据 |
| created_at / updated_at | datetime | |

**upsert 规则**：全量对账按 g_id upsert，保留 dismissed / dismissed_at；恢复完成按 g_id 物理删除。

### 5.3 状态推导（不新增状态表，避免漂移）

- **恢复中**：`EXISTS(download_tasks WHERE group='recovery' AND status IN ('queued','downloading','paused','error','error_lock'))`
- **锁定路径集合**：`SELECT DISTINCT c.scan_path_id FROM recovery_candidates c WHERE EXISTS(SELECT 1 FROM download_tasks t WHERE t.group='recovery' AND t.g_id=c.gid AND t.status IN (...))`
- 恢复按钮可点：候选非空且无恢复中任务。
- 阵亡按钮可点：全局阵亡名单非空。
- 恢复全部终结后：候选已清（成功）或转阵亡（确定性失败），自然回到 idle/ready。
- **不新增 recovery_state 表**：全部由查询推导，重启后状态自动恢复（与 RestoreTasks 语义一致）。

## 6. 判定规则

### 6.1 失效判定
`os.Stat(local_path)` 返回 `IsNotExist`（含父目录不存在）→ 失效。其他错误（权限/网络抖动）不判失效（保守）。

### 6.2 可恢复判定（进候选）
失效 且 `g_id != ''` 且 `token != ''` 且 `removed_status = false`。老化（aged_status）不影响判定。

### 6.3 阵亡分类
| reason | 场景 |
|---|---|
| `removed` | removed_status=1（EH 已下架/版权），或恢复任务 404/removed |
| `unlock-failed` | 恢复任务解锁失败（密码压缩包无法解锁） |
| `no-ehentai` | 失效且无 g_id/token（非 E 站来源） |
| `path-removed` | 用户移除路径时，未恢复的候选转写 |

### 6.4 恢复失败分类
- **确定性失败**（404 / removed / copyright / unlock-failed）→ 阵亡 + 任务 cancel。
- **可重试失败**（EOF/超时/连接重置/代理组失效/配额锁定）→ 保持 error/error_lock，留在恢复队列，可手动/自动重试。

## 7. 扫描对账阶段设计

### 7.1 触发与范围
- 仅在**全量扫描**结束后执行（增量扫描不做对账，维持现有语义）。
- 扫描路径根目录不存在时：扫描遍历返回 0 本 + 标记 `pathMissing`，**继续执行对账**（整条路径记录全部判失效），不返回错误。

### 7.2 流程
1. 扫描完成后进入「对账中」阶段（`ScanProgress` 增加阶段，前端展示）。
2. 取 `SELECT * FROM offline_comics WHERE scan_path_id = ?`（建议分批 chunk 1000 读取）。
3. 并发 Stat（限 16 并发，避免 SMB 卡顿）判定存在性。
4. 失效记录分类写入：
   - 有效 → 若在候选表 → 删除候选行（自愈）；若在阵亡表且未 dismissed → 删除阵亡行（已恢复）。
   - 失效 → 按 6.2/6.3 写入候选或阵亡（upsert）。
5. 更新 ExtraScanPath 的 comic_count（现有逻辑），进度上报。

### 7.3 性能
2601 条 Stat 预计 < 10s（网络盘快速失败）。候选/阵亡表规模 ≤ 记录数，无压力。对账可 RepeatableRead 内分批写。

## 8. 恢复流程设计

### 8.1 档位映射
| UI 档位 | mode | archive_type |
|---|---|---|
| H@H 原图 | archive | original |
| H@H 压缩 | archive | resample |
| 画廊逐图 | gallery | — |

### 8.2 启动接口
`POST /api/v1/offline/recovery/start`
- 鉴权：admin（恢复涉及下载权限；普通用户无下载权限时由现有 requireDownloadPermission 兜底）。
- 入参：`{ scheme: "archive-original" | "archive-resample" | "gallery" }`
- 校验：候选非空；无进行中恢复任务（防重复点击，409）。
- 逐条 `CreateTask`：
  - `GID/Token/Title/CoverURL` ← 候选
  - `Mode/ArchiveType` ← 档位
  - `UpdateForComicID = 候选.comic_id`（若原记录已被删则传空字符串并标记，finalizeUpdate 找不到旧记录时忽略）
  - `Group = "recovery"`
  - `UserID` = 当前用户
- 返回 `{ created, skipped, failed, errors }`。
- 幂等：已在恢复队列的 g_id（存在未终结同 gid 任务）→ skip。

### 8.3 取消语义
`POST /api/v1/offline/recovery/cancel`，入参 `{ gids?: string[] }`：
- 传 gids：仅取消这些 g_id 的进行中任务（CancelTask），**候选保留**。
- 不传：取消全部恢复任务（与 BatchAction 类似），**候选全部保留**，可再次点恢复幂等重入。
- 取消完成后锁集合按推导自动收缩。

## 9. 完成/失败钩子

### 9.1 finalizeUpdate（download.go 现有函数）
在现有「删除旧记录」前追加：
1. **恢复任务判定**：`task.Group == "recovery"`。
2. **防御校验**：按 g_id 查 `offline_comics` 新记录，且 `os.Stat(local_path)` 有效 → 才允许删旧记录；否则跳过删除 + log 告警（防悬空）。
3. 修正删除条件：恢复任务**固定强制删除**旧记录（`ForceDeleteOriginal=true` 语义或直接走删除分支），不受 `AutoUpdateDeleteOriginal` 设置影响。
4. 删除候选行（g_id）、删除阵亡行（g_id）。

### 9.2 failTask（download.go 现有函数）
- `task.Group == "recovery"` 且 `UpdateForComicID != ""`：
  - 错误分类（新增 `classifyRecoveryFailure`，基于现有 archive 锁定分类 + 404/removed 判定）：
    - 确定性 → 写 fallen（removed / unlock-failed），任务置 `cancelled`。
    - 可重试 → 保持 error / error_lock。

### 9.3 现有链路顺序（已核实，天然满足删除时机）
`executeTask`：`dispatchEngine`（下载 + 同步 `ingestDownloadDir` 入库）→ `finalizeUpdate` → `postDownloadBookkeeping`。新记录先于旧记录删除存在。

## 10. 扫描锁

- 锁粒度：**按路径**。判定：存在未终结恢复任务，且其 g_id 对应候选的 `scan_path_id` == 请求扫描的路径 → 拒绝。
- 后端：`TriggerScanPath` 增加守卫，返回 409「恢复任务进行中，该路径暂不可扫描」。
- 前端：恢复中该路径卡片「全量/增量」按钮禁用（其他路径正常）。
- 有候选但未点恢复：**不锁**（可正常扫描，符合珱垣设计）。
- 扫描路径卡片「路径不存在」状态：显示「路径不存在，建议进行恢复」；**不提示移除路径**（避免用户误删导致恢复记录丢失）。

## 11. 移除路径边界（Round54 补充）

| 场景 | 行为 |
|---|---|
| 该路径恢复中（未终结恢复任务） | 拒绝移除（409），提示等待或取消恢复 |
| 有未恢复候选、无恢复任务 | 二次确认（前端弹窗说明「N 本可恢复的本子将转入阵亡名单（原因：路径已移除），之后不可再恢复」）；确认后后端把候选按 g_id upsert 转为 fallen（reason=path-removed），再走现有「删记录 + 删路径」 |
| 无候选 | 现有逻辑直接删除 |
| 阵亡名单 | **全局保留，不随路径删除**（快照字段独立于记录） |

## 12. 阵亡名单 UI 与 API

### 12.1 菜单
下拉分组显示：
- 「EH 版权 / 下架」组：reason=removed
- 「无法恢复」组：reason ∈ {unlock-failed, no-ehentai, path-removed}
条目：封面（cover_url，缓存缺失时占位图）、标题、原因、日期。
x 按钮：toggle dismissed → 划线 + 移到列表末尾；撤销恢复原位置。

### 12.2 排序
未 dismissed 按 created_at 升序在前；dismissed 按 dismissed_at 升序在后（置底）。

## 13. API 清单

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/offline/recovery/status` | 候选数量、进行中任务数、锁定路径、phase（admin） |
| GET | `/api/v1/offline/recovery/candidates` | 候选列表（分页，预览用） |
| POST | `/api/v1/offline/recovery/start` | 启动恢复（scheme） |
| POST | `/api/v1/offline/recovery/cancel` | 取消（单条/全部） |
| GET | `/api/v1/offline/fallen` | 全局阵亡名单（分页，含分组/状态） |
| POST | `/api/v1/offline/fallen/dismiss` | `{ gid, dismissed }` toggle |
| GET | `/api/v1/scan-paths` | 响应扩展：`pathMissing`、`recoveryCandidateCount` |

## 14. 前端设计

文件：`src/components/settings/ExtraScanPathsSettings.vue`（按钮区）+ `src/stores/scanPathStore.ts`（API）+ 新组件 `RecoveryDialog.vue` / `FallenMenu.vue`。

### 14.1 按钮布局（路径卡片行）
```
[包含子文件夹 toggle] [离线维护 toggle] [全量] [增量] [恢复] [阵亡] [移除路径 x]
```
- 恢复：`disabled = 候选为空 || 恢复中`；title 说明。
- 阵亡：`disabled = 全局阵亡名单为空`。
- 恢复中该路径全量/增量禁用。
- 窄屏：按钮区 flex-wrap。

### 14.2 交互
- 恢复点击 → 弹窗：显示候选 N 本、三档位单选（标注 H@H 原图=原画质/耗 GP、H@H 压缩=省流量、画廊逐图=不耗 GP 但慢）、开始按钮。
- 恢复中：卡片显示恢复进度（复用下载任务列表 group='recovery'，或专用面板轮询 status）。
- 阵亡点击 → 下拉菜单（分组 + 条目 + x toggle）。
- 取消恢复：单条取消入口在恢复进度面板；全部取消按钮带确认。

## 15. 兼容与迁移

- 三表 gorm `AutoMigrate` 自动建表，旧库升级无破坏。
- 现有字段（`download_tasks.group`、`offline_comics.*`）无变更。
- `ExtraScanPath` 模型无新增列（pathMissing 为运行时查询字段）。

## 16. 测试计划

### 16.1 后端单测（services）
- 对账：正常失效/有效判定、根路径不存在整批失效、removed/no-ehentai 分类、自愈（有效但曾在候选/阵亡）。
- 阵亡 upsert：保留 dismissed/dismissed_at。
- 恢复启动：幂等（同 gid 跳过）、档位映射、无候选 409、恢复中 409。
- 取消：单条保留候选、全部保留候选。
- 扫描锁推导：恢复中锁对应路径、其他路径不锁。
- 移除路径：恢复中 409、有候选转阵亡（path-removed）。
- failTask 分类：确定性 → 阵亡 + cancelled；网络/配额 → 保持 error。
- finalizeUpdate：防御校验（新记录不存在 → 不删旧）、成功路径删候选/阵亡/旧记录。

### 16.2 handlers 测试
- 鉴权（admin vs member）、参数校验、409 场景。

### 16.3 前端
- `npm run type-check`；Playwright（如需要）验证按钮状态/弹窗/下拉/划线置底。

### 16.4 集成（珱垣环境，人工）
- 本地 Test 实例：造临时目录 → 入库 → 删目录 → 全量扫描 → 出候选 → 恢复 1 本真实下载 → 验证 ingest/旧记录删除/阵亡剔除/引用迁移。

## 17. 风险与注意

1. 797GB 下载耗时/GP 消耗：分批自然由调度器处理；恢复期间勿重启（重启后 RestoreTasks 会捞回 queued/downloading，无损失）。
2. SQLite delete 日志模式：对账/候选写库在服务运行中执行，注意锁竞争（短事务、批量 chunk 写）。
3. 扫描路径根目录不存在时对账依赖记录中的 local_path 失效判定（不依赖遍历），设计已覆盖。
4. 恢复任务失败分类依赖错误文本分类，需覆盖现有 archive 锁定分类的全部文案。
5. 恢复期间若用户移除路径被拒，需明确提示，避免困惑。

## 18. 分阶段实施计划（每阶段完成即报告，珱垣确认后推进）

| 阶段 | 内容 | 验证 |
|---|---|---|
| 1 | 数据层：三表模型 + AutoMigrate + 对账算法（失效判定/候选/阵亡写入/自愈/根路径不存在）+ 扫描集成（对账阶段、进度）+ 单测 | `go test ./internal/services/...` |
| 2 | 恢复后端：状态推导、候选列表、启动/取消接口、批量 CreateTask、扫描锁守卫、移除路径边界 | 单测 + handlers 测试 |
| 3 | 钩子：finalizeUpdate 防御校验与清理、failTask 分类入阵亡、阵亡 API（列表/dismiss） | 单测 |
| 4 | 前端：按钮布局、档位弹窗、恢复进度、阵亡下拉、x 划线置底、路径不存在提示、锁禁用 | `npm run type-check` |
| 5 | 全量测试 + 集成验证 + 文档更新（CHANGELOG 待发版时） | `go test ./...` + 集成 |

> 恢复 2601 本的真实下载由珱垣在线上手动触发（需要 E 站账号/代理/GP），AI 不代跑。
