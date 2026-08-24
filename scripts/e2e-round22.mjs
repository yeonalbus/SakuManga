// scripts/e2e-round22.mjs
// Round22 浏览器端到端回归：
//   A. 书架内本子排序视图：把手拖拽 + 操作菜单（移动到第 X 位 / 置顶）+ 完成后刷新持久化
//   B. 书架列表排序：侧栏置顶组把手拖拽 + 全部书架浮层拖拽/移到顶部
//   C. 快捷移除：书架内多选 → 移出书架（确认框）
//   D. 快捷加入扩大化：离线历史页多选加入 + 手气不错（离线结果）多选加入
// 用法：node scripts/e2e-round22.mjs
// 前置：后端(8081) + vite dev(5175) 已启动，测试库已重置 Yeon/admin123 密码。
import { chromium } from 'playwright-core'

const BASE = process.env.SAKU_TEST_BASE || 'http://localhost:5175'
const USER = 'Yeon'
const PASS = 'admin123'

// 测试库中的 4 本离线漫画（backend/manga.db 副本）
const C1 = 'cc7743cb4444e472d0b8e3b0932f96f9'
const C2 = '8e45208111e0edf850a7da09de317483'
const C3 = 'fd606e32746aa94b281fdf063d07f495'
const C4 = 'ffc4171fd479c4c8a40e4245858918df'
const ALL = [C1, C2, C3, C4]

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
let pass = true
const fail = (msg) => { pass = false; console.log(`[FAIL] ${msg}`) }
const ok = (msg) => console.log(`[OK] ${msg}`)

const browser = await chromium.launch({ channel: 'msedge', headless: true })
const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
const page = await context.newPage()
page.setDefaultTimeout(30000)

// ── 登录 ──
await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' })
await page.waitForSelector('#login-username')
await page.fill('#login-username', USER)
await page.fill('#login-password', PASS)
await page.click('.login-btn')
await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 60000 })
ok(`登录成功 (${USER})`)

// ── API 助手（页内 fetch，携带 saku_token）──
const api = async (method, path, body) =>
  page.evaluate(
    async ({ method, path, body }) => {
      const token = localStorage.getItem('saku_token')
      const res = await fetch('/api/v1' + path, {
        method,
        headers: {
          'Content-Type': 'application/json',
          Authorization: 'Bearer ' + (token || ''),
        },
        body: body ? JSON.stringify(body) : undefined,
      })
      return { status: res.status, data: await res.json().catch(() => null) }
    },
    { method, path, body },
  )

const getShelves = async () => (await api('GET', '/bookshelves')).data.bookshelves || []
const shelfById = async (id) => (await getShelves()).find((s) => s.id === id)

// ── 种子：清空书架 → 建 4 个书架 → A 放 4 本 → 置顶 A/B/C ──
{
  for (const s of await getShelves()) {
    await api('DELETE', `/bookshelves/${s.id}`)
  }
  const mk = async (name) => (await api('POST', '/bookshelves', { name })).data.data
  const A = await mk('测试书架A')
  const B = await mk('测试书架B')
  const C = await mk('测试书架C')
  const X = await mk('目标书架X')
  for (const cid of ALL) {
    await api('POST', `/bookshelves/${A.id}/comics`, { comicId: cid })
  }
  for (const s of [A, B, C]) {
    await api('PUT', `/bookshelves/${s.id}/pin`, { pinned: true })
  }
  globalThis.__SHELVES = { A: A.id, B: B.id, C: C.id, X: X.id }
  ok('种子数据就绪：书架 A/B/C/X，A 含 4 本，A/B/C 已置顶')
}

const S = globalThis.__SHELVES

// ── 拖拽助手 ──
async function dragHandle(handleLoc, targetLoc) {
  const hb = await handleLoc.boundingBox()
  if (!hb) throw new Error('把手不可见')
  await page.mouse.move(hb.x + hb.width / 2, hb.y + hb.height / 2)
  await page.mouse.down()
  const tb = await targetLoc.boundingBox()
  if (!tb) throw new Error('目标不可见')
  // 目标行上 1/4 处（行中线判定「插其前」需要指针位于目标行上半部）
  await page.mouse.move(tb.x + tb.width / 2, tb.y + tb.height * 0.25, { steps: 10 })
  await sleep(120)
  await page.mouse.up()
}

/** 断言书架 A 的展示顺序等于期望数组 */
async function assertShelfOrder(shelfId, expected, label) {
  const s = await shelfById(shelfId)
  const same =
    expected.every((id, i) => s.comicIds[i] === id) && s.comicIds.length === expected.length
  if (!same) fail(`${label}：期望 ${expected.join(',')}，实际 ${s.comicIds.join(',')}`)
  else ok(`${label} 顺序正确`)
}

/** 按「把 ids[from] 移动到 1-based 位置 position」计算期望顺序 */
function movedOrder(ids, from, position) {
  const arr = [...ids]
  const target = Math.max(0, Math.min(position - 1, arr.length - 1))
  const [item] = arr.splice(from, 1)
  arr.splice(target, 0, item)
  return arr
}

// ── A. 书架内本子排序：把手拖拽 + 操作菜单 ──
{
  await page.goto(`${BASE}/offline/bookshelf?id=${S.A}`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('.shelf-op-btn')
  await page.click('.shelf-op-btn:has-text("排序")')
  await page.waitForSelector('.sort-mini-list .sort-mini-card')
  const rowCount = await page.locator('.sort-mini-card').count()
  if (rowCount !== 4) fail(`排序视图应显示 4 行，实际 ${rowCount}`)
  else ok('排序视图显示 4 行')

  // 把手拖拽：最后一行（C4）拖到第一行位置（指针落第一行上 1/4 处）
  const lastHandle = page.locator('.sort-mini-card').nth(3).locator('.drag-handle')
  const firstRow = page.locator('.sort-mini-card').nth(0)
  await dragHandle(lastHandle, firstRow)
  await sleep(900)
  await assertShelfOrder(S.A, [C4, C1, C2, C3], '把手拖拽（C4 拖到顶部）')

  // 决策 D1 补充：行内长按 300ms 触发拖拽（非把手）
  {
    const row2 = page.locator('.sort-mini-card').nth(2) // C2
    const rb = await row2.boundingBox()
    await page.mouse.move(rb.x + rb.width / 2, rb.y + rb.height / 2)
    await page.mouse.down()
    await sleep(450) // > 300ms 长按触发拖拽
    const row0 = page.locator('.sort-mini-card').nth(0)
    const r0b = await row0.boundingBox()
    await page.mouse.move(r0b.x + r0b.width / 2, r0b.y + r0b.height * 0.25, { steps: 8 })
    await sleep(100)
    await page.mouse.up()
    await sleep(900)
    const curLp = (await shelfById(S.A)).comicIds
    const expectLp = [C2, C4, C1, C3]
    const sameLp = expectLp.every((id, i) => curLp[i] === id) && curLp.length === 4
    if (!sameLp) fail(`长按拖拽后顺序应为 ${expectLp.join(',')}，实际 ${curLp.join(',')}`)
    else ok('行内长按 300ms 拖拽（C2 到顶部）生效')
  }

  // 操作菜单：把当前第 2 行「移动到第 3 位」
  let cur = (await shelfById(S.A)).comicIds
  const from1 = 1 // 第 2 行
  const moved1 = cur[from1]
  await page.locator('.sort-mini-card').nth(from1).locator('.menu-trigger').click()
  await page.locator('.menu-item:has-text("移动到")').click()
  await page.waitForSelector('.modal-input')
  await page.fill('.modal-input', '3')
  await page.click('.btn-confirm')
  await sleep(900)
  await assertShelfOrder(S.A, movedOrder(cur, from1, 3), `移动到第 3 位（${moved1.slice(0, 8)}…）`)

  // 操作菜单：置顶最后一行（决策 D6：书架内本子菜单文案为「置顶」）
  cur = (await shelfById(S.A)).comicIds
  const from2 = cur.length - 1
  const moved2 = cur[from2]
  await page.locator('.sort-mini-card').nth(from2).locator('.menu-trigger').click()
  await page.locator('.menu-item:has-text("置顶")').click()
  await sleep(900)
  await assertShelfOrder(S.A, movedOrder(cur, from2, 1), `置顶（${moved2.slice(0, 8)}…）`)

  const finalOrder = (await shelfById(S.A)).comicIds
  await page.click('.toolbar-btn.primary') // 完成排序
  await sleep(300)
  // 刷新页面后顺序仍保持（持久化验证）
  await page.goto(`${BASE}/offline/bookshelf?id=${S.A}`, { waitUntil: 'domcontentloaded' })
  await sleep(800)
  await assertShelfOrder(S.A, finalOrder, '刷新后排序持久化保持')
}

// ── B. 书架列表排序 ──
{
  // B1 侧栏把手拖拽：置顶组第 3 个（C）拖到第 1 个（A）位置
  await page.goto(`${BASE}/offline/home`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('.sub-nav-item .drag-handle')
  const handles = page.locator('.sub-nav-item .drag-handle')
  const rows = page.locator('.sub-nav-item')
  await dragHandle(handles.nth(2), rows.nth(0))
  await sleep(900)
  const list1 = await getShelves()
  const order1 = list1.map((s) => s.id)
  const expectOrder1 = [S.C, S.A, S.B, S.X]
  const same1 = expectOrder1.every((id, i) => order1[i] === id) && order1.length === 4
  if (!same1) fail(`侧栏拖拽后书架顺序应为 ${expectOrder1.join(',')}，实际 ${order1.join(',')}`)
  else ok('侧栏把手拖拽：C 拖到顶部，全局顺序正确')

  // B2 全部书架浮层：把手拖拽 X 到顶部
  await page.click('.all-shelf-btn')
  await page.waitForSelector('.picker-item .drag-handle')
  const pickerRows = page.locator('.picker-item')
  const pickerHandles = page.locator('.picker-item .drag-handle')
  // 当前顺序 [C,A,B,X] → X 是最后一行
  await dragHandle(pickerHandles.nth(3), pickerRows.nth(0))
  await sleep(900)
  const list2 = await getShelves()
  const order2 = list2.map((s) => s.id)
  const expectOrder2 = [S.X, S.C, S.A, S.B]
  const same2 = expectOrder2.every((id, i) => order2[i] === id) && order2.length === 4
  if (!same2) fail(`浮层拖拽后顺序应为 ${expectOrder2.join(',')}，实际 ${order2.join(',')}`)
  else ok('全部书架浮层把手拖拽：X 拖到顶部，全局顺序正确')

  // B3 浮层操作菜单「移到顶部」：把 B 移到顶部
  const pickerRows2 = page.locator('.picker-item')
  // 顺序 [X,C,A,B] → B 在最后
  await pickerRows2.nth(3).locator('.menu-trigger').click()
  await page.locator('.menu-item:has-text("移到顶部")').click()
  await sleep(900)
  const list3 = await getShelves()
  const order3 = list3.map((s) => s.id)
  const expectOrder3 = [S.B, S.X, S.C, S.A]
  const same3 = expectOrder3.every((id, i) => order3[i] === id) && order3.length === 4
  if (!same3) fail(`浮层菜单移到顶部后顺序应为 ${expectOrder3.join(',')}，实际 ${order3.join(',')}`)
  else ok('浮层操作菜单「移到顶部」生效（B 到第 1 位）')
  await page.keyboard.press('Escape')
  await page.locator('.picker-close').click({ force: true }).catch(() => {})
}

// ── C. 快捷移除：书架内多选 → 移出书架 ──
{
  await page.goto(`${BASE}/offline/bookshelf?id=${S.A}`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('.item-card')
  // 长按第 1 张卡进入选择模式，再点第 2 张卡
  const card1 = page.locator('.item-card').nth(0)
  const b1 = await card1.boundingBox()
  await page.mouse.move(b1.x + b1.width / 2, b1.y + b1.height / 2)
  await page.mouse.down()
  await sleep(700) // > 600ms 长按
  await page.mouse.up()
  await page.waitForSelector('.select-toolbar')
  const card2 = page.locator('.item-card').nth(1)
  await card2.click()
  await sleep(200)
  const count = await page.locator('.select-count').textContent()
  if (!count || !count.includes('2')) fail(`选择数应为 2，实际 "${count}"`)
  else ok(`多选已选 2 本（${count.trim()}）`)

  // 移出书架 → 确认框
  await page.click('.toolbar-btn:has-text("移出书架")')
  await page.waitForSelector('.modal-mask')
  await page.click('.btn-confirm')
  await sleep(900)
  const a = await shelfById(S.A)
  if (a.comicIds.length !== 2) fail(`移出后应剩 2 本，实际 ${a.comicIds.length}`)
  else ok('快捷移除：多选 2 本已从书架移出，剩余 2 本')

  // 本子应保留在离线库（不删本地文件/历史）
  const off = await api('GET', '/comics/offline')
  if (off.status === 200 && Array.isArray(off.data)) {
    if (off.data.length === 4) ok('快捷移除不删除本地库记录（仍 4 本）')
    else fail(`快捷移除误删本地记录，剩 ${off.data.length} 本`)
  } else {
    fail(`查询离线库失败 status=${off.status}`)
  }
}

// ── D. 快捷加入扩大化 ──
{
  // D1 离线历史页：先造一条历史 → 长按 → 加入书架 X
  const seedComic = C2
  await api('POST', '/history', {
    comicId: seedComic,
    source: 'offline',
    gid: seedComic,
    comicTitle: 'e2e历史测试本',
    coverUrl: '',
    lastPageIndex: 0,
    totalPageCount: 10,
  })
  await page.goto(`${BASE}/offline/history`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('.item-card')
  const hCard = page.locator('.item-card').first()
  const hb = await hCard.boundingBox()
  await page.mouse.move(hb.x + hb.width / 2, hb.y + hb.height / 2)
  await page.mouse.down()
  await sleep(700)
  await page.mouse.up()
  await page.waitForSelector('.select-toolbar')
  await page.click('.toolbar-btn:has-text("加入书架")')
  await page.waitForSelector('.picker-item')
  await page.locator('.picker-item:has-text("目标书架X") .item-main').click()
  await sleep(900)
  const x1 = await shelfById(S.X)
  if (!x1.comicIds.includes(seedComic)) fail(`离线历史快捷加入失败：X 不含 ${seedComic}`)
  else ok('离线历史页：长按多选 → 加入书架 X 成功')

  // D2 手气不错：离线范围抽 4 本 → 长按 → 全选本页 → 加入书架 X
  await page.goto(`${BASE}/random`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('.draw-btn')
  // 数量选「4 本」+ 范围仅本地
  await page.click('.pill-btn:has-text("4 本")')
  await page.selectOption('select.dark-select:has(option[value="offline"])', 'offline')
  await page.click('.draw-btn')
  await page.waitForSelector('.results-grid .item-card', { timeout: 60000 })
  const drawnCount = await page.locator('.results-grid .item-card').count()
  if (drawnCount < 1) fail('抽卡无结果')
  const rCard = page.locator('.results-grid .item-card').first()
  const rb = await rCard.boundingBox()
  await page.mouse.move(rb.x + rb.width / 2, rb.y + rb.height / 2)
  await page.mouse.down()
  await sleep(700)
  await page.mouse.up()
  await page.waitForSelector('.select-toolbar', { timeout: 10000 })
  // 全选本页（离线结果全部选中）→ 加入书架 X
  await page.click('.toolbar-btn:has-text("全选本页")')
  await page.click('.toolbar-btn:has-text("加入书架")')
  await page.waitForSelector('.picker-item')
  await page.locator('.picker-item:has-text("目标书架X") .item-main').click()
  await sleep(900)
  const x2 = await shelfById(S.X)
  const allPresent = ALL.every((id) => x2.comicIds.includes(id))
  if (!allPresent) fail(`手气不错快捷加入失败：X 应含全部 4 本，实际 ${x2.comicIds.join(',')}`)
  else ok(`手气不错（离线结果）：多选全选 → 加入书架 X 成功（X 现有 ${x2.comicIds.length} 本）`)

  // D3 取消选择模式：重新长按进入选择 → 点「取消」→ 工具条消失且选择态清空
  const rCard2 = page.locator('.results-grid .item-card').first()
  const rb2 = await rCard2.boundingBox()
  await page.mouse.move(rb2.x + rb2.width / 2, rb2.y + rb2.height / 2)
  await page.mouse.down()
  await sleep(700)
  await page.mouse.up()
  await page.waitForSelector('.select-toolbar')
  await page.click('.toolbar-btn:has-text("取消")')
  const toolbarGone = (await page.locator('.select-toolbar').count()) === 0
  if (!toolbarGone) fail('取消后选择工具条应消失')
  else ok('取消选择模式正常')
}

await browser.close()
console.log(pass ? '\n===== ROUND22 E2E 全部通过 =====' : '\n===== ROUND22 E2E 存在失败项 =====')
process.exit(pass ? 0 : 1)
