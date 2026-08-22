<script setup lang="ts">
// 🔍 视口诊断面板（隐藏调试用，仅 iPad PWA 底部条问题排查）
// 读取所有视口/布局实测值，点击「复制 JSON」可粘贴给开发者
import { ref, onMounted } from "vue"
import { useRouter } from "vue-router"

const router = useRouter()
const data = ref<Record<string, unknown>>({})
const copied = ref(false)

const measure = () => {
  const rect = (s: string) => {
    const el = document.querySelector(s)
    if (!el) return null
    const r = el.getBoundingClientRect()
    const cs = getComputedStyle(el)
    return { top: r.top, bottom: r.bottom, height: r.height, scrollH: el.scrollHeight, bg: cs.backgroundColor }
  }
  const docEl = document.documentElement
  const cs = getComputedStyle(docEl)
  const vv = window.visualViewport
  data.value = {
    // 设备
    ua: navigator.userAgent.slice(0, 120),
    standalone: (navigator as unknown as { standalone?: boolean }).standalone ?? null,
    displayMode: window.matchMedia("(display-mode: standalone)").matches ? "standalone" : "browser",
    // 视口高度
    screenH: window.screen.height,
    screenW: window.screen.width,
    innerH: window.innerHeight,
    innerW: window.innerWidth,
    visualViewportH: vv ? vv.height : null,
    visualViewportOffsetTop: vv ? vv.offsetTop : null,
    visualViewportScale: vv ? vv.scale : null,
    outerH: window.outerHeight,
    devicePixelRatio: window.devicePixelRatio,
    docClientH: docEl.clientHeight,
    docOffsetH: docEl.offsetHeight,
    docScrollH: docEl.scrollHeight,
    bodyScrollH: document.body ? document.body.scrollHeight : null,
    // safe-area
    safeTop: cs.getPropertyValue("--safe-top"),
    safeBottom: cs.getPropertyValue("--safe-bottom"),
    safeLeft: cs.getPropertyValue("--safe-left"),
    safeRight: cs.getPropertyValue("--safe-right"),
    envSafeTop: getComputedStyle(document.body).paddingTop,
    // 布局实测
    htmlBg: cs.backgroundColor,
    bodyBg: getComputedStyle(document.body).backgroundColor,
    app: rect("#app"),
    appContainer: rect(".app-container"),
    rightWrapper: rect(".right-wrapper"),
    mainContent: rect(".main-content"),
    topBar: rect(".top-bar"),
    // 计算的视口单位
    vh100Test: (() => { const d = document.createElement("div"); d.style.cssText = "position:fixed;top:0;height:100vh;visibility:hidden"; document.body.appendChild(d); const h = d.getBoundingClientRect().height; d.remove(); return h })(),
    dvh100Test: (() => { const d = document.createElement("div"); d.style.cssText = "position:fixed;top:0;height:100dvh;visibility:hidden"; document.body.appendChild(d); const h = d.getBoundingClientRect().height; d.remove(); return h })(),
    svh100Test: (() => { const d = document.createElement("div"); d.style.cssText = "position:fixed;top:0;height:100svh;visibility:hidden"; document.body.appendChild(d); const h = d.getBoundingClientRect().height; d.remove(); return h })(),
  }
}

const copyJson = async () => {
  try {
    await navigator.clipboard.writeText(JSON.stringify(data.value, null, 2))
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    // 剪贴板失败则 fallback
  }
}

onMounted(() => {
  measure()
  window.addEventListener("resize", measure)
  window.addEventListener("orientationchange", () => setTimeout(measure, 400))
  if (window.visualViewport) window.visualViewport.addEventListener("resize", measure)
})
</script>

<template>
  <div class="diag-page">
    <h2>🔍 视口诊断（iPad PWA 底部条）</h2>
    <div class="diag-actions">
      <button class="diag-btn" @click="measure">🔄 重新测量</button>
      <button class="diag-btn primary" @click="copyJson">{{ copied ? "✓ 已复制" : "📋 复制 JSON" }}</button>
      <button class="diag-btn" @click="router.back()">返回</button>
    </div>
    <p class="diag-tip">
      请分别<b>竖屏</b>与<b>横屏</b>各打开一次本页并复制 JSON，
      连同截图一起发给开发者。重点看 <code>screenH</code> vs <code>visualViewportH</code> vs <code>innerH</code> 的差值，
      以及 <code>mainContent.bottom</code> 是否等于 <code>visualViewportH</code>。
    </p>
    <pre class="diag-json">{{ JSON.stringify(data, null, 2) }}</pre>
  </div>
</template>

<style scoped>
.diag-page { padding: 20px; max-width: 720px; margin: 0 auto; color: var(--app-text-2); font-size: 13px; }
h2 { color: var(--app-text-strong); margin-bottom: 12px; }
.diag-actions { display: flex; gap: 10px; margin-bottom: 12px; flex-wrap: wrap; }
.diag-btn { padding: 8px 16px; border: 1px solid var(--app-border-3); border-radius: 6px; background: var(--app-surface-3); color: var(--app-text-2); cursor: pointer; font-size: 13px; }
.diag-btn.primary { background: #007acc; border-color: #007acc; color: #fff; }
.diag-tip { background: var(--app-surface-2); border: 1px solid var(--app-border-2); padding: 10px 12px; border-radius: 6px; line-height: 1.6; margin-bottom: 12px; }
.diag-tip code { background: var(--app-surface-3); padding: 1px 5px; border-radius: 3px; }
.diag-json { background: var(--app-bg-deep); border: 1px solid var(--app-border-2); padding: 12px; border-radius: 6px; overflow: auto; max-height: 70vh; font-family: monospace; font-size: 12px; white-space: pre-wrap; word-break: break-all; color: var(--app-text-2); }
</style>