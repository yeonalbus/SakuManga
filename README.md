# 预览图
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


# 基于VUE开发的EhentaiViewer-WEB版
✅️更现代化的UI界面  
✅️线上和本地双系统联动管理  
✅️便于部署在主机端服务器上  
✅️人性化的本地管理，更快速的图片加载

# 接下来的计划：
开发到哪算哪
目前在线的收藏问题比较大，还有一些功能没有实现的

# 项目结构图
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
