/**
 * useGamepad - 手柄输入 composable
 *
 * 通过 Gamepad API（iOS Safari 16.4+ / Chrome 35+）读取手柄状态：
 * - 按键上升沿检测：仅在「按下」瞬间触发，防止按住连发
 * - gamepadconnected / gamepaddisconnected 事件 + rAF 轮询兜底，
 *   （部分浏览器不派发事件，轮询可稳定检测连接状态）
 * - 按键映射从 readerSettings 实时读取，支持自定义键位
 *
 * 8BitDo Micro（默认映射）：
 *   下一页：D-Pad 右 / A    上一页：D-Pad 左 / B    开关设置菜单：Start / Select
 */
import { onBeforeUnmount, ref, watch } from 'vue'
import { readerSettings } from '@/stores/readerSettings'

export interface GamepadCallbacks {
  /** 下一页（阅读器内部已兼容 RTL / Webtoon 方向） */
  onNext?: () => void
  /** 上一页 */
  onPrev?: () => void
  /** 切换设置菜单 */
  onToggle?: () => void
}

export function useGamepad(callbacks: GamepadCallbacks = {}) {
  const isConnected = ref(false)
  const gamepadName = ref('')

  // 上一次采样中按下的按键集合（用于上升沿检测）
  let prevPressed = new Set<number>()
  let rafId = 0
  let enabled = false

  function isButtonDown(pad: Gamepad, index: number): boolean {
    const btn = pad.buttons[index]
    return !!btn && (btn.pressed || btn.value > 0.5)
  }

  function handlePad(pad: Gamepad): void {
    // 每次采样从设置实时读取映射，自定义键位即时生效
    const { gamepadNextKeys, gamepadPrevKeys, gamepadToggleKeys } = readerSettings

    const nowPressed = new Set<number>()
    pad.buttons.forEach((_btn, i) => {
      if (isButtonDown(pad, i)) nowPressed.add(i)
    })

    const fireOnce = (idx: number, fn?: () => void): void => {
      // 上升沿：本次按下 && 上次未按
      if (nowPressed.has(idx) && !prevPressed.has(idx)) fn?.()
    }

    gamepadNextKeys.forEach((i) => fireOnce(i, callbacks.onNext))
    gamepadPrevKeys.forEach((i) => fireOnce(i, callbacks.onPrev))
    gamepadToggleKeys.forEach((i) => fireOnce(i, callbacks.onToggle))

    prevPressed = nowPressed
  }

  function poll(): void {
    rafId = requestAnimationFrame(poll)
    if (!('getGamepads' in navigator)) return
    const pads = navigator.getGamepads()
    let active: Gamepad | null = null
    for (const pad of pads) {
      if (pad && pad.connected) {
        active = pad
        break
      }
    }
    if (active) {
      if (!isConnected.value) {
        isConnected.value = true
        gamepadName.value = active.id
      }
      handlePad(active)
    } else if (isConnected.value) {
      isConnected.value = false
      gamepadName.value = ''
      prevPressed = new Set<number>()
    }
  }

  function onGamepadConnected(e: GamepadEvent): void {
    isConnected.value = true
    gamepadName.value = e.gamepad.id
  }

  function onGamepadDisconnected(): void {
    isConnected.value = false
    gamepadName.value = ''
    prevPressed = new Set<number>()
  }

  function start(): void {
    if (enabled || !('getGamepads' in navigator)) return
    enabled = true
    // 事件监听 + rAF 轮询兜底
    window.addEventListener('gamepadconnected', onGamepadConnected)
    window.addEventListener('gamepaddisconnected', onGamepadDisconnected)
    poll()
  }

  function stop(): void {
    if (!enabled) return
    enabled = false
    cancelAnimationFrame(rafId)
    window.removeEventListener('gamepadconnected', onGamepadConnected)
    window.removeEventListener('gamepaddisconnected', onGamepadDisconnected)
    isConnected.value = false
    gamepadName.value = ''
    prevPressed = new Set<number>()
  }

  // 依据设置开关自动启停
  watch(
    () => readerSettings.enableGamepad,
    (enable) => (enable ? start() : stop()),
    { immediate: true },
  )

  onBeforeUnmount(stop)

  return { isConnected, gamepadName, start, stop }
}
