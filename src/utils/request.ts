// src/utils/request.ts
import { API_BASE } from '@/config/api'

interface RequestOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined>
}

/**
 * 基础 Fetch 封装
 */
export async function http<T = any>(endpoint: string, options: RequestOptions = {}): Promise<T> {
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

  // 3. 构造默认 Header
  const defaultHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  // 4. 发起请求
  const response = await fetch(url, {
    headers: {
      ...defaultHeaders,
      ...headers,
    },
    ...restOptions,
  })

  // 5. 统一异常处理
  if (!response.ok) {
    const errData = await response.json().catch(() => ({}))
    throw new Error(errData.error || `HTTP 错误 ${response.status}`)
  }

  return response.json()
}
