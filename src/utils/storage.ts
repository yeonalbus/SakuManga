/**
 * localStorage 持久化工具
 * 供各领域 Store 复用：安全读取 JSON、失败时回退默认值。
 * 写入侧内置「配额超限自动回收」：优先裁剪可再生的低价值缓存
 * （阅读进度 saku_comic_progress:*、错误环、搜索历史、视图模式等），
 * 确保登录 token / 用户设置等关键数据始终能落盘，
 * 避免 QuotaExceededError 抛到渲染层导致白屏/报错，也避免「内存生效、刷新丢失」。
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

/** 每个账号阅读进度最多保留的条目数（超出删除最旧，进度可重新积累）。
 *  唯一权威定义：阅读进度写入（readingProgress.ts）与配额回收（本文件）共用，
 *  避免两侧阈值不一致导致 301~500 条区间漏过第一层回收。 */
export const MAX_PROGRESS_ENTRIES = 300

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

/** 其余低价值可再生缓存键：配额紧张时作为最后回收对象（均可由用户行为重新产生） */
const LOW_VALUE_KEYS = ['app_error_log', 'app_search_history', 'app_view_mode']

/** 删除低价值可再生缓存（错误环/搜索历史/视图模式），返回是否删除了至少一项 */
function evictLowValueCaches(): boolean {
  let removed = false
  for (const key of LOW_VALUE_KEYS) {
    try {
      if (localStorage.getItem(key) !== null) {
        localStorage.removeItem(key)
        removed = true
      }
    } catch {
      /* 单键异常忽略，继续尝试其他键 */
    }
  }
  return removed
}

/**
 * 安全写入 localStorage：
 * 1. 正常写入；
 * 2. 遇 QuotaExceededError 先裁剪进度缓存（每账号保留最近 MAX 条）后重试；
 * 3. opts.aggressive（关键数据：token / 用户设置）时走完整回收级联，
 *    依次「清空全部进度缓存 → 清空低价值缓存」直至写入成功；
 * 4. 仍失败时输出 console.warn（不再静默丢弃），便于排查。
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

  // 第一轮：裁剪进度缓存（每个账号只保留最近 MAX 条）后重试
  if (trimProgressMaps()) {
    try {
      localStorage.setItem(key, value)
      return true
    } catch {
      /* 空间仍不足，继续回收 */
    }
  }

  // 第二轮起（aggressive）：关键数据走完整回收级联，确保始终能落盘
  if (opts?.aggressive) {
    // 第二轮：删除全部进度缓存（低价值、可重新积累）
    clearProgressMaps()
    try {
      localStorage.setItem(key, value)
      return true
    } catch {
      /* 继续 */
    }
    // 第三轮：删除其余低价值可再生缓存（错误环/搜索历史/视图模式）
    if (evictLowValueCaches()) {
      try {
        localStorage.setItem(key, value)
        return true
      } catch {
        /* 空间仍不足 */
      }
    }
  }

  console.warn(
    `LocalStorage 写入失败[${key}]：配额不足且无可回收缓存，本次修改无法持久化`,
  )
  return false
}

/**
 * 写入 JSON 数据到 localStorage（内部捕获异常并自动回收配额，避免影响主流程）。
 * 所有设置 Store 均通过本函数落盘，属关键数据，统一走 aggressive 完整回收级联，
 * 避免配额紧张时设置「内存生效、刷新丢失」。
 */
export function saveStorage(key: string, value: unknown): boolean {
  return safeSetItem(key, JSON.stringify(value), { aggressive: true })
}
