<template>
  <div class="about-settings">
    <!-- Round29：版本行集成 GitHub 最新 release 检测（红点提醒 + 查看新版 + 手动检查更新） -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title-line">
          <span class="item-title">版本</span>
          <span v-if="newVersionAvailable" class="red-dot" title="GitHub 发布新版本"></span>
        </div>
        <div class="item-subtext">{{ version }}</div>
        <div v-if="newVersionAvailable" class="new-ver-banner">
          <span class="banner-icon">🔔</span>
          <span>发现新版本 <b>v{{ info?.latest }}</b>，点击「查看新版」前往下载（升级仅需替换 SakuManga.exe）</span>
        </div>
        <div v-else-if="checkFailed" class="check-fail">
          <span class="banner-icon">⚠️</span>
          <span>版本检测失败：{{ checkFailed }}（可点击「检查更新」重试）</span>
        </div>
      </div>
      <div class="version-actions">
        <button
          v-if="newVersionAvailable"
          class="update-btn"
          title="在浏览器打开 GitHub Release 页面查看并下载新版"
          @click="openLatestRelease"
        >
          ⬇️ 查看新版
        </button>
        <button class="check-btn" :disabled="checking" @click="manualCheck">
          {{ checking ? '⏳ 检查中…' : '🔄 检查更新' }}
        </button>
      </div>
    </div>

    <!-- Round24：服务端构建标识（部署后新旧核对；旧版后端无该接口显示 —） -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">服务端构建</div>
        <div class="item-subtext">{{ serverBuild || '—' }}</div>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">创作者</div>
        <div class="item-subtext">珱垣</div>
      </div>
    </div>

    <div
      class="setting-item clickable"
      @click="handleOpenLink('https://github.com/yeonalbus/SakuManga')"
    >
      <div class="item-info">
        <div class="item-title">Github</div>
        <div class="item-subtext link-text">https://github.com/yeonalbus/SakuManga</div>
      </div>
    </div>

    <!-- Round25：视口诊断由侧边栏菜单移入关于页（仅管理员可见，iPad PWA 底部条排查用） -->
    <div v-if="userStore.isAdmin" class="setting-item clickable" @click="goDiag">
      <div class="item-info">
        <div class="item-title">视口诊断</div>
        <div class="item-subtext">查看当前视口 / 设备信息，排查 iPad PWA 底部条问题</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { version } from '../../../package.json'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import { useUserStore } from '@/stores/userStore'
import { useVersionCheck } from '@/composables/useVersionCheck'

const { toast } = useUI()
const router = useRouter()
const userStore = useUserStore()

// Round29：GitHub 最新 release 版本检测（与 SettingsView 侧栏红点共享模块级状态）
const { checking, info, checkVersion } = useVersionCheck()
const newVersionAvailable = computed(() => !!info.value?.ok && !!info.value?.hasUpdate)
const checkFailed = computed(() => (info.value && !info.value.ok ? info.value.error || '未知原因' : ''))
onMounted(() => {
  // SettingsView 已触发过则直接命中缓存/去重，不产生额外请求
  checkVersion()
})

// 手动「检查更新」：强制绕过 6h 缓存重新检测，并按结果提示
const manualCheck = async () => {
  const r = await checkVersion(true)
  if (!r.ok) {
    toast.error(r.error || '版本检测失败，请检查网络后重试')
    return
  }
  if (r.hasUpdate) {
    toast.info(`发现新版本 v${r.latest}（当前 v${version}），已打开下载页面`)
    openLatestRelease()
  } else {
    toast.success(`已是最新版本 v${version}`)
  }
}

// 打开最新 release 页面（浏览器下载 SakuManga.exe 替换升级）
const openLatestRelease = () => {
  const url = info.value?.url || 'https://github.com/yeonalbus/SakuManga/releases/latest'
  window.open(url, '_blank')
}

// Round25：视口诊断（管理员专用，由侧边栏菜单移入）
const goDiag = () => {
  router.push('/diag')
}

// Round24：服务端构建标识（部署后新旧核对；旧版后端无该接口 → 保持 —）
const serverBuild = ref('')
onMounted(async () => {
  try {
    const res = await http<{ build?: string }>('/system/version')
    if (res?.build) serverBuild.value = res.build
  } catch {
    serverBuild.value = ''
  }
})

const handleOpenLink = (url: string) => {
  window.open(url, '_blank')
  toast.info(`已尝试在浏览器打开：${url}`)
}
</script>

<style scoped>
.about-settings {
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

/* ── Round29：版本更新检测样式 ── */
.item-title-line {
  display: flex;
  align-items: center;
  gap: 6px;
}

.red-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #ff4d4f;
  box-shadow: 0 0 0 2px rgba(255, 77, 79, 0.22);
  flex-shrink: 0;
}

.new-ver-banner {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-top: 6px;
  padding: 6px 10px;
  background-color: rgba(255, 193, 7, 0.1);
  border: 1px solid rgba(255, 193, 7, 0.35);
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: #ffd54f;
}
.new-ver-banner b {
  color: #ff7588;
}
.banner-icon {
  flex-shrink: 0;
}

.check-fail {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-top: 6px;
  padding: 6px 10px;
  background-color: rgba(255, 117, 136, 0.08);
  border: 1px solid rgba(255, 117, 136, 0.3);
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: #ff9aa8;
}

.version-actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex-shrink: 0;
}

.update-btn {
  background-color: #ff7588;
  border: none;
  color: #fff;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  transition: opacity 0.2s;
}
.update-btn:hover {
  opacity: 0.85;
}

.check-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 0.8rem;
  cursor: pointer;
  white-space: nowrap;
  transition:
    border-color 0.2s,
    color 0.2s;
}
.check-btn:hover:not(:disabled) {
  border-color: #007acc;
  color: #007acc;
}
.check-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.item-desc {
  font-size: 12px;
  color: var(--app-text-3);
}

.item-subtext {
  font-size: 13px;
  color: var(--app-text-2);
  line-height: 1.4;
}

.link-text {
  color: #ff7588;
  word-break: break-all;
}

.arrow-icon {
  font-size: 20px;
  color: var(--app-text-muted);
  margin-left: 8px;
}
</style>
