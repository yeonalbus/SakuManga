/**
 * 书签锚点迁移（Round49：从 useBookmarkCheck 提取为独立模块）
 *
 * 背景：失效检测已剪枝（不再有批量 check），但「锚点失效 → 按发布时间迁移到
 * 同时刻附近的画廊」这条兜底流程仍在服役——它现在肩负担全部失效处理：
 *   - 书签跳转定位失败（越过 / 到尽头）时自动触发；
 *   - 迁移成功后由调用方继续重新定位（useBookmarkLocate 的 relocateRequest）。
 *
 * 迁移只更新锚点四元组（gid/token/title/postedAt），书签的备注、标记、开关、
 * 置顶、已囤货状态一概不动；后端在锚点更新时顺带清除更新标记（红点）。
 *
 * 为什么统一走时间锚：用户关心的是「上次搜刮到了哪个位置」，而新版本画廊的
 * 发布时间不同、在列表中的位置也不同，直接换成新版本 gid 会把人带到另一个位置。
 */
import { bookmarkLocationLabel, updateBookmarkAnchor } from '@/stores/scrapeBookmarksStore'
import type { ScrapeBookmark } from '@/types/comic'
import { findTimeAnchorCandidate, formatTimeDiff } from '@/utils/bookmarkTimeAnchor'
import { useUI } from '@/composables/useUI'

export const useBookmarkMigrate = () => {
  const { toast, modal } = useUI()

  /** 书签展示名（名称优先，无名则位置标签） */
  const displayName = (bm: ScrapeBookmark): string =>
    bm.name.trim() || bookmarkLocationLabel(bm)

  /**
   * 时间锚迁移（需用户确认）：按发布时间定位「那时那刻」附近的画廊。
   *
   * @param replacedHint 该画廊是否被 E 站标记为「已被新版本取代」（用于提示文案）
   */
  const migrateByTimeAnchor = async (
    bm: ScrapeBookmark,
    replacedHint = false,
  ): Promise<boolean> => {
    const postedAt = bm.anchor?.postedAt?.trim()
    if (!postedAt) {
      toast.info(`「${displayName(bm)}」未记录发布时间，无法按时间迁移`)
      return false
    }
    toast.info(
      replacedHint
        ? `「${displayName(bm)}」已有新版本，按发布时间检索原位置…`
        : `正在为「${displayName(bm)}」检索同时刻画廊…`,
    )
    const cand = await findTimeAnchorCandidate(bm.config, postedAt)
    if (!cand) {
      toast.warning(`「${displayName(bm)}」未找到可迁移的同时刻画廊（保留位置快照）`)
      return false
    }
    if (cand.gid === bm.anchor?.gid) {
      toast.info(`「${displayName(bm)}」检索回到原画廊，无需迁移`)
      return false
    }
    const ok = await modal.confirm(
      `「${displayName(bm)}」${replacedHint ? '锚定画廊已有新版本（位置可能已变化）' : '锚定画廊已失效'}。\n\n` +
        `是否迁移到同时刻附近的画廊以恢复原位置？\n「${cand.title}」\n` +
        `发布时间 ${cand.postedAt}（相差 ${formatTimeDiff(cand.diffMs)}）`,
      '锚点迁移',
    )
    if (!ok) {
      toast.info(`已保留「${displayName(bm)}」的位置快照（锚点仍标记失效）`)
      return false
    }
    const prev = bm.anchor
    const success = await updateBookmarkAnchor(bm.id, {
      gid: cand.gid,
      token: cand.token,
      title: cand.title,
      postedAt: cand.postedAt || prev?.postedAt,
      // Round49：迁移时一并换封面（候选画廊的封面）；取不到则保留旧封面，避免丢图
      coverUrl: cand.coverUrl || prev?.coverUrl || undefined,
      invalid: null,
      migratedFrom: prev
        ? { gid: prev.gid, token: prev.token, title: prev.title, postedAt: prev.postedAt }
        : null,
      listIndex: prev?.listIndex,
    })
    if (success) toast.success(`已迁移到「${cand.title}」`)
    return success
  }

  return { migrateByTimeAnchor, displayName }
}
