// demo/api/library.mjs
// 书架 / 历史 / 阅读清单 / 评分（用户库数据）。
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { json } from './util.mjs'

export function register(add, { dataDir }) {
  const read = async (name) => JSON.parse(await readFile(join(dataDir, name), 'utf8'))

  add('GET', '/api/v1/bookshelves', async () => json(await read('bookshelves.json')))

  add('GET', '/api/v1/history', async ({ query }) => {
    const source = query.get('source')
    const all = await read('history.json')
    const items = source ? all.items.filter((h) => h.source === source) : all.items
    return json({ items, total: items.length })
  })

  add('GET', '/api/v1/reading-list', async ({ query }) => {
    const source = query.get('source') === 'online' ? 'online' : 'offline'
    const all = await read('reading-list.json')
    return json({ source, items: all[source] || [] })
  })

  add('GET', '/api/v1/ratings', async () => json({ ratings: {} }))
}
