<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import AppIcon from '@/components/icons/AppIcon.vue'

// ─────────────────────────────────────────────────────────────
// Round44：忽略清单独立页（从维护页弹层剥离）
// Round51：忽略语义改为「按组成员」——
//   - 忽略 = 「这一组（维护页上的那个簇）」，条目展示忽略时记录的那批本子；
//   - 出现「忽略之后新入库」的成员时，维护页会重新列出该组并允许「确认新增」；
//   - 旧逻辑条目已由后端在升级时清空（破坏性更新），本页一次性提示用户重新忽略。
// ─────────────────────────────────────────────────────────────

const { modal, toast } = useUI()

interface OfflineComicDTO {
  id: string
  title: string
  coverUrl: string
  pageCount?: number
  localPath?: string
  fileSize?: number
  onlineTags?: string
}

interface IgnoreMemberDTO {
  comic: OfflineComicDTO
  pageCount: number
  lang?: string
  artist?: string
  grouped: boolean
}

interface IgnoreItemDTO {
  id: string
  type: 'title' | 'gid'
  titleKey?: string
  artist?: string
  gid?: string
  note?: string
  createdAt: string
  members?: IgnoreMemberDTO[]
  matchedCount: number
  groupCount: number
  groupedCount: number
  newCount: number
}

// Round51：忽略逻辑升级（破坏性更新）一次性提示（不随刷新消失，点「知道了」后不再显示）
const UPGRADE_NOTICE_KEY = 'sakumanga.ignoreUpgradeNoticeDismissed'
const showUpgradeNotice = ref(localStorage.getItem(UPGRADE_NOTICE_KEY) !== '1')
const dismissUpgradeNotice = () => {
  showUpgradeNotice.value = false
  localStorage.setItem(UPGRADE_NOTICE_KEY, '1')
}

const items = ref<IgnoreItemDTO[]>([])
const loading = ref(true)
const openIds = ref<Record<string, boolean>>({})
const coverFailed = ref<Record<string, boolean>>({})
const busyId = ref('')

const titleItems = computed(() => items.value.filter((i) => i.type === 'title'))
const gidItems = computed(() => items.value.filter((i) => i.type === 'gid'))
const newItems = computed(() => items.value.filter((i) => i.newCount > 0))
const newMemberTotal = computed(() => newItems.value.reduce((s, i) => s + i.newCount, 0))
// 已不构成重复：组成员的现存成员 < 2（含全部被删除 → 0 本）
const staleItems = computed(() => items.value.filter((i) => i.matchedCount <= 1))

const load = async () => {
  loading.value = true
  try {
    const data = await http<{ items: IgnoreItemDTO[] }>('/offline/ignore/list')
    items.value = data?.items || []
    // 默认展开有新增的条目，便于直接确认
    const open: Record<string, boolean> = {}
    for (const it of items.value) {
      if (it.newCount > 0) open[it.id] = true
    }
    openIds.value = open
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '读取忽略清单失败')
  } finally {
    loading.value = false
  }
}

const toggle = (id: string) => {
  openIds.value[id] = !openIds.value[id]
}
const isOpen = (id: string) => !!openIds.value[id]

// 确认新增：把当前匹配集合写入成员快照，之后继续静默
const ackNew = async (item: IgnoreItemDTO) => {
  busyId.value = item.id
  try {
    await http(`/offline/ignore/${item.id}/ack`, { method: 'POST' })
    toast.success('已确认新增 —— 快照更新为当前成员，之后继续静默')
    await load()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '确认失败')
  } finally {
    busyId.value = ''
  }
}

// 恢复忽略：该作品重新参与疑似重复判定
const restore = async (item: IgnoreItemDTO) => {
  const name = item.titleKey || item.gid || '该条目'
  const confirmed = await modal.confirm(
    `恢复后「${name}」会重新参与疑似重复判定，下次查重（或立即）回到维护页列表。\n\n确定恢复吗？`,
    '↩ 恢复忽略',
  )
  if (!confirmed) return
  busyId.value = item.id
  try {
    await http(`/offline/ignore/${item.id}/restore`, { method: 'POST' })
    toast.success('已恢复，将在下次重新扫描后出现在维护列表')
    await load()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '恢复失败')
  } finally {
    busyId.value = ''
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
  return d.toLocaleDateString('zh-CN')
}
const onCoverError = (id: string) => {
  coverFailed.value[id] = true
}

const typeLabel = (t: IgnoreItemDTO['type']) => (t === 'title' ? '忽略组' : '父画廊')

onMounted(load)
</script>

<template>
  <div class="ignore-page">
    <div class="page-header">
      <div>
        <h2 class="page-title"><AppIcon name="eyeClosed" :size="18" /> 忽略清单</h2>
        <p class="subtitle">
          共 {{ items.length }} 条 · 忽略＝不再提示，随时可恢复；点开每条可看到它实际匹配到哪些本子
        </p>
      </div>
      <div class="header-actions">
        <button class="scan-btn ghost" :disabled="loading" @click="load">
          <AppIcon name="refresh" :size="15" /> 刷新
        </button>
        <router-link class="scan-btn ghost" to="/offline/maintain">
          <AppIcon name="arrowLeft" :size="14" /> 返回维护页
        </router-link>
      </div>
    </div>

    <!-- Round51：忽略逻辑升级（破坏性更新）一次性提示 -->
    <div v-if="showUpgradeNotice" class="alert-bar upgrade-bar">
      <span class="alert-icon"><AppIcon name="eyeClosed" :size="18" /></span>
      <div class="alert-text">
        忽略逻辑已升级为「按组忽略」，旧忽略记录已重置 —— 请在维护页重新忽略需要的组。重新忽略时记录的是整组本子，新入库的本子会单独提醒。
      </div>
      <button class="action-btn ghost-gray sm" @click="dismissUpgradeNotice">知道了</button>
    </div>

    <!-- 忽略后新增提醒 -->
    <div v-if="newItems.length > 0" class="alert-bar">
      <span class="alert-icon"><AppIcon name="eyeClosed" :size="18" /></span>
      <div class="alert-text">
        <b>{{ newItems.length }}</b> 条忽略项下有忽略之后新入库的本子（共
        <b>{{ newMemberTotal }}</b> 本）—— 确认后继续静默；不确认就会一直在维护页顶部提醒。
      </div>
    </div>

    <!-- 统计 -->
    <div class="stat-bar">
      <span class="stat-item">{{ items.length }} 条忽略</span>
      <span v-if="staleItems.length > 0" class="stat-item warn"
        >{{ staleItems.length }} 条已不构成重复（可清理）</span
      >
      <span class="spacer"></span>
      <span class="chip">忽略组（title 型） {{ titleItems.length }}</span>
      <span class="chip">父画廊（gid 型） {{ gidItems.length }}</span>
    </div>

    <div v-if="loading" class="empty-box">
      <span class="spinner"></span>
      <p class="empty-sub">加载中...</p>
    </div>

    <div v-else-if="items.length === 0" class="empty-box">
      <span class="icon"><AppIcon name="eyeClosed" :size="36" /></span>
      <p class="empty-title">暂无忽略条目</p>
      <p class="empty-sub">在维护页对「两本都留」的疑似重复组点「都留（不再提示）」后，会出现在这里。</p>
    </div>

    <div v-else class="ignore-list">
      <div
        v-for="item in items"
        :key="item.id"
        class="ignore-row"
        :class="{
          stale: item.matchedCount <= 1,
          hasnew: item.newCount > 0,
        }"
      >
        <div class="ignore-head" @click="toggle(item.id)">
          <span class="type-chip">{{ typeLabel(item.type) }}</span>
          <span class="ignore-key" :title="item.titleKey || `gid ${item.gid}`">
            {{ item.titleKey || `gid ${item.gid}` }}
          </span>
          <span class="ignore-sub">
            <template v-if="item.artist">artist: {{ item.artist }} · </template>
            忽略于 {{ formatDate(item.createdAt) }}
          </span>
          <span class="spacer"></span>
          <span v-if="item.newCount > 0" class="flag new">忽略后新增 {{ item.newCount }} 本</span>
          <span v-else-if="item.matchedCount === 0" class="flag stale"
            >组成员已被删除 · 可清理</span
          >
          <span v-else-if="item.matchedCount === 1" class="flag stale"
            >该忽略组当前已不构成重复</span
          >
          <span v-else class="flag ok"
            >覆盖 {{ item.matchedCount }} 本 · 压制 {{ item.groupCount }} 组</span
          >
          <span class="expand-hint">
            <AppIcon :name="isOpen(item.id) ? 'caretUp' : 'caretDown'" :size="12" />
            {{ isOpen(item.id) ? '收起' : '点开看成员' }}
          </span>
        </div>

        <div v-show="isOpen(item.id)" class="ignore-body">
          <template v-if="(item.members || []).length > 0">
            <div class="diffline">
              忽略时记录了 <b>{{ item.matchedCount }}</b> 本：标记「同组」的
              {{ item.groupedCount }} 本之间构成重复；其余本子只是同组内的其他成员（当前不构成重复）。
            </div>
            <div class="members">
              <div v-for="m in item.members" :key="m.comic.id" class="member">
                <div class="member-cover">
                  <img
                    v-if="m.comic.coverUrl && !coverFailed[m.comic.id]"
                    :src="m.comic.coverUrl"
                    :alt="m.comic.title"
                    loading="lazy"
                    @error="onCoverError(m.comic.id)"
                  />
                  <span v-else class="cover-fallback"><AppIcon name="eyeClosed" :size="20" /></span>
                </div>
                <div class="member-main">
                  <div class="member-title" :title="m.comic.title">{{ m.comic.title }}</div>
                  <div class="member-meta">
                    <span class="lang-chip" :class="{ 'is-null': !m.lang }">{{ m.lang || 'null' }}</span>
                    <span>{{ m.pageCount || 0 }} 页</span>
                    <span
                      ><AppIcon name="local" :size="12" /> {{ formatBytes(m.comic.fileSize) }}</span
                    >
                    <span class="member-flag" :class="{ grouped: m.grouped }">
                      {{ m.grouped ? '同组（会构成重复）' : '未构成重复组' }}
                    </span>
                  </div>
                  <div class="member-path">
                    <AppIcon name="folder" :size="12" /> {{ m.comic.localPath || '—' }}
                  </div>
                </div>
              </div>
            </div>
          </template>
          <div v-else class="diffline warn">
            这条忽略当前匹配不到任何本地本子（本子已被删除或改名）—— 可以清理掉，避免清单里堆积失效条目。
          </div>

          <div class="row-actions">
            <button
              v-if="item.newCount > 0"
              class="action-btn ok"
              :disabled="busyId === item.id"
              title="把新入库的本子纳入「已知」，之后继续静默"
              @click="ackNew(item)"
            >
              <AppIcon name="check" :size="13" /> 确认新增（更新快照）
            </button>
            <button class="action-btn ghost-gray" :disabled="busyId === item.id" @click="restore(item)">
              <AppIcon name="arrowLeft" :size="13" /> 恢复忽略
            </button>
            <span class="spacer"></span>
            <span class="tip">
              恢复 = 重新参与疑似重复判定；确认新增 = 新本子纳入「已知」后继续静默
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ignore-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border-2);
}
.page-title {
  margin: 0;
  font-size: 1.3rem;
  color: var(--app-text-strong);
}
.subtitle {
  margin: 4px 0 0;
  font-size: 0.8rem;
  color: var(--app-text-3);
}
.header-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.scan-btn {
  border: 1px solid transparent;
  border-radius: 6px;
  padding: 7px 14px;
  font-size: 0.82rem;
  font-family: inherit;
  cursor: pointer;
  background: #007acc;
  color: #fff;
  text-decoration: none;
}
.scan-btn.ghost {
  background: transparent;
  border-color: var(--app-border-3);
  color: var(--app-text-2);
}
.scan-btn.ghost:hover {
  border-color: #007acc;
  color: #7ec8ff;
}
.scan-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.alert-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  background: #2a2414;
  border: 1px solid rgba(245, 158, 11, 0.4);
  border-left: 3px solid #f59e0b;
  border-radius: 6px;
  padding: 10px 14px;
  font-size: 0.82rem;
  color: #f5d08a;
}
.alert-text b {
  color: #ffd98a;
}
.upgrade-bar {
  border-left-color: #007acc;
  background: #14283a;
  border-color: #24425c;
  color: #a9c8dd;
}
.upgrade-bar .alert-text {
  flex: 1;
}
.upgrade-bar .action-btn {
  flex: 0 0 auto;
}

.stat-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  padding: 10px 14px;
  font-size: 0.82rem;
  color: var(--app-text-2);
}
.stat-bar .spacer {
  flex: 1;
}
.stat-item.warn {
  color: #f5d08a;
}
.chip {
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  border-radius: 999px;
  padding: 2px 10px;
  font-size: 0.74rem;
  color: var(--app-text-2);
}

.ignore-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.ignore-row {
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-left: 3px solid var(--app-border-3);
  border-radius: 8px;
  overflow: hidden;
}
.ignore-row.hasnew {
  border-left-color: #f59e0b;
}
.ignore-row.stale {
  border-left-color: var(--app-text-muted);
}
.ignore-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  cursor: pointer;
  flex-wrap: wrap;
}
.ignore-head:hover {
  background: var(--app-surface-2);
}
.type-chip {
  font-size: 0.7rem;
  color: var(--app-text-3);
  border: 1px solid var(--app-border-3);
  border-radius: 999px;
  padding: 1px 8px;
  flex: 0 0 auto;
}
.ignore-key {
  font-size: 0.88rem;
  color: var(--app-text-strong);
  max-width: 52ch;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ignore-sub {
  font-size: 0.74rem;
  color: var(--app-text-muted);
}
.ignore-head .spacer {
  flex: 1;
}
.expand-hint {
  font-size: 0.72rem;
  color: var(--app-text-muted);
}
.flag {
  border-radius: 999px;
  font-size: 0.72rem;
  padding: 1px 9px;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-3);
  white-space: nowrap;
}
.flag.new {
  color: #ffd98a;
  border-color: #4a3c14;
  background: #2a2414;
}
.flag.ok {
  color: #00c2a8;
  border-color: #1d5148;
  background: #0f2622;
}

.ignore-body {
  padding: 0 14px 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.diffline {
  background: #14283a;
  border: 1px solid #24425c;
  border-radius: 6px;
  padding: 8px 12px;
  font-size: 0.78rem;
  color: #a9c8dd;
}
.diffline b {
  color: #7ec8ff;
}
.diffline.warn {
  background: #2a2414;
  border-color: #4a3c14;
  color: #f0d9a8;
}

.members {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.member {
  flex: 1 1 300px;
  min-width: 0;
  display: flex;
  gap: 10px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  padding: 10px;
}
.member-cover {
  width: 54px;
  height: 72px;
  border-radius: 6px;
  flex: 0 0 auto;
  overflow: hidden;
  background: var(--app-surface-4);
  display: flex;
  align-items: center;
  justify-content: center;
}
.member-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.cover-fallback {
  font-size: 1.2rem;
  color: var(--app-text-3);
}
.member-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.member-title {
  font-size: 0.82rem;
  color: var(--app-text-strong);
  line-height: 1.35;
}
.member-meta {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 0.72rem;
  color: var(--app-text-3);
}
.lang-chip {
  border: 1px dashed var(--app-border-3);
  border-radius: 6px;
  padding: 0 6px;
  max-width: 96px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lang-chip.is-null {
  color: var(--app-text-muted);
}
.member-flag {
  font-size: 0.68rem;
  border-radius: 6px;
  padding: 0 6px;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-3);
}
.member-flag.grouped {
  color: #00c2a8;
  border-color: #1d5148;
}
.member-flag.new {
  color: #ffd98a;
  border-color: #4a3c14;
  background: #2a2414;
}
.member-path {
  font-size: 0.7rem;
  color: var(--app-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.row-actions .spacer {
  flex: 1;
}
.tip {
  font-size: 0.72rem;
  color: var(--app-text-muted);
}
.action-btn {
  border: none;
  border-radius: 6px;
  padding: 6px 12px;
  font-size: 0.78rem;
  font-family: inherit;
  cursor: pointer;
  background: var(--app-surface-3);
  color: var(--app-fg);
}
.action-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.action-btn.ok {
  background: #00c2a8;
  color: #06231f;
  font-weight: 600;
}
.action-btn.ghost-gray {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
}
.action-btn.ghost-gray:hover {
  border-color: var(--app-text-3);
  color: var(--app-text-strong);
}

.empty-box {
  text-align: center;
  padding: 56px 20px;
  color: var(--app-text-3);
}
.empty-box .icon {
  font-size: 2.4rem;
}
.empty-title {
  margin: 12px 0 6px;
  color: var(--app-text-strong);
  font-size: 1.05rem;
}
.empty-sub {
  margin: 0;
  font-size: 0.8rem;
}
.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.25);
  border-top-color: #007acc;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  display: inline-block;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 720px) {
  .ignore-page {
    padding: 12px;
  }
  .page-header {
    flex-direction: column;
    align-items: stretch;
  }
  .header-actions {
    justify-content: flex-end;
  }
  .ignore-key {
    max-width: 100%;
    white-space: normal;
  }
  .ignore-sub {
    display: none;
  }
  .members {
    flex-direction: column;
  }
  .member {
    flex: 1 1 auto;
    flex-wrap: wrap;
  }
  .member-main {
    min-width: 140px;
  }
  .tip {
    display: none;
  }
}
</style>
