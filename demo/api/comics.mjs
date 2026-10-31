// demo/api/comics.mjs
// 漫画相关接口：离线列表/详情/封面/页面，在线列表/热门/详情/页面，随机抽卡。
// 数据来源：demo/data/offline.json、demo/data/online.json（每次请求实时读取，改完刷新即生效）。
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { json, file, notFound } from './util.mjs'

/** 内页素材文件（demo/assets/pages/p01.svg … p12.svg），按页码循环复用 */
const PAGE_FILES = Array.from({ length: 12 }, (_, i) => `p${String(i + 1).padStart(2, '0')}.svg`)

export function register(add, { dataDir, assetsDir }) {
  const readJson = async (name) => JSON.parse(await readFile(join(dataDir, name), 'utf8'))

  const offlineAll = async () => readJson('offline.json')
  const onlineAll = async () => readJson('online.json')

  const findOffline = async (id) => (await offlineAll()).find((c) => c.id === id)
  const findOnline = async (id) => (await onlineAll()).find((c) => String(c.id) === String(id))

  /** 离线封面：读 demo/assets/covers/<coverFile>；缺失时回退第一张，保证界面不裂图 */
  const coverResponse = async (comic) => {
    const name = comic.coverFile || 'c01.svg'
    return file(join(assetsDir, 'covers', name), 'image/svg+xml')
  }

  /** 离线内页：按物理索引循环取素材 */
  const pageResponse = (index) => {
    const i = Number(index)
    const safe = Number.isFinite(i) && i >= 0 ? i : 0
    const name = PAGE_FILES[safe % PAGE_FILES.length]
    return file(join(assetsDir, 'pages', name), 'image/svg+xml')
  }

  /** 离线详情兜底字段：列表接口的精简结构 + 详情页需要的完整字段 */
  const offlineDetail = (comic) => ({
    ...comic,
    hiddenPagesList: [],
    originalPageCount: comic.pageCount,
    tagSources: (comic.tagRaws || []).map(() => 'online'),
    onlineTagsList: comic.tagRaws || [],
    offlineAddTagsList: [],
    offlineRemoveTagsList: [],
  })

  // ── 在线（必须在 /comics/:id 之前注册，避免 "online" 被当成漫画 id）──
  add('GET', '/api/v1/comics/online', async ({ query }) => {
    const all = await onlineAll()
    // 搜索词/分类过滤：让搜索框在演示环境里也有可见反馈
    const kw = (query.get('keyword') || '').trim().toLowerCase()
    const cats = query.getAll('categories')
    let list = all
    if (cats.length > 0) list = list.filter((c) => cats.includes(c.category))
    if (kw) list = list.filter((c) => c.title.toLowerCase().includes(kw))
    return json({ comics: list, hasMore: false, totalPages: 1, currentPage: 1 })
  })

  add('GET', '/api/v1/comics/online/popular', async () => {
    const all = await onlineAll()
    const comics = [...all].sort((a, b) => (b.rating || 0) - (a.rating || 0)).slice(0, 8)
    return json({ comics })
  })

  add('GET', '/api/v1/comics/online/favorites', async () => {
    const all = await onlineAll()
    return json({ comics: all.filter((c) => c.isFavorite) })
  })

  add('GET', '/api/v1/comics/online/toplist', async () => {
    const all = await onlineAll()
    return json({ comics: [...all].sort((a, b) => (b.rating || 0) - (a.rating || 0)) })
  })

  add('GET', '/api/v1/online/watched', async () => {
    const all = await onlineAll()
    return json({ comics: all.slice(0, 6), hasMore: false })
  })

  add('GET', '/api/v1/comics/online/detail', async ({ query }) => {
    const id = query.get('id')
    const c = (await findOnline(id)) || (await onlineAll())[0]
    if (!c) return notFound('没有可用的演示画廊')
    return json({
      id: c.id,
      token: c.token,
      title: c.title,
      subTitle: '',
      coverUrl: c.coverUrl,
      source: 'online',
      category: c.category,
      uploader: c.uploader,
      rating: c.rating,
      pageCount: c.pageCount,
      updatedAt: c.updatedAt,
      tags: c.tags || [],
      previewPages: [],
      comments: [],
      isFavorite: !!c.isFavorite,
      favIndex: typeof c.favIndex === 'number' ? c.favIndex : null,
      isDownloaded: !!c.isDownloaded,
      localId: c.localId || '',
      maxPreviewPage: 0,
    })
  })

  add('GET', '/api/v1/comics/online/pages', async ({ query }) => {
    const id = query.get('id')
    const c = (await findOnline(id)) || (await onlineAll())[0]
    const total = c?.pageCount || PAGE_FILES.length
    const urls = Array.from({ length: total }, (_, i) => `/demo-assets/pages/${PAGE_FILES[i % PAGE_FILES.length]}`)
    return json({ total, urls })
  })

  add('GET', '/api/v1/comics/online/page', async ({ query }) => {
    const i = Number(query.get('index') || 0)
    const name = PAGE_FILES[(Number.isFinite(i) && i >= 0 ? i : 0) % PAGE_FILES.length]
    return json({ url: `/demo-assets/pages/${name}` })
  })

  // ── 随机抽卡 ──
  add('GET', '/api/v1/comics/random', async ({ query }) => {
    const count = Math.max(1, Math.min(60, Number(query.get('count') || 6)))
    const source = query.get('source') || 'all'
    const pool = []
    if (source !== 'online') pool.push(...(await offlineAll()))
    if (source !== 'offline') pool.push(...(await onlineAll()))
    // 稳定「随机」：按 id 哈希排序后取前 N，重复抽卡会换序但不依赖运行时随机
    const seed = Date.now() % 100000
    const shuffled = [...pool].sort(
      (a, b) => ((hash(a.id) + seed) % 997) - ((hash(b.id) + seed) % 997),
    )
    const comics = shuffled.slice(0, count).map((c) => ({
      ...c,
      source: c.source || 'offline',
      // 抽卡结果由 ItemCard 直接渲染，tags 必须是「汉字名数组」；
      // 离线库里的 tags 是 TagItem 对象数组（供列表页解析），这里做一次归一。
      tags: (c.tags || []).map((t) => (typeof t === 'string' ? t : t?.name || t?.key || '')).filter(Boolean),
    }))
    return json({
      comics,
      count: comics.length,
      mode: query.get('mode') === 'recommend' ? 'recommend' : 'random',
      onlineRandom: true,
    })
  })

  // ── 在线封面代理：演示数据本身就是本地相对路径，直接 302 到目标地址 ──
  // 注意：必须注册在 /comics/:id 之前，否则 "cover-proxy" 会被当成漫画 id
  add('GET', '/api/v1/comics/cover-proxy', async ({ query }) => {
    const target = query.get('url') || ''
    return { status: 302, headers: { Location: target }, body: '' }
  })

  // ── 离线 ──
  add('GET', '/api/v1/comics/offline', async () => json(await offlineAll()))

  add('GET', '/api/v1/comics/:id', async ({ params }) => {
    const c = await findOffline(params.id)
    return c ? json(offlineDetail(c)) : notFound('找不到该漫画')
  })

  add('GET', '/api/v1/comics/:id/cover', async ({ params }) => {
    const c = await findOffline(params.id)
    return c ? coverResponse(c) : notFound('找不到该漫画')
  })

  add('GET', '/api/v1/comics/:id/pages', async ({ params }) => {
    const c = await findOffline(params.id)
    if (!c) return notFound('找不到该漫画')
    const total = c.pageCount || PAGE_FILES.length
    return json({
      total,
      pages: Array.from({ length: total }, (_, i) => `${String(i + 1).padStart(3, '0')}.webp`),
      originalTotal: total,
      hiddenPages: [],
      originalCount: total,
    })
  })

  add('GET', '/api/v1/comics/:id/raw-page/:index', async ({ params }) =>
    pageResponse(params.index),
  )
  add('GET', '/api/v1/comics/:id/page/:index', async ({ params }) => pageResponse(params.index))

  add('GET', '/api/v1/comics/:id/chapters', async () => json([]))
  add('GET', '/api/v1/comics/:id/bookmarks', async () => json([]))
}

/** 稳定哈希：把字符串映射为整数，用于可复现的「随机」排序 */
function hash(s) {
  let h = 0
  for (let i = 0; i < String(s).length; i++) h = (h * 31 + String(s).charCodeAt(i)) | 0
  return Math.abs(h)
}
