<script setup lang="ts">
import { computed } from 'vue'
import ItemCard from '@/components/ItemCard.vue'
// 🟢 1. 从 comicStore 引入真实的全局离线漫画数据
import { offlineComics } from '@/stores/comicStore'
import type { OfflineComic } from '@/types/comic'

interface RankedOfflineComic extends OfflineComic {
  rank: number
}

// 🟢 2. 核心计算属性：根据阅读/点击次数（readCount）降序排列并生成 TOP 25
const rankedComics = computed<RankedOfflineComic[]>(() => {
  const sorted = [...offlineComics.value].sort((a, b) => {
    const countA = a.readCount || 0
    const countB = b.readCount || 0
    return countB - countA
  })

  // 截取前 25 名，并自动打上 1 ~ 25 名的 rank 标签
  return sorted.slice(0, 25).map((comic, index) => ({
    ...comic,
    rank: index + 1,
  }))
})

// 🟢 3. 动态派生前三名与剩余榜单
const topThree = computed(() => rankedComics.value.slice(0, 3))
const restItems = computed(() => rankedComics.value.slice(3, 25))

// 辅助展示函数：获取漫画的实际阅读/点击次数
const getComicReadCount = (item: OfflineComic) => {
  return item.readCount || 0
}
</script>

<template>
  <div class="leaderboard-page">
    <h2 class="page-title">📊 本地个人阅读频次榜 (TOP 25)</h2>

    <div class="podium-section">
      <div v-if="topThree[1]" class="podium-item rank-2-wrapper">
        <div class="podium-crown">🥈 NO.2</div>
        <ItemCard :comic="topThree[1]" :rank="2" size="large" mode="card" />
        <div class="read-count">{{ getComicReadCount(topThree[1]) }} 次阅读</div>
      </div>

      <div v-if="topThree[0]" class="podium-item rank-1-wrapper">
        <div class="podium-crown crown-gold">👑 NO.1</div>
        <ItemCard :comic="topThree[0]" :rank="1" size="large" mode="card" />
        <div class="read-count gold-text">{{ getComicReadCount(topThree[0]) }} 次阅读</div>
      </div>

      <div v-if="topThree[2]" class="podium-item rank-3-wrapper">
        <div class="podium-crown">🥉 NO.3</div>
        <ItemCard :comic="topThree[2]" :rank="3" size="large" mode="card" />
        <div class="read-count">{{ getComicReadCount(topThree[2]) }} 次阅读</div>
      </div>
    </div>

    <div v-if="restItems.length > 0" class="rest-section">
      <h3 class="section-subtitle">第 4 - 25 名</h3>
      <div class="card-grid">
        <div v-for="item in restItems" :key="item.id" class="grid-item-wrapper">
          <ItemCard :comic="item" :rank="item.rank" mode="card" />
          <div class="sub-read-count">{{ getComicReadCount(item) }} 次阅读</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.leaderboard-page {
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 20px;
  padding-bottom: 30px;
}

.page-title {
  font-size: 1.3rem;
  color: #fff;
  margin: 0;
}

.podium-section {
  display: flex;
  justify-content: center;
  align-items: flex-end;
  gap: 20px;
  padding: 20px 0;
  border-bottom: 1px solid var(--app-border-2);
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
  font-weight: 600;
  color: var(--app-text-2);
  margin-bottom: 8px;
}

.crown-gold {
  color: #ffd700;
  font-size: 1rem;
}

.read-count {
  margin-top: 8px;
  font-size: 0.85rem;
  color: var(--app-text-3);
}

.gold-text {
  color: #ffd700;
  font-weight: 600;
}

.rest-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-subtitle {
  font-size: 1.1rem;
  color: var(--app-text-2);
  margin: 0;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 16px;
}

.grid-item-wrapper {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.sub-read-count {
  margin-top: 6px;
  font-size: 0.78rem;
  color: var(--app-text-3);
}
</style>
