<template>
  <div class="network-settings">
    <!-- 🟢 代理服务器配置项（真实生效：后端 config.json + 系统代理自动兜底） -->
    <div class="setting-item clickable" @click="handleProxySetting">
      <div class="item-info">
        <div class="item-title">代理服务器地址</div>
        <div class="item-subtext">
          {{ effectiveLabel }}
        </div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <!-- 当前生效代理来源说明 -->
    <div v-if="effectiveSource === 'system'" class="setting-item hint">
      <div class="item-info">
        <div class="item-title">🖥️ 正在自动使用系统代理</div>
        <div class="item-subtext">
          检测到系统代理 {{ effectiveProxy }}（Clash / v2rayN 等）。如需固定代理地址，点击上方配置。
        </div>
      </div>
    </div>
    <div v-else-if="effectiveSource === 'none'" class="setting-item hint">
      <div class="item-info">
        <div class="item-title">📡 当前为直连模式</div>
        <div class="item-subtext">
          未检测到系统代理。若浏览器能访问 E 站但程序连不上，请在上方填写代理地址（如 http://127.0.0.1:7897）。
        </div>
      </div>
    </div>

    <!-- 请求超时时间（接线 request.ts：fetch AbortSignal） -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">请求超时时间</div>
        <div class="item-subtext">单次请求的最长等待时间，超时自动中断</div>
      </div>
      <div class="input-inline">
        <input
          type="number"
          v-model="networkSettings.requestTimeout"
          class="setting-input wider"
          min="1000"
          step="1000"
        />
        <span class="unit">ms</span>
        <span class="check-mark">✓</span>
      </div>
    </div>

    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import { networkSettings, resetNetworkSettings } from '@/stores/networkSettings'

const { toast, modal } = useUI()

// 代理地址来自后端 API（/network/proxy），不存入本地 store
const proxyAddress = ref('')
// 实际生效代理（手动优先，未配置时自动采用系统代理）与其来源：manual / system / none
const effectiveProxy = ref('')
const effectiveSource = ref<'manual' | 'system' | 'none'>('none')

// 设置项副标题文案
const effectiveLabel = computed(() => {
  if (effectiveSource.value === 'manual') {
    return `手动配置: ${effectiveProxy.value || proxyAddress.value || '（空）'}`
  }
  if (effectiveSource.value === 'system') {
    return `自动跟随系统代理: ${effectiveProxy.value}`
  }
  return '未设置 (直连模式)'
})

// 获取后端当前设置的代理
const fetchProxyConfig = async () => {
  try {
    const data = await http<{
      proxy: string
      effective: string
      source: 'manual' | 'system' | 'none'
    }>('/network/proxy')
    proxyAddress.value = data.proxy || ''
    effectiveProxy.value = data.effective || ''
    effectiveSource.value = data.source || 'none'
  } catch (err) {
    console.error('获取代理配置失败:', err)
  }
}

// 弹出输入框配置代理地址
const handleProxySetting = async () => {
  const input = await modal.prompt(
    '请输入 HTTP / SOCKS5 代理地址（如 http://127.0.0.1:7897，留空表示自动跟随系统代理）：',
    proxyAddress.value || effectiveProxy.value || 'http://127.0.0.1:7897',
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
      effectiveProxy.value = newProxy
      effectiveSource.value = newProxy ? 'manual' : 'system'
      toast.success(newProxy ? `代理成功更新为: ${newProxy}` : '已切换为自动跟随系统代理')
    } catch (err: unknown) {
      // 🔴 无论是网络连不上，还是后端返回了 400 错误（如“无效的代理格式”），
      // http 都会自动把后端的报错文字放入 err.message 中
      toast.error(err instanceof Error ? err.message : '设置失败')
    }
  }
}

const handleReset = () => {
  resetNetworkSettings()
  toast.success('已恢复默认网络设置')
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

/* 系统代理自动兜底 / 直连提示卡片 */
.setting-item.hint {
  background-color: var(--app-surface-2);
  border-color: rgba(255, 117, 136, 0.25);
}

.setting-item.hint .item-title {
  color: #ff7588;
}

.arrow-icon {
  font-size: 20px;
  color: var(--app-text-muted);
  margin-left: 8px;
}

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

.input-inline {
  display: flex;
  align-items: center;
  gap: 6px;
}

.setting-input {
  background: transparent;
  border: none;
  border-bottom: 1px solid var(--app-border-3);
  color: var(--app-text-strong);
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
  color: var(--app-text-strong);
}

.check-mark {
  color: var(--app-text-2);
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
