<template>
  <div class="profile-settings">
    <div class="profile-header-info">
      <p class="desc">Profile 用于配置 E 站浏览预设（如每页条数、默认原图展示、搜索过滤等）。</p>
    </div>

    <div class="profile-list">
      <div
        v-for="profile in profiles"
        :key="profile.id"
        class="profile-item"
        :class="{ active: currentProfileId === profile.id }"
        @click="selectProfile(profile.id)"
      >
        <div class="profile-meta">
          <span class="profile-name">{{ profile.name }}</span>
          <span v-if="profile.isDefault" class="default-badge">默认</span>
        </div>
        <div class="radio-indicator">
          <span v-if="currentProfileId === profile.id" class="dot"></span>
        </div>
      </div>
    </div>

    <button class="add-profile-btn" @click="handleAddProfile">➕ 新建 Profile 预设</button>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useUI } from '@/composables/useUI'

const { toast, modal } = useUI()

const currentProfileId = ref('default')

const profiles = ref([
  { id: 'default', name: 'Default Profile', isDefault: true },
  { id: 'profile_1', name: '画廊原图下载预设', isDefault: false },
  { id: 'profile_2', name: '移动端低流量模式', isDefault: false },
])

const selectProfile = (id: string) => {
  currentProfileId.value = id
  toast.success('已切换 Profile 预设')
}

const handleAddProfile = async () => {
  const name = await modal.prompt('请输入新的 Profile 名称:')
  if (name && name.trim()) {
    const newId = `profile_${Date.now()}`
    profiles.value.push({
      id: newId,
      name: name.trim(),
      isDefault: false,
    })
    currentProfileId.value = newId
    toast.success(`成功创建 Profile: ${name}`)
  }
}
</script>

<style scoped>
.profile-settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.profile-header-info .desc {
  margin: 0;
  font-size: 13px;
  color: #88888c;
  line-height: 1.5;
}

.profile-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.profile-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background-color: #1a1a1e;
  border-radius: 8px;
  border: 1px solid #26262a;
  cursor: pointer;
  transition: all 0.2s ease;
}

.profile-item:hover {
  background-color: #222226;
}

.profile-item.active {
  border-color: #ff7588;
  background-color: rgba(255, 117, 136, 0.05);
}

.profile-meta {
  display: flex;
  align-items: center;
  gap: 10px;
}

.profile-name {
  font-size: 14px;
  color: #ffffff;
  font-weight: 500;
}

.default-badge {
  font-size: 11px;
  background-color: #2b2b30;
  color: #a0a0a5;
  padding: 2px 6px;
  border-radius: 4px;
}

.radio-indicator {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  border: 2px solid #55555a;
  display: flex;
  align-items: center;
  justify-content: center;
}

.profile-item.active .radio-indicator {
  border-color: #ff7588;
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background-color: #ff7588;
}

.add-profile-btn {
  padding: 12px;
  background-color: #26262a;
  border: 1px dashed #3a3a40;
  color: #d0d0d5;
  border-radius: 8px;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.add-profile-btn:hover {
  background-color: #303036;
  color: #ffffff;
  border-color: #ff7588;
}
</style>
