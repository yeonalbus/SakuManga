/**
 * localStorage 持久化工具
 * 供各领域 Store 复用：安全读取 JSON、失败时回退默认值
 */

/** 从 localStorage 安全读取 JSON 数据，解析失败时返回默认值 */
export function loadStorage<T>(key: string, defaultValue: T): T {
  try {
    const saved = localStorage.getItem(key)
    return saved ? JSON.parse(saved) : defaultValue
  } catch (e) {
    console.error(`读取 LocalStorage [${key}] 失败`, e)
    return defaultValue
  }
}

/** 写入 JSON 数据到 localStorage（内部捕获异常，避免影响主流程） */
export function saveStorage(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch (e) {
    console.error(`写入 LocalStorage [${key}] 失败`, e)
  }
}
