<template>
  <div class="preference-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">启动时默认菜单</div>
      </div>
      <select v-model="preferenceSettings.defaultStartupMenu" class="setting-select">
        <option value="hot">热门</option>
        <option value="home">首页</option>
        <option value="sub">订阅</option>
        <option value="fav">收藏</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">隐藏快速回顶按钮</div>
        <div class="item-subtext">选择何时隐藏右下角的「回到顶部」按钮</div>
      </div>
      <select v-model="preferenceSettings.hideScrollToTopBtn" class="setting-select">
        <option value="scrolling_down">向下滚动时</option>
        <option value="always">总是隐藏</option>
        <option value="never">从不隐藏</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示画廊评论</div>
        <div class="item-subtext">在画廊详情页显示社区评论</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.showGalleryComments" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">以全屏模式启动</div>
        <div class="item-subtext">启动后自动进入全屏，F11 可手动切换</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.startInFullscreen" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">搜索选项继承</div>
        <div class="item-subtext">搜索时使用上一次搜索选项</div>
      </div>
      <select v-model="preferenceSettings.searchOptionsInherit" class="setting-select">
        <option value="all">继承全部</option>
        <option value="category_only">仅继承分类</option>
        <option value="none">不继承</option>
      </select>
    </div>

    <!-- S1 本地优先加载 -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">本地优先加载</div>
        <div class="item-subtext">在线画廊已有本地副本时，详情页预览/阅读优先使用本地文件（评论仍在线拉取）</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.preferLocalGallery" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useUI } from '@/composables/useUI'
import { preferenceSettings, resetPreferenceSettings } from '@/stores/preferenceSettings'

const { toast } = useUI()

// 恢复默认偏好设置
const handleReset = () => {
  resetPreferenceSettings()
  toast.success('已恢复默认偏好设置')
}
</script>

<style scoped>
.preference-settings {
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

.item-subtext {
  font-size: 13px;
  color: var(--app-text-3);
  line-height: 1.4;
}

.item-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.arrow-icon {
  font-size: 20px;
  color: var(--app-text-muted);
  margin-left: 8px;
}

.info-btn {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 1px solid var(--app-text-3);
  background: transparent;
  color: var(--app-text-3);
  font-size: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s ease;
}

.info-btn:hover {
  color: var(--app-text-strong);
  border-color: var(--app-text-strong);
}

/* 统一的原生下拉菜单 */
.setting-select {
  background-color: transparent;
  color: var(--app-text-strong);
  border: none;
  font-size: 14px;
  padding: 4px 8px;
  cursor: pointer;
  outline: none;
  border-bottom: 1px solid var(--app-border-3);
  text-align-last: right;
  transition: border-color 0.2s;
}

.setting-select:focus {
  border-bottom-color: #ff7588;
}

.setting-select option {
  background-color: var(--app-surface-2);
  color: var(--app-text-strong);
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
  background-color: var(--app-border-3);
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
  background-color: var(--app-text-2);
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

.reset-row {
  display: flex;
  justify-content: center;
  padding: 8px 0;
}

.reset-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  font-size: 13px;
  padding: 8px 20px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.reset-btn:hover {
  border-color: #ff7588;
  color: var(--app-text-strong);
}
</style>
