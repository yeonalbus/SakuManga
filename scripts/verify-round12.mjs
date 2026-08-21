// scripts/verify-round12.mjs
// Round12 回归验证：小详情面板展开/收起滚动补偿（点开画廊保持视线焦点）
// 前置：本地后端 127.0.0.1:8081（内置最新 webui）。
// 在线列表/详情 API 通过 Playwright route 拦截 mock（不依赖 E 站网络），确定性验证：
//   场景A（桌面）：autoPanelColumns=on（收起 4 列 / 展开 3 列）→ 点击第4行第4列卡片 →
//               面板展开后网格变 3 列（重排发生），但被点卡片视口位置位移 ≤ 2px；
//               收起后视野稳定（scrollTop 变化 ≤ 2px）。
//   场景B（iPad 横屏 1194px）：autoPanelColumns=off（前后均 4 列）→ 展开/收起同样稳定。
import { chromium } from "playwright-core"

const BASE = "http://127.0.0.1:8081"
const CHROME = "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
const USER = "admin"
const PASS = "admin123"

function log(mark, msg) { console.log(`[${mark}] ${msg}`) }
let pass = true
const fail = (msg) => { pass = false; log("FAIL", msg) }
const ok = (msg) => log("OK", msg)

// ---------- 假数据 ----------
const FAKE_COMICS = Array.from({ length: 24 }, (_, i) => ({
  id: String(1001 + i),
  token: "tok" + (1001 + i),
  title: `Test Gallery ${1001 + i}`,
  coverUrl: "",
  source: "online",
  category: "Doujinshi",
  rating: 4.5,
  tags: ["language:chinese"],
  pageCount: 29 + i,
  updatedAt: "2026-08-21 13:00",
  uploader: "Nid135",
}))

function mockApi(page, { autoPanelColumns }) {
  return page.route("**/api/v1/comics/online**", (route) => {
    const url = route.request().url()
    if (url.includes("/detail")) {
      const u = new URL(url)
      const id = u.searchParams.get("id") || "1001"
      const body = {
        id,
        title: "Test Gallery " + id,
        coverUrl: "",
        token: u.searchParams.get("token") || "tok" + id,
        subTitle: "",
        tags: ["language:chinese"],
        rating: 4.5,
        pageCount: 29,
        updatedAt: "2026-08-21 13:00",
        category: "Doujinshi",
        uploader: "Nid135",
        isFavorite: false,
        favIndex: 0,
        isDownloaded: false,
        maxPreviewPage: 1,
        previewPages: [],
        comments: [],
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) })
    }
    if (url.includes("/previews")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: "[]" })
    }
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ comics: FAKE_COMICS, hasMore: false }),
    })
  })
}

// 样式设置写入 localStorage（store 初始化时读取）
function settingsScript(autoPanelColumns) {
  const settings = {
    themeMode: "system",
    layoutMode: "auto",
    layoutModeByDevice: { mobile: "auto", tablet: "auto", desktop: "auto" },
    autoPanelColumns,
    cardPanelClosedCols: 4,
    cardPanelOpenCols: 3,
    compactPanelClosedCols: 3,
    compactPanelOpenCols: 2,
  }
  return `localStorage.setItem("saku_style_settings", ${JSON.stringify(JSON.stringify(settings))})`
}

// 测量卡片相对 #main-content 顶部的视口偏移（null=未找到）
const cardOffset = (page, gid) =>
  page.evaluate((gid) => {
    const main = document.querySelector("#main-content")
    const el = main && main.querySelector(`.item-card[data-gid="${gid}"]`)
    if (!el) return null
    return el.getBoundingClientRect().top - main.getBoundingClientRect().top
  }, gid)

const mainScrollTop = (page) => page.evaluate(() => {
  const main = document.querySelector("#main-content")
  return main ? main.scrollTop : -1
})

const gridCols = (page) => page.evaluate(() => {
  const grid = document.querySelector(".card-grid")
  if (!grid) return null
  return getComputedStyle(grid).gridTemplateColumns.split(" ").length
})

const cardVisible = (page, gid) => page.evaluate((gid) => {
  const main = document.querySelector("#main-content")
  const el = main && main.querySelector(`.item-card[data-gid="${gid}"]`)
  if (!el) return false
  const r = el.getBoundingClientRect()
  const m = main.getBoundingClientRect()
  return r.bottom > m.top && r.top < m.bottom
}, gid)

async function login(page) {
  await page.goto(BASE + "/login")
  await page.fill("#login-username", USER)
  await page.fill("#login-password", PASS)
  await Promise.all([page.waitForNavigation({ waitUntil: "domcontentloaded" }), page.click(".login-btn")])
  await page.waitForSelector(".top-bar, .card-grid", { timeout: 15000 })
  log("STEP", "登录成功")
}

// 场景A：桌面，autoPanelColumns=on（4列→3列重排 + 补偿）
async function scenarioA(browser) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  await context.addInitScript(settingsScript(true))
  const page = await context.newPage()
  page.setDefaultTimeout(25000)
  await mockApi(page, { autoPanelColumns: true })
  await login(page)

  await page.goto(BASE + "/online/home")
  await page.waitForSelector(".item-card", { timeout: 20000 })
  await page.waitForFunction(() => document.querySelectorAll(".item-card").length >= 16, { timeout: 10000 })
  await page.waitForTimeout(500)

  const cols0 = await gridCols(page)
  if (cols0 === 4) ok("场景A 初始 4 列网格")
  else fail(`场景A 初始列数异常: ${cols0}`)

  // 定位第4行第4列（index 15 = gid 1016），滚动使其进入视野
  const GID = "1016"
  await page.evaluate((gid) => {
    const main = document.querySelector("#main-content")
    const el = main.querySelector(`.item-card[data-gid="${gid}"]`)
    if (el) main.scrollTop = el.offsetTop - 200
  }, GID)
  await page.waitForTimeout(400)
  const offset0 = await cardOffset(page, GID)
  const scroll0 = await mainScrollTop(page)
  log("INFO", `场景A 点击前 offset=${offset0?.toFixed(1)} scrollTop=${scroll0}`)
  if (offset0 === null) { fail("场景A 未找到目标卡片"); await context.close(); return }

  await page.locator(`.item-card[data-gid="${GID}"]`).click()
  await page.waitForSelector(".detail-panel", { timeout: 8000 })
  await page.waitForTimeout(800) // 等补偿（nextTick + 双 rAF）与面板内容

  const cols1 = await gridCols(page)
  if (cols1 === 3) ok("场景A 面板展开后网格变 3 列（重排发生）")
  else fail(`场景A 展开后列数异常: ${cols1}`)
  const offset1 = await cardOffset(page, GID)
  const vis1 = await cardVisible(page, GID)
  log("INFO", `场景A 展开后 offset=${offset1?.toFixed(1)} visible=${vis1}`)
  if (offset0 !== null && offset1 !== null && Math.abs(offset1 - offset0) <= 2)
    ok("场景A 被点卡片保持视线焦点（位移 ≤ 2px）")
  else fail(`场景A 被点卡片位移过大: ${offset0} -> ${offset1}`)
  if (!vis1) fail("场景A 被点卡片被挤出视口")
  else ok("场景A 被点卡片仍在视口内")

  // 收起面板：视野稳定（补偿会调整 scrollTop 使卡片保持原位，断言卡片视口位置不变）
  const offsetBeforeClose = await cardOffset(page, GID)
  await page.locator(".detail-panel-close").click()
  await page.waitForTimeout(800)
  const offset2 = await cardOffset(page, GID)
  const vis2 = await cardVisible(page, GID)
  log("INFO", `场景A 收起前 offset=${offsetBeforeClose?.toFixed(1)} 收起后 offset=${offset2?.toFixed(1)} visible=${vis2}`)
  if (offsetBeforeClose !== null && offset2 !== null && Math.abs(offset2 - offsetBeforeClose) <= 2)
    ok("场景A 收起面板后卡片视野稳定（位移 ≤ 2px，不乱滑动）")
  else fail(`场景A 收起后卡片位置漂移: ${offsetBeforeClose} -> ${offset2}`)
  if (offset0 !== null && offset2 !== null && Math.abs(offset2 - offset0) <= 2)
    ok("场景A 收起后被点卡片回到展开前视口位置")
  else fail(`场景A 收起后相对展开前偏移: ${offset0} -> ${offset2}`)
  if (!vis2) fail("场景A 收起后卡片不可见")
  await context.close()
}

// 场景B：iPad 横屏 1194px，autoPanelColumns=off（前后均 4 列）
async function scenarioB(browser) {
  const context = await browser.newContext({ viewport: { width: 1194, height: 834 } })
  await context.addInitScript(settingsScript(false))
  const page = await context.newPage()
  page.setDefaultTimeout(25000)
  await mockApi(page, { autoPanelColumns: false })
  await login(page)

  await page.goto(BASE + "/online/home")
  await page.waitForSelector(".item-card", { timeout: 20000 })
  await page.waitForFunction(() => document.querySelectorAll(".item-card").length >= 16, { timeout: 10000 })
  await page.waitForTimeout(500)

  const cols0 = await gridCols(page)
  if (cols0 === 4) ok("场景B 初始 4 列网格")
  else fail(`场景B 初始列数异常: ${cols0}`)

  const GID = "1010"
  await page.evaluate((gid) => {
    const main = document.querySelector("#main-content")
    const el = main.querySelector(`.item-card[data-gid="${gid}"]`)
    if (el) main.scrollTop = el.offsetTop - 120
  }, GID)
  await page.waitForTimeout(400)
  const offset0 = await cardOffset(page, GID)
  const scroll0 = await mainScrollTop(page)
  log("INFO", `场景B 点击前 offset=${offset0?.toFixed(1)} scrollTop=${scroll0}`)
  if (offset0 === null) { fail("场景B 未找到目标卡片"); await context.close(); return }

  await page.locator(`.item-card[data-gid="${GID}"]`).click()
  await page.waitForSelector(".detail-panel", { timeout: 8000 })
  await page.waitForTimeout(800)

  const cols1 = await gridCols(page)
  if (cols1 === 4) ok("场景B 面板展开后仍 4 列（autoPanelColumns=off）")
  else fail(`场景B 展开后列数异常: ${cols1}`)
  const offset1 = await cardOffset(page, GID)
  const vis1 = await cardVisible(page, GID)
  log("INFO", `场景B 展开后 offset=${offset1?.toFixed(1)} visible=${vis1}`)
  if (offset0 !== null && offset1 !== null && Math.abs(offset1 - offset0) <= 2)
    ok("场景B 被点卡片保持视线焦点（位移 ≤ 2px）")
  else fail(`场景B 被点卡片位移过大: ${offset0} -> ${offset1}`)
  if (!vis1) fail("场景B 被点卡片被挤出视口")
  else ok("场景B 被点卡片仍在视口内")

  const offsetBeforeClose = await cardOffset(page, GID)
  await page.locator(".detail-panel-close").click()
  await page.waitForTimeout(800)
  const offset2 = await cardOffset(page, GID)
  const vis2 = await cardVisible(page, GID)
  log("INFO", `场景B 收起前 offset=${offsetBeforeClose?.toFixed(1)} 收起后 offset=${offset2?.toFixed(1)} visible=${vis2}`)
  if (offsetBeforeClose !== null && offset2 !== null && Math.abs(offset2 - offsetBeforeClose) <= 2)
    ok("场景B 收起面板后卡片视野稳定（位移 ≤ 2px，不乱滑动）")
  else fail(`场景B 收起后卡片位置漂移: ${offsetBeforeClose} -> ${offset2}`)
  if (offset0 !== null && offset2 !== null && Math.abs(offset2 - offset0) <= 2)
    ok("场景B 收起后被点卡片回到展开前视口位置")
  else fail(`场景B 收起后相对展开前偏移: ${offset0} -> ${offset2}`)
  if (!vis2) fail("场景B 收起后卡片不可见")
  await context.close()
}

const browser = await chromium.launch({ executablePath: CHROME, headless: true })
try {
  await scenarioA(browser)
  await scenarioB(browser)
} finally {
  await browser.close()
}

console.log(pass ? "\n===== ROUND12 全部验证通过 =====" : "\n===== ROUND12 存在失败项 =====")
process.exit(pass ? 0 : 1)