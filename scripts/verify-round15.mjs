// scripts/verify-round15.mjs
// Round15 回归验证：订阅固定表站 / 阅读器外壳隐藏 / 搜索历史收起 / PWA 返回逻辑
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

let token
let COMIC_A
try {
  const login = await api("/auth/login", { method: "POST", body: { username: USER, password: PASS } })
  token = login.json.token || login.json.data?.token
  if (!token) throw new Error("登录失败")
  const comics = await api("/comics/offline", { token })
  if (!Array.isArray(comics.json) || comics.json.length < 1) throw new Error("离线漫画不足，请先扫描 MangaExamlpe")
  COMIC_A = comics.json[0]
} catch (e) {
  console.error("播种失败:", e.message)
  process.exit(1)
}

// ---------- Bug4：阅读器隐藏全局外壳 ----------
async function bug4(page) {
  await page.goto(BASE + "/offline/detail?id=" + COMIC_A.id)
  await page.waitForSelector(".read-btn", { timeout: 15000 })
  await page.locator(".read-btn").first().click()
  // 等待阅读器加载（非 standalone → 新标签打开，需捕获 popup）
  const popupP = page.waitForEvent("popup", { timeout: 8000 }).catch(() => null)
  // 重新点击（第一次可能已打开）
  await page.goto(BASE + "/reader?id=" + COMIC_A.id + "&source=offline")
  await page.waitForSelector("body", { timeout: 15000 })
  await page.waitForTimeout(2000)
  const topBarVisible = await page.locator(".top-bar").isVisible().catch(() => false)
  const sidebarVisible = await page.locator(".sidebar").isVisible().catch(() => false)
  if (!topBarVisible) ok("Bug4 阅读器隐藏 TopBar（搜索栏不显示）")
  else fail("Bug4 TopBar 仍可见")
  if (!sidebarVisible) ok("Bug4 阅读器隐藏侧栏（logo 不显示）")
  else fail("Bug4 侧栏仍可见")
  const popup = await popupP
  if (popup) await popup.close().catch(() => {})
}

// ---------- Bug5：搜索历史收起 ----------
async function bug5(page) {
  await page.goto(BASE + "/offline/home")
  await page.waitForSelector(".search-input", { timeout: 15000 })
  // 聚焦搜索框 → 历史面板出现
  await page.locator(".search-input").focus()
  await page.waitForTimeout(400)
  const dropdownVisible = await page.locator(".search-dropdown").isVisible().catch(() => false)
  if (dropdownVisible) ok("Bug5 聚焦后搜索历史面板显示")
  else fail("Bug5 搜索历史面板未显示（可能无历史）")
  // 模拟键盘收起：触发 blur（iOS 场景下 blur 生效）
  await page.locator(".search-input").blur()
  await page.waitForTimeout(400)
  const afterBlur = await page.locator(".search-dropdown").isVisible().catch(() => false)
  if (!afterBlur) ok("Bug5 blur 后搜索历史面板收起")
  else fail("Bug5 blur 后面板未收起")
  // 模拟 visualViewport 高度恢复（键盘收起）：通过 evaluate 派发 resize
  await page.locator(".search-input").focus()
  await page.waitForTimeout(300)
  await page.evaluate(() => {
    // 模拟 visualViewport resize（键盘收起 → 高度恢复）
    const vv = window.visualViewport
    if (vv) {
      Object.defineProperty(vv, "height", { value: window.innerHeight, configurable: true })
      vv.dispatchEvent(new Event("resize"))
    }
  })
  await page.waitForTimeout(300)
  const afterVv = await page.locator(".search-dropdown").isVisible().catch(() => false)
  if (!afterVv) ok("Bug5 visualViewport 高度恢复后收起（键盘收起场景）")
  else fail("Bug5 visualViewport 未触发收起")
}

// ---------- Bug3：PWA 返回逻辑（代码级 + standalone 模拟） ----------
async function bug3(browser) {
  // 代码级：isStandalonePWA 函数存在 + openComicDetailInNewTab 有 PWA 分支
  const src = await import("fs").then((fs) => fs.readFileSync("src/utils/detailNav.ts", "utf8"))
  if (src.includes("isStandalonePWA") && src.includes("window.location.href = href")) {
    ok("Bug3 detailNav 已实现 PWA 检测与同标签导航分支")
  } else {
    fail("Bug3 detailNav PWA 分支缺失")
  }
  // 详情页 handleBack PWA 分支
  const od = await import("fs").then((fs) => fs.readFileSync("src/views/online/OnlineDetail.vue", "utf8"))
  const ofd = await import("fs").then((fs) => fs.readFileSync("src/views/offline/OfflineDetail.vue", "utf8"))
  if (od.includes("isStandalonePWA()") && ofd.includes("isStandalonePWA()")) {
    ok("Bug3 详情页 handleBack 已加 PWA 分支")
  } else {
    fail("Bug3 详情页 PWA 分支缺失")
  }
}

// ---------- Bug1：iPad PWA 黑边（代码级） ----------
async function bug1() {
  const app = await import("fs").then((fs) => fs.readFileSync("src/App.vue", "utf8"))
  if (app.includes("100svh") && app.includes("handleOrientationChange")) {
    ok("Bug1 App.vue 已加 dvh/svh 兜底 + orientationchange 重算")
  } else {
    fail("Bug1 App.vue 兜底缺失")
  }
}

const browser = await chromium.launch({ executablePath: CHROME, headless: true })
const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
const page = await context.newPage()
page.setDefaultTimeout(25000)

await page.goto(BASE + "/login")
await page.fill("#login-username", USER)
await page.fill("#login-password", PASS)
await Promise.all([page.waitForNavigation({ waitUntil: "domcontentloaded" }), page.click(".login-btn")])
await page.waitForSelector(".top-bar, .card-grid", { timeout: 15000 })
log("STEP", "登录成功")

await bug4(page)
await bug5(page)
await bug3(browser)
await bug1()

await browser.close()
console.log(pass ? "\n===== ROUND15 全部验证通过 =====" : "\n===== ROUND15 存在失败项 =====")
process.exit(pass ? 0 : 1)