<template>
  <div class="advanced-settings">
    <!-- 开启日志：门控前端错误上报（接线 errorReporter） -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启日志</div>
        <div class="item-subtext">记录前端运行错误并上报到服务端日志</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advancedSettings.enableLogs" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- 清除日志：展示真实占用，调用后端清日志接口 -->
    <div class="setting-item clickable" @click="handleClearLogs">
      <div class="item-info">
        <div class="item-title">清除日志</div>
        <div class="item-subtext">清除服务端收集的前端错误日志</div>
      </div>
      <span class="size-text">{{ logSize }}</span>
    </div>

    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import { advancedSettings, resetAdvancedSettings } from '@/stores/advancedSettings'

const { toast, modal } = useUI()

// 前端错误日志文件大小（后端 logs/client.log 的真实占用）
const logSize = ref('0B')

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

/** 拉取后端日志文件大小 */
const fetchLogSize = async () => {
  try {
    const data = await http<{ size: number }>('/client/log/size')
    logSize.value = formatSize(typeof data.size === 'number' ? data.size : 0)
  } catch (err) {
    console.error('获取日志大小失败:', err)
    logSize.value = '0B'
  }
}

/** 清除日志：调用后端 DELETE /client/log */
const handleClearLogs = async () => {
  const confirm = await modal.confirm('确定要清除所有系统日志文件吗？')
  if (!confirm) return
  try {
    await http('/client/log', { method: 'DELETE' })
    toast.success('系统日志已完全清除！')
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '清除日志失败')
  }
  fetchLogSize()
}

const handleReset = () => {
  resetAdvancedSettings()
  toast.success('已恢复默认高级设置')
}

onMounted(() => {
  fetchLogSize()
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

.size-text {
  font-size: 13px;
  font-weight: 600;
  color: #a891e3;
  font-family: monospace;
}

.arrow-icon {
  font-size: 20px;
  color: var(--app-text-muted);
  margin-left: 8px;
}

/* Switch 开关 */
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
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
</style>
