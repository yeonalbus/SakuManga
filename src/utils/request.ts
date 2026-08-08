// src/utils/request.ts
import { API_BASE, TOKEN_KEY } from '@/config/api'
import { networkSettings } from '@/stores/networkSettings'

/** 统一 HTTP 错误：携带状态码，供调用方区分「会话失效(401)」与普通/网络错误 */
export class HttpError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'HttpError'
    this.status = status
  }
}

interface RequestOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined>
  /**
   * 置位时 401 不触发全局登出（不清 token、不 dispatch app:unauthorized），仅抛错。
   * 用于登录等本就预期返回 401 的接口（如密码错误），避免「登录失败」被当作「会话失效」处理。
   */
  skipAuthRedirect?: boolean
}

/**
 * 401 会话失效是否已通知应用层（模块级标记）：
 * 并发请求同时 401 时只 dispatch 一次 app:unauthorized，
 * 避免 main.ts 监听被多次触发；成功响应后复位，保证下次失效可再次通知。
 */
let unauthorizedHandled = false

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
  //    ⚠️ 网络设置收敛后：超时时间由 networkSettings.requestTimeout 控制（合并连接/接收为单一超时）
  const timeout =
    Number.isFinite(networkSettings.requestTimeout) && networkSettings.requestTimeout > 0
      ? networkSettings.requestTimeout
      : 60_000
  const response = await fetch(url, {
    headers: {
      ...defaultHeaders,
      ...headers,
    },
    signal: options.signal ?? AbortSignal.timeout(timeout),
    ...restOptions,
  })

  // 5. 统一异常处理
  if (response.ok) {
    // 成功响应：复位 401 通知标记，保证下一次会话失效仍能触发跳转
    unauthorizedHandled = false
    return response.json()
  }
  // 会话失效：清除本地 token 并通知应用层跳转登录页（去重，见 unauthorizedHandled 注释）。
  // skipAuthRedirect 置位时（如 /auth/login 密码错误）跳过全局登出语义，仅抛错由调用方处理。
  if (response.status === 401 && !options.skipAuthRedirect) {
    localStorage.removeItem(TOKEN_KEY)
    if (!unauthorizedHandled) {
      unauthorizedHandled = true
      window.dispatchEvent(new Event('app:unauthorized'))
    }
  }
  const errData = await response.json().catch(() => ({}))
  throw new HttpError(response.status, errData.error || `HTTP 错误 ${response.status}`)
}
