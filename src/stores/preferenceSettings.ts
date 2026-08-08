/**
 * 偏好设置 Store
 *
 * 统一管理「设置中心 → 偏好」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - PreferenceSettings.vue 与各页面行为读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 *
 * ⚠️ 1.0 收敛：仅保留真正接线生效的最小可用集，
 *    其余 E-Hentai 客户端专有/无意义项已移除。
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 启动时默认菜单 */
export type StartupMenu = 'hot' | 'home' | 'sub' | 'fav'

/** 隐藏快速回顶按钮策略 */
export type HideScrollToTopBtn = 'scrolling_down' | 'always' | 'never'

/** 搜索选项继承策略 */
export type SearchOptionsInherit = 'all' | 'category_only' | 'none'

/** 偏好设置项集合 */
export interface PreferenceSettings {
  defaultStartupMenu: StartupMenu // 启动时默认菜单
  hideScrollToTopBtn: HideScrollToTopBtn // 隐藏快速回顶按钮
  showGalleryComments: boolean // 显示画廊评论
  startInFullscreen: boolean // 以全屏模式启动
  searchOptionsInherit: SearchOptionsInherit // 搜索选项继承
  preferLocalGallery: boolean // S1 本地优先加载：在线画廊有本地副本时详情页优先读本地
  resumeFromLastPage: boolean // 非历史入口「立即阅读」是否从上次阅读位置开始（默认关）
}

const STORAGE_KEY = 'saku_preference_settings'

/** 默认值（与当前界面现状保持一致） */
const defaultSettings: PreferenceSettings = {
  defaultStartupMenu: 'hot',
  hideScrollToTopBtn: 'scrolling_down',
  showGalleryComments: true,
  startInFullscreen: false,
  searchOptionsInherit: 'all',
  preferLocalGallery: true,
  resumeFromLastPage: false,
}

/** 响应式偏好设置（自动持久化） */
export const preferenceSettings = reactive<PreferenceSettings>({
  ...defaultSettings,
  ...loadStorage<Partial<PreferenceSettings>>(STORAGE_KEY, {}),
})

watch(
  preferenceSettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

/** 恢复默认偏好设置 */
export function resetPreferenceSettings(): void {
  Object.assign(preferenceSettings, defaultSettings)
}
