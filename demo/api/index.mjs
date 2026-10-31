// demo/api/index.mjs
// mock 路由表与分发器。
//
// 匹配规则：
//   1. 按注册顺序匹配（先注册者优先），所以 /comics/online 必须早于 /comics/:id；
//   2. 路径段数必须一致，:name 为占位参数；
//   3. 全部未命中时走 fallback，保证前端任何请求都能拿到「形状接近真实」的响应，
//      不会因为 404 触发 request.ts 的 401 处理而把演示环境踢回登录页。
import { register as registerAuth } from './auth.mjs'
import { register as registerComics } from './comics.mjs'
import { register as registerTags } from './tags.mjs'
import { register as registerLibrary } from './library.mjs'
import { register as registerBookmarks } from './bookmarks.mjs'
import { register as registerOffline } from './offline.mjs'

/**
 * GET 兜底表：路径 → 响应体。
 * 优先按数组/对象形状给值，因为前端各 store 的解析方式不同（有的期望裸数组，有的期望包一层）。
 */
const GET_FALLBACK = [
  [/^\/api\/v1\/bookshelves$/, () => ({ bookshelves: [] })],
  [/^\/api\/v1\/history$/, () => ({ items: [], total: 0 })],
  [/^\/api\/v1\/reading-list$/, (query) => ({ source: query.get('source') || 'offline', items: [] })],
  [/^\/api\/v1\/ratings$/, () => ({ ratings: {} })],
  [/^\/api\/v1\/scrape-bookmarks$/, () => ({ items: [] })],
  [/^\/api\/v1\/downloads$/, () => ({ tasks: [], total: 0 })],
  [
    /^\/api\/v1\/downloads\/stats$/,
    () => ({
      counts: {
        queued: 0,
        downloading: 0,
        paused: 0,
        completed: 0,
        error: 0,
        error_lock: 0,
        cancelled: 0,
      },
      total: 0,
      active: 0,
    }),
  ],
  [/^\/api\/v1\/tags\/dictionary$/, () => []],
  [/^\/api\/v1\/tags\/suggest$/, () => []],
  [/^\/api\/v1\/scan-paths$/, () => []],
  [/^\/api\/v1\/offline\/ignore\/list$/, () => ({ items: [] })],
  [/^\/api\/v1\/offline\/maintain\/result$/, () => ({ items: [] })],
  [/^\/api\/v1\/system\/version$/, () => ({
    version: '2.3.0',
    name: 'SakuManga',
    demo: true,
  })],
]

function fallback(method, pathname, query) {
  if (method === 'GET') {
    for (const [re, fn] of GET_FALLBACK) {
      if (re.test(pathname)) return { status: 200, body: fn(query) }
    }
    // 未知 GET：给空数组（列表类接口的通用安全形状）
    return { status: 200, body: [] }
  }
  // 写操作：演示环境一律「成功但无副作用」，避免前端弹错误提示影响截图
  return { status: 200, body: { ok: true, message: '演示环境：该操作已忽略' } }
}

export function createApi({ dataDir, assetsDir }) {
  const routes = []

  /** 注册路由；path 形如 /api/v1/comics/:id/cover */
  const add = (method, path, handler) => {
    routes.push({ method, parts: path.split('/').filter(Boolean), handler })
  }

  registerAuth(add)
  registerComics(add, { dataDir, assetsDir })
  registerTags(add, { dataDir })
  registerLibrary(add, { dataDir })
  registerBookmarks(add, { dataDir })
  registerOffline(add, { dataDir })

  /**
   * @param {{method:string, pathname:string, query:URLSearchParams}} ctx
   * @returns {Promise<{status?:number, headers?:Record<string,string>, body?:unknown, file?:string, type?:string}>}
   */
  const dispatch = async ({ method, pathname, query }) => {
    const segs = pathname.split('/').filter(Boolean)

    for (const route of routes) {
      if (route.method !== method) continue
      if (route.parts.length !== segs.length) continue

      const params = {}
      let matched = true
      for (let i = 0; i < route.parts.length; i++) {
        const part = route.parts[i]
        if (part.startsWith(':')) {
          params[part.slice(1)] = decodeURIComponent(segs[i])
        } else if (part !== segs[i]) {
          matched = false
          break
        }
      }
      if (!matched) continue

      return route.handler({ params, query, method })
    }

    return fallback(method, pathname, query)
  }

  return { dispatch, routeCount: routes.length }
}
