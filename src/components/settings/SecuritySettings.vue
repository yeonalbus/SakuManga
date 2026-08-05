<template>
  <div class="security-settings">
    <!-- ── 账户 ── -->
    <div class="section-title">👤 账户</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">当前用户</div>
        <div class="item-subtext">
          {{ userStore.user?.username }} · {{ userStore.isAdmin ? '管理员' : '成员' }}
        </div>
      </div>
      <span class="badge" :class="{ ex: userStore.user?.allowDownload || userStore.isAdmin }">
        {{
          userStore.isAdmin
            ? '管理员 · 下载不限'
            : userStore.user?.allowDownload
              ? '允许下载'
              : '禁止下载'
        }}
      </span>
    </div>

    <div v-if="userStore.isAdmin" class="setting-item">
      <div class="item-info">
        <div class="item-title">账户操作</div>
        <div class="item-subtext">修改当前账号的用户名或密码（管理员）</div>
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

    <div class="setting-item clickable danger" @click="handleLogout">
      <div class="item-info">
        <div class="item-title">退出登录</div>
        <div class="item-subtext">结束当前会话并返回登录页</div>
      </div>
      <span class="logout-icon">🚪</span>
    </div>

    <!-- ── 成员管理（仅管理员）── -->
    <template v-if="userStore.isAdmin">
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
    </template>

    <!-- ── 服务器（仅管理员）── -->
    <template v-if="userStore.isAdmin">
      <div class="section-title">🖥️ 服务器</div>

      <div class="setting-item">
        <div class="item-info">
          <div class="item-title">监听地址</div>
          <div class="item-subtext">127.0.0.1 仅本机可访问；0.0.0.0 允许局域网 / 公网访问</div>
        </div>
        <select v-model="serverForm.bindHost" class="setting-select">
          <option value="0.0.0.0">0.0.0.0（所有网卡）</option>
          <option value="127.0.0.1">127.0.0.1（仅本机）</option>
        </select>
      </div>

      <div class="setting-item">
        <div class="item-info">
          <div class="item-title">监听端口</div>
          <div class="item-subtext">修改后重启服务生效</div>
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

      <div class="setting-item">
        <div class="item-info">
          <div class="item-title">每用户历史记录上限</div>
          <div class="item-subtext">超出上限自动淘汰最旧记录</div>
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

// ── 账户 ──
const handleLogout = async () => {
  const confirmed = await modal.confirm('确定要退出登录吗？')
  if (!confirmed) return
  await userStore.logout()
  router.replace('/login')
}

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
  if (userStore.isAdmin) {
    loadMembers()
    loadServer()
  }
})
</script>

<style scoped>
.security-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
  color: #ff7588;
  letter-spacing: 0.5px;
  margin: 12px 0 4px;
  padding-bottom: 6px;
  border-bottom: 1px solid #26262a;
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

.setting-item.danger:hover {
  background-color: rgba(255, 117, 136, 0.08);
  border-color: rgba(255, 117, 136, 0.3);
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

.logout-icon {
  font-size: 18px;
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

/* 成员表单 */
.member-form {
  display: flex;
  gap: 8px;
  align-items: center;
}

.text-input {
  flex: 1;
  background: #121214;
  border: 1px solid #26262a;
  border-radius: 6px;
  padding: 8px 10px;
  color: #eee;
  font-size: 13px;
  outline: none;
  font-family: inherit;
}

.text-input:focus {
  border-color: #ff7588;
}

.action-btn {
  padding: 8px 14px;
  border-radius: 6px;
  border: 1px solid #36363a;
  background: #26262a;
  color: #eee;
  cursor: pointer;
  font-size: 13px;
  transition: all 0.2s;
  white-space: nowrap;
}

.action-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.action-btn.primary {
  background: #ff7588;
  border-color: #ff7588;
  color: #fff;
}

.action-btn.primary:hover:not(:disabled) {
  background: #f06477;
}

/* 成员列表 */
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
  background-color: #1a1a1e;
  border-radius: 8px;
  border: 1px solid #26262a;
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
  color: #fff;
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
  color: #88888c;
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
  border: 1px solid #36363a;
  background: #26262a;
  color: #ccc;
  cursor: pointer;
  font-size: 12px;
  transition: all 0.2s;
  white-space: nowrap;
}

.mini-btn:hover:not(:disabled) {
  background: #323236;
  color: #fff;
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
  color: #666;
  font-size: 13px;
  padding: 12px 0;
}

/* 下拉 / 输入 */
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
  color: #88888c;
}

/* 保存按钮 */
.reset-row {
  margin-top: 8px;
}

.reset-btn {
  width: 100%;
  background: #242428;
  border: 1px solid #3a3a3d;
  color: #ccc;
  padding: 10px 16px;
  border-radius: 8px;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.reset-btn:hover:not(:disabled) {
  background-color: #2e2e33;
  border-color: #ff7588;
  color: #ff7588;
}

.reset-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
