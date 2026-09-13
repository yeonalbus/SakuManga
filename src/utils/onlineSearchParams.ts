/**
 * 在线搜索参数构造（Round33 抽取）
 *
 * 原逻辑内联在 OnlineHome.initSearch，现抽为公共函数供两处复用：
 *  1. 首页执行搜索（OnlineHome）
 *  2. 书签失效后的「时间锚迁移」按原快照条件重新检索（bookmarkTimeAnchor）
 */
import type { FilterParams, SearchConfig } from '@/types/comic'
import { parseKeywordQueue } from '@/utils/tagFilter'

/**
 * 把 SearchConfig 快照转成列表接口参数。
 * - 顶栏主搜索词与筛选抽屉的多关键词队列合并为 f_search（空格分隔 = 隐式 AND）
 * - 负向项（`- ` 前缀）不下发（E 站不支持排除语法），交由渲染前本地剔除
 */
export const buildOnlineSearchParams = (cfg: SearchConfig): FilterParams => {
  const parsed = parseKeywordQueue(cfg.keywords)
  const searchBarParsed = parseKeywordQueue(cfg.keyword?.trim() ? [cfg.keyword] : [])
  const kwTokens = [...searchBarParsed.positive, ...parsed.positive]
    .map((t) => t.trim())
    .filter(Boolean)
  return {
    keyword: kwTokens.join(' '),
    categories: cfg.activeCategories,
    minRating: cfg.minRating,
    language: cfg.language,
    onlyRemoved: cfg.onlyRemoved,
    onlyTorrents: cfg.onlyTorrents,
    disableLangFilter: cfg.disableLangFilter,
    disableUploaderFilter: cfg.disableUploaderFilter,
    disableTagFilter: cfg.disableTagFilter,
  }
}
