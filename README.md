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
src/
├── assets/ # 静态资源 (CSS / 图片)
├── components/ # 可复用小组件 (卡片、弹窗、顶栏等)
│ ├── FilterBar.vue # 筛选组件
│ ├── GridContainer.vue # 卡片排列控制组件
│ ├── ItemCard.vue # 卡片样式（卡片/名片）切换组件
│ ├── ModeToggle.vue # 控制卡片切换组件
│ ├── OfflineSidebar.vue # 离线边栏组件
│ ├── OnlineSidebar.vue # 在线边栏组件
│ ├── PagiNation.vue # 翻页控件（目前没测试效果）
│ ├── RandomPicker.vue # 随机抽卡组件
│ ├── ReadingList.vue # 阅读清单组件
│ ├── SearchBar.vue # 搜索栏组件
│ └── TopBar.vue # 顶栏组件（包含搜索栏，筛选，随机抽卡和阅读清单）
├── views/ # 各个大页面
│ ├── online/ # 在线模块页面
│ │ ├── OnlineFavorites.vue # Ehentai收藏页面
│ │ ├── OnlineHistory.vue # 在线历史记录
│ │ ├── OnlineHome.vue # 在线首页
│ │ ├── OnlineHot.vue # 在线热门
│ │ ├── OnlineSub.vue # 在线订阅
│ │ └── OnlineTop.vue # 在线排行榜
│ ├── offline/ # 离线模块页面
│ │ ├── OfflineBookcase.vue # 本地书架
│ │ ├── OfflineHistory.vue # 本地历史记录
│ │ ├── OfflineHome.vue # 本地首页
│ │ ├── OfflineMaintain.vue # 本地书目维护
│ │ ├── OfflineToplist.vue # 本地阅读次数最多排行榜
│ │ └── OfflineUpdate.vue # 本地书目更新
│ ├── DownloadsView.vue # 下载界面
│ ├── NotFound.vue # 跳转页面错误占位符
│ └── SettingsView.vue # 设置界面
├── router/ # 路由配置
│ └── index.js # 路由表
├── stores/
│ ├── counter.ts # 忘了
│ └── viewMode.ts # 卡片模式切换
├── App.vue # 应用主外壳
└── main.js # 项目入口文件
```
