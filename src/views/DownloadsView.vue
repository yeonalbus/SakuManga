<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'

// ── 后端 DownloadTask 契约 ──
export interface DownloadTask {
  id: string
  gid: string
  token: string
  title: string
  coverUrl?: string
  mode: 'gallery' | 'archive'
  archiveType?: 'original' | 'resample'
  status: 'queued' | 'downloading' | 'paused' | 'completed' | 'error' | 'error_lock' | 'cancelled'
  priority: number
  group?: string
  totalFiles: number
  doneFiles: number
  totalBytes: number
  doneBytes: number
  speed: number
  archivePath?: string
  extractPath?: string
  error?: string
  updateForComicId?: string
  createdAt: string
  updatedAt: string
}

interface ListResponse {
  tasks: DownloadTask[]
  total: number
  page: number
  size: number
}

const { toast, modal } = useUI()

const tasks = ref<DownloadTask[]>([])
const total = ref(0)
const page = ref(1)
const size = 20
const modeFilter = ref<'all' | 'gallery' | 'archive'>('all')
const statusFilter = ref('')
const isLoading = ref(false)
let timer: number | null = null

const statusMeta: Record<string, { label: string; cls: string }> = {
  queued: { label: '排队中', cls: 'queued' },
  downloading: { label: '下载中', cls: 'downloading' },
  paused: { label: '已暂停', cls: 'paused' },
  completed: { label: '✓ 完成', cls: 'completed' },
  error: { label: '出错', cls: 'error' },
  error_lock: { label: '🔒 GP/配额锁定', cls: 'error_lock' },
  cancelled: { label: '已取消', cls: 'cancelled' },
}

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / size)))

const fetchTasks = async () => {
  isLoading.value = true
  try {
    const data = await http<ListResponse>('/downloads', {
      params: {
        page: page.value,
        size,
        mode: modeFilter.value === 'all' ? '' : modeFilter.value,
        status: statusFilter.value,
      },
    })
    tasks.value = data.tasks || []
    total.value = data.total || 0
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '获取下载任务失败')
  } finally {
    isLoading.value = false
  }
}

// 进度百分比（优先按字节，其次按文件数）
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

const progressText = (t: DownloadTask): string => {
  if (t.totalBytes > 0) {
    return `${formatBytes(t.doneBytes)} / ${formatBytes(t.totalBytes)}`
  }
  if (t.totalFiles > 0) {
    return `${t.doneFiles} / ${t.totalFiles} 文件`
  }
  return ''
}

// ── 任务操作 ──
const callAction = async (id: string, action: string, successMsg: string) => {
  try {
    await http(`/downloads/${id}/${action}`, { method: 'POST' })
    toast.success(successMsg)
    fetchTasks()
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '操作失败')
  }
}

const handlePause = (t: DownloadTask) => callAction(t.id, 'pause', '已暂停任务')
const handleResume = (t: DownloadTask) => callAction(t.id, 'resume', '已恢复任务')
const handleCancel = async (t: DownloadTask) => {
  const ok = await modal.confirm(`确定取消下载《${t.title}》吗？`)
  if (ok) {
    await callAction(t.id, 'cancel', '已取消任务')
  }
}
const handleRetry = (t: DownloadTask) => callAction(t.id, 'retry', '已重试任务')
const handleUnlock = (t: DownloadTask) => callAction(t.id, 'unlock', '已解锁任务')

// 分页
const changePage = (delta: number) => {
  const next = page.value + delta
  if (next < 1 || next > totalPages.value) return
  page.value = next
  fetchTasks()
}

// 过滤器
const setModeFilter = (m: 'all' | 'gallery' | 'archive') => {
  modeFilter.value = m
  page.value = 1
  fetchTasks()
}

const setStatusFilter = (s: string) => {
  statusFilter.value = s
  page.value = 1
  fetchTasks()
}

onMounted(() => {
  fetchTasks()
  timer = window.setInterval(fetchTasks, 2000) // 2s 轮询
})

onUnmounted(() => {
  if (timer !== null) {
    clearInterval(timer)
    timer = null
  }
})
</script>

<template>
  <div class="downloads-page">
    <div class="page-header">
      <h2 class="page-title">⬇️ 下载任务列表</h2>
      <span class="total-count">共 {{ total }} 个任务</span>
    </div>

    <!-- 模式筛选 -->
    <div class="filter-row">
      <button
        class="filter-btn"
        :class="{ active: modeFilter === 'all' }"
        @click="setModeFilter('all')"
      >
        全部
      </button>
      <button
        class="filter-btn"
        :class="{ active: modeFilter === 'gallery' }"
        @click="setModeFilter('gallery')"
      >
        🖼️ 画廊
      </button>
      <button
        class="filter-btn"
        :class="{ active: modeFilter === 'archive' }"
        @click="setModeFilter('archive')"
      >
        🗜️ 归档
      </button>

      <span class="filter-divider"></span>

      <button
        v-for="(meta, key) in statusMeta"
        :key="key"
        class="filter-btn status"
        :class="{ active: statusFilter === key }"
        @click="setStatusFilter(statusFilter === key ? '' : key)"
      >
        {{ meta.label }}
      </button>
    </div>

    <div v-if="isLoading && !tasks.length" class="loading-box">加载中...</div>
    <div v-else-if="!tasks.length" class="loading-box">暂无下载任务</div>

    <div v-else class="task-list">
      <div v-for="task in tasks" :key="task.id" class="download-card">
        <!-- 封面图 -->
        <div class="cover-box">
          <img
            v-if="task.coverUrl"
            :src="task.coverUrl"
            :alt="task.title"
            referrerpolicy="no-referrer"
          />
          <span v-else>{{ task.mode === 'archive' ? '🗜️' : '🖼️' }}</span>
        </div>

        <!-- 详细数据栏 -->
        <div class="task-info">
          <div class="task-header">
            <span class="task-title" :title="task.title">{{ task.title }}</span>
            <span class="task-speed">
              {{ task.status === 'downloading' ? formatSpeed(task.speed) : '' }}
            </span>
          </div>

          <div class="task-meta">
            <span class="meta-tag" :class="task.mode">
              {{
                task.mode === 'archive'
                  ? `归档${task.archiveType === 'original' ? '·原图' : '·压缩'}`
                  : '画廊'
              }}
            </span>
            <span v-if="task.group" class="meta-tag group">分组 {{ task.group }}</span>
            <span v-if="task.updateForComicId" class="meta-tag update">🔄 离线更新</span>
            <span v-if="task.error" class="meta-tag error" :title="task.error">{{
              task.error
            }}</span>
          </div>

          <!-- 进度条 -->
          <div class="progress-bar-bg">
            <div
              class="progress-bar-fill"
              :class="task.status"
              :style="{ width: percentOf(task) + '%' }"
            ></div>
          </div>

          <!-- 底部状态指示器与操作 -->
          <div class="task-footer">
            <span class="progress-text">
              {{ percentOf(task) }}%
              <template v-if="progressText(task)"> · {{ progressText(task) }}</template>
            </span>

            <div class="footer-actions">
              <span class="status-label" :class="statusMeta[task.status]?.cls">
                {{ statusMeta[task.status]?.label || task.status }}
              </span>

              <!-- 报错/锁阻断处理 -->
              <button
                v-if="task.status === 'error_lock'"
                class="action-btn lock"
                @click="handleUnlock(task)"
              >
                解锁
              </button>
              <button
                v-else-if="task.status === 'error'"
                class="action-btn retry"
                @click="handleRetry(task)"
              >
                重试
              </button>
              <button
                v-if="task.status === 'queued' || task.status === 'downloading'"
                class="action-btn pause"
                @click="handlePause(task)"
              >
                暂停
              </button>
              <button
                v-else-if="task.status === 'paused'"
                class="action-btn resume"
                @click="handleResume(task)"
              >
                恢复
              </button>
              <button
                v-if="
                  ['queued', 'downloading', 'paused', 'error', 'error_lock'].includes(task.status)
                "
                class="action-btn cancel"
                @click="handleCancel(task)"
              >
                取消
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 分页 -->
    <div v-if="totalPages > 1" class="pagination">
      <button class="page-btn" :disabled="page <= 1" @click="changePage(-1)">‹ 上一页</button>
      <span class="page-info">{{ page }} / {{ totalPages }}</span>
      <button class="page-btn" :disabled="page >= totalPages" @click="changePage(1)">
        下一页 ›
      </button>
    </div>
  </div>
</template>

<style scoped>
.downloads-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 20px;
  max-width: 1000px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.page-title {
  font-size: 1.3rem;
  color: var(--app-text-strong);
  margin: 0;
}

.total-count {
  font-size: 0.85rem;
  color: var(--app-text-3);
}

.filter-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.filter-btn {
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  color: var(--app-text-2);
  padding: 6px 14px;
  border-radius: 16px;
  font-size: 0.82rem;
  cursor: pointer;
  transition: all 0.2s;
}

.filter-btn:hover {
  border-color: #007acc;
  color: var(--app-text-strong);
}

.filter-btn.active {
  background: rgba(0, 122, 204, 0.15);
  border-color: #007acc;
  color: #007acc;
  font-weight: 600;
}

.filter-btn.status.active {
  border-color: #ff7588;
  color: #ff7588;
  background: rgba(255, 117, 136, 0.1);
}

.filter-divider {
  width: 1px;
  height: 18px;
  background: var(--app-border-2);
  margin: 0 4px;
}

.loading-box {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 160px;
  color: var(--app-text-3);
  font-size: 0.9rem;
}

.task-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.download-card {
  display: flex;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 12px;
  gap: 16px;
  align-items: center;
}

.cover-box {
  width: 60px;
  height: 80px;
  background-color: var(--app-border-2);
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  font-size: 1.6rem;
  overflow: hidden;
}

.cover-box img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.task-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.task-header {
  display: flex;
  justify-content: space-between;
  font-size: 0.9rem;
  color: var(--app-text-2);
  gap: 12px;
}

.task-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-speed {
  color: #007acc;
  font-family: monospace;
  flex-shrink: 0;
}

.task-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.meta-tag {
  font-size: 0.72rem;
  padding: 1px 8px;
  border-radius: 10px;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
}

.meta-tag.archive {
  border-color: #b8860b;
  color: #ffc107;
}

.meta-tag.gallery {
  border-color: #007acc;
  color: #5cb8ff;
}

.meta-tag.group {
  border-color: var(--app-border-3);
}

.meta-tag.update {
  border-color: #ff7588;
  color: #ff7588;
}

.meta-tag.error {
  border-color: #e63946;
  color: #ff6b6b;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.progress-bar-bg {
  height: 6px;
  background-color: var(--app-border-2);
  border-radius: 3px;
  overflow: hidden;
}

.progress-bar-fill {
  height: 100%;
  background-color: #007acc;
  transition: width 0.3s ease;
}
.progress-bar-fill.error_lock {
  background-color: #e6a23c;
}
.progress-bar-fill.completed {
  background-color: #67c23a;
}
.progress-bar-fill.paused {
  background-color: var(--app-text-3);
}
.progress-bar-fill.error {
  background-color: #e63946;
}

.task-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.8rem;
  color: var(--app-text-3);
  gap: 12px;
}

.progress-text {
  font-family: monospace;
  white-space: nowrap;
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.status-label {
  font-size: 0.78rem;
  padding: 2px 8px;
  border-radius: 4px;
}
.status-label.downloading {
  background: rgba(0, 122, 204, 0.15);
  color: #5cb8ff;
}
.status-label.completed {
  background: rgba(103, 194, 58, 0.15);
  color: #67c23a;
}
.status-label.error_lock {
  background: rgba(230, 162, 60, 0.2);
  color: #e6a23c;
}
.status-label.error {
  background: rgba(230, 57, 70, 0.2);
  color: #ff6b6b;
}
.status-label.paused {
  background: rgba(136, 136, 140, 0.2);
  color: var(--app-text-2);
}
.status-label.queued {
  background: rgba(136, 136, 140, 0.15);
  color: var(--app-text-3);
}
.status-label.cancelled {
  background: rgba(136, 136, 140, 0.15);
  color: var(--app-text-muted);
}

.action-btn {
  border: 1px solid;
  background: transparent;
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 0.78rem;
  cursor: pointer;
  transition: all 0.2s;
}

.action-btn.pause,
.action-btn.retry {
  border-color: #007acc;
  color: #5cb8ff;
}
.action-btn.pause:hover,
.action-btn.retry:hover {
  background: rgba(0, 122, 204, 0.15);
}

.action-btn.resume {
  border-color: #67c23a;
  color: #67c23a;
}
.action-btn.resume:hover {
  background: rgba(103, 194, 58, 0.15);
}

.action-btn.lock {
  border-color: #e6a23c;
  color: #e6a23c;
}
.action-btn.lock:hover {
  background: rgba(230, 162, 60, 0.15);
}

.action-btn.cancel {
  border-color: #e63946;
  color: #ff6b6b;
}
.action-btn.cancel:hover {
  background: rgba(230, 57, 70, 0.15);
}

.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 14px;
}

.page-btn {
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  color: var(--app-text-2);
  padding: 6px 16px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.85rem;
}
.page-btn:hover:not(:disabled) {
  border-color: #007acc;
  color: var(--app-text-strong);
}
.page-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.page-info {
  font-size: 0.85rem;
  color: var(--app-text-3);
}
</style>
