# Round55：书签交互优化 + 剔除标签反向写回（实施方案）

> 状态：方案已对齐（样板 [bookmark-filter-bar.html](mockups/bookmark-filter-bar.html) 已确认），待实施。
> 发版口径：本轮落地完成后**不发 v2.4.0**，仅完成代码修改、单测验证与 `git commit + push`；发版另行通知。

---

## 一、书签交互优化（纯前端）

### 1.1 书签页筛选与搜索（`src/views/online/OnlineBookmarks.vue`）

采用已确认的**方案 1（页内单行轻量过滤条，不动全局 `TopBar`）**，视觉参照 `DSPlans/plans/mockups/bookmark-filter-bar.html`：

- **过滤条位置**：放在 `.bm-page-head` 下方、卡片墙上方（仅在 `total > 0` 时渲染）。
- **左侧 — 标记筛选胶囊**：
  - 4 项：`全部` / `追新` / `兴趣` / `续扫`（与 `BookmarkCard` 顺序和色点保持一致：`#4fa3e3` / `#9a9a9a` / `#4cc38a`）。
  - 每项显示对应标记的全量书签数，点击切换当前激活标记。
- **右侧 — 紧凑关键词筛选框**：
  - 宽度约 `240px`（窄屏 `max-width: 820px` 下占满一行），占位符：`筛选当前书签（标题 / 副标题）…`。
  - 实时匹配主标题 `titleOf(bm)` 与副标题 `bookmarkDisplayTitle(bm)`（不区分大小写、`trim()` 后子串匹配）。
  - 非空时右侧显示一键清除按钮 `×`。
- **统计与空态**：
  - 有筛选条件时，页头统计文本变为 `筛选命中 ${matched} / 共 ${total} 条 · 置顶 ${pinnedCount}/${PINNED_BOOKMARK_LIMIT} · 有更新 ${updateCount}`。
  - 置顶区与普通区共用同一套过滤结果；若命中为 0，展示「没有匹配的书签」+ 一键「清空筛选」按钮。

### 1.2 书签重命名预填充（`src/components/BookmarkCard.vue`）

- 现状：`rename()` 调用 `modal.prompt('输入自定义名称（留空则恢复默认显示）', props.bm.name, '修改名称')`，未自定义过名称时输入框为空。
- 改动：
  1. 预填充值改为 `props.bm.name || title.value`（即卡片当前展示的主标题，例如 `搜索: XXXX` 或翻译名）。
  2. **防写死保护**：若该书签原本没有自定义名称（`!props.bm.name`），且用户点确定时未改动内容（`val.trim() === title.value`），视为无变更直接返回，不把动态默认标题固化为静态 `name`。
  3. 用户清空后点确定（`val.trim() === ''`）：照旧传 `''` 调用 `renameScrapeBookmark(props.bm.id, '')`，恢复默认显示。

### 1.3 创建书签「智能回显」+ 转移锚点同步保存配置

#### 候选匹配与选型纯函数（新增 `src/utils/bookmarkPrefill.ts` + 补单测）

输入：`bookmarks: ScrapeBookmark[]`、`currentConfig: Partial<SearchConfig>`、`targetPostedAt?: string`
输出：命中时返回选中的 `ScrapeBookmark`，无同条件书签返回 `null`。

1. **筛选同条件候选**：沿用 `isSameTransferScope(b.config, currentConfig)`（与「锚点转移」判据一致）。候选为空则返回 `null`。
2. **解析时间**：将 `targetPostedAt` 与各候选的 `b.anchor?.postedAt` 解析为毫秒时间戳（`Date.parse` / 兼容 `YYYY-MM-DD HH:mm` 格式，替换空格为 `T`）。
   - 若 `targetPostedAt` 不可解析，或所有候选都无有效 `postedAt`：兜底返回最近创建的候选（按 `createdAt` 降序取首条）。
   - 否则在有有效 `postedAt` 的候选集合中按时间**升序**排列（旧 → 新）。
3. **时间轴落点判定**（记当前画廊时间为 `T`）：
   - 若 `T` 落在两端之外（`T <= earliest` 或 `T >= latest`）：直接返回距离 `T` 最近的那条（即 `earliest` 或 `latest`）。
   - 若 `T` 夹在相邻两条 `older`（较旧，`older.t <= T`）与 `newer`（较新，`T <= newer.t`）之间：
     - 若存在 `older.t === T` 或 `newer.t === T`（同时间戳）：优先取等时间戳的那条（若两侧皆等，走下方优先级）。
     - **方向匹配判定**：
       - `olderMatch = older.role === 'newest'`（追新往更新方向推进，`T` 比它新 → 匹配）
       - `newerMatch = newer.role === 'oldest'`（续扫往更旧方向推进，`T` 比它旧 → 匹配）
     - **决策分支**：
       1. 仅 `olderMatch` 为真（如 `(兴趣, T, 追新)`）→ 选 `older`（追新）
       2. 仅 `newerMatch` 为真（如 `(续扫, T, 兴趣)`）→ 选 `newer`（续扫）
       3. 两侧都匹配（`newer=续扫, older=追新`），或正好是追新与续扫夹心（`newer=追新, older=续扫`）→ **统一选 `oldest`（续扫）**（避免误带出追新的「开启更新探测」）
       4. 其余两侧都不匹配（如 `newer=追新, older=兴趣`，或两侧同为一种标记）→ 选时间距离 `|T - postedAt|` 更近的那条；等距时按 `oldest > interest > newest` 兜底。

#### 弹窗与保存联动（`src/components/BookmarkCreateModal.vue` + `src/views/online/OnlineHome.vue`）

- `BookmarkCreateModal.vue` 新增可选 prop `prefillSource?: ScrapeBookmark | null`：
  - 每次 `props.show` 变为 `true` 时，若存在 `prefillSource`，将 `role / note / updateWatch / bulkDone` 初始化为 `prefillSource` 的对应值；否则重置为默认（`newest / '' / false / false`）。
  - 当命中 `prefillSource` 时，在弹窗信息块下方显示一行轻提示：`已载入已有书签配置（${roleLabel} · ${postedAt || '无时间'}）`。
- `OnlineHome.vue`：
  - 打开弹窗时根据 `pendingAnchor.value?.postedAt` 和当前 `snapshotOnlineSearchConfig()` 计算 `prefillSource` 传给 `BookmarkCreateModal`。
  - **锚点转移同步保存配置**：在 `handleBookmarkSave` 中，用户确认「锚点转移」时，除了调用 `updateBookmarkAnchor(existing.id, ...)`，若弹窗里的 `note`、`updateWatch` 或 `bulkDone` 与 `existing` 不一致，一并调用 `updateBookmarkFields(existing.id, { note, updateWatch, bulkDone })` 落库，避免用户在弹窗里的修改被丢弃。

---

## 二、功能简化：剔除「标签反向写回 ComicInfo」

### 2.1 边界与兼容红线

- **保留 `Comic.OfflineRemoveTags`**：每日 E 站 Tag 刷新时仍用于跳过本地手动删除的 tag，**不可删除**（仅清理注释中“写回剔除”措辞）。
- **数据库兼容**：SQLite + GORM `AutoMigrate` 不会删旧列；从 `TagMaintainSetting` 结构体中移除写回字段（或不再读写）对线上存量 `manga.db` 完全无损。
- **磁盘历史产物**：历史上已写回进本地文件夹/归档的 `ComicInfo.xml` 不做回滚或清理。

### 2.2 后端清理清单

1. `backend/internal/router/router.go`：
   - 删除 `admin.POST("/offline/tags/writeback", tagMaintainHandler.Writeback)` 及注释中的写回描述。
2. `backend/internal/handlers/tag_maintain.go`：
   - 删除 `Writeback` handler；
   - 从 `saveTagSettingReq` 与 `SaveSetting` 中移除 `EnableWeeklyWriteback` / `WritebackWeekday` / `WritebackHour`。
3. `backend/internal/models/models.go`：
   - 从 `TagMaintainSetting` 中移除 `EnableWeeklyWriteback`、`WritebackWeekday`、`WritebackHour`、`LastWeeklyRunAt`（确认全库无其他引用）；
   - 更新 `Comic.OfflineRemoveTags` 注释（去除“/写回剔除”）。
4. `backend/internal/services/tag_maintain.go`：
   - 删除 `WritebackResult`、`WritebackComicInfo()`、`writebackLocked()`，以及仅被写回调用的 `writeComicInfoFile()`、`writeComicInfoToDir()`、`writeComicInfoToZip()`（删除前用 `git grep` 二次确认无其他调用方）；
   - `LoadTagMaintainSetting` 默认值移除写回字段；
   - `TagMaintainProgress` 去掉 `Written` 字段（若前端不再展示）并将 `Type` 注释收敛为 `refresh`。
5. `backend/internal/services/tag_scheduler.go`：
   - 仅保留每日刷新调度（以及触发书签更新探测的既有钩子，注意**不要误删**每日刷新顺带触发的书签探测逻辑！）；
   - 移除每周写回调度分支与下一次写回时间计算。
6. 后端单测：搜索并清理/更新涉及 `Writeback` / `EnableWeeklyWriteback` 的测试用例（如 `tag_maintain_test.go` / `tag_scheduler_test.go` 等）。

### 2.3 前端清理清单

1. `src/components/settings/TagMaintainSettings.vue`：
   - 顶部状态区删除「上次每周写回」一项；
   - 删除「每周反向写回 ComicInfo」开关、写回日、写回时刻配置块与「立即写回」按钮及其 `.writeback-btn` 样式；
   - 删除 `handleWriteback`、`weekdayLabel`、`weekdayLabels`、以及 setting 类型和默认值里的 `enableWeeklyWriteback / writebackWeekday / writebackHour / lastWeeklyRunAt`；
   - 同步精简页面顶部说明文案，只介绍「每日自动刷新 E 站 Tag + 本地双轨三态管理」。

---

## 三、验证与收尾

1. **后端验证**：`cd backend && go test ./...` 全部通过。
2. **前端验证**：
   - 新增 `bookmarkPrefill.ts` 的单元测试覆盖所有边界条件（两端之外、方向匹配夹心、追新×续扫夹心、不匹配取最近、缺时间兜底）；
   - 运行 `npm run type-check` 和 `npm run build-only`（如需要同步内嵌前端则同步检查，否则遵循仓库既有提交习惯）。
3. **Git 提交与推送**：
   - 确认不碰 `YeonPlans/` 下的未授权文件；
   - 拆分为语义清晰的提交（如书签优化 + 剔除标签写回），`git push` 到 `origin/main`；
   - **不改 `package.json` 版本号、不打 tag、不发 v2.4.0**。
