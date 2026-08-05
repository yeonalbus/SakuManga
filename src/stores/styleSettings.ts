/**
 * 样式设置 Store
 *
 * 统一管理「设置中心 → 样式」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - StyleSettings.vue 与各页面布局读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 主题模式 */
export type ThemeMode = 'system' | 'dark' | 'light'

/** 画廊列表展示样式 */
export type GalleryListStyle = 'card' | 'compact'

/** 布局模式 */
export type LayoutMode = 'desktop' | 'mobile'

/** 样式设置项集合 */
export interface StyleSettings {
  // ── 主题 ──
  themeMode: ThemeMode // 主题模式：跟随系统 / 暗黑 / 亮色

  // ── 列表样式 ──
  globalGalleryStyle: GalleryListStyle // 画廊列表样式（全局）
  downloadGroupCols: string // 下载页网格布局列数（分组）：auto/2/3/4
  downloadGalleryCols: string // 下载页网格布局列数（画廊）：auto/3/4/5
  detailThumbCols: string // 详情页缩略图列数：auto/4/6/8

  // ── 布局 ──
  moveCoverToRight: boolean // 移动封面图至右侧（需要重启）
  layoutMode: LayoutMode // 布局模式：桌面 / 移动
}

const STORAGE_KEY = 'saku_style_settings'

/** 默认值（与当前深色界面现状保持一致） */
const defaultSettings: StyleSettings = {
  themeMode: 'system',
  globalGalleryStyle: 'card',
  downloadGroupCols: 'auto',
  downloadGalleryCols: 'auto',
  detailThumbCols: 'auto',
  moveCoverToRight: false,
  layoutMode: 'desktop',
}

/** 响应式样式设置（自动持久化） */
export const styleSettings = reactive<StyleSettings>({
  ...defaultSettings,
  ...loadStorage<Partial<StyleSettings>>(STORAGE_KEY, {}),
})

watch(
  styleSettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

/** 恢复默认样式设置 */
export function resetStyleSettings(): void {
  Object.assign(styleSettings, defaultSettings)
}
