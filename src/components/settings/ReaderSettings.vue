<template>
  <div class="reader-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启沉浸模式</div>
        <div class="item-subtext">隐藏顶部标题栏</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="immersiveMode" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">阅读时屏幕不自动锁定</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="keepAwake" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示缩略图</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="showThumbnails" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示滚动条</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="showScrollbar" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">底部显示状态信息</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="showBottomStatus" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启翻页动画</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="enableTurnAnimation" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">允许双击放大图片</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="allowDoubleTapZoom" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">允许单击后拖拽放大图片</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="allowSingleClickDragZoom" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启底部菜单</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="enableBottomMenu" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">反转翻页方向</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="reverseTurnDirection" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">禁用点击翻页手势</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="disableTapTurnGesture" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启图片压缩</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="enableImageCompression" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">菜单手势区域宽度比例</div>
      </div>
      <div class="input-inline">
        <input type="number" v-model="gestureZoneWidth" class="setting-input" min="10" max="90" />
        <span class="unit">%</span>
        <span class="check-mark">✓</span>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">使用第三方阅读器</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="useThirdPartyReader" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item clickable" @click="handleSelectThirdPartyPath">
      <div class="item-info">
        <div class="item-title">第三方阅读器路径（可执行文件）</div>
        <div class="item-subtext" v-if="thirdPartyPath">{{ thirdPartyPath }}</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">阅读方向</div>
      </div>
      <select v-model="readDirection" class="setting-select">
        <option value="rtl_double">从右至左(双列)</option>
        <option value="rtl_single">从右至左(单列)</option>
        <option value="ltr_double">从左至右(双列)</option>
        <option value="ltr_single">从左至右(单列)</option>
        <option value="webtoon">连续滚动(Webtoon)</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">预加载图片数量(在线模式)</div>
      </div>
      <select v-model="preloadOnline" class="setting-select">
        <option :value="5">5</option>
        <option :value="10">10</option>
        <option :value="15">15</option>
        <option :value="20">20</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">预加载图片数量(本地模式)</div>
      </div>
      <select v-model="preloadOffline" class="setting-select">
        <option :value="5">5</option>
        <option :value="10">10</option>
        <option :value="15">15</option>
        <option :value="20">20</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">单独展示首页(全局)</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="singleCover" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">图片间隔</div>
      </div>
      <select v-model="imageGap" class="setting-select">
        <option :value="0">0</option>
        <option :value="5">5</option>
        <option :value="10">10</option>
        <option :value="15">15</option>
        <option :value="20">20</option>
      </select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useUI } from '@/composables/useUI'

const { toast } = useUI()

// 开关与控件响应式变量 (根据截图对齐初始值)
const immersiveMode = ref(true)
const keepAwake = ref(true)
const showThumbnails = ref(true)
const showScrollbar = ref(true)
const showBottomStatus = ref(true)
const enableTurnAnimation = ref(true)
const allowDoubleTapZoom = ref(true)
const allowSingleClickDragZoom = ref(false)
const enableBottomMenu = ref(false)
const reverseTurnDirection = ref(false)
const disableTapTurnGesture = ref(false)
const enableImageCompression = ref(false)
const gestureZoneWidth = ref(60)
const useThirdPartyReader = ref(false)
const thirdPartyPath = ref('')
const readDirection = ref('rtl_double')
const preloadOnline = ref(10)
const preloadOffline = ref(10)
const singleCover = ref(true)
const imageGap = ref(10)

// 选择第三方程序路径
const handleSelectThirdPartyPath = async () => {
  toast.info('选择第三方阅读器可执行文件路径')
}
</script>

<style scoped>
.reader-settings {
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

.arrow-icon {
  font-size: 20px;
  color: #66666c;
  margin-left: 8px;
}

/* 统一的原生下拉菜单 */
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

/* 内联数值输入框 */
.input-inline {
  display: flex;
  align-items: center;
  gap: 6px;
}

.setting-input {
  background: transparent;
  border: none;
  border-bottom: 1px solid #44444a;
  color: #ffffff;
  font-size: 14px;
  width: 40px;
  text-align: center;
  outline: none;
}

.setting-input:focus {
  border-bottom-color: #ff7588;
}

.unit {
  font-size: 13px;
  color: #88888c;
}

.check-mark {
  color: #a0a0a5;
  font-size: 14px;
  margin-left: 4px;
}

/* 原生 Switch 开关 */
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
