<script setup lang="ts">
import { ref, computed, onMounted, onActivated, onUnmounted } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import { openComicDetailInNewTab } from '@/utils/detailNav'

// ─────────────────────────────────────────────────────────────
// Round44：本地书库维护 v3
//
// 设计要点（珱垣拍板，见 plans/round44-offline-maintain-ui-plan.md）：
//   - 目标「彻底清零」：主列表只显示待处理项，已忽略内容移出（忽略清单独立页）；
//   - 分组折叠总览：按「两本差在哪」分 6 类，默认只展开「高度相似」；
//   - 成员级动作统一为「移除这本」（选删除谁）；「保留」只有组级「都留（不再提示）」；
//   - 差异 chips 让用户不进对比页也能判断；需要细节时在列表内就地展开对比；
//   - 「含文件」为页面级勾选，默认不勾、记忆上次选择（存后端设置）；
//   - 忽略＝"直到有变化为止"：命中忽略且无新增的组静默；出现忽略后新入库的本子才列出并标注。
// ─────────────────────────────────────────────────────────────

const { modal, toast } = useUI()

interface OfflineComicDTO {
  id: string
  title: string
  coverUrl: string
  category?: string
  pageCount?: number
  updatedAt: string
  localPath?: string
  fileSize?: number
  gid?: string
  sourceMode?: string
  onlineTags?: string
}

interface DedupItemDTO {
  comic: OfflineComicDTO
  reason: string // 重复原因
  keep: boolean // true=建议保留，false=建议删除
  rule?: string // gid | hash | parent | signature（仅 parent 可忽略）
}

interface ClusterMemberDTO {
  comic: OfflineComicDTO
  pageCount: number
  lang?: string
  artist?: string
}

// 疑似重复组（名称级弱证据，只建议）
interface DedupClusterDTO {
  id: string
  titleKey: string
  artist?: string
  confidence: 'high' | 'medium'
  reason: string
  members: ClusterMemberDTO[]
  ignored?: boolean // 命中忽略但出现"忽略后新增"成员（Round44）
  ignoredNewCount?: number
  ignoreId?: string
}

interface DedupResultDTO {
  items: DedupItemDTO[]
  clusters?: DedupClusterDTO[]
  finishedAt?: number
  stale?: boolean
}

interface UnsyncedStatusDTO {
  lastLibraryChange: number
  resultFinishedAt: number
  hasUnsynced: boolean
}

interface OfflineTaskState {
  type: 'maintain' | 'update'
  status: 'idle' | 'running' | 'paused' | 'success' | 'error' | 'cancelled'
  phase?: string
  total: number
  done: number
  currentTitle?: string
  message?: string
  startedAt?: number
  finishedAt?: number
  error?: string
}

// ── 状态 ──
const items = ref<DedupItemDTO[]>([])
const clusters = ref<DedupClusterDTO[]>([])
const resultStale = ref(false)
const isScanning = ref(false)
const isRemoving = ref(false)
const removingId = ref('')
const coverFailed = ref<Record<string, boolean>>({})
const deleteFile = ref(false) // 「含文件」勾选（记忆于后端设置）
const density = ref<'comfortable' | 'compact'>(readDensity())
const moreMenuOpen = ref(false)
const expanded = ref<Record<string, boolean>>({}) // 就地展开对比的簇 id
const openGroups = ref<Record<string, boolean>>({ F: true, REMOVE: true })
const processedCount = ref(0) // 本次进入页面后已处理的组数（进度条用）

const taskState = ref<OfflineTaskState | null>(null)
let pollTimer: ReturnType<typeof setInterval> | null = null

const isTaskPaused = computed(() => taskState.value?.status === 'paused')
const isTaskRunning = computed(() => taskState.value?.status === 'running')
const progressPercent = computed(() => {
  const s = taskState.value
  if (!s || s.total <= 0) return 0
  return Math.min(100, Math.round((s.done / s.total) * 100))
})
const phaseText = computed(() => taskState.value?.phase || '')
const currentTitle = computed(() => taskState.value?.currentTitle || '')

const keepItems = computed(() => items.value.filter((i) => i.keep))
const removeItems = computed(() => items.value.filter((i) => !i.keep))

// ── 标签 / 差异判定（纯前端，数据已在 clusters 内）──
const parseTags = (raw?: string): string[] => {
  if (!raw) return []
  try {
    const arr = JSON.parse(raw)
    return Array.isArray(arr) ? arr.map((t) => String(t)) : []
  } catch {
    return []
  }
}
const hasChineseTag = (c: OfflineComicDTO) =>
  parseTags(c.onlineTags).some((t) => t.toLowerCase() === 'language:chinese')
const hasDecensored = (c: OfflineComicDTO) =>
  (c.title || '').toLowerCase().includes('decensored') ||
  parseTags(c.onlineTags).some((t) => t.toLowerCase().includes('decensored'))

const sizeRatio = (a?: number, b?: number) => {
  if (!a || !b || a <= 0 || b <= 0) return 1
  return a > b ? a / b : b / a
}
const dirOf = (p?: string) => {
  if (!p) return ''
  const parts = p.replace(/\\/g, '/').split('/').filter(Boolean)
  return parts.slice(-2).join('/')
}

// 「这两本差在哪」：分类（决定进哪个分组）+ chips（卡片上直接展示）
const classify = (cl: DedupClusterDTO): string => {
  const ms = cl.members
  if (ms.length >= 3) return 'MULTI'
  if (ms.length < 2) return 'F'
  const [a, b] = ms
  if (hasChineseTag(a.comic) !== hasChineseTag(b.comic)) return 'A'
  if (Math.abs((a.pageCount || 0) - (b.pageCount || 0)) > 5) return 'C'
  if (hasDecensored(a.comic) !== hasDecensored(b.comic)) return 'D'
  if (sizeRatio(a.comic.fileSize, b.comic.fileSize) >= 1.5) return 'E'
  return 'F'
}

const diffChips = (cl: DedupClusterDTO): string[] => {
  const out: string[] = []
  const ms = cl.members
  if (ms.length >= 3) out.push(`${ms.length} 本一组`)
  if (ms.length >= 2) {
    const [a, b] = ms
    out.push(`页数 ${a.pageCount || 0} vs ${b.pageCount || 0}`)
    const r = sizeRatio(a.comic.fileSize, b.comic.fileSize)
    if (r >= 1.5) out.push(`体积 ${r.toFixed(1)}×`)
    if ((a.lang || '') !== (b.lang || '')) out.push(`语言 ${a.lang || '—'} / ${b.lang || '—'}`)
    if (hasDecensored(a.comic) !== hasDecensored(b.comic)) out.push('去码标记不同')
    out.push(dirOf(a.comic.localPath) === dirOf(b.comic.localPath) ? '同目录' : '不同目录')
  }
  return out
}

const chipClass = (chip: string) => {
  if (chip.startsWith('语言')) return 'diff lang'
  if (chip.startsWith('体积')) return 'diff size'
  if (chip.startsWith('页数')) return 'diff page'
  return 'diff'
}

const GROUP_META: Record<string, { name: string; desc: string }> = {
  F: { name: '高度相似', desc: '标题 / 页数 / 体积几乎一致 —— 最像重复下载，建议先处理' },
  A: { name: '语言版本不同', desc: '一本中文版、一本无中文标记 —— 常常两本都要留' },
  E: { name: '体积差异大', desc: '同作品不同压制 / 画质 —— 留哪本看清晰度' },
  C: { name: '页数差异大', desc: '页数差 > 5 页 —— 可能是合集 / 分卷 / 重绘版' },
  D: { name: '去码 / 无码不同', desc: '标题或标签带 [Decensored] 差异' },
  MULTI: { name: '三本及以上', desc: '组内 3 本以上，可逐个移除' },
}
const GROUP_ORDER = ['F', 'A', 'E', 'C', 'D', 'MULTI']

// 分组：待处理簇（含"此前已忽略 · 有新增"的簇，它们也需要处理）
const groupedClusters = computed(() => {
  const byCat: Record<string, DedupClusterDTO[]> = {}
  for (const cl of clusters.value) {
    const cat = classify(cl)
    ;(byCat[cat] = byCat[cat] || []).push(cl)
  }
  return GROUP_ORDER.filter((k) => byCat[k]?.length).map((k) => ({
    key: k,
    meta: GROUP_META[k],
    list: byCat[k],
  }))
})

// 顶部忽略提示：命中忽略但出现"忽略后新增"的组
const newIgnoreClusters = computed(() => clusters.value.filter((c) => c.ignored))
const newIgnoreMemberCount = computed(() =>
  newIgnoreClusters.value.reduce((s, c) => s + (c.ignoredNewCount || 0), 0),
)

// ── 持久化（前端偏好）──
const DENSITY_KEY = 'saku-maintain-density'
function readDensity(): 'comfortable' | 'compact' {
  try {
    return localStorage.getItem(DENSITY_KEY) === 'compact' ? 'compact' : 'comfortable'
  } catch {
    return 'comfortable'
  }
}
const setDensity = (v: 'comfortable' | 'compact') => {
  density.value = v
  try {
    localStorage.setItem(DENSITY_KEY, v)
  } catch {
    /* 忽略 */
  }
}

// ── 数据加载 ──
const loadResult = async () => {
  try {
    const data = await http<DedupResultDTO>('/offline/maintain/result')
    items.value = data?.items || []
    clusters.value = data?.clusters || []
    resultStale.value = !!data?.stale
  } catch {
    // 结果暂未就绪，交给轮询下一轮
  }
}

// Round44：查重设置（联网复核 + 「含文件」记忆）
const loadDedupSetting = async () => {
  try {
    const s = await http<{ onlineVerify?: boolean; deleteFileDefault?: boolean }>(
      '/offline/dedup/setting',
    )
    deleteFile.value = !!s?.deleteFileDefault
  } catch {
    deleteFile.value = false
  }
}

const onDeleteFileChange = async () => {
  try {
    await http('/offline/dedup/setting', {
      method: 'POST',
      body: JSON.stringify({ deleteFileDefault: deleteFile.value }),
    })
    toast.info(
      deleteFile.value
        ? '已开启：之后的移除会同时删除本地文件（不可恢复）'
        : '已关闭：之后只移除记录，本地文件保留',
    )
  } catch (err) {
    deleteFile.value = !deleteFile.value
    toast.error(err instanceof Error ? err.message : '设置保存失败')
  }
}

// ── 任务控制（暂停 / 继续 / 取消）──
const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

const controlTask = async (action: 'pause' | 'resume') => {
  try {
    await http(`/offline/task/${action}`, { method: 'POST' })
    if (taskState.value) {
      taskState.value = { ...taskState.value, status: action === 'pause' ? 'paused' : 'running' }
    }
    toast.info(action === 'pause' ? '⏸ 已暂停，可继续或取消' : '▶ 已继续')
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || (action === 'pause' ? '暂停失败，任务可能已完成' : '继续失败'))
  }
}

const cancelTask = async () => {
  const confirmed = await modal.confirm(
    '取消后本次查重将立即终止，已完成的结果不会保留（下次需重新扫描）。\n\n确定取消吗？',
    '✖ 取消本次查重',
  )
  if (!confirmed) return
  try {
    await http('/offline/task/cancel', { method: 'POST' })
    toast.info('已请求取消，正在停止…')
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '取消失败，任务可能已完成')
  }
}

const pollProgress = () => {
  stopPolling()
  pollTimer = setInterval(async () => {
    try {
      const s = await http<OfflineTaskState>('/offline/maintain/progress')
      taskState.value = s
      if (s.status === 'success' || s.status === 'error' || s.status === 'cancelled') {
        stopPolling()
        isScanning.value = false
        if (s.status === 'error') {
          toast.error(s.error || '维护查重失败')
          return
        }
        if (s.status === 'cancelled') {
          toast.info('查重已取消')
          return
        }
        await loadResult()
      }
    } catch {
      // 网络抖动忽略，继续轮询
    }
  }, 1000)
}

const runMaintain = async (full = false) => {
  if (isScanning.value) return
  isScanning.value = true
  taskState.value = null
  moreMenuOpen.value = false
  try {
    await http<{ started: boolean }>(`/offline/maintain?full=${full}`)
    pollProgress()
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '启动查重失败，请检查后端是否运行')
    isScanning.value = false
  }
}

const runFullMaintain = async () => {
  moreMenuOpen.value = false
  const confirmed = await modal.confirm(
    '强制全量在线核对将忽略已核对标记，逐本联网抓取全部离线画廊详情页（每本约 1.2 秒限流退避），可能耗时数十分钟。确定继续吗？',
    '⚡ 强制全量在线核对',
  )
  if (!confirmed) return
  await runMaintain(true)
}

const isClearingRemoved = ref(false)
const clearRemovedAndRematch = async () => {
  moreMenuOpen.value = false
  if (isScanning.value || isClearingRemoved.value) return
  const confirmed = await modal.confirm(
    '将清除全部「已被删除/移除」标记并复位其核对时间戳，随后强制全量在线重新匹配这些画廊（可能耗时数十分钟）。\n\n仅适用于画廊已重新上传 / 源已恢复的情况。确定继续吗？',
    '🧹 清除移除标记并重新匹配',
  )
  if (!confirmed) return
  isClearingRemoved.value = true
  try {
    const data = await http<{ ok: boolean; cleared?: number }>('/offline/maintain/clear-removed', {
      method: 'POST',
    })
    const cleared = data.cleared ?? 0
    if (cleared > 0) {
      toast.success(`已清除 ${cleared} 个漫画的移除标记，开始全局重新匹配...`)
      await runMaintain(true)
    } else {
      toast.info('当前没有标记为「已删除/移除」的漫画，无需清除')
    }
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '清除移除标记失败')
  } finally {
    isClearingRemoved.value = false
  }
}

// ── 移除（主操作）／都留（忽略整组）／确认新增 ──
const doRemoveComic = async (c: OfflineComicDTO, withFile: boolean): Promise<boolean> => {
  if (isRemoving.value) return false
  const title = c.title || '未命名'
  const confirmed = await modal.confirm(
    withFile
      ? `确定移除《${title}》并同时删除其本地文件吗？\n\n📁 ${c.localPath || ''}\n\n此操作不可恢复！`
      : `确定移除《${title}》的记录吗？\n\n本地文件将保留：📁 ${c.localPath || ''}`,
    withFile ? '移除记录 + 本地文件' : '移除记录（保留文件）',
  )
  if (!confirmed) return false

  isRemoving.value = true
  removingId.value = c.id
  try {
    const data = await http<{ ok: boolean; alreadyDeleted?: boolean }>('/offline/maintain/remove', {
      method: 'POST',
      body: JSON.stringify({ comicId: c.id, deleteFile: withFile }),
    })
    if (data.alreadyDeleted) {
      toast.info(`《${title}》记录已不存在（可能已在其他设备删除），已同步刷新列表`)
    } else {
      toast.success(withFile ? `《${title}》已移除（含本地文件）` : `《${title}》记录已移除（保留本地文件）`)
    }
    return true
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '移除失败')
    return false
  } finally {
    isRemoving.value = false
    removingId.value = ''
  }
}

// 建议删除区（规则 1/2/3/4 强证据）：移除
const removeSuggested = async (item: DedupItemDTO) => {
  const ok = await doRemoveComic(item.comic, deleteFile.value)
  if (!ok) return
  processedCount.value++
  await loadResult()
}

// 成员级：移除这本（2 本组＝保留另一本；3 本组可逐个移除，剩 <2 本该组自动消失）
const removeMember = async (cl: DedupClusterDTO, m: ClusterMemberDTO) => {
  const ok = await doRemoveComic(m.comic, deleteFile.value)
  if (!ok) return
  expanded.value[cl.id] = false
  processedCount.value++
  await loadResult()
}

// 组级：都留（不再提示）＝忽略整组（可逆，可在忽略清单恢复）
const keepWholeGroup = async (cl: DedupClusterDTO) => {
  const name = cl.artist ? `${cl.titleKey}（artist:${cl.artist}）` : cl.titleKey
  const confirmed = await modal.confirm(
    `两本都留，之后不再提示与「${name}」相关的疑似重复。\n\n忽略是可逆的：随时可在「忽略清单」里恢复；若该作品之后又入库了新的本子，会自动重新提醒你。\n\n确定忽略整组吗？`,
    '🕶️ 都留（不再提示）',
  )
  if (!confirmed) return
  try {
    await http('/offline/ignore', {
      method: 'POST',
      body: JSON.stringify({ type: 'title', titleKey: cl.titleKey, artist: cl.artist || '' }),
    })
    toast.success('已忽略本组，可在「忽略清单」中恢复')
    processedCount.value++
    await loadResult()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '忽略失败')
  }
}

// 忽略项出现"忽略后新增" → 确认新增（刷新成员快照，之后继续静默）
const ackIgnoreNew = async (cl: DedupClusterDTO) => {
  if (!cl.ignoreId) return
  try {
    await http(`/offline/ignore/${cl.ignoreId}/ack`, { method: 'POST' })
    toast.success('已确认新增，该组回到静默（忽略清单里可随时恢复）')
    await loadResult()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '确认失败')
  }
}

// 规则 3（父子画廊）更新提示忽略（gid 型）
const ignoreParent = async (item: DedupItemDTO) => {
  const gid = item.comic.gid
  if (!gid) {
    toast.warning('该漫画缺少 gid，无法忽略')
    return
  }
  const confirmed = await modal.confirm(
    `忽略后，后续不再提示「${item.comic.title}」的「旧版被取代（父画廊关系）」。\n\n可在「忽略清单」中恢复。确定忽略此提示吗？`,
    '🕶️ 忽略此提示',
  )
  if (!confirmed) return
  try {
    await http('/offline/ignore', { method: 'POST', body: JSON.stringify({ type: 'gid', gid }) })
    toast.success('已忽略此更新提示')
    await loadResult()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '忽略失败')
  }
}

// ── 展示辅助 ──
const toggleGroup = (key: string) => {
  openGroups.value[key] = !openGroups.value[key]
}
const toggleExpand = (id: string) => {
  expanded.value[id] = !expanded.value[id]
}
const isExpanded = (id: string) => !!expanded.value[id]
const aliveCount = (cl: DedupClusterDTO) => cl.members.length

// 统计（进度条 / 汇总 chips）
const totalMemberCount = computed(() => clusters.value.reduce((s, c) => s + c.members.length, 0))
const highCount = computed(() => clusters.value.filter((c) => c.confidence === 'high').length)
const mediumCount = computed(() => clusters.value.filter((c) => c.confidence === 'medium').length)
// 进度：结果里只含"当前待处理"项，故以「本次会话已处理 + 当前待处理」为分母
const progressWidth = computed(() => {
  const total = clusters.value.length + processedCount.value
  if (total <= 0) return '0%'
  return `${Math.round((processedCount.value / total) * 100)}%`
})

// 成员相对差异（展开对比的面板头）
// 注意：多本组（≥3 本）时不能再用 members[1-mi]（mi=2 会取到 -1），统一与「成员 A」比较。
const memberDiff = (cl: DedupClusterDTO, mi: number) => {
  if (cl.members.length < 2) return ''
  const refIdx = mi === 0 ? 1 : 0
  const label = String.fromCharCode(65 + refIdx)
  const d = (cl.members[mi].pageCount || 0) - (cl.members[refIdx].pageCount || 0)
  if (d === 0) return `页数与成员 ${label} 相同`
  return d > 0 ? `比成员 ${label} 多 ${d} 页` : `比成员 ${label} 少 ${-d} 页`
}
const onCoverError = (id: string) => {
  coverFailed.value[id] = true
}
const openComic = (id: string) => openComicDetailInNewTab({ id, source: 'offline' })

const formatBytes = (bytes?: number) => {
  if (!bytes || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${units[i]}`
}
const formatDate = (iso?: string) => {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString('zh-CN', { hour12: false })
}
const modeText = (mode?: string) => (mode === 'gallery' ? '📁 画廊' : '🗜️ 归档')

const closeMenu = () => {
  moreMenuOpen.value = false
}

// ── 进入页面：拉结果 + 任务状态（不再自动扫描）──
const syncStaleOnEnter = async () => {
  if (isScanning.value) return
  try {
    const st = await http<UnsyncedStatusDTO>('/offline/maintain/unsynced')
    await loadResult()
    if (st.hasUnsynced) resultStale.value = true
  } catch {
    // 状态接口异常（如后端未启动），忽略
  }
}

const refreshOnEnter = async () => {
  await loadResult()
  try {
    const s = await http<OfflineTaskState>('/offline/maintain/progress')
    taskState.value = s
    if (s.status === 'running' || s.status === 'paused') {
      isScanning.value = true
      pollProgress()
      return
    }
    await syncStaleOnEnter()
  } catch {
    // 进度接口异常，页面保持空态
  }
}

let activatedOnce = false
onMounted(() => {
  void loadDedupSetting()
  void refreshOnEnter()
  window.addEventListener('click', closeMenu)
})
onActivated(() => {
  if (activatedOnce) {
    void loadDedupSetting()
    void refreshOnEnter()
  }
  activatedOnce = true
})
onUnmounted(() => {
  stopPolling()
  window.removeEventListener('click', closeMenu)
})
</script>

<template>
  <div class="maintenance-page" @click="closeMenu">
    <div class="page-header">
      <div>
        <h2 class="page-title">🛠️ 本地书库维护</h2>
        <p class="subtitle">
          扫描本地书库查重：只给建议，移除与忽略都由你确认；疑似重复按「两本差在哪」分组
        </p>
      </div>
      <div class="header-actions">
        <button
          class="scan-btn primary"
          :disabled="isScanning || isClearingRemoved"
          @click="runMaintain(false)"
        >
          {{ isScanning ? '⏳ 扫描中...' : '🔍 重新扫描' }}
        </button>
        <div class="menu-wrap">
          <button class="scan-btn ghost" @click.stop="moreMenuOpen = !moreMenuOpen">⋯ 更多</button>
          <div v-if="moreMenuOpen" class="more-menu" @click.stop>
            <button :disabled="isScanning || isClearingRemoved" @click="runFullMaintain">
              ⚡ 强制全量在线核对
              <small>忽略已核对标记，逐本联网，可能数十分钟</small>
            </button>
            <button :disabled="isScanning || isClearingRemoved" @click="clearRemovedAndRematch">
              🧹 清除移除标记并重新匹配
              <small>画廊已重新上传 / 更换源时使用</small>
            </button>
            <router-link to="/settings?tab=dedup" class="menu-link">
              ⚙️ 查重设置
              <small>联网复核（E 站反查）等，已收进设置页</small>
            </router-link>
          </div>
        </div>
      </div>
    </div>

    <!-- 顶部：忽略项出现「忽略后新增」的提醒（平时不占位） -->
    <div v-if="newIgnoreClusters.length > 0" class="alert-bar">
      <span class="alert-icon">🕶️</span>
      <div class="alert-text">
        <b>{{ newIgnoreClusters.length }}</b> 条忽略项下出现了忽略之后新入库的本子（共
        <b>{{ newIgnoreMemberCount }}</b> 本），可能构成新的重复 —— 已忽略的内容不会一直占位，只在有变化时提醒你。
      </div>
      <router-link class="action-btn ghost-gray" to="/offline/ignore">去忽略清单查看 →</router-link>
    </div>

    <!-- 结果过期提示 -->
    <div v-if="resultStale" class="stale-banner">
      <span class="stale-icon">⚠️</span>
      <div class="stale-info">
        <p class="stale-title">查重结果已过期</p>
        <p class="stale-sub">
          本地书库已发生变化（新下载 / 更新 / 删除），当前列表不再准确。请手动点击「重新扫描」获取最新结果。
        </p>
      </div>
      <button class="scan-btn primary" :disabled="isScanning" @click="runMaintain(false)">
        🔍 重新扫描
      </button>
    </div>

    <!-- 扫描进度 -->
    <div v-if="isScanning" class="scanning-banner" :class="{ 'is-paused': isTaskPaused }">
      <span v-if="!isTaskPaused" class="spinner"></span>
      <span v-else class="paused-icon">⏸</span>
      <div class="scanning-info">
        <p class="scanning-title">
          <template v-if="isTaskPaused">查重已暂停</template>
          <template v-else>{{ phaseText || '正在扫描本地书库查重...' }}</template>
          <span v-if="taskState && taskState.total > 0" class="scanning-percent"
            >{{ progressPercent }}%</span
          >
        </p>
        <div v-if="taskState && taskState.total > 0" class="progress-track">
          <div class="progress-fill" :style="{ width: progressPercent + '%' }"></div>
        </div>
        <p class="scanning-sub">
          <template v-if="isTaskPaused">
            已暂停于 {{ taskState?.done }} / {{ taskState?.total }} · {{ phaseText }}
          </template>
          <template v-else-if="taskState && taskState.total > 0">
            进度 {{ taskState.done }} / {{ taskState.total }} · {{ phaseText }}
          </template>
          <template v-else>正在启动维护任务...</template>
        </p>
        <p v-if="currentTitle" class="scanning-current">📖 {{ currentTitle }}</p>
      </div>
      <div class="banner-actions">
        <template v-if="isTaskPaused">
          <button class="control-btn resume" @click="controlTask('resume')">▶ 继续</button>
          <button class="control-btn cancel" @click="cancelTask">✖ 取消</button>
        </template>
        <button v-else-if="isTaskRunning" class="control-btn pause" @click="controlTask('pause')">
          ⏸ 暂停
        </button>
      </div>
    </div>

    <template v-else>
      <!-- 待办进度 + 视图密度 + 统计 + 含文件勾选 -->
      <div class="progress-card">
        <div class="progress-top">
          <span class="progress-num">待处理 <b>{{ clusters.length }}</b> 组</span>
          <span class="progress-meta">
            {{ totalMemberCount }} 本 · 建议删除 {{ removeItems.length }} 项
          </span>
          <span class="spacer"></span>
          <div class="seg">
            <button :class="{ on: density === 'comfortable' }" @click="setDensity('comfortable')">
              舒适
            </button>
            <button :class="{ on: density === 'compact' }" @click="setDensity('compact')">
              紧凑
            </button>
          </div>
          <div class="stat-chips">
            <span class="chip high"><i class="dot"></i>高置信 {{ highCount }}</span>
            <span class="chip medium"><i class="dot"></i>中置信 {{ mediumCount }}</span>
            <router-link class="chip link" to="/offline/ignore">🕶️ 忽略清单 →</router-link>
          </div>
        </div>
        <div class="track"><i :style="{ width: progressWidth }"></i></div>
        <div class="progress-foot">
          <label class="checkline" title="页面级一次设定：之后每一次「移除这本」都按这个选择执行">
            <input type="checkbox" v-model="deleteFile" @change="onDeleteFileChange" />
            <span>移除时删除本地文件</span>
            <span class="hint">默认不勾；记住上次选择</span>
          </label>
          <span class="spacer"></span>
          <span class="hint dim">不勾选＝只移除记录，本地文件保留（可随时重新扫描回来）</span>
        </div>
      </div>

      <!-- 空态 -->
      <div
        v-if="clusters.length === 0 && removeItems.length === 0"
        class="empty-box"
      >
        <span class="icon">{{ resultStale ? '🔄' : '🎉' }}</span>
        <p class="empty-title">
          {{ resultStale ? '查重结果已过期，请重新扫描' : '恭喜！本地画库暂无重复或异常项' }}
        </p>
        <p class="empty-sub">
          {{
            resultStale
              ? '本地书库可能已发生变化（如已在其他设备删除），重新扫描可获取最新一致的结果。'
              : '若刚导入新内容，可点击「重新扫描」再次核对。'
          }}
        </p>
      </div>

      <template v-else>
        <!-- 建议删除（强证据；仅在有内容时出现）-->
        <section v-if="removeItems.length > 0" class="group open">
          <div class="group-head" @click="toggleGroup('REMOVE')">
            <span class="arrow">▶</span>
            <span class="group-name">🗑️ 建议删除</span>
            <span class="group-count">{{ removeItems.length }} 项</span>
            <span class="group-desc">同 GID / 归档 Hash / 父子画廊等强证据规则判定</span>
          </div>
          <div v-show="openGroups['REMOVE'] !== false" class="group-body">            <div v-for="item in removeItems" :key="item.comic.id" class="card danger">
              <div class="card-head">
                <span class="card-key" :title="item.comic.title">{{ item.comic.title }}</span>
                <span class="mode-chip">{{ modeText(item.comic.sourceMode) }}</span>
                <span class="spacer"></span>
                <span class="reason-chip">⚠️ {{ item.reason }}</span>
              </div>
              <div class="member-meta">
                <span>📄 {{ item.comic.pageCount || 0 }} 页</span>
                <span>💾 {{ formatBytes(item.comic.fileSize) }}</span>
                <span>🕒 {{ formatDate(item.comic.updatedAt) }}</span>
              </div>
              <div class="member-path">📁 {{ item.comic.localPath || '—' }}</div>
              <div class="card-actions">
                <span class="spacer"></span>
                <button
                  v-if="item.rule === 'parent'"
                  class="action-btn ghost-gray"
                  :disabled="isRemoving"
                  title="忽略此更新提示（父画廊 gid），后续不再提示「旧版可删除」"
                  @click="ignoreParent(item)"
                >
                  🕶️ 忽略此提示
                </button>
                <button
                  class="action-btn danger-soft"
                  :disabled="isRemoving"
                  @click="removeSuggested(item)"
                >
                  {{ isRemoving && removingId === item.comic.id ? '⏳ 处理中...' : '🗑️ 移除这本' }}
                </button>
              </div>
            </div>
          </div>
        </section>

        <!-- 疑似重复：按差异模式分组折叠 -->
        <section
          v-for="g in groupedClusters"
          :key="g.key"
          class="group"
          :class="{ open: openGroups[g.key] }"
        >
          <div class="group-head" @click="toggleGroup(g.key)">
            <span class="arrow">▶</span>
            <span class="group-name">🔎 {{ g.meta.name }}</span>
            <span class="group-count">{{ g.list.length }} 组</span>
            <span class="group-desc">{{ g.meta.desc }}</span>
          </div>
          <div v-show="openGroups[g.key]" class="group-body">
            <div v-for="cl in g.list" :key="cl.id" class="card suspect" :class="{ ignored: cl.ignored }">
              <div class="card-head">
                <span class="card-key" :title="cl.titleKey">{{ cl.titleKey }}</span>
                <span class="card-artist">artist: {{ cl.artist || 'null' }}</span>
                <span class="conf" :class="cl.confidence">
                  {{ cl.confidence === 'high' ? '高置信' : '中置信' }}
                </span>
                <span v-if="cl.ignored" class="ignored-badge">
                  此前已忽略 · 新增 {{ cl.ignoredNewCount }} 本
                </span>
                <span class="spacer"></span>
                <div class="diffs">
                  <span v-for="c in diffChips(cl)" :key="c" :class="chipClass(c)">{{ c }}</span>
                </div>
              </div>

              <!-- 紧凑视图：一行一组 -->
              <template v-if="density === 'compact'">
                <div class="crow-line">
                  <div class="crow-main">
                    <div class="declared-hint">
                      {{ aliveCount(cl) === 2 ? '移除其中一本 ＝ 保留另一本' : `组内 ${aliveCount(cl)} 本 · 可逐个移除` }}
                    </div>
                  </div>
                  <div class="crow-actions">
                    <button
                      v-for="(m, mi) in cl.members"
                      :key="m.comic.id"
                      class="action-btn danger-soft sm"
                      :disabled="isRemoving"
                      :title="`移除《${m.comic.title}》（其余本子保留）`"
                      @click.stop="removeMember(cl, m)"
                    >
                      {{ isRemoving && removingId === m.comic.id ? '⏳' : `🗑️ ${String.fromCharCode(65 + mi)}` }}
                    </button>
                    <button class="action-btn ghost-gray sm" @click.stop="keepWholeGroup(cl)" title="两本都留（忽略本组）">
                      🕶️ 都留
                    </button>
                    <button class="action-btn ghost-teal sm" @click.stop="toggleExpand(cl.id)">
                      {{ isExpanded(cl.id) ? '▴ 收起' : '🔍 对比' }}
                    </button>
                    <button v-if="cl.ignored" class="action-btn ghost-gray sm" @click.stop="ackIgnoreNew(cl)">
                      ✔ 确认新增
                    </button>
                  </div>
                </div>
                <div v-show="isExpanded(cl.id)" class="inline-compare">
                  <div class="diffline">这两本差在哪：<b>{{ diffChips(cl).join(' · ') }}</b></div>
                  <div class="compare">
                    <div v-for="(m, mi) in cl.members" :key="m.comic.id" class="panel">
                      <div class="panel-head">
                        <span class="panel-badge info">成员 {{ String.fromCharCode(65 + mi) }}</span>
                        <span class="panel-diff">{{ memberDiff(cl, mi) }}</span>
                      </div>
                      <div class="panel-top">
                        <div class="cover lg" @click="openComic(m.comic.id)">
                          <img
                            v-if="m.comic.coverUrl && !coverFailed[m.comic.id]"
                            :src="m.comic.coverUrl"
                            :alt="m.comic.title"
                            loading="lazy"
                            @error="onCoverError(m.comic.id)"
                          />
                          <span v-else class="cover-fallback">{{ (cl.titleKey || '?').slice(0, 1) }}</span>
                        </div>
                        <div class="member-main">
                          <div class="panel-title" @click="openComic(m.comic.id)">{{ m.comic.title }}</div>
                          <div class="member-meta">
                            <span class="lang-chip" :class="{ 'is-null': !m.lang }">{{ m.lang || 'null' }}</span>
                            <span>📄 {{ m.pageCount }} 页</span>
                            <span>💾 {{ formatBytes(m.comic.fileSize) }}</span>
                            <span>artist: {{ m.artist || 'null' }}</span>
                          </div>
                          <div class="member-path">📁 {{ m.comic.localPath || '—' }}</div>
                        </div>
                      </div>
                      <button class="action-btn danger-soft" :disabled="isRemoving" @click="removeMember(cl, m)">
                        {{ isRemoving && removingId === m.comic.id ? '⏳ 处理中...' : '🗑️ 移除这本' }}
                      </button>
                    </div>
                  </div>
                </div>
              </template>

              <!-- 舒适视图：成员卡并排 -->
              <template v-else>
                <div class="members">
                  <div v-for="m in cl.members" :key="m.comic.id" class="member">
                    <div class="member-cover" @click="openComic(m.comic.id)">
                      <img
                        v-if="m.comic.coverUrl && !coverFailed[m.comic.id]"
                        :src="m.comic.coverUrl"
                        :alt="m.comic.title"
                        loading="lazy"
                        @error="onCoverError(m.comic.id)"
                      />
                      <span v-else class="cover-fallback">{{ (cl.titleKey || '?').slice(0, 1) }}</span>
                    </div>
                    <div class="member-main">
                      <div class="member-title" :title="m.comic.title" @click="openComic(m.comic.id)">
                        {{ m.comic.title || '（无标题）' }}
                      </div>
                      <div class="member-meta">
                        <span class="lang-chip" :class="{ 'is-null': !m.lang }">{{ m.lang || 'null' }}</span>
                        <span>{{ m.pageCount || 0 }} 页</span>
                        <span>💾 {{ formatBytes(m.comic.fileSize) }}</span>
                      </div>
                      <div class="member-path">📁 {{ m.comic.localPath || '—' }}</div>
                      <div class="member-actions">
                        <button
                          class="action-btn danger-soft sm"
                          :disabled="isRemoving"
                          :title="`仅移除这一本，组内其余本子照常保留`"
                          @click="removeMember(cl, m)"
                        >
                          {{ isRemoving && removingId === m.comic.id ? '⏳ 处理中...' : '🗑️ 移除这本' }}
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
                <div class="card-actions">
                  <span class="hint dim">
                    {{
                      aliveCount(cl) === 2
                        ? '移除其中一本 ＝ 保留另一本'
                        : `组内 ${aliveCount(cl)} 本 · 可逐个移除（移除后仍 ≥2 本会继续挂着）`
                    }}
                  </span>
                  <span class="spacer"></span>
                  <button class="action-btn ghost-teal" @click="toggleExpand(cl.id)">
                    {{ isExpanded(cl.id) ? '▴ 收起对比' : '🔍 展开对比' }}
                  </button>
                  <button
                    v-if="cl.ignored"
                    class="action-btn ghost-gray"
                    title="把新入库的本子纳入「已知」，之后继续静默"
                    @click="ackIgnoreNew(cl)"
                  >
                    ✔ 确认新增
                  </button>
                  <button class="action-btn ghost-gray" @click="keepWholeGroup(cl)">
                    🕶️ 都留（不再提示）
                  </button>
                </div>
                <div v-show="isExpanded(cl.id)" class="inline-compare">
                  <div class="diffline">这两本差在哪：<b>{{ diffChips(cl).join(' · ') }}</b></div>
                  <div class="compare">
                    <div v-for="(m, mi) in cl.members" :key="m.comic.id" class="panel">
                      <div class="panel-head">
                        <span class="panel-badge info">成员 {{ String.fromCharCode(65 + mi) }}</span>
                        <span class="panel-diff">{{ memberDiff(cl, mi) }}</span>
                      </div>
                      <div class="panel-top">
                        <div class="cover lg" @click="openComic(m.comic.id)">
                          <img
                            v-if="m.comic.coverUrl && !coverFailed[m.comic.id]"
                            :src="m.comic.coverUrl"
                            :alt="m.comic.title"
                            loading="lazy"
                            @error="onCoverError(m.comic.id)"
                          />
                          <span v-else class="cover-fallback">{{ (cl.titleKey || '?').slice(0, 1) }}</span>
                        </div>
                        <div class="member-main">
                          <div class="panel-title" @click="openComic(m.comic.id)">{{ m.comic.title }}</div>
                          <div class="member-meta">
                            <span class="lang-chip" :class="{ 'is-null': !m.lang }">{{ m.lang || 'null' }}</span>
                            <span>📄 {{ m.pageCount }} 页</span>
                            <span>💾 {{ formatBytes(m.comic.fileSize) }}</span>
                            <span>artist: {{ m.artist || 'null' }}</span>
                          </div>
                          <div class="member-path">📁 {{ m.comic.localPath || '—' }}</div>
                        </div>
                      </div>
                      <button class="action-btn danger-soft" :disabled="isRemoving" @click="removeMember(cl, m)">
                        {{ isRemoving && removingId === m.comic.id ? '⏳ 处理中...' : '🗑️ 移除这本' }}
                      </button>
                    </div>
                  </div>
                  <p class="hint dim">移除前会二次确认；是否连本地文件一起删由上方勾选决定</p>
                </div>
              </template>
            </div>
          </div>
        </section>

        <!-- 建议保留：无需操作的信息，默认收起 -->
        <div v-if="keepItems.length > 0" class="fold">
          <details>
            <summary>✔ 建议保留 {{ keepItems.length }} 项 —— 无需操作，点开查看</summary>
            <div class="fold-body">
              <div v-for="item in keepItems" :key="item.comic.id" class="keep-row">
                <span class="keep-title" @click="openComic(item.comic.id)">{{ item.comic.title }}</span>
                <span class="hint dim">{{ item.reason }}</span>
              </div>
            </div>
          </details>
        </div>

        <!-- 已忽略：主列表不占位，只在独立页可见 -->
        <div class="fold flat">
          <span>🕶️</span>
          <span>已忽略的作品移到 <router-link to="/offline/ignore">忽略清单</router-link> 独立页 —— 有新增本子时会自动回到这里提醒</span>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
/* ─────────────────────────────────────────────────────────────
   Round44 维护页 v3 样式（布局对齐 Test/mockups 样稿）
   ───────────────────────────────────────────────────────────── */
.maintenance-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
}
.page-title {
  margin: 0;
  font-size: 1.3rem;
  color: var(--app-text-strong);
}
.subtitle {
  margin: 4px 0 0;
  font-size: 0.8rem;
  color: var(--app-text-3);
}
.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 更多菜单 */
.menu-wrap {
  position: relative;
}
.more-menu {
  position: absolute;
  right: 0;
  top: calc(100% + 6px);
  min-width: 250px;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
  padding: 6px;
  z-index: 30;
}
.more-menu button,
.more-menu .menu-link {
  display: block;
  width: 100%;
  text-align: left;
  background: none;
  border: none;
  color: var(--app-fg);
  font-family: inherit;
  font-size: 0.82rem;
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  text-decoration: none;
}
.more-menu button:hover,
.more-menu .menu-link:hover {
  background: var(--app-surface-3);
}
.more-menu button:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.more-menu small {
  display: block;
  color: var(--app-text-muted);
  font-size: 0.72rem;
  margin-top: 2px;
}

/* 按钮 */
.scan-btn {
  border: 1px solid transparent;
  border-radius: 6px;
  padding: 7px 14px;
  font-size: 0.82rem;
  font-family: inherit;
  cursor: pointer;
  background: #007acc;
  color: #fff;
  transition: all 0.15s;
}
.scan-btn:hover {
  opacity: 0.9;
}
.scan-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.scan-btn.ghost {
  background: transparent;
  border-color: var(--app-border-3);
  color: var(--app-text-2);
}
.scan-btn.ghost:hover {
  border-color: #007acc;
  color: #7ec8ff;
  opacity: 1;
}
.action-btn {
  border: none;
  border-radius: 6px;
  padding: 6px 12px;
  font-size: 0.78rem;
  font-family: inherit;
  cursor: pointer;
  background: var(--app-surface-3);
  color: var(--app-fg);
  transition: all 0.15s;
  white-space: nowrap;
}
.action-btn:hover {
  background: var(--app-surface-3-hover);
}
.action-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.action-btn.sm {
  padding: 3px 9px;
  font-size: 0.74rem;
}
.action-btn.danger-soft {
  background: #5a3a3a;
  color: #ffc9d1;
}
.action-btn.ghost-gray {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  text-decoration: none;
}
.action-btn.ghost-gray:hover {
  border-color: var(--app-text-3);
  color: var(--app-text-strong);
}
.action-btn.ghost-teal {
  background: transparent;
  border: 1px solid #1d5148;
  color: #00c2a8;
}
.action-btn.ghost-teal:hover {
  background: #0f2622;
}

/* 顶部忽略新增提示 */
.alert-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  background: #2a2414;
  border: 1px solid rgba(245, 158, 11, 0.4);
  border-left: 3px solid #f59e0b;
  border-radius: 6px;
  padding: 10px 14px;
  font-size: 0.82rem;
  color: #f5d08a;
}
.alert-text {
  flex: 1;
  min-width: 0;
}
.alert-text b {
  color: #ffd98a;
}

/* 过期提示 */
.stale-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  background: #2a2414;
  border: 1px solid rgba(245, 158, 11, 0.4);
  border-left: 3px solid #f59e0b;
  border-radius: 6px;
  padding: 12px 14px;
}
.stale-info {
  flex: 1;
  min-width: 0;
}
.stale-title {
  margin: 0;
  font-size: 0.9rem;
  color: #ffd98a;
}
.stale-sub {
  margin: 3px 0 0;
  font-size: 0.78rem;
  color: #c9b184;
}

/* 扫描进度 */
.scanning-banner {
  display: flex;
  align-items: center;
  gap: 14px;
  background: var(--app-surface);
  border: 1px solid #007acc;
  border-radius: 8px;
  padding: 14px 16px;
}
.scanning-banner.is-paused {
  border-color: #f59e0b;
}
.scanning-info {
  flex: 1;
  min-width: 0;
}
.scanning-title {
  margin: 0;
  font-size: 0.9rem;
  color: var(--app-text-strong);
}
.scanning-percent {
  margin-left: 8px;
  color: #7ec8ff;
  font-weight: 700;
}
.scanning-sub {
  margin: 4px 0 0;
  font-size: 0.76rem;
  color: var(--app-text-3);
}
.scanning-current {
  margin: 4px 0 0;
  font-size: 0.76rem;
  color: var(--app-text-2);
}
.progress-track {
  height: 6px;
  background: var(--app-surface-3);
  border-radius: 999px;
  overflow: hidden;
  margin-top: 8px;
}
.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #007acc, #00c2a8);
  transition: width 0.3s;
}
.banner-actions {
  display: flex;
  gap: 8px;
}
.control-btn {
  border: none;
  border-radius: 6px;
  padding: 6px 12px;
  font-size: 0.78rem;
  font-family: inherit;
  cursor: pointer;
  color: #fff;
  background: var(--app-surface-3);
}
.control-btn.pause {
  background: #f59e0b;
  color: #2a2414;
}
.control-btn.resume {
  background: #00c2a8;
  color: #06231f;
}
.control-btn.cancel {
  background: #5a3a3a;
  color: #ffc9d1;
}
.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.25);
  border-top-color: #007acc;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  flex: 0 0 auto;
}
.paused-icon {
  font-size: 1.1rem;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* 待办进度卡 */
.progress-card {
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.progress-top {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.progress-num {
  font-size: 1rem;
  color: var(--app-text-strong);
}
.progress-num b {
  color: #7ec8ff;
  font-size: 1.3rem;
}
.progress-meta {
  font-size: 0.78rem;
  color: var(--app-text-3);
}
.progress-top .spacer,
.card-actions .spacer,
.progress-foot .spacer {
  flex: 1;
}
.seg {
  display: inline-flex;
  background: var(--app-surface-3);
  border-radius: 6px;
  padding: 2px;
  gap: 2px;
}
.seg button {
  border: none;
  background: none;
  color: var(--app-text-3);
  font-family: inherit;
  font-size: 0.75rem;
  padding: 3px 10px;
  border-radius: 4px;
  cursor: pointer;
}
.seg button.on {
  background: #007acc;
  color: #fff;
}
.stat-chips {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  border-radius: 999px;
  padding: 2px 10px;
  font-size: 0.74rem;
  color: var(--app-text-2);
  text-decoration: none;
}
.chip.high {
  color: #7ec8ff;
  border-color: #2b4a63;
  background: #14283a;
}
.chip.medium {
  color: #f5d08a;
  border-color: #4a3c14;
  background: #2a2414;
}
.chip .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}
.chip.link:hover {
  border-color: #007acc;
  color: #7ec8ff;
}
.track {
  height: 6px;
  background: var(--app-surface-3);
  border-radius: 999px;
  overflow: hidden;
}
.track > i {
  display: block;
  height: 100%;
  background: linear-gradient(90deg, #007acc, #00c2a8);
  transition: width 0.3s;
}
.progress-foot {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.checkline {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 0.76rem;
  color: var(--app-text-2);
  cursor: pointer;
  user-select: none;
}
.checkline input {
  accent-color: #ff7588;
  width: 14px;
  height: 14px;
}
.hint {
  font-size: 0.74rem;
}
.hint.dim {
  color: var(--app-text-muted);
}

/* 分组 */
.group {
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  overflow: hidden;
}
.group.open .group-head .arrow {
  transform: rotate(90deg);
}
.group-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  cursor: pointer;
  background: var(--app-surface-2);
}
.group-head:hover {
  background: var(--app-surface-3);
}
.group-head .arrow {
  color: var(--app-text-3);
  font-size: 0.7rem;
  transition: transform 0.15s;
}
.group-name {
  font-size: 0.92rem;
  color: var(--app-text-strong);
  white-space: nowrap;
}
.group-count {
  background: var(--app-surface-3);
  border-radius: 999px;
  padding: 1px 9px;
  font-size: 0.74rem;
  color: var(--app-text-2);
}
.group-desc {
  flex: 1;
  color: var(--app-text-muted);
  font-size: 0.76rem;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.group-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
}

/* 组卡片 */
.card {
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  background: var(--app-surface-2);
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.card.suspect {
  border-left: 3px solid #00c2a8;
}
.card.suspect.ignored {
  border-left-color: #f59e0b;
}
.card.danger {
  border-left: 3px solid #ff7588;
}
.card-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.card-key {
  font-size: 0.88rem;
  color: var(--app-text-strong);
  max-width: 46ch;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.card-artist {
  font-size: 0.76rem;
  color: var(--app-text-3);
}
.conf {
  border-radius: 999px;
  padding: 1px 9px;
  font-size: 0.72rem;
  border: 1px solid transparent;
  white-space: nowrap;
}
.conf.high {
  color: #00c2a8;
  border-color: #1d5148;
  background: #0f2622;
}
.conf.medium {
  color: #f5d08a;
  border-color: #4a3c14;
  background: #2a2414;
}
.ignored-badge {
  border-radius: 999px;
  padding: 1px 9px;
  font-size: 0.72rem;
  color: #ffd98a;
  border: 1px solid #4a3c14;
  background: #2a2414;
  white-space: nowrap;
}
.reason-chip {
  font-size: 0.74rem;
  color: #f0d9a8;
  background: #2a2414;
  border-radius: 6px;
  padding: 2px 8px;
}
.mode-chip {
  font-size: 0.72rem;
  color: var(--app-text-3);
  border: 1px solid var(--app-border-3);
  border-radius: 999px;
  padding: 1px 8px;
}
.diffs {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.diff {
  font-size: 0.72rem;
  border-radius: 6px;
  padding: 2px 8px;
  background: #14283a;
  border: 1px solid #24425c;
  color: #9bb6c8;
  white-space: nowrap;
}
.diff.lang {
  background: #2a2414;
  border-color: #4a3c14;
  color: #f5d08a;
}
.diff.size {
  background: #241a2a;
  border-color: #46305c;
  color: #cfb0e8;
}
.diff.page {
  background: #14281a;
  border-color: #24502c;
  color: #9fd8a6;
}

/* 成员卡（舒适视图） */
.members {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.member {
  flex: 1 1 280px;
  min-width: 0;
  display: flex;
  gap: 10px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 10px;
}
.member-cover {
  width: 54px;
  height: 72px;
  border-radius: 6px;
  flex: 0 0 auto;
  overflow: hidden;
  cursor: pointer;
  background: var(--app-surface-4);
  display: flex;
  align-items: center;
  justify-content: center;
}
.member-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.cover.lg {
  width: 132px;
  height: 176px;
  border-radius: 6px;
  flex: 0 0 auto;
  overflow: hidden;
  cursor: pointer;
  background: var(--app-surface-3);
  display: flex;
  align-items: center;
  justify-content: center;
}
.cover.lg img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.cover-fallback {
  font-size: 1.2rem;
  color: var(--app-text-3);
}
.member-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.member-title {
  font-size: 0.82rem;
  color: var(--app-text-strong);
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  cursor: pointer;
}
.panel-title {
  font-size: 0.85rem;
  color: var(--app-text-strong);
  line-height: 1.4;
  cursor: pointer;
}
.member-meta {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 0.72rem;
  color: var(--app-text-3);
}
.lang-chip {
  border: 1px dashed var(--app-border-3);
  border-radius: 6px;
  padding: 0 6px;
  max-width: 96px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lang-chip.is-null {
  color: var(--app-text-muted);
}
.member-path {
  font-size: 0.7rem;
  color: var(--app-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.member-actions {
  margin-top: 2px;
}
.card-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

/* 紧凑视图 */
.crow-line {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.crow-main {
  flex: 1;
  min-width: 0;
}
.declared-hint {
  font-size: 0.74rem;
  color: var(--app-text-muted);
}
.crow-actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

/* 就地展开对比 */
.inline-compare {
  display: flex;
  flex-direction: column;
  gap: 10px;
  border-top: 1px dashed var(--app-border-3);
  padding-top: 10px;
}
.diffline {
  background: #14283a;
  border: 1px solid #24425c;
  border-radius: 6px;
  padding: 7px 11px;
  font-size: 0.78rem;
  color: #a9c8dd;
}
.diffline b {
  color: #7ec8ff;
}
.compare {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 12px;
}
.panel {
  background: var(--app-surface);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.panel-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.panel-badge {
  border-radius: 999px;
  font-size: 0.72rem;
  padding: 1px 9px;
  color: #7ec8ff;
  border: 1px solid #2b4a63;
  background: #14283a;
}
.panel-diff {
  font-size: 0.74rem;
  color: var(--app-text-3);
}
.panel-top {
  display: flex;
  gap: 12px;
}

/* 折叠区 */
.fold {
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  padding: 10px 14px;
  font-size: 0.82rem;
  color: var(--app-text-3);
  display: flex;
  gap: 8px;
  align-items: center;
}
.fold.flat {
  background: transparent;
}
.fold a {
  color: #7ec8ff;
}
.fold summary {
  cursor: pointer;
}
.fold-body {
  margin-top: 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.keep-row {
  display: flex;
  gap: 10px;
  align-items: baseline;
  font-size: 0.78rem;
}
.keep-title {
  color: var(--app-text-2);
  cursor: pointer;
  max-width: 46ch;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 空态 */
.empty-box {
  text-align: center;
  padding: 56px 20px;
  color: var(--app-text-3);
}
.empty-box .icon {
  font-size: 2.4rem;
}
.empty-title {
  margin: 12px 0 6px;
  color: var(--app-text-strong);
  font-size: 1.05rem;
}
.empty-sub {
  margin: 0;
  font-size: 0.8rem;
}

/* ── 窄屏（手机 / PWA 可用性）：成员竖排 + 对比上下堆叠 + 按钮全宽 ── */
@media (max-width: 720px) {
  .maintenance-page {
    padding: 12px;
    gap: 12px;
  }
  .page-header {
    flex-direction: column;
    align-items: stretch;
  }
  .header-actions {
    justify-content: flex-end;
  }
  .compare {
    grid-template-columns: minmax(0, 1fr);
  }
  .members {
    flex-direction: column;
  }
  .member {
    flex: 1 1 auto;
    flex-wrap: wrap;
  }
  .member-main {
    min-width: 140px;
  }
  .member-actions {
    width: 100%;
  }
  .member-actions .action-btn {
    width: 100%;
  }
  .panel-top {
    flex-wrap: wrap;
  }
  .cover.lg {
    width: 96px;
    height: 128px;
  }
  .card-key {
    max-width: 100%;
    white-space: normal;
  }
  .group-desc {
    display: none;
  }
  .crow-actions {
    width: 100%;
    justify-content: flex-end;
  }
  .crow-actions .action-btn {
    flex: 1 1 auto;
  }
}
</style>
