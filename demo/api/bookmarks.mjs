// demo/api/bookmarks.mjs
// 搜刮书签（Round27/Round49）。结构见 src/types/comic.ts 的 ScrapeBookmark。
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { json } from './util.mjs'

export function register(add, { dataDir }) {
  const read = async () => JSON.parse(await readFile(join(dataDir, 'bookmarks.json'), 'utf8'))

  add('GET', '/api/v1/scrape-bookmarks', async () => json(await read()))

  // 更新探测：演示环境固定返回「全部已完成，无新增」（避免界面出现红点闪烁）
  add('POST', '/api/v1/scrape-bookmarks/probe', async () => {
    const data = await read()
    const results = data.items.map((b) => ({
      id: Number(String(b.id).replace(/\D/g, '')) || 0,
      status: 'ok',
      hasUpdate: false,
      checkedAt: 1761900000000,
    }))
    return json({ results })
  })
}
