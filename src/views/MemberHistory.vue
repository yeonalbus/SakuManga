<script setup lang="ts">
/**
 * 成员历史（仅管理员）：查看任意成员的阅读历史
 * 调后端 GET /admin/history?userId=&source=&limit=
 */
import { ref, computed, watch, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUserStore } from '@/stores/userStore'
import { http } from '@/utils/request'
import GridContainer from '@/components/GridContainer.vue'
import Pagination from '@/components/Pagination.vue'
import type { ComicItem } from '@/types/comic'

interface AdminHistoryItem {
  id: number
  userId?: number
  comicId: string
  source: 'online' | 'offline'
  comicTitle: string
  coverUrl: string
  lastChapterTitle?: string
  lastPageIndex: number
  totalPageCount: number
  lastReadAt: string
}
interface AdminHistoryResp {
  items: AdminHistoryItem[]
  total: number
  users?: Record<string, string>
}
interface MemberBrief {
  id: number
  username: string
  role: string
}

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const members = ref<MemberBrief[]>([])
const selectedUserId = ref<number | ''>('')
const source = ref<'online' | 'offline'>('online')
const records = ref<AdminHistoryItem[]>([])
const loading = ref(false)
const currentPage = ref(1)
const pageSize = 24

const totalPages = computed(() => Math.ceil(records.value.length / pageSize) || 1)
const currentPageItems = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return records.value.slice(start, start + pageSize)
})

const loadMembers = async () => {
  try {
    const data = await http<{ users: MemberBrief[] }>('/users')
    members.value = data.users || []
  } catch (e) {
    console.error('加载成员失败:', e)
  }
}

/** 管理员记录 → 漫画卡片（GridContainer 展示用） */
const displayItems = computed<ComicItem[]>(() =>
  currentPageItems.value.map(
    (r) =>
      ({
        id: r.comicId,
        title: r.comicTitle,
        coverUrl: r.coverUrl,
        source: r.source,
        pageCount: r.totalPageCount || undefined,
      }) as ComicItem,
  ),
)

const loadHistory = async () => {
  loading.value = true
  try {
    const params: Record<string, string | number> = { source: source.value, limit: 500 }
    if (selectedUserId.value !== '') params.userId = selectedUserId.value
    const data = await http<AdminHistoryResp>('/admin/history', { params })
    records.value = data.items || []
  } catch (e) {
    records.value = []
    console.error('加载成员历史失败:', e)
  } finally {
    loading.value = false
  }
}

watch([selectedUserId, source], () => {
  currentPage.value = 1
  loadHistory()
})

const handlePageChange = (page: number) => {
  currentPage.value = page
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(() => {
  // 等待当前用户信息加载完成后再校验管理员身份
  const stop = watch(
    () => userStore.user,
    (u) => {
      if (!u) return
      stop()
      if (!userStore.isAdmin) {
        router.replace('/settings')
        return
      }
      const initUserId = route.query.userId
      if (initUserId && !Array.isArray(initUserId)) {
        const uid = Number(initUserId)
        if (Number.isFinite(uid)) selectedUserId.value = uid
      }
      loadMembers().then(() => loadHistory())
    },
    { immediate: true },
  )
})
</script>

<template>
  <div class="member-history-view">
    <div class="view-header">
      <div class="header-left">
        <button class="back-btn" @click="router.back()">‹ 返回</button>
        <h2 class="view-title">📜 成员历史</h2>
      </div>

      <div class="filters">
        <select v-model="selectedUserId" class="filter-select">
          <option value="">全部成员</option>
          <option v-for="m in members" :key="m.id" :value="m.id">
            {{ m.username }}{{ m.role === 'admin' ? '（管理员）' : '' }}
          </option>
        </select>

        <select v-model="source" class="filter-select">
          <option value="online">🌐 在线</option>
          <option value="offline">📚 本地</option>
        </select>
      </div>
    </div>

    <div class="stat-bar">
      <span class="stat-total">共 {{ records.length }} 条记录</span>
    </div>

    <div v-if="loading" class="loading-tip">加载中…</div>

    <GridContainer v-else-if="records.length > 0" :items="displayItems">
      <template #footer>
        <Pagination
          v-if="totalPages >= 1"
          :current-page="currentPage"
          :total-pages="totalPages"
          @change="handlePageChange"
        />
      </template>
    </GridContainer>

    <div v-else class="empty-tip">该范围内暂无阅读历史</div>
  </div>
</template>

<style scoped>
.member-history-view {
  padding: 20px;
  min-height: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.view-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid #2a2a2e;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.back-btn {
  background-color: #2a2a2e;
  border: 1px solid #3d3d42;
  color: #ccc;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
}

.back-btn:hover {
  background-color: #3d3d42;
  color: #fff;
}

.view-title {
  font-size: 1.2rem;
  font-weight: 600;
  color: #fff;
  margin: 0;
}

.filters {
  display: flex;
  gap: 8px;
}

.filter-select {
  background-color: #2a2a2e;
  border: 1px solid #3d3d42;
  color: #ccc;
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 0.85rem;
  cursor: pointer;
}

.stat-bar {
  font-size: 0.82rem;
  color: #888;
}

.stat-total {
  color: #aaa;
}

.loading-tip {
  margin-top: 60px;
  text-align: center;
  color: #66666c;
}

.empty-tip {
  margin-top: 60px;
  text-align: center;
  color: #66666c;
  font-size: 0.95rem;
}
</style>
