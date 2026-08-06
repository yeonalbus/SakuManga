<template>
  <div class="advanced-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启日志</div>
        <div class="item-subtext">需要重启</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advancedSettings.enableLogs" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">记录全部日志</div>
        <div class="item-subtext">需要重启</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advancedSettings.recordAllLogs" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item clickable" @click="handleViewLogs">
      <div class="item-info">
        <div class="item-title">查看日志</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item clickable" @click="handleClearLogs">
      <div class="item-info">
        <div class="item-title">清除日志</div>
        <div class="item-subtext">长按清除</div>
      </div>
      <span class="size-text">{{ logSize }}</span>
    </div>

    <div class="setting-item clickable" @click="handleClearImageCache">
      <div class="item-info">
        <div class="item-title">清除图片缓存</div>
        <div class="item-subtext">长按清除</div>
      </div>
      <span class="size-text">{{ imageCacheSize }}</span>
    </div>

    <div class="setting-item clickable" @click="handleClearPageCache">
      <div class="item-info">
        <div class="item-title">清除页面缓存</div>
        <div class="item-subtext">长按清除</div>
      </div>
    </div>

    <div class="setting-item clickable" @click="handleImageSuperResolution">
      <div class="item-info">
        <div class="item-title">图片超分辨率</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">启动应用时检查更新</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advancedSettings.checkUpdatesOnStartup" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">检测剪切板中的画廊链接</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advancedSettings.detectClipboardLinks" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">无图模式</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advancedSettings.noImageMode" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item clickable" @click="handleImportData">
      <div class="item-info">
        <div class="item-title">导入数据</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item clickable" @click="handleExportData">
      <div class="item-info">
        <div class="item-title">导出数据</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useUI } from '@/composables/useUI'
import { advancedSettings, resetAdvancedSettings } from '@/stores/advancedSettings'

const { toast, modal } = useUI()

// 运行时显示状态（非持久化设置）
const logSize = ref('6.25MB')
const imageCacheSize = ref('535.14KB')

// 交互调取
const handleViewLogs = () => {
  toast.info('打开实时日志查看窗口')
}

const handleClearLogs = async () => {
  const confirm = await modal.confirm('确定要清除所有系统日志文件吗？')
  if (confirm) {
    logSize.value = '0B'
    toast.success('系统日志已完全清除！')
  }
}

const handleClearImageCache = async () => {
  const confirm = await modal.confirm('确定要清除本地图片缓存吗？')
  if (confirm) {
    imageCacheSize.value = '0B'
    toast.success('图片缓存清理成功！')
  }
}

const handleClearPageCache = async () => {
  const confirm = await modal.confirm('确定要清除页面 HTML 与数据缓存吗？')
  if (confirm) {
    toast.success('页面缓存清理成功！')
  }
}

const handleImageSuperResolution = () => {
  toast.info('打开图片超分辨率 (Super Resolution) 设置')
}

const handleImportData = () => {
  toast.info('请选择要导入的备份文件 (.json / .zip)')
}

const handleExportData = () => {
  toast.success('全量配置与离线数据已成功导出！')
}

const handleReset = () => {
  resetAdvancedSettings()
  toast.success('已恢复默认高级设置')
}
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
