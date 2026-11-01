# Round51：忽略语义改为「按组成员」实施计划

> 状态：**方案已确认（2026-11-01），施工中**。
> 上游：`DSPlans/Research/忽略逻辑现状调研.md`（§11 疑点、§13 决策）。
> 目标：把「忽略」从「按名字指纹」改为「按组（簇）成员集合」，同时解决恢复忽略耗时、忽略清单失效误报、comic 型悬空、旧 key 双轨、两套新增口径不一致。

---

## 一、已确认语义（珱垣拍板）

1. **忽略对象** = 维护页上的那个簇的**全部成员**（不是「名字」）。
2. **匹配规则**：
   - 簇成员**全部**在忽略集合内 → 静默（不出现）；
   - 簇成员有不在集合内的成员 → 列出该簇（`ignored=true`），其中「忽略之后新入库」的成员计入「新增 N 本」；
   - 集合内成员全被删除 → 该条目失效（不匹配任何簇）。
3. **保守口径**：「新增」只统计「入库时间晚于忽略创建时间」的成员；`AddedAt` 缺失或早于忽略时间 → 不计入。
4. **恢复忽略**：只删条目、不触发聚类重算（毫秒级）；被恢复的组等下次「重新扫描」才回到维护列表。
5. **存量迁移 = 破坏性更新**：升级后第一次启动清空旧忽略条目（一次性、幂等），忽略清单页一次性提示重新忽略；发布说明写明。
6. **删除 comic 型**（无入口、0 使用、与组忽略语义不兼容）。
7. **删除旧 key 双轨**（`titleEntriesNorm` / `ignoreTitleKeyOf` / `ignoreLookupKeys`）——不再依赖名字匹配。
8. **gid 型保留**（父画廊提示忽略），语义不变。
9. 检测层 `forceFull` 死参数已删除（2026-11-01）。

---

## 二、数据模型

`models.IgnoredIdentifier` 新增：

| 字段 | 说明 |
|------|------|
| `SnapshotVersion int` | 新条目 = 2；旧条目（自动迁移前）为 0。启动时 `DELETE WHERE snapshot_version < 2`（幂等：之后新条目都是 2） |

`SeenComicIDs` 语义变更：从「按名字匹配到的成员」变为「**忽略时该簇的全部成员 id**」（JSON 数组）。

---

## 三、后端改动

### 3.1 services/ignore.go（核心重构）

- `CreateIgnore(db, typ, titleKey, artist, gid, comicID, note string, memberIDs []string)`：
  - `title` 型：`memberIDs` 必填（前端传该簇成员）；`SeenComicIDs = 去重排序后的 memberIDs`；`SnapshotVersion = 2`。
  - 幂等：同 `type+titleKey+artist` 已存在 → 把新成员并入现有集合（`SeenComicIDs ∪ memberIDs`）并返回（不重置创建时间）。
  - `gid` 型：不变；`comic` 型：删除分支。
- `AckIgnoreSnapshot(db, id string, memberIDs []string)`：`title` 型把 `memberIDs` 并入集合（若空则返回现有）；`SnapshotVersion = 2`。
- `TitleIgnoreEntry`：保留 `ID / TitleKey / Artist / CreatedAt / seen`；新增 `MatchCluster(members []ClusterMember) (matched bool, newCount int)`：
  - 交集为空 → 不命中；
  - `C ⊆ S` → 命中，newCount=0（静默）；
  - 否则命中，newCount = `|{m∈C\S: 忽略后入库}|`（保守）。
- `IgnoreIndex`：
  - 删除 `titleEntriesNorm`、`MatchTitle`、`IsTitleIgnored`、`ignoreLookupKeys`、`ignoreTitleKeyOf`；
  - `titleEntries` 保留为 `map[ignoreID]*TitleIgnoreEntry`（按条目 ID，不再按名字键）；
  - 新增 `MatchCluster(members []ClusterMember) *TitleIgnoreEntry`（取交集最大者）。
- `ListIgnoresWithMembers`：成员来源改为「集合成员」（按 id 查库；缺失的 id 不展示，计入失效判定）；同组标注按 `clusterKey` 计算；`newCount` 从维护缓存中「与集合有交集的簇」的 `C\S 新入库数` 计算（无缓存则为 0）。
- `matchTitleMemberIDs`、`buildIgnoreMatchIndex`、`ignoreMatchIndex` 等名字指纹匹配辅助函数删除（不再使用）。
- `BackfillIgnoreSnapshots` 删除；改为 `PurgeLegacyIgnores(db) int`（`DELETE WHERE snapshot_version < 2`），启动时调用并打日志。
- `ErrIgnoreNotFound` 保留。

### 3.2 services/dedup_title.go / dedup_match.go

- 簇过滤改为 `entry := ignoreIdx.MatchCluster(cl.Members)`：
  - `entry == nil` → 保留；
  - `entry != nil && newCount == 0` → 静默（`continue`）；
  - `entry != nil && newCount > 0` → 保留，设 `Ignored=true / IgnoredNewCount=newCount / IgnoreID=entry.ID`。
- `comic` 型过滤（`IsComicIgnored`）删除，连同 `IgnoreIndex.comicIDs`。
- `CountNewMembers` 由 `MatchCluster` 取代（保留函数签名删除）。

### 3.3 services/offline_task.go

- `ApplyIgnoreFiltersToMaintainResult`：title 型过滤改 `MatchCluster`；逻辑不变（毫秒级）。

### 3.4 services/offline.go

- 规则 3 的 gid 忽略不变（`IsGIDIgnored` 保留）。
- 删除对 `IsComicIgnored` 的引用（若有）。

### 3.5 handlers/offline.go

- `createIgnoreReq` 增加 `Members []string`（`json:"members"`）。
- `CreateIgnore`：`comic` 型分支删除（类型校验在 service 层拒绝）；title/gid 仍 Apply 定向过滤；无 Sync 路径。
- `RestoreIgnore`：删除 `SyncMaintainDedupClusters` 调用（只删条目）。
- `AckIgnoreNewMembers`：请求体增加 `members`；调用 `AckIgnoreSnapshot(db, id, members)`。

### 3.6 main.go

- `BackfillIgnoreSnapshots` → `PurgeLegacyIgnores`（启动时清空旧条目，打日志）。

### 3.7 database/db.go

- `AutoMigrate` 已包含模型，新增字段自动迁移（`snapshot_version` 默认 0 → 旧条目；新条目写 2）。

---

## 四、前端改动

### 4.1 OfflineMaintain.vue

- `keepWholeGroup(cl)`：请求体 `{type:'title', titleKey, artist, members: cl.members.map(m=>m.comic.id)}`。
- `ackIgnoreNew(cl)`：请求体 `{members: cl.members.map(m=>m.comic.id)}`。
- 「此前已忽略 · 新增 N 本」徽标逻辑不变（后端返回 ignoredNewCount）。

### 4.2 OfflineIgnore.vue

- 成员卡片：后端返回集合成员（`comic` 字段），展示逻辑不变；「忽略后新增」成员标记可保留（后端 isNew 基于集合外成员？清单页改为「新增」来自缓存，实施时若难以保证则移除该标记，维护页顶部提醒兜底）。
- 失效文案：`matchedCount<=1` 的提示改为「该忽略组当前已不构成重复」；「可清理」提示保留。
- 顶部一次性提示（破坏性更新）：localStorage 键 `ignoreUpgradeNoticeDismissed`；未标记时显示「忽略逻辑已升级为按组忽略，旧忽略记录已重置，请在维护页重新忽略需要的组」，点「知道了」写入标记。
- 删除 `typeLabel` 的「成员级」分支与 `comicItems` 统计。

### 4.3 测试

- 更新 `ignore_members_test.go`、`ignore_round42_test.go`、`dedup_title_test.go`、`maintain_result_sync_test.go`、`offline_dedup_e2e_test.go`、`round50_maintain_delete_test.go` 中涉及：
  - `CreateIgnore` 新签名（传 members）；
  - `MatchTitle`/`CountNewMembers`/`IsComicIgnored` 删除后的用例；
  - 旧 key 双轨用例删除（`ignore_round42_test.go` 整体移除或改为「旧条目被 Purge」用例）；
  - 恢复忽略不重算的用例（新增：`RestoreIgnore` 后缓存不变，扫描后出现）；
  - 集合匹配用例：C⊆S 静默、C⊄S 列出且 newCount 保守、集合失效。
- 运行 `go test ./...` 与前端 `npm run type-check`。

---

## 五、验证清单

- [ ] 新建忽略：传 12 个成员 → 清单该条目覆盖 12 本，维护页该簇消失；
- [ ] 关闭重算：恢复忽略接口 < 100ms，维护列表不变化，重新扫描后该组出现；
- [ ] 新入库本子与组内成员相似 → 维护页列出并标「新增 N 本」，确认新增后静默；
- [ ] 旧条目启动清理（幂等：重启不重复清理新条目）；
- [ ] gid 忽略行为不变；
- [ ] 前端 type-check、后端全量测试通过。

---

> 备注：破坏性更新在发布说明（Release Notes）中写明「忽略记录已重置，需重新忽略一次」。
