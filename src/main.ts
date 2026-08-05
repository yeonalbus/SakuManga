import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { watch } from 'vue'
import router from './router'
import App from './App.vue'
import { styleSettings } from './stores/styleSettings'

const app = createApp(App)

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

app.use(createPinia())
app.use(router)
app.mount('#app')
