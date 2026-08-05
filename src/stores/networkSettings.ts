/**
 * 网络设置 Store
 *
 * 统一管理「设置中心 → 网络」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - NetworkSettings.vue 与请求层读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 *
 * 注意：代理服务器地址走后端 API（/network/proxy），不在此 Store 内。
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 网络设置项集合 */
export interface NetworkSettings {
  domainFronting: boolean // 开启域名前置（绕过 SNI 封锁）
  pageCacheTime: string // 页面缓存时间：1d/3d/7d/30d
  imageCacheTime: string // 图片缓存时间：7d/15d/30d/90d
  connectTimeout: number // 建立连接超时时间 (ms)
  receiveTimeout: number // 接收数据超时时间 (ms)
}

const STORAGE_KEY = 'saku_network_settings'

/** 默认值（与当前网络层现状保持一致） */
const defaultSettings: NetworkSettings = {
  domainFronting: false,
  pageCacheTime: '3d',
  imageCacheTime: '30d',
  connectTimeout: 6000,
  receiveTimeout: 6000,
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
