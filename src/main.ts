import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { watch } from 'vue'
import router from './router'
import App from './App.vue'
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

app.use(router)
app.mount('#app')
