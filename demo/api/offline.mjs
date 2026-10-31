// demo/api/offline.mjs
// 本地库离线工具：XP 词云 / 画质升级候选。
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { json } from './util.mjs'

export function register(add, { dataDir }) {
  const read = async (name) => JSON.parse(await readFile(join(dataDir, name), 'utf8'))

  // XP 词云（Round32）：group 参数仅影响前端筛选，演示环境一次给全量
  add('GET', '/api/v1/offline/xp-cloud', async () => json(await read('xpcloud.json')))

  // 画质升级候选
  add('GET', '/api/v1/offline/upgrade/list', async () => json(await read('upgrade.json')))
}
