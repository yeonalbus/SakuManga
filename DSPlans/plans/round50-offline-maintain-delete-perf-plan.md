# Round50：维护界面删除交互与查重口径修复方案

> 起因：用户反馈「维护界面点删除后要等很久、等待期间无法操作、操作结束后序列变化、只能单个删除」。
> 调研见 `DSPlans/Research/维护界面删除交互调研.md`（含全部量化证据与测量脚本）。
> 本文是施工方案：已定决策、分批步骤、影响面、验证方式。**未实施前不要当作完成记录。**

---

## 1. 目标

| # | 目标 | 验收口径 |
|---|------|----------|
| 1 | 删除操作从「秒级」降到「毫秒级」 | 单本删除端到端 < 200 ms（不含物理删文件） |
| 2 | 删除后列表不重排、不重编号 | 删除前后同组位置与 `cluster.id` 不变；仅被删成员消失／不足 2 本的组消失 |
| 3 | 老化本重新参与本地查重 | 同一份库：疑似重复组数从 41 组回到 288 组量级 |
| 4 | 支持批量删除 | 一次确认、一次请求、一次提交多个 `comicIds` |
| 5 | 结果响应体积下降 | 923 KB → ≈561 KB（只删未引用字段） |

---

## 2. 已定决策（2026-11-01 珱垣确认）

| 方向 | 结论 | 说明 |
|------|------|------|
| G 口径修复 | ✅ 做 | 老化只影响「联网核对」；本地查重（同 GID / hash / 目录签名 / 名称级）必须纳入老化本 |
| A 删除不重算 | ✅ 做 | 删除不可能产生新簇（已实验证明），改为增量裁剪 |
| B 排序/ID 稳定 | ✅ 做 | 稳定排序键 + 内容稳定 ID |
| C 前端乐观更新 | ⚠️ 视 A 结果 | A 落地后删除 ~50 ms，**不做乐观更新**；仅保留「全局锁 → per-item 状态」的修复 |
| D 批量删除 | ✅ 做 | 复用后端已有 `comicIds` 能力 |
| E 结果瘦身 | ✅ 做（仅做法 1） | 只删未引用字段；tag 判定搬后端（做法 2）留待视觉改造批次 |
| F 过渡动画 | ❌ 不做 | 视觉混乱本质是重排，不重排即可 |

**不在本方案内（记录备查）**：在线评分数据接入、画廊预览、渣翻/广告固定空行排版 —— 均属后续「维护页视觉改造」批次，与本轮 DTO 重构合并做。

---

## 3. 施工批次

### 批次 1：G 口径修复 + A 删除不重算（后端，两者互相依赖）

#### 1.1 G：拆分「本地查重」与「联网核对」两个候选集

**现状**：`filterOfflineUpdateEnabled`（`services/offline.go:1516`）一次排掉三类：禁用路径、老化本、失效本。它被扫描（`offline.go:806`）与同步（`offline_task.go:335`）共用，导致老化本连本地查重都不参与。

**改法**：

1. 新增 `filterOfflineLocalCandidates(db, comics)`：**只**排除「额外路径已关闭离线维护」的漫画（`ScanPathID != "" && !enableOfflineUpdate`），不排除老化/失效。
2. 保留 `filterOfflineUpdateEnabled` 原语义（联网用）：在本地候选基础上再排 `AgedStatus` / `RemovedStatus`。
3. 扫描主流程 `maintainDedupWithProgress`：
   - `comics` 改为**本地全集**（只排除禁用路径）→ 规则 1（同 GID）、规则 2（hash）、规则 3b（本地父关系）、规则 4（目录签名）、规则 5（名称级 + Tier2）、结果组装全部用它；
   - **联网环节**改为在遍历时跳过 `AgedStatus || RemovedStatus`：`backfillGIDOnline`（规则 0）、规则 3a（在线父子关系发现）。这样既不多读一次库，也不会让老化本触发联网。
   - `PublishLocalDedupSnapshot`（本地判重先行）用本地全集。
4. `SyncMaintainDedupClusters`（`offline_task.go:308`）候选集改为本地全集，与扫描口径一致。

**风险**：
- 规则 4 目录签名会对新增纳入的老化本做首次签名预热（I/O）；已有 `sig_dir_mtime` 缓存，二次运行即快。**这是 G 的必然代价**，但删除路径（A 落地后）不再触发它。
- `removed_status`（E 站已下架/版权移除）也一并纳入本地查重：它与老化的排除理由完全相同（"避免重复联网 404"是联网理由，与本地文件是否重复无关）。**本方案已实现**（`filterOfflineLocalCandidates` 只排除「关闭离线维护的额外路径」），联网环节仍跳过。

#### 1.2 A：删除路径改为增量裁剪

**现状**：`handlers/offline.go:RemoveDedup` 每次删除（含批量）都调用 `InvalidateMaintainDedupResult` + `SyncMaintainDedupClusters`，后者全量重跑聚类（实测 5.9～6.0 s）。

**改法**：

1. `offline_task.go` 新增 `PruneMaintainDedupMembers(removedIDs []string)`：
   - 锁内遍历 `offlineMaintainRes.Clusters`，从每个簇的 `Members` 中剔除命中 id；
   - 成员数 < 2 的簇**整簇移除**（该组处理完毕）；
   - **保持剩余簇的原有顺序与 `ID` 不变**（不重排、不重编号）；
   - 清 `Stale`、前移 `FinishedAt`（沿用现有"定向同步即视为已同步"语义）；
   - 落库一次快照（`PersistMaintainSnapshot`）。
2. `RemoveDedup` 的两个分支（批量 + 单删）改为：`InvalidateMaintainDedupResult(ids)`（剔除 Items）→ `PruneMaintainDedupMembers(ids)`（裁剪簇 + 落库）。**不再调用 `SyncMaintainDedupClusters`**。
3. **忽略 / 确认新增**（A2，2026-11-01 追加）：`CreateIgnore`（title / gid 型）与 `AckIgnoreNewMembers` 改为 `ApplyIgnoreFiltersToMaintainResult`——只把已算好的簇按忽略表过滤（毫秒级），不重跑聚类；`RestoreIgnore`（恢复忽略）与 comic 型（成员级）忽略仍走 `SyncMaintainDedupClusters` 全量重算，因为前者需要把被隐藏的簇重新算出来、后者会改变聚类输入。

**验收**：单本删除端到端 < 200 ms；同一 3 本组删 1 本后，组仍在原位置、`id` 不变、成员变 2。

#### 1.3 批次 1 验证

- 新增后端测试：
  - `TestFilterLocalCandidatesKeepsAged`：老化/失效本不被本地候选过滤掉；禁用路径仍被排除。
  - `TestPruneMaintainDedupMembers`：3 本簇删 1 本 → 2 本、ID/顺序不变；2 本簇删 1 本 → 整簇消失；items 中配对项同步消失。
  - `TestMaintainClustersIncludeAged`：构造 2 本老化 + 同标题 → 名称级聚类应产出该簇（回归 G）。
- `go test ./...` 全绿。
- 用线上库副本实测：`RemoveDedupComic` + `Prune` 总耗时 < 100 ms（对比改造前 6 s）。

---

### 批次 2：B 排序与 ID 稳定化（后端）

**现状**：`dedup_title.go:469-474` 用 `sort.Slice` 只按「成员数」降序，再按排序后下标生成 `cluster-%d`。

**改法**：

1. 排序改为 `sort.SliceStable`，多级键：**成员数降序 → 置信度（high 优先）→ TitleKey → Artist**，保证同输入同输出、且顺序可预测。
2. `Cluster.ID` 改为**内容稳定标识**：`cluster-<keyPart 的 8 位短哈希>`，`keyPart` = Tier1 的 `clusterKey`（含核心名 + 卷号 + 画师）/ Tier2 的对应键；哈希撞车时追加 `-2`、`-3` 兜底。
   - 影响：前端 `:key="cl.id"`、`expanded[cl.id]` 都变成不透明字符串，**无需改前端逻辑**；已确认前端没有用 cluster id 做路由参数。
   - 忽略匹配用 `TitleKey + Artist`，**不受 ID 变化影响**。
3. `DedupCluster` 增加内部字段（不参与 JSON）保存 `keyPart`，由 `buildCluster` / Tier2 构造处填充。

**验收**：重新扫描两次，同一组的 `id` 相同；删除某成员后该组的 `id` 不变。

---

### 批次 3：D 批量删除 + per-item 状态（前端）

**现状**：`OfflineMaintain.vue` 只有单本「移除这本」；全局 `isRemoving` 锁全页按钮；并发点击被静默丢弃（`:434`）。

**改法**：

1. **per-item 状态**：`isRemoving: boolean` → `removingIds: Set<string>`；按钮 disabled 只针对自身；点击已在飞行中的项给出提示而非静默返回。
2. **批量选择**（沿用 Round43 设计，交互范式参考 `useBatchSelection.ts`）：
   - 成员卡右上角勾选框（选择模式由工具栏按钮或长按进入）；
   - 工具栏常驻：`已选 N 本 · 涉及 K 组` + 「移除（保留文件）」/「移除（含文件）」两个按钮；
   - 约束：**同组至少保留 1 本**（已勾选数 = 组内成员数 − 1 时，禁选最后一本）；
   - 一次提交 `POST /offline/maintain/remove {comicIds, deleteFile}`；
   - 成功后**本地剔除**这些成员（配合后端 A 的结果），必要时再 `loadResult()` 校准。
3. 删除后不再 `await loadResult()` 全量重拉（A 后端已把结果算好，本地裁剪即可与后端一致）；改为「本地裁剪 + 静默校准」。

**验收**：勾选 3 本一次提交，一次确认弹窗、一次请求、列表只移除这 3 张卡，其余不动。

---

### 批次 4：E 结果瘦身（仅做法 1）

**现状**：`DedupItem.Comic` / `PairComic` / `ClusterMember.Comic` 直接内嵌 `models.OfflineComic` 全字段，响应 923 KB。

**改法**：

1. 新增视图结构 `MaintainComicView`（`services/offline_view.go`），字段清单：
   `id`、`title`、`coverUrl`、`localPath`、`fileSize`、`pageCount`、`sourceMode`、`gid`、`updatedAt`、`tags`、`onlineTags`
2. 提供一个转换函数 `toMaintainComicView(c models.OfflineComic)`，在构造 `DedupItem` / `ClusterMember` 处调用。
3. 影响面（已核对全部构造点）：`services/offline.go:694,1271,1275`、`services/maintain_auto.go:101`、`services/dedup_title.go:490`、`services/dedup_match.go:426`、相关测试构造。
4. 前端 `OfflineMaintain.vue` / `OfflineCompare.vue` 的 DTO 类型同步删字段（前端本就没读这些字段，属于类型清理）。

**风险**：忽略清单页（`ListIgnoresWithMembers`）若复用 `ClusterMember`，需核对 `OfflineIgnore.vue` 实际字段依赖后再合并改动；不满足则给忽略清单单独保留原类型。

**验收**：`/offline/maintain/result` 响应体约 561 KB；维护页与对比页显示与改造前一致。

---

## 4. 不做 / 待确认

| 项 | 处理 |
|----|------|
| 过渡动画（F） | 不做 |
| 前端乐观更新（C） | 不做（A 落地后无必要） |
| `removed_status` 纳入本地查重 | 待珱垣确认，本轮保持现有排除行为 |
| tag 判定搬后端（E 做法 2） | 与"在线评分 + 画廊预览 + 排版"合并到后续视觉改造批次 |
| 忽略/恢复忽略的 6 s 重算 | 本轮不优化（低频操作） |

---

## 5. 验证计划（每批次收尾）

1. `cd backend && go test ./...`
2. `npm run type-check`（前端批次）
3. 线上库**只读副本**实测关键耗时与组数（脚本见 `DSPlans/Research/_measure/`）
4. 实机验收由珱垣在维护页完成（删除操作会真实作用于线上 `manga.db`，AI 不代跑）

## 6. 风险与回滚

- G 使候选集从 5033 → 7817，**扫描/忽略操作的聚类耗时上升**；但删除路径（A）已不再触发聚类，日常手感不受影响。若发现扫描明显变慢，可临时回退为「排除老化」并记录。
- A 的正确性依赖「删除不产生新簇」这一性质（已用 `TestResearchIncremental` 在真实数据上验证：增量裁剪与全量重算的簇集合完全一致）。
- B 的 ID 变更会让**存量快照**里的 `cluster-N` 与新 `cluster-<hash>` 不一致，但快照在下次扫描/裁剪后自然刷新，无迁移成本。
- E 的 DTO 收缩后，**今后新增前端字段必须同步改后端 `MaintainComicView`**（否则前端读不到，属静默缺失）。字段口径约定见 `services/offline_view.go` 头部注释。

---

## 7. 施工结果（2026-11-01，代码已落地）

四个批次全部完成，`go test ./...`、`npm run type-check`、`npm run build` 均通过；关键指标用线上库只读副本实测。

| 指标 | 改造前 | 改造后 | 说明 |
|------|--------|--------|------|
| 疑似重复组数（同一份库） | 41 组 | **288 组**（扫描后 286～287 组，强证据判删项不参与聚类） | G 修复漏检 |
| 单本删除后端耗时 | 约 5.9～6.0 s | **6.75 ms**（RemoveDedupComic 6.24 ms + Prune 0.51 ms） | A，约 900 倍 |
| 删除后簇列表 | 全量重排 + 重编号 | 位置与 `cluster.id` 不变，仅被删成员消失 | A + B |
| 结果体积（同数据 A/B） | 1425 KB（623 成员 / 287 组） | **871 KB**，**−38.9%**（每成员 2343 B → 1431 B） | E |
| 本地候选 / 联网候选 | 5033 / 5033（混同） | **7816（含老化 2779）/ 5033（老化 0）** | G 分层 |
| 忽略本组 / 确认新增 | 全量重算 6～10 s | **毫秒级**（定向过滤忽略表） | A2 |

### 已知代价（实测）

- **首次扫描的签名预热**：G 让老化本回到目录签名规则，实测 `文件夹内容签名：复用缓存 6942 本 ｜ 重算 872 本`，整轮扫描约 **91 秒**（线上为网络盘，会更久）。这是**一次性**成本，之后 `sig_dir_mtime` 命中即复用（第二轮实测 `重算 0 本`，整体 65 秒）。
- **结果绝对体积反而上升**：因为组数从 209 涨到 287（成员 444 → 623），瘦身后的总量 871 KB 仍高于改造前的 923 KB（但同等数据下是 −38.9%）。若后续觉得仍偏大，再上「E 做法 2（tag 判定搬后端，再省约 36%）」。
- **忽略 / 恢复忽略仍是全量重算**（约 6 秒）：本轮未优化，属低频操作。

### 遗留与后续

- ~~`removed_status` 是否纳入本地查重~~ **已纳入**（与老化本同口径，见 §1.1 风险说明）。
- **忽略 / 确认新增 也改定向**（A2，2026-11-01 追加）：`ApplyIgnoreFiltersToMaintainResult` 只把已有簇按忽略表过滤，不重跑聚类（毫秒级）；恢复忽略与 comic 型（成员级）忽略仍走全量重算——前者需要把被隐藏的簇重新算出来，后者会改变聚类输入。已用 `TestApplyIgnoreFiltersMatchesSync` 锁定"定向过滤结果 == 全量重算结果"。
- 发版时需在 `CHANGELOG.md` 顶部新版本区块补写本轮条目（含上文指标），并在 `DSPlans/YeonBriefs/` 列出 README 待改点。
- 部署需按惯例重新构建前端并同步 `backend/webui/dist` + 重编 exe，由珱垣重启线上进程生效。
