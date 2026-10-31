// demo/pages.mjs
// 截图清单：一条 = 一张图。
//
// 字段：
//   name    产物文件名（不含扩展名），建议带序号控制顺序
//   path    路由（相对服务根路径，可带 query）
//   wait    页面打开后等待毫秒数（等首屏数据渲染完）
//   init    页面打开前的注入（localStorage 设置等），可省略
//   before  截图前的交互（点击展开、切换标签页等），可省略
//   settle  before 执行后的额外等待毫秒数（等 toast 淡出、动画结束）
//   skip    true 表示暂时不出图（保留配置便于随时启用）
//
// 加一张新图：复制一条，改 name / path 即可。
// 只想出某几张：node demo/shots.mjs --only reader,bookmarks

export const pages = [
  {
    name: '01-online-hot',
    path: '/online/hot',
    wait: 1800,
  },
  {
    name: '02-bookmarks',
    path: '/online/bookmarks',
    wait: 1800,
  },
  {
    name: '03-offline-home',
    path: '/offline/home',
    wait: 1800,
  },
  {
    name: '04-bookshelf-wall',
    path: '/offline/bookshelves',
    wait: 1800,
  },
  {
    name: '05-xp-cloud',
    path: '/offline/toplist',
    init: async (page) => {
      // 默认字号（13~42px）在 1600 宽画布上偏小、词云显得稀疏。
      // 截图时放大字号、收紧间距，让词云填满区域（参数面板会显示「已调整」角标，属正常状态）。
      await page.addInitScript(() => {
        const raw = localStorage.getItem('app_xp_cloud_settings')
        const cur = raw ? JSON.parse(raw) : {}
        Object.assign(cur, { fontMin: 20, fontMax: 78, lineRatio: 1.24, padX: 0.18 })
        localStorage.setItem('app_xp_cloud_settings', JSON.stringify(cur))
      })
    },
    wait: 1600,
    before: async (page) => {
      await page.getByText('XP 词云', { exact: false }).first().click()
    },
    settle: 1800,
  },
  {
    name: '06-random-draw',
    path: '/random',
    wait: 1500,
    before: async (page) => {
      await page.getByText('开始抽卡', { exact: false }).first().click()
    },
    settle: 4200,
  },
  {
    name: '07-upgrade',
    path: '/upgrade',
    wait: 1800,
  },
  {
    name: '08-offline-detail',
    path: '/offline/detail?id=demo-c01',
    wait: 1800,
  },
  {
    name: '09-reader',
    path: '/reader?source=offline&id=demo-c01&page=1',
    init: async (page) => {
      // 双页并排展示（默认 singleCover=true 时首页单独占一屏，画面会显得空）
      await page.addInitScript(() => {
        const raw = localStorage.getItem('saku_reader_settings')
        const cur = raw ? JSON.parse(raw) : {}
        cur.singleCover = false
        localStorage.setItem('saku_reader_settings', JSON.stringify(cur))
      })
    },
    wait: 2500,
    before: async (page) => {
      // 阅读器顶部工具栏悬浮在页面之上，默认页面会顶到视口边缘被盖住。
      // 限制图片高度，让整页完整落在工具栏与底部栏之间。
      await page.addStyleTag({
        content: '.manga-page-img{max-height:calc(100vh - 128px) !important}',
      })
      await page.waitForTimeout(400)
    },
  },
]
