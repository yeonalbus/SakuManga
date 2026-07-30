<template>
  <div class="network-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启域名前置</div>
        <div class="item-subtext">绕过SNI封锁</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="domainFronting" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item clickable" @click="handleProxySetting">
      <div class="item-info">
        <div class="item-title">代理服务器地址</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">页面缓存时间</div>
        <div class="item-subtext">你可以通过刷新页面来更新缓存</div>
      </div>
      <select v-model="pageCacheTime" class="setting-select">
        <option value="1d">1d</option>
        <option value="3d">3d</option>
        <option value="7d">7d</option>
        <option value="30d">30d</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">图片缓存时间</div>
        <div class="item-subtext">App启动时会自动清除过期的图片缓存</div>
      </div>
      <select v-model="imageCacheTime" class="setting-select">
        <option value="7d">7d</option>
        <option value="15d">15d</option>
        <option value="30d">30d</option>
        <option value="90d">90d</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">建立连接超时时间</div>
      </div>
      <div class="input-inline">
        <input type="number" v-model="connectTimeout" class="setting-input wider" />
        <span class="unit">ms</span>
        <span class="check-mark">✓</span>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">接收数据超时时间</div>
      </div>
      <div class="input-inline">
        <input type="number" v-model="receiveTimeout" class="setting-input wider" />
        <span class="unit">ms</span>
        <span class="check-mark">✓</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useUI } from '@/composables/useUI'

const { toast } = useUI()

// 响应式状态绑定
const domainFronting = ref(false)
const pageCacheTime = ref('3d')
const imageCacheTime = ref('30d')
const connectTimeout = ref(6000)
const receiveTimeout = ref(6000)

const handleProxySetting = () => {
  toast.info('打开代理服务器配置面板')
}
</script>

<style scoped>
.network-settings {
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

/* 下拉选择框 */
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

/* 内联输入框 */
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
  text-align: right;
  outline: none;
}

.setting-input.wider {
  width: 60px;
}

.setting-input:focus {
  border-bottom-color: #ff7588;
}

.unit {
  font-size: 13px;
  color: #ffffff;
}

.check-mark {
  color: #a0a0a5;
  font-size: 14px;
  margin-left: 4px;
}

/* Switch 开关 */
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
