# Round20 四项任务修复计划（历史重复 / PWA 详情跳转 / 抽卡 tag 统一 / 书架「已被删除」报错）

## 背景与问题

| 编号 | 类型 | 内容 |
|------|------|------|
| Bug1 | 离线历史 | 同一本子在历史网格重复显示（如 (2,1)-(2,3) 三格同为本子 A，点击其一后 (1,1) 又出现 A，原三格随机留一格）|
| Bug2 | PWA | iPad PWA 桌面模式，小详情页「画廊详情 ↗」报错并跳转阅读界面报「找不到漫画 / 属于在线资源，已切换到在线模式」；后端日志：gdata 未解析到 file 标签（gid=4045732）回退逐页抓 /s/ |
| Bug3 | 抽卡 tag | 在线 tag 格式 `female:"magical girl$"` 与本地 `female:magical girl` 失配，离线/全库抽卡时离线候选筛不出来 |
| Bug4 | 书架（无法复现）| 特定情况下书架报「本子已被删除 / 找不到该漫画」，无视报错切书架 B 仍报，回首页刷新后才消除 |

---

## 一、Bug1 离线历史重复显示

### 现状与根因

- 离线漫画 ID = **md5(本地路径)**（[scanner.go:18](backend/internal/services/scanner.go) `generateID`），同一画廊可合法存在多个 ID：
  - **更新替换**：`AutoUpdateDeleteOriginal=true` 时 [download.go:317](backend/internal/services/download.go) `finalizeUpdate` 直接 `Delete(&old)` 旧记录；新版本下载到新路径 → 新 md5 → 新记录（同 gid）；
  - **归档升级**：scanner 扫描时压缩包形态被文件夹形态替换（[scanner.go:346](backend/internal/services/scanner.go) 删旧插新）。
- 旧记录被删后，历史表（`HistoryRecord`，后端按 `user_id+comic_id+source` 去重，[library.go:474](backend/internal/handlers/library.go)）中**旧 id 行成为孤儿** → 历史网格同时出现「旧 id 记录 + 新 id 记录」。
- 前端 `addHistory` 按 `comic.id` 去重（[historyStore.ts:147](src/stores/historyStore.ts)）、后端按 `comic_id` 去重，**都无法识别「同 gid 不同 id」的重复**。
- 「点击后 (1,1) 又出现 A」：点击把该条（新 id）unshift 置顶，旧 id 条目仍在原位。

### 修复

1. **后端历史增加 gid 维度**：`HistoryRecord` 加 `Gid` 列（AutoMigrate）；`AddHistory` 入参增加 `gid`（前端离线 comic 已透传 `gid` 字段）；`GetHistory` 支持按 gid 去重。
2. **AddHistory（offline）同 gid 合并**：写入前删除该用户同 gid 的其余行（进度合并到当前行），保证每本每用户仅一条。
3. **GetHistory（offline）去重**：无 gid 时回退按 comic_id 去重，取 `last_read_at` 最新。
4. **删除/替换漫画时引用清理与迁移**（与 Bug4 共用一套服务函数）：
   - 有替换记录（同 gid 新 id）→ history / 书架 comicIds / 离线阅读清单中旧 id **迁移为新 id**；
   - 无替换 → 删除对应 history 行、从书架 comicIds 剔除、从离线阅读清单剔除。
5. **前端双保险**：`loadHistory` / `addHistory` 用 `gid || id` 去重（前端拿不到 gid 时经 `offlineComics` 反查）。

---

## 二、Bug2 PWA「画廊详情」跳转报错

### 现状与根因

**后端（日志已实证）**：
- `FetchOnlinePageUrls`（[eh_reader.go:79](backend/internal/services/eh_reader.go)）主方案 gdata（`api.php` namespace=1→0）解析 `file:` 标签；
- gid=4045732：gdata 返回了完整元数据（`filecount=2`、7 个普通 tag：parody/character/artist/male/female/other:variant set），但**完全没有 file 标签**（日志「标签数=7」即为完整数组）→ namespace=1/0 两次尝试均失败 → 回退逐页抓 /s/；
- 结论：**gdata 的 file 标签并非所有画廊都返回**（variant set 类容器画廊 / expunged 画廊等场景，具体规则需实测确认），当前把 gdata 当主方案在部分画廊上必然慢（10~28s）甚至失败。

**前端（阅读器误判链）**：
- [ComicReader.vue:202](src/views/ComicReader.vue) offline 404 分支：`/^\d+$/.test(realId)` 判为在线 gid 后，**必须有 route.query.token 才自动切在线**；无 token 时直接报「该漫画不在本地库中，且缺少在线画廊 token」；
- 「属于在线资源，已切换到在线模式」= 该分支的 toast（[ComicReader.vue:210](src/views/ComicReader.vue)）——说明当时 reader 以 `source=offline + 纯数字 id` 被打开（来源待真机日志确认：阅读清单快照缺失 source / 历史记录 / back-state 残留），随后在线 pages 又因 gdata 链路失败放大为报错。

**iPad PWA 桌面模式**（≥1025px → 宽屏面板）：
- 「画廊详情 ↗」= SPA push `/online/detail`（Round16 已改同标签，[OnlineDetailPanel.vue:23](src/components/OnlineDetailPanel.vue)）；
- 详情失败（token 缺失 / resolve-token 失败 / 画廊不可用）→ error-state 报错；随后点阅读/预览 → `/reader` → pages 接口 gdata 失败 → 报错链。具体「先报错再跳阅读器」的完整链路需真机日志取证。

### 修复

1. **后端 [eh_reader.go](backend/internal/services/eh_reader.go)**：
   - gdata 明确降级为「尽力而为」快速通道：无 file 标签立即回退（现有行为保留），并把「gdata 有元数据但无 file 标签」场景单独打 Debug 日志（含 namespace/标签样例）便于统计；
   - `fetchPageURLsByHTML` 失败错误可读化（410/403 映射已有，补充限流/会话失效提示）；
   - 新增管理员诊断接口（复用 DiagView）dump 单 gid 的 gdata 原始响应，用于确认 E 站侧规则。
2. **前端 [ComicReader.vue](src/views/ComicReader.vue)**：
   - offline 404 + 纯数字 id：**无 query token 也先 `resolveOnlineToken` 再自动切在线**，失败才报错；
   - 在线 pages 加载失败：明确 toast + 重试/返回按钮，不再误报「找不到漫画」；
   - 接入 errorReporter：记录 `{route.query, source, id, err, listHasId}`。
3. **iPad 复现取证**：给出真机操作清单（面板→画廊详情→阅读→历史入口等路径），结合新增日志确认「source=offline + 数字 gid」的确切来源。

---

## 三、Bug3 离线/在线 tag 筛选统一

### 现状与根因

| 位置 | 在线格式 | 本地匹配 | 结果 |
|------|---------|---------|------|
| OfflineHome 关卡 1/2（[OfflineHome.vue:196](src/views/offline/OfflineHome.vue)）| `female:"magical girl$"` | `t.includes(lowerKw)` 子串 | 失配 |
| 后端 random.go offline 分支（[random.go:149](backend/internal/handlers/random.go)）| 同上 | `tags LIKE "%kw%"`（JSON 元素子串）| 失配 |
| tagFilter.matchExcludes 负向 tag（[tagFilter.ts:80](src/utils/tagFilter.ts)）| 同上 | normalizeTag 后精确比较（引号/$ 保留）| 失配 |
| 联想插入（[RandomView.vue:85](src/views/RandomView.vue)）| offline 用裸 `ns:key`，在线用 f_search | — | 两种格式并存 |

### 修复（本地遵循线上格式）

1. **新增 f_search tag 解析器**（前后端各一份，语义一致）：
   - `parseFSearchTag(raw)` → `{ namespace?, key, anchored($) }`：解析 `ns:"key$"` / `ns:key$` / `ns:key` / 裸 `key`；
   - 规范形 `normalizeForMatch` → `ns:key`（小写、`_`→空格、去引号/锚点）。
2. **离线匹配语义对齐 E 站**：
   - 命中 tag 语法（含 `:`）：有 `$` → tagRaws **精确相等**；无 `$` → key **前缀匹配**（对齐 E 站 `ns:key` 前缀语义）；引号仅作分组；
   - 非 tag（裸词）→ 维持子串（标题/tag）。
3. **改造点**：
   - `src/utils/tagFilter.ts`：新增解析器 + `matchFSearchKeyword`；`matchExcludes` 的 excludeTags 走解析器；
   - `src/views/offline/OfflineHome.vue`：关卡 1/2 关键词先走 tag 解析匹配，非 tag 回退子串；
   - `backend/internal/handlers/random.go` `randomOffline`：keywords/excludeTags 解析后按 JSON 元素精确/前缀匹配（`tags LIKE '%"ns:key"%'`），非 tag 保持子串；
   - `src/views/RandomView.vue`：`useTagSuggest` 格式化**统一为 f_search 语法**（去掉 offline 裸格式分支）——本地遵循线上格式，联想插入、手输、历史配置三种来源同一语义。

---

## 四、Bug4 离线书架「本子已被删除」报错（无法复现）

### 现状与根因（源码已定位）

**报错来源（两处）**：
- [ComicReader.vue:219](src/views/ComicReader.vue) `modal.confirm('该漫画可能已从本地库移除，或本地扫描数据已过期。\n是否刷新离线列表后返回？', '找不到该漫画')` —— `/comics/{id}/pages` 返回 404 时触发（[comic.go:272](backend/internal/handlers/comic.go)）；
- [OfflineDetail.vue:136](src/views/offline/OfflineDetail.vue) toast「找不到该漫画，可能已从本地库移除」 —— `/comics/{id}` 返回 404 时触发（[comic.go:125](backend/internal/handlers/comic.go)）。

**触发条件（与用户描述吻合的合理推断）**：
1. **内存列表过期**：`fetchOfflineComics` 失败被静默吞掉（[comicStore.ts:106](src/stores/comicStore.ts) 仅 console.error）→ `offlineComics` 停留旧数据（iPad 弱网/PWA 长驻更容易）；
2. **服务端换 id/删记录**：更新替换（`finalizeUpdate` 删旧版）或扫描归档升级删旧记录 → 旧 id 点击即 404；书架显示的是新列表、点击的却是内存中残留的旧 id；
3. **弹窗全局粘滞**：`GlobalModal` 挂在 App 级（[App.vue:101](src/App.vue)），跨路由不自动关闭 → 「切书架 B 仍报错」；
4. **首页刷新后消除**：触发 `fetchOfflineComics` 成功 → 列表更新 → 不再 404。

无法复现的原因：需要「旧 id 残留 + 列表未刷新」的窗口期，且依赖服务端扫描/更新时序。

### 修复

1. **404 自愈**：reader / 详情 404 时自动重拉 `fetchOfflineComics`；若 id 仍不存在 → **非阻塞**处理（toast + 从书架/历史/清单剔除该 id），不弹阻塞确认框；
2. **弹窗不粘滞**：`router.afterEach` 全局取消未决 modal（防切页残留）；
3. **fetch 失败可见**：`fetchOfflineComics` 失败 toast 提示 + 可重试；bookshelf `restoreListState` 等待结果并校验；
4. **后端引用清理**（与 Bug1 修复 4 合并）：删除/替换漫画时同步清理 history / 书架 / 阅读清单引用 → 从源头消除旧 id；
5. **诊断上报**：errorReporter 记录该路径（id / source / route / 列表是否含该 id），为后续复现取证。

---

## 涉及文件

| 文件 | 改动 |
|------|------|
| `src/stores/historyStore.ts` | gid 维度：loadHistory / addHistory 按 `gid \|\| id` 去重；syncHistory 传 gid |
| `src/stores/comicStore.ts` | fetchOfflineComics 失败 toast + 重试；404 自愈辅助（按 id 剔除/迁移） |
| `src/views/ComicReader.vue` | offline 404 分支：无 token 先 resolve 再切在线；非阻塞自愈；在线 pages 失败可重试 |
| `src/views/offline/OfflineDetail.vue` | 404 自愈（刷新列表 + 剔除失效项） |
| `src/views/offline/OfflineHome.vue` | 关卡 1/2 tag 解析匹配 |
| `src/views/offline/OfflineBookshelf.vue` | restoreListState 校验 + 失效剔除 |
| `src/utils/tagFilter.ts` | parseFSearchTag / matchFSearchKeyword / matchExcludes 升级 |
| `src/views/RandomView.vue` | 联想统一 f_search 格式 |
| `src/router/index.ts` 或 `src/App.vue` | 路由切换自动取消未决 modal |
| `src/utils/errorReporter.ts` | Bug2/Bug4 诊断上报 |
| `backend/internal/models/models.go` | HistoryRecord 加 Gid 列 |
| `backend/internal/handlers/library.go` | AddHistory 同 gid 合并；GetHistory 去重 |
| `backend/internal/services/offline.go` / `download.go` | 删除/替换漫画时引用清理与历史迁移（新服务函数） |
| `backend/internal/services/eh_reader.go` | gdata 尽力而为 + Debug 日志 + 错误可读化 |
| `backend/internal/handlers/online.go` | （可选）gdata 诊断接口 |
| `backend/internal/handlers/random.go` | offline 分支 tag 解析匹配 |
| `backend/internal/services/tagfilter.go` | Go 版 f_search 解析器（新文件） |
| `scripts/verify-round20.mjs` | 验证脚本 |
| `plans/round20-four-issues-plan.md` | 本计划 |

---

## 决策点（请确认）

- **D1 Bug1 去重键与清理策略**
  - A（推荐）：后端 HistoryRecord 加 gid 列 + AddHistory 同 gid 合并 + GetHistory 去重 + 删除/替换漫画时迁移/清理引用（彻底根治；需 AutoMigrate 加列，旧数据按需回填 gid）；
  - B：仅前端按 `gid||id` 去重显示（改动小，但后端仍存冗余行、孤儿行点击仍 404）。
- **D2 Bug1 孤儿历史（漫画已从本地库删除）**
  - A（推荐）：列表加载/打开时自动剔除（id 不在本地库即移除，列表干净）；
  - B：保留并显示「已失效」角标，点击提示移除（保留阅读痕迹）。
- **D3 Bug2 页列表获取主方案**
  - A（推荐）：gdata 仅作快速通道，缺 file 标签立即回退预览页抓取，回退失败错误可读化；
  - B：完全弃用 gdata，一律抓预览页（更稳，多一次页面抓取，首屏略慢）。
- **D4 Bug2 前端误判链**
  - A（推荐）：offline 404 + 纯数字 id → 无 token 也先 resolveOnlineToken 自动切在线；在线 pages 失败给「重试/返回」；
  - B：仅补日志不动行为（等更多复现信息）。
- **D5 Bug3 无 `$` 时的本地匹配语义**
  - A（推荐）：对齐 E 站——有 `$` 精确、无 `$` 前缀匹配（`female:magical` 命中 `female:magical girl`）；
  - B：一律精确匹配（严格但可能漏筛）。
- **D6 Bug4 弹窗粘滞**
  - A（推荐）：路由切换自动取消未决 modal + 404 改非阻塞自愈；
  - B：仅把确认框改为 toast（最快，但丢失「刷新后返回」入口）。

---

## 验证方案

1. `go test ./...` + `npm run type-check` + 构建 + 同步 dist；
2. `scripts/verify-round20.mjs`：
   - Bug1：mock 后端历史（同 gid 两行）→ 断言 AddHistory 后仅一行、GetHistory 按 gid 去重；
   - Bug3：解析器单测（前后端各一）：`female:"magical girl$"` 精确命中 `female:magical girl`、`female:magical` 前缀命中、`- female:yuri` 正确排除、裸词回退子串；
   - Bug4：404 路径 → 断言不弹阻塞确认框、列表被刷新、modal 随路由关闭；
   - Bug2：代码级断言（gdata 无 file 标签 → 回退预览页；reader 数字 id 无 token → resolve 后切在线）。
3. 回归：round15 / 18 / 19 验证脚本复跑。
4. Bug2/Bug4 真机（iPad PWA）按操作清单复现取证，结合新日志确认链路。

---

## 提交计划（按模块拆分）

1. `fix(backend): 历史 gid 合并去重 + 漫画删除/替换时引用清理与迁移`；
2. `fix(backend): 阅读器页列表 gdata 降级与错误可读化`；
3. `fix(frontend): tag 筛选统一 f_search 语法（本地遵循线上格式）`；
4. `fix(frontend): 书架/阅读器 404 自愈 + 弹窗不粘滞 + PWA 跳转链日志`；
5. `chore(build): 同步内嵌前端构建产物到 backend/webui/dist`。

## 决策确认（用户已确认 2026-08-22）

- D1=A（后端 gid 列 + 合并去重 + 删除/替换时引用清理迁移）；
- D2=A（孤儿历史自动剔除）；
- D3=A（gdata 仅快速通道，缺 file 标签立即回退预览页抓取）；
- D4=A（offline+数字 id 无 token 也 resolve 后自动切在线；在线 pages 失败给重试/返回）；
- D5=A（有 `$` 精确、无 `$` 前缀匹配，对齐 E 站语义）；
- D6=A（404 非阻塞自愈 + 路由切换自动取消未决 modal）。

## 实施结果

- Bug1：`HistoryRecord` 新增 GID 列；`AddHistory` 入参 gid + 同 gid 旧行合并删除 + 缺 gid 回填；`GetHistory` 离线按 gid 去重（多取 2 倍防名额挤占）；新增 `comic_refs.go`（CleanupComicReferences / FindReplacementByGID），接线到 DeleteOfflineComic（查重删除迁移）、finalizeUpdate（更新删旧版清孤儿）、scanner 归档升级（迁移到新文件夹记录）；前端 `OfflineComic.gid` 类型、historyStore 按 gid||id 去重 + 孤儿剔除、libraryInit/历史页先拉列表再载历史。
- Bug2：eh_reader 新增会话失效/限流识别（classifySessionError），抓取错误统一 410/403/401/429 映射；ComicReader 离线 404 + 纯数字 id 无 token 也 `resolveOnlineToken` 自动切在线；新增阅读器错误层（重试/返回）；错误上报接线。**（2026-08-22 用户 iPad PWA 真机验证通过，链路已确认修复）**
- Bug3：前后端 f_search 解析器（tagfilter.go / tagFilter.ts），离线匹配对齐 E 站语义（$ 精确 / 无 $ 前缀），random.go 离线分支 + OfflineHome 关卡 1/2 + matchExcludes 升级，RandomView/FilterDrawer/SearchBar/TagChip 联想/点击插入统一为线上格式（本地遵循线上格式）。
- Bug4：ComicReader/OfflineDetail 404 非阻塞自愈（刷新列表 + purgeOrphanOfflineRefs 清理历史/清单/书架）；router.afterEach 自动取消未决 modal（弹窗不粘滞）；fetchOfflineComics 失败可见 toast。
- 验证：`go test ./...` 全过（新增 tagfilter / comic_refs / history_gid 单测）；`npm run type-check` 过；`verify-round20.mjs`（新增）与 round18/19 回归全过；round13/14/15 为需起本地服务的 Playwright E2E，本环境未运行。
- 构建：`npm run build` + dist 已同步 `backend/webui/dist`。

## 待确认后再开工

决策已全部确认，等待开工指令。
