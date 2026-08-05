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

// 控制右侧抽屉显隐
const isOpen = ref(false)

// 顶部分栏 Tab：'online' | 'offline'
const activeTab = ref<'online' | 'offline'>('online')

// 动态计算当前展示的列表，直接绑定全局 appStore
const currentList = computed(() =>
  activeTab.value === 'online' ? onlineReadingList.value : offlineReadingList.value,
)

// 触发跳转至连贯阅读器
const handleRead = (comic: ComicItem) => {
  toast.info(`即将开启连贯阅读：${comic.title}`)
  isOpen.value = false
  const query: Record<string, string> = {
    id: comic.id,
    source: comic.source,
  }
  // 在线模式必须携带 token，否则阅读器无法拉取 E 站页图（与 OnlineDetail 一致）
  if (comic.source === 'online') {
    query.token = comic.token || ''
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
  <div class="reading-list-container">
    <button class="trigger-btn" @click="isOpen = true">
      <span class="icon">📑</span>
      <span class="text">阅读清单</span>
      <span class="badge" v-if="onlineReadingList.length + offlineReadingList.length > 0">
        {{ onlineReadingList.length + offlineReadingList.length }}
      </span>
    </button>

    <Teleport to="body">
      <Transition name="fade">
        <div v-if="isOpen" class="drawer-backdrop" @click="isOpen = false"></div>
      </Transition>

      <Transition name="slide-right">
        <div v-if="isOpen" class="right-drawer">
          <div class="drawer-header">
            <h2 class="drawer-title">📑 候补阅读清单</h2>
            <button class="close-btn" @click="isOpen = false">✕</button>
          </div>

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

          <div class="action-bar">
            <button class="action-text-btn" @click="handleQuickImport">➕ 快捷导入</button>
            <button
              v-if="currentList.length > 0"
              class="action-text-btn danger"
              @click="handleClearAll"
            >
              🗑️ 清空当前
            </button>
          </div>

          <div class="drawer-body">
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
                  <button class="icon-btn play-btn" title="立即阅读" @click="handleRead(comic)">
                    ▶
                  </button>
                  <button class="icon-btn remove-btn" title="移出清单" @click="handleRemove(comic)">
                    ✕
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
/* 顶栏触发器 */
.trigger-btn {
  position: relative;
  display: flex;
  align-items: center;
  gap: 6px;
  background-color: transparent;
  border: 1px solid transparent;
  color: #ccc;
  padding: 6px 10px;
  border-radius: 8px;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
}

.trigger-btn:hover {
  background-color: #242428;
  border-color: #3a3a3d;
  color: #fff;
}

.badge {
  background-color: #ef4444;
  color: #fff;
  font-size: 0.7rem;
  font-weight: bold;
  padding: 1px 6px;
  border-radius: 10px;
  margin-left: 2px;
}

/* 遮罩 */
.drawer-backdrop {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  z-index: 2000;
  backdrop-filter: blur(2px);
}

/* 右侧抽屉 */
.right-drawer {
  position: fixed;
  top: 0;
  right: 0;
  width: 360px;
  height: 100vh;
  background-color: #161619;
  border-left: 1px solid #2a2a2d;
  box-shadow: -10px 0 30px rgba(0, 0, 0, 0.7);
  z-index: 2001;
  display: flex;
  flex-direction: column;
  color: #e0e0e0;
}

.drawer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
}

.drawer-title {
  font-size: 1.1rem;
  font-weight: bold;
  margin: 0;
  color: #fff;
}

.close-btn {
  background: transparent;
  border: none;
  font-size: 1.1rem;
  color: #888;
  cursor: pointer;
}
.close-btn:hover {
  color: #fff;
}

/* Tabs 分栏 */
.tabs-container {
  display: flex;
  border-bottom: 1px solid #2a2a2d;
  padding: 0 10px;
}

.tab-btn {
  flex: 1;
  background: transparent;
  border: none;
  color: #888;
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
}

/* 二级动作栏 */
.action-bar {
  display: flex;
  justify-content: space-between;
  padding: 8px 16px;
  background: #1e1e22;
  border-bottom: 1px solid #2a2a2d;
}

.action-text-btn {
  background: transparent;
  border: none;
  color: #aaa;
  font-size: 0.8rem;
  cursor: pointer;
  transition: color 0.2s;
}
.action-text-btn:hover {
  color: #fff;
}
.action-text-btn.danger:hover {
  color: #ef4444;
}

/* 主体列表区 */
.drawer-body {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #666;
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
  background-color: #1a1a1d;
  border: 1px solid #2a2a2d;
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
  background-color: #242428;
}

.cover-box {
  width: 50px;
  height: 70px;
  border-radius: 4px;
  overflow: hidden;
  background-color: #26262a;
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
  color: #fff;
  margin: 0 0 6px 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.meta-pages {
  font-size: 0.75rem;
  color: #888;
}

/* 悬浮动作按键 (默认隐藏，hover显示) */
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
  color: #666;
}
.remove-btn:hover {
  background-color: rgba(239, 68, 68, 0.2);
  color: #ef4444;
}

/* 右侧滑入动画 */
.slide-right-enter-active,
.slide-right-leave-active {
  transition: transform 0.3s ease;
}
.slide-right-enter-from,
.slide-right-leave-to {
  transform: translateX(100%);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
