# SakuHentai 十一项问题诊断报告（Round 6）

> 诊断依据：对前端 [`src/`](src/) 与后端 [`backend/`](backend/) 关键源码逐一核对，结合您提供的 404 日志复现路径。每项给出「现象 / 关键代码现状 / 根因」；修复方案与决策项见配套计划书 [`round6-eleven-issues-plan.md`](plans/round6-eleven-issues-plan.md)。

---

## 1. 本地优先加载（在线画廊有本地副本时优先读本地）

- 现象：在线画廊详情始终请求 E 站，即使该画廊已下载到本地。
- 现状：[`OnlineDetail.vue`](src/views/online/OnlineDetail.vue:1) `fetchDetail()` 只调 `GET /comics/online/detail`；后端 [`GetOnlineComicDetail`](backend/internal/handlers/online.go:320) 恒为在线抓取，无本地回退。后端其实已具备按 GID 查本地库的能力（[`AttachDownloadStates`](backend/internal/services/favorites.go:330) 给在线列表挂 `isDownloaded` 绿标），偏好设置 [`PreferenceSettings`](src/stores/preferenceSettings.ts:25) 亦无相关开关。
- 根因：缺「本地优先」数据通路与开关，属功能缺失而非缺陷。

## 2. toplist 与 e-hentai 展示不一致（连本子列表都有差异）

- 现象（经您纠正）：并非评分不一致，而是**榜单画廊列表本身与 E 站原页不一致**——出现/缺失的条目不同、顺序错位。
- 现状：[`toplist.go`](backend/internal/services/toplist.go:129) `fetchToplistPage`：
  - URL：`toplist.php?tl=X&p=N`，X ∈ {11,12,13,15}、每页 50 条；
  - 行选择器 `table.itg tr, div.gl1t, div.gl2t, table.glc tr`（第 163 行）——真实 toplist 结构为 `table.itg.gltc > tr.gtr0/gtr1`，行内列（封面/标题/上传者/评分/页数）与 GLIST 不同；该选择器能否**完整捕获全部条目**未经真实页面验证，是「列表差异」的头号嫌疑；
  - 分页参数递进、每页解析条数与 E 站实际翻页逻辑未对照；
  - 次生问题：评分用 GLIST 的 `.ir` 星级条（第 194 行），toplist 页没有 → 恒为 0，前端 [`ItemCard.vue`](src/components/ItemCard.vue:511) 名片模式 `comic.rating || '5.0'` 显示 5.0；`100000 - globalRank*1250`（第 200 行）为模拟分。
- 根因（待实锤）：URL/行选择器/分页与真实 toplist HTML 失配，导致**条目解析不全或错位**。需用测试账号抓真实 `toplist.php?tl=11&p=1` 页面逐条对照（账号见计划书 S2）。

## 3. 操作菜单「详情页面」按钮无反应

- 现象：跳转 tag/首页/热门/排行榜/收藏后，点「详情页面」无反应；点一张卡片（被识别为跳转、开新标签）后再点按钮才正常。
- 现状：[`FloatingToolbar.vue`](src/components/FloatingToolbar.vue:168) 按钮 emit `detail-toggle` → 页面回调 `togglePanel`。核心在 [`useDetailPanel.ts`](src/composables/useDetailPanel.ts:110)：
  ```ts
  const togglePanel = () => {
    if (isPanelOpen.value) closePanel()
    else if (panelGid.value) { isPanelOpen.value = true; openDetailPanel(...) }
  }
  ```
  当 `panelGid` 为空时是 **no-op**。`panelGid` 只在 `onMounted`（第 51-60 行）按 `route.path` 从 [`detailPanelStore`](src/stores/detailPanelStore.ts) 恢复，且需 `saved?.gid` 存在。
- 复现对照：
  - 步骤1：跳转到新列表页 → 新组件挂载，该 path 无已存 gid → `panelGid=''` → 按钮无反应；
  - 步骤2：点卡片 → `openDetail`（[`useDetailPanel.ts`](src/composables/useDetailPanel.ts:77)）宽屏+面板关闭时 `window.open` 新标签（第 86-90 行），仅写入 `panelGid` 不开面板；
  - 步骤3：再点按钮 → `panelGid` 有值 → 面板弹出。与您的三步复现完全吻合。
- 深层问题：面板状态按 `route.path`（非 fullPath）持久化，而 keep-alive 按 fullPath 缓存，query 变化重建组件时状态可能串位/丢失。

## 4. 搜索栏 backspace 后 tag 补全不触发

- 现象：删空/删部分再输入，联想框「有时」不出现。
- 现状：[`SearchBar.vue`](src/components/SearchBar.vue:56) `watch(keyword)`：
  - 空分支：清 `suggestedTags` 并 **若 store 有词则 `resetToHome()`（第 61-63 行）→ `router.push` 导航**；
  - 非空分支：150ms debounce 请求 `/tags/suggest`，**无过期响应丢弃守卫**（并发竞态：旧请求后到可覆盖新输入结果）；
  - 空分支未 `clearTimeout(suggestTimer)`（清除只在非空分支）；
  - 联想面板被 `isFocused` 门控（模板 `v-if="isFocused && ..."`），导航/失焦即置 false。
- 根因：空态自动跳转（导航干扰）＋ 响应竞态 ＋ `isFocused` 丢失的组合，导致「有时」不触发。删除空态强跳（Issue 9 同改）可去主干扰，再补序号守卫即可根治。

## 5. 失效画廊标记失败（维护查重仍重复在线 404）

- 现象：已 404 的画廊，维护查重每次都重新联网抓取（您提供了真实日志：`在线父子关系发现` → `在线详情拉取失败（跳过在线发现）: ... E 站返回 404`）。
- 现状：[`maintainDedupWithProgress`](backend/internal/services/offline.go:718)：
  - 第 728 行已调 `filterOfflineUpdateEnabled`（第 1080 行）会排除 `RemovedStatus` 漫画；
  - 但 3a 在线发现失败分支（第 866-872 行）：`FetchGalleryDetail` 报错时仅 `log` + `continue`，**既不写 `RemovedStatus/RemovedAt`，也不写 `parent_checked_at`**；
  - `parent_checked_at` 回写（第 915-919 行）只在抓取成功的路径执行 → 404 漫画永不进入增量跳过 → 每次维护都重复联网 404。
- 对比：`checkUpdatesWithProgress`（第 104-116 行）与 `ageCheckWithProgress`（第 284-298 行）**已正确**持久化 `RemovedStatus`——缺口仅在维护查重路径。
- 根因（坐实）：维护查重 3a 失败分支未标记移除、未回写核对时间戳。

## 6. 查重不跨文件夹比较内容

- 现象：下载路径放新版、额外扫描路径放旧版，查重不报警。
- 用户确认：额外路径「离线维护」开关**已开启**，仍不参与对比 → **排除「开关排除路径」这一假设**。
- 现状：规则 1（同 GID）、2（归档 hash）、3（父子关系，[`offline.go`](backend/internal/services/offline.go:828)）、4（文件夹内容签名，第 951 行）作用于 `filterOfflineUpdateEnabled`（第 1080 行）过滤后的全量 `comics`；该过滤仅排除「开关关闭路径 / 已移除 / 已过期」，开关开启的额外路径漫画**确实进入了查重集合**。
- 根因（已核实坐实）：**扫描器只在存在元数据 sidecar 时才写 `GID/Token/ParentGID`**（[`scanner.go`](backend/internal/services/scanner.go:368) 取值自 [`metadata.go`](backend/internal/services/metadata.go:350) 解析的 `.ehdata` / metadata / ComicInfo.xml Web URL / `oldVersionGalleryUrl`）：
  1. 额外路径多为**普通文件夹（无 sidecar）→ `GID=''`、`ParentGID=''`** → 规则 1（同 GID）与规则 3（父子关系）无键可匹配；
  2. 规则 4 只匹配**内容签名完全一致**（第 951 行 `folderSignature`），「旧版 vs 新版」内容不同 → 本就不该命中（该规则定位完全相同的副本，不是版本更新）；
  3. 规则 3a 在线父子发现需 `GID+Token` 且绑定 IPB 账号（`account.IPBMemberID` 非空）才发起，已核对过（`parent_checked_at != 0`）还会增量跳过；
  4. 即：**旧版↔新版识别本质上依赖「父子关系元数据」，额外路径无 GID 即无任何命中路径**。
- 结论：不是开关问题，而是**无 GID 元数据的跨文件夹版本识别缺失**——需新增「无元数据同名匹配 / 在线回填」识别通道（见计划书 D5）。

## 7. 对比界面格式不统一

- 现象：更新对比视图左（本地）右（新版）格式不一致，观感差。
- 现状：[`OfflineCompare.vue`](src/views/offline/OfflineCompare.vue:200)：
  - `type=update`：左 [`OfflineDetailPanel`](src/components/OfflineDetailPanel.vue:1)（紧凑，无 tabs、无评论），右 `OnlineDetail embedded`（完整 tabs：信息/预览/评论）→ 左右格式不统一；
  - `type=maintain`：左右均 `OfflineDetailPanel`（一致）。
- 根因：update 类型混用两套详情组件。

## 8. 滚动条样式

- 现象：暗色模式下出现白色默认滚动条，丑。
- 现状：[`App.vue`](src/App.vue:152) 全局无 scrollbar 样式；`#main-content` 为唯一滚动容器（第 137 行），滚动条为浏览器默认；仅 [`ComicReader.vue`](src/views/ComicReader.vue:1834) 与 [`ItemCard.vue`](src/components/ItemCard.vue:737) 有局部样式。
- 根因：缺少全局主题化 scrollbar CSS。

## 9. 搜索栏逻辑优化

- 现象：
  a) 点击 tag 补全项即跳转（应仅搜索按钮/回车才跳）；
  b) 打开漫画后清空搜索内容触发强跳回首页（离线/在线均有此隐患）。
- 现状：[`SearchBar.vue`](src/components/SearchBar.vue)：
  - 补全项模板 `@click="triggerSearch(...)"` 直接触发搜索与跳转；
  - `watch(keyword)` 空分支 `resetToHome()`（第 61-63 行）→ 置空 store 词 + `router.push` 首页；
  - `handleClearInput`（第 176 行）同样 `resetToHome()`。
- 根因：补全项点击与搜索绑定；空态删除即强跳。

## 10. 合并在线/离线跳转逻辑（详情一律新标签）

- 现状：
  - 在线宽屏+面板关闭：`openDetail` → `window.open` 新标签（[`useDetailPanel.ts`](src/composables/useDetailPanel.ts:86)）；
  - 在线窄屏：`router.push` 同标签（第 93 行）；
  - 离线：`ItemCard` `router.push('/offline/detail')` 同标签。
- 根因：三条路径行为不一致，未统一为新标签。

## 11. 返回按钮优化

- 现象：
  - 在线：返回=浏览器 back；新标签打开后返回失败（本标签 history 无入口页）；
  - 离线：返回回到「第一页第一行」而非记忆位置（首页/首页搜索/书架均复现）。
- 现状：
  - [`OnlineDetail.vue`](src/views/online/OnlineDetail.vue) / [`OfflineDetail.vue`](src/views/offline/OfflineDetail.vue) `handleBack()`：`history.length > 1 ? router.back() : push(home)`——新标签场景失效；
  - 离线列表状态恢复依赖 [`OfflineHome.vue`](src/views/offline/OfflineHome.vue:24) 与 [`OfflineBookshelf.vue`](src/views/offline/OfflineBookshelf.vue:80) 的 `onMounted`+`takeListState`；但 keep-alive 按 fullPath 缓存 → 返回时组件**被复用、不再 onMounted** → `takeListState` 不消费、页码不恢复；共享 `#main-content` 又保留的是详情页滚动 → 表现「回第一页第一行」。
- 根因：恢复逻辑挂在 `onMounted`（仅首次挂载）而非 `onActivated`；`router.afterEach` 的 `restoreScroll` 与列表状态恢复机制并存但未协调。

---

## 结论

- 纯 bug 级：**3、4、5、8、9、11**（可明确修复）；
- 功能/体验新增：**1、7、10**；
- 需真实页面样本佐证：**2**（榜单列表与 E 站不一致）；**6**（已排除开关假设，根因=额外路径无 GID 元数据）。
- 决策项集中在：本地优先呈现策略（D1-D2）、详情页按钮空态行为（D3）、404 直标与重匹配入口（D4）、跨文件夹版本识别实现路径（D5）、对比 tab 策略（D6）、清空按钮行为（D7）、新标签导航（D8）、返回按钮语义（D9）。
