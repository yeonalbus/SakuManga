<template>
  <div class="eh-settings">
    <!-- ==================== 主视图 ==================== -->
    <template v-if="currentView === 'main'">
      <!-- Profile 设置入口（uconfig.php 界面） -->
      <div class="setting-item clickable" @click="openProfiles">
        <div class="item-info">
          <div class="item-title">Profile 设置</div>
          <div class="item-subtext">
            在应用内直接配置/切换 E 站 Profile（新建、重命名、删除、设为默认），无需跳转网页
          </div>
        </div>
        <span class="arrow-icon">›</span>
      </div>

      <!-- 站点 -->
      <div class="setting-item">
        <div class="item-info">
          <div class="item-title">站点</div>
          <div class="item-subtext">选择默认浏览的 E 站站点</div>
        </div>
        <div class="site-segmented">
          <button
            class="segment-btn"
            :class="{ active: setting?.site === 'e-hentai' }"
            @click="saveSetting({ site: 'e-hentai' })"
          >
            E-Hentai
          </button>
          <button
            class="segment-btn"
            :class="{ active: setting?.site === 'exhentai' }"
            @click="saveSetting({ site: 'exhentai' })"
          >
            EXHentai
          </button>
        </div>
      </div>

      <!-- 优先重定向至表站 -->
      <div class="setting-item">
        <div class="item-info">
          <div class="item-title">优先重定向至表站</div>
          <div class="item-subtext">优先尝试从表站加载画廊详情页，以获得更好的网络体验</div>
        </div>
        <label class="toggle-switch">
          <input
            type="checkbox"
            :checked="!!setting?.preferRedirect"
            @change="onPreferRedirectChange"
          />
          <span class="slider"></span>
        </label>
      </div>

      <!-- 图片配额 -->
      <div
        class="setting-item clickable status-item"
        :class="{ refreshing: statusLoading }"
        @click="refreshStatus"
      >
        <div class="item-info">
          <div class="item-title">
            图片配额 <span v-if="statusLoading" class="loading-hint">刷新中…</span>
          </div>
          <div class="item-subtext">点击刷新</div>
        </div>
        <div class="item-right-content">
          <span class="quota-text"
            >{{ formatNumber(status.currentQuota) }} / {{ formatNumber(status.maxQuota) }}</span
          >
          <span v-if="!statusLoading" class="refresh-icon" title="刷新">↻</span>
        </div>
      </div>

      <!-- 资产 -->
      <div
        class="setting-item clickable status-item"
        :class="{ refreshing: statusLoading }"
        @click="refreshStatus"
      >
        <div class="item-info">
          <div class="item-title">
            资产 <span v-if="statusLoading" class="loading-hint">刷新中…</span>
          </div>
          <div class="item-subtext">GP / Credits / Hath，点击刷新</div>
        </div>
        <div class="assets-text">
          <span>GP: {{ status.assetGP || '--' }}</span>
          <span>Credits: {{ status.assetCredits || '--' }}</span>
          <span>Hath: {{ status.assetHath || '--' }}</span>
          <span v-if="!statusLoading" class="refresh-icon" title="刷新">↻</span>
        </div>
      </div>

      <!-- 我的标签 -->
      <div class="setting-item clickable" @click="currentView = 'mytags'">
        <div class="item-info">
          <div class="item-title">我的标签</div>
          <div class="item-subtext">管理关注和隐藏的标签（直连 E 站读写）</div>
        </div>
        <span class="arrow-icon">›</span>
      </div>
    </template>

    <!-- ==================== Profile 设置子视图（uconfig.php 界面） ==================== -->
    <ProfileSettings v-else-if="currentView === 'profiles'" @back="currentView = 'main'" />

    <!-- ==================== 我的标签子视图 ==================== -->
    <MyTagsSettings v-else-if="currentView === 'mytags'" @back="currentView = 'main'" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import type { EHSetting, EHSite, EHUserStatus } from '@/types/eh'
import ProfileSettings from './ProfileSettings.vue'
import MyTagsSettings from './MyTagsSettings.vue'

const { toast } = useUI()

const currentView = ref<'main' | 'profiles' | 'mytags'>('main')

const setting = ref<EHSetting | null>(null)

const status = ref<EHUserStatus>({
  currentQuota: 0,
  maxQuota: 0,
  assetGP: '',
  assetCredits: '',
  assetHath: '',
})
const statusLoading = ref(false)

const formatNumber = (n: number) => (n > 0 ? n.toLocaleString() : '--')

// 1. 加载当前生效设置
const loadSettings = async () => {
  try {
    setting.value = await http<EHSetting>('/eh/settings')
  } catch (err) {
    toast.error('加载 EH 设置失败')
    console.error(err)
  }
}

// 2. 保存站点配置（直接写入生效快照，不再经本地 Profile）
const saveSetting = async (patch: Partial<Pick<EHSetting, 'site' | 'preferRedirect'>>) => {
  try {
    await http('/eh/settings', {
      method: 'POST',
      body: JSON.stringify(patch),
    })
    if (patch.site && setting.value) setting.value.site = patch.site as EHSite
    if (patch.preferRedirect !== undefined && setting.value)
      setting.value.preferRedirect = patch.preferRedirect
    toast.success('设置已保存')
    // 站点切换会影响配额/资产来源，自动刷新
    if (patch.site) refreshStatus()
  } catch (err) {
    toast.error((err as Error)?.message || '保存设置失败')
    console.error(err)
    await loadSettings()
  }
}

// 3. 刷新图片配额与资产（点击状态卡片触发）
const refreshStatus = async () => {
  statusLoading.value = true
  try {
    status.value = await http<EHUserStatus>('/eh/status')
  } catch (err) {
    toast.error((err as Error)?.message || '读取配额与资产失败')
    console.error(err)
  } finally {
    statusLoading.value = false
  }
}

const onPreferRedirectChange = (e: Event) => {
  saveSetting({ preferRedirect: (e.target as HTMLInputElement).checked })
}

const openProfiles = () => {
  currentView.value = 'profiles'
}

onMounted(() => {
  loadSettings()
  refreshStatus()
})
</script>

<style scoped>
.eh-settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 每一个设置条目行 */
.setting-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px;
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

.arrow-icon {
  font-size: 20px;
  color: var(--app-text-muted);
  margin-left: 8px;
}

.item-right-content {
  display: flex;
  align-items: center;
  gap: 8px;
}

.quota-text {
  font-size: 14px;
  color: var(--app-text-2);
  font-family: monospace;
}

.assets-text {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: var(--app-text-2);
  font-family: monospace;
}

.refresh-icon {
  font-size: 16px;
  color: var(--app-text-muted);
  margin-left: 4px;
}

.status-item.refreshing {
  opacity: 0.6;
}

.loading-hint {
  font-size: 12px;
  color: #ff7588;
}

/* 分段选择控制器 */
.site-segmented {
  display: flex;
  background-color: var(--app-surface-3);
  padding: 3px;
  border-radius: 6px;
  gap: 2px;
}

.segment-btn {
  border: none;
  background: transparent;
  color: var(--app-text-3);
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.segment-btn.active {
  background-color: var(--app-border-3);
  color: var(--app-text-strong);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

/* 原生风 Switch 开关 */
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
</style>
