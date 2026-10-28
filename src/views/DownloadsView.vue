<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import {
  setTaskPriority,
  fetchDownloadStats,
  batchDownloadAction,
  type DownloadTask,
  type DownloadStatus,
  type DownloadStats,
  type DownloadCounts,
  type DownloadModeFilter,
  type BatchAction,
  type BatchActionResult,
} from '@/api/download'
import { useUserStore } from '@/stores/userStore'
// 点击下载条目 → 打开「在线详情界面」（PC 新标签 / PWA 同标签，与列表页一致）
import { openContentTab, buildDetailHref } from '@/utils/detailNav'
// 队列全部完成后 toast + 刷新本地库缓存
import { fetchOfflineComics } from '@/stores/comicStore'

interface ListResponse {
  tasks: DownloadTask[]
  total: number
  page: number
  size: number
}

interface BulkConfirmConfig {
  title: string
  message: string
  confirmText: string
  tone: 'primary' | 'danger' | 'warn'
  details?: string
  detailsTone?: 'danger' | 'warn'
}

const { toast, modal } = useUI()
const userStore = useUserStore()
// 下载权限：管理员始终可下载；普通用户需 allowDownload 许可（中心制：无许可用户直接隐藏下载列表）
const canDownload = computed(() => userStore.isAdmin || !!userStore.user?.allowDownload)

// ── 状态页签（已按确认口径删除「全部」；默认落在下载中） ──
const STATUS_TABS: DownloadStatus[] = [
  'downloading',
  'queued',
  'paused',
  'error',
  'error_lock',
  'completed',
  'cancelled',
]

const STATUS_META: Record<DownloadStatus, { label: string; short: string }> = {
  downloading: { label: '下载中', short: '下载中' },
  queued: { label: '排队中', short: '排队中' },
  paused: { label: '已暂停', short: '已暂停' },
  error: { label: '出错', short: '出错' },
  error_lock: { label: 'GP/配额锁定', short: '锁定' },
  completed: { label: '完成', short: '完成' },
  cancelled: { label: '已取消', short: '已取消' },
}

// 每个状态页签可用的批量动作；完成 / 已取消没有批量操作
const BULK_ACTIONS: Partial<Record<DownloadStatus, BatchAction[]>> = {
  downloading: ['pause', 'cancel'],
  queued: ['pause', 'cancel'],
  paused: ['resume', 'cancel'],
  error: ['retry', 'cancel'],
  error_lock: ['unlock', 'cancel'],
}

const BULK_META: Record<
  BatchAction,
  { label: string; verb: string; done: string; glyph: string; cls: string }
> = {
  pause: { label: '全部暂停', verb: '暂停', done: '已暂停', glyph: '⏸', cls: 'pause' },
  resume: { label: '全部恢复', verb: '恢复', done: '已恢复', glyph: '▶', cls: 'resume' },
  retry: { label: '全部重试', verb: '重试', done: '已重新排队', glyph: '↻', cls: 'retry' },
  cancel: { label: '全部取消', verb: '取消', done: '已取消', glyph: '✕', cls: 'cancel' },
  unlock: { label: '全部解锁', verb: '解锁', done: '已解锁', glyph: '🔓', cls: 'unlock' },
}

const MODE_OPTIONS: { key: DownloadModeFilter; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'gallery', label: '🖼️ 画廊' },
  { key: 'archive', label: '🗜️ 归档' },
]

const MODE_LABEL: Record<DownloadModeFilter, string> = {
  all: '全部模式',
  gallery: '画廊模式',
  archive: '归档模式',
}

const BULK_CONFIRM_THRESHOLD = 20

const emptyCounts = (): DownloadCounts => ({
  queued: 0,
  downloading: 0,
  paused: 0,
  completed: 0,
  error: 0,
  error_lock: 0,
  cancelled: 0,
})

const tasks = ref<DownloadTask[]>([])
const filteredTotal = ref(0)
const stats = ref<DownloadStats>({ counts: emptyCounts(), total: 0, active: 0 })
const page = ref(1)
const size = 20
const modeFilter = ref<DownloadModeFilter>('all')
const statusFilter = ref<DownloadStatus>('downloading')
const isLoading = ref(false)
const bulkRunning = ref<null | { action: BatchAction; total: number }>(null)
const tabsScrollEl = ref<HTMLElement | null>(null)
let timer: number | null = null
let unmounted = false
let fetchSeq = 0

const bulkActions = computed(() => BULK_ACTIONS[statusFilter.value] ?? [])
const currentCount = computed(() => stats.value.counts[statusFilter.value] ?? 0)
const showBulkBar = computed(
  () => !bulkRunning.value && currentCount.value > 0 && bulkActions.value.length > 0,
)
const bulkAccent = computed<BatchAction>(() => bulkActions.value[0] ?? 'cancel')
const totalPages = computed(() => Math.max(1, Math.ceil(filteredTotal.value / size)))
const showPagination = computed(() => filteredTotal.value > size)
const bulkSub = computed(() => {
  let text =
    statusFilter.value === 'error_lock'
      ? '解锁后任务重新排队，会再次消耗 GP；批量操作跨页生效'
      : '批量操作作用于当前筛选下的全部任务，跨页生效'
  if (currentCount.value > BULK_CONFIRM_THRESHOLD) {
    text += '；操作量较大，执行前会再次确认'
  }
  return text
})

// ── 格式化 ──
const percentOf = (t: DownloadTask): number => {
  if (t.totalBytes > 0) {
    return Math.min(100, Math.round((t.doneBytes / t.totalBytes) * 100))
  }
  if (t.totalFiles > 0) {
    return Math.min(100, Math.round((t.doneFiles / t.totalFiles) * 100))
  }
  return t.status === 'completed' ? 100 : 0
}

const formatBytes = (bytes: number): string => {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  const mb = bytes / 1024 / 1024
  if (mb < 1024) return `${mb.toFixed(1)} MB`
  return `${(mb / 1024).toFixed(2)} GB`
}

const formatSpeed = (speed: number): string => {
  if (!speed) return '0 KB/s'
  if (speed < 1024 * 1024) return `${(speed / 1024).toFixed(1)} KB/s`
  return `${(speed / 1024 / 1024).toFixed(2)} MB/s`
}

const progressExtra = (t: DownloadTask): string => {
  if (t.totalBytes > 0) {
    return ` · ${formatBytes(t.doneBytes)} / ${formatBytes(t.totalBytes)}`
  }
  if (t.totalFiles > 0) {
    return ` · ${t.doneFiles} / ${t.totalFiles} 文件`
  }
  return ''
}

const initialOf = (title: string): string => {
  const cleaned = title.replace(/^\[[^\]]+\]\s*/, '')
  return (cleaned || title).trim().charAt(0).toUpperCase()
}

const coverStyle = (task: DownloadTask): Record<string, string> => {
  let hue = 0
  for (const ch of task.title) {
    hue = (hue * 31 + ch.charCodeAt(0)) % 360
  }
  return { '--h1': String(hue), '--h2': String((hue + 40) % 360) }
}

// 归档限流(429)错误前端固化提示：兼容后端旧版本——后端可能只返回裸「分块 N 返回状态码
// 429（未获 206）」，统一映射为可读的降级指引；新后端返回的「归档下载被限流(HTTP 429)…」
// 已含明确文案，此处命中后同样拼上前缀提示不重复。
const displayTaskError = (t: DownloadTask): string => {
  const raw = t.error || ''
  if (t.mode === 'archive' && /429|限流/.test(raw) && !raw.includes('并发数太多')) {
    return `⚠️ 并发数太多：H@H 节点限流，请降低归档下载线程数或减少同时下载的归档任务数后重试（${raw}）`
  }
  return raw
}

const priorityLocked = (t: DownloadTask): boolean =>
  t.status === 'completed' || t.status === 'cancelled'

const canCancel = (t: DownloadTask): boolean =>
  ['queued', 'downloading', 'paused', 'error', 'error_lock'].includes(t.status)

// ── 数据获取 ──
const fetchData = async () => {
  const seq = ++fetchSeq
  isLoading.value = true
  try {
    const [list, st] = await Promise.all([
      http<ListResponse>('/downloads', {
        params: {
          page: page.value,
          size,
          mode: modeFilter.value === 'all' ? '' : modeFilter.value,
          status: statusFilter.value,
        },
      }),
      fetchDownloadStats(modeFilter.value),
    ])
    // 快速切页签 / 轮询与操作并发时，丢弃过期请求，避免旧数据覆盖新视图
    if (seq !== fetchSeq || unmounted) return
    tasks.value = list.tasks || []
    filteredTotal.value = list.total || 0
    stats.value = st
    // 单条操作后当前页可能因任务数减少而越界：回到第一页重新拉取，避免「有任务却显示空态」
    if (!tasks.value.length && filteredTotal.value > 0 && page.value > 1) {
      page.value = 1
      await fetchData()
      return
    }
    checkQueueCompletion(st.active)
  } catch (err: unknown) {
    if (seq === fetchSeq && !unmounted) {
      toast.error(err instanceof Error ? err.message : '获取下载任务失败')
    }
  } finally {
    if (seq === fetchSeq) isLoading.value = false
  }
}

function startPolling() {
  if (unmounted || timer !== null) return
  timer = window.setInterval(() => {
    if (!bulkRunning.value) fetchData()
  }, 2000)
}

function stopPolling() {
  if (timer !== null) {
    clearInterval(timer)
    timer = null
  }
}

// 队列「从有到无」→ toast + 刷新本地库缓存。全库维护查重仍为手动触发。
let lastActiveCount = -1
function checkQueueCompletion(active: number) {
  if (lastActiveCount > 0 && active === 0) {
    toast.success('下载队列已全部完成，本地书库已同步刷新')
    fetchOfflineComics()
  }
  lastActiveCount = active
}

// ── 筛选与分页 ──
function centerActiveTab() {
  const scroller = tabsScrollEl.value
  const active = scroller?.querySelector<HTMLElement>('.tab.active')
  if (!scroller || !active) return
  const centered = active.offsetLeft + active.offsetWidth / 2 - scroller.clientWidth / 2
  const max = scroller.scrollWidth - scroller.clientWidth
  scroller.scrollLeft = Math.max(0, Math.min(centered, max))
}

function setStatusFilter(status: DownloadStatus) {
  if (bulkRunning.value || statusFilter.value === status) return
  statusFilter.value = status
  page.value = 1
  fetchData()
  nextTick(centerActiveTab)
}

function setModeFilter(mode: DownloadModeFilter) {
  if (bulkRunning.value || modeFilter.value === mode) return
  modeFilter.value = mode
  page.value = 1
  fetchData()
}

function changePage(delta: number) {
  if (bulkRunning.value) return
  const next = page.value + delta
  if (next < 1 || next > totalPages.value) return
  page.value = next
  fetchData()
}

// ── 单条操作 ──
const callAction = async (id: string, action: string, successMsg: string) => {
  if (bulkRunning.value) return
  try {
    await http(`/downloads/${id}/${action}`, { method: 'POST' })
    toast.success(successMsg)
    await fetchData()
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '操作失败')
  }
}

const handlePause = (t: DownloadTask) => callAction(t.id, 'pause', '已暂停任务')
const handleResume = (t: DownloadTask) => callAction(t.id, 'resume', '已恢复任务')
const handleRetry = (t: DownloadTask) => callAction(t.id, 'retry', '已重试任务')
const handleUnlock = (t: DownloadTask) => callAction(t.id, 'unlock', '已解锁任务')

const handleCancel = async (t: DownloadTask) => {
  if (bulkRunning.value) return
  const isCorruptArchive =
    t.status === 'error' && t.mode === 'archive' && (t.error || '').startsWith('解压失败')
  const ok = await modal.confirm(
    `确定取消下载《${t.title}》吗？取消后任务将移入「已取消」，无法继续下载。`,
    '取消下载？',
    {
      confirmText: '确认取消',
      cancelText: '再想想',
      tone: 'danger',
      details: isCorruptArchive
        ? '该任务解压失败（压缩包已损坏）：取消将删除损坏的压缩包与半解压文件，下次下载需重新下载。'
        : undefined,
      detailsTone: 'danger',
    },
  )
  if (ok) {
    await callAction(t.id, 'cancel', isCorruptArchive ? '已取消并删除损坏压缩包' : '已取消任务')
  }
}

// ── 优先级 ──
const PRIORITY_MIN = 0
const PRIORITY_MAX = 99

const handlePriorityChange = async (t: DownloadTask, delta: number) => {
  if (bulkRunning.value) return
  const next = Math.min(PRIORITY_MAX, Math.max(PRIORITY_MIN, t.priority + delta))
  if (next === t.priority) return
  try {
    const updated = await setTaskPriority(t.id, next)
    t.priority = updated.priority
    toast.success(`已设置优先级 ${updated.priority}`)
    fetchData()
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '修改优先级失败')
  }
}

// ── 点击卡片 → 在线详情 ──
const handleOpenDetail = (t: DownloadTask) => {
  if (!t.gid) return
  openContentTab({ href: buildDetailHref({ id: t.gid, token: t.token }), id: t.gid })
}

// 事件委托：卡片整卡可点，但内部交互控件（操作按钮/优先级增减等）点击不触发跳转
const handleCardClick = (e: MouseEvent, t: DownloadTask) => {
  if (bulkRunning.value) return
  const target = e.target as HTMLElement
  if (target.closest('button, select, input, a, label')) return
  handleOpenDetail(t)
}

// ── 批量操作 ──
function buildBulkConfirm(action: BatchAction, count: number): BulkConfirmConfig {
  const pages = Math.max(1, Math.ceil(count / size))
  const scope =
    `「${STATUS_META[statusFilter.value].short}」视图下的全部 ${count} 个任务` +
    (pages > 1 ? `（跨 ${pages} 页）` : '')
  const common = { tone: 'primary' as const }

  switch (action) {
    case 'pause':
      return {
        title: `暂停 ${count} 个任务？`,
        message: `即将暂停${scope}。暂停会释放下载线程，可随时恢复。`,
        confirmText: '确认暂停',
        ...common,
      }
    case 'resume':
      return {
        title: `恢复 ${count} 个任务？`,
        message: `即将恢复${scope}。恢复后任务会重新排队并继续下载。`,
        confirmText: '确认恢复',
        ...common,
      }
    case 'retry':
      return {
        title: `重试 ${count} 个任务？`,
        message: `即将重试${scope}。重试会重新发起下载请求，若仍失败会再次进入出错。`,
        confirmText: '确认重试',
        ...common,
      }
    case 'unlock':
      return {
        title: `解锁 ${count} 个任务？`,
        message: `即将解锁${scope}。解锁后任务会重新排队。`,
        confirmText: '确认解锁',
        tone: 'warn',
        details: '解锁会重复消耗 GP 配额，请确认当前 GP 余额充足。',
        detailsTone: 'warn',
      }
    case 'cancel':
    default:
      return {
        title: `取消 ${count} 个任务？`,
        message: `即将取消${scope}。取消后任务将移入「已取消」，无法继续下载。`,
        confirmText: `确认取消 ${count} 个`,
        tone: 'danger',
        details:
          statusFilter.value === 'error' && modeFilter.value !== 'gallery'
            ? '取消会立即中断下载；解压失败的归档任务会同时删除损坏的压缩包与半解压文件。'
            : undefined,
        detailsTone: 'danger',
      }
  }
}

function showBulkResult(action: BatchAction, result: BatchActionResult) {
  const meta = BULK_META[action]
  const parts: string[] = []
  if (result.affected > 0) parts.push(`${meta.done} ${result.affected} 个`)
  if (result.failed > 0) parts.push(`失败 ${result.failed} 个`)
  if (result.skipped > 0) parts.push(`跳过 ${result.skipped} 个（状态已变化）`)
  if (!parts.length) {
    toast.info('没有可处理的任务')
    return
  }
  const message = parts.join('，')
  if (result.failed > 0) {
    const firstError = result.errors?.[0]
    toast.warning(firstError ? `${message}：${firstError}` : message)
  } else {
    toast.success(message)
  }
}

async function executeBulk(action: BatchAction, count: number) {
  if (bulkRunning.value) return
  bulkRunning.value = { action, total: count }
  stopPolling()
  try {
    const result = await batchDownloadAction(action, statusFilter.value, modeFilter.value)
    showBulkResult(action, result)
    page.value = 1
    await fetchData()
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '批量操作失败')
  } finally {
    bulkRunning.value = null
    startPolling()
  }
}

async function handleBulk(action: BatchAction) {
  if (bulkRunning.value) return
  const count = currentCount.value
  if (count <= 0) return
  // 全部取消无论数量都确认；其余动作 > 20 才确认。
  if (action === 'cancel' || count > BULK_CONFIRM_THRESHOLD) {
    const config = buildBulkConfirm(action, count)
    const ok = await modal.confirm(config.message, config.title, {
      confirmText: config.confirmText,
      cancelText: '再想想',
      tone: config.tone,
      details: config.details,
      detailsTone: config.detailsTone,
    })
    if (!ok) return
  }
  await executeBulk(action, count)
}

onMounted(() => {
  if (!canDownload.value) return // 无下载权限：不发起轮询
  fetchData()
  startPolling()
})

onUnmounted(() => {
  unmounted = true
  stopPolling()
})
</script>

<template>
  <div class="downloads-page" :class="{ 'bulk-busy': !!bulkRunning }">
    <!-- 无下载权限：直接隐藏任务列表与筛选区（中心制：无许可用户不展示下载功能） -->
    <div v-if="!canDownload" class="no-permission-box">🔒 无下载权限，请联系管理员开启</div>
    <template v-else>
      <div class="page-header">
        <h1 class="page-title"><span>⬇️</span>下载任务列表</h1>
        <div class="page-meta">
          <span
            >共 <b>{{ stats.total }}</b> 个任务 · 活动 <b>{{ stats.active }}</b></span
          >
          <span class="live"><span class="live-dot"></span>每 2 秒自动刷新</span>
        </div>
      </div>

      <!-- 筛选：模式（保留「全部」）+ 7 个状态页签（无「全部」） -->
      <section class="filter-card" aria-label="下载筛选">
        <div class="filter-top">
          <div class="filter-top-left">
            <span class="seg-label">模式</span>
            <div class="seg" role="group" aria-label="下载模式">
              <button
                v-for="m in MODE_OPTIONS"
                :key="m.key"
                type="button"
                :class="{ active: modeFilter === m.key }"
                :disabled="!!bulkRunning"
                @click="setModeFilter(m.key)"
              >
                {{ m.label }}
              </button>
            </div>
          </div>
          <span class="filter-hint">状态页签带任务数量；批量操作随页签变化</span>
        </div>
        <div class="tabs-wrap">
          <div ref="tabsScrollEl" class="tabs-scroll">
            <div class="tabs" role="tablist" aria-label="下载状态">
              <button
                v-for="status in STATUS_TABS"
                :key="status"
                type="button"
                class="tab"
                :class="{ active: statusFilter === status }"
                :data-status="status"
                role="tab"
                :aria-selected="statusFilter === status"
                :disabled="!!bulkRunning"
                @click="setStatusFilter(status)"
              >
                <span class="tab-dot"></span>
                <span>{{ STATUS_META[status].short }}</span>
                <span class="tab-badge">{{ stats.counts[status] ?? 0 }}</span>
              </button>
            </div>
          </div>
        </div>
      </section>

      <!-- 批量操作条：执行中 / 常规 -->
      <section
        v-if="bulkRunning"
        class="bulk-bar"
        :data-bulk="BULK_META[bulkRunning.action].cls"
        aria-live="polite"
      >
        <div class="bulk-info">
          <span class="bulk-icon"><span class="spinner"></span></span>
          <div class="bulk-text">
            <div class="bulk-title">
              正在{{ BULK_META[bulkRunning.action].verb }} {{ bulkRunning.total }} 个任务
            </div>
            <div class="bulk-sub">执行期间暂停自动刷新，完成后自动恢复</div>
          </div>
        </div>
        <div class="bulk-progress">
          <div class="progress-track"><i class="indeterminate"></i></div>
          <span class="bulk-counter">处理中…</span>
        </div>
      </section>
      <section v-else-if="showBulkBar" class="bulk-bar" :data-bulk="BULK_META[bulkAccent].cls">
        <div class="bulk-info">
          <span class="bulk-icon">{{ BULK_META[bulkAccent].glyph }}</span>
          <div class="bulk-text">
            <div class="bulk-title">
              {{ STATUS_META[statusFilter].short }}任务 <b>{{ currentCount }}</b> 个<span
                class="bulk-mode"
                >· {{ MODE_LABEL[modeFilter] }}</span
              >
            </div>
            <div class="bulk-sub">{{ bulkSub }}</div>
          </div>
        </div>
        <div class="bulk-actions">
          <template v-for="(action, idx) in bulkActions" :key="action">
            <span v-if="idx > 0" class="bulk-divider"></span>
            <button
              type="button"
              class="bulk-btn"
              :class="BULK_META[action].cls"
              :disabled="!!bulkRunning"
              @click="handleBulk(action)"
            >
              <span class="bglyph">{{ BULK_META[action].glyph }}</span
              >{{ BULK_META[action].label }}
            </button>
          </template>
        </div>
      </section>

      <div v-if="isLoading && !tasks.length" class="loading-box">加载中...</div>
      <div v-else-if="!tasks.length" class="empty-state">
        <div class="empty-icon">🗂️</div>
        <h3>「{{ STATUS_META[statusFilter].short }}」下暂无任务</h3>
        <p>
          切换上方页签查看其它状态{{
            modeFilter !== 'all' ? '，或把模式切回「全部」查看另一类' : ''
          }}。
        </p>
      </div>

      <div v-else class="task-list">
        <article
          v-for="task in tasks"
          :key="task.id"
          class="task-card"
          :data-status="task.status"
          :title="`点击查看《${task.title}》在线详情`"
          @click="handleCardClick($event, task)"
        >
          <div class="cover" :style="coverStyle(task)">
            <img
              v-if="task.coverUrl"
              :src="task.coverUrl"
              :alt="task.title"
              referrerpolicy="no-referrer"
            />
            <span v-else class="cover-initial">{{ initialOf(task.title) }}</span>
          </div>

          <div class="task-main">
            <div class="task-head">
              <h3 class="task-title" :title="task.title">{{ task.title }}</h3>
              <span v-if="task.status === 'downloading' && task.speed" class="speed">
                {{ formatSpeed(task.speed) }}
              </span>
              <span class="status-badge" :data-status="task.status">
                <span class="sdot"></span>{{ STATUS_META[task.status].label }}
              </span>
            </div>

            <div class="chips">
              <span v-if="task.mode === 'archive'" class="chip mode archive">
                🗜️ 归档{{ task.archiveType === 'resample' ? '·压缩' : '·原图' }}
              </span>
              <span v-else class="chip mode gallery">🖼️ 画廊</span>
              <span v-if="task.group" class="chip">分组 {{ task.group }}</span>
              <span class="chip priority" :class="{ zero: task.priority === 0 }">
                ⭐ 优先级 {{ task.priority }}
              </span>
              <span v-if="task.updateForComicId" class="chip update">🔄 离线更新</span>
              <span v-if="userStore.isAdmin && task.username" class="chip user">
                发起者：{{ task.username }}
              </span>
              <span v-if="task.nodeName" class="chip node" :title="`归档解锁时使用的出口节点：${task.nodeName}`">
                🌐 {{ task.nodeName }}
              </span>
            </div>

            <div v-if="task.error" class="task-error" :title="displayTaskError(task)">
              <span class="err-icon">⚠</span>
              <span class="err-text">{{ displayTaskError(task) }}</span>
            </div>

            <div class="progress-bar" :data-status="task.status">
              <i :style="{ width: percentOf(task) + '%' }"></i>
            </div>

            <div class="task-foot">
              <span class="progress-info">
                <b>{{ percentOf(task) }}%</b>{{ progressExtra(task) }}
              </span>
              <div class="card-actions">
                <span
                  class="prio"
                  :class="{ locked: priorityLocked(task) }"
                  title="下载优先级，数字越大越优先（0-99）"
                >
                  <button
                    type="button"
                    :disabled="
                      priorityLocked(task) || task.priority <= PRIORITY_MIN || !!bulkRunning
                    "
                    aria-label="降低优先级"
                    @click="handlePriorityChange(task, -1)"
                  >
                    −
                  </button>
                  <span class="prio-val">{{ task.priority }}</span>
                  <button
                    type="button"
                    :disabled="
                      priorityLocked(task) || task.priority >= PRIORITY_MAX || !!bulkRunning
                    "
                    aria-label="提高优先级"
                    @click="handlePriorityChange(task, 1)"
                  >
                    ＋
                  </button>
                </span>

                <button
                  v-if="task.status === 'error_lock'"
                  type="button"
                  class="card-btn lock"
                  :disabled="!!bulkRunning"
                  @click="handleUnlock(task)"
                >
                  解锁
                </button>
                <button
                  v-else-if="task.status === 'error'"
                  type="button"
                  class="card-btn retry"
                  :disabled="!!bulkRunning"
                  @click="handleRetry(task)"
                >
                  重试
                </button>

                <button
                  v-if="task.status === 'queued' || task.status === 'downloading'"
                  type="button"
                  class="card-btn pause"
                  :disabled="!!bulkRunning"
                  @click="handlePause(task)"
                >
                  暂停
                </button>
                <button
                  v-else-if="task.status === 'paused'"
                  type="button"
                  class="card-btn resume"
                  :disabled="!!bulkRunning"
                  @click="handleResume(task)"
                >
                  恢复
                </button>

                <button
                  v-if="canCancel(task)"
                  type="button"
                  class="card-btn cancel"
                  :disabled="!!bulkRunning"
                  @click="handleCancel(task)"
                >
                  取消
                </button>
              </div>
            </div>
          </div>
        </article>
      </div>

      <!-- 分页：单页隐藏 -->
      <div v-if="tasks.length && showPagination" class="pagination">
        <button
          type="button"
          class="page-btn"
          :disabled="page <= 1 || !!bulkRunning"
          @click="changePage(-1)"
        >
          ‹ 上一页
        </button>
        <span class="page-info"
          >第 <b>{{ page }}</b> / {{ totalPages }} 页 · 共 {{ filteredTotal }} 个任务</span
        >
        <button
          type="button"
          class="page-btn"
          :disabled="page >= totalPages || !!bulkRunning"
          @click="changePage(1)"
        >
          下一页 ›
        </button>
      </div>
    </template>
  </div>
</template>

<style scoped>
/* 状态色：与样稿一致；深色为默认，浅色由 data-theme 覆盖 */
.downloads-page {
  --st-queued: #9aa4ab;
  --st-queued-soft: rgba(154, 164, 171, 0.15);
  --st-downloading: #5cb8ff;
  --st-downloading-soft: rgba(0, 122, 204, 0.17);
  --st-paused: #d9a441;
  --st-paused-soft: rgba(217, 164, 65, 0.14);
  --st-error: #ff6b6b;
  --st-error-soft: rgba(230, 57, 70, 0.16);
  --st-lock: #e6a23c;
  --st-lock-soft: rgba(230, 162, 60, 0.15);
  --st-completed: #67c23a;
  --st-completed-soft: rgba(103, 194, 58, 0.15);
  --st-cancelled: #8a8a92;
  --st-cancelled-soft: rgba(138, 138, 146, 0.14);

  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 20px;
  max-width: 1000px;
  margin: 0 auto;
}

:root[data-theme='light'] .downloads-page {
  --st-queued: #5f6670;
  --st-queued-soft: rgba(95, 102, 112, 0.09);
  --st-downloading: #0066b8;
  --st-downloading-soft: rgba(0, 102, 184, 0.1);
  --st-paused: #946200;
  --st-paused-soft: rgba(148, 98, 0, 0.1);
  --st-error: #c62828;
  --st-error-soft: rgba(198, 40, 40, 0.08);
  --st-lock: #946200;
  --st-lock-soft: rgba(148, 98, 0, 0.1);
  --st-completed: #2e7d32;
  --st-completed-soft: rgba(46, 125, 50, 0.1);
  --st-cancelled: #6b6b72;
  --st-cancelled-soft: rgba(107, 107, 114, 0.09);
}

.page-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 14px;
  flex-wrap: wrap;
}

.page-title {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 1.3rem;
  font-weight: 700;
  letter-spacing: 0.2px;
  color: var(--app-text-strong);
}

.page-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  font-size: 0.78rem;
  color: var(--app-text-3);
}

.page-meta b {
  color: var(--app-text-strong);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.live {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding-left: 12px;
  border-left: 1px solid var(--app-border-2);
}

.live-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--st-completed);
  box-shadow: 0 0 0 0 var(--st-completed-soft);
  animation: live-pulse 2s ease-out infinite;
}

@keyframes live-pulse {
  0% {
    box-shadow: 0 0 0 0 var(--st-completed-soft);
  }
  70% {
    box-shadow: 0 0 0 5px transparent;
  }
  100% {
    box-shadow: 0 0 0 0 transparent;
  }
}

.filter-card {
  padding: 12px 14px 2px;
  border: 1px solid var(--app-border-2);
  border-radius: 12px;
  background: var(--app-surface-2);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.12);
}

.filter-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.filter-top-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.seg-label {
  flex: none;
  color: var(--app-text-3);
  font-size: 0.72rem;
}

.seg {
  display: inline-flex;
  gap: 2px;
  padding: 3px;
  border: 1px solid var(--app-border-2);
  border-radius: 9px;
  background: var(--app-input-bg);
}

.seg button {
  padding: 5px 14px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--app-text-3);
  font-size: 0.8rem;
  cursor: pointer;
  transition:
    background 0.15s,
    color 0.15s;
  white-space: nowrap;
}

.seg button:hover {
  color: var(--app-text-strong);
}

.seg button.active {
  background: var(--app-surface-3);
  color: var(--app-text-strong);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);
}

.seg button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.filter-hint {
  font-size: 0.72rem;
  color: var(--app-text-muted);
}

.tabs-wrap {
  position: relative;
}

.tabs-wrap::before,
.tabs-wrap::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  z-index: 1;
  display: none;
  width: 18px;
  pointer-events: none;
}

.tabs-wrap::before {
  left: 0;
  background: linear-gradient(90deg, var(--app-surface-2), transparent);
}

.tabs-wrap::after {
  right: 0;
  background: linear-gradient(270deg, var(--app-surface-2), transparent);
}

.tabs-scroll {
  margin-top: 11px;
  padding-top: 4px;
  border-top: 1px solid var(--app-border-2);
  overflow-x: auto;
  scrollbar-width: none;
  -webkit-overflow-scrolling: touch;
}

.tabs-scroll::-webkit-scrollbar {
  display: none;
}

.tabs {
  display: flex;
  min-width: max-content;
  gap: 2px;
}

.tab {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 8px 11px 9px;
  border: 0;
  border-bottom: 2px solid transparent;
  border-radius: 7px 7px 0 0;
  background: transparent;
  color: var(--app-text-3);
  font-size: 0.82rem;
  cursor: pointer;
  transition:
    background 0.15s,
    color 0.15s;
  white-space: nowrap;
}

.tab:hover {
  color: var(--app-text-2);
  background: var(--app-surface-2-hover);
}

.tab.active {
  color: var(--app-text-strong);
  border-bottom-color: var(--taccent);
}

.tab:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.tab-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex: none;
  background: var(--taccent);
}

.tab-badge {
  min-width: 20px;
  padding: 2px 6px;
  border-radius: 999px;
  background: var(--app-surface-3);
  color: var(--app-text-3);
  font-size: 0.68rem;
  line-height: 1.35;
  text-align: center;
  font-variant-numeric: tabular-nums;
}

.tab.active .tab-badge {
  background: var(--taccent-soft);
  color: var(--taccent);
  font-weight: 700;
}

.tab[data-status='downloading'] {
  --taccent: #5cb8ff;
  --taccent-soft: rgba(92, 184, 255, 0.16);
}
.tab[data-status='queued'] {
  --taccent: #9aa4ab;
  --taccent-soft: rgba(154, 164, 171, 0.16);
}
.tab[data-status='paused'] {
  --taccent: #d9a441;
  --taccent-soft: rgba(217, 164, 65, 0.14);
}
.tab[data-status='error'] {
  --taccent: #ff6b6b;
  --taccent-soft: rgba(255, 107, 107, 0.16);
}
.tab[data-status='error_lock'] {
  --taccent: #e6a23c;
  --taccent-soft: rgba(230, 162, 60, 0.15);
}
.tab[data-status='completed'] {
  --taccent: #67c23a;
  --taccent-soft: rgba(103, 194, 58, 0.15);
}
.tab[data-status='cancelled'] {
  --taccent: #8a8a92;
  --taccent-soft: rgba(138, 138, 146, 0.14);
}

:root[data-theme='light'] .tab[data-status='downloading'] {
  --taccent: #0066b8;
  --taccent-soft: rgba(0, 102, 184, 0.1);
}
:root[data-theme='light'] .tab[data-status='queued'] {
  --taccent: #5f6670;
  --taccent-soft: rgba(95, 102, 112, 0.1);
}
:root[data-theme='light'] .tab[data-status='paused'] {
  --taccent: #946200;
  --taccent-soft: rgba(148, 98, 0, 0.1);
}
:root[data-theme='light'] .tab[data-status='error'] {
  --taccent: #c62828;
  --taccent-soft: rgba(198, 40, 40, 0.08);
}
:root[data-theme='light'] .tab[data-status='error_lock'] {
  --taccent: #946200;
  --taccent-soft: rgba(148, 98, 0, 0.1);
}
:root[data-theme='light'] .tab[data-status='completed'] {
  --taccent: #2e7d32;
  --taccent-soft: rgba(46, 125, 50, 0.1);
}
:root[data-theme='light'] .tab[data-status='cancelled'] {
  --taccent: #6b6b72;
  --taccent-soft: rgba(107, 107, 114, 0.09);
}

/* ── 批量操作条 ── */
.bulk-bar {
  --bulk: var(--app-accent);
  --bulk-soft: rgba(0, 122, 204, 0.14);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 11px 14px;
  border: 1px solid var(--app-border-2);
  border-left: 3px solid var(--bulk);
  border-radius: 10px;
  background: var(--app-surface-2);
}

.bulk-bar[data-bulk='pause'] {
  --bulk: var(--st-downloading);
  --bulk-soft: var(--st-downloading-soft);
}
.bulk-bar[data-bulk='resume'] {
  --bulk: var(--st-completed);
  --bulk-soft: var(--st-completed-soft);
}
.bulk-bar[data-bulk='retry'] {
  --bulk: var(--st-downloading);
  --bulk-soft: var(--st-downloading-soft);
}
.bulk-bar[data-bulk='unlock'] {
  --bulk: var(--st-lock);
  --bulk-soft: var(--st-lock-soft);
}
.bulk-bar[data-bulk='cancel'] {
  --bulk: var(--st-error);
  --bulk-soft: var(--st-error-soft);
}

.bulk-info {
  display: flex;
  align-items: center;
  gap: 11px;
  min-width: 0;
}

.bulk-icon {
  display: grid;
  place-items: center;
  flex: none;
  width: 34px;
  height: 34px;
  border-radius: 9px;
  background: var(--bulk-soft);
  color: var(--bulk);
  font-size: 1rem;
}

.bulk-text {
  min-width: 0;
}

.bulk-title {
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--app-text-strong);
  line-height: 1.4;
}

.bulk-title b {
  color: var(--bulk);
  font-variant-numeric: tabular-nums;
}

.bulk-mode {
  font-size: 0.72rem;
  font-weight: 400;
  color: var(--app-text-3);
}

.bulk-sub {
  margin-top: 1px;
  font-size: 0.71rem;
  color: var(--app-text-3);
}

.bulk-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}

.bulk-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 7px 15px;
  border: 1px solid transparent;
  border-radius: 7px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  transition:
    filter 0.15s,
    background 0.15s,
    border-color 0.15s;
}

.bulk-btn .bglyph {
  font-size: 0.8rem;
  line-height: 1;
}

.bulk-btn.pause,
.bulk-btn.retry {
  background: var(--app-accent);
  border-color: var(--app-accent);
  color: #fff;
}

.bulk-btn.resume {
  background: var(--st-completed);
  border-color: var(--st-completed);
  color: #0c2006;
}

:root[data-theme='light'] .bulk-btn.resume {
  color: #fff;
}

.bulk-btn.unlock {
  background: var(--st-lock);
  border-color: var(--st-lock);
  color: #231604;
}

:root[data-theme='light'] .bulk-btn.unlock {
  color: #fff;
}

.bulk-btn.cancel {
  background: transparent;
  border-color: var(--st-error);
  color: var(--st-error);
}

.bulk-btn:hover {
  filter: brightness(1.08);
}

.bulk-btn.cancel:hover {
  background: var(--st-error-soft);
  filter: none;
}

.bulk-btn:active {
  transform: translateY(1px);
}

.bulk-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.bulk-divider {
  width: 1px;
  height: 20px;
  background: var(--app-border-3);
}

.bulk-progress {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 220px;
  max-width: 320px;
  flex: 1;
  justify-content: flex-end;
  font-size: 0.75rem;
  color: var(--app-text-3);
  font-variant-numeric: tabular-nums;
}

.progress-track {
  flex: 1;
  height: 5px;
  border-radius: 999px;
  background: var(--app-border-2);
  overflow: hidden;
}

.progress-track i {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: var(--bulk);
}

.progress-track i.indeterminate {
  width: 35%;
  animation: bulk-indeterminate 1.1s ease-in-out infinite;
}

@keyframes bulk-indeterminate {
  0% {
    transform: translateX(-100%);
  }
  100% {
    transform: translateX(320%);
  }
}

.bulk-counter {
  flex: none;
  color: var(--bulk);
  font-weight: 700;
}

.spinner {
  display: inline-block;
  width: 15px;
  height: 15px;
  border: 2px solid var(--app-border-3);
  border-top-color: var(--bulk, var(--app-accent));
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* ── 任务列表 ── */
.task-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.task-card {
  --st: var(--st-queued);
  position: relative;
  display: flex;
  gap: 14px;
  padding: 12px 14px 12px 17px;
  border: 1px solid var(--app-border-2);
  border-radius: 10px;
  background: var(--app-surface-2);
  cursor: pointer;
  transition:
    border-color 0.15s,
    background 0.15s;
}

.task-card[data-status='downloading'] {
  --st: var(--st-downloading);
}
.task-card[data-status='paused'] {
  --st: var(--st-paused);
}
.task-card[data-status='error'] {
  --st: var(--st-error);
}
.task-card[data-status='error_lock'] {
  --st: var(--st-lock);
}
.task-card[data-status='completed'] {
  --st: var(--st-completed);
}
.task-card[data-status='cancelled'] {
  --st: var(--st-cancelled);
}

.task-card::before {
  content: '';
  position: absolute;
  top: 13px;
  bottom: 13px;
  left: 0;
  width: 3px;
  border-radius: 0 3px 3px 0;
  background: var(--st);
  opacity: 0.8;
}

.task-card:hover {
  border-color: rgba(0, 122, 204, 0.55);
  background: var(--app-surface-2-hover);
}

.cover {
  position: relative;
  display: grid;
  place-items: center;
  flex: none;
  width: 56px;
  height: 76px;
  border-radius: 6px;
  overflow: hidden;
  background: linear-gradient(145deg, hsl(var(--h1) 34% 32%), hsl(var(--h2) 38% 17%));
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
}

.cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.cover-initial {
  color: rgba(255, 255, 255, 0.9);
  font-size: 1.45rem;
  font-weight: 700;
  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.35);
}

.task-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.task-head {
  display: flex;
  align-items: center;
  gap: 9px;
  min-width: 0;
}

.task-title {
  flex: 1;
  min-width: 0;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--app-text-strong);
  font-size: 0.9rem;
  font-weight: 600;
}

.speed {
  flex: none;
  color: var(--st-downloading);
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Liberation Mono', monospace;
  font-size: 0.74rem;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex: none;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 0.69rem;
  line-height: 1.45;
  white-space: nowrap;
}

.status-badge .sdot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
}

.status-badge[data-status='queued'] {
  color: var(--st-queued);
  background: var(--st-queued-soft);
}
.status-badge[data-status='downloading'] {
  color: var(--st-downloading);
  background: var(--st-downloading-soft);
}
.status-badge[data-status='paused'] {
  color: var(--st-paused);
  background: var(--st-paused-soft);
}
.status-badge[data-status='error'] {
  color: var(--st-error);
  background: var(--st-error-soft);
}
.status-badge[data-status='error_lock'] {
  color: var(--st-lock);
  background: var(--st-lock-soft);
}
.status-badge[data-status='completed'] {
  color: var(--st-completed);
  background: var(--st-completed-soft);
}
.status-badge[data-status='cancelled'] {
  color: var(--st-cancelled);
  background: var(--st-cancelled-soft);
}

.chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.chip {
  padding: 1px 8px;
  border: 1px solid var(--app-border-3);
  border-radius: 999px;
  color: var(--app-text-2);
  font-size: 0.69rem;
  line-height: 1.6;
  white-space: nowrap;
}

.chip.mode.gallery {
  border-color: rgba(92, 184, 255, 0.4);
  color: var(--st-downloading);
}

.chip.mode.archive {
  border-color: rgba(230, 162, 60, 0.42);
  color: var(--st-paused);
}

.chip.priority {
  border-color: rgba(230, 162, 60, 0.42);
  color: var(--st-paused);
}

.chip.priority.zero {
  border-color: var(--app-border-3);
  color: var(--app-text-3);
}

.chip.update {
  border-color: rgba(255, 117, 136, 0.42);
  color: #ff7588;
}

:root[data-theme='light'] .chip.update {
  color: #c2185b;
  border-color: rgba(194, 24, 91, 0.3);
}

.chip.user {
  border-color: rgba(171, 71, 188, 0.42);
  color: #ab47bc;
}

:root[data-theme='light'] .chip.user {
  color: #7b1fa2;
}

.task-error {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 4px 8px;
  border: 1px solid rgba(230, 57, 70, 0.28);
  border-radius: 6px;
  background: var(--st-error-soft);
  color: var(--st-error);
  font-size: 0.72rem;
  line-height: 1.45;
}

.task-error .err-icon {
  flex: none;
}

.task-error .err-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.progress-bar {
  height: 5px;
  border-radius: 999px;
  background: var(--app-border-2);
  overflow: hidden;
}

.progress-bar i {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: var(--st);
  transition: width 0.25s ease;
}

.task-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  min-width: 0;
}

.progress-info {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--app-text-3);
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Liberation Mono', monospace;
  font-size: 0.73rem;
}

.progress-info b {
  color: var(--app-text-2);
  font-weight: 600;
}

.card-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 6px;
  flex: none;
}

.prio {
  display: inline-flex;
  align-items: center;
  gap: 1px;
  padding: 1px 3px;
  border: 1px solid rgba(230, 162, 60, 0.35);
  border-radius: 6px;
  color: var(--st-paused);
  font-size: 0.7rem;
  line-height: 1.4;
}

.prio.locked {
  opacity: 0.55;
}

.prio button {
  width: 18px;
  height: 20px;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  font-size: 0.85rem;
  line-height: 1;
  cursor: pointer;
}

.prio button:hover:not(:disabled) {
  background: var(--st-paused-soft);
}

.prio button:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.prio-val {
  min-width: 18px;
  text-align: center;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-variant-numeric: tabular-nums;
}

.card-btn {
  padding: 3px 10px;
  border: 1px solid;
  border-radius: 6px;
  background: transparent;
  font-size: 0.74rem;
  line-height: 1.5;
  cursor: pointer;
  transition: background 0.15s;
}

.card-btn.pause,
.card-btn.retry {
  border-color: var(--app-accent);
  color: var(--st-downloading);
}

.card-btn.pause:hover,
.card-btn.retry:hover {
  background: var(--st-downloading-soft);
}

.card-btn.resume {
  border-color: var(--st-completed);
  color: var(--st-completed);
}

.card-btn.resume:hover {
  background: var(--st-completed-soft);
}

.card-btn.lock {
  border-color: var(--st-lock);
  color: var(--st-lock);
}

.card-btn.lock:hover {
  background: var(--st-lock-soft);
}

.card-btn.cancel {
  border-color: rgba(230, 57, 70, 0.6);
  color: var(--st-error);
}

.card-btn.cancel:hover {
  background: var(--st-error-soft);
}

.card-btn:disabled,
.bulk-busy .card-btn,
.bulk-busy .prio button,
.bulk-busy .page-btn {
  opacity: 0.5;
  cursor: not-allowed;
  pointer-events: none;
}

/* ── 空态 / 分页 / 无权限 ── */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 54px 20px;
  border: 1px dashed var(--app-border-3);
  border-radius: 12px;
  background: var(--app-surface-2);
  text-align: center;
}

.empty-icon {
  font-size: 1.9rem;
  opacity: 0.85;
}

.empty-state h3 {
  margin: 0;
  color: var(--app-text-strong);
  font-size: 0.95rem;
  font-weight: 600;
}

.empty-state p {
  margin: 0;
  color: var(--app-text-3);
  font-size: 0.78rem;
}

.loading-box {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 160px;
  color: var(--app-text-3);
  font-size: 0.9rem;
}

.no-permission-box {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 240px;
  font-size: 1rem;
  color: var(--app-text-2);
  border: 1px dashed var(--app-border-3);
  border-radius: 12px;
}

.pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 14px;
  padding-top: 4px;
}

.page-btn {
  padding: 6px 16px;
  border: 1px solid var(--app-border-2);
  border-radius: 6px;
  background: var(--app-surface-2);
  color: var(--app-text-2);
  font-size: 0.8rem;
  cursor: pointer;
}

.page-btn:hover:not(:disabled) {
  border-color: var(--app-accent);
  color: var(--app-text-strong);
}

.page-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.page-info {
  color: var(--app-text-3);
  font-size: 0.78rem;
}

.page-info b {
  color: var(--app-text-strong);
  font-variant-numeric: tabular-nums;
}

/* ── 窄屏 ── */
@media (max-width: 1024px) {
  .downloads-page {
    padding: 12px;
    gap: 12px;
  }

  .page-title {
    font-size: 1.15rem;
  }

  .live {
    padding-left: 0;
    border-left: 0;
  }

  .filter-card {
    padding: 10px 10px 2px;
  }

  .seg button {
    padding: 5px 11px;
    font-size: 0.76rem;
  }

  .filter-hint {
    display: none;
  }

  .tab {
    padding: 8px 9px 9px;
    font-size: 0.78rem;
  }

  .tabs-wrap::before,
  .tabs-wrap::after {
    display: block;
  }

  .bulk-bar {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;
    padding: 10px 12px;
  }

  .bulk-actions {
    display: flex;
    gap: 8px;
  }

  .bulk-btn {
    flex: 1;
    padding: 9px 10px;
  }

  .bulk-divider {
    display: none;
  }

  .bulk-progress {
    max-width: none;
    min-width: 0;
    justify-content: space-between;
  }

  .bulk-progress .progress-track {
    flex: 1;
  }

  .task-card {
    gap: 10px;
    padding: 10px 10px 10px 14px;
  }

  .cover {
    width: 46px;
    height: 62px;
  }

  .cover-initial {
    font-size: 1.2rem;
  }

  .task-title {
    font-size: 0.84rem;
  }

  .status-badge {
    font-size: 0.65rem;
    padding: 2px 7px;
  }

  .chip {
    font-size: 0.66rem;
  }

  .chip.user,
  .chip.priority {
    display: none;
  }

  .task-foot {
    flex-direction: column;
    align-items: stretch;
    gap: 7px;
  }

  .progress-info {
    white-space: normal;
  }

  .card-actions {
    justify-content: flex-start;
  }

  .card-btn {
    padding: 4px 12px;
    font-size: 0.74rem;
  }

  .pagination {
    gap: 8px;
  }

  .page-btn {
    padding: 6px 11px;
  }
}

@media (max-width: 480px) {
  .page-header {
    align-items: flex-start;
  }

  .page-meta {
    gap: 8px;
  }

  .live {
    font-size: 0.72rem;
  }

  .task-head {
    flex-wrap: wrap;
    gap: 4px 8px;
  }

  .task-title {
    flex-basis: 100%;
  }

  .speed {
    order: 2;
  }

  .status-badge {
    order: 3;
    margin-left: auto;
  }

  .pagination {
    gap: 8px;
  }

  .page-btn {
    padding: 6px 11px;
  }

  .bulk-title {
    font-size: 0.82rem;
  }
}

.bulk-busy .tab,
.bulk-busy .seg button {
  pointer-events: none;
}

@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
</style>
