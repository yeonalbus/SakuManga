<template>
  <div class="account-settings">
    <!-- 当前登录会话信息 -->
    <div class="setting-row">
      <div class="unbound-info">
        <span class="row-label">当前登录</span>
        <span class="sub-tip">
          {{ userStore.user?.username }} · {{ userStore.isAdmin ? '管理员' : '成员' }}
        </span>
      </div>
      <button class="action-btn" :disabled="submitting" @click="handleLogoutAccount">
        退出登录
      </button>
    </div>

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

      <div class="setting-row">
        <span class="row-label">Pass Hash</span>
        <span class="row-value hash">{{ accountInfo.ipb_pass_hash || '未配置' }}</span>
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

    <!-- ── 管理员区块（Round25：由原「安全」页并入）── -->
    <template v-if="userStore.isAdmin">
      <div class="section-title">👤 账户操作</div>

      <div class="setting-row">
        <div class="unbound-info">
          <span class="row-label">修改用户名 / 密码</span>
          <span class="sub-tip">修改当前登录账号的凭据</span>
        </div>
        <div class="member-actions">
          <button class="mini-btn" :disabled="memberSubmitting" @click="handleRenameCurrent">
            修改用户名
          </button>
          <button class="mini-btn" :disabled="memberSubmitting" @click="handleResetPwdCurrent">
            修改密码
          </button>
        </div>
      </div>

      <div class="section-title">👥 成员管理</div>

      <div class="member-form">
        <input v-model="newMember.username" placeholder="用户名" class="text-input" />
        <input
          v-model="newMember.password"
          type="password"
          placeholder="初始密码"
          class="text-input"
          @keyup.enter="handleCreateMember"
        />
        <button class="action-btn primary" :disabled="memberSubmitting" @click="handleCreateMember">
          {{ memberSubmitting ? '创建中…' : '新增成员' }}
        </button>
      </div>

      <div class="member-list">
        <div v-for="u in members" :key="u.id" class="member-row">
          <div class="member-info">
            <span class="member-name">
              {{ u.username }}
              <span v-if="u.id === userStore.user?.id" class="self-tag">我</span>
            </span>
            <span class="member-meta">
              {{ u.role === 'admin' ? '管理员' : '成员' }}
              <span class="badge" :class="{ ex: u.allowDownload || u.role === 'admin' }">
                {{ u.role === 'admin' ? '下载不限' : u.allowDownload ? '可下载' : '禁下载' }}
              </span>
            </span>
          </div>
          <div class="member-actions">
            <button
              v-if="u.role !== 'admin'"
              class="mini-btn"
              :disabled="memberSubmitting"
              @click="handleToggleDownload(u)"
            >
              {{ u.allowDownload ? '取消下载许可' : '开启下载许可' }}
            </button>
            <button class="mini-btn" :disabled="memberSubmitting" @click="handleRename(u)">
              改名
            </button>
            <button class="mini-btn" :disabled="memberSubmitting" @click="handleResetPwd(u)">
              {{ u.id === userStore.user?.id ? '修改密码' : '重置密码' }}
            </button>
            <button
              v-if="u.id !== userStore.user?.id"
              class="mini-btn"
              :disabled="memberSubmitting"
              @click="handleViewHistory(u)"
            >
              历史
            </button>
            <button
              v-if="u.id !== userStore.user?.id && u.role !== 'admin'"
              class="mini-btn danger"
              :disabled="memberSubmitting"
              @click="handleDeleteUser(u)"
            >
              删除
            </button>
          </div>
        </div>
        <div v-if="members.length === 0" class="empty-tip">暂无成员</div>
      </div>

      <div class="section-title">🖥️ 服务器</div>

      <div class="setting-row">
        <div class="unbound-info">
          <span class="row-label">监听地址</span>
          <span class="sub-tip">127.0.0.1 仅本机可访问；0.0.0.0 允许局域网 / 公网访问</span>
        </div>
        <select v-model="serverForm.bindHost" class="setting-select">
          <option value="0.0.0.0">0.0.0.0（所有网卡）</option>
          <option value="127.0.0.1">127.0.0.1（仅本机）</option>
        </select>
      </div>

      <div class="setting-row">
        <div class="unbound-info">
          <span class="row-label">监听端口</span>
          <span class="sub-tip">修改后重启服务生效</span>
        </div>
        <div class="input-inline">
          <input
            v-model.number="serverForm.port"
            type="number"
            class="setting-input wide"
            min="1"
            max="65535"
          />
        </div>
      </div>

      <div class="setting-row">
        <div class="unbound-info">
          <span class="row-label">每用户历史记录上限</span>
          <span class="sub-tip">超出上限自动淘汰最旧记录</span>
        </div>
        <div class="input-inline">
          <input
            v-model.number="serverForm.historyLimit"
            type="number"
            class="setting-input wide"
            min="10"
            max="100000"
          />
          <span class="unit">条</span>
        </div>
      </div>

      <div class="reset-row">
        <button class="reset-btn" :disabled="serverSubmitting" @click="handleSaveServer">
          {{ serverSubmitting ? '保存中…' : '保存服务器配置' }}
        </button>
      </div>
    </template>

    <!-- Cookie 编辑/绑定 弹窗 -->
    <div v-if="showModal" class="modal-mask" @click.self="handleCloseModal">
      <div class="modal-box">
        <h3>{{ isLoggedIn ? '更新 Cookie 凭证' : '绑定 Cookie 凭证' }}</h3>

        <!-- 方式 A：账号密码内部登录（免 F12 复制） -->
        <div class="login-form">
          <div class="login-mode-title">🔑 方式一：使用 E 站账号密码登录</div>
          <div class="login-mode-tip">无需打开浏览器复制 Cookie；遇验证码/风控时请改用方式二</div>
          <input
            v-model="loginForm.username"
            type="text"
            placeholder="E 站账号（用户名）"
            class="text-input full"
            autocomplete="username"
          />
          <input
            v-model="loginForm.password"
            type="password"
            placeholder="E 站密码"
            class="text-input full"
            autocomplete="current-password"
            @keyup.enter="handlePasswordLogin"
          />
          <button
            class="action-btn primary full-btn"
            :disabled="loggingIn || refreshing"
            @click="handlePasswordLogin"
          >
            {{ loggingIn ? '登录中…' : '登录并绑定' }}
          </button>
        </div>

        <div class="login-divider"><span>或</span></div>

        <!-- 方式 B：粘贴 Cookie -->
        <p class="modal-tip">方式二：可以粘贴完整 Cookie 字符串，或手动填写核心参数：</p>

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
          <button class="action-btn" :disabled="submitting || refreshing" @click="handleCloseModal">
            取消
          </button>
          <button
            class="action-btn refresh"
            :disabled="
              submitting ||
              refreshing ||
              !form.ipb_member_id.trim() ||
              !form.ipb_pass_hash.trim()
            "
            :title="
              form.ipb_member_id.trim() && form.ipb_pass_hash.trim()
                ? '自动刷新 igneous（里站）与 sk（表站）凭证，无需自行上站抓取'
                : '请先填写 ipb_member_id 与 ipb_pass_hash'
            "
            @click="handleRefreshCookies"
          >
            {{ refreshing ? '刷新中...' : '刷新凭证' }}
          </button>
          <button class="action-btn primary" :disabled="submitting || refreshing" @click="handleSaveCookies">
            {{ submitting ? '保存中...' : '保存并校验' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import { useUserStore } from '@/stores/userStore'
import type { UserInfo } from '@/types/user'

const { toast, modal } = useUI()
const router = useRouter()
const userStore = useUserStore()

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
const refreshing = ref(false)

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

// ── 方式一：账号密码内部登录（免 F12 复制 Cookie）──
const loggingIn = ref(false)
const loginForm = reactive({ username: '', password: '' })

const handlePasswordLogin = async () => {
  if (!loginForm.username.trim() || !loginForm.password) {
    toast.error('请输入 E 站账号与密码')
    return
  }
  loggingIn.value = true
  try {
    await http('/account/login', {
      method: 'POST',
      body: JSON.stringify({
        username: loginForm.username.trim(),
        password: loginForm.password,
      }),
    })
    loginForm.password = ''
    showModal.value = false
    toast.success('登录成功，E 站凭证已保存！')
    await loadAccountSettings()
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '登录失败（遇验证码或风控时请改用粘贴 Cookie 方式）')
  } finally {
    loggingIn.value = false
  }
}

// 1. 从后端加载已有账户配置
const loadAccountSettings = async () => {
  try {
    const json = await http<{
      isLoggedIn?: boolean
      data?: {
        ipb_member_id?: string
        ipb_pass_hash?: string
        igneous?: string
        sk?: string
        isEx?: boolean
      }
    }>('/account/settings')

    if (json.isLoggedIn && json.data) {
      isLoggedIn.value = true
      accountInfo.ipb_member_id = json.data.ipb_member_id || ''
      accountInfo.ipb_pass_hash = json.data.ipb_pass_hash || ''
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
  // 回显现有 Hash：手动修改 sk/igneous 时无需重填 pass_hash
  form.ipb_pass_hash = accountInfo.ipb_pass_hash
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
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : 'Cookie 校验失败，请检查网络或凭证！')
    console.error(err)
  } finally {
    submitting.value = false
  }
}

// 4.5 刷新凭证：用当前表单中的 member_id/pass_hash 请求后端刷新 sk / igneous，
// 新值回填表单（不落库），确认后再走「保存并校验」
const handleRefreshCookies = async () => {
  if (!form.ipb_member_id.trim() || !form.ipb_pass_hash.trim()) {
    toast.error('请先填写 ipb_member_id 与 ipb_pass_hash 后再刷新')
    return
  }
  refreshing.value = true
  try {
    const res = await http<{ igneous?: string; sk?: string }>('/account/refresh-cookies', {
      method: 'POST',
      body: JSON.stringify({
        ipb_member_id: form.ipb_member_id.trim(),
        ipb_pass_hash: form.ipb_pass_hash.trim(),
      }),
    })
    let got = false
    if (res.igneous) {
      form.igneous = res.igneous
      got = true
    }
    if (res.sk) {
      form.sk = res.sk
      got = true
    }
    if (got) {
      toast.success('已获取新凭证（sk / igneous），请确认后保存')
    } else {
      toast.error('未获取到新凭证，请检查凭证是否有效')
    }
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '刷新凭证失败')
  } finally {
    refreshing.value = false
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
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : '清除失败')
  } finally {
    submitting.value = false
  }
}

// 5.5 退出当前登录会话（返回登录页）
const handleLogoutAccount = async () => {
  const confirmed = await modal.confirm('确定要退出登录吗？')
  if (!confirmed) return
  await userStore.logout()
  router.replace('/login')
}

// ══════════ 管理员：账户操作 / 成员管理 / 服务器（Round25 由原「安全」页并入）══════════

// ── 成员管理 ──
const members = ref<UserInfo[]>([])
const memberSubmitting = ref(false)
const newMember = reactive({ username: '', password: '' })

const loadMembers = async () => {
  try {
    const data = await http<{ users: UserInfo[] }>('/users')
    members.value = data.users
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载成员失败')
  }
}

const handleViewHistory = (u: UserInfo) => {
  router.push(`/member-history?userId=${u.id}`)
}

const handleCreateMember = async () => {
  if (!newMember.username.trim() || !newMember.password.trim()) {
    toast.error('请输入用户名和初始密码')
    return
  }
  memberSubmitting.value = true
  try {
    await http('/users', {
      method: 'POST',
      body: JSON.stringify({
        username: newMember.username.trim(),
        password: newMember.password.trim(),
        role: 'member',
        allowDownload: false,
      }),
    })
    toast.success('成员已创建')
    newMember.username = ''
    newMember.password = ''
    await loadMembers()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '创建成员失败')
  } finally {
    memberSubmitting.value = false
  }
}

const handleToggleDownload = async (u: UserInfo) => {
  memberSubmitting.value = true
  try {
    await http(`/users/${u.id}`, {
      method: 'PUT',
      body: JSON.stringify({ allowDownload: !u.allowDownload }),
    })
    toast.success('下载许可已更新')
    await loadMembers()
    // 若修改的是当前用户，同步刷新本地会话信息
    if (u.id === userStore.user?.id) {
      await userStore.fetchMe()
    }
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '更新失败')
  } finally {
    memberSubmitting.value = false
  }
}

const handleResetPwd = async (u: UserInfo) => {
  const newPassword = await modal.prompt(`为「${u.username}」设置新密码：`, '', '重置密码')
  if (!newPassword || !String(newPassword).trim()) return
  memberSubmitting.value = true
  try {
    await http(`/users/${u.id}/password`, {
      method: 'PUT',
      body: JSON.stringify({ password: String(newPassword).trim() }),
    })
    toast.success('密码已重置')
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '重置密码失败')
  } finally {
    memberSubmitting.value = false
  }
}

const handleRename = async (u: UserInfo) => {
  const newName = await modal.prompt(`为「${u.username}」输入新用户名：`, u.username, '修改用户名')
  if (!newName || !newName.trim()) return
  const name = newName.trim()
  if (name === u.username) return
  memberSubmitting.value = true
  try {
    await http(`/users/${u.id}`, {
      method: 'PUT',
      body: JSON.stringify({ username: name }),
    })
    toast.success('用户名已更新')
    await loadMembers()
    // 若修改的是当前用户，同步刷新本地会话信息
    if (u.id === userStore.user?.id) {
      await userStore.fetchMe()
    }
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '修改用户名失败')
  } finally {
    memberSubmitting.value = false
  }
}

// 修改当前登录账号的用户名 / 密码（管理员）
const handleRenameCurrent = () => {
  const u = userStore.user
  if (u) handleRename(u)
}

const handleResetPwdCurrent = () => {
  const u = userStore.user
  if (u) handleResetPwd(u)
}

const handleDeleteUser = async (u: UserInfo) => {
  const confirmed = await modal.confirm(
    `确定删除成员「${u.username}」吗？其书架、历史、评分等个人数据将一并清除。`,
  )
  if (!confirmed) return
  memberSubmitting.value = true
  try {
    await http(`/users/${u.id}`, { method: 'DELETE' })
    toast.success('成员已删除')
    await loadMembers()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '删除失败')
  } finally {
    memberSubmitting.value = false
  }
}

// ── 服务器 ──
const serverForm = reactive({ bindHost: '0.0.0.0', port: 8081, historyLimit: 200 })
const serverSubmitting = ref(false)

const loadServer = async () => {
  try {
    const data = await http<{
      setting: { bindHost: string; port: number; historyLimit: number }
    }>('/server/setting')
    serverForm.bindHost = data.setting.bindHost
    serverForm.port = data.setting.port
    serverForm.historyLimit = data.setting.historyLimit
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载服务器配置失败')
  }
}

const handleSaveServer = async () => {
  serverSubmitting.value = true
  try {
    await http('/server/setting', {
      method: 'POST',
      body: JSON.stringify({ ...serverForm }),
    })
    toast.success('服务器配置已保存，重启服务后生效')
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    serverSubmitting.value = false
  }
}

onMounted(() => {
  loadAccountSettings()
  if (userStore.isAdmin) {
    loadMembers()
    loadServer()
  }
})
</script>

<style scoped>
.account-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* ── 分区标题（管理员区块）── */
.section-title {
  font-size: 13px;
  font-weight: 600;
  color: #ff7588;
  letter-spacing: 0.5px;
  margin: 12px 0 4px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--app-border-2);
}

.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background-color: var(--app-surface-2);
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
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
  color: var(--app-text-3);
}

.setting-row.clickable {
  cursor: pointer;
  transition:
    background-color 0.2s ease,
    border-color 0.2s ease;
}

.setting-row.clickable:hover {
  background-color: var(--app-surface-2-hover);
}

.setting-row.danger:hover {
  background-color: rgba(255, 117, 136, 0.08);
  border-color: rgba(255, 117, 136, 0.3);
}

.row-label {
  color: var(--app-text-2);
}

.row-value.highlight {
  color: #ff7588;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 8px;
}

.row-value.hash {
  font-family: 'Courier New', Consolas, monospace;
  font-size: 12px;
  color: var(--app-text-2);
  word-break: break-all;
  text-align: right;
}

.badge {
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 4px;
  background-color: var(--app-surface-3);
  color: var(--app-text-2);
}

.badge.ex {
  background-color: rgba(255, 117, 136, 0.2);
  color: #ff7588;
  border: 1px solid rgba(255, 117, 136, 0.4);
}

.arrow-icon {
  color: var(--app-text-muted);
  font-size: 18px;
}

.logout-icon {
  font-size: 16px;
}

.action-btn {
  padding: 6px 14px;
  border-radius: 6px;
  border: 1px solid var(--app-border-3);
  background: var(--app-surface-3);
  color: var(--app-text-2);
  cursor: pointer;
  font-size: 13px;
  transition: all 0.2s;
}

.action-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.action-btn:hover:not(:disabled) {
  background: var(--app-surface-3-hover);
}

.action-btn.primary {
  background: #ff7588;
  border-color: #ff7588;
  color: #fff;
}

.action-btn.primary:hover:not(:disabled) {
  background: #f06477;
}

.action-btn.refresh {
  border-color: #007acc;
  color: #5cb8ff;
}

.action-btn.refresh:hover:not(:disabled) {
  background: rgba(0, 122, 204, 0.15);
}

/* ── 成员管理 ── */
.member-form {
  display: flex;
  gap: 8px;
  align-items: center;
}

.text-input {
  flex: 1;
  background: var(--app-input-bg);
  border: 1px solid var(--app-border-2);
  border-radius: 6px;
  padding: 8px 10px;
  color: var(--app-text-2);
  font-size: 13px;
  outline: none;
  font-family: inherit;
}

.text-input:focus {
  border-color: #ff7588;
}

.member-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.member-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background-color: var(--app-surface-2);
  border-radius: 8px;
  border: 1px solid var(--app-border-2);
  gap: 12px;
}

.member-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.member-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--app-text-strong);
  display: flex;
  align-items: center;
  gap: 6px;
}

.self-tag {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  background-color: rgba(255, 117, 136, 0.2);
  color: #ff7588;
}

.member-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--app-text-3);
}

.member-actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.mini-btn {
  padding: 5px 10px;
  border-radius: 6px;
  border: 1px solid var(--app-border-3);
  background: var(--app-surface-3);
  color: var(--app-text-2);
  cursor: pointer;
  font-size: 12px;
  transition: all 0.2s;
  white-space: nowrap;
}

.mini-btn:hover:not(:disabled) {
  background: var(--app-surface-3-hover);
  color: var(--app-text-strong);
}

.mini-btn.danger:hover:not(:disabled) {
  background: rgba(255, 117, 136, 0.1);
  border-color: rgba(255, 117, 136, 0.4);
  color: #ff7588;
}

.mini-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.empty-tip {
  text-align: center;
  color: var(--app-text-muted);
  font-size: 13px;
  padding: 12px 0;
}

/* ── 服务器 ── */
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
  width: 40px;
  text-align: center;
  outline: none;
}

.setting-input.wide {
  width: 70px;
}

.setting-input:focus {
  border-bottom-color: #ff7588;
}

.unit {
  font-size: 13px;
  color: var(--app-text-3);
}

.reset-row {
  margin-top: 8px;
}

.reset-btn {
  width: 100%;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 10px 16px;
  border-radius: 8px;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.reset-btn:hover:not(:disabled) {
  background-color: var(--app-surface-3-hover);
  border-color: #ff7588;
  color: #ff7588;
}

.reset-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* ── Cookie 弹窗 ── */
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
  background: var(--app-surface-2);
  border: 1px solid var(--app-border-2);
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
  color: var(--app-text-strong);
  font-size: 16px;
}

/* ── 方式一：账号密码登录 ── */
.login-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  background-color: rgba(0, 122, 204, 0.06);
  border: 1px solid rgba(0, 122, 204, 0.3);
  border-radius: 8px;
}

.login-mode-title {
  font-size: 13px;
  font-weight: 600;
  color: #5cb8ff;
}

.login-mode-tip {
  font-size: 12px;
  color: var(--app-text-3);
}

.text-input.full {
  width: 100%;
  box-sizing: border-box;
}

.full-btn {
  width: 100%;
  padding: 8px 0;
}

.login-divider {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--app-text-muted);
  font-size: 12px;
}

.login-divider::before,
.login-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background-color: var(--app-border-3);
}

.modal-tip {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-3);
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-group label {
  font-size: 12px;
  color: var(--app-text-2);
}

.required {
  color: #ff7588;
}

.form-group input,
.form-group textarea {
  background: var(--app-input-bg);
  border: 1px solid var(--app-border-2);
  border-radius: 6px;
  padding: 8px 10px;
  color: var(--app-text-2);
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
