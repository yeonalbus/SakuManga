<script setup lang="ts">
/**
 * 画师/社团 Top 列表（Round32 阶段一）
 *
 * 画师/社团标签长尾分散、名字长短不一，做成词云排版杂乱，
 * 因此从词云体系剥离，以「Top 20 排行榜」形式单独呈现。
 */
import { computed } from 'vue'
import type { XpCloudArtist } from '@/types/comic'

const props = defineProps<{ artists: XpCloudArtist[] }>()
const emit = defineEmits<{ (e: 'select', artist: XpCloudArtist): void }>()

/** 进度条的基准（本数最多的画师占满） */
const maxCount = computed(() => Math.max(1, ...props.artists.map((a) => a.comicCount)))
</script>

<template>
  <div class="artist-top">
    <h3 class="artist-title">🎨 Top 画师 / 社团</h3>
    <div v-if="artists.length === 0" class="artist-empty">暂无画师/社团标签</div>
    <ol v-else class="artist-list">
      <li
        v-for="(artist, index) in artists"
        :key="`${artist.namespace}:${artist.key}`"
        class="artist-row"
        :title="`${artist.namespace}:${artist.key}`"
        @click="emit('select', artist)"
      >
        <span class="artist-rank" :class="{ top3: index < 3 }">{{ index + 1 }}</span>
        <div class="artist-main">
          <div class="artist-name">{{ artist.name }}</div>
          <div class="artist-bar">
            <span
              class="artist-bar-fill"
              :style="{ width: `${(artist.comicCount / maxCount) * 100}%` }"
            />
          </div>
        </div>
        <span class="artist-count">{{ artist.comicCount }} 本</span>
      </li>
    </ol>
  </div>
</template>

<style scoped>
.artist-top {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.artist-title {
  margin: 0;
  font-size: 0.95rem;
  color: var(--app-text-2);
}

.artist-empty {
  padding: 12px 0;
  font-size: 0.82rem;
  color: var(--app-text-3);
}

.artist-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 6px;
}

.artist-row {
  display: grid;
  grid-template-columns: 22px 1fr auto;
  align-items: center;
  gap: 8px;
  padding: 4px 6px;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.15s;
}

.artist-row:hover {
  background: var(--app-hover, rgba(255, 255, 255, 0.06));
}

.artist-rank {
  font-size: 0.75rem;
  color: var(--app-text-3);
  text-align: center;
}

.artist-rank.top3 {
  color: #ffd700;
  font-weight: 700;
}

.artist-main {
  min-width: 0;
}

.artist-name {
  font-size: 0.82rem;
  color: var(--app-text-strong);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.artist-bar {
  margin-top: 3px;
  height: 3px;
  border-radius: 2px;
  background: var(--app-border-2, rgba(255, 255, 255, 0.1));
  overflow: hidden;
}

.artist-bar-fill {
  display: block;
  height: 100%;
  border-radius: 2px;
  background: linear-gradient(90deg, #ff7b72, #ff8fab);
}

.artist-count {
  font-size: 0.75rem;
  color: var(--app-text-3);
  white-space: nowrap;
}
</style>
