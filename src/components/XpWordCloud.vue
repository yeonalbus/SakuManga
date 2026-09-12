<script setup lang="ts">
/**
 * XP 词云（Round32 阶段一；视觉改版）
 *
 * 自研 Canvas 阿基米德螺旋（归一化椭圆）+ 空间网格加速矩形碰撞检测，无第三方依赖。
 *
 * 视觉要点（详见 plans/round32-xp-cloud-recommend-plan.md 第十一章）：
 * - 碰撞盒留白：词间保留缝隙（lineRatio 1.45 / padX 0.3），解决旧版"密不透风"
 * - 椭圆轮廓：螺旋按归一化椭圆扩散，边缘自然收拢，不再矩形锯齿
 * - 字号分段 + 长词降档：前 N 名占高档区间形成焦点；长名降档避免占满半行
 * - 配色景深：命名空间主色 + 按权重调制明度/饱和/透明度（大词亮、尾词淡）
 * - 全水平排版：不做竖排/倾斜（竖排词在密集词云里会与相邻词视觉粘连）
 * - 文本治理：过滤超长词条、可只保留有中日韩译名的词条、按词边界截断
 *
 * 数据侧清洗（markdown 图标 / emoji / 双语取短侧）由后端 CleanTagDisplayName 完成。
 */
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import type { XpCloudTag } from '@/types/comic'
import { readXpTheme, xpTagColor } from '@/utils/tagColor'
import { xpCloudSettings } from '@/stores/xpCloudSettings'

const props = withDefaults(
  defineProps<{
    tags: XpCloudTag[]
    /** 画布高度（CSS px）：桌面 520 / 移动 360 */
    height?: number
  }>(),
  { height: 520 },
)

const emit = defineEmits<{
  (e: 'select', tag: XpCloudTag): void
  (e: 'stats', stats: XpCloudStats): void
}>()

/** 布局统计（供面板展示"放入 X 条 / 排除 Y 条"，也让用户理解参数影响） */
export interface XpCloudStats {
  source: number // 数据源词条数
  droppedLong: number // 超长被排除
  droppedNoTrans: number // 无译名被排除
  wanted: number // 取用词条数
  placed: number // 实际放入
  overflow: number // 放不下丢弃
}

interface Rect {
  x: number
  y: number
  w: number
  h: number
}

interface Candidate {
  tag: XpCloudTag
  text: string
  fullName: string
  truncated: boolean
}

interface PlacedWord extends Rect {
  tag: XpCloudTag
  text: string
  fullName: string
  truncated: boolean
  textW: number
  fontSize: number
  color: string
  t: number
}

const FONT_FAMILY =
  '"Helvetica Neue", Helvetica, Arial, "PingFang SC", "Microsoft YaHei", sans-serif'
/** 前 N 名占字号高档区间，形成视觉焦点 */
const TOP_N = 10
/** 中日韩字符（判断"是否有译名"） */
const RE_CJK = /[\u4e00-\u9fff\u3040-\u30ff\uac00-\ud7af]/

const containerRef = ref<HTMLDivElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)
const placed = shallowRef<PlacedWord[]>([])
const tooltip = ref<{ visible: boolean; x: number; y: number; word: PlacedWord | null }>({
  visible: false,
  x: 0,
  y: 0,
  word: null,
})

let ctx: CanvasRenderingContext2D | null = null
/** 离屏测量上下文（量文字宽度用；与主画布无关） */
const measureCtx =
  typeof document !== 'undefined' ? document.createElement('canvas').getContext('2d') : null
let hovered: PlacedWord | null = null
let dpr = 1
let widthCSS = 0
let heightCSS = 0
let rafId = 0
let resizeObserver: ResizeObserver | null = null
let themeObserver: MutationObserver | null = null

const fontOf = (size: number) => `700 ${size}px ${FONT_FAMILY}`

/** 短名截断：英文按词边界断开，中文直接截断，末尾加省略号 */
const shorten = (text: string, max: number): string => {
  if (text.length <= max) return text
  const cut = text.slice(0, max)
  if (/[A-Za-z0-9]$/.test(cut) && /[A-Za-z0-9]/.test(text[max] || '')) {
    const sp = cut.lastIndexOf(' ')
    if (sp > max * 0.5) return `${cut.slice(0, sp)}…`
  }
  return `${cut}…`
}

/** 字号：前 TOP_N 名占高档区间（1.0→0.58），其余按排名压到低档（0.58→0），层次分明 */
const fontSizeOf = (rank: number, total: number, fontMin: number, fontMax: number): number => {
  const n = Math.min(TOP_N, total)
  let t: number
  if (rank < n) {
    t = 1 - (rank / n) * 0.42
  } else {
    const rest = Math.max(1, total - n)
    t = 0.58 * (1 - (rank - n) / rest)
  }
  return Math.round(fontMin + (fontMax - fontMin) * t)
}

/** 长词降档：拉丁转写长名（无中文译名时的回退）不抢中心、不占半行 */
const lengthFactorOf = (len: number): number => (len > 16 ? 0.55 : len > 12 ? 0.75 : len > 9 ? 0.9 : 1)

/** 数据预处理：过滤 → 截断（清洗已由后端完成） */
const prepare = (tags: XpCloudTag[]): { list: Candidate[]; stats: XpCloudStats } => {
  const s = xpCloudSettings
  const list: Candidate[] = []
  let droppedLong = 0
  let droppedNoTrans = 0

  for (const tag of tags) {
    const name = (tag.name || tag.key || '').trim()
    if (!name) continue
    if (s.onlyTranslated && !RE_CJK.test(name)) {
      droppedNoTrans++
      continue
    }
    if (name.length > s.maxChars) {
      droppedLong++
      continue
    }
    list.push({
      tag,
      text: shorten(name, s.truncChars),
      fullName: name,
      truncated: name.length > s.truncChars,
    })
  }

  return {
    list,
    stats: {
      source: tags.length,
      droppedLong,
      droppedNoTrans,
      wanted: Math.min(s.wordCount, list.length),
      placed: 0,
      overflow: 0,
    },
  }
}

/** 布局：归一化椭圆螺旋 + 空间网格碰撞 */
const computeLayout = (candidates: Candidate[]): { placed: PlacedWord[]; overflow: number } => {
  const s = xpCloudSettings
  const words: PlacedWord[] = []
  const rects: Rect[] = []
  const grid = new Map<string, Rect[]>()
  const CELL = 34
  const keyOf = (gx: number, gy: number) => `${gx}:${gy}`
  let overflow = 0

  // 无测量上下文（SSR/极端环境）时直接返回空布局
  const mctx = measureCtx
  if (!mctx) return { placed: words, overflow: 0 }

  const insert = (r: Rect) => {
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
          if (r.x < o.x + o.w && r.x + r.w > o.x && r.y < o.y + o.h && r.y + r.h > o.y) return true
        }
      }
    }
    return false
  }

  const cx = widthCSS / 2
  const cy = heightCSS / 2
  const total = candidates.length

  for (let i = 0; i < total; i++) {
    const c = candidates[i]
    let fontSize = fontSizeOf(i, total, s.fontMin, s.fontMax)
    const factor = lengthFactorOf(c.text.length)
    if (factor < 1) fontSize = Math.max(s.fontMin, Math.round(fontSize * factor))

    mctx.font = fontOf(fontSize)
    const textW = mctx.measureText(c.text).width
    const padX = fontSize * s.padX
    const bw = textW + padX
    const bh = fontSize * s.lineRatio

    const aMax = Math.max(12, (widthCSS / 2) * 0.99 - bw / 2)
    const bMax = Math.max(12, (heightCSS / 2) * 0.99 - bh / 2)
    let slot: Rect | null = null

    outer: for (let u = 0; u <= 1.0001; u += 0.012) {
      const steps = Math.max(14, Math.round(u * 88))
      const phase = u * 7.5
      for (let k = 0; k < steps; k++) {
        const ang = (k / steps) * Math.PI * 2 + phase
        const dx = Math.cos(ang) * aMax * u
        const dy = Math.sin(ang) * bMax * u
        // 椭圆轮廓：盒子中心必须落在椭圆内
        if ((dx / aMax) ** 2 + (dy / bMax) ** 2 > 1) continue
        const rect: Rect = { x: cx + dx - bw / 2, y: cy + dy - bh / 2, w: bw, h: bh }
        if (!collides(rect)) {
          slot = rect
          break outer
        }
      }
    }

    if (!slot) {
      overflow++
      continue
    }

    const t = total > 1 ? 1 - i / (total - 1) : 1
    const word: PlacedWord = {
      tag: c.tag,
      text: c.text,
      fullName: c.fullName,
      truncated: c.truncated,
      textW,
      x: slot.x,
      y: slot.y,
      w: slot.w,
      h: slot.h,
      fontSize,
      // 配色跟随 App 主题（深色底用亮色，浅色底用深色）
      color: xpTagColor(c.tag.namespace, t, readXpTheme()),
      t,
    }
    words.push(word)
    rects.push(slot)
    insert(slot)
  }

  return { placed: words, overflow }
}

const draw = () => {
  const canvas = canvasRef.value
  if (!canvas || !ctx) return
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, widthCSS, heightCSS)
  ctx.textAlign = 'left'
  ctx.textBaseline = 'middle'

  for (const word of placed.value) {
    const isHovered = hovered === word
    ctx.font = fontOf(word.fontSize)
    ctx.fillStyle = word.color
    // hover 时其余词降透明度，突出当前词
    ctx.globalAlpha = hovered && !isHovered ? 0.35 : 1
    // 文字在碰撞盒内水平居中（盒宽 = 文字宽 + 左右留白）
    ctx.fillText(word.text, word.x + (word.w - word.textW) / 2, word.y + word.h / 2)
  }
  ctx.globalAlpha = 1

  if (placed.value.length === 0) {
    ctx.fillStyle = '#9aa4b2'
    ctx.font = `500 13px ${FONT_FAMILY}`
    ctx.textAlign = 'center'
    ctx.fillText('没有符合当前筛选条件的标签（可放宽词云参数）', widthCSS / 2, heightCSS / 2)
    ctx.textAlign = 'left'
  }
}

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

  const { list, stats } = prepare(props.tags)
  const { placed: words, overflow } = computeLayout(list.slice(0, stats.wanted))
  placed.value = words
  stats.placed = words.length
  stats.overflow = overflow
  draw()
  exposeLayoutDebug()
  emit('stats', stats)
}

const scheduleRelayout = () => {
  if (rafId) cancelAnimationFrame(rafId)
  rafId = requestAnimationFrame(() => {
    rafId = 0
    relayout()
  })
}

/** 布局调试快照（只读，供实机测试与问题排查读取） */
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
      color: w.color,
    })),
  }
}

const hitTest = (x: number, y: number): PlacedWord | null => {
  const list = placed.value
  for (let i = list.length - 1; i >= 0; i--) {
    const w = list[i]
    if (x >= w.x && x <= w.x + w.w && y >= w.y && y <= w.y + w.h) return w
  }
  return null
}

const localPos = (e: MouseEvent) => {
  const canvas = canvasRef.value
  if (!canvas) return { x: 0, y: 0 }
  const rect = canvas.getBoundingClientRect()
  return { x: e.clientX - rect.left, y: e.clientY - rect.top }
}

const onMove = (e: MouseEvent) => {
  const { x, y } = localPos(e)
  const hit = hitTest(x, y)
  if (hit !== hovered) {
    hovered = hit
    draw()
  }
  if (hit) {
    tooltip.value = { visible: true, x, y, word: hit }
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
  // 主题切换（<html data-theme>）时重新取色重绘
  if (typeof MutationObserver !== 'undefined') {
    themeObserver = new MutationObserver(() => scheduleRelayout())
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme'],
    })
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  themeObserver?.disconnect()
  themeObserver = null
  if (rafId) cancelAnimationFrame(rafId)
})

watch(() => props.tags, scheduleRelayout)
watch(() => props.height, scheduleRelayout)
// 参数变化（用户在面板拖动滑块）实时重排
watch(() => ({ ...xpCloudSettings }), scheduleRelayout, { deep: true })

defineExpose({ relayout })
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
      v-if="tooltip.visible && tooltip.word"
      class="xp-tooltip"
      :style="{ left: `${tooltip.x}px`, top: `${tooltip.y}px` }"
    >
      <div class="tt-title">
        <span class="tt-ns">{{ tooltip.word.tag.namespace }}:</span>{{ tooltip.word.fullName
        }}<span v-if="tooltip.word.truncated" class="tt-cut">（已截断）</span>
      </div>
      <div class="tt-meta">
        权重 {{ Math.round(tooltip.word.tag.weight * 100) }} · 库藏
        {{ tooltip.word.tag.comicCount }} 本 · 阅读
        {{ Math.round(tooltip.word.tag.readWeight * 100) }}
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
  max-width: 340px;
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

.tt-cut {
  opacity: 0.5;
  font-size: 0.72rem;
  margin-left: 4px;
}

.tt-meta {
  margin-top: 2px;
  font-size: 0.72rem;
  color: var(--app-text-3, #9aa4b2);
}
</style>
