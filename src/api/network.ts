// src/api/network.ts
// 网络设置相关 API：基础代理（已有）+ Round48 高级代理设置（Clash 控制面联动）
import { http } from '@/utils/request'

/** 高级代理设置（GET /network/advanced） */
export interface AdvancedProxySetting {
  enabled: boolean
  /** 控制面地址 host:port（注意是 external-controller 端口，不是代理端口） */
  controller: string
  /** 是否已设置密钥（明文不回传） */
  hasSecret: boolean
  /** e-hentai.org 所走的代理组（必填） */
  groupEGallery: string
  /** exhentai.org 所走的代理组（必填） */
  groupEGalleryEx: string
  /** hath.network 所走的代理组（可空） */
  groupHath: string
}

/** 保存请求体（POST /network/advanced） */
export interface AdvancedProxyPayload {
  enabled: boolean
  controller: string
  secret: string
  /** 未修改密钥时置 true，后端沿用原值 */
  keepSecret: boolean
  groupEGallery: string
  groupEGalleryEx: string
  groupHath: string
}

/** 保存响应 */
export interface AdvancedProxySaveResult {
  message: string
  /** 非阻断性提示（如"该组为自动选择组"或"控制面暂时连不上"） */
  warning?: string
}

/** Clash 连通性测试结果 */
export interface ClashTestResult {
  ok: boolean
  message: string
  version?: string
  group?: string
  groupType?: string
  now?: string
  options?: number
  usable?: number
  bestDelayMs?: number
}

/** 控制面里的策略组（下拉候选） */
export interface ClashGroupItem {
  name: string
  type: string
  now: string
  options: number
}

/** 读取高级代理设置 */
export function fetchAdvancedProxy(): Promise<AdvancedProxySetting> {
  return http<AdvancedProxySetting>('/network/advanced')
}

/** 保存高级代理设置（服务端会校验组是否存在） */
export function saveAdvancedProxy(payload: AdvancedProxyPayload): Promise<AdvancedProxySaveResult> {
  return http<AdvancedProxySaveResult>('/network/advanced', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

/** 测试 Clash 控制面连通性（不传参数则用已保存配置） */
export function testClashConnection(payload?: {
  controller?: string
  secret?: string
}): Promise<ClashTestResult> {
  return http<ClashTestResult>('/network/clash/test', {
    method: 'POST',
    body: JSON.stringify(payload ?? {}),
  })
}

/** 枚举控制面里的策略组（供用户下拉选择，避免手填错名） */
export function fetchClashGroups(): Promise<{ groups: ClashGroupItem[] }> {
  return http<{ groups: ClashGroupItem[] }>('/network/clash/groups')
}
