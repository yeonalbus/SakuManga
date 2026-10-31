#!/usr/bin/env node
// demo/shots.mjs
// 按 demo/pages.mjs 清单自动出图到 docs/screenshots/。
//
// 用法：
//   node demo/shots.mjs                     # 自动拉起演示服务 → 逐页截图 → 关服务
//   node demo/shots.mjs --base http://127.0.0.1:5200   # 复用已在跑的 npm run demo
//   node demo/shots.mjs --only 09-reader    # 只出指定页（逗号分隔，可写名字前缀）
//   node demo/shots.mjs --scale 1           # 降采样（默认 2 倍，出图更清晰但更大）
import { chromium } from 'playwright-core'
import { spawn } from 'node:child_process'
import { mkdir } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { pages } from './pages.mjs'

const HERE = dirname(fileURLToPath(import.meta.url))
const ROOT = resolve(HERE, '..')
const OUT_DIR = join(ROOT, 'docs', 'screenshots')

const argv = process.argv.slice(2)
const option = (name, def) => {
  const i = argv.indexOf(`--${name}`)
  if (i >= 0 && argv[i + 1]) return argv[i + 1]
  const inline = argv.find((a) => a.startsWith(`--${name}=`))
  return inline ? inline.split('=')[1] : def
}

const PORT = Number(option('port', 5200))
const SCALE = Number(option('scale', 2))
const VIEWPORT = { width: 1600, height: 900 }
const ONLY = (option('only', '') || '')
  .split(',')
  .map((s) => s.trim())
  .filter(Boolean)

const CHROME_CANDIDATES = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium',
].filter(Boolean)

async function probeBase(base) {
  try {
    const res = await fetch(`${base}/api/v1/auth/me`, { signal: AbortSignal.timeout(1500) })
    const data = await res.json()
    return data?.user?.username === 'demo'
  } catch {
    return false
  }
}

/** 等端口可用（最多 ~8s） */
async function waitForBase(base) {
  for (let i = 0; i < 40; i++) {
    if (await probeBase(base)) return true
    await new Promise((r) => setTimeout(r, 200))
  }
  return false
}

async function launchBrowser() {
  // 优先用系统 Chrome（channel），失败再按已知路径找
  try {
    return await chromium.launch({ channel: 'chrome', headless: true })
  } catch {
    /* 继续尝试 executablePath */
  }
  for (const path of CHROME_CANDIDATES) {
    try {
      return await chromium.launch({ executablePath: path, headless: true })
    } catch {
      /* 试下一个 */
    }
  }
  throw new Error('未找到可用的 Chrome。请安装 Chrome，或设置环境变量 CHROME_PATH 指向浏览器可执行文件。')
}

const selected = pages.filter((p) => {
  if (p.skip) return false
  if (ONLY.length === 0) return true
  return ONLY.some((k) => p.name === k || p.name.includes(k))
})

if (selected.length === 0) {
  console.error('没有匹配的页面。可用：')
  for (const p of pages) console.error('  -', p.name)
  process.exit(1)
}

// ── 准备服务 ──
let base = option('base', '')
let server = null

if (!base) {
  base = `http://127.0.0.1:${PORT}`
  const already = await probeBase(base)
  if (already) {
    console.log(`复用已在运行的演示服务：${base}`)
  } else {
    console.log(`启动演示服务：${base}`)
    server = spawn(process.execPath, [join(HERE, 'serve.mjs'), '--no-open', '--port', String(PORT)], {
      cwd: ROOT,
      stdio: 'inherit',
    })
    if (!(await waitForBase(base))) {
      server.kill()
      console.error('演示服务启动失败（端口被占用或 dist 未构建）。先跑 npm run build-only，或改用 --base 指定已运行的服务。')
      process.exit(1)
    }
  }
} else {
  base = base.replace(/\/$/, '')
  if (!(await waitForBase(base))) {
    console.error(`无法连接 ${base}（演示服务没在跑？先执行 npm run demo）`)
    process.exit(1)
  }
}

await mkdir(OUT_DIR, { recursive: true })

// ── 出图 ──
const browser = await launchBrowser()
const context = await browser.newContext({
  viewport: VIEWPORT,
  deviceScaleFactor: SCALE,
  locale: 'zh-CN',
  timezoneId: 'Asia/Shanghai',
})

let failed = 0

for (const item of selected) {
  const page = await context.newPage()
  const errors = []
  page.on('console', (m) => {
    if (m.type() === 'error') errors.push(m.text())
  })
  page.on('pageerror', (e) => errors.push('pageerror: ' + e.message))

  try {
    if (item.init) await item.init(page)
    await page.goto(base + item.path, { waitUntil: 'domcontentloaded', timeout: 30000 })
    await page.waitForTimeout(item.wait ?? 1800)

    if (item.before) await item.before(page)
    if (item.settle) await page.waitForTimeout(item.settle)

    const file = join(OUT_DIR, `${item.name}.png`)
    await page.screenshot({ path: file })
    console.log(`  ✓ ${item.name}.png`)
    if (errors.length > 0) {
      console.log(`    ⚠ 页面有 ${errors.length} 条控制台错误（图仍已生成，请留意）：`)
      for (const e of errors.slice(0, 3)) console.log('       ', e)
    }
  } catch (err) {
    failed++
    console.error(`  ✗ ${item.name}：${err.message}`)
  } finally {
    await page.close()
  }
}

await context.close()
await browser.close()

if (server) {
  server.kill()
}

console.log('')
console.log(`产物目录：docs/screenshots/（${selected.length - failed}/${selected.length} 成功）`)
if (failed > 0) process.exit(1)
