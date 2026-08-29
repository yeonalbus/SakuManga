/**
 * 阅读器设置 Store
 *
 * 统一管理「设置中心 → 阅读」与「阅读器内部抽屉」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - ReaderSettings.vue 与 ComicReader.vue 读写同一份数据，实现即时联动
 * 由原 ReaderSettings 组件内 ref 抽取而来
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 阅读方向 */
export type ReadDirection =
  | 'rtl_double' // 从右至左(双列)
  | 'rtl_single' // 从右至左(单列)
  | 'ltr_double' // 从左至右(双列)
  | 'ltr_single' // 从左至右(单列)
  | 'webtoon' // 连续滚动(Webtoon)

/** 页面缩放方式 */
export type PageFit = 'contain' | 'cover' | 'width'

/**
 * 图片缩放手势（Round25：由 allowDoubleTapZoom / allowSingleClickDragZoom 两开关合并）
 * - doubleTap     仅双击放大
 * - doubleTapDrag 双击 + 单击拖拽放大
 * - off           关闭全部缩放手势
 */
export type ZoomGesture = 'doubleTap' | 'doubleTapDrag' | 'off'

/** 阅读器设置项集合 */
/**
 * Gamepad 标准映射按键索引（standard mapping）
 * 8BitDo Micro：A/B + D-Pad + Start/Select，无摇杆/肩键。
 * 0:A  1:B  2:X  3:Y  4:LB  5:RB  6:LT  7:RT
 * 8:Select  9:Start  10:L3  11:R3
 * 12:D-Pad上  13:D-Pad下  14:D-Pad左  15:D-Pad右
 */
export const GAMEPAD_BUTTONS = {
  A: 0,
  B: 1,
  X: 2,
  Y: 3,
  LB: 4,
  RB: 5,
  LT: 6,
  RT: 7,
  SELECT: 8,
  START: 9,
  L3: 10,
  R3: 11,
  DPAD_UP: 12,
  DPAD_DOWN: 13,
  DPAD_LEFT: 14,
  DPAD_RIGHT: 15,
} as const

export interface ReaderSettings {
  // ── 阅读方向 / 页面布局 ──
  readDirection: ReadDirection
  pageFit: PageFit
  singleCover: boolean // 单独展示首页(全局)：双页模式下第一页单独占一屏
  imageGap: number // 图片间隔 (px)

  // ── 翻页行为 ──
  autoTurnInterval: number // 自动翻页(秒)，0 为关闭
  reverseTurnDirection: boolean // 反转翻页方向
  disableTapTurnGesture: boolean // 禁用点击翻页手势
  enableTurnAnimation: boolean // 开启翻页动画

  // ── 界面显隐 ──
  immersiveMode: boolean // 开启沉浸模式：进入阅读器时隐藏顶部标题栏
  showBottomBar: boolean // 显示底部栏（滚动条 + 状态信息，Round24 合并）
  enableBottomMenu: boolean // 开启底部菜单

  // ── 设备能力 ──
  keepAwake: boolean // 阅读时屏幕不自动锁定 (Wake Lock)
  customBrightness: boolean // 自定义屏幕亮度
  brightnessValue: number // 屏幕亮度 (20-100)
  zoomGesture: ZoomGesture // 图片缩放手势（Round25：双击/单击拖拽两开关合并）

  // ── 游戏手柄 ──
  enableGamepad: boolean // 启用手柄控制
  gamepadNextKeys: number[] // 下一页按键索引（可多个：D-Pad右 / A）
  gamepadPrevKeys: number[] // 上一页按键索引（可多个：D-Pad左 / B）
  gamepadToggleKeys: number[] // 切换设置菜单按键索引（Start / Select）
  // Round14：双击快速确认切本 + modal 手柄确认（可自定义，防与翻页键冲突）
  gamepadDoubleTapConfirm: boolean // 双击「下一页/上一页」快速确认切换漫画
  gamepadDoubleTapWindow: number // 双击时间窗（毫秒，默认 500）
  gamepadConfirmKeys: number[] // 确认按键（modal 确认 / 双击切本，默认 [A]）
  gamepadCancelKeys: number[] // 取消按键（modal 取消，默认 [B]）

  // ── 性能 / 扩展 ──
  preloadCount: number // 预加载图片数量（Round25：在线/本地两值合并为统一值）
}

const STORAGE_KEY = 'saku_reader_settings'

/** 默认值（与阅读器现状保持一致） */
const defaultSettings: ReaderSettings = {
  readDirection: 'rtl_double',
  pageFit: 'contain',
  singleCover: true,
  imageGap: 10,

  autoTurnInterval: 0,
  reverseTurnDirection: false,
  disableTapTurnGesture: false,
  enableTurnAnimation: true,

  immersiveMode: false,
  showBottomBar: true,
  enableBottomMenu: false,

  keepAwake: false,
  customBrightness: false,
  brightnessValue: 100,
  zoomGesture: 'doubleTap',

  enableGamepad: true,
  gamepadNextKeys: [GAMEPAD_BUTTONS.DPAD_RIGHT, GAMEPAD_BUTTONS.A],
  gamepadPrevKeys: [GAMEPAD_BUTTONS.DPAD_LEFT, GAMEPAD_BUTTONS.B],
  gamepadToggleKeys: [GAMEPAD_BUTTONS.START, GAMEPAD_BUTTONS.SELECT],
  gamepadDoubleTapConfirm: true,
  gamepadDoubleTapWindow: 500,
  gamepadConfirmKeys: [GAMEPAD_BUTTONS.A],
  gamepadCancelKeys: [GAMEPAD_BUTTONS.B],

  preloadCount: 10,
}

// Round24：清理旧版残留的失效设置项（时钟/电量组件已移除、缩略图/进度/滚动条/状态项已合并，旧键一并剔除）
const storedSettings = loadStorage<Partial<ReaderSettings> & {
  headerTextWidth?: string
  allowDoubleTapZoom?: boolean
  allowSingleClickDragZoom?: boolean
  preloadOnline?: number
  preloadOffline?: number
}>(STORAGE_KEY, {})

// Round25 迁移：字段合并（预加载在线/本地 → 统一值；双击/单击拖拽两开关 → 缩放手势枚举）
if (storedSettings.preloadCount === undefined) {
  storedSettings.preloadCount =
    storedSettings.preloadOnline ?? storedSettings.preloadOffline ?? defaultSettings.preloadCount
}
if (storedSettings.zoomGesture === undefined) {
  if (storedSettings.allowDoubleTapZoom) {
    storedSettings.zoomGesture = storedSettings.allowSingleClickDragZoom ? 'doubleTapDrag' : 'doubleTap'
  } else {
    storedSettings.zoomGesture = 'off'
  }
}

for (const stale of [
  'showClock',
  'showBattery',
  'showThumbnails',
  'showScrollbar',
  'showBottomStatus',
  'showProgress',
  // Round25：已被新字段取代/废弃的旧键
  'headerTextWidth',
  'allowDoubleTapZoom',
  'allowSingleClickDragZoom',
  'preloadOnline',
  'preloadOffline',
] as const) {
  delete (storedSettings as Record<string, unknown>)[stale]
}

/** 响应式阅读器设置（自动持久化） */
export const readerSettings = reactive<ReaderSettings>({
  ...defaultSettings,
  ...storedSettings,
})

watch(
  readerSettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

/** 由「阅读方向」解析出 RTL / 双页 / webtoon 三个布局开关 */
export function parseReadDirection(dir: ReadDirection): {
  isRTL: boolean
  isDoublePage: boolean
  isWebtoon: boolean
} {
  switch (dir) {
    case 'rtl_double':
      return { isRTL: true, isDoublePage: true, isWebtoon: false }
    case 'rtl_single':
      return { isRTL: true, isDoublePage: false, isWebtoon: false }
    case 'ltr_double':
      return { isRTL: false, isDoublePage: true, isWebtoon: false }
    case 'ltr_single':
      return { isRTL: false, isDoublePage: false, isWebtoon: false }
    case 'webtoon':
      return { isRTL: false, isDoublePage: false, isWebtoon: true }
  }
}

/** 恢复默认阅读器设置 */
export function resetReaderSettings(): void {
  Object.assign(readerSettings, defaultSettings)
}
