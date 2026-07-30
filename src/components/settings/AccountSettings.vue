<template>
  <div class="account-settings">
    <div class="setting-row">
      <span class="row-label">您已登录:</span>
      <span class="row-value highlight">E站用户</span>
    </div>

    <div class="setting-row clickable" @click="handleCheckCookies">
      <span class="row-label">查看 Cookie</span>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-row clickable danger" @click="handleLogout">
      <span class="row-label">退出当前账号</span>
      <span class="logout-icon">🚪</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useUI } from '@/composables/useUI'

const { toast, modal } = useUI()

// 查看凭证 Cookies
const handleCheckCookies = async () => {
  await modal.alert('当前凭证 (Cookies):\nipb_member_id=xxxx;\nipb_pass_hash=xxxx;\nigneous=xxxx;')
}

// 退出登录
const handleLogout = async () => {
  const confirmed = await modal.confirm('确定要退出登录并清除账户凭证吗？')
  if (confirmed) {
    toast.success('已成功退出登录！')
  }
}
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
}

.arrow-icon {
  color: #66666c;
  font-size: 18px;
}

.logout-icon {
  font-size: 16px;
}
</style>
