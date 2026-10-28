# Round27：搜刮书签

> 状态：规划定稿（珱垣确认四项决策后落盘）
> 发布收尾（版本号/Release Notes/打包/tag）：由珱垣在独立流程处理，本计划不涉及。
> 开发：等待珱垣确认「开工」后执行，不主动启动。

## 一、需求定稿

新增「搜刮书签」：把某次在线浏览的「位置」存成书签，之后一键回到现场。

已确认决策：

| 决策点 | 结论 |
| ---- | ---- |
| 入口位置 | 「🌐 在线模式」侧栏新增「🔖 书签」分组（不放工具组） |
| 书签语义 | A：一签一卡——位置快照 + 锚定一张 ItemCard |
| 创建方式 | 浮动工具栏「存为书签」+ 弹窗内锚定卡片拾取 |
| 跳转方式 | 桌面新标签 / PWA 同标签（沿用 `isStandalonePWA()` 分流） |

功能要点：

- 可配置多个书签，支持自定义名称
- 类型两种：`home`（首页）/ `search`（搜索内容），决定跳转目标
- 快照当时的搜索词 + 全部筛选状态（深拷贝 `onlineSearchConfig`）
- 锚定一张画廊卡片：跳转后列表加载完成自动滚动定位 + 视觉突出标记（常驻角标 + 跳转脉冲动画）

## 二、数据模型

```ts
// src/types/comic.ts 追加（或独立 src/types/bookmark.ts）
export interface ScrapeBookmark {
  id: string            // crypto.randomUUID()
  name: string          // 自定义名称
  type: 'home' | 'search'
  keyword: string       // type=search 时的搜索词（冗余便于侧栏展示）
  config: SearchConfig  // 创建时深拷贝的搜索/筛选快照
  anchor: {
    gid: string
    token?: string
    title?: string
  } | null
  createdAt: number
}
```

## 三、Store：`src/stores/scrapeBookmarksStore.ts`（新增）

- 持久化 localStorage，key `saku_scrape_bookmarks`，仿 `searchStore` 的
  `loadStorage` 恢复 + `watch(deep)` 自动落盘；恢复时逐项校验字段，异常项丢弃
- API：
  - `bookmarks: Ref<ScrapeBookmark[]>`
  - `add(name, type, keyword, config, anchor)` → 返回新书签
  - `remove(id)`、`rename(id, name)`（可选）、`getById(id)`
  - `isGidBookmarked(gid): boolean`（供 ItemCard 角标计算）
  - `bookmarkedGids: ComputedRef<Set<string>>`
  - `snapshotCurrentConfig()`：深拷贝 `onlineSearchConfig`

## 四、创建流程（浮动工具栏 → 弹窗 → 锚定拾取）

1. `FloatingToolbar.vue` 新增 `showBookmark` prop（仅 `/online/home` 传 true），
   菜单加「🔖 存为书签」项，emit `bookmark-create`
2. `OnlineHome.vue` 内挂 `BookmarkCreateModal`（新增组件）：
   - 打开时自动带出：类型（keyword 非空 → `search`，否则 `home`）、
     默认名称（`搜索: <keyword>` / `首页快照 HH:MM`）、搜索词展示
   - 名称可编辑；「选择锚定卡片」按钮 → 进入拾取模式（弹窗收起，
     列表加半透明遮罩提示「点击要锚定的卡片，Esc 取消」）
   - 点击卡片 → 锚定该卡（gid/token/title）→ 书签创建完成 → toast + 卡片即时出现 🔖 角标
   - 支持跳过锚定（纯位置快照，anchor=null）
3. 创建范围限制：仅 `/online/home` 提供入口（本版只支持 home/search 两类书签）

## 五、跳转与恢复（核心链路）

URL 契约：`/online/home?bm=<bookmarkId>`（同标签/新标签共用）

1. 侧栏点击书签（`OnlineSidebar.vue`）：
   - PWA：`router.push({ path: '/online/home', query: { bm: id } })`
   - 桌面：`window.open(router.resolve(...).href, '_blank')`（同 SearchBar 搜索分流）
2. `OnlineHome.vue` setup：**bm 分支优先于现有 kw 分支**：
   - 从 store 取书签 → `onlineSearchConfig.value = deepClone(bookmark.config)`
     （整体替换，**不**走 `applySearchOptionsInherit`——书签要完整恢复当时状态）
   - 现有 `watch(onlineSearchConfig, { deep: true })` 自动触发 `initSearch`
   - bm 参数消费后由既有 `writeKeywordToUrl` 的 replaceState 接管 URL
     （需确认：恢复完成首刷后，把 bm 从 URL 移除/替换为 kw，避免残留无效参数）
3. 锚定定位（`OnlineHome.vue`）：
   - `watch(onlineStore.comics)`（数据就绪）→ `nextTick` →
     `document.querySelector('.item-card[data-gid="<gid>"]')` → `scrollIntoView({ block: 'center' })`
   - 高亮：卡片加 `bookmark-pulse` class（2~3 次呼吸动画后移除）；常驻角标走 props（见六）
4. 时序注意：数据可能分多次到达（`loadMore` 追加），若锚点不在首屏结果，
   需在追加后重试定位（简化：首屏未命中则 toast 提示 + 仅恢复位置）

## 六、ItemCard 视觉突出

- `ItemCard.vue` 新增 prop `bookmarked?: boolean`：
  - 卡片 class 追加 `bookmarked`：金色（#ffc107 系）2px 边框 + 微弱金色辉光
  - 右上角新增 🔖 角标（card 模式放封面右上、避开 fav 星与下载徽标；
    compact 模式放缩略图右上，与 rank-badge 不重叠——rank 在左上）
  - 高亮态与选择模式（selected 粉色框）并存时以选择模式优先（z-index/样式不冲突）
- `GridContainer.vue` 透传：新增 `bookmarkedGids?: Set<string>` prop → 逐卡计算
- 列表页（OnlineHome）从 `scrapeBookmarksStore.bookmarkedGids` 传入
- 跳转脉冲动画：`.bookmark-pulse`（box-shadow 呼吸 2~3 次，动画结束后由页面移除 class）
- 主题适配：浅色主题下金色边框/辉光需同时可读（用 CSS 变量或透明金）

## 七、侧栏 UI（`OnlineSidebar.vue`）

- 「🌐 在线模式」组下新增「🔖 书签」nav-group：
  - 每条：`<button>` 形态（非 router-link，因跳转要拼 query），
    名称 + 类型 emoji（🏠/🔍），hover 显示 ✕（删除，confirm 弹窗防误删）
  - 空态：「暂无书签」灰字
  - 底部「＋ 管理书签」→ 跳 `/settings`（新增「搜刮书签」设置页，可选加分项；
    本期若赶时间可只做侧栏内 hover 删除 + 重命名能力）
- 侧栏 nav-group 现有样式复用（`.nav-group` / `.group-title` / link 类）

## 八、设置页（加分项，可砍）

`SettingsView.vue` 侧栏新增「搜刮书签」项 + `BookmarkSettings.vue`：
- 书签列表（名称/类型/锚定卡片/创建时间），支持改名、删除、点击跳转
- 若为赶 v2.0.0 收尾，本期可降级为「侧栏内管理」，设置页列入后续

## 九、边界与容错

- 锚定卡片不在当前结果（负向过滤命中 / 数据已被移出）→ toast 提示，仅恢复位置
- 书签跳转后用户手动搜索/改筛选 → 覆盖当前配置，书签快照不变（快照非联动）
- localStorage 数据损坏 / 版本不兼容 → 恢复时校验字段并丢弃异常项
- 同一 gid 多书签锚定 → 角标只显示一个（bookmarked 按 gid 集合判断即可）
- keep-alive：`?bm=` 与 `?kw=` 为不同 fullPath，组件会新建实例，逻辑无冲突；
  返回时旧实例由 keep-alive 缓存保留（现有行为不变）

## 十、明确不做（本期范围外）

- 离线模式书签（离线筛选状态快照链路不同，留待后续）
- 热门/订阅/排行榜/收藏/历史页书签（类型暂只 home/search）
- 多卡标签语义（B 方案）
- 书签分组/文件夹/排序/导入导出

## 十一、涉及文件清单

| 文件 | 动作 |
| ---- | ---- |
| `src/types/comic.ts` | 追加 `ScrapeBookmark` 类型（或独立 `src/types/bookmark.ts`） |
| `src/stores/scrapeBookmarksStore.ts` | 新增 |
| `src/components/OnlineSidebar.vue` | 新增「🔖 书签」分组 |
| `src/components/FloatingToolbar.vue` | 新增「存为书签」项 + `bookmark-create` 事件 |
| `src/components/BookmarkCreateModal.vue` | 新增（创建 + 锚定拾取） |
| `src/components/ItemCard.vue` | `bookmarked` prop + 角标/边框/脉冲动画 |
| `src/components/GridContainer.vue` | 透传 `bookmarkedGids` |
| `src/views/online/OnlineHome.vue` | bm 快照恢复 + 锚点定位 + 弹窗挂载 |
| （加分项）`src/components/settings/BookmarkSettings.vue` | 新增管理页 |
| （加分项）`src/views/SettingsView.vue` | 侧栏项 + 路由 tab |

## 十二、里程碑拆解（供开工日排序）

1. M1 数据层：类型 + store + 持久化（可独立测试）
2. M2 创建链路：FloatingToolbar 入口 + BookmarkCreateModal + 锚定拾取
3. M3 跳转恢复：侧栏入口 + OnlineHome bm 恢复 + 定位高亮
4. M4 视觉：ItemCard 角标/边框/脉冲 + GridContainer 透传
5. M5 收尾：边界容错 + 类型检查/构建 + 发布流程（版本号/Notes/打包/tag）
