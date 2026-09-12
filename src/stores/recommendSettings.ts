/**
 * 偏好推荐设置 Store（Round32 阶段二）
 *
 * 「随机抽卡 → ✨ 偏好推荐」的采样参数，持久化到 localStorage 跨会话复用。
 * 参数语义见 plans/round32-xp-cloud-recommend-plan.md（决策 D2：参数暴露给用户）。
 *
 * 注意：缺省值必须与后端 services/recommend.go 的默认常量保持一致，
 * 前端只在用户显式调整后才下发参数，未调整时由后端取默认。
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 推荐设置项集合 */
export interface RecommendSettings {
  /** 偏好侧重 θ：0=纯库藏（搜集广度），1=纯阅读（真实消耗） */
  theta: number
  /** 探索率 ε：以该概率走纯随机，防止 XP 固化 */
  explore: number
  /** 采样温度 T：越大分布越平缓（推荐越"散"） */
  temperature: number
  /** 排除读过的（read_count > 0） */
  excludeRead: boolean
  /** 排除已在书架的本子 */
  excludeShelf: boolean
  /** 记住上次使用的卡池模式（下次进入抽卡页直接沿用） */
  rememberMode: boolean
  /** 上次使用的卡池模式 */
  lastMode: 'random' | 'recommend'
}

const STORAGE_KEY = 'app_recommend_settings'

/** 与后端默认值对齐 */
export const RECO_DEFAULTS: Omit<RecommendSettings, 'rememberMode' | 'lastMode'> = {
  theta: 0.6,
  explore: 0.15,
  temperature: 0.5,
  excludeRead: false,
  excludeShelf: false,
}

export const recommendSettings = reactive<RecommendSettings>({
  ...RECO_DEFAULTS,
  rememberMode: true,
  lastMode: 'random',
  ...loadStorage<Partial<RecommendSettings>>(STORAGE_KEY, {}),
})

watch(
  () => ({ ...recommendSettings }),
  (val) => saveStorage(STORAGE_KEY, val),
  { deep: true },
)

/** 恢复默认参数（不影响记忆的卡池模式） */
export const resetRecommendSettings = () => {
  Object.assign(recommendSettings, RECO_DEFAULTS)
}
