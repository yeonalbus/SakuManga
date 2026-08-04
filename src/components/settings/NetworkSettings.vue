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

    <!-- 🟢 代理服务器配置项 -->
    <div class="setting-item clickable" @click="handleProxySetting">
      <div class="item-info">
        <div class="item-title">代理服务器地址</div>
        <div class="item-subtext">{{ proxyAddress || '未设置 (直连模式)' }}</div>
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
import { ref, onMounted } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'

const { toast, modal } = useUI()

// 响应式状态绑定
const domainFronting = ref(false)
const proxyAddress = ref('')
const pageCacheTime = ref('3d')
const imageCacheTime = ref('30d')
const connectTimeout = ref(6000)
const receiveTimeout = ref(6000)

// 获取后端当前设置的代理
const fetchProxyConfig = async () => {
  try {
    const data = await http<{ proxy: string }>('/network/proxy')
    proxyAddress.value = data.proxy || ''
  } catch (err) {
    console.error('获取代理配置失败:', err)
  }
}

// 弹出输入框配置代理地址
const handleProxySetting = async () => {
  const input = await modal.prompt(
    '请输入 HTTP / SOCKS5 代理地址（如 http://127.0.0.1:7897，留空表示直连）：',
    proxyAddress.value || 'http://127.0.0.1:7897',
    '配置代理服务器',
  )

  if (input !== null) {
    const newProxy = input.trim()
    try {
      // 🟢 使用 http 发起 POST 请求
      await http('/network/proxy', {
        method: 'POST',
        body: JSON.stringify({ proxy: newProxy }),
      })

      // 能走到这一步，说明后端响应了 200 OK（设置成功）
      proxyAddress.value = newProxy
      toast.success(newProxy ? `代理成功更新为: ${newProxy}` : '已切换为直连模式')
    } catch (err: any) {
      // 🔴 无论是网络连不上，还是后端返回了 400 错误（如“无效的代理格式”），
      // http 都会自动把后端的报错文字放入 err.message 中
      toast.error(err.message || '设置失败')
    }
  }
}

onMounted(() => {
  fetchProxyConfig()
})
</script>

<style scoped>
/* 样式部分保持不变 */
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
