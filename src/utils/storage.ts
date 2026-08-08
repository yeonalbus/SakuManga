/**
 * localStorage 持久化工具
 * 供各领域 Store 复用：安全读取 JSON、失败时回退默认值。
 * 写入侧内置「配额超限自动回收」：优先裁剪可再生的低价值缓存
 * （阅读进度 saku_comic_progress:* 等），确保登录 token / 用户设置等
 * 关键数据始终能落盘，避免 QuotaExceededError 抛到渲染层导致白屏/报错。
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

/** 判断是否 localStorage 配额超限错误 */
const isQuotaError = (e: unknown): boolean =>
  e instanceof DOMException && (e.name === 'QuotaExceededError' || e.code === 22)

/** 阅读进度存储 key 前缀（每个账号一个 Map） */
const PROGRESS_KEY_PREFIX = 'saku_comic_progress:'

/** 每个账号阅读进度最多保留的条目数（超出删除最旧，进度可重新积累） */
const MAX_PROGRESS_ENTRIES = 300

/**
 * 裁剪全部阅读进度 Map：每个账号只保留最近 MAX 条；解析失败直接删除该 key。
 * 返回是否释放了空间。
 */
function trimProgressMaps(): boolean {
  let trimmed = false
  // 倒序遍历：删除条目不影响前面未访问的 index
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const key = localStorage.key(i)
    if (!key || !key.startsWith(PROGRESS_KEY_PREFIX)) continue
    try {
      const raw = localStorage.getItem(key)
      if (!raw) continue
      const map = JSON.parse(raw) as Record<string, number>
      const entries = Object.entries(map)
      if (entries.length > MAX_PROGRESS_ENTRIES) {
        // 对象 key 按插入序排列，slice(-MAX) 保留最近写入的条目
        const keep = Object.fromEntries(entries.slice(-MAX_PROGRESS_ENTRIES))
        localStorage.setItem(key, JSON.stringify(keep))
        trimmed = true
      }
    } catch {
      localStorage.removeItem(key)
      trimmed = true
    }
  }
  return trimmed
}

/** 激进清理：删除全部阅读进度缓存（低价值数据，可重新积累） */
function clearProgressMaps(): void {
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const key = localStorage.key(i)
    if (key && key.startsWith(PROGRESS_KEY_PREFIX)) {
      localStorage.removeItem(key)
    }
  }
}

/**
 * 安全写入 localStorage：
 * 1. 正常写入；
 * 2. 遇 QuotaExceededError 先裁剪进度缓存后重试一次；
 * 3. opts.aggressive 时若仍失败则删除全部进度缓存再重试（用于 token 等关键数据）。
 * 返回是否写入成功。
 */
export function safeSetItem(
  key: string,
  value: string,
  opts?: { aggressive?: boolean },
): boolean {
  try {
    localStorage.setItem(key, value)
    return true
  } catch (e) {
    if (!isQuotaError(e)) return false
  }

  // 第一轮：裁剪进度缓存释放空间后重试
  if (trimProgressMaps()) {
    try {
      localStorage.setItem(key, value)
      return true
    } catch {
      /* 继续走激进清理 */
    }
  }

  // 第二轮（aggressive）：删除全部进度缓存兜底
  if (opts?.aggressive) {
    clearProgressMaps()
    try {
      localStorage.setItem(key, value)
      return true
    } catch {
      return false
    }
  }
  return false
}

/** 写入 JSON 数据到 localStorage（内部捕获异常并自动回收配额，避免影响主流程） */
export function saveStorage(key: string, value: unknown): void {
  safeSetItem(key, JSON.stringify(value))
}
