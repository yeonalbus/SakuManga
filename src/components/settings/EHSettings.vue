<template>
  <div class="eh-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">站点</div>
      </div>
      <div class="site-segmented">
        <button
          class="segment-btn"
          :class="{ active: currentSite === 'e-hentai' }"
          @click="currentSite = 'e-hentai'"
        >
          E-Hentai
        </button>
        <button
          class="segment-btn"
          :class="{ active: currentSite === 'exhentai' }"
          @click="currentSite = 'exhentai'"
        >
          EXHentai
        </button>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">优先重定向至表站</div>
        <div class="item-subtext">
          优先尝试从表站加载画廊详情页，以获得更好的网络体验，非必要不用关闭
        </div>
      </div>
      <div class="switch-control">
        <label class="toggle-switch">
          <input type="checkbox" v-model="preferRedirect" />
          <span class="slider"></span>
        </label>
      </div>
    </div>

    <div class="setting-item clickable" @click="handleOpenProfile">
      <div class="item-info">
        <div class="item-title">Profile设置</div>
        <div class="item-subtext">选择在 SakuHentai 中使用的 Profile</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item clickable" @click="handleOpenSiteSettings">
      <div class="item-info">
        <div class="item-title">站点设置</div>
        <div class="item-subtext">更改 E 站个人设置</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item clickable" @click="handleQuotaDetail">
      <div class="item-info">
        <div class="item-title">图片配额</div>
        <div class="item-subtext">长按重置，花费 0 GP</div>
      </div>
      <div class="item-right-content">
        <span class="quota-text">{{ currentQuota }} / {{ maxQuota }}</span>
        <span class="arrow-icon">›</span>
      </div>
    </div>

    <div class="setting-item clickable" @click="handleAssetsDetail">
      <div class="item-info">
        <div class="item-title">资产</div>
        <div class="item-subtext">
          GP: {{ assetGP }} k &nbsp;&nbsp;&nbsp; Credits: {{ assetCredits }}
        </div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item clickable" @click="handleOpenMyTags">
      <div class="item-info">
        <div class="item-title">我的标签</div>
        <div class="item-subtext">管理关注和隐藏的标签</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useUI } from '@/composables/useUI'

const { toast } = useUI()

// 响应式数据
const currentSite = ref<'e-hentai' | 'exhentai'>('exhentai')
const preferRedirect = ref(false)

// 模拟数据 (配额与资产)
const currentQuota = ref(0)
const maxQuota = ref(50000)
const assetGP = ref('51,667')
const assetCredits = ref('29,343')

// 页面交互处理 (后续对接网络/抽屉层)
const handleOpenProfile = () => {
  toast.info('打开 Profile 设置抽屉')
}

const handleOpenSiteSettings = () => {
  toast.info('打开 E 站 Web 页面个人设置')
}

const handleQuotaDetail = () => {
  toast.info('点击图片配额详情')
}

const handleAssetsDetail = () => {
  toast.info('打开资产明细')
}

const handleOpenMyTags = () => {
  toast.info('打开我的标签管理')
}
</script>

<style scoped>
.eh-settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 每一个设置条目行 */
.setting-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px;
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

.item-subtext {
  font-size: 13px;
  color: #88888c;
  line-height: 1.4;
}

.arrow-icon {
  font-size: 20px;
  color: #66666c;
  margin-left: 8px;
}

.item-right-content {
  display: flex;
  align-items: center;
  gap: 8px;
}

.quota-text {
  font-size: 14px;
  color: #a0a0a5;
  font-family: monospace;
}

/* 1. 分段选择控制器 (E-Hentai / EXHentai) */
.site-segmented {
  display: flex;
  background-color: #26262a;
  padding: 3px;
  border-radius: 6px;
  gap: 2px;
}

.segment-btn {
  border: none;
  background: transparent;
  color: #88888c;
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.segment-btn.active {
  background-color: #38383e;
  color: #ffffff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

/* 2. 原生风 Switch 开关 */
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
}

.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: #38383e;
  transition: 0.3s;
  border-radius: 24px;
}

.slider:before {
  position: absolute;
  content: '';
  height: 18px;
  width: 18px;
  left: 3px;
  bottom: 3px;
  background-color: #a0a0a5;
  transition: 0.3s;
  border-radius: 50%;
}

input:checked + .slider {
  background-color: #ff7588;
}

input:checked + .slider:before {
  transform: translateX(20px);
  background-color: #ffffff;
}
</style>
