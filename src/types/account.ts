// ==========================================
// E 站账号与偏好配置契约
// ==========================================

/** E 站可选项站点类型 */
export type EHSite = 'e-hentai' | 'exhentai'

/** 完整 Cookie 结构（用于前端表单校验与状态持久化） */
export interface EHCookies {
  ipb_member_id: string
  ipb_pass_hash: string
  igneous?: string
  sk?: string
}

/** EHSetting 持久化设置契约 */
export interface EHSetting {
  site: EHSite
  preferRedirect: boolean
  selectedProfile?: string
  isEx?: boolean // 当前账号权限标识
  cookies?: EHCookies // Cookie 明细（可选，用于前端管理弹窗）
}

/** 动态配额与资产信息契约 */
export interface EHUserStatus {
  currentQuota: number
  maxQuota: number
  assetGP: string
  assetCredits: string
}

/** E 站收藏夹分类 (Fav 0 ~ Fav 9) */
export interface EHFavoriteCategory {
  index: number // 0-9
  name: string // 分类名称
  count?: number // 该分类下的画廊数量
}

// ==========================================
// 通用用户偏好与系统设置契约
// ==========================================

export type ThemeMode = 'light' | 'dark' | 'system'

export interface GeneralUserSettings {
  theme: ThemeMode
  cardViewMode: 'card' | 'compact'
  imgResolution: 'auto' | '1280' | '1600' | '2400'
  enableImgProxy: boolean
  imgProxyServer?: string
  autoMarkReadOnEnd: boolean
}
