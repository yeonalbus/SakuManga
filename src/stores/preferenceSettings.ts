/**
 * 偏好设置 Store
 *
 * 统一管理「设置中心 → 偏好」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - PreferenceSettings.vue 与各页面行为读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 界面语言 */
export type AppLanguage = 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP'

/** 启动时默认菜单 */
export type StartupMenu = 'hot' | 'home' | 'sub' | 'fav'

/** 隐藏快速回顶按钮策略 */
export type HideScrollToTopBtn = 'scrolling_down' | 'always' | 'never'

/** 搜索选项继承策略 */
export type SearchOptionsInherit = 'all' | 'category_only' | 'none'

/** 偏好设置项集合 */
export interface PreferenceSettings {
  language: AppLanguage // 界面语言
  defaultStartupMenu: StartupMenu // 启动时默认菜单
  hideScrollToTopBtn: HideScrollToTopBtn // 隐藏快速回顶按钮
  preloadGalleryCover: boolean // 预载画廊封面
  allowLeftSwipeBack: boolean // 允许通过左滑手势返回（需要重启）
  showAllGalleryTitles: boolean // 显示所有画廊标题（原标题+日文标题）
  showTagVoteStatus: boolean // 显示画廊标签投票状态
  showGalleryComments: boolean // 显示画廊评论
  showAllGalleryComments: boolean // 显示画廊所有评论
  useDefaultFavorite: boolean // 使用默认收藏夹
  useDefaultTagSet: boolean // 关注标签时使用默认标签集
  startInFullscreen: boolean // 以全屏模式启动
  searchOptionsInherit: SearchOptionsInherit // 搜索选项继承
  showR18GTagImages: boolean // 标签数据中直接显示 R18G 图片
  showTimeInUTC: boolean // 画廊时间使用 UTC 展示
  showDawnEvent: boolean // 展示黎明之事件
  showHVEvent: boolean // 展示 HV 遭遇战事件
  useBuiltinBlocklist: boolean // 使用内置用户屏蔽名单
}

const STORAGE_KEY = 'saku_preference_settings'

/** 默认值（与当前界面现状保持一致） */
const defaultSettings: PreferenceSettings = {
  language: 'zh-CN',
  defaultStartupMenu: 'hot',
  hideScrollToTopBtn: 'scrolling_down',
  preloadGalleryCover: true,
  allowLeftSwipeBack: true,
  showAllGalleryTitles: true,
  showTagVoteStatus: false,
  showGalleryComments: true,
  showAllGalleryComments: false,
  useDefaultFavorite: true,
  useDefaultTagSet: true,
  startInFullscreen: false,
  searchOptionsInherit: 'all',
  showR18GTagImages: true,
  showTimeInUTC: true,
  showDawnEvent: true,
  showHVEvent: true,
  useBuiltinBlocklist: true,
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
