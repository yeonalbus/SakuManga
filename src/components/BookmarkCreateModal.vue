<script setup lang="ts">
// 搜刮书签确认弹窗（Round29 改造）
// 流程：点「存为书签」→ 直接进入拾取模式 → 点击卡片 → 本弹窗弹出（此时才出现）
//   · 自动带出锚定画廊的发布时间 + 页面位置信息（首页 / 搜索: xxx）
//   · 名称默认留空（留空时侧栏展示「位置 + 发布时间」）
//   · 「确认」保存；「取消」回到拾取模式（可改锚定其他卡片）
// 样式沿用 DateJumpModal 的 overlay/panel 暗色主题结构
import { ref, watch } from 'vue'
import type { ScrapeBookmarkRole } from '@/types/comic'

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
  (
    e: 'create',
    payload: {
      name: string
      role: ScrapeBookmarkRole
      note: string
      updateWatch: boolean
      bulkDone: boolean
    },
  ): void
}>()

const name = ref('')
/** Round49：书签标记（默认追新——最常见的创建场景就是搜刮到最新后存书签） */
const role = ref<ScrapeBookmarkRole>('newest')
const note = ref('')
const updateWatch = ref(false)
const bulkDone = ref(false)

const roleOptions: { value: ScrapeBookmarkRole; label: string; hint: string }[] = [
  { value: 'newest', label: '追新', hint: '标记上次搜刮到的最新位置' },
  { value: 'oldest', label: '续扫', hint: '标记上次搜刮到的最旧位置，下次继续向下扫' },
  { value: 'interest', label: '兴趣', hint: '只是感兴趣，暂未搜刮' },
]

// 每次打开重置
watch(
  () => props.show,
  (v) => {
    if (v) {
      name.value = ''
      role.value = 'newest'
      note.value = ''
      updateWatch.value = false
      bulkDone.value = false
    }
  },
)

const handleConfirm = () => {
  emit('create', {
    name: name.value.trim(),
    role: role.value,
    note: note.value.trim().slice(0, 40),
    updateWatch: updateWatch.value,
    bulkDone: bulkDone.value,
  })
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

            <!-- Round49：书签标题固定为「位置字段 + 非默认筛选摘要」，不再由用户手填； -->
            <!-- 书签标记 / 备注 / 更新探测 / 已囤货 均为书签项级配置 -->

            <div class="bm-field">
              <span class="bm-label">书签标记</span>
              <div class="bm-role-chips">
                <button
                  v-for="opt in roleOptions"
                  :key="opt.value"
                  type="button"
                  class="bm-chip"
                  :class="{ active: role === opt.value }"
                  :title="opt.hint"
                  @click="role = opt.value"
                >
                  {{ opt.label }}
                </button>
              </div>
            </div>

            <label class="bm-field">
              <span class="bm-label">备注（可选，最多 40 字）</span>
              <input
                v-model="note"
                type="text"
                class="bm-input"
                placeholder="例如：2026-10 已筛完这批"
                maxlength="40"
                @keyup.enter="handleConfirm"
              />
            </label>

            <div class="bm-toggles">
              <label class="bm-toggle">
                <input v-model="updateWatch" type="checkbox" />
                开启更新探测（每日检查是否有新内容）
              </label>
              <label class="bm-toggle">
                <input v-model="bulkDone" type="checkbox" />
                标记已囤货（该 tag 已做过无差别下载→筛选）
              </label>
            </div>
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

/* Round49：书签标记 chips / 开关行 */
.bm-role-chips {
  display: flex;
  gap: 6px;
}

.bm-chip {
  flex: 1;
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-3);
  color: var(--app-text-2);
  border-radius: 6px;
  padding: 6px 8px;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
}
.bm-chip:hover {
  color: var(--app-text-strong);
}
.bm-chip.active {
  border-color: #00a896;
  color: #00a896;
  background-color: rgba(0, 168, 150, 0.12);
}

.bm-toggles {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 10px;
  border-radius: 8px;
  background-color: var(--app-surface-3);
  border: 1px solid var(--app-border-2);
}

.bm-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.78rem;
  color: var(--app-text-2);
  cursor: pointer;
}
.bm-toggle input {
  accent-color: #00a896;
}
</style>
