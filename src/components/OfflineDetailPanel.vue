<script setup lang="ts">
import { ref } from 'vue'

// 本地离线漫画的紧凑详情数据（由父级从 GET /comics/:id 或维护查重结果归一化传入）
// Round4 任务一：双列对比视图的左侧/右侧本地面板数据源
export interface OfflineDetailComic {
  id: string
  title: string
  coverUrl: string
  category?: string
  pageCount?: number
  gid?: string
  publishedAt?: string
  addedAt?: string
  fileModifiedAt?: string
  updatedAt?: string
  localPath?: string
  fileSize?: number
  sourceMode?: string
}

export interface DetailTag {
  name: string
  source?: 'online' | 'local'
}

withDefaults(
  defineProps<{
    comic: OfflineDetailComic | null
    tags?: DetailTag[]
    badge?: string // 面板角标（本地原版 / 建议保留 / 建议删除）
    badgeType?: 'ok' | 'danger' | 'info'
    reason?: string // 查重原因 / 更新说明
  }>(),
  { comic: null, tags: () => [], badge: '', badgeType: 'info', reason: '' },
)

defineEmits<{
  (e: 'open-full'): void // 打开完整离线详情页（父级路由跳转 /offline/detail?id=...）
}>()

const coverFailed = ref(false)

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
</script>

<template>
  <aside class="offline-detail-panel">
    <header class="odp-header">
      <div class="odp-header-left">
        <span v-if="badge" class="odp-badge" :class="'odp-badge--' + badgeType">{{ badge }}</span>
        <button
          class="odp-title"
          :disabled="!comic"
          title="在新标签页打开完整详情"
          @click="$emit('open-full')"
        >
          📚 本地详情 <span class="open-hint">↗</span>
        </button>
      </div>
      <span v-if="comic" class="odp-mode">{{ modeText(comic.sourceMode) }}</span>
    </header>

    <div v-if="!comic" class="odp-empty">
      <div class="empty-icon">📭</div>
      <p>无本地数据</p>
    </div>

    <div v-else class="odp-body">
      <div class="odp-cover-wrap">
        <img
          v-if="comic.coverUrl && !coverFailed"
          :src="comic.coverUrl"
          :alt="comic.title"
          loading="lazy"
          @error="coverFailed = true"
        />
        <div v-else class="odp-cover-fallback">📚</div>
      </div>

      <h3 class="odp-title-main">{{ comic.title }}</h3>

      <div class="odp-chips">
        <span v-if="comic.category" class="odp-chip">{{ comic.category }}</span>
        <span class="odp-chip">{{ comic.pageCount || 0 }} 页</span>
        <span v-if="comic.gid" class="odp-chip mono">GID {{ comic.gid }}</span>
      </div>

      <dl class="odp-meta">
        <div class="odp-meta-row">
          <dt>发布时间</dt>
          <dd>{{ formatDate(comic.publishedAt) }}</dd>
        </div>
        <div class="odp-meta-row">
          <dt>入库时间</dt>
          <dd>{{ formatDate(comic.addedAt) }}</dd>
        </div>
        <div class="odp-meta-row">
          <dt>修改时间</dt>
          <dd>{{ formatDate(comic.fileModifiedAt || comic.updatedAt) }}</dd>
        </div>
        <div class="odp-meta-row">
          <dt>文件大小</dt>
          <dd>{{ formatBytes(comic.fileSize) }}</dd>
        </div>
      </dl>

      <div class="odp-path">📁 {{ comic.localPath || '—' }}</div>

      <div v-if="reason" class="odp-reason">
        <span class="odp-reason-icon">ℹ️</span>
        <span>{{ reason }}</span>
      </div>

      <div v-if="tags.length" class="odp-tags">
        <span
          v-for="t in tags"
          :key="t.name"
          class="odp-tag"
          :class="{ 'odp-tag--local': t.source === 'local' }"
          :title="t.source === 'local' ? '本地新增' : '官方标签'"
        >
          {{ t.name }}
        </span>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.offline-detail-panel {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  background: var(--app-surface);
  border: 1px solid var(--app-border-3);
  border-radius: 10px;
}

.odp-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--app-border-3);
  flex-shrink: 0;
}

.odp-header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.odp-badge {
  flex-shrink: 0;
  font-size: 0.72rem;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 999px;
  border: 1px solid transparent;
}

.odp-badge--ok {
  color: #2fbf8f;
  background: rgba(47, 191, 143, 0.12);
  border-color: rgba(47, 191, 143, 0.35);
}

.odp-badge--danger {
  color: #ff7588;
  background: rgba(255, 117, 136, 0.12);
  border-color: rgba(255, 117, 136, 0.35);
}

.odp-badge--info {
  color: #4da3ff;
  background: rgba(77, 163, 255, 0.12);
  border-color: rgba(77, 163, 255, 0.35);
}

.odp-title {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: transparent;
  border: 1px solid transparent;
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--app-text-strong);
  padding: 3px 8px;
  border-radius: 6px;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s ease;
}

.odp-title:hover:not(:disabled) {
  color: #ff7588;
  background: var(--app-surface-2);
  border-color: var(--app-border-3);
}

.odp-title:disabled {
  cursor: default;
  color: var(--app-text-3);
}

.open-hint {
  font-size: 0.75rem;
  opacity: 0.75;
}

.odp-mode {
  flex-shrink: 0;
  font-size: 0.72rem;
  color: var(--app-text-2);
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  padding: 2px 8px;
  border-radius: 999px;
}

.odp-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 24px;
  text-align: center;
  color: var(--app-text-3);
  font-size: 0.85rem;
}

.empty-icon {
  font-size: 2.4rem;
  opacity: 0.7;
}

.odp-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px;
  overflow-y: auto;
}

.odp-cover-wrap {
  position: relative;
  width: 100%;
  aspect-ratio: 3 / 4;
  border-radius: 8px;
  overflow: hidden;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  flex-shrink: 0;
}

.odp-cover-wrap img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.odp-cover-fallback {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 3rem;
  color: var(--app-text-3);
}

.odp-title-main {
  margin: 0;
  font-size: 1rem;
  font-weight: 700;
  line-height: 1.4;
  color: var(--app-text-strong);
  word-break: break-word;
}

.odp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.odp-chip {
  font-size: 0.72rem;
  color: var(--app-text-2);
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  padding: 2px 8px;
  border-radius: 999px;
}

.odp-chip.mono {
  font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
}

.odp-meta {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.odp-meta-row {
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-size: 0.8rem;
}

.odp-meta-row dt {
  flex-shrink: 0;
  width: 56px;
  color: var(--app-text-3);
}

.odp-meta-row dd {
  margin: 0;
  min-width: 0;
  color: var(--app-text-2);
  word-break: break-all;
}

.odp-path {
  font-size: 0.75rem;
  color: var(--app-text-3);
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 6px 8px;
  word-break: break-all;
  line-height: 1.5;
}

.odp-reason {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 0.8rem;
  color: var(--app-text-2);
  background: rgba(255, 193, 7, 0.08);
  border: 1px solid rgba(255, 193, 7, 0.25);
  border-radius: 6px;
  padding: 8px 10px;
  line-height: 1.5;
}

.odp-reason-icon {
  flex-shrink: 0;
}

.odp-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.odp-tag {
  font-size: 0.72rem;
  color: var(--app-text-2);
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  padding: 2px 8px;
  border-radius: 999px;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.odp-tag--local {
  color: #4da3ff;
  border-color: rgba(77, 163, 255, 0.4);
  background: rgba(77, 163, 255, 0.1);
}
</style>
