<template>
  <div class="extra-scan-settings">
    <!-- 顶部 Header -->
    <div class="header">
      <button class="back-btn" @click="$emit('back')" title="返回下载设置">‹</button>

      <div class="header-text">
        <div class="header-title">额外的画廊扫描路径</div>
        <div class="header-subtext">扫描额外的漫画路径，包括子文件夹中的漫画/压缩包</div>
      </div>
    </div>

    <!-- 添加新路径卡片 -->
    <div class="card add-card">
      <div class="input-label">添加新路径</div>
      <div class="input-group">
        <input
          v-model="inputPath"
          type="text"
          placeholder="输入或粘贴绝对路径，如 Z:\Comics\"
          class="setting-input"
          @keyup.enter="handleAdd"
        />
        <button @click="handleAdd" :disabled="!inputPath.trim()" class="btn btn-primary">
          添加
        </button>
      </div>
    </div>

    <!-- 动态路径列表 -->
    <div class="path-list">
      <div v-for="item in scanPaths" :key="item.id" class="card path-item">
        <div class="path-info">
          <!-- 路径文字与图标 -->
          <div class="path-title">
            <span class="folder-icon">📁</span>
            <span class="path-text" :title="item.path">{{ item.path }}</span>
          </div>

          <!-- 扫描状态与元信息 -->
          <div class="path-meta">
            <span v-if="item.lastScanned">上次扫描: {{ formatTime(item.lastScanned) }}</span>
            <span v-else class="status-unscanned">未扫描</span>
            <span v-if="item.comicCount !== undefined" class="count-text">
              • 发现 {{ item.comicCount }} 本漫画
            </span>
          </div>
        </div>

        <div class="path-actions">
          <!-- 包含子文件夹 Switch 开关 -->
          <label class="toggle-container" title="包含子文件夹">
            <span class="toggle-label">包含子文件夹</span>
            <span class="toggle-switch">
              <input
                type="checkbox"
                :checked="item.includeSubfolders"
                @change="
                  handleToggleSubfolders(item.id, ($event.target as HTMLInputElement).checked)
                "
              />
              <span class="slider"></span>
            </span>
          </label>

          <!-- 扫描按钮 -->
          <button
            @click="handleSingleScan(item)"
            :disabled="activeScanningId === item.id"
            class="btn btn-icon"
            title="扫描此路径"
          >
            <span :class="{ spinning: activeScanningId === item.id }">↻</span>
          </button>

          <!-- 删除按钮 -->
          <button @click="handleRemove(item.id)" class="btn btn-icon danger" title="移除路径">
            ✕
          </button>
        </div>
      </div>

      <!-- 无路径时的空状态卡片 -->
      <div v-if="scanPaths.length === 0" class="empty-state">
        暂无额外的扫描路径，请在上方输入框添加
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue' // 引入 onMounted
import {
  scanPaths,
  fetchScanPaths,
  addScanPath,
  toggleSubfolders,
  removeScanPath,
  updateScanPathStats,
  type ExtraScanPath,
} from '@/stores/scanPathStore'

defineEmits<{
  (e: 'back'): void
}>()

// 组件挂载时向 Go 后端拉取最新路径列表
onMounted(() => {
  fetchScanPaths()
})

const inputPath = ref('')
const activeScanningId = ref<string | null>(null)

// 1. 添加路径（addScanPath 返回 Promise<boolean>，必须 await 才能正确判断）
const handleAdd = async () => {
  const path = inputPath.value.trim()
  if (!path) return

  const ok = await addScanPath(path)
  if (ok) {
    inputPath.value = ''
  } else {
    alert('该路径已存在！')
  }
}

// 2. 切换子文件夹开关
const handleToggleSubfolders = (id: string, value: boolean) => {
  toggleSubfolders(id, value)
}

// 3. 移除路径
const handleRemove = (id: string) => {
  removeScanPath(id)
}

// 4. 单路径扫描
const handleSingleScan = async (item: ExtraScanPath) => {
  activeScanningId.value = item.id
  try {
    await updateScanPathStats(item.id)
  } finally {
    activeScanningId.value = null
  }
}

const formatTime = (ts: number) => {
  const d = new Date(ts)
  return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
}
</script>

<style scoped>
.extra-scan-settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 顶部 Header */
.header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid #26262a;
}

.back-btn {
  background: #1a1a1e;
  border: 1px solid #26262a;
  color: #a0a0a5;
  font-size: 24px;
  line-height: 1;
  width: 36px;
  height: 36px;
  border-radius: 8px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.back-btn:hover {
  background: #222226;
  color: #ffffff;
}

.header-title {
  font-size: 16px;
  font-weight: 600;
  color: #ffffff;
}

.header-subtext {
  font-size: 12px;
  color: #88888c;
  margin-top: 2px;
}

/* 卡片容器 */
.card {
  background-color: #1a1a1e;
  border: 1px solid #26262a;
  border-radius: 8px;
  padding: 14px 16px;
}

/* 添加路径卡片 */
.add-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.input-label {
  font-size: 13px;
  color: #a0a0a5;
}

.input-group {
  display: flex;
  gap: 10px;
}

.setting-input {
  flex: 1;
  background-color: #121214;
  border: 1px solid #38383e;
  border-radius: 6px;
  padding: 8px 12px;
  font-size: 13px;
  color: #ffffff;
  outline: none;
  transition: border-color 0.2s;
}

.setting-input:focus {
  border-color: #ff7588;
}

/* 路径列表卡片 */
.path-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.path-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.path-info {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.path-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.folder-icon {
  font-size: 14px;
  opacity: 0.8;
}

.path-text {
  font-size: 14px;
  font-weight: 500;
  font-family: monospace;
  color: #ffffff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.path-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: #88888c;
}

.status-unscanned {
  color: #e6a23c;
}

.count-text {
  color: #88888c;
}

.path-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

/* Toggle 开关 */
.toggle-container {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  user-select: none;
}

.toggle-label {
  font-size: 12px;
  color: #a0a0a5;
}

.toggle-switch {
  position: relative;
  display: inline-block;
  width: 38px;
  height: 20px;
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
  border-radius: 20px;
}

.slider:before {
  position: absolute;
  content: '';
  height: 14px;
  width: 14px;
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
  transform: translateX(18px);
  background-color: #ffffff;
}

/* 按钮基础及变体样式 */
.btn {
  border: none;
  border-radius: 6px;
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition:
    background-color 0.2s,
    opacity 0.2s;
}

.btn-primary {
  background-color: #ff7588;
  color: #ffffff;
}

.btn-primary:hover {
  background-color: #ff5e74;
}

.btn-primary:disabled {
  background-color: #26262a;
  color: #66666c;
  cursor: not-allowed;
}

.btn-icon {
  background: transparent;
  color: #a0a0a5;
  padding: 6px 8px;
  font-size: 15px;
}

.btn-icon:hover {
  background-color: #26262a;
  color: #ffffff;
}

.btn-icon.danger:hover {
  background-color: rgba(255, 77, 79, 0.15);
  color: #ff4d4f;
}

/* 旋转动画 */
.spinning {
  display: inline-block;
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

/* 空状态 */
.empty-state {
  text-align: center;
  padding: 32px 16px;
  border: 1px dashed #26262a;
  border-radius: 8px;
  color: #66666c;
  font-size: 13px;
}
</style>
