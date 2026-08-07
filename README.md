# 💡 为什么开发这个项目？

目前市面上的 Web 端媒体管理器在视觉呈现与交互体验上普遍有所欠缺；而体验优异的客户端软件又无法满足 自建服务器 / NAS 用户 对"集中托管、随时随地 Web 访问"的硬性需求。  
为了避免在多个互不相干的工具之间频繁切换与维护，SakuHentai 应运而生 —— 旨在用单一套件解决**资源刮削、本地存储与高质感阅读**的完整闭环。

✅️原生感 Web 视图：打破传统 Web 沉闷布局，提供桌面级的沉浸式阅读体验  
✅️元数据同步引擎：打通云端资源与本地存储，实现自动化抓取与统一整理  
✅️Headless 优先：Go 构筑的轻量后端，为 NAS/服务器环境量身定制  
✅️高吞吐缓存架构：针对大图库优化，基于预加载机制实现毫秒级响应

# ✅ 当前状态

**v1.2.0**（2026-08）：核心闭环持续完善，在线浏览与下载体验大幅增强。

- **在线浏览**：首页 / 热门 / 订阅 / 收藏 / 历史 / 排行榜（昨日 · 本月 · 本年 · All-Time 四种类型）/ 随机抽卡
- **下载系统**：画廊 + 归档多线程并发、下载优先级 + 抢占式调度、归档并发数控制、断点续传（.part / .bits）、下载完成自动消除更新标记
- **本地阅读**：离线书架 / 历史 / 排行榜 / 更新检测（周扫描 + 365 天老化）/ 维护查重（双列对比）/ 阅读进度续看（按账号）
- **智能筛选**：Tag 联想补全 + 负向排除（`-tag / -关键词`，离线过滤 + 在线本地丢弃）
- **Tag 维护**：词典同步 / 翻译 / 双轨三态维护
- **多用户**：管理员 / 成员 / 下载许可三级权限，阅读记录按账号隔离
- **设置中心**：8 大分组，每项设置均真实生效；内置日志系统（更新 / 维护 / 下载 / 错误四类日志）

# 🗺️ 后续计划

继续打磨在线浏览与阅读体验的细节优化，持续收敛下载稳定性与资源占用。

# 🚀 构建与运行

**一键打包（单文件 exe：前端 + 后端 + 系统托盘）：**

```bat
build-release.bat
```

生成 `SakuHentai.exe`：双击运行后最小化到系统托盘，右键托盘图标可选「打开界面 / 退出程序」；默认监听 `http://127.0.0.1:8081`，首次启动自动创建管理员 `admin/admin123`。打包脚本标题与前端「关于」页版本号均自动读取 `package.json`，发版只需修改一处。

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

> 后端 `//go:embed all:dist` 编译必需 `backend/webui/dist`（该目录不常驻仓库）：开发模式下请先执行一次 `npm run build` 并将 `dist/` 拷贝到 `backend/webui/dist`，或直接运行一次 `build-release.bat`。

# 🖼️ 预览图

## 在线界面

<img width="2440" height="1285" alt="image" src="https://github.com/user-attachments/assets/6ad55636-2335-4aac-8e4c-56e6ca637af2" />

## 离线界面

<img width="2447" height="1288" alt="image" src="https://github.com/user-attachments/assets/235923d4-89d8-4f77-a793-44311c28fa64" />


## 客制化抽卡

<img width="2423" height="1285" alt="image" src="https://github.com/user-attachments/assets/07e0d7f9-4c31-4eba-b3c1-8fb805deef3d" />


# ⚙️ 设置说明

设置页按主题分为 **8 大分组**，「账户与安全 / E 站连接 / 阅读体验 / 标签管理（我的标签）」全员可见，带 ⚠️ 标记的栏目仅管理员可见：

| 分组 | 栏目 | 说明 |
| --- | --- | --- |
| 账户与安全 | 账户 | E 站账号与 Cookie 配置 |
| | Profile ⚠️ | E 站 Profile 配置 |
| | 安全 ⚠️ | 用户管理 / 密码 |
| E 站连接 | EH 网站 | EH 站点访问配置 |
| | 网络 ⚠️ | 代理服务器地址、请求超时时间 |
| 阅读体验 | 样式 | 界面主题与外观 |
| | 阅读 | 阅读方向等阅读体验 |
| | 偏好 | 默认启动菜单、隐藏快速回顶、显示评论、全屏启动、搜索选项继承 |
| 下载管理 | 下载 ⚠️ | 保存路径、线程数、归档并发、下载优先级、更新方案 |
| 离线维护 | 更新扫描 ⚠️ | 每周自动更新扫描时刻、365 天老化规则 |
| 标签管理 | 我的标签 | 我的标签维护 |
| | Tag 维护 ⚠️ | 本地 Tag 字典维护（双轨三态） |
| 高级与日志 | 高级 ⚠️ | 开启日志、清除日志 |
| | 日志 ⚠️ | 更新 / 维护 / 下载 / 错误日志查看与实时尾随 |
| 关于 | 关于软件 | 版本信息（自动读取）、GitHub 链接 |

# 🌲 项目结构图

```
SakuHentai/
├── backend/                     # Go 后端服务（Gin + GORM + SQLite，单 exe 打包）
│   ├── main.go                  # 程序入口（--headless 双模式 + 系统托盘 + 自动端口）
│   ├── config.json              # 服务配置（本机代理地址等）
│   ├── internal/                # 核心逻辑（router / handlers / services / models / database / tray）
│   ├── webui/                   # 内嵌前端（go:embed all:dist，打包时拷入前端 dist/）
│   └── cmd_debug/               # 调试用命令行小工具（不影响主程序）
├── src/                         # Vue 3 前端核心源码（Vite + Pinia + Vue Router + TS）
│   ├── api/                     # 领域 API 封装（comic / download）
│   ├── components/              # 通用 UI 组件（含 common/ settings/ 详情面板等）
│   ├── composables/             # Vue 组合式函数（toast/modal/手柄/布局/联想/批量选择）
│   ├── config/                  # 接口基地址与 token 配置
│   ├── router/                  # 路由配置 + 登录守卫 + 管理员校验
│   ├── stores/                  # Pinia 全局状态（按领域分文件）
│   ├── types/                   # 数据契约类型
│   ├── utils/                   # 请求 / 存储 / 滚动记忆 / 错误上报 / 负向排除等
│   ├── views/                   # 页面级组件（online/offline/抽卡/下载/阅读/对比等）
│   ├── App.vue                  # 根组件
│   └── main.ts                  # 前端入口
├── public/                      # PWA 静态资源（favicon / manifest）
├── scripts/                     # 构建辅助脚本
├── plans/                       # 功能开发方案文档（Round1~5）
├── testdata_eh/                 # E 站抓取测试样本（HTML）
├── 学习笔记/                     # 个人学习笔记（已弃用）
├── 计划书/                       # 早期规划文档（已弃用）
├── vite.config.ts / package.json / tsconfig*.json   # 前端工程配置（package.json 为版本号唯一来源）
├── build-release.bat            # 一键打包单 exe（版本号自动读取）
└── README.md                    # 项目说明文档
```

# 📄 开源协议与致谢

## 致谢 / Acknowledgements

本项目在开发过程中借鉴与使用了以下优秀的开源项目及社区资源：

- **[JHentai](https://github.com/jiangtian616/JHenTai)**：感谢其在界面交互与部分业务逻辑实现上提供的灵感与思路。
- **[EhTagTranslation](https://github.com/EhTagTranslation/Database)**：本项目的标签汉化与翻译数据库来源于 EhTagTranslation 社区。
- **[e-hentai-tag-count](https://github.com/mokurin000/e-hentai-tag-count)**：本项目的标签联想来源于 e-hentai-tag-count 社区。

## 数据协议说明 (EhTagTranslation)

本项目集成的标签数据库内容由 **EhTagTranslation** 贡献者共同维护，数据依据 **[CC BY-NC-SA 3.0](https://creativecommons.org/licenses/by-nc-sa/3.0/deed.zh)**（署名-非商业性使用-相同方式共享 3.0）协议提供：

- **非商业性**：本项目及标签数据仅供个人学习与交流使用，严禁任何形式的商业盈利行为。
- **署名与共享**：对标签数据的二次分发或衍生使用均继承原协议条款。

## 开发者说明 / Notes

- **AI 辅助生成**：本项目的架构设计与绝大部分代码由 AI 辅助编写与重构。
- **测试状态**：项目目前处于早期快速迭代阶段，可能存在未覆盖的边缘场景或缺陷，欢迎提交 Issue 或 PR 协助完善。

## 开源协议 / License

本项目采用 [AGPL-3.0 License](LICENSE) 开源。
