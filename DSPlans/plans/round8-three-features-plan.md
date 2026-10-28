# Round8 三项优化计划

## 背景

本轮共三个需求，全部为前端改动，后端无接口变更：

1. **默认收藏夹**：偏好设置新增「默认收藏夹」配置，点击「加入收藏」时按配置走快捷路径。
2. **取消收藏 web 弹窗**：将取消收藏的浏览器 `window.confirm` 替换为项目统一的 web 弹窗。
3. **桌面小详情上边栏布局**：内嵌详情面板（360-420px 窄面板）顶部四个功能按钮改为横向滚动/滑动形式。

---

## 需求 1：默认收藏夹

### 现状

- 收藏相关逻辑全部集中在 [`src/views/online/OnlineDetail.vue`](src/views/online/OnlineDetail.vue:535)：
  - `handleSelectFavorite`（536-572）：`modal.prompt` 输入 0~9 → POST `/comics/online/favorite`
  - `handleRemoveFavorite`（575-602）：`window.confirm` → DELETE `/comics/online/favorite`
  - 长按 700ms 触发取消收藏（`handlePressStart`/`handlePressEnd`，615-626）
  - `handleFavClick`（628-634）：长按则跳过，否则走 `handleSelectFavorite`
- 偏好设置 store：[`src/stores/preferenceSettings.ts`](src/stores/preferenceSettings.ts:25)，localStorage 持久化
- 偏好设置 UI：[`src/components/settings/PreferenceSettings.vue`](src/components/settings/PreferenceSettings.vue:1)

### 改动点

**1a. [`src/stores/preferenceSettings.ts`](src/stores/preferenceSettings.ts:25)**
- `PreferenceSettings` 接口新增字段：`defaultFavFolder: number | null`
- `defaultSettings` 中默认值：`null`
- 无需迁移逻辑（新增字段，旧存储无此键自动回落默认 null）

**1b. [`src/components/settings/PreferenceSettings.vue`](src/components/settings/PreferenceSettings.vue:1)**
- 新增一个 `setting-item`：标题「默认收藏夹」，副标题说明「配置后点击加入收藏将直接收入默认收藏夹」
- 使用下拉 `select`：选项「未配置（手动选择）」（value=`null`）+ Fav 0 ~ Fav 9（value=`0`~`9`）
- 样式复用现有 `.setting-select`

**1c. [`src/views/online/OnlineDetail.vue`](src/views/online/OnlineDetail.vue:535)**
- 抽取公共函数 `setFavorite(idx: number)`：封装原 `handleSelectFavorite` 中的 POST + 状态更新 + `addHistory` + `updateOnlineFavoriteState` + toast 逻辑
- `handleSelectFavorite` 改为：`prompt` 拿到合法 idx 后调用 `setFavorite(idx)`
- `handleFavClick` 改为按配置分流：
  - 未配置（`defaultFavFolder === null`）→ 保持现状（调用 `handleSelectFavorite`）
  - 已配置且未收藏 → 直接 `setFavorite(defaultFavFolder)`，无确认
  - 已配置且已收藏、当前 `favIndex !== defaultFavFolder` → 切换为默认收藏夹（`setFavorite(defaultFavFolder)`），无确认
  - 已配置且已收藏、当前 `favIndex === defaultFavFolder` → 取消收藏（走 `handleRemoveFavorite`，**弹 web 确认框**，已与用户确认）
- 长按取消收藏（`handlePressStart`）逻辑保持不变

### 验证要点
- 未配置默认收藏夹时行为与现在完全一致（prompt 手动输入）
- 配置后：未收藏→直接收藏；已收藏不同夹→切换；已收藏同夹→取消
- 长按始终取消收藏

---

## 需求 2：取消收藏 web 弹窗

### 现状

- [`src/views/online/OnlineDetail.vue`](src/views/online/OnlineDetail.vue:578) 使用 `window.confirm`，属于浏览器原生弹窗，与应用内 `GlobalModal.vue` 风格不一致

### 改动点

**2a. [`src/views/online/OnlineDetail.vue`](src/views/online/OnlineDetail.vue:575)**
- `handleRemoveFavorite` 中：
  ```ts
  const confirmed = await modal.confirm(`确定要从收藏夹移除《${comic.value.title}》吗？`, '取消收藏')
  if (!confirmed) return
  ```
- `modal.confirm` 已存在（[`src/composables/useUI.ts`](src/composables/useUI.ts:114)），返回 `Promise<boolean>`，确认 `true`、取消 `false`

### 备注
- [`src/views/offline/OfflineUpdate.vue`](src/views/offline/OfflineUpdate.vue:221) 也有 `window.confirm`（删除本地记录确认），属另一场景，本次**不改**，仅在此记录。

---

## 需求 3：桌面小详情上边栏布局（含待确认决策点）

### 现状

- 小详情 = 内嵌面板模式（[`src/components/OnlineDetailPanel.vue`](src/components/OnlineDetailPanel.vue:44) 嵌入 `OnlineDetail embedded`），面板宽度 360-420px
- 顶部操作栏 [`OnlineDetail.vue`](src/views/online/OnlineDetail.vue:678) `.top-action-bar` → `.right-actions` 内含 4 个按钮（📑 加入清单 / ❤️ 加入收藏 / 📖 立即阅读 / ⬇️ 下载）
- 问题：`.detail-page.embedded .right-actions`（1096-1101）设 `flex: 1 1 100%; flex-wrap: wrap`，窄面板下 ⬇️ 下载 被挤到第二行
- 参考实现（已存在）：
  - mobile 功能条 `.detail-actions-bar`（[`OnlineDetail.vue`](src/views/online/OnlineDetail.vue:1238)）：`overflow-x: auto; flex-wrap: nowrap` 横向滚动
  - 详情 tab 条 `.detail-tabs`（[`OnlineDetail.vue`](src/views/online/OnlineDetail.vue:1117)）：`overflow-x: auto` 横向滑动

### 最终方案（用户已确认）

- **作用范围**：内嵌小详情面板（embedded）与全页详情页（非 embedded）统一改为横向滚动
- **交互形式**：`.right-actions` 改 `flex-wrap: nowrap; overflow-x: auto`，四个按钮单行横滑
- **滚动条**：隐藏滚动条（`::-webkit-scrollbar { display: none }`），靠鼠标滚轮 / 触控板横滑 / 触摸滑动
- **对齐**：保留现状（embedded 右对齐 `justify-content: flex-end`；非 embedded 由 `top-action-bar` 的 space-between 控制），横向滚动自然覆盖溢出场景

### 改动点

**3a. [`src/views/online/OnlineDetail.vue`](src/views/online/OnlineDetail.vue:1269)**
- 通用 `.right-actions` 增加：
  ```css
  .right-actions {
    display: flex;
    gap: 10px;
    flex-wrap: nowrap;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: none; /* Firefox */
  }
  .right-actions::-webkit-scrollbar {
    display: none; /* Chromium / WebKit */
  }
  .right-actions .action-btn,
  .right-actions .add-reading-btn,
  .right-actions .read-btn {
    flex-shrink: 0;
    white-space: nowrap;
  }
  ```
- **3b.** embedded 覆盖块（1096-1101）同步去掉 `flex-wrap: wrap`，保留 `flex: 1 1 100%; justify-content: flex-end; gap: 8px`，横向滚动由通用 `.right-actions` 规则生效
- **3c.** 需复核全页详情 `top-action-bar`（非 embedded，最大 1100px 内）在正常宽度下 4 按钮是否放得下；若放不下也会自然横滑，行为统一

---

## 执行顺序与验证

1. 需求 1（store → 设置 UI → 交互逻辑）
2. 需求 2（取消收藏 web 弹窗）
3. 需求 3（嵌入式面板滚动布局，按决策点定稿）
4. 验证：`npm run type-check`、`go test ./...`（后端无改动可不跑，但跑一遍更稳）
5. 按 Code 模式规则 git 提交（前端改动综合为一个 commit 或按功能拆分）
