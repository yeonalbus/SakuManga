# Round14 手柄双击切本 + 离线封面缓存 + 连续阅读计次修复计划

## 背景与需求

| 编号 | 类型 | 内容 |
|------|------|------|
| Opt1 | 优化 | 手柄翻页：连续观看模式下双击「下一页/上一页」配置按键快速确认切换漫画（减少误触/换手） |
| Opt2 | 优化 | 离线界面卡顿（疑似 ZIP/CBZ 封面反复解压，待根因确认已定） |
| Bug1 | Bug | 连续阅读队列中的书目阅读次数不被记录（从「阅读清单」直接点击阅读后 count 不涨） |

---

## 一、Bug1 连续阅读不计阅读次数（根因已确认）

### 现状与根因

- 阅读次数自增入口：`recordComicClick(comicId)`（[comicStore.ts:119](src/stores/comicStore.ts:119)），本地自增 + fire-and-forget 上报 `POST /comics/:id/click`（后端 DB 原子自增，[comic.go:339](backend/internal/handlers/comic.go:339)）；
- **唯一调用点**：[OfflineDetail.vue:295](src/views/offline/OfflineDetail.vue:295)（详情页「立即阅读」按钮）。
- 阅读清单页 `handleRead`（[ReadingListView.vue:31](src/views/ReadingListView.vue:31)）直接 `router.push({ path: "/reader", query: { id, source } })`，**不调用 recordComicClick**；
- `ComicReader.vue` 挂载（[onMounted:878](src/views/ComicReader.vue:878)）与路由 id 切换 watcher（[watch:947](src/views/ComicReader.vue:947)）均**不记录阅读次数** → 从清单进入连续阅读，队列所有书目 count 不涨。

### 修复（统一在阅读器入口计次，避免双计）

**1a. [ComicReader.vue](src/views/ComicReader.vue)**：在 `watch(() => route.query.id, ..., { immediate: true })` 内、`loadComicPages()` 前，对**离线源**调用 `recordComicClick(comicId)`（在线 gid 无 readCount 概念，后端接口也只对 OfflineComic 生效，跳过在线）。
   - 该 watcher 覆盖：清单入口、详情页入口、书架入口、连续切本（router.replace 换 id）→ 每次进入/切换一本都计一次，且只计一次（immediate + id 变化）。
**1b. [OfflineDetail.vue:295](src/views/offline/OfflineDetail.vue:295)**：移除 `recordComicClick(comic.value.id)` 与 `comic.value.readCount + 1` 手动自增（改由阅读器统一计次，防止详情页进入时双计）。
   - 详情页停留时不预增（只有真正进入阅读器才计次），语义更准确。

---

## 二、Opt1 手柄双击快速确认切本（连续观看）

### 现状

- 阅读器读到最后再按「下一页」→ `handleNextInQueue()`（[ComicReader.vue:393](src/views/ComicReader.vue:393)）弹 `modal.confirm`（需鼠标点击或键盘 Enter）；
- 手柄场景：弹出后**必须换手**（摸鼠标/键盘）点「确定」，或无法确认 → 体验割裂。

### 设计

**2a. 双击确认（上/下都支持，D1=B）**：
   - 在最后一页（`currentPage >= totalPages`）且队列有下一本时，快速**双击「下一页」按键**（阈值 500ms 内第二次触发）→ 跳过 modal 直接 `router.replace` 切到下一本；单击仍弹确认框（保留原行为，防止误触）。
   - 在第一页（`currentPage <= 1`）快速**双击「上一页」按键** → 若队列中存在上一本（新增 `getPrevComicInQueue`，取当前 id 在队列中的前一项）→ 直接 `router.replace` 切到上一本；单击保持原「已经是第一页」提示。
   - **配置项（D2=自定义按键，防与翻页键冲突）**：
     - `gamepadDoubleTapConfirm: boolean`（总开关，默认开）；
     - `gamepadDoubleTapWindow: number`（双击时间窗，默认 500ms）；
     - `gamepadConfirmKeys: number[]`（确认切本按键，默认 [A]，可自定义，如用户 B/X 翻页时可另配）；
     - `gamepadCancelKeys: number[]`（取消按键，默认 [B]，供 modal 取消 / 放弃切本）。
   - 实现位置：[useGamepad.ts](src/composables/useGamepad.ts) 的 `onNext/onPrev` 上升沿 → 阅读器侧做双击检测（记录上次触发时间戳）。
   - 队列支持：`readingStore.ts` 新增 `getPrevComicInQueue`（对称实现）。

**2b. modal 手柄确认（D2=A）**：GlobalModal 增加手柄按键监听（`gamepadConfirmKeys`=确认、`gamepadCancelKeys`=取消），单击弹框后也可纯手柄确认/取消，与双击双保险。

---

## 三、Opt2 离线界面卡顿（根因已确认：ZIP/CBZ 封面反复解压）

### 现状与根因

- 离线卡片封面 `coverUrl = /api/v1/comics/:id/cover`（[OfflineHome.vue](src/views/offline/OfflineHome.vue) / ItemCard）；
- 后端 `GetComicCover`（[comic.go:174](backend/internal/handlers/comic.go:174)）：ZIP/CBZ 时调 `GetCoverFromZip`（[cover.go:53](backend/internal/services/cover.go:53)）——**每次请求都 `zip.OpenReader` 重新打开整个压缩包**（如 192MB Archive），扫描中央目录 + 读取第一张完整原图（可能数 MB）返回；
- **无任何缓存**：无 `Cache-Control`/`ETag` 响应头（对比在线 cover-proxy 有 `max-age=86400`）；
- 离线首页 24 张/页 → 翻页/滚动时 24 个并发请求 → 每个请求重复解压大压缩包 → **后端 CPU/IO 打满 → 界面卡顿**。

### 修复（后端缓存优先，前端懒加载兜底）

**3a. 后端封面缓存**（核心）：新增 `services.GetCoverCached(comic)`：
   - 首次为某 comic 生成**缩略图缓存**：从 ZIP/Dir 读封面图 → **解码后等比缩放到 480px 宽**（Go `image` + `jpeg.Encode` 质量 80），写入 `<dataDir>/cover_cache/<id>.jpg`；
   - 后续请求直接 `c.File` 缓存文件（不再解压原包）；
   - 缓存失效：`os.Stat(comic.LocalPath).ModTime()` 与缓存文件 modtime 比较，源更新则重建。
   - 缩略图尺寸可配置（`coverThumbWidth`，默认 480）。
**3b. 响应头**：`GetComicCover` 加 `Cache-Control: public, max-age=86400` + `ETag`（缓存文件 modtime），浏览器二次进入零请求。
**3c. 散图文件夹**：`GetCoverFromDir` 同样走缩略图缓存（原图可能超大，直接 c.File 也卡）。
**3d. 前端**：ItemCard `loading="lazy"` 已有；确认 `decoding="async"` 与 `fetchpriority="low"` 加上（可选，低风险）。

---

## 四、涉及文件

| 文件 | 改动 |
|------|------|
| `src/views/ComicReader.vue` | Bug1 计次 + Opt1 双击检测接入 |
| `src/views/offline/OfflineDetail.vue` | 移除重复计次 |
| `src/composables/useGamepad.ts` | 双击时间戳检测 + 确认/取消按键回调（onConfirm/onCancel） |
| `src/stores/readingStore.ts` | 新增 getPrevComicInQueue（上一本） |
| `src/stores/readerSettings.ts` | 新增 gamepadDoubleTapConfirm / gamepadDoubleTapWindow / gamepadConfirmKeys / gamepadCancelKeys |
| `src/components/settings/ReaderSettings.vue` | 双击开关 + 时间窗 UI |
| `backend/internal/services/cover.go` | GetCoverCached 缩略图缓存（ZIP/Dir 共用） |
| `backend/internal/handlers/comic.go` | GetComicCover 走缓存 + Cache-Control/ETag |
| `scripts/verify-round14.mjs` | 新增验证脚本 |
| `plans/round14-handle-doubletap-cover-cache-plan.md` | 本计划 |

---

## 五、决策确认（用户已确认 2026-08-21）

- **D1 = B**：上/下都支持双击确认切本（新增 getPrevComicInQueue 提供上一本）；
- **D2 = 自定义按键**：提供 gamepadConfirmKeys / gamepadCancelKeys 两个可自定义配置项（默认 A=确认/B=取消），防止与用户自定义翻页键（如 B/X）冲突；同时 GlobalModal 支持手柄 A/B 确认；
- **D3 = A**：封面缩略图固定 480px；
- **D4 = A + 补充**：缓存放后端数据目录 `cover_cache/`；用户补充「离线现在基本是文件夹而非 zip」——卡顿主因修正为**文件夹封面直接输出完整原图（可能数 MB）+ 无缓存**，缩略图缓存对文件夹同样生效。

## 六、验证方案

1. `go test ./...`（封面缓存单测：ZIP 封面生成/命中/源变更重建）；`npm run type-check` + 构建 + 同步 dist；
2. `scripts/verify-round14.mjs`：
   - **Bug1**：清单加入 2 本 → 从清单「立即阅读」进入 → 退出 → 查 `/comics/offline` 两本 readCount=1；详情页进入 → readCount=1（不双计）；
   - **Opt1**：模拟手柄按键（页面内 dispatch Gamepad 事件或直接调用内部 handler）→ 最后一页连按两次 next → 断言直接切换（无 modal）；单击 → modal 出现；
   - **Opt2**：请求封面两次 → 断言第二次响应 `Cache-Control` 命中/第二次请求后 ZIP 未被重新打开（可用封面缓存文件存在 + modtime 断言），并测量首/次请求耗时对比。
3. 回归：Round12/13 验证脚本复跑。

---

## 七、提交计划（按模块拆分）

1. `fix(backend): 离线封面缩略图缓存（ZIP/CBZ 不再反复解压）+ Cache-Control`；
2. `fix(frontend): 连续阅读计次修复 + 手柄双击快速切本`；
3. `chore(build): 同步内嵌前端构建产物到 backend/webui/dist`。

## 实施结果

- Bug1：ComicReader 路由 id watcher 对离线源调用 `recordComicClick`；OfflineDetail 移除重复计次。验证：清单入口 +1、详情入口只 +1 无双计。
- Opt1：readerSettings 新增 `gamepadDoubleTapConfirm/gamepadDoubleTapWindow/gamepadConfirmKeys/gamepadCancelKeys`；readingStore 新增 `getPrevComicInQueue`；useGamepad 新增 onConfirm/onCancel + next/prev 携带触发时间戳；ComicReader 双击检测（边界页 + 时间窗内二次触发 → 强制切本，上/下均支持）；GlobalModal 手柄 A/B 确认取消；ReaderSettings 新增开关/时间窗/确认取消按键录制。
- Opt2：后端 `GetCoverThumb` 缩略图缓存（ZIP/Dir 共用，480px JPEG，源 modtime 失效重建，解码失败/小图回退原图直传）；`GetComicCover` 走缓存 + `Cache-Control: max-age=86400` + ETag/304。单测 `TestCoverThumbCache/TestCoverThumbSmallImageFallback` 通过；验证脚本确认 Cache-Control 生效、两次响应一致。
- 注：webp 封面无法标准库解码 → 回退原图直传（避免加 x/image 依赖）；ZIP 内 JPEG 大图可正常生成缩略图缓存（单测覆盖）。

## 待确认后再开工

请确认 D1~D4 后开始实现。
