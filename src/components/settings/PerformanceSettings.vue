<template>
  <div class="performance-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">下载页支持列表动画的最大画廊个数</div>
        <div class="item-subtext">
          列表模式下，拥有超过此配置个数画廊的分组在展开/收起时取消动画效果
        </div>
      </div>
      <div class="input-inline">
        <input
          type="number"
          v-model="performanceSettings.maxAnimationGalleries"
          class="setting-input"
        />
        <span class="check-mark">✓</span>
      </div>
    </div>

    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useUI } from '@/composables/useUI'
import { performanceSettings, resetPerformanceSettings } from '@/stores/performanceSettings'

const { toast } = useUI()

const handleReset = () => {
  resetPerformanceSettings()
  toast.success('已恢复默认性能设置')
}
</script>

<style scoped>
.performance-settings {
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

.input-inline {
  display: flex;
  align-items: center;
  gap: 6px;
}

.setting-input {
  background: transparent;
  border: none;
  border-bottom: 1px solid #44444a;
  color: #ffffff;
  font-size: 14px;
  width: 45px;
  text-align: center;
  outline: none;
}

.setting-input:focus {
  border-bottom-color: #ff7588;
}

.check-mark {
  color: #a0a0a5;
  font-size: 14px;
  margin-left: 4px;
}

.reset-row {
  display: flex;
  justify-content: center;
  padding: 8px 0;
}

.reset-btn {
  background: transparent;
  border: 1px solid #44444a;
  color: #a0a0a5;
  font-size: 13px;
  padding: 8px 20px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.reset-btn:hover {
  border-color: #ff7588;
  color: #ffffff;
}
</style>
