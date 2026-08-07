<template>
  <div class="log-settings">
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

      <div ref="terminalRef" class="terminal">
        <div v-if="monitorLines.length === 0" class="terminal-empty">
          {{ monitorPaused ? '已暂停，等待继续…' : '暂无日志输出…' }}
        </div>
        <div
          v-for="(line, i) in monitorLines"
          :key="i"
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
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'

const { toast } = useUI()

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
const subTab = ref<'monitor' | 'query'>('monitor')

// ── 监控 ──
interface LogTailLine {
  ts: number
  text: string
}

const monitorCategory = ref('update')
const monitorLines = ref<LogTailLine[]>([])
const monitorPaused = ref(false)
const monitorAutoScroll = ref(true)
const terminalRef = ref<HTMLElement | null>(null)
const lastTs = ref(0)
let pollTimer: ReturnType<typeof setInterval> | null = null

/** 行着色：错误红 / 警告黄 / 其余默认 */
const lineClass = (text: string): string => {
  if (/ERROR|失败|异常/.test(text)) return 'line-error'
  if (/WARN|警告|降级/.test(text)) return 'line-warn'
  return ''
}

const scrollTerminal = () => {
  if (!monitorAutoScroll.value) return
  void nextTick(() => {
    const el = terminalRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

const pollLogs = async () => {
  try {
    const data = await http<{ lines: LogTailLine[] }>('/logs/tail', {
      params: { category: monitorCategory.value, since: lastTs.value > 0 ? lastTs.value : undefined },
    })
    const lines = data.lines ?? []
    if (lines.length > 0) {
      monitorLines.value.push(...lines)
      if (monitorLines.value.length > 2000) {
        monitorLines.value = monitorLines.value.slice(-2000)
      }
      const maxTs = lines.reduce((m, l) => (l.ts > m ? l.ts : m), 0)
      if (maxTs > lastTs.value) lastTs.value = maxTs
      scrollTerminal()
    }
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

const togglePause = () => {
  monitorPaused.value = !monitorPaused.value
}

const clearMonitor = () => {
  monitorLines.value = []
  lastTs.value = 0
}

// 切换监控类目：清空并立即拉取一次
watch(monitorCategory, () => {
  monitorLines.value = []
  lastTs.value = 0
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

onMounted(async () => {
  // 拉取分类总览（可用日期来自各分类文件列表）
  try {
    const data = await http<LogCategoriesResp>('/logs/categories')
    categories.value = data.categories ?? []
    if (typeof data.client?.size === 'number') clientSize.value = data.client.size
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '获取日志分类失败')
  }
  // 初始化查询日期
  queryDate.value = availableDates.value[0] ?? ''
  // 启动监控轮询
  void pollLogs()
  startPolling()
})

onBeforeUnmount(stopPolling)
</script>

<style scoped>
.log-settings {
  display: flex;
  flex-direction: column;
  gap: 14px;
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
</style>
