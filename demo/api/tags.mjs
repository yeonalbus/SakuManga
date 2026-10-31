// demo/api/tags.mjs
// 标签词典与搜索联想。
//
// 词典不能留空：前端 tagStore 把「空数组」判定为加载失败并弹提示，
// 同时所有标签会退化成英文原文（截图里会看到 language:chinese 这种裸 key）。
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { json } from './util.mjs'

export function register(add, { dataDir }) {
  const tags = async () => JSON.parse(await readFile(join(dataDir, 'tags.json'), 'utf8'))

  add('GET', '/api/v1/tags/dictionary', async () => json(await tags()))

  add('GET', '/api/v1/tags/suggest', async ({ query }) => {
    const q = (query.get('q') || '').trim().toLowerCase()
    const limit = Math.min(50, Number(query.get('limit') || 20))
    const all = await tags()
    const list = q
      ? all.filter((t) => `${t.namespace}:${t.key} ${t.name}`.toLowerCase().includes(q))
      : all
    return json(
      list.slice(0, limit).map((t) => ({
        namespace: t.namespace,
        key: t.key,
        name: t.name,
        count: 0,
      })),
    )
  })

  add('GET', '/api/v1/tags/status', async () => json({ enabled: true, count: (await tags()).length, demo: true }))
}
