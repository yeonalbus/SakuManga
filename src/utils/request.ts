// src/utils/request.ts
import { API_BASE, TOKEN_KEY } from '@/config/api'

interface RequestOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined>
}

/**
 * 基础 Fetch 封装
 */
export async function http<T = unknown>(
  endpoint: string,
  options: RequestOptions = {},
): Promise<T> {
  const { params, headers, ...restOptions } = options

  // 1. 自动处理斜杠拼接
  const cleanEndpoint = endpoint.startsWith('/') ? endpoint : `/${endpoint}`
  let url = `${API_BASE}${cleanEndpoint}`

  // 2. 自动拼接 Query 参数
  if (params) {
    const searchParams = new URLSearchParams()
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null) {
        searchParams.append(key, String(value))
      }
    })
    const queryString = searchParams.toString()
    if (queryString) {
      url += `?${queryString}`
    }
  }

  // 3. 构造默认 Header（自动附加 Bearer token）
  const defaultHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) {
    defaultHeaders['Authorization'] = `Bearer ${token}`
  }

  // 4. 发起请求（默认 60s 超时，防止慢接口导致无限 loading；调用方可通过 signal 覆盖）
  const response = await fetch(url, {
    headers: {
      ...defaultHeaders,
      ...headers,
    },
    signal: options.signal ?? AbortSignal.timeout(60_000),
    ...restOptions,
  })

  // 5. 统一异常处理
  if (!response.ok) {
    // 会话失效：清除本地 token 并通知应用层跳转登录页
    if (response.status === 401) {
      localStorage.removeItem(TOKEN_KEY)
      window.dispatchEvent(new Event('app:unauthorized'))
    }
    const errData = await response.json().catch(() => ({}))
    throw new Error(errData.error || `HTTP 错误 ${response.status}`)
  }

  return response.json()
}
