/** E 站可选项站点类型 */
export type EHSite = 'e-hentai' | 'exhentai'

/** EHSetting 持久化设置契约 */
export interface EHSetting {
  site: EHSite
  preferRedirect: boolean
  selectedProfile?: string
}

/** 动态配额与资产信息契约 */
export interface EHUserStatus {
  currentQuota: number
  maxQuota: number
  assetGP: string
  assetCredits: string
}
