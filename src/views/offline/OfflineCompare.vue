<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import OfflineDetailPanel, { type DetailTag } from '@/components/OfflineDetailPanel.vue'
import OnlineDetail from '@/views/online/OnlineDetail.vue'
import { openComicDetailInNewTab } from '@/utils/detailNav'

// Round4 任务一：双列对比视图
//   type=update   → 左=本地原版（GET /comics/:id），右=线上最新版（newGID/newToken 复用 OnlineDetail embedded）
//   type=maintain → 左=建议保留，右=建议删除（均来自 /offline/maintain/result 的成对对象 pairComic）
// 桌面双列 grid（参照 OnlineDetailPanel 的 .online-split 布局）；移动端 / 强制移动形态上下堆叠。

const route = useRoute()
const router = useRouter()
const { toast, modal } = useUI()

const compareType = computed<'update' | 'maintain'>(() =>
  route.query.type === 'maintain' ? 'maintain' : 'update',
)
const comicId = computed(() => (route.query.id as string) || '')

const loading = ref(true)
const error = ref('')

// ── 本地漫画详情 DTO（GET /comics/:id 返回 OfflineComicResponse）──
interface OfflineDetailDTO {
  id: string
  title: string
  coverUrl: string
  category?: string
  pageCount?: number
  gid?: string
  token?: string // 本地画廊绑定的在线 Token（S7 左侧 OnlineDetail local-panel 加载用）
  publishedAt?: string
  addedAt?: string
  fileModifiedAt?: string
  updatedAt?: string
  localPath?: string
  fileSize?: number
  sourceMode?: string
  // GET /comics/:id 特有：更新标记
  newGID?: string
  newToken?: string
  updateNote?: string
  // 标签：/comics/:id 返回 TagRaws/TagSources；维护结果返回原始 "tags" JSON 字符串
  tags?: string | string[]
  TagRaws?: string[]
  TagSources?: string[]
}

// ── 维护查重结果项（含成对对象 pairComic，Round4 任务一）──
interface MaintainItem {
  comic: OfflineDetailDTO
  reason: string
  keep: boolean
  pairComic?: OfflineDetailDTO
}

// update 类型：本地原版 + 线上新版信息
const localComic = ref<OfflineDetailDTO | null>(null)
const localTags = ref<DetailTag[]>([])
const onlineGid = ref('')
const onlineToken = ref('')
const updateNote = ref('')

// S7：左侧「本地原版」使用 OnlineDetail local-panel 渲染（在线结构 + 本地页图预览），需本地 gid/token
const localGid = computed(() => localComic.value?.gid || '')
const localToken = computed(() => localComic.value?.token || '')

// maintain 类型：左=建议保留，右=建议删除
const leftComic = ref<OfflineDetailDTO | null>(null)
const leftTags = ref<DetailTag[]>([])
const leftReason = ref('')
const rightComic = ref<OfflineDetailDTO | null>(null)
const rightTags = ref<DetailTag[]>([])
const rightReason = ref('')

const downloading = ref(false)
const removing = ref(false)

// 从 DTO 归一化出面板标签列表（翻译/合并后的 TagRaws+TagSources 优先；退化为原始 tags JSON）
const buildTags = (comic: OfflineDetailDTO | null): DetailTag[] => {
  if (!comic) return []
  const out: DetailTag[] = []
  if (Array.isArray(comic.TagRaws) && Array.isArray(comic.TagSources)) {
    comic.TagRaws.forEach((name, i) => {
      out.push({ name, source: comic.TagSources?.[i] === 'local' ? 'local' : 'online' })
    })
  } else if (typeof comic.tags === 'string' && comic.tags) {
    try {
      const arr = JSON.parse(comic.tags)
      if (Array.isArray(arr)) arr.forEach((n) => out.push({ name: String(n) }))
    } catch {
      // 非 JSON 直接跳过
    }
  }
  return out.slice(0, 24)
}

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    if (compareType.value === 'update') {
      const d = await http<OfflineDetailDTO>(`/comics/${comicId.value}`)
      localComic.value = d
      localTags.value = buildTags(d)
      onlineGid.value = d.newGID || ''
      onlineToken.value = d.newToken || ''
      updateNote.value = d.updateNote || ''
    } else {
      const data = await http<{ items: MaintainItem[]; stale?: boolean }>('/offline/maintain/result')
      const items = data?.items || []
      const item = items.find((i) => i.comic.id === comicId.value)
      if (!item) {
        error.value = '未找到对应的维护项（记录可能已在其他设备删除）。请返回维护页重新扫描后重试。'
        return
      }
      const pairItem = item.pairComic
        ? items.find((i) => i.comic.id === item.pairComic?.id)
        : undefined
      if (item.keep) {
        // 左=建议保留（当前项），右=建议删除（成对对象）
        leftComic.value = item.comic
        leftTags.value = buildTags(item.comic)
        leftReason.value = item.reason
        rightComic.value = item.pairComic || null
        rightTags.value = buildTags(item.pairComic || null)
        rightReason.value = pairItem ? pairItem.reason : ''
      } else {
        // 左=建议保留（成对对象），右=建议删除（当前项）
        leftComic.value = item.pairComic || null
        leftTags.value = buildTags(item.pairComic || null)
        leftReason.value = pairItem ? pairItem.reason : ''
        rightComic.value = item.comic
        rightTags.value = buildTags(item.comic)
        rightReason.value = item.reason
      }
    }
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    error.value = msg || '加载对比数据失败'
  } finally {
    loading.value = false
  }
}

const goBack = () => {
  router.push(compareType.value === 'update' ? '/offline/update' : '/offline/maintain')
}

const openFullDetail = (comic: { id: string } | null) => {
  if (!comic?.id) return
  // S10：统一入口打开离线详情新标签（写新标签标记，返回时关闭标签）
  openComicDetailInNewTab({ id: comic.id, source: 'offline' })
}

// 更新类型：下载新版（复用 /offline/updates/download）
const downloadNew = async () => {
  if (!localComic.value || downloading.value) return
  downloading.value = true
  try {
    await http<{ task: unknown }>('/offline/updates/download', {
      method: 'POST',
      body: JSON.stringify({ comicId: comicId.value }),
    })
    toast.success(`《${localComic.value.title}》新版已加入下载队列 📥`)
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '加入下载队列失败')
  } finally {
    downloading.value = false
  }
}

// 维护类型：删除建议删除项（保留本地文件）
const removeRight = async () => {
  if (!rightComic.value || removing.value) return
  const c = rightComic.value
  const confirmed = await modal.confirm(
    `确定仅删除《${c.title}》的记录吗？\n\n本地文件将保留：📁 ${c.localPath || ''}`,
    '删除记录（保留文件）',
  )
  if (!confirmed) return
  removing.value = true
  try {
    await http<{ ok: boolean; alreadyDeleted?: boolean }>('/offline/maintain/remove', {
      method: 'POST',
      body: JSON.stringify({ comicId: c.id, deleteFile: false }),
    })
    toast.success(`《${c.title}》记录已删除（保留本地文件）`)
    goBack()
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    toast.error(msg || '删除失败')
  } finally {
    removing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="compare-page">
    <header class="compare-header">
      <button class="back-btn" @click="goBack">← 返回</button>
      <h2 class="compare-title">
        {{ compareType === 'update' ? '🔄 更新对比' : '🗂️ 维护对比' }}
      </h2>
      <div class="header-actions">
        <template v-if="compareType === 'update'">
          <button
            class="action-btn primary"
            :disabled="!localComic || downloading"
            @click="downloadNew"
          >
            {{ downloading ? '⏳ 加入中...' : '⬇️ 下载新版' }}
          </button>
        </template>
        <template v-else>
          <button
            v-if="rightComic"
            class="action-btn danger"
            :disabled="removing"
            @click="removeRight"
          >
            {{ removing ? '⏳ 删除中...' : '🗑️ 删除该对象' }}
          </button>
        </template>
      </div>
    </header>

    <div v-if="loading" class="compare-state">
      <span class="spinner"></span>
      <p>加载对比数据...</p>
    </div>

    <div v-else-if="error" class="compare-state error-state">
      <p class="error-icon">⚠️</p>
      <p>{{ error }}</p>
      <button class="action-btn ghost" @click="goBack">返回列表</button>
    </div>

    <div v-else class="online-split compare-split">
      <!-- 左侧：本地原版（update=OnlineDetail 纯本地版）/ 建议保留（maintain） -->
      <div class="split-main">
        <template v-if="compareType === 'update'">
          <aside class="compare-online-panel">
            <header class="compare-panel-header">
              <span class="compare-panel-title">📚 本地原版</span>
            </header>
            <div v-if="localGid" class="compare-online-body">
              <OnlineDetail embedded local-panel :gid="localGid" :token="localToken" />
            </div>
            <div v-else class="compare-online-empty">
              <div class="empty-icon">📭</div>
              <p>该本地漫画未绑定在线画廊（缺少 GID），无法在线对比详情。</p>
            </div>
          </aside>
        </template>
        <template v-else>
          <OfflineDetailPanel
            :comic="leftComic"
            :tags="leftTags"
            badge="建议保留"
            badge-type="ok"
            :reason="leftReason"
            @open-full="openFullDetail(leftComic)"
          />
        </template>
      </div>

      <!-- 右侧：线上最新版 / 建议删除 -->
      <div class="compare-right">
        <template v-if="compareType === 'update'">
          <aside class="compare-online-panel">
            <header class="compare-panel-header">
              <span class="compare-panel-title">🖼️ 线上最新版</span>
            </header>
            <div v-if="onlineGid" class="compare-online-body">
              <OnlineDetail embedded :gid="onlineGid" :token="onlineToken" />
            </div>
            <div v-else class="compare-online-empty">
              <div class="empty-icon">ℹ️</div>
              <p>{{ updateNote || '该漫画没有可用的新版画廊信息（可能未绑定 E 站账户，或线上画廊已不可用）。' }}</p>
            </div>
          </aside>
        </template>
        <template v-else>
          <OfflineDetailPanel
            :comic="rightComic"
            :tags="rightTags"
            badge="建议删除"
            badge-type="danger"
            :reason="rightReason"
            @open-full="openFullDetail(rightComic)"
          />
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.compare-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  height: 100%;
}

/* ── 顶栏 ── */
.compare-header {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.back-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 8px;
  padding: 6px 12px;
  font-size: 0.85rem;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s ease;
}

.back-btn:hover {
  color: var(--app-text-strong);
  border-color: var(--app-border-3);
  background: var(--app-surface-2);
}

.compare-title {
  margin: 0;
  font-size: 1.05rem;
  font-weight: 700;
  color: var(--app-text-strong);
  flex: 1;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.action-btn {
  border-radius: 8px;
  padding: 7px 14px;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  border: 1px solid transparent;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.action-btn.primary {
  background: #00a896;
  color: #fff;
}

.action-btn.primary:hover:not(:disabled) {
  background: #00b8a5;
}

.action-btn.danger {
  background: #ff5d73;
  color: #fff;
}

.action-btn.danger:hover:not(:disabled) {
  background: #ff6d81;
}

.action-btn.ghost {
  background: transparent;
  border-color: var(--app-border-3);
  color: var(--app-text-2);
}

.action-btn.ghost:hover:not(:disabled) {
  color: var(--app-text-strong);
  background: var(--app-surface-2);
}

/* ── 加载 / 错误态 ── */
.compare-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 48px 24px;
  color: var(--app-text-3);
  font-size: 0.9rem;
}

.spinner {
  width: 28px;
  height: 28px;
  border: 3px solid var(--app-border-3);
  border-top-color: #00a896;
  border-radius: 50%;
  animation: compare-spin 0.8s linear infinite;
}

@keyframes compare-spin {
  to {
    transform: rotate(360deg);
  }
}

.error-state p {
  margin: 0;
  text-align: center;
  line-height: 1.6;
}

.error-icon {
  font-size: 2rem;
}

/* ── 双列对比布局（宽屏 grid / 窄屏堆叠）── */
.compare-split {
  display: block;
}

@media (min-width: 1025px) {
  :global(html:not([data-layout='mobile']) .compare-split) {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 16px;
    align-items: start;
  }
}

.split-main,
.compare-right {
  min-width: 0;
  min-height: 0;
}

/* 更新类型右侧：线上详情容器（复用卡片外观，不 sticky，随页面滚动） */
.compare-online-panel {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  background: var(--app-surface);
  border: 1px solid var(--app-border-3);
  border-radius: 10px;
}

.compare-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--app-border-3);
  flex-shrink: 0;
}

.compare-panel-title {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--app-text-strong);
  padding: 3px 8px;
}

.compare-online-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.compare-online-empty {
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
  line-height: 1.6;
}

.empty-icon {
  font-size: 2.4rem;
  opacity: 0.7;
}
</style>
