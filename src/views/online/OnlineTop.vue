<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import ItemCard from '@/components/ItemCard.vue'
import type { OnlineComic } from '@/types/comic'
import { useUI } from '@/composables/useUI'

interface RankedOnlineComic extends OnlineComic {
  score: number
  rank: number
}

const { toast } = useUI()
const allItems = ref<RankedOnlineComic[]>([])
const isLoading = ref(true)

// 从 Go 内存缓存读取数据
const fetchToplist = async () => {
  isLoading.value = true
  try {
    const res = await fetch('http://localhost:8080/api/v1/comics/online/toplist')
    const data = await res.json()

    if (res.ok) {
      allItems.value = data.comics || []
    } else {
      toast.error(data.error || '获取排行榜失败')
    }
  } catch (err) {
    toast.error('网络连接失败')
    console.error(err)
  } finally {
    isLoading.value = false
  }
}

const topThree = computed(() => allItems.value.slice(0, 3))
const restItems = computed(() => allItems.value.slice(3, 25))

onMounted(() => {
  fetchToplist()
})
</script>

<template>
  <div class="leaderboard-page">
    <h2 class="page-title">🏆 官方全站热度榜 (TOP 25)</h2>

    <div v-if="isLoading" class="loading-state">加载热度榜单中...</div>

    <template v-else-if="allItems.length > 0">
      <!-- 领奖台前 3 名 -->
      <div class="podium-section">
        <div v-if="topThree[1]" class="podium-item rank-2-wrapper">
          <div class="podium-crown">🥈 NO.2</div>
          <ItemCard :comic="topThree[1]" :rank="2" size="large" mode="card" />
          <div class="rank-score">{{ topThree[1].score }} 热度</div>
        </div>

        <div v-if="topThree[0]" class="podium-item rank-1-wrapper">
          <div class="podium-crown gold">👑 NO.1 CHAMPION</div>
          <ItemCard :comic="topThree[0]" :rank="1" size="top1" mode="card" />
          <div class="rank-score gold-text">{{ topThree[0].score }} 热度</div>
        </div>

        <div v-if="topThree[2]" class="podium-item rank-3-wrapper">
          <div class="podium-crown">🥉 NO.3</div>
          <ItemCard :comic="topThree[2]" :rank="3" size="large" mode="card" />
          <div class="rank-score">{{ topThree[2].score }} 热度</div>
        </div>
      </div>

      <!-- 第 4 - 25 名 -->
      <div class="rest-section">
        <h3 class="section-subtitle">第 4 - 25 名</h3>
        <div class="card-grid">
          <div v-for="item in restItems" :key="item.id" class="grid-item-wrapper">
            <ItemCard :comic="item" :rank="item.rank" mode="card" />
            <div class="sub-score">{{ item.score }} 热度</div>
          </div>
        </div>
      </div>
    </template>

    <div v-else class="empty-tip">暂无榜单数据</div>
  </div>
</template>

<style scoped>
.leaderboard-page {
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding-bottom: 30px;
}

.loading-state,
.empty-tip {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 240px;
  color: #888;
}

.page-title {
  font-size: 1.3rem;
  color: #fff;
}

.podium-section {
  display: flex;
  justify-content: center;
  align-items: flex-end;
  gap: 20px;
  padding: 20px 0;
  border-bottom: 1px solid #2a2a2a;
}

.podium-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 170px;
}

.rank-1-wrapper {
  width: 210px;
  transform: translateY(-10px);
}

.podium-crown {
  font-size: 0.85rem;
  font-weight: bold;
  color: #aaa;
  margin-bottom: 8px;
}

.podium-crown.gold {
  color: #ffd700;
  font-size: 1rem;
}

.rank-score {
  margin-top: 8px;
  font-size: 0.85rem;
  color: #888;
}

.gold-text {
  color: #ffd700;
  font-weight: bold;
}

.section-subtitle {
  font-size: 0.95rem;
  color: #888;
  margin-bottom: 12px;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 16px;
}

.sub-score {
  font-size: 0.75rem;
  color: #666;
  text-align: right;
  margin-top: 4px;
}
</style>
