# SakuHentai 项目目录树

> 本文件用于快速定位项目文件。已按「前端 Vue 3 + 后端 Go/Gin」分层组织，并给出「功能 → 文件」索引，便于 AI 或新人快速找到需要修改的代码。
>
> 版本：v0.1 · 最近更新：2026-08

## 一、目录总览

```
SakuHentai/
├── backend/                        # Go 后端（Gin + GORM + SQLite，含内嵌前端 webui/）
├── src/                            # Vue 3 前端（Vite + Pinia + Vue Router）
├── public/                         # 静态资源（favicon 等）
├── testdata_eh/                    # E 站抓取测试样本（HTML）
├── JHentai/                        # Dart 爬虫参考实现（E-Hentai 解析/下载）
├── plans/                          # 功能开发方案文档（下载/标签维护/多用户）
├── 计划书/                          # 项目规划文档（已弃用）
├── 学习笔记/                        # 学习笔记（已弃用）
├── .roo/                           # Roo 自定义配置
├── package.json                    # 前端依赖与脚本（type-check / lint / dev / build）
├── vite.config.ts                  # Vite 构建与代理配置
├── eslint.config.ts / .oxlintrc.json / .prettierrc.json   # 代码规范配置
├── build-release.bat               # 一键打包单 exe（前端构建 → 拷贝 dist → go build）
├── dist/                           # Vite 构建产物（已 gitignore，供后端内嵌使用）
├── SakuHentai.exe                  # 打包产物（已 gitignore，双击运行即托盘）
└── PROJECT_TREE.md                 # 本文件
```

---

## 二、前端 `src/`

```
src/
├── main.ts                         # 入口：Pinia + Router + 主题应用 + 会话恢复 + 401 全局监听
├── App.vue                         # 根组件：布局（侧边栏/顶栏/主区）+ 全局 Toast/Modal 挂载
├── env.d.ts                        # Vite 环境变量类型声明
│
├── api/                            # 领域 API 封装（薄层，基于 utils/request）
│   └── comic.ts                    # 在线漫画列表 / 随机抽卡 API 封装
│
├── components/                     # 通用 UI 组件
│   ├── ItemCard.vue                # 漫画卡片（card/compact 两种模式）
│   ├── GridContainer.vue           # 通用网格容器（items 插槽渲染卡片）
│   ├── FilterDrawer.vue            # 筛选抽屉（分类/评分/语言/页数）
│   ├── SearchBar.vue               # 顶栏搜索框（含标签建议）
│   ├── TopBar.vue                  # 顶栏（搜索/筛选/阅读清单/模式切换）
│   ├── ModeToggle.vue              # 在线/离线模式切换
│   ├── OnlineSidebar.vue           # 在线侧边栏（导航菜单）
│   ├── OfflineSidebar.vue          # 离线侧边栏（书架管理）
│   ├── OnlineLoadBar.vue           # 在线加载条（游标加载状态）
│   ├── Pagination.vue              # 分页组件（离线数字分页）
│   ├── RandomPicker.vue            # 随机抽卡界面
│   ├── ReadingList.vue             # 阅读清单面板（在线/离线队列）
│   ├── FloatingToolbar.vue         # 悬浮操作球
│   ├── TagChip.vue                 # 标签胶囊（字典翻译 + 配色）
│   ├── common/
│   │   ├── GlobalModal.vue         # 全局弹窗（alert/confirm/prompt）
│   │   └── GlobalToast.vue         # 全局轻提示 Toast
│   └── settings/                   # 设置中心各面板（见下方细分）
│
├── composables/
│   └── useUI.ts                    # UI 组合式函数：toast / modal（类型安全泛型）
│
├── config/
│   └── api.ts                      # API_BASE（VITE_API_BASE 或 /api/v1）+ TOKEN_KEY
│
├── router/
│   └── index.ts                    # 路由表 + 登录守卫 + 滚动记忆（404 兜底）
│
├── stores/                         # Pinia 状态（按领域拆分，无上帝文件）
│   ├── userStore.ts                # 登录会话（token/user/登录/登出/恢复/401 清理）
│   ├── modeStore.ts                # 在线/离线模式单一数据源
│   ├── libraryInit.ts              # 登录后用户库数据初始化 + 旧 localStorage 数据迁移
│   ├── bookshelfStore.ts           # 本地书架 CRUD + 动态数量统计
│   ├── historyStore.ts             # 阅读历史（在线/离线）+ 收藏状态联动
│   ├── readingStore.ts             # 阅读清单队列（在线/离线）
│   ├── ratingStore.ts              # 个人评分映射（1-5 星，按用户隔离）
│   ├── comicStore.ts               # 在线/离线漫画数据源 + 阅读统计
│   ├── searchStore.ts              # 在线/离线搜索筛选配置（作用域隔离）
│   ├── onlineStore.ts              # 在线画廊主列表（游标加载）
│   ├── subStore.ts                 # 订阅列表（OnlineSub 专用）
│   ├── scanPathStore.ts            # 额外扫描路径
│   ├── tagStore.ts                 # 标签字典与翻译
│   ├── viewMode.ts                 # 卡片视图模式（card/compact）
│   ├── styleSettings.ts            # 样式设置（主题/视图模式，localStorage 持久化）
│   ├── readerSettings.ts           # 阅读器设置（方向/翻页/界面/持久化）
│   ├── preferenceSettings.ts       # 偏好设置（localStorage 持久化）
│   ├── networkSettings.ts          # 网络/代理设置
│   ├── downloadSettings.ts         # 下载设置
│   └── advancedSettings.ts         # 高级设置
│
├── types/                          # 数据契约类型
│   ├── comic.ts                    # 漫画核心契约（BaseComic/Online/Offline/FilterParams...）
│   ├── account.ts                  # 账户类型
│   ├── eh.ts                       # E 站设置/UConfig 相关类型
│   └── user.ts                     # 用户信息/登录结果类型
│
├── utils/
│   ├── request.ts                  # http() 统一请求封装（自动前缀/参数/token/401/超时）
│   ├── storage.ts                  # localStorage 安全读写
│   ├── scrollMemory.ts             # 路由切换滚动位置记忆
│   └── mockData.ts                 # 生成 mock 数据（SVG/在线/离线/历史）
│
└── views/                          # 页面级组件
    ├── ComicReader.vue             # 漫画阅读器（在线/离线、RTL/Webtoon、阅读设置联动）
    ├── DownloadsView.vue           # 下载管理页
    ├── LoginView.vue               # 登录页
    ├── MemberHistory.vue           # 成员历史（管理员，查看任意成员阅读记录）
    ├── SettingsView.vue            # 设置中心容器（11 个面板按角色过滤）
    ├── NotFound.vue                # 404 页面
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
        ├── OfflineMaintain.vue     # 离线书目维护（查重/移除）
        ├── OfflineToplist.vue      # 离线排行榜
        └── OfflineUpdate.vue       # 离线更新检测
```

### 设置面板 `components/settings/` 细分

| 文件                      | 职责                                      | 管理员可见 |
| ------------------------- | ----------------------------------------- | :--------: |
| `AccountSettings.vue`     | E 站账号登录/登出、Cookie 保存            |     —      |
| `EHSettings.vue`          | E 站站点偏好（Profile/uconfig/我的标签）  |     —      |
| `StyleSettings.vue`       | 样式/主题/卡片视图模式                    |     —      |
| `ReaderSettings.vue`      | 阅读器方向/翻页/界面                      |     —      |
| `PreferenceSettings.vue`  | 偏好设置 + 标签词典同步进度轮询           |     —      |
| `NetworkSettings.vue`     | 网络/代理配置                             |     ✔      |
| `DownloadSettings.vue`    | 下载设置                                  |     ✔      |
| `TagMaintainSettings.vue` | Tag 维护（双轨三态：设置/刷新/写回/进度） |     ✔      |
| `AdvancedSettings.vue`    | 高级设置（服务器/扫描路径等入口）         |     ✔      |
| `SecuritySettings.vue`    | 安全设置（用户管理/密码）                 |     ✔      |
| `AboutSettings.vue`       | 关于软件                                  |     —      |

---

## 三、后端 `backend/`

```
backend/
├── main.go                         # 启动入口：--headless 双模式、切换 exe 目录、自动端口、托盘、内嵌前端
├── config.json                     # 运行时配置（本机代理地址等）
├── go.mod / go.sum                 # Go 模块依赖
├── manga.db                        # SQLite 数据库文件（运行时生成，勿入库）
│
├── webui/                          # 内嵌前端（单 exe 打包）
│   ├── embed.go                    # go:embed all:dist + SPA 回退静态服务（/api 404 不吞错）
│   ├── favicon.ico                 # 前端图标副本
│   └── dist/                       # Vite 构建产物（build-release.bat 生成，含占位 index.html）
│
├── cmd_debug/                      # 调试用命令行工具（独立 main）
│   ├── main.go                     # 调试入口
│   ├── statusdebug/main.go         # 服务/账号状态调试
│   ├── readerdebug/main.go         # 阅读器/页图调试
│   ├── schemadump/main.go          # 数据库表结构导出
│   └── archivedebug/main.go        # 压缩包解析调试
│
├── data/                           # 标签词典/计数缓存（运行时下载生成，勿入库）
│   ├── db.raw.json + .etag         # 标签词典原始数据 + 校验
│   └── tagname_count.csv.gz + .etag # 标签热度计数 + 校验
│
└── internal/
    ├── database/
    │   └── db.go                   # SQLite 初始化（GORM AutoMigrate）+ 旧表结构迁移
    ├── models/                     # GORM 数据模型
    │   ├── models.go               # 通用模型（Base/枚举/常量）
    │   ├── account.go              # 账户设置模型
    │   ├── user.go                 # 用户/会话模型
    │   ├── server_setting.go       # 服务器设置模型
    │   ├── eh_setting.go           # EH 设置/Profile 模型
    │   ├── favorite.go             # 收藏状态模型
    │   ├── comic_rating.go         # 个人评分模型
    │   ├── reading_list.go         # 阅读清单模型
    │   └── download.go             # 下载任务/设置模型
    ├── middleware/
    │   └── auth.go                 # AuthRequired / AdminOnly / CurrentUser
    ├── router/
    │   └── router.go               # 全部 API 路由注册（/api/v1 分组：公开/登录/管理员）
    ├── tray/                       # 系统托盘（Windows）
    │   ├── tray_windows.go         # systray：打开界面/退出程序（go:build windows）
    │   ├── tray_other.go           # 非 Windows 空实现（headless，go:build !windows）
    │   └── icon.ico                # 托盘图标
    ├── handlers/                   # HTTP 处理器（薄层，参数解析 + 调用 services）
    │   ├── auth.go                 # 登录/登出/当前用户（auth/me）
    │   ├── account.go              # 账户设置（E 站凭证）
    │   ├── user.go                 # 用户管理（CRUD/重置密码，管理员）
    │   ├── server.go               # 服务器设置（监听地址/端口/历史上限）
    │   ├── comic.go                # 离线漫画/封面/详情/删除/页图
    │   ├── library.go              # 书架/历史/评分/阅读清单（按用户隔离）
    │   ├── online.go               # 在线画廊列表/详情/预览/热门/订阅/随机/封面代理
    │   ├── favorites.go            # 在线收藏（列表/排序/增删）
    │   ├── toplist.go              # 在线榜单
    │   ├── download.go             # 下载任务（创建/列表/GP/设置/暂停/恢复/取消）
    │   ├── offline.go              # 离线更新检测 + 维护查重
    │   ├── scan_path.go            # 扫描路径管理
    │   ├── tag.go                  # 标签引擎状态/建议/词典/同步
    │   ├── tag_maintain.go         # Tag 维护（设置/刷新/写回/进度/单本编辑）
    │   ├── eh_setting.go           # EH 设置/Profile/我的标签/uconfig 代理
    │   ├── eh_uconfig.go           # uconfig.php 代理
    │   └── network_handler.go      # 网络/代理配置
    └── services/                   # 业务服务层（抓取/解析/引擎/调度）
        ├── eh_types.go             # EHService 定义 + DTO/搜索参数类型
        ├── eh_auth.go              # E 站登录认证
        ├── eh_gallery.go           # 在线画廊列表抓取
        ├── eh_detail.go            # 画廊详情抓取
        ├── eh_parser.go            # HTML 解析（goquery）
        ├── eh_sub.go               # 订阅抓取
        ├── eh_reader.go            # 在线原图 URL 抓取（gdata API + 逐页 fallback）
        ├── eh_random.go            # 在线随机页采样
        ├── eh_setting.go           # E 站设置服务
        ├── eh_uconfig.go           # uconfig.php 服务
        ├── auth_service.go         # 登录认证服务（密码哈希/会话）
        ├── bootstrap.go            # 初始管理员创建 + 数据迁移
        ├── proxy.go                # 代理配置加载
        ├── cover.go                # 封面生成/代理
        ├── metadata.go             # 本地漫画元数据解析（zip/xml）
        ├── extract.go              # 压缩包解压
        ├── reader.go               # 离线阅读器页/图
        ├── scanner.go              # 本地扫描
        ├── scan_manager.go         # 扫描任务管理器（内存态进度，异步轮询）
        ├── favorites.go            # 在线收藏服务
        ├── toplist.go              # 榜单定时调度
        ├── download.go             # 下载管理器
        ├── download_archive.go     # 下载压缩包归档（最大文件，951 行）
        ├── download_gallery.go     # 下载画廊抓取
        ├── download_scheduler.go   # 下载调度
        ├── offline.go              # 离线更新/查重
        ├── tag_engine.go           # 标签翻译引擎（下载/进度/建议）
        ├── tag_maintain.go         # Tag 维护服务（743 行）
        ├── tag_scheduler.go        # Tag 维护定时调度
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
| 改历史记录                                        | `stores/historyStore.ts`、`views/online/OnlineHistory.vue`、`views/offline/OfflineHistory.vue`、`views/MemberHistory.vue`  |
| 改书架                                            | `stores/bookshelfStore.ts`、`views/offline/OfflineBookshelf.vue`、`components/OfflineSidebar.vue`                          |
| 改登录/会话                                       | `views/LoginView.vue`、`stores/userStore.ts`、`main.ts`（会话恢复）、`utils/request.ts`（401）                             |
| 改请求封装/接口地址                               | `src/utils/request.ts`、`src/config/api.ts`、`src/api/comic.ts`                                                            |
| 改全局弹窗/Toast                                  | `src/composables/useUI.ts`、`components/common/GlobalModal.vue`、`GlobalToast.vue`                                         |
| 改路由/404/登录守卫                               | `src/router/index.ts`                                                                                                      |
| 改数据契约类型                                    | `src/types/comic.ts`、`types/account.ts`、`types/eh.ts`、`types/user.ts`                                                   |
| 改设置中心                                        | `src/views/SettingsView.vue` + `components/settings/*`                                                                     |
| 改标签翻译/展示                                   | `stores/tagStore.ts`、`components/TagChip.vue`                                                                             |
| 改用户数据初始化/迁移                             | `stores/libraryInit.ts`                                                                                                    |

### 后端业务

| 需求                       | 定位文件                                                                                              |
| -------------------------- | ----------------------------------------------------------------------------------------------------- |
| 加/改 API 路由             | `backend/internal/router/router.go`                                                                   |
| 改离线漫画/封面/阅读器接口 | `backend/internal/handlers/comic.go`                                                                  |
| 改在线画廊接口             | `backend/internal/handlers/online.go`、`services/eh_gallery.go`、`eh_detail.go`、`eh_reader.go`       |
| 改收藏接口                 | `backend/internal/handlers/favorites.go`、`services/favorites.go`                                     |
| 改订阅接口                 | `backend/internal/handlers/online.go`（GetWatchedComics）、`services/eh_sub.go`                       |
| 改榜单                     | `backend/internal/handlers/toplist.go`、`services/toplist.go`                                         |
| 改 E 站账号/设置/我的标签  | `backend/internal/handlers/account.go`、`eh_setting.go`、`eh_uconfig.go` + 对应 services              |
| 改标签引擎/词典同步        | `backend/internal/handlers/tag.go`、`services/tag_engine.go`                                          |
| 改 Tag 维护                | `backend/internal/handlers/tag_maintain.go`、`services/tag_maintain.go`、`tag_scheduler.go`           |
| 改下载系统                 | `backend/internal/handlers/download.go`、`services/download*.go`                                      |
| 改书架/历史/评分/阅读清单  | `backend/internal/handlers/library.go`                                                                |
| 改用户管理/权限            | `backend/internal/handlers/user.go`、`middleware/auth.go`、`services/auth_service.go`、`bootstrap.go` |
| 改扫描                     | `backend/internal/handlers/scan_path.go`、`services/scanner.go`、`scan_manager.go`                    |
| 改数据模型                 | `backend/internal/models/*.go`                                                                        |
| 改数据库初始化/迁移        | `backend/internal/database/db.go`                                                                     |
| 改本地元数据解析           | `backend/internal/services/metadata.go`、`extract.go`                                                 |
| 改离线更新/维护            | `backend/internal/handlers/offline.go`、`services/offline.go`                                         |

### 构建 / 校验

| 命令           | 位置                                                               |
| -------------- | ------------------------------------------------------------------ |
| 前端类型检查   | `npm run type-check`（vue-tsc）                                    |
| 前端 Lint      | `npm run lint`（oxlint + eslint）                                  |
| 前端构建       | `npm run build`（type-check + vite build）                         |
| 后端编译       | `cd backend && go build ./...`                                     |
| 后端测试       | `cd backend && go test ./...`                                      |
| 后端运行       | `cd backend && go run .`                                           |
| 单 exe 打包    | `build-release.bat`（根目录生成 SakuHentai.exe，双击即托盘运行）   |
| 纯后端运行     | `SakuHentai.exe --headless`（NAS/无界面环境）                      |
| Linux 交叉编译 | `cd backend && set GOOS=linux & set GOARCH=amd64 & go build ./...` |

---

## 五、分层架构约定

- **前端**：`views`（页面）→ `components`（复用组件）→ `stores`（Pinia 状态，按领域一个文件）→ `utils` / `api`（基础设施）→ `types`（契约类型）。
- **后端**：`handlers`（HTTP 层，负责参数解析与响应）→ `services`（业务逻辑与 E 站抓取）→ `models`（GORM 模型）→ `database`（DB 连接）。路由集中注册在 `router` 包，中间件在 `middleware` 包。
- **前后端契约**：前端 `types/comic.ts` 与后端 `handlers` 返回的 JSON 字段需保持一致（例如 `readCount` 统一替代旧的 `clickCount`）。

---

## 六、发布注意（v0.1）

- **打包**：运行根目录 `build-release.bat` 生成单文件 `SakuHentai.exe`（内嵌前端 + 后端 + 托盘）。双击运行后最小化到系统托盘，右键菜单「打开界面 / 退出程序」；NAS/无界面环境用 `SakuHentai.exe --headless` 纯后端运行。
- **运行目录**：exe 启动时自动切换到自身所在目录，`manga.db` / `config.json` / `data/` 均跟随 exe 位置（首次运行自动生成）。
- **端口**：默认监听 `0.0.0.0:8081`（「高级设置」可改）；若被占用自动切换随机空闲端口。
- `backend/manga.db`、`backend/data/*` 为**运行时数据**，已加入 `.gitignore` 并 `git rm --cached`；`backend/config.json` 仍被追踪（含本机 Clash 代理 `127.0.0.1:7897`，属通用配置，如需开源可自行删除）。
- `backend/webui/dist/` 需保留在 git：占位 `index.html` 保证 `go build` 无需前端即可编译；正式打包由 `build-release.bat` 覆盖为真实产物（`//go:embed` 依赖此目录）。
- `cmd_debug/` 为调试工具，不影响主程序；如需精简发布产物可从 `go build` 目标中排除。
