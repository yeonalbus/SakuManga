/**
 * 网络设置 Store
 *
 * 统一管理「设置中心 → 网络」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - NetworkSettings.vue 与请求层读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 *
 * 注意：代理服务器地址走后端 API（/network/proxy），不在此 Store 内。
 *
 * ⚠️ 1.0 收敛：仅保留真正接线生效的最小可用集。
 *   - 删除：域名前置 / 页面缓存时间 / 图片缓存时间（纯摆设，无对应实现）
 *   - 超时：fetch 的 AbortSignal 无法区分「连接超时」与「接收超时」，
 *     故将原 connectTimeout / receiveTimeout 合并为单一请求超时 requestTimeout。
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 网络设置项集合 */
export interface NetworkSettings {
  requestTimeout: number // 单次请求超时时间 (ms)
}

const STORAGE_KEY = 'saku_network_settings'

/** 默认值（与当前请求层现状保持一致：默认 60s） */
const defaultSettings: NetworkSettings = {
  requestTimeout: 60000,
}

/** 响应式网络设置（自动持久化） */
export const networkSettings = reactive<NetworkSettings>({
  ...defaultSettings,
  ...loadStorage<Partial<NetworkSettings>>(STORAGE_KEY, {}),
})

watch(
  networkSettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

/** 恢复默认网络设置 */
export function resetNetworkSettings(): void {
  Object.assign(networkSettings, defaultSettings)
}
