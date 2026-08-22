// scripts/verify-round17.mjs
// Round17 回归验证：PWA manifest 补全 + 全局 a 拦截 + backState 无条件写入（返回来源页）
// 前置：本地后端 127.0.0.1:8081（内置最新 webui），已扫描 MangaExamlpe（2 本离线漫画）
import { chromium } from "playwright-core"

const BASE = "http://127.0.0.1:8081"
const CHROME = "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
const USER = "admin"
const PASS = "admin123"

function log(mark, msg) { console.log(`[${mark}] ${msg}`) }
let pass = true
const fail = (msg) => { pass = false; log("FAIL", msg) }
const ok = (msg) => log("OK", msg)

async function api(path, { method = "GET", body, token } = {}) {
  const headers = { "Content-Type": "application/json" }
  if (token) headers["Authorization"] = "Bearer " + token
  const res = await fetch(BASE + "/api/v1" + path, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined })
  const text = await res.text()
  let json = null
  try { json = JSON.parse(text) } catch {}
  return { status: res.status, json }
}

let token, COMIC_A
try {
  const login = await api("/auth/login", { method: "POST", body: { username: USER, password: PASS } })
  token = login.json.token || login.json.data?.token
  if (!token) throw new Error("登录失败")
  const comics = await api("/comics/offline", { token })
  if (!Array.isArray(comics.json) || comics.json.length < 1) throw new Error("离线漫画不足")
  COMIC_A = comics.json[0]
} catch (e) { console.error("播种失败:", e.message); process.exit(1) }

// PWA standalone 模拟
const pwaInit = `
  const origMatch = window.matchMedia.bind(window)
  window.matchMedia = (q) => {
    if (q === "(display-mode: standalone)") {
      return { matches: true, media: q, onchange: null, addEventListener: () => {}, removeEventListener: () => {}, addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false }
    }
    return origMatch(q)
  }
  Object.defineProperty(window.navigator, "standalone", { value: true, configurable: true })
`

// ---------- Bug3：PWA 下返回来源页（top=0 场景，模拟抽卡） ----------
async function bug3(browser) {
  const context = await browser.newContext({ viewport: { width: 800, height: 900 } })
  await context.addInitScript(pwaInit)
  const page = await context.newPage()
  page.setDefaultTimeout(25000)
  await page.goto(BASE + "/login")
  await page.fill("#login-username", USER)
  await page.fill("#login-password", PASS)
  await Promise.all([page.waitForNavigation({ waitUntil: "domcontentloaded" }), page.click(".login-btn")])
  await page.waitForSelector(".top-bar, .card-grid", { timeout: 15000 })

  // 离线首页（不滚动，top=0）→ 点卡片 → 详情 → 返回
  await page.goto(BASE + "/offline/home")
  await page.waitForSelector(".item-card", { timeout: 20000 })
  await page.waitForTimeout(1000)
  // 确认 scrollTop=0（未滚动场景）
  const top0 = await page.evaluate(() => document.querySelector("#main-content")?.scrollTop || 0)
  log("INFO", "Bug3 列表 scrollTop=" + top0)
  await page.locator(".item-card").first().click()
  await page.waitForSelector(".offline-detail-page, .detail-page", { timeout: 20000 })
  await page.waitForTimeout(1500)
  if (!page.url().includes("/offline/detail")) fail("Bug3 未进入详情: " + page.url())
  // 返回
  await page.locator(".back-btn, .detail-fab-back").first().click()
  await page.waitForTimeout(2000)
  const backUrl = page.url()
  if (backUrl.includes("/offline/home")) ok("Bug3 top=0 返回回到来源页（离线首页）")
  else fail("Bug3 top=0 返回 URL 异常: " + backUrl)
  const cards = await page.locator(".item-card").count()
  if (cards > 0) ok("Bug3 返回后列表数据保留（非整页刷新）")
  else fail("Bug3 返回后列表空")
  await context.close()
}

// ---------- 书架返回：fullPath（含书架 id）保留 ----------
async function bookshelfBack(browser) {
  const context = await browser.newContext({ viewport: { width: 800, height: 900 } })
  await context.addInitScript(pwaInit)
  const page = await context.newPage()
  page.setDefaultTimeout(25000)
  await page.goto(BASE + "/login")
  await page.fill("#login-username", USER)
  await page.fill("#login-password", PASS)
  await Promise.all([page.waitForNavigation({ waitUntil: "domcontentloaded" }), page.click(".login-btn")])
  await page.waitForSelector(".top-bar, .card-grid", { timeout: 15000 })

  // 创建书架并加入一本 → 进入书架页 → 点卡片 → 详情 → 返回
  const sh = await api("/bookshelves", { method: "POST", body: { name: "返回测试书架" }, token })
  const shelfId = sh.json.data?.id
  await api("/bookshelves/" + shelfId + "/comics", { method: "POST", body: { comicId: COMIC_A.id }, token })

  await page.goto(BASE + "/offline/bookshelf?id=" + shelfId)
  await page.waitForSelector(".item-card", { timeout: 20000 })
  await page.waitForTimeout(1000)
  const cardCount = await page.locator(".item-card").count()
  if (cardCount > 0) ok("书架返回 书架页有卡片")
  else fail("书架返回 书架页无卡片")

  await page.locator(".item-card").first().click()
  await page.waitForSelector(".offline-detail-page, .detail-page", { timeout: 20000 })
  await page.waitForTimeout(1500)
  if (!page.url().includes("/offline/detail")) fail("书架返回 未进入详情")

  await page.locator(".back-btn, .detail-fab-back").first().click()
  await page.waitForTimeout(2000)
  const backUrl = page.url()
  if (backUrl.includes("/offline/bookshelf") && backUrl.includes("id=" + shelfId)) {
    ok("书架返回 回到书架页且保留书架 id（fullPath 生效）")
  } else {
    fail("书架返回 URL 异常: " + backUrl)
  }
  // 清理测试书架
  await api("/bookshelves/" + shelfId, { method: "DELETE", token })
  await context.close()
}

// ---------- 代码级断言 ----------
async function codeChecks() {
  const fs = await import("fs")
  // manifest
  const manifest = JSON.parse(fs.readFileSync("public/site.webmanifest", "utf8"))
  if (manifest.start_url && manifest.scope && manifest.display_override) ok("manifest 已补 start_url/scope/display_override")
  else fail("manifest 缺 start_url/scope/display_override: " + JSON.stringify(manifest))
  // main.ts 全局 a 拦截
  const main = fs.readFileSync("src/main.ts", "utf8")
  if (main.includes("preventPwaLinkEscape") && main.includes("addEventListener('click'")) ok("全局 a 拦截已挂载")
  else fail("全局 a 拦截缺失")
  // detailNav 无条件写入
  const dn = fs.readFileSync("src/utils/detailNav.ts", "utf8")
  if (dn.includes("listState?.top ?? 0")) ok("backState 无条件写入来源")
  else fail("backState 仍条件写入")
  // 详情页 handleBack 无 PWA push 首页分支
  const od = fs.readFileSync("src/views/online/OnlineDetail.vue", "utf8")
  if (!od.includes("isStandalonePWA()")) ok("OnlineDetail 已移除 PWA push 首页分支")
  else fail("OnlineDetail 仍有 PWA 分支")
  // App.vue fixed inset-0 + #app 铺满 + right-wrapper auto（iPad 底部窄长条）
  const app = fs.readFileSync("src/App.vue", "utf8")
  if (app.includes("position: fixed") && app.includes("inset: 0")) ok("App.vue app-container 已改 fixed inset-0（修底部窄条）")
  else fail("App.vue 未用 fixed inset-0")
  if (app.includes("#app") && app.includes("min-height: 100dvh")) ok("App.vue #app 根节点铺满（防 Letterbox）")
  else fail("App.vue #app 未铺满")
  if (app.includes("height: auto") && app.includes("min-height: 0")) ok("App.vue right-wrapper 改 flex 撑满（不再 100dvh 叠加）")
  else fail("App.vue right-wrapper 未改 auto")
  const idx = fs.readFileSync("index.html", "utf8")
  if (idx.includes("maximum-scale=1.0") && idx.includes("user-scalable=no")) ok("index.html viewport 补全（防 iOS Letterbox）")
  else fail("index.html viewport 未补全")
  // detailNav fromFullPath
  if (dn.includes("fromFullPath")) ok("detailNav 已记录 fullPath（含书架 id）")
  else fail("detailNav 缺 fromFullPath")
  // RandomView provider
  const rv = fs.readFileSync("src/views/RandomView.vue", "utf8")
  if (rv.includes("setListStateProvider('/random'")) ok("RandomView 注册列表状态 provider")
  else fail("RandomView 未注册 provider")
}

const browser = await chromium.launch({ executablePath: CHROME, headless: true })
try {
  await bug3(browser)
  await bookshelfBack(browser)
  await codeChecks()
} finally { await browser.close() }

console.log(pass ? "\n===== ROUND17 全部验证通过 =====" : "\n===== ROUND17 存在失败项 =====")
process.exit(pass ? 0 : 1)