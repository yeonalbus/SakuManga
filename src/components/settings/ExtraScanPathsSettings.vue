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
        <div class="path-row">
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
              <span v-if="isScanning(item)" class="scan-badge">{{ scanModeText(item) }}</span>
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

            <!-- 扫描模式按钮（空闲或已完成时显示） -->
            <template v-if="!isScanning(item)">
              <button
                class="btn btn-mode"
                @click="handleSingleScan(item, 'full')"
                title="全文件夹扫描：重新扫描整个目录"
              >
                全量
              </button>
              <button
                class="btn btn-mode incremental"
                @click="handleSingleScan(item, 'incremental')"
                title="增量式更新：跳过已收录的漫画，仅入库新内容"
              >
                增量
              </button>
            </template>

            <!-- 扫描中显示旋转图标 -->
            <span v-else class="btn btn-icon" title="扫描中…">
              <span class="spinning">↻</span>
            </span>

            <!-- 删除按钮 -->
            <button @click="handleRemove(item.id)" class="btn btn-icon danger" title="移除路径">
              ✕
            </button>
          </div>
        </div>

        <!-- 扫描进度区域（扫描中） -->
        <div v-if="isScanning(item)" class="scan-progress-area">
          <!-- 计数阶段 -->
          <div v-if="progressOf(item)!.phase === 'counting'" class="progress-counting">
            <span class="spinning">↻</span> 正在扫描文件夹结构…
          </div>
          <!-- 扫描阶段 -->
          <template v-else>
            <div class="progress-bar">
              <div
                class="progress-fill"
                :style="{ width: progressPercent(progressOf(item)!) + '%' }"
              ></div>
            </div>
            <div class="progress-meta">
              <span
                v-if="progressOf(item)!.currentTitle"
                class="progress-title"
                :title="progressOf(item)!.currentTitle"
              >
                正在处理: {{ progressOf(item)!.currentTitle }}
              </span>
              <span v-else class="progress-title">正在扫描…</span>
              <span class="progress-num">
                {{ progressOf(item)!.processed }}/{{ progressOf(item)!.total || '?' }}
                <template v-if="progressOf(item)!.found">
                  • 发现 {{ progressOf(item)!.found }}
                </template>
                <template v-if="progressOf(item)!.skipped">
                  • 跳过 {{ progressOf(item)!.skipped }}
                </template>
              </span>
            </div>
          </template>
        </div>

        <!-- 完成提示区域 -->
        <div
          v-else-if="progressOf(item) && progressOf(item)!.done"
          class="scan-done-area"
          :class="{ error: progressOf(item)!.error }"
        >
          <span v-if="progressOf(item)!.error">⚠️ 扫描失败: {{ progressOf(item)!.error }}</span>
          <span v-else>
            ✅ 扫描完成，共 {{ progressOf(item)!.comicCount ?? progressOf(item)!.found }} 本
            <template v-if="progressOf(item)!.skipped"
              >（跳过 {{ progressOf(item)!.skipped }}）</template
            >
          </span>
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
import { ref, onMounted, watch } from 'vue'
import {
  scanPaths,
  scanProgress,
  fetchScanPaths,
  addScanPath,
  toggleSubfolders,
  removeScanPath,
  startScanPath,
  refreshScanProgress,
  ensurePolling,
  clearScanProgress,
  getScanProgress,
  hasActiveScan,
  type ExtraScanPath,
  type ScanProgress,
  type ScanMode,
} from '@/stores/scanPathStore'
import { useUI } from '@/composables/useUI'

const { toast } = useUI()

defineEmits<{
  (e: 'back'): void
}>()

// 组件挂载时向 Go 后端拉取最新路径列表，并恢复可能仍在进行的扫描进度
// （解决切页后“看起来被截断”的问题：后端扫描在 goroutine 中持续运行，回来时重新拉取）
onMounted(async () => {
  await fetchScanPaths()
  await refreshScanProgress()
  if (hasActiveScan()) ensurePolling()
})

// 记录上一轮各路径是否已完成，用于在扫描完成瞬间弹出提示
const prevDone = ref<Record<string, boolean>>({})
watch(
  scanProgress,
  (map) => {
    for (const [id, p] of Object.entries(map)) {
      if (p.done && !prevDone.value[id]) {
        if (p.error) {
          toast.error(`扫描失败：${p.error}`)
        } else {
          toast.success(`扫描完成：${p.comicCount ?? p.found} 本漫画`)
          // 完成后展示几秒，再收起进度提示区
          setTimeout(() => {
            if (getScanProgress(id)?.done) clearScanProgress(id)
          }, 4000)
        }
      }
    }
    prevDone.value = Object.fromEntries(Object.entries(map).map(([id, p]) => [id, !!p.done]))
  },
  { deep: true, immediate: true },
)

const inputPath = ref('')

// 1. 添加路径（addScanPath 返回 Promise<boolean>，必须 await 才能正确判断）
const handleAdd = async () => {
  const path = inputPath.value.trim()
  if (!path) return

  try {
    const ok = await addScanPath(path)
    if (ok) {
      inputPath.value = ''
      toast.success('路径已添加')
    }
  } catch (err) {
    // addScanPath 现在会抛出后端真实错误（如“该路径已存在”/“未登录”/“会话已失效”），
    // 这里直接透传给用户，避免把一切失败都误报成“该路径已存在”。
    const msg = err instanceof Error ? err.message : '添加失败'
    toast.warning(msg)
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

// 4. 单路径扫描（异步启动，进度由轮询实时更新）
const handleSingleScan = async (item: ExtraScanPath, mode: ScanMode) => {
  try {
    await startScanPath(item.id, mode)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '触发扫描失败')
  }
}

// 5. 进度辅助函数
const progressOf = (item: ExtraScanPath): ScanProgress | undefined => getScanProgress(item.id)

const isScanning = (item: ExtraScanPath): boolean => {
  const p = progressOf(item)
  return !!p && !p.done
}

const scanModeText = (item: ExtraScanPath): string => {
  const p = progressOf(item)
  return p?.mode === 'incremental' ? '增量更新中' : '全量扫描中'
}

const progressPercent = (p: ScanProgress): number => {
  if (p.phase !== 'scanning' || !p.total) return 0
  return Math.min(100, Math.round((p.processed / p.total) * 100))
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
  flex-direction: column;
  gap: 12px;
}

.path-row {
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

.scan-badge {
  font-size: 11px;
  color: #ff7588;
  border: 1px solid rgba(255, 117, 136, 0.4);
  border-radius: 4px;
  padding: 1px 6px;
  white-space: nowrap;
}

.path-actions {
  display: flex;
  align-items: center;
  gap: 8px;
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

/* 扫描模式按钮 */
.btn-mode {
  background: #121214;
  border: 1px solid #38383e;
  color: #a0a0a5;
  padding: 5px 10px;
  font-size: 12px;
}

.btn-mode:hover {
  border-color: #ff7588;
  color: #ff7588;
}

.btn-mode.incremental:hover {
  border-color: #4fc3f7;
  color: #4fc3f7;
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

/* 扫描进度区域 */
.scan-progress-area {
  border: 1px solid rgba(255, 117, 136, 0.25);
  background-color: rgba(255, 117, 136, 0.05);
  border-radius: 6px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.progress-counting {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #ffb6c1;
}

.progress-bar {
  height: 6px;
  background-color: #26262a;
  border-radius: 4px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #ff7588, #ff5e74);
  border-radius: 4px;
  transition: width 0.3s ease;
}

.progress-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
  color: #a0a0a5;
}

.progress-title {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.progress-num {
  flex-shrink: 0;
  color: #ffb6c1;
  font-variant-numeric: tabular-nums;
}

/* 完成提示区域 */
.scan-done-area {
  border: 1px solid rgba(82, 196, 26, 0.3);
  background-color: rgba(82, 196, 26, 0.06);
  border-radius: 6px;
  padding: 8px 12px;
  font-size: 13px;
  color: #95de64;
}

.scan-done-area.error {
  border-color: rgba(255, 77, 79, 0.3);
  background-color: rgba(255, 77, 79, 0.06);
  color: #ff7875;
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
