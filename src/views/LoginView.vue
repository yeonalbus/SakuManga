<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-logo">E-Manager</div>
      <div class="login-title">登录</div>
      <div class="login-subtitle">请使用管理员分配的账号登录</div>

      <form @submit.prevent="handleLogin">
        <div class="form-field">
          <label for="login-username">用户名</label>
          <input
            id="login-username"
            v-model="username"
            type="text"
            autocomplete="username"
            placeholder="请输入用户名"
            autofocus
          />
        </div>
        <div class="form-field">
          <label for="login-password">密码</label>
          <input
            id="login-password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            placeholder="请输入密码"
          />
        </div>

        <div v-if="errorMsg" class="error-msg">{{ errorMsg }}</div>

        <button type="submit" class="login-btn" :disabled="loading">
          {{ loading ? '登录中…' : '登 录' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUserStore } from '@/stores/userStore'
import { loadUserLibrary } from '@/stores/libraryInit'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const username = ref('')
const password = ref('')
const loading = ref(false)
const errorMsg = ref('')

async function handleLogin() {
  if (!username.value.trim() || !password.value) {
    errorMsg.value = '请输入用户名和密码'
    return
  }
  loading.value = true
  errorMsg.value = ''
  try {
    await userStore.login(username.value.trim(), password.value)
    // 登录成功后加载当前用户的库数据（书架/历史/阅读清单/评分 + 旧数据迁移）
    loadUserLibrary()
    const redirect =
      typeof route.query.redirect === 'string' ? route.query.redirect : '/online/home'
    await router.replace(redirect)
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : '登录失败，请检查用户名和密码'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  height: 100vh;
  width: 100vw;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--app-bg);
  padding: 20px;
}

.login-card {
  width: 360px;
  max-width: 100%;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 12px;
  padding: 36px 32px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.35);
}

.login-logo {
  font-size: 1.5rem;
  font-weight: bold;
  color: var(--app-accent);
  text-align: center;
  margin-bottom: 4px;
}

.login-title {
  font-size: 1.15rem;
  font-weight: 600;
  color: var(--app-fg);
  text-align: center;
  margin-top: 12px;
}

.login-subtitle {
  font-size: 0.85rem;
  color: #888;
  text-align: center;
  margin: 6px 0 24px;
}

.form-field {
  margin-bottom: 16px;
}

.form-field label {
  display: block;
  font-size: 0.82rem;
  color: var(--app-fg);
  margin-bottom: 6px;
}

.form-field input {
  width: 100%;
  padding: 10px 12px;
  font-size: 0.9rem;
  color: var(--app-fg);
  background: var(--app-bg);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  outline: none;
  transition: border-color 0.2s;
}

.form-field input:focus {
  border-color: var(--app-accent);
}

.error-msg {
  background: rgba(255, 77, 79, 0.12);
  color: #ff4d4f;
  font-size: 0.82rem;
  padding: 8px 12px;
  border-radius: 6px;
  margin-bottom: 16px;
}

.login-btn {
  width: 100%;
  padding: 11px 0;
  font-size: 0.95rem;
  font-weight: 600;
  color: #fff;
  background: var(--app-accent);
  border: none;
  border-radius: 8px;
  cursor: pointer;
  transition: opacity 0.2s;
}

.login-btn:hover {
  opacity: 0.9;
}

.login-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
