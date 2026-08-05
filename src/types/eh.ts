/** E 站可选项站点类型 */
export type EHSite = 'e-hentai' | 'exhentai'

/** 默认原图分辨率选项 */
export type EHResolution = 'auto' | '780' | '980' | '1280' | '1600' | '2400'

/** EHSetting 持久化设置契约（当前生效快照） */
export interface EHSetting {
  id?: number
  site: EHSite
  preferRedirect: boolean
  selectedProfile?: string
  selectedProfileName?: string
  updatedAt?: string
}

/** 站点设置 Profile（兼容保留，站点配置请使用 uconfig 接口） */
export interface EHProfile {
  id: number
  name: string
  isDefault?: boolean
  site: EHSite
  preferRedirect: boolean
  rowsPerPage: number
  topListSize: number
  resolution: EHResolution
  createdAt?: string
  updatedAt?: string
}

/** 动态配额与资产信息契约（资产含 GP / Credits / Hath） */
export interface EHUserStatus {
  currentQuota: number
  maxQuota: number
  assetGP: string
  assetCredits: string
  assetHath: string
}

/** 一个 Tagset（E 站 mytags 页顶部下拉的一个选项） */
export interface EHTagset {
  id: number
  name: string
  count: number
}

/** 我的标签（关注 / 隐藏），从 E 站 mytags 页读取 */
export interface EHMyTags {
  watched: string[]
  hidden: string[]
  tagsets: EHTagset[]
  currentTagset: number
}

/** /tags/suggest 联想返回的标签条目 */
export interface TagSuggestion {
  namespace: string
  key: string
  name: string
  intro?: string
  count: number
}

// ============================================================
// uconfig.php 结构化配置类型（后端解析，前端动态渲染）
// ============================================================

/** 单选 / 下拉的可选值 */
export interface UConfigOption {
  value: string
  label: string
  checked?: boolean
  disabled?: boolean
}

/** 分类开关（ct_* hidden + 相邻 label） */
export interface UConfigCategory {
  name: string
  label: string
  checked: boolean
}

/** 语言表格中的一格 */
export interface UConfigCell {
  name: string
  checked: boolean
}

/** 语言表格的一行（一种语言） */
export interface UConfigTableRow {
  label: string
  cells: UConfigCell[]
}

/** 语言表格（Original/Translated/Rewrite/All） */
export interface UConfigTable {
  columns: string[]
  rows: UConfigTableRow[]
}

/** 字段类型 */
export type UConfigFieldType =
  'radio' | 'select' | 'checkbox' | 'text' | 'textarea' | 'category' | 'language-table'

/** 一个结构化字段 */
export interface UConfigField {
  name: string
  type: UConfigFieldType
  label?: string
  hint?: string
  description?: string
  value?: string
  suffix?: string
  placeholder?: string
  maxLength?: number
  checked?: boolean
  options?: UConfigOption[]
  categories?: UConfigCategory[]
  table?: UConfigTable
}

/** 配置分组（对应页面中的 h2 标题） */
export interface UConfigSection {
  title: string
  fields: UConfigField[]
}

/** profile 下拉选项 */
export interface UConfigProfile {
  value: string
  label: string
}

/** uconfig.php 的完整结构化描述 */
export interface UConfigData {
  profiles: UConfigProfile[]
  selectedProfile: string
  sections: UConfigSection[]
}
