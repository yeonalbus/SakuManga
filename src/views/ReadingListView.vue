<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
// 🎯 阅读清单队列状态与操作方法（由 appStore 拆分而来）
import {
  onlineReadingList,
  offlineReadingList,
  clearReadingList,
  removeFromReadingList,
  moveInReadingList,
  addToReadingList,
} from '@/stores/readingStore'
import type { ComicItem } from '@/types/comic'
// Round11-Opt1：从书架快速导入（仅离线，增量式、按书架内顺序）
import { bookshelves, loadBookshelves } from '@/stores/bookshelfStore'
import { offlineComics, fetchOfflineComics } from '@/stores/comicStore'
// Round21：PC 桌面新标签打开阅读器
import { openContentTab } from '@/utils/detailNav'

const router = useRouter()
const { toast, modal } = useUI()

// 顶部分栏 Tab：'online' | 'offline'
const activeTab = ref<'online' | 'offline'>('online')

// 动态计算当前展示的列表，直接绑定全局 appStore
const currentList = computed(() =>
  activeTab.value === 'online' ? onlineReadingList.value : offlineReadingList.value,
)

// 触发跳转至连贯阅读器
const handleRead = (comic: ComicItem) => {
  toast.info(`即将开启连贯阅读：${comic.title}`)
  // bug3：以当前分栏（activeTab）强制确定 source，而不是依赖 comic.source。
  // 历史遗留的清单快照可能缺失/错误 source 字段，若照搬会造成在线 gid 被误判为离线模式。
  const src = activeTab.value
  const query: Record<string, string> = {
    id: comic.id,
    source: src,
  }
  // 在线模式必须携带 token，否则阅读器无法拉取 E 站页图（与 OnlineDetail 一致）
  if (src === 'online') {
    query.token = (comic as { token?: string }).token || ''
  }
  // Round21：PC 桌面新标签打开阅读器；PWA/窄屏或弹窗被拦截 → 降级同标签
  const href = router.resolve({ path: '/reader', query }).href
  openContentTab({ href, id: comic.id })
}

// 移出清单（Round10-Opt2：显式、幂等，只影响清单本身）
const handleRemove = (comic: ComicItem) => {
  removeFromReadingList(comic)
  toast.info(`已将《${comic.title}》移出清单`)
}

// 清空当前清单
const handleClearAll = async () => {
  const tabName = activeTab.value === 'online' ? '在线' : '本地'
  const confirmed = await modal.confirm(`确定要清空【${tabName}阅读清单】吗？`, '清空确认')

  if (confirmed) {
    clearReadingList(activeTab.value)
    toast.success(`${tabName}清单已清空`)
  }
}

// Round11-Opt1：从书架快速导入（仅离线 tab 显示入口）
const showShelfImport = ref(false)
const importingId = ref('')

const openShelfImport = async () => {
  // 确保离线漫画与书架数据已加载（直接进入本页时 offlineComics 可能尚未填充，
  // 否则导入会因映射为空而显示「本地库无匹配」导入 0 本）
  await Promise.all([fetchOfflineComics(), loadBookshelves()])
  showShelfImport.value = true
}

/**
 * 从指定书架增量导入到离线清单：
 * - 初始顺序 = 书架 comicIds 数组顺序（Round10 已保证即书架展示顺序）；
 * - 增量式：已在清单中的跳过，其余按顺序追加到清单末尾；
 * - 仅影响清单本身，不触碰书架/本地库。
 */
const importFromShelf = async (shelf: { id: string; name: string; comicIds?: string[] }) => {
  if (importingId.value) return
  importingId.value = shelf.id
  try {
    const ids = shelf.comicIds || []
    const byId = new Map(offlineComics.value.map((c) => [c.id, c]))
    const existing = new Set(offlineReadingList.value.map((c) => c.id))
    let added = 0
    let skipped = 0
    for (const cid of ids) {
      const comic = byId.get(cid)
      if (!comic) {
        skipped++
        continue
      }
      if (existing.has(cid)) {
        skipped++
        continue
      }
      addToReadingList(comic)
      existing.add(cid)
      added++
    }
    if (added > 0) {
      toast.success(`已从书架「${shelf.name}」导入 ${added} 本到本地清单（跳过 ${skipped} 本）`)
    } else {
      toast.info(`书架「${shelf.name}」无新增可导入（已在清单或本地库无匹配）`)
    }
    showShelfImport.value = false
  } finally {
    importingId.value = ''
  }
}
</script>

<template>
  <div class="reading-list-view">
    <!-- 页面标题 -->
    <div class="page-header">
      <h2 class="page-title">📑 候补阅读清单</h2>
      <span v-if="onlineReadingList.length + offlineReadingList.length > 0" class="total-badge">
        {{ onlineReadingList.length + offlineReadingList.length }} 本候补
      </span>
    </div>

    <!-- 在线/离线分栏 -->
    <div class="tabs-container">
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'online' }"
        @click="activeTab = 'online'"
      >
        🌐 在线清单 ({{ onlineReadingList.length }})
      </button>
      <button
        class="tab-btn"
        :class="{ active: activeTab === 'offline' }"
        @click="activeTab = 'offline'"
      >
        📚 本地清单 ({{ offlineReadingList.length }})
      </button>
    </div>

    <!-- 二级动作栏（Round11-Opt1：快速导入仅离线 tab 显示，在线取消） -->
    <div class="action-bar">
      <button v-if="activeTab === 'offline'" class="action-text-btn" @click="openShelfImport">
        ➕ 从书架导入
      </button>
      <button v-if="currentList.length > 0" class="action-text-btn danger" @click="handleClearAll">
        🗑️ 清空当前
      </button>
    </div>

    <!-- Round11-Opt1：书架选择弹层（离线清单从书架增量导入） -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="showShelfImport" class="shelf-import-mask" @click.self="showShelfImport = false">
          <div class="shelf-import-panel">
            <h3 class="import-title">📁 从书架导入</h3>
            <p class="import-hint">按书架内顺序增量追加到本地清单（已在清单中的自动跳过）</p>
            <div class="shelf-import-list">
              <div v-if="bookshelves.length === 0" class="import-empty">
                暂无书架，请先在「离线模式 → 书架」中创建
              </div>
              <button
                v-for="shelf in bookshelves"
                :key="shelf.id"
                class="shelf-import-item"
                :disabled="importingId === shelf.id"
                @click="importFromShelf(shelf)"
              >
                <span class="shelf-name">📁 {{ shelf.name }}</span>
                <span class="shelf-count">{{ shelf.count || 0 }} 本</span>
                <span class="import-action">{{ importingId === shelf.id ? '导入中…' : '导入 →' }}</span>
              </button>
            </div>
            <button class="import-close-btn" @click="showShelfImport = false">关闭</button>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- 列表主体 -->
    <div class="list-body">
      <div v-if="currentList.length === 0" class="empty-state">
        <span class="empty-icon">📭</span>
        <p>当前清单为空，快去添加点想看的本子吧！</p>
      </div>

      <div v-else class="mini-card-list">
        <div v-for="comic in currentList" :key="comic.id" class="mini-card">
          <div class="cover-box">
            <img :src="comic.coverUrl" :alt="comic.title" class="cover-img" />
          </div>

          <div class="info-box">
            <h4 class="title" :title="comic.title">{{ comic.title }}</h4>
            <span class="meta-pages">{{ comic.pageCount || 32 }} Pages</span>
          </div>

          <div class="card-actions">
            <!-- Round10-Opt1a：清单自定义排序（↑/↓） -->
            <button
              class="icon-btn move-btn"
              title="上移"
              :disabled="currentList[0]?.id === comic.id"
              @click="moveInReadingList(comic, -1)"
            >
              ↑
            </button>
            <button
              class="icon-btn move-btn"
              title="下移"
              :disabled="currentList[currentList.length - 1]?.id === comic.id"
              @click="moveInReadingList(comic, 1)"
            >
              ↓
            </button>
            <button class="icon-btn play-btn" title="立即阅读" @click="handleRead(comic)">▶</button>
            <button class="icon-btn remove-btn" title="移出清单" @click="handleRemove(comic)">
              ✕
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.reading-list-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 4px;
}

/* 页面标题 */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.page-title {
  font-size: 1.15rem;
  font-weight: bold;
  margin: 0;
  color: var(--app-text-strong);
}

.total-badge {
  background-color: #ef4444;
  color: #fff;
  font-size: 0.75rem;
  font-weight: bold;
  padding: 2px 10px;
  border-radius: 12px;
}

/* Tabs 分栏 */
.tabs-container {
  display: flex;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 10px;
  overflow: hidden;
}

.tab-btn {
  flex: 1;
  background: transparent;
  border: none;
  color: var(--app-text-3);
  padding: 10px 0;
  font-size: 0.9rem;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 0.2s;
}

.tab-btn.active {
  color: #007acc;
  border-bottom-color: #007acc;
  font-weight: bold;
  background: var(--app-surface-3);
}

/* 二级动作栏 */
.action-bar {
  display: flex;
  justify-content: space-between;
  padding: 8px 12px;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-2);
  border-radius: 10px;
}

.action-text-btn {
  background: transparent;
  border: none;
  color: var(--app-text-2);
  font-size: 0.8rem;
  cursor: pointer;
  transition: color 0.2s;
}
.action-text-btn:hover {
  color: var(--app-text-strong);
}
.action-text-btn.danger:hover {
  color: #ef4444;
}

/* 列表主体 */
.list-body {
  flex: 1;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 40vh;
  color: var(--app-text-muted);
  text-align: center;
  padding: 20px;
}
.empty-icon {
  font-size: 3rem;
  margin-bottom: 10px;
}

.mini-card-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/* 迷你卡片 UI */
.mini-card {
  display: flex;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 8px;
  gap: 12px;
  align-items: center;
  transition:
    border-color 0.2s,
    background-color 0.2s;
  position: relative;
}

.mini-card:hover {
  border-color: #007acc;
  background-color: var(--app-surface-3);
}

.cover-box {
  width: 50px;
  height: 70px;
  border-radius: 4px;
  overflow: hidden;
  background-color: var(--app-border-2);
  flex-shrink: 0;
}

.cover-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.info-box {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.title {
  font-size: 0.85rem;
  color: var(--app-text-strong);
  margin: 0 0 6px 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.meta-pages {
  font-size: 0.75rem;
  color: var(--app-text-3);
}

/* 悬浮动作按键 (默认隐藏，hover显示；移动端常显) */
.card-actions {
  display: flex;
  gap: 6px;
  opacity: 0;
  transition: opacity 0.2s;
}

.mini-card:hover .card-actions {
  opacity: 1;
}

.icon-btn {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.85rem;
}

.play-btn {
  background-color: rgba(0, 122, 204, 0.2);
  color: #007acc;
}
.play-btn:hover {
  background-color: #007acc;
  color: #fff;
}

.remove-btn {
  background-color: transparent;
  color: var(--app-text-muted);
}
.remove-btn:hover {
  background-color: rgba(239, 68, 68, 0.2);
  color: #ef4444;
}

.move-btn {
  background-color: transparent;
  color: var(--app-text-muted);
}
.move-btn:hover:not(:disabled) {
  background-color: rgba(16, 185, 129, 0.2);
  color: #10b981;
}
.move-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

/* Round11-Opt1：书架导入弹层 */
.shelf-import-mask {
  position: fixed;
  inset: 0;
  z-index: 9998;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(3px);
  display: flex;
  align-items: center;
  justify-content: center;
}

.shelf-import-panel {
  width: 90%;
  max-width: 420px;
  max-height: 75vh;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  border-radius: 12px;
  padding: 18px 20px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.import-title {
  margin: 0;
  font-size: 16px;
  color: var(--app-text-strong);
}

.import-hint {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-3);
}

.shelf-import-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  overflow-y: auto;
  max-height: 50vh;
}

.import-empty {
  padding: 20px 8px;
  text-align: center;
  color: var(--app-text-3);
  font-size: 13px;
}

.shelf-import-item {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
  border-radius: 8px;
  padding: 10px 12px;
  cursor: pointer;
  transition: all 0.15s;
  text-align: left;
}

.shelf-import-item:hover:not(:disabled) {
  border-color: #3d5afe;
  background: var(--app-surface-3-hover);
}

.shelf-import-item:disabled {
  opacity: 0.55;
  cursor: wait;
}

.shelf-import-item .shelf-name {
  flex: 1;
  font-size: 14px;
  color: var(--app-text-strong);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.shelf-import-item .shelf-count {
  font-size: 12px;
  color: var(--app-text-3);
  background: var(--app-surface-3);
  padding: 1px 8px;
  border-radius: 10px;
}

.shelf-import-item .import-action {
  font-size: 12px;
  color: #3d5afe;
  white-space: nowrap;
}

.import-close-btn {
  align-self: flex-end;
  background: var(--app-border-2);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 6px;
  padding: 6px 16px;
  font-size: 13px;
  cursor: pointer;
  transition: background-color 0.15s;
}

.import-close-btn:hover {
  background: var(--app-surface-3-hover);
  color: var(--app-text-strong);
}

/* 📱 移动形态（<1024px）：操作按钮常显（触摸屏无 hover），避免无法操作 */
@media (max-width: 1024px) {
  .card-actions {
    opacity: 1;
  }
}
</style>
