/**
 * 生成 PWA / iOS 主屏幕图标 public/apple-touch-icon.png（180x180）
 *
 * 纯 Node 实现（node:zlib 的 deflate + crc32），无需第三方依赖。
 * 图标为「深色底 + 蓝色圆角方块 + 浅色页面横条」，占位用，可自行替换。
 *
 * 用法：node scripts/gen-pwa-icon.mjs
 */
import { writeFileSync } from 'node:fs'
import { deflateSync, crc32 } from 'node:zlib'

const W = 180
const H = 180

// 配色
const BG = [18, 18, 18] // #121212
const ACCENT = [0, 122, 204] // #007acc
const LIGHT = [224, 224, 224] // #e0e0e0

// 生成 RGBA 原始像素（每行首字节为 filter type: None）
const raw = Buffer.alloc(H * (1 + W * 4))
const margin = 32
const radius = 24
const bw = W - 2 * margin
const bh = H - 2 * margin

for (let y = 0; y < H; y++) {
  const row = y * (1 + W * 4)
  raw[row] = 0
  for (let x = 0; x < W; x++) {
    const i = row + 1 + x * 4
    const ix = x - margin
    const iy = y - margin
    let c = BG

    if (ix >= 0 && iy >= 0 && ix < bw && iy < bh) {
      // 圆角矩形判定（与圆心距离法）
      const cx = Math.min(Math.max(ix, radius), bw - radius)
      const cy = Math.min(Math.max(iy, radius), bh - radius)
      const dist = Math.hypot(ix - cx, iy - cy)
      if (dist <= radius) c = ACCENT
      // 中间浅色页面横条
      if (iy > bh / 2 - 8 && iy < bh / 2 + 8 && ix > 14 && ix < bw - 14) {
        c = LIGHT
      }
    }

    raw[i] = c[0]
    raw[i + 1] = c[1]
    raw[i + 2] = c[2]
    raw[i + 3] = 255
  }
}

// PNG 分块组装
function chunk(type, data) {
  const len = Buffer.alloc(4)
  len.writeUInt32BE(data.length, 0)
  const typeBuf = Buffer.from(type, 'ascii')
  const crcBuf = Buffer.alloc(4)
  crcBuf.writeUInt32BE(crc32(Buffer.concat([typeBuf, data])) >>> 0, 0)
  return Buffer.concat([len, typeBuf, data, crcBuf])
}

const sig = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])

const ihdr = Buffer.alloc(13)
ihdr.writeUInt32BE(W, 0)
ihdr.writeUInt32BE(H, 4)
ihdr[8] = 8 // bit depth
ihdr[9] = 6 // color type: RGBA
ihdr[10] = 0
ihdr[11] = 0
ihdr[12] = 0

const png = Buffer.concat([
  sig,
  chunk('IHDR', ihdr),
  chunk('IDAT', deflateSync(raw)),
  chunk('IEND', Buffer.alloc(0)),
])

writeFileSync('public/apple-touch-icon.png', png)
console.log(`已生成 public/apple-touch-icon.png (${png.length} bytes, ${W}x${H})`)
