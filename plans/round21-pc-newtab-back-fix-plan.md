# Round21 PC 端「返回关闭当前标签页」修复计划（跳转策略平台分流）

## 背景与问题

| 编号 | 类型 | 内容 |
|------|------|------|
| Bug1 | PC | PC 浏览器中点击详情/阅读等页面的「返回」，**当前标签页被直接关闭**，只能靠浏览器回退才能回到正确位置 |
| 策略 | 讨论 | 用户新思路：跳转分流——PC 下仅「首页/菜单」在当前标签页内跳转，内容类（详情、阅读）新建标签页；并需明确「返回」语义（关闭标签 vs 回上一个状态） |

---

## 一、现状与根因

### 跳转链现状（Round16「根治 PWA 逃逸」后的状态）

| 入口 | 现状 |
|------|------|
| 卡片点击（ItemCard.handleCardClick / openDetailNav） | 默认 **SPA 同标签** push 详情；仅中键/Ctrl+点击走 window.open 新标签 |
| 小详情面板「画廊详情 ↗」（OnlineDetailPanel.openFullDetail） | SPA 同标签 push 完整详情 |
| 宽屏在线列表点卡（useDetailPanel.openDetail） | 打开右侧内嵌面板（同标签） |
| 「立即阅读」/预览切片（OnlineDetail / OfflineDetail.handleStartReading） | SPA 同标签 push /reader |
| 阅读清单「阅读」（ReadingListView.handleRead） | SPA 同标签 push /reader |
| 阅读器返回（ComicReader 顶栏「‹ 退出阅读」） | `router.back()` |

### 返回链现状（OnlineDetail / OfflineDetail handleBack 共用结构）

1. `window.opener` 非空 → `window.close()`（**关闭当前标签页**）；
2. `consumeBackState(id)` 命中 → `router.replace(来源)` 并恢复滚动/页码；
3. `isDetailNewTab(id)`（sessionStorage 标记 `saku_newtab_<id>`）→ `window.close()`；
4. `history.length > 1` → `router.back()`，否则回首页。

### 根因（为何 PC 点返回会关掉当前标签页）

- Round16 把「PC 默认新标签打开详情」改成了「一律 SPA 同标签」，但**返回逻辑中面向新标签的两条 window.close() 分支没有同步收紧**，出现上下文错配：
  - **路径①（window.opener）**：任何由 `window.open` 打开的标签（中键/Ctrl 点卡片、TagChip/搜索新标签、旧版本残留标签）内部再做 SPA 跳转（点卡片、画廊详情、阅读）后，`window.opener` 仍非空 → 点返回直接 `window.close()` **关掉整个标签**，丢失标签内全部浏览状态；
  - **路径③（sessionStorage 标记）**：`markComicOpenedInNewTab` 在 `window.open` 前写入**当前标签**，若 `window.open` 被拦截/失败则标记残留，之后同标签 SPA 跳转命中 `isDetailNewTab` → `window.close()`（标准浏览器对非脚本打开标签会忽略，但部分环境行为不一致）。
- 用户侧观感：「之前 PC 自行新标签跳转、直接关标签最方便；现在被改掉了」——即 Round16 的全局同标签化牺牲了桌面习惯，且返回分支没跟上。

---

## 二、方案：跳转策略平台分流 + 返回语义明确

### 核心判定：内容是否新标签

```
contentOpensNewTab = isWideViewport() && !isStandalonePWA()
  isWideViewport = matchMedia('(min-width: 1025px)') && html[data-layout] !== 'mobile'
```

- **PC 桌面浏览器**（宽视口、非 PWA、非强制移动形态）→ 内容类新标签；
- **PWA standalone（iPad/Android 主屏）** → 维持 SPA 同标签（**Round15/16 修复不得回归**）；
- **窄屏浏览器 / 强制移动形态** → SPA 同标签（移动端无多标签习惯）。

### 跳转分流（内容 vs 菜单）

| 动作 | PC 桌面 | PWA / 窄屏 |
|------|---------|-----------|
| 侧边栏/首页/菜单导航（router-link） | 当前标签页 | 当前标签页 |
| 卡片 → 详情（在线/离线/历史/榜单/收藏/订阅/抽卡结果） | **新标签** | 同标签 |
| 面板「画廊详情 ↗」→ 完整详情 | **新标签** | 同标签 |
| 「立即阅读」/预览切片 → 阅读器 | **新标签** | 同标签 |
| 阅读清单 → 阅读器 | **新标签** | 同标签 |
| 离线详情 → 阅读器 | **新标签** | 同标签 |
| 阅读器内「下一本/上一本」连贯阅读 | 同标签（阅读器内已打开，不另开） | 同标签 |

### 返回语义（决策 D2=A+自定义）

| 打开方式 | 返回行为 |
|---------|---------|
| **新标签打开的内容页**（PC） | **优先关闭当前标签页**，但须同时满足：① 当前路由 == 标签入口路由（D5 防误关）；② **opener 存活**（`window.opener && !window.opener.closed`，即存在来源标签，关闭后用户仍有可回页面）。**仅剩一个标签（opener 已关 / 无 opener）→ 不关，改回来源不关**：`consumeBackState → router.replace(来源)` 恢复滚动/页码，无 backState → `history.back()` → 首页兜底 |
| **同标签跳转**（PWA/窄屏/菜单导航） | **回到当前标签页的上一个状态**：`consumeBackState → router.replace(来源)`；无 backState → `history.back()`；再兜底首页 |
| 边界（直接输 URL / 刷新 / 外部进入） | 无 opener、无 backState、无入口标记 → `history.back()` → 首页兜底 |

### 误关防护（决策 D5=A）

- **新标签打开时记录「标签入口路由」**（sessionStorage `saku_tab_entry_<id>` = 打开时的 fullPath）；
- 返回时**仅当「当前路由 == 该标签被打开时的入口路由」且 opener 存活**才允许 window.close()；
- 若标签内继续 SPA 导航到了其他页面（如新标签里点了菜单/另一张卡），返回走 `consumeBackState / history.back()`，**绝不误关**；
- `markComicOpenedInNewTab`（`saku_newtab_`）与入口记录**仅在 `window.open` 成功（返回非 null）后写入**；open 返回 null（弹窗拦截）→ 降级为同标签 SPA 跳转。

---

## 三、涉及文件

| 文件 | 改动 |
|------|------|
| `src/utils/detailNav.ts` | 新增 `contentOpensNewTab()` 判定（宽度 + 布局 + standalone）；`openComicDetailInNewTab` 记录标签入口路由 `saku_tab_entry_<id>`；新增 `consumeTabEntry(id)`；`markComicOpenedInNewTab` 仅在 open 成功后写入；新增统一返回决策 `resolveBackAction(id, opts)`（close-tab / backState / history.back / home 四选一） |
| `src/views/online/OnlineDetail.vue` | `handleBack` 改用统一返回决策（含标签入口匹配）；`handleStartReading` PC 新标签打开阅读器 |
| `src/views/offline/OfflineDetail.vue` | 同上（handleBack + 立即阅读） |
| `src/views/ComicReader.vue` | 顶栏「退出阅读」/错误层「返回」按统一返回决策（新标签→close；同标签→backState/history.back） |
| `src/components/ItemCard.vue` | `openDetailNav`/`handleCardClick` 默认行为按 `contentOpensNewTab()` 分流（PC 新标签，其余同标签） |
| `src/components/OnlineDetailPanel.vue` | 「画廊详情 ↗」PC 新标签 |
| `src/composables/useDetailPanel.ts` | 窄屏分支沿用 SPA；宽屏面板行为按 D1 决策调整 |
| `src/views/ReadingListView.vue` | `handleRead` PC 新标签打开阅读器 |
| `src/views/online/OnlineHome.vue` 等列表页 | `panel-mode` 透传按 D1 决策调整（如需） |
| `scripts/verify-round21.mjs` | 新增验证脚本 |

---

## 四、决策点（请确认）

- **D1 PC 宽屏在线列表的「小详情面板」去留**
  - A（推荐）：保留面板，但普通点击仍开面板（预览）；面板内「画廊详情 ↗」与「立即阅读」改新标签——面板是「预览」不算跳转，符合「内容跳转才新标签」；
  - B：PC 普通点击卡片直接新标签完整详情（放弃面板默认打开，面板仅由操作菜单唤起）；
  - C：移除面板（PC 桌面不再渲染，仅窄屏回退完整详情）。
- **D2 新标签内容页的「返回」语义**
  - A（推荐）：**关闭当前标签页**（来源标签完好、位置保留；桌面最顺手，与旧行为一致）；
  - B：回到来源（在标签内 replace 回来源页，标签不关闭——保留浏览痕迹但多一步）。
- **D3 PC 下「立即阅读/预览→阅读器」是否新标签**
  - A（推荐）：新标签（阅读器是内容页，符合分流原则）；
  - B：同标签（阅读器连贯阅读体验更连续，但会覆盖详情页位置）。
- **D4 新标签判定的视口阈值**
  - A（推荐）：≥1025px 且非 standalone 且非强制移动形态（与现有 isWide 逻辑一致）；
  - B：≥768px（平板横屏浏览器也开新标签，可能误伤）；
  - C：仅按 standalone 判定（窄屏桌面浏览器也新标签，体验差）。
- **D5 误关防护**
  - A（推荐）：标签入口路由匹配 + open 成功后写标记（仅当前页==入口才允许 close）；
  - B：仅 opener 判定（简单但新标签内 SPA 深链仍会误关）。
- **D6 交付方式**（本 Bug 为 v1.4.0 发布后补丁）
  - A（推荐）：修复后重新 build-release.bat 覆盖生成 `SakuHentai.exe`，amend 发行提交并重打 tag（本地尚未推送，历史干净）；
  - B：修复后新增独立 fix 提交 + 新 exe（不移动 1.4.0 tag）。

---

## 五、验证方案

1. `npm run type-check` + `go test ./...` 无回归；`npm run build` + 同步 dist；
2. `scripts/verify-round21.mjs`：
   - 代码级：`contentOpensNewTab` 判定存在；ItemCard/详情/阅读器返回走统一决策；标签入口记录/消费存在；`saku_newtab_` 仅在 open 成功后写入；
   - 回归：Round15/16 的 PWA 同标签逻辑（standalone 分支）不被破坏；
3. 手动验证矩阵（PC Chrome / iPad PWA / 安卓 PWA / 手机浏览器）：
   - PC：点卡 → 新标签详情 → 返回=关标签，来源列表位置不变；新标签内点菜单再返回=不关标签、走 history.back；
   - PC：阅读 → 新标签阅读器 → 返回=关标签；
   - iPad PWA：点卡 → 同标签详情 → 返回=回来源位置（不刷新、不关标签）；
   - 直接输 URL 打开详情 → 返回=history.back/首页兜底。

---

## 六、涉及回归项

- Round15-Bug3 / Round16 / Round17（PWA 返回、逃逸、Android 返回）：standalone 分支保持 SPA 同标签，不回归；
- Round7 历史入口 `resume=1`、返回恢复来源滚动/页码：同标签分支沿用 consumeBackState，不回归；
- 中键/Ctrl 新标签（S10 桌面习惯）：与新的「PC 内容新标签」行为合并，行为一致。

---

## 决策确认（用户已确认 2026-08-22）

- D1=A：保留宽屏小详情面板（预览不算跳转）；面板内「画廊详情 ↗」「立即阅读」改新标签；
- D2=A+自定义：新标签内容页点返回=**关闭标签**，但**多一重判断**——仅当「存在来源标签（opener 存活）」且「当前页==标签入口路由」时才关闭；**只剩一个标签（opener 已关/无 opener）时改为回来源不关**（consumeBackState → 来源页 / history.back）；
- D3=A：PC 下阅读器（立即阅读/预览/清单阅读）新标签打开；
- D4=A：≥1025px 且非 standalone 且非强制移动形态才新标签；
- D5=A：标签入口路由匹配 + `window.open` 成功后才写标记（open 返回 null 时降级同标签跳转）；
- D6=A：修复后重跑 build-release.bat 覆盖 `SakuHentai.exe`，amend 发行提交并重打 `SakuHentai-1.4.0` tag。

## 实施结果

- `detailNav.ts`：新增 `contentOpensNewTab()`（≥1025px + 非 standalone + 非强制移动）、`openContentTab(href,id,force?)`（新标签记录入口路由 `saku_tab_entry_<id>`，open 被拦截/非 PC 降级同标签 `router.push`）、`shouldCloseTab(id,fullPath)`（仅 opener 存活 + 当前页==入口路由才允许 close）；移除旧 `saku_newtab_` 标记与 `isDetailNewTab`。
- 入口分流：ItemCard（在线/离线统一 openDetailNav，PC 新标签）、OnlineDetailPanel「画廊详情↗」、OnlineDetail/OfflineDetail「立即阅读/预览」、ReadingListView「阅读」均走 openContentTab。
- 返回统一：OnlineDetail / OfflineDetail handleBack、ComicReader `handleReaderBack`（退出阅读 + 错误层返回）共用 shouldCloseTab → consumeBackState → history.back → 首页兜底链；PWA/窄屏维持 SPA 同标签（Round15/16 不回归）。
- 验证：type-check、`verify-round21.mjs`、round18/19/20 回归全过；`build-release.bat` 已重打 `SakuHentai.exe`（含本修复）并通过 headless 冒烟。
- **Git 状态说明**：实施期间检测到另一会话并发活动（`a8b4845 Delete`、origin 已推送 `SakuHentai-1.4.0` tag → `4dd7a66`）。为不重写已推送历史，round21 收敛为 `origin/main` 之上的独立提交 `4c79ecd`（71 文件，round21 全部改动）；本地 tag 已对齐 origin（`4dd7a66`）。发行二进制（SakuHentai.exe，gitignored）已含修复。

## 待确认后再开工

决策已全部确认，等待开工指令。
