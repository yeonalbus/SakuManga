/**
 * 样式设置 Store
 *
 * 统一管理「设置中心 → 样式」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - StyleSettings.vue 与各页面布局读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 *
 * 布局模式按设备分类记忆：
 * - layoutModeByDevice 为 mobile / tablet / desktop 三组槽位，各自记忆布局模式
 * - 即便 iCloud 等把 localStorage 同步到多台设备，手机读手机槽位、iPad 读平板槽位，
 *   互不覆盖
 * - layoutMode 字段保留为「当前设备生效模式的镜像」，兼容旧代码 / 旧存储数据
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'
import { detectDeviceClass, type DeviceClass } from '@/utils/device'

/** 主题模式 */
export type ThemeMode = 'system' | 'dark' | 'light'

/** 布局模式：自动（跟随视口自适应）/ 桌面（强制桌面形态）/ 移动（强制移动形态） */
export type LayoutMode = 'auto' | 'desktop' | 'mobile'

/** 自动适配详情面板列数的取值范围（1~5） */
export const MIN_PANEL_COLS = 1
export const MAX_PANEL_COLS = 5

const clampPanelCols = (n: number): number => {
  if (!Number.isFinite(n)) return 5
  return Math.min(MAX_PANEL_COLS, Math.max(MIN_PANEL_COLS, Math.round(n)))
}

/** 样式设置项集合 */
export interface StyleSettings {
  // ── 主题 ──
  themeMode: ThemeMode // 主题模式：跟随系统 / 暗黑 / 亮色

  // ── 布局 ──
  layoutMode: LayoutMode // 当前设备生效的布局模式（镜像，兼容旧代码）
  layoutModeByDevice: Record<DeviceClass, LayoutMode> // 按设备分类记忆的布局模式

  // ── 自动适配详情面板列数（仅宽屏桌面在线列表生效）──
  autoPanelColumns: boolean // 开关：开启后按详情面板开/关注入列数
  cardPanelClosedCols: number // 卡片模式 · 面板收起列数（默认 5）
  cardPanelOpenCols: number // 卡片模式 · 面板展开列数（默认 3）
  compactPanelClosedCols: number // 名片模式 · 面板收起列数（默认 3）
  compactPanelOpenCols: number // 名片模式 · 面板展开列数（默认 2）
}

const STORAGE_KEY = 'saku_style_settings'

/** 默认值（与当前深色界面现状保持一致） */
const defaultSettings: StyleSettings = {
  themeMode: 'system',
  layoutMode: 'auto',
  layoutModeByDevice: {
    mobile: 'auto',
    tablet: 'auto',
    desktop: 'auto',
  },
  autoPanelColumns: true,
  cardPanelClosedCols: 5,
  cardPanelOpenCols: 3,
  compactPanelClosedCols: 3,
  compactPanelOpenCols: 2,
}

/** 当前设备类别（应用加载时检测一次，运行期不变） */
export const currentDeviceClass: DeviceClass = detectDeviceClass()

/**
 * 旧数据迁移：老版本只有 layoutMode 单值、无 layoutModeByDevice，
 * 把旧值展开到全部设备槽位，保证升级后各设备行为一致。
 */
function migrateLegacy(data: Partial<StyleSettings>): Partial<StyleSettings> {
  if (!data.layoutModeByDevice && data.layoutMode) {
    return {
      ...data,
      layoutModeByDevice: {
        mobile: data.layoutMode,
        tablet: data.layoutMode,
        desktop: data.layoutMode,
      },
    }
  }
  return data
}

const loaded = migrateLegacy(loadStorage<Partial<StyleSettings>>(STORAGE_KEY, {}))

/** 响应式样式设置（自动持久化） */
export const styleSettings = reactive<StyleSettings>({
  ...defaultSettings,
  ...loaded,
  // 新字段：旧数据缺失时回落默认值；越界（<1 或 >5）时钳制
  autoPanelColumns: loaded.autoPanelColumns ?? defaultSettings.autoPanelColumns,
  cardPanelClosedCols: clampPanelCols(
    loaded.cardPanelClosedCols ?? defaultSettings.cardPanelClosedCols,
  ),
  cardPanelOpenCols: clampPanelCols(loaded.cardPanelOpenCols ?? defaultSettings.cardPanelOpenCols),
  compactPanelClosedCols: clampPanelCols(
    loaded.compactPanelClosedCols ?? defaultSettings.compactPanelClosedCols,
  ),
  compactPanelOpenCols: clampPanelCols(
    loaded.compactPanelOpenCols ?? defaultSettings.compactPanelOpenCols,
  ),
  layoutMode: loaded.layoutModeByDevice?.[currentDeviceClass] ?? defaultSettings.layoutMode,
  // 深拷贝 layoutModeByDevice，避免写操作污染 defaultSettings
  layoutModeByDevice: {
    ...defaultSettings.layoutModeByDevice,
    ...loaded.layoutModeByDevice,
  },
})

watch(
  styleSettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

/** 当前设备生效的布局模式 */
export function getEffectiveMode(): LayoutMode {
  return styleSettings.layoutModeByDevice[currentDeviceClass]
}

/** 设置当前设备的布局模式（同时镜像到 layoutMode，兼容旧代码读取） */
export function setLayoutMode(mode: LayoutMode): void {
  styleSettings.layoutModeByDevice[currentDeviceClass] = mode
  styleSettings.layoutMode = mode
}

/** 恢复默认样式设置 */
export function resetStyleSettings(): void {
  Object.assign(styleSettings, {
    ...defaultSettings,
    layoutModeByDevice: { ...defaultSettings.layoutModeByDevice },
  })
}
