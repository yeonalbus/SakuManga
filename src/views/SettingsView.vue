<template>
  <div class="settings-container">
    <div class="settings-sidebar">
      <div class="sidebar-header">
        <h2>设置</h2>
      </div>
      <div class="menu-list">
        <template v-for="group in menuGroups" :key="group.title">
          <div class="menu-group-title">{{ group.title }}</div>
          <div
            v-for="item in group.items"
            :key="item.id"
            class="menu-item"
            :class="{ active: activeTab === item.id }"
            @click="handleMenuClick(item)"
          >
            <span class="item-icon">{{ item.icon }}</span>
            <span class="item-label">{{ item.label }}</span>
            <!-- Round29：关于软件入口红点——GitHub 有新版 release 时提醒 -->
            <span
              v-if="item.id === 'about' && newVersionAvailable"
              class="item-dot"
              title="发现新版本"
            ></span>
          </div>
        </template>
      </div>
    </div>

    <div class="settings-content">
      <div class="panel-section">
        <div class="panel-header">
          <button class="back-btn" @click="handleBack" title="返回上一页">←</button>
          <h3>{{ currentTabTitle }}</h3>
        </div>

        <div class="panel-body">
          <!-- Profile 由 EH 网站面板内部进入；我的标签独立组件含返回按钮，@back 回到所在分组默认项 -->
          <AccountSettings v-if="activeTab === 'account'" />
          <MyTagsSettings v-else-if="activeTab === 'my-tags'" @back="backToGroup('my-tags')" />
          <EHSettings v-else-if="activeTab === 'eh'" />
          <StyleSettings v-else-if="activeTab === 'style'" />
          <ReaderSettings v-else-if="activeTab === 'reader'" />
          <PreferenceSettings v-else-if="activeTab === 'preference'" />
          <NetworkSettings v-else-if="activeTab === 'network'" />
          <DownloadSettings v-else-if="activeTab === 'download'" />
          <DedupSettings v-else-if="activeTab === 'dedup'" />
          <TagMaintainSettings v-else-if="activeTab === 'tag-maintain'" />
          <UpdateScanSettings v-else-if="activeTab === 'update-scan'" />
          <LogSettings v-else-if="activeTab === 'logs'" />
          <AboutSettings v-else-if="activeTab === 'about'" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUserStore } from '@/stores/userStore'
import { useVersionCheck } from '@/composables/useVersionCheck'

// 导入全部设置子组件（Round25 收敛：Profile 并入 EH 网站、安全并入账户、高级并入日志）
import AccountSettings from '@/components/settings/AccountSettings.vue'
import EHSettings from '@/components/settings/EHSettings.vue'
import StyleSettings from '@/components/settings/StyleSettings.vue'
import ReaderSettings from '@/components/settings/ReaderSettings.vue'
import PreferenceSettings from '@/components/settings/PreferenceSettings.vue'
import NetworkSettings from '@/components/settings/NetworkSettings.vue'
import DownloadSettings from '@/components/settings/DownloadSettings.vue'
import TagMaintainSettings from '@/components/settings/TagMaintainSettings.vue'
import MyTagsSettings from '@/components/settings/MyTagsSettings.vue'
import UpdateScanSettings from '@/components/settings/UpdateScanSettings.vue'
import DedupSettings from '@/components/settings/DedupSettings.vue'
import LogSettings from '@/components/settings/LogSettings.vue'
import AboutSettings from '@/components/settings/AboutSettings.vue'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

// Round29：GitHub 最新 release 检测（打开设置页时触发一次；6h 缓存，见 useVersionCheck）
const { info, checkVersion } = useVersionCheck()
const newVersionAvailable = computed(() => !!info.value?.ok && !!info.value?.hasUpdate)
onMounted(() => {
  checkVersion()
})

// 菜单项定义
interface SettingsMenuItem {
  id: string
  label: string
  icon: string
  title: string // 面板标题
  adminOnly?: boolean // true=仅管理员可见（网络/下载/更新扫描/Tag维护/日志）
  href?: string // 🔍 存在时点击跳转路由（如 /diag），而非切换 tab
}

// 按主题分组的菜单（Round25 收敛：安全并入账户、Profile 移入 EH 网站、高级并入日志、诊断移入关于）
interface SettingsMenuGroup {
  title: string
  items: SettingsMenuItem[]
}

const allGroups: SettingsMenuGroup[] = [
  {
    title: '账户',
    items: [
      { id: 'account', label: '账户', icon: '👤', title: '账户设置' },
    ],
  },
  {
    title: 'E 站连接',
    items: [
      { id: 'eh', label: 'EH 网站', icon: '😄', title: 'EH 网站设置' },
      { id: 'network', label: '网络', icon: '📶', title: '网络设置', adminOnly: true },
    ],
  },
  {
    title: '阅读体验',
    items: [
      { id: 'style', label: '样式', icon: '🎨', title: '样式设置' },
      { id: 'reader', label: '阅读', icon: '📖', title: '阅读设置' },
      { id: 'preference', label: '偏好', icon: '⭐', title: '偏好设置' },
    ],
  },
  {
    title: '下载与离线',
    items: [
      { id: 'download', label: '下载', icon: '📥', title: '下载设置', adminOnly: true },
      { id: 'update-scan', label: '更新扫描', icon: '🔄', title: '更新扫描', adminOnly: true },
      { id: 'dedup', label: '维护查重', icon: '🔎', title: '维护查重设置', adminOnly: true },
    ],
  },
  {
    title: '标签管理',
    items: [
      { id: 'my-tags', label: '我的标签', icon: '🏷️', title: '我的标签' },
      { id: 'tag-maintain', label: 'Tag 维护', icon: '🧹', title: 'Tag 维护', adminOnly: true },
    ],
  },
  {
    title: '日志',
    items: [
      { id: 'logs', label: '日志', icon: '📜', title: '日志', adminOnly: true },
    ],
  },
  {
    title: '关于',
    items: [
      { id: 'about', label: '关于软件', icon: 'ℹ️', title: '关于软件' },
    ],
  },
]

// 全部菜单项扁平化（供权限校验与标题查找）
const allMenuItems = allGroups.flatMap((g) => g.items)

// 校验 tab 是否对当前角色可见
const isTabAllowed = (tab: string) => {
  const item = allMenuItems.find((i) => i.id === tab)
  return !!item && (!item.adminOnly || userStore.isAdmin)
}

// 按角色过滤后的分组（空分组不展示）
const menuGroups = computed(() =>
  allGroups
    .map((g) => ({ ...g, items: g.items.filter((i) => !i.adminOnly || userStore.isAdmin) }))
    .filter((g) => g.items.length > 0),
)

// 支持侧边栏快捷入口：/settings?tab=xxx 直达对应设置栏目（越权栏目回退到账户）
const initialTab = (route.query.tab as string) || 'account'
const activeTab = ref(isTabAllowed(initialTab) ? initialTab : 'account')

// 🔍 菜单点击：带 href 的项（如视口诊断）跳转路由，否则切换 tab
const handleMenuClick = (item: SettingsMenuItem & { href?: string }) => {
  if (item.href) {
    router.push(item.href)
    return
  }
  activeTab.value = item.id
}

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

// Profile / 我的标签 组件内部返回按钮：回到所在分组的第一个可访问项（如 profile→account、my-tags→tag-maintain）
const backToGroup = (fromId: string) => {
  const group = allGroups.find((g) => g.items.some((i) => i.id === fromId))
  if (group) {
    const target = group.items.find(
      (i) => i.id !== fromId && (!i.adminOnly || userStore.isAdmin),
    )
    if (target) {
      activeTab.value = target.id
      return
    }
  }
  activeTab.value = 'account'
}

const currentTabTitle = computed(() => {
  const current = allMenuItems.find((item) => item.id === activeTab.value)
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
  background-color: var(--app-bg-alt);
  color: var(--app-fg);
  box-sizing: border-box;
}

.settings-sidebar {
  width: 220px;
  background-color: var(--app-bg-deep);
  border-right: 1px solid var(--app-border-2);
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
  color: var(--app-text-strong);
}

.menu-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

/* 分组标题 */
.menu-group-title {
  padding: 12px 16px 4px;
  font-size: 11px;
  font-weight: 600;
  color: var(--app-text-muted);
  letter-spacing: 0.6px;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--app-text-2);
  font-size: 14px;
  transition: all 0.2s ease;
  margin-bottom: 2px;
}

.menu-item:hover {
  background-color: var(--app-surface-2-hover);
  color: var(--app-text-strong);
}

.menu-item.active {
  background-color: var(--app-surface-3);
  color: var(--app-text-strong);
  font-weight: 500;
}

.item-icon {
  font-size: 16px;
  width: 20px;
  text-align: center;
}

/* Round29：关于软件入口「发现新版本」红点徽标 */
.item-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #ff4d4f;
  box-shadow: 0 0 0 2px rgba(255, 77, 79, 0.22);
  flex-shrink: 0;
  margin-left: auto;
}

.settings-content {
  flex: 1;
  background-color: var(--app-bg-alt);
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
  border-bottom: 1px solid var(--app-border-2);
  margin-bottom: 20px;
}

.panel-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--app-text-strong);
  flex: 1;
}

.back-btn {
  background: transparent;
  border: none;
  color: var(--app-text-2);
  font-size: 18px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 4px;
  transition:
    color 0.2s,
    background-color 0.2s;
}

.back-btn:hover {
  color: var(--app-text-strong);
  background-color: var(--app-surface-2-hover);
}

.panel-body {
  display: flex;
  flex-direction: column;
}

/* 📱 移动形态（<1024px）：设置侧栏改为顶部横向 Tab 条，内容区占满全宽（解决侧栏挤压导致显示不全） */
@media (max-width: 1024px) {
  .settings-container {
    flex-direction: column;
  }

  .settings-sidebar {
    width: 100%;
    height: auto;
    flex-direction: row;
    align-items: center;
    border-right: none;
    border-bottom: 1px solid var(--app-border-2);
    flex-shrink: 0;
  }

  .sidebar-header {
    display: none;
  }

  .menu-list {
    display: flex;
    align-items: center;
    gap: 4px;
    overflow-x: auto;
    padding: 6px 8px;
    flex: 1;
    -webkit-overflow-scrolling: touch;
  }

  /* 横向 Tab 模式下隐藏分组标题，保持简洁 */
  .menu-group-title {
    display: none;
  }

  .menu-item {
    flex-shrink: 0;
    white-space: nowrap;
    padding: 8px 12px;
    font-size: 13px;
    margin-bottom: 0;
  }

  .settings-content {
    padding: 12px 16px;
  }

  .panel-section {
    max-width: 100%;
  }

  .panel-header {
    padding-bottom: 12px;
    margin-bottom: 14px;
  }
}
</style>
