<template>
  <div class="preference-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">语言</div>
      </div>
      <select v-model="preferenceSettings.language" class="setting-select">
        <option value="zh-CN">简体中文</option>
        <option value="zh-TW">繁體中文</option>
        <option value="en-US">English</option>
        <option value="ja-JP">日本語</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">启动时默认菜单</div>
      </div>
      <select v-model="preferenceSettings.defaultStartupMenu" class="setting-select">
        <option value="hot">热门</option>
        <option value="home">首页</option>
        <option value="sub">订阅</option>
        <option value="fav">收藏</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">隐藏快速回顶按钮</div>
      </div>
      <select v-model="preferenceSettings.hideScrollToTopBtn" class="setting-select">
        <option value="scrolling_down">向下滚动时</option>
        <option value="always">总是隐藏</option>
        <option value="never">从不隐藏</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">预载画廊封面</div>
        <div class="item-subtext">预先加载未显示在页面上的画廊的封面</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.preloadGalleryCover" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">允许通过左滑手势返回</div>
        <div class="item-subtext">需要重启</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.allowLeftSwipeBack" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示所有画廊标题</div>
        <div class="item-subtext">同时显示原标题和日文标题（如果可用）</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.showAllGalleryTitles" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示画廊标签投票状态</div>
        <div class="item-subtext">包括可信、存疑与错误三种状态</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.showTagVoteStatus" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示画廊评论</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.showGalleryComments" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">显示画廊所有评论</div>
        <div class="item-subtext">
          默认只会展示45个最高分评论和5个最新评论，低分评论也会被自动隐藏
        </div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.showAllGalleryComments" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">使用默认收藏夹</div>
        <div class="item-subtext">默认直接收藏，长按重新选择</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.useDefaultFavorite" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">关注标签时使用默认标签集</div>
        <div class="item-subtext">默认直接关注，长按重新选择</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.useDefaultTagSet" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">以全屏模式启动</div>
        <div class="item-subtext">F11手动切换全屏</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.startInFullscreen" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">搜索选项继承</div>
        <div class="item-subtext">搜索时使用上一次搜索选项</div>
      </div>
      <select v-model="preferenceSettings.searchOptionsInherit" class="setting-select">
        <option value="all">继承全部</option>
        <option value="category_only">仅继承分类</option>
        <option value="none">不继承</option>
      </select>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">标签数据中直接显示R18G图片</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.showR18GTagImages" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">画廊时间使用UTC展示</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.showTimeInUTC" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">展示黎明之事件</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.showDawnEvent" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">展示HV遭遇战事件</div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="preferenceSettings.showHVEvent" />
        <span class="slider"></span>
      </label>
    </div>

    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">使用内置用户屏蔽名单</div>
        <div class="item-subtext">过滤掉在名单中的用户评论</div>
      </div>
      <div class="item-actions">
        <button class="info-btn" title="查看说明" @click="handleBlocklistInfo">?</button>
        <label class="toggle-switch">
          <input type="checkbox" v-model="preferenceSettings.useBuiltinBlocklist" />
          <span class="slider"></span>
        </label>
      </div>
    </div>

    <div class="setting-item clickable" @click="handleBlockRules">
      <div class="item-info">
        <div class="item-title">屏蔽规则</div>
        <div class="item-subtext">针对画廊和评论设置额外的屏蔽规则</div>
      </div>
      <span class="arrow-icon">›</span>
    </div>

    <div class="reset-row">
      <button class="reset-btn" @click="handleReset">恢复默认设置</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useUI } from '@/composables/useUI'
import { preferenceSettings, resetPreferenceSettings } from '@/stores/preferenceSettings'

const { toast, modal } = useUI()

const handleBlocklistInfo = async () => {
  await modal.alert('内置屏蔽名单包含广告发布者、恶评用户等过滤规则，开启后可提升阅读体验。')
}

const handleBlockRules = () => {
  toast.info('打开高级屏蔽规则管理抽屉')
}

// 恢复默认偏好设置
const handleReset = () => {
  resetPreferenceSettings()
  toast.success('已恢复默认偏好设置')
}
</script>

<style scoped>
.preference-settings {
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

.item-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.arrow-icon {
  font-size: 20px;
  color: #66666c;
  margin-left: 8px;
}

.info-btn {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 1px solid #88888c;
  background: transparent;
  color: #88888c;
  font-size: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s ease;
}

.info-btn:hover {
  color: #ffffff;
  border-color: #ffffff;
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

.reset-row {
  display: flex;
  justify-content: center;
  padding: 8px 0;
}

.reset-btn {
  background: transparent;
  border: 1px solid #44444a;
  color: #a0a0a5;
  font-size: 13px;
  padding: 8px 20px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.reset-btn:hover {
  border-color: #ff7588;
  color: #ffffff;
}
</style>
