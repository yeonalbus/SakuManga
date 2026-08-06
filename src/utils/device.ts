/**
 * 设备类别识别工具
 *
 * 用于「按设备分类记忆」布局模式（mobile / tablet / desktop 三组槽位），
 * 避免手机与 iPad / 桌面共享同一份布局模式设置，互相覆盖。
 *
 * 识别要点：
 * - iPadOS 13+ 的 Safari/Chrome 将 UA 伪装成 Macintosh，需用
 *   navigator.maxTouchPoints > 1 兜底识别为平板
 * - Android 平板 UA 通常不含 "Mobile" / "Mobi"
 */
export type DeviceClass = 'mobile' | 'tablet' | 'desktop'

/** 设备类别中文名（用于设置页提示） */
export const DEVICE_CLASS_LABELS: Record<DeviceClass, string> = {
  mobile: '📱 手机',
  tablet: '📱 平板',
  desktop: '💻 桌面',
}

/** 判断当前设备类别：手机 / 平板 / 桌面 */
export function detectDeviceClass(): DeviceClass {
  if (typeof navigator === 'undefined') return 'desktop'

  const ua = navigator.userAgent || ''
  const platform = navigator.platform || ''
  const maxTouchPoints = navigator.maxTouchPoints || 0

  // 平板：iPad / 明确 Tablet 标记 / Android 平板（无 Mobile 标记）/ iPadOS 伪装 Mac
  const isTablet =
    /iPad/i.test(ua) ||
    /Tablet/i.test(ua) ||
    /PlayBook/i.test(ua) ||
    (/Android/i.test(ua) && !/Mobile/i.test(ua)) ||
    (/Mac/i.test(platform) && maxTouchPoints > 1)

  // 手机：iPhone / iPod / Mobi / Android 手机
  const isMobile =
    /iPhone/i.test(ua) ||
    /iPod/i.test(ua) ||
    /Mobi/i.test(ua) ||
    (/Android/i.test(ua) && /Mobile/i.test(ua))

  if (isTablet) return 'tablet'
  if (isMobile) return 'mobile'
  return 'desktop'
}
