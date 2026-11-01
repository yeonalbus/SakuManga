<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { API_BASE } from '@/config/api'
import { usePagePreview } from './usePagePreview'

/**
 * Round52：展开对比里的「页缩略图墙」。
 *
 * - 固定高度由外层（.thumb-wall）控制，内部滚动，2000+ 页也只会滚不会撑长页面；
 * - 初始渲染 12 张，滚到底（哨兵进入视口提前 120px）补 6 张，补完仍可滚则继续补；
 * - 点击缩略图 → 打开原图灯箱（PageLightbox）。
 */
const props = defineProps<{
  comicId: string
  pageCount: number
  label: string
  title: string
}>()

const INIT = 12
const BATCH = 6

const loaded = ref(0)
const busy = ref(false)
const wallRef = ref<HTMLElement | null>(null)
const sentinelRef = ref<HTMLElement | null>(null)
let observer: IntersectionObserver | null = null
let timer: number | null = null

const { openPreview } = usePagePreview()

const total = computed(() => Math.max(0, props.pageCount || 0))
const pages = computed(() => Array.from({ length: loaded.value }, (_, i) => i + 1))
const done = computed(() => loaded.value >= total.value)
const thumbUrl = (pageNumber: number) => `${API_BASE}/comics/${props.comicId}/page-thumb/${pageNumber - 1}`

const sentinelText = computed(() => {
  if (done.value) return `已全部加载（${total.value} 页）`
  if (busy.value) return '加载中…'
  return `继续向下滚动加载（剩 ${total.value - loaded.value} 页）`
})

function addPages(count: number) {
  loaded.value = Math.min(total.value, loaded.value + count)
}

function fillIfNeeded() {
  if (!wallRef.value || !sentinelRef.value || done.value) return
  const wall = wallRef.value.getBoundingClientRect()
  const sentinel = sentinelRef.value.getBoundingClientRect()
  if (sentinel.top <= wall.bottom + 120) loadMore()
}

function loadMore() {
  if (busy.value || done.value) return
  busy.value = true
  timer = window.setTimeout(() => {
    addPages(BATCH)
    busy.value = false
    timer = null
    window.requestAnimationFrame(fillIfNeeded)
  }, 160)
}

function onThumbClick(pageNumber: number) {
  openPreview(props.comicId, pageNumber, {
    label: props.label,
    title: props.title,
    pageCount: total.value,
  })
}

onMounted(() => {
  addPages(Math.min(INIT, total.value))
  if (sentinelRef.value && wallRef.value) {
    observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) loadMore()
        })
      },
      { root: wallRef.value, rootMargin: '0px 0px 120px 0px' },
    )
    observer.observe(sentinelRef.value)
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
  if (timer !== null) window.clearTimeout(timer)
  timer = null
})

// 切换对比成员时复用同一组件实例 → 必须重置加载进度（否则沿用上一本的已载页数）
watch(
  () => props.comicId,
  () => {
    if (timer !== null) {
      window.clearTimeout(timer)
      timer = null
    }
    busy.value = false
    loaded.value = Math.min(INIT, total.value)
    requestAnimationFrame(fillIfNeeded)
  },
)
</script>

<template>
  <div ref="wallRef" class="thumb-wall">
    <div class="tg">
      <button
        v-for="p in pages"
        :key="p"
        type="button"
        class="tg-thumb"
        :title="`第 ${p} 页（点击看原图）`"
        @click="onThumbClick(p)"
      >
        <img :src="thumbUrl(p)" :alt="`第 ${p} 页`" loading="lazy" />
        <span class="no">{{ p }}</span>
      </button>
    </div>
    <div ref="sentinelRef" class="tg-sentinel">{{ sentinelText }}</div>
  </div>
</template>

<style scoped>
.thumb-wall {
  height: 320px;
  overflow-y: auto;
  overscroll-behavior: contain;
  background: var(--app-bg-alt);
  border: 1px solid var(--app-border-2);
  border-radius: 6px;
  scrollbar-width: thin;
  scrollbar-color: #3a3a40 transparent;
}
.thumb-wall::-webkit-scrollbar {
  width: 8px;
}
.thumb-wall::-webkit-scrollbar-thumb {
  background: #3a3a40;
  border-radius: 4px;
}
.tg {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(64px, 1fr));
  gap: 4px;
  padding: 4px;
}
.tg-thumb {
  position: relative;
  padding: 0;
  aspect-ratio: 7 / 10;
  border-radius: 4px;
  overflow: hidden;
  border: 1px solid var(--app-border-2);
  background: #0b0b0d;
  cursor: zoom-in;
}
.tg-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.tg-thumb:hover {
  border-color: var(--app-accent);
}
.tg-thumb .no {
  position: absolute;
  right: 2px;
  bottom: 2px;
  font-size: 9px;
  line-height: 1;
  padding: 1px 3px;
  border-radius: 2px;
  background: rgba(0, 0, 0, 0.66);
  color: #c9c9cf;
}
.tg-sentinel {
  text-align: center;
  font-size: 11px;
  color: var(--app-text-muted);
  padding: 2px 0 8px;
}
</style>
