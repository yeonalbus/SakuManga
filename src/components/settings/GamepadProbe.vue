<template>
  <div class="gp-probe">
    <div class="gp-probe-head">
      <span class="gp-probe-title">手柄诊断（定位用，定位完可删）</span>
      <button class="gp-probe-copy" @click="copyJson">
        {{ copied ? '已复制' : '复制 JSON' }}
      </button>
    </div>
    <pre class="gp-probe-body">{{ text }}</pre>
  </div>
</template>

<script setup lang="ts">
/**
 * 临时诊断组件：把浏览器层手柄可见性原样打印出来，用于定位「检测不到手柄」。
 *
 * 对比口径：
 * - pads.live > 0 → 浏览器能拿到手柄，若 SakuManga 仍显示“未检测到手柄”则是程序问题；
 * - pads.live = 0 → 浏览器/系统层没把手柄暴露成 Gamepad，与 SakuManga 代码无关。
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'

const text = ref('采样中…')
const copied = ref(false)

let timer: ReturnType<typeof setInterval> | null = null
let connectedCount = 0
let disconnectedCount = 0

interface StandaloneNavigator extends Navigator {
  standalone?: boolean
}

function describePad(pad: Gamepad): string {
  const pressed: number[] = []
  pad.buttons.forEach((btn, i) => {
    if (btn.pressed) pressed.push(i)
  })
  return JSON.stringify({
    index: pad.index,
    id: pad.id,
    connected: pad.connected,
    mapping: pad.mapping,
    buttons: pad.buttons.length,
    axes: pad.axes.length,
    pressed,
  })
}

function sample(): void {
  const lines: string[] = []
  const mql = window.matchMedia('(display-mode: standalone)')
  const hasApi = typeof navigator.getGamepads === 'function'

  lines.push(`ua: ${navigator.userAgent}`)
  lines.push(`touchPoints: ${navigator.maxTouchPoints} · secureContext: ${window.isSecureContext}`)
  lines.push(`topFrame: ${window.top === window.self} · standalone: ${mql.matches}`)
  lines.push(`iosStandalone: ${String((navigator as StandaloneNavigator).standalone ?? 'n/a')}`)
  lines.push(`getGamepads: ${hasApi}`)

  let pads: (Gamepad | null)[] = []
  let error = ''
  try {
    pads = hasApi ? Array.from(navigator.getGamepads()) : []
  } catch (e) {
    error = e instanceof Error ? e.message : String(e)
  }

  const live = pads.filter((p): p is Gamepad => !!p && p.connected)
  lines.push(`pads.length: ${pads.length} · pads.live: ${live.length}`)
  live.forEach((p) => lines.push(`  ${describePad(p)}`))
  if (error) lines.push(`getGamepads error: ${error}`)
  lines.push(`events: connected=${connectedCount} · disconnected=${disconnectedCount}`)
  lines.push(`sampleAt: ${new Date().toLocaleTimeString()}`)

  text.value = lines.join('\n')
  if (copied.value) copied.value = false
}

async function copyJson(): Promise<void> {
  try {
    await navigator.clipboard.writeText(text.value)
    copied.value = true
  } catch {
    copied.value = false
  }
}

const onConnected = (): void => {
  connectedCount += 1
  sample()
}
const onDisconnected = (): void => {
  disconnectedCount += 1
  sample()
}
const onVisible = (): void => {
  if (document.visibilityState === 'visible') sample()
}

onMounted(() => {
  window.addEventListener('gamepadconnected', onConnected)
  window.addEventListener('gamepaddisconnected', onDisconnected)
  document.addEventListener('visibilitychange', onVisible)
  sample()
  timer = setInterval(sample, 500)
})

onBeforeUnmount(() => {
  window.removeEventListener('gamepadconnected', onConnected)
  window.removeEventListener('gamepaddisconnected', onDisconnected)
  document.removeEventListener('visibilitychange', onVisible)
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.gp-probe {
  border: 1px dashed var(--app-border-3, #555);
  border-radius: 6px;
  padding: 10px 12px;
  background: var(--app-bg-deep, #101010);
}
.gp-probe-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}
.gp-probe-title {
  font-size: 12px;
  color: var(--app-text-3, #999);
}
.gp-probe-copy {
  padding: 3px 10px;
  font-size: 11px;
  border: 1px solid var(--app-border-3, #555);
  border-radius: 4px;
  background: var(--app-surface-3, #222);
  color: var(--app-text-2, #ccc);
  cursor: pointer;
}
.gp-probe-body {
  margin: 0;
  font-family: monospace;
  font-size: 11px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--app-text-2, #ccc);
  max-height: 40vh;
  overflow: auto;
}
</style>
