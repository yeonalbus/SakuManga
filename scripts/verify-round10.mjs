// scripts/verify-round10.mjs
// Round10 回归验证（自包含：开头自动重置并播种测试数据）
//   书架1 含 c1,c2（c1=4星）；书架2 含 c2；离线清单含 c1,c2
// 用法：node scripts/verify-round10.mjs （需本地后端 127.0.0.1:8081 已启动）
import { chromium } from 'playwright-core'

const BASE = 'http://127.0.0.1:8081'
const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const USER = 'admin'
const PASS = 'admin123'

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
function log(mark, msg) {
  console.log(`[${mark}] ${msg}`)
}
let pass = true
const fail = (msg) => {
  pass = false
  log('FAIL', msg)
}
const ok = (msg) => log('OK', msg)

async function api(path, { method = 'GET', body, token } = {}) {
  const headers = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = 'Bearer ' + token
  const res = await fetch(BASE + '/api/v1' + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const text = await res.text()
  let json = null
  try { json = JSON.parse(text) } catch { /* ignore */ }
  return { status: res.status, json }
}

// ---------- 播种测试数据 ----------
let token
try {
  const login = await api('/auth/login', { method: 'POST', body: { username: USER, password: PASS } })
  token = login.json.token || login.json.data?.token
  if (!token) throw new Error('登录失败: ' + JSON.stringify(login.json))

  // 清空旧书架
  const shelves = await api('/bookshelves', { token })
  for (const s of shelves.json.bookshelves || []) {
    await api('/bookshelves/' + s.id, { method: 'DELETE', token })
  }
  // 清空离线清单
  await api('/reading-list', { method: 'PUT', body: { source: 'offline', items: [] }, token })

  // 重建书架
  const s1 = await api('/bookshelves', { method: 'POST', body: { name: '测试书架1' }, token })
  const s2 = await api('/bookshelves', { method: 'POST', body: { name: '测试书架2' }, token })
  const SHELF1 = s1.json.data.id
  const SHELF2 = s2.json.data.id

  const comics = await api('/comics/offline', { token })
  if (!Array.isArray(comics.json) || comics.json.length < 2) {
    throw new Error('离线漫画不足 2 本，请先扫描 MangaExamlpe')
  }
  const c1 = comics.json[0]
  const c2 = comics.json[1]
  const C1 = c1.id

  await api('/bookshelves/' + SHELF1 + '/comics', { method: 'POST', body: { comicId: c1.id }, token })
  await api('/bookshelves/' + SHELF1 + '/comics', { method: 'POST', body: { comicId: c2.id }, token })
  await api('/bookshelves/' + SHELF2 + '/comics', { method: 'POST', body: { comicId: c2.id }, token })
  await api('/ratings/' + c1.id, { method: 'PUT', body: { score: 4 }, token })
  await api('/reading-list', { method: 'PUT', body: { source: 'offline', items: [c1, c2] }, token })

  // 导出到全局
  globalThis.__SHELF1 = SHELF1
  globalThis.__SHELF2 = SHELF2
  globalThis.__C1 = C1
  log('SEED', '测试数据已播种: 书架1=' + SHELF1 + ' 书架2=' + SHELF2)
} catch (e) {
  console.error('播种失败:', e.message)
  process.exit(1)
}

const SHELF1 = globalThis.__SHELF1
const C1 = globalThis.__C1

const browser = await chromium.launch({ executablePath: CHROME, headless: true })
const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
const page = await context.newPage()
page.setDefaultTimeout(25000)

// ---------- 登录 ----------
await page.goto(BASE + '/login')
await page.fill('#login-username', USER)
await page.fill('#login-password', PASS)
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }), page.click('.login-btn')])
await page.waitForSelector('.card-grid, .offline-home-view, .top-bar', { timeout: 15000 })
log('STEP', '登录成功，落地页加载')

// ---------- Bug1：直接打开书架页（全新标签），不应为空 ----------
const page2 = await context.newPage()
await page2.goto(BASE + '/offline/bookshelf?id=' + SHELF1)
await page2.waitForTimeout(2500)
const cards1 = await page2.locator('.item-card').count()
if (cards1 >= 2) ok(`Bug1 直接刷新书架页显示 ${cards1} 张卡片（>=2）`)
else fail(`Bug1 书架页卡片数异常: ${cards1}`)
await page2.close()

// ---------- Bug2：离线首页卡片显示个人评分 ⭐ 4.0 ----------
await page.goto(BASE + '/offline/home')
await page.waitForSelector('.item-card', { timeout: 15000 })
await page.waitForTimeout(1200)
const cardTitles = await page.locator('.item-card').allTextContents()
const ratingTexts = await page.locator('.item-card .rating, .item-card .rating-text').allTextContents()
log('INFO', '卡片评分文本: ' + ratingTexts.join(' | '))
const has4 = ratingTexts.some((t) => t.includes('4.0'))
if (has4) ok('Bug2 已评分卡片显示 ⭐ 4.0')
else fail('Bug2 未找到 ⭐ 4.0 评分卡片（实际: ' + ratingTexts.join(' | ') + '）')

// ---------- Bug3：筛选抽屉最低评分 3 星，只剩已评分卡片 ----------
const filterTrigger = page.locator('.filter-trigger-btn')
if (await filterTrigger.count() > 0) {
  await filterTrigger.first().click()
  await page.waitForSelector('.filter-drawer', { timeout: 8000 })
  const select = page.locator('.filter-drawer select.dark-select.mini')
  if (await select.count() > 0) {
    await select.first().selectOption({ label: '3 ⭐' })
    await page.click('.filter-drawer .apply-btn')
    await page.waitForTimeout(1200)
    const after = await page.locator('.item-card').count()
    if (after === 1) ok(`Bug3 3星筛选后剩 ${after} 张卡片（仅已评分）`)
    else fail(`Bug3 3星筛选后卡片数异常: ${after}`)
    // 清空筛选（重置 + 应用）
    await filterTrigger.first().click()
    await page.waitForSelector('.filter-drawer', { timeout: 8000 })
    const resetBtn = page.locator('.filter-drawer .icon-btn[title="重置筛选"]')
    if (await resetBtn.count() > 0) await resetBtn.first().click()
    await page.click('.filter-drawer .apply-btn')
    await page.waitForTimeout(800)
  } else {
    fail('筛选抽屉未找到最低评分下拉')
  }
} else {
  fail('未找到筛选按钮（.filter-trigger-btn）')
}

// ---------- Opt1a：阅读清单 ↑/↓ 排序并持久化 ----------
await page.goto(BASE + '/reading-list')
await page.waitForSelector('.tab-btn', { timeout: 15000 })
// 默认显示「在线清单」tab，切到「本地清单」验证离线队列
await page.locator('.tab-btn:has-text("本地清单")').click()
await page.waitForSelector('.mini-card', { timeout: 15000 })
await page.waitForTimeout(800)
let titles = await page.locator('.mini-card .title').allTextContents()
log('INFO', '清单初始顺序: ' + titles.map((t) => t.slice(0, 12)).join(' | '))
// 第一行点 ↓（若第一行已是最后一行则跳过）
const firstMoveDown = page.locator('.mini-card').first().locator('.move-btn').nth(1)
await firstMoveDown.click()
await page.waitForTimeout(800)
let titles2 = await page.locator('.mini-card .title').allTextContents()
log('INFO', '清单下移后顺序: ' + titles2.map((t) => t.slice(0, 12)).join(' | '))
await page.reload()
await page.waitForSelector('.tab-btn', { timeout: 15000 })
await page.locator('.tab-btn:has-text("本地清单")').click()
await page.waitForSelector('.mini-card', { timeout: 15000 })
await page.waitForTimeout(800)
let titles3 = await page.locator('.mini-card .title').allTextContents()
if (titles2.join('|') === titles3.join('|') && titles.join('|') !== titles2.join('|')) {
  ok('Opt1a 清单排序生效且刷新后保持')
} else {
  fail('Opt1a 清单排序未持久化: ' + titles3.map((t) => t.slice(0, 12)).join(' | '))
}

// ---------- Opt2：单本移出（只影响清单） ----------
await page.locator('.mini-card').first().locator('.remove-btn').click()
await page.waitForTimeout(800)
const afterRemove = await page.locator('.mini-card').count()
if (afterRemove === 1) ok('Opt2 单本移出成功（清单从 2 本剩 1 本）')
else fail('Opt2 移出后清单数量异常: ' + afterRemove)
// 本地库不受影响
const offCount = await (await fetch(BASE + '/api/v1/comics/offline', { headers: { Authorization: 'Bearer ' + (await getToken()) } })).json()
if (offCount.length === 2) ok('Opt2 本地库未受影响（仍 2 本）')
else fail('Opt2 本地库数量异常: ' + offCount.length)

// ---------- Opt1b：书架内项目排序 ----------
await page.goto(BASE + '/offline/bookshelf?id=' + SHELF1)
await page.waitForSelector('.shelf-op-btn', { timeout: 15000 })
await page.waitForTimeout(1000)
const beforeTitles = await page.locator('.item-card .card-title, .item-card .compact-title').allTextContents()
log('INFO', '书架初始顺序: ' + beforeTitles.map((t) => t.slice(0, 12)).join(' | '))
await page.locator('.shelf-op-btn:has-text("排序")').click()
await page.waitForSelector('.sort-mini-list', { timeout: 8000 })
// 第一行 ↓
await page.locator('.sort-mini-card').first().locator('.move-btn').nth(1).click()
await page.waitForTimeout(300)
await page.locator('.sort-actions .toolbar-btn.primary').click()
await page.waitForTimeout(1500)
const afterTitles = await page.locator('.item-card .card-title, .item-card .compact-title').allTextContents()
log('INFO', '书架排序后顺序: ' + afterTitles.map((t) => t.slice(0, 12)).join(' | '))
await page.reload()
await page.waitForSelector('.item-card', { timeout: 15000 })
await page.waitForTimeout(1500)
const reloadTitles = await page.locator('.item-card .card-title, .item-card .compact-title').allTextContents()
if (afterTitles.join('|') === reloadTitles.join('|') && afterTitles.join('|') !== beforeTitles.join('|')) {
  ok('Opt1b 书架内项目排序生效且刷新后保持')
} else {
  fail('Opt1b 书架排序未持久化: ' + reloadTitles.map((t) => t.slice(0, 12)).join(' | '))
}

// ---------- Opt1c：侧栏书架顺序 ↑/↓ 并持久化 ----------
{
  await page.goto(BASE + '/offline/bookshelf?id=' + SHELF1)
  await page.waitForSelector('.sub-nav-item', { timeout: 15000 })
  await page.waitForTimeout(800)
  const shelfNamesBefore = await page.locator('.sub-nav-item .shelf-name').allTextContents()
  log('INFO', '侧栏书架顺序: ' + shelfNamesBefore.join(' | '))
  if (shelfNamesBefore.length >= 2) {
    // 第一个书架点 ↓
    const firstDown = page.locator('.sub-nav-item').first().locator('.move-btn').nth(1)
    await firstDown.click()
    await page.waitForTimeout(800)
    const shelfNamesMid = await page.locator('.sub-nav-item .shelf-name').allTextContents()
    await page.reload()
    await page.waitForSelector('.sub-nav-item', { timeout: 15000 })
    await page.waitForTimeout(800)
    const shelfNamesAfter = await page.locator('.sub-nav-item .shelf-name').allTextContents()
    if (shelfNamesMid.join('|') === shelfNamesAfter.join('|') && shelfNamesMid.join('|') !== shelfNamesBefore.join('|')) {
      ok('Opt1c 侧栏书架顺序调整生效且刷新后保持: ' + shelfNamesAfter.join(' | '))
    } else {
      fail('Opt1c 侧栏书架顺序未持久化: ' + shelfNamesAfter.join(' | '))
    }
  } else {
    fail('Opt1c 侧栏书架不足 2 个，无法验证顺序')
  }
}

// ---------- Opt3：改名 + 删除（header 入口） ----------
await page.locator('.shelf-op-btn:has-text("改名")').click()
await page.waitForSelector('.modal-input', { timeout: 8000 })
// prompt 输入框
const promptInput = page.locator('.modal-input')
if (await promptInput.count() > 0) {
  await promptInput.first().fill('改名书架X')
  await page.locator('.modal-actions .btn-confirm').first().click()
  await page.waitForTimeout(1000)
  const headerTitle = await page.locator('.shelf-title').textContent()
  if (headerTitle.includes('改名书架X')) ok('Opt3 header 改名成功: ' + headerTitle)
  else fail('Opt3 header 改名未生效: ' + headerTitle)
} else {
  fail('Opt3 改名弹窗输入框未找到')
}
// 删除（书架空：先建临时书架删除）
await page.goto(BASE + '/offline/bookshelf?id=' + SHELF1)
await page.waitForSelector('.shelf-op-btn', { timeout: 15000 })
await page.locator('.shelf-op-btn:has-text("删除")').click()
await page.waitForSelector('.modal-actions .btn-confirm', { timeout: 8000 })
await page.locator('.modal-actions .btn-confirm').first().click()
await page.waitForURL('**/offline/home**', { timeout: 8000 })
ok('Opt3 header 删除书架后跳回离线首页')

await browser.close()
console.log(pass ? '\n===== ROUND10 全部验证通过 =====' : '\n===== ROUND10 存在失败项 =====')
process.exit(pass ? 0 : 1)

async function getToken() {
  const res = await fetch(BASE + '/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: USER, password: PASS }),
  })
  const j = await res.json()
  return j.token || j.data?.token
}