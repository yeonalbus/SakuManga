<script setup lang="ts">
import { ref, computed, onMounted, onActivated, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import { openComicDetailInNewTab } from '@/utils/detailNav'

const { modal, toast } = useUI()
const router = useRouter()

// 后端 GET /offline/maintain 返回的离线漫画 DTO
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
}

interface DedupItemDTO {
  comic: OfflineComicDTO
  reason: string // 重复原因
  keep: boolean // true=建议保留，false=建议删除
  rule?: string // Round26 O2：gid | hash | parent | signature（仅 parent 可忽略）
}

// Round26 O3：疑似重复组（名称级弱证据，只建议）
interface ClusterMemberDTO {
  comic: OfflineComicDTO
  pageCount: number
  lang?: string
}
interface DedupClusterDTO {
  id: string
  titleKey: string
  artist?: string
  confidence: 'high' | 'medium'
  reason: string
  members: ClusterMemberDTO[]
  ignored?: boolean // 全量核对时命中忽略（增量已跳过，不会返回）
}

// Round26 O2：忽略清单条目
interface IgnoreItemDTO {
  id: string
  type: 'title' | 'gid' | 'comic'
  titleKey?: string
  artist?: string
  gid?: string
  comicId?: string
  comicTitle?: string // comic 型：被忽略漫画的标题（后端补查）
  note?: string
  createdAt: string
}

interface DedupResultDTO {
  items: DedupItemDTO[]
  clusters?: DedupClusterDTO[] // Round26 O3
  finishedAt?: number // 结果生成时间戳(ms)
  stale?: boolean // 结果是否已过期（删除操作后置 true，提示重新扫描）
}

// 需求4（⑨ 改造）：书库变更与查重结果的同步状态（进入维护界面时仅用于「过期提示」，不再自动触发增量查重）
interface UnsyncedStatusDTO {
  lastLibraryChange: number // 书库最近一次变更时间戳(ms)
  resultFinishedAt: number // 最近一次查重结果生成时间戳(ms)
  hasUnsynced: boolean // 是否存在尚未反映到查重结果的变更
}

const items = ref<DedupItemDTO[]>([])
const resultStale = ref(false) // 幽灵文件修复：结果过期标记（跨设备删除后旧缓存不可信）
const isScanning = ref(false)
const isRemoving = ref(false)
const removingId = ref('')
const coverFailed = ref<Record<string, boolean>>({})
// 批量删除：多选“建议删除”项后一次提交，避免反复“删除→刷新”
const selectedIds = ref<string[]>([])

// ── Round26 O2/O3：疑似重复簇 + 忽略标记 ──
const clusters = ref<DedupClusterDTO[]>([]) // O3 疑似重复组（不含已忽略）
const ignoredClusters = ref<DedupClusterDTO[]>([]) // 全量核对返回的已忽略簇（折叠区展示）
const ignoreItems = ref<IgnoreItemDTO[]>([]) // O2 忽略清单
const ignoreModalOpen = ref(false)
const ignoreCount = computed(() => ignoreItems.value.length)
const activeClusters = computed(() => clusters.value.filter((c) => !c.ignored))

// ── 任务进度（问题3：异步任务 + 进度轮询，让用户看到“现在进度在哪”）──
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

const taskState = ref<OfflineTaskState | null>(null)
const progressPercent = computed(() => {
  const s = taskState.value
  if (!s || s.total <= 0) return 0
  return Math.min(100, Math.round((s.done / s.total) * 100))
})
const phaseText = computed(() => taskState.value?.phase || '')
const currentTitle = computed(() => taskState.value?.currentTitle || '')
// Round29：暂停/继续/取消（任务控制按钮组）
const isTaskPaused = computed(() => taskState.value?.status === 'paused')
const isTaskRunning = computed(() => taskState.value?.status === 'running')
let pollTimer: ReturnType<typeof setInterval> | null = null

const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// Round29：任务控制（暂停/继续/取消，作用于单槽位任务）
const controlTask = async (action: 'pause' | 'resume') => {
  try {
    await http(`/offline/task/${action}`, { method: 'POST' })
    if (taskState.value) {
      taskState.value = { ...taskState.value, status: action === 'pause' ? 'paused' : 'running' }
    }
    if (action === 'pause') toast.info('⏸ 已暂停，可在下方继续或取消')
    else toast.info('▶ 已继续')
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
    // 交由轮询接收 cancelled 终态收尾
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '取消失败，任务可能已完成')
  }
}

// 拉取查重结果（任务完成后调用）
const loadResult = async () => {
  try {
    const data = await http<DedupResultDTO>('/offline/maintain/result')
    items.value = data?.items || []
    clusters.value = data?.clusters || []
    ignoredClusters.value = clusters.value.filter((c) => c.ignored)
    clusters.value = clusters.value.filter((c) => !c.ignored)
    resultStale.value = !!data?.stale
  } catch {
    // 结果暂未就绪，交给轮询下一轮
  }
}

// ── Round26 O2：忽略标记 ──

// 拉取忽略清单（计数 + 弹层数据）
const loadIgnoreList = async () => {
  try {
    const data = await http<{ items: IgnoreItemDTO[] }>('/offline/ignore/list')
    ignoreItems.value = data?.items || []
  } catch {
    // 接口异常忽略
  }
}

// 忽略一个疑似重复组（title 型：核心名 + 画师）——从忽略选择弹层的「忽略整组」进入
const ignoreCluster = async (cluster: DedupClusterDTO) => {
  const name = cluster.artist ? `${cluster.titleKey}（artist:${cluster.artist}）` : cluster.titleKey
  const confirmed = await modal.confirm(
    `忽略后，后续增量查重不再提示与「${name}」相关的疑似重复；全量核对仍会列出（可在忽略清单中恢复）。\n\n确定忽略整组吗？`,
    '🕶️ 忽略本组',
  )
  if (!confirmed) return
  try {
    await http('/offline/ignore', {
      method: 'POST',
      body: JSON.stringify({ type: 'title', titleKey: cluster.titleKey, artist: cluster.artist || '' }),
    })
    toast.success('已忽略本组，可在「忽略清单」中恢复')
    clusters.value = clusters.value.filter((c) => c.id !== cluster.id)
    await loadIgnoreList()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '忽略失败')
  }
}

// ── Round26-2：忽略选择弹层（整组忽略 / 成员级忽略）──
const ignorePickerOpen = ref(false)
const ignorePickerCluster = ref<DedupClusterDTO | null>(null)
const ignoreSelectedMembers = ref<string[]>([])

const openIgnorePicker = (cluster: DedupClusterDTO) => {
  ignorePickerCluster.value = cluster
  ignoreSelectedMembers.value = []
  ignorePickerOpen.value = true
}

// 成员级忽略：选中的成员不再参与疑似重复聚类（误判剔除）
const confirmIgnoreMembers = async () => {
  const cluster = ignorePickerCluster.value
  if (!cluster || ignoreSelectedMembers.value.length === 0) return
  const ids = [...ignoreSelectedMembers.value]
  try {
    for (const m of cluster.members) {
      if (ids.includes(m.comic.id)) {
        await http('/offline/ignore', {
          method: 'POST',
          body: JSON.stringify({ type: 'comic', comicId: m.comic.id }),
        })
      }
    }
    toast.success(`已忽略 ${ids.length} 个成员（可在「忽略清单」中恢复）`)
    // 本地即时收缩：剩余成员 <2 则整簇移除，否则保留收缩后的簇
    const remain = cluster.members.filter((m) => !ids.includes(m.comic.id))
    if (remain.length < 2) {
      clusters.value = clusters.value.filter((c) => c.id !== cluster.id)
    } else {
      const idx = clusters.value.findIndex((c) => c.id === cluster.id)
      if (idx >= 0) clusters.value[idx] = { ...cluster, members: remain }
    }
    ignorePickerOpen.value = false
    await loadIgnoreList()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '忽略成员失败')
  }
}

// 进入簇对比视图（双列 + 标签卡切换，OfflineCompare type=cluster）
const openClusterCompare = (cluster: DedupClusterDTO) => {
  router.push({
    path: '/offline/compare',
    query: { type: 'cluster', titleKey: cluster.titleKey, artist: cluster.artist || '' },
  })
}

// 忽略规则 3 父画廊更新提示（gid 型：忽略父画廊 gid）
const ignoreParent = async (item: DedupItemDTO) => {
  const gid = item.comic.gid
  if (!gid) {
    toast.warning('该漫画缺少 gid，无法忽略')
    return
  }
  const confirmed = await modal.confirm(
    `忽略后，后续增量查重不再提示「${item.comic.title}」的「旧版被取代」；全量核对仍会列出（可在忽略清单中恢复）。\n\n确定忽略此提示吗？`,
    '🕶️ 忽略此提示',
  )
  if (!confirmed) return
  try {
    await http('/offline/ignore', {
      method: 'POST',
      body: JSON.stringify({ type: 'gid', gid }),
    })
    toast.success('已忽略此更新提示，可在「忽略清单」中恢复')
    items.value = items.value.filter((i) => i.comic.id !== item.comic.id)
    // 同步清理勾选残留（该条目已不在建议删除区，避免批量删除提交不存在的 id）
    selectedIds.value = selectedIds.value.filter((id) => id !== item.comic.id)
    await loadIgnoreList()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '忽略失败')
  }
}

// 恢复忽略条目（下次查重重新参与）
const restoreIgnore = async (ig: IgnoreItemDTO) => {
  try {
    await http(`/offline/ignore/${ig.id}/restore`, { method: 'POST' })
    toast.success('已恢复，下次查重重新参与判定')
    ignoreItems.value = ignoreItems.value.filter((i) => i.id !== ig.id)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '恢复失败')
  }
}

// 全量结果中的已忽略簇 → 解除忽略（按 titleKey+artist 匹配忽略清单条目恢复）
const restoreCluster = async (cluster: DedupClusterDTO) => {
  const match = ignoreItems.value.find(
    (ig) =>
      ig.type === 'title' &&
      ig.titleKey === cluster.titleKey &&
      (ig.artist || '') === (cluster.artist || ''),
  )
  if (!match) {
    toast.warning('未找到对应的忽略条目，请前往「忽略清单」管理')
    return
  }
  await restoreIgnore(match)
  ignoredClusters.value = ignoredClusters.value.filter((c) => c.id !== cluster.id)
}

// 忽略清单弹层分组
const titleIgnores = computed(() => ignoreItems.value.filter((i) => i.type === 'title'))
const gidIgnores = computed(() => ignoreItems.value.filter((i) => i.type === 'gid'))
const comicIgnores = computed(() => ignoreItems.value.filter((i) => i.type === 'comic'))

// 点击簇成员 → 打开该本地漫画详情
const openClusterMember = (m: ClusterMemberDTO) => {
  openComicDetailInNewTab({ id: m.comic.id, source: 'offline' })
}

// 轮询维护任务进度（1s 一次；结束后停止并拉取结果）
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
          // Round29：用户取消——不拉取结果
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

// 异步启动维护查重：接口立即返回，随后轮询进度
// full=false 走增量（跳过已核对过父画廊关系的漫画）；full=true 强制全量在线核对（需求1）
const runMaintain = async (full = false) => {
  if (isScanning.value) return
  isScanning.value = true
  taskState.value = null
  try {
    await http<{ started: boolean }>(`/offline/maintain?full=${full}`)
    pollProgress()
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '启动查重失败，请检查后端是否运行')
    isScanning.value = false
  }
}

// 强制全量在线核对：忽略 parent_checked_at 增量标记，联网逐本重抓全部画廊详情（可能数十分钟）
const runFullMaintain = async () => {
  const confirmed = await modal.confirm(
    '强制全量在线核对将忽略已核对标记，逐本联网抓取全部离线画廊详情页（每本约 1.2 秒限流退避），可能耗时数十分钟。确定继续吗？',
    '⚡ 强制全量在线核对',
  )
  if (!confirmed) return
  await runMaintain(true)
}

// S5/D4：清除全部「已被删除/移除」标记并复位核对时间戳，随后强制全量在线重新匹配
// （失效画廊修复后重新参与查重/更新检测；此前增量核对已跳过，需全量重新在线核对）。
const isClearingRemoved = ref(false)
const clearRemovedAndRematch = async () => {
  if (isScanning.value || isClearingRemoved.value) return
  const confirmed = await modal.confirm(
    '将清除全部「已被删除/移除」标记并复位其核对时间戳，随后强制全量在线重新匹配这些画廊（可能耗时数十分钟）。\n\n仅适用于画廊已重新上传 / 源已恢复的情况。确定继续吗？',
    '🧹 清除移除标记并全局重新匹配',
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

const keepItems = computed(() => items.value.filter((i) => i.keep))
const removeItems = computed(() => items.value.filter((i) => !i.keep))
const activeClusterMemberCount = computed(() =>
  activeClusters.value.reduce((sum, c) => sum + c.members.length, 0),
)
const isSelectAll = computed(
  () => removeItems.value.length > 0 && selectedIds.value.length === removeItems.value.length,
)

// 删除重复项：deleteFile=true 时同时物理删除本地文件
const removeComic = async (item: DedupItemDTO, deleteFile: boolean) => {
  if (isRemoving.value) return
  const c = item.comic
  const title = c.title || '未命名'

  const confirmed = await modal.confirm(
    deleteFile
      ? `确定删除《${title}》并同时删除其本地文件吗？\n\n📁 ${c.localPath || ''}\n\n此操作不可恢复！`
      : `确定仅删除《${title}》的记录吗？\n\n本地文件将保留：📁 ${c.localPath || ''}`,
    deleteFile ? '删除记录 + 本地文件' : '删除记录（保留文件）',
  )
  if (!confirmed) return

  isRemoving.value = true
  removingId.value = c.id
  try {
    const data = await http<{ ok: boolean; alreadyDeleted?: boolean }>('/offline/maintain/remove', {
      method: 'POST',
      body: JSON.stringify({ comicId: c.id, deleteFile }),
    })
    if (data.alreadyDeleted) {
      // 幽灵文件容错：记录已不存在（可能已在其他设备删除）→ 视为删除成功并同步刷新列表
      toast.info(`《${title}》记录已不存在（可能已在其他设备删除），已同步刷新列表`)
    } else {
      toast.success(
        deleteFile ? `《${title}》记录与本地文件已删除 🗑️` : `《${title}》记录已删除（保留本地文件）`,
      )
    }
    // 本地过滤该项（保留/删除关系在结果里已固定，无需重新全盘扫描）
    items.value = items.value.filter((i) => i.comic.id !== c.id)
    selectedIds.value = selectedIds.value.filter((id) => id !== c.id)
    // 本地列表已与后端一致，清除过期标记避免误报
    resultStale.value = false
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '删除失败')
  } finally {
    isRemoving.value = false
    removingId.value = ''
  }
}

const onCoverError = (id: string) => {
  coverFailed.value[id] = true
}

// Round4 任务一：点击卡片进入双列对比视图（左=建议保留，右=建议删除）
const openCompare = (comicId: string) => {
  router.push({ path: '/offline/compare', query: { type: 'maintain', id: comicId } })
}

// 勾选/取消勾选单个删除项
const toggleSelect = (id: string) => {
  if (isRemoving.value) return
  const idx = selectedIds.value.indexOf(id)
  if (idx >= 0) selectedIds.value.splice(idx, 1)
  else selectedIds.value.push(id)
}

// 全选/取消全选“建议删除”项
const toggleSelectAll = () => {
  if (isRemoving.value) return
  selectedIds.value = isSelectAll.value ? [] : removeItems.value.map((i) => i.comic.id)
}

// 批量删除：comicIds 一次提交后端，避免反复“删除→刷新”
const removeSelected = async (deleteFile: boolean) => {
  if (isRemoving.value) return
  if (selectedIds.value.length === 0) {
    toast.warning('请先勾选要删除的项')
    return
  }
  const ids = [...selectedIds.value]
  const count = ids.length
  const confirmed = await modal.confirm(
    deleteFile
      ? `确定批量删除选中的 ${count} 项，并同时删除其本地文件吗？\n\n此操作不可恢复！`
      : `确定批量删除选中的 ${count} 项记录吗？\n\n本地文件将保留。`,
    deleteFile ? '批量删除记录 + 本地文件' : '批量删除记录（保留文件）',
  )
  if (!confirmed) return

  isRemoving.value = true
  try {
    const data = await http<{ ok: boolean; deleted?: number; alreadyDeleted?: boolean }>(
      '/offline/maintain/remove',
      {
        method: 'POST',
        body: JSON.stringify({ comicIds: ids, deleteFile }),
      },
    )
    const deleted = data.deleted ?? count
    if (data.alreadyDeleted) {
      toast.info(`已处理 ${deleted} 项（部分记录已不存在，视为已删除），已同步刷新列表`)
    } else {
      toast.success(
        deleteFile
          ? `已批量删除 ${deleted} 项（记录 + 本地文件）🗑️`
          : `已批量删除 ${deleted} 项记录（保留本地文件）`,
      )
    }
    selectedIds.value = []
    // 本地过滤已删除项（保留/删除关系在结果里已固定，无需重新全盘扫描）
    items.value = items.value.filter((i) => !ids.includes(i.comic.id))
    // 本地列表已与后端一致，清除过期标记避免误报
    resultStale.value = false
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '批量删除失败')
  } finally {
    isRemoving.value = false
  }
}

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

// ⑨ Round26：检测书库变更 → 仅置「结果过期」提示，不再自动启动增量查重。
// 手动「重新扫描 / 强制全量在线核对」按钮始终保留（见模板）。
const syncStaleOnEnter = async () => {
  if (isScanning.value) return // 已有任务在跑（含后台定时器触发的），不重复拉取
  try {
    const st = await http<UnsyncedStatusDTO>('/offline/maintain/unsynced')
    await loadResult()
    await loadIgnoreList()
    if (st.hasUnsynced) {
      // 书库有未反映到查重结果的变更：仅标记过期提示，等待用户手动点「重新扫描」
      resultStale.value = true
    }
  } catch {
    // 状态接口异常（如后端未启动），忽略
  }
}

// bug2 修复：进入页面不再自动启动维护任务（否则会与后台正在运行的维护任务冲突，被后端 409 拒绝）。
// 改为同步一次当前任务状态：后台任务在跑（含暂停中）则接管轮询显示进度/控制按钮；否则拉取最近结果并同步「过期」提示。
// Round26-⑨：不再因书库变更自动触发增量查重，只提示过期，手动扫描。
const refreshOnEnter = async () => {
  try {
    const s = await http<OfflineTaskState>('/offline/maintain/progress')
    taskState.value = s
    if (s.status === 'running' || s.status === 'paused') {
      isScanning.value = true
      pollProgress()
      return // 后台任务在跑（或暂停中）：交给轮询，不重复拉取结果
    }
    await syncStaleOnEnter()
  } catch {
    // 进度接口异常（如后端未启动），忽略，页面保持空态
  }
}

let activatedOnce = false
onMounted(refreshOnEnter)
onActivated(() => {
  if (activatedOnce) {
    refreshOnEnter()
  }
  activatedOnce = true
})
onUnmounted(stopPolling)
</script>

<template>
  <div class="maintenance-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">🛠️ 本地书库维护与查重</h2>
        <p class="subtitle">
          扫描本地书库：同 GID 重复、归档 Hash 相同、父画廊关系旧版被取代，均可在此清理
        </p>
      </div>

      <div class="header-actions">
        <!-- Round26 O2：忽略清单入口（计数常驻） -->
        <button
          class="scan-btn ignore"
          title="查看全部忽略条目（疑似重复组 / 父画廊更新提示），可逐条恢复"
          @click="ignoreModalOpen = true"
        >
          🕶️ 忽略清单 <span class="count-badge">{{ ignoreCount }}</span>
        </button>
        <button
          class="scan-btn ghost"
          :disabled="isScanning || isClearingRemoved"
          title="清除全部「已被删除/移除」标记并复位核对时间戳，随后强制全量在线重新匹配（失效画廊修复后重新参与查重）"
          @click="clearRemovedAndRematch"
        >
          {{ isClearingRemoved ? '⏳ 处理中...' : '🧹 清除移除标记并全局重新匹配' }}
        </button>
        <button
          class="scan-btn ghost"
          :disabled="isScanning || isClearingRemoved"
          title="忽略已核对标记，联网逐本重抓全部画廊详情页，可能耗时数十分钟"
          @click="runFullMaintain"
        >
          ⚡ 强制全量在线核对
        </button>
        <button
          class="scan-btn primary"
          :disabled="isScanning || isClearingRemoved"
          @click="runMaintain(false)"
        >
          {{ isScanning ? '扫描中...' : '🔍 重新扫描' }}
        </button>
      </div>
    </div>

    <div class="scope-hint">
      💡 范围：默认查重所有离线漫画。可在「设置 → 额外扫描路径」中关闭某路径的「离线维护」开关，
      该路径下的漫画将不参与本查重（下载导入的漫画始终参与）。
    </div>

    <!-- ⑨ Round26：结果过期（书库变更 / 跨设备删除）→ 仅提示，不再自动扫描，引导手动「重新扫描」 -->
    <div v-if="resultStale" class="stale-banner">
      <span class="stale-icon">⚠️</span>
      <div class="stale-info">
        <p class="stale-title">查重结果已过期</p>
        <p class="stale-sub">
          本地书库已发生变化（新下载 / 更新 / 删除），当前列表不再准确。已停止自动扫描，请手动点击「重新扫描」获取最新结果。
        </p>
      </div>
      <div class="stale-action">
        <button class="scan-btn primary" :disabled="isScanning" @click="runMaintain(false)">
          🔍 重新扫描
        </button>
      </div>
    </div>

    <!-- Round29：任务控制——扫描中可暂停；暂停后可继续/取消 -->
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
            已暂停于 {{ taskState?.done }} / {{ taskState?.total }} · {{ phaseText }}，可继续或取消
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
        <button
          v-else-if="isTaskRunning"
          class="control-btn pause"
          title="暂停后当前项处理完即停止，可随时继续或取消"
          @click="controlTask('pause')"
        >
          ⏸ 暂停
        </button>
      </div>
    </div>

    <div v-else-if="items.length === 0 && activeClusters.length === 0 && ignoredClusters.length === 0" class="empty-box">
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
      <div class="summary-bar">
        <span class="summary-item warn"
          >🗑️ 建议删除 <b>{{ removeItems.length }}</b> 项</span
        >
        <span v-if="activeClusters.length > 0" class="summary-item suspect"
          >🔎 疑似重复 <b>{{ activeClusters.length }}</b> 组
          <span class="hint">（共 {{ activeClusterMemberCount }} 本，仅建议不自动处理）</span></span
        >
        <span v-if="ignoredClusters.length > 0" class="summary-item ignored"
          >🕶️ 已忽略 <b>{{ ignoredClusters.length }}</b> 组
          <span class="hint">（全量核对仍会列出）</span></span
        >
        <span class="summary-item ok"
          >✔ 建议保留 <b>{{ keepItems.length }}</b> 项</span
        >
      </div>

      <!-- 建议删除区 -->
      <div v-if="removeItems.length > 0" class="section">
        <h3 class="section-title danger">🗑️ 建议删除</h3>
        <div class="remove-toolbar">
          <label class="select-all">
            <input
              type="checkbox"
              :checked="isSelectAll"
              :disabled="isRemoving || removeItems.length === 0"
              @change="toggleSelectAll"
            />
            <span>全选</span>
          </label>
          <span class="selected-count"
            >已选 {{ selectedIds.length }} / {{ removeItems.length }} 项</span
          >
          <div class="batch-actions">
            <button
              class="action-btn danger-soft"
              :disabled="isRemoving || selectedIds.length === 0"
              @click="removeSelected(false)"
            >
              {{ isRemoving ? '⏳ 处理中...' : '批量删除（保留文件）' }}
            </button>
            <button
              class="action-btn danger"
              :disabled="isRemoving || selectedIds.length === 0"
              @click="removeSelected(true)"
            >
              {{ isRemoving ? '⏳ 处理中...' : '批量删除（含文件）' }}
            </button>
          </div>
        </div>
        <div class="item-list">
          <div
            v-for="item in removeItems"
            :key="item.comic.id"
            class="dedup-card remove-card"
            :class="{ selected: selectedIds.includes(item.comic.id) }"
            title="点击查看双列对比（左=建议保留，右=建议删除）"
            @click="openCompare(item.comic.id)"
          >
            <label class="select-check" @click.stop>
              <input
                type="checkbox"
                :checked="selectedIds.includes(item.comic.id)"
                :disabled="isRemoving"
                @change="toggleSelect(item.comic.id)"
              />
            </label>
            <div class="cover-box">
              <img
                v-if="item.comic.coverUrl && !coverFailed[item.comic.id]"
                :src="item.comic.coverUrl"
                :alt="item.comic.title"
                loading="lazy"
                @error="onCoverError(item.comic.id)"
              />
              <span v-else class="cover-fallback">🗑️</span>
            </div>

            <div class="card-main">
              <div class="card-top">
                <h4 class="card-title">{{ item.comic.title }}</h4>
                <span class="mode-chip">{{ modeText(item.comic.sourceMode) }}</span>
                <span class="compare-hint">⇄ 对比</span>
              </div>
              <div class="reason-box">
                <span class="reason-icon">⚠️</span>
                <span>{{ item.reason }}</span>
              </div>
              <div class="card-meta">
                <span class="meta-text">📄 {{ item.comic.pageCount || 0 }} 页</span>
                <span class="meta-text">💾 {{ formatBytes(item.comic.fileSize) }}</span>
                <span class="meta-text">🕒 {{ formatDate(item.comic.updatedAt) }}</span>
              </div>
              <div class="card-path">📁 {{ item.comic.localPath || '—' }}</div>
            </div>

            <div class="card-actions" @click.stop>
              <button
                class="action-btn danger-soft"
                :disabled="isRemoving"
                @click="removeComic(item, false)"
              >
                {{
                  isRemoving && removingId === item.comic.id ? '⏳ 处理中...' : '删除（保留文件）'
                }}
              </button>
              <button
                class="action-btn danger"
                :disabled="isRemoving"
                @click="removeComic(item, true)"
              >
                {{ isRemoving && removingId === item.comic.id ? '⏳ 处理中...' : '删除（含文件）' }}
              </button>
              <!-- Round26 O2：仅规则 3（父子画廊）可忽略（gid 粒度）；同 GID/hash/签名不提供忽略 -->
              <button
                v-if="item.rule === 'parent'"
                class="action-btn ghost-gray"
                :disabled="isRemoving"
                title="忽略此更新提示（父画廊 gid），后续增量查重不再提示「旧版可删除」"
                @click="ignoreParent(item)"
              >
                🕶️ 忽略此提示
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Round26 O3：疑似重复区（名称级弱证据，只建议，不自动处理） -->
      <div v-if="activeClusters.length > 0" class="section">
        <h3 class="section-title suspect">🔎 疑似重复（名称级，仅建议 · 不自动处理）</h3>
        <div class="item-list">
          <div
            v-for="cluster in activeClusters"
            :key="cluster.id"
            class="cluster-card"
          >
            <div class="cluster-head">
              <span class="cluster-name">{{ cluster.titleKey }}</span>
              <span
                class="conf-badge"
                :class="cluster.confidence === 'high' ? 'conf-high' : 'conf-medium'"
              >
                {{ cluster.confidence === 'high' ? '高置信' : '中置信' }}
              </span>
              <span class="cluster-reason">{{ cluster.reason }}</span>
              <div class="cluster-actions">
                <button
                  class="action-btn ghost-teal"
                  title="进入双列对比（标签卡可切换组内任意两本）"
                  @click="openClusterCompare(cluster)"
                >
                  ⇄ 查看对比
                </button>
                <button
                  class="action-btn ghost-gray"
                  title="忽略本组（整组或仅选择部分成员，可在忽略清单中恢复）"
                  @click="openIgnorePicker(cluster)"
                >
                  🕶️ 忽略本组
                </button>
              </div>
            </div>
            <div class="members">
              <div
                v-for="(m, mi) in cluster.members"
                :key="m.comic.id"
                class="member"
                :title="`打开本地详情：${m.comic.title}`"
                @click="openClusterMember(m)"
              >
                <div class="member-cover">
                  <img
                    v-if="m.comic.coverUrl && !coverFailed[m.comic.id]"
                    :src="m.comic.coverUrl"
                    :alt="m.comic.title"
                    loading="lazy"
                    @error="onCoverError(m.comic.id)"
                  />
                  <span v-else class="cover-fallback">{{ (cluster.titleKey || '?').slice(0, 1) }}</span>
                </div>
                <div class="member-title">{{ m.comic.title }}</div>
                <div class="member-meta">
                  <span v-if="m.lang" class="lang-chip">{{ m.lang }}</span>
                  <span class="member-pages">{{ m.pageCount || 0 }} 页</span>
                  <span v-if="mi === 0 && cluster.artist" class="member-artist">artist: {{ cluster.artist }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Round26 O2：已忽略折叠区（全量核对仍列出，默认折叠，可单独解除） -->
      <details v-if="ignoredClusters.length > 0" class="ignored-section">
        <summary class="ignored-head">
          <span class="arrow">▶</span>
          <span>🕶️ 已忽略（{{ ignoredClusters.length }} 组 · 全量核对仍会列出，可单独解除）</span>
          <span class="ignored-manage">
            <button class="scan-btn ignore" style="padding: 4px 12px; font-size: 0.78rem" @click="ignoreModalOpen = true">
              管理忽略清单
            </button>
          </span>
        </summary>
        <div v-for="cluster in ignoredClusters" :key="cluster.id" class="ignored-item">
          <span class="ignored-badge">已忽略</span>
          <div class="ignored-info">
            <div class="ignored-name">{{ cluster.titleKey }}</div>
            <div class="ignored-sub">
              {{ cluster.artist ? 'artist: ' + cluster.artist + ' · ' : '' }}{{ cluster.members.length }} 个成员 · 全量核对列出
            </div>
          </div>
          <button
            class="action-btn ghost-gray"
            title="解除忽略，下次查重重新参与判定"
            @click="restoreCluster(cluster)"
          >
            解除忽略
          </button>
        </div>
      </details>

      <!-- 建议保留区 -->
      <div v-if="keepItems.length > 0" class="section">
        <h3 class="section-title ok">✔ 建议保留</h3>
        <div class="item-list">
          <div
            v-for="item in keepItems"
            :key="item.comic.id"
            class="dedup-card keep-card"
            title="点击查看双列对比（左=建议保留，右=建议删除）"
            @click="openCompare(item.comic.id)"
          >
            <div class="cover-box">
              <img
                v-if="item.comic.coverUrl && !coverFailed[item.comic.id]"
                :src="item.comic.coverUrl"
                :alt="item.comic.title"
                loading="lazy"
                @error="onCoverError(item.comic.id)"
              />
              <span v-else class="cover-fallback">✔</span>
            </div>

            <div class="card-main">
              <div class="card-top">
                <h4 class="card-title">{{ item.comic.title }}</h4>
                <span class="mode-chip">{{ modeText(item.comic.sourceMode) }}</span>
                <span class="compare-hint">⇄ 对比</span>
              </div>
              <div class="keep-box">
                <span class="keep-icon">✔</span>
                <span>保留该副本（与上面待删除项形成同一组）</span>
              </div>
              <div class="card-meta">
                <span class="meta-text">📄 {{ item.comic.pageCount || 0 }} 页</span>
                <span class="meta-text">💾 {{ formatBytes(item.comic.fileSize) }}</span>
                <span class="meta-text">🕒 {{ formatDate(item.comic.updatedAt) }}</span>
              </div>
              <div class="card-path">📁 {{ item.comic.localPath || '—' }}</div>
            </div>

            <div class="card-actions">
              <span class="kept-badge">已保留</span>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>

  <!-- Round26-2：忽略选择弹层（整组忽略 / 成员级忽略） -->
  <div v-if="ignorePickerOpen" class="modal-mask" @click.self="ignorePickerOpen = false">
    <div class="modal-box">
      <div class="modal-head">
        <span class="modal-title">🕶️ 忽略选择</span>
        <button class="modal-close" title="关闭" @click="ignorePickerOpen = false">×</button>
      </div>
      <p class="modal-sub">
        组「{{ ignorePickerCluster?.titleKey }}」共 {{ ignorePickerCluster?.members.length }} 个成员。
        可忽略整组（后续不再提示该作品的疑似重复），或仅忽略部分成员（将误判成员剔除，不再参与疑似判定）。
      </p>
      <div class="ignore-group-title">选择要忽略的成员</div>
      <label
        v-for="m in ignorePickerCluster?.members || []"
        :key="m.comic.id"
        class="member-check"
      >
        <input type="checkbox" :value="m.comic.id" v-model="ignoreSelectedMembers" />
        <span class="member-check-title" :title="m.comic.title">{{ m.comic.title }}</span>
        <span v-if="m.lang" class="lang-chip">{{ m.lang }}</span>
        <span class="member-pages">{{ m.pageCount || 0 }} 页</span>
      </label>
      <div class="modal-actions">
        <button
          class="action-btn danger-soft"
          :disabled="ignoreSelectedMembers.length === 0"
          title="仅忽略勾选的成员（误判剔除，可在忽略清单中恢复）"
          @click="confirmIgnoreMembers"
        >
          忽略所选成员（{{ ignoreSelectedMembers.length }}）
        </button>
        <button
          class="action-btn ghost-gray"
          title="忽略整组（按核心名+画师记录，关联画廊一并忽略）"
          @click="ignoreCluster(ignorePickerCluster!)"
        >
          忽略整组
        </button>
      </div>
    </div>
  </div>

  <!-- Round26 O2：忽略清单弹层（快速查看 / 恢复忽略条目） -->
  <div v-if="ignoreModalOpen" class="modal-mask" @click.self="ignoreModalOpen = false">
    <div class="modal-box">
      <div class="modal-head">
        <span class="modal-title">🕶️ 忽略清单（{{ ignoreCount }}）</span>
        <button class="modal-close" title="关闭" @click="ignoreModalOpen = false">×</button>
      </div>
      <p class="modal-sub">忽略的疑似重复组与父画廊更新提示，恢复后将于下次查重重新参与判定。</p>

      <template v-if="ignoreItems.length === 0">
        <div class="modal-empty">暂无忽略条目</div>
      </template>
      <template v-else>
        <div v-if="titleIgnores.length > 0" class="ignore-group-title">作品指纹（title 型 · 核心名 + 画师）</div>
        <div v-for="ig in titleIgnores" :key="ig.id" class="ignore-row">
          <div class="ignore-main">
            <div class="ignore-name">{{ ig.titleKey }}</div>
            <div class="ignore-sub">
              {{ ig.artist ? 'artist: ' + ig.artist + ' · ' : '' }}忽略于 {{ formatDate(ig.createdAt) }}
            </div>
          </div>
          <button class="action-btn ghost-gray" title="恢复后下次查重重新参与判定" @click="restoreIgnore(ig)">恢复</button>
        </div>

        <div v-if="gidIgnores.length > 0" class="ignore-group-title">父画廊（gid 型 · 不再提示「旧版可删除」）</div>
        <div v-for="ig in gidIgnores" :key="ig.id" class="ignore-row">
          <div class="ignore-main">
            <div class="ignore-name">gid {{ ig.gid }}</div>
            <div class="ignore-sub">忽略于 {{ formatDate(ig.createdAt) }}</div>
          </div>
          <button class="action-btn ghost-gray" title="恢复后下次查重重新参与判定" @click="restoreIgnore(ig)">恢复</button>
        </div>

        <div v-if="comicIgnores.length > 0" class="ignore-group-title">成员（comic 型 · 不再参与疑似判定）</div>
        <div v-for="ig in comicIgnores" :key="ig.id" class="ignore-row">
          <div class="ignore-main">
            <div class="ignore-name">{{ ig.comicTitle || '漫画 ' + (ig.comicId || '') }}</div>
            <div class="ignore-sub">忽略于 {{ formatDate(ig.createdAt) }}</div>
          </div>
          <button class="action-btn ghost-gray" title="恢复后该漫画重新参与疑似判定" @click="restoreIgnore(ig)">恢复</button>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.maintenance-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
  gap: 12px;
  flex-wrap: wrap;
}

.page-title {
  font-size: 1.3rem;
  color: var(--app-text-strong);
  margin: 0;
}

.subtitle {
  font-size: 0.85rem;
  color: var(--app-text-3);
  margin: 4px 0 0 0;
  max-width: 640px;
}

.scan-btn {
  background: #007acc;
  color: #fff;
  border: none;
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 0.88rem;
  cursor: pointer;
  transition: opacity 0.2s;
}
.scan-btn:hover {
  opacity: 0.85;
}
.scan-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.header-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.scan-btn.ghost {
  background: transparent;
  color: #7ec8ff;
  border: 1px solid #007acc;
}
.scan-btn.ghost:hover {
  opacity: 0.85;
}

.stale-banner {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin: 4px 0 12px;
  padding: 12px 14px;
  background-color: rgba(255, 193, 7, 0.08);
  border: 1px solid rgba(255, 193, 7, 0.4);
  border-left: 3px solid #ffc107;
  border-radius: 6px;
}
.stale-icon {
  font-size: 1.1rem;
  line-height: 1.4;
}
.stale-info {
  flex: 1;
  min-width: 0;
}
.stale-title {
  color: #ffd54f;
  margin: 0;
  font-weight: 600;
  font-size: 0.88rem;
}
.stale-sub {
  color: #b8a26a;
  margin: 4px 0 0 0;
  font-size: 0.78rem;
  line-height: 1.5;
}
.stale-action {
  flex-shrink: 0;
  align-self: center;
}
.stale-action .scan-btn {
  padding: 6px 14px;
  font-size: 0.8rem;
}

.scanning-banner {
  display: flex;
  align-items: center;
  gap: 14px;
  background-color: #14283a;
  border: 1px solid #007acc;
  border-radius: 8px;
  padding: 14px 16px;
}

/* Round29：暂停态样式 + 任务控制按钮 */
.scanning-banner.is-paused {
  border-color: #f59e0b;
  background-color: #2a2414;
}
.paused-icon {
  font-size: 1.3rem;
  flex-shrink: 0;
}
.banner-actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex-shrink: 0;
}
.control-btn {
  border: none;
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  color: #fff;
  transition: opacity 0.2s;
}
.control-btn:hover {
  opacity: 0.85;
}
.control-btn.pause {
  background: #6b7280;
}
.control-btn.resume {
  background: #00a896;
}
.control-btn.cancel {
  background: #ff7588;
}
.scope-hint {
  margin: 4px 0 12px;
  padding: 10px 14px;
  background-color: rgba(61, 90, 254, 0.08);
  border: 1px solid rgba(61, 90, 254, 0.35);
  border-left: 3px solid #3d5afe;
  border-radius: 6px;
  color: #a8b0d8;
  font-size: 0.78rem;
  line-height: 1.5;
}
.scanning-title {
  color: #fff;
  margin: 0;
  font-weight: 600;
  font-size: 0.92rem;
}
.scanning-sub {
  color: #9bb6c8;
  margin: 3px 0 0 0;
  font-size: 0.78rem;
}
.scanning-info {
  flex: 1;
  min-width: 0;
}
.scanning-percent {
  margin-left: 8px;
  color: #7ec8ff;
  font-weight: 700;
}
.scanning-current {
  color: #d7e6f0;
  margin: 6px 0 0 0;
  font-size: 0.82rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.progress-track {
  height: 6px;
  background-color: rgba(255, 255, 255, 0.12);
  border-radius: 4px;
  margin-top: 8px;
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #007acc, #4cc3ff);
  border-radius: 4px;
  transition: width 0.3s ease;
}

.spinner {
  width: 18px;
  height: 18px;
  border: 2px solid rgba(255, 255, 255, 0.2);
  border-top-color: #007acc;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  flex-shrink: 0;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 0;
  color: var(--app-text-3);
  text-align: center;
}
.empty-box .icon {
  font-size: 3rem;
  margin-bottom: 12px;
}
.empty-title {
  color: var(--app-text-2);
  font-size: 1rem;
  margin: 0;
}
.empty-sub {
  font-size: 0.82rem;
  margin: 6px 0 0 0;
  max-width: 420px;
}

.summary-bar {
  display: flex;
  gap: 16px;
  font-size: 0.85rem;
  flex-wrap: wrap;
}
.summary-item b {
  font-size: 1rem;
}
.summary-item.warn {
  color: #f59e0b;
}
.summary-item.ok {
  color: #4caf50;
}

.section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.section-title {
  font-size: 0.95rem;
  margin: 0;
}
.section-title.danger {
  color: #ff7588;
}
.section-title.ok {
  color: #4caf50;
}

.item-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.dedup-card {
  display: flex;
  gap: 14px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 14px;
  align-items: flex-start;
  cursor: pointer;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}
.dedup-card:hover {
  border-color: #00a896;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.18);
}
.remove-card {
  border-left: 3px solid #ff7588;
}
.keep-card {
  border-left: 3px solid #4caf50;
  opacity: 0.92;
}

.compare-hint {
  flex-shrink: 0;
  font-size: 0.7rem;
  font-weight: 600;
  color: #00a896;
  border: 1px solid rgba(0, 168, 150, 0.4);
  background: rgba(0, 168, 150, 0.1);
  padding: 2px 8px;
  border-radius: 999px;
}

.cover-box {
  width: 56px;
  height: 78px;
  border-radius: 6px;
  overflow: hidden;
  background-color: var(--app-surface-3);
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}
.cover-box img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.cover-fallback {
  font-size: 1.4rem;
}

.card-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.card-top {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.card-title {
  font-size: 0.92rem;
  color: var(--app-text-strong);
  margin: 0;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.mode-chip {
  font-size: 0.7rem;
  color: #9bb6c8;
  background-color: #14283a;
  border: 1px solid #007acc;
  padding: 2px 8px;
  border-radius: 10px;
  flex-shrink: 0;
}

.reason-box {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 0.82rem;
  color: #f59e0b;
  background-color: #2a2414;
  border: 1px solid #5a4a1a;
  border-radius: 6px;
  padding: 6px 10px;
}
.reason-icon {
  flex-shrink: 0;
}

.keep-box {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 0.82rem;
  color: #4caf50;
  background-color: #14281a;
  border: 1px solid #1f4a2a;
  border-radius: 6px;
  padding: 6px 10px;
}
.keep-icon {
  flex-shrink: 0;
}

.card-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.meta-text {
  font-size: 0.75rem;
  color: var(--app-text-3);
}

.card-path {
  font-size: 0.72rem;
  color: #007acc;
  font-family: Consolas, monospace;
  word-break: break-all;
}

.card-actions {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
  flex-shrink: 0;
  width: 150px;
  justify-content: center;
}

.action-btn {
  border: none;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.2s;
  color: #fff;
}
.action-btn:hover {
  opacity: 0.85;
}
.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.action-btn.danger-soft {
  background-color: #5a3a3a;
}
.action-btn.danger {
  background-color: #ff7588;
}

.kept-badge {
  display: inline-block;
  text-align: center;
  font-size: 0.8rem;
  font-weight: 600;
  color: #4caf50;
  border: 1px solid #2f6b3a;
  background-color: #14281a;
  padding: 6px 10px;
  border-radius: 6px;
}

.remove-toolbar {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
  background-color: rgba(245, 158, 11, 0.06);
  border: 1px solid rgba(245, 158, 11, 0.25);
  border-radius: 6px;
  padding: 8px 12px;
}
.select-all {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #e5e7eb;
  font-size: 0.82rem;
  cursor: pointer;
  user-select: none;
}
.select-all input,
.select-check input {
  width: 16px;
  height: 16px;
  accent-color: #ff7588;
  cursor: pointer;
}
.select-check {
  flex-shrink: 0;
  padding-top: 2px;
  cursor: pointer;
}
.select-all:has(input:disabled),
.select-check:has(input:disabled) {
  opacity: 0.5;
  cursor: not-allowed;
}
.selected-count {
  color: #f59e0b;
  font-size: 0.82rem;
  font-weight: 600;
}
.batch-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
  flex-wrap: wrap;
}
.dedup-card.selected {
  border-color: #ff7588;
  box-shadow: 0 0 0 1px rgba(255, 117, 136, 0.4);
}

@media (max-width: 720px) {
  .dedup-card {
    flex-direction: column;
    align-items: stretch;
  }
  .card-actions {
    width: 100%;
    flex-direction: row;
  }
  .card-actions .action-btn {
    flex: 1;
  }
  .kept-badge {
    text-align: center;
  }
}

/* ── Round26 O2/O3：主按钮 / 忽略清单按钮 / 统计条 / 簇卡片 / 折叠区 / 弹层 ── */
.scan-btn.primary {
  background: #007acc;
  color: #fff;
}
.scan-btn.ignore {
  background: transparent;
  color: #b8b8c0;
  border: 1px solid var(--app-border-3);
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.count-badge {
  background: var(--app-surface-3);
  color: #e8e8f0;
  border-radius: 999px;
  padding: 1px 8px;
  font-size: 0.75rem;
  font-weight: 700;
}

.summary-item.suspect {
  color: #00c2a8;
}
.summary-item.ignored {
  color: #9a9aa2;
}
.summary-item .hint {
  font-size: 0.72rem;
  color: var(--app-text-muted);
  font-weight: 400;
}

.section-title.suspect {
  color: #00c2a8;
}
.section-title.ignored {
  color: #9a9aa2;
}

.action-btn.ghost-gray {
  background: transparent;
  color: #b8b8c0;
  border: 1px solid var(--app-border-3);
  font-weight: 500;
}
.action-btn.ghost-teal {
  background: transparent;
  color: #00c2a8;
  border: 1px solid rgba(0, 194, 168, 0.5);
  font-weight: 500;
}

/* 疑似重复簇卡片（O3） */
.cluster-card {
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-left: 3px solid #00a896;
  border-radius: 8px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  transition: border-color 0.15s;
}
.cluster-card:hover {
  border-color: #00a896;
}
.cluster-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.cluster-name {
  font-size: 0.95rem;
  color: var(--app-text-strong);
  font-weight: 700;
}
.conf-badge {
  font-size: 0.7rem;
  font-weight: 700;
  padding: 2px 10px;
  border-radius: 999px;
  flex-shrink: 0;
}
.conf-high {
  color: #34d399;
  border: 1px solid rgba(52, 211, 153, 0.4);
  background: rgba(52, 211, 153, 0.1);
}
.conf-medium {
  color: #fbbf24;
  border: 1px solid rgba(251, 191, 36, 0.4);
  background: rgba(251, 191, 36, 0.1);
}
.cluster-reason {
  font-size: 0.78rem;
  color: var(--app-text-3);
  flex: 1;
  min-width: 200px;
}
.cluster-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
  flex-shrink: 0;
}
.members {
  display: flex;
  gap: 12px;
  overflow-x: auto;
  padding-bottom: 2px;
}
.member {
  display: flex;
  flex-direction: column;
  gap: 6px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-2);
  border-radius: 6px;
  padding: 10px;
  width: 168px;
  flex-shrink: 0;
  cursor: pointer;
  transition: border-color 0.15s;
}
.member:hover {
  border-color: var(--app-accent);
}
.member-cover {
  width: 100%;
  height: 96px;
  border-radius: 4px;
  overflow: hidden;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--app-surface-3);
}
.member-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.member-cover .cover-fallback {
  font-size: 1.5rem;
  font-weight: 800;
  color: rgba(255, 255, 255, 0.7);
}
.member-title {
  font-size: 0.78rem;
  color: var(--app-text-strong);
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  min-height: 2.1em;
}
.member-meta {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  align-items: center;
}
.lang-chip {
  font-size: 0.68rem;
  color: #c9d6e0;
  background: #14283a;
  border: 1px solid var(--app-border-3);
  padding: 1px 6px;
  border-radius: 8px;
}
.member-pages {
  font-size: 0.7rem;
  color: var(--app-text-3);
}
.member-artist {
  font-size: 0.68rem;
  color: var(--app-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 已忽略折叠区（O2） */
.ignored-section {
  border: 1px dashed var(--app-border-3);
  border-radius: 8px;
  background: rgba(138, 138, 146, 0.04);
  padding: 10px 14px;
}
.ignored-head {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  user-select: none;
  font-size: 0.88rem;
  color: #9a9aa2;
  font-weight: 600;
  list-style: none;
}
.ignored-head::-webkit-details-marker {
  display: none;
}
.ignored-head .arrow {
  transition: transform 0.2s;
  font-size: 0.7rem;
}
details[open] .ignored-head .arrow {
  transform: rotate(90deg);
}
.ignored-manage {
  margin-left: auto;
}
.ignored-item {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 10px;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-left: 3px solid #8a8a92;
  border-radius: 6px;
  padding: 10px 12px;
}
.ignored-badge {
  font-size: 0.7rem;
  font-weight: 700;
  color: #b0b0b8;
  background: rgba(138, 138, 146, 0.12);
  border: 1px solid var(--app-border-3);
  padding: 2px 10px;
  border-radius: 999px;
  flex-shrink: 0;
}
.ignored-info {
  flex: 1;
  min-width: 0;
}
.ignored-name {
  font-size: 0.85rem;
  color: var(--app-text-strong);
}
.ignored-sub {
  font-size: 0.72rem;
  color: var(--app-text-muted);
}
.ignored-item .action-btn {
  flex-shrink: 0;
}

/* 忽略清单弹层（O2） */
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}
.modal-box {
  width: 560px;
  max-width: 92vw;
  max-height: 78vh;
  overflow: auto;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 10px;
  padding: 18px 20px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.5);
}
.modal-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.modal-title {
  font-size: 1.02rem;
  color: var(--app-text-strong);
  font-weight: 700;
  flex: 1;
}
.modal-close {
  border: none;
  background: transparent;
  color: var(--app-text-3);
  font-size: 1.1rem;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 4px;
}
.modal-close:hover {
  color: var(--app-text-strong);
  background: var(--app-surface-3);
}
.modal-sub {
  font-size: 0.76rem;
  color: var(--app-text-3);
}
.ignore-group-title {
  font-size: 0.78rem;
  font-weight: 700;
  color: var(--app-text-2);
  margin-top: 4px;
}
.ignore-row {
  display: flex;
  align-items: center;
  gap: 12px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-2);
  border-radius: 6px;
  padding: 10px 12px;
}
.ignore-row + .ignore-row {
  margin-top: 8px;
}
.ignore-main {
  flex: 1;
  min-width: 0;
}
.ignore-name {
  font-size: 0.85rem;
  color: var(--app-text-strong);
  word-break: break-all;
}
.ignore-sub {
  font-size: 0.72rem;
  color: var(--app-text-muted);
  margin-top: 2px;
}
.ignore-row .action-btn {
  flex-shrink: 0;
}
.modal-empty {
  text-align: center;
  color: var(--app-text-muted);
  font-size: 0.85rem;
  padding: 28px 0;
}

/* Round26-2：忽略选择弹层（成员多选） */
.member-check {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-2);
  border-radius: 6px;
  padding: 8px 12px;
  cursor: pointer;
  font-size: 0.82rem;
}
.member-check:hover {
  border-color: var(--app-accent);
}
.member-check input {
  width: 16px;
  height: 16px;
  accent-color: var(--app-accent);
  flex-shrink: 0;
}
.member-check-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--app-text-strong);
}
.modal-actions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
  margin-top: 4px;
  flex-wrap: wrap;
}
</style>
