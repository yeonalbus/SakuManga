<script setup lang="ts">
// 搜刮书签确认弹窗（Round29 改造）
// 流程：点「存为书签」→ 直接进入拾取模式 → 点击卡片 → 本弹窗弹出（此时才出现）
//   · 自动带出锚定画廊的发布时间 + 页面位置信息（首页 / 搜索: xxx）
//   · 名称默认留空（留空时侧栏展示「位置 + 发布时间」）
//   · 「确认」保存；「取消」回到拾取模式（可改锚定其他卡片）
// 样式沿用 DateJumpModal 的 overlay/panel 暗色主题结构
import { ref, watch } from 'vue'

const props = defineProps<{
  show: boolean
  type: 'home' | 'search'
  keyword: string
  /** 位置标签（由父组件按 bookmarkLocationLabel 生成，如「首页」「搜索: touhou」） */
  locationLabel: string
  /** 锚定画廊标题（可空） */
  anchorTitle?: string
  /** 锚定画廊发布时间（E 站 posted 日期，可空） */
  postedAt?: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'create', payload: { name: string }): void
}>()

const name = ref('')

// 每次打开重置为空（默认留空）
watch(
  () => props.show,
  (v) => {
    if (v) name.value = ''
  },
)

const handleConfirm = () => {
  emit('create', { name: name.value.trim() })
}
</script>

<template>
  <Teleport to="body">
    <Transition name="bm-fade">
      <div v-if="show" class="bm-overlay" @click.self="emit('close')">
        <div class="bm-panel">
          <div class="bm-header">
            <span class="bm-title">🔖 存为书签</span>
            <button class="bm-close" title="关闭" @click="emit('close')">✕</button>
          </div>

          <div class="bm-body">
            <!-- 自动记录的关键信息：位置（页面上下文）+ 发布时间 -->
            <div class="bm-info">
              <span class="bm-type-badge" :class="type">
                {{ type === 'search' ? '🔍 搜索内容' : '🏠 首页' }}
              </span>
              <span v-if="type === 'search' && keyword" class="bm-keyword" :title="keyword">
                {{ keyword }}
              </span>
              <span v-else class="bm-keyword muted">无搜索词（首页流）</span>
            </div>

            <div class="bm-record">
              <div class="bm-record-row">
                <span class="bm-record-label">📅 发布时间</span>
                <span class="bm-record-value">{{ postedAt || '未知' }}</span>
              </div>
              <div class="bm-record-row">
                <span class="bm-record-label">📍 位置</span>
                <span class="bm-record-value" :title="locationLabel">{{ locationLabel }}</span>
              </div>
              <div v-if="anchorTitle" class="bm-record-row">
                <span class="bm-record-label">🎴 锚定画廊</span>
                <span class="bm-record-value" :title="anchorTitle">{{ anchorTitle }}</span>
              </div>
            </div>

            <!-- 自定义名称（可选，默认留空） -->
            <label class="bm-field">
              <span class="bm-label">书签名称（可选）</span>
              <input
                v-model="name"
                type="text"
                class="bm-input"
                placeholder="留空则显示「位置 + 发布时间」"
                maxlength="40"
                @keyup.enter="handleConfirm"
              />
            </label>
          </div>

          <div class="bm-footer">
            <button class="bm-btn primary" @click="handleConfirm">确认</button>
            <button class="bm-btn ghost" @click="emit('close')">取消</button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* 与 DateJumpModal 一致的暗色主题弹窗骨架 */
.bm-overlay {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background-color: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
}

.bm-panel {
  width: 100%;
  max-width: 380px;
  background-color: var(--app-surface-2);
  border: 1px solid var(--app-border-3);
  border-radius: 12px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.55);
  overflow: hidden;
}

.bm-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--app-border-2);
}

.bm-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--app-text-strong);
}

.bm-close {
  background: transparent;
  border: none;
  color: var(--app-text-muted);
  cursor: pointer;
  font-size: 0.85rem;
  padding: 4px;
  border-radius: 6px;
}
.bm-close:hover {
  color: var(--app-text-strong);
  background-color: var(--app-surface-3);
}

.bm-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
}

.bm-info {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.bm-type-badge {
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.78rem;
  font-weight: 600;
  color: #ffffff;
  white-space: nowrap;
}
.bm-type-badge.search {
  background-color: #007acc;
}
.bm-type-badge.home {
  background-color: #00a896;
}

.bm-keyword {
  font-size: 0.8rem;
  color: var(--app-text-2);
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.bm-keyword.muted {
  color: var(--app-text-muted);
}

/* 自动记录信息块（发布时间 / 位置 / 锚定画廊） */
.bm-record {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 8px;
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-2);
}

.bm-record-row {
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-size: 0.8rem;
  min-width: 0;
}

.bm-record-label {
  color: var(--app-text-3);
  white-space: nowrap;
  flex-shrink: 0;
}

.bm-record-value {
  color: var(--app-text-strong);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bm-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.bm-label {
  font-size: 0.75rem;
  color: var(--app-text-3);
}

.bm-input {
  background-color: var(--app-input-bg);
  border: 1px solid var(--app-border-3);
  border-radius: 8px;
  color: var(--app-text-strong);
  font-size: 0.88rem;
  padding: 8px 10px;
  outline: none;
  transition: border-color 0.15s ease;
}
.bm-input:focus {
  border-color: #00a896;
}

.bm-footer {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--app-border-2);
}

.bm-btn {
  flex: 1;
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.bm-btn:hover {
  background-color: var(--app-surface-3-hover);
  color: var(--app-text-strong);
}
.bm-btn.primary {
  background-color: #00a896;
  border-color: #00a896;
  color: #ffffff;
}
.bm-btn.primary:hover {
  background-color: #00c4af;
  color: #ffffff;
}
.bm-btn.ghost {
  background: transparent;
  border-color: transparent;
}
.bm-btn.ghost:hover {
  background-color: var(--app-surface-3);
}

/* 淡入动画 */
.bm-fade-enter-active,
.bm-fade-leave-active {
  transition: opacity 0.18s ease;
}
.bm-fade-enter-from,
.bm-fade-leave-to {
  opacity: 0;
}
</style>
