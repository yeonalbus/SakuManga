<template>
  <div class="style-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">主题模式</div>
      </div>
      <select v-model="themeMode" class="setting-select">
        <option value="system">跟随系统</option>
        <option value="dark">暗黑模式</option>
        <option value="light">亮色模式</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示样式</div>
        <div class="item-subtext">画廊列表的默认展示形态</div>
      </div>

      <div class="view-mode-segmented">
        <button
          class="segment-btn"
          :class="{ active: viewMode === 'card' }"
          @click="setViewMode('card')"
        >
          🎴 卡片
        </button>
        <button
          class="segment-btn"
          :class="{ active: viewMode === 'compact' }"
          @click="setViewMode('compact')"
        >
          🪪 名片
        </button>
      </div>
    </div>

    <div class="setting-item clickable" @click="handleThemeColor">
      <div class="item-info">
        <div class="item-title">主题颜色</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">画廊列表样式(全局)</div>
      </div>
      <select v-model="globalGalleryStyle" class="setting-select">
        <option value="card">卡片</option>
        <option value="compact">名片</option>
      </select>
    </div>

    <div class="setting-item clickable" @click="handlePageGalleryStyle">
      <div class="item-info">
        <div class="item-title">画廊列表样式(页面)</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">下载页网格布局列数(分组)</div>
      </div>
      <select v-model="downloadGroupCols" class="setting-select">
        <option value="auto">自动</option>
        <option value="2">2 列</option>
        <option value="3">3 列</option>
        <option value="4">4 列</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">下载页网格布局列数(画廊)</div>
      </div>
      <select v-model="downloadGalleryCols" class="setting-select">
        <option value="auto">自动</option>
        <option value="3">3 列</option>
        <option value="4">4 列</option>
        <option value="5">5 列</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">详情页缩略图列数</div>
      </div>
      <select v-model="detailThumbCols" class="setting-select">
        <option value="auto">自动</option>
        <option value="4">4 列</option>
        <option value="6">6 列</option>
        <option value="8">8 列</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">移动封面图至右侧</div>
        <div class="item-subtext">需要重启</div>
      </div>
      <div class="switch-control">
        <label class="toggle-switch">
          <input type="checkbox" v-model="moveCoverToRight" />
          <span class="slider"></span>
        </label>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">布局模式</div>
        <div class="item-subtext">双列带侧栏，支持键盘操作</div>
      </div>
      <select v-model="layoutMode" class="setting-select">
        <option value="desktop">桌面模式</option>
        <option value="mobile">移动模式</option>
      </select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useUI } from '@/composables/useUI'
import { viewMode, toggleViewMode } from '@/stores/viewMode'

const { toast } = useUI()

// 设置项响应式绑定
const themeMode = ref('system')
const globalGalleryStyle = ref('card')
const downloadGroupCols = ref('auto')
const downloadGalleryCols = ref('auto')
const detailThumbCols = ref('auto')
const moveCoverToRight = ref(false)
const layoutMode = ref('desktop')

// 交互调取
const handleThemeColor = () => {
  toast.info('打开主题颜色选择器')
}

const handlePageGalleryStyle = () => {
  toast.info('设置独立页面的列表样式')
}

const setViewMode = (targetMode: 'card' | 'compact') => {
  if (viewMode.value !== targetMode) {
    toggleViewMode()
    toast.info(`已切换显示样式为：${targetMode === 'card' ? '卡片' : '名片'}`)
  }
}
</script>

<style scoped>
.style-settings {
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

/* 统一的原生下拉菜单排版，去除底层粗黑框 */
.setting-select {
  background-color: transparent;
  color: #ffffff;
  border: none;
  font-size: 14px;
  padding: 4px 8px;
  cursor: pointer;
  outline: none;
  border-bottom: 1px solid #44444a;
  text-align-last: right;
  transition: border-color 0.2s;
}

.setting-select:focus {
  border-bottom-color: #ff7588;
}

.setting-select option {
  background-color: #1a1a1e;
  color: #ffffff;
}

/* 原生 Switch 开关 */
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

/* 接入胶囊分段按钮的专属 CSS 样式 */
.view-mode-segmented {
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
  display: flex;
  align-items: center;
  gap: 4px;
}

.segment-btn:hover {
  color: #ffffff;
}

.segment-btn.active {
  background-color: #38383e;
  color: #ffffff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
  font-weight: 600;
}
</style>
