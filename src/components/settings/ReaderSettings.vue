<template>
  <div class="reader-settings">
    <!-- ── 阅读方向 / 页面布局 ── -->
    <div class="section-title">📐 布局</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">阅读方向</div>
      </div>
      <select v-model="readerSettings.readDirection" class="setting-select">
        <option value="rtl_double">从右至左(双列)</option>
        <option value="rtl_single">从右至左(单列)</option>
        <option value="ltr_double">从左至右(双列)</option>
        <option value="ltr_single">从左至右(单列)</option>
        <option value="webtoon">连续滚动(Webtoon)</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">页面缩放</div>
        <div class="item-subtext">匹配屏幕 / 覆盖屏幕 / 适应宽度</div>
      </div>
      <select v-model="readerSettings.pageFit" class="setting-select">
        <option value="contain">匹配屏幕</option>
        <option value="cover">覆盖屏幕</option>
        <option value="width">适应宽度</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">图片间隔</div>
      </div>
      <select v-model="readerSettings.imageGap" class="setting-select">
        <option :value="0">0</option>
        <option :value="5">5</option>
        <option :value="10">10</option>
        <option :value="15">15</option>
        <option :value="20">20</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">双页首页模式</div>
        <div class="item-subtext">双列阅读时的首页排版</div>
      </div>
      <select v-model="readerSettings.singleCover" class="setting-select">
        <option :value="true">双页带首页（P1 单独一屏）</option>
        <option :value="false">双页不带首页（P1+P2 并排）</option>
      </select>
    </div>

    <!-- ── 翻页行为 ── -->
    <div class="section-title">📖 翻页行为</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">自动翻页(秒)</div>
        <div class="item-subtext">0 表示关闭</div>
      </div>
      <div class="input-inline">
        <input
          v-model.number="readerSettings.autoTurnInterval"
          type="number"
          class="setting-input wide"
          min="0"
          max="120"
        />
        <span class="unit">秒</span>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启翻页动画</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.enableTurnAnimation" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">反转翻页方向</div>
        <div class="item-subtext">左右点击/按键方向互换</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.reverseTurnDirection" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">禁用点击翻页手势</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.disableTapTurnGesture" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- ── 界面显隐 ── -->
    <div class="section-title">🖥️ 界面显隐</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启沉浸模式</div>
        <div class="item-subtext">进入阅读器时隐藏顶部标题栏</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.immersiveMode" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启底部菜单</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.enableBottomMenu" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">底部显示状态信息</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.showBottomStatus" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示滚动条</div>
        <div class="item-subtext">底部页码进度条</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.showScrollbar" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示缩略图</div>
        <div class="item-subtext">阅读器底部缩略图进度条（点击底部区域切换显隐）</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.showThumbnails" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示时钟</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.showClock" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示进度</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.showProgress" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示电量</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.showBattery" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- ── 设备能力 ── -->
    <div class="section-title">🔋 设备能力</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">阅读时屏幕不自动锁定</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.keepAwake" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">自定义屏幕亮度</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.customBrightness" />
        <span class="slider"></span>
      </label>
    </div>

    <div v-if="readerSettings.customBrightness" class="setting-item">
      <div class="item-info">
        <div class="item-title">屏幕亮度</div>
        <div class="item-subtext">{{ readerSettings.brightnessValue }}%</div>
      </div>
      <input
        v-model.number="readerSettings.brightnessValue"
        type="range"
        min="20"
        max="100"
        class="setting-range"
      />
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">允许双击放大图片</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.allowDoubleTapZoom" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">允许单击后拖拽放大图片</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.allowSingleClickDragZoom" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- ── 游戏手柄 ── -->
    <div class="section-title">🎮 游戏手柄</div>

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

    <!-- ── 性能 / 扩展 ── -->
    <div class="section-title">⚡ 性能 / 扩展</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">预加载图片数量(在线模式)</div>
      </div>
      <select v-model="readerSettings.preloadOnline" class="setting-select">
        <option :value="5">5</option>
        <option :value="10">10</option>
        <option :value="15">15</option>
        <option :value="20">20</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">预加载图片数量(本地模式)</div>
      </div>
      <select v-model="readerSettings.preloadOffline" class="setting-select">
        <option :value="5">5</option>
        <option :value="10">10</option>
        <option :value="15">15</option>
        <option :value="20">20</option>
      </select>
    </div>

    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useUI } from '@/composables/useUI'
import { readerSettings, resetReaderSettings, GAMEPAD_BUTTONS } from '@/stores/readerSettings'

const { toast } = useUI()

// 恢复默认阅读设置
const handleReset = () => {
  resetReaderSettings()
  toast.success('已恢复默认阅读设置')
}

// ── 游戏手柄：连接状态检测 ──
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

// ── 游戏手柄：键位映射（8BitDo Micro 预设 + 自定义录制） ──
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
.reader-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
  color: #ff7588;
  letter-spacing: 0.5px;
  margin: 12px 0 4px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--app-border-2);
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

.setting-item.clickable {
  cursor: pointer;
}

.setting-item.clickable:hover {
  background-color: var(--app-surface-2-hover);
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

.arrow-icon {
  font-size: 20px;
  color: var(--app-text-muted);
  margin-left: 8px;
}

/* 统一的原生下拉菜单 */
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

/* 内联数值输入框 */
.input-inline {
  display: flex;
  align-items: center;
  gap: 6px;
}

.setting-input {
  background: transparent;
  border: none;
  border-bottom: 1px solid var(--app-border-3);
  color: var(--app-text-strong);
  font-size: 14px;
  width: 40px;
  text-align: center;
  outline: none;
}

.setting-input.wide {
  width: 60px;
}

.setting-input:focus {
  border-bottom-color: #ff7588;
}

.setting-range {
  accent-color: #ff7588;
  width: 140px;
  cursor: pointer;
}

.unit {
  font-size: 13px;
  color: var(--app-text-3);
}

.check-mark {
  color: var(--app-text-2);
  font-size: 14px;
  margin-left: 4px;
}

/* 原生 Switch 开关 */
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
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

/* 恢复默认按钮 */
.reset-row {
  margin-top: 8px;
}

.reset-btn {
  width: 100%;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 10px 16px;
  border-radius: 8px;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.reset-btn:hover {
  background-color: var(--app-surface-3-hover);
  border-color: #ff7588;
  color: #ff7588;
}

/* ── 游戏手柄设置 ── */
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
