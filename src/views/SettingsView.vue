<template>
  <div class="settings-container">
    <div class="settings-sidebar">
      <div class="sidebar-header">
        <h2>设置</h2>
      </div>
      <div class="menu-list">
        <div
          v-for="item in menuItems"
          :key="item.id"
          class="menu-item"
          :class="{ active: activeTab === item.id }"
          @click="activeTab = item.id"
        >
          <span class="item-icon">{{ item.icon }}</span>
          <span class="item-label">{{ item.label }}</span>
        </div>
      </div>
    </div>

    <div class="settings-content">
      <div class="panel-section">
        <div class="panel-header">
          <button class="back-btn" @click="handleBack" title="返回上一页">←</button>
          <h3>{{ currentTabTitle }}</h3>
        </div>

        <div class="panel-body">
          <AccountSettings v-if="activeTab === 'account'" />
          <EHSettings v-else-if="activeTab === 'eh'" />
          <StyleSettings v-else-if="activeTab === 'style'" />
          <ReaderSettings v-else-if="activeTab === 'reader'" />
          <PreferenceSettings v-else-if="activeTab === 'preference'" />
          <NetworkSettings v-else-if="activeTab === 'network'" />
          <DownloadSettings v-else-if="activeTab === 'download'" />
          <TagMaintainSettings v-else-if="activeTab === 'tag-maintain'" />
          <AdvancedSettings v-else-if="activeTab === 'advanced'" />
          <SecuritySettings v-else-if="activeTab === 'security'" />
          <AboutSettings v-else-if="activeTab === 'about'" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUserStore } from '@/stores/userStore'

// 导入全部 11 个设置子组件
import AccountSettings from '@/components/settings/AccountSettings.vue'
import EHSettings from '@/components/settings/EHSettings.vue'
import StyleSettings from '@/components/settings/StyleSettings.vue'
import ReaderSettings from '@/components/settings/ReaderSettings.vue'
import PreferenceSettings from '@/components/settings/PreferenceSettings.vue'
import NetworkSettings from '@/components/settings/NetworkSettings.vue'
import DownloadSettings from '@/components/settings/DownloadSettings.vue'
import TagMaintainSettings from '@/components/settings/TagMaintainSettings.vue'
import AdvancedSettings from '@/components/settings/AdvancedSettings.vue'
import SecuritySettings from '@/components/settings/SecuritySettings.vue'
import AboutSettings from '@/components/settings/AboutSettings.vue'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

// 全部栏目定义；adminOnly=true 的栏目（网络/下载/Tag维护/高级/安全）仅管理员可见
const allMenuItems = [
  { id: 'account', label: '账户', icon: '👤', title: '账户设置', adminOnly: false },
  { id: 'eh', label: 'EH', icon: '😄', title: 'EH 网站设置', adminOnly: false },
  { id: 'style', label: '样式', icon: '🎨', title: '样式设置', adminOnly: false },
  { id: 'reader', label: '阅读', icon: '📖', title: '阅读设置', adminOnly: false },
  { id: 'preference', label: '偏好', icon: '⭐', title: '偏好设置', adminOnly: false },
  { id: 'network', label: '网络', icon: '📶', title: '网络设置', adminOnly: true },
  { id: 'download', label: '下载', icon: '📥', title: '下载设置', adminOnly: true },
  { id: 'tag-maintain', label: 'Tag 维护', icon: '🏷️', title: 'Tag 维护', adminOnly: true },
  { id: 'advanced', label: '高级', icon: '⚙️', title: '高级设置', adminOnly: true },
  { id: 'security', label: '安全', icon: '🛡️', title: '安全设置', adminOnly: true },
  { id: 'about', label: '关于', icon: 'ℹ️', title: '关于软件', adminOnly: false },
]

// 按角色过滤菜单
const menuItems = computed(() =>
  allMenuItems.filter((item) => !item.adminOnly || userStore.isAdmin),
)

// 校验 tab 是否对当前角色可见
const isTabAllowed = (tab: string) => {
  const item = allMenuItems.find((i) => i.id === tab)
  return !!item && (!item.adminOnly || userStore.isAdmin)
}

// 支持侧边栏快捷入口：/settings?tab=xxx 直达对应设置栏目（越权栏目回退到账户）
const initialTab = (route.query.tab as string) || 'account'
const activeTab = ref(isTabAllowed(initialTab) ? initialTab : 'account')

// 已停留在设置页时，侧边栏再次点击同 URL（仅 query 变化）也能实时切换
watch(
  () => route.query.tab,
  (tab) => {
    if (tab && typeof tab === 'string') {
      activeTab.value = isTabAllowed(tab) ? tab : 'account'
    }
  },
)

// 用户信息异步加载完成后重新校验当前栏目
// （刷新页面直达 /settings?tab=xxx 时 user 尚为 null，isAdmin 不可判定，需在此恢复/回退）
watch(
  () => userStore.user,
  (u) => {
    if (!u) return
    if (!isTabAllowed(activeTab.value)) {
      activeTab.value = isTabAllowed(initialTab) ? initialTab : 'account'
    }
  },
)

const currentTabTitle = computed(() => {
  const current = menuItems.value.find((item) => item.id === activeTab.value)
  return current ? current.title : '设置'
})

const handleBack = () => {
  if (window.history.length > 1) {
    router.back()
  } else {
    router.push('/online/home')
  }
}
</script>

<style scoped>
.settings-container {
  display: flex;
  width: 100%;
  height: 100%;
  background-color: #121214;
  color: #e0e0e0;
  box-sizing: border-box;
}

.settings-sidebar {
  width: 220px;
  background-color: #18181c;
  border-right: 1px solid #2a2a2d;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.sidebar-header {
  padding: 20px 24px 12px;
}

.sidebar-header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #ffffff;
}

.menu-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  border-radius: 6px;
  cursor: pointer;
  color: #a0a0a0;
  font-size: 14px;
  transition: all 0.2s ease;
  margin-bottom: 2px;
}

.menu-item:hover {
  background-color: #26262a;
  color: #ffffff;
}

.menu-item.active {
  background-color: #2b2b30;
  color: #ffffff;
  font-weight: 500;
}

.item-icon {
  font-size: 16px;
  width: 20px;
  text-align: center;
}

.settings-content {
  flex: 1;
  background-color: #121214;
  overflow-y: auto;
  padding: 24px 32px;
}

.panel-section {
  max-width: 800px;
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 16px;
  border-bottom: 1px solid #26262a;
  margin-bottom: 20px;
}

.panel-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: #ffffff;
  flex: 1;
}

.back-btn {
  background: transparent;
  border: none;
  color: #a0a0a0;
  font-size: 18px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 4px;
  transition:
    color 0.2s,
    background-color 0.2s;
}

.back-btn:hover {
  color: #ffffff;
  background-color: #26262a;
}

.panel-body {
  display: flex;
  flex-direction: column;
}
</style>
