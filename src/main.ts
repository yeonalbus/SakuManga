import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { watch } from 'vue'
import router from './router'
import App from './App.vue'
// Round22：拖拽排序全局样式（把手 / 幽灵卡 / 落位指示线）
import './styles/dragSort.css'
import { styleSettings } from './stores/styleSettings'
import { useUserStore } from './stores/userStore'
import { loadUserLibrary } from './stores/libraryInit'
import { reportError } from './utils/errorReporter'

const app = createApp(App)

// 全局错误捕获（问题8）：Vue 渲染/生命周期错误集中上报并记录日志，
// 配合 App.vue 中的错误边界避免单个组件报错导致整棵树卸载白屏。
app.config.errorHandler = (err, _instance, info) => {
  console.error('[global-error]', err, info)
  reportError(
    'error',
    err instanceof Error ? err.message : err,
    err instanceof Error ? err.stack : undefined,
    info,
  )
}

// 非 Vue 域错误（资源加载失败 / 未捕获 Promise 拒绝）也一并上报落盘
window.addEventListener('error', (e) => {
  reportError(
    'error',
    e.message,
    e.error instanceof Error ? e.error.stack : undefined,
    'window:error',
  )
})
window.addEventListener('unhandledrejection', (e) => {
  const reason = e.reason
  reportError(
    'error',
    reason instanceof Error ? reason.message : String(reason ?? 'unknown rejection'),
    reason instanceof Error ? reason.stack : undefined,
    'unhandledrejection',
  )
})

// 应用主题模式（system / dark / light）到 <html data-theme>
// 全局样式根据 data-theme 切换 CSS 变量（见 App.vue 的 :root 定义）
const applyTheme = (mode: string) => {
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  const isDark = mode === 'dark' || (mode === 'system' && prefersDark)
  document.documentElement.setAttribute('data-theme', isDark ? 'dark' : 'light')
}
applyTheme(styleSettings.themeMode)
watch(
  () => styleSettings.themeMode,
  (mode) => applyTheme(mode),
)

const pinia = createPinia()
app.use(pinia)

// 恢复登录会话：本地存在 token 时向服务端校验并加载当前用户信息
const userStore = useUserStore(pinia)
// 会话恢复成功后，加载当前用户的书架/历史/阅读清单/评分（含旧数据迁移）
userStore.fetchMe().then((ok) => {
  if (ok) loadUserLibrary()
})

// 全局监听 401：会话失效时清空用户状态并回到登录页
window.addEventListener('app:unauthorized', () => {
  userStore.clear()
  if (router.currentRoute.value.path !== '/login') {
    router.replace({
      path: '/login',
      query: { redirect: router.currentRoute.value.fullPath },
    })
  }
})


// ─────────────────────────────────────────────────────────────
// Round17：iOS PWA 链接逃逸防护
//
// iOS standalone 模式下，点击同源 <a>（含 Vue Router router-link 的底层
// <a href>）可能触发原生导航，逃逸出 PWA 显示 Safari 顶栏/底部工具栏。
// 在捕获阶段拦截：PWA 下对「同源 + 非 target=_blank」的 <a> 点击
// preventDefault，改由 Vue Router 完成 SPA 导航（保持 standalone）。
// 跨域/外链（target=_blank）保持默认，iOS 会走 SFSafariViewController。
// ─────────────────────────────────────────────────────────────
const isStandaloneMode = (): boolean => {
  if (typeof window === 'undefined') return false
  if (window.matchMedia('(display-mode: standalone)').matches) return true
  if ((window.navigator as unknown as { standalone?: boolean }).standalone === true) return true
  return false
}

const preventPwaLinkEscape = (e: MouseEvent) => {
  if (!isStandaloneMode()) return
  // 仅拦截主键左键（不拦截中键/Ctrl 新标签语义）
  if (e.button !== 0 && e.button !== undefined) return
  if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return
  const target = e.target as Element | null
  const anchor = target?.closest?.('a[href]') as HTMLAnchorElement | null
  if (!anchor) return
  if (anchor.target === '_blank') return // 外链/新标签走系统浏览器，不拦截
  const href = anchor.getAttribute('href') || ''
  if (!href || href.startsWith('javascript:')) return
  // 同源判断：相对路径或同 origin 绝对路径
  let url: URL
  try {
    url = new URL(href, window.location.origin)
  } catch {
    return
  }
  if (url.origin !== window.location.origin) return // 跨域交给系统浏览器
  // 拦截原生导航 → 交给 Vue Router SPA 处理
  e.preventDefault()
  e.stopPropagation()
  void router.push(url.pathname + url.search + url.hash)
}
document.addEventListener('click', preventPwaLinkEscape, true)

app.use(router)
app.mount('#app')
