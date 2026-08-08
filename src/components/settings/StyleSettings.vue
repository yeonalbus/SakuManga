<template>
  <div class="style-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">主题模式</div>
      </div>
      <select v-model="styleSettings.themeMode" class="setting-select">
        <option value="system">跟随系统</option>
        <option value="dark">暗黑模式</option>
        <option value="light">亮色模式</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示样式</div>
        <div class="item-subtext">画廊列表的展示形态：卡片 / 名片</div>
      </div>

      <div class="view-mode-segmented">
        <button
          class="segment-btn"
          :class="{ active: viewMode === 'card' }"
          @click="setViewMode('card')"
        >
          🎴 卡片
        </button>
        <button
          class="segment-btn"
          :class="{ active: viewMode === 'compact' }"
          @click="setViewMode('compact')"
        >
          🪪 名片
        </button>
      </div>
    </div>

    <!-- 自动适配详情面板列数（仅宽屏桌面在线列表生效） -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">自动适配详情面板列数</div>
        <div class="item-subtext">开启后，宽屏桌面在线列表会按详情面板「收起 / 展开」自动切换每行画廊数</div>
      </div>
      <button
        class="toggle-btn"
        :class="{ active: styleSettings.autoPanelColumns }"
        @click="toggleAutoPanelColumns"
      >
        {{ styleSettings.autoPanelColumns ? '已开启' : '已关闭' }}
      </button>
    </div>

    <div v-if="styleSettings.autoPanelColumns" class="setting-item">
      <div class="item-info">
        <div class="item-title">面板列数</div>
        <div class="item-subtext">卡片 / 名片分别设置「收起 / 展开」时的每行画廊数，范围 1-5</div>
      </div>
      <div class="columns-config panel-cols">
        <label class="columns-row">
          <span class="columns-label">🎴 卡片·收起</span>
          <select v-model.number="styleSettings.cardPanelClosedCols" class="setting-select">
            <option v-for="n in 5" :key="`cc-${n}`" :value="n">{{ n }} 个</option>
          </select>
        </label>
        <label class="columns-row">
          <span class="columns-label">🎴 卡片·展开</span>
          <select v-model.number="styleSettings.cardPanelOpenCols" class="setting-select">
            <option v-for="n in 5" :key="`co-${n}`" :value="n">{{ n }} 个</option>
          </select>
        </label>
        <label class="columns-row">
          <span class="columns-label">🪪 名片·收起</span>
          <select v-model.number="styleSettings.compactPanelClosedCols" class="setting-select">
            <option v-for="n in 5" :key="`mpc-${n}`" :value="n">{{ n }} 个</option>
          </select>
        </label>
        <label class="columns-row">
          <span class="columns-label">🪪 名片·展开</span>
          <select v-model.number="styleSettings.compactPanelOpenCols" class="setting-select">
            <option v-for="n in 5" :key="`mpo-${n}`" :value="n">{{ n }} 个</option>
          </select>
        </label>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">布局模式</div>
        <div class="item-subtext">自动跟随视口；桌面/移动用于手动强制布局形态</div>
        <div class="item-subtext device-hint">
          当前设备：{{ deviceLabel }}，各设备分别记忆、互不干扰
        </div>
      </div>
      <select v-model="currentLayoutMode" class="setting-select">
        <option value="auto">自动</option>
        <option value="desktop">桌面模式</option>
        <option value="mobile">移动模式</option>
      </select>
    </div>

    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useUI } from '@/composables/useUI'
import { viewMode, toggleViewMode } from '@/stores/viewMode'
import {
  styleSettings,
  resetStyleSettings,
  currentDeviceClass,
  setLayoutMode,
} from '@/stores/styleSettings'
import { DEVICE_CLASS_LABELS } from '@/utils/device'

const { toast } = useUI()

const deviceLabel = DEVICE_CLASS_LABELS[currentDeviceClass]

// 布局模式：get 读镜像字段，set 写入当前设备槽位（setLayoutMode 自动镜像）
const currentLayoutMode = computed({
  get: () => styleSettings.layoutMode,
  set: (mode) => setLayoutMode(mode),
})

const setViewMode = (targetMode: 'card' | 'compact') => {
  if (viewMode.value !== targetMode) {
    toggleViewMode()
    toast.info(`已切换显示样式为：${targetMode === 'card' ? '卡片' : '名片'}`)
  }
}

// 自动适配详情面板列数开关
const toggleAutoPanelColumns = () => {
  styleSettings.autoPanelColumns = !styleSettings.autoPanelColumns
  toast.info(
    styleSettings.autoPanelColumns
      ? '已开启自动适配详情面板列数'
      : '已关闭自动适配详情面板列数',
  )
}

// 恢复默认样式设置
const handleReset = () => {
  resetStyleSettings()
  toast.success('已恢复默认样式设置')
}
</script>

<style scoped>
.style-settings {
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

.device-hint {
  font-size: 12px;
  color: #ff7588;
}

/* 统一的原生下拉菜单排版，去除底层粗黑框 */
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

/* 接入胶囊分段按钮的专属 CSS 样式 */
.view-mode-segmented {
  display: flex;
  background-color: var(--app-surface-3);
  padding: 3px;
  border-radius: 6px;
  gap: 2px;
}

/* 面板列数：卡片 / 名片 · 收起/展开 四个独立行 */
.columns-config {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.columns-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.columns-label {
  font-size: 13px;
  color: var(--app-text-2);
  white-space: nowrap;
}

/* 面板列数子配置：行距更紧凑 */
.panel-cols {
  gap: 4px;
}

/* 自动适配详情面板列数开关（胶囊按钮） */
.toggle-btn {
  background: transparent;
  border: 1px solid var(--app-border-3);
  color: var(--app-text-3);
  padding: 6px 14px;
  border-radius: 20px;
  font-size: 13px;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s ease;
}

.toggle-btn:hover {
  color: var(--app-text-strong);
}

.toggle-btn.active {
  background-color: #00a896;
  border-color: #00a896;
  color: #fff;
  font-weight: 600;
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
  display: flex;
  align-items: center;
  gap: 4px;
}

.segment-btn:hover {
  color: var(--app-text-strong);
}

.segment-btn.active {
  background-color: var(--app-border-3);
  color: var(--app-text-strong);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
  font-weight: 600;
}

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
