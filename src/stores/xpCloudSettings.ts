/**
 * XP 词云展示设置（Round32 视觉改版）
 *
 * 词云的排版与配色参数，持久化到 localStorage，用户可在词云面板的「⚙️ 词云参数」里自行调节。
 * 默认值与 plans/round32-xp-cloud-recommend-plan.md 第十一章（视觉改版）一致，
 * 也与验收样板 Test/pw-bug/xp-mock/wordcloud-mock.html 的参数保持一致。
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

export interface XpCloudSettings {
  /** 最多展示词条数（放不下时自动少放，以留白换观感） */
  wordCount: number
  /** 字号下限（px） */
  fontMin: number
  /** 字号上限（px） */
  fontMax: number
  /** 碰撞盒高度系数：越大词间距越松（1.2=旧版密排，1.45=有呼吸感） */
  lineRatio: number
  /** 碰撞盒左右额外留白（相对字号） */
  padX: number
  /** 短名截断字数（英文按词边界断开） */
  truncChars: number
  /** 超过该字数的词条不参与词云（长横条是词云排版的头号破坏者） */
  maxChars: number
  /** 仅显示含中日韩译名的词条（过滤无译名的拉丁转写长名） */
  onlyTranslated: boolean
}

const STORAGE_KEY = 'app_xp_cloud_settings'

export const XP_CLOUD_DEFAULTS: XpCloudSettings = {
  wordCount: 100,
  fontMin: 13,
  fontMax: 42,
  lineRatio: 1.45,
  padX: 0.3,
  truncChars: 14,
  maxChars: 20,
  onlyTranslated: true,
}

export const xpCloudSettings = reactive<XpCloudSettings>({
  ...XP_CLOUD_DEFAULTS,
  ...loadStorage<Partial<XpCloudSettings>>(STORAGE_KEY, {}),
})

watch(
  () => ({ ...xpCloudSettings }),
  (val) => saveStorage(STORAGE_KEY, val),
  { deep: true },
)

/** 恢复默认参数 */
export const resetXpCloudSettings = () => {
  Object.assign(xpCloudSettings, XP_CLOUD_DEFAULTS)
}

/** 参数是否偏离默认（面板角标用） */
export const xpCloudDirtyCount = (): number => {
  let n = 0
  for (const key of Object.keys(XP_CLOUD_DEFAULTS) as (keyof XpCloudSettings)[]) {
    if (xpCloudSettings[key] !== XP_CLOUD_DEFAULTS[key]) n++
  }
  return n
}
