<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
// 🎯 阅读清单队列状态与操作方法（由 appStore 拆分而来）
import {
  onlineReadingList,
  offlineReadingList,
  clearReadingList,
  toggleReadingList,
} from '@/stores/readingStore'
import type { ComicItem } from '@/types/comic'

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
  router.push({ path: '/reader', query })
}

// 移出清单
const handleRemove = (comic: ComicItem) => {
  toggleReadingList(comic)
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

// 预留的快捷导入占位
const handleQuickImport = async () => {
  const tabName = activeTab.value === 'online' ? '收藏夹' : '本地书架'
  toast.info(`调起【从${tabName}导入】面板... (待后端接入)`)
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

    <!-- 二级动作栏 -->
    <div class="action-bar">
      <button class="action-text-btn" @click="handleQuickImport">➕ 快捷导入</button>
      <button v-if="currentList.length > 0" class="action-text-btn danger" @click="handleClearAll">
        🗑️ 清空当前
      </button>
    </div>

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

/* 📱 移动形态（<1024px）：操作按钮常显（触摸屏无 hover），避免无法操作 */
@media (max-width: 1024px) {
  .card-actions {
    opacity: 1;
  }
}
</style>
