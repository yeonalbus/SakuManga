# Round56 本地失效本子双轨生命周期 & 个人推荐与排行榜算法优化方案

> 状态：已实施（珱垣确认并验收，2026-11-25）
> 提交记录：`f916ae4`（双轨失效生命周期）、`9364200`（偏好信号、综合榜与推荐算法优化）
> 关联前序：`round54-recovery-fallen-plan.md`（恢复与阵亡基础功能）、`round32-xp-cloud-recommend-plan.md`（XP 词云与本地推荐初版）

---

## 一、任务一：本地失效本子双轨生命周期（修复裂图残留 Bug）

### 1.1 背景与根因

Round54 实现了「全量扫描失效对账 + 恢复 + 阵亡」，但当时约束对账阶段「绝不修改 `offline_comics`」，同时 `recordRecoveryFallen` 也仅写入 `fallen_comics` 而未删除对应 `offline_comics` 记录。这导致：

1. 本地本子被删除后，仍继续出现在**离线首页、书架、排行榜、本地浏览历史、在线页「已下载」/本地优先、随机抽卡、XP 词云、维护查重/更新检测/画质升级**中，封面加载 404 后显示为裂图占位卡。
2. 若简单粗暴「一发现缺失就物理删除 `offline_comics` 并清引用」，会把**待恢复候选**的书架归属/排序、阅读历史进度、个人评分全部抹掉，恢复完成后 `finalizeUpdate` 将无引用可迁移。

### 1.2 核心设计：双轨生命周期

按失效本子的可恢复性拆为两条轨道：

| 轨道 | 触发时机 | 对 `offline_comics` 的处理 | 引用（书架/历史/评分/清单） | 磁盘文件 |
|---|---|---|---|---|
| **轨道 A：已阵亡（不可恢复）** | 对账判阵亡（`no-ehentai` / `removed_status=1` / 已在阵亡表）；恢复任务确定性失败 `recordRecoveryFallen`；`ClearRecoveryCandidates` | 校验 `!pathExistsConservative(local_path)` 后**物理删除** | 批量/单条清理并迁移到同 `g_id` 存活副本（`CleanupComicReferencesBatch` / `CleanupComicAuxiliaryBatch`）+ 清理封面/页缓存 + 异步重建 XP | **绝不触碰**（文件本已缺失） |
| **轨道 B：恢复候选（待恢复/恢复中）** | 全量扫描对账判可恢复；`ReconcileDefaultDownloadComics` | 置 `file_missing = true`（**软隐藏**，加索引） | **冻结保留**在 DB 中，等待恢复完成后由 `finalizeUpdate` 迁移到新本子 | 不触碰 |

### 1.3 关键机制

1. **全站统一过滤 `file_missing = false`**：
   - 离线列表 `GetOfflineComics`、随机抽卡/推荐 `buildOfflineQuery`、XP 词云 `Rebuild`/`meta`、更新检测/老化/查重/升级/Tag 刷新、在线「已下载」`AttachDownloadStates` / `AttachDetailDownloadState`、在线「本地优先」`preferLocal`、下载去重 `CreateTask`、替换查找 `FindReplacementByGID`。
   - **书架与历史特殊保护**：
     - `loadBookshelfStats` / `GetBookshelves`：`count`、`unreadCount`、`coverUrl` 仅统计 `file_missing = false` 的存活本，但数据库 `comic_ids` 保留冻结 ID；`ReorderBookshelfComics` 全量模式重排时自动保留原书架中的 `file_missing = true` ID，避免重排丢失冻结引用。
     - `GetHistory(source=offline)`：通过 `NOT EXISTS` 过滤掉 `file_missing = true` 的记录。
2. **自愈与扫描恢复**：
   - 对账发现文件重新存在时，`file_missing` 还原为 `false` 并清除候选/未 `dismissed` 阵亡；`scanner.go` 扫描到已存在记录时同样清除 `file_missing`。
3. **启动期幂等校准（`SyncRecoveryAndFallenOnStartup`）**：
   - 服务启动时（`main.go`）自动把存量 `recovery_candidates` 对应的 `offline_comics` 置 `file_missing = true`，并将存量 `fallen_comics` 仍残留的失效 `offline_comics` 物理清理（二次 `os.Stat` 确认缺失），确保恢复锁定期间升级重启即可立即生效。
4. **新增边界入口**：
   - **清空候选**：`POST /api/v1/offline/recovery/clear-candidates`（恢复进行中返回 409），将全部候选转为阵亡（`reason = abandoned`）并彻底清理对应的失效记录与引用。前端入口在 `ExtraScanPathsSettings.vue`「恢复」按钮右侧。
   - **默认下载目录失效检测**：`POST /api/v1/offline/recovery/reconcile-default`，对 `scan_path_id = ''` 的下载导入本子执行双轨对账。前端入口在 `DownloadSettings.vue`「下载路径」分组下。

---

## 二、任务二：个人偏好信号、综合偏好榜与推荐算法优化

### 2.1 背景与问题

1. **评分 Bug**：原 `xp_cloud.go` 将个人评分按 `1~10` 分制计算 `(score - 1) / 9`，而实际 `comic_ratings` 为 **1~5 星**，导致 5★ 满分权重被腰斩，且 1~2★ 低分雷作仍能拿正权重并被继续推荐。
2. **信号单一与漏接**：原计算未纳入**书架收藏**与**阅读完读深度**；本地排行榜仅按全库 `readCount` 降序，导致测试本或老本子长期霸榜，第 7 名以后大量并列。
3. **覆盖率误报**：XP 词云原采用 `readSignalComics / totalComics < 5%` 触发「阅读信号稀疏」提醒，在 7000~9000+ 本大库藏下门槛高达 350~450+ 本，与个人真实阅读节奏不匹配。

### 2.2 多信号融合公式（`XpFormulaVersion = 3`）

- **低分排除红线**：仅采信 `comic_ratings` 中用户自己打出的 **1~2 星**（**绝不使用 E 站社区评分 `community_rating`**）。个人 1~2 星本在 XP 词云中阅读侧权重恒为 `0`，在**本地排行榜**与**偏好推荐**中彻底排除。
- **单本阅读侧综合偏好分 `readWeightOf`**：
  $$r(c) = 0.45 \cdot \min\left(1, \frac{\ln(1 + \text{readCount})}{\ln(1 + 10)}\right) + 0.30 \cdot \mathbb{I}_{\text{inShelf}}(c) + w_{\text{rating}}(c) + 0.20 \cdot \text{progress}(c) + 0.15 \cdot e^{-\Delta\text{days}/90}$$
  - **阅读次数（0.45）**：在 10 次左右趋近饱和，避免单本误点刷屏淹没整体偏好。
  - **书架收藏（0.30）**：被当前用户加入任意书架即生效；书架增删/删架时联动 `XpRecomposeComicForUser`。
  - **个人星级**：5★ `+0.35`、4★ `+0.175`、3★ `0`、1~2★ 排除。
  - **完读深度（0.20）**：复用 `history_records` 的 `(last_page_index + 1) / total_page_count`（上限 1），区分「真看完」与「点开即关」。
  - **近期性（0.15）**：90 天半衰期指数衰减。
- **词云覆盖率提示口径**：改按绝对有效偏好样本数判定：`< 20` 建议以库藏为主、`20 ~ 49` 提示正在积累、`≥ 50` 样本充分不再提示警告。

### 2.3 本地个人综合偏好榜（TOP 25）

- 新增后端接口 `GET /api/v1/offline/toplist?limit=25`（`XpCloudService.PersonalToplist`），按当前登录用户隔离计算。
- 直接复用多信号偏好分排序（同分依次按 `readCount`、`lastReadAt`、`comicId` 决胜），排除个人 1~2 星与 `file_missing = true` 本子。
- 前端 `OfflineToplist.vue` 去掉「阅读频次榜」与切换开关，直接展示「本地个人综合偏好榜 (TOP 25)」，卡片下方显示简洁信号摘要（如 `15 次阅读 · ★5 · 书架`）。

### 2.4 本地偏好推荐增强（`RecommendService`）

1. **个人 1~2 星彻底排除**：仅过滤 `comic_ratings` 中的 1~2★，不采信社区评分。
2. **近期已读冷却**：近 **7 天内读过**（`history_records.last_read_at`）的本子打分乘以冷却系数 `0.35`，减少刚看完又抽到的重复感。
3. **冷启动自适应 $\theta$**：当调用方未显式传入 `recoTheta` 且用户有效偏好样本 `< 50` 本时，自动在 `0.20 ~ 0.60` 之间按 `样本数 / 50` 线性插值，低样本阶段自动借力库藏侧权重。

---

## 三、涉及文件与验证记录

### 3.1 主要改动文件
- **后端模型/入口**：`backend/internal/models/models.go`、`backend/internal/models/recovery.go`、`backend/main.go`、`backend/internal/router/router.go`
- **后端服务**：`backend/internal/services/recovery.go`、`backend/internal/services/scanner.go`、`backend/internal/services/xp_cloud.go`、`backend/internal/services/recommend.go`、`backend/internal/services/favorites.go`、`backend/internal/services/download.go`、`backend/internal/services/comic_refs.go`、`backend/internal/services/offline.go`、`backend/internal/services/offline_task.go`、`backend/internal/services/ignore.go`、`backend/internal/services/upgrade.go`、`backend/internal/services/tag_maintain.go`
- **后端处理器**：`backend/internal/handlers/recovery.go`、`backend/internal/handlers/comic.go`、`backend/internal/handlers/library.go`、`backend/internal/handlers/online.go`、`backend/internal/handlers/random.go`、`backend/internal/handlers/offline.go`、`backend/internal/handlers/xp_cloud.go`
- **前端**：`src/stores/scanPathStore.ts`、`src/stores/bookshelfStore.ts`、`src/api/xpCloud.ts`、`src/components/settings/ExtraScanPathsSettings.vue`、`src/components/settings/FallenMenu.vue`、`src/components/settings/DownloadSettings.vue`、`src/views/offline/OfflineToplist.vue`（及同步 `backend/webui/dist/`）
- **单测**：`backend/internal/services/recovery_test.go`、`backend/internal/handlers/recovery_test.go`、`backend/internal/handlers/library_test.go`、`backend/internal/services/xp_cloud_test.go`、`backend/internal/services/recommend_test.go`

### 3.2 验证记录
- `cd backend && go test ./...`：全部测试包通过（`internal/handlers`、`internal/services` 均通过）。
- `npm run type-check` & `npm run build-only`：均通过，构建产物已同步至 `backend/webui/dist`。

### 3.3 后续发版参考要点（`CHANGELOG` / `BRIEF` 素材）
- **Bug 修复**：修复本地本子被删除后仍在首页/书架/排行榜/历史等位置显示裂图的问题，区分「可恢复软隐藏保引用」与「已阵亡彻底清理」，并支持启动自动校准。
- **新功能**：新增「清空恢复候选（转阵亡并删记录）」按钮与「默认下载目录失效检测与清理」入口。
- **算法优化**：升级个人偏好计算模型（融合阅读次数饱和、书架收藏、修正后 1~5★ 评分、完读深度与近期性；排除个人 1~2★）；本地排行榜升级为「个人综合偏好榜 (TOP 25)」；优化 XP 词云样本提示与推荐冷启动/近期冷却策略。
