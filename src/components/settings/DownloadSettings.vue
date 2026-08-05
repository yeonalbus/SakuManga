<template>
  <!-- 当 showExtraScanPaths 为 true 时显示子组件，并在收到 back 事件时切回 -->
  <ExtraScanPathsSettings v-if="showExtraScanPaths" @back="showExtraScanPaths = false" />

  <!-- 主设置列表 -->
  <div v-else class="download-settings">
    <!-- ── 下载路径 ── -->
    <div class="section-title">📁 下载路径</div>

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

    <div class="setting-item clickable" @click="handleResetDownloadPath">
      <div class="item-info">
        <div class="item-title">重置下载路径</div>
        <div class="item-subtext">长按以重置压缩包路径与解压路径</div>
      </div>
    </div>

    <!-- ── 下载行为 ── -->
    <div class="section-title">⬇️ 下载行为</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">默认选中下载原图</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="downloadSettings.defaultDownloadOriginal" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item clickable" @click="handleResetDownloadGroup">
      <div class="item-info">
        <div class="item-title">默认分组（下载）</div>
        <div class="item-subtext">长按以重置</div>
      </div>
      <span class="row-value">{{ downloadSettings.defaultDownloadGroup }}</span>
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
          默认情况下逐优先级别下载画廊，且每个优先级下只会同时下载一个画廊 | 需要重启
        </div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="downloadSettings.downloadAllGalleriesSamePriority" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- ── 归档设置 ── -->
    <div class="section-title">🗜️ 归档设置</div>

    <div class="setting-item clickable" @click="handleResetArchiveGroup">
      <div class="item-info">
        <div class="item-title">默认分组（归档）</div>
        <div class="item-subtext">长按以重置</div>
      </div>
      <span class="row-value">{{ downloadSettings.defaultArchiveGroup }}</span>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">归档下载线程数</div>
        <div class="item-subtext">所有任务活跃线程数之和若超过10将导致下载失败</div>
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
        <div class="item-subtext">在有足够的线程下载之前，归档任务会保持等待状态</div>
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
import { ref } from 'vue'
import { useUI } from '@/composables/useUI'
import ExtraScanPathsSettings from './ExtraScanPathsSettings.vue' // 1. 引入画廊路径设置组件
import { downloadSettings, resetDownloadSettings } from '@/stores/downloadSettings'

const { toast, modal } = useUI()

// 2. 增加界面切换的状态
const showExtraScanPaths = ref(false)

// 交互事件处理
const handleSelectArchivePath = () => {
  toast.info('修改压缩包路径')
}

const handleSelectExtractPath = () => {
  toast.info('修改解压后的文件夹存储路径')
}

const handleSelectSingleImagePath = () => {
  toast.info('修改单张图片保存路径')
}

const handleResetDownloadPath = () => {
  toast.info('已长按重置下载路径')
}

// 3. 修改点击处理函数：切换为 true
const handleExtraScanPaths = () => {
  showExtraScanPaths.value = true
}

const handleResetDownloadGroup = () => {
  toast.info('重置下载默认分组')
}

const handleResetArchiveGroup = () => {
  toast.info('重置归档默认分组')
}

const handleRestoreDownloadTasks = async () => {
  const confirm = await modal.confirm('确定要通过本地元数据恢复丢失的下载记录吗？')
  if (confirm) {
    toast.success('正在尝试从元数据恢复下载任务...')
  }
}

// 恢复默认下载设置
const handleReset = () => {
  resetDownloadSettings()
  toast.success('已恢复默认下载设置')
}
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

.row-value {
  font-size: 14px;
  color: #d0d0d0;
}

.arrow-icon {
  font-size: 20px;
  color: #66666c;
  margin-left: 8px;
}

/* 行内多下拉控件组 */
.inline-controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.inline-text {
  font-size: 13px;
  color: #88888c;
}

/* 下拉菜单 */
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
  background-color: #38383e;
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
  background-color: #a0a0a5;
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
  background: #242428;
  border: 1px solid #3a3a3d;
  color: #ccc;
  padding: 10px 16px;
  border-radius: 8px;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.reset-btn:hover {
  background-color: #2e2e33;
  border-color: #ff7588;
  color: #ff7588;
}
</style>
