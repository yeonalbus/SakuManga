/**
 * 书签「按日期定位」编排（Round34）
 *
 * 背景：搜刮书签锚定的是「某次浏览到的某张卡片」，而在线列表从今天倒序展示；
 * 若只做滚动定位，就必须从最新一路翻到书签那天（实际永远翻不到，还会被误判失效）。
 *
 * 流程（E 站 seek 实测语义：`?seek=YYYY-MM-DD` 只吃日粒度，返回「该日末尾 → 更早」的倒序结果）：
 *   1. 首屏由页面按 `seek=<anchor.postedAt 日期>` 加载（本文件提供 `beginLocate` 返回该日期）
 *   2. 在已加载数据里找锚定 gid：
 *      · 命中 → 滚动居中 + 脉冲高亮 → 结束（并清除会话级失效标记）
 *      · 未命中且未越过 postedAt → 自动 next 翻页（静默追加，600ms 节流，硬异常上限 100 页）
 *      · 越过 / 到底 / 报错 → 结束，交由现有失效判定与时间锚迁移兜底
 *
 * Round35：翻页终止改为**动态判据**（不再用 15 页固定上限）——
 * 列表按发布时间倒序，只要本页最旧一条已早于锚定时间，后续只会更旧、不可能再有锚点，
 * 即可停止；因此「当日画廊密集」时多翻几页、「稀疏」时首屏即停，不会扫不全也不会白扫。
 *
 * 不打断原则：用户可自由滚动、手动点「加载更多」，与自动循环共用 store 的 isLoading 守卫。
 *
 * ⚠️ 状态必须**按组件实例隔离**：keep-alive 会同时保活多个 OnlineHome 实例
 * （PWA 同标签点书签 = 新 query → 新实例，旧实例仍在缓存里），若用模块级单例，
 * 旧实例的 abortLocate 会误杀新实例正在进行的定位（Round34 Bug1）。
 */
import { computed, nextTick, ref } from 'vue'
import type { ScrapeBookmark } from '@/types/comic'
import { useOnlineStore } from '@/stores/onlineStore'
import {
  bookmarkLocationLabel,
  clearAnchorFailed,
  markAnchorFailed,
} from '@/stores/scrapeBookmarksStore'
import { hasPassedTarget, seekDateOf } from '@/utils/bookmarkTimeAnchor'
import { useBookmarkCheck } from '@/composables/useBookmarkCheck'
import { useUI } from '@/composables/useUI'

/** 自动向下翻页的硬异常上限（页）：正常终止条件是「本页最旧一条已早于锚定时间」，
 *  与当日画廊密度无关（密集日多翻几页、稀疏日首屏即停）；此上限仅防 next 游标异常循环。 */
export const LOCATE_HARD_CAP_PAGES = 100
/** 每次翻页之间的间隔（毫秒）——列表接口不走后端 EHRateLimiter，限速必须由前端自理 */
export const LOCATE_PAGE_INTERVAL_MS = 600
/** 单页请求偶发失败（E 站限流/瞬时 5xx）时的最大重试次数 */
export const LOCATE_MAX_RETRY = 2
/** 重试间隔（毫秒；按第几次失败线性放大） */
export const LOCATE_RETRY_INTERVAL_MS = 1200
/** 连续被其他加载占用（IntersectionObserver 抢跑等）的最大重试次数 */
const LOCATE_MAX_SKIPPED = 20

/** 定位结束原因 */
export type LocateFinishReason =
  | 'found' // 命中锚定卡片
  | 'hidden' // 数据已加载，但被本地负向过滤规则隐藏（DOM 里没有卡片）
  | 'passed' // 已扫过书签时间点：该时段检索结果中没有这张卡
  | 'exhausted' // 列表已到尽头
  | 'limit' // 达到自动翻页上限
  | 'error' // 请求报错
  | 'aborted' // 被新定位 / 离开页面中止

/** 定位进行态（供提示条渲染；null = 未在定位） */
export interface BookmarkLocateState {
  bookmarkId: string
  name: string
  gid: string
  postedAt: string
  /** 本次 seek 的日期（'' = 书签缺发布时间，未做日期跳转） */
  seekDate: string
  /** 已扫描页数（含首屏） */
  pages: number
  /** 已扫描卡片数（约） */
  cards: number
}

const sleep = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms))

/** 查找锚定卡片的 DOM（列表渲染 + 未被负向过滤时才存在） */
const findCardEl = (gid: string): HTMLElement | null =>
  document.querySelector<HTMLElement>(`.item-card[data-gid="${gid}"]`)

/** 滚动到卡片中部 + 脉冲高亮（与 Round27 定位表现一致） */
const scrollToCard = (el: HTMLElement): void => {
  el.scrollIntoView({ block: 'center', behavior: 'smooth' })
  el.classList.add('bookmark-pulse')
  window.setTimeout(() => el.classList.remove('bookmark-pulse'), 2600)
}

/**
 * 组合式入口：每个页面实例持有一份独立定位状态。
 * 返回 `beginLocate`（登记并取得首屏 seek 日期）/ `runLocate`（自动翻页循环）/ `abortLocate`（中止）。
 */
export const useBookmarkLocate = () => {
  const locateState = ref<BookmarkLocateState | null>(null)
  /** 本实例的定位操作序号：begin/abort 时自增，循环每轮比对（旧循环立刻失效） */
  let locSeq = 0

  const locating = computed(() => locateState.value !== null)
  const locateInfo = computed(() => locateState.value)

  /** 结束定位（清状态 + 按原因提示/兜底） */
  const finish = async (
    reason: LocateFinishReason,
    st: BookmarkLocateState,
    detail?: string,
  ): Promise<void> => {
    if (locateState.value?.bookmarkId === st.bookmarkId) locateState.value = null
    const { toast } = useUI()
    switch (reason) {
      case 'found':
        // 定位成功：清掉会话级失效标记（此前只写不清，⚠️ 会永久残留）
        clearAnchorFailed(st.gid)
        return
      case 'hidden':
        toast.info('已加载到书签位置，但锚定画廊被当前负向过滤规则隐藏')
        return
      case 'limit':
        toast.info(
          `已扫描 ${st.pages} 页仍未越过书签「${st.name}」的锚定时间点，本次未判定（可稍后重试）`,
        )
        return
      case 'error':
        toast.error(`书签定位中断：${detail || '加载失败'}`)
        return
      case 'aborted':
        return
      case 'passed':
      case 'exhausted': {
        markAnchorFailed(st.gid)
        toast.warning(
          reason === 'passed'
            ? `书签「${st.name}」在当前搜索&筛选条件下已看不到锚定画廊（可能已失效）`
            : `已翻到列表尽头仍未找到书签「${st.name}」的锚定画廊（该条件下已不可见）`,
        )
        // 兜底：按书签原始搜索条件复核单条（Round35：与定位循环同一判据）
        const { runCheck } = useBookmarkCheck()
        const summary = await runCheck([st.bookmarkId])
        if (summary && summary.migrated > 0) {
          toast.info('锚点已迁移到同时刻的画廊；请从侧栏重新打开该书签')
        } else if (summary && summary.invalid === 0 && summary.errors === 0) {
          toast.info('复核检索仍能看到锚定画廊，可重试定位（列表结果可能已变化）')
        }
        return
      }
    }
  }

  /** 登记待定位书签，返回首屏 seek 日期（'' = 该跳转不带 seek） */
  const beginLocate = (bm: ScrapeBookmark): string => {
    abortLocate()
    const gid = bm.anchor?.gid || ''
    if (!gid) return ''
    const postedAt = bm.anchor?.postedAt?.trim() || ''
    locateState.value = {
      bookmarkId: bm.id,
      name: bm.name.trim() || bookmarkLocationLabel(bm) || gid,
      gid,
      postedAt,
      seekDate: seekDateOf(postedAt),
      pages: 0,
      cards: 0,
    }
    return locateState.value.seekDate
  }

  /** 中止当前定位（改搜索、刷新、离开页面、重新定位时调用） */
  const abortLocate = (): void => {
    locSeq += 1
    locateState.value = null
  }

  /**
   * 运行定位循环（页面首屏加载完成后调用，无需 await）。
   * 每轮：DOM 命中 → 滚动；否则判定越过/到底/上限后自动 next 翻页。
   */
  const runLocate = async (): Promise<void> => {
    const st = locateState.value
    if (!st) return
    const online = useOnlineStore()
    const mySeq = locSeq

    // 首屏（页面已按 seek 加载完成）计为第 1 页
    st.pages = 1
    st.cards = online.comics.length

    let skippedStreak = 0
    let failStreak = 0
    // 退出由内部 return 控制（命中 / 越过 / 到底 / 上限 / 中止）
    while (true) {
      if (mySeq !== locSeq || !locateState.value) return
      await nextTick()
      if (mySeq !== locSeq || !locateState.value) return

      // ① 命中：滚动居中 + 脉冲
      const el = findCardEl(st.gid)
      if (el) {
        scrollToCard(el)
        await finish('found', st)
        return
      }

      // ② 数据里有、DOM 里没有 → 被本地负向过滤规则剔除（继续翻页也找不到）
      if (online.comics.some((c) => c.id === st.gid)) {
        await finish('hidden', st)
        return
      }

      // ③ 越过判定：本页最旧一条已不晚于书签发布时间 → 该时段结果已扫完
      if (
        st.postedAt &&
        hasPassedTarget(online.comics[online.comics.length - 1]?.updatedAt, st.postedAt)
      ) {
        await finish('passed', st)
        return
      }

      // ④ 尽头 / 上限
      if (!online.hasMore || !online.nextGid) {
        await finish('exhausted', st)
        return
      }
      if (st.pages >= LOCATE_HARD_CAP_PAGES) {
        await finish('limit', st)
        return
      }

      // ⑤ 自动向下翻页（静默追加；节流避免触发 E 站限流）
      await sleep(LOCATE_PAGE_INTERVAL_MS)
      if (mySeq !== locSeq || !locateState.value) return

      const res = await online.loadMore()
      if (mySeq !== locSeq || !locateState.value) return

      if (res.skipped) {
        // 与其他加载（IntersectionObserver / 手动点击）竞争 → 稍后重试
        skippedStreak += 1
        if (skippedStreak > LOCATE_MAX_SKIPPED) {
          await finish('error', st, '加载被其他操作持续占用，请稍后重试')
          return
        }
        await sleep(300)
        continue
      }
      skippedStreak = 0

      if (res.error) {
        // 偶发失败（E 站限流 / 瞬时 5xx）先重试，避免一次抖动毁掉整次定位
        failStreak += 1
        if (failStreak <= LOCATE_MAX_RETRY) {
          await sleep(LOCATE_RETRY_INTERVAL_MS * failStreak)
          continue
        }
        await finish('error', st, res.error)
        return
      }
      failStreak = 0

      if (res.added === 0) {
        await finish('exhausted', st)
        return
      }
      st.pages += 1
      st.cards += res.added
    }
  }

  return { locateInfo, locating, beginLocate, runLocate, abortLocate }
}
