<template>
  <div class="log-settings">
    <!-- ── 日志开关（Round25：由原「高级」页并入）── -->
    <div class="section-title">⚙️ 日志开关</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">启用系统日志</div>
        <div class="item-subtext">记录更新检测 / 维护查重 / 下载任务 / 扫描等操作日志到 backend/logs（下方「监控 / 查询」实时查看）</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advancedSettings.systemLogsEnabled" @change="handleSystemLogsToggle" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">前端错误上报</div>
        <div class="item-subtext">将前端运行错误上报到服务端 logs/client.log（「前端错误」Tab 查看）</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advancedSettings.enableLogs" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item clickable" @click="openClearModal">
      <div class="item-info">
        <div class="item-title">清除日志</div>
        <div class="item-subtext">按类别与时间范围精细清理系统日志与前端错误日志（当前共占用 {{ totalLogSize }}）</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <!-- 各分类占用总览 -->
    <div class="size-overview">
      <div v-for="cat in categories" :key="cat.category" class="size-chip">
        <span class="chip-label">{{ cat.label }}</span>
        <span class="chip-size">{{ catTotalSize(cat) }}</span>
      </div>
      <div class="size-chip">
        <span class="chip-label">前端错误</span>
        <span class="chip-size">{{ formatSize(clientSize) }}</span>
      </div>
    </div>

    <!-- 子 Tab：监控 / 查询 -->
    <div class="sub-tabs">
      <button
        class="sub-tab"
        :class="{ active: subTab === 'monitor' }"
        @click="subTab = 'monitor'"
      >
        📡 监控
      </button>
      <button
        class="sub-tab"
        :class="{ active: subTab === 'query' }"
        @click="subTab = 'query'"
      >
        🔍 查询
      </button>
      <button
        class="sub-tab"
        :class="{ active: subTab === 'client' }"
        @click="subTab = 'client'"
      >
        🐞 前端错误
      </button>
    </div>

    <!-- ── 监控：实时滚动终端 ── -->
    <div v-show="subTab === 'monitor'" class="monitor-panel">
      <div class="monitor-toolbar">
        <select v-model="monitorCategory" class="cat-select">
          <option v-for="opt in categoryOptions" :key="opt.value" :value="opt.value">
            {{ opt.label }}
          </option>
        </select>

        <div class="toolbar-spacer"></div>

        <label class="auto-scroll-label">
          <input type="checkbox" v-model="monitorAutoScroll" />
          自动滚动
        </label>
        <button class="toolbar-btn" @click="togglePause">
          {{ monitorPaused ? '▶ 继续' : '⏸ 暂停' }}
        </button>
        <button class="toolbar-btn" @click="clearMonitor">🗑 清屏</button>
      </div>

      <div
        ref="terminalRef"
        class="terminal"
        @scroll.passive="onTerminalScroll"
        @wheel.passive="markUserScroll"
        @touchmove.passive="markUserScroll"
      >
        <div v-if="monitorLines.length === 0" class="terminal-empty">
          {{ monitorPaused ? '已暂停，等待继续…' : '暂无日志输出…' }}
        </div>
        <div
          v-for="line in monitorLines"
          :key="line.id"
          class="terminal-line"
          :class="lineClass(line.text)"
        >
          {{ line.text }}
        </div>
      </div>
    </div>

    <!-- ── 查询：类目 / 日期 / 关键词 / 分页 ── -->
    <div v-show="subTab === 'query'" class="query-panel">
      <div class="query-bar">
        <select v-model="queryCategory" class="cat-select">
          <option v-for="opt in categoryOptions" :key="opt.value" :value="opt.value">
            {{ opt.label }}
          </option>
        </select>

        <select v-model="queryDate" class="cat-select">
          <option v-for="d in availableDates" :key="d" :value="d">{{ d }}</option>
        </select>

        <input
          v-model="queryKeyword"
          type="text"
          class="keyword-input"
          placeholder="关键词（可空）…"
          @keyup.enter="runQuery(0)"
        />

        <button class="query-btn" :disabled="queryLoading" @click="runQuery(0)">
          {{ queryLoading ? '查询中…' : '查询' }}
        </button>
      </div>

      <div class="query-meta">
        共 {{ queryTotal }} 条
        <span v-if="queryOffset + queryLines.length < queryTotal" class="meta-hint">
          （仅显示第 {{ queryOffset + 1 }}–{{ queryOffset + queryLines.length }} 条）
        </span>
      </div>

      <div class="query-result">
        <div v-if="queryLines.length === 0" class="query-empty">无匹配日志</div>
        <div v-for="(line, i) in queryLines" :key="i" class="query-line" :class="lineClass(line)">
          {{ line }}
        </div>
      </div>

      <div class="query-pager">
        <button class="pager-btn" :disabled="queryOffset <= 0" @click="runQuery(Math.max(0, queryOffset - queryLimit))">
          上一页
        </button>
        <span class="pager-info">
          第 {{ queryPage }} / {{ totalPages }} 页
        </span>
        <button
          class="pager-btn"
          :disabled="queryOffset + queryLimit >= queryTotal"
          @click="runQuery(queryOffset + queryLimit)"
        >
          下一页
        </button>
      </div>
    </div>

    <!-- ── 前端错误：浏览器上报（POST /client/log）落盘的 client.log 内容 ── -->
    <div v-show="subTab === 'client'" class="query-panel">
      <div class="query-bar">
        <span class="meta-hint">浏览器崩溃/报错自动上报（logs/client.log），最新在前</span>
        <div class="toolbar-spacer"></div>
        <button class="query-btn" :disabled="clientLoading" @click="fetchClientLogs(0)">
          {{ clientLoading ? '加载中…' : '刷新' }}
        </button>
      </div>

      <div class="query-meta">
        共 {{ clientTotal }} 条
        <span v-if="clientEntries.length" class="meta-hint">
          （第 {{ clientOffset + 1 }}–{{ clientOffset + clientEntries.length }} 条）
        </span>
      </div>

      <div v-if="clientEntries.length === 0 && !clientLoading" class="query-empty">
        暂无前端错误记录
      </div>
      <div v-for="(e, i) in clientEntries" :key="i" class="client-log-entry">
        <div class="client-log-head">
          <span class="client-log-level" :class="'lv-' + (e.level || 'info')">
            {{ (e.level || 'info').toUpperCase() }}
          </span>
          <span class="client-log-ts">{{ formatTs(e.ts) }}</span>
          <span class="client-log-msg">{{ e.message }}</span>
        </div>
        <div v-if="e.url || e.info" class="client-log-meta">
          {{ e.url }}{{ e.info ? ' · ' + e.info : '' }}
        </div>
        <details v-if="e.stack" class="client-log-details">
          <summary>堆栈</summary>
          <pre class="client-log-stack">{{ e.stack }}</pre>
        </details>
      </div>

      <div class="query-pager">
        <button
          class="pager-btn"
          :disabled="clientOffset <= 0"
          @click="fetchClientLogs(Math.max(0, clientOffset - clientLimit))"
        >
          上一页
        </button>
        <span class="pager-info">第 {{ clientPage }} / {{ clientPages }} 页</span>
        <button
          class="pager-btn"
          :disabled="clientOffset + clientLimit >= clientTotal"
          @click="fetchClientLogs(clientOffset + clientLimit)"
        >
          下一页
        </button>
      </div>
    </div>

    <!-- ── 清除日志精细管理弹窗（Round25：由原「高级」页并入）── -->
    <Transition name="fade">
      <div v-if="clearModalOpen" class="clear-mask" @click.self="clearModalOpen = false">
        <div class="clear-card">
          <h3 class="clear-title">清除日志</h3>

          <div class="clear-field">
            <div class="field-label">目标</div>
            <div class="option-group">
              <button
                v-for="opt in clearTargets"
                :key="opt.value"
                class="option-btn"
                :class="{ active: clearTarget === opt.value }"
                @click="clearTarget = opt.value"
              >
                {{ opt.label }}
              </button>
            </div>
          </div>

          <div class="clear-field">
            <div class="field-label">范围</div>
            <div class="option-group">
              <button
                v-for="opt in clearRanges"
                :key="opt.value"
                class="option-btn"
                :class="{ active: clearRange === opt.value }"
                @click="clearRange = opt.value"
              >
                {{ opt.label }}
              </button>
            </div>
            <input
              v-if="clearRange === 'custom'"
              v-model="clearCustomDate"
              type="date"
              class="date-input"
              :max="todayStr"
            />
          </div>

          <div class="clear-actions">
            <button class="btn btn-cancel" @click="clearModalOpen = false">取消</button>
            <button class="btn btn-danger" :disabled="!canSubmitClear" @click="handleClearLogs">确认清除</button>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
// Round25：日志开关与清除由原「高级」页并入
import { advancedSettings } from '@/stores/advancedSettings'

const { toast } = useUI()

// ── 日志开关（Round25）──
/** 日志占用总览（后端 /logs/categories 返回四类 + 前端错误日志大小） */
const totalLogSize = ref('0B')

/** 拉取各类日志大小并汇总（复用下方 LogCategoriesResp 类型） */
const fetchLogSizes = async () => {
  try {
    const data = await http<LogCategoriesResp>('/logs/categories')
    let total = 0
    for (const cat of data.categories ?? []) {
      for (const f of cat.files ?? []) {
        total += typeof f.size === 'number' ? f.size : 0
      }
    }
    if (typeof data.client?.size === 'number') total += data.client.size
    totalLogSize.value = formatSize(total)
  } catch (err) {
    console.error('获取日志大小失败:', err)
    totalLogSize.value = '0B'
  }
}

/** 切换「启用系统日志」时持久化到后端（本地已由 store 自动持久化） */
const handleSystemLogsToggle = async () => {
  try {
    await http('/logs/settings', {
      method: 'POST',
      body: JSON.stringify({ systemLogsEnabled: advancedSettings.systemLogsEnabled }),
    })
    toast.success(advancedSettings.systemLogsEnabled ? '已启用系统日志' : '已停用系统日志落盘')
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '保存日志设置失败')
  }
}

// ── 清除日志精细管理（Round25）──
const clearModalOpen = ref(false)
const clearTarget = ref('')
const clearRange = ref('keep7')
const clearCustomDate = ref('')

const clearTargets = [
  { value: '', label: '全部' },
  { value: 'update', label: '更新' },
  { value: 'maintain', label: '维护' },
  { value: 'download', label: '下载' },
  { value: 'other', label: '其他' },
  { value: 'client', label: '前端错误' },
]

const clearRanges = [
  { value: 'keep7', label: '保留最近 7 天' },
  { value: 'keep30', label: '保留最近 30 天' },
  { value: 'keep90', label: '保留最近 90 天' },
  { value: 'all', label: '清除全部' },
  { value: 'custom', label: '清除指定日期之前' },
]

const todayStr = computed(() => {
  const d = new Date()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${mm}-${dd}`
})

const canSubmitClear = computed(() => clearRange.value !== 'custom' || clearCustomDate.value !== '')

const openClearModal = () => {
  clearTarget.value = ''
  clearRange.value = 'keep7'
  clearCustomDate.value = ''
  clearModalOpen.value = true
}

/** 计算 DELETE /logs 的 before 参数（不含 → 清除全部） */
const buildBeforeParam = (): string | undefined => {
  if (clearRange.value === 'all') return undefined
  if (clearRange.value === 'custom') return clearCustomDate.value
  const keepDays = parseInt(clearRange.value.replace('keep', ''), 10) || 7
  const d = new Date()
  d.setDate(d.getDate() - keepDays)
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${mm}-${dd}`
}

/** 执行精细清除 */
const handleClearLogs = async () => {
  const before = buildBeforeParam()
  try {
    await http('/logs', {
      method: 'DELETE',
      params: { category: clearTarget.value || undefined, before },
    })
    toast.success('日志已清除')
    clearModalOpen.value = false
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '清除日志失败')
  }
  fetchLogSizes()
}

// ── 分类总览数据（GET /logs/categories）──
interface LogFileInfo {
  date: string
  size: number
}
interface LogCatInfo {
  category: string
  label: string
  files: LogFileInfo[]
}
interface LogCategoriesResp {
  categories?: LogCatInfo[]
  client?: { category: string; label: string; size: number }
}

const categories = ref<LogCatInfo[]>([])
const clientSize = ref(0)

const categoryOptions = computed(() =>
  categories.value.map((c) => ({ value: c.category, label: c.label })),
)

/** 单个分类各日文件大小合计 */
const catTotalSize = (cat: LogCatInfo): string =>
  formatSize((cat.files ?? []).reduce((sum, f) => sum + (f.size || 0), 0))

const formatSize = (bytes: number): string => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 ? 0 : 1)}${units[i]}`
}

// ── 子 Tab ──
const subTab = ref<'monitor' | 'query' | 'client'>('monitor')

// ── 监控 ──
interface LogTailLine {
  ts: number
  text: string
}

/** 终端保留的最大行数。
 *  Round41：原为 2000 行 —— 超过上限后 slice(-2000) 使全体行索引位移，
 *  Vue 按索引 key 匹配会把 2000 个节点逐行改写，且代价与新增行数无关
 *  （只要有 1 行新行就整表重写）。实测真实 Chrome 下滚动阶段约 1 核持续满载。 */
const MONITOR_MAX_LINES = 300
/** 增量轮询单次接收上限（安全阀：日志暴增时避免一次性灌入，终端只保留尾部若干行） */
const MONITOR_FETCH_LIMIT = 600
/** 判定「贴底跟随」的容差（px） */
const MONITOR_STICK_PX = 24
/** 认定某次滚动属于"用户翻历史"的时间窗（ms）：超出该窗口的滚动视为内容更新/滚动锚定引起 */
const USER_SCROLL_WINDOW_MS = 600

interface MonitorLine extends LogTailLine {
  /** 前端自增 id：为 v-for 提供稳定 key（索引 key 是整表重写的根因） */
  id: number
}

const monitorCategory = ref('update')
const monitorLines = ref<MonitorLine[]>([])
const monitorPaused = ref(false)
const monitorAutoScroll = ref(true)
/** 终端是否贴底：用户往上翻历史时置 false（不再被新日志拽回底部），滚回底部自动恢复 */
const stickToBottom = ref(true)
const terminalRef = ref<HTMLElement | null>(null)
const lastTs = ref(0)
let pollTimer: ReturnType<typeof setInterval> | null = null
let lineSeq = 0
let scrollRaf: number | null = null

/** 行着色：错误红 / 警告黄 / 其余默认 */
const lineClass = (text: string): string => {
  if (/ERROR|失败|异常/.test(text)) return 'line-error'
  if (/WARN|警告|降级/.test(text)) return 'line-warn'
  return ''
}

/** 用户主动滚动输入（滚轮/触摸）的最近时刻：用于区分"用户翻历史"与"内容更新/滚动锚定" */
let lastUserScrollInput = 0
const markUserScroll = () => {
  lastUserScrollInput = Date.now()
}

/** 滚动处理：
 *  - 到达/回到底部 → 恢复跟随（任何来源都可恢复）；
 *  - 离开底部 → 仅在"刚发生过用户滚动输入"时解除跟随，
 *    否则内容增删/浏览器滚动锚定引起的位置变化会被误判成用户翻历史，导致跟随意外中断。 */
const onTerminalScroll = () => {
  const el = terminalRef.value
  if (!el) return
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight <= MONITOR_STICK_PX
  if (atBottom) {
    stickToBottom.value = true
    return
  }
  if (Date.now() - lastUserScrollInput < USER_SCROLL_WINDOW_MS) stickToBottom.value = false
}

/** 追加新行后按需滚到底：rAF 合并到帧末，每帧最多一次 */
const scrollTerminal = () => {
  if (!monitorAutoScroll.value || !stickToBottom.value) return
  if (scrollRaf !== null) return
  scrollRaf = requestAnimationFrame(() => {
    scrollRaf = null
    const el = terminalRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

const pollLogs = async () => {
  const firstLoad = lastTs.value <= 0
  try {
    const data = await http<{ lines: LogTailLine[] }>('/logs/tail', {
      params: {
        category: monitorCategory.value,
        // 首屏/切换类别：只取尾部 N 行（原先不带 since → 全量拉整天日志，实测单次可达 2MB）
        since: firstLoad ? undefined : lastTs.value,
        limit: firstLoad ? MONITOR_MAX_LINES : MONITOR_FETCH_LIMIT,
      },
    })
    const lines = data.lines ?? []
    if (lines.length === 0) return
    const appended: MonitorLine[] = lines.map((l) => ({ id: ++lineSeq, ts: l.ts, text: l.text }))
    const merged = monitorLines.value.concat(appended)
    monitorLines.value =
      merged.length > MONITOR_MAX_LINES ? merged.slice(merged.length - MONITOR_MAX_LINES) : merged
    const maxTs = lines.reduce((m, l) => (l.ts > m ? l.ts : m), 0)
    if (maxTs > lastTs.value) lastTs.value = maxTs
    scrollTerminal()
  } catch {
    /* 轮询失败静默，下一轮重试 */
  }
}

const startPolling = () => {
  stopPolling()
  pollTimer = setInterval(() => {
    if (!monitorPaused.value) void pollLogs()
  }, 1000)
}

const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

/** 页面切到后台停轮询、恢复可见时立即补一次（长时间后台不该持续拉日志） */
const handleVisibilityChange = () => {
  if (document.hidden) {
    stopPolling()
    return
  }
  void pollLogs()
  startPolling()
}

const togglePause = () => {
  monitorPaused.value = !monitorPaused.value
}

const clearMonitor = () => {
  monitorLines.value = []
  lastTs.value = 0
  stickToBottom.value = true
}

// 切换监控类目：清空并立即拉取一次（走首屏路径，只取尾部 N 行）
watch(monitorCategory, () => {
  monitorLines.value = []
  lastTs.value = 0
  stickToBottom.value = true
  void pollLogs()
})

// ── 查询 ──
const queryCategory = ref('update')
const queryDate = ref('')
const queryKeyword = ref('')
const queryLines = ref<string[]>([])
const queryTotal = ref(0)
const queryOffset = ref(0)
const queryLimit = 50
const queryLoading = ref(false)

const availableDates = computed(() => {
  const cat = categories.value.find((c) => c.category === queryCategory.value)
  return (cat?.files ?? []).map((f) => f.date)
})

const queryPage = computed(() => Math.floor(queryOffset.value / queryLimit) + 1)
const totalPages = computed(() => Math.max(1, Math.ceil(queryTotal.value / queryLimit)))

const runQuery = async (pageOffset: number) => {
  queryLoading.value = true
  try {
    const data = await http<{ total: number; lines: string[] }>('/logs/query', {
      params: {
        category: queryCategory.value,
        date: queryDate.value || undefined,
        keyword: queryKeyword.value || undefined,
        offset: pageOffset,
        limit: queryLimit,
      },
    })
    queryTotal.value = data.total ?? 0
    queryLines.value = data.lines ?? []
    queryOffset.value = pageOffset
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '查询日志失败')
  } finally {
    queryLoading.value = false
  }
}

watch(queryCategory, () => {
  const first = availableDates.value[0] ?? ''
  queryDate.value = first
  runQuery(0)
})

// ── 前端错误（GET /client/log，倒序分页）──
interface ClientLogEntry {
  ts: string
  level: string
  message: string
  stack?: string
  url?: string
  info?: string
}

const clientEntries = ref<ClientLogEntry[]>([])
const clientTotal = ref(0)
const clientOffset = ref(0)
const clientLimit = 50
const clientLoading = ref(false)

const clientPage = computed(() => Math.floor(clientOffset.value / clientLimit) + 1)
const clientPages = computed(() => Math.max(1, Math.ceil(clientTotal.value / clientLimit)))

const fetchClientLogs = async (offset: number) => {
  clientLoading.value = true
  try {
    const data = await http<{ total: number; entries: ClientLogEntry[] }>('/client/log', {
      params: { offset, limit: clientLimit },
    })
    clientTotal.value = data.total ?? 0
    clientEntries.value = data.entries ?? []
    clientOffset.value = offset
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '获取前端错误日志失败')
  } finally {
    clientLoading.value = false
  }
}

/** ISO 时间戳 → 本地可读（解析失败原样返回） */
const formatTs = (iso: string): string => {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

watch(subTab, (tab) => {
  if (tab === 'client') void fetchClientLogs(0)
})

onMounted(async () => {
  // 拉取分类总览（可用日期来自各分类文件列表）
  try {
    const data = await http<LogCategoriesResp>('/logs/categories')
    categories.value = data.categories ?? []
    if (typeof data.client?.size === 'number') clientSize.value = data.client.size
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '获取日志分类失败')
  }
  // Round25：初始化日志占用总览 + 从后端读取系统日志开关（后端为准）
  fetchLogSizes()
  try {
    const data = await http<{ systemLogsEnabled?: boolean }>('/logs/settings')
    if (typeof data.systemLogsEnabled === 'boolean') {
      advancedSettings.systemLogsEnabled = data.systemLogsEnabled
    }
  } catch {
    /* 后端不可达时保留本地默认值 */
  }
  // 初始化查询日期
  queryDate.value = availableDates.value[0] ?? ''
  // 启动监控轮询（Round41：页面隐藏时自动停、恢复可见时自动续）
  document.addEventListener('visibilitychange', handleVisibilityChange)
  void pollLogs()
  startPolling()
})

onBeforeUnmount(() => {
  stopPolling()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  if (scrollRaf !== null) {
    cancelAnimationFrame(scrollRaf)
    scrollRaf = null
  }
})
</script>

<style scoped>
.log-settings {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

/* ── 日志开关区（Round25：由原「高级」页并入）── */
.section-title {
  font-size: 13px;
  font-weight: 600;
  color: #ff7588;
  letter-spacing: 0.5px;
  margin: 12px 0 4px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--app-border-2);
}

.setting-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background-color: var(--app-surface-2);
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
  transition: background-color 0.2s ease;
}

.setting-item.clickable {
  cursor: pointer;
}

.setting-item.clickable:hover {
  background-color: var(--app-surface-2-hover);
}

.item-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-right: 16px;
}

.item-title {
  font-size: 15px;
  font-weight: 500;
  color: var(--app-text-strong);
}

.item-subtext {
  font-size: 13px;
  color: var(--app-text-3);
  line-height: 1.4;
}

.arrow-icon {
  font-size: 22px;
  color: var(--app-text-muted);
  flex-shrink: 0;
}

.toggle-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
  flex-shrink: 0;
}

.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--app-border-3);
  transition: 0.3s;
  border-radius: 24px;
}

.slider:before {
  position: absolute;
  content: '';
  height: 18px;
  width: 18px;
  left: 3px;
  bottom: 3px;
  background-color: var(--app-text-2);
  transition: 0.3s;
  border-radius: 50%;
}

input:checked + .slider {
  background-color: #ff7588;
}

input:checked + .slider:before {
  transform: translateX(20px);
  background-color: #ffffff;
}

/* ── 清除日志弹窗（Round25：由原「高级」页并入）── */
.clear-mask {
  position: fixed;
  inset: 0;
  z-index: 9998;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
}

.clear-card {
  width: 92%;
  max-width: 460px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  border-radius: 12px;
  padding: 20px 24px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5);
}

.clear-title {
  margin: 0 0 16px 0;
  font-size: 18px;
  color: var(--app-text-strong);
}

.clear-field {
  margin-bottom: 16px;
}

.field-label {
  font-size: 13px;
  color: var(--app-text-3);
  margin-bottom: 8px;
}

.option-group {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.option-btn {
  padding: 6px 12px;
  border-radius: 6px;
  border: 1px solid var(--app-border-3);
  background: var(--app-surface-2);
  color: var(--app-text-2);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.option-btn:hover {
  border-color: #ff7588;
  color: var(--app-text-strong);
}

.option-btn.active {
  border-color: #ff7588;
  background: rgba(255, 117, 136, 0.12);
  color: var(--app-text-strong);
}

.date-input {
  margin-top: 10px;
  width: 100%;
  padding: 8px 10px;
  background: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  color: var(--app-text-strong);
  font-size: 14px;
  box-sizing: border-box;
  color-scheme: dark;
}

.clear-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 8px;
}

.btn {
  padding: 8px 18px;
  border-radius: 6px;
  border: none;
  font-size: 14px;
  cursor: pointer;
  transition: opacity 0.2s;
}

.btn:hover {
  opacity: 0.85;
}

.btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-cancel {
  background: var(--app-surface-3);
  color: var(--app-text-2);
  border: 1px solid var(--app-border-3);
}

.btn-danger {
  background: #d64045;
  color: #fff;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* 分类占用总览 */
.size-overview {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.size-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 6px;
  font-size: 13px;
}

.chip-label {
  color: var(--app-text-2);
}

.chip-size {
  color: #a891e3;
  font-family: monospace;
  font-weight: 600;
}

/* 子 Tab */
.sub-tabs {
  display: flex;
  gap: 8px;
  border-bottom: 1px solid var(--app-border-2);
  padding-bottom: 8px;
}

.sub-tab {
  background: transparent;
  border: none;
  color: var(--app-text-2);
  font-size: 14px;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.sub-tab:hover {
  color: var(--app-text-strong);
  background-color: var(--app-surface-2-hover);
}

.sub-tab.active {
  color: #ff7588;
  background: rgba(255, 117, 136, 0.1);
  font-weight: 500;
}

/* ── 监控 ── */
.monitor-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.monitor-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.toolbar-spacer {
  flex: 1;
}

.auto-scroll-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--app-text-2);
  cursor: pointer;
}

.toolbar-btn,
.query-btn {
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  font-size: 13px;
  padding: 6px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.toolbar-btn:hover,
.query-btn:hover {
  border-color: #ff7588;
  color: var(--app-text-strong);
}

.cat-select {
  background: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-strong);
  font-size: 13px;
  padding: 6px 10px;
  border-radius: 6px;
  outline: none;
  color-scheme: dark;
}

.terminal {
  height: 360px;
  overflow-y: auto;
  /* 禁用滚动锚定：日志终端需要"贴底跟随"，浏览器锚定会在内容增删时改写 scrollTop，
     既让视图跳动、又会造成"用户翻历史"的误判（Round41） */
  overflow-anchor: none;
  background: #0d1117;
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  padding: 10px 12px;
  font-family: 'Consolas', 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

.terminal-empty,
.query-empty {
  color: var(--app-text-3);
  font-size: 13px;
  padding: 12px 4px;
}

.terminal-line {
  color: #7ee787;
}

.terminal-line.line-warn,
.query-line.line-warn {
  color: #e3b341;
}

.terminal-line.line-error,
.query-line.line-error {
  color: #ff6b6b;
}

/* ── 查询 ── */
.query-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.query-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.keyword-input {
  flex: 1;
  min-width: 160px;
  background: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-strong);
  font-size: 13px;
  padding: 6px 10px;
  border-radius: 6px;
  outline: none;
}

.keyword-input:focus {
  border-color: #007acc;
}

.query-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.query-meta {
  font-size: 13px;
  color: var(--app-text-2);
}

.meta-hint {
  color: var(--app-text-3);
}

.query-result {
  max-height: 380px;
  overflow-y: auto;
  background: #0d1117;
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  padding: 10px 12px;
  font-family: 'Consolas', 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

.query-line {
  color: #a5d6ff;
}

.query-pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 4px 0;
}

.pager-btn {
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  font-size: 13px;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.pager-btn:hover:not(:disabled) {
  border-color: #ff7588;
  color: var(--app-text-strong);
}

.pager-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.pager-info {
  font-size: 13px;
  color: var(--app-text-2);
}

/* ── 前端错误（client.log 条目）── */
.client-log-entry {
  background: #0d1117;
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 13px;
  line-height: 1.5;
}

.client-log-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.client-log-level {
  font-size: 11px;
  font-weight: 600;
  padding: 1px 7px;
  border-radius: 10px;
  background: var(--app-surface-2);
  color: var(--app-text-2);
  font-family: 'Consolas', 'Courier New', monospace;
}

.client-log-level.lv-error {
  background: rgba(255, 107, 107, 0.15);
  color: #ff6b6b;
}

.client-log-level.lv-warn {
  background: rgba(227, 179, 65, 0.15);
  color: #e3b341;
}

.client-log-ts {
  font-size: 12px;
  color: var(--app-text-3);
  font-family: 'Consolas', 'Courier New', monospace;
}

.client-log-msg {
  color: var(--app-text-strong);
  word-break: break-all;
  flex: 1;
}

.client-log-meta {
  margin-top: 4px;
  font-size: 12px;
  color: var(--app-text-3);
  word-break: break-all;
}

.client-log-details {
  margin-top: 4px;
}

.client-log-details summary {
  font-size: 12px;
  color: var(--app-text-2);
  cursor: pointer;
  user-select: none;
}

.client-log-stack {
  margin-top: 6px;
  max-height: 240px;
  overflow: auto;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 8px 10px;
  font-family: 'Consolas', 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  color: #a5d6ff;
}
</style>
