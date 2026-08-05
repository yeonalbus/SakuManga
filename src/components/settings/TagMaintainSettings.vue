<template>
  <div class="tag-maintain-settings">
    <!-- 双轨模型说明 -->
    <div class="intro-card">
      <div class="intro-title">🏷️ 双轨 Tag 维护说明</div>
      <div class="intro-text">
        <p>· <b>OnlineTags</b>：E站官方 tag，每日刷新时整体覆盖。</p>
        <p>· <b>OfflineAddTags</b>：本地新增 tag（客制化，不写回）。</p>
        <p>· <b>OfflineRemoveTags</b>：本地删除的 online tag（刷新时略过、写回时剔除）。</p>
        <p>每周反向写回仅写入 <b>OnlineTags − OfflineRemoveTags</b>，本地客制化不会写回。</p>
      </div>
    </div>

    <!-- 最近执行状态 -->
    <div class="status-card">
      <div class="status-row">
        <span class="status-key">上次每日刷新</span>
        <span class="status-val">{{ formatTime(setting.lastDailyRunAt) }}</span>
      </div>
      <div class="status-row">
        <span class="status-key">上次每周写回</span>
        <span class="status-val">{{ formatTime(setting.lastWeeklyRunAt) }}</span>
      </div>
    </div>

    <!-- 标签数据（翻译/排序）下载管理 -->
    <div class="setting-item column-layout">
      <div class="main-row">
        <div class="item-info">
          <div class="item-title">开启标签中文翻译</div>
          <div class="item-subtext">版本: {{ tagCNVersion }}</div>
        </div>
        <div class="item-actions">
          <button
            class="icon-action-btn"
            :class="{ spinning: transProgress.status === 'downloading' }"
            title="检查更新"
            @click="handleRefreshTagTranslation"
          >
            🔄
          </button>
          <label class="toggle-switch">
            <input type="checkbox" v-model="enableTagCNTranslation" />
            <span class="slider"></span>
          </label>
        </div>
      </div>

      <div v-if="transProgress.status === 'downloading'" class="progress-box">
        <div class="progress-bar-bg">
          <div
            class="progress-bar-fill"
            :style="{ width: transProgress.progress.toFixed(1) + '%' }"
          ></div>
        </div>
        <span class="tag-progress-text"
          >{{ transProgress.progress.toFixed(1) }}% ({{ formatSize(transProgress.downloaded) }} /
          {{ formatSize(transProgress.total) }})</span
        >
      </div>
    </div>

    <div class="setting-item column-layout">
      <div class="main-row">
        <div class="item-info">
          <div class="item-title">标签补全排序规则</div>
          <div class="item-subtext">版本: {{ tagSortVersion }}</div>
        </div>
        <div class="item-actions">
          <button
            class="icon-action-btn"
            :class="{ spinning: sortProgress.status === 'downloading' }"
            title="检查更新"
            @click="handleRefreshTagSort"
          >
            🔄
          </button>
          <label class="toggle-switch">
            <input type="checkbox" v-model="enableTagSortRules" />
            <span class="slider"></span>
          </label>
        </div>
      </div>

      <div v-if="sortProgress.status === 'downloading'" class="progress-box">
        <div class="progress-bar-bg">
          <div
            class="progress-bar-fill"
            :style="{ width: sortProgress.progress.toFixed(1) + '%' }"
          ></div>
        </div>
        <span class="tag-progress-text"
          >{{ sortProgress.progress.toFixed(1) }}% ({{ formatSize(sortProgress.downloaded) }} /
          {{ formatSize(sortProgress.total) }})</span
        >
      </div>
    </div>

    <div class="setting-item update-cycle-item">
      <div class="item-info">
        <div class="item-title">🔄 标签数据自动更新周期</div>
        <div class="item-subtext">
          启动时自动查找本地文件，若缺失或不是最新版本将自行下载；之后每
          {{ updateCycleHours }} 小时自动检查一次更新
        </div>
      </div>
    </div>

    <!-- 每日刷新 -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">每日 Tag 刷新</div>
        <div class="item-subtext">
          每天东八区 {{ setting.refreshHour }}:00 联网核对 E 站画廊，有变动则更新 OnlineTags
        </div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="setting.enableDailyRefresh" @change="saveSetting" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">每日刷新时刻</div>
        <div class="item-subtext">东八区小时（0-23），默认 6 点</div>
      </div>
      <input
        class="num-input"
        type="number"
        min="0"
        max="23"
        v-model.number="setting.refreshHour"
        @change="saveSetting"
      />
    </div>

    <!-- 每周写回 -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">每周反向写回 ComicInfo</div>
        <div class="item-subtext">
          每周{{ weekdayLabel }} {{ setting.writebackHour }}:00（东八区）将数据库 tag 写回
          ComicInfo.xml（zip/cbz 自动重打包）
        </div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="setting.enableWeeklyWriteback" @change="saveSetting" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">写回日</div>
        <div class="item-subtext">每周的哪一天执行反向写回</div>
      </div>
      <select class="select-input" v-model.number="setting.writebackWeekday" @change="saveSetting">
        <option v-for="(label, i) in weekdayLabels" :key="i" :value="i">{{ label }}</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">写回时刻</div>
        <div class="item-subtext">东八区小时（0-23），默认 6 点</div>
      </div>
      <input
        class="num-input"
        type="number"
        min="0"
        max="23"
        v-model.number="setting.writebackHour"
        @change="saveSetting"
      />
    </div>

    <!-- 手动触发 -->
    <div class="manual-row">
      <button class="action-btn refresh-btn" :disabled="busy" @click="handleRefresh">
        🔄 立即刷新
      </button>
      <button class="action-btn writeback-btn" :disabled="busy" @click="handleWriteback">
        💾 立即写回
      </button>
    </div>

    <!-- 进度 banner -->
    <div v-if="progress && progress.status === 'running'" class="progress-banner running">
      <div class="progress-text">{{ progress.message }}</div>
      <div class="progress-track">
        <div class="progress-fill" :style="{ width: percent + '%' }"></div>
      </div>
      <div class="progress-meta">{{ progress.done }} / {{ progress.total }}</div>
    </div>
    <div
      v-else-if="progress && (progress.status === 'success' || progress.status === 'error')"
      class="progress-banner"
      :class="progress.status"
    >
      {{ progress.message }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { http } from '@/utils/request'
import { useUI } from '@/composables/useUI'

interface TagMaintainSetting {
  enableDailyRefresh: boolean
  enableWeeklyWriteback: boolean
  refreshHour: number
  writebackWeekday: number
  writebackHour: number
  lastDailyRunAt?: number
  lastWeeklyRunAt?: number
}

interface TagMaintainProgress {
  status: 'idle' | 'running' | 'success' | 'error'
  type: 'refresh' | 'writeback'
  done: number
  total: number
  updated?: number
  written?: number
  failed?: number
  noTags?: number
  message: string
  startedAt?: number
}

const weekdayLabels = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
const weekdayLabel = computed(() => weekdayLabels[setting.value.writebackWeekday] ?? '周日')

const setting = ref<TagMaintainSetting>({
  enableDailyRefresh: true,
  enableWeeklyWriteback: true,
  refreshHour: 6,
  writebackWeekday: 0,
  writebackHour: 6,
})
const progress = ref<TagMaintainProgress | null>(null)
const busy = ref(false)
const { toast } = useUI()

let pollTimer: number | null = null

// ── 标签数据（翻译/排序）下载控制（/tags API）──
const enableTagCNTranslation = ref(true)
const tagCNVersion = ref('未加载')

const enableTagSortRules = ref(true)
const tagSortVersion = ref('未加载')

// 自动更新周期（小时），后端 /tags/status 返回
const updateCycleHours = ref(24)

interface ProgressData {
  status: 'idle' | 'downloading' | 'success' | 'error'
  progress: number
  downloaded: number
  total: number
  errorMsg?: string
}

const transProgress = ref<ProgressData>({ status: 'idle', progress: 0, downloaded: 0, total: 0 })
const sortProgress = ref<ProgressData>({ status: 'idle', progress: 0, downloaded: 0, total: 0 })

// 上一次状态记录，用于防重弹窗
const prevTransStatus = ref<string>('idle')
const prevSortStatus = ref<string>('idle')

let engineTimer: number | null = null

const formatSize = (bytes: number) => {
  if (!bytes) return '0 B'
  const mb = bytes / (1024 * 1024)
  return `${mb.toFixed(2)} MB`
}

const formatTime = (ts?: number) => {
  if (!ts) return '从未执行'
  const d = new Date(ts)
  return d.toLocaleString('zh-CN', { hour12: false })
}

const percent = computed(() => {
  if (!progress.value || progress.value.total <= 0) return 0
  return Math.min(100, Math.round((progress.value.done / progress.value.total) * 100))
})

const fetchSetting = async () => {
  try {
    setting.value = await http<TagMaintainSetting>('/offline/tags/setting')
  } catch {
    toast.error('读取 Tag 维护设置失败')
  }
}

const saveSetting = async () => {
  try {
    const saved = await http<TagMaintainSetting>('/offline/tags/setting', {
      method: 'POST',
      body: JSON.stringify(setting.value),
    })
    setting.value = saved
    toast.success('Tag 维护设置已保存')
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '保存设置失败')
  }
}

const startPolling = () => {
  stopPolling()
  pollTimer = window.setInterval(async () => {
    try {
      progress.value = await http<TagMaintainProgress>('/offline/tags/progress')
      if (progress.value.status === 'success' || progress.value.status === 'error') {
        stopPolling()
        busy.value = false
        if (progress.value.status === 'success') toast.success(progress.value.message)
        else toast.error(progress.value.message)
        fetchSetting()
      }
    } catch {
      // 忽略轮询瞬时错误
    }
  }, 1000)
}

const stopPolling = () => {
  if (pollTimer) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

const handleRefresh = async () => {
  if (busy.value) return
  busy.value = true
  try {
    await http('/offline/tags/refresh', { method: 'POST' })
    toast.info('已开始刷新 Tag，请稍候…')
    startPolling()
  } catch (e) {
    busy.value = false
    toast.error(e instanceof Error ? e.message : '触发刷新失败')
  }
}

const handleWriteback = async () => {
  if (busy.value) return
  busy.value = true
  try {
    await http('/offline/tags/writeback', { method: 'POST' })
    toast.info('已开始反向写回，请稍候…')
    startPolling()
  } catch (e) {
    busy.value = false
    toast.error(e instanceof Error ? e.message : '触发写回失败')
  }
}

// ── 标签数据（翻译/排序）下载：轮询进度 / 获取状态 / 手动同步 ──
const pollTagEngineProgress = async () => {
  try {
    const data = await http<{
      transProgress: ProgressData
      sortProgress: ProgressData
    }>('/tags/progress')

    transProgress.value = data.transProgress
    sortProgress.value = data.sortProgress

    // 仅在状态从 downloading 变化时触发一次 Toast
    if (prevTransStatus.value === 'downloading') {
      if (transProgress.value.status === 'success') {
        toast.success('标签翻译数据库更新完成！')
        fetchTagEngineStatus()
      } else if (transProgress.value.status === 'error') {
        toast.error(`翻译库更新失败: ${transProgress.value.errorMsg || '网络超时'}`)
      }
    }
    prevTransStatus.value = transProgress.value.status

    if (prevSortStatus.value === 'downloading') {
      if (sortProgress.value.status === 'success') {
        toast.success('标签排序规则更新完成！')
        fetchTagEngineStatus()
      } else if (sortProgress.value.status === 'error') {
        toast.error(`排序库更新失败: ${sortProgress.value.errorMsg || '网络超时'}`)
      }
    }
    prevSortStatus.value = sortProgress.value.status

    // 只要有一个还在下载就继续轮询
    if (
      transProgress.value.status === 'downloading' ||
      sortProgress.value.status === 'downloading'
    ) {
      engineTimer = window.setTimeout(pollTagEngineProgress, 500)
    }
  } catch (err) {
    console.error('获取进度失败:', err)
  }
}

const fetchTagEngineStatus = async () => {
  try {
    const data = await http<{
      enableCN: boolean
      tagCNVersion?: string
      enableSort: boolean
      tagSortVersion?: string
      updateCycleHours?: number
    }>('/tags/status')

    enableTagCNTranslation.value = data.enableCN
    tagCNVersion.value = data.tagCNVersion || '尚未下载'
    enableTagSortRules.value = data.enableSort
    tagSortVersion.value = data.tagSortVersion || '尚未下载'
    if (typeof data.updateCycleHours === 'number' && data.updateCycleHours > 0) {
      updateCycleHours.value = data.updateCycleHours
    }
  } catch (err) {
    console.error('获取引擎状态失败:', err)
  }
}

const handleRefreshTagTranslation = async () => {
  toast.info('正在检查并同步标签中文翻译数据库...')
  prevTransStatus.value = 'downloading'
  try {
    await http<{ ok: boolean }>('/tags/sync/translation', { method: 'POST' })
    pollTagEngineProgress()
  } catch {
    toast.error('触发同步失败')
  }
}

const handleRefreshTagSort = async () => {
  toast.info('正在检查并同步标签补全排序规则...')
  prevSortStatus.value = 'downloading'
  try {
    await http<{ ok: boolean }>('/tags/sync/count', { method: 'POST' })
    pollTagEngineProgress()
  } catch {
    toast.error('触发同步失败')
  }
}

onMounted(() => {
  fetchSetting()
  fetchTagEngineStatus()
  pollTagEngineProgress() // 进页面先查一次，防止后台正好在自动更新
})

onUnmounted(() => {
  stopPolling()
  if (engineTimer) clearTimeout(engineTimer)
})
</script>

<style scoped>
.tag-maintain-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.intro-card {
  padding: 14px 16px;
  background-color: #1a1a1e;
  border-radius: 8px;
  border: 1px solid #26262a;
}

.intro-title {
  font-size: 15px;
  font-weight: 600;
  color: #ffffff;
  margin-bottom: 8px;
}

.intro-text {
  font-size: 13px;
  color: #88888c;
  line-height: 1.7;
}

.intro-text b {
  color: #a891e3;
}

.status-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 14px 16px;
  background-color: #1a1a1e;
  border-radius: 8px;
  border: 1px solid #26262a;
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.status-key {
  font-size: 13px;
  color: #88888c;
}

.status-val {
  font-size: 13px;
  font-weight: 600;
  color: #a891e3;
  font-family: monospace;
}

.setting-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background-color: #1a1a1e;
  border-radius: 8px;
  border: 1px solid #26262a;
  transition: background-color 0.2s ease;
}

.item-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 78%;
}

.item-title {
  font-size: 15px;
  font-weight: 500;
  color: #ffffff;
}

.item-subtext {
  font-size: 13px;
  color: #88888c;
  line-height: 1.4;
}

.num-input {
  width: 72px;
  padding: 8px 10px;
  background-color: #121214;
  border: 1px solid #38383e;
  border-radius: 6px;
  color: #ffffff;
  font-size: 14px;
  text-align: center;
  outline: none;
}

.num-input:focus {
  border-color: #a891e3;
}

.select-input {
  padding: 8px 10px;
  background-color: #121214;
  border: 1px solid #38383e;
  border-radius: 6px;
  color: #ffffff;
  font-size: 14px;
  outline: none;
  cursor: pointer;
}

.select-input:focus {
  border-color: #a891e3;
}

/* Switch 开关 */
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
  background-color: #38383e;
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
  background-color: #a0a0a5;
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

/* 手动触发按钮 */
.manual-row {
  display: flex;
  gap: 12px;
  padding: 4px 0;
}

.action-btn {
  flex: 1;
  padding: 12px 16px;
  border: none;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 600;
  color: #ffffff;
  cursor: pointer;
  transition:
    transform 0.15s ease,
    opacity 0.2s ease,
    background-color 0.2s ease;
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.refresh-btn {
  background-color: #ff7588;
}

.refresh-btn:not(:disabled):hover {
  background-color: #ff5f75;
  transform: translateY(-1px);
}

.writeback-btn {
  background-color: #4c9aff;
}

.writeback-btn:not(:disabled):hover {
  background-color: #3385f5;
  transform: translateY(-1px);
}

/* 进度 banner */
.progress-banner {
  padding: 14px 16px;
  border-radius: 8px;
  border: 1px solid #26262a;
  font-size: 13px;
}

.progress-banner.running {
  background-color: #12182a;
  border-color: #33507a;
  color: #9fc3ff;
}

.progress-banner.success {
  background-color: #14251c;
  border-color: #2d5a3f;
  color: #7fdca0;
}

.progress-banner.error {
  background-color: #2a1418;
  border-color: #5a2d35;
  color: #ff9aa8;
}

.progress-text {
  margin-bottom: 8px;
  word-break: break-all;
}

.progress-track {
  height: 6px;
  background-color: #2a2a2e;
  border-radius: 3px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background-color: #4c9aff;
  border-radius: 3px;
  transition: width 0.3s ease;
}

.progress-meta {
  margin-top: 6px;
  text-align: right;
  font-family: monospace;
  font-size: 12px;
  opacity: 0.7;
}

/* 标签数据（翻译/排序）下载样式 */
.column-layout {
  flex-direction: column !important;
  align-items: stretch !important;
  gap: 10px;
}

.main-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.item-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.icon-action-btn {
  background: transparent;
  border: none;
  font-size: 14px;
  cursor: pointer;
  padding: 4px;
  border-radius: 50%;
  transition: transform 0.2s ease;
  color: #a0a0a5;
}

.icon-action-btn:hover {
  transform: rotate(180deg);
  color: #ffffff;
}

.icon-action-btn.spinning {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.progress-box {
  display: flex;
  align-items: center;
  gap: 12px;
  background: #121214;
  padding: 8px 12px;
  border-radius: 6px;
}

.progress-bar-bg {
  flex: 1;
  height: 6px;
  background-color: #2a2a2e;
  border-radius: 3px;
  overflow: hidden;
}

.progress-bar-fill {
  height: 100%;
  background: linear-gradient(90deg, #ff7588, #ff9800);
  transition: width 0.2s ease;
}

.tag-progress-text {
  font-size: 11px;
  color: #aaa;
  font-family: monospace;
}
</style>
