/**
 * 书签失效检测编排（Round33）
 *
 * 流程：
 *  1. 调用后端批量检测（POST /scrape-bookmarks/check）
 *  2. 按结果分类处理：
 *     - replaced（E 站标记被新版本取代）→ **自动精确迁移**（零歧义，直接换 gid/token）
 *     - removed / copyright / invalid → 写入失效标记（持久化）
 *     - ok → 静默刷新元信息（标题/发布时间可能已更新）
 *     - error（网络/限流/缺 token）→ 不判定失效，跳过
 *  3. 对本次确认失效的书签：若记录过发布时间 → 计算「时间锚候选」并**询问用户**是否迁移
 */
import { ref } from 'vue'
import type { ScrapeBookmark, ScrapeBookmarkCheckResult } from '@/types/comic'
import {
  scrapeBookmarks,
  updateBookmarkAnchor,
  checkScrapeBookmarks,
  clearBookmarkInvalid,
  markBookmarkInvalid,
  bookmarkLocationLabel,
} from '@/stores/scrapeBookmarksStore'
import { findTimeAnchorCandidate, formatTimeDiff } from '@/utils/bookmarkTimeAnchor'
import { useUI } from '@/composables/useUI'

/** 检测摘要（供 UI 提示） */
export interface BookmarkCheckSummary {
  total: number
  ok: number
  invalid: number
  replaced: number
  migrated: number
  errors: number
}

export const useBookmarkCheck = () => {
  const { toast, modal } = useUI()
  const checking = ref(false)

  /** 书签展示名（名称优先，无名则位置标签） */
  const displayName = (bm: ScrapeBookmark): string =>
    bm.name.trim() || bookmarkLocationLabel(bm)

  /**
   * 精确迁移：把锚点换成 E 站标记的新版本画廊（零歧义，自动执行）
   * 保留原锚点快照到 migratedFrom（可回溯 / 撤销）
   */
  const migrateToNewVersion = async (
    bm: ScrapeBookmark,
    target: { gid: string; token?: string; title?: string },
  ): Promise<boolean> => {
    const prev = bm.anchor
    if (!prev) return false
    const ok = await updateBookmarkAnchor(bm.id, {
      gid: target.gid,
      token: target.token || prev.token,
      // 新版本标题通常相同：优先用已知标题，缺省沿用原锚点标题
      title: target.title || prev.title,
      postedAt: prev.postedAt,
      invalid: null,
      migratedFrom: {
        gid: prev.gid,
        token: prev.token,
        title: prev.title,
        postedAt: prev.postedAt,
      },
      listIndex: prev.listIndex,
    })
    return ok
  }

  /** 时间锚迁移（需用户确认）：按发布时间定位「那时那刻」附近的画廊 */
  const migrateByTimeAnchor = async (bm: ScrapeBookmark): Promise<boolean> => {
    const postedAt = bm.anchor?.postedAt?.trim()
    if (!postedAt) {
      toast.info(`「${displayName(bm)}」未记录发布时间，无法按时间迁移`)
      return false
    }
    toast.info(`正在为「${displayName(bm)}」检索同时刻画廊…`)
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
      `「${displayName(bm)}」锚定画廊已失效。\n\n` +
        `是否迁移到同时刻附近的画廊？\n「${cand.title}」\n` +
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
      invalid: null,
      migratedFrom: prev
        ? { gid: prev.gid, token: prev.token, title: prev.title, postedAt: prev.postedAt }
        : null,
      listIndex: prev?.listIndex,
    })
    if (success) toast.success(`已迁移到「${cand.title}」`)
    return success
  }

  /**
   * 应用检测结果。
   * @param askTimeAnchor 对确认失效的书签是否询问时间锚迁移
   */
  const applyResults = async (
    results: ScrapeBookmarkCheckResult[],
    opts?: { askTimeAnchor?: boolean },
  ): Promise<BookmarkCheckSummary> => {
    const summary: BookmarkCheckSummary = {
      total: results.length,
      ok: 0,
      invalid: 0,
      replaced: 0,
      migrated: 0,
      errors: 0,
    }
    const invalidOnes: ScrapeBookmark[] = []

    for (const r of results) {
      const bm = scrapeBookmarks.value.find((b) => b.id === String(r.id))
      if (!bm || !bm.anchor) continue
      switch (r.status) {
        case 'ok': {
          summary.ok++
          // 静默刷新元信息：标题改名 / 发布时间补齐
          const ref = r.refreshed
          if (ref) {
            const changed =
              (ref.title && ref.title !== bm.anchor.title) ||
              (ref.postedAt && ref.postedAt !== bm.anchor.postedAt)
            if (changed) {
              await updateBookmarkAnchor(bm.id, {
                ...bm.anchor,
                title: ref.title || bm.anchor.title,
                postedAt: ref.postedAt || bm.anchor.postedAt,
                invalid: null,
              })
            } else if (bm.anchor.invalid) {
              // 曾误判失效：恢复正常后清除标记
              await clearBookmarkInvalid(bm.id)
            }
          }
          break
        }
        case 'replaced': {
          summary.replaced++
          if (r.newVersion?.gid) {
            const done = await migrateToNewVersion(bm, r.newVersion)
            if (done) summary.migrated++
          }
          break
        }
        case 'removed':
        case 'copyright':
        case 'invalid': {
          summary.invalid++
          await markBookmarkInvalid(bm.id, r.status)
          invalidOnes.push(bm)
          break
        }
        default: {
          summary.errors++
          break
        }
      }
    }

    // 失效书签：按发布时间询问迁移（逐个确认，保持用户掌控）
    if (opts?.askTimeAnchor !== false) {
      for (const bm of invalidOnes) {
        if (bm.anchor?.postedAt) {
          const done = await migrateByTimeAnchor(bm)
          if (done) {
            summary.migrated++
            summary.invalid--
          }
        }
      }
    }

    return summary
  }

  /**
   * 执行检测。
   * @param ids 指定书签（跳转失败确认场景）；缺省 = 全量检测
   */
  const runCheck = async (ids?: string[]): Promise<BookmarkCheckSummary | null> => {
    if (checking.value) return null
    checking.value = true
    try {
      const results = await checkScrapeBookmarks(ids)
      if (results.length === 0) {
        toast.info('没有可检测的书签（未锚定卡片）')
        return null
      }
      const summary = await applyResults(results, { askTimeAnchor: true })
      const parts: string[] = [`有效 ${summary.ok}`]
      if (summary.migrated > 0) parts.push(`已迁移 ${summary.migrated}`)
      if (summary.invalid > 0) parts.push(`失效 ${summary.invalid}`)
      if (summary.errors > 0) parts.push(`未判定 ${summary.errors}`)
      const msg = `书签检测完成：${parts.join(' / ')}`
      if (summary.invalid > 0) toast.warning(msg)
      else toast.success(msg)
      return summary
    } catch (e) {
      console.error('书签失效检测失败:', e)
      toast.error(`检测失败：${e instanceof Error ? e.message : '未知错误'}`)
      return null
    } finally {
      checking.value = false
    }
  }

  return { checking, runCheck, applyResults, migrateByTimeAnchor }
}
