<template>
  <div class="dedup-settings">
    <!-- 功能说明 -->
    <div class="intro-card">
      <div class="intro-title">🔎 本地书库维护 / 查重设置</div>
      <div class="intro-text">
        <p>· 维护页扫描本地书库，产出「<b>建议删除</b>」（同 GID / 归档 Hash / 父子画廊等强证据）与
          「<b>疑似重复</b>」（名称级弱证据，按「两本差在哪」分组）。两者都只给建议，<b>不会自动删除或忽略</b>。</p>
        <p>· 本页两项设置都作用于维护页：联网复核（提升疑似重复的可信度）与「移除时删除本地文件」的默认勾选。</p>
      </div>
    </div>

    <!-- 联网复核 -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">联网复核（E 站反查）</div>
        <div class="item-subtext">
          开启后，「近似判定」的疑似重复组会用标题去 E 站反查：命中同标题条目 → 升为高置信；
          未命中 → 标注「未确认」供优先人工确认。本地精确归一组不重复联网；
          每本约 1.2s 限流、单次上限 30 组，需已绑定 E 站账号。默认关闭。
        </div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="onlineVerify" @change="saveOnlineVerify" />
        <span class="slider"></span>
      </label>
    </div>

    <!-- 含文件默认值 -->
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">移除时同时删除本地文件</div>
        <div class="item-subtext">
          维护页「移除这本」的默认选择：不勾选＝只移除记录，本地文件保留（可随时重新扫描回来）；
          勾选＝连同本地文件一起删除（<b>不可恢复</b>）。维护页上的勾选会记住最近一次的选择。
        </div>
      </div>
      <label class="toggle-switch">
        <input type="checkbox" v-model="deleteFileDefault" @change="saveDeleteFileDefault" />
        <span class="slider"></span>
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useUI } from '@/composables/useUI'
import { http } from '@/utils/request'

const { toast } = useUI()

const onlineVerify = ref(false)
const deleteFileDefault = ref(false)

const load = async () => {
  try {
    const s = await http<{ onlineVerify?: boolean; deleteFileDefault?: boolean }>(
      '/offline/dedup/setting',
    )
    onlineVerify.value = !!s?.onlineVerify
    deleteFileDefault.value = !!s?.deleteFileDefault
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '读取查重设置失败')
  }
}

const saveOnlineVerify = async () => {
  try {
    const s = await http<{ onlineVerify?: boolean }>('/offline/dedup/setting', {
      method: 'POST',
      body: JSON.stringify({ onlineVerify: onlineVerify.value }),
    })
    onlineVerify.value = !!s?.onlineVerify
    toast.success(onlineVerify.value ? '已开启联网复核（下次扫描生效）' : '已关闭联网复核')
  } catch (err) {
    onlineVerify.value = !onlineVerify.value
    toast.error(err instanceof Error ? err.message : '设置保存失败')
  }
}

const saveDeleteFileDefault = async () => {
  try {
    const s = await http<{ deleteFileDefault?: boolean }>('/offline/dedup/setting', {
      method: 'POST',
      body: JSON.stringify({ deleteFileDefault: deleteFileDefault.value }),
    })
    deleteFileDefault.value = !!s?.deleteFileDefault
    toast.success(
      deleteFileDefault.value
        ? '维护页「移除这本」将默认同时删除本地文件'
        : '维护页「移除这本」将默认只移除记录（保留本地文件）',
    )
  } catch (err) {
    deleteFileDefault.value = !deleteFileDefault.value
    toast.error(err instanceof Error ? err.message : '设置保存失败')
  }
}

onMounted(load)
</script>

<style scoped>
.dedup-settings {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.intro-card {
  background: #14283a;
  border: 1px solid #24425c;
  border-left: 3px solid #007acc;
  border-radius: 8px;
  padding: 12px 14px;
}
.intro-title {
  font-size: 0.9rem;
  color: var(--app-text-strong);
  margin-bottom: 6px;
}
.intro-text {
  font-size: 0.78rem;
  color: #9bb6c8;
  line-height: 1.7;
}
.intro-text p {
  margin: 0;
}
.intro-text b {
  color: #7ec8ff;
}

.setting-item {
  display: flex;
  align-items: center;
  gap: 14px;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  padding: 12px 14px;
}
.item-info {
  flex: 1;
  min-width: 0;
}
.item-title {
  font-size: 0.86rem;
  color: var(--app-text-strong);
}
.item-subtext {
  margin-top: 3px;
  font-size: 0.75rem;
  color: var(--app-text-3);
  line-height: 1.6;
}
.item-subtext b {
  color: #ff9aa8;
}

.toggle-switch {
  position: relative;
  display: inline-block;
  width: 42px;
  height: 22px;
  flex: 0 0 auto;
}
.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}
.slider {
  position: absolute;
  cursor: pointer;
  inset: 0;
  background: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  border-radius: 999px;
  transition: 0.2s;
}
.slider::before {
  content: '';
  position: absolute;
  height: 16px;
  width: 16px;
  left: 2px;
  top: 2px;
  background: var(--app-text-3);
  border-radius: 50%;
  transition: 0.2s;
}
.toggle-switch input:checked + .slider {
  background: #007acc;
  border-color: #007acc;
}
.toggle-switch input:checked + .slider::before {
  transform: translateX(20px);
  background: #fff;
}
</style>
