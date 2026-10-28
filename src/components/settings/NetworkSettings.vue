<template>
  <div class="network-settings">
    <!-- ───────── 基础代理设置 ───────── -->
    <div class="section-title">基础代理设置</div>

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

    <!-- ───────── 高级代理设置 ───────── -->
    <div class="section-title with-divider">
      高级代理设置
      <span class="section-tag">Clash 联动</span>
    </div>

    <div class="setting-item hint-soft">
      <div class="item-info">
        <div class="item-title">⚠️ 高级功能说明</div>
        <div class="item-subtext">
          用于优化大批量归档下载：遇到「请求 archiver.php 失败」时自动更换出口节点重试。<br />
          不开不影响日常使用；开启后可能因反复解锁画廊产生额外 <b>GP 开销</b>。<br />
          该设置影响本实例上的<b>所有用户与所有下载任务</b>。
        </div>
      </div>
    </div>

    <!-- 总开关（始终可点，不受下面的置灰影响） -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">启用高级代理联动</div>
        <div class="item-subtext">
          {{ advanced.enabled ? '已启用：归档下载失败时会自动测延迟并切换节点' : '已关闭：所有联动逻辑不生效' }}
        </div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="advanced.enabled" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- 可编辑区域：关闭时置灰并禁止交互 -->
    <div class="advanced-body" :class="{ 'is-disabled': !advanced.enabled }">
      <div v-if="!advanced.enabled" class="disabled-mask">开启上方开关后可编辑</div>

      <!-- 代理组配置 -->
      <div class="group-title">代理组配置</div>
      <div class="setting-item column">
        <div class="item-info">
          <div class="item-title">e-hentai.org 所走的代理组 <span class="required">*</span></div>
          <div class="item-subtext">
            在 Clash Verge 的「订阅 - 编辑规则 - DOMAIN-SUFFIX」里为 e-hentai.org 指定的代理策略组名
          </div>
        </div>
        <input
          v-model="advanced.groupEGallery"
          class="text-input"
          list="clash-groups"
          placeholder="例如：Ehentai"
        />
      </div>

      <div class="setting-item column">
        <div class="item-info">
          <div class="item-title">exhentai.org 所走的代理组 <span class="required">*</span></div>
          <div class="item-subtext">里站所走的代理组名；若与表站相同，填同一个即可</div>
        </div>
        <input
          v-model="advanced.groupEGalleryEx"
          class="text-input"
          list="clash-groups"
          placeholder="例如：Ehentai"
        />
      </div>

      <div class="setting-item column">
        <div class="item-info">
          <div class="item-title">hath.network 所走的代理组（可选）</div>
          <div class="item-subtext">
            留空 = H@H 请求不参与联动切换，保持现状（推荐，H@H 常走 DIRECT 即可）
          </div>
        </div>
        <input
          v-model="advanced.groupHath"
          class="text-input"
          list="clash-groups"
          placeholder="留空表示不联动"
        />
      </div>

      <!-- 代理连接配置 -->
      <div class="group-title">代理连接配置</div>
      <div class="setting-item hint-soft">
        <div class="item-info">
          <div class="item-subtext">
            适配任何 <b>mihomo 内核</b>（Clash Verge Rev / Mihomo Party / OpenClash 等）。<br />
            要填的是 <b>「外部控制端口」</b>（如 <code>9090</code>），<b>不是</b>代理端口（如 <code>7897</code>）——
            填错会永远连不上。
          </div>
        </div>
      </div>

      <div class="setting-item column">
        <div class="item-info">
          <div class="item-title">控制面地址</div>
          <div class="item-subtext">external-controller 的 host:port</div>
        </div>
        <input
          v-model="advanced.controller"
          class="text-input"
          placeholder="127.0.0.1:9090"
        />
      </div>

      <div class="setting-item column">
        <div class="item-info">
          <div class="item-title">控制面密钥（Secret）</div>
          <div class="item-subtext">
            {{ saved.hasSecret ? '已设置密钥（显示为掩码）；需要修改时重新输入，留空表示保持不变' : 'Clash 未设置密钥时留空' }}
          </div>
        </div>
        <input
          v-model="secretInput"
          class="text-input"
          type="password"
          :placeholder="saved.hasSecret ? '已设置（留空保持不变，可直接测试连接）' : '可留空'"
        />
      </div>

      <!-- 连通性测试 -->
      <div class="setting-item column">
        <div class="item-info">
          <div class="item-title">连通性测试</div>
          <div class="item-subtext">验证控制面可达 + 代理组存在 + 节点能连上 E 站</div>
        </div>
        <div class="test-row">
          <button class="action-btn" :disabled="testing" @click="handleTest">
            {{ testing ? '测试中…' : '测试连接' }}
          </button>
          <button class="action-btn ghost" :disabled="loadingGroups" @click="loadGroups">
            {{ loadingGroups ? '读取中…' : '读取候选组' }}
          </button>
        </div>
        <div v-if="testResult" class="test-result" :class="{ bad: !testResult.ok }">
          {{ testResult.message }}
        </div>
      </div>

      <!-- 保存 -->
      <div class="save-row">
        <button class="save-btn" :disabled="saving" @click="handleSave">
          {{ saving ? '保存中…' : '保存高级代理设置' }}
        </button>
      </div>
    </div>

    <!-- 候选组 datalist（后端枚举，避免手填错名） -->
    <datalist id="clash-groups">
      <option v-for="g in groupCandidates" :key="g.name" :value="g.name">
        {{ g.type }} · 当前 {{ g.now }} · {{ g.options }} 个选项
      </option>
    </datalist>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, reactive } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import { networkSettings, resetNetworkSettings } from '@/stores/networkSettings'
import {
  fetchAdvancedProxy,
  saveAdvancedProxy,
  testClashConnection,
  fetchClashGroups,
  type AdvancedProxySetting,
  type ClashGroupItem,
  type ClashTestResult,
} from '@/api/network'

const { toast, modal } = useUI()

// ── 基础代理设置 ──
const proxyAddress = ref('')
const effectiveProxy = ref('')
const effectiveSource = ref<'manual' | 'system' | 'none'>('none')

const effectiveLabel = computed(() => {
  if (effectiveSource.value === 'manual') {
    return `手动配置: ${effectiveProxy.value || proxyAddress.value || '（空）'}`
  }
  if (effectiveSource.value === 'system') {
    return `自动跟随系统代理: ${effectiveProxy.value}`
  }
  return '未设置 (直连模式)'
})

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

const handleProxySetting = async () => {
  const input = await modal.prompt(
    '请输入 HTTP / SOCKS5 代理地址（如 http://127.0.0.1:7897，留空表示自动跟随系统代理）：',
    proxyAddress.value || effectiveProxy.value || 'http://127.0.0.1:7897',
    '配置代理服务器',
  )

  if (input !== null) {
    const newProxy = input.trim()
    try {
      await http('/network/proxy', {
        method: 'POST',
        body: JSON.stringify({ proxy: newProxy }),
      })
      proxyAddress.value = newProxy
      effectiveProxy.value = newProxy
      effectiveSource.value = newProxy ? 'manual' : 'system'
      toast.success(newProxy ? `代理成功更新为: ${newProxy}` : '已切换为自动跟随系统代理')
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : '设置失败')
    }
  }
}

const handleReset = () => {
  resetNetworkSettings()
  toast.success('已恢复默认网络设置')
}

// ── 高级代理设置 ──
const advanced = reactive<AdvancedProxySetting>({
  enabled: false,
  controller: '127.0.0.1:9090',
  hasSecret: false,
  groupEGallery: '',
  groupEGalleryEx: '',
  groupHath: '',
})
// saved 保存"上次从后端读到的状态"，用于密钥 keepSecret 判断与占位提示
const saved = reactive({ hasSecret: false })
const secretInput = ref('')
const testing = ref(false)
const saving = ref(false)
const loadingGroups = ref(false)
const testResult = ref<ClashTestResult | null>(null)
const groupCandidates = ref<ClashGroupItem[]>([])

const fetchAdvanced = async () => {
  try {
    const data = await fetchAdvancedProxy()
    Object.assign(advanced, data)
    saved.hasSecret = data.hasSecret
  } catch (err) {
    console.error('获取高级代理设置失败:', err)
  }
}

const handleTest = async () => {
  testing.value = true
  testResult.value = null
  try {
    const result = await testClashConnection({
      controller: advanced.controller,
      secret: secretInput.value || undefined,
    })
    testResult.value = result
    if (result.ok) {
      const parts = [`mihomo ${result.version ?? ''}`]
      if (result.group) parts.push(`组：${result.group}`)
      if (result.now) parts.push(`当前：${result.now}`)
      if (result.bestDelayMs) parts.push(`最优延迟 ${result.bestDelayMs}ms`)
      if (typeof result.usable === 'number') parts.push(`可用 ${result.usable}/${result.options ?? '?'}`)
      testResult.value = { ...result, message: parts.join(' · ') }
    }
  } catch (err: unknown) {
    testResult.value = { ok: false, message: err instanceof Error ? err.message : '测试失败' }
  } finally {
    testing.value = false
  }
}

const loadGroups = async () => {
  loadingGroups.value = true
  try {
    const data = await fetchClashGroups()
    groupCandidates.value = data.groups ?? []
    if (groupCandidates.value.length === 0) {
      toast.error('未读取到任何策略组，请确认控制面地址与密钥')
    } else {
      toast.success(`已读取 ${groupCandidates.value.length} 个策略组，可在输入框下拉选择`)
    }
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '读取策略组失败')
  } finally {
    loadingGroups.value = false
  }
}

const handleSave = async () => {
  if (advanced.enabled) {
    if (!advanced.groupEGallery.trim() || !advanced.groupEGalleryEx.trim()) {
      toast.error('请先填写 e-hentai.org 与 exhentai.org 的代理组名称（或关闭高级代理设置）')
      return
    }
    if (!advanced.controller.trim()) {
      toast.error('请填写 Clash 控制面地址（如 127.0.0.1:9090）')
      return
    }
  }

  saving.value = true
  try {
    const result = await saveAdvancedProxy({
      enabled: advanced.enabled,
      controller: advanced.controller.trim(),
      secret: secretInput.value,
      keepSecret: secretInput.value === '' && saved.hasSecret,
      groupEGallery: advanced.groupEGallery.trim(),
      groupEGalleryEx: advanced.groupEGalleryEx.trim(),
      groupHath: advanced.groupHath.trim(),
    })
    // 保存成功后**不清空密钥输入框**：type=password 会继续显示为掩码，
    // 用户既能确认自己填过密码，也能直接点「测试连接」而不用重新输入。
    await fetchAdvanced()
    toast.success('高级代理设置已保存')
    if (result.warning) {
      toast.warning(result.warning)
    }
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  fetchProxyConfig()
  fetchAdvanced()
})
</script>

<style scoped>
.network-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* ── 板块标题 ── */
.section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-2);
  letter-spacing: 0.5px;
  padding: 4px 2px;
}

.section-title.with-divider {
  margin-top: 12px;
  padding-top: 16px;
  border-top: 1px solid var(--app-border-2);
  display: flex;
  align-items: center;
  gap: 8px;
}

.section-tag {
  font-size: 11px;
  font-weight: 500;
  color: #ff7588;
  border: 1px solid rgba(255, 117, 136, 0.4);
  border-radius: 4px;
  padding: 1px 6px;
}

.group-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-3);
  padding: 6px 2px 0;
}

/* ── 基础设置项（沿用原有视觉） ── */
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

.setting-item.column {
  flex-direction: column;
  align-items: stretch;
  gap: 10px;
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
  line-height: 1.5;
}

.item-subtext code {
  background: var(--app-surface-1, rgba(0, 0, 0, 0.15));
  border-radius: 3px;
  padding: 0 4px;
  font-size: 12px;
}

.required {
  color: #ff7588;
}

/* 提示卡片 */
.setting-item.hint {
  background-color: var(--app-surface-2);
  border-color: rgba(255, 117, 136, 0.25);
}

.setting-item.hint .item-title {
  color: #ff7588;
}

.setting-item.hint-soft {
  background-color: var(--app-surface-2);
  border-style: dashed;
}

.arrow-icon {
  font-size: 20px;
  color: var(--app-text-muted);
  margin-left: 8px;
}

/* ── 输入 ── */
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

.text-input {
  width: 100%;
  box-sizing: border-box;
  background: var(--app-surface-1, transparent);
  border: 1px solid var(--app-border-3);
  border-radius: 6px;
  color: var(--app-text-strong);
  font-size: 14px;
  padding: 8px 10px;
  outline: none;
  transition: border-color 0.2s;
}

.text-input:focus {
  border-color: #ff7588;
}

.text-input::placeholder {
  color: var(--app-text-muted);
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

/* ── 开关 ── */
.toggle-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
  flex-shrink: 0;
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

/* ── 高级区域置灰 ── */
.advanced-body {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 12px;
  transition: filter 0.2s ease, opacity 0.2s ease;
}

.advanced-body.is-disabled {
  filter: grayscale(0.9);
  opacity: 0.55;
  pointer-events: none;
  user-select: none;
}

.disabled-mask {
  position: absolute;
  inset: -8px;
  z-index: 2;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 12px;
  font-size: 12px;
  color: var(--app-text-3);
  background: linear-gradient(
    180deg,
    var(--app-surface-1, rgba(0, 0, 0, 0.2)) 0%,
    transparent 40%
  );
  border-radius: 8px;
  pointer-events: none;
}

/* ── 操作按钮 ── */
.test-row {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.action-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-strong);
  font-size: 13px;
  padding: 7px 14px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.action-btn:hover:not(:disabled) {
  border-color: #ff7588;
  color: #ff7588;
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.action-btn.ghost {
  color: var(--app-text-2);
}

.test-result {
  font-size: 13px;
  color: #4caf82;
  line-height: 1.5;
  word-break: break-all;
}

.test-result.bad {
  color: #ff7588;
}

.save-row {
  display: flex;
  justify-content: flex-end;
  padding-top: 4px;
}

.save-btn {
  background: rgba(255, 117, 136, 0.14);
  border: 1px solid rgba(255, 117, 136, 0.5);
  color: #ff7588;
  font-size: 13px;
  padding: 8px 18px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.save-btn:hover:not(:disabled) {
  background: rgba(255, 117, 136, 0.24);
}

.save-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* ── 底部重置 ── */
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
