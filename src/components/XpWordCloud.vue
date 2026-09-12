<script setup lang="ts">
/**
 * XP 词云（Round32 阶段一）
 *
 * 自研 Canvas 阿基米德螺旋布局 + 空间网格加速矩形碰撞检测（不引入第三方依赖）。
 * - 字号：按权重对数归一映射（长尾词不会被压成不可读的小字）
 * - 配色：按命名空间取色（utils/tagColor.ts，与 TagChip 同源）
 * - 交互：hover 高亮 + tooltip（权重/本数）、点击 emit select（父级做快捷搜索跳转）
 * - 自适应：ResizeObserver 重排，支持高 DPI
 */
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import type { XpCloudTag } from '@/types/comic'
import { tagTextColor } from '@/utils/tagColor'

const props = withDefaults(
  defineProps<{
    tags: XpCloudTag[]
    /** 画布高度（CSS px） */
    height?: number
    /** 最多参与排布的词条数（超出部分丢弃：长尾词本就无展示价值） */
    maxWords?: number
  }>(),
  { height: 380, maxWords: 140 },
)

const emit = defineEmits<{ (e: 'select', tag: XpCloudTag): void }>()

interface Rect {
  x: number
  y: number
  w: number
  h: number
}

interface PlacedWord extends Rect {
  tag: XpCloudTag
  text: string
  fontSize: number
  color: string
}

const FONT_FAMILY =
  '"Helvetica Neue", Helvetica, Arial, "PingFang SC", "Microsoft YaHei", sans-serif'

const containerRef = ref<HTMLDivElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)
const placed = shallowRef<PlacedWord[]>([])
const tooltip = ref<{ visible: boolean; x: number; y: number; tag: XpCloudTag | null }>({
  visible: false,
  x: 0,
  y: 0,
  tag: null,
})

let ctx: CanvasRenderingContext2D | null = null
let hovered: PlacedWord | null = null
let dpr = 1
let widthCSS = 0
let heightCSS = 0
let rafId = 0
let resizeObserver: ResizeObserver | null = null

/** 字体串（动画状态下加粗） */
const fontOf = (size: number, bold = false) =>
  `${bold ? 800 : 700} ${size}px ${FONT_FAMILY}`

/** 超长词截断（避免单个长词把螺旋中心占满） */
const fitText = (text: string, size: number, maxWidth: number): string => {
  if (!ctx) return text
  ctx.font = fontOf(size)
  if (ctx.measureText(text).width <= maxWidth) return text
  let cut = text
  while (cut.length > 2 && ctx.measureText(cut + '…').width > maxWidth) {
    cut = cut.slice(0, -1)
  }
  return cut + '…'
}

/**
 * 螺旋排布：从中心向外，按权重降序依次落位。
 * 命中第一个不与已有词重叠的槽位即放置；容器放不下则丢弃该词（长尾自然被裁剪）。
 */
const computeLayout = (): PlacedWord[] => {
  if (!ctx || widthCSS <= 0 || heightCSS <= 0) return []
  const list = props.tags.slice(0, props.maxWords)
  if (list.length === 0) return []

  const compact = widthCSS < 560
  const fontSizeMin = compact ? 11 : 12
  const fontSizeMax = compact ? 22 : 34

  // 权重对数归一（weight 已是 0~1 归一值，再取 log 平滑分布）
  const logs = list.map((t) => Math.log(Math.max(1e-6, t.weight)))
  const logMax = Math.max(...logs)
  const logSpan = Math.max(1e-6, logMax - Math.min(...logs))

  // 空间网格加速碰撞检测
  const CELL = 28
  const grid = new Map<string, Rect[]>()
  const keyOf = (gx: number, gy: number) => `${gx}:${gy}`

  const insertGrid = (r: Rect) => {
    const x0 = Math.floor(r.x / CELL)
    const x1 = Math.floor((r.x + r.w) / CELL)
    const y0 = Math.floor(r.y / CELL)
    const y1 = Math.floor((r.y + r.h) / CELL)
    for (let gx = x0; gx <= x1; gx++) {
      for (let gy = y0; gy <= y1; gy++) {
        const k = keyOf(gx, gy)
        const bucket = grid.get(k)
        if (bucket) bucket.push(r)
        else grid.set(k, [r])
      }
    }
  }

  const collides = (r: Rect): boolean => {
    const x0 = Math.floor(r.x / CELL)
    const x1 = Math.floor((r.x + r.w) / CELL)
    const y0 = Math.floor(r.y / CELL)
    const y1 = Math.floor((r.y + r.h) / CELL)
    for (let gx = x0; gx <= x1; gx++) {
      for (let gy = y0; gy <= y1; gy++) {
        const bucket = grid.get(keyOf(gx, gy))
        if (!bucket) continue
        for (const o of bucket) {
          if (r.x < o.x + o.w && r.x + r.w > o.x && r.y < o.y + o.h && r.y + r.h > o.y) {
            return true
          }
        }
      }
    }
    return false
  }

  const ASPECT = 0.62 // 纵向压扁：贴合宽扁容器，词云更紧凑
  const findSlot = (w: number, h: number): { x: number; y: number } | null => {
    const cx = widthCSS / 2 - w / 2
    const cy = heightCSS / 2 - h / 2
    const maxR = Math.max(widthCSS, heightCSS) * 0.7
    for (let r = 0; r <= maxR; r += 2.5) {
      const steps = Math.max(12, Math.round((2 * Math.PI * Math.max(r * ASPECT, 10)) / 7))
      const phase = r * 0.35 // 每圈相位偏移，避免同一角度反复落空
      for (let i = 0; i < steps; i++) {
        const angle = (i / steps) * Math.PI * 2 + phase
        const x = cx + r * Math.cos(angle)
        const y = cy + r * ASPECT * Math.sin(angle)
        if (x < 1 || y < 1 || x + w > widthCSS - 1 || y + h > heightCSS - 1) continue
        const rect: Rect = { x, y, w, h }
        if (!collides(rect)) return { x, y }
      }
    }
    return null
  }

  const words: PlacedWord[] = []
  for (let i = 0; i < list.length; i++) {
    const tag = list[i]
    const ratio = (logs[i] - (logMax - logSpan)) / logSpan // 0（最小）~1（最大）
    const fontSize = Math.round(fontSizeMin + (fontSizeMax - fontSizeMin) * ratio)
    const text = fitText(tag.name || tag.key, fontSize, widthCSS * 0.62)
    ctx.font = fontOf(fontSize)
    const w = ctx.measureText(text).width
    const h = fontSize * 1.2
    const slot = findSlot(w, h)
    if (!slot) continue
    const word: PlacedWord = {
      tag,
      text,
      x: slot.x,
      y: slot.y,
      w,
      h,
      fontSize,
      color: tagTextColor(tag.namespace),
    }
    words.push(word)
    insertGrid(word)
  }
  return words
}

/** 绘制（hover 时非命中词降透明度，突出当前词） */
const draw = () => {
  const canvas = canvasRef.value
  if (!canvas || !ctx) return
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, widthCSS, heightCSS)
  ctx.textBaseline = 'top'

  for (const word of placed.value) {
    const isHovered = hovered === word
    ctx.font = fontOf(word.fontSize, isHovered)
    ctx.fillStyle = word.color
    ctx.globalAlpha = hovered && !isHovered ? 0.4 : 1
    ctx.fillText(word.text, word.x, word.y)
    if (isHovered) {
      ctx.globalAlpha = 1
      ctx.strokeStyle = word.color
      ctx.lineWidth = 1.5
      ctx.beginPath()
      ctx.moveTo(word.x, word.y + word.h - 1)
      ctx.lineTo(word.x + word.w, word.y + word.h - 1)
      ctx.stroke()
    }
  }
  ctx.globalAlpha = 1

  if (placed.value.length === 0) {
    ctx.fillStyle = '#9aa4b2'
    ctx.font = fontOf(13)
    ctx.textAlign = 'center'
    ctx.fillText('暂无可统计的标签', widthCSS / 2, heightCSS / 2 - 8)
    ctx.textAlign = 'left'
  }
}

/** 重排 + 重绘 */
const relayout = () => {
  const canvas = canvasRef.value
  const container = containerRef.value
  if (!canvas || !container || !ctx) return
  widthCSS = container.clientWidth
  heightCSS = props.height
  dpr = Math.min(window.devicePixelRatio || 1, 2)
  canvas.width = Math.max(1, Math.floor(widthCSS * dpr))
  canvas.height = Math.max(1, Math.floor(heightCSS * dpr))
  canvas.style.width = `${widthCSS}px`
  canvas.style.height = `${heightCSS}px`
  hovered = null
  tooltip.value.visible = false
  placed.value = computeLayout()
  draw()
  exposeLayoutDebug()
}

/**
 * 布局调试快照（只读，供实机测试与问题排查读取）。
 * 布局为纯几何计算，快照可直接断言「是否重叠 / 是否越界」，比像素分析可靠。
 */
const exposeLayoutDebug = () => {
  if (typeof window === 'undefined') return
  ;(window as unknown as Record<string, unknown>).__xpWordCloudDebug = {
    total: props.tags.length,
    placed: placed.value.length,
    size: { width: widthCSS, height: heightCSS },
    words: placed.value.map((w) => ({
      text: w.text,
      ns: w.tag.namespace,
      x: w.x,
      y: w.y,
      w: w.w,
      h: w.h,
      fontSize: w.fontSize,
    })),
  }
}

const scheduleRelayout = () => {
  if (rafId) cancelAnimationFrame(rafId)
  rafId = requestAnimationFrame(() => {
    rafId = 0
    relayout()
  })
}

/** 命中测试：逆序遍历（后绘制的在上层） */
const hitTest = (x: number, y: number): PlacedWord | null => {
  const list = placed.value
  for (let i = list.length - 1; i >= 0; i--) {
    const w = list[i]
    if (x >= w.x && x <= w.x + w.w && y >= w.y && y <= w.y + w.h) return w
  }
  return null
}

const localPos = (e: MouseEvent | TouchEvent) => {
  const canvas = canvasRef.value
  if (!canvas) return { x: 0, y: 0 }
  const rect = canvas.getBoundingClientRect()
  const point = 'touches' in e ? e.touches[0] : (e as MouseEvent)
  return { x: point.clientX - rect.left, y: point.clientY - rect.top }
}

const onMove = (e: MouseEvent) => {
  const { x, y } = localPos(e)
  const hit = hitTest(x, y)
  if (hit !== hovered) {
    hovered = hit
    draw()
  }
  if (hit) {
    tooltip.value = { visible: true, x, y, tag: hit.tag }
  } else {
    tooltip.value.visible = false
  }
}

const onLeave = () => {
  if (hovered) {
    hovered = null
    draw()
  }
  tooltip.value.visible = false
}

const onClick = (e: MouseEvent) => {
  const { x, y } = localPos(e)
  const hit = hitTest(x, y)
  if (hit) emit('select', hit.tag)
}

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return
  ctx = canvas.getContext('2d')
  relayout()
  if (containerRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => scheduleRelayout())
    resizeObserver.observe(containerRef.value)
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  if (rafId) cancelAnimationFrame(rafId)
})

watch(
  () => props.tags,
  () => scheduleRelayout(),
)
watch(
  () => props.height,
  () => scheduleRelayout(),
)
</script>

<template>
  <div ref="containerRef" class="xp-cloud">
    <canvas
      ref="canvasRef"
      class="xp-canvas"
      @mousemove="onMove"
      @mouseleave="onLeave"
      @click="onClick"
    />
    <div
      v-if="tooltip.visible && tooltip.tag"
      class="xp-tooltip"
      :style="{ left: `${tooltip.x}px`, top: `${tooltip.y}px` }"
    >
      <div class="tt-title">
        <span class="tt-ns">{{ tooltip.tag.namespace }}:</span>{{ tooltip.tag.name }}
      </div>
      <div class="tt-meta">
        权重 {{ Math.round(tooltip.tag.weight * 100) }} · 库藏 {{ tooltip.tag.comicCount }} 本 · 阅读
        {{ Math.round(tooltip.tag.readWeight * 100) }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.xp-cloud {
  position: relative;
  width: 100%;
}

.xp-canvas {
  display: block;
  cursor: pointer;
}

.xp-tooltip {
  position: absolute;
  z-index: 5;
  transform: translate(12px, 12px);
  max-width: 260px;
  padding: 6px 10px;
  border-radius: 6px;
  background: var(--app-bg-2, rgba(20, 22, 28, 0.94));
  border: 1px solid var(--app-border-2, rgba(255, 255, 255, 0.14));
  color: var(--app-text-strong, #fff);
  font-size: 0.78rem;
  line-height: 1.45;
  pointer-events: none;
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.28);
}

.tt-ns {
  opacity: 0.65;
  margin-right: 2px;
}

.tt-meta {
  margin-top: 2px;
  font-size: 0.72rem;
  color: var(--app-text-3, #9aa4b2);
}
</style>
