<template>
  <div class="advanced-settings">
    <!-- 启用系统日志：控制四类操作日志（更新/维护/下载/其他）落盘（Round4 任务七） -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">启用系统日志</div>
        <div class="item-subtext">记录更新检测 / 维护查重 / 下载任务 / 扫描等操作日志到 backend/logs（可到「日志」页实时监控与查询）</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advancedSettings.systemLogsEnabled" @change="handleSystemLogsToggle" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- 前端错误上报：沿用原 enableLogs 门控 errorReporter -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">前端错误上报</div>
        <div class="item-subtext">将前端运行错误上报到服务端 logs/client.log，用于排查页面异常</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advancedSettings.enableLogs" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- 清除日志：精细管理（目标 + 范围），不再一键全清 -->
    <div class="setting-item clickable" @click="openClearModal">
      <div class="item-info">
        <div class="item-title">清除日志</div>
        <div class="item-subtext">按类别与时间范围精细清理系统日志与前端错误日志（当前共占用 {{ totalLogSize }}）</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>

    <!-- 清除日志精细管理弹窗 -->
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
import { ref, computed, onMounted } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import { advancedSettings, resetAdvancedSettings } from '@/stores/advancedSettings'

const { toast } = useUI()

// ── 日志占用总览（后端 /logs/categories 返回四类 + 前端错误日志大小）──
const totalLogSize = ref('0B')

/** 字节数 → 人类可读大小 */
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

interface LogCategoriesResp {
  categories?: Array<{ category: string; files?: Array<{ size: number }> }>
  client?: { size?: number }
}

/** 拉取各类日志大小并汇总 */
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

// ── 启用系统日志开关：与后端 /logs/settings 双向同步 ──
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

// ── 清除日志精细管理 ──
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

const handleReset = () => {
  resetAdvancedSettings()
  // 同步后端系统日志开关（恢复默认 = 启用）
  http('/logs/settings', {
    method: 'POST',
    body: JSON.stringify({ systemLogsEnabled: true }),
  }).catch(() => {})
  toast.success('已恢复默认高级设置')
}

onMounted(async () => {
  // 从后端读取持久化的系统日志开关（后端为准）
  try {
    const data = await http<{ systemLogsEnabled?: boolean }>('/logs/settings')
    if (typeof data.systemLogsEnabled === 'boolean') {
      advancedSettings.systemLogsEnabled = data.systemLogsEnabled
    }
  } catch {
    /* 后端不可达时保留本地默认值 */
  }
  fetchLogSizes()
})
</script>

<style scoped>
.advanced-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
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

.reset-row {
  display: flex;
  justify-content: center;
  padding: 8px 0;
}

.reset-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  font-size: 13px;
  padding: 8px 20px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.reset-btn:hover {
  border-color: #ff7588;
  color: var(--app-text-strong);
}

/* ── 清除日志弹窗 ── */
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
</style>
