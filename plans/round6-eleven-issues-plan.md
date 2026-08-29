# SakuHentai 十一项问题实施计划书（Round 6）

> 配套诊断报告：[`round6-eleven-issues-diagnostic-report.md`](plans/round6-eleven-issues-diagnostic-report.md)
> 约定：`S1-S11` 对应 11 项；`D1-D9` 为需您拍板的决策点。全部改动完成后按 `.roo/rules/code.md` 走验证 + git 提交。

---

## 一、决策点确认表（D1-D9）

| 编号 | 决策项 | 推荐方案 | 备选 |
|------|--------|----------|------|
| **D1** | 本地优先呈现策略（S1） | A+B 混合：阅读/预览页图用本地文件，元数据与「评论」tab 仍走在线，顶部显示「本地版本」徽章 + 一键切回在线 | A 仅页图用本地；B 完整切本地详情；C 提示条手动切换 |
| **D2** | 新设置项名称与默认值（S1） | 「是否优先加载本地画廊」默认开启 | 默认关闭 |
| **D3** | 无选中内容时「详情页面」按钮行为（S3） | toast 提示「请先点选一张卡片」；同时把面板状态按 fullPath 持久化，跳转后返回列表可恢复最近浏览 | 无反馈 no-op（维持现状） |
| **D4** | 404 直接标记 + 重新匹配入口（S5） | 维护查重 3a 失败即写 `RemovedStatus`（您已认可）；在离线维护页新增「清除移除标记 / 全局重新匹配」入口 | 仅标记，不做重匹配入口 |
| **D5** ✅已选 B | 跨文件夹版本识别的实现路径（S6；开关已确认开启，根因=额外路径无 GID 元数据） | **已选 B 在线回填**：对无 GID 文件夹按标题在线搜索回填 GID/ParentGID/Token，使规则 1/3 完整生效（需 IPB 账号，有 404/限流风险，配合 parent_checked_at 增量与限流）；前置保留「扫描器 sidecar 元数据补提」 | A 本地同名匹配（纯离线，备选）；C 仅元数据提取增强（覆盖最弱） |
| **D6** | 对比界面本地面板 tab 策略（S7） | 左右统一在线详情结构；本地无社区评论时隐藏「评论」tab（仅新版保留）；本地面板保留「信息」tab、「预览」tab 按本地页图渲染 | 本地保持紧凑面板，仅统一视觉 |
| **D7** | 搜索栏清空按钮是否回首页（S9） | 清空 ✕ 仅清空关键词，不导航 | 保留回首页导航 |
| **D8** | 离线详情（及在线窄屏）一律新标签打开（S10） | 是：统一 `window.open` 新标签，入口列表滚动/页码天然保留 | 保持现状（离线同标签） |
| **D9** | 详情页返回按钮语义（S11） | 新标签打开 → 返回=关闭本标签（检测 `window.opener`/sessionStorage 标记）；同标签 → 历史返回 + 列表状态在 `onActivated` 恢复 | 新标签打开 → 返回=跳转入口列表页 |

---

## 二、分项方案与改动点

### S1 本地优先加载（含 D1、D2）

- 后端：
  - [`PreferenceSettings`](src/stores/preferenceSettings.ts:25) 前端新增 `preferLocalGallery: boolean`（默认 `true`），沿用 deep-watch 持久化 `saku_preference_settings`；[`PreferenceSettings.vue`](src/components/settings/PreferenceSettings.vue:1) 在「加载选项」分组加 `.setting-item` + `.toggle-switch` 行。
  - 新增接口 `GET /comics/online/detail` 增强：当开启本地优先且按 `gid` 查到本地 `OfflineComic` 时，响应附加 `local = { comicId, pageCount, coverUrl, localPath, hasComments }`（元数据与评论仍在线抓取）。
- 前端：
  - [`OnlineDetail.vue`](src/views/online/OnlineDetail.vue:1) `fetchDetail()`：若 `preferenceSettings.preferLocalGallery` 且响应带 `local` → 预览/阅读页图改走本地接口 `/comics/:id/pages`；顶部显示「本地版本」徽章 + 手动切回在线按钮；「评论」tab 仍在线拉取。
- 验证：在线打开已下载画廊，确认页图走本地、离线可用、徽章显示。

### S2 toplist 展示对齐（列表差异，需真实样本）

- **抓取真实样本（测试账号）**：`igneous=<E_IGNEOUS>; ipb_member_id=<E_IPB_MEMBER_ID>; ipb_pass_hash=<E_IPB_PASS_HASH>; sk=<E_SK>`（凭据经环境变量注入，勿写死）。抓 `toplist.php?tl=11&p=1`（另取 p=2、tl=12/13/15 各一页）HTML 存档至 [`testdata_eh/eh_toplist_*.html`](testdata_eh/)（后端可用现有带 Cookie 的 HTTP 客户端导出，或临时调试端点落盘），新增解析单测。
- **对照清单（针对「列表本身差异」）**：① 行选择器是否捕获全部条目（真实结构 `table.itg.gltc > tr.gtr0/gtr1`）；② 每页条数与分页参数递进是否与 E 站一致；③ 与 E 站页面逐条比对出现/缺失/错位的条目；④ 再处理次生评分问题（真实「Average: X.XX」→ 5 星映射，连带修正 [`ItemCard.vue`](src/components/ItemCard.vue:511) 恒显示 5.0）。
- 重写 [`fetchToplistPage`](backend/internal/services/toplist.go:129)：按样本修正行选择器/列提取与分页；评分解析真实数值；**移除** `.ir` 假设与 `100000 - rank*1250` 模拟分；补 pageCount / uploader / fileSize。
- 验证：`go test ./...` 断言「条目集合 + 顺序 + 评分」与样本逐条一致；前端卡片字段完整。

### S3 详情面板按钮（含 D3）

- [`useDetailPanel.ts`](src/composables/useDetailPanel.ts:110) `togglePanel()`：
  - `panelGid` 为空 → `toast.info('请先点选一张卡片')`（不静默）；
- [`detailPanelStore.ts`](src/stores/detailPanelStore.ts) 改为按 `route.fullPath` 持久化（兼容迁移旧 key），`onMounted` 恢复逻辑同步改为 fullPath；跳转后返回列表可恢复最近浏览 gid。
- 验证：按您三步复现路径逐帧确认 1/2/3 步骤行为正确。

### S4 搜索补全竞态（Issue 4）

- [`SearchBar.vue`](src/components/SearchBar.vue:56)：
  - 空分支补 `clearTimeout(suggestTimer)`；
  - 非空分支引入请求序号（`suggestSeq`），响应返回时校验序号与当前 `keyword` 一致才写入 `suggestedTags`；
  - 依赖 S9 移除空态强跳后复查 `isFocused` 不再被导航干扰。
- 验证：反复「删空 → 重输」20 次，联想必现。

### S5 失效画廊标记（含 D4）

- [`maintainDedupWithProgress`](backend/internal/services/offline.go:718) 3a 失败分支（第 866-872 行）：
  - `errors.As(err, &services.ErrGalleryUnavailable{...})`，Kind ∈ `removed`/`copyright` → 持久化 `removed_status=true + removed_at=now`，同时回写 `parent_checked_at` 防重复抓；
  - 其他网络错误：仅回写 `parent_checked_at`（增量跳过，避免每次重试），由全量 `forceFull` 再核对。
- 离线维护页（[`OfflineMaintain.vue`](src/views/offline/OfflineMaintain.vue:1)）新增「清除移除标记 / 全局重新匹配」入口（后端对应批量清零 `removed_status` 并触发 `forceFull`）。
- 验证：构造 404 画廊跑维护查重，日志仅首次 404，二次增量跳过；`RemovedStatus` 落库。

### S6 跨文件夹查重（含 D5；已选 B 在线回填）

- 扫描器元数据补提：[`scanner.go`](backend/internal/services/scanner.go:368) 对**额外路径**也尝试从 `.ehdata` / metadata / ComicInfo.xml（Web URL）提取 GID/Token/ParentGID——能提则规则 1/3 自动跨路径生效。
- **D5-B 在线回填**：对 `GID==''` 的额外路径文件夹，维护查重阶段按标题走在线搜索，解析出 `GID/Token/ParentGID` 回写 `offline_comics`；回填成功后由规则 1/3 完成跨路径「新版/旧版」识别。
  - 前置条件：绑定 IPB 账号（`account.IPBMemberID` 非空），未绑定则跳过回填；
  - 回填失败/无网络/非画廊 → 写 `parent_checked_at` 增量跳过 + 计数限制，避免每次维护重复搜索（配合 S5 限流）；
  - 命名冲突/多结果 → 置信度不足时不回填（防误写）。
- 验证：下载路径新版 + 额外路径旧版（无 sidecar）场景回填后由规则 3 报出「旧版被新版取代」。

### S7 对比界面统一（含 D6）

- [`OfflineCompare.vue`](src/views/offline/OfflineCompare.vue:200) `type=update` 左侧由 `OfflineDetailPanel` 改为与右侧一致的在线详情结构组件（复用/抽公共详情组件）：
  - 本地无 `hasComments` → 隐藏「评论」tab；
  - 「预览」tab 用本地页图渲染；
- 验证：update 对比左右视觉一致，评论仅右侧可见。

### S8 全局滚动条

- [`App.vue`](src/App.vue:152) 全局样式：
  - `::-webkit-scrollbar`（8px）+ `::-webkit-scrollbar-thumb`（用 `--app-border-3` / `--app-surface-3-hover` 明暗适配）+ `::-webkit-scrollbar-track`；
  - `scrollbar-width: thin; scrollbar-color: var(--app-border-3) transparent;`（Firefox）；
  - hover 加深；保留现有局部（ItemCard/ComicReader）样式。
- 验证：明暗主题下滚动条颜色、宽度符合主题。

### S9 搜索栏跳转逻辑（含 D7）

- [`SearchBar.vue`](src/components/SearchBar.vue)：
  - tag 补全项点击仅 `keyword.value = tagText`（保留焦点），不调 `triggerSearch`；
  - 仅搜索按钮 / 回车调 `triggerSearch`；历史记录 chip 点击仍可触发搜索（保留）；
  - `watch(keyword)` 空分支与 `handleClearInput`：**去掉 `router.push` 导航**，仅置空 store 关键词（D7 决定 ✕ 按钮是否保留导航）；
- 验证：点补全不跳转；删字不跳首页；在线/离线打开漫画返回后删字不再强跳。

### S10 统一新标签导航（含 D8）

- 抽公共工具 `openComicDetailInNewTab(comic)`：统一 `window.open(router.resolve(...).href, '_blank')`，并写入 sessionStorage 标记（如 `saku_newtab_<gid>=1`）供详情页返回语义判断（S11）。
- [`ItemCard.vue`](src/components/ItemCard.vue) 离线卡片点击 → 新标签；[`useDetailPanel.ts`](src/composables/useDetailPanel.ts:93) 在线窄屏 → 新标签（与宽屏面板关闭行为一致）。
- 验证：在线/离线详情均在浏览器新标签打开，入口列表保持原滚动/页码。

### S11 返回按钮（含 D9）

- 详情页 `handleBack()`（在线/离线）统一逻辑：
  - 由本应用新标签打开（检测 `window.opener` 或 S10 sessionStorage 标记）→ `window.close()`（`window.open` 打开的标签可用）；
  - 否则同标签打开 → `router.back()`（history>1）或 `router.push(home)`。
- 列表状态恢复迁移到 `onActivated`（不只 `onMounted`）：[`OfflineHome.vue`](src/views/offline/OfflineHome.vue:24)、[`OfflineBookshelf.vue`](src/views/offline/OfflineBookshelf.vue:80) 消费 `takeListState` 并在渲染后恢复 `#main-content` 滚动（rAF/nextTick）；与 `router.afterEach` 的 `restoreScroll` 协调（取最大值/后到者生效），避免互相覆盖。
- 验证：离线详情返回落在原页码原行；在线新标签返回关闭标签。

---

## 三、分阶段实施（todo 蓝图）

- 阶段 A（纯 bug，风险低）：S8 滚动条 → S9 搜索栏 → S4 补全竞态 → S3 详情按钮。
- 阶段 B（后端标记/查重）：S5 失效画廊标记 → S6 跨文件夹查重（待 D5）。
- 阶段 C（跳转与返回联动）：S10 新标签统一 → S11 返回按钮 + 列表状态恢复（待 D8/D9）。
- 阶段 D（功能新增）：S1 本地优先（待 D1/D2）→ S7 对比统一（待 D6）→ S2 toplist 对齐（需真实页面样本）。

## 四、验证与回归清单

- 后端：`cd backend && go test ./...`（含 toplist/offline_removed 新单测）。
- 前端：`npm run type-check` 或 `npm run build`；按 S3 三步复现、S4 反复输入、S9 跳转、S10/S11 新标签与返回逐项手测。
- 明暗主题冒烟：S8 滚动条、S1 徽章在两种主题下正常。
- 验证通过后按 `.roo/rules/code.md` 拆分 commit（后端/前端可分提交）。

## 五、风险与注意

- S2 依赖真实 toplist 样本（已提供测试账号）；抓取/解析变更需回归其余榜单类型（11/12/13/15）与分页。
- S6：已选 D5-B 在线回填，会对大量无 GID 文件夹发起标题搜索 → 增加 404/限流风险，需配合 S5 的 `parent_checked_at` 增量跳过与限流；回填失败需降级（不重复搜），命名多结果需置信度门槛防误写。
- S10 新标签改变用户既有「同标签返回」习惯，返回语义（D9）需一并上线，避免半成品。
- S3 面板状态按 fullPath 持久化属数据结构变更，注意旧 key 兼容/清理。
