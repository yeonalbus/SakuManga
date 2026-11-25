import type { ScrapeBookmark, ScrapeBookmarkRole, SearchConfig } from '../types/comic'
import { isSameTransferScope } from './bookmarkScope'

/**
 * 将 E 站发布时间字符串（如 "2026-08-10 10:00" 或 ISO）解析为毫秒时间戳；
 * 无法解析时返回 null。
 */
export const parseBookmarkPostedAt = (raw?: string | null): number | null => {
  const s = (raw || '').trim()
  if (!s) return null
  // 兼容 "YYYY-MM-DD HH:mm" / "YYYY-MM-DD HH:mm:ss"
  const normalized = /^\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}/.test(s) ? s.replace(/\s+/, 'T') : s
  const ms = Date.parse(normalized)
  return Number.isFinite(ms) ? ms : null
}

/** 同距兜底优先级：续扫 > 兴趣 > 追新（避免误带出追新的更新探测） */
const ROLE_TIE_PRIORITY: Record<ScrapeBookmarkRole, number> = {
  oldest: 3,
  interest: 2,
  newest: 1,
}

const pickByCreatedAtDesc = (candidates: ScrapeBookmark[]): ScrapeBookmark | null => {
  if (candidates.length === 0) return null
  let best = candidates[0]!
  for (let i = 1; i < candidates.length; i++) {
    const cur = candidates[i]!
    if ((cur.createdAt || 0) > (best.createdAt || 0)) best = cur
  }
  return best
}

const pickCloser = (
  a: { bm: ScrapeBookmark; t: number },
  b: { bm: ScrapeBookmark; t: number },
  targetMs: number,
): ScrapeBookmark => {
  const da = Math.abs(targetMs - a.t)
  const db = Math.abs(targetMs - b.t)
  if (da !== db) return da < db ? a.bm : b.bm
  const pa = ROLE_TIE_PRIORITY[a.bm.role] ?? 0
  const pb = ROLE_TIE_PRIORITY[b.bm.role] ?? 0
  if (pa !== pb) return pa > pb ? a.bm : b.bm
  return (a.bm.createdAt || 0) >= (b.bm.createdAt || 0) ? a.bm : b.bm
}

/**
 * 从同检索条件候选书签中挑选最适合预填充的一条（纯函数，无 store 依赖）。
 *
 * 规则：
 * 1. 无有效 targetPostedAt 或全部候选缺 postedAt → 取 createdAt 最大的候选；
 * 2. 落在时间轴两端之外 → 取距离最近的一端；
 * 3. 夹在相邻两条 older (<= T) 与 newer (>= T) 之间：
 *    - 若仅一侧与 T 等时间戳 → 取该侧；
 *    - 方向匹配：older 为追新(newest) 或 newer 为续扫(oldest)；
 *      · 仅一侧方向匹配 → 取匹配侧；
 *      · 追新与续扫夹心（无论谁上谁下）→ 统一取续扫(oldest)，避免误带出追新更新探测；
 *      · 其余情况 → 取时间距离 |T - postedAt| 更近的一侧。
 */
export const pickPrefillFromCandidates = (
  candidates: ScrapeBookmark[],
  targetPostedAt?: string,
): ScrapeBookmark | null => {
  if (candidates.length === 0) return null

  const targetMs = parseBookmarkPostedAt(targetPostedAt)
  if (targetMs === null) return pickByCreatedAtDesc(candidates)

  const timed = candidates
    .map((bm) => ({ bm, t: parseBookmarkPostedAt(bm.anchor?.postedAt) }))
    .filter((item): item is { bm: ScrapeBookmark; t: number } => item.t !== null)
    .sort((a, b) => a.t - b.t || (b.bm.createdAt || 0) - (a.bm.createdAt || 0))

  if (timed.length === 0) return pickByCreatedAtDesc(candidates)

  const earliest = timed[0]!
  const latest = timed[timed.length - 1]!

  if (targetMs <= earliest.t) return earliest.bm
  if (targetMs >= latest.t) return latest.bm

  // 找到夹住 targetMs 的相邻区间 [older, newer]
  let older = earliest
  let newer = latest
  for (let i = 0; i < timed.length - 1; i++) {
    const cur = timed[i]!
    const next = timed[i + 1]!
    if (cur.t <= targetMs && targetMs <= next.t) {
      older = cur
      newer = next
      break
    }
  }

  // 恰好落在某一侧时间戳上
  const olderEq = older.t === targetMs
  const newerEq = newer.t === targetMs
  if (olderEq !== newerEq) {
    return olderEq ? older.bm : newer.bm
  }

  // 追新与续扫夹心（无论哪侧在上）→ 优先载入续扫，避免误开更新探测
  const hasNewest = older.bm.role === 'newest' || newer.bm.role === 'newest'
  const hasOldest = older.bm.role === 'oldest' || newer.bm.role === 'oldest'
  if (hasNewest && hasOldest) {
    return older.bm.role === 'oldest' ? older.bm : newer.bm
  }

  // 方向匹配：追新往更新方向推进（T > older），续扫往更旧方向推进（T < newer）
  const olderMatch = older.bm.role === 'newest'
  const newerMatch = newer.bm.role === 'oldest'
  if (olderMatch !== newerMatch) {
    return olderMatch ? older.bm : newer.bm
  }

  return pickCloser(older, newer, targetMs)
}

/**
 * 为「存为书签」弹窗挑选同条件（isSameTransferScope）的预填充参考书签。
 */
export const pickPrefillBookmark = (
  bookmarks: ScrapeBookmark[],
  currentConfig: Partial<SearchConfig>,
  targetPostedAt?: string,
): ScrapeBookmark | null => {
  const candidates = bookmarks.filter((b) => isSameTransferScope(b.config, currentConfig))
  return pickPrefillFromCandidates(candidates, targetPostedAt)
}
