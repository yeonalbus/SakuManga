<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'

const { modal, toast } = useUI()

// 后端 GET /offline/maintain 返回的离线漫画 DTO
interface OfflineComicDTO {
  id: string
  title: string
  coverUrl: string
  category?: string
  pageCount?: number
  updatedAt: string
  localPath?: string
  fileSize?: number
  gid?: string
  sourceMode?: string
}

interface DedupItemDTO {
  comic: OfflineComicDTO
  reason: string // 重复原因
  keep: boolean // true=建议保留，false=建议删除
}

interface DedupResultDTO {
  items: DedupItemDTO[]
}

const items = ref<DedupItemDTO[]>([])
const isScanning = ref(false)
const isRemoving = ref(false)
const removingId = ref('')
const coverFailed = ref<Record<string, boolean>>({})
// 批量删除：多选“建议删除”项后一次提交，避免反复“删除→刷新”
const selectedIds = ref<string[]>([])

// 拉取查重结果
const runMaintain = async () => {
  isScanning.value = true
  try {
    const data = await http<DedupResultDTO>('/offline/maintain')
    items.value = data.items || []
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '查重失败，请检查后端是否运行')
  } finally {
    isScanning.value = false
  }
}

const keepItems = computed(() => items.value.filter((i) => i.keep))
const removeItems = computed(() => items.value.filter((i) => !i.keep))
const isSelectAll = computed(
  () => removeItems.value.length > 0 && selectedIds.value.length === removeItems.value.length,
)

// 删除重复项：deleteFile=true 时同时物理删除本地文件
const removeComic = async (item: DedupItemDTO, deleteFile: boolean) => {
  if (isRemoving.value) return
  const c = item.comic
  const title = c.title || '未命名'

  const confirmed = await modal.confirm(
    deleteFile
      ? `确定删除《${title}》并同时删除其本地文件吗？\n\n📁 ${c.localPath || ''}\n\n此操作不可恢复！`
      : `确定仅删除《${title}》的记录吗？\n\n本地文件将保留：📁 ${c.localPath || ''}`,
    deleteFile ? '删除记录 + 本地文件' : '删除记录（保留文件）',
  )
  if (!confirmed) return

  isRemoving.value = true
  removingId.value = c.id
  try {
    await http<{ ok: boolean }>('/offline/maintain/remove', {
      method: 'POST',
      body: JSON.stringify({ comicId: c.id, deleteFile }),
    })
    toast.success(
      deleteFile ? `《${title}》记录与本地文件已删除 🗑️` : `《${title}》记录已删除（保留本地文件）`,
    )
    // 刷新查重结果（删除后保留/删除关系会变化）
    await runMaintain()
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '删除失败')
  } finally {
    isRemoving.value = false
    removingId.value = ''
  }
}

const onCoverError = (id: string) => {
  coverFailed.value[id] = true
}

// 勾选/取消勾选单个删除项
const toggleSelect = (id: string) => {
  if (isRemoving.value) return
  const idx = selectedIds.value.indexOf(id)
  if (idx >= 0) selectedIds.value.splice(idx, 1)
  else selectedIds.value.push(id)
}

// 全选/取消全选“建议删除”项
const toggleSelectAll = () => {
  if (isRemoving.value) return
  selectedIds.value = isSelectAll.value ? [] : removeItems.value.map((i) => i.comic.id)
}

// 批量删除：comicIds 一次提交后端，避免反复“删除→刷新”
const removeSelected = async (deleteFile: boolean) => {
  if (isRemoving.value) return
  if (selectedIds.value.length === 0) {
    toast.warning('请先勾选要删除的项')
    return
  }
  const ids = [...selectedIds.value]
  const count = ids.length
  const confirmed = await modal.confirm(
    deleteFile
      ? `确定批量删除选中的 ${count} 项，并同时删除其本地文件吗？\n\n此操作不可恢复！`
      : `确定批量删除选中的 ${count} 项记录吗？\n\n本地文件将保留。`,
    deleteFile ? '批量删除记录 + 本地文件' : '批量删除记录（保留文件）',
  )
  if (!confirmed) return

  isRemoving.value = true
  try {
    const data = await http<{ ok: boolean; deleted?: number }>('/offline/maintain/remove', {
      method: 'POST',
      body: JSON.stringify({ comicIds: ids, deleteFile }),
    })
    const deleted = data.deleted ?? count
    toast.success(
      deleteFile
        ? `已批量删除 ${deleted} 项（记录 + 本地文件）🗑️`
        : `已批量删除 ${deleted} 项记录（保留本地文件）`,
    )
    selectedIds.value = []
    // 刷新查重结果（删除后保留/删除关系会变化）
    await runMaintain()
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '批量删除失败')
  } finally {
    isRemoving.value = false
  }
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

const modeText = (mode?: string) => (mode === 'gallery' ? '📁 画廊' : '🗜️ 归档')

onMounted(runMaintain)
</script>

<template>
  <div class="maintenance-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">🛠️ 本地书库维护与查重</h2>
        <p class="subtitle">
          扫描本地书库：同 GID 重复、归档 Hash 相同、父画廊关系旧版被取代，均可在此清理
        </p>
      </div>

      <button class="scan-btn" :disabled="isScanning" @click="runMaintain">
        {{ isScanning ? '扫描中...' : '🔍 重新全盘扫描' }}
      </button>
    </div>

    <div class="scope-hint">
      💡 范围：默认查重所有离线漫画。可在「设置 → 额外扫描路径」中关闭某路径的「离线维护」开关，
      该路径下的漫画将不参与本查重（下载导入的漫画始终参与）。
    </div>

    <div v-if="isScanning" class="scanning-banner">
      <span class="spinner"></span>
      <div>
        <p class="scanning-title">正在扫描本地书库查重...</p>
        <p class="scanning-sub">对归档文件计算 Hash 可能需要一些时间，请稍候。</p>
      </div>
    </div>

    <div v-else-if="items.length === 0" class="empty-box">
      <span class="icon">🎉</span>
      <p class="empty-title">恭喜！本地画库暂无重复或异常项</p>
      <p class="empty-sub">若刚导入新内容，可点击「重新全盘扫描」再次核对。</p>
    </div>

    <template v-else>
      <div class="summary-bar">
        <span class="summary-item warn"
          >🗑️ 建议删除 <b>{{ removeItems.length }}</b> 项</span
        >
        <span class="summary-item ok"
          >✔ 建议保留 <b>{{ keepItems.length }}</b> 项</span
        >
      </div>

      <!-- 建议删除区 -->
      <div v-if="removeItems.length > 0" class="section">
        <h3 class="section-title danger">🗑️ 建议删除</h3>
        <div class="remove-toolbar">
          <label class="select-all">
            <input
              type="checkbox"
              :checked="isSelectAll"
              :disabled="isRemoving || removeItems.length === 0"
              @change="toggleSelectAll"
            />
            <span>全选</span>
          </label>
          <span class="selected-count"
            >已选 {{ selectedIds.length }} / {{ removeItems.length }} 项</span
          >
          <div class="batch-actions">
            <button
              class="action-btn danger-soft"
              :disabled="isRemoving || selectedIds.length === 0"
              @click="removeSelected(false)"
            >
              {{ isRemoving ? '⏳ 处理中...' : '批量删除（保留文件）' }}
            </button>
            <button
              class="action-btn danger"
              :disabled="isRemoving || selectedIds.length === 0"
              @click="removeSelected(true)"
            >
              {{ isRemoving ? '⏳ 处理中...' : '批量删除（含文件）' }}
            </button>
          </div>
        </div>
        <div class="item-list">
          <div
            v-for="item in removeItems"
            :key="item.comic.id"
            class="dedup-card remove-card"
            :class="{ selected: selectedIds.includes(item.comic.id) }"
          >
            <label class="select-check" @click.stop>
              <input
                type="checkbox"
                :checked="selectedIds.includes(item.comic.id)"
                :disabled="isRemoving"
                @change="toggleSelect(item.comic.id)"
              />
            </label>
            <div class="cover-box">
              <img
                v-if="item.comic.coverUrl && !coverFailed[item.comic.id]"
                :src="item.comic.coverUrl"
                :alt="item.comic.title"
                loading="lazy"
                @error="onCoverError(item.comic.id)"
              />
              <span v-else class="cover-fallback">🗑️</span>
            </div>

            <div class="card-main">
              <div class="card-top">
                <h4 class="card-title">{{ item.comic.title }}</h4>
                <span class="mode-chip">{{ modeText(item.comic.sourceMode) }}</span>
              </div>
              <div class="reason-box">
                <span class="reason-icon">⚠️</span>
                <span>{{ item.reason }}</span>
              </div>
              <div class="card-meta">
                <span class="meta-text">📄 {{ item.comic.pageCount || 0 }} 页</span>
                <span class="meta-text">💾 {{ formatBytes(item.comic.fileSize) }}</span>
                <span class="meta-text">🕒 {{ formatDate(item.comic.updatedAt) }}</span>
              </div>
              <div class="card-path">📁 {{ item.comic.localPath || '—' }}</div>
            </div>

            <div class="card-actions">
              <button
                class="action-btn danger-soft"
                :disabled="isRemoving"
                @click="removeComic(item, false)"
              >
                {{
                  isRemoving && removingId === item.comic.id ? '⏳ 处理中...' : '删除（保留文件）'
                }}
              </button>
              <button
                class="action-btn danger"
                :disabled="isRemoving"
                @click="removeComic(item, true)"
              >
                {{ isRemoving && removingId === item.comic.id ? '⏳ 处理中...' : '删除（含文件）' }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 建议保留区 -->
      <div v-if="keepItems.length > 0" class="section">
        <h3 class="section-title ok">✔ 建议保留</h3>
        <div class="item-list">
          <div v-for="item in keepItems" :key="item.comic.id" class="dedup-card keep-card">
            <div class="cover-box">
              <img
                v-if="item.comic.coverUrl && !coverFailed[item.comic.id]"
                :src="item.comic.coverUrl"
                :alt="item.comic.title"
                loading="lazy"
                @error="onCoverError(item.comic.id)"
              />
              <span v-else class="cover-fallback">✔</span>
            </div>

            <div class="card-main">
              <div class="card-top">
                <h4 class="card-title">{{ item.comic.title }}</h4>
                <span class="mode-chip">{{ modeText(item.comic.sourceMode) }}</span>
              </div>
              <div class="keep-box">
                <span class="keep-icon">✔</span>
                <span>保留该副本（与上面待删除项形成同一组）</span>
              </div>
              <div class="card-meta">
                <span class="meta-text">📄 {{ item.comic.pageCount || 0 }} 页</span>
                <span class="meta-text">💾 {{ formatBytes(item.comic.fileSize) }}</span>
                <span class="meta-text">🕒 {{ formatDate(item.comic.updatedAt) }}</span>
              </div>
              <div class="card-path">📁 {{ item.comic.localPath || '—' }}</div>
            </div>

            <div class="card-actions">
              <span class="kept-badge">已保留</span>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.maintenance-page {
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

.scan-btn {
  background: #007acc;
  color: #fff;
  border: none;
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 0.88rem;
  cursor: pointer;
  transition: opacity 0.2s;
}
.scan-btn:hover {
  opacity: 0.85;
}
.scan-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.scanning-banner {
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
.scanning-title {
  color: #fff;
  margin: 0;
  font-weight: 600;
  font-size: 0.92rem;
}
.scanning-sub {
  color: #9bb6c8;
  margin: 3px 0 0 0;
  font-size: 0.78rem;
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

.summary-bar {
  display: flex;
  gap: 16px;
  font-size: 0.85rem;
  flex-wrap: wrap;
}
.summary-item b {
  font-size: 1rem;
}
.summary-item.warn {
  color: #f59e0b;
}
.summary-item.ok {
  color: #4caf50;
}

.section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.section-title {
  font-size: 0.95rem;
  margin: 0;
}
.section-title.danger {
  color: #ff7588;
}
.section-title.ok {
  color: #4caf50;
}

.item-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.dedup-card {
  display: flex;
  gap: 14px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 14px;
  align-items: flex-start;
}
.remove-card {
  border-left: 3px solid #ff7588;
}
.keep-card {
  border-left: 3px solid #4caf50;
  opacity: 0.92;
}

.cover-box {
  width: 56px;
  height: 78px;
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
  font-size: 1.4rem;
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
  font-size: 0.92rem;
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

.reason-box {
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
.reason-icon {
  flex-shrink: 0;
}

.keep-box {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 0.82rem;
  color: #4caf50;
  background-color: #14281a;
  border: 1px solid #1f4a2a;
  border-radius: 6px;
  padding: 6px 10px;
}
.keep-icon {
  flex-shrink: 0;
}

.card-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.meta-text {
  font-size: 0.75rem;
  color: var(--app-text-3);
}

.card-path {
  font-size: 0.72rem;
  color: #007acc;
  font-family: Consolas, monospace;
  word-break: break-all;
}

.card-actions {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
  flex-shrink: 0;
  width: 150px;
  justify-content: center;
}

.action-btn {
  border: none;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.2s;
  color: #fff;
}
.action-btn:hover {
  opacity: 0.85;
}
.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.action-btn.danger-soft {
  background-color: #5a3a3a;
}
.action-btn.danger {
  background-color: #ff7588;
}

.kept-badge {
  display: inline-block;
  text-align: center;
  font-size: 0.8rem;
  font-weight: 600;
  color: #4caf50;
  border: 1px solid #2f6b3a;
  background-color: #14281a;
  padding: 6px 10px;
  border-radius: 6px;
}

.remove-toolbar {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
  background-color: rgba(245, 158, 11, 0.06);
  border: 1px solid rgba(245, 158, 11, 0.25);
  border-radius: 6px;
  padding: 8px 12px;
}
.select-all {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #e5e7eb;
  font-size: 0.82rem;
  cursor: pointer;
  user-select: none;
}
.select-all input,
.select-check input {
  width: 16px;
  height: 16px;
  accent-color: #ff7588;
  cursor: pointer;
}
.select-check {
  flex-shrink: 0;
  padding-top: 2px;
  cursor: pointer;
}
.select-all:has(input:disabled),
.select-check:has(input:disabled) {
  opacity: 0.5;
  cursor: not-allowed;
}
.selected-count {
  color: #f59e0b;
  font-size: 0.82rem;
  font-weight: 600;
}
.batch-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
  flex-wrap: wrap;
}
.dedup-card.selected {
  border-color: #ff7588;
  box-shadow: 0 0 0 1px rgba(255, 117, 136, 0.4);
}

@media (max-width: 720px) {
  .dedup-card {
    flex-direction: column;
    align-items: stretch;
  }
  .card-actions {
    width: 100%;
    flex-direction: row;
  }
  .card-actions .action-btn {
    flex: 1;
  }
  .kept-badge {
    text-align: center;
  }
}
</style>
