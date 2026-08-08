/**
 * 页面隐藏 flush 工具
 *
 * 各 Store 的「后端同步」普遍采用防抖（如下载设置 300ms、阅读清单 200ms），
 * 用户修改后若在防抖窗口内关闭/刷新页面，防抖计时器随页面销毁而丢失，
 * 后端（唯一事实来源）仍保留旧值，下次启动会覆盖本地改动。
 *
 * 本工具在页面即将不可见时触发回调，供 Store 用 keepalive fetch 兜底 flush 未落盘改动。
 * - 同时监听 pagehide 与 beforeunload，覆盖关闭/刷新/前进后退/切后台等场景；
 * - 回调必须幂等（内部用 pending 标记去重），双事件重复触发不会重复上报。
 */
export function onPageHide(cb: () => void): void {
  if (typeof window === 'undefined') return
  window.addEventListener('pagehide', cb)
  // 兜底：极少数环境（如部分 WebView）只触发 beforeunload；幂等回调可安全双注册
  window.addEventListener('beforeunload', cb)
}
