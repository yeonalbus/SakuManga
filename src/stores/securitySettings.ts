/**
 * 安全设置 Store
 *
 * 统一管理「设置中心 → 安全」的配置：
 * - 全部配置持久化到 localStorage，跨页面共享
 * - SecuritySettings.vue 与鉴权逻辑读写同一份数据
 * 参照 readerSettings.ts 持久化模式
 */
import { reactive, watch } from 'vue'
import { loadStorage, saveStorage } from '@/utils/storage'

/** 安全设置项集合 */
export interface SecuritySettings {
  enablePasswordAuth: boolean // 开启密码认证
  enableBiometricAuth: boolean // 开启生物认证
}

const STORAGE_KEY = 'saku_security_settings'

/** 默认值 */
const defaultSettings: SecuritySettings = {
  enablePasswordAuth: false,
  enableBiometricAuth: false,
}

/** 响应式安全设置（自动持久化） */
export const securitySettings = reactive<SecuritySettings>({
  ...defaultSettings,
  ...loadStorage<Partial<SecuritySettings>>(STORAGE_KEY, {}),
})

watch(
  securitySettings,
  (val) => {
    saveStorage(STORAGE_KEY, val)
  },
  { deep: true },
)

/** 恢复默认安全设置 */
export function resetSecuritySettings(): void {
  Object.assign(securitySettings, defaultSettings)
}
