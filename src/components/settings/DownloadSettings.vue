<template>
  <!-- 当 showExtraScanPaths 为 true 时显示子组件，并在收到 back 事件时切回 -->
  <ExtraScanPathsSettings v-if="showExtraScanPaths" @back="showExtraScanPaths = false" />

  <!-- 主设置列表 -->
  <div v-else class="download-settings">
    <!-- ── 下载路径 ── -->
    <div class="section-title">📁 下载路径</div>

    <div v-if="usingDefaultPaths" class="path-hint">
      ⚠️ 当前使用默认下载目录（程序目录下的 downloads
      文件夹）。建议点击上方路径，修改为你自己的下载目录。
    </div>

    <div class="setting-item clickable" @click="handleSelectArchivePath">
      <div class="item-info">
        <div class="item-title">压缩包路径</div>
        <div class="item-subtext">{{ downloadSettings.archivePath }}</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item clickable" @click="handleSelectExtractPath">
      <div class="item-info">
        <div class="item-title">解压后的文件夹存储路径</div>
        <div class="item-subtext">{{ downloadSettings.extractPath }}</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item clickable" @click="handleSelectSingleImagePath">
      <div class="item-info">
        <div class="item-title">单张图片保存路径</div>
        <div class="item-subtext">{{ downloadSettings.singleImageSavePath }}</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item clickable" @click="handleExtraScanPaths">
      <div class="item-info">
        <div class="item-title">额外的画廊扫描路径</div>
        <div class="item-subtext">扫描并加载本地画廊的路径。请不要使用SD卡或系统路径</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div
      class="setting-item clickable"
      @mousedown="handleResetPressStart"
      @mouseup="handleResetPressEnd"
      @mouseleave="handleResetPressEnd"
      @touchstart="handleResetPressStart"
      @touchend="handleResetPressEnd"
      @contextmenu.prevent
    >
      <div class="item-info">
        <div class="item-title">重置下载路径</div>
        <div class="item-subtext">长按以重置压缩包路径、解压路径与单张图片保存路径</div>
      </div>
    </div>

    <!-- ── 下载行为 ── -->
    <div class="section-title">⬇️ 下载行为</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">默认下载配置</div>
        <div class="item-subtext">快捷/批量下载与详情下载面板默认采用的方案</div>
      </div>
      <select v-model="downloadSettings.defaultDownloadScheme" class="setting-select">
        <option v-for="opt in DEFAULT_DOWNLOAD_SCHEME_OPTIONS" :key="opt.value" :value="opt.value">
          {{ opt.label }}
        </option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">同时下载图片数量</div>
      </div>
      <select v-model="downloadSettings.concurrentImageDownloads" class="setting-select">
        <option :value="3">3</option>
        <option :value="5">5</option>
        <option :value="10">10</option>
        <option :value="15">15</option>
        <option :value="20">20</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">速度限制</div>
        <div class="item-subtext">下载太快可能会被限制</div>
      </div>
      <div class="inline-controls">
        <select v-model="downloadSettings.speedLimitImages" class="setting-select">
          <option :value="30">30</option>
          <option :value="50">50</option>
          <option :value="99">99</option>
        </select>
        <span class="inline-text">图片 每</span>
        <select v-model="downloadSettings.speedLimitInterval" class="setting-select">
          <option value="1s">1s</option>
          <option value="2s">2s</option>
          <option value="5s">5s</option>
        </select>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">同一优先级下同时下载所有画廊</div>
        <div class="item-subtext">
          默认情况下逐优先级别下载画廊，且每个优先级下只会同时下载一个画廊（实时生效）
        </div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="downloadSettings.downloadAllGalleriesSamePriority" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- ── 归档设置 ── -->
    <div class="section-title">🗜️ 归档设置</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">归档下载线程数</div>
        <div class="item-subtext">
          单个归档文件分片下载的并发线程数；所有任务活跃线程数之和上限为 10，超出部分自动排队等待
        </div>
      </div>
      <select v-model="downloadSettings.archiveThreads" class="setting-select">
        <option :value="3">3</option>
        <option :value="5">5</option>
        <option :value="10">10</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">控制归档下载并发数</div>
        <div class="item-subtext">
          开启后归档任务需先获取全局线程配额，线程不足时保持等待状态；关闭后各任务直接使用设置线程数下载
        </div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="downloadSettings.controlArchiveConcurrency" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">归档下载完成后删除原压缩包</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="downloadSettings.deleteZipAfterArchiveDownload" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- ── 下载任务 ── -->
    <div class="section-title">📋 下载任务</div>

    <div class="setting-item clickable" @click="handleRestoreDownloadTasks">
      <div class="item-info">
        <div class="item-title">恢复下载任务</div>
        <div class="item-subtext">通过下载元数据来恢复下载记录</div>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">自动恢复下载任务</div>
        <div class="item-subtext">应用每次启动时尝试恢复下载任务</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="downloadSettings.autoResumeTasks" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- ── 自动更新画廊 ── -->
    <div class="section-title">🔄 自动更新</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">自动更新画廊</div>
        <div class="item-subtext">检测到画廊有更新时自动下载新版本（需先运行「检测更新」）</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="downloadSettings.autoUpdateGallery" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">更新下载方案</div>
        <div class="item-subtext">下载新版本时采用的方案</div>
      </div>
      <select v-model="downloadSettings.autoUpdateScheme" class="setting-select">
        <option value="archive">归档（H@H）</option>
        <option value="gallery">画廊（逐图）</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">无 H@H 时自动降级为画廊下载</div>
        <div class="item-subtext">归档下载失败时回退到逐图下载</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="downloadSettings.autoUpdateFallbackToGallery" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">下载新版本后删除旧版本</div>
        <div class="item-subtext">下载完成后自动删除旧版本文件夹与记录</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="downloadSettings.autoUpdateDeleteOriginal" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- 恢复默认设置 -->
    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'
import ExtraScanPathsSettings from './ExtraScanPathsSettings.vue'
import {
  downloadSettings,
  fetchDownloadSettings,
  isUsingDefaultDownloadPaths,
  resetDownloadPaths,
  resetDownloadSettings,
  DEFAULT_DOWNLOAD_SCHEME_OPTIONS,
} from '@/stores/downloadSettings'

const { toast, modal } = useUI()

// 界面切换：额外画廊扫描路径子组件
const showExtraScanPaths = ref(false)

// 首次使用引导：三个下载路径仍为默认值时提示用户修改
const usingDefaultPaths = computed(() => isUsingDefaultDownloadPaths())

const handleExtraScanPaths = () => {
  showExtraScanPaths.value = true
}

// ── 下载路径：点击弹出输入框修改，写入 store 后自动同步到后端 ──
type PathKey = 'archivePath' | 'extractPath' | 'singleImageSavePath'

const editPath = async (key: PathKey, title: string, message: string) => {
  const value = await modal.prompt(message, downloadSettings[key], `修改${title}`)
  if (value !== null && value.trim() !== '') {
    downloadSettings[key] = value.trim()
    toast.success(`${title}已更新`)
  }
}

const handleSelectArchivePath = () =>
  editPath('archivePath', '压缩包路径', '请输入压缩包存储路径：')
const handleSelectExtractPath = () =>
  editPath('extractPath', '解压后的文件夹存储路径', '请输入解压后的文件夹存储路径：')
const handleSelectSingleImagePath = () =>
  editPath('singleImageSavePath', '单张图片保存路径', '请输入单张图片保存路径：')

// ── 重置下载路径：长按触发（参照 OnlineDetail 收藏长按模式）──
let pressTimer: number | null = null

const clearResetPressTimer = () => {
  if (pressTimer !== null) {
    clearTimeout(pressTimer)
    pressTimer = null
  }
}

const handleResetPressStart = () => {
  clearResetPressTimer()
  pressTimer = window.setTimeout(() => {
    doResetDownloadPath()
  }, 700)
}

const handleResetPressEnd = () => {
  clearResetPressTimer()
}

const doResetDownloadPath = async () => {
  const confirm = await modal.confirm(
    '确定要将压缩包路径、解压路径与单张图片保存路径重置为默认值吗？',
  )
  if (confirm) {
    resetDownloadPaths()
    toast.success('已重置下载路径')
  }
}

// ── 恢复下载任务：调用后端元数据恢复接口 ──
const handleRestoreDownloadTasks = async () => {
  const confirm = await modal.confirm('确定要通过本地元数据恢复丢失的下载记录吗？')
  if (!confirm) return
  try {
    const res = await http<{ restored: number }>('/downloads/restore', { method: 'POST' })
    toast.success(`已恢复 ${res.restored} 个下载任务`)
  } catch (err) {
    toast.error(`恢复下载任务失败：${err instanceof Error ? err.message : String(err)}`)
  }
}

// 恢复默认下载设置
const handleReset = () => {
  resetDownloadSettings()
  toast.success('已恢复默认下载设置')
}

// 挂载时从后端拉取最新设置（后端为唯一事实来源）
onMounted(() => {
  fetchDownloadSettings()
})

onUnmounted(() => {
  clearResetPressTimer()
})
</script>

<style scoped>
.download-settings {
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
  border-bottom: 1px solid var(--app-border-2);
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

.item-subtext {
  font-size: 13px;
  color: var(--app-text-3);
  line-height: 1.4;
}

.row-value {
  font-size: 14px;
  color: var(--app-text-2);
}

.arrow-icon {
  font-size: 20px;
  color: var(--app-text-muted);
  margin-left: 8px;
}

/* 首次使用路径引导提示 */
.path-hint {
  padding: 10px 14px;
  background-color: rgba(255, 117, 136, 0.08);
  border: 1px solid rgba(255, 117, 136, 0.35);
  border-radius: 8px;
  color: #ff7588;
  font-size: 13px;
  line-height: 1.5;
}

/* 行内多下拉控件组 */
.inline-controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.inline-text {
  font-size: 13px;
  color: var(--app-text-3);
}

/* 下拉菜单 */
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

/* Switch 开关 */
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

/* 恢复默认按钮 */
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

.reset-btn:hover {
  background-color: var(--app-surface-3-hover);
  border-color: #ff7588;
  color: #ff7588;
}
</style>
