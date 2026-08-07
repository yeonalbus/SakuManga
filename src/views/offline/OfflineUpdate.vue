<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'

const router = useRouter()
const { toast } = useUI()

// 后端 GET /offline/updates 返回的离线漫画 DTO（含更新标记字段）
interface OfflineComicDTO {
  id: string
  title: string
  coverUrl: string
  category?: string
  pageCount?: number
  updatedAt: string
  localPath?: string
  fileSize?: number
  needsUpdate?: boolean
  gid?: string
  newGID?: string
  newToken?: string
  updateNote?: string
  sourceMode?: string
}

interface UpdateListResponse {
  items: OfflineComicDTO[]
  total: number
}

interface CheckResult {
  checked: number
  needsUpdate: OfflineComicDTO[]
  parentFound: number
}

const updates = ref<OfflineComicDTO[]>([])
const total = ref(0)
const isLoading = ref(false)
const isChecking = ref(false)
const downloadingId = ref('')
// 每个漫画的下载方案覆盖：''(按设置) | archive | gallery
const modeFor = ref<Record<string, string>>({})

// ── 任务进度（问题3：异步任务 + 进度轮询，让用户看到“现在进度在哪”）──
interface OfflineTaskState {
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

const taskState = ref<OfflineTaskState | null>(null)
const progressPercent = computed(() => {
  const s = taskState.value
  if (!s || s.total <= 0) return 0
  return Math.min(100, Math.round((s.done / s.total) * 100))
})
const phaseText = computed(() => taskState.value?.phase || '')
const currentTitle = computed(() => taskState.value?.currentTitle || '')
let pollTimer: ReturnType<typeof setInterval> | null = null

const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// 轮询更新检测进度（1s 一次；结束后停止并读取结果、刷新列表）
const pollProgress = () => {
  stopPolling()
  pollTimer = setInterval(async () => {
    try {
      const s = await http<OfflineTaskState>('/offline/updates/check/progress')
      taskState.value = s
      if (s.status === 'success' || s.status === 'error') {
        stopPolling()
        isChecking.value = false
        if (s.status === 'error') {
          toast.error(s.error || '更新检测失败（可能未绑定 E 站账户或网络异常）')
          return
        }
        // 任务完成：读取结果并刷新更新列表
        try {
          const result = await http<CheckResult>('/offline/updates/check/result')
          const found = result?.needsUpdate?.length || 0
          toast.success(
            `检测完成：核对 ${result?.checked ?? 0} 个画廊，发现 ${found} 个需要更新` +
              (result?.parentFound ? `（父画廊关系发现 ${result.parentFound} 个）` : ''),
          )
        } catch {
          // 结果读取失败不阻断刷新
        }
        await fetchUpdates()
      }
    } catch {
      // 网络抖动忽略，继续轮询
    }
  }, 1000)
}

const fetchUpdates = async () => {
  isLoading.value = true
  try {
    const data = await http<UpdateListResponse>('/offline/updates')
    updates.value = data.items || []
    total.value = data.total || 0
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '获取更新列表失败')
  } finally {
    isLoading.value = false
  }
}

// 联网核对全部离线画廊（异步任务：接口立即返回，随后轮询进度）
const runCheck = async () => {
  if (isChecking.value) return
  isChecking.value = true
  taskState.value = null
  try {
    await http<{ started: boolean }>('/offline/updates/check', {
      method: 'POST',
    })
    pollProgress()
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '启动更新检测失败（可能未绑定 E 站账户或网络异常）')
    isChecking.value = false
  }
}

const startDownload = async (comic: OfflineComicDTO) => {
  if (downloadingId.value) return
  downloadingId.value = comic.id
  try {
    await http<{ task: unknown }>('/offline/updates/download', {
      method: 'POST',
      body: JSON.stringify({
        comicId: comic.id,
        mode: modeFor.value[comic.id] || undefined,
      }),
    })
    toast.success(`《${comic.title}》新版已加入下载队列 📥`)
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '加入下载队列失败')
  } finally {
    downloadingId.value = ''
  }
}

const goDownloads = () => {
  router.push('/downloads')
}

// Round4 任务一：点击卡片进入双列对比视图（左=本地原版，右=线上最新版）
const openCompare = (comicId: string) => {
  router.push({ path: '/offline/compare', query: { type: 'update', id: comicId } })
}

const formatBytes = (bytes?: number) => {
  if (!bytes || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${units[i]}`
}

const formatDate = (iso?: string) => {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString('zh-CN', { hour12: false })
}

const coverFailed = ref<Record<string, boolean>>({})
const onCoverError = (id: string) => {
  coverFailed.value[id] = true
}

const isDownloading = computed(() => downloadingId.value !== '')

onMounted(fetchUpdates)
onUnmounted(stopPolling)
</script>

<template>
  <div class="update-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">🔄 离线更新检测</h2>
        <p class="subtitle">
          联网核对每个离线画廊的在线详情，发现新增页数或更新版（父画廊关系）时标记为「需要更新」
        </p>
      </div>
      <div class="header-actions">
        <button class="link-btn" :disabled="isLoading" @click="fetchUpdates">⟳ 刷新列表</button>
        <button class="link-btn" @click="goDownloads">📥 前往下载列表</button>
        <button class="check-btn" :disabled="isChecking" @click="runCheck">
          {{ isChecking ? '⏳ 检测中...' : '🔍 开始检测' }}
        </button>
      </div>
    </div>

    <div class="scope-hint">
      💡 范围：默认检测所有离线漫画。可在「设置 → 额外扫描路径」中关闭某路径的「离线维护」开关，
      该路径下的漫画将不参与本检测（下载导入的漫画始终参与）。
    </div>

    <div v-if="isChecking" class="checking-banner">
      <span class="spinner"></span>
      <div class="checking-info">
        <p class="checking-title">
          {{ phaseText || '正在联网核对离线画廊更新' }}
          <span v-if="taskState && taskState.total > 0" class="checking-percent"
            >{{ progressPercent }}%</span
          >
        </p>
        <div v-if="taskState && taskState.total > 0" class="progress-track">
          <div class="progress-fill" :style="{ width: progressPercent + '%' }"></div>
        </div>
        <p class="checking-sub">
          <template v-if="taskState && taskState.total > 0">
            进度 {{ taskState.done }} / {{ taskState.total }} · {{ phaseText }}
          </template>
          <template v-else>正在启动检测任务...</template>
        </p>
        <p v-if="currentTitle" class="checking-current">📖 {{ currentTitle }}</p>
      </div>
    </div>

    <div v-if="isLoading" class="empty-box">
      <span class="spinner"></span>
      <p>加载中...</p>
    </div>

    <div v-else-if="updates.length === 0" class="empty-box">
      <span class="icon">✅</span>
      <p class="empty-title">暂无需要更新的漫画</p>
      <p class="empty-sub">
        点击「开始检测」联网核对所有离线画廊，有新增页数或更新版时会出现在这里。
      </p>
    </div>

    <div v-else class="update-list">
      <div class="summary-bar">
        <span
          >共 <b class="highlight">{{ total }}</b> 个漫画需要更新</span
        >
      </div>

      <div
        v-for="comic in updates"
        :key="comic.id"
        class="update-card"
        title="点击查看双列对比（左=本地原版，右=线上最新版）"
        @click="openCompare(comic.id)"
      >
        <div class="cover-box">
          <img
            v-if="comic.coverUrl && !coverFailed[comic.id]"
            :src="comic.coverUrl"
            :alt="comic.title"
            loading="lazy"
            @error="onCoverError(comic.id)"
          />
          <span v-else class="cover-fallback">🔄</span>
        </div>

        <div class="card-main">
          <div class="card-top">
            <h3 class="card-title">{{ comic.title }}</h3>
            <span class="mode-chip">{{
              comic.sourceMode === 'gallery' ? '📁 画廊' : '🗜️ 归档'
            }}</span>
            <span class="compare-hint">⇄ 对比</span>
          </div>

          <div class="card-tags">
            <span v-if="comic.category" class="cat-chip">{{ comic.category }}</span>
            <span class="meta-text">📄 {{ comic.pageCount || 0 }} 页</span>
            <span class="meta-text">💾 {{ formatBytes(comic.fileSize) }}</span>
            <span class="meta-text">🕒 {{ formatDate(comic.updatedAt) }}</span>
          </div>

          <div class="update-note">
            <span class="note-icon">⚠️</span>
            <span>{{ comic.updateNote || '检测到新版本' }}</span>
          </div>

          <div v-if="comic.newGID" class="new-gid">
            <span class="new-gid-label">新版 GID</span>
            <code>{{ comic.newGID }}</code>
          </div>
        </div>

        <div class="card-actions" @click.stop>
          <label class="mode-label" for="mode">下载方案</label>
          <select
            :id="`mode-${comic.id}`"
            v-model="modeFor[comic.id]"
            class="mode-select"
            :disabled="isDownloading"
          >
            <option value="">按设置（默认）</option>
            <option value="archive">🗜️ 归档（H@H）</option>
            <option value="gallery">📁 画廊（逐图）</option>
          </select>
          <button class="download-btn" :disabled="isDownloading" @click="startDownload(comic)">
            {{ downloadingId === comic.id ? '⏳ 加入中...' : '⬇️ 下载新版' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.update-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
  gap: 12px;
  flex-wrap: wrap;
}

.page-title {
  font-size: 1.3rem;
  color: var(--app-text-strong);
  margin: 0;
}

.subtitle {
  font-size: 0.85rem;
  color: var(--app-text-3);
  margin: 4px 0 0 0;
  max-width: 640px;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.link-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 8px 14px;
  border-radius: 6px;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
}
.link-btn:hover {
  border-color: #007acc;
  color: #007acc;
}
.link-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.check-btn {
  background: #007acc;
  color: #fff;
  border: none;
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 0.88rem;
  cursor: pointer;
  transition: opacity 0.2s;
}
.check-btn:hover {
  opacity: 0.85;
}
.check-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.checking-banner {
  display: flex;
  align-items: center;
  gap: 14px;
  background-color: #14283a;
  border: 1px solid #007acc;
  border-radius: 8px;
  padding: 14px 16px;
}
.scope-hint {
  margin: 4px 0 12px;
  padding: 10px 14px;
  background-color: rgba(61, 90, 254, 0.08);
  border: 1px solid rgba(61, 90, 254, 0.35);
  border-left: 3px solid #3d5afe;
  border-radius: 6px;
  color: #a8b0d8;
  font-size: 0.78rem;
  line-height: 1.5;
}
.checking-title {
  color: #fff;
  margin: 0;
  font-weight: 600;
  font-size: 0.92rem;
}
.checking-sub {
  color: #9bb6c8;
  margin: 3px 0 0 0;
  font-size: 0.78rem;
}
.checking-info {
  flex: 1;
  min-width: 0;
}
.checking-percent {
  margin-left: 8px;
  color: #7ec8ff;
  font-weight: 700;
}
.checking-current {
  color: #d7e6f0;
  margin: 6px 0 0 0;
  font-size: 0.82rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.progress-track {
  height: 6px;
  background-color: rgba(255, 255, 255, 0.12);
  border-radius: 4px;
  margin-top: 8px;
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #007acc, #4cc3ff);
  border-radius: 4px;
  transition: width 0.3s ease;
}

.spinner {
  width: 18px;
  height: 18px;
  border: 2px solid rgba(255, 255, 255, 0.2);
  border-top-color: #007acc;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  flex-shrink: 0;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 0;
  color: var(--app-text-3);
  text-align: center;
}
.empty-box .icon {
  font-size: 3rem;
  margin-bottom: 12px;
}
.empty-title {
  color: var(--app-text-2);
  font-size: 1rem;
  margin: 0;
}
.empty-sub {
  font-size: 0.82rem;
  margin: 6px 0 0 0;
  max-width: 420px;
}

.update-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.summary-bar {
  font-size: 0.85rem;
  color: var(--app-text-2);
}
.highlight {
  color: #ff7588;
}

.update-card {
  display: flex;
  gap: 16px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 14px;
  align-items: flex-start;
  cursor: pointer;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.update-card:hover {
  border-color: #00a896;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.18);
}

.compare-hint {
  flex-shrink: 0;
  font-size: 0.7rem;
  font-weight: 600;
  color: #00a896;
  border: 1px solid rgba(0, 168, 150, 0.4);
  background: rgba(0, 168, 150, 0.1);
  padding: 2px 8px;
  border-radius: 999px;
}

.cover-box {
  width: 64px;
  height: 90px;
  border-radius: 6px;
  overflow: hidden;
  background-color: var(--app-surface-3);
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}
.cover-box img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.cover-fallback {
  font-size: 1.6rem;
}

.card-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.card-top {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.card-title {
  font-size: 0.95rem;
  color: var(--app-text-strong);
  margin: 0;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.mode-chip {
  font-size: 0.7rem;
  color: #9bb6c8;
  background-color: #14283a;
  border: 1px solid #007acc;
  padding: 2px 8px;
  border-radius: 10px;
  flex-shrink: 0;
}

.card-tags {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.cat-chip {
  font-size: 0.72rem;
  color: #ff7588;
  border: 1px solid #5a2a33;
  padding: 1px 8px;
  border-radius: 10px;
}
.meta-text {
  font-size: 0.76rem;
  color: var(--app-text-3);
}

.update-note {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 0.82rem;
  color: #f59e0b;
  background-color: #2a2414;
  border: 1px solid #5a4a1a;
  border-radius: 6px;
  padding: 6px 10px;
}
.note-icon {
  flex-shrink: 0;
}

.new-gid {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.75rem;
  color: var(--app-text-3);
}
.new-gid-label {
  flex-shrink: 0;
}
.new-gid code {
  font-family: Consolas, monospace;
  color: #007acc;
  background-color: #0f202e;
  padding: 2px 6px;
  border-radius: 4px;
}

.card-actions {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
  flex-shrink: 0;
  width: 150px;
}
.mode-label {
  font-size: 0.7rem;
  color: var(--app-text-3);
}
.mode-select {
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 7px 8px;
  border-radius: 6px;
  font-size: 0.8rem;
  outline: none;
  cursor: pointer;
}
.mode-select:focus {
  border-color: #007acc;
}
.mode-select:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.download-btn {
  background-color: #ff7588;
  border: none;
  color: #fff;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 0.82rem;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.2s;
}
.download-btn:hover {
  opacity: 0.85;
}
.download-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

@media (max-width: 720px) {
  .update-card {
    flex-direction: column;
    align-items: stretch;
  }
  .card-actions {
    width: 100%;
  }
}
</style>
