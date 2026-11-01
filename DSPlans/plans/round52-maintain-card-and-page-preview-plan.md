# Round52 维护卡片改版 + 展开区页预览 · 方案

> 状态：待施工（珱垣 2026-11-01 定稿：成员卡 5 行、展开区缩略图墙 320 高 + 64px、纯文字评分、不做页码对齐）
> 视觉样稿：`_local/Test/mockups2/current.html`（现有样式还原 + 本方案改动，`_local/` 不入库）
> 涉及文件：`src/views/offline/OfflineMaintain.vue`、`src/stores/ratingStore.ts`、`src/components/ItemCard.vue`、`backend/internal/models/models.go`、`backend/internal/services/{offline_view,metadata,offline,tag_maintain,cover}.go`、`backend/internal/handlers/comic.go`、`backend/internal/router/router.go`

## 一、目标

1. 维护页成员卡统一为 **5 行常驻**：标题 / 语言·页数·大小 / 评分★+4 标签槽 / 路径 / 移除。
2. 新增**社区评分**（E 站全站评分）字段与展示口径：维护卡显示社区评分；本地卡片在缺个人评分时以社区评分兜底。
3. 展开对比项从「成员信息放大版」改为**页预览缩略图墙**：左右两栏、栏内滚动、滚动加载、点击出原图，用于对比汉化组翻译差异与页数差异位置。
4. 不改动维护页既有框架：页头、进度卡、分组、建议删除卡、组级动作、展开区外框均保留。

## 二、数据层：社区评分

### 2.1 字段

- `offline_comics` 新增列 **`community_rating`**（REAL，默认 0，GORM AutoMigrate 自动加列）。
- `models.OfflineComic` 新增 `CommunityRating float64 \`json:"communityRating"\``。
- 维护结果视图 `services.MaintainComicView`（Round50 瘦身 DTO）新增 `CommunityRating float64 \`json:"communityRating"\``，并同步 `toMaintainComicView`。

**不复用既有 `rating` 列**：`rating` 现由前端 `getEffectiveRating()` 当作「离线社区评分」兜底使用，语义已与「用户个人评分（ComicRating 表）」分离但历史上从未被写入过（线上 7778 本全为 0）。新增独立列是为了让「用户评分 / 社区评分」在数据库层彻底分开，避免后续误用。

### 2.2 写入点（均为顺带写入，不新增网络请求）

| 场景 | 位置 | 数据来源 | 说明 |
|---|---|---|---|
| 入库扫描（本地，无网络） | `services/scanner.go` processCandidate | metadata JSON 的 `rating`、ComicInfo `CommunityRating` | 需给 `ParsedMetadata` 增加 `Rating` 字段并在 `parseEHJSONMetadata` / `applyXMLMeta` 解析；**仅在库中值为 0 时写入**（联网值优先） |
| 强制全量在线核对 | `services/offline.go` maintainDedup 3a 段 | 已有 `FetchGalleryDetail().Rating` | 该段已对每本拉详情，零额外请求 |
| 每日 Tag 刷新 | `services/tag_maintain.go` refreshAllTagsLocked | 同上 | 同上；该任务遍历全部含 GID 的本（含老化本） |
| 更新检测（顺带，可选） | `services/offline.go` checkUpdatesWithProgress | 同上 | 同上 |

写入统一走 `db.Model(c).Update("community_rating", v)`，并仅当新值与旧值不同才写（减少写放大）。

### 2.3 读取与展示

- 维护卡第三行（最左）：`★ xx.xx`（两位小数，`tabular-nums`），值为 0 时显示 `★ —`（灰）。
- 本地卡片 `ItemCard`：`getEffectiveRating()` 兜底链改为 **个人评分 → `comic.communityRating` → 0**（当前为个人评分 → `comic.rating`）。
- 离线星级筛选（`OfflineHome`）与随机推荐沿用各自逻辑；筛选走 `getEffectiveRating` 后自动生效。
- 本地详情页 **本次不显示**社区评分（详情页 UI 另行改版）。
- **硬约束**：社区评分不得参与 XP 词云权重（`xp_cloud.go` 只读 `ComicRating`，本次不动）；不得用于任何自动删除，模糊本子去留仅由用户决定。

### 2.4 旧数据回填

- 线上实测：离线漫画 **7778 本**，`rating` 全 0；老化（`aged_status=1`）**2743 本**；父链已核对 7439 本。
- `强制全量在线核对`（`forceFull`）3a 段对 `AgedStatus` / `RemovedStatus` **硬跳过**，因此：
  - 非老化本：跑一次「强制全量在线核对」即可写入，约 5000 本 × 1.2s ≈ **100 分钟**；
  - 老化本：随**每日 Tag 刷新**自然补齐（该任务 `WHERE g_id != ''` 不过滤老化，整轮约 2.6 小时）。
- **不新增独立回填任务**（珱垣决策）。若日后需要老化本立即补齐，只需放开 3a 段的 `AgedStatus` 跳过（多约 55 分钟）。

## 三、后端：页缩略图接口

### 3.1 接口

`GET /api/v1/comics/:id/page-thumb/:index`（public 组，与 `/comics/:id/page/:index` 同级）

- `index` 为**有效页索引**（与 `/page/:index` 一致，自动跳过隐藏页），与维护页 `pageCount` 口径一致。
- 返回 JPEG（宽 240px、质量 80），带 `Cache-Control: public, max-age=86400` 与 `ETag`（源文件 mtime + size）。

### 3.2 实现（复用封面已有机制）

- 缓存路径：`<dataDir>/page_cache/<comicId>/<index>.jpg`（新增目录，随 `cover_cache` 同级）。
- 复用 `services/cover.go` 的单飞锁（per comic+page）、解码信号量（2）、源变更检测思路；
- 无法解码（AVIF 等）时回退返回原图字节（与封面一致的行为）；
- 新增 `page_cache` 清理：删除不存在漫画的目录 + 超过 N 天未访问的分页缓存（可复用 `cover_cache_gc` 的遍历逻辑）。

## 四、前端：维护页改动

### 4.1 成员卡（5 行）

行结构（两处卡片统一：`建议删除` 项与 `疑似重复` 成员卡）：

1. **标题**（2 行 clamp）
2. **语言 / 页数 / 文件大小**（语言缺失显示虚线 `null`，与现状一致）
3. **评分 + 4 个决策标签槽**：`★ 4.31`（有）/ `★ —`（无，灰）；标签固定 4 槽（马赛克修正 / 渣翻 / 外部广告 / 无修正），缺失的用透明占位保留位置
4. **文件路径**（单行省略）
5. **移除按钮**（整行或左对齐，沿用 `.action-btn.danger-soft`）

其它保留：`card-head` 的高置信/差异 chips、`mode-chip`、`reason-chip`、`pick-corner` 勾选框。

封面：撑满 5 行内容高度（`align-self: stretch`，宽度约 106px），上下留白相等。

### 4.2 展开对比区（就地）

外框与高度框架不变：

```
.inline-compare
├─ .diffline   这两本差在哪：……
└─ .compare
   ├─ .panel（成员 A）
   │  ├─ .panel-head   成员 A + 差异说明 + 已载 N/M 页
   │  ├─ .thumb-wall   缩略图墙（高 320，内部滚动）
   │  └─ 移除这本（整宽）
   └─ .panel（成员 B）同上
```

- **缩略图墙**：`height: 320px; overflow-y: auto`；`grid-template-columns: repeat(auto-fill, minmax(64px, 1fr)); gap: 4px`；暗色细滚动条。
- **懒加载**：初始 12 张；滚动到墙底（提前 120px）自动补 6 张；每次补完检查是否仍可滚，可滚则继续补；全部加载后显示「已全部加载（N 页）」。两栏独立加载。
- **长本**：2000+ 页也只滚不撑高，按批请求。
- **点击缩略图** → 灯箱：
  - 原图直出（复用现有 `/comics/:id/raw-page/:index`，不缩放，画质差异可辨）；
  - `← →` 翻页、`Esc` 关闭；
  - **切换**按钮状态机：
    - 每本记录 `lastPage`（同本翻页只更新最后一次）；
    - `recent` 按打开顺序去重记录本；
    - 切换目标 = `recent` 中第一个 ≠ 当前本；
    - 切换后把目标本移到 `recent` 最前 → 再点即回跳；
    - 目标本从未打开过 → 按钮置灰「另一本未打开」；
    - 多本（≥3）时只保留最近两个：A P1 → C P5 → B P6，切换→C P5，再切→B P6。
- **禁止页码对齐**：不做「A/B 同页并排」，切换只按上述最近两本逻辑跳转。

### 4.3 改动范围（不做）

- 紧凑视图（一行一组 A/B 按钮）**不改**；
- 独立对比页 `OfflineCompare.vue` **不改**；
- 本地详情页**不显示**社区评分；
- 忽略清单页**不改**。

## 五、验收

1. 成员卡 5 行常驻；缺语言/缺标签时仍占位；同组两张卡第三行位置对齐。
2. 评分三态一致：`★ 4.31` / `★ 2.00` / `★ —`（纯文字、无背景框、数字等宽）。
3. 展开区：两栏缩略图墙（高 320、64px 缩略图）、栏内滚动条、滚动加载（12 → +6/批）、`已载 N/M` 与「已全部加载」文案；页面不因展开而无限变长。
4. 点击缩略图出原图；`← →` 翻页；切换状态机满足：单本未打开置灰、两本来回、三本只留最近两个、同本只记最后一页（含 A P1 → 切走 → 回来是最后翻到的页）。
5. 缩略图接口：首请求生成缓存、二次命中返回 304/缓存、删除画廊后缓存目录可清理、隐藏页索引正确跳过。
6. 社区评分：入库扫描（本地 metadata）、强制全量在线核对、每日 Tag 刷新三条路径均能写入；本地卡片在缺个人评分时显示社区评分；本地详情页不显示；XP 词云权重不受影响。
7. 200 页以上本子展开不卡、滚动加载连续、页面高度稳定。

## 六、风险与取舍

- **缩略图 IO**：zip 每取一页需打开压缩包；靠单飞锁 + 解码信号量（2）+ 240px 缓存降低开销，首次展开仍可能有 1~2 秒生成延迟（显示「加载中…」）。
- **老化本评分延迟**：只能等每日 Tag 刷新（最长约 1 天），不接受时再放开 3a 段跳过。
- **维护结果体积**：`MaintainComicView` 仅 +1 个 float 字段，可忽略（Round50 已瘦身）。
- **评分缺失**：无 GID 且本地 metadata 无 rating 的本子长期显示 `★ —`，属预期。
