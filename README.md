# 基于VUE开发的EhentaiViewer-WEB版
✅️更现代化的UI界面  
✅️线上和本地双系统联动管理  
✅️便于部署在主机端服务器上  
<img width="2462" height="1290" alt="49260a1b175d5e25c1b0e47b3c2b7af4" src="https://github.com/user-attachments/assets/ffaf4e65-570f-48e1-b709-af4cdf7ae4e8" />
<img width="2461" height="1292" alt="db6ce865e8e1a4be0825d2fe7ca19e28" src="https://github.com/user-attachments/assets/9c0d8051-4e85-455a-aa20-775c3908148a" />

# 接下来的计划：前端开发
## 第一阶段：数据契约与基础设施（立规矩、做基建）  
先花半天时间把“规矩”和“通用工具”盖好，不涉及具体业务页面：  
  
**1、定义 TypeScript 数据契约 (src/types/comic.ts)**
* 把本子（ComicItem）、书架（Bookshelf）、下载任务（DownloadTask）、筛选条件（FilterOptions）的数据字段全部写死。

**2、替换原生弹窗（Toast & Modal）**
* 封装一个全局 Toast（代替 alert）和 Modal（代替 prompt），挂在 App.vue。

**3、本地状态持久化（Mock Store）**
* 用 pinia 或简单的 ref + localStorage，把视图模式、书架列表、阅读清单存起来，保证刷新页面数据不丢失。

## 第二阶段：前端业务功能逐个击破（界面开发）
有了第一阶段的 TypeScript 类型和全局 Toast，后面的页面开发就可以完全不考虑数据对接，纯靠模拟数据把 UI 和交互填满。  
建议按从易到难、从外到内的顺序开发：  

* **外观细节与局部样式：** Fav 0~9 的 2x5 配色网格、卡片/名片的评分展示。
* **侧边栏与书架交互：** 离线侧边栏 📚 书架 的折叠/展开、创建新书架。
* **本子详情页：** 在线版（标签云/画师/E站评分）与离线版（本地文件信息/文件路径/修改 Tag）。
* **顶栏三大件：**
    * 搜索框（历史搜索 + Tag 联想下拉）
    * 筛选框（搜索参数 vs 内存过滤）
    * 随机本子抽卡（ Modal 弹窗 + 抽卡结果）
* **右侧阅读清单抽屉：** 侧边栏抽屉展开、队列排序、批量取消。
* **本子阅读器组件：** 单双页切换、从右往左翻页、全屏控制、连贯读取阅读清单。
* **辅助界面：** 本地维护排查（VS 查重对比）、系统设置界面。

## 第三阶段：交互体验与细节打磨（体验注入）
当所有页面和功能能跑通后，集中补齐这些提升爽快感的交互细节：

* **批量选择模式：** 卡片勾选框显影、底部批量移入书架/阅读清单控制栏。
* **键盘快捷键：** `/` 聚焦搜索框、`Esc` 关闭抽屉/Modal、`A/D` 或 `←/→` 翻页。
* **加载与空状态：** 无数据时的 placeholder 占位图、数据加载中的骨架屏（Skeleton）。

## 第四阶段：开发后端与血脉打通（接口对接）
到了这一步，前端已经是一个可以直接用鼠标流畅点选、样式完整的“单机版应用”了。  
此时再去写后端（不管是 Python, Go 还是 Node.js）：  
后端只需要按照第一阶段写好的 comic.ts 接口规范输出 JSON。  
前端只需要写一个 API 服务层（src/api/*.ts），把页面里原本读取 localStorage 或静态 Mock 的代码替换成 axios.get() 即可。  

**具体计划建议看记录**
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
