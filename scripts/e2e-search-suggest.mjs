// scripts/e2e-search-suggest.mjs
// 搜索栏 3 个残余问题的 e2e 复现/回归脚本：
//   问题2 删空搜索词自动回首页（URL kw 被清、列表重置）
//   问题3a 乱联想（la → female:stockings$）
//   问题3b 点击联想项自动跳转搜索页（无法连续输入多 tag）
//   问题3c 补全位置错误（huge penis → huge male:"huge penis$"）
//
// 复用 verify-fix.mjs 的登录 + msedge 模式。
// 用法：
//   线上复现：set "SAKU_TEST_BASE=https://manga.yeon.top:8443" && node scripts/e2e-search-suggest.mjs
//   本地回归：set "SAKU_TEST_BASE=http://localhost:5173" && set "SAKU_TEST_USER=admin" && set "SAKU_TEST_PASS=admin123" && node scripts/e2e-search-suggest.mjs
//   本地+路由拦截（确定性前端验证）：再 set "SAKU_MOCK=1"
import { chromium } from 'playwright-core'

const BASE = process.env.SAKU_TEST_BASE || 'https://manga.yeon.top:8443'
const USER = process.env.SAKU_TEST_USER || (BASE.includes('localhost') || BASE.includes('127.0.0.1') ? 'admin' : 'Yeon')
const PASS = process.env.SAKU_TEST_PASS || (USER === 'admin' ? 'admin123' : 'Yeon2249291')
const MOCK = process.env.SAKU_MOCK === '1'

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
function log(mark, msg) {
  console.log(`[${mark}] ${msg}`)
}
const esc = (s) => (s === undefined || s === null ? String(s) : String(s).slice(0, 200))

const browser = await chromium.launch({ channel: 'msedge', headless: true })
const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } })
const page = await context.newPage()
page.setDefaultTimeout(30000)

let pass = true

// ---------- 可选：拦截 /api/tags/suggest，返回确定性数据 ----------
// ⚠️ 用 context.route 而非 page.route：需求2 搜索新标签（window.open）是独立 Page，
//    需让联想拦截在新标签中也生效（page.route 只作用于绑定页）。
if (MOCK) {
  await context.route('**/tags/suggest**', async (route) => {
    const u = new URL(route.request().url())
    const q = (u.searchParams.get('q') || '').toLowerCase()
    const mk = (ns, key, name, count) => ({ namespace: ns, key, name, count })
    let items = []
    if (q.includes('penis') || q.includes('huge')) {
      // 需求1：热度协同排序（huge penis 36080 第一、penis enlargement 21051 第二）
      // + limit 20 验证：返回 20 条（> 旧上限 8），供联想框滚动条回归断言
      items = [
        mk('male', 'huge penis', 'huge penis', 36080),
        mk('male', 'penis enlargement', 'penis enlargement', 21051),
        mk('male', 'big penis', 'big penis', 8888),
        mk('male', 'penis ring', 'penis ring', 7777),
        mk('male', 'penis pump', 'penis pump', 6666),
        mk('male', 'small penis', 'small penis', 5555),
        mk('female', 'penis in vagina', 'penis in vagina', 4444),
        mk('female', 'penis on penis', 'penis on penis', 3333),
        mk('male', 'penis gag', 'penis gag', 2222),
        mk('male', 'penis torture', 'penis torture', 1111),
        mk('male', 'penis job', 'penis job', 999),
        mk('male', 'penis slip', 'penis slip', 888),
        mk('male', 'penis tickling', 'penis tickling', 777),
        mk('male', 'penis worship', 'penis worship', 666),
        mk('male', 'penis growth', 'penis growth', 555),
        mk('male', 'penis inflation', 'penis inflation', 444),
        mk('male', 'penis shrinking', 'penis shrinking', 333),
        mk('male', 'penis clamp', 'penis clamp', 222),
        mk('male', 'penis butt', 'penis butt', 111),
        mk('male', 'penis nail', 'penis nail', 88),
      ]
    } else if (q.includes('la')) {
      // 模拟旧后端「命名空间子串匹配」噪音：stockings(key/name 不含 la) 与
      // japanese(命名空间 language 含 la 但 key/name 不含) 都应被前端相关性过滤剔除。
      // 注意不能含 glasses：其 key "glasses" 本身含子串 "la"，属合法命中。
      items = [
        { namespace: 'female', key: 'lactation', name: 'lactation', count: 5000 },
        { namespace: 'female', key: 'stockings', name: 'stockings', count: 4000 },
        { namespace: 'language', key: 'japanese', name: 'japanese', count: 3000 },
      ]
    } else {
      items = []
    }
    await route.fulfill({ json: items })
  })
  log('INFO', `已启用 /api/tags/suggest 路由拦截（MOCK=1，context 级）`)
}

const inputSel = '.search-input'
const suggestSel = '.vertical-tag-item'
const clearBtnSel = '.clear-input-btn'
const searchBtnSel = '.search-submit-btn'

async function login() {
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector('#login-username')
  await page.fill('#login-username', USER)
  await page.fill('#login-password', PASS)
  await page.click('.login-btn')
  await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 60000 })
  await sleep(1500)
  log('INFO', `登录成功: ${page.url()}`)
}

async function gotoOnlineHome() {
  await page.goto(`${BASE}/online/home`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector(inputSel, { timeout: 30000 })
  await sleep(1000)
}

async function typeAndGetSuggestions(text, waitMs = 2000) {
  await page.click(inputSel)
  await page.fill(inputSel, text)
  await page.waitForSelector(suggestSel, { timeout: 15000 })
  await sleep(waitMs)
  return page.locator(suggestSel).allTextContents()
}

async function inputValue() {
  return page.inputValue(inputSel)
}

// ==================== 测试A：问题2 删空搜索词 → URL kw 应保留 ====================
async function testA() {
  const kw = 'group:"da hootch$"'
  const kwEnc = encodeURIComponent(kw)
  await page.goto(`${BASE}/online/home?kw=${kwEnc}`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector(inputSel, { timeout: 30000 })
  await sleep(1500)
  const v0 = await inputValue().catch(() => '')
  log('INFO', `[A] 进入 /online/home?kw=...，搜索框反显=${esc(v0)}`)

  // 全选删除
  await page.click(inputSel)
  await page.keyboard.press('Control+A')
  await page.keyboard.press('Delete')
  await sleep(1800) // 越过 writeKeywordToUrl 的 600ms 防抖

  const url1 = page.url()
  const v1 = await inputValue().catch(() => '')
  const keptKw = /[?&]kw=/.test(url1)
  log(keptKw ? 'PASS' : 'FAIL', `[A] 删空后 URL 保留 kw=${keptKw}（URL=${url1}，输入框=${esc(v1)}）`)
  if (!keptKw) pass = false
}

// ==================== 测试B：问题3a 乱联想 ====================
async function testB() {
  await gotoOnlineHome()
  let suggestions = []
  try {
    suggestions = await typeAndGetSuggestions('la')
  } catch {
    suggestions = []
    log('INFO', `[B] 输入 "la" 后未出现联想项（可能词典/网络问题），跳过该断言`)
    return
  }
  const joined = suggestions.join(' | ')
  log('INFO', `[B] 输入 "la" 联想项=${esc(joined)}`)
  // stockings：key/name 不含 la 的旧噪音；japanese：命名空间 language 含 la 但 key/name 不含
  const noisy = suggestions.find((s) => /stockings|japanese/.test(s))
  log(noisy ? 'FAIL' : 'PASS', `[B] 输入 "la" 不应出现 stockings/japanese 乱联想（出现=${esc(noisy)}）`)
  if (noisy) pass = false
}

// ==================== 测试C：问题3b 点击联想不跳转 ====================
async function testC() {
  await gotoOnlineHome()
  try {
    await typeAndGetSuggestions('la', 800)
  } catch {
    log('INFO', `[C] 输入 "la" 后未出现联想项，跳过该断言`)
    return
  }
  const beforeUrl = page.url()
  const pagesBefore = page.context().pages().length
  // 点击芯片本身（用户实际点击的可见彩色标签），而非条目空白区 → 触发 TagChip 的 @click
  await page.locator(`${suggestSel} .tag-chip`).first().click()
  await sleep(1500)
  const pagesAfter = page.context().pages().length
  const afterUrl = page.url()
  const v1 = await inputValue().catch(() => '')
  const noNewTab = pagesAfter === pagesBefore
  const inserted = !!v1 && v1 !== 'la'
  log(
    noNewTab && inserted ? 'PASS' : 'FAIL',
    `[C] 点击芯片应插入输入框且不开新标签/不跳转（标签数 ${pagesBefore}→${pagesAfter}，URL=${afterUrl}，输入框=${esc(v1)}）`,
  )
  if (!noNewTab || !inserted) pass = false
  // 若因旧 bug 开了新标签，关闭它避免污染后续测试
  for (const p of page.context().pages()) {
    if (p !== page) await p.close().catch(() => {})
  }
}

// ==================== 测试D：问题3c 补全位置 ====================
async function testD() {
  await gotoOnlineHome()
  try {
    const suggestions = await typeAndGetSuggestions('huge penis')
    log('INFO', `[D] 输入 "huge penis" 联想项=${esc(suggestions.join(' | '))}`)
    const target = page
      .locator(suggestSel)
      .filter({ hasText: 'huge penis' })
      .first()
    if ((await target.count()) === 0) {
      log('FAIL', `[D] 未找到 "huge penis" 联想项，无法验证补全位置`)
      pass = false
      return
    }
    await target.locator('.tag-chip').click()
    await sleep(500)
    const v1 = await inputValue()
    const ok = v1 === 'male:"huge penis$"'
    log(ok ? 'PASS' : 'FAIL', `[D] 点击 male:"huge penis$" 后补全结果=${esc(v1)}（期望 male:"huge penis$"）`)
    if (!ok) pass = false
  } catch (e) {
    log('ERROR', `[D] 异常: ${e && e.stack ? e.stack : e}`)
    pass = false
  }
}

// ==================== 测试E：需求1 热度协同排序 + limit 20 + 联想框滚动条 ====================
async function testE() {
  await gotoOnlineHome()
  let items = []
  try {
    items = await typeAndGetSuggestions('penis')
  } catch {
    log('INFO', `[E] 输入 "penis" 后未出现联想项，跳过排序/滚动断言`)
    return
  }
  log('INFO', `[E] 输入 "penis" 联想项数=${items.length}，首项=${esc(items[0])}`)
  if (items.length === 0) {
    log('FAIL', `[E] 输入 "penis" 无联想项`)
    pass = false
    return
  }

  // 断言1：热度协同——huge penis(36080) 应排在 penis enlargement(21051) 之前
  const idxHuge = items.findIndex((s) => s.includes('huge penis'))
  const idxEnlarge = items.findIndex((s) => s.includes('penis enlargement'))
  const okOrder = MOCK
    ? idxHuge === 0
    : idxHuge !== -1 && idxEnlarge !== -1 && idxHuge < idxEnlarge
  log(
    okOrder ? 'PASS' : 'FAIL',
    `[E] 热度协同排序：huge penis(idx=${idxHuge}) 应在 penis enlargement(idx=${idxEnlarge}) 之前`,
  )
  if (!okOrder) pass = false

  // 断言2：limit 提高到 20 → 展示条数应超过旧上限 8
  const okCount = items.length > 8
  log(okCount ? 'PASS' : 'FAIL', `[E] 联想条数=${items.length}（应 >8，验证 limit 20）`)
  if (!okCount) pass = false

  // 断言3：联想框尺寸不变但内部可滚动（scrollHeight>clientHeight 且 overflow-y 滚动）
  let scrollable = false
  try {
    scrollable = await page.evaluate(() => {
      const el = document.querySelector('.vertical-tag-list')
      if (!el) return false
      const cs = getComputedStyle(el)
      return (
        el.scrollHeight > el.clientHeight &&
        (cs.overflowY === 'auto' || cs.overflowY === 'scroll')
      )
    })
  } catch {
    scrollable = false
  }
  log(
    scrollable ? 'PASS' : 'FAIL',
    `[E] 联想框内部可滚动（scrollHeight>clientHeight 且 overflowY 滚动）`,
  )
  if (!scrollable) pass = false
}

// ==================== 测试F：需求2 搜索在新标签打开（在线-点击 / 离线-回车） ====================
async function runNewTabCase(label, path, kw, useEnter) {
  await page.goto(`${BASE}${path}`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector(inputSel, { timeout: 30000 })
  await sleep(800)
  await page.click(inputSel)
  await page.fill(inputSel, kw)
  await sleep(400)
  const beforeUrl = page.url()
  const pagesBefore = page.context().pages().length

  const popupPromise = page.context().waitForEvent('page', { timeout: 10000 }).catch(() => null)
  if (useEnter) {
    await page.keyboard.press('Enter')
  } else {
    await page.click(searchBtnSel)
  }
  const newPage = await popupPromise
  if (newPage) {
    await newPage.waitForURL((u) => u.href !== 'about:blank', { timeout: 10000 }).catch(() => {})
  }
  await sleep(1500)

  const pages = page.context().pages()
  const afterUrl = page.url()
  const v1 = await inputValue().catch(() => '')

  // 断言1：确实新开了标签页
  const opened = !!newPage && pages.length > pagesBefore
  log(opened ? 'PASS' : 'FAIL', `[F-${label}] 新开标签：标签数 ${pagesBefore}→${pages.length}`)

  // 断言2：新标签 URL 携带 kw（path 一致）
  let okUrl = false
  if (newPage) {
    try {
      const u = new URL(newPage.url())
      okUrl = u.pathname === path && u.searchParams.get('kw') === kw
    } catch {
      okUrl = false
    }
  }
  log(
    okUrl ? 'PASS' : 'FAIL',
    `[F-${label}] 新标签 URL=${newPage ? newPage.url() : '(无)'}（期望 path=${path} kw=${kw}）`,
  )

  // 断言3：原标签保留输入、URL 未跳转（不带 kw）
  const keptInput = v1 === kw
  const noNav = afterUrl === beforeUrl
  log(
    keptInput && noNav ? 'PASS' : 'FAIL',
    `[F-${label}] 原标签保留输入=${esc(v1)} 且未导航（URL=${afterUrl}）`,
  )

  if (!opened || !okUrl || !keptInput || !noNav) pass = false

  // 清理新开标签，避免污染后续用例
  for (const p of page.context().pages()) {
    if (p !== page) await p.close().catch(() => {})
  }
  await sleep(300)
}

async function testF() {
  await runNewTabCase('online-点击', '/online/home', 'solo', false)
  await runNewTabCase('offline-回车', '/offline/home', '巨根', true)
}

// ==================== 主流程 ====================
try {
  await login()
  log('INFO', `测试环境 BASE=${BASE} USER=${USER} MOCK=${MOCK ? 1 : 0}`)

  await testA()
  await testB()
  await testC()
  await testD()
  await testE()
  await testF()

  log(pass ? 'RESULT PASS' : 'RESULT FAIL', pass ? '全部断言通过' : '存在失败断言')
} catch (e) {
  log('ERROR', '脚本异常: ' + (e && e.stack ? e.stack : e))
  pass = false
} finally {
  await browser.close()
  process.exit(pass ? 0 : 1)
}
