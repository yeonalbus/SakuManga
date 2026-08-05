# SakuHentai 项目目录树

> 本文件用于快速定位项目文件。已按「前端 Vue 3 + 后端 Go/Gin」分层组织，并给出「功能 → 文件」索引，便于 AI 或新人快速找到需要修改的代码。

## 一、目录总览

```
SakuHentai/
├── backend/                        # Go 后端（Gin + GORM + SQLite）
├── src/                            # Vue 3 前端（Vite + Pinia + Vue Router）
├── public/                         # 静态资源
├── testdata_eh/                    # E 站抓取测试样本（HTML/RSS）
├── 计划书/                          # 项目规划文档
├── 学习笔记/                        # 学习笔记
├── package.json                    # 前端依赖与脚本（type-check / lint / dev / build）
└── PROJECT_TREE.md                 # 本文件
```

---

## 二、前端 `src/`

```
src/
├── main.ts                         # 入口：挂载 Pinia 与 Router
├── App.vue                         # 根组件：布局 + 全局弹窗/Toast 挂载
├── env.d.ts                        # Vite 环境变量类型声明
├── api/
│   └── comic.ts                    # 在线漫画列表 API 封装（OnlineComicResponse）
├── components/
│   ├── FilterDrawer.vue            # 筛选抽屉（分类/评分/页数）
│   ├── FloatingToolbar.vue         # 悬浮操作球
│   ├── GridContainer.vue           # 通用网格容器（items 插槽渲染卡片）
│   ├── ItemCard.vue                # 漫画卡片（card/compact 两种模式）
│   ├── ModeToggle.vue              # 在线/离线模式切换
│   ├── OfflineSidebar.vue          # 离线侧边栏（书架管理）
│   ├── OnlineLoadBar.vue           # 在线加载条（游标加载状态）
│   ├── OnlineSidebar.vue           # 在线侧边栏（导航）
│   ├── Pagination.vue              # 分页组件（离线数字分页）
│   ├── RandomPicker.vue            # 随机挑选
│   ├── ReadingList.vue             # 阅读清单面板（在线/离线队列）
│   ├── SearchBar.vue               # 顶栏搜索框（含标签建议）
│   ├── TagChip.vue                 # 标签胶囊（字典翻译 + 配色）
│   ├── TopBar.vue                  # 顶栏（搜索/筛选/模式切换）
│   ├── common/
│   │   ├── GlobalModal.vue         # 全局弹窗（alert/confirm/prompt）
│   │   └── GlobalToast.vue         # 全局轻提示 Toast
│   └── settings/                   # 设置中心各面板（见下文细分）
├── composables/
│   └── useUI.ts                    # UI 组合式函数：toast / modal（类型安全泛型）
├── config/
│   └── api.ts                      # API_BASE 配置（VITE_API_BASE 或 /api/v1）
├── router/
│   └── index.ts                    # 路由表（含 /not-found 404 兜底）
├── stores/                         # Pinia 状态（按领域拆分，无上帝文件）
│   ├── comicStore.ts               # 在线/离线漫画数据源 + 阅读统计
│   ├── bookshelfStore.ts           # 本地书架 CRUD + 动态数量统计
│   ├── searchStore.ts              # 在线/离线搜索筛选配置（作用域隔离）
│   ├── readingStore.ts             # 阅读清单队列（在线/离线）
│   ├── readerSettings.ts           # 阅读器设置（方向/翻页/界面/持久化 localStorage）
│   ├── historyStore.ts             # 阅读历史 + 收藏状态联动
│   ├── onlineStore.ts              # 在线画廊主列表（游标加载）
│   ├── subStore.ts                 # 订阅列表（OnlineSub 专用）
│   ├── scanPathStore.ts            # 额外扫描路径
│   ├── tagStore.ts                 # 标签字典与翻译
│   └── viewMode.ts                 # 卡片视图模式（card/compact）
├── types/
│   ├── comic.ts                    # 漫画核心契约（BaseComic/Online/Offline/FilterParams...）
│   ├── account.ts                  # 账户类型
│   └── eh.ts                       # E 站设置/UConfig 相关类型
├── utils/
│   ├── request.ts                  # http() 统一请求封装（自动前缀/参数/异常）
│   ├── storage.ts                  # localStorage 安全读写
│   └── mockData.ts                 # 生成 mock 数据（SVG/在线/离线/历史）
└── views/
    ├── ComicReader.vue             # 漫画阅读器（在线/离线、RTL/Webtoon、阅读设置联动）
    ├── DownloadsView.vue           # 下载管理页
    ├── NotFound.vue                # 404 页面
    ├── SettingsView.vue            # 设置中心容器
    ├── online/
    │   ├── OnlineHome.vue          # 在线首页
    │   ├── OnlineFavorites.vue     # 在线收藏夹（0~9 分类 + 游标加载）
    │   ├── OnlineHistory.vue       # 在线历史
    │   ├── OnlineHot.vue           # 在线热门
    │   ├── OnlineSub.vue           # 在线订阅
    │   ├── OnlineTop.vue           # 在线榜单
    │   └── OnlineDetail.vue        # 在线详情（预览/评论/收藏）
    └── offline/
        ├── OfflineHome.vue         # 离线首页
        ├── OfflineBookshelf.vue    # 离线书架
        ├── OfflineDetail.vue       # 离线详情（打分/标签/书架）
        ├── OfflineHistory.vue      # 离线历史
        ├── OfflineMaintain.vue     # 离线书目维护
        ├── OfflineToplist.vue      # 离线排行榜
        └── OfflineUpdate.vue       # 离线更新
```

### 设置面板 `components/settings/` 细分

| 文件                                                                                                                                               | 职责                            |
| -------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------- |
| `AccountSettings.vue`                                                                                                                              | E 站账号登录/登出、Cookie 保存  |
| `EHSettingSettings.vue` / `EHSettings.vue`                                                                                                         | E 站站点偏好                    |
| `ProfileSettings.vue`                                                                                                                              | EH 配置文件（Profile）管理      |
| `MyTagsSettings.vue`                                                                                                                               | 我的标签（关注/隐藏）           |
| `NetworkSettings.vue`                                                                                                                              | 网络/代理配置                   |
| `PreferenceSettings.vue`                                                                                                                           | 偏好设置 + 标签词典同步进度轮询 |
| `ExtraScanPathsSettings.vue`                                                                                                                       | 额外扫描路径管理                |
| `DownloadSettings.vue` / `ReaderSettings.vue`                                                                                                      | 下载 / 阅读器设置               |
| `MouseWheelSettings.vue` / `PerformanceSettings.vue` / `SecuritySettings.vue` / `StyleSettings.vue` / `AdvancedSettings.vue` / `AboutSettings.vue` | 其余设置项                      |

---

## 三、后端 `backend/`

```
backend/
├── main.go                         # 启动入口：DB 初始化 + 代理配置 + Router 装配 + 监听
├── config.json                     # 运行时配置（代理等）
├── go.mod / go.sum                 # Go 模块依赖
├── manga.db                        # SQLite 数据库文件
├── cmd_debug/
│   └── main.go / statusdebug/main.go   # 调试用命令行工具
├── data/                           # 标签词典/计数 缓存数据
└── internal/
    ├── database/
    │   └── db.go                   # SQLite 初始化（GORM）
    ├── models/
    │   ├── models.go               # 通用模型
    │   ├── account.go              # 账户设置模型
    │   ├── eh_setting.go           # E 站设置模型
    │   └── favorite.go             # 收藏模型
    ├── router/
    │   └── router.go               # 全部 API 路由注册（/api/v1 分组）
    ├── handlers/                   # HTTP 处理器（薄层，调用 services）
    │   ├── account.go              # 账户设置
    │   ├── comic.go                # 离线漫画/封面/详情/阅读器
    │   ├── eh_setting.go           # EH 设置/Profile/我的标签/UConfig
    │   ├── eh_uconfig.go           # uconfig.php 代理
    │   ├── favorites.go            # 在线收藏夹（列表/排序/增删）
    │   ├── network_handler.go      # 网络/代理配置
    │   ├── online.go               # 在线画廊列表/详情/预览/热门/订阅/阅读页
    │   ├── scan_path.go            # 扫描路径管理
    │   ├── tag.go                  # 标签引擎状态/同步/建议/进度
    │   └── toplist.go              # 榜单
    └── services/                   # 业务服务层（抓取/解析/引擎）
        ├── eh_types.go             # EHService 定义
        ├── eh_auth.go              # E 站登录认证
        ├── eh_gallery.go           # 在线画廊抓取
        ├── eh_detail.go            # 画廊详情抓取
        ├── eh_parser.go            # HTML 解析
        ├── eh_sub.go               # 订阅抓取
        ├── eh_setting.go           # E 站设置服务
        ├── eh_uconfig.go           # uconfig.php 服务
        ├── cover.go                # 封面生成/代理
        ├── favorites.go            # 收藏服务
        ├── metadata.go             # 元数据
        ├── proxy.go                # 代理配置
        ├── reader.go               # 离线阅读器页/图
        ├── eh_reader.go            # 在线原图 URL 抓取（gdata API + 逐页 fallback）
        ├── scanner.go              # 本地扫描
        ├── tag_engine.go           # 标签翻译引擎（下载/进度/建议）
        ├── toplist.go              # 榜单定时调度
        └── eh_setting_mytags_test.go  # 我的标签测试
```

---

## 四、功能 → 文件快速定位索引

> AI 接到任务时，可先按「要改什么」在下表找到对应文件。

### 前端业务

| 需求                                              | 定位文件                                                                                                                   |
| ------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| 改首页/列表卡片布局                               | `src/views/online/OnlineHome.vue`、`src/views/offline/OfflineHome.vue`、`src/components/ItemCard.vue`、`GridContainer.vue` |
| 改搜索/筛选逻辑                                   | `src/components/SearchBar.vue`、`FilterDrawer.vue`、`stores/searchStore.ts`、`types/comic.ts`（FilterParams）              |
| 改阅读器（在线/离线、翻页/方向/Webtoon/设置联动） | `src/views/ComicReader.vue`、`stores/readerSettings.ts`、`components/settings/ReaderSettings.vue`                          |
| 改在线详情/预览/收藏                              | `src/views/online/OnlineDetail.vue`                                                                                        |
| 改离线详情/打分/标签                              | `src/views/offline/OfflineDetail.vue`                                                                                      |
| 改收藏夹                                          | `src/views/online/OnlineFavorites.vue`                                                                                     |
| 改阅读清单                                        | `src/components/ReadingList.vue`、`stores/readingStore.ts`                                                                 |
| 改历史记录                                        | `stores/historyStore.ts`、`views/online/OnlineHistory.vue`、`views/offline/OfflineHistory.vue`                             |
| 改书架                                            | `stores/bookshelfStore.ts`、`views/offline/OfflineBookshelf.vue`、`components/OfflineSidebar.vue`                          |
| 改请求封装/接口地址                               | `src/utils/request.ts`、`src/config/api.ts`、`src/api/comic.ts`                                                            |
| 改全局弹窗/Toast                                  | `src/composables/useUI.ts`、`components/common/GlobalModal.vue`、`GlobalToast.vue`                                         |
| 改路由/404                                        | `src/router/index.ts`                                                                                                      |
| 改数据契约类型                                    | `src/types/comic.ts`、`types/account.ts`、`types/eh.ts`                                                                    |
| 改设置中心                                        | `src/views/SettingsView.vue` + `components/settings/*`                                                                     |
| 改标签翻译/展示                                   | `stores/tagStore.ts`、`components/TagChip.vue`                                                                             |

### 后端业务

| 需求                       | 定位文件                                                                                        |
| -------------------------- | ----------------------------------------------------------------------------------------------- |
| 加/改 API 路由             | `backend/internal/router/router.go`                                                             |
| 改离线漫画/封面/阅读器接口 | `backend/internal/handlers/comic.go`                                                            |
| 改在线画廊接口             | `backend/internal/handlers/online.go`、`services/eh_gallery.go`、`eh_detail.go`、`eh_reader.go` |
| 改收藏接口                 | `backend/internal/handlers/favorites.go`、`services/favorites.go`                               |
| 改订阅接口                 | `backend/internal/handlers/online.go`（GetWatchedComics）、`services/eh_sub.go`                 |
| 改榜单                     | `backend/internal/handlers/toplist.go`、`services/toplist.go`                                   |
| 改 E 站账号/设置/我的标签  | `backend/internal/handlers/account.go`、`eh_setting.go`、`eh_uconfig.go` + 对应 services        |
| 改标签引擎/词典同步        | `backend/internal/handlers/tag.go`、`services/tag_engine.go`                                    |
| 改数据模型                 | `backend/internal/models/*.go`                                                                  |
| 改数据库初始化             | `backend/internal/database/db.go`                                                               |
| 改扫描路径                 | `backend/internal/handlers/scan_path.go`、`services/scanner.go`                                 |

### 构建 / 校验

| 命令         | 位置                              |
| ------------ | --------------------------------- |
| 前端类型检查 | `npm run type-check`（vue-tsc）   |
| 前端 Lint    | `npm run lint`（oxlint + eslint） |
| 后端编译     | `cd backend && go build ./...`    |
| 后端测试     | `cd backend && go test ./...`     |

---

## 五、分层架构约定

- **前端**：`views`（页面）→ `components`（复用组件）→ `stores`（Pinia 状态，按领域一个文件）→ `utils` / `api`（基础设施）→ `types`（契约类型）。
- **后端**：`handlers`（HTTP 层，负责参数解析与响应）→ `services`（业务逻辑与 E 站抓取）→ `models`（GORM 模型）→ `database`（DB 连接）。路由集中注册在 `router` 包。
- **前后端契约**：前端 `types/comic.ts` 与后端 `handlers` 返回的 JSON 字段需保持一致（例如 `readCount` 统一替代旧的 `clickCount`）。
