// scripts/verify-fix.mjs
// 修复验证脚本：确认「配额写满时设置仍能持久化（刷新不丢）」。
// 与 diag-persist.mjs（复现脚本）同构，但 reload 后直接读 localStorage（不依赖设置页 UI），
// 规避本地 vite preview 将 /api 相对路径代理到本机 8081 导致页面卡顿的干扰。
// 用法：set "SAKU_TEST_BASE=http://localhost:5198" && node scripts/verify-fix.mjs
import { chromium } from 'playwright-core'

const BASE = process.env.SAKU_TEST_BASE || 'https://manga.yeon.top:8443'
const USER = process.env.SAKU_TEST_USER || 'Yeon'
const PASS = process.env.SAKU_TEST_PASS || 'Yeon2249291'

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
function log(mark, msg) {
  console.log(`[${mark}] ${msg}`)
}

const getPrefRaw = (page) =>
  page.evaluate(() => {
    try {
      return localStorage.getItem('saku_preference_settings')
    } catch {
      return null
    }
  })

const getPref = (page) =>
  page.evaluate(() => {
    try {
      return JSON.parse(localStorage.getItem('saku_preference_settings') || 'null')
    } catch {
      return 'PARSE_ERR'
    }
  })

const favSelect = (page) => page.locator('select.setting-select').filter({ hasText: 'Fav' })
const setFavFolder = (page, label) => favSelect(page).selectOption({ label })

async function openPreferenceTab(page) {
  await page.goto(`${BASE}/settings?tab=preference`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector('select.setting-select', { timeout: 30000 })
  await sleep(600)
}

const browser = await chromium.launch({ channel: 'msedge', headless: true })
const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } })
const page = await context.newPage()
page.setDefaultTimeout(30000)

let pass = true
try {
  // ---------- 登录 ----------
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector('#login-username')
  await page.fill('#login-username', USER)
  await page.fill('#login-password', PASS)
  await page.click('.login-btn')
  await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 60000 })
  await sleep(1500)
  log('INFO', `登录成功: ${page.url()}`)

  // ---------- 用恰好 300 条进度 map 填满配额 ----------
  const fill = await page.evaluate(() => {
    const big = {}
    for (let j = 0; j < 300; j++) big['online:' + (1000000000000 + j)] = j * 7 + 1
    const bigRaw = JSON.stringify(big)
    let bi = 0
    for (;;) {
      try {
        localStorage.setItem('saku_comic_progress:vfybig' + bi, bigRaw)
        bi++
      } catch {
        break
      }
    }
    let si = 0
    for (;;) {
      try {
        localStorage.setItem('saku_comic_progress:vfysmall' + si, '{"online:1":1}')
        si++
      } catch {
        break
      }
    }
    return { bi, si }
  })
  const probe = await page.evaluate(() => {
    try {
      localStorage.setItem('__vfyprobe', 'x'.repeat(50))
      localStorage.removeItem('__vfyprobe')
      return true
    } catch {
      return false
    }
  })
  log(probe ? 'FAIL' : 'PASS', `配额已写满（大map ${fill.bi} 小map ${fill.si}）`)
  if (probe) pass = false

  // ---------- 删除设置键 + 重填，模拟「新增键写入」在最坏时刻 ----------
  await page.evaluate(() => {
    localStorage.removeItem('saku_preference_settings')
    let i = 0
    for (;;) {
      try {
        localStorage.setItem('saku_comic_progress:vfyrefill' + i, '{"online:2":2}')
        i++
      } catch {
        break
      }
    }
    return i
  })
  const probe2 = await page.evaluate(() => {
    try {
      localStorage.setItem('__vfyprobe2', 'x'.repeat(50))
      localStorage.removeItem('__vfyprobe2')
      return true
    } catch {
      return false
    }
  })
  log(probe2 ? 'FAIL' : 'PASS', `重填后配额仍满`)
  if (probe2) pass = false

  // ---------- 配额满时改默认收藏夹（= 新增键写入，修复前必丢） ----------
  await openPreferenceTab(page)
  await setFavFolder(page, 'Fav 7')
  await sleep(800)
  const pref1 = await getPref(page)
  const ok1 = pref1 && pref1.defaultFavFolder === 7
  log(ok1 ? 'PASS' : 'FAIL', `配额满时新增键写入：localStorage defaultFavFolder=${pref1?.defaultFavFolder}（期望 7）`)
  if (!ok1) pass = false

  // ---------- 刷新后持久化确认（直接读 localStorage，不依赖 UI） ----------
  await page.reload({ waitUntil: 'domcontentloaded' })
  await sleep(1200)
  const pref2 = await getPref(page)
  const ok2 = pref2 && pref2.defaultFavFolder === 7
  log(ok2 ? 'PASS' : 'FAIL', `刷新后：localStorage defaultFavFolder=${pref2?.defaultFavFolder}（期望 7）`)
  if (!ok2) pass = false
  const hasToken = await page.evaluate(() => Boolean(localStorage.getItem('saku_token')))
  log('INFO', `刷新后 saku_token 存在=${hasToken}，当前 URL=${page.url()}`)

  // ---------- 清理 ----------
  await page.evaluate(() => {
    for (let i = localStorage.length - 1; i >= 0; i--) {
      const k = localStorage.key(i)
      if (k && (k.startsWith('saku_comic_progress:') || k.startsWith('__'))) localStorage.removeItem(k)
    }
  })

  log(pass ? 'RESULT PASS' : 'RESULT FAIL', pass ? '修复验证通过：配额写满时设置仍能持久化，刷新不丢' : '修复验证未通过')
} catch (e) {
  log('ERROR', '脚本异常: ' + (e && e.stack ? e.stack : e))
  pass = false
} finally {
  await browser.close()
  process.exit(pass ? 0 : 1)
}
