// 发布前修复验证脚本（Playwright-core + 系统 Edge，直连线上后端 CORS）
// 覆盖：
//   A 修复#4 登录失败（401）不再触发全局登出语义：显示错误、停留登录页、不写 token
//   B 修复#5 登录落地页统一遵循「启动时默认菜单」偏好（LoginView fallback + 登录守卫重定向）
//   C 修复#2 下载设置改完立即关闭/刷新仍落盘后端（pagehide keepalive flush 兜底）
//   D 修复#1 fetchMe 仅 401 才清 token；网络/服务端错误(500)保留会话
// 用法：node scripts/verify-release-fixes.mjs
//   BASE   = http://localhost:5198（dist-verify 的 vite preview 地址，可用 SAKU_VERIFY_BASE 覆盖）
//   API    = https://manga.yeon.top:8443/api/v1（已嵌入构建，此处仅供页面内 fetch 使用）
//   SAKU_VERIFY_USER / SAKU_VERIFY_PASS 可覆盖账号（默认 Yeon / Yeon2249291）
import { chromium } from 'playwright-core'

const BASE = process.env.SAKU_VERIFY_BASE || 'http://localhost:5198'
const API = 'https://manga.yeon.top:8443/api/v1'
const USER = process.env.SAKU_VERIFY_USER || 'Yeon'
const PASS = process.env.SAKU_VERIFY_PASS || 'Yeon2249291'

const results = []
const check = (name, ok, detail = '') => {
  results.push({ name, ok })
  console.log(`${ok ? 'PASS' : 'FAIL'} | ${name}${detail ? ' | ' + detail : ''}`)
}

const browser = await chromium.launch({ channel: 'msedge', headless: false, ignoreHTTPSErrors: true })
const ctx = await browser.newContext({ ignoreHTTPSErrors: true })
const page = await ctx.newPage()
page.setDefaultTimeout(20000)

try {
  // ── A: 修复#4 登录失败 ──
  await page.goto(BASE + '/login', { waitUntil: 'domcontentloaded' })
  await page.fill('#login-username', USER)
  await page.fill('#login-password', 'WRONG_PASSWORD_xyz')
  await page.click('button.login-btn')
  await page.waitForSelector('.error-msg', { timeout: 15000 })
  const errText = (await page.textContent('.error-msg').catch(() => '')) || ''
  const urlA = page.url()
  const tokenA = await page.evaluate(() => localStorage.getItem('saku_token'))
  check('A-1 密码错误显示提示', errText.length > 0, errText)
  check('A-2 仍停留在登录页', urlA.includes('/login'), urlA)
  check('A-3 未写入 token', tokenA === null || tokenA === '', String(tokenA))

  // ── B: 修复#5 登录落地页 ──
  // 用正确密码登录（无 redirect 参数 → LoginView fallback 应为默认偏好 hot → /online/hot）
  await page.fill('#login-password', PASS)
  await page.click('button.login-btn')
  await page.waitForURL((u) => !u.pathname.includes('/login'), { timeout: 30000 })
  const urlB = page.url()
  check('B-1 登录后按默认启动菜单(hot)落地', urlB.includes('/online/hot'), urlB)

  // 修改偏好为 fav，整页加载 /login 验证登录守卫按 defaultStartupMenu 重定向
  await page.evaluate(() => {
    const key = 'saku_preference_settings'
    let cur = {}
    try {
      cur = JSON.parse(localStorage.getItem(key) || '{}')
    } catch {}
    cur.defaultStartupMenu = 'fav'
    localStorage.setItem(key, JSON.stringify(cur))
  })
  await page.goto(BASE + '/login', { waitUntil: 'domcontentloaded' })
  await page.waitForURL((u) => u.pathname.includes('/online/favorites'), { timeout: 30000 })
  check('B-2 登录守卫按 defaultStartupMenu 重定向', page.url().includes('/online/favorites'), page.url())

  // ── C: 修复#2 下载设置改完立即刷新仍落盘后端 ──
  await page.goto(BASE + '/settings?tab=download', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(2000) // 等待下载面板渲染与后端拉取
  const dlSelect = page.locator('.setting-select').nth(1) // 第2个 = 同时下载图片数量
  await dlSelect.waitFor({ timeout: 15000 })
  const curVal = await dlSelect.inputValue()
  const options = await dlSelect.locator('option').evaluateAll((els) => els.map((o) => o.value))
  const newVal = options.find((v) => v !== curVal) || options[0]
  await dlSelect.selectOption(newVal)
  // 立即整页刷新：300ms 防抖窗口内触发 pagehide keepalive flush，后端应已落盘新值
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(2500) // 等待 keepalive flush 与后端写入完成
  const backendVal = await page.evaluate(async (api) => {
    const token = localStorage.getItem('saku_token')
    const res = await fetch(api + '/downloads/settings', {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (!res.ok) return null
    const data = await res.json()
    return data.concurrentImageDownloads
  }, API)
  check('C-1 改后立即刷新后端落盘', String(backendVal) === String(newVal), `期望=${newVal} 实际=${backendVal}`)

  // ── D: 修复#1 fetchMe 非401错误不清 token，401 才清 ──
  await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1500)
  const tokBefore = await page.evaluate(() => localStorage.getItem('saku_token'))
  check('D-1 已登录（token 存在）', !!tokBefore, String(!!tokBefore))

  // /auth/me 返回 500（服务端/网络错误）→ 刷新后 token 应保留
  await page.route('**/auth/me', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"boom"}' }),
  )
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1500)
  const tokAfter500 = await page.evaluate(() => localStorage.getItem('saku_token'))
  check('D-2 非401错误刷新后 token 保留', !!tokAfter500, String(!!tokAfter500))
  await page.unroute('**/auth/me')

  // /auth/me 返回 401（会话真实失效）→ 刷新后 token 应被清除
  await page.route('**/auth/me', (route) =>
    route.fulfill({ status: 401, contentType: 'application/json', body: '{"error":"unauth"}' }),
  )
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1500)
  const tokAfter401 = await page.evaluate(() => localStorage.getItem('saku_token'))
  check('D-3 401刷新后 token 被清除', !tokAfter401, String(!!tokAfter401))
  await page.unroute('**/auth/me')
} catch (e) {
  console.error('SCRIPT_ERROR:', e)
  check('脚本执行', false, String(e))
} finally {
  await browser.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n===== ${failed.length === 0 ? '全部通过' : `失败 ${failed.length} 项`} =====`)
process.exit(failed.length === 0 ? 0 : 1)
