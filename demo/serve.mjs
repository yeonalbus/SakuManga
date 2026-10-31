#!/usr/bin/env node
// demo/serve.mjs
// 演示环境服务：静态托管 dist/（发布版外观）+ 拦截 /api/v1/** 返回 demo/data 里的假数据。
//
// 为什么不用 vite dev：dev server 会把请求转发到真实后端（127.0.0.1:8081），
// 演示环境需要的是「有数据的完整界面」。这里用一个进程同时扮演静态服务器和 mock 后端。
//
// 用法：
//   node demo/serve.mjs             # 默认 5199 端口，自动打开浏览器
//   node demo/serve.mjs --port 5200
//   node demo/serve.mjs --no-open   # 不自动开浏览器（自动化/CI 用）
import http from 'node:http'
import { readFile } from 'node:fs/promises'
import { exec } from 'node:child_process'
import { dirname, extname, join, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createApi } from './api/index.mjs'

const HERE = dirname(fileURLToPath(import.meta.url))
const ROOT = resolve(HERE, '..')
const DIST = join(ROOT, 'dist')
const ASSETS = join(HERE, 'assets')
const DATA = join(HERE, 'data')

// ── 参数 ──
const argv = process.argv.slice(2)
const flag = (name) => argv.includes(`--${name}`)
const option = (name, def) => {
  const i = argv.indexOf(`--${name}`)
  if (i >= 0 && argv[i + 1]) return argv[i + 1]
  const inline = argv.find((a) => a.startsWith(`--${name}=`))
  return inline ? inline.split('=')[1] : def
}

const PORT = Number(option('port', 5200))
const HOST = option('host', '127.0.0.1')
const AUTO_OPEN = !flag('no-open')

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.webp': 'image/webp',
  '.gif': 'image/gif',
  '.ico': 'image/x-icon',
  '.webmanifest': 'application/manifest+json',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
  '.txt': 'text/plain; charset=utf-8',
  '.map': 'application/json',
}

// ── 工具 ──
const mimeOf = (p) => MIME[extname(p).toLowerCase()] || 'application/octet-stream'

/** 拼接静态路径并阻止目录穿越；越界返回 null */
function safeJoin(base, urlPath) {
  const rel = urlPath.replace(/^\/+/, '')
  const abs = resolve(base, rel)
  if (abs !== base && !abs.startsWith(base + sep)) return null
  return abs
}

async function readIfExists(abs) {
  try {
    return await readFile(abs)
  } catch {
    return null
  }
}

// ── 启动前检查 dist ──
const rawIndex = await readIfExists(join(DIST, 'index.html'))
if (!rawIndex) {
  console.error('')
  console.error('  ✗ 未找到 dist/index.html —— 演示环境托管的是构建产物。')
  console.error('    请先执行：npm run build-only')
  console.error('')
  process.exit(1)
}

// 注入登录态：任何浏览器打开演示地址都自动带上 demo token，
// 否则前端路由守卫会把访客踢到登录页（这是「自己动手截图」能跑起来的前提）。
const TOKEN_INJECT =
  '<script>try{localStorage.setItem("saku_token","demo-token")}catch(e){}</script>'
const indexHtml = Buffer.from(
  rawIndex.toString('utf8').replace('</head>', `${TOKEN_INJECT}</head>`),
  'utf8',
)

const api = createApi({ dataDir: DATA, assetsDir: ASSETS })

// ── 请求处理 ──
async function handleApi(res, req, url, pathname) {
  const result = await api.dispatch({
    method: req.method || 'GET',
    pathname,
    query: url.searchParams,
  })

  if (result && 'file' in result) {
    const data = await readIfExists(result.file)
    if (!data) {
      res.writeHead(404, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ error: `演示素材缺失：${result.file}` }))
      return
    }
    res.writeHead(200, {
      'Content-Type': result.type || mimeOf(result.file),
      'Cache-Control': 'no-cache',
    })
    res.end(data)
    return
  }

  if (result && 'raw' in result) {
    res.writeHead(result.status || 200, {
      'Content-Type': result.type || 'application/octet-stream',
      'Cache-Control': 'no-cache',
    })
    res.end(result.raw)
    return
  }

  const body = result && 'body' in result ? result.body : (result ?? {})
  const isText = typeof body === 'string'
  const payload = isText ? body : JSON.stringify(body)

  res.writeHead(result?.status || 200, {
    'Content-Type': isText ? 'text/plain; charset=utf-8' : 'application/json; charset=utf-8',
    'Cache-Control': 'no-store',
    ...result?.headers,
  })
  res.end(payload)
}

async function handleStatic(res, pathname) {
  // 静态资源（带扩展名）：命中就返回，未命中给 404（不回退 index.html，避免把 JS 请求当页面）
  const ext = extname(pathname)
  if (ext && ext !== '.html') {
    const abs = safeJoin(DIST, pathname)
    const data = abs ? await readIfExists(abs) : null
    if (!data) {
      res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' })
      res.end('404')
      return
    }
    res.writeHead(200, { 'Content-Type': mimeOf(abs), 'Cache-Control': 'no-cache' })
    res.end(data)
    return
  }

  // 页面请求：dist 内的真实 html 优先，其余一律回退注入过登录态的 index.html（SPA history 路由）
  const abs = safeJoin(DIST, pathname)
  const real = abs && abs.endsWith('.html') ? await readIfExists(abs) : null
  res.writeHead(200, {
    'Content-Type': 'text/html; charset=utf-8',
    'Cache-Control': 'no-store',
  })
  res.end(real || indexHtml)
}

async function handleDemoAsset(res, pathname) {
  const rel = pathname.slice('/demo-assets/'.length)
  const abs = safeJoin(ASSETS, rel)
  const data = abs ? await readIfExists(abs) : null
  if (!data) {
    res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' })
    res.end('404')
    return
  }
  res.writeHead(200, { 'Content-Type': mimeOf(abs), 'Cache-Control': 'no-cache' })
  res.end(data)
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url || '/', `http://${req.headers.host || '127.0.0.1'}`)
  let pathname
  try {
    pathname = decodeURIComponent(url.pathname)
  } catch {
    pathname = url.pathname
  }

  try {
    if (pathname.startsWith('/api/v1/')) return await handleApi(res, req, url, pathname)
    if (pathname.startsWith('/demo-assets/')) return await handleDemoAsset(res, pathname)
    return await handleStatic(res, pathname)
  } catch (err) {
    console.error(`[demo] 请求失败 ${pathname}:`, err)
    if (!res.headersSent) {
      res.writeHead(500, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ error: String(err && err.message) }))
    } else {
      res.end()
    }
  }
})

server.on('error', (err) => {
  if (err.code === 'EADDRINUSE') {
    console.error('')
    console.error(`  ✗ 端口 ${PORT} 已被占用。换一个：node demo/serve.mjs --port ${PORT + 1}`)
    console.error('')
    process.exit(1)
  }
  if (err.code === 'EACCES') {
    console.error('')
    console.error(`  ✗ 端口 ${PORT} 被系统或安全软件拒绝（EACCES）。换一个：node demo/serve.mjs --port ${PORT + 1}`)
    console.error('')
    process.exit(1)
  }
  throw err
})

server.listen(PORT, HOST, () => {
  const base = `http://${HOST}:${PORT}`
  console.log('')
  console.log('  SakuManga 演示环境')
  console.log(`  地址   ${base}`)
  console.log(`  数据   demo/data/*.json（改完刷新页面即生效）`)
  console.log(`  素材   demo/assets/（封面 covers/、内页 pages/）`)
  console.log(`  路由   ${api.routeCount} 条 mock 接口 + 兜底`)
  console.log('')
  console.log('  按 Ctrl+C 停止')
  console.log('')

  if (AUTO_OPEN) {
    const cmd =
      process.platform === 'win32'
        ? `start "" "${base}"`
        : process.platform === 'darwin'
          ? `open "${base}"`
          : `xdg-open "${base}"`
    exec(cmd, () => {
      /* 打开失败不影响服务 */
    })
  }
})

// 让 npm run demo 在 Ctrl+C 时安静退出
process.on('SIGINT', () => {
  console.log('\n  已停止演示环境')
  server.close(() => process.exit(0))
  setTimeout(() => process.exit(0), 500)
})
