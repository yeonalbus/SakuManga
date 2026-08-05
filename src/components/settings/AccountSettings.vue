<template>
  <div class="account-settings">
    <!-- 状态 1：未绑定账号 -->
    <template v-if="!isLoggedIn">
      <div class="setting-row unbound-card">
        <div class="unbound-info">
          <span class="row-label">当前未绑定 E 站凭证</span>
          <span class="sub-tip">需要绑定 Cookie 才能加载在线画廊与访问里站</span>
        </div>
        <button class="action-btn primary" :disabled="submitting" @click="handleOpenBindModal">
          绑定 Cookie
        </button>
      </div>
    </template>

    <!-- 状态 2：已绑定账号 -->
    <template v-else>
      <div class="setting-row">
        <span class="row-label">用户 UID</span>
        <span class="row-value highlight">
          {{ accountInfo.ipb_member_id || '未知用户' }}
          <span class="badge" :class="{ ex: hasExAccess }">
            {{ hasExAccess ? 'ExHentai' : 'E-Hentai' }}
          </span>
        </span>
      </div>

      <div class="setting-row clickable" @click="handleOpenBindModal">
        <span class="row-label">查看 / 更新凭证 (Cookie)</span>
        <span class="arrow-icon">›</span>
      </div>

      <div class="setting-row clickable danger" @click="handleLogout">
        <span class="row-label">清除凭证并退出</span>
        <span class="logout-icon">🚪</span>
      </div>
    </template>

    <!-- Cookie 编辑/绑定 弹窗 -->
    <div v-if="showModal" class="modal-mask" @click.self="handleCloseModal">
      <div class="modal-box">
        <h3>{{ isLoggedIn ? '更新 Cookie 凭证' : '绑定 Cookie 凭证' }}</h3>
        <p class="modal-tip">可以粘贴完整 Cookie 字符串，或手动填写核心参数：</p>

        <div class="form-group">
          <label>快速粘贴整条 Cookie (可选)</label>
          <textarea
            v-model="rawCookieInput"
            rows="3"
            placeholder="粘贴形如 ipb_member_id=xxx; ipb_pass_hash=xxx; igneous=xxx; sk=xxx; 的完整字符串"
            @input="handleAutoParseCookie"
          ></textarea>
        </div>

        <div class="form-grid">
          <div class="form-group">
            <label>ipb_member_id <span class="required">*</span></label>
            <input v-model="form.ipb_member_id" type="text" placeholder="例如: 1234567" />
          </div>
          <div class="form-group">
            <label>
              ipb_pass_hash
              <span v-if="!isLoggedIn" class="required">*</span>
              <span v-else class="sub-tip">(留空表示保持现有 Hash 不变)</span>
            </label>
            <input v-model="form.ipb_pass_hash" type="text" placeholder="32位的 Hash 字符串" />
          </div>
          <div class="form-group">
            <label>igneous (里站必需)</label>
            <input v-model="form.igneous" type="text" placeholder="访问 ExHentai 所需凭证" />
          </div>
          <div class="form-group">
            <label>sk (偏好设置 Cookie)</label>
            <input v-model="form.sk" type="text" placeholder="可选，用于同步 E 站个人偏好" />
          </div>
        </div>

        <div class="modal-actions">
          <button class="action-btn" :disabled="submitting" @click="handleCloseModal">取消</button>
          <button class="action-btn primary" :disabled="submitting" @click="handleSaveCookies">
            {{ submitting ? '保存中...' : '保存并校验' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'

const { toast, modal } = useUI()

interface EAccountConfig {
  ipb_member_id: string
  ipb_pass_hash: string
  igneous: string
  sk: string
}

// 状态定义
const isLoggedIn = ref(false)
const hasExAccess = ref(false)
const submitting = ref(false)

const accountInfo = reactive<EAccountConfig>({
  ipb_member_id: '',
  ipb_pass_hash: '',
  igneous: '',
  sk: '',
})

// 弹窗表单状态
const showModal = ref(false)
const rawCookieInput = ref('')
const form = reactive<EAccountConfig>({
  ipb_member_id: '',
  ipb_pass_hash: '',
  igneous: '',
  sk: '',
})

// 1. 从后端加载已有账户配置
const loadAccountSettings = async () => {
  try {
    const json = await http('/account/settings')

    if (json.isLoggedIn && json.data) {
      isLoggedIn.value = true
      accountInfo.ipb_member_id = json.data.ipb_member_id || ''
      accountInfo.igneous = json.data.igneous || ''
      accountInfo.sk = json.data.sk || ''
      hasExAccess.value = json.data.isEx || false
    } else {
      isLoggedIn.value = false
    }
  } catch (err) {
    console.error('获取账号配置失败:', err)
  }
}

// 2. 打开编辑弹窗
const handleOpenBindModal = () => {
  form.ipb_member_id = accountInfo.ipb_member_id
  form.ipb_pass_hash = '' // 出于安全控制，编辑时不回显原 Hash
  form.igneous = accountInfo.igneous
  form.sk = accountInfo.sk
  rawCookieInput.value = ''
  showModal.value = true
}

const handleCloseModal = () => {
  if (submitting.value) return
  showModal.value = false
}

// 3. 自动解析 Cookie 字符串
const handleAutoParseCookie = () => {
  if (!rawCookieInput.value.trim()) return

  const cookies: Record<string, string> = {}
  rawCookieInput.value.split(';').forEach((item) => {
    const [key, ...valParts] = item.split('=')
    if (key && valParts.length > 0) {
      cookies[key.trim()] = valParts.join('=').trim()
    }
  })

  if (cookies['ipb_member_id']) form.ipb_member_id = cookies['ipb_member_id']
  if (cookies['ipb_pass_hash']) form.ipb_pass_hash = cookies['ipb_pass_hash']
  if (cookies['igneous']) form.igneous = cookies['igneous']
  if (cookies['sk']) form.sk = cookies['sk']
}

// 4. 保存凭证并发给 Go 后端持久化
const handleSaveCookies = async () => {
  if (!form.ipb_member_id.trim()) {
    toast.error('ipb_member_id 为必填项！')
    return
  }

  if (!isLoggedIn.value && !form.ipb_pass_hash.trim()) {
    toast.error('首次绑定时 ipb_pass_hash 为必填项！')
    return
  }

  submitting.value = true
  try {
    const payload: Record<string, string> = {
      ipb_member_id: form.ipb_member_id.trim(),
      igneous: form.igneous.trim(),
      sk: form.sk.trim(),
    }

    if (form.ipb_pass_hash.trim()) {
      payload.ipb_pass_hash = form.ipb_pass_hash.trim()
    }

    // 改为函数调用模式，通过 method 指定 POST
    await http('/account/settings', {
      method: 'POST',
      body: JSON.stringify(payload),
    })

    showModal.value = false
    toast.success('凭证校验通过并已保存！')
    await loadAccountSettings()
  } catch (err: any) {
    toast.error(err?.message || err?.error || 'Cookie 校验失败，请检查网络或凭证！')
    console.error(err)
  } finally {
    submitting.value = false
  }
}

// 5. 退出/清除账号
const handleLogout = async () => {
  const confirmed = await modal.confirm('确定要退出登录并清除当前账号凭证吗？')
  if (!confirmed) return

  submitting.value = true
  try {
    // 改为函数调用模式，通过 method 指定 DELETE
    await http('/account/settings', {
      method: 'DELETE',
    })
    toast.success('已成功清除凭证！')
    await loadAccountSettings()
  } catch (err: any) {
    toast.error(err?.message || '清除失败')
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  loadAccountSettings()
})
</script>

<style scoped>
.account-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background-color: #1a1a1e;
  border-radius: 8px;
  border: 1px solid #26262a;
  font-size: 14px;
}

.unbound-card {
  padding: 18px 16px;
}

.unbound-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sub-tip {
  font-size: 12px;
  color: #888;
}

.setting-row.clickable {
  cursor: pointer;
  transition:
    background-color 0.2s ease,
    border-color 0.2s ease;
}

.setting-row.clickable:hover {
  background-color: #222226;
}

.setting-row.danger:hover {
  background-color: rgba(255, 117, 136, 0.08);
  border-color: rgba(255, 117, 136, 0.3);
}

.row-label {
  color: #d0d0d0;
}

.row-value.highlight {
  color: #ff7588;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 8px;
}

.badge {
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 4px;
  background-color: #333;
  color: #aaa;
}

.badge.ex {
  background-color: rgba(255, 117, 136, 0.2);
  color: #ff7588;
  border: 1px solid rgba(255, 117, 136, 0.4);
}

.arrow-icon {
  color: #66666c;
  font-size: 18px;
}

.logout-icon {
  font-size: 16px;
}

.action-btn {
  padding: 6px 14px;
  border-radius: 6px;
  border: 1px solid #36363a;
  background: #26262a;
  color: #eee;
  cursor: pointer;
  font-size: 13px;
  transition: all 0.2s;
}

.action-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.action-btn:hover:not(:disabled) {
  background: #323236;
}

.action-btn.primary {
  background: #ff7588;
  border-color: #ff7588;
  color: #fff;
}

.action-btn.primary:hover:not(:disabled) {
  background: #f06477;
}

.modal-mask {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-box {
  background: #1a1a1e;
  border: 1px solid #26262a;
  border-radius: 10px;
  padding: 20px;
  width: 480px;
  max-width: 90vw;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.modal-box h3 {
  margin: 0;
  color: #fff;
  font-size: 16px;
}

.modal-tip {
  margin: 0;
  font-size: 12px;
  color: #888;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-group label {
  font-size: 12px;
  color: #aaa;
}

.required {
  color: #ff7588;
}

.form-group input,
.form-group textarea {
  background: #121214;
  border: 1px solid #26262a;
  border-radius: 6px;
  padding: 8px 10px;
  color: #eee;
  font-size: 13px;
  outline: none;
  font-family: inherit;
}

.form-group input:focus,
.form-group textarea:focus {
  border-color: #ff7588;
}

.form-grid {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 10px;
}
</style>
