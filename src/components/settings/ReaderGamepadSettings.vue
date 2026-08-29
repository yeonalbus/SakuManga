<template>
  <div class="gamepad-settings">
    <!-- 页头 -->
    <div class="page-header">
      <button class="back-btn" @click="$emit('back')">← 返回</button>
      <div class="page-title">🎮 手柄设置</div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">启用手柄控制</div>
        <div class="item-subtext">配合 8BitDo Micro 手柄按键翻页</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.enableGamepad" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">手柄连接状态</div>
        <div class="item-subtext" :class="{ 'text-connected': gamepadConnected }">
          {{ gamepadConnected ? `已连接：${gamepadName}` : '未检测到手柄' }}
        </div>
      </div>
    </div>

    <div v-for="slot in keySlots" :key="slot.key" class="setting-item setting-column">
      <div class="item-info">
        <div class="item-title">{{ slot.label }}</div>
        <div class="key-list">
          <span v-for="k in readerSettings[slot.key]" :key="k" class="key-chip">
            {{ btnName(k) }}
            <button class="key-remove" title="移除该键位" @click="removeKey(slot.key, k)">×</button>
          </span>
          <span v-if="readerSettings[slot.key].length === 0" class="key-empty">未设置</span>
        </div>
      </div>
      <button
        class="key-capture"
        :class="{ capturing: captureSlot === slot.key }"
        @click="startCapture(slot.key)"
      >
        {{ captureSlot === slot.key ? '请按手柄按键…' : '＋ 录制' }}
      </button>
    </div>

    <div class="preset-row">
      <button class="preset-btn" @click="applyMicroPreset">🔄 恢复 8BitDo Micro 默认键位</button>
    </div>

    <!-- Round14：双击快速确认切本 -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">双击翻页键快速切本</div>
        <div class="item-subtext">
          读到最后一页双击「下一页」（第一页双击「上一页」）直接切到清单下一/上一本，无需弹窗确认；单击仍弹确认框防误触
        </div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.gamepadDoubleTapConfirm" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">双击时间窗（毫秒）</div>
        <div class="item-subtext">两次按键间隔小于该值视为双击确认</div>
      </div>
      <select v-model="readerSettings.gamepadDoubleTapWindow" class="setting-select">
        <option :value="300">300ms（快速）</option>
        <option :value="500">500ms（默认）</option>
        <option :value="800">800ms（宽松）</option>
      </select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useUI } from '@/composables/useUI'
import { readerSettings, GAMEPAD_BUTTONS } from '@/stores/readerSettings'

defineEmits<{ (e: 'back'): void }>()

const { toast } = useUI()

// ── 连接状态检测 ──
const gamepadConnected = ref(false)
const gamepadName = ref('')

function getActivePad(): Gamepad | null {
  if (!('getGamepads' in navigator)) return null
  const pads = navigator.getGamepads()
  for (const pad of pads) {
    if (pad && pad.connected) return pad
  }
  return null
}

function refreshConnection(): void {
  const pad = getActivePad()
  gamepadConnected.value = !!pad
  gamepadName.value = pad ? pad.id : ''
}

let statusTimer: ReturnType<typeof setInterval> | null = null

// ── 键位映射（8BitDo Micro 预设 + 自定义录制） ──
const KEY_BUTTON_NAMES: Record<number, string> = {
  [GAMEPAD_BUTTONS.A]: 'A',
  [GAMEPAD_BUTTONS.B]: 'B',
  [GAMEPAD_BUTTONS.X]: 'X',
  [GAMEPAD_BUTTONS.Y]: 'Y',
  [GAMEPAD_BUTTONS.LB]: 'LB',
  [GAMEPAD_BUTTONS.RB]: 'RB',
  [GAMEPAD_BUTTONS.LT]: 'LT',
  [GAMEPAD_BUTTONS.RT]: 'RT',
  [GAMEPAD_BUTTONS.SELECT]: 'Select',
  [GAMEPAD_BUTTONS.START]: 'Start',
  [GAMEPAD_BUTTONS.L3]: 'L3',
  [GAMEPAD_BUTTONS.R3]: 'R3',
  [GAMEPAD_BUTTONS.DPAD_UP]: 'D-Pad↑',
  [GAMEPAD_BUTTONS.DPAD_DOWN]: 'D-Pad↓',
  [GAMEPAD_BUTTONS.DPAD_LEFT]: 'D-Pad←',
  [GAMEPAD_BUTTONS.DPAD_RIGHT]: 'D-Pad→',
}

function btnName(key: number): string {
  return KEY_BUTTON_NAMES[key] ?? `键${key}`
}

const keySlots = [
  { key: 'gamepadNextKeys', label: '下一页按键' },
  { key: 'gamepadPrevKeys', label: '上一页按键' },
  { key: 'gamepadToggleKeys', label: '切换设置菜单' },
  // Round14：确认/取消（modal 与双击切本共用；可自定义防与翻页键冲突）
  { key: 'gamepadConfirmKeys', label: '确认按键' },
  { key: 'gamepadCancelKeys', label: '取消按键' },
] as const

type GamepadKeySlot = (typeof keySlots)[number]['key']

const captureSlot = ref<GamepadKeySlot | null>(null)
const prevPressed = new Set<number>()
let captureTimer: ReturnType<typeof setInterval> | null = null

const getKeyArr = (slot: GamepadKeySlot): number[] => readerSettings[slot]

function startCapture(slot: GamepadKeySlot): void {
  stopCapture()
  captureSlot.value = slot
  prevPressed.clear()
  const pad = getActivePad()
  if (pad) {
    pad.buttons.forEach((b, i) => {
      if (b.pressed) prevPressed.add(i)
    })
  }
  // 轮询等待下一次按键（上升沿），50ms 采样一次
  captureTimer = setInterval(() => {
    const currentSlot = captureSlot.value
    if (!currentSlot) return
    const p = getActivePad()
    if (!p) return
    p.buttons.forEach((b, i) => {
      if (!b.pressed) return
      if (!prevPressed.has(i)) {
        const arr = getKeyArr(currentSlot)
        if (!arr.includes(i)) arr.push(i)
        toast.success(`已录制 ${btnName(i)}`)
        stopCapture()
      } else {
        prevPressed.add(i)
      }
    })
  }, 50)
}

function stopCapture(): void {
  captureSlot.value = null
  if (captureTimer) {
    clearInterval(captureTimer)
    captureTimer = null
  }
  prevPressed.clear()
}

function removeKey(slot: GamepadKeySlot, key: number): void {
  const arr = getKeyArr(slot)
  const idx = arr.indexOf(key)
  if (idx >= 0) arr.splice(idx, 1)
}

function applyMicroPreset(): void {
  readerSettings.gamepadNextKeys = [GAMEPAD_BUTTONS.DPAD_RIGHT, GAMEPAD_BUTTONS.A]
  readerSettings.gamepadPrevKeys = [GAMEPAD_BUTTONS.DPAD_LEFT, GAMEPAD_BUTTONS.B]
  readerSettings.gamepadToggleKeys = [GAMEPAD_BUTTONS.START, GAMEPAD_BUTTONS.SELECT]
  // Round14：确认/取消按键恢复默认（A=确认 B=取消）
  readerSettings.gamepadConfirmKeys = [GAMEPAD_BUTTONS.A]
  readerSettings.gamepadCancelKeys = [GAMEPAD_BUTTONS.B]
  readerSettings.gamepadDoubleTapConfirm = true
  readerSettings.gamepadDoubleTapWindow = 500
  toast.success('已恢复 8BitDo Micro 默认键位')
}

onMounted(() => {
  refreshConnection()
  window.addEventListener('gamepadconnected', refreshConnection)
  window.addEventListener('gamepaddisconnected', refreshConnection)
  statusTimer = setInterval(refreshConnection, 3000)
})

onBeforeUnmount(() => {
  window.removeEventListener('gamepadconnected', refreshConnection)
  window.removeEventListener('gamepaddisconnected', refreshConnection)
  if (statusTimer) clearInterval(statusTimer)
  stopCapture()
})
</script>

<style scoped>
.gamepad-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* 页头 */
.page-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.back-btn {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.back-btn:hover {
  background: var(--app-border-3);
  color: var(--app-text-strong);
}

.page-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--app-text-strong);
}

.setting-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background-color: var(--app-surface-2);
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
  transition: background-color 0.2s ease;
}

.item-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.item-title {
  font-size: 15px;
  font-weight: 500;
  color: var(--app-text-strong);
}

.item-subtext {
  font-size: 13px;
  color: var(--app-text-3);
  line-height: 1.4;
}

.setting-select {
  background-color: transparent;
  color: var(--app-text-strong);
  border: none;
  font-size: 14px;
  padding: 4px 8px;
  cursor: pointer;
  outline: none;
  border-bottom: 1px solid var(--app-border-3);
  text-align-last: right;
  transition: border-color 0.2s;
}

.setting-select:focus {
  border-bottom-color: #ff7588;
}

.setting-select option {
  background-color: var(--app-surface-2);
  color: var(--app-text-strong);
}

.toggle-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
  flex-shrink: 0;
}

.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--app-border-3);
  transition: 0.3s;
  border-radius: 24px;
}

.slider:before {
  position: absolute;
  content: '';
  height: 18px;
  width: 18px;
  left: 3px;
  bottom: 3px;
  background-color: var(--app-text-2);
  transition: 0.3s;
  border-radius: 50%;
}

input:checked + .slider {
  background-color: #ff7588;
}

input:checked + .slider:before {
  transform: translateX(20px);
  background-color: #ffffff;
}

/* ── 键位区 ── */
.setting-column {
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
}

.text-connected {
  color: #4ade80;
}

.key-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.key-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 3px 8px;
  font-size: 12px;
  color: var(--app-text-strong);
}

.key-remove {
  background: transparent;
  border: none;
  color: var(--app-text-3);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  padding: 0;
}

.key-remove:hover {
  color: #ff7588;
}

.key-empty {
  color: var(--app-text-muted);
  font-size: 13px;
}

.key-capture {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 5px 12px;
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.key-capture:hover {
  border-color: #ff7588;
  color: #ff7588;
}

.key-capture.capturing {
  background: #ff7588;
  border-color: #ff7588;
  color: #fff;
  animation: capture-pulse 1s ease-in-out infinite;
}

@keyframes capture-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.6;
  }
}

.preset-row {
  margin-top: 4px;
}

.preset-btn {
  width: 100%;
  background: var(--app-surface-3);
  border: 1px dashed var(--app-border-3);
  color: var(--app-text-2);
  padding: 9px 16px;
  border-radius: 8px;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.preset-btn:hover {
  border-color: #ff7588;
  color: #ff7588;
}
</style>
