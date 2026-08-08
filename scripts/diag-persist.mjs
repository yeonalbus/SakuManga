// scripts/diag-persist.mjs
// 持久化 bug 浏览器诊断脚本（Playwright-core + 系统 Edge channel 'msedge'）
// 针对 https://manga.yeon.top:8443 线上构建验证偏好设置持久化行为：
//   A. 干净状态：设置「默认收藏夹」→ 刷新 → 应保留（证明线上代码正常）
//   C. 配额写满（用恰好 300 条的进度 map 填充，trimProgressMaps 不会裁剪）：
//      删掉设置键后改「默认收藏夹」（= 新增键写入）→ 刷新 → 预期丢失（复现用户症状）
// 用法：node scripts/diag-persist.mjs
// 环境变量：SAKU_TEST_USER / SAKU_TEST_PASS 可覆盖账号（默认 Yeon / Yeon2249291）
import { chromium } from 'playwright-core'

// SAKU_TEST_BASE 可覆盖为本地验证构建（如 http://localhost:5199）
const BASE = process.env.SAKU_TEST_BASE || 'https://manga.yeon.top:8443'
const USER = process.env.SAKU_TEST_USER || 'Yeon'
const PASS = process.env.SAKU_TEST_PASS || 'Yeon2249291'

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const fmt = (n) =>
  n >= 1024 * 1024 ? (n / 1024 / 1024).toFixed(2) + ' MB' : (n / 1024).toFixed(1) + ' KB'

function log(mark, msg) {
  console.log(`[${mark}] ${msg}`)
}

async function dumpLS(page, tag) {
  const info = await page.evaluate(() => {
    const real = []
    let realTotal = 0
    let junkCount = 0
    let junkTotal = 0
    let progCount = 0
    let progTotal = 0
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i)
      const v = localStorage.getItem(k)
      const n = (k?.length || 0) + (v?.length || 0)
      if (k && k.startsWith('saku_comic_progress:')) {
        progCount++
        progTotal += n
      } else if (k && k.startsWith('__')) {
        junkCount++
        junkTotal += n
      } else {
        real.push({ k, n, preview: v ? v.slice(0, 80) : '' })
        realTotal += n
      }
    }
    return { real, realTotal, junkCount, junkTotal, progCount, progTotal }
  })
  console.log(
    `--- localStorage @ ${tag} | 业务键 ${info.real.length} 个 ${fmt(info.realTotal)} | 进度map ${info.progCount} 个 ${fmt(info.progTotal)} | 测试键 ${info.junkCount} 个 ${fmt(info.junkTotal)} ---`,
  )
  for (const { k, n, preview } of info.real) {
    console.log(`    ${k}  ${fmt(n)}  ${JSON.stringify(preview)}`)
  }
  return info
}

const getPref = (page) =>
  page.evaluate(() => {
    try {
      return JSON.parse(localStorage.getItem('saku_preference_settings') || 'null')
    } catch {
      return 'PARSE_ERR'
    }
  })

const favSelect = (page) => page.locator('select.setting-select').filter({ hasText: 'Fav' })
async function setFavFolder(page, label) {
  await favSelect(page).selectOption({ label })
}
async function readFavUi(page) {
  return favSelect(page).inputValue()
}

// 通过 ?tab=preference 直达偏好设置面板（设置页支持 query.tab 初始化 activeTab）
async function openPreferenceTab(page) {
  await page.goto(`${BASE}/settings?tab=preference`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector('select.setting-select', { timeout: 30000 })
  await sleep(600)
}

const browser = await chromium.launch({ channel: 'msedge', headless: true })
const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } })
const page = await context.newPage()
page.setDefaultTimeout(30000)

try {
  // ---------- 登录 ----------
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector('#login-username')
  await page.fill('#login-username', USER)
  await page.fill('#login-password', PASS)
  await page.click('.login-btn')
  await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 60000 })
  await sleep(1500)
  log('INFO', `登录成功，当前 URL: ${page.url()}`)

  // ===== 测试 A：干净状态持久化 =====
  await openPreferenceTab(page)
  log('TEST', 'A1: 干净状态设置 默认收藏夹 = Fav 3')
  await setFavFolder(page, 'Fav 3')
  await sleep(800)
  let pref = await getPref(page)
  log(pref && pref.defaultFavFolder === 3 ? 'PASS' : 'FAIL',
    `A1: 写入后 localStorage defaultFavFolder=${pref?.defaultFavFolder}（期望 3），UI=${await readFavUi(page)}`)
  await page.reload({ waitUntil: 'domcontentloaded' })
  await openPreferenceTab(page)
  pref = await getPref(page)
  log(pref && pref.defaultFavFolder === 3 ? 'PASS' : 'FAIL',
    `A2: 刷新后 localStorage defaultFavFolder=${pref?.defaultFavFolder}（期望 3），UI=${await readFavUi(page)}`)

  // ===== 测试 C：配额写满 + 新键写入 =====
  log('TEST', 'C1: 用「恰好 300 条」的进度 map 填满配额（trimProgressMaps 不会裁剪它们）')
  const fillInfo = await page.evaluate(() => {
    // 300 条/个的进度 map（>300 才会被 trim，恰好 300 不会被裁剪）
    const big = {}
    for (let j = 0; j < 300; j++) big['online:' + (1000000000000 + j)] = j * 7 + 1
    const bigRaw = JSON.stringify(big)
    let bi = 0
    for (;;) {
      try {
        localStorage.setItem('saku_comic_progress:diagbig' + bi, bigRaw)
        bi++
      } catch {
        break
      }
    }
    // 小块进度 map 顶满剩余空间（free < ~30B）
    let si = 0
    for (;;) {
      try {
        localStorage.setItem('saku_comic_progress:diagsmall' + si, '{"online:1":1}')
        si++
      } catch {
        break
      }
    }
    return { bi, si }
  })
  await dumpLS(page, '配额写满后')
  const probeOk = await page.evaluate(() => {
    try {
      localStorage.setItem('__probe', 'x'.repeat(50))
      localStorage.removeItem('__probe')
      return true
    } catch {
      return false
    }
  })
  log(probeOk ? 'FAIL' : 'PASS',
    `C1: 配额探测(50B 应写不进) → ${probeOk ? '仍可写入，配额未满' : '已满'}（大map ${fillInfo.bi} 个小map ${fillInfo.si} 个）`)

  log('TEST', 'C2: 删除设置键（模拟从未成功落盘）→ 用小块进度 map 重新顶满 → 配额满时改 默认收藏夹 = Fav 7（= 新增键写入）')
  await page.evaluate(() => {
    localStorage.removeItem('saku_preference_settings')
    // 重新顶满刚释放的空间（小块进度 map，trim 不会裁剪）
    let i = 0
    for (;;) {
      try {
        localStorage.setItem('saku_comic_progress:diagrefill' + i, '{"online:2":2}')
        i++
      } catch {
        break
      }
    }
    return i
  })
  const probe2 = await page.evaluate(() => {
    try {
      localStorage.setItem('__probe2', 'x'.repeat(50))
      localStorage.removeItem('__probe2')
      return true
    } catch {
      return false
    }
  })
  log(probe2 ? 'FAIL' : 'PASS', `C2: 重填后配额探测(50B 应写不进) → ${probe2 ? '仍可写入' : '已满'}`)
  await setFavFolder(page, 'Fav 7')
  await sleep(800)
  const uiNow = await readFavUi(page)
  pref = await getPref(page)
  log('INFO', `C2: 刷新前 UI=${uiNow}，localStorage 设置键=${pref === null ? '(不存在)' : `defaultFavFolder=${pref.defaultFavFolder}`}`)
  if (uiNow === '7' && pref === null) {
    log('REPRO', '√ 复现成功：UI 内存值=7，localStorage 无设置键 → 新增键写入被静默丢弃（正是用户症状）')
  } else if (uiNow === '7' && pref?.defaultFavFolder === 7) {
    log('INFO', '配额满时新增键写入居然成功（未复现）')
  } else {
    log('INFO', `其他情况：UI=${uiNow} pref=${JSON.stringify(pref)}`)
  }

  await page.reload({ waitUntil: 'domcontentloaded' })
  await openPreferenceTab(page)
  pref = await getPref(page)
  const uiB = await readFavUi(page)
  log(pref?.defaultFavFolder === 7 ? 'NOTE' : 'REPRO',
    `C3: 刷新后 localStorage 设置键=${pref === null ? '(不存在)' : `defaultFavFolder=${pref.defaultFavFolder}`}，UI=${uiB}（若为 未配置 即复现“刷新丢失”）`)

  // ---------- 清理 ----------
  await page.evaluate(() => {
    for (let i = localStorage.length - 1; i >= 0; i--) {
      const k = localStorage.key(i)
      if (k && (k.startsWith('saku_comic_progress:') || k.startsWith('__'))) localStorage.removeItem(k)
    }
  })
  await dumpLS(page, '清理后')
  log('DONE', '诊断脚本结束')
} catch (e) {
  log('ERROR', '脚本异常: ' + (e && e.stack ? e.stack : e))
  try {
    await dumpLS(page, '异常时')
  } catch {}
} finally {
  await browser.close()
}
