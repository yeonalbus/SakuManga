// src/stores/userStore.ts
// 登录会话管理：token 存 localStorage，登录 / 登出 / 恢复会话
import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { http, HttpError } from '@/utils/request'
import { safeSetItem } from '@/utils/storage'
import { TOKEN_KEY } from '@/config/api'
import type { UserInfo, LoginResult } from '@/types/user'

export const useUserStore = defineStore('user', () => {
  // token 从 localStorage 初始化，页面刷新后仍保持登录状态
  const token = ref<string>(localStorage.getItem(TOKEN_KEY) || '')
  const user = ref<UserInfo | null>(null)

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.role === 'admin')
  const isEx = computed(() => !!user.value?.isEx)

  function setToken(value: string) {
    token.value = value
    if (value) {
      // 配额保护：token 是关键数据，配额满时 aggressive 回收（清进度缓存）确保能持久化，
      // 避免 localStorage 超限导致 token 写不进去、请求不带 Authorization 而误报「未登录」
      safeSetItem(TOKEN_KEY, value, { aggressive: true })
    } else {
      localStorage.removeItem(TOKEN_KEY)
    }
  }

  // 登录成功后写入 token 与用户信息
  // skipAuthRedirect：密码错误等 401 不应触发全局「会话失效」登出语义，仅由登录页展示错误
  async function login(username: string, password: string): Promise<UserInfo> {
    const data = await http<LoginResult>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
      skipAuthRedirect: true,
    })
    setToken(data.token)
    user.value = data.user
    return data.user
  }

  // 恢复会话：本地有 token 时向服务端校验并加载用户信息
  // 返回是否恢复成功（token 失效 / 网络失败返回 false，不抛异常）。
  // 仅 401（会话确实失效，http() 已同时 dispatch app:unauthorized 全局清理）才清空本地 token；
  // 网络错误/超时保留 token，避免瞬时网络波动导致用户被误登出。
  async function fetchMe(): Promise<boolean> {
    if (!token.value) return false
    try {
      const data = await http<{ user: UserInfo }>('/auth/me')
      user.value = data.user
      return true
    } catch (e) {
      if (e instanceof HttpError && e.status === 401) {
        setToken('')
        user.value = null
      }
      return false
    }
  }

  // 登出：通知服务端销毁会话并清空本地状态
  async function logout() {
    try {
      if (token.value) {
        await http<{ message: string }>('/auth/logout', { method: 'POST' })
      }
    } catch {
      // 忽略登出接口异常，本地状态照常清理
    } finally {
      setToken('')
      user.value = null
    }
  }

  // 仅清空本地状态（供 401 全局处理调用）
  function clear() {
    setToken('')
    user.value = null
  }

  return {
    token,
    user,
    isAuthenticated,
    isAdmin,
    isEx,
    login,
    fetchMe,
    logout,
    clear,
    setToken,
  }
})
