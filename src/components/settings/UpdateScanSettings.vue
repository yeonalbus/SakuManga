<template>
  <div class="update-scan-settings">
    <!-- 功能说明 -->
    <div class="intro-card">
      <div class="intro-title">🔄 每周自动更新扫描说明</div>
      <div class="intro-text">
        <p>· 到点自动执行「<b>更新检测 + 老化判定</b>」完整扫描，逐本联网核对 E 站画廊是否有新版。</p>
        <p>· <b>Aged Status（老化）</b>：发布超过 365 天且确认无新版的漫画将被标记为「已老化」，
          之后不再参与扫描，避免重复联网。</p>
        <p>· 开启「自动下载新版」后（下载设置中 <b>autoUpdateGallery</b>），检测到新版会自动入队下载。</p>
      </div>
    </div>

    <!-- 最近执行状态 -->
    <div class="status-card">
      <div class="status-row">
        <span class="status-key">上次自动扫描</span>
        <span class="status-val">{{ formatTime(setting.lastWeeklyScanAt) }}</span>
      </div>
      <div class="status-row">
        <span class="status-key">下次预计执行</span>
        <span class="status-val">{{ nextRunLabel }}</span>
      </div>
    </div>

    <!-- 开关 -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启每周自动更新扫描</div>
        <div class="item-subtext">按下方设定的扫描日与时刻自动联网检测</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="setting.enableWeeklyScan" @change="saveSetting" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- 扫描日 -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">扫描日</div>
        <div class="item-subtext">每周的哪一天执行自动扫描</div>
      </div>
      <select class="select-input" v-model.number="setting.scanWeekday" @change="saveSetting">
        <option v-for="(label, i) in weekdayLabels" :key="i" :value="i">{{ label }}</option>
      </select>
    </div>

    <!-- 扫描时刻 -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">扫描时刻</div>
        <div class="item-subtext">系统本地小时（0-23），默认 6 点</div>
      </div>
      <input
        class="num-input"
        type="number"
        min="0"
        max="23"
        v-model.number="setting.scanHour"
        @change="saveSetting"
      />
    </div>

    <!-- 手动触发 -->
    <div class="manual-row">
      <button class="action-btn refresh-btn" :disabled="busy" @click="handleManualScan">
        🔄 立即扫描一次
      </button>
    </div>

    <!-- 进度 banner -->
    <div v-if="progress && progress.status === 'running'" class="progress-banner running">
      <div class="progress-text">
        {{ progress.phase || '扫描中' }}<template v-if="progress.currentTitle">「{{ progress.currentTitle }}」</template>
      </div>
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
      <template v-if="progress.status === 'success'">✅ 扫描完成：检测更新 {{ progress.total }} 本</template>
      <template v-else>❌ 扫描失败：{{ progress.error || progress.message || '未知错误' }}</template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { http } from '@/utils/request'
import { useUI } from '@/composables/useUI'

interface UpdateScanSetting {
  enableWeeklyScan: boolean
  scanWeekday: number
  scanHour: number
  lastWeeklyScanAt?: number
}

// 离线任务进度快照（/offline/updates/check/progress 返回结构）
interface OfflineTaskProgress {
  type: 'maintain' | 'update'
  status: 'idle' | 'running' | 'success' | 'error'
  phase?: string
  total: number
  done: number
  currentTitle?: string
  message?: string
  startedAt?: number
  finishedAt?: number
  error?: string
}

const weekdayLabels = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

const setting = ref<UpdateScanSetting>({
  enableWeeklyScan: false,
  scanWeekday: 0,
  scanHour: 6,
})
const progress = ref<OfflineTaskProgress | null>(null)
const busy = ref(false)
const { toast } = useUI()

let pollTimer: number | null = null

const formatTime = (ts?: number) => {
  if (!ts) return '从未执行'
  const d = new Date(ts)
  return d.toLocaleString('zh-CN', { hour12: false })
}

const percent = computed(() => {
  if (!progress.value || progress.value.total <= 0) return 0
  return Math.min(100, Math.round((progress.value.done / progress.value.total) * 100))
})

// 下次预计执行时间：系统本地时区下一个「扫描日 + 扫描时刻」
const nextRunLabel = computed(() => {
  if (!setting.value.enableWeeklyScan) return '未启用'
  const now = new Date()
  const hour = setting.value.scanHour || 0
  const weekday = setting.value.scanWeekday || 0
  // 从今天开始向后找第一个满足 weekday 且时刻在未来的时间点
  for (let offset = 0; offset <= 7; offset++) {
    const candidate = new Date(now)
    candidate.setDate(now.getDate() + offset)
    candidate.setHours(hour, 0, 0, 0)
    if (candidate.getDay() !== weekday) continue
    if (candidate.getTime() <= now.getTime()) continue
    return candidate.toLocaleString('zh-CN', { hour12: false })
  }
  return '无法计算'
})

const fetchSetting = async () => {
  try {
    setting.value = await http<UpdateScanSetting>('/offline/update-scan/setting')
  } catch {
    toast.error('读取更新扫描设置失败')
  }
}

const saveSetting = async () => {
  try {
    const saved = await http<UpdateScanSetting>('/offline/update-scan/setting', {
      method: 'POST',
      body: JSON.stringify(setting.value),
    })
    setting.value = saved
    toast.success('更新扫描设置已保存')
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '保存设置失败')
  }
}

const startPolling = () => {
  stopPolling()
  pollTimer = window.setInterval(async () => {
    try {
      progress.value = await http<OfflineTaskProgress>('/offline/updates/check/progress')
      if (progress.value.status === 'success' || progress.value.status === 'error') {
        stopPolling()
        busy.value = false
        if (progress.value.status === 'success') toast.success('扫描完成')
        else toast.error(progress.value.error || progress.value.message || '扫描失败')
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

// 手动触发：复用离线更新检测接口（现同时执行更新检测 + 老化判定）
const handleManualScan = async () => {
  if (busy.value) return
  busy.value = true
  try {
    await http('/offline/updates/check', { method: 'POST' })
    toast.info('已开始扫描，请稍候…')
    startPolling()
  } catch (e) {
    busy.value = false
    toast.error(e instanceof Error ? e.message : '触发扫描失败')
  }
}

onMounted(() => {
  fetchSetting()
  // 进页面先查一次进度，防止后台正好在自动/手动扫描
  http<OfflineTaskProgress>('/offline/updates/check/progress')
    .then((p) => {
      progress.value = p
      if (p.status === 'running' && p.type === 'update') startPolling()
    })
    .catch(() => {})
})

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped>
.update-scan-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.intro-card {
  padding: 14px 16px;
  background-color: var(--app-surface-2);
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
}

.intro-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--app-text-strong);
  margin-bottom: 8px;
}

.intro-text {
  font-size: 13px;
  color: var(--app-text-3);
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
  background-color: var(--app-surface-2);
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.status-key {
  font-size: 13px;
  color: var(--app-text-3);
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
  background-color: var(--app-surface-2);
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
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
  color: var(--app-text-strong);
}

.item-subtext {
  font-size: 13px;
  color: var(--app-text-3);
  line-height: 1.4;
}

.num-input {
  width: 72px;
  padding: 8px 10px;
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  color: var(--app-text-strong);
  font-size: 14px;
  text-align: center;
  outline: none;
}

.num-input:focus {
  border-color: #a891e3;
}

.select-input {
  padding: 8px 10px;
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  color: var(--app-text-strong);
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

/* 进度 banner */
.progress-banner {
  padding: 14px 16px;
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
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
  background-color: var(--app-border-2);
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
</style>
