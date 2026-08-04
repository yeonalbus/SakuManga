# 💡 为什么开发这个项目？
  目前市面上的 Web 端媒体管理器在视觉呈现与交互体验上普遍有所欠缺；而体验优异的客户端软件又无法满足 自建服务器 / NAS 用户 对“集中托管、随时随地 Web 访问”的硬性需求。  
  为了避免在多个互不相干的工具之间频繁切换与维护，SakuHentai 应运而生 —— 旨在用单一套件解决**资源刮削、本地存储与高质感阅读**的完整闭环。  
  
  ✅️原生感 Web 视图：打破传统 Web 沉闷布局，提供桌面级的沉浸式阅读体验
  ✅️元数据同步引擎：打通云端资源与本地存储，实现自动化抓取与统一整理
  ✅️Headless 优先：Go 构筑的轻量后端，为 NAS/服务器环境量身定制
  ✅️高吞吐缓存架构：针对大图库优化，基于预加载机制实现毫秒级响应

# 📷 预览图
## 在线界面
<img width="2457" height="1287" alt="image" src="https://github.com/user-attachments/assets/37d1d499-a84e-419b-9b8c-94cf7b3b8e4f" />

## 在线排行榜
<img width="2452" height="1279" alt="image" src="https://github.com/user-attachments/assets/0ff94560-7ead-40b1-b2be-b1066e067f5f" />

## 下载界面（未完工）
<img width="2454" height="1289" alt="image" src="https://github.com/user-attachments/assets/11983327-d0c8-4db0-9553-861f17d697a5" />

## 本地首页
<img width="2455" height="1287" alt="image" src="https://github.com/user-attachments/assets/7f277b6e-7082-48f7-8dd4-fd1fbd29e9b3" />

## 抽卡界面（未完工）
<img width="1661" height="1109" alt="image" src="https://github.com/user-attachments/assets/d9f9d0ef-3136-4037-9b80-3e5edcd0310b" />

## 阅读界面（未完工）
<img width="2455" height="1286" alt="image" src="https://github.com/user-attachments/assets/f73edeb9-4dd2-41ad-89e9-d42ee1459b39" />

# 📚 接下来的计划：
开发到哪算哪
目前在线的收藏问题比较大，还有一些功能没有实现的

# 🌲 项目结构图
```
SakuHentai/
├── backend/                # Go 后端服务
│   ├── config.json         # 服务配置文件
│   ├── internal/           # 核心业务逻辑 (路由、控制层、服务层)
│   ├── main.go             # 后端程序入口
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
├── go.mod                  # Go 模块依赖
└── README.md               # 项目说明文档
```
