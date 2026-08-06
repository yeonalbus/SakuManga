# 💡 为什么开发这个项目？

目前市面上的 Web 端媒体管理器在视觉呈现与交互体验上普遍有所欠缺；而体验优异的客户端软件又无法满足 自建服务器 / NAS 用户 对"集中托管、随时随地 Web 访问"的硬性需求。  
为了避免在多个互不相干的工具之间频繁切换与维护，SakuHentai 应运而生 —— 旨在用单一套件解决**资源刮削、本地存储与高质感阅读**的完整闭环。

✅️原生感 Web 视图：打破传统 Web 沉闷布局，提供桌面级的沉浸式阅读体验  
✅️元数据同步引擎：打通云端资源与本地存储，实现自动化抓取与统一整理  
✅️Headless 优先：Go 构筑的轻量后端，为 NAS/服务器环境量身定制  
✅️高吞吐缓存架构：针对大图库优化，基于预加载机制实现毫秒级响应

# ✅ 当前状态

**v1.0.0**：在线浏览、排行榜、下载、本地阅读、抽卡、Tag 维护、多用户等核心闭环已可用；设置项已收敛为最小可用集，每项设置均真实生效。

# 🗺️ 后续计划

继续打磨使用体验，聚焦在线浏览与阅读体验的细节优化。

# 🚀 构建与运行

**一键打包（单文件 exe：前端 + 后端 + 系统托盘）：**

```bat
build-release.bat
```

生成 `SakuHentai.exe`：双击运行后最小化到系统托盘，右键托盘图标可选「打开界面 / 退出程序」；默认监听 `http://127.0.0.1:8081`，首次启动自动创建管理员 `admin/admin123`。

**纯后端运行（NAS / 无界面环境）：**

```bat
SakuHentai.exe --headless
```

**开发模式：**

```bat
npm install
cd backend && go run .        # 后端 http://127.0.0.1:8081
npm run dev                   # 前端开发服务器（Vite 代理到后端 API）
```

# 🖼️ 预览图

## 在线界面

<img width="2457" height="1287" alt="image" src="https://github.com/user-attachments/assets/37d1d499-a84e-419b-9b8c-94cf7b3b8e4f" />

## 在线排行榜

<img width="2452" height="1279" alt="image" src="https://github.com/user-attachments/assets/0ff94560-7ead-40b1-b2be-b1066e067f5f" />

## 下载界面

<img width="2454" height="1289" alt="image" src="https://github.com/user-attachments/assets/11983327-d0c8-4db0-9553-861f17d697a5" />

## 本地首页

<img width="2455" height="1287" alt="image" src="https://github.com/user-attachments/assets/7f277b6e-7082-48f7-8dd4-fd1fbd29e9b3" />

## 抽卡界面

<img width="1661" height="1109" alt="image" src="https://github.com/user-attachments/assets/d9f9d0ef-3136-4037-9b80-3e5edcd0310b" />

## 阅读界面

<img width="2455" height="1286" alt="image" src="https://github.com/user-attachments/assets/f73edeb9-4dd2-41ad-89e9-d42ee1459b39" />

# ⚙️ 设置说明

设置页共 11 个栏目，其中「网络 / 下载 / Tag 维护 / 高级 / 安全」仅管理员可见：

| 栏目     | 说明                                                                       |
| -------- | -------------------------------------------------------------------------- |
| 账户     | 个人资料与账号信息                                                         |
| EH       | EH 网站账号与访问配置                                                      |
| 样式     | 界面主题与外观                                                             |
| 阅读     | 阅读方向等阅读体验                                                         |
| 偏好     | 默认启动菜单、隐藏快速回顶按钮、显示画廊评论、以全屏模式启动、搜索选项继承 |
| 网络     | 代理服务器地址（后端生效）、请求超时时间                                   |
| 下载     | 下载保存路径等下载行为                                                     |
| Tag 维护 | 本地 Tag 字典维护                                                          |
| 高级     | 开启日志、清除日志                                                         |
| 安全     | 安全策略                                                                   |
| 关于     | 版本信息、GitHub 链接                                                      |

# 🌲 项目结构图

```
SakuHentai/
├── backend/                # Go 后端服务（Gin + GORM + SQLite）
│   ├── config.json         # 服务配置文件
│   ├── webui/              # 内嵌前端产物（go:embed，单 exe 打包用）
│   ├── internal/           # 核心业务逻辑 (路由、控制层、服务层、托盘)
│   ├── main.go             # 后端程序入口（--headless 双模式 + 系统托盘）
│   └── manga.db            # SQLite 本地数据库
├── src/                    # Vue 3 前端核心源码
│   ├── api/                # Axios/Fetch 接口封装
│   ├── components/         # 可复用的 UI 通用组件
│   ├── composables/        # Vue 组合式函数 (Hooks)
│   ├── router/             # 路由配置 (Vue Router)
│   ├── stores/             # 全局状态管理 (Pinia)
│   ├── views/              # 页面级组件 (各路由对应页面)
│   ├── App.vue             # 根组件
│   └── main.ts             # 前端入口文件
├── 学习笔记/                # 个人开发过程中的语法与功能总结（已弃用）
├── 计划书/                  # 前端开发排期与规划文档（已弃用）
├── vite.config.ts          # Vite 构建与代理配置
├── package.json            # 前端项目依赖与脚本
├── build-release.bat       # 一键打包单 exe
├── go.mod                  # Go 模块依赖（位于 backend/ 下）
└── README.md               # 项目说明文档
```
