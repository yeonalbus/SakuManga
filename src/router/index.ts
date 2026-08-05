//配置路由表
import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { TOKEN_KEY } from '@/config/api'
import { getMainContent, rememberScroll, restoreScroll } from '@/utils/scrollMemory'

//显式声明 : RouteRecordRaw[]
const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/LoginView.vue'),
  },
  {
    path: '/',
    redirect: '/online/home',
  },
  {
    path: '/online/home',
    name: 'OnlineHome',
    component: () => import('@/views/online/OnlineHome.vue'),
  },
  {
    path: '/online/favorites',
    name: 'OnlineFavorites',
    component: () => import('@/views/online/OnlineFavorites.vue'),
  },
  {
    path: '/online/hot',
    name: 'OnlineHot',
    component: () => import('@/views/online/OnlineHot.vue'),
  },
  {
    path: '/online/sub',
    name: 'OnlineSub',
    component: () => import('@/views/online/OnlineSub.vue'),
  },
  {
    path: '/online/history',
    name: 'OnlineHistory',
    component: () => import('@/views/online/OnlineHistory.vue'),
  },
  {
    path: '/online/toplist',
    name: 'OnlineTop',
    component: () => import('@/views/online/OnlineTop.vue'),
  },
  {
    path: '/offline/home',
    name: 'OfflineHome',
    component: () => import('@/views/offline/OfflineHome.vue'),
  },
  {
    path: '/offline/update',
    name: 'OfflineUpdate',
    component: () => import('@/views/offline/OfflineUpdate.vue'),
  },
  {
    path: '/offline/bookshelf',
    name: 'OfflineBookshelf',
    component: () => import('@/views/offline/OfflineBookshelf.vue'),
  },
  {
    path: '/offline/maintain',
    name: 'OfflineMaintain',
    component: () => import('@/views/offline/OfflineMaintain.vue'),
  },
  {
    path: '/offline/toplist',
    name: 'OfflineToplist',
    component: () => import('@/views/offline/OfflineToplist.vue'),
  },
  {
    path: '/offline/history',
    name: 'OfflineHistory',
    component: () => import('@/views/offline/OfflineHistory.vue'),
  },
  {
    path: '/member-history',
    name: 'MemberHistory',
    component: () => import('@/views/MemberHistory.vue'),
  },
  {
    path: '/downloads',
    name: 'Downloads',
    component: () => import('@/views/DownloadsView.vue'),
  },
  {
    path: '/settings',
    name: 'Settings',
    component: () => import('@/views/SettingsView.vue'),
  },
  {
    path: '/online/detail',
    name: 'OnlineDetail',
    component: () => import('@/views/online/OnlineDetail.vue'),
  },
  {
    path: '/offline/detail',
    name: 'OfflineDetail',
    component: () => import('@/views/offline/OfflineDetail.vue'),
  },
  // 📖 漫画阅读器路由
  {
    path: '/reader',
    name: 'ComicReader',
    component: () => import('@/views/ComicReader.vue'),
  },
  // 404 兜底：任意未匹配路径重定向到 NotFound 页面
  {
    path: '/not-found',
    name: 'NotFound',
    component: () => import('@/views/NotFound.vue'),
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/not-found',
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 全局登录守卫：未登录只能访问 /login；已登录访问 /login 时重定向到主界面
router.beforeEach((to) => {
  const token = localStorage.getItem(TOKEN_KEY)
  const isLoginPage = to.path === '/login'
  if (!token && !isLoginPage) {
    // 记录目标地址，登录成功后跳回
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (token && isLoginPage) {
    return { path: '/online/home' }
  }
  return true
})

// 返回逻辑优化：离开页面时记录 .main-content 滚动位置，返回时恢复
router.beforeEach((to, from) => {
  const el = getMainContent()
  if (el && from.path !== '/login') {
    rememberScroll(from.path, el.scrollTop)
  }
  return true
})

router.afterEach((to) => {
  restoreScroll(to.path)
})

export default router
