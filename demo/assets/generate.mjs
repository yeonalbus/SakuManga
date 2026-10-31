// demo/assets/generate.mjs
// 生成演示用的占位封面（covers/）与阅读器内页（pages/）。
//
// 设计约束（见 demo/README.md）：
//   1. 纯几何排版 + 文字，不含任何真实作品画面，无 R18 内容、无版权素材；
//   2. 确定性生成（固定种子），重复运行产物逐字节一致，便于 diff 审阅；
//   3. 产物入库（demo/assets/covers、demo/assets/pages），本脚本仅在需要重做素材时运行。
//
// 用法：node demo/assets/generate.mjs
import { mkdir, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const HERE = dirname(fileURLToPath(import.meta.url))

/** 确定性伪随机（mulberry32）：同一种子 → 同一套图 */
function rng(seed) {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const esc = (s) =>
  String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

// ─────────────────────────────────────────────────────────────
// 封面：600×800（3:4），暗色几何排版
// ─────────────────────────────────────────────────────────────
const COVERS = [
  { file: 'c01', title: '夜行電車', sub: 'Night Train', base: '#1e2a44', accent: '#7aa7ff', seed: 11 },
  { file: 'c02', title: '机械之心', sub: 'Mechanical Heart', base: '#2c1f3d', accent: '#b98cff', seed: 23 },
  { file: 'c03', title: '星海回响', sub: 'Echo of the Star Sea', base: '#12303a', accent: '#4fd6c0', seed: 37 },
  { file: 'c04', title: '雨后的图书馆', sub: 'Library After Rain', base: '#33261c', accent: '#e0a96d', seed: 41 },
  { file: 'c05', title: '纸鸢与风', sub: 'Paper Kite & Wind', base: '#2b2f22', accent: '#b6d96a', seed: 53 },
  { file: 'c06', title: '像素迷宫', sub: 'Pixel Maze', base: '#301b26', accent: '#ff8fa8', seed: 67 },
  { file: 'c07', title: '海边的天文台', sub: 'Observatory by the Sea', base: '#152238', accent: '#8fb8ff', seed: 71 },
  { file: 'c08', title: '午后三点的猫', sub: 'Cat at Three O\'Clock', base: '#2f2418', accent: '#f0c070', seed: 83 },
]

function coverSvg({ title, sub, base, accent, seed }) {
  const rand = rng(seed)
  const W = 600
  const H = 800

  // 装饰：右上一个半透明大圆、左下一组斜线、右上角小方块阵列
  const cx = 430 + Math.round(rand() * 60)
  const cy = 170 + Math.round(rand() * 40)
  const r = 150 + Math.round(rand() * 40)

  const stripes = Array.from({ length: 7 }, (_, i) => {
    const x = 30 + i * 34
    return `<line x1="${x}" y1="${H - 40}" x2="${x + 120}" y2="${H - 210}" stroke="${accent}" stroke-width="2" opacity="0.18"/>`
  }).join('')

  const dots = Array.from({ length: 4 }, (_, row) =>
    Array.from({ length: 4 }, (_, col) => {
      const on = rand() > 0.45
      return on
        ? `<rect x="${40 + col * 16}" y="${40 + row * 16}" width="8" height="8" fill="${accent}" opacity="${0.25 + rand() * 0.4}"/>`
        : ''
    }).join(''),
  ).join('')

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0.4" y2="1">
      <stop offset="0" stop-color="${base}"/>
      <stop offset="1" stop-color="#0b0d12"/>
    </linearGradient>
  </defs>
  <rect width="${W}" height="${H}" fill="url(#bg)"/>
  <circle cx="${cx}" cy="${cy}" r="${r}" fill="${accent}" opacity="0.10"/>
  <circle cx="${cx}" cy="${cy}" r="${r}" fill="none" stroke="${accent}" stroke-width="1.5" opacity="0.35"/>
  <circle cx="${cx}" cy="${cy}" r="${r * 0.62}" fill="none" stroke="${accent}" stroke-width="1" opacity="0.20"/>
  ${stripes}
  ${dots}
  <rect x="36" y="36" width="${W - 72}" height="${H - 72}" fill="none" stroke="${accent}" stroke-width="1" opacity="0.18" rx="6"/>
  <text x="56" y="${H - 232}" fill="${accent}" font-family="'Segoe UI', 'Microsoft YaHei', sans-serif" font-size="13" letter-spacing="4" opacity="0.75">SAMPLE ARTWORK</text>
  <text x="56" y="${H - 150}" fill="#f2f5fa" font-family="'Segoe UI', 'Microsoft YaHei', sans-serif" font-size="52" font-weight="700">${esc(title)}</text>
  <text x="58" y="${H - 108}" fill="${accent}" font-family="'Segoe UI', 'Microsoft YaHei', sans-serif" font-size="19" opacity="0.9">${esc(sub)}</text>
  <rect x="56" y="48" width="46" height="3" fill="${accent}" opacity="0.7"/>
  <text x="${W - 56}" y="${H - 56}" text-anchor="end" fill="#8b93a3" font-family="monospace" font-size="12">demo / ${esc(seed)}</text>
</svg>
`
}

// ─────────────────────────────────────────────────────────────
// 内页：800×1160（阅读器竖版页面），纸质底色 + 分镜框占位
// 只画「版面结构」，不画任何画面内容
// ─────────────────────────────────────────────────────────────
function pageSvg(index, seed) {
  const rand = rng(seed)
  const W = 800
  const H = 1160
  const M = 56 // 页边距
  const gap = 18

  // 2~3 个分镜：按画格切分，覆盖整页（不用单格，避免整页一个巨大空镜）
  const count = 2 + Math.floor(rand() * 2)
  const panels = []

  if (count === 2) {
    const h1 = Math.round((H - M * 2 - gap - 24) * (0.42 + rand() * 0.16))
    panels.push({ x: M, y: M, w: W - M * 2, h: h1 })
    panels.push({ x: M, y: M + h1 + gap, w: W - M * 2, h: H - M * 2 - 24 - h1 - gap })
  } else {
    const h1 = Math.round((H - M * 2 - gap * 2 - 24) * 0.34)
    const h2 = Math.round((H - M * 2 - gap * 2 - 24) * 0.40)
    panels.push({ x: M, y: M, w: W - M * 2, h: h1 })
    panels.push({ x: M, y: M + h1 + gap, w: W - M * 2, h: h2 })
    panels.push({ x: M, y: M + h1 + h2 + gap * 2, w: W - M * 2, h: H - M * 2 - 24 - h1 - h2 - gap * 2 })
  }

  const body = panels
    .map((p, i) => {
      // 分镜内：斜线网络（占位）+ 几何图形 + 一个对话气泡轮廓。
      // 用抽象图形而非真实画面：既能看出「这是一页漫画分镜」，又不涉及任何具体内容。
      const lines = Array.from({ length: 6 }, (_, k) => {
        const y = p.y + 40 + k * ((p.h - 80) / 5)
        return `<line x1="${p.x + 20}" y1="${y}" x2="${p.x + p.w - 20}" y2="${y}" stroke="#cfccc2" stroke-width="1" opacity="0.45"/>`
      }).join('')

      const bubbleX = p.x + p.w * (0.18 + rand() * 0.28)
      const bubbleY = p.y + p.h * (0.16 + rand() * 0.18)
      const bubbleW = p.w * (0.26 + rand() * 0.12)
      const bubbleH = Math.min(96, p.h * 0.16)

      const bubble = `<g opacity="0.9">
        <rect x="${bubbleX}" y="${bubbleY}" width="${bubbleW}" height="${bubbleH}" rx="16" fill="#fffdf8" stroke="#5a5a52" stroke-width="1.5"/>
        <path d="M${bubbleX + 20} ${bubbleY + bubbleH} L${bubbleX + 24} ${bubbleY + bubbleH + 18} L${bubbleX + 44} ${bubbleY + bubbleH}" fill="#fffdf8" stroke="#5a5a52" stroke-width="1.5"/>
      </g>`

      // 几何图形：大圆轮廓 / 实心矩形 / 速度线组，按分镜随机组合。
      // 第一个分镜必定有圆（保证任何一页都不会是空白分镜）。
      const shapes = []
      if (i < 2 || rand() > 0.25) {
        const r = Math.min(p.w, p.h) * (0.18 + rand() * 0.2)
        // 圆必须完整落在分镜框内（避免溢出画布被 viewBox 裁切，看起来像素材没画完）
        const cx = Math.min(p.x + p.w - r - 16, p.x + p.w * (0.5 + rand() * 0.3))
        const cy = Math.min(p.y + p.h - r - 16, p.y + p.h * (0.4 + rand() * 0.3))
        shapes.push(
          `<circle cx="${cx}" cy="${cy}" r="${r}" fill="#d9d6c8" opacity="0.75"/>
           <circle cx="${cx}" cy="${cy}" r="${r}" fill="none" stroke="#3d3d38" stroke-width="1.2" opacity="0.5"/>`,
        )
      }
      if (rand() > 0.45) {
        const w = p.w * (0.2 + rand() * 0.2)
        const h = p.h * (0.12 + rand() * 0.14)
        const x = p.x + p.w * (0.08 + rand() * 0.12)
        const y = p.y + p.h * (0.5 + rand() * 0.3)
        shapes.push(
          `<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="#b9b5a4" opacity="0.6"/>
           <rect x="${x}" y="${y}" width="${w}" height="${h}" fill="none" stroke="#3d3d38" stroke-width="1.2" opacity="0.45"/>`,
        )
      }
      if (rand() > 0.5) {
        const sx = p.x + p.w * 0.55
        const sy = p.y + 24
        const speed = Array.from({ length: 6 }, (_, k) => {
          const yy = sy + k * ((p.h * 0.3) / 6)
          return `<line x1="${sx}" y1="${yy}" x2="${sx + p.w * 0.34}" y2="${yy}" stroke="#8f8b7d" stroke-width="1" opacity="0.5"/>`
        }).join('')
        shapes.push(speed)
      }

      const tone = i % 2 === 0 ? '#eceade' : '#e6e3d6'
      return `<rect x="${p.x}" y="${p.y}" width="${p.w}" height="${p.h}" fill="${tone}" stroke="#3d3d38" stroke-width="2"/>
      <rect x="${p.x + 8}" y="${p.y + 8}" width="${p.w - 16}" height="${p.h - 16}" fill="none" stroke="#3d3d38" stroke-width="1" opacity="0.25"/>
      ${lines}
      ${shapes.join('')}
      ${bubble}`
    })
    .join('')

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">
  <rect width="${W}" height="${H}" fill="#f7f5ee"/>
  ${body}
  <text x="${W - M}" y="${H - 22}" text-anchor="end" fill="#9a968a" font-family="monospace" font-size="15">${index}</text>
</svg>
`
}

// ─────────────────────────────────────────────────────────────
// 写入
// ─────────────────────────────────────────────────────────────
const coverDir = join(HERE, 'covers')
const pageDir = join(HERE, 'pages')
await mkdir(coverDir, { recursive: true })
await mkdir(pageDir, { recursive: true })

for (const c of COVERS) {
  await writeFile(join(coverDir, `${c.file}.svg`), coverSvg(c), 'utf8')
}

const PAGE_COUNT = 12
for (let i = 1; i <= PAGE_COUNT; i++) {
  const idx = String(i).padStart(2, '0')
  // 页面种子与页码绑定：第 5 页永远是同样的版面，便于截图对比
  await writeFile(join(pageDir, `p${idx}.svg`), pageSvg(i, 1000 + i * 7), 'utf8')
}

console.log(`封面 ${COVERS.length} 张 → demo/assets/covers/`)
console.log(`内页 ${PAGE_COUNT} 张 → demo/assets/pages/`)
