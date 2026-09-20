# Round44：维护界面 v3 重构方案（已确认）

> 状态：**方案已讨论确认（珱垣拍板），施工中**。
> 上游：`plans/round26-o2-o3-dedup-refine-plan.md`（O2 忽略标记 / O3 疑似重复簇）、`plans/round43-offline-dedup-delete-plan.md`（成员级删除）。
> 数据基线：实机库 `JHentai/DataBase/manga.db` —— 4,101 本离线漫画（查重候选 3,501）、疑似重复 **70 组 / 141 本**（高置信 47 / 中置信 23）、建议删除 0 项、忽略 11 条。
> 样稿：`Test/mockups/`（本地预览，不入库），已按本方案确认。

---

## 一、起因（实测归因）

1. **信息量与真实内容不匹配**：维护页为四种规则 × 三种操作 × 两种删除语义做了全量平铺，但实机库里「建议删除」为 0 项，真正有内容的只有「疑似重复 70 组」。
2. **"70 组挂着不清零"是机制造成的**：Round26 决策 4 规定"全量核对时已忽略项仍列出（折叠+徽标）"，导致每忽略一组只是从"疑似重复"搬到"已忽略"，总数几乎不变；且被忽略的组不再提示同作品新入库的重复。
3. **单组决策成本高**：必须跳转双列对比页再返回，70 组 × 2 次跳转 = 整理成本过高，于是"挂着不动"。
4. **忽略清单只能看文字**：title 型只显示指纹，看不到它到底覆盖哪些本子；实机库中已有 2 条失效条目（匹配 0 本 / 仅剩 1 本）长期占位。

## 二、已确认决策（珱垣拍板）

| # | 决策点 | 结论 |
|---|--------|------|
| 1 | 目标 | **彻底清零**：主列表待处理数能归到 0 |
| 2 | 视图骨架 | 分组折叠总览 + 组内决策单元；**不做**单组队列模式 |
| 3 | 分组维度 | 按「两本差在哪」分 6 类：高度相似 / 语言版本不同 / 体积差异 / 页数差异 / 去码不同 / 三本及以上；默认只展开「高度相似」 |
| 4 | 视图密度 | 默认**舒适**（成员卡并排），可切**紧凑**（一行一组） |
| 5 | 成员级动作 | 统一为「**移除这本**」（选删除谁）：2 本组等价于保留另一本，3 本组可逐个移除；移除后组内 <2 本该组自动消失 |
| 6 | 组级动作 | 「**都留（不再提示）**」= 忽略整组（可逆），是唯一的"保留"出口 |
| 7 | 删除语义 | 主按钮＝移除记录；「**含文件**」为页面级勾选项，默认不勾、**记忆上次选择（存后端设置）** |
| 8 | 批量删除入口 | **移除**（原"勾选 → 批量删除"取消，逐本移除已覆盖） |
| 9 | 每组至少保留 1 本 | **撤销该约束**（疑似重复本身要求 ≥2 本，不存在"只剩 1 本"的组） |
| 10 | 批量忽略 / 半自动决策 | **都不做**；疑似重复永远由用户逐组亲自确认 |
| 11 | 忽略清单 | **独立页**（不在维护页弹层）；成员按**宽松口径**展示（指纹匹配到的全部本，标注「同组 / 未构成重复组」） |
| 12 | 忽略语义升级 | **成员快照 + 新增感知**：忽略条目记录忽略当时的成员集合；簇成员**全在快照内 → 静默**，**出现快照外的本 → 照常列出**并标注"此前已忽略 · 新增 N 本"；点「确认新增」把新本子纳入快照后继续静默 |
| 13 | 撤销 Round26 决策 4 | 全量核对**不再永久列出**已忽略项（"永久占位"改为"有变化才冒头"） |
| 14 | 新增提示位置 | 主列表**顶部汇总提示条**（"N 条忽略项下出现新入库本子"） |
| 15 | 瘦身 | 页头收敛为「重新扫描 + ⋯ 更多」（全量核对 / 清除移除标记 / 查重设置）；联网复核开关挪入设置页；长说明改短；「建议保留」默认折叠；已忽略移出主列表 |
| 16 | 移动端 | 本次做**窄屏可用**（成员卡竖排、对比上下堆叠、按钮全宽）；完整多端适配留给 2.4/2.5 统一规划 |
| 17 | 样板流程 | 沿用 Round26：静态 HTML mockup → 确认 → 移植（已完成） |

## 三、后端改动

### 3.1 数据模型

| 模型 | 改动 |
|------|------|
| `models.IgnoredIdentifier` | 新增 `SeenComicIDs string`（JSON 数组，title 型的成员快照；`json:"-"` 不外发） |
| `models.DedupSetting` | 新增 `DeleteFileDefault bool`（"含文件"记忆，默认 false） |
| `services.DedupCluster` | 新增 `IgnoredNewCount int` / `IgnoreID string`（新增感知展示用） |

### 3.2 忽略匹配与新增感知（`services/ignore.go`）

- `IgnoreIndex` 的 title 部分升级为 `map[key]*TitleIgnoreEntry`，条目携带 `ID` 与快照集合 `seen map[string]bool`。
- `MatchTitle(titleKey, artist) *TitleIgnoreEntry`：先精确键，未命中再用 Round42 D8 的归一键兜底（保持存量旧 key 兼容）。
- `Entry.CountNewMembers(ids []string) int`：统计簇成员中**不在快照内**的本数。
- `CreateIgnore`：title 型创建时即计算并写入快照（`matchTitleMemberIDs`，一次全库指纹扫描）。
- `BackfillIgnoreSnapshots(db)`：启动时幂等回填存量（快照为空）条目的快照，避免升级后旧忽略项集体"复活"。
- `ListIgnoresWithMembers(db)`：为忽略清单页返回成员（宽松口径），成员带 `grouped`（同 `clusterKey` 内 ≥2 本，即真的会构成重复）与 `isNew`（忽略后新增）标记，并给出 `matchedCount / groupCount / newCount`。
- `AckIgnoreSnapshot(db, id)`：确认新增 —— 把快照更新为当前匹配集合。

### 3.3 忽略过滤语义（`services/dedup_title.go` / `dedup_match.go`）

Tier1 与 Tier2 的忽略判定统一改为：

```
命中 title 型忽略 →
  簇成员全在快照内  → 静默跳过（不再列出）
  存在快照外的成员  → 列出，标记 Ignored=true / IgnoredNewCount=N / IgnoreID
```

不再区分增量 / 全量（`forceFull` 不再影响忽略判定）——对应决策 12、13。

### 3.4 接口

| 接口 | 改动 |
|------|------|
| `GET /offline/ignore/list` | 返回体新增成员数组与 `matchedCount / groupCount / newCount` |
| `POST /offline/ignore/:id/ack` | **新增**：确认新增（刷新成员快照） |
| `GET/POST /offline/dedup/setting` | 新增 `deleteFileDefault` 读写 |

## 四、前端改动

| 文件 | 改动 |
|------|------|
| `src/views/offline/OfflineMaintain.vue` | 重构：页头收敛（重新扫描 + ⋯ 更多菜单）、顶部忽略新增提示条、待办进度条（70 → 0）、按差异模式 6 类分组折叠、舒适/紧凑视图、成员卡「移除这本」、组级「都留（不再提示）」、组内就地展开对比、页面级「含文件」勾选（默认不勾 + 后端记忆）、窄屏可用；移除已忽略折叠区与忽略清单弹层 |
| `src/views/offline/OfflineIgnore.vue` | **新建**：忽略清单独立页（条目折叠展开成员卡片、同组/未构成重复组标注、失效条目清理、忽略后新增提醒与确认新增、恢复忽略） |
| `src/router/index.ts` | 新增 `/offline/ignore` 路由（`meta.requiresAdmin`） |
| `src/components/OfflineSidebar.vue` | 新增「忽略清单」入口（admin） |
| `src/views/offline/OfflineCompare.vue` | 文案对齐：「留这本」→「移除这本」 |

## 五、验证

- `cd backend && go test ./...`（含新增：快照/新增感知/宽松口径成员/确认新增的单测）
- `npm run type-check`、`npm run build`（产物同步 `backend/webui/dist`）
- Playwright 实机回归：更新 `Test/pw-bug/verify-round26-ui.mjs`（忽略清单独立页导航）与 `verify-round26-cluster.mjs`，新增维护页 v3 主流程验证脚本

## 六、影响面

- 后端：`models/ignore.go`、`models/dedup_setting.go`、`services/ignore.go`、`services/dedup_title.go`、`services/dedup_match.go`、`services/dedup_verify.go`、`services/offline_task.go`、`handlers/offline.go`、`router/router.go`、`main.go`
- 前端：`views/offline/OfflineMaintain.vue`、`views/offline/OfflineIgnore.vue`（新）、`views/offline/OfflineCompare.vue`、`router/index.ts`、`components/OfflineSidebar.vue`、`components/settings/DedupSettings.vue`（新）、`views/SettingsView.vue`
- 文档：本文件、`README.md`、`VerNotes/wiki.md`、`PROJECT_TREE.md`
- 产物：`backend/webui/dist`

## 七、施工实测记录（2026-09-20）

**验证环境**：实机库副本（`Test/pw-bug/v3/manga.db`，4,101 本 / 70 组 / 11 条忽略）+ 重新编译的 exe（内嵌新前端产物）+ Playwright 脚本 `Test/pw-bug/verify-round44-maintain.mjs`（21 项断言，**21/21 通过**）。全程未触碰真实库。

**施工中发现并修复的三个问题**：

1. **渲染崩溃（3 本组展开对比）**：`memberDiff` 用 `members[1-mi]` 比较，`mi=2` 时索引变成 −1 → `undefined.pageCount` → 整个页面被错误边界接管。
   修法：改为与「成员 A」比较（`refIdx = mi === 0 ? 1 : 0`），对任意成员数都安全。
2. **「新增感知」假阳性**：忽略键是「核心名 + **组级** artist」，而簇内成员的 artist 可能各不相同（「作者未知兜底」合并出的组）→ 那些成员不在快照内 → 忽略后立刻被误报为「新增 N 本」。
   修法：新增的判定叠加「**入库时间晚于忽略创建时间**」这一条件（`CountNewMembers` 同时看快照与 `AddedAt`），命中真实语义（忽略之后新入库的本子），并让忽略清单页的 `isNew` 标注同口径。
3. **升级后的存量忽略**：Round44 之前写入的条目没有成员快照，若把空快照当作「无已知成员」，所有已忽略的组会集体复活。
   修法：`BackfillIgnoreSnapshots` 在服务启动时按当前匹配集合幂等回填（本次实机库回填 11 条，启动日志可见）。

**实测到的行为特征（非缺陷，供体验参考）**：忽略 / 恢复 / 确认新增都会触发后端 `SyncMaintainDedupClusters` 同步重算名称级簇（数千本聚类），响应约 **2~3 秒**（实测 3087ms），前端有 toast 提示、列表随后刷新。若日后体感偏慢，可考虑把该重算改为异步或只重算 Tier1。

**验收要点（脚本覆盖）**：页头收敛为「重新扫描 + ⋯ 更多」；差异分组 6 类；待处理组数＝卡片数（70）；差异 chips；成员级「移除这本」/ 组级「都留（不再提示）」；就地展开对比；「含文件」勾选刷新后仍保持（后端记忆）；「都留」后该组从列表消失（70 → 69）；忽略清单独立页成员卡片 + 「同组」标注 + 失效条目提示 + 恢复后条目减少；`POST /offline/ignore/:id/ack` 可用；窄屏 390px 无横向溢出；无前端运行时错误。

