<template>
  <div class="about-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">版本</div>
        <div class="item-subtext">8.0.10+305</div>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">创作者</div>
        <div class="item-subtext">酱天小禽兽(JTMonster)</div>
      </div>
    </div>

    <div
      class="setting-item clickable"
      @click="handleOpenLink('https://github.com/jiangtian616/JHentai')"
    >
      <div class="item-info">
        <div class="item-title">Github</div>
        <div class="item-subtext link-text">https://github.com/jiangtian616/JHentai</div>
      </div>
    </div>

    <div class="setting-item clickable" @click="handleOpenLink('https://t.me/+PindoE9yvlpmOWI9')">
      <div class="item-info">
        <div class="item-title">Telegram(Chinese Mainly)</div>
        <div class="item-desc">账号登录、里站、网络等基础问题请自行搜索解决</div>
        <div class="item-subtext link-text">https://t.me/+PindoE9yvlpmOWI9</div>
      </div>
    </div>

    <div
      class="setting-item clickable"
      @click="handleOpenLink('https://github.com/jiangtian616/JHentai/wiki')"
    >
      <div class="item-info">
        <div class="item-title">常见问题</div>
        <div class="item-subtext link-text">https://github.com/jiangtian616/JHentai/wiki</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
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
  background-color: #1a1a1e;
  border-radius: 8px;
  border: 1px solid #26262a;
  transition: background-color 0.2s ease;
}

.setting-item.clickable {
  cursor: pointer;
}

.setting-item.clickable:hover {
  background-color: #222226;
}

.item-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.item-title {
  font-size: 15px;
  font-weight: 500;
  color: #ffffff;
}

.item-desc {
  font-size: 12px;
  color: #88888c;
}

.item-subtext {
  font-size: 13px;
  color: #a0a0a5;
  line-height: 1.4;
}

.link-text {
  color: #ff7588;
  word-break: break-all;
}
</style>
