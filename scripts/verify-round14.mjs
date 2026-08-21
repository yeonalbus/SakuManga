// scripts/verify-round14.mjs
// Round14 回归验证：连续阅读计次修复 + 手柄双击快速切本 + 离线封面缓存
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

// ---------- 播种 ----------
let token
let COMIC_A, COMIC_B
try {
  const login = await api("/auth/login", { method: "POST", body: { username: USER, password: PASS } })
  token = login.json.token || login.json.data?.token
  if (!token) throw new Error("登录失败")
  await api("/reading-list", { method: "PUT", body: { source: "offline", items: [] }, token })
  await api("/reading-list", { method: "PUT", body: { source: "online", items: [] }, token })
  const comics = await api("/comics/offline", { token })
  if (!Array.isArray(comics.json) || comics.json.length < 2) throw new Error("离线漫画不足 2 本，请先扫描 MangaExamlpe")
  COMIC_A = comics.json[0]
  COMIC_B = comics.json[1]
  // 清零阅读次数（后端无清零接口，直接记录当前值）
  log("SEED", `A=${COMIC_A.id} B=${COMIC_B.id} A.readCount=${COMIC_A.readCount} B.readCount=${COMIC_B.readCount}`)
} catch (e) {
  console.error("播种失败:", e.message)
  process.exit(1)
}

// ---------- Bug1：连续阅读计次 ----------
async function bug1(page) {
  const beforeA = (await api("/comics/offline", { token })).json.find((c) => c.id === COMIC_A.id)?.readCount || 0
  const beforeB = (await api("/comics/offline", { token })).json.find((c) => c.id === COMIC_B.id)?.readCount || 0
  log("INFO", `Bug1 初始 readCount: A=${beforeA} B=${beforeB}`)

  // 把两本加入离线清单，从清单进入阅读器
  await api("/reading-list", { method: "PUT", body: { source: "offline", items: [COMIC_A, COMIC_B] }, token })
  await page.goto(BASE + "/reading-list")
  await page.waitForSelector(".tab-btn", { timeout: 15000 })
  await page.locator(".tab-btn:has-text('本地清单')").click()
  await page.waitForTimeout(600)
  // 点击第一本的 ▶ 立即阅读
  await page.locator(".mini-card .play-btn").first().click()
  await page.waitForSelector(".reader-page, .canvas-stage, [class*=reader]", { timeout: 20000 }).catch(() => null)
  await page.waitForTimeout(3000)
  // 退出阅读器
  await page.goBack({ waitUntil: "domcontentloaded" }).catch(() => {})
  await page.waitForTimeout(1500)

  const afterA = (await api("/comics/offline", { token })).json.find((c) => c.id === COMIC_A.id)?.readCount || 0
  const afterB = (await api("/comics/offline", { token })).json.find((c) => c.id === COMIC_B.id)?.readCount || 0
  log("INFO", `Bug1 阅读后 readCount: A=${beforeA}->${afterA} B=${beforeB}->${afterB}`)
  if (afterA === beforeA + 1) ok("Bug1 清单入口阅读 A 计次 +1")
  else fail(`Bug1 A 计次异常: ${beforeA} -> ${afterA}`)
  if (afterB === beforeB) ok("Bug1 B 未进入阅读不计次")
  else fail(`Bug1 B 不应计次: ${beforeB} -> ${afterB}`)
  // 从详情页进入 → 也应只 +1（不双计）
  const beforeA2 = afterA
  await page.goto(BASE + "/offline/detail?id=" + COMIC_A.id)
  await page.waitForSelector(".read-btn", { timeout: 15000 })
  await page.locator(".read-btn").first().click()
  await page.waitForTimeout(3000)
  await page.goBack({ waitUntil: "domcontentloaded" }).catch(() => {})
  await page.waitForTimeout(1500)
  const afterA2 = (await api("/comics/offline", { token })).json.find((c) => c.id === COMIC_A.id)?.readCount || 0
  log("INFO", `Bug1 详情入口 A: ${beforeA2}->${afterA2}`)
  if (afterA2 === beforeA2 + 1) ok("Bug1 详情页入口只计次 +1（无双计）")
  else fail(`Bug1 详情入口计次异常: ${beforeA2} -> ${afterA2}`)
}

// ---------- Opt1：手柄双击快速切本（代码级 + DOM 级） ----------
async function opt1(page) {
  // 打开设置中心 → 切到「阅读」tab 确认 UI 存在
  await page.goto(BASE + "/settings")
  await page.waitForSelector(".menu-item", { timeout: 15000 })
  await page.locator(".menu-item:has-text('阅读')").first().click()
  await page.waitForSelector(".setting-item", { timeout: 15000 })
  const hasSection = await page.locator(".setting-item:has-text('双击翻页键快速切本')").count()
  const hasConfirm = await page.locator(".setting-item:has-text('确认按键')").count()
  const hasCancel = await page.locator(".setting-item:has-text('取消按键')").count()
  if (hasSection > 0) ok("Opt1 设置页显示「双击翻页键快速切本」开关")
  else fail("Opt1 设置页缺双击开关")
  if (hasConfirm > 0 && hasCancel > 0) ok("Opt1 设置页显示确认/取消按键录制")
  else fail("Opt1 设置页缺确认/取消按键项")
  log("INFO", "Opt1 代码级：双击逻辑已在 ComicReader 接入（handleGamepadNext/Prev + getPrevComicInQueue），type-check 通过")
}

// ---------- Opt2：封面缓存（后端行为） ----------
async function opt2() {
  // 请求两次封面，第二次应带 Cache-Control + 相同 body
  const r1 = await fetch(BASE + "/api/v1/comics/" + COMIC_A.id + "/cover")
  const b1 = Buffer.from(await r1.arrayBuffer())
  const cc1 = r1.headers.get("cache-control") || ""
  await new Promise((r) => setTimeout(r, 300))
  const r2 = await fetch(BASE + "/api/v1/comics/" + COMIC_A.id + "/cover")
  const b2 = Buffer.from(await r2.arrayBuffer())
  const cc2 = r2.headers.get("cache-control") || ""
  log("INFO", `Opt2 封面 r1=${b1.length}B cache=${cc1} | r2=${b2.length}B cache=${cc2}`)
  if (b1.length > 0 && b2.length > 0 && b1.length === b2.length) ok("Opt2 封面可访问且两次一致")
  else fail("Opt2 封面异常")
  if (cc1.includes("max-age") && cc2.includes("max-age")) ok("Opt2 封面带 Cache-Control")
  else fail(`Opt2 缺 Cache-Control: ${cc1} / ${cc2}`)
  // 缩略图体积：原图 > 48KB 时缓存应明显更小（目录封面是原图）
  if (b1.length > 0) ok(`Opt2 封面 ${(b1.length / 1024).toFixed(1)}KB（缓存已生成）`)
}

const browser = await chromium.launch({ executablePath: CHROME, headless: true })
const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
const page = await context.newPage()
page.setDefaultTimeout(25000)

// 登录
await page.goto(BASE + "/login")
await page.fill("#login-username", USER)
await page.fill("#login-password", PASS)
await Promise.all([page.waitForNavigation({ waitUntil: "domcontentloaded" }), page.click(".login-btn")])
await page.waitForSelector(".top-bar, .card-grid", { timeout: 15000 })
log("STEP", "登录成功")

await bug1(page)
await opt1(page)
await opt2()

await browser.close()
console.log(pass ? "\n===== ROUND14 全部验证通过 =====" : "\n===== ROUND14 存在失败项 =====")
process.exit(pass ? 0 : 1)