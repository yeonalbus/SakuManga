// scripts/verify-round13.mjs
// Round13 回归验证：离线多选快捷加入书架 + 侧栏置顶/检索浮层
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
let COMIC_A, COMIC_B, SHELF_A, SHELF_B
let SHELF_C, SHELF_D, SHELF_E, SHELF_F, SHELF_G // 6 个用于上限测试
try {
  const login = await api("/auth/login", { method: "POST", body: { username: USER, password: PASS } })
  token = login.json.token || login.json.data?.token
  if (!token) throw new Error("登录失败")

  // 清空书架
  const shelves = await api("/bookshelves", { token })
  for (const s of shelves.json.bookshelves || []) await api("/bookshelves/" + s.id, { method: "DELETE", token })

  const comics = await api("/comics/offline", { token })
  if (!Array.isArray(comics.json) || comics.json.length < 2) throw new Error("离线漫画不足 2 本，请先扫描 MangaExamlpe")
  COMIC_A = comics.json[0]
  COMIC_B = comics.json[1]

  // 书架 A/B（加入测试用），书架 C~G（上限测试用）
  const mk = async (name) => { const r = await api("/bookshelves", { method: "POST", body: { name }, token }); return r.json.data.id }
  SHELF_A = await mk("测试书架A")
  SHELF_B = await mk("测试书架B")
  SHELF_C = await mk("上限书架C")
  SHELF_D = await mk("上限书架D")
  SHELF_E = await mk("上限书架E")
  SHELF_F = await mk("上限书架F")
  SHELF_G = await mk("上限书架G")
  log("SEED", `书架A/B + 上限书架 C~G（共 7 个）`)
} catch (e) {
  console.error("播种失败:", e.message)
  process.exit(1)
}

// ---------- Opt1：多选快捷加入书架 ----------
async function opt1(page) {
  await page.goto(BASE + "/offline/home")
  await page.waitForSelector(".item-card", { timeout: 20000 })
  await page.waitForTimeout(800)

  // 长按第一张卡片进入多选
  await page.locator(".item-card").first().dispatchEvent("pointerdown")
  await page.waitForTimeout(900) // 长按阈值
  await page.locator(".item-card").first().dispatchEvent("pointerup")
  await page.waitForSelector(".select-toolbar", { timeout: 8000 })
  ok("Opt1 长按进入多选模式")

  // 再选第二张
  await page.locator(".item-card").nth(1).click()
  await page.waitForTimeout(300)
  const selected = await page.locator(".select-count").textContent()
  if (selected && selected.includes("2")) ok("Opt1 已选 2 部: " + selected.trim())
  else fail("Opt1 已选数量异常: " + selected)

  // 加入书架A
  await page.locator(".toolbar-btn:has-text('加入书架')").click()
  await page.waitForSelector(".picker-panel", { timeout: 8000 })
  await page.locator(".picker-item:has-text('测试书架A') .item-main").click()
  await page.waitForTimeout(1200)
  const a = await api("/bookshelves", { token })
  const shelfA = (a.json.bookshelves || []).find((s) => s.id === SHELF_A)
  if (shelfA && shelfA.count === 2) ok("Opt1 书架A 加入 2 本（count=2）")
  else fail("Opt1 书架A count 异常: " + JSON.stringify(shelfA))

  // 再次选 1 本加入书架A → skipped
  await page.locator(".item-card").first().dispatchEvent("pointerdown")
  await page.waitForTimeout(900)
  await page.locator(".item-card").first().dispatchEvent("pointerup")
  await page.waitForSelector(".select-toolbar", { timeout: 8000 })
  await page.locator(".toolbar-btn:has-text('加入书架')").click()
  await page.waitForSelector(".picker-panel", { timeout: 8000 })
  await page.locator(".picker-item:has-text('测试书架A') .item-main").click()
  await page.waitForTimeout(1200)
  const a2 = await api("/bookshelves", { token })
  const shelfA2 = (a2.json.bookshelves || []).find((s) => s.id === SHELF_A)
  if (shelfA2 && shelfA2.count === 2) ok("Opt1 重复加入自动跳过（count 仍 2）")
  else fail("Opt1 重复加入后 count 异常: " + JSON.stringify(shelfA2))
  log("INFO", "Opt1 完成")
}

// ---------- Opt2：侧栏置顶 + 检索浮层 ----------
async function opt2(page) {
  await page.goto(BASE + "/offline/home")
  await page.waitForSelector(".item-card", { timeout: 20000 })
  await page.waitForTimeout(600)

  // 侧栏应显示「全部书架」入口，且无置顶书架时显示提示
  const hint = await page.locator(".pin-hint").count()
  if (hint > 0) ok("Opt2 无置顶时显示提示")
  else fail("Opt2 未显示置顶提示")

  // 打开全部书架浮层，置顶 5 个
  await page.locator(".all-shelf-btn").click()
  await page.waitForSelector(".picker-panel", { timeout: 8000 })
  // 搜索过滤定位
  await page.locator(".picker-search").fill("上限书架")
  await page.waitForTimeout(300)
  const limitItems = await page.locator(".picker-item").count()
  if (limitItems === 5) ok("Opt2 搜索过滤出 5 个上限书架")
  else fail("Opt2 搜索过滤数量异常: " + limitItems)

  // 逐个置顶（前 5 个）：只点「未置顶」书架的按钮，已置顶的按钮会变为「取消置顶」且位置不变
  for (let i = 0; i < 5; i++) {
    const unpinned = page.locator(".picker-item:not(.pinned) .mini-btn.pin").first()
    if ((await unpinned.count()) === 0) break
    await unpinned.click()
    await page.waitForTimeout(500)
  }
  await page.waitForTimeout(500)

  // 第 6 个置顶 → 应提示上限（清空搜索后看最后一个书架）
  await page.locator(".picker-search").fill("")
  await page.waitForTimeout(300)
  // 上限书架G 未被置顶（C~F 置顶了 4 个 + 测试书架A 可能被置顶？不，只置顶了搜索出的 5 个中的... 实际置顶的是 C~G 中前 5 个，即 C,D,E,F 加最后一个？
  // 简化断言：已置顶数量 = 5
  const pinnedNow = await page.locator(".picker-item.pinned").count()
  if (pinnedNow === 5) ok("Opt2 置顶 5 个成功")
  else fail("Opt2 置顶数量异常: " + pinnedNow)

  // 尝试再置顶一个未置顶的 → toast 上限提示
  const unpinned = page.locator(".picker-item:not(.pinned) .mini-btn.pin").first()
  if (await unpinned.count() > 0) {
    await unpinned.click()
    await page.waitForTimeout(600)
    const pinnedAfter = await page.locator(".picker-item.pinned").count()
    if (pinnedAfter === 5) ok("Opt2 第 6 个置顶被拦截（仍 5 个）")
    else fail("Opt2 第 6 个置顶未被拦截: " + pinnedAfter)
  } else {
    ok("Opt2 全部已置顶，跳过上限拦截断言")
  }

  // 关闭浮层，侧栏应显示 5 个置顶书架
  await page.locator(".picker-close").click()
  await page.waitForTimeout(500)
  const sidebarPinned = await page.locator(".foldable-body .sub-nav-item").count()
  if (sidebarPinned === 5) ok("Opt2 侧栏常驻 5 个置顶书架")
  else fail("Opt2 侧栏置顶书架数量异常: " + sidebarPinned)

  // 侧栏滚动：工具/系统组可见（滚到底部能找到「系统」）
  const sysVisible = await page.locator(".nav-group:has-text('⚙️ 系统')").count()
  if (sysVisible > 0) ok("Opt2 系统组仍在侧栏（可滚动到达）")
  else fail("Opt2 系统组缺失")
  log("INFO", "Opt2 完成")
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

await opt1(page)
await opt2(page)

await browser.close()
console.log(pass ? "\n===== ROUND13 全部验证通过 =====" : "\n===== ROUND13 存在失败项 =====")
process.exit(pass ? 0 : 1)