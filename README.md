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

> 后端 `//go:embed all:dist` 编译必需 `backend/webui/dist`（该目录不常驻仓库）：开发模式下请先执行一次 `npm run build` 并将 `dist/` 拷贝到 `backend/webui/dist`，或直接运行一次 `build-release.bat`。

# 🖼️ 预览图

## 在线界面

<img width="2418" height="1288" alt="image" src="https://github.com/user-attachments/assets/b58524a2-9f90-45be-90ca-2bdc5f1a735d" />

## 下载界面

<img width="2453" height="1286" alt="image" src="https://github.com/user-attachments/assets/241a67a0-23f6-4dcc-92aa-0fbd9c4656cd" />

## 阅读界面

<img width="2453" height="1289" alt="image" src="https://github.com/user-attachments/assets/8623608a-4a2a-4199-8d43-3cccbcfc412b" />

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
├── backend/                     # Go 后端服务（Gin + GORM + SQLite，单 exe 打包）
│   ├── main.go                  # 程序入口（--headless 双模式 + 系统托盘 + 自动端口）
│   ├── config.json              # 服务配置（本机代理地址等）
│   ├── internal/                # 核心逻辑（router / handlers / services / models / database / tray）
│   ├── webui/                   # 内嵌前端（go:embed all:dist，打包时拷入前端 dist/）
│   └── cmd_debug/               # 调试用命令行小工具（不影响主程序）
├── src/                         # Vue 3 前端核心源码（Vite + Pinia + Vue Router + TS）
│   ├── api/                     # 领域 API 封装
│   ├── components/              # 通用 UI 组件（含 common/ 与 settings/）
│   ├── composables/             # Vue 组合式函数（toast/modal/手柄/布局）
│   ├── config/                  # 接口基地址与 token 配置
│   ├── router/                  # 路由配置 + 登录守卫
│   ├── stores/                  # Pinia 全局状态（按领域分文件）
│   ├── types/                   # 数据契约类型
│   ├── utils/                   # 请求 / 存储 / 错误上报等基础设施
│   ├── views/                   # 页面级组件（online/offline/抽卡/下载/阅读等）
│   ├── App.vue                  # 根组件
│   └── main.ts                  # 前端入口
├── public/                      # PWA 静态资源（favicon / manifest）
├── scripts/                     # 构建辅助脚本
├── plans/                       # 功能开发方案文档
├── testdata_eh/                 # E 站抓取测试样本（HTML）
├── 学习笔记/                     # 个人学习笔记（已弃用）
├── 计划书/                       # 早期规划文档（已弃用）
├── vite.config.ts / package.json / tsconfig*.json   # 前端工程配置
├── build-release.bat            # 一键打包单 exe
└── README.md                    # 项目说明文档
```

# 📄 开源协议与致谢

## 致谢 / Acknowledgements

本项目在开发过程中借鉴与使用了以下优秀的开源项目及社区资源：

- **[JHentai](https://github.com/jiangtian616/JHenTai)**：感谢其在界面交互与部分业务逻辑实现上提供的灵感与思路。
- **[EhTagTranslation](https://github.com/EhTagTranslation/Database)**：本项目的标签汉化与翻译数据库来源于 EhTagTranslation 社区。
- **[e-hentai-tag-count]https://github.com/mokurin000/e-hentai-tag-count**：本项目的标签联想来源于 e-hentai-tag-count 社区。

## 数据协议说明 (EhTagTranslation)

本项目集成的标签数据库内容由 **EhTagTranslation** 贡献者共同维护，数据依据 **[CC BY-NC-SA 3.0](https://creativecommons.org/licenses/by-nc-sa/3.0/deed.zh)**（署名-非商业性使用-相同方式共享 3.0）协议提供：

- **非商业性**：本项目及标签数据仅供个人学习与交流使用，严禁任何形式的商业盈利行为。
- **署名与共享**：对标签数据的二次分发或衍生使用均继承原协议条款。

## 开发者说明 / Notes

- **AI 辅助生成**：本项目的架构设计与绝大部分代码由 AI 辅助编写与重构。
- **测试状态**：项目目前处于早期快速迭代阶段，可能存在未覆盖的边缘场景或缺陷，欢迎提交 Issue 或 PR 协助完善。

## 开源协议 / License

本项目采用 [AGPL-3.0 License](LICENSE) 开源。
