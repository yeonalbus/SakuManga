/**
 * 书签「时间锚」迁移（Round33）
 *
 * 背景：锚定画廊失效（被删除/下架）后无法精确恢复，但书签已记录锚定画廊的
 * 发布时间（anchor.postedAt）。E 站列表按发布时间倒序，因此可用
 * `seek=<postedAt 日期>` 定位到"那时那刻"的时间点，再取该时间点附近
 * 发布时间最接近的画廊作为新锚点。
 *
 * 为什么不用「位置索引」：首页流持续有新画廊插入头部，位置 N 的内容随时间漂移；
 * 而"某时刻附近有哪些画廊"是稳定事实，时间锚更可靠（详见计划文档）。
 */
import type { OnlineComic, ScrapeBookmark } from '@/types/comic'
import { fetchOnlineComicsApi } from '@/api/comic'
import { buildOnlineSearchParams } from '@/utils/onlineSearchParams'

/** 时间锚候选结果 */
export interface TimeAnchorCandidate {
  gid: string
  token?: string
  title: string
  postedAt: string
  /** 与目标发布时间的偏差（毫秒，越小越接近） */
  diffMs: number
  /** 已检索页数（诊断用） */
  pagesScanned: number
}

/** 解析 E 站发布时间文本（"2026-09-12 01:47"）为时间戳；无法解析返回 null */
export const parsePostedAt = (raw?: string): number | null => {
  const s = (raw || '').trim()
  if (!s) return null
  // 兼容 "2026-09-12 01:47" 与 "2026-09-12"
  const m = s.match(/^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2}))?/)
  if (!m) return null
  const [, y, mo, d, hh = '00', mm = '00'] = m
  const t = new Date(
    Number(y),
    Number(mo) - 1,
    Number(d),
    Number(hh),
    Number(mm),
  ).getTime()
  return Number.isFinite(t) ? t : null
}

/**
 * 按发布时间定位候选画廊。
 *
 * @param config   书签创建时的搜索/筛选快照（保证迁移后仍是同一检索上下文）
 * @param postedAt 锚定画廊发布时间（"2026-09-12 01:47"）
 * @param opts.maxPages 最多翻页数（默认 6 页 ≈ 150 张卡）
 * @returns 最接近目标时间的画廊；检索范围内无结果返回 null
 */
export const findTimeAnchorCandidate = async (
  config: ScrapeBookmark['config'],
  postedAt: string,
  opts?: { maxPages?: number },
): Promise<TimeAnchorCandidate | null> => {
  const target = parsePostedAt(postedAt)
  if (target === null) return null
  const seekDate = (postedAt || '').trim().slice(0, 10)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(seekDate)) return null
  const baseParams = buildOnlineSearchParams(config)
  const maxPages = Math.max(1, opts?.maxPages ?? 6)

  let best: { comic: OnlineComic; diffMs: number } | null = null
  let next: string | undefined
  let pagesScanned = 0

  for (let page = 0; page < maxPages; page++) {
    let res: { comics?: OnlineComic[]; next?: string }
    try {
      res = await fetchOnlineComicsApi(
        page === 0 ? { ...baseParams, seek: seekDate } : { ...baseParams, next },
      )
    } catch (e) {
      console.error('[书签时间锚] 检索失败:', e)
      break
    }
    const comics = res.comics || []
    pagesScanned++
    if (comics.length === 0) break

    for (const c of comics) {
      const t = parsePostedAt(c.updatedAt)
      if (t === null) continue
      const diffMs = Math.abs(t - target)
      if (!best || diffMs < best.diffMs) best = { comic: c, diffMs }
    }

    // 列表按发布时间倒序：本页最旧一条已早于目标时间 → 已越过，无需继续翻页
    const oldest = comics[comics.length - 1]
    const oldestT = parsePostedAt(oldest.updatedAt)
    if (oldestT !== null && oldestT <= target) break

    next = res.next
    if (!next) break
  }

  if (!best) return null
  return {
    gid: best.comic.id,
    token: best.comic.source === 'online' ? best.comic.token : undefined,
    title: best.comic.title,
    postedAt: best.comic.updatedAt || '',
    diffMs: best.diffMs,
    pagesScanned,
  }
}

/** 把时间偏差格式化成人类可读文本（如「23 分钟」「3 小时」） */
export const formatTimeDiff = (diffMs: number): string => {
  const min = Math.round(diffMs / 60000)
  if (min < 1) return '几乎同一时刻'
  if (min < 60) return `${min} 分钟`
  const hour = Math.round(min / 60)
  if (hour < 24) return `${hour} 小时`
  return `${Math.round(hour / 24)} 天`
}
