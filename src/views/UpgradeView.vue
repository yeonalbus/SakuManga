<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'

const router = useRouter()
const { toast } = useUI()

// 后端 GET /offline/upgrade/list 返回的升级候选 DTO
interface UpgradeCandidateDTO {
  id: string
  title: string
  coverUrl: string
  category?: string
  pageCount?: number
  fileSize?: number
  localPath?: string
  gid?: string
  updatedAt: string
  sourceMode?: string // gallery | archive
  downloadScheme?: string // gallery | archiveResample | ''（存量未知）
  upgrading?: boolean // 该 gid 已有进行中的下载任务
}

interface UpgradeListResponse {
  items: UpgradeCandidateDTO[]
  total: number
}

const candidates = ref<UpgradeCandidateDTO[]>([])
const total = ref(0)
const isLoading = ref(false)
const upgradingId = ref('') // 正在发起升级的单本 id
const isBatchRunning = ref(false) // 批量升级进行中
const batchDone = ref(0)
const batchTotal = ref(0)

// 下载方案展示映射（升级候选不可能出现 archiveOriginal，已达标被排除）
const SCHEME_META: Record<string, { label: string; cls: string }> = {
  gallery: { label: '📁 画廊下载', cls: 'scheme-gallery' },
  galleryOriginal: { label: '📁 画廊原图', cls: 'scheme-gallery' },
  archiveResample: { label: '🗜️ 归档压缩', cls: 'scheme-archive' },
  '': { label: '❓ 版本未知', cls: 'scheme-unknown' },
}
const schemeMeta = (s?: string) => SCHEME_META[s || ''] || SCHEME_META['']

const fetchCandidates = async () => {
  isLoading.value = true
  try {
    const data = await http<UpgradeListResponse>('/offline/upgrade/list')
    candidates.value = data.items || []
    total.value = data.total || 0
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '获取升级候选列表失败')
  } finally {
    isLoading.value = false
  }
}

// 单本升级：创建归档原图下载任务（后端复用 UpdateForComicID 关联旧版，完成后自动删除）
const startUpgrade = async (candidate: UpgradeCandidateDTO) => {
  if (upgradingId.value || candidate.upgrading) return
  upgradingId.value = candidate.id
  try {
    await http<{ task: unknown }>('/offline/upgrade/download', {
      method: 'POST',
      body: JSON.stringify({ comicId: candidate.id }),
    })
    toast.success(`《${candidate.title}》升级任务已加入下载队列 ⬆️`)
    candidate.upgrading = true
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '加入升级队列失败')
  } finally {
    upgradingId.value = ''
  }
}

// 批量升级：逐本串行创建任务（gid 去重由后端兜底），实时显示进度
const startBatchUpgrade = async () => {
  if (isBatchRunning.value) return
  const targets = candidates.value.filter((c) => !c.upgrading)
  if (targets.length === 0) {
    toast.info('没有可升级的漫画（均已加入或升级中）')
    return
  }
  isBatchRunning.value = true
  batchDone.value = 0
  batchTotal.value = targets.length
  let ok = 0
  let fail = 0
  for (const c of targets) {
    try {
      await http<{ task: unknown }>('/offline/upgrade/download', {
        method: 'POST',
        body: JSON.stringify({ comicId: c.id }),
      })
      c.upgrading = true
      ok++
    } catch {
      fail++
    }
    batchDone.value++
  }
  isBatchRunning.value = false
  toast.success(`批量升级完成：成功加入 ${ok} 个${fail ? `，失败 ${fail} 个` : ''} 📥`)
}

const goDownloads = () => {
  router.push('/downloads')
}

const batchPercent = computed(() => {
  if (batchTotal.value <= 0) return 0
  return Math.min(100, Math.round((batchDone.value / batchTotal.value) * 100))
})
const upgradableCount = computed(() => candidates.value.filter((c) => !c.upgrading).length)

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

const isAnyBusy = computed(() => upgradingId.value !== '' || isBatchRunning.value)

onMounted(fetchCandidates)
</script>

<template>
  <div class="upgrade-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">⬆️ 图片质量升级</h2>
        <p class="subtitle">
          检测「非归档原图」版本的本子（画廊下载 / 归档压缩），确认后升级为归档原图（H@H
          原图包）下载方案，完成后自动删除旧版本
        </p>
      </div>
      <div class="header-actions">
        <button class="link-btn" :disabled="isLoading" @click="fetchCandidates">⟳ 刷新列表</button>
        <button class="link-btn" @click="goDownloads">📥 前往下载列表</button>
        <button
          class="check-btn"
          :disabled="isBatchRunning || upgradableCount === 0"
          @click="startBatchUpgrade"
        >
          {{ isBatchRunning ? `⏳ 批量升级中 ${batchDone}/${batchTotal}` : '⬆️ 全部升级' }}
        </button>
      </div>
    </div>

    <div class="scope-hint">
      💡 范围：仅检测 SakuManga 自己下载的本子（以元数据来源标记为唯一标准，不做文件尺寸判定）。
      方案识别顺序：下载记录方案 → 下载任务列表方案（归档·原图/归档·压缩/画廊）→ 落地目录形态
      （归档目录视为归档原图）。手动导入 / 其他下载器下载的本子不纳入升级范围。
    </div>

    <!-- 批量升级进度条 -->
    <div v-if="isBatchRunning" class="batch-banner">
      <span class="spinner"></span>
      <div class="batch-info">
        <p class="batch-title">
          正在批量加入升级队列... <span class="batch-percent">{{ batchPercent }}%</span>
        </p>
        <div class="progress-track">
          <div class="progress-fill" :style="{ width: batchPercent + '%' }"></div>
        </div>
        <p class="batch-sub">进度 {{ batchDone }} / {{ batchTotal }}（逐本创建下载任务）</p>
      </div>
    </div>

    <div v-if="isLoading" class="empty-box">
      <span class="spinner"></span>
      <p>加载中...</p>
    </div>

    <div v-else-if="candidates.length === 0" class="empty-box">
      <span class="icon">✅</span>
      <p class="empty-title">暂无需要升级的漫画</p>
      <p class="empty-sub">
        所有 SakuManga 下载的本子都已是归档原图版本，或本地暂无符合条件的漫画。
      </p>
    </div>

    <div v-else class="upgrade-list">
      <div class="summary-bar">
        <span
          >共 <b class="highlight">{{ total }}</b> 个漫画可升级
          <template v-if="upgradableCount < total">（{{ upgradableCount }} 个待升级，其余升级中）</template></span
        >
      </div>

      <div v-for="candidate in candidates" :key="candidate.id" class="upgrade-card">
        <div class="cover-box">
          <img
            v-if="candidate.coverUrl && !coverFailed[candidate.id]"
            :src="candidate.coverUrl"
            :alt="candidate.title"
            loading="lazy"
            @error="onCoverError(candidate.id)"
          />
          <span v-else class="cover-fallback">⬆️</span>
        </div>

        <div class="card-main">
          <div class="card-top">
            <h3 class="card-title">{{ candidate.title }}</h3>
            <span class="scheme-chip" :class="schemeMeta(candidate.downloadScheme).cls">{{
              schemeMeta(candidate.downloadScheme).label
            }}</span>
            <span v-if="candidate.upgrading" class="upgrading-chip">⏳ 升级中</span>
          </div>

          <div class="card-tags">
            <span v-if="candidate.category" class="cat-chip">{{ candidate.category }}</span>
            <span class="meta-text">📄 {{ candidate.pageCount || 0 }} 页</span>
            <span class="meta-text">💾 {{ formatBytes(candidate.fileSize) }}</span>
            <span class="meta-text">🕒 {{ formatDate(candidate.updatedAt) }}</span>
            <span class="meta-text">🔗 gid: {{ candidate.gid || '—' }}</span>
          </div>

          <div class="upgrade-note">
            <span class="note-icon">⬆️</span>
            <span>升级为「归档原图」：H@H 原图包下载，下载完成后自动删除旧版本。</span>
          </div>
        </div>

        <div class="card-actions">
          <button
            class="upgrade-btn"
            :disabled="isAnyBusy || candidate.upgrading"
            @click="startUpgrade(candidate)"
          >
            {{
              candidate.upgrading
                ? '⏳ 升级中'
                : upgradingId === candidate.id
                  ? '⏳ 加入中...'
                  : '⬆️ 升级为原图'
            }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.upgrade-page {
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

.batch-banner {
  display: flex;
  align-items: center;
  gap: 14px;
  background-color: #14283a;
  border: 1px solid #007acc;
  border-radius: 8px;
  padding: 14px 16px;
}
.batch-info {
  flex: 1;
  min-width: 0;
}
.batch-title {
  color: #fff;
  margin: 0;
  font-weight: 600;
  font-size: 0.92rem;
}
.batch-percent {
  margin-left: 8px;
  color: #7ec8ff;
  font-weight: 700;
}
.batch-sub {
  color: #9bb6c8;
  margin: 3px 0 0 0;
  font-size: 0.78rem;
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

.upgrade-list {
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

.upgrade-card {
  display: flex;
  gap: 16px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 14px;
  align-items: flex-start;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}
.upgrade-card:hover {
  border-color: #00a896;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.18);
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

.scheme-chip {
  font-size: 0.7rem;
  padding: 2px 8px;
  border-radius: 10px;
  flex-shrink: 0;
}
.scheme-gallery {
  color: #9bb6c8;
  background-color: #14283a;
  border: 1px solid #007acc;
}
.scheme-archive {
  color: #f0b35c;
  background-color: #2a2414;
  border: 1px solid #5a4a1a;
}
.scheme-unknown {
  color: #b8b8b8;
  background-color: #222;
  border: 1px solid #555;
}

.upgrading-chip {
  flex-shrink: 0;
  font-size: 0.7rem;
  font-weight: 600;
  color: #4cc3ff;
  border: 1px solid rgba(76, 195, 255, 0.4);
  background: rgba(76, 195, 255, 0.1);
  padding: 2px 8px;
  border-radius: 999px;
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

.upgrade-note {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 0.82rem;
  color: #4cc3ff;
  background-color: #0f202e;
  border: 1px solid #007acc;
  border-radius: 6px;
  padding: 6px 10px;
}
.note-icon {
  flex-shrink: 0;
}

.card-actions {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
  flex-shrink: 0;
  width: 150px;
}

.upgrade-btn {
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
.upgrade-btn:hover {
  opacity: 0.85;
}
.upgrade-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

@media (max-width: 720px) {
  .upgrade-card {
    flex-direction: column;
    align-items: stretch;
  }
  .card-actions {
    width: 100%;
  }
}
</style>
