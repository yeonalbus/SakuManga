<template>
  <div class="about-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">版本</div>
        <div class="item-subtext">{{ version }}</div>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">创作者</div>
        <div class="item-subtext">珱垣</div>
      </div>
    </div>

    <div
      class="setting-item clickable"
      @click="handleOpenLink('https://github.com/yeonalbus/SakuHentai')"
    >
      <div class="item-info">
        <div class="item-title">Github</div>
        <div class="item-subtext link-text">https://github.com/yeonalbus/SakuHentai</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { version } from '../../../package.json'
import { useUI } from '@/composables/useUI'

const { toast } = useUI()

const handleOpenLink = (url: string) => {
  window.open(url, '_blank')
  toast.info(`已尝试在浏览器打开：${url}`)
}
</script>

<style scoped>
.about-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.setting-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background-color: var(--app-surface-2);
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
  transition: background-color 0.2s ease;
}

.setting-item.clickable {
  cursor: pointer;
}

.setting-item.clickable:hover {
  background-color: var(--app-surface-2-hover);
}

.item-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.item-title {
  font-size: 15px;
  font-weight: 500;
  color: var(--app-text-strong);
}

.item-desc {
  font-size: 12px;
  color: var(--app-text-3);
}

.item-subtext {
  font-size: 13px;
  color: var(--app-text-2);
  line-height: 1.4;
}

.link-text {
  color: #ff7588;
  word-break: break-all;
}
</style>
