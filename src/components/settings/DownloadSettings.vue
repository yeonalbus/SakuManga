<template>
  <div class="download-settings">
    <div class="setting-item clickable" @click="handleSelectDownloadPath">
      <div class="item-info">
        <div class="item-title">下载路径</div>
        <div class="item-subtext">{{ downloadPath }}</div>
      </div>
    </div>

    <div class="setting-item clickable" @click="handleResetDownloadPath">
      <div class="item-info">
        <div class="item-title">重置下载路径</div>
        <div class="item-subtext">长按以重置</div>
      </div>
    </div>

    <div class="setting-item clickable" @click="handleExtraScanPaths">
      <div class="item-info">
        <div class="item-title">额外的画廊扫描路径</div>
        <div class="item-subtext">扫描并加载本地画廊的路径。请不要使用SD卡或系统路径</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item clickable" @click="handleSelectSingleImagePath">
      <div class="item-info">
        <div class="item-title">单张图片保存路径</div>
        <div class="item-subtext">{{ singleImageSavePath }}</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">默认选中下载原图</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="defaultDownloadOriginal" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item clickable" @click="handleResetDownloadGroup">
      <div class="item-info">
        <div class="item-title">默认分组（下载）</div>
        <div class="item-subtext">长按以重置</div>
      </div>
    </div>

    <div class="setting-item clickable" @click="handleResetArchiveGroup">
      <div class="item-info">
        <div class="item-title">默认分组（归档）</div>
        <div class="item-subtext">长按以重置</div>
      </div>
      <span class="row-value">{{ defaultArchiveGroup }}</span>
    </div>

    <div class="setting-item clickable" @click="handleArchiveBotSettings">
      <div class="item-info">
        <div class="item-title">归档机器人设置</div>
        <div class="item-subtext">使用归档机器人免费获取归档链接</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">同时下载图片数量</div>
      </div>
      <select v-model="concurrentImageDownloads" class="setting-select">
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
        <select v-model="speedLimitImages" class="setting-select">
          <option :value="30">30</option>
          <option :value="50">50</option>
          <option :value="99">99</option>
        </select>
        <span class="inline-text">图片 每</span>
        <select v-model="speedLimitInterval" class="setting-select">
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
        <input type="checkbox" v-model="downloadAllGalleriesSamePriority" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">使用JH服务器加速画廊更新</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="useJHServerAccelerate" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">归档下载线程数</div>
        <div class="item-subtext">所有任务活跃线程数之和若超过10将导致下载失败</div>
      </div>
      <select v-model="archiveThreads" class="setting-select">
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
        <input type="checkbox" v-model="controlArchiveConcurrency" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">归档下载完成后删除原压缩包</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="deleteZipAfterArchiveDownload" />
        <span class="slider"></span>
      </label>
    </div>

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
        <input type="checkbox" v-model="autoResumeTasks" />
        <span class="slider"></span>
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useUI } from '@/composables/useUI'

const { toast, modal } = useUI()

// 状态声明 (按照截图默认值对齐)
const downloadPath = ref('Z:\\Comics')
const singleImageSavePath = ref('Z:\\Comics')
const defaultDownloadOriginal = ref(true)
const defaultArchiveGroup = ref('默认')
const concurrentImageDownloads = ref(10)
const speedLimitImages = ref(99)
const speedLimitInterval = ref('1s')
const downloadAllGalleriesSamePriority = ref(true)
const useJHServerAccelerate = ref(true)
const archiveThreads = ref(10)
const controlArchiveConcurrency = ref(true)
const deleteZipAfterArchiveDownload = ref(true)
const autoResumeTasks = ref(true)

// 交互事件处理
const handleSelectDownloadPath = () => {
  toast.info('修改全局下载路径')
}

const handleResetDownloadPath = () => {
  toast.info('已长按重置下载路径')
}

const handleExtraScanPaths = () => {
  toast.info('打开额外的画廊扫描路径设置')
}

const handleSelectSingleImagePath = () => {
  toast.info('修改单张图片保存路径')
}

const handleResetDownloadGroup = () => {
  toast.info('重置下载默认分组')
}

const handleResetArchiveGroup = () => {
  toast.info('重置归档默认分组')
}

const handleArchiveBotSettings = () => {
  toast.info('打开归档机器人配置抽屉')
}

const handleRestoreDownloadTasks = async () => {
  const confirm = await modal.confirm('确定要通过本地元数据恢复丢失的下载记录吗？')
  if (confirm) {
    toast.success('正在尝试从元数据恢复下载任务...')
  }
}
</script>

<style scoped>
.download-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
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
</style>
