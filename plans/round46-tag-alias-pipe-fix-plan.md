# Round46：E-H tag 别名/提示后缀（`主 tag | 别名`）污染修复

> 状态：**Step 1–4 完成**（解析修复 + 存量清洗迁移 + 回归用例 + 本机 `go test ./...` 全绿），待用户实机验收（Step 5）。
> 实机部署与验收由用户执行（SakuManga 进程运行在另一台主机，本机无操作权限）。
> 起因：2026-10-26 用户反馈（截图 + E-H 实网对照）：
> 1. 本地卡片语言角标、本地作品标签、Tag 属性云显示 `language:"japanese | definition revised please read the wiki$"`；
> 2. 点击 `artist: gohan pakupaku maru | gpm` 跳转搜索 `artist:"gohan pakupaku maru | gpm$"` → 0 结果；
> 3. 同一作品在 E-H 官网详情页显示完整文本，但 tag 页/卡片/搜索一律用 `|` 左侧，例：`cure answer | anna akechi` → `character:"cure answer$"`。

---

## 一、实测归因

### 1.1 E-H 页面结构（实网抓取，2026-10-26）

`https://e-hentai.org/g/<gid>/<token>/` 的 `#taglist` 中每个 tag：

```html
<div id="td_artist:gohan_pakupaku_maru" class="gt">
  <a id="ta_artist:gohan_pakupaku_maru"
     href="https://e-hentai.org/tag/artist:gohan+pakupaku+maru"
     onclick="return toggle_tagmenu(…,'artist:gohan pakupaku maru',this)">
     gohan pakupaku maru | gpm
  </a>
</div>
```

- `|` 左侧 = 规范 tag key（与 `<a id="ta_…">`、`href="/tag/…"`、`onclick` 参数逐字一致）；
- `|` 右侧 = 展示用别名，或站点提示（`language:japanese` 的 `| definition revised please read the wiki`）；
- 已复查页面：`mopuo oyakata | mopumopu`（`group:mopuo oyakata`）、`takizawa taki | ta ta`（`artist:takizawa taki`）、`japanese | definition revised …`（`language:japanese`）。

### 1.2 搜索对照实验（E-H 实网）

| 搜索式 | 结果数 |
|---|---|
| `artist:"gohan pakupaku maru$"`（左半） | 160 |
| `artist:"gohan pakupaku maru \| gpm$"`（现状整串） | 0 |
| `artist:"gpm$"`（右半别名，E-H 也建了 tag） | 161 |
| `parody:"pokemon$"` / `parody:"pokemon \| pocket monsters$"` | 45,298 / 0 |
| `character:"gudao$"` / `character:"gudao \| ritsuka fujimaru$"` | 13,687 / 0 |
| `parody:"hyperdimension neptunia$"` / 整串 | 1,350 / 0 |

### 1.3 代码根因

| # | 位置 | 事实 |
|---|---|---|
| A | `services/eh_detail.go:111-116` `FetchGalleryDetail` | `tagNode.Text()` 取 `<a>` 显示文本，`|` 及右侧别名/提示一并写入 key |
| B | 同上，选择器 `div[id^='ta_']` | `ta_` 前缀是 `<a>` 的 id，`div` 的 id 是 `td_…` → 该分支永不命中，只剩 `div.gt/div.gtl` 外层 |
| C | `services/tag_maintain.go:309-337` 每日 Tag 刷新 | 以 `FetchGalleryDetail` 结果整体覆盖 `OnlineTags`，脏值随刷新扩散 |
| D | 调用点 | `handlers/online.go:462`（在线详情）、`services/download_gallery.go:126`（下载入库）、`services/extract.go:162`（外部提取）、`services/offline.go:107/298/979/1026`（刷新/导入）、`services/tag_maintain.go:309`（每日刷新） |
| E | 展示/搜索链路 | `TagEngine.TranslateTags`（tag_engine.go:562-568）与前端 `tagStore.translate`（tagStore.ts:157-193）按 `namespace:key` 精确查词典，脏 key 必 miss → 回退原文；`TagChip.vue:109` 点击用脏 key 生成 f_search（tagFilter.ts:193-204 加引号 + `$`）→ 搜索 0 条 |

### 1.4 实机库污染面（`Y:\SakuManga\manga.db` 只读快照，2026-10-26）

| 项 | 数值 |
|---|---|
| `offline_comics` 总数 | 5472 |
| `online_tags` 含 ` \| ` | 1777 条记录 / 2184 个 tag |
| 脏 tag 命名空间分布 | language 803、parody 521、character 442、artist 380、group 38 |
| `language` 干净 key | japanese 0 条（全被覆盖）；chinese 3825、translated 3460、korean 84、english 53 等干净 |
| 旧 `tags` 字段含 ` \| ` | 1065 条 |
| `offline_add_tags` / `offline_remove_tags` | 0 条（用户手动增删未受污染） |
| `xp_tag_stats` / `xp_comic_stats` | 444 / 2292 条脏数据 |
| 词典 `data/db.raw.json` | 干净：`language:japanese` → 日语；脏 key 不命中 |

---

## 二、目标与非目标

### 目标

1. **解析以 `|` 左侧为准**：抓取 E-H 详情页时只入库规范 tag key（`namespace:key`），右侧别名/提示一律丢弃；
2. **与 E-H 行为一致**：tag 页/卡片/搜索均使用规范 key，显示翻译走词典（`cure answer | anna akechi` → `character:cure answer` → 词典命中 → 正常翻译名）；
3. **存量数据自愈**：已入库的历史脏 tag（1777 条记录、1065 条旧 `tags` 字段）在升级重启后自动归一，无需用户手工跑脚本；
4. **统计一致**：清洗后触发 XP 快照重建，词云/推荐恢复干净词条；
5. **零前端改动**：翻译、搜索、角标、词云全部随数据源自动恢复。

### 非目标

- 不保留别名信息（用户确认右侧"丢掉、不关心"）；不支持「按别名离线搜索」（如需，另建别名映射表，后续可选项）；
- 不改 E-H 抓取流程、限流、鉴权；
- 不改 `FetchGalleryDetail` 之外的抓取逻辑；
- 不改前端展示形态（不恢复「主 | 别名」完整显示，E-H 详情页的完整文本仅作抓取输入）。

---

## 三、方案设计

### 3.1 解析层（根治）

新增 `services.parseGalleryTags(doc *goquery.Document) []string`，`FetchGalleryDetail` 改为调用：

1. 遍历 `#taglist tr`，取 `td.tc` 的 namespace（去尾部 `:`，空则 `other`）；
2. 遍历该行 `<a id^='ta_'>`，**优先取 `id`**：去掉 `ta_` 前缀得到 `namespace:key`，`_` → 空格（E-H 的 id 以 `_` 表示空格）；
3. **兜底顺序**：`href` 的 `/tag/<ns>:<key>`（`+` → 空格）→ `onclick` 的 `'ns:key'` → 显示文本按 `|` 截断左半（保留旧口径）→ `div.gt/div.gtl` 文本（最后兜底，兼容历史页面）；
4. 空 key 跳过；同一行去重（保持原顺序）。

### 3.2 存量清洗（升级自愈）

新增 `services.repairTagAliasSuffix(db)`：

- 扫描 `offline_comics` 的 `online_tags` 与 `tags`（JSON 数组），对每个元素：
  - 含 `" | "` → 取 `|` 左侧并 `TrimSpace`；无变化不写库；
- 有变更才 `Update` 对应列（幂等，天然可重跑）；
- 在 `StartTagMaintainScheduler` 启动时执行于 `MigrateLegacyTags` 之后（旧 `tags` 迁移后再统一清洗）；
- 有变更时调用 `XpRebuildAsync("Tag 别名清洗")` 重建 XP 快照；
- 日志输出：修复漫画数 / 修复 tag 数。

### 3.3 回归用例

- `parseGalleryTags`：真实 HTML 片段（含别名、`definition revised` 提示、无 id 的 href 兜底、无别名的普通 tag、多行 namespace）；
- `repairTagAliasSuffix`：sqlite 内存库造脏数据 → 断言归一结果与幂等性；
- `go test ./...` 全绿；前端 `npm run type-check` 不受影响（零改动）。

### 3.4 前端

零改动。修复后：
- TagChip：`character:cure answer` 查词典命中 → 显示翻译名；
- 语言角标：`language:japanese` → 日本語；
- 点击 chip：`character:"cure answer$"` → 在线/离线均可命中；
- XP 词云：干净词条。

---

## 四、实施步骤

| Step | 内容 | 产出 |
|---|---|---|
| 1 | 解析修复 + 单测 | `eh_detail.go` 抽取 `parseGalleryTags`、新增 `eh_tags_test.go` |
| 2 | 存量清洗迁移 + 单测 | `tag_alias_repair.go`、`tag_alias_repair_test.go`，挂载 `tag_scheduler.go` |
| 3 | 本机验证 | `cd backend && go test ./...` |
| 4 | 提交推送 | 解析修复 + 迁移分两条 commit，推 `origin/main` |
| 5 | **用户实机验收**（本机无法操作该主机） | 见下方验收清单 |

### 本机验证记录（2026-10-26）

- 新增用例 `go test ./internal/services/ -run 'TestParseGalleryTags|TestNormalizeTagKey|TestStripTagAliasSuffix|TestRepairTagAliasSuffix'`：6 个用例全 PASS；
- `go test ./...`：`internal/handlers` 4.0s、`internal/services` 12.5s 全 PASS；
- 真实页面回归（临时用例，验证后已删除）：`https://e-hentai.org/g/4283189/ca56e1e35d/` 解析出 9 个规范 tag，`language:japanese`（原显示文本 `japanese | definition revised please read the wiki`）已正确归一，全部结果不含 `|`；
- 前端零改动，未跑 `npm run type-check`。

---

## 五、验收清单（用户实机）

1. 部署新版本重启（启动日志出现 `[tagm] Tag 别名清洗完成：修复 X 个漫画 / Y 个 tag`）；
2. 打开含 `cure answer | anna akechi` 的本地作品 → 「本地作品标签」显示翻译名而非 `|` 原文；
3. 点击任意 tag chip → 搜索框为 `character:"cure answer$"` 形式，结果非 0；
4. 卡片语言角标显示「日本語 / 中文 / English」；
5. Tag 属性云无 `|` 后缀；
6. XP 词云/推荐无 `|` 后缀词条；
7. 次日 6:00 每日 Tag 刷新后再复查 2–6（验证解析修复已生效，不再回写脏数据）。

---

## 六、风险与回滚

| 风险 | 处理 |
|---|---|
| 启动迁移改库失败 | 迁移逐条 `Update` 且记录日志，单条失败不阻断启动；幂等可重跑 |
| 迁移误伤（tag 名本身含 ` \| `） | E-H tag 名不允许 `\|`，且只处理 `" | "`（两侧空格的形态），实测 223 种脏 tag 全部符合 |
| 解析兜底取错 | 取 id 为主，且新增真实页面片段回归用例；异常可回滚该 commit（数据由下次刷新自然恢复） |
| XP 重建耗时 | 沿用既有 `XpRebuildAsync` 异步重建，不阻塞启动 |
| 实机不可逆 | 建议验收前手动备份 `Y:\SakuManga\manga.db`（22 MB） |

---

## 七、变更文件清单（预期）

- `backend/internal/services/eh_detail.go`（解析改为取规范 tag）
- `backend/internal/services/eh_tags_test.go`（新增：解析回归）
- `backend/internal/services/tag_alias_repair.go`（新增：存量清洗迁移）
- `backend/internal/services/tag_alias_repair_test.go`（新增：迁移回归）
- `backend/internal/services/tag_scheduler.go`（启动时挂载迁移 + XP 重建）
