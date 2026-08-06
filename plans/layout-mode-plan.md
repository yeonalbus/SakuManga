# 样式布局规划：样式设置瘦身 + 布局模式（桌面 / 移动 / 自动）

> 关联 Todo：规划-样式布局 → 优化样式设置 → 打通布局模式 → 移动端组件适配 → 真机验证

## 1. 背景与目标

用户反馈问题3（窄屏适配）：抽卡/筛选组件挤成一团、detail/设置界面显示不全、画廊阅读有问题，窄屏几乎没有适配。

本轮目标分两步：

1. **优化样式设置**：合并「显示样式」与「画廊列表样式」；删除一批无效/冗余选项，消除「设置不生效」的假象。
2. **实现布局模式**：让 `layoutMode` 真正驱动布局，三态 `auto / desktop / mobile`，`auto` 为默认（沿用现有 CSS @media 视口自适应）；`desktop / mobile` 手动强制覆盖。iPad 竖屏（≤767px）归移动布局。

## 2. 现状审计（关键发现）

全库搜索确认以下结论（`downloadGroupCols / downloadGalleryCols / detailThumbCols / moveCoverToRight / globalGalleryStyle / layoutMode` 的消费点）：

| 设置项                   | store 字段            | 真实消费点                                           | 结论         |
| ------------------------ | --------------------- | ---------------------------------------------------- | ------------ |
| 主题颜色                 | -                     | 无（`handleThemeColor` 仅 toast「开发中」）          | 删（占位）   |
| 画廊列表样式(全局)       | `globalGalleryStyle`  | **无消费点**                                         | 删（死选项） |
| 画廊列表样式(页面)       | -                     | 无（`handlePageGalleryStyle` 仅 toast 提示跟随全局） | 删（占位）   |
| 下载页网格布局列数(分组) | `downloadGroupCols`   | **无消费点**（DownloadsView 未用）                   | 删（死选项） |
| 下载页网格布局列数(画廊) | `downloadGalleryCols` | **无消费点**（DownloadsView 未用）                   | 删（死选项） |
| 移动封面图至右侧         | `moveCoverToRight`    | **无消费点**                                         | 删（死选项） |
| 详情页缩略图列数         | `detailThumbCols`     | **无消费点**（Online/OfflineDetail 未用）            | 删（死选项） |
| 布局模式                 | `layoutMode`          | **无消费点**（当前响应式为纯 CSS @media）            | 打通（本轮） |

**真正生效的单一数据源**：卡片/名片切换由 [`viewMode`](src/stores/viewMode.ts:10) store 驱动（[`GridContainer.vue`](src/components/GridContainer.vue:30) `:class="viewMode"`、[`ItemCard.vue`](src/components/ItemCard.vue:46) `props.mode ?? viewMode.value`）。`globalGalleryStyle` 与之功能重复且不生效。

**结论**：删除全部列出的 6 项 + 合并到 `viewMode`，零功能损失；同时修掉了「设置不生效」的隐藏 bug。

## 3. 总体设计：布局模式三态

### 3.1 概念划分（实现的关键）

将现有响应式规则分为两类，采用不同策略：

- **窄屏防溢出（窄屏适配）**：如 FilterDrawer 占满宽度、Pagination 隐藏快捷跳页、ComicReader 安全区、GridContainer 渐进列数、detail/设置页排版。**永远跟随视口 @media**，不随 layoutMode 变化——防止手动 desktop 时窄屏内容撑爆（正是问题3要修的）。
- **布局形态（移动形态）**：App 侧栏「常驻 vs 抽屉」、TopBar 汉堡让位与安全区、是否启用触屏优化显示逻辑。**跟随 layoutMode**（`auto` 时由视口决定；`desktop / mobile` 时强制覆盖）。

> 说明：`auto` 默认 = 现有行为（视口窄 → 移动形态）；手动 `mobile` 让宽视口也应用移动形态（用于预览/平板强制）；手动 `desktop` 让窄视口也保持桌面形态（侧栏常驻等），但**不取消窄屏防溢出**，因此手机上强制 desktop 仍不会挤成一团，只是形态偏桌面。

### 3.2 数据流

```
styleSettings.layoutMode ('auto'|'desktop'|'mobile')
        │
        ▼
useLayoutMode() composable（新增 src/composables/useLayoutMode.ts）
  - computed effectiveLayout：
      auto    → matchMedia('(max-width: 767px)').matches ? 'mobile' : 'desktop'
      desktop → 'desktop'
      mobile  → 'mobile'
  - watch styleSettings.layoutMode / matchMedia change
  - 副作用：document.documentElement.setAttribute('data-layout', effectiveLayout)
        │
        ▼
<html data-layout="desktop|mobile">  ← 与 data-theme 并列（main.ts applyTheme 已写 html 属性，两者并存）
        │
        ▼
CSS 双触发：形态类规则 = @media(视口窄) ∪ html[data-layout='mobile']；
        desktop 强制 = html[data-layout='desktop'] 覆盖 @media 的形态规则（App 侧栏）
```

断点统一取 **767px**（与 [`App.vue`](src/App.vue:355)、[`TopBar.vue`](src/components/TopBar.vue:100) 现有断点一致）。

## 4. 分步实施

### Step 1：优化样式设置（瘦身 + 合并）

- **合并**：删除 [`StyleSettings.vue`](src/components/settings/StyleSettings.vue:46) 的「画廊列表样式(全局)」项与 `globalGalleryStyle` store 字段；「显示样式」segmented 按钮（`viewMode`）保留为唯一入口。
- **删除 6 项 UI**：[`StyleSettings.vue`](src/components/settings/StyleSettings.vue:38) 主题颜色、[`StyleSettings.vue`](src/components/settings/StyleSettings.vue:56) 页面样式、[`StyleSettings.vue`](src/components/settings/StyleSettings.vue:63) 下载分组列数、[`StyleSettings.vue`](src/components/settings/StyleSettings.vue:75) 下载画廊列数、[`StyleSettings.vue`](src/components/settings/StyleSettings.vue:99) 移动封面、[`StyleSettings.vue`](src/components/settings/StyleSettings.vue:87) 详情缩略图列数。
- **同步清理 store**：[`styleSettings.ts`](src/stores/styleSettings.ts:22) 接口 `StyleSettings` 移除 `globalGalleryStyle / downloadGroupCols / downloadGalleryCols / detailThumbCols / moveCoverToRight`；默认值同步；`layoutMode` 类型由 `'desktop'|'mobile'` 改为 `'auto'|'desktop'|'mobile'`，默认 `'auto'`。
- **脚本清理**：[`StyleSettings.vue`](src/components/settings/StyleSettings.vue:137) 删除 `handleThemeColor`、`handlePageGalleryStyle` 两个占位 handler。
- 持久化无迁移负担：`saku_style_settings` 中残留旧字段会随 watch 深写回自动移除（reactive 对象无该 key）。

### Step 2：打通布局模式

- **新增 [`src/composables/useLayoutMode.ts`](src/composables/useLayoutMode.ts)**：实现 3.2 的数据流；`effectiveLayout` 写 `<html data-layout>`；监听 `matchMedia('(max-width: 767px)')`。
- **接入**：在 [`App.vue`](src/App.vue:1) 的 `setup` 调用 `useLayoutMode()`（应用启动即生效，跨路由保持）。
- **App 侧栏形态双触发**：把 [`App.vue`](src/App.vue:355) 的 `@media (max-width: 767px)` 侧栏抽屉规则扩展为「视口窄 ∪ `html[data-layout='mobile']`」双触发；新增 `html[data-layout='desktop']` 时侧栏保持常驻（覆盖 @media，但保留 `.main-content` 窄屏 padding 等防溢出项）。
- **TopBar 形态双触发**：[`TopBar.vue`](src/components/TopBar.vue:100) 安全区/汉堡让位规则加 `html[data-layout='mobile']` 触发。
- **GridContainer 列数（可选，优先级低）**：`html[data-layout='desktop']` 强制 card 4 列 / compact 2 列；`html[data-layout='mobile']` 强制 card 2 列 / compact 1 列；`auto` 保持现有 1024/768/480 渐进。若工作量大可延后，`auto` 已覆盖绝大多数场景。

> 注：FilterDrawer / ReadingList / Pagination / ComicReader 的 @media 属「窄屏防溢出」，本轮不改为形态驱动，保持纯视口。

### Step 3：移动端页面专项适配（按顺序）

> 全部为「窄屏防溢出」类规则（跟随 @media），不涉及 layoutMode。逐页补全 `@media (max-width: 767px)` 适配。

1. **画廊列表层**：[`GridContainer.vue`](src/components/GridContainer.vue:88) 卡片列数/间距复核；[`RandomPicker.vue`](src/components/RandomPicker.vue:258) 抽卡面板窄屏布局（当前挤成一团）；[`FilterDrawer.vue`](src/components/FilterDrawer.vue:573) 抽屉内筛选项排版；[`TopBar.vue`](src/components/TopBar.vue:59) 抽卡按钮/筛选按钮/搜索栏空间分配（窄屏放不下时收纳或换行）。
2. **详情页**：[`OnlineDetail.vue`](src/views/online/OnlineDetail.vue:447) 与 [`OfflineDetail.vue`](src/views/offline/OfflineDetail.vue:280)：封面/信息区单列堆叠、标签区换行、章节/缩略图网格列数下调、操作按钮区自适应。
3. **设置中心**：[`SettingsView.vue`](src/views/SettingsView.vue:44) 与全部 `settings/*.vue` 面板：表单控件最小宽度、分组卡片间距、安全区、长文本省略。
4. **管理页**：[`DownloadsView.vue`](src/views/DownloadsView.vue:172)、[`OfflineUpdate.vue`](src/views/offline/OfflineUpdate.vue:138)、[`OfflineMaintain.vue`](src/views/offline/OfflineMaintain.vue:177) 窄屏卡片/表格/进度条。
5. **阅读器复查**：[`ComicReader.vue`](src/views/ComicReader.vue:1917) 现有 767px 适配复查，重点：翻页按钮触控热区、上下浮动条内容在极窄屏不溢出。

### Step 4：真机验证

- iPad Safari 竖屏（移动形态 + 浅色主题无深色残留）、横屏（桌面形态）；
- 手机 Safari：画廊列表/抽卡筛选/详情/设置/下载/阅读器全流程；
- 手动切换 desktop/mobile 强制形态生效；`auto` 旋转屏幕自动切换。

## 5. 验证清单

- [ ] `npm run type-check` 通过（删除 store 字段后无残留引用）
- [ ] `npm run build` 通过
- [ ] StyleSettings 仅剩：主题模式 / 显示样式 / 布局模式 / 恢复默认
- [ ] 旧 localStorage 无 `downloadGroupCols` 等残留键写回
- [ ] `<html data-layout>` 随布局模式正确切换
- [ ] 真机各页面窄屏无横向滚动、无溢出

## 6. 风险与注意

- **形态 vs 防溢出**的概念划分是本方案的骨架，实施时务必区分，避免「手动 desktop 时把窄屏防溢出也关了」导致回归。
- `data-layout` 与 `data-theme` 同挂在 `<html>`，二者独立、互不影响。
- 删除 store 字段后，`type-check` 能兜底任何遗漏的消费点引用。
- `viewMode`（卡片/名片）与 layoutMode（桌面/移动）是**两个正交维度**，不互相覆盖：移动形态下用户仍可在 card(2 列)/compact(1 列) 间切换。
