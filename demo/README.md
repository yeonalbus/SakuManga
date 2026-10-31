# demo/ — 演示数据与截图流水线

给 README / Release 用的**示例截图**。界面是真的，数据是假的。

- 不碰业务代码：`src/`、`vite.config.ts`、后端一个字都不用改
- 不依赖真实书库、不依赖网络、不需要 E 站账号
- 截图里的书名、标签、封面、内页全部是自造素材，**不含任何真实作品内容**

## 快速开始

```bash
npm run build-only   # 首次或改了前端代码后需要（演示环境托管 dist/）
npm run demo         # 起服务并自动打开浏览器
```

打开后就是 `http://127.0.0.1:5200`，已自动登录 `demo` 账号，本地库、书架、书签、词云都有数据。

> **不要用 `npm run dev` 截图。** dev server 会把接口转发给真实后端（127.0.0.1:8081），
> 那边没有演示数据，看到的是空列表或直接跳登录页。演示环境必须走 `npm run demo`。

## 自己手动截图

```bash
npm run demo
```

1. 浏览器自动弹出（普通 Chrome / Edge 即可，不需要任何特殊工具）
2. 左侧菜单点进要截的页面，随便点、随便滚、展开面板
3. 截图：`Win + Shift + S` 框选，或 `F12` → `Ctrl + Shift + P` → 输入 `screenshot` → 选「Capture screenshot」
4. 截完关掉浏览器窗口，终端 `Ctrl + C` 停服务

几个要知道的点：

| 情况 | 说明 |
|---|---|
| 刷新后数据没了？ | 不会。数据在 `demo/data/*.json`，刷新不会变 |
| 界面里的操作会保存吗？ | 会写到 localStorage，但每次 `npm run demo` 都是干净状态（不持久化） |
| 想换一批数据 | 直接编辑 `demo/data/*.json`，保存后**刷新页面**即生效 |
| 想换封面/内页图 | 替换 `demo/assets/covers/`、`demo/assets/pages/` 里的 SVG（同名覆盖） |
| 端口被占用 | `npm run demo -- --port 5201` |

## 自动出图（可选）

```bash
npm run demo:shots                 # 自动起服务 → 按清单逐页截图 → 存 docs/screenshots/
npm run demo:shots -- --only 09-reader   # 只重出某一张
npm run demo -- --port 5210        # 服务已手动开着时，出图脚本会自动复用它
```

页面清单在 `demo/pages.mjs`，一条 = 一张图：

```js
{
  name: '05-xp-cloud',        // 产物文件名
  path: '/offline/toplist',   // 路由
  wait: 1600,                 // 打开后等多久（等数据渲染）
  before: async (page) => {   // 截图前的交互（点击/切换标签页），可省略
    await page.getByText('XP 词云').first().click()
  },
  settle: 1800,               // 交互后额外等待（等 toast 消失）
}
```

不满意某张图就改这条配置（换路由、换等待时间、加点击展开），然后
`npm run demo:shots -- --only 那张的名字`，不用重写脚本。

## 目录

```
demo/
├── serve.mjs            # 本地服务：托管 dist/ + 拦截 /api/v1/** 返回假数据
├── shots.mjs            # 自动出图（Playwright）
├── pages.mjs            # 截图清单
├── api/                 # 一个页面一个 handler，按路由段匹配
│   ├── index.mjs        #   路由表 + 兜底响应
│   ├── auth.mjs         #   登录态（固定 demo 用户）
│   ├── comics.mjs       #   离线/在线漫画、封面、内页、抽卡
│   ├── tags.mjs         #   标签词典（留空会导致标签退化成英文）
│   ├── library.mjs      #   书架 / 历史 / 阅读清单 / 评分
│   ├── bookmarks.mjs    #   搜刮书签
│   └── offline.mjs      #   XP 词云 / 画质升级
├── data/                # 纯 JSON 假数据（改这里最直观）
│   ├── offline.json     #   本地漫画（含 coverFile 指向封面素材）
│   ├── online.json      #   在线列表/热门
│   ├── bookshelves.json #   书架
│   ├── bookmarks.json   #   书签
│   ├── xpcloud.json     #   词云数据
│   ├── upgrade.json     #   升级候选
│   ├── history.json     #   历史记录
│   ├── reading-list.json#   阅读清单
│   └── tags.json        #   标签翻译
└── assets/              # 自造素材
    ├── covers/*.svg     #   封面（几何排版风）
    ├── pages/*.svg      #   阅读器内页（分镜占位）
    └── generate.mjs     #   重新生成素材（确定性随机，产物可 diff）
```

## 排障

| 现象 | 原因 / 处理 |
|---|---|
| 打开是登录页 | 地址不对。演示环境是 `http://127.0.0.1:5200`，不是 5173 |
| 一片空白 / 接口全 404 | `dist/` 不存在或过旧：`npm run build-only` |
| 标签显示成 `language:chinese` 这种裸 key | `demo/data/tags.json` 里缺这条的翻译 |
| 封面裂图 | 对应 `assets/covers/*.svg` 缺失，或 `data/*.json` 里 `coverFile` 写错 |
| 页面里某块比真实界面少东西 | 该接口的假数据字段不全，对照 `src/types/` 补字段 |
| 出图时浏览器找不到 | 装 Chrome，或 `set CHROME_PATH=...` 指向浏览器 exe |
| 端口 EACCES / 被占用 | `npm run demo -- --port 5201`（避开系统保留端口） |

## 两个约定

1. **只增不改**：`demo/` 独立于业务代码。以后前端接口变了，改 `demo/api/` 里的对应 handler 即可，
   不要为了演示环境去改 `src/`。
2. **门面数据要审**：`demo/data/*.json` 就是截图上会出现的文字，新增条目时自己过一眼，
   别把真实作品标题/标签带进来。素材同理——`assets/` 里只放自造图。
