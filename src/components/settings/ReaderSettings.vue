<template>
  <div class="reader-settings">
    <!-- ── 阅读方向 / 页面布局 ── -->
    <div class="section-title">📐 布局</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">阅读方向</div>
      </div>
      <select v-model="readerSettings.readDirection" class="setting-select">
        <option value="rtl_double">从右至左(双列)</option>
        <option value="rtl_single">从右至左(单列)</option>
        <option value="ltr_double">从左至右(双列)</option>
        <option value="ltr_single">从左至右(单列)</option>
        <option value="webtoon">连续滚动(Webtoon)</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">页面缩放</div>
        <div class="item-subtext">匹配屏幕 / 覆盖屏幕 / 适应宽度</div>
      </div>
      <select v-model="readerSettings.pageFit" class="setting-select">
        <option value="contain">匹配屏幕</option>
        <option value="cover">覆盖屏幕</option>
        <option value="width">适应宽度</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">图片间隔</div>
        <div class="item-subtext">{{ readerSettings.imageGap }}px</div>
      </div>
      <input
        v-model.number="readerSettings.imageGap"
        type="range"
        min="0"
        max="20"
        step="1"
        class="setting-range"
      />
    </div>

    <!-- 双页首页模式：仅双列阅读方向时有效，其余方向隐藏 -->
    <div v-if="isDoublePage" class="setting-item">
      <div class="item-info">
        <div class="item-title">双页首页模式</div>
        <div class="item-subtext">双列阅读时的首页排版</div>
      </div>
      <select v-model="readerSettings.singleCover" class="setting-select">
        <option :value="true">双页带首页（P1 单独一屏）</option>
        <option :value="false">双页不带首页（P1+P2 并排）</option>
      </select>
    </div>

    <!-- ── 翻页行为 ── -->
    <div class="section-title">📖 翻页行为</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">自动翻页(秒)</div>
        <div class="item-subtext">0 表示关闭</div>
      </div>
      <div class="input-inline">
        <input
          v-model.number="readerSettings.autoTurnInterval"
          type="number"
          class="setting-input wide"
          min="0"
          max="120"
        />
        <span class="unit">秒</span>
      </div>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启翻页动画</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.enableTurnAnimation" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">反转翻页方向</div>
        <div class="item-subtext">左右点击/按键方向互换</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.reverseTurnDirection" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">禁用点击翻页手势</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.disableTapTurnGesture" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- ── 界面显隐 ── -->
    <div class="section-title">🖥️ 界面显隐</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启沉浸模式</div>
        <div class="item-subtext">进入阅读器时隐藏顶部标题栏</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.immersiveMode" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">开启底部菜单</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.enableBottomMenu" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示底部栏</div>
        <div class="item-subtext">底部页码进度条与状态信息（合并）</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.showBottomBar" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- ── 设备能力 ── -->
    <div class="section-title">🔋 设备能力</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">阅读时屏幕不自动锁定</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.keepAwake" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">自定义屏幕亮度</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="readerSettings.customBrightness" />
        <span class="slider"></span>
      </label>
    </div>

    <div v-if="readerSettings.customBrightness" class="setting-item">
      <div class="item-info">
        <div class="item-title">屏幕亮度</div>
        <div class="item-subtext">{{ readerSettings.brightnessValue }}%</div>
      </div>
      <input
        v-model.number="readerSettings.brightnessValue"
        type="range"
        min="20"
        max="100"
        class="setting-range"
      />
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">图片缩放手势</div>
        <div class="item-subtext">双击 / 单击拖拽放大图片（Round25 两开关合并）</div>
      </div>
      <select v-model="readerSettings.zoomGesture" class="setting-select">
        <option value="doubleTapDrag">双击 + 拖拽</option>
        <option value="doubleTap">仅双击</option>
        <option value="off">关闭</option>
      </select>
    </div>

    <!-- ── 游戏手柄 ── -->
    <div class="section-title">🎮 游戏手柄</div>

    <!-- Round25：手柄细节全部收进子视图，主页只留一行入口 -->
    <div v-if="currentView === 'main'" class="setting-item clickable" @click="currentView = 'gamepad'">
      <div class="item-info">
        <div class="item-title">手柄设置</div>
        <div class="item-subtext">
          {{ readerSettings.enableGamepad ? '已启用 · 键位/切本细节进入配置' : '未启用' }}
        </div>
      </div>
      <span class="arrow-icon">›</span>
    </div>
    <ReaderGamepadSettings v-else @back="currentView = 'main'" />

    <!-- ── 性能 / 扩展 ── -->
    <div class="section-title">⚡ 性能 / 扩展</div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">预加载图片数量</div>
        <div class="item-subtext">在线 / 本地模式统一生效</div>
      </div>
      <select v-model="readerSettings.preloadCount" class="setting-select">
        <option :value="5">5</option>
        <option :value="10">10</option>
        <option :value="15">15</option>
        <option :value="20">20</option>
      </select>
    </div>

    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useUI } from '@/composables/useUI'
import { readerSettings, resetReaderSettings, parseReadDirection } from '@/stores/readerSettings'
import ReaderGamepadSettings from './ReaderGamepadSettings.vue'

const { toast } = useUI()

// Round25：手柄设置收进子视图（main / gamepad）
const currentView = ref<'main' | 'gamepad'>('main')

// 双页首页模式：仅双列阅读方向时显示该项
const isDoublePage = computed(() => parseReadDirection(readerSettings.readDirection).isDoublePage)

// 恢复默认阅读设置
const handleReset = () => {
  resetReaderSettings()
  toast.success('已恢复默认阅读设置')
}
</script>

<style scoped>
.reader-settings {
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

.arrow-icon {
  font-size: 20px;
  color: var(--app-text-muted);
  margin-left: 8px;
}

/* 统一的原生下拉菜单 */
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

/* 内联数值输入框 */
.input-inline {
  display: flex;
  align-items: center;
  gap: 6px;
}

.setting-input {
  background: transparent;
  border: none;
  border-bottom: 1px solid var(--app-border-3);
  color: var(--app-text-strong);
  font-size: 14px;
  width: 40px;
  text-align: center;
  outline: none;
}

.setting-input.wide {
  width: 60px;
}

.setting-input:focus {
  border-bottom-color: #ff7588;
}

.setting-range {
  accent-color: #ff7588;
  width: 140px;
  cursor: pointer;
}

.unit {
  font-size: 13px;
  color: var(--app-text-3);
}

.check-mark {
  color: var(--app-text-2);
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
